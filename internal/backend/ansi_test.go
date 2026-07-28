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

func TestStripANSIDCS(t *testing.T) {
	// DCS: ESC P ... ST(ESC \)
	in := []byte("before\x1bP12345678\x1b\\after")
	got := StripANSI(in)
	if string(got) != "beforeafter" {
		t.Fatalf("expected 'beforeafter', got %q", got)
	}
}

func TestStripANSISOS(t *testing.T) {
	// SOS: ESC X ... ST(ESC \)
	in := []byte("before\x1bXsos data\x1b\\after")
	got := StripANSI(in)
	if string(got) != "beforeafter" {
		t.Fatalf("expected 'beforeafter', got %q", got)
	}
}

func TestStripANSIPM(t *testing.T) {
	// PM: ESC ^ ... ST(ESC \)
	in := []byte("before\x1b^pm data\x1b\\after")
	got := StripANSI(in)
	if string(got) != "beforeafter" {
		t.Fatalf("expected 'beforeafter', got %q", got)
	}
}

func TestStripANSIAPC(t *testing.T) {
	// APC: ESC _ ... ST(ESC \)
	in := []byte("before\x1b_apc data\x1b\\after")
	got := StripANSI(in)
	if string(got) != "beforeafter" {
		t.Fatalf("expected 'beforeafter', got %q", got)
	}
}

func TestStripANSITwoCharEscape(t *testing.T) {
	// Two-character escapes: ESC 7 (save cursor), ESC 8 (restore cursor)
	in := []byte("a\x1b7b\x1b8c")
	got := StripANSI(in)
	if string(got) != "abc" {
		t.Fatalf("expected 'abc', got %q", got)
	}
}

func TestStripANSISingleCharEscape(t *testing.T) {
	// Single-character escapes: ESC D (index), ESC M (reverse index), ESC c (reset)
	in := []byte("a\x1bDb\x1bMc\x1bcd")
	got := StripANSI(in)
	if string(got) != "abcd" {
		t.Fatalf("expected 'abcd', got %q", got)
	}
}

func TestStripANSIIncompleteDCS(t *testing.T) {
	// DCS without ST terminator at end of data
	in := []byte("before\x1bPunfinished")
	got := StripANSI(in)
	if string(got) != "before" {
		t.Fatalf("expected 'before', got %q", got)
	}
}

func FuzzStripANSI(f *testing.F) {
	f.Add([]byte("hello"))
	f.Add([]byte("\x1b[31mred\x1b[0m"))
	f.Add([]byte("\x1b[38;5;196m\x1b[1mbold\x1b[0m"))
	f.Add([]byte("line1\x1b[K\nline2\x1b[J"))
	f.Fuzz(func(t *testing.T, data []byte) {
		result := StripANSI(data)
		// Must never panic
		// Must never produce more bytes than input
		if len(result) > len(data) {
			t.Fatalf("result len %d > input len %d", len(result), len(data))
		}
		// Result must not contain ESC byte
		for _, b := range result {
			if b == 0x1B {
				t.Fatal("result contains ESC byte")
			}
		}
	})
}

func TestStripANSIOnlyEscape(t *testing.T) {
	// Input that is ONLY escape sequences should produce empty output
	in := []byte("\x1b[31m\x1b[1m\x1b[0m")
	got := StripANSI(in)
	if len(got) != 0 {
		t.Fatalf("expected empty, got %q", got)
	}
}
