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
	"golang.org/x/sys/unix"
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

// DisablePTYEcho disables local ECHO on the PTY master. This prevents the
// PTY from echoing keystrokes back to the user when running in interactive
// mode (-interactive flag). Without this, every keystroke forwarded from
// stdin → PTY gets echoed back through the PTY output as raw escape
// sequences (e.g. ^[[<35u) and appears on the user's terminal alongside
// the client's TUI output.
//
// The PTY starts with ECHO enabled by default (matching real terminal
// behavior). Interactive mode forwards raw keystrokes to the PTY, but the
// terminal emulator on the user's side already echoes what the user types.
// Disabling ECHO on the PTY eliminates the double-echo and the raw escape
// artifacts while still letting the client receive all keystrokes normally.
func DisablePTYEcho(ptm *os.File) error {
	// ECHO is a slave-side terminal flag. We need to open the slave
	// device and clear ECHO on its termios. Modifying the master's
	// termios has no effect on echo behavior.
	n, err := unix.IoctlGetInt(int(ptm.Fd()), unix.TIOCGPTN)
	if err != nil {
		return fmt.Errorf("get pty number: %w", err)
	}
	slaveName := fmt.Sprintf("/dev/pts/%d", n)
	slave, err := os.OpenFile(slaveName, os.O_RDWR, 0)
	if err != nil {
		return fmt.Errorf("open slave %s: %w", slaveName, err)
	}
	defer slave.Close()

	termios, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		return fmt.Errorf("get slave termios: %w", err)
	}
	termios.Lflag &^= unix.ECHO
	if err := unix.IoctlSetTermios(int(slave.Fd()), unix.TCSETS, termios); err != nil {
		return fmt.Errorf("set slave termios: %w", err)
	}
	return nil
}

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
		close(p.done)
	}()

	go forwardSignals(cmd, p.done)

	return p, nil
}

func forwardSignals(cmd *exec.Cmd, done <-chan struct{}, sigCh ...chan os.Signal) {
	ch := make(chan os.Signal, 1)
	if len(sigCh) > 0 && sigCh[0] != nil {
		ch = sigCh[0]
	} else {
		signal.Notify(ch, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		defer signal.Stop(ch)
	}

	select {
	case sig := <-ch:
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

// PID returns the process ID of the spawned child.
func (p *PtyProcess) PID() int {
	if p.Cmd != nil && p.Cmd.Process != nil {
		return p.Cmd.Process.Pid
	}
	return -1
}

// DisablePTYEcho disables terminal echo on this PTY. See the package-level
// DisablePTYEcho function for details on why this is needed.
func (p *PtyProcess) DisablePTYEcho() error {
	return DisablePTYEcho(p.PTY)
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
