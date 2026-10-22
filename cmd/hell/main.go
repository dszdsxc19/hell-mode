package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/dszdsxc19/hell-mode/internal/cli"
)

var version = "0.1.0-dev"

// main owns process termination; command code remains usable from integration tests.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(cli.Execute(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr, version))
}
