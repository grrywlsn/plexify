package plex

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"
)

func TestHTTPDo_WrapsBodyWithCancelAndReadsOK(t *testing.T) {
	t.Parallel()

	sent := `<MediaContainer size="0"></MediaContainer>`
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		if r.Header.Get("Accept-Encoding") != "identity" {
			t.Fatalf("downstream must see Accept-Encoding=identity, got %q", r.Header.Get("Accept-Encoding"))
		}
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader(sent)),
		}, nil
	})

	c := &Client{
		httpClient: &http.Client{
			Transport: &acceptIdentityTransport{base: base},
			Timeout:   0,
		},
	}

	req, err := http.NewRequest(http.MethodGet, "http://plex.test/section", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.httpDo(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	bc, ok := resp.Body.(*bodyCancelCloser)
	if !ok {
		t.Fatalf("expected *bodyCancelCloser, got %T", resp.Body)
	}

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if string(raw) != sent {
		t.Fatalf("body: got %q, want %q", raw, sent)
	}

	if err := bc.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestHTTPDo_NilResponseBodyBecomesReadableEmpty(t *testing.T) {
	t.Parallel()

	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       nil,
		}, nil
	})

	c := &Client{httpClient: &http.Client{Transport: base, Timeout: 0}}
	req, err := http.NewRequest(http.MethodGet, "http://plex.test/", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.httpDo(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	if len(raw) != 0 {
		t.Fatalf("expected empty body, got %q", raw)
	}
}

func TestHTTPDo_RequestAlreadyHasDeadline_DoesNotWrapBody(t *testing.T) {
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

	c := &Client{httpClient: &http.Client{Transport: base, Timeout: 0}}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://plex.test/", nil)
	if err != nil {
		t.Fatal(err)
	}

	resp, err := c.httpDo(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	if _, ok := resp.Body.(*bodyCancelCloser); ok {
		t.Fatal("did not expect bodyCancelCloser when request context already had deadline")
	}
}

func TestBodyCancelCloser_InvokesCancelOnce(t *testing.T) {
	t.Parallel()

	var cancels int
	bc := &bodyCancelCloser{
		ReadCloser: io.NopCloser(strings.NewReader("xy")),
		cancel:     func() { cancels++ },
	}
	if _, err := io.ReadAll(bc); err != nil {
		t.Fatal(err)
	}
	if err := bc.Close(); err != nil {
		t.Fatal(err)
	}
	if cancels != 1 {
		t.Fatalf("cancel called %d times, want 1", cancels)
	}
	if err := bc.Close(); err != nil {
		t.Fatal(err)
	}
	if cancels != 1 {
		t.Fatalf("second Close must not run cancel again; got %d calls", cancels)
	}
}

func TestHTTPDo_OnDoErrorCancelIsNotLeaked(t *testing.T) {
	t.Parallel()

	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		return nil, io.ErrUnexpectedEOF
	})

	c := &Client{httpClient: &http.Client{Transport: base, Timeout: 0}}
	req, err := http.NewRequest(http.MethodGet, "http://plex.test/", nil)
	if err != nil {
		t.Fatal(err)
	}

	_, err = c.httpDo(req)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestHTTPDo_SequentialRequestsUnderRateLimit(t *testing.T) {
	t.Parallel()

	n := 25
	count := 0
	base := roundTripperFunc(func(r *http.Request) (*http.Response, error) {
		count++
		return &http.Response{
			StatusCode: http.StatusOK,
			ProtoMajor: 1,
			ProtoMinor: 1,
			Header:     make(http.Header),
			Body:       io.NopCloser(strings.NewReader("{}")),
		}, nil
	})

	rt := &acceptIdentityTransport{base: newRateLimitedTransport(base, 100)}
	c := &Client{httpClient: &http.Client{Transport: rt, Timeout: 0}}

	for i := 0; i < n; i++ {
		req, err := http.NewRequest(http.MethodGet, "http://plex.test/", nil)
		if err != nil {
			t.Fatal(err)
		}
		resp, err := c.httpDo(req)
		if err != nil {
			t.Fatalf("iter %d: %v", i, err)
		}
		if _, err := io.Copy(io.Discard, resp.Body); err != nil {
			t.Fatal(err)
		}
		if err := resp.Body.Close(); err != nil {
			t.Fatal(err)
		}
	}

	if count != n {
		t.Fatalf("server saw %d requests, want %d", count, n)
	}
}
