package main

import (
	"flag"
	"log"

	"github.com/romayengineer/loopai-mcp/internal/backend"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func main() {
	socketPath := flag.String("socket", proto.DefaultSocketPath(), "unix socket path")
	flag.Parse()

	b := backend.New(*socketPath, backend.HandleLauncher)
	log.Printf("loopai-backend starting on %s", *socketPath)
	if err := b.Run(); err != nil {
		log.Fatal(err)
	}
}
