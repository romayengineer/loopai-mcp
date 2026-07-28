package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"text/template"

	"github.com/romayengineer/loopai-mcp/internal/backend"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func main() {
	socketPath := flag.String("socket", proto.DefaultSocketPath(), "unix socket path")
	promptsDir := flag.String("prompts-dir", "prompts", "prompt template directory")
	logLevel := flag.String("log-level", "info", "log level (debug, info, warn, error)")
	flag.Parse()

	proto.SetLogDefault(proto.ParseLogLevel(*logLevel))

	if flag.NArg() > 0 && flag.Arg(0) == "stop" {
		if err := stopBackend(*socketPath); err != nil {
			slog.Error("stop", "error", err)
			os.Exit(1)
		}
		return
	}

	// Validate prompts directory exists and is readable before starting backend
	if err := validatePromptsDir(*promptsDir); err != nil {
		slog.Error("prompts directory validation", "path", *promptsDir, "error", err)
		os.Exit(1)
	}

	if err := checkAndWritePID(*socketPath); err != nil {
		slog.Error("pid check", "error", err)
		os.Exit(1)
	}
	defer os.Remove(filepath.Join(filepath.Dir(*socketPath), "loopai.pid"))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	backend.DefaultPromptsDir = *promptsDir
	b := backend.New(*socketPath, backend.HandleLauncher)

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig)
		cancel()
		b.Stop()
	}()

	if err := b.Run(ctx); err != nil {
		slog.Error("backend", "error", err)
		os.Exit(1)
	}
}

// expectedTemplates lists the prompt template files required for operation.
// Each is validated at startup to catch template syntax errors early.
var expectedTemplates = []string{
	"compile-fail.md", "compile-pass.md",
	"lint-fail.md", "lint-pass.md",
	"test-fail.md", "test-pass.md",
	"idle.md",
}

// validatePromptsDir checks that the prompts directory exists, is readable,
// and that all expected template files are parseable Go templates.
// This prevents startup failures when enforcing phases.
func validatePromptsDir(dir string) error {
	info, err := os.Stat(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("prompts directory not found: %s (create it or use -prompts-dir flag)", dir)
		}
		return fmt.Errorf("cannot access prompts directory: %w", err)
	}

	if !info.IsDir() {
		return fmt.Errorf("prompts path is not a directory: %s", dir)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return fmt.Errorf("cannot read prompts directory: %w", err)
	}

	// Build a set of existing files for existence check
	existing := make(map[string]bool, len(entries))
	for _, e := range entries {
		existing[e.Name()] = true
	}

	// Validate each expected template exists and is a valid Go template.
	for _, name := range expectedTemplates {
		if !existing[name] {
			slog.Warn("expected prompt template not found", "name", name, "path", dir)
			continue // non-fatal — the PromptLoader will return a fallback
		}
		path := filepath.Join(dir, name)
		data, err := os.ReadFile(path)
		if err != nil {
			slog.Warn("cannot read prompt template", "name", name, "error", err)
			continue
		}
		if _, err := template.New(name).Option("missingkey=error").Parse(string(data)); err != nil {
			return fmt.Errorf("invalid template %s: %w", path, err)
		}
	}

	slog.Debug("prompts directory validated", "path", dir, "templates", len(expectedTemplates))
	return nil
}
