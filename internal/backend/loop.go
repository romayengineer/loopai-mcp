package backend

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

// HandleLauncher drives the enforcement loop for a single launcher
// connection: reads idle messages, runs enforcement tools, and sends
// prompts with the result back to the PTY.
func HandleLauncher(ctx context.Context, conn LauncherConn) {
	defer conn.Close()
	gate := NewGate(NewPromptLoader(DefaultPromptsDir), ToolRunner{})

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := conn.Receive(ctx)
		if err != nil {
			slog.Debug("launcher disconnected", "error", err)
			return
		}

		switch msg.Type {
		case proto.MsgOutput:
			// Discard client output — enforcement runs independently.
			slog.Debug("received output (discarded)", "payload", string(msg.Payload))

		case proto.MsgIdle:
			gate.HandleEnforcement(ctx, conn)

		case proto.MsgExited:
			var p proto.ExitedPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				slog.Warn("bad exited payload", "error", err)
				return
			}
			slog.Info("client exited", "code", p.Code, "signal", p.Signal)
			return

		case proto.MsgStarted:
			var p proto.StartedPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				slog.Warn("bad started payload", "error", err)
				continue
			}
			slog.Info("client started", "client", p.Client, "pid", p.Pid)

		default:
			slog.Warn("unknown message", "type", msg.Type)
		}
	}
}
