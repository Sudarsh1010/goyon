package bugs

import (
	"context"
	"errors"
	"testing"

	"go.uber.org/goleak"

	"github.com/sudarsh1010/goyon/par"
)

// TestParGoroutineLeak is the paired exhibit for TestNaiveGoroutineLeak.
// The naïve shape — a worker parked forever on a send nobody receives — is
// unexpressible in par: the call returns only when every goroutine it
// spawned has finished (P4, structured concurrency). goleak verifies that
// nothing outlives the call, on both the success and the error path.
func TestParGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	ctx := t.Context()
	input := make([]int, 64)
	for i := range input {
		input[i] = i
	}

	t.Run("success path", func(t *testing.T) {
		results, err := par.Map(ctx, input, func(ctx context.Context, i int, v int) (int, error) {
			return v * v, nil
		})
		if err != nil {
			t.Fatal(err)
		}
		if len(results) != len(input) {
			t.Fatalf("got %d results, want %d", len(results), len(input))
		}
	})

	t.Run("error path", func(t *testing.T) {
		err := par.ForEach(ctx, input, func(ctx context.Context, i int, v int) error {
			if v == 3 {
				return errors.New("boom")
			}
			<-ctx.Done() // parked workers must be cancelled, never orphaned
			return nil
		})
		if err == nil {
			t.Fatal("expected the boom error, got nil")
		}
	})
}
