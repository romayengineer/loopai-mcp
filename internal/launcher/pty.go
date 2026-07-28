// Package launcher provides PTY lifecycle and I/O streaming for spawning
// TUI agent clients (Claude Code, OpenCode, etc.) in a pseudo-terminal and
// piping their output to a backend over a Unix socket.
package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/creack/pty"
)

const (
	DefaultRows     uint16 = 40
	DefaultCols     uint16 = 120
	TermEnv                = "TERM=xterm-256color"
	ptyCloseTimeout        = 5 * time.Second
)

type PtyProcess struct {
	PTY      *os.File
	Cmd      *exec.Cmd
	done     chan struct{}
	exitCode atomic.Int64
	exitErr  atomic.Value // stores error
}

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
			cmd.Process.Signal(sig)
		}
	case <-done:
	}
}

func (p *PtyProcess) Resize(rows, cols uint16) error {
	return pty.Setsize(p.PTY, &pty.Winsize{Rows: rows, Cols: cols})
}

func (p *PtyProcess) Write(data []byte) (int, error) {
	return p.PTY.Write(data)
}

func (p *PtyProcess) Read(buf []byte) (int, error) {
	return p.PTY.Read(buf)
}

func (p *PtyProcess) Signal(sig os.Signal) error {
	return p.Cmd.Process.Signal(sig)
}

func (p *PtyProcess) Wait() <-chan struct{} {
	return p.done
}

func (p *PtyProcess) ExitCode() int {
	return int(p.exitCode.Load())
}

func (p *PtyProcess) Close() error {
	closeErr := p.PTY.Close()

	// Only kill if the process hasn't already exited.
	select {
	case <-p.done:
	default:
		if p.Cmd.Process != nil {
			if err := p.Cmd.Process.Kill(); err != nil && closeErr == nil {
				closeErr = err
			}
		}
	}

	select {
	case <-p.done:
	case <-time.After(ptyCloseTimeout):
	}

	return closeErr
}
