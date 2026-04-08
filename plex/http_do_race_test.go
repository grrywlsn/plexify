package plex

import (
	"context"
	"io"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// TestHTTPDo_ConcurrentDoesNotRaceOnCallerHeader stresses parallel Plex-style GETs through
// httpDo + acceptIdentityTransport (same stack as production match concurrency).
func TestHTTPDo_ConcurrentDoesNotRaceOnCallerHeader(t *testing.T) {
	t.Parallel()

	const workers = 64
	var wg sync.WaitGroup
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Errorf("missing identity Accept-Encoding: %q", r.Header.Get("Accept-Encoding"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("<MediaContainer></MediaContainer>")),
		}, nil
	})

	c := &Client{
		httpClient: &http.Client{
			Transport: &acceptIdentityTransport{base: base},
			Timeout:   0,
		},
	}

	ctx := context.Background()
	for i := range workers {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			u := "http://plex.test/search?q=" + strconv.Itoa(i)
			req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
			if err != nil {
				t.Error(err)
				return
			}
			req.Header.Set("Accept", "application/xml")
			req.Header.Set("X-Worker", strconv.Itoa(i))
			if v := req.Header.Get("Accept"); v != "application/xml" {
				t.Errorf("header before httpDo: %q", v)
				return
			}
			resp, err := c.httpDo(req)
			if err != nil {
				t.Error(err)
				return
			}
			if _, err := io.Copy(io.Discard, resp.Body); err != nil {
				t.Error(err)
			}
			if err := resp.Body.Close(); err != nil {
				t.Error(err)
			}
			if v := req.Header.Get("Accept-Encoding"); v != "" {
				t.Errorf("caller request must not gain Accept-Encoding after RoundTrip; got %q", v)
			}
		}(i)
	}
	wg.Wait()
}
