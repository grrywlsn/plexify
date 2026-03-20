package track

import (
	"reflect"
	"testing"
)

func TestPrimaryListedArtist(t *testing.T) {
	tests := []struct {
		in, want string
	}{
		{"", ""},
		{"  ", ""},
		{"Le Youth, Forester, Robertson", "Le Youth"},
		{"TOMORA, AURORA, Tom Rowlands", "TOMORA"},
		{"Solo Act", "Solo Act"},
		{"  A , B , C  ", "A"},
	}
	for _, tc := range tests {
		if got := PrimaryListedArtist(tc.in); got != tc.want {
			t.Errorf("PrimaryListedArtist(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestTrack_PlexSearchArtistCandidates(t *testing.T) {
	t.Parallel()
	if got := (Track{Artist: ""}).PlexSearchArtistCandidates(); !reflect.DeepEqual(got, []string{""}) {
		t.Errorf("empty: %v", got)
	}
	if got := (Track{Artist: "Solo"}).PlexSearchArtistCandidates(); !reflect.DeepEqual(got, []string{"Solo"}) {
		t.Errorf("solo: %v", got)
	}
	want := []string{"Le Youth", "Le Youth, Forester, Robertson"}
	if got := (Track{Artist: "Le Youth, Forester, Robertson"}).PlexSearchArtistCandidates(); !reflect.DeepEqual(got, want) {
		t.Errorf("multi: got %v want %v", got, want)
	}
}
