package launcher

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"os"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const readBufSize = 65536

// PipePTYToBackend reads PTY output and forwards it to the backend
// over the Unix socket. Idle is reset on each chunk of output.
func PipePTYToBackend(ctx context.Context, pc *proto.Conn, proc *PtyProcess, idle *IdleDetector) error {
	buf := make([]byte, readBufSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := proc.Read(buf)
		if n > 0 {
			idle.Reset()
			data := make([]byte, n)
			copy(data, buf[:n])
			msg, err := proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: data})
			if err != nil {
				return fmt.Errorf("create output message: %w", err)
			}
			if err := pc.Send(ctx, msg); err != nil {
				return fmt.Errorf("send output to backend: %w", err)
			}
		}
		if err != nil {
			if err == io.EOF {
				return nil
			}
			return fmt.Errorf("read from PTY: %w", err)
		}
	}
}

// PipeBackendToPTY receives messages from the backend and writes them
// into the PTY (type prompts, Ctrl+C, shutdown).
func PipeBackendToPTY(ctx context.Context, pc *proto.Conn, proc *PtyProcess) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := pc.Receive(ctx)
		if err != nil {
			return fmt.Errorf("receive from backend: %w", err)
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
				return fmt.Errorf("write to PTY: %w", err)
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
