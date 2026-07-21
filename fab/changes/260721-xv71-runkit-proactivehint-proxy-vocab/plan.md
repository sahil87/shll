# Plan: Extend run-kit proactive-hint proxy vocabulary

**Change**: 260721-xv71-runkit-proactivehint-proxy-vocab
**Intake**: `intake.md`

## Requirements

### CLI: run-kit `ProactiveHint` roster value

#### R1: New two-sentence `ProactiveHint` value on the run-kit Roster entry
The run-kit entry in `Roster` (`src/cmd/shll/tools.go`, the only entry with a non-empty `ProactiveHint`) MUST carry the user-approved draft wording verbatim, replacing the current single-sentence value. The new value MUST preserve both functions: (a) proxy trigger vocabulary ("to proxy a local http port to the user's browser") and (b) the skill-shadowing counter-instruction ("The user may be viewing this session remotely through run-kit's web dashboard, where `open`/`xdg-open` and localhost URLs never reach them — before opening any file or local port in a browser, read `shll skill run-kit` for the proxied-iframe recipe."). No other Roster field changes — `SkillHint` stays `"tmux sessions"`, `Description` stays as-is.

- **GIVEN** the `Roster` slice in `src/cmd/shll/tools.go`
- **WHEN** `agentSkillDescription()` renders the frontmatter description
- **THEN** the rendered description contains the new two-sentence `ProactiveHint` verbatim, positioned after the last tool clause and before the two-step pointer (`Run \`shll skill\``)
- **AND** no other tool carries a `ProactiveHint` (sprawl guard intact)

#### R2: `ProactiveHint` field doc comment updated to sentence(s)/prose
The `ProactiveHint` field doc comment on the `Tool` struct (`src/cmd/shll/tools.go`) MUST no longer claim the field is "a complete sentence" — the new value is two sentences. It SHALL say the field holds complete sentence(s)/prose appended verbatim, keeping the rest of the contract text intact (agent-proactive vocabulary, run-kit-only sprawl guard, optional-by-design, Constitution III single-sourcing).

- **GIVEN** the `Tool` struct doc comments
- **WHEN** a reader inspects the `ProactiveHint` field contract
- **THEN** the doc comment accurately describes a multi-sentence prose value and retains the sprawl-guard and optional-by-design contract text

#### R3: `TestRosterProactiveHint` extended to pin the two load-bearing fragments
`TestRosterProactiveHint` (`src/cmd/shll/agent_setup_test.go`) MUST additionally assert the rendered description contains the two load-bearing fragments — `"to proxy a local http port"` (function a) and `"before opening any file or local port in a browser, read"` (function b) — so a future rewording cannot silently drop either function. The existing assertions stay unchanged: (i) exactly run-kit carries a `ProactiveHint` (sprawl guard), (ii) the hint appears verbatim in the description (dynamic containment), (iii) position after the last tool clause and before the two-step pointer. The test's doc comment MUST be updated to describe the fragment pins.

- **GIVEN** the extended `TestRosterProactiveHint`
- **WHEN** a future edit removes or rewords either the proxy-vocabulary fragment or the counter-instruction fragment
- **THEN** the test fails, naming the missing fragment

#### R4: Hard test-pinned invariants hold unchanged
The rendered description MUST remain a single line with no `: ` (colon-space) sequence (unquoted YAML frontmatter scalar) and no ` #` comment-start sequence. `TestAgentSetup_DescriptionSingleLine` and `TestRosterSkillHints` MUST keep passing without modification. No change to `agentSkillDescription()`, the skill body (`agentSkillContent`), or any propagation code (`refreshPlacedAgentSkills` / `agentSkillPlacementState` already cover propagation — verified).

- **GIVEN** the new `ProactiveHint` value in place
- **WHEN** `go test ./cmd/shll/ -run 'TestAgentSetup|TestRoster'` runs
- **THEN** all tests pass, including the unmodified `TestAgentSetup_DescriptionSingleLine` and `TestRosterSkillHints`

### Non-Goals

- The bootstrap skill BODY (`agentSkillContent` in `agent_setup.go`, including its "Run-kit also has agent-proactive capabilities …" line) — stays thin; a different design surface.
- run-kit's own bundle / topic pages — different repo.
- Patching `visual-explainer` or any third-party plugin.
- A run-kit `agent-setup` session-start hook (the durable escalation) — deliberately deferred as follow-up.
- Run-kit's Roster `Description` and `SkillHint` — decided scope is the `ProactiveHint` field only.

## Tasks

### Phase 2: Core Implementation

- [x] T001 Replace the run-kit `ProactiveHint` value in `src/cmd/shll/tools.go` (Roster entry, ~line 157) with the user-approved two-sentence wording from the intake, verbatim <!-- R1 -->
- [x] T002 [P] Update the `ProactiveHint` field doc comment on the `Tool` struct in `src/cmd/shll/tools.go` (~lines 58–67) from "a complete sentence" to complete sentence(s)/prose, keeping the rest of the contract text intact <!-- R2 -->
- [x] T003 Extend `TestRosterProactiveHint` in `src/cmd/shll/agent_setup_test.go` (~line 400) with containment assertions for the two load-bearing fragments (`"to proxy a local http port"`, `"before opening any file or local port in a browser, read"`); update the test's doc comment; leave sprawl-guard and position assertions unchanged <!-- R3 -->

### Phase 3: Integration & Edge Cases

- [x] T004 Run the scoped tests from `src/`: `go test ./cmd/shll/ -run 'TestAgentSetup|TestRoster'` — all pass, including unmodified `TestAgentSetup_DescriptionSingleLine` and `TestRosterSkillHints`; then run the full `go test ./...` from `src/` as final verification <!-- R4 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The run-kit Roster entry's `ProactiveHint` holds the approved two-sentence value verbatim, carrying both the proxy-vocabulary fragment and the skill-shadowing counter-instruction; no other Roster field changed
- [x] A-002 R2: The `ProactiveHint` field doc comment describes sentence(s)/prose (not "a complete sentence") and retains the sprawl-guard, optional-by-design, and Constitution III contract text
- [x] A-003 R3: `TestRosterProactiveHint` pins both load-bearing fragments via containment assertions and its doc comment describes them

### Behavioral Correctness

- [x] A-004 R1: The rendered `agentSkillDescription()` output contains the new hint verbatim, positioned after the last tool clause and before the two-step pointer, with exactly run-kit carrying a hint (existing assertions confirm)

### Scenario Coverage

- [x] A-005 R3: Removing either fragment from the `ProactiveHint` value would fail the extended test (fragment pins are independent containment checks against the rendered description)

### Edge Cases & Error Handling

- [x] A-006 R4: The rendered description remains a single line with no `: ` sequence — `TestAgentSetup_DescriptionSingleLine` passes unchanged; `TestRosterSkillHints` passes unchanged; full `go test ./...` is green

### Code Quality

- [x] A-007 Pattern consistency: The edit follows the existing Roster-literal and doc-comment style of `tools.go`; test extension follows `agent_setup_test.go`'s existing assertion style
- [x] A-008 No unnecessary duplication: No builder change, no new helpers — the existing `agentSkillDescription()` appends the value verbatim; fragment pins live only in the test

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
- Propagation verified at plan time (intake §6): `refreshPlacedAgentSkills` (agent_setup.go:381, gated on prior placement) runs at the end of `shll update` (update.go:399); `shll doctor` WARNs on byte-stale placements (doctor.go:384). No propagation code changes.

## Deletion Candidates

- None — this change extends an existing string field value (run-kit's `ProactiveHint`), updates its doc comment, and adds two containment assertions to an existing test. It introduces no new symbols and removes nothing, so it makes no existing code redundant or unused.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Pin exactly the two fragments the intake exemplifies (`"to proxy a local http port"`, `"before opening any file or local port in a browser, read"`) as the test's containment strings | The intake gives these as the canonical examples of the two load-bearing functions; both are substrings of the verbatim-adopted value, distinctive, and neither overlaps existing description text | S:85 R:90 A:95 D:90 |
| 2 | Certain | Assert the fragments against the rendered description (`agentSkillDescription()` output), not `rk.ProactiveHint` directly | The intake allows either; the description is the user-facing surface the whole test already targets, and containment in the description transitively pins the Roster value (verbatim-append is already asserted) | S:80 R:90 A:90 D:85 |

2 assumptions (2 certain, 0 confident, 0 tentative).
