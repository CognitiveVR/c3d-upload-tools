package manifest

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

const bashManifest = `{
  "objects": [
    {
      "id": "cube",
      "mesh": "cube",
      "name": "cube",
      "scaleCustom": [
        1.0,
        1.0,
        1.0
      ],
      "initialPosition": [
        0.0,
        0.0,
        0.0
      ],
      "initialRotation": [
        0.0,
        0.0,
        0.0,
        1.0
      ]
    },
    {
      "id": "Lantern-PNG",
      "mesh": "Lantern",
      "name": "Lantern",
      "scaleCustom": [
        1.0,
        1.0,
        1.0
      ],
      "initialPosition": [
        0.0,
        0.0,
        0.0
      ],
      "initialRotation": [
        0.0,
        0.0,
        0.0,
        1.0
      ]
    }
  ]
}
`

func writeFile(t *testing.T, name, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func decodeAny(t *testing.T, data []byte) any {
	t.Helper()
	var v any
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("unmarshal %s: %v", data, err)
	}
	return v
}

func boolPtr(b bool) *bool        { return &b }
func floatPtr(f float64) *float64 { return &f }
func stringPtr(s string) *string  { return &s }

func validManifest() *Manifest {
	return &Manifest{Objects: []Entry{DefaultEntry("cube", "cube")}}
}

func wantErrNaming(t *testing.T, err error, parts ...string) {
	t.Helper()
	if err == nil {
		t.Fatalf("got nil error, want one naming %q", parts)
	}
	for _, p := range parts {
		if !strings.Contains(err.Error(), p) {
			t.Errorf("error %q does not name %q", err, p)
		}
	}
}

func TestRoundTripBashManifest(t *testing.T) {
	m, err := Load(writeFile(t, "scene_object_manifest.json", bashManifest))
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := m.Save(out); err != nil {
		t.Fatal(err)
	}
	saved := readFile(t, out)
	if want := strings.ReplaceAll(bashManifest, ".0", ""); string(saved) != want {
		t.Errorf("round trip is not byte-stable apart from number format:\n got %s\nwant %s", saved, want)
	}
	if got, want := decodeAny(t, saved), decodeAny(t, []byte(bashManifest)); !reflect.DeepEqual(got, want) {
		t.Errorf("round trip changed content:\n got %s\nwant %s", saved, bashManifest)
	}
	if !bytes.HasSuffix(saved, []byte("}\n")) || bytes.HasSuffix(saved, []byte("\n\n")) {
		t.Errorf("saved file does not end with exactly one newline: %q", saved)
	}
	if !bytes.HasPrefix(saved, []byte("{\n  \"objects\": [\n    {\n      \"id\": \"cube\",\n")) {
		t.Errorf("saved file is not indented by two spaces:\n%s", saved)
	}
	if bytes.Contains(saved, []byte("\t")) {
		t.Errorf("saved file contains a tab:\n%s", saved)
	}
}

func TestSaveKeyOrder(t *testing.T) {
	e := DefaultEntry("id-1", "cube")
	e.IsController = boolPtr(false)
	e.Scale = floatPtr(2)
	e.InitialScale = []float64{1, 1, 1}
	e.ExternalID = stringPtr("ext-42")
	e.FileType = stringPtr("gltf")
	path := filepath.Join(t.TempDir(), "m.json")
	if err := (&Manifest{Objects: []Entry{e}}).Save(path); err != nil {
		t.Fatal(err)
	}
	saved := readFile(t, path)
	keys := []string{"id", "mesh", "name", "scaleCustom", "initialPosition", "initialRotation", "isController", "scale", "initialScale", "externalId", "fileType"}
	last := -1
	for _, k := range keys {
		i := bytes.Index(saved, []byte(`"`+k+`":`))
		if i < 0 {
			t.Fatalf("key %q missing from:\n%s", k, saved)
		}
		if i < last {
			t.Errorf("key %q is out of order in:\n%s", k, saved)
		}
		last = i
	}
	if !bytes.Contains(saved, []byte(`"isController": false`)) {
		t.Errorf("isController is not the JSON boolean false:\n%s", saved)
	}
}

func TestIsControllerSerializesAsBoolean(t *testing.T) {
	src := writeFile(t, "m.json", `{"objects":[`+
		`{"id":"a","mesh":"a","name":"a","scaleCustom":[1,1,1],"initialPosition":[0,0,0],"initialRotation":[0,0,0,1],"isController":true},`+
		`{"id":"b","mesh":"b","name":"b","scaleCustom":[1,1,1],"initialPosition":[0,0,0],"initialRotation":[0,0,0,1],"isController":false}]}`)
	m, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := m.Save(out); err != nil {
		t.Fatal(err)
	}
	objects := decodeAny(t, readFile(t, out)).(map[string]any)["objects"].([]any)
	for i, want := range []bool{true, false} {
		value := objects[i].(map[string]any)["isController"]
		if got, ok := value.(bool); !ok || got != want {
			t.Errorf("objects[%d].isController = %#v, want JSON boolean %v", i, value, want)
		}
	}
}

func TestSaveOmitsUnsetOptionalFields(t *testing.T) {
	e := DefaultEntry("cube", "cube")
	e.ScaleCustom = nil
	e.InitialPosition = nil
	e.InitialRotation = nil
	path := filepath.Join(t.TempDir(), "m.json")
	if err := (&Manifest{Objects: []Entry{e}}).Save(path); err != nil {
		t.Fatal(err)
	}
	saved := readFile(t, path)
	for _, k := range []string{"scaleCustom", "initialPosition", "initialRotation", "isController", "scale", "initialScale", "externalId", "fileType"} {
		if bytes.Contains(saved, []byte(`"`+k+`":`)) {
			t.Errorf("unset key %q written:\n%s", k, saved)
		}
	}
	if bytes.Contains(saved, []byte("null")) {
		t.Errorf("saved file contains null:\n%s", saved)
	}
}

func TestSaveOmitsTransformsMissingFromLoadedFile(t *testing.T) {
	src := writeFile(t, "m.json", `{"objects":[{"id":"a","mesh":"a","name":"a"}]}`)
	m, err := Load(src)
	if err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "out.json")
	if err := m.Save(out); err != nil {
		t.Fatal(err)
	}
	if saved := readFile(t, out); bytes.Contains(saved, []byte("null")) {
		t.Errorf("saved file contains null:\n%s", saved)
	}
}

func TestSaveEmptyManifestWritesEmptyArray(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.json")
	if err := (&Manifest{}).Save(path); err != nil {
		t.Fatal(err)
	}
	if saved := readFile(t, path); string(saved) != "{\n  \"objects\": []\n}\n" {
		t.Errorf("saved = %q", saved)
	}
}

func TestSaveDoesNotEscapeHTML(t *testing.T) {
	path := filepath.Join(t.TempDir(), "m.json")
	if err := (&Manifest{Objects: []Entry{DefaultEntry("a<b>&c", "mesh")}}).Save(path); err != nil {
		t.Fatal(err)
	}
	if saved := readFile(t, path); !bytes.Contains(saved, []byte(`"a<b>&c"`)) {
		t.Errorf("id was HTML-escaped:\n%s", saved)
	}
}

func TestSaveReplacesFileAndLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "m.json")
	if err := os.WriteFile(path, []byte("old"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := validManifest().Save(path); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "m.json" {
		t.Errorf("directory holds %v, want only m.json", entries)
	}
	got, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, validManifest()) {
		t.Errorf("reloaded %+v", got)
	}
}

func TestSaveFailureLeavesNoTemporaryFile(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "m.json")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	err := validManifest().Save(target)
	wantErrNaming(t, err, target)
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Errorf("directory holds %v, want only the m.json directory", entries)
	}
}

func TestLoadMissingFileNamesPath(t *testing.T) {
	path := filepath.Join(t.TempDir(), "absent_object_manifest.json")
	_, err := Load(path)
	wantErrNaming(t, err, path)
}

func TestLoadOrEmpty(t *testing.T) {
	m, err := LoadOrEmpty(filepath.Join(t.TempDir(), "absent.json"))
	if err != nil {
		t.Fatal(err)
	}
	if m == nil || len(m.Objects) != 0 {
		t.Errorf("LoadOrEmpty of a missing file = %+v, want an empty manifest", m)
	}

	if _, err := LoadOrEmpty(writeFile(t, "bad.json", "{")); err == nil {
		t.Error("LoadOrEmpty of malformed JSON returned no error")
	}

	m, err = LoadOrEmpty(writeFile(t, "ok.json", bashManifest))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 2 {
		t.Errorf("LoadOrEmpty of an existing file returned %d objects, want 2", len(m.Objects))
	}
}

func TestLoadWithoutObjectsKeyIsEmpty(t *testing.T) {
	m, err := Load(writeFile(t, "m.json", "{}"))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 0 {
		t.Errorf("objects = %+v, want none", m.Objects)
	}
}

func TestLoadRejectsObjectList(t *testing.T) {
	path := writeFile(t, "list.json", "\n  [{\"sdkId\":\"a\"}]")
	_, err := Load(path)
	wantErrNaming(t, err, path, "object list --write-manifest")
}

func TestLoadRejectsNonObjectTopLevel(t *testing.T) {
	for _, content := range []string{"null", `"text"`, "42"} {
		path := writeFile(t, "m.json", content)
		_, err := Load(path)
		wantErrNaming(t, err, path)
	}
}

func TestLoadAcceptsByteOrderMark(t *testing.T) {
	m, err := Load(writeFile(t, "bom.json", "\xef\xbb\xbf"+bashManifest))
	if err != nil {
		t.Fatal(err)
	}
	if len(m.Objects) != 2 {
		t.Errorf("got %d objects, want 2", len(m.Objects))
	}
}

func TestLoadMalformedNamesPath(t *testing.T) {
	path := writeFile(t, "m.json", `{"objects": "nope"}`)
	_, err := Load(path)
	wantErrNaming(t, err, path)
}

func TestDefaultEntry(t *testing.T) {
	got := DefaultEntry("id-1", "cube")
	want := Entry{
		ID: "id-1", Mesh: "cube", Name: "cube",
		ScaleCustom:     []float64{1, 1, 1},
		InitialPosition: []float64{0, 0, 0},
		InitialRotation: []float64{0, 0, 0, 1},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("DefaultEntry = %+v, want %+v", got, want)
	}
}

func TestDefaultPath(t *testing.T) {
	const id = "00000000-0000-4000-8000-000000000001"
	if got, want := DefaultPath(id), id+"_object_manifest.json"; got != want {
		t.Errorf("DefaultPath = %q, want %q", got, want)
	}
}

func TestUpsertExistingIDChangesOnlyMesh(t *testing.T) {
	existing := Entry{
		ID: "Lantern-PNG", Mesh: "Lantern", Name: "Front lantern",
		ScaleCustom:     []float64{2, 2, 2},
		InitialPosition: []float64{1, 2, 3},
		InitialRotation: []float64{0, 0.7, 0, 0.7},
		IsController:    boolPtr(false),
		Scale:           floatPtr(3),
		InitialScale:    []float64{1.5, 1.5, 1.5},
		ExternalID:      stringPtr("ext-42"),
		FileType:        stringPtr("gltf"),
	}
	m := &Manifest{Objects: []Entry{DefaultEntry("cube", "cube"), existing}}
	if got := m.Upsert("lantern-png", "LanternV2"); got != Updated {
		t.Fatalf("Upsert = %v, want %v", got, Updated)
	}
	if len(m.Objects) != 2 {
		t.Fatalf("got %d objects, want 2", len(m.Objects))
	}
	want := Entry{
		ID: "Lantern-PNG", Mesh: "LanternV2", Name: "Front lantern",
		ScaleCustom:     []float64{2, 2, 2},
		InitialPosition: []float64{1, 2, 3},
		InitialRotation: []float64{0, 0.7, 0, 0.7},
		IsController:    boolPtr(false),
		Scale:           floatPtr(3),
		InitialScale:    []float64{1.5, 1.5, 1.5},
		ExternalID:      stringPtr("ext-42"),
		FileType:        stringPtr("gltf"),
	}
	if !reflect.DeepEqual(m.Objects[1], want) {
		t.Errorf("updated entry = %+v, want %+v", m.Objects[1], want)
	}
	if !reflect.DeepEqual(m.Objects[0], DefaultEntry("cube", "cube")) {
		t.Errorf("other entry changed: %+v", m.Objects[0])
	}
}

func TestUpsertNewIDAppendsDefaults(t *testing.T) {
	m := validManifest()
	if got := m.Upsert("lantern-1", "Lantern"); got != Appended {
		t.Fatalf("Upsert = %v, want %v", got, Appended)
	}
	if len(m.Objects) != 2 {
		t.Fatalf("got %d objects, want 2", len(m.Objects))
	}
	if !reflect.DeepEqual(m.Objects[1], DefaultEntry("lantern-1", "Lantern")) {
		t.Errorf("appended entry = %+v", m.Objects[1])
	}
}

func TestUpsertIntoEmptyManifest(t *testing.T) {
	var m Manifest
	if got := m.Upsert("a", "a"); got != Appended {
		t.Fatalf("Upsert = %v, want %v", got, Appended)
	}
	if len(m.Objects) != 1 {
		t.Errorf("got %d objects, want 1", len(m.Objects))
	}
}

func TestMergeOutcomeString(t *testing.T) {
	if Appended.String() != "appended" || Updated.String() != "updated" {
		t.Errorf("String() = %q, %q", Appended, Updated)
	}
}

func TestIDForMesh(t *testing.T) {
	m := &Manifest{Objects: []Entry{
		DefaultEntry("cube-1", "cube"),
		DefaultEntry("lantern-1", "Lantern"),
		DefaultEntry("lantern-2", "Lantern"),
	}}
	if id, ok := m.IDForMesh("Lantern"); !ok || id != "lantern-1" {
		t.Errorf("IDForMesh(Lantern) = %q, %v, want lantern-1, true", id, ok)
	}
	if id, ok := m.IDForMesh("lantern"); ok {
		t.Errorf("IDForMesh(lantern) = %q, true, want no match", id)
	}
	if _, ok := m.IDForMesh("sphere"); ok {
		t.Error("IDForMesh(sphere) found a match")
	}
}

func TestValidateID(t *testing.T) {
	for _, id := range []string{"a", "Lantern-PNG", "00000000-0000-4000-8000-000000000001", "with space ~!", strings.Repeat("x", 255)} {
		if err := ValidateID(id); err != nil {
			t.Errorf("ValidateID(%q) = %v, want nil", id, err)
		}
	}
	invalid := map[string]string{
		"empty":     "",
		"256 chars": strings.Repeat("x", 256),
		"non-ASCII": "café",
		"emoji":     "tree-\U0001F384",
		"tab":       "a\tb",
		"newline":   "a\nb",
		"DEL":       "a\x7fb",
	}
	for name, id := range invalid {
		if err := ValidateID(id); err == nil {
			t.Errorf("ValidateID(%s) = nil, want an error", name)
		}
	}
}

func TestValidateAcceptsValidManifest(t *testing.T) {
	e := DefaultEntry("id-1", "café")
	e.Name = strings.Repeat("é", 255)
	e.InitialScale = []float64{1, 1, 1}
	m := &Manifest{Objects: []Entry{e, DefaultEntry("id-2", "cube")}}
	if err := m.Validate(); err != nil {
		t.Errorf("Validate = %v, want nil", err)
	}
	if err := (&Manifest{}).Validate(); err != nil {
		t.Errorf("Validate of an empty manifest = %v, want nil", err)
	}
}

func TestValidateRejects(t *testing.T) {
	cases := []struct {
		name   string
		mutate func(*Entry)
		parts  []string
	}{
		{"short scaleCustom", func(e *Entry) { e.ScaleCustom = []float64{2, 2} }, []string{"scaleCustom", "[2 2]"}},
		{"short initialScale", func(e *Entry) { e.InitialScale = []float64{1, 1} }, []string{"initialScale", "[1 1]"}},
		{"long initialPosition", func(e *Entry) { e.InitialPosition = []float64{0, 0, 0, 0} }, []string{"initialPosition"}},
		{"short initialRotation", func(e *Entry) { e.InitialRotation = []float64{0, 0, 1} }, []string{"initialRotation", "4"}},
		{"256-character name", func(e *Entry) { e.Name = strings.Repeat("n", 256) }, []string{"name"}},
		{"name with emoji", func(e *Entry) { e.Name = "tree \U0001F384" }, []string{"name"}},
		{"empty name", func(e *Entry) { e.Name = "" }, []string{"name"}},
		{"empty mesh", func(e *Entry) { e.Mesh = "" }, []string{"mesh"}},
		{"mesh with control character", func(e *Entry) { e.Mesh = "a\x01" }, []string{"mesh"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			e := DefaultEntry("obj-7", "cube")
			tc.mutate(&e)
			err := (&Manifest{Objects: []Entry{DefaultEntry("other", "other"), e}}).Validate()
			wantErrNaming(t, err, append([]string{`"obj-7"`}, tc.parts...)...)
			if err != nil && strings.Contains(err.Error(), `"other"`) {
				t.Errorf("error %q blames the valid entry", err)
			}
		})
	}
}

func TestValidateRejectsInvalidID(t *testing.T) {
	long := strings.Repeat("x", 256)
	wantErrNaming(t, (&Manifest{Objects: []Entry{DefaultEntry(long, "cube")}}).Validate(), long, "id")
	wantErrNaming(t, (&Manifest{Objects: []Entry{DefaultEntry("café", "cube")}}).Validate(), "café", "id")
}

func TestValidateRejectsIDsThatDifferOnlyInCase(t *testing.T) {
	m := &Manifest{Objects: []Entry{DefaultEntry("Cube", "cube"), DefaultEntry("other", "other"), DefaultEntry("cube", "cube")}}
	wantErrNaming(t, m.Validate(), `"Cube"`, `"cube"`, "id")
}

func TestValidateReportsEveryProblem(t *testing.T) {
	a := DefaultEntry("a", "")
	b := DefaultEntry("b", "b")
	b.ScaleCustom = []float64{1}
	wantErrNaming(t, (&Manifest{Objects: []Entry{a, b}}).Validate(), `"a"`, "mesh", `"b"`, "scaleCustom")
}
