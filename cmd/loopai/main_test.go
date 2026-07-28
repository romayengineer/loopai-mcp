package main

import (
	"bytes"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func TestDefaultClientDefault(t *testing.T) {
	os.Unsetenv("LOOPAI_CLIENT")
	if c := defaultClient(); c != "claude" {
		t.Fatalf("expected 'claude', got %q", c)
	}
}

func TestDefaultClientFromEnv(t *testing.T) {
	os.Setenv("LOOPAI_CLIENT", "opencode")
	defer os.Unsetenv("LOOPAI_CLIENT")
	if c := defaultClient(); c != "opencode" {
		t.Fatalf("expected 'opencode', got %q", c)
	}
}

func TestDefaultClientEmptyEnv(t *testing.T) {
	os.Setenv("LOOPAI_CLIENT", "")
	defer os.Unsetenv("LOOPAI_CLIENT")
	if c := defaultClient(); c != "claude" {
		t.Fatalf("expected 'claude' for empty env, got %q", c)
	}
}

func TestFilterCSIuPlainText(t *testing.T) {
	var buf bytes.Buffer
	input := []byte("hello world\n")
	n, err := filterCSIu(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 12 {
		t.Fatalf("expected 12 bytes written, got %d", n)
	}
	if buf.String() != "hello world\n" {
		t.Fatalf("expected 'hello world\\n', got %q", buf.String())
	}
}

func TestFilterCSIuStripsSequence(t *testing.T) {
	var buf bytes.Buffer
	// CSI u sequence for F1: ESC [ < 3 5 ; 5 u
	// Should be stripped, only "abc" should pass through
	input := []byte("a\x1b[<35;5ubc")
	n, err := filterCSIu(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 bytes ('abc'), got %d", n)
	}
	if buf.String() != "abc" {
		t.Fatalf("expected 'abc', got %q", buf.String())
	}
}

func TestFilterCSIuMultipleSeqs(t *testing.T) {
	var buf bytes.Buffer
	// Two CSI u sequences with text between
	input := []byte("a\x1b[<1;2ub\x1b[<3;4;5uc")
	n, err := filterCSIu(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 3 {
		t.Fatalf("expected 3 bytes, got %d", n)
	}
	if buf.String() != "abc" {
		t.Fatalf("expected 'abc', got %q", buf.String())
	}
}

func TestFilterCSIuNonCSIEscape(t *testing.T) {
	var buf bytes.Buffer
	// ESC followed by non-'[' (not a CSI sequence) should pass through
	input := []byte("a\x1bXb")
	n, err := filterCSIu(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 4 {
		t.Fatalf("expected 4 bytes ('a\\x1bXb'), got %d", n)
	}
	if buf.String() != "a\x1bXb" {
		t.Fatalf("expected 'a\\x1bXb', got %q", buf.String())
	}
}

func TestFilterCSIuNonCSIuBracket(t *testing.T) {
	var buf bytes.Buffer
	// ESC [ followed by non-'<' (CSI but not CSI u) should pass through
	input := []byte("a\x1b[1mb")
	n, err := filterCSIu(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 6 {
		t.Fatalf("expected 6 bytes ('a\\x1b[1mb'), got %d", n)
	}
	if buf.String() != "a\x1b[1mb" {
		t.Fatalf("expected 'a\\x1b[1mb', got %q", buf.String())
	}
}

func TestFilterCSIuPartialAtEOF(t *testing.T) {
	var buf bytes.Buffer
	// Incomplete CSI u at EOF — should flush as-is
	input := []byte("a\x1b[<35")
	n, err := filterCSIu(&buf, bytes.NewReader(input))
	if err == nil || err.Error() != "EOF" {
		t.Fatalf("expected EOF, got %v", err)
	}
	if n != 6 {
		t.Fatalf("expected 6 bytes (incomplete seq flushed), got %d", n)
	}
	if buf.String() != "a\x1b[<35" {
		t.Fatalf("expected 'a\\x1b[<35', got %q", buf.String())
	}
}

func TestLauncherConnectBackendNotRunning(t *testing.T) {
	// Only run if the binary was compiled (e.g. after make build)
	binary := "../bin/loopai"
	if _, err := os.Stat(binary); err != nil {
		t.Skip("binary not found, run 'make build' first")
	}

	out, err := exec.Command(binary, "-socket", "/tmp/loopai-test-nonexistent-"+t.Name()+".sock").CombinedOutput()
	if err == nil {
		t.Fatal("expected error exit from launcher with no backend")
	}
	output := string(out)
	if !strings.Contains(output, "backend not running") {
		t.Fatalf("expected 'backend not running' in error output, got: %s", output)
	}
}
