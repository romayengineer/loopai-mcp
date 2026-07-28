package backend

import (
	"strings"
	"testing"
)

// TestPhaseDetectionOnCleanedData verifies that phase triggers are detected
// on ANSI-stripped data, ensuring consistency with buffer analysis.
// This test ensures that ANSI codes don't break phase trigger detection.
func TestPhaseDetectionOnCleanedData(t *testing.T) {
	tests := []struct {
		name     string
		data     string
		expected Phase
	}{
		{
			name:     "clean trigger",
			data:     "> go build ./...\n",
			expected: PhaseCompile,
		},
		{
			name:     "with ANSI color codes",
			data:     "\x1b[32m> go build ./...\x1b[0m\n",
			expected: PhaseCompile,
		},
		{
			name:     "with mixed ANSI sequences",
			data:     "\x1b[1m>\x1b[0m \x1b[36mgo build\x1b[0m ./...\n",
			expected: PhaseCompile,
		},
		{
			name:     "go test with ANSI",
			data:     "\x1b[32m> go test ./...\x1b[0m\n",
			expected: PhaseTest,
		},
		{
			name:     "golangci-lint with ANSI",
			data:     "\x1b[32m> golangci-lint run ./...\x1b[0m\n",
			expected: PhaseLint,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			buf := NewOutputBuffer()
			buf.Write([]byte(tt.data))
			if p := buf.CurrentPhase(); p != tt.expected {
				t.Errorf("expected phase %s, got %s", tt.expected, p)
			}
		})
	}
}

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

func TestAnalyzeOutput(t *testing.T) {
	tests := []struct {
		name  string
		ouput string
		phase Phase
		want  PhaseResult
	}{
		{
			name:  "compile error",
			ouput: "# github.com/user/repo/pkg\n./main.go:23:2: undefined: Foo\n./main.go:25:9: cannot use Bar (type string) as type int",
			phase: PhaseCompile,
			want:  ResultFailure,
		},
		{
			name:  "compile vet error",
			ouput: "./handler.go:42:2: unreachable code",
			phase: PhaseCompile,
			want:  ResultFailure,
		},
		{
			name:  "compile empty output success",
			ouput: "",
			phase: PhaseCompile,
			want:  ResultSuccess,
		},
		{
			name:  "test pass",
			ouput: "ok  github.com/pkg/foo\t0.234s\n",
			phase: PhaseTest,
			want:  ResultSuccess,
		},
		{
			name:  "test fail",
			ouput: "--- FAIL: TestFoo\n    foo_test.go:10: expected 3, got 5\nFAIL",
			phase: PhaseTest,
			want:  ResultFailure,
		},
		{
			name:  "test data race",
			ouput: "WARNING: DATA RACE\nWrite at 0x123 by goroutine 5:",
			phase: PhaseTest,
			want:  ResultFailure,
		},
		{
			name:  "lint error",
			ouput: "main.go:23:2: unused: variable x is unused",
			phase: PhaseLint,
			want:  ResultFailure,
		},
		{
			name:  "unknown phase returns unknown",
			ouput: "some random output",
			phase: PhaseUnknown,
			want:  ResultUnknown,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := analyzeOutput(tt.ouput, tt.phase)
			if got != tt.want {
				t.Fatalf("analyzeOutput(phase=%s): got %s, want %s", tt.phase, got, tt.want)
			}
		})
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

func TestOutputBufferKeepsFirstPhase(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("> go build"))
	buf.Write([]byte("normal text"))
	buf.Write([]byte("> go test"))
	if p := buf.CurrentPhase(); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile (first trigger), got %s", p)
	}
}

func TestOutputBufferPhaseResetOnNewWindow(t *testing.T) {
	buf := NewOutputBuffer()
	// First idle window: compile
	buf.Write([]byte("> go build"))
	if p := buf.CurrentPhase(); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile before reset, got %s", p)
	}
	buf.Reset()
	// Second idle window: test
	buf.Write([]byte("> go test"))
	if p := buf.CurrentPhase(); p != PhaseTest {
		t.Fatalf("expected PhaseTest after reset, got %s", p)
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

func TestANSIPhaseTriggerStillDetected(t *testing.T) {
	buf := NewOutputBuffer()
	buf.Write([]byte("\x1b[31m> go build ./...\x1b[0m\n"))
	if p := buf.CurrentPhase(); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile even with ANSI escapes, got %s", p)
	}
}

func TestAnalyzeLintEmptyOutputSuccess(t *testing.T) {
	r := analyzeOutput("", PhaseLint)
	if r != ResultSuccess {
		t.Fatalf("expected ResultSuccess for empty lint output, got %s", r)
	}
}

func TestAnalyzeTestUnmatchedOutputUnknown(t *testing.T) {
	r := analyzeOutput("some random output", PhaseTest)
	if r != ResultUnknown {
		t.Fatalf("expected ResultUnknown for unmatched test output, got %s", r)
	}
}

func TestAnalyzeCompileWithOutputButNoError(t *testing.T) {
	// "go build" that prints something but not an error (e.g. build stats)
	r := analyzeOutput("> go build ./...\n", PhaseCompile)
	if r != ResultSuccess {
		t.Fatalf("expected ResultSuccess for non-error build output, got %s", r)
	}
}

func TestAnalyzeGoModErrorPatterns(t *testing.T) {
	tests := []struct {
		name   string
		output string
	}{
		{"cannot find module", "go: github.com/user/repo: cannot find module providing package"},
		{"missing go.sum", "missing go.sum entry for module github.com/user/repo v1.2.3"},
		{"inconsistent vendoring", "inconsistent vendoring: package is in vendor but not in go.mod"},
		{"found packages", "found packages e (e.go) and main (main.go) in /tmp/test"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := analyzeOutput(tt.output, PhaseCompile)
			if r != ResultFailure {
				t.Errorf("expected ResultFailure for %q, got %s", tt.output, r)
			}
		})
	}
}

func TestExtractErrorLinesGoModErrors(t *testing.T) {
	output := "go: github.com/user/repo: cannot find module providing package\n> go build\n"
	got := extractErrorLines(output, PhaseCompile)
	if !strings.Contains(got, "cannot find module") {
		t.Errorf("expected error lines to contain 'cannot find module', got: %q", got)
	}
	if strings.Contains(got, "go build") {
		t.Errorf("expected error lines not to contain command echo, got: %q", got)
	}
}

func TestExtractErrorLinesGoSumErrors(t *testing.T) {
	output := "missing go.sum entry for module\nsome other text"
	got := extractErrorLines(output, PhaseCompile)
	if !strings.Contains(got, "missing go.sum") {
		t.Errorf("expected error lines to contain 'missing go.sum', got: %q", got)
	}
}

func TestOutputBufferConcurrentWrite(t *testing.T) {
	buf := NewOutputBuffer()
	done := make(chan struct{})
	go func() {
		for i := 0; i < 100; i++ {
			buf.Write([]byte("> go build\n"))
			buf.Analyze()
			buf.Reset()
		}
		close(done)
	}()
	for i := 0; i < 100; i++ {
		buf.Write([]byte("some output\n"))
		_ = buf.String()
	}
	<-done
}

// TestOutputBufferSizeLimitProtection verifies that the buffer enforces
// the maxBufferSize limit to prevent memory exhaustion attacks.
func TestOutputBufferSizeLimitProtection(t *testing.T) {
	buf := NewOutputBuffer()

	// Create data that's just under the limit
	largeData := make([]byte, maxBufferSize-1000)
	for i := 0; i < len(largeData); i++ {
		largeData[i] = 'a'
	}

	buf.Write(largeData)
	if buf.String() == "" {
		t.Fatal("expected buffer to contain data before limit")
	}

	// Try to write data that would exceed the limit
	buf.Write([]byte("x"))
	buf.Write([]byte("y"))
	buf.Write([]byte("z"))

	// Buffer should still contain original data
	if !strings.Contains(buf.String(), "a") {
		t.Fatal("expected buffer to still contain original data after overflow")
	}

	// Reset should clear overflow flag
	buf.Reset()
	buf.Write([]byte("new data"))
	if !strings.Contains(buf.String(), "new") {
		t.Fatal("expected buffer to accept data after reset")
	}
}

// TestOutputBufferOverflowDropsData verifies that writes after overflow
// are silently dropped to prevent memory growth.
func TestOutputBufferOverflowDropsData(t *testing.T) {
	buf := NewOutputBuffer()

	// Fill buffer to limit
	for i := 0; i < 5; i++ {
		buf.Write(make([]byte, maxBufferSize/5))
	}

	sizeBefore := buf.String()
	lenBefore := len(sizeBefore)

	// Try to write more data
	buf.Write([]byte("this should be dropped"))

	// Size should not have grown
	if len(buf.String()) > lenBefore {
		t.Errorf("buffer grew after overflow: %d -> %d", lenBefore, len(buf.String()))
	}
}
