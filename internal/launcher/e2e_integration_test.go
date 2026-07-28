//go:build integration

package launcher

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/backend"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func TestEndToEndEcho(t *testing.T) {
	socketPath := filepath.Join("/tmp", "loopai-e2e-echo.sock")
	os.Remove(socketPath)
	t.Cleanup(func() { os.Remove(socketPath) })

	var (
		gotOutput  bool
		gotExited  bool
		exitCode   int = -99
		mu         sync.Mutex
		outputData string
	)

	handler := func(pc *proto.Conn) {
		defer pc.Close()
		for {
			msg, err := pc.Receive()
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
			case proto.MsgStarted:
				var p proto.StartedPayload
				json.Unmarshal(msg.Payload, &p)
			case proto.MsgIdle:
				pc.Send(proto.NewMessage(proto.MsgType, proto.TypePayload{Text: "done"}))
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

	b := backend.New(socketPath, handler)
	go func() {
		if err := b.Run(); err != nil {
			t.Errorf("backend: %v", err)
		}
	}()
	time.Sleep(200 * time.Millisecond)

	conn, err := proto.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	defer conn.Close()
	pc := proto.NewConn(conn)

	proc, err := Spawn("echo", []string{"hello world"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()

	pc.Send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid:    proc.Cmd.Process.Pid,
		Client: "echo",
	}))

	idle := NewIdleDetector(2*time.Second, func() {
		pc.Send(proto.NewMessage(proto.MsgIdle, proto.IdlePayload{}))
	})
	idle.Start()

	errCh := make(chan error, 2)
	go func() { errCh <- PipePTYToBackend(pc, proc, idle) }()
	go func() { errCh <- PipeBackendToPTY(pc, proc) }()

	<-errCh
	idle.Stop()

	<-proc.Wait()
	actualCode := proc.ExitCode()
	pc.Send(proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: actualCode}))

	time.Sleep(100 * time.Millisecond)

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
	socketPath := filepath.Join("/tmp", "loopai-e2e-exit.sock")
	os.Remove(socketPath)
	t.Cleanup(func() { os.Remove(socketPath) })

	var (
		exitCode int = -99
		mu       sync.Mutex
	)

	handler := func(pc *proto.Conn) {
		defer pc.Close()
		for {
			msg, err := pc.Receive()
			if err != nil {
				return
			}
			mu.Lock()
			if msg.Type == proto.MsgExited {
				var p proto.ExitedPayload
				json.Unmarshal(msg.Payload, &p)
				exitCode = p.Code
				mu.Unlock()
				return
			}
			mu.Unlock()
		}
	}

	b := backend.New(socketPath, handler)
	go b.Run()
	time.Sleep(200 * time.Millisecond)

	conn, err := proto.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	pc := proto.NewConn(conn)

	proc, err := Spawn("sh", []string{"-c", "exit 42"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()
	defer conn.Close()

	pc.Send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid: proc.Cmd.Process.Pid, Client: "sh",
	}))

	idle := NewIdleDetector(5*time.Second, func() {})
	idle.Start()

	errCh := make(chan error, 2)
	go func() { errCh <- PipePTYToBackend(pc, proc, idle) }()
	go func() { errCh <- PipeBackendToPTY(pc, proc) }()

	<-errCh
	idle.Stop()
	<-proc.Wait()

	code := proc.ExitCode()
	pc.Send(proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: code}))

	time.Sleep(100 * time.Millisecond)

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
	socketPath := filepath.Join("/tmp", "loopai-e2e-stream.sock")
	os.Remove(socketPath)
	t.Cleanup(func() { os.Remove(socketPath) })

	var outputChunks []string
	var mu sync.Mutex

	handler := func(pc *proto.Conn) {
		for {
			msg, err := pc.Receive()
			if err != nil {
				return
			}
			mu.Lock()
			if msg.Type == proto.MsgOutput {
				var p proto.OutputPayload
				json.Unmarshal(msg.Payload, &p)
				outputChunks = append(outputChunks, string(p.Data))
			}
			if msg.Type == proto.MsgExited {
				mu.Unlock()
				pc.Close()
				return
			}
			mu.Unlock()
		}
	}

	b := backend.New(socketPath, handler)
	go b.Run()
	time.Sleep(200 * time.Millisecond)

	conn, err := proto.Connect(socketPath)
	if err != nil {
		t.Fatalf("connect: %v", err)
	}
	pc := proto.NewConn(conn)

	proc, err := Spawn("sh", []string{"-c", "echo line1 && echo line2 && echo line3"})
	if err != nil {
		t.Fatalf("spawn: %v", err)
	}
	defer proc.Close()
	defer conn.Close()

	pc.Send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid: proc.Cmd.Process.Pid, Client: "sh",
	}))

	idle := NewIdleDetector(5*time.Second, func() {})
	idle.Start()

	errCh := make(chan error, 2)
	go func() { errCh <- PipePTYToBackend(pc, proc, idle) }()
	go func() { errCh <- PipeBackendToPTY(pc, proc) }()

	<-errCh
	idle.Stop()
	<-proc.Wait()

	code := proc.ExitCode()
	pc.Send(proto.NewMessage(proto.MsgExited, proto.ExitedPayload{Code: code}))

	time.Sleep(100 * time.Millisecond)

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
