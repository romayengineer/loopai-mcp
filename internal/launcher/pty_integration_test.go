//go:build integration

package launcher

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

func TestSpawnEchoAndReadOutput(t *testing.T) {
	proc, err := Spawn("echo", []string{"hello from pty"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer func() {
		if err := proc.Close(); err != nil {
			t.Errorf("close: %v", err)
		}
	}()

	var buf bytes.Buffer
	_, err = io.Copy(&buf, proc)
	if err != nil && err != io.EOF {
		t.Fatalf("read PTY: %v", err)
	}

	<-proc.Wait()
	code := proc.ExitCode()
	if code != 0 {
		t.Fatalf("exit code: expected 0, got %d", code)
	}

	output := buf.String()
	if !strings.Contains(output, "hello from pty") {
		t.Fatalf("expected output to contain 'hello from pty', got: %q", output)
	}
}

func TestSpawnFailsOnMissingBinary(t *testing.T) {
	_, err := Spawn("nonexistent-binary-12345", nil)
	if err == nil {
		t.Fatal("expected error for missing binary")
	}
}

func TestSpawnEchoAndWaitExitCode(t *testing.T) {
	proc, err := Spawn("sh", []string{"-c", "echo ok && exit 42"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()

	io.Copy(io.Discard, proc)
	<-proc.Wait()

	code := proc.ExitCode()
	if code != 42 {
		t.Fatalf("exit code: expected 42, got %d", code)
	}
}

func TestSpawnWriteToPTY(t *testing.T) {
	proc, err := Spawn("sh", []string{"-c", "read -r line; echo \"you said: $line\""})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()

	if _, err := proc.Write([]byte("hello from PTY input\n")); err != nil {
		t.Fatalf("write to PTY: %v", err)
	}

	var buf bytes.Buffer
	_, err = io.Copy(&buf, proc)
	if err != nil && err != io.EOF {
		t.Fatalf("read PTY: %v", err)
	}

	<-proc.Wait()
	code := proc.ExitCode()
	if code != 0 {
		t.Fatalf("exit code: expected 0, got %d", code)
	}

	output := buf.String()
	if !strings.Contains(output, "you said: hello from PTY input") {
		t.Fatalf("expected 'you said: hello from PTY input', got: %q", output)
	}
}

func TestSpawnConcurrentReadAndWrite(t *testing.T) {
	proc, err := Spawn("sh", []string{"-c", `
		echo "ready"
		read -r line
		echo "received: $line"
	`})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()

	var once sync.Once
	done := make(chan struct{})

	go func() {
		buf := make([]byte, 4096)
		for {
			n, err := proc.Read(buf)
			if err != nil {
				break
			}
			if strings.Contains(string(buf[:n]), "ready") {
				once.Do(func() {
					if _, wErr := proc.Write([]byte("ping\n")); wErr != nil {
						t.Errorf("write: %v", wErr)
					}
				})
			}
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timeout waiting for PTY I/O")
	}

	<-proc.Wait()
	code := proc.ExitCode()
	if code != 0 {
		t.Fatalf("exit code: expected 0, got %d", code)
	}
}

func TestSpawnSignalInterrupt(t *testing.T) {
	proc, err := Spawn("sh", []string{"-c", "trap '' INT; sleep 10"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()

	time.Sleep(200 * time.Millisecond)

	if err := proc.Signal(os.Kill); err != nil {
		t.Fatalf("signal: %v", err)
	}

	select {
	case <-proc.Wait():
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for process to exit after signal")
	}
}

func TestPtyProcessResize(t *testing.T) {
	proc, err := Spawn("sh", []string{"-c", "stty -a; sleep 10"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()

	// Resize to explicit dimensions
	pp, ok := proc.(*PtyProcess)
	if !ok {
		t.Fatal("spawn did not return *PtyProcess")
	}
	if err := pp.Resize(20, 80); err != nil {
		t.Fatalf("resize: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	// Verify resize took effect by checking output
	var buf bytes.Buffer
	_, err = io.Copy(&buf, proc)
	if err != nil && err != io.EOF {
		t.Fatalf("read PTY: %v", err)
	}
	<-proc.Wait()

	output := buf.String()
	if !strings.Contains(output, "rows 20;") && !strings.Contains(output, "rows 20") {
		t.Fatalf("expected stty output to show rows 20, got: %q", output)
	}
	if !strings.Contains(output, "columns 80;") && !strings.Contains(output, "columns 80") {
		t.Fatalf("expected stty output to show columns 80, got: %q", output)
	}
}

func TestForwardSignalsDonePath(t *testing.T) {
	// The done path of forwardSignals should cause the function to exit
	// without forwarding any signal.
	cmd := exec.Command("sh", "-c", "sleep 10")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	defer cmd.Process.Kill()

	done := make(chan struct{})
	sigCh := make(chan os.Signal, 1)

	go forwardSignals(cmd, done, sigCh)

	// Close done first, then send. The done path should be taken.
	close(done)
	sigCh <- syscall.SIGUSR1

	// Process should still be running (signal was not forwarded)
	// Kill it and verify it wasn't already dead
	if err := cmd.Process.Signal(os.Kill); err != nil {
		t.Fatalf("process should still be alive after done path: %v", err)
	}
	cmd.Wait()
}

func TestForwardSignalsSignalPath(t *testing.T) {
	// The signal path should forward the signal to the process.
	cmd := exec.Command("sh", "-c", "trap 'exit 42' USR1; while true; do sleep 1; done")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start: %v", err)
	}
	// No defer Kill - the process should exit via the signal

	done := make(chan struct{})
	sigCh := make(chan os.Signal, 1)

	go forwardSignals(cmd, done, sigCh)

	// Give the shell time to install the trap before we send the signal
	time.Sleep(50 * time.Millisecond)

	sigCh <- syscall.SIGUSR1

	err := cmd.Wait()
	if exitErr, ok := err.(*exec.ExitError); ok {
		if code := exitErr.ExitCode(); code != 42 {
			t.Fatalf("expected exit code 42, got %d", code)
		}
	} else if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// err == nil means process exited with 0, not expected
	if err == nil {
		t.Fatal("expected process to exit with non-zero code from signal handler")
	}
}

func TestPtyProcessResizeClosed(t *testing.T) {
	pp, err := PtyProcessFromSpawn("echo", []string{"test"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer pp.Close()

	// Close the PTY fd directly
	pp.PTY.Close()

	err = pp.Resize(20, 80)
	if err == nil {
		t.Fatal("expected error resizing closed PTY")
	}
}

func TestPtyProcessWriteClosed(t *testing.T) {
	pp, err := PtyProcessFromSpawn("echo", []string{"test"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer pp.Close()

	pp.PTY.Close()

	_, err = pp.Write([]byte("hello"))
	if err == nil {
		t.Fatal("expected error writing to closed PTY")
	}
}

func TestPtyProcessSignalRunning(t *testing.T) {
	pp, err := PtyProcessFromSpawn("sh", []string{"-c", "trap '' USR1; while true; do sleep 1; done"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer pp.Close()

	time.Sleep(50 * time.Millisecond)

	if err := pp.Signal(syscall.SIGUSR1); err != nil {
		t.Fatalf("signal: %v", err)
	}
}

func TestPtyProcessSignalExited(t *testing.T) {
	// Signal on an already-exited process should return an error.
	proc, err := Spawn("echo", []string{"quick"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	io.Copy(io.Discard, proc)
	<-proc.Wait()

	err = proc.Signal(os.Interrupt)
	if err == nil {
		t.Fatal("expected error signaling exited process, got nil")
	}
}

// PtyProcessFromSpawn is a helper that spawns a process and returns the underlying *PtyProcess.
func PtyProcessFromSpawn(client string, args []string) (*PtyProcess, error) {
	proc, err := Spawn(client, args)
	if err != nil {
		return nil, err
	}
	pp, ok := proc.(*PtyProcess)
	if !ok {
		proc.Close()
		return nil, fmt.Errorf("Spawn did not return *PtyProcess")
	}
	return pp, nil
}

func TestPtyProcessCloseWhenDone(t *testing.T) {
	// Verify that Close doesn't panic when the process has already exited.
	proc, err := Spawn("echo", []string{"done"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	io.Copy(io.Discard, proc)
	<-proc.Wait()

	if err := proc.Close(); err != nil {
		t.Fatalf("close after exit: %v", err)
	}
	// Closing again should not panic
	if err := proc.Close(); err == nil {
		t.Log("second close returned nil (expected with already-closed PTY)")
	}
}
