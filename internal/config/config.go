// Package config resolves CLI settings from flags, the process environment,
// a .env file and ~/.cognitive3d/upload.yaml, in that order of precedence.
package config

import (
	"errors"
	"fmt"
	"strings"
	"time"
)

const (
	prodBaseURL = "https://data.cognitive3d.com/v0"
	devBaseURL  = "https://data.c3ddev.com/v0"
)

// Flags carries the persistent flag values; the zero value of a field means
// the flag was not given.
type Flags struct {
	Environment string
	GatewayURL  string
	APIKey      string
	SceneID     string
	Format      string
	ConfigPath  string
	Timeout     time.Duration
	Verbose     bool
	Quiet       bool
}

// Settings is the fully resolved configuration. Environment is always "prod"
// or "dev"; a zero Timeout means no deadline.
type Settings struct {
	Environment string
	GatewayURL  string
	APIKey      string
	SceneID     string
	Format      string
	Timeout     time.Duration
	Verbose     bool
	Quiet       bool
	ConfigPath  string
}

// normalizeEnvironment maps every accepted spelling to "prod" or "dev".
func normalizeEnvironment(value string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "prod", "production":
		return "prod", nil
	case "dev", "development":
		return "dev", nil
	}
	return "", fmt.Errorf("unknown environment %q: use production, prod, development or dev", value)
}

// BaseURL returns the gateway base URL for an environment spelling.
func BaseURL(environment string) (string, error) {
	env, err := normalizeEnvironment(environment)
	if err != nil {
		return "", err
	}
	if env == "dev" {
		return devBaseURL, nil
	}
	return prodBaseURL, nil
}

// Resolve merges flags, the process environment (env), the .env map and the
// config file. A missing API key is not an error here; see RequireAPIKey.
func Resolve(flags Flags, env func(string) string, dotenv map[string]string, file *File) (Settings, error) {
	if env == nil {
		env = func(string) string { return "" }
	}
	lookup := func(name string) string {
		if v := env(name); v != "" {
			return v
		}
		return dotenv[name]
	}

	rawEnvironment := firstNonEmpty(flags.Environment, lookup("C3D_DEFAULT_ENVIRONMENT"), lookup("C3D_ENV"))
	if rawEnvironment == "" && file != nil {
		rawEnvironment = file.DefaultEnvironment
	}
	if rawEnvironment == "" {
		rawEnvironment = "prod"
	}
	environment, err := normalizeEnvironment(rawEnvironment)
	if err != nil {
		return Settings{}, err
	}

	var section EnvironmentSection
	if file != nil {
		section = file.Production
		if environment == "dev" {
			section = file.Development
		}
	}

	gatewayURL := firstNonEmpty(flags.GatewayURL, lookup("C3D_UPLOAD_GATEWAY_URL"))
	if gatewayURL == "" {
		if gatewayURL, err = BaseURL(environment); err != nil {
			return Settings{}, err
		}
	}

	return Settings{
		Environment: environment,
		GatewayURL:  strings.TrimRight(gatewayURL, "/"),
		APIKey:      firstNonEmpty(flags.APIKey, lookup("C3D_DEVELOPER_API_KEY"), section.DeveloperAPIKey),
		SceneID:     firstNonEmpty(flags.SceneID, lookup("C3D_SCENE_ID"), section.SceneID),
		Format:      firstNonEmpty(flags.Format, "text"),
		Timeout:     flags.Timeout,
		Verbose:     flags.Verbose,
		Quiet:       flags.Quiet,
		ConfigPath:  flags.ConfigPath,
	}, nil
}

// RequireAPIKey fails when no developer key was resolved. Commands call it
// immediately before their first network call so dry runs work without one.
func RequireAPIKey(s Settings) error {
	if s.APIKey == "" {
		return errors.New("C3D_DEVELOPER_API_KEY is not set. Please set it with: export C3D_DEVELOPER_API_KEY=your_api_key")
	}
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if v != "" {
			return v
		}
	}
	return ""
}
