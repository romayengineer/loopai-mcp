package backend

import (
	"regexp"
	"strings"
)

const defaultMaxErrorBytes = 100 * 1024

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
	{Trigger: regexp.MustCompile(`(?m)cannot find module`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)missing go.sum`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)inconsistent vendoring`), Result: ResultFailure},
	{Trigger: regexp.MustCompile(`(?m)found packages`), Result: ResultFailure},
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
	{Phase: PhaseCompile, Regexp: regexp.MustCompile(`(?m)\bgo mod\b`)},
	{Phase: PhaseCompile, Regexp: regexp.MustCompile(`(?m)\bgo generate\b`)},
	{Phase: PhaseCompile, Regexp: regexp.MustCompile(`(?m)\bgo run\b`)},
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

// extractErrorLines returns only the lines from output that match the
// failure patterns for the given phase. This is used to populate the
// Errors template variable with relevant error lines instead of passing
// the entire multi-megabyte output buffer to the prompt template.
//
// For unknown phases or phases with no failure patterns, returns the
// full output unchanged (with trailing whitespace trimmed).
func extractErrorLines(output string, p Phase) string {
	// Trim trailing whitespace before splitting to avoid empty trailing
	// elements from strings.Split.
	output = strings.TrimRight(output, "\n\r\t ")

	patterns := gatePatterns(p)
	if len(patterns) == 0 {
		return output
	}

	var errorPatterns []*regexp.Regexp
	for _, pm := range patterns {
		if pm.Result == ResultFailure {
			errorPatterns = append(errorPatterns, pm.Trigger)
		}
	}
	if len(errorPatterns) == 0 {
		return output
	}

	lines := strings.Split(output, "\n")
	var matched []string
	for _, line := range lines {
		for _, re := range errorPatterns {
			if re.MatchString(line) {
				matched = append(matched, line)
				break
			}
		}
	}
	return strings.Join(matched, "\n")
}

// extractErrorLinesMax is like extractErrorLines but caps the total
// output to maxBytes to prevent unbounded memory when the error output
// itself is very large (e.g. thousands of compiler errors).
func extractErrorLinesMax(output string, p Phase, maxBytes int) string {
	result := extractErrorLines(output, p)
	if len(result) > maxBytes {
		return result[:maxBytes]
	}
	return result
}
