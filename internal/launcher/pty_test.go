package launcher

import (
	"os"
	"os/exec"
	"testing"
)

func newNilPtyProcess() *PtyProcess {
	p := &PtyProcess{
		Cmd:  &exec.Cmd{},
		done: make(chan struct{}),
	}
	p.exitCode.Store(-1)
	return p
}

func TestPtyProcessSignalNilProcess(t *testing.T) {
	p := newNilPtyProcess()
	err := p.Signal(os.Kill)
	if err == nil {
		t.Fatal("expected error when signaling a process that was never started")
	}
}

func TestPtyProcessPIDNilCmd(t *testing.T) {
	var p PtyProcess
	if pid := p.PID(); pid != -1 {
		t.Fatalf("expected -1 for nil Cmd, got %d", pid)
	}
}

func TestPtyProcessPIDNilProcess(t *testing.T) {
	p := PtyProcess{Cmd: &exec.Cmd{}}
	if pid := p.PID(); pid != -1 {
		t.Fatalf("expected -1 for nil Process, got %d", pid)
	}
}

func TestPtyProcessExitCodeDefaultsToMinusOne(t *testing.T) {
	p := newNilPtyProcess()
	if code := p.ExitCode(); code != -1 {
		t.Fatalf("expected -1 for unset exit code, got %d", code)
	}
}

func TestPtyProcessNilWaitDone(t *testing.T) {
	p := newNilPtyProcess()
	done := p.Wait()
	if done == nil {
		t.Fatal("Wait returned nil channel")
	}
}

func TestPtyProcessNilClose(t *testing.T) {
	p := newNilPtyProcess()
	err := p.Close()
	if err == nil {
		t.Fatal("expected error closing nil PTY")
	}
}
