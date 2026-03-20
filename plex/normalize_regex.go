package plex

import "regexp"

// Precompiled patterns for title normalization (avoid MustCompile per call).
var (
	reStripParens       = regexp.MustCompile(`\([^)]*\)`)
	reStripSquare       = regexp.MustCompile(`\[[^\]]*\]`)
	reStripCurly        = regexp.MustCompile(`\{[^}]*\}`)
	reCollapseSpace     = regexp.MustCompile(`\s+`)
	reFeaturingInParens = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s*\(feat\.?\s+[^)]+\)`),
		regexp.MustCompile(`(?i)\s*\(featuring\s+[^)]+\)`),
		regexp.MustCompile(`(?i)\s*\(ft\.?\s+[^)]+\)`),
	}
	reWithWord = regexp.MustCompile(`(?i)\bwith\b`)

	reYearRemastered = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s*-\s*\d{4}\s+remastered\s*$`),
		regexp.MustCompile(`(?i)\s*\(\s*\d{4}\s+remastered\s*\)\s*$`),
	}
	reMovieSoundtrack = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s*-\s*from\s+the\s+.+\s+movie\s+soundtrack\s*$`),
		regexp.MustCompile(`(?i)\s*-\s*from\s+the\s+.+\s+soundtrack\s*$`),
		regexp.MustCompile(`(?i)\s*-\s*from\s+.+\s+soundtrack\s*$`),
		regexp.MustCompile(`(?i)\s*\(\s*from\s+the\s+.+\s+movie\s+soundtrack\s*\)\s*$`),
		regexp.MustCompile(`(?i)\s*\(\s*from\s+the\s+.+\s+soundtrack\s*\)\s*$`),
		regexp.MustCompile(`(?i)\s*\(\s*from\s+.+\s+soundtrack\s*\)\s*$`),
	}
	reFromQuotedTitle = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s*-\s*from\s+"[^"]*"\s*$`),
		regexp.MustCompile(`(?i)\s*\(\s*from\s+"[^"]*"\s*\)\s*$`),
		regexp.MustCompile("(?i)\\s*-\\s*from\\s+\u201C[^\u201D]*\u201D\\s*$"),
		regexp.MustCompile("(?i)\\s*\\(\\s*from\\s+\u201C[^\u201D]*\u201D\\s*\\)\\s*$"),
	}
	reStreamingSeries = []*regexp.Regexp{
		regexp.MustCompile(`(?i)\s*-\s*from\s+the\s+(netflix|hulu|prime video|apple tv|disney|paramount|hbo)\s+(series|show)\s+.*$`),
		regexp.MustCompile(`(?i)\s*\(\s*from\s+the\s+(netflix|hulu|prime video|apple tv|disney|paramount|hbo)\s+(series|show)\s+.*\)\s*$`),
	}
)
