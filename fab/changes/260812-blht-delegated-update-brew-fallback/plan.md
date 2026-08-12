# Plan: Delegated Update Brew Fallback

**Change**: 260812-blht-delegated-update-brew-fallback
**Intake**: `intake.md`

## Requirements

### CLI: `shll update` delegation-failure fallback

#### R1: Single fallback attempt on delegated failure
When a delegated `<tool> update` invocation fails — non-zero exit code OR transport error — after the existing unlinked-keg relink heal has had its chance, `upgradeTool` SHALL fall back exactly once to `brew upgrade <t.Formula>` via `proc.RunForeground`. The fallback SHALL apply on the delegation path only (`len(t.Update) > 0`); a tool with no Update argv keeps its existing single `brew upgrade` (no second attempt).

- **GIVEN** an installed roster tool whose own `update` subcommand exits 1 (e.g. idea ≤ 0.1.2 SIGKILLing its brew child at 120s)
- **WHEN** `shll update` runs its per-tool upgrade
- **THEN** shll runs `brew upgrade sahil87/tap/<formula>` once as a fallback
- **AND** no second fallback attempt is made if that also fails

- **GIVEN** a roster tool with no `Update` argv
- **WHEN** its `brew upgrade` fails
- **THEN** no additional brew upgrade is attempted (the primary already was brew)

- **GIVEN** the delegated update returns `proc.ErrNotFound`
- **WHEN** the existing relink heal (`brew link` + retry) runs
- **THEN** the fallback fires only if the healed retry (or the link itself) still leaves a failure

#### R2: Fallback note line
Before running the fallback, `upgradeTool` SHALL print a note line to stdout built from a named constant (mirroring `relinkNoteFmt` — code-quality.md: no magic strings) carrying: the tool name, the delegated failure detail (exit code or error text), and the exact fallback command.

- **GIVEN** a delegated `idea update` that exited 1
- **WHEN** the fallback is about to run
- **THEN** stdout shows a line equivalent to `note: idea's own update failed (exit code 1) — falling back to 'brew upgrade sahil87/tap/idea'`

#### R3: Outcome accounting
Fallback success (exit 0, nil error) SHALL count the tool as succeeded — the caller's existing `succeeded++`, version re-query, and "What changed:" digest path run unchanged. Fallback failure SHALL be returned as the tool's outcome (code/error), feeding the existing `anyFailed` accounting and stderr reporting; the note line (R2) already documents the original delegated failure.

- **GIVEN** the fallback `brew upgrade` exits 0
- **WHEN** the roster loop records the tool's outcome
- **THEN** the tool counts as succeeded and appears in the digest if its version changed

- **GIVEN** the fallback also fails
- **WHEN** the roster loop records the outcome
- **THEN** the tool counts as failed and `shll update` exits non-zero (best-effort loop continues)

#### R4: Brew-safety and proc conformance
The fallback invocation SHALL route through `internal/proc` with the caller's context — no new deadline, no timeout, no signal sent to brew — conformant with the toolkit update standard's brew-safety clause and Constitution I (explicit argument slices, no shell strings).

- **GIVEN** the fallback runs on a machine where brew stalls for minutes
- **WHEN** brew eventually completes
- **THEN** shll has not killed or signalled it (no deadline is armed anywhere in `shll update`)

#### R5: Test coverage
Fake-`proc.Runner` tests SHALL cover: (a) delegated non-zero exit → note printed + fallback argv recorded + success counted; (b) delegated failure + fallback failure → tool failed, exit non-zero; (c) delegated success → no fallback invocation; (d) ErrNotFound → relink heal ordering precedes fallback; (e) no-Update-argv tool → exactly one `brew upgrade`.

- **GIVEN** the fake runner scripts a delegated failure then a fallback success
- **WHEN** `runUpdate` executes
- **THEN** the recorded invocations show `<tool> update …` followed by `brew upgrade <formula>`, and the summary counts the tool as succeeded

### Non-Goals

- Broken-keg reinstall escalation (mid-pour kill leaving brew believing the new version is installed) — deliberate follow-up, not this change.
- Post-fallback binary verification probe (`<tool> --version`).
- Version-pinned knowledge of idea's bug.
- `HOMEBREW_NO_GITHUB_API` injection.
- Changes to `shll install`, the dry-run preview, or `upgradeArgv` (the fallback is conditional runtime recovery, never part of the planned/previewed argv).

### Design Decisions

#### Generic fallback over version-pinned knowledge
**Decision**: Trigger the fallback on any delegated-update failure for any roster tool, with no knowledge of which tool/version is broken.
**Why**: Rescues the live idea ≤ 0.1.2 catch-22 and any future tool that ships a broken `update`; no permanent tool-specific quirk in shll.
**Rejected**: Hardcoding "idea ≤ 0.1.2 is broken" — covers one incident, requires a shll release per future incident, and rots.
*Introduced by*: 260812-blht-delegated-update-brew-fallback

#### Fallback on any failure, ordered after the relink heal
**Decision**: The trigger is any failure (non-zero exit or exec/transport error) of the final delegated outcome, evaluated after the existing ErrNotFound → `brew link` → retry heal.
**Why**: One coherent rule; also rescues corrupted-binary cases where the delegated binary cannot exec at all. The relink heal stays first because it is the more specific remedy.
**Rejected**: Non-zero-exit-only trigger — leaves the exec-error family (broken binary) unrescued for no simplicity gain.
*Introduced by*: 260812-blht-delegated-update-brew-fallback

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add `fallbackNoteFmt` named constant and the single-attempt fallback branch to `upgradeTool` in `src/cmd/shll/update.go`: after the primary delegation + relink heal, on any remaining failure (code != 0 or err != nil) on the delegation path, print the note (tool name, failure detail, fallback command) and run `proc.RunForeground(ctx, brewBinary, "upgrade", t.Formula)` once, returning its outcome <!-- R1 R2 R3 R4 -->
- [x] T002 Add fake-runner tests to `src/cmd/shll/update_test.go` covering the five R5 scenarios (fallback success, fallback failure, no-fallback-on-success, relink-heal ordering, no-argv single upgrade), following the existing recorder patterns <!-- R5 -->

### Phase 3: Integration & Edge Cases

- [x] T003 Run the scoped test suite (`go test ./cmd/shll/ -run 'Update'` from `src/`, then the full `go test ./...`) and fix anything the new branch breaks <!-- R5 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: On a delegated-update failure, exactly one fallback `brew upgrade <formula>` runs, on the delegation path only
- [x] A-002 R2: The note line prints to stdout from a named constant and carries tool name, failure detail, and the fallback command
- [x] A-003 R3: Fallback success counts the tool as succeeded (digest re-query unchanged); fallback failure feeds `anyFailed`

### Behavioral Correctness

- [x] A-004 R1: A tool with no Update argv still gets exactly one `brew upgrade` (no double attempt)
- [x] A-005 R1: The ErrNotFound relink heal runs before the fallback; the fallback fires only when the healed retry still fails

### Scenario Coverage

- [x] A-006 R5: All five fake-runner scenarios exist and pass; the full test suite is green

### Edge Cases & Error Handling

- [x] A-007 R3: When both the delegation and the fallback fail, the tool is reported failed and `shll update` exits non-zero while the roster loop continues (Constitution V)

### Code Quality

- [x] A-008 Pattern consistency: The fallback mirrors the existing relink-heal shape in `upgradeTool` (note constant + guarded retry), matching surrounding comment density and naming
- [x] A-009 No unnecessary duplication: Reuses `brewBinary`, `t.Formula`, and `proc.RunForeground`; no new argv-building duplication (`upgradeArgv` untouched)
- [x] A-010 No magic strings: The note text is a named constant (`fallbackNoteFmt`), per code-quality.md

### Security

- [x] A-011 R4: The fallback routes through `internal/proc` with an explicit argument slice and the caller's context — no shell strings, no new deadline, no signals to brew (Constitution I + update-standard brew-safety clause)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

None — this change adds new functionality (the delegation-failure fallback) without making any existing code redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Tests follow the existing fake `proc.Runner` recorder pattern in `update_test.go` | Test seam mandated by spec DD #7; existing relink-heal tests are the direct template | S:85 R:90 A:95 D:95 |
| 2 | Confident | Exact note wording chosen at apply within R2's required ingredients (tool name, failure detail, command) | Intake delegates wording to implementer; easily reworded later | S:70 R:90 A:85 D:80 |

2 assumptions (1 certain, 1 confident, 0 tentative).
