package plex

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewRateLimitedTransport_zeroDisables(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	t.Cleanup(ts.Close)

	rt := newRateLimitedTransport(http.DefaultTransport, 0)
	c := &http.Client{Transport: rt}

	start := time.Now()
	for range 5 {
		resp, err := c.Get(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	if d := time.Since(start); d > 2*time.Second {
		t.Fatalf("expected no rate limit with rps=0, took %v", d)
	}
}

func TestNewRateLimitedTransport_limits(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = io.Copy(w, strings.NewReader("ok"))
	}))
	t.Cleanup(ts.Close)

	rt := newRateLimitedTransport(http.DefaultTransport, 2)
	c := &http.Client{Transport: rt}

	start := time.Now()
	for range 3 {
		resp, err := c.Get(ts.URL)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	// 2 rps → ≥0.5s between first two and next two starts; three calls need meaningful delay vs unconstrained localhost.
	if d := time.Since(start); d < 400*time.Millisecond {
		t.Fatalf("expected rate limit to add delay, took only %v", d)
	}
}
