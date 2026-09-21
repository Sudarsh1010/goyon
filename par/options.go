package par

import "runtime"

type cfg struct {
	concurrency int
}

// Option configures a call to a par function.
type Option func(*cfg)

func defaultConfig() cfg {
	return cfg{
		concurrency: runtime.GOMAXPROCS(0),
	}
}

func resolveConfig(options ...Option) cfg {
	opts := defaultConfig()
	for _, o := range options {
		o(&opts)
	}
	return opts
}

// WithConcurrency sets a maximum concurrency for a par function call.
// by default, concurrency configured to runtime.GOMAXPROCS(0).
// values < 1 are treated as 1
func WithConcurrency(i int) Option {
	return func(c *cfg) {
		c.concurrency = max(i, 1)
	}
}
