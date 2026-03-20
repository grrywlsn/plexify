package app

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/internal/cliutil"
	"github.com/grrywlsn/plexify/musicsocial"
	"github.com/grrywlsn/plexify/plex"
	"github.com/grrywlsn/plexify/track"
)

const (
	playlistSeparator      = "🎵"
	playlistSeparatorCount = 40
)

// PlaylistMeta represents metadata for a playlist
type PlaylistMeta struct {
	ID          string
	Name        string
	Description string
	ArtworkURL  string // Unused for source API (no cover URL); kept for Plex poster hook
	PageURL     string // Canonical source playlist URL (e.g. on music-social.com) for Plex description attribution
}

// Application holds clients and config for a single run (no persisted state between invocations).
type Application struct {
	config      *config.Config
	musicSocial *musicsocial.Client
	plexClient  *plex.Client
}

// NewApplication creates a new application instance. debug enables verbose Plex search logging.
func NewApplication(cfg *config.Config, debug bool) (*Application, error) {
	ms, err := musicsocial.NewClient(cfg.MusicSocial.BaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to create music-social.com API client: %w", err)
	}

	plexClient := plex.NewClient(cfg)
	plexClient.SetDebug(debug)
	if cfg.Plex.ExactMatchesOnly {
		slog.Info("Plex track matching: exact-matches-only (raw title/artist strategy; no title normalizations or full-library scan)")
	}

	return &Application{
		config:      cfg,
		musicSocial: ms,
		plexClient:  plexClient,
	}, nil
}

// Run executes the main application logic
func (app *Application) Run(ctx context.Context) error {
	if err := app.discoverServerID(ctx); err != nil {
		slog.Warn("failed to auto-discover Plex server ID", "err", err)
		slog.Warn("using configured server ID; set PLEX_SERVER_ID if sync fails")
	}

	playlistMetas, err := app.getPlaylistMetadata()
	if err != nil {
		return fmt.Errorf("failed to get playlist metadata: %w", err)
	}

	if len(playlistMetas) == 0 {
		return ErrNoPlaylists
	}

	return app.processPlaylists(ctx, playlistMetas)
}

func (app *Application) discoverServerID(ctx context.Context) error {
	if app.config.Plex.ServerID != "" {
		return nil
	}

	fmt.Println("🔍 Auto-discovering Plex server ID...")
	serverID, err := app.plexClient.GetServerID(ctx)
	if err != nil {
		return fmt.Errorf("failed to auto-discover server ID: %w", err)
	}

	app.config.Plex.ServerID = serverID
	app.plexClient.SetServerID(serverID)
	fmt.Printf("✅ Discovered server ID: %s\n", serverID)
	return nil
}

func playlistPageURL(app *Application, summaryURL, playlistID string) string {
	if summaryURL != "" {
		return summaryURL
	}
	return app.musicSocial.PlaylistPageURL(playlistID)
}

// getPlaylistMetadata retrieves metadata for all playlists to be processed
func (app *Application) getPlaylistMetadata() ([]PlaylistMeta, error) {
	var playlistMetas []PlaylistMeta
	seenIDs := make(map[string]bool)

	if app.config.MusicSocial.Username != "" {
		publicPlaylists, err := app.musicSocial.ListUserPlaylists(app.config.MusicSocial.Username)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch public playlists for user %s: %w", app.config.MusicSocial.Username, err)
		}

		for _, pl := range publicPlaylists {
			if !seenIDs[pl.ID] {
				seenIDs[pl.ID] = true
				playlistMetas = append(playlistMetas, PlaylistMeta{
					ID:          pl.ID,
					Name:        pl.Title,
					Description: pl.Description,
					PageURL:     playlistPageURL(app, pl.URL, pl.ID),
				})
			}
		}
		fmt.Printf("🎵 Found %d public playlist(s) on music-social.com for user %s\n", len(publicPlaylists), app.config.MusicSocial.Username)
	}

	if len(app.config.MusicSocial.PlaylistIDs) > 0 {
		addedCount := 0
		for _, playlistID := range app.config.MusicSocial.PlaylistIDs {
			if seenIDs[playlistID] {
				continue
			}

			pl, err := app.musicSocial.GetPlaylist(playlistID)
			if err != nil {
				slog.Info(fmt.Sprintf("failed to get playlist %s: %v", playlistID, err))
				continue
			}

			seenIDs[playlistID] = true
			playlistMetas = append(playlistMetas, PlaylistMeta{
				ID:          playlistID,
				Name:        pl.Title,
				Description: pl.Description,
				PageURL:     playlistPageURL(app, "", playlistID),
			})
			addedCount++
		}
		if addedCount > 0 {
			fmt.Printf("🎵 Found %d additional playlist(s) from MUSIC_SOCIAL_PLAYLIST_ID\n", addedCount)
		}
	}

	playlistMetas = app.filterExcludedPlaylists(playlistMetas)

	fmt.Printf("🎵 Processing %d source playlist(s)...\n\n", len(playlistMetas))

	return playlistMetas, nil
}

func (app *Application) filterExcludedPlaylists(playlists []PlaylistMeta) []PlaylistMeta {
	if len(app.config.MusicSocial.ExcludedPlaylistIDs) == 0 {
		return playlists
	}

	excludedSet := make(map[string]bool)
	for _, id := range app.config.MusicSocial.ExcludedPlaylistIDs {
		excludedSet[id] = true
	}

	var filtered []PlaylistMeta
	for _, pl := range playlists {
		if excludedSet[pl.ID] {
			fmt.Printf("⏭️  Excluding playlist: %s (%s)\n", pl.Name, pl.ID)
		} else {
			filtered = append(filtered, pl)
		}
	}

	return filtered
}

func (app *Application) processPlaylists(ctx context.Context, playlistMetas []PlaylistMeta) error {
	for playlistIndex, meta := range playlistMetas {
		if err := app.processPlaylist(ctx, meta, playlistIndex+1, len(playlistMetas)); err != nil {
			slog.Info(fmt.Sprintf("failed to process playlist %s: %v", meta.ID, err))
			continue
		}

		if playlistIndex < len(playlistMetas)-1 {
			fmt.Println("\n" + strings.Repeat(playlistSeparator, playlistSeparatorCount))
			fmt.Println()
		}
	}

	fmt.Println("\n🎉 All playlists processed!")
	return nil
}

func (app *Application) processPlaylist(ctx context.Context, meta PlaylistMeta, index, total int) error {
	fmt.Printf("📋 Playlist %d/%d: %s\n", index, total, meta.ID)
	fmt.Println(cliutil.RepeatChar("=", cliutil.SectionWidth))

	if app.config.Plex.DryRun {
		fmt.Println("⚠️  Dry-run mode: Plex playlists will not be created, cleared, or modified.")
		fmt.Println()
	}

	pl, err := app.musicSocial.GetPlaylist(meta.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch playlist: %w", err)
	}

	songs := pl.Tracks
	app.displaySongs(songs)

	fmt.Println("\n" + cliutil.RepeatChar("=", cliutil.SectionWidth))
	fmt.Println("MATCHING SONGS TO PLEX LIBRARY")
	fmt.Println(cliutil.RepeatChar("=", cliutil.SectionWidth))

	matchResults, playlist, diffView, err := app.plexClient.MatchPlaylist(ctx, songs, meta.Name, meta.Description, meta.PageURL, meta.ArtworkURL)
	if err != nil {
		return fmt.Errorf("failed to match songs to Plex: %w", err)
	}

	app.displayMatchingResults(matchResults, songs, playlist, diffView)

	return nil
}

func (app *Application) displaySongs(songs []track.Track) {
	fmt.Printf("Songs in playlist (%d total):\n", len(songs))
	fmt.Println(strings.Repeat("-", 60))

	for i, song := range songs {
		fmt.Printf("%3d. %s - %s (%s)\n", i+1, song.Artist, song.Name, song.Album)
	}

	fmt.Println()
	fmt.Printf("Successfully fetched %d songs from source playlist\n", len(songs))
}

func (app *Application) displayMatchingResults(matchResults []plex.MatchResult, songs []track.Track, playlist *plex.PlexPlaylist, diffView plex.PlaylistDiffView) {
	fmt.Println("\n" + cliutil.RepeatChar("=", cliutil.SectionWidth))
	fmt.Println("MATCHING RESULTS")
	fmt.Println(cliutil.RepeatChar("=", cliutil.SectionWidth))

	var titleMatches, noMatches int
	var missingTracks []plex.MatchResult

	for i, result := range matchResults {
		status := "❌ No match"
		if result.PlexTrack != nil {
			switch result.MatchType {
			case plex.MatchTypeTitleArtist:
				status = "🔍 Title/Artist match"
				titleMatches++
			}
		} else {
			noMatches++
			missingTracks = append(missingTracks, result)
		}

		fmt.Printf("%3d. %s - %s: %s", i+1, result.SourceTrack.Artist, result.SourceTrack.Name, status)
		if result.PlexTrack != nil {
			fmt.Printf(" (Plex: %s - %s)", result.PlexTrack.Artist, result.PlexTrack.Title)
		}
		fmt.Println()
	}

	app.displaySummary(songs, titleMatches, noMatches, playlist, diffView)

	if len(missingTracks) > 0 {
		app.displayMissingTracksSummary(missingTracks)
	}
}

func (app *Application) displaySummary(songs []track.Track, titleMatches, noMatches int, playlist *plex.PlexPlaylist, diffView plex.PlaylistDiffView) {
	fmt.Println("\n" + cliutil.RepeatChar("=", cliutil.SectionWidth))
	fmt.Println("SUMMARY")
	fmt.Println(cliutil.RepeatChar("=", cliutil.SectionWidth))
	fmt.Printf("Total songs: %d\n", len(songs))
	if len(songs) > 0 {
		fmt.Printf("Title/Artist matches: %d (%.1f%%)\n", titleMatches, float64(titleMatches)/float64(len(songs))*100)
		fmt.Printf("No matches: %d (%.1f%%)\n", noMatches, float64(noMatches)/float64(len(songs))*100)
	}

	if titleMatches > 0 {
		fmt.Printf("\n✅ Found %d matched tracks in Plex library\n", titleMatches)
		if app.config.Plex.DryRun {
			fmt.Println("ℹ️  Dry-run: playlist on Plex was not modified.")
		} else if playlist != nil {
			fmt.Printf("✅ Successfully created/updated playlist: %s (ID: %s)\n", playlist.Title, playlist.ID)
		}
	} else {
		fmt.Println("\n❌ No matches found")
	}

	if diffView.PlaylistTitle != "" {
		plex.FprintPlaylistDiffEmbedded(os.Stdout, diffView, plex.StdoutSupportsColor())
	}
}

func (app *Application) displayMissingTracksSummary(missingTracks []plex.MatchResult) {
	fmt.Println("\n" + cliutil.RepeatChar("=", cliutil.SectionWidth))
	fmt.Println("MISSING TRACKS SUMMARY")
	fmt.Println(cliutil.RepeatChar("=", cliutil.SectionWidth))
	fmt.Printf("Tracks not found in Plex library (%d total):\n", len(missingTracks))
	fmt.Println(strings.Repeat("-", 80))

	for i, result := range missingTracks {
		st := result.SourceTrack
		fmt.Printf("%3d. %s - %s\n", i+1, st.Artist, st.Name)
		if st.ISRC != "" {
			fmt.Printf("     ISRC: %s\n", st.ISRC)
		} else {
			fmt.Printf("     ISRC: (not available)\n")
		}
		if st.MusicBrainzID != "" {
			fmt.Printf("     MusicBrainz ID: %s - https://musicbrainz.org/recording/%s\n", st.MusicBrainzID, st.MusicBrainzID)
		}
		if i < len(missingTracks)-1 {
			fmt.Println()
		}
	}
}

// PrintNoPlaylistsMessage writes the standard help when no playlists are configured.
func PrintNoPlaylistsMessage() {
	fmt.Println("❌ No playlists specified!")
	fmt.Println("Please provide either:")
	fmt.Println("  - MUSIC_SOCIAL_USERNAME to fetch all public playlists for that user")
	fmt.Println("  - MUSIC_SOCIAL_PLAYLIST_ID with comma-separated playlist IDs")
	fmt.Println("  - -MUSIC_SOCIAL_USERNAME or -MUSIC_SOCIAL_PLAYLIST_ID CLI flags")
	fmt.Println("\nExample:")
	fmt.Println("  ./plexify -MUSIC_SOCIAL_USERNAME your_username")
	fmt.Println("  ./plexify -MUSIC_SOCIAL_PLAYLIST_ID pl_abc123,pl_def456")
}
