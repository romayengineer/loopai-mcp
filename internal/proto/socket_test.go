package proto

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"
)

type mockConn struct {
	net.Conn
	readErr  error
	writeErr error
}

func (m *mockConn) Read(b []byte) (int, error) {
	return 0, m.readErr
}

func (m *mockConn) Write(b []byte) (int, error) {
	return 0, m.writeErr
}

func (m *mockConn) SetReadDeadline(_ time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(_ time.Time) error {
	return nil
}

func (m *mockConn) Close() error {
	return nil
}

func TestNewConn(t *testing.T) {
	c := NewConn(&mockConn{})
	if c == nil {
		t.Fatal("NewConn returned nil")
	}
}

func TestSendEncodeError(t *testing.T) {
	c := NewConn(&mockConn{writeErr: errors.New("write failed")})
	err := c.Send(context.Background(), Message{Type: MsgOutput})
	if err == nil {
		t.Fatal("expected error from Send with broken writer")
	}
}

func TestReceiveScanError(t *testing.T) {
	c := NewConn(&mockConn{readErr: errors.New("read failed")})
	_, err := c.Receive(context.Background())
	if err == nil {
		t.Fatal("expected error from Receive with broken reader")
	}
}

func TestReceiveDecodeError(t *testing.T) {
	// Garbage bytes in the reader will fail JSON decode.
	// Use a real pipe since bufio.Scanner buffers reads internally.
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	cc := NewConn(client)
	go func() {
		server.Write([]byte("{invalid json}\n"))
		server.Close()
	}()

	_, err := cc.Receive(context.Background())
	if err == nil {
		t.Fatal("expected decode error from invalid JSON")
	}
}

func TestReceiveConnectionClosed(t *testing.T) {
	server, client := net.Pipe()
	cc := NewConn(client)
	server.Close()

	_, err := cc.Receive(context.Background())
	if err == nil {
		t.Fatal("expected error from closed connection")
	}
}

func TestReceiveSetReadDeadlineError(t *testing.T) {
	cc := NewConn(&deadlineErrorConn{})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()

	_, err := cc.Receive(ctx)
	if err == nil {
		t.Fatal("expected error from SetReadDeadline failure")
	}
}

func TestSendSetWriteDeadlineError(t *testing.T) {
	cc := NewConn(&deadlineErrorConn{})
	ctx, cancel := context.WithDeadline(context.Background(), time.Now().Add(time.Second))
	defer cancel()

	err := cc.Send(ctx, Message{Type: MsgOutput})
	if err == nil {
		t.Fatal("expected error from SetWriteDeadline failure")
	}
}

type deadlineErrorConn struct {
	net.Conn
}

func (d *deadlineErrorConn) SetWriteDeadline(_ time.Time) error {
	return errors.New("deadline error")
}

func (d *deadlineErrorConn) SetReadDeadline(_ time.Time) error {
	return errors.New("deadline error")
}
