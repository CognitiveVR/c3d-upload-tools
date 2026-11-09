package cli

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/manifest"
)

const objectSceneID = "obj-scene"

// objectScene registers a scene with versions 1 and 2.
func objectScene(fake *fakegateway.Server) {
	fake.AddScene(fakegateway.Scene{ID: objectSceneID, SDKFacingID: objectSceneID, SceneName: "Objects",
		Versions: []fakegateway.Version{{ID: 21, VersionNumber: 2}, {ID: 20, VersionNumber: 1}}})
}

// objectManifestPath is where the default manifest lands: the working
// directory isolate() set up.
func objectManifestPath() string { return manifest.DefaultPath(objectSceneID) }

func loadManifest(t *testing.T, path string) *manifest.Manifest {
	t.Helper()
	m, err := manifest.Load(path)
	if err != nil {
		t.Fatalf("manifest %s: %v", path, err)
	}
	return m
}

var objectIDLine = regexp.MustCompile(`^Object ID: ([0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12})\n$`)

func requestCount(fake *fakegateway.Server) int { return len(fake.Requests()) }

func TestObjectUploadSendsPartsToTheLatestVersionAndSavesTheManifest(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")

	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID)
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	match := objectIDLine.FindStringSubmatch(got.stdout)
	if match == nil {
		t.Fatalf("stdout = %q, want one Object ID line with a UUID", got.stdout)
	}
	id := match[1]
	if !strings.Contains(got.stderr, "generated") || !strings.Contains(got.stderr, id) {
		t.Errorf("stderr should log the id source: %q", got.stderr)
	}

	var verify, sceneGET int
	for _, r := range fake.Requests() {
		switch {
		case r.Path == "/v0/apiKeys/verify":
			verify++
		case r.Method == http.MethodGet && r.Path == "/v0/scenes/"+objectSceneID:
			sceneGET++
		}
	}
	if verify != 1 || sceneGET != 1 {
		t.Errorf("verify %d, scene lookups %d, want 1 and 1", verify, sceneGET)
	}
	sent := postsTo(fake)
	if len(sent) != 2 || sent[1].Path != "/v0/objects/"+objectSceneID || sent[1].Query.Get("version") != "2" {
		t.Fatalf("POSTs = %+v, want the files then the manifest entry", sent)
	}
	if sent[0].Path != "/v0/objects/"+objectSceneID+"/cube" || sent[0].Query.Get("version") != "2" || !sent[0].Complete {
		t.Errorf("POST = %s ?%v complete=%v", sent[0].Path, sent[0].Query, sent[0].Complete)
	}
	want := []string{"cube.gltf", "cube.bin", "cvr_object_thumbnail.png"}
	if len(sent[0].Parts) != len(want) {
		t.Fatalf("parts = %+v", sent[0].Parts)
	}
	for i, name := range want {
		if p := sent[0].Parts[i]; p.Field != "file" || p.Filename != name || p.ContentType != "application/octet-stream" {
			t.Errorf("part %d = %s %s %s", i, p.Field, p.Filename, p.ContentType)
		}
	}

	m := loadManifest(t, objectManifestPath())
	if len(m.Objects) != 1 || m.Objects[0].ID != id || m.Objects[0].Mesh != "cube" || m.Objects[0].Name != "cube" {
		t.Errorf("manifest = %+v", m.Objects)
	}
}

func TestObjectUploadWithoutManifestUploadPrintsTheCommandToRun(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--upload-manifest=false", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	for _, r := range postsTo(fake) {
		if r.Path == "/v0/objects/"+objectSceneID {
			t.Errorf("manifest POSTed: %+v", r)
		}
	}
	hint := "object manifest upload --scene-id " + objectSceneID + " --version 2"
	if !strings.Contains(got.stderr, hint) || strings.Contains(got.stderr, "--manifest ") {
		t.Errorf("stderr should name the manifest upload command for the version, without --manifest for the default file: %q", got.stderr)
	}
	if result := decodeObject(t, got.stdout); result["manifestUploaded"] != false {
		t.Errorf("result = %v", result)
	}
}

func TestQuoteArgument(t *testing.T) {
	cases := map[string]string{
		"obj-scene_object_manifest.json": "obj-scene_object_manifest.json",
		`C:\Users\me\m.json`:             `"C:\Users\me\m.json"`,
		"/tmp/x/manifest.json":           "/tmp/x/manifest.json",
		"~/m.json":                       `"~/m.json"`,
		"=m.json":                        `"=m.json"`,
		"a=b/m.json":                     "a=b/m.json",
		"/tmp/my manifest.json":          `"/tmp/my manifest.json"`,
		"/work/R&D/m.json":               `"/work/R&D/m.json"`,
		`C:\Users\me\{Game}\m.json`:      `"C:\Users\me\{Game}\m.json"`,
		"/tmp/proj(1)/m.json":            `"/tmp/proj(1)/m.json"`,
		"/tmp/o'brien/m.json":            `"/tmp/o'brien/m.json"`,
	}
	for in, want := range cases {
		if got := quoteArgument(in); got != want {
			t.Errorf("quoteArgument(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestObjectUploadHintNamesACustomManifestFile(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	custom := filepath.Join(t.TempDir(), "custom.json")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--manifest", custom, "--upload-manifest=false")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	hint := "object manifest upload --scene-id " + objectSceneID + " --version 2 --manifest " + quoteArgument(custom)
	if !strings.Contains(got.stderr, hint) {
		t.Errorf("stderr should name the custom manifest file: %q", got.stderr)
	}

	spaced := filepath.Join(t.TempDir(), "my manifest.json")
	got = runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--manifest", spaced, "--upload-manifest=false")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if !strings.Contains(got.stderr, `--manifest "`+spaced+`"`) {
		t.Errorf("a manifest path with a space must be quoted in the hint: %q", got.stderr)
	}
}

// The manifest POST after the files carries only this object's entry, with the
// transforms the local file holds for it; other entries in the file stay local.
func TestObjectUploadPostsOnlyThisObjectsManifestEntryAfterTheFiles(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	cube := manifest.DefaultEntry("cube-1", "cube")
	cube.ScaleCustom = []float64{2, 2, 2}
	existing := &manifest.Manifest{Objects: []manifest.Entry{manifest.DefaultEntry("other", "mesh-other"), cube}}
	if err := existing.Save(objectManifestPath()); err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	sent := postsTo(fake)
	if len(sent) != 2 || sent[0].Path != "/v0/objects/"+objectSceneID+"/cube" || sent[1].Path != "/v0/objects/"+objectSceneID {
		t.Fatalf("POSTs = %+v", sent)
	}
	if sent[1].Query.Get("version") != "2" || sent[1].Header.Get("Content-Type") != "application/json" {
		t.Errorf("manifest POST = ?%v %v", sent[1].Query, sent[1].Header)
	}
	var body manifest.Manifest
	if err := json.Unmarshal(sent[1].Body, &body); err != nil {
		t.Fatalf("manifest body = %s (%v)", sent[1].Body, err)
	}
	if len(body.Objects) != 1 || !reflect.DeepEqual(body.Objects[0], cube) {
		t.Errorf("POSTed %+v, want only the cube entry with its scale", body.Objects)
	}
	if saved := loadManifest(t, objectManifestPath()); len(saved.Objects) != 2 {
		t.Errorf("saved manifest = %+v, want both entries kept locally", saved.Objects)
	}
	result := decodeObject(t, got.stdout)
	if result["manifestUploaded"] != true || result["objectId"] != "cube-1" {
		t.Errorf("result = %v", result)
	}
}

func TestObjectUploadJSONResult(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID,
		"--object-id", "cube-1", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	want := fmt.Sprintf(`{"objectId":"cube-1","mesh":"cube","sceneId":%q,"versionNumber":2,"manifestPath":%q,"manifestUploaded":true}`+"\n",
		objectSceneID, objectManifestPath())
	if got.stdout != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
}

func TestObjectUploadEscapesTheMeshNameButNotThePartFilenames(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := t.TempDir()
	copyBinary(t, filepath.Join(testdataDir, "scene-glb", "scene.glb"), filepath.Join(dir, "My Object (v2).glb"))
	copyBinary(t, filepath.Join(testdataDir, "object-gltf", "cvr_object_thumbnail.png"), filepath.Join(dir, "cvr_object_thumbnail.png"))

	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "My Object (v2)", "--scene-id", objectSceneID)
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	sent := postsTo(fake)
	if len(sent) != 2 || sent[0].RawPath != "/v0/objects/"+objectSceneID+"/My%20Object%20%28v2%29" {
		t.Fatalf("POSTs = %+v", sent)
	}
	if sent[0].Parts[0].Filename != "My Object (v2).glb" || sent[0].Parts[1].Filename != "cvr_object_thumbnail.png" {
		t.Errorf("parts = %+v", sent[0].Parts)
	}
	if m := loadManifest(t, objectManifestPath()); m.Objects[0].Mesh != "My Object (v2)" {
		t.Errorf("manifest = %+v", m.Objects)
	}
}

func copyBinary(t *testing.T, from, to string) {
	t.Helper()
	data, err := os.ReadFile(from)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(to, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestObjectUploadMissingThumbnailExitsFourWithZeroRequestsEvenWithAnInvalidKey(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	dir := sceneDir(t, "object-gltf")
	if err := os.Remove(filepath.Join(dir, "cvr_object_thumbnail.png")); err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID)
	if got.code != 4 || !strings.Contains(got.stderr, "cvr_object_thumbnail.png is missing") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestObjectUploadModelCheckErrorsAreShownAndExitFour(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := t.TempDir()
	copyBinary(t, filepath.Join(testdataDir, "object-gltf", "cvr_object_thumbnail.png"), filepath.Join(dir, "cvr_object_thumbnail.png"))
	if err := os.WriteFile(filepath.Join(dir, "Chair.glb"), []byte("not a glb, but long enough to read"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "Chair", "--scene-id", objectSceneID)
	if got.code != 4 || requestCount(fake) != 0 || !strings.Contains(got.stderr, "[ERROR]") || !strings.Contains(got.stderr, "model check found") {
		t.Errorf("exit %d, %d requests, stderr %q", got.code, requestCount(fake), got.stderr)
	}
}

func TestObjectUploadWarnsAboutSkippedFiles(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	if err := os.WriteFile(filepath.Join(dir, "notes.txt"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID)
	if got.code != 0 || strings.Count(got.stderr, "skipping") != 1 || !strings.Contains(got.stderr, "notes.txt") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	for _, p := range postsTo(fake)[0].Parts {
		if p.Filename == "notes.txt" {
			t.Error("notes.txt was uploaded")
		}
	}
}

func TestObjectUploadMalformedManifestExitsFourWithZeroRequests(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	if err := os.WriteFile(objectManifestPath(), []byte(`{"objects": [`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID)
	if got.code != 4 || !strings.Contains(got.stderr, objectManifestPath()) || requestCount(fake) != 0 {
		t.Errorf("exit %d, %d requests, stderr %q", got.code, requestCount(fake), got.stderr)
	}
}

func TestObjectUploadInvalidManifestEntryExitsFourWithZeroRequests(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	if err := os.WriteFile(objectManifestPath(), []byte(`{"objects":[{"id":"a","mesh":"","name":"x"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID)
	if got.code != 4 || !strings.Contains(got.stderr, "mesh") || requestCount(fake) != 0 {
		t.Errorf("exit %d, %d requests, stderr %q", got.code, requestCount(fake), got.stderr)
	}
}

func TestObjectUploadMeshOutsideLatin1ExitsFourWithZeroRequests(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := t.TempDir()
	copyBinary(t, filepath.Join(testdataDir, "scene-glb", "scene.glb"), filepath.Join(dir, "Вазa.glb"))
	copyBinary(t, filepath.Join(testdataDir, "object-gltf", "cvr_object_thumbnail.png"), filepath.Join(dir, "cvr_object_thumbnail.png"))
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "Вазa", "--scene-id", objectSceneID)
	if got.code != 4 || !strings.Contains(got.stderr, "Latin-1") || requestCount(fake) != 0 {
		t.Errorf("exit %d, %d requests, stderr %q", got.code, requestCount(fake), got.stderr)
	}
}

func TestObjectUploadObjectIDFlagIsUsedVerbatim(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--object-id", "My Id/1")
	if got.code != 0 || got.stdout != "Object ID: My Id/1\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, "from --object-id") {
		t.Errorf("stderr = %q", got.stderr)
	}
	if m := loadManifest(t, objectManifestPath()); m.Objects[0].ID != "My Id/1" {
		t.Errorf("manifest = %+v", m.Objects)
	}
}

func TestObjectUploadInvalidObjectIDExitsFourWithZeroRequests(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID,
		"--object-id", strings.Repeat("x", 300))
	if got.code != 4 || !strings.Contains(got.stderr, "255") || requestCount(fake) != 0 {
		t.Errorf("exit %d, %d requests, stderr %q", got.code, requestCount(fake), got.stderr)
	}
}

func TestObjectUploadReusesTheManifestIdForTheSameMeshAndKeepsItsTransforms(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	existing := `{"objects":[
	  {"id":"keep-me","mesh":"cube","name":"My Cube","scaleCustom":[2,2,2],"initialPosition":[1,2,3],"initialRotation":[0,0,0,1]},
	  {"id":"other","mesh":"sphere","name":"Ball"}]}`
	if err := os.WriteFile(objectManifestPath(), []byte(existing), 0o644); err != nil {
		t.Fatal(err)
	}
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID)
	if got.code != 0 || got.stdout != "Object ID: keep-me\n" {
		t.Fatalf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if !strings.Contains(got.stderr, "reused from "+objectManifestPath()) {
		t.Errorf("stderr = %q", got.stderr)
	}
	m := loadManifest(t, objectManifestPath())
	if len(m.Objects) != 2 {
		t.Fatalf("manifest = %+v", m.Objects)
	}
	e := m.Objects[0]
	if e.ID != "keep-me" || e.Name != "My Cube" || fmt.Sprint(e.ScaleCustom) != "[2 2 2]" || fmt.Sprint(e.InitialPosition) != "[1 2 3]" {
		t.Errorf("entry = %+v", e)
	}
}

func TestObjectUploadVersionFlagSkipsTheSceneLookup(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--version", "3")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if n := getsTo(fake, "/v0/scenes/"); n != 0 {
		t.Errorf("%d scene lookups, want 0", n)
	}
	reqs := fake.Requests()
	if reqs[0].Path != "/v0/apiKeys/verify" {
		t.Errorf("first request = %s, want the preflight", reqs[0].Path)
	}
	if sent := postsTo(fake); len(sent) != 2 || sent[0].Query.Get("version") != "3" || sent[1].Query.Get("version") != "3" {
		t.Errorf("POSTs = %+v, want the files and the manifest entry both on version 3", sent)
	}
}

func TestObjectUploadRejectsAVersionBelowOne(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	for _, v := range []string{"0", "-1"} {
		got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--version="+v)
		if got.code != 4 || !strings.Contains(got.stderr, "--version") {
			t.Errorf("--version=%s: exit %d, stderr %q", v, got.code, got.stderr)
		}
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestObjectUploadManifestFlagIsHonored(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	other := filepath.Join(t.TempDir(), "other.json")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID,
		"--manifest", other, "--object-id", "c1")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if m := loadManifest(t, other); len(m.Objects) != 1 || m.Objects[0].ID != "c1" {
		t.Errorf("manifest = %+v", m.Objects)
	}
	if _, err := os.Stat(objectManifestPath()); !os.IsNotExist(err) {
		t.Errorf("default manifest exists (err %v)", err)
	}
}

func TestObjectUploadDryRunMakesNoRequestsNeedsNoKeyAndWritesNothing(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", "")
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--dry-run", "--object-id", "dry-1")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
	for _, want := range []string{
		"POST /objects/" + objectSceneID + "/cube?version=<resolved at run time>",
		"Authorization: APIKEY:DEVELOPER (not set)",
		"file  cube.gltf  ", "file  cube.bin  ", "file  cvr_object_thumbnail.png  ",
		"Object ID: dry-1 (from --object-id)",
	} {
		if !strings.Contains(got.stdout, want) {
			t.Errorf("stdout lacks %q: %q", want, got.stdout)
		}
	}
	if _, err := os.Stat(objectManifestPath()); !os.IsNotExist(err) {
		t.Errorf("dry run wrote a manifest (err %v)", err)
	}
}

func TestObjectUploadDryRunSaysAGeneratedIDIsIllustrative(t *testing.T) {
	startFake(t)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--dry-run")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	line := regexp.MustCompile(`(?m)^Object ID: [0-9a-f-]{36} \(illustrative; a new id is generated at upload\)$`)
	if !line.MatchString(got.stdout) {
		t.Errorf("stdout = %q, want the generated id marked illustrative", got.stdout)
	}
}

func TestObjectUploadDryRunJSONEscapesTheMeshAndRedactsTheKey(t *testing.T) {
	startFake(t)
	dir := t.TempDir()
	copyBinary(t, filepath.Join(testdataDir, "scene-glb", "scene.glb"), filepath.Join(dir, "My Object (v2).glb"))
	copyBinary(t, filepath.Join(testdataDir, "object-gltf", "cvr_object_thumbnail.png"), filepath.Join(dir, "cvr_object_thumbnail.png"))
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "My Object (v2)", "--scene-id", objectSceneID,
		"--version", "4", "--dry-run", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	result := decodeObject(t, got.stdout)
	if result["path"] != "/objects/"+objectSceneID+"/My%20Object%20%28v2%29?version=4" || result["dryRun"] != true ||
		result["objectId"] == "" || result["idSource"] != "generated" {
		t.Errorf("dry run = %v", result)
	}
	auth, _ := result["authorization"].(string)
	if strings.Contains(auth, "valid") && len(auth) > len("APIKEY:DEVELOPER ")+len("vali…") {
		t.Errorf("authorization = %q", auth)
	}
}

func TestObjectUploadSceneWithoutVersionsExitsThree(t *testing.T) {
	fake := startFake(t)
	fake.AddScene(fakegateway.Scene{ID: "empty-scene", SDKFacingID: "empty-scene"})
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", "empty-scene")
	if got.code != 3 || !strings.Contains(got.stderr, "no versions") || len(postsTo(fake)) != 0 {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	if _, err := os.Stat(manifest.DefaultPath("empty-scene")); !os.IsNotExist(err) {
		t.Errorf("manifest written (err %v)", err)
	}
}

func TestObjectUploadUnknownSceneExitsThreeAndSavesNoManifest(t *testing.T) {
	fake := startFake(t)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID)
	if got.code != 3 || len(postsTo(fake)) != 0 {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	assertVerifyBeforeSceneGET(t, fake, objectSceneID)
}

func TestObjectUploadFailedPostKeepsTheManifestUntouched(t *testing.T) {
	fake := startFake(t)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--version", "1")
	if got.code != 3 || got.stdout != "" {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if len(postsTo(fake)) != 1 {
		t.Errorf("POSTs = %+v", postsTo(fake))
	}
	if _, err := os.Stat(objectManifestPath()); !os.IsNotExist(err) {
		t.Errorf("manifest written after a failed upload (err %v)", err)
	}
}

func TestObjectUploadRequiresObjectFilenameAndSceneID(t *testing.T) {
	fake := startFake(t)
	dir := sceneDir(t, "object-gltf")
	cases := map[string][]string{
		"--object-filename": {"object", "upload", "--object-dir", dir, "--scene-id", objectSceneID},
		"--object-dir":      {"object", "upload", "--object-filename", "cube", "--scene-id", objectSceneID},
		"--scene-id":        {"object", "upload", "--object-dir", dir, "--object-filename", "cube"},
	}
	for flag, args := range cases {
		got := runCommand(t, args...)
		if got.code != 4 || !strings.Contains(got.stderr, flag) {
			t.Errorf("without %s: exit %d, stderr %q", flag, got.code, got.stderr)
		}
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestObjectUploadAcceptsLegacyUnderscoreFlags(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	got := runCommand(t, "object", "upload", "--object_dir", dir, "--object_filename", "cube", "--scene_id", objectSceneID, "--object_id", "legacy-1", "--dry_run")
	if got.code != 0 || !strings.Contains(got.stdout, "Object ID: legacy-1") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestObjectUploadManifestUploadFailureStillSavesAndReportsTheObject(t *testing.T) {
	fake := startFake(t)
	dir := sceneDir(t, "object-gltf")
	fake.FailWith(http.StatusOK, "") // preflight
	fake.FailWith(http.StatusOK, "") // object POST
	fake.FailWith(http.StatusBadRequest, "Invalid json.")
	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID,
		"--version", "1", "--object-id", "c1", "--format", "json")
	if got.code == 0 || !strings.Contains(got.stderr, "Invalid json.") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	result := decodeObject(t, got.stdout)
	if result["objectId"] != "c1" || result["manifestUploaded"] != false {
		t.Errorf("result = %v", result)
	}
	if m := loadManifest(t, objectManifestPath()); len(m.Objects) != 1 {
		t.Errorf("manifest = %+v", m.Objects)
	}
}

func TestObjectUploadKeepsAnEntryAddedToTheManifestDuringTheUpload(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	saved := make(chan error, 1)
	fake.OnRoute("POST /v0/objects/{sceneId}/{meshName}", func() {
		other := &manifest.Manifest{Objects: []manifest.Entry{manifest.DefaultEntry("other", "mesh-other")}}
		saved <- other.Save(objectManifestPath())
	})

	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID, "--object-id", "c1")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	select {
	case err := <-saved:
		if err != nil {
			t.Fatal(err)
		}
	default:
		t.Fatal("the object POST never happened")
	}
	var ids []string
	for _, e := range loadManifest(t, objectManifestPath()).Objects {
		ids = append(ids, e.ID)
	}
	if !slices.Equal(ids, []string{"other", "c1"}) {
		t.Errorf("manifest ids = %v, want the entry written during the upload and the new one", ids)
	}
}

func TestObjectUploadManifestSaveFailureStillReportsTheObjectAndExitsOne(t *testing.T) {
	fake := startFake(t)
	objectScene(fake)
	dir := sceneDir(t, "object-gltf")
	locked := t.TempDir()
	makeReadOnly(t, locked)
	target := filepath.Join(locked, "m.json")

	got := runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID,
		"--object-id", "c1", "--manifest", target)
	if got.code != 1 || got.stdout != "Object ID: c1\n" || !strings.Contains(got.stderr, "manifest could not be saved") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if len(postsTo(fake)) != 1 {
		t.Errorf("POSTs = %+v", postsTo(fake))
	}

	got = runCommand(t, "object", "upload", "--object-dir", dir, "--object-filename", "cube", "--scene-id", objectSceneID,
		"--object-id", "c1", "--manifest", target, "--format", "json")
	result := decodeObject(t, got.stdout)
	if got.code != 1 || result["objectId"] != "c1" || result["manifestUploaded"] != false {
		t.Errorf("exit %d, result %v", got.code, result)
	}
}
