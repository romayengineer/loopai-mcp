package main

import (
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"time"
)

const (
	readBufSize                 = 65536
	captureDirMode  os.FileMode = 0755
	captureFileMode os.FileMode = 0644
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

	var (
		mu       sync.Mutex
		captured []byte
		wg       sync.WaitGroup
	)

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, readBufSize)
		for {
			n, err := stdout.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				mu.Lock()
				captured = append(captured, chunk...)
				mu.Unlock()
				if _, wErr := os.Stdout.Write(chunk); wErr != nil {
					slog.Warn("stdout write", "error", wErr)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	wg.Add(1)
	go func() {
		defer wg.Done()
		buf := make([]byte, readBufSize)
		for {
			n, err := stderr.Read(buf)
			if n > 0 {
				chunk := make([]byte, n)
				copy(chunk, buf[:n])
				mu.Lock()
				captured = append(captured, chunk...)
				mu.Unlock()
				if _, wErr := os.Stderr.Write(chunk); wErr != nil {
					slog.Warn("stderr write", "error", wErr)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(*duration):
		if err := cmd.Process.Kill(); err != nil {
			slog.Warn("kill process", "error", err)
		}
		wg.Wait()
	}

	if err := cmd.Wait(); err != nil {
		slog.Warn("process wait", "error", err)
	}

	outPath := *outputFile
	if outPath == "" {
		if err := os.MkdirAll("testdata", captureDirMode); err != nil {
			slog.Error("create testdata dir", "error", err)
			os.Exit(1)
		}
		outPath = fmt.Sprintf("testdata/capture-%d.raw", time.Now().Unix())
	}

	if err := os.MkdirAll(filepath.Dir(outPath), captureDirMode); err != nil {
		slog.Error("create output dir", "error", err)
		os.Exit(1)
	}
	if err := os.WriteFile(outPath, captured, captureFileMode); err != nil {
		slog.Error("write capture", "error", err)
		os.Exit(1)
	}
	// Verify the file was written
	if info, err := os.Stat(outPath); err != nil {
		slog.Warn("stat capture file", "error", err)
	} else {
		slog.Info("captured output", "file", outPath, "bytes", info.Size())
	}
}
