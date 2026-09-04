# Plan: Skill Tutorial/Onboarding Routing

**Change**: 260903-qn14-skill-tutorial-onboarding-routing
**Intake**: `intake.md`

## Requirements

### Bootstrap skill: tutorial/onboarding routing

- **R1**: The generated `shll-toolkit` skill frontmatter description MUST contain the tutorial-routing sentence — trigger words "tutorial", "tour", "onboarding" scoped to run-kit or its web dashboard, pointing at `shll skill run-kit tutorial` — delivered by appending a third sentence to run-kit's Roster `ProactiveHint` (`src/cmd/shll/tools.go`), single-sourced through `agentSkillDescription()`.
  - GIVEN the Roster's run-kit entry, WHEN `agentSkillDescription()` renders the description, THEN the output contains the sentence `When the user asks for a tutorial, tour, or onboarding of run-kit or its web dashboard, read` followed by the `shll skill run-kit tutorial` pointer, positioned between the tool clauses and the closing two-step pointer (the existing ProactiveHint splice point).
  - GIVEN the extended hint, WHEN the description is rendered, THEN it remains a single line containing no `": "` sequence (the unquoted-YAML-scalar invariant pinned by `TestAgentSetup_DescriptionSingleLine`).

- **R2**: The skill body (`agentSkillContent`, `src/cmd/shll/agent_setup.go`) MUST carry the same routing instruction, appended to the existing run-kit capabilities paragraph (the one ending "…see `shll skill run-kit` (and `shll skill run-kit code` for the editor bridge).").
  - GIVEN the placed SKILL.md body, WHEN an agent activates the skill on a tutorial/tour/onboarding request, THEN the body instructs it to read `shll skill run-kit tutorial` and follow it.

- **R3**: The `ProactiveHint` field doc comment (`src/cmd/shll/tools.go`) MUST be widened to cover routing vocabulary generally (the field is mechanically "extra per-tool description sentences appended verbatim"), without renaming the field.
  - GIVEN the doc comment, WHEN a reader consults the field contract, THEN it describes both reach-for-unprompted (proactive) and request-routing sentences.

- **R4**: Tests MUST pin both surfaces: the rendered description contains the routing fragments, and `agentSkillContent` contains the body routing line. Existing invariant tests (`TestAgentSetup_DescriptionSingleLine`, `TestRosterProactiveHint`, `TestRosterSkillHints`) MUST continue to pass.
  - GIVEN `go test ./cmd/shll/`, WHEN the suite runs, THEN the new fragment assertions pass and no existing assertion regresses.

### Non-Goals

- No run-kit changes — its `tutorial` topic already exists (verified at intake: `rk skill topics` → `code, display, mux, tutorial`).
- No new Roster field, no field rename, no docs-site or standards change, no CLI surface change.

### Design Decisions

- **Decision**: The routed command is the composer form `shll skill run-kit tutorial`, not the backlog's literal `rk skill tutorial`.
  **Why**: Matches the body's existing `shll skill run-kit` / `shll skill run-kit code` pointer style; the `rk` legacy alias resolves to the same place.
  **Rejected**: Literal `rk skill tutorial` — works, but teaches a second front door inconsistently with the rest of the skill.
  *Introduced by*: 260903-qn14-skill-tutorial-onboarding-routing

- **Decision**: The description sentence rides `ProactiveHint` even though it is reactive routing, with the doc comment widened rather than the field renamed.
  **Why**: The field is mechanically "sentences appended verbatim into the description"; a rename ripples through builder, tests, and memory for no behavioral gain.
  **Rejected**: New Roster field (`RoutingHint`) — a second splice point in the builder for one sentence; hardcoding in `agentSkillDescription()` — breaks Roster single-sourcing.
  *Introduced by*: 260903-qn14-skill-tutorial-onboarding-routing

## Tasks

### Phase 2: Core Implementation

- [x] T001 Append the tutorial-routing sentence to run-kit's `ProactiveHint` in `src/cmd/shll/tools.go` (exact text: `When the user asks for a tutorial, tour, or onboarding of run-kit or its web dashboard, read `+"`shll skill run-kit tutorial`"+` and follow it.`) and widen the `ProactiveHint` field doc comment to cover routing vocabulary <!-- R1, R3 -->
- [x] T002 [P] Append the same routing sentence to the run-kit paragraph in `agentSkillContent` in `src/cmd/shll/agent_setup.go` <!-- R2 -->

### Phase 3: Integration & Edge Cases

- [x] T003 Extend `src/cmd/shll/agent_setup_test.go`: add the tutorial-routing fragments (e.g. `tutorial, tour, or onboarding` and `shll skill run-kit tutorial`) to `TestRosterProactiveHint`'s load-bearing-fragment checks, and assert `agentSkillContent` carries the body routing line (in `TestAgentSetup_BodyTeachesTwoStepAndStandards` or a sibling); run `go test ./cmd/shll/` <!-- R4 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The rendered description contains the tutorial-routing sentence with trigger words "tutorial", "tour", "onboarding" and the `shll skill run-kit tutorial` pointer, spliced at the existing ProactiveHint position.
- [x] A-002 R2: `agentSkillContent`'s run-kit paragraph ends with the routing instruction pointing at `shll skill run-kit tutorial`.
- [x] A-003 R3: The `ProactiveHint` doc comment describes routing vocabulary as well as proactive vocabulary; the field name is unchanged.

### Behavioral Correctness

- [x] A-004 R1: The description remains a single line with no `": "` sequence (`TestAgentSetup_DescriptionSingleLine` passes unmodified).
- [x] A-005 R4: `go test ./cmd/shll/` passes — new fragment assertions green, no existing assertion regressed.

### Scenario Coverage

- [x] A-006 R1: `TestRosterProactiveHint` still asserts exactly run-kit carries a `ProactiveHint`, verbatim inclusion, and position between clauses and trailer — now including the tutorial fragments.

### Code Quality

- [x] A-007 Pattern consistency: edits follow the existing string-constant/Roster style (no magic strings introduced; the sentence lives in the Roster value, not scattered).
- [x] A-008 No unnecessary duplication: the routing sentence appears exactly twice by design (Roster hint → description; body paragraph), matching the skill's existing description/body duplication pattern.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Body routing line reuses the identical sentence text as the description (not a reworded variant) | One string of truth per surface keeps test fragments simple; the existing body paragraph already mirrors description vocabulary | S:60 R:90 A:85 D:70 |
| 2 | Confident | Test fragments added to the existing `TestRosterProactiveHint` fragment table rather than a new test function | The fragment table is the established pin-against-rewording mechanism for description vocabulary | S:55 R:90 A:85 D:70 |

2 assumptions (0 certain, 2 confident, 0 tentative, 0 unresolved).

## Deletion Candidates

- None — this change adds new functionality (a third `ProactiveHint` sentence, a body routing line, and their test pins) without making any existing code, symbol, or branch redundant.
