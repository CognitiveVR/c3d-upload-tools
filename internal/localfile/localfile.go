// Package localfile describes a regular file in an upload directory and turns
// it into a multipart part.
package localfile

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

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

// Part builds a multipart part under field that streams f from disk.
func Part(field string, f File) gateway.Part {
	path := f.Path
	return gateway.Part{
		Field: field, Filename: f.Name, ContentType: octetStream, Size: f.Size,
		Open: func() (io.ReadCloser, error) { return os.Open(path) },
	}
}
