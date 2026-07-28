package main

import (
	"context"
	"flag"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/romayengineer/loopai-mcp/internal/launcher"
	"github.com/romayengineer/loopai-mcp/internal/proto"
)

const defaultIdleTimeout = 5 * time.Second

func main() {
	client := flag.String("client", defaultClient(), "client binary to spawn (claude, opencode, etc.)")
	socketPath := flag.String("socket", proto.DefaultSocketPath(), "unix socket path")
	idleTimeout := flag.Duration("idle", defaultIdleTimeout, "idle timeout before signaling backend")
	passthrough := flag.Bool("passthrough", false, "show PTY output on terminal")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	conn, err := proto.Connect(*socketPath)
	if err != nil {
		slog.Error("connect to backend", "error", err)
		os.Exit(1)
	}
	pc := proto.NewConn(conn)

	args := flag.Args()
	if args == nil {
		args = []string{}
	}

	proc, err := launcher.Spawn(*client, args)
	if err != nil {
		pc.Close()
		slog.Error("spawn client", "client", *client, "error", err)
		os.Exit(1)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	defer pc.Close()
	defer proc.Close()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		sig := <-sigCh
		slog.Info("shutting down", "signal", sig)
		cancel()
		pc.Close()
	}()

	msg, err := proto.NewMessage(proto.MsgStarted, proto.StartedPayload{
		Pid:    proc.PID(),
		Client: *client,
	})
	if err != nil {
		slog.Warn("marshal started", "error", err)
	} else if err := pc.Send(ctx, msg); err != nil {
		slog.Warn("send started", "error", err)
	}

	idle := launcher.NewIdleDetector(*idleTimeout, func() {
		msg, mErr := proto.NewMessage(proto.MsgIdle, proto.IdlePayload{})
		if mErr != nil {
			slog.Warn("marshal idle", "error", mErr)
			return
		}
		if err := pc.Send(ctx, msg); err != nil {
			slog.Warn("send idle", "error", err)
		}
	})
	idle.Start()

	// Wrap the PTY reader with optional passthrough to terminal
	r := io.Reader(proc)
	if *passthrough {
		r = io.TeeReader(proc, os.Stdout)
	}

	errCh := make(chan error, 2)
	go func() {
		err := launcher.PipePTYToBackend(ctx, pc, r, idle)
		errCh <- err
	}()
	go func() {
		err := launcher.PipeBackendToPTY(ctx, pc, proc)
		errCh <- err
	}()

	<-errCh
	idle.Stop()

	// Tell PipeBackendToPTY to stop before sending the exit message,
	// otherwise it may still be blocked on Receive, causing a data race.
	shutdownMsg, sErr := proto.NewMessage(proto.MsgShutdown, proto.ShutdownPayload{})
	if sErr != nil {
		slog.Warn("marshal shutdown", "error", sErr)
	} else if err := pc.Send(ctx, shutdownMsg); err != nil {
		slog.Warn("send shutdown", "error", err)
	}
	<-errCh

	<-proc.Wait()
	exitCode := proc.ExitCode()

	msg, err = proto.NewMessage(proto.MsgExited, proto.ExitedPayload{
		Code: exitCode,
	})
	if err != nil {
		slog.Warn("marshal exited", "error", err)
	} else if err := pc.Send(ctx, msg); err != nil {
		slog.Warn("send exited", "error", err)
	}

	if exitCode != 0 {
		slog.Warn("client exited", "code", exitCode)
		os.Exit(exitCode)
	}
}

func defaultClient() string {
	if c := os.Getenv("LOOPAI_CLIENT"); c != "" {
		return c
	}
	return "claude"
}
