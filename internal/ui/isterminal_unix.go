//go:build !windows

package ui

import "os"

// IsTerminal reports whether f is an interactive terminal: a character
// device that is not the null device.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	info, err := f.Stat()
	if err != nil || info.Mode()&os.ModeCharDevice == 0 {
		return false
	}
	null, err := os.Open(os.DevNull)
	if err != nil {
		return true
	}
	defer func() { _ = null.Close() }()
	nullInfo, err := null.Stat()
	return err != nil || !os.SameFile(info, nullInfo)
}
