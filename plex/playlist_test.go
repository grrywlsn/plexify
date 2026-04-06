package plex

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func TestChunkTrackIDsByCommaLen_splitsOnRepeatedKey(t *testing.T) {
	tests := []struct {
		name   string
		ids    []string
		maxLen int
		want   [][]string
	}{
		{
			name:   "consecutive duplicate",
			ids:    []string{"11", "11"},
			maxLen: 100,
			want:   [][]string{{"11"}, {"11"}},
		},
		{
			name:   "repeat after other keys flushes prior batch then solo repeat",
			ids:    []string{"1", "2", "1"},
			maxLen: 100,
			want:   [][]string{{"1", "2"}, {"1"}},
		},
		{
			name:   "no duplicate single batch",
			ids:    []string{"1", "2", "3"},
			maxLen: 100,
			want:   [][]string{{"1", "2", "3"}},
		},
		{
			name:   "triple same key",
			ids:    []string{"9", "9", "9"},
			maxLen: 50,
			want:   [][]string{{"9"}, {"9"}, {"9"}},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := chunkTrackIDsByCommaLen(tt.ids, tt.maxLen, 100)
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("got %#v want %#v", got, tt.want)
			}
		})
	}
}

func TestChunkTrackIDsByCommaLen_respectsMaxItems(t *testing.T) {
	got := chunkTrackIDsByCommaLen([]string{"1", "2", "3", "4"}, 100, 2)
	want := [][]string{{"1", "2"}, {"3", "4"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestChunkTrackIDsByCommaLen_repeatFlushesBeforeMaxItemsCap(t *testing.T) {
	// Repeat must not share a comma batch with earlier keys; flush [1,2] before solo [1].
	got := chunkTrackIDsByCommaLen([]string{"1", "2", "1"}, 100, 2)
	want := [][]string{{"1", "2"}, {"1"}}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %#v want %#v", got, want)
	}
}

func TestAddTracksToPlaylist_duplicateLibraryTrackSplitsBatchesUsesAfter(t *testing.T) {
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
			after := r.URL.Query().Get("after")
			switch putCalls {
			case 1:
				if after != "" {
					t.Errorf("first PUT should not use after=, got %q", after)
				}
				if strings.Contains(uri, ",") {
					t.Errorf("first batch should be single key, uri=%q", uri)
				}
				if !strings.Contains(uri, "/library/metadata/11") {
					t.Errorf("expected metadata/11 in uri, got %q", uri)
				}
				_, _ = fmt.Fprintf(w, `<MediaContainer leafCountAdded="1"/>`)
			case 2:
				if after != "100" {
					t.Errorf("second PUT after= want 100, got %q", after)
				}
				if !strings.Contains(uri, "/library/metadata/11") {
					t.Errorf("second batch uri should contain metadata/11, got %q", uri)
				}
				_, _ = fmt.Fprintf(w, `<MediaContainer leafCountAdded="1"/>`)
			default:
				t.Fatalf("unexpected PUT #%d", putCalls)
			}
		case http.MethodGet:
			getCalls++
			if r.URL.Query().Get("X-Plex-Container-Start") != "0" {
				http.Error(w, "unexpected start", http.StatusBadRequest)
				return
			}
			_, _ = fmt.Fprintf(w, `<MediaContainer size="1" totalSize="2"><Track ratingKey="11" playlistItemID="100" title="One"/></MediaContainer>`)
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
	if getCalls != 1 {
		t.Fatalf("expected 1 GET (tail after first batch), got %d", getCalls)
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
