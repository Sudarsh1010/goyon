//go:build naivebug

package bugs

import (
	"sync"
	"testing"
)

// TestNaiveDeadlock reproduces the unbuffered-channel blocking bug:
// Tu et al. (ASPLOS'19), Figure 1 — the study found 58% of blocking bugs
// came from message passing, with unbuffered-channel misuse the classic
// shape. The real-world fix was to make the channel buffered.
//
// Shape: N workers send results into an unbuffered channel. The consumer
// only needs the first result, reads it, and stops receiving. The remaining
// senders block on `ch <-` forever, so wg.Wait() never returns.
//
// A deadlock cannot fail by assertion — nobody is left running to assert.
// The failure voice is the timeout:
//
//	go test -tags naivebug -timeout 3s -run TestNaiveDeadlock ./bugs/
//
// panic: test timed out — with workers parked at "chan send" in the dump.
//
// goyon's fix is stronger than the paper's buffered-channel fix (§4.1):
// no unbuffered internal channels anywhere, and no channels in the public
// API at all (P1).
func TestNaiveDeadlock(t *testing.T) {
	ch := make(chan int) // unbuffered: the Figure 1 shape

	var wg sync.WaitGroup
	for i := range 4 {
		wg.Add(1)
		go func(v int) {
			defer wg.Done()
			ch <- v * v // blocks forever once the consumer stops receiving
		}(i)
	}

	first := <-ch // consumer takes one value and walks away
	t.Logf("got first result %d, waiting for workers...", first)

	wg.Wait() // never returns: 3 senders are blocked on ch forever
	t.Log("unreachable")
}
