package track

import "strings"

// PrimaryListedArtist returns the first listed artist when s contains multiple names.
// Comma-separated lists (typical on music-social.com) use the first segment, e.g.
// "Le Youth, Forester, Robertson" → "Le Youth".
// Ampersand collaborations use the first segment before " & ", e.g.
// "SOPHIE & Bibi Bourelly" → "SOPHIE" (Plex often stores only the headliner).
// If there is no such delimiter, returns strings.TrimSpace(s). Empty input returns "".
func PrimaryListedArtist(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	parts := strings.Split(s, ",")
	if len(parts) == 1 {
		return primaryBeforeAmpersand(s)
	}
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			return primaryBeforeAmpersand(t)
		}
	}
	return s
}

// primaryBeforeAmpersand returns the segment before the first " & " when s lists
// multiple artists that way; otherwise returns s (already trimmed).
func primaryBeforeAmpersand(s string) string {
	seg := strings.SplitN(s, " & ", 2)
	if len(seg) < 2 {
		return s
	}
	if first := strings.TrimSpace(seg[0]); first != "" {
		return first
	}
	return s
}

// PlexSearchArtistCandidates returns artist strings to try against Plex, in order.
// The primary (first comma- or ampersand-listed) name is tried first so lookups match
// typical single-artist Plex metadata; the full original string is tried second when it
// differs (fallback for band names that legitimately contain commas or "&").
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
