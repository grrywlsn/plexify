package plex

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/track"
)

// manyAliases is the shape MusicBrainz artist_alias actually returns: locale and script
// variants, legal names, search hints and misspellings, with no per-artist cap.
var manyAliases = []string{
	"Nancy Ajram", "Nancy Agram", "Nancy Ahjram", "Nansi Ajram", "Nancy Ajrams",
	"ナンシー・アジュラム", "낸시 아즈람", "Нэнси Аджрам", "Nancy A.", "Nansy Ajram",
	"Nancy Nabil Ajram", "N. Ajram", "nancyajram", "Ajram Nancy", "Nancy Ajram Live",
}

func aliasCountingServer(t *testing.T, allScans, searches *int64) *httptest.Server {
	t.Helper()
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/all") {
			atomic.AddInt64(allScans, 1)
		} else {
			atomic.AddInt64(searches, 1)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
	}))
	t.Cleanup(ts.Close)
	return ts
}

// An unmatched track must download the library at most once, however many aliases it carries.
// Previously every artist candidate ran its own full-library scan.
func TestSearchTrack_fullLibraryScannedOncePerTrack(t *testing.T) {
	t.Parallel()

	var allScans, searches int64
	ts := aliasCountingServer(t, &allScans, &searches)

	cfg := &config.Config{Plex: config.PlexConfig{
		URL:                    ts.URL,
		Token:                  "tok",
		LibrarySectionID:       2,
		MatchConfidencePercent: config.DefaultMatchConfidencePercent,
	}}
	c := NewClient(cfg)

	_, kind, err := c.SearchTrack(context.Background(), track.Track{
		Name:                     "Shhadi Ya Deni",
		Artist:                   "نانسي عجرم",
		Album:                    "Shhadi Ya Deni",
		MusicBrainzArtistCredits: []string{"نانسي عجرم"},
		MusicBrainzArtistAliases: manyAliases,
	})
	if err != nil {
		t.Fatal(err)
	}
	if kind != MatchTypeNone {
		t.Fatalf("kind = %s, want %s", kind, MatchTypeNone)
	}
	if got := atomic.LoadInt64(&allScans); got != 1 {
		t.Fatalf("full-library scans = %d, want exactly 1 (aliases must not each trigger one)", got)
	}
	t.Logf("indexed searches=%d full-library scans=%d", atomic.LoadInt64(&searches), atomic.LoadInt64(&allScans))
}

// Aliases are a last resort and must not reach the full-library scan, which is where a loosely
// related alias can match an unrelated artist.
func TestSearchTrack_aliasesDoNotTriggerFullLibraryScan(t *testing.T) {
	t.Parallel()

	var allScans, searches int64
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if strings.HasSuffix(r.URL.Path, "/all") {
			atomic.AddInt64(&allScans, 1)
			// A library scan under the alias "Nancy" would otherwise match Nancy Wilson.
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Track ratingKey="wrong" title="Alone" grandparentTitle="Nancy Wilson" parentTitle="Alone"/>` +
				`</MediaContainer>`))
			return
		}
		atomic.AddInt64(&searches, 1)
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
	}))
	defer ts.Close()

	cfg := &config.Config{Plex: config.PlexConfig{
		URL:                    ts.URL,
		Token:                  "tok",
		LibrarySectionID:       2,
		MatchConfidencePercent: config.DefaultMatchConfidencePercent,
	}}
	c := NewClient(cfg)

	got, kind, err := c.SearchTrack(context.Background(), track.Track{
		Name:                     "Alone",
		Artist:                   "نانسي عجرم",
		Album:                    "Alone",
		MusicBrainzArtistAliases: []string{"Nancy"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("alias must not match unrelated artist %q via full-library scan (kind=%s)", got.DisplayArtist(), kind)
	}
	if got := atomic.LoadInt64(&allScans); got != 1 {
		t.Fatalf("full-library scans = %d, want exactly 1 (canonical tier only)", got)
	}
}

// The canonical tier must be fully exhausted, including the full-library scan, before any alias
// is queried.
func TestSearchTrack_canonicalFullLibraryBeatsAlias(t *testing.T) {
	t.Parallel()

	var aliasQueried atomic.Bool
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if strings.Contains(r.URL.Query().Get("query"), "Nancy Ajram") {
			aliasQueried.Store(true)
		}
		if strings.HasSuffix(r.URL.Path, "/all") {
			_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="1">` +
				`<Track ratingKey="canonical" title="Shhadi Ya Deni" grandparentTitle="نانسي عجرم" parentTitle="Shhadi Ya Deni"/>` +
				`</MediaContainer>`))
			return
		}
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="0"></MediaContainer>`))
	}))
	defer ts.Close()

	cfg := &config.Config{Plex: config.PlexConfig{
		URL:                    ts.URL,
		Token:                  "tok",
		LibrarySectionID:       2,
		MatchConfidencePercent: config.DefaultMatchConfidencePercent,
	}}
	c := NewClient(cfg)

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
		t.Fatalf("expected canonical full-library match, got %+v (%s)", got, kind)
	}
	if aliasQueried.Load() {
		t.Fatal("alias queried before the canonical full-library scan resolved the track")
	}
}
