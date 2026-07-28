package backend

import (
	"strings"
	"testing"
)

func TestPhaseString(t *testing.T) {
	tests := []struct {
		phase Phase
		want  string
	}{
		{PhaseUnknown, "unknown"},
		{PhaseCompile, "compile"},
		{PhaseLint, "lint"},
		{PhaseTest, "test"},
		{Phase(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.phase.String()
		if got != tt.want {
			t.Errorf("Phase(%d).String() = %q, want %q", tt.phase, got, tt.want)
		}
	}
}

func TestPhaseResultString(t *testing.T) {
	tests := []struct {
		result PhaseResult
		want   string
	}{
		{ResultUnknown, "unknown"},
		{ResultSuccess, "success"},
		{ResultFailure, "failure"},
		{PhaseResult(99), "unknown"},
	}
	for _, tt := range tests {
		got := tt.result.String()
		if got != tt.want {
			t.Errorf("PhaseResult(%d).String() = %q, want %q", tt.result, got, tt.want)
		}
	}
}

func TestDetectPhaseTriggerGoBuild(t *testing.T) {
	if p := detectPhaseTrigger("> go build ./...\n"); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile, got %s", p)
	}
}

func TestDetectPhaseTriggerGoVet(t *testing.T) {
	if p := detectPhaseTrigger("> go vet ./...\n"); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile for go vet, got %s", p)
	}
}

func TestDetectPhaseTriggerGoInstall(t *testing.T) {
	if p := detectPhaseTrigger("> go install ./...\n"); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile for go install, got %s", p)
	}
}

func TestDetectPhaseTriggerGoTest(t *testing.T) {
	if p := detectPhaseTrigger("> go test ./...\n"); p != PhaseTest {
		t.Fatalf("expected PhaseTest, got %s", p)
	}
}

func TestDetectPhaseTriggerGoModTidy(t *testing.T) {
	if p := detectPhaseTrigger("> go mod tidy\n"); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile for go mod tidy, got %s", p)
	}
}

func TestDetectPhaseTriggerGoModDownload(t *testing.T) {
	if p := detectPhaseTrigger("> go mod download\n"); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile for go mod download, got %s", p)
	}
}

func TestDetectPhaseTriggerGoModVerify(t *testing.T) {
	if p := detectPhaseTrigger("> go mod verify\n"); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile for go mod verify, got %s", p)
	}
}

func TestDetectPhaseTriggerGoGenerate(t *testing.T) {
	if p := detectPhaseTrigger("> go generate ./...\n"); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile for go generate, got %s", p)
	}
}

func TestDetectPhaseTriggerGoRun(t *testing.T) {
	if p := detectPhaseTrigger("> go run ./cmd/main.go\n"); p != PhaseCompile {
		t.Fatalf("expected PhaseCompile for go run, got %s", p)
	}
}

func TestDetectPhaseTriggerGolangciLint(t *testing.T) {
	if p := detectPhaseTrigger("> golangci-lint run ./...\n"); p != PhaseLint {
		t.Fatalf("expected PhaseLint, got %s", p)
	}
}

func TestDetectPhaseTriggerBareLintWord(t *testing.T) {
	if p := detectPhaseTrigger("I will lint the code now"); p != PhaseLint {
		t.Fatalf("expected PhaseLint for bare lint word, got %s", p)
	}
}

func TestDetectPhaseTriggerNoMatch(t *testing.T) {
	if p := detectPhaseTrigger("some random output"); p != PhaseUnknown {
		t.Fatalf("expected PhaseUnknown, got %s", p)
	}
}

func TestGatePatternsNonNull(t *testing.T) {
	for _, phase := range []Phase{PhaseCompile, PhaseLint, PhaseTest} {
		if patterns := gatePatterns(phase); patterns == nil {
			t.Errorf("gatePatterns(%s) returned nil", phase)
		}
	}
	if patterns := gatePatterns(PhaseUnknown); patterns != nil {
		t.Errorf("gatePatterns(PhaseUnknown) should return nil, got %v", patterns)
	}
}

func TestExtractErrorLines(t *testing.T) {
	tests := []struct {
		name     string
		output   string
		phase    Phase
		expected string
		contains []string // optional: expected substrings
	}{
		{
			name:     "compile error",
			output:   "# github.com/user/repo/pkg\n./main.go:23:2: undefined: Foo\nsome info text\n",
			phase:    PhaseCompile,
			expected: "# github.com/user/repo/pkg\n./main.go:23:2: undefined: Foo",
		},
		{
			name:     "compile no error",
			output:   "> go build ./...",
			phase:    PhaseCompile,
			expected: "",
		},
		{
			name:     "test failure",
			output:   "--- FAIL: TestFoo\n\tfoo_test.go:10: got 4, want 5\nFAIL\nok  github.com/bar\t0.2s\n",
			phase:    PhaseTest,
			expected: "--- FAIL: TestFoo\n\tfoo_test.go:10: got 4, want 5",
		},
		{
			name:     "test pass extracts nothing",
			output:   "ok  github.com/user/repo\t0.234s\n",
			phase:    PhaseTest,
			expected: "",
		},
		{
			name:     "lint error",
			output:   "main.go:23:2: unused: variable x is unused\ninternal/handler.go:42:6: exported func is unused\n",
			phase:    PhaseLint,
			expected: "main.go:23:2: unused: variable x is unused\ninternal/handler.go:42:6: exported func is unused",
		},
		{
			name:     "unknown phase returns full output",
			output:   "some random text\nmore text\n",
			phase:    PhaseUnknown,
			expected: "some random text\nmore text",
			contains: []string{"some random text", "more text"},
		},
		{
			name:     "empty output",
			output:   "",
			phase:    PhaseCompile,
			expected: "",
		},
		{
			name:     "data race detection",
			output:   "WARNING: DATA RACE\nWrite at 0x123 by goroutine 5:\n  main.go:42\n",
			phase:    PhaseTest,
			contains: []string{"WARNING: DATA RACE"},
		},
		{
			name:     "compile vet error",
			output:   "# github.com/user/repo\n./handler.go:42:2: unreachable code\n",
			phase:    PhaseCompile,
			expected: "# github.com/user/repo\n./handler.go:42:2: unreachable code",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := extractErrorLines(tt.output, tt.phase)
			if tt.expected != "" && got != tt.expected {
				t.Errorf("extractErrorLines:\ngot:\n%q\nwant:\n%q", got, tt.expected)
			}
			for _, substr := range tt.contains {
				if !strings.Contains(got, substr) {
					t.Errorf("expected result to contain %q, got:\n%q", substr, got)
				}
			}
		})
	}
}

func TestExtractErrorLinesMax(t *testing.T) {
	output := "# github.com/user/repo/pkg\n./main.go:23:2: undefined: Foo\n"
	gotFull := extractErrorLines(output, PhaseCompile)
	// Both lines are error matches, so they're both kept
	if !strings.Contains(gotFull, "# github.com") || !strings.Contains(gotFull, "main.go:23") {
		t.Fatalf("expected error lines, got %q", gotFull)
	}

	gotCapped := extractErrorLinesMax(output, PhaseCompile, 10)
	if len(gotCapped) > 10 {
		t.Fatalf("expected max 10 bytes, got %d: %q", len(gotCapped), gotCapped)
	}
}

func TestExtractErrorLinesUnknownPhaseFullOutput(t *testing.T) {
	output := "some random output\nwithout errors\n"
	got := extractErrorLines(output, PhaseUnknown)
	// Trailing newline is trimmed
	expected := "some random output\nwithout errors"
	if got != expected {
		t.Errorf("expected %q for unknown phase, got %q", expected, got)
	}
}

func TestExtractErrorLinesMaxUnderLimit(t *testing.T) {
	output := "WARNING: DATA RACE"
	got := extractErrorLinesMax(output, PhaseTest, 1_000_000)
	if got != output {
		t.Errorf("expected full output when under limit, got %q", got)
	}
}
