package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAddTracksToPlaylist_duplicateLibraryTrackUsesCommaMetadataURI(t *testing.T) {
	var putCalls, getCalls int

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.Contains(r.URL.Path, "/playlists/") || !strings.HasSuffix(r.URL.Path, "/items") {
			http.NotFound(w, r)
			return
		}
		switch r.Method {
		case http.MethodPut:
			putCalls++
			uri := r.URL.Query().Get("uri")
			if r.URL.Query().Get("after") != "" {
				t.Errorf("unexpected after= on single-batch duplicate add: %q", r.URL.Query().Get("after"))
			}
			if !strings.Contains(uri, "/library/metadata/11,11") {
				t.Errorf("expected comma-separated rating keys in uri, got %q", uri)
			}
			_, _ = fmt.Fprintf(w, `<MediaContainer leafCountAdded="2"/>`)
		case http.MethodGet:
			getCalls++
			http.Error(w, "unexpected GET in single-batch flow", http.StatusBadRequest)
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
	if putCalls != 1 {
		t.Fatalf("expected 1 PUT, got %d", putCalls)
	}
	if getCalls != 0 {
		t.Fatalf("expected no GET, got %d", getCalls)
	}
}

func TestAddTracksToPlaylist_multiBatchUsesAfterBetweenChunks(t *testing.T) {
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
			uri := r.URL.Query().Get("uri")
			switch putCalls {
			case 1:
				if afterSeen[0] != "" {
					t.Errorf("first PUT should have empty after, got %q", afterSeen[0])
				}
				if !strings.Contains(uri, "/library/metadata/a") || strings.Contains(uri, ",") {
					t.Errorf("first batch should be single key a, uri=%q", uri)
				}
			case 2:
				if afterSeen[1] != "100" {
					t.Errorf("second PUT after= want 100, got %q", afterSeen[1])
				}
				if !strings.Contains(uri, "/library/metadata/b") {
					t.Errorf("second batch uri should contain metadata/b, got %q", uri)
				}
			default:
				t.Fatalf("unexpected PUT #%d", putCalls)
			}
			_, _ = fmt.Fprintf(w, `<MediaContainer leafCountAdded="1"/>`)
		case http.MethodGet:
			start := r.URL.Query().Get("X-Plex-Container-Start")
			if start != "0" {
				http.Error(w, "unexpected start", http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprintf(w, `<MediaContainer size="1" totalSize="1"><Track ratingKey="a" playlistItemID="100" title="A"/></MediaContainer>`)
		default:
			http.Error(w, "method", http.StatusMethodNotAllowed)
		}
	}))
	defer srv.Close()

	cl := &Client{
		baseURL:                      strings.TrimSuffix(srv.URL, "/"),
		token:                        "token",
		serverID:                     "machine-id",
		httpClient:                   srv.Client(),
		playlistBatchMaxCommaKeysLen: 2,
	}

	err := cl.AddTracksToPlaylist(context.Background(), "42", []string{"a", "b"})
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
