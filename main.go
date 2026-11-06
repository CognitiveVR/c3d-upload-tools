package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/cognitivevr/c3d-upload-tools/internal/cli"
	"github.com/cognitivevr/c3d-upload-tools/internal/ui"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	code := cli.Execute(ctx, os.Args[1:], cli.IO{
		Stdin:       os.Stdin,
		Stdout:      os.Stdout,
		Stderr:      os.Stderr,
		Interactive: ui.IsTerminal(os.Stdin) && ui.IsTerminal(os.Stderr),
	})
	stop()
	os.Exit(code)
}
