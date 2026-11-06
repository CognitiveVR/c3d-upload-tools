package cli

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/manifest"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/object"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/scene"
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

// manifestUploadResult has ObjectsWithoutFiles nil, encoded as null, when the
// follow-up listing failed and the objects were not verified.
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
	wanted, versionGiven, err := versionFlagValue(flags)
	if err != nil {
		return err
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
		return printManifestDryRun(rt, sceneID, versionText(wanted, versionGiven), file, len(m.Objects))
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

	result := manifestUploadResult{SceneID: sceneID, VersionNumber: chosen.Number, ObjectCount: len(m.Objects)}
	raw, err := object.List(ctx, client, chosen.ID)
	if err != nil {
		rt.log.Warn("The manifest was uploaded, but listing the objects of version %d failed: %s", chosen.Number, userMessage(err))
		if rt.settings.Format == formatJSON {
			return json.NewEncoder(rt.streams.Stdout).Encode(result)
		}
		rt.log.Info("Objects without files: not verified")
		return nil
	}
	missing, err := object.WithoutFiles(raw)
	if err != nil {
		return err
	}
	result.ObjectsWithoutFiles = []string{}
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
		dryRunRequest: newDryRunRequest(rt, http.MethodPost, object.ManifestPath(sceneID)+"?version="+version, nil),
		ManifestPath:  file,
		ObjectCount:   count,
	}
	return printDryRun(rt, request.dryRunRequest, request, fmt.Sprintf("Objects: %d\nManifest: %s\n", count, file))
}
