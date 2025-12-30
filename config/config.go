package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration values
type Config struct {
	Spotify SpotifyConfig
	Plex    PlexConfig
}

// SpotifyConfig holds Spotify API configuration
type SpotifyConfig struct {
	ClientID            string
	ClientSecret        string
	RedirectURI         string
	Username            string   // Spotify username to get all public playlists
	PlaylistIDs         []string // Spotify playlist IDs from comma-separated list
	ExcludedPlaylistIDs []string // Playlist IDs to exclude from processing
}

// PlexConfig holds Plex server configuration
type PlexConfig struct {
	URL              string
	Token            string
	LibrarySectionID int
	ServerID         string
}

// Load loads configuration following the specified order:
// 1. Start with empty values (except SPOTIFY_REDIRECT_URI which defaults to http://localhost:8080/callback)
// 2. Load from OS environment variables (only if they exist)
// 3. Load from .env file (only if it exists and values exist)
// 4. Apply CLI flag overrides (only if they exist)
func Load() (*Config, error) {
	config := &Config{}

	// Step 1: Initialize with default values
	config.initializeDefaults()

	// Step 2: Load from OS environment variables (only if they exist)
	config.loadFromOSEnv()

	// Step 3: Load from .env file (only if it exists and values exist)
	config.loadFromEnvFile()

	// Validate required configuration
	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// LoadWithOverrides loads configuration and applies CLI flag overrides
func LoadWithOverrides(overrides map[string]string) (*Config, error) {
	config := &Config{}

	// Step 1: Initialize with default values
	config.initializeDefaults()

	// Step 2: Load from OS environment variables (only if they exist)
	config.loadFromOSEnv()

	// Step 3: Load from .env file (only if it exists and values exist)
	config.loadFromEnvFile()

	// Step 4: Apply CLI flag overrides (only if they exist)
	config.applyOverrides(overrides)

	// Validate required configuration after all sources have been loaded
	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// initializeDefaults sets up the initial configuration with default values
func (c *Config) initializeDefaults() {
	c.Spotify = SpotifyConfig{
		ClientID:            "",                               // Empty by default
		ClientSecret:        "",                               // Empty by default
		RedirectURI:         "http://localhost:8080/callback", // Default value
		Username:            "",                               // Empty by default
		PlaylistIDs:         nil,                              // Empty by default
		ExcludedPlaylistIDs: nil,                              // Empty by default
	}

	c.Plex = PlexConfig{
		URL:              "", // Empty by default
		Token:            "", // Empty by default
		LibrarySectionID: 0,  // Empty by default
		ServerID:         "", // Empty by default (will be auto-discovered)
	}
}

// loadFromOSEnv loads configuration from OS environment variables (only if they exist)
func (c *Config) loadFromOSEnv() {
	// Spotify configuration
	if value := os.Getenv("SPOTIFY_CLIENT_ID"); value != "" {
		c.Spotify.ClientID = value
	}
	if value := os.Getenv("SPOTIFY_CLIENT_SECRET"); value != "" {
		c.Spotify.ClientSecret = value
	}
	if value := os.Getenv("SPOTIFY_REDIRECT_URI"); value != "" {
		c.Spotify.RedirectURI = value
	}
	if value := os.Getenv("SPOTIFY_USERNAME"); value != "" {
		c.Spotify.Username = value
	}
	if value := os.Getenv("SPOTIFY_PLAYLIST_ID"); value != "" {
		c.Spotify.PlaylistIDs = parseCommaSeparatedList(value)
	}
	if value := os.Getenv("SPOTIFY_PLAYLIST_EXCLUDED_ID"); value != "" {
		c.Spotify.ExcludedPlaylistIDs = parseCommaSeparatedList(value)
	}

	// Plex configuration
	if value := os.Getenv("PLEX_URL"); value != "" {
		c.Plex.URL = value
	}
	if value := os.Getenv("PLEX_TOKEN"); value != "" {
		c.Plex.Token = value
	}
	if value := os.Getenv("PLEX_LIBRARY_SECTION_ID"); value != "" {
		if sectionID, err := parseLibrarySectionID(value); err == nil {
			c.Plex.LibrarySectionID = sectionID
		}
	}
	if value := os.Getenv("PLEX_SERVER_ID"); value != "" {
		c.Plex.ServerID = value
	}
}

// loadFromEnvFile loads configuration from .env file (only if it exists and values exist)
func (c *Config) loadFromEnvFile() {
	// Load .env file if it exists
	if err := godotenv.Load(); err != nil {
		// .env file doesn't exist, skip this step
		return
	}

	// Spotify configuration (only replace if values exist and are not empty)
	if value := os.Getenv("SPOTIFY_CLIENT_ID"); value != "" {
		c.Spotify.ClientID = value
	}
	if value := os.Getenv("SPOTIFY_CLIENT_SECRET"); value != "" {
		c.Spotify.ClientSecret = value
	}
	if value := os.Getenv("SPOTIFY_REDIRECT_URI"); value != "" {
		c.Spotify.RedirectURI = value
	}
	if value := os.Getenv("SPOTIFY_USERNAME"); value != "" {
		c.Spotify.Username = value
	}
	if value := os.Getenv("SPOTIFY_PLAYLIST_ID"); value != "" {
		c.Spotify.PlaylistIDs = parseCommaSeparatedList(value)
	}
	if value := os.Getenv("SPOTIFY_PLAYLIST_EXCLUDED_ID"); value != "" {
		c.Spotify.ExcludedPlaylistIDs = parseCommaSeparatedList(value)
	}

	// Plex configuration (only replace if values exist and are not empty)
	if value := os.Getenv("PLEX_URL"); value != "" {
		c.Plex.URL = value
	}
	if value := os.Getenv("PLEX_TOKEN"); value != "" {
		c.Plex.Token = value
	}
	if value := os.Getenv("PLEX_LIBRARY_SECTION_ID"); value != "" {
		if sectionID, err := parseLibrarySectionID(value); err == nil {
			c.Plex.LibrarySectionID = sectionID
		}
	}
	if value := os.Getenv("PLEX_SERVER_ID"); value != "" {
		c.Plex.ServerID = value
	}
}

// parseCommaSeparatedList parses a comma-separated string into a slice of trimmed strings
func parseCommaSeparatedList(input string) []string {
	if input == "" {
		return nil
	}

	items := strings.Split(input, ",")
	for i, item := range items {
		items[i] = strings.TrimSpace(item)
	}

	return items
}

// parseLibrarySectionID parses the library section ID from string
func parseLibrarySectionID(value string) (int, error) {
	if value == "0" || value == "your_music_library_section_id" {
		return 0, nil
	}

	sectionID, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid section ID '%s': %w", value, err)
	}

	return sectionID, nil
}

// validate checks that all required configuration values are present
func (c *Config) validate() error {
	var missingFields []string

	// Check Spotify configuration
	if c.Spotify.ClientID == "" {
		missingFields = append(missingFields, "SPOTIFY_CLIENT_ID")
	}
	if c.Spotify.ClientSecret == "" {
		missingFields = append(missingFields, "SPOTIFY_CLIENT_SECRET")
	}

	// Check Plex configuration
	if c.Plex.URL == "" {
		missingFields = append(missingFields, "PLEX_URL")
	}
	if c.Plex.Token == "" {
		missingFields = append(missingFields, "PLEX_TOKEN")
	}
	if c.Plex.LibrarySectionID == 0 {
		missingFields = append(missingFields, "PLEX_LIBRARY_SECTION_ID")
	}

	// Check playlist configuration
	if c.Spotify.Username == "" && len(c.Spotify.PlaylistIDs) == 0 {
		missingFields = append(missingFields, "SPOTIFY_USERNAME or SPOTIFY_PLAYLIST_ID")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("missing required configuration values:\n%s\n\nSet these values via environment variables, .env file, or CLI flags", strings.Join(missingFields, "\n"))
	}

	return nil
}

// applyOverrides applies CLI flag overrides to the configuration (only if they exist)
func (c *Config) applyOverrides(overrides map[string]string) {
	for key, value := range overrides {
		// Only apply if the value is not empty
		if value == "" {
			continue
		}

		switch key {
		case "SPOTIFY_CLIENT_ID":
			c.Spotify.ClientID = value
		case "SPOTIFY_CLIENT_SECRET":
			c.Spotify.ClientSecret = value
		case "SPOTIFY_REDIRECT_URI":
			c.Spotify.RedirectURI = value
		case "SPOTIFY_USERNAME":
			c.Spotify.Username = value
		case "SPOTIFY_PLAYLIST_ID":
			c.Spotify.PlaylistIDs = parseCommaSeparatedList(value)
		case "SPOTIFY_PLAYLIST_EXCLUDED_ID":
			c.Spotify.ExcludedPlaylistIDs = parseCommaSeparatedList(value)
		case "PLEX_URL":
			c.Plex.URL = value
		case "PLEX_TOKEN":
			c.Plex.Token = value
		case "PLEX_LIBRARY_SECTION_ID":
			if sectionID, err := parseLibrarySectionID(value); err == nil {
				c.Plex.LibrarySectionID = sectionID
			}
		case "PLEX_SERVER_ID":
			c.Plex.ServerID = value
		}
	}
}
