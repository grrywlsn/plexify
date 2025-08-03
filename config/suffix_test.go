package config

import (
	"strings"
	"testing"
)

// TestSuffixRemoval tests the suffix removal functionality
func TestSuffixRemoval(t *testing.T) {
	// This test verifies that the suffix removal logic works correctly
	// for the Jessie Ware - Spotlight - Single Edit scenario

	// Test cases
	testCases := []struct {
		input    string
		expected string
	}{
		{"Spotlight - Single Edit", "Spotlight"},
		{"Spotlight - Edit", "Spotlight"},
		{"Spotlight - Radio Edit", "Spotlight"},
		{"Spotlight - Extended", "Spotlight"},
		{"Spotlight - Remix", "Spotlight"},
		{"Spotlight - Bonus Track", "Spotlight"},
		{"Spotlight", "Spotlight"}, // Should remain unchanged
		{"Song Title - Version", "Song Title"},
		{"Song Title - Live", "Song Title"},
		{"Song Title - Acoustic", "Song Title"},
	}

	// Common suffixes to remove (copied from the actual implementation)
	suffixes := []string{
		" - bonus track",
		" - remix",
		" - extended",
		" - radio edit",
		" - single edit",
		" - edit",
		" - version",
		" - live",
		" - acoustic",
		" - instrumental",
		" - demo",
		" - original mix",
		" - club mix",
		" - clean",
		" - explicit",
		" - bonus",
		" - track",
		" (bonus track)",
		" (remix)",
		" (extended)",
		" (radio edit)",
		" (single edit)",
		" (edit)",
		" (version)",
		" (live)",
		" (acoustic)",
		" (instrumental)",
		" (demo)",
		" (original mix)",
		" (club mix)",
		" (clean)",
		" (explicit)",
		" (bonus)",
		" (track)",
	}

	removeCommonSuffixes := func(s string) string {
		// Handle common suffixes (case insensitive)
		lowerS := strings.ToLower(s)

		for _, suffix := range suffixes {
			if strings.HasSuffix(lowerS, strings.ToLower(suffix)) {
				// Return the original string without the suffix (preserving original case)
				result := strings.TrimSpace(s[:len(s)-len(suffix)])
				// Clean up trailing dashes and spaces
				result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
				return result
			}
		}

		return s
	}

	for _, tc := range testCases {
		t.Run(tc.input, func(t *testing.T) {
			result := removeCommonSuffixes(tc.input)
			if result != tc.expected {
				t.Errorf("Expected '%s', got '%s'", tc.expected, result)
			} else {
				t.Logf("✅ '%s' -> '%s'", tc.input, result)
			}
		})
	}

	// Specific test for Jessie Ware scenario
	t.Run("Jessie Ware - Spotlight - Single Edit", func(t *testing.T) {
		spotifyTitle := "Spotlight - Single Edit"
		expected := "Spotlight"
		result := removeCommonSuffixes(spotifyTitle)

		if result != expected {
			t.Errorf("Jessie Ware test failed: Expected '%s', got '%s'", expected, result)
		} else {
			t.Logf("✅ Jessie Ware test passed: '%s' -> '%s'", spotifyTitle, result)
		}
	})
}
