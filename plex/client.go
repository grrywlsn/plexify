package plex

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"log"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/garry/plexify/config"
	"github.com/garry/plexify/spotify"
)

// Constants for Plex API
const (
	// Plex API constants
	PlexMusicTrackType = "10"

	// Search confidence thresholds
	MinConfidenceScore = 0.7

	// HTTP timeouts
	DefaultHTTPTimeout = 30 * time.Second

	// Match types
	MatchTypeTitleArtist = "title_artist"
	MatchTypeNone        = "none"
	MatchTypeError       = "error"

	// HTTP status codes
	StatusOK        = http.StatusOK
	StatusCreated   = http.StatusCreated
	StatusNoContent = http.StatusNoContent

	// Search parameters
	SearchLimit = 100
)

// Client wraps the Plex API client
type Client struct {
	baseURL    string
	token      string
	sectionID  int
	serverID   string
	httpClient *http.Client
	debug      bool
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

// MatchResult represents the result of matching a Spotify song to Plex
type MatchResult struct {
	SpotifySong spotify.Song
	PlexTrack   *PlexTrack
	MatchType   string // "title_artist" or "none"
	Confidence  float64
}

// NewClient creates a new Plex client
func NewClient(cfg *config.Config) *Client {
	return &Client{
		baseURL:    cfg.Plex.URL,
		token:      cfg.Plex.Token,
		sectionID:  cfg.Plex.LibrarySectionID,
		serverID:   cfg.Plex.ServerID,
		httpClient: &http.Client{Timeout: DefaultHTTPTimeout},
		debug:      false,
	}
}

// NewClientWithTLSConfig creates a new Plex client with custom TLS configuration
func NewClientWithTLSConfig(cfg *config.Config, skipTLSVerify bool) *Client {
	httpClient := &http.Client{Timeout: DefaultHTTPTimeout}

	if skipTLSVerify {
		httpClient.Transport = &http.Transport{
			TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		}
	}

	return &Client{
		baseURL:    cfg.Plex.URL,
		token:      cfg.Plex.Token,
		sectionID:  cfg.Plex.LibrarySectionID,
		serverID:   cfg.Plex.ServerID,
		httpClient: httpClient,
		debug:      false,
	}
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

// SearchTrack searches for a track in Plex using title/artist matching
func (c *Client) SearchTrack(ctx context.Context, song spotify.Song) (*PlexTrack, string, error) {
	c.debugLog("🔍 SearchTrack: searching for '%s' by '%s'", song.Name, song.Artist)

	// Try different search strategies in order of preference
	searchStrategies := []struct {
		name string
		fn   func(context.Context, string, string) (*PlexTrack, error)
	}{
		{"exact title/artist", func(ctx context.Context, title, artist string) (*PlexTrack, error) {
			return c.trySearchVariations(ctx, title, artist)
		}},
		{"single quote variations", func(ctx context.Context, title, artist string) (*PlexTrack, error) {
			if strings.Contains(title, "'") || strings.Contains(artist, "'") {
				return c.searchByTitleWithSingleQuoteVariations(ctx, title, artist)
			}
			return nil, nil
		}},
		{"brackets removed", func(ctx context.Context, title, artist string) (*PlexTrack, error) {
			cleanTitle := c.removeBrackets(title)
			if cleanTitle != title {
				return c.trySearchVariations(ctx, cleanTitle, artist)
			}
			return nil, nil
		}},
		{"featuring removed", func(ctx context.Context, title, artist string) (*PlexTrack, error) {
			featuringTitle := c.removeFeaturing(title)
			if featuringTitle != title && featuringTitle != c.removeBrackets(title) {
				return c.trySearchVariations(ctx, featuringTitle, artist)
			}
			return nil, nil
		}},
		{"normalized title", func(ctx context.Context, title, artist string) (*PlexTrack, error) {
			normalizedTitle := c.normalizeTitle(title)
			if normalizedTitle != title && normalizedTitle != c.removeBrackets(title) && normalizedTitle != c.removeFeaturing(title) {
				return c.trySearchVariations(ctx, normalizedTitle, artist)
			}
			return nil, nil
		}},
		{"with removed", func(ctx context.Context, title, artist string) (*PlexTrack, error) {
			withTitle := c.removeWith(title)
			if withTitle != title && withTitle != c.removeBrackets(title) && withTitle != c.removeFeaturing(title) && withTitle != c.normalizeTitle(title) {
				return c.trySearchVariations(ctx, withTitle, artist)
			}
			return nil, nil
		}},
		{"suffixes removed", func(ctx context.Context, title, artist string) (*PlexTrack, error) {
			suffixTitle := c.RemoveCommonSuffixes(title)
			if suffixTitle != title && suffixTitle != c.removeBrackets(title) && suffixTitle != c.removeFeaturing(title) && suffixTitle != c.normalizeTitle(title) && suffixTitle != c.removeWith(title) {
				c.debugLog("🔍 SearchTrack: trying suffix-removed title '%s' for '%s' by '%s'", suffixTitle, title, artist)
				return c.trySearchVariations(ctx, suffixTitle, artist)
			}
			return nil, nil
		}},
		{"full library search", func(ctx context.Context, title, artist string) (*PlexTrack, error) {
			c.debugLog("🔍 SearchTrack: trying full library search for '%s' by '%s'", title, artist)
			return c.searchEntireLibrary(ctx, title, artist)
		}},
	}

	for _, strategy := range searchStrategies {
		if track, err := strategy.fn(ctx, song.Name, song.Artist); err == nil && track != nil {
			log.Printf("✅ SearchTrack: found match '%s' by '%s' using %s", track.Title, track.Artist, strategy.name)
			return track, MatchTypeTitleArtist, nil
		}
	}

	return nil, MatchTypeNone, nil
}

// trySearchVariations tries different search strategies for a given title and artist
func (c *Client) trySearchVariations(ctx context.Context, title, artist string) (*PlexTrack, error) {
	// Try combined search first (most efficient)
	if track, err := c.searchByCombinedQuery(ctx, title, artist); err == nil && track != nil {
		return track, nil
	}

	// Try title search
	if track, err := c.searchByTitle(ctx, title, artist); err == nil && track != nil {
		return track, nil
	}

	// Try artist search
	if track, err := c.searchByArtist(ctx, title, artist); err == nil && track != nil {
		return track, nil
	}

	return nil, nil
}

// searchByTitle searches for tracks by title in the music library
func (c *Client) searchByTitle(ctx context.Context, title, artist string) (*PlexTrack, error) {

	// Use the library search endpoint
	reqURL := fmt.Sprintf("%s/library/sections/%d/search", c.baseURL, c.sectionID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("query", title)
	params.Add("type", PlexMusicTrackType) // Type 10 = music tracks

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("plex search API returned status %d", resp.StatusCode)
	}

	var searchResp PlexResponse
	if err := xml.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Find best match among search results
	log.Printf("🔍 searchByTitle: searching for '%s' by '%s', found %d results", title, artist, len(searchResp.Tracks))
	if len(searchResp.Tracks) > 0 && c.debug {
		for i, track := range searchResp.Tracks {
			c.debugLog("  Result %d: '%s' by '%s' (ID: %s)", i+1, track.Title, track.Artist, track.ID)
		}
	}
	result := c.FindBestMatch(searchResp.Tracks, title, artist)
	if result != nil {
		log.Printf("✅ searchByTitle: found match '%s' by '%s'", result.Title, result.Artist)
	} else {
		c.debugLog("❌ searchByTitle: no match found")
	}
	return result, nil
}

// searchByArtist searches for tracks by artist in the music library
func (c *Client) searchByArtist(ctx context.Context, title, artist string) (*PlexTrack, error) {

	// Use the library search endpoint with artist query
	reqURL := fmt.Sprintf("%s/library/sections/%d/search", c.baseURL, c.sectionID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("query", artist)
	params.Add("type", PlexMusicTrackType) // Type 10 = music tracks

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create artist search request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make artist search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("plex artist search API returned status %d", resp.StatusCode)
	}

	var searchResp PlexResponse
	if err := xml.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode artist search response: %w", err)
	}

	// Find best match among search results
	result := c.FindBestMatch(searchResp.Tracks, title, artist)
	if result != nil {
		log.Printf("✅ searchByArtist: found match '%s' by '%s'", result.Title, result.Artist)
	}
	return result, nil
}

// searchByCombinedQuery searches using a combined title + artist query (most efficient)
func (c *Client) searchByCombinedQuery(ctx context.Context, title, artist string) (*PlexTrack, error) {
	// Try the most likely combination first
	query := fmt.Sprintf("%s %s", title, artist)

	reqURL := fmt.Sprintf("%s/library/sections/%d/search", c.baseURL, c.sectionID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("query", query)
	params.Add("type", PlexMusicTrackType) // Type 10 = music tracks

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create combined search request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make combined search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == StatusOK {
		var searchResp PlexResponse
		if err := xml.NewDecoder(resp.Body).Decode(&searchResp); err == nil {
			if track := c.FindBestMatch(searchResp.Tracks, title, artist); track != nil {
				log.Printf("✅ searchByCombinedQuery: found match '%s' by '%s'", track.Title, track.Artist)
				return track, nil
			}
		}
	}

	return nil, nil
}

// searchByTitleWithSingleQuoteVariations searches for tracks with single quotes by trying different variations
func (c *Client) searchByTitleWithSingleQuoteVariations(ctx context.Context, title, artist string) (*PlexTrack, error) {
	// If the title doesn't contain single quotes, use the regular search
	if !strings.Contains(title, "'") {
		return c.searchByTitle(ctx, title, artist)
	}

	// Try different variations of the title with single quotes
	variations := []string{
		title,                               // Original title
		strings.ReplaceAll(title, "'", ""),  // Remove all single quotes
		strings.ReplaceAll(title, "'", "`"), // Replace with backtick
		strings.ReplaceAll(title, "'", "′"), // Replace with prime symbol
		strings.ReplaceAll(title, "'", "'"), // Replace with different quote character
	}

	// Also try variations with common contractions expanded
	if strings.Contains(title, "n't") {
		variations = append(variations, strings.ReplaceAll(title, "n't", " not"))
	}
	if strings.Contains(title, "'t") {
		variations = append(variations, strings.ReplaceAll(title, "'t", " not"))
	}
	if strings.Contains(title, "'s") {
		variations = append(variations, strings.ReplaceAll(title, "'s", " is"))
		variations = append(variations, strings.ReplaceAll(title, "'s", "s")) // Just remove the apostrophe
	}
	if strings.Contains(title, "'re") {
		variations = append(variations, strings.ReplaceAll(title, "'re", " are"))
	}
	if strings.Contains(title, "'ll") {
		variations = append(variations, strings.ReplaceAll(title, "'ll", " will"))
	}
	if strings.Contains(title, "'ve") {
		variations = append(variations, strings.ReplaceAll(title, "'ve", " have"))
	}
	if strings.Contains(title, "'d") {
		variations = append(variations, strings.ReplaceAll(title, "'d", " would"))
		variations = append(variations, strings.ReplaceAll(title, "'d", " had"))
	}

	// Try each variation
	for _, variation := range variations {
		if variation == "" {
			continue
		}

		// Try combined search first
		track, err := c.searchByCombinedQuery(ctx, variation, artist)
		if err == nil && track != nil {
			return track, nil
		}

		// Try title search
		track, err = c.searchByTitle(ctx, variation, artist)
		if err == nil && track != nil {
			return track, nil
		}

		// Try artist search
		track, err = c.searchByArtist(ctx, variation, artist)
		if err == nil && track != nil {
			return track, nil
		}
	}

	return nil, nil
}

// searchEntireLibrary is a fallback method that searches through all tracks in the library
// This is used when the regular search methods fail to find tracks that should exist
func (c *Client) searchEntireLibrary(ctx context.Context, title, artist string) (*PlexTrack, error) {
	// Get all tracks from the library
	reqURL := fmt.Sprintf("%s/library/sections/%d/all", c.baseURL, c.sectionID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("type", PlexMusicTrackType) // Type 10 = music tracks

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create library request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make library request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("plex library API returned status %d", resp.StatusCode)
	}

	var libraryResp PlexResponse
	if err := xml.NewDecoder(resp.Body).Decode(&libraryResp); err != nil {
		return nil, fmt.Errorf("failed to decode library response: %w", err)
	}

	// Find best match among all tracks
	c.debugLog("🔍 searchEntireLibrary: searching for '%s' by '%s' in entire library (%d tracks)", title, artist, len(libraryResp.Tracks))
	result := c.FindBestMatch(libraryResp.Tracks, title, artist)
	if result != nil {
		log.Printf("✅ searchEntireLibrary: found match '%s' by '%s' for search '%s' by '%s'", result.Title, result.Artist, title, artist)
	} else {
		log.Printf("❌ searchEntireLibrary: no match found for search '%s' by '%s'", title, artist)
	}
	return result, nil
}

// FindBestMatch finds the best matching track from search results
func (c *Client) FindBestMatch(tracks []PlexTrack, title, artist string) *PlexTrack {
	if len(tracks) == 0 {
		return nil
	}

	titleLower := strings.ToLower(strings.TrimSpace(title))
	artistLower := strings.ToLower(strings.TrimSpace(artist))

	c.debugLog("🔍 FindBestMatch: searching for '%s' by '%s' among %d tracks", title, artist, len(tracks))

	// First, check for exact matches before applying any transformations
	for _, track := range tracks {
		trackTitle := strings.ToLower(strings.TrimSpace(track.Title))
		trackArtist := strings.ToLower(strings.TrimSpace(track.Artist))

		// Check for exact title and artist match
		if titleLower == trackTitle && artistLower == trackArtist {
			c.debugLog("✅ FindBestMatch: exact match found '%s' by '%s'", track.Title, track.Artist)
			return &track
		}
	}

	var bestMatch *PlexTrack
	var bestScore float64
	var bestArtistSimilarity float64

	for _, track := range tracks {
		trackTitle := strings.ToLower(strings.TrimSpace(track.Title))
		trackArtist := strings.ToLower(strings.TrimSpace(track.Artist))

		// Calculate similarity scores with original titles
		titleSimilarity := c.calculateStringSimilarity(titleLower, trackTitle)
		artistSimilarity := c.calculateStringSimilarity(artistLower, trackArtist)

		c.debugLog("🔍 FindBestMatch: '%s' by '%s' -> '%s' by '%s'", title, artist, track.Title, track.Artist)
		c.debugLog("   Original title similarity: %.3f ('%s' vs '%s')", titleSimilarity, titleLower, trackTitle)
		c.debugLog("   Original artist similarity: %.3f ('%s' vs '%s')", artistSimilarity, artistLower, trackArtist)

		// Also try with normalized punctuation for artist matching
		punctuationArtistLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(artist)))
		punctuationTrackArtistLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(track.Artist)))
		punctuationArtistSimilarity := c.calculateStringSimilarity(punctuationArtistLower, punctuationTrackArtistLower)

		// Use the better artist similarity
		if punctuationArtistSimilarity > artistSimilarity {
			c.debugLog("   Using normalized artist similarity: %.3f (was %.3f)", punctuationArtistSimilarity, artistSimilarity)
			artistSimilarity = punctuationArtistSimilarity
		}

		// Also try with cleaned titles (without brackets) for better matching
		cleanTitleLower := strings.ToLower(strings.TrimSpace(c.removeBrackets(title)))
		cleanTrackTitle := strings.ToLower(strings.TrimSpace(c.removeBrackets(track.Title)))

		// Calculate similarity with cleaned titles
		cleanTitleSimilarity := c.calculateStringSimilarity(cleanTitleLower, cleanTrackTitle)
		c.debugLog("   Clean title similarity: %.3f ('%s' vs '%s')", cleanTitleSimilarity, cleanTitleLower, cleanTrackTitle)

		// Also try with featuring removed for better matching
		featuringTitleLower := strings.ToLower(strings.TrimSpace(c.removeFeaturing(title)))
		featuringTrackTitleLower := strings.ToLower(strings.TrimSpace(c.removeFeaturing(track.Title)))

		// Calculate similarity with featuring removed
		featuringTitleSimilarity := c.calculateStringSimilarity(featuringTitleLower, featuringTrackTitleLower)
		c.debugLog("   Featuring-removed title similarity: %.3f ('%s' vs '%s')", featuringTitleSimilarity, featuringTitleLower, featuringTrackTitleLower)

		// Also try with normalized titles for better matching
		normalizedTitleLower := strings.ToLower(strings.TrimSpace(c.normalizeTitle(title)))
		normalizedTrackTitleLower := strings.ToLower(strings.TrimSpace(c.normalizeTitle(track.Title)))

		// Calculate similarity with normalized titles
		normalizedTitleSimilarity := c.calculateStringSimilarity(normalizedTitleLower, normalizedTrackTitleLower)
		c.debugLog("   Normalized title similarity: %.3f ('%s' vs '%s')", normalizedTitleSimilarity, normalizedTitleLower, normalizedTrackTitleLower)

		// Also try with "with" removed for better matching
		withTitleLower := strings.ToLower(strings.TrimSpace(c.removeWith(title)))
		withTrackTitleLower := strings.ToLower(strings.TrimSpace(c.removeWith(track.Title)))

		// Calculate similarity with "with" removed
		withTitleSimilarity := c.calculateStringSimilarity(withTitleLower, withTrackTitleLower)
		c.debugLog("   'With'-removed title similarity: %.3f ('%s' vs '%s')", withTitleSimilarity, withTitleLower, withTrackTitleLower)

		// Also try with common suffixes removed for better matching
		suffixTitleLower := strings.ToLower(strings.TrimSpace(c.RemoveCommonSuffixes(title)))
		suffixTrackTitleLower := strings.ToLower(strings.TrimSpace(c.RemoveCommonSuffixes(track.Title)))

		// Calculate similarity with common suffixes removed
		suffixTitleSimilarity := c.calculateStringSimilarity(suffixTitleLower, suffixTrackTitleLower)
		c.debugLog("   Suffix-removed title similarity: %.3f ('%s' vs '%s')", suffixTitleSimilarity, suffixTitleLower, suffixTrackTitleLower)

		// Also try with normalized punctuation for better matching
		punctuationTitleLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(title)))
		punctuationTrackTitleLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(track.Title)))

		// Calculate similarity with normalized punctuation
		punctuationTitleSimilarity := c.calculateStringSimilarity(punctuationTitleLower, punctuationTrackTitleLower)
		c.debugLog("   Punctuation-normalized title similarity: %.3f ('%s' vs '%s')", punctuationTitleSimilarity, punctuationTitleLower, punctuationTrackTitleLower)

		// Use the best of the seven title similarities
		if cleanTitleSimilarity > titleSimilarity {
			c.debugLog("   Using clean title similarity: %.3f (was %.3f)", cleanTitleSimilarity, titleSimilarity)
			titleSimilarity = cleanTitleSimilarity
		}
		if featuringTitleSimilarity > titleSimilarity {
			c.debugLog("   Using featuring-removed title similarity: %.3f (was %.3f)", featuringTitleSimilarity, titleSimilarity)
			titleSimilarity = featuringTitleSimilarity
		}
		if normalizedTitleSimilarity > titleSimilarity {
			c.debugLog("   Using normalized title similarity: %.3f (was %.3f)", normalizedTitleSimilarity, titleSimilarity)
			titleSimilarity = normalizedTitleSimilarity
		}
		if withTitleSimilarity > titleSimilarity {
			c.debugLog("   Using 'with'-removed title similarity: %.3f (was %.3f)", withTitleSimilarity, titleSimilarity)
			titleSimilarity = withTitleSimilarity
		}
		if suffixTitleSimilarity > titleSimilarity {
			c.debugLog("   Using suffix-removed title similarity: %.3f (was %.3f)", suffixTitleSimilarity, titleSimilarity)
			titleSimilarity = suffixTitleSimilarity
		}
		if punctuationTitleSimilarity > titleSimilarity {
			c.debugLog("   Using punctuation-normalized title similarity: %.3f (was %.3f)", punctuationTitleSimilarity, titleSimilarity)
			titleSimilarity = punctuationTitleSimilarity
		}

		// Combined score (title is more important than artist)
		score := (titleSimilarity * 0.7) + (artistSimilarity * 0.3)

		c.debugLog("   Final title similarity: %.3f", titleSimilarity)
		c.debugLog("   Final artist similarity: %.3f", artistSimilarity)
		c.debugLog("   Combined score: %.3f (%.3f * 0.7 + %.3f * 0.3)", score, titleSimilarity, artistSimilarity)

		// Additional check: if title similarity is very high (>90%), require reasonable artist similarity
		// Special case: be more lenient with "Various Artists" for compilation albums
		if titleSimilarity > 0.9 && artistSimilarity < 0.3 {
			// Check if this is a "Various Artists" compilation album case
			if strings.ToLower(strings.TrimSpace(track.Artist)) == "various artists" {
				c.debugLog("🎵 FindBestMatch: allowing 'Various Artists' compilation match '%s' by '%s' (title: %.3f > 0.9, artist: %.3f < 0.3 but is Various Artists)",
					track.Title, track.Artist, titleSimilarity, artistSimilarity)
				// Don't skip this match - it's a valid compilation album case
			} else {
				// Skip this match - title is very similar but artist is too different
				c.debugLog("🚫 FindBestMatch: rejecting '%s' by '%s' (title: %.3f > 0.9, artist: %.3f < 0.3)",
					track.Title, track.Artist, titleSimilarity, artistSimilarity)
				continue
			}
		}

		// Additional check: if title similarity is high (>70%), require minimum artist similarity
		// Special case: be more lenient with "Various Artists" for compilation albums
		if titleSimilarity > 0.7 && artistSimilarity < 0.2 {
			// Check if this is a "Various Artists" compilation album case
			if strings.ToLower(strings.TrimSpace(track.Artist)) == "various artists" {
				c.debugLog("🎵 FindBestMatch: allowing 'Various Artists' compilation match '%s' by '%s' (title: %.3f > 0.7, artist: %.3f < 0.2 but is Various Artists)",
					track.Title, track.Artist, titleSimilarity, artistSimilarity)
				// Don't skip this match - it's a valid compilation album case
			} else {
				// Skip this match - title is similar but artist is too different
				c.debugLog("🚫 FindBestMatch: rejecting '%s' by '%s' (title: %.3f > 0.7, artist: %.3f < 0.2)",
					track.Title, track.Artist, titleSimilarity, artistSimilarity)
				continue
			}
		}

		// Update best match if this score is higher, or if scores are equal, prefer better artist match
		if score > bestScore {
			oldScore := bestScore
			bestScore = score
			// Create a copy of the track to avoid pointer aliasing
			trackCopy := track
			bestMatch = &trackCopy
			bestArtistSimilarity = artistSimilarity
			c.debugLog("📈 FindBestMatch: new best match '%s' by '%s' (score: %.3f > %.3f, title: %.3f, artist: %.3f)",
				track.Title, track.Artist, score, oldScore, titleSimilarity, artistSimilarity)
		} else if score == bestScore && artistSimilarity > bestArtistSimilarity {
			bestScore = score
			// Create a copy of the track to avoid pointer aliasing
			trackCopy := track
			bestMatch = &trackCopy
			bestArtistSimilarity = artistSimilarity
			c.debugLog("🎯 FindBestMatch: tie-breaker! '%s' by '%s' wins (same score: %.3f, better artist: %.3f > %.3f)",
				track.Title, track.Artist, score, artistSimilarity, bestArtistSimilarity)
		} else {
			c.debugLog("⏭️  FindBestMatch: skipping '%s' by '%s' (score: %.3f, current best: %.3f)",
				track.Title, track.Artist, score, bestScore)
		}

		// Perfect match - return immediately
		if titleSimilarity == 1.0 && artistSimilarity == 1.0 {
			c.debugLog("🎯 FindBestMatch: perfect match found '%s' by '%s'", track.Title, track.Artist)
			return &track
		}
	}

	// Only return a match if the score is above a threshold
	if bestScore >= MinConfidenceScore {
		log.Printf("✅ FindBestMatch: FINAL RESULT - returning match '%s' by '%s' (score: %.3f >= %.3f) for search '%s' by '%s'",
			bestMatch.Title, bestMatch.Artist, bestScore, MinConfidenceScore, title, artist)
		return bestMatch
	}

	c.debugLog("❌ FindBestMatch: FINAL RESULT - no match found (best score: %.3f < %.3f) for search '%s' by '%s'", bestScore, MinConfidenceScore, title, artist)
	return nil
}

// FindBestMatchWithNormalizedPunctuation finds the best matching track using normalized punctuation
func (c *Client) FindBestMatchWithNormalizedPunctuation(tracks []PlexTrack, title, artist string) *PlexTrack {
	if len(tracks) == 0 {
		return nil
	}

	// Normalize punctuation for both search terms and track data
	normalizedTitle := c.normalizePunctuation(title)
	normalizedArtist := c.normalizePunctuation(artist)

	titleLower := strings.ToLower(strings.TrimSpace(normalizedTitle))
	artistLower := strings.ToLower(strings.TrimSpace(normalizedArtist))

	log.Printf("🔍 FindBestMatchWithNormalizedPunctuation: searching for '%s' by '%s' among %d tracks", normalizedTitle, normalizedArtist, len(tracks))

	// First, check for exact matches with normalized punctuation
	for _, track := range tracks {
		normalizedTrackTitle := c.normalizePunctuation(track.Title)
		normalizedTrackArtist := c.normalizePunctuation(track.Artist)

		trackTitle := strings.ToLower(strings.TrimSpace(normalizedTrackTitle))
		trackArtist := strings.ToLower(strings.TrimSpace(normalizedTrackArtist))

		// Check for exact title and artist match
		if titleLower == trackTitle && artistLower == trackArtist {
			log.Printf("✅ FindBestMatchWithNormalizedPunctuation: exact match found '%s' by '%s'", track.Title, track.Artist)
			return &track
		}
	}

	var bestMatch *PlexTrack
	var bestScore float64
	var bestArtistSimilarity float64

	for _, track := range tracks {
		normalizedTrackTitle := c.normalizePunctuation(track.Title)
		normalizedTrackArtist := c.normalizePunctuation(track.Artist)

		trackTitle := strings.ToLower(strings.TrimSpace(normalizedTrackTitle))
		trackArtist := strings.ToLower(strings.TrimSpace(normalizedTrackArtist))

		// Calculate similarity scores with normalized punctuation
		titleSimilarity := c.calculateStringSimilarity(titleLower, trackTitle)
		artistSimilarity := c.calculateStringSimilarity(artistLower, trackArtist)

		// Combined score (title is more important than artist)
		score := (titleSimilarity * 0.7) + (artistSimilarity * 0.3)

		// Additional check: if title similarity is very high (>90%), require reasonable artist similarity
		if titleSimilarity > 0.9 && artistSimilarity < 0.3 {
			// Skip this match - title is very similar but artist is too different
			log.Printf("🚫 FindBestMatchWithNormalizedPunctuation: rejecting '%s' by '%s' (title: %.3f > 0.9, artist: %.3f < 0.3)",
				track.Title, track.Artist, titleSimilarity, artistSimilarity)
			continue
		}

		// Additional check: if title similarity is high (>70%), require minimum artist similarity
		if titleSimilarity > 0.7 && artistSimilarity < 0.2 {
			// Skip this match - title is similar but artist is too different
			log.Printf("🚫 FindBestMatchWithNormalizedPunctuation: rejecting '%s' by '%s' (title: %.3f > 0.7, artist: %.3f < 0.2)",
				track.Title, track.Artist, titleSimilarity, artistSimilarity)
			continue
		}

		// Update best match if this score is higher, or if scores are equal, prefer better artist match
		if score > bestScore || (score == bestScore && artistSimilarity > bestArtistSimilarity) {
			bestScore = score
			// Create a copy of the track to avoid pointer aliasing
			trackCopy := track
			bestMatch = &trackCopy
			bestArtistSimilarity = artistSimilarity
			log.Printf("📈 FindBestMatchWithNormalizedPunctuation: new best match '%s' by '%s' (score: %.3f, title: %.3f, artist: %.3f)",
				track.Title, track.Artist, score, titleSimilarity, artistSimilarity)
		}

		// Perfect match - return immediately
		if titleSimilarity == 1.0 && artistSimilarity == 1.0 {
			log.Printf("🎯 FindBestMatchWithNormalizedPunctuation: perfect match found '%s' by '%s'", track.Title, track.Artist)
			return &track
		}
	}

	// Only return a match if the score is above a threshold
	if bestScore >= MinConfidenceScore {
		log.Printf("✅ FindBestMatchWithNormalizedPunctuation: returning match '%s' by '%s' (score: %.3f >= %.3f)",
			bestMatch.Title, bestMatch.Artist, bestScore, MinConfidenceScore)
		return bestMatch
	}

	log.Printf("❌ FindBestMatchWithNormalizedPunctuation: no match found (best score: %.3f < %.3f)", bestScore, MinConfidenceScore)
	return nil
}

// CreatePlaylist creates a new playlist in Plex

// escapeDescription decodes HTML entities in playlist descriptions
func (c *Client) escapeDescription(description string) string {
	// Decode HTML entities to get the actual characters
	// This handles cases like &#x2F; -> /
	return html.UnescapeString(description)
}

// addSyncAttribution adds a sync attribution line to the description
func (c *Client) addSyncAttribution(description, spotifyPlaylistID string) string {
	if spotifyPlaylistID == "" {
		return description
	}

	// Create the attribution line
	syncLine := fmt.Sprintf("synced from Spotify: https://open.spotify.com/playlist/%s", spotifyPlaylistID)

	// If there's existing description, add newlines before the attribution
	if description != "" {
		return description + "\n\n" + syncLine
	}

	// If no existing description, just return the attribution
	return syncLine
}

// CreatePlaylist creates a new playlist with an initial track (required for sync operations)
func (c *Client) CreatePlaylist(ctx context.Context, title, description, trackURI, spotifyPlaylistID string) (*PlexPlaylist, error) {
	// Use the correct Plex API endpoint for playlist creation
	reqURL := fmt.Sprintf("%s/playlists", c.baseURL)

	// Add parameters to URL query string (matching Plex Web behavior)
	params := url.Values{}
	params.Add("type", "audio")
	params.Add("title", title)
	params.Add("smart", "0")
	params.Add("uri", trackURI)

	// Always add description with sync attribution if we have a spotifyPlaylistID
	// or if there's an original description
	if spotifyPlaylistID != "" || description != "" {
		descriptionWithAttribution := c.addSyncAttribution(description, spotifyPlaylistID)
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

	log.Printf("Successfully created playlist with track: %s (ID: %s)", createdPlaylist.Title, createdPlaylist.ID)

	return createdPlaylist, nil
}

// GetPlaylists retrieves all playlists from the Plex server
func (c *Client) GetPlaylists(ctx context.Context) ([]PlexPlaylist, error) {
	reqURL := fmt.Sprintf("%s/playlists", c.baseURL)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create playlists request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make playlists request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("plex playlists API returned status %d", resp.StatusCode)
	}

	var playlistResp PlexResponse
	if err := xml.NewDecoder(resp.Body).Decode(&playlistResp); err != nil {
		return nil, fmt.Errorf("failed to decode playlists response: %w", err)
	}

	return playlistResp.Playlists, nil
}

// UpdatePlaylistMetadata updates the metadata of an existing playlist
func (c *Client) UpdatePlaylistMetadata(ctx context.Context, playlistID, title, description, spotifyPlaylistID string) error {
	// Use the Plex API endpoint for updating playlist metadata
	reqURL := fmt.Sprintf("%s/playlists/%s", c.baseURL, playlistID)

	// Add parameters to URL query string
	params := url.Values{}
	params.Add("type", "audio")
	if title != "" {
		params.Add("title", title)
	}

	// Always add description with sync attribution if we have a spotifyPlaylistID
	// or if there's an original description
	if spotifyPlaylistID != "" || description != "" {
		descriptionWithAttribution := c.addSyncAttribution(description, spotifyPlaylistID)
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

	log.Printf("Successfully updated playlist metadata: %s (ID: %s)", title, playlistID)
	return nil
}

// ClearPlaylist removes all tracks from an existing playlist
func (c *Client) ClearPlaylist(ctx context.Context, playlistID string) error {
	log.Printf("Clearing playlist %s", playlistID)

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

	log.Printf("Successfully cleared playlist: %s", playlistID)
	return nil
}

// AddTracksToPlaylist adds tracks to an existing playlist
func (c *Client) AddTracksToPlaylist(ctx context.Context, playlistID string, trackIDs []string) error {
	if len(trackIDs) == 0 {
		return nil
	}

	log.Printf("Adding %d tracks to playlist %s", len(trackIDs), playlistID)

	// Add tracks one by one using the correct Plex API format
	successCount := 0
	for _, trackID := range trackIDs {

		// Build request URL - use the correct Plex API endpoint
		reqURL := fmt.Sprintf("%s/playlists/%s/items", c.baseURL, playlistID)
		params := url.Values{}
		params.Add("X-Plex-Token", c.token)
		params.Add("uri", fmt.Sprintf("server://%s/com.plexapp.plugins.library/library/metadata/%s", c.serverID, trackID))

		req, err := http.NewRequestWithContext(ctx, "PUT", reqURL+"?"+params.Encode(), nil)
		if err != nil {
			log.Printf("Failed to create request for track %s: %v", trackID, err)
			continue
		}

		req.Header.Set("Accept", "application/xml")
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

		// Make request
		resp, err := c.httpClient.Do(req)
		if err != nil {
			log.Printf("Failed to make request for track %s: %v", trackID, err)
			continue
		}
		defer resp.Body.Close()

		if resp.StatusCode != StatusOK {
			body, _ := io.ReadAll(resp.Body)
			c.debugLog("Plex API returned status %d for track %s: %s", resp.StatusCode, trackID, string(body))
			continue
		}

		// Read response to check if track was actually added
		body, _ := io.ReadAll(resp.Body)
		if len(body) > 0 {
			// Check if the response indicates the track was added
			if strings.Contains(string(body), "leafCountAdded") {
				c.debugLog("Track %s: API response received", trackID)
			}

			// Check if tracks were actually added
			if strings.Contains(string(body), `leafCountAdded="0"`) {
				c.debugLog("⚠️  Warning: Track %s was not added (leafCountAdded=0)", trackID)
				// Don't count as success if track wasn't actually added
				continue
			} else if strings.Contains(string(body), `leafCountAdded="1"`) {
				c.debugLog("✅ Track %s was successfully added", trackID)
			}
		}

		successCount++
	}

	if successCount == 0 {
		return fmt.Errorf("failed to add any tracks to playlist - this may be due to server configuration restrictions or playlist permissions. Please check if playlist modifications are enabled on your Plex server and ensure your token has write permissions")
	}

	c.debugLog("Successfully processed %d/%d tracks for playlist %s", successCount, len(trackIDs), playlistID)
	return nil
}

// MatchSpotifyPlaylist matches Spotify songs to Plex tracks and adds them to an existing playlist if mapped
func (c *Client) MatchSpotifyPlaylist(ctx context.Context, songs []spotify.Song, playlistName, description string, spotifyPlaylistID string) ([]MatchResult, *PlexPlaylist, error) {
	log.Printf("Starting sequential matching of %d Spotify songs to Plex tracks", len(songs))

	// Process songs sequentially
	results := make([]MatchResult, len(songs))
	var matchedTrackIDs []string
	var titleMatches, noMatches int

	for i, song := range songs {
		log.Printf("Processing song %d/%d: %s - %s", i+1, len(songs), song.Artist, song.Name)

		track, matchType, err := c.SearchTrack(ctx, song)

		if err != nil {
			log.Printf("Error searching for track %s - %s: %v", song.Artist, song.Name, err)
			results[i] = MatchResult{
				SpotifySong: song,
				PlexTrack:   nil,
				MatchType:   MatchTypeError,
				Confidence:  0.0,
			}
			continue
		}

		results[i] = MatchResult{
			SpotifySong: song,
			PlexTrack:   track,
			MatchType:   matchType,
			Confidence:  c.calculateConfidence(song, track, matchType),
		}

		if track != nil {
			matchedTrackIDs = append(matchedTrackIDs, track.ID)
			switch matchType {
			case MatchTypeTitleArtist:
				titleMatches++
			}
		} else {
			noMatches++
		}
	}

	// Process matched tracks and create or update playlist
	var playlist *PlexPlaylist
	if len(matchedTrackIDs) > 0 {
		log.Printf("Found %d matched tracks", len(matchedTrackIDs))

		// First, check if a playlist with this name already exists
		log.Printf("Checking for existing playlist: %s", playlistName)
		existingPlaylists, err := c.GetPlaylists(ctx)
		if err != nil {
			log.Printf("❌ Failed to get existing playlists: %v", err)
			return results, nil, err
		}

		// Look for existing playlist with the same name
		var existingPlaylist *PlexPlaylist
		for _, p := range existingPlaylists {
			if p.Title == playlistName {
				existingPlaylist = &p
				break
			}
		}

		if existingPlaylist != nil {
			// Use existing playlist
			playlist = existingPlaylist
			log.Printf("✅ Found existing playlist: %s (ID: %s, Current tracks: %d)", existingPlaylist.Title, existingPlaylist.ID, existingPlaylist.TrackCount)
			log.Printf("🔄 Syncing playlist to match Spotify source of truth...")

			// Update playlist metadata (title and description) to match Spotify
			if err := c.UpdatePlaylistMetadata(ctx, existingPlaylist.ID, playlistName, description, spotifyPlaylistID); err != nil {
				log.Printf("⚠️  Warning: Failed to update playlist metadata: %v", err)
			} else {
				log.Printf("✅ Successfully updated playlist metadata to match Spotify")
			}
		} else {
			// Create new playlist
			log.Printf("Creating new playlist: %s", playlistName)
			playlist, err = c.CreatePlaylist(ctx, playlistName, description, "", spotifyPlaylistID)
			if err != nil {
				log.Printf("❌ Failed to create playlist: %v", err)
				return results, nil, err
			}
			log.Printf("✅ Created new playlist: %s (ID: %s)", playlist.Title, playlist.ID)
		}

		// Clear existing tracks and add new ones
		log.Printf("Clearing playlist and adding %d tracks", len(matchedTrackIDs))
		if err := c.ClearPlaylist(ctx, playlist.ID); err != nil {
			log.Printf("❌ Failed to clear playlist: %v", err)
			return results, playlist, err
		}

		if err := c.AddTracksToPlaylist(ctx, playlist.ID, matchedTrackIDs); err != nil {
			log.Printf("❌ Failed to add tracks to playlist: %v", err)
			return results, playlist, err
		}

		log.Printf("✅ Successfully added %d tracks to playlist", len(matchedTrackIDs))
	} else {
		log.Printf("No tracks matched, skipping playlist creation")
	}

	return results, playlist, nil
}

// calculateConfidence calculates a confidence score for the match
func (c *Client) calculateConfidence(song spotify.Song, track *PlexTrack, matchType string) float64 {
	if track == nil {
		return 0.0
	}

	switch matchType {
	case MatchTypeTitleArtist:
		// Calculate confidence based on title and artist similarity
		titleSimilarity := c.calculateStringSimilarity(strings.ToLower(song.Name), strings.ToLower(track.Title))
		artistSimilarity := c.calculateStringSimilarity(strings.ToLower(song.Artist), strings.ToLower(track.Artist))

		// Also try with cleaned titles (without brackets) for better matching
		cleanTitleSimilarity := c.calculateStringSimilarity(
			strings.ToLower(c.removeBrackets(song.Name)),
			strings.ToLower(c.removeBrackets(track.Title)),
		)

		// Also try with featuring removed for better matching
		featuringTitleSimilarity := c.calculateStringSimilarity(
			strings.ToLower(c.removeFeaturing(song.Name)),
			strings.ToLower(c.removeFeaturing(track.Title)),
		)

		// Also try with normalized titles for better matching
		normalizedTitleSimilarity := c.calculateStringSimilarity(
			strings.ToLower(c.normalizeTitle(song.Name)),
			strings.ToLower(c.normalizeTitle(track.Title)),
		)

		// Also try with "with" removed for better matching
		withTitleSimilarity := c.calculateStringSimilarity(
			strings.ToLower(c.removeWith(song.Name)),
			strings.ToLower(c.removeWith(track.Title)),
		)

		// Also try with common suffixes removed for better matching
		suffixTitleSimilarity := c.calculateStringSimilarity(
			strings.ToLower(c.RemoveCommonSuffixes(song.Name)),
			strings.ToLower(c.RemoveCommonSuffixes(track.Title)),
		)

		// Use the best of the six title similarities
		if cleanTitleSimilarity > titleSimilarity {
			titleSimilarity = cleanTitleSimilarity
		}
		if featuringTitleSimilarity > titleSimilarity {
			titleSimilarity = featuringTitleSimilarity
		}
		if normalizedTitleSimilarity > titleSimilarity {
			titleSimilarity = normalizedTitleSimilarity
		}
		if withTitleSimilarity > titleSimilarity {
			titleSimilarity = withTitleSimilarity
		}
		if suffixTitleSimilarity > titleSimilarity {
			titleSimilarity = suffixTitleSimilarity
		}

		return (titleSimilarity * 0.7) + (artistSimilarity * 0.3)
	default:
		return 0.0
	}
}

// calculateStringSimilarity calculates similarity between two strings
func (c *Client) calculateStringSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	// Handle empty strings
	if s1 == "" || s2 == "" {
		return 0.0
	}

	// Check for substring matches (one string contains the other)
	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		// Calculate how much of the longer string is covered
		longer := s1
		shorter := s2
		if len(s2) > len(s1) {
			longer = s2
			shorter = s1
		}
		return float64(len(shorter)) / float64(len(longer))
	}

	// Check for word-level matches
	words1 := strings.Fields(s1)
	words2 := strings.Fields(s2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	// Count matching words
	matchingWords := 0
	for _, word1 := range words1 {
		for _, word2 := range words2 {
			if word1 == word2 {
				matchingWords++
				break
			}
		}
	}

	// Calculate word similarity
	wordSimilarity := float64(matchingWords) / float64(max(len(words1), len(words2)))

	// Combine with length similarity
	lengthSimilarity := 1.0 - float64(abs(len(s1)-len(s2)))/float64(max(len(s1), len(s2)))

	// Return weighted average
	return (wordSimilarity * 0.7) + (lengthSimilarity * 0.3)
}

// removeBrackets removes text in brackets from a string
func (c *Client) removeBrackets(s string) string {
	// Remove content in parentheses, square brackets, and curly brackets
	// This handles various formats like (feat. Artist), [feat. Artist], {feat. Artist}

	// Remove parentheses content
	s = regexp.MustCompile(`\([^)]*\)`).ReplaceAllString(s, "")

	// Remove square brackets content
	s = regexp.MustCompile(`\[[^\]]*\]`).ReplaceAllString(s, "")

	// Remove curly brackets content
	s = regexp.MustCompile(`\{[^}]*\}`).ReplaceAllString(s, "")

	// Clean up extra whitespace and normalize multiple spaces to single spaces
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")

	return s
}

// removeFeaturing removes "featuring" and any text after it from a string
func (c *Client) removeFeaturing(s string) string {
	// Handle various "featuring" formats (case insensitive)
	lowerS := strings.ToLower(s)

	// Check for "featuring" patterns
	patterns := []string{
		" featuring ",
		" feat. ",
		" feat ",
		" ft. ",
		" ft ",
	}

	for _, pattern := range patterns {
		lastIndex := strings.LastIndex(lowerS, pattern)
		if lastIndex != -1 {
			// Return the original string up to the pattern (preserving original case)
			return strings.TrimSpace(s[:lastIndex])
		}
	}

	return s
}

// removeWith removes "with" and any text after it from a string
func (c *Client) removeWith(s string) string {
	// Handle "with" format (case insensitive)
	lowerS := strings.ToLower(s)

	// First check for "with" at the beginning (but only if followed by more text)
	if strings.HasPrefix(lowerS, "with ") && len(lowerS) > 4 {
		result := strings.TrimSpace(s[5:]) // Remove "with " from beginning
		// Clean up trailing dashes and spaces
		result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
		return result
	}

	// Use regex to match "with" as a whole word, not as part of another word
	// This prevents matching "without", "within", etc.
	re := regexp.MustCompile(`(?i)\bwith\b`)
	matches := re.FindAllStringIndex(lowerS, -1)

	if len(matches) > 0 {
		// Use the last match
		lastMatch := matches[len(matches)-1]

		// Only remove "with" if it's followed by additional text (not at the very end)
		if lastMatch[1] < len(lowerS) {
			// Return the original string up to the match (preserving original case)
			result := strings.TrimSpace(s[:lastMatch[0]])
			// Clean up trailing dashes and spaces
			result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
			return result
		}
	}

	return s
}

// RemoveCommonSuffixes removes common suffixes like "bonus track", "remix", "extended", etc. from track titles.
func (c *Client) RemoveCommonSuffixes(s string) string {
	// Handle common suffixes (case insensitive)
	lowerS := strings.ToLower(s)

	// Common suffixes to remove
	suffixes := []string{
		" - bonus track",
		" - remix",
		" - extended",
		" - radio edit",
		" - single edit",
		" - edit",
		" - version",
		" - live",
		" - acoustic",
		" - instrumental",
		" - demo",
		" - original mix",
		" - club mix",
		" - clean",
		" - explicit",
		" - bonus",
		" - track",
		" - remastered",
		// Soundtrack suffixes
		" - from the motion picture",
		" - from the film",
		" - from the movie",
		" - from the soundtrack",
		" - soundtrack version",
		" - film version",
		" - movie version",
		" (bonus track)",
		" (remix)",
		" (extended)",
		" (radio edit)",
		" (single edit)",
		" (edit)",
		" (version)",
		" (live)",
		" (acoustic)",
		" (instrumental)",
		" (demo)",
		" (original mix)",
		" (club mix)",
		" (clean)",
		" (explicit)",
		" (bonus)",
		" (track)",
		" (remastered)",
		// Soundtrack suffixes in parentheses (handled separately below)
		" (from the soundtrack)",
		" (soundtrack version)",
		" (film version)",
		" (movie version)",
	}

	for _, suffix := range suffixes {
		if strings.HasSuffix(lowerS, strings.ToLower(suffix)) {
			// Return the original string without the suffix (preserving original case)
			result := strings.TrimSpace(s[:len(s)-len(suffix)])
			// Clean up trailing dashes and spaces
			result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
			return result
		}
	}

	// Handle special cases with quotes that need regex matching
	// These patterns can have varying content inside quotes
	soundtrackPatterns := []string{
		" - from the motion picture",
		" - from the film",
		" - from the movie",
		" - love theme from",
		"(from the motion picture",
		"(from the film",
		"(from the movie",
		"(love theme from",
	}

	for _, pattern := range soundtrackPatterns {
		lowerPattern := strings.ToLower(pattern)
		if strings.Contains(lowerS, lowerPattern) {
			// Find the position of the pattern
			patternIndex := strings.Index(lowerS, lowerPattern)
			if patternIndex > 0 {
				// Return the original string up to the pattern (preserving original case)
				result := strings.TrimSpace(s[:patternIndex])
				// Clean up trailing dashes and spaces
				result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
				return result
			}
		}
	}

	return s
}

// normalizeTitle normalizes track titles by handling dashes and case differences
func (c *Client) normalizeTitle(s string) string {
	// Convert to lowercase for case-insensitive comparison
	s = strings.ToLower(s)

	// Replace dashes with parentheses for better matching
	// "Mood Ring (By Demand) - Pride Remix" -> "Mood Ring (By Demand) (Pride Remix)"
	// Handle multiple dashes by replacing each one with a separate set of parentheses
	parts := strings.Split(s, " - ")
	if len(parts) > 1 {
		// Keep the first part as is, wrap each subsequent part in parentheses
		result := parts[0]
		for i := 1; i < len(parts); i++ {
			result += " (" + strings.TrimSpace(parts[i]) + ")"
		}
		s = result
	}

	// Clean up extra whitespace
	s = strings.TrimSpace(s)
	s = regexp.MustCompile(`\s+`).ReplaceAllString(s, " ")

	return s
}

// normalizePunctuation normalizes various punctuation marks to standard forms
func (c *Client) normalizePunctuation(s string) string {
	// Normalize various types of dashes to standard hyphens
	s = strings.ReplaceAll(s, "\u2010", "-") // En dash to hyphen
	s = strings.ReplaceAll(s, "\u2014", "-") // Em dash to hyphen
	s = strings.ReplaceAll(s, "\u2015", "-") // Horizontal bar to hyphen

	// Normalize multiplication symbol to 'x' for artist names like "Chloe × Halle"
	s = strings.ReplaceAll(s, "\u00D7", "x") // Multiplication symbol to 'x'

	// Normalize various types of apostrophes to standard apostrophes
	s = strings.ReplaceAll(s, "\u2019", "'") // Right single quotation mark to apostrophe
	s = strings.ReplaceAll(s, "\u2018", "'") // Left single quotation mark to apostrophe
	s = strings.ReplaceAll(s, "\u0060", "'") // Grave accent to apostrophe
	s = strings.ReplaceAll(s, "\u2032", "'") // Prime symbol to apostrophe

	// Normalize various types of quotes to standard quotes
	s = strings.ReplaceAll(s, "\u201C", "\"") // Left double quotation mark to straight quote
	s = strings.ReplaceAll(s, "\u201D", "\"") // Right double quotation mark to straight quote
	s = strings.ReplaceAll(s, "\u2018", "'")  // Left single quotation mark to straight quote
	s = strings.ReplaceAll(s, "\u2019", "'")  // Right single quotation mark to straight quote

	return s
}

// debugLog logs a message only if debug mode is enabled
func (c *Client) debugLog(format string, args ...interface{}) {
	if c.debug {
		log.Printf(format, args...)
	}
}

// Helper functions
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func abs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}
