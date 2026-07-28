package launcher

import (
	"encoding/json"
	"io"
	"log/slog"
	"os"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const readBufSize = 65536

func PipePTYToBackend(pc *proto.Conn, proc *PtyProcess, idle *IdleDetector) error {
	buf := make([]byte, readBufSize)
	for {
		n, err := proc.Read(buf)
		if n > 0 {
			idle.Reset()
			data := make([]byte, n)
			copy(data, buf[:n])
			msg := proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: data})
			if err := pc.Send(msg); err != nil {
				return err
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return err
		}
	}
}

func PipeBackendToPTY(pc *proto.Conn, proc *PtyProcess) error {
	for {
		msg, err := pc.Receive()
		if err != nil {
			return err
		}

		switch msg.Type {
		case proto.MsgType:
			var p proto.TypePayload
			if err := json.Unmarshal(msg.Payload, &p); err != nil {
				slog.Warn("bad type payload", "error", err)
				continue
			}
			input := []byte(p.Text + "\n")
			if _, err := proc.Write(input); err != nil {
				return err
			}

		case proto.MsgCtrlC:
			if err := proc.Signal(os.Interrupt); err != nil {
				slog.Warn("ctrl+c error", "error", err)
			}

		case proto.MsgShutdown:
			return nil

		default:
			slog.Warn("unknown message from backend", "type", msg.Type)
		}
	}
}
