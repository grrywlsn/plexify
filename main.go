package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"strconv"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/internal/app"
)

// Version is set via: -ldflags "-X main.version=1.2.3"
var version = "dev"

var debugMode bool

func parseFlags() map[string]string {
	overrides := make(map[string]string)

	var musicSocialURL, musicSocialUsername, musicSocialPlaylistID, musicSocialPlaylistExcludedID string
	flag.StringVar(&musicSocialURL, "MUSIC_SOCIAL_URL", "", "music-social.com API base URL (default https://music-social.com; overrides env)")
	flag.StringVar(&musicSocialUsername, "MUSIC_SOCIAL_USERNAME", "", "Username whose public playlists to sync (overrides env var)")
	flag.StringVar(&musicSocialPlaylistID, "MUSIC_SOCIAL_PLAYLIST_ID", "", "Comma-separated playlist IDs (overrides env var)")
	flag.StringVar(&musicSocialPlaylistExcludedID, "MUSIC_SOCIAL_PLAYLIST_EXCLUDED_ID", "", "Comma-separated playlist IDs to exclude (overrides env var)")

	var plexURL, plexToken, plexLibrarySectionID, plexServerID string
	flag.StringVar(&plexURL, "PLEX_URL", "", "Plex server URL (overrides env var)")
	flag.StringVar(&plexToken, "PLEX_TOKEN", "", "Plex authentication token (overrides env var)")
	flag.StringVar(&plexLibrarySectionID, "PLEX_LIBRARY_SECTION_ID", "", "Plex library section ID (overrides env var)")
	flag.StringVar(&plexServerID, "PLEX_SERVER_ID", "", "Plex server ID (overrides env var)")

	var dryRun bool
	flag.BoolVar(&dryRun, "dry-run", false, "Match and show diff only; do not modify Plex playlists (same as PLEXIFY_DRY_RUN=true)")

	var plexMatchConcurrency int
	flag.IntVar(&plexMatchConcurrency, "plex-match-concurrency", 0, "Parallel Plex track lookups (1–32); 0 uses PLEX_MATCH_CONCURRENCY or default 1")

	var plexInsecureTLS bool
	flag.BoolVar(&plexInsecureTLS, "plex-insecure-tls", false, "Skip TLS certificate verification for Plex HTTPS (default on; same as PLEX_INSECURE_SKIP_VERIFY=true)")

	var plexVerifyTLS bool
	flag.BoolVar(&plexVerifyTLS, "plex-verify-tls", false, "Verify Plex HTTPS certificates (same as PLEX_VERIFY_TLS=true; overrides insecure default)")

	var plexFastSearch bool
	flag.BoolVar(&plexFastSearch, "plex-fast-search", false, "Skip full-library Plex scan; use indexed /search only (same as PLEXIFY_FAST_SEARCH=true)")

	var exactMatchesOnly bool
	flag.BoolVar(&exactMatchesOnly, "exact-matches-only", false, "Match using raw title/artist only (first strategy); skip normalizations and full-library scan (PLEXIFY_EXACT_MATCHES_ONLY)")

	var plexMaxRPS float64
	flag.Float64Var(&plexMaxRPS, "plex-max-rps", -1, "Max Plex HTTP requests per second (0 = unlimited; negative uses PLEX_MAX_REQUESTS_PER_SECOND or default 4)")

	flag.BoolVar(&debugMode, "DEBUG", false, "Enable debug output")

	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "Show version information")

	flag.Parse()

	if showVersion {
		fmt.Printf("Plexify version %s\n", version)
		os.Exit(0)
	}

	if musicSocialURL != "" {
		overrides["MUSIC_SOCIAL_URL"] = musicSocialURL
	}
	if musicSocialUsername != "" {
		overrides["MUSIC_SOCIAL_USERNAME"] = musicSocialUsername
	}
	if musicSocialPlaylistID != "" {
		overrides["MUSIC_SOCIAL_PLAYLIST_ID"] = musicSocialPlaylistID
	}
	if musicSocialPlaylistExcludedID != "" {
		overrides["MUSIC_SOCIAL_PLAYLIST_EXCLUDED_ID"] = musicSocialPlaylistExcludedID
	}
	if plexURL != "" {
		overrides["PLEX_URL"] = plexURL
	}
	if plexToken != "" {
		overrides["PLEX_TOKEN"] = plexToken
	}
	if plexLibrarySectionID != "" {
		overrides["PLEX_LIBRARY_SECTION_ID"] = plexLibrarySectionID
	}
	if plexServerID != "" {
		overrides["PLEX_SERVER_ID"] = plexServerID
	}
	if dryRun {
		overrides["PLEXIFY_DRY_RUN"] = "true"
	}
	if plexMatchConcurrency > 0 {
		overrides["PLEX_MATCH_CONCURRENCY"] = strconv.Itoa(plexMatchConcurrency)
	}
	if plexInsecureTLS {
		overrides["PLEX_INSECURE_SKIP_VERIFY"] = "true"
	}
	if plexVerifyTLS {
		overrides["PLEX_VERIFY_TLS"] = "true"
	}
	if plexFastSearch {
		overrides["PLEXIFY_FAST_SEARCH"] = "true"
	}
	if exactMatchesOnly {
		overrides["PLEXIFY_EXACT_MATCHES_ONLY"] = "true"
	}
	if plexMaxRPS >= 0 {
		overrides["PLEX_MAX_REQUESTS_PER_SECOND"] = strconv.FormatFloat(plexMaxRPS, 'f', -1, 64)
	}

	return overrides
}

func main() {
	overrides := parseFlags()

	level := slog.LevelInfo
	if debugMode {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	cfg, err := config.LoadWithOverrides(overrides)
	if err != nil {
		fmt.Fprintf(os.Stderr, "❌ Configuration Error:\n%s\n\n", err)
		fmt.Fprintf(os.Stderr, "💡 Quick Setup:\n")
		fmt.Fprintf(os.Stderr, "1. Create a .env file with your settings, or\n")
		fmt.Fprintf(os.Stderr, "2. Set environment variables, or\n")
		fmt.Fprintf(os.Stderr, "3. Use CLI flags (e.g., -MUSIC_SOCIAL_URL=https://music.example.com)\n\n")
		fmt.Fprintf(os.Stderr, "📖 See README.md for detailed configuration options\n")
		os.Exit(1)
	}

	a, err := app.NewApplication(cfg, debugMode)
	if err != nil {
		slog.Error("failed to create application", "err", err)
		os.Exit(1)
	}

	ctx := context.Background()
	if err := a.Run(ctx); err != nil {
		if errors.Is(err, app.ErrNoPlaylists) {
			app.PrintNoPlaylistsMessage()
			os.Exit(1)
		}
		slog.Error("application failed", "err", err)
		os.Exit(1)
	}
}
