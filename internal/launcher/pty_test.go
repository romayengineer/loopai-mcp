package launcher

import (
	"fmt"
	"os"
	"os/exec"
	"testing"

	"github.com/creack/pty"
	"golang.org/x/sys/unix"
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

func TestDisablePTYEcho(t *testing.T) {
	// Open a real PTY to test the ECHO disable function.
	ptm, _, err := pty.Open()
	if err != nil {
		t.Fatalf("open PTY: %v", err)
	}
	defer ptm.Close()

	// Get the slave name so we can check ECHO on the slave directly
	n, err := unix.IoctlGetInt(int(ptm.Fd()), unix.TIOCGPTN)
	if err != nil {
		t.Fatalf("TIOCGPTN: %v", err)
	}
	slaveName := fmt.Sprintf("/dev/pts/%d", n)
	slave, err := os.OpenFile(slaveName, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open slave: %v", err)
	}
	defer slave.Close()

	// Verify ECHO is initially enabled on the slave
	termios, err := unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatalf("get termios: %v", err)
	}
	if termios.Lflag&unix.ECHO == 0 {
		t.Fatal("expected ECHO to be enabled by default")
	}

	// Disable ECHO via the master
	if err := DisablePTYEcho(ptm); err != nil {
		t.Fatalf("DisablePTYEcho: %v", err)
	}

	// Verify ECHO is now disabled on the slave
	termios, err = unix.IoctlGetTermios(int(slave.Fd()), unix.TCGETS)
	if err != nil {
		t.Fatalf("get termios after disable: %v", err)
	}
	if termios.Lflag&unix.ECHO != 0 {
		t.Fatal("expected ECHO to be disabled after DisablePTYEcho")
	}
}

func TestPtyProcessNilClose(t *testing.T) {
	p := newNilPtyProcess()
	err := p.Close()
	if err == nil {
		t.Fatal("expected error closing nil PTY")
	}
}
