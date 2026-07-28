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
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	var level slog.Level
	switch *logLevel {
	case "debug":
		level = slog.LevelDebug
	case "info":
		level = slog.LevelInfo
	case "warn":
		level = slog.LevelWarn
	case "error":
		level = slog.LevelError
	default:
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

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
