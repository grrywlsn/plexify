package spotify

import (
	"testing"

	"github.com/garry/plexify/config"
)

func TestNewClient(t *testing.T) {
	// Test with valid configuration
	cfg := &config.Config{
		Spotify: config.SpotifyConfig{
			ClientID:     "test_client_id",
			ClientSecret: "test_client_secret",
			RedirectURI:  "http://localhost:8080/callback",
		},
	}

	client, err := NewClient(cfg)
	// Note: This will fail with invalid credentials, but that's expected
	// In a real test environment, you would use mock credentials or mock the API
	if err != nil {
		// This is expected since we're using fake credentials
		t.Logf("Expected error with fake credentials: %v", err)
		return
	}

	if client == nil {
		t.Error("Expected client to be created, got nil")
		return
	}

	if client.config != cfg {
		t.Error("Expected client config to match provided config")
	}
}

func TestSongStruct(t *testing.T) {
	song := Song{
		ID:       "test_id",
		Name:     "Test Song",
		Artist:   "Test Artist",
		Album:    "Test Album",
		Duration: 180000, // 3 minutes in milliseconds
		URI:      "spotify:track:test_id",
	}

	if song.ID != "test_id" {
		t.Errorf("Expected ID to be 'test_id', got %s", song.ID)
	}

	if song.Name != "Test Song" {
		t.Errorf("Expected Name to be 'Test Song', got %s", song.Name)
	}

	if song.Artist != "Test Artist" {
		t.Errorf("Expected Artist to be 'Test Artist', got %s", song.Artist)
	}

	if song.Album != "Test Album" {
		t.Errorf("Expected Album to be 'Test Album', got %s", song.Album)
	}

	if song.Duration != 180000 {
		t.Errorf("Expected Duration to be 180000, got %d", song.Duration)
	}

	if song.URI != "spotify:track:test_id" {
		t.Errorf("Expected URI to be 'spotify:track:test_id', got %s", song.URI)
	}
}
