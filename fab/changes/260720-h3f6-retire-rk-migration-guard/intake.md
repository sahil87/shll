# Intake: Retire the rk→run-kit Brew-Formula Migration Guard

**Change**: 260720-h3f6-retire-rk-migration-guard
**Created**: 2026-07-20

## Origin

Promptless dispatch (Create-Intake Procedure, `{questioning-mode} = promptless-defer`) from a synthesized user conversation. The user diagnosed the noise, traced it to the migration guard, and explicitly chose full retirement over silencing:

> Retire the rk→run-kit brew-formula migration guard — the migration window is closed.
>
> Every `shll update` run prints brew's `Warning: Formula sahil87/tap/rk was renamed to sahil87/tap/run-kit.` because shll probes the legacy formula `sahil87/tap/rk` on every run (`probeRunKitMigration` — even on fully migrated machines, where branch 1 probes it for dual-rack detection), brew warns whenever the old name is referenced (the rename mapping lives permanently in the tap's `formula_renames.json`), and `proc.Run` (TransportCapture) passes subprocess stderr straight through to the terminal. Retire the guard entirely rather than merely silencing the probe's stderr.

User-confirmed decisions (verbatim intent, one-shot): (1) remove `LegacyFormula` and ALL machinery keyed on it across update/install/doctor/uninstall, simplifying brew.go's leaf plumbing if that is a clean reduction; (2) KEEP `LegacyName` and every surface built on it (binary-alias/display concerns); (3) NO homebrew-tap change — `formula_renames.json` stays; (4) straggler migration path is documented in run-kit's README (already done, outside this change); (5) all affected tests updated to the new spec (Constitution Test Integrity).

## Why

1. **The pain point**: every `shll update` (and any command that runs the guard's probes — `install`, `doctor`, `uninstall`) references `sahil87/tap/rk`, and brew 6 prints `Warning: Formula sahil87/tap/rk was renamed to sahil87/tap/run-kit.` to stderr on every such reference. `internal/proc`'s `Run` (TransportCapture) deliberately streams subprocess stderr to the terminal (src/internal/proc/proc.go), so the warning lands in the user's face on every run — including fully migrated machines, where `probeRunKitMigration` branch 1 still probes the legacy formula purely for dual-rack detection (src/cmd/shll/update.go:564).

2. **If we don't fix it**: permanent noise on every update run for every user, forever — the rename mapping in the tap's `formula_renames.json` is permanent by design (removing it would strand pre-rename kegs with "formula not found"), so the warning never ages out on its own. The guard also carries ~350 lines of transitional classification machinery (three-state keg-leaf gating, dual-rack notes, brew-direct migration action) whose window has closed.

3. **Why this approach**: the user decided the migration window is closed. The `Tool.LegacyFormula` field was documented as transitional from day one ("retire once legacy kegs die out" — src/cmd/shll/tools.go:83–89). Full retirement removes both the warning and the dead machinery at the root.
   - **Rejected — silence only the probe's stderr** (switch the legacy probe to `proc.RunCaptured`): keeps dead machinery alive and still runs a pointless probe on every migrated machine; the user rejected this in favor of retirement.
   - **Rejected — remove the tap's rename mapping**: would strand a straggler's pre-rename keg with "formula not found" on `brew upgrade`/`brew uninstall sahil87/tap/rk`. The mapping stays (verified: the tap has only `run-kit.rb` plus the rename mapping).

## What Changes

All paths relative to repo root; all line references are pre-change HEAD.

### tools.go — remove the `LegacyFormula` roster field

- Delete the `LegacyFormula string` field from the `Tool` struct (src/cmd/shll/tools.go:83–89) and the `LegacyFormula: formulaPrefix + "rk"` value from run-kit's roster entry (src/cmd/shll/tools.go:165).
- KEEP `LegacyName string` (tools.go:75–82) and run-kit's `LegacyName: "rk"` — but update its doc comment: it is no longer "transitional … can be retired"; it is the retained binary-alias/display surface (the run-kit formula still installs `rk` as an interchangeable command alias).
- KEEP `legacyAliases = map[string]string{"rk": "run-kit"}` (tools.go:109) and the `resolveTargets` alias resolution + `printAliasNotices` — `shll install rk` / `shll update rk` / `shll uninstall rk` / `shll skill rk` / `shll changelog rk@…` keep working.

### update.go — remove the migration guard and action

- Delete `probeRunKitMigration` (update.go:550–593) and its dispatch from `probeTool` (update.go:530–532) — `probeTool` becomes the plain single-probe path for every tool.
- Delete the `needsMigration` and `dualRack` fields from `probeResult` (update.go:109–119) and every consumer:
  - `upgradeTool`'s migration branch (update.go:659–661) and dual-rack note emission (update.go:676–678).
  - `upgradeArgv`'s `needsMigration` parameter and branch (update.go:755–758) — signature shrinks to `upgradeArgv(t Tool, supportsSkipFlag bool)`; the dry-run preview call site (update.go:271) and install.go's preview call site follow.
- Delete `migrateRunKit` (update.go:708–742) and the migration-only note constants `migrationDaemonNoteFmt` and `migrationDualRackNoteFmt` (update.go:611–634).
- KEEP `relinkNoteFmt` and the delegation-path unlinked-keg self-heal (`brew link` + one retry on `proc.ErrNotFound`, update.go:664–673) — it is keyed on an already-migrated keg, not on `LegacyFormula`.
- Clean the now-stale comments: the subset-run comment about legacy-keg run-kit counting as installed (update.go:216–219), `beforeVersion`'s legacy-keg sentence (update.go:106–107), and the dry-run preview note about the migration argv (update.go:378–379).
- Resulting behavior: a never-migrated machine (legacy `rk` keg only, no `run-kit` keg) probes `sahil87/tap/run-kit` → not installed → `shll update` skips it gracefully (Constitution V); `shll update run-kit` errors named-but-not-installed, same as any absent tool.

### install.go — remove the migration classification path

- In `runInstall`'s missing-partition (install.go:170–187): delete the `t.LegacyFormula != ""` branch — every tool goes through the plain `isInstalled(ctx, t.Formula)` check. A legacy-only machine therefore classifies run-kit as missing → normal trust + `brew install sahil87/tap/run-kit` (this is the intended new behavior: install the new formula; the orphan `rk` keg is manual cleanup per run-kit's README).
- Delete the `migrate` field from `installTarget` (install.go:63–66), the dry-run migration preview row (install.go:212–214), and the whole `m.migrate` action branch incl. the trust-new-formula-first-then-migrate handling (install.go:251–285).

### doctor.go — remove migration findings

- Delete `resolveMigrationFacts` (doctor.go:311–321) and its call in `runDoctor` (doctor.go:166–171).
- Delete the `migration probeResult` parameter from `evaluateTool` (doctor.go:327) and its two branches: the pending-migration WARN (doctor.go:357–361) and the dual-rack WARN (doctor.go:379–383).
- Delete the suggestion constants `suggestPendingMigrationFmt` (doctor.go:65–68) and `suggestDualRackFmt` (doctor.go:69–78).
- KEEP doctor's use of `probeVersion`'s legacy-name fallback (a `LegacyName` surface — an old `rk`-only binary on PATH still reports a version honestly).
- Resulting behavior: a legacy-only machine's run-kit row is FAIL "not installed — run 'shll install'" (binary typically still on PATH via the legacy-name fallback → the row degrades per the ordinary trust/wiring checks instead of a migration WARN; exact row outcome follows from the existing non-migration logic, no new special case).

### uninstall.go — remove the legacy-keg sweep

- Delete `uninstallRunKit` (uninstall.go:462–484), `probeRunKitInstalled` (uninstall.go:500–512), the `runKit`/`runKitNewInstalled`/`runKitLegacyKeg` fields on `uninstallTarget`, the `t.LegacyFormula != ""` branch in the actionable-set build (uninstall.go:206–220), the `case a.runKit:` action branch (uninstall.go:318–322), and the legacy preview row in `previewRowsFor` (uninstall.go:407–419) — run-kit becomes a plain `probeInstalledVersion` + `uninstallOne(t.Name, t.Formula)` target.
- PRESERVE the run-kit daemon-stop hint (`runKitDaemonStopHintFmt`, keyed today on the `a.runKit` case at uninstall.go:319–322): re-key it on successful removal of the run-kit roster entry (mechanism decided at apply — e.g. match on the roster entry by name via a named constant; no magic strings per code-quality.md).
- The Long help's mention of the `rk` legacy alias stays (alias surface is kept).

### brew.go — collapse the leaf plumbing

- After the removals above, the keg LEAF-NAME return of `probeInstalledLeaf` (brew.go:164–170) and `parseBrewLeaf` (brew.go:227–240) lose their only load-bearing consumers (every migration classification). Collapse to a version-only probe: fold `probeInstalledLeaf` into `probeInstalledVersion` as THE sole `brew list --formula --versions` invocation (returning `installed, version`), delete `parseBrewLeaf`, and rewrite the leaf-centric doc comments (brew.go:141–163, 220–226). KEEP `parseBrewVersion` (multi-keg max logic) and the thin `isInstalled`/`installedVersion` wrappers unchanged.
- The single-brew-invocation design decision stays intact: one `brew list` call per formula powering all install/version facts, no duplicated brew reads.

### Tests — conform to the new spec (Constitution Test Integrity)

- `update_test.go` (~163 migration-related mentions), `install_test.go` (~46), `doctor_test.go` (~60), `uninstall_test.go` (~43), `brew_test.go` (leaf-parsing tests), `tools_test.go` (LegacyFormula roster assertions): delete tests of removed machinery; update tests that assert `sahil87/tap/rk` probes in fake-runner call logs; add/keep coverage that a legacy-only machine is treated as "run-kit not installed" on each surface. Tests asserting the KEPT `LegacyName` surfaces (`version_test.go` fallback probe, `agent_setup_test.go` vocabulary token, alias-resolution tests) stay.

### Explicitly out of scope

- `homebrew-tap` repo: `formula_renames.json` (`{"rk": "run-kit"}`) stays untouched.
- run-kit's README straggler note: already edited in that repo.
- `LegacyName` and every surface built on it: `shll install rk`-style target aliases (`legacyAliases`), version.go's `rk --version` ErrNotFound fallback (version.go:86–88), agent_setup.go's `run-kit/rk` vocabulary token (agent_setup.go:86–88), skill.go/changelog.go alias resolution.
- `docs/memory/cli/*` content updates happen at hydrate, not during apply.

### Expected user-visible outcome

`shll update` / `install` / `doctor` / `uninstall` never reference `sahil87/tap/rk`, so brew's rename warning disappears. A never-migrated machine (legacy `rk` keg only) is treated as "run-kit not installed" — `shll install run-kit` installs the new formula; orphan-keg cleanup is manual per the run-kit README (`brew uninstall sahil87/tap/rk`, then `brew install sahil87/tap/run-kit` if needed).

## Affected Memory

- `cli/update`: (modify) remove the "transitional rk→run-kit brew-direct migration guard (keg-leaf gate)" — the probe/digest description reflects the plain single-probe path; keep the `rk` legacy target alias.
- `cli/install`: (modify) remove the legacy-keg brew-direct migration routing (trust-then-migrate); missing run-kit is a plain bootstrap install.
- `cli/doctor`: (modify) remove the "run-kit migration pending/dual-rack" checks and their WARN suggestions; keep the legacy-name version-probe fallback note.
- `cli/uninstall`: (modify) remove the leaf-verified dual-name run-kit sweep; run-kit is a plain reverse-roster removal; note how the daemon-stop hint is now keyed.
- `cli/commands`: (modify) Roster description — `LegacyFormula` field gone; `LegacyName` retained as a binary-alias/display field.

## Impact

- **Code**: `src/cmd/shll/{tools,update,install,doctor,uninstall,brew}.go` — net deletion (~350 lines of guard machinery plus comments); no new subcommands, flags, or output surfaces added (Constitution VII untouched). `src/internal/proc` untouched.
- **Tests**: `src/cmd/shll/{update,install,doctor,uninstall,brew,tools}_test.go` — large deletions plus behavior-conformance updates.
- **Behavioral**: legacy-only machines lose automated migration (now README-documented manual path); migrated machines lose only the noise (no functional change beyond the disappearing dual-rack note).
- **External**: none — no tap change, no run-kit change, no standards surface change (CLI command set, help-dump contract, and README are unchanged in shape; `shll uninstall`/`doctor` Long help lose only migration sentences if any — check `docs/site/standards/` help-output rules if Long texts are edited, per Constitution Toolkit Standards).

## Open Questions

- None — all consequential decisions were made and confirmed by the user in the originating conversation (see Origin).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Retire the guard fully (remove `LegacyFormula` + all keyed machinery) instead of silencing the probe's stderr | Discussed — user explicitly rejected the RunCaptured-silencing alternative; migration window declared closed | S:95 R:70 A:95 D:95 |
| 2 | Certain | Keep `LegacyName` and every surface on it (target alias, version fallback, agent-setup vocabulary token) | Discussed — user-confirmed: binary-alias/display concerns, not formula migration; run-kit still installs `rk` as an alias | S:95 R:90 A:95 D:95 |
| 3 | Certain | No homebrew-tap change — `formula_renames.json` stays | Discussed — user-confirmed; removing it would strand pre-rename kegs; tap verified to hold only run-kit.rb + the mapping | S:95 R:60 A:90 D:95 |
| 4 | Certain | Tests conform to the new no-guard spec (delete/rewrite migration tests), per Constitution Test Integrity | Discussed — user-confirmed decision 5; constitution makes the direction non-negotiable | S:90 R:85 A:95 D:95 |
| 5 | Confident | Collapse brew.go's leaf plumbing: fold `probeInstalledLeaf` into a version-only `probeInstalledVersion`, delete `parseBrewLeaf`, keep the single-brew-invocation design | User authorized "if that is a clean reduction"; verified no non-migration consumer of the leaf return remains, so the reduction is clean; easily reversible if apply finds a snag | S:75 R:80 A:85 D:75 |
| 6 | Confident | Preserve the uninstall daemon-stop hint by re-keying it on the run-kit roster entry (exact mechanism — e.g. named-constant name match — decided at apply) | Hint is user-facing value unrelated to formula migration (it fires on any successful run-kit removal); codebase pattern (no magic strings) constrains the mechanism; trivially reversible | S:60 R:85 A:80 D:65 |
| 7 | Certain | Legacy-only machines degrade to the ordinary "not installed" handling per surface (update: graceful skip / named error; install: fresh `brew install run-kit`; doctor: ordinary non-migration row; uninstall: "not installed" skip) with no replacement hint text | Follows directly from user's "simply treated as run-kit not installed" outcome statement + Constitution V; per-surface wording falls out of existing code paths, nothing new invented | S:80 R:85 A:85 D:75 |
| 8 | Certain | Update retained fields' doc comments (`LegacyName` no longer "transitional — can be retired"; brew.go leaf-rationale comments rewritten) rather than leaving stale migration references | User framed LegacyName as a kept alias/display concern; stale comments would contradict the new spec; pure-comment change, fully reversible | S:70 R:95 A:90 D:80 |

8 assumptions (6 certain, 2 confident, 0 tentative, 0 unresolved).
