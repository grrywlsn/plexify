package plex

import "net/http"

// roundTripperFunc adapts a function to [http.RoundTripper] for tests.
type roundTripperFunc func(*http.Request) (*http.Response, error)

func (f roundTripperFunc) RoundTrip(r *http.Request) (*http.Response, error) {
	return f(r)
}
