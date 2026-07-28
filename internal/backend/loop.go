package backend

import (
	"context"
	"encoding/json"
	"log/slog"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const (
	promptCompileFail = "The last compile attempt failed. Fix the errors above and re-run the build."
	promptLintFail    = "The last lint check found issues. Fix them and re-run the linter."
	promptTestFail    = "The last test run had failures. Fix them and re-run the tests."
)

// Gate tracks the enforcement state machine across compile/lint/test phases.
type Gate struct {
	output      *OutputBuffer
	pendingLint bool
	pendingTest bool
}

// NewGate creates a Gate with a fresh output buffer.
func NewGate() *Gate {
	return &Gate{
		output: NewOutputBuffer(),
	}
}

// HandleLauncher drives the enforcement loop for a single launcher
// connection: reads output/idle/exited messages, runs phase detection,
// and sends type prompts to advance through compile → lint → test.
func HandleLauncher(ctx context.Context, pc *proto.Conn) {
	defer pc.Close()
	gate := NewGate()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msg, err := pc.Receive(ctx)
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
			gate.handleIdle(ctx, pc)

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

func (g *Gate) handleOutput(data []byte) {
	g.output.Write(data)
}

func (g *Gate) handleIdle(ctx context.Context, pc *proto.Conn) {
	result := g.output.Analyze()
	phase := result.Phase
	res := result.Result
	g.output.Reset()

	slog.Debug("idle analysis",
		"phase", phase,
		"result", res,
	)

	send := func(text string) {
		msg, mErr := proto.NewMessage(proto.MsgType, proto.TypePayload{
			Text: text,
		})
		if mErr != nil {
			slog.Warn("marshal message", "error", mErr)
			return
		}
		if err := pc.Send(ctx, msg); err != nil {
			slog.Warn("send failed", "error", err)
		}
	}

	switch phase {
	case PhaseCompile:
		switch res {
		case ResultSuccess:
			slog.Info("compile passed, next: lint")
			send("Build succeeded. Now run the linter (golangci-lint run ./...).")
		case ResultFailure:
			slog.Info("compile failed, prompting fix")
			send(promptCompileFail)
		}

	case PhaseLint:
		switch res {
		case ResultSuccess:
			slog.Info("lint passed, next: test")
			send("Linting passed. Now run the tests (go test ./...).")
		case ResultFailure:
			slog.Info("lint failed, prompting fix")
			send(promptLintFail)
		}

	case PhaseTest:
		switch res {
		case ResultSuccess:
			slog.Info("all gates passed")
			send("All checks passed. The task is complete.")
		case ResultFailure:
			slog.Info("tests failed, prompting fix")
			send(promptTestFail)
		}

	case PhaseUnknown:
		slog.Debug("no phase detected on idle, no action")
	}
}
