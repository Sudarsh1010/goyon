package par_test

import (
	"context"
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/sudarsh1010/goyon/par"
)

func TestMapDoubleElements(t *testing.T) {
	ctx := t.Context()

	tests := []struct {
		input []int
		want  []int
		name  string
	}{
		{
			input: []int{1, 2, 3},
			want:  []int{2, 4, 6},
			name:  "TestMapBasic",
		},

		{
			input: []int{},
			want:  []int{},
			name:  "TestMapEmptySlice",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := par.Map(ctx, test.input, func(ctx context.Context, i int, u int) (int, error) {
				return u * 2, nil
			})
			if err != nil {
				t.Fatal(err)
			}

			if !slices.Equal(test.want, got) {
				t.Fatalf("got %v want %v", got, test.want)
			}
		})
	}
}

func TestMapBoundsConcurrency(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 20)
	for i := range input {
		input[i] = i
	}

	tr := newConcTracker()

	_, err := par.Map(ctx, input, func(ctx context.Context, i int, v int) (int, error) {
		tr.Enter()
		defer tr.Exit()

		time.Sleep(10 * time.Millisecond) // hold the slot so overlap is observable
		return v * 2, nil
	}, par.WithConcurrency(2))
	if err != nil {
		t.Fatal(err)
	}

	if got := tr.Max(); got > 2 {
		t.Fatalf("max concurrent calls = %d, want <= 2", got)
	}
}

func TestMapPreservesOrder(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 50)
	for i := range input {
		input[i] = i
	}

	got, err := par.Map(ctx, input, func(ctx context.Context, i int, v int) (int, error) {
		// Later elements finish first: if results were assembled in
		// completion order, the output would come back reversed.
		time.Sleep(time.Duration(len(input)-i) * time.Millisecond)
		return v * 10, nil
	}, par.WithConcurrency(50))
	if err != nil {
		t.Fatal(err)
	}

	for i, v := range got {
		if v != i*10 {
			t.Fatalf("got[%d] = %d, want %d", i, v, i*10)
		}
	}
}

func TestMapWithConcurrencyZeroRunsSerially(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 10)
	for i := range input {
		input[i] = i
	}

	tr := newConcTracker()

	_, err := par.Map(ctx, input, func(ctx context.Context, i int, v int) (int, error) {
		tr.Enter()
		defer tr.Exit()

		time.Sleep(5 * time.Millisecond)
		return v, nil
	}, par.WithConcurrency(0))
	if err != nil {
		t.Fatal(err)
	}

	if got := tr.Max(); got != 1 {
		t.Fatalf("max concurrent calls = %d, want exactly 1 (WithConcurrency(0) clamps to serial)", got)
	}
}

func TestMapRecoversPanic(t *testing.T) {
	ctx := t.Context()
	input := []int{0, 1, 2, 3}

	t.Run("Kaboom", func(t *testing.T) {
		_, err := par.Map(ctx, input, func(ctx context.Context, i int, u int) (int, error) {
			if u == 2 {
				panic("kaboom")
			}
			return u, nil
		})
		if err == nil {
			t.Fatal("expected the kaboom error, got nil")
		}

		var pe *par.PanicError
		if !errors.As(err, &pe) {
			t.Fatalf("got %T (%v), want *par.PanicError", err, err)
		}
		if pe.Value != "kaboom" {
			t.Fatalf("PanicError.Value = %v, want %q", pe.Value, "kaboom")
		}
		if pe.Index != 2 {
			t.Fatalf("PanicError.Index = %d, want 2", pe.Index)
		}
		if len(pe.Stack) == 0 {
			t.Fatal("PanicError.Stack is empty, want captured goroutine stack")
		}
	})
}
