package proto

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"
)

const (
	SocketFileName                = "loopai.sock"
	SocketDirMode     os.FileMode = 0700
	SocketFileMode    os.FileMode = 0600
	socketDialTimeout             = 5 * time.Second
)

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
	if err := os.MkdirAll(dir, SocketDirMode); err != nil {
		return filepath.Join(dir, SocketFileName)
	}
	return filepath.Join(dir, SocketFileName)
}

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
			err = fmt.Errorf("chmod socket: %w (close: %v)", err, closeErr)
		}
		return nil, fmt.Errorf("chmod socket: %w", err)
	}
	return ln, nil
}

func Connect(socketPath string) (net.Conn, error) {
	conn, err := net.DialTimeout("unix", socketPath, socketDialTimeout)
	if err != nil {
		return nil, fmt.Errorf("connect to %s: %w", socketPath, err)
	}
	return conn, nil
}

type Conn struct {
	conn   net.Conn
	writer *json.Encoder
	reader *bufio.Scanner
}

func NewConn(conn net.Conn) *Conn {
	return &Conn{
		conn:   conn,
		writer: json.NewEncoder(conn),
		reader: bufio.NewScanner(conn),
	}
}

func (c *Conn) Send(ctx context.Context, msg Message) error {
	if deadline, ok := ctx.Deadline(); ok {
		if err := c.conn.SetWriteDeadline(deadline); err != nil {
			return fmt.Errorf("set write deadline: %w", err)
		}
	}
	return c.writer.Encode(msg)
}

func (c *Conn) Receive(ctx context.Context) (Message, error) {
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
	return msg, nil
}

func (c *Conn) Close() error {
	return c.conn.Close()
}
