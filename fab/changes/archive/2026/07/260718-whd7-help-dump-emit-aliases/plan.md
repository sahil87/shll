# Plan: Help-Dump Emits Command Aliases

**Change**: 260718-whd7-help-dump-emit-aliases
**Intake**: `intake.md`

## Requirements

### Producer: `helpNode` schema

#### R1: Optional `aliases` field on `helpNode`
The `helpNode` struct in `src/cmd/shll/help_dump.go` MUST gain an optional `Aliases []string` field with JSON tag `json:"aliases,omitempty"`, placed immediately after `Name`. Under `omitempty`, a nil or empty slice MUST serialize to NO `aliases` key at all (absence, never `[]` or `null`) — deliberately contrasting with the required `Commands` field (always `[]`, never omitted). The Go struct field order (Name, then Aliases) pins the JSON key order.

- **GIVEN** the `helpNode` struct
- **WHEN** a node's `Aliases` slice is nil or empty
- **THEN** the emitted JSON for that node contains no `aliases` key
- **AND** every currently-emitted node stays byte-identical to today's output
- **GIVEN** a node whose command has one or more aliases
- **WHEN** the node is serialized
- **THEN** the `aliases` key appears immediately after `name`, before `path`

#### R2: `buildNode` populates `Aliases` from `cmd.Aliases`
`buildNode` MUST populate the new field directly from cobra's own data model (`cmd.Aliases`, a `[]string`) — consistent with the walk-never-parse rule — in cobra's declared order, with no re-sorting or normalization. A nil/empty `cmd.Aliases` passes through unchanged (relying on `omitempty`).

- **GIVEN** a cobra command with `Aliases: []string{"shell-install"}`
- **WHEN** `buildNode` produces its node
- **THEN** the node's `Aliases` equals `["shell-install"]` in declared order
- **GIVEN** a cobra command with no aliases
- **WHEN** `buildNode` produces its node
- **THEN** the node's `Aliases` is nil/empty and emits no `aliases` key

#### R3: `schema_version` unchanged; determinism preserved
The change MUST NOT bump `helpDumpSchemaVersion` (stays integer `1`) — an optional additive field is the non-breaking evolution path. `aliases` MUST add no time-varying data; `nodeText` is untouched, so byte-for-byte `text` tests and structural-determinism remain unaffected.

- **GIVEN** the emitted document
- **WHEN** `schema_version` is read
- **THEN** it is still `1`
- **GIVEN** two successive dumps of the same tree
- **WHEN** compared byte-for-byte
- **THEN** they are identical

### Standard: canonical help-dump document

#### R4: Standard documents the optional `aliases` field
The canonical standard `docs/site/standards/help-dump.md` MUST document `aliases` in its Output-shape Node example (an optional line with an `omitted when none` comment) plus a normative sentence: `aliases` is an optional additive field under `schema_version: 1`; producers SHOULD emit it when the framework exposes alias metadata (Cobra `cmd.Aliases`); consumers MUST treat alias-form invocations (path with the name replaced by an alias) as valid commands; other tools adopt on their own release cadence (no flag-day, no version bump).

- **GIVEN** the canonical standard
- **WHEN** a reader inspects the Output-shape Node example
- **THEN** it shows the optional `aliases` line
- **AND** a normative sentence states the optional/additive semantics, producer SHOULD, and consumer MUST

#### R5: Embedded standards copy re-synced (drift guard green)
After editing the canonical standard, `scripts/sync-standards.sh` MUST be run so `src/cmd/shll/standards/help-dump.md` matches the canonical file byte-for-byte, keeping `TestStandardsEmbedMatchesCanonical` green.

- **GIVEN** the canonical standard was edited
- **WHEN** `scripts/sync-standards.sh` runs
- **THEN** `src/cmd/shll/standards/help-dump.md` is byte-identical to `docs/site/standards/help-dump.md`
- **AND** `TestStandardsEmbedMatchesCanonical` passes

### Tests: help-dump conformance

#### R6: Synthetic-tree contract shape covers aliases
`src/cmd/shll/help_dump_test.go` MUST be extended so the synthetic tree includes an aliased visible child; the test MUST assert that child's node carries `"aliases": [...]` in declared order, and that unaliased nodes AND the root carry NO `aliases` key (absence, not `[]`/`null`).

- **GIVEN** a synthetic tree with an aliased visible child
- **WHEN** the dump is produced and decoded
- **THEN** the aliased node's `aliases` equals the declared alias list in order
- **AND** the root and any unaliased node emit no `aliases` key

#### R7: Real Execute-path asserts `shell-setup` aliases
A test MUST drive the real `rootCmd.Execute()` path (via the existing `dumpViaExecute` helper, per the prune-before-render design decision) and assert the `shell-setup` node carries exactly `["shell-install"]`.

- **GIVEN** the real command tree dumped via `dumpViaExecute`
- **WHEN** the `shell-setup` node is located
- **THEN** its `aliases` equals exactly `["shell-install"]`

## Tasks

### Phase 1: Producer

- [x] T001 Add `Aliases []string` field with tag `json:"aliases,omitempty"` to `helpNode` in `src/cmd/shll/help_dump.go`, immediately after `Name`, with a concise comment matching the file's existing comment density (note the `omitempty` absence semantics vs. required `Commands`) <!-- R1 -->
- [x] T002 Populate `Aliases: cmd.Aliases` in the `buildNode` return literal in `src/cmd/shll/help_dump.go`, placed right after `Name` (declared order, no sorting) <!-- R2 -->

### Phase 2: Standard + embed

- [x] T003 Update `docs/site/standards/help-dump.md`: add the optional `aliases` line to the Output-shape Node example (with an `omitted when none` comment) and a normative sentence on its optional/additive semantics (producer SHOULD, consumer MUST, per-tool cadence, no version bump) <!-- R4 -->
- [x] T004 Run `scripts/sync-standards.sh` to re-sync `src/cmd/shll/standards/help-dump.md` from the canonical document <!-- R5 -->

### Phase 3: Tests

- [x] T005 Extend `syntheticRoot()` in `src/cmd/shll/help_dump_test.go` to add an aliased visible child, and extend `TestHelpDump_ContractShape` to assert its node carries `"aliases"` in declared order while the root and unaliased nodes carry no `aliases` key (absence) <!-- R6 -->
- [x] T006 Add an Execute-path assertion (via `dumpViaExecute`) in `src/cmd/shll/help_dump_test.go` that the `shell-setup` node carries exactly `["shell-install"]` <!-- R7 -->

### Phase 4: Verification

- [x] T007 Run the relevant help_dump tests, then the drift guard, then the full `src` package tests and `gofmt`; confirm byte-for-byte `text` / determinism tests are unaffected <!-- R3 R5 -->

## Execution Order

- T001 blocks T002 (same struct/function; field must exist before it is populated)
- T003 blocks T004 (sync copies the edited canonical file)
- T005, T006 depend on T001–T002 (the field must exist to assert on it)
- T007 runs last (validates all prior tasks)

## Acceptance

### Functional Completeness

- [x] A-001 R1: `helpNode` has `Aliases []string` with tag `json:"aliases,omitempty"` immediately after `Name`; nil/empty emits no `aliases` key
- [x] A-002 R2: `buildNode` sets `Aliases: cmd.Aliases` in declared order, no sorting/normalization
- [x] A-003 R3: `helpDumpSchemaVersion` is still `1`; no schema bump
- [x] A-004 R4: canonical `docs/site/standards/help-dump.md` documents the optional `aliases` field (example line + normative sentence)
- [x] A-005 R5: `src/cmd/shll/standards/help-dump.md` is byte-identical to the canonical file
- [x] A-006 R6: synthetic-tree test asserts aliased node carries `aliases` in order and root/unaliased nodes carry no key
- [x] A-007 R7: Execute-path test asserts `shell-setup` node carries exactly `["shell-install"]`

### Behavioral Correctness

- [x] A-008 R1: only aliased nodes change bytes; every unaliased node's JSON is byte-identical to pre-change output (absence, not empty array)
- [x] A-009 R3: byte-for-byte `text` tests and `TestHelpDump_StructuralDeterminism` still pass (aliases add no time-varying data; `nodeText` untouched)

### Scenario Coverage

- [x] A-010 R6: `TestHelpDump_ContractShape` (or its extension) exercises the aliased/unaliased/root cases and passes
- [x] A-011 R7: the Execute-path alias assertion passes via `dumpViaExecute`
- [x] A-012 R5: `TestStandardsEmbedMatchesCanonical` passes after the re-sync

### Edge Cases & Error Handling

- [x] A-013 R1: a command with a nil `Aliases` slice and one with an empty slice both emit no `aliases` key (verified by the root/unaliased-node assertions)

### Code Quality

- [x] A-014 Pattern consistency: the new field and `buildNode` line follow the file's existing naming, ordering, and comment density (help_dump.go is heavily documented; the addition is concise and matched)
- [x] A-015 No unnecessary duplication: reuses cobra's `cmd.Aliases` data model and the existing `dumpViaExecute` helper rather than adding new machinery (Constitution III — walk, never parse; wrap, don't reinvent)
- [x] A-016 No magic strings: no hardcoded alias literals in producer code — aliases derive from `cmd.Aliases`; test literals (`shell-install`) are assertions, not producer constants

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Emit an optional `aliases` field on `helpNode` (not duplicate Node entries), placed after `Name` with tag `json:"aliases,omitempty"` | Intake resolves this via the standard's § Schema evolution (optional additive fields); duplicate nodes would fabricate `name`/`path` and break text↔commands coherence. Struct field order pins JSON key order | S:90 R:70 A:95 D:90 |
| 2 | Certain | No `schema_version` bump | Standard reserves bumps for breaking shape changes and names optional additions as the non-breaking path; coordinated 7-tool bump explicitly not needed | S:80 R:65 A:95 D:95 |
| 3 | Confident | Standard's normative sentence lives in the Output-shape section adjacent to the example line (intake offered "Output shape or Schema evolution"); § Schema evolution already carries the general optional-field rule, so the field-specific text is co-located with its example | Reversible doc placement; both sections are valid per intake; adjacency to the example is the clearer read for a producer scanning the Node shape | S:60 R:85 A:75 D:65 |
| 4 | Confident | Add the aliased synthetic child as a NEW visible child (e.g. `aliased` with `Aliases: ["alias-one"]`) rather than mutating the existing `visible` leaf, and keep single-alias `shell-install` for the real-tree test | Preserves the existing `TestHelpDump_ContractShape` "exactly one visible child" invariant would break if a second visible child is added — so assert on the aliased child specifically and update the visible-count expectation accordingly; least-surprise, keeps unrelated assertions intact | S:55 R:80 A:80 D:70 |

4 assumptions (2 certain, 2 confident, 0 tentative).
