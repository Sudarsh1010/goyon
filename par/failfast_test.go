package par_test

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"

	"github.com/sudarsh1010/goyon/par"
)

// WithFailFast(false): every unit runs to completion, no cancellation,
// and all errors are joined (errors.Is reaches each sentinel).
// Default (true) is already pinned by TestForEachReturnsFirstError.

func TestForEachFailFastFalseRunsAllAndJoins(t *testing.T) {
	ctx := t.Context()

	errA := errors.New("boom-a")
	errB := errors.New("boom-b")
	input := make([]int, 20)
	for i := range input {
		input[i] = i
	}

	var visited atomic.Int64
	err := par.ForEach(ctx, input, func(ctx context.Context, i int, v int) error {
		visited.Add(1)
		switch v {
		case 3:
			return errA
		case 11:
			return errB
		}
		return nil
	}, par.WithFailFast(false))
	if err == nil {
		t.Fatal("expected joined error, got nil")
	}
	if !errors.Is(err, errA) || !errors.Is(err, errB) {
		t.Fatalf("joined error %v missing sentinels", err)
	}
	if visited.Load() != int64(len(input)) {
		t.Fatalf("visited %d, want %d — no early cancellation", visited.Load(), len(input))
	}
}

func TestForEachFailFastFalseNoErrorsIsNil(t *testing.T) {
	ctx := t.Context()

	err := par.ForEach(ctx, []int{1, 2, 3}, func(ctx context.Context, i int, v int) error {
		return nil
	}, par.WithFailFast(false))
	if err != nil {
		t.Fatalf("got %v, want nil", err)
	}
}

func TestMapFailFastFalseStillDropsPartialResults(t *testing.T) {
	ctx := t.Context()

	results, err := par.Map(ctx, []int{1, 2, 3}, func(ctx context.Context, i int, v int) (int, error) {
		if v == 2 {
			return 0, errors.New("boom")
		}
		return v, nil
	}, par.WithFailFast(false))
	if err == nil {
		t.Fatal("expected the boom error, got nil")
	}
	if results != nil {
		t.Fatalf("got %v, want nil — partial results are never returned", results)
	}
}

func TestReduceFailFastFalseReturnsError(t *testing.T) {
	ctx := t.Context()

	_, err := par.Reduce(ctx, []int{1, 2, 3, 4}, func(ctx context.Context, a, b int) (int, error) {
		if b == 3 {
			return 0, errors.New("boom")
		}
		return a + b, nil
	}, par.WithFailFast(false))
	if err == nil {
		t.Fatal("expected the boom error, got nil")
	}
}
