# Intake: Extend run-kit ProactiveHint to cover hosted-artifact delivery

**Change**: 260722-e09x-runkit-proactivehint-artifact-vocab
**Created**: 2026-07-22

## Origin

Backlog item `[e09x]` (2026-07-22), invoked via `/fab-new e09x` (one-shot, no prior conversation in session — all decisions below come from the backlog entry itself, which records a user-decided fix):

> Extend run-kit ProactiveHint counter-instruction to cover hosted-artifact delivery — follow-up to shipped change 260721-xv71 (which extended backlog [pv7t]). Observed failure 2026-07-22 in a run-kit-managed session (run-kit repo, fab-discuss): user asked 'show me a few examples' of tooltip designs; agent delivered visuals TWICE via the Claude Code Artifact tool (publishes to claude.ai and returns a hosted URL) instead of run-kit's proxied-iframe Visual Display Recipe. xv71's skill-shadowing counter-instruction ('before opening any file or local port in a browser, read shll skill run-kit') never fired because Artifact publishing neither opens a file nor touches a local port — a delivery-path hole in the vocabulary, not a new mechanism gap. DECIDED FIX (wording refinement allowed, both functions MUST survive): in src/cmd/shll/tools.go Roster run-kit entry (the only entry with non-empty ProactiveHint), extend the counter-instruction so it also collides with hosted publishing […] CONSTRAINTS (per xv71 pattern): (1) preserve function a (proxy trigger vocabulary 'to proxy a local http port') and function b (open/xdg-open counter-instruction) — this ADDS function c (artifact/hosted-page counter-instruction); (2) ProactiveHint stays run-kit-only (TestRosterProactiveHint sprawl guard, src/cmd/shll/agent_setup_test.go ~line 400); (3) extend TestRosterProactiveHint to pin a new load-bearing fragment for function c (e.g. 'publishing an artifact') alongside the existing pinned fragments, so future rewording cannot silently drop it; (4) update the ProactiveHint field doc comment in tools.go if sentence-count phrasing needs it; (5) mind the description length budget xv71 recorded — trim prose, not functions. OUT OF SCOPE: run-kit SessionStart-hook context injection (the xv71-deferred durable escalation) — explicitly REJECTED by the user 2026-07-22 (messes with user context); description wording is the chosen mechanism. Users must re-run shll agent-setup to receive the new skill text.

## Why

1. **The pain point**: change xv71 added a skill-shadowing counter-instruction to run-kit's `ProactiveHint` so that an agent about to deliver visual content via a *local* path (`open`/`xdg-open`, localhost URL) is redirected to `shll skill run-kit` for the proxied-iframe recipe. On 2026-07-22 a third delivery path surfaced: the Claude Code **Artifact tool**, which publishes HTML to claude.ai and returns a hosted URL. Artifact publishing neither opens a file nor touches a local port, so xv71's counter-instruction never collided with it — the agent delivered visuals twice via hosted artifacts, forcing a user viewing the session through run-kit's web dashboard to leave the dashboard. This is a **vocabulary hole in the existing mechanism**, not a new mechanism gap: the counter-instruction's trigger surface simply doesn't name hosted publishing.

2. **The consequence of not fixing**: every run-kit-managed session where the harness offers an Artifact-style hosted-publishing tool will keep routing visual delivery to claude.ai instead of the dashboard's proxied iframe. The failure recurs precisely for the users the ProactiveHint exists to serve (remote dashboard viewers), and it already recurred twice in one session.

3. **Why this approach**: the same reasoning as xv71 (recorded in `docs/memory/cli/agent-setup.md` § "the ProactiveHint does two jobs"). The only shll-owned text guaranteed in every session's context is the placed skill's frontmatter *description*; skill bodies load only on activation, which is exactly what shadowing prevents. The durable alternative — run-kit SessionStart-hook context injection — was explicitly **rejected by the user on 2026-07-22** ("messes with user context"). Description wording is the chosen mechanism; the fix remains probabilistic by design, exactly like xv71.

## What Changes

### 1. `src/cmd/shll/tools.go` — extend the run-kit `ProactiveHint` value

The run-kit Roster entry (the only entry with a non-empty `ProactiveHint`, currently at `src/cmd/shll/tools.go:163`) carries this two-sentence value today:

> Also use proactively — without the user naming a tool — to show the user visual content (HTML, diagrams, reports, a local dev server) in a browser window, to proxy a local http port to the user's browser, or to push a notification to their devices (run-kit). The user may be viewing this session remotely through run-kit's web dashboard, where `open`/`xdg-open` and localhost URLs never reach them — before opening any file or local port in a browser, read `shll skill run-kit` for the proxied-iframe recipe.

Extend the counter-instruction so it also collides with hosted publishing (function **c**), while functions **a** (proxy trigger vocabulary) and **b** (open/xdg-open counter-instruction) survive. The backlog's example wording:

> …where open/xdg-open and localhost URLs never reach them, and publishing to a hosted artifact page (e.g. claude.ai) forces them to leave the dashboard — before opening any file or local port in a browser, or publishing an artifact/hosted page to show the user something, read shll skill run-kit for the proxied-iframe recipe.

**Recommended refinement** (wording refinement is explicitly allowed): the backlog example rewords the middle of function b's pinned test fragment (`"before opening any file or local port in a browser, read"` would no longer match), while constraint (3) says the new fragment is pinned *alongside the existing pinned fragments*. Prefer a phrasing that keeps both existing fragments byte-stable, e.g.:

> The user may be viewing this session remotely through run-kit's web dashboard, where `open`/`xdg-open` and localhost URLs never reach them and publishing to a hosted artifact page (e.g. claude.ai) forces them off the dashboard — before opening any file or local port in a browser, read `shll skill run-kit` for the proxied-iframe recipe; the same applies before publishing an artifact or hosted page to show the user something.
<!-- assumed: fragment-preserving wording over backlog's verbatim example — constraint (3) says new fragment lands "alongside the existing pinned fragments"; apply may refine phrasing so long as all three functions each keep a pinned fragment and existing fragments stay byte-stable -->

**Hard constraints on the final wording** (whatever refinement apply lands on):

- Function **a** survives: the fragment `to proxy a local http port` still appears (sentence 1 is untouched).
- Function **b** survives: the open/xdg-open counter-instruction still fires; its pinned fragment `before opening any file or local port in a browser, read` stays byte-stable.
- Function **c** added: hosted/artifact publishing is named both in the *reason* clause (leaving the dashboard) and the *action* clause (before publishing … read `shll skill run-kit`), and contains the fragment `publishing an artifact`.
- Single-line, `: `-free invariant holds (the value is spliced into an unquoted YAML scalar — `TestAgentSetup_DescriptionSingleLine` pins this). Note `e.g. claude.ai` carries no `: ` sequence; keep it that way (no `https://` URLs).
- Length budget (xv71's recorded limitation: "harness skill descriptions have practical length budgets"): keep the net addition to roughly the backlog example's size (~25–30 words). Trim prose, not functions.

### 2. `src/cmd/shll/tools.go` — `ProactiveHint` field doc comment

The field doc comment (`src/cmd/shll/tools.go:64-73`) says "one or more complete sentences" — sentence-count-neutral, so it likely needs **no change**. Check it during apply per backlog constraint (4) and update only if the final wording invalidates any phrasing.

### 3. `src/cmd/shll/agent_setup_test.go` — extend `TestRosterProactiveHint`

The test (~line 395–463):

- **Add** a third pinned load-bearing fragment for function c — `"publishing an artifact"` (backlog-suggested) — to the fragment containment loop alongside the two existing fragments, so a future rewording cannot silently drop the artifact counter-instruction.
- **Update the test's doc comment** (currently "pins the hint's two load-bearing functions … (a) … (b) …") to describe three functions, adding (c) the hosted-artifact counter-instruction.
- Everything else in the test stays: the exactly-run-kit sprawl guard, the verbatim-containment check, and the position check (after tool clauses, before the two-step pointer).

### Out of scope

- **run-kit SessionStart-hook context injection** — the xv71-deferred durable escalation, explicitly rejected by the user 2026-07-22 (messes with user context).
- Any change to sentence 1 of the hint, to `SkillHint`, to the description builder (`agentSkillDescription()` just appends the value verbatim), or to any other Roster entry.
- Distribution: users must re-run `shll agent-setup` to receive the new skill text — this is inherent to the placement mechanism (no code work; worth a line in the PR body).

## Affected Memory

- `cli/agent-setup`: (modify) The Design Decision "the `ProactiveHint` does two jobs" and the description-builder section become three jobs/functions — add function c (hosted-artifact counter-instruction), its pinned fragment `"publishing an artifact"`, the updated hint text, and the 2026-07-22 observed-failure rationale (Artifact tool bypassed both existing trigger surfaces). The rejected-alternatives list gains the explicit user rejection of the SessionStart hook (previously "deliberately deferred as follow-up" — now rejected).

## Impact

- **Code**: `src/cmd/shll/tools.go` (one Roster string value; field doc comment only if needed) and `src/cmd/shll/agent_setup_test.go` (one fragment added to `TestRosterProactiveHint` + its doc comment). No new subcommands, no subprocess changes, no API changes — Constitution I/III/IV/V/VII untouched.
- **Tests**: `TestRosterProactiveHint` extended; `TestAgentSetup_DescriptionSingleLine` and the verbatim/position checks must keep passing with the new value. Scope: `go test ./src/cmd/shll/ -run 'TestRosterProactiveHint|TestAgentSetup_DescriptionSingleLine'` first, then the package.
- **Docs/memory**: `docs/memory/cli/agent-setup.md` at hydrate.
- **Users**: re-run `shll agent-setup` to pick up the new description (refresh also propagates on the next `shll update` per the change-#50 machinery).

## Open Questions

*(none — the backlog entry records a user-decided fix with explicit constraints and an explicit out-of-scope rejection)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Mechanism is description-wording only — extend run-kit's `ProactiveHint`; no SessionStart hook | Backlog records DECIDED FIX; hook explicitly REJECTED by user 2026-07-22 | S:95 R:90 A:95 D:95 |
| 2 | Certain | Functions a and b survive verbatim in function; function c is added; `ProactiveHint` stays run-kit-only (sprawl guard) | Backlog constraints (1) and (2), literal | S:95 R:85 A:95 D:95 |
| 3 | Confident | Final wording keeps the two existing pinned test fragments byte-stable (recommended refinement) rather than adopting the backlog example verbatim (which rewords the middle of fragment b) | Constraint (3) says the new fragment is pinned "alongside the existing pinned fragments"; wording refinement explicitly allowed — apply decides final phrasing under the hard constraints above | S:70 R:85 A:80 D:65 |
| 4 | Certain | New pinned fragment for function c is `"publishing an artifact"` | Backlog-suggested example fragment; trivially adjustable if final wording differs | S:80 R:95 A:85 D:80 |
| 5 | Certain | Single-line `: `-free invariant maintained by the new wording | `TestAgentSetup_DescriptionSingleLine` pins it; recorded in cli/agent-setup memory | S:90 R:90 A:95 D:95 |
| 6 | Certain | Sentence 1 (trigger vocabulary incl. function a) unchanged; extension confined to the counter-instruction sentence | Backlog fix targets "the counter-instruction"; smallest change satisfying constraint (1) | S:80 R:85 A:85 D:75 |
| 7 | Confident | Net length addition held to roughly the backlog example's size (~25–30 words); no trimming of existing functions | Constraint (5): mind xv71's recorded length budget — trim prose, not functions | S:75 R:85 A:75 D:70 |
| 8 | Certain | `change_type` is `fix` (observed-failure follow-up; xv71 shipped as `fix:`) | Consistent with sibling change 260721-xv71 and #72's conventional-commit prefix | S:70 R:90 A:80 D:80 |

8 assumptions (6 certain, 2 confident, 0 tentative, 0 unresolved).
