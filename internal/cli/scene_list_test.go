package cli

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/fakegateway"
)

func TestSceneListTextShowsGatewayIDOnlyForProjectifiedScenes(t *testing.T) {
	fake := startFake(t)
	addListScenes(fake)
	got := runCommand(t, "scene", "list")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	lines := strings.Split(strings.TrimRight(got.stdout, "\n"), "\n")
	if len(lines) != 3 {
		t.Fatalf("stdout = %q, want a header and two rows", got.stdout)
	}
	if g := strings.Join(strings.Fields(lines[0]), " "); g != "ID GATEWAY ID NAME LATEST SDK VERSION" {
		t.Errorf("header = %q", lines[0])
	}
	if g := strings.Join(strings.Fields(lines[1]), " "); g != testSceneUUID+" p12-"+testSceneUUID+" Lobby 3 cli-go-v1.3.0" {
		t.Errorf("projectified row = %q", lines[1])
	}
	if g := strings.Join(strings.Fields(lines[2]), " "); g != "plain-scene Plain" {
		t.Errorf("plain row = %q, want no gateway id, latest or sdk version", lines[2])
	}
	if getsTo(fake, "/v0/apiKeys/verify") != 1 {
		t.Errorf("list should preflight once: %+v", fake.Requests())
	}
}

func TestSceneListJSONPrintsTheArrayWithStringIDs(t *testing.T) {
	fake := startFake(t)
	projectified := addListScenes(fake)
	got := runCommand(t, "scene", "list", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	var scenes []struct {
		ID          string `json:"id"`
		SDKFacingID string `json:"sdkFacingId"`
		SceneName   string `json:"sceneName"`
		Versions    []struct {
			ID            int64  `json:"id"`
			VersionNumber int    `json:"versionNumber"`
			SDKVersion    string `json:"sdkVersion"`
		} `json:"versions"`
	}
	if err := json.Unmarshal([]byte(got.stdout), &scenes); err != nil {
		t.Fatalf("stdout is not a JSON array: %v: %q", err, got.stdout)
	}
	if len(scenes) != 2 || scenes[0].ID != projectified || scenes[0].SDKFacingID != testSceneUUID || len(scenes[0].Versions) != 3 || scenes[1].Versions == nil {
		t.Errorf("scenes = %+v", scenes)
	}
}

func TestSceneListWithAnExpiredKeyExitsTwo(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	got := runCommand(t, "scene", "list")
	if got.code != 2 || got.stdout != "" || !strings.Contains(got.stderr, "Your developer API key has expired.") {
		t.Errorf("exit %d, stdout %q, stderr %q", got.code, got.stdout, got.stderr)
	}
}

func TestSceneListReplacesControlCharactersFromTheGateway(t *testing.T) {
	fake := startFake(t)
	fake.AddScene(fakegateway.Scene{ID: "p1-abc", SDKFacingID: "abc", SceneName: "Bad\x1b[31m\tname",
		Versions: []fakegateway.Version{{ID: 1, VersionNumber: 1, SDKVersion: "v\x1b1"}}})
	got := runCommand(t, "scene", "list")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if strings.Contains(got.stdout, "\x1b") {
		t.Errorf("stdout carries an escape: %q", got.stdout)
	}
	lines := strings.Split(strings.TrimRight(got.stdout, "\n"), "\n")
	if len(lines) != 2 || !strings.Contains(lines[1], "Bad\uFFFD[31m\uFFFDname") || strings.Contains(lines[1], "\t") {
		t.Errorf("stdout = %q", got.stdout)
	}
}
