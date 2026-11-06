package cli

import (
	"bytes"
	"context"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
)

// runWithInput runs the CLI with stdin holding input and IO.Interactive set
// to interactive.
func runWithInput(t *testing.T, interactive bool, input string, args ...string) outcome {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := Execute(context.Background(), args, IO{Stdin: strings.NewReader(input), Stdout: &stdout, Stderr: &stderr, Interactive: interactive})
	return outcome{stdout: stdout.String(), stderr: stderr.String(), code: code}
}

func deletesTo(fake *fakegateway.Server) []fakegateway.Recorded {
	var out []fakegateway.Recorded
	for _, r := range fake.Requests() {
		if r.Method == http.MethodDelete {
			out = append(out, r)
		}
	}
	return out
}

func uploadedHead(t *testing.T) *fakegateway.Server {
	t.Helper()
	fake := startFake(t)
	egoScene(fake)
	if got := egoUpload(t, "head"); got.code != 0 {
		t.Fatalf("upload: exit %d, stderr %q", got.code, got.stderr)
	}
	return fake
}

func TestEgoDeleteYesSendsTheDeleteAndLogsTheResult(t *testing.T) {
	fake := uploadedHead(t)
	got := runCommand(t, "ego", "delete", "--slot", "head", "--scene-id", egoSceneID, "--yes")
	if got.code != 0 || got.stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, "[INFO] Deleted slot head from version 2") {
		t.Errorf("stderr = %q", got.stderr)
	}
	sent := deletesTo(fake)
	if len(sent) != 1 || sent[0].Path != "/v0/scenes/"+egoSceneID+"/egos/head" || sent[0].Query.Get("version") != "2" {
		t.Errorf("DELETEs = %+v", sent)
	}

	again := runCommand(t, "ego", "delete", "--slot", "head", "--scene-id", egoSceneID, "--yes")
	if again.code != 0 || !strings.Contains(again.stderr, "[INFO] Slot head was already empty in version 2") {
		t.Errorf("exit %d, stderr %q", again.code, again.stderr)
	}
	if n := len(deletesTo(fake)); n != 2 {
		t.Errorf("%d DELETEs, want 2", n)
	}
}

func TestEgoDeleteJSONReportsWhetherTheSlotExisted(t *testing.T) {
	uploadedHead(t)
	for _, existed := range []bool{true, false} {
		got := runCommand(t, "ego", "delete", "--slot", "head", "--scene-id", egoSceneID, "--yes", "--version", "2", "--format", "json")
		if got.code != 0 {
			t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
		}
		want := map[string]any{"sceneId": egoSceneID, "versionNumber": 2.0, "slot": "head", "deleted": true, "existed": existed}
		if result := decodeOne(t, got.stdout); !reflect.DeepEqual(result, want) {
			t.Errorf("result = %v, want %v", result, want)
		}
	}
}

func TestEgoDeleteInteractivePromptNamesTheVersionAndHonorsTheAnswer(t *testing.T) {
	for _, tc := range []struct {
		answer  string
		deleted bool
	}{
		{"y\n", true}, {"YES\n", true}, {" Yes \n", true},
		{"n\n", false}, {"\n", false}, {"", false}, {"yeah\n", false},
	} {
		t.Run(strings.TrimSpace(tc.answer), func(t *testing.T) {
			fake := uploadedHead(t)
			got := runWithInput(t, true, tc.answer, "ego", "delete", "--slot", "head", "--scene-id", egoSceneID)
			if got.code != 0 || got.stdout != "" {
				t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
			}
			if prompt := `Delete ego slot "head" from scene ` + egoSceneID + ` version 2? [y/N] `; !strings.Contains(got.stderr, prompt) {
				t.Errorf("stderr = %q, want the prompt %q", got.stderr, prompt)
			}
			if n := len(deletesTo(fake)); (n == 1) != tc.deleted || n > 1 {
				t.Errorf("%d DELETEs, want deleted=%v", n, tc.deleted)
			}
			if aborted := strings.Contains(got.stderr, "Aborted.\n"); aborted == tc.deleted {
				t.Errorf("stderr = %q, want Aborted. only when not deleted", got.stderr)
			}
		})
	}
}

func TestEgoDeleteWithoutYesRefusesWhenNotInteractive(t *testing.T) {
	fake := uploadedHead(t)
	before := requestCount(fake)
	got := runWithInput(t, false, "y\n", "ego", "delete", "--slot", "head", "--scene-id", egoSceneID)
	if got.code != 4 || got.stdout != "" || !strings.Contains(got.stderr, "refusing to delete without --yes when not running interactively") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if n := requestCount(fake) - before; n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestEgoDeleteUnknownSceneExitsThreeAfterPreflight(t *testing.T) {
	fake := startFake(t)
	got := runCommand(t, "ego", "delete", "--slot", "head", "--scene-id", egoSceneID, "--yes")
	if got.code != 3 || got.stdout != "" || !strings.Contains(got.stderr, "Scene "+egoSceneID+" was not found") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	assertVerifyBeforeSceneGET(t, fake, egoSceneID)
	if len(deletesTo(fake)) != 0 {
		t.Errorf("DELETEs = %+v", deletesTo(fake))
	}
}

func TestEgoDeleteUnknownVersionExitsThreeWithoutADelete(t *testing.T) {
	fake := uploadedHead(t)
	got := runCommand(t, "ego", "delete", "--slot", "head", "--scene-id", egoSceneID, "--yes", "--version", "9")
	if got.code != 3 || !strings.Contains(got.stderr, "has no version 9") || len(deletesTo(fake)) != 0 {
		t.Errorf("exit %d, stderr %q, DELETEs %d", got.code, got.stderr, len(deletesTo(fake)))
	}
}

func TestEgoDeleteRejectsBadInputBeforeAnyRequest(t *testing.T) {
	fake := startFake(t)
	for _, args := range [][]string{
		{"--scene-id", egoSceneID, "--yes"},
		{"--slot", "Head", "--scene-id", egoSceneID, "--yes"},
		{"--slot", "head", "--yes"},
		{"--slot", "head", "--scene-id", egoSceneID, "--yes", "--version", "0"},
	} {
		if got := runCommand(t, append([]string{"ego", "delete"}, args...)...); got.code != 4 || got.stdout != "" {
			t.Errorf("%v: exit %d, stdout %q, stderr %q", args, got.code, got.stdout, got.stderr)
		}
	}
	if got := runCommand(t, "ego", "delete", "--slot", "head", "--scene-id", egoSceneID, "--dry-run"); got.code != 4 {
		t.Errorf("--dry-run: exit %d, want 4 (not a delete flag)", got.code)
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestEgoDeleteWithAnExpiredKeyExitsTwoBeforeAnySceneRequest(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	got := runCommand(t, "ego", "delete", "--slot", "head", "--scene-id", egoSceneID, "--yes")
	if got.code != 2 || got.stdout != "" || sceneRequests(fake) != 0 {
		t.Errorf("exit %d, stdout %q, scene requests %d, stderr %q", got.code, got.stdout, sceneRequests(fake), got.stderr)
	}
}
