package plex

import (
	"strings"
	"testing"

	"github.com/grrywlsn/plexify/internal/cliutil"
)

func TestFprintPlaylistDiffEmbedded_usesHyphenRule(t *testing.T) {
	var b strings.Builder
	old := []PlexTrack{{ID: "1", Artist: "O", Title: "Old"}}
	desired := []PlaylistDesiredEntry{
		desiredEntry("s", "src", "2", "N", "New", 0.75),
	}
	view := NewPlaylistDiffView("Test PL", old, desired)
	FprintPlaylistDiffEmbedded(&b, view, false)
	out := b.String()
	if !strings.Contains(out, "PLAYLIST CHANGES — Test PL") {
		t.Fatal("missing header", out)
	}
	if !strings.Contains(out, cliutil.RepeatChar("-", cliutil.SectionWidth)) {
		t.Fatal("expected full-width hyphen underline", out)
	}
	if strings.Contains(out, "\033[") {
		t.Fatal("should not contain ANSI when useColor=false")
	}
}
