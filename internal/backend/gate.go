package backend

import (
	"context"
	"log/slog"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const idleCooldown = 30 * time.Second

// PromptRenderer renders prompt templates. Decouples the enforcement
// state machine from the template loading and rendering implementation.
type PromptRenderer interface {
	// Render reads and renders a prompt template by name, substituting vars.
	Render(name string, vars PromptVars) string
}

// Gate tracks the enforcement state machine across compile/lint/test phases.
type Gate struct {
	output         OutputAnalyzer
	prompts        PromptRenderer
	lastIdlePrompt time.Time
	lastSend       map[string]time.Time // per-prompt-name cooldown
	attempts       map[Phase]int        // per-phase attempt counter (for detecting loops)
	promptCooldown time.Duration        // min interval between same prompt; 0 = default
}

// defaultPromptCooldown is the minimum interval between sending the same
// prompt name. Prevents rapid-fire prompt injection when idle fires frequently.
const defaultPromptCooldown = 5 * time.Second

// NewGate creates a Gate with the given output analyzer and prompt renderer.
func NewGate(analyzer OutputAnalyzer, prompts PromptRenderer) *Gate {
	return &Gate{
		output:         analyzer,
		prompts:        prompts,
		lastSend:       make(map[string]time.Time),
		attempts:       make(map[Phase]int),
		promptCooldown: defaultPromptCooldown,
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

	// Track phase attempts. Increment on failure so the prompt template can
	// detect when the model is stuck in a loop. Reset on success so the
	// counter starts fresh when a phase passes and later fails again.
	if phase != PhaseUnknown {
		switch res {
		case ResultFailure:
			g.attempts[phase]++
		case ResultSuccess:
			g.attempts[phase] = 0
		}
	} else if len(rawOutput) > 0 {
		// Count unknown-phase idle events as context for the "idle" prompt.
		g.attempts[PhaseUnknown]++
	}

	// Extract only the error lines from the output.
	errLines := extractErrorLinesMax(rawOutput, phase, defaultMaxErrorBytes)

	// Cap the full output to prevent multi-megabyte payloads being passed
	// to prompt templates (which would slow rendering and waste bandwidth).
	outputCap := rawOutput
	if len(outputCap) > defaultMaxErrorBytes {
		outputCap = outputCap[:defaultMaxErrorBytes]
	}

	vars := PromptVars{
		Phase:         phase.String(),
		Result:        res.String(),
		BufSize:       len(rawOutput),
		Output:        outputCap,
		Errors:        errLines,
		PhaseAttempts: g.attempts[phase],
		TotalAttempts: g.attempts[PhaseCompile] + g.attempts[PhaseLint] + g.attempts[PhaseTest],
	}

	slog.Debug("idle analysis",
		"phase", vars.Phase,
		"result", vars.Result,
		"buf_size", vars.BufSize,
		"errors_size", len(errLines),
		"phase_attempts", vars.PhaseAttempts,
		"total_attempts", vars.TotalAttempts,
	)

	send := func(name string) {
		// Per-prompt cooldown: skip if we sent the same prompt recently.
		// Prevents rapid-fire prompt injection when idle fires frequently.
		if last, ok := g.lastSend[name]; ok && time.Since(last) < g.promptCooldown {
			slog.Debug("prompt suppressed by cooldown", "name", name, "since", time.Since(last))
			return
		}
		g.lastSend[name] = time.Now()

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
			slog.Info("compile failed, prompting fix",
				"buf_size", vars.BufSize,
				"errors_size", len(errLines),
			)
			send("compile-fail")
		}

	case PhaseLint:
		switch res {
		case ResultSuccess:
			slog.Info("lint passed, next: test", "buf_size", vars.BufSize)
			send("lint-pass")
		case ResultFailure:
			slog.Info("lint failed, prompting fix",
				"buf_size", vars.BufSize,
				"errors_size", len(errLines),
			)
			send("lint-fail")
		}

	case PhaseTest:
		switch res {
		case ResultSuccess:
			slog.Info("all gates passed", "buf_size", vars.BufSize)
			send("test-pass")
		case ResultFailure:
			slog.Info("tests failed, prompting fix",
				"buf_size", vars.BufSize,
				"errors_size", len(errLines),
			)
			send("test-fail")
		}

	case PhaseUnknown:
		if len(rawOutput) > 0 && time.Since(g.lastIdlePrompt) >= idleCooldown {
			g.lastIdlePrompt = time.Now()
			slog.Info("idle output with no phase detected, sending best-practices prompt",
				"buf_size", vars.BufSize,
			)
			send("idle")
		} else {
			slog.Debug("no phase detected on idle, no action")
		}
	}
}
