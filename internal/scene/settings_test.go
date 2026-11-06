package scene

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const testSDKVersion = "cli-go-v2.0.0"

func TestBuildSettingsWithoutExisting(t *testing.T) {
	data, got, err := BuildSettings(nil, "X", testSDKVersion)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"scale\": 1,\n  \"sceneName\": \"X\",\n  \"sdkVersion\": \"cli-go-v2.0.0\"\n}\n"
	if string(data) != want {
		t.Fatalf("data = %q, want %q", data, want)
	}
	if got.SceneName != "X" || got.Scale != 1 || got.SDKVersion != testSDKVersion {
		t.Fatalf("settings = %+v", got)
	}
}

func TestBuildSettingsKeepsExtraAndScale(t *testing.T) {
	dir := t.TempDir()
	body := `{"zeta":{"a":1},"sceneName":"Old","scale":2.5,"sdkVersion":"old","alpha":"a&b<c>"}`
	if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	existing, err := ReadSettings(dir)
	if err != nil || existing == nil {
		t.Fatalf("ReadSettings = %v, %v", existing, err)
	}
	data, got, err := BuildSettings(existing, "", testSDKVersion)
	if err != nil {
		t.Fatal(err)
	}
	want := "{\n  \"scale\": 2.5,\n  \"sceneName\": \"Old\",\n  \"sdkVersion\": \"cli-go-v2.0.0\",\n  \"alpha\": \"a&b<c>\",\n  \"zeta\": {\n    \"a\": 1\n  }\n}\n"
	if string(data) != want {
		t.Fatalf("data = %q, want %q", data, want)
	}
	if got.Scale != 2.5 || got.SceneName != "Old" || len(got.Extra) != 2 {
		t.Fatalf("settings = %+v", got)
	}
}

func TestBuildSettingsNameArgumentWins(t *testing.T) {
	_, got, err := BuildSettings(&Settings{SceneName: "Old", Scale: 1}, "New", testSDKVersion)
	if err != nil || got.SceneName != "New" {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestBuildSettingsZeroScaleBecomesOne(t *testing.T) {
	_, got, err := BuildSettings(&Settings{SceneName: "Old"}, "", testSDKVersion)
	if err != nil || got.Scale != 1 {
		t.Fatalf("got %+v, %v", got, err)
	}
}

func TestBuildSettingsNoName(t *testing.T) {
	for _, existing := range []*Settings{nil, {Scale: 1}} {
		if _, _, err := BuildSettings(existing, "", testSDKVersion); !errors.Is(err, ErrNoSceneName) {
			t.Fatalf("err = %v, want ErrNoSceneName", err)
		}
	}
}

func TestReadSettingsMissing(t *testing.T) {
	got, err := ReadSettings(t.TempDir())
	if got != nil || err != nil {
		t.Fatalf("got %v, %v", got, err)
	}
}

func TestReadSettingsMalformed(t *testing.T) {
	for _, body := range []string{"{not json", "[1,2]", `"text"`, ""} {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, "settings.json"), []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
		_, err := ReadSettings(dir)
		if err == nil || !strings.Contains(err.Error(), "settings.json") {
			t.Fatalf("body %q: err = %v", body, err)
		}
	}
}

func TestWriteSettingsWritesReturnedBytes(t *testing.T) {
	dir := t.TempDir()
	data, _, err := BuildSettings(nil, "X", testSDKVersion)
	if err != nil {
		t.Fatal(err)
	}
	if err := WriteSettings(dir, data); err != nil {
		t.Fatal(err)
	}
	onDisk, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	if string(onDisk) != string(data) {
		t.Fatalf("on disk %q, built %q", onDisk, data)
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("entries = %v, %v", entries, err)
	}
}

func TestWriteSettingsLeavesNoTempFileOnError(t *testing.T) {
	dir := t.TempDir()
	target := filepath.Join(dir, "settings.json")
	if err := os.Mkdir(target, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(target, "keep"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := WriteSettings(dir, []byte("{}\n")); err == nil {
		t.Fatal("expected rename onto a non-empty directory to fail")
	}
	entries, err := os.ReadDir(dir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("temp file left behind: %v, %v", entries, err)
	}
}
