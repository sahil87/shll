# Plan: Retire the rk→run-kit Brew-Formula Migration Guard

**Change**: 260720-h3f6-retire-rk-migration-guard
**Intake**: `intake.md`

## Requirements

### Roster: remove the transitional `LegacyFormula` field

#### R1: `Tool.LegacyFormula` is removed; `LegacyName` is retained with an accurate doc comment
The `Tool` struct SHALL NOT carry a `LegacyFormula` field, and run-kit's roster entry SHALL NOT set one. `LegacyName` (and every surface built on it — `legacyAliases`, `resolveTargets` alias resolution, `printAliasNotices`, the version-probe PATH fallback, the agent-setup vocabulary token) SHALL be retained. `LegacyName`'s doc comment MUST no longer describe the field as "transitional … can be retired"; it SHALL describe the retained binary-alias/display surface (the run-kit formula still installs `rk` as an interchangeable command alias).

- **GIVEN** the roster in `src/cmd/shll/tools.go`
- **WHEN** the change is applied
- **THEN** no `LegacyFormula` field or value exists anywhere in `src/`
- **AND** `shll update rk` / `shll install rk` / `shll uninstall rk` still resolve via `legacyAliases` with the `note: rk is now run-kit` notice

### update: plain single-probe path, no migration action

#### R2: `shll update` carries no migration machinery
`probeRunKitMigration`, its dispatch from `probeTool`, the `needsMigration`/`dualRack` fields on `probeResult`, `upgradeTool`'s migration branch and dual-rack note emission, `migrateRunKit`, and the constants `migrationDaemonNoteFmt`/`migrationDualRackNoteFmt` SHALL all be deleted (`src/cmd/shll/update.go`). `upgradeArgv`'s signature SHALL shrink to `upgradeArgv(t Tool, supportsSkipFlag bool)`, with the dry-run preview call site and install.go's preview call site following. `relinkNoteFmt` and the delegation-path unlinked-keg self-heal (`brew link` + one retry on `proc.ErrNotFound`) MUST be kept. Stale migration comments (the subset-run legacy-keg note, `beforeVersion`'s legacy-keg sentence, `probeTool`'s MIGRATION GATE block, the roster-loop dispatch bullet) MUST be removed or rewritten.

- **GIVEN** a never-migrated machine (legacy `rk` keg only, no `run-kit` keg)
- **WHEN** `shll update` runs whole-roster
- **THEN** run-kit probes `sahil87/tap/run-kit` → not installed → skipped gracefully (Constitution V), and `sahil87/tap/rk` is never referenced
- **AND WHEN** `shll update run-kit` is run on that machine
- **THEN** it errors `run-kit: not installed`, same as any absent named tool

### install: plain bootstrap path

#### R3: `shll install` carries no migration classification
`runInstall`'s missing-partition SHALL NOT branch on `LegacyFormula` — every tool goes through the plain `isInstalled(ctx, t.Formula)` check. The `installTarget` struct (whose only remaining field would be `tool`) SHALL be removed along with its `migrate` field, the dry-run migration preview row, and the whole trust-then-migrate action branch (`src/cmd/shll/install.go`).

- **GIVEN** a legacy-only machine
- **WHEN** `shll install` runs
- **THEN** run-kit classifies as missing → normal trust + `brew install sahil87/tap/run-kit` (orphan `rk` keg cleanup is manual per run-kit's README)

### doctor: no migration findings

#### R4: `shll doctor` carries no migration checks
`resolveMigrationFacts`, its call in `runDoctor`, the `migration probeResult` parameter on `evaluateTool`, the pending-migration WARN branch, the dual-rack WARN branch, and the constants `suggestPendingMigrationFmt`/`suggestDualRackFmt` SHALL be deleted (`src/cmd/shll/doctor.go`). Doctor's use of `probeVersion`'s legacy-name fallback (a `LegacyName` surface) MUST be kept.

- **GIVEN** a legacy-only machine (old `rk` binary still on PATH)
- **WHEN** `shll doctor` runs
- **THEN** run-kit's row is produced by the ordinary non-migration logic (the legacy-name fallback still reports a version honestly; no migration WARN, no `brew list` migration probe)

### uninstall: plain reverse-roster removal

#### R5: `shll uninstall` carries no legacy-keg sweep; the daemon-stop hint survives
`uninstallRunKit`, `probeRunKitInstalled`, the `runKit`/`runKitNewInstalled`/`runKitLegacyKeg` fields on `uninstallTarget`, the `LegacyFormula` branch in the actionable-set build, the `case a.runKit:` action branch, and the legacy preview rows in `previewRowsFor` SHALL be deleted (`src/cmd/shll/uninstall.go`) — run-kit becomes a plain `probeInstalledVersion` + `uninstallOne(t.Name, t.Formula)` target. The `runKitDaemonStopHintFmt` hint SHALL be re-keyed on successful removal of the run-kit roster entry, matched by name against the existing named constant `runKitToolName` (no magic strings). The Long help's mention of the `rk` legacy alias stays.

- **GIVEN** run-kit installed on the new formula
- **WHEN** `shll uninstall run-kit --yes` succeeds
- **THEN** exactly `brew uninstall sahil87/tap/run-kit` runs and the daemon-stop hint still prints
- **AND GIVEN** a legacy-only machine
- **WHEN** `shll uninstall run-kit --yes` runs
- **THEN** run-kit reports `not installed` and is skipped (repair-path semantics, exit 0)

### brew: collapse the leaf plumbing

#### R6: version-only probe — one `brew list` invocation, no leaf parsing
`probeInstalledLeaf` SHALL be folded into `probeInstalledVersion`, which becomes THE sole `brew list --formula --versions` invocation in `cmd/shll` (returning `installed, version`). `parseBrewLeaf` SHALL be deleted and the leaf-centric doc comments rewritten. `parseBrewVersion` (multi-keg max logic) and the thin `isInstalled`/`installedVersion` wrappers MUST be kept unchanged. The single-brew-invocation design decision stays intact.

- **GIVEN** the post-change `src/cmd/shll/brew.go`
- **WHEN** any caller needs a brew install/version fact
- **THEN** the fact flows from the single `probeInstalledVersion` read (or its thin wrappers), and no leaf name is parsed anywhere

### Tests: conform to the new spec

#### R7: tests are updated to the no-guard spec (Constitution Test Integrity)
Tests of removed machinery SHALL be deleted; tests asserting `sahil87/tap/rk` probes in fake-runner call logs SHALL be updated; coverage that a legacy-only machine is treated as "run-kit not installed" SHALL be added or kept on each surface (update / install / doctor / uninstall). Tests asserting the KEPT `LegacyName` surfaces (version fallback probe, agent-setup vocabulary token, alias-resolution tests) MUST stay green.

- **GIVEN** the updated test suite
- **WHEN** `go test ./...` runs in `src/`
- **THEN** all tests pass and no test references `LegacyFormula`, `needsMigration`, `dualRack`, `migrateRunKit`, `probeRunKitMigration`, `uninstallRunKit`, `probeRunKitInstalled`, `resolveMigrationFacts`, `probeInstalledLeaf`, or `parseBrewLeaf`

### Non-Goals

- No `homebrew-tap` change — `formula_renames.json` stays.
- No changes to `LegacyName` surfaces: `legacyAliases`, version.go's fallback probe, agent_setup.go's `run-kit/rk` vocabulary token, skill.go/changelog.go alias resolution.
- No memory (`docs/memory/`) edits during apply — hydrate owns them.
- No new subcommands, flags, or output surfaces (Constitution VII untouched).

### Design Decisions

#### Collapse `installTarget` to a plain `[]Tool`
**Decision**: With `migrate` gone, delete the `installTarget` struct entirely and make the missing-partition a `[]Tool`.
**Why**: A one-field wrapper struct is dead weight; the intake authorizes clean reductions of guard plumbing.
**Rejected**: Keeping a single-field struct — preserves nothing and reads as vestigial.
*Introduced by*: 260720-h3f6-retire-rk-migration-guard

#### Re-key the daemon-stop hint on `a.tool.Name == runKitToolName`
**Decision**: In the uninstall loop's success path, set `runKitName` when the removed roster entry's name equals the existing `runKitToolName` constant.
**Why**: The hint fires on any successful run-kit removal (its value is unrelated to formula migration); `runKitToolName` already exists (install.go) so no magic string is introduced.
**Rejected**: A new bool field on `uninstallTarget` — more plumbing for the same fact.
*Introduced by*: 260720-h3f6-retire-rk-migration-guard

### Deprecated Requirements

#### The rk→run-kit migration guard (9bak/kkaj machinery)
**Reason**: The migration window is closed; the permanent tap rename mapping makes brew's warning permanent noise on every `shll update`.
**Migration**: Stragglers follow run-kit's README manual path (`brew uninstall rk`, then `shll install`). N/A in shll code.

## Tasks

### Phase 1: Core removals (implementation files)

- [x] T001 Remove `LegacyFormula` field + run-kit's value from `src/cmd/shll/tools.go`; rewrite `LegacyName`'s doc comment as the retained binary-alias/display surface <!-- R1 -->
- [x] T002 Remove the migration guard from `src/cmd/shll/update.go`: `probeRunKitMigration` + `probeTool` dispatch, `needsMigration`/`dualRack` fields + all consumers, `migrateRunKit`, `migrationDaemonNoteFmt`/`migrationDualRackNoteFmt`; shrink `upgradeArgv` to `(t Tool, supportsSkipFlag bool)`; keep `relinkNoteFmt` + the unlinked-keg self-heal; clean stale comments <!-- R2 -->
- [x] T003 Remove the migration classification from `src/cmd/shll/install.go`: `installTarget` struct → `[]Tool`, drop the `LegacyFormula` partition branch, the dry-run migration row, and the trust-then-migrate action branch; fix the `upgradeArgv` preview call site <!-- R3 -->
- [x] T004 Remove migration findings from `src/cmd/shll/doctor.go`: `resolveMigrationFacts` + `runDoctor` call, `evaluateTool`'s `migration` param + both WARN branches, `suggestPendingMigrationFmt`/`suggestDualRackFmt`; keep the legacy-name version fallback <!-- R4 -->
- [x] T005 Remove the legacy-keg sweep from `src/cmd/shll/uninstall.go`: `uninstallRunKit`, `probeRunKitInstalled`, the three runKit fields, the `LegacyFormula` actionable branch, the `case a.runKit:` branch, the legacy preview rows; re-key the daemon-stop hint on `a.tool.Name == runKitToolName` <!-- R5 -->
- [x] T006 Collapse `src/cmd/shll/brew.go`: fold `probeInstalledLeaf` into `probeInstalledVersion` (the sole `brew list` invocation), delete `parseBrewLeaf`, rewrite leaf-centric doc comments; keep `parseBrewVersion` + `isInstalled`/`installedVersion` unchanged <!-- R6 -->

### Phase 2: Test conformance

- [x] T007 Update `src/cmd/shll/update_test.go`: delete the nine `TestUpdate_Migration*` tests + the `runKitLegacyFormula` fixture plumbing; keep/adjust alias-notice coverage; add legacy-only-machine coverage (whole-roster graceful skip + named `run-kit: not installed` error) <!-- R7 -->
- [x] T008 Update `src/cmd/shll/install_test.go`: delete `TestInstall_LegacyKegRoutesThroughMigration` / `TestInstall_MigrationTrustsRunKitFormulaFirst` / `TestInstall_MigrationNoTrustSkipsTrustStep`; add legacy-only-machine coverage (run-kit missing → trust + `brew install sahil87/tap/run-kit`); keep `TestInstall_LegacyAliasResolvesWithNotice` <!-- R7 -->
- [x] T009 Update `src/cmd/shll/doctor_test.go`: delete `TestDoctor_PendingMigration*` / `TestDoctor_DualRackWarns`; keep `TestDoctor_MigratedRunKitLegacyBinaryOnPathNotFail` (LegacyName surface), adjusting fakes off the removed leaf shapes; assert no migration `brew list` probes remain <!-- R7 -->
- [x] T010 Update `src/cmd/shll/uninstall_test.go`: delete the dual-rack/legacy-only sweep + preview tests; run-kit becomes a plain target; add legacy-only-machine coverage (`not installed` skip); keep `TestUninstall_LegacyAliasResolvesWithNotice` <!-- R7 -->
- [x] T011 Update `src/cmd/shll/brew_test.go`: delete `TestParseBrewLeaf` / `TestProbeInstalledLeaf_ReturnsLeafAndVersion`; keep/extend `probeInstalledVersion`-level coverage of the collapsed probe <!-- R7 -->
- [x] T012 Verify `src/cmd/shll/tools_test.go` carries no `LegacyFormula` assertions (alias tests stay); adjust only if compilation or assertions break <!-- R7 -->

### Phase 3: Verification

- [x] T013 `cd src && gofmt -l . && go vet ./... && go test ./...` all green; grep confirms zero references to the removed identifiers <!-- R7 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `Tool` has no `LegacyFormula` field; run-kit's roster entry sets none; `LegacyName` doc comment describes the retained alias/display surface
- [x] A-002 R2: update.go has no `probeRunKitMigration`/`migrateRunKit`/`needsMigration`/`dualRack`/migration-note constants; `upgradeArgv(t, supportsSkipFlag)` two-arg form
- [x] A-003 R3: install.go partitions missing tools solely via `isInstalled(ctx, t.Formula)`; no `installTarget`/`migrate` machinery
- [x] A-004 R4: doctor.go has no `resolveMigrationFacts`/migration WARNs/migration suggestion constants
- [x] A-005 R5: uninstall.go treats run-kit as a plain target; daemon-stop hint keyed on `runKitToolName` name match after successful removal
- [x] A-006 R6: `probeInstalledVersion` is the sole `brew list --formula --versions` invocation; `parseBrewLeaf` gone; `parseBrewVersion` + wrappers unchanged

### Behavioral Correctness

- [x] A-007 R2: no code path in update/install/doctor/uninstall ever references `sahil87/tap/rk`
- [x] A-008 R5: successful run-kit removal still prints the daemon-stop hint; failed removal does not

### Removal Verification

- [x] A-009 R7: zero source or test references to the removed identifiers (grep-verified)

### Scenario Coverage

- [x] A-010 R2: tests cover legacy-only machine → whole-roster `shll update` skips run-kit; `shll update run-kit` errors not-installed
- [x] A-011 R3: test covers legacy-only machine → `shll install` runs trust + `brew install sahil87/tap/run-kit`
- [x] A-012 R4: test covers legacy-name fallback still reporting a version with no migration WARN
- [x] A-013 R5: test covers legacy-only machine → `shll uninstall run-kit` reports not-installed, exit 0

### Edge Cases & Error Handling

- [x] A-014 R2: the unlinked-keg self-heal (`relinkNoteFmt`, `brew link` + one retry on `proc.ErrNotFound`) is preserved with its tests green
- [x] A-015 R1: `rk` alias resolution + notice still works on update/install/uninstall (existing tests green)

### Code Quality

- [x] A-016 Pattern consistency: retained code follows existing patterns; no magic strings introduced (daemon hint keyed via `runKitToolName`)
- [x] A-017 No unnecessary duplication: single-brew-invocation design intact (one `brew list` read powering all install/version facts)
- [x] A-018 All subprocess invocation stays routed through `internal/proc` (Constitution I); no direct `os/exec` introduced

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

- None — this change IS the deletion. It retired the migration guard (`LegacyFormula` + all keyed machinery: `probeRunKitMigration`, `migrateRunKit`, `resolveMigrationFacts`, `uninstallRunKit`, `probeRunKitInstalled`, `probeInstalledLeaf`, `parseBrewLeaf`, the `installTarget` struct, and the migration note/suggestion constants) at the root. Verified no now-orphaned code remains: every retained helper (`brewTrustFormula`, `probeVersionByName`, `toolSupportsSkipFlag`, `parseBrewVersion`, `isInstalled`, `installedVersion`, `probeInstalledVersion`, `brewUninstallArgv`, `uninstallOne`, `relinkNoteFmt` + the self-heal) keeps its call sites, and every retained `LegacyName` surface (`legacyAliases`, version fallback, agent-setup token) is still referenced.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Delete the `installTarget` struct entirely (missing-partition becomes `[]Tool`) rather than keeping a one-field wrapper | Intake authorizes clean reductions; a single-field struct is vestigial; trivially reversible | S:70 R:90 A:90 D:80 |
| 2 | Certain | Daemon-stop hint re-keyed via `a.tool.Name == runKitToolName` on the success path | Intake assumption 6 delegated the mechanism to apply with a named-constant constraint; `runKitToolName` already exists in install.go | S:75 R:90 A:95 D:90 |
| 3 | Confident | Legacy-only-machine coverage is one focused test per surface (update ×2 paths, install, doctor, uninstall), driven by fakes where the `sahil87/tap/run-kit` probe exits 1 | Intake requires "add/keep coverage … on each surface" without prescribing shape; mirrors existing per-surface fake-runner test patterns | S:65 R:85 A:90 D:80 |

3 assumptions (1 certain, 2 confident, 0 tentative).
