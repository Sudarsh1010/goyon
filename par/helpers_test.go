package par_test

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// concTracker measures the peak number of simultaneously executing work
// functions, from inside the functions themselves — the only observatory a
// par caller has (the API exposes no internals). Used by the concurrency-
// bound tests; see TestConcTrackerRecordsMax for its contract.
type concTracker struct {
	inFlight atomic.Int64
	maxSeen  atomic.Int64
}

func newConcTracker() *concTracker {
	return &concTracker{}
}

// Enter marks the start of one work function. It updates the peak with a
// CAS loop rather than a blind Store: two goroutines recording 7 and 8 can
// race, and Store could write 7 after 8 — silently losing the true peak.
// The loop retries until the peak is at least our observed value.
func (t *concTracker) Enter() {
	cur := t.inFlight.Add(1)
	for {
		m := t.maxSeen.Load()
		if cur <= m || t.maxSeen.CompareAndSwap(m, cur) {
			break
		}
	}
}

// Exit marks the end of one work function.
func (t *concTracker) Exit() {
	t.inFlight.Add(-1)
}

// Max returns the peak number of concurrent Enter()s ever observed.
func (t *concTracker) Max() int64 {
	return t.maxSeen.Load()
}

// TestConcTrackerRecordsMax drives the shared concurrency-instrumentation
// helper used by the bounds tests. Contract:
//   - Max() is the HIGHEST number of simultaneous Enter()s ever observed
//     (a historical peak, not the current count).
//   - Zero usage means zero.
//   - It is itself race-free under -race (a helper that races would
//     invalidate every test that uses it).
func TestConcTrackerRecordsMax(t *testing.T) {
	t.Run("sequential", func(t *testing.T) {
		tr := newConcTracker()
		if got := tr.Max(); got != 0 {
			t.Fatalf("fresh tracker: Max() = %d, want 0", got)
		}

		tr.Enter()
		tr.Enter()
		tr.Exit()
		if got := tr.Max(); got != 2 {
			t.Fatalf("Max() = %d, want 2", got)
		}

		tr.Exit() // fully drained — but Max is a peak, not the current count
		if got := tr.Max(); got != 2 {
			t.Fatalf("after Exit: Max() = %d, want 2 (peak is historical)", got)
		}
	})

	t.Run("concurrent", func(t *testing.T) {
		tr := newConcTracker()

		var wg sync.WaitGroup
		for range 8 {
			wg.Add(1)
			go func() {
				defer wg.Done()
				tr.Enter()
				defer tr.Exit()
				time.Sleep(10 * time.Millisecond) // force the overlap
			}()
		}
		wg.Wait()

		if got := tr.Max(); got != 8 {
			t.Fatalf("Max() = %d, want 8", got)
		}
	})
}
