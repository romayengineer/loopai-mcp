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
	"golang.org/x/term"
)

const defaultIdleTimeout = 5 * time.Second

func main() {
	client := flag.String("client", defaultClient(), "client binary to spawn (claude, opencode, etc.)")
	socketPath := flag.String("socket", proto.DefaultSocketPath(), "unix socket path")
	idleTimeout := flag.Duration("idle", defaultIdleTimeout, "idle timeout before signaling backend")
	passthrough := flag.Bool("passthrough", false, "show PTY output on terminal")
	interactive := flag.Bool("interactive", false, "interactive mode: stdin -> PTY, PTY -> terminal (implies -passthrough)")
	flag.Parse()

	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, nil)))

	conn, err := proto.Connect(*socketPath)
	if err != nil {
		slog.Error("backend not running — start it with 'loopai-backend'", "error", err)
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

	// Interactive mode: forward stdin -> PTY and enable passthrough
	if *interactive {
		*passthrough = true

		if term.IsTerminal(int(os.Stdin.Fd())) {
			oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
			if err != nil {
				slog.Error("raw terminal", "error", err)
			} else {
				defer func() {
					if err := term.Restore(int(os.Stdin.Fd()), oldState); err != nil {
						slog.Warn("restore terminal", "error", err)
					}
				}()
			}
		}

		go func() {
			written, err := io.Copy(proc, os.Stdin)
			if err != nil {
				slog.Warn("stdin copy", "written", written, "error", err)
			}
		}()
	}

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
