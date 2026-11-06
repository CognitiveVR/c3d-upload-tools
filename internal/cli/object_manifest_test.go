package cli

import (
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/manifest"
)

// writeManifestFile saves a manifest with the given ids, each mesh named "mesh-<id>"
// unless the id is "cube", whose mesh is "cube".
func writeManifestFile(t *testing.T, path string, ids ...string) {
	t.Helper()
	m := &manifest.Manifest{}
	for _, id := range ids {
		mesh := "mesh-" + id
		if id == "cube" {
			mesh = "cube"
		}
		e := manifest.DefaultEntry(id, mesh)
		e.InitialPosition = []float64{1, 2, 3}
		m.Objects = append(m.Objects, e)
	}
	if err := m.Save(path); err != nil {
		t.Fatal(err)
	}
}

func uploadCube(t *testing.T, version string) {
	t.Helper()
	got := runCommand(t, "object", "upload", "--object-dir", sceneDir(t, "object-gltf"), "--object-filename", "cube",
		"--scene-id", objectSceneID, "--version", version, "--object-id", "cube", "--manifest", "scratch_manifest.json")
	if got.code != 0 {
		t.Fatalf("object upload: exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestObjectManifestUploadMissingFileExitsFourNamingItWithZeroRequests(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--manifest", "mispelled.json")
	if got.code != 4 || !strings.Contains(got.stderr, "mispelled.json") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	got = runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID)
	if got.code != 4 || !strings.Contains(got.stderr, objectManifestPath()) {
		t.Errorf("default path: exit %d, stderr %q", got.code, got.stderr)
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestObjectManifestUploadDuplicateIDsExitFourWithZeroRequests(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	writeManifestFile(t, objectManifestPath(), "dup", "DUP")
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID)
	if got.code != 4 || !strings.Contains(strings.ToLower(got.stderr), "dup") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestObjectManifestUploadPostsToTheLatestVersionAndWarnsForObjectsWithoutFiles(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	uploadCube(t, "2")
	writeManifestFile(t, objectManifestPath(), "cube", "other")
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	var posted *fakegateway.Recorded
	for _, r := range postsTo(fake) {
		if r.Path == "/v0/objects/"+objectSceneID {
			posted = &r
		}
	}
	if posted == nil || posted.Query.Get("version") != "2" || posted.Header.Get("Content-Type") != "application/json" {
		t.Fatalf("manifest POST = %+v", posted)
	}
	result := decodeObject(t, got.stdout)
	without, _ := result["objectsWithoutFiles"].([]any)
	if result["sceneId"] != objectSceneID || result["versionNumber"] != 2.0 || result["objectCount"] != 2.0 || len(without) != 1 || without[0] != "other" {
		t.Errorf("result = %v", result)
	}
	for _, want := range []string{"Manifest uploaded: 2 objects to version 2", `(mesh mesh-other) has no files in version 2; upload it again for this version`} {
		if !strings.Contains(got.stderr, want) {
			t.Errorf("stderr lacks %q: %q", want, got.stderr)
		}
	}
	if strings.Contains(got.stderr, "mesh cube)") {
		t.Errorf("cube has files in version 2: %q", got.stderr)
	}
}

func TestObjectManifestUploadToANewVersionWarnsForEveryObject(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	uploadCube(t, "1")
	writeManifestFile(t, objectManifestPath(), "cube")
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--version", "2")
	if got.code != 0 || got.stdout != "" {
		t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, `object "cube" (mesh cube) has no files in version 2`) {
		t.Errorf("stderr = %q", got.stderr)
	}
	got = runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--version", "1")
	if got.code != 0 || strings.Contains(got.stderr, "has no files") {
		t.Errorf("version 1: exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestObjectManifestUploadTextModeLogsOnStderrOnly(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	writeManifestFile(t, objectManifestPath(), "cube")
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID)
	if got.code != 0 || got.stdout != "" || !strings.Contains(got.stderr, "[INFO] Manifest uploaded: 1 objects to version 2") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestObjectManifestUploadEmptyBody400ShowsTheManifestHint(t *testing.T) {
	fake := startFake(t)
	writeManifestFile(t, objectManifestPath(), "cube")
	fake.FailWith(http.StatusOK, "")
	fake.FailWith(http.StatusOK, `{"id":"obj-scene","versions":[{"id":21,"versionNumber":2}]}`)
	fake.FailWith(http.StatusBadRequest, "")
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID)
	if got.code != 1 || !strings.Contains(got.stderr, "The gateway rejected the manifest (status 400). The usual causes are duplicate ids or malformed vectors") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestObjectManifestUploadInvalidJSON400ShowsTheGatewayBody(t *testing.T) {
	fake := startFake(t)
	writeManifestFile(t, objectManifestPath(), "cube")
	fake.FailWith(http.StatusOK, "")
	fake.FailWith(http.StatusOK, `{"id":"obj-scene","versions":[{"id":21,"versionNumber":2}]}`)
	fake.FailWith(http.StatusBadRequest, "Invalid json.")
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID)
	if got.code != 1 || !strings.Contains(got.stderr, "Invalid json.") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestObjectManifestUploadUnknownVersionExitsThreeBeforeThePost(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	writeManifestFile(t, objectManifestPath(), "cube")
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--version", "9")
	if got.code != 3 || !strings.Contains(got.stderr, "no version 9") || !strings.Contains(got.stderr, "2") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	if len(postsTo(fake)) != 0 {
		t.Errorf("POSTs = %+v", postsTo(fake))
	}
}

func TestObjectManifestUploadRejectsAVersionBelowOne(t *testing.T) {
	fake := startFake(t)
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--version", "0")
	if got.code != 4 || requestCount(fake) != 0 {
		t.Errorf("exit %d, %d requests, stderr %q", got.code, requestCount(fake), got.stderr)
	}
}

func TestObjectManifestUploadDryRunNeedsNoKeyAndSendsNothing(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", "")
	writeManifestFile(t, "custom.json", "cube", "other")
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--manifest", "custom.json", "--dry-run")
	if got.code != 0 || requestCount(fake) != 0 {
		t.Fatalf("exit %d, %d requests, stderr %q", got.code, requestCount(fake), got.stderr)
	}
	for _, want := range []string{"POST /objects/" + objectSceneID + "?version=<resolved at run time>", "Authorization: APIKEY:DEVELOPER (not set)", "Objects: 2", "Manifest: custom.json"} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q: %q", want, got.stdout)
		}
	}

	got = runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--manifest", "custom.json", "--dry-run", "--version", "3", "--format", "json")
	var plan map[string]any
	if err := json.Unmarshal([]byte(got.stdout), &plan); err != nil {
		t.Fatalf("stdout %q: %v", got.stdout, err)
	}
	if plan["path"] != "/objects/"+objectSceneID+"?version=3" || plan["objectCount"] != 2.0 || plan["dryRun"] != true {
		t.Errorf("plan = %v", plan)
	}
}

func TestObjectManifestUploadLeavesTheManifestFileUntouched(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	writeManifestFile(t, objectManifestPath(), "cube")
	before, _ := os.ReadFile(objectManifestPath())
	got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID)
	after, _ := os.ReadFile(objectManifestPath())
	if got.code != 0 || string(before) != string(after) {
		t.Errorf("exit %d; manifest changed", got.code)
	}
}
