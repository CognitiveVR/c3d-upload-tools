package scene

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/localfile"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/model"
)

const (
	screenshotFileName = "screenshot.png"
	screenshotDirName  = "screenshot"
	modelBaseName      = "scene"
)

// LocalPlan is everything a scene upload sends except settings.json, read
// from the directory without any network call. Textures and Skipped are file
// names; Screenshot is an absolute path; TotalBytes counts the model files,
// textures and screenshot.
type LocalPlan struct {
	Dir        string
	Files      model.Files
	Textures   []string
	Skipped    []string
	Screenshot string
	Existing   *Settings
	TotalBytes int64
	Warnings   []string

	models     []localfile.File
	textures   []localfile.File
	screenshot localfile.File
}

// UploadPlan is a LocalPlan with its settings.json built: Parts are in the
// gateway's order and SettingsBytes is exactly what the settings part sends.
type UploadPlan struct {
	Local         *LocalPlan
	SceneName     string
	Parts         []gateway.Part
	SettingsBytes []byte
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

// Plan reads dir and decides what a scene upload would send. Every error is a
// problem with the directory's contents.
func Plan(dir string) (*LocalPlan, error) {
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	files, err := model.Resolve(abs, model.SceneMode, modelBaseName)
	if err != nil {
		return nil, err
	}
	p := &LocalPlan{Dir: abs, Files: files}

	var modelNames []string
	for _, path := range files.Paths {
		f, err := localfile.Stat(abs, filepath.Base(path))
		if err != nil {
			return nil, err
		}
		p.models = append(p.models, f)
		modelNames = append(modelNames, f.Name)
	}

	if p.screenshot, err = findScreenshot(abs); err != nil {
		return nil, err
	}
	p.Screenshot = p.screenshot.Path

	if p.Existing, err = ReadSettings(abs); err != nil {
		return nil, err
	}

	exclude := append(append([]string{}, modelNames...), screenshotFileName, settingsFileName)
	p.Textures, p.Skipped = model.Textures(abs, exclude)
	for _, name := range p.Textures {
		if files.Kind == model.GLB && model.IsReservedConverterName(name, modelBaseName) {
			return nil, fmt.Errorf("found %s beside %s: that name is reserved for an image the gateway extracts from the .glb (%s_<n>.<ext>); rename or remove it",
				name, modelNames[0], modelBaseName)
		}
		f, err := localfile.Stat(abs, name)
		if err != nil {
			return nil, err
		}
		p.textures = append(p.textures, f)
	}
	for _, name := range p.Skipped {
		p.Warnings = append(p.Warnings, fmt.Sprintf("skipping %s: only .png, .jpg, .jpeg and .webp files are uploaded beside the model", name))
	}

	findings, err := model.Check(files, abs, append(modelNames, p.Textures...), model.SceneMode)
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

	for _, f := range p.files() {
		p.TotalBytes += f.Size
	}
	if err := checkSizes(p); err != nil {
		return nil, err
	}
	return p, nil
}

// files lists every file the upload sends, screenshot last.
func (p *LocalPlan) files() []localfile.File {
	out := append(append([]localfile.File{}, p.models...), p.textures...)
	if p.screenshot.Name != "" {
		out = append(out, p.screenshot)
	}
	return out
}

// findScreenshot takes screenshot.png from dir, else from the screenshot
// subdirectory the Unity SDK exports to. The name must match exactly, so that
// a differently cased file is reported the same way on every file system.
// Like every other part, it must be a regular file, not a link.
func findScreenshot(dir string) (localfile.File, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return localfile.File{}, err
	}
	var near []string
	hasSubdir := false
	for _, e := range entries {
		switch {
		case e.Name() == screenshotFileName:
			return localfile.Stat(dir, screenshotFileName)
		case strings.EqualFold(e.Name(), screenshotFileName):
			near = append(near, e.Name())
		case e.Name() == screenshotDirName && e.Type().IsDir():
			hasSubdir = true
		}
	}
	if hasSubdir {
		subdir := filepath.Join(dir, screenshotDirName)
		subEntries, err := os.ReadDir(subdir)
		if err != nil {
			return localfile.File{}, err
		}
		for _, e := range subEntries {
			switch {
			case e.Name() == screenshotFileName:
				return localfile.Stat(subdir, screenshotFileName)
			case strings.EqualFold(e.Name(), screenshotFileName):
				near = append(near, filepath.Join(screenshotDirName, e.Name()))
			}
		}
	}
	if len(near) > 0 {
		return localfile.File{}, fmt.Errorf("%s is missing from %s: found %s, but the name must be exactly %s", screenshotFileName, dir, strings.Join(near, ", "), screenshotFileName)
	}
	return localfile.File{}, fmt.Errorf("%s is missing from %s: a scene upload needs it as the scene's screenshot, in that directory or in its %s subdirectory", screenshotFileName, dir, screenshotDirName)
}

// checkSizes applies the size thresholds of model.CheckSizes to the plan.
func checkSizes(p *LocalPlan) error {
	var sized []model.SizedFile
	for _, f := range p.files() {
		sized = append(sized, model.SizedFile{Name: f.Name, Bytes: f.Size})
	}
	warnings, err := model.CheckSizes(sized, p.TotalBytes)
	p.Warnings = append(p.Warnings, warnings...)
	return err
}

// Finalize builds settings.json in memory for sceneName (empty keeps the
// existing name) and lays out the parts: model files, textures and
// settings.json under field file, then the screenshot under field screenshot.
func Finalize(local *LocalPlan, sceneName, sdkVersion string) (*UploadPlan, error) {
	data, settings, err := BuildSettings(local.Existing, sceneName, sdkVersion)
	if err != nil {
		return nil, err
	}
	var parts []gateway.Part
	for _, f := range append(append([]localfile.File{}, local.models...), local.textures...) {
		parts = append(parts, localfile.Part("file", f))
	}
	parts = append(parts, gateway.Part{
		Field: "file", Filename: settingsFileName, ContentType: "application/json", Size: int64(len(data)),
		Open: func() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(data)), nil },
	})
	parts = append(parts, localfile.Part("screenshot", local.screenshot))
	return &UploadPlan{Local: local, SceneName: settings.SceneName, Parts: parts, SettingsBytes: data}, nil
}
