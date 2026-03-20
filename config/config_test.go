package config

import (
	"os"
	"strings"
	"testing"
)

func TestConfigValidation(t *testing.T) {
	cfg := &Config{}

	err := cfg.validate()
	if err == nil {
		t.Error("Expected validation to fail with empty config")
	}

	errorMsg := err.Error()
	if !strings.Contains(errorMsg, "MUSIC_SOCIAL_URL") {
		t.Error("Expected error message to mention MUSIC_SOCIAL_URL")
	}
	if !strings.Contains(errorMsg, "PLEX_URL") {
		t.Error("Expected error message to mention PLEX_URL")
	}

	cfg = &Config{
		MusicSocial: MusicSocialConfig{
			BaseURL:     "https://music.example.com",
			PlaylistIDs: []string{"pl_abc"},
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

	cfg.MusicSocial.PlaylistIDs = []string{}
	cfg.MusicSocial.Username = "user"
	err = cfg.validate()
	if err != nil {
		t.Errorf("Expected no validation error with username, got %v", err)
	}

	cfg.MusicSocial.BaseURL = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing base URL")
	}

	cfg.MusicSocial.BaseURL = "https://music.example.com"
	cfg.MusicSocial.Username = ""
	cfg.MusicSocial.PlaylistIDs = []string{}
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing playlist source")
	}

	cfg.MusicSocial.Username = "user"
	cfg.Plex.URL = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing Plex URL")
	}

	cfg.Plex.URL = "http://test.plex.server:32400"
	cfg.Plex.Token = ""
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing Plex Token")
	}

	cfg.Plex.Token = "test_token"
	cfg.Plex.LibrarySectionID = 0
	err = cfg.validate()
	if err == nil {
		t.Error("Expected validation error for missing LibrarySectionID")
	}
}

func TestConfigHierarchy(t *testing.T) {
	os.Setenv("MUSIC_SOCIAL_URL", "https://env-music.example.com")
	os.Setenv("PLEX_URL", "http://test:32400")
	os.Setenv("PLEX_TOKEN", "test_token")
	os.Setenv("PLEX_LIBRARY_SECTION_ID", "1")
	os.Setenv("MUSIC_SOCIAL_USERNAME", "env_user")
	defer func() {
		os.Unsetenv("MUSIC_SOCIAL_URL")
		os.Unsetenv("PLEX_URL")
		os.Unsetenv("PLEX_TOKEN")
		os.Unsetenv("PLEX_LIBRARY_SECTION_ID")
		os.Unsetenv("MUSIC_SOCIAL_USERNAME")
	}()

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Failed to load config: %v", err)
	}

	if cfg.MusicSocial.Username != "env_user" {
		t.Errorf("Expected username 'env_user', got '%s'", cfg.MusicSocial.Username)
	}

	overrides := map[string]string{
		"MUSIC_SOCIAL_USERNAME": "cli_user",
	}

	cfgWithOverrides, err := LoadWithOverrides(overrides)
	if err != nil {
		t.Fatalf("Failed to load config with overrides: %v", err)
	}

	if cfgWithOverrides.MusicSocial.Username != "cli_user" {
		t.Errorf("Expected username 'cli_user' after CLI override, got '%s'", cfgWithOverrides.MusicSocial.Username)
	}

	multipleOverrides := map[string]string{
		"MUSIC_SOCIAL_USERNAME":   "cli_user2",
		"PLEX_LIBRARY_SECTION_ID": "5",
		"PLEX_URL":                "http://test2:32400",
	}

	cfgMultiple, err := LoadWithOverrides(multipleOverrides)
	if err != nil {
		t.Fatalf("Failed to load config with overrides: %v", err)
	}

	if cfgMultiple.MusicSocial.Username != "cli_user2" {
		t.Errorf("Expected username 'cli_user2', got '%s'", cfgMultiple.MusicSocial.Username)
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
		MusicSocial: MusicSocialConfig{
			Username: "original_user",
		},
		Plex: PlexConfig{
			LibrarySectionID: 1,
		},
	}

	overrides := map[string]string{
		"MUSIC_SOCIAL_USERNAME":   "new_user",
		"PLEX_LIBRARY_SECTION_ID": "10",
		"PLEX_URL":                "http://override:32400",
	}

	cfg.applyOverrides(overrides)

	if cfg.MusicSocial.Username != "new_user" {
		t.Errorf("Expected username 'new_user', got '%s'", cfg.MusicSocial.Username)
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

	if cfg.MusicSocial.BaseURL != "" {
		t.Errorf("Expected empty BaseURL, got '%s'", cfg.MusicSocial.BaseURL)
	}
	if cfg.MusicSocial.Username != "" {
		t.Errorf("Expected empty Username, got '%s'", cfg.MusicSocial.Username)
	}
	if cfg.MusicSocial.PlaylistIDs != nil {
		t.Errorf("Expected nil PlaylistIDs, got %v", cfg.MusicSocial.PlaylistIDs)
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
	if cfg.Plex.MaxRequestsPerSecond != 4 {
		t.Errorf("Expected MaxRequestsPerSecond 4, got %v", cfg.Plex.MaxRequestsPerSecond)
	}
	if cfg.Plex.MatchConfidencePercent != DefaultMatchConfidencePercent {
		t.Errorf("Expected MatchConfidencePercent %d, got %d", DefaultMatchConfidencePercent, cfg.Plex.MatchConfidencePercent)
	}
	if !cfg.Plex.InsecureSkipVerify {
		t.Error("Expected InsecureSkipVerify true by default")
	}
}

func TestLoadFromOSEnv(t *testing.T) {
	cfg := &Config{}
	cfg.initializeDefaults()

	os.Setenv("MUSIC_SOCIAL_URL", "https://ms.example.com")
	os.Setenv("MUSIC_SOCIAL_USERNAME", "test_user")
	os.Setenv("PLEX_URL", "http://test:32400")
	defer func() {
		os.Unsetenv("MUSIC_SOCIAL_URL")
		os.Unsetenv("MUSIC_SOCIAL_USERNAME")
		os.Unsetenv("PLEX_URL")
	}()

	cfg.loadFromOSEnv()

	if cfg.MusicSocial.BaseURL != "https://ms.example.com" {
		t.Errorf("Expected BaseURL 'https://ms.example.com', got '%s'", cfg.MusicSocial.BaseURL)
	}
	if cfg.MusicSocial.Username != "test_user" {
		t.Errorf("Expected Username 'test_user', got '%s'", cfg.MusicSocial.Username)
	}
	if cfg.Plex.URL != "http://test:32400" {
		t.Errorf("Expected URL 'http://test:32400', got '%s'", cfg.Plex.URL)
	}
}

func TestLoadExactMatchesOnlyFromEnv(t *testing.T) {
	defer os.Unsetenv("PLEXIFY_EXACT_MATCHES_ONLY")

	cfg := &Config{}
	cfg.initializeDefaults()
	os.Setenv("PLEXIFY_EXACT_MATCHES_ONLY", "true")
	cfg.loadFromOSEnv()
	if !cfg.Plex.ExactMatchesOnly {
		t.Error("expected ExactMatchesOnly true")
	}
}

func TestLoadPlexMaxRequestsPerSecondFromEnv(t *testing.T) {
	defer os.Unsetenv("PLEX_MAX_REQUESTS_PER_SECOND")

	cfg := &Config{}
	cfg.initializeDefaults()
	os.Setenv("PLEX_MAX_REQUESTS_PER_SECOND", "0")
	cfg.loadFromOSEnv()
	if cfg.Plex.MaxRequestsPerSecond != 0 {
		t.Errorf("expected 0 (unlimited), got %v", cfg.Plex.MaxRequestsPerSecond)
	}

	cfg.initializeDefaults()
	os.Setenv("PLEX_MAX_REQUESTS_PER_SECOND", "8.5")
	cfg.loadFromOSEnv()
	if cfg.Plex.MaxRequestsPerSecond != 8.5 {
		t.Errorf("expected 8.5, got %v", cfg.Plex.MaxRequestsPerSecond)
	}
}

func TestApplyOverridesEmptyValues(t *testing.T) {
	cfg := &Config{
		MusicSocial: MusicSocialConfig{
			Username: "original_user",
		},
		Plex: PlexConfig{
			LibrarySectionID: 1,
		},
	}

	overrides := map[string]string{
		"MUSIC_SOCIAL_USERNAME":   "",
		"PLEX_LIBRARY_SECTION_ID": "10",
		"PLEX_URL":                "",
	}

	cfg.applyOverrides(overrides)

	if cfg.MusicSocial.Username != "original_user" {
		t.Errorf("Expected username 'original_user' (unchanged), got '%s'", cfg.MusicSocial.Username)
	}

	if cfg.Plex.LibrarySectionID != 10 {
		t.Errorf("Expected library section ID 10, got %d", cfg.Plex.LibrarySectionID)
	}

	if cfg.Plex.URL != "" {
		t.Errorf("Expected empty URL (unchanged), got '%s'", cfg.Plex.URL)
	}
}

func TestParseMatchConfidencePercent(t *testing.T) {
	t.Parallel()
	tests := []struct {
		raw  string
		want int
		fail bool
	}{
		{"80", 80, false},
		{" 75% ", 75, false},
		{"0", 0, false},
		{"100", 100, false},
		{"101", 0, true},
		{"-1", 0, true},
		{"abc", 0, true},
		{"", 0, true},
	}
	for _, tt := range tests {
		got, err := ParseMatchConfidencePercent(tt.raw)
		if tt.fail {
			if err == nil {
				t.Errorf("ParseMatchConfidencePercent(%q): expected error", tt.raw)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseMatchConfidencePercent(%q): %v", tt.raw, err)
			continue
		}
		if got != tt.want {
			t.Errorf("ParseMatchConfidencePercent(%q) = %d, want %d", tt.raw, got, tt.want)
		}
	}
}

func TestApplyOverridesMatchConfidencePercent(t *testing.T) {
	cfg := &Config{}
	cfg.initializeDefaults()
	cfg.applyOverrides(map[string]string{"PLEXIFY_MATCH_CONFIDENCE_PERCENT": "65"})
	if cfg.Plex.MatchConfidencePercent != 65 {
		t.Fatalf("got %d", cfg.Plex.MatchConfidencePercent)
	}
	cfg.applyOverrides(map[string]string{"PLEXIFY_MATCH_CONFIDENCE_PERCENT": "not-a-number"})
	if cfg.Plex.MatchConfidencePercent != 65 {
		t.Fatalf("invalid override should not change value, got %d", cfg.Plex.MatchConfidencePercent)
	}
}

func TestNormalizeMusicSocialBaseURL(t *testing.T) {
	got, err := NormalizeMusicSocialBaseURL("  https://example.com/path/  ")
	if err != nil {
		t.Fatal(err)
	}
	if got != "https://example.com/path" {
		t.Errorf("got %q", got)
	}

	if _, err := NormalizeMusicSocialBaseURL("ftp://x"); err == nil {
		t.Error("expected error for non-http scheme")
	}
}
