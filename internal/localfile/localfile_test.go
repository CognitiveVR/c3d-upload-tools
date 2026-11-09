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

func TestPartRefusesAFileSwappedForALinkAfterStat(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.bin")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	other := filepath.Join(dir, "other.bin")
	if err := os.WriteFile(other, []byte("xyz"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Stat(dir, "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(path); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(other, path); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	rc, err := Part("file", f).Open()
	if err == nil {
		_ = rc.Close()
		t.Fatal("opened a file that became a link")
	}
	if !strings.Contains(err.Error(), path) || !strings.Contains(err.Error(), "replaced or is a link") {
		t.Fatalf("error %q", err)
	}
}

func TestPartInReadsAFileInASubdirectory(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.bin"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Stat(sub, "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	part := PartIn("file", root, f)
	if part.Filename != "a.bin" || part.Size != 3 {
		t.Fatalf("part = %+v", part)
	}
	rc, err := part.Open()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = rc.Close() }()
	data, err := io.ReadAll(rc)
	if err != nil || string(data) != "abc" {
		t.Fatalf("read %q, %v", data, err)
	}
}

func TestPartInRefusesADirectorySwappedForALinkLeavingRoot(t *testing.T) {
	root := t.TempDir()
	sub := filepath.Join(root, "sub")
	if err := os.Mkdir(sub, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sub, "a.bin"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Stat(sub, "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	outside := t.TempDir()
	if err := os.WriteFile(filepath.Join(outside, "a.bin"), []byte("xyz"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(sub, sub+"-real"); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, sub); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	rc, err := PartIn("file", root, f).Open()
	if err == nil {
		_ = rc.Close()
		t.Fatal("opened a file through a directory that became a link out of root")
	}
	if !strings.Contains(err.Error(), f.Path) || !strings.Contains(err.Error(), "replaced or is a link") {
		t.Fatalf("error %q, want the replaced-file message naming the path", err)
	}
	rc, err = Part("file", f).Open()
	if err == nil {
		_ = rc.Close()
		t.Log("Part follows the directory link; PartIn is the guard for subdirectories")
	}
}

func TestPartInFallsBackToPartOutsideRoot(t *testing.T) {
	root := t.TempDir()
	elsewhere := t.TempDir()
	if err := os.WriteFile(filepath.Join(elsewhere, "a.bin"), []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := Stat(elsewhere, "a.bin")
	if err != nil {
		t.Fatal(err)
	}
	rc, err := PartIn("file", root, f).Open()
	if err != nil {
		t.Fatal(err)
	}
	_ = rc.Close()
}

func TestOpenRegular(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a.png")
	if err := os.WriteFile(path, []byte("abc"), 0o644); err != nil {
		t.Fatal(err)
	}
	f, err := OpenRegular(path)
	if err != nil {
		t.Fatal(err)
	}
	_ = f.Close()
	if _, err := OpenRegular(dir); err == nil || !strings.Contains(err.Error(), "replaced or is a link") {
		t.Fatalf("directory: got %v", err)
	}
	if _, err := OpenRegular(filepath.Join(dir, "missing")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("missing: got %v", err)
	}
	link := filepath.Join(dir, "link.png")
	if err := os.Symlink(path, link); err != nil {
		t.Skipf("cannot create a symlink here: %v", err)
	}
	if _, err := OpenRegular(link); err == nil || !strings.Contains(err.Error(), "replaced or is a link") {
		t.Fatalf("symlink: got %v", err)
	}
}
