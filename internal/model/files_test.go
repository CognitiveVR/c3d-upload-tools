package model

import (
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func mkDir(t *testing.T, names ...string) string {
	t.Helper()
	dir := t.TempDir()
	for _, n := range names {
		if err := os.WriteFile(filepath.Join(dir, n), []byte("x"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

func TestResolveAccepts(t *testing.T) {
	cases := []struct {
		name  string
		mode  Mode
		base  string
		files []string
		kind  Kind
		want  []string
	}{
		{"scene gltf pair", SceneMode, "scene", []string{"scene.gltf", "scene.bin", "screenshot.png"}, GLTFSeparate, []string{"scene.gltf", "scene.bin"}},
		{"scene glb of any name", SceneMode, "scene", []string{"MyRoom.glb", "screenshot.png"}, GLB, []string{"MyRoom.glb"}},
		{"scene upper-case extension", SceneMode, "scene", []string{"Room.GLB"}, GLB, []string{"Room.GLB"}},
		{"scene glb name with equals sign", SceneMode, "scene", []string{"Room=v2.glb"}, GLB, []string{"Room=v2.glb"}},
		{"scene upper-case gltf extension", SceneMode, "scene", []string{"scene.GLTF", "scene.BIN"}, GLTFSeparate, []string{"scene.GLTF", "scene.BIN"}},
		{"named gltf pair", NamedMode, "cube", []string{"cube.gltf", "cube.bin", "cvr_object_thumbnail.png"}, GLTFSeparate, []string{"cube.gltf", "cube.bin"}},
		{"named glb", NamedMode, "cube", []string{"cube.glb", "cvr_object_thumbnail.png"}, GLB, []string{"cube.glb"}},
		{"named upper-case extension", NamedMode, "cube", []string{"cube.GLB"}, GLB, []string{"cube.GLB"}},
		{"named ignores another base glb when own exists", NamedMode, "cube", []string{"cube.glb", "other.glb"}, GLB, []string{"cube.glb"}},
		{"any single glb", AnyMode, "", []string{"head.glb"}, GLB, []string{"head.glb"}},
		{"any gltf and bin", AnyMode, "", []string{"head.gltf", "head.bin"}, GLTFSeparate, []string{"head.gltf", "head.bin"}},
		{"any gltf without bin", AnyMode, "", []string{"head.gltf"}, GLTFSeparate, []string{"head.gltf"}},
		{"any gltf with several bins sorted", AnyMode, "", []string{"head.gltf", "b.bin", "a.bin"}, GLTFSeparate, []string{"head.gltf", "a.bin", "b.bin"}},
		{"hidden files and junk ignored", SceneMode, "scene", []string{"scene.glb", ".hidden.glb", ".DS_Store", "Thumbs.db", "desktop.ini", "Desktop.INI", "._scene.bin"}, GLB, []string{"scene.glb"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := mkDir(t, c.files...)
			got, err := Resolve(dir, c.mode, c.base)
			if err != nil {
				t.Fatalf("Resolve: %v", err)
			}
			if got.Kind != c.kind {
				t.Errorf("Kind = %v, want %v", got.Kind, c.kind)
			}
			abs, _ := filepath.Abs(dir)
			var want []string
			for _, n := range c.want {
				want = append(want, filepath.Join(abs, n))
			}
			if !reflect.DeepEqual(got.Paths, want) {
				t.Errorf("Paths = %v, want %v", got.Paths, want)
			}
		})
	}
}

func TestResolveRejects(t *testing.T) {
	cases := []struct {
		name  string
		mode  Mode
		base  string
		files []string
		want  []string
	}{
		{"scene two glb", SceneMode, "scene", []string{"one.glb", "two.glb"}, []string{"one.glb", "two.glb"}},
		{"scene two glb differing case", SceneMode, "scene", []string{"one.glb", "two.GLB"}, []string{"one.glb", "two.GLB"}},
		{"scene glb beside gltf and bin", SceneMode, "scene", []string{"scene.glb", "scene.gltf", "scene.bin"}, []string{"scene.glb", "scene.gltf", "scene.bin"}},
		{"scene glb beside gltf", SceneMode, "scene", []string{"MyRoom.glb", "scene.gltf"}, []string{"MyRoom.glb", "scene.gltf"}},
		{"scene glb beside stray bin", SceneMode, "scene", []string{"scene.glb", "scene.bin"}, []string{"scene.glb", "scene.bin"}},
		{"scene gltf without bin", SceneMode, "scene", []string{"scene.gltf"}, []string{"scene.gltf", "scene.bin"}},
		{"scene bin without gltf", SceneMode, "scene", []string{"scene.bin"}, []string{"scene.bin", "scene.gltf"}},
		{"scene nothing", SceneMode, "scene", []string{"screenshot.png"}, []string{".glb", "scene.gltf", "scene.bin"}},
		{"scene empty dir", SceneMode, "scene", nil, []string{"scene.gltf"}},
		{"named glb beside pair", NamedMode, "cube", []string{"cube.glb", "cube.gltf", "cube.bin"}, []string{"cube.glb", "cube.gltf", "cube.bin"}},
		{"named other base glb is not the model", NamedMode, "cube", []string{"other.glb"}, []string{"cube.glb", "cube.gltf"}},
		{"named nothing", NamedMode, "cube", []string{"cvr_object_thumbnail.png"}, []string{"cube.glb"}},
		{"named case-different base", NamedMode, "cube", []string{"Cube.glb"}, []string{"Cube.glb", "cube"}},
		{"named case-different bin", NamedMode, "cube", []string{"cube.gltf", "Cube.bin"}, []string{"Cube.bin", "cube"}},
		{"any two glb", AnyMode, "", []string{"a.glb", "b.glb"}, []string{"a.glb", "b.glb"}},
		{"any glb beside gltf", AnyMode, "", []string{"a.glb", "b.gltf"}, []string{"a.glb", "b.gltf"}},
		{"any glb beside bin", AnyMode, "", []string{"a.glb", "b.bin"}, []string{"a.glb", "b.bin"}},
		{"any two gltf", AnyMode, "", []string{"head.gltf", "hand.gltf"}, []string{"head.gltf", "hand.gltf"}},
		{"any bin without gltf", AnyMode, "", []string{"head.bin"}, []string{"head.bin"}},
		{"any nothing", AnyMode, "", []string{"ego.json"}, []string{".glb", ".gltf"}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			dir := mkDir(t, c.files...)
			got, err := Resolve(dir, c.mode, c.base)
			if err == nil {
				t.Fatalf("Resolve succeeded with %+v, want error", got)
			}
			for _, w := range c.want {
				if !strings.Contains(err.Error(), w) {
					t.Errorf("error %q does not name %q", err, w)
				}
			}
		})
	}
}

func TestResolveMixedFormMessage(t *testing.T) {
	dir := mkDir(t, "scene.glb", "scene.gltf")
	_, err := Resolve(dir, SceneMode, "scene")
	want := "found both scene.glb and scene.gltf; a scene directory holds one .glb or scene.gltf plus scene.bin, never both"
	if err == nil || err.Error() != want {
		t.Fatalf("error = %v, want %q", err, want)
	}
}

func TestResolveIgnoresDirectoriesAndMissingDir(t *testing.T) {
	dir := mkDir(t, "scene.gltf", "scene.bin")
	if err := os.Mkdir(filepath.Join(dir, "other.glb"), 0o755); err != nil {
		t.Fatal(err)
	}
	got, err := Resolve(dir, SceneMode, "scene")
	if err != nil || got.Kind != GLTFSeparate {
		t.Fatalf("Resolve = %+v, %v; want the gltf pair (a directory named other.glb is not a model)", got, err)
	}
	if _, err := Resolve(filepath.Join(dir, "missing"), SceneMode, "scene"); err == nil {
		t.Fatal("Resolve on a missing directory succeeded")
	}
}

func TestResolveFixtures(t *testing.T) {
	cases := []struct {
		dir  string
		mode Mode
		base string
		kind Kind
	}{
		{"scene-gltf", SceneMode, "scene", GLTFSeparate},
		{"scene-glb", SceneMode, "scene", GLB},
		{"object-gltf", NamedMode, "cube", GLTFSeparate},
		{"object-gltf", AnyMode, "", GLTFSeparate},
	}
	for _, c := range cases {
		got, err := Resolve(filepath.Join("..", "..", "testdata", c.dir), c.mode, c.base)
		if err != nil || got.Kind != c.kind {
			t.Errorf("%s: Resolve = %+v, %v; want kind %v", c.dir, got, err, c.kind)
		}
	}
}

func TestIsReservedConverterName(t *testing.T) {
	uuid := "8f966d6a-a9d5-4010-9620-e1cfe823998c"
	cases := []struct {
		name, base string
		want       bool
	}{
		{"scene_0.png", "scene", true},
		{"scene_1.png", "scene", true},
		{"scene_12.webp", "scene", true},
		{"Scene_0.png", "scene", false},
		{"scene_a.png", "scene", false},
		{"scene_.png", "scene", false},
		{"scene_0", "scene", false},
		{"scene_0.tar.gz", "scene", false},
		{"scene.png", "scene", false},
		{"other_0.png", "scene", false},
		{uuid + "_3.png", uuid, true},
	}
	for _, c := range cases {
		if got := IsReservedConverterName(c.name, c.base); got != c.want {
			t.Errorf("IsReservedConverterName(%q, %q) = %v, want %v", c.name, c.base, got, c.want)
		}
	}
}

func TestGLBObjectBaseName(t *testing.T) {
	cases := []struct{ in, want string }{
		{"empty rack", "empty_rack"},
		{"a-b_c.d", "a-b_c_d"},
		{"Cube9", "Cube9"},
		{"", "object"},
		{"héllo", "h_llo"},
		{strings.Repeat("a", 100), strings.Repeat("a", 64)},
		{strings.Repeat("é", 100), strings.Repeat("_", 64)},
		{"a\xffb", "a_b"},
	}
	for _, c := range cases {
		if got := GLBObjectBaseName(c.in); got != c.want {
			t.Errorf("GLBObjectBaseName(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestTextures(t *testing.T) {
	dir := mkDir(t,
		"scene.glb", "scene.gltf", "scene.bin",
		"screenshot.png", "settings.json", "cvr_object_thumbnail.png",
		"b.PNG", "a.jpg", "c.jpeg", "d.WEBP",
		"model.ktx2", "notes.txt",
		".DS_Store", "Thumbs.db", "desktop.ini", ".hidden.png",
	)
	if err := os.Mkdir(filepath.Join(dir, "sub.png"), 0o755); err != nil {
		t.Fatal(err)
	}
	keep, skipped := Textures(dir, []string{"scene.glb", "scene.gltf", "scene.bin", "screenshot.png", "settings.json", "cvr_object_thumbnail.png"})
	if want := []string{"a.jpg", "b.PNG", "c.jpeg", "d.WEBP"}; !reflect.DeepEqual(keep, want) {
		t.Errorf("keep = %v, want %v", keep, want)
	}
	if want := []string{"model.ktx2", "notes.txt"}; !reflect.DeepEqual(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

func TestTexturesSkipsUnselectedModelFiles(t *testing.T) {
	dir := mkDir(t, "cube.glb", "other.glb", "extra.bin", "a.png")
	keep, skipped := Textures(dir, []string{"cube.glb"})
	if want := []string{"a.png"}; !reflect.DeepEqual(keep, want) {
		t.Errorf("keep = %v, want %v", keep, want)
	}
	if want := []string{"extra.bin", "other.glb"}; !reflect.DeepEqual(skipped, want) {
		t.Errorf("skipped = %v, want %v", skipped, want)
	}
}

func TestTexturesFixture(t *testing.T) {
	keep, skipped := Textures(filepath.Join("..", "..", "testdata", "scene-gltf"), []string{"scene.gltf", "scene.bin", "screenshot.png"})
	if want := []string{"texture1.png", "texture2.png", "texture3.png"}; !reflect.DeepEqual(keep, want) {
		t.Errorf("keep = %v, want %v", keep, want)
	}
	if len(skipped) != 0 {
		t.Errorf("skipped = %v, want none", skipped)
	}
}

func TestFixturesAreBinaryInGit(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not available")
	}
	top, err := exec.Command("git", "rev-parse", "--show-toplevel").Output()
	if err != nil {
		t.Skipf("not inside a git work tree: %v", err)
	}
	root := strings.TrimSpace(string(top))
	out, err := exec.Command("git", "-C", root, "ls-files", "--eol", "testdata").Output()
	if err != nil {
		t.Skipf("git ls-files failed: %v", err)
	}
	checked := 0
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		meta, path, ok := strings.Cut(line, "\t")
		if !ok {
			continue
		}
		switch strings.ToLower(filepath.Ext(path)) {
		case ".glb", ".bin", ".png", ".jpg", ".jpeg", ".webp":
		default:
			continue
		}
		_, rest, found := strings.Cut(meta, "attr/")
		if !found {
			t.Errorf("%s: no attr column in %q", path, meta)
			continue
		}
		if attr := strings.TrimSpace(rest); attr != "-text" {
			t.Errorf("%s: attr/%s, want attr/-text", path, attr)
		}
		checked++
	}
	if checked == 0 {
		t.Error("no binary fixtures found in git's index under testdata (run git add testdata)")
	}
}
