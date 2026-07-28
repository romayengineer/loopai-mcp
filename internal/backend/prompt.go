package backend

import (
	"bytes"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"text/template"
)

// PromptVars holds variables available to prompt templates.
type PromptVars struct {
	FailedTool string // name of the tool that failed ("go build", etc.)
	Output     string // tool output (truncated)
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

// DefaultPromptsDir is the directory containing prompt template files.
var DefaultPromptsDir = "prompts"

// Render reads the template file name.md from the prompts directory,
// renders it with the provided vars, and returns the result.
// If the file cannot be read or the template is invalid, it logs a warning
// and returns a fallback message.
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
		return string(data)
	}
	return string(bytes.TrimRight(buf.Bytes(), "\n\r\t "))
}
