package main

import (
	"context"
	"flag"
	"fmt"
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
	promptsDir := flag.String("prompts-dir", "prompts", "prompt template directory")
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

	// Validate prompts directory exists and is readable before starting backend
	if err := validatePromptsDir(*promptsDir); err != nil {
		slog.Error("prompts directory validation", "path", *promptsDir, "error", err)
		os.Exit(1)
	}

	if err := checkAndWritePID(*socketPath); err != nil {
		slog.Error("pid check", "error", err)
		os.Exit(1)
	}
	defer os.Remove(filepath.Join(filepath.Dir(*socketPath), "loopai.pid"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend.DefaultPromptsDir = *promptsDir
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

// validatePromptsDir checks that the prompts directory exists and is readable.
// This prevents startup failures when enforcing phases.
func validatePromptsDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("prompts directory not found: %s (create it or use -prompts-dir flag)", dir)
		}
		return fmt.Errorf("cannot access prompts directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("prompts path is not a directory: %s", dir)
	}

	// Try to read directory to verify permissions
	if _, err := os.ReadDir(dir); err != nil {
		return fmt.Errorf("cannot read prompts directory: %w", err)
	}

	slog.Debug("prompts directory validated", "path", dir)
	return nil
}
