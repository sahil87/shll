# Plan: Code Bridge Skill Vocabulary

**Change**: 260826-e0gt-code-bridge-skill-vocabulary
**Intake**: `intake.md`

## Requirements

### cli/setup: agent-proactive vocabulary for the Code Bridge

#### R1: `ProactiveHint` names the editor-command capability
run-kit's `Roster.ProactiveHint` (`src/cmd/shll/tools.go`) MUST extend sentence one's reach-for-unprompted list with the code-bridge vocabulary, exactly: `…, to push a notification to their devices, or to act inside the user's code editor — run any VS Code palette command (refresh a PR list, open a diff, focus a view) from the shell with `rk code exec` (run-kit).` Sentence two (the skill-shadowing counter-instruction) MUST remain byte-identical.

- **GIVEN** the Roster
- **WHEN** `agentSkillDescription()` renders the placed SKILL.md description
- **THEN** the description contains `rk code exec` and still contains the three pinned fragments `to proxy a local http port`, `before opening any file or local port in a browser, read`, `publishing an artifact`
- **AND** the description is one line with no `: ` sequence, and the hint falls after the last tool clause and before `Run \`shll skill\``
- **AND** exactly run-kit carries a `ProactiveHint`

#### R2: `Description` one-liner names the capability
run-kit's `Roster.Description` MUST become exactly: `Run-kit — tmux session manager with a web UI; can display web pages/HTML to the user, push notifications, and run VS Code palette commands in its code editor via `rk code exec` (rk stays as an alias)`.

- **GIVEN** `shll list` (or the bare `shll skill` glossary)
- **WHEN** the run-kit row renders
- **THEN** it shows the new one-liner

#### R3: SKILL.md body pointer line names the capability and its topic page
The `agentSkillContent` proactive-capabilities pointer line (`src/cmd/shll/agent_setup.go`) MUST become exactly: `Run-kit also has agent-proactive capabilities — visual display in a browser window, push notifications, and running VS Code palette commands inside the user's code editor (`rk code exec`); see `shll skill run-kit` (and `shll skill run-kit code` for the editor bridge).` No recipe is added to the body.

- **GIVEN** the placed SKILL.md
- **WHEN** an agent reads the body
- **THEN** it sees `rk code exec` and the topic pointer `shll skill run-kit code`, and the two-step + `shll standards` teaching is unchanged

#### R4: Tests pin the new vocabulary
`TestRosterProactiveHint` MUST assert a fourth fragment `rk code exec`; a body test MUST assert `rk code exec` and `shll skill run-kit code` in `agentSkillContent`. Comments describing "three jobs" are updated to four.

- **GIVEN** a future rewording drops the code-bridge vocabulary
- **WHEN** `go test ./cmd/shll/` runs
- **THEN** it fails

#### R5: README stays in sync
`README.md` MUST reflect the new run-kit one-liner in the `shll list` sample output (line ~189) and mention the editor-command capability in the agent-proactive sentence description (line ~262).

- **GIVEN** the README `shll list` sample and `shll setup agent` prose
- **WHEN** read after this change
- **THEN** both match the new roster text

### Non-Goals
- No new Roster field, subcommand, or shll topic page (Constitution VII).
- No `ProactiveHint` for rk-desktop or any other tool.
- No runtime probe for `rk code` availability — run-kit's own `rk skill code` page teaches the gate.

### Design Decisions

#### Extend sentence one rather than add a third sentence
**Decision**: The code-bridge vocabulary joins sentence one's list of proactive capabilities.
**Why**: Keeps the single-line description shorter; the capability is a peer of display/proxy/notify, not a counter-instruction.
**Rejected**: A standalone third sentence — reads cleanly but lengthens an already long YAML line for no trigger gain.
*Introduced by*: 260826-e0gt-code-bridge-skill-vocabulary

## Tasks

### Phase 2: Core Implementation

- [x] T001 Update run-kit `Description` and `ProactiveHint` string literals in `src/cmd/shll/tools.go` to the exact texts in R1/R2 <!-- R1, R2 -->
- [x] T002 [P] Update the proactive-capabilities pointer line in `agentSkillContent` in `src/cmd/shll/agent_setup.go` to the exact text in R3 <!-- R3 -->
- [x] T003 [P] Update `README.md`: run-kit row in the `shll list` sample and the agent-proactive sentence prose in the `shll setup agent` section <!-- R5 -->

### Phase 3: Integration & Edge Cases

- [x] T004 Extend `TestRosterProactiveHint` with fragment `rk code exec` and update its doc comment to four functions; add body assertions for `rk code exec` and `shll skill run-kit code` in `src/cmd/shll/agent_setup_test.go` <!-- R4 -->
- [x] T005 Run `cd src && go test ./cmd/shll/` and `gofmt -l cmd/shll`; fix any failure <!-- R1, R4 -->

## Acceptance

### Functional Completeness
- [x] A-001 R1: `agentSkillDescription()` output contains `rk code exec` and sentence two of the hint is unchanged
- [x] A-002 R2: run-kit `Description` equals the R2 text
- [x] A-003 R3: `agentSkillContent` contains the R3 pointer line
- [x] A-004 R4: `TestRosterProactiveHint` asserts the `rk code exec` fragment; a body test asserts `rk code exec` and `shll skill run-kit code`
- [x] A-005 R5: README `shll list` sample row and `setup agent` prose reflect the new text

### Behavioral Correctness
- [x] A-006 R1: `TestAgentSetup_DescriptionSingleLine`, `TestRosterProactiveHint` (position + three original fragments + exactly-run-kit), `TestRosterSkillHints` all pass
- [x] A-007 R3: `TestAgentSetup_BodyTeachesTwoStepAndStandards` passes (two-step, topic form, `shll standards`, no stanza/sentinel wording)

### Code Quality
- [x] A-008 Pattern consistency: changes are string-literal edits on existing Roster/body surfaces; no new fields, constants, or subprocesses
- [x] A-009 No unnecessary duplication: the body carries a pointer only, not the `rk code` recipe (Constitution III)
- [x] A-010 `go vet`/`gofmt` clean on `src/cmd/shll`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Extend sentence one of the `ProactiveHint` (intake Unresolved #8 resolved) | Peer capability, shorter line; one-string edit if revisited | S:60 R:95 A:70 D:65 |
| 2 | Confident | README needs two edits (intake Tentative #7 resolved — it quotes both the roster line and the proactive sentence) | Verified by grep at plan time | S:85 R:95 A:90 D:90 |
| 3 | Confident | Add body assertions inside `TestAgentSetup_BodyTeachesTwoStepAndStandards` rather than a new test | Same subject (body teaching), one place to read | S:70 R:95 A:85 D:75 |

3 assumptions (0 certain, 3 confident, 0 tentative, 0 unresolved).

## Deletion Candidates

- None — this change adds new vocabulary to existing string literals without making existing code redundant
