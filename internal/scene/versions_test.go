package scene

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
)

const testSceneID = "0b1f6c3e-2a4d-4e8b-9c7a-5d3e1f2a4b6c"

func fastPolling(t *testing.T) {
	t.Helper()
	saved := pollInterval
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = saved })
}

func newClient(t *testing.T, fake *fakegateway.Server) *gateway.Client {
	t.Helper()
	client := gateway.New(fake.URL(), fake.Key(), "test", nil)
	client.MarkPreflighted()
	return client
}

func TestDescribeMapsTheSceneObject(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{
		ID: "p12-" + testSceneID, SDKFacingID: testSceneID, SceneName: "Lobby",
		Versions: []fakegateway.Version{{ID: 3000000001, VersionNumber: 1, SDKVersion: "cli-go-v1.0.0"}},
	})
	info, err := Describe(context.Background(), newClient(t, fake), "p12-"+testSceneID)
	if err != nil {
		t.Fatal(err)
	}
	want := Info{ID: "p12-" + testSceneID, SDKFacingID: testSceneID, SceneName: "Lobby",
		Versions: []Version{{Number: 1, ID: 3000000001, SDKVersion: "cli-go-v1.0.0"}}}
	if info.ID != want.ID || info.SDKFacingID != want.SDKFacingID || info.SceneName != want.SceneName || !slices.Equal(info.Versions, want.Versions) {
		t.Errorf("info = %+v, want %+v", info, want)
	}
}

func TestDescribeUnknownSceneIsSceneNotAccessible(t *testing.T) {
	fake := fakegateway.New(t)
	_, err := Describe(context.Background(), newClient(t, fake), testSceneID)
	var gerr *gateway.Error
	if !errors.As(err, &gerr) || gerr.Kind != gateway.SceneNotAccessible {
		t.Errorf("err = %v, want SceneNotAccessible", err)
	}
}

func TestLatestVersionIsTheMaximumNumber(t *testing.T) {
	info := Info{Versions: []Version{{Number: 1, ID: 10}, {Number: 3, ID: 30}, {Number: 2, ID: 20}}}
	if v, ok := LatestVersion(info); !ok || v.Number != 3 || v.ID != 30 {
		t.Errorf("latest = %+v %v, want number 3", v, ok)
	}
	if _, ok := LatestVersion(Info{}); ok {
		t.Error("a scene without versions has no latest version")
	}
	if v, ok := VersionByNumber(info, 2); !ok || v.ID != 20 {
		t.Errorf("version 2 = %+v %v", v, ok)
	}
	if _, ok := VersionByNumber(info, 4); ok {
		t.Error("version 4 does not exist")
	}
}

func TestWaitForVersionPollsUntilTheVersionAppears(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneID, Versions: []fakegateway.Version{{ID: 1, VersionNumber: 1}}})
	fake.SetNextUpload(testSceneID, 1, fakegateway.Version{ID: 2002, VersionNumber: 2})
	client := newClient(t, fake)
	if err := client.Get(context.Background(), "/scenes/"+testSceneID, nil, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := client.PostMultipart(context.Background(), "/scenes/"+testSceneID, nil, nil, nil); err != nil {
		t.Fatal(err)
	}

	v, latest, err := WaitForVersion(context.Background(), client, testSceneID, 2, time.Second)
	if err != nil || v.Number != 2 || v.ID != 2002 || latest != 2 {
		t.Fatalf("got %+v latest %d err %v, want version 2", v, latest, err)
	}
	gets := 0
	for _, r := range fake.Requests() {
		if r.Method == "GET" {
			gets++
		}
	}
	if gets != 3 {
		t.Errorf("GET requests = %d, want the first lookup and two polls", gets)
	}
}

func TestWaitForVersionReportsConcurrentVersions(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneID, Versions: []fakegateway.Version{
		{ID: 1, VersionNumber: 1}, {ID: 3, VersionNumber: 3}, {ID: 2, VersionNumber: 2},
	}})
	_, latest, err := WaitForVersion(context.Background(), newClient(t, fake), testSceneID, 2, time.Second)
	var concurrent *ConcurrentVersionsError
	if !errors.As(err, &concurrent) || !slices.Equal(concurrent.Candidates, []int{2, 3}) || concurrent.SceneID != testSceneID {
		t.Fatalf("err = %v, want candidates [2 3]", err)
	}
	if latest != 3 {
		t.Errorf("latest = %d, want 3", latest)
	}
}

func TestWaitForVersionTimesOut(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneID, Versions: []fakegateway.Version{{ID: 1, VersionNumber: 4}}})
	start := time.Now()
	_, latest, err := WaitForVersion(context.Background(), newClient(t, fake), testSceneID, 5, 50*time.Millisecond)
	var notVisible *VersionNotVisibleError
	if !errors.As(err, &notVisible) || notVisible.Expected != 5 || notVisible.SceneID != testSceneID || notVisible.Cause != nil {
		t.Fatalf("err = %v, want VersionNotVisibleError for 5", err)
	}
	if latest != 4 || notVisible.LatestSeen != 4 {
		t.Errorf("latest = %d, error latest %d, want 4", latest, notVisible.LatestSeen)
	}
	if elapsed := time.Since(start); elapsed > time.Second {
		t.Errorf("waited %v for a 50ms timeout", elapsed)
	}
}

func TestWaitForVersionStopsOnALookupFailure(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	_, _, err := WaitForVersion(context.Background(), newClient(t, fake), testSceneID, 1, time.Second)
	var notVisible *VersionNotVisibleError
	var gerr *gateway.Error
	if !errors.As(err, &notVisible) || !errors.As(notVisible.Cause, &gerr) || gerr.Kind != gateway.SceneNotAccessible {
		t.Errorf("err = %v, want VersionNotVisibleError caused by SceneNotAccessible", err)
	}
}
