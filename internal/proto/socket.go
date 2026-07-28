package proto

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	// SocketFileName is the default Unix socket filename.
	SocketFileName = "loopai.sock"
	// SocketDirMode is the permission mode for the socket directory.
	SocketDirMode os.FileMode = 0700
	// SocketFileMode is the permission mode for the socket file.
	SocketFileMode    os.FileMode = 0600
	socketDialTimeout             = 5 * time.Second
	// maxMessageSize limits the size of a single message to prevent memory exhaustion.
	// Typical messages are <100KB; 10MB is a generous limit.
	maxMessageSize = 10 * 1024 * 1024
)

// DefaultSocketPath returns the default Unix socket path, using
// LOOPAI_SOCKET_DIR, ~/.config/loopai, or /tmp as the directory.
func DefaultSocketPath() string {
	dir := os.Getenv("LOOPAI_SOCKET_DIR")
	if dir == "" {
		home, err := os.UserHomeDir()
		if err == nil {
			dir = filepath.Join(home, ".config", "loopai")
		} else {
			dir = "/tmp"
		}
	}
	// MkdirAll failure is tolerated because the caller (Listen) also creates
	// the directory with the same permissions. If the directory doesn't exist,
	// Listen will fail with a clear error. This function is best-effort so that
	// standalone path lookups (e.g. flag defaults) don't fail unnecessarily.
	if err := os.MkdirAll(dir, SocketDirMode); err != nil {
		return filepath.Join(dir, SocketFileName)
	}
	return filepath.Join(dir, SocketFileName)
}

// Listen creates a Unix socket listener at the given path, removing
// any stale socket file first.
func Listen(socketPath string) (net.Listener, error) {
	if err := os.Remove(socketPath); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("remove stale socket: %w", err)
	}
	parent := filepath.Dir(socketPath)
	if err := os.MkdirAll(parent, SocketDirMode); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	if err := os.Chmod(socketPath, SocketFileMode); err != nil {
		if closeErr := ln.Close(); closeErr != nil {
			return nil, fmt.Errorf("chmod socket: %w (close: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return ln, nil
}

// Connect dials a Unix socket at the given path with a dial timeout.
func Connect(socketPath string) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", socketPath, socketDialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", socketPath, err)
	}
	return conn, nil
}

// Conn wraps a net.Conn with a JSON encoder/decoder for reading and
// writing newline-delimited Message frames.
type Conn struct {
	conn   net.Conn
	writer *json.Encoder
	reader *bufio.Scanner
}

// NewConn creates a Conn wrapping the given network connection.
// The scanner is configured with maxMessageSize to prevent memory exhaustion
// from maliciously large or corrupted messages.
func NewConn(conn net.Conn) *Conn {
	scanner := bufio.NewScanner(conn)
	// Set buffer size limit: default 64KB is too small for large output chunks,
	// but 10MB is a reasonable limit for typical compile/test output.
	scanner.Buffer(make([]byte, 64*1024), maxMessageSize)
	return &Conn{
		conn:   conn,
		writer: json.NewEncoder(conn),
		reader: scanner,
	}
}

// Send encodes and writes a Message to the connection. If ctx carries
// a deadline, the write deadline is set accordingly. Returns ctx.Err()
// if the context is already cancelled before attempting the write.
func (c *Conn) Send(ctx context.Context, msg Message) error {
	select {
	case <-ctx.Done():
		return fmt.Errorf("send %s: %w", msg.Type, ctx.Err())
	default:
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("set write deadline: %w", err)
		}
	}
	err := c.writer.Encode(msg)
	if err == nil {
		slog.Debug("sent message", "type", msg.Type)
	}
	return err
}

// Receive reads a newline-delimited JSON Message from the connection.
// If ctx carries a deadline, the read deadline is set accordingly.
// Returns ctx.Err() if the context is already cancelled before attempting
// the read.
func (c *Conn) Receive(ctx context.Context) (Message, error) {
	select {
	case <-ctx.Done():
		return Message{}, fmt.Errorf("receive: %w", ctx.Err())
	default:
	}
	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetReadDeadline(deadline); err != nil {
			return Message{}, fmt.Errorf("set read deadline: %w", err)
		}
	}
	if !c.reader.Scan() {
		if err := c.reader.Err(); err != nil {
			return Message{}, err
		}
		return Message{}, fmt.Errorf("connection closed")
	}
	var msg Message
	if err := json.Unmarshal(c.reader.Bytes(), &msg); err != nil {
		return Message{}, fmt.Errorf("decode message: %w", err)
	}
	slog.Debug("received message", "type", msg.Type)
	return msg, nil
}

// Close closes the underlying network connection.
func (c *Conn) Close() error {
	return c.conn.Close()
}
