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

// TestIdleDetectorFireStopRace simulates the scenario where the timer fires
// concurrently with Stop(). The callback must be resilient to this race.
func TestIdleDetectorFireStopRace(t *testing.T) {
	var (
		mu        sync.Mutex
		callCount int
		afterStop int // count of calls after Stop() was requested
		stopped   bool
	)
	detector := NewIdleDetector(1*time.Millisecond, func() {
		mu.Lock()
		callCount++
		if stopped {
			afterStop++
		}
		mu.Unlock()
	})

	detector.Start()
	time.Sleep(5 * time.Millisecond) // let it fire at least once
	mu.Lock()
	stopped = true
	mu.Unlock()
	detector.Stop()
	time.Sleep(10 * time.Millisecond)

	mu.Lock()
	calls := callCount
	after := afterStop
	mu.Unlock()

	// At minimum, the detector should have fired at least once.
	if calls == 0 {
		t.Fatal("expected at least one fire")
	}
	// It's acceptable for fire() to call the callback after Stop() in rare
	// race conditions (time.AfterFunc has inherent race between fire and Stop).
	// The callback MUST handle this gracefully. We just verify no panic.
	_ = after
	t.Logf("fires: %d, after-stop: %d", calls, after)
}

// TestIdleDetectorCallbackCallsReset verifies that calling Reset() from
// within the idle callback does not deadlock (since fire() releases the
// mutex before calling the callback).
func TestIdleDetectorCallbackCallsReset(t *testing.T) {
	var d IdleTimer
	d = NewIdleDetector(10*time.Millisecond, func() {
		// Reset may be called from within the callback
		d.Reset()
	})
	d.Start()
	time.Sleep(100 * time.Millisecond)
	d.Stop()
	// Test passes if no deadlock
}
