package bench_test

import (
	"context"
	"fmt"
	"runtime"
	"testing"

	"github.com/sudarsh1010/goyon/par"
	"golang.org/x/sync/errgroup"
)

// benchSink defeats dead-code elimination: a write to a package-level
// variable is observable, so the benchmarked call cannot be optimized away.
var benchSink any

func double(v int) int {
	return v * 2
}

func BenchmarkMap(b *testing.B) {
	ctx := b.Context()

	for _, n := range []int{100, 10_000} {
		input := make([]int, n)
		for i := range input {
			input[i] = i
		}

		b.Run(fmt.Sprintf("n=%d", n), func(b *testing.B) {
			b.Run("par", func(b *testing.B) {
				for i := 0; i < b.N; i++ {
					results, err := par.Map(ctx, input, func(ctx context.Context, i int, v int) (int, error) {
						return double(v), nil
					})
					if err != nil {
						b.Fatal(err)
					}
					benchSink = results
				}
			})

			b.Run("errgroup", func(b *testing.B) {
				// Honest baseline: exactly what par.Map wraps. All per-call
				// setup (group, limit, results slice) is inside the loop
				// par pays those costs per call too.
				for i := 0; i < b.N; i++ {
					g, _ := errgroup.WithContext(ctx)
					g.SetLimit(runtime.GOMAXPROCS(0))

					results := make([]int, len(input))
					for j, v := range input {
						g.Go(func() error {
							results[j] = double(v)
							return nil
						})
					}
					if err := g.Wait(); err != nil {
						b.Fatal(err)
					}
					benchSink = results
				}
			})
		})
	}
}
