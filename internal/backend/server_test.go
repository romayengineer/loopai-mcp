package backend

import (
	"context"
	"testing"
)

func TestBackendStopBeforeRun(t *testing.T) {
	b := New("/tmp/nonexistent/test_stop_before_run.sock", func(_ context.Context, _ LauncherConn) {})
	// Should not panic when Stop is called before Run.
	b.Stop()
}

func TestBackendStopTwice(t *testing.T) {
	b := New("/tmp/nonexistent/test_stop_twice.sock", func(_ context.Context, _ LauncherConn) {})
	b.Stop()
	b.Stop() // should not panic
}
