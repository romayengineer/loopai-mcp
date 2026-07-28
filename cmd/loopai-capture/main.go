package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"time"
)

func main() {
	client := flag.String("client", "claude", "client binary to capture")
	args := flag.String("args", "-p \"list files\"", "client arguments")
	duration := flag.Duration("duration", 15*time.Second, "capture duration")
	outputFile := flag.String("output", "", "output file (default: auto-generated)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelInfo})))

	slog.Info("starting capture", "client", *client, "args", *args, "duration", *duration)

	binary, err := exec.LookPath(*client)
	if err != nil {
		slog.Error("client not found", "error", err)
		os.Exit(1)
	}

	cmd := exec.Command(binary, *args)
	cmd.Env = append(os.Environ(),
		"TERM=xterm-256color",
		"CLAUDE_CODE_AUTO_MODE=1",
	)

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.Error("stdout pipe", "error", err)
		os.Exit(1)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		slog.Error("stderr pipe", "error", err)
		os.Exit(1)
	}

	if err := cmd.Start(); err != nil {
		slog.Error("start", "error", err)
		os.Exit(1)
	}

	var captured []byte
	buf := make([]byte, 65536)
	done := make(chan struct{})

	go func() {
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				captured = append(captured, buf[:n]...)
				os.Stdout.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
		close(done)
	}()

	go func() {
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				captured = append(captured, buf[:n]...)
				os.Stderr.Write(buf[:n])
			}
			if err != nil {
				break
			}
		}
	}()

	select {
	case <-done:
	case <-time.After(*duration):
		cmd.Process.Kill()
	}

	cmd.Wait()

	outPath := *outputFile
	if outPath == "" {
		_ = os.MkdirAll("testdata", 0755)
		outPath = fmt.Sprintf("testdata/capture-%d.raw", time.Now().Unix())
	}
	// Make path relative to repo root
	fullPath := outPath
	if !filepath.IsAbs(outPath) {
		fullPath = filepath.Join("/Users/macbookpro/Projects/romayengineer/loopai-mcp", outPath)
	}
	os.MkdirAll(filepath.Dir(fullPath), 0755)
	if err := os.WriteFile(fullPath, captured, 0644); err != nil {
		slog.Error("write capture", "error", err)
		os.Exit(1)
	}
	slog.Info("captured output", "file", outPath, "bytes", len(captured))
}
