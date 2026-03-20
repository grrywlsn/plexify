package cliutil

import "strings"

// SectionWidth is the default width for banner lines (matches historical plexify CLI output).
const SectionWidth = 80

// RepeatChar repeats r width times (typically "=" or "-").
func RepeatChar(r string, width int) string {
	if width <= 0 {
		return ""
	}
	return strings.Repeat(r, width)
}
