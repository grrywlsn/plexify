package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAddTracksToPlaylist_duplicateLibraryTrackUsesAfter(t *testing.T) {
	var putCalls int
	var afterSeen []string

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/playlists/") || !strings.HasSuffix(r.URL.Path, "/items") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			putCalls++
			afterSeen = append(afterSeen, r.URL.Query().Get("after"))
			_, _ = fmt.Fprintf(w, `<MediaContainer leafCountAdded="1"/>`)
		case http.MethodGet:
			start := r.URL.Query().Get("X-Plex-Container-Start")
			switch start {
			case "0":
				_, _ = fmt.Fprintf(w, `<MediaContainer size="1" totalSize="2"><Track ratingKey="11" playlistItemID="100" title="One"/></MediaContainer>`)
			case "1":
				_, _ = fmt.Fprintf(w, `<MediaContainer size="1" totalSize="2"><Track ratingKey="11" playlistItemID="101" title="One"/></MediaContainer>`)
			default:
				http.Error(w, "unexpected start", http.StatusBadRequest)
			}
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	cl := &Client{
		baseURL:    strings.TrimSuffix(srv.URL, "/"),
		token:      "token",
		serverID:   "machine-id",
		httpClient: srv.Client(),
	}

	err := cl.AddTracksToPlaylist(context.Background(), "42", []string{"11", "11"})
	if err != nil {
		t.Fatal(err)
	}
	if putCalls != 2 {
		t.Fatalf("expected 2 PUTs, got %d", putCalls)
	}
	if len(afterSeen) != 2 || afterSeen[0] != "" || afterSeen[1] != "100" {
		t.Fatalf("after query params: %#v (want \"\", \"100\")", afterSeen)
	}
}
