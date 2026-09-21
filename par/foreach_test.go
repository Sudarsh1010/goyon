package par_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sudarsh1010/goyon/par"
)

func TestForEachVisitsEveryElement(t *testing.T) {
	ctx := t.Context()

	input := []string{"a", "b", "c", "d"}
	seen := make([]string, len(input)) // disjoint per-index slots: race-free

	err := par.ForEach(ctx, input, func(ctx context.Context, i int, v string) error {
		seen[i] = v // also verifies fn receives the correct (i, v) pairs — P3
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Equal(seen, input) {
		t.Fatalf("got %v, want every element visited in slot order %v", seen, input)
	}
}

func TestForEachReturnsFirstError(t *testing.T) {
	ctx := t.Context()

	input := []int{0, 1, 2, 3}
	err := par.ForEach(ctx, input, func(ctx context.Context, i int, v int) error {
		if v == 1 {
			return errors.New("boom")
		}
		return nil
	})

	if err == nil {
		t.Fatal("expected the boom error, got nil")
	}
	if err.Error() != "boom" {
		t.Fatalf("got %q, want %q", err, "boom")
	}
}

func TestForEachBoundsConcurrency(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 20)
	for i := range input {
		input[i] = i
	}

	tr := newConcTracker() // RED: undefined — write it in helpers_test.go

	err := par.ForEach(ctx, input, func(ctx context.Context, i int, v int) error {
		tr.Enter()
		defer tr.Exit()

		time.Sleep(5 * time.Millisecond) // hold the slot so overlap is observable
		return nil
	}, par.WithConcurrency(3))
	if err != nil {
		t.Fatal(err)
	}

	if got := tr.Max(); got > 3 {
		t.Fatalf("max concurrent calls = %d, want <= 3", got)
	}
}
