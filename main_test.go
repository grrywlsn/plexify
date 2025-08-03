package main

import (
	"testing"

	"github.com/garry/plexify/plex"
	"github.com/garry/plexify/spotify"
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
