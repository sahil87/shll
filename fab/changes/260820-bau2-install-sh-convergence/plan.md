# Plan: install.sh convergence + phase polish

**Change**: 260820-bau2-install-sh-convergence
**Intake**: `intake.md`

## Requirements

### Bootstrap: install-then-update convergence

#### R1: The bootstrap hand-off converges the machine to complete and current
The final action of `scripts/install.sh` SHALL change from `exec shll install "$@"` to install-then-update: `shll install "$@"` followed by `exec shll update <tool-names-only>`. **All args pass verbatim to `shll install`** (its full flag surface — `--no-trust`/`--no-shell-setup`/`--no-agent-setup` — is public bootstrap surface). **Only positional tool names pass to `shll update`**: the script SHALL filter out every dash-prefixed arg before the update exec, by generic `-*` pattern only — no flag-name knowledge enters the script. <!-- rework: cycle 1 — verbatim two-verb passthrough broke `sh -s -- --no-agent-setup` (shll update has no such flag; cobra unknown-flag error after a successful install) --> A failed `shll install` MUST stop the bootstrap — under `set -e` the script exits with install's status and the update pass never runs. `install` and `update` semantics stay untouched — convergence lives entirely in the script.

- **GIVEN** a machine with some tools missing and some installed-but-stale
- **WHEN** `curl -fsSL https://shll.ai/install | sh` runs to completion
- **THEN** `shll install` fills the gaps, then `exec shll update` upgrades the already-installed tools (freshly installed ones are cheap no-op updates)

- **GIVEN** the script is invoked as `sh -s -- hop`
- **WHEN** it reaches the hand-off
- **THEN** it runs `shll install hop`, then `exec shll update hop` — the tool subset reaches both verbs; each verb validates the names itself

- **GIVEN** the script is invoked as `sh -s -- --no-agent-setup`
- **WHEN** it reaches the hand-off
- **THEN** the flag reaches `shll install` untouched and is filtered from the update argv — `exec shll update` runs bare, with no unknown-flag failure

- **GIVEN** `shll install` exits non-zero (any per-tool failure)
- **WHEN** the hand-off sequence evaluates
- **THEN** the update pass is skipped and the script exits non-zero with install's status — fail-visible, no silent partial convergence

#### R2: Phase lines announce the script's three phases, gated on tty and NO_COLOR
The script SHALL print a `→ {phase}` announcement at the start and a `✓ {phase}` completion line for each of its three phases — **preflight**, **brew bootstrap**, **shll handoff** — colored only when stdout is a terminal (`test -t 1`) AND `NO_COLOR` is unset/empty. When not colored, the plain glyph lines still print (piped output MUST carry zero escape sequences). The script stays dumb: no scroll regions, no percentages. The handoff phase prints only its `→` line — `exec` replaces the process, so shll's own output is the rest.

- **GIVEN** an interactive terminal run with `NO_COLOR` unset
- **WHEN** the script runs
- **THEN** each phase prints a colored `→`/`✓` line as it starts/completes

- **GIVEN** output is piped (`| tee log`) or `NO_COLOR=1` is set
- **WHEN** the script runs
- **THEN** the phase lines print without any ANSI escape sequence

#### R3: OSC 9;4 indeterminate progress brackets the brew bootstrap
The script SHALL emit an OSC 9;4 indeterminate-progress sequence (single `printf`, state 3) immediately before the Homebrew bootstrap and clear it (state 0) immediately after, under the same tty gate as the phase lines (`test -t 1`). Harmless on non-supporting terminals. No tmux-passthrough wrapping — the script stays dumb; that sophistication lives in shll's Go side.

- **GIVEN** a tty run on a machine without Homebrew
- **WHEN** the headless brew bootstrap runs
- **THEN** OSC `9;4;3` is emitted before the installer and OSC `9;4;0` after, so supporting terminals show an indeterminate progress hint for the slowest step

- **GIVEN** piped (non-tty) output
- **WHEN** the script runs
- **THEN** no OSC sequence is emitted

#### R4: The update side effect is documented in the script header and on the install page
The script's header comment SHALL describe the new convergence contract (install-then-update, and that updating installed tools runs their `update` contracts — e.g. run-kit's daemon restart). `docs/site/install.md` (the shll.ai install page source in this repo) SHALL document the same side effect where it describes the bootstrap hand-off.

- **GIVEN** a reader of `scripts/install.sh` or the shll.ai install page
- **WHEN** they read the bootstrap description
- **THEN** both state that the bootstrap installs missing tools **and updates installed ones**, naming the update-contract side effect

### Non-Goals

- No change to `shll install` or `shll update` semantics — the verbs stay distinct (intake decision, do not re-litigate).
- No roster/subset knowledge in the script — layering contract unchanged.
- No progress percentages, scroll regions, or tmux passthrough in the script.

### Design Decisions

#### Phase lines and OSC go to stdout under a single tty gate
**Decision**: Phase lines and the OSC 9;4 sequences print to stdout, gated by one helper decision (`test -t 1` AND `NO_COLOR` unset for color; `test -t 1` alone for OSC emission).
**Why**: The script's existing informational lines already go to stdout; the intake specifies `test -t 1` as the gate. One gate decision keeps the script dumb.
**Rejected**: stderr emission mirroring `shll update`'s Go-side OSC — the script has no stderr-progress convention, and splitting streams adds logic for no user benefit.
*Introduced by*: 260820-bau2-install-sh-convergence

## Tasks

### Phase 2: Core Implementation

- [x] T001 Change the hand-off in `scripts/install.sh` to install-then-update: `shll install "$@"` (all args verbatim), then filter dash-prefixed args out of the positional params (generic `-*` match, POSIX sh), then `exec shll update` with the tool names only; update the header comment to describe the convergence contract, its update side effect, and the flags-are-install-only rule <!-- R1, R4 --> <!-- rework: cycle 1 — flag passthrough regression -->
- [x] T006 Fix `docs/site/install.md` hand-off description: tool names ride both verbs, flags reach `shll install` only (correct the "Args pass through to both verbs" wording added in the first pass) <!-- R4 --> <!-- rework: cycle 1 — docs contradicted the corrected behavior -->
- [x] T002 Add phase-line helpers (colored `→`/`✓`, gated on `test -t 1` and `NO_COLOR`) to `scripts/install.sh` and announce the three phases: preflight, brew bootstrap, shll handoff <!-- R2 -->
- [x] T003 Emit OSC 9;4 indeterminate (state 3) before the Homebrew bootstrap and clear (state 0) after it in `scripts/install.sh`, tty-gated <!-- R3 -->

### Phase 3: Integration & Edge Cases

- [x] T004 Verify the script with `sh -n scripts/install.sh` and `shellcheck` (when available); confirm piped output carries no escape sequences <!-- R2 -->

### Phase 4: Polish

- [x] T005 Update `docs/site/install.md` to document that the bootstrap now ends install-then-update (already-installed tools are upgraded, running their update contracts — e.g. run-kit's daemon restart) <!-- R4 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The script's hand-off is `shll install "$@"` then `exec shll update <tool-names-only>` — all args reach install; dash-prefixed args are filtered from the update argv by generic `-*` match
- [x] A-002 R2: All three phases print `→` start and (where reachable) `✓` completion lines
- [x] A-003 R3: OSC 9;4 state-3/state-0 brackets the brew bootstrap, tty-gated
- [x] A-004 R4: Script header comment and `docs/site/install.md` both document the update side effect

### Behavioral Correctness

- [x] A-005 R1: A failing `shll install` exits the script non-zero without running `shll update`
- [x] A-006 R2: Non-tty or `NO_COLOR` output contains zero ANSI/OSC escape sequences *(piped/non-tty verified escape-free; NO_COLOR on a tty strips color but still emits OSC 9;4 — intentional per plan.md Design Decision: `test -t 1` alone gates OSC)*

### Scenario Coverage

- [x] A-007 R1: Subset invocation (`sh -s -- hop`) passes the tool subset to both verbs; flag invocation (`sh -s -- --no-agent-setup`) reaches install only and produces no unknown-flag failure at the update exec
- [x] A-012 R4: `docs/site/install.md` states the corrected passthrough rule — tool names to both verbs, flags to install only

### Edge Cases & Error Handling

- [x] A-008 R3: OSC emission is skipped entirely when brew is already present (the bootstrap block doesn't run)

### Code Quality

- [x] A-009 Pattern consistency: New shell code is POSIX sh, stays inside the `main()`/function wrapper structure, and passes `sh -n` (and shellcheck when available)
- [x] A-010 No roster knowledge enters the script — all per-tool logic stays behind `shll install`/`shll update` (Constitution III layering guard)
- [x] A-011 No unnecessary duplication: one gating helper decision, not per-call-site tty probes

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Phase lines + OSC print to stdout under a single `test -t 1` gate | Script's informational lines already go to stdout; intake names the gate explicitly | S:80 R:90 A:85 D:80 |
| 2 | Confident | Phase shape is `→ {phase}` at start, `✓ {phase}` at completion; handoff phase prints `→` only (exec replaces the process) | Intake specifies `✓/→` glyphs and three phases; exec semantics make a handoff `✓` impossible | S:75 R:90 A:85 D:75 |
| 3 | Confident | OSC 9;4 uses state 3 (indeterminate) before the bootstrap and state 0 (clear) after, no tmux passthrough | Intake says "a single printf" indeterminate; passthrough sophistication belongs to the Go side | S:75 R:95 A:80 D:75 |
| 4 | Certain | Docs-site edit lands in this repo's `docs/site/install.md` | The install page is sourced here (constitution's docs/site → shll.ai rendering); intake's conditional resolves true | S:85 R:90 A:95 D:90 |

4 assumptions (1 certain, 3 confident, 0 tentative).
