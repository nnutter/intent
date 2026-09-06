// Command intent lists Go refactorings in observed repository changes.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nnutter/intent/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		os.Exit(1)
	}
}
