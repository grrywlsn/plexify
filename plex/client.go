package plex

import (
	"context"
	"crypto/tls"
	"encoding/xml"
	"fmt"
	"log/slog"
	"net/http"
	"net/url"
	"time"

	"github.com/LukeHagar/plexgo"
	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/track"
)

// MatchKind describes how a source track was matched in Plex.
type MatchKind string

// Constants for Plex API
const (
	// Plex API constants
	PlexMusicTrackType = "10"

	// HTTP timeouts
	DefaultHTTPTimeout = 30 * time.Second

	// Match types (typed string constants)
	MatchTypeTitleArtist MatchKind = "title_artist"
	MatchTypeNone        MatchKind = "none"
	MatchTypeError       MatchKind = "error"
	// MatchKindISRC is reserved for tests / future ISRC-based confidence.
	MatchKindISRC MatchKind = "isrc"

	// HTTP status codes
	StatusOK        = http.StatusOK
	StatusCreated   = http.StatusCreated
	StatusNoContent = http.StatusNoContent

	// Search parameters
	SearchLimit = 100
)

// Client wraps the Plex API client
type Client struct {
	baseURL               string
	token                 string
	sectionID             int
	serverID              string
	httpClient            *http.Client
	debug                 bool
	plexgoClient          *plexgo.PlexAPI
	matchConcurrency      int
	dryRun                bool
	skipFullLibrarySearch bool
	exactMatchesOnly      bool
	// matchConfidencePercent is the minimum combined match score (0–100) as a fraction in minMatchScore; nil means use config.DefaultMatchConfidencePercent (for tests using &Client{}).
	matchConfidencePercent *int
}

// PlexTrack represents a track from Plex
type PlexTrack struct {
	ID        string `xml:"ratingKey,attr"`
	Title     string `xml:"title,attr"`
	Artist    string `xml:"grandparentTitle,attr"`
	Album     string `xml:"parentTitle,attr"`
	Duration  int    `xml:"duration,attr"`
	AddedAt   string `xml:"addedAt,attr"`
	UpdatedAt string `xml:"updatedAt,attr"`
	File      string `xml:"file,attr"`
}

// PlexPlaylist represents a Plex playlist
type PlexPlaylist struct {
	ID          string `xml:"ratingKey,attr" json:"ratingKey"`
	Title       string `xml:"title,attr" json:"title"`
	Description string `xml:"summary,attr" json:"summary"`
	TrackCount  int    `xml:"leafCount,attr" json:"leafCount"`
	CreatedAt   string `xml:"createdAt,attr" json:"createdAt"`
	UpdatedAt   string `xml:"updatedAt,attr" json:"updatedAt"`
}

// PlexPlaylistJSON is used for JSON responses where timestamps are numbers
type PlexPlaylistJSON struct {
	ID          string      `json:"ratingKey"`
	Title       string      `json:"title"`
	Description string      `json:"summary"`
	TrackCount  int         `json:"leafCount"`
	CreatedAt   interface{} `json:"createdAt"` // Can be string or number
	UpdatedAt   interface{} `json:"updatedAt"` // Can be string or number
}

// PlexResponse represents the XML response from Plex API
type PlexResponse struct {
	XMLName   xml.Name       `xml:"MediaContainer"`
	Tracks    []PlexTrack    `xml:"Track"`
	Playlists []PlexPlaylist `xml:"Playlist"`
}

// PlexServerInfo represents server information from Plex API
type PlexServerInfo struct {
	XMLName           xml.Name `xml:"MediaContainer"`
	FriendlyName      string   `xml:"friendlyName,attr"`
	MachineIdentifier string   `xml:"machineIdentifier,attr"`
	Version           string   `xml:"version,attr"`
	Platform          string   `xml:"platform,attr"`
	PlatformVersion   string   `xml:"platformVersion,attr"`
}

// MatchResult represents the result of matching a source track to Plex
type MatchResult struct {
	SourceTrack track.Track
	PlexTrack   *PlexTrack
	MatchType   MatchKind
	Confidence  float64
}

// NewClient creates a new Plex client using TLS settings from cfg (InsecureSkipVerify defaults true in config.Load).
func NewClient(cfg *config.Config) *Client {
	return NewClientWithTLSConfig(cfg, cfg.Plex.InsecureSkipVerify)
}

// NewClientWithTLSConfig creates a new Plex client. If skipTLSVerify is true, TLS certificate verification is disabled (default for many Plex HTTPS setups).
func NewClientWithTLSConfig(cfg *config.Config, skipTLSVerify bool) *Client {
	httpClient := &http.Client{Timeout: DefaultHTTPTimeout}

	var baseTransport http.RoundTripper = http.DefaultTransport
	if skipTLSVerify {
		if dt, ok := http.DefaultTransport.(*http.Transport); ok {
			tr := dt.Clone()
			tr.TLSClientConfig = &tls.Config{InsecureSkipVerify: true}
			baseTransport = tr
		} else {
			baseTransport = &http.Transport{
				TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
			}
		}
	}

	rps := cfg.Plex.MaxRequestsPerSecond
	var rt http.RoundTripper = baseTransport
	if rps > 0 {
		rt = newRateLimitedTransport(baseTransport, rps)
	}
	httpClient.Transport = &acceptIdentityTransport{base: rt}

	plexgoClient := plexgo.New(
		plexgo.WithSecurity(cfg.Plex.Token),
		plexgo.WithServerURL(cfg.Plex.URL),
		plexgo.WithClient(httpClient),
	)

	mc := cfg.Plex.MatchConcurrency
	if mc < 1 {
		mc = 1
	}
	if mc > 32 {
		mc = 32
	}

	mp := cfg.Plex.MatchConfidencePercent
	mpCopy := mp
	return &Client{
		baseURL:                cfg.Plex.URL,
		token:                  cfg.Plex.Token,
		sectionID:              cfg.Plex.LibrarySectionID,
		serverID:               cfg.Plex.ServerID,
		httpClient:             httpClient,
		debug:                  false,
		plexgoClient:           plexgoClient,
		matchConcurrency:       mc,
		dryRun:                 cfg.Plex.DryRun,
		skipFullLibrarySearch:  cfg.Plex.SkipFullLibrarySearch,
		exactMatchesOnly:       cfg.Plex.ExactMatchesOnly,
		matchConfidencePercent: &mpCopy,
	}
}

func (c *Client) minMatchScore() float64 {
	if c.matchConfidencePercent == nil {
		return float64(config.DefaultMatchConfidencePercent) / 100.0
	}
	p := *c.matchConfidencePercent
	if p < 0 {
		return float64(config.DefaultMatchConfidencePercent) / 100.0
	}
	if p > 100 {
		return 1.0
	}
	return float64(p) / 100.0
}

// SetDryRun toggles whether playlist mutations are applied (false = default).
func (c *Client) SetDryRun(dryRun bool) {
	c.dryRun = dryRun
}

// SetSkipFullLibrarySearch skips the expensive /all scan when true (indexed /search only).
func (c *Client) SetSkipFullLibrarySearch(skip bool) {
	c.skipFullLibrarySearch = skip
}

// SetExactMatchesOnly restricts track search to the raw title/artist strategy only (no normalizations, no /all).
func (c *Client) SetExactMatchesOnly(exact bool) {
	c.exactMatchesOnly = exact
}

// SetMatchConcurrency sets parallel track match workers (1–32).
func (c *Client) SetMatchConcurrency(n int) {
	if n < 1 {
		n = 1
	}
	if n > 32 {
		n = 32
	}
	c.matchConcurrency = n
}

// GetHTTPClient returns the HTTP client for external use
func (c *Client) GetHTTPClient() *http.Client {
	return c.httpClient
}

// GetBaseURL returns the base URL
func (c *Client) GetBaseURL() string {
	return c.baseURL
}

// GetToken returns the authentication token
func (c *Client) GetToken() string {
	return c.token
}

// GetServerInfo retrieves server information from the Plex API
func (c *Client) GetServerInfo(ctx context.Context) (*PlexServerInfo, error) {
	reqURL := fmt.Sprintf("%s/", c.baseURL)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create server info request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make server info request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("plex server info API returned status %d", resp.StatusCode)
	}

	var serverInfo PlexServerInfo
	if err := xml.NewDecoder(resp.Body).Decode(&serverInfo); err != nil {
		return nil, fmt.Errorf("failed to decode server info response: %w", err)
	}

	return &serverInfo, nil
}

// GetServerID retrieves the server ID (machine identifier) from the Plex API
func (c *Client) GetServerID(ctx context.Context) (string, error) {
	serverInfo, err := c.GetServerInfo(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get server info: %w", err)
	}

	if serverInfo.MachineIdentifier == "" {
		return "", fmt.Errorf("server info response does not contain machine identifier")
	}

	return serverInfo.MachineIdentifier, nil
}

// SetServerID updates the server ID in the client
func (c *Client) SetServerID(serverID string) {
	c.serverID = serverID
}

// SetDebug enables or disables debug mode
func (c *Client) SetDebug(debug bool) {
	c.debug = debug
}

// debugLog logs a message only if debug mode is enabled
func (c *Client) debugLog(format string, args ...interface{}) {
	if c.debug {
		slog.Debug(fmt.Sprintf(format, args...))
	}
}
