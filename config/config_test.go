package config

import (
	"os"
	"strings"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	// Test that validation fails when required fields are missing
	cfg := &Config{}

	err := cfg.validate()
	if err == nil {
		t.Error("Expected validation to fail with empty config")
	}

	// Check that error message includes helpful information
	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "SPOTIFY_CLIENT_ID") {
		t.Error("Expected error message to mention SPOTIFY_CLIENT_ID")
	}
	if !strings.Contains(errorMsg, "PLEX_URL") {
		t.Error("Expected error message to mention PLEX_URL")
	}

	// Test valid configuration with playlist IDs
	cfg = &Config{
		Spotify: SpotifyConfig{
			ClientID:     "test_client_id",
			ClientSecret: "test_client_secret",
			PlaylistIDs:  []string{"test_playlist_id"},
		},
		Plex: PlexConfig{
			URL:              "http://test.plex.server:32400",
			Token:            "test_token",
			LibrarySectionID: 1,
		},
	}

	err = cfg.validate()
	if err != nil {
		t.Errorf("Expected no validation error, got %v", err)
	}

	// Test valid configuration with username
	cfg.Spotify.PlaylistIDs = []string{}
	cfg.Spotify.Username = "test_username"
	err = cfg.validate()
	if err != nil {
		t.Errorf("Expected no validation error with username, got %v", err)
	}

	// Test missing Spotify ClientID
	cfg.Spotify.ClientID = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing ClientID")
	}

	// Test missing Spotify ClientSecret
	cfg.Spotify.ClientID = "test_client_id"
	cfg.Spotify.ClientSecret = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing ClientSecret")
	}

	// Test missing playlist source (both username and playlist IDs)
	cfg.Spotify.ClientSecret = "test_client_secret"
	cfg.Spotify.PlaylistIDs = []string{}
	cfg.Spotify.Username = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing playlist source")
	}

	// Test missing Plex URL
	cfg.Spotify.Username = "test_username"
	cfg.Plex.URL = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing Plex URL")
	}

	// Test missing Plex Token
	cfg.Plex.URL = "http://test.plex.server:32400"
	cfg.Plex.Token = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing Plex Token")
	}

	// Test missing Plex LibrarySectionID
	cfg.Plex.Token = "test_token"
	cfg.Plex.LibrarySectionID = 0
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing LibrarySectionID")
	}
}

func TestConfigHierarchy(t *testing.T) {
	// Test the configuration hierarchy: defaults -> OS env -> .env -> CLI flags

	// Set up required environment variables for validation
	os.Setenv("SPOTIFY_CLIENT_ID", "test_client_id")
	os.Setenv("SPOTIFY_CLIENT_SECRET", "test_client_secret")
	os.Setenv("PLEX_URL", "http://test:32400")
	os.Setenv("PLEX_TOKEN", "test_token")
	os.Setenv("PLEX_LIBRARY_SECTION_ID", "1")
	os.Setenv("SPOTIFY_USERNAME", "env_user")
	defer func() {
		os.Unsetenv("SPOTIFY_CLIENT_ID")
		os.Unsetenv("SPOTIFY_CLIENT_SECRET")
		os.Unsetenv("PLEX_URL")
		os.Unsetenv("PLEX_TOKEN")
		os.Unsetenv("PLEX_LIBRARY_SECTION_ID")
		os.Unsetenv("SPOTIFY_USERNAME")
	}()

	// Load base config (should use env var)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.Spotify.Username != "env_user" {
		t.Errorf("Expected username 'env_user', got '%s'", cfg.Spotify.Username)
	}

	// Test CLI override
	overrides := map[string]string{
		"SPOTIFY_USERNAME": "cli_user",
	}

	cfgWithOverrides, err := LoadWithOverrides(overrides)
	if err != nil {
		t.Fatalf("Failed to load config with overrides: %v", err)
	}

	if cfgWithOverrides.Spotify.Username != "cli_user" {
		t.Errorf("Expected username 'cli_user' after CLI override, got '%s'", cfgWithOverrides.Spotify.Username)
	}

	// Test multiple overrides
	multipleOverrides := map[string]string{
		"SPOTIFY_USERNAME":        "cli_user2",
		"PLEX_LIBRARY_SECTION_ID": "5",
		"PLEX_URL":                "http://test2:32400",
	}

	cfgMultiple, err := LoadWithOverrides(multipleOverrides)
	if err != nil {
		t.Fatalf("Failed to load config with overrides: %v", err)
	}

	if cfgMultiple.Spotify.Username != "cli_user2" {
		t.Errorf("Expected username 'cli_user2', got '%s'", cfgMultiple.Spotify.Username)
	}

	if cfgMultiple.Plex.LibrarySectionID != 5 {
		t.Errorf("Expected library section ID 5, got %d", cfgMultiple.Plex.LibrarySectionID)
	}

	if cfgMultiple.Plex.URL != "http://test2:32400" {
		t.Errorf("Expected URL 'http://test2:32400', got '%s'", cfgMultiple.Plex.URL)
	}
}

func TestApplyOverrides(t *testing.T) {
	cfg := &Config{
		Spotify: SpotifyConfig{
			Username: "original_user",
		},
		Plex: PlexConfig{
			LibrarySectionID: 1,
		},
	}

	overrides := map[string]string{
		"SPOTIFY_USERNAME":        "new_user",
		"PLEX_LIBRARY_SECTION_ID": "10",
		"PLEX_URL":                "http://override:32400",
	}

	cfg.applyOverrides(overrides)

	if cfg.Spotify.Username != "new_user" {
		t.Errorf("Expected username 'new_user', got '%s'", cfg.Spotify.Username)
	}

	if cfg.Plex.LibrarySectionID != 10 {
		t.Errorf("Expected library section ID 10, got %d", cfg.Plex.LibrarySectionID)
	}

	if cfg.Plex.URL != "http://override:32400" {
		t.Errorf("Expected URL 'http://override:32400', got '%s'", cfg.Plex.URL)
	}
}

func TestInitializeDefaults(t *testing.T) {
	cfg := &Config{}
	cfg.initializeDefaults()

	// Test that defaults are set correctly
	if cfg.Spotify.ClientID != "" {
		t.Errorf("Expected empty ClientID, got '%s'", cfg.Spotify.ClientID)
	}
	if cfg.Spotify.ClientSecret != "" {
		t.Errorf("Expected empty ClientSecret, got '%s'", cfg.Spotify.ClientSecret)
	}
	if cfg.Spotify.RedirectURI != "http://localhost:8080/callback" {
		t.Errorf("Expected default RedirectURI, got '%s'", cfg.Spotify.RedirectURI)
	}
	if cfg.Spotify.Username != "" {
		t.Errorf("Expected empty Username, got '%s'", cfg.Spotify.Username)
	}
	if cfg.Spotify.PlaylistIDs != nil {
		t.Errorf("Expected nil PlaylistIDs, got %v", cfg.Spotify.PlaylistIDs)
	}

	if cfg.Plex.URL != "" {
		t.Errorf("Expected empty URL, got '%s'", cfg.Plex.URL)
	}
	if cfg.Plex.Token != "" {
		t.Errorf("Expected empty Token, got '%s'", cfg.Plex.Token)
	}
	if cfg.Plex.LibrarySectionID != 0 {
		t.Errorf("Expected 0 LibrarySectionID, got %d", cfg.Plex.LibrarySectionID)
	}
	if cfg.Plex.ServerID != "" {
		t.Errorf("Expected empty ServerID, got '%s'", cfg.Plex.ServerID)
	}
}

func TestLoadFromOSEnv(t *testing.T) {
	cfg := &Config{}
	cfg.initializeDefaults()

	// Set some environment variables
	os.Setenv("SPOTIFY_CLIENT_ID", "test_client_id")
	os.Setenv("SPOTIFY_USERNAME", "test_user")
	os.Setenv("PLEX_URL", "http://test:32400")
	defer func() {
		os.Unsetenv("SPOTIFY_CLIENT_ID")
		os.Unsetenv("SPOTIFY_USERNAME")
		os.Unsetenv("PLEX_URL")
	}()

	cfg.loadFromOSEnv()

	// Test that values were loaded
	if cfg.Spotify.ClientID != "test_client_id" {
		t.Errorf("Expected ClientID 'test_client_id', got '%s'", cfg.Spotify.ClientID)
	}
	if cfg.Spotify.Username != "test_user" {
		t.Errorf("Expected Username 'test_user', got '%s'", cfg.Spotify.Username)
	}
	if cfg.Plex.URL != "http://test:32400" {
		t.Errorf("Expected URL 'http://test:32400', got '%s'", cfg.Plex.URL)
	}

	// Test that empty values don't override defaults
	if cfg.Spotify.RedirectURI != "http://localhost:8080/callback" {
		t.Errorf("Expected default RedirectURI, got '%s'", cfg.Spotify.RedirectURI)
	}
}

func TestApplyOverridesEmptyValues(t *testing.T) {
	cfg := &Config{
		Spotify: SpotifyConfig{
			Username: "original_user",
		},
		Plex: PlexConfig{
			LibrarySectionID: 1,
		},
	}

	// Test that empty values in overrides don't change existing values
	overrides := map[string]string{
		"SPOTIFY_USERNAME":        "", // Empty value
		"PLEX_LIBRARY_SECTION_ID": "10",
		"PLEX_URL":                "", // Empty value
	}

	cfg.applyOverrides(overrides)

	// Username should remain unchanged because override was empty
	if cfg.Spotify.Username != "original_user" {
		t.Errorf("Expected username 'original_user' (unchanged), got '%s'", cfg.Spotify.Username)
	}

	// LibrarySectionID should be updated
	if cfg.Plex.LibrarySectionID != 10 {
		t.Errorf("Expected library section ID 10, got %d", cfg.Plex.LibrarySectionID)
	}

	// URL should remain unchanged because override was empty
	if cfg.Plex.URL != "" {
		t.Errorf("Expected empty URL (unchanged), got '%s'", cfg.Plex.URL)
	}
}
