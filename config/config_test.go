package config

import (
	"os"
	"strings"
	"testing"
)

func TestGetEnv(t *testing.T) {
	// Test with existing environment variable
	os.Setenv("TEST_VAR", "test_value")
	defer os.Unsetenv("TEST_VAR")

	value := getEnv("TEST_VAR", "default")
	if value != "test_value" {
		t.Errorf("Expected 'test_value', got %s", value)
	}

	// Test with non-existing environment variable
	value = getEnv("NON_EXISTENT_VAR", "default_value")
	if value != "default_value" {
		t.Errorf("Expected 'default_value', got %s", value)
	}
}

func TestConfigValidation(t *testing.T) {
	// Test that validation fails when required fields are missing
	cfg := &Config{}

	err := cfg.validate()
	if err == nil {
		t.Error("Expected validation to fail with empty config")
	}

	// Check that error message includes helpful information
	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "SPOTIFY_CLIENT_ID is required") {
		t.Error("Expected error message to mention SPOTIFY_CLIENT_ID")
	}
	if !strings.Contains(errorMsg, "PLEX_URL is required") {
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
	// Test the configuration hierarchy: env vars → .env → CLI flags

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
