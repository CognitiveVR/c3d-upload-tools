package cli

import (
	"context"
	"os"

	"github.com/spf13/cobra"

	"github.com/cognitivevr/c3d-upload-tools/internal/config"
	"github.com/cognitivevr/c3d-upload-tools/internal/dotenv"
	"github.com/cognitivevr/c3d-upload-tools/internal/gateway"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
)

// skipRuntimeAnnotation marks commands that need no configuration, so a bad
// config file or environment variable cannot break them.
const skipRuntimeAnnotation = "skipRuntime"

// runtime is the per-invocation state shared by commands: the resolved
// settings, the logger and the lazily built gateway client. It is created by
// Execute and populated by the root PersistentPreRunE.
type runtime struct {
	streams  IO
	settings config.Settings
	log      *ui.Logger
	cancel   context.CancelFunc
	gw       *gateway.Client
}

func (rt *runtime) prepare(cmd *cobra.Command) error {
	if cmd == cmd.Root() || cmd.Annotations[skipRuntimeAnnotation] != "" {
		return nil
	}
	flags, err := flagValues(cmd)
	if err != nil {
		return err
	}
	dotenvValues, err := dotenv.LoadCurrentDirectory()
	if err != nil {
		return &usageError{message: err.Error()}
	}
	file, err := config.LoadFile(flags.ConfigPath)
	if err != nil {
		return &usageError{message: err.Error()}
	}
	settings, err := config.Resolve(flags, os.Getenv, dotenvValues, file)
	if err != nil {
		return &usageError{message: err.Error()}
	}
	rt.settings = settings
	rt.log = ui.New(rt.streams.Stderr, ui.Options{
		JSON:    settings.Format == formatJSON,
		Verbose: settings.Verbose,
		Quiet:   settings.Quiet,
		Color:   true,
	})
	for _, warning := range settings.Warnings {
		rt.log.Warn("%s", warning)
	}
	if settings.Timeout > 0 {
		ctx, cancel := context.WithTimeout(cmd.Context(), settings.Timeout)
		rt.cancel = cancel
		cmd.SetContext(ctx)
	}
	return nil
}

func (rt *runtime) release() {
	if rt.cancel != nil {
		rt.cancel()
	}
}

func flagValues(cmd *cobra.Command) (config.Flags, error) {
	fs := cmd.Flags()
	var flags config.Flags
	var err error
	stringFlags := []struct {
		name string
		dest *string
	}{
		{"env", &flags.Environment}, {"gateway-url", &flags.GatewayURL}, {"api-key", &flags.APIKey},
		{formatFlag, &flags.Format}, {"config", &flags.ConfigPath},
	}
	for _, s := range stringFlags {
		if *s.dest, err = fs.GetString(s.name); err != nil {
			return flags, err
		}
	}
	if flags.Timeout, err = fs.GetDuration("timeout"); err != nil {
		return flags, err
	}
	if flags.Verbose, err = fs.GetBool("verbose"); err != nil {
		return flags, err
	}
	if flags.Quiet, err = fs.GetBool("quiet"); err != nil {
		return flags, err
	}
	if fs.Lookup(sceneIDFlag) != nil {
		if flags.SceneID, err = fs.GetString(sceneIDFlag); err != nil {
			return flags, err
		}
	}
	return flags, nil
}

// client returns the gateway client, creating it on first use.
func (rt *runtime) client() *gateway.Client {
	if rt.gw == nil {
		rt.gw = gateway.New(rt.settings.GatewayURL, rt.settings.APIKey, gateway.UserAgent(), rt.log)
	}
	return rt.gw
}
