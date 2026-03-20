package track

import (
	"net/url"
	"strings"
	"testing"
)

func TestHarmonyAddToMusicBrainzURL(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		spotifyAlbumURI   string
		appleMusicAlbumID string
		wantReleaseInURL  string // substring that must appear decoded in the `url` query param
		wantEmpty         bool
	}{
		{
			name:             "spotify URI",
			spotifyAlbumURI:  "spotify:album:1qSS0T6Ffrb3rFVpizzOuk",
			wantReleaseInURL: "https://open.spotify.com/album/1qSS0T6Ffrb3rFVpizzOuk",
		},
		{
			name:             "spotify HTTPS",
			spotifyAlbumURI:  "https://open.spotify.com/album/abcXYZ",
			wantReleaseInURL: "https://open.spotify.com/album/abcXYZ",
		},
		{
			name:              "apple only",
			appleMusicAlbumID: "1630768988",
			wantReleaseInURL:  "https://music.apple.com/us/album/_/1630768988",
		},
		{
			name:              "spotify preferred over apple",
			spotifyAlbumURI:   "spotify:album:onlySpotify",
			appleMusicAlbumID: "999",
			wantReleaseInURL:  "https://open.spotify.com/album/onlySpotify",
		},
		{
			name:            "no album",
			spotifyAlbumURI: "spotify:track:xyz",
			wantEmpty:       true,
		},
		{
			name:            "whitespace only",
			spotifyAlbumURI: "   ",
			wantEmpty:       true,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got := HarmonyAddToMusicBrainzURL(tt.spotifyAlbumURI, tt.appleMusicAlbumID)
			if tt.wantEmpty {
				if got != "" {
					t.Fatalf("expected empty, got %q", got)
				}
				return
			}
			if got == "" {
				t.Fatal("expected non-empty URL")
			}
			if !strings.HasPrefix(got, harmonyReleaseBase+"?") {
				t.Fatalf("unexpected base: %q", got)
			}
			u, err := url.Parse(got)
			if err != nil {
				t.Fatal(err)
			}
			release := u.Query().Get("url")
			if release != tt.wantReleaseInURL {
				t.Fatalf("url query param: got %q want %q", release, tt.wantReleaseInURL)
			}
			for _, key := range []string{"gtin", "region", "musicbrainz", "deezer", "itunes", "spotify", "tidal"} {
				if u.Query().Get(key) != "" {
					t.Fatalf("expected empty %q, got %q", key, u.Query().Get(key))
				}
			}
		})
	}
}
