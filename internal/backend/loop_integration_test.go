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

func TestLoopNoPromptOnNonBuildOutput(t *testing.T) {
	sp := loopSocketPath(t, "noop.sock")
	h := startLoopHarness(t, context.Background(), sp, HandleLauncher)

	h.sendOutput("I'm thinking about the architecture\n")
	h.sendIdle()

	msg, ok := h.expectMsg(1 * time.Second)
	if ok {
		t.Fatalf("expected no prompt, got %s", msg.Type)
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
