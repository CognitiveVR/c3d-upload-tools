package cli

import (
	"encoding/json"
	"fmt"
	"strconv"
	"text/tabwriter"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/scene"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
)

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
		_, _ = fmt.Fprintf(w, "%s\t%s\t%s\t%s\t%s\n", ui.Printable(info.SDKFacingID), gatewayIDCell(info), ui.Printable(info.SceneName), latest, ui.Printable(sdkVersion))
	}
	return w.Flush()
}

// gatewayIDCell is the gateway id, empty when it is the SDK-facing id.
func gatewayIDCell(info scene.Info) string {
	if info.ID == info.SDKFacingID {
		return ""
	}
	return ui.Printable(info.ID)
}
