# Plan: Install-Composition Toolkit Standard

**Change**: 260720-w6ay-install-composition-standard
**Intake**: `intake.md`

## Requirements

### Standards: the install-composition document

#### R1: Standard document exists with the established shape
A new file `docs/site/standards/install-composition.md` MUST exist and follow the established companion-standard structure: `# Standard: install-composition` title; a producer-facing intro stating what the standard governs, naming the principles it implements (№7 compose-don't-reinvent, №8 graceful degradation) with a link to `principles.md`, and its scope (all seven tap formulas for Policy A; the six roster-tool repos plus the tap README for Policy B — `shll`'s own README is out of Policy B's producer scope because it, with shll.ai, *is* the centralized install documentation, mirroring `update.md`'s shll-out-of-producer-scope phrasing). Length SHOULD stay at or under `update.md`'s (~67 lines).

- **GIVEN** the seven existing files in `docs/site/standards/`
- **WHEN** `install-composition.md` is added
- **THEN** its title, intro (principles named + scope note), MUST/SHOULD obligation sections, and closing `## Verifying conformance` checklist match the structure and tone of `update.md`/`readme-extraction.md`

#### R2: Policy A — no inter-tool formula dependencies
The document MUST state: toolkit formulas MUST NOT declare `depends_on` on sibling toolkit formulas; `shll install` is the composition point that installs the full roster (and accepts a subset); a formula edge duplicates roster knowledge in the tap and forces lockstep installs/uninstalls. It MUST carry the context receipt: `fab-kit` and `hop` previously declared `depends_on` on `wt`/`idea` — those edges are removed, and the `all` meta-formula is retired in favor of `shll install`.

- **GIVEN** a tool author editing a tap formula
- **WHEN** they read the standard
- **THEN** the obligation (no sibling `depends_on`), the reason (shll install owns composition), and the precedent receipt are all stated

#### R3: Policy A, binary half — probe at runtime, degrade gracefully
The document MUST state: a tool that invokes a sibling tool at runtime MUST probe for it — `command -v <tool>` in shell/skill code, `exec.LookPath` in Go — and MUST NOT assume presence; on a missing sibling it MUST degrade gracefully with an actionable install hint, with the example message verbatim: `wt is not installed. Install it: brew install sahil87/tap/wt`.

- **GIVEN** a tool that shells out to a sibling
- **WHEN** the sibling is not installed
- **THEN** the standard requires a probe before invocation and a skip-with-hint (never a crash), with the exact hint format shown

#### R4: Policy B — install documentation is centralized
The document MUST state: per-tool READMEs and the tap README MUST NOT carry per-formula `brew install` instructions; they link to https://shll.ai for install steps (the curl bootstrap / `shll install`). The supported-vs-unsupported line MUST be explicit: individual formula installs remain **supported** (`brew install sahil87/tap/<tool>` works, and `shll install` accepts a subset) — what is unsupported is **documenting** them per-repo, which drifts.

- **GIVEN** a repo README with an install section
- **WHEN** checked against the standard
- **THEN** a per-formula `brew install` line is a violation, a link to shll.ai is conformant, and the supported/documented distinction is unambiguous

#### R5: Verifying conformance checklist
The document MUST close with a `## Verifying conformance` section: no `depends_on` on a sibling in the tool's tap formula; every sibling invocation is behind a probe; missing-sibling paths emit the install hint; README/tap-README install sections link to shll.ai instead of carrying `brew install` lines.

- **GIVEN** a tool author shipping a change touching a formula, a sibling invocation, or a README install section
- **WHEN** they run the checklist
- **THEN** each of the four obligations has a checkable line

### Standards: index and embed wiring

#### R6: principles.md index updates
`docs/site/standards/principles.md` MUST be updated in exactly three places: (1) the "Six companion standards" intro paragraph becomes seven, with the three-plus-three categorization sentence reworked to include `install-composition`; (2) "The contracts" table gains a row — `install-composition` | №7, №8 | binary + repo | one-line description; (3) the closing "Consuming these standards" paragraph's parenthesized companion list gains `install-composition`. The ten principle sections themselves MUST NOT be edited.

- **GIVEN** the current `principles.md`
- **WHEN** the update lands
- **THEN** the intro count, contracts table, and consuming list all include install-composition, and a diff shows no change inside the `## 1.`–`## 10.` sections

#### R7: Embed wiring — sync script, Go roster, committed copies
The standard MUST be wired into `shll standards`: (1) `scripts/sync-standards.sh` `STANDARDS=(...)` array gains `install-composition`; (2) `src/cmd/shll/standards.go` `standardsRoster` gains an entry — `Name: "install-composition"`, one-line `Description`, `Scope: "binary+repo"` (existing pinned vocabulary), `SourcePath: "docs/site/standards/install-composition.md"`, `EmbedName: "install-composition.md"` — appended last, consistent with the existing roster order (which mirrors the contracts-table order); (3) `scripts/sync-standards.sh` is run and the refreshed embed copies under `src/cmd/shll/standards/` (new `install-composition.md` AND updated `principles.md`) are left committed-ready.

- **GIVEN** the built binary after this change
- **WHEN** `shll standards` and `shll standards install-composition` run
- **THEN** the list shows eight entries and the reader prints the document byte-identical to its canonical source

#### R8: Roster-driven tests stay green with no test-code changes
`go build ./...` and the standards tests (`go test ./cmd/shll/ -run 'TestStandards'`) MUST pass with no edits to `src/cmd/shll/standards_test.go` — the tests are roster-driven (list, JSON, byte-identical reader, drift guard, integrity/scope vocabulary) and pick up the new entry automatically.

- **GIVEN** the wired roster and refreshed embed copies
- **WHEN** `cd src && go test ./cmd/shll/ -run 'TestStandards' -v` runs
- **THEN** all standards tests pass, including the drift guard comparing embedded bytes against `docs/site/standards/`

### Non-Goals

- Removing the `depends_on` edges in `fab-kit`/`hop` or retiring the `all` meta-formula — parallel changes in other repos, cited as context only.
- Conforming the tap README or per-tool READMEs — each repo conforms on its own cadence (the phased-rollout convention).
- Any shll binary behavior change beyond the data-only roster entry.

## Tasks

### Phase 2: Core Implementation

- [x] T001 Author `docs/site/standards/install-composition.md` — title, intro (principles №7/№8 + scope carve-out), Policy A formula section with context receipt, Policy A probe/degrade section with verbatim hint, Policy B section with supported-vs-unsupported line, `## Verifying conformance` checklist; match `update.md`'s structure/tone and stay at or under its length <!-- R1, R2, R3, R4, R5 -->
- [x] T002 Update `docs/site/standards/principles.md` in three places only: intro paragraph (six → seven companion standards, reworked categorization sentence), "The contracts" table (new `install-composition` row: №7, №8 | binary + repo | one-line description), "Consuming these standards" parenthesized list <!-- R6 -->

### Phase 3: Integration & Edge Cases

- [x] T003 [P] Append `install-composition` to the `STANDARDS=(...)` array in `scripts/sync-standards.sh` <!-- R7 -->
- [x] T004 [P] Add the `standardsRoster` entry in `src/cmd/shll/standards.go` (Name `install-composition`, one-line Description, Scope `binary+repo`, SourcePath `docs/site/standards/install-composition.md`, EmbedName `install-composition.md`), appended last to match roster order <!-- R7 -->
- [x] T005 Run `scripts/sync-standards.sh` to refresh the embed copies under `src/cmd/shll/standards/` (new file + updated `principles.md` copy) <!-- R7 -->
- [x] T006 Verify: `cd src && go build ./...` and `go test ./cmd/shll/ -run 'TestStandards' -v` pass with no test-code changes; run the full `go test ./cmd/shll/` package if quick <!-- R8 -->

## Execution Order

- T001 and T002 before T005 (the sync copies canonical files)
- T003 and T004 are independent of each other ([P]) but both precede T005/T006
- T006 last

## Acceptance

### Functional Completeness

- [ ] A-001 R1: `docs/site/standards/install-composition.md` exists with the `# Standard: install-composition` title, producer-facing intro naming principles №7/№8, the Policy A/B scope note (shll README carve-out for Policy B), and length ≤ ~update.md's
- [ ] A-002 R2: Policy A section states the MUST NOT `depends_on` obligation, names `shll install` as the composition point (full roster + subset), and carries the fab-kit/hop + `all`-retirement context receipt
- [ ] A-003 R3: Probe section mandates `command -v` (shell/skills) / `exec.LookPath` (Go) and skip-with-hint degradation, with the verbatim example `wt is not installed. Install it: brew install sahil87/tap/wt`
- [ ] A-004 R4: Policy B section bans per-formula `brew install` lines in per-tool/tap READMEs, points to https://shll.ai, and states the supported-installs-vs-unsupported-documentation line explicitly
- [ ] A-005 R5: `## Verifying conformance` closes the document with checkable lines covering formula edges, probes, hints, and README install sections
- [ ] A-006 R6: `principles.md` updated in exactly the three index places (intro count + categorization, contracts table row, consuming list); no edits inside the ten principle sections
- [ ] A-007 R7: sync-script array, Go roster entry (Scope `binary+repo`, appended last), and refreshed committed embed copies (new file + principles.md) are all in place

### Scenario Coverage

- [ ] A-008 R8: `go build ./...` and `go test ./cmd/shll/ -run 'TestStandards' -v` pass with zero changes to `standards_test.go` (drift guard green against the refreshed copies)

### Edge Cases & Error Handling

- [ ] A-009 R1: the scope asymmetry is stated without ambiguity — Policy A binds all seven formulas including shll's; Policy B excludes shll's own README (it is the central install doc, not a violation)

### Code Quality

- [ ] A-010 Pattern consistency: the new standard's structure/tone matches the existing companion standards, and the roster entry matches the existing `standard` struct field conventions
- [ ] A-011 No unnecessary duplication: the standard links to principles/companion pages rather than restating them; the roster reuses the pinned scope vocabulary (no new scope value, no magic strings)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Contracts-table scope cell rendered `binary + repo` (spaced, matching the existing skill row); Go roster Scope stays `binary+repo` (the test-pinned vocabulary) | The markdown table and the Go field are two surfaces with two existing conventions — matching each in place beats forcing one form onto both | S:60 R:95 A:85 D:75 |
| 2 | Confident | Append the new entry last in both the `STANDARDS` array and `standardsRoster` (after `shell-init`), and last in the contracts table | Existing roster order mirrors the principles contracts-table order (principles first, then the six in table order); appending preserves that mirror and is the lowest-churn ordering | S:65 R:90 A:85 D:80 |
| 3 | Confident | Intro categorization reworked as three documentation/help + four composition-surface standards, with install-composition joining the "surfaces shll composes" group | install-composition's subject is exactly how shll composes the toolkit at install time — the natural fourth member of the update/version/shell-init group; a new third category would over-structure a one-paragraph enumeration | S:55 R:90 A:75 D:65 |
| 4 | Certain | Roster Description one-liner: "No sibling `depends_on` between toolkit formulas; probe siblings at runtime; install docs centralized on shll.ai" | Intake supplies the example description nearly verbatim; trimmed to match sibling entries' one-line register | S:85 R:95 A:90 D:85 |

4 assumptions (1 certain, 3 confident, 0 tentative).
