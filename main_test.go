package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"strings"
	"testing"
)

const subprocessEnv = "C3D_UPLOAD_TEST_SUBPROCESS"

func TestMain(m *testing.M) {
	if os.Getenv(subprocessEnv) == "1" {
		main()
		os.Exit(0)
	}
	os.Exit(m.Run())
}

type cliResult struct {
	stdout   string
	stderr   string
	exitCode int
}

func runCLI(t *testing.T, args ...string) cliResult {
	t.Helper()
	cmd := exec.Command(os.Args[0], args...)
	cmd.Env = append(os.Environ(), subprocessEnv+"=1")
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	code := 0
	if err := cmd.Run(); err != nil {
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) {
			t.Fatalf("running %v: %v", args, err)
		}
		code = exitErr.ExitCode()
	}
	return cliResult{stdout: stdout.String(), stderr: stderr.String(), exitCode: code}
}

func TestVersionPrintsSemverAndExitsZero(t *testing.T) {
	res := runCLI(t, "version")
	if res.exitCode != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr %q)", res.exitCode, res.stderr)
	}
	if want := "cognitive3d-cli-upload 0.0.0-dev\n"; res.stdout != want {
		t.Errorf("stdout = %q, want %q", res.stdout, want)
	}

	res = runCLI(t, "version", "--format", "json")
	if res.exitCode != 0 {
		t.Fatalf("json exit code = %d, want 0 (stderr %q)", res.exitCode, res.stderr)
	}
	var got map[string]string
	if err := json.Unmarshal([]byte(res.stdout), &got); err != nil {
		t.Fatalf("stdout is not JSON: %v: %q", err, res.stdout)
	}
	if got["version"] != "0.0.0-dev" {
		t.Errorf("version = %q, want 0.0.0-dev", got["version"])
	}
	for _, key := range []string{"commit", "date"} {
		if _, ok := got[key]; !ok {
			t.Errorf("JSON is missing %q: %q", key, res.stdout)
		}
	}
}

func TestUnknownCommandAndFlagExitFourWithOneLine(t *testing.T) {
	for _, args := range [][]string{{"nope"}, {"version", "--nope"}, {"version", "extra"}, {"--format", "yaml", "version"}} {
		res := runCLI(t, args...)
		if res.exitCode != 4 {
			t.Errorf("%v: exit code = %d, want 4", args, res.exitCode)
		}
		if res.stdout != "" {
			t.Errorf("%v: stdout = %q, want empty", args, res.stdout)
		}
		if n := len(strings.Split(strings.TrimRight(res.stderr, "\n"), "\n")); n != 1 {
			t.Errorf("%v: stderr has %d lines, want 1: %q", args, n, res.stderr)
		}
	}
}
