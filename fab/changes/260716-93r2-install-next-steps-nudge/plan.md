# Plan: Post-install "Next steps" nudge in `shll install`

**Change**: 260716-93r2-install-next-steps-nudge
**Intake**: `intake.md`

## Requirements

<!-- Derived from intake.md What-Changes §1–4 and the graded assumptions.
     The change is additive output on `shll install`: a state-gated "Next steps"
     block printed after the outcome, reusing doctor's read-only wiring detector
     and the shared install probe. No new subprocess paths, no state. -->

### install: "Next steps" nudge block

#### R1: Nudge block emission points
`runInstall` SHALL print a "Next steps" block to **stdout** on both non-preview outcome paths, after the run's outcome has been reported:
- On the **install-loop path**, after `printSummaryTail(...)`.
- On the **short-circuit path** ("All sahil87 tools already installed."), after the `allInstalledMsg` line.

The block SHALL be omitted entirely (no `Next steps:` header) when neither the shell-setup nudge nor the run-kit nudge fires.

- **GIVEN** an install run that installs one or more tools and finishes (loop path)
- **WHEN** at least one nudge gate fires
- **THEN** the "Next steps" block is printed to stdout after the summary tail, preceded by a blank line
- **AND GIVEN** the "All sahil87 tools already installed." short-circuit path with a firing gate
- **THEN** the block is printed after the `allInstalledMsg` line

#### R2: `--dry-run` prints no nudges
`runInstall` SHALL NOT print any nudge on the `--dry-run` path — dry-run is a command preview, not an outcome. The brew-missing and unknown-target early-return error paths likewise emit no nudge (they return before reaching the outcome).

- **GIVEN** `shll install --dry-run` (any wiring / run-kit state)
- **WHEN** the preview is printed
- **THEN** no "Next steps" block and no nudge line appears in the output

#### R3: shell-setup nudge gate (rc wiring, read-only)
The shell-setup nudge line SHALL be printed only when the shll sentinel block is NOT wired in the user's rc file, gated by reusing doctor's existing read-only `resolveWiringFact(env)` detector. The nudge condition SHALL be `shellResolved && !corrupt && !wired`. It SHALL be quiet on the two edge states: an unresolvable `$SHELL` and a corrupt (open-without-close) shll block. The reuse SHALL be strictly read-only (`os.ReadFile` + parse); `shll install` writes no rc file.

- **GIVEN** an rc file with no shll block (unwired), `$SHELL` resolvable
- **WHEN** the "Next steps" block is emitted
- **THEN** the shell-setup nudge line is printed (run `shll shell-setup`, then `exec $SHELL`)
- **AND GIVEN** an rc file already carrying the shll eval block (wired)
- **THEN** the shell-setup nudge line is omitted
- **AND GIVEN** an unresolvable `$SHELL` or a corrupt shll block
- **THEN** the shell-setup nudge line is omitted (quiet on the edge states)

#### R4: run-kit agent-setup nudge gate (run-kit installed after this run)
The run-kit agent-setup line SHALL be printed only when run-kit is installed after the run completes, re-derived by a post-run probe via the shared install probe (`toolInstalled`), not by tracking what this run did (Constitution II — stateless re-derive). The line SHALL be informational and marked "optional, once per machine" — shll SHALL NOT probe run-kit-internal agent-setup state (Constitution II/III).

- **GIVEN** run-kit is installed (this run, pre-installed, migrated, or a subset run where run-kit is present)
- **WHEN** the "Next steps" block is emitted
- **THEN** the run-kit agent-setup informational line is printed
- **AND GIVEN** run-kit is not installed after the run
- **THEN** the run-kit agent-setup line is omitted

#### R5: Framing, constants, and the `env` seam
The block SHALL go to **stdout** with the existing color/TTY framing (the single `colorEnabled(stdout)` decision already computed for headers/tail; the block reuses it, and `arrow(color)` for any glyph). All nudge message strings SHALL be named constants in `install.go` (no magic strings, per code-quality.md), mirroring `allInstalledMsg` / `shllSelfInstallNote`. `runInstall` SHALL gain an `env func(string) string` parameter for the wiring probe (production passes `os.Getenv`), mirroring `runDoctor`'s established test seam. Nudges SHALL print regardless of per-tool install failures (`anyFailed`) — the block is informational and orthogonal to install outcome.

- **GIVEN** production invocation via the cobra factory
- **WHEN** `runInstall` is called
- **THEN** `env` is `os.Getenv` and the block reuses the existing `colorEnabled(stdout)` / framing helpers
- **AND** every nudge string is a named constant in `install.go`

### Non-Goals

- Any shll.ai repo change (PR #90 covers the site side) — out of scope.
- Changing `shll doctor` wording or `shell_setup.go` / `doctor.go` behavior — the wiring detector is reused as-is, read-only.
- Gating the run-kit line on "agent-setup already ran" — that is run-kit-internal state; Constitution II/III forbid probing it. Accepted trade-off: the line prints even for users who already ran agent-setup. (Stricter "only when this run installed run-kit" recorded as a fallback, not implemented.)
- `shll update` / `shll uninstall` output.

### Design Decisions

1. **Reuse `resolveWiringFact` in-place (no move).** `resolveWiringFact` lives in `doctor.go`, same `main` package — *Why*: it is already the established read-only composition of shell-setup's primitives, and doctor proves the reuse pattern; moving it would be churn with no benefit — *Rejected*: extracting it to a shared file (unnecessary; same-package call works).
2. **`env` parameter on `runInstall`.** Thread `env func(string) string` through, mirroring `runDoctor` — *Why*: the required rc-file test cases need a controllable env pointing at a `t.TempDir()` rc file, exactly as doctor's tests do — *Rejected*: reading `os.Getenv` directly inside a helper (untestable without touching the real `~/.zshrc`).
3. **Post-run `toolInstalled` re-probe for run-kit.** Re-derive run-kit presence after the run — *Why*: Constitution II (stateless) and uniform across loop / short-circuit / migration / subset paths — *Rejected*: tracking whether this run installed run-kit (stateful, misses pre-installed/subset cases).

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add named nudge constants to `src/cmd/shll/install.go` — a `nextStepsHeader` ("Next steps:"), a `shellSetupNudgeFmt`/`shellSetupNudge` line (run `shll shell-setup`, then `exec $SHELL`), and a `runKitAgentSetupNudge` line ("optional, once per machine" wording), following the `allInstalledMsg` / `shllSelfInstallNote` convention. <!-- R5 -->
- [x] T002 Add the `env func(string) string` parameter to `runInstall` in `src/cmd/shll/install.go` and thread `os.Getenv` from the cobra factory `newInstallCmd` (mirroring `runDoctor`). <!-- R5 -->
- [x] T003 Implement a `printNextSteps(stdout io.Writer, ctx, env, color)` helper (or inline block) in `src/cmd/shll/install.go` that computes the two gates — `w := resolveWiringFact(env)` with condition `w.shellResolved && !w.corrupt && !w.wired`, and `toolInstalled(ctx, runKitTool)` — prints a blank line + `nextStepsHeader` + the applicable line(s), and prints nothing when neither gate fires. Resolve the run-kit `Tool` from `Roster` (the `LegacyFormula`-bearing entry / `rosterHas("run-kit")`). <!-- R3 --> <!-- R4 -->
- [x] T004 Call the nudge block on the two outcome paths in `runInstall`: after `printSummaryTail(...)` on the install-loop path, and after the `allInstalledMsg` line on the short-circuit path. Ensure the `--dry-run`, brew-missing, and unknown-target paths return before it. <!-- R1 --> <!-- R2 -->

### Phase 3: Tests

- [x] T005 Update all existing `runInstall(...)` call sites in `src/cmd/shll/install_test.go` for the new `env` parameter. Existing golden-string tests pass a **wired** env (via a shared `installWiredEnv(t)` helper that resolves zsh + a `t.TempDir()` `.zshrc` containing the shll block) so the shell-setup nudge is suppressed; account for the run-kit agent-setup line where the fake reports run-kit installed (append it to the affected goldens, or point the run-kit probe at a not-installed state). <!-- R1 --> <!-- R5 -->
- [x] T006 Add the intake-required new test cases to `src/cmd/shll/install_test.go`: (a) shell-setup nudge **shown** when rc unwired; (b) **hidden** when wired; (c) agent-setup line **gated on run-kit presence** (shown installed / hidden absent); (d) **nothing** printed on `--dry-run`; (e) **short-circuit path** still nudges when unwired. Reuse the doctor-style `t.TempDir()` rc + env pattern; drive `runInstall` with a fake `proc.Runner` and `bytes.Buffer` writers. <!-- R1 --> <!-- R2 --> <!-- R3 --> <!-- R4 -->

## Execution Order

- T001, T002 precede T003 (constants + signature before the helper that uses them).
- T003 precedes T004 (helper before its call sites).
- T004 precedes T005/T006 (behavior in place before tests are updated/added).

## Acceptance

### Functional Completeness

- [x] A-001 R1: The "Next steps" block prints to stdout after `printSummaryTail` (loop path) and after `allInstalledMsg` (short-circuit path), and is omitted entirely when neither gate fires.
- [x] A-002 R2: No nudge output appears on the `--dry-run`, brew-missing, or unknown-target paths.
- [x] A-003 R3: The shell-setup nudge prints iff `resolveWiringFact(env)` reports `shellResolved && !corrupt && !wired`; the reuse is read-only.
- [x] A-004 R4: The run-kit agent-setup line prints iff `toolInstalled(ctx, run-kit)` is true after the run; it is marked "optional, once per machine".
- [x] A-005 R5: `runInstall` gains an `env func(string) string` parameter (production `os.Getenv`); all nudge strings are named constants in `install.go`; the block reuses the existing `colorEnabled` framing.

### Behavioral Correctness

- [x] A-006 R3: A wired rc file suppresses the shell-setup nudge; an unwired one shows it; unresolvable `$SHELL` and corrupt block are both quiet.
- [x] A-007 R4: The run-kit line is present when run-kit is installed and absent when not, across loop / short-circuit paths.

### Scenario Coverage

- [x] A-008 R1: `install_test.go` covers the shown-when-unwired, hidden-when-wired, agent-setup-gated, dry-run-silent, and short-circuit-nudges cases (intake's five required cases).

### Edge Cases & Error Handling

- [x] A-009 R2: `--dry-run` (with any wiring/run-kit state) emits no nudge — verified by test.
- [x] A-010 R3: Unresolvable `$SHELL` / corrupt block → no shell-setup nudge (quiet edge states) — covered by the gate condition.

### Code Quality

- [x] A-011 Pattern consistency: New code follows install.go's constant/helper conventions and reuses `ui.go` framing helpers and the existing `resolveWiringFact` / `toolInstalled` probes (Constitution III/IV — wrap, don't reinvent; no new subprocess or detection path).
- [x] A-012 No unnecessary duplication: The wiring detector and install probe are reused as-is; no duplicate detection logic is written.
- [x] A-013 Security (Constitution I): No new subprocess path in command code — the wiring probe is file I/O (`resolveWiringFact`), the run-kit probe already routes through `internal/proc` via `toolInstalled`.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

None — this change adds new functionality without making existing code redundant. The nudge block strictly reuses existing detectors/probes (`resolveWiringFact`, `toolInstalled`, `arrow`, `colorEnabled`); `rosterHas` (src/cmd/shll/tools.go) was refactored into a thin delegate of the new `rosterTool` but keeps three real call sites (tools.go:239, tools.go:246, changelog.go:225), and doctor's `suggestNotWired` remains doctor-owned.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Block prints after `printSummaryTail` (loop) and after `allInstalledMsg` (short-circuit); never on `--dry-run`/error paths | Intake decisions 1, 3, 5 verbatim | S:95 R:90 A:95 D:95 |
| 2 | Certain | shell-setup nudge gated via read-only `resolveWiringFact`; no nudge for wired users | Intake decision 2; detector exists, doctor proves reuse | S:95 R:85 A:95 D:95 |
| 3 | Certain | run-kit line gated on `toolInstalled` post-run; "optional, once per machine"; no run-kit-internal probe | Intake decision 4; Constitution II/III | S:90 R:80 A:95 D:90 |
| 4 | Certain | `runInstall` gains `env func(string) string` (prod `os.Getenv`), mirroring `runDoctor` test seam | Intake assumption 9; doctor sets the precedent | S:90 R:85 A:95 D:90 |
| 5 | Confident | Nudges print regardless of `anyFailed`; block reuses the existing `colorEnabled(stdout)` framing | Intake assumptions 4, 5; wiring need orthogonal to install outcome | S:60 R:90 A:80 D:75 |
| 6 | Confident | Existing golden tests updated to a wired env to suppress the shell-setup nudge; run-kit line appended to goldens where the fake reports run-kit installed | Not in intake; the deterministic way to keep goldens stable given the new state-dependent output; trivially reversible | S:55 R:80 A:80 D:70 |
| 7 | Confident | Exact nudge wording: shell-setup line mirrors doctor's `suggestNotWired` ("run 'shll shell-setup' then 'exec $SHELL'"); run-kit line mirrors the site's install-guide phrasing | Intake assumption 8 — semantic content fixed, byte-exact text finalized at apply | S:55 R:95 A:75 D:65 |
| 8 | Confident | run-kit `Tool` resolved from `Roster` (the `LegacyFormula`-bearing entry) rather than a new hardcoded descriptor | Constitution III (roster is source of truth); `toolInstalled` takes a `Tool` | S:60 R:85 A:85 D:75 |

8 assumptions (4 certain, 4 confident, 0 tentative).
