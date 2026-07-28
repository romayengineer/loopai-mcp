// Package backend implements the LoopAI-MCP server that receives client
// terminal output from the launcher, runs the enforcement state machine,
// and makes decisions (type prompts, send Ctrl+C, etc.).
package backend

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"sync/atomic"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

// LauncherConn is the interface that a launcher connection must satisfy.
// It decouples the enforcement loop from the underlying transport.
type LauncherConn interface {
	Send(context.Context, proto.Message) error
	Receive(context.Context) (proto.Message, error)
	Close() error
}

// Backend is the LoopAI-MCP server. It listens on a Unix socket,
// accepts launcher connections, and dispatches each to a handler.
type Backend struct {
	socketPath string
	handler    func(context.Context, LauncherConn)
	ln         atomic.Value // stores net.Listener
}

// New creates a Backend that will listen on the given socket path.
func New(socketPath string, handler func(context.Context, LauncherConn)) *Backend {
	return &Backend{
		socketPath: socketPath,
		handler:    handler,
	}
}

// Run starts the Unix socket accept loop. It blocks until the context
// is cancelled, an accept error occurs, or the listener is closed.
func (b *Backend) Run(ctx context.Context) error {
	ln, err := proto.Listen(b.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	b.ln.Store(ln)

	slog.Info("backend started", "socket", b.socketPath)

	for {
		select {
		case <-ctx.Done():
			return nil
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return fmt.Errorf("accept: %w", err)
		}
		slog.Info("launcher connected")
		pc := proto.NewConn(conn)
		go b.handler(ctx, pc)
	}
}

// Stop closes the listener and stops accepting new connections.
func (b *Backend) Stop() {
	if v := b.ln.Load(); v != nil {
		ln, ok := v.(net.Listener)
		if !ok {
			slog.Warn("stored value is not a listener")
			return
		}
		if err := ln.Close(); err != nil {
			slog.Warn("close listener", "error", err)
		}
	}
}
