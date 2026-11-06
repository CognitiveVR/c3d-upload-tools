package scene

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/localfile"
)

// copyFixture copies testdata/<name> into a fresh temporary directory.
func copyFixture(t *testing.T, name string) string {
	t.Helper()
	src := filepath.Join("..", "..", "testdata", name)
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		in, err := os.Open(filepath.Join(src, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out, err := os.Create(filepath.Join(dst, e.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if _, err := io.Copy(out, in); err != nil {
			t.Fatal(err)
		}
		_ = in.Close()
		if err := out.Close(); err != nil {
			t.Fatal(err)
		}
	}
	return dst
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func fileSize(t *testing.T, path string) int64 {
	t.Helper()
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return info.Size()
}

func TestPlanGLTFSceneListsModelTexturesAndScreenshot(t *testing.T) {
	dir := copyFixture(t, "scene-gltf")
	plan, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Files.Paths) != 2 || filepath.Base(plan.Files.Paths[0]) != "scene.gltf" || filepath.Base(plan.Files.Paths[1]) != "scene.bin" {
		t.Errorf("model files = %v", plan.Files.Paths)
	}
	if want := []string{"texture1.png", "texture2.png", "texture3.png"}; !slices.Equal(plan.Textures, want) {
		t.Errorf("textures = %v, want %v", plan.Textures, want)
	}
	if filepath.Base(plan.Screenshot) != "screenshot.png" || !filepath.IsAbs(plan.Screenshot) {
		t.Errorf("screenshot = %q", plan.Screenshot)
	}
	var total int64
	for _, name := range []string{"scene.gltf", "scene.bin", "texture1.png", "texture2.png", "texture3.png", "screenshot.png"} {
		total += fileSize(t, filepath.Join(dir, name))
	}
	if plan.TotalBytes != total {
		t.Errorf("TotalBytes = %d, want %d", plan.TotalBytes, total)
	}
	if plan.Existing != nil || len(plan.Skipped) != 0 || len(plan.Warnings) != 0 {
		t.Errorf("existing %v, skipped %v, warnings %v; want none", plan.Existing, plan.Skipped, plan.Warnings)
	}
}

func TestPlanReadsExistingSettingsAndNeverUploadsTheFile(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	writeFile(t, dir, "settings.json", `{"sceneName":"Kept","scale":2}`)
	plan, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Existing == nil || plan.Existing.SceneName != "Kept" || plan.Existing.Scale != 2 {
		t.Errorf("existing = %+v", plan.Existing)
	}
	if len(plan.Textures) != 0 || len(plan.Skipped) != 0 {
		t.Errorf("textures %v, skipped %v; settings.json must be neither", plan.Textures, plan.Skipped)
	}
}

func TestPlanWarnsOncePerSkippedFile(t *testing.T) {
	dir := copyFixture(t, "scene-gltf")
	writeFile(t, dir, "notes.txt", "x")
	writeFile(t, dir, "texture.tga", "x")
	plan, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"notes.txt", "texture.tga"}; !slices.Equal(plan.Skipped, want) {
		t.Fatalf("skipped = %v, want %v", plan.Skipped, want)
	}
	if len(plan.Warnings) != 2 || !strings.Contains(plan.Warnings[0], "notes.txt") || !strings.Contains(plan.Warnings[1], "texture.tga") {
		t.Errorf("warnings = %q, want one per skipped file", plan.Warnings)
	}
}

func TestPlanRejectsBothModelLayouts(t *testing.T) {
	dir := copyFixture(t, "scene-gltf")
	writeFile(t, dir, "model.glb", "glTF")
	if _, err := Plan(dir); err == nil || !strings.Contains(err.Error(), "never both") {
		t.Errorf("err = %v, want the both-layouts error", err)
	}
}

func TestPlanRequiresScreenshotWithExactName(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	if err := os.Remove(filepath.Join(dir, "screenshot.png")); err != nil {
		t.Fatal(err)
	}
	_, err := Plan(dir)
	if err == nil || !strings.Contains(err.Error(), "screenshot.png") {
		t.Fatalf("err = %v, want it to name screenshot.png", err)
	}

	writeFile(t, dir, "Screenshot.PNG", "png")
	_, err = Plan(dir)
	if err == nil || !strings.Contains(err.Error(), "screenshot.png") || !strings.Contains(err.Error(), "Screenshot.PNG") {
		t.Errorf("err = %v, want it to name screenshot.png and the found Screenshot.PNG", err)
	}
}

func TestPlanRejectsASymlinkedScreenshot(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	target := filepath.Join(dir, "real.png")
	if err := os.Rename(filepath.Join(dir, "screenshot.png"), target); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(target, filepath.Join(dir, "screenshot.png")); err != nil {
		t.Skipf("cannot create a symbolic link here: %v", err)
	}
	_, err := Plan(dir)
	if err == nil || !strings.Contains(err.Error(), "screenshot.png is a symbolic link; it must be a regular file") {
		t.Errorf("err = %v, want the symbolic-link error", err)
	}
}

func TestPlanFailsTheModelCheckWithEveryError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "scene.gltf", `{"buffers":[{"uri":"scene.bin","byteLength":9}],"images":[{"uri":"missing.png"},{"uri":"data:image/png;base64,AA=="}]}`)
	writeFile(t, dir, "scene.bin", "1234")
	writeFile(t, dir, "screenshot.png", "png")
	_, err := Plan(dir)
	var check *CheckError
	if !errors.As(err, &check) {
		t.Fatalf("err = %v, want *CheckError", err)
	}
	if len(check.Errors) != 2 || !strings.Contains(check.Errors[0], "byteLength") || !strings.Contains(check.Errors[1], "missing.png") {
		t.Errorf("errors = %q, want byteLength and missing.png", check.Errors)
	}
	if len(check.Warnings) != 1 || !strings.Contains(check.Warnings[0], "data:") {
		t.Errorf("warnings = %q, want the data: uri warning", check.Warnings)
	}
}

func TestPlanKeepsModelCheckWarningsAndContinues(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "scene.gltf", `{"buffers":[{"uri":"scene.bin","byteLength":4}],"images":[{"uri":"data:image/png;base64,AA=="}]}`)
	writeFile(t, dir, "scene.bin", "1234")
	writeFile(t, dir, "screenshot.png", "png")
	plan, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "data:") {
		t.Errorf("warnings = %q", plan.Warnings)
	}
}

func TestPlanRejectsReservedConverterNameBesideGLB(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	writeFile(t, dir, "scene_0.png", "png")
	if _, err := Plan(dir); err == nil || !strings.Contains(err.Error(), "scene_0.png") || !strings.Contains(err.Error(), "reserved") {
		t.Errorf("err = %v, want the reserved-name error naming scene_0.png", err)
	}
	gltf := copyFixture(t, "scene-gltf")
	writeFile(t, gltf, "scene_0.png", "png")
	if _, err := Plan(gltf); err != nil {
		t.Errorf("beside a .gltf the name is not reserved: %v", err)
	}
}

func TestCheckSizesWarnsAtTwoGibibytesAndRefusesAboveThree(t *testing.T) {
	plan := &LocalPlan{TotalBytes: 2 << 30}
	if err := checkSizes(plan); err != nil {
		t.Fatalf("2 GiB: err = %v, want a warning only", err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "2 GiB") {
		t.Errorf("2 GiB: warnings = %q", plan.Warnings)
	}

	plan = &LocalPlan{TotalBytes: 2<<30 - 1}
	if err := checkSizes(plan); err != nil || len(plan.Warnings) != 0 {
		t.Errorf("under 2 GiB: err %v, warnings %q; want neither", err, plan.Warnings)
	}

	plan = &LocalPlan{TotalBytes: 3 << 30}
	if err := checkSizes(plan); err != nil {
		t.Errorf("exactly 3 GiB is accepted by the gateway: err = %v", err)
	}

	plan = &LocalPlan{TotalBytes: 31 << 30 / 10}
	err := checkSizes(plan)
	if err == nil || !strings.Contains(err.Error(), "3 GiB") {
		t.Errorf("3.1 GiB: err = %v, want the 3 GiB refusal", err)
	}
}

func TestCheckSizesWarnsPerFileAtFiveHundredMegabytes(t *testing.T) {
	plan := &LocalPlan{
		TotalBytes: 900 * 1000 * 1000,
		models:     []localfile.File{{Name: "scene.glb", Size: 500 * 1000 * 1000}},
		textures:   []localfile.File{{Name: "small.png", Size: 400 * 1000 * 1000}},
	}
	if err := checkSizes(plan); err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "scene.glb") || !strings.Contains(plan.Warnings[0], "500 MB") {
		t.Errorf("warnings = %q, want one naming scene.glb and 500 MB", plan.Warnings)
	}
}

func TestFinalizeOrdersPartsAndBuildsSettingsInMemory(t *testing.T) {
	dir := copyFixture(t, "scene-gltf")
	local, err := Plan(dir)
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Finalize(local, "My Scene", "cli-go-v9.9.9")
	if err != nil {
		t.Fatal(err)
	}
	type wantPart struct{ field, filename, contentType string }
	want := []wantPart{
		{"file", "scene.gltf", "application/octet-stream"},
		{"file", "scene.bin", "application/octet-stream"},
		{"file", "texture1.png", "application/octet-stream"},
		{"file", "texture2.png", "application/octet-stream"},
		{"file", "texture3.png", "application/octet-stream"},
		{"file", "settings.json", "application/json"},
		{"screenshot", "screenshot.png", "application/octet-stream"},
	}
	if len(plan.Parts) != len(want) {
		t.Fatalf("parts = %+v", plan.Parts)
	}
	for i, w := range want {
		p := plan.Parts[i]
		if p.Field != w.field || p.Filename != w.filename || p.ContentType != w.contentType {
			t.Errorf("part %d = %s %s %s, want %+v", i, p.Field, p.Filename, p.ContentType, w)
		}
		rc, err := p.Open()
		if err != nil {
			t.Fatal(err)
		}
		data, err := io.ReadAll(rc)
		_ = rc.Close()
		if err != nil || int64(len(data)) != p.Size {
			t.Errorf("part %d: read %d bytes (err %v), Size %d", i, len(data), err, p.Size)
		}
		if p.Filename == "settings.json" && string(data) != string(plan.SettingsBytes) {
			t.Errorf("settings part = %q, want SettingsBytes %q", data, plan.SettingsBytes)
		}
	}
	if !strings.Contains(string(plan.SettingsBytes), `"sceneName": "My Scene"`) || !strings.Contains(string(plan.SettingsBytes), `"sdkVersion": "cli-go-v9.9.9"`) {
		t.Errorf("settings = %s", plan.SettingsBytes)
	}
	if plan.SceneName != "My Scene" {
		t.Errorf("SceneName = %q", plan.SceneName)
	}
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("Finalize must not write settings.json: stat err %v", err)
	}
}

func TestFinalizeWithoutAnyNameIsErrNoSceneName(t *testing.T) {
	local, err := Plan(copyFixture(t, "scene-glb"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := Finalize(local, "", "cli-go-v1.0.0"); !errors.Is(err, ErrNoSceneName) {
		t.Errorf("err = %v, want ErrNoSceneName", err)
	}
}

func TestCheckSettingsWritableLeavesNothingBehind(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	before, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := CheckSettingsWritable(dir); err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "settings.json", `{"sceneName":"X"}`)
	if err := CheckSettingsWritable(dir); err != nil {
		t.Fatal(err)
	}
	if got := readSettings(t, dir); string(got) != `{"sceneName":"X"}` {
		t.Errorf("settings.json = %q, want it untouched", got)
	}
	after, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(after) != len(before)+1 {
		t.Errorf("directory has %d entries, want %d", len(after), len(before)+1)
	}
}

func TestCheckSettingsWritableRefusesAReadOnlyDirectory(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	makeReadOnly(t, dir)
	err := CheckSettingsWritable(dir)
	if err == nil || !strings.Contains(err.Error(), dir) || !strings.Contains(err.Error(), "the upload was not started") {
		t.Fatalf("err = %v, want the directory named and the upload not started", err)
	}
}

// A read-only settings.json blocks the upload only on Windows, where the
// rename cannot replace it; elsewhere the write replaces it.
func TestCheckSettingsWritableReadOnlySettingsFile(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	path := filepath.Join(dir, "settings.json")
	if err := os.WriteFile(path, []byte(`{"sceneName":"X"}`), 0o444); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(path, 0o644) })
	err := CheckSettingsWritable(dir)
	if runtime.GOOS != "windows" {
		if err != nil {
			t.Fatalf("err = %v, want a read-only settings.json accepted", err)
		}
		if err := WriteSettings(dir, []byte("{}\n")); err != nil {
			t.Fatalf("WriteSettings over a read-only settings.json: %v", err)
		}
		return
	}
	if err == nil || !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "the upload was not started") {
		t.Fatalf("err = %v, want settings.json named and the upload not started", err)
	}
}
