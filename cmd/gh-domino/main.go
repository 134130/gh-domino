package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/134130/gh-domino/internal/cli"
)

func stderr(msg string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, msg, args...)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		stderr("%s\n", err.Error())
		os.Exit(1)
	}
}
