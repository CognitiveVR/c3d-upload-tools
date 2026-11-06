package fakegateway

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http"
	"strconv"
	"strings"
	"testing"
)

func postObjectFile(t *testing.T, s *Server, scene, mesh string, version int, filename string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	part, err := mw.CreateFormFile("file", filename)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = part.Write([]byte("x"))
	_ = mw.Close()
	req, err := http.NewRequest("POST", s.URL()+"/objects/"+scene+"/"+mesh+"?version="+strconv.Itoa(version), &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", header(s.Key()))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
}

func postManifest(t *testing.T, s *Server, scene string, version int, body string) (int, string) {
	t.Helper()
	return call(t, s, "POST", "/objects/"+scene+"?version="+strconv.Itoa(version), header(s.Key()), strings.NewReader(body))
}

func listObjects(t *testing.T, s *Server, versionID string) []map[string]any {
	t.Helper()
	status, body := call(t, s, "GET", "/versions/"+versionID+"/objects", header(s.Key()), nil)
	if status != 200 {
		t.Fatalf("GET objects of %s: %d %q", versionID, status, body)
	}
	var rows []map[string]any
	if err := json.Unmarshal([]byte(body), &rows); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return rows
}

func TestObjectListFollowsManifestPostsAndPerVersionFiles(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-a", Versions: []Version{{ID: 11, VersionNumber: 1}, {ID: 12, VersionNumber: 2}}})
	postObjectFile(t, s, "scene-a", "cube", 1, "cube.gltf")
	postObjectFile(t, s, "scene-a", "cube", 1, "cube.bin")
	if rows := listObjects(t, s, "11"); len(rows) != 0 {
		t.Fatalf("rows before any manifest: %v", rows)
	}
	manifest := `{"objects":[{"id":"c1","mesh":"cube","name":"Cube","scaleCustom":[2,3,4],"initialPosition":[1,2,3],"initialRotation":[0,0,0,1]},{"id":"s1","mesh":"sphere","name":"Sphere"}]}`
	postObjectFile(t, s, "scene-a", "cube", 1, "cube.gltf")
	for _, version := range []int{1, 2} {
		if status, body := postManifest(t, s, "scene-a", version, manifest); status != 201 || body != "" {
			t.Fatalf("manifest v%d: %d %q", version, status, body)
		}
	}
	rows := listObjects(t, s, "11")
	if len(rows) != 2 || rows[0]["sdkId"] != "c1" || rows[0]["meshName"] != "cube" || rows[0]["scaleCustomY"] != 3.0 || rows[0]["initialPositionZ"] != 3.0 || rows[0]["versionId"] != 11.0 {
		t.Fatalf("rows = %v", rows)
	}
	files, _ := rows[0]["files"].([]any)
	if len(files) != 2 || files[0].(map[string]any)["type"] != "gltf" || files[1].(map[string]any)["type"] != "bin" {
		t.Errorf("cube files in version 1 = %v, want gltf and bin once each", rows[0]["files"])
	}
	if files, ok := rows[1]["files"].([]any); !ok || len(files) != 0 {
		t.Errorf("sphere files = %v, want an empty array", rows[1]["files"])
	}
	if rows[0]["initialScaleX"] != nil {
		t.Errorf("initialScaleX = %v, want null", rows[0]["initialScaleX"])
	}
	for _, row := range listObjects(t, s, "12") {
		if files, ok := row["files"].([]any); !ok || len(files) != 0 {
			t.Errorf("version 2 row %v has files from version 1", row)
		}
	}
}

func TestManifestPostUpsertsBySDKIDWithoutDeleting(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-a", Versions: []Version{{ID: 11, VersionNumber: 1}}})
	postManifest(t, s, "scene-a", 1, `{"objects":[{"id":"A","mesh":"m1","name":"one"},{"id":"b","mesh":"m2","name":"two"}]}`)
	postManifest(t, s, "scene-a", 1, `{"objects":[{"id":"a","mesh":"m1","name":"renamed"}]}`)
	rows := listObjects(t, s, "11")
	if len(rows) != 2 || rows[0]["name"] != "renamed" || rows[1]["sdkId"] != "b" {
		t.Errorf("rows = %v", rows)
	}
}

func TestManifestPostRejections(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-a", Versions: []Version{{ID: 11, VersionNumber: 1}}})
	tests := []struct {
		name, body string
		status     int
		text       string
	}{
		{"malformed", `{"objects":`, 400, "Invalid json."},
		{"short vector", `{"objects":[{"id":"a","mesh":"m","name":"n","scaleCustom":[1,1]}]}`, 400, "Invalid json."},
		{"duplicate ids", `{"objects":[{"id":"a","mesh":"m","name":"n"},{"id":"A","mesh":"m","name":"n"}]}`, 400, ""},
	}
	for _, tt := range tests {
		if status, body := postManifest(t, s, "scene-a", 1, tt.body); status != tt.status || body != tt.text {
			t.Errorf("%s: got %d %q, want %d %q", tt.name, status, body, tt.status, tt.text)
		}
	}
	if rows := listObjects(t, s, "11"); len(rows) != 0 {
		t.Errorf("rejected posts left rows: %v", rows)
	}
	if status, _ := call(t, s, "GET", "/versions/999/objects", header(s.Key()), nil); status != 404 {
		t.Errorf("unknown version id: %d, want 404", status)
	}
}
