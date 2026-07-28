//go:build integration

package launcher

import (
	"context"
	"encoding/json"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/backend"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const (
	startupTimeout = 200 * time.Millisecond
	settleTimeout  = 100 * time.Millisecond
)

func e2eSocketPath(t *testing.T, name string) string {
	t.Helper()
	path := "/tmp/loopai-e2e-" + name
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func startE2EBackend(t *testing.T, socketPath string, handler func(context.Context, backend.LauncherConn)) *backend.Backend {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	b := backend.New(socketPath, handler)
	go func() {
		if err := b.Run(ctx); err != nil {
			t.Logf("backend exited: %v", err)
		}
	}()
	t.Cleanup(func() { cancel(); b.Stop() })
	time.Sleep(startupTimeout)
	return b
}

func TestEndToEndEcho(t *testing.T) {
	sp := e2eSocketPath(t, "echo.sock")

	var (
		gotOutput  bool
		gotExited  bool
		exitCode   int = -99
		mu         sync.Mutex
		outputData string
	)

	handler := func(ctx context.Context, conn backend.LauncherConn) {
		defer conn.Close()
		for {
			msg, err := conn.Receive(ctx)
			if err != nil {
				return
			}
			mu.Lock()
			switch msg.Type {
			case proto.MsgOutput:
				var p proto.OutputPayload
				if err := json.Unmarshal(msg.Payload, &p); err == nil {
					outputData += string(p.Data)
					gotOutput = true
				}
			case proto.MsgIdle:
				reply, mErr := proto.NewMessage(proto.MsgType, proto.TypePayload{Text: "done"})
				if mErr != nil {
					mu.Unlock()
					return
				}
				if err := conn.Send(ctx, reply); err != nil {
					mu.Unlock()
					return
				}
			case proto.MsgExited:
				var p proto.ExitedPayload
				if err := json.Unmarshal(msg.Payload, &p); err == nil {
					exitCode = p.Code
					gotExited = true
				}
				mu.Unlock()
				return
			}
			mu.Unlock()
		}
	}

	startE2EBackend(t, sp, handler)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()
	pc := proto.NewConn(conn)

	proc, err := Spawn("echo", []string{"hello world"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer func() {
		if err := proc.Close(); err != nil {
			t.Errorf("close proc: %v", err)
		}
	}()

	ctx := context.Background()

	startMsg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid: proc.PID(), Client: "echo",
	})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, startMsg); err != nil {
		t.Fatalf("send started: %v", err)
	}

	idle := NewIdleDetector(2*time.Second, func() {
		idleMsg, mErr := proto.NewMessage(proto.MsgIdle, proto.IdlePayload{})
		if mErr != nil {
			t.Logf("marshal idle: %v", mErr)
			return
		}
		if err := pc.Send(ctx, idleMsg); err != nil {
			t.Logf("send idle: %v", err)
		}
	})
	idle.Start()

	errCh := make(chan error, 2)
	go func() { errCh <- PipePTYToBackend(ctx, pc, proc, idle) }()
	go func() { errCh <- PipeBackendToPTY(ctx, pc, proc) }()

	<-errCh
	idle.Stop()

	<-proc.Wait()
	actualCode := proc.ExitCode()

	exitMsg, err := proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: actualCode})
	if err != nil {
		t.Logf("marshal exited: %v", err)
	} else if err := pc.Send(ctx, exitMsg); err != nil {
		t.Logf("send exited: %v", err)
	}

	time.Sleep(settleTimeout)

	mu.Lock()
	defer mu.Unlock()

	if !gotOutput {
		t.Fatal("backend never received output")
	}
	if outputData != "hello world\r\n" && outputData != "hello world\n" {
		t.Fatalf("unexpected output: %q", outputData)
	}
	if !gotExited {
		t.Fatal("backend never received exited message")
	}
	if exitCode != 0 {
		t.Fatalf("exit code: expected 0, got %d", exitCode)
	}
	if actualCode != 0 {
		t.Fatalf("actual exit code: expected 0, got %d", actualCode)
	}
}

func TestEndToEndExitCode(t *testing.T) {
	sp := e2eSocketPath(t, "exit.sock")

	var (
		exitCode int = -99
		mu       sync.Mutex
	)

	handler := func(ctx context.Context, conn backend.LauncherConn) {
		defer conn.Close()
		for {
			msg, err := conn.Receive(ctx)
			if err != nil {
				return
			}
			mu.Lock()
			if msg.Type == proto.MsgExited {
				var p proto.ExitedPayload
				if err := json.Unmarshal(msg.Payload, &p); err == nil {
					exitCode = p.Code
				}
				mu.Unlock()
				return
			}
			mu.Unlock()
		}
	}

	startE2EBackend(t, sp, handler)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()
	pc := proto.NewConn(conn)
	ctx := context.Background()

	proc, err := Spawn("sh", []string{"-c", "exit 42"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()

	startMsg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid: proc.PID(), Client: "sh",
	})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, startMsg); err != nil {
		t.Fatalf("send started: %v", err)
	}

	idle := NewIdleDetector(5*time.Second, func() {})
	idle.Start()

	errCh := make(chan error, 2)
	go func() { errCh <- PipePTYToBackend(ctx, pc, proc, idle) }()
	go func() { errCh <- PipeBackendToPTY(ctx, pc, proc) }()

	<-errCh
	idle.Stop()
	<-proc.Wait()

	code := proc.ExitCode()
	exitMsg, err := proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: code})
	if err != nil {
		t.Logf("marshal exited: %v", err)
	} else if err := pc.Send(ctx, exitMsg); err != nil {
		t.Logf("send exited: %v", err)
	}

	time.Sleep(settleTimeout)

	mu.Lock()
	defer mu.Unlock()
	if code != 42 {
		t.Fatalf("expected exit code 42, got %d", code)
	}
	if exitCode != 42 {
		t.Fatalf("backend received exit code: expected 42, got %d", exitCode)
	}
}

func TestEndToEndOutputStreaming(t *testing.T) {
	sp := e2eSocketPath(t, "stream.sock")

	var outputChunks []string
	var mu sync.Mutex

	handler := func(ctx context.Context, conn backend.LauncherConn) {
		defer conn.Close()
		for {
			msg, err := conn.Receive(ctx)
			if err != nil {
				return
			}
			mu.Lock()
			switch msg.Type {
			case proto.MsgOutput:
				var p proto.OutputPayload
				if err := json.Unmarshal(msg.Payload, &p); err == nil {
					outputChunks = append(outputChunks, string(p.Data))
				}
			case proto.MsgExited:
				mu.Unlock()
				return
			}
			mu.Unlock()
		}
	}

	startE2EBackend(t, sp, handler)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()
	pc := proto.NewConn(conn)
	ctx := context.Background()

	proc, err := Spawn("sh", []string{"-c", "echo line1 && echo line2 && echo line3"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()

	startMsg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid: proc.PID(), Client: "sh",
	})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, startMsg); err != nil {
		t.Fatalf("send started: %v", err)
	}

	idle := NewIdleDetector(5*time.Second, func() {})
	idle.Start()

	errCh := make(chan error, 2)
	go func() { errCh <- PipePTYToBackend(ctx, pc, proc, idle) }()
	go func() { errCh <- PipeBackendToPTY(ctx, pc, proc) }()

	<-errCh
	idle.Stop()
	<-proc.Wait()

	code := proc.ExitCode()
	exitMsg, err := proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: code})
	if err != nil {
		t.Logf("marshal exited: %v", err)
	} else if err := pc.Send(ctx, exitMsg); err != nil {
		t.Logf("send exited: %v", err)
	}

	time.Sleep(settleTimeout)

	mu.Lock()
	defer mu.Unlock()

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}
	if len(outputChunks) == 0 {
		t.Fatal("expected at least one output chunk")
	}

	var fullOutput string
	for _, chunk := range outputChunks {
		fullOutput += chunk
	}
	if len(fullOutput) < 20 {
		t.Fatalf("expected substantial output, got %q", fullOutput)
	}
}
