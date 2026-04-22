package plex

import (
	"testing"

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
}
