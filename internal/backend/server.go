// Package backend implements the LoopAI-MCP server that receives client
// terminal output from the launcher, runs the enforcement state machine,
// and makes decisions (type prompts, send Ctrl+C, etc.).
package backend

import (
	"context"
	"log/slog"
	"net"
	"sync/atomic"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

type Backend struct {
	socketPath string
	handler    func(context.Context, *proto.Conn)
	ln         atomic.Value // stores net.Listener
}

func New(socketPath string, handler func(context.Context, *proto.Conn)) *Backend {
	return &Backend{
		socketPath: socketPath,
		handler:    handler,
	}
}

func (b *Backend) Run(ctx context.Context) error {
	ln, err := proto.Listen(b.socketPath)
	if err != nil {
		return err
	}
	b.ln.Store(ln)

	slog.Info("backend started", "socket", b.socketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		slog.Info("launcher connected")
		pc := proto.NewConn(conn)
		go b.handler(ctx, pc)
	}
}

func (b *Backend) Stop() {
	if v := b.ln.Load(); v != nil {
		v.(net.Listener).Close()
	}
}
