# Plan: Still-Waiting Heartbeat for Silent Update Children

**Change**: 260912-e3x5-update-silence-heartbeat
**Intake**: `intake.md`

## Requirements

### CLI: still-waiting heartbeat for silent write-phase children

#### R1: Heartbeat in the shared streamed-child seam
`runStreamedChild` (`src/cmd/shll/brew.go`) SHALL run every write-phase child through a helper that watches both tee'd streams for silence and, once the child has produced no byte on either stream for the initial threshold (`childSilenceHeartbeat`, 30s), SHALL print one still-waiting line. Because `shll update`, `shll install`, `brew trust`, the relink heal's `brew link`, and every delegated `<tool> update` / `rk desktop update` already ride `runStreamedChild`, they all inherit the behavior with no per-command wiring.

- **GIVEN** `shll update shll` running `brew upgrade sahil87/tap/shll` whose bottle download crawls
- **WHEN** 30s pass with no output from brew
- **THEN** stderr shows `shll: still waiting on 'brew upgrade sahil87/tap/shll' (no output for 30s; …)`
- **AND** brew keeps running untouched

- **GIVEN** `shll install` running `brew install sahil87/tap/wt` that goes quiet
- **WHEN** the same silence elapses
- **THEN** the same line appears naming `brew install sahil87/tap/wt` — no install-specific code

#### R2: Back-off, reset on output
The required silence SHALL double after each heartbeat (30s, 1m, 2m, 4m, …), and any child output SHALL reset both the silence clock and the threshold to the initial value.

- **GIVEN** a child silent since t=0 with the 30s initial threshold
- **WHEN** the watch is polled at 29s, 30s, 45s, 60s
- **THEN** heartbeats are due at 30s and 60s only

- **GIVEN** the child then writes at 70s
- **WHEN** the watch is polled at 99s and 100s
- **THEN** no heartbeat at 99s and one at 100s reporting 30s of silence

#### R3: Stream discipline and wording
The heartbeat SHALL go to stderr only (never stdout), SHALL be built from a named constant (`childHeartbeatFmt`), SHALL be plain ASCII, and SHALL carry the child's argv as rendered by `argvString`, the formatted silence so far (`formatDuration`), the usual cause, and the statement that no deadline is imposed.

- **GIVEN** a silent child `brew upgrade sahil87/tap/shll`
- **WHEN** a heartbeat fires
- **THEN** stdout is unchanged and stderr gains one line containing `still waiting on 'brew upgrade sahil87/tap/shll'` and `a slow or stalled download is the usual cause`

#### R4: Transport contract untouched
The watcher SHALL only print: no context cancellation, no signal, no deadline on the child (update standard, brew-handling safety). Exit-code semantics SHALL pass through `proc.RunStreamedTail` unchanged (non-zero exit via `code`, `err` only for exec failures). The heartbeat SHALL be written to the caller's stderr directly — not through the activity wrapper (it must not reset the clock) and not through `internal/proc` (it must not enter the bounded tail ring). The watcher SHALL be stopped and joined before the helper returns. `internal/proc` SHALL NOT change.

- **GIVEN** a child that exits 3 after some silence
- **WHEN** the helper returns
- **THEN** it returns `(3, nil)` and no heartbeat line lands after the return

#### R5: Byte-identical fast path
A child that produces output or exits before the initial threshold SHALL yield no heartbeat bytes, so every existing install/update golden string is unchanged.

- **GIVEN** the existing fake-runner tests (children return instantly)
- **WHEN** the suite runs
- **THEN** every golden passes unmodified

#### R6: Real clock, explicit test timings
The watch SHALL run on the real wall clock, not the `nowFunc` seam (`clock.go`), and the helper SHALL take its silence threshold and poll interval as parameters so tests drive real silence with millisecond values.

- **GIVEN** `TestUpdate_HeadersAndTail` with its scripted `installFakeClock` sequence
- **WHEN** the run's write-phase children go through the heartbeat helper
- **THEN** the `in 1m12s` duration golden is unaffected

#### R7: Documentation
`shll update --help` and `shll install --help`, the README `shll update` section, `docs/site/workflows.md`, and the memory files `docs/memory/cli/update.md` (new section, Design Decision, Test seam) and `docs/memory/cli/install.md` (cross-reference) SHALL describe the heartbeat.

- **GIVEN** a reader of `shll update --help`
- **WHEN** they reach the streaming sentence
- **THEN** it says what happens after 30s of silence and that no deadline is imposed on brew

### Non-Goals

- Any deadline, timeout, or signal on brew — forbidden by the standard; unnecessary once the wait is visible.
- A bounded `brew fetch` pre-phase — killing brew orphans its curl (observed); repeats brew's own HEAD probe.
- Defaulting `HOMEBREW_DOWNLOAD_CONCURRENCY=1` — a follow-up candidate with fleet-wide and env-injection trade-offs.
- A pty for brew children — already rejected for the transport (yud0).
- Changes to `internal/proc`, argvs, the dry-run preview, or OSC progress.

### Design Decisions

#### Heartbeat on silence, not a deadline, a pty, or a brew env knob
**Decision**: A silent write-phase child gets a still-waiting line on stderr after 30s of no output, backing off (doubling) between lines; shll still never bounds or signals brew.
**Why**: The failure the user experiences is a *silent* wait, not a wrong result — visibility fixes it fully. brew legitimately blocks for minutes on the network, so any bound would either fire on healthy slow runs or be too long to help, and the standard's brew-safety clause forbids short timeouts and `SIGKILL`. The line names the exact child argv and the silence so far — what a user needs to decide whether to keep waiting.
**Rejected**: a bounded `brew fetch` phase (killing brew orphans curl — observed live; the standard says no short timeouts; repeats brew's HEAD probe); a pty for brew children (new dependency and platform surface; tty progress bars would pollute the tail ring); defaulting `HOMEBREW_DOWNLOAD_CONCURRENCY=1` (adds the URL line but silently changes brew's download behavior for every shll-spawned brew and contradicts the no-environment-injection posture — a follow-up candidate); a fixed-interval heartbeat (a 10-minute stall would print 20 near-identical lines).
*Introduced by*: 260912-e3x5-update-silence-heartbeat

#### Real clock for the watch, explicit timings for tests
**Decision**: `silenceWatch` runs on `time.Now`; `runStreamedChildWithHeartbeat` takes the threshold and poll interval as parameters, and `runStreamedChild` passes the production constants.
**Why**: The `nowFunc` seam exists so the summary tail's duration golden can consume a scripted sequence of times; a watcher polling it would eat those values. Explicit parameters keep the timing tests deterministic without a mutable package var for the threshold.
**Rejected**: polling `nowFunc` (breaks the duration golden and races the scripted closure); a package-level threshold var swapped by tests (mutable state where a parameter suffices).
*Introduced by*: 260912-e3x5-update-silence-heartbeat

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add `src/cmd/shll/heartbeat.go`: `childSilenceHeartbeat`, `heartbeatPollInterval`, `childHeartbeatFmt` constants; `silenceWatch` (`touch`/`due` with doubling back-off); `activityWriter`; `runStreamedChildWithHeartbeat` (wraps the tee writers, runs `proc.RunStreamedTail`, polls from one goroutine, stops and joins it before returning); `heartbeatLoop` <!-- R1 R2 R3 R4 R6 -->
- [x] T002 Make `runStreamedChild` in `src/cmd/shll/brew.go` a one-liner over `runStreamedChildWithHeartbeat` with the production constants; refresh its doc comment <!-- R1 R5 -->
- [x] T003 Add `src/cmd/shll/heartbeat_test.go`: `silenceWatch` synthetic-time test, `activityWriter` forward+touch test, silent-child heartbeat integration test, chatty-child suppression test, production fast-path byte-silence + exit-code passthrough test <!-- R2 R3 R4 R5 R6 -->

### Phase 3: Integration & Edge Cases

- [x] T004 Run `gofmt -l`, `go vet ./...`, the scoped tests with `-race`, then the full `go test ./...` from `src/`; confirm every existing golden is untouched <!-- R5 R6 -->

### Phase 4: Polish

- [x] T005 Update the Long help of `shll update` (`src/cmd/shll/update.go`) and `shll install` (`src/cmd/shll/install.go`), the README `shll update` section, and `docs/site/workflows.md` with the heartbeat sentence <!-- R7 -->
- [x] T006 Update `docs/memory/cli/update.md` (description, null-stdin bullet, new *Silence heartbeat* section, Design Decision, Test seam, transport cross-reference) and `docs/memory/cli/install.md` (cross-reference); regenerate indexes with `fab docs-index docs/memory` <!-- R7 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: Every write-phase child of `update` and `install` goes through the heartbeat helper via the unchanged `runStreamedChild` call sites (no per-command wiring added)
- [x] A-002 R2: The watch is due at exactly the initial threshold, doubles after each heartbeat, and resets fully on any child output
- [x] A-003 R3: The heartbeat line is stderr-only, built from `childHeartbeatFmt`, plain ASCII, and names the child argv and the silence so far

### Behavioral Correctness

- [x] A-004 R4: The watcher never cancels, signals, or bounds the child; exit codes pass through as before; the line never enters the tail ring; the goroutine is joined before return
- [x] A-005 R5: An instant child yields zero heartbeat bytes — the full existing suite passes with no golden edits

### Scenario Coverage

- [x] A-006 R1 R3: A real-time test drives a silent fake child past a millisecond threshold and observes the heartbeat on stderr with an empty stdout
- [x] A-007 R2: A real-time test drives a chatty fake child and observes no heartbeat with all output forwarded

### Edge Cases & Error Handling

- [x] A-008 R4: A non-zero exit after silence still returns `(code, nil)` through the helper
- [x] A-009 R6: The heartbeat never reads `nowFunc`, so `TestUpdate_HeadersAndTail`'s scripted clock and `in 1m12s` golden are unaffected

### Code Quality

- [x] A-010 Pattern consistency: seam-style helper with production constants passed from the one-liner call site, matching the `runStreamedChild`/`proc.RunStreamedTail` shape and surrounding comment density
- [x] A-011 No unnecessary duplication: reuses `argvString`, `formatDuration`, the existing `fakeRunner`/`installFakeRunner` test seam; `internal/proc` untouched
- [x] A-012 No magic strings or numbers: `childSilenceHeartbeat`, `heartbeatPollInterval`, `childHeartbeatFmt` are named constants (code-quality.md)

### Security

- [x] A-013 R4: No new subprocess surface — the helper passes the same explicit argv slice to `proc.RunStreamedTail` (Constitution I) and sends no signal to any child

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

None — the change adds a watcher around an existing seam; no existing code becomes redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Timing tests use millisecond thresholds through the parameterized helper with the existing fake `proc.Runner` (a respond function that sleeps or writes) | Direct reuse of the mandated test seam; no real subprocess | S:85 R:90 A:95 D:95 |
| 2 | Confident | Heartbeat line written straight to the caller's stderr from the watcher goroutine while the child's tee may also write there | `os.Stderr` writes are whole-line syscalls in production; in tests only one side writes per scenario | S:70 R:85 A:80 D:80 |

2 assumptions (1 certain, 1 confident, 0 tentative).
