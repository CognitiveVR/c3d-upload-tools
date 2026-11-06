package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/manifest"
	"github.com/cognitivevr/c3d-upload-tools/internal/object"
	"github.com/cognitivevr/c3d-upload-tools/internal/scene"
)

const (
	objectDirFlag      = "object-dir"
	objectFilenameFlag = "object-filename"
	objectIDFlag       = "object-id"
	manifestFlag       = "manifest"
	uploadManifestFlag = "upload-manifest"

	runTimeVersion = "(resolved at run time)"
)

func newObjectCmd(rt *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "object",
		Short: "Upload dynamic objects",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newObjectUploadCmd(rt), newObjectManifestCmd(rt), newObjectListCmd(rt))
	return cmd
}

func newObjectUploadCmd(rt *runtime) *cobra.Command {
	upload := &cobra.Command{
		Use:   "upload",
		Short: "Upload one dynamic object's files and merge it into the local manifest file",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runObjectUpload(cmd, rt)
		},
	}
	flags := upload.Flags()
	flags.String(objectDirFlag, "", "directory holding the object's model, textures and cvr_object_thumbnail.png (required)")
	flags.String(objectFilenameFlag, "", "the object's model name without extension, which is also its mesh name (required)")
	flags.String(objectIDFlag, "", "manifest id; defaults to the id the manifest holds for this mesh, else a new UUID")
	flags.String(sceneIDFlag, "", "the scene to upload to (or set C3D_SCENE_ID)")
	flags.Int(versionFlag, 0, "scene version number; the latest version when omitted")
	flags.String(manifestFlag, "", "manifest file to merge the object into (default <scene id>_object_manifest.json)")
	flags.Bool(uploadManifestFlag, false, "also upload the manifest after the object")
	flags.Bool(dryRunFlag, false, "print the request without sending it")
	return upload
}

type objectResult struct {
	ObjectID         string `json:"objectId"`
	Mesh             string `json:"mesh"`
	SceneID          string `json:"sceneId"`
	VersionNumber    int    `json:"versionNumber"`
	ManifestPath     string `json:"manifestPath"`
	ManifestUploaded bool   `json:"manifestUploaded"`
}

func runObjectUpload(cmd *cobra.Command, rt *runtime) error {
	flags := cmd.Flags()
	dir, _ := flags.GetString(objectDirFlag)
	mesh, _ := flags.GetString(objectFilenameFlag)
	flagID, _ := flags.GetString(objectIDFlag)
	manifestFile, _ := flags.GetString(manifestFlag)
	uploadManifest, _ := flags.GetBool(uploadManifestFlag)
	dryRun, _ := flags.GetBool(dryRunFlag)
	wanted, _ := flags.GetInt(versionFlag)
	versionGiven := flags.Changed(versionFlag)
	switch {
	case dir == "":
		return usageErrorf("--%s is required", objectDirFlag)
	case mesh == "":
		return usageErrorf("--%s is required", objectFilenameFlag)
	case versionGiven && wanted < 1:
		return usageErrorf("invalid --%s %d: want a version number of 1 or more", versionFlag, wanted)
	}
	sceneID, err := requireSceneID(rt)
	if err != nil {
		return err
	}

	plan, err := object.Plan(dir, mesh)
	if err != nil {
		return planFailure(rt, err)
	}
	for _, warning := range plan.Warnings {
		rt.log.Warn("%s", warning)
	}

	if manifestFile == "" {
		manifestFile = manifest.DefaultPath(sceneID)
	}
	m, err := manifest.LoadOrEmpty(manifestFile)
	if err != nil {
		return &usageError{message: err.Error()}
	}
	if err := m.Validate(); err != nil {
		return usageErrorf("%s is not a valid manifest: %v", manifestFile, err)
	}
	id, source, err := object.ResolveID(flagID, m, mesh)
	if err != nil {
		return &usageError{message: err.Error()}
	}
	merged := &manifest.Manifest{Objects: slices.Clone(m.Objects)}
	merged.Upsert(id, mesh)
	if err := merged.Validate(); err != nil {
		return usageErrorf("cannot add the object to %s: %v", manifestFile, err)
	}
	rt.log.Info("Object id %s (%s)", id, describeIDSource(source, manifestFile))

	if dryRun {
		if uploadManifest {
			rt.log.Info("--%s would also POST %s?version=%s after the object", uploadManifestFlag, object.ManifestPath(sceneID), versionText(wanted, versionGiven))
		}
		return printObjectDryRun(rt, plan, sceneID, id, source, manifestFile, versionText(wanted, versionGiven))
	}

	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	versionNumber := wanted
	if !versionGiven {
		if versionNumber, err = latestVersionNumber(ctx, rt, sceneID); err != nil {
			return err
		}
	}

	client := rt.client()
	if err := object.Upload(ctx, client, plan, sceneID, versionNumber, rt.log, uploadProgress(rt, plan.TotalBytes)); err != nil {
		return err
	}
	m.Upsert(id, mesh)
	result := objectResult{ObjectID: id, Mesh: mesh, SceneID: sceneID, VersionNumber: versionNumber, ManifestPath: manifestFile}
	if err := m.Save(manifestFile); err != nil {
		if printErr := printObjectResult(rt, result); printErr != nil {
			return printErr
		}
		return fmt.Errorf("object %s was uploaded as id %s, but the manifest could not be saved: %w", mesh, id, err)
	}
	rt.log.Info("Saved object %s to %s", id, manifestFile)

	if uploadManifest {
		if err := m.Validate(); err != nil {
			return usageErrorf("%s is not a valid manifest: %v", manifestFile, err)
		}
		if err := object.UploadManifest(ctx, client, m, sceneID, versionNumber); err != nil {
			if printErr := printObjectResult(rt, result); printErr != nil {
				return printErr
			}
			return err
		}
		result.ManifestUploaded = true
		rt.log.Info("Uploaded the manifest to scene %s version %d", sceneID, versionNumber)
	}
	return printObjectResult(rt, result)
}

func latestVersionNumber(ctx context.Context, rt *runtime, sceneID string) (int, error) {
	info, err := scene.Describe(ctx, rt.client(), sceneID)
	if err != nil {
		return 0, err
	}
	latest, ok := scene.LatestVersion(info)
	if !ok {
		return 0, &codedError{code: exitNotFound, kind: "no_versions", message: fmt.Sprintf("scene %s has no versions", sceneID)}
	}
	return latest.Number, nil
}

func versionText(wanted int, given bool) string {
	if given {
		return strconv.Itoa(wanted)
	}
	return runTimeVersion
}

func describeIDSource(source object.IDSource, manifestFile string) string {
	if source == object.ReusedFromManifest {
		return "reused from " + manifestFile
	}
	return source.String()
}

func idSourceCode(source object.IDSource) string {
	switch source {
	case object.Flag:
		return "flag"
	case object.ReusedFromManifest:
		return "manifest"
	default:
		return "generated"
	}
}

func printObjectResult(rt *runtime, result objectResult) error {
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(rt.streams.Stdout).Encode(result)
	}
	_, err := fmt.Fprintf(rt.streams.Stdout, "Object ID: %s\n", result.ObjectID)
	return err
}

type objectDryRun struct {
	dryRunRequest
	ObjectID     string `json:"objectId"`
	IDSource     string `json:"idSource"`
	ManifestPath string `json:"manifestPath"`
}

func printObjectDryRun(rt *runtime, plan *object.UploadPlan, sceneID, id string, source object.IDSource, manifestFile, version string) error {
	request := objectDryRun{
		dryRunRequest: dryRunRequest{
			DryRun: true, Method: http.MethodPost, Path: object.UploadPath(sceneID, plan.Mesh) + "?version=" + version,
			Authorization: "APIKEY:DEVELOPER " + redactKey(rt.settings.APIKey),
		},
		ObjectID: id, IDSource: idSourceCode(source), ManifestPath: manifestFile,
	}
	for _, p := range plan.Parts {
		request.Parts = append(request.Parts, dryRunPart{Field: p.Field, Filename: p.Filename, Bytes: p.Size})
	}
	out := rt.streams.Stdout
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(out).Encode(request)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s\nAuthorization: %s\n", request.Method, request.Path, request.Authorization)
	for _, p := range request.Parts {
		fmt.Fprintf(&b, "%s  %s  %d bytes\n", p.Field, p.Filename, p.Bytes)
	}
	fmt.Fprintf(&b, "Object ID: %s (%s)\nManifest: %s\n", id, describeIDSource(source, manifestFile), manifestFile)
	_, err := io.WriteString(out, b.String())
	return err
}
