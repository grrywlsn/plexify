package plex

import (
	"strings"
)

// removeBrackets removes text in brackets from a string
func (c *Client) removeBrackets(s string) string {
	// Remove content in parentheses, square brackets, and curly brackets
	// This handles various formats like (feat. Artist), [feat. Artist], {feat. Artist}

	s = reStripParens.ReplaceAllString(s, "")
	s = reStripSquare.ReplaceAllString(s, "")
	s = reStripCurly.ReplaceAllString(s, "")

	s = strings.TrimSpace(s)
	s = reCollapseSpace.ReplaceAllString(s, " ")

	return s
}

// removeFeaturing removes "featuring" and any text after it from a string
func (c *Client) removeFeaturing(s string) string {
	// First, remove featuring inside parentheses like "(feat. X)" or "(featuring X)"
	// This handles cases like "Timeless (feat. Playboi Carti & Doechii) - Remix"
	for _, re := range reFeaturingInParens {
		if re.MatchString(s) {
			s = re.ReplaceAllString(s, "")
			s = strings.TrimSpace(s)
		}
	}

	// Handle various "featuring" formats outside parentheses (case insensitive)
	lowerS := strings.ToLower(s)

	// Check for "featuring" patterns
	patterns := []string{
		" featuring ",
		" feat. ",
		" feat ",
		" ft. ",
		" ft ",
	}

	for _, pattern := range patterns {
		lastIndex := strings.LastIndex(lowerS, pattern)
		if lastIndex != -1 {
			// Return the original string up to the pattern (preserving original case)
			return strings.TrimSpace(s[:lastIndex])
		}
	}

	return s
}

// removeWith removes "with" and any text after it from a string
func (c *Client) removeWith(s string) string {
	// Handle "with" format (case insensitive)
	lowerS := strings.ToLower(s)

	// First check for "with" at the beginning (but only if followed by more text)
	if strings.HasPrefix(lowerS, "with ") && len(lowerS) > 4 {
		result := strings.TrimSpace(s[5:]) // Remove "with " from beginning
		// Clean up trailing dashes and spaces
		result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
		return result
	}

	matches := reWithWord.FindAllStringIndex(lowerS, -1)

	if len(matches) > 0 {
		// Use the last match
		lastMatch := matches[len(matches)-1]

		// Only remove "with" if it's followed by additional text (not at the very end)
		if lastMatch[1] < len(lowerS) {
			// Return the original string up to the match (preserving original case)
			result := strings.TrimSpace(s[:lastMatch[0]])
			// Clean up trailing dashes and spaces
			result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
			return result
		}
	}

	return s
}

// RemoveCommonSuffixes removes common suffixes like "bonus track", "remix", "extended", etc. from track titles.
func (c *Client) RemoveCommonSuffixes(s string) string {
	// Handle common suffixes (case insensitive)
	lowerS := strings.ToLower(s)

	// Common suffixes to remove
	suffixes := []string{
		" - bonus track",
		" - remix",
		" - extended",
		" - radio edit",
		" - single edit",
		" - edit",
		" - version",
		" - live",
		" - acoustic",
		" - instrumental",
		" - demo",
		" - original mix",
		" - club mix",
		" - clean",
		" - explicit",
		" - bonus",
		" - track",
		" - remastered",
		// Soundtrack suffixes
		" - from the motion picture",
		" - from the film",
		" - from the movie",
		" - from the soundtrack",
		" - soundtrack version",
		" - film version",
		" - movie version",
		" (bonus track)",
		" (remix)",
		" (extended)",
		" (radio edit)",
		" (single edit)",
		" (edit)",
		" (version)",
		" (live)",
		" (acoustic)",
		" (instrumental)",
		" (demo)",
		" (original mix)",
		" (club mix)",
		" (clean)",
		" (explicit)",
		" (bonus)",
		" (track)",
		" (remastered)",
		// Soundtrack suffixes in parentheses (handled separately below)
		" (from the soundtrack)",
		" (soundtrack version)",
		" (film version)",
		" (movie version)",
	}

	for _, suffix := range suffixes {
		if strings.HasSuffix(lowerS, strings.ToLower(suffix)) {
			// Return the original string without the suffix (preserving original case)
			result := strings.TrimSpace(s[:len(s)-len(suffix)])
			// Clean up trailing dashes and spaces
			result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
			return result
		}
	}

	for _, pattern := range reYearRemastered {
		if pattern.MatchString(s) {
			// Find the position of the pattern
			matches := pattern.FindStringIndex(s)
			if len(matches) > 0 {
				// Return the original string up to the pattern (preserving original case)
				result := strings.TrimSpace(s[:matches[0]])
				// Clean up trailing dashes and spaces
				result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
				return result
			}
		}
	}

	// Handle movie/film soundtrack patterns with movie names
	// Example: "Friend Of Mine - from the Smurfs Movie Soundtrack"
	// Example: "Song Name - from the Avatar Soundtrack"
	// Example: "Song Name (from the Frozen Movie Soundtrack)"
	for _, pattern := range reMovieSoundtrack {
		if pattern.MatchString(s) {
			// Find the position of the pattern
			matches := pattern.FindStringIndex(s)
			if len(matches) > 0 {
				// Return the original string up to the pattern (preserving original case)
				result := strings.TrimSpace(s[:matches[0]])
				// Clean up trailing dashes and spaces
				result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
				return result
			}
		}
	}

	// Handle generic "From" with quoted title patterns (common on streaming catalogs for soundtracks)
	// Example: 'Save The Day - From "Hoppers"'
	// Example: 'Song Name (From "Movie Title")'
	for _, pattern := range reFromQuotedTitle {
		if pattern.MatchString(s) {
			matches := pattern.FindStringIndex(s)
			if len(matches) > 0 {
				result := strings.TrimSpace(s[:matches[0]])
				result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
				return result
			}
		}
	}

	// Handle streaming service series patterns (Netflix, Hulu, Prime Video, etc.)
	// Example: " - From the Netflix Series "Show Name" Season X"
	for _, pattern := range reStreamingSeries {
		if pattern.MatchString(s) {
			// Find the position of the pattern
			matches := pattern.FindStringIndex(s)
			if len(matches) > 0 {
				// Return the original string up to the pattern (preserving original case)
				result := strings.TrimSpace(s[:matches[0]])
				// Clean up trailing dashes and spaces
				result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
				return result
			}
		}
	}

	// Handle special cases with quotes that need regex matching
	// These patterns can have varying content inside quotes
	soundtrackPatterns := []string{
		" - from the motion picture",
		" - from the film",
		" - from the movie",
		" - love theme from",
		"(from the motion picture",
		"(from the film",
		"(from the movie",
		"(love theme from",
	}

	for _, pattern := range soundtrackPatterns {
		lowerPattern := strings.ToLower(pattern)
		if strings.Contains(lowerS, lowerPattern) {
			// Find the position of the pattern
			patternIndex := strings.Index(lowerS, lowerPattern)
			if patternIndex > 0 {
				// Return the original string up to the pattern (preserving original case)
				result := strings.TrimSpace(s[:patternIndex])
				// Clean up trailing dashes and spaces
				result = strings.TrimSpace(strings.TrimSuffix(result, "-"))
				return result
			}
		}
	}

	return s
}

// normalizeTitle normalizes track titles by handling dashes and case differences
func (c *Client) normalizeTitle(s string) string {
	// Convert to lowercase for case-insensitive comparison
	s = strings.ToLower(s)

	// Replace dashes with parentheses for better matching
	// "Mood Ring (By Demand) - Pride Remix" -> "Mood Ring (By Demand) (Pride Remix)"
	// Handle multiple dashes by replacing each one with a separate set of parentheses
	parts := strings.Split(s, " - ")
	if len(parts) > 1 {
		// Keep the first part as is, wrap each subsequent part in parentheses
		result := parts[0]
		for i := 1; i < len(parts); i++ {
			result += " (" + strings.TrimSpace(parts[i]) + ")"
		}
		s = result
	}

	s = strings.TrimSpace(s)
	s = reCollapseSpace.ReplaceAllString(s, " ")

	return s
}

// normalizePunctuation normalizes various punctuation marks to standard forms
func (c *Client) normalizePunctuation(s string) string {
	// Normalize various types of dashes to standard hyphens
	s = strings.ReplaceAll(s, "\u2010", "-") // En dash to hyphen
	s = strings.ReplaceAll(s, "\u2014", "-") // Em dash to hyphen
	s = strings.ReplaceAll(s, "\u2015", "-") // Horizontal bar to hyphen

	// Normalize multiplication symbol to 'x' for artist names like "Chloe × Halle"
	s = strings.ReplaceAll(s, "\u00D7", "x") // Multiplication symbol to 'x'

	// Normalize various types of apostrophes to standard apostrophes
	s = strings.ReplaceAll(s, "\u2019", "'") // Right single quotation mark to apostrophe
	s = strings.ReplaceAll(s, "\u2018", "'") // Left single quotation mark to apostrophe
	s = strings.ReplaceAll(s, "\u0060", "'") // Grave accent to apostrophe
	s = strings.ReplaceAll(s, "\u2032", "'") // Prime symbol to apostrophe

	// Normalize various types of quotes to standard quotes
	s = strings.ReplaceAll(s, "\u201C", "\"") // Left double quotation mark to straight quote
	s = strings.ReplaceAll(s, "\u201D", "\"") // Right double quotation mark to straight quote
	s = strings.ReplaceAll(s, "\u2018", "'")  // Left single quotation mark to straight quote
	s = strings.ReplaceAll(s, "\u2019", "'")  // Right single quotation mark to straight quote

	// Normalize ellipsis character to three periods
	s = strings.ReplaceAll(s, "\u2026", "...") // Horizontal ellipsis to three periods

	return s
}

// normalizeAccents removes or normalizes accented characters to their base form
func (c *Client) normalizeAccents(s string) string {
	// Common accent mappings for music-related terms
	accentMap := map[rune]rune{
		// Spanish/Portuguese accents - lowercase
		'á': 'a', 'à': 'a', 'â': 'a', 'ã': 'a', 'ä': 'a', 'å': 'a', 'ā': 'a', 'ă': 'a', 'ą': 'a',
		'é': 'e', 'è': 'e', 'ê': 'e', 'ë': 'e', 'ē': 'e', 'ĕ': 'e', 'ė': 'e', 'ę': 'e',
		'í': 'i', 'ì': 'i', 'î': 'i', 'ï': 'i', 'ī': 'i', 'ĭ': 'i', 'į': 'i',
		'ó': 'o', 'ò': 'o', 'ô': 'o', 'õ': 'o', 'ö': 'o', 'ø': 'o', 'ō': 'o', 'ŏ': 'o', 'ő': 'o',
		'ú': 'u', 'ù': 'u', 'û': 'u', 'ü': 'u', 'ū': 'u', 'ŭ': 'u', 'ů': 'u', 'ű': 'u',
		'ý': 'y', 'ÿ': 'y', 'ŷ': 'y',
		'ñ': 'n', 'ń': 'n', 'ņ': 'n', 'ň': 'n',
		'ç': 'c', 'ć': 'c', 'ĉ': 'c', 'ċ': 'c', 'č': 'c',
		'ś': 's', 'ŝ': 's', 'ş': 's', 'š': 's',
		'ź': 'z', 'ż': 'z', 'ž': 'z',
		'ł': 'l', 'ĺ': 'l', 'ļ': 'l', 'ľ': 'l',
		'ř': 'r', 'ŕ': 'r', 'ŗ': 'r',
		'ğ': 'g', 'ģ': 'g', 'ġ': 'g',
		'ḫ': 'h', 'ĥ': 'h', 'ħ': 'h',
		'ḏ': 'd', 'ď': 'd', 'đ': 'd',
		'ṯ': 't', 'ť': 't', 'ţ': 't',
		'ḅ': 'b', 'ḃ': 'b',
		'ṗ': 'p', 'ṕ': 'p',
		'ḳ': 'k', 'ḵ': 'k',
		'ḷ': 'l', 'ḹ': 'l',
		'ṁ': 'm', 'ṃ': 'm',
		'ṅ': 'n', 'ṇ': 'n',
		'ṡ': 's', 'ṣ': 's',
		'ṫ': 't', 'ṭ': 't',
		'ṻ': 'u', 'ṳ': 'u',
		'ṽ': 'v', 'ṿ': 'v',
		'ẁ': 'w', 'ẃ': 'w', 'ẅ': 'w', 'ẇ': 'w', 'ẉ': 'w',
		'ẋ': 'x', 'ẍ': 'x',
		'ỳ': 'y', 'ỹ': 'y', 'ỷ': 'y',
		'ẑ': 'z', 'ẓ': 'z', 'ẕ': 'z',

		// Spanish/Portuguese accents - uppercase
		'Á': 'A', 'À': 'A', 'Â': 'A', 'Ã': 'A', 'Ä': 'A', 'Å': 'A', 'Ā': 'A', 'Ă': 'A', 'Ą': 'A',
		'É': 'E', 'È': 'E', 'Ê': 'E', 'Ë': 'E', 'Ē': 'E', 'Ĕ': 'E', 'Ė': 'E', 'Ę': 'E',
		'Í': 'I', 'Ì': 'I', 'Î': 'I', 'Ï': 'I', 'Ī': 'I', 'Ĭ': 'I', 'Į': 'I',
		'Ó': 'O', 'Ò': 'O', 'Ô': 'O', 'Õ': 'O', 'Ö': 'O', 'Ø': 'O', 'Ō': 'O', 'Ŏ': 'O', 'Ő': 'O',
		'Ú': 'U', 'Ù': 'U', 'Û': 'U', 'Ü': 'U', 'Ū': 'U', 'Ŭ': 'U', 'Ů': 'U', 'Ű': 'U',
		'Ý': 'Y', 'Ÿ': 'Y', 'Ŷ': 'Y',
		'Ñ': 'N', 'Ń': 'N', 'Ņ': 'N', 'Ň': 'N',
		'Ç': 'C', 'Ć': 'C', 'Ĉ': 'C', 'Ċ': 'C', 'Č': 'C',
		'Ś': 'S', 'Ŝ': 'S', 'Ş': 'S', 'Š': 'S',
		'Ź': 'Z', 'Ż': 'Z', 'Ž': 'Z',
		'Ł': 'L', 'Ĺ': 'L', 'Ļ': 'L', 'Ľ': 'L',
		'Ř': 'R', 'Ŕ': 'R', 'Ŗ': 'R',
		'Ğ': 'G', 'Ģ': 'G', 'Ġ': 'G',
		'Ḫ': 'H', 'Ĥ': 'H', 'Ħ': 'H',
		'Ḏ': 'D', 'Ď': 'D', 'Đ': 'D',
		'Ṯ': 'T', 'Ť': 'T', 'Ţ': 'T',
		'Ḅ': 'B', 'Ḃ': 'B',
		'Ṗ': 'P', 'Ṕ': 'P',
		'Ḳ': 'K', 'Ḵ': 'K',
		'Ḷ': 'L', 'Ḹ': 'L',
		'Ṁ': 'M', 'Ṃ': 'M',
		'Ṅ': 'N', 'Ṇ': 'N',
		'Ṡ': 'S', 'Ṣ': 'S',
		'Ṫ': 'T', 'Ṭ': 'T',
		'Ṻ': 'U', 'Ṳ': 'U',
		'Ṽ': 'V', 'Ṿ': 'V',
		'Ẁ': 'W', 'Ẃ': 'W', 'Ẅ': 'W', 'Ẇ': 'W', 'Ẉ': 'W',
		'Ẋ': 'X', 'Ẍ': 'X',
		'Ỳ': 'Y', 'Ỹ': 'Y', 'Ỷ': 'Y',
		'Ẑ': 'Z', 'Ẓ': 'Z', 'Ẕ': 'Z',
	}

	result := make([]rune, 0, len(s))
	for _, r := range s {
		if replacement, exists := accentMap[r]; exists {
			result = append(result, replacement)
		} else {
			result = append(result, r)
		}
	}

	return string(result)
}
