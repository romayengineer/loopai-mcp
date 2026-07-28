package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/launcher"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

func main() {
	client := flag.String("client", defaultClient(), "client binary to spawn (claude, opencode, etc.)")
	socketPath := flag.String("socket", proto.DefaultSocketPath(), "unix socket path")
	idleTimeout := flag.Duration("idle", 5*time.Second, "idle timeout before signaling backend")
	flag.Parse()

	conn, err := proto.Connect(*socketPath)
	if err != nil {
		log.Fatalf("connect to backend: %v\n  (is loopai-backend running?)", err)
	}
	pc := proto.NewConn(conn)
	defer pc.Close()

	args := flag.Args()
	if args == nil {
		args = []string{}
	}

	proc, err := launcher.Spawn(*client, args)
	if err != nil {
		log.Fatal(err)
	}
	defer proc.Close()

	if err := pc.Send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid:    proc.Cmd.Process.Pid,
		Client: *client,
	})); err != nil {
		log.Printf("send started: %v", err)
	}

	idle := launcher.NewIdleDetector(*idleTimeout, func() {
		if err := pc.Send(proto.NewMessage(proto.MsgIdle, proto.IdlePayload{})); err != nil {
			log.Printf("send idle: %v", err)
		}
	})
	idle.Start()

	errCh := make(chan error, 2)
	go func() {
		err := launcher.PipePTYToBackend(pc, proc, idle)
		errCh <- err
	}()
	go func() {
		err := launcher.PipeBackendToPTY(pc, proc)
		errCh <- err
	}()

	<-errCh
	idle.Stop()

	<-proc.Wait()
	exitCode := proc.ExitCode()

	if err := pc.Send(proto.NewMessage(proto.MsgExited, proto.ExitedPayload{
		Code: exitCode,
	})); err != nil {
		log.Printf("send exited: %v", err)
	}

	if exitCode != 0 {
		fmt.Fprintf(os.Stderr, "client exited with code %d\n", exitCode)
	}
}

func defaultClient() string {
	if c := os.Getenv("LOOPAI_CLIENT"); c != "" {
		return c
	}
	return "claude"
}
