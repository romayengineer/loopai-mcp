// Package launcher provides PTY lifecycle and I/O streaming for spawning
// TUI agent clients (Claude Code, OpenCode, etc.) in a pseudo-terminal and
// piping their output to a backend over a Unix socket.
package launcher

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// IdleTimer is the interface for an idle detector that can be started,
// reset (to extend the idle window), and stopped.
type IdleTimer interface {
	Start()
	Stop()
	Reset()
}

// IdleDetector fires a callback after a configurable period of inactivity.
// Reset can be called to extend the idle window (e.g. on each byte of output).
//
// The zero value is not usable; use NewIdleDetector to create one.
type IdleDetector struct {
	mu      sync.Mutex
	timeout time.Duration
	onIdle  func()
	timer   *time.Timer
	running atomic.Bool
}

// NewIdleDetector creates an idle detector that calls onIdle after
// the given timeout without a Reset call.
func NewIdleDetector(timeout time.Duration, onIdle func()) IdleTimer {
	return &IdleDetector{
		timeout: timeout,
		onIdle:  onIdle,
	}
}

// Start begins the idle timer. No-op if already running.
func (d *IdleDetector) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running.CompareAndSwap(false, true) {
		return
	}
	d.timer = time.AfterFunc(d.timeout, d.fire)
	slog.Debug("idle detector started", "timeout", d.timeout)
}

// Reset resets the idle timer to the full timeout. No-op if not running.
func (d *IdleDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running.Load() {
		return
	}
	d.timer.Reset(d.timeout)
}

// Stop cancels the idle timer. No-op if not running.
func (d *IdleDetector) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if !d.running.CompareAndSwap(true, false) {
		return
	}
	d.timer.Stop()
	slog.Debug("idle detector stopped")
}

// fire is called by the timer goroutine. It checks the running state
// under the mutex to minimize the race window with Stop(). Because the
// callback is called outside the lock (to prevent deadlock if the
// callback calls Reset()), a small race window remains, but the callback
// MUST be resilient to false triggers (e.g., by checking ctx.Err()
// before sending on a connection that may be closing).
func (d *IdleDetector) fire() {
	d.mu.Lock()
	running := d.running.Load()
	d.mu.Unlock()
	if !running {
		return
	}
	slog.Debug("idle detector fired", "timeout", d.timeout)
	d.onIdle()
}
