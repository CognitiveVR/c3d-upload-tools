package fakegateway

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
	"testing"
	"time"
)

func call(t *testing.T, s *Server, method, path, auth string, body io.Reader) (int, string) {
	t.Helper()
	req, err := http.NewRequest(method, s.URL()+path, body)
	if err != nil {
		t.Fatal(err)
	}
	if auth != "" {
		req.Header.Set("Authorization", auth)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(data)
}

func header(key string) string { return "APIKEY:DEVELOPER " + key }

func TestKeysAreRandomAndDistinct(t *testing.T) {
	a, b := New(t), New(t)
	if a.Key() == a.ExpiredKey() || a.Key() == b.Key() || a.ExpiredKey() == b.ExpiredKey() {
		t.Fatalf("keys not distinct: %q %q %q %q", a.Key(), a.ExpiredKey(), b.Key(), b.ExpiredKey())
	}
}

func TestURLEndsWithAPIVersion(t *testing.T) {
	if s := New(t); !strings.HasSuffix(s.URL(), "/v0") {
		t.Fatalf("URL %q does not end with /v0", s.URL())
	}
}

func TestVerifyAuth(t *testing.T) {
	s := New(t)
	tests := []struct {
		name, auth string
		status     int
		body       string
	}{
		{"valid", header(s.Key()), 200, ""},
		{"expired", header(s.ExpiredKey()), 401, "Key expired."},
		{"unknown", header("fake-unknown"), 401, "Nonexistent API key."},
		{"wrong scheme", "Bearer " + s.Key(), 401, "Nonexistent API key."},
		{"missing", "", 401, "Missing API key."},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			status, body := call(t, s, "GET", "/apiKeys/verify", tt.auth, nil)
			if status != tt.status || body != tt.body {
				t.Fatalf("got %d %q, want %d %q", status, body, tt.status, tt.body)
			}
		})
	}
}

func TestAccountRoutesRejectExpiredKey(t *testing.T) {
	s := New(t)
	for _, route := range []struct{ method, path string }{{"GET", "/scenes"}, {"POST", "/scenes"}, {"GET", "/user"}} {
		status, body := call(t, s, route.method, route.path, header(s.ExpiredKey()), nil)
		if status != 401 || body != "Key expired." {
			t.Errorf("%s %s: got %d %q", route.method, route.path, status, body)
		}
	}
}

func TestSceneScopedRoutesAcceptExpiredKey(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-a", SceneName: "A"})
	for _, route := range []struct {
		method, path string
		want         int
	}{{"GET", "/scenes/scene-a", 200}, {"POST", "/scenes/scene-a", 201}, {"POST", "/objects/scene-a", 200}} {
		status, _ := call(t, s, route.method, route.path, header(s.ExpiredKey()), nil)
		if status != route.want {
			t.Errorf("%s %s with expired key: got %d, want %d", route.method, route.path, status, route.want)
		}
	}
	if status, body := call(t, s, "POST", "/scenes/scene-a", header("fake-unknown"), nil); status != 401 || body != "Nonexistent API key." {
		t.Errorf("unknown key on scene route: got %d %q", status, body)
	}
}

func TestSceneLookup(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-a", SDKFacingID: "scene-a", SceneName: "A", Versions: []Version{{ID: 7, VersionNumber: 2, SDKVersion: "cli-go-v1.0.0", Scale: 1}}})
	status, body := call(t, s, "GET", "/scenes/scene-a", header(s.Key()), nil)
	if status != 200 {
		t.Fatalf("got %d", status)
	}
	var got Scene
	if err := json.Unmarshal([]byte(body), &got); err != nil {
		t.Fatal(err)
	}
	if got.SceneName != "A" || len(got.Versions) != 1 || got.Versions[0].ID != 7 || got.Versions[0].VersionNumber != 2 {
		t.Fatalf("scene %+v", got)
	}
	if !strings.Contains(body, `"sdkFacingId":"scene-a"`) || !strings.Contains(body, `"versionNumber":2`) {
		t.Fatalf("wire names wrong: %s", body)
	}
	if status, body := call(t, s, "GET", "/scenes/unknown", header(s.Key()), nil); status != 401 || body != "" {
		t.Fatalf("unknown scene: got %d %q", status, body)
	}
}

func TestUser(t *testing.T) {
	s := New(t)
	s.SetUser(User{ProjectName: "P", OrganizationName: "O", KeyExpiresAt: 1700000000000})
	status, body := call(t, s, "GET", "/user", header(s.Key()), nil)
	if status != 200 || body != `{"projectName":"P","organizationName":"O","keyExpiresAt":1700000000000}` {
		t.Fatalf("got %d %s", status, body)
	}
}

func TestFailuresApplyToOneRequestEachInOrder(t *testing.T) {
	s := New(t)
	s.FailWith(503, "busy")
	s.FailWithHeaders(500, "boom", http.Header{"Retry-After": {"0"}})
	if status, body := call(t, s, "GET", "/anything", header(s.Key()), nil); status != 503 || body != "busy" {
		t.Fatalf("first: %d %q", status, body)
	}
	req, _ := http.NewRequest("GET", s.URL()+"/anything", nil)
	req.Header.Set("Authorization", header(s.Key()))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != 500 || resp.Header.Get("Retry-After") != "0" {
		t.Fatalf("second: %d %v", resp.StatusCode, resp.Header)
	}
	if status, _ := call(t, s, "GET", "/anything", header(s.Key()), nil); status != 200 {
		t.Fatalf("third: %d", status)
	}
}

func TestFailWithOverridesAuth(t *testing.T) {
	s := New(t)
	s.FailWith(413, "<html>too big</html>")
	if status, _ := call(t, s, "POST", "/scenes", "", nil); status != 413 {
		t.Fatalf("got %d", status)
	}
}

func TestDelayAppliesToNextRequest(t *testing.T) {
	s := New(t)
	s.Delay(150 * time.Millisecond)
	start := time.Now()
	call(t, s, "GET", "/apiKeys/verify", header(s.Key()), nil)
	if elapsed := time.Since(start); elapsed < 150*time.Millisecond {
		t.Fatalf("first request took %v", elapsed)
	}
	start = time.Now()
	call(t, s, "GET", "/apiKeys/verify", header(s.Key()), nil)
	if elapsed := time.Since(start); elapsed > 100*time.Millisecond {
		t.Fatalf("second request took %v", elapsed)
	}
}

func TestDelayEndsWhenClientGivesUp(t *testing.T) {
	s := New(t)
	s.Delay(time.Minute)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, "GET", s.URL()+"/apiKeys/verify", nil)
	if resp, err := http.DefaultClient.Do(req); err == nil {
		_ = resp.Body.Close()
		t.Fatal("expected the client deadline to fire")
	}
}

func TestRecordsRequests(t *testing.T) {
	s := New(t)
	call(t, s, "POST", "/objects/scene-a?version=3", header(s.Key()), strings.NewReader(`{"objects":[]}`))

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	w, _ := mw.CreateFormFile("file", "cube.glb")
	_, _ = w.Write([]byte("0123456789"))
	w, _ = mw.CreateFormFile("screenshot", "screenshot.png")
	_, _ = w.Write([]byte("png"))
	_ = mw.Close()
	req, _ := http.NewRequest("POST", s.URL()+"/scenes/scene-a", &buf)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", header(s.Key()))
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()

	got := s.Requests()
	if len(got) != 2 {
		t.Fatalf("recorded %d requests", len(got))
	}
	first := got[0]
	if first.Method != "POST" || first.Path != "/v0/objects/scene-a" || first.Query.Get("version") != "3" || string(first.Body) != `{"objects":[]}` {
		t.Fatalf("first %+v", first)
	}
	if !first.Complete || !got[1].Complete {
		t.Fatalf("complete bodies not marked complete: %v %v", first.Complete, got[1].Complete)
	}
	if first.Header.Get("Authorization") != header(s.Key()) {
		t.Fatalf("header not recorded: %v", first.Header)
	}
	parts := got[1].Parts
	if len(parts) != 2 || parts[0].Field != "file" || parts[0].Filename != "cube.glb" || parts[0].Size != 10 || string(parts[0].Body) != "0123456789" ||
		parts[0].ContentType != "application/octet-stream" ||
		parts[1].Field != "screenshot" || parts[1].Filename != "screenshot.png" || parts[1].Size != 3 {
		t.Fatalf("parts %+v", parts)
	}

	got[0].Header.Set("Authorization", "changed")
	got[0].Body[0] = 'X'
	again := s.Requests()
	if again[0].Header.Get("Authorization") != header(s.Key()) || again[0].Body[0] != '{' {
		t.Fatal("Requests did not return copies")
	}
}

func TestRecordsBodyLengthAndTransferEncoding(t *testing.T) {
	s := New(t)
	call(t, s, "POST", "/objects/scene-a", header(s.Key()), strings.NewReader(`{"objects":[]}`))
	call(t, s, "POST", "/objects/scene-a", header(s.Key()), io.MultiReader(strings.NewReader(`[]`)))
	got := s.Requests()
	if got[0].ContentLength != 14 || got[0].BodyBytes != 14 || len(got[0].TransferEncoding) != 0 {
		t.Errorf("sized body: length %d read %d encoding %v", got[0].ContentLength, got[0].BodyBytes, got[0].TransferEncoding)
	}
	if got[1].ContentLength != -1 || got[1].BodyBytes != 2 || len(got[1].TransferEncoding) != 1 || got[1].TransferEncoding[0] != "chunked" {
		t.Errorf("chunked body: length %d read %d encoding %v", got[1].ContentLength, got[1].BodyBytes, got[1].TransferEncoding)
	}
}

func getScene(t *testing.T, s *Server, id string) Scene {
	t.Helper()
	status, body := call(t, s, "GET", "/scenes/"+id, header(s.Key()), nil)
	if status != 200 {
		t.Fatalf("GET /scenes/%s: got %d %q", id, status, body)
	}
	var scene Scene
	if err := json.Unmarshal([]byte(body), &scene); err != nil {
		t.Fatal(err)
	}
	return scene
}

func versionNumbers(scene Scene) []int {
	var out []int
	for _, v := range scene.Versions {
		out = append(out, v.VersionNumber)
	}
	return out
}

func TestCreateSceneAnswers201WithNewIDAndVersionOne(t *testing.T) {
	s := New(t)
	status, id := call(t, s, "POST", "/scenes", header(s.Key()), nil)
	if status != 201 || len(id) != 36 {
		t.Fatalf("POST /scenes: got %d %q, want 201 and a UUID", status, id)
	}
	if _, again := call(t, s, "POST", "/scenes", header(s.Key()), nil); again == id {
		t.Errorf("two scenes got the same id %q", id)
	}
	scene := getScene(t, s, id)
	if scene.ID != id || scene.SDKFacingID != id || len(scene.Versions) != 1 || scene.Versions[0].VersionNumber != 1 || scene.Versions[0].ID == 0 {
		t.Errorf("scene %+v, want id %s with version 1", scene, id)
	}
	if status, body := call(t, s, "POST", "/scenes", header(s.ExpiredKey()), nil); status != 401 || body != "Key expired." {
		t.Errorf("expired key: got %d %q", status, body)
	}
}

func TestAddVersionAppendsNextNumberAtOnce(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-a", Versions: []Version{{ID: 5, VersionNumber: 1}, {ID: 9, VersionNumber: 3}, {ID: 7, VersionNumber: 2}}})
	if status, body := call(t, s, "POST", "/scenes/scene-a", header(s.Key()), nil); status != 201 || body != "" {
		t.Fatalf("POST /scenes/scene-a: got %d %q, want 201 empty", status, body)
	}
	scene := getScene(t, s, "scene-a")
	if got := versionNumbers(scene); len(got) != 4 || got[3] != 4 || scene.Versions[3].ID == 0 {
		t.Errorf("versions %+v, want 4 appended", scene.Versions)
	}
	if status, body := call(t, s, "POST", "/scenes/unknown", header(s.Key()), nil); status != 401 || body != "" {
		t.Errorf("unknown scene: got %d %q, want 401 empty", status, body)
	}
}

func TestSetNextUploadHidesVersionsForSomePolls(t *testing.T) {
	s := New(t)
	s.AddScene(Scene{ID: "scene-a", Versions: []Version{{ID: 5, VersionNumber: 1}}})
	s.AddScene(Scene{ID: "scene-b", Versions: []Version{{ID: 6, VersionNumber: 1}}})
	s.SetNextUpload("scene-a", 2, Version{ID: 50, VersionNumber: 2}, Version{ID: 51, VersionNumber: 3})

	if got := versionNumbers(getScene(t, s, "scene-a")); len(got) != 1 {
		t.Fatalf("before the POST: versions %v", got)
	}
	call(t, s, "POST", "/scenes/scene-b", header(s.Key()), nil)
	if got := versionNumbers(getScene(t, s, "scene-b")); len(got) != 2 {
		t.Errorf("another scene's POST must use the default: versions %v", got)
	}
	call(t, s, "POST", "/scenes/scene-a", header(s.Key()), nil)
	for poll := 1; poll <= 2; poll++ {
		if got := versionNumbers(getScene(t, s, "scene-a")); len(got) != 1 {
			t.Fatalf("poll %d: versions %v, want still hidden", poll, got)
		}
	}
	scene := getScene(t, s, "scene-a")
	if got := versionNumbers(scene); len(got) != 3 || got[1] != 2 || got[2] != 3 || scene.Versions[2].ID != 51 {
		t.Fatalf("third poll: versions %+v, want 2 and 3 visible", scene.Versions)
	}

	call(t, s, "POST", "/scenes/scene-a", header(s.Key()), nil)
	if got := versionNumbers(getScene(t, s, "scene-a")); len(got) != 4 || got[3] != 4 {
		t.Errorf("a planned upload applies once: versions %v", got)
	}
}
