package gateway

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
)

func TestExitCode(t *testing.T) {
	tests := []struct {
		err  error
		want int
	}{
		{nil, 0},
		{errors.New("local"), 1},
		{&Error{Kind: KeyExpired}, 2},
		{&Error{Kind: Unauthorized}, 2},
		{&Error{Kind: Forbidden}, 2},
		{&Error{Kind: SceneNotAccessible}, 3},
		{&Error{Kind: NotFound}, 3},
		{&Error{Kind: TooLarge}, 4},
		{&Error{Kind: Conflict}, 1},
		{&Error{Kind: ManifestRejected}, 1},
		{&Error{Kind: ClientError}, 1},
		{&Error{Kind: ServerError}, 1},
		{&Error{Kind: Network}, 1},
		{&Error{Kind: Interrupted}, 1},
		{&Error{Kind: LocalFile}, 4},
		{fmt.Errorf("scene show: %w", &Error{Kind: NotFound}), 3},
	}
	for _, tt := range tests {
		if got := ExitCode(tt.err); got != tt.want {
			t.Errorf("ExitCode(%v) = %d, want %d", tt.err, got, tt.want)
		}
	}
}

func TestKindNames(t *testing.T) {
	want := map[ErrorKind]string{
		KeyExpired: "key_expired", Unauthorized: "unauthorized", SceneNotAccessible: "scene_not_accessible",
		Forbidden: "forbidden", NotFound: "not_found", Conflict: "conflict", TooLarge: "too_large",
		ManifestRejected: "manifest_rejected", ClientError: "client_error", ServerError: "server_error",
		Network: "network", Interrupted: "interrupted", LocalFile: "local_file",
	}
	for kind, name := range want {
		if kind.String() != name {
			t.Errorf("%d.String() = %q, want %q", kind, kind.String(), name)
		}
	}
}

func TestKeyExpiredMatchIsCaseInsensitiveAndOnly401(t *testing.T) {
	if e := newStatusError("GET", "/apiKeys/verify", 401, []byte("KEY EXPIRED"), "", false); e.Kind != KeyExpired {
		t.Errorf("401 KEY EXPIRED: %v", e.Kind)
	}
	if e := newStatusError("GET", "/scenes/s1", 401, []byte("Key expired."), "", true); e.Kind != KeyExpired {
		t.Errorf("401 Key expired. after preflight: %v", e.Kind)
	}
	if e := newStatusError("GET", "/scenes", 403, []byte("key expired"), "", false); e.Kind != Forbidden {
		t.Errorf("403 key expired: %v", e.Kind)
	}
}

func TestSceneNotAccessibleOnlyOnSceneScopedRoutes(t *testing.T) {
	tests := []struct {
		path string
		kind ErrorKind
		msg  string
	}{
		{"/objects/s9/cube", SceneNotAccessible, "Scene s9 was not found in the project this key belongs to."},
		{"/scenes/s9/egos/head", SceneNotAccessible, "Scene s9 was not found"},
		{"/versions/12/objects", SceneNotAccessible, "The scene was not found in the project this key belongs to."},
		{"/user", Unauthorized, "Authentication failed."},
		{"/scenes", Unauthorized, "Authentication failed."},
	}
	for _, tt := range tests {
		e := newStatusError("GET", tt.path, 401, nil, "", true)
		if e.Kind != tt.kind || !strings.HasPrefix(e.UserMessage(), tt.msg) {
			t.Errorf("%s: kind %v message %q", tt.path, e.Kind, e.UserMessage())
		}
	}
}

func TestHTMLDetectedByContentType(t *testing.T) {
	e := newStatusError("GET", "/scenes", 401, []byte("Denied by proxy"), "text/html; charset=utf-8", false)
	if !strings.HasPrefix(e.UserMessage(), "Authentication failed. Please check your developer API key.") {
		t.Errorf("401 html: %q", e.UserMessage())
	}
	e = newStatusError("POST", "/objects/s1/cube", 400, []byte("Denied by proxy"), "text/html", false)
	if e.UserMessage() != "POST /objects/s1/cube failed with status 400: (no details from the gateway)" {
		t.Errorf("400 html: %q", e.UserMessage())
	}
	e = newStatusError("GET", "/scenes", 502, []byte("Bad gateway"), "text/html", false)
	if strings.Contains(e.UserMessage(), "Bad gateway") {
		t.Errorf("502 html: %q", e.UserMessage())
	}
}

func TestMessageAndHintSplitTheUserMessage(t *testing.T) {
	tests := []struct {
		name          string
		err           *Error
		message, hint string
		joiner        string
	}{
		{"expired", newStatusError("GET", "/apiKeys/verify", 401, []byte("Key expired"), "", false),
			"Your developer API key has expired.", keyRotationSteps, "\n\n"},
		{"unauthorized", newStatusError("GET", "/apiKeys/verify", 401, nil, "", false),
			"Authentication failed. Please check your developer API key.", keyVerificationSteps, "\n\n"},
		{"scene not accessible", newStatusError("GET", "/scenes/s1", 401, nil, "", true),
			"Scene s1 was not found in the project this key belongs to.", "Check the id with: cognitive3d-cli-upload scene list", " "},
		{"lost after sending", newTransportError("POST", "/scenes/s1", "data.c3ddev.com", context.DeadlineExceeded, true, true),
			"The connection was lost while waiting for the gateway's answer. The upload may still have completed.",
			"Check with: cognitive3d-cli-upload scene list before retrying, because a retry would create another version.", " "},
		{"forbidden", newStatusError("GET", "/scenes/s1", 403, nil, "", true),
			"Access forbidden. Your API key may not have permission for this operation.", "", ""},
		{"server error after sending", sent(newStatusError("POST", "/scenes/s1", 502, nil, "", true)),
			"The Cognitive3D gateway returned a server error (status 502).",
			"Check with: cognitive3d-cli-upload scene list before retrying, because a retry would create another version.", " "},
		{"server error with body after sending", sent(newStatusError("POST", "/scenes/s1/egos/head", 500, []byte("upstream timeout"), "text/plain", true)),
			"The Cognitive3D gateway returned a server error (status 500): upstream timeout",
			"Check with: cognitive3d-cli-upload ego list --scene-id s1 before retrying, because a retry replaces the same content.", " "},
		{"client error after sending", sent(newStatusError("POST", "/objects/s1/cube", 422, []byte("bad mesh"), "text/plain", true)),
			"POST /objects/s1/cube failed with status 422: bad mesh", "", ""},
		{"client error after sending a scene", sent(newStatusError("POST", "/scenes/s1", 422, nil, "", true)),
			"POST /scenes/s1 failed with status 422: (no details from the gateway)", "", ""},
		{"server error before sending", newStatusError("POST", "/scenes/s1", 502, nil, "", true),
			"The Cognitive3D gateway returned a server error (status 502). Please try again later.", "", ""},
		{"server error after sending a manifest", sent(newStatusError("POST", "/objects/s1", 503, nil, "", true)),
			"The Cognitive3D gateway returned a server error (status 503). Please try again later.", "", ""},
		{"server error on a GET", sent(newStatusError("GET", "/scenes/s1", 503, nil, "", true)),
			"The Cognitive3D gateway returned a server error (status 503). Please try again later.", "", ""},
	}
	for _, tt := range tests {
		message, hint := tt.err.MessageAndHint()
		if message != tt.message || hint != tt.hint {
			t.Errorf("%s: message %q hint %q", tt.name, message, hint)
		}
		if want := tt.message + tt.joiner + tt.hint; tt.err.UserMessage() != want {
			t.Errorf("%s: UserMessage %q, want %q", tt.name, tt.err.UserMessage(), want)
		}
	}
}

func TestClientErrorBodyCollapsedAndTruncated(t *testing.T) {
	body := "first\n\n   second\t" + strings.Repeat("x", 400)
	e := newStatusError("POST", "/objects/s1/cube", 422, []byte(body), "text/plain", false)
	prefix := "POST /objects/s1/cube failed with status 422: "
	msg := e.UserMessage()
	if !strings.HasPrefix(msg, prefix+"first second x") {
		t.Fatalf("message %q", msg)
	}
	if shown := strings.TrimPrefix(msg, prefix); len([]rune(shown)) != 300 {
		t.Fatalf("shown body is %d characters", len([]rune(shown)))
	}
	if e := newStatusError("POST", "/objects/s1/cube", 400, []byte("  \n"), "", false); e.UserMessage() != "POST /objects/s1/cube failed with status 400: (no details from the gateway)" {
		t.Fatalf("empty body: %q", e.UserMessage())
	}
}

func TestServerErrorBodyOfThreeHundredCharactersIsNotShown(t *testing.T) {
	e := newStatusError("GET", "/scenes", 503, []byte(strings.Repeat("y", 300)), "text/plain", false)
	if e.UserMessage() != "The Cognitive3D gateway returned a server error (status 503). Please try again later." {
		t.Fatalf("message %q", e.UserMessage())
	}
}

func TestManifestRouteIsExact(t *testing.T) {
	if e := newStatusError("POST", "/objects/s1", 400, nil, "", false); e.Kind != ManifestRejected {
		t.Errorf("/objects/s1: %v", e.Kind)
	}
	if e := newStatusError("POST", "/objects/s1/cube", 400, nil, "", false); e.Kind != ClientError {
		t.Errorf("/objects/s1/cube: %v", e.Kind)
	}
}

func TestLostConnectionMessageNamesTheCheckCommand(t *testing.T) {
	const newVersion = "a retry would create another version."
	const sameContent = "a retry replaces the same content."
	tests := []struct {
		method, path, check, consequence string
	}{
		{"POST", "/scenes", "cognitive3d-cli-upload scene list", newVersion},
		{"POST", "/scenes/s1", "cognitive3d-cli-upload scene list", newVersion},
		{"POST", "/scenes/s1/screenshot", "cognitive3d-cli-upload scene list", sameContent},
		{"POST", "/objects/s1/cube", "cognitive3d-cli-upload object list --scene-id s1", sameContent},
		{"POST", "/scenes/s1/egos/head", "cognitive3d-cli-upload ego list --scene-id s1", sameContent},
	}
	for _, tt := range tests {
		e := newTransportError(tt.method, tt.path, "data.c3ddev.com", context.DeadlineExceeded, true, true)
		want := "The connection was lost while waiting for the gateway's answer. The upload may still have completed. Check with: " +
			tt.check + " before retrying, because " + tt.consequence
		if e.UserMessage() != want {
			t.Errorf("%s %s: %q", tt.method, tt.path, e.UserMessage())
		}
	}
	for _, route := range [][2]string{{"POST", "/objects/s1"}, {"GET", "/scenes/s1"}, {"DELETE", "/scenes/s1/egos/head"}} {
		e := newTransportError(route[0], route[1], "data.c3ddev.com", errors.New("connection reset by peer"), true, false)
		if e.UserMessage() != "Could not reach data.c3ddev.com: connection reset by peer" {
			t.Errorf("%s %s: %q", route[0], route[1], e.UserMessage())
		}
	}
}

func TestErrorStringIsOneTechnicalLine(t *testing.T) {
	e := newStatusError("POST", "/scenes/s1", 500, []byte("body text\nsecond line"), "", false)
	got := e.Error()
	for _, want := range []string{"POST /scenes/s1", "500", "server_error"} {
		if !strings.Contains(got, want) {
			t.Errorf("Error() %q missing %q", got, want)
		}
	}
	if strings.Contains(got, "body text") || strings.Contains(got, "\n") {
		t.Errorf("Error() %q", got)
	}
	cause := errors.New("dial tcp: refused")
	n := newTransportError("GET", "/user", "host", cause, false, false)
	if !errors.Is(n, cause) || !strings.Contains(n.Error(), "network") {
		t.Errorf("transport Error() %q", n.Error())
	}
}

func TestLocalFileErrorShowsTheCause(t *testing.T) {
	cause := fmt.Errorf("read cube.glb: %w", errors.New("disk read failed"))
	e := newLocalFileError("POST", "/scenes/s1", cause)
	if e.Kind != LocalFile || e.BodySent || e.StatusCode != 0 || !errors.Is(e, cause) {
		t.Fatalf("%+v", e)
	}
	if e.UserMessage() != "read cube.glb: disk read failed" {
		t.Errorf("UserMessage %q", e.UserMessage())
	}
	if got := e.Error(); !strings.Contains(got, "POST /scenes/s1") || !strings.Contains(got, "local_file") {
		t.Errorf("Error() %q", got)
	}
}

func sent(e *Error) *Error {
	e.BodySent = true
	return e
}
