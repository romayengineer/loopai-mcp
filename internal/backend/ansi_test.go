package backend

import (
	"testing"
)

func TestStripANSIEmpty(t *testing.T) {
	got := StripANSI(nil)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %v", got)
	}
}

func TestStripANSIPlainText(t *testing.T) {
	in := []byte("hello world")
	got := StripANSI(in)
	if string(got) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", got)
	}
}

func TestStripANSICSI(t *testing.T) {
	in := []byte("\x1b[31mred\x1b[0m")
	got := StripANSI(in)
	if string(got) != "red" {
		t.Fatalf("expected 'red', got %q", got)
	}
}

func TestStripANSIMultipleCSIs(t *testing.T) {
	in := []byte("\x1b[1m\x1b[32mbold green\x1b[0m")
	got := StripANSI(in)
	if string(got) != "bold green" {
		t.Fatalf("expected 'bold green', got %q", got)
	}
}

func TestStripANSICSIParametric(t *testing.T) {
	in := []byte("\x1b[38;5;196mtruecolor\x1b[0m")
	got := StripANSI(in)
	if string(got) != "truecolor" {
		t.Fatalf("expected 'truecolor', got %q", got)
	}
}

func TestStripANSIOSC(t *testing.T) {
	in := []byte("\x1b]0;title\x07visible")
	got := StripANSI(in)
	if string(got) != "visible" {
		t.Fatalf("expected 'visible', got %q", got)
	}
}

func TestStripANSIESCBackslash(t *testing.T) {
	in := []byte("\x1b]0;title\x1b\\visible")
	got := StripANSI(in)
	if string(got) != "visible" {
		t.Fatalf("expected 'visible', got %q", got)
	}
}

func TestStripANSIIncompleteSequence(t *testing.T) {
	in := []byte("text\x1b[31m")
	got := StripANSI(in)
	// Incomplete sequence at end: CSI with no final byte is consumed (fallback path).
	// The ESC is consumed, so only "text" remains.
	if string(got) != "text" {
		t.Fatalf("expected 'text', got %q", got)
	}
}

func TestStripANSIMixed(t *testing.T) {
	in := []byte("line1\x1b[K\nline2\x1b[1D\x1b[J")
	got := StripANSI(in)
	if string(got) != "line1\nline2" {
		t.Fatalf("expected 'line1\\nline2', got %q", got)
	}
}

func BenchmarkStripANSI(b *testing.B) {
	data := []byte("\x1b[31mhello \x1b[1mworld\x1b[0m\n\x1b[32mgreen\x1b[0m")
	for i := 0; i < b.N; i++ {
		StripANSI(data)
	}
}

func BenchmarkStripANSIComplex(b *testing.B) {
	data := []byte("\x1b[38;5;196m\x1b[1mbold red\x1b[0m \x1b]0;title\x07visible\x1b[K\n\x1b[2J")
	for i := 0; i < b.N; i++ {
		StripANSI(data)
	}
}
