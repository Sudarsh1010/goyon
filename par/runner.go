package par

import (
	"context"
	"runtime/debug"

	"golang.org/x/sync/errgroup"
)

func run[T any](
	ctx context.Context,
	items []T,
	fn func(context.Context, int, T) error,
	opts ...Option,
) error {
	config := resolveConfig(opts...)

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(config.concurrency)

	for i, item := range items {
		g.Go(func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = &PanicError{
						Value: r,
						Stack: debug.Stack(),
						Index: i,
					}
				}
			}()

			return fn(gctx, i, item)
		})
	}

	return g.Wait()
}
