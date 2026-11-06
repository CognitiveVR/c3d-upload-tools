package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/scene"
	"github.com/cognitivevr/c3d-upload-tools/internal/version"
)

var testdataDir = func() string {
	wd, err := os.Getwd()
	if err != nil {
		panic(err)
	}
	return filepath.Join(wd, "..", "..", "testdata")
}()

// sceneDir copies testdata/<fixture> into a fresh temporary directory.
func sceneDir(t *testing.T, fixture string) string {
	t.Helper()
	src := filepath.Join(testdataDir, fixture)
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

func postsTo(fake *fakegateway.Server) []fakegateway.Recorded {
	var out []fakegateway.Recorded
	for _, r := range fake.Requests() {
		if r.Method == http.MethodPost {
			out = append(out, r)
		}
	}
	return out
}

func settingsOnDisk(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatalf("settings.json: %v", err)
	}
	return data
}

func assertNoSettingsFile(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("settings.json exists (stat err %v), want none", err)
	}
}

func decodeObject(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var got map[string]any
	if err := json.Unmarshal([]byte(stdout), &got); err != nil {
		t.Fatalf("stdout is not one JSON object: %v: %q", err, stdout)
	}
	return got
}

func TestSceneUploadNewSceneSendsPartsInOrderAndPrintsTheID(t *testing.T) {
	fake := startFake(t)
	dir := sceneDir(t, "scene-gltf")
	got := runCommand(t, "scene", "upload", "--scene-dir", dir, "--scene-name", "New Scene")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}

	sent := postsTo(fake)
	if len(sent) != 1 || sent[0].Path != "/v0/scenes" || !sent[0].Complete {
		t.Fatalf("POSTs = %+v, want one complete POST /v0/scenes", sent)
	}
	type wantPart struct{ field, filename, contentType string }
	want := []wantPart{
		{"file", "scene.gltf", "application/octet-stream"},
		{"file", "scene.bin", "application/octet-stream"},
		{"file", "texture1.png", "application/octet-stream"},
		{"file", "texture2.png", "application/octet-stream"},
		{"file", "texture3.png", "application/octet-stream"},
		{"file", "settings.json", "application/json"},
		{"screenshot", "screenshot.png", "application/octet-stream"},
	}
	parts := sent[0].Parts
	if len(parts) != len(want) {
		t.Fatalf("parts = %+v", parts)
	}
	for i, w := range want {
		if parts[i].Field != w.field || parts[i].Filename != w.filename || parts[i].ContentType != w.contentType {
			t.Errorf("part %d = %s %s %s, want %+v", i, parts[i].Field, parts[i].Filename, parts[i].ContentType, w)
		}
	}
	written := settingsOnDisk(t, dir)
	if parts[5].Size != int64(len(written)) || !bytes.Equal(parts[5].Body, written) {
		t.Errorf("settings part = %d bytes %q, written %d bytes %q", parts[5].Size, parts[5].Body, len(written), written)
	}
	if !bytes.Contains(written, []byte(`"sceneName": "New Scene"`)) || !bytes.Contains(written, []byte(`"sdkVersion": "`+version.SDKVersion()+`"`)) {
		t.Errorf("settings.json = %s", written)
	}

	var sceneID string
	for _, r := range fake.Requests() {
		if id, ok := strings.CutPrefix(r.Path, "/v0/scenes/"); ok && r.Method == http.MethodGet {
			sceneID = id
		}
	}
	if sceneID == "" || got.stdout != sceneID+"\n" {
		t.Errorf("stdout = %q, want the new scene id %q and a newline", got.stdout, sceneID)
	}
	if strings.Contains(got.stderr, "upload them again") {
		t.Errorf("a new scene has nothing to upload again: %q", got.stderr)
	}
}

func TestSceneUploadNewSceneJSON(t *testing.T) {
	startFake(t)
	got := runCommand(t, "scene", "upload", "--scene-dir", sceneDir(t, "scene-glb"), "--scene-name", "S", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	result := decodeObject(t, got.stdout)
	if len(result) != 4 || result["created"] != true || result["versionNumber"] != float64(1) || result["versionId"] == nil || result["sceneId"] == "" {
		t.Errorf("result = %v, want exactly sceneId, created, versionNumber 1 and versionId", result)
	}
	for _, line := range strings.Split(strings.TrimRight(got.stderr, "\n"), "\n") {
		if line != "" && !json.Valid([]byte(line)) {
			t.Errorf("stderr line is not JSON: %q", line)
		}
	}
}

func TestSceneUploadNewVersionReportsItAndWarnsAboutDependents(t *testing.T) {
	fake := startFake(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneUUID, SDKFacingID: testSceneUUID, SceneName: "Existing",
		Versions: []fakegateway.Version{{ID: 11, VersionNumber: 1}}})
	fake.SetNextUpload(testSceneUUID, 1, fakegateway.Version{ID: 12, VersionNumber: 2})
	got := runCommand(t, "scene", "upload", "--scene-dir", sceneDir(t, "scene-glb"), "--scene-id", testSceneUUID, "--scene-name", "Renamed", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if sent := postsTo(fake); len(sent) != 1 || sent[0].Path != "/v0/scenes/"+testSceneUUID {
		t.Fatalf("POSTs = %+v", sent)
	}
	result := decodeObject(t, got.stdout)
	if result["sceneId"] != testSceneUUID || result["created"] != false || result["versionNumber"] != float64(2) || result["versionId"] != float64(12) {
		t.Errorf("result = %v", result)
	}
	if want := "upload them again for version 2"; !strings.Contains(got.stderr, want) || !strings.Contains(got.stderr, `"level":"warn"`) {
		t.Errorf("stderr = %q, want a warn record containing %q", got.stderr, want)
	}
}

func TestSceneUploadConcurrentVersionsExitsOneWithCandidates(t *testing.T) {
	for _, format := range []string{formatText, formatJSON} {
		t.Run(format, func(t *testing.T) {
			fake := startFake(t)
			fake.AddScene(fakegateway.Scene{ID: testSceneUUID, SDKFacingID: testSceneUUID, SceneName: "Existing",
				Versions: []fakegateway.Version{{ID: 11, VersionNumber: 1}}})
			fake.SetNextUpload(testSceneUUID, 0, fakegateway.Version{ID: 12, VersionNumber: 2}, fakegateway.Version{ID: 13, VersionNumber: 3})
			dir := sceneDir(t, "scene-glb")
			got := runCommand(t, "scene", "upload", "--scene-dir", dir, "--scene-id", testSceneUUID, "--format", format)
			if got.code != 1 {
				t.Fatalf("exit %d, want 1 (stderr %q)", got.code, got.stderr)
			}
			if format == formatText {
				if got.stdout != testSceneUUID+"\n" {
					t.Errorf("stdout = %q, want the scene id", got.stdout)
				}
			} else {
				result := decodeObject(t, got.stdout)
				if _, ok := result["versionNumber"]; ok {
					t.Errorf("result %v must not claim a versionNumber", result)
				}
				if _, ok := result["versionId"]; ok {
					t.Errorf("result %v must not claim a versionId", result)
				}
				if result["concurrentVersions"] != true || fmt.Sprint(result["candidateVersions"]) != "[2 3]" {
					t.Errorf("result = %v, want concurrentVersions and candidates [2 3]", result)
				}
				if !strings.Contains(got.stderr, `"error":"concurrent_versions"`) {
					t.Errorf("stderr = %q, want the concurrent_versions record", got.stderr)
				}
			}
			want := "The upload finished, but versions 2 and 3 appeared while it ran, so this upload cannot be told from another. Check with: cognitive3d-cli-upload scene show --scene-id " + testSceneUUID
			if !strings.Contains(got.stderr, want) {
				t.Errorf("stderr = %q, want %q", got.stderr, want)
			}
			if !strings.Contains(got.stderr, "upload them again for the new version") {
				t.Errorf("stderr = %q, want the dependents warning", got.stderr)
			}
			settingsOnDisk(t, dir)
		})
	}
}

func TestSceneUploadExistingSceneWithoutNameUsesTheGatewayName(t *testing.T) {
	fake := startFake(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneUUID, SDKFacingID: testSceneUUID, SceneName: "From Gateway",
		Versions: []fakegateway.Version{{ID: 11, VersionNumber: 1}}})
	dir := sceneDir(t, "scene-glb")
	got := runCommand(t, "scene", "upload", "--scene-dir", dir, "--scene-id", testSceneUUID)
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	sent := postsTo(fake)
	if len(sent) != 1 || !bytes.Contains(sent[0].Parts[1].Body, []byte(`"sceneName": "From Gateway"`)) {
		t.Fatalf("settings part = %q, want the gateway's name", sent[0].Parts[1].Body)
	}
	if !bytes.Contains(settingsOnDisk(t, dir), []byte(`"sceneName": "From Gateway"`)) {
		t.Error("settings.json on disk lacks the gateway's name")
	}
}

func TestSceneUploadDryRunNeedsNoKeyAndSendsNothing(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", "")
	dir := sceneDir(t, "scene-glb")
	got := runCommand(t, "scene", "upload", "--scene-dir", dir, "--scene-name", "Dry", "--dry-run")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if n := len(fake.Requests()); n != 0 {
		t.Errorf("fake received %d requests, want 0", n)
	}
	settings, _, err := scene.BuildSettings(nil, "Dry", version.SDKVersion())
	if err != nil {
		t.Fatal(err)
	}
	want := fmt.Sprintf("POST /scenes\nAuthorization: APIKEY:DEVELOPER (not set)\n"+
		"file  scene.glb  730040 bytes\nfile  settings.json  %d bytes\nscreenshot  screenshot.png  2149708 bytes\n", len(settings))
	if got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	assertNoSettingsFile(t, dir)
}

func TestSceneUploadDryRunRedactsTheKeyAndPrintsJSON(t *testing.T) {
	fake := startFake(t)
	got := runCommand(t, "scene", "upload", "--scene-dir", sceneDir(t, "scene-glb"), "--scene-name", "Dry", "--dry-run")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if want := "Authorization: APIKEY:DEVELOPER " + fake.Key()[:4] + "…\n"; !strings.Contains(got.stdout, want) {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	if strings.Contains(got.stdout+got.stderr, fake.Key()) {
		t.Error("dry run printed the whole key")
	}

	fake.AddScene(fakegateway.Scene{ID: testSceneUUID, SceneName: "S"})
	got = runCommand(t, "scene", "upload", "--scene-dir", sceneDir(t, "scene-glb"), "--scene-id", testSceneUUID, "--dry-run", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	result := decodeObject(t, got.stdout)
	if result["dryRun"] != true || result["method"] != "POST" || result["path"] != "/scenes/"+testSceneUUID {
		t.Errorf("result = %v", result)
	}
	parts, _ := result["parts"].([]any)
	if len(parts) != 3 {
		t.Fatalf("parts = %v", result["parts"])
	}
	first, _ := parts[0].(map[string]any)
	if first["field"] != "file" || first["filename"] != "scene.glb" || first["bytes"] != float64(730040) {
		t.Errorf("first part = %v", first)
	}
	if n := len(fake.Requests()); n != 0 {
		t.Errorf("fake received %d requests, want 0", n)
	}
}

func TestSceneUploadLocalValidationExitsFourWithoutRequests(t *testing.T) {
	cases := []struct {
		name  string
		setup func(t *testing.T, dir string)
		args  []string
		want  string
	}{
		{"both model layouts", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "model.glb"), []byte("glTF"), 0o644); err != nil {
				t.Fatal(err)
			}
		}, []string{"--scene-name", "S"}, "never both"},
		{"missing screenshot", func(t *testing.T, dir string) {
			if err := os.Remove(filepath.Join(dir, "screenshot.png")); err != nil {
				t.Fatal(err)
			}
		}, []string{"--scene-name", "S"}, "screenshot.png is missing"},
		{"missing scene name", nil, nil, "scene name is required"},
		{"missing scene name in a dry run", nil, []string{"--dry-run"}, "scene name is required"},
		{"model check error", func(t *testing.T, dir string) {
			if err := os.WriteFile(filepath.Join(dir, "scene.gltf"), []byte(`{"images":[{"uri":"gone.png"}]}`), 0o644); err != nil {
				t.Fatal(err)
			}
		}, []string{"--scene-name", "S"}, "gone.png is missing"},
		{"malformed scene id", nil, []string{"--scene-id", "../x"}, "invalid scene id"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			fake := startFake(t)
			dir := sceneDir(t, "scene-gltf")
			if tc.setup != nil {
				tc.setup(t, dir)
			}
			got := runCommand(t, append([]string{"scene", "upload", "--scene-dir", dir}, tc.args...)...)
			if got.code != 4 || got.stdout != "" {
				t.Fatalf("exit %d, stdout %q, want 4 and nothing (stderr %q)", got.code, got.stdout, got.stderr)
			}
			if !strings.Contains(got.stderr, tc.want) {
				t.Errorf("stderr = %q, want %q", got.stderr, tc.want)
			}
			if n := len(fake.Requests()); n != 0 {
				t.Errorf("fake received %d requests, want 0", n)
			}
		})
	}
}

func TestSceneUploadModelCheckPrintsEveryFinding(t *testing.T) {
	startFake(t)
	dir := sceneDir(t, "scene-gltf")
	gltf := `{"buffers":[{"uri":"scene.bin","byteLength":1}],"images":[{"uri":"gone.png"},{"uri":"data:image/png;base64,AA=="}]}`
	if err := os.WriteFile(filepath.Join(dir, "scene.gltf"), []byte(gltf), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, "scene", "upload", "--scene-dir", dir, "--scene-name", "S")
	if got.code != 4 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	for _, want := range []string{"[ERROR] buffers[0] declares byteLength", "[ERROR] images[0] uri \"gone.png\"", "[WARN] images[1] holds an embedded data: uri", "[ERROR] the model check found 2 errors"} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr = %q, want %q", got.stderr, want)
		}
	}
}

func TestSceneUploadRequiresSceneDir(t *testing.T) {
	fake := startFake(t)
	got := runCommand(t, "scene", "upload", "--scene-name", "S")
	if got.code != 4 || !strings.Contains(got.stderr, "--scene-dir is required") || len(fake.Requests()) != 0 {
		t.Errorf("exit %d, stderr %q, requests %d", got.code, got.stderr, len(fake.Requests()))
	}
}

func TestSceneUploadMissingNameWithExpiredKeyIsStillAUsageError(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	got := runCommand(t, "scene", "upload", "--scene-dir", sceneDir(t, "scene-glb"))
	if got.code != 4 || len(fake.Requests()) != 0 {
		t.Errorf("exit %d, requests %d, want 4 and none (stderr %q)", got.code, len(fake.Requests()), got.stderr)
	}
}

func TestSceneUploadExpiredKeyExitsTwoBeforeThePOST(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	dir := sceneDir(t, "scene-glb")
	got := runCommand(t, "scene", "upload", "--scene-dir", dir, "--scene-name", "S")
	if got.code != 2 || !strings.Contains(got.stderr, "Your developer API key has expired.") {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if n := len(postsTo(fake)); n != 0 {
		t.Errorf("POSTs = %d, want 0", n)
	}
	assertNoSettingsFile(t, dir)
}

func TestSceneUploadUnknownSceneIDExitsThreeWithoutPOST(t *testing.T) {
	fake := startFake(t)
	got := runCommand(t, "scene", "upload", "--scene-dir", sceneDir(t, "scene-glb"), "--scene-id", testSceneUUID, "--scene-name", "S")
	if got.code != 3 {
		t.Fatalf("exit %d, want 3 (stderr %q)", got.code, got.stderr)
	}
	if want := "Scene " + testSceneUUID + " was not found in the project this key belongs to."; !strings.Contains(got.stderr, want) {
		t.Errorf("stderr = %q, want %q", got.stderr, want)
	}
	if n := len(postsTo(fake)); n != 0 {
		t.Errorf("POSTs = %d, want 0", n)
	}
}

func TestSceneUploadCancelledAfterTheBodyWasSent(t *testing.T) {
	fake := startFake(t)
	fake.Delay(0)
	fake.Delay(10 * time.Second)
	dir := sceneDir(t, "scene-glb")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		deadline := time.Now().Add(5 * time.Second)
		for time.Now().Before(deadline) {
			if sent := postsTo(fake); len(sent) == 1 && sent[0].Complete {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		cancel()
	}()

	var stdout, stderr bytes.Buffer
	code := Execute(ctx, []string{"scene", "upload", "--scene-dir", dir, "--scene-name", "S"},
		IO{Stdin: &bytes.Buffer{}, Stdout: &stdout, Stderr: &stderr})
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("exit %d, stdout %q, want 1 and nothing (stderr %q)", code, stdout.String(), stderr.String())
	}
	if want := "The upload may still have completed. Check with: cognitive3d-cli-upload scene list"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want %q", stderr.String(), want)
	}
	assertNoSettingsFile(t, dir)
}

func TestRedactKey(t *testing.T) {
	for key, want := range map[string]string{"": "(not set)", "abc": "…", "abcd": "…", "abcdefgh": "abcd…"} {
		if got := redactKey(key); got != want {
			t.Errorf("redactKey(%q) = %q, want %q", key, got, want)
		}
	}
}

func TestJoinNumbers(t *testing.T) {
	for _, tc := range []struct {
		in   []int
		want string
	}{{[]int{2}, "2"}, {[]int{2, 3}, "2 and 3"}, {[]int{2, 3, 4}, "2, 3 and 4"}} {
		if got := joinNumbers(tc.in); got != tc.want {
			t.Errorf("joinNumbers(%v) = %q, want %q", tc.in, got, tc.want)
		}
	}
}
