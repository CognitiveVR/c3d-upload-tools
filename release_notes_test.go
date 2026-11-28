package main

import (
	"bytes"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

const releaseNotesChangelog = `# Changelog

Intro text that belongs to no version.

## [Unreleased]

- An unreleased change that wraps
  onto a second line.

## [2.1.0] - 2027-01-15

### Added

- A first entry that wraps
  onto a second line and
  a third. See the [guide](docs/guide.md#usage).
- An entry with an [absolute link](https://example.com/page) and an [anchor](#section).
  - A nested entry stays on its own line.

` + "```bash\ncommand --flag\n  indented output\n```" + `

### Fixed

- A fix.

## [2.0.10] - 2027-01-02

- The 2.0.10 entry.

## [2.0.1] - 2026-12-01

## [2.0.0] - 2026-11-27

- The 2.0.0 entry.
`

// runReleaseNotes runs scripts/release-notes.sh against a changelog written to
// a temporary directory and returns stdout, stderr and the exit code.
func runReleaseNotes(t *testing.T, changelog string, args ...string) (string, string, int) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("the release job runs the script on macOS; bash is not assumed on Windows")
	}
	bash, err := exec.LookPath("bash")
	if err != nil {
		t.Skip("bash not found")
	}
	script, err := filepath.Abs(filepath.Join("scripts", "release-notes.sh"))
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "CHANGELOG.md")
	if err := os.WriteFile(path, []byte(changelog), 0o600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(bash, append([]string{script}, append(args, path)...)...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("running the script: %v", err)
		}
		code = exitErr.ExitCode()
	}
	return stdout.String(), stderr.String(), code
}

func TestReleaseNotesPrintsTheSectionForTheVersion(t *testing.T) {
	want := `### Added

- A first entry that wraps onto a second line and a third. See the [guide](https://github.com/CognitiveVR/c3d-upload-tools/blob/v2.1.0/docs/guide.md#usage).
- An entry with an [absolute link](https://example.com/page) and an [anchor](#section).
  - A nested entry stays on its own line.

` + "```bash\ncommand --flag\n  indented output\n```" + `

### Fixed

- A fix.
`
	for _, version := range []string{"2.1.0", "v2.1.0"} {
		stdout, stderr, code := runReleaseNotes(t, releaseNotesChangelog, version)
		if code != 0 {
			t.Fatalf("%s: exit code %d, stderr %q", version, code, stderr)
		}
		if stdout != want {
			t.Errorf("%s: notes differ\n got: %q\nwant: %q", version, stdout, want)
		}
	}
}

func TestReleaseNotesMatchesTheWholeVersion(t *testing.T) {
	stdout, stderr, code := runReleaseNotes(t, releaseNotesChangelog, "2.0.10")
	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if stdout != "- The 2.0.10 entry.\n" {
		t.Errorf("got %q", stdout)
	}
}

func TestReleaseNotesPrereleaseUsesUnreleased(t *testing.T) {
	stdout, stderr, code := runReleaseNotes(t, releaseNotesChangelog, "2.2.0-rc.1")
	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if stdout != "- An unreleased change that wraps onto a second line.\n" {
		t.Errorf("got %q", stdout)
	}
}

func TestReleaseNotesFailsWithoutNotes(t *testing.T) {
	cases := []struct {
		name      string
		changelog string
		version   string
		message   string
	}{
		{"missing section", releaseNotesChangelog, "3.0.0", "## [3.0.0]"},
		{"empty section", releaseNotesChangelog, "2.0.1", "## [2.0.1]"},
		{"prerelease with nothing unreleased", "# Changelog\n\n## [Unreleased]\n\n## [2.0.0] - 2026-11-27\n\n- An entry.\n", "2.1.0-rc.1", "## [Unreleased]"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runReleaseNotes(t, tc.changelog, tc.version)
			if code == 0 {
				t.Fatalf("exit code 0, stdout %q", stdout)
			}
			if stdout != "" {
				t.Errorf("stdout should be empty, got %q", stdout)
			}
			if !strings.Contains(stderr, tc.message) {
				t.Errorf("stderr %q does not name %q", stderr, tc.message)
			}
		})
	}
}

func TestReleaseNotesLatestIsTheNewestDatedSection(t *testing.T) {
	stdout, stderr, code := runReleaseNotes(t, releaseNotesChangelog, "latest")
	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "### Added\n") {
		t.Errorf("latest should print the 2.1.0 section, got %q", stdout)
	}
}

// The repository's own changelog must yield notes, or the next release fails.
func TestReleaseNotesReadsTheRepositoryChangelog(t *testing.T) {
	changelog, err := os.ReadFile("CHANGELOG.md")
	if err != nil {
		t.Fatal(err)
	}
	stdout, stderr, code := runReleaseNotes(t, string(changelog), "latest")
	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if strings.TrimSpace(stdout) == "" {
		t.Fatal("no notes for the newest version")
	}
	for _, line := range strings.Split(stdout, "\n") {
		if text := strings.TrimLeft(line, " "); text != line && !strings.HasPrefix(text, "- ") {
			t.Errorf("a wrapped line survived: %q", line)
		}
	}
}
