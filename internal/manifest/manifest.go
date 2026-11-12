// Package manifest reads, edits, validates and writes the local object
// manifest file. Fields other than the ones on Manifest and Entry are dropped
// on load.
package manifest

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"unicode/utf8"
)

const maxLength = 255

// Manifest is the content of a <scene_id>_object_manifest.json file.
type Manifest struct {
	Objects []Entry `json:"objects"`
}

// Entry is one dynamic object instance. Field order is the key order written by Save.
type Entry struct {
	ID              string    `json:"id"`
	Mesh            string    `json:"mesh"`
	Name            string    `json:"name"`
	ScaleCustom     []float64 `json:"scaleCustom,omitempty"`
	InitialPosition []float64 `json:"initialPosition,omitempty"`
	InitialRotation []float64 `json:"initialRotation,omitempty"`
	IsController    *bool     `json:"isController,omitempty"`
	Scale           *float64  `json:"scale,omitempty"`
	InitialScale    []float64 `json:"initialScale,omitempty"`
	ExternalID      *string   `json:"externalId,omitempty"`
	FileType        *string   `json:"fileType,omitempty"`
}

// MergeOutcome reports what Upsert did.
type MergeOutcome int

const (
	Appended MergeOutcome = iota + 1
	Updated
)

func (o MergeOutcome) String() string {
	switch o {
	case Appended:
		return "appended"
	case Updated:
		return "updated"
	default:
		return fmt.Sprintf("MergeOutcome(%d)", int(o))
	}
}

// DefaultEntry returns an entry named after its mesh with unit scale, origin
// position and identity rotation.
func DefaultEntry(id, mesh string) Entry {
	return Entry{
		ID:              id,
		Mesh:            mesh,
		Name:            mesh,
		ScaleCustom:     []float64{1, 1, 1},
		InitialPosition: []float64{0, 0, 0},
		InitialRotation: []float64{0, 0, 0, 1},
	}
}

// DefaultPath returns the manifest file name for a scene, relative to the working directory.
func DefaultPath(sceneID string) string {
	return sceneID + "_object_manifest.json"
}

// Load reads and parses the manifest at path. A missing file is an error.
func Load(path string) (*Manifest, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	raw = bytes.TrimPrefix(raw, []byte("\xef\xbb\xbf"))
	trimmed := bytes.TrimLeft(raw, " \t\r\n")
	if len(trimmed) > 0 && trimmed[0] == '[' {
		return nil, fmt.Errorf("parse %s: the file looks like an object list, not a manifest; write a manifest with \"object list --write-manifest\"", path)
	}
	if len(trimmed) == 0 || trimmed[0] != '{' {
		return nil, fmt.Errorf("parse %s: top level is not a JSON object", path)
	}
	var m Manifest
	if err := json.Unmarshal(raw, &m); err != nil {
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	if m.Objects == nil {
		m.Objects = []Entry{}
	}
	return &m, nil
}

// LoadOrEmpty is Load, except that a missing file yields an empty manifest.
func LoadOrEmpty(path string) (*Manifest, error) {
	m, err := Load(path)
	if errors.Is(err, fs.ErrNotExist) {
		return &Manifest{Objects: []Entry{}}, nil
	}
	return m, err
}

// Upsert sets the mesh of the entry whose id matches without regard to case,
// keeping its stored id and every other field except a name that was the old
// mesh name, which follows the new one; or appends DefaultEntry(id, mesh).
func (m *Manifest) Upsert(id, mesh string) MergeOutcome {
	return m.Adopt(DefaultEntry(id, mesh))
}

// Adopt is Upsert with e as the entry appended when no entry has its id.
func (m *Manifest) Adopt(e Entry) MergeOutcome {
	for i := range m.Objects {
		if strings.EqualFold(m.Objects[i].ID, e.ID) {
			if m.Objects[i].Name == m.Objects[i].Mesh {
				m.Objects[i].Name = e.Mesh
			}
			m.Objects[i].Mesh = e.Mesh
			return Updated
		}
	}
	m.Objects = append(m.Objects, e)
	return Appended
}

// EntryByID returns the entry whose id equals id without regard to case, the
// same match Upsert uses.
func (m *Manifest) EntryByID(id string) (Entry, bool) {
	for _, e := range m.Objects {
		if strings.EqualFold(e.ID, id) {
			return e, true
		}
	}
	return Entry{}, false
}

// IDForMesh returns the id of the first entry whose mesh equals mesh exactly.
func (m *Manifest) IDForMesh(mesh string) (string, bool) {
	for _, e := range m.Objects {
		if e.Mesh == mesh {
			return e.ID, true
		}
	}
	return "", false
}

// ValidateID reports whether id is 1 to 255 printable ASCII characters.
func ValidateID(id string) error {
	if len(id) == 0 || len(id) > maxLength {
		return fmt.Errorf("id must be 1 to %d characters, got %d", maxLength, len(id))
	}
	for i := 0; i < len(id); i++ {
		if id[i] < 0x20 || id[i] > 0x7e {
			return fmt.Errorf("id must contain only printable ASCII characters, found byte 0x%02x at offset %d", id[i], i)
		}
	}
	return nil
}

func validateLatin1(field, value string) error {
	n := utf8.RuneCountInString(value)
	if n == 0 || n > maxLength {
		return fmt.Errorf("%s must be 1 to %d characters, got %d", field, maxLength, n)
	}
	for _, r := range value {
		if r < 0x20 || r > 0xff {
			return fmt.Errorf("%s must contain only Latin-1 characters (U+0020 to U+00FF), found %U", field, r)
		}
	}
	return nil
}

func validateLength(field string, values []float64, want int) error {
	if values != nil && len(values) != want {
		return fmt.Errorf("%s must have %d components, got %v", field, want, values)
	}
	return nil
}

// Validate checks every entry and returns all problems found, each naming the
// entry's id and the field.
func (m *Manifest) Validate() error {
	var errs []error
	for i, e := range m.Objects {
		fail := func(err error) {
			errs = append(errs, fmt.Errorf("object %q: %w", e.ID, err))
		}
		if err := ValidateID(e.ID); err != nil {
			fail(err)
		} else {
			for _, prior := range m.Objects[:i] {
				if strings.EqualFold(prior.ID, e.ID) {
					fail(fmt.Errorf("id duplicates %q (ids are compared without case)", prior.ID))
					break
				}
			}
		}
		for _, check := range []error{
			validateLatin1("mesh", e.Mesh),
			validateLatin1("name", e.Name),
			validateLength("scaleCustom", e.ScaleCustom, 3),
			validateLength("initialPosition", e.InitialPosition, 3),
			validateLength("initialRotation", e.InitialRotation, 4),
			validateLength("initialScale", e.InitialScale, 3),
		} {
			if check != nil {
				fail(check)
			}
		}
	}
	return errors.Join(errs...)
}

// Save writes the manifest to path with two-space indentation and a trailing
// newline, through a temporary file in the same directory and a rename, and
// never leaves the temporary file behind.
func (m *Manifest) Save(path string) error {
	out := *m
	if out.Objects == nil {
		out.Objects = []Entry{}
	}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(out); err != nil {
		return fmt.Errorf("encode manifest: %w", err)
	}

	tmp, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+"-*.tmp")
	if err != nil {
		return fmt.Errorf("write %s: %w", path, err)
	}
	tmpName := tmp.Name()
	_, err = tmp.Write(buf.Bytes())
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmpName, 0o644)
	}
	if err == nil {
		err = os.Rename(tmpName, path)
	}
	if err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", path, err)
	}
	return nil
}
