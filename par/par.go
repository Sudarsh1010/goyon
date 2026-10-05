package par

import (
	"context"
	"sync/atomic"

	"github.com/sudarsh1010/goyon/internal/engine"
)

// Map applies fn to each item concurrently and returns results in input
// order. At most the configured number of operations run at once; on the
// first error or panic the rest are cancelled and the error is returned.
// Partial results are not returned. An empty items slice returns an
// empty, non-nil result slice.
func Map[T, R any](
	ctx context.Context,
	items []T,
	fn func(ctx context.Context, i int, v T) (R, error),
	opts ...Option,
) ([]R, error) {
	results := make([]R, len(items))

	err := run(ctx, items, func(gctx context.Context, i int, item T) error {
		r, err := fn(gctx, i, item)
		if err != nil {
			return err
		}
		results[i] = r
		return nil
	}, opts...)
	if err != nil {
		return nil, err
	}

	return results, nil
}

// ForEach applies fn to each item concurrently, with the same bounds,
// cancellation, and panic semantics as Map. Execution order is not
// guaranteed.
func ForEach[T any](
	ctx context.Context,
	items []T,
	fn func(ctx context.Context, i int, v T) error,
	opts ...Option,
) error {
	return run(ctx, items, fn, opts...)
}

// Reduce folds items into a single value, seeding with the identity (the
// zero value of T, or WithIdentity). fn MUST be associative and
// commutative: items are partitioned into contiguous chunks folded in
// parallel into private partials, then combined. WithOrderedReduce forces
// a sequential left fold for non-associative fn.
func Reduce[T any](
	ctx context.Context,
	items []T,
	fn func(ctx context.Context, a, b T) (T, error),
	opts ...Option,
) (T, error) {
	config := resolveConfig(opts...)

	var identity T
	if config.hasIdentity {
		identity = config.identity.(T)
	}

	if config.ordered {
		return foldSeq(ctx, identity, items, 0, fn)
	}

	numChunks := min(config.concurrency, len(items))
	if numChunks == 0 {
		return identity, nil
	}
	chunkSize := (len(items) + numChunks - 1) / numChunks
	numChunks = (len(items) + chunkSize - 1) / chunkSize // last chunk short, never empty

	partials := make([]T, numChunks)
	chunks := make([]int, numChunks)
	for i := range chunks {
		chunks[i] = i
	}

	err := run(ctx, chunks, func(gctx context.Context, c int, _ int) error {
		lo := c * chunkSize
		hi := min(lo+chunkSize, len(items))

		acc, err := foldSeq(gctx, identity, items[lo:hi], lo, fn)
		if err != nil {
			return err
		}
		partials[c] = acc
		return nil
	}, opts...)
	if err != nil {
		return identity, err
	}

	return foldSeq(ctx, identity, partials, 0, fn)
}

// Filter keeps the items for which pred returns true, preserving input
// order. Bounds, cancellation, and panic semantics match Map.
func Filter[T any](
	ctx context.Context,
	items []T,
	pred func(ctx context.Context, i int, v T) (bool, error),
	opts ...Option,
) ([]T, error) {
	keep := make([]bool, len(items))

	err := run(ctx, items, func(gctx context.Context, i int, v T) error {
		ok, err := pred(gctx, i, v)
		if err != nil {
			return err
		}
		keep[i] = ok
		return nil
	}, opts...)
	if err != nil {
		return nil, err
	}

	out := make([]T, 0, len(items))
	for i, v := range items {
		if keep[i] {
			out = append(out, v)
		}
	}
	return out, nil
}

// MapUnordered is Map without the ordering guarantee: results appear in
// completion order. Same multiset as Map, same error and panic semantics.
func MapUnordered[T, R any](
	ctx context.Context,
	items []T,
	fn func(ctx context.Context, i int, v T) (R, error),
	opts ...Option,
) ([]R, error) {
	results := make([]R, len(items))
	var next atomic.Int64

	err := run(ctx, items, func(gctx context.Context, i int, v T) error {
		r, err := fn(gctx, i, v)
		if err != nil {
			return err
		}
		results[next.Add(1)-1] = r // completion-order slot: lock-free append
		return nil
	}, opts...)
	if err != nil {
		return nil, err
	}

	return results, nil
}

func run[T any](ctx context.Context, units []T, fn func(context.Context, int, T) error, opts ...Option) error {
	config := resolveConfig(opts...)
	return engine.Run(ctx, units, config.concurrency, config.failFast, fn)
}

// foldSeq threads acc through the sequential engine; capture is safe
// because RunSeq is strictly sequential.
func foldSeq[T any](
	ctx context.Context,
	acc T,
	items []T,
	base int,
	fn func(ctx context.Context, a, b T) (T, error),
) (T, error) {
	err := engine.RunSeq(ctx, items, base, func(ctx context.Context, _ int, item T) error {
		var ferr error
		acc, ferr = fn(ctx, acc, item)
		return ferr
	})
	return acc, err
}
