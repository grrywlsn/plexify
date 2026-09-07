package track

import "strings"

// ArtistMatchTier describes the authority of an artist name used in a Plex
// search. Aliases are deliberately the final, lower-confidence tier.
type ArtistMatchTier uint8

const (
	ArtistMatchPrimary ArtistMatchTier = iota
	ArtistMatchCredit
	ArtistMatchAlias
)

// PlexSearchArtistCandidate is one ordered artist query and its confidence tier.
type PlexSearchArtistCandidate struct {
	Name string
	Tier ArtistMatchTier
}

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
// When MusicBrainzArtistCredits is set (music-social musicbrainz.artist_credits), each distinct
// credit name is appended next. MusicBrainz aliases are appended last.
func (t Track) PlexSearchArtistMatchCandidates() []PlexSearchArtistCandidate {
	seen := make(map[string]struct{})
	var out []PlexSearchArtistCandidate
	appendUnique := func(s string, tier ArtistMatchTier) {
		s = strings.TrimSpace(s)
		if s == "" {
			return
		}
		k := strings.ToLower(s)
		if _, ok := seen[k]; ok {
			return
		}
		seen[k] = struct{}{}
		out = append(out, PlexSearchArtistCandidate{Name: s, Tier: tier})
	}

	full := strings.TrimSpace(t.Artist)
	if full != "" {
		primary := PrimaryListedArtist(t.Artist)
		appendUnique(primary, ArtistMatchPrimary)
		if primary != full {
			appendUnique(full, ArtistMatchPrimary)
		}
	}
	for _, name := range t.MusicBrainzArtistCredits {
		appendUnique(name, ArtistMatchCredit)
	}
	for _, name := range t.MusicBrainzArtistAliases {
		appendUnique(name, ArtistMatchAlias)
	}
	if len(out) == 0 {
		return []PlexSearchArtistCandidate{{Name: "", Tier: ArtistMatchPrimary}}
	}
	return out
}

// PlexSearchArtistCandidates returns the ordered artist query strings. It is
// retained for callers that do not need tier metadata.
func (t Track) PlexSearchArtistCandidates() []string {
	candidates := t.PlexSearchArtistMatchCandidates()
	out := make([]string, len(candidates))
	for i, candidate := range candidates {
		out[i] = candidate.Name
	}
	return out
}
