package plex

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strconv"
	"strings"
)

// sourceServiceDisplayName labels music-social.com in Plex descriptions and CLI diff output.
const sourceServiceDisplayName = "music-social.com"

// escapeDescription decodes HTML entities in playlist descriptions
func (c *Client) escapeDescription(description string) string {
	// Decode HTML entities to get the actual characters
	// This handles cases like &#x2F; -> /
	return html.UnescapeString(description)
}

// addSyncAttribution adds a sync attribution line to the description
func (c *Client) addSyncAttribution(description, sourcePlaylistURL string) string {
	if sourcePlaylistURL == "" {
		return description
	}

	syncLine := fmt.Sprintf("synced from %s: %s", sourceServiceDisplayName, sourcePlaylistURL)

	// If there's existing description, add newlines before the attribution
	if description != "" {
		return description + "\n\n" + syncLine
	}

	// If no existing description, just return the attribution
	return syncLine
}

// CreatePlaylist creates a new playlist with an initial track (required for sync operations)
func (c *Client) CreatePlaylist(ctx context.Context, title, description, trackURI, sourcePlaylistURL string) (*PlexPlaylist, error) {
	// Use the correct Plex API endpoint for playlist creation
	reqURL := fmt.Sprintf("%s/playlists", c.baseURL)

	// Add parameters to URL query string (matching Plex Web behavior)
	params := url.Values{}
	params.Add("type", "audio")
	params.Add("title", title)
	params.Add("smart", "0")
	params.Add("uri", trackURI)

	// Always add description with sync attribution if we have a source URL
	// or if there's an original description
	if sourcePlaylistURL != "" || description != "" {
		descriptionWithAttribution := c.addSyncAttribution(description, sourcePlaylistURL)
		escapedDescription := c.escapeDescription(descriptionWithAttribution)
		params.Add("summary", escapedDescription)
	}

	params.Add("X-Plex-Token", c.token)

	// Create request with empty body (matching Plex Web behavior)
	req, err := http.NewRequestWithContext(ctx, "POST", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create playlist request: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Plex-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make playlist creation request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK && resp.StatusCode != StatusCreated {
		// Read the response body to get more details about the error
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("plex playlist creation API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse the JSON response to get the created playlist
	var playlistResp struct {
		MediaContainer struct {
			Metadata []PlexPlaylistJSON `json:"Metadata"`
		} `json:"MediaContainer"`
	}

	if err := json.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return nil, fmt.Errorf("failed to decode playlist creation response: %w", err)
	}

	if len(playlistResp.MediaContainer.Metadata) == 0 {
		return nil, fmt.Errorf("no playlist returned from creation request")
	}

	// Convert JSON response to our standard PlexPlaylist struct
	jsonPlaylist := playlistResp.MediaContainer.Metadata[0]
	createdPlaylist := &PlexPlaylist{
		ID:          jsonPlaylist.ID,
		Title:       jsonPlaylist.Title,
		Description: jsonPlaylist.Description,
		TrackCount:  jsonPlaylist.TrackCount,
		CreatedAt:   fmt.Sprintf("%v", jsonPlaylist.CreatedAt),
		UpdatedAt:   fmt.Sprintf("%v", jsonPlaylist.UpdatedAt),
	}

	slog.Info(fmt.Sprintf("Successfully created playlist with track: %s (ID: %s)", createdPlaylist.Title, createdPlaylist.ID))

	return createdPlaylist, nil
}

// plexPlaylistsListContainer is the XML envelope for GET /playlists (paginated).
type plexPlaylistsListContainer struct {
	XMLName   xml.Name       `xml:"MediaContainer"`
	Size      int            `xml:"size,attr"`
	TotalSize int            `xml:"totalSize,attr"`
	Playlists []PlexPlaylist `xml:"Playlist"`
}

// GetPlaylists retrieves all playlists from the Plex server (paginated; default Plex page size can omit older playlists).
func (c *Client) GetPlaylists(ctx context.Context) ([]PlexPlaylist, error) {
	const pageSize = 200
	var all []PlexPlaylist
	offset := 0
	for {
		reqURL := fmt.Sprintf("%s/playlists", c.baseURL)
		params := url.Values{}
		params.Add("X-Plex-Token", c.token)
		params.Add("X-Plex-Container-Start", strconv.Itoa(offset))
		params.Add("X-Plex-Container-Size", strconv.Itoa(pageSize))

		req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("failed to create playlists request: %w", err)
		}
		req.Header.Set("Accept", "application/xml")
		req.Header.Set("X-Plex-Token", c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("failed to make playlists request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read playlists body: %w", readErr)
		}
		if resp.StatusCode != StatusOK {
			return nil, fmt.Errorf("plex playlists API returned status %d: %s", resp.StatusCode, string(body))
		}

		var container plexPlaylistsListContainer
		if err := xml.Unmarshal(body, &container); err != nil {
			return nil, fmt.Errorf("failed to decode playlists response: %w", err)
		}

		all = append(all, container.Playlists...)
		got := len(container.Playlists)
		if got == 0 {
			break
		}
		offset += got
		if container.TotalSize > 0 && len(all) >= container.TotalSize {
			break
		}
		if got < pageSize {
			break
		}
	}
	return all, nil
}

// UpdatePlaylistMetadata updates the metadata of an existing playlist
func (c *Client) UpdatePlaylistMetadata(ctx context.Context, playlistID, title, description, sourcePlaylistURL string) error {
	// Use the Plex API endpoint for updating playlist metadata
	reqURL := fmt.Sprintf("%s/playlists/%s", c.baseURL, playlistID)

	// Add parameters to URL query string
	params := url.Values{}
	params.Add("type", "audio")
	if title != "" {
		params.Add("title", title)
	}

	// Always add description with sync attribution if we have a source URL
	// or if there's an original description
	if sourcePlaylistURL != "" || description != "" {
		descriptionWithAttribution := c.addSyncAttribution(description, sourcePlaylistURL)
		escapedDescription := c.escapeDescription(descriptionWithAttribution)
		params.Add("summary", escapedDescription)
	}

	params.Add("X-Plex-Token", c.token)

	// Create request with PUT method for updates
	req, err := http.NewRequestWithContext(ctx, "PUT", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("failed to create playlist update request: %w", err)
	}

	req.Header.Set("Accept", "application/json, text/plain, */*")
	req.Header.Set("X-Plex-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make playlist update request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK && resp.StatusCode != StatusCreated {
		// Read the response body to get more details about the error
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plex playlist update API returned status %d: %s", resp.StatusCode, string(body))
	}

	slog.Info(fmt.Sprintf("Successfully updated playlist metadata: %s (ID: %s)", title, playlistID))
	return nil
}

// ClearPlaylist removes all tracks from an existing playlist
func (c *Client) ClearPlaylist(ctx context.Context, playlistID string) error {
	slog.Info(fmt.Sprintf("Clearing playlist %s", playlistID))

	// Use the Plex API endpoint to clear playlist items
	reqURL := fmt.Sprintf("%s/playlists/%s/items", c.baseURL, playlistID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)

	req, err := http.NewRequestWithContext(ctx, "DELETE", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return fmt.Errorf("failed to create playlist clear request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")
	req.Header.Set("X-Plex-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to make playlist clear request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK && resp.StatusCode != StatusNoContent {
		// Read the response body to get more details about the error
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("plex playlist clear API returned status %d: %s", resp.StatusCode, string(body))
	}

	slog.Info(fmt.Sprintf("Successfully cleared playlist: %s", playlistID))
	return nil
}

// plexPlaylistItemsContainer is the XML envelope for GET /playlists/{id}/items.
type plexPlaylistItemsContainer struct {
	XMLName   xml.Name    `xml:"MediaContainer"`
	Size      int         `xml:"size,attr"`
	TotalSize int         `xml:"totalSize,attr"`
	Offset    int         `xml:"offset,attr"`
	Tracks    []PlexTrack `xml:"Track"`
}

// GetPlaylistItems returns playlist entries in order (library tracks with ratingKey, title, artist).
func (c *Client) GetPlaylistItems(ctx context.Context, playlistID string) ([]PlexTrack, error) {
	const pageSize = 200
	var all []PlexTrack
	offset := 0
	for {
		reqURL := fmt.Sprintf("%s/playlists/%s/items", c.baseURL, playlistID)
		params := url.Values{}
		params.Add("X-Plex-Token", c.token)
		params.Add("X-Plex-Container-Start", strconv.Itoa(offset))
		params.Add("X-Plex-Container-Size", strconv.Itoa(pageSize))

		req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
		if err != nil {
			return nil, fmt.Errorf("create playlist items request: %w", err)
		}
		req.Header.Set("Accept", "application/xml")
		req.Header.Set("X-Plex-Token", c.token)

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("playlist items request: %w", err)
		}
		body, readErr := io.ReadAll(resp.Body)
		resp.Body.Close()
		if readErr != nil {
			return nil, fmt.Errorf("read playlist items body: %w", readErr)
		}
		if resp.StatusCode != StatusOK {
			return nil, fmt.Errorf("playlist items API status %d: %s", resp.StatusCode, string(body))
		}

		var container plexPlaylistItemsContainer
		if err := xml.Unmarshal(body, &container); err != nil {
			return nil, fmt.Errorf("decode playlist items: %w", err)
		}

		all = append(all, container.Tracks...)
		got := len(container.Tracks)
		if got == 0 {
			break
		}
		offset += got
		if container.TotalSize > 0 && len(all) >= container.TotalSize {
			break
		}
		if got < pageSize {
			break
		}
	}
	return all, nil
}

var leafCountAddedXMLRe = regexp.MustCompile(`(?i)leafCountAdded="([0-9]+)"`)

func parseLeafCountAdded(body []byte) (n int, ok bool) {
	m := leafCountAddedXMLRe.FindSubmatch(body)
	if len(m) < 2 {
		return 0, false
	}
	v, err := strconv.Atoi(string(m[1]))
	if err != nil {
		return 0, false
	}
	return v, true
}

// getPlaylistTrackAt returns a single playlist row by 0-based index (GET .../items with container window).
func (c *Client) getPlaylistTrackAt(ctx context.Context, playlistID string, index int) (*PlexTrack, error) {
	if index < 0 {
		return nil, fmt.Errorf("negative playlist index %d", index)
	}
	reqURL := fmt.Sprintf("%s/playlists/%s/items", c.baseURL, playlistID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("X-Plex-Container-Start", strconv.Itoa(index))
	params.Add("X-Plex-Container-Size", "1")

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("X-Plex-Token", c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return nil, readErr
	}
	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("playlist items API status %d: %s", resp.StatusCode, string(body))
	}

	var container plexPlaylistItemsContainer
	if err := xml.Unmarshal(body, &container); err != nil {
		return nil, fmt.Errorf("decode playlist items: %w", err)
	}
	if len(container.Tracks) != 1 {
		return nil, fmt.Errorf("expected 1 playlist row at index %d, got %d", index, len(container.Tracks))
	}
	t := container.Tracks[0]
	return &t, nil
}

func (c *Client) putPlaylistTrack(ctx context.Context, playlistID, trackID, afterPlaylistItemID string) (leafAdded int, err error) {
	reqURL := fmt.Sprintf("%s/playlists/%s/items", c.baseURL, playlistID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("uri", fmt.Sprintf("server://%s/com.plexapp.plugins.library/library/metadata/%s", c.serverID, trackID))
	if strings.TrimSpace(afterPlaylistItemID) != "" {
		params.Add("after", afterPlaylistItemID)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return 0, err
	}
	req.Header.Set("Accept", "application/xml")
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return 0, err
	}
	body, readErr := io.ReadAll(resp.Body)
	resp.Body.Close()
	if readErr != nil {
		return 0, readErr
	}
	if resp.StatusCode != StatusOK {
		return 0, fmt.Errorf("plex add-to-playlist status %d for track %s: %s", resp.StatusCode, trackID, string(body))
	}
	n, ok := parseLeafCountAdded(body)
	if !ok {
		// Some responses omit leafCountAdded; treat as success for backward compatibility.
		n = 1
	}
	return n, nil
}

// AddTracksToPlaylist adds tracks to an existing playlist in source order.
// Plex deduplicates bare PUT adds of the same library track; after the first row, each add uses after=<previous playlistItemID>
// so the same ratingKey can appear multiple times (matching source playlists with repeats).
func (c *Client) AddTracksToPlaylist(ctx context.Context, playlistID string, trackIDs []string) error {
	if len(trackIDs) == 0 {
		return nil
	}

	slog.Info(fmt.Sprintf("Adding %d tracks to playlist %s", len(trackIDs), playlistID))

	var after string
	for i, trackID := range trackIDs {
		if i > 0 && strings.TrimSpace(after) == "" {
			return fmt.Errorf("internal: empty playlistItemID anchor before add %d", i)
		}
		leaf, err := c.putPlaylistTrack(ctx, playlistID, trackID, after)
		if err != nil {
			return fmt.Errorf("add track %s to playlist %s: %w", trackID, playlistID, err)
		}
		if leaf < 1 {
			return fmt.Errorf("plex did not add track %s to playlist %s (leafCountAdded=0); repeating the same library item requires Plex append-after (after=) support", trackID, playlistID)
		}
		lastIdx := i
		last, err := c.getPlaylistTrackAt(ctx, playlistID, lastIdx)
		if err != nil {
			return fmt.Errorf("read playlist row after add: %w", err)
		}
		if strings.TrimSpace(last.PlaylistItemID) == "" {
			return fmt.Errorf("playlist row at index %d has no playlistItemID (cannot append further items)", lastIdx)
		}
		after = last.PlaylistItemID
		c.debugLog("playlist append anchor after index %d: playlistItemID=%s", lastIdx, after)
	}

	c.debugLog("Successfully added %d tracks to playlist %s", len(trackIDs), playlistID)
	return nil
}

// SetPlaylistPosterUsingPlexgo uses the plexgo SDK to set playlist poster
func (c *Client) SetPlaylistPosterUsingPlexgo(ctx context.Context, playlistID, artworkURL string) error {
	if artworkURL == "" {
		return nil // No artwork to set
	}

	// Convert playlist ID to int64 (plexgo expects int64 for ratingKey)
	ratingKey, err := strconv.ParseInt(playlistID, 10, 64)
	if err != nil {
		return fmt.Errorf("failed to convert playlist ID to int64: %w", err)
	}

	// Use plexgo SDK's PostMediaPoster function
	_, err = c.plexgoClient.Library.PostMediaPoster(ctx, ratingKey, &artworkURL, nil)
	if err != nil {
		return fmt.Errorf("plexgo SDK failed to set poster: %w", err)
	}

	return nil
}
