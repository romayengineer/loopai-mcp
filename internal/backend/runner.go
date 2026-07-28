package backend

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"time"
)

// CompileDir is the working directory for running enforcement tools.
// Set by cmd/loopai-backend before starting.
var CompileDir string

// MaxToolOutput is the maximum bytes of tool output to include in prompts.
// Set via the -tool-output-max flag.
var MaxToolOutput = 4096

// ToolResult holds the result of running a single enforcement tool.
type ToolResult struct {
	Name   string // "go build", "golangci-lint", "go test"
	Passed bool
	Output string // tool output, truncated to MaxToolOutput
}

// RunAll runs build → lint → test in sequence, stopping at the first failure.
// Each tool is run with a timeout. Returns the results of all tools that ran.
func RunAll() []ToolResult {
	var results []ToolResult

	r := runBuild()
	results = append(results, r)
	if !r.Passed {
		return results
	}

	r = runLint()
	results = append(results, r)
	if !r.Passed {
		return results
	}

	r = runTest()
	results = append(results, r)
	return results
}

func runTool(name, dir string, args []string, timeout time.Duration) ToolResult {
	slog.Debug("running tool", "tool", name, "dir", dir, "args", args, "timeout", timeout)

	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	defer cancel()

	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Dir = dir

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()

	combined := stdout.String()
	if stderr.Len() > 0 {
		if len(combined) > 0 {
			combined += "\n"
		}
		combined += stderr.String()
	}

	// Truncate output to MaxToolOutput
	if len(combined) > MaxToolOutput {
		combined = combined[:MaxToolOutput] + "\n... (truncated)"
	}

	passed := err == nil
	if ctx.Err() != nil {
		combined = fmt.Sprintf("tool timed out after %v\n%s", timeout, combined)
		passed = false
	}

	slog.Debug("tool result",
		"tool", name,
		"passed", passed,
		"output_len", len(combined),
	)

	return ToolResult{
		Name:   name,
		Passed: passed,
		Output: combined,
	}
}

func runBuild() ToolResult {
	return runTool("go", CompileDir, []string{"build", "./..."}, 60*time.Second)
}

func runLint() ToolResult {
	return runTool("golangci-lint", CompileDir, []string{"run", "./..."}, 120*time.Second)
}

func runTest() ToolResult {
	return runTool("go", CompileDir, []string{"test", "./..."}, 120*time.Second)
}

func init() {
	var err error
	CompileDir, err = os.Getwd()
	if err != nil {
		CompileDir = "."
	}
}
