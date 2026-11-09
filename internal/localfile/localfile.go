// Package localfile describes a regular file in an upload directory and turns
// it into a multipart part.
package localfile

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/model"
)

const octetStream = "application/octet-stream"

// File is a regular file: its base name, its path and its size in bytes.
type File struct {
	Name string
	Path string
	Size int64
}

// Stat describes the file name in dir. The name must be a safe path segment,
// and the entry a regular file, not a link. A missing file returns an error
// wrapping fs.ErrNotExist.
func Stat(dir, name string) (File, error) {
	if err := model.SafeSegment(name); err != nil {
		return File{}, fmt.Errorf("cannot upload %q: %w", name, err)
	}
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return File{}, err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return File{}, fmt.Errorf("%s is a symbolic link; it must be a regular file", name)
	case !info.Mode().IsRegular():
		return File{}, fmt.Errorf("%s is not a regular file", name)
	}
	return File{Name: name, Path: path, Size: info.Size()}, nil
}

// Part builds a multipart part under field that streams f from disk. Open
// refuses the file if it was replaced by a link or a non-regular entry.
func Part(field string, f File) gateway.Part {
	path := f.Path
	return gateway.Part{
		Field: field, Filename: f.Name, ContentType: octetStream, Size: f.Size,
		Open: func() (io.ReadCloser, error) { return OpenRegular(path) },
	}
}

// OpenRegular opens path and confirms that the opened file is the regular
// file now at path, not a link. A missing file returns an error wrapping
// fs.ErrNotExist.
func OpenRegular(path string) (*os.File, error) {
	before, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if !before.Mode().IsRegular() {
		return nil, replacedError(path)
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	opened, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	current, err := os.Lstat(path)
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	if !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		_ = f.Close()
		return nil, replacedError(path)
	}
	return f, nil
}

// PartIn is Part for a file below root, possibly in a subdirectory: Open
// resolves the path inside an os.Root, so a directory component swapped for
// a link that leaves root is refused, not followed.
func PartIn(field, root string, f File) gateway.Part {
	rel, err := filepath.Rel(root, f.Path)
	if err != nil || rel == "." || strings.HasPrefix(rel, "..") {
		return Part(field, f)
	}
	return gateway.Part{
		Field: field, Filename: f.Name, ContentType: octetStream, Size: f.Size,
		Open: func() (io.ReadCloser, error) { return OpenRegularIn(root, rel) },
	}
}

// OpenRegularIn is OpenRegular for the file at rel below root, with every
// path component resolved inside root.
func OpenRegularIn(root, rel string) (*os.File, error) {
	r, err := os.OpenRoot(root)
	if err != nil {
		return nil, err
	}
	defer func() { _ = r.Close() }()
	path := filepath.Join(root, rel)
	before, err := r.Lstat(rel)
	if err != nil {
		return nil, escapeError(path, err)
	}
	if !before.Mode().IsRegular() {
		return nil, replacedError(path)
	}
	f, err := r.Open(rel)
	if err != nil {
		return nil, escapeError(path, err)
	}
	opened, err := f.Stat()
	if err != nil {
		_ = f.Close()
		return nil, err
	}
	current, err := r.Lstat(rel)
	if err != nil {
		_ = f.Close()
		return nil, escapeError(path, err)
	}
	if !current.Mode().IsRegular() || !os.SameFile(opened, current) {
		_ = f.Close()
		return nil, replacedError(path)
	}
	return f, nil
}

// escapeError turns os.Root's refusal to leave the root (a directory on the
// way swapped for a link out of it) into the same message a replaced file
// gets. Other errors pass through unchanged.
func escapeError(path string, err error) error {
	var pathErr *fs.PathError
	if errors.As(err, &pathErr) && pathErr.Err != nil && pathErr.Err.Error() == "path escapes from parent" {
		return fmt.Errorf("%w: %w", replacedError(path), err)
	}
	return err
}

func replacedError(path string) error {
	return fmt.Errorf("%s was replaced or is a link; it must be a regular file", path)
}
