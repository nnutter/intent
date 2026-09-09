// Command intent lists Go refactorings in observed repository changes.
package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/nnutter/intent/internal/cli"
)

// version is the binary version reported by --version.
// Override it when building:
//
//	go build -ldflags "-X main.version=v1.2.3" .
var version = "dev"

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.Execute(ctx, os.Args[1:], os.Stdout, os.Stderr, version); err != nil {
		os.Exit(1)
	}
}
