package gateway

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"mime"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/ui"
)

type sleepRecorder struct {
	mu        sync.Mutex
	durations []time.Duration
}

func (r *sleepRecorder) sleep(_ context.Context, d time.Duration) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.durations = append(r.durations, d)
	return nil
}

func (r *sleepRecorder) got() []time.Duration {
	r.mu.Lock()
	defer r.mu.Unlock()
	return append([]time.Duration(nil), r.durations...)
}

func newTestClient(t *testing.T, fake *fakegateway.Server, key string, opts ...Option) (*Client, *sleepRecorder) {
	t.Helper()
	c := New(fake.URL(), key, UserAgent(), nil, opts...)
	rec := &sleepRecorder{}
	c.sleep = rec.sleep
	c.random = func() float64 { return 0.5 }
	return c, rec
}

func gatewayError(t *testing.T, err error) *Error {
	t.Helper()
	var gerr *Error
	if !errors.As(err, &gerr) {
		t.Fatalf("want *gateway.Error, got %T: %v", err, err)
	}
	return gerr
}

func memoryPart(field, filename, contentType, content string) Part {
	return Part{
		Field: field, Filename: filename, ContentType: contentType, Size: int64(len(content)),
		Open: func() (io.ReadCloser, error) { return io.NopCloser(strings.NewReader(content)), nil },
	}
}

func sceneParts() []Part {
	return []Part{
		memoryPart("file", "cube.glb", "application/octet-stream", "glb-bytes"),
		memoryPart("file", "settings.json", "application/json", `{"scale":1}`),
		memoryPart("screenshot", "screenshot.png", "application/octet-stream", "png"),
	}
}

func TestRequestHeaders(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "s1"})
	c, _ := newTestClient(t, fake, fake.Key())
	ctx := context.Background()
	if err := c.VerifyKey(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PostJSON(ctx, "/objects/s1", nil, map[string]any{"objects": []any{}}); err != nil {
		t.Fatal(err)
	}
	if _, err := c.PostMultipart(ctx, "/scenes/s1", nil, sceneParts(), nil); err != nil {
		t.Fatal(err)
	}
	if err := c.Delete(ctx, "/scenes/s1/egos/head", nil, nil); err != nil {
		t.Fatal(err)
	}
	reqs := fake.Requests()
	if len(reqs) != 4 {
		t.Fatalf("recorded %d requests", len(reqs))
	}
	userAgent := regexp.MustCompile(`^cognitive3d-cli-upload/\S+ \(\w+/\w+\)$`)
	for _, r := range reqs {
		if got := r.Header.Values("Authorization"); len(got) != 1 || got[0] != "APIKEY:DEVELOPER "+fake.Key() {
			t.Errorf("%s %s Authorization = %q", r.Method, r.Path, got)
		}
		if ua := r.Header.Get("User-Agent"); !userAgent.MatchString(ua) {
			t.Errorf("%s %s User-Agent = %q", r.Method, r.Path, ua)
		}
	}
	if reqs[0].Header.Get("Accept") != "application/json" || reqs[3].Header.Get("Accept") != "application/json" {
		t.Errorf("Accept missing on GET or DELETE")
	}
	if reqs[1].Header.Get("Content-Type") != "application/json" || string(reqs[1].Body) != `{"objects":[]}` {
		t.Errorf("PostJSON sent %q %q", reqs[1].Header.Get("Content-Type"), reqs[1].Body)
	}
	if !strings.HasPrefix(reqs[2].Header.Get("Content-Type"), "multipart/form-data; boundary=") {
		t.Errorf("multipart Content-Type = %q", reqs[2].Header.Get("Content-Type"))
	}
	if reqs[0].Path != "/v0/apiKeys/verify" || reqs[3].Method != http.MethodDelete {
		t.Errorf("unexpected requests %s %s, %s %s", reqs[0].Method, reqs[0].Path, reqs[3].Method, reqs[3].Path)
	}
}

func TestMultipartPartsAndProgress(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "s1"})
	c, _ := newTestClient(t, fake, fake.Key())
	var terminal bytes.Buffer
	progress := ui.NewProgress(&terminal, 23, "Uploading")
	resp, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, sceneParts(), progress)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 201 {
		t.Fatalf("status %d", resp.StatusCode)
	}
	parts := fake.Requests()[0].Parts
	want := []fakegateway.RecordedPart{
		{Field: "file", Filename: "cube.glb", Size: 9, Body: []byte("glb-bytes")},
		{Field: "file", Filename: "settings.json", Size: 11, Body: []byte(`{"scale":1}`)},
		{Field: "screenshot", Filename: "screenshot.png", Size: 3, Body: []byte("png")},
	}
	if len(parts) != len(want) {
		t.Fatalf("parts %+v", parts)
	}
	for i := range want {
		if parts[i].Field != want[i].Field || parts[i].Filename != want[i].Filename || parts[i].Size != want[i].Size || !bytes.Equal(parts[i].Body, want[i].Body) {
			t.Errorf("part %d = %+v, want %+v", i, parts[i], want[i])
		}
	}
	if !strings.Contains(terminal.String(), "Uploading") {
		t.Errorf("progress not drawn: %q", terminal.String())
	}
	wire := recordedLength(t, fake.Requests()[0], sceneParts())
	progress.Done()
	if want := fmt.Sprintf("\rUploading: 100%% %d B of %d B\n", wire, wire); !strings.HasSuffix(terminal.String(), want) {
		t.Errorf("final line %q, want suffix %q", terminal.String(), want)
	}
}

func TestMultipartOpenErrorIsReturned(t *testing.T) {
	t.Parallel()
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	errMissing := errors.New("missing file")
	parts := []Part{{Field: "file", Filename: "cube.glb", Open: func() (io.ReadCloser, error) { return nil, errMissing }}}
	_, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, parts, nil)
	if !errors.Is(err, errMissing) {
		t.Fatalf("got %v", err)
	}
	gerr := gatewayError(t, err)
	if gerr.Kind != LocalFile || ExitCode(err) != 4 || gerr.BodySent || gerr.UserMessage() != "open cube.glb: missing file" {
		t.Fatalf("kind %v exit %d bodySent %v message %q", gerr.Kind, ExitCode(err), gerr.BodySent, gerr.UserMessage())
	}
}

func TestUserDecodesExpiry(t *testing.T) {
	fake := fakegateway.New(t)
	fake.SetUser(fakegateway.User{ProjectName: "P", OrganizationName: "O", KeyExpiresAt: 1767225600000})
	c, _ := newTestClient(t, fake, fake.Key())
	u, err := c.User(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if u.ProjectName != "P" || u.OrganizationName != "O" || u.KeyExpiresAtMilliseconds != 1767225600000 {
		t.Fatalf("user %+v", u)
	}
}

func TestExpiredKey(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.ExpiredKey())
	err := c.VerifyKey(context.Background())
	gerr := gatewayError(t, err)
	if gerr.Kind != KeyExpired || gerr.StatusCode != 401 || ExitCode(err) != 2 {
		t.Fatalf("kind %v status %d exit %d", gerr.Kind, gerr.StatusCode, ExitCode(err))
	}
	msg := gerr.UserMessage()
	for _, want := range []string{"Your developer API key has expired.", "  3. Generate a new developer API key", `export C3D_DEVELOPER_API_KEY="your_new_key"`} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestEmptyUnauthorizedBeforePreflight(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	err := c.Get(context.Background(), "/scenes/x", nil, nil)
	gerr := gatewayError(t, err)
	if gerr.Kind != Unauthorized || ExitCode(err) != 2 {
		t.Fatalf("kind %v exit %d", gerr.Kind, ExitCode(err))
	}
	msg := gerr.UserMessage()
	if strings.Contains(strings.ToLower(msg), "expired") {
		t.Errorf("message mentions expiry:\n%s", msg)
	}
	if !strings.HasPrefix(msg, "Authentication failed. Please check your developer API key.") || !strings.Contains(msg, "--env prod or --env dev") {
		t.Errorf("message:\n%s", msg)
	}
}

func TestEmptyUnauthorizedAfterPreflightIsSceneNotAccessible(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	c.MarkPreflighted()
	err := c.Get(context.Background(), "/scenes/x", nil, nil)
	gerr := gatewayError(t, err)
	if gerr.Kind != SceneNotAccessible || ExitCode(err) != 3 {
		t.Fatalf("kind %v exit %d", gerr.Kind, ExitCode(err))
	}
	want := "Scene x was not found in the project this key belongs to. Check the id with: cognitive3d-cli-upload scene list"
	if gerr.UserMessage() != want {
		t.Fatalf("message %q", gerr.UserMessage())
	}
}

func TestUnauthorizedWithBodyAfterPreflight(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	c.MarkPreflighted()
	fake.FailWith(401, "No scene access.")
	err := c.Get(context.Background(), "/scenes/x", nil, nil)
	gerr := gatewayError(t, err)
	if gerr.Kind != Unauthorized || ExitCode(err) != 2 {
		t.Fatalf("kind %v exit %d", gerr.Kind, ExitCode(err))
	}
	if !strings.HasPrefix(gerr.UserMessage(), "Authentication failed: No scene access.") {
		t.Fatalf("message:\n%s", gerr.UserMessage())
	}
}

func TestStatusMapping(t *testing.T) {
	const html = "<html><body><h1>502 Bad Gateway</h1></body></html>"
	tests := []struct {
		name      string
		status    int
		body      string
		multipart bool
		path      string
		kind      ErrorKind
		exit      int
		contains  string
		absent    string
	}{
		{"forbidden", 403, "", false, "/objects/s1", Forbidden, 2, "Access forbidden.", ""},
		{"not found", 404, "", false, "/objects/s1", NotFound, 3, "Resource not found.", ""},
		{"conflict", 409, "cube.glb exists", true, "/objects/s1/cube", Conflict, 1, "The gateway reported a file conflict: cube.glb exists", ""},
		{"too large html", 413, html, true, "/scenes/s1", TooLarge, 4, "(3 GiB per request)", "<"},
		{"manifest empty 400", 400, "", false, "/objects/s1", ManifestRejected, 1, "The gateway rejected the manifest (status 400). The usual causes are duplicate ids or malformed vectors; the gateway does not say which.", ""},
		{"manifest invalid json", 400, "Invalid json.", false, "/objects/s1", ClientError, 1, "failed with status 400: Invalid json.", ""},
		{"object upload 400", 400, "Bad file name.", true, "/objects/s1/cube", ClientError, 1, "failed with status 400: Bad file name.", ""},
		{"server error text", 500, "Error looking up scene version.", false, "/objects/s1", ServerError, 1, "The Cognitive3D gateway returned a server error (status 500): Error looking up scene version.", ""},
		{"server error html", 500, html, false, "/objects/s1", ServerError, 1, "The Cognitive3D gateway returned a server error (status 500). Please try again later.", "<"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := fakegateway.New(t)
			c, _ := newTestClient(t, fake, fake.Key())
			fake.FailWith(tt.status, tt.body)
			var err error
			if tt.multipart {
				_, err = c.PostMultipart(context.Background(), tt.path, nil, sceneParts(), nil)
			} else {
				_, err = c.PostJSON(context.Background(), tt.path, nil, []any{})
			}
			gerr := gatewayError(t, err)
			if gerr.Kind != tt.kind || gerr.StatusCode != tt.status || ExitCode(err) != tt.exit {
				t.Fatalf("kind %v status %d exit %d", gerr.Kind, gerr.StatusCode, ExitCode(err))
			}
			msg := gerr.UserMessage()
			if !strings.Contains(msg, tt.contains) {
				t.Errorf("message %q does not contain %q", msg, tt.contains)
			}
			if tt.absent != "" && strings.Contains(msg, tt.absent) {
				t.Errorf("message %q contains %q", msg, tt.absent)
			}
		})
	}
}

func TestGetRetriesServiceUnavailableHonoringRetryAfter(t *testing.T) {
	fake := fakegateway.New(t)
	c, sleeps := newTestClient(t, fake, fake.Key())
	retryNow := http.Header{"Retry-After": {"0"}}
	fake.FailWithHeaders(503, "", retryNow)
	fake.FailWithHeaders(503, "", retryNow)
	if _, err := c.User(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.Requests()); n != 3 {
		t.Fatalf("recorded %d requests, want 3", n)
	}
	if got := sleeps.got(); len(got) != 2 || got[0] != 0 || got[1] != 0 {
		t.Fatalf("sleeps %v, want [0 0]", got)
	}
}

func TestGetBackoffDoublesWithJitterThenGivesUp(t *testing.T) {
	fake := fakegateway.New(t)
	c, sleeps := newTestClient(t, fake, fake.Key())
	fake.FailWith(502, "")
	fake.FailWith(504, "")
	fake.FailWith(503, "")
	err := c.Get(context.Background(), "/scenes", nil, nil)
	gerr := gatewayError(t, err)
	if gerr.Kind != ServerError || gerr.StatusCode != 503 {
		t.Fatalf("kind %v status %d", gerr.Kind, gerr.StatusCode)
	}
	if n := len(fake.Requests()); n != 3 {
		t.Fatalf("recorded %d requests, want 3", n)
	}
	want := []time.Duration{1250 * time.Millisecond, 2500 * time.Millisecond}
	if got := sleeps.got(); len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("sleeps %v, want %v", got, want)
	}
}

func TestJitterFractionStaysInUnitInterval(t *testing.T) {
	for i := 0; i < 10000; i++ {
		if f := jitterFraction(); f < 0 || f >= 1 {
			t.Fatalf("jitterFraction() = %v; want [0, 1)", f)
		}
	}
}

func TestRetryAfterIsCappedAtSixtySeconds(t *testing.T) {
	fake := fakegateway.New(t)
	c, sleeps := newTestClient(t, fake, fake.Key())
	fake.FailWithHeaders(503, "", http.Header{"Retry-After": {"3600"}})
	if err := c.VerifyKey(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sleeps.got(); len(got) != 1 || got[0] != 60*time.Second {
		t.Fatalf("sleeps %v", got)
	}
}

func TestOversizedRetryAfterIsCappedNotOverflowed(t *testing.T) {
	fake := fakegateway.New(t)
	c, sleeps := newTestClient(t, fake, fake.Key())
	fake.FailWithHeaders(503, "", http.Header{"Retry-After": {"9300000000"}})
	if err := c.VerifyKey(context.Background()); err != nil {
		t.Fatal(err)
	}
	if got := sleeps.got(); len(got) != 1 || got[0] != 60*time.Second {
		t.Fatalf("sleeps %v", got)
	}
}

func TestGetRetriesTooManyRequestsHonoringRetryAfter(t *testing.T) {
	fake := fakegateway.New(t)
	c, sleeps := newTestClient(t, fake, fake.Key())
	fake.FailWithHeaders(429, "", http.Header{"Retry-After": {"0"}})
	if err := c.VerifyKey(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.Requests()); n != 2 {
		t.Fatalf("recorded %d requests, want 2", n)
	}
	if got := sleeps.got(); len(got) != 1 || got[0] != 0 {
		t.Fatalf("sleeps %v, want [0]", got)
	}
}

func TestGetRetriesInternalServerError(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	fake.FailWith(500, "")
	if err := c.VerifyKey(context.Background()); err != nil {
		t.Fatal(err)
	}
	if n := len(fake.Requests()); n != 2 {
		t.Fatalf("recorded %d requests, want 2", n)
	}
}

func TestGetDoesNotRetryOtherStatuses(t *testing.T) {
	for _, status := range []int{400, 404, 501, 505} {
		t.Run(strconv.Itoa(status), func(t *testing.T) {
			fake := fakegateway.New(t)
			c, _ := newTestClient(t, fake, fake.Key())
			fake.FailWith(status, "")
			if err := c.VerifyKey(context.Background()); err == nil {
				t.Fatal("want error")
			}
			if n := len(fake.Requests()); n != 1 {
				t.Fatalf("recorded %d requests, want 1", n)
			}
		})
	}
}

func TestPostIsNotRetried(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	fake.FailWith(503, "")
	_, err := c.PostJSON(context.Background(), "/objects/s1", nil, []any{})
	if gatewayError(t, err).StatusCode != 503 {
		t.Fatalf("got %v", err)
	}
	if n := len(fake.Requests()); n != 1 {
		t.Fatalf("recorded %d requests, want 1", n)
	}
}

func TestUploadCutOffByDeadlineAfterBodySent(t *testing.T) {
	t.Parallel()
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	fake.Delay(3 * time.Second)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	_, err := c.PostMultipart(ctx, "/scenes/s1", nil, sceneParts(), nil)
	gerr := gatewayError(t, err)
	if gerr.Kind != Interrupted || !gerr.BodySent || ExitCode(err) != 1 {
		t.Fatalf("kind %v bodySent %v exit %d", gerr.Kind, gerr.BodySent, ExitCode(err))
	}
	msg := gerr.UserMessage()
	for _, want := range []string{"The upload may still have completed.", "Check with: cognitive3d-cli-upload scene list before retrying"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message missing %q:\n%s", want, msg)
		}
	}
}

func TestInterruptedBeforeSending(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := c.PostMultipart(ctx, "/scenes/s1", nil, sceneParts(), nil)
	gerr := gatewayError(t, err)
	if gerr.Kind != Interrupted || gerr.BodySent {
		t.Fatalf("kind %v bodySent %v", gerr.Kind, gerr.BodySent)
	}
	if !strings.HasPrefix(gerr.UserMessage(), "Could not reach 127.0.0.1:") {
		t.Fatalf("message %q", gerr.UserMessage())
	}
}

// blockingPart's reader blocks until ctx is done, then streams one byte per
// read, so the body is still being sent when the request is cancelled.
func blockingPart(ctx context.Context, started chan<- struct{}) Part {
	var once sync.Once
	return Part{Field: "file", Filename: "big.glb", Size: 1 << 20, Open: func() (io.ReadCloser, error) {
		return io.NopCloser(readerFunc(func(p []byte) (int, error) {
			once.Do(func() { close(started) })
			<-ctx.Done()
			return copy(p, "x"), nil
		})), nil
	}}
}

type readerFunc func(p []byte) (int, error)

func (f readerFunc) Read(p []byte) (int, error) { return f(p) }

func TestCancelMidBodyIsNotBodySent(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	started := make(chan struct{})
	go func() {
		<-started
		cancel()
	}()
	_, err := c.PostMultipart(ctx, "/scenes/s1", nil, []Part{blockingPart(ctx, started)}, nil)
	gerr := gatewayError(t, err)
	if gerr.Kind != Interrupted || gerr.BodySent {
		t.Fatalf("kind %v bodySent %v", gerr.Kind, gerr.BodySent)
	}
	msg := gerr.UserMessage()
	if !strings.Contains(msg, "Could not reach") || strings.Contains(msg, "may still have completed") {
		t.Fatalf("message %q", msg)
	}
}

func TestPartReadErrorIsReturned(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	errDisk := errors.New("disk read failed")
	part := Part{Field: "file", Filename: "cube.glb", Size: 100, Open: func() (io.ReadCloser, error) {
		sent := false
		return io.NopCloser(readerFunc(func(p []byte) (int, error) {
			if !sent {
				sent = true
				return copy(p, "partial"), nil
			}
			return 0, errDisk
		})), nil
	}}
	_, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, []Part{part}, nil)
	if !errors.Is(err, errDisk) {
		t.Fatalf("got %v", err)
	}
	gerr := gatewayError(t, err)
	if gerr.Kind != LocalFile || ExitCode(err) != 4 || gerr.UserMessage() != "read cube.glb: disk read failed" {
		t.Fatalf("kind %v exit %d message %q", gerr.Kind, ExitCode(err), gerr.UserMessage())
	}
	for _, r := range fake.Requests() {
		if r.Complete {
			t.Fatalf("fake recorded a complete request: %+v", r)
		}
	}
}

func TestMultipartWithoutDeadlineAwaitsSlowResponse(t *testing.T) {
	t.Parallel()
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "s1"})
	c, _ := newTestClient(t, fake, fake.Key())
	fake.Delay(3 * time.Second)
	resp, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, sceneParts(), nil)
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("resp %+v err %v", resp, err)
	}
}

func TestResponseHeaderTimeoutAppliesToGetButNotMultipart(t *testing.T) {
	t.Parallel()
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key(), WithResponseHeaderTimeout(100*time.Millisecond))
	for range 3 {
		fake.Delay(400 * time.Millisecond)
	}
	err := c.Get(context.Background(), "/scenes/s1", nil, nil)
	gerr := gatewayError(t, err)
	if gerr.Kind != Network || ExitCode(err) != 1 {
		t.Fatalf("kind %v exit %d: %v", gerr.Kind, ExitCode(err), err)
	}
	if n := len(fake.Requests()); n != 3 {
		t.Fatalf("GET attempts %d, want 3", n)
	}

	fake.AddScene(fakegateway.Scene{ID: "s1"})
	fake.Delay(400 * time.Millisecond)
	resp, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, sceneParts(), nil)
	if err != nil || resp.StatusCode != 201 {
		t.Fatalf("multipart resp %+v err %v", resp, err)
	}
}

// hangUpListener accepts every connection and closes it at once, which fails
// a request quickly with the same error on every OS.
func hangUpListener(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()
	return ln.Addr().String()
}

func TestUnreachableHostIsNetwork(t *testing.T) {
	addr := hangUpListener(t)
	c := New("http://"+addr+"/v0", "fake-key-unreachable", UserAgent(), nil)
	rec := &sleepRecorder{}
	c.sleep = rec.sleep
	err := c.VerifyKey(context.Background())
	gerr := gatewayError(t, err)
	if gerr.Kind != Network || gerr.BodySent {
		t.Fatalf("kind %v bodySent %v", gerr.Kind, gerr.BodySent)
	}
	if !strings.HasPrefix(gerr.UserMessage(), "Could not reach "+addr+": ") {
		t.Fatalf("message %q", gerr.UserMessage())
	}
	if len(rec.got()) != 2 {
		t.Fatalf("transport errors not retried: sleeps %v", rec.got())
	}
}

func TestKeyNeverInErrors(t *testing.T) {
	fake := fakegateway.New(t)
	ctx := context.Background()
	check := func(name string, err error, key string) {
		t.Helper()
		if err == nil {
			t.Fatalf("%s: want error", name)
		}
		if strings.Contains(err.Error(), key) {
			t.Errorf("%s: key in Error(): %q", name, err.Error())
		}
		if strings.Contains(gatewayError(t, err).UserMessage(), key) {
			t.Errorf("%s: key in UserMessage()", name)
		}
	}

	expired, _ := newTestClient(t, fake, fake.ExpiredKey())
	check("expired", expired.VerifyKey(ctx), fake.ExpiredKey())

	c, _ := newTestClient(t, fake, fake.Key())
	fake.FailWith(500, "server log line")
	_, err := c.PostJSON(ctx, "/objects/s1", nil, []any{})
	check("server error", err, fake.Key())
	if strings.Contains(err.Error(), "server log line") {
		t.Errorf("Error() carries the body: %q", err.Error())
	}

	ctxDone, cancel := context.WithCancel(ctx)
	cancel()
	_, err = c.PostMultipart(ctxDone, "/scenes/s1", nil, sceneParts(), nil)
	check("interrupted", err, fake.Key())
}

// waitForRequests returns the fake's requests once at least n have been
// recorded; the server may still be reading a body the client gave up on.
func waitForRequests(t *testing.T, fake *fakegateway.Server, n int) []fakegateway.Recorded {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		reqs := fake.Requests()
		if len(reqs) >= n {
			return reqs
		}
		if time.Now().After(deadline) {
			t.Fatalf("fake recorded %d requests, want %d", len(reqs), n)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// recordedLength is the length NewMultipartBody computes for parts with the
// boundary the recorded request declared.
func recordedLength(t *testing.T, r fakegateway.Recorded, parts []Part) int64 {
	t.Helper()
	_, params, err := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if err != nil {
		t.Fatal(err)
	}
	boundary := params["boundary"]
	if !regexp.MustCompile(`^[0-9a-f]{60}$`).MatchString(boundary) {
		t.Fatalf("boundary %q", boundary)
	}
	body, _, length, err := NewMultipartBody(parts, boundary)
	if err != nil {
		t.Fatal(err)
	}
	_ = body.Close()
	return length
}

func TestMultipartSendsContentLengthNotChunked(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "s1"})
	c, _ := newTestClient(t, fake, fake.Key())
	if _, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, sceneParts(), ui.NewProgress(nil, 0, "")); err != nil {
		t.Fatal(err)
	}
	r := fake.Requests()[0]
	want := recordedLength(t, r, sceneParts())
	if r.Header.Get("Content-Length") != strconv.FormatInt(want, 10) || r.ContentLength != want {
		t.Errorf("Content-Length header %q, request length %d, want %d", r.Header.Get("Content-Length"), r.ContentLength, want)
	}
	if len(r.TransferEncoding) != 0 || r.Header.Get("Transfer-Encoding") != "" {
		t.Errorf("Transfer-Encoding %v", r.TransferEncoding)
	}
	if r.BodyBytes != want || !r.Complete {
		t.Errorf("fake read %d bytes complete %v, want %d", r.BodyBytes, r.Complete, want)
	}
}

func TestMultipartFileChangedSizeIsLocalFileError(t *testing.T) {
	const planned = 256 << 10
	tests := []struct {
		name   string
		change func(path string) error
		want   string
	}{
		{"truncated", func(path string) error { return os.Truncate(path, planned/2) }, "read big.glb: file size changed from 262144 to 131072 bytes"},
		{"extended", func(path string) error { return os.WriteFile(path, make([]byte, planned*3/2), 0o600) }, "read big.glb: file size changed from 262144 to more than 262144 bytes"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fake := fakegateway.New(t)
			c, _ := newTestClient(t, fake, fake.Key())
			path := filepath.Join(t.TempDir(), "big.glb")
			if err := os.WriteFile(path, make([]byte, planned), 0o600); err != nil {
				t.Fatal(err)
			}
			info, err := os.Stat(path)
			if err != nil {
				t.Fatal(err)
			}
			part := Part{Field: "file", Filename: "big.glb", Size: info.Size(), Open: func() (io.ReadCloser, error) { return os.Open(path) }}
			if err := tt.change(path); err != nil {
				t.Fatal(err)
			}
			resp, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, []Part{part}, nil)
			if resp != nil {
				t.Fatalf("success reported: %+v", resp)
			}
			gerr := gatewayError(t, err)
			if gerr.Kind != LocalFile || ExitCode(err) != 4 || gerr.BodySent || gerr.UserMessage() != tt.want {
				t.Fatalf("kind %v exit %d bodySent %v message %q", gerr.Kind, ExitCode(err), gerr.BodySent, gerr.UserMessage())
			}
			reqs := waitForRequests(t, fake, 1)
			if reqs[0].Complete {
				t.Fatalf("fake recorded a complete request: %+v", reqs[0].Parts)
			}
		})
	}
}

type zeroReader struct{}

func (zeroReader) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

const syntheticUploadBytes = 100_000_000

func TestMultipartHundredMegabyteUpload(t *testing.T) {
	fake := fakegateway.New(t)
	fake.AddScene(fakegateway.Scene{ID: "s1"})
	c, _ := newTestClient(t, fake, fake.Key())
	parts := []Part{{Field: "file", Filename: "big.glb", Size: syntheticUploadBytes, Open: func() (io.ReadCloser, error) {
		return io.NopCloser(io.LimitReader(zeroReader{}, syntheticUploadBytes)), nil
	}}}
	if _, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, parts, nil); err != nil {
		t.Fatal(err)
	}
	r := fake.Requests()[0]
	want := recordedLength(t, r, parts)
	if r.Header.Get("Content-Length") != strconv.FormatInt(want, 10) {
		t.Errorf("Content-Length %q, want %d", r.Header.Get("Content-Length"), want)
	}
	if r.BodyBytes != want || !r.Complete {
		t.Errorf("fake read %d bytes complete %v, want %d", r.BodyBytes, r.Complete, want)
	}
	if len(r.Parts) != 1 || r.Parts[0].Size != syntheticUploadBytes {
		t.Errorf("parts %+v", r.Parts)
	}
}

func TestMultipartCancelHalfwayReturnsWithinOneSecond(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	var produced int64
	var cancelledAt atomic.Int64
	part := Part{Field: "file", Filename: "big.glb", Size: syntheticUploadBytes, Open: func() (io.ReadCloser, error) {
		source := io.LimitReader(zeroReader{}, syntheticUploadBytes)
		return io.NopCloser(readerFunc(func(p []byte) (int, error) {
			n, err := source.Read(p)
			produced += int64(n)
			if produced >= syntheticUploadBytes/2 && cancelledAt.Load() == 0 {
				cancelledAt.Store(time.Now().UnixNano())
				cancel()
			}
			return n, err
		})), nil
	}}
	_, err := c.PostMultipart(ctx, "/scenes/s1", nil, []Part{part}, nil)
	returned := time.Now()
	if cancelledAt.Load() == 0 {
		t.Fatalf("the upload never reached half way: %v", err)
	}
	if elapsed := returned.Sub(time.Unix(0, cancelledAt.Load())); elapsed > time.Second {
		t.Errorf("returned %v after the cancel", elapsed)
	}
	gerr := gatewayError(t, err)
	if gerr.Kind != Interrupted || gerr.BodySent {
		t.Fatalf("kind %v bodySent %v", gerr.Kind, gerr.BodySent)
	}
}
