package par

import "runtime"

type cfg struct {
	concurrency int
	identity    any // travels untyped; recovered by the T that knows
	hasIdentity bool
	ordered     bool
	failFast    bool
}

// Option configures a par call.
type Option func(*cfg)

func resolveConfig(options ...Option) cfg {
	c := cfg{concurrency: runtime.GOMAXPROCS(0), failFast: true}
	for _, o := range options {
		o(&c)
	}
	return c
}

// WithConcurrency caps concurrent work functions. Minimum 1; default
// runtime.GOMAXPROCS(0).
func WithConcurrency(n int) Option {
	return func(c *cfg) {
		c.concurrency = max(n, 1)
	}
}

// WithIdentity seeds Reduce with v instead of T's zero value — right for
// product (1) or min (+Inf), unneeded for sum or concat.
func WithIdentity[T any](v T) Option {
	return func(c *cfg) {
		c.identity = v
		c.hasIdentity = true
	}
}

// WithOrderedReduce forces sequential left-fold semantics, for fn that is
// not associative. There is no honest parallel shortcut for those.
func WithOrderedReduce() Option {
	return func(c *cfg) {
		c.ordered = true
	}
}

// WithFailFast sets first-error-cancellation behavior. Default true; with
// false, every unit runs to completion and all errors are joined
// (errors.Is reaches each one). Sequential folds still stop at their
// first error — a fold cannot continue past one.
func WithFailFast(b bool) Option {
	return func(c *cfg) {
		c.failFast = b
	}
}
