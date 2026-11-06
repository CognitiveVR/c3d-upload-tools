package localfile

import (
	"errors"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestStatRegularFile(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "a.bin"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Stat(dir, "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	if f.Name != "a.bin" || f.Size != 3 || f.Path != filepath.Join(dir, "a.bin") {
		t.Fatalf("got %+v", f)
	}
	rc, err := Part("file", f).Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	if data, _ := io.ReadAll(rc); string(data) != "abc" {
		t.Fatalf("part read %q", data)
	}
}

func TestStatRejections(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := Stat(dir, "missing.png"); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: got %v", err)
	}
	if _, err := Stat(dir, "sub"); err == nil || !strings.Contains(err.Error(), "sub is not a regular file") {
		t.Fatalf("directory: got %v", err)
	}
	if _, err := Stat(dir, "../x"); err == nil {
		t.Fatal("unsafe name accepted")
	}
	if runtime.GOOS != "windows" {
		if err := os.Symlink(filepath.Join(dir, "sub"), filepath.Join(dir, "link")); err != nil {
			t.Fatal(err)
		}
		if _, err := Stat(dir, "link"); err == nil || !strings.Contains(err.Error(), "link is a symbolic link; it must be a regular file") {
			t.Fatalf("symlink: got %v", err)
		}
	}
}
