# Plan: Two-region install/update terminal UX

**Change**: 260820-yud0-install-update-terminal-ux
**Intake**: `intake.md`

## Requirements

### CLI: Pinned status region (tty only)

#### R1: Scroll-region lifecycle
On a tty, `shll install` and `shll update` SHALL run their per-tool write phase inside a two-region terminal layout: a pinned one-line header at the top of the screen and a DECSTBM scroll region beneath it in which child output streams naturally (no alternate screen, no output withholding).

- **GIVEN** stdout is a real terminal
- **WHEN** the install/update write phase begins
- **THEN** shll emits a DECSTBM margin sequence reserving the top line, draws the status header there, and parks the cursor inside the scroll region
- **AND** all subsequent child output scrolls within the region while the header stays pinned

#### R2: Header content — current tool, honest k/n, next tool
The pinned header SHALL show the current verb + tool, the honest `k/n` step count over the tools acted on this run, and the next tool when one exists — e.g. `Installing run-kit (2/7) · next: rk-desktop`; the final tool omits the `next:` clause. Exact styling follows `ui.go` conventions (color via the run's single `colorEnabled` decision; the `·` separator ASCII-degrades when color is off).

- **GIVEN** a tty run acting on tools `run-kit, rk-desktop, fab-kit` (n=3)
- **WHEN** the second tool's boundary is reached
- **THEN** the header reads `Installing rk-desktop (2/3) · next: fab-kit`
- **AND** at the third tool it reads `Installing fab-kit (3/3)` with no next clause

#### R3: Region restore on every exit path
The scroll region SHALL be restored (margins reset via `ESC[r`, header line released) on **every** exit path: normal return and error return via `defer`, SIGINT/SIGTERM via a signal handler that resets the region and re-raises the signal, and the region + header SHALL be re-applied on SIGWINCH using the freshly queried terminal size. shll adds **no** new child-kill logic — children receive the terminal-generated SIGINT natively; the update standard's brew-safety clause (no SIGKILL, no short timeout on brew) is untouched.

- **GIVEN** a tty run with the region active
- **WHEN** the user presses Ctrl-C (or the process receives SIGTERM)
- **THEN** the margins are reset before the process dies and the terminal is left usable
- **AND** **WHEN** the terminal is resized mid-run **THEN** the margins and header are re-applied for the new size

### CLI: Degradation

#### R4: Non-tty output unchanged; NO_COLOR as today
When stdout is not a tty (CI, pipes), the sequential `printToolHeader` output SHALL remain **exactly** today's — `shll update | cat` / `shll install | cat` output is byte-identical in structure to today (no region sequences, no pinned header, no failure-tail block). `NO_COLOR` SHALL keep governing styling exactly as today (glyph/ANSI degrade); the region itself is gated on tty-ness only (it is terminal *state*, not styling — the same line OSC 9;4 draws). The existing `==>`/`▸ [N/M]` in-stream headers keep printing in **both** modes (scrollback continuity + shared degradation path).

- **GIVEN** stdout is a `bytes.Buffer` or a pipe
- **WHEN** `shll update` / `shll install` runs
- **THEN** no DECSTBM/cursor sequences are emitted and every existing golden-string test passes unchanged

### Proc: Prompt-hang hardening

#### R5: Children run with stdin from the null device
Install/update children (brew trust/install/update/upgrade/link, delegated `<tool> update`, `rk desktop install|update`) SHALL run with stdin from the null device (`cmd.Stdin = nil` — Go's documented null-device behavior), so a child that attempts an interactive prompt reads EOF and fails fast instead of hanging the walk. This enforces the toolkit's prompt-free standard; no interactive accommodation is added (a genuinely unavoidable question reads `/dev/tty` in the tool itself). Stdin redirection lives in `internal/proc` (Constitution I) as a new transport — `RunForeground` itself is untouched, and the end-of-run agent-skill refresh subprocess (`shll setup agent`, which may legitimately confirm via run-kit when `--yes` is absent) deliberately keeps inherited stdin.

- **GIVEN** a child tool whose update tries to read a confirmation from stdin
- **WHEN** `shll update` delegates to it
- **THEN** the child reads EOF, exits non-zero, and the run records that tool as failed instead of hanging

#### R6: Failure shows the captured output tail
The new proc transport SHALL tee each child's stdout/stderr to the run's writers **live** (streaming, never buffered-until-exit) while capturing a bounded interleaved tail (last ~4KB). On a child failure in region mode, shll SHALL print that tail to stderr framed with the tool name (e.g. `--- last output: <tool> ---`), so the cause — including an attempted prompt — stays visible even though lines scrolled out of a DECSTBM region never enter scrollback. Non-tty runs print no tail block (output is already fully present in the log — R4's byte-identical guarantee).

- **GIVEN** a region-mode run where a child fails after producing output
- **WHEN** the failure is recorded
- **THEN** the last captured lines of that child's output are re-printed to stderr under a tool-named frame

### CLI: Determinate OSC 9;4 progress

#### R7: `shll install` gains determinate OSC 9;4 progress
`shll install`'s write phase SHALL construct the existing `progressReporter` (`src/cmd/shll/progress.go`, stderr, tmux-passthrough-wrapped, tty-gated) and emit determinate `pos/total` progress mirroring `shll update`'s wiring: `set((pos-1)*100/total)` at each tool boundary, an `errorState` pulse at each failure, `set(100)`/`errorState(100)` at the tail, and a deferred `remove()` covering every post-construction exit. Progress is honest `k/n` steps over the tools acted on — never a synthetic percent across tools.

- **GIVEN** a tty `shll install` run installing 4 tools
- **WHEN** the third tool's header prints
- **THEN** an OSC 9;4 `set(50)` sequence is emitted on stderr
- **AND** on run completion the state is set to 100 and then removed

#### R8: `shll update`'s existing emission is kept and verified
`shll update`'s OSC 9;4 emission (already determinate per-tool since rbdd: indeterminate over the run-wide brew refresh, `set` at boundaries, error pulses, clear-on-finish) SHALL be preserved as-is; this change verifies it against the intake contract (determinate `pos/total`, tmux passthrough, clear on finish, error state on failure) and extends nothing beyond keeping it correct alongside the region integration.

- **GIVEN** the existing `TestUpdate_Progress*` tests
- **WHEN** the region integration lands
- **THEN** those tests still pass byte-identically

### Non-Goals

- No alternate-screen TUI, no full output buffering — child output must stream live.
- No pty allocation for children (see Assumption 1) — a pipe-tee is the accepted transport.
- No new kill/timeout logic around brew or delegated children (update standard brew-safety clause).
- No change to `shll uninstall` (its own `Proceed?` prompt reads shll's stdin, not a child's) or to any read-only probe transport.
- No Windows support (Constitution: darwin/linux only — SIGWINCH is available on both).

### Design Decisions

#### Tee-with-pipe transport, not pty and not inherited fds
**Decision**: Region-mode children get piped stdout/stderr tee'd live to the terminal plus a bounded tail ring; they do not inherit the terminal fds.
**Why**: The failure tail (R6) requires capture, and DECSTBM-scrolled lines are lost from scrollback, so capture is load-bearing; a tee still streams live (the intake's "no output buffering"). Children seeing a pipe degrade their own tty-only rendering (e.g. brew progress bars) — accepted: the toolkit standard already requires children to behave non-interactively, and degraded child output inside a coherent region beats lost failure context.
**Rejected**: pty allocation (new dependency, platform surface, overkill for a framing feature); inherited fds with no tail (fails R6 — a mid-run failure's output can scroll away irrecoverably).
*Introduced by*: 260820-yud0-install-update-terminal-ux

#### Stdin hardening scoped to roster/brew children only
**Decision**: The null-stdin transport covers brew and roster-tool children in install/update; the end-of-run agent-skill refresh subprocess keeps `RunForeground` (inherited stdin).
**Why**: The refresh re-runs `shll setup agent`, whose run-kit hook delegation legitimately confirms interactively when `--yes` was not passed — null stdin would break the interactive path that exists by design.
**Rejected**: hardening every subprocess in the two commands uniformly (breaks the documented interactive refresh contract).
*Introduced by*: 260820-yud0-install-update-terminal-ux

#### Region gate is tty-only, independent of NO_COLOR
**Decision**: The scroll region enables on stdout-tty alone (a `regionWriterIsTTY` seam mirroring `progressWriterIsTTY`); NO_COLOR keeps governing only styling inside the header text.
**Why**: NO_COLOR is a styling convention (no-color.org); a scroll region is terminal state, the same line `progress.go` already draws for OSC 9;4. `NO_COLOR` behavior therefore stays "exactly as today" per the intake.
**Rejected**: disabling the region under NO_COLOR (conflates styling with layout; leaves NO_COLOR-tty users with the old opaque walk for no styling gain).
*Introduced by*: 260820-yud0-install-update-terminal-ux

## Tasks

### Phase 1: Proc transport

- [x] T001 Add the streamed-tail transport to `src/internal/proc/proc.go`: new `TransportStreamTail` + `RunStreamedTail(ctx, stdout, stderr io.Writer, name string, args ...string) (code int, tail []byte, err error)` — `cmd.Stdin = nil` (null device), child stdout → `io.MultiWriter(stdout, ring)`, child stderr → `io.MultiWriter(stderr, ring)`, one bounded interleaved ring (~4KB named constant), Foreground-style exit-code semantics (`ErrNotFound` mapping, exitCode extraction). Request gains the writer fields used only by this transport. <!-- R5 -->
- [x] T002 Add `src/internal/proc` tests for the new transport: stdin reads EOF (a `sh -c 'read x'` child fails fast), live tee (writer receives bytes), tail bounded and interleaved, exit-code/ErrNotFound mapping. <!-- R5 -->

### Phase 2: Region primitive

- [x] T003 Create `src/cmd/shll/region.go`: `statusRegion` type with named escape-sequence constants (DECSTBM set/reset, cursor save/restore/home, line clear); `newStatusRegion(w io.Writer, ...)` gated by a swappable `regionWriterIsTTY` seam (mirrors `progressWriterIsTTY`) plus a swappable `terminalSize` seam; methods `start()`, `setHeader(text string)`, `stop()`; SIGWINCH handler re-queries size and re-applies margins + header; SIGINT/SIGTERM handler resets margins, stops the handler, and re-raises; disabled instance is a total no-op (zero bytes). <!-- R1 -->
- [x] T004 [P] Add the header-text builder in `region.go` (or `ui.go` if it fits better): verb (`Installing`/`Updating`) + tool + `(k/n)` + optional `· next: <tool>`; `·` ASCII-degrades via the existing color decision. <!-- R2 -->
- [x] T005 Create `src/cmd/shll/region_test.go`: byte-exact start/header/stop sequences with an injected size, resize redraw, restore-on-stop, disabled no-op, header-builder forms (with/without next, ASCII degrade). <!-- R1 -->

### Phase 3: Command integration

- [x] T006 Wire the region into `src/cmd/shll/install.go`: construct at write-phase start (after dry-run/short-circuit returns), `defer stop()`; `installHeader` additionally updates the pinned header with next-tool lookahead across both phases; switch the trust, `brew install`, and delegated `rk desktop install` children to `proc.RunStreamedTail`; on child failure in region mode print the framed tail to stderr. <!-- R1 -->
- [x] T007 Add determinate OSC 9;4 progress to `src/cmd/shll/install.go`: `newProgressReporter(stderr, env)` + deferred `remove()`, `set((pos-1)*100/total)` at each boundary, `errorState` pulses on failure, final `set(100)`/`errorState(100)` — mirroring update.go's wiring (note: `runInstall` needs the `env` seam it already takes threaded to the reporter). <!-- R7 -->
- [x] T008 Wire the region into `src/cmd/shll/update.go`: region lifecycle around the write phase, header states for the brew metadata refresh, the `shll (self)` step, and each roster tool (with next-tool lookahead); switch `brew update/upgrade/link`, delegated `<tool> update`, and `rk desktop update` children to `proc.RunStreamedTail` with failure tails; keep the existing OSC 9;4 emission unchanged; leave `refreshPlacedAgentSkills`'s subprocess on `RunForeground` (Design Decision 2). <!-- R3 -->
- [x] T009 Verify the non-tty guarantee: run the full existing `install_test.go` / `update_test.go` suites — every golden must pass unchanged (buffers disable the region seam, so zero new bytes on the non-tty path). Fix any drift in shll's framing, never in the goldens. <!-- R4 -->

### Phase 4: Tests & conformance

- [x] T010 Add tty-mode tests in `install_test.go` / `update_test.go`: force `regionWriterIsTTY`/size seams → region sequences present and restored, pinned-header text at boundaries, failure prints the framed tail; assert via the fake `proc.Runner` that install/update children carry the new transport while the agent-refresh subprocess keeps `TransportForeground`. <!-- R6 -->
- [x] T011 [P] Add OSC 9;4 wiring tests for install (`TestInstall_Progress*`): determinate set at boundaries, error pulse on failure, final state + remove, no emission on dry-run/short-circuit/non-write paths. <!-- R7 -->
- [x] T012 Conformance pass: re-read `docs/site/standards/update.md` (prompt-free + brew-safety), `install-composition.md`, and `principles.md` against the diff — no SIGKILL/timeout added, delegation argvs unchanged, prompt-free enforced not accommodated; note the result in `## Notes`. Then run the full suite (`cd src && go test ./...`). <!-- R8 -->

## Execution Order

- T001 blocks T002, T006, T008 (transport must exist before integration).
- T003/T004 block T005/T006/T008 (region primitive before wiring).
- T006–T008 block T009–T011.
- T012 runs last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: On a (seam-forced) tty, install/update emit DECSTBM setup, a pinned header, and margin reset — asserted byte-exact in region/command tests
- [x] A-002 R2: Header shows verb + tool + honest `(k/n)` + `next:` clause (omitted on the last tool), with ASCII degrade
- [x] A-003 R5: Every install/update child (brew trust/install/update/upgrade/link, delegated tool argvs) runs via the null-stdin streamed transport; the agent-refresh subprocess does not
- [x] A-004 R6: A failing child's bounded output tail is re-printed to stderr with a tool-named frame in region mode
- [x] A-005 R7: `shll install` emits determinate OSC 9;4 (boundary set, failure pulse, final state, deferred remove), tmux-wrapped under $TMUX

### Behavioral Correctness

- [x] A-006 R4: All pre-existing install/update golden-string tests pass unchanged (non-tty output byte-identical in structure)
- [x] A-007 R8: All pre-existing `TestUpdate_Progress*` tests pass unchanged

### Scenario Coverage

- [x] A-008 R3: Region restore is exercised for normal return, error return, and (unit-level) the signal path; SIGWINCH re-applies margins with the new size
- [x] A-009 R5: A child that reads stdin fails fast (EOF) instead of hanging — covered by a proc test

### Edge Cases & Error Handling

- [x] A-010 R1: Region-disabled instance (non-tty) writes zero bytes from every method
- [x] A-011 R6: Tail ring is bounded (a chatty child cannot grow memory unbounded) and interleaves both streams
- [x] A-012 R7: No OSC emission on install's dry-run, short-circuit, brew-missing, or unknown-target paths

### Code Quality

- [x] A-013 Pattern consistency: seams mirror existing patterns (`progressWriterIsTTY`, `nowFunc`, `proc.Runner`); escape strings are named constants; no magic numbers
- [x] A-014 No unnecessary duplication: header/tail framing reuses `ui.go` helpers; the OSC wiring reuses `progress.go` verbatim
- [x] A-015 Subprocess invocation stays inside `internal/proc` — no `os/exec` in command code (Constitution I)
- [x] A-016 No regex over brew output; no hardcoded brew paths; roster untouched

### Security

- [x] A-017 R5: The new transport passes explicit argv slices to `exec.CommandContext` (no shell strings, ctx threaded)

## Notes

- T012 conformance pass (2026-08-20): re-read `docs/site/standards/update.md`, `install-composition.md`, `principles.md` against the diff. **Conformant.** Brew-safety: no SIGKILL/SIGTERM/timeout logic added — the only new signal path is the region's SIGINT/SIGTERM restore-and-re-raise (terminal default preserved; children receive the terminal-generated signal natively). Prompt-free: enforced, not accommodated — every install/update write child (brew trust/install/update/upgrade/link, delegated `<tool> update`, `rk desktop install|update`) runs via `TransportStreamTail` with null stdin (`cmd.Stdin = nil` → EOF); the only interactive path remains the pre-existing agent-refresh foreground subprocess (Design Decision 2). Delegation argvs unchanged: `upgradeArgv`, `t.Install`, `--skip-brew-update` probing all byte-identical; only the transport changed. All subprocess work stays inside `internal/proc` (Constitution I) — no `os/exec` in command code.
- Full suite after the change: `cd src && go test ./...` → all four packages ok (2026-08-20).
- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds new functionality without making existing code redundant. `proc.RunForeground` in particular is NOT a candidate: it retains live call sites (agent_setup.go:376, agent_setup.go:447, uninstall.go:409, and the update agent-skill refresh per Design Decision 2).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Tentative | Region-mode children see a pipe (tee), not a pty — brew's tty-only progress rendering degrades inside the region | R6's tail requires capture; pty rejected as a new dependency for a framing feature; children are prompt-free by standard anyway | S:55 R:60 A:70 D:45 |
| 2 | Confident | Stdin hardening excludes the end-of-run agent-skill refresh subprocess | run-kit's hook confirmation is a documented interactive path without `--yes`; nulling it would break that contract | S:70 R:85 A:85 D:80 |
| 3 | Confident | Region gate = tty-only, independent of NO_COLOR (styling still NO_COLOR-gated) | Mirrors `progress.go`'s documented styling-vs-state line; intake pins NO_COLOR "exactly as today" | S:70 R:85 A:80 D:75 |
| 4 | Confident | Update's OSC emission is already determinate (rbdd) — this change adds install parity and changes update's emission not at all | `update.go` already emits `set((pos-1)*100/total)` per boundary + error pulses + clear; intake wording predates rbdd's landing | S:75 R:90 A:90 D:85 |
| 5 | Confident | In-stream `==>`/`▸` per-tool headers keep printing in region mode too | Scroll-region content above the last screen is lost to scrollback; in-stream headers keep boundaries greppable and share the degradation path | S:65 R:90 A:80 D:75 |
| 6 | Tentative | Tail bound ≈4KB interleaved, printed to stderr, region mode only | Size is a judgment call (named constant, trivially tunable); non-tty printing would break R4's byte-identical guarantee | S:50 R:90 A:75 D:60 |
| 7 | Confident | Ctrl-C handling = restore margins + re-raise; no new child-signal logic | Terminal delivers SIGINT to the foreground process group natively; brew-safety clause forbids adding kill logic | S:70 R:80 A:85 D:80 |

7 assumptions (0 certain, 5 confident, 2 tentative).
