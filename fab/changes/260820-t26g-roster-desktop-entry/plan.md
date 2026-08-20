# Plan: Roster reorder + rk-desktop roster entry

**Change**: 260820-t26g-roster-desktop-entry
**Intake**: `intake.md`

## Requirements

### CLI: Roster order

#### R1: Importance-descending roster order with dependency adjacency
The `Roster` slice in `src/cmd/shll/tools.go` SHALL be ordered `run-kit, rk-desktop, fab-kit, wt, idea, tu, hop`. Every roster-driven surface (install/update/uninstall walk order, list/doctor/version row order, shell-init composition order) SHALL inherit this order from the single slice — no surface MAY hardcode a second order. The old leaves-first ordering contract (`TestRosterLeavesBeforeDependents`, the leaves-first design decision) is replaced by the new importance-descending contract.

- **GIVEN** the current roster `wt, idea, tu, run-kit, hop, fab-kit`
- **WHEN** the change lands
- **THEN** `Roster` equals `run-kit, rk-desktop, fab-kit, wt, idea, tu, hop`
- **AND** `shll list`, `shll doctor`, `shll version`, `shll install`, `shll update`, `shll uninstall`, and `shll shell-init` all reflect the new order

### CLI: Non-brew Tool seam

#### R2: Delegated (non-brew) Tool model
The `Tool` model SHALL grow a reusable non-brew seam — delegated argv fields for install/update plus a probe spec — alongside the existing `Formula`-driven default path, so a future non-brew tool reuses it rather than special-casing rk-desktop by name. A delegated tool carries NO `Formula`; brew-centric helpers that assume a formula MUST branch on the seam's presence.

- **GIVEN** the existing `Tool` struct where every entry is brew-backed
- **WHEN** a non-brew roster entry is added
- **THEN** install delegates to the entry's delegated install argv, update to its delegated update argv, and the installed-probe to its probe spec — never to `brew install/upgrade/list`
- **AND** no `if tool.Name == "rk-desktop"` special-case exists outside the roster declaration itself

#### R3: rk-desktop roster entry
The roster SHALL gain an `rk-desktop` entry using the R2 seam: install delegates to `rk desktop install`, update to `rk desktop update`, installed-probe runs `rk desktop status` and parses the `Installed:` line (`Installed: not installed` = absent; `Installed: v<X>` = present). It carries a `Description` and `SkillHint` like every entry; `ShellInit` is empty (it ships no shell integration); `Repo` is `run-kit` (it ships with the run-kit repo, no repo of its own).

- **GIVEN** a machine with `rk` installed on darwin
- **WHEN** `shll install` runs and rk-desktop is absent
- **THEN** it invokes `rk desktop install` for the rk-desktop entry
- **AND** `rk desktop status` afterwards reports `Installed: v…`

#### R4: Probe-based platform/prerequisite gating — never a hardcoded platform check
rk-desktop is actionable only when the `rk` binary is present AND `rk desktop` does not refuse the platform. Detection of an unsupported-platform refusal SHALL match the existing run-kit `errDesktopMacOnly` message (`rk desktop is macOS-only …`, from run-kit `cmd/rk/desktop.go`). shll MUST NOT contain a `runtime.GOOS`/darwin check for rk-desktop. An unsupported-platform refusal SHALL be a skip-with-note (exit unaffected) in whole-roster runs, and an explicit message on a targeted `shll install rk-desktop` run — distinguished from a real failure in both cases.

- **GIVEN** a Linux machine with `rk` installed (rk desktop refuses non-darwin)
- **WHEN** `shll install` runs whole-roster
- **THEN** rk-desktop is skipped with a note and the run still exits per the other tools' outcomes
- **AND WHEN** `shll install rk-desktop` is run explicitly
- **THEN** the refusal message is printed explicitly (not conflated with an install failure)

#### R5: run-kit-failure cascade skip
In a whole-roster `shll install` run, rk-desktop SHALL be processed immediately after run-kit (guaranteed by R1 ordering); if run-kit's install failed in that run, rk-desktop SHALL be skipped with a note (its prerequisite is absent).

- **GIVEN** a machine with no `rk` binary
- **WHEN** `shll install` runs whole-roster and run-kit's install fails
- **THEN** rk-desktop is skipped with a note naming the run-kit failure as the cause

#### R6: No formula edge, no depends_on
rk-desktop's dependency on run-kit SHALL be expressed as the runtime probe + roster adjacency only (install-composition standard, Policy A). No new formula and no `depends_on` is introduced.

- **GIVEN** the roster after this change
- **WHEN** install/update/uninstall process rk-desktop
- **THEN** no brew formula is referenced for it and run-kit's formula gains no `depends_on`

### CLI: Surface sweep

#### R7: install delegated path (`src/cmd/shll/install.go`)
`shll install` SHALL handle a delegated (formula-less) entry: the installed-probe uses the entry's probe spec instead of `isInstalled(formula)`; the trust step is skipped (no formula to trust); the write delegates to the entry's install argv foregrounded. R4/R5 gating applies. Long help SHALL enumerate the roster in the new order and describe the rk-desktop delegation.

- **GIVEN** the new roster
- **WHEN** `shll install` encounters the rk-desktop entry missing per its probe
- **THEN** it runs `rk desktop install` (no `brew trust`, no `brew install`) with the R4/R5 gates applied

#### R8: update delegated path (`src/cmd/shll/update.go`)
`shll update` SHALL handle a delegated entry: the install probe uses the probe spec (not `brew list`); the upgrade delegates to the entry's update argv (`rk desktop update`) with NO unlinked-keg relink heal and NO brew-upgrade fallback (there is no formula). `--skip-brew-update` probing SHALL NOT apply to a delegated entry. Long help SHALL be updated.

- **GIVEN** rk-desktop installed per `rk desktop status`
- **WHEN** `shll update` runs
- **THEN** it foregrounds `rk desktop update` and never touches brew for that entry
- **AND** a refusal/failure of `rk desktop update` is surfaced without a brew fallback

#### R9: version/list/doctor probes for the delegated entry
`shll version` SHALL report rk-desktop's installed version from `rk desktop status`'s `Installed:` line (or `not installed`), never via `<name> --version` (rk-desktop ships no version subcommand). `shll list` SHALL show the rk-desktop row via the same probe. `shll doctor` SHALL check rk-desktop via the probe with rk-desktop-appropriate suggestions (install hint `rk desktop install`, no formula-trust check, no wiring check), excluding it from the trust sub-check.

- **GIVEN** `rk desktop status` reports `Installed: v1.2.3`
- **WHEN** `shll version` runs
- **THEN** the rk-desktop row reads `v1.2.3`
- **AND** `shll doctor` shows rk-desktop OK with no trust/wiring check

#### R10: uninstall exclusion for non-brew entries
`shll uninstall` SHALL exclude rk-desktop from actionable brew-removal targets (there is no keg). rk-desktop SHALL remain a valid named target; naming it SHALL print a skip-with-note line (`rk uninstall`-style delegation is out of scope) and SHALL NOT affect the exit code. Uninstall Long help SHALL reflect the new order and the exclusion.

- **GIVEN** rk-desktop installed
- **WHEN** `shll uninstall rk-desktop` (or the no-args sweep) runs
- **THEN** rk-desktop is noted as not brew-managed / skipped with a note, never `brew uninstall`ed, exit unaffected

#### R11: agent-setup SkillHint composition
`agentSkillDescription()` in `src/cmd/shll/agent_setup.go` SHALL pick up rk-desktop's `SkillHint` automatically from the roster (no special-casing) — the existing per-entry loop already does this; this requirement pins that the generated description stays a single line and includes the rk-desktop clause.

- **GIVEN** the new roster entry carries `SkillHint`
- **WHEN** `shll setup agent --print` runs
- **THEN** the generated description contains the rk-desktop clause in roster position and remains one line

#### R12: Test sweep
`TestRosterLeavesBeforeDependents` SHALL be replaced by an exact-order assertion of the new roster; every test asserting roster length (6 → 7), row counts (`len(Roster)+1` goldens survive), Long-help roster enumerations, or per-tool golden output SHALL be updated to the new order and the rk-desktop entry.

- **GIVEN** the new roster and seam
- **WHEN** `go test ./...` runs
- **THEN** all tests pass with the new order and entry covered

### Docs: Site sweep

#### R13: docs/site roster enumerations updated
`docs/site/install.md`, `docs/site/workflows.md`, and `docs/site/skill.md` roster enumerations SHALL be updated to the new order including rk-desktop (description of the delegated install path where the page describes install). Pages SHALL NOT hardcode the old leaves-first order.

- **GIVEN** the current docs enumerate `wt, idea, tu, rk, hop, fab-kit`
- **WHEN** the change lands
- **THEN** the enumerations read `run-kit, rk-desktop, fab-kit, wt, idea, tu, hop` and mention the `rk desktop` delegation where install is described

### Standards: Conformance

#### R14: Standards conformance pass
The change SHALL be checked against `docs/site/standards/` — principles, install-composition, update, help-dump, skill, readme-extraction — and any violation introduced by this change fixed in-place. Findings SHALL be recorded in `conformance-report.md` in the change folder (the conformance-report convention).

- **GIVEN** the implemented change
- **WHEN** each governing standard file is read and checked
- **THEN** no new violation ships; the report records per-standard PASS/fixed/deferred

### Non-Goals

- run-kit companion change (freezing `errDesktopMacOnly` with a test upstream, or a stable token/exit code) — documented fallback only if message matching proves unstable; not queued here.
- `rk desktop uninstall` delegation from `shll uninstall` — decided at plan: exclusion (R10), not delegation.
- Changing shell-init behavior — rk-desktop has no shell-init; the composed blob's content is unchanged beyond ordering.
- README.md changes — the README is slimmed to bootstrap+pointer and does not enumerate the roster.

### Design Decisions

#### Delegated argv fields on the Tool struct, not a separate type
**Decision**: rk-desktop's non-brew behavior lives as optional fields on the existing `Tool` struct (e.g. `Install []string` delegated argv, plus a probe spec), with `Formula == ""` marking the non-brew path.
**Why**: one roster type, one iteration path — every surface already walks `[]Tool`; a parallel type would force every consumer to branch at the walk level instead of once at the brew-helper level.
**Rejected**: a separate `DelegatedTool` type / second slice (every consumer branches twice); name-keyed special cases (violates R2's reuse mandate).
*Introduced by*: 260820-t26g-roster-desktop-entry

#### Platform-gate detection by message matching
**Decision**: an unsupported-platform refusal from `rk desktop …` is detected by matching the `errDesktopMacOnly` message substring (`rk desktop is macOS-only`) in the command's output/error.
**Why**: the message already exists, is the documented refusal, and needs zero run-kit changes; matching it is testable with a fake runner.
**Rejected**: a dedicated exit code or stable stderr token (requires a run-kit companion release before shll can land — the documented fallback if matching proves unstable); a hardcoded `runtime.GOOS == "darwin"` gate (explicitly forbidden by the intake — when run-kit grows Linux support shll must need zero changes).
*Introduced by*: 260820-t26g-roster-desktop-entry

#### Uninstall excludes non-brew entries
**Decision**: `shll uninstall` skips rk-desktop with a note; no `rk desktop uninstall` delegation.
**Why**: uninstall is the brew clean-slate repair path (its contract is brew-keg removal); rk-desktop has no keg, and whether `rk desktop uninstall` even exists is unverified — delegating would invent a contract.
**Rejected**: delegating to `rk desktop uninstall` (unverified subcommand; scope creep beyond the intake, which left delegation-or-exclusion to the plan).
*Introduced by*: 260820-t26g-roster-desktop-entry

## Tasks

### Phase 1: Setup

- [x] T001 Reorder `Roster` in `src/cmd/shll/tools.go` to `run-kit, rk-desktop, fab-kit, wt, idea, tu, hop` and add the delegated-argv/probe seam fields to `Tool` (delegated install argv, probe spec) plus the `rk-desktop` entry (no Formula; Description; SkillHint; Repo `run-kit`; `Update` = `{"rk","desktop","update"}`; install = `rk desktop install`; probe = parse `Installed:` from `rk desktop status`). Add the macOS-only refusal detection helper (substring match on the `errDesktopMacOnly` message). <!-- R1 R2 R3 R4 -->
- [x] T002 Update `src/cmd/shll/tools_test.go`: replace `TestRosterLeavesBeforeDependents` with an exact-order assertion; update `TestShllSelf_NotInRoster` (len 7); add seam-field tests (rk-desktop has no Formula, has delegated argv/probe; brew tools unchanged). <!-- R1 R2 R3 R12 -->

### Phase 2: Core Implementation

- [x] T003 `src/cmd/shll/install.go` + `install_test.go`: delegated-entry install path — probe spec instead of `isInstalled`, no trust step, foreground delegated argv; whole-roster skip-with-note on unsupported platform (exit unaffected); explicit message on targeted `shll install rk-desktop`; R5 cascade skip when run-kit failed this run; update Long help. <!-- R4 R5 R7 -->
- [x] T004 `src/cmd/shll/update.go` + `update_test.go`: delegated-entry update path — probe spec in `probeTool`, no `--skip-brew-update` probe, `upgradeTool`/`upgradeArgv` delegate to `rk desktop update` with no relink heal and no brew fallback; version-bump re-query tolerates the delegated probe; update Long help. <!-- R8 -->
- [x] T005 [P] `src/cmd/shll/version.go` + `version_test.go`: `probeToolVersion` honors the probe spec (run `rk desktop status`, parse `Installed:` line → version or not-installed) so `toolVersion`/`toolInstalled` work unchanged for the delegated entry. <!-- R9 -->
- [x] T006 [P] `src/cmd/shll/list.go` (verify no code change needed beyond probe reuse) + `list_test.go`: rk-desktop row order/status coverage in table and `--json`. <!-- R9 R12 -->
- [x] T007 `src/cmd/shll/doctor.go` + `doctor_test.go`: rk-desktop evaluation — probe via the delegated probe, rk-desktop-appropriate suggestions (`rk desktop install`), exclusion from trust and wiring checks; Long help refresh if it enumerates tools. <!-- R9 -->
- [x] T008 `src/cmd/shll/uninstall.go` + `uninstall_test.go`: exclude formula-less entries from brew removal with a skip-with-note; keep `rk-desktop` a valid named target (repair-path exit-0 semantics); update Long help. <!-- R10 -->

### Phase 3: Integration & Edge Cases

- [x] T009 Sweep remaining test surfaces for hardcoded roster order/count: `shell_init_test.go`, `setup_test.go`, `help_dump_test.go`, `standards_test.go`, `skill_test.go`, `changelog_test.go`, `check_updates_test.go`, `main_test.go`, `brew_test.go`, `ui_test.go`, `agent_setup_test.go` — update any assertion that hardcodes the old order, tool list, or `len(Roster)`-derived goldens. <!-- R12 -->
- [x] T010 Verify agent-setup description composition picks up rk-desktop's SkillHint (`agent_setup_test.go` — single-line assertion + clause presence). <!-- R11 -->
- [x] T011 Update docs: `docs/site/install.md`, `docs/site/workflows.md`, `docs/site/skill.md` roster enumerations and install-path descriptions to the new order + rk-desktop delegation. <!-- R13 -->

### Phase 4: Polish

- [x] T012 Standards conformance pass against `docs/site/standards/` (principles, install-composition, update, help-dump, skill, readme-extraction); fix any violation introduced; write `fab/changes/260820-t26g-roster-desktop-entry/conformance-report.md`. <!-- R14 -->

## Execution Order

- T001 blocks everything (the seam + roster are the shared input).
- T002 follows T001 (asserts the seam it just created).
- T003–T008 are per-surface and ordered install → update → version → list → doctor → uninstall (install/update touch the same fake-runner seams; version precedes list/doctor, which reuse its probe).
- T005 blocks T006/T007 (list and doctor consume `toolInstalled`/`probeToolVersion`).
- T009–T011 follow the surface tasks; T012 is last (conformance checks the finished change).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `Roster` in `src/cmd/shll/tools.go` is exactly `run-kit, rk-desktop, fab-kit, wt, idea, tu, hop` and every roster-driven surface derives its order from that slice.
- [x] A-002 R2: The non-brew seam is field-driven on `Tool` (no name-keyed special-casing outside the roster declaration); a hypothetical second delegated tool would need only a roster entry.
- [x] A-003 R3: The `rk-desktop` roster entry installs via `rk desktop install`, updates via `rk desktop update`, and probes via the `Installed:` line of `rk desktop status`.
- [x] A-004 R4: No `runtime.GOOS`/darwin check exists for rk-desktop; gating is the rk-presence probe + the `errDesktopMacOnly` message match.
- [x] A-005 R5: A whole-roster `shll install` where run-kit's install failed skips rk-desktop with a note.
- [x] A-006 R6: No formula and no `depends_on` is introduced for rk-desktop.
- [x] A-007 R7: `shll install` handles the delegated entry end-to-end (probe, no trust, delegated write, both refusal modes) with tests.
- [x] A-008 R8: `shll update` delegates rk-desktop to `rk desktop update` with no relink heal and no brew fallback, with tests.
- [x] A-009 R9: `shll version`, `shll list`, `shll doctor` all show rk-desktop correctly (installed version from the `Installed:` line; doctor skips trust/wiring for it), with tests.
- [x] A-010 R10: `shll uninstall` never brew-uninstalls rk-desktop; naming it prints a skip note and exits 0.
- [x] A-011 R11: The generated agent-skill description includes rk-desktop's SkillHint clause and stays single-line.

### Behavioral Correctness

- [x] A-012 R1: `shll list`/`version`/`doctor`/`install`/`update`/`uninstall` output order changed from leaves-first to the new order (visible in golden/test assertions).

### Scenario Coverage

- [x] A-013 R4: Test: whole-roster install on an unsupported platform prints the rk-desktop skip note and exits 0 (given other tools succeed).
- [x] A-014 R4: Test: targeted `shll install rk-desktop` on an unsupported platform prints the explicit refusal message, distinct from a failure.
- [x] A-015 R5: Test: whole-roster install with a failing run-kit skips rk-desktop with the cascade note.

### Edge Cases & Error Handling

- [x] A-016 R4: rk absent from PATH → rk-desktop skipped (prerequisite missing), no crash, exit unaffected in whole-roster runs.
- [x] A-017 R9: `rk desktop status` with `Installed: not installed` → version/list/doctor report not-installed (`not installed` label / missing marker / FAIL-or-skip per surface contract).
- [x] A-018 R8: `rk desktop update` failing surfaces as the tool's failure (no brew fallback attempted, no relink note).

### Code Quality

- [x] A-019: All subprocess calls route through `internal/proc` (Constitution I) — no raw `os/exec` in new code.
- [x] A-020: Sub-tool integration goes through the sub-tool's own CLI (Constitution III) — rk-desktop install/update/probe are `rk desktop …` invocations, never reimplemented logic.
- [x] A-021: Graceful degradation everywhere (Constitution V) — missing rk, unsupported platform, absent rk-desktop all skip/note, never crash.
- [x] A-022: No magic strings — the macOS-only match token, `rk` binary name, delegated argvs, and note wordings are named constants.
- [x] A-023 Pattern consistency: New code follows naming and structural patterns of surrounding code (runXxx seams, fake-runner tests, preview/argv single-source builders).
- [x] A-024 No unnecessary duplication: the probe-spec parse lives in one place consumed by install/update/version/list/doctor.

### Security

- [x] A-025 R2: All new subprocess invocations use explicit argument slices via `proc.Run`/`proc.RunForeground` — no shell strings (Constitution I).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds new functionality without leaving existing code redundant. The one contract it superseded (the leaves-first roster ordering: `TestRosterLeavesBeforeDependents`, the `rosterEdge` type, and the leaves-first rationale comments in `tools.go`/`uninstall.go`) was removed in-place by the change itself; `installedVersion` retains its shll-self/changelog call sites.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | rk-desktop `Update` argv is `{"rk","desktop","update"}` and its install argv `{"rk","desktop","install"}` — carried on the existing `Update`/new delegated fields | Intake specifies both delegations verbatim; `rk desktop update --help` exists in run-kit; trivially editable in one roster line | S:85 R:90 A:80 D:75 |
| 2 | Confident | `--skip-brew-update` probing does not apply to delegated entries (no brew update is hoisted for them) | The flag exists to skip a tool's internal brew refresh; `rk desktop update` downloads a DMG, not brew — probing would be noise | S:75 R:85 A:75 D:70 |
| 3 | Confident | rk-desktop's `Repo` is `run-kit` (its list/docs link points at the run-kit repo) | rk-desktop ships with run-kit and has no own repo; the list memory's dead-link footgun says store Repo explicitly | S:70 R:90 A:70 D:70 |
| 4 | Confident | rk-desktop `SkillHint` is "desktop viewer shell" | Matches the intake's naming ("desktop viewer shell"); consistent with existing hint style (task-domain phrase) | S:65 R:95 A:70 D:70 |
| 5 | Confident | `shll uninstall rk-desktop` prints a skip note and exits 0 (no `rk desktop uninstall` delegation) | Intake left delegation-vs-exclusion to the plan; exclusion matches uninstall's brew-keg repair contract and invents no unverified subcommand | S:75 R:85 A:75 D:70 |
| 6 | Confident | Version display for rk-desktop parses `Installed: v<X>` into the existing `normalizeVersion` pipeline (status output's other lines ignored) | The probe already distinguishes `not installed`; normalizeVersion is the shared shape-enforcer | S:75 R:85 A:70 D:70 |
| 7 | Certain | The old leaves-first test/design decision is replaced (not extended) by the new order — runtime/brew dependency edges are no longer the ordering principle | Intake marks the new order Confident-resolved and says the reorder "gives the order meaning"; keeping both contracts would be contradictory | S:85 R:80 A:85 D:85 |

7 assumptions (1 certain, 6 confident, 0 tentative).
