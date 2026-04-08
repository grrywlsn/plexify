package plex

import (
	"net/http"
	"sync"
	"time"
)

// newRateLimitedTransport wraps base with a minimum interval between outbound
// requests (~1/rps). rps <= 0 disables limiting.
//
// We avoid golang.org/x/time/rate.Limiter: on Go 1.26+, Limiter.Wait can fatally
// error with real request contexts ("receive on synctest channel from outside bubble").
func newRateLimitedTransport(base http.RoundTripper, rps float64) http.RoundTripper {
	if rps <= 0 {
		return base
	}
	if base == nil {
		base = http.DefaultTransport
	}
	gap := minIntervalFromRPS(rps)
	if gap <= 0 {
		return base
	}
	return &rateLimitedTransport{
		base:   base,
		minGap: gap,
	}
}

func minIntervalFromRPS(rps float64) time.Duration {
	if rps <= 0 {
		return 0
	}
	sec := 1.0 / rps
	ns := sec * float64(time.Second)
	if ns < 1 {
		return 1 * time.Nanosecond
	}
	return time.Duration(ns)
}

type rateLimitedTransport struct {
	base   http.RoundTripper
	minGap time.Duration
	mu     sync.Mutex
	last   time.Time // last time a slot was taken (zero = no request yet)
}

func (t *rateLimitedTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	if t.minGap <= 0 {
		return t.base.RoundTrip(req)
	}
	for {
		var wait time.Duration
		t.mu.Lock()
		now := time.Now()
		if t.last.IsZero() {
			wait = 0
		} else if elapsed := now.Sub(t.last); elapsed < t.minGap {
			wait = t.minGap - elapsed
		}
		if wait <= 0 {
			t.last = now
			t.mu.Unlock()
			break
		}
		t.mu.Unlock()
		timer := time.NewTimer(wait)
		select {
		case <-timer.C:
		case <-req.Context().Done():
			if !timer.Stop() {
				<-timer.C
			}
			return nil, req.Context().Err()
		}
	}
	return t.base.RoundTrip(req)
}
