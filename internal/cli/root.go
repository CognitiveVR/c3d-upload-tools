package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

const (
	binaryName = "cognitive3d-cli-upload"

	exitFailure = 1
	exitUsage   = 4
)

type IO struct {
	Stdin       io.Reader
	Stdout      io.Writer
	Stderr      io.Writer
	Interactive bool
}

type usageError struct{ message string }

func (e *usageError) Error() string { return e.message }

func usageErrorf(format string, args ...any) error {
	return &usageError{message: fmt.Sprintf(format, args...)}
}

func normalizeFlag(_ *pflag.FlagSet, name string) pflag.NormalizedName {
	return pflag.NormalizedName(strings.ReplaceAll(name, "_", "-"))
}

func Execute(ctx context.Context, args []string, streams IO) int {
	return run(ctx, newRootCmd(streams), args, streams)
}

func run(ctx context.Context, root *cobra.Command, args []string, streams IO) int {
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	_, _ = fmt.Fprintf(streams.Stderr, "%s: %v\n", binaryName, err)
	var usage *usageError
	if errors.As(err, &usage) {
		return exitUsage
	}
	return exitFailure
}

func newRootCmd(streams IO) *cobra.Command {
	root := &cobra.Command{
		Use:           binaryName,
		Short:         "Upload scenes, dynamic objects and egocentric models to Cognitive3D",
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.ArbitraryArgs,
		PersistentPreRunE: func(cmd *cobra.Command, _ []string) error {
			format, _ := cmd.Flags().GetString("format")
			if format != formatText && format != formatJSON {
				return usageErrorf("invalid --format %q: want text or json", format)
			}
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) > 0 {
				return usageErrorf("unknown command %q", args[0])
			}
			return cmd.Help()
		},
	}
	root.SetIn(streams.Stdin)
	root.SetOut(streams.Stdout)
	root.SetErr(streams.Stderr)
	root.SetGlobalNormalizationFunc(normalizeFlag)
	root.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &usageError{message: err.Error()}
	})

	flags := root.PersistentFlags()
	flags.String("env", "", "environment: prod or dev")
	flags.String("gateway-url", "", "gateway base URL")
	flags.String("api-key", "", "developer API key")
	flags.String(formatFlag, formatText, "output format: text or json")
	flags.Duration("timeout", 0, "request timeout")
	flags.Bool("verbose", false, "verbose logging")
	flags.Bool("quiet", false, "suppress informational logging")
	flags.String("config", "", "path to the config file")

	root.AddCommand(newVersionCmd(streams))
	return root
}
