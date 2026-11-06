package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
)

const testSceneUUID = "0b1f6c3e-2a4d-4e8b-9c7a-5d3e1f2a4b6c"

// isolate points the process at an empty home and working directory with no
// C3D_* variables, so no real .env or upload.yaml is read.
func isolate(t *testing.T) {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	for _, name := range []string{
		"C3D_DEVELOPER_API_KEY", "C3D_UPLOAD_GATEWAY_URL", "C3D_SCENE_ID",
		"C3D_DEFAULT_ENVIRONMENT", "C3D_ENV", "NO_COLOR",
	} {
		t.Setenv(name, "")
	}
	t.Chdir(t.TempDir())
}

type outcome struct {
	stdout string
	stderr string
	code   int
}

func runCommand(t *testing.T, args ...string) outcome {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), args, IO{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr})
	return outcome{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func startFake(t *testing.T) *fakegateway.Server {
	t.Helper()
	isolate(t)
	fake := fakegateway.New(t)
	t.Setenv("C3D_UPLOAD_GATEWAY_URL", fake.URL())
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.Key())
	return fake
}

func TestAuthVerifyTextPrintsProjectOrganizationAndNeverExpiry(t *testing.T) {
	startFake(t)
	got := runCommand(t, "auth", "verify")
	if got.code != 0 {
		t.Fatalf("exit code = %d, want 0 (stderr %q)", got.code, got.stderr)
	}
	want := "Environment: prod\nProject: Fake Project\nOrganization: Fake Organization\nKey expires: never\n"
	if got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	if !strings.Contains(got.stderr, "[WARN]") {
		t.Errorf("stderr should carry the foreign-host warning, got %q", got.stderr)
	}
}

func TestAuthVerifyPrintsKeyExpiryAsRFC3339(t *testing.T) {
	fake := startFake(t)
	expiry := time.Date(2031, 3, 4, 5, 6, 7, 0, time.UTC)
	fake.SetUser(fakegateway.User{ProjectName: "P", OrganizationName: "O", KeyExpiresAt: expiry.UnixMilli()})
	got := runCommand(t, "auth", "verify")
	if got.code != 0 {
		t.Fatalf("exit code = %d (stderr %q)", got.code, got.stderr)
	}
	if want := "Key expires: 2031-03-04T05:06:07Z\n"; !strings.Contains(got.stdout, want) {
		t.Errorf("stdout = %q, want it to contain %q", got.stdout, want)
	}
}

func TestAuthVerifyExpiredKeyPrintsRotationGuidanceAndExitsTwo(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	got := runCommand(t, "auth", "verify")
	if got.code != 2 {
		t.Fatalf("exit code = %d, want 2", got.code)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
	for _, want := range []string{"Your developer API key has expired.", "Manage developer key"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr missing %q: %q", want, got.stderr)
		}
	}
}

func TestAuthVerifyWithoutKeyExitsFourWithRequireAPIKeyText(t *testing.T) {
	startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", "")
	got := runCommand(t, "auth", "verify")
	if got.code != 4 {
		t.Fatalf("exit code = %d, want 4", got.code)
	}
	if got.stdout != "" {
		t.Errorf("stdout = %q, want empty", got.stdout)
	}
	if want := "C3D_DEVELOPER_API_KEY is not set. Please set it with: export C3D_DEVELOPER_API_KEY=your_api_key"; !strings.Contains(got.stderr, want) {
		t.Errorf("stderr = %q, want it to contain %q", got.stderr, want)
	}
}

func TestAuthVerifySceneReachableAndProjectified(t *testing.T) {
	fake := startFake(t)
	projectified := "p12-" + testSceneUUID
	fake.AddScene(fakegateway.Scene{ID: projectified, SDKFacingID: testSceneUUID, SceneName: "S"})
	fake.AddScene(fakegateway.Scene{ID: testSceneUUID, SDKFacingID: testSceneUUID, SceneName: "S"})

	for _, id := range []string{projectified, testSceneUUID} {
		got := runCommand(t, "auth", "verify", "--scene-id", id)
		if got.code != 0 {
			t.Fatalf("%s: exit code = %d (stderr %q)", id, got.code, got.stderr)
		}
		if want := "Scene " + id + ": reachable\n"; !strings.HasSuffix(got.stdout, want) {
			t.Errorf("%s: stdout = %q, want suffix %q", id, got.stdout, want)
		}
	}
}

func TestAuthVerifySceneIDFallsBackToEnvironment(t *testing.T) {
	fake := startFake(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneUUID, SDKFacingID: testSceneUUID, SceneName: "S"})
	t.Setenv("C3D_SCENE_ID", testSceneUUID)
	got := runCommand(t, "auth", "verify")
	if got.code != 0 || !strings.Contains(got.stdout, "Scene "+testSceneUUID+": reachable") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestAuthVerifyUnknownSceneExitsThreeAndPrintsNothingMoreOnStdout(t *testing.T) {
	startFake(t)
	got := runCommand(t, "auth", "verify", "--scene-id", testSceneUUID)
	if got.code != 3 {
		t.Fatalf("exit code = %d, want 3 (stderr %q)", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, "reachable") {
		t.Errorf("stdout = %q, must not claim the scene is reachable", got.stdout)
	}
	if want := "Scene " + testSceneUUID + " is not reachable with this key"; !strings.Contains(got.stderr, "[ERROR] "+want) {
		t.Errorf("stderr = %q, want ERROR line %q", got.stderr, want)
	}
}

func TestAuthVerifyRejectsMalformedSceneIDBeforeAnyRequest(t *testing.T) {
	fake := startFake(t)
	got := runCommand(t, "auth", "verify", "--scene-id", "../etc")
	if got.code != 4 {
		t.Fatalf("exit code = %d, want 4", got.code)
	}
	if n := len(fake.Requests()); n != 0 {
		t.Errorf("fake received %d requests, want 0", n)
	}
}

func TestAuthVerifyJSONEmitsStructOnly(t *testing.T) {
	fake := startFake(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneUUID, SDKFacingID: testSceneUUID, SceneName: "S"})

	got := runCommand(t, "auth", "verify", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit code = %d (stderr %q)", got.code, got.stderr)
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &decoded); err != nil {
		t.Fatalf("stdout is not one JSON object: %v: %q", err, got.stdout)
	}
	want := map[string]any{
		"environment": "prod", "projectName": "Fake Project", "organizationName": "Fake Organization",
		"keyExpiresAt": nil, "sceneReachable": nil,
	}
	if len(decoded) != len(want) {
		t.Errorf("keys = %v, want exactly %v", decoded, want)
	}
	for key, value := range want {
		if v, ok := decoded[key]; !ok || v != value {
			t.Errorf("%s = %v (present %v), want %v", key, v, ok, value)
		}
	}
	for _, line := range strings.Split(strings.TrimRight(got.stderr, "\n"), "\n") {
		var record map[string]any
		if err := json.Unmarshal([]byte(line), &record); err != nil {
			t.Errorf("stderr line is not JSON: %q", line)
		}
	}

	got = runCommand(t, "auth", "verify", "--format", "json", "--scene-id", testSceneUUID)
	if err := json.Unmarshal([]byte(got.stdout), &decoded); err != nil || decoded["sceneReachable"] != true {
		t.Errorf("stdout = %q (err %v), want sceneReachable true", got.stdout, err)
	}
}

func TestAuthVerifyJSONReportsGatewayErrorAsRecordOnStderr(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	got := runCommand(t, "auth", "verify", "--format", "json")
	if got.code != 2 || got.stdout != "" {
		t.Fatalf("exit %d, stdout %q", got.code, got.stdout)
	}
	lines := strings.Split(strings.TrimSpace(got.stderr), "\n")
	var record map[string]any
	if err := json.Unmarshal([]byte(lines[len(lines)-1]), &record); err != nil {
		t.Fatalf("last stderr line is not a JSON record: %v: %q", err, got.stderr)
	}
	if record["error"] != "key_expired" || record["status_code"] != float64(401) {
		t.Errorf("record = %v, want error key_expired and status_code 401", record)
	}
}

func TestTimeoutBoundsTheCommand(t *testing.T) {
	fake := startFake(t)
	fake.Delay(3 * time.Second)
	start := time.Now()
	got := runCommand(t, "auth", "verify", "--timeout", "300ms")
	if got.code != 1 {
		t.Fatalf("exit code = %d, want 1 (stderr %q)", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, "Could not reach") {
		t.Errorf("stderr = %q, want Could not reach", got.stderr)
	}
	if elapsed := time.Since(start); elapsed > 2500*time.Millisecond {
		t.Errorf("command took %v, want it cut off by the 300ms timeout", elapsed)
	}
}

func TestInvalidConfigurationIsAUsageError(t *testing.T) {
	startFake(t)
	got := runCommand(t, "auth", "verify", "--env", "staging")
	if got.code != 4 || got.stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if n := len(strings.Split(strings.TrimRight(got.stderr, "\n"), "\n")); n != 1 {
		t.Errorf("stderr has %d lines, want 1: %q", n, got.stderr)
	}
}
