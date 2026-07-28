package main

import (
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// checkAndWritePID checks for an existing backend PID file. If the PID is
// alive, it returns an error. If the PID is stale, it removes the stale file.
// On success, it atomically writes the current process PID to the file.
func checkAndWritePID(socketPath string) error {
	socketDir := filepath.Dir(socketPath)
	pidPath := filepath.Join(socketDir, "loopai.pid")

	data, err := os.ReadFile(pidPath)
	if err == nil {
		pid, parseErr := strconv.Atoi(strings.TrimSpace(string(data)))
		if parseErr == nil && pid > 0 {
			proc, findErr := os.FindProcess(pid)
			if findErr == nil {
				if proc.Signal(syscall.Signal(0)) == nil {
					return fmt.Errorf("backend already running on PID %d (%s)", pid, pidPath)
				}
			}
		}
		os.Remove(pidPath)
	}

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

// stopBackend reads the PID file, sends SIGTERM to the backend process, and
// waits for the PID file to disappear (the backend's defer removes it on exit).
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

	for i := 0; i < 30; i++ {
		if _, err := os.Stat(pidPath); os.IsNotExist(err) {
			slog.Info("backend stopped", "pid", pid)
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("PID %d did not stop within 3s (check %s)", pid, socketPath)
}
