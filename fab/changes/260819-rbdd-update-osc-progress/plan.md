# Plan: shll update emits OSC 9;4 terminal progress

**Change**: 260819-rbdd-update-osc-progress
**Intake**: `intake.md`

## Requirements

### CLI: OSC 9;4 progress reporter (`progress.go`)

#### R1: Reporter type and sequence forms
A new `progressReporter` type in `src/cmd/shll/progress.go` SHALL emit BEL-terminated OSC 9;4 (ConEmu/Windows Terminal) progress sequences through four methods, and every method MUST be a no-op when the reporter is disabled. Sequence pieces are named constants (code-quality.md: no magic strings).

| Method | Bytes emitted (unwrapped) |
|--------|---------------------------|
| `set(percent int)` | `"\x1b]9;4;1;{percent}\x07"` |
| `indeterminate()` | `"\x1b]9;4;3;0\x07"` |
| `errorState(percent int)` | `"\x1b]9;4;2;{percent}\x07"` |
| `remove()` | `"\x1b]9;4;0;0\x07"` |

- **GIVEN** an enabled reporter writing to a `bytes.Buffer`
- **WHEN** `set(50)` is called
- **THEN** the buffer contains exactly `"\x1b]9;4;1;50\x07"`

- **GIVEN** a disabled reporter
- **WHEN** any method is called
- **THEN** zero bytes are written

#### R2: Enablement gating — TTY only, NO_COLOR-independent
The constructor `newProgressReporter(w io.Writer, env func(string) string)` MUST enable emission only when `w` is a real terminal, determined via a swappable package-level seam `var progressWriterIsTTY = defaultProgressWriterIsTTY` (default: `w` is an `*os.File` AND `term.IsTerminal(fd)` — mirroring `defaultStdinIsTTY` in `ui.go`). The gate MUST NOT consult `NO_COLOR`: per the existing `stdinIsTTY` precedent, `NO_COLOR` governs styling; OSC 9;4 is terminal progress state.

- **GIVEN** a `bytes.Buffer` writer (never a TTY)
- **WHEN** the reporter is constructed via `newProgressReporter`
- **THEN** it is disabled and all methods write nothing

- **GIVEN** the seam forced true and `NO_COLOR` set in the environment
- **WHEN** `set(50)` is called
- **THEN** the sequence is still emitted

#### R3: tmux passthrough wrapping
When `env("TMUX")` is non-empty, every emitted sequence MUST be wrapped in the DCS tmux passthrough envelope: `"\x1bPtmux;"` + the sequence with each `\x1b` byte doubled + `"\x1b\\"`. Only ESC bytes are doubled; the BEL terminator is untouched.

- **GIVEN** an enabled reporter constructed with `env("TMUX") = "/tmp/tmux-1000/default,123,0"`
- **WHEN** `set(50)` is called
- **THEN** the buffer contains exactly `"\x1bPtmux;\x1b\x1b]9;4;1;50\x07\x1b\\"`

### CLI: `shll update` wiring (`update.go`)

#### R4: Lifecycle start and guaranteed removal
`runUpdate` MUST construct the reporter on its `stderr` writer at write-phase start (alongside the existing `start := nowFunc()`, i.e. after the dry-run return), immediately `defer remove()`, and emit `indeterminate()` before the run-wide `brew update --quiet`. Every post-construction exit path — the brew-update-failure `errSilent` return, the success return, a panic — MUST end with `remove()` (the defer guarantees this).

- **GIVEN** `brew update --quiet` fails
- **WHEN** `runUpdate` returns `errSilent`
- **THEN** a `remove()` sequence was emitted after the `indeterminate()`

#### R5: Determinate progress and error states
Inside the existing `updateHeader` closure, each header MUST emit `set((pos-1)*100/total)` (integer division — progress completed so far, using the precomputed `pos`/`total`). Each failure site — self-upgrade error or non-zero code, `upgradeTool` error or non-zero code — MUST emit `errorState(pos*100/total)` (the failing tool's slot consumed). After the roster loop, before the agent-skill refresh and digest, `runUpdate` MUST emit `errorState(100)` when `anyFailed` else `set(100)`; the deferred `remove()` then clears the state at exit.

- **GIVEN** shll-self plus one roster tool installed (total = 2), all succeeding
- **WHEN** the run completes
- **THEN** the stderr emission order is `indeterminate()`, `set(0)`, `set(50)`, `set(100)`, `remove()`

- **GIVEN** the same run where the roster tool fails
- **WHEN** the run completes
- **THEN** an `errorState(100)` follows the failure (position 2 of 2) and the tail emits `errorState(100)`, then `remove()`

#### R6: No emission on non-write paths
The dry-run path, the `noToolsInstalledMsg` short-circuit, and every error return before the write phase (unknown target, `hasBrew` failure, named-but-not-installed) MUST emit no OSC bytes — the reporter is simply not constructed there.

- **GIVEN** `--dry-run`
- **WHEN** `runUpdate` returns
- **THEN** stderr contains no `"\x1b]9;4"` substring

#### R7: Output compatibility
stdout framing (status line, headers, summary tail, digest, previews) MUST be byte-identical to before this change, and on a non-TTY stderr the feature is entirely inert — all existing `update_test.go` golden-string assertions pass unmodified.

- **GIVEN** the existing test suite driving `runUpdate` with `bytes.Buffer` writers
- **WHEN** the suite runs after this change
- **THEN** every pre-existing assertion passes without edits

### Non-Goals

- `shll install` and other commands — rbdd's decided scope is `shll update` only; the pattern can extend later.
- The producer `update` standard (`docs/site/standards/update.md`) — untouched; no roster tool emits anything. Revisit only when a second emitter appears.
- Progress coverage of the concurrent probe phase — it precedes the dry-run branch, and constructing the reporter there would emit on no-write paths (see Assumptions).

### Design Decisions

#### Progress emission is consumer-side, not a toolkit standard
**Decision**: Implement OSC 9;4 emission directly in `shll update`; do not add it to the producer-facing `update` standard.
**Why**: OSC 9;4 is a singleton terminal channel — one progress state per terminal. shll delegates with inherited stdio, so per-tool emission inside the compose would conflict (a tool's terminal `remove` clears shll's roster-level bar mid-loop). Only the orchestrator knows "tool N of M", which is the signal the run-kit tile consumer wants. The existing standard explicitly assigns consumer-side compose behavior to shll.
**Rejected**: A producer-standard clause obligating each roster tool to emit — requires a remove/re-assert coordination protocol plus a 6-repo rollout while zero tools emit today; standardize from working practice when a second emitter appears.
*Introduced by*: 260819-rbdd-update-osc-progress

#### stderr channel, TTY-gated, NO_COLOR-independent
**Decision**: Emit on stderr, enabled only when stderr is a real TTY, independent of `NO_COLOR`.
**Why**: The framing headers/tail must stay on stdout (stream discipline with foregrounded sub-tool output); OSC is an invisible control channel and stderr keeps it out of piped stdout while still reaching the terminal. `NO_COLOR` governs styling per no-color.org — the codebase already draws this line (`defaultStdinIsTTY` ignores `NO_COLOR` for interactivity).
**Rejected**: stdout emission (pollutes `shll update | tee` capture ordering concerns and couples to the color gate); honoring `NO_COLOR` (wrong scope — it is a styling convention, and the TTY gate already excludes pipes/CI).
*Introduced by*: 260819-rbdd-update-osc-progress

## Tasks

### Phase 2: Core Implementation

- [x] T001 Create `src/cmd/shll/progress.go`: OSC 9;4 sequence constants, `progressReporter` type, `newProgressReporter(w, env)` constructor, the `progressWriterIsTTY` swappable seam (default mirrors `defaultStdinIsTTY`), the four methods (`set`/`indeterminate`/`errorState`/`remove`), and the tmux DCS-passthrough wrap (ESC-doubling) applied when `env("TMUX")` is non-empty. No subprocess calls. <!-- R1 R2 R3 -->
- [x] T002 [P] Create `src/cmd/shll/progress_test.go`: byte-exact assertions for all four sequences, disabled-reporter no-op, `bytes.Buffer` constructor disablement, NO_COLOR-set-still-emits (seam forced true), and tmux-wrap ESC-doubling. <!-- R1 R2 R3 -->

### Phase 3: Integration & Edge Cases

- [x] T003 Wire the reporter into `runUpdate` in `src/cmd/shll/update.go`: construct on `stderr` + `defer remove()` + `indeterminate()` at write-phase start (with `start := nowFunc()`); `set((pos-1)*100/total)` inside the `updateHeader` closure; `errorState(pos*100/total)` at the self-upgrade and `upgradeTool` failure sites; `errorState(100)`/`set(100)` after the loop before the refresh/digest. No emission on dry-run, short-circuit, or pre-write error paths. <!-- R4 R5 R6 -->
- [x] T004 Extend `src/cmd/shll/update_test.go`: emission-order tests with the seam forced true (full-success order; failed-tool error pulse + error tail; brew-update-failure deferred remove), zero-OSC assertions for dry-run and no-tools runs, and confirm all pre-existing golden assertions pass unmodified. <!-- R5 R6 R7 -->

### Phase 4: Polish

- [x] T005 Run `gofmt`, `go vet`, and the full `go test ./...` suite from `src/`; confirm the build via `just build`. <!-- R7 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The four methods emit the exact BEL-terminated OSC 9;4 forms when enabled, and a disabled reporter writes zero bytes — verified byte-exact in `progress_test.go`
- [x] A-002 R2: `newProgressReporter` disables for non-TTY writers (`bytes.Buffer` test), and emission is independent of `NO_COLOR` (test with seam forced true and `NO_COLOR` set)
- [x] A-003 R3: With `TMUX` set, every sequence is wrapped in the DCS envelope with each ESC doubled — verified byte-exact
- [x] A-004 R4: `remove()` is emitted on the success path and on the post-construction brew-update-failure path (deferred), verified by test

### Behavioral Correctness

- [x] A-005 R5: A full-success run emits `indeterminate` → `set(0)` → … → `set(100)` in order; a failed-tool run includes an `errorState` pulse at the failure and an `errorState(100)` tail

### Scenario Coverage

- [x] A-006 R6: Dry-run and no-tools-installed runs emit zero OSC bytes on stderr

### Edge Cases & Error Handling

- [x] A-007 R5: Percent math stays within 0–100 for all totals (including total = 1, whole-run shll-self-only: `set(0)` then `set(100)`); integer division never divides by zero (emission sites are only reachable with total ≥ 1)

### Code Quality

- [x] A-008 Pattern consistency: the seam mirrors the `stdinIsTTY` swappable-var pattern; all sequence pieces are named constants (no magic strings); comments follow ui.go's density and rationale style
- [x] A-009 No unnecessary duplication: one shared emit/wrap helper inside `progressReporter`; `progress.go` makes no subprocess calls (Constitution I — presentation only) and reuses the existing `env` injection seam
- [x] A-010 Existing tests untouched: no pre-existing golden-string assertion in `update_test.go` is modified (R7's byte-identical stdout guarantee)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

None — this change adds new functionality without making existing code redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | TTY gating via a swappable package var `progressWriterIsTTY` rather than threading a bool parameter | Mirrors the established `stdinIsTTY` seam; keeps `runUpdate`'s signature unchanged; tests force it deterministically | S:70 R:90 A:85 D:75 |
| 2 | Certain | Percent formula: `(pos-1)*100/total` at headers, `pos*100/total` at failure pulses, integer division | Intake-specified header formula; the failure pulse uses the slot-consumed percent so the next header's `set` is monotonic at the same value | S:80 R:90 A:85 D:80 |
| 3 | Confident | Reporter constructed after the probe phase (write-phase start), so the concurrent probes run uncovered | Probes are fast and read-only; the dry-run branch sits between probes and write-phase, and constructing earlier would emit on no-write paths (violating R6) | S:65 R:90 A:80 D:70 |

3 assumptions (2 certain, 1 confident, 0 tentative).
