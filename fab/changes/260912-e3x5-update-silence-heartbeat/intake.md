# Intake: Still-Waiting Heartbeat for Silent Update Children

**Change**: 260912-e3x5-update-silence-heartbeat
**Created**: 2026-09-12

## Origin

Conversational — emerged from a debugging session (2026-09-12) on a `shll update shll` run that appeared to hang.

> Can you check why `shll udpdate shll` is getting stuck

> open a fix in the shll repo (new worktree - then send PR)

Diagnosis established before any fix was proposed:

- Every read-only step was fast: `shll update shll --dry-run` (all probes) finished in 1.2s, `brew update --quiet` in 1.1s. The wait was entirely inside `brew upgrade sahil87/tap/shll`.
- `brew fetch sahil87/tap/shll` never finished inside a 180s cap. The 0.1.31 release asset exists; the download of it from `release-assets.githubusercontent.com` was crawling: one of GitHub's four anycast IPs (`185.199.109.133`) did not connect at all from the machine, each connect attempt stalled ~10s, brew's own HEAD probe of the asset took 45s, and a direct `curl -L` of the 4.7MB bottle took 34s. A core bottle from `ghcr.io` fetched in 1.5s — the degradation was route-specific.
- Homebrew 6's download queue (`download_queue.rb`) renders progress only when stdout is a TTY and goes quiet whenever its download concurrency is above 1. shll's streamed-tail transport hands brew a pipe, so between `==> Fetching shll from sahil87/tap` and the finished download the terminal showed nothing.
- shll deliberately imposes no deadline on brew (the update standard's brew-handling safety clause), so nothing else ever broke the silence. A slow download was indistinguishable from a hang.

Key decisions from the discussion:

- **Fix the visibility, not the wait.** The user-facing failure is a *silent* wait. A still-waiting heartbeat on stderr after 30s of child silence — naming the exact child argv and the silence so far — converts it into a legible wait. shll still never bounds or signals brew.
- **No bounded `brew fetch` phase.** The agent initially floated a capped, interruptible `brew fetch` before the unbounded `brew upgrade`, then withdrew it: killing a brew child orphans its `curl` (observed live — the SIGTERM'd brew left `curl` running against the very `.incomplete` file the next attempt resumes), and the update standard forbids short timeouts around brew.
- **No `HOMEBREW_DOWNLOAD_CONCURRENCY=1` default.** Also floated (it makes brew print `==> Downloading <url>` on a pipe), then dropped for this change: it silently changes brew's download behavior for every shll-spawned brew (and, by inheritance, every delegated `<tool> update`'s brew) and contradicts the documented no-environment-injection posture, for the marginal gain of a URL line. Recorded as a candidate follow-up.
- **Back-off, not a drumbeat.** The required silence doubles after each line (30s, 1m, 2m, 4m, …) so a long stall yields a short series (principle №9 — bounded, high-signal output); any child output resets both the clock and the threshold.

## Why

**Problem**: a brew download that crawls — a degraded route, a stalled CDN edge — turns `shll update` (and `shll install`) into a blank terminal for minutes. brew's own progress UI is TTY-gated and shll pipes brew, so the last visible line is `==> Fetching …`; shll's write phase has no deadline by design, so nothing ever follows. The user cannot tell a slow download from a wedged process, and the natural reaction — Ctrl-C and retry — is exactly the mid-transaction interruption the brew-safety clause exists to avoid.

**Consequence of not fixing**: every slow-network `shll update` reads as a bug in shll. Users kill it (risking a half-swapped keg), or stop trusting the command, or file "shll update hangs" reports whose root cause is a network route shll cannot see.

**Why this approach**: the toolkit's own standards already pin the constraints — no short timeout on `brew upgrade`, no `SIGKILL` to brew, `SIGTERM` only with a generous grace (`docs/site/standards/update.md`), and the streamed-tail transport's live tee is the seam every write-phase child already rides (`runStreamedChild`, `brew.go`). Watching that tee for silence and printing a bounded, backing-off still-waiting line to stderr fixes the perceived hang completely without touching brew's behavior, the transport contract, exit codes, or any golden output — an instant child stays byte-identical. Visibility beats a deadline here because the wait is brew's and legitimately unbounded.

## What Changes

### Silence heartbeat in the shared streamed-child seam (src/cmd/shll/heartbeat.go, brew.go)

`runStreamedChild(ctx, stdout, stderr, argv...)` (`brew.go`) becomes a one-liner over a new `runStreamedChildWithHeartbeat(ctx, stdout, stderr, silence, poll, argv...)` (`heartbeat.go`) that:

1. Wraps both tee writers in an `activityWriter` — forwards bytes verbatim, stamps a `silenceWatch` on every write.
2. Runs the child through `proc.RunStreamedTail` exactly as before (null stdin, live tee, bounded tail ring; exit-code semantics unchanged).
3. Runs one watcher goroutine that polls the watch every `heartbeatPollInterval` (1s) and, when the child has been silent for the current threshold, prints one line to **stderr** from the named constant `childHeartbeatFmt`:

```
shll: still waiting on 'brew upgrade sahil87/tap/shll' (no output for 30s; a slow or stalled download is the usual cause, and no deadline is imposed)
```

4. Stops and joins the watcher before returning, so no heartbeat can land after the child's exit is reported.

`silenceWatch` owns the arithmetic: `touch(now)` resets the clock and the threshold; `due(now)` reports the silence and whether a heartbeat is due, doubling the threshold when it is (initial `childSilenceHeartbeat` = 30s → 1m → 2m → …). It runs on the real wall clock (`time.Now`), deliberately **not** the `nowFunc` seam (`clock.go`) — that seam feeds the summary tail's duration golden a scripted sequence of times, and a poller reading it would consume those values.

The heartbeat writes to the caller's stderr directly — not through the `activityWriter` (so it does not reset the clock) and not through `internal/proc` (so it never enters the tail ring; the tail stays pure child output). Because `shll install`'s write phase, `brew trust`, the relink heal's `brew link`, and every delegated `<tool> update` / `rk desktop update` already ride `runStreamedChild`, they all inherit the heartbeat with no per-command wiring. The end-of-run `shll setup agent` refresh keeps `proc.RunForeground` and is untouched.

### Help text and docs

- `shll update --help` / `shll install --help` Long: one sentence after "streams directly to your terminal" describing the 30s still-waiting line, the back-off, and that no deadline is imposed on brew.
- README `shll update` section and `docs/site/workflows.md` day-to-day walkthrough: the same sentence in user-facing prose.
- `docs/memory/cli/update.md`: a new *Silence heartbeat on write-phase children* section, a Design Decision (heartbeat over a deadline, a pty, a bounded fetch phase, or a brew env knob), the Test-seam list, and cross-reference touch-ups; `docs/memory/cli/install.md` points at it.

### Tests (src/cmd/shll/heartbeat_test.go)

- `silenceWatch` unit test with synthetic times: not due at 29s, due at 30s, doubling (45s not due, 60s due), reset on `touch`.
- `activityWriter` forwards bytes verbatim and counts as output.
- Real-time integration through `runStreamedChildWithHeartbeat` with millisecond thresholds and the existing fake `proc.Runner`: a silent child produces at least one heartbeat on stderr and nothing on stdout; a chatty child produces none and its lines are forwarded; the production `runStreamedChild` with an instant child stays byte-silent on both streams and passes a non-zero exit through as `(code, nil)`.

### Non-Goals

- Any deadline, timeout, or signal on brew — the standard forbids it and the heartbeat makes it unnecessary.
- A bounded `brew fetch` pre-phase — killing brew orphans curl (observed); repeats brew's own HEAD probe.
- Defaulting `HOMEBREW_DOWNLOAD_CONCURRENCY=1` for brew children — a candidate follow-up with its own trade-offs (fleet-wide download-behavior change, env injection), not part of this change.
- A pty for brew children — already rejected for the transport (yud0).
- Changing `internal/proc` — the heartbeat lives entirely at the `cmd/shll` seam; the transport, tail ring, and `Request` shape are untouched.

## Affected Memory

- `cli/update`: (modify) new *Silence heartbeat on write-phase children* section; new Design Decision; Test-seam additions; description + cross-reference touch-ups
- `cli/install`: (modify) one cross-reference sentence — install's write children inherit the heartbeat through `runStreamedChild`

## Impact

- `src/cmd/shll/heartbeat.go` (new), `src/cmd/shll/brew.go` (`runStreamedChild` delegates), `src/cmd/shll/heartbeat_test.go` (new), `src/cmd/shll/update.go` + `install.go` (Long help), `README.md`, `docs/site/workflows.md`, `docs/memory/cli/update.md`, `docs/memory/cli/install.md`.
- No change to `internal/proc`, argvs, exit codes, the dry-run preview, OSC progress, or any existing golden string (an instant child emits no heartbeat).

## Open Questions

- None blocking. Whether to also default `HOMEBREW_DOWNLOAD_CONCURRENCY=1` (URL visibility on a pipe) is deferred to a follow-up decision by the maintainer.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Heartbeat lives in the shared `runStreamedChild` seam so `update` and `install` (and trust/link/delegated children) all inherit it | Single existing choke point for every write-phase child; no per-command wiring | S:85 R:90 A:95 D:95 |
| 2 | Certain | No deadline, no signal to brew — the watcher only prints | Update standard's brew-safety clause; the live incident's own diagnosis showed killing brew orphans curl | S:95 R:95 A:95 D:95 |
| 3 | Confident | 30s initial threshold, doubling back-off (30s, 1m, 2m, …), reset on any output | Named constants, trivially retuned; back-off keeps a long stall to a handful of lines (principle №9) | S:70 R:90 A:80 D:75 |
| 4 | Confident | Plain-ASCII line on stderr naming the child argv, the silence so far, the usual cause, and that no deadline is imposed | stderr is diagnostics (principle №2); ASCII sidesteps the glyph-degrade rule; wording is the implementer's within those ingredients | S:70 R:90 A:85 D:80 |
| 5 | Confident | `HOMEBREW_DOWNLOAD_CONCURRENCY=1` default NOT adopted; recorded as a follow-up candidate | Agent proposed then withdrew it (env-injection posture, fleet-wide behavior change); user did not object; reversible in a later change | S:65 R:85 A:70 D:65 |
| 6 | Confident | Watch runs on `time.Now`, not the `nowFunc` seam; timing tests pass millisecond thresholds through `runStreamedChildWithHeartbeat` | The seam's scripted sequence would be consumed by a poller and break the duration golden; explicit parameters keep tests deterministic without mutable package state | S:80 R:90 A:85 D:85 |

6 assumptions (2 certain, 4 confident, 0 tentative, 0 unresolved).
