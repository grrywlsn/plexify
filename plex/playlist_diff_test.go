package plex

import (
	"strings"
	"testing"

	"github.com/grrywlsn/plexify/track"
)

func desiredEntry(sourceArtist, sourceTitle, plexID, plexArtist, plexTitle string, conf float64) PlaylistDesiredEntry {
	return PlaylistDesiredEntry{
		MatchResult: MatchResult{
			SourceTrack: track.Track{Artist: sourceArtist, Name: sourceTitle},
			PlexTrack:   &PlexTrack{ID: plexID, Artist: plexArtist, Title: plexTitle},
			MatchType:   MatchTypeTitleArtist,
			Confidence:  conf,
		},
	}
}

func TestBuildPlaylistDiffOps_identical(t *testing.T) {
	old := []PlexTrack{
		{ID: "10", Artist: "A", Title: "One"},
		{ID: "20", Artist: "B", Title: "Two"},
	}
	desired := []PlaylistDesiredEntry{
		desiredEntry("a", "one", "10", "A", "One", 0.95),
		desiredEntry("b", "two", "20", "B", "Two", 0.95),
	}
	ops := BuildPlaylistDiffOps(old, desired)
	if len(ops) != 0 {
		t.Fatalf("expected no ops, got %d: %+v", len(ops), ops)
	}
}

func TestBuildPlaylistDiffOps_addOnly(t *testing.T) {
	desired := []PlaylistDesiredEntry{
		desiredEntry("s1", "Song1", "1", "P1", "Song1", 0.9),
		desiredEntry("s2", "Song2", "2", "P2", "Song2", 0.85),
	}
	ops := BuildPlaylistDiffOps(nil, desired)
	if len(ops) != 2 {
		t.Fatalf("expected 2 add ops, got %d", len(ops))
	}
	for i, op := range ops {
		if op.Kind != PlaylistDiffAdd || op.NewIdx != i || op.OldIdx != -1 {
			t.Fatalf("op %d: want add newIdx=%d, got %+v", i, i, op)
		}
	}
}

func TestBuildPlaylistDiffOps_removeOnly(t *testing.T) {
	old := []PlexTrack{
		{ID: "9", Artist: "X", Title: "Gone"},
	}
	ops := BuildPlaylistDiffOps(old, nil)
	if len(ops) != 1 || ops[0].Kind != PlaylistDiffRemove || ops[0].OldIdx != 0 {
		t.Fatalf("got %+v", ops)
	}
}

func TestBuildPlaylistDiffOps_substitutionCoalesced(t *testing.T) {
	old := []PlexTrack{
		{ID: "1", Artist: "A", Title: "Keep"},
		{ID: "2", Artist: "OldArt", Title: "OldTitle"},
		{ID: "3", Artist: "C", Title: "Tail"},
	}
	desired := []PlaylistDesiredEntry{
		desiredEntry("a", "keep", "1", "A", "Keep", 1),
		desiredEntry("ns", "new", "99", "NewArt", "NewTitle", 0.8),
		desiredEntry("c", "tail", "3", "C", "Tail", 1),
	}
	ops := BuildPlaylistDiffOps(old, desired)
	if len(ops) != 1 {
		t.Fatalf("expected 1 coalesced change op, got %d: %+v", len(ops), ops)
	}
	if ops[0].Kind != PlaylistDiffChange || ops[0].OldIdx != 1 || ops[0].NewIdx != 1 {
		t.Fatalf("unexpected op: %+v", ops[0])
	}
}

func TestBuildPlaylistDiffOps_reorderShowsAsAddRemove(t *testing.T) {
	old := []PlexTrack{
		{ID: "a", Artist: "A", Title: "1"},
		{ID: "b", Artist: "B", Title: "2"},
	}
	desired := []PlaylistDesiredEntry{
		desiredEntry("", "", "b", "B", "2", 1),
		desiredEntry("", "", "a", "A", "1", 1),
	}
	ops := BuildPlaylistDiffOps(old, desired)
	// Swap: LCS length 1 — expect multiple ops (no single coalesce for pure swap)
	if len(ops) < 2 {
		t.Fatalf("expected at least 2 ops for swap, got %d %+v", len(ops), ops)
	}
}

func TestFprintPlaylistDiff_noColor(t *testing.T) {
	var b strings.Builder
	old := []PlexTrack{{ID: "1", Artist: "O", Title: "Old"}}
	desired := []PlaylistDesiredEntry{
		desiredEntry("s", "src", "2", "N", "New", 0.75),
	}
	view := NewPlaylistDiffView("Test PL", old, desired)
	FprintPlaylistDiff(&b, view, false)
	out := b.String()
	if !strings.Contains(out, "PLAYLIST CHANGES") {
		t.Fatal("missing header")
	}
	if strings.Contains(out, "\033[") {
		t.Fatal("should not contain ANSI when useColor=false")
	}
	if !strings.Contains(out, "was (plex)") {
		t.Fatal("expected change section", out)
	}
}
