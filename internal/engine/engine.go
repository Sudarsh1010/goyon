// Package engine is goyon's single execution seam: every par operation
// schedules through Run (concurrent mode) or RunSeq (sequential mode).
// Swapping the concurrency implementation (errgroup today, custom worker
// pool tomorrow) changes only this package.
package engine

import (
	"context"
	"fmt"
	"runtime/debug"

	"golang.org/x/sync/errgroup"
)

// PanicError wraps a panic recovered from a work function.
type PanicError struct {
	Value any
	Stack []byte
	Index int // unit index; -1 when unknown
}

func (e *PanicError) Error() string {
	return fmt.Sprintf("par: worker panic at index %d: %v", e.Index, e.Value)
}

func (e *PanicError) Unwrap() error {
	if err, ok := e.Value.(error); ok {
		return err
	}
	return nil
}

// Run executes fn for each unit concurrently, at most concurrency at a
// time, on a derived context that is cancelled on the first error or
// panic. Units must be independent.
func Run[T any](
	ctx context.Context,
	units []T,
	concurrency int,
	fn func(ctx context.Context, i int, u T) error,
) error {
	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(max(concurrency, 1))

	for i, u := range units {
		g.Go(func() (err error) {
			defer func() {
				if r := recover(); r != nil {
					err = &PanicError{Value: r, Stack: debug.Stack(), Index: i}
				}
			}()
			return fn(gctx, i, u)
		})
	}

	return g.Wait()
}

// RunSeq executes fn for each unit in order on the calling goroutine,
// stopping at the first error. Closures may capture state: execution is
// strictly sequential and this is the only guarantee that makes it safe.
// Indices passed to fn — and reported by PanicError — start at base.
func RunSeq[T any](
	ctx context.Context,
	units []T,
	base int,
	fn func(ctx context.Context, i int, u T) error,
) (err error) {
	cur := -1
	defer func() {
		if r := recover(); r != nil {
			err = &PanicError{Value: r, Stack: debug.Stack(), Index: cur}
		}
	}()

	for j, u := range units {
		cur = base + j
		if err := fn(ctx, cur, u); err != nil {
			return err
		}
	}

	return nil
}
