package plex

import (
	"io"
	"net/http"
	"strings"
	"sync"
)

// pipelineLockTransport serializes the entire Plex HTTP interaction: one in-flight
// RoundTrip plus response body read/close at a time for this client.
//
// A mutex held only for RoundTrip is insufficient: net/http can return response headers
// while the body is still streaming, so multiple goroutines (e.g. PLEX_MATCH_CONCURRENCY > 1)
// would still interleave body reads on the same *http.Transport, which has been observed
// in production as GC "bad pointer" / heap corruption under load (Docker/Kubernetes).
type pipelineLockTransport struct {
	mu   *sync.Mutex
	base http.RoundTripper
}

func newPipelineLockTransport(mu *sync.Mutex, base http.RoundTripper) http.RoundTripper {
	if mu == nil {
		panic("pipelineLockTransport: nil mutex")
	}
	if base == nil {
		base = http.DefaultTransport
	}
	return &pipelineLockTransport{mu: mu, base: base}
}

func (t *pipelineLockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	t.mu.Lock()
	resp, err := t.base.RoundTrip(req)
	if err != nil {
		t.mu.Unlock()
		return nil, err
	}
	if resp == nil {
		t.mu.Unlock()
		return nil, nil
	}
	body := resp.Body
	if body == nil {
		body = io.NopCloser(strings.NewReader(""))
	}
	resp.Body = &pipelineUnlockOnClose{ReadCloser: body, mu: t.mu}
	return resp, nil
}

type pipelineUnlockOnClose struct {
	io.ReadCloser
	mu   *sync.Mutex
	once sync.Once
}

func (b *pipelineUnlockOnClose) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(func() {
		if b.mu != nil {
			b.mu.Unlock()
		}
	})
	return err
}
