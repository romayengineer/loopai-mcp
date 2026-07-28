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
	if r != ResultSuccess {
		t.Fatalf("expected ResultSuccess (no errors = success), got %s", r)
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

func TestMultiChunkCompileError(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("I'll build the project now.\n"))
	buf.Write([]byte("> go build ./...\n"))
	buf.Write([]byte("# github.com/user/repo\n"))
	buf.Write([]byte("./main.go:23:2: undefined: Foo\n"))
	result := buf.Analyze()
	if result.Phase != PhaseCompile {
		t.Fatalf("expected PhaseCompile, got %s", result.Phase)
	}
	if result.Result != ResultFailure {
		t.Fatalf("expected ResultFailure, got %s", result.Result)
	}
}

func TestMultiChunkCompileSuccessThenTestFail(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("> go build ./...\n"))
	buf.Write([]byte("")) // empty = success (no errors)
	r1 := buf.Analyze()
	if r1.Phase != PhaseCompile || r1.Result != ResultSuccess {
		t.Fatalf("compile: expected success, got %s/%s", r1.Phase, r1.Result)
	}

	buf.Reset()
	buf.Write([]byte("Now running tests.\n"))
	buf.Write([]byte("> go test ./...\n"))
	buf.Write([]byte("--- FAIL: TestAdd\n    add_test.go:10: got 4, want 5\nFAIL\n"))
	r2 := buf.Analyze()
	if r2.Phase != PhaseTest || r2.Result != ResultFailure {
		t.Fatalf("test: expected failure, got %s/%s", r2.Phase, r2.Result)
	}
}

func TestNoFalsePositiveOnNaturalLanguage(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("We need to refactor this codebase.\n"))
	buf.Write([]byte("The build is taking too long.\n"))
	buf.Write([]byte("I think we should split the package.\n"))
	result := buf.Analyze()
	if result.Phase != PhaseUnknown {
		t.Fatalf("expected PhaseUnknown for natural language, got %s", result.Phase)
	}
}

func TestLintThenTestSequence(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("> golangci-lint run ./...\n"))
	buf.Write([]byte("")) // empty = lint passed
	r1 := buf.Analyze()
	if r1.Phase != PhaseLint || r1.Result != ResultSuccess {
		t.Fatalf("lint: expected success, got %s/%s", r1.Phase, r1.Result)
	}

	buf.Reset()
	buf.Write([]byte("> go test ./...\n"))
	buf.Write([]byte("ok  github.com/user/repo\t0.234s\n"))
	r2 := buf.Analyze()
	if r2.Phase != PhaseTest || r2.Result != ResultSuccess {
		t.Fatalf("test: expected success, got %s/%s", r2.Phase, r2.Result)
	}
}

func TestToolCallFraming(t *testing.T) {
	// Simulate Claude Code's tool call output format.
	buf := NewOutputBuffer()
	buf.Write([]byte("I'll check the code first.\n"))
	buf.Write([]byte("\n"))
	buf.Write([]byte("> Tool\n"))
	buf.Write([]byte("  Reading file: main.go\n"))
	buf.Write([]byte("  Let me run go build\n"))
	buf.Write([]byte("> Bash\n"))
	buf.Write([]byte("  $ go build ./...\n"))
	buf.Write([]byte("  ./main.go:5:2: undefined: Bar\n"))
	result := buf.Analyze()
	if result.Phase == PhaseUnknown {
		t.Fatal("expected a phase to be detected even with tool call framing")
	}
}
