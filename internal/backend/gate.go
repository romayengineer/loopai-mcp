package backend

import (
	"context"
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
	output OutputAnalyzer
}

// NewGate creates a Gate with the given output analyzer.
func NewGate(analyzer OutputAnalyzer) *Gate {
	return &Gate{
		output: analyzer,
	}
}

func (g *Gate) handleOutput(data []byte) {
	g.output.Write(data)
}

func (g *Gate) handleIdle(ctx context.Context, conn LauncherConn) {
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
		if err := conn.Send(ctx, msg); err != nil {
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
