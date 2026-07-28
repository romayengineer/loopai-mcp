package main

import (
	"os"
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
