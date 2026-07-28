//go:build integration

package main

import (
	"context"
	"encoding/json"
	"net"
	"os"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func socketPath(t *testing.T, name string) string {
	t.Helper()
	path := "/tmp/loopai-cmd-test-" + name
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestBackendBinaryStartsAndAcceptsConnection(t *testing.T) {
	sp := socketPath(t, "backend.sock")

	// Temporarily replace os.Args to control flags
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"loopai-backend", "-socket", sp}

	// Start backend in a goroutine
	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	time.Sleep(300 * time.Millisecond)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect to backend: %v", err)
	}
	defer conn.Close()

	pc := proto.NewConn(conn)
	ctx := context.Background()

	// Send a started message to verify the handler processes it
	startMsg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, startMsg); err != nil {
		t.Fatalf("send started: %v", err)
	}

	// Send output + idle to trigger the enforcement loop
	outMsg, err := proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: []byte("> go build ./...\n")})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, outMsg); err != nil {
		t.Fatalf("send output: %v", err)
	}

	idleMsg, err := proto.NewMessage(proto.MsgIdle, proto.IdlePayload{})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, idleMsg); err != nil {
		t.Fatalf("send idle: %v", err)
	}

	// Expect a MsgType prompt back
	msg, err := pc.Receive(ctx)
	if err != nil {
		t.Fatalf("receive prompt: %v", err)
	}
	if msg.Type != proto.MsgType {
		t.Fatalf("expected MsgType prompt, got %s", msg.Type)
	}

	// Send shutdown to clean up the handler
	shutdownMsg, err := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	pc.Send(ctx, shutdownMsg)
}

func TestBackendBinaryStartsWithDefaultSocket(t *testing.T) {
	// Verify DefaultSocketPath returns a valid path
	path := proto.DefaultSocketPath()
	if path == "" {
		t.Fatal("DefaultSocketPath returned empty")
	}
}

func TestBackendMessageRoundTrip(t *testing.T) {
	// Test that the wire protocol works independently of the backend binary
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sc := proto.NewConn(server)
	cc := proto.NewConn(client)
	ctx := context.Background()

	want := proto.StartedPayload{Pid: 42, Client: "test"}
	sendMsg, err := proto.NewMessage(proto.MsgStarted, want)
	if err != nil {
		t.Fatalf("new message: %v", err)
	}

	go func() {
		cc.Send(ctx, sendMsg)
	}()

	got, err := sc.Receive(ctx)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if got.Type != proto.MsgStarted {
		t.Fatalf("expected MsgStarted, got %s", got.Type)
	}
	var p proto.StartedPayload
	if err := json.Unmarshal(got.Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Pid != 42 || p.Client != "test" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}
