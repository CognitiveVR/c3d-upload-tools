package cli

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/fakegateway"
)

const egoSceneID = "ego-scene"

// egoScene registers a scene with versions 1 and 2.
func egoScene(fake *fakegateway.Server) {
	fake.AddScene(fakegateway.Scene{ID: egoSceneID, SDKFacingID: egoSceneID, SceneName: "Egos",
		Versions: []fakegateway.Version{{ID: 31, VersionNumber: 2}, {ID: 30, VersionNumber: 1}}})
}

// fastWait makes --wait poll every millisecond and give up after timeout.
func fastWait(t *testing.T, timeout time.Duration) {
	t.Helper()
	interval, previous := egoWaitInterval, egoWaitTimeout
	egoWaitInterval, egoWaitTimeout = time.Millisecond, timeout
	t.Cleanup(func() { egoWaitInterval, egoWaitTimeout = interval, previous })
}

func egoPolls(fake *fakegateway.Server) int {
	n := 0
	for _, r := range fake.Requests() {
		if r.Method == http.MethodGet && r.Path == "/v0/scenes/"+egoSceneID+"/egos" {
			n++
		}
	}
	return n
}

func egoUpload(t *testing.T, slot string, extra ...string) outcome {
	t.Helper()
	args := append([]string{"ego", "upload", "--slot", slot, "--ego-dir", sceneDir(t, "ego-head"), "--scene-id", egoSceneID}, extra...)
	return runCommand(t, args...)
}

// decodeOne decodes stdout as exactly one JSON object.
func decodeOne(t *testing.T, stdout string) map[string]any {
	t.Helper()
	dec := json.NewDecoder(strings.NewReader(stdout))
	var got map[string]any
	if err := dec.Decode(&got); err != nil {
		t.Fatalf("stdout is not JSON: %v: %q", err, stdout)
	}
	var extra any
	if err := dec.Decode(&extra); err != io.EOF {
		t.Fatalf("stdout holds more than one JSON value: %q", stdout)
	}
	return got
}

func TestEgoUploadPostsEveryPartToTheLatestVersionAndPrintsTheSlot(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	got := egoUpload(t, "head")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if want := "Slot: head\nModel: head.gltf\nFiles: 4\n"; got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	assertVerifyBeforeSceneGET(t, fake, egoSceneID)
	sent := postsTo(fake)
	if len(sent) != 1 {
		t.Fatalf("POSTs = %+v", sent)
	}
	if sent[0].Path != "/v0/scenes/"+egoSceneID+"/egos/head" || sent[0].Query.Get("version") != "2" || !sent[0].Complete {
		t.Errorf("POST = %s ?%v complete=%v", sent[0].Path, sent[0].Query, sent[0].Complete)
	}
	var names []string
	for _, p := range sent[0].Parts {
		if p.Field != "file" {
			t.Errorf("part %s field = %q, want file", p.Filename, p.Field)
		}
		names = append(names, p.Filename)
	}
	if want := []string{"head.gltf", "head.bin", "head_albedo.png", "ego.json"}; !reflect.DeepEqual(names, want) {
		t.Errorf("parts = %v, want %v", names, want)
	}
	if egoPolls(fake) != 0 {
		t.Errorf("polled without --wait")
	}
}

func TestEgoUploadVersionFlagSkipsTheSceneLookup(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	got := egoUpload(t, "head", "--version", "1")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if n := getsTo(fake, "/v0/scenes/"+egoSceneID); n != 0 {
		t.Errorf("%d scene GETs, want 0", n)
	}
	if sent := postsTo(fake); len(sent) != 1 || sent[0].Query.Get("version") != "1" {
		t.Errorf("POSTs = %+v", sent)
	}
}

func TestEgoUploadReplacingASlotIsTheSamePost(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	for range 2 {
		if got := egoUpload(t, "head"); got.code != 0 {
			t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
		}
	}
	sent := postsTo(fake)
	if len(sent) != 2 || sent[0].Path != sent[1].Path || !reflect.DeepEqual(sent[0].Query, sent[1].Query) || len(sent[1].Parts) != len(sent[0].Parts) {
		t.Fatalf("POSTs = %+v", sent)
	}
	got := runCommand(t, "ego", "list", "--scene-id", egoSceneID, "--format", "json")
	if egos, _ := decodeOne(t, got.stdout)["egos"].([]any); len(egos) != 1 {
		t.Errorf("slots after a replace = %v", egos)
	}
}

func TestEgoUploadWaitPrintsOptimizedOnceTheSlotFlips(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	fake.SetOptimizedAfterPolls(egoSceneID, 2, "head", 2)
	fastWait(t, time.Minute)
	got := egoUpload(t, "head", "--wait")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if want := "Slot: head\nModel: head.gltf\nFiles: 4\nOptimized: true\n"; got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	if n := egoPolls(fake); n != 2 {
		t.Errorf("%d polls, want 2", n)
	}
}

func TestEgoUploadWaitTimeoutWarnsAndExitsZero(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	fastWait(t, 20*time.Millisecond)
	got := egoUpload(t, "head", "--wait")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if want := "Slot: head\nModel: head.gltf\nFiles: 4\n"; got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	if want := "[WARN] " + optimizationPendingMessage(20*time.Millisecond); !strings.Contains(got.stderr, want) {
		t.Errorf("stderr = %q, want it to contain %q", got.stderr, want)
	}
	if egoPolls(fake) == 0 {
		t.Errorf("no polls")
	}
}

func TestEgoUploadWaitCutOffByTimeoutSaysTheModelWasUploaded(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	fastWait(t, time.Minute)
	got := egoUpload(t, "head", "--wait", "--timeout", "1s")
	if got.code != 1 {
		t.Fatalf("exit %d, want 1 (stderr %q)", got.code, got.stderr)
	}
	want := "The model was uploaded; waiting for optimization was cut off (context deadline exceeded). Check with: " +
		binaryName + " ego list --scene-id " + egoSceneID
	if !strings.Contains(got.stderr, want) {
		t.Errorf("stderr = %q, want it to contain %q", got.stderr, want)
	}
	if want := "Slot: head\n"; !strings.HasPrefix(got.stdout, want) {
		t.Errorf("stdout = %q, want the upload result", got.stdout)
	}
}

func TestOptimizationPendingMessageNamesTheTimeoutInSeconds(t *testing.T) {
	want := "Optimization still pending after 120 seconds; small models are never marked optimized."
	if got := optimizationPendingMessage(120 * time.Second); got != want {
		t.Errorf("message = %q, want %q", got, want)
	}
	if egoWaitInterval != 5*time.Second || egoWaitTimeout != 120*time.Second {
		t.Errorf("wait defaults = %v every %v, want 120s every 5s", egoWaitTimeout, egoWaitInterval)
	}
}

func TestEgoUploadJSONPrintsOneObjectAfterTheWait(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	fake.SetOptimizedAfterPolls(egoSceneID, 2, "head", 3)
	fastWait(t, time.Minute)
	got := egoUpload(t, "head", "--wait", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	result := decodeOne(t, got.stdout)
	want := map[string]any{"sceneId": egoSceneID, "versionNumber": 2.0, "slot": "head", "model": "head.gltf", "fileCount": 4.0, "optimized": true}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("result = %v, want %v", result, want)
	}
	if n := egoPolls(fake); n != 3 {
		t.Errorf("%d polls, want 3: the result follows the poll that saw the flip", n)
	}
}

func TestEgoUploadJSONOptimizedIsNullWithoutWaitAndFalseOnTimeout(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	fastWait(t, 10*time.Millisecond)
	for _, tc := range []struct {
		args []string
		want any
	}{
		{[]string{"--format", "json"}, nil},
		{[]string{"--format", "json", "--wait"}, false},
	} {
		got := egoUpload(t, "head", tc.args...)
		if got.code != 0 {
			t.Fatalf("%v: exit %d, stderr %q", tc.args, got.code, got.stderr)
		}
		result := decodeOne(t, got.stdout)
		if value, present := result["optimized"]; !present || value != tc.want {
			t.Errorf("%v: optimized = %v (present %v), want %v", tc.args, value, present, tc.want)
		}
		warning := `{"level":"warn","message":"` + optimizationPendingMessage(10*time.Millisecond) + `"}`
		if warned := strings.Contains(got.stderr, warning); warned != (tc.want == false) {
			t.Errorf("%v: stderr = %q, want the pending warning only after a wait", tc.args, got.stderr)
		}
	}
}

func TestEgoUploadDryRunMakesNoRequestsAndNeedsNoKey(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", "")
	got := egoUpload(t, "head", "--dry-run", "--wait")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
	for _, want := range []string{
		"POST /scenes/" + egoSceneID + "/egos/head?version=<resolved at run time>\n",
		"Authorization: APIKEY:DEVELOPER (not set)\n",
		"file  head.gltf  ", "file  head.bin  ", "file  head_albedo.png  ", "file  ego.json  ",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q: %q", want, got.stdout)
		}
	}
}

func TestEgoUploadDryRunJSONNamesTheVersionAndRedactsTheKey(t *testing.T) {
	fake := startFake(t)
	got := egoUpload(t, "controller_left", "--dry-run", "--version", "3", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	result := decodeOne(t, got.stdout)
	if result["dryRun"] != true || result["method"] != "POST" || result["path"] != "/scenes/"+egoSceneID+"/egos/controller_left?version=3" ||
		result["slot"] != "controller_left" {
		t.Errorf("dry run = %v", result)
	}
	if auth, _ := result["authorization"].(string); strings.Contains(auth, fake.Key()) || !strings.HasSuffix(auth, "…") {
		t.Errorf("authorization = %q", auth)
	}
	if parts, _ := result["parts"].([]any); len(parts) != 4 {
		t.Errorf("parts = %v", result["parts"])
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestEgoUploadRejectsBadInputBeforeAnyRequest(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	dir := sceneDir(t, "ego-head")
	broken := sceneDir(t, "ego-head")
	if err := os.WriteFile(filepath.Join(broken, "head.gltf"),
		[]byte(`{"asset":{"version":"2.0"},"buffers":[{"uri":"head.bin","byteLength":840}],"images":[{"uri":"missing.png"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want string
	}{
		{"no slot", []string{"--ego-dir", dir, "--scene-id", egoSceneID}, "--slot is required"},
		{"no directory", []string{"--slot", "head", "--scene-id", egoSceneID}, "--ego-dir is required"},
		{"bad slot", []string{"--slot", "Head", "--ego-dir", dir, "--scene-id", egoSceneID}, `invalid slot "Head"`},
		{"path slot", []string{"--slot", "../head", "--ego-dir", dir, "--scene-id", egoSceneID}, "invalid slot"},
		{"no scene", []string{"--slot", "head", "--ego-dir", dir}, "--scene-id is required"},
		{"version zero", []string{"--slot", "head", "--ego-dir", dir, "--scene-id", egoSceneID, "--version", "0"}, "invalid --version 0"},
		{"model check", []string{"--slot", "head", "--ego-dir", broken, "--scene-id", egoSceneID}, "missing.png"},
	} {
		got := runCommand(t, append([]string{"ego", "upload"}, tc.args...)...)
		if got.code != 4 || got.stdout != "" || !strings.Contains(got.stderr, tc.want) {
			t.Errorf("%s: exit %d, stdout %q, stderr %q, want exit 4 and %q", tc.name, got.code, got.stdout, got.stderr, tc.want)
		}
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestEgoUploadUnknownSceneExitsThreeWithoutAPost(t *testing.T) {
	fake := startFake(t)
	got := egoUpload(t, "head")
	if got.code != 3 || got.stdout != "" || !strings.Contains(got.stderr, "Scene "+egoSceneID+" was not found") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if len(postsTo(fake)) != 0 {
		t.Errorf("POSTs = %+v", postsTo(fake))
	}
	assertVerifyBeforeSceneGET(t, fake, egoSceneID)
}

func TestEgoUploadWithAnExpiredKeyExitsTwoBeforeAnySceneRequest(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	got := egoUpload(t, "head")
	if got.code != 2 || sceneRequests(fake) != 0 {
		t.Errorf("exit %d, scene requests %d, stderr %q", got.code, sceneRequests(fake), got.stderr)
	}
}

func TestEgoUploadCustomSlotWarnsAndUploads(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	got := egoUpload(t, "left_foot")
	if got.code != 0 || !strings.Contains(got.stderr, `[WARN] slot "left_foot" is not one of the known slots`) {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if sent := postsTo(fake); len(sent) != 1 || sent[0].Path != "/v0/scenes/"+egoSceneID+"/egos/left_foot" {
		t.Errorf("POSTs = %+v", sent)
	}
}

func TestEgoUploadVersionFlagToAnUnknownSceneExitsThree(t *testing.T) {
	fake := startFake(t)
	got := egoUpload(t, "head", "--version", "1")
	if got.code != 3 || got.stdout != "" || !strings.Contains(got.stderr, "Scene "+egoSceneID+" was not found in the project this key belongs to") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if n := getsTo(fake, "/v0/scenes/"+egoSceneID); n != 0 {
		t.Errorf("%d scene GETs, want 0", n)
	}
	if sent := postsTo(fake); len(sent) != 1 {
		t.Errorf("POSTs = %+v, want the one rejected POST", sent)
	}
}
