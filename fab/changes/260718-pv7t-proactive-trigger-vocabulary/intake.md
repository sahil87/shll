# Intake: Proactive-Capability Trigger Vocabulary

**Change**: 260718-pv7t-proactive-trigger-vocabulary
**Created**: 2026-07-18

## Origin

Backlog item `[pv7t]` via `/fab-new pv7t` (one-shot invocation, no prior conversation). Raw backlog text:

> Add proactive-capability trigger vocabulary (run-kit visual display + notify) to the placed shll-toolkit Agent Skill description and the `shll skill` roster line. PROBLEM: run-kit has two agent-PROACTIVE capabilities — visual display (iframe windows + /proxy + the Visual Display Recipe; topic page `rk skill display`, run-kit PR #386) and out-of-band notify (`rk notify`) — that an agent should reach for UNPROMPTED when it has visual content to show or needs to ping the human. Today they are invisible at every ambient layer: the placed SKILL.md description (built by agentSkillDescription() in src/cmd/shll/agent_setup.go from Roster.SkillHint) renders run-kit as "tmux sessions (run-kit/rk)" (SkillHint at src/cmd/shll/tools.go:155), so a display-shaped request ("show me this as a diagram/report in the browser") matches nothing; the hop-1 roster line (Roster.Description: "tmux session manager with a web UI") also lacks display/notify vocabulary; the FIRST mention of "show web content visually" is hop-2 (`shll skill run-kit` — the bundle's own "When to use" section). The agent-setup design already names the governing principle (agent_setup.go comment: "the description is the only text in an agent's context BEFORE the skill is invoked, so task-shaped requests must match there, not just tool names") — and since change agst deliberately REJECTED bundle aggregation/per-tool skill placement in favor of the thin bootstrap + runtime two-step (docs/memory/cli/standards-content.md §forward design), the description IS the designed ambient trigger surface: this is a missing-vocabulary bug in it, not a missing mechanism. DISTINCTION WITH TEETH (sprawl guard): reactive tools (wt/idea/tu/hop/fab-kit) live correctly behind the two-step router because the USER'S words name them; only agent-proactive capabilities earn description space, and today that set is exactly run-kit's display + notify. IMPLEMENTATION (preferred): add an optional Roster field (e.g. ProactiveHint string, empty for most tools) rendered by agentSkillDescription() as one additional sentence after the tool clauses — e.g. "Also use proactively — without the user naming a tool — to show the user visual content (HTML, diagrams, reports, a local dev server) in a browser window, or to push a notification to their devices (run-kit)." — mirroring the "Also use proactively when…" trigger idiom Claude Code's own built-in skills use. Keep SkillHint as the reactive task-domain phrase. CONSTRAINTS: the description MUST stay a single line (TestAgentSetup_DescriptionSingleLine); the Roster stays the single source of truth (extend TestRosterSkillHints or add a sibling test pinning run-kit's ProactiveHint); whatever content-hash/staleness marker the self-maintaining refresh uses (PR #50: update refresh + doctor staleness check) must change so `shll update`/doctor re-place the skill on existing machines. ALSO in the same edit: enrich run-kit's Roster.Description so hop-1 browsers see the capability (e.g. "Run-kit — tmux session manager with a web UI; can display web pages/HTML to the user and push notifications (rk stays as an alias)"), and consider one thin "proactive capabilities → `shll skill run-kit`" line in the bootstrap body (agentSkillContent) — body text loads only on activation so it is cheap, but keep the thin-bootstrap genre. VERIFY: after `shll agent-setup`, both ~/.agents/skills/shll-toolkit/SKILL.md and ~/.claude/skills/shll-toolkit/SKILL.md carry the new sentence, and a tool-name-free display-shaped prompt plausibly matches the description.

## Why

1. **The pain point**: run-kit carries the toolkit's only two agent-*proactive* capabilities — visual display (iframe windows + `/proxy` + the Visual Display Recipe, topic page `rk skill display`) and out-of-band notify (`rk notify`) — which an agent should reach for *unprompted* when it has visual content to show or needs to ping the human. Both are invisible at every ambient layer today. The placed `shll-toolkit` SKILL.md description renders run-kit as `tmux sessions (run-kit/rk)` (from `Roster.SkillHint`, `src/cmd/shll/tools.go:155`), so a display-shaped request ("show me this as a diagram in the browser") matches nothing. The hop-1 roster line (`Roster.Description: "Run-kit — tmux session manager with a web UI (rk stays as an alias)"`) also lacks display/notify vocabulary. The first mention of "show web content visually" is hop-2 (`shll skill run-kit`) — which an agent only reaches *after* the skill has already triggered.

2. **The consequence if unfixed**: agents on machines wired via `shll agent-setup` never discover the display/notify capabilities from a task-shaped prompt, so the capabilities go unused precisely in the "proactive" scenarios they were built for. The description is the *designed* ambient trigger surface — change `agst` deliberately rejected bundle aggregation and per-tool skill placement in favor of the thin bootstrap + runtime two-step (`docs/memory/cli/standards-content.md` §forward design) — so there is no other layer where this vocabulary could live.

3. **Why this approach**: this is a missing-vocabulary bug in the existing trigger surface, not a missing mechanism. The fix adds the vocabulary where the design says triggers belong (the description, per the `agent_setup.go` comment: "the description is the only text in an agent's context BEFORE the skill is invoked"), keeps the Roster as the single source of truth (Constitution III), and holds the sprawl guard: reactive tools (wt/idea/tu/hop/fab-kit) stay behind the two-step router because the *user's* words name them; only agent-proactive capabilities earn description space, and today that set is exactly run-kit's display + notify.

## What Changes

### 1. `Tool.ProactiveHint` field (`src/cmd/shll/tools.go`)

Add an optional field to the `Tool` struct:

```go
// ProactiveHint is a complete sentence describing a capability the AGENT should
// reach for unprompted (without the user naming a tool) — the agent-proactive
// trigger vocabulary appended to the generated `shll agent-setup` skill
// description. Empty for every tool except run-kit (sprawl guard: only
// agent-proactive capabilities earn description space; reactive tools stay
// behind the two-step router because the user's words name them). Kept on the
// Roster (Constitution III) so the description cannot drift from the managed set.
ProactiveHint string
```

Only run-kit's Roster entry populates it (see §2 for the value). `SkillHint` stays `"tmux sessions"` — the reactive task-domain phrase is unchanged.

### 2. `agentSkillDescription()` renders the proactive sentence (`src/cmd/shll/agent_setup.go`)

The builder appends each non-empty `ProactiveHint` as one additional sentence after the tool clauses, before the closing two-step teaching pointer. Resulting shape:

```
Use when driving any sahil87 toolkit CLI or shll itself — {clauses}. {ProactiveHint sentence(s)} Run `shll skill` to list the installed tools; run `shll skill <tool>` for that tool's full usage bundle before using it.
```

run-kit's `ProactiveHint` value (the complete sentence, stored verbatim on the roster entry):

```
Also use proactively — without the user naming a tool — to show the user visual content (HTML, diagrams, reports, a local dev server) in a browser window, or to push a notification to their devices (run-kit).
```

This mirrors the "Also use proactively when…" trigger idiom Claude Code's own built-in skills use. The sentence contains no newline and no `: ` sequence, so `TestAgentSetup_DescriptionSingleLine` (single line, unquoted-YAML-safe) keeps passing.
<!-- assumed: ProactiveHint holds the complete rendered sentence verbatim (builder just appends it), rather than a fragment composed into a builder-owned "Also use proactively" preamble — simplest faithful reading of the backlog while exactly one tool has a hint -->

### 3. Enrich run-kit's `Roster.Description` (`src/cmd/shll/tools.go`)

```go
Description: "Run-kit — tmux session manager with a web UI; can display web pages/HTML to the user and push notifications (rk stays as an alias)",
```

This flows to every hop-1 surface that renders `Roster.Description`: `shll list` and the bare `shll skill` glossary (both print `tool.Name\ttool.Description` — verified in `skill.go:104-107` and list.go). Also refresh the `shll list` example output in `README.md` (line ~186) that quotes the old description — per the Toolkit Standards constitution clause, check `docs/site/standards/` for the README-governing standard before editing.

### 4. Thin proactive-capabilities line in the bootstrap body (`agentSkillContent`)

Add one line to the placed SKILL.md body pointing proactive capabilities at the run-kit bundle, keeping the thin-bootstrap genre (body text loads only on activation, so it is cheap), e.g.:

```
Run-kit also has agent-proactive capabilities — visual display in a browser window and push notifications; see `shll skill run-kit`.
```
<!-- assumed: include the body pointer line (backlog said "consider") — activation-cost-only, one line, keeps the thin-bootstrap genre -->

### 5. Tests (`src/cmd/shll/agent_setup_test.go`)

- Add a sibling test to `TestRosterSkillHints` pinning the ProactiveHint contract: run-kit's `ProactiveHint` is non-empty, the generated description contains it, and it precedes the two-step pointer. (`TestRosterSkillHints` itself enforces the *every-tool* SkillHint contract; `ProactiveHint` is optional-by-design, so it gets its own assertions rather than an extension.)
- `TestAgentSetup_DescriptionSingleLine` continues to guard the YAML shape (single line, no `: `) — the new sentence must satisfy it as-is.
- Existing content assertions (`shll skill <tool>` pointer, `shll standards` pointer, no stanza/sentinel wording) continue to pass.

### 6. Staleness / self-maintaining refresh — no work needed (verified)

The backlog constraint "whatever content-hash/staleness marker the refresh uses must change" is satisfied for free: `agentSkillPlacementState` (`agent_setup.go:331-346`) byte-compares each placed file against the running binary's `agentSkillContent`. Any description or body change alters those bytes, so existing placements immediately read as stale — `shll update`'s conditional refresh (`refreshPlacedAgentSkills`, placed-only) re-places them and `shll doctor`'s shll-row staleness check flags them, with zero changes to that machinery.

## Affected Memory

- `cli/agent-setup`: (modify) description builder gains the ProactiveHint sentence; bootstrap body gains the proactive-capabilities pointer line; trigger-vocabulary section updated
- `cli/commands`: (modify) `Tool` struct gains the `ProactiveHint` field; the quoted Roster slice (run-kit's Description + new field) updates
- `cli/list`: (modify) example output quotes run-kit's old Description in two places (table + JSON)

## Impact

- `src/cmd/shll/tools.go` — `Tool` struct (+`ProactiveHint` doc'd field), run-kit Roster entry (Description + ProactiveHint)
- `src/cmd/shll/agent_setup.go` — `agentSkillDescription()` rendering, `agentSkillContent` body line
- `src/cmd/shll/agent_setup_test.go` — sibling ProactiveHint test; existing tests unchanged but re-verified
- `README.md` — refresh the `shll list` example output line quoting run-kit's description (standards check per Constitution "Toolkit Standards" before editing)
- No new subcommands, no new subprocesses, no `internal/proc` surface change — Constitution I/II/V untouched; III reinforced (Roster stays the single source of truth)
- Verify (from backlog): after `shll agent-setup`, both `~/.agents/skills/shll-toolkit/SKILL.md` and `~/.claude/skills/shll-toolkit/SKILL.md` carry the new sentence; a tool-name-free display-shaped prompt plausibly matches the description

## Open Questions

*(none — the backlog entry specifies mechanism, constraints, wording examples, and verification; all residual decisions graded Confident or better below)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Mechanism: optional `ProactiveHint string` on `Tool`, empty for all tools except run-kit, rendered by `agentSkillDescription()` as one additional sentence after the tool clauses | Backlog names this as the preferred implementation; preserves Roster single-source-of-truth (Constitution III) | S:90 R:75 A:90 D:85 |
| 2 | Confident | `ProactiveHint` holds the complete sentence verbatim (builder appends it between the tool clauses and the two-step pointer); run-kit's value = the backlog's example sentence | Simplest faithful reading of "rendered as one additional sentence"; a builder-owned composed preamble is over-engineering while exactly one tool has a hint; sentence verified single-line and `: `-free | S:75 R:85 A:75 D:65 |
| 3 | Certain | Enrich run-kit's `Roster.Description` with display + notify vocabulary (backlog's example wording), flowing to `shll list`, the `shll skill` glossary, and the README example output | Backlog explicitly requires it ("ALSO in the same edit") with the wording given; hop-1 surfaces verified to render `Roster.Description` | S:90 R:90 A:85 D:80 |
| 4 | Confident | Include the thin body pointer line ("proactive capabilities → `shll skill run-kit`") in `agentSkillContent` | Backlog says "consider"; included because body text is activation-cost-only, one line, and keeps the thin-bootstrap genre — easily removed if it reads as sprawl | S:50 R:85 A:65 D:55 |
| 5 | Certain | No staleness-marker work: byte-compare in `agentSkillPlacementState` makes any content change self-propagating via `shll update` refresh + `shll doctor` | Verified in `agent_setup.go:331-346` — placed bytes are compared against the running binary's canonical content; no hash/marker exists to bump | S:85 R:90 A:95 D:90 |
| 6 | Certain | `SkillHint` stays `"tmux sessions"`; no other tool gains a `ProactiveHint` (sprawl guard) | Backlog explicit: reactive phrase unchanged; only agent-proactive capabilities earn description space, and today that set is exactly run-kit's display + notify | S:95 R:85 A:90 D:90 |
| 7 | Confident | Testing via a new sibling test pinning run-kit's ProactiveHint (non-empty + rendered in description) rather than extending `TestRosterSkillHints` | Backlog offers either; `TestRosterSkillHints` enforces an every-tool required-field contract while `ProactiveHint` is optional-by-design — different assertions, separate test | S:70 R:90 A:80 D:70 |

7 assumptions (4 certain, 3 confident, 0 tentative, 0 unresolved).
