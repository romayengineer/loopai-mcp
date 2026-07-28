package backend

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const defaultIdlePrompt = "list the directory contents"

func HandleLauncher(ctx context.Context, pc *proto.Conn) {
	defer pc.Close()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := pc.Receive()
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
			if _, err := os.Stdout.Write(p.Data); err != nil {
				slog.Error("write output", "error", err)
			}

		case proto.MsgIdle:
			slog.Info("client idle, sending prompt")
			if err := pc.Send(proto.NewMessage(proto.MsgType, proto.TypePayload{
				Text: defaultIdlePrompt,
			})); err != nil {
				slog.Warn("send idle prompt failed", "error", err)
				return
			}

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
