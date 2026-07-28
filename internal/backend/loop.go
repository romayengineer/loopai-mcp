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

// HandleLauncher drives the enforcement loop for a single launcher connection.
// It reads output/idle/exited messages, runs phase detection, and sends fix
// prompts to advance through the compile → lint → test enforcement gates.
//
// The handler exits when the client exits (MsgExited) or disconnects,
// ensuring proper cleanup via defer conn.Close().
func HandleLauncher(ctx context.Context, conn LauncherConn) {
	defer conn.Close()
	gate := NewGate(NewOutputBuffer(), NewPromptLoader(DefaultPromptsDir))

	for {
		select {
		case <-ctx.Done():
			slog.Debug("handler context cancelled")
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
				// Log and continue for malformed output; don't break the loop.
				// Output is informational and a parse error shouldn't stop enforcement.
				slog.Warn("malformed output payload, skipping", "error", err)
				continue
			}
			gate.handleOutput(p.Data)

		case proto.MsgIdle:
			gate.handleIdle(ctx, conn)

		case proto.MsgStarted:
			var p proto.StartedPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				// Log and continue for malformed startup message.
				slog.Warn("malformed started payload, skipping", "error", err)
				continue
			}
			slog.Info("client started", "client", p.Client, "pid", p.Pid)

		case proto.MsgExited:
			var p proto.ExitedPayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				// Exit immediately on malformed exit message to prevent indefinite wait.
				// The exit signal is critical and should not be skipped.
				slog.Warn("malformed exited payload, exiting handler", "error", err)
				return
			}
			slog.Info("client exited", "code", p.Code, "signal", p.Signal)
			return

		default:
			slog.Warn("unknown message type", "type", msg.Type)
		}
	}
}
