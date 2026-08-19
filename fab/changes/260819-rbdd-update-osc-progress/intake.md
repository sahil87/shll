# Intake: shll update emits OSC 9;4 terminal progress

**Change**: 260819-rbdd-update-osc-progress
**Created**: 2026-08-19

## Origin

Backlog item `[rbdd]`, refined in a `/fab-discuss` session (2026-08-20) before `/fab-new`:

> shll update emits OSC 9;4 progress (ConEmu/Windows Terminal convention) — run-kit tiles render it live via @xterm/addon-progress (rk sessions already have allow-passthrough on). DECIDED 2026-08-20: implement directly in shll update (consumer-side), NOT in the producer update standard — OSC 9;4 is a singleton terminal channel, so per-tool emission inside the compose would need a remove/re-assert coordination protocol + 6-repo rollout; standardize in update.md only when a second emitter appears. SHAPE: determinate 'set' keyed to roster position, 'indeterminate' during initial brew update + self-upgrade, 'error' pulse on a failed tool, 'remove' on ALL exit paths; gated on stderr-is-a-TTY; tmux-passthrough-wrapped (ESC Ptmux) when inside tmux

Key decisions from the discussion (all user-confirmed):

1. **Direct implementation in `shll update`, NOT the producer `update` standard.** The existing `docs/site/standards/update.md` is producer-facing (what each roster tool's `update` must do) and explicitly carves out the consumer side as "shll's job, lives in its own memory". Progress during the compose is consumer-side: only the orchestrator knows "tool N of M".
2. **Why not a toolkit standard (yet):** OSC 9;4 is a singleton channel — the terminal holds exactly one progress state. shll delegates with inherited stdio, so per-tool emission inside the compose would conflict (a tool's terminal `remove` clears shll's roster-level bar mid-loop). A producer standard would need a remove/re-assert coordination protocol plus a 6-repo rollout. Standardize only when a second emitter appears.
3. **rk passthrough confirmed:** run-kit sessions already have tmux `allow-passthrough` on, so DCS-tmux-wrapped sequences reach the run-kit tile's xterm.js (`@xterm/addon-progress`).

## Why

`shll update` is the toolkit's longest-running command: `brew update`, shll self-upgrade, then up to six delegated per-tool updates plus a network-fetching release digest — routinely minutes of wall-clock. Its output is a streamed wall of sub-tool text; the only progress signal today is the in-band `▸ [N/M]` header, which requires reading scrollback.

The concrete consumer is the run-kit dashboard: its update button runs `shll update` unattended in an rk-jobs tmux window (see change 3ovi's `--yes` plumbing), and rk tiles render terminals via xterm.js with `@xterm/addon-progress` — which parses exactly the OSC 9;4 (ConEmu/Windows Terminal) progress convention and shows a live progress line on the tile. Terminals that also understand the sequence natively (e.g. Windows Terminal taskbar) benefit for free; terminals that don't consume-and-ignore OSC harmlessly.

Without this, a user watching the dashboard (or a glanceable terminal tab) has no at-a-glance signal of how far along an update run is or whether it hit a failure. With it, the tile shows a moving determinate bar keyed to roster position, an error-colored state on a failed tool, and a clean removal at exit.

Doing nothing also leaves rbdd's decided design undocumented in code — the decision (direct-in-shll, not-a-standard) was already made and recorded in the backlog.

## What Changes

### New presentation helper: `src/cmd/shll/progress.go`

A small, subprocess-free sibling of `ui.go` owning all OSC 9;4 emission. Shape:

```go
// progressReporter emits OSC 9;4 (ConEmu/Windows Terminal) progress sequences.
type progressReporter struct {
    w       io.Writer // stderr in production
    enabled bool      // w is a real TTY (see gating below)
    tmux    bool      // wrap sequences for tmux passthrough
}

func newProgressReporter(w io.Writer, env func(string) string) *progressReporter
```

Methods (no-ops when `!enabled`):

| Method | Sequence (unwrapped) | Meaning |
|--------|---------------------|---------|
| `set(percent int)` | `ESC ] 9 ; 4 ; 1 ; {percent} BEL` | determinate progress, 0–100 |
| `indeterminate()` | `ESC ] 9 ; 4 ; 3 ; 0 BEL` | activity without a known fraction |
| `errorState(percent int)` | `ESC ] 9 ; 4 ; 2 ; {percent} BEL` | error-colored progress state |
| `remove()` | `ESC ] 9 ; 4 ; 0 ; 0 BEL` | clear the progress state |

Concrete bytes for `set(50)`: `"\x1b]9;4;1;50\x07"` (BEL-terminated — the ConEmu convention form; `@xterm/addon-progress` parses it).

**tmux wrapping**: when `env("TMUX") != ""`, wrap each sequence in the DCS tmux passthrough envelope — `ESC P tmux ;` + the sequence with every `ESC` doubled + `ESC \` — e.g. `"\x1bPtmux;\x1b\x1b]9;4;1;50\x07\x1b\\"`. rk sessions already run `allow-passthrough on`, so the inner sequence reaches the outer terminal / xterm.js tile.

**Gating** (`enabled`): `w` is an `*os.File` AND `term.IsTerminal(fd)` — the same structure as `defaultStdinIsTTY` in `ui.go`. Deliberately **independent of `NO_COLOR`**: per the existing `defaultStdinIsTTY` precedent comment, `NO_COLOR` governs *styling*; OSC 9;4 is terminal progress *state*, not styling, and a non-TTY stream (pipe/CI/test buffer) is already excluded by the TTY check.

**Testability**: tests construct `progressReporter{w: &buf, enabled: true, tmux: ...}` directly and assert emitted bytes (a `bytes.Buffer` is never a TTY, so `newProgressReporter` naturally disables in tests — mirroring `colorEnabled`'s determinism). The `env` seam reuses `runUpdate`'s existing injected `env func(string) string` for `TMUX` control.

### Wiring in `src/cmd/shll/update.go` (`runUpdate`)

Emission targets **stderr** (the `stderr io.Writer` parameter) — the framing headers/tail stay on stdout untouched; OSC is an invisible control channel and stderr keeps it out of piped stdout while still reaching the terminal.

Lifecycle, keyed to the existing structure of `runUpdate`:

1. **Construct + start**: after the subset/`hasBrew`/short-circuit guards and the dry-run return — i.e., only when the write-phase actually begins (alongside the existing `start := nowFunc()`) — construct the reporter and `defer remove()` so **every** exit path (brew-update failure `errSilent` return, panic, success) clears the terminal's progress state. Then emit `indeterminate()` covering the run-wide `brew update --quiet`.
2. **Determinate loop**: inside the existing `updateHeader` closure (which already increments `pos` against the precomputed `total`), emit `set((pos-1)*100/total)` — progress-completed-so-far at each tool boundary, covering both the `shll (self)` step and each roster tool.
3. **Error pulse**: at each point `anyFailed` is set inside the loop (self-upgrade failure, `upgradeTool` error or non-zero code), emit `errorState` with the current percent; the next tool's header `set(...)` resumes the normal state.
4. **Tail**: after the loop, emit `errorState(100)` when `anyFailed` else `set(100)` — so the bar reads complete (error-colored on partial failure) while the agent-skill refresh and the `What changed:` digest run; the deferred `remove()` then clears it at exit.

**No emission** on: the dry-run path, the `noToolsInstalledMsg` short-circuit, and the pre-`brew update` error returns (the reporter isn't constructed yet — nothing to clean up).

### Explicit non-goals

- **`shll install` / other commands** — same pattern would fit, but rbdd's decided scope is `shll update`; extend later if wanted.
- **The producer `update` standard** (`docs/site/standards/update.md`) — untouched. No roster tool is asked to emit anything. Revisit only when a second emitter appears (then the standard needs the remove/re-assert protocol).
- **Progress *content* changes** — headers, summary tail, digest are byte-identical; this change adds only invisible control sequences on stderr.

## Affected Memory

- `cli/update`: (modify) add the OSC 9;4 progress emission — sequence forms, stderr-TTY gating, NO_COLOR independence, tmux passthrough wrapping, lifecycle (indeterminate → determinate → error pulse → deferred remove), and the not-a-standard design decision (singleton channel; standardize on second emitter).

## Impact

- `src/cmd/shll/progress.go` *(new)* — reporter type, sequence constants, tmux wrapping (~80 lines)
- `src/cmd/shll/progress_test.go` *(new)* — byte-level sequence assertions, tmux-wrap and gating tests
- `src/cmd/shll/update.go` — reporter construction, deferred remove, 4 emission call sites (status/loop/error/tail)
- `src/cmd/shll/update_test.go` — existing golden-string tests keep passing unchanged (bytes.Buffer writers are not TTYs, so the reporter is disabled and stdout/stderr stay byte-identical); add a wired-in test driving an enabled reporter if the seam allows
- No new dependency (`golang.org/x/term` already present via `ui.go`), no new subprocess (Constitution I intact — presentation only), no new subcommand (Constitution VII intact), stateless (Constitution II intact)

## Open Questions

None — the scope fork (direct vs. standard), stream/gating, and tmux handling were all resolved in the discuss session and recorded in backlog rbdd.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Implement in `shll update` only; producer `update` standard and `shll install` are non-goals | Discussed — user decided after the singleton-channel analysis; recorded in backlog rbdd | S:90 R:70 A:95 D:95 |
| 2 | Certain | Emit to stderr, gated on stderr-is-a-TTY | User-specified in backlog rbdd ("gated on stderr-is-a-TTY") | S:90 R:90 A:90 D:90 |
| 3 | Certain | tmux passthrough wrapping (DCS `ESC Ptmux;` envelope, ESC doubled) when `$TMUX` is set | User confirmed rk sessions run `allow-passthrough on`; wrapping named in backlog rbdd | S:85 R:85 A:85 D:90 |
| 4 | Confident | Progress emission is independent of `NO_COLOR` | NO_COLOR governs styling per no-color.org; codebase precedent (`defaultStdinIsTTY` comment) already distinguishes styling from non-styling gates; trivially reversible | S:60 R:95 A:75 D:60 |
| 5 | Confident | State mapping: indeterminate through `brew update`; `set((pos-1)*100/total)` at each header; error pulse with current percent on failure; `errorState(100)`/`set(100)` after loop; deferred `remove()` on all exit paths | Backlog SHAPE text plus the natural mapping onto `runUpdate`'s existing `pos`/`total`/`anyFailed` structure | S:75 R:85 A:80 D:70 |
| 6 | Certain | New `progress.go` presentation helper with injected writer/env seams, no subprocess calls | Mirrors established `ui.go` patterns (colorEnabled/stdinIsTTY seams, named constants); Constitution I | S:65 R:90 A:90 D:80 |
| 7 | Certain | BEL-terminated sequence form `ESC ] 9 ; 4 ; {st} ; {pr} BEL` | ConEmu convention's canonical form; parsed by `@xterm/addon-progress` and Windows Terminal | S:70 R:95 A:85 D:85 |
| 8 | Certain | No emission on dry-run, no-tools short-circuit, or pre-`brew update` error paths | Nothing long-running to report there; reporter constructed only when the write-phase begins, so the deferred remove pairs with actual emission | S:70 R:90 A:85 D:85 |

8 assumptions (6 certain, 2 confident, 0 tentative, 0 unresolved).
