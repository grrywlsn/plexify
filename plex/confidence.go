package plex

import (
	"fmt"
	"math"
	"strings"

	"github.com/grrywlsn/plexify/track"
)

// formatConfidencePercent renders a 0–1 score as a whole percent for user-facing output (e.g. 0.8 → "80%", 1.0 → "100%").
func formatConfidencePercent(x float64) string {
	if math.IsNaN(x) || math.IsInf(x, 0) {
		return "0%"
	}
	if x < 0 {
		x = 0
	}
	if x > 1 {
		x = 1
	}
	p := int(math.Round(x * 100))
	if p < 0 {
		p = 0
	}
	if p > 100 {
		p = 100
	}
	return fmt.Sprintf("%d%%", p)
}

func intAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// artistSimilarityTitleArtistForPlexField mirrors the song↔Plex artist comparisons in MatchTypeTitleArtist for one Plex artist string (album or track).
func (c *Client) artistSimilarityTitleArtistForPlexField(song track.Track, plexField string) float64 {
	plexLower := strings.ToLower(plexField)
	artistSimilarity := c.calculateStringSimilarity(strings.ToLower(track.PrimaryListedArtist(song.Artist)), plexLower)
	if full := strings.ToLower(strings.TrimSpace(song.Artist)); full != "" {
		if sim := c.calculateStringSimilarity(full, plexLower); sim > artistSimilarity {
			artistSimilarity = sim
		}
	}
	if v := c.calculateStringSimilarity(
		strings.ToLower(c.removeFeaturing(song.Artist)),
		strings.ToLower(c.removeFeaturing(plexField)),
	); v > artistSimilarity {
		artistSimilarity = v
	}
	if v := c.calculateStringSimilarity(
		strings.ToLower(c.removeFeaturing(track.PrimaryListedArtist(song.Artist))),
		strings.ToLower(c.removeFeaturing(plexField)),
	); v > artistSimilarity {
		artistSimilarity = v
	}
	return artistSimilarity
}

// bestArtistSimilarityTitleArtist returns the best 0–1 match over grandparent, originalTitle, and GrandparentTitleSort.
func (c *Client) bestArtistSimilarityTitleArtist(song track.Track, plex *PlexTrack) float64 {
	var best float64
	if s := strings.TrimSpace(plex.Artist); s != "" {
		best = c.artistSimilarityTitleArtistForPlexField(song, s)
	}
	if s := strings.TrimSpace(plex.OriginalTitle); s != "" {
		if v := c.artistSimilarityTitleArtistForPlexField(song, s); v > best {
			best = v
		}
	}
	if s := strings.TrimSpace(plex.GrandparentTitleSort); s != "" {
		if v := c.artistSimilarityTitleArtistForPlexField(song, s); v > best {
			best = v
		}
	}
	return best
}

// calculateConfidence calculates a confidence score for the match
func (c *Client) calculateConfidence(song track.Track, plexTrack *PlexTrack, matchType MatchKind) float64 {
	if plexTrack == nil {
		return 0.0
	}

	switch matchType {
	case MatchTypeTitleArtist:
		titleSimilarity := c.calculateStringSimilarity(strings.ToLower(song.Name), strings.ToLower(plexTrack.Title))
		artistSimilarity := c.bestArtistSimilarityTitleArtist(song, plexTrack)

		titleVariantPairs := []struct{ a, b string }{
			{strings.ToLower(c.removeBrackets(song.Name)), strings.ToLower(c.removeBrackets(plexTrack.Title))},
			{strings.ToLower(c.removeFeaturing(song.Name)), strings.ToLower(c.removeFeaturing(plexTrack.Title))},
			{strings.ToLower(c.normalizeTitle(song.Name)), strings.ToLower(c.normalizeTitle(plexTrack.Title))},
			{strings.ToLower(c.removeWith(song.Name)), strings.ToLower(c.removeWith(plexTrack.Title))},
			{strings.ToLower(c.RemoveCommonSuffixes(song.Name)), strings.ToLower(c.RemoveCommonSuffixes(plexTrack.Title))},
			{strings.ToLower(c.normalizeAccents(song.Name)), strings.ToLower(c.normalizeAccents(plexTrack.Title))},
		}
		for _, p := range titleVariantPairs {
			sim := c.calculateStringSimilarity(p.a, p.b)
			if sim > titleSimilarity {
				titleSimilarity = sim
			}
		}

		// Blend album only when both sides have album metadata (avoid punishing missing Plex parentTitle).
		if strings.TrimSpace(song.Album) != "" && strings.TrimSpace(plexTrack.Album) != "" {
			albumSim := c.bestAlbumSimilarity(song.Album, plexTrack.Album)
			return (titleSimilarity * 0.55) + (artistSimilarity * 0.25) + (albumSim * 0.20)
		}
		return (titleSimilarity * 0.7) + (artistSimilarity * 0.3)
	default:
		return 0.0
	}
}

// calculateStringSimilarity calculates similarity between two strings
func (c *Client) calculateStringSimilarity(s1, s2 string) float64 {
	if s1 == s2 {
		return 1.0
	}

	if s1 == "" || s2 == "" {
		return 0.0
	}

	if strings.Contains(s1, s2) || strings.Contains(s2, s1) {
		longer := s1
		shorter := s2
		if len(s2) > len(s1) {
			longer = s2
			shorter = s1
		}
		return float64(len(shorter)) / float64(len(longer))
	}

	words1 := strings.Fields(s1)
	words2 := strings.Fields(s2)

	if len(words1) == 0 || len(words2) == 0 {
		return 0.0
	}

	matchingWords := 0
	for _, word1 := range words1 {
		for _, word2 := range words2 {
			if word1 == word2 {
				matchingWords++
				break
			}
		}
	}

	wordSimilarity := float64(matchingWords) / float64(max(len(words1), len(words2)))
	lengthSimilarity := 1.0 - float64(intAbs(len(s1)-len(s2)))/float64(max(len(s1), len(s2)))

	return (wordSimilarity * 0.7) + (lengthSimilarity * 0.3)
}
