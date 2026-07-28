package backend

import (
	"log/slog"
	"strings"
	"sync"
)

// maxBufferSize limits the output buffer to prevent memory exhaustion.
// Typical compile/test output is 1-2MB; 10MB is a generous limit that
// catches pathological cases (e.g., infinite loop printing to stdout).
const (
	maxBufferSize    = 10 * 1024 * 1024
	maxTriggerSample = 4 * 1024 // max bytes used for phase trigger detection
)

// OutputAnalyzer is the interface for analyzing terminal output and
// determining the current phase and its result. Decouples the
// enforcement state machine from the concrete buffer implementation.
type OutputAnalyzer interface {
	Write(data []byte)
	Analyze() GateResult
	Reset()
	String() string
}

// OutputBuffer accumulates terminal output between idle events and
// detects the current phase (compile/lint/test) from the content.
// The buffer is capped at maxBufferSize to prevent memory exhaustion.
type OutputBuffer struct {
	mu       sync.Mutex
	buf      strings.Builder
	phase    Phase
	overflow bool
}

// NewOutputBuffer creates an empty output buffer.
// The zero value of OutputBuffer is also usable since sync.Mutex
// and strings.Builder both have useful zero values.
func NewOutputBuffer() *OutputBuffer {
	return &OutputBuffer{}
}

// Write appends output data, stripping ANSI escape sequences, and
// updates the detected phase. Phase detection is performed on cleaned
// (ANSI-stripped) data to ensure consistency with buffer analysis.
// If the buffer exceeds maxBufferSize, further writes are dropped and
// overflow flag is set to prevent memory exhaustion.
func (b *OutputBuffer) Write(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()

	// Check if we've already overflowed; if so, drop remaining data
	if b.overflow {
		slog.Warn("output buffer overflow, discarding data", "buf_size", b.buf.Len())
		return
	}

	clean := StripANSI(data)

	// Check if adding this data would exceed limit
	if b.buf.Len()+len(clean) > maxBufferSize {
		b.overflow = true
		slog.Error("output buffer size limit exceeded", "limit", maxBufferSize, "current", b.buf.Len())
		return
	}

	b.buf.Write(clean)

	// Detect phase on a sample of the cleaned data (first 4KB). Phase
	// trigger patterns are simple keywords ("go build", "golangci-lint")
	// that always appear at the start of a tool command line, so a small
	// sample is sufficient. Truncating prevents regex slowdown on large
	// output (e.g. 2MB build logs).
	if b.phase == PhaseUnknown {
		sample := clean
		if len(sample) > maxTriggerSample {
			sample = sample[:maxTriggerSample]
		}
		if p := detectPhaseTrigger(string(sample)); p != PhaseUnknown {
			slog.Debug("phase detected", "phase", p)
			b.phase = p
		}
	}
}

// CurrentPhase returns the most recently detected phase.
func (b *OutputBuffer) CurrentPhase() Phase {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.phase
}

// Analyze returns the phase and its result (success/failure) based on
// the buffered output up to this point.
func (b *OutputBuffer) Analyze() GateResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.phase == PhaseUnknown {
		return GateResult{Phase: PhaseUnknown, Result: ResultUnknown}
	}

	result := analyzeOutput(b.buf.String(), b.phase)
	return GateResult{Phase: b.phase, Result: result}
}

// Reset clears the buffer, phase, and overflow flag.
func (b *OutputBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
	b.phase = PhaseUnknown
	b.overflow = false
}

// String returns the accumulated output as a string.
func (b *OutputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
