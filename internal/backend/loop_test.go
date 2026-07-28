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

func (c *mockLauncherConn) Close() error {
	return nil
}

type mockLauncherConnSendError struct {
	mockLauncherConn
}

func (c *mockLauncherConnSendError) Send(_ context.Context, _ proto.Message) error {
	return errors.New("send failed")
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
