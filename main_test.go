package main

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
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
	return runCLIWith(t, "", nil, args...)
}

// isolatedEnvironment blanks every C3D_* variable and points HOME at an empty
// directory, so no real configuration is read.
func isolatedEnvironment(t *testing.T) []string {
	t.Helper()
	home := t.TempDir()
	env := []string{subprocessEnv + "=1", "HOME=" + home, "USERPROFILE=" + home, "NO_COLOR=1"}
	for _, entry := range os.Environ() {
		if name, _, _ := strings.Cut(entry, "="); strings.HasPrefix(name, "C3D_") && name != subprocessEnv {
			env = append(env, name+"=")
		}
	}
	return env
}

// runCLIWith runs the CLI in dir (a fresh temporary directory when empty) with
// an isolated environment plus extra, whose entries win.
func runCLIWith(t *testing.T, dir string, extra []string, args ...string) cliResult {
	t.Helper()
	if dir == "" {
		dir = t.TempDir()
	}
	cmd := exec.Command(os.Args[0], args...)
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), isolatedEnvironment(t)...), extra...)
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
	if want := "cognitive3d-cli-upload 0.0.0-dev (commit unknown, built unknown)\n"; res.stdout != want {
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

func authEnvironment(t *testing.T, fake *fakegateway.Server, key string) (string, []string) {
	t.Helper()
	home := t.TempDir()
	env := []string{
		"HOME=" + home, "USERPROFILE=" + home, "NO_COLOR=1",
		"C3D_UPLOAD_GATEWAY_URL=" + fake.URL(),
		"C3D_DEVELOPER_API_KEY=" + key,
		"C3D_SCENE_ID=", "C3D_ENV=", "C3D_DEFAULT_ENVIRONMENT=",
	}
	return t.TempDir(), env
}

const processSceneUUID = "0b1f6c3e-2a4d-4e8b-9c7a-5d3e1f2a4b6c"

func TestAuthVerifyAcrossTheProcessBoundary(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "p12-" + processSceneUUID, SDKFacingID: processSceneUUID, SceneName: "S"})

	t.Run("valid key", func(t *testing.T) {
		dir, env := authEnvironment(t, fake, fake.Key())
		res := runCLIWith(t, dir, env, "auth", "verify")
		want := "Environment: prod\nProject: Fake Project\nOrganization: Fake Organization\nKey expires: never\n"
		if res.exitCode != 0 || res.stdout != want {
			t.Errorf("exit %d, stdout %q, want 0 and %q (stderr %q)", res.exitCode, res.stdout, want, res.stderr)
		}
		if !strings.Contains(res.stderr, "[WARN]") || !strings.Contains(res.stderr, "not a Cognitive3D host") {
			t.Errorf("stderr should warn about the foreign host: %q", res.stderr)
		}
	})

	t.Run("expired key", func(t *testing.T) {
		dir, env := authEnvironment(t, fake, fake.ExpiredKey())
		res := runCLIWith(t, dir, env, "auth", "verify")
		if res.exitCode != 2 || res.stdout != "" || !strings.Contains(res.stderr, "Your developer API key has expired.") {
			t.Errorf("exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
		}
	})

	t.Run("unknown scene", func(t *testing.T) {
		dir, env := authEnvironment(t, fake, fake.Key())
		id := "11111111-2222-4333-8444-555555555555"
		res := runCLIWith(t, dir, env, "auth", "verify", "--scene-id", id)
		if res.exitCode != 3 || strings.Contains(res.stdout, "reachable") ||
			!strings.Contains(res.stderr, "Scene "+id+" is not reachable with this key") {
			t.Errorf("exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
		}
	})

	t.Run("projectified scene", func(t *testing.T) {
		dir, env := authEnvironment(t, fake, fake.Key())
		id := "p12-" + processSceneUUID
		res := runCLIWith(t, dir, env, "auth", "verify", "--scene-id", id)
		if res.exitCode != 0 || !strings.HasSuffix(res.stdout, "Scene "+id+": reachable\n") {
			t.Errorf("exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
		}
	})

	t.Run("json", func(t *testing.T) {
		dir, env := authEnvironment(t, fake, fake.Key())
		res := runCLIWith(t, dir, env, "auth", "verify", "--format", "json")
		var got map[string]any
		if err := json.Unmarshal([]byte(res.stdout), &got); err != nil || res.exitCode != 0 {
			t.Fatalf("exit %d, stdout %q (err %v), stderr %q", res.exitCode, res.stdout, err, res.stderr)
		}
		if got["projectName"] != "Fake Project" || got["keyExpiresAt"] != nil || got["sceneReachable"] != nil {
			t.Errorf("unexpected JSON: %v", got)
		}
	})

	t.Run("no key", func(t *testing.T) {
		dir, env := authEnvironment(t, fake, "")
		res := runCLIWith(t, dir, env, "auth", "verify")
		want := "C3D_DEVELOPER_API_KEY is not set. Please set it with: export C3D_DEVELOPER_API_KEY=your_api_key"
		if res.exitCode != 4 || res.stdout != "" || !strings.Contains(res.stderr, want) {
			t.Errorf("exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
		}
	})
}

func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("testdata", name)
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dst, e.Name()), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func TestSceneUploadInterruptedAfterTheBodyWasSent(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("os.Interrupt cannot be sent to a process on Windows")
	}
	fake := fakegateway.New(t)
	fake.Delay(0)
	fake.Delay(10 * time.Second)
	sceneDir := copyFixture(t, "scene-glb")
	dir, env := authEnvironment(t, fake, fake.Key())

	cmd := exec.Command(os.Args[0], "scene", "upload", "--scene-dir", sceneDir, "--scene-name", "S")
	cmd.Dir = dir
	cmd.Env = append(append(os.Environ(), isolatedEnvironment(t)...), env...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	exited := make(chan error, 1)
	go func() { exited <- cmd.Wait() }()
	// stop kills and reaps the child before failing; stderr is safe to read
	// only once Wait has returned.
	stop := func(message string) {
		t.Helper()
		_ = cmd.Process.Kill()
		<-exited
		t.Fatalf("%s (stderr %q)", message, stderr.String())
	}

	received := false
	for deadline := time.Now().Add(3 * time.Second); !received && time.Now().Before(deadline); {
		for _, r := range fake.Requests() {
			received = received || (r.Method == http.MethodPost && r.Complete)
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !received {
		stop("the fake did not receive the whole POST within three seconds")
	}
	if err := cmd.Process.Signal(os.Interrupt); err != nil {
		stop("sending the interrupt: " + err.Error())
	}

	select {
	case err := <-exited:
		var exitErr *exec.ExitError
		if !errors.As(err, &exitErr) || exitErr.ExitCode() != 1 {
			t.Errorf("exit = %v, want exit code 1", err)
		}
	case <-time.After(2 * time.Second):
		stop("the CLI did not exit within two seconds of the interrupt")
	}
	if want := "The upload may still have completed."; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	if stdout.Len() != 0 {
		t.Errorf("stdout = %q, want empty", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(sceneDir, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("settings.json exists (stat err %v), want none", err)
	}
}

func TestTimeoutAcrossTheProcessBoundary(t *testing.T) {
	fake := fakegateway.New(t)
	fake.Delay(3 * time.Second)
	dir, env := authEnvironment(t, fake, fake.Key())
	res := runCLIWith(t, dir, env, "auth", "verify", "--timeout", "1s")
	if res.exitCode != 1 || res.stdout != "" || !strings.Contains(res.stderr, "Could not reach") {
		t.Errorf("exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
}

func TestObjectUploadDryRunAcrossTheProcessBoundary(t *testing.T) {
	fake := fakegateway.New(t)
	objectDir := copyFixture(t, "object-gltf")
	dir, env := authEnvironment(t, fake, "")
	res := runCLIWith(t, dir, env, "object", "upload", "--object-dir", objectDir, "--object-filename", "cube",
		"--scene-id", "s-1", "--object-id", "cube-1", "--dry-run")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "POST /objects/s-1/cube?version=") || !strings.Contains(res.stdout, "Object ID: cube-1") {
		t.Errorf("exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	if n := len(fake.Requests()); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
	if _, err := os.Stat(filepath.Join(dir, "s-1_object_manifest.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("dry run wrote a manifest (stat err %v)", err)
	}
}

func TestEgoUploadDryRunAcrossTheProcessBoundary(t *testing.T) {
	fake := fakegateway.New(t)
	egoDir := copyFixture(t, "ego-head")
	dir, env := authEnvironment(t, fake, "")
	res := runCLIWith(t, dir, env, "ego", "upload", "--slot", "head", "--ego-dir", egoDir, "--scene-id", "s-1", "--dry-run")
	if res.exitCode != 0 || !strings.Contains(res.stdout, "POST /scenes/s-1/egos/head?version=<latest>") || !strings.Contains(res.stdout, "file  head.gltf  ") {
		t.Errorf("exit %d, stdout %q, stderr %q", res.exitCode, res.stdout, res.stderr)
	}
	if n := len(fake.Requests()); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}
