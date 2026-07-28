package backend

import (
	"context"
	"errors"
	"net"
	"testing"
)

func TestBackendStopBeforeRun(t *testing.T) {
	b := New("/tmp/nonexistent/test_stop_before_run.sock", func(_ context.Context, _ LauncherConn) {})
	b.Stop()
}

func TestBackendStopTwice(t *testing.T) {
	b := New("/tmp/nonexistent/test_stop_twice.sock", func(_ context.Context, _ LauncherConn) {})
	b.Stop()
	b.Stop()
}

func TestBackendStopWrongType(t *testing.T) {
	b := New("/tmp/nonexistent/test_stop_wrong_type.sock", func(_ context.Context, _ LauncherConn) {})
	b.ln.Store("not a listener")
	b.Stop()
}

type errListener struct {
	net.Listener
	err error
}

func (l *errListener) Accept() (net.Conn, error) {
	return nil, l.err
}

type errSocketListener struct {
	err error
}

func (l *errSocketListener) Listen(_ string) (net.Listener, error) {
	return nil, l.err
}

func TestBackendRunListenError(t *testing.T) {
	b := New("/tmp/nonexistent/test_run_listen_error.sock", func(_ context.Context, _ LauncherConn) {})
	b.listener = &errSocketListener{err: errors.New("listen failed")}

	err := b.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from Run when Listen fails, got nil")
	}
}

func TestBackendRunContextCancel(t *testing.T) {
	b := New("/tmp/nonexistent/test_run_cancel.sock", func(_ context.Context, _ LauncherConn) {})
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := b.Run(ctx)
	if err != context.Canceled {
		t.Fatalf("expected context.Canceled after cancelled context, got %v", err)
	}
}

type immediateListener struct {
	acceptReturned bool
}

func (l *immediateListener) Accept() (net.Conn, error) {
	if !l.acceptReturned {
		l.acceptReturned = true
		return nil, errors.New("accept failed")
	}
	select {}
}

func (l *immediateListener) Close() error {
	return nil
}

func (l *immediateListener) Addr() net.Addr {
	return nil
}

type closeErrorListener struct {
	net.Listener
}

func (l *closeErrorListener) Close() error {
	return errors.New("close failed")
}

func TestBackendStopCloseError(t *testing.T) {
	// Store a listener in ln that returns an error on Close.
	// Stop() should log and handle it gracefully.
	b := New("/tmp/nonexistent/test_stop_close_error.sock", func(_ context.Context, _ LauncherConn) {})
	b.ln.Store(&closeErrorListener{})
	b.Stop()
	// Should not panic
}

func TestBackendRunAcceptError(t *testing.T) {
	b := New("/tmp/nonexistent/test_run_accept_error.sock", func(_ context.Context, _ LauncherConn) {})
	b.listener = socketListenFunc(func(_ string) (net.Listener, error) {
		return &immediateListener{}, nil
	})

	err := b.Run(context.Background())
	if err == nil {
		t.Fatal("expected error from Run when Accept fails, got nil")
	}
}
