package cli

import (
	"context"
	"regexp"

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

var sceneIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{1,255}$`)

// normalizeSceneID validates a scene id for use in a URL path. The gateway
// compares ids literally, so the id is passed through unchanged.
func normalizeSceneID(id string) (string, error) {
	if !sceneIDPattern.MatchString(id) {
		return "", usageErrorf("invalid scene id %q: use 1 to 255 letters, digits, '-' or '_'", id)
	}
	return id, nil
}
