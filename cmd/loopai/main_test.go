package main

import (
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
