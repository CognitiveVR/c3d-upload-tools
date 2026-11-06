package model

import (
	"fmt"
	"strings"
)

const maxSegmentBytes = 255

// SafeSegment reports whether s can be used as a single URL path segment or
// file name without escaping out of its directory. It does not percent-encode;
// callers pass the segment through url.PathEscape.
func SafeSegment(s string) error {
	switch {
	case s == "":
		return fmt.Errorf("path segment is empty")
	case s == "." || s == "..":
		return fmt.Errorf("path segment %q is not allowed", s)
	case len(s) > maxSegmentBytes:
		return fmt.Errorf("path segment is %d bytes long; the maximum is %d", len(s), maxSegmentBytes)
	case strings.ContainsAny(s, `/\`):
		return fmt.Errorf("path segment %q contains a path separator", s)
	}
	for i := 0; i < len(s); i++ {
		if s[i] < 0x20 || s[i] == 0x7f {
			return fmt.Errorf("path segment %q contains a control character", s)
		}
	}
	return nil
}
