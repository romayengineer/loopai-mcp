//go:build integration

package backend

import (
	"context"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const testStartupTimeout = 200 * time.Millisecond

func socketPath(t *testing.T, name string) string {
	t.Helper()
	path := "/tmp/loopai-test-" + name
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func startTestBackend(t *testing.T, sp string, handler func(context.Context, *proto.Conn)) *Backend {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	b := New(sp, handler)
	go func() {
		if err := b.Run(ctx); err != nil {
			t.Logf("backend exited: %v", err)
		}
	}()
	t.Cleanup(func() { cancel(); b.Stop() })
	time.Sleep(testStartupTimeout)
	return b
}

func TestBackendAcceptsLauncher(t *testing.T) {
	sp := socketPath(t, "accept.sock")

	var started atomic.Bool
	handler := func(_ context.Context, pc *proto.Conn) {
		defer pc.Close()
		msg, err := pc.Receive()
		if err != nil {
			t.Errorf("receive: %v", err)
			return
		}
		if msg.Type != proto.MsgStarted {
			t.Errorf("expected %q, got %q", proto.MsgStarted, msg.Type)
		}
		started.Store(true)

		if err := pc.Send(proto.NewMessage(proto.MsgType, proto.TypePayload{Text: "hello"})); err != nil {
			t.Errorf("send reply: %v", err)
		}
	}

	startTestBackend(t, sp, handler)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	pc := proto.NewConn(conn)
	if err := pc.Send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"})); err != nil {
		t.Fatalf("send started: %v", err)
	}

	msg, err := pc.Receive()
	if err != nil {
		t.Fatalf("receive reply: %v", err)
	}
	if msg.Type != proto.MsgType {
		t.Fatalf("expected %q, got %q", proto.MsgType, msg.Type)
	}

	if !started.Load() {
		t.Fatal("handler was not called")
	}
}

func TestBackendFullExchange(t *testing.T) {
	sp := socketPath(t, "full.sock")

	handler := func(_ context.Context, pc *proto.Conn) {
		defer pc.Close()
		for i := 0; i < 3; i++ {
			msg, err := pc.Receive()
			if err != nil {
				t.Errorf("receive: %v", err)
				return
			}
			if msg.Type == proto.MsgExited {
				return
			}
		}
	}

	startTestBackend(t, sp, handler)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	pc := proto.NewConn(conn)

	send := func(msg proto.Message) {
		if err := pc.Send(msg); err != nil {
			t.Errorf("send: %v", err)
		}
	}

	send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"}))
	send(proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: []byte("test output\n")}))
	send(proto.NewMessage(proto.MsgIdle, proto.IdlePayload{}))
	send(proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: 0}))
}
