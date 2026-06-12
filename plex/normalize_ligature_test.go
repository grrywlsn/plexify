package plex

import (
	"strings"
	"testing"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/track"
)

// Tests for Latin typographic ligatures (œ, Œ, æ, Æ) in normalizeAccents and downstream matching.
// Regression guard: plain ASCII digraphs "oe"/"ae" must stay unchanged; unrelated scripts unchanged.

func TestNormalizeAccents_latinLigaturesAndRegression(t *testing.T) {
	t.Parallel()
	c := &Client{}

	tests := []struct {
		name     string
		input    string
		expected string
	}{
		{"empty", "", ""},
		{"track116_plex_style", "Cœur Stone", "Coeur Stone"},
		{"lowercase_oe_ligature_word", "fœtal", "foetal"},
		{"uppercase_OE_ligature", "ŒDIPE", "OEDIPE"},
		{"ae_ligature", "Æther", "AEther"},
		{"ae_ligature_lower", "æon", "aeon"},
		{"multiple_ligatures", "Œuvre æsthetic", "OEuvre aesthetic"},
		{"ligature_plus_trailing_accent", "Cœur Café", "Coeur Cafe"},
		// Regression: ASCII "oe" is two letters, not U+0153 — must not be altered.
		{"ascii_digraph_unchanged", "coefficient", "coefficient"},
		{"ascii_digraph_unchanged2", "foehammer", "foehammer"},
		// Regression: digits and spaces only.
		{"digits_ascii", "Track 99 oe", "Track 99 oe"},
		{"cyrillic_unchanged", "Русский", "Русский"},
	}

	for _, tt := range tests {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := c.normalizeAccents(tt.input)
			if got != tt.expected {
				t.Fatalf("normalizeAccents(%q) = %q, want %q", tt.input, got, tt.expected)
			}
		})
	}
}

func TestLigatureNormalizedTitles_equalForSimilarity(t *testing.T) {
	t.Parallel()
	c := &Client{}
	// music-social / MB style vs Plex metadata (track 116 scenario)
	a := strings.ToLower(strings.TrimSpace(c.normalizeAccents("Coeur Stone")))
	b := strings.ToLower(strings.TrimSpace(c.normalizeAccents("Cœur Stone")))
	if a != b {
		t.Fatalf("after normalizeAccents+lower, titles must match for scoring: %q vs %q", a, b)
	}
	if sim := c.calculateStringSimilarity(a, b); sim != 1.0 {
		t.Fatalf("similarity want 1.0, got %f for %q vs %q", sim, a, b)
	}
}

func TestLigatureBlendedScore_exceedsDefaultThreshold(t *testing.T) {
	t.Parallel()
	c := &Client{}
	titleSim := c.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(c.normalizeAccents("Coeur Stone"))),
		strings.ToLower(strings.TrimSpace(c.normalizeAccents("Cœur Stone"))),
	)
	artistSim := 1.0
	blended := titleSim*0.7 + artistSim*0.3
	min := defaultMinMatchScore()
	if blended < min {
		t.Fatalf("blended score %f < default threshold %f (titleSim=%f)", blended, min, titleSim)
	}
}

func TestFindBestMatch_latinLigature_asciiSourceUnicodePlex(t *testing.T) {
	t.Parallel()
	defaultPct := config.DefaultMatchConfidencePercent
	c := &Client{matchConfidencePercent: &defaultPct}
	tracks := []PlexTrack{{
		ID:     "plex1",
		Title:  "Cœur Stone",
		Artist: "MIKA",
		Album:  "Cœur Stone",
	}}
	got := c.FindBestMatch(tracks, "Coeur Stone", "MIKA", "Coeur Stone")
	if got == nil || got.ID != "plex1" {
		t.Fatalf("expected Plex œ-title to match source ASCII Coeur Stone, got %+v", got)
	}
}

func TestFindBestMatch_latinLigature_unicodeSourceAsciiPlex(t *testing.T) {
	t.Parallel()
	defaultPct := config.DefaultMatchConfidencePercent
	c := &Client{matchConfidencePercent: &defaultPct}
	tracks := []PlexTrack{{
		ID:     "plex1",
		Title:  "Coeur Stone",
		Artist: "MIKA",
		Album:  "Coeur Stone",
	}}
	got := c.FindBestMatch(tracks, "Cœur Stone", "MIKA", "Cœur Stone")
	if got == nil || got.ID != "plex1" {
		t.Fatalf("expected Plex ASCII title to match source œ spelling, got %+v", got)
	}
}

func TestFindBestMatch_latinLigature_picksCorrectAmongCandidates(t *testing.T) {
	t.Parallel()
	defaultPct := config.DefaultMatchConfidencePercent
	c := &Client{matchConfidencePercent: &defaultPct}
	tracks := []PlexTrack{
		{ID: "wrong", Title: "Coeur Break", Artist: "MIKA", Album: "Other"},
		{ID: "right", Title: "Cœur Stone", Artist: "MIKA", Album: "Cœur Stone"},
	}
	got := c.FindBestMatch(tracks, "Coeur Stone", "MIKA", "Coeur Stone")
	if got == nil || got.ID != "right" {
		t.Fatalf("expected id right, got %+v", got)
	}
}

func TestCalculateConfidence_ligatureTitleMIKA(t *testing.T) {
	t.Parallel()
	c := &Client{}
	song := track.Track{Name: "Coeur Stone", Artist: "MIKA", Album: "Coeur Stone"}
	plex := &PlexTrack{Title: "Cœur Stone", Artist: "MIKA", Album: "Cœur Stone"}
	conf := c.calculateConfidence(song, plex, MatchTypeTitleArtist)
	if conf < defaultMinMatchScore() {
		t.Fatalf("confidence %f < default min %f for Coeur vs Cœur + MIKA", conf, defaultMinMatchScore())
	}
}
