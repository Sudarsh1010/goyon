package par_test

import (
	"context"
	"errors"
	"slices"
	"testing"

	"github.com/sudarsh1010/goyon/par"
)

// MapUnordered returns the same multiset of results as Map, in
// unspecified (completion) order — the permutation is the contract.

func TestMapUnorderedReturnsPermutation(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 200)
	for i := range input {
		input[i] = i
	}

	got, err := par.MapUnordered(ctx, input, func(ctx context.Context, i int, v int) (int, error) {
		return v * 3, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != len(input) {
		t.Fatalf("got %d results, want %d", len(got), len(input))
	}

	slices.Sort(got)
	want := make([]int, len(input))
	for i, v := range input {
		want[i] = v * 3
	}
	if !slices.Equal(got, want) {
		t.Fatal("results are not a permutation of the mapped input")
	}
}

func TestMapUnorderedEmpty(t *testing.T) {
	ctx := t.Context()

	got, err := par.MapUnordered(ctx, []int{}, func(ctx context.Context, i int, v int) (int, error) {
		return v, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if got == nil || len(got) != 0 {
		t.Fatalf("got %#v, want empty non-nil", got)
	}
}

func TestMapUnorderedReturnsFirstError(t *testing.T) {
	ctx := t.Context()

	_, err := par.MapUnordered(ctx, []int{1, 2, 3}, func(ctx context.Context, i int, v int) (int, error) {
		if v == 2 {
			return 0, errors.New("boom")
		}
		return v, nil
	})
	if err == nil {
		t.Fatal("expected the boom error, got nil")
	}
	if err.Error() != "boom" {
		t.Fatalf("got %q, want %q", err, "boom")
	}
}

func TestMapUnorderedRecoversPanic(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 12)
	for i := range input {
		input[i] = i
	}

	_, err := par.MapUnordered(ctx, input, func(ctx context.Context, i int, v int) (int, error) {
		if v == 7 {
			panic("bang")
		}
		return v, nil
	})

	var pe *par.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("got %T (%v), want *par.PanicError", err, err)
	}
	if pe.Index != 7 {
		t.Fatalf("PanicError.Index = %d, want 7", pe.Index)
	}
}
