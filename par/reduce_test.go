package par_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/sudarsh1010/goyon/par"
)

// RED: par.Reduce, par.WithIdentity, and par.WithOrderedReduce do not exist
// yet. This file does not compile — that IS the first red (lesson 0001).
//
// Locked signature (design doc §3.1):
//
//	func Reduce[T any](ctx context.Context, items []T,
//	    fn func(ctx context.Context, a, b T) (T, error), opts ...Option) (T, error)
//
// fn combines two values into one. It MUST be associative and commutative
// unless WithOrderedReduce is given — the contract is documented, not
// enforced (§5). Identity is the zero value of T unless WithIdentity is.
//
// Suggested cycle order: the tests below are ordered from "trivially
// sequential passes" to "must actually be parallel". Get each green before
// moving on. Resist writing the real engine until a test demands it.

// Cycle 1 — the whole point: fold a slice into one value.
// Sum is associative + commutative, so any reduction strategy passes.
func TestReduceSumsIntegers(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 100)
	for i := range input {
		input[i] = i + 1 // 1..100
	}

	sum, err := par.Reduce(
		ctx,
		input,
		func(ctx context.Context, a, b int) (int, error) {
			return a + b, nil
		},
	)
	if err != nil {
		t.Fatal(err)
	}

	if sum != 5050 {
		t.Fatalf("sum of 1..100 = %d, want 5050", sum)
	}
}

// Cycle 2 — empty input returns the identity (zero value of T), not an
// error. "Reduce nothing" is a question with a defined answer: the identity.
func TestReduceEmptySliceReturnsZeroValue(t *testing.T) {
	ctx := t.Context()

	sum, err := par.Reduce(ctx, []int{}, func(ctx context.Context, a, b int) (int, error) {
		return a + b, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if sum != 0 {
		t.Fatalf("sum of empty = %d, want 0 (zero value)", sum)
	}
}

// Cycle 3 — a single element reduces to itself. (Does fn get called zero
// times or once? Not pinned — only the result is the contract.)
func TestReduceSingleElement(t *testing.T) {
	ctx := t.Context()

	got, err := par.Reduce(ctx, []int{7}, func(ctx context.Context, a, b int) (int, error) {
		return a + b, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got != 7 {
		t.Fatalf("reduce of [7] = %d, want 7", got)
	}
}

// Cycle 4 — WithIdentity overrides the zero value. This is what makes
// Reduce useful for ops whose identity isn't the zero value: product's
// identity is 1, min's is +Inf, and empty input returns the GIVEN identity.
func TestReduceWithIdentity(t *testing.T) {
	ctx := t.Context()

	mul := func(ctx context.Context, a, b int) (int, error) {
		return a * b, nil
	}

	t.Run("empty returns the given identity", func(t *testing.T) {
		got, err := par.Reduce(ctx, []int{}, mul, par.WithIdentity(1))
		if err != nil {
			t.Fatal(err)
		}
		if got != 1 {
			t.Fatalf("product of empty = %d, want 1 (given identity)", got)
		}
	})

	t.Run("product of 1..5", func(t *testing.T) {
		got, err := par.Reduce(ctx, []int{1, 2, 3, 4, 5}, mul, par.WithIdentity(1))
		if err != nil {
			t.Fatal(err)
		}
		if got != 120 {
			t.Fatalf("product of 1..5 = %d, want 120", got)
		}
	})
}

// Cycle 5 — WithOrderedReduce pins left-fold semantics for NON-associative
// ops. Contract: the result equals a sequential left fold starting from the
// identity — fn(...fn(fn(identity, items[0]), items[1]), ...).
//
// Subtraction is the trap: it is neither associative nor commutative, so
// any tree/parallel recombination gives a different answer. If this passes,
// ordered mode is genuinely sequential in its combination ORDER (it may
// still overlap the fn CALLS' context setup — but each combination must
// wait for the running left result).
func TestReduceOrderedIsLeftFold(t *testing.T) {
	ctx := t.Context()

	input := []int{100, 1, 2, 3}

	got, err := par.Reduce(ctx, input, func(ctx context.Context, a, b int) (int, error) {
		return a - b, nil
	}, par.WithOrderedReduce())
	if err != nil {
		t.Fatal(err)
	}

	// Sequential left fold from the zero value: ((0-100)-1)-2-3
	want := 0
	for _, v := range input {
		want -= v
	}
	if got != want {
		t.Fatalf("ordered reduce = %d, want left fold %d", got, want)
	}
}

// Cycle 6 — Reduce is actually parallel. Without this test, a for-loop
// passes everything above. Default concurrency is GOMAXPROCS; with 32
// sleeping combinations, peak overlap must exceed 1.
func TestReduceRunsConcurrently(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 32)
	for i := range input {
		input[i] = 1
	}

	tr := newConcTracker()

	_, err := par.Reduce(ctx, input, func(ctx context.Context, a, b int) (int, error) {
		tr.Enter()
		defer tr.Exit()

		time.Sleep(5 * time.Millisecond) // hold the slot so overlap is observable
		return a + b, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := tr.Max(); got < 2 {
		t.Fatalf("max concurrent combines = %d, want >= 2 — Reduce must not be sequential", got)
	}
}

// Cycle 7 — first error wins and is returned (fail-fast, §3.3 default).
// Partial accumulation is discarded; the caller gets the error, not a sum.
func TestReduceReturnsFirstError(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 30)
	for i := range input {
		input[i] = i
	}

	_, err := par.Reduce(ctx, input, func(ctx context.Context, a, b int) (int, error) {
		if a == 13 || b == 13 {
			return 0, errors.New("boom")
		}
		return a + b, nil
	})
	if err == nil {
		t.Fatal("expected the boom error, got nil")
	}
	if err.Error() != "boom" {
		t.Fatalf("got %q, want %q", err, "boom")
	}
}

// Cycle 8 — a panicking combine function is recovered and returned as
// *PanicError (P6). Unordered mode: Index is the GLOBAL element index.
func TestReduceRecoversPanic(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 16)
	for i := range input {
		input[i] = i
	}

	_, err := par.Reduce(ctx, input, func(ctx context.Context, a, b int) (int, error) {
		if b == 7 {
			panic("bang")
		}
		return a + b, nil
	})
	if err == nil {
		t.Fatal("expected a *PanicError, got nil")
	}

	var pe *par.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("got %T (%v), want *par.PanicError", err, err)
	}
	if pe.Value != "bang" {
		t.Fatalf("PanicError.Value = %v, want %q", pe.Value, "bang")
	}
	if pe.Index != 7 {
		t.Fatalf("PanicError.Index = %d, want 7 (global element index)", pe.Index)
	}
}

// Cycle 9 — ordered mode reports the element index too, not -1.
func TestReduceOrderedPanicReportsElementIndex(t *testing.T) {
	ctx := t.Context()

	input := []int{10, 20, 30, 40, 50}

	_, err := par.Reduce(ctx, input, func(ctx context.Context, a, b int) (int, error) {
		if b == 30 {
			panic("bang")
		}
		return a - b, nil
	}, par.WithOrderedReduce())
	if err == nil {
		t.Fatal("expected a *PanicError, got nil")
	}

	var pe *par.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("got %T (%v), want *par.PanicError", err, err)
	}
	if pe.Index != 2 {
		t.Fatalf("PanicError.Index = %d, want 2 (element 30 is at index 2)", pe.Index)
	}
}

// Bonus retrieval exercise (LR-0006): before running the suite, predict
// OUT LOUD which tests a trivial sequential implementation would pass.
// Answer key: cycles 1–5. If cycle 6 doesn't fail on a sequential Reduce,
// the test is broken, not the library.
