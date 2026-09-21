package par_test

import (
	"testing"

	"go.uber.org/goleak"
)

// P9: every par test run is gated on goroutine leaks — if any call leaves a
// goroutine behind, the whole suite fails, regardless of which test leaked.
func TestMain(m *testing.M) {
	goleak.VerifyTestMain(m)
}
