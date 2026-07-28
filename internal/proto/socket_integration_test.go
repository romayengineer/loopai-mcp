//go:build integration

package proto_test

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

var bg = context.Background()

func socketPath(t *testing.T, name string) string {
	t.Helper()
	path := "/tmp/loopai-test-" + name
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func TestSocketListenConnectRoundTrip(t *testing.T) {
	sp := socketPath(t, "roundtrip.sock")

	ln, err := proto.Listen(sp)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	serverConn, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer serverConn.Close()

	clientSide := proto.NewConn(conn)
	serverSide := proto.NewConn(serverConn)

	sendMsg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 100, Client: "test"})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := clientSide.Send(bg, sendMsg); err != nil {
		t.Fatalf("client send: %v", err)
	}

	recvMsg, err := serverSide.Receive(bg)
	if err != nil {
		t.Fatalf("server receive: %v", err)
	}
	if recvMsg.Type != proto.MsgStarted {
		t.Fatalf("expected %q, got %q", proto.MsgStarted, recvMsg.Type)
	}
	var p proto.StartedPayload
	if err := json.Unmarshal(recvMsg.Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Pid != 100 || p.Client != "test" {
		t.Fatalf("unexpected payload: %+v", p)
	}

	reply, err := proto.NewMessage(proto.MsgType, proto.TypePayload{Text: "hello"})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := serverSide.Send(bg, reply); err != nil {
		t.Fatalf("server send: %v", err)
	}

	recv2, err := clientSide.Receive(bg)
	if err != nil {
		t.Fatalf("client receive: %v", err)
	}
	if recv2.Type != proto.MsgType {
		t.Fatalf("expected %q, got %q", proto.MsgType, recv2.Type)
	}
}

func TestSocketMultipleMessages(t *testing.T) {
	sp := socketPath(t, "multi.sock")

	ln, err := proto.Listen(sp)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	sc, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer sc.Close()

	client := proto.NewConn(conn)
	server := proto.NewConn(sc)

	rawMessages := []struct {
		t   proto.MessageType
		pay interface{}
	}{
		{proto.MsgOutput, proto.OutputPayload{Data: []byte("line1\n")}},
		{proto.MsgOutput, proto.OutputPayload{Data: []byte("line2\n")}},
		{proto.MsgIdle, proto.IdlePayload{}},
		{proto.MsgExited, proto.ExitedPayload{Code: 0}},
	}
	messages := make([]proto.Message, len(rawMessages))
	for i, r := range rawMessages {
		msg, err := proto.NewMessage(r.t, r.pay)
		if err != nil {
			t.Fatalf("new message #%d: %v", i, err)
		}
		messages[i] = msg
	}

	for _, msg := range messages {
		if err := client.Send(bg, msg); err != nil {
			t.Fatalf("send: %v", err)
		}
	}

	for i, expected := range messages {
		received, err := server.Receive(bg)
		if err != nil {
			t.Fatalf("receive #%d: %v", i, err)
		}
		if received.Type != expected.Type {
			t.Fatalf("msg #%d: expected %q, got %q", i, expected.Type, received.Type)
		}
	}
}

func TestSocketDefaultPath(t *testing.T) {
	os.Setenv("LOOPAI_SOCKET_DIR", "/tmp/loopai-test")
	defer os.Unsetenv("LOOPAI_SOCKET_DIR")

	path := proto.DefaultSocketPath()
	if path != "/tmp/loopai-test/loopai.sock" {
		t.Fatalf("unexpected path: %s", path)
	}
}

func TestSocketCleanupOnListen(t *testing.T) {
	sp := socketPath(t, "cleanup.sock")

	if err := os.WriteFile(sp, []byte("stale"), 0644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	ln, err := proto.Listen(sp)
	if err != nil {
		t.Fatalf("listen (should remove stale): %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Errorf("close listener: %v", err)
	}
}

func TestSocketBinaryPayload(t *testing.T) {
	sp := socketPath(t, "binary.sock")

	ln, err := proto.Listen(sp)
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()

	sc, err := ln.Accept()
	if err != nil {
		t.Fatalf("accept: %v", err)
	}
	defer sc.Close()

	client := proto.NewConn(conn)
	server := proto.NewConn(sc)

	binaryData := []byte{0x00, 0x01, 0x02, 0xFE, 0xFF}
	msg, err := proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: binaryData})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := client.Send(bg, msg); err != nil {
		t.Fatalf("send: %v", err)
	}

	recv, err := server.Receive(bg)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if recv.Type != proto.MsgOutput {
		t.Fatalf("expected %q, got %q", proto.MsgOutput, recv.Type)
	}
	var p proto.OutputPayload
	if err := json.Unmarshal(recv.Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(p.Data) != 5 || p.Data[0] != 0x00 || p.Data[4] != 0xFF {
		t.Fatalf("unexpected binary data: %v", p.Data)
	}
}

func TestListenInvalidPath(t *testing.T) {
	_, err := proto.Listen("/nonexistent_dir/socket.sock")
	if err == nil {
		t.Fatal("expected error for Listen with invalid parent directory path")
	}
}

func TestConnectNonexistentSocket(t *testing.T) {
	_, err := proto.Connect("/tmp/loopai-test-nonexistent-12345.sock")
	if err == nil {
		t.Fatal("expected error for Connect to nonexistent socket")
	}
}

func TestListenPathTooLong(t *testing.T) {
	// Create a socket path that exceeds the Unix socket max path length (~108 bytes).
	dir, err := os.MkdirTemp("", "loopai-test-*")
	if err != nil {
		t.Fatalf("temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	// Build a path > 108 bytes so net.Listen("unix", path) fails
	longPath := filepath.Join(dir, strings.Repeat("a", 150))
	_, err = proto.Listen(longPath)
	if err == nil {
		t.Fatal("expected error when listening with too-long socket path")
	}
}

func TestDefaultSocketPathMkdirFail(t *testing.T) {
	// Create a regular file, then try to use a subdirectory of it as socket dir.
	// os.MkdirAll will fail because the parent is a file, not a directory.
	f, err := os.CreateTemp("", "loopai-test-*")
	if err != nil {
		t.Fatalf("create temp: %v", err)
	}
	f.Close()
	defer os.Remove(f.Name())

	t.Setenv("LOOPAI_SOCKET_DIR", f.Name()+"/subdir")
	path := proto.DefaultSocketPath()
	if path == "" {
		t.Fatal("DefaultSocketPath returned empty even with MkdirAll failure")
	}
	if !strings.HasSuffix(path, "/subdir/loopai.sock") {
		t.Fatalf("expected path ending with '/subdir/loopai.sock', got %q", path)
	}
}

func TestListenStaleSocketRemoved(t *testing.T) {
	sp := socketPath(t, "stale2.sock")
	// Create a stale file (not a socket)
	if err := os.WriteFile(sp, []byte("stale data"), 0644); err != nil {
		t.Fatalf("write stale: %v", err)
	}

	ln, err := proto.Listen(sp)
	if err != nil {
		t.Fatalf("listen after removing stale: %v", err)
	}
	if err := ln.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
}
