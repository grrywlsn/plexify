package plex

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/track"
)

func TestPlexTrack_DisplayArtist(t *testing.T) {
	t.Parallel()
	va := PlexTrack{Artist: "Various Artists", OriginalTitle: "Whethan"}
	if g, w := va.DisplayArtist(), "Whethan"; g != w {
		t.Fatalf("DisplayArtist: got %q, want %q", g, w)
	}
	if p := (PlexTrack{Artist: "ABBA"}); p.DisplayArtist() != "ABBA" {
		t.Fatal("expected grandparent when OriginalTitle empty")
	}
}

func TestPlexTrack_exactTitleAndArtistMatch(t *testing.T) {
	t.Parallel()
	album := "Fifty Shades Freed"
	tr := PlexTrack{Title: "High", Artist: "Various Artists", OriginalTitle: "Whethan", Album: album}
	if !tr.exactTitleAndArtistMatch("high", "whethan") {
		t.Fatal("expected match against originalTitle")
	}
	if tr.exactTitleAndArtistMatch("other", "whethan") {
		t.Fatal("title must not match")
	}
}

func TestFindBestMatch_variousArtistsOriginalTitle(t *testing.T) {
	t.Parallel()
	c := &Client{}
	album := "Fifty Shades Freed: The Final Chapter: Original Motion Picture Soundtrack"
	tracks := []PlexTrack{{
		ID:            "1",
		Title:         "High",
		Artist:        "Various Artists",
		OriginalTitle: "Whethan",
		Album:         album,
	}}
	got := c.FindBestMatch(tracks, "High", "Whethan", album)
	if got == nil || got.ID != "1" {
		t.Fatalf("expected match, got %v", got)
	}
}

func TestFindBestMatch_variousArtistsTwoCandidatesOriginalTitlePicks(t *testing.T) {
	t.Parallel()
	c := &Client{}
	album := "Fifty Shades Freed: The Final Chapter: Original Motion Picture Soundtrack"
	tracks := []PlexTrack{
		{ID: "a", Title: "High", Artist: "Various Artists", OriginalTitle: "Wrong Artist", Album: album},
		{ID: "b", Title: "High", Artist: "Various Artists", OriginalTitle: "Whethan", Album: album},
	}
	got := c.FindBestMatch(tracks, "High", "Whethan", album)
	if got == nil || got.ID != "b" {
		t.Fatalf("expected id b, got %v", got)
	}
}

func TestFindBestMatch_variousArtistsOriginalTitleCaseInsensitive(t *testing.T) {
	t.Parallel()
	c := &Client{}
	album := "Fifty Shades Freed: The Final Chapter: Original Motion Picture Soundtrack"
	tracks := []PlexTrack{{
		ID: "1", Title: "High", Artist: "Various Artists", OriginalTitle: "Whethan", Album: album,
	}}
	if c.FindBestMatch(tracks, "High", "Whethan", album) == nil {
		t.Fatal("expected match with canonical case")
	}
}

func TestFindBestMatchWithNormalizedPunctuation_variousArtistsOriginalTitle(t *testing.T) {
	t.Parallel()
	c := &Client{}
	album := "Fifty Shades Freed: The Final Chapter: Original Motion Picture Soundtrack"
	tracks := []PlexTrack{{
		ID: "1", Title: "High", Artist: "Various Artists", OriginalTitle: "Whethan", Album: album,
	}}
	if got := c.FindBestMatchWithNormalizedPunctuation(tracks, "High", "Whethan", album); got == nil {
		t.Fatal("expected match")
	}
}

// Source catalogs (e.g. music-social / MusicBrainz) often use U+2013 EN DASH in titles; Plex
// and local files often store the ASCII hyphen-minus. Both matchers must treat them as equivalent.
func TestFindBestMatch_enDashInTitleMatchesPlexHyphen(t *testing.T) {
	t.Parallel()
	c := &Client{}
	srcTitle := "9\u20135" // 9–5
	plexTracks := []PlexTrack{{
		ID: "1", Title: "9-5", Artist: "Biig Piig", Album: "11:11",
	}}
	if got := c.FindBestMatch(plexTracks, srcTitle, "Biig Piig", "11:11"); got == nil || got.ID != "1" {
		t.Fatalf("FindBestMatch: expected en-dash source title to match Plex hyphen, got %v", got)
	}
	if got := c.FindBestMatchWithNormalizedPunctuation(plexTracks, srcTitle, "Biig Piig", "11:11"); got == nil || got.ID != "1" {
		t.Fatalf("FindBestMatchWithNormalizedPunctuation: expected match, got %v", got)
	}
}

func TestCalculateConfidence_variousArtistsOriginalTitle(t *testing.T) {
	t.Parallel()
	c := &Client{}
	album := "Fifty Shades Freed: The Final Chapter: Original Motion Picture Soundtrack"
	song := track.Track{Name: "High", Artist: "Whethan", Album: album}
	plexTr := &PlexTrack{Title: "High", Artist: "Various Artists", OriginalTitle: "Whethan", Album: album}
	conf := c.calculateConfidence(song, plexTr, MatchTypeTitleArtist)
	if min := c.minMatchScore(); conf < min {
		t.Fatalf("expected confidence >= %g, got %g", min, conf)
	}
}

func TestIndexedTrackSearchStrategies_order(t *testing.T) {
	c := &Client{}
	strategies := c.indexedTrackSearchStrategies()
	if len(strategies) < 2 {
		t.Fatalf("expected multiple strategies, got %d", len(strategies))
	}
	if strategies[0].name != "exact title/artist" {
		t.Fatalf("first strategy should be exact title/artist, got %q", strategies[0].name)
	}
	if strategies[1].name != "punctuation normalized" {
		t.Fatalf("second strategy should be punctuation normalized, got %q", strategies[1].name)
	}
}

// When the source title uses a Unicode en dash, Plex /search may return no rows for that query, even
// though the file is stored as "9-5". A second strategy must repeat the search with
// normalizePunctuation (ASCII hyphen) in the request.
func TestSearchTrack_punctuationNormalizedQueryFindsPlexHyphenTitle(t *testing.T) {
	t.Parallel()
	const sectionID = 2
	srcTitle := "9\u20135" // 9–5
	srcArtist := "Biig Piig"
	srcAlbum := "11:11"

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != fmt.Sprintf("/library/sections/%d/search", sectionID) {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/xml")
		q := r.URL.Query().Get("query")
		if strings.Contains(q, "\u2013") {
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
			return
		}
		if q == "Biig Piig" {
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
			return
		}
		if q == "9-5" || q == "9-5 Biig Piig" {
			_, _ = fmt.Fprintf(w, `<?xml version="1.0"?><MediaContainer size="1">`+
				`<Track ratingKey="1" title="9-5" grandparentTitle="Biig Piig" parentTitle="11:11"/>`+
				`</MediaContainer>`)
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
	}))
	defer ts.Close()

	cfg := &config.Config{
		Plex: config.PlexConfig{
			URL:                    ts.URL,
			Token:                  "tok",
			LibrarySectionID:       sectionID,
			MatchConfidencePercent: config.DefaultMatchConfidencePercent,
		},
	}
	c := NewClient(cfg)
	// Reproduce: indexed search only; en-dash must not force relying on a generous artist result set.
	c.SetSkipFullLibrarySearch(true)

	got, kind, err := c.SearchTrack(context.Background(), track.Track{Name: srcTitle, Artist: srcArtist, Album: srcAlbum})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "1" {
		t.Fatalf("expected match via punctuation-normalized search query, got %v (kind %s)", got, kind)
	}
	if kind != MatchTypeTitleArtist {
		t.Errorf("expected MatchTypeTitleArtist, got %s", kind)
	}
}

func TestSearchTrack_artistAliasFallback(t *testing.T) {
	t.Parallel()

	const sectionID = 2
	var queries []string
	var queryMu sync.Mutex
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		query := r.URL.Query().Get("query")
		queryMu.Lock()
		queries = append(queries, query)
		queryMu.Unlock()
		if strings.Contains(query, "Nancy Ajram") {
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Track ratingKey="nancy" title="Shhadi Ya Deni" grandparentTitle="Nancy Ajram" parentTitle="Shhadi Ya Deni"/>` +
				`</MediaContainer>`))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
	}))
	defer ts.Close()

	cfg := &config.Config{Plex: config.PlexConfig{
		URL:                    ts.URL,
		Token:                  "tok",
		LibrarySectionID:       sectionID,
		MatchConfidencePercent: config.DefaultMatchConfidencePercent,
	}}
	c := NewClient(cfg)
	c.SetSkipFullLibrarySearch(true)
	song := track.Track{
		Name:                     "Shhadi Ya Deni",
		Artist:                   "نانسي عجرم",
		Album:                    "Shhadi Ya Deni",
		MusicBrainzArtistAliases: []string{"Nancy Ajram"},
	}

	got, kind, err := c.SearchTrack(context.Background(), song)
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "nancy" {
		t.Fatalf("expected Nancy Ajram alias match, got %+v", got)
	}
	if kind != MatchTypeTitleArtistAlias {
		t.Fatalf("kind = %s, want %s", kind, MatchTypeTitleArtistAlias)
	}
	if confidence := c.calculateConfidence(song, got, kind); math.Abs(confidence-aliasConfidenceFactor) > scoreEqEps {
		t.Fatalf("confidence = %f, want %f", confidence, aliasConfidenceFactor)
	}

	queryMu.Lock()
	defer queryMu.Unlock()
	var firstCanonical, firstAlias = -1, -1
	for i, query := range queries {
		if firstCanonical < 0 && strings.Contains(query, "نانسي عجرم") {
			firstCanonical = i
		}
		if firstAlias < 0 && strings.Contains(query, "Nancy Ajram") {
			firstAlias = i
		}
	}
	if firstCanonical < 0 || firstAlias < 0 || firstCanonical >= firstAlias {
		t.Fatalf("canonical artist must be searched before alias; queries: %q", queries)
	}
}

func TestSearchTrack_artistAliasDiscountMustMeetThreshold(t *testing.T) {
	t.Parallel()

	const sectionID = 2
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if strings.Contains(r.URL.Query().Get("query"), "Nancy Ajram") {
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Track ratingKey="nancy" title="Shhadi Ya Deni" grandparentTitle="Nancy Ajram" parentTitle="Shhadi Ya Deni"/>` +
				`</MediaContainer>`))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
	}))
	defer ts.Close()

	cfg := &config.Config{Plex: config.PlexConfig{
		URL:                    ts.URL,
		Token:                  "tok",
		LibrarySectionID:       sectionID,
		MatchConfidencePercent: 96,
	}}
	c := NewClient(cfg)
	c.SetSkipFullLibrarySearch(true)
	got, kind, err := c.SearchTrack(context.Background(), track.Track{
		Name:                     "Shhadi Ya Deni",
		Artist:                   "نانسي عجرم",
		Album:                    "Shhadi Ya Deni",
		MusicBrainzArtistAliases: []string{"Nancy Ajram"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil || kind != MatchTypeNone {
		t.Fatalf("95%% alias match must not pass 96%% threshold: got %+v (%s)", got, kind)
	}
}

func TestSearchTrack_primaryArtistWinsBeforeAlias(t *testing.T) {
	t.Parallel()

	const sectionID = 2
	var aliasQueried atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		query := r.URL.Query().Get("query")
		if strings.Contains(query, "نانسي عجرم") {
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Track ratingKey="canonical" title="Shhadi Ya Deni" grandparentTitle="نانسي عجرم" parentTitle="Shhadi Ya Deni"/>` +
				`</MediaContainer>`))
			return
		}
		if strings.Contains(query, "Nancy Ajram") {
			aliasQueried.Store(true)
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
	}))
	defer ts.Close()

	cfg := &config.Config{Plex: config.PlexConfig{
		URL:                    ts.URL,
		Token:                  "tok",
		LibrarySectionID:       sectionID,
		MatchConfidencePercent: config.DefaultMatchConfidencePercent,
	}}
	c := NewClient(cfg)
	c.SetSkipFullLibrarySearch(true)
	got, kind, err := c.SearchTrack(context.Background(), track.Track{
		Name:                     "Shhadi Ya Deni",
		Artist:                   "نانسي عجرم",
		Album:                    "Shhadi Ya Deni",
		MusicBrainzArtistAliases: []string{"Nancy Ajram"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || got.ID != "canonical" || kind != MatchTypeTitleArtist {
		t.Fatalf("expected canonical match, got %+v (%s)", got, kind)
	}
	if aliasQueried.Load() {
		t.Fatal("alias should not be queried after a canonical match")
	}
}

func TestSearchByTitle_DoesNotDeadlockOnArtistSortRetry(t *testing.T) {
	t.Parallel()

	const sectionID = 2
	var metadataGets int32

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.URL.Path == fmt.Sprintf("/library/sections/%d/search", sectionID):
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Track ratingKey="1" title="Completely Different" grandparentTitle="Nope" grandparentRatingKey="900" parentTitle="X"/>` +
				`</MediaContainer>`))
			return
		case strings.HasPrefix(r.URL.Path, "/library/metadata/"):
			atomic.AddInt32(&metadataGets, 1)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Artist ratingKey="900" title="Nope" titleSort="Still Nope"/>` +
				`</MediaContainer>`))
			return
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}))
	defer ts.Close()

	cfg := &config.Config{
		Plex: config.PlexConfig{
			URL:                    ts.URL,
			Token:                  "tok",
			LibrarySectionID:       sectionID,
			MatchConfidencePercent: 99,
		},
	}
	c := NewClient(cfg)
	done := make(chan struct{})
	var (
		got *PlexTrack
		err error
	)
	go func() {
		got, err = c.searchByTitle(context.Background(), "Wanted Song", "Wanted Artist", "Wanted Album")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("searchByTitle deadlocked while fetching artist metadata")
	}

	if err != nil {
		t.Fatalf("searchByTitle returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no match, got %+v", got)
	}
	if atomic.LoadInt32(&metadataGets) == 0 {
		t.Fatal("expected artist metadata retry request")
	}
}

func TestSearchByCombinedQuery_DoesNotDeadlockOnArtistSortRetry(t *testing.T) {
	t.Parallel()

	const sectionID = 2
	var (
		searchGets   int32
		metadataGets int32
	)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		switch {
		case r.URL.Path == fmt.Sprintf("/library/sections/%d/search", sectionID):
			atomic.AddInt32(&searchGets, 1)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Track ratingKey="2" title="Wrong Track" grandparentTitle="Nope" grandparentRatingKey="901" parentTitle="Y"/>` +
				`</MediaContainer>`))
			return
		case strings.HasPrefix(r.URL.Path, "/library/metadata/"):
			atomic.AddInt32(&metadataGets, 1)
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Artist ratingKey="901" title="Nope" titleSort="Still Nope"/>` +
				`</MediaContainer>`))
			return
		default:
			w.WriteHeader(http.StatusNotFound)
			return
		}
	}))
	defer ts.Close()

	cfg := &config.Config{
		Plex: config.PlexConfig{
			URL:                    ts.URL,
			Token:                  "tok",
			LibrarySectionID:       sectionID,
			MatchConfidencePercent: 99,
		},
	}
	c := NewClient(cfg)

	done := make(chan struct{})
	var (
		got *PlexTrack
		err error
	)
	go func() {
		got, err = c.searchByCombinedQuery(context.Background(), "Wanted Song", "Wanted Artist", "Wanted Album")
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("searchByCombinedQuery deadlocked while fetching artist metadata")
	}

	if err != nil {
		t.Fatalf("searchByCombinedQuery returned error: %v", err)
	}
	if got != nil {
		t.Fatalf("expected no match, got %+v", got)
	}
	if atomic.LoadInt32(&searchGets) == 0 {
		t.Fatal("expected combined search request")
	}
	if atomic.LoadInt32(&metadataGets) == 0 {
		t.Fatal("expected artist metadata retry request")
	}
}
