package ui

import (
	"strings"
	"unicode"
)

// Printable replaces control characters, the Unicode line and paragraph
// separators and the bidirectional embedding, override and isolate controls
// with U+FFFD, so text from the network cannot move the cursor, start an
// escape sequence, reorder what is shown or break a tab-separated table.
func Printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 || isBidiControl(r) {
			return 0xFFFD
		}
		return r
	}, s)
}

func isBidiControl(r rune) bool {
	return (r >= 0x202A && r <= 0x202E) || (r >= 0x2066 && r <= 0x2069)
}
