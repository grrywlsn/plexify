package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/grrywlsn/plexify/config"
)

func TestSearchEntireLibrary_DecodesBufferedXML(t *testing.T) {
	t.Parallel()

	const sectionID = 7
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := fmt.Sprintf("/library/sections/%d/all", sectionID)
		if r.URL.Path != wantPath {
			t.Errorf("request path %q, want %q", r.URL.Path, wantPath)
		}
		if got := r.URL.Query().Get("type"); got != PlexMusicTrackType {
			t.Errorf("type param %q, want %q", got, PlexMusicTrackType)
		}
		w.Header().Set("Content-Type", "application/xml")
		_, _ = fmt.Fprintf(w, `<?xml version="1.0"?>`+
			`<MediaContainer>`+
			`<Track ratingKey="42" title="Favorite Person" grandparentTitle="Royal"/>`+
			`</MediaContainer>`)
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

	tr, err := c.searchEntireLibrary(context.Background(), "Favorite Person", "Royal", "")
	if err != nil {
		t.Fatal(err)
	}
	if tr == nil {
		t.Fatal("expected track, got nil")
	}
	if tr.ID != "42" || tr.Title != "Favorite Person" || tr.Artist != "Royal" {
		t.Fatalf("unexpected track: %+v", tr)
	}
}
