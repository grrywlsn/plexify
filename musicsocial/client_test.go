package musicsocial

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_ListUserPlaylists(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/users/alice/playlists.json" {
			http.NotFound(w, r)
			return
		}
		_ = json.NewEncoder(w).Encode([]PlaylistSummary{
			{ID: "pl1", Title: "One", TrackCount: 2, URL: "https://example.com/playlist/pl1"},
		})
	}))
	defer ts.Close()

	c, err := NewClient(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	list, err := c.ListUserPlaylists("alice")
	if err != nil {
		t.Fatal(err)
	}
	if len(list) != 1 || list[0].ID != "pl1" || list[0].Title != "One" {
		t.Fatalf("unexpected list: %+v", list)
	}
}

func TestClient_GetPlaylist(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/playlist/pl1.json" {
			http.NotFound(w, r)
			return
		}
		doc := map[string]any{
			"id": "pl1", "title": "One", "owner": "alice", "track_count": 1,
			"updated_at": "2025-01-01T00:00:00Z",
			"tracks": []map[string]any{
				{"position": 1, "title": "Song", "artist": "Artist", "duration_ms": 180000,
					"musicbrainz": map[string]any{
						"track_gid": "mbid", "release_group_gid": "rg-mbid",
						"isrcs": []string{"USXXX123"},
					}},
			},
		}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer ts.Close()

	c, err := NewClient(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	pl, err := c.GetPlaylist("pl1")
	if err != nil {
		t.Fatal(err)
	}
	if pl.Title != "One" || len(pl.Tracks) != 1 {
		t.Fatalf("unexpected playlist: %+v", pl)
	}
	tr := pl.Tracks[0]
	if tr.Name != "Song" || tr.Artist != "Artist" || tr.MusicBrainzID != "mbid" ||
		tr.MusicBrainzReleaseGroupID != "rg-mbid" || tr.ISRC != "USXXX123" {
		t.Fatalf("unexpected track: %+v", tr)
	}
}

func TestClient_GetPlaylist_streamingAlbums(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/playlist/pl2.json" {
			http.NotFound(w, r)
			return
		}
		doc := map[string]any{
			"id": "pl2", "title": "Mix", "owner": "bob", "track_count": 2,
			"updated_at": "2025-01-01T00:00:00Z",
			"tracks": []map[string]any{
				{
					"position": 1, "title": "A", "artist": "Art",
					"spotify": map[string]any{
						"track_uri": "spotify:track:xyz", "album_uri": "spotify:album:albumSpotify",
					},
				},
				{
					"position": 2, "title": "B", "artist": "Art",
					"apple_music": map[string]any{"album_id": "1630768988"},
				},
			},
		}
		_ = json.NewEncoder(w).Encode(doc)
	}))
	defer ts.Close()

	c, err := NewClient(ts.URL)
	if err != nil {
		t.Fatal(err)
	}
	pl, err := c.GetPlaylist("pl2")
	if err != nil {
		t.Fatal(err)
	}
	if len(pl.Tracks) != 2 {
		t.Fatalf("want 2 tracks, got %d", len(pl.Tracks))
	}
	if pl.Tracks[0].SpotifyAlbumURI != "spotify:album:albumSpotify" || pl.Tracks[0].AppleMusicAlbumID != "" {
		t.Fatalf("track 0: %+v", pl.Tracks[0])
	}
	if pl.Tracks[1].SpotifyAlbumURI != "" || pl.Tracks[1].AppleMusicAlbumID != "1630768988" {
		t.Fatalf("track 1: %+v", pl.Tracks[1])
	}
}

func TestClient_GetPlaylist_invalidID(t *testing.T) {
	c, err := NewClient("https://example.com")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := c.GetPlaylist("bad/id"); err == nil {
		t.Fatal("expected error")
	}
}

func TestPlaylistPageURL(t *testing.T) {
	c, err := NewClient("https://example.com/social")
	if err != nil {
		t.Fatal(err)
	}
	u := c.PlaylistPageURL("abc")
	if u != "https://example.com/social/playlist/abc" {
		t.Fatalf("got %s", u)
	}
}
