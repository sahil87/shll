# Intake: Extend run-kit proactive-hint proxy vocabulary

**Change**: 260721-xv71-runkit-proactivehint-proxy-vocab
**Created**: 2026-07-22

## Origin

Promptless dispatch (fab Create-Intake Procedure, `{questioning-mode} = promptless-defer`) from a synthesized description — the sole source for this intake. The description encodes a completed discussion: an observed skill-shadowing failure, its diagnosis, a decided fix with user-approved draft wording, hard test-pinned constraints, an explicit out-of-scope list, and a recorded (non-blocking) concern.

> **Title**: extend run-kit proactive-hint proxy vocabulary. **Decided change**: extend run-kit's `ProactiveHint` field in shll's tool Roster with (a) proxy trigger vocabulary and (b) a skill-shadowing counter-instruction, using user-approved draft wording (refinement allowed but both functions and all constraints preserved). Stop at intake `ready`; do not activate.

Extends shipped backlog item `[pv7t]` (2026-07-18), which introduced the `ProactiveHint` Roster field and its current run-kit-only sentence. This change is a vocabulary/wording extension of that mechanism, not a new mechanism.

## Why

**Problem (real, observed failure)**: The `shll-toolkit` bootstrap Agent Skill placed by `shll agent-setup` fails to route agents to run-kit's visual-display/proxy recipe under **skill shadowing**. In an observed session, the user asked Claude to "show me a few examples" of UI options; Claude activated a competing content-generation plugin skill (`visual-explainer:generate-web-diagram`, description "Generate a beautiful standalone HTML diagram and open it in the browser"), which carries its own complete delivery path — write HTML to `~/.agent/diagrams/`, then `open` (macOS) / `xdg-open` (Linux). That plugin contains zero mentions of rk/run-kit/proxy/iframe/tmux, so Claude opened the file via `file://` in a local desktop browser: no HTTP server, no `/proxy/<port>/` path, no iframe window in the run-kit dashboard. When the user views the session remotely through run-kit's web dashboard, a local `open` never reaches them at all. The shll-toolkit skill's body (which points at `shll skill run-kit`, whose bundle DOES carry the proxy pattern and the Visual Display Recipe) never loaded, because the harness picked the more specific generation skill.

**Consequence if unfixed**: every generation-style skill (any plugin that both produces content and opens it locally) silently defeats the run-kit delivery path for remote users — the exact users run-kit's dashboard exists for. The failure is invisible to the agent: from its perspective, `open` succeeded.

**Diagnosis (from discussion)**: generation and delivery are orthogonal concerns, but generation skills bundle both, so any generation skill shadows the toolkit skill. The only shll-owned text guaranteed to be in every session's context is the placed skill's frontmatter **description** (descriptions of all installed skills are always listed; bodies load only on activation). Therefore the fix lives in the description — and not merely as more trigger vocabulary (the user's words "show me examples" contain no proxy/remote/port vocabulary, so triggers alone would not have fired), but as a **counter-instruction that collides with the competing skill's action**: a statement that local `open`/localhost URLs may never reach the user, creating an unresolved gap at exactly the moment the agent is about to run `open`, sending it to `shll skill run-kit`.

**Why this approach over alternatives** (alternatives rejected in discussion):
- Patching `visual-explainer` — rejected: third-party plugin; users can't patch it, and the class of shadowing skills is open-ended.
- Changing the bootstrap skill BODY — rejected: the body loads only on activation, which is precisely what shadowing prevents; the body stays thin by design.
- Changing run-kit's own bundle — rejected: different repo, and the bundle is hop-2 (already unreachable in the failure mode).
- A run-kit `agent-setup` session-start hook (durable, deterministic escalation) — deliberately **deferred as a possible follow-up**, explicitly out of scope here.

**Known limitation (recorded, not a blocker)**: the fix is probabilistic — a description line competes with a skill the harness has already committed to and can lose. The new wording roughly doubles the proactive portion of the description, and harness skill descriptions have practical length budgets. The durable escalation (run-kit session-start hook) is the deliberate fallback if this proves insufficient.

## What Changes

### 1. `Roster` run-kit entry — `ProactiveHint` value (src/cmd/shll/tools.go)

The Roster lives in `src/cmd/shll/tools.go`; run-kit's entry is at line 157 (the only entry with a non-empty `ProactiveHint`). Replace the current value:

```go
ProactiveHint: "Also use proactively — without the user naming a tool — to show the user visual content (HTML, diagrams, reports, a local dev server) in a browser window, or to push a notification to their devices (run-kit)."
```

with the user-approved draft wording (adopt verbatim; refinement is allowed but MUST preserve both functions and all constraints below):

```go
ProactiveHint: "Also use proactively — without the user naming a tool — to show the user visual content (HTML, diagrams, reports, a local dev server) in a browser window, to proxy a local http port to the user's browser, or to push a notification to their devices (run-kit). The user may be viewing this session remotely through run-kit's web dashboard, where `open`/`xdg-open` and localhost URLs never reach them — before opening any file or local port in a browser, read `shll skill run-kit` for the proxied-iframe recipe."
```

The two functions, both REQUIRED:
- **(a) Proxy trigger vocabulary** — "to proxy a local http port to the user's browser" — matches requests that DO name proxying/dev servers.
- **(b) Skill-shadowing counter-instruction** — "The user may be viewing this session remotely … before opening any file or local port in a browser, read `shll skill run-kit` for the proxied-iframe recipe." — fires at the moment any competing skill's delivery step (`open`/`xdg-open`/localhost URL) is about to run, regardless of which skill was activated.

No change to `agentSkillDescription()` in `src/cmd/shll/agent_setup.go` — it already appends each non-empty `ProactiveHint` verbatim between the tool clauses and the two-step pointer. No change to `SkillHint` ("tmux sessions") or to run-kit's `Description` (the hop-1 `shll list` line) — the decided scope is the `ProactiveHint` field only.

### 2. `ProactiveHint` field doc comment (src/cmd/shll/tools.go, ~lines 58–67)

The field doc reads "ProactiveHint is a complete sentence describing a capability …". The new value is two sentences; update the doc comment to say sentence(s)/prose appended verbatim, keeping the rest of the contract text (agent-proactive vocabulary, run-kit-only sprawl guard, optional-by-design) intact.

### 3. `TestRosterProactiveHint` (src/cmd/shll/agent_setup_test.go, ~line 400)

The test asserts (i) **exactly run-kit** carries a `ProactiveHint` (sprawl guard), (ii) the rendered description contains `rk.ProactiveHint` **verbatim** (a dynamic containment check against the Roster value — no hardcoded sentence literal in the test today), and (iii) position: after the last tool clause, before the two-step pointer (`Run `shll skill``). Assertions (i) and (iii) stay unchanged. Because (ii) is dynamic, the test passes mechanically with any new wording — so, per Constitution Test Integrity (tests conform to the spec), **extend the test to pin the new spec**: assert the description (or `rk.ProactiveHint`) contains the two load-bearing fragments, e.g. `"to proxy a local http port"` (function a) and `"before opening any file or local port in a browser, read"` (function b), so a future rewording cannot silently drop either function. Update the test's doc comment accordingly.

### 4. Hard constraints (test-pinned invariants — MUST hold)

- The rendered description remains a **single line** with **no `: ` (colon-space) sequence** — it is an unquoted YAML frontmatter scalar; pinned by `TestAgentSetup_DescriptionSingleLine` (src/cmd/shll/agent_setup_test.go:359). The draft wording satisfies both (verified: no newline, no `: `; the backticks, slashes, and em-dashes it contains are safe mid-scalar in YAML plain style, and it introduces no ` #` comment-start sequence).
- Sprawl guard and position assertions in `TestRosterProactiveHint` stay as-is.
- Constitution Test Integrity: tests are updated to the new spec, never the implementation bent to old test text.

### 5. Explicitly out of scope

- The bootstrap skill BODY (`agentSkillContent` in agent_setup.go — including its existing "Run-kit also has agent-proactive capabilities …" line): stays thin; body content changes are a different design surface.
- run-kit's own bundle / topic pages (different repo).
- Patching `visual-explainer` or any third-party plugin.
- A run-kit `agent-setup` session-start hook (the durable escalation) — noted as a possible follow-up, deliberately deferred.

### 6. Propagation (verified — no extra work)

`refreshPlacedAgentSkills` (src/cmd/shll/agent_setup.go:380, called from update.go:399) re-runs `shll agent-setup` as a subprocess at the end of every `shll update` **whenever a prior placement exists** (opt-in gate: `agentSkillPlacementState`), so the new description reaches users' `~/.claude/skills/shll-toolkit/SKILL.md` and `~/.agents/skills/shll-toolkit/SKILL.md` on their next update. `shll doctor` independently WARNs on byte-stale placements (`agentSkillPlacementState`'s byte comparison against the running binary's content — content-hash based, so no marker needs manual bumping). Verify this claim holds during planning; no propagation code changes are expected.

## Affected Memory

- `cli/agent-setup`: (modify) the `ProactiveHint` contract prose (agent-setup.md ~line 66: "holds the complete sentence verbatim", the `TestRosterProactiveHint` description) — update to the two-sentence value, the two-function rationale (proxy vocabulary + shadowing counter-instruction), and the extended test pins.
- `cli/commands`: (modify) the verbatim run-kit Roster snippet (commands.md ~line 89 reproduces the entry including the old `ProactiveHint` string) — refresh to the new value.

## Impact

- **Code**: `src/cmd/shll/tools.go` (one Roster field value + one field doc comment). No builder, no new subcommand (Constitution VII untouched), no subprocess changes (Constitution I untouched), no state (Constitution II untouched).
- **Tests**: `src/cmd/shll/agent_setup_test.go` (`TestRosterProactiveHint` extension; `TestAgentSetup_DescriptionSingleLine` and `TestRosterSkillHints` must keep passing unchanged). Scoped run: `go test ./cmd/shll/ -run 'TestAgentSetup|TestRoster'` from `src/`.
- **Docs/memory**: the two `docs/memory/cli/` files above (hydrate stage).
- **Users**: placed skill descriptions refresh on next `shll update` (or explicit `shll agent-setup`); until then `shll doctor` reports the placement stale. Behavioral effect is probabilistic by design (see Why — known limitation).

## Open Questions

- None — the synthesized description resolves scope, wording, constraints, and out-of-scope boundaries; no decision landed below the ask threshold (see Assumptions).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Single-field change: only run-kit's `ProactiveHint` value (plus its field doc comment) in `src/cmd/shll/tools.go`; `agentSkillDescription()` builder untouched | Explicitly decided in the source description; builder already appends hints verbatim — verified in agent_setup.go:81–101 | S:95 R:90 A:95 D:95 |
| 2 | Certain | Adopt the user-approved draft wording verbatim (no refinement) | Description says refinement allowed but verbatim is the approved baseline; draft verified against both pinned invariants (single line, no `: `, no ` #`) — refining risks constraint drift for no gain | S:90 R:85 A:85 D:80 |
| 3 | Confident | `TestRosterProactiveHint` needs no literal-string swap (its verbatim check is dynamic against the Roster), but is extended to pin the two new load-bearing fragments (proxy vocabulary + counter-instruction) | Source description said "the test's pinned string must be updated" — inspection shows the check is dynamic (agent_setup_test.go:420), so the spec-faithful move is adding fragment pins per Test Integrity; sprawl guard + position assertions stay verbatim | S:70 R:85 A:85 D:70 |
| 4 | Certain | Update the `ProactiveHint` field doc comment from "a complete sentence" to sentence(s)/prose | New value is two sentences; the doc comment would otherwise contradict the field it documents; purely descriptive, zero behavior | S:70 R:90 A:90 D:85 |
| 5 | Confident | Run-kit's Roster `Description` (hop-1 `shll list` line) and `SkillHint` stay unchanged | Decided scope names the `ProactiveHint` field only; the prior change ([pv7t]) already enriched Description with display/notify vocabulary | S:80 R:85 A:80 D:70 |
| 6 | Certain | Out-of-scope set honored: no SKILL.md body redesign, no run-kit bundle change, no visual-explainer patch, no run-kit session-start hook | All four exclusions explicit in the source description (the hook explicitly "deferred, out of scope here") | S:95 R:90 A:95 D:95 |
| 7 | Certain | Propagation requires no code: `shll update`'s conditional refresh re-places the skill; doctor's byte-diff flags staleness | Verified in source (agent_setup.go:340–396, update.go:399) — refresh is gated on prior placement only, not on a manual staleness marker | S:85 R:90 A:95 D:90 |
| 8 | Confident | Accept the description-length concern (proactive portion ~doubles) and the probabilistic-fix limitation without a mitigation task in this change | Source description says record in intake, not a blocker; the durable escalation (run-kit hook) is deliberately deferred as follow-up | S:75 R:80 A:70 D:65 |

8 assumptions (5 certain, 3 confident, 0 tentative, 0 unresolved).
