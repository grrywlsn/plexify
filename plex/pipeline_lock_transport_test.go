package plex

import (
	"io"
	"net/http"
	"strings"
	"sync"
	"testing"
)

func TestPipelineLockTransport_ReleasesLockOnBodyClose(t *testing.T) {
	t.Parallel()

	var mu sync.Mutex
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})

	rt := newPipelineLockTransport(&mu, base)
	req, err := http.NewRequest(http.MethodGet, "http://plex.test/", nil)
	if err != nil {
		t.Fatal(err)
	}
	resp, err := rt.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	if mu.TryLock() {
		t.Fatal("mutex should still be held after RoundTrip (TryLock must fail)")
	}
	if _, err := io.ReadAll(resp.Body); err != nil {
		t.Fatal(err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatal(err)
	}
	if !mu.TryLock() {
		t.Fatal("mutex should be unlocked after body Close")
	}
	mu.Unlock()
}
