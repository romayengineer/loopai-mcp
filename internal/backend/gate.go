package backend

import (
	"context"
	"log/slog"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const enforceCooldown = 30 * time.Second

// PromptRenderer renders prompt templates. Decouples the enforcement
// state machine from the template loading and rendering implementation.
type PromptRenderer interface {
	Render(name string, vars PromptVars) string
}

// Gate manages the enforcement loop: running tools and sending prompts.
type Gate struct {
	prompts     PromptRenderer
	lastEnforce time.Time
}

// NewGate creates a Gate with the given prompt renderer.
func NewGate(prompts PromptRenderer) *Gate {
	return &Gate{
		prompts: prompts,
	}
}

// HandleEnforcement runs the enforcement tools (build → lint → test) and sends
// a prompt to the launcher with the result. If all tools pass, a best-practices
// prompt is sent. If any tool fails, a failure prompt with the error output is
// sent on the first idle event, and the message is repeated on subsequent idles.
func (g *Gate) HandleEnforcement(ctx context.Context, conn LauncherConn) {
	if time.Since(g.lastEnforce) < enforceCooldown {
		slog.Debug("enforcement suppressed by cooldown", "since", time.Since(g.lastEnforce))
		return
	}
	g.lastEnforce = time.Now()

	slog.Info("running enforcement tools")
	results := RunAll()

	var promptName string
	vars := PromptVars{
		FailedTool: "",
		Output:     "",
	}

	if allPassed(results) {
		promptName = "idle"
		slog.Info("all enforcement tools passed")
	} else {
		// Find the first failure
		for _, r := range results {
			if !r.Passed {
				promptName = "failure"
				vars.FailedTool = r.Name
				vars.Output = r.Output
				slog.Info("enforcement failed", "tool", r.Name, "output_len", len(r.Output))
				break
			}
		}
	}

	text := g.prompts.Render(promptName, vars)
	slog.Debug("sending prompt", "name", promptName, "length", len(text))
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

func allPassed(results []ToolResult) bool {
	for _, r := range results {
		if !r.Passed {
			return false
		}
	}
	return true
}
