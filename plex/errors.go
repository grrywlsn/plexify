package plex

import (
	"context"
	"encoding/xml"
	"errors"
	"io"
	"net"
	"net/url"
	"syscall"
)

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
	return false
}
