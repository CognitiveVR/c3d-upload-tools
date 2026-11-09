package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"slices"
	"strings"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/manifest"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/object"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/scene"
)

const (
	objectDirFlag      = "object-dir"
	objectFilenameFlag = "object-filename"
	objectIDFlag       = "object-id"
	manifestFlag       = "manifest"
	uploadManifestFlag = "upload-manifest"
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
	flags.Bool(uploadManifestFlag, true, "upload this object's manifest entry after its files, so it appears in the dashboard; --upload-manifest=false leaves that to object manifest upload")
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
	// ManifestEntryExisting is true when the version already listed the
	// object, so no manifest entry was sent.
	ManifestEntryExisting bool `json:"manifestEntryExisting"`
}

func runObjectUpload(cmd *cobra.Command, rt *runtime) error {
	flags := cmd.Flags()
	dir, _ := flags.GetString(objectDirFlag)
	mesh, _ := flags.GetString(objectFilenameFlag)
	flagID, _ := flags.GetString(objectIDFlag)
	manifestFile, _ := flags.GetString(manifestFlag)
	uploadManifest, _ := flags.GetBool(uploadManifestFlag)
	dryRun, _ := flags.GetBool(dryRunFlag)
	switch {
	case dir == "":
		return usageErrorf("--%s is required", objectDirFlag)
	case mesh == "":
		return usageErrorf("--%s is required", objectFilenameFlag)
	}
	wanted, versionGiven, err := versionFlagValue(flags)
	if err != nil {
		return err
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
	if dryRun {
		rt.log.Info("Object id %s (%s)", id, describeIDSource(source, manifestFile))
		manifestPost := ""
		if uploadManifest {
			manifestPost = object.ManifestPath(sceneID) + "?version=" + versionText(wanted, versionGiven)
			rt.log.Info("The object's manifest entry is then POSTed to %s unless the version already lists the object (--%s=false skips that)", manifestPost, uploadManifestFlag)
		}
		return printObjectDryRun(rt, plan, sceneID, id, source, manifestFile, versionText(wanted, versionGiven), manifestPost)
	}

	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	client := rt.client()
	versionNumber := wanted
	var listed []object.Listed
	listedOK := false
	if uploadManifest {
		// The version's current objects decide whether the entry is new, and
		// give a fresh checkout its id back instead of a second generated one.
		version, err := chooseVersion(ctx, rt, sceneID, wanted, versionGiven)
		if err != nil {
			return err
		}
		versionNumber = version.Number
		raw, err := object.List(ctx, client, version.ID)
		if err == nil {
			listed, err = object.All(raw)
		}
		if err != nil {
			rt.log.Warn("Could not list the objects of version %d (%s); the manifest entry is not uploaded, run %s afterwards", versionNumber, userMessage(err), manifestUploadCommand(sceneID, versionNumber, manifestFile))
		} else {
			listedOK = true
			if existing, ok := object.FindMesh(listed, mesh); ok && source == object.Generated {
				id, source = existing.SDKID, object.ReusedFromVersion
			}
		}
	} else if !versionGiven {
		if versionNumber, err = latestVersionNumber(ctx, rt, sceneID); err != nil {
			return err
		}
	}
	rt.log.Info("Object id %s (%s)", id, describeIDSource(source, manifestFile))

	if err := object.Upload(ctx, client, plan, sceneID, versionNumber, rt.log, uploadProgress(rt, plan.TotalBytes)); err != nil {
		return err
	}
	result := objectResult{ObjectID: id, Mesh: mesh, SceneID: sceneID, VersionNumber: versionNumber, ManifestPath: manifestFile}
	current, err := saveObjectEntry(manifestFile, id, mesh)
	if err != nil {
		if printErr := printObjectResult(rt, result); printErr != nil {
			return printErr
		}
		return fmt.Errorf("object %s was uploaded as id %s, but the manifest could not be saved: %w", mesh, id, err)
	}
	rt.log.Info("Saved object %s to %s", id, manifestFile)

	switch {
	case !uploadManifest:
		rt.log.Info("The object appears in the dashboard once the manifest is uploaded: run %s after your last object upload", manifestUploadCommand(sceneID, versionNumber, manifestFile))
		return printObjectResult(rt, result)
	case !listedOK:
		return printObjectResult(rt, result)
	}
	if _, ok := object.FindID(listed, id); ok {
		result.ManifestEntryExisting = true
		rt.log.Info("Version %d already lists object %s; its scale and position there are kept (run %s to push edits from %s)", versionNumber, id, manifestUploadCommand(sceneID, versionNumber, manifestFile), manifestFile)
		return printObjectResult(rt, result)
	}
	entry, ok := current.EntryByID(id)
	if !ok {
		if printErr := printObjectResult(rt, result); printErr != nil {
			return printErr
		}
		return fmt.Errorf("object %s was uploaded as id %s, but %s has no entry for it to upload", mesh, id, manifestFile)
	}
	result.ObjectID = entry.ID
	single := &manifest.Manifest{Objects: []manifest.Entry{entry}}
	if err := object.UploadManifest(ctx, client, single, sceneID, versionNumber); err != nil {
		if printErr := printObjectResult(rt, result); printErr != nil {
			return printErr
		}
		return err
	}
	result.ManifestUploaded = true
	rt.log.Info("Uploaded the manifest entry for object %s to scene %s version %d", entry.ID, sceneID, versionNumber)
	return printObjectResult(rt, result)
}

// manifestUploadCommand is the command that pushes the whole local manifest
// to the version this upload used.
func manifestUploadCommand(sceneID string, versionNumber int, manifestFile string) string {
	command := fmt.Sprintf("%s object manifest upload --%s %s --%s %d", binaryName, sceneIDFlag, sceneID, versionFlag, versionNumber)
	if manifestFile != manifest.DefaultPath(sceneID) {
		command += fmt.Sprintf(" --%s %s", manifestFlag, quoteArgument(manifestFile))
	}
	return command
}

// saveObjectEntry re-reads the manifest file, so entries written while the
// upload ran are kept, then upserts id and mesh, validates and saves it.
func saveObjectEntry(file, id, mesh string) (*manifest.Manifest, error) {
	m, err := manifest.LoadOrEmpty(file)
	if err != nil {
		return nil, err
	}
	m.Upsert(id, mesh)
	if err := m.Validate(); err != nil {
		return nil, fmt.Errorf("%s is not a valid manifest: %w", file, err)
	}
	if err := m.Save(file); err != nil {
		return nil, err
	}
	return m, nil
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

// quoteArgument double-quotes a value holding anything outside the characters
// cmd.exe, PowerShell and sh all pass through bare, or starting with a
// character sh expands. Quotes do not protect `$`, backquotes, `%`, `!`, a
// `"` or a doubled `\` in every shell; those paths stay the user's problem.
func quoteArgument(value string) string {
	if strings.HasPrefix(value, "~") || strings.HasPrefix(value, "=") {
		return `"` + value + `"`
	}
	for _, r := range value {
		plain := r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || strings.ContainsRune(`/.-_:~+=`, r)
		if !plain {
			return `"` + value + `"`
		}
	}
	return value
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
	case object.ReusedFromVersion:
		return "version"
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
	// ManifestPost is the path the object's manifest entry is POSTed to after
	// the files, empty with --upload-manifest=false.
	ManifestPost string `json:"manifestPost,omitempty"`
}

func printObjectDryRun(rt *runtime, plan *object.UploadPlan, sceneID, id string, source object.IDSource, manifestFile, version, manifestPost string) error {
	request := objectDryRun{
		dryRunRequest: newDryRunRequest(rt, http.MethodPost, object.UploadPath(sceneID, plan.Mesh)+"?version="+version, plan.Parts),
		ObjectID:      id,
		IDSource:      idSourceCode(source),
		ManifestPath:  manifestFile,
		ManifestPost:  manifestPost,
	}
	origin := describeIDSource(source, manifestFile)
	if source == object.Generated {
		origin = "illustrative; a new id is generated at upload"
	}
	extra := fmt.Sprintf("Object ID: %s (%s)\nManifest: %s\n", id, origin, manifestFile)
	if manifestPost != "" {
		extra += fmt.Sprintf("Then: POST %s with this object's manifest entry, unless the version already lists it\n", manifestPost)
	}
	return printDryRun(rt, request.dryRunRequest, request, extra)
}
