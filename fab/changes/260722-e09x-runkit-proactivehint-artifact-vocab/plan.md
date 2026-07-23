# Plan: Extend run-kit ProactiveHint to cover hosted-artifact delivery

**Change**: 260722-e09x-runkit-proactivehint-artifact-vocab
**Intake**: `intake.md`

## Requirements

### CLI: run-kit `ProactiveHint` — hosted-artifact counter-instruction (function c)

#### R1: Function c added — hosted publishing named in reason and action clauses
The run-kit Roster entry's `ProactiveHint` value (`src/cmd/shll/tools.go`, the only non-empty `ProactiveHint`) MUST be extended so the counter-instruction also collides with hosted-artifact publishing: the *reason* clause MUST name hosted publishing forcing the user off the dashboard, and the *action* clause MUST tell the agent to read `shll skill run-kit` before publishing an artifact/hosted page. The final wording MUST contain the load-bearing fragment `publishing an artifact`.

- **GIVEN** a run-kit-managed session where the harness offers an Artifact-style hosted-publishing tool
- **WHEN** the agent is about to publish visual content to a hosted page (e.g. claude.ai) — a delivery path that opens no file and touches no local port
- **THEN** the always-in-context skill description's counter-instruction names that exact action ("publishing an artifact or hosted page") and routes the agent to `shll skill run-kit` for the proxied-iframe recipe

#### R2: Functions a and b survive — existing pinned fragments byte-stable
Sentence 1 of the hint (agent-proactive trigger vocabulary, including function a) MUST be untouched, and the final wording MUST keep both existing pinned test fragments byte-stable: `to proxy a local http port` (function a) and `before opening any file or local port in a browser, read` (function b). `ProactiveHint` MUST remain run-kit-only (the sprawl guard).

- **GIVEN** the extended `ProactiveHint` value
- **WHEN** `agentSkillDescription()` renders the frontmatter description
- **THEN** both existing fragments appear verbatim, sentence 1 is byte-identical to today's, and exactly run-kit carries a `ProactiveHint`

#### R3: Rendering invariants hold — single-line, `: `-free, length-budgeted
The extended value MUST remain a single line with no `: ` sequence (it is spliced into an unquoted YAML scalar — pinned by `TestAgentSetup_DescriptionSingleLine`; `e.g. claude.ai` carries no `: ` and no `https://` URL is used). The net length addition MUST be held to roughly the backlog example's size (~25–30 words) — trim prose, never functions. The `ProactiveHint` field doc comment (`tools.go`) is checked and updated only if the final phrasing invalidates it ("one or more complete sentences" is sentence-count-neutral, so it likely needs no change).

- **GIVEN** the extended `ProactiveHint` value spliced into the generated description
- **WHEN** `TestAgentSetup_DescriptionSingleLine` runs
- **THEN** the description is one line containing no `: ` sequence, and the net addition over the prior value is ~25–30 words

#### R4: `TestRosterProactiveHint` pins function c alongside a and b
`TestRosterProactiveHint` (`src/cmd/shll/agent_setup_test.go`) MUST gain a third pinned load-bearing fragment for function c — `publishing an artifact` — in the fragment containment loop alongside the two existing fragments, and its doc comment MUST describe three functions (adding (c) the hosted-artifact counter-instruction). Everything else in the test stays: the exactly-run-kit sprawl guard, the verbatim-containment check, and the position check (after tool clauses, before the two-step pointer).

- **GIVEN** a future rewording of the `ProactiveHint` that drops the hosted-artifact counter-instruction
- **WHEN** `TestRosterProactiveHint` runs
- **THEN** the missing fragment `publishing an artifact` fails the test, so function c cannot be silently dropped

### Non-Goals

- run-kit SessionStart-hook context injection — the xv71-deferred durable escalation, explicitly rejected by the user 2026-07-22 (messes with user context)
- Any change to sentence 1 of the hint, to `SkillHint`, to the description builder (`agentSkillDescription()` appends the value verbatim), or to any other Roster entry
- Distribution: users re-run `shll agent-setup` to receive the new text — inherent to the placement mechanism, no code work (PR-body line only)

## Tasks

### Phase 2: Core Implementation

- [x] T001 Extend the run-kit Roster entry's `ProactiveHint` string in `src/cmd/shll/tools.go` per the intake's recommended fragment-preserving wording: add the reason clause "and publishing to a hosted artifact page (e.g. claude.ai) forces them off the dashboard" and the action clause "; the same applies before publishing an artifact or hosted page to show the user something." — keeping both existing fragments byte-stable, sentence 1 untouched, single-line and `: `-free. Check the `ProactiveHint` field doc comment and update only if the final phrasing invalidates it. <!-- R1, R2, R3 -->
- [x] T002 Extend `TestRosterProactiveHint` in `src/cmd/shll/agent_setup_test.go`: add the third pinned fragment `"publishing an artifact"` (labeled `(c) hosted-artifact counter-instruction`) to the fragment containment loop, and update the test's doc comment (and the in-test loop comment) from two load-bearing functions to three. Leave the sprawl guard, verbatim-containment, and position checks unchanged. <!-- R4 -->

### Phase 3: Integration & Edge Cases

- [x] T003 Run the scoped tests from `src/`: `go test ./cmd/shll/ -run 'TestRosterProactiveHint|TestAgentSetup_DescriptionSingleLine'`, then the full package suite `go test ./...`; fix and retry on failure (max 3 attempts). <!-- R1, R2, R3, R4 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The rendered description contains the fragment `publishing an artifact`, and the hosted-publishing path is named in both the reason clause (forced off the dashboard) and the action clause (read `shll skill run-kit` before publishing an artifact/hosted page)
- [x] A-002 R2: The fragments `to proxy a local http port` and `before opening any file or local port in a browser, read` appear byte-stable; sentence 1 of the hint is unchanged; exactly run-kit carries a `ProactiveHint`
- [x] A-003 R3: The description remains single-line with no `: ` sequence (`TestAgentSetup_DescriptionSingleLine` passes); the net addition is ~25–30 words; no existing function was trimmed
- [x] A-004 R4: `TestRosterProactiveHint` pins three fragments (a, b, c), its doc comment describes three functions, and the sprawl-guard/verbatim/position checks are intact

### Behavioral Correctness

- [x] A-005 R1: A rewording that drops the hosted-artifact counter-instruction now fails `TestRosterProactiveHint` (fragment c is load-bearing in the test)

### Scenario Coverage

- [x] A-006 R4: Scoped tests (`TestRosterProactiveHint`, `TestAgentSetup_DescriptionSingleLine`) pass, and the full `go test ./...` suite is green

### Code Quality

- [x] A-007 Pattern consistency: The change follows the xv71 pattern exactly — value-only Roster string edit plus pinned-fragment test extension; no builder, subcommand, or subprocess changes
- [x] A-008 No unnecessary duplication: Only `src/cmd/shll/tools.go` and `src/cmd/shll/agent_setup_test.go` are touched; the hint text is single-sourced on the Roster (Constitution III)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change extends an existing Roster field value and adds one pinned test fragment (plus doc-comment text); it introduces no new symbol, branch, or file and makes no existing code redundant or unused.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Adopt the intake's recommended fragment-preserving wording verbatim (semicolon extension of the counter-instruction sentence) rather than the backlog's example (which rewords fragment b) | Intake assumption 3 (Confident) already selected this direction; it is the only offered wording satisfying all hard constraints simultaneously | S:85 R:90 A:90 D:85 |
| 2 | Certain | `ProactiveHint` field doc comment left unchanged | It says "one or more complete sentences" — sentence-count-neutral; the final value is still two sentences (the second extended via `and`/`;` clauses), so nothing is invalidated | S:85 R:95 A:90 D:90 |
| 3 | Certain | Updating the test's in-body loop comment ("both load-bearing functions … drop either" → three functions) counts as part of the intake's doc-comment update, not extra scope | The intake pins the test's doc comment and the fragment; a stale inline comment contradicting the three-fragment loop would fail A-007 pattern consistency | S:75 R:95 A:90 D:85 |

3 assumptions (3 certain, 0 confident, 0 tentative).
