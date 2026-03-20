package track

import "strings"

// PrimaryListedArtist returns the first comma-separated segment when s lists multiple
// artists (e.g. music-social.com "Le Youth, Forester, Robertson" → "Le Youth").
// If there is no comma, returns strings.TrimSpace(s). Empty input returns "".
func PrimaryListedArtist(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	parts := strings.Split(s, ",")
	if len(parts) == 1 {
		return s
	}
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			return t
		}
	}
	return s
}

// PlexSearchArtistCandidates returns artist strings to try against Plex, in order.
// The primary (first comma-separated) name is tried first so lookups match typical
// single-artist Plex metadata; the full original string is tried second when it
// differs (fallback for band names that legitimately contain commas).
func (t Track) PlexSearchArtistCandidates() []string {
	full := strings.TrimSpace(t.Artist)
	if full == "" {
		return []string{""}
	}
	primary := PrimaryListedArtist(t.Artist)
	if primary == full {
		return []string{full}
	}
	return []string{primary, full}
}
