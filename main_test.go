package main

import (
	"testing"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/plex"
	"github.com/grrywlsn/plexify/spotify"
)

func TestDisplayMissingTracksSummary(t *testing.T) {
	app := &Application{}

	// Create test data with missing tracks
	missingTracks := []plex.MatchResult{
		{
			SpotifySong: spotify.Song{
				ID:            "spotify_track_id_1",
				Name:          "Test Song 1",
				Artist:        "Test Artist 1",
				ISRC:          "TEST12345678",
				MusicBrainzID: "musicbrainz_id_1",
			},
			PlexTrack:  nil,
			MatchType:  "none",
			Confidence: 0.0,
		},
		{
			SpotifySong: spotify.Song{
				ID:            "spotify_track_id_2",
				Name:          "Test Song 2",
				Artist:        "Test Artist 2",
				ISRC:          "", // Empty ISRC to test that case
				MusicBrainzID: "", // Empty MusicBrainz ID to test that case
			},
			PlexTrack:  nil,
			MatchType:  "none",
			Confidence: 0.0,
		},
	}

	// Test that the function doesn't panic
	app.displayMissingTracksSummary(missingTracks)
}

func TestFilterExcludedPlaylists_NoExclusions(t *testing.T) {
	app := &Application{
		config: &config.Config{
			Spotify: config.SpotifyConfig{
				ExcludedPlaylistIDs: nil, // No exclusions
			},
		},
	}

	playlists := []PlaylistMeta{
		{ID: "playlist1", Name: "Playlist 1"},
		{ID: "playlist2", Name: "Playlist 2"},
		{ID: "playlist3", Name: "Playlist 3"},
	}

	result := app.filterExcludedPlaylists(playlists)

	if len(result) != 3 {
		t.Errorf("Expected 3 playlists, got %d", len(result))
	}
}

func TestFilterExcludedPlaylists_SingleExclusion(t *testing.T) {
	app := &Application{
		config: &config.Config{
			Spotify: config.SpotifyConfig{
				ExcludedPlaylistIDs: []string{"playlist2"},
			},
		},
	}

	playlists := []PlaylistMeta{
		{ID: "playlist1", Name: "Playlist 1"},
		{ID: "playlist2", Name: "Playlist 2"},
		{ID: "playlist3", Name: "Playlist 3"},
	}

	result := app.filterExcludedPlaylists(playlists)

	if len(result) != 2 {
		t.Errorf("Expected 2 playlists, got %d", len(result))
	}

	// Verify the correct playlists remain
	for _, pl := range result {
		if pl.ID == "playlist2" {
			t.Errorf("Playlist2 should have been excluded but was found in result")
		}
	}

	// Verify playlist1 and playlist3 are still present
	found1, found3 := false, false
	for _, pl := range result {
		if pl.ID == "playlist1" {
			found1 = true
		}
		if pl.ID == "playlist3" {
			found3 = true
		}
	}
	if !found1 {
		t.Error("Playlist1 should be in the result but was not found")
	}
	if !found3 {
		t.Error("Playlist3 should be in the result but was not found")
	}
}

func TestFilterExcludedPlaylists_MultipleExclusions(t *testing.T) {
	app := &Application{
		config: &config.Config{
			Spotify: config.SpotifyConfig{
				ExcludedPlaylistIDs: []string{"playlist1", "playlist3"},
			},
		},
	}

	playlists := []PlaylistMeta{
		{ID: "playlist1", Name: "Playlist 1"},
		{ID: "playlist2", Name: "Playlist 2"},
		{ID: "playlist3", Name: "Playlist 3"},
		{ID: "playlist4", Name: "Playlist 4"},
	}

	result := app.filterExcludedPlaylists(playlists)

	if len(result) != 2 {
		t.Errorf("Expected 2 playlists, got %d", len(result))
	}

	// Verify the correct playlists remain
	for _, pl := range result {
		if pl.ID == "playlist1" || pl.ID == "playlist3" {
			t.Errorf("Playlist %s should have been excluded but was found in result", pl.ID)
		}
	}

	// Verify playlist2 and playlist4 are still present
	found2, found4 := false, false
	for _, pl := range result {
		if pl.ID == "playlist2" {
			found2 = true
		}
		if pl.ID == "playlist4" {
			found4 = true
		}
	}
	if !found2 {
		t.Error("Playlist2 should be in the result but was not found")
	}
	if !found4 {
		t.Error("Playlist4 should be in the result but was not found")
	}
}

func TestFilterExcludedPlaylists_ExcludeNonexistent(t *testing.T) {
	app := &Application{
		config: &config.Config{
			Spotify: config.SpotifyConfig{
				ExcludedPlaylistIDs: []string{"nonexistent_playlist"},
			},
		},
	}

	playlists := []PlaylistMeta{
		{ID: "playlist1", Name: "Playlist 1"},
		{ID: "playlist2", Name: "Playlist 2"},
	}

	result := app.filterExcludedPlaylists(playlists)

	// All playlists should remain since the excluded ID doesn't match any
	if len(result) != 2 {
		t.Errorf("Expected 2 playlists, got %d", len(result))
	}
}

func TestFilterExcludedPlaylists_ExcludeAll(t *testing.T) {
	app := &Application{
		config: &config.Config{
			Spotify: config.SpotifyConfig{
				ExcludedPlaylistIDs: []string{"playlist1", "playlist2"},
			},
		},
	}

	playlists := []PlaylistMeta{
		{ID: "playlist1", Name: "Playlist 1"},
		{ID: "playlist2", Name: "Playlist 2"},
	}

	result := app.filterExcludedPlaylists(playlists)

	if len(result) != 0 {
		t.Errorf("Expected 0 playlists, got %d", len(result))
	}
}

func TestFilterExcludedPlaylists_EmptyInput(t *testing.T) {
	app := &Application{
		config: &config.Config{
			Spotify: config.SpotifyConfig{
				ExcludedPlaylistIDs: []string{"playlist1"},
			},
		},
	}

	playlists := []PlaylistMeta{}

	result := app.filterExcludedPlaylists(playlists)

	if len(result) != 0 {
		t.Errorf("Expected 0 playlists, got %d", len(result))
	}
}
