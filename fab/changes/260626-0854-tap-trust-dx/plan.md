# Plan: Tap-trust DX for Homebrew 6.0

**Change**: 260626-0854-tap-trust-dx
**Intake**: `intake.md`

## Requirements

<!-- Derived from intake.md (5 change areas + non-goals). RFC-2119 statements with
     stable R# IDs and GIVEN/WHEN/THEN scenarios. -->

### `shll install`: per-formula trust by default

#### R1: Trust each formula before installing it
`shll install` SHALL, before running `brew install sahil87/tap/<formula>` for each missing roster tool, establish per-formula Homebrew trust by running `brew trust --formula sahil87/tap/<formula>` for that formula — UNLESS the `--no-trust` flag is given. Trust granularity MUST be per-formula (`--formula`), NEVER whole-tap (`--tap`).

- **GIVEN** brew is present and supports `brew trust`, and `hop` is missing
- **WHEN** `shll install` (no `--no-trust`) runs
- **THEN** `brew trust --formula sahil87/tap/hop` runs before `brew install sahil87/tap/hop`
- **AND** the trust call is per-formula, not `brew trust --tap sahil87/tap`

#### R2: `--no-trust` opts out of the trust step
`shll install` SHALL accept a `--no-trust` boolean flag that skips the per-formula trust step entirely; the install attempts proceed unchanged.

- **GIVEN** brew is present
- **WHEN** `shll install --no-trust` runs with missing tools
- **THEN** NO `brew trust` invocation is recorded
- **AND** `brew install sahil87/tap/<formula>` still runs for each missing tool

#### R3: Trust degrades gracefully (Constitution V)
WHEN `brew trust` is unavailable (brew absent, or too old to ship the `trust` subcommand) OR the per-formula trust ceremony fails (non-zero exit / transport error), `shll install` SHALL warn to stderr and CONTINUE to the install attempt rather than aborting. The `brewTrustAvailable` capability probe gates the trust step; a failed `brewTrustFormula` MUST NOT set the run's failure flag on its own.

- **GIVEN** brew is present but `brew trust` is unrecognized (older brew)
- **WHEN** `shll install` runs
- **THEN** no `brew trust` ceremony runs, install proceeds, exit reflects only install outcomes
- **AND GIVEN** `brew trust --formula …` exits non-zero for one tool
- **THEN** a warning is written to stderr and `brew install` for that tool is still attempted

#### R4: `brewTrustFormula` helper routes through `internal/proc` (Constitution I)
A new helper `brewTrustFormula(ctx, formula)` in `brew.go` SHALL run `brew trust --formula <formula>` via `internal/proc` (foregrounded so the user sees brew's own `Trusted formula:` / `Already trusted formula:` output) and return `(int, error)`. `brewTrustAvailable` MUST be reused as the capability gate (not reimplemented).

- **GIVEN** the trust step runs
- **WHEN** `brewTrustFormula(ctx, "sahil87/tap/hop")` is invoked
- **THEN** it routes through `proc.RunForeground` with argv `["brew","trust","--formula","sahil87/tap/hop"]`

### `shll shell-setup`: remove `--trust-tap`

#### R5: `--trust-tap` flag and its whole-tap ceremony are removed entirely
`shll shell-setup` SHALL NOT expose a `--trust-tap` flag. The `export HOMEBREW_REQUIRE_TAP_TRUST=1` managed line, its merge logic (`exportTrustLine`, the export branch of `wantLines`/`blockMatch.hasExport`), the `ensureTrustFunc` seam threaded through `runShellSetup`/`runShellSetupDefault`/`runShellSetupPrint`, and the now-unused whole-tap ceremony in `brew.go` (`ensureTapTrust`, `brewTrustTap`, `trustHatchHint`, and `tapName` if unused elsewhere) SHALL be removed. `shell-setup` reverts to maintaining only the `eval "$(shll shell-init <shell>)"` line.

- **GIVEN** the rebuilt binary
- **WHEN** `shll shell-setup --trust-tap zsh` is run
- **THEN** cobra reports `--trust-tap` as an unknown flag
- **AND** `shll shell-setup zsh` writes only the eval line in the sentinel block

#### R6: `shell_setup.go` stays subprocess-free; `TestNoProcImports` strengthens
`shell_setup.go` SHALL import neither `internal/proc` nor `os/exec` after the `ensureTrustFunc` seam is removed (pure file I/O). `TestNoProcImports` MUST continue to pass (and the file becomes strictly file-I/O-only — the seam that bridged to `brew.go` is gone).

- **GIVEN** the rewritten `shell_setup.go`
- **WHEN** `TestNoProcImports` runs
- **THEN** it passes — the source contains neither `internal/proc` nor `"os/exec"`

#### R7: Stale export line is actively stripped on the next run (migration)
WHEN an existing shll block contains a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line, the next `shll shell-setup` run SHALL rewrite the block to drop that line (active cleanup), leaving only the eval line. A plain re-run against a block already containing only the eval line stays a byte-identical no-op.

- **GIVEN** an rc file whose shll block carries both the export and eval lines
- **WHEN** `shll shell-setup zsh` runs
- **THEN** the block is rewritten to contain only the eval line; the export line is gone
- **AND** the surrounding rc content is preserved

### Remove the 38a6 Linux workaround

#### R8: `brewEnv()` injection and its `osGoos` gate are removed
The `brewEnv()` helper, the `noRequireTapTrustEnv` constant, and the `osGoos` cross-platform gate used only by `brewEnv` SHALL be removed from `brew.go`. The `brew install` (install.go), `brew update --quiet`, `brew upgrade` self, and `brew upgrade <formula>` fallback (update.go) call sites SHALL no longer inject any workaround env. `osGoos` itself is retained ONLY if still used by `shell_setup.go`'s `resolveRcFile` (darwin-vs-other bash branch).

- **GIVEN** the rebuilt binary on Linux
- **WHEN** `shll install` / `shll update` run `brew install`/`update`/`upgrade`
- **THEN** no `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` env entry is passed to any brew subprocess

#### R9: `proc` Env / RunForegroundEnv plumbing is stripped (nothing else uses it)
After R8, the `proc.Request.Env` field, the `RunForegroundEnv` function, and the env-append branch in `defaultRunner` SHALL be removed, reverting `proc` to its pre-38a6 `Run`/`RunForeground` surface — UNLESS a non-38a6 consumer remains (none does). Call sites revert to `proc.RunForeground`.

- **GIVEN** the 38a6 workaround is removed and no other caller constructs `Request.Env`
- **WHEN** the `proc` package is reviewed
- **THEN** `RunForegroundEnv`, `Request.Env`, and the env-append branch are gone; `proc.RunForeground` is the foreground transport everywhere

#### R10: Homebrew >= 6.0.4 floor is documented, not gated in code
The change SHALL NOT add any in-code Homebrew version gate. The Homebrew >= 6.0.4 requirement (and the `brew update` remedy for 6.0.0–6.0.3 Linux stragglers) SHALL be documented in the README only.

- **GIVEN** the codebase after this change
- **WHEN** grepping for a brew-version comparison guarding install/upgrade
- **THEN** none exists; the floor lives in README prose

### README + docs rewrite

#### R11: Bootstrap-first quick-start
README.md and `docs/site/install.md` SHALL present a bootstrap-first quick-start: `brew trust sahil87/tap/shll && brew install sahil87/tap/shll` (bootstrap), then `shll install`, then `shll shell-setup`, then `exec $SHELL`. The 3-line primary form (legible) MAY be accompanied by a chained one-liner variant.

- **GIVEN** the README quick-start
- **WHEN** a new user reads it
- **THEN** trust is established (bootstrap for shll, then per-formula via `shll install`) BEFORE any sandboxed install runs

#### R12: Troubleshooting rewrite + flag docs update
The obsolete "Tap sahil87/tap is allowed by default" *warning* troubleshooting SHALL be rewritten as a **hard block** (Homebrew 6.0 made trust mandatory by default): briefly explain the load-gate vs. sandboxed-install-gate distinction (why CLI-naming the formula is not enough) and why the bootstrap `brew trust` line is required (binary-download formula runs a sandboxed `def install`, not a bottle pour). All `--trust-tap` documentation SHALL be removed; `--no-trust` on `shll install` SHALL be documented; the Homebrew >= 6.0.4 requirement and `brew update` remedy SHALL be noted. `docs/site/workflows.md` references to `--trust-tap` SHALL be updated.

- **GIVEN** the rewritten docs
- **WHEN** grepping README.md / docs/site/*.md for `--trust-tap`
- **THEN** no `--trust-tap` usage remains; `--no-trust` and the bootstrap `brew trust` line are documented

### `shll doctor`: read-only "formula trusted?" check

#### R13: Per-installed-tool trust sub-check (WARN on installed-but-untrusted)
`shll doctor` SHALL add a read-only per-tool sub-check: for each **installed** roster tool, query trust state via `brew trust --json=v1` and WARN if the tool's formula is not trusted. A formula counts as trusted when its qualified name (`sahil87/tap/<formula>`) appears in the JSON `formulae` array OR its tap (`sahil87/tap`) appears in the `taps` array (tap- or formula-level trust both count). The trust WARN MUST fold into the existing worst-check-wins OK/WARN/FAIL marker (same tier as "installed but unwired"). It MUST NEVER read `~/.homebrew/trust.json` directly (Constitution III). The `shll` self row and not-installed tools (already FAIL on the binary check) are unaffected.

- **GIVEN** brew present, `brew trust` available, `hop` installed but not trusted
- **WHEN** `shll doctor` runs
- **THEN** hop's line is WARN with suggestion `formula not trusted — run 'shll install' (or 'brew trust --formula sahil87/tap/hop'); future upgrades will fail without it`
- **AND** a binary FAIL still dominates the trust WARN (worst-check-wins)
- **AND** the `shll` self row stays OK (no trust check)

#### R14: Trust sub-check degrades gracefully and preserves the exit/JSON contract
WHEN brew is absent OR too old to ship `brew trust` (pre-6.0), the trust sub-check SHALL be skipped silently — doctor MUST NEVER WARN on a trust state it cannot determine. doctor SHALL stay strictly read-only and keep its any-FAIL→exit-1 / `--json` contracts. The `brew trust --json=v1` query SHOULD run once per `doctor` invocation (the trust set is a single brew-wide fact), reusing `brewTrustAvailable` as the gate.

- **GIVEN** brew too old to ship `brew trust`
- **WHEN** `shll doctor` runs with all roster tools installed and wired
- **THEN** no trust WARN appears and the run exits 0
- **AND** `--json` emits the same shape (no new always-present field is required, but installed-untrusted tools render WARN consistently in text and JSON)

### Non-Goals

- **Shipping real `bottle do` bottles from CI.** Out of scope — a release-infra change across all tap repos; eliminating the bootstrap step is a separate backlog item. (intake Assumption 8)
- **`shll update` mutating trust.** `update` will NOT establish trust; trust mutation stays confined to `install`, and `doctor` surfaces any untrusted-but-installed tool. (intake Assumption 7)
- **In-code Homebrew version gate.** The 6.0.4 floor is documented, not enforced in code (R10).

### Design Decisions

1. **Per-formula trust in `install`, not whole-tap.** Homebrew recommends per-formula trust for third-party taps, and shll knows its exact roster — *Why*: trusts only what shll manages, at the recommended granularity — *Rejected*: `brew trust --tap` (the removed `--trust-tap` behavior; trusts more than needed).
2. **Strip the stale export line actively (R7).** The block is already rewritten on every install via the per-line merge, so dropping `exportTrustLine` from `wantLines` makes an existing export-bearing block get rewritten without it for free — *Why*: cleanliness, the intake's stated leaning, and the merge path already exists — *Rejected*: leaving the inert export line (harmless but untidy, and would require special-casing to preserve a line we no longer manage). [Open question 1 — resolved]
3. **Strip the proc Env/RunForegroundEnv plumbing (R9).** The 38a6 workaround was its only consumer — *Why*: backlog `[tkch]` prefers simplest; dead infra invites confusion — *Rejected*: keeping `Request.Env`/`RunForegroundEnv` as general-purpose infra (YAGNI; re-addable in one commit if a future need arises). [Open question 2 — resolved]
4. **Single `brew trust --json=v1` query per `doctor` run.** Trust is a brew-wide fact; query once and check each installed roster formula against the parsed set — *Why*: matches the single-rc-read pattern `resolveWiringFact` already uses, avoids N brew spawns — *Rejected*: per-tool `brew trust --json` calls (redundant).

## Tasks

### Phase 1: proc + brew helper foundation

- [x] T001 Strip the 38a6 Env plumbing from `src/internal/proc/proc.go`: remove `Request.Env`, `RunForegroundEnv`, and the `if len(req.Env) > 0 { cmd.Env = append(os.Environ(), req.Env...) }` branch in `defaultRunner` (drop the now-unused `os` import only if nothing else needs it — `os.Stdin/Stdout/Stderr` still use it, so keep). Revert to `Run`/`RunForeground` only. <!-- R9 -->
- [x] T002 Update `src/internal/proc/proc_test.go`: remove `TestRunForegroundEnv_RecordsEnvAndTransport`, `TestRunForegroundEnv_TransportError`, `TestRunForeground_NoEnv` (Env-specific), and the `TestDefaultRunner_EnvAppendedToParent` env-append assertions; keep `TestDefaultRunner_RealBinary` and the core Run/RunForeground/ErrNotFound tests. <!-- R9 -->

### Phase 2: `brew.go` — add per-formula trust, remove whole-tap ceremony + workaround

- [x] T003 In `src/cmd/shll/brew.go`: add `brewTrustFormula(ctx, formula string) (int, error)` running `proc.RunForeground(ctx, brewBinary, "trust", "--formula", formula)`. Keep `brewTrustAvailable`. Remove `brewTrustTap`, `ensureTapTrust`, `trustHatchHint`, `brewEnv`, and `noRequireTapTrustEnv`. Add `brewTrustList(ctx) ([]string trustedTaps, []string trustedFormulae, ok bool)` that runs `brew trust --json=v1` via `proc.Run`, JSON-decodes `{taps,formulae}`, and reports ok=false on any error (degradation). <!-- R4 R3 R8 R13 -->
- [x] T004 Remove the `osGoos` package var from `src/cmd/shll/brew.go` if it lived there (it lives in `shell_setup.go`); confirm `osGoos` stays in `shell_setup.go` for `resolveRcFile`. Remove `brewEnv`'s `osGoos` usage. <!-- R8 -->
- [x] T005 Update `src/cmd/shll/brew_test.go`: keep `TestBrewTrustAvailable_*`; remove `TestBrewTrustTap_*`, `TestEnsureTapTrust_*`, `TestBrewEnv_*`. Add `TestBrewTrustFormula_BuildsFormulaArg` (asserts argv `brew trust --formula <formula>`, per-formula not `--tap`), `TestBrewTrustFormula_SurfacesNonZeroExit`, `TestBrewTrustFormula_SurfacesError`, and `TestBrewTrustList_*` (parses taps+formulae, degrades on error/absence). <!-- R4 R13 -->

### Phase 3: `shll install` — trust-by-default + `--no-trust`, remove workaround

- [x] T006 In `src/cmd/shll/install.go`: add a `--no-trust` bool flag (named constant `noTrustFlag`/usage) on `newInstallCmd`, thread `noTrust bool` into `runInstall`. Before each missing tool's install loop iteration (or as a pre-install trust pass over `missing`), when `!noTrust` and `brewTrustAvailable(ctx)`, run `brewTrustFormula(ctx, t.Formula)`; on failure write a warning to stderr and continue (do NOT set `anyFailed`). Replace `proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "install", t.Formula)` with `proc.RunForeground(ctx, brewBinary, "install", t.Formula)`. <!-- R1 R2 R3 R8 -->
- [x] T007 Update `src/cmd/shll/install_test.go`: remove `TestInstall_BrewInstallCarriesWorkaroundEnvOnLinux`, `TestInstall_BrewInstallNoWorkaroundEnvOnDarwin`, and the `noneInstalledRunner` helper if now unused. Add tests: `TestInstall_TrustsEachFormulaBeforeInstall` (per-formula trust precedes install, per-formula not `--tap`), `TestInstall_NoTrustSkipsTrustStep` (`--no-trust` → no `brew trust` calls), `TestInstall_TrustUnavailableSkipsGracefully` (older brew → no trust calls, install proceeds, exit 0), `TestInstall_TrustFailureContinues` (trust non-zero → warning, install still attempted, exit reflects install only). Update any golden/`findCall`/`envContains` usages that depended on the removed env. <!-- R1 R2 R3 -->

### Phase 4: `shll update` — remove workaround

- [x] T008 In `src/cmd/shll/update.go`: replace the three `proc.RunForegroundEnv(ctx, brewEnv(), …)` brew calls (metadata refresh, shll self-upgrade) and the `upgradeTool` `brew upgrade <formula>` fallback with plain `proc.RunForeground`. Remove the `var env []string; if argv[0] == brewBinary { env = brewEnv() }` block in `upgradeTool`. <!-- R8 -->
- [x] T009 Update `src/cmd/shll/update_test.go`: remove `TestUpdate_LiveBrewCallsCarryWorkaroundEnvOnLinux`, `TestUpdate_LiveBrewCallsNoWorkaroundEnvOnDarwin`, `TestUpdate_BrewUpgradeFallbackCarriesWorkaroundEnvOnLinux`, `TestUpdate_DelegationNeverCarriesEnvOnLinux`, and the `allBrewInstalledRunner` helper if unused. Keep `findCall`/`envContains`/`installedOnly`/`setOsGoos` if still referenced by other tests (else remove). <!-- R8 -->

### Phase 5: `shll shell-setup` — remove `--trust-tap`, pure rc-wiring

- [x] T010 Rewrite `src/cmd/shll/shell_setup.go` to pure rc-wiring: drop the `trustTap` flag from `newShellSetupCmd`, the `ensureTrustFunc` type + the `ensureTrust` parameter on `runShellSetup`/`runShellSetupDefault`/`runShellSetupPrint`, the `exportTrustLine` constant, the export branch of `wantLines` (drop `wantExport`/`existing.hasExport`), and `blockMatch.hasExport` + its detection in `findBlockWith`. The cobra `RunE` no longer passes `ensureTapTrust`. `wantLines` returns just `[evalLine(shell)]`; `--print` prints the eval-only block. Keep legacy-sentinel migration and the partial-block refusal. The existing rewrite/merge path strips a stale export line for free (R7): a block whose only managed line we now recognize is the eval line, so a rewrite that drops the no-longer-recognized export line lands the merged block as eval-only. Keep `osGoos` (used by `resolveRcFile`). Update the Long help to remove `--trust-tap`. <!-- R5 R6 R7 -->
- [x] T011 Update `src/cmd/shll/shell_setup_test.go`: remove all `--trust-tap`/`ensureTapTrust`/`installTrustSuccessRunner`/combined-block tests (`TestTrustTap_*`, `TestBuildBlock_CombinedTrust`, `TestPrintTrustTap_*`, `TestMigration_*OnTrustTap`, `tCombinedZsh`, the `installTrustSuccessRunner` helper). Add `TestMigration_StripsStaleExportLine` (a block with export+eval rewrites to eval-only, surrounding content preserved). Keep + strengthen `TestNoProcImports`. Re-point migration tests that used `--trust-tap` to the plain-install path. Adjust `TestMigration_BothSentinelsPresent*` to no longer expect the export line. Keep `setOsGoos`/`osGoos` usage for the bash-default tests. <!-- R5 R6 R7 -->

### Phase 6: `shll doctor` — read-only formula-trusted check

- [x] T012 In `src/cmd/shll/doctor.go`: extend the run to query trust ONCE via `brewTrustList(ctx)` gated by `brewTrustAvailable(ctx)` (skip silently when brew absent / trust unavailable). Thread a `trustFact` (trusted? per installed roster formula, plus an `available bool`) into `evaluateTool`. For an installed shell-init-or-not roster tool that passes binary checks: if trust is available and the formula is NOT trusted (neither its qualified name in `formulae` nor `sahil87/tap` in `taps`), set WARN with suggestion `suggestNotTrustedFmt` = `"formula not trusted — run 'shll install' (or 'brew trust --formula %s'); future upgrades will fail without it"`. Fold into worst-check-wins (binary FAIL dominates; trust WARN co-equal with unwired WARN — emit the trust warning when both apply, or keep wiring's existing precedence and document). The `shll` self row (`shllDoctorResult`) is unchanged. Add a `Trusted bool` JSON field to `doctorResult` only if needed for parity; otherwise keep schema and reflect via Status/Suggestion. Keep read-only + exit contracts. <!-- R13 R14 -->
- [x] T013 Update `src/cmd/shll/doctor_test.go`: extend `doctorFake` to answer `brew trust --json=v1` (and `brew trust --help` for the availability gate) with a configurable trusted-set; default to "all trusted" so existing goldens stay OK. Add `TestDoctor_InstalledUntrustedWarns` (installed tool absent from the trust set → WARN with the not-trusted suggestion, exit 0), `TestDoctor_TapLevelTrustCounts` (tap in `taps` → all formulae count trusted, no WARN), `TestDoctor_TrustUnavailableSkipsCheck` (older brew → no trust WARN even when untrusted, exit 0), `TestDoctor_BinaryFailDominatesTrust` (missing binary → FAIL not trust-WARN), `TestDoctor_ShllRowNoTrustCheck` (shll self row stays OK). Ensure existing all-OK goldens still pass with the default-trusted fake. <!-- R13 R14 -->

### Phase 7: docs

- [x] T014 Rewrite `README.md`: bootstrap-first quick-start (`brew trust sahil87/tap/shll && brew install sahil87/tap/shll`, `shll install`, `shll shell-setup`, `exec $SHELL`) + chained one-liner variant; rewrite the "allowed by default" warning troubleshooting as a hard-block explanation (load-gate vs sandboxed-install-gate; why the bootstrap `brew trust` is required); remove all `--trust-tap` docs; document `--no-trust` on `shll install`; note Homebrew >= 6.0.4 + the `brew update` remedy. Update the `shll install` / `shll shell-setup` command sections and the composition table. <!-- R11 R12 R10 R1 R2 R13 -->
- [x] T015 Rewrite `docs/site/install.md` and update `docs/site/workflows.md`: same bootstrap-first ordering, hard-block troubleshooting, `--trust-tap` removal, `--no-trust` documentation, doctor trust sub-check mention, Homebrew >= 6.0.4 note. <!-- R11 R12 R10 -->

### Phase 8: build + verify

- [x] T016 Run `cd src && go build ./... && go test ./...` (and `just build` from repo root if present); fix any compile/test failures so the whole module is green. <!-- R1 R2 R3 R4 R5 R6 R7 R8 R9 R13 R14 -->

## Execution Order

- T001→T002 (proc) before T003 (brew helper uses only `proc.RunForeground`/`proc.Run`).
- T003→T004 (brew.go) before T006/T008/T012 (install/update/doctor call the new/changed helpers).
- T006/T007, T008/T009, T010/T011, T012/T013 are per-command pairs (impl then test).
- T014/T015 (docs) independent of code; can run anytime.
- T016 last (full build + test).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll install` runs `brew trust --formula sahil87/tap/<formula>` (per-formula, not `--tap`) before each missing tool's `brew install`, by default. (install.go:170-185 interleaves `brewTrustFormula(ctx, t.Formula)` before `proc.RunForeground(... "install", t.Formula)`; `TestInstall_TrustsEachFormulaBeforeInstall` asserts ordering + never `--tap`.)
- [x] A-002 R2: `shll install --no-trust` records no `brew trust` invocation and still installs missing tools. (`noTrustFlag` const + `trustEnabled := !noTrust && brewTrustAvailable(ctx)`; `TestInstall_NoTrustSkipsTrustStep` passes.)
- [x] A-003 R3: When `brew trust` is unavailable or a per-formula trust fails, `shll install` warns and still attempts the install; a trust failure alone does not flip the run to exit 1. (Trust failure writes to stderr, never sets `anyFailed`; `TestInstall_TrustUnavailableSkipsGracefully` + `TestInstall_TrustFailureContinues` pass.)
- [x] A-004 R4: `brewTrustFormula` routes `brew trust --formula <formula>` through `internal/proc`; `brewTrustAvailable` is reused as the gate. (brew.go:`brewTrustFormula` → `proc.RunForeground(ctx, brewBinary, "trust", "--formula", formula)`; gate reused in install.go and doctor.go.)
- [x] A-005 R5: `--trust-tap` is gone (unknown flag); `exportTrustLine`, `ensureTrustFunc`, `ensureTapTrust`, `brewTrustTap`, `trustHatchHint` are removed; `shell-setup` writes only the eval line. (Grep confirms no code references; `TestTrustTapFlagRemoved` asserts unknown flag + Long help clean.)
- [x] A-006 R6: `shell_setup.go` imports neither `internal/proc` nor `os/exec`; `TestNoProcImports` passes. (Import block has only bytes/context/errors/fmt/io/os/path/filepath/runtime/strings + cobra; `TestNoProcImports` strengthened with an `ensureTrustFunc`-absence guard.)
- [x] A-007 R7: An existing block carrying a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line is rewritten to eval-only on the next `shll shell-setup` run, surrounding content preserved. (`findBlockWith` no longer recognizes the export line; `wantLines` returns eval-only; `TestMigration_StripsStaleExportLine` passes.)
- [x] A-008 R8: No brew subprocess carries `HOMEBREW_NO_REQUIRE_TAP_TRUST=1`; `brewEnv`/`noRequireTapTrustEnv` are removed. (Removed from brew.go; install.go/update.go call sites now use plain `proc.RunForeground`; grep clean.)
- [x] A-009 R9: `proc.Request.Env`, `RunForegroundEnv`, and the env-append branch are removed; call sites use `proc.RunForeground`. (proc.go Request struct = Name/Args/Transport/Dir only; `RunForegroundEnv` + env-append branch gone; no `.Env` field access in src.)
- [x] A-010 R10: No in-code Homebrew version gate exists; the 6.0.4 floor lives in README prose. (Only "6.0.4" mentions in code are comments; floor documented in README.md:33,230 + install.md:7,149.)
- [x] A-011 R11: README + `docs/site/install.md` quick-start is bootstrap-first (`brew trust …shll && brew install …shll`, `shll install`, `shll shell-setup`, `exec $SHELL`). (README.md:23-26 + install.md:14 lead with the bootstrap line; chained one-liner at README.md:31.)
- [x] A-012 R12: No `--trust-tap` docs remain in README/docs; `--no-trust` and the bootstrap `brew trust` line are documented; the warning troubleshooting is rewritten as a hard block; `workflows.md` updated. (Remaining `--trust-tap` mentions are migration/troubleshooting prose only; hard-block section at README.md:210 / install.md:129; `--no-trust` documented in all three.)
- [x] A-013 R13: `shll doctor` WARNs on an installed-but-untrusted roster formula via `brew trust --json=v1` (tap- or formula-level trust counts), never reads `trust.json` directly, folds into worst-check-wins, leaves the `shll` self row OK. (`resolveTrustFact` + `trustFact.trusts` (tap OR formula), `brewTrustList` JSON-decodes; shll row built separately; `TestDoctor_InstalledUntrustedWarns`/`TestDoctor_TapLevelTrustCounts`/`TestDoctor_ShllRowNoTrustCheck` pass.)
- [x] A-014 R14: When brew/`brew trust` is absent, doctor skips the trust sub-check silently (no WARN), stays read-only, and keeps any-FAIL→exit-1 / `--json`. (`resolveTrustFact` gated by `brewTrustAvailable`; degrades to `available:false` on JSON failure too; `TestDoctor_TrustUnavailableSkipsCheck` passes; read-only preserved — only `brew trust --json=v1` and `--help` added.)

### Behavioral Correctness

- [x] A-015 R5: `shll shell-setup zsh` on a fresh rc writes exactly the eval-only sentinel block (no export line) — `TestPlainInstall_NewSentinelEvalOnly` still passes. (Test present and green.)
- [x] A-016 R7: A plain re-run against an eval-only block is a byte-identical no-op (idempotency preserved). (`TestMigration_StaleExportThenReRunIsNoop` asserts byte-identical second run + "already installed" message.)
- [x] A-017 R13: A binary FAIL dominates the trust WARN (worst-check-wins) — `TestDoctor_BinaryFailDominatesTrust`. (Version checks `return markerFail` before the trust block in `evaluateTool`; test asserts FAIL line carries install — not trust — suggestion.)

### Removal Verification

- [x] A-018 R5/R8/R9: No references remain to `--trust-tap`, `exportTrustLine`, `ensureTapTrust`, `brewTrustTap`, `trustHatchHint`, `brewEnv`, `noRequireTapTrustEnv`, `RunForegroundEnv`, or `Request.Env` anywhere in `src/` (grep clean); no dead `osGoos` usage beyond `resolveRcFile`. (Grep over `src/**/*.go`: all remaining `--trust-tap`/`ensureTrustFunc` hits are comments or removal-assertion tests; `osGoos` used only in `shell_setup.go:138` resolveRcFile darwin branch + its test helper.)
- [x] A-019 R1: `TestInstall_TrustsEachFormulaBeforeInstall` exercises the trust-then-install ordering. (Present; asserts per-tool trustIdx < installIdx and no `--tap`.)
- [x] A-020 R13: `TestDoctor_InstalledUntrustedWarns` + `TestDoctor_TapLevelTrustCounts` + `TestDoctor_TrustUnavailableSkipsCheck` exercise the new sub-check. (All three present and green, plus `TestDoctor_UntrustedJSONWarn` for --json parity.)

### Edge Cases & Error Handling

- [x] A-021 R3: Older-brew (no `brew trust`) install path: trust step skipped silently, install proceeds (`TestInstall_TrustUnavailableSkipsGracefully`). (Present and green.)
- [x] A-022 R14: Older-brew doctor path: trust sub-check skipped, no WARN, exit 0 (`TestDoctor_TrustUnavailableSkipsCheck`). (Present and green.)

### Code Quality

- [x] A-023 Pattern consistency: New code follows naming and structural patterns of surrounding code (named constants for the `--no-trust` flag, the not-trusted suggestion format string, and the trust argv; helpers in `brew.go`). (`noTrustFlag`/`noTrustFlagUsage`/`suggestNotTrustedFmt` constants; trust helpers in brew.go mirror `brewTrustAvailable`; `resolveTrustFact` mirrors `resolveWiringFact`.)
- [x] A-024 No unnecessary duplication: `brewTrustAvailable` reused as the gate in both install and doctor; the trust-JSON parse single-sourced in `brewTrustList`; no `brew trust --json` parsing duplicated across files. (Single `brewTrustList` JSON decode; doctor's `resolveTrustFact` wraps it; install gates on the same `brewTrustAvailable`.)
- [x] A-025 Subprocess routing (Constitution I): every new subprocess (`brew trust --formula`, `brew trust --json=v1`) routes through `internal/proc`; no raw `os/exec` in command code; `shell_setup.go` stays subprocess-free. (`brewTrustFormula`→`proc.RunForeground`; `brewTrustList`→`proc.Run`; no `os/exec` in cmd/shll; `TestNoProcImports` enforces.)
- [x] A-026 Wrap-don't-reinvent (Constitution III/IV): trust state is read via `brew trust --json=v1`, never by parsing `~/.homebrew/trust.json`; no regex over brew output (JSON decode used). (`brewTrustList` uses `encoding/json`; no `trust.json` path read anywhere in src.)
- [x] A-027 Graceful degradation (Constitution V): brew/`brew trust` absence degrades (install proceeds; doctor skips) rather than erroring. (install `trustEnabled` gate; doctor `resolveTrustFact` returns `available:false`; both covered by tests.)
- [x] A-028 Magic strings: formula/argv/suggestion strings are named constants per code-quality.md. (`brewBinary`, `noTrustFlag`, `noTrustFlagUsage`, `suggestNotTrustedFmt`, `tapName`, `formulaPrefix` all named; argv literals `"trust"/"--formula"/"--json=v1"` are brew's fixed subcommand grammar, consistent with the existing `"install"`/`"upgrade"` literals — see Should-fix note.)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

<!-- This change is predominantly REMOVAL (the 38a6 workaround, --trust-tap ceremony,
     and proc Env plumbing were already deleted during apply). The candidates below
     are code that STILL EXISTS and is now dead/redundant after those removals. -->

- `src/cmd/shll/shell_setup.go:183` `wantLines(_ blockMatch, shell string)` — the `blockMatch` parameter is now fully unused (the sole caller at line 345 passes `blockMatch{}`); could drop to `wantLines(shell string)`. Its single call `buildBlockBody(wantLines(blockMatch{}, shell))` is byte-for-byte equivalent to the existing `buildBlock(shell)` helper (line 190), so `wantLines` could be inlined/removed and `runShellSetupDefault` could compute `desired := buildBlock(shell)` directly. Not dead overall (it produces the eval line), just redundant indirection left behind by the removed export-line merge.
- `src/cmd/shll/shell_setup.go:255` `runShellSetup(ctx context.Context, …)` — the `ctx` parameter is now dead (`_ = ctx` at line 259); shell-setup performs no subprocess/ctx-scoped work after the ceremony seam was removed. Retained for signature stability (a defensible call, but the parameter could be dropped along with the `context` import if signature churn is acceptable).
- `src/cmd/shll/brew_test.go:375` `TestTapName_NoTrailingSlash` — a sanity guard retained from the deleted ceremony tests. Still valid (asserts `tapName == "sahil87/tap"`, which doctor's tap-level trust check and `brewTrustList` parse depend on), so KEEP; flagged only because it is a leftover from removed tests and could be folded into a `tools.go`-adjacent test if those constants ever move.

(No truly dead/zero-call-site production symbols remain: `brewTrustFormula`, `brewTrustList`, `resolveTrustFact`, `trustFact.trusts`, `noTrustFlag`, and `suggestNotTrustedFmt` all have live call sites, and `blockMatch.hasEval` is still read by `doctor.go`'s `resolveWiringFact`.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Strip the stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line actively on the next `shll shell-setup` run (drop `exportTrustLine`/the export branch from `wantLines`), rather than preserving it | Intake Open Question 1 explicitly leans "strip it for cleanliness since the block is already rewritten"; the per-line merge path already rewrites the block, so dropping the no-longer-recognized line is free; reversible (docs/code) | S:80 R:75 A:80 D:80 |
| 2 | Confident | Strip the `proc.Request.Env` / `RunForegroundEnv` / env-append plumbing (revert proc to pre-38a6 surface) since nothing else uses it | Intake Open Question 2 + backlog `[tkch]` both prefer simplest; grep confirms the 38a6 workaround was the sole consumer; one-commit-reversible if a future need arises | S:80 R:80 A:85 D:75 |
| 3 | Confident | Query trust state once per `doctor` run via `brew trust --json=v1`, checking each installed roster formula against the parsed `{taps,formulae}` (tap- OR formula-level trust counts) | Verified the live `brew trust --json=v1` shape on 6.0.4 (`{taps,formulae,casks,commands}`); matches the single-rc-read pattern `resolveWiringFact` uses; Constitution III (no direct trust.json read) | S:85 R:75 A:85 D:80 |
| 4 | Confident | Trust WARN is co-equal with the unwired WARN under worst-check-wins; when an installed tool is both untrusted and unwired, surface the trust warning (trust breaks upgrades — higher user impact) while a binary FAIL still dominates both | Intake says "same tier as installed-but-unwired" and worst-check-wins; both are WARN so exit is identical; the suggestion choice is a presentation detail, reversible | S:70 R:80 A:75 D:65 |
| 5 | Confident | `shll install` runs the per-formula trust as a step immediately before each tool's `brew install` (interleaved in the existing loop), not as a separate up-front pass | Keeps trust adjacent to the install it unblocks, mirrors the existing per-tool-header loop structure; idempotent either way; reversible | S:75 R:80 A:75 D:70 |
| 6 | Confident | Keep `doctorResult`'s JSON schema unchanged (no new `trusted` field); reflect untrusted state via `Status: WARN` + the not-trusted `Suggestion` | Intake says "shll self row unchanged" and never mandates a new field; doctor's text/JSON derive from one struct, so Status/Suggestion carry the signal consistently; adding a field is a larger, less-reversible schema change | S:70 R:70 A:75 D:65 |
| 7 | Certain | `--no-trust` is a bool flag on `shll install` (no new top-level command), trust-by-default otherwise | User explicitly chose "default-on, `--no-trust` opt-out" (intake Assumption 1); Constitution VII (flag, not command) | S:90 R:75 A:90 D:90 |

7 assumptions (1 certain, 6 confident, 0 tentative).
