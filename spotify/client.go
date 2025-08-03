package spotify

import (
	"context"
	"fmt"

	"github.com/garry/plexify/config"
	"github.com/garry/plexify/musicbrainz"
	"github.com/zmb3/spotify/v2"
	spotifyauth "github.com/zmb3/spotify/v2/auth"
	"golang.org/x/oauth2"
)

// Client wraps the Spotify API client
type Client struct {
	client *spotify.Client
	config *config.Config
}

// Song represents a track from a Spotify playlist
type Song struct {
	ID            string
	Name          string
	Artist        string
	Album         string
	Duration      int
	URI           string
	ISRC          string
	MusicBrainzID string
}

// PlaylistInfo represents basic information about a playlist
type PlaylistInfo struct {
	ID          string
	Name        string
	Description string
	Owner       string
	TrackCount  int
	Public      bool
}

// NewClient creates a new Spotify client with authentication
func NewClient(cfg *config.Config) (*Client, error) {
	auth := spotifyauth.New(
		spotifyauth.WithRedirectURL(cfg.Spotify.RedirectURI),
		spotifyauth.WithClientID(cfg.Spotify.ClientID),
		spotifyauth.WithClientSecret(cfg.Spotify.ClientSecret),
		spotifyauth.WithScopes(
			spotifyauth.ScopePlaylistReadPrivate,
			spotifyauth.ScopePlaylistReadCollaborative,
			spotifyauth.ScopeUserReadPrivate,
		),
	)

	// For CLI/cronjob usage, we'll use the client credentials flow
	// This is simpler than authorization code flow for automated tools
	ctx := context.Background()

	// Create token source using client credentials
	token, err := auth.Exchange(ctx, "", oauth2.SetAuthURLParam("grant_type", "client_credentials"))
	if err != nil {
		return nil, fmt.Errorf("failed to exchange token: %w", err)
	}

	httpClient := spotifyauth.New().Client(ctx, token)
	client := spotify.New(httpClient)

	return &Client{
		client: client,
		config: cfg,
	}, nil
}

// GetUserPublicPlaylists fetches all public playlists for a Spotify user
func (c *Client) GetUserPublicPlaylists(username string) ([]PlaylistInfo, error) {
	ctx := context.Background()

	var playlists []PlaylistInfo

	// Get user's public playlists
	userPlaylists, err := c.client.GetPlaylistsForUser(ctx, username)
	if err != nil {
		return nil, fmt.Errorf("failed to get user playlists: %w", err)
	}

	// Collect all playlists, handling pagination
	for {
		for _, playlist := range userPlaylists.Playlists {
			if playlist.IsPublic {
				playlistInfo := PlaylistInfo{
					ID:          string(playlist.ID),
					Name:        playlist.Name,
					Description: playlist.Description,
					Owner:       playlist.Owner.DisplayName,
					TrackCount:  int(playlist.Tracks.Total),
					Public:      playlist.IsPublic,
				}
				playlists = append(playlists, playlistInfo)
			}
		}
		if err := c.client.NextPage(ctx, userPlaylists); err != nil {
			break
		}
	}

	return playlists, nil
}

// GetPlaylistSongs fetches all songs from a Spotify playlist
func (c *Client) GetPlaylistSongs(playlistID string) ([]Song, error) {
	ctx := context.Background()

	// Validate playlist exists
	if err := c.validatePlaylist(ctx, playlistID); err != nil {
		return nil, fmt.Errorf("playlist validation failed: %w", err)
	}

	var songs []Song
	page := 1

	// Iterate through all tracks in the playlist
	for {
		playlistTracks, err := c.client.GetPlaylistTracks(ctx, spotify.ID(playlistID), spotify.Offset((page-1)*100), spotify.Limit(100))
		if err != nil {
			return nil, fmt.Errorf("failed to get playlist tracks (page %d): %w", page, err)
		}

		// Process tracks in this page
		for _, item := range playlistTracks.Tracks {
			track := item.Track
			song := c.convertTrackToSong(track)
			songs = append(songs, song)
		}

		// Check if we've processed all tracks
		if len(playlistTracks.Tracks) < 100 {
			break
		}
		page++
	}

	return songs, nil
}

// validatePlaylist checks if a playlist exists and is accessible
func (c *Client) validatePlaylist(ctx context.Context, playlistID string) error {
	_, err := c.client.GetPlaylist(ctx, spotify.ID(playlistID))
	if err != nil {
		return fmt.Errorf("playlist not found or not accessible: %w", err)
	}
	return nil
}

// convertTrackToSong converts a Spotify track to our Song struct
func (c *Client) convertTrackToSong(track spotify.FullTrack) Song {
	// Get artist name (handle multiple artists)
	artist := ""
	if len(track.Artists) > 0 {
		artist = track.Artists[0].Name
	}

	return Song{
		ID:            string(track.ID),
		Name:          track.Name,
		Artist:        artist,
		Album:         track.Album.Name,
		Duration:      track.Duration,
		URI:           string(track.URI),
		ISRC:          track.ExternalIDs["isrc"],
		MusicBrainzID: "", // Will be populated later if needed
	}
}

// GetPlaylistInfo returns basic information about a playlist
func (c *Client) GetPlaylistInfo(playlistID string) (*spotify.FullPlaylist, error) {
	ctx := context.Background()

	playlist, err := c.client.GetPlaylist(ctx, spotify.ID(playlistID))
	if err != nil {
		return nil, fmt.Errorf("failed to get playlist info: %w", err)
	}

	return playlist, nil
}

// PopulateMusicBrainzIDs populates MusicBrainz IDs for songs using ISRC or artist/title search
func (c *Client) PopulateMusicBrainzIDs(ctx context.Context, songs []Song, mbClient *musicbrainz.Client) {
	for i := range songs {
		var musicBrainzID string
		var err error

		// Try ISRC first if available
		if songs[i].ISRC != "" {
			musicBrainzID, err = mbClient.GetMusicBrainzIDByISRC(ctx, songs[i].ISRC)
			if err == nil && musicBrainzID != "" {
				songs[i].MusicBrainzID = musicBrainzID
				continue
			}
		}

		// Fall back to artist/title search if ISRC search failed or ISRC not available
		musicBrainzID, err = mbClient.GetMusicBrainzIDByArtistAndTitle(ctx, songs[i].Artist, songs[i].Name)
		if err == nil && musicBrainzID != "" {
			songs[i].MusicBrainzID = musicBrainzID
		}
	}
}
