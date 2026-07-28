package backend

import (
	"testing"
)

func TestDetectGoBuildTrigger(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("> go build ./...\n"))
	if p := buf.CurrentPhase(); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile, got %s", p)
	}
}

func TestDetectGoTestTrigger(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("> go test ./...\n"))
	if p := buf.CurrentPhase(); p != PhaseTest {
		t.Fatalf("expected PhaseTest, got %s", p)
	}
}

func TestDetectLintTrigger(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("> golangci-lint run ./...\n"))
	if p := buf.CurrentPhase(); p != PhaseLint {
		t.Fatalf("expected PhaseLint, got %s", p)
	}
}

func TestNoTriggerOnNormalOutput(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("I think we should refactor the auth module\n"))
	if p := buf.CurrentPhase(); p != PhaseUnknown {
		t.Fatalf("expected PhaseUnknown, got %s", p)
	}
}

func TestAnalyzeGoBuildError(t *testing.T) {
	out := `# github.com/user/repo/pkg
./main.go:23:2: undefined: Foo
./main.go:25:9: cannot use Bar (type string) as type int`
	r := analyzeOutput(out, PhaseCompile)
	if r != ResultFailure {
		t.Fatalf("expected ResultFailure, got %s", r)
	}
}

func TestAnalyzeGoTestPass(t *testing.T) {
	out := "ok  github.com/pkg/foo\t0.234s\n"
	r := analyzeOutput(out, PhaseTest)
	if r != ResultSuccess {
		t.Fatalf("expected ResultSuccess, got %s", r)
	}
}

func TestAnalyzeGoTestFail(t *testing.T) {
	out := `--- FAIL: TestFoo
    foo_test.go:10: expected 3, got 5
FAIL`
	r := analyzeOutput(out, PhaseTest)
	if r != ResultFailure {
		t.Fatalf("expected ResultFailure, got %s", r)
	}
}

func TestAnalyzeGoTestRace(t *testing.T) {
	out := `WARNING: DATA RACE
Write at 0x123 by goroutine 5:`
	r := analyzeOutput(out, PhaseTest)
	if r != ResultFailure {
		t.Fatalf("expected ResultFailure, got %s", r)
	}
}

func TestAnalyzeEmptyCompileOutput(t *testing.T) {
	r := analyzeOutput("", PhaseCompile)
	if r != ResultUnknown {
		t.Fatalf("expected ResultUnknown, got %s", r)
	}
}

func TestAnalyzeUnknownPhase(t *testing.T) {
	r := analyzeOutput("some random output", PhaseUnknown)
	if r != ResultUnknown {
		t.Fatalf("expected ResultUnknown, got %s", r)
	}
}

func TestOutputBufferAccumulates(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("hello "))
	buf.Write([]byte("world"))
	if s := buf.String(); s != "hello world" {
		t.Fatalf("expected 'hello world', got %q", s)
	}
}

func TestOutputBufferReset(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("> go build"))
	buf.Reset()
	if s := buf.String(); s != "" {
		t.Fatalf("expected empty, got %q", s)
	}
	if p := buf.CurrentPhase(); p != PhaseUnknown {
		t.Fatalf("expected PhaseUnknown after reset, got %s", p)
	}
}

func TestAnalyzeGoVetError(t *testing.T) {
	out := `./handler.go:42:2: unreachable code`
	r := analyzeOutput(out, PhaseCompile)
	if r != ResultFailure {
		t.Fatalf("expected ResultFailure, got %s", r)
	}
}

func TestAnalyzeGoLintError(t *testing.T) {
	out := `main.go:23:2: unused: variable x is unused`
	r := analyzeOutput(out, PhaseLint)
	if r != ResultFailure {
		t.Fatalf("expected ResultFailure, got %s", r)
	}
}

func TestOutputBufferKeepsLatestPhase(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("> go build"))
	buf.Write([]byte("normal text"))
	buf.Write([]byte("> go test"))
	if p := buf.CurrentPhase(); p != PhaseTest {
		t.Fatalf("expected PhaseTest (latest trigger), got %s", p)
	}
}
