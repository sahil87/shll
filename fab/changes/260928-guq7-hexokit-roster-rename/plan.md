# Plan: HexoKit Roster Rename (run-kit → hexokit)

**Change**: 260928-guq7-hexokit-roster-rename
**Intake**: `intake.md`

## Requirements

### CLI: Roster identity

#### R1: The run-kit roster entry is renamed to hexokit
The Roster entry formerly named `run-kit` SHALL have `Name: "hexokit"`, `Formula: formulaPrefix + "hexokit"`, `Update: {"hexokit", "update"}`, and `Repo: "run-kit"` (unchanged until the R2 repo rename). Its `Description` SHALL lead with `HexoKit —`, and its `ProactiveHint` SHALL route agents to `shll skill hexokit` / `shll skill hexokit tutorial` while keeping `rk code exec`. The `rk-desktop` entry keeps its Name, Repo, and `rk desktop …` argvs; its Description says `HexoKit desktop viewer shell`. Every roster-order surface (install/update/list/doctor/version/shell-init) inherits the new name.

- **GIVEN** the compiled shll binary
- **WHEN** `shll list --json` runs
- **THEN** the first roster row is named `hexokit` with repo URL `https://github.com/sahil87/run-kit`
- **AND** `shll install` would run `brew install sahil87/tap/hexokit` for a missing install, `shll update` delegates to `hexokit update`

#### R2: Legacy names are a list and include both prior names
`Tool.LegacyName string` SHALL be replaced by `Tool.LegacyNames []string`; the hexokit entry carries `{"rk", "run-kit"}`, every other entry none. The version probe (`probeToolVersion`) SHALL, when `<Name> --version` fails with `proc.ErrNotFound` only, retry each legacy name in order, returning the first result that is not `ErrNotFound`; a non-ErrNotFound failure (present-but-broken binary) SHALL NOT defer to the next name. The generated agent-skill description SHALL render the tool token as `hexokit/rk/run-kit`.

- **GIVEN** only `run-kit` is on PATH (no `hexokit`, no `rk`)
- **WHEN** `shll version` probes the hexokit tool
- **THEN** it reports run-kit's version under the `hexokit` label
- **GIVEN** `hexokit` is missing and `rk` exists but exits non-zero
- **WHEN** the probe runs
- **THEN** rk's error is returned and `run-kit` is not tried

#### R3: Prior names resolve as target aliases
`legacyAliases` SHALL map both `rk` and `run-kit` to `hexokit`, so `shll update|install|uninstall|skill|changelog` accept either token, print `note: <alias> is now hexokit` where they already print notices, and keep aliases out of the valid-targets diagnostic.

- **GIVEN** a user runs `shll update run-kit`
- **WHEN** targets are resolved
- **THEN** the hexokit tool is selected and `note: run-kit is now hexokit` is printed
- **GIVEN** `shll skill rk display`
- **THEN** shll invokes `hexokit skill display`

#### R4: Agent-setup delegation targets hexokit
`shll setup agent` SHALL delegate hook wiring to `hexokit agent setup` (forwarding `--uninstall`/`--yes` as today), via a renamed named constant (`hexokitToolName`). The uninstall daemon-stop hint SHALL key on that constant matching the roster Name. Substrate identifiers (`rkBinary`, rk-desktop refusal token) SHALL be unchanged.

- **GIVEN** `shll setup agent --yes`
- **WHEN** the delegation runs
- **THEN** the recorded argv is `hexokit agent setup --yes`

### CLI: check-updates

#### R5: Manifest lookup tolerates the pre-rename manifest key
In the `released` backend, a roster tool's manifest row SHALL be looked up by `Name` first, then by each `LegacyNames` entry in order; `inManifest` is true when any key matched. JSON output keeps `name: "hexokit"`, `formula: "hexokit"`.

- **GIVEN** a manifest containing only a `run-kit` key
- **WHEN** `shll check-updates --json` runs with hexokit installed
- **THEN** the `hexokit` row carries run-kit's `latest`/`notify`
- **GIVEN** a manifest containing both `hexokit` and `run-kit` keys
- **THEN** the `hexokit` key's values win

### Docs: present-tense surfaces

#### R6: Docs name hexokit as the managed tool
README.md and `docs/site/**` present-tense text (roster lists, valid targets, sample rows, `shll skill hexokit …`, `hexokit agent setup`, the tap-qualified formula `sahil87/tap/hexokit`) SHALL name `hexokit`, noting `rk`/`run-kit` as accepted legacy aliases. The stale README "Legacy `rk` keg exception" paragraph SHALL be removed or rewritten. Embedded copies (`src/cmd/shll/standards/*.md`, `src/cmd/shll/skill/skill.md`) SHALL be regenerated via `scripts/sync-standards.sh`, not hand-edited. Historical references (D11) and `github.com/sahil87/run-kit` links stay.

- **GIVEN** the docs after the change
- **WHEN** grepping present-tense roster lists
- **THEN** they list `hexokit`, and `TestStandardsEmbedMatchesCanonical` passes

### Non-Goals

- `versions.json` — built by hexokit-site from `help/<slug>.json` + `versions-policy.json`; the "S3 envelope carry-over" is hexokit-site work (R1(d)).
- run-kit's `updatecheck.runKitTool` accepting `hexokit` — cross-repo follow-up.
- `Repo` flips (R2); renaming substrate (`rk`, `RK_*`, `rk-desktop`) per D2.
- Adding `xk` as an alias.
- `docs/memory/**` — hydrate.

### Design Decisions

#### LegacyNames slice over a second string field
**Decision**: Replace `LegacyName string` with `LegacyNames []string` ordered by rename recency-of-alias (`rk`, then `run-kit`).
**Why**: The tool now has two prior names that both still resolve on disk (`rk` symlink, Homebrew's `run-kit` compat link); a slice expresses N renames without new fields.
**Rejected**: A parallel `LegacyName2` field (doesn't scale, awkward call sites); dropping `rk` (still the canonical short command per D2).
*Introduced by*: 260928-guq7-hexokit-roster-rename

#### Manifest key fallback in shll
**Decision**: check-updates looks up `Name`, then legacy names, in the versions manifest.
**Why**: The manifest is renamed in a different repo on its own schedule; shll must not lose hexokit's update signal in either ordering.
**Rejected**: Waiting for hexokit-site to rename first (couples releases); publishing both keys only on the site (still breaks older/newer shll in one direction).
*Introduced by*: 260928-guq7-hexokit-roster-rename

## Tasks

### Phase 2: Core Implementation

- [x] T001 `src/cmd/shll/tools.go`: rename the roster entry per R1 (Name/Formula/Update/Description/ProactiveHint; rk-desktop Description; Repo doc comment explaining Name≠Repo until R2; roster-order comments); replace `LegacyName` with `LegacyNames []string` + doc comment; `legacyAliases` → `{"rk": "hexokit", "run-kit": "hexokit"}` + doc comments <!-- R1 R2 R3 -->
- [x] T002 `src/cmd/shll/version.go` (`probeToolVersion` legacy chain, ErrNotFound-only), `doctor.go` comments, `agent_setup.go` (`agentSkillDescription` renders `/`-joined legacy names; `runKitToolName` → `hexokitToolName = "hexokit"` and related doc/user-visible strings), `uninstall.go` (daemon-stop hint keyed on `hexokitToolName`), plus any other `LegacyName`/`run-kit` references in non-test `src/cmd/shll/*.go` (list.go, skill.go, changelog.go, setup.go help text, main.go comment) <!-- R2 R3 R4 -->
- [x] T003 `src/cmd/shll/check_updates.go`: add a `manifestEntry` helper (Name, then LegacyNames) used by `resolveOneTarget`; update the `formulaLeaf` comment example <!-- R5 -->

### Phase 3: Integration & Edge Cases

- [x] T004 Tests in `src/cmd/shll/*_test.go`: update every roster-name assertion to `hexokit` (keep `internal/changelog`'s `ReleasesURL("run-kit")` — repo slug); add coverage for the legacy probe chain (rk then run-kit; non-ErrNotFound stops the chain), `run-kit`/`rk` alias → hexokit with notice, manifest key fallback (legacy-only key, both keys → hexokit wins, neither), description token `hexokit/rk/run-kit` + new ProactiveHint, delegation argv `hexokit agent setup …`. Run `cd src && gofmt -l . && go vet ./... && go test ./...` <!-- R1 R2 R3 R4 R5 -->

### Phase 4: Polish

- [x] T005 Docs: README.md and `docs/site/{install,workflows,skill}.md`, `docs/site/standards/{update,version,principles,install-composition,help-dump,config-home}.md` per R6 (remove/rewrite README's stale "Legacy `rk` keg exception"); then run `./scripts/sync-standards.sh` to refresh `src/cmd/shll/standards/*.md` and `src/cmd/shll/skill/skill.md`; re-run the drift-guard test <!-- R6 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: Roster entry is `hexokit` / `sahil87/tap/hexokit` / `{"hexokit","update"}` / Repo `run-kit`; Description and ProactiveHint name HexoKit and `shll skill hexokit`
- [x] A-002 R2: `LegacyNames []string{"rk","run-kit"}` replaces `LegacyName`; no `LegacyName` identifier remains
- [x] A-003 R3: `legacyAliases` maps `rk` and `run-kit` to `hexokit`
- [x] A-004 R4: agent-setup delegation argv is `hexokit agent setup …`; uninstall hint keys on the renamed constant
- [x] A-005 R5: `resolveOneTarget` resolves manifest rows through the legacy-key fallback helper
- [x] A-006 R6: README + docs/site present-tense surfaces say hexokit; embeds synced

### Scenario Coverage

- [x] A-007 R2: Test: only `run-kit` on PATH → version reported under hexokit; `rk` present-but-broken → `run-kit` not tried
- [x] A-008 R3: Test: `update run-kit` selects hexokit and prints `note: run-kit is now hexokit`; `skill rk` invokes `hexokit skill`
- [x] A-009 R5: Tests: legacy-only key resolves; both keys → hexokit wins; neither → not in manifest
- [x] A-010 R2: Test: agent skill description contains `hexokit/rk/run-kit`

### Edge Cases & Error Handling

- [x] A-011 R2: All three names ErrNotFound → ErrNotFound (not installed), not a different error

### Code Quality

- [x] A-012 Pattern consistency: New code follows naming and structural patterns of surrounding code (doc-comment density, named constants)
- [x] A-013 No unnecessary duplication: legacy-name iteration isn't copy-pasted across probe/description/manifest beyond what's clear
- [x] A-014 No magic strings: tool names come from roster fields / named constants
- [x] A-015 Subprocess invocation still routes through `internal/proc`
- [x] A-016: `gofmt -l`, `go vet ./...`, `go test ./...` all clean

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — the rename replaced `Tool.LegacyName` and its single-name probe/alias call sites in place (converted to the `LegacyNames []string` forms in `src/cmd/shll/tools.go`, `version.go`, `agent_setup.go`); no surviving file, symbol, branch, or config was made redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Legacy probe chain stops at the first non-ErrNotFound result | Generalizes the existing ErrNotFound-only rule to N names without masking a broken binary | S:80 R:85 A:85 D:80 |
| 2 | Confident | `legacyAliases` stays an explicit map (not derived from LegacyNames) | Existing pattern; derivation would add indirection for two entries | S:70 R:90 A:80 D:70 |
| 3 | Confident | Manifest fallback prefers `Name` over legacy keys | The new key is authoritative once published | S:80 R:90 A:85 D:85 |

3 assumptions (0 certain, 3 confident, 0 tentative).
