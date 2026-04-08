package plex

import (
	"encoding/xml"
	"io"
	"net/http"
	"strings"
	"testing"
)

const sampleSearchXML = `<?xml version="1.0" encoding="UTF-8"?>` + "\n" +
	`<MediaContainer size="1">` +
	`<Track ratingKey="42" title="Vertigo" grandparentTitle="Jai Wolf" parentTitle="Album"/>` +
	`</MediaContainer>`

func TestDecodePlexResponseXML_ValidSearchPayload(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(sampleSearchXML)),
	}

	var got PlexResponse
	if err := decodePlexResponseXML(resp, &got); err != nil {
		t.Fatal(err)
	}

	if len(got.Tracks) != 1 {
		t.Fatalf("tracks: got %d, want 1", len(got.Tracks))
	}
	if got.Tracks[0].ID != "42" || got.Tracks[0].Title != "Vertigo" {
		t.Fatalf("unexpected track: %#v", got.Tracks[0])
	}
	if got.Tracks[0].Artist != "Jai Wolf" {
		t.Fatalf("artist: got %q", got.Tracks[0].Artist)
	}
}

func TestDecodePlexResponseXML_ValidServerInfo(t *testing.T) {
	t.Parallel()

	const xmlDoc = `<MediaContainer friendlyName="Home" machineIdentifier="abc" version="1.0"/>`
	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader(xmlDoc)),
	}

	var got PlexServerInfo
	if err := decodePlexResponseXML(resp, &got); err != nil {
		t.Fatal(err)
	}
	if got.FriendlyName != "Home" || got.MachineIdentifier != "abc" {
		t.Fatalf("unexpected decode: %+v", got)
	}
}

func TestDecodePlexResponseXML_InvalidXML(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("<MediaContainer><unclosed>")),
	}

	var got PlexResponse
	err := decodePlexResponseXML(resp, &got)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestDecodePlexResponseXML_EmptyBody(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(strings.NewReader("")),
	}

	var got PlexResponse
	err := decodePlexResponseXML(resp, &got)
	if err == nil {
		t.Fatal("expected error unmarshaling empty document")
	}
	if _, ok := err.(*xml.SyntaxError); !ok {
		// encoding/xml may return SyntaxError or wrapped — accept non-nil
		if !strings.Contains(err.Error(), "EOF") && !strings.Contains(err.Error(), "syntax") {
			t.Fatalf("unexpected error type: %T %v", err, err)
		}
	}
}

func TestDecodePlexResponseXML_NilResponse(t *testing.T) {
	t.Parallel()

	var got PlexResponse
	err := decodePlexResponseXML(nil, &got)
	if err == nil || !strings.Contains(err.Error(), "nil response") {
		t.Fatalf("expected nil response error, got %v", err)
	}
}

func TestDecodePlexResponseXML_NilBody(t *testing.T) {
	t.Parallel()

	resp := &http.Response{StatusCode: http.StatusOK, Body: nil}
	var got PlexResponse
	err := decodePlexResponseXML(resp, &got)
	if err == nil || !strings.Contains(err.Error(), "nil response body") {
		t.Fatalf("expected nil body error, got %v", err)
	}
}

func TestDecodePlexResponseXML_ReadError(t *testing.T) {
	t.Parallel()

	resp := &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(errReader{io.ErrUnexpectedEOF}),
	}

	var got PlexResponse
	err := decodePlexResponseXML(resp, &got)
	if err == nil || err != io.ErrUnexpectedEOF {
		t.Fatalf("got %v, want %v", err, io.ErrUnexpectedEOF)
	}
}

type errReader struct{ err error }

func (e errReader) Read(p []byte) (int, error) {
	return 0, e.err
}
