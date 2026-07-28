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

	pc.Send(proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid:    proc.Cmd.Process.Pid,
		Client: *client,
	}))

	idle := launcher.NewIdleDetector(*idleTimeout, func() {
		pc.Send(proto.NewMessage(proto.MsgIdle, proto.IdlePayload{}))
	})
	idle.Start()

	errCh := make(chan error, 2)
	go func() {
		errCh <- launcher.PipePTYToBackend(pc, proc, idle)
	}()
	go func() {
		errCh <- launcher.PipeBackendToPTY(pc, proc)
	}()

	<-errCh
	idle.Stop()

	<-proc.Wait()
	exitCode := proc.ExitCode()
	pc.Send(proto.NewMessage(proto.MsgExited, proto.ExitedPayload{
		Code: exitCode,
	}))

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
