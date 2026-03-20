package plex

import (
	"strings"

	"github.com/grrywlsn/plexify/track"
)

func intAbs(x int) int {
	if x < 0 {
		return -x
	}
	return x
}

// calculateConfidence calculates a confidence score for the match
func (c *Client) calculateConfidence(song track.Track, plexTrack *PlexTrack, matchType MatchKind) float64 {
	if plexTrack == nil {
		return 0.0
	}

	switch matchType {
	case MatchTypeTitleArtist:
		titleSimilarity := c.calculateStringSimilarity(strings.ToLower(song.Name), strings.ToLower(plexTrack.Title))
		artistSimilarity := c.calculateStringSimilarity(strings.ToLower(song.Artist), strings.ToLower(plexTrack.Artist))

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

		featuringArtistSimilarity := c.calculateStringSimilarity(
			strings.ToLower(c.removeFeaturing(song.Artist)),
			strings.ToLower(c.removeFeaturing(plexTrack.Artist)),
		)
		if featuringArtistSimilarity > artistSimilarity {
			artistSimilarity = featuringArtistSimilarity
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
