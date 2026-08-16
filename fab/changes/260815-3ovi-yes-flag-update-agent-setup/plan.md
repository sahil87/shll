# Plan: Add -y/--yes to shll update and shll agent-setup

**Change**: 260815-3ovi-yes-flag-update-agent-setup
**Intake**: `intake.md`

## Requirements

### CLI: `shll agent-setup --yes`

#### R1: agent-setup accepts and propagates `--yes`
`shll agent-setup` SHALL accept a `--yes` bool flag with shorthand `-y`. When set, the `run-kit agent-setup` delegation argv MUST include `--yes` — on both the install path and the `--uninstall` path (the delegation helper is shared). Without the flag, the delegated argv MUST be byte-identical to today (`run-kit agent-setup` / `run-kit agent-setup --uninstall`). shll's own skill placement is already promptless and MUST NOT change.

- **GIVEN** run-kit is on PATH with pending hook changes
- **WHEN** `shll agent-setup --yes` runs
- **THEN** the delegation invokes `run-kit agent-setup --yes` and run-kit's `Write these changes? [y/N]` prompt is skipped
- **AND** `shll agent-setup` (no flag) still delegates `run-kit agent-setup` with no `--yes`

- **GIVEN** the `--uninstall` mode
- **WHEN** `shll agent-setup --uninstall --yes` runs
- **THEN** the delegation invokes `run-kit agent-setup --uninstall --yes`

#### R2: `--yes` + `--print` is a harmless no-op
`--print` never delegates, so combining it with `--yes` SHALL NOT be a usage error — the run behaves exactly as `--print` alone (content + target paths, no writes, no delegation). This contrasts deliberately with `--print`+`--uninstall`, which stays a mutual-exclusion usage error.

- **GIVEN** any environment
- **WHEN** `shll agent-setup --print --yes` runs
- **THEN** the output equals `shll agent-setup --print` and exit code is 0

### CLI: `shll update --yes`

#### R3: update accepts `--yes` and threads it into the agent-setup refresh only
`shll update` SHALL accept the same `--yes`/`-y` flag. When set, the end-of-run agent-skill refresh subprocess (`refreshPlacedAgentSkills`) MUST invoke `shll agent-setup --yes` instead of `shll agent-setup`. This is the flag's ONLY consumption point: the per-tool delegated `<tool> update [--skip-brew-update]` argvs and the shll self-upgrade `brew upgrade` argv MUST be untouched (they are already bound prompt-free by the update standard).

- **GIVEN** a prior `shll agent-setup` placement exists
- **WHEN** `shll update --yes` completes its roster loop
- **THEN** the refresh subprocess argv is `shll agent-setup --yes`
- **AND** every per-tool upgrade argv is identical to a run without the flag

- **GIVEN** no placement exists
- **WHEN** `shll update --yes` runs
- **THEN** no refresh subprocess runs at all (the placement gate is unchanged)

#### R4: dry-run preview reflects `--yes`
When `--yes` is set, a placement exists, and `--dry-run` is passed, the preview tail line MUST render the real argv: `Then: shll agent-setup --yes (refresh placed agent skills)`. Without `--yes` the existing line is unchanged. The preview and the live path MUST derive from the same argv decision (single source of truth — an inaccurate preview is worse than none, toolkit principle №5).

- **GIVEN** a placement exists
- **WHEN** `shll update --yes --dry-run` runs
- **THEN** stdout contains `Then: shll agent-setup --yes (refresh placed agent skills)`
- **AND** `shll update --dry-run` (no flag) still prints `Then: shll agent-setup (refresh placed agent skills)`

#### R5: help text documents the flag on both commands
Both commands' cobra registration SHALL surface `--yes`/`-y` in `--help` output, and each `Long` text SHALL gain a sentence explaining the flag's purpose (unattended/agent-driven runs; update's sentence lands in its existing agent-setup-refresh paragraph). The `--skip-brew-update` literal-substring contract in update's help MUST remain intact.

- **GIVEN** the built binary
- **WHEN** `shll update --help` and `shll agent-setup --help` run
- **THEN** both outputs contain `--yes` and `-y`
- **AND** `shll update --help` still contains the literal substring `--skip-brew-update`

### Design Decisions

#### Reuse the uninstall `--yes` flag constants
**Decision**: Register the flag via the existing `yesFlag`/`yesFlagShorthand` constants from `src/cmd/shll/uninstall.go` (same `main` package); give agent-setup and update their own usage-string constants since `yesFlagUsage` describes shll's *own* prompt, which neither command has.
**Why**: code-quality.md forbids magic strings, and the constants already exist in-package; a tailored usage string tells the truth — the skipped prompt belongs to the delegated `run-kit agent-setup`.
**Rejected**: duplicating new flag-name constants (drift risk); reusing `yesFlagUsage` verbatim (misleading — implies shll itself prompts).
*Introduced by*: 260815-3ovi-yes-flag-update-agent-setup

#### Explicit flag plumbing, not TTY detection
**Decision**: Thread `--yes` explicitly through the chain `shll update --yes` → `shll agent-setup --yes` → `run-kit agent-setup --yes`.
**Why**: a pane-TTY-but-unattended session (rk-jobs tmux window) is structurally undetectable — stdin IS a TTY, so TTY heuristics cannot distinguish attended from unattended (backlog-decided mechanism).
**Rejected**: TTY detection (fails the motivating case); making update's agent-setup refresh unconditionally `--yes` (removes user consent for run-kit's hook writes on attended runs).
*Introduced by*: 260815-3ovi-yes-flag-update-agent-setup

### Non-Goals

- Threading `--yes` into per-tool delegated `<tool> update` calls — those are bound prompt-free by `docs/site/standards/update.md`; the delegated argv stays fixed.
- run-kit's consumer change (`handleShllUpdate` appending `--yes` to its job argv) — separate change in the run-kit repo.
- Any change to run-kit's own `--yes` support (already shipped).

## Tasks

### Phase 1: Core Implementation

- [x] T001 [P] `src/cmd/shll/agent_setup.go`: add `--yes`/`-y` via `yesFlag`/`yesFlagShorthand` with a new agent-setup usage-string constant; thread `yes bool` through `runAgentSetup` → `runAgentInstall`/`runAgentUninstall` → `delegateRunKitAgentSetup`, appending `--yes` to the run-kit argv when set (install and uninstall paths); add a Long-help sentence <!-- R1, R2, R5 -->
- [x] T002 [P] `src/cmd/shll/update.go`: add `--yes`/`-y` with a new update usage-string constant; thread `yes bool` through `runUpdate` → `refreshPlacedAgentSkills`, appending `--yes` to the `shll agent-setup` subprocess argv when set; render the dry-run preview line with `--yes` from the same argv decision; add a Long-help sentence <!-- R3, R4, R5 -->

### Phase 2: Tests

- [x] T003 [P] `src/cmd/shll/agent_setup_test.go`: delegation argv includes `--yes` when set (install and `--uninstall` paths), excludes it when unset; `--print --yes` behaves as `--print` alone with exit 0 <!-- R1, R2 -->
- [x] T004 [P] `src/cmd/shll/update_test.go`: refresh subprocess argv is `shll agent-setup --yes` with the flag / `shll agent-setup` without; per-tool upgrade argvs unchanged under `--yes`; dry-run preview line with and without `--yes`; help output of both commands contains `--yes` and update's still contains `--skip-brew-update` <!-- R3, R4, R5 -->

### Phase 3: Verification

- [x] T005 Run `gofmt`, `go vet`, and the package test suite (`go test ./cmd/shll/...` from `src/`); fix anything surfaced <!-- R1, R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll agent-setup --yes` delegates `run-kit agent-setup --yes` (and `--uninstall --yes` in uninstall mode); without the flag the delegated argv is unchanged
- [x] A-002 R3: `shll update --yes` makes the placement-gated refresh subprocess argv `shll agent-setup --yes`; without the flag it stays `shll agent-setup`
- [x] A-003 R5: `--yes`/`-y` appears in both commands' help output with an accurate usage string

### Behavioral Correctness

- [x] A-004 R3: per-tool delegated update argvs and the shll self-upgrade argv are byte-identical with and without `--yes`
- [x] A-005 R4: `shll update --yes --dry-run` previews `Then: shll agent-setup --yes (refresh placed agent skills)`; the no-flag preview line is unchanged

### Scenario Coverage

- [x] A-006 R1: tests assert the delegation argv contents for install, uninstall, and no-flag cases via the fake proc.Runner
- [x] A-007 R2: test covers `--print --yes` as a no-op (no delegation, exit 0)

### Edge Cases & Error Handling

- [x] A-008 R3: with no placement, `shll update --yes` runs no refresh subprocess (placement gate unchanged)
- [x] A-009 R5: `shll update --help` still contains the literal substring `--skip-brew-update` (frozen textual contract)

### Code Quality

- [x] A-010 Pattern consistency: flag wiring mirrors uninstall.go's (named constants, `BoolP`, seam-threaded bool params); no magic strings
- [x] A-011 No unnecessary duplication: reuses `yesFlag`/`yesFlagShorthand` and the `appendArg` fresh-slice helper where applicable; preview and live refresh argv share one source of truth
- [x] A-012 Subprocess discipline: all argv changes stay inside existing `internal/proc` call sites (Constitution I); no new `os/exec` usage

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds new functionality without making existing code redundant (the only removed symbol, the `updatePreviewSkillRefreshLine` constant, was replaced in-place by `updatePreviewSkillRefreshFmt` as part of the diff; nothing is left unused)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | New per-command usage-string constants (`agentSetupYesUsage`-style) rather than reusing `yesFlagUsage` | Intake assumption #7 carried forward; uninstall's wording describes shll's own prompt | S:50 R:95 A:75 D:60 |
| 2 | Confident | `--yes` appended to the run-kit argv via the existing fresh-slice append discipline (never mutating a shared slice) | Mirrors `appendArg`'s slice-aliasing guard already documented in update.go | S:55 R:90 A:85 D:80 |
| 3 | Certain | Tests drive the extracted seams (`runAgentSetup`, `runUpdate`) with the fake `proc.Runner`, matching existing test style | Test-alongside strategy per code-quality.md; both seams were extracted for exactly this | S:80 R:90 A:90 D:90 |

3 assumptions (1 certain, 2 confident, 0 tentative).