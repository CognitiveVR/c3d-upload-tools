package object

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/manifest"
)

func listFake(t *testing.T) *fakegateway.Server {
	t.Helper()
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "scene-1", SDKFacingID: "scene-1", Versions: []fakegateway.Version{{ID: 31, VersionNumber: 1}}})
	return fake
}

func TestListReturnsTheRawArrayOfTheVersion(t *testing.T) {
	fake := listFake(t)
	client := newClient(t, fake)
	m := &manifest.Manifest{Objects: []manifest.Entry{manifest.DefaultEntry("a", "cube"), manifest.DefaultEntry("b", "ball")}}
	if err := UploadManifest(context.Background(), client, m, "scene-1", 1); err != nil {
		t.Fatal(err)
	}
	raw, err := List(context.Background(), client, 31)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(raw), "[") || !strings.Contains(string(raw), `"sdkId":"a"`) {
		t.Errorf("raw = %s", raw)
	}
	last := fake.Requests()[len(fake.Requests())-1]
	if last.Method != http.MethodGet || last.Path != "/v0/versions/31/objects" {
		t.Errorf("request = %+v", last)
	}
	missing, err := WithoutFiles(raw)
	if err != nil || len(missing) != 2 || missing[0].SDKID != "a" || missing[1].MeshName != "ball" {
		t.Errorf("WithoutFiles = %+v, %v", missing, err)
	}
}

func TestListUnknownVersionIsAnError(t *testing.T) {
	fake := listFake(t)
	if _, err := List(context.Background(), newClient(t, fake), 999); err == nil {
		t.Error("want an error for a 404")
	}
}

func TestListRejectsAnAnswerThatIsNotAnArray(t *testing.T) {
	fake := listFake(t)
	fake.FailWith(http.StatusOK, `{"objects":[]}`)
	if _, err := List(context.Background(), newClient(t, fake), 31); err == nil || !strings.Contains(err.Error(), "not a JSON array") {
		t.Errorf("err = %v", err)
	}
	fake.FailWith(http.StatusOK, "")
	if _, err := List(context.Background(), newClient(t, fake), 31); err == nil {
		t.Error("want an error for an empty answer")
	}
}

func TestIndentUsesTwoSpacesAndEndsWithANewline(t *testing.T) {
	got, err := Indent([]byte(`[{"a":1}]`))
	if err != nil || string(got) != "[\n  {\n    \"a\": 1\n  }\n]\n" {
		t.Errorf("got %q, %v", got, err)
	}
	if _, err := Indent([]byte(`[`)); err == nil {
		t.Error("want an error for invalid JSON")
	}
}

func TestWithoutFilesSkipsObjectsThatHaveFiles(t *testing.T) {
	raw := []byte(`[{"sdkId":"a","meshName":"m","files":[{"name":"m.glb","type":"glb"}]},{"sdkId":"b","meshName":"n","files":[]},{"sdkId":"c","meshName":"o"}]`)
	got, err := WithoutFiles(raw)
	if err != nil || len(got) != 2 || got[0].SDKID != "b" || got[1].SDKID != "c" {
		t.Errorf("got %+v, %v", got, err)
	}
}
