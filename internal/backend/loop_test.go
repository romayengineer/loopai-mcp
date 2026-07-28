package backend

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

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
	g := NewGate(r)
	if g == nil {
		t.Fatal("NewGate returned nil")
	}
	if g.prompts != r {
		t.Fatal("NewGate did not store prompts")
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
	r := &mockPromptRenderer{}
	gate := NewGate(r)
	var conn mockLauncherConn

	// First enforcement should run tools (they will fail since no project exists)
	gate.HandleEnforcement(context.Background(), &conn)

	// Second call immediately should be suppressed by cooldown
	second := time.Now()
	gate.HandleEnforcement(context.Background(), &conn)
	if time.Since(second) > 100*time.Millisecond {
		t.Fatal("HandleEnforcement took too long — probably ran tools instead of cooldown")
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
