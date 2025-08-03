package musicbrainz

import (
	"context"
	"testing"
)

func TestNewClient(t *testing.T) {
	client := NewClient()
	if client == nil {
		t.Error("Expected client to be created, got nil")
	}

	if client.httpClient == nil {
		t.Error("Expected httpClient to be initialized, got nil")
	}

	if client.userAgent == "" {
		t.Error("Expected userAgent to be set, got empty string")
	}
}

func TestGetMusicBrainzIDByISRC(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	// Test with empty ISRC
	_, err := client.GetMusicBrainzIDByISRC(ctx, "")
	if err == nil {
		t.Error("Expected error for empty ISRC, got nil")
	}

	// Test with a known ISRC (this will make a real API call)
	// Note: This test might fail if the API is down or the ISRC is no longer valid
	isrc := "USRC12345678"
	musicBrainzID, err := client.GetMusicBrainzIDByISRC(ctx, isrc)

	// We expect either a valid ID or an error (if the ISRC doesn't exist)
	if err != nil {
		t.Logf("Expected error for ISRC %s: %v", isrc, err)
	} else {
		if musicBrainzID == "" {
			t.Error("Expected non-empty MusicBrainz ID, got empty string")
		}
		t.Logf("Found MusicBrainz ID: %s for ISRC: %s", musicBrainzID, isrc)
	}

	// Test with the specific ISRC mentioned by the user
	specificISRC := "QM6N21781333"
	t.Logf("Testing specific ISRC: %s", specificISRC)
	musicBrainzID, err = client.GetMusicBrainzIDByISRC(ctx, specificISRC)

	if err != nil {
		t.Logf("Error for ISRC %s: %v", specificISRC, err)
	} else {
		t.Logf("Found MusicBrainz ID: %s for ISRC: %s", musicBrainzID, specificISRC)
		expectedID := "5da7cc9a-81e8-4e33-b023-2be9febab808"
		if musicBrainzID != expectedID {
			t.Errorf("Expected MusicBrainz ID %s, got %s", expectedID, musicBrainzID)
		}
	}
}

func TestGetMusicBrainzIDByArtistAndTitle(t *testing.T) {
	client := NewClient()
	ctx := context.Background()

	// Test with empty artist
	_, err := client.GetMusicBrainzIDByArtistAndTitle(ctx, "", "Test Song")
	if err == nil {
		t.Error("Expected error for empty artist, got nil")
	}

	// Test with empty title
	_, err = client.GetMusicBrainzIDByArtistAndTitle(ctx, "Test Artist", "")
	if err == nil {
		t.Error("Expected error for empty title, got nil")
	}

	// Test with a known artist and title (this will make a real API call)
	// Note: This test might fail if the API is down or the track doesn't exist
	artist := "The Beatles"
	title := "Hey Jude"
	musicBrainzID, err := client.GetMusicBrainzIDByArtistAndTitle(ctx, artist, title)

	// We expect either a valid ID or an error (if the track doesn't exist)
	if err != nil {
		t.Logf("Expected error for artist: %s, title: %s: %v", artist, title, err)
	} else {
		if musicBrainzID == "" {
			t.Error("Expected non-empty MusicBrainz ID, got empty string")
		}
		t.Logf("Found MusicBrainz ID: %s for artist: %s, title: %s", musicBrainzID, artist, title)
	}
}
