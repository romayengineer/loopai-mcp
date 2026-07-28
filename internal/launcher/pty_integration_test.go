//go:build integration

package launcher

import (
	"bytes"
	"io"
	"os"
	"strings"
	"sync"
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
