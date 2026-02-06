package musicbrainz

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Client wraps the MusicBrainz API client
type Client struct {
	httpClient *http.Client
	userAgent  string
}

// Recording represents a MusicBrainz recording
type Recording struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"title,attr"`
}

// Release represents a MusicBrainz release
type Release struct {
	ID   string `xml:"id,attr"`
	Name string `xml:"title,attr"`
}

// Track represents a MusicBrainz track with recording and release info
type Track struct {
	Recording Recording `xml:"recording"`
	Release   Release   `xml:"release"`
}

// SearchResponse represents the response from MusicBrainz search API
type SearchResponse struct {
	TrackList struct {
		Tracks []Track `xml:"track"`
	} `xml:"track-list"`
}

// ISRCResponse represents the response from MusicBrainz ISRC API
type ISRCResponse struct {
	ISRC struct {
		RecordingList struct {
			Recordings []Recording `xml:"recording"`
		} `xml:"recording-list"`
	} `xml:"isrc"`
}

// NewClient creates a new MusicBrainz client
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
		userAgent: "Plexify/1.0 (https://github.com/grrywlsn/plexify)",
	}
}

// GetMusicBrainzIDByISRC searches for a track by ISRC and returns the MusicBrainz recording ID
func (c *Client) GetMusicBrainzIDByISRC(ctx context.Context, isrc string) (string, error) {
	if isrc == "" {
		return "", fmt.Errorf("ISRC cannot be empty")
	}

	// MusicBrainz API endpoint for searching by ISRC
	baseURL := "https://musicbrainz.org/ws/2/isrc/"

	// Build query parameters
	params := url.Values{}
	params.Add("fmt", "xml")

	// Create request URL with ISRC in the path
	reqURL := baseURL + isrc + "?" + params.Encode()

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers for MusicBrainz API
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/xml")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("MusicBrainz API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse XML response
	var isrcResp ISRCResponse
	if err := xml.NewDecoder(resp.Body).Decode(&isrcResp); err != nil {
		return "", fmt.Errorf("failed to decode XML response: %w", err)
	}

	// Check if we found any recordings
	if len(isrcResp.ISRC.RecordingList.Recordings) == 0 {
		return "", fmt.Errorf("no recordings found for ISRC: %s", isrc)
	}

	// Return the first recording's ID
	return isrcResp.ISRC.RecordingList.Recordings[0].ID, nil
}

// GetMusicBrainzIDByArtistAndTitle searches for a track by artist and title
func (c *Client) GetMusicBrainzIDByArtistAndTitle(ctx context.Context, artist, title string) (string, error) {
	if artist == "" || title == "" {
		return "", fmt.Errorf("artist and title cannot be empty")
	}

	// MusicBrainz API endpoint for searching recordings
	baseURL := "https://musicbrainz.org/ws/2/recording/"

	// Build query parameters - search for artist and title
	query := fmt.Sprintf("artist:\"%s\" AND recording:\"%s\"",
		strings.ReplaceAll(artist, "\"", "\\\""),
		strings.ReplaceAll(title, "\"", "\\\""))

	params := url.Values{}
	params.Add("query", query)
	params.Add("fmt", "xml")

	// Create request
	req, err := http.NewRequestWithContext(ctx, "GET", baseURL+"?"+params.Encode(), nil)
	if err != nil {
		return "", fmt.Errorf("failed to create request: %w", err)
	}

	// Set required headers for MusicBrainz API
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/xml")

	// Make request
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("failed to make request: %w", err)
	}
	defer resp.Body.Close()

	// Check response status
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("MusicBrainz API returned status %d: %s", resp.StatusCode, string(body))
	}

	// Parse XML response
	var searchResp SearchResponse
	if err := xml.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return "", fmt.Errorf("failed to decode XML response: %w", err)
	}

	// Check if we found any tracks
	if len(searchResp.TrackList.Tracks) == 0 {
		return "", fmt.Errorf("no tracks found for artist: %s, title: %s", artist, title)
	}

	// Return the first track's recording ID
	return searchResp.TrackList.Tracks[0].Recording.ID, nil
}
