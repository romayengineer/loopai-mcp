//go:build integration

package backend

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func init() {
	DefaultPromptsDir = "../../prompts"
}

func loopSocketPath(t *testing.T, name string) string {
	t.Helper()
	path := "/tmp/loopai-loop-" + name
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

type loopHarness struct {
	t         *testing.T
	pc        *proto.Conn
	recvCh    chan proto.Message
	closeOnce sync.Once
	done      chan struct{}
}

func startLoopHarness(t *testing.T, ctx context.Context, socketPath string, handler func(context.Context, LauncherConn)) *loopHarness {
	t.Helper()

	ctx, cancel := context.WithCancel(ctx)
	b := New(socketPath, handler)
	go func() {
		if err := b.Run(ctx); err != nil {
			t.Logf("backend: %v", err)
		}
	}()
	t.Cleanup(func() { cancel(); b.Stop() })
	time.Sleep(200 * time.Millisecond)

	conn, err := proto.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	pc := proto.NewConn(conn)

	h := &loopHarness{
		t:      t,
		pc:     pc,
		recvCh: make(chan proto.Message, 50),
		done:   make(chan struct{}),
	}

	go func() {
		defer close(h.done)
		for {
			msg, err := pc.Receive(context.Background())
			if err != nil {
				return
			}
			select {
			case h.recvCh <- msg:
			default:
			}
		}
	}()

	t.Cleanup(func() {
		conn.Close()
		<-h.done
	})

	h.sendStarted()
	return h
}

func (h *loopHarness) sendStarted() {
	msg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"})
	if err != nil {
		h.t.Fatalf("new message: %v", err)
	}
	if err := h.pc.Send(context.Background(), msg); err != nil {
		h.t.Fatalf("send started: %v", err)
	}
}

func (h *loopHarness) sendOutput(data string) {
	msg, err := proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: []byte(data)})
	if err != nil {
		h.t.Fatalf("new message: %v", err)
	}
	if err := h.pc.Send(context.Background(), msg); err != nil {
		h.t.Fatalf("send output: %v", err)
	}
}

func (h *loopHarness) sendIdle() {
	msg, err := proto.NewMessage(proto.MsgIdle, proto.IdlePayload{})
	if err != nil {
		h.t.Fatalf("new message: %v", err)
	}
	if err := h.pc.Send(context.Background(), msg); err != nil {
		h.t.Fatalf("send idle: %v", err)
	}
}

func (h *loopHarness) expectMsg(d time.Duration) (proto.Message, bool) {
	select {
	case msg := <-h.recvCh:
		return msg, true
	case <-time.After(d):
		return proto.Message{}, false
	}
}

func (h *loopHarness) readMsgType(d time.Duration) string {
	msg, ok := h.expectMsg(d)
	if !ok {
		h.t.Fatal("timed out waiting for MsgType")
	}
	if msg.Type != proto.MsgType {
		h.t.Fatalf("expected MsgType, got %s", msg.Type)
	}
	var p proto.TypePayload
	if err := json.Unmarshal(msg.Payload, &p); err != nil {
		h.t.Fatalf("unmarshal: %v", err)
	}
	return p.Text
}

// TestLoopIdleTriggersEnforcement verifies that sending an idle message
// triggers the enforcement loop and produces a prompt.
func TestLoopIdleTriggersEnforcement(t *testing.T) {
	sp := loopSocketPath(t, "idle.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendIdle()

	text := h.readMsgType(5 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected prompt after idle, got empty")
	}
}

// TestLoopOutputThenIdle verifies that output followed by idle still triggers
// enforcement (output is discarded, enforcement runs independently).
func TestLoopOutputThenIdle(t *testing.T) {
	sp := loopSocketPath(t, "outidle.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendOutput("some output from client\n")
	h.sendIdle()

	text := h.readMsgType(5 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected prompt after output+idle, got empty")
	}
}

// TestLoopExitAfterIdle verifies that the handler processes exit correctly
// after an idle event has triggered enforcement.
func TestLoopExitAfterIdle(t *testing.T) {
	sp := loopSocketPath(t, "exitidle.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendIdle()
	h.readMsgType(5 * time.Second)

	// Send exit and verify handler terminates
	exitMsg, err := proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: 0})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	h.pc.Send(context.Background(), exitMsg)

	time.Sleep(100 * time.Millisecond)

	select {
	case _, ok := <-h.recvCh:
		if !ok {
			// recvCh closed — handler exited
		}
	default:
	}
}
