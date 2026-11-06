package cli

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/manifest"
	"github.com/cognitivevr/c3d-upload-tools/internal/object"
	"github.com/cognitivevr/c3d-upload-tools/internal/scene"
)

func newObjectManifestCmd(rt *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "manifest",
		Short: "Work with the object manifest",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	upload := &cobra.Command{
		Use:   "upload",
		Short: "POST the local manifest file to a scene version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObjectManifestUpload(cmd, rt)
		},
	}
	flags := upload.Flags()
	flags.String(sceneIDFlag, "", "the scene to upload to (or set C3D_SCENE_ID)")
	flags.Int(versionFlag, 0, "scene version number; the latest version when omitted")
	flags.String(manifestFlag, "", "manifest file to upload (default <scene id>_object_manifest.json)")
	flags.Bool(dryRunFlag, false, "print the request without sending it")
	cmd.AddCommand(upload)
	return cmd
}

type manifestUploadResult struct {
	SceneID             string   `json:"sceneId"`
	VersionNumber       int      `json:"versionNumber"`
	ObjectCount         int      `json:"objectCount"`
	ObjectsWithoutFiles []string `json:"objectsWithoutFiles"`
}

func runObjectManifestUpload(cmd *cobra.Command, rt *runtime) error {
	flags := cmd.Flags()
	file, _ := flags.GetString(manifestFlag)
	dryRun, _ := flags.GetBool(dryRunFlag)
	wanted, _ := flags.GetInt(versionFlag)
	versionGiven := flags.Changed(versionFlag)
	if versionGiven && wanted < 1 {
		return usageErrorf("invalid --%s %d: want a version number of 1 or more", versionFlag, wanted)
	}
	sceneID, err := requireSceneID(rt)
	if err != nil {
		return err
	}
	if file == "" {
		file = manifest.DefaultPath(sceneID)
	}
	m, err := manifest.Load(file)
	if err != nil {
		return &usageError{message: err.Error()}
	}
	if err := m.Validate(); err != nil {
		return usageErrorf("%s is not a valid manifest: %v", file, err)
	}
	if dryRun {
		version := "<latest>"
		if versionGiven {
			version = versionText(wanted, true)
		}
		return printManifestDryRun(rt, sceneID, version, file, len(m.Objects))
	}

	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	client := rt.client()
	info, err := scene.Describe(ctx, client, sceneID)
	if err != nil {
		return err
	}
	chosen, err := pickVersion(info, sceneID, wanted, versionGiven)
	if err != nil {
		return err
	}
	if err := object.UploadManifest(ctx, client, m, sceneID, chosen.Number); err != nil {
		return err
	}
	rt.log.Info("Manifest uploaded: %d objects to version %d", len(m.Objects), chosen.Number)

	raw, err := object.List(ctx, client, chosen.ID)
	if err != nil {
		return fmt.Errorf("the manifest was uploaded, but listing the objects of version %d failed: %w", chosen.Number, err)
	}
	missing, err := object.WithoutFiles(raw)
	if err != nil {
		return err
	}
	result := manifestUploadResult{SceneID: sceneID, VersionNumber: chosen.Number, ObjectCount: len(m.Objects), ObjectsWithoutFiles: []string{}}
	for _, o := range missing {
		rt.log.Warn("object %q (mesh %s) has no files in version %d; upload it again for this version", o.SDKID, o.MeshName, chosen.Number)
		result.ObjectsWithoutFiles = append(result.ObjectsWithoutFiles, o.SDKID)
	}
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(rt.streams.Stdout).Encode(result)
	}
	return nil
}

type manifestDryRun struct {
	dryRunRequest
	ManifestPath string `json:"manifestPath"`
	ObjectCount  int    `json:"objectCount"`
}

func printManifestDryRun(rt *runtime, sceneID, version, file string, count int) error {
	request := manifestDryRun{
		dryRunRequest: dryRunRequest{
			DryRun: true, Method: http.MethodPost, Path: object.ManifestPath(sceneID) + "?version=" + version,
			Authorization: "APIKEY:DEVELOPER " + redactKey(rt.settings.APIKey),
		},
		ManifestPath: file, ObjectCount: count,
	}
	out := rt.streams.Stdout
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(out).Encode(request)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\nAuthorization: %s\nObjects: %d\nManifest: %s\n", request.Method, request.Path, request.Authorization, count, file)
	_, err := io.WriteString(out, b.String())
	return err
}
