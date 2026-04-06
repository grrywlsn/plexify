package plex

import (
	"context"
	"encoding/xml"
	"fmt"
	"log/slog"
	"math"
	"net/http"
	"net/url"
	"strings"

	"github.com/grrywlsn/plexify/track"
)

// searchPhase orders cheap Plex /search attempts: one combined query, then separate title and artist queries.
type searchPhase int

const (
	searchPhaseCombined searchPhase = iota
	searchPhaseTitleArtist
)

func (p searchPhase) tierLabel() string {
	switch p {
	case searchPhaseCombined:
		return "combined-query"
	case searchPhaseTitleArtist:
		return "title-or-artist-search"
	default:
		return "unknown"
	}
}

type trackSearchStrategy struct {
	name string
	fn   func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error)
}

// SearchTrack searches for a track in Plex using title/artist matching.
// It uses a tiered pipeline: all strategies run with combined-query search first, then again with
// title/artist search; only if still unmatched and SkipFullLibrarySearch is false, it scans /all.
// With ExactMatchesOnly, only the first strategy (raw source title/artist) runs and full-library scan is skipped.
//
// When the source artist field lists multiple names separated by commas (typical on music-social.com),
// the primary (first) name is used first for Plex queries, then the full string is retried if needed.
func (c *Client) SearchTrack(ctx context.Context, song track.Track) (*PlexTrack, MatchKind, error) {
	if err := ctx.Err(); err != nil {
		return nil, MatchTypeError, fmt.Errorf("search cancelled: %w", err)
	}

	candidates := song.PlexSearchArtistCandidates()
	for i, searchArtist := range candidates {
		if i > 0 {
			c.debugLog("🔍 SearchTrack: no match with primary artist; retrying with full artist field %q", searchArtist)
		}
		found, err := c.searchTrackWithArtist(ctx, song, searchArtist)
		if err != nil {
			return nil, MatchTypeError, err
		}
		if found != nil {
			return found, MatchTypeTitleArtist, nil
		}
	}

	return nil, MatchTypeNone, nil
}

// searchTrackWithArtist runs the search pipeline for a single artist string (title still from song).
func (c *Client) searchTrackWithArtist(ctx context.Context, song track.Track, artist string) (*PlexTrack, error) {
	c.debugLog("🔍 SearchTrack: searching for '%s' by '%s' (source artist field: %q)", song.Name, artist, song.Artist)

	indexedStrategies := c.indexedTrackSearchStrategies()
	if c.exactMatchesOnly {
		indexedStrategies = indexedStrategies[:1]
		c.debugLog("🔍 SearchTrack: exact-matches-only (raw title/artist only, no /all)")
	}

	for _, phase := range []searchPhase{searchPhaseCombined, searchPhaseTitleArtist} {
		for _, strategy := range indexedStrategies {
			if err := ctx.Err(); err != nil {
				return nil, fmt.Errorf("search cancelled: %w", err)
			}
			tr, err := strategy.fn(ctx, phase, song.Name, artist, song.Album)
			if err != nil {
				return nil, err
			}
			if tr != nil {
				slog.Info(fmt.Sprintf("✅ SearchTrack: found match '%s' by '%s' using %s [%s tier]", tr.Title, tr.Artist, strategy.name, phase.tierLabel()))
				return tr, nil
			}
		}
	}

	if !c.skipFullLibrarySearch && !c.exactMatchesOnly {
		if err := ctx.Err(); err != nil {
			return nil, fmt.Errorf("search cancelled: %w", err)
		}
		c.debugLog("🔍 SearchTrack: trying full library search for '%s' by '%s'", song.Name, artist)
		tr, err := c.searchEntireLibrary(ctx, song.Name, artist, song.Album)
		if err != nil {
			return nil, err
		}
		if tr != nil {
			slog.Info(fmt.Sprintf("✅ SearchTrack: found match '%s' by '%s' using full library search", tr.Title, tr.Artist))
			return tr, nil
		}
	}

	return nil, nil
}

func (c *Client) indexedTrackSearchStrategies() []trackSearchStrategy {
	return []trackSearchStrategy{
		{"exact title/artist", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			return c.trySearchVariationsPhase(ctx, title, artist, sourceAlbum, phase)
		}},
		{"single quote variations", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			if strings.Contains(title, "'") || strings.Contains(artist, "'") ||
				strings.Contains(title, "'") || strings.Contains(artist, "'") {
				return c.searchByTitleWithSingleQuoteVariationsPhase(ctx, title, artist, sourceAlbum, phase)
			}
			return nil, nil
		}},
		{"brackets removed", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			cleanTitle := c.removeBrackets(title)
			if cleanTitle != title {
				return c.trySearchVariationsPhase(ctx, cleanTitle, artist, sourceAlbum, phase)
			}
			return nil, nil
		}},
		{"featuring removed", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			featuringTitle := c.removeFeaturing(title)
			if featuringTitle != title && featuringTitle != c.removeBrackets(title) {
				return c.trySearchVariationsPhase(ctx, featuringTitle, artist, sourceAlbum, phase)
			}
			return nil, nil
		}},
		{"featuring removed + normalized", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			featuringTitle := c.removeFeaturing(title)
			if featuringTitle != title {
				normalizedFeaturingTitle := c.normalizeTitle(featuringTitle)
				if normalizedFeaturingTitle != featuringTitle {
					c.debugLog("🔍 SearchTrack: trying featuring-removed + normalized '%s' for '%s' by '%s'", normalizedFeaturingTitle, title, artist)
					return c.trySearchVariationsPhase(ctx, normalizedFeaturingTitle, artist, sourceAlbum, phase)
				}
			}
			return nil, nil
		}},
		{"artist featuring removed", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			featuringArtist := c.removeFeaturing(artist)
			if featuringArtist != artist {
				c.debugLog("🔍 SearchTrack: trying artist featuring-removed '%s' by '%s' for '%s' by '%s'", title, featuringArtist, title, artist)
				return c.trySearchVariationsPhase(ctx, title, featuringArtist, sourceAlbum, phase)
			}
			return nil, nil
		}},
		{"normalized title", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			normalizedTitle := c.normalizeTitle(title)
			if normalizedTitle != title && normalizedTitle != c.removeBrackets(title) && normalizedTitle != c.removeFeaturing(title) {
				return c.trySearchVariationsPhase(ctx, normalizedTitle, artist, sourceAlbum, phase)
			}
			return nil, nil
		}},
		{"with removed", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			withTitle := c.removeWith(title)
			if withTitle != title && withTitle != c.removeBrackets(title) && withTitle != c.removeFeaturing(title) && withTitle != c.normalizeTitle(title) {
				return c.trySearchVariationsPhase(ctx, withTitle, artist, sourceAlbum, phase)
			}
			return nil, nil
		}},
		{"suffixes removed", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			suffixTitle := c.RemoveCommonSuffixes(title)
			if suffixTitle != title && suffixTitle != c.removeBrackets(title) && suffixTitle != c.removeFeaturing(title) && suffixTitle != c.normalizeTitle(title) && suffixTitle != c.removeWith(title) {
				c.debugLog("🔍 SearchTrack: trying suffix-removed title '%s' for '%s' by '%s'", suffixTitle, title, artist)
				return c.trySearchVariationsPhase(ctx, suffixTitle, artist, sourceAlbum, phase)
			}
			return nil, nil
		}},
		{"accent normalization", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			accentTitle := c.normalizeAccents(title)
			accentArtist := c.normalizeAccents(artist)
			if accentTitle != title || accentArtist != artist {
				c.debugLog("🔍 SearchTrack: trying accent-normalized '%s' by '%s' for '%s' by '%s'", accentTitle, accentArtist, title, artist)
				return c.trySearchVariationsPhase(ctx, accentTitle, accentArtist, sourceAlbum, phase)
			}
			return nil, nil
		}},
	}
}

// trySearchVariationsPhase runs one tier of indexed /search: combined query, or title then artist.
func (c *Client) trySearchVariationsPhase(ctx context.Context, title, artist, sourceAlbum string, phase searchPhase) (*PlexTrack, error) {
	switch phase {
	case searchPhaseCombined:
		return c.searchByCombinedQuery(ctx, title, artist, sourceAlbum)
	case searchPhaseTitleArtist:
		if track, err := c.searchByTitle(ctx, title, artist, sourceAlbum); err != nil {
			return nil, err
		} else if track != nil {
			return track, nil
		}
		return c.searchByArtist(ctx, title, artist, sourceAlbum)
	default:
		return nil, nil
	}
}

// searchByTitle searches for tracks by title in the music library
func (c *Client) searchByTitle(ctx context.Context, title, artist, sourceAlbum string) (*PlexTrack, error) {

	// Use the library search endpoint
	reqURL := fmt.Sprintf("%s/library/sections/%d/search", c.baseURL, c.sectionID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("query", title)
	params.Add("type", PlexMusicTrackType) // Type 10 = music tracks

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create search request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("plex search API returned status %d", resp.StatusCode)
	}

	var searchResp PlexResponse
	if err := xml.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}

	// Find best match among search results
	slog.Info(fmt.Sprintf("🔍 searchByTitle: searching for '%s' by '%s', found %d results", title, artist, len(searchResp.Tracks)))
	if len(searchResp.Tracks) > 0 && c.debug {
		for i, track := range searchResp.Tracks {
			c.debugLog("  Result %d: '%s' by '%s' (ID: %s)", i+1, track.Title, track.Artist, track.ID)
		}
	}
	result := c.FindBestMatch(searchResp.Tracks, title, artist, sourceAlbum)
	if result != nil {
		slog.Info(fmt.Sprintf("✅ searchByTitle: found match '%s' by '%s'", result.Title, result.Artist))
	} else {
		c.debugLog("❌ searchByTitle: no match found")
	}
	return result, nil
}

// searchByArtist searches for tracks by artist in the music library
func (c *Client) searchByArtist(ctx context.Context, title, artist, sourceAlbum string) (*PlexTrack, error) {

	// Use the library search endpoint with artist query
	reqURL := fmt.Sprintf("%s/library/sections/%d/search", c.baseURL, c.sectionID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("query", artist)
	params.Add("type", PlexMusicTrackType) // Type 10 = music tracks

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create artist search request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make artist search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("plex artist search API returned status %d", resp.StatusCode)
	}

	var searchResp PlexResponse
	if err := xml.NewDecoder(resp.Body).Decode(&searchResp); err != nil {
		return nil, fmt.Errorf("failed to decode artist search response: %w", err)
	}

	// Find best match among search results
	result := c.FindBestMatch(searchResp.Tracks, title, artist, sourceAlbum)
	if result != nil {
		slog.Info(fmt.Sprintf("✅ searchByArtist: found match '%s' by '%s'", result.Title, result.Artist))
	}
	return result, nil
}

// searchByCombinedQuery searches using a combined title + artist query (most efficient)
func (c *Client) searchByCombinedQuery(ctx context.Context, title, artist, sourceAlbum string) (*PlexTrack, error) {
	// Try the most likely combination first
	query := fmt.Sprintf("%s %s", title, artist)

	reqURL := fmt.Sprintf("%s/library/sections/%d/search", c.baseURL, c.sectionID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("query", query)
	params.Add("type", PlexMusicTrackType) // Type 10 = music tracks

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create combined search request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make combined search request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == StatusOK {
		var searchResp PlexResponse
		if err := xml.NewDecoder(resp.Body).Decode(&searchResp); err == nil {
			if track := c.FindBestMatch(searchResp.Tracks, title, artist, sourceAlbum); track != nil {
				slog.Info(fmt.Sprintf("✅ searchByCombinedQuery: found match '%s' by '%s'", track.Title, track.Artist))
				return track, nil
			}
		}
	}

	return nil, nil
}

// searchByTitleWithSingleQuoteVariationsPhase runs one indexed tier for apostrophe title variations.
func (c *Client) searchByTitleWithSingleQuoteVariationsPhase(ctx context.Context, title, artist, sourceAlbum string, phase searchPhase) (*PlexTrack, error) {
	hasStandardApostrophe := strings.Contains(title, "'")
	hasCurlyApostrophe := strings.Contains(title, "'")

	if !hasStandardApostrophe && !hasCurlyApostrophe {
		return c.trySearchVariationsPhase(ctx, title, artist, sourceAlbum, phase)
	}

	seen := make(map[string]bool)
	var variations []string

	addVariation := func(v string) {
		if v != "" && !seen[v] {
			seen[v] = true
			variations = append(variations, v)
		}
	}

	addVariation(title)
	addVariation(strings.ReplaceAll(title, "'", "'"))
	addVariation(strings.ReplaceAll(title, "'", "'"))
	noApostrophe := strings.ReplaceAll(strings.ReplaceAll(title, "'", ""), "'", "")
	addVariation(noApostrophe)

	for _, variation := range variations {
		track, err := c.trySearchVariationsPhase(ctx, variation, artist, sourceAlbum, phase)
		if err != nil {
			return nil, err
		}
		if track != nil {
			return track, nil
		}
	}

	return nil, nil
}

// searchByTitleWithSingleQuoteVariations searches for tracks with single quotes by trying different variations.
// Note: Plex's search API often doesn't handle special apostrophe characters well; full-library scan may still be needed unless fast search is on.
func (c *Client) searchByTitleWithSingleQuoteVariations(ctx context.Context, title, artist, sourceAlbum string) (*PlexTrack, error) {
	if track, err := c.searchByTitleWithSingleQuoteVariationsPhase(ctx, title, artist, sourceAlbum, searchPhaseCombined); err != nil || track != nil {
		return track, err
	}
	return c.searchByTitleWithSingleQuoteVariationsPhase(ctx, title, artist, sourceAlbum, searchPhaseTitleArtist)
}

// searchEntireLibrary is a fallback method that searches through all tracks in the library
// This is used when the regular search methods fail to find tracks that should exist
func (c *Client) searchEntireLibrary(ctx context.Context, title, artist, sourceAlbum string) (*PlexTrack, error) {
	// Get all tracks from the library
	reqURL := fmt.Sprintf("%s/library/sections/%d/all", c.baseURL, c.sectionID)
	params := url.Values{}
	params.Add("X-Plex-Token", c.token)
	params.Add("type", PlexMusicTrackType) // Type 10 = music tracks

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create library request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make library request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != StatusOK {
		return nil, fmt.Errorf("plex library API returned status %d", resp.StatusCode)
	}

	var libraryResp PlexResponse
	if err := xml.NewDecoder(resp.Body).Decode(&libraryResp); err != nil {
		return nil, fmt.Errorf("failed to decode library response: %w", err)
	}

	// Find best match among all tracks
	c.debugLog("🔍 searchEntireLibrary: searching for '%s' by '%s' in entire library (%d tracks)", title, artist, len(libraryResp.Tracks))
	result := c.FindBestMatch(libraryResp.Tracks, title, artist, sourceAlbum)
	if result != nil {
		slog.Info(fmt.Sprintf("✅ searchEntireLibrary: found match '%s' by '%s' for search '%s' by '%s'", result.Title, result.Artist, title, artist))
	} else {
		slog.Info(fmt.Sprintf("❌ searchEntireLibrary: no match found for search '%s' by '%s'", title, artist))
	}
	return result, nil
}

const scoreEqEps = 1e-9

// bestAlbumSimilarity returns 0 if sourceAlbum is empty; otherwise the best string similarity
// between source and Plex album fields using a few normalizations (title-style).
func (c *Client) bestAlbumSimilarity(sourceAlbum, plexAlbum string) float64 {
	if strings.TrimSpace(sourceAlbum) == "" {
		return 0
	}
	s := strings.ToLower(strings.TrimSpace(sourceAlbum))
	p := strings.ToLower(strings.TrimSpace(plexAlbum))
	if p == "" {
		return 0
	}
	best := c.calculateStringSimilarity(s, p)
	if v := c.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(c.normalizeTitle(sourceAlbum))),
		strings.ToLower(strings.TrimSpace(c.normalizeTitle(plexAlbum))),
	); v > best {
		best = v
	}
	if v := c.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(c.removeBrackets(sourceAlbum))),
		strings.ToLower(strings.TrimSpace(c.removeBrackets(plexAlbum))),
	); v > best {
		best = v
	}
	if v := c.calculateStringSimilarity(
		strings.ToLower(strings.TrimSpace(c.normalizeAccents(sourceAlbum))),
		strings.ToLower(strings.TrimSpace(c.normalizeAccents(plexAlbum))),
	); v > best {
		best = v
	}
	return best
}

// FindBestMatch finds the best matching track from search results. When sourceAlbum is non-empty,
// album similarity is blended into the score so duplicate title/artist releases can be disambiguated.
func (c *Client) FindBestMatch(tracks []PlexTrack, title, artist, sourceAlbum string) *PlexTrack {
	if len(tracks) == 0 {
		return nil
	}

	titleLower := strings.ToLower(strings.TrimSpace(title))
	artistLower := strings.ToLower(strings.TrimSpace(artist))
	sourceAlbumTrim := strings.TrimSpace(sourceAlbum)
	useAlbumInScore := sourceAlbumTrim != ""

	c.debugLog("🔍 FindBestMatch: searching for '%s' by '%s' among %d tracks", title, artist, len(tracks))

	var exactMatches []PlexTrack
	for _, tr := range tracks {
		trackTitle := strings.ToLower(strings.TrimSpace(tr.Title))
		trackArtist := strings.ToLower(strings.TrimSpace(tr.Artist))
		if titleLower == trackTitle && artistLower == trackArtist {
			exactMatches = append(exactMatches, tr)
		}
	}
	switch len(exactMatches) {
	case 1:
		t := exactMatches[0]
		c.debugLog("✅ FindBestMatch: single exact match '%s' by '%s'", t.Title, t.Artist)
		return &t
	case 0:
		// fall through to similarity scoring
	default:
		if useAlbumInScore {
			var best PlexTrack
			var bestAl float64 = -1
			for _, tr := range exactMatches {
				al := c.bestAlbumSimilarity(sourceAlbum, tr.Album)
				if bestAl < 0 || al > bestAl+scoreEqEps {
					bestAl = al
					best = tr
				}
			}
			t := best
			c.debugLog("✅ FindBestMatch: multiple exact title/artist; picked by album (album similarity %s)", formatConfidencePercent(bestAl))
			return &t
		}
		// Multiple exact matches and no source album: use full scoring below.
	}

	var bestMatch *PlexTrack
	var bestScore float64
	var bestArtistSimilarity float64
	var bestAlbumSimilarity float64 = -1

	for _, track := range tracks {
		trackTitle := strings.ToLower(strings.TrimSpace(track.Title))
		trackArtist := strings.ToLower(strings.TrimSpace(track.Artist))

		// Calculate similarity scores with original titles
		titleSimilarity := c.calculateStringSimilarity(titleLower, trackTitle)
		artistSimilarity := c.calculateStringSimilarity(artistLower, trackArtist)

		c.debugLog("🔍 FindBestMatch: '%s' by '%s' -> '%s' by '%s'", title, artist, track.Title, track.Artist)
		c.debugLog("   Original title similarity: %s ('%s' vs '%s')", formatConfidencePercent(titleSimilarity), titleLower, trackTitle)
		c.debugLog("   Original artist similarity: %s ('%s' vs '%s')", formatConfidencePercent(artistSimilarity), artistLower, trackArtist)

		// Also try with normalized punctuation for artist matching
		punctuationArtistLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(artist)))
		punctuationTrackArtistLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(track.Artist)))
		punctuationArtistSimilarity := c.calculateStringSimilarity(punctuationArtistLower, punctuationTrackArtistLower)

		// Also try with accent normalization for artist matching
		accentArtistLower := strings.ToLower(strings.TrimSpace(c.normalizeAccents(artist)))
		accentTrackArtistLower := strings.ToLower(strings.TrimSpace(c.normalizeAccents(track.Artist)))
		accentArtistSimilarity := c.calculateStringSimilarity(accentArtistLower, accentTrackArtistLower)

		// Also try with featuring removed for artist matching
		featuringArtistLower := strings.ToLower(strings.TrimSpace(c.removeFeaturing(artist)))
		featuringTrackArtistLower := strings.ToLower(strings.TrimSpace(c.removeFeaturing(track.Artist)))
		featuringArtistSimilarity := c.calculateStringSimilarity(featuringArtistLower, featuringTrackArtistLower)

		// Use the better artist similarity
		if punctuationArtistSimilarity > artistSimilarity {
			c.debugLog("   Using normalized artist similarity: %s (was %s)", formatConfidencePercent(punctuationArtistSimilarity), formatConfidencePercent(artistSimilarity))
			artistSimilarity = punctuationArtistSimilarity
		}
		if accentArtistSimilarity > artistSimilarity {
			c.debugLog("   Using accent-normalized artist similarity: %s (was %s)", formatConfidencePercent(accentArtistSimilarity), formatConfidencePercent(artistSimilarity))
			artistSimilarity = accentArtistSimilarity
		}
		if featuringArtistSimilarity > artistSimilarity {
			c.debugLog("   Using featuring-removed artist similarity: %s (was %s)", formatConfidencePercent(featuringArtistSimilarity), formatConfidencePercent(artistSimilarity))
			artistSimilarity = featuringArtistSimilarity
		}

		// Also try with cleaned titles (without brackets) for better matching
		cleanTitleLower := strings.ToLower(strings.TrimSpace(c.removeBrackets(title)))
		cleanTrackTitle := strings.ToLower(strings.TrimSpace(c.removeBrackets(track.Title)))

		// Calculate similarity with cleaned titles
		cleanTitleSimilarity := c.calculateStringSimilarity(cleanTitleLower, cleanTrackTitle)
		c.debugLog("   Clean title similarity: %s ('%s' vs '%s')", formatConfidencePercent(cleanTitleSimilarity), cleanTitleLower, cleanTrackTitle)

		// Also try with featuring removed for better matching
		featuringTitleLower := strings.ToLower(strings.TrimSpace(c.removeFeaturing(title)))
		featuringTrackTitleLower := strings.ToLower(strings.TrimSpace(c.removeFeaturing(track.Title)))

		// Calculate similarity with featuring removed
		featuringTitleSimilarity := c.calculateStringSimilarity(featuringTitleLower, featuringTrackTitleLower)
		c.debugLog("   Featuring-removed title similarity: %s ('%s' vs '%s')", formatConfidencePercent(featuringTitleSimilarity), featuringTitleLower, featuringTrackTitleLower)

		// Also try with normalized titles for better matching
		normalizedTitleLower := strings.ToLower(strings.TrimSpace(c.normalizeTitle(title)))
		normalizedTrackTitleLower := strings.ToLower(strings.TrimSpace(c.normalizeTitle(track.Title)))

		// Calculate similarity with normalized titles
		normalizedTitleSimilarity := c.calculateStringSimilarity(normalizedTitleLower, normalizedTrackTitleLower)
		c.debugLog("   Normalized title similarity: %s ('%s' vs '%s')", formatConfidencePercent(normalizedTitleSimilarity), normalizedTitleLower, normalizedTrackTitleLower)

		// Also try with combined featuring removal + normalization for better matching
		// This handles cases like "Timeless (feat. X) - Remix" vs "Timeless (Remix)"
		featuringNormalizedTitleLower := strings.ToLower(strings.TrimSpace(c.normalizeTitle(c.removeFeaturing(title))))
		featuringNormalizedTrackTitleLower := strings.ToLower(strings.TrimSpace(c.normalizeTitle(c.removeFeaturing(track.Title))))

		// Calculate similarity with combined featuring removal + normalization
		featuringNormalizedTitleSimilarity := c.calculateStringSimilarity(featuringNormalizedTitleLower, featuringNormalizedTrackTitleLower)
		c.debugLog("   Featuring+normalized title similarity: %s ('%s' vs '%s')", formatConfidencePercent(featuringNormalizedTitleSimilarity), featuringNormalizedTitleLower, featuringNormalizedTrackTitleLower)

		// Also try with "with" removed for better matching
		withTitleLower := strings.ToLower(strings.TrimSpace(c.removeWith(title)))
		withTrackTitleLower := strings.ToLower(strings.TrimSpace(c.removeWith(track.Title)))

		// Calculate similarity with "with" removed
		withTitleSimilarity := c.calculateStringSimilarity(withTitleLower, withTrackTitleLower)
		c.debugLog("   'With'-removed title similarity: %s ('%s' vs '%s')", formatConfidencePercent(withTitleSimilarity), withTitleLower, withTrackTitleLower)

		// Also try with common suffixes removed for better matching
		suffixTitleLower := strings.ToLower(strings.TrimSpace(c.RemoveCommonSuffixes(title)))
		suffixTrackTitleLower := strings.ToLower(strings.TrimSpace(c.RemoveCommonSuffixes(track.Title)))

		// Calculate similarity with common suffixes removed
		suffixTitleSimilarity := c.calculateStringSimilarity(suffixTitleLower, suffixTrackTitleLower)
		c.debugLog("   Suffix-removed title similarity: %s ('%s' vs '%s')", formatConfidencePercent(suffixTitleSimilarity), suffixTitleLower, suffixTrackTitleLower)

		// Also try with normalized punctuation for better matching
		punctuationTitleLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(title)))
		punctuationTrackTitleLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(track.Title)))

		// Calculate similarity with normalized punctuation
		punctuationTitleSimilarity := c.calculateStringSimilarity(punctuationTitleLower, punctuationTrackTitleLower)
		c.debugLog("   Punctuation-normalized title similarity: %s ('%s' vs '%s')", formatConfidencePercent(punctuationTitleSimilarity), punctuationTitleLower, punctuationTrackTitleLower)

		// Also try with accent normalization for better matching
		accentTitleLower := strings.ToLower(strings.TrimSpace(c.normalizeAccents(title)))
		accentTrackTitleLower := strings.ToLower(strings.TrimSpace(c.normalizeAccents(track.Title)))

		// Calculate similarity with accent normalization
		accentTitleSimilarity := c.calculateStringSimilarity(accentTitleLower, accentTrackTitleLower)
		c.debugLog("   Accent-normalized title similarity: %s ('%s' vs '%s')", formatConfidencePercent(accentTitleSimilarity), accentTitleLower, accentTrackTitleLower)

		// Use the best of the eight title similarities
		if cleanTitleSimilarity > titleSimilarity {
			c.debugLog("   Using clean title similarity: %s (was %s)", formatConfidencePercent(cleanTitleSimilarity), formatConfidencePercent(titleSimilarity))
			titleSimilarity = cleanTitleSimilarity
		}
		if featuringTitleSimilarity > titleSimilarity {
			c.debugLog("   Using featuring-removed title similarity: %s (was %s)", formatConfidencePercent(featuringTitleSimilarity), formatConfidencePercent(titleSimilarity))
			titleSimilarity = featuringTitleSimilarity
		}
		if normalizedTitleSimilarity > titleSimilarity {
			c.debugLog("   Using normalized title similarity: %s (was %s)", formatConfidencePercent(normalizedTitleSimilarity), formatConfidencePercent(titleSimilarity))
			titleSimilarity = normalizedTitleSimilarity
		}
		if featuringNormalizedTitleSimilarity > titleSimilarity {
			c.debugLog("   Using featuring+normalized title similarity: %s (was %s)", formatConfidencePercent(featuringNormalizedTitleSimilarity), formatConfidencePercent(titleSimilarity))
			titleSimilarity = featuringNormalizedTitleSimilarity
		}
		if withTitleSimilarity > titleSimilarity {
			c.debugLog("   Using 'with'-removed title similarity: %s (was %s)", formatConfidencePercent(withTitleSimilarity), formatConfidencePercent(titleSimilarity))
			titleSimilarity = withTitleSimilarity
		}
		if suffixTitleSimilarity > titleSimilarity {
			c.debugLog("   Using suffix-removed title similarity: %s (was %s)", formatConfidencePercent(suffixTitleSimilarity), formatConfidencePercent(titleSimilarity))
			titleSimilarity = suffixTitleSimilarity
		}
		if punctuationTitleSimilarity > titleSimilarity {
			c.debugLog("   Using punctuation-normalized title similarity: %s (was %s)", formatConfidencePercent(punctuationTitleSimilarity), formatConfidencePercent(titleSimilarity))
			titleSimilarity = punctuationTitleSimilarity
		}
		if accentTitleSimilarity > titleSimilarity {
			c.debugLog("   Using accent-normalized title similarity: %s (was %s)", formatConfidencePercent(accentTitleSimilarity), formatConfidencePercent(titleSimilarity))
			titleSimilarity = accentTitleSimilarity
		}

		albumSimilarity := 0.0
		if useAlbumInScore {
			albumSimilarity = c.bestAlbumSimilarity(sourceAlbum, track.Album)
		}

		var score float64
		if useAlbumInScore {
			score = (titleSimilarity * 0.55) + (artistSimilarity * 0.25) + (albumSimilarity * 0.20)
		} else {
			score = (titleSimilarity * 0.7) + (artistSimilarity * 0.3)
		}

		c.debugLog("   Final title similarity: %s", formatConfidencePercent(titleSimilarity))
		c.debugLog("   Final artist similarity: %s", formatConfidencePercent(artistSimilarity))
		if useAlbumInScore {
			c.debugLog("   Album similarity: %s", formatConfidencePercent(albumSimilarity))
			c.debugLog("   Combined score: %s (0.55·title + 0.25·artist + 0.20·album)", formatConfidencePercent(score))
		} else {
			c.debugLog("   Combined score: %s (%s * 0.7 + %s * 0.3)", formatConfidencePercent(score), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
		}

		// Additional check: if title similarity is very high (>90%), require reasonable artist similarity
		// Special case: be more lenient with "Various Artists" for compilation albums
		if titleSimilarity > 0.9 && artistSimilarity < 0.3 {
			// Check if this is a "Various Artists" compilation album case
			if strings.ToLower(strings.TrimSpace(track.Artist)) == "various artists" {
				c.debugLog("🎵 FindBestMatch: allowing 'Various Artists' compilation match '%s' by '%s' (title: %s > 90%%, artist: %s < 30%% but is Various Artists)",
					track.Title, track.Artist, formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
				// Don't skip this match - it's a valid compilation album case
			} else {
				// Skip this match - title is very similar but artist is too different
				c.debugLog("🚫 FindBestMatch: rejecting '%s' by '%s' (title: %s > 90%%, artist: %s < 30%%)",
					track.Title, track.Artist, formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
				continue
			}
		}

		// Additional check: if title similarity is high (>70%), require minimum artist similarity
		// Special case: be more lenient with "Various Artists" for compilation albums
		if titleSimilarity > 0.7 && artistSimilarity < 0.2 {
			// Check if this is a "Various Artists" compilation album case
			if strings.ToLower(strings.TrimSpace(track.Artist)) == "various artists" {
				c.debugLog("🎵 FindBestMatch: allowing 'Various Artists' compilation match '%s' by '%s' (title: %s > 70%%, artist: %s < 20%% but is Various Artists)",
					track.Title, track.Artist, formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
				// Don't skip this match - it's a valid compilation album case
			} else {
				// Skip this match - title is similar but artist is too different
				c.debugLog("🚫 FindBestMatch: rejecting '%s' by '%s' (title: %s > 70%%, artist: %s < 20%%)",
					track.Title, track.Artist, formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
				continue
			}
		}

		pick := false
		if bestMatch == nil {
			pick = true
		} else if score > bestScore+scoreEqEps {
			pick = true
		} else if math.Abs(score-bestScore) <= scoreEqEps {
			if useAlbumInScore {
				if albumSimilarity > bestAlbumSimilarity+scoreEqEps {
					pick = true
				} else if math.Abs(albumSimilarity-bestAlbumSimilarity) <= scoreEqEps &&
					artistSimilarity > bestArtistSimilarity+scoreEqEps {
					pick = true
				}
			} else if artistSimilarity > bestArtistSimilarity+scoreEqEps {
				pick = true
			}
		}

		if pick {
			prevBest := bestMatch
			oldScore := bestScore
			bestScore = score
			trackCopy := track
			bestMatch = &trackCopy
			bestArtistSimilarity = artistSimilarity
			bestAlbumSimilarity = albumSimilarity
			if prevBest == nil || score > oldScore+scoreEqEps {
				c.debugLog("📈 FindBestMatch: new best match '%s' by '%s' (score: %s > %s, title: %s, artist: %s)",
					track.Title, track.Artist, formatConfidencePercent(score), formatConfidencePercent(oldScore), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
			} else {
				c.debugLog("🎯 FindBestMatch: tie-breaker '%s' by '%s' (score: %s)", track.Title, track.Artist, formatConfidencePercent(score))
			}
		} else {
			c.debugLog("⏭️  FindBestMatch: skipping '%s' by '%s' (score: %s, current best: %s)",
				track.Title, track.Artist, formatConfidencePercent(score), formatConfidencePercent(bestScore))
		}

		// Perfect title+artist: return immediately only when album is not used for disambiguation.
		if !useAlbumInScore && titleSimilarity == 1.0 && artistSimilarity == 1.0 {
			c.debugLog("🎯 FindBestMatch: perfect match found '%s' by '%s'", track.Title, track.Artist)
			trackCopy := track
			return &trackCopy
		}
	}

	// Only return a match if the score is above a threshold
	minScore := c.minMatchScore()
	if bestScore >= minScore {
		slog.Info(fmt.Sprintf("✅ FindBestMatch: FINAL RESULT - returning match '%s' by '%s' (score: %s >= %s) for search '%s' by '%s'",
			bestMatch.Title, bestMatch.Artist, formatConfidencePercent(bestScore), formatConfidencePercent(minScore), title, artist))
		return bestMatch
	}

	c.debugLog("❌ FindBestMatch: FINAL RESULT - no match found (best score: %s < %s) for search '%s' by '%s'", formatConfidencePercent(bestScore), formatConfidencePercent(minScore), title, artist)
	return nil
}

// FindBestMatchWithNormalizedPunctuation finds the best matching track using normalized punctuation.
// When sourceAlbum is non-empty, album similarity is blended into the score (same weights as FindBestMatch).
func (c *Client) FindBestMatchWithNormalizedPunctuation(tracks []PlexTrack, title, artist, sourceAlbum string) *PlexTrack {
	if len(tracks) == 0 {
		return nil
	}

	normalizedTitle := c.normalizePunctuation(title)
	normalizedArtist := c.normalizePunctuation(artist)

	titleLower := strings.ToLower(strings.TrimSpace(normalizedTitle))
	artistLower := strings.ToLower(strings.TrimSpace(normalizedArtist))
	sourceAlbumTrim := strings.TrimSpace(sourceAlbum)
	useAlbumInScore := sourceAlbumTrim != ""

	slog.Info(fmt.Sprintf("🔍 FindBestMatchWithNormalizedPunctuation: searching for '%s' by '%s' among %d tracks", normalizedTitle, normalizedArtist, len(tracks)))

	var exactMatches []PlexTrack
	for _, tr := range tracks {
		normalizedTrackTitle := c.normalizePunctuation(tr.Title)
		normalizedTrackArtist := c.normalizePunctuation(tr.Artist)
		trackTitle := strings.ToLower(strings.TrimSpace(normalizedTrackTitle))
		trackArtist := strings.ToLower(strings.TrimSpace(normalizedTrackArtist))
		if titleLower == trackTitle && artistLower == trackArtist {
			exactMatches = append(exactMatches, tr)
		}
	}
	switch len(exactMatches) {
	case 1:
		t := exactMatches[0]
		slog.Info(fmt.Sprintf("✅ FindBestMatchWithNormalizedPunctuation: single exact match '%s' by '%s'", t.Title, t.Artist))
		return &t
	case 0:
	default:
		if useAlbumInScore {
			var best PlexTrack
			var bestAl float64 = -1
			for _, tr := range exactMatches {
				al := c.bestAlbumSimilarity(sourceAlbum, tr.Album)
				if bestAl < 0 || al > bestAl+scoreEqEps {
					bestAl = al
					best = tr
				}
			}
			t := best
			slog.Info(fmt.Sprintf("✅ FindBestMatchWithNormalizedPunctuation: multiple exact; picked by album (similarity %s)", formatConfidencePercent(bestAl)))
			return &t
		}
	}

	var bestMatch *PlexTrack
	var bestScore float64
	var bestArtistSimilarity float64
	var bestAlbumSimilarity float64 = -1

	for _, track := range tracks {
		normalizedTrackTitle := c.normalizePunctuation(track.Title)
		normalizedTrackArtist := c.normalizePunctuation(track.Artist)

		trackTitle := strings.ToLower(strings.TrimSpace(normalizedTrackTitle))
		trackArtist := strings.ToLower(strings.TrimSpace(normalizedTrackArtist))

		titleSimilarity := c.calculateStringSimilarity(titleLower, trackTitle)
		artistSimilarity := c.calculateStringSimilarity(artistLower, trackArtist)

		albumSimilarity := 0.0
		if useAlbumInScore {
			albumSimilarity = c.bestAlbumSimilarity(sourceAlbum, track.Album)
		}

		var score float64
		if useAlbumInScore {
			score = (titleSimilarity * 0.55) + (artistSimilarity * 0.25) + (albumSimilarity * 0.20)
		} else {
			score = (titleSimilarity * 0.7) + (artistSimilarity * 0.3)
		}

		if titleSimilarity > 0.9 && artistSimilarity < 0.3 {
			slog.Info(fmt.Sprintf("🚫 FindBestMatchWithNormalizedPunctuation: rejecting '%s' by '%s' (title: %s > 90%%, artist: %s < 30%%)",
				track.Title, track.Artist, formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity)))
			continue
		}

		if titleSimilarity > 0.7 && artistSimilarity < 0.2 {
			slog.Info(fmt.Sprintf("🚫 FindBestMatchWithNormalizedPunctuation: rejecting '%s' by '%s' (title: %s > 70%%, artist: %s < 20%%)",
				track.Title, track.Artist, formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity)))
			continue
		}

		pick := false
		if bestMatch == nil {
			pick = true
		} else if score > bestScore+scoreEqEps {
			pick = true
		} else if math.Abs(score-bestScore) <= scoreEqEps {
			if useAlbumInScore {
				if albumSimilarity > bestAlbumSimilarity+scoreEqEps {
					pick = true
				} else if math.Abs(albumSimilarity-bestAlbumSimilarity) <= scoreEqEps &&
					artistSimilarity > bestArtistSimilarity+scoreEqEps {
					pick = true
				}
			} else if artistSimilarity > bestArtistSimilarity+scoreEqEps {
				pick = true
			}
		}

		if pick {
			bestScore = score
			trackCopy := track
			bestMatch = &trackCopy
			bestArtistSimilarity = artistSimilarity
			bestAlbumSimilarity = albumSimilarity
			slog.Info(fmt.Sprintf("📈 FindBestMatchWithNormalizedPunctuation: new best match '%s' by '%s' (score: %s, title: %s, artist: %s)",
				track.Title, track.Artist, formatConfidencePercent(score), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity)))
		}

		if !useAlbumInScore && titleSimilarity == 1.0 && artistSimilarity == 1.0 {
			slog.Info(fmt.Sprintf("🎯 FindBestMatchWithNormalizedPunctuation: perfect match found '%s' by '%s'", track.Title, track.Artist))
			trackCopy := track
			return &trackCopy
		}
	}

	minScore := c.minMatchScore()
	if bestScore >= minScore {
		slog.Info(fmt.Sprintf("✅ FindBestMatchWithNormalizedPunctuation: returning match '%s' by '%s' (score: %s >= %s)",
			bestMatch.Title, bestMatch.Artist, formatConfidencePercent(bestScore), formatConfidencePercent(minScore)))
		return bestMatch
	}

	slog.Info(fmt.Sprintf("❌ FindBestMatchWithNormalizedPunctuation: no match found (best score: %s < %s)", formatConfidencePercent(bestScore), formatConfidencePercent(minScore)))
	return nil
}
