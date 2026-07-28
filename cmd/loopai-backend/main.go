package main

import (
	"context"
	"flag"
	"log/slog"
	"os"

	"github.com/romayengineer/loopai-mcp/internal/backend"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func main() {
	socketPath := flag.String("socket", proto.DefaultSocketPath(), "unix socket path")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	b := backend.New(*socketPath, backend.HandleLauncher)
	if err := b.Run(context.Background()); err != nil {
		slog.Error("backend", "error", err)
		os.Exit(1)
	}
}
