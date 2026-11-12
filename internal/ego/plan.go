// Package ego plans an egocentric (session replay) model upload for one slot.
package ego

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strings"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/localfile"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/model"
)

const (
	// ManifestFileName is the one optional metadata file beside the model.
	ManifestFileName = "ego.json"
	maxManifestBytes = 64 * 1024
)

// KnownSlots are the slots the gateway gives a meaning; any other valid slot
// is stored as a custom key.
var KnownSlots = []string{"head", "controller_left", "controller_right"}

var slotPattern = regexp.MustCompile(`^[a-z0-9_-]{1,64}$`)

var imageExtensions = map[string]bool{".png": true, ".jpg": true, ".webp": true}

// UploadPlan is everything an ego upload sends, read from the directory
// without any network call. Parts are in the gateway's order: model files,
// images, then ego.json. Images holds file names, sorted.
type UploadPlan struct {
	Slot        string
	Files       model.Files
	Parts       []gateway.Part
	Images      []string
	HasManifest bool
	TotalBytes  int64
	Warnings    []string
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

// ValidateSlot reports whether slot can name an ego slot.
func ValidateSlot(slot string) error {
	if err := model.SafeSegment(slot); err != nil || !slotPattern.MatchString(slot) {
		return fmt.Errorf("invalid slot %q: use 1 to 64 lowercase letters, digits, '-' or '_'", slot)
	}
	return nil
}

// Plan reads dir and decides what an upload of the model in it to slot would
// send. Every error is a problem with the slot or the directory's contents.
func Plan(dir, slot string) (*UploadPlan, error) {
	if err := ValidateSlot(slot); err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return nil, err
	}
	files, err := model.Resolve(abs, model.AnyMode, "")
	if err != nil {
		return nil, err
	}
	p := &UploadPlan{Slot: slot, Files: files}
	if !isKnownSlot(slot) {
		p.Warnings = append(p.Warnings, fmt.Sprintf("slot %q is not one of the known slots %s", slot, strings.Join(KnownSlots, ", ")))
	}

	var models, images []localfile.File
	var modelNames, imageNames []string
	for _, path := range files.Paths {
		f, err := localfile.Stat(abs, filepath.Base(path))
		if err != nil {
			return nil, err
		}
		models = append(models, f)
		modelNames = append(modelNames, f.Name)
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return nil, err
	}
	reservedBase := model.GLBObjectBaseName(slot)
	var manifest *localfile.File
	for _, e := range entries {
		name := e.Name()
		if model.IsSystemFile(name) || slices.Contains(modelNames, name) {
			continue
		}
		if e.IsDir() {
			return nil, fmt.Errorf("found the subdirectory %s: subdirectories are not uploaded; move its files next to the model or remove it", name)
		}
		ext := strings.ToLower(filepath.Ext(name))
		switch {
		case name == ManifestFileName:
			f, err := localfile.Stat(abs, name)
			if err != nil {
				return nil, err
			}
			if f.Size > maxManifestBytes {
				return nil, fmt.Errorf("%s is %d bytes; the limit is 64 KiB (%d bytes)", name, f.Size, maxManifestBytes)
			}
			manifest = &f
		case imageExtensions[ext]:
			if files.Kind == model.GLB && model.IsReservedConverterName(name, reservedBase) {
				return nil, fmt.Errorf("found %s beside %s: that name is reserved for an image the gateway extracts from the .glb (%s_<n>.<ext>); rename or remove it",
					name, modelNames[0], reservedBase)
			}
			f, err := localfile.Stat(abs, name)
			if err != nil {
				return nil, err
			}
			images = append(images, f)
			imageNames = append(imageNames, name)
		default:
			return nil, fmt.Errorf("found %s: an egocentric model directory holds only the model, .png, .jpg and .webp images, and an optional %s; remove it", name, ManifestFileName)
		}
	}
	sort.Slice(images, func(i, j int) bool { return images[i].Name < images[j].Name })
	sort.Strings(imageNames)
	p.Images = imageNames
	p.HasManifest = manifest != nil

	findings, err := model.Check(files, abs, append(append([]string{}, modelNames...), imageNames...), model.AnyMode)
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
	for _, f := range append(append([]localfile.File{}, models...), images...) {
		p.Parts = append(p.Parts, localfile.Part("file", f))
		p.TotalBytes += f.Size
		sized = append(sized, model.SizedFile{Name: f.Name, Bytes: f.Size})
	}
	if manifest != nil {
		part := localfile.Part("file", *manifest)
		part.ContentType = "application/json"
		p.Parts = append(p.Parts, part)
		p.TotalBytes += manifest.Size
		sized = append(sized, model.SizedFile{Name: manifest.Name, Bytes: manifest.Size})
	}
	warnings, err := model.CheckSizes(sized, p.TotalBytes)
	p.Warnings = append(p.Warnings, warnings...)
	if err != nil {
		return nil, err
	}
	return p, nil
}

func isKnownSlot(slot string) bool { return slices.Contains(KnownSlots, slot) }
