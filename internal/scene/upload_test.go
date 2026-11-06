package scene

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/ui"
)

func finalizedPlan(t *testing.T, fixture, name string) *UploadPlan {
	t.Helper()
	local, err := Plan(copyFixture(t, fixture))
	if err != nil {
		t.Fatal(err)
	}
	plan, err := Finalize(local, name, "cli-go-v0.0.0-test")
	if err != nil {
		t.Fatal(err)
	}
	return plan
}

func testLogger() (*ui.Logger, *bytes.Buffer) {
	var buf bytes.Buffer
	return ui.New(&buf, ui.Options{}), &buf
}

func readSettings(t *testing.T, dir string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(dir, "settings.json"))
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func assertNoSettings(t *testing.T, dir string) {
	t.Helper()
	if _, err := os.Stat(filepath.Join(dir, "settings.json")); !errors.Is(err, os.ErrNotExist) {
		t.Errorf("settings.json exists (stat err %v), want none", err)
	}
}

func posts(fake *fakegateway.Server) []fakegateway.Recorded {
	var out []fakegateway.Recorded
	for _, r := range fake.Requests() {
		if r.Method == http.MethodPost {
			out = append(out, r)
		}
	}
	return out
}

func TestUploadNewSceneWritesSettingsAndReportsVersionOne(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	plan := finalizedPlan(t, "scene-gltf", "New Scene")
	log, logs := testLogger()

	result, err := Upload(context.Background(), newClient(t, fake), plan, "", 0, log, nil)
	if err != nil {
		t.Fatal(err)
	}
	sent := posts(fake)
	if len(sent) != 1 || sent[0].Path != "/v0/scenes" {
		t.Fatalf("POSTs = %+v, want one to /v0/scenes", sent)
	}
	if !result.Created || result.SceneID == "" || result.VersionNumber != 1 || result.VersionID == 0 || result.ConcurrentVersions {
		t.Errorf("result = %+v", result)
	}
	if got := readSettings(t, plan.Local.Dir); !bytes.Equal(got, plan.SettingsBytes) {
		t.Errorf("settings.json = %q, want the uploaded bytes %q", got, plan.SettingsBytes)
	}
	if strings.Contains(logs.String(), "upload them again") {
		t.Errorf("a new scene has nothing to upload again: %q", logs.String())
	}
}

func TestUploadNewVersionWaitsForItAndWarnsAboutDependents(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneID, Versions: []fakegateway.Version{{ID: 1, VersionNumber: 1}}})
	fake.SetNextUpload(testSceneID, 1, fakegateway.Version{ID: 2002, VersionNumber: 2})
	plan := finalizedPlan(t, "scene-glb", "Existing")
	log, logs := testLogger()

	result, err := Upload(context.Background(), newClient(t, fake), plan, testSceneID, 1, log, nil)
	if err != nil {
		t.Fatal(err)
	}
	if sent := posts(fake); len(sent) != 1 || sent[0].Path != "/v0/scenes/"+testSceneID {
		t.Fatalf("POSTs = %+v", sent)
	}
	if result.SceneID != testSceneID || result.Created || result.VersionNumber != 2 || result.VersionID != 2002 ||
		result.ConcurrentVersions || result.CandidateVersions != nil {
		t.Errorf("result = %+v", result)
	}
	if want := "[WARN] objects, manifests and egos belong to a scene version; upload them again for version 2"; !strings.Contains(logs.String(), want) {
		t.Errorf("logs = %q, want %q", logs.String(), want)
	}
	readSettings(t, plan.Local.Dir)
}

func TestUploadConcurrentVersionsClaimsNoNumber(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneID, Versions: []fakegateway.Version{{ID: 1, VersionNumber: 1}}})
	fake.SetNextUpload(testSceneID, 0, fakegateway.Version{ID: 2, VersionNumber: 2}, fakegateway.Version{ID: 3, VersionNumber: 3})
	plan := finalizedPlan(t, "scene-glb", "Existing")
	log, logs := testLogger()

	result, err := Upload(context.Background(), newClient(t, fake), plan, testSceneID, 1, log, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !result.ConcurrentVersions || !slices.Equal(result.CandidateVersions, []int{2, 3}) || result.VersionNumber != 0 || result.VersionID != 0 {
		t.Errorf("result = %+v", result)
	}
	if want := "[WARN] objects, manifests and egos belong to a scene version; upload them again for the new version"; !strings.Contains(logs.String(), want) {
		t.Errorf("logs = %q, want %q", logs.String(), want)
	}
	readSettings(t, plan.Local.Dir)
}

func TestUploadVersionNotVisibleKeepsSettingsAndReturnsTheError(t *testing.T) {
	fastPolling(t)
	saved := versionWait
	versionWait = 20 * time.Millisecond
	t.Cleanup(func() { versionWait = saved })
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: testSceneID, Versions: []fakegateway.Version{{ID: 1, VersionNumber: 1}}})
	fake.SetNextUpload(testSceneID, 1_000_000, fakegateway.Version{ID: 2, VersionNumber: 2})
	plan := finalizedPlan(t, "scene-glb", "Existing")
	log, _ := testLogger()

	result, err := Upload(context.Background(), newClient(t, fake), plan, testSceneID, 1, log, nil)
	var notVisible *VersionNotVisibleError
	if !errors.As(err, &notVisible) || notVisible.Expected != 2 {
		t.Fatalf("err = %v, want VersionNotVisibleError for 2", err)
	}
	if result.SceneID != testSceneID {
		t.Errorf("result = %+v, want the scene id", result)
	}
	readSettings(t, plan.Local.Dir)
}

// makeReadOnly removes write permission from dir until the test ends, and
// skips the test where that does not stop file creation (root, Windows).
func makeReadOnly(t *testing.T, dir string) {
	t.Helper()
	if err := os.Chmod(dir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })
	probe, err := os.CreateTemp(dir, "probe")
	if err == nil {
		_ = probe.Close()
		_ = os.Remove(probe.Name())
		t.Skip("a read-only directory still accepts new files here")
	}
}

func TestUploadSettingsWriteFailureWarnsAndStillWaitsForTheVersion(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	plan := finalizedPlan(t, "scene-glb", "S")
	makeReadOnly(t, plan.Local.Dir)
	log, logs := testLogger()

	result, err := Upload(context.Background(), newClient(t, fake), plan, "", 0, log, nil)
	if err != nil {
		t.Fatalf("err = %v, want the upload reported as done", err)
	}
	if result.SceneID == "" || !result.Created || result.VersionNumber != 1 {
		t.Fatalf("result = %+v, want the new scene and its version", result)
	}
	if msg := logs.String(); !strings.Contains(msg, "[WARN]") || !strings.Contains(msg, result.SceneID) || !strings.Contains(msg, "settings.json could not be written") {
		t.Errorf("logs = %q, want a warning naming the scene id and settings.json", msg)
	}
	assertNoSettings(t, plan.Local.Dir)
}

func TestUploadGatewayFailureWritesNothing(t *testing.T) {
	fake := fakegateway.New(t)
	fake.FailWith(http.StatusInternalServerError, "conversion failed")
	plan := finalizedPlan(t, "scene-glb", "S")
	log, _ := testLogger()
	_, err := Upload(context.Background(), newClient(t, fake), plan, "", 0, log, nil)
	var gerr *gateway.Error
	if !errors.As(err, &gerr) || gerr.Kind != gateway.ServerError {
		t.Fatalf("err = %v, want ServerError", err)
	}
	assertNoSettings(t, plan.Local.Dir)
}

// waitUntil polls cond for up to five seconds and reports whether it held.
func waitUntil(cond func() bool) bool {
	deadline := time.Now().Add(5 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			return false
		}
		time.Sleep(5 * time.Millisecond)
	}
	return true
}

func TestUploadCancelledAfterTheBodyWasSentMayHaveCompleted(t *testing.T) {
	fake := fakegateway.New(t)
	fake.Delay(10 * time.Second)
	plan := finalizedPlan(t, "scene-glb", "S")
	log, _ := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		waitUntil(func() bool {
			sent := posts(fake)
			return len(sent) == 1 && sent[0].Complete
		})
		cancel()
	}()

	_, err := Upload(ctx, newClient(t, fake), plan, "", 0, log, nil)
	var gerr *gateway.Error
	if !errors.As(err, &gerr) || gerr.Kind != gateway.Interrupted || !gerr.BodySent {
		t.Fatalf("err = %v, want Interrupted with the body sent", err)
	}
	if !strings.Contains(gerr.UserMessage(), "may still have completed") {
		t.Errorf("message = %q", gerr.UserMessage())
	}
	assertNoSettings(t, plan.Local.Dir)
}

// stallingReader reports its first Read on started and holds it until
// released, then reads zeros like a slow file.
type stallingReader struct {
	started  chan struct{}
	released chan struct{}
	once     sync.Once
}

func (s *stallingReader) Read(p []byte) (int, error) {
	s.once.Do(func() { close(s.started) })
	<-s.released
	clear(p)
	return len(p), nil
}

func (s *stallingReader) Close() error { return nil }

func TestUploadCancelledBeforeTheBodyWasSentCouldNotReach(t *testing.T) {
	fake := fakegateway.New(t)
	plan := finalizedPlan(t, "scene-glb", "S")
	stall := &stallingReader{started: make(chan struct{}), released: make(chan struct{})}
	plan.Parts[0].Open = func() (io.ReadCloser, error) { return stall, nil }
	log, _ := testLogger()
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		<-stall.started
		cancel()
		close(stall.released)
	}()

	_, err := Upload(ctx, newClient(t, fake), plan, "", 0, log, nil)
	var gerr *gateway.Error
	if !errors.As(err, &gerr) || gerr.Kind != gateway.Interrupted || gerr.BodySent {
		t.Fatalf("err = %v, want Interrupted with the body not sent", err)
	}
	msg := gerr.UserMessage()
	if !strings.HasPrefix(msg, "Could not reach ") || strings.Contains(msg, "may still have completed") {
		t.Errorf("message = %q, want Could not reach and no claim that the upload may have completed", msg)
	}
	assertNoSettings(t, plan.Local.Dir)
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

func TestUploadHundredMegabyteSceneSendsAnExactContentLength(t *testing.T) {
	fastPolling(t)
	fake := fakegateway.New(t)
	plan := finalizedPlan(t, "scene-glb", "Big")
	const size = 100 << 20
	plan.Parts[0].Size = size
	plan.Parts[0].Open = func() (io.ReadCloser, error) {
		return io.NopCloser(io.LimitReader(zeroReader{}, size)), nil
	}
	log, _ := testLogger()

	if _, err := Upload(context.Background(), newClient(t, fake), plan, "", 0, log, nil); err != nil {
		t.Fatal(err)
	}
	sent := posts(fake)
	if len(sent) != 1 || !sent[0].Complete {
		t.Fatalf("POSTs = %d, want one complete", len(sent))
	}
	if sent[0].ContentLength <= size || sent[0].ContentLength != sent[0].BodyBytes || len(sent[0].TransferEncoding) != 0 {
		t.Errorf("Content-Length %d, body bytes %d, transfer encoding %v", sent[0].ContentLength, sent[0].BodyBytes, sent[0].TransferEncoding)
	}
	if sent[0].Parts[0].Size != size {
		t.Errorf("model part = %d bytes, want %d", sent[0].Parts[0].Size, size)
	}
}
