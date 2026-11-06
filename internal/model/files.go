// Package model decides which files in a directory make up a 3D model and
// which are textures. It reads the directory and nothing else.
package model

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
)

type Mode int

const (
	// SceneMode accepts one .glb of any name, or scene.gltf plus scene.bin.
	SceneMode Mode = iota
	// NamedMode accepts <baseName>.glb, or <baseName>.gltf plus <baseName>.bin.
	NamedMode
	// AnyMode accepts one .glb of any name, or exactly one .gltf with zero or
	// more .bin files.
	AnyMode
)

type Kind int

const (
	GLB Kind = iota + 1
	GLTFSeparate
)

// Files lists the model files to upload as absolute paths: the model first,
// then any .bin files in sorted order.
type Files struct {
	Kind  Kind
	Paths []string
}

const maxBaseNameBytes = 64

var reservedConverterRest = regexp.MustCompile(`^[0-9]+\.[^.]+$`)

var textureExtensions = map[string]bool{".png": true, ".jpg": true, ".jpeg": true, ".webp": true}

// listFiles returns the names of the regular, non-hidden files directly in dir
// in sorted order.
func listFiles(dir string) ([]string, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var names []string
	for _, e := range entries {
		name := e.Name()
		if !e.Type().IsRegular() || strings.HasPrefix(name, ".") || strings.EqualFold(name, "Thumbs.db") {
			continue
		}
		names = append(names, name)
	}
	return names, nil
}

func splitExt(name string) (stem, ext string) {
	ext = filepath.Ext(name)
	return strings.TrimSuffix(name, ext), strings.ToLower(ext)
}

func isModelExt(ext string) bool {
	return ext == ".glb" || ext == ".gltf" || ext == ".bin"
}

func joinNames(names []string) string {
	switch len(names) {
	case 0:
		return ""
	case 1:
		return names[0]
	}
	return strings.Join(names[:len(names)-1], ", ") + " and " + names[len(names)-1]
}

// Resolve picks the model files in dir according to mode. baseName is the
// scene's fixed base name for SceneMode, the object name for NamedMode and
// ignored for AnyMode.
func Resolve(dir string, mode Mode, baseName string) (Files, error) {
	names, err := listFiles(dir)
	if err != nil {
		return Files{}, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Files{}, err
	}

	var glbs, gltfs, bins []string
	for _, name := range names {
		stem, ext := splitExt(name)
		if !isModelExt(ext) {
			continue
		}
		if mode != AnyMode {
			wantsBase := ext != ".glb" || mode == NamedMode
			if wantsBase && stem != baseName {
				if strings.EqualFold(stem, baseName) {
					return Files{}, fmt.Errorf("found %s but the model name is %s; the file name must match it exactly, apart from the extension's case", name, baseName)
				}
				continue
			}
		}
		switch ext {
		case ".glb":
			glbs = append(glbs, name)
		case ".gltf":
			gltfs = append(gltfs, name)
		case ".bin":
			bins = append(bins, name)
		}
	}

	var layout, noun string
	switch mode {
	case SceneMode:
		noun = "a scene"
		layout = fmt.Sprintf("one .glb or %s.gltf plus %s.bin", baseName, baseName)
	case NamedMode:
		noun = "an object"
		layout = fmt.Sprintf("%s.glb or %s.gltf plus %s.bin", baseName, baseName, baseName)
	default:
		noun = "a model"
		layout = "one .glb or one .gltf plus its .bin files"
	}

	if len(glbs) > 1 {
		return Files{}, fmt.Errorf("found %d .glb files: %s; %s directory holds exactly one .glb", len(glbs), joinNames(glbs), noun)
	}
	if len(gltfs) > 1 {
		return Files{}, fmt.Errorf("found %d .gltf files: %s; %s directory holds exactly one .gltf", len(gltfs), joinNames(gltfs), noun)
	}
	if mode != AnyMode && len(bins) > 1 {
		return Files{}, fmt.Errorf("found %d .bin files: %s; %s directory holds exactly one .bin", len(bins), joinNames(bins), noun)
	}

	separate := append(append([]string{}, gltfs...), bins...)
	if len(glbs) == 1 && len(separate) > 0 {
		all := append(append([]string{}, glbs...), separate...)
		prefix := "found "
		if len(all) == 2 {
			prefix = "found both "
		}
		return Files{}, fmt.Errorf("%s%s; %s directory holds %s, never both", prefix, joinNames(all), noun, layout)
	}
	if len(glbs) == 1 {
		return Files{Kind: GLB, Paths: []string{filepath.Join(abs, glbs[0])}}, nil
	}

	if len(gltfs) == 0 {
		switch {
		case len(bins) > 0 && mode == AnyMode:
			return Files{}, fmt.Errorf("found %s without a .gltf", joinNames(bins))
		case len(bins) > 0:
			return Files{}, fmt.Errorf("found %s without %s.gltf", bins[0], baseName)
		}
		return Files{}, fmt.Errorf("no model found in %s: expected %s", dir, layout)
	}
	if mode != AnyMode && len(bins) == 0 {
		return Files{}, fmt.Errorf("found %s without %s.bin", gltfs[0], baseName)
	}

	paths := []string{filepath.Join(abs, gltfs[0])}
	for _, b := range bins {
		paths = append(paths, filepath.Join(abs, b))
	}
	return Files{Kind: GLTFSeparate, Paths: paths}, nil
}

// IsReservedConverterName reports whether name has the shape <base>_<n>.<ext>,
// which the gateway's converter writes for images extracted from a .glb and
// refuses to receive beside it.
func IsReservedConverterName(name, base string) bool {
	rest, ok := strings.CutPrefix(name, base+"_")
	return ok && reservedConverterRest.MatchString(rest)
}

// GLBObjectBaseName returns the base name the gateway gives a dynamic
// object's converted files: every rune outside [A-Za-z0-9_-] becomes '_', at
// most 64 bytes, and "object" when nothing is left.
func GLBObjectBaseName(objectName string) string {
	var b strings.Builder
	for _, r := range objectName {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '_', r == '-':
			b.WriteRune(r)
		default:
			b.WriteByte('_')
		}
	}
	s := b.String()
	if len(s) > maxBaseNameBytes {
		s = s[:maxBaseNameBytes]
	}
	if s == "" {
		return "object"
	}
	return s
}

// Textures splits the other files in dir into the texture files to upload
// (.png, .jpg, .jpeg, .webp, matched case-insensitively) and the ones to skip.
// Model files and the names in exclude appear in neither list. Both lists are
// file names, sorted. An unreadable directory yields two empty lists.
func Textures(dir string, exclude []string) (keep []string, skipped []string) {
	names, err := listFiles(dir)
	if err != nil {
		return nil, nil
	}
	for _, name := range names {
		if excluded(name, exclude) {
			continue
		}
		_, ext := splitExt(name)
		switch {
		case textureExtensions[ext]:
			keep = append(keep, name)
		case isModelExt(ext):
		default:
			skipped = append(skipped, name)
		}
	}
	sort.Strings(keep)
	sort.Strings(skipped)
	return keep, skipped
}

func excluded(name string, exclude []string) bool {
	for _, e := range exclude {
		if strings.EqualFold(name, e) {
			return true
		}
	}
	return false
}
