package gateway

import (
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/cognitivevr/c3d-upload-tools/internal/fakegateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
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

func newTestClient(t *testing.T, fake *fakegateway.Server, key string) (*Client, *sleepRecorder) {
	t.Helper()
	c := New(fake.URL(), key, UserAgent(), nil)
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
	c, _ := newTestClient(t, fake, fake.Key())
	var terminal bytes.Buffer
	progress := ui.NewProgress(&terminal, 23, "Uploading")
	resp, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, sceneParts(), progress)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
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
}

func TestMultipartOpenErrorIsReturned(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	errMissing := errors.New("missing file")
	parts := []Part{{Field: "file", Filename: "cube.glb", Open: func() (io.ReadCloser, error) { return nil, errMissing }}}
	_, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, parts, nil)
	if !errors.Is(err, errMissing) {
		t.Fatalf("got %v", err)
	}
	var gerr *Error
	if errors.As(err, &gerr) {
		t.Fatalf("a local file error must not be a gateway error: %v", err)
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

func TestGetDoesNotRetryOtherStatuses(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	fake.FailWith(500, "")
	if err := c.VerifyKey(context.Background()); err == nil {
		t.Fatal("want error")
	}
	if n := len(fake.Requests()); n != 1 {
		t.Fatalf("recorded %d requests", n)
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

func TestMultipartWithoutDeadlineAwaitsSlowResponse(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	fake.Delay(3 * time.Second)
	resp, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, sceneParts(), nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("resp %+v err %v", resp, err)
	}
}

func TestResponseHeaderTimeoutAppliesToGetButNotMultipart(t *testing.T) {
	fake := fakegateway.New(t)
	c, _ := newTestClient(t, fake, fake.Key())
	c.ResponseHeaderTimeout = 100 * time.Millisecond
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

	fake.Delay(400 * time.Millisecond)
	resp, err := c.PostMultipart(context.Background(), "/scenes/s1", nil, sceneParts(), nil)
	if err != nil || resp.StatusCode != 200 {
		t.Fatalf("multipart resp %+v err %v", resp, err)
	}
}

func TestUnreachableHostIsNetwork(t *testing.T) {
	c := New("http://127.0.0.1:1/v0", "fake-key-unreachable", UserAgent(), nil)
	rec := &sleepRecorder{}
	c.sleep = rec.sleep
	err := c.VerifyKey(context.Background())
	gerr := gatewayError(t, err)
	if gerr.Kind != Network || gerr.BodySent {
		t.Fatalf("kind %v bodySent %v", gerr.Kind, gerr.BodySent)
	}
	if !strings.HasPrefix(gerr.UserMessage(), "Could not reach 127.0.0.1:1: ") {
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
