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

// MessageSender can send a proto.Message to the backend.
type MessageSender interface {
	Send(context.Context, proto.Message) error
}

// MessageReceiver can receive a proto.Message from the backend.
type MessageReceiver interface {
	Receive(context.Context) (proto.Message, error)
}

// Resetter can reset an idle timeout or similar timer.
type Resetter interface {
	Reset()
}

// PtyWriter is a PTY that can be written to and signaled.
type PtyWriter interface {
	io.Writer
	Signal(os.Signal) error
}

// PipePTYToBackend reads PTY output and forwards it to the backend
// over the Unix socket. Idle is reset on each chunk of output.
func PipePTYToBackend(ctx context.Context, sender MessageSender, r io.Reader, resetter Resetter) error {
	buf := make([]byte, readBufSize)
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		n, err := r.Read(buf)
		if n > 0 {
			resetter.Reset()
			data := make([]byte, n)
			copy(data, buf[:n])
			preview := string(data)
			if len(preview) > 120 {
				preview = preview[:120] + "..."
			}
			slog.Debug("output from PTY", "bytes", n, "preview", preview)
			msg, err := proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: data})
			if err != nil {
				return fmt.Errorf("create output message: %w", err)
			}
			if err := sender.Send(ctx, msg); err != nil {
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
func PipeBackendToPTY(ctx context.Context, receiver MessageReceiver, proc PtyWriter) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		msg, err := receiver.Receive(ctx)
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
			// Write the prompt text (no trailing newline — the client's TUI
			// would display it as literal text rather than submitting it).
			if _, err := proc.Write([]byte(p.Text)); err != nil {
				return fmt.Errorf("write prompt to PTY: %w", err)
			}
			// Send carriage return to simulate Enter keypress. In raw terminal
			// mode the Enter key produces \r, not \n. The client's TUI (Claude
			// Code, etc.) recognizes \r as the "submit input" command.
			if _, err := proc.Write([]byte("\r")); err != nil {
				return fmt.Errorf("write enter to PTY: %w", err)
			}
			slog.Debug("typed into PTY", "text", p.Text, "len", len(p.Text))

		case proto.MsgCtrlC:
			slog.Debug("sending ctrl+c")
			if err := proc.Signal(os.Interrupt); err != nil {
				slog.Warn("ctrl+c error", "error", err)
			}

		case proto.MsgShutdown:
			slog.Debug("received shutdown")
			return nil

		default:
			slog.Warn("unknown message from backend", "type", msg.Type)
		}
	}
}
