package bugs

import (
	"context"
	"testing"
	"time"

	"github.com/sudarsh1010/goyon/par"
)

// TestParWaitGroupWaitInLoop is the paired exhibit for
// TestNaiveWaitGroupWaitInLoop (Docker#25384). The naïve shape deadlocks
// because Wait() runs inside the spawn loop with the counter pre-set to
// len(plugins). In par there is no counter and no Wait() for the user to
// misplace: every item gets its goroutine, and ForEach returns only after
// all of them finish. The test simply completing proves the absence of
// circular wait; the assertions prove every plugin actually loaded.
func TestParWaitGroupWaitInLoop(t *testing.T) {
	ctx := t.Context()
	plugins := []string{"auth", "cache", "metrics", "tracing"}

	loaded := make([]bool, len(plugins))
	err := par.ForEach(ctx, plugins, func(ctx context.Context, i int, name string) error {
		loaded[i] = true // load plugin
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for i, ok := range loaded {
		if !ok {
			t.Fatalf("plugin %q was never loaded", plugins[i])
		}
	}
}

// TestParWaitGroupAddAfterWait is the paired exhibit for
// TestNaiveWaitGroupAddAfterWait. The naïve shape lets Wait() return before
// the work completes, silently skipping synchronization. par has no Add()
// to place too late: ForEach returning IS the guarantee that all work is
// done. (Each goroutine writes only its own done[i] — disjoint slots, so
// the only thing on display is the awaited-work guarantee.)
func TestParWaitGroupAddAfterWait(t *testing.T) {
	ctx := t.Context()
	input := make([]int, 8)
	for i := range input {
		input[i] = i
	}

	done := make([]bool, len(input))
	err := par.ForEach(ctx, input, func(ctx context.Context, i int, v int) error {
		time.Sleep(20 * time.Millisecond) // the "work"
		done[i] = true
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}

	for i, ok := range done {
		if !ok {
			t.Fatalf("ForEach returned before work item %d completed: work silently un-awaited", i)
		}
	}
}
