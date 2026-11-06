package cli

import (
	"encoding/json"
	"os"
	"reflect"
	"strings"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/manifest"
)

// seedObjects uploads the cube's files and a two-entry manifest to version 2,
// and a one-entry manifest to version 1.
func seedObjects(t *testing.T, fake *fakegateway.Server) {
	t.Helper()
	objectScene(fake)
	uploadCube(t, "2")
	writeManifestFile(t, "seed.json", "cube", "other")
	if got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--manifest", "seed.json"); got.code != 0 {
		t.Fatalf("seed v2: exit %d, stderr %q", got.code, got.stderr)
	}
	writeManifestFile(t, "seed1.json", "cube")
	if got := runCommand(t, "object", "manifest", "upload", "--scene-id", objectSceneID, "--manifest", "seed1.json", "--version", "1"); got.code != 0 {
		t.Fatalf("seed v1: exit %d, stderr %q", got.code, got.stderr)
	}
}

func decodeList(t *testing.T, stdout string) []map[string]any {
	t.Helper()
	var rows []map[string]any
	if err := json.Unmarshal([]byte(stdout), &rows); err != nil {
		t.Fatalf("stdout is not a JSON array: %v: %q", err, stdout)
	}
	return rows
}

func TestObjectListPrintsTheLatestVersionsArrayInBothFormats(t *testing.T) {
	fake := startFake(t)
	seedObjects(t, fake)
	for _, format := range []string{"text", "json"} {
		got := runCommand(t, "object", "list", "--scene-id", objectSceneID, "--format", format)
		if got.code != 0 {
			t.Fatalf("%s: exit %d, stderr %q", format, got.code, got.stderr)
		}
		rows := decodeList(t, got.stdout)
		if len(rows) != 2 || rows[0]["sdkId"] != "cube" || rows[0]["versionId"] != 21.0 {
			t.Errorf("%s: rows = %v", format, rows)
		}
		if files, _ := rows[0]["files"].([]any); len(files) != 3 {
			t.Errorf("%s: cube files = %v", format, rows[0]["files"])
		}
		if !strings.HasPrefix(got.stdout, "[\n  {\n    \"id\": ") || !strings.HasSuffix(got.stdout, "]\n") {
			t.Errorf("%s: stdout is not two-space indented: %.60q", format, got.stdout)
		}
	}
}

func TestObjectListVersionFlagUsesVersionByNumber(t *testing.T) {
	fake := startFake(t)
	seedObjects(t, fake)
	got := runCommand(t, "object", "list", "--scene-id", objectSceneID, "--version", "1")
	rows := decodeList(t, got.stdout)
	if got.code != 0 || len(rows) != 1 || rows[0]["versionId"] != 20.0 {
		t.Fatalf("exit %d, rows %v", got.code, rows)
	}
	if files, _ := rows[0]["files"].([]any); len(files) != 0 {
		t.Errorf("version 1 cube has files %v, want none", files)
	}
	reqs := fake.Requests()
	if last := reqs[len(reqs)-1]; last.Path != "/v0/versions/20/objects" {
		t.Errorf("last request = %s", last.Path)
	}
}

func TestObjectListUnknownVersionExitsThreeNamingTheKnownOnes(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	got := runCommand(t, "object", "list", "--scene-id", objectSceneID, "--version", "7")
	if got.code != 3 || !strings.Contains(got.stderr, "no version 7") || !strings.Contains(got.stderr, "2 and 1") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	for _, r := range fake.Requests() {
		if strings.Contains(r.Path, "/versions/") {
			t.Errorf("listing requested after an unknown version: %s", r.Path)
		}
	}
}

func TestObjectListSceneWithoutVersionsExitsThree(t *testing.T) {
	fake := startFake(t)
	fake.AddScene(fakegateway.Scene{ID: objectSceneID})
	got := runCommand(t, "object", "list", "--scene-id", objectSceneID)
	if got.code != 3 || !strings.Contains(got.stderr, "has no versions") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestObjectListWithoutWriteFlagsWritesNothingAndKeepsTheManifestByteIdentical(t *testing.T) {
	fake := startFake(t)
	seedObjects(t, fake)
	writeManifestFile(t, objectManifestPath(), "local-only")
	before, _ := os.ReadFile(objectManifestPath())
	entriesBefore, _ := os.ReadDir(".")
	got := runCommand(t, "object", "list", "--scene-id", objectSceneID)
	after, _ := os.ReadFile(objectManifestPath())
	entriesAfter, _ := os.ReadDir(".")
	if got.code != 0 || string(before) != string(after) || len(entriesBefore) != len(entriesAfter) {
		t.Errorf("exit %d; manifest changed or files added (%d -> %d entries)", got.code, len(entriesBefore), len(entriesAfter))
	}
	if _, err := os.Stat(objectSceneID + "_object_list.json"); !os.IsNotExist(err) {
		t.Errorf("list file exists (err %v)", err)
	}
}

func TestObjectListWriteManifestReshapesTheListWithItsTransforms(t *testing.T) {
	fake := startFake(t)
	seedObjects(t, fake)
	writeManifestFile(t, objectManifestPath(), "local-only")
	got := runCommand(t, "object", "list", "--scene-id", objectSceneID, "--write-manifest")
	if got.code != 0 || !strings.Contains(got.stderr, objectManifestPath()) {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	m := loadManifest(t, objectManifestPath())
	if len(m.Objects) != 2 || m.Objects[0].ID != "cube" || m.Objects[1].ID != "other" {
		t.Fatalf("manifest = %+v", m.Objects)
	}
	if !reflect.DeepEqual(m.Objects[0].InitialPosition, []float64{1, 2, 3}) || !reflect.DeepEqual(m.Objects[0].ScaleCustom, []float64{1, 1, 1}) ||
		!reflect.DeepEqual(m.Objects[0].InitialRotation, []float64{0, 0, 0, 1}) || m.Objects[0].Mesh != "cube" {
		t.Errorf("cube entry = %+v", m.Objects[0])
	}
	if _, err := os.Stat(objectSceneID + "_object_list.json"); !os.IsNotExist(err) {
		t.Errorf("list file written without --write-list (err %v)", err)
	}
	if err := m.Validate(); err != nil {
		t.Errorf("written manifest is invalid: %v", err)
	}
}

func TestObjectListWriteListWritesTheRawArray(t *testing.T) {
	fake := startFake(t)
	seedObjects(t, fake)
	got := runCommand(t, "object", "list", "--scene-id", objectSceneID, "--write-list")
	if got.code != 0 || !strings.Contains(got.stderr, objectSceneID+"_object_list.json") {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	data, err := os.ReadFile(objectSceneID + "_object_list.json")
	if err != nil {
		t.Fatal(err)
	}
	if string(data) != got.stdout {
		t.Errorf("file differs from stdout:\n%s\n%s", data, got.stdout)
	}
	if _, err := os.Stat(manifest.DefaultPath(objectSceneID)); !os.IsNotExist(err) {
		t.Errorf("manifest written without --write-manifest (err %v)", err)
	}
}

func TestObjectListRequiresASceneID(t *testing.T) {
	fake := startFake(t)
	got := runCommand(t, "object", "list")
	if got.code != 4 || requestCount(fake) != 0 {
		t.Errorf("exit %d, %d requests, stderr %q", got.code, requestCount(fake), got.stderr)
	}
}
