package musicsocial

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/grrywlsn/plexify/track"
)

const defaultHTTPTimeout = 60 * time.Second

// Client fetches playlist data from a music-social instance over HTTPS.
type Client struct {
	base *url.URL
	http *http.Client
}

// NewClient parses baseURL (scheme + host, optional path) and returns a client.
func NewClient(baseURL string) (*Client, error) {
	s := strings.TrimSpace(baseURL)
	s = strings.TrimRight(s, "/")
	u, err := url.Parse(s)
	if err != nil {
		return nil, fmt.Errorf("parse base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return nil, fmt.Errorf("base URL must use http or https")
	}
	if u.Host == "" {
		return nil, fmt.Errorf("base URL must include a host")
	}

	return &Client{
		base: u,
		http: &http.Client{Timeout: defaultHTTPTimeout},
	}, nil
}

// PlaylistSummary is one row from GET /users/{username}/playlists.json
type PlaylistSummary struct {
	ID          string `json:"id"`
	Title       string `json:"title"`
	Description string `json:"description,omitempty"`
	TrackCount  int    `json:"track_count"`
	UpdatedAt   string `json:"updated_at"`
	URL         string `json:"url"`
}

// Playlist is a full playlist from GET /playlist/{id}.json
type Playlist struct {
	ID          string
	Title       string
	Description string
	Owner       string
	SourceURL   string
	TrackCount  int
	UpdatedAt   string
	Tracks      []track.Track
}

type playlistJSONDoc struct {
	ID          string             `json:"id"`
	Title       string             `json:"title"`
	Description string             `json:"description,omitempty"`
	Owner       string             `json:"owner"`
	SourceURL   string             `json:"source_url,omitempty"`
	TrackCount  int                `json:"track_count"`
	UpdatedAt   string             `json:"updated_at"`
	Tracks      []playlistTrackDTO `json:"tracks"`
}

type playlistTrackDTO struct {
	Position   int         `json:"position"`
	Title      string      `json:"title"`
	Artist     string      `json:"artist"`
	Album      string      `json:"album,omitempty"`
	DurationMS int         `json:"duration_ms,omitempty"`
	MB         *mbTrackDTO `json:"musicbrainz,omitempty"`
}

type mbTrackDTO struct {
	TrackGID string   `json:"track_gid"`
	ISRCs    []string `json:"isrcs,omitempty"`
}

// ListUserPlaylists returns public playlists for username.
func (c *Client) ListUserPlaylists(username string) ([]PlaylistSummary, error) {
	if username == "" {
		return nil, fmt.Errorf("username is required")
	}
	u := c.base.JoinPath("users", url.PathEscape(username), "playlists.json")
	body, status, err := c.get(u.String())
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("user not found: %s", username)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("list playlists: HTTP %d", status)
	}

	var out []PlaylistSummary
	if err := json.Unmarshal(body, &out); err != nil {
		return nil, fmt.Errorf("decode playlists list: %w", err)
	}
	return out, nil
}

// GetPlaylist fetches a single playlist with tracks.
func (c *Client) GetPlaylist(playlistID string) (*Playlist, error) {
	if playlistID == "" {
		return nil, fmt.Errorf("playlist id is required")
	}
	// Avoid path tricks: allow only simple ids (alphanumeric + common safe chars)
	if strings.ContainsAny(playlistID, "/?#") {
		return nil, fmt.Errorf("invalid playlist id")
	}

	u := c.base.JoinPath("playlist", playlistID+".json")
	body, status, err := c.get(u.String())
	if err != nil {
		return nil, err
	}
	if status == http.StatusNotFound {
		return nil, fmt.Errorf("playlist not found: %s", playlistID)
	}
	if status != http.StatusOK {
		return nil, fmt.Errorf("get playlist: HTTP %d", status)
	}

	var doc playlistJSONDoc
	if err := json.Unmarshal(body, &doc); err != nil {
		return nil, fmt.Errorf("decode playlist: %w", err)
	}

	tracks := make([]track.Track, 0, len(doc.Tracks))
	for _, tj := range doc.Tracks {
		tr := track.Track{
			ID:       fmt.Sprintf("%s:%d", doc.ID, tj.Position),
			Name:     tj.Title,
			Artist:   tj.Artist,
			Album:    tj.Album,
			Duration: tj.DurationMS,
		}
		if tj.MB != nil {
			tr.MusicBrainzID = tj.MB.TrackGID
			if len(tj.MB.ISRCs) > 0 {
				tr.ISRC = tj.MB.ISRCs[0]
			}
		}
		tracks = append(tracks, tr)
	}

	return &Playlist{
		ID:          doc.ID,
		Title:       doc.Title,
		Description: doc.Description,
		Owner:       doc.Owner,
		SourceURL:   doc.SourceURL,
		TrackCount:  doc.TrackCount,
		UpdatedAt:   doc.UpdatedAt,
		Tracks:      tracks,
	}, nil
}

// PlaylistPageURL builds the canonical human-readable playlist URL (no .json).
func (c *Client) PlaylistPageURL(playlistID string) string {
	return c.base.JoinPath("playlist", playlistID).String()
}

func (c *Client) get(rawURL string) ([]byte, int, error) {
	req, err := http.NewRequest(http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, 0, err
	}
	req.Header.Set("Accept", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("GET %s: %w", rawURL, err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read body: %w", err)
	}
	return body, resp.StatusCode, nil
}
