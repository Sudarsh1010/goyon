//go:build naivebug

package bugs

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestNaiveCaptureRace reproduces the anonymous-function capture bug class:
// Tu et al. (ASPLOS'19), Figure 8 — 11 real bugs in the study, including
// Docker's loop-variable race.
//
// The paper's Figure 8 captures the loop variable itself:
//
//	for i := 17; i <= 21; i++ {
//	    go func() { fmt.Sprintf("v1.%d", i) }() // races with the loop
//	}
//
// Go 1.22+ made loop variables per-iteration, so that exact code no longer
// races. What the language can never fix is the deeper pattern: goroutines
// racing on a SHARED OUTER VARIABLE. That is what this test reproduces:
// every goroutine appends to the captured `results` slice without
// synchronization.
//
// The paper's own fix for Figure 8 — go func(i int) { ... }(i) — is
// "parameters over captures", which is goyon's P3 and the (ctx, i, v)
// signature. The paired TestPar* version (Phase 1) will use par.Map, whose
// API makes this bug structurally unnecessary.
//
// Expected failure: `go test -tags naivebug -race -run TestNaiveCaptureRace ./bugs/`
// reports a DATA RACE on the slice append, and usually lost updates too.
func TestNaiveCaptureRace(t *testing.T) {
	urls := make([]string, 20)
	for i := range urls {
		urls[i] = fmt.Sprintf("https://example.com/%d", i)
	}

	var wg sync.WaitGroup
	var results []string // captured by every goroutine: the shared outer variable

	for _, u := range urls {
		wg.Add(1)
		go func() {
			defer wg.Done()
			time.Sleep(time.Millisecond)            // force the overlap — a demo that can't reproduce the bug proves nothing
			results = append(results, "body of "+u) // DATA RACE: concurrent slice-header writes
		}()
	}
	wg.Wait()

	if len(results) != len(urls) {
		t.Fatalf("lost updates: got %d results, want %d", len(results), len(urls))
	}
}
