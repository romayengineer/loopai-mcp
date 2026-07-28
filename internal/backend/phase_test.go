package backend

import (
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
