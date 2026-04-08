package plex

import (
	"context"
	"io"
	"net/http"
	"sync"
)

// httpDo behaves like http.Client.Do with an overall deadline of [DefaultHTTPTimeout]
// when req.Context has no deadline. The cancel func runs when resp.Body is closed
// (or immediately if Do returns an error).
//
// [http.Client.Timeout] must be zero when using custom RoundTripper wrappers: if the
// transport is not *http.Transport, net/http falls back to setRequestCancel's legacy
// timer goroutine, which can fatally error on Go 1.26+ ("select on synctest…").
func (c *Client) httpDo(req *http.Request) (*http.Response, error) {
	if _, ok := req.Context().Deadline(); ok {
		return c.httpClient.Do(req)
	}
	ctx, cancel := context.WithTimeout(req.Context(), DefaultHTTPTimeout)
	req = req.Clone(ctx)
	resp, err := c.httpClient.Do(req)
	if err != nil {
		cancel()
		return nil, err
	}
	resp.Body = &bodyCancelCloser{ReadCloser: resp.Body, cancel: cancel}
	return resp, nil
}

type bodyCancelCloser struct {
	io.ReadCloser
	cancel context.CancelFunc
	once   sync.Once
}

func (b *bodyCancelCloser) Close() error {
	err := b.ReadCloser.Close()
	b.once.Do(b.cancel)
	return err
}
