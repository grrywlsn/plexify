package lidarr

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/grrywlsn/plexify/config"
)

// Client calls the Lidarr HTTP API (v1).
type Client struct {
	base   string
	apiKey string
	http   *http.Client

	addDefMu    sync.Mutex
	addDefCache *addDefaults
}

type addDefaults struct {
	rootFolderPath    string
	qualityProfileID  int
	metadataProfileID int
}

// NewClient builds a client for the given Lidarr config. Base URL and API key must be non-empty.
func NewClient(cfg *config.LidarrConfig) (*Client, error) {
	if cfg == nil {
		return nil, fmt.Errorf("nil Lidarr config")
	}
	base := strings.TrimRight(strings.TrimSpace(cfg.URL), "/")
	key := strings.TrimSpace(cfg.Token)
	if base == "" || key == "" {
		return nil, fmt.Errorf("Lidarr base URL and API token are required")
	}
	var tr http.RoundTripper
	if dt, ok := http.DefaultTransport.(*http.Transport); ok {
		t := dt.Clone()
		if cfg.InsecureSkipVerify {
			t.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
		}
		tr = t
	} else if cfg.InsecureSkipVerify {
		tr = &http.Transport{TLSClientConfig: &tls.Config{InsecureSkipVerify: true}}
	} else {
		tr = http.DefaultTransport
	}
	return &Client{
		base:   base,
		apiKey: key,
		http: &http.Client{
			Timeout:   60 * time.Second,
			Transport: tr,
		},
	}, nil
}

// AddReleaseGroupResult is the outcome of trying to add a MusicBrainz release group to Lidarr.
type AddReleaseGroupResult struct {
	ReleaseGroupID string
	AlreadyPresent bool
	Added          bool
	// EnsuredMonitored is true when the release group was already in Lidarr and we successfully ran
	// monitor/album/artist refresh (PUT album/monitor, PUT album, PUT artist as needed).
	EnsuredMonitored bool
}

// AddReleaseGroupIfMissing looks up a MusicBrainz release group (Lidarr foreign album id), adds it
// with search when not already in the Lidarr library, or when already present forces album, releases,
// and artist monitored via the Lidarr API so missing tracks stay visible.
// Transient write failures (e.g. Lidarr SQLite read-only) are retried with exponential backoff.
func (c *Client) AddReleaseGroupIfMissing(ctx context.Context, releaseGroupMBID string) (AddReleaseGroupResult, error) {
	rid := strings.TrimSpace(releaseGroupMBID)
	out := AddReleaseGroupResult{ReleaseGroupID: rid}
	if rid == "" {
		return out, fmt.Errorf("empty release group id")
	}

	var lastErr error
	for attempt := 0; attempt < writeMaxAttempts; attempt++ {
		if attempt > 0 {
			delay := writeBackoffFunc(attempt - 1)
			reason := lidarrFailureReason(lastErr)
			slog.WarnContext(ctx, "lidarr: transient write error; retrying",
				"release_group", rid, "attempt", attempt+1, "max", writeMaxAttempts, "backoff", delay, "reason", reason, "err", lastErr)
			fmt.Printf("⏳ Lidarr: temporarily unavailable (%s); retrying in %s (%d/%d)\n",
				reason, delay.Round(time.Second), attempt+1, writeMaxAttempts)
			select {
			case <-ctx.Done():
				return out, ctx.Err()
			case <-time.After(delay):
			}
		}

		res, err := c.addReleaseGroupIfMissingOnce(ctx, rid)
		if err == nil {
			return res, nil
		}
		lastErr = err
		if ctx.Err() != nil {
			return out, ctx.Err()
		}
		if !isTransientLidarrErr(err) {
			return out, err
		}
		if attempt+1 >= writeMaxAttempts {
			return out, &WriteError{ReleaseGroupID: rid, Reason: lidarrFailureReason(err)}
		}
	}
	return out, &WriteError{ReleaseGroupID: rid, Reason: lidarrFailureReason(lastErr)}
}

func (c *Client) addReleaseGroupIfMissingOnce(ctx context.Context, rid string) (AddReleaseGroupResult, error) {
	out := AddReleaseGroupResult{ReleaseGroupID: rid}

	existing, err := c.fetchAlbumsByForeignAlbumID(ctx, rid)
	if err != nil {
		return out, err
	}
	if len(existing) > 0 {
		out.AlreadyPresent = true
		if err := c.ensureExistingAlbumsMonitored(ctx, existing); err != nil {
			return out, err
		}
		out.EnsuredMonitored = true
		return out, nil
	}

	album, err := c.lookupFirstAlbum(ctx, rid)
	if err != nil {
		return out, err
	}
	if err := c.applyArtistAddDefaults(ctx, album); err != nil {
		return out, err
	}
	if err := c.postAlbum(ctx, album); err != nil {
		return out, err
	}
	out.Added = true
	return out, nil
}

func (c *Client) fetchAlbumsByForeignAlbumID(ctx context.Context, foreignAlbumID string) ([]map[string]interface{}, error) {
	u, err := url.Parse(c.base + "/api/v1/album")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("foreignAlbumId", foreignAlbumID)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	c.setDefaultHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("list album by foreign id: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var albums []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&albums); err != nil {
		return nil, fmt.Errorf("decode album list: %w", err)
	}
	return albums, nil
}

func (c *Client) putAlbumsMonitor(ctx context.Context, albumIDs []int, monitored bool) error {
	if len(albumIDs) == 0 {
		return nil
	}
	ids := make([]interface{}, len(albumIDs))
	for i, id := range albumIDs {
		ids[i] = id
	}
	payload := map[string]interface{}{
		"albumIds":  ids,
		"monitored": monitored,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	u := c.base + "/api/v1/album/monitor"
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT album/monitor: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) getAlbumJSON(ctx context.Context, albumID int) (map[string]interface{}, error) {
	u := c.base + "/api/v1/album/" + strconv.Itoa(albumID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setDefaultHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET album %d: %s: %s", albumID, resp.Status, strings.TrimSpace(string(b)))
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode album %d: %w", albumID, err)
	}
	return out, nil
}

func (c *Client) putAlbumJSON(ctx context.Context, albumID int, album map[string]interface{}) error {
	u := c.base + "/api/v1/album/" + strconv.Itoa(albumID)
	body, err := json.Marshal(album)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT album %d: %s: %s", albumID, resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) getArtistJSON(ctx context.Context, artistID int) (map[string]interface{}, error) {
	u := c.base + "/api/v1/artist/" + strconv.Itoa(artistID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	c.setDefaultHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("GET artist %d: %s: %s", artistID, resp.Status, strings.TrimSpace(string(b)))
	}
	var out map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, fmt.Errorf("decode artist %d: %w", artistID, err)
	}
	return out, nil
}

func (c *Client) putArtistJSON(ctx context.Context, artistID int, artist map[string]interface{}) error {
	u := c.base + "/api/v1/artist/" + strconv.Itoa(artistID)
	body, err := json.Marshal(artist)
	if err != nil {
		return err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("PUT artist %d: %s: %s", artistID, resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

var lidarrReleaseJSONKeys = []string{"releases", "Releases", "albumReleases", "AlbumReleases"}

func jsonTruthy(v interface{}) bool {
	switch x := v.(type) {
	case bool:
		return x
	case float64:
		return x != 0
	case int:
		return x != 0
	case int64:
		return x != 0
	case json.Number:
		i, err := x.Int64()
		return err == nil && i != 0
	case string:
		s := strings.ToLower(strings.TrimSpace(x))
		return s == "true" || s == "1"
	default:
		return false
	}
}

// ensureAtMostOneMonitoredReleasePerAlbum leaves exactly one monitored=true release row per
// distinct release id on the album payload. Lidarr only supports one monitored AlbumRelease per
// album; multiple break the UI/API (Lidarr#3784, "Sequence contains more than one element").
func ensureAtMostOneMonitoredReleasePerAlbum(album map[string]interface{}) {
	if album == nil {
		return
	}
	var orderedIDs []int
	seenID := make(map[int]struct{})
	for _, key := range lidarrReleaseJSONKeys {
		list, ok := album[key].([]interface{})
		if !ok {
			continue
		}
		for _, item := range list {
			rel, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			id := intFromInterface(rel["id"])
			if id > 0 {
				if _, dup := seenID[id]; dup {
					continue
				}
				seenID[id] = struct{}{}
				orderedIDs = append(orderedIDs, id)
			}
		}
	}
	if len(orderedIDs) == 0 {
		// No numeric ids (unusual): pick the first release in stable key order (key + slice index) as
		// the sole monitored row. Go maps cannot be compared with ==, so we use position, not pointers.
		var winKey string
		winIdx := -1
	outer:
		for _, key := range lidarrReleaseJSONKeys {
			list, ok := album[key].([]interface{})
			if !ok {
				continue
			}
			for i, item := range list {
				if _, ok := item.(map[string]interface{}); ok {
					winKey, winIdx = key, i
					break outer
				}
			}
		}
		if winIdx < 0 || winKey == "" {
			return
		}
		for _, key := range lidarrReleaseJSONKeys {
			list, ok := album[key].([]interface{})
			if !ok {
				continue
			}
			for i, item := range list {
				rel, ok := item.(map[string]interface{})
				if !ok {
					continue
				}
				rel["monitored"] = key == winKey && i == winIdx
			}
		}
		return
	}
	winnerID := orderedIDs[0]
	for _, id := range orderedIDs {
		if r, ok := releaseMapByID(album, id); ok && jsonTruthy(r["monitored"]) {
			winnerID = id
			break
		}
	}
	for _, key := range lidarrReleaseJSONKeys {
		list, ok := album[key].([]interface{})
		if !ok {
			continue
		}
		for _, item := range list {
			rel, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			id := intFromInterface(rel["id"])
			rel["monitored"] = id > 0 && id == winnerID
		}
	}
}

func releaseMapByID(album map[string]interface{}, want int) (map[string]interface{}, bool) {
	for _, key := range lidarrReleaseJSONKeys {
		list, ok := album[key].([]interface{})
		if !ok {
			continue
		}
		for _, item := range list {
			rel, ok := item.(map[string]interface{})
			if !ok {
				continue
			}
			if intFromInterface(rel["id"]) == want {
				return rel, true
			}
		}
	}
	return nil, false
}

// applyMonitoringToExistingAlbumMap sets monitored on the album and nested artist, and ensures
// exactly one monitored album release per Lidarr album (see ensureAtMostOneMonitoredReleasePerAlbum).
// Used with GET /api/v1/album/{id} payloads so Lidarr lists missing tracks for that release group.
func applyMonitoringToExistingAlbumMap(album map[string]interface{}) {
	if album == nil {
		return
	}
	album["monitored"] = true
	if art, ok := album["artist"].(map[string]interface{}); ok && art != nil {
		art["monitored"] = true
	}
	ensureAtMostOneMonitoredReleasePerAlbum(album)
}

func (c *Client) ensureArtistMonitored(ctx context.Context, artistID int) error {
	art, err := c.getArtistJSON(ctx, artistID)
	if err != nil {
		return err
	}
	art["monitored"] = true
	return c.putArtistJSON(ctx, artistID, art)
}

// ensureExistingAlbumsMonitored forces album and artist monitored in Lidarr when the MusicBrainz release
// group is already in the library (e.g. Plexify added it earlier unmonitored). Exactly one album release
// per album is left monitored on the PUT payload (Lidarr#3784).
func (c *Client) ensureExistingAlbumsMonitored(ctx context.Context, libraryAlbums []map[string]interface{}) error {
	var albumIDs []int
	for _, a := range libraryAlbums {
		id := intFromInterface(a["id"])
		if id > 0 {
			albumIDs = append(albumIDs, id)
		}
	}
	if len(albumIDs) == 0 {
		return fmt.Errorf("Lidarr album list by foreignAlbumId returned no usable album id")
	}
	if err := c.putAlbumsMonitor(ctx, albumIDs, true); err != nil {
		return err
	}
	artistSeen := make(map[int]struct{})
	for _, albumID := range albumIDs {
		full, err := c.getAlbumJSON(ctx, albumID)
		if err != nil {
			return err
		}
		applyMonitoringToExistingAlbumMap(full)
		if err := c.putAlbumJSON(ctx, albumID, full); err != nil {
			return err
		}
		if art, ok := full["artist"].(map[string]interface{}); ok && art != nil {
			aid := intFromInterface(art["id"])
			if aid > 0 {
				if _, ok := artistSeen[aid]; !ok {
					artistSeen[aid] = struct{}{}
					if err := c.ensureArtistMonitored(ctx, aid); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

func (c *Client) lookupFirstAlbum(ctx context.Context, releaseGroupMBID string) (map[string]interface{}, error) {
	terms := []string{"lidarr:" + releaseGroupMBID, releaseGroupMBID}
	for _, term := range terms {
		album, err := c.albumLookup(ctx, term)
		if err != nil {
			return nil, err
		}
		if album != nil {
			return album, nil
		}
	}
	return nil, fmt.Errorf("Lidarr album/lookup returned no results for release group %s", releaseGroupMBID)
}

func (c *Client) albumLookup(ctx context.Context, term string) (map[string]interface{}, error) {
	u, err := url.Parse(c.base + "/api/v1/album/lookup")
	if err != nil {
		return nil, err
	}
	q := u.Query()
	q.Set("term", term)
	u.RawQuery = q.Encode()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	c.setDefaultHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("album/lookup: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var albums []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&albums); err != nil {
		return nil, fmt.Errorf("decode lookup: %w", err)
	}
	if len(albums) == 0 {
		return nil, nil
	}
	return albums[0], nil
}

// applyArtistAddDefaults sets artist.qualityProfileId, artist.metadataProfileId, and artist.rootFolderPath
// when the lookup payload has 0/empty values — Lidarr requires these for a new artist. Uses the first
// root folder from GET /api/v1/rootfolder (and its default* profile ids when set), else first profile
// from GET /api/v1/qualityprofile and GET /api/v1/metadataprofile. Results are cached per Client.
func (c *Client) applyArtistAddDefaults(ctx context.Context, album map[string]interface{}) error {
	def, err := c.getAddDefaults(ctx)
	if err != nil {
		return err
	}
	art, ok := album["artist"].(map[string]interface{})
	if !ok || art == nil {
		art = make(map[string]interface{})
		album["artist"] = art
	}
	if !positiveIntFromJSON(art["qualityProfileId"]) {
		art["qualityProfileId"] = def.qualityProfileID
	}
	if !positiveIntFromJSON(art["metadataProfileId"]) {
		art["metadataProfileId"] = def.metadataProfileID
	}
	if s, _ := art["rootFolderPath"].(string); strings.TrimSpace(s) == "" {
		art["rootFolderPath"] = def.rootFolderPath
	}
	return nil
}

func (c *Client) getAddDefaults(ctx context.Context) (*addDefaults, error) {
	c.addDefMu.Lock()
	defer c.addDefMu.Unlock()
	if c.addDefCache != nil {
		return c.addDefCache, nil
	}
	d, err := c.fetchAddDefaults(ctx)
	if err != nil {
		return nil, err
	}
	c.addDefCache = d
	return c.addDefCache, nil
}

func (c *Client) fetchAddDefaults(ctx context.Context) (*addDefaults, error) {
	roots, err := c.getJSONSlice(ctx, "/api/v1/rootfolder")
	if err != nil {
		return nil, fmt.Errorf("root folder: %w", err)
	}
	if len(roots) == 0 {
		return nil, fmt.Errorf("Lidarr has no root folders configured (Settings → Media Management → Root Folders)")
	}
	root := roots[0]
	path, _ := root["path"].(string)
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("first Lidarr root folder has no path")
	}
	out := &addDefaults{rootFolderPath: path}

	qp := intFromInterface(root["defaultQualityProfileId"])
	mp := intFromInterface(root["defaultMetadataProfileId"])
	if qp <= 0 {
		profiles, err := c.getJSONSlice(ctx, "/api/v1/qualityprofile")
		if err != nil {
			return nil, fmt.Errorf("quality profile: %w", err)
		}
		if len(profiles) == 0 {
			return nil, fmt.Errorf("Lidarr has no quality profiles")
		}
		qp = intFromInterface(profiles[0]["id"])
	}
	if mp <= 0 {
		profiles, err := c.getJSONSlice(ctx, "/api/v1/metadataprofile")
		if err != nil {
			return nil, fmt.Errorf("metadata profile: %w", err)
		}
		if len(profiles) == 0 {
			return nil, fmt.Errorf("Lidarr has no metadata profiles")
		}
		mp = intFromInterface(profiles[0]["id"])
	}
	if qp <= 0 || mp <= 0 {
		return nil, fmt.Errorf("could not resolve default quality (%d) or metadata (%d) profile id", qp, mp)
	}
	out.qualityProfileID = qp
	out.metadataProfileID = mp
	return out, nil
}

func (c *Client) getJSONSlice(ctx context.Context, path string) ([]map[string]interface{}, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.base+path, nil)
	if err != nil {
		return nil, err
	}
	c.setDefaultHeaders(req)
	resp, err := c.http.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("%s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	var out []map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func intFromInterface(v interface{}) int {
	switch x := v.(type) {
	case float64:
		return int(x)
	case int:
		return x
	case int64:
		return int(x)
	case json.Number:
		i, _ := x.Int64()
		return int(i)
	default:
		return 0
	}
}

func positiveIntFromJSON(v interface{}) bool {
	return intFromInterface(v) > 0
}

// ensureMonitoredAddPayload forces the nested artist to monitored and addOptions.monitor=all so Lidarr tracks
// missing files (Wanted → Missing). At most one lookup release row is marked monitored per album id
// (Lidarr#3784). Lidarr's AddArtistService sets artist.Monitored=false when artist.addOptions.monitor is
// "none" (common on lookup payloads); we override to "all" for this add path.
func ensureMonitoredAddPayload(album map[string]interface{}) {
	if album == nil {
		return
	}
	art, ok := album["artist"].(map[string]interface{})
	if !ok || art == nil {
		return
	}
	art["monitored"] = true
	var ao map[string]interface{}
	if existing, ok := art["addOptions"].(map[string]interface{}); ok && existing != nil {
		ao = existing
	} else {
		ao = make(map[string]interface{})
		art["addOptions"] = ao
	}
	ao["monitor"] = "all"

	ensureAtMostOneMonitoredReleasePerAlbum(album)
}

func (c *Client) postAlbum(ctx context.Context, album map[string]interface{}) error {
	ensureMonitoredAddPayload(album)
	album["monitored"] = true
	album["addOptions"] = map[string]interface{}{
		"searchForNewAlbum": true,
	}
	body, err := json.Marshal(album)
	if err != nil {
		return err
	}
	u := c.base + "/api/v1/album"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, u, bytes.NewReader(body))
	if err != nil {
		return err
	}
	c.setDefaultHeaders(req)
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("add album: %s: %s", resp.Status, strings.TrimSpace(string(b)))
	}
	return nil
}

func (c *Client) setDefaultHeaders(req *http.Request) {
	req.Header.Set("X-Api-Key", c.apiKey)
	req.Header.Set("Accept", "application/json")
}
