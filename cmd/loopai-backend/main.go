package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/backend"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func checkAndWritePID(socketPath string) error {
	socketDir := filepath.Dir(socketPath)
	pidPath := filepath.Join(socketDir, "loopai.pid")

	data, err := os.ReadFile(pidPath)
	if err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil && pid > 0 {
			proc, findErr := os.FindProcess(pid)
			if findErr == nil {
				// Signal 0 checks if the process is alive without sending a signal.
				if proc.Signal(syscall.Signal(0)) == nil {
					return fmt.Errorf("backend already running on PID %d (%s)", pid, pidPath)
				}
			}
		}
		// Stale PID file — remove it.
		os.Remove(pidPath)
	}

	// Atomic write: temp file + rename
	tmpPath := pidPath + ".tmp"
	if err := os.WriteFile(tmpPath, []byte(fmt.Sprintf("%d\n", os.Getpid())), 0644); err != nil {
		return fmt.Errorf("write pid tmp: %w", err)
	}
	if err := os.Rename(tmpPath, pidPath); err != nil {
		os.Remove(tmpPath)
		return fmt.Errorf("rename pid file: %w", err)
	}
	return nil
}

func stopBackend(socketPath string) error {
	socketDir := filepath.Dir(socketPath)
	pidPath := filepath.Join(socketDir, "loopai.pid")

	data, err := os.ReadFile(pidPath)
	if err != nil {
		return fmt.Errorf("no running backend found at %s: %w", pidPath, err)
	}

	pid, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return fmt.Errorf("invalid pid file %s: %w", pidPath, err)
	}

	proc, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("process %d not found: %w", pid, err)
	}

	if err := proc.Signal(syscall.SIGTERM); err != nil {
		return fmt.Errorf("stop PID %d: %w", pid, err)
	}

	// Wait for the pid file to disappear. The backend's defer os.Remove(pidPath)
	// runs after signal handler cancels the context + stops the listener.
	for i := 0; i < 30; i++ {
		if _, err := os.Stat(pidPath); os.IsNotExist(err) {
			slog.Info("backend stopped", "pid", pid)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("PID %d did not stop within 3s (check %s)", pid, socketPath)
}

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
