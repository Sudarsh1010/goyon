//go:build naivebug

package bugs

import (
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestNaiveWaitGroupWaitInLoop reproduces WaitGroup misuse, blocking flavor:
// Tu et al. (ASPLOS'19), Figure 5 — Docker#25384, the paper's exact shape:
//
//	group.Add(len(pm.plugins))
//	for _, p := range pm.plugins {
//	    go func(p *plugin) { defer group.Done() }(p)
//	    group.Wait() // inside the loop
//	}
//
// Add() counts ALL the plugins up front, but Wait() is called after spawning
// only the FIRST goroutine. The counter can never reach zero — later
// goroutines don't exist yet — so Wait() blocks forever, and with it the
// loop that would have spawned them. Circular wait, no locks involved.
//
// Failure voice: timeout.
//
//	go test -tags naivebug -timeout 3s -run TestNaiveWaitGroupWaitInLoop ./bugs/
//
// The dump shows the main goroutine parked at sync.WaitGroup.Wait.
// goyon's P2: users never touch a WaitGroup; lifecycle is encapsulated
// and correct-once inside the library.
func TestNaiveWaitGroupWaitInLoop(t *testing.T) {
	plugins := []string{"auth", "cache", "metrics", "tracing"}

	var group sync.WaitGroup
	group.Add(len(plugins))

	for _, p := range plugins {
		go func(name string) {
			defer group.Done()
			_ = name // load plugin
		}(p)

		group.Wait() // BUG: inside the loop. Counter is 4, one Done fired: blocks forever.
	}

	t.Log("unreachable")
}

// TestNaiveWaitGroupAddAfterWait reproduces WaitGroup misuse, non-blocking
// flavor: Tu et al. (ASPLOS'19), Figure 9 — Add() inside the goroutine
// instead of before go:
//
//	go func() {
//	    p.wg.Add(1) // too late: Wait() may already have returned
//	    ...
//	    p.wg.Done()
//	}()
//	p.wg.Wait()
//
// Wait() observes a zero counter and returns immediately; the goroutine's
// work is never awaited. The program "works" — it just silently skips the
// synchronization. No deadlock, no panic: the failure is a wrong result.
//
// Failure voice: assertion failure. The sleep before Add() widens the race
// window so Wait() always wins — deterministic, not flaky.
//
//	go test -tags naivebug -race -run TestNaiveWaitGroupAddAfterWait ./bugs/
//
// (done is atomic so the ONLY bug on display is the WaitGroup misuse; an
// unsynchronized flag would add a second, distracting race.)
func TestNaiveWaitGroupAddAfterWait(t *testing.T) {
	var wg sync.WaitGroup
	var done atomic.Bool

	go func() {
		time.Sleep(20 * time.Millisecond) // force Wait() to run before Add()
		wg.Add(1)                         // BUG: Add belongs BEFORE the go statement
		defer wg.Done()

		time.Sleep(20 * time.Millisecond) // the "work"
		done.Store(true)
	}()

	wg.Wait() // counter is 0: returns immediately, work never awaited

	if !done.Load() {
		t.Fatal("Wait() returned before the goroutine's work completed: work silently un-awaited")
	}
}
