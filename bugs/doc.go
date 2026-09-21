// Package bugs contains reproductions of real-world Go concurrency bug
// archetypes from Tu et al., "Understanding Real-World Concurrency Bugs in
// Go" (ASPLOS'19) — the empirical foundation of goyon's design.
//
// These tests are DESIGNED TO FAIL against naïve implementations: the race
// detector fires, or the test deadlocks until -timeout kills it. They prove
// the bug classes are real and reproducible, and they stand as "before"
// exhibits — each archetype will gain a paired TestPar* test that solves the
// same problem with the par package and must stay green forever.
//
// Archetype tests are excluded from the default test run behind the
// "naivebug" build tag, so CI's go test ./... stays green. Run them
// explicitly:
//
//	go test -tags naivebug -race -timeout 10s -v ./bugs/
package bugs
