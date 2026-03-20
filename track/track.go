package track

// Track is a normalized track from a source playlist (e.g. music-social.com) or any future source.
type Track struct {
	ID            string // Optional stable id for logging (e.g. position-based key)
	Name          string // Track title
	Artist        string
	Album         string
	Duration      int // milliseconds
	ISRC          string
	MusicBrainzID string // Recording MBID when known
}
