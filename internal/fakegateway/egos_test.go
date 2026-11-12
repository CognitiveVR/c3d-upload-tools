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

type egoListing struct {
	SceneID string `json:"sceneId"`
	Version int    `json:"version"`
	Egos    []struct {
		Key       string          `json:"key"`
		Model     string          `json:"model"`
		Files     []string        `json:"files"`
		Manifest  json.RawMessage `json:"manifest"`
		Optimized bool            `json:"optimized"`
	} `json:"egos"`
}

// postEgo sends names in order, each under the field "file" with its content
// from files.
func postEgo(t *testing.T, s *Server, scene, slot string, version int, files map[string]string, names ...string) (int, string) {
	t.Helper()
	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	for _, name := range names {
		part, err := mw.CreateFormFile("file", name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = part.Write([]byte(files[name]))
	}
	_ = mw.Close()
	req, err := http.NewRequest(http.MethodPost, s.URL()+"/scenes/"+scene+"/egos/"+slot+"?version="+strconv.Itoa(version), &buf)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", header(s.Key()))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	var body bytes.Buffer
	_, _ = body.ReadFrom(resp.Body)
	return resp.StatusCode, body.String()
}

func listEgos(t *testing.T, s *Server, scene string, version int) egoListing {
	t.Helper()
	status, body := call(t, s, http.MethodGet, "/scenes/"+scene+"/egos?version="+strconv.Itoa(version), header(s.Key()), nil)
	if status != http.StatusOK {
		t.Fatalf("GET egos: %d %q", status, body)
	}
	var l egoListing
	if err := json.Unmarshal([]byte(body), &l); err != nil {
		t.Fatalf("body %q: %v", body, err)
	}
	return l
}

func TestEgoSlotsArePerSceneAndVersion(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-e", Versions: []Version{{ID: 1, VersionNumber: 1}, {ID: 2, VersionNumber: 2}}})
	files := map[string]string{"head.gltf": "{}", "head.bin": "b", "albedo.png": "p", "ego.json": `{"slot":"head"}`}
	status, body := postEgo(t, s, "scene-e", "head", 2, files, "head.gltf", "head.bin", "albedo.png", "ego.json")
	if status != http.StatusOK {
		t.Fatalf("POST: %d %q", status, body)
	}
	var slot map[string]any
	if err := json.Unmarshal([]byte(body), &slot); err != nil || slot["key"] != "head" || slot["model"] != "head.gltf" {
		t.Errorf("POST answer %q (%v)", body, err)
	}

	l := listEgos(t, s, "scene-e", 2)
	if l.SceneID != "scene-e" || l.Version != 2 || len(l.Egos) != 1 {
		t.Fatalf("listing = %+v", l)
	}
	e := l.Egos[0]
	if e.Key != "head" || e.Model != "head.gltf" || len(e.Files) != 4 || e.Optimized || string(e.Manifest) != `{"slot":"head"}` {
		t.Errorf("slot = %+v manifest %s", e, e.Manifest)
	}
	if other := listEgos(t, s, "scene-e", 1); len(other.Egos) != 0 {
		t.Errorf("version 1 = %+v", other)
	}

	postEgo(t, s, "scene-e", "head", 2, map[string]string{"new.glb": "g"}, "new.glb")
	l = listEgos(t, s, "scene-e", 2)
	if len(l.Egos) != 1 || l.Egos[0].Model != "new.glb" || len(l.Egos[0].Files) != 1 || string(l.Egos[0].Manifest) != "null" {
		t.Errorf("replaced slot = %+v", l.Egos)
	}

	for range 2 {
		status, body := call(t, s, http.MethodDelete, "/scenes/scene-e/egos/head?version=2", header(s.Key()), nil)
		if status != http.StatusOK || body != `{"deleted":true}` {
			t.Errorf("DELETE: %d %q", status, body)
		}
	}
	if l := listEgos(t, s, "scene-e", 2); len(l.Egos) != 0 {
		t.Errorf("after delete = %+v", l)
	}
}

func TestEgoListingSortsSlotsByKey(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-s", Versions: []Version{{ID: 1, VersionNumber: 1}}})
	for _, slot := range []string{"head", "custom", "controller_left"} {
		postEgo(t, s, "scene-s", slot, 1, map[string]string{"m.glb": "g"}, "m.glb")
	}
	l := listEgos(t, s, "scene-s", 1)
	var keys []string
	for _, e := range l.Egos {
		keys = append(keys, e.Key)
	}
	if got, want := strings.Join(keys, " "), "controller_left custom head"; got != want {
		t.Errorf("keys = %q, want %q", got, want)
	}
}

func TestEgoRoutesAnswerAnUnknownSceneWithAnEmpty401(t *testing.T) {
	s := New(t)
	for _, tc := range []struct{ method, path string }{
		{http.MethodGet, "/scenes/nope/egos?version=1"},
		{http.MethodDelete, "/scenes/nope/egos/head?version=1"},
	} {
		if status, body := call(t, s, tc.method, tc.path, header(s.Key()), nil); status != http.StatusUnauthorized || body != "" {
			t.Errorf("%s: %d %q", tc.method, status, body)
		}
	}
	if status, body := postEgo(t, s, "nope", "head", 1, map[string]string{"a.glb": "g"}, "a.glb"); status != http.StatusUnauthorized || body != "" {
		t.Errorf("POST: %d %q", status, body)
	}
}

func TestSetOptimizedAfterPollsFlipsOnTheNthListing(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-o", Versions: []Version{{ID: 1, VersionNumber: 1}}})
	s.SetOptimizedAfterPolls("scene-o", 1, "head", 3)
	postEgo(t, s, "scene-o", "head", 1, map[string]string{"h.glb": "g"}, "h.glb")
	for poll := 1; poll <= 4; poll++ {
		l := listEgos(t, s, "scene-o", 1)
		if want := poll >= 3; len(l.Egos) != 1 || l.Egos[0].Optimized != want {
			t.Errorf("poll %d: %+v, want optimized %v", poll, l.Egos, want)
		}
	}
}
