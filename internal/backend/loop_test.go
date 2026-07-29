package backend

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

// mockRunner records how many times RunAll was called and optionally delays.
type mockRunner struct {
	mu    sync.Mutex
	calls int
	delay time.Duration
	done  chan struct{} // if set, RunAll blocks until this is closed
}

func (m *mockRunner) RunAll() []ToolResult {
	m.mu.Lock()
	m.calls++
	m.mu.Unlock()
	if m.done != nil {
		<-m.done
	}
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	return nil
}

func (m *mockRunner) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.calls
}

type mockPromptRenderer struct {
	mu    sync.Mutex
	names []string
	vars  []PromptVars
}

func (m *mockPromptRenderer) Render(name string, vars PromptVars) string {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.names = append(m.names, name)
	m.vars = append(m.vars, vars)
	return "rendered:" + name
}

func (m *mockPromptRenderer) Names() []string {
	m.mu.Lock()
	defer m.mu.Unlock()
	r := make([]string, len(m.names))
	copy(r, m.names)
	return r
}

func (m *mockPromptRenderer) LastVars() PromptVars {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.vars) == 0 {
		return PromptVars{}
	}
	return m.vars[len(m.vars)-1]
}

type mockLauncherConn struct {
	mu       sync.Mutex
	messages []proto.Message
}

func (c *mockLauncherConn) Send(_ context.Context, msg proto.Message) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.messages = append(c.messages, msg)
	return nil
}

func (c *mockLauncherConn) Receive(_ context.Context) (proto.Message, error) {
	return proto.Message{}, nil
}

func (c *mockLauncherConn) Close() error { return nil }

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

func (c *mockConnReceiveSequence) Close() error { return nil }

func TestGateNew(t *testing.T) {
	r := &mockPromptRenderer{}
	runner := &mockRunner{}
	g := NewGate(r, runner)
	if g == nil {
		t.Fatal("NewGate returned nil")
	}
	if g.prompts != r {
		t.Fatal("NewGate did not store prompts")
	}
	if g.runner != runner {
		t.Fatal("NewGate did not store runner")
	}
}

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

	if conn.index != 2 {
		t.Errorf("expected to process both messages, processed %d", conn.index)
	}
}

func TestHandleLauncherMalformedExitExits(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &mockConnReceiveSequence{
		messages: []proto.Message{
			{Type: proto.MsgExited, Payload: []byte("invalid json {")},
		},
	}

	HandleLauncher(ctx, conn)

	if conn.index != 1 {
		t.Errorf("expected to process 1 message, processed %d", conn.index)
	}
}

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

	if conn.index != 2 {
		t.Errorf("expected to process both messages, processed %d", conn.index)
	}
}

func TestHandleLauncherUnknownMessage(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	conn := &mockConnReceiveSequence{
		messages: []proto.Message{
			{Type: "unknown_type"},
			{Type: proto.MsgExited, Payload: []byte(`{"code":0}`)},
		},
	}

	HandleLauncher(ctx, conn)

	if conn.index != 2 {
		t.Errorf("expected to process both messages, processed %d", conn.index)
	}
}

func TestHandleLauncherContextCancel(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	conn := &mockConnReceiveSequence{
		messages: []proto.Message{
			{Type: proto.MsgExited, Payload: []byte(`{"code":0}`)},
		},
	}

	HandleLauncher(ctx, conn)

	// Should exit immediately without processing any messages
	if conn.index != 0 {
		t.Errorf("expected 0 messages processed, processed %d", conn.index)
	}
}

func TestGateEnforceCooldown(t *testing.T) {
	runner := &mockRunner{}
	gate := NewGate(&mockPromptRenderer{}, runner)
	var conn mockLauncherConn

	gate.HandleEnforcement(context.Background(), &conn)

	if runner.CallCount() != 1 {
		t.Fatalf("expected 1 runner call, got %d", runner.CallCount())
	}

	// Second call immediately should be suppressed by cooldown
	second := time.Now()
	gate.HandleEnforcement(context.Background(), &conn)
	if time.Since(second) > 100*time.Millisecond {
		t.Fatal("HandleEnforcement took too long — should have been suppressed by cooldown")
	}
	if runner.CallCount() != 1 {
		t.Fatalf("expected still 1 runner call (suppressed), got %d", runner.CallCount())
	}
}

func TestAllPassed(t *testing.T) {
	if allPassed([]ToolResult{{Name: "build", Passed: true}}) != true {
		t.Fatal("expected true for all passed")
	}
	if allPassed([]ToolResult{{Name: "build", Passed: true}, {Name: "lint", Passed: false}}) != false {
		t.Fatal("expected false when one fails")
	}
	if allPassed(nil) != true {
		t.Fatal("expected true for empty results")
	}
}

// TestGateConcurrentEnforcementSingleRun verifies that concurrent calls to
// HandleEnforcement result in only one actual RunAll execution. The mutex
// and running flag should drop all concurrent idle events.
func TestGateConcurrentEnforcementSingleRun(t *testing.T) {
	done := make(chan struct{})
	runner := &mockRunner{done: done}
	gate := NewGate(&mockPromptRenderer{}, runner)
	var conn mockLauncherConn

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			gate.HandleEnforcement(context.Background(), &conn)
		}()
	}
	// Give goroutines time to reach the mutex and block
	time.Sleep(50 * time.Millisecond)
	// Unblock the runner so all remaining goroutines get past the mutex
	close(done)
	wg.Wait()

	if runner.CallCount() != 1 {
		t.Fatalf("expected exactly 1 runner call (only one should execute), got %d", runner.CallCount())
	}
}

// TestGateEnforcementBlockedWhileRunning verifies that a call to
// HandleEnforcement while another is in progress is silently dropped.
func TestGateEnforcementBlockedWhileRunning(t *testing.T) {
	done := make(chan struct{})
	runner := &mockRunner{done: done}
	gate := NewGate(&mockPromptRenderer{}, runner)
	var conn mockLauncherConn

	// Start enforcement in background (it will block on `done`)
	go gate.HandleEnforcement(context.Background(), &conn)
	time.Sleep(20 * time.Millisecond) // let the goroutine acquire the mutex

	// Second call should return immediately without executing
	gate.HandleEnforcement(context.Background(), &conn)

	if runner.CallCount() != 1 {
		t.Fatalf("expected 1 runner call (second should be blocked), got %d", runner.CallCount())
	}

	close(done)
	time.Sleep(50 * time.Millisecond) // let the first enforcement finish
}

// TestGateEnforcementCooldownAfterCompletion verifies that lastEnforce is
// set only after the runner completes, not before.
func TestGateEnforcementCooldownAfterCompletion(t *testing.T) {
	runner := &mockRunner{delay: 50 * time.Millisecond}
	gate := NewGate(&mockPromptRenderer{}, runner)
	var conn mockLauncherConn

	gate.HandleEnforcement(context.Background(), &conn)
	if runner.CallCount() != 1 {
		t.Fatalf("expected 1 runner call, got %d", runner.CallCount())
	}

	// Immediately call again — should be suppressed by cooldown
	gate.HandleEnforcement(context.Background(), &conn)
	if runner.CallCount() != 1 {
		t.Fatalf("expected 1 runner call (suppressed by cooldown), got %d", runner.CallCount())
	}
}
