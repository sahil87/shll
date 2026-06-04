package main

import "time"

// nowFunc is the package-level wall-clock seam used to measure how long a
// `shll update` / `shll install` run took, so the summary tail can append a
// run duration (e.g. `in 1m12s`). It mirrors the `proc.Runner` injection seam
// (internal/proc/proc.go): production wiring uses the real clock, and tests swap
// it for a deterministic clock via a t.Cleanup helper (installFakeClock) so the
// golden summary-tail strings stay exact rather than racing a real wall clock.
//
// The duration is a fact about the run, not an outcome claim — the summary tail
// still never asserts "updated" vs. "up-to-date" (the honesty constraint).
var nowFunc = time.Now
