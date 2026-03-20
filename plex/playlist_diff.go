package plex

import (
	"fmt"
	"io"
	"os"

	"github.com/grrywlsn/plexify/internal/cliutil"
	"golang.org/x/term"
)

// ANSI color codes for TTY diff output.
const (
	ansiReset  = "\033[0m"
	ansiRed    = "\033[31m"
	ansiGreen  = "\033[32m"
	ansiYellow = "\033[33m"
	ansiBold   = "\033[1m"
)

// WriterSupportsColor is true when w is an *os.File TTY and NO_COLOR is unset.
func WriterSupportsColor(w io.Writer) bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// StdoutSupportsColor is true when stdout is a terminal and NO_COLOR is unset.
func StdoutSupportsColor() bool {
	return WriterSupportsColor(os.Stdout)
}

// PlaylistDesiredEntry is one matched row in music-social order for the target Plex playlist.
type PlaylistDesiredEntry struct {
	MatchResult MatchResult // PlexTrack non-nil
}

// PlaylistDiffView bundles current and desired playlist rows plus diff ops for rendering.
type PlaylistDiffView struct {
	PlaylistTitle string
	Old           []PlexTrack
	Desired       []PlaylistDesiredEntry
	Ops           []PlaylistDiffOp
}

// NewPlaylistDiffView builds a view with Ops from Old and Desired.
func NewPlaylistDiffView(playlistTitle string, old []PlexTrack, desired []PlaylistDesiredEntry) PlaylistDiffView {
	return PlaylistDiffView{
		PlaylistTitle: playlistTitle,
		Old:           old,
		Desired:       desired,
		Ops:           BuildPlaylistDiffOps(old, desired),
	}
}

// PlaylistDiffKind classifies one line of playlist change output.
type PlaylistDiffKind int

const (
	PlaylistDiffAdd PlaylistDiffKind = iota
	PlaylistDiffRemove
	PlaylistDiffChange
)

// PlaylistDiffOp describes a single add, remove, or substitution for display.
type PlaylistDiffOp struct {
	Kind   PlaylistDiffKind
	OldIdx int // index into old []PlexTrack, or -1
	NewIdx int // index into desired []PlaylistDesiredEntry, or -1
}

type rawDiffKind int

const (
	rawEqual rawDiffKind = iota
	rawDelete
	rawInsert
)

type rawDiffItem struct {
	kind   rawDiffKind
	oldIdx int
	newIdx int
}

// BuildPlaylistDiffOps compares current Plex playlist track keys with desired keys (LCS / Myers-style backtrack).
func BuildPlaylistDiffOps(old []PlexTrack, desired []PlaylistDesiredEntry) []PlaylistDiffOp {
	oldKeys := make([]string, len(old))
	for i := range old {
		oldKeys[i] = old[i].ID
	}
	newKeys := make([]string, len(desired))
	for i := range desired {
		newKeys[i] = desired[i].MatchResult.PlexTrack.ID
	}

	raw := lcsRawDiff(oldKeys, newKeys)
	return coalesceRawToDisplay(raw)
}

func lcsRawDiff(oldKeys, newKeys []string) []rawDiffItem {
	n, m := len(oldKeys), len(newKeys)
	if n == 0 && m == 0 {
		return nil
	}

	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := 1; i <= n; i++ {
		for j := 1; j <= m; j++ {
			if oldKeys[i-1] == newKeys[j-1] {
				dp[i][j] = dp[i-1][j-1] + 1
			} else {
				dp[i][j] = max(dp[i-1][j], dp[i][j-1])
			}
		}
	}

	var ops []rawDiffItem
	i, j := n, m
	for i > 0 || j > 0 {
		if i > 0 && j > 0 && oldKeys[i-1] == newKeys[j-1] {
			ops = append(ops, rawDiffItem{rawEqual, i - 1, j - 1})
			i--
			j--
		} else if i > 0 && (j == 0 || dp[i-1][j] >= dp[i][j-1]) {
			ops = append(ops, rawDiffItem{rawDelete, i - 1, -1})
			i--
		} else {
			ops = append(ops, rawDiffItem{rawInsert, -1, j - 1})
			j--
		}
	}

	for l, r := 0, len(ops)-1; l < r; l, r = l+1, r-1 {
		ops[l], ops[r] = ops[r], ops[l]
	}
	return ops
}

func coalesceRawToDisplay(raw []rawDiffItem) []PlaylistDiffOp {
	var out []PlaylistDiffOp
	for i := 0; i < len(raw); i++ {
		switch raw[i].kind {
		case rawEqual:
			continue
		case rawDelete:
			if i+1 < len(raw) && raw[i+1].kind == rawInsert {
				out = append(out, PlaylistDiffOp{
					Kind:   PlaylistDiffChange,
					OldIdx: raw[i].oldIdx,
					NewIdx: raw[i+1].newIdx,
				})
				i++
			} else {
				out = append(out, PlaylistDiffOp{
					Kind:   PlaylistDiffRemove,
					OldIdx: raw[i].oldIdx,
					NewIdx: -1,
				})
			}
		case rawInsert:
			if i+1 < len(raw) && raw[i+1].kind == rawDelete {
				// LCS backtrack often emits insert-before-delete for a substitution
				out = append(out, PlaylistDiffOp{
					Kind:   PlaylistDiffChange,
					OldIdx: raw[i+1].oldIdx,
					NewIdx: raw[i].newIdx,
				})
				i++
			} else {
				out = append(out, PlaylistDiffOp{
					Kind:   PlaylistDiffAdd,
					OldIdx: -1,
					NewIdx: raw[i].newIdx,
				})
			}
		}
	}
	return out
}

// FprintPlaylistDiff writes a git-style colored summary with full-width section borders (standalone block).
func FprintPlaylistDiff(w io.Writer, view PlaylistDiffView, useColor bool) {
	fprintPlaylistDiff(w, view, useColor, true)
}

// FprintPlaylistDiffEmbedded writes the same lines under a subsection header (for use inside SUMMARY).
func FprintPlaylistDiffEmbedded(w io.Writer, view PlaylistDiffView, useColor bool) {
	fprintPlaylistDiff(w, view, useColor, false)
}

func fprintPlaylistDiff(w io.Writer, view PlaylistDiffView, useColor, outerBanner bool) {
	eq := cliutil.RepeatChar("=", cliutil.SectionWidth)
	fmt.Fprintln(w, "")
	if outerBanner {
		fmt.Fprintln(w, eq)
		fmt.Fprintf(w, "PLAYLIST CHANGES — %s\n", view.PlaylistTitle)
		fmt.Fprintln(w, eq)
	} else {
		fmt.Fprintf(w, "PLAYLIST CHANGES — %s\n", view.PlaylistTitle)
		fmt.Fprintln(w, cliutil.RepeatChar("-", cliutil.SectionWidth))
	}

	if len(view.Ops) == 0 {
		fmt.Fprintln(w, "(no changes — playlist already matches desired track list and order)")
		fmt.Fprintln(w, "")
		return
	}

	printLine := func(color, text string) {
		if useColor {
			fmt.Fprintf(w, "%s%s%s\n", color, text, ansiReset)
		} else {
			fmt.Fprintln(w, text)
		}
	}

	for _, op := range view.Ops {
		switch op.Kind {
		case PlaylistDiffAdd:
			ent := view.Desired[op.NewIdx]
			src := ent.MatchResult.SourceTrack
			pt := ent.MatchResult.PlexTrack
			printLine(ansiGreen+ansiBold, "+ music-social: "+formatArtistTitle(src.Artist, src.Name))
			printLine(ansiGreen, "+ plex:         "+formatArtistTitle(pt.Artist, pt.Title)+fmt.Sprintf(" (confidence: %s)", formatConfidencePercent(ent.MatchResult.Confidence)))
		case PlaylistDiffRemove:
			pt := &view.Old[op.OldIdx]
			printLine(ansiRed+ansiBold, "- plex:         "+formatArtistTitle(pt.Artist, pt.Title))
		case PlaylistDiffChange:
			oldTr := &view.Old[op.OldIdx]
			ent := view.Desired[op.NewIdx]
			src := ent.MatchResult.SourceTrack
			newPt := ent.MatchResult.PlexTrack
			printLine(ansiYellow+ansiBold, "~ was (plex):    "+formatArtistTitle(oldTr.Artist, oldTr.Title))
			printLine(ansiYellow, "~ now (source): "+formatArtistTitle(src.Artist, src.Name))
			printLine(ansiYellow, "~ now (plex):   "+formatArtistTitle(newPt.Artist, newPt.Title)+fmt.Sprintf(" (confidence: %s)", formatConfidencePercent(ent.MatchResult.Confidence)))
		}
		fmt.Fprintln(w, "")
	}
}

func formatArtistTitle(artist, title string) string {
	return fmt.Sprintf("%s - %s", artist, title)
}
