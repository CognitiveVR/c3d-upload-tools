package ego

import (
	"context"
	"net/http"
	"net/http/httptest"
	"reflect"
	"testing"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
)

const egoSceneID = "ego-scene"

func egoFake(t *testing.T) (*fakegateway.Server, *gateway.Client) {
	t.Helper()
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: egoSceneID, Versions: []fakegateway.Version{{ID: 1, VersionNumber: 1}, {ID: 2, VersionNumber: 2}}})
	return fake, gateway.New(fake.URL(), fake.Key(), "test", nil)
}

func uploadHead(t *testing.T, client *gateway.Client, version int) UploadResult {
	t.Helper()
	plan, err := Plan(copyFixture(t, "ego-head"), "head")
	if err != nil {
		t.Fatal(err)
	}
	result, err := Upload(context.Background(), client, plan, egoSceneID, version, nil, nil)
	if err != nil {
		t.Fatalf("Upload: %v", err)
	}
	return result
}

func TestUploadPostsEveryPartUnderFileToTheSlotAndVersion(t *testing.T) {
	fake, client := egoFake(t)
	result := uploadHead(t, client, 2)
	if want := (UploadResult{Slot: "head", Model: "head.gltf", FileCount: 4}); result != want {
		t.Errorf("result = %+v, want %+v", result, want)
	}
	reqs := fake.Requests()
	if len(reqs) != 1 {
		t.Fatalf("requests = %+v", reqs)
	}
	r := reqs[0]
	if r.Method != http.MethodPost || r.Path != "/v0/scenes/"+egoSceneID+"/egos/head" || r.Query.Get("version") != "2" || !r.Complete {
		t.Errorf("request = %s %s ?%v complete=%v", r.Method, r.Path, r.Query, r.Complete)
	}
	var names []string
	for _, p := range r.Parts {
		if p.Field != "file" {
			t.Errorf("part %s has field %q", p.Filename, p.Field)
		}
		names = append(names, p.Filename)
	}
	if want := []string{"head.gltf", "head.bin", "head_albedo.png", "ego.json"}; !reflect.DeepEqual(names, want) {
		t.Errorf("parts = %v, want %v", names, want)
	}
}

func TestListReturnsTheSlotsOfTheVersion(t *testing.T) {
	_, client := egoFake(t)
	uploadHead(t, client, 2)
	listing, err := List(context.Background(), client, egoSceneID, 2)
	if err != nil {
		t.Fatal(err)
	}
	want := Listing{SceneID: egoSceneID, Version: 2, Egos: []Slot{{
		Key: "head", Model: "head.gltf", Files: []string{"head.gltf", "head.bin", "head_albedo.png", "ego.json"}, Manifest: true,
	}}}
	if !reflect.DeepEqual(listing, want) {
		t.Errorf("listing = %+v, want %+v", listing, want)
	}
	empty, err := List(context.Background(), client, egoSceneID, 1)
	if err != nil || empty.Egos == nil || len(empty.Egos) != 0 {
		t.Errorf("version 1 = %+v (%v), want an empty, non-nil slot list", empty, err)
	}
}

func TestListTreatsAnyNonNullManifestAsPresent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"sceneId":"s","version":3,"egos":[
			{"key":"a","model":"a.glb","files":["a.glb"],"manifest":"{\"x\":1}","optimized":true},
			{"key":"b","model":"b.glb","files":null,"manifest":{"x":1},"optimized":false},
			{"key":"c","model":"c.glb","manifest":null},
			{"key":"d","model":"d.glb"}]}`))
	}))
	t.Cleanup(srv.Close)
	listing, err := List(context.Background(), gateway.New(srv.URL, "k", "test", nil), "s", 3)
	if err != nil {
		t.Fatal(err)
	}
	if listing.SceneID != "s" || listing.Version != 3 || len(listing.Egos) != 4 {
		t.Fatalf("listing = %+v", listing)
	}
	for i, want := range []bool{true, true, false, false} {
		if got := listing.Egos[i].Manifest; got != want {
			t.Errorf("slot %s manifest = %v, want %v", listing.Egos[i].Key, got, want)
		}
		if listing.Egos[i].Files == nil {
			t.Errorf("slot %s files = nil, want an empty list", listing.Egos[i].Key)
		}
	}
	if !listing.Egos[0].Optimized || listing.Egos[1].Optimized {
		t.Errorf("optimized flags = %+v", listing.Egos)
	}
}

func TestDeleteReportsWhetherTheSlotHeldAModelAndAlwaysSendsTheDelete(t *testing.T) {
	fake, client := egoFake(t)
	uploadHead(t, client, 1)
	for _, wantExisted := range []bool{true, false} {
		result, err := Delete(context.Background(), client, egoSceneID, "head", 1)
		if err != nil {
			t.Fatal(err)
		}
		if want := (DeleteResult{Slot: "head", Existed: wantExisted}); result != want {
			t.Errorf("result = %+v, want %+v", result, want)
		}
	}
	var deletes int
	for _, r := range fake.Requests() {
		if r.Method == http.MethodDelete {
			deletes++
			if r.Path != "/v0/scenes/"+egoSceneID+"/egos/head" || r.Query.Get("version") != "1" {
				t.Errorf("DELETE %s ?%v", r.Path, r.Query)
			}
		}
	}
	if deletes != 2 {
		t.Errorf("%d DELETEs, want 2", deletes)
	}
}

func TestPathsEscapeTheSceneAndSlot(t *testing.T) {
	if got, want := SlotPath("p1-a b", "head"), "/scenes/p1-a%20b/egos/head"; got != want {
		t.Errorf("SlotPath = %q, want %q", got, want)
	}
	if got, want := ListPath("s"), "/scenes/s/egos"; got != want {
		t.Errorf("ListPath = %q, want %q", got, want)
	}
}
