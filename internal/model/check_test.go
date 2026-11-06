package model

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeFile(t *testing.T, dir, name string, data []byte) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func gltfFiles(dir, model string) Files {
	return Files{Kind: GLTFSeparate, Paths: []string{filepath.Join(dir, model)}}
}

func oneFinding(t *testing.T, got []Finding, sev Severity, wants ...string) {
	t.Helper()
	if len(got) != 1 {
		t.Fatalf("want 1 finding, got %d: %+v", len(got), got)
	}
	if got[0].Severity != sev {
		t.Fatalf("severity = %d, want %d (%q)", got[0].Severity, sev, got[0].Message)
	}
	for _, w := range wants {
		if !strings.Contains(got[0].Message, w) {
			t.Errorf("message %q does not contain %q", got[0].Message, w)
		}
	}
}

func TestCheckGLTFURIs(t *testing.T) {
	cases := []struct {
		name     string
		json     string
		onDisk   map[string]int
		uploaded []string
		mode     Mode
		severity Severity
		wants    []string
	}{
		{"buffer not uploaded", `{"buffers":[{"uri":"other.bin","byteLength":4}]}`, nil, []string{"obj.gltf", "obj.bin"}, NamedMode, Error, []string{"other.bin", "missing"}},
		{"subdirectory", `{"images":[{"uri":"textures/a.png"}]}`, nil, []string{"obj.gltf"}, NamedMode, Error, []string{"textures/a.png", "subdirectories are not uploaded"}},
		{"parent directory", `{"buffers":[{"uri":"../scene.bin","byteLength":4}]}`, nil, []string{"scene.gltf", "scene.bin"}, NamedMode, Error, []string{"../scene.bin", "parent directory"}},
		{"backslash", `{"images":[{"uri":"a\\b.png"}]}`, nil, []string{"obj.gltf"}, NamedMode, Error, []string{"subdirector"}},
		{"remote", `{"images":[{"uri":"https://example.com/a.png"}]}`, nil, []string{"obj.gltf"}, NamedMode, Error, []string{"remote uri"}},
		{"bad percent escape", `{"images":[{"uri":"a%zz.png"}]}`, nil, []string{"obj.gltf"}, NamedMode, Error, []string{"percent-decoded"}},
		{"case differs", `{"buffers":[{"uri":"lantern.bin","byteLength":4}]}`, nil, []string{"obj.gltf", "Lantern.bin"}, NamedMode, Error, []string{"lantern.bin", "Lantern.bin", "case differs"}},
		{"scene buffer must be scene.bin", `{"buffers":[{"uri":"other.bin","byteLength":4}]}`, map[string]int{"other.bin": 4}, []string{"scene.gltf", "other.bin"}, SceneMode, Error, []string{"the gateway stores every .bin as scene.bin"}},
		{"skipped texture", `{"images":[{"uri":"tex.ktx2"}]}`, map[string]int{"tex.ktx2": 3}, []string{"obj.gltf"}, NamedMode, Error, []string{"tex.ktx2", "not an accepted texture type", ".png .jpg .jpeg .webp"}},
		{"byteLength mismatch", `{"buffers":[{"uri":"obj.bin","byteLength":10}]}`, map[string]int{"obj.bin": 4}, []string{"obj.gltf", "obj.bin"}, NamedMode, Error, []string{"10 bytes", "4 bytes"}},
		{"data uri", `{"images":[{"uri":"data:image/png;base64,AAAA"}]}`, nil, []string{"obj.gltf"}, NamedMode, Warning, []string{"data:"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "obj.gltf", []byte(tc.json))
			for name, size := range tc.onDisk {
				writeFile(t, dir, name, make([]byte, size))
			}
			got, err := Check(gltfFiles(dir, "obj.gltf"), dir, tc.uploaded, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			oneFinding(t, got, tc.severity, tc.wants...)
		})
	}
}

func TestCheckGLTFURIMatches(t *testing.T) {
	cases := []struct {
		name     string
		json     string
		onDisk   map[string]int
		uploaded []string
		mode     Mode
	}{
		{"percent-encoded space", `{"images":[{"uri":"my%20tex.png"}]}`, nil, []string{"obj.gltf", "my tex.png"}, NamedMode},
		{"leading dot slash", `{"buffers":[{"uri":"./scene.bin","byteLength":4}]}`, map[string]int{"scene.bin": 4}, []string{"scene.gltf", "scene.bin"}, SceneMode},
		{"matching byteLength", `{"buffers":[{"uri":"obj.bin","byteLength":4}],"images":[{"uri":"a.png"}]}`, map[string]int{"obj.bin": 4}, []string{"obj.gltf", "obj.bin", "a.png"}, NamedMode},
		{"embedded buffer without uri", `{"buffers":[{"byteLength":4}],"images":[{"bufferView":0}]}`, nil, []string{"obj.gltf"}, NamedMode},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "obj.gltf", []byte(tc.json))
			for name, size := range tc.onDisk {
				writeFile(t, dir, name, make([]byte, size))
			}
			got, err := Check(gltfFiles(dir, "obj.gltf"), dir, tc.uploaded, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 0 {
				t.Fatalf("want no findings, got %+v", got)
			}
		})
	}
}

func TestCheckGLTFInvalidJSONIsFinding(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "obj.gltf", []byte("{not json"))
	got, err := Check(gltfFiles(dir, "obj.gltf"), dir, []string{"obj.gltf"}, NamedMode)
	if err != nil {
		t.Fatal(err)
	}
	oneFinding(t, got, Error, "not a valid glTF JSON")
}

func TestCheckMissingModelFileReturnsError(t *testing.T) {
	dir := t.TempDir()
	if _, err := Check(gltfFiles(dir, "gone.gltf"), dir, nil, NamedMode); err == nil {
		t.Fatal("want an error reading a missing model")
	}
}

func glbBytes(magic, version, total, chunkLength, chunkType uint32, body []byte) []byte {
	b := make([]byte, 20, 20+len(body))
	binary.LittleEndian.PutUint32(b[0:], magic)
	binary.LittleEndian.PutUint32(b[4:], version)
	binary.LittleEndian.PutUint32(b[8:], total)
	binary.LittleEndian.PutUint32(b[12:], chunkLength)
	binary.LittleEndian.PutUint32(b[16:], chunkType)
	return append(b, body...)
}

func validGLB(jsonText string) []byte {
	body := []byte(jsonText)
	for len(body)%4 != 0 {
		body = append(body, ' ')
	}
	return glbBytes(glbMagic, 2, uint32(20+len(body)), uint32(len(body)), glbChunkJSON, body)
}

func TestCheckGLB(t *testing.T) {
	short := []byte("{}  ")
	cases := []struct {
		name  string
		data  []byte
		wants []string
	}{
		{"wrong magic", glbBytes(0x12345678, 2, 24, 4, glbChunkJSON, short), []string{"magic"}},
		{"version 1", glbBytes(glbMagic, 1, 24, 4, glbChunkJSON, short), []string{"version 1"}},
		{"length mismatch", glbBytes(glbMagic, 2, 99, 4, glbChunkJSON, short), []string{"99 bytes", "24 bytes"}},
		{"first chunk not JSON", glbBytes(glbMagic, 2, 24, 4, 0x004E4942, short), []string{"not JSON"}},
		{"JSON chunk over limit", glbBytes(glbMagic, 2, 24, 9<<20, glbChunkJSON, short), []string{"JSON chunk is 9437184 bytes", "8 MB limit"}},
		{"chunk past end", glbBytes(glbMagic, 2, 24, 400, glbChunkJSON, short), []string{"truncated"}},
		{"header only", glbBytes(glbMagic, 2, 12, 0, 0, nil)[:12], []string{"no JSON chunk"}},
		{"too short for header", []byte("glTF"), []string{"truncated"}},
		{"external buffer uri", validGLB(`{"buffers":[{"uri":"a.bin","byteLength":4}]}`), []string{"buffers[0]", "a.bin", "a .glb must be self-contained"}},
		{"external image uri", validGLB(`{"images":[{"uri":"a.png"}]}`), []string{"images[0]", "a.png", "self-contained"}},
		{"invalid JSON chunk", validGLB(`{oops`), []string{"not valid glTF JSON"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			writeFile(t, dir, "m.glb", tc.data)
			files := Files{Kind: GLB, Paths: []string{filepath.Join(dir, "m.glb")}}
			got, err := Check(files, dir, []string{"m.glb"}, AnyMode)
			if err != nil {
				t.Fatal(err)
			}
			oneFinding(t, got, Error, tc.wants...)
		})
	}
}

func TestCheckGLBValid(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, dir, "m.glb", validGLB(`{"buffers":[{"byteLength":4}],"images":[{"bufferView":0}]}`))
	files := Files{Kind: GLB, Paths: []string{filepath.Join(dir, "m.glb")}}
	got, err := Check(files, dir, []string{"m.glb"}, AnyMode)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Fatalf("want no findings, got %+v", got)
	}
}

func TestCheckFixturesHaveNoFindings(t *testing.T) {
	cases := []struct {
		dir  string
		mode Mode
		base string
	}{
		{"scene-gltf", SceneMode, "scene"},
		{"scene-glb", SceneMode, "scene"},
		{"object-gltf", NamedMode, "cube"},
	}
	for _, tc := range cases {
		t.Run(tc.dir, func(t *testing.T) {
			dir := filepath.Join("..", "..", "testdata", tc.dir)
			files, err := Resolve(dir, tc.mode, tc.base)
			if err != nil {
				t.Fatal(err)
			}
			var uploaded []string
			for _, p := range files.Paths {
				uploaded = append(uploaded, filepath.Base(p))
			}
			keep, _ := Textures(dir, uploaded)
			uploaded = append(uploaded, keep...)
			got, err := Check(files, dir, uploaded, tc.mode)
			if err != nil {
				t.Fatal(err)
			}
			if len(got) != 0 {
				t.Fatalf("want no findings, got %+v", got)
			}
		})
	}
}
