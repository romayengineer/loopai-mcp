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

func TestFilterCSIPlainText(t *testing.T) {
	var buf bytes.Buffer
	input := []byte("hello world\n")
	n, err := filterCSI(&buf, bytes.NewReader(input))
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

func TestFilterCSIStripsSequence(t *testing.T) {
	var buf bytes.Buffer
	// CSI u sequence for F1: ESC [ < 3 5 ; 5 u
	// Should be stripped, only "abc" should pass through
	input := []byte("a\x1b[<35;5ubc")
	n, err := filterCSI(&buf, bytes.NewReader(input))
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

func TestFilterCSIMultipleSeqs(t *testing.T) {
	var buf bytes.Buffer
	// Two CSI u sequences with text between
	input := []byte("a\x1b[<1;2ub\x1b[<3;4;5uc")
	n, err := filterCSI(&buf, bytes.NewReader(input))
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

func TestFilterCSINonCSIEscape(t *testing.T) {
	var buf bytes.Buffer
	// ESC followed by non-'[' (not a CSI sequence) should pass through
	input := []byte("a\x1bXb")
	n, err := filterCSI(&buf, bytes.NewReader(input))
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

func TestFilterCSINonCSIuBracket(t *testing.T) {
	var buf bytes.Buffer
	// CSI sequence ESC [ 1 m should be stripped (final byte 'm' is 0x40-0x7E)
	input := []byte("a\x1b[1mb")
	n, err := filterCSI(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 bytes ('ab'), got %d", n)
	}
	if buf.String() != "ab" {
		t.Fatalf("expected 'ab', got %q", buf.String())
	}
}

func TestFilterCSIArrowKeys(t *testing.T) {
	var buf bytes.Buffer
	// Arrow up/down/right/left
	input := []byte("a\x1b[A\x1b[B\x1b[C\x1b[Dx")
	n, err := filterCSI(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 bytes ('ax'), got %d", n)
	}
	if buf.String() != "ax" {
		t.Fatalf("expected 'ax', got %q", buf.String())
	}
}

func TestFilterCSIModifiedArrow(t *testing.T) {
	var buf bytes.Buffer
	// Ctrl+right = ESC [ 1 ; 5 C (CSI with params before final byte)
	input := []byte("a\x1b[1;5Cb")
	n, err := filterCSI(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 bytes ('ab'), got %d", n)
	}
	if buf.String() != "ab" {
		t.Fatalf("expected 'ab', got %q", buf.String())
	}
}

func TestFilterCSSS3(t *testing.T) {
	var buf bytes.Buffer
	// SS3 F1-F4 = ESC O P/Q/R/S
	input := []byte("a\x1bOP\x1bOQ\x1bOR\x1bOSb")
	n, err := filterCSI(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 bytes ('ab'), got %d", n)
	}
	if buf.String() != "ab" {
		t.Fatalf("expected 'ab', got %q", buf.String())
	}
}

func TestFilterCSIPartialAtEOF(t *testing.T) {
	var buf bytes.Buffer
	// Incomplete CSI u at EOF — should flush as-is
	input := []byte("a\x1b[<35")
	n, err := filterCSI(&buf, bytes.NewReader(input))
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

func TestFilterCSIHomeEnd(t *testing.T) {
	var buf bytes.Buffer
	// Home = ESC [ H or ESC [ 1 ~, End = ESC [ F or ESC [ 4 ~
	input := []byte("a\x1b[H\x1b[Fb")
	n, err := filterCSI(&buf, bytes.NewReader(input))
	if err != nil && err.Error() != "EOF" {
		t.Fatalf("unexpected error: %v", err)
	}
	if n != 2 {
		t.Fatalf("expected 2 bytes ('ab'), got %d", n)
	}
	if buf.String() != "ab" {
		t.Fatalf("expected 'ab', got %q", buf.String())
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
