package backend

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

// DefaultPromptsDir is the directory containing prompt template files.
// Set before starting the backend to use a custom location.
var DefaultPromptsDir = "prompts"

// HandleLauncher drives the enforcement loop for a single launcher
// connection: reads output/idle/exited messages, runs phase detection,
// and sends type prompts to advance through compile → lint → test.
func HandleLauncher(ctx context.Context, conn LauncherConn) {
	defer conn.Close()
	gate := NewGate(NewOutputBuffer(), NewPromptLoader(DefaultPromptsDir))

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
			var p proto.OutputPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				slog.Warn("bad output payload", "error", err)
				continue
			}
			gate.handleOutput(p.Data)

		case proto.MsgIdle:
			gate.handleIdle(ctx, conn)

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
