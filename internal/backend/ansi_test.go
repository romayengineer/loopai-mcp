package backend

import "testing"

func eq(t *testing.T, got, want string) {
	t.Helper()
	if got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestStripANSINoCodes(t *testing.T) {
	eq(t, string(StripANSI([]byte("hello world"))), "hello world")
}

func TestStripANSIColor(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b[31mred\x1b[0m"))), "red")
}

func TestStripANSIBold(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b[1mbold\x1b[22m"))), "bold")
}

func TestStripANSICursorMove(t *testing.T) {
	eq(t, string(StripANSI([]byte("line1\x1b[1Bline2"))), "line1line2")
}

func TestStripANSIScreenClear(t *testing.T) {
	eq(t, string(StripANSI([]byte("before\x1b[2Jafter"))), "beforeafter")
}

func TestStripANSISaveRestore(t *testing.T) {
	eq(t, string(StripANSI([]byte("a\x1b7b\x1b8c"))), "abc")
}

func TestStripANSIOSCTitle(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b]0;title\x07content"))), "content")
}

func TestStripANSIOSCLong(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b]4;10;rgb:0000/0000/0000\x1b\\text"))), "text")
}

func TestStripANSIMultipleColors(t *testing.T) {
	input := "\x1b[38;5;174m╭───\x1b[39m hello \x1b[1mworld\x1b[22m"
	eq(t, string(StripANSI([]byte(input))), "╭─── hello world")
}

func TestStripANSIAlternateScreen(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b[?1049h\x1b[2J\x1b[Halt screen"))), "alt screen")
}

func TestStripANSIComplexTCI(t *testing.T) {
	// Real output from Claude Code TUI (simplified).
	input := "\x1b[?25l\x1b[?2004h\x1b[?1004h\x1b[?2031h\x1b[<u\x1b[>1uBuilding...\x1b[?25h"
	eq(t, string(StripANSI([]byte(input))), "Building...")
}

func TestStripANSIDECPrivate(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b[?25h\x1b[?1000h\x1b[?1002htext\x1b[?25l"))), "text")
}

func TestStripANSISGRReset(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b[mreset\x1b[0m"))), "reset")
}

func TestStripANSIScrollingRegion(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b[rcontent\x1b[2;20r"))), "content")
}

func TestStripANSICursorPosition(t *testing.T) {
	eq(t, string(StripANSI([]byte("\x1b[12;34Hmoved\x1b[1;1H"))), "moved")
}

func TestStripANSIDCSString(t *testing.T) {
	// Device Control String.
	eq(t, string(StripANSI([]byte("\x1bP0;1;2;4;8;16;32;64;128;255\x1b\\data"))), "data")
}

func TestStripANSISOS(t *testing.T) {
	// Start of String.
	eq(t, string(StripANSI([]byte("\x1bXthis is a string\x1b\\rest"))), "rest")
}

func TestStripANSIPM(t *testing.T) {
	// Privacy Message.
	eq(t, string(StripANSI([]byte("\x1b^private\x1b\\visible"))), "visible")
}

func TestStripANSIControlChars(t *testing.T) {
	// Control characters that are not escape sequences should pass through.
	eq(t, string(StripANSI([]byte("line1\nline2\tindented"))), "line1\nline2\tindented")
}

func TestStripANSIEmpty(t *testing.T) {
	eq(t, string(StripANSI([]byte{})), "")
}

func TestStripANSINil(t *testing.T) {
	eq(t, string(StripANSI(nil)), "")
}
