package plex

import (
	"context"
	"fmt"
	"io"
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
// When MusicBrainz artist_credits are present on the track, each distinct credit name is tried after that,
// which often matches Plex display metadata without fetching Plex Artist titleSort.
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
				if ctx.Err() != nil {
					return nil, fmt.Errorf("search cancelled: %w", ctx.Err())
				}
				if isTransientPlexErr(err) {
					slog.WarnContext(ctx, "plex search step failed; trying next strategy",
						"err", err, "strategy", strategy.name, "phase", phase.tierLabel(),
						"title", song.Name, "artist", artist)
					continue
				}
				return nil, err
			}
			if tr != nil {
				slog.Debug(fmt.Sprintf("✅ SearchTrack: found match '%s' by '%s' using %s [%s tier]", tr.Title, tr.DisplayArtist(), strategy.name, phase.tierLabel()))
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
			if ctx.Err() != nil {
				return nil, fmt.Errorf("search cancelled: %w", ctx.Err())
			}
			if isTransientPlexErr(err) {
				slog.WarnContext(ctx, "full library Plex scan failed; treating as no match for this track",
					"err", err, "title", song.Name, "artist", artist)
				tr, err = nil, nil
			} else {
				return nil, err
			}
		}
		if tr != nil {
			slog.Debug(fmt.Sprintf("✅ SearchTrack: found match '%s' by '%s' using full library search", tr.Title, tr.DisplayArtist()))
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
		// Indexed Plex /search is literal: en dash vs hyphen in the query string changes hits. Re-run
		// with the same normalizations as FindBestMatch (e.g. U+2013 → -) so "9–5" finds "9-5".
		{"punctuation normalized", func(ctx context.Context, phase searchPhase, title, artist, sourceAlbum string) (*PlexTrack, error) {
			pt := c.normalizePunctuation(title)
			pa := c.normalizePunctuation(artist)
			if pt == title && pa == artist {
				return nil, nil
			}
			return c.trySearchVariationsPhase(ctx, pt, pa, sourceAlbum, phase)
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

	resp, err := c.httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make search request: %w", err)
	}

	if resp.StatusCode != StatusOK {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newPlexHTTPError(resp.StatusCode, "search by title", b)
	}

	var searchResp PlexResponse
	if err := decodePlexResponseXML(resp, &searchResp); err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("failed to decode search response: %w", err)
	}
	_ = resp.Body.Close()

	// Find best match among search results
	slog.Debug(fmt.Sprintf("🔍 searchByTitle: searching for '%s' by '%s', found %d results", title, artist, len(searchResp.Tracks)))
	if len(searchResp.Tracks) > 0 && c.debug {
		for i, track := range searchResp.Tracks {
			c.debugLog("  Result %d: '%s' by '%s' (ID: %s)", i+1, track.Title, track.DisplayArtist(), track.ID)
		}
	}
	result := c.findBestMatchWithOptionalArtistSortRetry(ctx, searchResp.Tracks, title, artist, sourceAlbum, false)
	if result != nil {
		slog.Debug(fmt.Sprintf("✅ searchByTitle: found match '%s' by '%s'", result.Title, result.DisplayArtist()))
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

	resp, err := c.httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make artist search request: %w", err)
	}

	if resp.StatusCode != StatusOK {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newPlexHTTPError(resp.StatusCode, "search by artist", b)
	}

	var searchResp PlexResponse
	if err := decodePlexResponseXML(resp, &searchResp); err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("failed to decode artist search response: %w", err)
	}
	_ = resp.Body.Close()

	// Find best match among search results
	result := c.findBestMatchWithOptionalArtistSortRetry(ctx, searchResp.Tracks, title, artist, sourceAlbum, false)
	if result != nil {
		slog.Debug(fmt.Sprintf("✅ searchByArtist: found match '%s' by '%s'", result.Title, result.DisplayArtist()))
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

	resp, err := c.httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make combined search request: %w", err)
	}

	if resp.StatusCode != StatusOK {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newPlexHTTPError(resp.StatusCode, "combined search", b)
	}

	var searchResp PlexResponse
	if err := decodePlexResponseXML(resp, &searchResp); err != nil {
		_ = resp.Body.Close()
		slog.WarnContext(ctx, "combined Plex search returned OK but XML decode failed; trying other strategies",
			"err", err, "query", query)
		return nil, nil
	}
	_ = resp.Body.Close()
	if track := c.findBestMatchWithOptionalArtistSortRetry(ctx, searchResp.Tracks, title, artist, sourceAlbum, false); track != nil {
		slog.Debug(fmt.Sprintf("✅ searchByCombinedQuery: found match '%s' by '%s'", track.Title, track.DisplayArtist()))
		return track, nil
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

	libCtx, cancel := context.WithTimeout(ctx, FullLibraryHTTPTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(libCtx, "GET", reqURL+"?"+params.Encode(), nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create library request: %w", err)
	}

	req.Header.Set("Accept", "application/xml")

	resp, err := c.httpDo(req)
	if err != nil {
		return nil, fmt.Errorf("failed to make library request: %w", err)
	}

	if resp.StatusCode != StatusOK {
		b, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		return nil, newPlexHTTPError(resp.StatusCode, "library all", b)
	}

	var libraryResp PlexResponse
	if err := decodePlexResponseXML(resp, &libraryResp); err != nil {
		_ = resp.Body.Close()
		return nil, fmt.Errorf("failed to decode library response: %w", err)
	}
	_ = resp.Body.Close()

	// Find best match among all tracks
	c.debugLog("🔍 searchEntireLibrary: searching for '%s' by '%s' in entire library (%d tracks)", title, artist, len(libraryResp.Tracks))
	result := c.findBestMatchWithOptionalArtistSortRetry(ctx, libraryResp.Tracks, title, artist, sourceAlbum, true)
	if result != nil {
		slog.Debug(fmt.Sprintf("✅ searchEntireLibrary: found match '%s' by '%s' for search '%s' by '%s'", result.Title, result.DisplayArtist(), title, artist))
	} else {
		slog.Debug(fmt.Sprintf("❌ searchEntireLibrary: no match found for search '%s' by '%s'", title, artist))
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

// artistMatchSimilarity returns the best 0–1 match between a source artist string and one Plex artist field
// (album or track), using the same normalizations as FindBestMatch.
func (c *Client) artistMatchSimilarity(artist, plexArtist string) float64 {
	artistLower := strings.ToLower(strings.TrimSpace(artist))
	trackArtist := strings.ToLower(strings.TrimSpace(plexArtist))
	if trackArtist == "" {
		return 0
	}
	artistSimilarity := c.calculateStringSimilarity(artistLower, trackArtist)

	punctuationArtistLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(artist)))
	punctuationTrackArtistLower := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(plexArtist)))
	if v := c.calculateStringSimilarity(punctuationArtistLower, punctuationTrackArtistLower); v > artistSimilarity {
		artistSimilarity = v
	}
	accentArtistLower := strings.ToLower(strings.TrimSpace(c.normalizeAccents(artist)))
	accentTrackArtistLower := strings.ToLower(strings.TrimSpace(c.normalizeAccents(plexArtist)))
	if v := c.calculateStringSimilarity(accentArtistLower, accentTrackArtistLower); v > artistSimilarity {
		artistSimilarity = v
	}
	featuringArtistLower := strings.ToLower(strings.TrimSpace(c.removeFeaturing(artist)))
	featuringTrackArtistLower := strings.ToLower(strings.TrimSpace(c.removeFeaturing(plexArtist)))
	if v := c.calculateStringSimilarity(featuringArtistLower, featuringTrackArtistLower); v > artistSimilarity {
		artistSimilarity = v
	}
	return artistSimilarity
}

// bestPlexTrackArtistSimilarity is the max artist similarity over grandparent (album), originalTitle (track),
// and GrandparentTitleSort (Plex Artist titleSort) when set.
func (c *Client) bestPlexTrackArtistSimilarity(artist string, tr PlexTrack) float64 {
	best := c.artistMatchSimilarity(artist, tr.Artist)
	if s := strings.TrimSpace(tr.OriginalTitle); s != "" {
		if v := c.artistMatchSimilarity(artist, s); v > best {
			best = v
		}
	}
	if s := strings.TrimSpace(tr.GrandparentTitleSort); s != "" {
		if v := c.artistMatchSimilarity(artist, s); v > best {
			best = v
		}
	}
	return best
}

// bestPlexTrackArtistSimilarityNormPunct matches FindBestMatchWithNormalizedPunctuation (normalized punctuation on both sides).
func (c *Client) bestPlexTrackArtistSimilarityNormPunct(artist string, tr PlexTrack) float64 {
	an := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(artist)))
	sim := c.calculateStringSimilarity(an, strings.ToLower(strings.TrimSpace(c.normalizePunctuation(tr.Artist))))
	if s := strings.TrimSpace(tr.OriginalTitle); s != "" {
		if v := c.calculateStringSimilarity(an, strings.ToLower(strings.TrimSpace(c.normalizePunctuation(s)))); v > sim {
			sim = v
		}
	}
	if s := strings.TrimSpace(tr.GrandparentTitleSort); s != "" {
		if v := c.calculateStringSimilarity(an, strings.ToLower(strings.TrimSpace(c.normalizePunctuation(s)))); v > sim {
			sim = v
		}
	}
	return sim
}

// plexTrackNormPunctTitleArtistMatch returns true if normalized-punctuation title and artist match this track
// (album artist and/or per-track originalTitle on the artist side).
func (c *Client) plexTrackNormPunctTitleArtistMatch(tr PlexTrack, titleLower, artistLower string) bool {
	nt := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(tr.Title)))
	if titleLower != nt {
		return false
	}
	na := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(tr.Artist)))
	if artistLower == na {
		return true
	}
	if s := strings.TrimSpace(tr.OriginalTitle); s != "" {
		no := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(s)))
		if artistLower == no {
			return true
		}
	}
	if s := strings.TrimSpace(tr.GrandparentTitleSort); s != "" {
		ns := strings.ToLower(strings.TrimSpace(c.normalizePunctuation(s)))
		if artistLower == ns {
			return true
		}
	}
	return false
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
		if tr.exactTitleAndArtistMatch(titleLower, artistLower) {
			exactMatches = append(exactMatches, tr)
		}
	}
	switch len(exactMatches) {
	case 1:
		t := exactMatches[0]
		c.debugLog("✅ FindBestMatch: single exact match '%s' by '%s'", t.Title, t.DisplayArtist())
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

		// Calculate similarity scores with original titles
		titleSimilarity := c.calculateStringSimilarity(titleLower, trackTitle)
		artistSimilarity := c.bestPlexTrackArtistSimilarity(artist, track)

		c.debugLog("🔍 FindBestMatch: '%s' by '%s' -> '%s' by '%s'", title, artist, track.Title, track.DisplayArtist())
		c.debugLog("   Original title similarity: %s ('%s' vs '%s')", formatConfidencePercent(titleSimilarity), titleLower, trackTitle)
		c.debugLog("   Best artist similarity (album + track fields): %s", formatConfidencePercent(artistSimilarity))

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
					track.Title, track.DisplayArtist(), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
				// Don't skip this match - it's a valid compilation album case
			} else {
				// Skip this match - title is very similar but artist is too different
				c.debugLog("🚫 FindBestMatch: rejecting '%s' by '%s' (title: %s > 90%%, artist: %s < 30%%)",
					track.Title, track.DisplayArtist(), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
				continue
			}
		}

		// Additional check: if title similarity is high (>70%), require minimum artist similarity
		// Special case: be more lenient with "Various Artists" for compilation albums
		if titleSimilarity > 0.7 && artistSimilarity < 0.2 {
			// Check if this is a "Various Artists" compilation album case
			if strings.ToLower(strings.TrimSpace(track.Artist)) == "various artists" {
				c.debugLog("🎵 FindBestMatch: allowing 'Various Artists' compilation match '%s' by '%s' (title: %s > 70%%, artist: %s < 20%% but is Various Artists)",
					track.Title, track.DisplayArtist(), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
				// Don't skip this match - it's a valid compilation album case
			} else {
				// Skip this match - title is similar but artist is too different
				c.debugLog("🚫 FindBestMatch: rejecting '%s' by '%s' (title: %s > 70%%, artist: %s < 20%%)",
					track.Title, track.DisplayArtist(), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
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
					track.Title, track.DisplayArtist(), formatConfidencePercent(score), formatConfidencePercent(oldScore), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity))
			} else {
				c.debugLog("🎯 FindBestMatch: tie-breaker '%s' by '%s' (score: %s)", track.Title, track.DisplayArtist(), formatConfidencePercent(score))
			}
		} else {
			c.debugLog("⏭️  FindBestMatch: skipping '%s' by '%s' (score: %s, current best: %s)",
				track.Title, track.DisplayArtist(), formatConfidencePercent(score), formatConfidencePercent(bestScore))
		}

		// Perfect title+artist: return immediately only when album is not used for disambiguation.
		if !useAlbumInScore && titleSimilarity == 1.0 && artistSimilarity == 1.0 {
			c.debugLog("🎯 FindBestMatch: perfect match found '%s' by '%s'", track.Title, track.DisplayArtist())
			trackCopy := track
			return &trackCopy
		}
	}

	// Only return a match if the score is above a threshold
	minScore := c.minMatchScore()
	if bestScore >= minScore {
		slog.Debug(fmt.Sprintf("✅ FindBestMatch: FINAL RESULT - returning match '%s' by '%s' (score: %s >= %s) for search '%s' by '%s'",
			bestMatch.Title, bestMatch.DisplayArtist(), formatConfidencePercent(bestScore), formatConfidencePercent(minScore), title, artist))
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

	slog.Debug(fmt.Sprintf("🔍 FindBestMatchWithNormalizedPunctuation: searching for '%s' by '%s' among %d tracks", normalizedTitle, normalizedArtist, len(tracks)))

	var exactMatches []PlexTrack
	for _, tr := range tracks {
		if c.plexTrackNormPunctTitleArtistMatch(tr, titleLower, artistLower) {
			exactMatches = append(exactMatches, tr)
		}
	}
	switch len(exactMatches) {
	case 1:
		t := exactMatches[0]
		slog.Debug(fmt.Sprintf("✅ FindBestMatchWithNormalizedPunctuation: single exact match '%s' by '%s'", t.Title, t.DisplayArtist()))
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
			slog.Debug(fmt.Sprintf("✅ FindBestMatchWithNormalizedPunctuation: multiple exact; picked by album (similarity %s)", formatConfidencePercent(bestAl)))
			return &t
		}
	}

	var bestMatch *PlexTrack
	var bestScore float64
	var bestArtistSimilarity float64
	var bestAlbumSimilarity float64 = -1

	for _, track := range tracks {
		normalizedTrackTitle := c.normalizePunctuation(track.Title)
		trackTitle := strings.ToLower(strings.TrimSpace(normalizedTrackTitle))

		titleSimilarity := c.calculateStringSimilarity(titleLower, trackTitle)
		artistSimilarity := c.bestPlexTrackArtistSimilarityNormPunct(artist, track)

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
			if strings.ToLower(strings.TrimSpace(track.Artist)) == "various artists" {
				// Match FindBestMatch: allow VA compilations
			} else {
				slog.Debug(fmt.Sprintf("🚫 FindBestMatchWithNormalizedPunctuation: rejecting '%s' by '%s' (title: %s > 90%%, artist: %s < 30%%)",
					track.Title, track.DisplayArtist(), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity)))
				continue
			}
		}

		if titleSimilarity > 0.7 && artistSimilarity < 0.2 {
			if strings.ToLower(strings.TrimSpace(track.Artist)) == "various artists" {
				// allow VA
			} else {
				slog.Debug(fmt.Sprintf("🚫 FindBestMatchWithNormalizedPunctuation: rejecting '%s' by '%s' (title: %s > 70%%, artist: %s < 20%%)",
					track.Title, track.DisplayArtist(), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity)))
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
			bestScore = score
			trackCopy := track
			bestMatch = &trackCopy
			bestArtistSimilarity = artistSimilarity
			bestAlbumSimilarity = albumSimilarity
			slog.Debug(fmt.Sprintf("📈 FindBestMatchWithNormalizedPunctuation: new best match '%s' by '%s' (score: %s, title: %s, artist: %s)",
				track.Title, track.DisplayArtist(), formatConfidencePercent(score), formatConfidencePercent(titleSimilarity), formatConfidencePercent(artistSimilarity)))
		}

		if !useAlbumInScore && titleSimilarity == 1.0 && artistSimilarity == 1.0 {
			slog.Debug(fmt.Sprintf("🎯 FindBestMatchWithNormalizedPunctuation: perfect match found '%s' by '%s'", track.Title, track.DisplayArtist()))
			trackCopy := track
			return &trackCopy
		}
	}

	minScore := c.minMatchScore()
	if bestScore >= minScore {
		slog.Debug(fmt.Sprintf("✅ FindBestMatchWithNormalizedPunctuation: returning match '%s' by '%s' (score: %s >= %s)",
			bestMatch.Title, bestMatch.DisplayArtist(), formatConfidencePercent(bestScore), formatConfidencePercent(minScore)))
		return bestMatch
	}

	slog.Debug(fmt.Sprintf("❌ FindBestMatchWithNormalizedPunctuation: no match found (best score: %s < %s)", formatConfidencePercent(bestScore), formatConfidencePercent(minScore)))
	return nil
}
