# Intake: Skill standard — landed-design note for `shll agent-setup`

**Change**: 260718-fw9d-skill-standard-landed-design-note
**Created**: 2026-07-18

## Origin

One-shot invocation: `/fab-new fw9d` (backlog ID, cold — no prior conversation in session). Raw backlog entry:

> [fw9d] 2026-07-18: Update the skill standard's stale "Forward design: shll agent-setup" paragraph — the design landed, via a different mechanism. docs/site/standards/skill.md:70 still reads "(Planned, not yet built — …)" and describes agent-setup as "aggregate every installed tool's core `<tool> skill` output into the agent's context". Change agst (shipped 2026-07-18, PRs #48/#50) landed `shll agent-setup` as thin-bootstrap SKILLS PLACEMENT (one shll-toolkit skill at ~/.agents/skills + ~/.claude/skills, description roster-driven) + the runtime two-step (`shll skill` glossary → `shll skill <tool>` bundle on demand), explicitly REJECTING context aggregation and per-tool bundle placement (bundles go stale between updates; listing lines multiply) — recorded in docs/memory/cli/standards-content.md §"Forward design — LANDED (change agst)" and cross-linked from docs/memory/cli/agent-setup.md:95. The agst intake itself flags this standard-text edit as a likely small follow-up (intake.md ~line 124). WORK: rewrite the paragraph as a landed-design note — the placement mechanism, the two-step, hook-wiring delegation to `run-kit agent-setup` (already hook-only today) — and REPRICE the budget rationale: the ≤150-line + static-only rules are now motivated by use-time `shll skill <tool>` pulls into a paying context (plus the `shll skill` glossary aggregating roster one-liners), not by N-bundle concatenation into one payload. The memory files are already correct — this is a docs/site/standards edit only; re-run the standards sync + drift-guard (the embedded copies must match byte-for-byte) and ship as a normal docs-type fab change. Cross-ref: pairs with [pv7t] (which relies on the placement design's description-as-trigger-surface being the documented mechanism).

Key decisions were made in the backlog entry itself (it is the distilled output of the agst-change session): rewrite as a landed-design note, reprice the budget rationale, docs-only scope, sync + drift-guard re-run.

## Why

1. **The pain point**: `docs/site/standards/skill.md` § "Forward design: `shll agent-setup`" (lines 68–70) still describes the command as *"(Planned, not yet built — …)"* that will *"aggregate every installed tool's core `<tool> skill` output into the agent's context"*. Change `agst` shipped `shll agent-setup` on 2026-07-18 (PRs #48/#50) with that aggregation mechanism **explicitly rejected** — what landed is thin-bootstrap skills placement plus the runtime two-step. The toolkit's binding producer-facing standard now misdescribes a shipped command.
2. **The consequence if unfixed**: this standard renders on shll.ai (`/shll/standards/skill`) and is served by `shll standards skill` (embedded in the binary) — it is the contract the six other tool repos will read when adopting `<tool> skill`. Producers would design against a rejected mechanism and misread the budget rationale (pricing a one-payload N-bundle concatenation that will never happen). Backlog item `[pv7t]` also relies on the placement design's description-as-trigger-surface being the *documented* mechanism.
3. **Why this approach**: the memory files (`cli/standards-content`, `cli/agent-setup`) already record the landed design accurately — the standard document is the only stale surface. The agst intake itself flagged this edit as the expected small follow-up. Constitution § Toolkit Standards makes `docs/site/standards/` the canonical source; the fix is a canonical-text edit + embed re-sync, nothing more.

## What Changes

### 1. Rewrite § "Forward design: `shll agent-setup`" as a landed-design note (`docs/site/standards/skill.md:68-70`)

Current text (to be replaced in full):

> ## Forward design: `shll agent-setup`
>
> *(Planned, not yet built — recorded here because it is why bundles must stay small and static.)* A future `shll agent-setup` will graduate from `run-kit agent-setup`: it will **aggregate every installed tool's core `<tool> skill` output** — never topic pages — into the agent's context, and **delegate run-kit hook installation to `run-kit agent-setup`** (whose context-injection responsibility will be removed, leaving it to do only hook wiring). When N bundles are concatenated into one context payload, every wasted line is paid N times — which is the whole reason for the static-only rule and the ≤150-line budget above.

Replacement is a **landed-design note** covering, in the standard's existing register:

- **What landed — skills placement, not context aggregation.** `shll agent-setup` places one thin bootstrap Agent Skill (`shll-toolkit`) into the harnesses' global skills directories (`~/.agents/skills/` + `~/.claude/skills/`); the skill's description is roster-driven (front-loads the tool names as trigger words) and its body teaches the runtime two-step. Aggregating bundles into the agent's context — and placing per-tool bundles as skill files — was rejected: placed copies go stale between updates, and per-tool skills multiply listing lines.
- **The runtime two-step.** `shll skill` prints an installed-only glossary (one line per tool); `shll skill <tool>` streams that tool's core bundle on demand, byte-identical from the installed binary — so bundle content stays version-locked by construction.
- **Hook-wiring delegation.** `shll agent-setup` delegates run-kit's dashboard-hook wiring to `run-kit agent-setup`, which is hook-only today (its context-injection responsibility was removed as designed).
- **Why the rules above still hold.** The budget/static motive survives the mechanism change: every `shll skill <tool>` call pulls the core bundle into a paying context, and the glossary lists every installed tool — so a bloated bundle still taxes every conversation that pulls it (see § 2 below).

Retitle the heading **`## Landed design: \`shll agent-setup\``** (the section is no longer a forward design); the anchor becomes `#landed-design-shll-agent-setup`. Update the two in-file links that target the old anchor (lines 45 and 51). Verified: no other file in `docs/site/` or the memory tree links to the standard's `#forward-design-shll-agent-setup` anchor (memory cross-links target `cli/standards-content.md`'s own section, not this document's anchor).

Register constraint: this is a public producer-facing standard — describe the design via the commands (`shll agent-setup`, `shll skill`, `run-kit agent-setup`); do **not** cite internal change IDs (agst) or PR numbers in the standard text (the existing document contains none; that provenance lives in memory).

### 2. Reprice the budget rationale at its three cross-reference sites

The stale "N bundles concatenated into one payload" pricing appears outside the rewritten section and must be updated in the same pass:

- **Line 45** (§ Rules with teeth, "Bounded — ≤150 lines"): currently *"bundles will later be **aggregated** across every installed tool (see [Forward design](#forward-design-shll-agent-setup)); a bloated bundle taxes every conversation that pays for it"*. Reprice: agents pull a bundle into a paying context at use time via `shll skill <tool>`, and the `shll skill` glossary aggregates roster one-liners — a bloated bundle taxes every conversation that pulls it. Link retargets to the renamed anchor.
- **Line 51** (§ Topic pages intro): currently *"The ≤150-line budget prices the toolkit-wide *aggregate* (see [Forward design](#forward-design-shll-agent-setup))"*. Reprice: the budget prices the use-time pull (each `shll skill <tool>` serves exactly one core bundle), so it deliberately does not scale with tool size. Link retargets to the renamed anchor.
- **Line 56** (§ Topic pages bullet): currently *"aggregation (`shll agent-setup`) reads **core bundles only**"*. Correct the mechanism: it is the runtime two-step (`shll skill <tool>`) that serves **core bundles only** — never topic pages. The bullet's invariant is retained verbatim in spirit: *a tool's ambient context cost stays ≤150 lines no matter how many topics it ships*.

No other section of the standard references the forward design (verified by grep); §§ The gap it fills / Precedent / Invocation contract / Content / Name rationale / Adoption / Verifying conformance are untouched.

### 3. Re-run the standards sync + drift guard

The standard is embedded in the shll binary via the committed copy `src/cmd/shll/standards/skill.md`, which must match the canonical file byte-for-byte:

1. Run `scripts/sync-standards.sh` to refresh the embedded copies from `docs/site/standards/`.
2. Run the drift guard: `go test ./cmd/shll -run TestStandardsEmbedMatchesCanonical` (from `src/`) — must pass.
3. Commit the canonical file and the regenerated embedded copy together.

## Affected Memory

- `cli/standards-content`: (modify) minimal hydrate touch — the § "Forward design: `shll agent-setup` — LANDED (change agst)…" section currently frames the *standard document* as still sketching the aggregation mechanism ("the standard first sketched a context-aggregation mechanism… but via a different route"); record that the standard text itself now carries the landed-design note (this change), keeping the historical first-sketched record intact. The backlog states memory is "already correct" — nothing in it becomes false; this is bookkeeping that the stale-text caveat is resolved, not a rewrite.

## Impact

- **Files**: `docs/site/standards/skill.md` (canonical, hand-edited) and `src/cmd/shll/standards/skill.md` (embedded copy, regenerated by the sync script). No Go source changes; no CLI-surface change.
- **Rendered/served surfaces**: `shll.ai/shll/standards/skill` (pulled `docs/site/**` tree) and `shll standards skill` stdout (next release's embed).
- **Tests**: `TestStandardsEmbedMatchesCanonical` (drift guard) is the only affected test — it fails if step 3's sync is skipped, passes after.
- **Scope guard**: docs-type change; no code, no roster, no subcommand behavior. Backlog `[pv7t]` is a separate item (pairs with, not part of, this change).

## Open Questions

*(none — the backlog entry resolves scope, mechanism, and shipping procedure)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Rewrite § Forward design (skill.md:68–70) as a landed-design note covering skills placement, the runtime two-step, and run-kit hook delegation | Backlog `[fw9d]` specifies exactly this WORK; memory `cli/agent-setup` + `cli/standards-content` confirm every fact of the landed design | S:95 R:90 A:95 D:95 |
| 2 | Certain | Reprice the ≤150-line + static-only rationale to use-time `shll skill <tool>` pulls into a paying context (glossary aggregates one-liners only) | Backlog states the REPRICE and its exact motivation verbatim | S:90 R:90 A:90 D:90 |
| 3 | Certain | Re-run `scripts/sync-standards.sh` + `TestStandardsEmbedMatchesCanonical` and ship as a docs-type change | Backlog explicit; sync script, embedded copy, and drift-guard test verified present in repo | S:95 R:90 A:95 D:90 |
| 4 | Confident | Update all three stale cross-reference sites outside the section (lines 45, 51, 56), not only the § body | Backlog says "REPRICE the budget rationale" without enumerating lines; grep of the file shows exactly these three sites carry the stale aggregate pricing — leaving them would contradict the rewrite | S:65 R:85 A:85 D:70 |
| 5 | Confident | Retitle the heading to `## Landed design: …` and retarget the two in-file anchors; old anchor has no external referencers | "Forward design" is false once the note records a landed design; grep verified no external links target `#forward-design-shll-agent-setup` — rename is contained to this file | S:60 R:90 A:85 D:70 |
| 6 | Confident | Keep the standard's public register (no change-IDs/PR numbers in the document); memory gets only a minimal bookkeeping touch at hydrate | Existing standard text cites no internal provenance; backlog says memory is "already correct", so anything beyond a resolved-caveat note would be scope creep | S:65 R:90 A:80 D:70 |

6 assumptions (3 certain, 3 confident, 0 tentative, 0 unresolved).
