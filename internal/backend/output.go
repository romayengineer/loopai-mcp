package backend

import (
	"strings"
	"sync"
)

type OutputBuffer struct {
	mu    sync.Mutex
	buf   strings.Builder
	phase Phase
}

func NewOutputBuffer() *OutputBuffer {
	return &OutputBuffer{}
}

func (b *OutputBuffer) Write(data []byte) {
	b.mu.Lock()
	defer b.mu.Unlock()
	clean := StripANSI(data)
	b.buf.Write(clean)

	if p := detectPhaseTrigger(string(data)); p != PhaseUnknown {
		b.phase = p
	}
}

func (b *OutputBuffer) CurrentPhase() Phase {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.phase
}

func (b *OutputBuffer) Analyze() GateResult {
	b.mu.Lock()
	defer b.mu.Unlock()

	if b.phase == PhaseUnknown {
		return GateResult{Phase: PhaseUnknown, Result: ResultUnknown}
	}

	result := analyzeOutput(b.buf.String(), b.phase)
	return GateResult{Phase: b.phase, Result: result}
}

func (b *OutputBuffer) Reset() {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf.Reset()
	b.phase = PhaseUnknown
}

func (b *OutputBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}
