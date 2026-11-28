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

func TestReleaseNotesReflowKeepsMarkdownStructure(t *testing.T) {
	const fence = "```"
	cases := []struct {
		name string
		in   string
		want string // empty means the section must come out unchanged
		crlf bool
	}{
		{
			name: "fenced code under a list item",
			in: "- Upload a scene with:\n\n  " + fence + "bash\n  upload \\\n    --scene-id abc\n  " + fence + "\n\n" +
				"- Another entry:\n  " + fence + "json\n  {\"a\": 1}\n  " + fence + "\n",
		},
		{
			name: "tilde fence",
			in:   "~~~text\nplain\n  indented\n~~~\n",
		},
		{
			name: "text after a closing fence",
			in:   "- item\n  " + fence + "\n  code\n  " + fence + "\n  trailing text\n",
		},
		{
			name: "version heading inside a fence",
			in:   "- before\n\n" + fence + "text\n## [0.5.0] in an example\n" + fence + "\n\n- after\n",
		},
		{
			name: "indented line after a subsection heading",
			in:   "### Added\n  stray text\n\n- item\n",
		},
		{
			name: "nested list markers",
			in:   "- item\n  * star\n  + plus\n  1. one\n  2) two\n",
		},
		{
			name: "block quote and table rows under a list item",
			in:   "- item\n  > quoted\n- next\n  | a | b |\n  | - | - |\n",
		},
		{
			name: "continuation that starts like a marker",
			in:   "- pass\n  --upload-manifest=false or\n  *never* or\n  2026 to skip it\n",
			want: "- pass --upload-manifest=false or *never* or 2026 to skip it\n",
		},
		{
			name: "tab-indented continuation",
			in:   "- item\n\tcontinued\n",
			want: "- item continued\n",
		},
		{
			name: "CRLF line endings",
			in:   "- a wrapped\n  line\n",
			want: "- a wrapped line\n",
			crlf: true,
		},
		{
			name: "image and link targets",
			in:   "- See ![shot](docs/shot.png) and the [guide](docs/a.md).\n",
			want: "- See ![shot](https://github.com/CognitiveVR/c3d-upload-tools/raw/v1.0.0/docs/shot.png)" +
				" and the [guide](https://github.com/CognitiveVR/c3d-upload-tools/blob/v1.0.0/docs/a.md).\n",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			changelog := "# Changelog\n\n## [1.0.0] - 2027-01-01\n\n" + tc.in + "\n## [0.9.0] - 2026-01-01\n\n- old\n"
			if tc.crlf {
				changelog = strings.ReplaceAll(changelog, "\n", "\r\n")
			}
			want := tc.want
			if want == "" {
				want = tc.in
			}
			stdout, stderr, code := runReleaseNotes(t, changelog, "1.0.0")
			if code != 0 {
				t.Fatalf("exit code %d, stderr %q", code, stderr)
			}
			if stdout != want {
				t.Errorf("notes differ\n got: %q\nwant: %q", stdout, want)
			}
		})
	}
}

func TestReleaseNotesPrereleaseSections(t *testing.T) {
	cases := []struct {
		name      string
		changelog string
		want      string
		section   string
	}{
		{
			name:      "its own section",
			changelog: "## [Unreleased]\n\n- unreleased\n\n## [2.1.0-rc.1] - 2027-01-10\n\n- rc entry\n\n## [2.1.0] - 2027-01-15\n\n- final entry\n",
			want:      "- rc entry\n",
			section:   "## [2.1.0-rc.1]",
		},
		{
			name:      "the version it leads to, when its own section has only headings",
			changelog: "## [Unreleased]\n\n## [2.1.0-rc.1] - 2027-01-10\n\n### Added\n\n## [2.1.0] - 2027-01-15\n\n- final entry\n",
			want:      "- final entry\n",
			section:   "## [2.1.0]",
		},
		{
			name:      "Unreleased, when the version has no section yet",
			changelog: "## [Unreleased]\n\n- unreleased\n\n## [2.0.0] - 2026-11-27\n\n- old\n",
			want:      "- unreleased\n",
			section:   "## [Unreleased]",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			stdout, stderr, code := runReleaseNotes(t, tc.changelog, "v2.1.0-rc.1")
			if code != 0 {
				t.Fatalf("exit code %d, stderr %q", code, stderr)
			}
			if stdout != tc.want {
				t.Errorf("got %q, want %q", stdout, tc.want)
			}
			if !strings.Contains(stderr, tc.section) {
				t.Errorf("stderr %q does not name the section used, %q", stderr, tc.section)
			}
		})
	}
}

func TestReleaseNotesFailsWithoutEntries(t *testing.T) {
	cases := []struct {
		name      string
		changelog string
		version   string
		messages  []string
	}{
		{"missing section", releaseNotesChangelog, "3.0.0", []string{"## [3.0.0]"}},
		{"empty section", releaseNotesChangelog, "2.0.1", []string{"## [2.0.1]"}},
		{
			"section with only headings",
			"## [2.2.0] - 2027-02-01\n\n### Added\n\n### Fixed\n\n## [2.1.0] - 2027-01-15\n\n- An entry.\n",
			"2.2.0",
			[]string{"## [2.2.0]"},
		},
		{
			"prerelease with no entries anywhere it looks",
			"# Changelog\n\n## [Unreleased]\n\n### Added\n\n## [2.0.0] - 2026-11-27\n\n- An entry.\n",
			"2.1.0-rc.1",
			[]string{"## [2.1.0-rc.1]", "## [2.1.0]", "## [Unreleased]"},
		},
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
			for _, message := range tc.messages {
				if !strings.Contains(stderr, message) {
					t.Errorf("stderr %q does not name %q", stderr, message)
				}
			}
		})
	}
}

// The Release workflow asks for the Unreleased section by name to warn about
// entries a stable tag would leave out.
func TestReleaseNotesUnreleasedByName(t *testing.T) {
	if _, stderr, code := runReleaseNotes(t, releaseNotesChangelog, "Unreleased"); code != 0 {
		t.Errorf("entries under Unreleased: exit code %d, stderr %q", code, stderr)
	}
	empty := "## [Unreleased]\n\n### Added\n\n## [2.0.0] - 2026-11-27\n\n- An entry.\n"
	if stdout, _, code := runReleaseNotes(t, empty, "Unreleased"); code == 0 {
		t.Errorf("nothing under Unreleased: exit code 0, stdout %q", stdout)
	}
}

func TestReleaseNotesLatestIsTheNewestVersionedSection(t *testing.T) {
	stdout, stderr, code := runReleaseNotes(t, releaseNotesChangelog, "latest")
	if code != 0 {
		t.Fatalf("exit code %d, stderr %q", code, stderr)
	}
	if !strings.HasPrefix(stdout, "### Added\n") {
		t.Errorf("latest should print the 2.1.0 section, got %q", stdout)
	}
	if !strings.Contains(stderr, "## [2.1.0]") {
		t.Errorf("stderr %q does not name the section used", stderr)
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
	if !strings.Contains(stdout, "\n- ") && !strings.HasPrefix(stdout, "- ") {
		t.Errorf("no list entries in the notes for the newest version:\n%s", stdout)
	}
}
