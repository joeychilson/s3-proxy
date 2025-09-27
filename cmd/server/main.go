package main

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/joeychilson/s3-proxy/internal/config"
	"github.com/joeychilson/s3-proxy/internal/server"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := config.Load()
	if err != nil {
		slog.Error("load config", "error", err)
		os.Exit(1)
	}

	srv, err := server.New(ctx, cfg)
	if err != nil {
		slog.Error("init server", "error", err)
		os.Exit(1)
	}

	if err := srv.ListenAndServe(ctx); err != nil && !errors.Is(err, context.Canceled) {
		slog.Error("server exit", "error", err)
		os.Exit(1)
	}
}
