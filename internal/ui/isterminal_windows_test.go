//go:build windows

package ui

import (
	"os"
	"testing"
)

func TestIsTerminalWindowsPipeAndNUL(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	defer w.Close()
	if IsTerminal(r) || IsTerminal(w) {
		t.Error("pipe")
	}
	null, err := os.OpenFile("NUL", os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer null.Close()
	if IsTerminal(null) {
		t.Error("NUL")
	}
}
