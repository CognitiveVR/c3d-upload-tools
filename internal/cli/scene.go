package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/ego"
	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/object"
	"github.com/cognitivevr/c3d-upload-tools/internal/scene"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
	"github.com/cognitivevr/c3d-upload-tools/internal/version"
)

const (
	sceneDirFlag  = "scene-dir"
	sceneNameFlag = "scene-name"
	dryRunFlag    = "dry-run"

	runTimeSceneName = "(resolved at run time)"

	fileFlag    = "file"
	versionFlag = "version"
)

func newSceneCmd(rt *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "scene",
		Short: "Upload and inspect scenes",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return cmd.Help()
		},
	}
	cmd.AddCommand(newSceneUploadCmd(rt), newSceneListCmd(rt), newSceneShowCmd(rt), newSceneScreenshotCmd(rt))
	return cmd
}

func newSceneUploadCmd(rt *runtime) *cobra.Command {
	upload := &cobra.Command{
		Use:   "upload",
		Short: "Create a scene, or a new version of one, from a directory",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSceneUpload(cmd, rt)
		},
	}
	flags := upload.Flags()
	flags.String(sceneDirFlag, "", "directory holding the model, its textures and screenshot.png (required)")
	flags.String(sceneNameFlag, "", "scene name; required for a new scene unless settings.json has one")
	flags.String(sceneIDFlag, "", "upload a new version of this scene")
	flags.Bool(dryRunFlag, false, "print the request without sending it")
	return upload
}

func runSceneUpload(cmd *cobra.Command, rt *runtime) error {
	flags := cmd.Flags()
	dir, _ := flags.GetString(sceneDirFlag)
	name, _ := flags.GetString(sceneNameFlag)
	dryRun, _ := flags.GetBool(dryRunFlag)
	if dir == "" {
		return usageErrorf("--%s is required", sceneDirFlag)
	}
	sceneID := rt.settings.SceneID
	if sceneID != "" {
		var err error
		if sceneID, err = normalizeSceneID(sceneID); err != nil {
			return err
		}
	}

	local, err := scene.Plan(dir)
	if err != nil {
		return planFailure(rt, err)
	}
	for _, warning := range local.Warnings {
		rt.log.Warn("%s", warning)
	}
	knownName := name
	if knownName == "" && local.Existing != nil {
		knownName = local.Existing.SceneName
	}
	if sceneID == "" && knownName == "" {
		return &usageError{message: scene.ErrNoSceneName.Error()}
	}

	if dryRun {
		dryName := name
		if knownName == "" {
			dryName = runTimeSceneName
			rt.log.Info("the scene name is read from scene %s when the upload runs, so the size of settings.json may differ", sceneID)
		}
		plan, err := scene.Finalize(local, dryName, version.SDKVersion())
		if err != nil {
			return err
		}
		return printSceneDryRun(rt, plan, scene.UploadPath(sceneID))
	}

	ctx := cmd.Context()
	if err := preflight(ctx, rt); err != nil {
		return err
	}
	client := rt.client()
	previousLatest := 0
	if sceneID != "" {
		info, err := scene.Describe(ctx, client, sceneID)
		if err != nil {
			return err
		}
		if latest, ok := scene.LatestVersion(info); ok {
			previousLatest = latest.Number
		}
		if knownName == "" {
			name = info.SceneName
		}
	}
	plan, err := scene.Finalize(local, name, version.SDKVersion())
	if errors.Is(err, scene.ErrNoSceneName) {
		return &usageError{message: err.Error()}
	}
	if err != nil {
		return err
	}

	progress := uploadProgress(rt, local.TotalBytes+int64(len(plan.SettingsBytes)))
	result, err := scene.Upload(ctx, client, plan, sceneID, previousLatest, rt.log, progress)
	if err != nil {
		if result.SceneID != "" {
			if printErr := printSceneResult(rt, result); printErr != nil {
				return printErr
			}
		}
		var notVisible *scene.VersionNotVisibleError
		if errors.As(err, &notVisible) {
			return &codedError{code: exitFailure, kind: "version_not_visible", message: notVisibleMessage(notVisible)}
		}
		return err
	}
	if err := printSceneResult(rt, result); err != nil {
		return err
	}
	if result.ConcurrentVersions {
		return &codedError{
			code: exitFailure, kind: "concurrent_versions",
			message: fmt.Sprintf("The upload finished, but versions %s appeared while it ran, so this upload cannot be told from another. Check with: %s",
				joinNumbers(result.CandidateVersions), sceneShowCommand(result.SceneID)),
		}
	}
	return nil
}

// planFailure shows every model-check finding before the summary, and makes
// any planning failure a usage error.
func planFailure(rt *runtime, err error) error {
	var sceneCheck *scene.CheckError
	var objectCheck *object.CheckError
	var egoCheck *ego.CheckError
	var warnings, errs []string
	switch {
	case errors.As(err, &sceneCheck):
		warnings, errs = sceneCheck.Warnings, sceneCheck.Errors
	case errors.As(err, &objectCheck):
		warnings, errs = objectCheck.Warnings, objectCheck.Errors
	case errors.As(err, &egoCheck):
		warnings, errs = egoCheck.Warnings, egoCheck.Errors
	}
	for _, warning := range warnings {
		rt.log.Warn("%s", warning)
	}
	for _, message := range errs {
		rt.log.Error("%s", message)
	}
	return &usageError{message: err.Error()}
}

func uploadProgress(rt *runtime, total int64) *ui.Progress {
	if rt.settings.Quiet || rt.settings.Format == formatJSON {
		return nil
	}
	f, ok := rt.streams.Stderr.(*os.File)
	if !ok || !ui.IsTerminal(f) {
		return nil
	}
	return ui.NewProgress(f, total, "Uploading")
}

func printSceneResult(rt *runtime, result scene.Result) error {
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(rt.streams.Stdout).Encode(result)
	}
	_, err := fmt.Fprintln(rt.streams.Stdout, result.SceneID)
	return err
}

type dryRunPart struct {
	Field    string `json:"field"`
	Filename string `json:"filename"`
	Bytes    int64  `json:"bytes"`
}

type dryRunRequest struct {
	DryRun        bool         `json:"dryRun"`
	Method        string       `json:"method"`
	Path          string       `json:"path"`
	Authorization string       `json:"authorization"`
	Parts         []dryRunPart `json:"parts"`
}

func printSceneDryRun(rt *runtime, plan *scene.UploadPlan, path string) error {
	request := dryRunRequest{
		DryRun: true, Method: http.MethodPost, Path: path,
		Authorization: "APIKEY:DEVELOPER " + redactKey(rt.settings.APIKey),
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
	_, err := io.WriteString(out, b.String())
	return err
}

// redactKey shows at most the first four characters of a key, and none of a
// key too short to keep the rest hidden.
func redactKey(key string) string {
	switch {
	case key == "":
		return "(not set)"
	case len(key) <= 4:
		return "…"
	}
	return key[:4] + "…"
}

func notVisibleMessage(e *scene.VersionNotVisibleError) string {
	check := "Check with: " + sceneShowCommand(e.SceneID)
	if e.Cause == nil {
		return fmt.Sprintf("The upload finished, but version %d of scene %s is not visible yet. %s", e.Expected, e.SceneID, check)
	}
	cause := e.Cause.Error()
	var gerr *gateway.Error
	if errors.As(e.Cause, &gerr) {
		cause = gerr.UserMessage()
	}
	return fmt.Sprintf("The upload finished, but version %d of scene %s could not be read back: %s\n%s", e.Expected, e.SceneID, cause, check)
}

func sceneShowCommand(sceneID string) string {
	return binaryName + " scene show --scene-id " + sceneID
}

// joinNumbers writes 2, 3 and 4.
func joinNumbers(numbers []int) string {
	words := make([]string, len(numbers))
	for i, n := range numbers {
		words[i] = strconv.Itoa(n)
	}
	if len(words) < 2 {
		return strings.Join(words, "")
	}
	return strings.Join(words[:len(words)-1], ", ") + " and " + words[len(words)-1]
}

func newSceneShowCmd(rt *runtime) *cobra.Command {
	show := &cobra.Command{
		Use:   "show",
		Short: "Show one scene with its versions, latest first",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			sceneID, err := requireSceneID(rt)
			if err != nil {
				return err
			}
			ctx := cmd.Context()
			if err := preflight(ctx, rt); err != nil {
				return err
			}
			info, err := scene.Describe(ctx, rt.client(), sceneID)
			if err != nil {
				return err
			}
			info.Versions = scene.NewestFirst(info.Versions)
			return printSceneInfo(rt, info)
		},
	}
	show.Flags().String(sceneIDFlag, "", "the scene to show (or set C3D_SCENE_ID)")
	return show
}

func requireSceneID(rt *runtime) (string, error) {
	if rt.settings.SceneID == "" {
		return "", usageErrorf("--%s is required (or set C3D_SCENE_ID)", sceneIDFlag)
	}
	return normalizeSceneID(rt.settings.SceneID)
}

func printSceneInfo(rt *runtime, info scene.Info) error {
	out := rt.streams.Stdout
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(out).Encode(info)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Scene: %s\nID: %s\n", ui.Printable(info.SceneName), ui.Printable(info.SDKFacingID))
	if cell := gatewayIDCell(info); cell != "" {
		fmt.Fprintf(&b, "Gateway ID: %s\n", cell)
	}
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "VERSION\tVERSION ID\tSDK VERSION")
	for _, v := range info.Versions {
		_, _ = fmt.Fprintf(w, "%d\t%d\t%s\n", v.Number, v.ID, ui.Printable(v.SDKVersion))
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := io.WriteString(out, b.String())
	return err
}
