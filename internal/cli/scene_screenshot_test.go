package cli

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSceneScreenshotPostsOnePartToTheLatestVersion(t *testing.T) {
	fake := startFake(t)
	projectified := addListScenes(fake)
	body := append(append([]byte{}, pngSignature...), []byte("image-bytes")...)
	file := writeFile(t, "shot.png", body)

	got := runCommand(t, "scene", "screenshot", "--scene-id", projectified, "--file", file)
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	if got.stdout != "" || !strings.Contains(got.stderr, "[INFO] Screenshot replaced for version 3") {
		t.Errorf("stdout %q, stderr %q", got.stdout, got.stderr)
	}
	assertVerifyBeforeSceneGET(t, fake, projectified)
	sent := postsTo(fake)
	if len(sent) != 1 || sent[0].Path != "/v0/scenes/"+projectified+"/screenshot" || sent[0].Query.Get("version") != "3" {
		t.Fatalf("POSTs = %+v", sent)
	}
	if len(sent[0].Parts) != 1 {
		t.Fatalf("parts = %+v", sent[0].Parts)
	}
	part := sent[0].Parts[0]
	if part.Field != "screenshot" || part.Filename != "screenshot.png" || part.ContentType != "application/octet-stream" ||
		part.Size != int64(len(body)) || !bytes.Equal(part.Body, body) {
		t.Errorf("part = %+v", part)
	}
}

func TestSceneScreenshotVersionFlagAndJSONOutput(t *testing.T) {
	fake := startFake(t)
	projectified := addListScenes(fake)
	file := writeFile(t, "shot.png", pngSignature)
	got := runCommand(t, "scene", "screenshot", "--scene-id", projectified, "--file", file, "--version", "2", "--format", "json")
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	want := fmt.Sprintf(`{"sceneId":%q,"versionNumber":2,"screenshotReplaced":true}`, projectified)
	if strings.TrimSpace(got.stdout) != want {
		t.Errorf("stdout = %q, want %q", got.stdout, want)
	}
	if sent := postsTo(fake); len(sent) != 1 || sent[0].Query.Get("version") != "2" {
		t.Errorf("POSTs = %+v", sent)
	}
}

func TestSceneScreenshotUnknownVersionNamesTheKnownOnes(t *testing.T) {
	fake := startFake(t)
	projectified := addListScenes(fake)
	file := writeFile(t, "shot.png", pngSignature)
	got := runCommand(t, "scene", "screenshot", "--scene-id", projectified, "--file", file, "--version", "9")
	if got.code != 3 || !strings.Contains(got.stderr, "version 9") || !strings.Contains(got.stderr, "3, 2 and 1") {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	if len(postsTo(fake)) != 0 {
		t.Error("nothing should be posted")
	}
}

func TestSceneScreenshotSceneWithoutVersionsExitsThree(t *testing.T) {
	fake := startFake(t)
	addListScenes(fake)
	file := writeFile(t, "shot.png", pngSignature)
	got := runCommand(t, "scene", "screenshot", "--scene-id", "plain-scene", "--file", file)
	if got.code != 3 || !strings.Contains(got.stderr, "no versions") || len(postsTo(fake)) != 0 {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestSceneScreenshotUnknownSceneExitsThree(t *testing.T) {
	fake := startFake(t)
	file := writeFile(t, "shot.png", pngSignature)
	got := runCommand(t, "scene", "screenshot", "--scene-id", testSceneUUID, "--file", file)
	if got.code != 3 {
		t.Errorf("exit %d, stderr %q", got.code, got.stderr)
	}
	assertVerifyBeforeSceneGET(t, fake, testSceneUUID)
}

func TestSceneScreenshotRejectsBadFilesBeforeAnyRequest(t *testing.T) {
	fake := startFake(t)
	addListScenes(fake)
	dir := t.TempDir()
	link := filepath.Join(dir, "link.png")
	target := writeFile(t, "real.png", pngSignature)
	cases := map[string]string{
		"not a png":   writeFile(t, "shot.png", []byte("GIF89a not a png")),
		"too short":   writeFile(t, "short.png", pngSignature[:4]),
		"missing":     filepath.Join(dir, "missing.png"),
		"a directory": dir,
	}
	if os.Symlink(target, link) == nil {
		cases["a symlink"] = link
	}
	for name, file := range cases {
		got := runCommand(t, "scene", "screenshot", "--scene-id", "plain-scene", "--file", file)
		if got.code != 4 || got.stdout != "" || !strings.Contains(got.stderr, file) {
			t.Errorf("%s: exit %d, stdout %q, stderr %q", name, got.code, got.stdout, got.stderr)
		}
	}
	if got := runCommand(t, "scene", "screenshot", "--scene-id", "plain-scene", "--file", writeFile(t, "x.png", []byte("nope"))); !strings.Contains(got.stderr, "not a PNG") {
		t.Errorf("stderr = %q, want it to say the file is not a PNG", got.stderr)
	}
	if len(fake.Requests()) != 0 {
		t.Errorf("requests = %+v, want none", fake.Requests())
	}
}

func TestSceneScreenshotUploadsTheFileItCheckedWhenThePathIsSwappedForALink(t *testing.T) {
	fake := startFake(t)
	projectified := addListScenes(fake)
	body := append(append([]byte{}, pngSignature...), []byte("checked")...)
	file := writeFile(t, "shot.png", body)
	other := writeFile(t, "other.png", append(append([]byte{}, pngSignature...), []byte("swapped in")...))
	swapped := make(chan error, 1)
	fake.OnRoute("GET /v0/scenes/{sceneId}", func() {
		if err := os.Remove(file); err != nil {
			swapped <- err
			return
		}
		swapped <- os.Symlink(other, file)
	})

	got := runCommand(t, "scene", "screenshot", "--scene-id", projectified, "--file", file)
	select {
	case err := <-swapped:
		if err != nil {
			t.Skipf("cannot swap the file for a symlink here: %v", err)
		}
	default:
		t.Fatalf("the scene was never looked up: exit %d, stderr %q", got.code, got.stderr)
	}
	if got.code != 0 {
		t.Fatalf("exit %d, stderr %q", got.code, got.stderr)
	}
	sent := postsTo(fake)
	if len(sent) != 1 || len(sent[0].Parts) != 1 || !bytes.Equal(sent[0].Parts[0].Body, body) {
		t.Fatalf("POSTs = %+v, want the checked bytes", sent)
	}
}

func TestSceneScreenshotRequiresFileAndSceneID(t *testing.T) {
	startFake(t)
	if got := runCommand(t, "scene", "screenshot", "--scene-id", "x"); got.code != 4 || !strings.Contains(got.stderr, "--file") {
		t.Errorf("no file: exit %d, stderr %q", got.code, got.stderr)
	}
	file := writeFile(t, "shot.png", pngSignature)
	if got := runCommand(t, "scene", "screenshot", "--file", file); got.code != 4 || !strings.Contains(got.stderr, "--scene-id") {
		t.Errorf("no scene id: exit %d, stderr %q", got.code, got.stderr)
	}
}

func TestSceneScreenshotWithAnExpiredKeyExitsTwoBeforeAnySceneRequest(t *testing.T) {
	fake := startFake(t)
	t.Setenv("C3D_DEVELOPER_API_KEY", fake.ExpiredKey())
	file := writeFile(t, "shot.png", pngSignature)
	got := runCommand(t, "scene", "screenshot", "--scene-id", testSceneUUID, "--file", file)
	if got.code != 2 || got.stdout != "" || sceneRequests(fake) != 0 {
		t.Errorf("exit %d, stdout %q, scene requests %d, stderr %q", got.code, got.stdout, sceneRequests(fake), got.stderr)
	}
}

func TestSceneScreenshotRejectsVersionsBelowOneBeforeAnyRequest(t *testing.T) {
	fake := startFake(t)
	file := writeFile(t, "shot.png", pngSignature)
	for _, v := range []string{"0", "-1"} {
		got := runCommand(t, "scene", "screenshot", "--scene-id", "plain-scene", "--file", file, "--version="+v)
		if got.code != 4 || got.stdout != "" || !strings.Contains(got.stderr, "--version") {
			t.Errorf("--version=%s: exit %d, stdout %q, stderr %q", v, got.code, got.stdout, got.stderr)
		}
	}
	if len(fake.Requests()) != 0 {
		t.Errorf("requests = %+v, want none", fake.Requests())
	}
}
