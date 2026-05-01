package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/track"
)

func TestFindBestMatch_grandparentTitleSortRename(t *testing.T) {
	t.Parallel()
	c := &Client{}
	defaultPct := config.DefaultMatchConfidencePercent
	c.matchConfidencePercent = &defaultPct
	tracks := []PlexTrack{{
		ID:                   "1",
		Title:                "Dirty Talk",
		Artist:               "Diana Gordon",
		GrandparentTitleSort: "Wynter Gordon",
		Album:                "Dirty Talk",
	}}
	got := c.FindBestMatch(tracks, "Dirty Talk", "Wynter Gordon", "Dirty Talk")
	if got == nil || got.ID != "1" {
		t.Fatalf("expected match via GrandparentTitleSort, got %v", got)
	}
}

func TestPlexTrack_exactTitleAndArtistMatch_grandparentTitleSort(t *testing.T) {
	t.Parallel()
	tr := PlexTrack{Title: "Dirty Talk", Artist: "Diana Gordon", GrandparentTitleSort: "Wynter Gordon"}
	if !tr.exactTitleAndArtistMatch("dirty talk", "wynter gordon") {
		t.Fatal("expected exact match on GrandparentTitleSort")
	}
	if !tr.exactTitleAndArtistMatch("dirty talk", "diana gordon") {
		t.Fatal("expected exact match on grandparentTitle (display artist)")
	}
}

func TestFindBestMatch_twoCandidates_sortDoesNotStealOriginalTitle(t *testing.T) {
	t.Parallel()
	c := &Client{}
	defaultPct := config.DefaultMatchConfidencePercent
	c.matchConfidencePercent = &defaultPct
	tracks := []PlexTrack{
		{ID: "a", Title: "Song", Artist: "Various Artists", OriginalTitle: "Other", Album: "Comp"},
		{ID: "b", Title: "Song", Artist: "Various Artists", OriginalTitle: "Madonna", Album: "Comp", GrandparentTitleSort: "Madonna Sort"},
	}
	got := c.FindBestMatch(tracks, "Song", "Madonna", "Comp")
	if got == nil || got.ID != "b" {
		t.Fatalf("wanted originalTitle row b, got %v", got)
	}
}

func TestCalculateConfidence_grandparentTitleSort(t *testing.T) {
	t.Parallel()
	c := &Client{}
	song := track.Track{Name: "Dirty Talk", Artist: "Wynter Gordon", Album: "Dirty Talk"}
	withSort := &PlexTrack{Title: "Dirty Talk", Artist: "Diana Gordon", GrandparentTitleSort: "Wynter Gordon", Album: "Dirty Talk"}
	noSort := &PlexTrack{Title: "Dirty Talk", Artist: "Diana Gordon", Album: "Dirty Talk"}
	confWith := c.calculateConfidence(song, withSort, MatchTypeTitleArtist)
	confNo := c.calculateConfidence(song, noSort, MatchTypeTitleArtist)
	if confWith <= confNo {
		t.Fatalf("expected higher confidence with matching GrandparentTitleSort: %g vs %g", confWith, confNo)
	}
}

func TestFindBestMatchWithOptionalArtistSortRetry_fetchesMetadataOnce(t *testing.T) {
	t.Parallel()
	var metadataGets int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		if strings.HasPrefix(r.URL.Path, "/library/metadata/") {
			atomic.AddInt32(&metadataGets, 1)
			fmt.Fprintf(w, `<?xml version="1.0"?><MediaContainer size="1">
				<Artist ratingKey="900" title="Diana Gordon" titleSort="Wynter Gordon"/></MediaContainer>`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := &config.Config{
		Plex: config.PlexConfig{
			URL:                    ts.URL,
			Token:                  "tok",
			LibrarySectionID:       2,
			MatchConfidencePercent: 92,
		},
	}
	c := NewClient(cfg)
	tracks := []PlexTrack{{
		ID:                   "50",
		Title:                "Dirty Talk",
		Artist:               "Unrelated Plex Display Name Xq",
		GrandparentRatingKey: "900",
		Album:                "Dirty Talk",
	}}
	got := c.findBestMatchWithOptionalArtistSortRetry(context.Background(), tracks, "Dirty Talk", "Wynter Gordon", "Dirty Talk", false)
	if got == nil || got.ID != "50" {
		t.Fatalf("expected retry match via artist metadata, got %v", got)
	}
	if atomic.LoadInt32(&metadataGets) != 1 {
		t.Fatalf("expected exactly one GET /library/metadata/, got %d", metadataGets)
	}
	if strings.TrimSpace(tracks[0].GrandparentTitleSort) != "Wynter Gordon" {
		t.Fatalf("expected slice mutated with sort title, got %q", tracks[0].GrandparentTitleSort)
	}
}

func TestFindBestMatchWithOptionalArtistSortRetry_noMetadataWhenPass1Works(t *testing.T) {
	t.Parallel()
	var metadataGets int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/library/metadata/") {
			atomic.AddInt32(&metadataGets, 1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	defer ts.Close()

	cfg := &config.Config{
		Plex: config.PlexConfig{
			URL:                    ts.URL,
			Token:                  "tok",
			LibrarySectionID:       2,
			MatchConfidencePercent: config.DefaultMatchConfidencePercent,
		},
	}
	c := NewClient(cfg)
	tracks := []PlexTrack{{ID: "51", Title: "Honey", Artist: "Robyn", Album: "Body Talk", GrandparentRatingKey: "902"}}
	got := c.findBestMatchWithOptionalArtistSortRetry(context.Background(), tracks, "Honey", "Robyn", "Body Talk", false)
	if got == nil || got.ID != "51" {
		t.Fatalf("expected Pass 1 match, got %v", got)
	}
	if atomic.LoadInt32(&metadataGets) != 0 {
		t.Fatalf("expected zero metadata GET when Pass 1 succeeds, got %d", metadataGets)
	}
}

func TestGrandparentKeysForLibrarySortEnrichment_respectsThresholdAndCap(t *testing.T) {
	t.Parallel()
	c := &Client{}
	tracks := make([]PlexTrack, libraryArtistSortEnrichMaxKeys+3)
	for i := range tracks {
		tracks[i] = PlexTrack{
			Title:                fmt.Sprintf("Other Song %d", i),
			GrandparentRatingKey: fmt.Sprintf("k%d", i),
			Artist:               "A",
			GrandparentTitleSort: "",
		}
	}
	tracks[0] = PlexTrack{Title: "Target Hit", GrandparentRatingKey: "best", Artist: "X"}
	m := c.grandparentKeysForLibrarySortEnrichment(tracks, "Target Hit")
	if len(m) != 1 {
		t.Fatalf("want 1 key above threshold, got %d", len(m))
	}
	if _, ok := m["best"]; !ok {
		t.Fatal("expected best key")
	}
}
