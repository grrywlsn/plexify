package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	"github.com/garry/plexify/config"
	"github.com/garry/plexify/musicbrainz"
	"github.com/garry/plexify/plex"
	"github.com/garry/plexify/spotify"
)

// Version information - set during build
var version = "dev"

// Constants for display formatting
const (
	separatorLine          = "="
	separatorLength        = 80
	playlistSeparator      = "🎵"
	playlistSeparatorCount = 40
)

// Exit codes
const (
	exitCodeSuccess     = 0
	exitCodeNoPlaylists = 1
	exitCodeConfigError = 2
	exitCodeClientError = 3
)

// PlaylistMeta represents metadata for a playlist
type PlaylistMeta struct {
	ID          string
	Name        string
	Description string
}

// Application represents the main application state
type Application struct {
	config            *config.Config
	spotifyClient     *spotify.Client
	plexClient        *plex.Client
	musicBrainzClient *musicbrainz.Client
}

// NewApplication creates a new application instance
func NewApplication(cfg *config.Config) (*Application, error) {
	spotifyClient, err := spotify.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to create Spotify client: %w", err)
	}

	plexClient := plex.NewClientWithTLSConfig(cfg, true)
	plexClient.SetDebug(debugMode)

	musicBrainzClient := musicbrainz.NewClient()

	return &Application{
		config:            cfg,
		spotifyClient:     spotifyClient,
		plexClient:        plexClient,
		musicBrainzClient: musicBrainzClient,
	}, nil
}

// Run executes the main application logic
func (app *Application) Run(ctx context.Context) error {
	// Auto-discover server ID if not provided
	if err := app.discoverServerID(ctx); err != nil {
		log.Printf("⚠️  Warning: Failed to auto-discover server ID: %v", err)
		log.Printf("   Using default server ID. If you encounter issues, please set PLEX_SERVER_ID manually.")
	}

	// Get playlist metadata
	playlistMetas, err := app.getPlaylistMetadata()
	if err != nil {
		return fmt.Errorf("failed to get playlist metadata: %w", err)
	}

	// Validate we have playlists to process
	if len(playlistMetas) == 0 {
		app.printNoPlaylistsMessage()
		os.Exit(exitCodeNoPlaylists)
	}

	// Process each playlist
	return app.processPlaylists(ctx, playlistMetas)
}

// discoverServerID attempts to auto-discover the Plex server ID
func (app *Application) discoverServerID(ctx context.Context) error {
	if app.config.Plex.ServerID != "" {
		return nil // Already set
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

// getPlaylistMetadata retrieves metadata for all playlists to be processed
func (app *Application) getPlaylistMetadata() ([]PlaylistMeta, error) {
	var playlistMetas []PlaylistMeta

	if app.config.Spotify.Username != "" {
		// Fetch all public playlists for the user
		publicPlaylists, err := app.spotifyClient.GetUserPublicPlaylists(app.config.Spotify.Username)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch public playlists for user %s: %w", app.config.Spotify.Username, err)
		}

		for _, pl := range publicPlaylists {
			playlistMetas = append(playlistMetas, PlaylistMeta{
				ID:          pl.ID,
				Name:        pl.Name,
				Description: pl.Description,
			})
		}
		fmt.Printf("🎵 Processing %d public Spotify playlist(s) for user %s...\n\n", len(playlistMetas), app.config.Spotify.Username)
	} else {
		// Process specific playlist IDs
		for _, playlistID := range app.config.Spotify.PlaylistIDs {
			playlistInfo, err := app.spotifyClient.GetPlaylistInfo(playlistID)
			if err != nil {
				log.Printf("❌ Failed to get playlist info for %s: %v", playlistID, err)
				continue
			}
			playlistMetas = append(playlistMetas, PlaylistMeta{
				ID:          playlistID,
				Name:        playlistInfo.Name,
				Description: playlistInfo.Description,
			})
		}
		fmt.Printf("🎵 Processing %d Spotify playlist(s)...\n\n", len(playlistMetas))
	}

	return playlistMetas, nil
}

// processPlaylists processes each playlist sequentially
func (app *Application) processPlaylists(ctx context.Context, playlistMetas []PlaylistMeta) error {
	for playlistIndex, meta := range playlistMetas {
		if err := app.processPlaylist(ctx, meta, playlistIndex+1, len(playlistMetas)); err != nil {
			log.Printf("❌ Failed to process playlist %s: %v", meta.ID, err)
			continue
		}

		// Add separator between playlists
		if playlistIndex < len(playlistMetas)-1 {
			fmt.Println("\n" + strings.Repeat(playlistSeparator, playlistSeparatorCount))
			fmt.Println()
		}
	}

	fmt.Println("\n🎉 All playlists processed!")
	return nil
}

// processPlaylist processes a single playlist
func (app *Application) processPlaylist(ctx context.Context, meta PlaylistMeta, index, total int) error {
	fmt.Printf("📋 Playlist %d/%d: %s\n", index, total, meta.ID)
	fmt.Println(strings.Repeat(separatorLine, separatorLength))

	// Fetch songs from the playlist
	songs, err := app.spotifyClient.GetPlaylistSongs(meta.ID)
	if err != nil {
		return fmt.Errorf("failed to fetch playlist songs: %w", err)
	}

	// Display the songs
	app.displaySongs(songs)

	// Match songs and create playlist
	fmt.Println("\n" + strings.Repeat(separatorLine, separatorLength))
	fmt.Println("MATCHING SONGS TO PLEX LIBRARY")
	fmt.Println(strings.Repeat(separatorLine, separatorLength))

	matchResults, playlist, err := app.plexClient.MatchSpotifyPlaylist(ctx, songs, meta.Name, meta.Description, meta.ID)
	if err != nil {
		return fmt.Errorf("failed to match songs to Plex: %w", err)
	}

	// Populate MusicBrainz IDs for missing tracks
	app.populateMusicBrainzIDsForMissingTracks(ctx, matchResults)

	// Display results
	app.displayMatchingResults(matchResults, songs, playlist)

	return nil
}

// displaySongs displays the list of songs in a playlist
func (app *Application) displaySongs(songs []spotify.Song) {
	fmt.Printf("Songs in playlist (%d total):\n", len(songs))
	fmt.Println(strings.Repeat("-", 60))

	for i, song := range songs {
		fmt.Printf("%3d. %s - %s (%s)\n", i+1, song.Artist, song.Name, song.Album)
	}

	fmt.Println()
	fmt.Printf("Successfully fetched %d songs from Spotify playlist\n", len(songs))
}

// displayMatchingResults displays the results of matching songs to Plex
func (app *Application) displayMatchingResults(matchResults []plex.MatchResult, songs []spotify.Song, playlist *plex.PlexPlaylist) {
	fmt.Println("\n" + strings.Repeat(separatorLine, separatorLength))
	fmt.Println("MATCHING RESULTS")
	fmt.Println(strings.Repeat(separatorLine, separatorLength))

	var titleMatches, noMatches int
	var missingTracks []plex.MatchResult

	for i, result := range matchResults {
		status := "❌ No match"
		if result.PlexTrack != nil {
			switch result.MatchType {
			case "title_artist":
				status = "🔍 Title/Artist match"
				titleMatches++
			}
		} else {
			noMatches++
			missingTracks = append(missingTracks, result)
		}

		fmt.Printf("%3d. %s - %s: %s", i+1, result.SpotifySong.Artist, result.SpotifySong.Name, status)
		if result.PlexTrack != nil {
			fmt.Printf(" (Plex: %s - %s)", result.PlexTrack.Artist, result.PlexTrack.Title)
		}
		fmt.Println()
	}

	// Display summary
	app.displaySummary(songs, titleMatches, noMatches, playlist)

	// Display missing tracks summary if there are any
	if len(missingTracks) > 0 {
		app.displayMissingTracksSummary(missingTracks)
	}
}

// displaySummary displays a summary of the matching results
func (app *Application) displaySummary(songs []spotify.Song, titleMatches, noMatches int, playlist *plex.PlexPlaylist) {
	fmt.Println("\n" + strings.Repeat(separatorLine, separatorLength))
	fmt.Println("SUMMARY")
	fmt.Println(strings.Repeat(separatorLine, separatorLength))
	fmt.Printf("Total songs: %d\n", len(songs))
	fmt.Printf("Title/Artist matches: %d (%.1f%%)\n", titleMatches, float64(titleMatches)/float64(len(songs))*100)
	fmt.Printf("No matches: %d (%.1f%%)\n", noMatches, float64(noMatches)/float64(len(songs))*100)
	fmt.Printf("Total matches: %d (%.1f%%)\n", titleMatches, float64(titleMatches)/float64(len(songs))*100)

	if titleMatches > 0 {
		fmt.Printf("\n✅ Found %d matched tracks in Plex library\n", titleMatches)
		if playlist != nil {
			fmt.Printf("✅ Successfully created/updated playlist: %s (ID: %s)\n", playlist.Title, playlist.ID)
		}
	} else {
		fmt.Println("\n❌ No matches found")
	}
}

// displayMissingTracksSummary displays a summary of tracks that were not matched
func (app *Application) displayMissingTracksSummary(missingTracks []plex.MatchResult) {
	fmt.Println("\n" + strings.Repeat(separatorLine, separatorLength))
	fmt.Println("MISSING TRACKS SUMMARY")
	fmt.Println(strings.Repeat(separatorLine, separatorLength))
	fmt.Printf("Tracks not found in Plex library (%d total):\n", len(missingTracks))
	fmt.Println(strings.Repeat("-", 80))

	for i, result := range missingTracks {
		fmt.Printf("%3d. %s - %s\n", i+1, result.SpotifySong.Artist, result.SpotifySong.Name)
		fmt.Printf("     Spotify track ID: %s\n", result.SpotifySong.ID)
		if result.SpotifySong.ISRC != "" {
			fmt.Printf("     ISRC: %s\n", result.SpotifySong.ISRC)
		} else {
			fmt.Printf("     ISRC: (not available)\n")
		}
		if result.SpotifySong.MusicBrainzID != "" {
			fmt.Printf("     MusicBrainz ID: %s - https://musicbrainz.org/recording/%s\n", result.SpotifySong.MusicBrainzID, result.SpotifySong.MusicBrainzID)
		} else {
			fmt.Printf("     MusicBrainz ID: (not found)\n")
		}
		if i < len(missingTracks)-1 {
			fmt.Println()
		}
	}
}

// populateMusicBrainzIDsForMissingTracks populates MusicBrainz IDs for tracks that weren't matched
func (app *Application) populateMusicBrainzIDsForMissingTracks(ctx context.Context, matchResults []plex.MatchResult) {
	var missingSongs []spotify.Song

	// Collect songs that weren't matched
	for _, result := range matchResults {
		if result.PlexTrack == nil {
			missingSongs = append(missingSongs, result.SpotifySong)
		}
	}

	// Only proceed if there are missing tracks
	if len(missingSongs) == 0 {
		return
	}

	fmt.Println("\n🔍 Looking up MusicBrainz IDs for missing tracks...")

	// Populate MusicBrainz IDs
	app.spotifyClient.PopulateMusicBrainzIDs(ctx, missingSongs, app.musicBrainzClient)

	// Update the match results with the populated MusicBrainz IDs
	songIndex := 0
	for i := range matchResults {
		if matchResults[i].PlexTrack == nil {
			matchResults[i].SpotifySong.MusicBrainzID = missingSongs[songIndex].MusicBrainzID
			songIndex++
		}
	}
}

// printNoPlaylistsMessage displays a helpful message when no playlists are specified
func (app *Application) printNoPlaylistsMessage() {
	fmt.Println("❌ No playlists specified!")
	fmt.Println("Please provide either:")
	fmt.Println("  - SPOTIFY_USERNAME environment variable to fetch all public playlists for a user")
	fmt.Println("  - SPOTIFY_PLAYLIST_ID environment variable with comma-separated playlist IDs")
	fmt.Println("  - -username command line flag to specify a Spotify username")
	fmt.Println("  - -playlists command line flag to specify playlist IDs")
	fmt.Println("\nExample:")
	fmt.Println("  ./plexify -username your_spotify_username")
	fmt.Println("  ./plexify -playlists 37i9dQZF1DXcBWIGoYBM5M,37i9dQZF1DXcBWIGoYBM5N")
	fmt.Println("  ./plexify -debug -playlists 37i9dQZF1DXcBWIGoYBM5M  # with debug output")
}

// Global debug flag
var debugMode bool

// IsDebugMode returns true if debug mode is enabled
func IsDebugMode() bool {
	return debugMode
}

// parseFlags parses command line flags and updates configuration
func parseFlags(cfg *config.Config) {
	var playlistIDs string
	flag.StringVar(&playlistIDs, "playlists", "", "Comma-separated list of Spotify playlist IDs (overrides SPOTIFY_PLAYLIST_ID env var)")
	var spotifyUsername string
	flag.StringVar(&spotifyUsername, "username", "", "Spotify username to fetch all public playlists (overrides SPOTIFY_USERNAME env var)")
	flag.BoolVar(&debugMode, "debug", false, "Enable debug output (detailed matching and similarity information)")

	// Version flag
	var showVersion bool
	flag.BoolVar(&showVersion, "version", false, "Show version information")

	flag.Parse()

	// Handle version flag
	if showVersion {
		fmt.Printf("Plexify version %s\n", version)
		os.Exit(0)
	}

	// Use command line playlist IDs if provided, otherwise use config
	if playlistIDs != "" {
		playlistIDList := strings.Split(playlistIDs, ",")
		for i, id := range playlistIDList {
			playlistIDList[i] = strings.TrimSpace(id)
		}
		cfg.Spotify.PlaylistIDs = playlistIDList
	}

	// Use command line username if provided, otherwise use config
	if spotifyUsername != "" {
		cfg.Spotify.Username = spotifyUsername
	}
}

func main() {
	// Load configuration
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// Parse command line flags
	parseFlags(cfg)

	// Create application
	app, err := NewApplication(cfg)
	if err != nil {
		log.Fatalf("Failed to create application: %v", err)
	}

	// Run application
	ctx := context.Background()
	if err := app.Run(ctx); err != nil {
		log.Fatalf("Application failed: %v", err)
	}
}
