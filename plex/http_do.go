package plex

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
)

// httpDo behaves like http.Client.Do with an overall deadline of [DefaultHTTPTimeout]
// when req.Context has no deadline. The cancel func runs when resp.Body is closed
// (or immediately if Do returns an error).
//
// The outbound request is always a clone of req. net/http may mutate the request
// during redirects; sharing the caller's *http.Request across goroutines (e.g. parallel
// track matching) plus transport mutation caused concurrent map writes on Header.
//
// [http.Client.Timeout] must be zero when using custom RoundTripper wrappers: if the
// transport is not *http.Transport, net/http falls back to setRequestCancel's legacy
// timer goroutine, which can fatally error on Go 1.26+ ("select on synctest…").
func (c *Client) httpDo(req *http.Request) (*http.Response, error) {
	if req == nil {
		return nil, fmt.Errorf("nil request")
	}
	baseCtx := req.Context()
	sendCtx := baseCtx
	var cancel context.CancelFunc
	if _, hasDeadline := baseCtx.Deadline(); !hasDeadline {
		sendCtx, cancel = context.WithTimeout(baseCtx, DefaultHTTPTimeout)
	}
	reqSend := req.Clone(sendCtx)
	if reqSend == nil {
		if cancel != nil {
			cancel()
		}
		return nil, fmt.Errorf("http.Request.Clone returned nil")
	}
	resp, err := c.httpClient.Do(reqSend)
	if err != nil {
		if cancel != nil {
			cancel()
		}
		return nil, err
	}
	body := resp.Body
	if body == nil {
		// net/http usually sets a non-nil Body; guard so xml.NewDecoder never sees a nil Reader.
		body = io.NopCloser(strings.NewReader(""))
	}
	if cancel != nil {
		resp.Body = &bodyCancelCloser{ReadCloser: body, cancel: cancel}
	} else {
		resp.Body = body
	}
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
