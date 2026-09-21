//go:build naivebug

package bugs

import (
	"testing"

	"go.uber.org/goleak"
)

// TestNaiveGoroutineLeak reproduces the goroutine-leak bug class: Tu et al.
// (ASPLOS'19) found leaks recurring across all six studied applications
// ("goroutine leak" is one of the study's keyword categories) — a goroutine
// parked forever on a channel operation nobody will ever complete, invisible
// to tests because nothing ever observes it.
//
// Shape: a worker spawned to deliver a result, but the receiver has walked
// away. The send blocks forever. The test function returns; the goroutine
// does not.
//
// Failure voice: goleak. go.uber.org/goleak inspects the process's
// goroutines at test end and fails if any outlived the test:
//
//	go test -tags naivebug -run TestNaiveGoroutineLeak ./bugs/
//
//	goleak: Errors on successful test run: found unexpected goroutines
//
// goyon's P4 (structured concurrency): every par call returns only when all
// its goroutines have finished — the API makes "the receiver walked away"
// unexpressible. goleak gates the library's own tests in CI (P9).
func TestNaiveGoroutineLeak(t *testing.T) {
	defer goleak.VerifyNone(t)

	ch := make(chan int) // unbuffered, and the only receiver is... nobody
	go func() {
		ch <- 42 // blocks forever: orphaned child goroutine
	}()

	// The test returns here, leaving the worker parked on chan send.
	// goleak catches what assertion-based tests cannot see.
}
