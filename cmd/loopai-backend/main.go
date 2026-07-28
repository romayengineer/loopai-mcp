package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/romayengineer/loopai-mcp/internal/backend"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func main() {
	socketPath := flag.String("socket", proto.DefaultSocketPath(), "unix socket path")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig)
		cancel()
	}()

	b := backend.New(*socketPath, backend.HandleLauncher)
	if err := b.Run(ctx); err != nil {
		slog.Error("backend", "error", err)
		os.Exit(1)
	}
}
