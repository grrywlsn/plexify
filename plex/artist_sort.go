package plex

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"sort"
	"strings"
)

// Threshold for including a track's grandparent artist in sort-title enrichment during full-library scan.
const libraryArtistSortEnrichTitleThreshold = 0.85

// Max distinct artist rating keys to fetch when enriching full-library results (after title filter).
const libraryArtistSortEnrichMaxKeys = 500

type plexArtistMediaContainer struct {
	XMLName xml.Name       `xml:"MediaContainer"`
	Artists []plexArtistEl `xml:"Artist"`
	Dirs    []plexArtistEl `xml:"Directory"` // some Plex builds use Directory for artists
}

type plexArtistEl struct {
	Title     string `xml:"title,attr"`
	TitleSort string `xml:"titleSort,attr"`
	Type      string `xml:"type,attr"`
}

func (c *Client) ensureArtistSortCache() {
	c.artistSortMu.Lock()
	defer c.artistSortMu.Unlock()
	if c.artistSortCache == nil {
		c.artistSortCache = make(map[string]string)
	}
}

// getArtistTitleSort fetches Plex Artist titleSort for an artist rating key (Sort Artist in the UI).
func (c *Client) getArtistTitleSort(ctx context.Context, artistRatingKey string) (string, error) {
	key := strings.TrimSpace(artistRatingKey)
	if key == "" {
		return "", nil
	}
	reqURL := fmt.Sprintf("%s/library/metadata/%s", strings.TrimSuffix(c.baseURL, "/"), url.PathEscape(key))
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("artist metadata request: %w", err)
	}
	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpDo(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return "", newPlexHTTPError(resp.StatusCode, "artist metadata", b)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", err
	}
	var mc plexArtistMediaContainer
	if err := xml.Unmarshal(raw, &mc); err != nil {
		return "", fmt.Errorf("decode artist metadata: %w", err)
	}
	el := firstArtistElement(mc)
	if el == nil {
		return "", nil
	}
	ts := strings.TrimSpace(el.TitleSort)
	if ts == "" {
		ts = strings.TrimSpace(el.Title)
	}
	return ts, nil
}

func firstArtistElement(mc plexArtistMediaContainer) *plexArtistEl {
	for i := range mc.Artists {
		return &mc.Artists[i]
	}
	for i := range mc.Dirs {
		if strings.EqualFold(strings.TrimSpace(mc.Dirs[i].Type), "artist") {
			return &mc.Dirs[i]
		}
	}
	if len(mc.Dirs) > 0 {
		return &mc.Dirs[0]
	}
	return nil
}

// getArtistTitleSortCached returns titleSort for key, using an in-memory cache.
func (c *Client) getArtistTitleSortCached(ctx context.Context, artistRatingKey string) (string, error) {
	key := strings.TrimSpace(artistRatingKey)
	if key == "" {
		return "", nil
	}
	c.ensureArtistSortCache()
	c.artistSortMu.Lock()
	if ts, ok := c.artistSortCache[key]; ok {
		c.artistSortMu.Unlock()
		return ts, nil
	}
	c.artistSortMu.Unlock()

	ts, err := c.getArtistTitleSort(ctx, key)
	if err != nil {
		return "", err
	}
	c.artistSortMu.Lock()
	c.artistSortCache[key] = ts
	c.artistSortMu.Unlock()
	return ts, nil
}

// enrichGrandparentSortTitles fills GrandparentTitleSort on tracks. If keysFilter is nil, every distinct
// GrandparentRatingKey in tracks is fetched. If non-nil, only keys present in the map are fetched and applied.
func (c *Client) enrichGrandparentSortTitles(ctx context.Context, tracks []PlexTrack, keysFilter map[string]struct{}) error {
	wantKeys := keysFilter
	if wantKeys == nil {
		wantKeys = make(map[string]struct{})
		for _, tr := range tracks {
			if k := strings.TrimSpace(tr.GrandparentRatingKey); k != "" {
				wantKeys[k] = struct{}{}
			}
		}
	}
	for k := range wantKeys {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if _, err := c.getArtistTitleSortCached(ctx, k); err != nil {
			slog.WarnContext(ctx, "plex artist sort fetch failed; continuing without titleSort for key",
				"key", k, "err", err)
		}
	}
	c.ensureArtistSortCache()
	c.artistSortMu.Lock()
	cache := c.artistSortCache
	c.artistSortMu.Unlock()

	for i := range tracks {
		k := strings.TrimSpace(tracks[i].GrandparentRatingKey)
		if k == "" {
			continue
		}
		if keysFilter != nil {
			if _, ok := keysFilter[k]; !ok {
				continue
			}
		}
		if ts, ok := cache[k]; ok {
			tracks[i].GrandparentTitleSort = ts
		}
	}
	return nil
}

type keyScore struct {
	key   string
	score float64
}

// grandparentKeysForLibrarySortEnrichment returns artist rating keys to enrich after Pass 1 fails on /all:
// keys whose track has title similarity to queryTitle above libraryArtistSortEnrichTitleThreshold,
// capped at libraryArtistSortEnrichMaxKeys by best title score.
func (c *Client) grandparentKeysForLibrarySortEnrichment(tracks []PlexTrack, queryTitle string) map[string]struct{} {
	tl := strings.ToLower(strings.TrimSpace(queryTitle))
	best := make(map[string]float64)
	for _, tr := range tracks {
		k := strings.TrimSpace(tr.GrandparentRatingKey)
		if k == "" {
			continue
		}
		ts := c.calculateStringSimilarity(tl, strings.ToLower(strings.TrimSpace(tr.Title)))
		if ts < libraryArtistSortEnrichTitleThreshold {
			continue
		}
		if prev, ok := best[k]; !ok || ts > prev {
			best[k] = ts
		}
	}
	if len(best) == 0 {
		return nil
	}
	var pairs []keyScore
	for k, s := range best {
		pairs = append(pairs, keyScore{key: k, score: s})
	}
	sort.Slice(pairs, func(i, j int) bool {
		if pairs[i].score != pairs[j].score {
			return pairs[i].score > pairs[j].score
		}
		return pairs[i].key < pairs[j].key
	})
	out := make(map[string]struct{})
	for i := 0; i < len(pairs) && i < libraryArtistSortEnrichMaxKeys; i++ {
		out[pairs[i].key] = struct{}{}
	}
	return out
}

// findBestMatchWithOptionalArtistSortRetry runs FindBestMatch without GrandparentTitleSort; if it returns nil,
// enriches artist sort titles and runs FindBestMatch once more. libraryScan triggers bounded key selection for /all.
func (c *Client) findBestMatchWithOptionalArtistSortRetry(ctx context.Context, tracks []PlexTrack, title, artist, sourceAlbum string, libraryScan bool) *PlexTrack {
	if len(tracks) == 0 {
		return nil
	}
	if first := c.FindBestMatch(tracks, title, artist, sourceAlbum); first != nil {
		return first
	}
	var keysFilter map[string]struct{}
	if libraryScan {
		keysFilter = c.grandparentKeysForLibrarySortEnrichment(tracks, title)
		if len(keysFilter) == 0 {
			return nil
		}
	}
	if err := c.enrichGrandparentSortTitles(ctx, tracks, keysFilter); err != nil {
		slog.WarnContext(ctx, "enrich grandparent sort titles failed", "err", err)
		return nil
	}
	return c.FindBestMatch(tracks, title, artist, sourceAlbum)
}
