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
		{"SOPHIE & Bibi Bourelly", "SOPHIE"},
		{"Simon & Garfunkel", "Simon"},
		{"Headliner & Guest", "Headliner"},
		{"SingleName", "SingleName"},
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
	wantAmp := []string{"SOPHIE", "SOPHIE & Bibi Bourelly"}
	if got := (Track{Artist: "SOPHIE & Bibi Bourelly"}).PlexSearchArtistCandidates(); !reflect.DeepEqual(got, wantAmp) {
		t.Errorf("ampersand collab: got %v want %v", got, wantAmp)
	}
	mb := []string{"Wynter Gordon", "Diana Gordon"}
	wantMB := []string{"Wynter Gordon", "Diana Gordon"}
	if got := (Track{Artist: "Wynter Gordon", MusicBrainzArtistCredits: mb}).PlexSearchArtistCandidates(); !reflect.DeepEqual(got, wantMB) {
		t.Errorf("mb dedupe with artist: got %v want %v", got, wantMB)
	}
	wantMB2 := []string{"Le Youth", "Le Youth, Forester, Robertson", "Guest"}
	if got := (Track{Artist: "Le Youth, Forester, Robertson", MusicBrainzArtistCredits: []string{"Guest"}}).PlexSearchArtistCandidates(); !reflect.DeepEqual(got, wantMB2) {
		t.Errorf("mb after comma artist: got %v want %v", got, wantMB2)
	}
	wantCreditsOnly := []string{"Diana Gordon"}
	if got := (Track{MusicBrainzArtistCredits: []string{"Diana Gordon"}}).PlexSearchArtistCandidates(); !reflect.DeepEqual(got, wantCreditsOnly) {
		t.Errorf("credits only: got %v want %v", got, wantCreditsOnly)
	}
}
