# Goyon

Safe, simple data-parallelism for Go; rayon's ergonomics, designed against the concurrency bug patterns found in real-world Go code.

> **Honest scope:** Go can't give you Rust's compile-time race-freedom. goyon gives you the next best thing: an API where the common bug classes (channel deadlocks, `WaitGroup` misuse, loop-variable capture races, goroutine leaks) are structurally avoided; verified with `-race` and `goleak` in CI, plus an optional `go vet` analyzer.

## Install

```bash
go get github.com/sudarsh1010/goyon
```

```go
import "github.com/sudarsh1010/goyon/par"
```

## Quick start

```go
// Parallel map, order-preserving, error + panic safe
urls := []string{"https://a", "https://b", "https://c"}
bodies, err := par.Map(ctx, urls, func(ctx context.Context, i int, u string) ([]byte, error) {
    return fetch(ctx, u) // i and u are PARAMETERS — no loop-capture races
})

// Parallel for-each with bounded concurrency
err := par.ForEach(ctx, jobs, process, par.WithConcurrency(8))

// Fork-join
err := par.Join(ctx,
    func(ctx context.Context) error { return migrateDB(ctx) },
    func(ctx context.Context) error { return warmCache(ctx) },
)
```

First error (or a worker panic, returned as `*par.PanicError`) cancels the rest. No goroutine ever outlives the call.

## API

| Function               | Purpose                                                 |
| ---------------------- | ------------------------------------------------------- |
| `Map` / `MapUnordered` | Transform each element; ordered by default              |
| `ForEach`              | Run a function per element                              |
| `Filter`               | Keep matching elements, order-preserving                |
| `Reduce`               | Fold to one value (`WithOrderedReduce` for determinism) |
| `Join` / `Scope`       | Fork-join; structured dynamic spawning                  |

Options: `WithConcurrency(n)` (default `GOMAXPROCS`), `WithChunkSize(n)`, `WithFailFast(false)` to collect all errors via `errors.Join`.

## Why not raw goroutines?

An empirical study of 171 concurrency bugs in Docker, Kubernetes, etcd, gRPC & co. (_Understanding Real-World Concurrency Bugs in Go_, ASPLOS'19) found:

- **58% of blocking bugs** came from message passing (channels, `select`) — goyon has **no channels in its public API**
- **11 bugs** from anonymous-function variable capture — goyon passes `(i, v)` as **parameters**
- **WaitGroup misuse** (`Add` after `Wait`, `Wait` inside loops) — goyon hides all WaitGroups internally

```go
// Before: three bug classes in five lines
var wg sync.WaitGroup
for i, u := range urls {
    wg.Add(1)
    go func() { defer wg.Done(); results[i] = fetch(u) }() // captures i, u!
}
wg.Wait()

// After
results, err := par.Map(ctx, urls, fetch)
```

## Design principles

1. **No channels in the public API**; all synchronization is internal. Message passing caused 58% of blocking bugs in the wild; users shouldn't have to touch it.
2. **No user-managed `WaitGroup` or `sync.Once`**; lifecycle primitives are encapsulated and correct-once.
3. **Parameters over captures**; work functions receive `(ctx, i, v)`; the API passes loop variables explicitly instead of letting you capture them.
4. **Structured concurrency**; every call returns only when all its goroutines finish. Nothing outlives the call.
5. **Bounded parallelism by default**; workers capped at `GOMAXPROCS`, never unbounded fan-out.
6. **Panics become errors**; a worker panic is recovered and returned as `*PanicError`, never crashes your process.
7. **Context is first-class but library-owned**; derived contexts are created and cancelled internally; users can't shadow or leak them.
8. **Order-preserving by default**; value-returning ops match sequential semantics; unordered variants are explicitly named.
9. **Verified, not trusted**; `-race` and `goleak` gate every commit, with the empirical bug archetypes as regression tests.

## The Share Rule (safety contract)

A work function may **read** anything, but may only **write** its own locals, its return value, or memory it exclusively owns. Need a shared accumulator? Use `par.Reduce`; don't hand-roll one.

The companion `goyonvet` analyzer (Phase 6, planned) flags unsynchronized writes to captured variables at `go vet` time.

## Guarantees

- Every test runs under `go test -race` and `uber.org/goleak`
- Worker panics never crash your process — they return as `*PanicError` with stack
- Bounded parallelism by default; no unbounded fan-out
- Cancellation is prompt and complete: zero leaked goroutines, ever

## Performance

Phase 1 gate: `par.Map` vs. a hand-rolled `errgroup` baseline doing identical work (design doc §6: within ~10%). Median of 10 runs, Ryzen 7 5700, Go 1.27.1:

| Benchmark                     |     ns/op |   B/op | allocs/op | overhead |
| ----------------------------- | --------: | -----: | --------: | -------: |
| `BenchmarkMap/n=100/par`      |    30,553 | 10,048 |       208 |     ~+4% |
| `BenchmarkMap/n=100/errgroup` |    29,335 |  8,392 |       206 |          |
| `BenchmarkMap/n=10000/par`    | 2,625,000 | 962,282 |    20,008 |   ~+1.3% |
| `BenchmarkMap/n=10000/errgroup` | 2,591,000 | 802,220 |    20,006 |        |

The wrapper costs a constant 2 allocations; time overhead amortizes with input size. Reproduce:

```bash
go test -run=XXX -bench=BenchmarkMap -benchmem -count=10 ./bench/
```

## Status

Phase 2 slice-op layer complete: `Map`, `MapUnordered`, `ForEach`, `Filter`, `Reduce` with `WithConcurrency`, `WithIdentity`, `WithOrderedReduce`, `WithFailFast` — all green under `-race` + `goleak`, benchmark gate passed, ASPLOS'19 bug archetypes as regression tests (`bugs/`). Next: adaptive chunking and the hand-rolled worker pool in `internal/engine` (design doc §4.1–4.2). Not yet released.

## License

MIT
