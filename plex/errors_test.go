package plex

import (
	"context"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"syscall"
	"testing"
)

func TestIsTransientPlexErr(t *testing.T) {
	t.Parallel()

	syn := &xml.SyntaxError{Line: 1}

	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"canceled", context.Canceled, false},
		{"wrapped cancel", fmt.Errorf("x: %w", context.Canceled), false},
		{"deadline", context.DeadlineExceeded, true},
		{"dns timeout", &net.DNSError{IsTimeout: true}, true},
		{"xml syntax", syn, true},
		{"wrapped xml", fmt.Errorf("decode: %w", syn), true},
		{"eof", io.EOF, true},
		{"unexpected eof", io.ErrUnexpectedEOF, true},
		{"conn reset", syscall.ECONNRESET, true},
		{"econnrefused", syscall.ECONNREFUSED, true},
		{"etimedout", syscall.ETIMEDOUT, true},
		{"opaque", errors.New("plex search API returned status 401"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := isTransientPlexErr(tt.err); got != tt.want {
				t.Fatalf("isTransientPlexErr(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}

func TestIsTransientPlexErr_URLErrorTimeout(t *testing.T) {
	t.Parallel()

	err := &url.Error{Op: "Get", URL: "http://plex.test/", Err: errTimeout{}}
	if !isTransientPlexErr(err) {
		t.Fatal("expected url.Error wrapping timeout to be transient")
	}
}

type errTimeout struct{}

func (errTimeout) Error() string   { return "timeout" }
func (errTimeout) Timeout() bool   { return true }
func (errTimeout) Temporary() bool { return true }
