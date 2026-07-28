package backend

import (
	"strings"
	"sync"
)

// OutputBuffer accumulates terminal output between idle events and
// detects the current phase (compile/lint/test) from the content.
type OutputBuffer struct {
	mu    sync.Mutex
	buf   strings.Builder
	phase Phase
}

// NewOutputBuffer creates an empty output buffer.
func NewOutputBuffer() *OutputBuffer {
	return &OutputBuffer{}
}

// Write appends output data, stripping ANSI escape sequences, and
// updates the detected phase.
func (b *OutputBuffer) Write(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	clean := StripANSI(data)
	b.buf.Write(clean)

	if p := detectPhaseTrigger(string(data)); p != PhaseUnknown {
		b.phase = p
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

// Reset clears the buffer and resets the phase to unknown.
func (b *OutputBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
	b.phase = PhaseUnknown
}

// String returns the accumulated output as a string.
func (b *OutputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
