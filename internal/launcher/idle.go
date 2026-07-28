package launcher

import (
	"log/slog"
	"sync"
	"sync/atomic"
	"time"
)

// IdleDetector fires a callback after a configurable period of inactivity.
// Reset can be called to extend the idle window (e.g. on each byte of output).
type IdleDetector struct {
	mu      sync.Mutex
	timeout time.Duration
	onIdle  func()
	timer   *time.Timer
	running atomic.Bool
}

// NewIdleDetector creates an idle detector that calls onIdle after
// the given timeout without a Reset call.
func NewIdleDetector(timeout time.Duration, onIdle func()) *IdleDetector {
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
}

func (d *IdleDetector) fire() {
	if !d.running.Load() {
		return
	}
	slog.Debug("idle detector fired", "timeout", d.timeout)
	d.onIdle()
}
