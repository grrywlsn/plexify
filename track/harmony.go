package track

import (
	"fmt"
	"net/url"
	"strings"
)

const harmonyReleaseBase = "https://harmony.pulsewidth.org.uk/release"

// HarmonyAddToMusicBrainzURL returns a Harmony “add release” URL for MusicBrainz, matching
// music-social’s admin panel. Spotify album takes precedence over Apple Music when both are set.
// Returns "" when neither album-level identifier is usable (e.g. only track_uri, no album).
func HarmonyAddToMusicBrainzURL(spotifyAlbumURI, appleMusicAlbumID string) string {
	releaseURL := spotifyAlbumHTTPSURL(spotifyAlbumURI)
	if releaseURL == "" && strings.TrimSpace(appleMusicAlbumID) != "" {
		id := strings.TrimSpace(appleMusicAlbumID)
		releaseURL = fmt.Sprintf("https://music.apple.com/us/album/_/%s", url.PathEscape(id))
	}
	if releaseURL == "" {
		return ""
	}
	v := url.Values{}
	v.Set("url", releaseURL)
	v.Set("gtin", "")
	v.Set("region", "")
	v.Set("musicbrainz", "")
	v.Set("deezer", "")
	v.Set("itunes", "")
	v.Set("spotify", "")
	v.Set("tidal", "")
	return harmonyReleaseBase + "?" + v.Encode()
}

func spotifyAlbumHTTPSURL(spotifyAlbumURI string) string {
	s := strings.TrimSpace(spotifyAlbumURI)
	if s == "" {
		return ""
	}
	const prefix = "spotify:album:"
	if strings.HasPrefix(s, prefix) {
		id := strings.TrimSpace(strings.TrimPrefix(s, prefix))
		if id == "" {
			return ""
		}
		return "https://open.spotify.com/album/" + url.PathEscape(id)
	}
	u, err := url.Parse(s)
	if err != nil || u.Scheme == "" || u.Host == "" {
		return ""
	}
	host := strings.ToLower(strings.TrimPrefix(u.Host, "www."))
	if host != "open.spotify.com" {
		return ""
	}
	// Path like /album/{id} or /intl-de/album/{id}
	parts := strings.Split(strings.Trim(u.Path, "/"), "/")
	for i := 0; i < len(parts)-1; i++ {
		if parts[i] == "album" && parts[i+1] != "" {
			return "https://open.spotify.com/album/" + url.PathEscape(parts[i+1])
		}
	}
	return ""
}
