package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/joho/godotenv"
)

// Config holds all configuration values
type Config struct {
	MusicSocial MusicSocialConfig
	Plex        PlexConfig
}

// MusicSocialConfig holds music-social HTTP API configuration
type MusicSocialConfig struct {
	BaseURL             string
	Username            string   // List public playlists for this user
	PlaylistIDs         []string // Explicit playlist IDs (comma-separated in env)
	ExcludedPlaylistIDs []string // Playlist IDs to skip
}

// PlexConfig holds Plex server configuration
type PlexConfig struct {
	URL              string
	Token            string
	LibrarySectionID int
	ServerID         string
	// InsecureSkipVerify disables TLS certificate verification for Plex HTTPS (default true for typical LAN/self-signed setups). Set PLEX_VERIFY_TLS=true for strict verification.
	InsecureSkipVerify    bool
	MatchConcurrency      int  // parallel Plex track lookups (1 = sequential, max 32)
	DryRun                bool // if true, do not create/update/clear/add playlist items on Plex
	SkipFullLibrarySearch bool // if true, do not call /library/sections/{id}/all as last resort (PLEXIFY_FAST_SEARCH)
	// ExactMatchesOnly uses only the first Plex search strategy (raw title/artist) and skips full-library scan (PLEXIFY_EXACT_MATCHES_ONLY).
	ExactMatchesOnly bool
	// MaxRequestsPerSecond caps outbound Plex HTTP requests (token bucket, burst 1). Default 4; 0 = unlimited (PLEX_MAX_REQUESTS_PER_SECOND).
	MaxRequestsPerSecond float64
	// MatchConfidencePercent is the minimum combined title/artist match score (0–100) required to accept a Plex track. Default 80 (PLEXIFY_MATCH_CONFIDENCE_PERCENT).
	MatchConfidencePercent int
}

// Load loads configuration following the specified order:
// 1. Start with empty values
// 2. Load from OS environment variables (only if they exist)
// 3. Load from .env file in the process working directory (optional convenience; not persisted app state)
// 4. Apply CLI flag overrides (only if they exist)
func Load() (*Config, error) {
	config := &Config{}

	config.initializeDefaults()
	config.loadFromOSEnv()
	config.loadFromEnvFile()
	if err := applyMatchConfidencePercentFromEnv(config); err != nil {
		return nil, err
	}

	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

// LoadWithOverrides loads configuration and applies CLI flag overrides
func LoadWithOverrides(overrides map[string]string) (*Config, error) {
	config := &Config{}

	config.initializeDefaults()
	config.loadFromOSEnv()
	config.loadFromEnvFile()
	if err := applyMatchConfidencePercentFromEnv(config); err != nil {
		return nil, err
	}
	config.applyOverrides(overrides)

	if err := config.validate(); err != nil {
		return nil, err
	}

	return config, nil
}

func (c *Config) initializeDefaults() {
	c.MusicSocial = MusicSocialConfig{
		BaseURL:             "",
		Username:            "",
		PlaylistIDs:         nil,
		ExcludedPlaylistIDs: nil,
	}

	c.Plex = PlexConfig{
		URL:                    "",
		Token:                  "",
		LibrarySectionID:       0,
		ServerID:               "",
		InsecureSkipVerify:     true,
		MatchConcurrency:       1,
		DryRun:                 false,
		SkipFullLibrarySearch:  false,
		ExactMatchesOnly:       false,
		MaxRequestsPerSecond:   4,
		MatchConfidencePercent: DefaultMatchConfidencePercent,
	}
}

// DefaultMatchConfidencePercent is the default minimum match score (whole percent) when PLEXIFY_MATCH_CONFIDENCE_PERCENT is unset.
const DefaultMatchConfidencePercent = 80

func (c *Config) loadFromOSEnv() {
	if value := os.Getenv("MUSIC_SOCIAL_URL"); value != "" {
		c.MusicSocial.BaseURL = value
	}
	if value := os.Getenv("MUSIC_SOCIAL_USERNAME"); value != "" {
		c.MusicSocial.Username = value
	}
	if value := os.Getenv("MUSIC_SOCIAL_PLAYLIST_ID"); value != "" {
		c.MusicSocial.PlaylistIDs = parseCommaSeparatedList(value)
	}
	if value := os.Getenv("MUSIC_SOCIAL_PLAYLIST_EXCLUDED_ID"); value != "" {
		c.MusicSocial.ExcludedPlaylistIDs = parseCommaSeparatedList(value)
	}

	if value := os.Getenv("PLEX_URL"); value != "" {
		c.Plex.URL = value
	}
	if value := os.Getenv("PLEX_TOKEN"); value != "" {
		c.Plex.Token = value
	}
	if value := os.Getenv("PLEX_LIBRARY_SECTION_ID"); value != "" {
		if sectionID, err := parseLibrarySectionID(value); err == nil {
			c.Plex.LibrarySectionID = sectionID
		}
	}
	if value := os.Getenv("PLEX_SERVER_ID"); value != "" {
		c.Plex.ServerID = value
	}
	c.loadPlexTLSFromEnv()
	if parseBoolEnv("PLEXIFY_DRY_RUN") || parseBoolEnv("DRY_RUN") {
		c.Plex.DryRun = true
	}
	if n, ok := parseIntEnv("PLEX_MATCH_CONCURRENCY"); ok {
		c.Plex.MatchConcurrency = n
	}
	if parseBoolEnv("PLEXIFY_FAST_SEARCH") || parseBoolEnv("PLEX_SKIP_FULL_LIBRARY_SEARCH") {
		c.Plex.SkipFullLibrarySearch = true
	}
	if parseBoolEnv("PLEXIFY_EXACT_MATCHES_ONLY") {
		c.Plex.ExactMatchesOnly = true
	}
	if f, ok := parseFloatEnv("PLEX_MAX_REQUESTS_PER_SECOND"); ok {
		c.Plex.MaxRequestsPerSecond = f
	}
}

func (c *Config) loadFromEnvFile() {
	if err := godotenv.Load(); err != nil {
		return
	}

	if value := os.Getenv("MUSIC_SOCIAL_URL"); value != "" {
		c.MusicSocial.BaseURL = value
	}
	if value := os.Getenv("MUSIC_SOCIAL_USERNAME"); value != "" {
		c.MusicSocial.Username = value
	}
	if value := os.Getenv("MUSIC_SOCIAL_PLAYLIST_ID"); value != "" {
		c.MusicSocial.PlaylistIDs = parseCommaSeparatedList(value)
	}
	if value := os.Getenv("MUSIC_SOCIAL_PLAYLIST_EXCLUDED_ID"); value != "" {
		c.MusicSocial.ExcludedPlaylistIDs = parseCommaSeparatedList(value)
	}

	if value := os.Getenv("PLEX_URL"); value != "" {
		c.Plex.URL = value
	}
	if value := os.Getenv("PLEX_TOKEN"); value != "" {
		c.Plex.Token = value
	}
	if value := os.Getenv("PLEX_LIBRARY_SECTION_ID"); value != "" {
		if sectionID, err := parseLibrarySectionID(value); err == nil {
			c.Plex.LibrarySectionID = sectionID
		}
	}
	if value := os.Getenv("PLEX_SERVER_ID"); value != "" {
		c.Plex.ServerID = value
	}
	c.loadPlexTLSFromEnv()
	if parseBoolEnv("PLEXIFY_DRY_RUN") || parseBoolEnv("DRY_RUN") {
		c.Plex.DryRun = true
	}
	if n, ok := parseIntEnv("PLEX_MATCH_CONCURRENCY"); ok {
		c.Plex.MatchConcurrency = n
	}
	if parseBoolEnv("PLEXIFY_FAST_SEARCH") || parseBoolEnv("PLEX_SKIP_FULL_LIBRARY_SEARCH") {
		c.Plex.SkipFullLibrarySearch = true
	}
	if parseBoolEnv("PLEXIFY_EXACT_MATCHES_ONLY") {
		c.Plex.ExactMatchesOnly = true
	}
	if f, ok := parseFloatEnv("PLEX_MAX_REQUESTS_PER_SECOND"); ok {
		c.Plex.MaxRequestsPerSecond = f
	}
}

// loadPlexTLSFromEnv applies PLEX_INSECURE_SKIP_VERIFY and PLEX_VERIFY_TLS (VERIFY wins when truthy).
func (c *Config) loadPlexTLSFromEnv() {
	if v, ok := os.LookupEnv("PLEX_INSECURE_SKIP_VERIFY"); ok {
		c.Plex.InsecureSkipVerify = isTruthy(v)
	}
	if parseBoolEnv("PLEX_VERIFY_TLS") {
		c.Plex.InsecureSkipVerify = false
	}
}

func parseBoolEnv(key string) bool {
	return isTruthy(os.Getenv(key))
}

func isTruthy(s string) bool {
	v := strings.TrimSpace(strings.ToLower(s))
	switch v {
	case "1", "true", "yes", "on":
		return true
	default:
		return false
	}
}

func parseIntEnv(key string) (int, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0, false
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, false
	}
	return n, true
}

func parseFloatEnv(key string) (float64, bool) {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return 0, false
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0, false
	}
	return f, true
}

func parseCommaSeparatedList(input string) []string {
	if input == "" {
		return nil
	}

	items := strings.Split(input, ",")
	for i, item := range items {
		items[i] = strings.TrimSpace(item)
	}

	return items
}

func parseLibrarySectionID(value string) (int, error) {
	if value == "0" || value == "your_music_library_section_id" {
		return 0, nil
	}

	sectionID, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("invalid section ID '%s': %w", value, err)
	}

	return sectionID, nil
}

// NormalizeMusicSocialBaseURL trims space and trailing slashes and validates as an absolute http(s) URL.
func NormalizeMusicSocialBaseURL(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimRight(s, "/")
	if s == "" {
		return "", fmt.Errorf("empty base URL")
	}
	u, err := url.Parse(s)
	if err != nil {
		return "", fmt.Errorf("invalid base URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", fmt.Errorf("base URL must use http or https")
	}
	if u.Host == "" {
		return "", fmt.Errorf("base URL must include a host")
	}
	return s, nil
}

func (c *Config) validate() error {
	var missingFields []string

	if strings.TrimSpace(c.MusicSocial.BaseURL) == "" {
		missingFields = append(missingFields, "MUSIC_SOCIAL_URL")
	} else {
		base, err := NormalizeMusicSocialBaseURL(c.MusicSocial.BaseURL)
		if err != nil {
			return fmt.Errorf("invalid MUSIC_SOCIAL_URL: %w", err)
		}
		c.MusicSocial.BaseURL = base
	}

	if c.Plex.URL == "" {
		missingFields = append(missingFields, "PLEX_URL")
	}
	if c.Plex.Token == "" {
		missingFields = append(missingFields, "PLEX_TOKEN")
	}
	if c.Plex.LibrarySectionID == 0 {
		missingFields = append(missingFields, "PLEX_LIBRARY_SECTION_ID")
	}

	if c.MusicSocial.Username == "" && len(c.MusicSocial.PlaylistIDs) == 0 {
		missingFields = append(missingFields, "MUSIC_SOCIAL_USERNAME or MUSIC_SOCIAL_PLAYLIST_ID")
	}

	if len(missingFields) > 0 {
		return fmt.Errorf("missing required configuration values:\n%s\n\nSet these values via environment variables, .env file, or CLI flags", strings.Join(missingFields, "\n"))
	}

	if err := validateMatchConfidencePercent(c.Plex.MatchConfidencePercent); err != nil {
		return err
	}

	c.normalizePlexRuntime()
	return nil
}

func (c *Config) normalizePlexRuntime() {
	if c.Plex.MatchConcurrency < 1 {
		c.Plex.MatchConcurrency = 1
	}
	if c.Plex.MatchConcurrency > 32 {
		c.Plex.MatchConcurrency = 32
	}
	if c.Plex.MaxRequestsPerSecond < 0 {
		c.Plex.MaxRequestsPerSecond = 0
	}
	if c.Plex.MaxRequestsPerSecond > 10000 {
		c.Plex.MaxRequestsPerSecond = 10000
	}
}

func (c *Config) applyOverrides(overrides map[string]string) {
	for key, value := range overrides {
		if value == "" {
			continue
		}

		switch key {
		case "MUSIC_SOCIAL_URL":
			c.MusicSocial.BaseURL = value
		case "MUSIC_SOCIAL_USERNAME":
			c.MusicSocial.Username = value
		case "MUSIC_SOCIAL_PLAYLIST_ID":
			c.MusicSocial.PlaylistIDs = parseCommaSeparatedList(value)
		case "MUSIC_SOCIAL_PLAYLIST_EXCLUDED_ID":
			c.MusicSocial.ExcludedPlaylistIDs = parseCommaSeparatedList(value)
		case "PLEX_URL":
			c.Plex.URL = value
		case "PLEX_TOKEN":
			c.Plex.Token = value
		case "PLEX_LIBRARY_SECTION_ID":
			if sectionID, err := parseLibrarySectionID(value); err == nil {
				c.Plex.LibrarySectionID = sectionID
			}
		case "PLEX_SERVER_ID":
			c.Plex.ServerID = value
		case "PLEXIFY_DRY_RUN", "DRY_RUN":
			if isTruthy(value) {
				c.Plex.DryRun = true
			}
		case "PLEX_MATCH_CONCURRENCY":
			if n, err := strconv.Atoi(value); err == nil {
				c.Plex.MatchConcurrency = n
			}
		case "PLEXIFY_FAST_SEARCH", "PLEX_SKIP_FULL_LIBRARY_SEARCH":
			if isTruthy(value) {
				c.Plex.SkipFullLibrarySearch = true
			}
		case "PLEXIFY_EXACT_MATCHES_ONLY":
			if isTruthy(value) {
				c.Plex.ExactMatchesOnly = true
			}
		case "PLEX_MAX_REQUESTS_PER_SECOND":
			if f, err := strconv.ParseFloat(strings.TrimSpace(value), 64); err == nil {
				c.Plex.MaxRequestsPerSecond = f
			}
		case "PLEXIFY_MATCH_CONFIDENCE_PERCENT":
			if p, err := ParseMatchConfidencePercent(value); err == nil {
				c.Plex.MatchConfidencePercent = p
			}
		}
	}
	c.applyPlexTLSOverrides(overrides)
}

func (c *Config) applyPlexTLSOverrides(overrides map[string]string) {
	if v, ok := overrides["PLEX_INSECURE_SKIP_VERIFY"]; ok && v != "" {
		c.Plex.InsecureSkipVerify = isTruthy(v)
	}
	if v, ok := overrides["PLEX_VERIFY_TLS"]; ok && v != "" && isTruthy(v) {
		c.Plex.InsecureSkipVerify = false
	}
}

// applyMatchConfidencePercentFromEnv sets Plex.MatchConfidencePercent from PLEXIFY_MATCH_CONFIDENCE_PERCENT when the variable is set to a non-empty value.
func applyMatchConfidencePercentFromEnv(c *Config) error {
	v, ok := os.LookupEnv("PLEXIFY_MATCH_CONFIDENCE_PERCENT")
	if !ok {
		return nil
	}
	s := strings.TrimSpace(v)
	if s == "" {
		return nil
	}
	p, err := ParseMatchConfidencePercent(s)
	if err != nil {
		return fmt.Errorf("invalid PLEXIFY_MATCH_CONFIDENCE_PERCENT: %w", err)
	}
	c.Plex.MatchConfidencePercent = p
	return nil
}

// ParseMatchConfidencePercent parses an integer 0–100, with optional trailing "%".
func ParseMatchConfidencePercent(raw string) (int, error) {
	s := strings.TrimSpace(raw)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimSpace(s)
	if s == "" {
		return 0, fmt.Errorf("empty value")
	}
	p, err := strconv.Atoi(s)
	if err != nil {
		return 0, fmt.Errorf("must be an integer 0–100, got %q", raw)
	}
	if err := validateMatchConfidencePercent(p); err != nil {
		return 0, err
	}
	return p, nil
}

func validateMatchConfidencePercent(p int) error {
	if p < 0 || p > 100 {
		return fmt.Errorf("must be between 0 and 100 inclusive, got %d", p)
	}
	return nil
}
