package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"os/signal"
	"syscall"

	"github.com/creack/pty"
)

type PtyProcess struct {
	PTY      *os.File
	Cmd      *exec.Cmd
	done     chan struct{}
	exitCode int
	exitErr  error
}

func Spawn(client string, args []string) (*PtyProcess, error) {
	binary, err := exec.LookPath(client)
	if err != nil {
		return nil, fmt.Errorf("client %q not found on PATH: %w", client, err)
	}

	cmd := exec.Command(binary, args...)
	cmd.Env = append(os.Environ(), "TERM=xterm-256color")

	ptym, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		return nil, fmt.Errorf("start PTY: %w", err)
	}

	p := &PtyProcess{
		PTY:      ptym,
		Cmd:      cmd,
		done:     make(chan struct{}),
		exitCode: -1,
	}

	go func() {
		p.exitErr = cmd.Wait()
		if cmd.ProcessState != nil {
			if status, ok := cmd.ProcessState.Sys().(syscall.WaitStatus); ok {
				if status.Exited() {
					p.exitCode = status.ExitStatus()
				} else if status.Signaled() {
					p.exitCode = -int(status.Signal())
				}
			} else {
				p.exitCode = cmd.ProcessState.ExitCode()
			}
		}
		close(p.done)
	}()

	go func() {
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM, syscall.SIGHUP)
		select {
		case sig := <-sigCh:
			cmd.Process.Signal(sig)
		case <-p.done:
			signal.Stop(sigCh)
		}
	}()

	return p, nil
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
	return p.exitCode
}

func (p *PtyProcess) Close() error {
	p.PTY.Close()
	if p.Cmd.Process != nil {
		p.Cmd.Process.Kill()
	}
	<-p.done
	return p.exitErr
}
