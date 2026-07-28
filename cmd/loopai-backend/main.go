package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/romayengineer/loopai-mcp/internal/backend"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func main() {
	socketPath := flag.String("socket", proto.DefaultSocketPath(), "unix socket path")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	if flag.NArg() > 0 && flag.Arg(0) == "stop" {
		if err := stopBackend(*socketPath); err != nil {
			slog.Error("stop", "error", err)
			os.Exit(1)
		}
		return
	}

	if err := checkAndWritePID(*socketPath); err != nil {
		slog.Error("pid check", "error", err)
		os.Exit(1)
	}
	defer os.Remove(filepath.Join(filepath.Dir(*socketPath), "loopai.pid"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	b := backend.New(*socketPath, backend.HandleLauncher)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig)
		cancel()
		b.Stop()
	}()

	if err := b.Run(ctx); err != nil {
		slog.Error("backend", "error", err)
		os.Exit(1)
	}
}
