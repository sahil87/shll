# Intake: Skill Tutorial/Onboarding Routing

**Change**: 260903-qn14-skill-tutorial-onboarding-routing
**Created**: 2026-09-03

## Origin

Backlog item `[qn14]` (2026-09-03), one-shot via `/fab-new qn14`. Raw input:

> shll-toolkit skill: add tutorial/onboarding routing — description + body line 'when the user asks for a tutorial, tour, or onboarding of run-kit or the dashboard, read rk skill tutorial and follow it'. Today the trigger words live only inside rk skill's topic index, which agents never read unless already driving a toolkit CLI — 'Onboard me to run-kit' pattern-matches Claude Code's generic ONBOARDING.md flow instead (observed 2026-09-02 on run-kit).

Verified at intake time: run-kit ships a `tutorial` topic today — `rk skill topics` prints `code, display, mux, tutorial` (exit 0).

## Why

1. **The pain point**: run-kit ships a `tutorial` topic page, but the words that should route to it — "tutorial", "tour", "onboarding" — appear nowhere in the `shll-toolkit` bootstrap skill. The skill's frontmatter `description` is the only text in an agent's context *before* the skill is invoked, so a request like "Onboard me to run-kit" never activates the skill: on 2026-09-02 it pattern-matched Claude Code's generic ONBOARDING.md flow instead (which produces a repo-onboarding guide — the wrong artifact entirely). The trigger words currently live only inside rk's own topic index, a surface agents reach only when *already* driving a toolkit CLI.

2. **Consequence of not fixing**: The tutorial topic stays undiscoverable from the request that most needs it. Every "give me a tour of run-kit / the dashboard" request either lands on a generic harness flow or forces the user to name the mechanism themselves (`rk skill tutorial`), defeating the point of a routed skill.

3. **Why this approach**: The bootstrap skill already solves exactly this class of problem twice — reactive task-domain clauses (`SkillHint`) and agent-proactive vocabulary (`ProactiveHint`), both single-sourced from the Roster so the description cannot drift from the managed set. Adding the tutorial routing as one more Roster-sourced sentence plus one body line rides the existing mechanism at zero structural cost: no new subcommand, no new file, no new harness wiring.

## What Changes

Both edits land in shll's Go source (the bootstrap skill is a `setup agent` artifact built from the Roster — not a docs-site file), and take effect on the next `shll setup agent` / `shll update` refresh.

### 1. Description trigger vocabulary — extend run-kit's `ProactiveHint` (`src/cmd/shll/tools.go`)

Append one routing sentence to run-kit's existing `ProactiveHint` (currently two sentences: proactive capabilities + the dashboard skill-shadowing counter-instruction):

> When the user asks for a tutorial, tour, or onboarding of run-kit or its web dashboard, read `shll skill run-kit tutorial` and follow it.

This puts the trigger words ("tutorial", "tour", "onboarding") into the frontmatter `description` — the pre-invocation activation surface — via the existing seam: `agentSkillDescription()` appends each non-empty `ProactiveHint` verbatim between the tool clauses and the closing two-step pointer. The output MUST remain a single line (YAML frontmatter value; pinned by `TestAgentSetup_DescriptionSingleLine`).

The `ProactiveHint` doc comment (`tools.go`) is widened to cover routing vocabulary generally — the field is mechanically "extra per-tool description sentences appended verbatim", and this sentence is *reactive* routing rather than reach-for-unprompted vocabulary. The field is NOT renamed (renaming would ripple through the builder, tests, and memory for no behavioral gain).

### 2. Body routing line — extend the run-kit paragraph in `agentSkillContent` (`src/cmd/shll/agent_setup.go`)

Append the same routing instruction to the existing run-kit capabilities paragraph in the skill body (the one ending "…see `shll skill run-kit` (and `shll skill run-kit code` for the editor bridge)"):

> When the user asks for a tutorial, tour, or onboarding of run-kit or its web dashboard, read `shll skill run-kit tutorial` and follow it.

The command form is the composer route `shll skill run-kit tutorial` — consistent with the body's existing `shll skill run-kit` / `shll skill run-kit code` pointers (the backlog's `rk skill tutorial` resolves identically through the `rk` → `run-kit` legacy alias, but the skill teaches the composer front door).

### 3. Tests (`src/cmd/shll/agent_setup_test.go`)

- Extend the description assertions: the built description contains the tutorial-routing sentence (trigger words + the `shll skill run-kit tutorial` pointer); `TestAgentSetup_DescriptionSingleLine` continues to pass with the longer hint.
- Assert the body (`agentSkillContent`) carries the routing line.

## Affected Memory

- `cli/setup`: (modify) The bootstrap-skill section documents `ProactiveHint`'s content and its "two sentences, four jobs" framing plus the body's run-kit paragraph — update for the third (routing) sentence, the widened field semantics, and the new body line.

## Impact

- `src/cmd/shll/tools.go` — run-kit Roster entry's `ProactiveHint` (+ field doc comment).
- `src/cmd/shll/agent_setup.go` — `agentSkillContent` run-kit paragraph.
- `src/cmd/shll/agent_setup_test.go` — description/body content assertions.
- No CLI surface change, no new subcommand (Constitution VII untouched), no docs-site or standards change. The placed skill refreshes on the next `shll setup agent` (or `shll update`'s self-refresh).
- Downstream: none in run-kit — its `tutorial` topic already exists and is verified present.

## Open Questions

- None — the backlog entry carries the routing wording near-verbatim, and the one external precondition (run-kit ships a `tutorial` topic) was verified at intake time.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Description trigger words ride run-kit's existing `ProactiveHint` Roster field, not a new field or a hardcoded builder sentence | Single-sourcing rule: the description is built from the Roster so it cannot drift; the field is the established seam for extra per-tool description sentences | S:70 R:85 A:85 D:80 |
| 2 | Certain | run-kit ships the `tutorial` topic the routing points at | Verified at intake: `rk skill topics` → `code, display, mux, tutorial`, exit 0 | S:90 R:90 A:100 D:95 |
| 3 | Certain | Trigger words are "tutorial", "tour", "onboarding", scoped to run-kit or its web dashboard | Backlog wording carried near-verbatim | S:85 R:85 A:85 D:85 |
| 4 | Confident | The routed command is the composer form `shll skill run-kit tutorial`, not the backlog's literal `rk skill tutorial` | Body precedent: existing pointers use `shll skill run-kit` / `shll skill run-kit code`; the `rk` alias resolves to the same place either way | S:55 R:90 A:75 D:60 |
| 5 | Confident | Body line is appended to the existing run-kit capabilities paragraph rather than a new standalone section | Same subject (run-kit agent guidance); keeps the body thin per the skill's design | S:60 R:90 A:80 D:65 |
| 6 | Confident | `ProactiveHint` keeps its name; only its doc comment widens to cover routing vocabulary | Mechanically the field is "sentences appended verbatim to the description"; a rename ripples through builder, tests, and memory for no behavioral gain | S:50 R:85 A:80 D:60 |

6 assumptions (3 certain, 3 confident, 0 tentative, 0 unresolved).
