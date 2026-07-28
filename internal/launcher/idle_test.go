package launcher

import (
	"sync"
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

func TestIdleDetectorConcurrentReset(t *testing.T) {
	detector := NewIdleDetector(1*time.Second, func() {})
	detector.Start()
	defer detector.Stop()

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			detector.Reset()
		}()
	}
	wg.Wait()
	// Test passes if no panic from timer race
}

func TestIdleDetectorFireAfterStop(t *testing.T) {
	var fired atomic.Bool
	detector := NewIdleDetector(20*time.Millisecond, func() {
		fired.Store(true)
	})
	detector.Start()
	detector.Stop()

	// Wait long enough for the timer to have fired
	time.Sleep(50 * time.Millisecond)

	if fired.Load() {
		t.Fatal("expected callback NOT to fire after Stop")
	}
}

func TestIdleDetectorStartTwice(t *testing.T) {
	d := NewIdleDetector(1*time.Second, func() {})
	d.Start()
	d.Start() // second call should be a no-op (CAS fails)
	d.Stop()
}

func TestIdleDetectorStartRace(t *testing.T) {
	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d := NewIdleDetector(1*time.Second, func() {})
			d.Start()
			d.Stop()
		}()
	}
	wg.Wait()
	// Test passes if no panic from concurrent Start/Stop races
}
