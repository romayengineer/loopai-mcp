package backend

import "regexp"

// Phase identifies which enforcement gate the model is currently in.
type Phase int

const (
	PhaseUnknown Phase = iota
	PhaseCompile
	PhaseLint
	PhaseTest
)

func (p Phase) String() string {
	switch p {
	case PhaseCompile:
		return "compile"
	case PhaseLint:
		return "lint"
	case PhaseTest:
		return "test"
	default:
		return "unknown"
	}
}

// PhaseResult indicates whether a given phase passed or failed.
type PhaseResult int

const (
	ResultUnknown PhaseResult = iota
	ResultSuccess
	ResultFailure
)

func (r PhaseResult) String() string {
	switch r {
	case ResultSuccess:
		return "success"
	case ResultFailure:
		return "failure"
	default:
		return "unknown"
	}
}

// GateResult pairs a detected phase with its success/failure result.
type GateResult struct {
	Phase  Phase
	Result PhaseResult
}

type phaseMatch struct {
	Trigger *regexp.Regexp
	Result  PhaseResult
}

// Go patterns
var compilePatterns = []phaseMatch{
	{Trigger: regexp.MustCompile(`(?m)^\S+\.go:\d+:\d+: `), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)^\S+\.go:\d+: `), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)^#\s+\S+`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)^FAIL\s+\S+\[build failed\]`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)compilation error`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)cannot find package`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)undefined:`), Result: ResultFailure},
}

var testPatterns = []phaseMatch{
	{Trigger: regexp.MustCompile(`(?m)^ok\s+\S+`), Result: ResultSuccess},
	{Trigger: regexp.MustCompile(`(?m)^--- FAIL:\s+Test`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)^FAIL\s+\S+`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)^\t\S+\.go:\d+: `), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)WARNING: DATA RACE`), Result: ResultFailure},
}

var lintPatterns = []phaseMatch{
	{Trigger: regexp.MustCompile(`(?m)^\S+\.go:\d+:\d+: `), Result: ResultFailure},
}

var phaseTriggers = []struct {
	Phase  Phase
	Regexp *regexp.Regexp
}{
	{Phase: PhaseCompile, Regexp: regexp.MustCompile(`(?m)\bgo build\b`)},
	{Phase: PhaseCompile, Regexp: regexp.MustCompile(`(?m)\bgo install\b`)},
	{Phase: PhaseCompile, Regexp: regexp.MustCompile(`(?m)\bgo vet\b`)},
	{Phase: PhaseLint, Regexp: regexp.MustCompile(`(?m)\bgolangci-lint\b`)},
	{Phase: PhaseLint, Regexp: regexp.MustCompile(`(?m)\blint\b`)},
	{Phase: PhaseTest, Regexp: regexp.MustCompile(`(?m)\bgo test\b`)},
}

func gatePatterns(p Phase) []phaseMatch {
	switch p {
	case PhaseCompile:
		return compilePatterns
	case PhaseLint:
		return lintPatterns
	case PhaseTest:
		return testPatterns
	default:
		return nil
	}
}

func detectPhaseTrigger(output string) Phase {
	for _, t := range phaseTriggers {
		if t.Regexp.MatchString(output) {
			return t.Phase
		}
	}
	return PhaseUnknown
}

func analyzeOutput(output string, p Phase) PhaseResult {
	if p == PhaseUnknown {
		return ResultUnknown
	}
	patterns := gatePatterns(p)
	for _, pm := range patterns {
		if pm.Trigger.MatchString(output) {
			return pm.Result
		}
	}
	// For compile and lint, no error patterns in the output means the tool
	// produced no output, which means success (go build / golangci-lint print
	// nothing on success, only on failure).
	if p == PhaseCompile || p == PhaseLint {
		return ResultSuccess
	}
	return ResultUnknown
}
