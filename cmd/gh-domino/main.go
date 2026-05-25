package main

import (
	"context"
	"errors"
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
	go func() {
		<-ctx.Done()
		stop()
	}()

	if err := cli.Run(ctx, os.Args[1:], os.Stdout, os.Stderr); err != nil {
		if errors.Is(err, context.Canceled) {
			os.Exit(130)
		}
		stderr("%s\n", err.Error())
		os.Exit(1)
	}
}
