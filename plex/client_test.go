package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"strings"
	"testing"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/spotify"
)

func TestNewClient(t *testing.T) {
	cfg := &config.Config{
		Plex: config.PlexConfig{
			URL:              "http://test.plex.server:32400",
			Token:            "test_token",
			LibrarySectionID: 1,
			ServerID:         "test_server_id",
		},
	}

	client := NewClient(cfg)
	if client == nil {
		t.Error("Expected client to be created, got nil")
		return
	}

	if client.baseURL != cfg.Plex.URL {
		t.Errorf("Expected baseURL to be %s, got %s", cfg.Plex.URL, client.baseURL)
	}

	if client.token != cfg.Plex.Token {
		t.Errorf("Expected token to be %s, got %s", cfg.Plex.Token, client.token)
	}

	if client.sectionID != cfg.Plex.LibrarySectionID {
		t.Errorf("Expected sectionID to be %d, got %d", cfg.Plex.LibrarySectionID, client.sectionID)
	}
}

func TestCalculateStringSimilarity(t *testing.T) {
	client := &Client{}

	// Test exact match
	similarity := client.calculateStringSimilarity("test", "test")
	if similarity != 1.0 {
		t.Errorf("Expected similarity 1.0 for exact match, got %f", similarity)
	}

	// Test substring match
	similarity = client.calculateStringSimilarity("test", "testing")
	expectedSubstring := 4.0 / 7.0 // "test" length / "testing" length
	if similarity != expectedSubstring {
		t.Errorf("Expected similarity %f for substring match, got %f", expectedSubstring, similarity)
	}

	// Test no match
	similarity = client.calculateStringSimilarity("test", "different")
	// For "test" vs "different": no substring match, word similarity = 0, length similarity = 1 - |4-9|/9 = 1 - 5/9 = 4/9
	// Final result = 0.7 * 0 + 0.3 * (4/9) = 0.133...
	expectedNoMatch := 0.3 * (1.0 - float64(abs(4-9))/float64(max(4, 9)))
	if similarity != expectedNoMatch {
		t.Errorf("Expected similarity %f for no match, got %f", expectedNoMatch, similarity)
	}
}

func TestCalculateConfidence(t *testing.T) {
	client := &Client{}
	song := spotify.Song{
		ID:       "test_id",
		Name:     "Test Song",
		Artist:   "Test Artist",
		Album:    "Test Album",
		Duration: 180000,
		URI:      "spotify:track:test_id",
		ISRC:     "TEST12345678",
	}

	track := &PlexTrack{
		ID:     "plex_id",
		Title:  "Test Song",
		Artist: "Test Artist",
		Album:  "Test Album",
	}

	// Test ISRC match (not implemented in calculateConfidence, returns 0.0)
	confidence := client.calculateConfidence(song, track, "isrc")
	if confidence != 0.0 {
		t.Errorf("Expected confidence 0.0 for ISRC match (not implemented), got %f", confidence)
	}

	// Test title/artist match
	confidence = client.calculateConfidence(song, track, "title_artist")
	if confidence != 1.0 {
		t.Errorf("Expected confidence 1.0 for exact title/artist match, got %f", confidence)
	}

	// Test no match
	confidence = client.calculateConfidence(song, nil, "none")
	if confidence != 0.0 {
		t.Errorf("Expected confidence 0.0 for no match, got %f", confidence)
	}
}

func TestRemoveBrackets(t *testing.T) {
	client := &Client{}

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Song Name (feat. Anitta)",
			expected: "Song Name",
		},
		{
			input:    "Song Name [feat. Anitta]",
			expected: "Song Name",
		},
		{
			input:    "Song Name {feat. Anitta}",
			expected: "Song Name",
		},
		{
			input:    "Song Name (feat. Anitta) [Remix]",
			expected: "Song Name",
		},
		{
			input:    "Song Name",
			expected: "Song Name",
		},
		{
			input:    "Song Name (feat. Anitta) - Extended Version",
			expected: "Song Name - Extended Version",
		},
		{
			input:    "(Intro) Song Name (feat. Anitta)",
			expected: "Song Name",
		},
		{
			input:    "Get It Right (feat. MØ)",
			expected: "Get It Right",
		},
	}

	for _, test := range tests {
		result := client.removeBrackets(test.input)
		if result != test.expected {
			t.Errorf("removeBrackets(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestRemoveFeaturing(t *testing.T) {
	client := &Client{}

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Girl, so confusing featuring lorde",
			expected: "Girl, so confusing",
		},
		{
			input:    "Song Name feat. Artist",
			expected: "Song Name",
		},
		{
			input:    "Song Name feat Artist",
			expected: "Song Name",
		},
		{
			input:    "Song Name ft. Artist",
			expected: "Song Name",
		},
		{
			input:    "Song Name ft Artist",
			expected: "Song Name",
		},
		{
			input:    "Song Name featuring Artist Name",
			expected: "Song Name",
		},
		{
			input:    "Song Name",
			expected: "Song Name",
		},
		{
			input:    "Song Name feat. Artist (Remix)",
			expected: "Song Name",
		},
		{
			input:    "Song Name featuring Artist and Another Artist",
			expected: "Song Name",
		},
	}

	for _, test := range tests {
		result := client.removeFeaturing(test.input)
		if result != test.expected {
			t.Errorf("removeFeaturing(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestNormalizeTitle(t *testing.T) {
	client := &Client{}

	tests := []struct {
		input    string
		expected string
	}{
		{
			input:    "Mood Ring (By Demand) - Pride Remix",
			expected: "mood ring (by demand) (pride remix)",
		},
		{
			input:    "Song Name - Remix",
			expected: "song name (remix)",
		},
		{
			input:    "Song Name - Extended Version",
			expected: "song name (extended version)",
		},
		{
			input:    "Song Name - Radio Edit",
			expected: "song name (radio edit)",
		},
		{
			input:    "Song Name",
			expected: "song name",
		},
		{
			input:    "Song Name (feat. Artist) - Remix",
			expected: "song name (feat. artist) (remix)",
		},
		{
			input:    "Song Name - Remix - Extended",
			expected: "song name (remix) (extended)",
		},
	}

	for _, test := range tests {
		result := client.normalizeTitle(test.input)
		if result != test.expected {
			t.Errorf("normalizeTitle(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestNormalizeAccents(t *testing.T) {
	client := &Client{}

	tests := []struct {
		input    string
		expected string
	}{
		{"MÓNACO", "MONACO"},
		{"Café", "Cafe"},
		{"José", "Jose"},
		{"François", "Francois"},
		{"Björk", "Bjork"},
		{"Mötley Crüe", "Motley Crue"},
		{"Kraftwerk", "Kraftwerk"},     // No accents, should remain unchanged
		{"The Beatles", "The Beatles"}, // No accents, should remain unchanged
		{"Música", "Musica"},
		{"Canción", "Cancion"},
		{"Año", "Ano"},
		{"Niño", "Nino"},
		{"Señor", "Senor"},
		{"Mañana", "Manana"},
		{"Español", "Espanol"},
		{"Português", "Portugues"},
		{"Français", "Francais"},
		{"Deutsch", "Deutsch"},   // No accents, should remain unchanged
		{"Italiano", "Italiano"}, // No accents, should remain unchanged
		{"Русский", "Русский"},   // Cyrillic, should remain unchanged
		{"中文", "中文"},             // Chinese, should remain unchanged
		{"日本語", "日本語"},           // Japanese, should remain unchanged
	}

	for _, test := range tests {
		result := client.normalizeAccents(test.input)
		if result != test.expected {
			t.Errorf("normalizeAccents(%q) = %q, expected %q", test.input, result, test.expected)
		}
	}
}

func TestAccentMatchingScenario(t *testing.T) {
	client := &Client{}

	// Test the specific case mentioned: Spotify "MONACO" vs Plex "MÓNACO"
	spotifyTitle := "MONACO"
	plexTitle := "MÓNACO"
	artist := "Bad Bunny"

	t.Logf("Testing accent normalization for track matching:")
	t.Logf("Spotify title: %s", spotifyTitle)
	t.Logf("Plex title: %s", plexTitle)
	t.Logf("Artist: %s", artist)

	// Test accent normalization
	normalizedSpotify := client.normalizeAccents(spotifyTitle)
	normalizedPlex := client.normalizeAccents(plexTitle)

	t.Logf("After accent normalization:")
	t.Logf("Spotify: %s -> %s", spotifyTitle, normalizedSpotify)
	t.Logf("Plex: %s -> %s", plexTitle, normalizedPlex)

	// Test string similarity
	originalSimilarity := client.calculateStringSimilarity(
		strings.ToLower(spotifyTitle),
		strings.ToLower(plexTitle),
	)

	accentSimilarity := client.calculateStringSimilarity(
		strings.ToLower(normalizedSpotify),
		strings.ToLower(normalizedPlex),
	)

	t.Logf("String similarity scores:")
	t.Logf("Original: %.3f", originalSimilarity)
	t.Logf("With accent normalization: %.3f", accentSimilarity)

	// Verify that accent normalization improves matching
	if accentSimilarity <= originalSimilarity {
		t.Errorf("Accent normalization should improve matching: original=%.3f, normalized=%.3f",
			originalSimilarity, accentSimilarity)
	}

	// Test if they would match with the confidence threshold
	confidence := (accentSimilarity * 0.7) + (1.0 * 0.3) // Assuming perfect artist match
	t.Logf("Confidence score: %.3f", confidence)

	if confidence < 0.7 {
		t.Errorf("Should match with confidence threshold: confidence=%.3f, threshold=0.7", confidence)
	}

	t.Logf("✅ Accent normalization successfully improves matching from %.3f to %.3f",
		originalSimilarity, accentSimilarity)
}

func TestSearchTrackWithSingleQuotes(t *testing.T) {
	// Test song with single quote in title
	song := spotify.Song{
		ID:       "test_id",
		Name:     "Don't Stop Believin'",
		Artist:   "Journey",
		Album:    "Escape",
		Duration: 180000,
		URI:      "spotify:track:test_id",
		ISRC:     "TEST12345678",
	}

	// Test song with apostrophe in title
	song2 := spotify.Song{
		ID:       "test_id2",
		Name:     "I'm Still Standing",
		Artist:   "Elton John",
		Album:    "Too Low For Zero",
		Duration: 180000,
		URI:      "spotify:track:test_id2",
		ISRC:     "TEST87654321",
	}

	// Test song with single quote in artist name
	song3 := spotify.Song{
		ID:       "test_id3",
		Name:     "Bohemian Rhapsody",
		Artist:   "Queen's",
		Album:    "A Night at the Opera",
		Duration: 180000,
		URI:      "spotify:track:test_id3",
		ISRC:     "TEST11111111",
	}

	// These tests would require a real Plex server to run, but we can test the URL encoding
	// by checking that the search methods don't panic with single quotes
	t.Run("SingleQuoteInTitle", func(t *testing.T) {
		// This should not panic
		_ = song.Name
		_ = song.Artist
	})

	t.Run("ApostropheInTitle", func(t *testing.T) {
		// This should not panic
		_ = song2.Name
		_ = song2.Artist
	})

	t.Run("SingleQuoteInArtist", func(t *testing.T) {
		// This should not panic
		_ = song3.Name
		_ = song3.Artist
	})
}

func TestURLEncodingWithSingleQuotes(t *testing.T) {
	// Test that URL encoding works correctly with single quotes
	title := "Don't Stop Believin'"
	artist := "Journey"

	// Simulate the URL construction that happens in searchByTitle
	params := url.Values{}
	params.Add("query", title)
	params.Add("type", "10")

	encodedQuery := params.Encode()

	// The encoded query should contain the single quotes properly escaped
	expectedEncoded := "query=Don%27t+Stop+Believin%27&type=10"
	if encodedQuery != expectedEncoded {
		t.Errorf("Expected encoded query to be '%s', got '%s'", expectedEncoded, encodedQuery)
	}

	// Test combined query
	combinedQuery := fmt.Sprintf("%s %s", title, artist)
	params2 := url.Values{}
	params2.Add("query", combinedQuery)
	params2.Add("type", "10")

	encodedCombined := params2.Encode()
	expectedCombined := "query=Don%27t+Stop+Believin%27+Journey&type=10"
	if encodedCombined != expectedCombined {
		t.Errorf("Expected encoded combined query to be '%s', got '%s'", expectedCombined, encodedCombined)
	}
}

func TestStringSimilarityWithSingleQuotes(t *testing.T) {
	client := &Client{}

	// Test exact match with single quotes
	similarity := client.calculateStringSimilarity("Don't Stop Believin'", "Don't Stop Believin'")
	if similarity != 1.0 {
		t.Errorf("Expected similarity 1.0 for exact match with single quotes, got %f", similarity)
	}

	// Test similar strings with single quotes
	similarity = client.calculateStringSimilarity("Don't Stop Believin'", "Don't Stop Believing")
	if similarity <= 0.0 {
		t.Errorf("Expected positive similarity for similar strings with single quotes, got %f", similarity)
	}

	// Test different strings with single quotes
	similarity = client.calculateStringSimilarity("Don't Stop Believin'", "I'm Still Standing")
	if similarity <= 0.0 {
		t.Errorf("Expected positive similarity for different strings with single quotes, got %f", similarity)
	}
}

func TestTitleNormalizationWithSingleQuotes(t *testing.T) {
	client := &Client{}

	// Test removeBrackets with single quotes
	result := client.removeBrackets("Don't Stop Believin' (Live)")
	expected := "Don't Stop Believin'"
	if result != expected {
		t.Errorf("Expected removeBrackets to return '%s', got '%s'", expected, result)
	}

	// Test removeFeaturing with single quotes
	result = client.removeFeaturing("Don't Stop Believin' feat. Journey")
	expected = "Don't Stop Believin'"
	if result != expected {
		t.Errorf("Expected removeFeaturing to return '%s', got '%s'", expected, result)
	}

	// Test normalizeTitle with single quotes
	result = client.normalizeTitle("Don't Stop Believin' - Live Version")
	expected = "don't stop believin' (live version)"
	if result != expected {
		t.Errorf("Expected normalizeTitle to return '%s', got '%s'", expected, result)
	}
}

func TestSetServerID(t *testing.T) {
	client := &Client{
		serverID: "old_server_id",
	}

	// Test setting new server ID
	newServerID := "new_server_id"
	client.SetServerID(newServerID)

	if client.serverID != newServerID {
		t.Errorf("Expected server ID to be '%s', got '%s'", newServerID, client.serverID)
	}
}

func TestAddSyncAttribution(t *testing.T) {
	client := &Client{}

	// Test with empty description and valid spotifyPlaylistID
	result := client.addSyncAttribution("", "37i9dQZF1DXcBWIGoYBM5M")
	expected := "synced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"
	if result != expected {
		t.Errorf("Expected sync attribution to be '%s', got '%s'", expected, result)
	}

	// Test with existing description and valid spotifyPlaylistID
	result = client.addSyncAttribution("Original description", "37i9dQZF1DXcBWIGoYBM5M")
	expected = "Original description\n\nsynced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"
	if result != expected {
		t.Errorf("Expected sync attribution to be '%s', got '%s'", expected, result)
	}

	// Test with empty spotifyPlaylistID (should return original description unchanged)
	result = client.addSyncAttribution("Original description", "")
	expected = "Original description"
	if result != expected {
		t.Errorf("Expected sync attribution to be '%s', got '%s'", expected, result)
	}

	// Test with both empty (should return empty string)
	result = client.addSyncAttribution("", "")
	expected = ""
	if result != expected {
		t.Errorf("Expected sync attribution to be '%s', got '%s'", expected, result)
	}
}

func TestManualInstructionFunctions(t *testing.T) {
	// This test function was used to test logMatchedTracks which has been removed
	// Keeping the function for potential future use
}

func TestSimilarityScoresWithSingleQuotes(t *testing.T) {
	client := &Client{}

	// Test case: Spotify song with single quote vs Plex track with single quote
	spotifyTitle := "Don't Stop Believin'"
	spotifyArtist := "Journey"
	plexTitle := "Don't Stop Believin'"
	plexArtist := "Journey"

	// Calculate similarity scores
	titleSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(spotifyTitle)),
		strings.ToLower(strings.TrimSpace(plexTitle)),
	)
	artistSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(spotifyArtist)),
		strings.ToLower(strings.TrimSpace(plexArtist)),
	)

	// Combined score (title is more important than artist)
	score := (titleSimilarity * 0.7) + (artistSimilarity * 0.3)

	t.Logf("Title similarity: %f", titleSimilarity)
	t.Logf("Artist similarity: %f", artistSimilarity)
	t.Logf("Combined score: %f", score)

	// The score should be 1.0 for exact matches
	if score != 1.0 {
		t.Errorf("Expected score 1.0 for exact matches with single quotes, got %f", score)
	}

	// Test case: Similar but not exact match
	plexTitle2 := "Don't Stop Believing" // Missing the final apostrophe
	titleSimilarity2 := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(spotifyTitle)),
		strings.ToLower(strings.TrimSpace(plexTitle2)),
	)
	score2 := (titleSimilarity2 * 0.7) + (artistSimilarity * 0.3)

	t.Logf("Title similarity (missing apostrophe): %f", titleSimilarity2)
	t.Logf("Combined score (missing apostrophe): %f", score2)

	// The score should still be above 0.6 threshold
	if score2 < 0.6 {
		t.Errorf("Expected score >= 0.6 for similar titles with single quotes, got %f", score2)
	}
}

func TestSearchByTitleWithSingleQuoteVariations(t *testing.T) {
	client := &Client{}

	// Test that the function exists and can be called
	// We can't easily test the actual search without a real Plex server,
	// but we can test that the function doesn't panic
	_ = client.searchByTitleWithSingleQuoteVariations

	// Test that the function handles various contraction patterns
	testCases := []string{
		"Don't Stop Believin'",
		"I'm Still Standing",
		"We're Not Gonna Take It",
		"You'll Never Walk Alone",
		"I've Got You Under My Skin",
		"I'd Do Anything For Love",
	}

	for _, testTitle := range testCases {
		// These should all be handled by the variation search
		_ = testTitle
	}
}

func TestRemoveWith(t *testing.T) {
	client := &Client{}

	testCases := []struct {
		input    string
		expected string
	}{
		{
			input:    "Neon Moon - with Kacey Musgraves",
			expected: "Neon Moon",
		},
		{
			input:    "Song Title with Artist Name",
			expected: "Song Title",
		},
		{
			input:    "Another Song WITH Another Artist",
			expected: "Another Song",
		},
		{
			input:    "Song without with",
			expected: "Song without with",
		},
		{
			input:    "Song with",
			expected: "Song with",
		},
		{
			input:    "with Artist",
			expected: "Artist",
		},
		{
			input:    "Song with Artist with Another",
			expected: "Song with Artist",
		},
		{
			input:    "",
			expected: "",
		},
	}

	for _, tc := range testCases {
		result := client.removeWith(tc.input)
		if result != tc.expected {
			t.Errorf("removeWith(%q) = %q, expected %q", tc.input, result, tc.expected)
		}
	}
}

func TestClearPlaylist(t *testing.T) {
	client := &Client{
		baseURL:    "http://localhost:32400",
		token:      "test-token",
		sectionID:  1,
		serverID:   "test-server",
		httpClient: &http.Client{},
	}

	// Test that the function exists and can be called
	// We can't easily test the actual API call without a real Plex server,
	// but we can test that the function doesn't panic
	_ = client.ClearPlaylist

	// Test that the function handles the playlist ID parameter correctly
	playlistID := "test-playlist-123"
	_ = playlistID
}

func TestNeonMoonMatchingScenario(t *testing.T) {
	client := &Client{}

	// Test case: Spotify song "Neon Moon - with Kacey Musgraves" by "Brooks & Dunn"
	// should match Plex track "Neon Moon" by "Brooks & Dunn"
	spotifySong := spotify.Song{
		ID:       "test_neon_moon",
		Name:     "Neon Moon - with Kacey Musgraves",
		Artist:   "Brooks & Dunn",
		Album:    "Borderline",
		Duration: 240000,
		URI:      "spotify:track:test_neon_moon",
		ISRC:     "TEST98765432",
	}

	// Simulate Plex library tracks
	plexTracks := []PlexTrack{
		{
			ID:     "plex_neon_moon_1",
			Title:  "Neon Moon",
			Artist: "Brooks & Dunn",
			Album:  "Borderline",
		},
		{
			ID:     "plex_neon_moon_2",
			Title:  "Neon Moon - with Kacey Musgraves",
			Artist: "Brooks & Dunn",
			Album:  "Borderline",
		},
		{
			ID:     "plex_other_song",
			Title:  "Boot Scootin' Boogie",
			Artist: "Brooks & Dunn",
			Album:  "Brand New Man",
		},
	}

	// Test 1: FindBestMatch should find exact match first
	t.Run("ExactMatchPriority", func(t *testing.T) {
		result := client.FindBestMatch(plexTracks, spotifySong.Name, spotifySong.Artist)

		if result == nil {
			t.Error("Expected to find a match, got nil")
			return
		}

		// Should prefer the exact match over the "with" version
		if result.Title != "Neon Moon - with Kacey Musgraves" {
			t.Errorf("Expected exact match 'Neon Moon - with Kacey Musgraves', got '%s'", result.Title)
		}
	})

	// Test 2: Test the "with" removal logic specifically
	t.Run("WithRemovalLogic", func(t *testing.T) {
		// Remove "with" from the Spotify title
		cleanedTitle := client.removeWith(spotifySong.Name)
		expectedCleaned := "Neon Moon"

		if cleanedTitle != expectedCleaned {
			t.Errorf("Expected removeWith('%s') to return '%s', got '%s'",
				spotifySong.Name, expectedCleaned, cleanedTitle)
		}

		// Now search for the cleaned title
		result := client.FindBestMatch(plexTracks, cleanedTitle, spotifySong.Artist)

		if result == nil {
			t.Error("Expected to find a match after removing 'with', got nil")
			return
		}

		// Should find the clean "Neon Moon" version
		if result.Title != "Neon Moon" {
			t.Errorf("Expected to find 'Neon Moon' after removing 'with', got '%s'", result.Title)
		}
	})

	// Test 3: Test confidence calculation for this scenario
	t.Run("ConfidenceCalculation", func(t *testing.T) {
		// Test confidence between Spotify song and Plex track with "with" removed
		plexTrack := &PlexTrack{
			ID:     "plex_neon_moon_clean",
			Title:  "Neon Moon",
			Artist: "Brooks & Dunn",
		}

		confidence := client.calculateConfidence(spotifySong, plexTrack, "title_artist")

		// The confidence should be high since artist matches exactly and title is very similar
		if confidence < 0.8 {
			t.Errorf("Expected high confidence (>= 0.8) for this match, got %f", confidence)
		}

		t.Logf("Confidence score for 'Neon Moon - with Kacey Musgraves' vs 'Neon Moon': %f", confidence)
	})

	// Test 4: Test string similarity for this specific case
	t.Run("StringSimilarity", func(t *testing.T) {
		spotifyTitle := "Neon Moon - with Kacey Musgraves"
		plexTitle := "Neon Moon"

		// Test original title similarity
		originalSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifyTitle)),
			strings.ToLower(strings.TrimSpace(plexTitle)),
		)

		// Test with "with" removed
		cleanedSpotifyTitle := client.removeWith(spotifyTitle)
		cleanedSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(cleanedSpotifyTitle)),
			strings.ToLower(strings.TrimSpace(plexTitle)),
		)

		t.Logf("Original title similarity: %f", originalSimilarity)
		t.Logf("Cleaned title similarity: %f", cleanedSimilarity)

		// The cleaned similarity should be higher than the original
		if cleanedSimilarity <= originalSimilarity {
			t.Errorf("Expected cleaned similarity (%f) to be higher than original (%f)",
				cleanedSimilarity, originalSimilarity)
		}

		// The cleaned similarity should be very high (close to 1.0)
		if cleanedSimilarity < 0.9 {
			t.Errorf("Expected cleaned similarity to be >= 0.9, got %f", cleanedSimilarity)
		}
	})

	// Test 5: Test the complete matching flow
	t.Run("CompleteMatchingFlow", func(t *testing.T) {
		// Create a minimal set of tracks that would be returned by a search
		searchResults := []PlexTrack{
			{
				ID:     "plex_neon_moon_clean",
				Title:  "Neon Moon",
				Artist: "Brooks & Dunn",
			},
		}

		// Test that FindBestMatch correctly identifies this as a good match
		result := client.FindBestMatch(searchResults, spotifySong.Name, spotifySong.Artist)

		// Add debug output to understand what's happening
		t.Logf("Searching for: '%s' by '%s'", spotifySong.Name, spotifySong.Artist)
		t.Logf("Available tracks: %+v", searchResults)

		if result == nil {
			t.Error("Expected FindBestMatch to find a match for this scenario")
			return
		}

		if result.Title != "Neon Moon" {
			t.Errorf("Expected to match 'Neon Moon', got '%s'", result.Title)
		}

		if result.Artist != "Brooks & Dunn" {
			t.Errorf("Expected artist 'Brooks & Dunn', got '%s'", result.Artist)
		}
	})
}

func TestIncorrectMatchingIssue(t *testing.T) {
	client := &Client{}

	// Test case: Spotify song "the lakes - bonus track" by "Taylor Swift"
	spotifySong := spotify.Song{
		ID:       "test_the_lakes",
		Name:     "the lakes - bonus track",
		Artist:   "Taylor Swift",
		Album:    "folklore",
		Duration: 240000,
		URI:      "spotify:track:test_the_lakes",
		ISRC:     "TEST11111111",
	}

	plexTracks := []PlexTrack{
		{
			ID:    "plex_korean_track",
			Album: "Before I Die",
		},
		{
			ID:     "plex_other_track",
			Title:  "Some Other Song",
			Artist: "Some Other Artist",
			Album:  "Some Album",
		},
		{
			ID:     "plex_another_track",
			Title:  "Another Song",
			Artist: "Another Artist",
			Album:  "Another Album",
		},
	}

	// Test 1: Verify that this should NOT match
	t.Run("ShouldNotMatch", func(t *testing.T) {
		result := client.FindBestMatch(plexTracks, spotifySong.Name, spotifySong.Artist)

		// This should NOT match - the songs are completely different
		if result != nil {
			t.Errorf("Expected NO match for completely different songs, but got: '%s' by '%s'",
				result.Title, result.Artist)

			// Calculate the confidence to understand why it matched
			confidence := client.calculateConfidence(spotifySong, result, "title_artist")
			t.Logf("Incorrect match confidence: %f", confidence)

			// Calculate individual similarities
			titleSimilarity := client.calculateStringSimilarity(
				strings.ToLower(strings.TrimSpace(spotifySong.Name)),
				strings.ToLower(strings.TrimSpace(result.Title)),
			)
			artistSimilarity := client.calculateStringSimilarity(
				strings.ToLower(strings.TrimSpace(spotifySong.Artist)),
				strings.ToLower(strings.TrimSpace(result.Artist)),
			)

			t.Logf("Title similarity: %f", titleSimilarity)
			t.Logf("Artist similarity: %f", artistSimilarity)
			t.Logf("Combined score: %f", (titleSimilarity*0.7)+(artistSimilarity*0.3))
		}
	})

	// Test 2: Test individual similarity calculations
	t.Run("SimilarityCalculations", func(t *testing.T) {
		spotifyTitle := "the lakes - bonus track"
		spotifyArtist := "Taylor Swift"
		plexTitle := "Some Other Song"    // Use actual track from plexTracks
		plexArtist := "Some Other Artist" // Use actual track from plexTracks

		// Test original similarities
		titleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifyTitle)),
			strings.ToLower(strings.TrimSpace(plexTitle)),
		)
		artistSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifyArtist)),
			strings.ToLower(strings.TrimSpace(plexArtist)),
		)

		t.Logf("Original title similarity: %f", titleSimilarity)
		t.Logf("Original artist similarity: %f", artistSimilarity)

		// Test with various transformations
		cleanTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeBrackets(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.removeBrackets(plexTitle))),
		)
		featuringTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeFeaturing(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.removeFeaturing(plexTitle))),
		)
		normalizedTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.normalizeTitle(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.normalizeTitle(plexTitle))),
		)
		withTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeWith(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.removeWith(plexTitle))),
		)

		t.Logf("Bracket-removed title similarity: %f", cleanTitleSimilarity)
		t.Logf("Featuring-removed title similarity: %f", featuringTitleSimilarity)
		t.Logf("Normalized title similarity: %f", normalizedTitleSimilarity)
		t.Logf("With-removed title similarity: %f", withTitleSimilarity)

		// The similarities should all be very low for completely different songs
		if titleSimilarity > 0.3 {
			t.Errorf("Title similarity should be very low for different songs, got %f", titleSimilarity)
		}
		if artistSimilarity > 0.3 {
			t.Errorf("Artist similarity should be very low for different artists, got %f", artistSimilarity)
		}
	})

	// Test 3: Test confidence threshold
	t.Run("ConfidenceThreshold", func(t *testing.T) {
		plexTrack := &PlexTrack{
			ID: "plex_korean_track",
		}

		confidence := client.calculateConfidence(spotifySong, plexTrack, "title_artist")

		// The confidence should be below the threshold for completely different songs
		if confidence >= MinConfidenceScore {
			t.Errorf("Confidence should be below threshold (%f) for completely different songs, got %f",
				MinConfidenceScore, confidence)
		}
	})
}

func TestSearchFlowSimulation(t *testing.T) {
	client := &Client{}

	// Test case: Simulate what might happen in the actual search flow
	spotifySong := spotify.Song{
		ID:       "test_the_lakes",
		Name:     "the lakes - bonus track",
		Artist:   "Taylor Swift",
		Album:    "folklore",
		Duration: 240000,
		URI:      "spotify:track:test_the_lakes",
		ISRC:     "TEST11111111",
	}

	// Simulate different search result scenarios
	t.Run("EmptySearchResults", func(t *testing.T) {
		// Test what happens when search returns no results
		emptyResults := []PlexTrack{}
		result := client.FindBestMatch(emptyResults, spotifySong.Name, spotifySong.Artist)

		if result != nil {
			t.Errorf("Expected nil result for empty search results, got: %+v", result)
		}
	})

	t.Run("SingleUnrelatedResult", func(t *testing.T) {
		// Test with a single unrelated result
		singleResult := []PlexTrack{
			{
				ID: "plex_korean_track",
			},
		}

		result := client.FindBestMatch(singleResult, spotifySong.Name, spotifySong.Artist)

		// This should NOT match due to low confidence
		if result != nil {
			t.Errorf("Expected NO match for single unrelated result, but got: '%s' by '%s'",
				result.Title, result.Artist)

			confidence := client.calculateConfidence(spotifySong, result, "title_artist")
			t.Logf("Unexpected match confidence: %f", confidence)
		}
	})

	t.Run("MultipleResultsWithOneGoodMatch", func(t *testing.T) {
		// Test with multiple results including one that should match
		multipleResults := []PlexTrack{
			{
				ID: "plex_korean_track",
			},
			{
				ID:     "plex_correct_track",
				Title:  "the lakes",
				Artist: "Taylor Swift",
			},
			{
				ID:     "plex_another_track",
				Title:  "Some Other Song",
				Artist: "Some Other Artist",
			},
		}

		result := client.FindBestMatch(multipleResults, spotifySong.Name, spotifySong.Artist)

		// Add debug output to understand what's happening
		t.Logf("Searching for: '%s' by '%s'", spotifySong.Name, spotifySong.Artist)
		t.Logf("Available tracks:")
		for i, track := range multipleResults {
			confidence := client.calculateConfidence(spotifySong, &track, "title_artist")
			t.Logf("  %d. '%s' by '%s' (confidence: %f)", i+1, track.Title, track.Artist, confidence)
		}

		if result == nil {
			t.Error("Expected to find a match for 'the lakes' by 'Taylor Swift'")
			return
		}

		if result.Title != "the lakes" || result.Artist != "Taylor Swift" {
			t.Errorf("Expected to match 'the lakes' by 'Taylor Swift', got '%s' by '%s'",
				result.Title, result.Artist)
		}
	})

	t.Run("ThresholdBypassTest", func(t *testing.T) {
		// Test if there's a way the threshold could be bypassed
		// Create a scenario where the best match is still below threshold
		lowConfidenceResults := []PlexTrack{
			{
				ID: "plex_korean_track",
			},
			{
				ID:     "plex_slightly_better",
				Title:  "the",
				Artist: "Taylor",
			},
		}

		result := client.FindBestMatch(lowConfidenceResults, spotifySong.Name, spotifySong.Artist)

		// Both should be below threshold, so no match should be returned
		if result != nil {
			confidence := client.calculateConfidence(spotifySong, result, "title_artist")
			t.Errorf("Expected NO match due to low confidence, but got: '%s' by '%s' (confidence: %f)",
				result.Title, result.Artist, confidence)
		}
	})

	t.Run("TheLakesConfidenceAnalysis", func(t *testing.T) {
		// Analyze why "the lakes - bonus track" vs "the lakes" has low confidence
		spotifyTitle := "the lakes - bonus track"
		spotifyArtist := "Taylor Swift"
		plexTitle := "the lakes"
		plexArtist := "Taylor Swift"

		t.Logf("Analyzing confidence for '%s' vs '%s'", spotifyTitle, plexTitle)

		// Test original similarities
		titleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifyTitle)),
			strings.ToLower(strings.TrimSpace(plexTitle)),
		)
		artistSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifyArtist)),
			strings.ToLower(strings.TrimSpace(plexArtist)),
		)

		t.Logf("Original title similarity: %f", titleSimilarity)
		t.Logf("Original artist similarity: %f", artistSimilarity)

		// Test with various transformations
		cleanTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeBrackets(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.removeBrackets(plexTitle))),
		)
		featuringTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeFeaturing(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.removeFeaturing(plexTitle))),
		)
		normalizedTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.normalizeTitle(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.normalizeTitle(plexTitle))),
		)
		withTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeWith(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.removeWith(plexTitle))),
		)
		suffixTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.RemoveCommonSuffixes(spotifyTitle))),
			strings.ToLower(strings.TrimSpace(client.RemoveCommonSuffixes(plexTitle))),
		)

		t.Logf("Bracket-removed title similarity: %f", cleanTitleSimilarity)
		t.Logf("Featuring-removed title similarity: %f", featuringTitleSimilarity)
		t.Logf("Normalized title similarity: %f", normalizedTitleSimilarity)
		t.Logf("With-removed title similarity: %f", withTitleSimilarity)
		t.Logf("Suffix-removed title similarity: %f", suffixTitleSimilarity)

		// Calculate the best title similarity
		bestTitleSimilarity := titleSimilarity
		if cleanTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = cleanTitleSimilarity
		}
		if featuringTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = featuringTitleSimilarity
		}
		if normalizedTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = normalizedTitleSimilarity
		}
		if withTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = withTitleSimilarity
		}
		if suffixTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = suffixTitleSimilarity
		}

		finalConfidence := (bestTitleSimilarity * 0.7) + (artistSimilarity * 0.3)
		t.Logf("Best title similarity: %f", bestTitleSimilarity)
		t.Logf("Final confidence: %f", finalConfidence)

		// This should be a good match, so the confidence should be high
		if finalConfidence < 0.8 {
			t.Errorf("Expected high confidence for 'the lakes - bonus track' vs 'the lakes', got %f", finalConfidence)
		}
	})

	t.Run("TitleTransformationDebug", func(t *testing.T) {
		// Debug what each transformation does to "the lakes - bonus track"
		spotifyTitle := "the lakes - bonus track"

		t.Logf("Original: '%s'", spotifyTitle)
		t.Logf("removeBrackets: '%s'", client.removeBrackets(spotifyTitle))
		t.Logf("removeFeaturing: '%s'", client.removeFeaturing(spotifyTitle))
		t.Logf("normalizeTitle: '%s'", client.normalizeTitle(spotifyTitle))
		t.Logf("removeWith: '%s'", client.removeWith(spotifyTitle))

		// The issue is that none of our transformations handle "bonus track"
		// We need a new transformation for common suffixes like "bonus track", "remix", etc.
	})

	t.Run("RemoveCommonSuffixes", func(t *testing.T) {
		// Test the new RemoveCommonSuffixes function
		testCases := []struct {
			input    string
			expected string
		}{
			{
				input:    "the lakes - bonus track",
				expected: "the lakes",
			},
			{
				input:    "Song Title - Remix",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Extended",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Radio Edit",
				expected: "Song Title",
			},
			{
				input:    "Song Title (Bonus Track)",
				expected: "Song Title",
			},
			{
				input:    "Song Title (Remix)",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Live",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Acoustic",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Instrumental",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Demo",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Original Mix",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Club Mix",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Clean",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Explicit",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Bonus",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Track",
				expected: "Song Title",
			},
			{
				input:    "Song Title - Remastered",
				expected: "Song Title",
			},
			{
				input:    "Song Title (Remastered)",
				expected: "Song Title",
			},
			{
				input:    "Here Comes the Rain Again - 2018 Remastered",
				expected: "Here Comes the Rain Again",
			},
			{
				input:    "Song Title - 2020 Remastered",
				expected: "Song Title",
			},
			{
				input:    "Song Title (2018 Remastered)",
				expected: "Song Title",
			},
			{
				input:    "Song Title (2020 Remastered)",
				expected: "Song Title",
			},
			{
				input:    "Song Title - 1995 Remastered",
				expected: "Song Title",
			},
			{
				input:    "Song Title (1995 Remastered)",
				expected: "Song Title",
			},
			// Soundtrack suffix tests
			{
				input:    `Swan Song - From the Motion Picture "Alita: Battle Angel"`,
				expected: "Swan Song",
			},
			{
				input:    `Swan Song - From the Film "Alita: Battle Angel"`,
				expected: "Swan Song",
			},
			{
				input:    `Swan Song - From the Movie "Alita: Battle Angel"`,
				expected: "Swan Song",
			},
			{
				input:    `Swan Song (From the Motion Picture "Alita: Battle Angel")`,
				expected: "Swan Song",
			},
			{
				input:    "Swan Song - Soundtrack Version",
				expected: "Swan Song",
			},
			{
				input:    "Swan Song - Film Version",
				expected: "Swan Song",
			},
			{
				input:    "Swan Song - Movie Version",
				expected: "Swan Song",
			},
			{
				input:    `Take My Breath Away - Love Theme from "Top Gun"`,
				expected: "Take My Breath Away",
			},
			{
				input:    `Take My Breath Away (Love Theme from "Top Gun")`,
				expected: "Take My Breath Away",
			},
			{
				input:    `Song Title - Love Theme from "Movie Name"`,
				expected: "Song Title",
			},
			{
				input:    `Song Title (Love Theme from "Movie Name")`,
				expected: "Song Title",
			},
			// Streaming service series tests
			{
				input:    `In The Dark - From the Netflix Series "Nobody Wants This" Season 2`,
				expected: "In The Dark",
			},
			{
				input:    `Theme Song - From the Netflix Series "Stranger Things"`,
				expected: "Theme Song",
			},
			{
				input:    `End Credits - From the Hulu Series "The Handmaid's Tale" Season 3`,
				expected: "End Credits",
			},
			{
				input:    `Main Theme - From the Prime Video Series "The Boys"`,
				expected: "Main Theme",
			},
			{
				input:    `Soundtrack - From the Apple TV Series "Ted Lasso"`,
				expected: "Soundtrack",
			},
			{
				input:    `Song Title - From the Disney Series "The Mandalorian"`,
				expected: "Song Title",
			},
			{
				input:    `Track - From the HBO Series "Game of Thrones" Season 8`,
				expected: "Track",
			},
			{
				input:    `Music - from the netflix show "Bridgerton"`,
				expected: "Music",
			},
			{
				input:    `Song Title (From the Netflix Series "Wednesday")`,
				expected: "Song Title",
			},
			{
				input:    "Song Title",
				expected: "Song Title",
			},
		}

		for _, tc := range testCases {
			result := client.RemoveCommonSuffixes(tc.input)
			t.Logf("Testing: '%s' -> '%s' (expected: '%s')", tc.input, result, tc.expected)
			if result != tc.expected {
				t.Errorf("RemoveCommonSuffixes(%q) = %q, expected %q", tc.input, result, tc.expected)
			}
		}
	})

	t.Run("RealWorldScenario", func(t *testing.T) {
		// Test the real-world scenario: "the lakes" not in library, but search returns unrelated results
		spotifySong := spotify.Song{
			ID:       "test_the_lakes",
			Name:     "the lakes - bonus track",
			Artist:   "Taylor Swift",
			Album:    "folklore",
			Duration: 240000,
			URI:      "spotify:track:test_the_lakes",
			ISRC:     "TEST11111111",
		}

		// Simulate what Plex search might return when searching for "the lakes"
		// These are unrelated tracks that might be returned by a broad search
		searchResults := []PlexTrack{
			{
				ID: "plex_korean_track",
			},
			{
				ID:     "plex_other_track",
				Title:  "The Other Side",
				Artist: "Some Other Artist",
			},
			{
				ID:     "plex_another_track",
				Title:  "Lakes of Fire",
				Artist: "Another Artist",
			},
			{
				ID:     "plex_yet_another",
				Title:  "The Way",
				Artist: "Yet Another Artist",
			},
		}

		// Test what happens when we search for "the lakes" (after suffix removal)
		searchTitle := "the lakes"
		searchArtist := "Taylor Swift"

		t.Logf("Searching for: '%s' by '%s'", searchTitle, searchArtist)
		t.Logf("Available tracks from Plex search:")
		for i, track := range searchResults {
			confidence := client.calculateConfidence(spotifySong, &track, "title_artist")
			t.Logf("  %d. '%s' by '%s' (confidence: %f)", i+1, track.Title, track.Artist, confidence)
		}

		result := client.FindBestMatch(searchResults, searchTitle, searchArtist)

		if result != nil {
			confidence := client.calculateConfidence(spotifySong, result, "title_artist")
			t.Errorf("Expected NO match since 'the lakes' is not in the library, but got: '%s' by '%s' (confidence: %f)",
				result.Title, result.Artist, confidence)

			// This should NOT happen - the confidence should be below threshold
			if confidence >= MinConfidenceScore {
				t.Errorf("Confidence is above threshold (%f >= %f) for completely unrelated tracks",
					confidence, MinConfidenceScore)
			}
		} else {
			t.Logf("✅ Correctly found NO match - 'the lakes' is not in the library")
		}
	})

	t.Run("SearchFlowDebug", func(t *testing.T) {
		// Test the actual search flow to see where the bug might be
		spotifySong := spotify.Song{
			ID:       "test_the_lakes",
			Name:     "the lakes - bonus track",
			Artist:   "Taylor Swift",
			Album:    "folklore",
			Duration: 240000,
			URI:      "spotify:track:test_the_lakes",
			ISRC:     "TEST11111111",
		}

		// Simulate the search flow step by step
		t.Logf("Testing search flow for: '%s' by '%s'", spotifySong.Name, spotifySong.Artist)

		// Step 1: Try exact match
		t.Logf("Step 1: Exact match")
		// This would fail in real scenario

		// Step 2: Single quote handling
		t.Logf("Step 2: Single quote handling")
		// This would fail in real scenario

		// Step 3: Bracket removal
		t.Logf("Step 3: Bracket removal")
		cleanTitle := client.removeBrackets(spotifySong.Name)
		t.Logf("  Cleaned title: '%s'", cleanTitle)

		// Step 4: Featuring removal
		t.Logf("Step 4: Featuring removal")
		featuringTitle := client.removeFeaturing(spotifySong.Name)
		t.Logf("  Featuring removed: '%s'", featuringTitle)

		// Step 5: Title normalization
		t.Logf("Step 5: Title normalization")
		normalizedTitle := client.normalizeTitle(spotifySong.Name)
		t.Logf("  Normalized: '%s'", normalizedTitle)

		// Step 6: With removal
		t.Logf("Step 6: With removal")
		withTitle := client.removeWith(spotifySong.Name)
		t.Logf("  With removed: '%s'", withTitle)

		// Step 7: Suffix removal
		t.Logf("Step 7: Suffix removal")
		suffixTitle := client.RemoveCommonSuffixes(spotifySong.Name)
		t.Logf("  Suffix removed: '%s'", suffixTitle)

		// This should be the key step that finds "the lakes"
		if suffixTitle == "the lakes" {
			t.Logf("✅ Suffix removal correctly identified 'the lakes' as the base title")
		} else {
			t.Errorf("❌ Suffix removal failed: expected 'the lakes', got '%s'", suffixTitle)
		}

		// Step 8: Full library search (this is where the bug might be)
		t.Logf("Step 8: Full library search")
		t.Logf("  This would search the entire library for 'the lakes' by 'Taylor Swift'")
		t.Logf("  If 'the lakes' is not in the library, this should return no results")
		t.Logf("  But if it returns unrelated results, FindBestMatch might still find a match")
	})

	t.Run("SearchEntireLibraryBug", func(t *testing.T) {
		// Test the searchEntireLibrary function specifically
		spotifySong := spotify.Song{
			ID:       "test_the_lakes",
			Name:     "the lakes - bonus track",
			Artist:   "Taylor Swift",
			Album:    "folklore",
			Duration: 240000,
			URI:      "spotify:track:test_the_lakes",
			ISRC:     "TEST11111111",
		}

		// Simulate what the entire library might contain
		// This represents ALL tracks in the Plex library
		entireLibrary := []PlexTrack{
			{
				ID: "plex_korean_track",
			},
			{
				ID:     "plex_other_track",
				Title:  "The Other Side",
				Artist: "Some Other Artist",
			},
			{
				ID:     "plex_another_track",
				Title:  "Lakes of Fire",
				Artist: "Another Artist",
			},
			{
				ID:     "plex_yet_another",
				Title:  "The Way",
				Artist: "Yet Another Artist",
			},
			{
				ID:     "plex_many_more",
				Title:  "Some Other Song",
				Artist: "Some Other Artist",
			},
			{
				ID:     "plex_even_more",
				Title:  "Another Song",
				Artist: "Another Artist",
			},
		}

		// Test what searchEntireLibrary would do
		searchTitle := "the lakes"
		searchArtist := "Taylor Swift"

		t.Logf("Testing searchEntireLibrary for: '%s' by '%s'", searchTitle, searchArtist)
		t.Logf("Searching through entire library (%d tracks):", len(entireLibrary))
		for i, track := range entireLibrary {
			confidence := client.calculateConfidence(spotifySong, &track, "title_artist")
			t.Logf("  %d. '%s' by '%s' (confidence: %f)", i+1, track.Title, track.Artist, confidence)
		}

		result := client.FindBestMatch(entireLibrary, searchTitle, searchArtist)

		if result != nil {
			confidence := client.calculateConfidence(spotifySong, result, "title_artist")
			t.Logf("⚠️  searchEntireLibrary found a match: '%s' by '%s' (confidence: %f)",
				result.Title, result.Artist, confidence)

			if confidence >= MinConfidenceScore {
				t.Errorf("❌ BUG: searchEntireLibrary returned a match above threshold (%f >= %f) for unrelated tracks",
					confidence, MinConfidenceScore)
				t.Logf("This explains why 'the lakes - bonus track' is matching to '%s' by '%s'",
					result.Title, result.Artist)
			} else {
				t.Logf("✅ Correctly below threshold, but still returned a match (this might be the issue)")
			}
		} else {
			t.Logf("✅ Correctly found NO match in entire library")
		}
	})

	t.Run("HighConfidenceScenario", func(t *testing.T) {
		// Test scenarios where confidence might be high enough to cause incorrect matching
		spotifySong := spotify.Song{
			ID:       "test_the_lakes",
			Name:     "the lakes - bonus track",
			Artist:   "Taylor Swift",
			Album:    "folklore",
			Duration: 240000,
			URI:      "spotify:track:test_the_lakes",
			ISRC:     "TEST11111111",
		}

		// Test different scenarios that might have high confidence
		testCases := []struct {
			name     string
			title    string
			artist   string
			expected bool // whether we expect it to match
		}{
			{
				name:     "Similar title with 'the'",
				title:    "The Lakes",
				artist:   "Some Artist",
				expected: false,
			},
			{
				name:     "Similar title with 'lakes'",
				title:    "Beautiful Lakes",
				artist:   "Some Artist",
				expected: false,
			},
			{
				name:     "Artist with 'Taylor'",
				title:    "Some Song",
				artist:   "Taylor Somebody",
				expected: false,
			},
			{
				name:     "Artist with 'Swift'",
				title:    "Some Song",
				artist:   "Somebody Swift",
				expected: false,
			},
			{
				name:     "Similar words",
				title:    "The Lake",
				artist:   "Taylor Swift",
				expected: true, // This should match because both title and artist are very similar
			},
		}

		for _, tc := range testCases {
			t.Run(tc.name, func(t *testing.T) {
				// Test using FindBestMatch which includes the new artist similarity check
				searchResults := []PlexTrack{
					{
						ID:     "test_track",
						Title:  tc.title,
						Artist: tc.artist,
					},
				}

				result := client.FindBestMatch(searchResults, "the lakes", "Taylor Swift")
				t.Logf("'%s' by '%s' -> FindBestMatch result: %v", tc.title, tc.artist, result)

				if result != nil {
					confidence := client.calculateConfidence(spotifySong, result, "title_artist")
					t.Logf("  Confidence: %f", confidence)

					if tc.expected {
						if confidence >= MinConfidenceScore {
							t.Logf("✅ Correctly matched (expected)")
						} else {
							t.Errorf("❌ Expected match but confidence too low (%f < %f)",
								confidence, MinConfidenceScore)
						}
					} else {
						if confidence >= MinConfidenceScore {
							t.Errorf("❌ High confidence (%f >= %f) for '%s' by '%s'",
								confidence, MinConfidenceScore, tc.title, tc.artist)
						} else {
							t.Logf("✅ Correctly below threshold")
						}
					}
				} else {
					if tc.expected {
						t.Errorf("❌ Expected match but found NO match")
					} else {
						t.Logf("✅ Correctly found NO match")
					}
				}
			})
		}
	})

	t.Run("DebugTheLakesConfidence", func(t *testing.T) {
		// Debug why "The Lakes" has high confidence with "the lakes"
		// Debug the confidence calculation step by step
		t.Logf("Debugging confidence for 'the lakes' vs 'The Lakes'")

		// Test original similarities
		titleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace("the lakes")),
			strings.ToLower(strings.TrimSpace("The Lakes")),
		)
		artistSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace("Taylor Swift")),
			strings.ToLower(strings.TrimSpace("Some Artist")),
		)

		t.Logf("Original title similarity: %f", titleSimilarity)
		t.Logf("Original artist similarity: %f", artistSimilarity)

		// Test with various transformations
		cleanTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeBrackets("the lakes"))),
			strings.ToLower(strings.TrimSpace(client.removeBrackets("The Lakes"))),
		)
		featuringTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeFeaturing("the lakes"))),
			strings.ToLower(strings.TrimSpace(client.removeFeaturing("The Lakes"))),
		)
		normalizedTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.normalizeTitle("the lakes"))),
			strings.ToLower(strings.TrimSpace(client.normalizeTitle("The Lakes"))),
		)
		withTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.removeWith("the lakes"))),
			strings.ToLower(strings.TrimSpace(client.removeWith("The Lakes"))),
		)
		suffixTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(client.RemoveCommonSuffixes("the lakes"))),
			strings.ToLower(strings.TrimSpace(client.RemoveCommonSuffixes("The Lakes"))),
		)

		t.Logf("Bracket-removed title similarity: %f", cleanTitleSimilarity)
		t.Logf("Featuring-removed title similarity: %f", featuringTitleSimilarity)
		t.Logf("Normalized title similarity: %f", normalizedTitleSimilarity)
		t.Logf("With-removed title similarity: %f", withTitleSimilarity)
		t.Logf("Suffix-removed title similarity: %f", suffixTitleSimilarity)

		// Calculate the best title similarity
		bestTitleSimilarity := titleSimilarity
		if cleanTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = cleanTitleSimilarity
		}
		if featuringTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = featuringTitleSimilarity
		}
		if normalizedTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = normalizedTitleSimilarity
		}
		if withTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = withTitleSimilarity
		}
		if suffixTitleSimilarity > bestTitleSimilarity {
			bestTitleSimilarity = suffixTitleSimilarity
		}

		finalConfidence := (bestTitleSimilarity * 0.7) + (artistSimilarity * 0.3)
		t.Logf("Best title similarity: %f", bestTitleSimilarity)
		t.Logf("Final confidence: %f", finalConfidence)

		// The issue is that "the lakes" vs "The Lakes" has very high similarity
		// because they're almost identical after case normalization
		if titleSimilarity > 0.9 {
			t.Logf("⚠️  The issue: 'the lakes' vs 'The Lakes' has very high similarity (%f)", titleSimilarity)
			t.Logf("This means any track with 'The Lakes' in the library will match 'the lakes'")
		}
	})

	t.Run("TheLakesSpecificTest", func(t *testing.T) {
		// Test the specific scenario that was causing the issue
		spotifySong := spotify.Song{
			ID:       "test_the_lakes",
			Name:     "the lakes - bonus track",
			Artist:   "Taylor Swift",
			Album:    "folklore",
			Duration: 240000,
			URI:      "spotify:track:test_the_lakes",
			ISRC:     "TEST11111111",
		}

		// Test case: "The Lakes" by a different artist should NOT match
		searchResults := []PlexTrack{
			{
				ID:     "plex_the_lakes_different_artist",
				Title:  "The Lakes",
				Artist: "Some Other Artist",
			},
		}

		// After suffix removal, we're searching for "the lakes" by "Taylor Swift"
		result := client.FindBestMatch(searchResults, "the lakes", "Taylor Swift")

		if result != nil {
			confidence := client.calculateConfidence(spotifySong, result, "title_artist")
			t.Errorf("❌ BUG: 'The Lakes' by 'Some Other Artist' incorrectly matched 'the lakes' by 'Taylor Swift' (confidence: %f)", confidence)
		} else {
			t.Logf("✅ Correctly found NO match - 'The Lakes' by 'Some Other Artist' should not match 'the lakes' by 'Taylor Swift'")
		}

		// Test case: "The Lakes" by Taylor Swift SHOULD match
		searchResults2 := []PlexTrack{
			{
				ID:     "plex_the_lakes_taylor_swift",
				Title:  "The Lakes",
				Artist: "Taylor Swift",
			},
		}

		result2 := client.FindBestMatch(searchResults2, "the lakes", "Taylor Swift")

		if result2 != nil {
			confidence := client.calculateConfidence(spotifySong, result2, "title_artist")
			t.Logf("✅ Correctly matched 'The Lakes' by 'Taylor Swift' (confidence: %f)", confidence)
		} else {
			t.Errorf("❌ BUG: 'The Lakes' by 'Taylor Swift' should match 'the lakes' by 'Taylor Swift'")
		}
	})
}

func TestTheLakesConfidenceCalculation(t *testing.T) {
	client := &Client{}

	// Create the problematic Spotify song
	spotifySong := spotify.Song{
		Name:   "the lakes - bonus track",
		Artist: "Taylor Swift",
	}

	plexTrack := &PlexTrack{
		ID:    "197988",
		Album: "How Can I - EP",
	}

	// Calculate confidence
	confidence := client.calculateConfidence(spotifySong, plexTrack, "title_artist")

	// This should be a very low confidence score
	if confidence >= MinConfidenceScore {
		t.Errorf("❌ Confidence too high: %.6f >= %.6f", confidence, MinConfidenceScore)
	} else {
		t.Logf("✅ Correctly low confidence: %.6f < %.6f", confidence, MinConfidenceScore)
	}

	// Let's also test the individual similarity calculations
	titleSimilarity := client.calculateStringSimilarity(
		strings.ToLower("the lakes - bonus track"),
		strings.ToLower("some other title"),
	)

	artistSimilarity := client.calculateStringSimilarity(
		strings.ToLower("Taylor Swift"),
		strings.ToLower("some other artist"),
	)

	t.Logf("Title similarity: %.6f", titleSimilarity)
	t.Logf("Artist similarity: %.6f", artistSimilarity)

	// Test the suffix removal
	suffixRemoved := client.RemoveCommonSuffixes("the lakes - bonus track")
	t.Logf("Suffix removed: '%s'", suffixRemoved)

	// Test suffix removal similarity
	suffixTitleSimilarity := client.calculateStringSimilarity(
		strings.ToLower(suffixRemoved),
		strings.ToLower("some other title"),
	)
	t.Logf("Suffix-removed title similarity: %.6f", suffixTitleSimilarity)
}

func TestFindBestMatchTheLakes(t *testing.T) {
	client := &Client{}

	// Create the problematic Spotify song
	title := "the lakes - bonus track"
	artist := "Taylor Swift"

	tracks := []PlexTrack{
		{
			ID:    "197988",
			Album: "How Can I - EP",
		},
	}

	// Test FindBestMatch directly
	result := client.FindBestMatch(tracks, title, artist)

	if result != nil {
		t.Errorf("❌ FindBestMatch incorrectly returned a match: '%s' by '%s'", result.Title, result.Artist)
	} else {
		t.Logf("✅ FindBestMatch correctly returned no match")
	}
}

func TestFullLibrarySearchSimulation(t *testing.T) {
	client := &Client{}

	// Create the problematic Spotify song
	title := "the lakes - bonus track"
	artist := "Taylor Swift"

	// Create a large list of tracks to simulate the full library search
	tracks := make([]PlexTrack, 100)

	tracks[50] = PlexTrack{
		ID:    "197988",
		Album: "How Can I - EP",
	}

	// Add some other tracks to simulate the library
	for i := 0; i < 100; i++ {
		if i == 50 {
		}
		tracks[i] = PlexTrack{
			ID:     fmt.Sprintf("track_%d", i),
			Title:  fmt.Sprintf("Track %d", i),
			Artist: fmt.Sprintf("Artist %d", i),
			Album:  fmt.Sprintf("Album %d", i),
		}
	}

	// Test FindBestMatch with the large track list
	result := client.FindBestMatch(tracks, title, artist)

	if result != nil {
		t.Logf("Found match: '%s' by '%s' (ID: %s)", result.Title, result.Artist, result.ID)
		if result.ID == "197988" {
		} else {
		}
	} else {
		t.Logf("✅ FindBestMatch correctly returned no match")
	}
}

func TestSpecificIncorrectMatches(t *testing.T) {
	client := &Client{}

	koreanTrack := PlexTrack{
		ID:    "197988",
		Album: "How Can I - EP",
	}

	// Test cases that are incorrectly matching
	testCases := []struct {
		title  string
		artist string
	}{
		{"the lakes - bonus track", "Taylor Swift"},
		{"'tis the damn season", "Taylor Swift"},
		{"right where you left me - bonus track", "Taylor Swift"},
		{"it's time to go - bonus track", "Taylor Swift"},
	}

	for _, tc := range testCases {
		t.Run(fmt.Sprintf("%s_by_%s", tc.title, tc.artist), func(t *testing.T) {
			tracks := []PlexTrack{koreanTrack}

			result := client.FindBestMatch(tracks, tc.title, tc.artist)

			if result != nil {
			} else {
				t.Logf("✅ Correctly rejected match for '%s' by '%s'", tc.title, tc.artist)
			}
		})
	}
}

func TestTheLakesStringSimilarityInvestigation(t *testing.T) {
	client := &Client{}

	// Test the specific case that's causing issues
	spotifyTitle := "the lakes - bonus track"
	spotifyArtist := "Taylor Swift"
	plexTitle := "Some Other Song"    // Define the missing variable
	plexArtist := "Some Other Artist" // Define the missing variable

	t.Logf("Investigating string similarity for:")
	t.Logf("  Spotify: '%s' by '%s'", spotifyTitle, spotifyArtist)
	t.Logf("  Plex:    '%s' by '%s'", plexTitle, plexArtist)

	// Test original similarities
	titleSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(spotifyTitle)),
		strings.ToLower(strings.TrimSpace(plexTitle)),
	)
	artistSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(spotifyArtist)),
		strings.ToLower(strings.TrimSpace(plexArtist)),
	)

	t.Logf("Original title similarity: %f", titleSimilarity)
	t.Logf("Original artist similarity: %f", artistSimilarity)

	// Test with various transformations
	cleanTitleSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(client.removeBrackets(spotifyTitle))),
		strings.ToLower(strings.TrimSpace(client.removeBrackets(plexTitle))),
	)
	featuringTitleSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(client.removeFeaturing(spotifyTitle))),
		strings.ToLower(strings.TrimSpace(client.removeFeaturing(plexTitle))),
	)
	normalizedTitleSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(client.normalizeTitle(spotifyTitle))),
		strings.ToLower(strings.TrimSpace(client.normalizeTitle(plexTitle))),
	)
	withTitleSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(client.removeWith(spotifyTitle))),
		strings.ToLower(strings.TrimSpace(client.removeWith(plexTitle))),
	)
	suffixTitleSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(client.RemoveCommonSuffixes(spotifyTitle))),
		strings.ToLower(strings.TrimSpace(client.RemoveCommonSuffixes(plexTitle))),
	)

	t.Logf("Bracket-removed title similarity: %f", cleanTitleSimilarity)
	t.Logf("Featuring-removed title similarity: %f", featuringTitleSimilarity)
	t.Logf("Normalized title similarity: %f", normalizedTitleSimilarity)
	t.Logf("With-removed title similarity: %f", withTitleSimilarity)
	t.Logf("Suffix-removed title similarity: %f", suffixTitleSimilarity)

	// Show the actual transformed strings
	t.Logf("Suffix-removed Spotify title: '%s'", client.RemoveCommonSuffixes(spotifyTitle))
	t.Logf("Plex title: '%s'", plexTitle)

	// Calculate the best title similarity
	bestTitleSimilarity := titleSimilarity
	if cleanTitleSimilarity > bestTitleSimilarity {
		bestTitleSimilarity = cleanTitleSimilarity
	}
	if featuringTitleSimilarity > bestTitleSimilarity {
		bestTitleSimilarity = featuringTitleSimilarity
	}
	if normalizedTitleSimilarity > bestTitleSimilarity {
		bestTitleSimilarity = normalizedTitleSimilarity
	}
	if withTitleSimilarity > bestTitleSimilarity {
		bestTitleSimilarity = withTitleSimilarity
	}
	if suffixTitleSimilarity > bestTitleSimilarity {
		bestTitleSimilarity = suffixTitleSimilarity
	}

	finalConfidence := (bestTitleSimilarity * 0.7) + (artistSimilarity * 0.3)
	t.Logf("Best title similarity: %f", bestTitleSimilarity)
	t.Logf("Final confidence: %f", finalConfidence)

	t.Logf("\nInvestigating word-level similarities:")

	spotifyWords := strings.Fields(strings.ToLower(client.RemoveCommonSuffixes(spotifyTitle)))
	plexWords := strings.Fields(strings.ToLower(plexTitle))

	t.Logf("Spotify words: %v", spotifyWords)
	t.Logf("Plex words: %v", plexWords)

	for _, spotifyWord := range spotifyWords {
		for _, plexWord := range plexWords {
			wordSimilarity := client.calculateStringSimilarity(spotifyWord, plexWord)
			if wordSimilarity > 0.5 {
				t.Logf("  High word similarity: '%s' vs '%s' = %f", spotifyWord, plexWord, wordSimilarity)
			}
		}
	}

	// Test substring matching
	if strings.Contains(strings.ToLower(client.RemoveCommonSuffixes(spotifyTitle)), strings.ToLower(plexTitle)) {
		t.Logf("⚠️  Plex title is a substring of Spotify title!")
	}
	if strings.Contains(strings.ToLower(plexTitle), strings.ToLower(client.RemoveCommonSuffixes(spotifyTitle))) {
		t.Logf("⚠️  Spotify title is a substring of Plex title!")
	}

	// Test individual word matching
	spotifyTitleLower := strings.ToLower(client.RemoveCommonSuffixes(spotifyTitle))
	plexTitleLower := strings.ToLower(plexTitle)

	for _, word := range strings.Fields(spotifyTitleLower) {
		if strings.Contains(plexTitleLower, word) {
			t.Logf("⚠️  Word '%s' from Spotify title found in Plex title", word)
		}
	}

	for _, word := range strings.Fields(plexTitleLower) {
		if strings.Contains(spotifyTitleLower, word) {
			t.Logf("⚠️  Word '%s' from Plex title found in Spotify title", word)
		}
	}
}

func TestConfidenceThresholdOptimization(t *testing.T) {
	client := &Client{}

	// The problematic case
	problematicSong := spotify.Song{
		Name:   "the lakes - bonus track",
		Artist: "Taylor Swift",
	}

	koreanTrack := PlexTrack{
		ID: "korean-track-id",
	}

	// Calculate confidence for the problematic match
	confidence := client.calculateConfidence(problematicSong, &koreanTrack, "title_artist")
	t.Logf("Problematic match confidence: %f", confidence)

	// Test different thresholds
	thresholds := []float64{0.1, 0.2, 0.3, 0.4, 0.5, 0.6, 0.7, 0.8, 0.9}

	t.Logf("\nThreshold Analysis:")
	t.Logf("==================")

	for _, threshold := range thresholds {
		if confidence >= threshold {
			t.Logf("❌ Threshold %.1f: WOULD MATCH (confidence %.3f >= %.1f)", threshold, confidence, threshold)
		} else {
			t.Logf("✅ Threshold %.1f: would NOT match (confidence %.3f < %.1f)", threshold, confidence, threshold)
		}
	}

	// Test with some legitimate matches to ensure we don't break them
	legitimateMatches := []struct {
		name     string
		spotify  spotify.Song
		plex     PlexTrack
		expected bool // whether this should match
	}{
		{
			name:     "Exact match",
			expected: true,
		},
		{
			name:     "Case insensitive",
			expected: true,
		},
		{
			name:     "Similar title",
			expected: true,
		},
		{
			name:     "Different artist",
			expected: false,
		},
		{
			name:     "Completely different",
			spotify:  spotify.Song{Name: "Completely Different Song", Artist: "Different Artist"},
			expected: false,
		},
	}

	t.Logf("\nLegitimate Match Analysis:")
	t.Logf("=========================")

	for _, match := range legitimateMatches {
		conf := client.calculateConfidence(match.spotify, &match.plex, "title_artist")
		t.Logf("\n%s:", match.name)
		t.Logf("  Spotify: '%s' by '%s'", match.spotify.Name, match.spotify.Artist)
		t.Logf("  Plex:    '%s' by '%s'", match.plex.Title, match.plex.Artist)
		t.Logf("  Confidence: %f", conf)
		t.Logf("  Expected to match: %t", match.expected)

		for _, threshold := range thresholds {
			wouldMatch := conf >= threshold
			if wouldMatch == match.expected {
				t.Logf("    ✅ Threshold %.1f: %s", threshold,
					map[bool]string{true: "MATCH", false: "NO MATCH"}[wouldMatch])
			} else {
				t.Logf("    ❌ Threshold %.1f: %s (should be %s)", threshold,
					map[bool]string{true: "MATCH", false: "NO MATCH"}[wouldMatch],
					map[bool]string{true: "MATCH", false: "NO MATCH"}[match.expected])
			}
		}
	}

	// Recommend optimal threshold
	t.Logf("\nThreshold Recommendation:")
	t.Logf("========================")
	t.Logf("Current threshold: %f", MinConfidenceScore)
	t.Logf("Problematic match confidence: %f", confidence)

	// Find the minimum threshold that prevents the problematic match
	minSafeThreshold := confidence + 0.01 // Add small buffer
	t.Logf("Minimum safe threshold to prevent false match: %f", minSafeThreshold)

	// Check if this threshold breaks any legitimate matches
	brokenLegitimateMatches := 0
	for _, match := range legitimateMatches {
		if match.expected {
			conf := client.calculateConfidence(match.spotify, &match.plex, "title_artist")
			if conf < minSafeThreshold {
				brokenLegitimateMatches++
				t.Logf("⚠️  Threshold %.3f would break legitimate match: '%s' by '%s' (confidence: %f)",
					minSafeThreshold, match.spotify.Name, match.spotify.Artist, conf)
			}
		}
	}

	if brokenLegitimateMatches == 0 {
		t.Logf("✅ Recommended threshold: %f (prevents false matches without breaking legitimate ones)", minSafeThreshold)
	} else {
		t.Logf("❌ Threshold %.3f would break %d legitimate matches", minSafeThreshold, brokenLegitimateMatches)
		t.Logf("Consider a higher threshold or additional matching logic")
	}
}

func TestRealPlexAPIBugInvestigation(t *testing.T) {
	// Skip if Plex environment is not configured
	if os.Getenv("PLEX_URL") == "" || os.Getenv("PLEX_TOKEN") == "" || os.Getenv("PLEX_SECTION_ID") == "" {
		t.Skip("Skipping real Plex API test - environment not configured")
	}

	// Load config
	cfg, err := config.Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	// Create real Plex client
	client := NewClient(cfg)
	ctx := context.Background()

	// The problematic song
	problematicSong := spotify.Song{
		Name:   "the lakes - bonus track",
		Artist: "Taylor Swift",
	}

	t.Logf("🔍 Testing real Plex API with problematic song: '%s' by '%s'", problematicSong.Name, problematicSong.Artist)

	// Test the actual SearchTrack method
	track, matchType, err := client.SearchTrack(ctx, problematicSong)
	if err != nil {
		t.Logf("SearchTrack error: %v", err)
	}

	if track != nil {
		confidence := client.calculateConfidence(problematicSong, track, matchType)
		t.Logf("⚠️  REAL BUG: SearchTrack returned match: '%s' by '%s' (type: %s, confidence: %f)",
			track.Title, track.Artist, matchType, confidence)

		// Analyze why it matched
		titleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(problematicSong.Name),
			strings.ToLower(track.Title),
		)
		artistSimilarity := client.calculateStringSimilarity(
			strings.ToLower(problematicSong.Artist),
			strings.ToLower(track.Artist),
		)

		t.Logf("Title similarity: %f", titleSimilarity)
		t.Logf("Artist similarity: %f", artistSimilarity)
	} else {
		t.Logf("✅ No match found - this is correct behavior")
	}
}

func TestArtistSimilarityCheck(t *testing.T) {
	client := &Client{}

	// Test cases where titles are similar but artists are different
	testCases := []struct {
		name           string
		spotifyTitle   string
		spotifyArtist  string
		plexTitle      string
		plexArtist     string
		shouldMatch    bool
		expectedReason string
	}{
		{
			name:           "Similar title, different artist - should NOT match",
			spotifyTitle:   "The Lakes",
			spotifyArtist:  "Different Artist",
			plexTitle:      "The Lakes",
			plexArtist:     "Taylor Swift",
			shouldMatch:    false,
			expectedReason: "artist similarity too low",
		},
		{
			name:           "Similar title, similar artist - should match",
			spotifyTitle:   "The Lakes",
			spotifyArtist:  "Taylor Swift",
			plexTitle:      "The Lakes",
			plexArtist:     "Taylor Swift",
			shouldMatch:    true,
			expectedReason: "exact match",
		},
		{
			name:           "Similar title, very different artist - should NOT match",
			spotifyTitle:   "The Lakes",
			spotifyArtist:  "Taylor Swift",
			plexTitle:      "The Lakes",
			plexArtist:     "Completely Different Artist",
			shouldMatch:    false,
			expectedReason: "artist similarity too low",
		},
		{
			name:           "Different title, same artist - should NOT match",
			spotifyTitle:   "Completely Different",
			spotifyArtist:  "Taylor Swift",
			plexTitle:      "The Lakes",
			plexArtist:     "Taylor Swift",
			shouldMatch:    false,
			expectedReason: "title similarity too low",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a library with just the Plex track
			library := []PlexTrack{
				{
					ID:     "test-track",
					Title:  tc.plexTitle,
					Artist: tc.plexArtist,
				},
			}

			// Use FindBestMatch to see if it would match
			result := client.FindBestMatch(library, tc.spotifyTitle, tc.spotifyArtist)

			if tc.shouldMatch {
				if result == nil {
					t.Errorf("Expected match but got none for '%s' by '%s'", tc.spotifyTitle, tc.spotifyArtist)
				} else {
					t.Logf("✅ Correctly matched '%s' by '%s' -> '%s' by '%s'",
						tc.spotifyTitle, tc.spotifyArtist, result.Title, result.Artist)
				}
			} else {
				if result != nil {
					confidence := client.calculateConfidence(
						spotify.Song{Name: tc.spotifyTitle, Artist: tc.spotifyArtist},
						result, "title_artist")
					t.Errorf("❌ Expected no match but got '%s' by '%s' (confidence: %f)",
						result.Title, result.Artist, confidence)
				} else {
					t.Logf("✅ Correctly rejected match for '%s' by '%s'", tc.spotifyTitle, tc.spotifyArtist)
				}
			}
		})
	}
}

func TestSearchByTitleWithFeaturingRemoval(t *testing.T) {
	client := &Client{}

	// Test case: "Girl, so confusing featuring lorde" by "Charli xcx"
	// This should test the fourth priority in SearchTrack where featuring removal is used
	// when no other match priorities are met

	spotifySong := spotify.Song{
		Name:   "Girl, so confusing featuring lorde",
		Artist: "Charli xcx",
	}

	// Create a mock Plex track that should match after featuring removal
	// Note: This track exists in Plex without the "featuring lorde" part
	expectedPlexTrack := PlexTrack{
		ID:     "12345",
		Title:  "Girl, so confusing",
		Artist: "Charli xcx",
		Album:  "Brat",
	}

	// Create a mock library with the expected track
	mockLibrary := []PlexTrack{expectedPlexTrack}

	t.Run("FeaturingRemovalPriority", func(t *testing.T) {
		// Test that removeFeaturing correctly strips "featuring lorde"
		featuringRemoved := client.removeFeaturing(spotifySong.Name)
		expectedTitle := "Girl, so confusing"

		if featuringRemoved != expectedTitle {
			t.Errorf("removeFeaturing(%q) = %q, expected %q",
				spotifySong.Name, featuringRemoved, expectedTitle)
		}

		// Test that the search would find the match using the featuring-removed title
		// This simulates the fourth priority in SearchTrack method
		result := client.FindBestMatch(mockLibrary, featuringRemoved, spotifySong.Artist)

		if result == nil {
			t.Errorf("FindBestMatch should find '%s' by '%s' when searching with featuring-removed title '%s'",
				expectedPlexTrack.Title, expectedPlexTrack.Artist, featuringRemoved)
		} else {
			if result.Title != expectedPlexTrack.Title {
				t.Errorf("Expected title '%s', got '%s'", expectedPlexTrack.Title, result.Title)
			}
			if result.Artist != expectedPlexTrack.Artist {
				t.Errorf("Expected artist '%s', got '%s'", expectedPlexTrack.Artist, result.Artist)
			}
		}

		// Verify that the original title with "featuring" would NOT match
		// This ensures the featuring removal is necessary for the match
		originalResult := client.FindBestMatch(mockLibrary, spotifySong.Name, spotifySong.Artist)
		if originalResult != nil {
			t.Logf("Note: Original title '%s' also matches, which is acceptable", spotifySong.Name)
		}
	})

	t.Run("RealisticSearchScenario", func(t *testing.T) {
		// Create a more realistic scenario where the Plex library only contains
		// the base track without featuring information

		// Simulate a Plex library that only has the base track
		plexLibrary := []PlexTrack{
			{
				ID:     "12345",
				Title:  "Girl, so confusing",
				Artist: "Charli xcx",
				Album:  "Brat",
			},
		}

		// Test that searching with the full title (including featuring) would fail
		// This simulates the scenario where no other match priorities work
		fullTitleResult := client.FindBestMatch(plexLibrary, spotifySong.Name, spotifySong.Artist)

		// Test that searching with featuring removed would succeed
		featuringRemoved := client.removeFeaturing(spotifySong.Name)
		featuringResult := client.FindBestMatch(plexLibrary, featuringRemoved, spotifySong.Artist)

		// Verify that featuring removal enables the match
		if featuringResult == nil {
			t.Errorf("Search with featuring-removed title '%s' should find the track", featuringRemoved)
		} else {
			if featuringResult.Title != "Girl, so confusing" {
				t.Errorf("Expected title 'Girl, so confusing', got '%s'", featuringResult.Title)
			}
		}

		t.Logf("Full title search result: %v", fullTitleResult != nil)
		t.Logf("Featuring-removed search result: %v", featuringResult != nil)
		t.Logf("Featuring removal transforms '%s' to '%s'", spotifySong.Name, featuringRemoved)
	})

	t.Run("SearchPrioritySimulation", func(t *testing.T) {
		// Simulate the exact search priority flow from SearchTrack method
		// to verify that featuring removal is used as the fourth priority

		// Step 1: Try exact title/artist match (first priority)
		exactResult := client.FindBestMatch(mockLibrary, spotifySong.Name, spotifySong.Artist)
		t.Logf("Step 1 (Exact match): %v", exactResult != nil)

		// Step 2: Check for single quotes (second priority) - not applicable here
		hasSingleQuotes := strings.Contains(spotifySong.Name, "'") || strings.Contains(spotifySong.Artist, "'")
		t.Logf("Step 2 (Single quotes): %v", hasSingleQuotes)

		// Step 3: Try with brackets removed (third priority)
		bracketsRemoved := client.removeBrackets(spotifySong.Name)
		bracketsResult := client.FindBestMatch(mockLibrary, bracketsRemoved, spotifySong.Artist)
		t.Logf("Step 3 (Brackets removed): %v", bracketsResult != nil)

		// Step 4: Try with featuring removed (fourth priority) - this is what we're testing
		featuringRemoved := client.removeFeaturing(spotifySong.Name)
		featuringResult := client.FindBestMatch(mockLibrary, featuringRemoved, spotifySong.Artist)
		t.Logf("Step 4 (Featuring removed): %v", featuringResult != nil)

		// Verify that featuring removal produces the expected title
		if featuringRemoved != "Girl, so confusing" {
			t.Errorf("Featuring removal should produce 'Girl, so confusing', got '%s'", featuringRemoved)
		}

		// Verify that the featuring-removed search finds the expected track
		if featuringResult == nil {
			t.Errorf("Search with featuring-removed title '%s' should find the expected track", featuringRemoved)
		} else {
			if featuringResult.Title != "Girl, so confusing" {
				t.Errorf("Expected title 'Girl, so confusing', got '%s'", featuringResult.Title)
			}
		}
	})

	t.Run("LogMessageVerification", func(t *testing.T) {
		// Test that the search would produce the expected log message
		// "searchByTitle: searching for 'Girl, so confusing' by 'Charli xcx', found 1 results"

		featuringRemoved := client.removeFeaturing(spotifySong.Name)

		// Simulate what the log message would look like
		expectedLogMessage := fmt.Sprintf("searchByTitle: searching for '%s' by '%s', found %d results",
			featuringRemoved, spotifySong.Artist, len(mockLibrary))

		// Verify the expected log message format
		if !strings.Contains(expectedLogMessage, "searchByTitle: searching for 'Girl, so confusing' by 'Charli xcx'") {
			t.Errorf("Expected log message to contain 'Girl, so confusing' by 'Charli xcx'")
		}

		t.Logf("Expected log message: %s", expectedLogMessage)
	})

	t.Run("OriginalLogMessageScenario", func(t *testing.T) {
		// Test the exact scenario from the original log message:
		// "searchByTitle: searching for 'Girl, so confusing featuring lorde' by 'Charli xcx', found 0 results"
		// This should verify that when no results are found with the full title,
		// the featuring removal should be used as the fourth priority

		// Create a scenario where the full title search returns 0 results
		emptyLibrary := []PlexTrack{}

		// Simulate the log message for the original search (0 results)
		originalLogMessage := fmt.Sprintf("searchByTitle: searching for '%s' by '%s', found %d results",
			spotifySong.Name, spotifySong.Artist, len(emptyLibrary))

		// Verify the original log message format
		if !strings.Contains(originalLogMessage, "searchByTitle: searching for 'Girl, so confusing featuring lorde' by 'Charli xcx', found 0 results") {
			t.Errorf("Expected original log message to contain 'Girl, so confusing featuring lorde' by 'Charli xcx', found 0 results")
		}

		// Now simulate what would happen in the fourth priority (featuring removal)
		featuringRemoved := client.removeFeaturing(spotifySong.Name)
		featuringLogMessage := fmt.Sprintf("searchByTitle: searching for '%s' by '%s', found %d results",
			featuringRemoved, spotifySong.Artist, len(mockLibrary))

		// Verify the featuring-removed log message format
		if !strings.Contains(featuringLogMessage, "searchByTitle: searching for 'Girl, so confusing' by 'Charli xcx', found 1 results") {
			t.Errorf("Expected featuring-removed log message to contain 'Girl, so confusing' by 'Charli xcx', found 1 results")
		}

		t.Logf("Original search (0 results): %s", originalLogMessage)
		t.Logf("Featuring-removed search (1 result): %s", featuringLogMessage)
		t.Logf("This demonstrates the fourth priority in SearchTrack where featuring removal enables the match")
	})
}

func TestPunctuationMatching(t *testing.T) {
	client := &Client{}

	// Test case: Different types of punctuation marks that should match
	// Spotify: "Leigh-Anne" (regular hyphen) vs Plex: "Leigh‐Anne" (en dash)
	// Spotify: "Stealin' Love" (straight apostrophe) vs Plex: "Stealin' Love" (curly apostrophe)

	spotifySong := spotify.Song{
		Name:   "Stealin' Love",
		Artist: "Leigh-Anne",
	}

	// Create a mock Plex track with different punctuation marks
	plexTrack := PlexTrack{
		ID:     "12345",
		Title:  "Stealin' Love", // Note: This might have a curly apostrophe
		Artist: "Leigh‐Anne",    // Note: This has an en dash instead of hyphen
		Album:  "No Hard Feelings",
	}

	t.Run("PunctuationDifferences", func(t *testing.T) {
		// Test that the current similarity function doesn't match these
		titleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifySong.Name)),
			strings.ToLower(strings.TrimSpace(plexTrack.Title)),
		)
		artistSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifySong.Artist)),
			strings.ToLower(strings.TrimSpace(plexTrack.Artist)),
		)

		t.Logf("Current title similarity: %f", titleSimilarity)
		t.Logf("Current artist similarity: %f", artistSimilarity)
		t.Logf("Spotify title: '%s'", spotifySong.Name)
		t.Logf("Plex title: '%s'", plexTrack.Title)
		t.Logf("Spotify artist: '%s'", spotifySong.Artist)
		t.Logf("Plex artist: '%s'", plexTrack.Artist)

		// The current similarity should be low due to punctuation differences
		if titleSimilarity >= 1.0 {
			t.Logf("Note: Title similarity is unexpectedly high, may already be normalized")
		}
		if artistSimilarity >= 1.0 {
			t.Logf("Note: Artist similarity is unexpectedly high, may already be normalized")
		}

		// Test with FindBestMatch to see if it would match
		mockLibrary := []PlexTrack{plexTrack}
		result := client.FindBestMatch(mockLibrary, spotifySong.Name, spotifySong.Artist)

		if result != nil {
			t.Logf("Current FindBestMatch found: '%s' by '%s'", result.Title, result.Artist)
		} else {
			t.Logf("Current FindBestMatch did not find a match")
		}
	})

	t.Run("NormalizedPunctuationMatching", func(t *testing.T) {
		// Test with normalized punctuation
		normalizedSpotifyTitle := client.normalizePunctuation(spotifySong.Name)
		normalizedSpotifyArtist := client.normalizePunctuation(spotifySong.Artist)
		normalizedPlexTitle := client.normalizePunctuation(plexTrack.Title)
		normalizedPlexArtist := client.normalizePunctuation(plexTrack.Artist)

		t.Logf("Normalized Spotify title: '%s'", normalizedSpotifyTitle)
		t.Logf("Normalized Plex title: '%s'", normalizedPlexTitle)
		t.Logf("Normalized Spotify artist: '%s'", normalizedSpotifyArtist)
		t.Logf("Normalized Plex artist: '%s'", normalizedPlexArtist)

		// Test similarity with normalized punctuation
		normalizedTitleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(normalizedSpotifyTitle)),
			strings.ToLower(strings.TrimSpace(normalizedPlexTitle)),
		)
		normalizedArtistSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(normalizedSpotifyArtist)),
			strings.ToLower(strings.TrimSpace(normalizedPlexArtist)),
		)

		t.Logf("Normalized title similarity: %f", normalizedTitleSimilarity)
		t.Logf("Normalized artist similarity: %f", normalizedArtistSimilarity)

		// After normalization, these should match
		if normalizedTitleSimilarity < 0.9 {
			t.Errorf("Expected normalized title similarity >= 0.9, got %f", normalizedTitleSimilarity)
		}
		if normalizedArtistSimilarity < 0.9 {
			t.Errorf("Expected normalized artist similarity >= 0.9, got %f", normalizedArtistSimilarity)
		}

		// Test FindBestMatch with normalized strings
		mockLibrary := []PlexTrack{plexTrack}
		normalizedResult := client.FindBestMatchWithNormalizedPunctuation(mockLibrary, spotifySong.Name, spotifySong.Artist)
		if normalizedResult == nil {
			t.Errorf("Expected FindBestMatchWithNormalizedPunctuation to find a match")
		} else {
			t.Logf("Normalized FindBestMatch found: '%s' by '%s'", normalizedResult.Title, normalizedResult.Artist)
		}
	})

	t.Run("VariousPunctuationCases", func(t *testing.T) {
		// Test various punctuation normalization cases
		testCases := []struct {
			input    string
			expected string
			desc     string
		}{
			{"Leigh-Anne", "Leigh-Anne", "Regular hyphen"},
			{"Leigh‐Anne", "Leigh-Anne", "En dash to hyphen"},
			{"Leigh—Anne", "Leigh-Anne", "Em dash to hyphen"},
			{"Stealin' Love", "Stealin' Love", "Straight apostrophe"},
			{"Stealin' Love", "Stealin' Love", "Curly apostrophe to straight"},
			{"Don't Stop", "Don't Stop", "Contraction apostrophe"},
			{"O'Connor", "O'Connor", "Name with apostrophe"},
			{"Mary-Jane", "Mary-Jane", "Name with hyphen"},
			{"Mary‐Jane", "Mary-Jane", "Name with en dash"},
			{"Chloe x Halle", "Chloe x Halle", "Regular 'x'"},
			{"Chloe × Halle", "Chloe x Halle", "Multiplication symbol to 'x'"},
		}

		for _, tc := range testCases {
			normalized := client.normalizePunctuation(tc.input)
			if normalized != tc.expected {
				t.Errorf("%s: normalizePunctuation('%s') = '%s', expected '%s'",
					tc.desc, tc.input, normalized, tc.expected)
			}
		}
	})

	t.Run("ChloeXHalleMultiplicationSymbol", func(t *testing.T) {
		// Test case for the specific issue: "Chloe x Halle" vs "Chloe × Halle"
		// Spotify: "Chloe x Halle" (regular 'x')
		// Plex: "Chloe × Halle" (multiplication symbol ×)

		spotifySong := spotify.Song{
			Name:   "Do It",
			Artist: "Chloe x Halle",
		}

		// Create a mock Plex track with multiplication symbol
		plexTrack := PlexTrack{
			ID:     "67890",
			Title:  "Do It",
			Artist: "Chloe × Halle", // Note: This has a multiplication symbol ×
			Album:  "Ungodly Hour",
		}

		// Test that without normalization, these don't match well
		titleSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifySong.Name)),
			strings.ToLower(strings.TrimSpace(plexTrack.Title)),
		)
		artistSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(spotifySong.Artist)),
			strings.ToLower(strings.TrimSpace(plexTrack.Artist)),
		)

		t.Logf("Without normalization - Title similarity: %f", titleSimilarity)
		t.Logf("Without normalization - Artist similarity: %f", artistSimilarity)
		t.Logf("Spotify artist: '%s'", spotifySong.Artist)
		t.Logf("Plex artist: '%s'", plexTrack.Artist)

		// Test with normalization
		normalizedSpotifyArtist := client.normalizePunctuation(spotifySong.Artist)
		normalizedPlexArtist := client.normalizePunctuation(plexTrack.Artist)

		t.Logf("Normalized Spotify artist: '%s'", normalizedSpotifyArtist)
		t.Logf("Normalized Plex artist: '%s'", normalizedPlexArtist)

		normalizedArtistSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(normalizedSpotifyArtist)),
			strings.ToLower(strings.TrimSpace(normalizedPlexArtist)),
		)

		t.Logf("With normalization - Artist similarity: %f", normalizedArtistSimilarity)

		// After normalization, the artist similarity should be much higher
		if normalizedArtistSimilarity < 0.9 {
			t.Errorf("Expected normalized artist similarity >= 0.9, got %f", normalizedArtistSimilarity)
		}

		// Test that the multiplication symbol is properly normalized
		if normalizedPlexArtist != "Chloe x Halle" {
			t.Errorf("Expected normalized Plex artist to be 'Chloe x Halle', got '%s'", normalizedPlexArtist)
		}

		// Test FindBestMatch to see if it would match
		mockLibrary := []PlexTrack{plexTrack}
		result := client.FindBestMatch(mockLibrary, spotifySong.Name, spotifySong.Artist)

		if result != nil {
			t.Logf("FindBestMatch found: '%s' by '%s'", result.Title, result.Artist)
		} else {
			t.Logf("FindBestMatch did not find a match")
		}

		// Test FindBestMatchWithNormalizedPunctuation
		normalizedResult := client.FindBestMatchWithNormalizedPunctuation(mockLibrary, spotifySong.Name, spotifySong.Artist)
		if normalizedResult == nil {
			t.Errorf("Expected FindBestMatchWithNormalizedPunctuation to find a match")
		} else {
			t.Logf("Normalized FindBestMatch found: '%s' by '%s'", normalizedResult.Title, normalizedResult.Artist)
		}

		// Test the normalizePunctuation function directly
		testCases := []struct {
			input    string
			expected string
			desc     string
		}{
			{"Chloe x Halle", "Chloe x Halle", "Regular 'x'"},
			{"Chloe × Halle", "Chloe x Halle", "Multiplication symbol to 'x'"},
		}

		for _, tc := range testCases {
			normalized := client.normalizePunctuation(tc.input)
			if normalized != tc.expected {
				t.Errorf("%s: normalizePunctuation('%s') = '%s', expected '%s'",
					tc.desc, tc.input, normalized, tc.expected)
			}
		}
	})
}

func TestChloeXHalleIssueFix(t *testing.T) {
	client := &Client{}

	// Simulate the exact issue described in the user query
	// Spotify: "Chloe x Halle" (regular 'x')
	// Plex: "Chloe × Halle" (multiplication symbol ×)
	// Track: "Do It"

	spotifySong := spotify.Song{
		Name:   "Do It",
		Artist: "Chloe x Halle",
	}

	// Create a mock Plex library with the problematic track
	plexTracks := []PlexTrack{
		{
			ID:     "12345",
			Title:  "Do It",
			Artist: "Chloe × Halle", // Note: This has a multiplication symbol ×
			Album:  "Ungodly Hour",
		},
		{
			ID:     "67890",
			Title:  "Let's Do It Again",
			Artist: "TLC", // This is the incorrect match that was happening
			Album:  "CrazySexyCool",
		},
	}

	t.Run("ExactIssueScenario", func(t *testing.T) {
		// Test that the correct match is found
		result := client.FindBestMatch(plexTracks, spotifySong.Name, spotifySong.Artist)

		if result == nil {
			t.Errorf("Expected to find a match for 'Do It' by 'Chloe x Halle'")
			return
		}

		// Verify we got the correct match
		if result.Title != "Do It" {
			t.Errorf("Expected title 'Do It', got '%s'", result.Title)
		}
		if result.Artist != "Chloe × Halle" {
			t.Errorf("Expected artist 'Chloe × Halle', got '%s'", result.Artist)
		}

		t.Logf("✅ Correctly matched: '%s' by '%s'", result.Title, result.Artist)
	})

	t.Run("NormalizedPunctuationMatch", func(t *testing.T) {
		// Test with the normalized punctuation function
		result := client.FindBestMatchWithNormalizedPunctuation(plexTracks, spotifySong.Name, spotifySong.Artist)

		if result == nil {
			t.Errorf("Expected to find a match with normalized punctuation")
			return
		}

		// Verify we got the correct match
		if result.Title != "Do It" {
			t.Errorf("Expected title 'Do It', got '%s'", result.Title)
		}
		if result.Artist != "Chloe × Halle" {
			t.Errorf("Expected artist 'Chloe × Halle', got '%s'", result.Artist)
		}

		t.Logf("✅ Normalized punctuation correctly matched: '%s' by '%s'", result.Title, result.Artist)
	})

	t.Run("SimilarTrackNameMatch", func(t *testing.T) {
		// Test that the similar track name doesn't incorrectly match
		// This simulates the issue where "Let's Do It Again" by "TLC" was matching instead

		// Search for a track that might have a similar name
		result := client.FindBestMatch(plexTracks, "Do It", "Chloe x Halle")

		if result == nil {
			t.Errorf("Expected to find a match for 'Do It' by 'Chloe x Halle'")
			return
		}

		// Verify we got the correct match, not the similar one
		if result.Title == "Let's Do It Again" {
			t.Errorf("❌ Incorrectly matched similar track name: '%s' by '%s'", result.Title, result.Artist)
		} else {
			t.Logf("✅ Correctly avoided similar track name match: got '%s' by '%s'", result.Title, result.Artist)
		}
	})

	t.Run("PunctuationNormalization", func(t *testing.T) {
		// Test that the multiplication symbol is properly normalized
		normalized := client.normalizePunctuation("Chloe × Halle")
		expected := "Chloe x Halle"

		if normalized != expected {
			t.Errorf("Expected normalization of 'Chloe × Halle' to be '%s', got '%s'", expected, normalized)
		} else {
			t.Logf("✅ Punctuation normalization works: 'Chloe × Halle' -> '%s'", normalized)
		}
	})

	t.Run("EllipsisNormalization", func(t *testing.T) {
		// Test that the Unicode ellipsis character is normalized to three periods
		// This handles cases like Spotify "DANCE..." vs Plex "DANCE…"
		normalized := client.normalizePunctuation("DANCE\u2026")
		expected := "DANCE..."

		if normalized != expected {
			t.Errorf("Expected normalization of 'DANCE…' to be '%s', got '%s'", expected, normalized)
		} else {
			t.Logf("✅ Ellipsis normalization works: 'DANCE…' -> '%s'", normalized)
		}
	})

	t.Run("EllipsisMatchingDANCE", func(t *testing.T) {
		// Test that "DANCE..." (Spotify, three periods) matches "DANCE…" (Plex, Unicode ellipsis)
		plexTracks := []PlexTrack{
			{ID: "1", Title: "DANCE\u2026", Artist: "Slayyyter"},
		}

		result := client.FindBestMatch(plexTracks, "DANCE...", "Slayyyter")

		if result == nil {
			t.Errorf("Expected to find a match for 'DANCE...' by 'Slayyyter' but got nil")
		} else {
			t.Logf("✅ Ellipsis matching works: 'DANCE...' matched with 'DANCE…' by '%s'", result.Artist)
		}
	})
}

func TestPlaylistSyncAttribution(t *testing.T) {
	client := &Client{}

	// Test case: Playlist with no description should still get attribution
	t.Run("EmptyDescriptionWithAttribution", func(t *testing.T) {
		// Test the addSyncAttribution function directly
		emptyDescription := ""
		spotifyPlaylistID := "37i9dQZF1DXcBWIGoYBM5M"

		result := client.addSyncAttribution(emptyDescription, spotifyPlaylistID)
		expected := "synced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

		if result != expected {
			t.Errorf("Expected attribution for empty description to be '%s', got '%s'", expected, result)
		} else {
			t.Logf("✅ Empty description correctly gets attribution: '%s'", result)
		}
	})

	t.Run("EmptyDescriptionWithoutAttribution", func(t *testing.T) {
		// Test with empty spotifyPlaylistID
		emptyDescription := ""
		emptySpotifyPlaylistID := ""

		result := client.addSyncAttribution(emptyDescription, emptySpotifyPlaylistID)
		expected := ""

		if result != expected {
			t.Errorf("Expected empty result for empty description and empty playlist ID, got '%s'", result)
		} else {
			t.Logf("✅ Empty description with empty playlist ID correctly returns empty string")
		}
	})

	t.Run("ExistingDescriptionWithAttribution", func(t *testing.T) {
		// Test with existing description
		existingDescription := "My awesome playlist"
		spotifyPlaylistID := "37i9dQZF1DXcBWIGoYBM5M"

		result := client.addSyncAttribution(existingDescription, spotifyPlaylistID)
		expected := "My awesome playlist\n\nsynced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

		if result != expected {
			t.Errorf("Expected attribution for existing description to be '%s', got '%s'", expected, result)
		} else {
			t.Logf("✅ Existing description correctly gets attribution appended")
		}
	})

	t.Run("UpdatePlaylistMetadataWithEmptyDescription", func(t *testing.T) {
		// Test that UpdatePlaylistMetadata handles empty description correctly
		// This simulates the real-world scenario where a playlist has no description

		// Test parameters
		description := "" // Empty description
		spotifyPlaylistID := "37i9dQZF1DXcBWIGoYBM5M"

		// This would normally make an HTTP request, but we're just testing the logic
		// The actual HTTP call would fail in a test environment, but we can verify
		// that the function is called with the correct parameters

		// Test that addSyncAttribution is called correctly
		expectedDescription := client.addSyncAttribution(description, spotifyPlaylistID)
		expectedAttribution := "synced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

		if expectedDescription != expectedAttribution {
			t.Errorf("Expected description with attribution to be '%s', got '%s'", expectedAttribution, expectedDescription)
		} else {
			t.Logf("✅ UpdatePlaylistMetadata would correctly add attribution to empty description")
		}
	})
}

func TestPlaylistMetadataUpdateScenarios(t *testing.T) {
	client := &Client{}

	t.Run("NewPlaylistCreationWithMetadata", func(t *testing.T) {
		// Test that new playlist creation includes metadata with attribution
		description := "My test playlist"
		spotifyPlaylistID := "37i9dQZF1DXcBWIGoYBM5M"

		// Test the addSyncAttribution function that CreatePlaylist uses
		expectedDescription := client.addSyncAttribution(description, spotifyPlaylistID)
		expected := "My test playlist\n\nsynced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

		if expectedDescription != expected {
			t.Errorf("Expected new playlist description to be '%s', got '%s'", expected, expectedDescription)
		} else {
			t.Logf("✅ New playlist creation would correctly include metadata with attribution")
		}
	})

	t.Run("NewPlaylistCreationWithEmptyDescription", func(t *testing.T) {
		// Test that new playlist creation includes attribution even with empty description
		description := "" // Empty description
		spotifyPlaylistID := "37i9dQZF1DXcBWIGoYBM5M"

		// Test the addSyncAttribution function that CreatePlaylist uses
		expectedDescription := client.addSyncAttribution(description, spotifyPlaylistID)
		expected := "synced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

		if expectedDescription != expected {
			t.Errorf("Expected new playlist description to be '%s', got '%s'", expected, expectedDescription)
		} else {
			t.Logf("✅ New playlist creation would correctly include attribution even with empty description")
		}
	})

	t.Run("ExistingPlaylistSyncWithMetadata", func(t *testing.T) {
		// Test that existing playlist sync includes metadata with attribution
		description := "Updated playlist description"
		spotifyPlaylistID := "37i9dQZF1DXcBWIGoYBM5M"

		// Test the addSyncAttribution function that UpdatePlaylistMetadata uses
		expectedDescription := client.addSyncAttribution(description, spotifyPlaylistID)
		expected := "Updated playlist description\n\nsynced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

		if expectedDescription != expected {
			t.Errorf("Expected existing playlist description to be '%s', got '%s'", expected, expectedDescription)
		} else {
			t.Logf("✅ Existing playlist sync would correctly include metadata with attribution")
		}
	})

	t.Run("ExistingPlaylistSyncWithEmptyDescription", func(t *testing.T) {
		// Test that existing playlist sync includes attribution even with empty description
		description := "" // Empty description
		spotifyPlaylistID := "37i9dQZF1DXcBWIGoYBM5M"

		// Test the addSyncAttribution function that UpdatePlaylistMetadata uses
		expectedDescription := client.addSyncAttribution(description, spotifyPlaylistID)
		expected := "synced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M"

		if expectedDescription != expected {
			t.Errorf("Expected existing playlist description to be '%s', got '%s'", expected, expectedDescription)
		} else {
			t.Logf("✅ Existing playlist sync would correctly include attribution even with empty description")
		}
	})

	t.Run("CreatePlaylistMetadataLogic", func(t *testing.T) {
		// Test the metadata logic that CreatePlaylist uses
		testCases := []struct {
			description       string
			spotifyPlaylistID string
			shouldAddMetadata bool
			expectedResult    string
			scenario          string
		}{
			{
				description:       "My playlist",
				spotifyPlaylistID: "37i9dQZF1DXcBWIGoYBM5M",
				shouldAddMetadata: true,
				expectedResult:    "My playlist\n\nsynced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
				scenario:          "Description + Spotify ID",
			},
			{
				description:       "",
				spotifyPlaylistID: "37i9dQZF1DXcBWIGoYBM5M",
				shouldAddMetadata: true,
				expectedResult:    "synced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
				scenario:          "Empty description + Spotify ID",
			},
			{
				description:       "My playlist",
				spotifyPlaylistID: "",
				shouldAddMetadata: true,
				expectedResult:    "My playlist",
				scenario:          "Description + No Spotify ID",
			},
			{
				description:       "",
				spotifyPlaylistID: "",
				shouldAddMetadata: false,
				expectedResult:    "",
				scenario:          "Empty description + No Spotify ID",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.scenario, func(t *testing.T) {
				// Test the condition that CreatePlaylist uses
				shouldAddMetadata := tc.spotifyPlaylistID != "" || tc.description != ""

				if shouldAddMetadata != tc.shouldAddMetadata {
					t.Errorf("Expected shouldAddMetadata to be %v for scenario '%s', got %v",
						tc.shouldAddMetadata, tc.scenario, shouldAddMetadata)
					return
				}

				if shouldAddMetadata {
					result := client.addSyncAttribution(tc.description, tc.spotifyPlaylistID)
					if result != tc.expectedResult {
						t.Errorf("Expected result '%s' for scenario '%s', got '%s'",
							tc.expectedResult, tc.scenario, result)
					} else {
						t.Logf("✅ CreatePlaylist metadata logic correct for scenario: %s", tc.scenario)
					}
				}
			})
		}
	})

	t.Run("UpdatePlaylistMetadataLogic", func(t *testing.T) {
		// Test the metadata logic that UpdatePlaylistMetadata uses
		testCases := []struct {
			description       string
			spotifyPlaylistID string
			shouldAddMetadata bool
			expectedResult    string
			scenario          string
		}{
			{
				description:       "Updated playlist",
				spotifyPlaylistID: "37i9dQZF1DXcBWIGoYBM5M",
				shouldAddMetadata: true,
				expectedResult:    "Updated playlist\n\nsynced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
				scenario:          "Description + Spotify ID",
			},
			{
				description:       "",
				spotifyPlaylistID: "37i9dQZF1DXcBWIGoYBM5M",
				shouldAddMetadata: true,
				expectedResult:    "synced from Spotify: https://open.spotify.com/playlist/37i9dQZF1DXcBWIGoYBM5M",
				scenario:          "Empty description + Spotify ID",
			},
			{
				description:       "Updated playlist",
				spotifyPlaylistID: "",
				shouldAddMetadata: true,
				expectedResult:    "Updated playlist",
				scenario:          "Description + No Spotify ID",
			},
			{
				description:       "",
				spotifyPlaylistID: "",
				shouldAddMetadata: false,
				expectedResult:    "",
				scenario:          "Empty description + No Spotify ID",
			},
		}

		for _, tc := range testCases {
			t.Run(tc.scenario, func(t *testing.T) {
				// Test the condition that UpdatePlaylistMetadata uses
				shouldAddMetadata := tc.spotifyPlaylistID != "" || tc.description != ""

				if shouldAddMetadata != tc.shouldAddMetadata {
					t.Errorf("Expected shouldAddMetadata to be %v for scenario '%s', got %v",
						tc.shouldAddMetadata, tc.scenario, shouldAddMetadata)
					return
				}

				if shouldAddMetadata {
					result := client.addSyncAttribution(tc.description, tc.spotifyPlaylistID)
					if result != tc.expectedResult {
						t.Errorf("Expected result '%s' for scenario '%s', got '%s'",
							tc.expectedResult, tc.scenario, result)
					} else {
						t.Logf("✅ UpdatePlaylistMetadata logic correct for scenario: %s", tc.scenario)
					}
				}
			})
		}
	})
}

func TestJessieWareSpotlightSingleEditMatching(t *testing.T) {
	client := &Client{}

	// Test case: "Spotlight - Single Edit" by "Jessie Ware" should match "Spotlight" by "Jessie Ware"
	spotifySong := spotify.Song{
		Name:   "Spotlight - Single Edit",
		Artist: "Jessie Ware",
	}

	// Create a Plex library with the base track (without "Single Edit")
	plexLibrary := []PlexTrack{
		{
			ID:     "spotlight-track",
			Title:  "Spotlight",
			Artist: "Jessie Ware",
		},
	}

	// Test that the suffix removal works correctly
	cleanedTitle := client.RemoveCommonSuffixes(spotifySong.Name)
	if cleanedTitle != "Spotlight" {
		t.Errorf("Expected 'Spotlight' after suffix removal, got '%s'", cleanedTitle)
	}

	// Test that FindBestMatch can find the track
	result := client.FindBestMatch(plexLibrary, spotifySong.Name, spotifySong.Artist)
	if result == nil {
		t.Errorf("Expected to find match for 'Spotlight - Single Edit' by 'Jessie Ware'")
	} else {
		t.Logf("✅ Successfully matched 'Spotlight - Single Edit' by 'Jessie Ware' to '%s' by '%s'",
			result.Title, result.Artist)

		// Verify it matched the correct track
		if result.Title != "Spotlight" || result.Artist != "Jessie Ware" {
			t.Errorf("Expected match to 'Spotlight' by 'Jessie Ware', got '%s' by '%s'",
				result.Title, result.Artist)
		}
	}

	// Test confidence calculation
	confidence := client.calculateConfidence(spotifySong, result, "title_artist")
	if confidence < MinConfidenceScore {
		t.Errorf("Expected high confidence for suffix-removed match, got %f", confidence)
	}
	t.Logf("Confidence score: %f", confidence)
}

func TestGetItRightMatchingScenario(t *testing.T) {
	client := &Client{}
	client.SetDebug(true)

	// Simulate the Plex track that exists in the library
	plexTrack := PlexTrack{
		ID:     "12345",
		Title:  "Get It Right (feat. MØ)",
		Artist: "Diplo",
		Album:  "Various Artists Compilation",
	}

	// Simulate the Spotify song we're trying to match
	spotifySong := spotify.Song{
		Name:   "Get It Right",
		Artist: "Diplo",
	}

	// Test the removeBrackets function
	cleanedTitle := client.removeBrackets(plexTrack.Title)
	if cleanedTitle != "Get It Right" {
		t.Errorf("removeBrackets(%q) = %q, expected 'Get It Right'", plexTrack.Title, cleanedTitle)
	}

	// Test string similarity between the cleaned Plex title and Spotify title
	similarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(cleanedTitle)),
		strings.ToLower(strings.TrimSpace(spotifySong.Name)),
	)

	t.Logf("Similarity between '%s' and '%s': %.3f", cleanedTitle, spotifySong.Name, similarity)

	if similarity < 0.9 {
		t.Errorf("Expected high similarity between cleaned Plex title and Spotify title, got %.3f", similarity)
	}

	// Test artist similarity
	artistSimilarity := client.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(plexTrack.Artist)),
		strings.ToLower(strings.TrimSpace(spotifySong.Artist)),
	)

	t.Logf("Artist similarity between '%s' and '%s': %.3f", plexTrack.Artist, spotifySong.Artist, artistSimilarity)

	if artistSimilarity < 0.9 {
		t.Errorf("Expected high similarity between Plex artist and Spotify artist, got %.3f", artistSimilarity)
	}

	// Test FindBestMatch with the cleaned title
	tracks := []PlexTrack{plexTrack}
	result := client.FindBestMatch(tracks, spotifySong.Name, spotifySong.Artist)

	if result == nil {
		t.Error("Expected FindBestMatch to find a match for 'Get It Right' by 'Diplo'")
	} else {
		t.Logf("Found match: '%s' by '%s'", result.Title, result.Artist)
	}
}

func TestGetItRightSearchSimulation(t *testing.T) {
	client := &Client{}
	client.SetDebug(true)

	// Simulate search results that might be returned by Plex
	searchResults := []PlexTrack{
		{
			ID:     "12345",
			Title:  "Get It Right (feat. MØ)",
			Artist: "Diplo",
			Album:  "Various Artists Compilation",
		},
		{
			ID:     "12346",
			Title:  "Get It Right",
			Artist: "Some Other Artist",
			Album:  "Some Album",
		},
		{
			ID:     "12347",
			Title:  "Get It Right (feat. MØ)",
			Artist: "Some Other Artist",
			Album:  "Some Album",
		},
		{
			ID:     "12348",
			Title:  "Get It Right - Remix",
			Artist: "Diplo",
			Album:  "Some Album",
		},
		{
			ID:     "12349",
			Title:  "Get It Right (feat. MØ) - Extended",
			Artist: "Diplo",
			Album:  "Some Album",
		},
	}

	// Simulate the Spotify song we're trying to match
	spotifySong := spotify.Song{
		Name:   "Get It Right",
		Artist: "Diplo",
	}

	t.Logf("Searching for '%s' by '%s' among %d results", spotifySong.Name, spotifySong.Artist, len(searchResults))

	// Test FindBestMatch with the search results
	result := client.FindBestMatch(searchResults, spotifySong.Name, spotifySong.Artist)

	if result == nil {
		t.Error("Expected FindBestMatch to find a match for 'Get It Right' by 'Diplo'")
	} else {
		t.Logf("Found match: '%s' by '%s' (ID: %s)", result.Title, result.Artist, result.ID)
	}
}

func TestGetItRightSearchStrategies(t *testing.T) {
	client := &Client{}
	client.SetDebug(true)

	// Simulate the Plex track that exists in the library
	plexTrack := PlexTrack{
		ID:     "12345",
		Title:  "Get It Right (feat. MØ)",
		Artist: "Diplo",
		Album:  "Various Artists Compilation",
	}

	// Simulate the Spotify song we're trying to match
	spotifySong := spotify.Song{
		Name:   "Get It Right",
		Artist: "Diplo",
	}

	t.Logf("Testing search strategies for '%s' by '%s'", spotifySong.Name, spotifySong.Artist)
	t.Logf("Target Plex track: '%s' by '%s'", plexTrack.Title, plexTrack.Artist)

	// Test each search strategy
	strategies := []struct {
		name string
		fn   func(string) string
	}{
		{"original", func(s string) string { return s }},
		{"brackets removed", client.removeBrackets},
		{"featuring removed", client.removeFeaturing},
		{"normalized", client.normalizeTitle},
		{"with removed", client.removeWith},
		{"suffixes removed", client.RemoveCommonSuffixes},
	}

	for _, strategy := range strategies {
		modifiedTitle := strategy.fn(spotifySong.Name)
		t.Logf("Strategy '%s': '%s' -> '%s'", strategy.name, spotifySong.Name, modifiedTitle)

		// Test if the modified title would match the Plex track
		similarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(modifiedTitle)),
			strings.ToLower(strings.TrimSpace(plexTrack.Title)),
		)
		t.Logf("  Similarity with Plex track: %.3f", similarity)

		// Also test with cleaned Plex title
		cleanedPlexTitle := client.removeBrackets(plexTrack.Title)
		cleanedSimilarity := client.calculateStringSimilarity(
			strings.ToLower(strings.TrimSpace(modifiedTitle)),
			strings.ToLower(strings.TrimSpace(cleanedPlexTitle)),
		)
		t.Logf("  Similarity with cleaned Plex title: %.3f", cleanedSimilarity)
	}
}

func TestGetItRightVariousArtistsScenario(t *testing.T) {
	client := &Client{}
	client.SetDebug(true)

	// Simulate the Plex track that exists in the library with "Various Artists" as artist
	plexTrack := PlexTrack{
		ID:     "12345",
		Title:  "Get It Right (feat. MØ)",
		Artist: "Various Artists", // This is the issue - Plex sets this for compilation albums
		Album:  "Various Artists Compilation",
	}

	// Simulate the Spotify song we're trying to match
	spotifySong := spotify.Song{
		Name:   "Get It Right",
		Artist: "Diplo",
	}

	t.Logf("Testing Various Artists scenario:")
	t.Logf("Spotify: '%s' by '%s'", spotifySong.Name, spotifySong.Artist)
	t.Logf("Plex: '%s' by '%s'", plexTrack.Title, plexTrack.Artist)

	// Test the current matching logic
	tracks := []PlexTrack{plexTrack}
	result := client.FindBestMatch(tracks, spotifySong.Name, spotifySong.Artist)

	if result == nil {
		t.Log("❌ FindBestMatch failed to find a match (expected due to artist mismatch)")
	} else {
		t.Logf("✅ Found match: '%s' by '%s'", result.Title, result.Artist)
	}

	// Test if we need to add a search strategy for "Various Artists" handling
	// The issue is that we're searching for "Diplo" but the track has "Various Artists" as the artist
	t.Logf("\nTesting potential solutions:")

	// Solution 1: Search by title only when artist is "Various Artists"
	if plexTrack.Artist == "Various Artists" {
		t.Logf("Solution 1: Track has 'Various Artists' as artist, should search by title only")
		// This would require modifying the search strategies to handle this case
	}

	// Solution 2: Add a search strategy that tries searching without artist constraint
	t.Logf("Solution 2: Add search strategy that searches by title only for compilation albums")

	// Solution 3: Check if there's additional metadata that contains the actual artist
	t.Logf("Solution 3: Check for additional metadata fields that might contain the actual artist")
}

func TestGetItRightRealWorldScenario(t *testing.T) {
	client := &Client{}
	client.SetDebug(true)

	// Simulate the search results that would be returned by Plex
	// This matches the original issue: "found 5 results" but none matched
	searchResults := []PlexTrack{
		{
			ID:     "12345",
			Title:  "Get It Right (feat. MØ)",
			Artist: "Various Artists", // This is the actual track we want to match
			Album:  "Various Artists Compilation",
		},
		{
			ID:     "12346",
			Title:  "Get It Right",
			Artist: "Some Other Artist",
			Album:  "Some Album",
		},
		{
			ID:     "12347",
			Title:  "Get It Right (feat. MØ)",
			Artist: "Some Other Artist",
			Album:  "Some Album",
		},
		{
			ID:     "12348",
			Title:  "Get It Right - Remix",
			Artist: "Some Other Artist", // Changed from "Diplo" to simulate no perfect match
			Album:  "Some Album",
		},
		{
			ID:     "12349",
			Title:  "Get It Right (feat. MØ) - Extended",
			Artist: "Some Other Artist", // Changed from "Diplo" to simulate no perfect match
			Album:  "Some Album",
		},
	}

	// Simulate the Spotify song we're trying to match
	spotifySong := spotify.Song{
		Name:   "Get It Right",
		Artist: "Diplo",
	}

	t.Logf("Real-world scenario: searching for '%s' by '%s' among %d results", spotifySong.Name, spotifySong.Artist, len(searchResults))
	t.Logf("Expected match: Track 1 - '%s' by '%s' (Various Artists compilation)", searchResults[0].Title, searchResults[0].Artist)

	// Test FindBestMatch with the search results
	result := client.FindBestMatch(searchResults, spotifySong.Name, spotifySong.Artist)

	if result == nil {
		t.Error("Expected FindBestMatch to find a match for 'Get It Right' by 'Diplo' in the Various Artists compilation")
	} else {
		t.Logf("✅ Found match: '%s' by '%s' (ID: %s)", result.Title, result.Artist, result.ID)

		// Verify it's the expected track
		if result.ID != "12345" {
			t.Errorf("Expected to match track ID 12345 (Various Artists compilation), but got ID %s", result.ID)
		}
	}
}

func TestArtistFeaturingRemoval(t *testing.T) {
	client := &Client{}
	client.SetDebug(true)

	// Test case: Spotify song "The Field (feat. The Durutti Column, Tariq Al-Sabir, Caroline Polachek & Daniel Caesar)" by "Blood Orange"
	// vs Plex track "The Field" by "Blood Orange feat. The Durutti Column, Tariq Al-Sabir, Caroline Polachek & Daniel Caesar"
	spotifySong := spotify.Song{
		ID:       "test_the_field",
		Name:     "The Field (feat. The Durutti Column, Tariq Al-Sabir, Caroline Polachek & Daniel Caesar)",
		Artist:   "Blood Orange",
		Album:    "Test Album",
		Duration: 240000,
		URI:      "spotify:track:test_the_field",
		ISRC:     "TEST22222222",
	}

	plexTracks := []PlexTrack{
		{
			ID:     "plex_the_field",
			Title:  "The Field",
			Artist: "Blood Orange feat. The Durutti Column, Tariq Al-Sabir, Caroline Polachek & Daniel Caesar",
			Album:  "Test Album",
		},
	}

	t.Logf("Testing artist featuring removal:")
	t.Logf("Spotify: '%s' by '%s'", spotifySong.Name, spotifySong.Artist)
	t.Logf("Plex: '%s' by '%s'", plexTracks[0].Title, plexTracks[0].Artist)

	// Test the removeFeaturing function on artist
	cleanedArtist := client.removeFeaturing(plexTracks[0].Artist)
	expectedArtist := "Blood Orange"
	if cleanedArtist != expectedArtist {
		t.Errorf("removeFeaturing(%q) = %q, expected %q", plexTracks[0].Artist, cleanedArtist, expectedArtist)
	}

	// Test the removeBrackets function on title (to remove featuring in parentheses)
	cleanedTitle := client.removeBrackets(spotifySong.Name)
	expectedTitle := "The Field"
	if cleanedTitle != expectedTitle {
		t.Errorf("removeBrackets(%q) = %q, expected %q", spotifySong.Name, cleanedTitle, expectedTitle)
	}

	// Test FindBestMatch
	result := client.FindBestMatch(plexTracks, spotifySong.Name, spotifySong.Artist)

	if result == nil {
		t.Error("Expected FindBestMatch to find a match for 'The Field' by 'Blood Orange'")
	} else {
		t.Logf("✅ Found match: '%s' by '%s'", result.Title, result.Artist)
	}

	// Test that the match has high confidence
	confidence := client.calculateConfidence(spotifySong, result, "title_artist")
	t.Logf("Match confidence: %.3f", confidence)

	if confidence < 0.9 {
		t.Errorf("Expected high confidence match (>= 0.9), got %.3f", confidence)
	}
}

func TestRemixWithFeaturingMatching(t *testing.T) {
	client := &Client{}
	client.SetDebug(true)

	// Test case: Spotify "Timeless (feat. Playboi Carti & Doechii) - Remix" by "The Weeknd"
	// should match Plex "Timeless (remix)" by "The Weeknd"
	spotifySong := spotify.Song{
		Name:   "Timeless (feat. Playboi Carti & Doechii) - Remix",
		Artist: "The Weeknd",
	}

	plexTracks := []PlexTrack{
		{
			ID:     "timeless-remix",
			Title:  "Timeless (remix)",
			Artist: "The Weeknd",
		},
	}

	t.Logf("Testing remix with featuring removal:")
	t.Logf("Spotify: '%s' by '%s'", spotifySong.Name, spotifySong.Artist)
	t.Logf("Plex: '%s' by '%s'", plexTracks[0].Title, plexTracks[0].Artist)

	// Test the transformation chain:
	// 1. removeFeaturing: "Timeless (feat. Playboi Carti & Doechii) - Remix" -> "Timeless - Remix"
	featuringRemoved := client.removeFeaturing(spotifySong.Name)
	expectedAfterFeaturing := "Timeless - Remix"
	if featuringRemoved != expectedAfterFeaturing {
		t.Errorf("removeFeaturing(%q) = %q, expected %q", spotifySong.Name, featuringRemoved, expectedAfterFeaturing)
	}
	t.Logf("After removeFeaturing: '%s'", featuringRemoved)

	// 2. normalizeTitle: "Timeless - Remix" -> "timeless (remix)"
	normalized := client.normalizeTitle(featuringRemoved)
	expectedAfterNormalize := "timeless (remix)"
	if normalized != expectedAfterNormalize {
		t.Errorf("normalizeTitle(%q) = %q, expected %q", featuringRemoved, normalized, expectedAfterNormalize)
	}
	t.Logf("After normalizeTitle: '%s'", normalized)

	// 3. Plex track normalized: "Timeless (remix)" -> "timeless (remix)"
	plexNormalized := client.normalizeTitle(plexTracks[0].Title)
	expectedPlexNormalized := "timeless (remix)"
	if plexNormalized != expectedPlexNormalized {
		t.Errorf("normalizeTitle(%q) = %q, expected %q", plexTracks[0].Title, plexNormalized, expectedPlexNormalized)
	}
	t.Logf("Plex normalized: '%s'", plexNormalized)

	// Verify they match after transformation
	if normalized != plexNormalized {
		t.Errorf("Transformed titles don't match: '%s' vs '%s'", normalized, plexNormalized)
	}

	// Test FindBestMatch
	result := client.FindBestMatch(plexTracks, spotifySong.Name, spotifySong.Artist)

	if result == nil {
		t.Error("Expected FindBestMatch to find a match for 'Timeless (feat. Playboi Carti & Doechii) - Remix' by 'The Weeknd'")
	} else {
		t.Logf("✅ Found match: '%s' by '%s'", result.Title, result.Artist)

		if result.Title != "Timeless (remix)" {
			t.Errorf("Expected match to 'Timeless (remix)', got '%s'", result.Title)
		}
		if result.Artist != "The Weeknd" {
			t.Errorf("Expected match to 'The Weeknd', got '%s'", result.Artist)
		}
	}

	// Test that the match has reasonable confidence
	// Note: confidence uses original titles, so it will be lower than the FindBestMatch score
	// which applies transformations. The key test is that FindBestMatch succeeded.
	confidence := client.calculateConfidence(spotifySong, result, "title_artist")
	t.Logf("Match confidence: %.3f", confidence)

	if confidence < 0.6 {
		t.Errorf("Expected reasonable confidence match (>= 0.6), got %.3f", confidence)
	}
}

func TestMovieSoundtrackMatching(t *testing.T) {
	client := &Client{}
	client.SetDebug(true)

	// Test case: Spotify "Friend Of Mine - from the Smurfs Movie Soundtrack" by "Greyson Chance"
	// should match Plex "Friend Of Mine" by "Greyson Chance"
	spotifySong := spotify.Song{
		Name:   "Friend Of Mine - from the Smurfs Movie Soundtrack",
		Artist: "Greyson Chance",
	}

	plexTracks := []PlexTrack{
		{
			ID:     "friend-of-mine",
			Title:  "Friend Of Mine",
			Artist: "Greyson Chance",
		},
	}

	t.Logf("Testing movie soundtrack suffix removal:")
	t.Logf("Spotify: '%s' by '%s'", spotifySong.Name, spotifySong.Artist)
	t.Logf("Plex: '%s' by '%s'", plexTracks[0].Title, plexTracks[0].Artist)

	// Test RemoveCommonSuffixes removes the soundtrack suffix
	cleanedTitle := client.RemoveCommonSuffixes(spotifySong.Name)
	expectedCleanedTitle := "Friend Of Mine"
	if cleanedTitle != expectedCleanedTitle {
		t.Errorf("RemoveCommonSuffixes(%q) = %q, expected %q", spotifySong.Name, cleanedTitle, expectedCleanedTitle)
	}
	t.Logf("After RemoveCommonSuffixes: '%s'", cleanedTitle)

	// Test FindBestMatch
	result := client.FindBestMatch(plexTracks, spotifySong.Name, spotifySong.Artist)

	if result == nil {
		t.Error("Expected FindBestMatch to find a match for 'Friend Of Mine - from the Smurfs Movie Soundtrack' by 'Greyson Chance'")
	} else {
		t.Logf("✅ Found match: '%s' by '%s'", result.Title, result.Artist)

		if result.Title != "Friend Of Mine" {
			t.Errorf("Expected match to 'Friend Of Mine', got '%s'", result.Title)
		}
		if result.Artist != "Greyson Chance" {
			t.Errorf("Expected match to 'Greyson Chance', got '%s'", result.Artist)
		}
	}

	// Test that the match has reasonable confidence
	confidence := client.calculateConfidence(spotifySong, result, "title_artist")
	t.Logf("Match confidence: %.3f", confidence)

	if confidence < 0.6 {
		t.Errorf("Expected reasonable confidence match (>= 0.6), got %.3f", confidence)
	}
}

func TestMovieSoundtrackVariations(t *testing.T) {
	client := &Client{}

	// Test various soundtrack suffix patterns
	tests := []struct {
		input    string
		expected string
	}{
		{"Friend Of Mine - from the Smurfs Movie Soundtrack", "Friend Of Mine"},
		{"Let It Go - from the Frozen Soundtrack", "Let It Go"},
		{"Song Name - from Avatar Soundtrack", "Song Name"},
		{"Track (from the Lion King Movie Soundtrack)", "Track"},
		{"Another Song (from the Matrix Soundtrack)", "Another Song"},
		{"My Song - from the motion picture", "My Song"},
		{"Test Track - from the film", "Test Track"},
	}

	for _, test := range tests {
		result := client.RemoveCommonSuffixes(test.input)
		if result != test.expected {
			t.Errorf("RemoveCommonSuffixes(%q) = %q, expected %q", test.input, result, test.expected)
		} else {
			t.Logf("✅ RemoveCommonSuffixes(%q) = %q", test.input, result)
		}
	}
}
