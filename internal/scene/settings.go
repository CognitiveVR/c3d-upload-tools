// Package scene plans and performs scene uploads.
package scene

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/localfile"
)

const settingsFileName = "settings.json"

var utf8BOM = []byte("\xef\xbb\xbf")

var openSettings = localfile.OpenRegular

// ErrNoSceneName reports that neither the caller nor an existing settings.json supplied a scene name.
var ErrNoSceneName = errors.New("scene name is required: pass --scene-name or add sceneName to settings.json")

// Settings is the content of a scene's settings.json. Extra holds every other
// top-level field of an existing file so that rebuilding keeps them.
type Settings struct {
	SceneName  string
	Scale      float64
	SDKVersion string
	Extra      map[string]json.RawMessage
}

// ReadSettings reads dir/settings.json, which must be a regular file, not a
// link. It returns nil, nil when the file does not exist.
func ReadSettings(dir string) (*Settings, error) {
	path := filepath.Join(dir, settingsFileName)
	if _, err := localfile.Stat(dir, settingsFileName); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, err
	}
	f, err := openSettings(path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	raw, err := io.ReadAll(f)
	_ = f.Close()
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", path, err)
	}
	raw = bytes.TrimPrefix(raw, utf8BOM)
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(raw, &fields); err != nil || fields == nil {
		if err == nil {
			err = errors.New("top level is not a JSON object")
		}
		return nil, fmt.Errorf("parse %s: %w", path, err)
	}
	s := &Settings{}
	known := []struct {
		key  string
		dest any
	}{{"scale", &s.Scale}, {"sceneName", &s.SceneName}, {"sdkVersion", &s.SDKVersion}}
	for _, k := range known {
		value, ok := fields[k.key]
		if !ok {
			continue
		}
		if err := json.Unmarshal(value, k.dest); err != nil {
			return nil, fmt.Errorf("parse %s: field %q: %w", path, k.key, err)
		}
		delete(fields, k.key)
	}
	if len(fields) > 0 {
		s.Extra = fields
	}
	return s, nil
}

// BuildSettings returns the settings.json bytes for a scene upload and the
// Settings they describe. A non-empty sceneName wins over the existing name;
// sdkVersion always replaces the existing value; a missing or zero scale becomes 1.
func BuildSettings(existing *Settings, sceneName, sdkVersion string) ([]byte, Settings, error) {
	out := Settings{Scale: 1}
	if existing != nil {
		out = *existing
		if out.Scale == 0 {
			out.Scale = 1
		}
		out.Extra = make(map[string]json.RawMessage, len(existing.Extra))
		for k, v := range existing.Extra {
			switch k {
			case "scale", "sceneName", "sdkVersion":
			default:
				out.Extra[k] = v
			}
		}
	}
	if sceneName != "" {
		out.SceneName = sceneName
	}
	if out.SceneName == "" {
		return nil, Settings{}, ErrNoSceneName
	}
	out.SDKVersion = sdkVersion

	data, err := encodeSettings(out)
	if err != nil {
		return nil, Settings{}, err
	}
	return data, out, nil
}

// encodeSettings writes scale, sceneName, sdkVersion, then Extra keys sorted,
// indented by two spaces with a trailing newline.
func encodeSettings(s Settings) ([]byte, error) {
	keys := make([]string, 0, len(s.Extra))
	for k := range s.Extra {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var compact bytes.Buffer
	compact.WriteByte('{')
	write := func(key string, value []byte) error {
		if compact.Len() > 1 {
			compact.WriteByte(',')
		}
		k, err := marshalNoEscape(key)
		if err != nil {
			return err
		}
		compact.Write(k)
		compact.WriteByte(':')
		compact.Write(value)
		return nil
	}
	for _, f := range []struct {
		key   string
		value any
	}{{"scale", s.Scale}, {"sceneName", s.SceneName}, {"sdkVersion", s.SDKVersion}} {
		v, err := marshalNoEscape(f.value)
		if err != nil {
			return nil, fmt.Errorf("encode settings: %w", err)
		}
		if err := write(f.key, v); err != nil {
			return nil, fmt.Errorf("encode settings: %w", err)
		}
	}
	for _, k := range keys {
		if err := write(k, s.Extra[k]); err != nil {
			return nil, fmt.Errorf("encode settings: %w", err)
		}
	}
	compact.WriteByte('}')

	var out bytes.Buffer
	if err := json.Indent(&out, compact.Bytes(), "", "  "); err != nil {
		return nil, fmt.Errorf("encode settings: %w", err)
	}
	out.WriteByte('\n')
	return out.Bytes(), nil
}

func marshalNoEscape(v any) ([]byte, error) {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil
}

// CheckSettingsWritable confirms, without changing settings.json, that
// WriteSettings can create a file in dir and replace an existing
// settings.json.
func CheckSettingsWritable(dir string) error {
	probe, err := os.CreateTemp(dir, ".settings-*")
	if err != nil {
		return fmt.Errorf("cannot write %s into %s, so the upload was not started: %w", settingsFileName, dir, err)
	}
	_ = probe.Close()
	if err := os.Remove(probe.Name()); err != nil {
		return fmt.Errorf("cannot write %s into %s, so the upload was not started: %w", settingsFileName, dir, err)
	}
	return checkExistingSettingsWritable(filepath.Join(dir, settingsFileName))
}

// WriteSettings writes data to dir/settings.json through a temporary file in
// dir followed by a rename, and never leaves the temporary file behind.
func WriteSettings(dir string, data []byte) error {
	tmp, err := os.CreateTemp(dir, ".settings-*.json")
	if err != nil {
		return fmt.Errorf("write %s: %w", settingsFileName, err)
	}
	tmpName := tmp.Name()
	_, err = tmp.Write(data)
	if closeErr := tmp.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Chmod(tmpName, 0o644)
	}
	if err == nil {
		err = os.Rename(tmpName, filepath.Join(dir, settingsFileName))
	}
	if err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("write %s: %w", settingsFileName, err)
	}
	return nil
}
