package backend

import (
	"context"
	"log/slog"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const idleCooldown = 30 * time.Second

// Gate tracks the enforcement state machine across compile/lint/test phases.
type Gate struct {
	output         OutputAnalyzer
	prompts        *PromptLoader
	lastIdlePrompt time.Time
}

// NewGate creates a Gate with the given output analyzer and prompt loader.
func NewGate(analyzer OutputAnalyzer, prompts *PromptLoader) *Gate {
	return &Gate{
		output:  analyzer,
		prompts: prompts,
	}
}

func (g *Gate) handleOutput(data []byte) {
	g.output.Write(data)
}

func (g *Gate) handleIdle(ctx context.Context, conn LauncherConn) {
	result := g.output.Analyze()
	phase := result.Phase
	res := result.Result
	rawOutput := g.output.String()
	g.output.Reset()

	vars := PromptVars{
		Phase:   phase.String(),
		Result:  res.String(),
		BufSize: len(rawOutput),
		Output:  rawOutput,
		Errors:  rawOutput,
	}

	slog.Debug("idle analysis",
		"phase", vars.Phase,
		"result", vars.Result,
		"buf_size", vars.BufSize,
	)

	send := func(name string) {
		text := g.prompts.Render(name, vars)
		slog.Debug("sending prompt", "name", name, "length", len(text))
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
			slog.Info("compile passed, next: lint", "buf_size", vars.BufSize)
			send("compile-pass")
		case ResultFailure:
			slog.Info("compile failed, prompting fix", "buf_size", vars.BufSize)
			send("compile-fail")
		}

	case PhaseLint:
		switch res {
		case ResultSuccess:
			slog.Info("lint passed, next: test", "buf_size", vars.BufSize)
			send("lint-pass")
		case ResultFailure:
			slog.Info("lint failed, prompting fix", "buf_size", vars.BufSize)
			send("lint-fail")
		}

	case PhaseTest:
		switch res {
		case ResultSuccess:
			slog.Info("all gates passed", "buf_size", vars.BufSize)
			send("test-pass")
		case ResultFailure:
			slog.Info("tests failed, prompting fix", "buf_size", vars.BufSize)
			send("test-fail")
		}

	case PhaseUnknown:
		if len(rawOutput) > 0 && time.Since(g.lastIdlePrompt) >= idleCooldown {
			g.lastIdlePrompt = time.Now()
			slog.Info("idle output with no phase detected, sending best-practices prompt", "buf_size", vars.BufSize)
			send("idle")
		} else {
			slog.Debug("no phase detected on idle, no action")
		}
	}
}
