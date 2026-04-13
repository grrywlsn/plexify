package plex

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
)

const maxPlexHTTPErrorBody = 2048

// PlexHTTPError is returned when the Plex HTTP API responds with an unexpected status code.
// Transient statuses (5xx, 408, 429) are recognized by [isTransientPlexErr] for retries and soft-degrades.
type PlexHTTPError struct {
	StatusCode int
	Op         string
	Body       string
}

func newPlexHTTPError(status int, op string, body []byte) *PlexHTTPError {
	s := string(body)
	if len(s) > maxPlexHTTPErrorBody {
		s = s[:maxPlexHTTPErrorBody] + "…"
	}
	s = strings.ReplaceAll(s, "\x00", "")
	return &PlexHTTPError{StatusCode: status, Op: op, Body: strings.TrimSpace(s)}
}

func (e *PlexHTTPError) Error() string {
	if e == nil {
		return ""
	}
	if e.Body != "" {
		return fmt.Sprintf("plex %s: HTTP %d: %s", e.Op, e.StatusCode, e.Body)
	}
	return fmt.Sprintf("plex %s: HTTP %d", e.Op, e.StatusCode)
}

// transientHTTPStatus reports status codes that often succeed on retry (overloaded Plex, proxies, K8s paths).
func transientHTTPStatus(code int) bool {
	switch code {
	case http.StatusRequestTimeout, http.StatusTooManyRequests:
		return true
	default:
		return code >= 500 && code <= 599
	}
}

// isTransientPlexErr reports unknown or transport-level failures that are worth retrying
// or skipping to the next search strategy, rather than aborting the whole track match.
//
// Context cancellation from the caller is not transient: check ctx.Err() before using this.
func isTransientPlexErr(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) {
		return false
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return true
	}
	var netErr net.Error
	if errors.As(err, &netErr) && netErr.Timeout() {
		return true
	}
	var uErr *url.Error
	if errors.As(err, &uErr) && uErr.Err != nil {
		if ne, ok := uErr.Err.(net.Error); ok && ne.Timeout() {
			return true
		}
	}
	var syn *xml.SyntaxError
	if errors.As(err, &syn) {
		return true
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		return true
	}
	if errors.Is(err, syscall.ECONNRESET) || errors.Is(err, syscall.ECONNREFUSED) ||
		errors.Is(err, syscall.ETIMEDOUT) {
		return true
	}
	if errors.Is(err, net.ErrClosed) {
		return true
	}
	var pe *PlexHTTPError
	if errors.As(err, &pe) && pe != nil && transientHTTPStatus(pe.StatusCode) {
		return true
	}
	return false
}
