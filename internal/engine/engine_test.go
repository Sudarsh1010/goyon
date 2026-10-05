package engine_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/goleak"

	"github.com/sudarsh1010/goyon/internal/engine"
)

func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}

// This suite pins Run's CONTRACT. It is green today (errgroup) and must
// stay green when Run is rewritten as a worker pool. Only
// TestRunAllocationBudget is red — it encodes §4.1 and drives the rewrite.

func TestRunExecutesEveryUnit(t *testing.T) {
	units := make([]int, 100)
	var ran atomic.Int64

	err := engine.Run(t.Context(), units, 8, true, func(ctx context.Context, i, u int) error {
		ran.Add(1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if ran.Load() != 100 {
		t.Fatalf("ran %d units, want 100", ran.Load())
	}
}

func TestRunBoundsConcurrency(t *testing.T) {
	units := make([]int, 64)

	var inFlight, maxSeen atomic.Int64
	err := engine.Run(t.Context(), units, 4, true, func(ctx context.Context, i, u int) error {
		cur := inFlight.Add(1)
		for {
			m := maxSeen.Load()
			if cur <= m || maxSeen.CompareAndSwap(m, cur) {
				break
			}
		}
		time.Sleep(2 * time.Millisecond)
		inFlight.Add(-1)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got := maxSeen.Load(); got > 4 {
		t.Fatalf("peak concurrency = %d, want <= 4", got)
	}
}

func TestRunFirstErrorCancelsRest(t *testing.T) {
	boom := errors.New("boom")
	units := make([]int, 32)
	for i := range units {
		units[i] = i
	}

	err := engine.Run(t.Context(), units, 8, true, func(ctx context.Context, i, u int) error {
		if u == 0 {
			return boom // first claimed index: deterministic
		}
		<-ctx.Done() // parked units must be released by the cancellation
		return nil
	})
	if !errors.Is(err, boom) {
		t.Fatalf("got %v, want boom", err)
	}
}

func TestRunCollectsAllErrors(t *testing.T) {
	units := make([]int, 16)
	for i := range units {
		units[i] = i
	}
	var ran atomic.Int64

	err := engine.Run(t.Context(), units, 4, false, func(ctx context.Context, i, u int) error {
		ran.Add(1)
		if u%5 == 0 {
			return errors.New("boom")
		}
		return nil
	})
	if err == nil {
		t.Fatal("expected joined error, got nil")
	}
	if ran.Load() != 16 {
		t.Fatalf("ran %d units, want 16 — collect mode never cancels", ran.Load())
	}
}

func TestRunPanicBecomesPanicError(t *testing.T) {
	units := make([]int, 16)
	for i := range units {
		units[i] = i
	}

	err := engine.Run(t.Context(), units, 8, true, func(ctx context.Context, i, u int) error {
		if u == 7 {
			panic("bang")
		}
		return nil
	})

	var pe *engine.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("got %T (%v), want *engine.PanicError", err, err)
	}
	if pe.Index != 7 {
		t.Fatalf("Index = %d, want 7", pe.Index)
	}
}

func TestRunSeqIsOrderedCaptureSafeAndBased(t *testing.T) {
	units := []int{10, 20, 30}
	var order []int // append is safe: RunSeq is strictly sequential

	err := engine.RunSeq(t.Context(), units, 100, func(ctx context.Context, i, u int) error {
		if i != 100+len(order) {
			t.Fatalf("index = %d, want %d (base offset)", i, 100+len(order))
		}
		order = append(order, u)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(order) != 3 || order[0] != 10 || order[1] != 20 || order[2] != 30 {
		t.Fatalf("order = %v, want [10 20 30]", order)
	}
}

func TestRunSeqStopsAtFirstError(t *testing.T) {
	var calls atomic.Int64

	err := engine.RunSeq(t.Context(), []int{1, 2, 3}, 0, func(ctx context.Context, i, u int) error {
		calls.Add(1)
		return errors.New("boom")
	})
	if err == nil || err.Error() != "boom" {
		t.Fatalf("got %v, want boom", err)
	}
	if calls.Load() != 1 {
		t.Fatalf("called %d times, want 1", calls.Load())
	}
}

// §4.1: the engine allocates per WORKER, never per unit. Red against the
// errgroup implementation (a closure + goroutine per unit ≈ 2 allocs
// each); green only for a real worker pool. This is the rewrite's target.
func TestRunAllocationBudget(t *testing.T) {
	const n = 1000
	units := make([]int, n)
	fn := func(ctx context.Context, i, u int) error { return nil }

	allocs := testing.AllocsPerRun(20, func() {
		if err := engine.Run(context.Background(), units, 16, true, fn); err != nil {
			t.Fatal(err)
		}
	})
	if allocs > 200 {
		t.Fatalf("%.0f allocs for %d units, want <= 200 — allocate per worker, not per unit", allocs, n)
	}
}
