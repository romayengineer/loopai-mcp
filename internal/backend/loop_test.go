package backend

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

// mockPromptRenderer implements PromptRenderer for testing.
type mockPromptRenderer struct {
	mu       sync.Mutex
	names    []string     // records which templates were requested
	received []PromptVars // records the vars passed to each Render call
}

func (m *mockPromptRenderer) Render(name string, vars PromptVars) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.names = append(m.names, name)
	m.received = append(m.received, vars)
	return "rendered:" + name
}

func (m *mockPromptRenderer) Names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	result := make([]string, len(m.names))
	copy(result, m.names)
	return result
}

func (m *mockPromptRenderer) LastVars() PromptVars {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.received) == 0 {
		return PromptVars{}
	}
	return m.received[len(m.received)-1]
}

type mockAnalyzer struct {
	result GateResult
}

func (m *mockAnalyzer) Write([]byte)        {}
func (m *mockAnalyzer) Analyze() GateResult { return m.result }
func (m *mockAnalyzer) Reset()              {}
func (m *mockAnalyzer) String() string      { return "" }

type mockLauncherConn struct {
	mu           sync.Mutex
	messages     []proto.Message
	receiveCount int
}

func (c *mockLauncherConn) Send(_ context.Context, msg proto.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msg)
	return nil
}

func (c *mockLauncherConn) Receive(_ context.Context) (proto.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.receiveCount++
	return proto.Message{}, nil
}

func (c *mockLauncherConn) Close() error {
	return nil
}

type mockLauncherConnSendError struct {
	mockLauncherConn
}

func (c *mockLauncherConnSendError) Send(_ context.Context, _ proto.Message) error {
	return errors.New("send failed")
}

// mockConnReceiveSequence returns different messages in sequence, then closes
type mockConnReceiveSequence struct {
	mu       sync.Mutex
	messages []proto.Message
	index    int
	sent     []proto.Message
}

func (c *mockConnReceiveSequence) Send(_ context.Context, msg proto.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.sent = append(c.sent, msg)
	return nil
}

func (c *mockConnReceiveSequence) Receive(_ context.Context) (proto.Message, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.index >= len(c.messages) {
		return proto.Message{}, errors.New("connection closed")
	}
	msg := c.messages[c.index]
	c.index++
	return msg, nil
}

func (c *mockConnReceiveSequence) Close() error {
	return nil
}

func testPrompts() *PromptLoader {
	// When running go test, the working directory is the package directory
	// (internal/backend/). Prompts are at the project root (../../prompts/).
	return NewPromptLoader("../../prompts")
}

func TestGateHandleIdleSendError(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseCompile, Result: ResultSuccess},
	}, testPrompts())
	var conn mockLauncherConnSendError

	gate.handleIdle(context.Background(), &conn)
	// Test passes if no panic from the send failure
}

func TestGateHandleIdle(t *testing.T) {
	tests := []struct {
		name   string
		phase  Phase
		result PhaseResult
		want   int // expected number of messages
	}{
		{"compile success", PhaseCompile, ResultSuccess, 1},
		{"compile failure", PhaseCompile, ResultFailure, 1},
		{"lint success", PhaseLint, ResultSuccess, 1},
		{"lint failure", PhaseLint, ResultFailure, 1},
		{"test success", PhaseTest, ResultSuccess, 1},
		{"test failure", PhaseTest, ResultFailure, 1},
		{"unknown phase", PhaseUnknown, ResultUnknown, 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gate := NewGate(&mockAnalyzer{
				result: GateResult{Phase: tt.phase, Result: tt.result},
			}, testPrompts())
			var conn mockLauncherConn

			gate.handleIdle(context.Background(), &conn)
			if len(conn.messages) != tt.want {
				t.Fatalf("expected %d messages, got %d", tt.want, len(conn.messages))
			}
			if tt.want > 0 && conn.messages[0].Type != proto.MsgType {
				t.Fatalf("expected MsgType, got %s", conn.messages[0].Type)
			}
		})
	}
}

// TestGateWithMockPromptRenderer verifies that Gate uses the PromptRenderer
// interface correctly — it calls Render with the right template name and
// passes extracted error lines (not the full output buffer) as Errors.
func TestGateWithMockPromptRenderer(t *testing.T) {
	renderer := &mockPromptRenderer{}
	analyzer := &mockAnalyzer{
		result: GateResult{Phase: PhaseCompile, Result: ResultFailure},
	}
	gate := NewGate(analyzer, renderer)
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)

	names := renderer.Names()
	if len(names) != 1 || names[0] != "compile-fail" {
		t.Fatalf("expected prompt 'compile-fail', got %v", names)
	}
}

// TestGateWithRealPromptLoader verifies Gate still works with the
// concrete PromptLoader (implements PromptRenderer via the interface).
func TestGateWithRealPromptLoader(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseTest, Result: ResultSuccess},
	}, NewPromptLoader("."))
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 || conn.messages[0].Type != proto.MsgType {
		t.Fatalf("expected 1 MsgType message, got %d", len(conn.messages))
	}
}

// TestGateErrorsFieldIsExtracted verifies that the Errors field passed to
// the prompt renderer contains only extracted error lines, not the full
// output buffer.
func TestGateErrorsFieldIsExtracted(t *testing.T) {
	renderer := &mockPromptRenderer{}
	buf := NewOutputBuffer()
	gate := NewGate(buf, renderer)

	gate.handleOutput([]byte("> go build ./...\n"))
	gate.handleOutput([]byte("./main.go:23:2: undefined: Foo\n"))
	gate.handleOutput([]byte("some informational log line\n"))
	gate.handleIdle(context.Background(), &mockLauncherConn{})

	vars := renderer.LastVars()
	if vars.Phase != "compile" || vars.Result != "failure" {
		t.Fatalf("expected compile/failure, got %s/%s", vars.Phase, vars.Result)
	}
	if len(vars.Errors) == 0 {
		t.Fatal("expected non-empty Errors")
	}
	if strings.Contains(vars.Errors, "informational") {
		t.Fatalf("Errors should not contain non-error lines, got: %q", vars.Errors)
	}
	if !strings.Contains(vars.Errors, "main.go:23") {
		t.Fatalf("Errors should contain error lines, got: %q", vars.Errors)
	}
}

// TestGatePromptCooldown verifies that consecutive send calls for the same
// prompt name are throttled within promptCooldown. Only the first should go through.
func TestGatePromptCooldown(t *testing.T) {
	renderer := &mockPromptRenderer{}
	analyzer := &mockAnalyzer{
		result: GateResult{Phase: PhaseCompile, Result: ResultFailure},
	}
	gate := NewGate(analyzer, renderer)
	var conn mockLauncherConn

	// First call should send the prompt
	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message on first call, got %d", len(conn.messages))
	}
	conn.messages = nil
	analyzer.result = GateResult{Phase: PhaseCompile, Result: ResultFailure}

	// Second call immediately after should be suppressed by cooldown
	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 0 {
		t.Fatalf("expected 0 messages (cooldown), got %d", len(conn.messages))
	}
	// Verify only one template render happened
	names := renderer.Names()
	if len(names) != 1 {
		t.Fatalf("expected 1 template render, got %d: %v", len(names), names)
	}
}

// TestGatePromptCooldownDifferentNamesNotThrottled verifies that different
// prompt names are not throttled by each other's cooldown.
func TestGatePromptCooldownDifferentNamesNotThrottled(t *testing.T) {
	renderer := &mockPromptRenderer{}
	analyzer := &mockAnalyzer{}
	gate := NewGate(analyzer, renderer)
	var conn mockLauncherConn

	// First idle: compile failure
	analyzer.result = GateResult{Phase: PhaseCompile, Result: ResultFailure}
	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message for compile-fail, got %d", len(conn.messages))
	}
	conn.messages = nil

	// Second idle immediately: different prompt (lint success) should not be throttled
	analyzer.result = GateResult{Phase: PhaseLint, Result: ResultSuccess}
	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message for lint-pass, got %d", len(conn.messages))
	}
}

// TestGateIdleCooldownResetsAfterTimeout verifies that the idle prompt
// cooldown only applies to the PhaseUnknown case and resets after the
// 30-second window.
func TestGateIdleCooldownResetsAfterTimeout(t *testing.T) {
	renderer := &mockPromptRenderer{}
	buf := NewOutputBuffer()
	gate := NewGate(buf, renderer)
	var conn mockLauncherConn

	// First idle with output but no phase -> send idle prompt
	gate.handleOutput([]byte("I'm thinking about the code\n"))
	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message for first idle, got %d", len(conn.messages))
	}
	conn.messages = nil

	// Second idle immediately -> suppressed by 30s cooldown
	buf.Write([]byte("Still thinking...\n"))
	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 0 {
		t.Fatalf("expected 0 messages (idle cooldown), got %d", len(conn.messages))
	}
}

// TestHandleLauncherMalformedOutputContinues verifies that HandleLauncher
// continues processing when it receives a malformed OutputPayload (JSON decode error).
// This ensures output parse errors don't break the enforcement loop.
func TestHandleLauncherMalformedOutputContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &mockConnReceiveSequence{
		messages: []proto.Message{
			{Type: proto.MsgOutput, Payload: []byte("invalid json {")},
			{Type: proto.MsgExited, Payload: []byte(`{"code":0,"signal":""}`)},
		},
	}

	HandleLauncher(ctx, conn)

	// If HandleLauncher continued after malformed output, it would have processed
	// the exit message and exited cleanly. If it didn't continue, the test would
	// not reach this assertion (deadlock or panic).
	if conn.index != 2 {
		t.Errorf("expected to process both messages, processed %d", conn.index)
	}
}

// TestHandleLauncherMalformedExitExits verifies that HandleLauncher exits
// immediately when receiving a malformed ExitedPayload, rather than continuing.
// This prevents indefinite waiting for a valid exit signal.
func TestHandleLauncherMalformedExitExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &mockConnReceiveSequence{
		messages: []proto.Message{
			{Type: proto.MsgExited, Payload: []byte("invalid json {")},
		},
	}

	HandleLauncher(ctx, conn)

	// HandleLauncher should exit after the malformed exit message,
	// having called Receive only once.
	if conn.index != 1 {
		t.Errorf("expected to process 1 message, processed %d", conn.index)
	}
}

// TestHandleLauncherMalformedStartedContinues verifies that HandleLauncher
// continues processing when receiving a malformed StartedPayload.
func TestHandleLauncherMalformedStartedContinues(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &mockConnReceiveSequence{
		messages: []proto.Message{
			{Type: proto.MsgStarted, Payload: []byte("invalid json {")},
			{Type: proto.MsgExited, Payload: []byte(`{"code":0,"signal":""}`)},
		},
	}

	HandleLauncher(ctx, conn)

	// Should process both messages and exit normally
	if conn.index != 2 {
		t.Errorf("expected to process both messages, processed %d", conn.index)
	}
}

func TestGatePhaseAttemptsTracking(t *testing.T) {
	renderer := &mockPromptRenderer{}
	buf := NewOutputBuffer()
	gate := NewGate(buf, renderer)

	// First compile failure
	gate.handleOutput([]byte("> go build ./...\n"))
	gate.handleOutput([]byte("./main.go:5:2: undefined: Foo\n"))
	gate.handleIdle(context.Background(), &mockLauncherConn{})

	vars := renderer.LastVars()
	if vars.PhaseAttempts != 1 {
		t.Fatalf("expected 1 compile attempt, got %d", vars.PhaseAttempts)
	}
	if vars.TotalAttempts != 1 {
		t.Fatalf("expected 1 total attempt, got %d", vars.TotalAttempts)
	}

	// Second compile failure (same phase) — need to bypass cooldown
	gate.promptCooldown = 0 // disable cooldown for testing
	buf.Write([]byte("> go build ./...\n"))
	buf.Write([]byte("./main.go:10:2: undefined: Bar\n"))
	gate.handleIdle(context.Background(), &mockLauncherConn{})

	vars2 := renderer.LastVars()
	if vars2.PhaseAttempts != 2 {
		t.Fatalf("expected 2 compile attempts, got %d", vars2.PhaseAttempts)
	}
	if vars2.TotalAttempts != 2 {
		t.Fatalf("expected 2 total attempts, got %d", vars2.TotalAttempts)
	}
	gate.promptCooldown = defaultPromptCooldown // restore
}

func TestGatePhaseAttemptsResetOnNewPhase(t *testing.T) {
	renderer := &mockPromptRenderer{}
	buf := NewOutputBuffer()
	gate := NewGate(buf, renderer)

	// Compile fails once
	gate.handleOutput([]byte("> go build ./...\n"))
	gate.handleOutput([]byte("./main.go:5:2: undefined: Foo\n"))
	gate.handleIdle(context.Background(), &mockLauncherConn{})

	// Lint fails once — phase-specific counter resets for the new phase
	buf.Write([]byte("> golangci-lint run ./...\n"))
	buf.Write([]byte("main.go:5:2: unused: x\n"))
	gate.handleIdle(context.Background(), &mockLauncherConn{})

	vars := renderer.LastVars()
	if vars.Phase != "lint" || vars.Result != "failure" {
		t.Fatalf("expected lint/failure, got %s/%s", vars.Phase, vars.Result)
	}
	if vars.PhaseAttempts != 1 {
		t.Fatalf("expected 1 lint attempt, got %d", vars.PhaseAttempts)
	}
	// Total attempts accumulates across all phases
	if vars.TotalAttempts != 2 {
		t.Fatalf("expected 2 total attempts (1 compile + 1 lint), got %d", vars.TotalAttempts)
	}
}

// TestHandleLauncherCustomGateFactory verifies that HandleLauncher uses
// NewGateFunc to create the Gate, enabling dependency injection.
func TestHandleLauncherCustomGateFactory(t *testing.T) {
	orig := NewGateFunc
	defer func() { NewGateFunc = orig }()

	var created bool
	NewGateFunc = func() *Gate {
		created = true
		return NewGate(&mockAnalyzer{}, &mockPromptRenderer{})
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &mockConnReceiveSequence{
		messages: []proto.Message{
			{Type: proto.MsgExited, Payload: []byte(`{"code":0}`)},
		},
	}

	HandleLauncher(ctx, conn)
	if !created {
		t.Fatal("expected NewGateFunc to be called")
	}
}

// TestGateOutputCapping verifies that the Output field in PromptVars
// is capped at defaultMaxErrorBytes.
func TestGateOutputCapping(t *testing.T) {
	renderer := &mockPromptRenderer{}
	buf := NewOutputBuffer()
	gate := NewGate(buf, renderer)

	// Write output larger than the cap
	large := make([]byte, defaultMaxErrorBytes*2)
	for i := range large {
		large[i] = 'A'
	}
	gate.handleOutput(append([]byte("> go build ./...\n"), large...))
	gate.handleIdle(context.Background(), &mockLauncherConn{})

	vars := renderer.LastVars()
	if len(vars.Output) > defaultMaxErrorBytes {
		t.Fatalf("expected Output capped at %d, got %d bytes", defaultMaxErrorBytes, len(vars.Output))
	}
	if vars.BufSize < defaultMaxErrorBytes {
		t.Fatalf("expected BufSize to report actual size (%d+), got %d", defaultMaxErrorBytes, vars.BufSize)
	}
}

// TestGatePhaseAttemptsResetOnSuccess verifies that attempts counter
// resets to 0 when a phase succeeds.
func TestGatePhaseAttemptsResetOnSuccess(t *testing.T) {
	renderer := &mockPromptRenderer{}
	buf := NewOutputBuffer()
	gate := NewGate(buf, renderer)
	gate.promptCooldown = 0

	// First: compile fails
	gate.handleOutput([]byte("> go build ./...\n"))
	gate.handleOutput([]byte("./main.go:5:2: undefined: Foo\n"))
	gate.handleIdle(context.Background(), &mockLauncherConn{})

	vars := renderer.LastVars()
	if vars.PhaseAttempts != 1 {
		t.Fatalf("expected 1 compile attempt after failure, got %d", vars.PhaseAttempts)
	}

	// Second: compile passes — should reset attempts to 0
	buf.Write([]byte("> go build ./...\n"))
	gate.handleIdle(context.Background(), &mockLauncherConn{})

	vars2 := renderer.LastVars()
	if vars2.PhaseAttempts != 0 {
		t.Fatalf("expected 0 compile attempts after success, got %d", vars2.PhaseAttempts)
	}
	if vars2.Result != "success" {
		t.Fatalf("expected success result, got %s", vars2.Result)
	}
}

// TestGatePhaseAttemptsAccumulatesAcrossFailures verifies that attempts
// accumulate across multiple failures in a row.
func TestGatePhaseAttemptsAccumulatesAcrossFailures(t *testing.T) {
	renderer := &mockPromptRenderer{}
	buf := NewOutputBuffer()
	gate := NewGate(buf, renderer)
	gate.promptCooldown = 0

	// Three consecutive compile failures
	for i := 0; i < 3; i++ {
		buf.Write([]byte("> go build ./...\n"))
		buf.Write([]byte("./main.go:5:2: undefined: Foo\n"))
		gate.handleIdle(context.Background(), &mockLauncherConn{})
	}

	vars := renderer.LastVars()
	if vars.PhaseAttempts != 3 {
		t.Fatalf("expected 3 compile attempts after 3 failures, got %d", vars.PhaseAttempts)
	}
}
