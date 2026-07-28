package backend

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"text/template"
)

// PromptVars holds variables available to prompt templates.
type PromptVars struct {
	Phase    string // "compile", "lint", "test", "unknown"
	Result   string // "success", "failure"
	BufSize  int    // size of the output buffer being analyzed
	Errors   string // error lines extracted from output (for failure prompts)
	Output   string // full output since last idle (for idle prompt)
	BuildOut string // output produced during the build/lint/test step

	// PhaseAttempts is how many times this specific phase has failed
	// consecutively. Useful for detecting when the model is stuck.
	PhaseAttempts int
	// TotalAttempts is the sum of compile + lint + test failures across
	// the entire session. Useful for context-aware prompts.
	TotalAttempts int
}

// PromptLoader reads prompt templates from a directory. Each template is
// read from disk on every call, so edits take effect without a restart.
type PromptLoader struct {
	dir string
}

// NewPromptLoader creates a loader that reads templates from dir.
func NewPromptLoader(dir string) *PromptLoader {
	return &PromptLoader{dir: dir}
}

// Render reads the template file name.md from the prompts directory,
// renders it with the provided vars, and returns the result.
// If the file cannot be read or the template is invalid, it logs a warning
// and returns a fallback message containing just the template name.
func (p *PromptLoader) Render(name string, vars PromptVars) string {
	path := filepath.Join(p.dir, name+".md")
	data, err := os.ReadFile(path)
	if err != nil {
		slog.Warn("read prompt template", "name", name, "path", path, "error", err)
		return fmt.Sprintf("[prompt:%s]", name)
	}

	tmpl, err := template.New(name).Option("missingkey=error").Parse(string(data))
	if err != nil {
		slog.Warn("parse prompt template", "name", name, "error", err)
		return string(data)
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, vars); err != nil {
		slog.Warn("render prompt template", "name", name, "error", err)
		// Fallback: render with simple string replacement
		result := string(data)
		result = strings.ReplaceAll(result, "{{.Phase}}", vars.Phase)
		result = strings.ReplaceAll(result, "{{.Result}}", vars.Result)
		result = strings.ReplaceAll(result, "{{.Errors}}", vars.Errors)
		result = strings.ReplaceAll(result, "{{.Output}}", vars.Output)
		result = strings.ReplaceAll(result, "{{.BuildOut}}", vars.BuildOut)
		result = strings.ReplaceAll(result, "{{.PhaseAttempts}}", fmt.Sprintf("%d", vars.PhaseAttempts))
		result = strings.ReplaceAll(result, "{{.TotalAttempts}}", fmt.Sprintf("%d", vars.TotalAttempts))
		return result
	}
	return strings.TrimRight(buf.String(), "\n\r\t ")
}
