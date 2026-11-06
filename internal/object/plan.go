// Package object plans and sends a dynamic object upload and decides the
// object's manifest id.
package object

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/model"
)

const (
	// ThumbnailFileName is the thumbnail every object upload carries.
	ThumbnailFileName = "cvr_object_thumbnail.png"
	octetStream       = "application/octet-stream"
)

// UploadPlan is everything an object upload sends, read from the directory
// without any network call. Parts are in the gateway's order: model files,
// textures, thumbnail. Textures and Skipped are file names.
type UploadPlan struct {
	Dir        string
	Mesh       string
	Files      model.Files
	Textures   []string
	Skipped    []string
	Parts      []gateway.Part
	TotalBytes int64
	Warnings   []string
}

// CheckError reports a model that failed the model check. Warnings are the
// plan's other warnings, which callers show alongside the errors.
type CheckError struct {
	Errors   []string
	Warnings []string
}

func (e *CheckError) Error() string {
	if len(e.Errors) == 1 {
		return "the model check found 1 error; fix it and run the upload again"
	}
	return fmt.Sprintf("the model check found %d errors; fix them and run the upload again", len(e.Errors))
}

type localFile struct {
	name string
	path string
	size int64
}

// Plan reads dir and decides what an upload of the object objectName (the mesh
// name) would send. Every error is a problem with the name or the directory's
// contents.
func Plan(dir, objectName string) (*UploadPlan, error) {
	if err := model.SafeSegment(objectName); err != nil {
		return nil, fmt.Errorf("invalid object filename: %w", err)
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	files, err := model.Resolve(abs, model.NamedMode, objectName)
	if err != nil {
		return nil, err
	}
	p := &UploadPlan{Dir: abs, Mesh: objectName, Files: files}

	var models, textures []localFile
	var modelNames []string
	for _, path := range files.Paths {
		f, err := statFile(abs, filepath.Base(path))
		if err != nil {
			return nil, err
		}
		models = append(models, f)
		modelNames = append(modelNames, f.name)
	}

	thumbnail, err := findThumbnail(abs)
	if err != nil {
		return nil, err
	}

	exclude := append(append([]string{}, modelNames...), ThumbnailFileName)
	p.Textures, p.Skipped = model.Textures(abs, exclude)
	reservedBase := model.GLBObjectBaseName(objectName)
	for _, name := range p.Textures {
		if files.Kind == model.GLB && model.IsReservedConverterName(name, reservedBase) {
			return nil, fmt.Errorf("found %s beside %s: that name is reserved for an image the gateway extracts from the .glb (%s_<n>.<ext>); rename or remove it",
				name, modelNames[0], reservedBase)
		}
		f, err := statFile(abs, name)
		if err != nil {
			return nil, err
		}
		textures = append(textures, f)
	}
	for _, name := range p.Skipped {
		p.Warnings = append(p.Warnings, fmt.Sprintf("skipping %s: only .png, .jpg, .jpeg and .webp files are uploaded beside the model", name))
	}

	findings, err := model.Check(files, abs, append(append([]string{}, modelNames...), p.Textures...), model.NamedMode)
	if err != nil {
		return nil, err
	}
	var errs []string
	for _, f := range findings {
		if f.Severity == model.Error {
			errs = append(errs, f.Message)
		} else {
			p.Warnings = append(p.Warnings, f.Message)
		}
	}
	if len(errs) > 0 {
		return nil, &CheckError{Errors: errs, Warnings: p.Warnings}
	}

	for _, f := range append(append(models, textures...), thumbnail) {
		p.Parts = append(p.Parts, filePart(f))
		p.TotalBytes += f.size
	}
	return p, nil
}

func statFile(dir, name string) (localFile, error) {
	if err := model.SafeSegment(name); err != nil {
		return localFile{}, fmt.Errorf("cannot upload %q: %w", name, err)
	}
	path := filepath.Join(dir, name)
	info, err := os.Lstat(path)
	if err != nil {
		return localFile{}, err
	}
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		return localFile{}, fmt.Errorf("%s is a symbolic link; it must be a regular file", name)
	case !info.Mode().IsRegular():
		return localFile{}, fmt.Errorf("%s is not a regular file", name)
	}
	return localFile{name: name, path: path, size: info.Size()}, nil
}

// findThumbnail matches the thumbnail name exactly, so that a differently
// cased file is reported the same way on every file system.
func findThumbnail(dir string) (localFile, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return localFile{}, err
	}
	var near []string
	for _, e := range entries {
		switch {
		case e.Name() == ThumbnailFileName:
			return statFile(dir, ThumbnailFileName)
		case strings.EqualFold(e.Name(), ThumbnailFileName):
			near = append(near, e.Name())
		}
	}
	if len(near) > 0 {
		return localFile{}, fmt.Errorf("%s is missing from %s: found %s, but the name must be exactly %s", ThumbnailFileName, dir, strings.Join(near, ", "), ThumbnailFileName)
	}
	return localFile{}, fmt.Errorf("%s is missing from %s: an object upload needs it as the object's thumbnail", ThumbnailFileName, dir)
}

func filePart(f localFile) gateway.Part {
	path := f.path
	return gateway.Part{
		Field: "file", Filename: f.name, ContentType: octetStream, Size: f.size,
		Open: func() (io.ReadCloser, error) { return os.Open(path) },
	}
}
