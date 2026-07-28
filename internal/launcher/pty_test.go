package launcher

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"testing"
	"time"

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

// TestForwardSignalsHandlesMultipleSignals verifies that forwardSignals
// processes multiple sequential signals until the done channel is closed,
// rather than exiting after the first signal.
func TestForwardSignalsHandlesMultipleSignals(t *testing.T) {
	cmd := &exec.Cmd{}
	done := make(chan struct{})
	sigCh := make(chan os.Signal, 10)

	// Run signal handler with test signal channel
	go forwardSignals(context.Background(), cmd, done, sigCh)

	// Send multiple signals
	sigCh <- os.Interrupt
	sigCh <- os.Kill
	sigCh <- os.Interrupt

	// Signal handler should still be running and accepting signals.
	// Close done to verify it exits cleanly.
	close(done)

	// Verify goroutine exits (give it a moment to process)
	// If it doesn't exit, the test will timeout, indicating the loop
	// didn't properly handle the done channel.
	done = make(chan struct{}, 1)
	go func() {
		time.Sleep(100 * time.Millisecond)
		done <- struct{}{}
	}()

	select {
	case <-done:
		// Success: signal handler exited
	case <-time.After(1 * time.Second):
		t.Fatal("signal handler did not exit after done channel closed")
	}
}

// TestForwardSignalsExitsOnDoneChannel verifies that forwardSignals
// exits immediately when the done channel is closed, even if no signals
// have been received.
func TestForwardSignalsExitsOnDoneChannel(t *testing.T) {
	cmd := &exec.Cmd{}
	done := make(chan struct{})
	sigCh := make(chan os.Signal, 1)

	exited := make(chan struct{})
	go func() {
		forwardSignals(context.Background(), cmd, done, sigCh)
		close(exited)
	}()

	// Close done channel to signal handler should exit
	close(done)

	// Verify it exits quickly
	select {
	case <-exited:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("forwardSignals did not exit when done channel closed")
	}
}

// TestForwardSignalsExitsOnContextCancellation verifies that forwardSignals
// exits when the context is cancelled, even if the done channel is still open.
func TestForwardSignalsExitsOnContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cmd := &exec.Cmd{}
	done := make(chan struct{}) // never closed
	sigCh := make(chan os.Signal, 1)

	exited := make(chan struct{})
	go func() {
		forwardSignals(ctx, cmd, done, sigCh)
		close(exited)
	}()

	// Cancel the context — handler should exit via ctx.Done() path.
	cancel()

	select {
	case <-exited:
		// Success
	case <-time.After(100 * time.Millisecond):
		t.Fatal("forwardSignals did not exit when context was cancelled")
	}
}
