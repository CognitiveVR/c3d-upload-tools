// Package object plans and sends a dynamic object upload and decides the
// object's manifest id.
package object

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/localfile"
	"github.com/cognitivevr/c3d-upload-tools/internal/model"
)

const (
	// ThumbnailFileName is the thumbnail every object upload carries.
	ThumbnailFileName = "cvr_object_thumbnail.png"
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

	var models, textures []localfile.File
	var modelNames []string
	for _, path := range files.Paths {
		f, err := localfile.Stat(abs, filepath.Base(path))
		if err != nil {
			return nil, err
		}
		models = append(models, f)
		modelNames = append(modelNames, f.Name)
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
		f, err := localfile.Stat(abs, name)
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

	var sized []model.SizedFile
	for _, f := range append(append(models, textures...), thumbnail) {
		p.Parts = append(p.Parts, localfile.Part("file", f))
		p.TotalBytes += f.Size
		sized = append(sized, model.SizedFile{Name: f.Name, Bytes: f.Size})
	}
	warnings, err := model.CheckSizes(sized, p.TotalBytes)
	p.Warnings = append(p.Warnings, warnings...)
	if err != nil {
		return nil, err
	}
	return p, nil
}

// findThumbnail matches the thumbnail name exactly, so that a differently
// cased file is reported the same way on every file system.
func findThumbnail(dir string) (localfile.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return localfile.File{}, err
	}
	var near []string
	for _, e := range entries {
		switch {
		case e.Name() == ThumbnailFileName:
			return localfile.Stat(dir, ThumbnailFileName)
		case strings.EqualFold(e.Name(), ThumbnailFileName):
			near = append(near, e.Name())
		}
	}
	if len(near) > 0 {
		return localfile.File{}, fmt.Errorf("%s is missing from %s: found %s, but the name must be exactly %s", ThumbnailFileName, dir, strings.Join(near, ", "), ThumbnailFileName)
	}
	return localfile.File{}, fmt.Errorf("%s is missing from %s: an object upload needs it as the object's thumbnail", ThumbnailFileName, dir)
}
