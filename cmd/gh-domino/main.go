package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"

	"github.com/134130/gh-domino/internal/domino"
)

func stderr(msg string, args ...interface{}) {
	_, _ = fmt.Fprintf(os.Stderr, msg, args...)
}

func main() {
	cfg, err := domino.ParseConfig()
	if err != nil {
		stderr("%s", err.Error())
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	if err := domino.Run(ctx, cfg); err != nil {
		stderr("%s\n", err.Error())
		os.Exit(1)
	}
}
