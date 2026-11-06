package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/v2/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/v2/internal/scene"
)

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
	wanted, versionGiven, err := versionFlagValue(flags)
	if err != nil {
		return err
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
	if versionGiven {
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
