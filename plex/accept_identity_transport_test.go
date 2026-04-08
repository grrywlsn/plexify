package plex

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestAcceptIdentityTransport_SetsIdentityWhenHeaderUnset(t *testing.T) {
	t.Parallel()

	var seen string
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seen = r.Header.Get("Accept-Encoding")
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	tr := &acceptIdentityTransport{base: base}
	req, err := http.NewRequest(http.MethodGet, "http://plex.test/", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := tr.RoundTrip(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if seen != "identity" {
		t.Fatalf("Accept-Encoding: got %q, want identity", seen)
	}
	if got := req.Header.Get("Accept-Encoding"); got != "" {
		t.Fatalf("caller request must not be mutated; Accept-Encoding got %q", got)
	}
}

func TestAcceptIdentityTransport_PreservesExplicitAcceptEncoding(t *testing.T) {
	t.Parallel()

	var seen string
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		seen = r.Header.Get("Accept-Encoding")
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("")),
		}, nil
	})

	tr := &acceptIdentityTransport{base: base}
	req, err := http.NewRequest(http.MethodGet, "http://plex.test/", nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Accept-Encoding", "gzip")

	if _, err := tr.RoundTrip(req); err != nil {
		t.Fatal(err)
	}

	if seen != "gzip" {
		t.Fatalf("Accept-Encoding: got %q, want gzip", seen)
	}
}

func TestAcceptIdentityTransport_NilBaseUsesDefaultTransport(t *testing.T) {
	t.Parallel()

	tr := &acceptIdentityTransport{base: nil}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://127.0.0.1:1/", nil)
	if err != nil {
		t.Fatal(err)
	}
	// DefaultTransport dials; connection refused is fast. Ensures nil base does not panic.
	_, err = tr.RoundTrip(req)
	if err == nil {
		t.Fatal("expected dial / connection error")
	}
}
