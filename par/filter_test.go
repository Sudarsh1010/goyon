package par_test

import (
	"context"
	"errors"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/sudarsh1010/goyon/par"
)

func TestFilterKeepsMatchesInOrder(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 100)
	for i := range input {
		input[i] = i
	}

	got, err := par.Filter(ctx, input, func(ctx context.Context, i int, v int) (bool, error) {
		return v%2 == 0, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	want := make([]int, 0, 50)
	for v := range input {
		if v%2 == 0 {
			want = append(want, v)
		}
	}
	if !slices.Equal(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestFilterEmptyVariants(t *testing.T) {
	ctx := t.Context()
	keepAll := func(ctx context.Context, i int, v int) (bool, error) { return true, nil }

	t.Run("empty input", func(t *testing.T) {
		got, err := par.Filter(ctx, []int{}, keepAll)
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got) != 0 {
			t.Fatalf("got %#v, want empty non-nil", got)
		}
	})

	t.Run("none match", func(t *testing.T) {
		got, err := par.Filter(ctx, []int{1, 3, 5}, func(ctx context.Context, i int, v int) (bool, error) {
			return false, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if got == nil || len(got) != 0 {
			t.Fatalf("got %#v, want empty non-nil", got)
		}
	})

	t.Run("all match returns input", func(t *testing.T) {
		input := []int{4, 5, 6}
		got, err := par.Filter(ctx, input, keepAll)
		if err != nil {
			t.Fatal(err)
		}
		if !slices.Equal(got, input) {
			t.Fatalf("got %v, want %v", got, input)
		}
	})
}

func TestFilterPredSeesEveryIndex(t *testing.T) {
	ctx := t.Context()

	input := []string{"a", "b", "c", "d"}
	seen := make([]bool, len(input))

	_, err := par.Filter(ctx, input, func(ctx context.Context, i int, v string) (bool, error) {
		seen[i] = v == input[i]
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for i, ok := range seen {
		if !ok {
			t.Fatalf("index %d never saw its pair", i)
		}
	}
}

func TestFilterRunsConcurrently(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 32)
	tr := newConcTracker()

	_, err := par.Filter(ctx, input, func(ctx context.Context, i int, v int) (bool, error) {
		tr.Enter()
		defer tr.Exit()

		time.Sleep(5 * time.Millisecond)
		return true, nil
	})
	if err != nil {
		t.Fatal(err)
	}

	if got := tr.Max(); got < 2 {
		t.Fatalf("max concurrent preds = %d, want >= 2", got)
	}
}

func TestFilterReturnsFirstError(t *testing.T) {
	ctx := t.Context()

	_, err := par.Filter(ctx, []int{0, 1, 2, 3, 4, 5}, func(ctx context.Context, i int, v int) (bool, error) {
		if v == 5 {
			return false, errors.New("boom")
		}
		return true, nil
	})
	if err == nil {
		t.Fatal("expected the boom error, got nil")
	}
	if err.Error() != "boom" {
		t.Fatalf("got %q, want %q", err, "boom")
	}
}

func TestFilterRecoversPanic(t *testing.T) {
	ctx := t.Context()

	input := make([]int, 16)
	for i := range input {
		input[i] = i
	}

	_, err := par.Filter(ctx, input, func(ctx context.Context, i int, v int) (bool, error) {
		if v == 7 {
			panic("bang")
		}
		return true, nil
	})

	var pe *par.PanicError
	if !errors.As(err, &pe) {
		t.Fatalf("got %T (%v), want *par.PanicError", err, err)
	}
	if pe.Index != 7 {
		t.Fatalf("PanicError.Index = %d, want 7", pe.Index)
	}
}

func TestFilterFailFastFalseCollectsAllErrors(t *testing.T) {
	ctx := t.Context()

	errA := errors.New("boom-a")
	errB := errors.New("boom-b")
	input := make([]int, 20)
	for i := range input {
		input[i] = i
	}

	var visited atomic.Int64
	_, err := par.Filter(ctx, input, func(ctx context.Context, i int, v int) (bool, error) {
		visited.Add(1)
		switch v {
		case 3:
			return false, errA
		case 11:
			return false, errB
		}
		return true, nil
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
