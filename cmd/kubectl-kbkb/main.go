package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/omakeno/kbkb/v2/internal/cli"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if err := cli.NewCmd().ExecuteContext(ctx); err != nil {
		os.Exit(1)
	}
}
