package plex

import (
	"fmt"
	"net/http"
)

// acceptIdentityTransport sets Accept-Encoding: identity on outbound requests when unset.
// Plex returns large XML; forcing identity skips net/http's gzipReader wrapper around the
// response body, which can interact badly with bodyEOFSignal under real Plex traffic.
//
// RoundTrip always clones before mutating headers so the caller's *http.Request is never
// written to by this layer (safe with parallel callers sharing patterns and net/http).
type acceptIdentityTransport struct {
	base http.RoundTripper
}

func (t *acceptIdentityTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	r := req.Clone(req.Context())
	if r == nil {
		return nil, fmt.Errorf("http.Request.Clone returned nil")
	}
	if r.Header.Get("Accept-Encoding") == "" {
		r.Header.Set("Accept-Encoding", "identity")
	}
	return base.RoundTrip(r)
}
