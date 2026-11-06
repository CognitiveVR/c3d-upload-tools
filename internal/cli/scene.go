package cli

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"strings"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
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

	exitNotFound = 3
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
	var check *scene.CheckError
	if errors.As(err, &check) {
		for _, warning := range check.Warnings {
			rt.log.Warn("%s", warning)
		}
		for _, message := range check.Errors {
			rt.log.Error("%s", message)
		}
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

func newSceneListCmd(rt *runtime) *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List the key's scenes with their latest version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if err := preflight(ctx, rt); err != nil {
				return err
			}
			scenes, err := scene.List(ctx, rt.client())
			if err != nil {
				return err
			}
			return printSceneList(rt, scenes)
		},
	}
}

func printSceneList(rt *runtime, scenes []scene.Info) error {
	out := rt.streams.Stdout
	if rt.settings.Format == formatJSON {
		if scenes == nil {
			scenes = []scene.Info{}
		}
		return json.NewEncoder(out).Encode(scenes)
	}
	w := tabwriter.NewWriter(out, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "ID\tGATEWAY ID\tNAME\tLATEST\tSDK VERSION")
	for _, info := range scenes {
		latest, sdkVersion := "", ""
		if v, ok := scene.LatestVersion(info); ok {
			latest, sdkVersion = strconv.Itoa(v.Number), v.SDKVersion
		}
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", info.SDKFacingID, gatewayIDCell(info), info.SceneName, latest, sdkVersion)
	}
	return w.Flush()
}

// gatewayIDCell is the gateway id, empty when it is the SDK-facing id.
func gatewayIDCell(info scene.Info) string {
	if info.ID == info.SDKFacingID {
		return ""
	}
	return info.ID
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
	fmt.Fprintf(&b, "Scene: %s\nID: %s\n", info.SceneName, info.SDKFacingID)
	if cell := gatewayIDCell(info); cell != "" {
		fmt.Fprintf(&b, "Gateway ID: %s\n", cell)
	}
	w := tabwriter.NewWriter(&b, 0, 0, 2, ' ', 0)
	_, _ = fmt.Fprintln(w, "VERSION\tVERSION ID\tSDK VERSION")
	for _, v := range info.Versions {
		_, _ = fmt.Fprintf(w, "%d\t%d\t%s\n", v.Number, v.ID, v.SDKVersion)
	}
	if err := w.Flush(); err != nil {
		return err
	}
	_, err := io.WriteString(out, b.String())
	return err
}

func newSceneScreenshotCmd(rt *runtime) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "screenshot",
		Short: "Replace the screenshot of an existing scene version",
		Args:  noArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSceneScreenshot(cmd, rt)
		},
	}
	flags := cmd.Flags()
	flags.String(sceneIDFlag, "", "the scene (or set C3D_SCENE_ID)")
	flags.String(fileFlag, "", "PNG file to upload (required)")
	flags.Int(versionFlag, 0, "version number; the latest version when omitted")
	return cmd
}

var pngSignature = []byte{0x89, 0x50, 0x4E, 0x47, 0x0D, 0x0A, 0x1A, 0x0A}

func runSceneScreenshot(cmd *cobra.Command, rt *runtime) error {
	flags := cmd.Flags()
	file, _ := flags.GetString(fileFlag)
	if file == "" {
		return usageErrorf("--%s is required", fileFlag)
	}
	wanted, _ := flags.GetInt(versionFlag)
	if flags.Changed(versionFlag) && wanted < 1 {
		return usageErrorf("invalid --%s %d: want a version number of 1 or more", versionFlag, wanted)
	}
	sceneID, err := requireSceneID(rt)
	if err != nil {
		return err
	}
	size, err := checkPNG(file)
	if err != nil {
		return err
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
	var chosen scene.Version
	if flags.Changed(versionFlag) {
		var ok bool
		if chosen, ok = scene.VersionByNumber(info, wanted); !ok {
			return &codedError{code: exitNotFound, kind: "version_not_found", message: unknownVersionMessage(info, sceneID, wanted)}
		}
	} else {
		var ok bool
		if chosen, ok = scene.LatestVersion(info); !ok {
			return &codedError{code: exitNotFound, kind: "no_versions", message: fmt.Sprintf("scene %s has no versions", sceneID)}
		}
	}

	part := gateway.Part{
		Field: "screenshot", Filename: "screenshot.png", ContentType: "application/octet-stream", Size: size,
		Open: func() (io.ReadCloser, error) { return os.Open(file) },
	}
	query := url.Values{"version": {strconv.Itoa(chosen.Number)}}
	if _, err := client.PostMultipart(ctx, "/scenes/"+url.PathEscape(sceneID)+"/screenshot", query, []gateway.Part{part}, nil); err != nil {
		return err
	}
	if rt.settings.Format == formatJSON {
		return json.NewEncoder(rt.streams.Stdout).Encode(struct {
			SceneID            string `json:"sceneId"`
			VersionNumber      int    `json:"versionNumber"`
			ScreenshotReplaced bool   `json:"screenshotReplaced"`
		}{sceneID, chosen.Number, true})
	}
	rt.log.Info("Screenshot replaced for version %d", chosen.Number)
	return nil
}

func unknownVersionMessage(info scene.Info, sceneID string, wanted int) string {
	if len(info.Versions) == 0 {
		return fmt.Sprintf("scene %s has no version %d: it has no versions", sceneID, wanted)
	}
	numbers := make([]int, len(info.Versions))
	for i, v := range scene.NewestFirst(info.Versions) {
		numbers[i] = v.Number
	}
	return fmt.Sprintf("scene %s has no version %d; its versions are %s", sceneID, wanted, joinNumbers(numbers))
}

// checkPNG returns the size of file once it is known to be a regular file
// that starts with the PNG signature.
func checkPNG(file string) (int64, error) {
	info, err := os.Lstat(file)
	if err != nil {
		return 0, usageErrorf("cannot use screenshot %s: %v", file, err)
	}
	if !info.Mode().IsRegular() {
		return 0, usageErrorf("screenshot %s is not a PNG: it is not a regular file", file)
	}
	f, err := os.Open(file)
	if err != nil {
		return 0, usageErrorf("cannot use screenshot %s: %v", file, err)
	}
	defer func() { _ = f.Close() }()
	head := make([]byte, len(pngSignature))
	if _, err := io.ReadFull(f, head); err != nil || !bytes.Equal(head, pngSignature) {
		return 0, usageErrorf("screenshot %s is not a PNG: it does not start with the PNG signature", file)
	}
	return info.Size(), nil
}
