# Plan: Proactive-Capability Trigger Vocabulary

**Change**: 260718-pv7t-proactive-trigger-vocabulary
**Intake**: `intake.md`

## Requirements

### Roster: Proactive-capability trigger vocabulary

#### R1: `Tool.ProactiveHint` field
The `Tool` struct in `src/cmd/shll/tools.go` SHALL carry an optional `ProactiveHint string` field: a complete sentence describing an agent-proactive capability (one the agent should reach for unprompted, without the user naming a tool). It is empty for every tool except run-kit (the sprawl guard: only agent-proactive capabilities earn description space). It is single-sourced on the Roster (Constitution III) so the generated skill description cannot drift from the managed set.

- **GIVEN** the `Tool` struct definition
- **WHEN** a roster entry declares an agent-proactive capability
- **THEN** it populates `ProactiveHint` with the complete rendered sentence, verbatim
- **AND** every other roster entry leaves `ProactiveHint` empty (`""`)

#### R2: run-kit's `ProactiveHint` value
run-kit's Roster entry SHALL set `ProactiveHint` to exactly:
`Also use proactively — without the user naming a tool — to show the user visual content (HTML, diagrams, reports, a local dev server) in a browser window, or to push a notification to their devices (run-kit).`

- **GIVEN** run-kit is the only tool carrying the toolkit's agent-proactive capabilities (visual display + notify)
- **WHEN** the roster is defined
- **THEN** run-kit's `ProactiveHint` holds the sentence above verbatim
- **AND** the sentence contains no newline and no `: ` sequence (unquoted-YAML-safe)

#### R3: `agentSkillDescription()` renders the proactive sentence(s)
`agentSkillDescription()` in `src/cmd/shll/agent_setup.go` SHALL append each non-empty `ProactiveHint` (in Roster order) as an additional sentence after the tool clauses and before the closing two-step teaching pointer. `SkillHint` rendering (the `hint (name)` clauses) is unchanged.

- **GIVEN** the roster has exactly one tool (run-kit) with a non-empty `ProactiveHint`
- **WHEN** `agentSkillDescription()` runs
- **THEN** the output shape is: `Use when driving any sahil87 toolkit CLI or shll itself — {clauses}. {ProactiveHint} Run \`shll skill\` to list the installed tools; run \`shll skill <tool>\` for that tool's full usage bundle before using it.`
- **AND** the `ProactiveHint` text appears after the tool clauses and before the `Run \`shll skill\`` pointer
- **AND** the whole description stays a single line with no `: ` sequence (`TestAgentSetup_DescriptionSingleLine` keeps passing)

#### R4: Enrich run-kit's `Roster.Description`
run-kit's `Roster.Description` in `src/cmd/shll/tools.go` SHALL become:
`Run-kit — tmux session manager with a web UI; can display web pages/HTML to the user and push notifications (rk stays as an alias)`
so the display/notify vocabulary reaches every hop-1 surface that renders `Roster.Description` (`shll list` table + `--json`, the bare `shll skill` glossary).

- **GIVEN** the hop-1 surfaces render `Roster.Description` verbatim
- **WHEN** an agent or user browses `shll list` / bare `shll skill`
- **THEN** run-kit's row carries the display/notify capability vocabulary

#### R5: Thin proactive-capabilities pointer in the bootstrap body
The placed SKILL.md body (`agentSkillContent` in `src/cmd/shll/agent_setup.go`) SHALL carry one thin line pointing agent-proactive capabilities at the run-kit bundle, keeping the thin-bootstrap genre (body text loads only on activation):
`Run-kit also has agent-proactive capabilities — visual display in a browser window and push notifications; see \`shll skill run-kit\`.`

- **GIVEN** the body loads only after the skill activates (activation-cost-only)
- **WHEN** an agent has activated the toolkit skill
- **THEN** the body points proactive capabilities at `shll skill run-kit`
- **AND** the line introduces no stanza/sentinel wording (existing `TestAgentSetup_BodyTeachesTwoStepAndStandards` guard keeps passing)

#### R6: README `shll list` example accuracy
The `shll list` example output block in `README.md` (~line 186) SHALL be refreshed so its run-kit row reflects the new `Roster.Description`, keeping the example accurate against actual command output (readme-extraction standard rule 7 — command/flag accuracy).

- **GIVEN** the README `shll list` example quotes run-kit's description
- **WHEN** the description changes
- **THEN** the README example row is updated to the new description text

### Non-Goals

- **No staleness-marker work.** `agentSkillPlacementState` byte-compares placed files against the running binary's canonical content, so any description/body change is self-propagating via `shll update`'s refresh + `shll doctor`'s staleness check (verified in `agent_setup.go`). No hash/marker exists to bump.
- **No new `ProactiveHint` on any other tool.** Reactive tools (wt/idea/tu/hop/fab-kit) stay behind the two-step router; the sprawl guard is deliberate.
- **`SkillHint` stays `"tmux sessions"`** — the reactive task-domain phrase is unchanged.

### Design Decisions

1. **`ProactiveHint` holds the complete sentence verbatim; the builder just appends it**: *Why*: simplest faithful reading of "rendered as one additional sentence" while exactly one tool has a hint — a builder-owned "Also use proactively" preamble would be over-engineering. *Rejected*: storing a fragment and composing a builder-owned preamble.
2. **Sibling test for the ProactiveHint contract, not an extension of `TestRosterSkillHints`**: *Why*: `TestRosterSkillHints` enforces an every-tool required-field contract; `ProactiveHint` is optional-by-design (empty for all but run-kit) — different assertions. *Rejected*: extending `TestRosterSkillHints` to assert on an optional field.

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add the optional `ProactiveHint string` field to the `Tool` struct in `src/cmd/shll/tools.go` with a doc comment explaining the sprawl guard (empty for all tools except run-kit) and the Constitution III single-source rationale <!-- R1 -->
- [x] T002 Populate run-kit's Roster entry in `src/cmd/shll/tools.go`: set `ProactiveHint` to the verbatim sentence (R2) and change `Description` to the enriched display/notify wording (R4) <!-- R2 --> <!-- R4 -->
- [x] T003 Update `agentSkillDescription()` in `src/cmd/shll/agent_setup.go` to append each non-empty `ProactiveHint` (Roster order) between the tool clauses and the closing two-step pointer <!-- R3 -->
- [x] T004 Add the thin proactive-capabilities pointer line to `agentSkillContent` body in `src/cmd/shll/agent_setup.go` <!-- R5 -->

### Phase 3: Integration & Edge Cases

- [x] T005 Add a sibling test to `TestRosterSkillHints` in `src/cmd/shll/agent_setup_test.go` pinning the ProactiveHint contract: run-kit's `ProactiveHint` is non-empty, the generated description contains it, and it precedes the `Run \`shll skill\`` two-step pointer <!-- R3 -->
- [x] T006 Run the scoped test suite (`cd src && go test ./cmd/shll/`) and confirm `TestAgentSetup_DescriptionSingleLine`, `TestRosterSkillHints`, `TestAgentSetup_BodyTeachesTwoStepAndStandards`, and the new sibling test all pass <!-- R3 -->

### Phase 4: Polish

- [x] T007 Refresh the run-kit row in the `shll list` example output in `README.md` (~line 186) to match the new `Roster.Description` <!-- R6 -->

## Execution Order

- T001 blocks T002 (T002 populates the field T001 adds) and T003/T005 (they render/assert on it)
- T002 also feeds T007 (README example must match the new Description)
- T006 runs after T001–T005

## Acceptance

### Functional Completeness

- [x] A-001 R1: The `Tool` struct carries an optional `ProactiveHint string` field, documented, empty for all tools except run-kit
- [x] A-002 R2: run-kit's `ProactiveHint` equals the intake's verbatim sentence and is newline-free and `: `-free
- [x] A-003 R3: `agentSkillDescription()` appends each non-empty `ProactiveHint` after the tool clauses and before the two-step pointer
- [x] A-004 R4: run-kit's `Roster.Description` carries the display/notify vocabulary and flows to `shll list` (table + JSON) and bare `shll skill`
- [x] A-005 R5: `agentSkillContent` body carries the thin proactive-capabilities → `shll skill run-kit` pointer
- [x] A-006 R6: the README `shll list` example run-kit row matches the new `Roster.Description`

### Behavioral Correctness

- [x] A-007 R3: `TestAgentSetup_DescriptionSingleLine` still passes — the enriched description remains a single line with no `: ` sequence
- [x] A-008 R1: no other roster tool gains a non-empty `ProactiveHint` (sprawl guard held); `SkillHint` stays `"tmux sessions"` for run-kit

### Scenario Coverage

- [x] A-009 R3: the new sibling test asserts run-kit's `ProactiveHint` is non-empty, present in the description, and precedes the two-step pointer
- [x] A-010 R4: `TestList_*` / `TestSkill_*` (or equivalent) still pass with the new run-kit description

### Edge Cases & Error Handling

- [x] A-011 R3: with exactly one tool carrying a `ProactiveHint`, the description renders exactly one proactive sentence (no trailing/leading spacing artifacts, no double period)

### Code Quality

- [x] A-012 Pattern consistency: the new field, doc comment, and builder loop follow the existing `SkillHint` conventions in `tools.go` / `agent_setup.go`
- [x] A-013 No unnecessary duplication: the proactive sentence lives once on the Roster (Constitution III); the builder appends it rather than re-declaring it in `agent_setup.go`

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

None — this change adds new functionality without making existing code redundant

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | `ProactiveHint` holds the complete sentence verbatim; the builder appends it between the tool clauses and the two-step pointer | Simplest faithful reading of "one additional sentence" while exactly one tool has a hint; a builder-owned preamble is over-engineering; sentence verified single-line and `: `-free | S:75 R:85 A:75 D:65 |
| 2 | Confident | Include the thin body pointer line in `agentSkillContent` (intake said "consider") | Activation-cost-only, one line, keeps the thin-bootstrap genre; easily removed if it reads as sprawl | S:50 R:85 A:65 D:55 |
| 3 | Confident | New sibling test (not an extension of `TestRosterSkillHints`) pins the ProactiveHint contract | `TestRosterSkillHints` enforces an every-tool required field; `ProactiveHint` is optional-by-design — separate assertions | S:70 R:90 A:80 D:70 |
| 4 | Certain | README `shll list` example run-kit row refreshed to match the new Description (readme-extraction rule 7) | Standard requires README prose to be accurate against actual output; intake explicitly calls out the line | S:90 R:90 A:85 D:80 |

4 assumptions (1 certain, 3 confident, 0 tentative).
