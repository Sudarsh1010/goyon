// Package par provides generic data-parallel operations over slices:
// Map, ForEach, and friends — with bounded workers, first-error
// cancellation, and panic recovery. No channels and no WaitGroups appear
// in the API; all synchronization is internal and correct-once.
//
// # The Share Rule (safety contract)
//
// Go cannot enforce race-freedom at compile time, so par states it as a
// contract: a work function may READ anything, but may only WRITE (a) its
// own local variables, (b) the output value it returns, and (c) memory it
// exclusively owns. Never mutate captured variables without your own
// synchronization. The (ctx, i, v) parameters exist so the common case
// never needs captures at all; need a shared accumulator? Use Reduce —
// don't hand-roll one.
//
// # Guarantees
//
// Structured concurrency: every call returns only when all its goroutines
// have finished. No goroutine ever outlives the call.
//
// Fail-fast: the first error returned by a work function cancels the
// context passed to the remaining functions, and that first error — not
// the resulting cancellation noise — is what the call returns, once the
// running functions have finished.
//
// Work functions that need to respond promptly to cancellation should
// respect the context passed to them.
//
// Operations run with bounded concurrency, defaulting to
// runtime.GOMAXPROCS(0); see WithConcurrency.
package par
