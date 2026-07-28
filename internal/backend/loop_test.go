package backend

import (
	"context"
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

func TestGateHandleIdleCompileSuccess(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseCompile, Result: ResultSuccess},
	})
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(conn.messages))
	}
	if conn.messages[0].Type != proto.MsgType {
		t.Fatalf("expected MsgType, got %s", conn.messages[0].Type)
	}
}

func TestGateHandleIdleCompileFailure(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseCompile, Result: ResultFailure},
	})
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(conn.messages))
	}
}

func TestGateHandleIdleLintSuccess(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseLint, Result: ResultSuccess},
	})
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(conn.messages))
	}
}

func TestGateHandleIdleLintFailure(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseLint, Result: ResultFailure},
	})
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(conn.messages))
	}
}

func TestGateHandleIdleTestSuccess(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseTest, Result: ResultSuccess},
	})
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(conn.messages))
	}
}

func TestGateHandleIdleTestFailure(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseTest, Result: ResultFailure},
	})
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 1 {
		t.Fatalf("expected 1 message, got %d", len(conn.messages))
	}
}

func TestGateHandleIdleUnknownPhase(t *testing.T) {
	gate := NewGate(&mockAnalyzer{
		result: GateResult{Phase: PhaseUnknown, Result: ResultUnknown},
	})
	var conn mockLauncherConn

	gate.handleIdle(context.Background(), &conn)
	if len(conn.messages) != 0 {
		t.Fatalf("expected 0 messages for unknown phase, got %d", len(conn.messages))
	}
}
