package lidarr

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/grrywlsn/plexify/config"
)

func TestIsTransientLidarrErr(t *testing.T) {
	t.Parallel()
	cases := []struct {
		err  error
		want bool
	}{
		{err: nil, want: false},
		{err: context.Canceled, want: false},
		{err: context.DeadlineExceeded, want: true},
		{err: fmtError("PUT album 1: 500 Internal Server Error: readonly database"), want: true},
		{err: fmtError("add album: 503 Service Unavailable"), want: true},
		{err: fmtError("Lidarr album/lookup returned no results for release group x"), want: false},
		{err: fmtError("GET album 1: 404 Not Found"), want: false},
	}
	for _, tc := range cases {
		if got := isTransientLidarrErr(tc.err); got != tc.want {
			t.Errorf("isTransientLidarrErr(%v) = %v, want %v", tc.err, got, tc.want)
		}
	}
}

type fmtError string

func (e fmtError) Error() string { return string(e) }

func TestWriteErrorUserMessage(t *testing.T) {
	we := &WriteError{ReleaseGroupID: "abc-123", Reason: "database read-only"}
	msg := we.UserMessage()
	if msg == "" || !containsAll(msg, "abc-123", "re-run Plexify", "database read-only") {
		t.Fatalf("unexpected message: %q", msg)
	}
}

func containsAll(s string, parts ...string) bool {
	for _, p := range parts {
		if !strings.Contains(s, p) {
			return false
		}
	}
	return true
}

func withFastLidarrRetries(t *testing.T) {
	t.Helper()
	oldMax := writeMaxAttempts
	oldBackoff := writeBackoffFunc
	t.Cleanup(func() {
		writeMaxAttempts = oldMax
		writeBackoffFunc = oldBackoff
	})
	writeMaxAttempts = 4
	writeBackoffFunc = func(int) time.Duration { return time.Millisecond }
}

func TestAddReleaseGroupIfMissing_RetriesTransientWrite(t *testing.T) {
	withFastLidarrRetries(t)

	mbid := "aaaaaaaa-aaaa-aaaa-aaaa-aaaaaaaaaaaa"
	var putCalls atomic.Int32
	readonlyBody := `{"message":"attempt to write a readonly database"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album" && r.URL.Query().Get("foreignAlbumId") == mbid:
			_, _ = w.Write([]byte(`[{"id":42,"foreignAlbumId":"` + mbid + `"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/album/monitor":
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album/42":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 42.0, "monitored": false,
				"artist": map[string]interface{}{"id": 7.0, "monitored": false},
				"releases": []interface{}{
					map[string]interface{}{"id": 1.0, "monitored": false},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/album/42":
			n := putCalls.Add(1)
			if n < 3 {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(readonlyBody))
				return
			}
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/artist/7":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{"id": 7.0, "monitored": false})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/artist/7":
			w.WriteHeader(http.StatusAccepted)
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	c, err := NewClient(&config.LidarrConfig{URL: srv.URL, Token: "key"})
	if err != nil {
		t.Fatal(err)
	}
	res, err := c.AddReleaseGroupIfMissing(context.Background(), mbid)
	if err != nil {
		t.Fatalf("expected success after retries, got %v", err)
	}
	if !res.AlreadyPresent || !res.EnsuredMonitored {
		t.Fatalf("got %#v", res)
	}
	if putCalls.Load() != 3 {
		t.Fatalf("expected 3 PUT album attempts, got %d", putCalls.Load())
	}
}

func TestAddReleaseGroupIfMissing_WriteErrorAfterRetries(t *testing.T) {
	withFastLidarrRetries(t)

	mbid := "bbbbbbbb-bbbb-bbbb-bbbb-bbbbbbbbbbbb"
	readonlyBody := `{"message":"attempt to write a readonly database"}`

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album" && r.URL.Query().Get("foreignAlbumId") == mbid:
			_, _ = w.Write([]byte(`[{"id":99,"foreignAlbumId":"` + mbid + `"}]`))
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/album/monitor":
			w.WriteHeader(http.StatusAccepted)
		case r.Method == http.MethodGet && r.URL.Path == "/api/v1/album/99":
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"id": 99.0, "monitored": false,
				"artist": map[string]interface{}{"id": 8.0, "monitored": false},
				"releases": []interface{}{
					map[string]interface{}{"id": 2.0, "monitored": false},
				},
			})
		case r.Method == http.MethodPut && r.URL.Path == "/api/v1/album/99":
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(readonlyBody))
		default:
			t.Fatalf("unexpected %s %s", r.Method, r.URL.String())
		}
	}))
	defer srv.Close()

	c, err := NewClient(&config.LidarrConfig{URL: srv.URL, Token: "key"})
	if err != nil {
		t.Fatal(err)
	}
	_, err = c.AddReleaseGroupIfMissing(context.Background(), mbid)
	if err == nil {
		t.Fatal("expected error")
	}
	we, ok := AsWriteError(err)
	if !ok {
		t.Fatalf("expected WriteError, got %T: %v", err, err)
	}
	if we.ReleaseGroupID != mbid {
		t.Fatalf("release group: %q", we.ReleaseGroupID)
	}
	if we.Reason != "database read-only" {
		t.Fatalf("reason: %q", we.Reason)
	}
	if !containsAll(we.UserMessage(), "re-run Plexify", mbid) {
		t.Fatalf("user message: %q", we.UserMessage())
	}
}
