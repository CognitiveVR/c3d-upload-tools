package object

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/manifest"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
)

var uuidV4 = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestNewUUIDIsLowercaseV4AndDiffers(t *testing.T) {
	a, b := NewUUID(), NewUUID()
	if !uuidV4.MatchString(a) || !uuidV4.MatchString(b) || a == b {
		t.Errorf("ids %q and %q", a, b)
	}
}

func TestResolveIDPrefersTheFlagThenTheManifestThenGenerates(t *testing.T) {
	m := &manifest.Manifest{Objects: []manifest.Entry{manifest.DefaultEntry("existing", "cube")}}

	id, src, err := ResolveID("from-flag", m, "cube")
	if err != nil || id != "from-flag" || src != Flag {
		t.Errorf("flag: %q %v %v", id, src, err)
	}
	id, src, err = ResolveID("", m, "cube")
	if err != nil || id != "existing" || src != ReusedFromManifest {
		t.Errorf("manifest: %q %v %v", id, src, err)
	}
	id, src, err = ResolveID("", m, "other")
	if err != nil || !uuidV4.MatchString(id) || src != Generated {
		t.Errorf("generated: %q %v %v", id, src, err)
	}
	id, src, err = ResolveID("", &manifest.Manifest{}, "cube")
	if err != nil || src != Generated || id == "" {
		t.Errorf("empty manifest: %q %v %v", id, src, err)
	}
}

func TestResolveIDRejectsAnInvalidFlagNamingTheRule(t *testing.T) {
	for _, bad := range []string{strings.Repeat("a", 300), "tab\tid", "naïve"} {
		_, _, err := ResolveID(bad, nil, "cube")
		if err == nil || !strings.Contains(err.Error(), "--object-id") {
			t.Errorf("ResolveID(%.20q) err = %v", bad, err)
		}
	}
}

func TestIDSourceStrings(t *testing.T) {
	for src, want := range map[IDSource]string{Flag: "--object-id", ReusedFromManifest: "manifest", Generated: "generated"} {
		if !strings.Contains(src.String(), want) {
			t.Errorf("%d = %q, want it to contain %q", int(src), src.String(), want)
		}
	}
}

func newClient(t *testing.T, fake *fakegateway.Server) *gateway.Client {
	t.Helper()
	c := gateway.New(fake.URL(), fake.Key(), "test", nil)
	return c
}

func TestUploadPostsEveryPartUnderFieldFileWithTheVersion(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "scene-1", SDKFacingID: "scene-1"})
	plan, err := Plan(copyFixture(t, "object-gltf"), "cube")
	if err != nil {
		t.Fatal(err)
	}
	if err := Upload(context.Background(), newClient(t, fake), plan, "scene-1", 3, nil, nil); err != nil {
		t.Fatal(err)
	}
	reqs := fake.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %+v", reqs)
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/v0/objects/scene-1/cube" || r.Query.Get("version") != "3" || !r.Complete {
		t.Errorf("request = %+v", r)
	}
	if len(r.Parts) != 3 {
		t.Fatalf("parts = %+v", r.Parts)
	}
	for _, p := range r.Parts {
		if p.Field != "file" {
			t.Errorf("part %s has field %q", p.Filename, p.Field)
		}
	}
}

func TestUploadEscapesTheMeshNameInThePath(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "scene-1", SDKFacingID: "scene-1"})
	plan, err := Plan(glbDir(t, "My Object (v2)"), "My Object (v2)")
	if err != nil {
		t.Fatal(err)
	}
	if err := Upload(context.Background(), newClient(t, fake), plan, "scene-1", 1, nil, nil); err != nil {
		t.Fatal(err)
	}
	r := fake.Requests()[0]
	if r.RawPath != "/v0/objects/scene-1/My%20Object%20%28v2%29" || r.Path != "/v0/objects/scene-1/My Object (v2)" {
		t.Errorf("raw path %q, path %q", r.RawPath, r.Path)
	}
	if r.Parts[0].Filename != "My Object (v2).glb" {
		t.Errorf("filename = %q", r.Parts[0].Filename)
	}
}

func TestUploadManifestPostsTheJSONBody(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "scene-1", SDKFacingID: "scene-1"})
	m := &manifest.Manifest{Objects: []manifest.Entry{manifest.DefaultEntry("id-1", "cube")}}
	if err := UploadManifest(context.Background(), newClient(t, fake), m, "scene-1", 2); err != nil {
		t.Fatal(err)
	}
	r := fake.Requests()[0]
	if r.Method != http.MethodPost || r.Path != "/v0/objects/scene-1" || r.Query.Get("version") != "2" ||
		r.Header.Get("Content-Type") != "application/json" {
		t.Errorf("request = %+v", r)
	}
	var got manifest.Manifest
	if err := json.Unmarshal(r.Body, &got); err != nil || len(got.Objects) != 1 || got.Objects[0].ID != "id-1" {
		t.Errorf("body = %s (err %v)", r.Body, err)
	}
}

func TestFailedUploadEndsTheProgressLine(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "scene-1", SDKFacingID: "scene-1"})
	fake.FailWith(http.StatusInternalServerError, "conversion failed")
	plan, err := Plan(copyFixture(t, "object-gltf"), "cube")
	if err != nil {
		t.Fatal(err)
	}
	var terminal bytes.Buffer
	progress := ui.NewProgress(&terminal, plan.TotalBytes, "cube")
	if err := Upload(context.Background(), newClient(t, fake), plan, "scene-1", 1, nil, progress); err == nil {
		t.Fatal("want an error for a 500 from the gateway")
	}
	if !strings.HasPrefix(terminal.String(), "\r") || !strings.HasSuffix(terminal.String(), "\n") {
		t.Errorf("terminal = %q, want a drawn line ended by a newline", terminal.String())
	}
}

func TestUploadToAnUnknownSceneFails(t *testing.T) {
	fake := fakegateway.New(t)
	plan, err := Plan(copyFixture(t, "object-gltf"), "cube")
	if err != nil {
		t.Fatal(err)
	}
	if err := Upload(context.Background(), newClient(t, fake), plan, "nope", 1, nil, nil); err == nil {
		t.Error("want an error for a 401 from the gateway")
	}
}
