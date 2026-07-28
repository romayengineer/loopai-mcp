package backend

import (
	"context"
	"errors"
	"sync"
	"testing"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

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

func TestGateHandleIdleSendError(t *testing.T) {
	// handleIdle's send() closure should log and continue on Send error.
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseCompile, Result: ResultSuccess},
	}, NewPromptLoader("."))
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
			}, NewPromptLoader("."))
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
