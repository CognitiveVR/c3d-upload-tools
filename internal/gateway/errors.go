package gateway

import (
	"errors"
	"fmt"
	"net/http"
	"strings"
)

type ErrorKind int

const (
	KeyExpired ErrorKind = iota
	Unauthorized
	SceneNotAccessible
	Forbidden
	NotFound
	Conflict
	TooLarge
	ManifestRejected
	ClientError
	ServerError
	Network
	Interrupted
	LocalFile
)

var kindNames = [...]string{
	"key_expired", "unauthorized", "scene_not_accessible", "forbidden", "not_found", "conflict",
	"too_large", "manifest_rejected", "client_error", "server_error", "network", "interrupted",
	"local_file",
}

// String is the snake_case code used in JSON error records.
func (k ErrorKind) String() string {
	if k < 0 || int(k) >= len(kindNames) {
		return fmt.Sprintf("kind_%d", int(k))
	}
	return kindNames[k]
}

const (
	binaryName       = "cognitive3d-cli-upload"
	bodyDisplayLimit = 300
	noDetails        = "(no details from the gateway)"
)

const keyRotationSteps = `To fix this issue:
  1. Log into the Cognitive3D dashboard
  2. Go to Settings (gear icon) → 'Manage developer key'
  3. Generate a new developer API key
  4. Update your environment variable: export C3D_DEVELOPER_API_KEY="your_new_key"`

const keyVerificationSteps = `Verify your API key is correct:
  1. Check the Cognitive3D dashboard: Settings → 'Manage developer key'
  2. Ensure you're using the correct environment (--env prod or --env dev)
  3. Update your key: export C3D_DEVELOPER_API_KEY="your_correct_key"`

// Error is a failed gateway call. StatusCode is zero for Network,
// Interrupted and LocalFile. Error() never contains the API key or the
// response body.
type Error struct {
	Operation  string
	StatusCode int
	Body       string
	Kind       ErrorKind
	BodySent   bool

	method      string
	path        string
	host        string
	contentType string
	cause       error
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("gateway: %s: %s: %v", e.Operation, e.Kind, e.cause)
	}
	return fmt.Sprintf("gateway: %s: status %d (%s)", e.Operation, e.StatusCode, e.Kind)
}

func (e *Error) Unwrap() error { return e.cause }

// ExitCode maps an error to the process exit code: 0 for nil, 2 for
// authentication, 3 for not found or not accessible, 4 for too large or a
// local file that could not be read, else 1.
func ExitCode(err error) int {
	if err == nil {
		return 0
	}
	var gerr *Error
	if !errors.As(err, &gerr) {
		return 1
	}
	switch gerr.Kind {
	case KeyExpired, Unauthorized, Forbidden:
		return 2
	case SceneNotAccessible, NotFound:
		return 3
	case TooLarge, LocalFile:
		return 4
	}
	return 1
}

// newLocalFileError is a failure to open or read a part during an upload;
// cause names the file.
func newLocalFileError(method, path string, cause error) *Error {
	return &Error{Operation: method + " " + path, Kind: LocalFile, method: method, path: path, cause: cause}
}

func newStatusError(method, path string, status int, body []byte, contentType string, preflighted bool) *Error {
	e := &Error{
		Operation: method + " " + path, StatusCode: status, Body: string(body),
		method: method, path: path, contentType: contentType,
	}
	e.Kind = classify(path, status, strings.TrimSpace(e.Body), preflighted)
	return e
}

func newTransportError(method, path, host string, cause error, bodySent, interrupted bool) *Error {
	kind := Network
	if interrupted {
		kind = Interrupted
	}
	return &Error{
		Operation: method + " " + path, Kind: kind, BodySent: bodySent,
		method: method, path: path, host: host, cause: cause,
	}
}

func classify(path string, status int, body string, preflighted bool) ErrorKind {
	switch {
	case status == http.StatusUnauthorized && strings.Contains(strings.ToLower(body), "key expired"):
		return KeyExpired
	case status == http.StatusUnauthorized && body == "" && preflighted && isSceneScoped(path):
		return SceneNotAccessible
	case status == http.StatusUnauthorized:
		return Unauthorized
	case status == http.StatusForbidden:
		return Forbidden
	case status == http.StatusNotFound:
		return NotFound
	case status == http.StatusConflict:
		return Conflict
	case status == http.StatusRequestEntityTooLarge:
		return TooLarge
	case status == http.StatusBadRequest && body == "" && isManifestRoute(path):
		return ManifestRejected
	case status >= 500:
		return ServerError
	}
	return ClientError
}

// UserMessage is the text shown to the user for this error.
func (e *Error) UserMessage() string {
	switch e.Kind {
	case KeyExpired:
		return "Your developer API key has expired.\n\n" + keyRotationSteps
	case SceneNotAccessible:
		if id := sceneIDFromPath(e.path); id != "" {
			return "Scene " + id + " was not found in the project this key belongs to. Check the id with: " + binaryName + " scene list"
		}
		return "The scene was not found in the project this key belongs to. Check the id with: " + binaryName + " scene list"
	case Unauthorized:
		if body := e.displayBody(); body != "" {
			return "Authentication failed: " + body + "\n\n" + keyVerificationSteps
		}
		return "Authentication failed. Please check your developer API key.\n\n" + keyVerificationSteps
	case Forbidden:
		return "Access forbidden. Your API key may not have permission for this operation."
	case NotFound:
		return "Resource not found. The ID may be incorrect or the resource may not exist."
	case Conflict:
		return "The gateway reported a file conflict: " + orNoDetails(e.displayBody())
	case TooLarge:
		return "The upload exceeds the gateway's size limit (3 GiB per request)."
	case ManifestRejected:
		return "The gateway rejected the manifest (status 400). The usual causes are duplicate ids or malformed vectors; the gateway does not say which."
	case ServerError:
		if body := collapseWhitespace(e.Body); body != "" && !e.isHTML() && len([]rune(body)) < bodyDisplayLimit {
			return fmt.Sprintf("The Cognitive3D gateway returned a server error (status %d): %s", e.StatusCode, body)
		}
		return fmt.Sprintf("The Cognitive3D gateway returned a server error (status %d). Please try again later.", e.StatusCode)
	case Network, Interrupted:
		if e.BodySent {
			if check, retry := checkCommand(e.method, e.path); check != "" {
				return "The connection was lost while waiting for the gateway's answer. The upload may still have completed. Check with: " +
					check + " before retrying, because " + retry
			}
		}
		return fmt.Sprintf("Could not reach %s: %v", e.host, e.cause)
	case LocalFile:
		if e.cause != nil {
			return e.cause.Error()
		}
	}
	return fmt.Sprintf("%s failed with status %d: %s", e.Operation, e.StatusCode, orNoDetails(e.displayBody()))
}

// displayBody is the body with whitespace collapsed and cut to 300
// characters, or empty when the body is empty or HTML.
func (e *Error) displayBody() string {
	if e.isHTML() {
		return ""
	}
	body := []rune(collapseWhitespace(e.Body))
	if len(body) > bodyDisplayLimit {
		body = body[:bodyDisplayLimit]
	}
	return string(body)
}

func (e *Error) isHTML() bool {
	return strings.Contains(strings.ToLower(e.contentType), "html") || strings.HasPrefix(strings.TrimSpace(e.Body), "<")
}

func orNoDetails(body string) string {
	if body == "" {
		return noDetails
	}
	return body
}

func collapseWhitespace(s string) string { return strings.Join(strings.Fields(s), " ") }

func segments(path string) []string {
	return strings.Split(strings.Trim(path, "/"), "/")
}

func isSceneScoped(path string) bool {
	seg := segments(path)
	if len(seg) < 2 {
		return false
	}
	switch seg[0] {
	case "scenes", "objects", "versions":
		return true
	}
	return false
}

func isManifestRoute(path string) bool {
	seg := segments(path)
	return len(seg) == 2 && seg[0] == "objects" && seg[1] != ""
}

func sceneIDFromPath(path string) string {
	seg := segments(path)
	if len(seg) >= 2 && (seg[0] == "scenes" || seg[0] == "objects") {
		return seg[1]
	}
	return ""
}

const (
	retryCreatesVersion  = "a retry would create another version."
	retryReplacesContent = "a retry replaces the same content."
)

// checkCommand names the command that shows whether an upload landed and
// what a retry of it does, or returns empty for routes that are not uploads.
// Only POST /scenes and POST /scenes/{id} create versions.
func checkCommand(method, path string) (command, retry string) {
	if method != http.MethodPost {
		return "", ""
	}
	seg := segments(path)
	switch {
	case seg[0] == "scenes" && len(seg) <= 2:
		return binaryName + " scene list", retryCreatesVersion
	case seg[0] == "scenes" && len(seg) == 3 && seg[2] == "screenshot":
		return binaryName + " scene list", retryReplacesContent
	case len(seg) == 3 && seg[0] == "objects":
		return binaryName + " object list --scene-id " + seg[1], retryReplacesContent
	case len(seg) == 4 && seg[0] == "scenes" && seg[2] == "egos":
		return binaryName + " ego list --scene-id " + seg[1], retryReplacesContent
	}
	return "", ""
}
