package lidarr

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"syscall"
	"time"
)

const defaultWriteMaxAttempts = 6

var (
	writeMaxAttempts = defaultWriteMaxAttempts
	writeBackoffFunc = defaultWriteBackoff
)

func defaultWriteBackoff(attempt int) time.Duration {
	delay := 5 * time.Second
	for i := 0; i < attempt; i++ {
		delay *= 2
		if delay > 60*time.Second {
			return 60 * time.Second
		}
	}
	return delay
}

// WriteError is returned when Lidarr could not persist changes after retries (e.g. read-only database).
type WriteError struct {
	ReleaseGroupID string
	Reason         string
}

func (e *WriteError) Error() string {
	if e == nil {
		return ""
	}
	if e.Reason != "" {
		return fmt.Sprintf("could not write to Lidarr for release group %s: %s", e.ReleaseGroupID, e.Reason)
	}
	return fmt.Sprintf("could not write to Lidarr for release group %s", e.ReleaseGroupID)
}

// UserMessage is a short CLI-friendly message that prompts the user to re-run Plexify.
func (e *WriteError) UserMessage() string {
	if e == nil {
		return ""
	}
	if e.Reason != "" {
		return fmt.Sprintf("Lidarr could not save changes for release group %s (%s). Once Lidarr is healthy, re-run Plexify to retry.", e.ReleaseGroupID, e.Reason)
	}
	return fmt.Sprintf("Lidarr could not save changes for release group %s. Once Lidarr is healthy, re-run Plexify to retry.", e.ReleaseGroupID)
}

// AsWriteError reports whether err is or wraps a *WriteError.
func AsWriteError(err error) (*WriteError, bool) {
	var we *WriteError
	if errors.As(err, &we) && we != nil {
		return we, true
	}
	return nil, false
}

func isTransientLidarrErr(err error) bool {
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

	msg := strings.ToLower(err.Error())
	if strings.Contains(msg, "readonly database") {
		return true
	}
	for _, code := range []int{
		http.StatusRequestTimeout,
		http.StatusTooManyRequests,
		http.StatusInternalServerError,
		http.StatusBadGateway,
		http.StatusServiceUnavailable,
		http.StatusGatewayTimeout,
	} {
		if strings.Contains(msg, fmt.Sprintf(": %d ", code)) {
			return true
		}
	}
	return false
}

func lidarrFailureReason(err error) string {
	if err == nil {
		return ""
	}
	msg := err.Error()
	lower := strings.ToLower(msg)
	switch {
	case strings.Contains(lower, "readonly database"):
		return "database read-only"
	case strings.Contains(lower, "connection refused"):
		return "connection refused"
	case strings.Contains(lower, "timeout"):
		return "timeout"
	case strings.Contains(lower, "internal server error"):
		return "server error"
	default:
		if i := strings.Index(msg, ": "); i >= 0 && len(msg) > i+2 {
			rest := strings.TrimSpace(msg[i+2:])
			if j := strings.Index(rest, "\n"); j >= 0 {
				rest = strings.TrimSpace(rest[:j])
			}
			if len(rest) > 120 {
				rest = rest[:120] + "…"
			}
			if rest != "" {
				return rest
			}
		}
		return "unavailable"
	}
}
