package version

import "testing"

func TestSDKVersion(t *testing.T) {
	if got, want := SDKVersion(), "cli-go-v0.0.0-dev"; got != want {
		t.Errorf("default SDKVersion = %q, want %q", got, want)
	}

	original := Version
	t.Cleanup(func() { Version = original })
	Version = "1.2.0"
	if got, want := SDKVersion(), "cli-go-v1.2.0"; got != want {
		t.Errorf("SDKVersion = %q, want %q", got, want)
	}
}
