package cli

import (
	"encoding/json"
	"fmt"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/version"
)

const (
	formatFlag = "format"
	formatText = "text"
	formatJSON = "json"
)

func newVersionCmd(streams IO) *cobra.Command {
	return &cobra.Command{
		Use:         "version",
		Short:       "Print version, commit and build date",
		Annotations: map[string]string{skipRuntimeAnnotation: "true"},
		Args: func(_ *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErrorf("unexpected argument %q", args[0])
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			info := version.Current()
			format, _ := cmd.Flags().GetString(formatFlag)
			if format == formatJSON {
				return json.NewEncoder(streams.Stdout).Encode(info)
			}
			_, err := fmt.Fprintf(streams.Stdout, "%s %s\n", binaryName, info.Version)
			return err
		},
	}
}
