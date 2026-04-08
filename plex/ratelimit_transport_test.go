package plex

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
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
	// 2 rps → ~500ms between starts; three sequential calls need ≈1s of spacing.
	if d := time.Since(start); d < 400*time.Millisecond {
		t.Fatalf("expected rate limit to add delay, took only %v", d)
	}
}

func TestMinIntervalFromRPS(t *testing.T) {
	t.Parallel()

	if g := minIntervalFromRPS(0); g != 0 {
		t.Fatalf("0 rps: got %v", g)
	}
	if g := minIntervalFromRPS(4); g != 250*time.Millisecond {
		t.Fatalf("4 rps: got %v want 250ms", g)
	}
	if g := minIntervalFromRPS(10); g != 100*time.Millisecond {
		t.Fatalf("10 rps: got %v", g)
	}
}

func TestRateLimitedTransport_ContextCanceledDuringWait(t *testing.T) {
	t.Parallel()

	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("ok")),
		}, nil
	})
	rt := newRateLimitedTransport(base, 2)

	resp, err := rt.RoundTrip(mustReq(t, context.Background()))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()

	ctx, cancel := context.WithCancel(context.Background())
	req := mustReq(t, ctx)
	ch := make(chan error, 1)
	go func() {
		_, err := rt.RoundTrip(req)
		ch <- err
	}()
	time.Sleep(30 * time.Millisecond)
	cancel()
	select {
	case err := <-ch:
		if err != context.Canceled {
			t.Fatalf("got %v, want %v", err, context.Canceled)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for RoundTrip after cancel")
	}
}

func TestRateLimitedTransport_SequentialRespectsGap(t *testing.T) {
	t.Parallel()

	var times []time.Time
	var mu sync.Mutex
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		mu.Lock()
		times = append(times, time.Now())
		mu.Unlock()
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("x")),
		}, nil
	})
	rt := newRateLimitedTransport(base, 100) // 10ms between starts

	for range 3 {
		resp, err := rt.RoundTrip(mustReq(t, context.Background()))
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
	}
	mu.Lock()
	defer mu.Unlock()
	if len(times) != 3 {
		t.Fatalf("got %d calls", len(times))
	}
	if times[1].Sub(times[0]) < 8*time.Millisecond {
		t.Fatalf("2nd too soon after 1st: %v", times[1].Sub(times[0]))
	}
	if times[2].Sub(times[1]) < 8*time.Millisecond {
		t.Fatalf("3rd too soon after 2nd: %v", times[2].Sub(times[1]))
	}
}

func mustReq(tb testing.TB, ctx context.Context) *http.Request {
	tb.Helper()
	r, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://plex.test/p", nil)
	if err != nil {
		tb.Fatal(err)
	}
	return r
}
