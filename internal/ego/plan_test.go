package ego

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

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

func remove(t *testing.T, dir string, names ...string) {
	t.Helper()
	for _, n := range names {
		if err := os.Remove(filepath.Join(dir, n)); err != nil {
			t.Fatal(err)
		}
	}
}

func partNames(p *UploadPlan) []string {
	var names []string
	for _, part := range p.Parts {
		names = append(names, part.Field+":"+part.Filename+":"+part.ContentType)
	}
	return names
}

func mustFail(t *testing.T, dir, slot string, want ...string) error {
	t.Helper()
	_, err := Plan(dir, slot)
	if err == nil {
		t.Fatalf("Plan(%q) succeeded; want an error containing %q", slot, want)
	}
	text := err.Error()
	var ce *CheckError
	if errors.As(err, &ce) {
		text += " " + strings.Join(ce.Errors, " ")
	}
	for _, w := range want {
		if !strings.Contains(text, w) {
			t.Fatalf("error %q does not contain %q", text, w)
		}
	}
	return err
}

func TestPlanGLTFFixture(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	p, err := Plan(dir, "head")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{
		"file:head.gltf:application/octet-stream",
		"file:head.bin:application/octet-stream",
		"file:head_albedo.png:application/octet-stream",
		"file:ego.json:application/json",
	}
	if got := partNames(p); !slices.Equal(got, want) {
		t.Fatalf("parts = %v; want %v", got, want)
	}
	if p.Slot != "head" || !p.HasManifest || !slices.Equal(p.Images, []string{"head_albedo.png"}) {
		t.Fatalf("plan = %+v", p)
	}
	var total int64
	for _, part := range p.Parts {
		total += part.Size
	}
	if p.TotalBytes != total || total == 0 {
		t.Fatalf("TotalBytes = %d; parts sum to %d", p.TotalBytes, total)
	}
	if len(p.Warnings) != 0 {
		t.Fatalf("warnings = %v", p.Warnings)
	}
}

func TestPlanAcceptsAnyBaseName(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	for _, ext := range []string{".gltf", ".bin"} {
		if err := os.Rename(filepath.Join(dir, "head"+ext), filepath.Join(dir, "whatever"+ext)); err != nil {
			t.Fatal(err)
		}
	}
	b, err := os.ReadFile(filepath.Join(dir, "whatever.gltf"))
	if err != nil {
		t.Fatal(err)
	}
	writeFile(t, dir, "whatever.gltf", strings.ReplaceAll(string(b), "head.bin", "whatever.bin"))
	p, err := Plan(dir, "head")
	if err != nil {
		t.Fatal(err)
	}
	if got := partNames(p)[0]; got != "file:whatever.gltf:application/octet-stream" {
		t.Fatalf("first part = %s", got)
	}
}

func TestPlanGLB(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".glb") {
			remove(t, dir, e.Name())
		}
	}
	p, err := Plan(dir, "controller_left")
	if err != nil {
		t.Fatal(err)
	}
	if len(p.Parts) != 1 || p.HasManifest || len(p.Images) != 0 {
		t.Fatalf("plan = %+v", p)
	}
}

func TestPlanUnknownSlotWarns(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	p, err := Plan(dir, "camera_mount")
	if err != nil {
		t.Fatal(err)
	}
	want := `slot "camera_mount" is not one of the known slots head, controller_left, controller_right`
	if !slices.Contains(p.Warnings, want) {
		t.Fatalf("warnings = %v; want %q", p.Warnings, want)
	}
}

func TestPlanKnownSlotsDoNotWarn(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	for _, slot := range KnownSlots {
		p, err := Plan(dir, slot)
		if err != nil {
			t.Fatal(err)
		}
		if len(p.Warnings) != 0 {
			t.Fatalf("slot %s: warnings = %v", slot, p.Warnings)
		}
	}
}

func TestValidateSlot(t *testing.T) {
	for _, slot := range []string{"head", "controller_left", "a", "a-b_9", strings.Repeat("a", 64)} {
		if err := ValidateSlot(slot); err != nil {
			t.Errorf("ValidateSlot(%q) = %v", slot, err)
		}
	}
	for _, slot := range []string{"", "Head", "a b", "a/b", "..", "a.b", "é", strings.Repeat("a", 65), "a\nb"} {
		err := ValidateSlot(slot)
		if err == nil {
			t.Errorf("ValidateSlot(%q) accepted", slot)
			continue
		}
		if !strings.Contains(err.Error(), "use 1 to 64 lowercase letters, digits, '-' or '_'") {
			t.Errorf("ValidateSlot(%q) = %v", slot, err)
		}
	}
}

func TestPlanRejectsUppercaseSlot(t *testing.T) {
	mustFail(t, copyFixture(t, "ego-head"), "Head", `invalid slot "Head"`)
}

func TestPlanRejectsOtherFiles(t *testing.T) {
	for _, name := range []string{"model.obj", "photo.jpeg", "PHOTO.JPEG", "ego.json.gz", "notes.txt"} {
		t.Run(name, func(t *testing.T) {
			dir := copyFixture(t, "ego-head")
			writeFile(t, dir, name, "x")
			mustFail(t, dir, "head", name)
		})
	}
}

func TestPlanSkipsSystemFiles(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	for _, name := range []string{"desktop.ini", "Thumbs.db", ".DS_Store"} {
		writeFile(t, dir, name, "x")
	}
	p, err := Plan(dir, "head")
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range partNames(p) {
		if name == "desktop.ini" || name == "Thumbs.db" || name == ".DS_Store" {
			t.Errorf("parts include %s", name)
		}
	}
}

func TestPlanRejectsMiscasedManifestName(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	remove(t, dir, "ego.json")
	writeFile(t, dir, "EGO.JSON", "{}")
	mustFail(t, dir, "head", "EGO.JSON")
}

func TestPlanAcceptsImageExtensionsCaseInsensitively(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	writeFile(t, dir, "A.PNG", "x")
	writeFile(t, dir, "b.jpg", "x")
	writeFile(t, dir, "c.WebP", "x")
	p, err := Plan(dir, "head")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"A.PNG", "b.jpg", "c.WebP", "head_albedo.png"}
	if !slices.Equal(p.Images, want) {
		t.Fatalf("Images = %v; want %v", p.Images, want)
	}
}

func TestPlanEgoJSONLimit(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	writeFile(t, dir, "ego.json", strings.Repeat("x", 64*1024))
	if _, err := Plan(dir, "head"); err != nil {
		t.Fatalf("64 KiB ego.json rejected: %v", err)
	}
	writeFile(t, dir, "ego.json", strings.Repeat("x", 64*1024+1))
	mustFail(t, dir, "head", "ego.json", "64 KiB")
}

func TestPlanWithoutEgoJSON(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	remove(t, dir, "ego.json")
	p, err := Plan(dir, "head")
	if err != nil {
		t.Fatal(err)
	}
	if p.HasManifest {
		t.Fatal("HasManifest set without ego.json")
	}
}

func TestPlanRejectsSubdirectory(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	if err := os.Mkdir(filepath.Join(dir, "extras"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustFail(t, dir, "head", "extras", "subdirector")
}

func TestPlanRejectsReservedConverterName(t *testing.T) {
	dir := copyFixture(t, "scene-glb")
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !strings.HasSuffix(e.Name(), ".glb") {
			remove(t, dir, e.Name())
		}
	}
	writeFile(t, dir, "head_1.png", "x")
	mustFail(t, dir, "head", "head_1.png", "reserved")
	if _, err := Plan(dir, "controller_left"); err != nil {
		t.Fatalf("head_1.png is only reserved for slot head: %v", err)
	}
}

func TestPlanReservedNameBesideGLTFIsAllowed(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	writeFile(t, dir, "head_1.png", "x")
	if _, err := Plan(dir, "head"); err != nil {
		t.Fatal(err)
	}
}

func gltfWithImage(uri string) string {
	return `{"asset":{"version":"2.0"},"buffers":[{"uri":"head.bin","byteLength":840}],"images":[{"uri":"` + uri + `"}]}`
}

func TestPlanImageURIMissing(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	writeFile(t, dir, "head.gltf", gltfWithImage("missing.png"))
	err := mustFail(t, dir, "head", "missing.png")
	var ce *CheckError
	if !errors.As(err, &ce) || len(ce.Errors) == 0 {
		t.Fatalf("want a *CheckError, got %T", err)
	}
}

func TestPlanImageURICaseMismatch(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	writeFile(t, dir, "tex.png", "x")
	writeFile(t, dir, "head.gltf", gltfWithImage("Tex.png"))
	mustFail(t, dir, "head", "Tex.png")
}

func TestPlanImageURIPresent(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	writeFile(t, dir, "head.gltf", gltfWithImage("head_albedo.png"))
	if _, err := Plan(dir, "head"); err != nil {
		t.Fatal(err)
	}
}

func TestPlanImageURIJPEGIsAnUnlistedFile(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	writeFile(t, dir, "tex.jpeg", "x")
	writeFile(t, dir, "head.gltf", gltfWithImage("tex.jpeg"))
	err := mustFail(t, dir, "head", "tex.jpeg", "an egocentric model directory holds only")
	var ce *CheckError
	if errors.As(err, &ce) {
		t.Fatalf("want the unlisted-file error, got a model check failure: %v", ce.Errors)
	}
}

func TestPlanNeedsAModel(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	remove(t, dir, "head.gltf", "head.bin")
	mustFail(t, dir, "head", "no model")
}

func TestPlanRejectsSymlink(t *testing.T) {
	dir := copyFixture(t, "ego-head")
	if err := os.Symlink(filepath.Join(dir, "head_albedo.png"), filepath.Join(dir, "link.png")); err != nil {
		t.Skip("symlinks unavailable")
	}
	mustFail(t, dir, "head", "link.png")
}
