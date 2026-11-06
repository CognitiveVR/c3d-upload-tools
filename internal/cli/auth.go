package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/ui"
)

type authResult struct {
	Environment      string     `json:"environment"`
	ProjectName      string     `json:"projectName"`
	OrganizationName string     `json:"organizationName"`
	KeyExpiresAt     *time.Time `json:"keyExpiresAt"`
	SceneReachable   *bool      `json:"sceneReachable"`
}

func newAuthCmd(rt *runtime) *cobra.Command {
	auth := &cobra.Command{
		Use:   "auth",
		Short: "Check the developer API key",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	auth.AddCommand(newAuthVerifyCmd(rt))
	return auth
}

func newAuthVerifyCmd(rt *runtime) *cobra.Command {
	verify := &cobra.Command{
		Use:   "verify",
		Short: "Verify the developer API key and report project, organization and key expiry",
		Long: `Verify the developer API key and report project, organization and key expiry.

With --scene-id, also check that the scene is reachable with this key. A scene
the key cannot reach is reported (the result shows sceneReachable false in
JSON) and the command exits 3.`,
		Args: noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAuthVerify(cmd, rt)
		},
	}
	verify.Flags().String(sceneIDFlag, "", "also check that this scene is reachable with the key")
	return verify
}

func runAuthVerify(cmd *cobra.Command, rt *runtime) error {
	sceneID := rt.settings.SceneID
	if sceneID != "" {
		var err error
		if sceneID, err = normalizeSceneID(sceneID); err != nil {
			return err
		}
	}

	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	client := rt.client()
	user, err := client.User(ctx)
	if err != nil {
		return err
	}

	result := authResult{
		Environment:      rt.settings.Environment,
		ProjectName:      user.ProjectName,
		OrganizationName: user.OrganizationName,
	}
	if user.KeyExpiresAtMilliseconds != 0 {
		expiry := time.UnixMilli(user.KeyExpiresAtMilliseconds).UTC()
		result.KeyExpiresAt = &expiry
	}

	var sceneErr error
	if sceneID != "" {
		reachable := true
		if err := client.Get(ctx, "/scenes/"+sceneID, nil, nil); err != nil {
			var gerr *gateway.Error
			if !errors.As(err, &gerr) || gerr.Kind != gateway.SceneNotAccessible {
				return err
			}
			reachable = false
			sceneErr = &codedError{
				code: gateway.ExitCode(err), kind: gerr.Kind.String(), statusCode: gerr.StatusCode,
				message: fmt.Sprintf("Scene %s is not reachable with this key", sceneID),
			}
		}
		result.SceneReachable = &reachable
	}

	if err := printAuthResult(rt, result, sceneID); err != nil {
		return err
	}
	return sceneErr
}

func printAuthResult(rt *runtime, result authResult, sceneID string) error {
	out := rt.streams.Stdout
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(out).Encode(result)
	}
	expiry := "never"
	if result.KeyExpiresAt != nil {
		expiry = result.KeyExpiresAt.Format(time.RFC3339)
	}
	_, err := fmt.Fprintf(out, "Environment: %s\nProject: %s\nOrganization: %s\nKey expires: %s\n",
		result.Environment, ui.Printable(result.ProjectName), ui.Printable(result.OrganizationName), expiry)
	if err == nil && result.SceneReachable != nil && *result.SceneReachable {
		_, err = fmt.Fprintf(out, "Scene %s: reachable\n", sceneID)
	}
	return err
}
