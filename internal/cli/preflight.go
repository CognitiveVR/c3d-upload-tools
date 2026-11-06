package cli

import (
	"context"
	"regexp"
	"strings"

	"github.com/cognitivevr/c3d-upload-tools/internal/config"
)

// preflight checks that a key is configured and accepted by the gateway, then
// marks the client so a later empty 401 on a scene route reads as "scene not
// accessible". Commands that name a scene call it after local planning and
// before their first other request.
func preflight(ctx context.Context, rt *runtime) error {
	if err := config.RequireAPIKey(rt.settings); err != nil {
		return &usageError{message: err.Error()}
	}
	client := rt.client()
	if err := client.VerifyKey(ctx); err != nil {
		return err
	}
	client.MarkPreflighted()
	return nil
}

const sceneIDFlag = "scene-id"

var (
	sceneIDPattern   = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)
	uuidShapePattern = regexp.MustCompile(`^[0-9A-Fa-f]{8}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{4}-[0-9A-Fa-f]{12}$`)
)

// normalizeSceneID validates a scene id for use in a URL path and lowercases
// a UUID-shaped one; any other form is passed through verbatim.
func normalizeSceneID(id string) (string, error) {
	if !sceneIDPattern.MatchString(id) {
		return "", usageErrorf("invalid scene id %q: use 1 to 255 letters, digits, '-' or '_'", id)
	}
	if uuidShapePattern.MatchString(id) {
		return strings.ToLower(id), nil
	}
	return id, nil
}
