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

func startTestBackend(t *testing.T, sp string, handler func(context.Context, LauncherConn)) *Backend {
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
	handler := func(ctx context.Context, conn LauncherConn) {
		defer conn.Close()
		msg, err := conn.Receive(ctx)
		if err != nil {
			t.Errorf("receive: %v", err)
			return
		}
		if msg.Type != proto.MsgStarted {
			t.Errorf("expected %q, got %q", proto.MsgStarted, msg.Type)
		}
		started.Store(true)

		reply, mErr := proto.NewMessage(proto.MsgType, proto.TypePayload{Text: "hello"})
		if mErr != nil {
			t.Errorf("new message: %v", mErr)
			return
		}
		if err := conn.Send(ctx, reply); err != nil {
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
	ctx := context.Background()
	sendMsg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, sendMsg); err != nil {
		t.Fatalf("send started: %v", err)
	}

	msg, err := pc.Receive(ctx)
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

	handler := func(ctx context.Context, conn LauncherConn) {
		defer conn.Close()
		for {
			msg, err := conn.Receive(ctx)
			if err != nil {
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
	ctx := context.Background()

	send := func(msg proto.Message) {
		if err := pc.Send(ctx, msg); err != nil {
			t.Errorf("send: %v", err)
		}
	}

	msgs := []struct {
		t   proto.MessageType
		pay interface{}
	}{
		{proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"}},
		{proto.MsgOutput, proto.OutputPayload{Data: []byte("test output\n")}},
		{proto.MsgIdle, proto.IdlePayload{}},
		{proto.MsgExited, proto.ExitedPayload{Code: 0}},
	}
	for _, m := range msgs {
		msg, mErr := proto.NewMessage(m.t, m.pay)
		if mErr != nil {
			t.Fatalf("new message: %v", mErr)
		}
		send(msg)
	}
}

func TestBackendRunStopRace(t *testing.T) {
	sp := socketPath(t, "runstop.sock")

	ctx, cancel := context.WithCancel(context.Background())
	b := New(sp, func(_ context.Context, _ LauncherConn) {})
	defer cancel()

	// Start Run in the background - it will block on ln.Accept()
	go func() {
		b.Run(ctx)
	}()

	time.Sleep(testStartupTimeout)

	// Stop while Run is blocked on Accept
	b.Stop()

	// Should not panic or deadlock
	time.Sleep(100 * time.Millisecond)
}
