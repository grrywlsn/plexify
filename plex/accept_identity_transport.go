package plex

import "net/http"

// acceptIdentityTransport sets Accept-Encoding: identity on outbound requests when unset.
// Plex returns large XML; forcing identity skips net/http's gzipReader wrapper around the
// response body, which can interact badly with bodyEOFSignal under real Plex traffic.
type acceptIdentityTransport struct {
	base http.RoundTripper
}

func (t *acceptIdentityTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}
	r := req
	if req.Header.Get("Accept-Encoding") == "" {
		r = req.Clone(req.Context())
		r.Header.Set("Accept-Encoding", "identity")
	}
	return base.RoundTrip(r)
}
