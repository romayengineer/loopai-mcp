package proto

import (
	"bufio"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"path/filepath"
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
	os.MkdirAll(dir, 0700)
	return filepath.Join(dir, "loopai.sock")
}

func Listen(socketPath string) (net.Listener, error) {
	os.Remove(socketPath)
	parent := filepath.Dir(socketPath)
	if err := os.MkdirAll(parent, 0700); err != nil {
		return nil, fmt.Errorf("create socket dir: %w", err)
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", socketPath, err)
	}
	os.Chmod(socketPath, 0600)
	return ln, nil
}

func Connect(socketPath string) (net.Conn, error) {
	conn, err := net.Dial("unix", socketPath)
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

func (c *Conn) Send(msg Message) error {
	return c.writer.Encode(msg)
}

func (c *Conn) Receive() (Message, error) {
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
