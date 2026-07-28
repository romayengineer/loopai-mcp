//go:build integration

package main

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func socketPath(t *testing.T, name string) string {
	t.Helper()
	path := "/tmp/loopai-cmd-test-" + name
	os.Remove(path)
	t.Cleanup(func() { os.Remove(path) })
	return path
}

func socketDir(t *testing.T, name string) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "loopai-cmd-test-"+name)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return dir
}

func TestPidCheckFresh(t *testing.T) {
	dir := socketDir(t, "pidfresh")
	sp := filepath.Join(dir, "loopai.sock")

	if err := checkAndWritePID(sp); err != nil {
		t.Fatalf("expected no error for fresh start, got: %v", err)
	}

	// Verify pid file was created
	pidPath := filepath.Join(dir, "loopai.pid")
	if _, err := os.Stat(pidPath); err != nil {
		t.Fatalf("expected pid file at %s: %v", pidPath, err)
	}

	// Cleanup for the next test
	os.Remove(pidPath)
}

func TestPidCheckStale(t *testing.T) {
	dir := socketDir(t, "pidstale")
	sp := filepath.Join(dir, "loopai.sock")
	pidPath := filepath.Join(dir, "loopai.pid")

	// Write a pid file with a clearly dead PID
	if err := os.WriteFile(pidPath, []byte("999999999\n"), 0644); err != nil {
		t.Fatalf("write stale pid: %v", err)
	}

	// checkAndWritePID should remove the stale pid and succeed
	if err := checkAndWritePID(sp); err != nil {
		t.Fatalf("expected success after removing stale pid, got: %v", err)
	}
}

func TestPidCheckRunning(t *testing.T) {
	dir := socketDir(t, "pidrunning")
	sp := filepath.Join(dir, "loopai.sock")
	pidPath := filepath.Join(dir, "loopai.pid")

	// Write our own PID — we are running, so this is a live pid
	if err := os.WriteFile(pidPath, []byte("999999999\n"), 0644); err != nil {
		t.Fatalf("write pid: %v", err)
	}
	// 999999999 is unlikely to be alive; use a process that's guaranteed running.
	// Instead, start a background process and use its PID.
	cmd := exec.Command("sh", "-c", "sleep 30")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer cmd.Process.Kill()

	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0644); err != nil {
		t.Fatalf("write live pid: %v", err)
	}

	err := checkAndWritePID(sp)
	if err == nil {
		t.Fatal("expected error when backend is already running, got nil")
	}
}

func TestPidStopRunning(t *testing.T) {
	dir := socketDir(t, "stoprunning")
	sp := filepath.Join(dir, "loopai.sock")

	// Start a fake "backend" process
	cmd := exec.Command("sh", "-c", "trap '' TERM; while true; do sleep 1; done")
	if err := cmd.Start(); err != nil {
		t.Fatalf("start helper: %v", err)
	}
	defer cmd.Process.Kill()

	// Write its pid to the pid file
	pidPath := filepath.Join(dir, "loopai.pid")
	if err := os.WriteFile(pidPath, []byte(fmt.Sprintf("%d\n", cmd.Process.Pid)), 0644); err != nil {
		t.Fatalf("write pid: %v", err)
	}

	// stopBackend sends SIGTERM and removes the pid file
	stopBackend(sp)

	// Verify pid file was removed
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("expected pid file to be removed, got: %v", err)
	}
}

func TestPidStopNoPidFile(t *testing.T) {
	dir := socketDir(t, "stoppid")
	sp := filepath.Join(dir, "loopai.sock")

	err := stopBackend(sp)
	if err == nil {
		t.Fatal("expected error when no pid file exists, got nil")
	}
}

func TestBackendStartStopIntegration(t *testing.T) {
	binary := "../bin/loopai-backend"
	if _, err := os.Stat(binary); err != nil {
		t.Skip("binary not found, run 'make build' first")
	}

	dir := socketDir(t, "stopint")
	sp := filepath.Join(dir, "loopai.sock")
	pidPath := filepath.Join(dir, "loopai.pid")

	// Start the backend binary
	cmd := exec.Command(binary, "-socket", sp)
	if err := cmd.Start(); err != nil {
		t.Fatalf("start backend: %v", err)
	}
	defer cmd.Process.Kill()

	time.Sleep(300 * time.Millisecond)

	// Run stop subcommand
	output, err := exec.Command(binary, "-socket", sp, "stop").CombinedOutput()
	if err != nil {
		t.Fatalf("stop failed: %v\n%s", err, output)
	}

	// Wait for process to exit
	done := make(chan struct{})
	go func() { cmd.Wait(); close(done) }()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("timeout waiting for backend to shut down")
	}

	// Verify pid file was removed
	if _, err := os.Stat(pidPath); !os.IsNotExist(err) {
		t.Fatalf("pid file still exists: %v", err)
	}
}

func TestBackendBinaryStartsAndAcceptsConnection(t *testing.T) {
	sp := socketPath(t, "backend.sock")

	// Temporarily replace os.Args to control flags
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()
	os.Args = []string{"loopai-backend", "-socket", sp}

	// Start backend in a goroutine
	done := make(chan struct{})
	go func() {
		main()
		close(done)
	}()

	time.Sleep(300 * time.Millisecond)

	conn, err := proto.Connect(sp)
	if err != nil {
		t.Fatalf("connect to backend: %v", err)
	}
	defer conn.Close()

	pc := proto.NewConn(conn)
	ctx := context.Background()

	// Send a started message to verify the handler processes it
	startMsg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{Pid: 1, Client: "test"})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, startMsg); err != nil {
		t.Fatalf("send started: %v", err)
	}

	// Send output + idle to trigger the enforcement loop
	outMsg, err := proto.NewMessage(proto.MsgOutput, proto.OutputPayload{Data: []byte("> go build ./...\n")})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, outMsg); err != nil {
		t.Fatalf("send output: %v", err)
	}

	idleMsg, err := proto.NewMessage(proto.MsgIdle, proto.IdlePayload{})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	if err := pc.Send(ctx, idleMsg); err != nil {
		t.Fatalf("send idle: %v", err)
	}

	// Expect a MsgType prompt back
	msg, err := pc.Receive(ctx)
	if err != nil {
		t.Fatalf("receive prompt: %v", err)
	}
	if msg.Type != proto.MsgType {
		t.Fatalf("expected MsgType prompt, got %s", msg.Type)
	}

	// Send shutdown to clean up the handler
	shutdownMsg, err := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	if err != nil {
		t.Fatalf("new message: %v", err)
	}
	pc.Send(ctx, shutdownMsg)
}

func TestBackendBinaryStartsWithDefaultSocket(t *testing.T) {
	// Verify DefaultSocketPath returns a valid path
	path := proto.DefaultSocketPath()
	if path == "" {
		t.Fatal("DefaultSocketPath returned empty")
	}
}

func TestBackendMessageRoundTrip(t *testing.T) {
	// Test that the wire protocol works independently of the backend binary
	server, client := net.Pipe()
	defer server.Close()
	defer client.Close()

	sc := proto.NewConn(server)
	cc := proto.NewConn(client)
	ctx := context.Background()

	want := proto.StartedPayload{Pid: 42, Client: "test"}
	sendMsg, err := proto.NewMessage(proto.MsgStarted, want)
	if err != nil {
		t.Fatalf("new message: %v", err)
	}

	go func() {
		cc.Send(ctx, sendMsg)
	}()

	got, err := sc.Receive(ctx)
	if err != nil {
		t.Fatalf("receive: %v", err)
	}
	if got.Type != proto.MsgStarted {
		t.Fatalf("expected MsgStarted, got %s", got.Type)
	}
	var p proto.StartedPayload
	if err := json.Unmarshal(got.Payload, &p); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if p.Pid != 42 || p.Client != "test" {
		t.Fatalf("unexpected payload: %+v", p)
	}
}
