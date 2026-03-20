package track

// Track is a normalized track from a source playlist (e.g. music-social.com) or any future source.
type Track struct {
	ID                        string // Optional stable id for logging (e.g. position-based key)
	Name                      string // Track title
	Artist                    string
	Album                     string
	Duration                  int // milliseconds
	ISRC                      string
	MusicBrainzID             string // Recording MBID when known
	MusicBrainzReleaseGroupID string // When set (from API), missing-track summary links to release group instead of recording

	// Streaming album identifiers from music-social.com JSON (optional).
	SpotifyAlbumURI   string // e.g. spotify:album:{id} or https://open.spotify.com/album/...
	AppleMusicAlbumID string // Apple Music catalog album id (storefront fixed to us for Harmony)
}
