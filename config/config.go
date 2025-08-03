package config

import (
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration values
type Config struct {
	Spotify SpotifyConfig
	Plex    PlexConfig
	App     AppConfig
}

// SpotifyConfig holds Spotify API configuration
type SpotifyConfig struct {
	ClientID     string
	ClientSecret string
	RedirectURI  string
	Username     string   // Spotify username to get all public playlists
	PlaylistIDs  []string // Spotify playlist IDs from comma-separated list (legacy)
}

// PlexConfig holds Plex server configuration
type PlexConfig struct {
	URL              string
	Token            string
	LibrarySectionID int
	ServerID         string
}

// AppConfig holds application-level configuration
type AppConfig struct {
	// Reserved for future use
}

// Load loads configuration from environment variables
func Load() (*Config, error) {
	// Load environment variables from .env file
	if err := loadEnvFile(); err != nil {
		log.Printf(".env file not found, using system environment variables")
	}

	config := &Config{}

	// Load Spotify configuration
	if err := config.loadSpotifyConfig(); err != nil {
		return nil, fmt.Errorf("failed to load Spotify config: %w", err)
	}

	// Load Plex configuration
	if err := config.loadPlexConfig(); err != nil {
		return nil, fmt.Errorf("failed to load Plex config: %w", err)
	}

	// Load application configuration
	config.loadAppConfig()

	// Validate required configuration
	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// loadEnvFile loads environment variables from .env file
func loadEnvFile() error {
	return godotenv.Load()
}

// loadSpotifyConfig loads Spotify configuration from environment variables
func (c *Config) loadSpotifyConfig() error {
	// Load playlist IDs from comma-separated list
	playlistIDsStr := getEnv("SPOTIFY_PLAYLIST_ID", "")
	var playlistIDs []string

	if playlistIDsStr != "" {
		playlistIDs = parseCommaSeparatedList(playlistIDsStr)
	}

	c.Spotify = SpotifyConfig{
		ClientID:     getEnv("SPOTIFY_CLIENT_ID", ""),
		ClientSecret: getEnv("SPOTIFY_CLIENT_SECRET", ""),
		RedirectURI:  getEnv("SPOTIFY_REDIRECT_URI", "http://localhost:8080/callback"),
		Username:     getEnv("SPOTIFY_USERNAME", ""),
		PlaylistIDs:  playlistIDs,
	}

	return nil
}

// loadPlexConfig loads Plex configuration from environment variables
func (c *Config) loadPlexConfig() error {
	// Load library section ID
	librarySectionID, err := parseLibrarySectionID(getEnv("PLEX_LIBRARY_SECTION_ID", "0"))
	if err != nil {
		return fmt.Errorf("invalid PLEX_LIBRARY_SECTION_ID: %w", err)
	}

	c.Plex = PlexConfig{
		URL:              getEnv("PLEX_URL", ""),
		Token:            getEnv("PLEX_TOKEN", ""),
		LibrarySectionID: librarySectionID,
		ServerID:         getEnv("PLEX_SERVER_ID", ""),
	}

	// Log auto-discovery message if server ID is not set
	if c.Plex.ServerID == "" {
		log.Printf("PLEX_SERVER_ID not set, attempting to auto-discover from server...")
	}

	return nil
}

// loadAppConfig loads application configuration from environment variables
func (c *Config) loadAppConfig() {
	c.App = AppConfig{
		// Reserved for future use
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
	// Validate Spotify configuration
	if err := c.validateSpotifyConfig(); err != nil {
		return fmt.Errorf("Spotify config validation failed: %w", err)
	}

	// Validate Plex configuration
	if err := c.validatePlexConfig(); err != nil {
		return fmt.Errorf("Plex config validation failed: %w", err)
	}

	return nil
}

// validateSpotifyConfig validates Spotify configuration
func (c *Config) validateSpotifyConfig() error {
	if c.Spotify.ClientID == "" {
		return fmt.Errorf("SPOTIFY_CLIENT_ID is required")
	}
	if c.Spotify.ClientSecret == "" {
		return fmt.Errorf("SPOTIFY_CLIENT_SECRET is required")
	}

	// Note: SPOTIFY_PLAYLIST_ID is optional - users can use SPOTIFY_USERNAME instead
	// or provide playlist IDs via command line arguments

	return nil
}

// validatePlexConfig validates Plex configuration
func (c *Config) validatePlexConfig() error {
	if c.Plex.URL == "" {
		return fmt.Errorf("PLEX_URL is required")
	}
	if c.Plex.Token == "" {
		return fmt.Errorf("PLEX_TOKEN is required")
	}
	if c.Plex.LibrarySectionID == 0 {
		return fmt.Errorf("PLEX_LIBRARY_SECTION_ID is required")
	}

	return nil
}

// getEnv gets an environment variable with a fallback default value
func getEnv(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
