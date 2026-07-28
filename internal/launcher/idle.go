package launcher

import (
	"log"
	"sync"
	"time"
)

type IdleDetector struct {
	timeout  time.Duration
	timer    *time.Timer
	onIdle   func()
	mu       sync.Mutex
	stopped  bool
}

func NewIdleDetector(timeout time.Duration, onIdle func()) *IdleDetector {
	return &IdleDetector{
		timeout: timeout,
		onIdle:  onIdle,
	}
}

func (d *IdleDetector) Start() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped {
		return
	}
	d.timer = time.AfterFunc(d.timeout, d.fire)
}

func (d *IdleDetector) Reset() {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.stopped || d.timer == nil {
		return
	}
	d.timer.Reset(d.timeout)
}

func (d *IdleDetector) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()
	d.stopped = true
	if d.timer != nil {
		d.timer.Stop()
	}
}

func (d *IdleDetector) fire() {
	d.mu.Lock()
	stopped := d.stopped
	d.mu.Unlock()
	if stopped {
		return
	}
	log.Printf("[idle] no output for %v", d.timeout)
	d.onIdle()
}
