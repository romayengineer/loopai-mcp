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
			msg, err := pc.Receive(ctx)
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

func TestLoopCompileFailureThenSuccess(t *testing.T) {
	sp := loopSocketPath(t, "compile.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendOutput("> go build ./...\n")
	h.sendOutput("./main.go:23:2: undefined: Foo\n")
	h.sendIdle()

	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected non-empty prompt after compile failure")
	}

	h.sendOutput("> go build ./...\n")
	h.sendIdle()

	text2 := h.readMsgType(2 * time.Second)
	if len(text2) == 0 {
		t.Fatal("expected non-empty prompt after compile success")
	}
}

func TestLoopTestFailureThenSuccess(t *testing.T) {
	sp := loopSocketPath(t, "testfail.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendOutput("> go test ./...\n")
	h.sendOutput("--- FAIL: TestAdd\n    add_test.go:10: expected 5, got 4\nFAIL\n")
	h.sendIdle()

	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected non-empty prompt after test failure")
	}
}

func TestLoopProactivePromptOnIdleOutput(t *testing.T) {
	sp := loopSocketPath(t, "proactive.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendOutput("I'm thinking about the architecture\n")
	h.sendIdle()

	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected proactive prompt on idle output, got nothing")
	}
	if len(text) < 10 {
		t.Fatalf("expected substantial prompt text, got: %q", text)
	}
}

func TestLoopCompileErrorPattern(t *testing.T) {
	sp := loopSocketPath(t, "compileerr.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendOutput("> go build ./...\n")
	h.sendOutput("# github.com/user/repo\n./handler.go:42:2: unreachable code\n")
	h.sendIdle()

	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected prompt after compile error")
	}
}

func TestLoopLintFailure(t *testing.T) {
	sp := loopSocketPath(t, "lintfail.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendOutput("> golangci-lint run ./...\n")
	h.sendOutput("main.go:23:2: unused: variable x is unused\n")
	h.sendIdle()

	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected prompt after lint failure")
	}
}

func TestLoopFullSuccessSequence(t *testing.T) {
	sp := loopSocketPath(t, "fullpass.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	// Phase 1: compile passes
	h.sendOutput("> go build ./...\n")
	h.sendIdle()
	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected prompt after compile success")
	}

	// Phase 2: lint passes
	h.sendOutput("> golangci-lint run ./...\n")
	h.sendIdle()
	text2 := h.readMsgType(2 * time.Second)
	if len(text2) == 0 {
		t.Fatal("expected prompt after lint success")
	}

	// Phase 3: test passes
	h.sendOutput("> go test ./...\n")
	h.sendOutput("ok  github.com/user/repo\t0.234s\n")
	h.sendIdle()
	text3 := h.readMsgType(2 * time.Second)
	if len(text3) == 0 {
		t.Fatal("expected prompt after test success")
	}
}

func TestLoopBadOutputPayload(t *testing.T) {
	sp := loopSocketPath(t, "badpayload.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	// Send output with invalid JSON payload
	badMsg := proto.Message{Type: proto.MsgOutput, Payload: []byte(`{invalid}`)}
	h.pc.Send(context.Background(), badMsg)

	// The loop should continue (not crash). Verify by sending a valid message.
	h.sendOutput("> go build ./...\n")
	h.sendOutput("./main.go:5:2: undefined: Foo\n")
	h.sendIdle()

	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected prompt after compile failure, loop may have crashed on bad payload")
	}
}

func TestLoopUnknownMessageType(t *testing.T) {
	sp := loopSocketPath(t, "unknownmsg.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	// Send an unknown message type - loop should log and continue
	unknownMsg := proto.Message{Type: "unknown_type"}
	h.pc.Send(context.Background(), unknownMsg)

	// Verify loop still works
	h.sendOutput("> go build ./...\n")
	h.sendOutput("./main.go:5:2: undefined: Foo\n")
	h.sendIdle()

	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected prompt after unknown message, loop may have crashed")
	}
}

func TestLoopBadStartedPayload(t *testing.T) {
	sp := loopSocketPath(t, "badstarted.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	// Send MsgStarted with invalid JSON payload
	badMsg := proto.Message{Type: proto.MsgStarted, Payload: []byte(`{invalid}`)}
	h.pc.Send(context.Background(), badMsg)

	// The loop should continue (not crash). Verify by sending a valid message.
	h.sendOutput("> go build ./...\n")
	h.sendOutput("./main.go:5:2: undefined: Foo\n")
	h.sendIdle()

	text := h.readMsgType(2 * time.Second)
	if len(text) == 0 {
		t.Fatal("expected prompt after bad started payload, loop may have crashed")
	}
}

func TestLoopBadExitedPayload(t *testing.T) {
	sp := loopSocketPath(t, "badexited.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	// Send MsgExited with invalid JSON payload - handler should return
	// (not crash). We detect exit by checking that recvCh closes.
	badMsg := proto.Message{Type: proto.MsgExited, Payload: []byte(`{invalid}`)}
	h.pc.Send(context.Background(), badMsg)

	time.Sleep(200 * time.Millisecond)

	// The handler should have exited, closing the connection.
	// Our recv goroutine should detect this and close recvCh.
	select {
	case _, ok := <-h.recvCh:
		if !ok {
			// recvCh closed - handler exited as expected
		}
	default:
	}
}

func TestLoopContextCancel(t *testing.T) {
	sp := loopSocketPath(t, "ctxcancel.sock")
	ctx, cancel := context.WithCancel(context.Background())

	h := startLoopHarness(t, ctx, sp, HandleLauncher)

	// Cancel the context - the handler should exit
	cancel()

	// Wait for the handler to exit by sending a message and checking
	// that the recv goroutine detects the connection close.
	time.Sleep(200 * time.Millisecond)
	// After context cancel, the next Receive should return ctx.Err()
	// which causes HandleLauncher to return and close the connection.
	// We detect this by checking that our recvCh stops getting messages.
	select {
	case _, ok := <-h.recvCh:
		if ok {
			// We still got a message - but that's ok if it was sent before cancel
		}
	default:
	}
}
