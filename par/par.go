package par

import (
	"context"
)

// Map applies fn to each item in items concurrently and returns the results
// in the same order as the input items.
//
// The index passed to fn corresponds to the item's position in items, and the
// returned slice preserves that same ordering regardless of the order in which
// operations complete.
//
// At most the configured number of operations run concurrently. If fn returns
// an error, Map cancels the context passed to other operations and returns the
// error. Partial results are not returned.
//
// The context passed to fn is derived from ctx and is cancelled when ctx is
// cancelled or when another invocation of fn returns an error.
//
// An empty items slice returns an empty, non-nil result slice.
func Map[T, R any](
	ctx context.Context,
	items []T,
	fn func(ctx context.Context, i int, v T) (R, error),
	opts ...Option,
) ([]R, error) {
	results := make([]R, len(items))

	if err := run(
		ctx,
		items,
		func(gctx context.Context, i int, item T) error {
			result, err := fn(gctx, i, item)
			if err != nil {
				return err
			}

			results[i] = result
			return nil
		},
		opts...,
	); err != nil {
		return nil, err
	}

	return results, nil
}

// ForEach applies fn to each item in items concurrently.
//
// At most the configured number of operations run concurrently. If fn returns
// an error, ForEach cancels the context passed to other operations and returns
// the error after the running operations finish.
//
// The context passed to fn is derived from ctx and is cancelled when ctx is
// cancelled or when another invocation of fn returns an error.
//
// ForEach does not guarantee the order in which fn is executed.
func ForEach[T any](
	ctx context.Context,
	items []T,
	fn func(ctx context.Context, i int, v T) error,
	opts ...Option,
) error {
	return run(ctx, items, fn, opts...)
}
