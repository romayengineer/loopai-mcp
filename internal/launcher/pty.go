// Package launcher provides PTY lifecycle and I/O streaming for spawning
// TUI agent clients (Claude Code, OpenCode, etc.) in a pseudo-terminal and
// piping their output to a backend over a Unix socket.
package launcher

import (
	"context"
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
	TermEnv          = "TERM=xterm-256color"
	ptyCloseTimeout  = 5 * time.Second
	gracefulKillWait = 3 * time.Second
)

// Process is the interface for a PTY-based child process. It decouples
// consumers from the concrete PtyProcess, enabling alternative implementations
// for testing.
type Process interface {
	io.ReadWriteCloser
	Signal(os.Signal) error
	Wait() <-chan struct{}
	ExitCode() int
	PID() int
	// DisablePTYEcho disables terminal echo on the PTY. Needed in interactive
	// mode to prevent keystroke echo from producing raw escape sequences
	// on the user's terminal. See DisablePTYEcho docs for details.
	DisablePTYEcho() error
}

// PtyProcess manages a child process running inside a pseudo-terminal.
// It provides Read/Write access to the PTY, signal delivery, and exit
// code tracking via atomic fields safe for concurrent access.
type PtyProcess struct {
	PTY      *os.File
	Cmd      *exec.Cmd
	done     chan struct{}
	exitCode atomic.Int64
}

// Spawn starts a client binary inside a PTY and returns a Process
// that can be used to interact with it.
func Spawn(client string, args []string) (Process, error) {
	return spawn(context.Background(), client, args)
}

// SpawnContext starts a client binary inside a PTY with context support.
// The context can be used to cancel the launch. If the context is cancelled
// before the process starts, the PTY is cleaned up.
func SpawnContext(ctx context.Context, client string, args []string) (Process, error) {
	return spawn(ctx, client, args)
}

func spawn(ctx context.Context, client string, args []string) (Process, error) {
	binary, err := exec.LookPath(client)
	if err != nil {
		return nil, fmt.Errorf("client %q not found on PATH: %w", client, err)
	}

	cmd := exec.CommandContext(ctx, binary, args...)
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
		if err := cmd.Wait(); err != nil {
			slog.Debug("process wait", "error", err)
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
		slog.Debug("process exited", "pid", p.PID(), "exit_code", p.exitCode.Load())
		close(p.done)
	}()

	go forwardSignals(ctx, cmd, p.done)

	slog.Info("client spawned", "client", client, "pid", p.PID())
	return p, nil
}

// forwardSignals forwards signals from the parent process to the child process.
// It handles multiple sequential signals (SIGINT, SIGTERM, SIGHUP) by looping
// until the process exits, the context is cancelled, or the done channel closes.
// Accepts optional pre-configured signal channel for testing.
func forwardSignals(ctx context.Context, cmd *exec.Cmd, done <-chan struct{}, sigCh ...chan os.Signal) {
	ch := make(chan os.Signal, 1)
	if len(sigCh) > 0 && sigCh[0] != nil {
		ch = sigCh[0]
	} else {
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		defer signal.Stop(ch)
	}

	for {
		select {
		case sig := <-ch:
			if cmd.Process != nil {
				slog.Debug("forwarding signal", "signal", sig, "pid", cmd.Process.Pid)
				if err := cmd.Process.Signal(sig); err != nil {
					slog.Warn("forward signal", "signal", sig, "error", err)
				}
			} else {
				slog.Debug("signal before process start", "signal", sig)
			}
		case <-done:
			slog.Debug("process exited, stopping signal handler")
			return
		case <-ctx.Done():
			slog.Debug("context cancelled, stopping signal handler")
			return
		}
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

// PID returns the process ID of the spawned child.
func (p *PtyProcess) PID() int {
	if p.Cmd != nil && p.Cmd.Process != nil {
		return p.Cmd.Process.Pid
	}
	return -1
}

// Close closes the PTY file descriptor and terminates the child process.
// It tries SIGTERM first for a graceful shutdown, then escalates to SIGKILL
// after gracefulKillWait if the process hasn't exited. This minimizes the
// risk of orphaned or misbehaving child processes.
func (p *PtyProcess) Close() error {
	closeErr := p.PTY.Close()

	// Check if the process has already exited.
	select {
	case <-p.done:
		// Already exited — nothing more to do.
		if closeErr != nil {
			return fmt.Errorf("close PTY: %w", closeErr)
		}
		return nil
	default:
	}

	// Process is still running. Try graceful SIGTERM first.
	if p.Cmd.Process != nil {
		slog.Debug("sending SIGTERM to process", "pid", p.PID())
		if err := p.Cmd.Process.Signal(syscall.SIGTERM); err != nil {
			slog.Warn("signal SIGTERM", "pid", p.PID(), "error", err)
			// If SIGTERM fails, fall through to SIGKILL.
		} else {
			// Wait briefly for graceful shutdown.
			select {
			case <-p.done:
				slog.Debug("process exited after SIGTERM", "pid", p.PID())
				if closeErr != nil {
					return fmt.Errorf("close PTY: %w", closeErr)
				}
				return nil
			case <-time.After(gracefulKillWait):
				slog.Debug("process did not exit after SIGTERM, sending SIGKILL", "pid", p.PID())
			}
		}
	}

	// Escalate to SIGKILL.
	if p.Cmd.Process != nil {
		if err := p.Cmd.Process.Kill(); err != nil && closeErr == nil {
			closeErr = fmt.Errorf("kill: %w", err)
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
