package config

import (
	"os"
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
	// Test valid configuration
	cfg := &Config{
		Spotify: SpotifyConfig{
			ClientID:     "test_client_id",
			ClientSecret: "test_client_secret",
			PlaylistIDs:  []string{"test_playlist_id"},
		},
		Plex: PlexConfig{
			URL:              "http://test.plex.server:32400",
			Token:            "test_token",
			LibrarySectionID: 1,
			ServerID:         "test_server_id",
		},
	}

	err := cfg.validate()
	if err != nil {
		t.Errorf("Expected no validation error, got %v", err)
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

	// Test missing Spotify PlaylistIDs and Username (both optional now)
	cfg.Spotify.ClientSecret = "test_client_secret"
	cfg.Spotify.PlaylistIDs = []string{}
	cfg.Spotify.Username = ""
	err = cfg.validate()
	if err != nil {
		t.Errorf("Expected no validation error for missing PlaylistIDs and Username (both optional), got %v", err)
	}

	// Test missing Plex URL
	cfg.Spotify.PlaylistIDs = []string{"test_playlist_id"}
	cfg.Plex.URL = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing Plex URL")
	}

	// Test missing Plex Token
	cfg.Plex.URL = "http://test.plex.server:32400"
	cfg.Plex.ServerID = "test_server_id"
	cfg.Plex.Token = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing Plex Token")
	}

	// Test missing Plex LibrarySectionID
	cfg.Plex.Token = "test_token"
	cfg.Plex.ServerID = "test_server_id"
	cfg.Plex.LibrarySectionID = 0
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing LibrarySectionID")
	}
}
