package model

import (
	"strings"
	"testing"
)

func TestSafeSegment(t *testing.T) {
	ok := []string{"abc", "scene 1", "a.b", "..a", "a..", ".hidden", "é", strings.Repeat("a", 255), "8f966d6a-a9d5-4010-9620-e1cfe823998c"}
	for _, s := range ok {
		if err := SafeSegment(s); err != nil {
			t.Errorf("SafeSegment(%q) = %v, want nil", s, err)
		}
	}
	bad := []string{"", ".", "..", "a/b", "/a", "a/", `a\b`, "a\x00b", "a\nb", "a\tb", "a\x1fb", "a\x7fb", strings.Repeat("a", 256), strings.Repeat("é", 128)}
	for _, s := range bad {
		if err := SafeSegment(s); err == nil {
			t.Errorf("SafeSegment(%q) = nil, want error", s)
		}
	}
}
