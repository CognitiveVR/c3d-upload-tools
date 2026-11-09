package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func envMap(m map[string]string) func(string) string {
	return func(k string) string { return m[k] }
}

func fileWith(def, prodKey, prodScene, devKey, devScene string) *File {
	f := &File{DefaultEnvironment: def}
	f.Production.DeveloperAPIKey = prodKey
	f.Production.SceneID = prodScene
	f.Development.DeveloperAPIKey = devKey
	f.Development.SceneID = devScene
	return f
}

func TestResolveAPIKeyPrecedence(t *testing.T) {
	file := fileWith("", "file-key", "", "", "")
	tests := []struct {
		name   string
		flags  Flags
		env    map[string]string
		dotenv map[string]string
		file   *File
		want   string
	}{
		{"flag wins", Flags{APIKey: "flag"}, map[string]string{"C3D_DEVELOPER_API_KEY": "env"}, map[string]string{"C3D_DEVELOPER_API_KEY": "dot"}, file, "flag"},
		{"env beats dotenv", Flags{}, map[string]string{"C3D_DEVELOPER_API_KEY": "env"}, map[string]string{"C3D_DEVELOPER_API_KEY": "dot"}, file, "env"},
		{"dotenv beats file", Flags{}, nil, map[string]string{"C3D_DEVELOPER_API_KEY": "dot"}, file, "dot"},
		{"file last", Flags{}, nil, nil, file, "file-key"},
		{"none", Flags{}, nil, nil, nil, ""},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Resolve(tc.flags, envMap(tc.env), tc.dotenv, tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if s.APIKey != tc.want {
				t.Fatalf("APIKey = %q, want %q", s.APIKey, tc.want)
			}
		})
	}
}

func TestResolveSceneIDPrecedence(t *testing.T) {
	file := fileWith("", "", "file-scene", "", "")
	tests := []struct {
		name   string
		flags  Flags
		env    map[string]string
		dotenv map[string]string
		want   string
	}{
		{"flag wins", Flags{SceneID: "flag"}, map[string]string{"C3D_SCENE_ID": "env"}, map[string]string{"C3D_SCENE_ID": "dot"}, "flag"},
		{"env beats dotenv", Flags{}, map[string]string{"C3D_SCENE_ID": "env"}, map[string]string{"C3D_SCENE_ID": "dot"}, "env"},
		{"dotenv beats file", Flags{}, nil, map[string]string{"C3D_SCENE_ID": "dot"}, "dot"},
		{"file last", Flags{}, nil, nil, "file-scene"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Resolve(tc.flags, envMap(tc.env), tc.dotenv, file)
			if err != nil {
				t.Fatal(err)
			}
			if s.SceneID != tc.want {
				t.Fatalf("SceneID = %q, want %q", s.SceneID, tc.want)
			}
		})
	}
}

func TestResolveEnvironmentPrecedence(t *testing.T) {
	file := fileWith("development", "", "", "", "")
	tests := []struct {
		name   string
		flags  Flags
		env    map[string]string
		dotenv map[string]string
		file   *File
		want   string
	}{
		{"flag wins", Flags{Environment: "prod"}, map[string]string{"C3D_DEFAULT_ENVIRONMENT": "dev"}, nil, file, "prod"},
		{"default environment variable", Flags{}, map[string]string{"C3D_DEFAULT_ENVIRONMENT": "prod", "C3D_ENV": "dev"}, nil, file, "prod"},
		{"dotenv default variable beats process C3D_ENV", Flags{}, map[string]string{"C3D_ENV": "dev"}, map[string]string{"C3D_DEFAULT_ENVIRONMENT": "prod"}, nil, "prod"},
		{"C3D_ENV after default variable", Flags{}, map[string]string{"C3D_ENV": "prod"}, nil, file, "prod"},
		{"dotenv default variable", Flags{}, nil, map[string]string{"C3D_DEFAULT_ENVIRONMENT": "prod"}, file, "prod"},
		{"dotenv C3D_ENV", Flags{}, nil, map[string]string{"C3D_ENV": "prod"}, file, "prod"},
		{"file default", Flags{}, nil, nil, file, "dev"},
		{"built-in default", Flags{}, nil, nil, nil, "prod"},
		{"built-in default with empty file", Flags{}, nil, nil, &File{}, "prod"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Resolve(tc.flags, envMap(tc.env), tc.dotenv, tc.file)
			if err != nil {
				t.Fatal(err)
			}
			if s.Environment != tc.want {
				t.Fatalf("Environment = %q, want %q", s.Environment, tc.want)
			}
		})
	}
}

func TestResolveNormalizesEnvironmentSpellings(t *testing.T) {
	for in, want := range map[string]string{
		"production": "prod", "prod": "prod", "development": "dev", "dev": "dev",
		"PROD": "prod", "Production": "prod", "DEV": "dev", "Development": "dev",
	} {
		s, err := Resolve(Flags{Environment: in}, envMap(nil), nil, nil)
		if err != nil {
			t.Fatalf("%q: %v", in, err)
		}
		if s.Environment != want {
			t.Fatalf("%q normalized to %q, want %q", in, s.Environment, want)
		}
	}
}

func TestResolveRejectsUnknownEnvironment(t *testing.T) {
	badFile := fileWith("staging", "", "", "", "")
	badFile.Path = "/cfg/upload.yaml"
	sources := map[string]struct {
		flags  Flags
		env    map[string]string
		dotenv map[string]string
		file   *File
		source string
	}{
		"flag":   {Flags{Environment: "staging"}, nil, nil, nil, "--env"},
		"env":    {Flags{}, map[string]string{"C3D_ENV": "staging"}, nil, nil, "C3D_ENV in the environment"},
		"dotenv": {Flags{}, nil, map[string]string{"C3D_DEFAULT_ENVIRONMENT": "staging"}, nil, "C3D_DEFAULT_ENVIRONMENT in .env"},
		"file":   {Flags{}, nil, nil, badFile, "/cfg/upload.yaml"},
	}
	for name, tc := range sources {
		t.Run(name, func(t *testing.T) {
			_, err := Resolve(tc.flags, envMap(tc.env), tc.dotenv, tc.file)
			if err == nil {
				t.Fatal("expected error")
			}
			for _, part := range []string{tc.source, `"staging"`, "production", "prod", "development", "dev"} {
				if !strings.Contains(err.Error(), part) {
					t.Fatalf("error %q does not mention %s", err, part)
				}
			}
		})
	}
}

func TestResolveFileValuesApplyOnlyToResolvedEnvironment(t *testing.T) {
	file := fileWith("", "prod-key", "prod-scene", "dev-key", "dev-scene")
	s, err := Resolve(Flags{Environment: "dev"}, envMap(nil), nil, file)
	if err != nil {
		t.Fatal(err)
	}
	if s.APIKey != "dev-key" || s.SceneID != "dev-scene" {
		t.Fatalf("dev got %q/%q", s.APIKey, s.SceneID)
	}
	s, err = Resolve(Flags{Environment: "production"}, envMap(nil), nil, file)
	if err != nil {
		t.Fatal(err)
	}
	if s.APIKey != "prod-key" || s.SceneID != "prod-scene" {
		t.Fatalf("prod got %q/%q", s.APIKey, s.SceneID)
	}
}

func TestResolveGatewayURL(t *testing.T) {
	tests := []struct {
		name   string
		flags  Flags
		env    map[string]string
		dotenv map[string]string
		want   string
	}{
		{"prod default", Flags{}, nil, nil, "https://data.cognitive3d.com/v0"},
		{"dev default", Flags{Environment: "dev"}, nil, nil, "https://data.c3ddev.com/v0"},
		{"env override", Flags{}, map[string]string{"C3D_UPLOAD_GATEWAY_URL": "https://env.test/v0"}, nil, "https://env.test/v0"},
		{"dotenv override", Flags{}, nil, map[string]string{"C3D_UPLOAD_GATEWAY_URL": "https://dot.test/v0"}, "https://dot.test/v0"},
		{"flag beats env", Flags{GatewayURL: "https://flag.test/v0"}, map[string]string{"C3D_UPLOAD_GATEWAY_URL": "https://env.test/v0"}, nil, "https://flag.test/v0"},
		{"trailing slash trimmed", Flags{GatewayURL: "https://flag.test/v0/"}, nil, nil, "https://flag.test/v0"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			s, err := Resolve(tc.flags, envMap(tc.env), tc.dotenv, nil)
			if err != nil {
				t.Fatal(err)
			}
			if s.GatewayURL != tc.want {
				t.Fatalf("GatewayURL = %q, want %q", s.GatewayURL, tc.want)
			}
		})
	}
}

func TestResolvePassThroughDefaults(t *testing.T) {
	s, err := Resolve(Flags{}, envMap(nil), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Format != "text" || s.Timeout != 0 || s.Verbose || s.Quiet || s.ConfigPath != "" {
		t.Fatalf("unexpected defaults: %+v", s)
	}
	s, err = Resolve(Flags{Format: "json", Timeout: 5 * time.Second, Verbose: true, Quiet: true, ConfigPath: "x.yaml"}, envMap(nil), nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if s.Format != "json" || s.Timeout != 5*time.Second || !s.Verbose || !s.Quiet || s.ConfigPath != "x.yaml" {
		t.Fatalf("flags not carried: %+v", s)
	}
}

func TestResolveWithoutKeyThenRequireAPIKey(t *testing.T) {
	s, err := Resolve(Flags{}, envMap(nil), nil, nil)
	if err != nil {
		t.Fatalf("Resolve must not fail without a key: %v", err)
	}
	err = RequireAPIKey(s)
	want := "no developer API key: set C3D_DEVELOPER_API_KEY with: export C3D_DEVELOPER_API_KEY=your_api_key (or pass --api-key your_api_key)"
	if err == nil || err.Error() != want {
		t.Fatalf("RequireAPIKey = %v, want %q", err, want)
	}
	s.APIKey = "k"
	if err := RequireAPIKey(s); err != nil {
		t.Fatal(err)
	}
}

func TestBaseURL(t *testing.T) {
	for in, want := range map[string]string{
		"prod": "https://data.cognitive3d.com/v0", "production": "https://data.cognitive3d.com/v0",
		"dev": "https://data.c3ddev.com/v0", "development": "https://data.c3ddev.com/v0",
	} {
		got, err := BaseURL(in)
		if err != nil || got != want {
			t.Fatalf("BaseURL(%q) = %q, %v; want %q", in, got, err, want)
		}
	}
	if _, err := BaseURL("staging"); err == nil || !strings.Contains(err.Error(), `"staging"`) {
		t.Fatalf("expected error naming staging, got %v", err)
	}
}

const sampleYAML = `default_environment: dev
production:
  developer_api_key: pk
  scene_id: ps
development:
  developer_api_key: dk
  scene_id: ds
`

func TestLoadFileParses(t *testing.T) {
	p := filepath.Join(t.TempDir(), "upload.yaml")
	if err := os.WriteFile(p, []byte(sampleYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	if f == nil || f.DefaultEnvironment != "dev" || f.Production.DeveloperAPIKey != "pk" || f.Production.SceneID != "ps" ||
		f.Development.DeveloperAPIKey != "dk" || f.Development.SceneID != "ds" {
		t.Fatalf("parsed %+v", f)
	}
}

func TestLoadFileExplicitMissingIsError(t *testing.T) {
	p := filepath.Join(t.TempDir(), "nope.yaml")
	f, err := LoadFile(p)
	if err == nil || f != nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("got %v, %v; want error naming %s", f, err, p)
	}
}

func TestLoadFileUnreadableIsError(t *testing.T) {
	p := t.TempDir()
	if _, err := LoadFile(p); err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("expected error naming %s, got %v", p, err)
	}
}

func TestLoadFileInvalidYAMLNamesPath(t *testing.T) {
	p := filepath.Join(t.TempDir(), "bad.yaml")
	if err := os.WriteFile(p, []byte("production: [unclosed"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadFile(p); err == nil || !strings.Contains(err.Error(), p) {
		t.Fatalf("expected error naming %s, got %v", p, err)
	}
}

func TestLoadFileEmptyPathMissingDefaultIsNil(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	f, err := LoadFile("")
	if err != nil || f != nil {
		t.Fatalf("got %v, %v; want nil, nil", f, err)
	}
}

func TestLoadFileEmptyPathReadsDefault(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	if err := os.MkdirAll(filepath.Join(dir, ".cognitive3d"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".cognitive3d", "upload.yaml"), []byte(sampleYAML), 0o600); err != nil {
		t.Fatal(err)
	}
	f, err := LoadFile("")
	if err != nil || f == nil || f.DefaultEnvironment != "dev" {
		t.Fatalf("got %+v, %v", f, err)
	}
}

func TestDefaultFilePath(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	got, err := DefaultFilePath()
	if err != nil {
		t.Fatal(err)
	}
	if want := filepath.Join(dir, ".cognitive3d", "upload.yaml"); got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestResolveGatewayURLSafety(t *testing.T) {
	const warnFmt = "Gateway %s is not a Cognitive3D host; your API key will be sent there."
	rejected := map[string]string{
		"http://evil.example/v0": "https is required",
		"ftp://localhost/v0":     "https is required",
		"not a url":              "no host",
		"https:///v0":            "no host",
		"http://[::1":            "not a valid URL",
	}
	for raw, part := range rejected {
		_, err := Resolve(Flags{}, envMap(nil), map[string]string{"C3D_UPLOAD_GATEWAY_URL": raw}, nil)
		if err == nil || !strings.Contains(err.Error(), part) || !strings.Contains(err.Error(), raw) {
			t.Fatalf("%q: got %v, want error naming the URL and %q", raw, err, part)
		}
	}
	accepted := map[string]string{
		"http://localhost:8080/v0":     "localhost",
		"http://127.0.0.1:8080/v0":     "127.0.0.1",
		"http://[::1]:8080/v0":         "::1",
		"https://proxy.example.com/v0": "proxy.example.com",
		"http://LOCALHOST:8080/v0":     "localhost",
	}
	for raw, host := range accepted {
		s, err := Resolve(Flags{GatewayURL: raw}, envMap(nil), nil, nil)
		if err != nil {
			t.Fatalf("%q: %v", raw, err)
		}
		if len(s.Warnings) != 1 || s.Warnings[0] != fmt.Sprintf(warnFmt, host) {
			t.Fatalf("%q: warnings %q", raw, s.Warnings)
		}
	}
	for _, e := range []string{"prod", "dev"} {
		s, err := Resolve(Flags{Environment: e}, envMap(nil), nil, nil)
		if err != nil || len(s.Warnings) != 0 {
			t.Fatalf("%s: warnings %q, err %v", e, s.Warnings, err)
		}
	}
}

func TestResolveGatewayHostCaseInsensitive(t *testing.T) {
	for _, raw := range []string{"https://DATA.COGNITIVE3D.COM/v0", "https://Data.C3DDev.com/v0"} {
		s, err := Resolve(Flags{GatewayURL: raw}, envMap(nil), nil, nil)
		if err != nil || len(s.Warnings) != 0 {
			t.Fatalf("%q: warnings %q, err %v", raw, s.Warnings, err)
		}
	}
}

func TestResolveForeignDotenvGatewayWithTrustedKeyIsRejected(t *testing.T) {
	foreign := map[string]string{"C3D_UPLOAD_GATEWAY_URL": "https://proxy.example.com/v0"}
	const wantFmt = "C3D_UPLOAD_GATEWAY_URL in .env points at proxy.example.com, which is not a Cognitive3D gateway, while your API key comes from %s. Pass --gateway-url explicitly if you mean it."

	cases := []struct {
		name   string
		flags  Flags
		env    map[string]string
		dotenv map[string]string
		file   *File
		source string
	}{
		{"flag key", Flags{APIKey: "k"}, nil, foreign, nil, "the --api-key flag"},
		{"process environment key", Flags{}, map[string]string{"C3D_DEVELOPER_API_KEY": "k"}, foreign, nil, "the process environment"},
		{"config file key", Flags{}, nil, foreign, fileWith("", "k", "", "", ""), "the config file"},
	}
	for _, tc := range cases {
		_, err := Resolve(tc.flags, envMap(tc.env), tc.dotenv, tc.file)
		if err == nil || err.Error() != fmt.Sprintf(wantFmt, tc.source) {
			t.Errorf("%s: err = %v, want %q", tc.name, err, fmt.Sprintf(wantFmt, tc.source))
		}
	}
}

func TestResolveForeignGatewayKeepsWarningWhenSourcesAreAligned(t *testing.T) {
	const url = "https://proxy.example.com/v0"
	cases := []struct {
		name   string
		flags  Flags
		env    map[string]string
		dotenv map[string]string
	}{
		{"flag url", Flags{GatewayURL: url}, map[string]string{"C3D_DEVELOPER_API_KEY": "k"}, nil},
		{"environment url", Flags{}, map[string]string{"C3D_UPLOAD_GATEWAY_URL": url, "C3D_DEVELOPER_API_KEY": "k"}, nil},
		{"dotenv url and dotenv key", Flags{}, nil, map[string]string{"C3D_UPLOAD_GATEWAY_URL": url, "C3D_DEVELOPER_API_KEY": "k"}},
		{"dotenv url and no key", Flags{}, nil, map[string]string{"C3D_UPLOAD_GATEWAY_URL": url}},
	}
	for _, tc := range cases {
		s, err := Resolve(tc.flags, envMap(tc.env), tc.dotenv, nil)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(s.Warnings) != 1 || !strings.Contains(s.Warnings[0], "proxy.example.com") {
			t.Errorf("%s: warnings = %q, want the foreign-host warning", tc.name, s.Warnings)
		}
	}
}
