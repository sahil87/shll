# Plan: run-kit Rename + Migration Guard

**Change**: 260709-9bak-run-kit-rename-migration-guard
**Intake**: `intake.md`

## Requirements

### Roster: run-kit identity rename

#### R1: Roster entry identity flips to run-kit
The `Roster` entry for the tmux/web-UI tool SHALL be renamed from `rk` to `run-kit`: `Name: "run-kit"`, `Formula: sahil87/tap/run-kit` (via `formulaPrefix + "run-kit"`), `Update: []string{"run-kit", "update"}`. The `Repo` field SHALL stay `"run-kit"` (now equal to `Name`). The roster POSITION is unchanged (4th, between `tu` and `hop`). The `Description` may drop the parenthetical "(rk stays as an alias)" or keep the existing text — it MUST remain non-empty.

- **GIVEN** the renamed roster
- **WHEN** any command iterates `Roster`
- **THEN** the tool is named `run-kit` everywhere (display in `list`/`version`/`doctor`, the update digest, JSON output), its formula is `sahil87/tap/run-kit`, and its update delegation is `run-kit update`
- **AND** `Repo` resolves the repo URL to `https://github.com/sahil87/run-kit` (unchanged), so `TestList_RepoLinks` still passes and `github.com/sahil87/rk` remains absent

#### R2: Leaves-first ordering contract updated for the rename
The ordering invariant test `TestRosterLeavesBeforeDependents` and the `tools.go` ordering comment SHALL reference the renamed tool: the `rk -> wt` runtime-invocation edge becomes `run-kit -> wt`. The `rk`/`run-kit` repo-slug footgun note in `tools.go` (and the roster-invariant docs) SHALL be updated to reflect that `Name` and `Repo` now MATCH for run-kit (the `github.com/sahil87/rk` 404 note is retired/repointed to the legacy alias context, not the roster identity).

- **GIVEN** the leaves-first invariant test
- **WHEN** it builds the `name → index` map from the live `Roster`
- **THEN** the edge `{dependent: "run-kit", dep: "wt"}` is asserted (not `{dependent: "rk", ...}`) and passes because `run-kit` sits after `wt`

### Targets: legacy `rk` alias

#### R3: `rk` resolves to `run-kit` as a legacy target alias
`resolveTargets` SHALL accept the legacy token `rk` and resolve it to the canonical `run-kit` tool for both `shll update rk` and `shll install rk`. `resolveTargets` MUST stay IO-free (no subprocess/brew calls). It SHALL return enough information for the caller to print a one-line notice (e.g. `note: rk is now run-kit`) to stdout. The valid-targets list in error messages (`validTargets`) SHALL show canonical names only — `rk` MUST NOT appear in the valid-targets diagnostic.

- **GIVEN** `shll update rk` (or `shll install rk`)
- **WHEN** `resolveTargets` processes the args
- **THEN** the `run-kit` tool is selected exactly as if `run-kit` had been named, and the alias use is signalled to the caller
- **AND** the caller prints a one-line `note: rk is now run-kit` to stdout
- **AND** `shll update foo` still errors `unknown target "foo" (valid targets: shll, wt, idea, tu, run-kit, hop, fab-kit)` with no `rk` in the list

### version: PATH-probe legacy fallback

#### R4: `probeToolVersion` falls back to the legacy `rk` binary on ErrNotFound only
`probeToolVersion` SHALL, when the primary `<tool.Name> --version` fails with `proc.ErrNotFound` **only** (not a non-zero exit, not a timeout/deadline), AND the tool declares a legacy binary name, retry once with the legacy binary name. For `run-kit` the legacy name is `rk`. The display name in `version`/`list`/`doctor` SHALL remain `run-kit` regardless of which probe name succeeded. This fallback serves display surfaces only; it MUST NOT influence the brew-keg migration gate.

- **GIVEN** a pre-2.5.13 install where only `rk` is on PATH (no `run-kit` binary)
- **WHEN** `shll list`/`version`/`doctor` probe `run-kit --version` and get `proc.ErrNotFound`
- **THEN** the probe retries `rk --version`, succeeds, and the row shows `run-kit <version>` as installed
- **AND** a present-but-broken `run-kit` (non-zero exit, timeout) does NOT trigger the fallback (it stays reported via the primary probe's error)

### update/install: migration guard

#### R5: Migration gate classifies by keg leaf name, never exit code alone
The install/version probe SHALL expose the keg **leaf name** parsed from `brew list --formula --versions` stdout (the first whitespace field of the first non-empty line, e.g. `rk` from `rk 2.5.13` or `run-kit` from `run-kit 3.0.0`). The migration gate in `probeTool` SHALL classify run-kit as follows: (1) new-formula probe (`sahil87/tap/run-kit`) installed with leaf `run-kit` → migrated, normal delegation; (2) new-formula probe not-installed → probe the legacy formula `sahil87/tap/rk`; if it reports leaf `rk` → legacy keg → `needsMigration=true` with `beforeVersion` from the legacy keg; (3) neither → not installed. Observed state B (new-formula probe reports leaf `run-kit` post-rename-resolution) MUST classify as migrated, not legacy.

- **GIVEN** observed state A/B/C from the intake
- **WHEN** `probeTool` runs for run-kit
- **THEN** state A (legacy keg leaf `rk`, unlinked) → `needsMigration=true`, beforeVersion from the `rk` keg; state B (`brew list ... run-kit` reports leaf `run-kit`) → migrated (normal delegation); state C (both kegs; leaf `run-kit` from the new-formula probe) → migrated, with dual-rack orphan detected

#### R6: Migration action is brew-direct, never delegated to the old binary
For a `needsMigration` run-kit, the sequential upgrade loop SHALL run the migration action instead of delegating to `run-kit update`: (1) `brew upgrade sahil87/tap/rk` (brew resolves the rename and migrates the keg); (2) post-check — if `run-kit --version` still fails with `proc.ErrNotFound`, run `brew link run-kit`; (3) print (never run) a note suggesting `run-kit serve --restart` for the daemon; (4) if the dual-rack leftover is detected, print a one-line cleanup note. The migration MUST NOT delegate to the old binary and MUST NOT uninstall→install.

- **GIVEN** a legacy-keg machine running `shll update` or `shll update run-kit`
- **WHEN** the upgrade loop reaches run-kit
- **THEN** `brew upgrade sahil87/tap/rk` runs (brew-direct); if `run-kit --version` still `ErrNotFound` afterward, `brew link run-kit` runs; a `run-kit serve --restart` note is printed but never executed; the old binary is never invoked for the migration

#### R7: Both whole-roster and named-subset updates flow through the guard
`shll update` (whole roster) and `shll update run-kit` (and `shll update rk` via the alias) SHALL both flow through `probeTool` + the migration action. The named-subset "not installed" error SHALL treat a legacy-keg machine as installed (the leaf-name gate marks it `installed=true, needsMigration=true`), so `shll update run-kit` on a legacy machine migrates rather than erroring `not installed`.

- **GIVEN** a legacy-keg machine
- **WHEN** the user runs `shll update run-kit` (or `shll update rk`)
- **THEN** the subset run treats run-kit as installed and performs the migration action, never printing `run-kit: not installed`

#### R8: Dry-run preview shows the real migration argv via a single source of truth
The `--dry-run` preview SHALL render the exact migration command (`brew upgrade sahil87/tap/rk`) for a `needsMigration` run-kit, sourced from the same single-source-of-truth (`upgradeArgv` or a parallel mechanism) the live run uses, so preview and live run cannot drift.

- **GIVEN** a legacy-keg machine
- **WHEN** `shll update --dry-run` (or `... --dry-run run-kit`) runs
- **THEN** the preview row for run-kit reads `brew upgrade sahil87/tap/rk` (the migration argv), and this string is built from the same code path the live migration uses

#### R9: Digest reads before-version from the legacy keg and after-version from the new formula
For a migrated run-kit, the "What changed:" digest `beforeVersion` SHALL come from the legacy keg (e.g. `2.5.13`), and the post-upgrade after-version re-query SHALL read the NEW formula `sahil87/tap/run-kit` (`installedVersion(ctx, "sahil87/tap/run-kit")`), so the digest renders `run-kit 2.5.13 → 3.0.0`. Release notes continue to key off `Repo: "run-kit"`.

- **GIVEN** a legacy `rk` keg at 2.5.13 being migrated to run-kit 3.0.0
- **WHEN** the digest is built after a successful migration
- **THEN** the bump reads `run-kit 2.5.13 → 3.0.0` (before from the legacy `rk` keg, after re-queried from `sahil87/tap/run-kit`)

### install: legacy-keg routing

#### R10: `shll install` routes a legacy-keg run-kit through the migration action
`shll install run-kit` (or whole-roster `shll install`) on a legacy-keg machine SHALL detect the legacy keg via the same gate helper and run the migration action instead of a blind `brew install sahil87/tap/run-kit`. A migrated or fully-absent run-kit SHALL retain unchanged install behavior (blind `brew install` for absent; skip for present).

- **GIVEN** a legacy-keg machine
- **WHEN** `shll install run-kit` runs
- **THEN** the migration action (`brew upgrade sahil87/tap/rk` + post-check/link + notes) runs instead of `brew install sahil87/tap/run-kit`
- **AND** an entirely-absent run-kit still runs the plain `brew install sahil87/tap/run-kit`

### doctor: pending-migration + dual-rack WARN

#### R11: Doctor WARNs on pending migration and dual-rack orphan (read-only)
`shll doctor` SHALL WARN when the legacy `rk` keg (leaf `rk`) is still present ("pending migration — run `shll update run-kit`") and when the dual-rack orphan (both kegs) is detected. It SHALL reuse the guard's detection helper, stay strictly read-only, and never affect the exit code beyond the existing WARN semantics (WARN never flips exit to 1). The PATH-probe legacy fallback (R4) flows through automatically. The formula-trust list references SHALL be verified/updated to the new formula name where run-kit is involved.

- **GIVEN** a legacy-keg machine (state A) or a dual-rack machine (state C)
- **WHEN** `shll doctor` evaluates run-kit
- **THEN** run-kit's row is WARN with a pending-migration suggestion (state A) or a dual-rack note (state C), exit code unaffected by the WARN

### Non-Goals

- No changes to the run-kit repo or the Homebrew tap (both are done, verified live as prerequisites).
- No uninstall→install in the update/install path (that repair sequence is owned by the sibling `shll uninstall` change).
- No generic unlinked-keg doctor check for arbitrary tools — only the run-kit migration states.
- No automatic cleanup of the dual-rack orphan — detection + one-line note only.
- No dynamic tool discovery — the roster stays hardcoded (Constitution: Tool Roster Source of Truth).
- `shll changelog rk@...` alias — in scope ONLY if changelog's name-matching reuses the same `resolveTargets` helper (it does, via `parseChangelogSpecs`), so the alias lands there for free; no bespoke changelog code beyond what the shared alias provides.

### Design Decisions

1. **Legacy alias shape**: add a package-level `legacyAliases = map[string]string{"rk": "run-kit"}` consulted inside `resolveTargets` before `rosterHas`, and return a `[]string` (or bool) of aliased names so the caller prints the notice. — *Why*: minimal, IO-free, single-sourced, and automatically shared by `update`/`install`/`changelog` (all route through `resolveTargets`). — *Rejected*: an `Aliases []string` field on `Tool` (heavier; would require every consumer to scan the roster for alias membership rather than a direct map lookup, and complicates the changelog spec parser which keys by name).

2. **Legacy binary name on the Tool struct**: add a `LegacyName string` field to `Tool` (empty for all but run-kit, `"rk"`) so `probeToolVersion`'s fallback is data-driven, not a hardcoded `if tool.Name == "run-kit"`. — *Why*: keeps the roster the single source of truth (Constitution III) and matches the existing `Repo`-override pattern. — *Rejected*: a hardcoded name check in `version.go` (couples probe logic to a specific tool name).

3. **Leaf-name exposure**: add a sibling `probeInstalledLeaf(ctx, formula) (installed bool, leaf, version string)` (or extend `probeInstalledVersion` to also return the leaf) in `brew.go`, parsing the first field of the `brew list` line via `strings.Fields` (never a regex — code-quality.md). The migration gate consumes the leaf; existing callers keep the two-return `probeInstalledVersion` wrapper. — *Why*: the intake calls out that `parseBrewVersion` discards the leaf token; exposing it is the smallest change. — *Rejected*: re-parsing brew output at the call site (duplicates the sole `brew list` invocation).

4. **Migration argv single-source-of-truth**: represent the migration in `probeResult` as a `needsMigration bool` and have `upgradeArgv`/`upgradeTool` branch on it, returning/executing the migration sequence. The dry-run preview calls the same `upgradeArgv` (which returns the FIRST/primary migration command `brew upgrade sahil87/tap/rk` for preview) so preview and live cannot drift for the primary action. The conditional `brew link` and the daemon/dual-rack notes are live-run-only side effects (a dry-run preview of a conditional post-check would be misleading), documented as such. — *Why*: matches the intake's "same single source of truth as the live run (`upgradeArgv` or parallel mechanism)". — *Rejected*: a fully separate preview builder for migration (drift risk the intake explicitly warns against).

5. **Shared migration detection helper**: extract a small `runKitMigration` struct + a `classifyRunKit`-style helper (or reuse the leaf-parsing gate) so `update`, `install`, and `doctor` all consume ONE detection path. — *Why*: the intake requires doctor and install to "reuse the guard's detection helper". — *Rejected*: independent re-detection in each command (drift + Constitution III).

## Tasks

### Phase 1: Roster identity, leaf parser, alias (foundation)

- [x] T001 In `src/cmd/shll/tools.go`, add a `LegacyName string` field to the `Tool` struct (doc: legacy binary name for the PATH-probe fallback; empty for all tools except run-kit) and a package-level `legacyAliases = map[string]string{"rk": "run-kit"}` with a doc comment (transitional; sunset note). <!-- R1 R3 -->
- [x] T002 In `src/cmd/shll/tools.go`, rename the roster entry `rk` → `run-kit`: `Name: "run-kit"`, `Formula: formulaPrefix + "run-kit"`, `Update: []string{"run-kit", "update"}`, `Repo: "run-kit"`, `LegacyName: "rk"`, keeping the roster POSITION (4th) unchanged. Update the ordering comment (`rk -> wt` edge → `run-kit -> wt`) and the `Repo != Name` footgun note (Name and Repo now match for run-kit; the `github.com/sahil87/rk` 404 note repoints to the legacy-alias context). <!-- R1 R2 -->
- [x] T003 In `src/cmd/shll/tools.go`, update `resolveTargets` to consult `legacyAliases` before `rosterHas`: an aliased arg resolves to the canonical roster tool and is recorded so the caller can print a notice. Add a return value carrying the aliased names (change signature to `(selected []Tool, selfSelected bool, aliased []string, err error)`), or a helper the caller can query. Keep `resolveTargets` IO-free. Ensure `validTargets` (the error diagnostic) lists canonical names only (no `rk`). <!-- R3 -->
- [x] T004 In `src/cmd/shll/brew.go`, expose the keg leaf name: add `parseBrewLeaf(out string) string` (first whitespace field of the first non-empty line, `""` on empty) and a `probeInstalledLeaf(ctx, formula) (installed bool, leaf, version string)` (the sole `brew list --formula --versions` read now also returns the leaf). Keep `probeInstalledVersion`/`isInstalled`/`installedVersion` as thin wrappers over the new primitive so there is still exactly one `brew list` invocation. <!-- R5 -->

### Phase 2: version PATH-probe fallback

- [x] T005 In `src/cmd/shll/version.go`, update `probeToolVersion` to retry with `tool.LegacyName` when (and only when) the primary `<tool.Name> --version` fails with `proc.ErrNotFound` AND `tool.LegacyName != ""`. On any other error (non-zero exit, timeout/deadline), do NOT retry — return the primary error. Display name is untouched (callers still use `tool.Name`). <!-- R4 -->

### Phase 3: migration guard (update)

- [x] T006 In `src/cmd/shll/update.go`, add a `needsMigration bool` field to `probeResult` (doc: transitional run-kit legacy-keg flag; sunset note). <!-- R5 -->
- [x] T007 In `src/cmd/shll/update.go`, extend `probeTool` to run the run-kit migration gate: when the tool declares a legacy formula (add a `LegacyFormula string` field on `Tool`, `sahil87/tap/rk` for run-kit) — if the primary `probeInstalledLeaf(ctx, t.Formula)` reports installed with leaf == `t.Name` → migrated (normal); if the primary probe is not-installed, probe `t.LegacyFormula`; if THAT reports leaf == the legacy leaf (`rk`) → set `installed=true, needsMigration=true, beforeVersion` from the legacy keg. Otherwise not installed. Non-run-kit tools (no `LegacyFormula`) keep today's single-probe behavior. Add the `LegacyFormula` field + run-kit value in `tools.go`. <!-- R5 -->
- [x] T008 In `src/cmd/shll/update.go`, add the migration action helper `migrateRunKit(ctx, stdout, stderr, t) (int, error)` (or inline in the loop): (1) `proc.RunForeground(ctx, brewBinary, "upgrade", t.LegacyFormula)`; (2) post-check `probeToolVersion`-style `run-kit --version` — if `proc.ErrNotFound`, `proc.RunForeground(ctx, brewBinary, "link", t.Name)`; (3) print the `run-kit serve --restart` daemon note (named constant, never executed); (4) if the dual-rack leftover is detected (both `t.Formula` and `t.LegacyFormula` report installed), print a one-line cleanup note. Return the migration's exit code. <!-- R6 --> <!-- rework: review must-fix — R6(4) dual-rack note is dead code (a needsMigration probe never carries dualRack; a pre-migration dual-rack is branch 1, not a migration). Detect dual-rack by RE-PROBING the legacy formula (leaf `rk`) AFTER the upgrade inside migrateRunKit, per this task's own prescription. Also (review should-fix): gate steps 2–4 on the brew upgrade exit code == 0 — a failed migration must not run the link post-check or print the daemon note. -->
- [x] T009 In `src/cmd/shll/update.go`, wire the migration into `upgradeArgv` and `upgradeTool`: for a `needsMigration` tool, `upgradeArgv` returns the primary migration argv `{brew, upgrade, t.LegacyFormula}` (single source of truth for the dry-run preview), and `upgradeTool` (or the loop) runs the full `migrateRunKit` sequence. Thread `needsMigration` from `probes[i]` into the upgrade-loop call. <!-- R6 R8 -->
- [x] T010 In `src/cmd/shll/update.go`, ensure the digest re-query for a migrated run-kit reads the NEW formula: after a successful migration, `makeBump(t.Name, t.Repo, probes[i].beforeVersion, installedVersion(ctx, t.Formula))` where `t.Formula` is `sahil87/tap/run-kit` (already the renamed formula) — confirm beforeVersion came from the legacy keg (T007) so the bump renders `run-kit 2.5.13 → 3.0.0`. <!-- R9 -->
- [x] T011 In `src/cmd/shll/update.go`, ensure the named-subset not-installed check treats a legacy-keg machine as installed: since T007 marks `probes[i].installed=true` for a legacy keg, `shll update run-kit` passes the not-installed guard and proceeds to migration. Verify the subset filter (`probes[i].installed = false` for non-selected) does not clear the migration flag for a selected run-kit. <!-- R7 -->
- [x] T012 In `src/cmd/shll/update.go` (and the shared caller path), print the one-line alias notice (`note: rk is now run-kit`) to stdout when `resolveTargets` reports `rk` was aliased. Add the notice as a named constant. <!-- R3 -->

### Phase 4: install-path guard + doctor

- [x] T013 In `src/cmd/shll/install.go`, update the missing-partition + install loop: probe the run-kit gate (reuse the shared detection from T007's helper) — when a legacy keg is detected for run-kit, run the same `migrateRunKit` migration action instead of `brew install sahil87/tap/run-kit`; a fully-absent run-kit keeps the blind `brew install`; a migrated/present run-kit is skipped as installed. Also print the alias notice for `shll install rk`. <!-- R10 R3 --> <!-- rework: review should-fix — the migration route must NOT skip install's per-formula trust step (installed ≠ trusted; that inequality is doctor's trust-WARN premise, change 0854). When trustEnabled, trust sahil87/tap/run-kit before running the migration action, matching the trust-then-act contract of the normal install path. -->
- [x] T014 In `src/cmd/shll/doctor.go`, add pending-migration + dual-rack WARN: reuse the shared detection helper (from T007) to classify run-kit; when a legacy keg is present → WARN with a "pending migration — run `shll update run-kit`" suggestion (named constant); when dual-rack → WARN/note. Stay read-only (no brew writes). Confirm the PATH-probe fallback (T005) flows through `probeVersion` so a legacy-only-on-PATH run-kit is not FAIL. Verify formula-trust references (`suggestNotTrustedFmt`, tap/formula names) use the new formula name for run-kit. <!-- R11 --> <!-- rework: review should-fix ×2 — (a) the pending-migration WARN hardcodes OnPath=true/VersionOK=true without probing; in observed state A (unlinked keg) the binary is NOT on PATH, so --json field semantics are misreported to CI consumers: run the real probe (or report the fields honestly) while keeping the WARN dominance. (b) probeVersion duplicates probeToolVersion's ErrNotFound-only legacy fallback verbatim — call probeToolVersion instead so the R4 fallback contract lives in one place. -->

### Phase 5: tests

- [x] T015 In `src/cmd/shll/tools_test.go`, update `TestRosterLeavesBeforeDependents` edge `{dependent: "rk", dep: "wt"}` → `{dependent: "run-kit", dep: "wt"}` and its comment; add `resolveTargets` tests: `rk` resolves to `run-kit` (with aliased signalled), `validTargets` lists `run-kit` not `rk`, alias works for both allowShll true/false. Update `TestShllSelf_*` if any reference rk (they don't). <!-- R1 R2 R3 -->
- [x] T016 In `src/cmd/shll/version_test.go`, add a test that a run-kit install visible only under the legacy `rk` binary (primary `run-kit --version` → `proc.ErrNotFound`, `rk --version` → version) is reported installed with display name `run-kit`; and that a present-but-broken `run-kit` (non-`ErrNotFound` error) does NOT fall back. Update any `rk` roster-name references. <!-- R4 -->
- [x] T017 In `src/cmd/shll/update_test.go`, update all `"rk"` roster references to `"run-kit"` where they mean the roster tool (delegated `run-kit update`, `formulaPrefix+"run-kit"`, headers, digest lines — note `changelog.CompareURL("run-kit", ...)` repo slug is unchanged). Add gate-classification tests for observed states A (legacy keg unlinked → migration via `brew upgrade sahil87/tap/rk` + `brew link run-kit`), B (`brew list sahil87/tap/rk` reports leaf `run-kit` → migrated, normal delegation, no migration), C (dual-rack → migrated + cleanup note). Add: `shll update run-kit` / `shll update rk` on a legacy machine migrates (not "not installed"); alias notice printed; dry-run preview shows `brew upgrade sahil87/tap/rk`; digest renders `run-kit 2.5.13 → 3.0.0`. <!-- R5 R6 R7 R8 R9 R3 -->
- [x] T018 In `src/cmd/shll/install_test.go`, update `rk` roster references to `run-kit` (`formulaPrefix+"run-kit"`, the `brew install sahil87/tap/run-kit` preview row). Add: legacy-keg run-kit routes through migration (`brew upgrade sahil87/tap/rk`) instead of `brew install`; absent run-kit still `brew install sahil87/tap/run-kit`; `shll install rk` alias resolves + prints the notice. <!-- R10 R3 -->
- [x] T019 In `src/cmd/shll/doctor_test.go`, update `rk` roster references to `run-kit`; add: legacy-keg run-kit → WARN pending-migration (exit unaffected); dual-rack → WARN/note; legacy-only-on-PATH run-kit → not FAIL (PATH fallback). Update the non-shell-init tool lists (`idea`, `rk`, `fab-kit` → `idea`, `run-kit`, `fab-kit`). <!-- R11 R4 -->
- [x] T020 In `src/cmd/shll/list_test.go` and `src/cmd/shll/changelog_test.go`, update `rk` roster references to `run-kit`; confirm `TestList_RepoLinks` still asserts run-kit resolves to `.../run-kit` and `.../rk` is absent (now via `Name == Repo == "run-kit"`). For changelog: `shll changelog rk` (no-range) and `shll changelog rk@0.1.0..0.2.0` still work via the alias (repo slug run-kit unchanged); update the roster-name references. <!-- R1 R3 -->
- [x] T021 Run `cd src && go test ./...` (scoped `go test ./cmd/shll/...` first), fix failures, iterate until green. <!-- R1 R2 R3 R4 R5 R6 R7 R8 R9 R10 R11 --> <!-- rework: re-run after the T008/T013/T014 rework; add test pins for the reworked behavior (post-migration dual-rack re-probe note, failed-migration gating, install trust-before-migrate, doctor honest on_path/version_ok in state A). Also: delete the stray untracked 10MB `src/shll` compiled binary (blocks /git-pr), and update README.md rk→run-kit identity references (lines ~5, 71, 90, 139, 146, 160 — incl. the valid-targets list). -->

## Execution Order

- T001–T004 (Phase 1) are the foundation; T002 depends on T001 (fields), T003 depends on T001 (`legacyAliases`), T004 is independent within Phase 1.
- T005 depends on T001 (`LegacyName`).
- T006–T012 (Phase 3) depend on T004 (leaf parser) and T007's `LegacyFormula` field; T008/T009 depend on T007; T010/T011 depend on T007.
- T013 (install) and T014 (doctor) depend on T007's shared detection helper.
- T015–T020 depend on the corresponding source tasks; T021 runs last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: The `Roster` run-kit entry has `Name: "run-kit"`, `Formula: "sahil87/tap/run-kit"`, `Update: {"run-kit","update"}`, `Repo: "run-kit"`, at its original 4th position; display/JSON/digest all render `run-kit`.
- [x] A-002 R2: `TestRosterLeavesBeforeDependents` asserts the `run-kit -> wt` edge and passes; the `tools.go` ordering + footgun comments reflect the rename.
- [x] A-003 R3: `resolveTargets` resolves `rk` → `run-kit` IO-free and signals the alias; `validTargets` lists canonical names only; callers print `note: rk is now run-kit`.
- [x] A-004 R4: `probeToolVersion` retries the legacy `rk` binary only on `proc.ErrNotFound` for run-kit; display stays `run-kit`; other errors do not trigger the fallback.
- [x] A-005 R5: `probeTool` classifies run-kit by keg leaf name across states A/B/C; the leaf parser (`parseBrewLeaf`/`probeInstalledLeaf`) exposes the first field with no regex.
- [x] A-006 R6: A `needsMigration` run-kit runs `brew upgrade sahil87/tap/rk`, conditionally `brew link run-kit` on post-check `ErrNotFound`, prints (never runs) the daemon note, and never delegates to the old binary.
- [x] A-007 R7: Both `shll update` and `shll update run-kit`/`rk` flow through the guard on a legacy machine; the named-subset not-installed error treats the legacy keg as installed.
- [x] A-008 R8: The dry-run preview renders `brew upgrade sahil87/tap/rk` for run-kit, sourced from the same `upgradeArgv` the live run uses.
- [x] A-009 R9: The digest bump for a migrated run-kit reads before from the legacy `rk` keg and after from `sahil87/tap/run-kit`, rendering `run-kit 2.5.13 → 3.0.0`.
- [x] A-010 R10: `shll install run-kit`/`rk` on a legacy machine runs the migration action instead of `brew install`; absent run-kit still `brew install sahil87/tap/run-kit`.
- [x] A-011 R11: `shll doctor` WARNs (exit unaffected) for pending-migration (legacy keg) and dual-rack; reuses the guard detection helper; is read-only; formula-trust references use the new formula name.

### Behavioral Correctness

- [x] A-012 R5: Observed state B (`brew list sahil87/tap/rk` reports leaf `run-kit`) classifies as migrated, NOT legacy — exit-code alone does not gate.
- [x] A-013 R4: The scope separation holds — the PATH-probe legacy fallback affects only display surfaces (`list`/`version`/`doctor`), never the brew-keg migration gate (a non-brew `rk` install is shown but never migrated).
- [x] A-014 R6: The migration is brew-direct and idempotent-safe; a dual-rack (state C) emits only a one-line cleanup note and no destructive action.

### Scenario Coverage

- [x] A-015 R5 R6 R7: `update_test.go` covers states A/B/C, whole-roster + subset (`run-kit`/`rk`) migration, and the not-installed→installed reclassification.
- [x] A-016 R8 R9: `update_test.go` pins the dry-run migration argv and the `run-kit 2.5.13 → 3.0.0` digest transition.
- [x] A-017 R10 R11: `install_test.go` and `doctor_test.go` cover the legacy-keg routing and the pending-migration/dual-rack WARN.

### Edge Cases & Error Handling

- [x] A-018 R4: A present-but-broken `run-kit` (non-`ErrNotFound`) does not silently defer to `rk`.
- [x] A-019 R6: Post-migration `brew link run-kit` runs only when `run-kit --version` still returns `proc.ErrNotFound` (the unlinked-keg pathology, state A); a linked migration skips it.
- [x] A-020 R3: `shll update foo` (genuine unknown) still errors with a canonical-only valid-targets list that includes `run-kit` and excludes `rk`.

### Code Quality

- [x] A-021 Pattern consistency: New code follows the existing `probeResult`/`upgradeArgv`/named-constant patterns and the `runXxx(ctx, writers…)` test seam; no magic strings (constants for the alias notice, daemon note, migration suggestions).
- [x] A-022 No unnecessary duplication: The keg-leaf parse lives in exactly one place (`brew.go`); the migration detection is a single shared helper reused by `update`/`install`/`doctor`; the leaf parser splits on whitespace, never a regex (code-quality.md anti-pattern).
- [x] A-023 Constitution I: All new subprocess invocations (`brew upgrade sahil87/tap/rk`, `brew link run-kit`, `rk --version` fallback) route through `internal/proc`; tests use the fake `proc.Runner` seam with no live brew.
- [x] A-024 Constitution III/IV/V: Migration wraps brew (never reimplements run-kit's logic — the daemon note is printed, not executed); delegation stays the norm for non-migration runs; an absent run-kit still degrades gracefully; the guard is documented as transitional (sunset note in code comments).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

None — this change adds new functionality without making existing code redundant. All three candidates from the prior review cycle are resolved by the rework: the `migrateRunKit` dead `dualRack` branch was replaced by a live post-migration re-probe of the legacy formula (`src/cmd/shll/update.go:669`); `doctor.go`'s inline legacy-name fallback in `probeVersion` was replaced by delegation to the shared `probeToolVersion` (`src/cmd/shll/doctor.go:446`); and the stray `src/shll` compiled binary was deleted. The `Tool.Repo` field (now `Name == Repo` for every roster entry) was considered and deliberately retained per its updated doc comment (`src/cmd/shll/tools.go:51-56`) as future-proofing for a divergent binary/repo slug.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Roster rename keeps position 4th and `Repo: "run-kit"` (Name==Repo now); LegacyName/LegacyFormula added as new `Tool` fields | Intake gives the exact roster literal and says position unchanged, Repo stays run-kit; new fields mirror the existing `Repo`-override pattern (Constitution III — roster is the source of truth) | S:95 R:80 A:90 D:90 |
| 2 | Certain | Migration gate classifies by keg leaf name from `brew list --formula --versions` stdout (first field), never exit code alone | Intake assumption #2 (Certain) + observed state B proves exit-code gating is insufficient; leaf parse via `strings.Fields` matches `parseBrewVersion`'s no-regex rule | S:90 R:85 A:90 D:90 |
| 3 | Certain | Migration action is brew-direct (`brew upgrade sahil87/tap/rk` + conditional `brew link run-kit`), never delegate to the old binary, never uninstall→install | Intake assumption #3 + area 4 spell out the exact sequence; one deterministic path for linked/unlinked states | S:85 R:75 A:85 D:80 |
| 4 | Confident | Legacy alias implemented as a `legacyAliases` map consulted in `resolveTargets`, returning aliased names for the caller to print the notice; `resolveTargets` signature gains an `aliased []string` return | Intake area 2 says "plan decides the shape" and lists a map as one option; the map is IO-free, single-sourced, and shared by update/install/changelog automatically. The signature change touches 3 callers but is mechanical | S:75 R:70 A:80 D:70 |
| 5 | Confident | PATH-probe fallback is data-driven via a `LegacyName` field, retried only on `proc.ErrNotFound`, and affects display surfaces only | Intake area 3 fixes the ErrNotFound-only trigger and display-only scope; the `LegacyName` field avoids a hardcoded tool-name check (Constitution III) | S:80 R:80 A:85 D:80 |
| 6 | Confident | Dry-run preview shows only the PRIMARY migration command (`brew upgrade sahil87/tap/rk`) via `upgradeArgv`; the conditional `brew link` + daemon/dual-rack notes are live-run-only | Intake says "show the real migration argv via the same single source of truth (`upgradeArgv` or parallel mechanism)"; a conditional post-check cannot be previewed accurately, so previewing the primary action is the faithful single-source read | S:65 R:70 A:75 D:65 |
| 7 | Confident | A single shared detection helper (leaf-name classifier keyed on `LegacyFormula`) is reused by update/install/doctor | Intake requires doctor + install to "reuse the guard's detection helper"; one path avoids drift (Constitution III) | S:70 R:80 A:80 D:75 |
| 8 | Confident | Daemon restart handling is a printed `run-kit serve --restart` note (named constant), never executed; dual-rack is a printed one-line note, never acted on | Intake assumptions #7 and #10 + area 4 make both print-only (Constitution III — never reimplement run-kit logic; don't risk deleting a good keg) | S:70 R:85 A:70 D:70 |
| 9 | Confident | `shll changelog rk@...` / `shll changelog rk` gains the alias for free because `parseChangelogSpecs` routes names through `resolveTargets`; no bespoke changelog code | Intake area 2 makes it conditional on the shared helper being reused — it is (confirmed by reading `changelog.go:226`); so the alias lands without extra work | S:75 R:85 A:85 D:80 |
| 10 | Confident | Digest before-version comes from the legacy `rk` keg (captured in `probeTool`) and after-version re-queries `sahil87/tap/run-kit` (`t.Formula`, already renamed) | Intake area 4 "Digest" bullet specifies exactly this; `t.Formula` is the renamed formula post-T002, so the existing re-query path already reads the new formula | S:80 R:80 A:85 D:80 |

10 assumptions (3 certain, 7 confident, 0 tentative).
