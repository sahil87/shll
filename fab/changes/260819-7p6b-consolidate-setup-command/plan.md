# Plan: Consolidate setup commands into `shll setup`

**Change**: 260819-7p6b-consolidate-setup-command
**Intake**: `intake.md`

## Requirements

### CLI: the `shll setup` command family

#### R1: Runnable `shll setup` parent
`shll setup` SHALL be a new visible top-level command that runs the shell half then the agent half (the same order as install's `runPostInstallSetup`), each via the existing internals (`runShellSetup*` / `runAgentSetup`). Its ONLY flag SHALL be `--yes`/`-y`, forwarded to the agent half's run-kit delegation. Both halves always run (the agent half runs even when the shell half failed, mirroring the halves' independence in install); the exit code is worst-wins — the maximum of the two halves' exit codes (0 success / 1 operational / 2 usage per the toolkit convention).

- **GIVEN** a machine with an rc file and both skill dirs absent
- **WHEN** `shll setup` runs interactively
- **THEN** the rc block is written, both SKILL.md files are placed, run-kit's hook prompt fires (no `--yes` inferred), and the exit code is 0

- **GIVEN** `$SHELL` resolves but the rc file does not exist
- **WHEN** `shll setup` runs
- **THEN** the shell half fails with its exit-2 diagnostic, the agent half STILL runs to completion, and the process exits 2 (worst-wins)

#### R2: `shll setup shell [shell]` subcommand
`shll setup shell` SHALL carry `shell-setup`'s full current surface — `[shell]` positional, `--print`, `--uninstall`, `--rc-file` — with byte-identical behavior, as a thin cobra face over the same `runShellSetup` seam.

- **GIVEN** an rc file already carrying the sentinel block
- **WHEN** `shll setup shell zsh` runs
- **THEN** the run is a byte-identical no-op with the "already installed" message, exit 0

#### R3: `shll setup agent` subcommand
`shll setup agent` SHALL carry `agent-setup`'s full current surface — `--print`, `--uninstall`, `--yes`/`-y` — with identical behavior, as a thin cobra face over `runAgentSetup`.

- **GIVEN** run-kit installed
- **WHEN** `shll setup agent --yes` runs
- **THEN** both SKILL.md files are placed with the per-path summary and the delegation argv is `run-kit agent setup --yes`

### CLI: hidden deprecated old spellings

#### R4: Old spellings survive hidden for one release
`shll shell-setup` (keeping its `shell-install` alias) and `shll agent-setup` SHALL remain registered top-level commands marked `Hidden: true`, with identical flags and behavior, delegating to the same internals. They SHALL NOT print any deprecation warning (no cobra `Deprecated:` field — silent delegation, per the iags precedent that warnings leak through the update refresh). Their Short/Long SHALL note the rename ("renamed to `shll setup shell`" / "renamed to `shll setup agent`").

- **GIVEN** the new binary on PATH
- **WHEN** an OLD shll binary's update self-refresh executes `shll agent-setup --yes` (the cross-release-boundary compat case from `refreshArgv`)
- **THEN** the invocation succeeds with agent-setup behavior and writes NO deprecation text to stderr

- **GIVEN** the new binary
- **WHEN** `shll shell-install zsh` runs (the legacy alias)
- **THEN** it dispatches to the same hidden command and behaves as before

#### R5: Flag-surface parity cannot drift
The new subcommands and the hidden old commands SHALL share command construction (a parameterized builder or shared factory per pair), so the flag sets cannot drift apart.

- **GIVEN** the built cobra tree
- **WHEN** the flag sets of `setup shell` vs hidden `shell-setup`, and `setup agent` vs hidden `agent-setup`, are compared
- **THEN** each pair is identical (asserted by test)

### CLI: update self-refresh argv

#### R6: `refreshArgv` flips to the new spelling
`refreshArgv(yes)` SHALL emit `[shll, setup, agent, (--yes)]`. Because it is the single source of truth shared by the live refresh subprocess and `shll update`'s dry-run preview line, both flip together. `update.go`'s Long help and `updateYesUsage` prose SHALL name the new spelling.

- **GIVEN** a placed agent skill
- **WHEN** `shll update --dry-run` previews the end-of-run refresh
- **THEN** the preview line shows `shll setup agent` (with `--yes` when passed)

### CLI: pointer-string sweep

#### R7: Human-facing strings name the new spellings
All user-visible strings naming the old commands SHALL be updated to the new spellings: `doctor.go`'s `suggestNotWired`, `suggestShellUnresolvableFmt`, `suggestCorruptBlock`, `suggestSkillStale` and doctor's Long prose; `install.go`'s Long help and the `shellSetupNudgeFmt`/`agentSetupNudgeFmt` nudge lines; `uninstall.go`'s `shellUnwireHint`; `root.go`'s `rootLong` (the two lines collapse to one `shll setup` line; the visible user-facing subcommand count becomes twelve); the error/diagnostic prefixes in `shell_setup.go`/`agent_setup.go` (`shll shell-setup:` → `shll setup shell:`, `agentSetupErrPrefix` → `shll setup agent`), shared by the hidden old spellings. Install's `--no-shell-setup`/`--no-agent-setup` flags keep their names.

- **GIVEN** an unwired rc file
- **WHEN** `shll doctor` reports
- **THEN** the suggestion reads `run 'shll setup shell' then 'exec $SHELL'` (and the stale-skill hint names `shll setup agent`)

### Docs & standards

#### R8: Standards revision — both edits
`docs/site/standards/principles.md` line 88 SHALL name `shll setup` in place of `shll shell-setup`, and `docs/site/standards/install-composition.md` SHALL be expanded with a section documenting the install→setup composition: the manager's `install` runs the setup steps in-process at the end, and the consolidated re-runnable entry point (`shll setup`) is the recovery path when a shell or agent harness is added later. After any standards edit, `scripts/sync-standards.sh` SHALL be re-run so the embedded copies match (`TestStandardsEmbedMatchesCanonical` stays green).

- **GIVEN** the standards edits
- **WHEN** `go test ./... -run TestStandardsEmbedMatchesCanonical` runs
- **THEN** it passes

#### R9: README and docs-site pages updated
`README.md` and the shll.ai pages (`docs/site/install.md`, `docs/site/workflows.md`, `docs/site/skill.md`, `docs/site/standards/skill.md` landed-design note) SHALL name the new spellings where they refer to shll's own commands (run-kit's `run-kit agent setup` mentions are NOT shll commands and stay). README section anchors (`### shll shell-setup — …`) move to the new names.

- **GIVEN** the updated docs
- **WHEN** grepping README.md and docs/site for `shll shell-setup`/`shll agent-setup`
- **THEN** remaining hits are only deliberate rename/back-compat mentions (e.g. the upgrade note), not instructions to run the old spellings

### Tests

#### R10: Compat and behavior coverage
Tests SHALL cover: the hidden old spellings still dispatch (both, plus the `shell-install` alias); `shll setup` parent behavior (both halves run, worst-wins exit, `--yes` forwarding); the new refresh argv; flag-surface parity (R5); and `help_dump_test.go`'s aliased-node test adapted (hiding `shell-setup` prunes it from the dump — pick a synthetic-tree subject or assert prune behavior). Existing string-assertion tests follow the renamed strings.

- **GIVEN** the full suite
- **WHEN** `go test ./...` runs in `src/`
- **THEN** all tests pass

### Non-Goals

- Removing the old spellings (a future change after one release cycle).
- Renaming install's `--no-shell-setup`/`--no-agent-setup` flags.
- Any behavior change to what the setup halves do — pure CLI-surface relocation.
- Changes to `scripts/install.sh` (it only execs `shll install`, which calls the internals in-process).

### Design Decisions

#### Subcommands over a flag union
**Decision**: `shll setup` / `setup shell` / `setup agent` — a parent with two subcommands, not one command with selector flags.
**Why**: The halves have disjoint flag sets (`--rc-file`/positional shell vs `--yes`); a union surface would be awkward and error-prone. Matches run-kit's noun-verb precedent (`run-kit agent setup`).
**Rejected**: `--only-shell`/`--only-agent` flags on one command (flag union); keeping two top-level commands (the discoverability problem this change exists to fix).
*Introduced by*: 260819-7p6b-consolidate-setup-command

#### Hidden top-level commands, not cobra aliases, for the old spellings
**Decision**: The old spellings stay as `Hidden: true` top-level commands sharing construction with the new subcommands.
**Why**: Cobra aliases cannot relocate a command under a new parent, and the compat contract is argv acceptance — `refreshArgv` in OLD binaries executes `shll agent-setup --yes` against the NEW binary across the release boundary.
**Rejected**: cobra `Deprecated:` messages (leak through the update refresh — the iags-precedent UX bug); immediate removal (breaks every cross-boundary `shll update`).
*Introduced by*: 260819-7p6b-consolidate-setup-command

## Tasks

### Phase 1: Setup

- [x] T001 Create `src/cmd/shll/setup.go`: `newSetupCmd()` parent (RunE runs shell half then agent half via existing internals, `--yes`/`-y` only, worst-wins exit) with `newSetupShellCmd()`/`newSetupAgentCmd()` subcommands built from shared, parameterized builders reused by the hidden old spellings <!-- R1 R2 R3 R5 -->

### Phase 2: Core Implementation

- [x] T002 Rewire `src/cmd/shll/root.go`: register `newSetupCmd()`; keep `newShellSetupCmd()`/`newAgentSetupCmd()` registered but `Hidden: true` with "renamed to `shll setup …`" Short/Long (no `Deprecated:` field, `shell-install` alias retained); collapse `rootLong`'s two lines into one `shll setup` line <!-- R4 -->
- [x] T003 Flip `refreshArgv` in `src/cmd/shll/agent_setup.go` to `[shll, setup, agent, (--yes)]` (adjust `agentSetupSub` or successor constant + comments); update `src/cmd/shll/update.go` Long help and `updateYesUsage` prose <!-- R6 -->
- [x] T004 Update diagnostics and help text in `src/cmd/shll/shell_setup.go` and `src/cmd/shll/agent_setup.go`: error prefixes to `shll setup shell:` / `shll setup agent`, Long help moves to the new commands <!-- R7 -->

### Phase 3: Integration & Edge Cases

- [x] T005 [P] Sweep `src/cmd/shll/doctor.go` (`suggestNotWired`, `suggestShellUnresolvableFmt`, `suggestCorruptBlock`, `suggestSkillStale`, Long prose), `src/cmd/shll/install.go` (Long help, `shellSetupNudgeFmt`, `agentSetupNudgeFmt`), and `src/cmd/shll/uninstall.go` (`shellUnwireHint`) to the new spellings <!-- R7 -->
- [x] T006 Tests: new `setup_test.go` (parent runs both halves, worst-wins exit, `--yes` forwarding, subcommand dispatch); hidden-old-spelling compat tests (`shell-setup`, `agent-setup`, `shell-install` alias dispatch; no deprecation output); flag-surface parity test per pair <!-- R10 R5 R4 -->
- [x] T007 Adapt existing tests to the renamed strings and argv: `update_test.go` (refresh argv + preview), `doctor_test.go`, `install_test.go`, `uninstall_test.go`, `shell_setup_test.go`, `agent_setup_test.go`; adapt `help_dump_test.go`'s `TestHelpDump_EmitsAliasesRealTree` (hidden `shell-setup` is pruned from the dump) <!-- R10 R6 -->

### Phase 4: Polish

- [x] T008 [P] Update `README.md`, `docs/site/install.md`, `docs/site/workflows.md`, `docs/site/skill.md`, and `docs/site/standards/skill.md`'s landed-design note to the new spellings (keep `run-kit agent setup` mentions) <!-- R9 -->
- [x] T009 Standards: fix `docs/site/standards/principles.md` line 88 to `shll setup`; expand `docs/site/standards/install-composition.md` with the install→setup composition section; run `scripts/sync-standards.sh`; verify `TestStandardsEmbedMatchesCanonical` <!-- R8 -->
- [x] T010 Run the full suite (`go test ./...` in `src/`) and `just build`; fix any fallout <!-- R10 -->

## Execution Order

- T001 blocks T002 (root registration needs the new factories)
- T002–T004 before T006/T007 (tests assert the new tree and strings)
- T005 and T008 are independent sweeps; T009 independent of code tasks; T010 last

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll setup` exists, runs shell then agent half with only `--yes`/`-y`, worst-wins exit code
- [x] A-002 R2: `shll setup shell [shell]` carries the full shell-setup surface with unchanged behavior
- [x] A-003 R3: `shll setup agent` carries the full agent-setup surface with unchanged behavior
- [x] A-004 R4: `shll shell-setup` (+ `shell-install` alias) and `shll agent-setup` dispatch hidden, silent, behavior-identical
- [x] A-005 R6: `refreshArgv` emits `shll setup agent [--yes]`; dry-run preview matches
- [x] A-006 R8: principles.md line 88 fixed; install-composition.md documents the install→setup composition; embeds re-synced

### Behavioral Correctness

- [x] A-007 R1: the agent half runs even when the shell half fails, and the exit code is the max of the halves
- [x] A-008 R4: `shll agent-setup --yes` (the old-binary refresh argv) succeeds against the new binary with no deprecation text on stderr

### Removal Verification

- [x] A-009 R7: `shll --help` and `rootLong` no longer list `shell-setup`/`agent-setup`; the dumped help tree (Hidden-filtered) drops both and gains the `setup` family

### Scenario Coverage

- [x] A-010 R2: idempotent re-run of `shll setup shell` is a byte-identical no-op (existing tests still pin this through the new face)
- [x] A-011 R10: flag-surface parity per pair is test-asserted

### Edge Cases & Error Handling

- [x] A-012 R1: `shll setup` with unresolvable `$SHELL` or missing rc file surfaces the shell half's exit-2 diagnostic (no quiet install-style skip) and still runs the agent half
- [x] A-013 R3: run-kit absent → agent half skips the delegation silently (Constitution V, unchanged)

### Code Quality

- [x] A-014 Pattern consistency: new code follows the `newXxxCmd()` factory + writer-seam test pattern; strings are named constants (no magic strings)
- [x] A-015 No unnecessary duplication: command construction shared between new subcommands and hidden old spellings; no logic moved out of the existing run funcs
- [x] A-016 Constitution I: no new direct `os/exec` — subprocess work stays in the existing `internal/proc` paths; `shell_setup.go` stays proc-free (`TestNoProcImports` green)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change relocates the CLI surface (new `setup` family + hidden old spellings) without making existing code redundant; the old inline Long help texts were moved into `setup.go` constants, not left dead. (The old spellings' removal is a deliberate future change after one release cycle, tracked as a Non-Goal here.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Diagnostics/error prefixes adopt the new canonical spellings (`shll setup shell:` / `shll setup agent:`), shared by the hidden old spellings | One prefix per code path; the canonical name is the one users should see and re-type | S:60 R:85 A:80 D:75 |
| 2 | Confident | Bare `shll setup` runs the agent half even when the shell half failed; exit is the max of the halves' codes | Mirrors the halves' independence in install's auto-run; worst-wins matches the toolkit exit-code convention (intake #8 confirmed non-zero-on-either-failure; the max rule is the natural refinement) | S:70 R:80 A:80 D:70 |
| 3 | Confident | Bare `shll setup`'s shell half uses standalone semantics (error on unresolvable `$SHELL`/missing rc), not install's quiet-skip gating | An explicit user command should surface problems; quiet-skip exists so setup failures can't fail an install, which doesn't apply here | S:60 R:80 A:80 D:75 |
| 4 | Confident | Shared parameterized builders (not copy-pasted factories) implement the new/old command pairs | Prevents flag-set drift (R5); low-risk refactor within one file's factory functions | S:55 R:85 A:80 D:75 |
| 5 | Confident | install-composition.md expansion is scoped to shll's own composition (install runs setup in-process; `shll setup` is the re-runnable entry point) — no new obligations on the six roster tools | User chose expansion, but imposing new producer obligations would exceed this change's intent; the section documents manager behavior | S:55 R:60 A:70 D:65 |
| 6 | Certain | `rootLong`'s visible subcommand count becomes twelve; hidden commands stay out of the count (matching the existing help-dump Hidden-filter convention) | Mechanical consequence of hiding two and adding one visible command | S:85 R:90 A:90 D:90 |
| 7 | Confident | `worstError` tie-break: on equal exit codes, prefer the error whose message `translateExit` has not printed yet (an `errExitCode` with a msg) over an already-printed `errSilent`, so the unprinted diagnostic is not shadowed | Plan settled worst-wins by code but not the tie case; losing the unprinted usage message would hide the actionable diagnostic | S:55 R:90 A:80 D:70 |
| 8 | Confident | Bare `shll setup`'s `--yes` reuses the existing `agentSetupYesUsage` string rather than a new constant | The flag's only consumption point on the parent is the same run-kit delegation; a second identical string would be duplication, not clarity | S:50 R:90 A:85 D:80 |
| 9 | Certain | Hidden old spellings' Short/Long wording: `renamed to \`shll setup shell\` (hidden; kept for one release cycle)` (and the `agent` twin), one line each | R4 requires the rename note; the exact wording is presentation | S:50 R:95 A:90 D:90 |
| 10 | Confident | README gains a short `### shll setup` parent section ahead of the two renamed half sections (rather than only renaming the two existing sections) | The consolidated entry point is the change's headline; documenting only the halves would leave the parent undiscovered in the README | S:50 R:85 A:75 D:65 |
| 11 | Certain | `shell_setup.go`'s diagnostics stay string literals (respelled to `shll setup shell:`), not hoisted into a new prefix constant; the agent side keeps its existing `agentSetupErrPrefix` constant (respelled) | Matches each file's established pattern; a wholesale constant refactor would churn 30 sites for no behavior change | S:55 R:90 A:85 D:85 |

11 assumptions total (3 certain, 8 confident, 0 tentative) — rows 1–6 pre-existing, 7–11 recorded during apply.
