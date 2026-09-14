package plex

import (
	"context"
	"encoding/xml"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grrywlsn/plexify/config"
	"github.com/grrywlsn/plexify/track"
)

// Plex can return a track with an empty title and the real title in titleSort. These two
// payloads are trimmed from live /library/sections/6/all responses.
const (
	melatoninTrackXML = `<Track ratingKey="220940" title="" titleSort="Melatonin" ` +
		`grandparentTitle="Tinashe" grandparentRatingKey="196519" parentTitle="Melatonin" duration="222178"/>`
	shesTheBestTrackXML = `<Track ratingKey="220798" title="" titleSort="She’s the Best" ` +
		`grandparentTitle="Troye Sivan" grandparentRatingKey="196414" parentTitle="She’s the Best" duration="254027"/>`
)

func TestPlexTrackUnmarshalXML_titleSortFallback(t *testing.T) {
	t.Parallel()

	var resp PlexResponse
	body := `<?xml version="1.0"?><MediaContainer size="2">` + melatoninTrackXML + shesTheBestTrackXML +
		`<Track ratingKey="3" title="Real Title" titleSort="Sort Title" grandparentTitle="Someone"/>` +
		`</MediaContainer>`
	if err := xml.Unmarshal([]byte(body), &resp); err != nil {
		t.Fatal(err)
	}
	if len(resp.Tracks) != 3 {
		t.Fatalf("got %d tracks, want 3", len(resp.Tracks))
	}
	if resp.Tracks[0].Title != "Melatonin" {
		t.Errorf("empty title should fall back to titleSort, got %q", resp.Tracks[0].Title)
	}
	if resp.Tracks[1].Title != "She’s the Best" {
		t.Errorf("empty title should fall back to titleSort, got %q", resp.Tracks[1].Title)
	}
	if resp.Tracks[2].Title != "Real Title" {
		t.Errorf("a present title must win over titleSort, got %q", resp.Tracks[2].Title)
	}
	if resp.Tracks[2].TitleSort != "Sort Title" {
		t.Errorf("titleSort should still be decoded, got %q", resp.Tracks[2].TitleSort)
	}
}

// These two tracks exist in Plex but were reported missing: the empty title scored as a blank
// string, so an otherwise exact match fell under the confidence threshold.
func TestSearchTrack_matchesTracksWithEmptyTitle(t *testing.T) {
	t.Parallel()

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/xml")
		_, _ = w.Write([]byte(`<?xml version="1.0"?><MediaContainer size="2">` +
			melatoninTrackXML + shesTheBestTrackXML + `</MediaContainer>`))
	}))
	defer ts.Close()

	cfg := &config.Config{Plex: config.PlexConfig{
		URL:                    ts.URL,
		Token:                  "tok",
		LibrarySectionID:       6,
		MatchConfidencePercent: config.DefaultMatchConfidencePercent,
	}}
	c := NewClient(cfg)
	c.SetSkipFullLibrarySearch(true)

	tests := []struct {
		song   track.Track
		wantID string
	}{
		{
			song: track.Track{
				Name: "Melatonin", Artist: "Tinashe", Album: "Melatonin",
				MusicBrainzArtistAliases: []string{"Tinashe Jorgenson Kachingwe"},
			},
			wantID: "220940",
		},
		{
			song: track.Track{
				Name: "She’s the Best", Artist: "Troye Sivan", Album: "She’s the Best",
				MusicBrainzArtistAliases: []string{"トロイ・シヴァン", "트로이 시반", "Troye Sivan Mellet", "트로이"},
			},
			wantID: "220798",
		},
	}

	for _, tc := range tests {
		t.Run(tc.song.Artist+" - "+tc.song.Name, func(t *testing.T) {
			got, kind, err := c.SearchTrack(context.Background(), tc.song)
			if err != nil {
				t.Fatal(err)
			}
			if got == nil {
				t.Fatal("expected a match")
			}
			if got.ID != tc.wantID {
				t.Fatalf("matched %q (%s), want id %s", got.Title, got.ID, tc.wantID)
			}
			if kind != MatchTypeTitleArtist {
				t.Errorf("kind = %s, want %s (canonical artist, not an alias)", kind, MatchTypeTitleArtist)
			}
			if conf := c.calculateConfidence(tc.song, got, kind); conf < c.minMatchScore() {
				t.Errorf("confidence %s below threshold %s",
					formatConfidencePercent(conf), formatConfidencePercent(c.minMatchScore()))
			}
		})
	}
}
