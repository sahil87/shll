# Plan: Add `shll check-updates` — the toolkit's single update-check surface

**Change**: 260720-puxw-check-updates-command
**Intake**: `intake.md`

## Requirements

### CLI: the `check-updates` subcommand

#### R1: New read-only top-level subcommand
`shll check-updates` SHALL be a new user-facing top-level subcommand (`cobra.NoArgs`, no positional args in v1) that reports, for shll itself plus every roster tool (shll first, then leaves-first `Roster` order), the installed version vs the latest available version. It MUST never write — no brew mutation, no self-upgrade, no side effects. It SHALL be registered in `newRootCmd()` (`src/cmd/shll/root.go`) and listed in `rootLong` (the user-facing surface becomes thirteen). Constitution VII justification (from the intake, carried into memory at hydrate): a machine primitive for an external consumer (run-kit's daemon) plus an internal consumer (`shll changelog`); check-only semantics cannot be a flag on `update` (whose contract is to perform writes), and whole-roster version resolution against the toolkit manifest is inherently the meta-tool's job.

- **GIVEN** a machine with some roster tools installed
- **WHEN** `shll check-updates` runs
- **THEN** one row per tool (shll first, then roster order) reports installed vs latest, and no write operation (brew upgrade/install, file write) is performed

#### R2: Backend flags, mutually exclusive, `--released` default
The command SHALL accept two mutually exclusive backend flags: `--released` (fetch `https://shll.ai/versions.json`) and `--github` (latest release tag per tool via the unauthenticated GitHub API). Running with no backend flag MUST behave as `--released`. Passing both flags MUST be a usage error: exit 2 via `errExitCode{code: usageExitCode}` with a stderr message naming both flags. The GitHub backend is deliberately named `--github`, not `--homebrew`.

- **GIVEN** `shll check-updates --released --github`
- **WHEN** the command runs
- **THEN** it exits 2 with a stderr diagnostic naming the mutually exclusive flags, before any network/brew access

#### R3: Installed anchors from brew; brew gate
Installed versions MUST come from brew reads via the existing probe patterns (`installedVersion` over `probeInstalledLeaf` in `brew.go`, routed through `internal/proc` — Constitution I). shll-self's installed anchor is its **brew-formula** version (`installedVersion(ctx, shllFormula)`), never the running binary's ldflags version (the `shll changelog` bare-sweep precedent). Brew absence MUST be gated exactly like changelog's no-range forms: `brewMissingHint` on stderr + `errSilent` (exit 1), checked before any backend fetch. Per-tool brew probes SHOULD run concurrently, results indexed by position (the `probeInstalled`/`resolveChangelog` pattern).

- **GIVEN** brew is not on PATH
- **WHEN** `shll check-updates` runs (either backend)
- **THEN** `brewMissingHint` is printed to stderr and the exit code is 1, with no backend fetch attempted

#### R4: `--released` backend — the manifest is the roster + policy authority
The `--released` backend SHALL perform exactly one HTTP GET of `https://shll.ai/versions.json` (schema 1: `{schema, generated_at, tools: {name: {latest, notify, formula}}}`) per invocation — no caching (Constitution II). `latest` and `notify` come from the manifest, looked up by tool name. A manifest fetch failure (transport error, timeout, non-200, decode failure) or an unsupported `schema` value MUST fail the whole check: stderr diagnostic + `errSilent` (exit 1) — `--released` has exactly one fetch, so its failure fails the check.

- **GIVEN** the manifest endpoint returns a non-200 status
- **WHEN** `shll check-updates --released` runs
- **THEN** a diagnostic goes to stderr, nothing is written to stdout, and the exit code is 1

#### R5: `--github` backend — delegated, concurrent, per-tool degradation
The `--github` backend SHALL resolve each tool's latest release tag via the resolver seam's delegation to `internal/changelog` (`LatestTag` — no duplicated GitHub fetch code), fanned out concurrently with order preserved by index (the `FetchAll`/`resolveChangelog` pattern). No notify policy exists in this backend. A per-tool fetch failure MUST degrade per-tool — the JSON row is omitted, the human row shows `unavailable` — and the run still exits 0 (the changelog degradation precedent, Constitution V).

- **GIVEN** one repo's releases fetch returns 404 while others succeed
- **WHEN** `shll check-updates --github` runs
- **THEN** the failed tool degrades (omitted from `--json` `tools[]`; `unavailable` in human output), the other tools render normally, and the exit code is 0

#### R6: `--json` machine contract
`--json` SHALL emit the agreed envelope to stdout: `{"schema": 1, "source": "released"|"github", "tools": [...]}` with per-tool rows `{name, formula, installed, latest, notify?, update_available, notable?}`. Rules:

- `source` names the backend that produced the data.
- **Unresolvable-row rule**: a row is emitted only when both `installed` and `latest` resolve. Not-installed, missing-from-manifest (`--released`), or fetch-failed (`--github`) tools are omitted from `tools[]` (absent row = never matches for consumers).
- `--github` rows MUST omit `notify` and `notable` entirely; `--released` rows MUST carry both — including an explicit `"notable": false` (so `notable` is a `*bool` with `omitempty`, not a plain bool).
- Encoding follows the `list`/`doctor` precedent: `json.Encoder`, `SetEscapeHTML(false)`, 2-space indent, trailing newline. An empty resolved set emits `"tools": []`, never `null`.
- Evolution rule (documented, external): consumers tolerate unknown fields; additions are additive-only.

- **GIVEN** run-kit installed at 3.8.1, manifest latest 3.8.2 with `notify: minor`
- **WHEN** `shll check-updates --json --released` runs
- **THEN** the run-kit row reads `{"name":"run-kit","formula":"run-kit","installed":"3.8.1","latest":"3.8.2","notify":"minor","update_available":true,"notable":false}`

#### R7: `update_available` and notify-threshold (`notable`) semantics
`update_available` SHALL be `installed < latest` via `changelog.CompareVer` semantics (normalize `v` prefix + brew `_N` revision suffix). `notable` SHALL be true iff an update is available AND the pending bump crosses the tool's notify threshold: `never` → never notable; `patch` → any pending bump notable; `minor` → notable iff a minor-or-higher component increases (a patch-only bump is not notable); a major bump crosses every non-`never` threshold. An unknown/future `notify` value MUST be treated as `minor` (forward-compat conservatism). The threshold computation lives in the resolver package (R9).

- **GIVEN** `notify: minor` and a 3.8.1 → 3.8.2 bump
- **WHEN** notability is computed
- **THEN** `update_available: true, notable: false` (patch-only bump does not cross the minor threshold)
- **AND** the same versions under `notify: patch` yield `notable: true`

#### R8: Human (non-`--json`) output
Human output SHALL be a column-aligned, self-labeling `tabwriter` table in the `shll version` style (shll first, roster order) — **no** `▸`/`==>` per-tool headers and no summary tail (the per-tool-output-separation spec excludes read-only self-labeling aggregations). The version transition uses the shared `arrow(color)` helper so `→` degrades to `->` on non-TTY/`NO_COLOR`. Statuses: `update available` (with ` (notable)` suffix on the released backend when notable), `up to date`, `not installed` (Constitution V reporting — nothing hidden from humans), `unavailable` (github per-tool fetch failure), `not in manifest` (released, name absent from manifest).

- **GIVEN** shll 0.1.5→0.1.6 notable, wt up to date at 0.1.3, idea not installed
- **WHEN** `shll check-updates` renders to a non-TTY stream
- **THEN** rows read `shll  0.1.5 -> 0.1.6  update available (notable)`, `wt  0.1.3  up to date`, `idea  not installed` — aligned, no ANSI, no per-tool headers

### Internal: the resolver seam (`src/internal/versions`)

#### R9: New `internal/versions` resolver package
A new package `src/internal/versions` SHALL provide "latest version per tool" with the two backends:

- Owns the manifest fetch: URL as a named constant (`https://shll.ai/versions.json`), per-request context timeout mirroring `internal/changelog`'s `requestTimeout`, schema-1 decode, and a typed unavailability error (`ErrUnavailable` sentinel, wrapped with detail). This package is the single surface that absorbs future `versions.json` schema evolution.
- Owns the notify-threshold computation (`Notable(notify, installed, latest)` per R7).
- The GitHub backend (`LatestGitHub(ctx, repo)`) is a thin delegation to `internal/changelog.LatestTag`, returning the release list too, so the single-GET contract is preserved for changelog's consumption (R10).
- Test seams mirror `internal/changelog`: package-level `manifestURL`/`httpClient` vars + an exported `SetTransportForTest(url, client) (restore func())`.
- HTTP stays in internal packages only: `cmd/shll` never imports `net/http` (`TestCmdShllNoNetHTTP` continues to enforce this).
- The bump-classification primitive is a small export on `internal/changelog` (`FirstDiffComponent(a, b) int` — index of the first dot-component where the versions differ numerically, -1 when equal) so version-parsing knowledge stays in changelog and `versions` owns only the policy mapping.

- **GIVEN** an httptest server serving a schema-1 manifest
- **WHEN** `versions.FetchManifest(ctx)` runs against the swapped seam
- **THEN** it returns the decoded tools map; a non-200/malformed/unsupported-schema response returns an error wrapping `ErrUnavailable`

#### R10: `shll changelog` consumes the resolver at the package seam
`shll changelog`'s no-range "installed → latest" anchor (`resolveOneSpec` in `src/cmd/shll/changelog.go`) SHALL consume `versions.LatestGitHub` instead of calling `changelog.LatestTag` directly. Its CLI surface, output, and default GitHub-notes behavior stay unchanged; the single-GET no-range contract MUST stay green (`TestChangelog_NoRangeSingleFetchPerRepo`).

- **GIVEN** the existing changelog test suite
- **WHEN** the seam swap lands
- **THEN** all changelog tests pass unchanged, including the single-GET assertion (GET count == 1 per repo)

### CLI: exit codes and invariants

#### R11: Exit-code semantics and guarded invariants
Following the toolkit `0/1/2` convention (`translateExit`):

| Condition | Exit |
|-----------|------|
| Check ran (regardless of pending updates — verdicts live in the output) | 0 |
| Check itself failed: `--released` manifest fetch/schema failure, brew missing | 1 (`errSilent` after a stderr diagnostic) |
| Usage error: both backend flags, unknown flag/arg | 2 |
| `--github` per-tool fetch failures | degrade per-tool, exit 0 |

No distinct exit code for "notable updates exist". Invariants that MUST stay green: `TestCmdShllNoNetHTTP`, `TestChangelog_NoRangeSingleFetchPerRepo`, the changelog output golden strings, `TestShllSelf_NotInRoster` (`len(Roster) == 6` — check-updates iterates shll-self + Roster, never adds shll to Roster).

- **GIVEN** updates are pending for several tools
- **WHEN** `shll check-updates` completes successfully
- **THEN** the exit code is 0 (verdicts are data, not exit codes)

### Docs: README, rootLong, skill bundle

#### R12: Documentation per the toolkit standards
Per the Standards check obligation (Constitution — Toolkit Standards, checked against `docs/site/standards/principles.md`, `help-dump.md`, `readme-extraction.md`):

- `rootLong` (`root.go`) SHALL list `shll check-updates` (thirteen user-facing subcommands). The hidden `help-dump` envelope picks the new subcommand up automatically (programmatic cobra walk — no producer change needed).
- `README.md` SHALL gain a `### shll check-updates` section under `## Commands` and a row in the "How composition works" table. Links/images rules of the readme-extraction standard are unaffected (no new images or relative links leaving the published set).
- `docs/site/skill.md` (shll's own agent bundle) SHALL gain a one-line capabilities-map entry, and `scripts/sync-standards.sh` MUST be re-run so the embedded copy stays byte-matched (`TestSkillEmbedMatchesCanonical`).
- Help text on the command SHALL be layered (short summary, flags, concrete examples) per principle №3.

- **GIVEN** the shipped change
- **WHEN** `shll --help` and `shll help-dump` run
- **THEN** `check-updates` appears in both, and the skill-bundle drift guard passes

### Non-Goals

- No `--released`/`--github` modes on `shll update` — update stays brew-driven; `check-updates` is check-only.
- No caching of any kind (Constitution II).
- No change to run-kit itself (parallel change in the run-kit repo).
- No positional tool args in v1 — whole-roster sweep only (`cobra.NoArgs`); subset targeting via `resolveTargets` is a compatible later addition.

### Design Decisions

#### Mutual exclusion enforced in the run seam, not cobra flag groups
**Decision**: `runCheckUpdates` receives both backend bools and returns `&errExitCode{code: usageExitCode, msg: …}` when both are set, rather than using `cobra.MarkFlagsMutuallyExclusive`.
**Why**: cobra's flag-group violation error is a plain `fmt.Errorf` that matches none of `cobraUsageErrorPrefixes`, so it would exit 1 — the intake pins exit 2. The explicit check follows the `shell-init`/`shell-setup` precedent (`errExitCode{code: 2}` for user-invocation errors) and keeps the case testable through the writer seam.
**Rejected**: `MarkFlagsMutuallyExclusive` + adding its message prefix to `cobraUsageErrorPrefixes` — couples exit-code policy to a cobra-internal message shape for no gain.
*Introduced by*: 260720-puxw-check-updates-command

#### `notable` as `*bool` with `omitempty`
**Decision**: the JSON row's `notable` field is `*bool `json:"notable,omitempty"`` (nil on `--github` rows, `&value` on `--released` rows); `notify` is `string `json:"notify,omitempty"``.
**Why**: the contract requires `"notable": false` to be emitted on released rows (the intake's worked example) while the key is omitted entirely on github rows — a plain `bool` + `omitempty` would wrongly drop `false` everywhere.
**Rejected**: two row struct types per backend (more code, same bytes); always emitting `notable` with an invented default on github rows (the intake explicitly chose honest omission).
*Introduced by*: 260720-puxw-check-updates-command

#### Bump classification exported from `internal/changelog`
**Decision**: add `changelog.FirstDiffComponent(a, b string) int` and have `versions.Notable` map policy over it.
**Why**: version-component parsing (`verComponent`, normalize rules) already lives in changelog; duplicating it in `versions` is the code-quality anti-pattern the intake's "small exports for the seam" line anticipates.
**Rejected**: re-implementing component parsing in `versions` (drift risk); exporting the raw `verComponent` (weaker, caller must split/normalize itself).
*Introduced by*: 260720-puxw-check-updates-command

## Tasks

### Phase 1: Setup — the resolver package

- [x] T001 Add `FirstDiffComponent(a, b string) int` to `src/internal/changelog/changelog.go` (index of first numerically-differing dot-component after normalize, -1 when equal) with table tests in `src/internal/changelog/changelog_test.go` <!-- R9 -->
- [x] T002 Create `src/internal/versions/versions.go`: manifest types (`Manifest`, `ManifestTool`), named constants (`manifestURLDefault = "https://shll.ai/versions.json"`, `manifestSchema = 1`, `requestTimeout`), seams (`manifestURL`/`httpClient` vars + `SetTransportForTest`), `ErrUnavailable`, `FetchManifest(ctx)`, `Notable(notify, installed, latest)` (policy constants `never`/`patch`/`minor`, unknown→minor), `LatestGitHub(ctx, repo)` delegating to `changelog.LatestTag` <!-- R9, R7 -->
- [x] T003 Create `src/internal/versions/versions_test.go`: httptest manifest happy path, non-200 → `ErrUnavailable`, malformed JSON, unsupported schema; `Notable` policy×bump table (never/patch/minor/unknown × patch/minor/major/none); `LatestGitHub` single-fetch delegation via `changelog.SetTransportForTest` <!-- R9, R7 -->

### Phase 2: Core Implementation — the command

- [x] T004 Create `src/cmd/shll/check_updates.go`: `newCheckUpdatesCmd()` (flags `--released`/`--github`/`--json`, `cobra.NoArgs`, layered Long help) + `runCheckUpdates(ctx, stdout, stderr, released, github, jsonOut)` seam — mutual-exclusion usage error (exit 2), brew gate (`brewMissingHint` + `errSilent`), target set (shll-self first via `shllFormula`, then Roster; leaf formula via `strings.TrimPrefix(Formula, formulaPrefix)`), released path (one `versions.FetchManifest`, failure → stderr + `errSilent`), github path (per-tool `versions.LatestGitHub`), concurrent per-row brew probes indexed by position, `update_available`/`notable` computation via `changelog.CompareVer` + `versions.Notable`, versions normalized via `changelog.NormalizeVer` <!-- R1 R2 R3 R4 R5 R7 -->
- [x] T005 In `check_updates.go`: JSON renderer `writeCheckUpdatesJSON` — envelope `{schema, source, tools}`, row struct with `notify string,omitempty` + `notable *bool,omitempty`, unresolvable-row omission, `json.Encoder` `SetEscapeHTML(false)` 2-space indent, empty set → `[]` <!-- R6 -->
- [x] T006 In `check_updates.go`: human renderer `writeCheckUpdatesTable` — `tabwriter` (version-style config), `arrow(color)` transition, status constants (`up to date`, `update available`, `update available (notable)`, `not installed` via `notInstalledLabel`, `unavailable`, `not in manifest`), no headers/tail <!-- R8 -->
- [x] T007 Register in `src/cmd/shll/root.go`: `newCheckUpdatesCmd()` in `AddCommand` (after `newUpdateCmd()`) + a `shll check-updates` line in `rootLong` (after the `shll update` line) <!-- R1 R12 -->
- [x] T008 Swap `src/cmd/shll/changelog.go` `resolveOneSpec` to call `versions.LatestGitHub(ctx, repo)` instead of `changelog.LatestTag(ctx, repo)`; run the changelog test suite to confirm surface + single-GET contract unchanged <!-- R10 -->

### Phase 3: Integration & Edge Cases — command tests

- [x] T009 Create `src/cmd/shll/check_updates_test.go`: released happy-path table (notable/plain/up-to-date/not-installed rows, shll-first roster order, ASCII degrade, stderr empty); `--json --released` contract assertions (field values incl. literal `"notable": false`, unresolved rows omitted, `SetEscapeHTML` behavior); `--github` JSON omits `notify`/`notable` keys + `source:"github"` (via the `changelogServer` helper); both-flags → `errExitCode{usageExitCode}`; manifest fetch failure → stderr + `errSilent`, empty stdout; github per-tool 404 degrades (human `unavailable`, JSON row omitted, nil error); brew missing → `brewMissingHint` + `errSilent`; released not-in-manifest row; root registration + rootLong mention <!-- R1 R2 R3 R4 R5 R6 R7 R8 R11 R12 -->
- [x] T010 Run the full `./...` test suite from `src/`; confirm the pinned invariants stay green (`TestCmdShllNoNetHTTP`, `TestChangelog_NoRangeSingleFetchPerRepo`, `TestShllSelf_NotInRoster`, changelog goldens, help-dump tests) and fix any regressions <!-- R11 -->

### Phase 4: Polish — docs

- [x] T011 Update `README.md`: `### shll check-updates` section under `## Commands` (between `shll update` and `shll changelog`) with usage examples, `--json` contract sketch, exit codes; add a `shll check-updates` row to the "How composition works" table <!-- R12 -->
- [x] T012 Update `docs/site/skill.md` capabilities map with a `shll check-updates` line (and the output-contracts `--json` mention), then run `scripts/sync-standards.sh` so the embedded copy stays byte-matched (`TestSkillEmbedMatchesCanonical`) <!-- R12 -->

## Execution Order

- T001 → T002 → T003 (resolver builds on the changelog export)
- T002 blocks T004 (command consumes the resolver); T004 blocks T005/T006/T007
- T008 needs T002; independent of T004–T007
- T009 needs T004–T007; T010 last in Phase 3
- T011/T012 are independent [P]-equivalent doc tasks

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll check-updates` exists as a registered read-only subcommand (shll-self first, roster order), listed in `rootLong`, performing no writes — `TestCheckUpdates_RegisteredInRoot`, `checkUpdateTargets()` prepends `shllSelf` then Roster; no brew mutation path
- [x] A-002 R2: `--released` is the default backend; `--github` selects GitHub; both together exit 2 via `errExitCode{usageExitCode}` — `TestCheckUpdates_BothBackendFlagsUsageError`
- [x] A-003 R3: installed anchors come from brew probes (`installedVersion`, shll-self via `shllFormula`); brew missing → `brewMissingHint` + exit 1 before any fetch — `TestCheckUpdates_BrewMissingHint` (manifest guard confirms gate precedes fetch)
- [x] A-004 R4: `--released` performs exactly one GET of the versions manifest; latest+notify come from it; fetch/schema failure fails the whole check (exit 1) — `TestCheckUpdates_ManifestFetchFailureExit1`, `TestCheckUpdates_UnsupportedSchemaFailsCheck`, `versions.FetchManifest`
- [x] A-005 R5: `--github` resolves latest tags via the resolver's delegation to `internal/changelog` with concurrent order-preserved fan-out — `resolveCheckUpdates` (goroutine-per-target, indexed by position), `versions.LatestGitHub`
- [x] A-006 R6: `--json` emits the exact envelope/row contract (schema 1, source, unresolvable-row omission, github rows omit notify/notable, released rows carry `"notable": false` explicitly, list/doctor encoder settings) — `TestCheckUpdates_JSONContractReleased`, `TestCheckUpdates_GithubJSONOmitsNotifyNotable`
- [x] A-007 R7: `update_available` and `notable` follow the CompareVer + threshold semantics (never/patch/minor, unknown→minor, major crosses all non-never) — `versions.Notable` + `TestNotable` table
- [x] A-008 R8: human output is a version-style aligned table with the documented statuses, arrow ASCII degrade, no per-tool headers, no summary tail — `writeCheckUpdatesTable`/`checkUpdateCells`, `TestCheckUpdates_ReleasedHappyPathTable`
- [x] A-009 R9: `internal/versions` owns manifest fetch (constants, timeout, seams, `SetTransportForTest`), `Notable`, and `LatestGitHub`; `cmd/shll` imports no `net/http` — `versions.go`, `TestCmdShllNoNetHTTP` green
- [x] A-010 R10: changelog's no-range anchor consumes `versions.LatestGitHub`; changelog CLI surface/output unchanged — `resolveOneSpec` swap, `TestChangelog_NoRangeSingleFetchPerRepo` + changelog goldens green
- [x] A-011 R12: README section + composition-table row, rootLong thirteen lines, skill-bundle line + re-synced embed — README `### shll check-updates` + composition row, `rootLong`, `docs/site/skill.md`/`src/cmd/shll/skill/skill.md` byte-matched (`TestSkillEmbedMatchesCanonical`)

### Behavioral Correctness

- [x] A-012 R7: a `notify: minor` patch-only bump reports `update_available: true, notable: false` (the intake's worked example) — `TestCheckUpdates_JSONContractReleased` (run-kit row), `TestNotable` (minor/patch case)
- [x] A-013 R11: a successful check with pending updates exits 0 (no verdict exit code) — `TestCheckUpdates_ReleasedHappyPathTable`/`JSONContractReleased` return nil error with pending bumps present

### Scenario Coverage

- [x] A-014 R6: JSON rows are omitted for not-installed / not-in-manifest / fetch-failed tools while human output still reports them (test-pinned) — `TestCheckUpdates_NotInManifestRow`, `TestCheckUpdates_GithubPerToolFailureDegrades`
- [x] A-015 R5: a `--github` per-tool 404 degrades that row (`unavailable` human note, JSON omission) and exits 0 (test-pinned) — `TestCheckUpdates_GithubPerToolFailureDegrades`
- [x] A-016 R4: manifest fetch failure writes a stderr diagnostic, emits no stdout, exits 1 (test-pinned) — `TestCheckUpdates_ManifestFetchFailureExit1`

### Edge Cases & Error Handling

- [x] A-017 R7: unknown/future `notify` values are treated as `minor` (test-pinned in the `Notable` table) — `TestNotable` (`"weird"`, `""` cases)
- [x] A-018 R6: an empty resolved set emits `"tools": []`, never `null` — `TestCheckUpdates_EmptyResolvedSetEmitsEmptyArray`; `make([]checkUpdateItem, 0, …)` guarantees non-nil
- [x] A-019 R2: the mutual-exclusion usage error fires before any network or brew access — `TestCheckUpdates_BothBackendFlagsUsageError` asserts zero recorded subprocess calls + manifest guard

### Code Quality

- [x] A-020 Pattern consistency: new code follows the `newXxxCmd()` + `runXxx(writer seam)` factory pattern, tabwriter/version table config, and the changelog seam idioms — verified against `changelog.go`/`list.go`/`version.go`
- [x] A-021 No unnecessary duplication: GitHub fetch, version compare/normalize, and component parsing are reused from `internal/changelog` — nothing re-implemented — `LatestGitHub` delegates to `LatestTag`; `Notable` maps over `CompareVer`/`FirstDiffComponent`
- [x] A-022 Named constants: manifest URL, schema, source names, status labels, flag names — no magic strings/numbers — all present (`manifestURLDefault`, `manifestSchema`, `sourceReleased`/`sourceGithub`, `checkStatus*`, `releasedFlag`/`githubFlag`)
- [x] A-023 No regex over brew output; no hardcoded brew paths; JSON decoded via `encoding/json` — installed via `installedVersion` (brew `--versions`), manifest via `json.Unmarshal`, no regex/paths in the diff

### Security

- [x] A-024 R3: every subprocess invocation routes through `internal/proc` with explicit argument slices (Constitution I); `net/http` stays confined to internal packages (`TestCmdShllNoNetHTTP` green) — brew reads go through the existing `installedVersion`/`hasBrew` (proc) helpers; new HTTP only in `internal/versions`

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds new functionality (a new subcommand + resolver package) without making existing code redundant. `changelog.LatestTag` remains the delegate target of `versions.LatestGitHub` (still called + tested), so the `resolveOneSpec` seam swap retires no code.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Mutual exclusion enforced by an explicit check in `runCheckUpdates` returning `errExitCode{usageExitCode}`, not `cobra.MarkFlagsMutuallyExclusive` | Cobra's flag-group error is a plain error that would exit 1, not the intake-pinned 2; the shell-init precedent uses explicit `errExitCode{code: 2}` | S:75 R:85 A:85 D:70 |
| 2 | Confident | JSON `formula` field carries the tap-relative leaf name (`run-kit`, `shll`), derived from the roster formula via `TrimPrefix(formulaPrefix)`, identical across backends | Matches the intake's worked example (`"formula": "run-kit"`) and the manifest's own `formula` values; roster-derived keeps Constitution III single-sourcing | S:65 R:80 A:75 D:60 |
| 3 | Confident | JSON and human `installed`/`latest` values are the normalized forms (`NormalizeVer`: `v` prefix + brew `_N` revision stripped) | One comparable form on both sides — the changelog rendering precedent; run-kit joins on clean values | S:65 R:80 A:80 D:70 |
| 4 | Confident | Released rows echo the manifest's raw `notify` string; `notable` is computed treating unknown/empty values as `minor` | Honest echo of the policy source plus the intake's Assumption-13 forward-compat rule; an empty notify is omitted via `omitempty` while `notable` stays present | S:55 R:80 A:70 D:55 |
| 5 | Confident | `notable` serialized as `*bool` + `omitempty` (nil on github rows, pointer on released rows) | The contract requires `"notable": false` emitted on released rows AND full omission on github rows — plain bool+omitempty cannot express both | S:75 R:85 A:90 D:80 |
| 6 | Confident | Manifest `schema != 1` fails the whole check (exit 1) rather than best-effort decoding | shll is the single surface absorbing schema evolution; failing loudly beats silently misreading policy, and run-kit's skip-on-nonzero contract tolerates it | S:55 R:80 A:70 D:55 |
| 7 | Confident | Human status for a released-backend tool absent from the manifest is `not in manifest`; `unavailable` is reserved for github per-tool fetch failures | The intake specifies human statuses only for not-installed/unavailable; a distinct self-describing label keeps the two unresolved causes distinguishable for humans | S:50 R:85 A:70 D:55 |
| 8 | Confident | Bump classification exported as `changelog.FirstDiffComponent(a, b) int`; `versions.Notable` maps policy over it | Version-component parsing already lives in changelog; the intake anticipates "possible small exports for the seam"; duplication is a code-quality anti-pattern | S:60 R:85 A:80 D:60 |
| 9 | Confident | Resolver home is the new `src/internal/versions` package; `LatestGitHub` is a thin delegation to `changelog.LatestTag` returning the release list, and `resolveOneSpec` consumes it | Intake Assumption 14's primary option; returning the list preserves the single-GET contract pinned by `TestChangelog_NoRangeSingleFetchPerRepo` | S:75 R:80 A:85 D:70 |
| 10 | Confident | A not-installed tool renders `not installed` in the version column with an empty status cell | Matches the intake's illustrative sketch (`idea     not installed`) and reuses `notInstalledLabel` | S:70 R:90 A:80 D:75 |
| 11 | Confident | Placement: `check-updates` listed directly after `update` in `AddCommand`/`rootLong`; README section between `shll update` and `shll changelog`; `docs/site/skill.md` capabilities line added + embed re-synced | It is update's check-only sibling; skill.md is part of `docs/site/` which the Standards obligation covers, and the embed drift guard forces the sync | S:50 R:95 A:80 D:60 |

11 assumptions (0 certain, 11 confident, 0 tentative).
