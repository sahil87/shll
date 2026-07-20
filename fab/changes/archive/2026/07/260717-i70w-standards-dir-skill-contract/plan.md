# Plan: Standards directory restructure + the `<tool> skill` contract standard

**Change**: 260717-i70w-standards-dir-skill-contract
**Intake**: `intake.md`

## Requirements

### Docs: standards directory restructure

#### R1: Move the three standards pages into `docs/site/standards/`
The three toolkit-wide standards pages SHALL move from `docs/site/` into a new `docs/site/standards/` subdirectory, filenames unchanged. shll's own tool docs (`install.md`, `workflows.md`) SHALL stay flat in `docs/site/`. Intra-family relative links (principles ↔ help-dump ↔ readme-extraction ↔ skill) remain same-directory and MUST NOT break.

- **GIVEN** the flat layout `docs/site/{principles,help-dump,readme-extraction}.md`
- **WHEN** this change is applied
- **THEN** the files live at `docs/site/standards/{principles,help-dump,readme-extraction}.md`
- **AND** `docs/site/install.md` and `docs/site/workflows.md` remain unmoved
- **AND** every relative link inside the moved pages still resolves inside `docs/site/` (docs/site closure rule 1 — no `..` escapes)

### Docs: the fourth standard — `skill.md`

#### R2: Author `docs/site/standards/skill.md` (the `<tool> skill` contract)
A fourth producer-facing standard SHALL be authored specifying that every toolkit CLI exposes a `<tool> skill` subcommand printing a stable, one-page markdown skill bundle for agent consumption, versioned with the binary. It MUST mirror the `help-dump.md` standard's register and structure (single `#` H1, invocation contract, rules with teeth, a verification section). It MUST state the invocation contract (command name exactly `skill`; raw markdown to stdout byte-identical to the repo's canonical `docs/site/skill.md`; stderr empty; exit 0; content embedded at build with a sync + drift-guard pattern; page renders at `/<tool>/skill` on shll.ai). It MUST ground the `run-kit context` precedent accurately (a ~102-line agent-optimized markdown document mixing static capability prose with a small dynamic Environment header — session/pane/server URL — whereas the skill bundle genre is static-only; dynamic environment info stays in separate commands like `run-kit context`). It MUST state the genre discipline (when-to-use, capabilities map, composition patterns, output/exit-code contracts, gotchas — NOT a second README, NOT flag reference), the ≤150-line hard budget (principle №9), that it implements principles №3 + №10 with scope `binary+repo`, the `skill`-vs-`agent` name rationale, the phased per-repo adoption note (no tool ships `skill` today; this change authors the standard only), and a clearly-marked one-paragraph forward-design note on the planned `shll agent-setup` graduation from `run-kit agent-setup`.

- **GIVEN** the toolkit gap (no offline, embedded, agent-facing tool-usage bundle exists)
- **WHEN** `docs/site/standards/skill.md` is authored
- **THEN** it is a single-`#`-H1 markdown page in the same register/structure as `help-dump.md`
- **AND** every intra-standards link is relative (same directory) and every link leaving the published set is an absolute `https://…` URL
- **AND** it contains no images (docs/site closure)
- **AND** its own body length stays within the register of the sibling standards (the ≤150-line budget it prescribes for bundles is described in prose, not a limit on this standard document)

### Docs: principles.md content updates

#### R3: Update the moved `principles.md` to name the third mechanical contract
The moved `docs/site/standards/principles.md` SHALL be updated so its companions paragraph names three contracts (incl. skill) at same-directory relative links; a new short "The contracts" section after the summary table SHALL list each mechanical contract under the principle(s) it implements (№3 → help-dump, skill; №10 → readme-extraction, skill), stating the two-tier structure (foundation vs mechanical contracts) and the scope vocabulary (binary/repo); principles №3 and №10 SHALL gain a natural sentence referencing the skill contract (№10's enforcement note phrased as SHOULD-phased); and the "Consuming these standards" section URLs SHALL update to `/shll/standards/…` with the companions list gaining skill.

- **GIVEN** the moved `docs/site/standards/principles.md`
- **WHEN** the content updates are applied
- **THEN** the companions paragraph names help-dump, readme-extraction, and skill at same-directory links
- **AND** a "The contracts" section maps each contract to its principle(s) with the foundation-vs-mechanical two-tier framing and binary/repo scope vocabulary
- **AND** the "Consuming these standards" URLs point at `/shll/standards/…`

### CLI: `shll standards` roster + scope column

#### R4: Add the `skill` roster entry and a `scope` field to the `standard` struct
`standardsRoster` (`src/cmd/shll/standards.go`) SHALL gain a fourth entry `skill`, and the `standard` struct SHALL gain a `Scope` field. Scope values SHALL be: `principles`=`foundation`, `help-dump`=`binary`, `readme-extraction`=`repo`, `skill`=`binary+repo`. All four `SourcePath` values SHALL update to `docs/site/standards/<name>.md`.

- **GIVEN** the three-entry roster with a scope-less `standard` struct
- **WHEN** this change is applied
- **THEN** `standardsRoster` has four entries in order principles, help-dump, readme-extraction, skill
- **AND** each entry carries the correct scope value
- **AND** every `SourcePath` is `docs/site/standards/<name>.md`

#### R5: The bare list output gains a scope column
`writeStandardsTable` SHALL emit a three-column aligned tabwriter table (name · scope · description) using the same tabwriter config as today (minwidth 0, tabwidth 0, padding 2, padchar space, no color), in roster order.

- **GIVEN** `shll standards` invoked with no argument
- **WHEN** the table is rendered
- **THEN** each row is `<name> <scope> <description>`, tab-aligned, in roster order, escape-free
- **AND** the 4-row form matches the block in intake Part C

#### R6: `--json` gains an additive `scope` field
`standardJSONItem` SHALL gain a `scope` JSON field (additive, per format-stability rule / principle №2), and `writeStandardsJSON` SHALL populate it. `source_path` values SHALL reflect the new `docs/site/standards/<name>.md` paths.

- **GIVEN** `shll standards --json`
- **WHEN** the JSON array is emitted
- **THEN** each object carries `name`, `description`, `source_path` (at the new path), and the new `scope` field
- **AND** existing fields are unchanged (additive only)

### CLI: sync script + drift guard

#### R7: Sync script points at the new source dir and syncs four files
`scripts/sync-standards.sh` SHALL set `SRC_DIR="docs/site/standards"` and add `skill` to the `STANDARDS` array (now 4 files). The embedded-copy destination layout under `src/cmd/shll/standards/` is apply's call (assumption #9); whichever layout is chosen, the `//go:embed` pattern, `standardsEmbedDir`, and each roster entry's `EmbedName` MUST stay mutually consistent, and the drift guard MUST pass.

- **GIVEN** the sync script sourcing from `docs/site`
- **WHEN** this change is applied
- **THEN** `SRC_DIR="docs/site/standards"` and `STANDARDS=(principles help-dump readme-extraction skill)`
- **AND** running the script produces embedded copies that the drift guard accepts

#### R8: Tests cover the 4-entry roster, new paths, and the scope column/field
`standards_test.go` SHALL be updated so `TestStandardsEmbedMatchesCanonical` covers all four files at their new canonical paths, `TestStandards_ListTable` asserts the scope column, `TestStandards_ListJSON`/`TestStandards_ListJSONFieldNames` assert the `scope` field, and `TestStandardsRosterIntegrity` passes for the 4-entry roster (its `docs/site/` prefix check still passes at the new paths; whether to tighten it to `docs/site/standards/` is apply's call, assumption #12).

- **GIVEN** the updated roster, paths, and renderers
- **WHEN** `go test ./...` runs
- **THEN** all standards tests pass, including the drift guard against the four canonical files
- **AND** the list/JSON tests assert the scope column and field

### Docs: README updates

#### R9: Update README standards links and the `shll standards` example
`README.md` SHALL update its Reference-section standards links to `docs/site/standards/<name>.md` (and add a skill line describing the new standard), and update the `### shll standards` section's example output to the 4-row scope-column form and its `source_path` mention to `docs/site/standards/<name>.md`.

- **GIVEN** the README referencing the flat standards paths and a 3-row example
- **WHEN** this change is applied
- **THEN** the Reference links point at `docs/site/standards/<name>.md` and include a skill entry
- **AND** the `### shll standards` example shows the 4-row scope-column output and the new `source_path`

### Non-Goals

- Implementing `shll skill` or any tool's bundle content (per-repo follow-up wave — intake assumption #3).
- `shll agent-setup` (future graduation change).
- The shll.ai banner-URL / old-flat-URL redirect follow-up (separate repo).
- The 6-repo constitution amendments (separate wave).
- Any change to shll.ai's pull/render pipeline; `docs/site/` stays canonical.

### Design Decisions

1. **Embedded-copy layout under `src/cmd/shll/standards/`**: keep the copies flat (filenames unchanged), NOT mirrored into a `standards/` subdir — *Why*: the drift guard is the contract (intake assumption #9), and a flat layout is a minimal diff to the sync script and `//go:embed standards/*.md` glob — `EmbedName` stays a bare basename, `SourcePath`'s basename still equals `EmbedName` (satisfying `TestStandardsRosterIntegrity`), and no `//go:embed` pattern change is needed. *Rejected*: mirroring `docs/site/standards/` under the embed dir — extra glob/path complexity for no drift-guard benefit.
2. **`TestStandardsRosterIntegrity` SourcePath check**: tighten the prefix assertion from `docs/site/` to `docs/site/standards/` — *Why*: all four SourcePaths now live under `docs/site/standards/`, so the tighter check documents the new invariant and would catch a regression that reintroduced a flat path; low-risk, still passes (intake assumption #12). *Rejected*: leaving it at `docs/site/` — still passes but no longer expresses the true invariant.

## Tasks

### Phase 1: Docs restructure

- [x] T001 Move `docs/site/principles.md`, `docs/site/help-dump.md`, `docs/site/readme-extraction.md` into `docs/site/standards/` via `git mv` (filenames unchanged); leave `docs/site/install.md` and `docs/site/workflows.md` in place <!-- R1 -->

### Phase 2: New standard authoring

- [x] T002 Author `docs/site/standards/skill.md` — the `<tool> skill` contract standard, mirroring `help-dump.md`'s register/structure (single `#` H1, invocation contract, rules with teeth, verification section); ground the `run-kit context` precedent accurately (static prose + small dynamic Environment header; skill bundle is static-only); state genre discipline, ≤150-line budget (№9), implements №3+№10 scope `binary+repo`, `skill`-vs-`agent` name rationale, phased adoption note, and the clearly-marked planned `shll agent-setup` paragraph; intra-standards links relative, external links absolute `https://…`, no images <!-- R2 -->

### Phase 3: principles.md content updates

- [x] T003 Update `docs/site/standards/principles.md`: companions paragraph names three contracts at same-directory links; add a "The contracts" section after the summary table (№3 → help-dump, skill; №10 → readme-extraction, skill; foundation-vs-mechanical two-tier + binary/repo scope vocabulary); add a natural skill sentence to №3 and №10 (№10's enforcement note SHOULD-phased); update "Consuming these standards" URLs to `/shll/standards/…` and its companions list to include skill <!-- R3 -->

### Phase 4: CLI roster + renderers

- [x] T004 In `src/cmd/shll/standards.go`: add a `Scope` field to the `standard` struct (with a doc comment); add the fourth `skill` roster entry; populate `Scope` on all four entries (principles=`foundation`, help-dump=`binary`, readme-extraction=`repo`, skill=`binary+repo`); update all four `SourcePath` values to `docs/site/standards/<name>.md` <!-- R4 -->
- [x] T005 In `src/cmd/shll/standards.go`: update `writeStandardsTable` to emit the three-column `name\tscope\tdescription` row (same tabwriter config) <!-- R5 -->
- [x] T006 In `src/cmd/shll/standards.go`: add a `Scope string \`json:"scope"\`` field to `standardJSONItem` and populate it in `writeStandardsJSON` <!-- R6 -->

### Phase 5: sync script + embed copies

- [x] T007 Update `scripts/sync-standards.sh`: `SRC_DIR="docs/site/standards"` and `STANDARDS=(principles help-dump readme-extraction skill)`; run it to (re-)produce the committed embedded copies under `src/cmd/shll/standards/` (3 refreshed + 1 new `skill.md`) <!-- R7 -->

### Phase 6: tests

- [x] T008 Update `src/cmd/shll/standards_test.go`: `TestStandards_ListTable` asserts each row contains name, scope, and description (roster order, escape-free); `TestStandards_ListJSON` asserts the `scope` field per item; `TestStandards_ListJSONFieldNames` asserts `"scope"` is present; tighten `TestStandardsRosterIntegrity`'s SourcePath prefix check to `docs/site/standards/`; confirm `TestStandardsEmbedMatchesCanonical` covers all four files at the new paths (roster-driven — passes once T004/T007 land) <!-- R8 -->

### Phase 7: README

- [x] T009 Update `README.md`: Reference-section standards links → `docs/site/standards/<name>.md` (+ add a skill line); `### shll standards` example → the 4-row scope-column form; `source_path` mention → `docs/site/standards/<name>.md` <!-- R9 -->

### Phase 8: validation

- [x] T010 Run `cd src && go build ./... && go test ./...`, `gofmt -l` on changed Go files, and `go vet ./...`; fix any failures <!-- R1 R2 R3 R4 R5 R6 R7 R8 R9 -->

## Execution Order

- T001 (move) blocks T003 (edits the moved principles.md) and T007 (sync sources from the moved location).
- T002 (author skill.md) blocks T007 (skill.md must exist to be synced) and T008's drift-guard coverage.
- T004 (roster/struct) blocks T005, T006, T008.
- T007 (sync) must run after T001 + T002 (sources in place) and after T004 (roster paths) so the copies match the roster; it blocks T008's drift guard and T010.
- T010 runs last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: The three standards pages live at `docs/site/standards/{principles,help-dump,readme-extraction}.md`; `install.md` and `workflows.md` remain flat in `docs/site/`
- [x] A-002 R2: `docs/site/standards/skill.md` exists, is a single-`#`-H1 page in help-dump.md's register/structure, and covers every required content point (invocation contract, run-kit context precedent nuance, genre discipline, ≤150-line budget, №3+№10 / `binary+repo`, name rationale, phased adoption, planned `shll agent-setup` paragraph)
- [x] A-003 R3: The moved `principles.md` names three contracts at same-directory links, carries a "The contracts" section with the two-tier + scope framing, references skill in №3/№10, and its "Consuming these standards" URLs are `/shll/standards/…`
- [x] A-004 R4: `standardsRoster` has four entries (principles, help-dump, readme-extraction, skill) with correct scope values and `docs/site/standards/<name>.md` SourcePaths; the `standard` struct has a `Scope` field
- [x] A-005 R5: `shll standards` (bare) prints the 4-row `name · scope · description` aligned table matching the intake Part C block
- [x] A-006 R6: `shll standards --json` emits `{name, description, source_path, scope}` per entry with `source_path` at the new paths; existing fields unchanged
- [x] A-007 R7: `scripts/sync-standards.sh` sources from `docs/site/standards` and syncs four files; the embedded copies match the canonical sources
- [x] A-008 R8: `standards_test.go` covers the 4-entry roster, new paths, and the scope column/field; all standards tests pass
- [x] A-009 R9: `README.md` Reference links point at `docs/site/standards/<name>.md` (incl. a skill line) and the `### shll standards` example shows the 4-row scope-column output with the new `source_path`

### Behavioral Correctness

- [x] A-010 R5: The scope column is inserted between name and description without breaking tabwriter alignment or introducing ANSI escapes (a bytes.Buffer output stays escape-free)
- [x] A-011 R6: The `scope` JSON field is additive — no existing field is renamed, removed, or reordered in a breaking way (format-stability rule / principle №2)

### Scenario Coverage

- [x] A-012 R2: Every intra-standards link in `skill.md` is relative (same directory); every link leaving the published set is an absolute `https://…` URL; there are no images and exactly one `#` H1 (docs/site closure rules)
- [x] A-013 R1: No relative link inside any moved standards page escapes `docs/site/` (closure rule 1); intra-family links (principles ↔ help-dump ↔ readme-extraction ↔ skill) resolve same-directory

### Edge Cases & Error Handling

- [x] A-014 R8: `TestStandardsEmbedMatchesCanonical` fails loudly (naming the file) if any embedded copy drifts from its new canonical `docs/site/standards/` source — the drift guard remains the enforcement seam at the new paths

### Code Quality

- [x] A-015 Pattern consistency: New code follows the surrounding `standards.go` patterns — named constants (no magic strings for the scope column), roster-driven derivation, tabwriter/json.Encoder idioms mirroring `shll list`
- [x] A-016 No unnecessary duplication: The scope column reuses the existing tabwriter writer and roster loop; the `scope` field reuses the existing encoder path — no parallel renderers introduced
- [x] A-017 No hardcoded homebrew paths / no new subprocess: `internal/proc` untouched (Constitution I vacuous); no `os/exec` added (this change adds no subprocess surface)
- [x] A-018 Constitution VII: No new top-level subcommand is added — `standards` already justified; the change is a roster row + scope field, the designed extension path

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change moves and extends existing surfaces without making existing code redundant: the flat `docs/site/{principles,help-dump,readme-extraction}.md` were relocated via `git mv` (not superseded), the two-column table renderer was replaced in place, and every new symbol (`Scope` field, `skill` roster entry) has live call sites in the table/JSON renderers and tests.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Embedded copies stay flat under `src/cmd/shll/standards/` (filenames unchanged), NOT mirrored into a subdir | Intake assumption #9 explicitly delegated the layout to apply; flat is the minimal diff, keeps `//go:embed standards/*.md`, `EmbedName` as a bare basename, and `SourcePath` basename == `EmbedName` (roster-integrity invariant) with no glob change | S:80 R:90 A:85 D:75 |
| 2 | Confident | Tighten `TestStandardsRosterIntegrity`'s SourcePath prefix check from `docs/site/` to `docs/site/standards/` | Intake assumption #12 left this to apply; all four paths now live under the subdir, so the tighter check expresses the real invariant and still passes | S:70 R:90 A:80 D:70 |
| 3 | Confident | The scope column sits between name and description (`name · scope · description`), matching the exact 4-row block in intake Part C | Intake Part C reproduces the output block with scope as the middle column; treated as the literal spec | S:85 R:85 A:80 D:80 |

3 assumptions (1 certain, 2 confident, 0 tentative).
