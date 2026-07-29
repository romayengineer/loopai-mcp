package backend

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const enforceCooldown = 30 * time.Second

// PromptRenderer renders prompt templates. Decouples the enforcement
// state machine from the template loading and rendering implementation.
type PromptRenderer interface {
	Render(name string, vars PromptVars) string
}

// Runner runs the enforcement tools. Decouples Gate from the concrete
// tool execution, enabling tests to inject a mock runner.
type Runner interface {
	RunAll() []ToolResult
}

// Gate manages the enforcement loop: running tools and sending prompts.
type Gate struct {
	mu          sync.Mutex
	prompts     PromptRenderer
	runner      Runner
	lastEnforce time.Time
	running     bool
}

// NewGate creates a Gate with the given prompt renderer and runner.
func NewGate(prompts PromptRenderer, runner Runner) *Gate {
	return &Gate{
		prompts: prompts,
		runner:  runner,
	}
}

// HandleEnforcement runs the enforcement tools (build → lint → test) and sends
// a prompt to the launcher with the result. If all tools pass, a best-practices
// prompt is sent. If any tool fails, a failure prompt with the error output is
// sent on the first idle event, and the message is repeated on subsequent idles.
func (g *Gate) HandleEnforcement(ctx context.Context, conn LauncherConn) {
	g.mu.Lock()
	if g.running {
		slog.Debug("enforcement skipped — already running")
		g.mu.Unlock()
		return
	}
	if time.Since(g.lastEnforce) < enforceCooldown {
		slog.Debug("enforcement suppressed by cooldown", "since", time.Since(g.lastEnforce))
		g.mu.Unlock()
		return
	}
	g.running = true
	g.mu.Unlock()

	slog.Info("running enforcement tools")
	results := g.runner.RunAll()

	g.mu.Lock()
	g.lastEnforce = time.Now()
	g.running = false
	g.mu.Unlock()

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
