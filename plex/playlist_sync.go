package plex

import (
	"context"
	"fmt"
	"log/slog"
	"strings"
	"sync"

	"github.com/grrywlsn/plexify/track"
)

// DesiredPlaylistEntries builds the ordered target playlist rows from match results (source order, Plex tracks only).
func DesiredPlaylistEntries(results []MatchResult) []PlaylistDesiredEntry {
	out := make([]PlaylistDesiredEntry, 0, len(results))
	for _, r := range results {
		if r.PlexTrack != nil {
			out = append(out, PlaylistDesiredEntry{MatchResult: r})
		}
	}
	return out
}

// MatchedTrackIDs returns Plex rating keys for desired entries in order.
func MatchedTrackIDs(entries []PlaylistDesiredEntry) []string {
	ids := make([]string, 0, len(entries))
	for _, e := range entries {
		ids = append(ids, e.MatchResult.PlexTrack.ID)
	}
	return ids
}

// MatchSourceTracks resolves each source track against the Plex library.
// When matchConcurrency > 1, lookups run in parallel (bounded); order of results matches songs.
func (c *Client) MatchSourceTracks(ctx context.Context, songs []track.Track) []MatchResult {
	n := len(songs)
	results := make([]MatchResult, n)
	if n == 0 {
		return results
	}

	slog.InfoContext(ctx, "matching source tracks to Plex",
		"count", n, "concurrency", c.matchConcurrency)

	if c.matchConcurrency <= 1 {
		for i := range songs {
			if err := ctx.Err(); err != nil {
				for j := i; j < n; j++ {
					results[j] = MatchResult{
						SourceTrack: songs[j],
						PlexTrack:   nil,
						MatchType:   MatchTypeError,
						Confidence:  0,
					}
				}
				return results
			}
			song := songs[i]
			slog.InfoContext(ctx, "processing song",
				"index", i+1, "total", n, "artist", song.Artist, "title", song.Name)
			c.fillMatchResult(ctx, &results[i], song)
		}
		return results
	}

	var wg sync.WaitGroup
	sem := make(chan struct{}, c.matchConcurrency)
	var mu sync.Mutex
	for i := range songs {
		i, song := i, songs[i]
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			if err := ctx.Err(); err != nil {
				mu.Lock()
				results[i] = MatchResult{
					SourceTrack: song,
					PlexTrack:   nil,
					MatchType:   MatchTypeError,
					Confidence:  0,
				}
				mu.Unlock()
				return
			}

			var slot MatchResult
			c.fillMatchResult(ctx, &slot, song)
			mu.Lock()
			results[i] = slot
			mu.Unlock()
		}()
	}
	wg.Wait()
	return results
}

func (c *Client) fillMatchResult(ctx context.Context, out *MatchResult, song track.Track) {
	plexTr, matchType, err := c.SearchTrack(ctx, song)
	if err != nil {
		slog.InfoContext(ctx, "search error", "artist", song.Artist, "title", song.Name, "err", err)
		*out = MatchResult{
			SourceTrack: song,
			PlexTrack:   nil,
			MatchType:   MatchTypeError,
			Confidence:  0.0,
		}
		return
	}
	*out = MatchResult{
		SourceTrack: song,
		PlexTrack:   plexTr,
		MatchType:   matchType,
		Confidence:  c.calculateConfidence(song, plexTr, matchType),
	}
}

// FindPlaylistByTitle returns an existing playlist with the given title, or nil if none.
func (c *Client) FindPlaylistByTitle(ctx context.Context, title string) (*PlexPlaylist, error) {
	slog.InfoContext(ctx, "checking for existing playlist", "title", title)
	existingPlaylists, err := c.GetPlaylists(ctx)
	if err != nil {
		slog.InfoContext(ctx, "failed to list playlists", "err", err)
		return nil, err
	}
	want := strings.TrimSpace(title)
	for i := range existingPlaylists {
		if strings.TrimSpace(existingPlaylists[i].Title) == want {
			pl := existingPlaylists[i]
			return &pl, nil
		}
	}
	return nil, nil
}

// EnsurePlaylistAndSync creates or updates playlist metadata, clears items, adds tracks, and sets poster.
// If existing is non-nil, that playlist is updated; otherwise a new playlist is created.
func (c *Client) EnsurePlaylistAndSync(ctx context.Context, playlistName, description, sourcePlaylistURL, artworkURL string, trackIDs []string, existing *PlexPlaylist) (*PlexPlaylist, error) {
	var playlist *PlexPlaylist
	var err error
	if existing != nil {
		playlist = existing
		slog.InfoContext(ctx, "found existing playlist",
			"title", existing.Title, "id", existing.ID, "track_count", existing.TrackCount)
		slog.InfoContext(ctx, "syncing playlist to match music-social source")
		if err := c.UpdatePlaylistMetadata(ctx, existing.ID, playlistName, description, sourcePlaylistURL); err != nil {
			slog.InfoContext(ctx, "playlist metadata update failed", "err", err)
		} else {
			slog.InfoContext(ctx, "updated playlist metadata")
		}
	} else {
		slog.InfoContext(ctx, "creating new playlist", "title", playlistName)
		playlist, err = c.CreatePlaylist(ctx, playlistName, description, "", sourcePlaylistURL)
		if err != nil {
			slog.InfoContext(ctx, "create playlist failed", "err", err)
			return nil, err
		}
		slog.InfoContext(ctx, "created playlist", "title", playlist.Title, "id", playlist.ID)
	}

	slog.InfoContext(ctx, "clearing playlist and adding tracks", "count", len(trackIDs))
	if err := c.ClearPlaylist(ctx, playlist.ID); err != nil {
		slog.InfoContext(ctx, "clear playlist failed", "err", err)
		return playlist, err
	}
	if err := c.AddTracksToPlaylist(ctx, playlist.ID, trackIDs); err != nil {
		slog.InfoContext(ctx, "add tracks failed", "err", err)
		return playlist, err
	}
	slog.InfoContext(ctx, "added tracks to playlist", "count", len(trackIDs))

	if artworkURL != "" {
		slog.InfoContext(ctx, "setting playlist artwork")
		if err := c.SetPlaylistPosterUsingPlexgo(ctx, playlist.ID, artworkURL); err != nil {
			slog.InfoContext(ctx, "set playlist artwork failed", "err", err)
		} else {
			slog.InfoContext(ctx, "set playlist artwork OK")
		}
	}
	return playlist, nil
}

// MatchPlaylist matches source tracks to Plex tracks and syncs a playlist. Diff output is returned for the caller to print.
func (c *Client) MatchPlaylist(ctx context.Context, songs []track.Track, playlistName, description string, sourcePlaylistURL string, artworkURL string) ([]MatchResult, *PlexPlaylist, PlaylistDiffView, error) {
	results := c.MatchSourceTracks(ctx, songs)
	desired := DesiredPlaylistEntries(results)
	trackIDs := MatchedTrackIDs(desired)
	var view PlaylistDiffView

	if len(trackIDs) == 0 {
		slog.InfoContext(ctx, "no tracks matched, skipping playlist")
		return results, nil, view, nil
	}

	slog.InfoContext(ctx, "matched tracks", "count", len(trackIDs))

	existing, err := c.FindPlaylistByTitle(ctx, playlistName)
	if err != nil {
		return results, nil, view, err
	}

	var oldItems []PlexTrack
	if existing != nil {
		oldItems, err = c.GetPlaylistItems(ctx, existing.ID)
		if err != nil {
			slog.InfoContext(ctx, "read playlist items for diff failed", "err", err)
			return results, nil, view, fmt.Errorf("get playlist items for diff: %w", err)
		}
	}

	view = NewPlaylistDiffView(playlistName, oldItems, desired)

	if c.dryRun {
		slog.InfoContext(ctx, "dry-run: skipping Plex playlist mutations", "playlist", playlistName)
		if existing != nil {
			return results, existing, view, nil
		}
		return results, nil, view, nil
	}

	playlist, err := c.EnsurePlaylistAndSync(ctx, playlistName, description, sourcePlaylistURL, artworkURL, trackIDs, existing)
	if err != nil {
		return results, playlist, view, err
	}
	return results, playlist, view, nil
}
