package config

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// EnvironmentSection holds the per-environment values of the config file.
type EnvironmentSection struct {
	DeveloperAPIKey string `yaml:"developer_api_key"`
	SceneID         string `yaml:"scene_id"`
}

// File is the parsed ~/.cognitive3d/upload.yaml.
type File struct {
	DefaultEnvironment string             `yaml:"default_environment"`
	Production         EnvironmentSection `yaml:"production"`
	Development        EnvironmentSection `yaml:"development"`
}

// DefaultFilePath returns <home>/.cognitive3d/upload.yaml.
func DefaultFilePath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("cannot locate the home directory for the default config file: %w", err)
	}
	return filepath.Join(home, ".cognitive3d", "upload.yaml"), nil
}

// LoadFile reads the config file at path. An empty path means the default
// location, where a missing file is not an error and yields (nil, nil). A
// path given explicitly must be readable.
func LoadFile(path string) (*File, error) {
	explicit := path != ""
	if !explicit {
		p, err := DefaultFilePath()
		if err != nil {
			return nil, nil
		}
		path = p
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if !explicit && errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("cannot read config file %s: %w", path, err)
	}
	var f File
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("cannot parse config file %s: %w", path, err)
	}
	return &f, nil
}
