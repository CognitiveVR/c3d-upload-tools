package cli

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func tableRows(stdout string) []string {
	var rows []string
	for _, line := range strings.Split(strings.TrimRight(stdout, "\n"), "\n") {
		rows = append(rows, strings.Join(strings.Fields(line), " "))
	}
	return rows
}

func TestEgoListTextShowsEveryKnownSlotThenExtraKeys(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	for _, slot := range []string{"zz_extra", "head", "aa_extra"} {
		if got := egoUpload(t, slot); got.code != 0 {
			t.Fatalf("upload %s: exit %d, stderr %q", slot, got.code, got.stderr)
		}
	}
	got := runCommand(t, "ego", "list", "--scene-id", egoSceneID)
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	want := []string{
		"SLOT MODEL FILES MANIFEST OPTIMIZED",
		"head head.gltf 4 true false",
		"controller_left empty",
		"controller_right empty",
		"aa_extra head.gltf 4 true false",
		"zz_extra head.gltf 4 true false",
	}
	if rows := tableRows(got.stdout); !reflect.DeepEqual(rows, want) {
		t.Errorf("rows = %q, want %q", rows, want)
	}
	if strings.Contains(got.stdout, " \n") {
		t.Errorf("stdout has trailing spaces: %q", got.stdout)
	}
	assertVerifyBeforeSceneGET(t, fake, egoSceneID)
	for _, r := range fake.Requests() {
		if r.Path == "/v0/scenes/"+egoSceneID+"/egos" && r.Query.Get("version") != "2" {
			t.Errorf("listed version %q, want the latest, 2", r.Query.Get("version"))
		}
	}
}

func TestEgoListJSONHoldsOnlyTheSlotsTheGatewayReturned(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	if got := egoUpload(t, "controller_right", "--version", "1"); got.code != 0 {
		t.Fatalf("upload: exit %d, stderr %q", got.code, got.stderr)
	}
	got := runCommand(t, "ego", "list", "--scene-id", egoSceneID, "--version", "1", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	want := map[string]any{
		"sceneId": egoSceneID, "versionNumber": 1.0,
		"egos": []any{map[string]any{
			"slot": "controller_right", "model": "head.gltf",
			"files":    []any{"head.gltf", "head.bin", "head_albedo.png", "ego.json"},
			"manifest": true, "optimized": false,
		}},
	}
	if result := decodeOne(t, got.stdout); !reflect.DeepEqual(result, want) {
		t.Errorf("result = %v, want %v", result, want)
	}

	empty := runCommand(t, "ego", "list", "--scene-id", egoSceneID, "--format", "json")
	if egos, ok := decodeOne(t, empty.stdout)["egos"].([]any); !ok || len(egos) != 0 {
		t.Errorf("version 2 = %q, want an empty egos array", empty.stdout)
	}
}

func TestEgoListUnknownVersionExitsThreeNamingTheKnownOnes(t *testing.T) {
	fake := startFake(t)
	egoScene(fake)
	got := runCommand(t, "ego", "list", "--scene-id", egoSceneID, "--version", "7")
	if got.code != 3 || got.stdout != "" || !strings.Contains(got.stderr, "has no version 7; its versions are 2 and 1") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	if egoPolls(fake) != 0 {
		t.Errorf("listed an unknown version")
	}
}

func TestEgoListUnknownSceneExitsThreeAfterPreflight(t *testing.T) {
	fake := startFake(t)
	got := runCommand(t, "ego", "list", "--scene-id", egoSceneID)
	if got.code != 3 || got.stdout != "" || !strings.Contains(got.stderr, "Scene "+egoSceneID+" was not found") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
	assertVerifyBeforeSceneGET(t, fake, egoSceneID)
}

func TestEgoListRejectsBadInputBeforeAnyRequest(t *testing.T) {
	fake := startFake(t)
	for _, args := range [][]string{{"ego", "list"}, {"ego", "list", "--scene-id", egoSceneID, "--version", "0"}} {
		if got := runCommand(t, args...); got.code != 4 {
			t.Errorf("%v: exit %d, stderr %q", args, got.code, got.stderr)
		}
	}
	if n := requestCount(fake); n != 0 {
		t.Errorf("%d requests, want 0", n)
	}
}

func TestEgoListReplacesControlCharactersFromTheGateway(t *testing.T) {
	isolate(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/v0/apiKeys/verify":
		case "/v0/scenes/s":
			_, _ = w.Write([]byte(`{"id":"s","sdkFacingId":"s","sceneName":"S","versions":[{"id":1,"versionNumber":1}]}`))
		case "/v0/scenes/s/egos":
			_, _ = w.Write([]byte(`{"sceneId":"s","version":1,"egos":[{"key":"bad\u001b[2Jkey","model":"m\u0007.glb","files":[],"manifest":null,"optimized":false}]}`))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("C3D_UPLOAD_GATEWAY_URL", srv.URL+"/v0")
	t.Setenv("C3D_DEVELOPER_API_KEY", "any-key")
	got := runCommand(t, "ego", "list", "--scene-id", "s")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if strings.ContainsAny(got.stdout, "\x1b\x07") || !strings.Contains(got.stdout, "bad�[2Jkey") || !strings.Contains(got.stdout, "m�.glb") {
		t.Errorf("stdout = %q", got.stdout)
	}
}
