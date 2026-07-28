package launcher

import (
	"sync/atomic"
	"testing"
	"time"
)

func TestIdleDetectorFires(t *testing.T) {
	var fired atomic.Bool
	detector := NewIdleDetector(50*time.Millisecond, func() {
		fired.Store(true)
	})
	detector.Start()
	defer detector.Stop()

	time.Sleep(100 * time.Millisecond)
	if !fired.Load() {
		t.Fatal("expected idle callback to fire after timeout")
	}
}

func TestIdleDetectorResetPreventsFire(t *testing.T) {
	var fired atomic.Bool
	detector := NewIdleDetector(100*time.Millisecond, func() {
		fired.Store(true)
	})
	detector.Start()
	defer detector.Stop()

	for i := 0; i < 5; i++ {
		time.Sleep(50 * time.Millisecond)
		detector.Reset()
	}

	if fired.Load() {
		t.Fatal("expected idle callback NOT to fire with resets keeping it alive")
	}
}

func TestIdleDetectorStopsBeforeFire(t *testing.T) {
	var fired atomic.Bool
	detector := NewIdleDetector(50*time.Millisecond, func() {
		fired.Store(true)
	})
	detector.Start()
	detector.Stop()

	time.Sleep(100 * time.Millisecond)
	if fired.Load() {
		t.Fatal("expected idle callback NOT to fire after Stop")
	}
}

func TestIdleDetectorResetAfterStop(t *testing.T) {
	var fired atomic.Bool
	detector := NewIdleDetector(20*time.Millisecond, func() {
		fired.Store(true)
	})
	detector.Start()
	detector.Stop()
	detector.Reset() // should be a no-op, not panic

	time.Sleep(50 * time.Millisecond)
	if fired.Load() {
		t.Fatal("expected idle callback NOT to fire after Stop")
	}
}

func TestIdleDetectorDoubleStop(t *testing.T) {
	detector := NewIdleDetector(20*time.Millisecond, func() {})
	detector.Start()
	detector.Stop()
	detector.Stop() // should not panic
}
