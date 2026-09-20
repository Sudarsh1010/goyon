# goyon, Safe Data-Parallelism for Go

**Design sketch & phased roadmap**
Working name: `goyon` (a nod to Rust's `rayon`). Module path placeholder: `github.com/sudarsh1010/goyon`. Package name: `par`.

---

## 1. Positioning & Honest Scope

### What this library is

A **data-parallelism library** for Go: parallel `Map`/`ForEach`/`Reduce`/`Filter`/`Join` over slices, with structured concurrency, bounded workers, panic propagation, and cancellation; designed so that the bug patterns catalogued in *Understanding Real-World Concurrency Bugs in Go* (ASPLOS'19) are **structurally impossible or strongly discouraged**.

### What this library is NOT

- **Not a static race-freedom guarantee.** Rayon's headline ("no data races, enforced by the compiler") comes from Rust's ownership/`Send`/`Sync` type system. Go has no equivalent machinery. goyon's safety is delivered by *API shape* (parameters instead of captures, no exposed channels), *correct-once encapsulation* (WaitGroups, worker lifecycle), and *test-time verification* (`-race`, `goleak`).
- **Not a work-stealing scheduler.** Go's runtime (GMP) already work-steals goroutines across OS threads. goyon does not reimplement rayon's deque scheduler; it partitions work and lets the runtime schedule it.

### One-sentence pitch

> "Rayon's ergonomics for Go, designed against the empirical bug patterns of real-world Go concurrency code."

---

## 2. Design Principles (mapped to the paper's bug taxonomy)

| # | Principle | Bug pattern it eliminates (from the paper) |
| --- | ----------- | -------------------------------------------- |
| P1 | **No channels in the public API.** All synchronization is internal. | 58% of blocking bugs caused by message passing; unbuffered-channel deadlocks; `select` non-determinism bugs; `context`/`time` interplay bugs |
| P2 | **No user-managed `WaitGroup` / `sync.Once`.** | WaitGroup misuse (`Add` after `Wait`, `Wait` inside loop), double-close panics |
| P3 | **Parameters over captures.** Work functions receive `(ctx, index, value)`; the API passes loop variables explicitly. | 11 anonymous-function capture races (e.g., the Docker loop-variable race) |
| P4 | **Structured concurrency.** Every operation returns only when all its goroutines have finished (or the first error cancels the rest). No leaked goroutines, ever. | Goroutine leaks from missing sends/receives; orphaned child goroutines |
| P5 | **Bounded parallelism by default.** Default workers = `GOMAXPROCS(0)`, never unbounded fan-out. | Resource exhaustion; thundering-herd channel blocking |
| P6 | **Panics become errors.** A panic in a worker is recovered and returned as a typed error with stack; other workers are cancelled. | Crashes inside `go func()` taking down the process |
| P7 | **`context.Context` is first-class**; but created/owned by the library internally where lifecycle matters. | Bugs from shadowed/replaced `context.WithCancel` objects |
| P8 | **Order-preserving by default** for value-returning ops; explicitly-named unordered variants for speed. | Non-determinism assumptions (analogous to `select` randomness bugs) |
| P9 | **Verified under `-race` + `goleak` in CI**, with the paper's bug archetypes reproduced as regression tests. | The library itself becoming a source of the bugs it prevents |

---

## 3. Core API Surface

All functions are generic (Go 1.18+). Error-returning variants are the default; infallible convenience wrappers exist for pure functions.

### 3.1 Data-parallel operations (the "par_iter" layer)

```go
package par

// ForEach runs fn for every element. Returns on first error (cancelling
// remaining work) or when all elements are processed.
func ForEach[T any](ctx context.Context, items []T,
    fn func(ctx context.Context, i int, v T) error, opts ...Option) error

// Map applies fn to every element and returns results in input order.
// out[i] == fn(in[i]) for all i if no error occurs.
func Map[T, R any](ctx context.Context, items []T,
    fn func(ctx context.Context, i int, v T) (R, error), opts ...Option) ([]R, error)

// MapUnordered is Map without the ordering guarantee; faster for
// latency-sensitive pipelines where order doesn't matter.
func MapUnordered[T, R any](ctx context.Context, items []T,
    fn func(ctx context.Context, i int, v T) (R, error), opts ...Option) ([]R, error)

// Filter keeps elements for which pred returns true. Order-preserving.
func Filter[T any](ctx context.Context, items []T,
    pred func(ctx context.Context, i int, v T) (bool, error), opts ...Option) ([]T, error)

// Reduce folds items into an accumulator. fn MUST be associative and
// commutative; identity is the zero value unless WithIdentity is given.
// Deterministic reduction (left-fold semantics) available via WithOrderedReduce.
func Reduce[T any](ctx context.Context, items []T,
    fn func(ctx context.Context, a, b T) (T, error), opts ...Option) (T, error)
```

### 3.2 Fork-join (the "join/scope" layer)

```go
// Join runs all fns concurrently and returns when all complete.
// First error cancels the rest.
func Join(ctx context.Context, fns ...func(ctx context.Context) error) error

// Scope is the structured-concurrency primitive for dynamic spawning,
// analogous to rayon's scope: no goroutine outlives the Scope call.
type Scope struct { /* unexported */ }

func NewScope(ctx context.Context, opts ...Option) *Scope

// Go schedules fn inside the scope. NOT a raw goroutine spawn:
// captures the worker-limit semaphore and panic recovery.
func (s *Scope) Go(fn func(ctx context.Context) error)

// Wait blocks until every fn given to Go has returned. Returns the
// first error (if any). After Wait returns, the scope owns no goroutines.
func (s *Scope) Wait() error
```

### 3.3 Options

```go
type Option func(*config)

func WithConcurrency(n int) Option        // default: runtime.GOMAXPROCS(0)
func WithChunkSize(n int) Option          // default: adaptive (see §4.2)
func WithIdentity[T any](v T) Option      // Reduce zero/identity value
func WithOrderedReduce() Option           // deterministic fold
func WithFailFast(b bool) Option          // default true; false = collect all errors (errors.Join)
```

### 3.4 Errors

```go
// PanicError wraps a recovered worker panic.
type PanicError struct {
    Value any
    Stack []byte
    Index int // element index being processed, -1 for Join/Scope
}
func (e *PanicError) Error() string
func (e *PanicError) Unwrap() error
```

---

## 4. Internal Architecture

### 4.1 Execution engine (per call)

```
user call
   │
   ▼
partition items into chunks (adaptive, see 4.2)
   │
   ▼
spawn min(nChunks, concurrency) worker goroutines
   │        each worker: pull chunk indices from an internal
   │        buffered work queue → run fn(ctx, i, v) → write
   │        result into a preallocated results slot (out[i])
   ▼
first error / panic → cancel derived ctx → workers drain
   │
   ▼
internal WaitGroup → assemble ordered results → return
```

Key properties:

- **No unbuffered internal channels anywhere.** Work distribution uses a buffered index queue or an atomic counter (`atomic.Int64` fetch-add); chosen to make "blocked forever" structurally impossible. (This is P1 applied to our own implementation.)
- **Preallocated output slots** (`out[i]`) mean workers never write to shared cursors; each worker owns disjoint indices. This is the library-level equivalent of rayon's disjoint borrows: the API *convention* is that `fn` must not mutate shared state outside its `(i, v)`; inside, the engine itself only touches disjoint memory.
- **One internal `sync.WaitGroup` per call**, fully encapsulated (P2).

### 4.2 Adaptive chunking

Static chunking (`len/nWorkers`) is simple but suffers stragglers; per-element scheduling adds overhead. Default: start with chunks of `max(1, len/(workers*4))`, workers fetch-add over chunk boundaries. Tune with benchmarks in Phase 5. Exposed via `WithChunkSize`.

### 4.3 Cancellation & error flow

Each operation derives a `context.WithCancel` internally (P7; the user never sees it, can't shadow it). First error → cancel → workers observe `ctx.Done()` between chunks and exit. `FailFast=false` collects errors with `errors.Join` (Go 1.20+).

### 4.4 Panic recovery

Every worker runs under `defer recover()`. A recovered panic is stored (once, atomically) as `*PanicError`, triggers cancellation, and is returned by the top-level call. The process never dies from a worker panic (P6).

---

## 5. Safety Contract (documented, not enforced)

Because Go can't check this at compile time, the docs state rayon's rule as a **contract**:

> **The Share Rule:** a work function may read anything, but may only write (a) its own local variables, (b) the output value it returns, and (c) memory it exclusively owns. Never mutate captured variables unless you synchronize them yourself.

Supporting measures:

1. **API shape nudges compliance**; `(i, v)` parameters make the common capture mistake unnecessary (P3).
2. **`par.Reduce` exists precisely so users don't hand-roll shared accumulators.**
3. **A companion `go vet` analyzer (stretch goal, Phase 6)** flags loop-variable and outer-variable captures inside goyon callbacks that are assigned without synchronization; mechanically catching the paper's anonymous-function bug class.
4. **CI gate:** `go test -race ./...` + `uber.org/goleak` on every test.

---

## 6. Phased Roadmap

### Phase 0 - Foundations & Spec (week 1)

**Goal:** lock the API before writing engine code.

- Finalize API surface (§3) as a godoc skeleton; write the safety contract (§5).
- Prior-art review: `errgroup`, `sourcegraph/conc`, `golang.org/x/sync/semaphore`; document what goyon does differently (order preservation, adaptive chunking, PanicError, vet analyzer).
- Reproduce 4–6 bug archetypes from the paper as **failing example tests** (unbuffered-channel deadlock, loop-capture race, WaitGroup-in-loop, double-close panic, goroutine leak).
- **Exit criteria:** API reviewed (self-review checklist or one external reviewer), archetype tests failing on naïve implementations, repo + CI scaffolding (`go test -race`, `go vet`, golangci-lint, goleak) green.

### Phase 1 - Core engine: ForEach + Map (weeks 2–3)

**Goal:** the smallest end-to-end vertical slice.

- Execution engine (§4.1–4.4): bounded workers, atomic-counter work distribution, preallocated ordered output, internal context, panic recovery, first-error cancellation.
- Implement `ForEach`, `Map`, `WithConcurrency`.
- **Exit criteria:** archetype tests now pass when rewritten with goyon; `go test -race` clean; `goleak` clean; naive microbenchmark vs. raw `errgroup` within ~10% overhead.

### Phase 2 - Completing the data-parallel layer (weeks 3–4)

- `MapUnordered`, `Filter`, `Reduce` (+ `WithIdentity`, `WithOrderedReduce`), `WithFailFast`.
- Adaptive chunking (§4.2).
- **Exit criteria:** full slice-op coverage; property-based tests (`rapid` or `gopter`): Map ≡ sequential map on all inputs; Reduce with ordered mode ≡ left fold; unordered reduce tested with associative ops only.

### Phase 3 - Fork-join & structured concurrency (week 5)

- `Join`, `Scope` (§3.2), `PanicError` polish (typed, stack capture, `errors.As` support).
- Nesting semantics defined and tested (a `Scope` inside a `Map` callback; worker-limit interaction documented).
- **Exit criteria:** nested-usage tests pass under `-race`; docs include a "replacing `go` + `WaitGroup`" recipe table mapped to the paper's Figure 5/8/9 patterns.

### Phase 4 - Hardening & fuzzing (week 6)

- Fuzz tests for chunk boundaries (empty slice, 1 element, huge slice, `WithConcurrency(1)`, `n > len(items)`).
- Cancellation storm tests (cancel parent ctx at random points; assert no leak, no race, prompt return).
- Panic-in-worker tests (panic at first/middle/last element; assert siblings cancelled, single typed error).
- **Exit criteria:** 24h CI fuzz soak clean; `-race` + `goleak` clean across the matrix; coverage ≥ 90% on engine internals.

### Phase 5 - Benchmarks & real-world validation (weeks 7–8)

- Benchmark suite: goyon vs. sequential vs. `errgroup` vs. `conc` across workload sizes (1e2/1e4/1e6) and fn costs (ns/µs/ms).
- Port 2–3 real concurrent code paths (e.g., a batch image processor, a fan-out HTTP checker) from raw goroutines to goyon; measure lines-of-sync-code removed.
- Write the "bug-pattern → goyon idiom" guide, using the paper's actual bug examples as before/after.
- **Exit criteria:** no regression > 10% vs. hand-rolled on realistic workloads; doc guide complete.

### Phase 6 - Ecosystem & v1.0 (weeks 9–10, stretch)

- **Companion `go vet` analyzer** (`goyonvet`): flags unsynchronized writes to captured variables inside goyon callbacks, and loop-variable capture pitfalls. This is the closest Go can get to rayon's compile-time check; ship it as a separate module.
- README, pkg.go.dev polish, examples directory, semver v1.0.0.
- Optional later: streaming/pipeline API (`FromChan`/`ToChan` adapters kept deliberately *at the edges*, never in the core), `ParMap` for maps.

---

## 7. Explicit Non-Goals

- Custom work-stealing scheduler / thread pinning (the runtime already does this).
- Compile-time race-freedom guarantees (impossible in Go; the vet analyzer is the best-effort approximation).
- Channels/CSP reimplementation; goyon is the *shared-memory-free, channel-free* style; users who want pipelines keep using channels.
- Simulating Rust lifetimes. The Share Rule is documentation + tooling, not types.

## 8. Success Metrics

1. The paper's bug archetypes are expressible as tests that **cannot** be written against goyon's API without obvious deliberate effort.
2. Zero `-race` / `goleak` failures in CI, ever, on the library itself.
3. Porting real code to goyon removes ≥ 80% of hand-written synchronization lines.
4. `goyonvet` catches the loop-capture race in a naïve usage sample with zero false positives on the test corpus.
