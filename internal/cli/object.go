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
	if err := checkMerge(m, id, mesh, nil); err != nil {
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
	// remote holds the version's objects as manifest entries, nil when they
	// could not be listed or were not asked for.
	var remote *manifest.Manifest
	if uploadManifest {
		version, err := chooseVersion(ctx, rt, sceneID, wanted, versionGiven)
		if err != nil {
			return err
		}
		versionNumber = version.Number
		raw, err := object.List(ctx, client, version.ID)
		if err == nil {
			remote, err = manifest.FromObjectList(raw)
		}
		if err != nil {
			remote = nil
			unchecked := ""
			if source == object.Generated {
				unchecked = fmt.Sprintf(", and the new id for mesh %s was generated without checking whether the version already lists it", mesh)
			}
			rt.log.Warn("Could not list the objects of version %d (%s); the manifest entry is not uploaded%s. Check them with %s before running %s", versionNumber, userMessage(err), unchecked, objectListCommand(sceneID, versionNumber), manifestUploadCommand(sceneID, versionNumber, manifestFile))
		} else if rowID, ok := remote.IDForMesh(mesh); ok && !strings.EqualFold(rowID, id) {
			_, taken := m.EntryByID(rowID)
			switch {
			case source != object.Generated:
				rt.log.Warn("Version %d already lists mesh %s as object %s; this upload uses id %s (%s), so the dashboard will show both", versionNumber, mesh, rowID, id, describeIDSource(source, manifestFile))
			case taken || manifest.ValidateID(rowID) != nil:
				rt.log.Warn("Version %d lists mesh %s as object %s, but that id cannot be reused here (%s), so a new id is generated and the dashboard will show both", versionNumber, mesh, rowID, reuseRefusal(taken, manifestFile))
			default:
				id, source = rowID, object.ReusedFromVersion
			}
		}
	} else if !versionGiven {
		if versionNumber, err = latestVersionNumber(ctx, rt, sceneID); err != nil {
			return err
		}
	}
	// The local entry for an id the version already lists starts from the
	// scale and position the version holds, so a later manifest upload does
	// not replace them with defaults.
	var seed *manifest.Entry
	if remote != nil {
		if row, ok := remote.EntryByID(id); ok {
			row.Mesh = mesh
			seed = &row
			if err := checkMerge(m, id, mesh, seed); err != nil {
				return usageErrorf("cannot add the object to %s: %v", manifestFile, err)
			}
		}
	}
	rt.log.Info("Object id %s (%s)", id, describeIDSource(source, manifestFile))

	if err := object.Upload(ctx, client, plan, sceneID, versionNumber, rt.log, uploadProgress(rt, plan.TotalBytes)); err != nil {
		return err
	}
	result := objectResult{ObjectID: id, Mesh: mesh, SceneID: sceneID, VersionNumber: versionNumber, ManifestPath: manifestFile}
	current, err := saveObjectEntry(manifestFile, id, mesh, seed)
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
	case remote == nil:
		return printObjectResult(rt, result)
	}
	row, listed := remote.EntryByID(id)
	if listed && row.Mesh == mesh {
		result.ManifestEntryExisting = true
		rt.log.Info("Version %d already lists object %s with mesh %s, so no manifest entry is sent; its scale and position are in %s (run %s to push edits)", versionNumber, id, mesh, manifestFile, manifestUploadCommand(sceneID, versionNumber, manifestFile))
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
	if listed {
		rt.log.Info("Updated the manifest entry for object %s in scene %s version %d from mesh %s to %s", entry.ID, sceneID, versionNumber, row.Mesh, mesh)
	} else {
		rt.log.Info("Uploaded the manifest entry for object %s to scene %s version %d", entry.ID, sceneID, versionNumber)
	}
	return printObjectResult(rt, result)
}

// reuseRefusal says why a listed object's id is not reused for a new entry.
func reuseRefusal(taken bool, manifestFile string) string {
	if taken {
		return manifestFile + " uses it for another mesh"
	}
	return "it is not a valid manifest id"
}

// checkMerge validates the manifest as saveObjectEntry would save it.
func checkMerge(m *manifest.Manifest, id, mesh string, seed *manifest.Entry) error {
	merged := &manifest.Manifest{Objects: slices.Clone(m.Objects)}
	adoptEntry(merged, id, mesh, seed)
	return merged.Validate()
}

func adoptEntry(m *manifest.Manifest, id, mesh string, seed *manifest.Entry) {
	if seed != nil {
		m.Adopt(*seed)
		return
	}
	m.Upsert(id, mesh)
}

func objectListCommand(sceneID string, versionNumber int) string {
	return fmt.Sprintf("%s object list --%s %s --%s %d", binaryName, sceneIDFlag, sceneID, versionFlag, versionNumber)
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
// upload ran are kept, then upserts id and mesh (seed, when given, is the
// entry appended if none has the id), validates and saves it.
func saveObjectEntry(file, id, mesh string, seed *manifest.Entry) (*manifest.Manifest, error) {
	m, err := manifest.LoadOrEmpty(file)
	if err != nil {
		return nil, err
	}
	adoptEntry(m, id, mesh, seed)
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
