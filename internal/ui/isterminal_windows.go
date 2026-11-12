//go:build windows

package ui

import (
	"os"
	"syscall"
)

// IsTerminal reports whether f is an interactive console. NUL is a character
// device too, so only a handle that has a console mode counts.
func IsTerminal(f *os.File) bool {
	if f == nil {
		return false
	}
	var mode uint32
	return syscall.GetConsoleMode(syscall.Handle(f.Fd()), &mode) == nil
}
