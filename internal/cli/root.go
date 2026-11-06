package cli

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
)

const (
	binaryName = "cognitive3d-cli-upload"

	exitFailure  = 1
	exitNotFound = 3
	exitUsage    = 4
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
	rt := &runtime{streams: streams}
	return run(ctx, newRootCmd(streams, rt), rt, args, streams)
}

// codedError is a failure with its own exit code and machine-readable kind,
// for outcomes the command words itself.
type codedError struct {
	code       int
	kind       string
	statusCode int
	message    string
}

func (e *codedError) Error() string { return e.message }

func noArgs(_ *cobra.Command, args []string) error {
	if len(args) > 0 {
		return usageErrorf("unexpected argument %q", args[0])
	}
	return nil
}

func run(ctx context.Context, root *cobra.Command, rt *runtime, args []string, streams IO) int {
	defer rt.release()
	root.SetArgs(args)
	err := root.ExecuteContext(ctx)
	if err == nil {
		return 0
	}
	return report(rt, streams, args, err)
}

// report presents err on stderr and returns the exit code. Before the
// logger exists, args decide whether the record is JSON.
func report(rt *runtime, streams IO, args []string, err error) int {
	var log *ui.Logger
	if rt != nil {
		log = rt.log
	}
	jsonMode := log != nil && rt.settings.Format == formatJSON
	if log == nil {
		jsonMode = formatFromArgs(args) == formatJSON
		if jsonMode {
			log = ui.New(streams.Stderr, ui.Options{JSON: true})
		}
	}
	code, kind, status, message, hint := exitFailure, "error", 0, err.Error(), ""
	var gerr *gateway.Error
	var coded *codedError
	var usage *usageError
	switch {
	case errors.As(err, &gerr):
		code, kind, status, message = gateway.ExitCode(err), gerr.Kind.String(), gerr.StatusCode, gerr.UserMessage()
		if jsonMode {
			message, hint = gerr.MessageAndHint()
		}
	case errors.As(err, &coded):
		code, kind, status, message = coded.code, coded.kind, coded.statusCode, coded.message
	case errors.As(err, &usage):
		code, kind = exitUsage, "usage"
	}
	switch {
	case jsonMode:
		log.ErrorRecord(kind, message, hint, status)
	case log == nil:
		_, _ = fmt.Fprintf(streams.Stderr, "%s: %s\n", binaryName, message)
	default:
		log.Error("%s", message)
	}
	return code
}

// formatFromArgs is the last --format value in args before a "--", or empty.
func formatFromArgs(args []string) string {
	format := ""
	for i, arg := range args {
		switch {
		case arg == "--":
			return format
		case arg == "--"+formatFlag && i+1 < len(args):
			format = args[i+1]
		case strings.HasPrefix(arg, "--"+formatFlag+"="):
			format = strings.TrimPrefix(arg, "--"+formatFlag+"=")
		}
	}
	return format
}

func newRootCmd(streams IO, rt *runtime) *cobra.Command {
	cobra.EnableTraverseRunHooks = true
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
			return rt.prepare(cmd)
		},
		PersistentPostRunE: func(*cobra.Command, []string) error {
			rt.release()
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
	flags.String("gateway-url", "", "gateway base URL, including the API version path, for example https://data.c3ddev.com/v0")
	flags.String("api-key", "", "developer API key")
	flags.String(formatFlag, formatText, "output format: text or json")
	flags.Duration("timeout", 0, "request timeout")
	flags.Bool("verbose", false, "verbose logging")
	flags.Bool("quiet", false, "suppress informational logging")
	flags.String("config", "", "path to the config file")

	root.AddCommand(newVersionCmd(streams), newAuthCmd(rt), newSceneCmd(rt), newObjectCmd(rt), newEgoCmd(rt))
	return root
}
