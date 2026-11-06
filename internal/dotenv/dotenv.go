// Package dotenv parses .env files with the same rules as the bash loader
// (load_env_file in upload-utils.sh).
package dotenv

import (
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strings"
)

const whitespace = " \t\v\f\r"

var keyPattern = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// Parse reads KEY=value lines and returns the variables found.
//
// Lines that are blank, start with '#', have no '=' or carry an invalid key
// name are skipped silently. Keys and values are trimmed of whitespace, one
// matching pair of surrounding quotes is removed from a value, and nothing is
// expanded or unescaped. When a key repeats, the last value wins.
func Parse(r io.Reader) (map[string]string, error) {
	data, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}
	text := strings.TrimPrefix(string(data), "\xef\xbb\xbf")
	text = strings.ReplaceAll(text, "\r\n", "\n")
	text = strings.ReplaceAll(text, "\r", "\n")

	values := make(map[string]string)
	for _, line := range strings.Split(text, "\n") {
		trimmed := strings.Trim(line, whitespace)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		key = strings.Trim(key, whitespace)
		if rest, ok := strings.CutPrefix(key, "export "); ok {
			key = strings.Trim(rest, whitespace)
		}
		if !keyPattern.MatchString(key) {
			continue
		}
		values[key] = unquote(strings.Trim(value, whitespace))
	}
	return values, nil
}

func unquote(value string) string {
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if first == last && (first == '"' || first == '\'') {
			return value[1 : len(value)-1]
		}
	}
	return value
}

// LoadCurrentDirectory parses ./.env. A missing .env, or one that is not a
// regular file (such as a virtualenv directory), yields an empty map and no
// error.
func LoadCurrentDirectory() (map[string]string, error) {
	if info, err := os.Stat(".env"); err == nil && !info.Mode().IsRegular() {
		return map[string]string{}, nil
	}
	file, err := os.Open(".env")
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("read .env: %w", err)
	}
	values, parseErr := Parse(file)
	closeErr := file.Close()
	if parseErr != nil {
		return nil, fmt.Errorf("read .env: %w", parseErr)
	}
	if closeErr != nil {
		return nil, fmt.Errorf("read .env: %w", closeErr)
	}
	return values, nil
}
