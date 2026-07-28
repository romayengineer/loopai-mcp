package backend

import (
	"log"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

type Backend struct {
	socketPath string
	handler    func(*proto.Conn)
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
	defer ln.Close()

	log.Printf("backend listening on %s", b.socketPath)

	for {
		conn, err := ln.Accept()
		if err != nil {
			return err
		}
		log.Printf("launcher connected")
		pc := proto.NewConn(conn)
		go b.handler(pc)
	}
}
