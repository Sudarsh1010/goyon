package par

import "github.com/sudarsh1010/goyon/internal/engine"

// PanicError wraps a panic recovered from a work function. It never
// crashes the process; it is returned by the top-level call after the
// remaining work functions have been cancelled (P6).
type PanicError = engine.PanicError
