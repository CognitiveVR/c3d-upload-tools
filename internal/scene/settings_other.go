//go:build !windows

package scene

// checkExistingSettingsWritable accepts any existing settings.json: the
// rename in WriteSettings replaces even a read-only file once the directory
// is writable.
func checkExistingSettingsWritable(string) error { return nil }
