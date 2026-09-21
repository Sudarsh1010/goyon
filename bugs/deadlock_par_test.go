package bugs

import (
	"context"
	"errors"
	"testing"

	"github.com/sudarsh1010/goyon/par"
)

func TestParDeadlock(t *testing.T) {
	ctx := t.Context()
	want := "boom"

	input := make([]int, 8)
	for i := range input {
		input[i] = i
	}

	t.Run("TestParFirstErrorCancelsWorkers", func(t *testing.T) {
		_, err := par.Map(ctx, input, func(ctx context.Context, i int, v int) (int, error) {
			if v == 2 {
				return 0, errors.New(want)
			}

			<-ctx.Done()

			return v * v, nil
		})
		if err == nil {
			t.Fatal("expected the boom error, got nil")
		}
		if err.Error() != "boom" {
			t.Fatalf("got %q, want %q", err, "boom")
		}
	})
}
