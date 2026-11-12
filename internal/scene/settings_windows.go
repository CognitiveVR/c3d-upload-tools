//go:build windows

package scene

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// checkExistingSettingsWritable refuses a settings.json that does not open
// for writing, because the rename in WriteSettings cannot replace a read-only
// file on Windows.
func checkExistingSettingsWritable(path string) error {
	f, err := os.OpenFile(path, os.O_WRONLY, 0)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("%s is not writable, so the upload was not started: %w", path, err)
	}
	return f.Close()
}
