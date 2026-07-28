package launcher

import (
	"os"
	"os/exec"
	"testing"
)

func TestPtyProcessSignalNilProcess(t *testing.T) {
	p := &PtyProcess{
		Cmd:  &exec.Cmd{},
		done: make(chan struct{}),
	}
	p.exitCode.Store(-1)

	err := p.Signal(os.Kill)
	if err == nil {
		t.Fatal("expected error when signaling a process that was never started")
	}
}
