package launcher

import (
	"log"
	"sync/atomic"
	"time"
)

type IdleDetector struct {
	timeout time.Duration
	onIdle  func()
	timer   *time.Timer
	running atomic.Bool
}

func NewIdleDetector(timeout time.Duration, onIdle func()) *IdleDetector {
	return &IdleDetector{
		timeout: timeout,
		onIdle:  onIdle,
	}
}

func (d *IdleDetector) Start() {
	if !d.running.CompareAndSwap(false, true) {
		return
	}
	d.timer = time.AfterFunc(d.timeout, d.fire)
}

func (d *IdleDetector) Reset() {
	if !d.running.Load() {
		return
	}
	d.timer.Reset(d.timeout)
}

func (d *IdleDetector) Stop() {
	if !d.running.CompareAndSwap(true, false) {
		return
	}
	d.timer.Stop()
}

func (d *IdleDetector) fire() {
	if !d.running.Load() {
		return
	}
	log.Printf("[idle] no output for %v", d.timeout)
	d.onIdle()
}
