//go:build integration

package backend

import (
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func socketPath(t *testing.T, name string) string {
	t.Helper()
	path := filepath.Join("/tmp", "loopai-test-"+name)
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestBackendAcceptsLauncher(t *testing.T) {
	sp := socketPath(t, "accept.sock")

	var started atomic.Bool
	handler := func(pc *proto.Conn) {
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

		pc.Send(proto.NewMessage(proto.MsgType, proto.TypePayload{Text: "hello"}))
	}

	backend := New(sp, handler)
	go func() {
		if err := backend.Run(); err != nil {
			t.Errorf("backend: %v", err)
		}
	}()
	time.Sleep(200 * time.Millisecond)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	pc := proto.NewConn(conn)
	pc.Send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"}))

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

	handler := func(pc *proto.Conn) {
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

	backend := New(sp, handler)
	go func() {
		backend.Run()
	}()
	time.Sleep(200 * time.Millisecond)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	pc := proto.NewConn(conn)
	pc.Send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"}))
	pc.Send(proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: []byte("test output\n")}))
	pc.Send(proto.NewMessage(proto.MsgIdle, proto.IdlePayload{}))
	pc.Send(proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: 0}))
}
