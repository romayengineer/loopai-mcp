// Package launcher provides PTY lifecycle and I/O streaming for spawning
// TUI agent clients (Claude Code, OpenCode, etc.) in a pseudo-terminal and
// piping their output to a backend over a Unix socket.
package launcher

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"os"
	"os/exec"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	// DefaultRows is the default PTY height in rows.
	DefaultRows uint16 = 40
	// DefaultCols is the default PTY width in columns.
	DefaultCols uint16 = 120
	// TermEnv is the TERM environment variable value for the spawned client.
	TermEnv         = "TERM=xterm-256color"
	ptyCloseTimeout = 5 * time.Second
)

// PtyProcess manages a child process running inside a pseudo-terminal.
// It provides Read/Write access to the PTY, signal delivery, and exit
// code tracking via atomic fields safe for concurrent access.
type PtyProcess struct {
	PTY      *os.File
	Cmd      *exec.Cmd
	done     chan struct{}
	exitCode atomic.Int64
	exitErr  atomic.Value // stores error
}

// Spawn starts a client binary inside a PTY and returns a PtyProcess
// that can be used to interact with it.
func Spawn(client string, args []string) (*PtyProcess, error) {
	binary, err := exec.LookPath(client)
	if err != nil {
		return nil, fmt.Errorf("client %q not found on PATH: %w", client, err)
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), TermEnv)

	ptym, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: DefaultRows, Cols: DefaultCols})
	if err != nil {
		return nil, fmt.Errorf("start PTY: %w", err)
	}

	p := &PtyProcess{
		PTY:  ptym,
		Cmd:  cmd,
		done: make(chan struct{}),
	}
	p.exitCode.Store(-1)

	go func() {
		waitErr := cmd.Wait()
		if waitErr != nil {
			p.exitErr.Store(waitErr)
		}
		if cmd.ProcessState != nil {
			state := cmd.ProcessState
			if status, ok := state.Sys().(syscall.WaitStatus); ok {
				if status.Exited() {
					p.exitCode.Store(int64(status.ExitStatus()))
				} else if status.Signaled() {
					p.exitCode.Store(int64(-int(status.Signal())))
				} else {
					p.exitCode.Store(int64(state.ExitCode()))
				}
			} else {
				p.exitCode.Store(int64(state.ExitCode()))
			}
		}
		close(p.done)
	}()

	go forwardSignals(cmd, p.done)

	return p, nil
}

func forwardSignals(cmd *exec.Cmd, done <-chan struct{}) {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
	defer signal.Stop(sigCh)

	select {
	case sig := <-sigCh:
		if cmd.Process != nil {
			if err := cmd.Process.Signal(sig); err != nil {
				slog.Warn("forward signal", "signal", sig, "error", err)
			}
		}
	case <-done:
	}
}

// Resize changes the PTY terminal dimensions.
func (p *PtyProcess) Resize(rows, cols uint16) error {
	if err := pty.Setsize(p.PTY, &pty.Winsize{Rows: rows, Cols: cols}); err != nil {
		return fmt.Errorf("resize PTY: %w", err)
	}
	return nil
}

// Write data into the PTY (as if the user typed it).
func (p *PtyProcess) Write(data []byte) (int, error) {
	n, err := p.PTY.Write(data)
	if err != nil {
		return n, fmt.Errorf("write PTY: %w", err)
	}
	return n, nil
}

// Read data from the PTY output. Converts EIO (slave closed) to io.EOF.
func (p *PtyProcess) Read(buf []byte) (int, error) {
	n, err := p.PTY.Read(buf)
	if err != nil {
		if errors.Is(err, syscall.EIO) {
			return n, io.EOF
		}
	}
	return n, err
}

// Signal sends a signal to the child process.
func (p *PtyProcess) Signal(sig os.Signal) error {
	if p.Cmd.Process == nil {
		return fmt.Errorf("signal %s: process not started", sig)
	}
	if err := p.Cmd.Process.Signal(sig); err != nil {
		return fmt.Errorf("signal %s: %w", sig, err)
	}
	return nil
}

// Wait returns a channel that is closed when the child process exits.
func (p *PtyProcess) Wait() <-chan struct{} {
	return p.done
}

// ExitCode returns the process exit code, or -1 if not yet exited.
func (p *PtyProcess) ExitCode() int {
	return int(p.exitCode.Load())
}

// Close closes the PTY file descriptor and kills the process if running.
func (p *PtyProcess) Close() error {
	closeErr := p.PTY.Close()

	// Only kill if the process hasn't already exited.
	select {
	case <-p.done:
	default:
		if p.Cmd.Process != nil {
			if err := p.Cmd.Process.Kill(); err != nil && closeErr == nil {
				closeErr = fmt.Errorf("kill: %w", err)
			}
		}
	}

	select {
	case <-p.done:
	case <-time.After(ptyCloseTimeout):
	}

	if closeErr != nil {
		return fmt.Errorf("close PTY: %w", closeErr)
	}
	return nil
}
