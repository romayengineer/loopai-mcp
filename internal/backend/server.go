// Package backend implements the LoopAI-MCP server that receives client
// terminal output from the launcher, runs the enforcement state machine,
// and makes decisions (type prompts, send Ctrl+C, etc.).
package backend

import (
	"log"
	"net"
	"sync"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

type Backend struct {
	socketPath string
	handler    func(*proto.Conn)
	ln         net.Listener
	mu         sync.Mutex
	started    bool
}

func New(socketPath string, handler func(*proto.Conn)) *Backend {
	return &Backend{
		socketPath: socketPath,
		handler:    handler,
	}
}

func (b *Backend) Run() error {
	ln, err := proto.Listen(b.socketPath)
	if err != nil {
		return err
	}

	b.mu.Lock()
	b.ln = ln
	b.started = true
	b.mu.Unlock()

	log.Printf("backend listening on %s", b.socketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			b.mu.Lock()
			stopped := !b.started
			b.mu.Unlock()
			if stopped {
				return nil
			}
			return err
		}
		log.Printf("launcher connected")
		pc := proto.NewConn(conn)
		go b.handler(pc)
	}
}

func (b *Backend) Stop() {
	b.mu.Lock()
	defer b.mu.Unlock()
	if !b.started {
		return
	}
	b.started = false
	if b.ln != nil {
		b.ln.Close()
	}
}
