// Package backend implements the LoopAI-MCP server that receives client
// terminal output from the launcher, runs the enforcement state machine,
// and makes decisions (type prompts, send Ctrl+C, etc.).
package backend

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const acceptRetryDelay = 100 * time.Millisecond

// LauncherConn is the interface that a launcher connection must satisfy.
// It decouples the enforcement loop from the underlying transport.
type LauncherConn interface {
	Send(context.Context, proto.Message) error
	Receive(context.Context) (proto.Message, error)
	Close() error
}

// SocketListener creates a named Unix socket listener.
// Decouples Backend from the concrete proto.Listen implementation.
type SocketListener interface {
	Listen(socketPath string) (net.Listener, error)
}

// socketListenFunc adapts a function to the SocketListener interface.
type socketListenFunc func(string) (net.Listener, error)

func (f socketListenFunc) Listen(socketPath string) (net.Listener, error) {
	return f(socketPath)
}

// Backend is the LoopAI-MCP server. It listens on a Unix socket,
// accepts launcher connections, and dispatches each to a handler.
// On Stop(), the listener is closed and Run() waits for all handlers
// to complete (up to the context deadline).
type Backend struct {
	socketPath string
	handler    func(context.Context, LauncherConn)
	ln         atomic.Value // stores net.Listener
	listener   SocketListener
	wg         sync.WaitGroup // tracks active handler goroutines
}

// New creates a Backend that will listen on the given socket path.
func New(socketPath string, handler func(context.Context, LauncherConn)) *Backend {
	return &Backend{
		socketPath: socketPath,
		handler:    handler,
		listener:   socketListenFunc(proto.Listen),
	}
}

// Run starts the Unix socket accept loop. It blocks until the context
// is cancelled, an accept error occurs, or the listener is closed.
// Retries temporary acceptance errors. Tracks handler goroutines via
// a WaitGroup; Stop() waits for all handlers to complete.
func (b *Backend) Run(ctx context.Context) error {
	ln, err := b.listener.Listen(b.socketPath)
	if err != nil {
		return fmt.Errorf("listen: %w", err)
	}
	b.ln.Store(ln)

	slog.Info("backend started", "socket", b.socketPath)

	for {
		select {
		case <-ctx.Done():
			b.wg.Wait()
			return ctx.Err()
		default:
		}

		conn, err := ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				b.wg.Wait()
				return ctx.Err()
			}
			// Retry on timeout errors (e.g. EINTR from interrupted accept).
			// On listener.Close(), we get net.ErrClosed which is not a timeout,
			// so this retry only triggers for transient OS-level interruptions.
			var netErr net.Error
			if errors.As(err, &netErr) && netErr.Timeout() {
				slog.Debug("timeout accept error, retrying", "error", err)
				time.Sleep(acceptRetryDelay)
				continue
			}
			return fmt.Errorf("accept: %w", err)
		}
		slog.Info("launcher connected", "local_addr", conn.LocalAddr().String())
		pc := proto.NewConn(conn)
		b.wg.Add(1)
		go func() {
			defer b.wg.Done()
			b.handler(ctx, pc)
		}()
	}
}

// Stop closes the listener and stops accepting new connections.
// Blocks until all handler goroutines have completed.
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
	b.wg.Wait()
}
