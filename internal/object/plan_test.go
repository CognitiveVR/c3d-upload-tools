package object

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
		copyFile(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
	}
	return dst
}

func copyFile(t *testing.T, from, to string) {
	t.Helper()
	in, err := os.Open(from)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = in.Close() }()
	out, err := os.Create(to)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(out, in); err != nil {
		t.Fatal(err)
	}
	if err := out.Close(); err != nil {
		t.Fatal(err)
	}
}

func writeFile(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func partNames(p *UploadPlan) []string {
	var names []string
	for _, part := range p.Parts {
		names = append(names, part.Field+":"+part.Filename)
	}
	return names
}

// glbDir returns a directory holding <name>.glb and a thumbnail.
func glbDir(t *testing.T, name string) string {
	t.Helper()
	dir := t.TempDir()
	copyFile(t, filepath.Join("..", "..", "testdata", "scene-glb", "scene.glb"), filepath.Join(dir, name+".glb"))
	copyFile(t, filepath.Join("..", "..", "testdata", "object-gltf", ThumbnailFileName), filepath.Join(dir, ThumbnailFileName))
	return dir
}

func TestPlanGLTFObjectListsModelThenThumbnailUnderFieldFile(t *testing.T) {
	dir := copyFixture(t, "object-gltf")
	plan, err := Plan(dir, "cube")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"file:cube.gltf", "file:cube.bin", "file:" + ThumbnailFileName}
	if got := partNames(plan); !slices.Equal(got, want) {
		t.Errorf("parts = %v, want %v", got, want)
	}
	if plan.Mesh != "cube" || len(plan.Warnings) != 0 {
		t.Errorf("mesh %q, warnings %v", plan.Mesh, plan.Warnings)
	}
	var total int64
	for _, p := range plan.Parts {
		total += p.Size
		if p.ContentType != "application/octet-stream" {
			t.Errorf("%s content type %q", p.Filename, p.ContentType)
		}
	}
	if plan.TotalBytes != total {
		t.Errorf("TotalBytes = %d, want %d", plan.TotalBytes, total)
	}
}

func TestPlanPutsTexturesBetweenModelAndThumbnailAndWarnsAboutSkippedFiles(t *testing.T) {
	dir := glbDir(t, "Chair")
	writeFile(t, dir, "wood.png", "x")
	writeFile(t, dir, "Albedo.JPG", "x")
	writeFile(t, dir, "notes.txt", "x")
	plan, err := Plan(dir, "Chair")
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"file:Chair.glb", "file:Albedo.JPG", "file:wood.png", "file:" + ThumbnailFileName}
	if got := partNames(plan); !slices.Equal(got, want) {
		t.Errorf("parts = %v, want %v", got, want)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "notes.txt") {
		t.Errorf("warnings = %v, want one naming notes.txt", plan.Warnings)
	}
}

func TestPlanMissingThumbnailNamesTheFile(t *testing.T) {
	dir := copyFixture(t, "object-gltf")
	if err := os.Remove(filepath.Join(dir, ThumbnailFileName)); err != nil {
		t.Fatal(err)
	}
	_, err := Plan(dir, "cube")
	if err == nil || !strings.Contains(err.Error(), ThumbnailFileName+" is missing") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanThumbnailMustMatchTheNameExactly(t *testing.T) {
	dir := copyFixture(t, "object-gltf")
	if err := os.Rename(filepath.Join(dir, ThumbnailFileName), filepath.Join(dir, "CVR_Object_Thumbnail.png")); err != nil {
		t.Fatal(err)
	}
	_, err := Plan(dir, "cube")
	if err == nil || !strings.Contains(err.Error(), "CVR_Object_Thumbnail.png") || !strings.Contains(err.Error(), "must be exactly") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanRejectsASymbolicLinkThumbnail(t *testing.T) {
	dir := copyFixture(t, "object-gltf")
	real := filepath.Join(dir, "real.png")
	if err := os.Rename(filepath.Join(dir, ThumbnailFileName), real); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(real, filepath.Join(dir, ThumbnailFileName)); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	_, err := Plan(dir, "cube")
	if err == nil || !strings.Contains(err.Error(), "symbolic link") {
		t.Fatalf("err = %v", err)
	}
}

func TestPlanObjectNameMustBeASafeSegment(t *testing.T) {
	dir := copyFixture(t, "object-gltf")
	for _, name := range []string{"", "..", "a/b", `a\b`} {
		if _, err := Plan(dir, name); err == nil || !strings.Contains(err.Error(), "invalid object filename") {
			t.Errorf("Plan(%q) err = %v", name, err)
		}
	}
}

func TestPlanModelNameMustMatchTheObjectName(t *testing.T) {
	dir := copyFixture(t, "object-gltf")
	if _, err := Plan(dir, "sphere"); err == nil {
		t.Error("a cube model is not the model of an object named sphere")
	}
}

func TestPlanRejectsATextureNamedLikeAConverterOutputBesideAGLB(t *testing.T) {
	dir := glbDir(t, "My Object (v2)")
	writeFile(t, dir, "My_Object__v2__0.png", "x")
	_, err := Plan(dir, "My Object (v2)")
	if err == nil || !strings.Contains(err.Error(), "My_Object__v2__0.png") || !strings.Contains(err.Error(), "reserved") {
		t.Fatalf("err = %v", err)
	}

	// The same name is fine for a different object.
	other := glbDir(t, "Chair")
	writeFile(t, other, "My_Object__v2__0.png", "x")
	if _, err := Plan(other, "Chair"); err != nil {
		t.Errorf("unrelated texture rejected: %v", err)
	}
}

func TestPlanModelCheckFailureIsACheckErrorWithWarnings(t *testing.T) {
	dir := glbDir(t, "Chair")
	writeFile(t, dir, "bad.txt", "x")
	if err := os.WriteFile(filepath.Join(dir, "Chair.glb"), []byte("not a glb at all, but long enough"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := Plan(dir, "Chair")
	var check *CheckError
	if !errors.As(err, &check) || len(check.Errors) == 0 {
		t.Fatalf("err = %v, want a CheckError", err)
	}
	if len(check.Warnings) != 1 || !strings.Contains(check.Warnings[0], "bad.txt") {
		t.Errorf("warnings = %v", check.Warnings)
	}
}

// sparseTexture adds a texture of the given size without writing its bytes.
func sparseTexture(t *testing.T, dir, name string, size int64) {
	t.Helper()
	writeFile(t, dir, name, "x")
	if err := os.Truncate(filepath.Join(dir, name), size); err != nil {
		t.Skipf("cannot make a sparse file: %v", err)
	}
}

func TestPlanWarnsAtTwoGiBAndRefusesAboveThree(t *testing.T) {
	dir := copyFixture(t, "object-gltf")
	sparseTexture(t, dir, "big.png", 2<<30)
	plan, err := Plan(dir, "cube")
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.Warnings) != 2 || !strings.Contains(plan.Warnings[0], "big.png") || !strings.Contains(plan.Warnings[1], "2 GiB") {
		t.Errorf("warnings = %q", plan.Warnings)
	}

	dir = copyFixture(t, "object-gltf")
	sparseTexture(t, dir, "big.png", 31<<30/10)
	if _, err := Plan(dir, "cube"); err == nil || !strings.Contains(err.Error(), "3 GiB") {
		t.Errorf("3.1 GiB: err = %v", err)
	}
}
