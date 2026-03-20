package plex

import (
	"testing"

	"github.com/grrywlsn/plexify/track"
)

func TestDesiredPlaylistEntries_skipsNilPlex(t *testing.T) {
	results := []MatchResult{
		{SourceTrack: track.Track{Name: "a"}, PlexTrack: &PlexTrack{ID: "1"}, MatchType: MatchTypeTitleArtist},
		{SourceTrack: track.Track{Name: "b"}, PlexTrack: nil, MatchType: MatchTypeNone},
		{SourceTrack: track.Track{Name: "c"}, PlexTrack: &PlexTrack{ID: "3"}, MatchType: MatchTypeTitleArtist},
	}
	got := DesiredPlaylistEntries(results)
	if len(got) != 2 {
		t.Fatalf("len=%d want 2", len(got))
	}
	if got[0].MatchResult.PlexTrack.ID != "1" || got[1].MatchResult.PlexTrack.ID != "3" {
		t.Fatalf("unexpected order/ids: %+v", got)
	}
}

func TestMatchedTrackIDs_order(t *testing.T) {
	entries := []PlaylistDesiredEntry{
		{MatchResult: MatchResult{PlexTrack: &PlexTrack{ID: "x"}}},
		{MatchResult: MatchResult{PlexTrack: &PlexTrack{ID: "y"}}},
	}
	ids := MatchedTrackIDs(entries)
	if len(ids) != 2 || ids[0] != "x" || ids[1] != "y" {
		t.Fatalf("got %v", ids)
	}
}

func TestMatchedTrackIDs_empty(t *testing.T) {
	if len(MatchedTrackIDs(nil)) != 0 {
		t.Fatal("expected empty")
	}
}
