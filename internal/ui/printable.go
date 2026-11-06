package ui

import (
	"strings"
	"unicode"
)

// Printable replaces control characters and the Unicode line and paragraph
// separators with U+FFFD, so text from the network cannot move the cursor,
// start an escape sequence or break a tab-separated table.
func Printable(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == 0x2028 || r == 0x2029 {
			return 0xFFFD
		}
		return r
	}, s)
}
