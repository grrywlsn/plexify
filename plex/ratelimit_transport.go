package plex

import (
	"net/http"

	"golang.org/x/time/rate"
)

// newRateLimitedTransport wraps base with a token-bucket limiter. rps <= 0 disables limiting.
func newRateLimitedTransport(base http.RoundTripper, rps float64) http.RoundTripper {
	if rps <= 0 {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &rateLimitedTransport{
		base: base,
		lim:  rate.NewLimiter(rate.Limit(rps), 1),
	}
}

type rateLimitedTransport struct {
	base http.RoundTripper
	lim  *rate.Limiter
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if err := t.lim.Wait(req.Context()); err != nil {
		return nil, err
	}
	return t.base.RoundTrip(req)
}
