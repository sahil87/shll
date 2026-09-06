# Intake: Absorb agentskills.io Guidance into the Skill Standard

**Change**: 260905-q8i5-absorb-agentskills-skill-standard
**Created**: 2026-09-05

## Origin

Promptless dispatch (`/fab-proceed` create-intake, `{questioning-mode} = promptless-defer`), synthesized from a discussion session that compared the toolkit's producer-facing skill-bundle standard (`docs/site/standards/skill.md`) against the agentskills.io open Agent Skills specification (https://agentskills.io/specification).

> Amend `docs/site/standards/skill.md` to absorb three specific things from the agentskills.io specification: (1) description-writing rules for the bootstrap skill, (2) explicit agentskills.io spec-conformance for the placed bootstrap skill including `skills-ref validate` in the conformance checklist and CI, (3) mechanical (failing-test) enforcement of the ≤150-line budget and the `skill topics` contract. Record the explicitly rejected absorptions as design rationale. The core designs deliberately do NOT converge — our version-locking is the better answer to their staleness problem.

Key decisions from the discussion (see Assumptions for grading):
- The two specs are **complementary, not competing**: agentskills.io governs harness-side *placed* skill files (`SKILL.md` with YAML frontmatter in skills directories); our standard governs binary-embedded, version-locked *bundles* served over stdout. The bundle design does not converge toward theirs.
- Exactly three absorptions were chosen; four candidate absorptions were explicitly rejected (their 500-line budget, frontmatter on bundles, the experimental `allowed-tools` field, the `scripts/`/`references/`/`assets/` folder conventions) and are to be recorded in the standard as design rationale, not implemented.
- Implementations in other toolkit repos (run-kit, fab-kit) are out of scope; the standard binds them on their own adoption cadence.

## Why

1. **Activation quality is the highest-leverage gap.** The agentskills.io activation model is description-driven: the `description` frontmatter is the only text in an agent's context *before* a skill is invoked, so it must state what the skill does AND when to use it, with trigger keywords, in ≤1024 characters. Our standard's "Landed design: `shll setup agent`" section says the `shll-toolkit` bootstrap skill's description is roster-driven but gives **no writing rules** — the craft that makes the placed skill actually fire (front-loaded tool names, task-shaped trigger phrases, what+when structure) lives only in `agent_setup.go` comments. Uncodified, it erodes on the next edit.
2. **The placed skill is read by many clients.** `shll setup agent` places the bootstrap skill at `~/.agents/skills/` and `~/.claude/skills/` — paths read by agentskills.io-compatible clients (Claude Code, Codex, Gemini CLI, Cursor, OpenCode, and any future adopter of the open standard). A placed file that violates the spec (invalid frontmatter, bad name, oversized description) silently fails to load on some clients. Nothing in our standard today *requires* spec conformance for the placed artifact, and nothing validates it mechanically.
3. **Prose checklists drift.** The standard's "Verifying conformance" section is a prose checklist; only byte-identity is pinned by a drift-guard test today (per the standard's text). Their spec ships a reference validator (`skills-ref validate`, github.com/agentskills/agentskills); we should require the cheap-to-automate checks (line budget, `skill topics` contract, placed-skill validity) to be failing tests, not review items — otherwise conformance decays between audits.

If we don't do this: the bootstrap skill's activation quality depends on one Go file's comments, cross-client compatibility of the placed skill is untested, and the standard's checklist keeps items that nothing enforces.

Why this approach over alternatives: converging the bundle format onto agentskills.io (frontmatter'd bundles, their 500-line budget, their folder layout) was considered and rejected — bundles are stdout payloads pulled per-conversation, version-locked inside the binary, which is a strictly better answer to the staleness problem their placed-file format has. Absorption is limited to the three places where their spec genuinely covers ground ours doesn't.

## What Changes

All edits target `docs/site/standards/skill.md` (canonical) plus its consequences in this repo. The standard is one of the nine documents embedded in the shll binary (`shll standards skill`), so the embedded copy must be refreshed (see §5).

### 1. Description-writing rules for the bootstrap skill (highest value)

Add a short subsection under **"Landed design: `shll setup agent`"** codifying how the `shll-toolkit` bootstrap skill's `description` frontmatter must be written. Rules to codify (matching what `agentSkillDescription()` in `src/cmd/shll/agent_setup.go` already does, so the standard captures existing practice):

- **Tool names front-loaded** — every roster tool's name (and legacy alias, e.g. `run-kit/rk`) appears as trigger vocabulary.
- **Task-shaped trigger phrases, not just nouns** — each tool contributes a task-domain phrase (today: `Roster.SkillHint`, e.g. "git worktrees", "tmux sessions"), because agents match task-shaped requests ("create a worktree"), not tool names alone.
- **What + when structure** — the description states what the skill does AND when to use it (the agentskills.io activation contract: the description is the only text in context before invocation).
- **≤1024 characters** — the agentskills.io cap on the `description` field, enforced strictly (no exemption; see §6 for how the currently oversized description is brought under the cap in this change).
- **Description carries triggers; body carries operations** — the description is activation vocabulary (what + when); operational/recipe prose belongs in the skill body, which is read at activation.

### 2. Spec-conformance of the placed bootstrap skill

In the same "Landed design" area, make agentskills.io conformance an **explicit requirement** for the placed skill (the artifact written to `~/.agents/skills/shll-toolkit/SKILL.md` and `~/.claude/skills/shll-toolkit/SKILL.md`):

- Valid YAML frontmatter with the portable `name` + `description` fields.
- **Name constraints**: 1–64 chars, lowercase alphanumeric + hyphens, no leading/trailing/consecutive hyphens (`^[a-z0-9]+(-[a-z0-9]+)*$`), and MUST match the skill directory name. (`skillDirName = "shll-toolkit"` already satisfies this; the requirement pins it.)
- **Description ≤1024 chars.**
- Add **`skills-ref validate`** (the reference validator from github.com/agentskills/agentskills) to the standard's **"Verifying conformance"** checklist for tools that place skills, and — where applicable — wire it into CI. For this repo that means validating the generated `shll-toolkit` SKILL.md content (CI is `.github/workflows/ci.yml`: gofmt, go vet, go test today; if `skills-ref` is not practically installable in CI, equivalent checks land as Go tests — the rules are mechanical).

Scope note: only the **placed** skill is bound by agentskills.io; bundles (`<tool> skill` stdout) explicitly are not (see §4).

### 3. Mechanical validation of bundles

Extend the standard — "Rules with teeth" and "Verifying conformance" — to require that two currently prose-only checks are **enforced by a failing test, not a review item**, in each adopting repo:

- The **≤150-line budget** for the core bundle and each topic page.
- The **`skill topics` contract** (reserved enumeration topic: one name per line to stdout, stderr empty, exit 0; empty output for a topic-less tool; no content topic named `topics`).

The standard mandates the **outcome** (a test fails when violated), not the mechanism — extending the existing drift-guard test or adding a small shared conformance test both conform. shll is already the reference implementation for its own bundle: `src/cmd/shll/skill_test.go` `TestSkillEmbedMatchesCanonical` pins byte-identity of `docs/site/skill.md` AND enforces the 150-line budget, and the `shll skill shll topics` tests pin the reserved-topic contract. Implementation work in this repo is therefore an audit-and-fill (confirm both checks exist and cover the standard's wording; add only what's missing), plus the standard text.

### 4. Rejected absorptions recorded as design rationale (not tasks)

Add a short "deliberately not absorbed" passage (placement flexible — e.g. near "The gap it fills" or a dedicated subsection) recording why the following agentskills.io features do NOT apply to bundles:

- **Their 500-line budget** — ours is deliberately tighter (≤150): bundles are pulled per-conversation via `shll skill <tool>`, so every line taxes every conversation that pulls it.
- **Frontmatter on the bundles themselves** — bundles are stdout payloads, not placed files; there is no loader to read frontmatter.
- **The experimental `allowed-tools` field** — not applicable to a stdout payload.
- **The `scripts/`/`references/`/`assets/` folder conventions** — bundles defer executable behavior to the tool itself (the binary is the "script").

Also state the complementary-not-converging relationship explicitly: agentskills.io governs the *placed* harness-side format (which our bootstrap skill conforms to, §2); the bundle genre stays binary-embedded and version-locked — the staleness problem their placed files have is exactly what version-locking solves.

### 5. Embedded-copy refresh (mechanical consequence)

`docs/site/standards/skill.md` is embedded in the binary: `scripts/sync-standards.sh` copies it to `src/cmd/shll/standards/skill.md`, and `TestStandardsEmbedMatchesCanonical` (in `src/cmd/shll/standards_test.go`) fails the build on divergence. After amending the canonical file, run `scripts/sync-standards.sh` and commit both copies.

### 6. shll's own conformance follow-through: compress + relocate the bootstrap description (resolved)

Constitution § Toolkit Standards binds this repo to its own standards. The name-rule and frontmatter-validity requirements of §2 are already satisfied by `agent_setup.go`; the ≤1024 description cap is **not** — the roster-generated description measures ~1.3KB (run-kit's `ProactiveHint` alone is ~900 chars of deliberately added trigger/operational vocabulary from changes 260721-xv71, 260722-e09x, 260826-e0gt). Resolution (user decision — **compress + relocate**), in scope for THIS change:

- Rework the `shll-toolkit` bootstrap-skill description generation (`agentSkillDescription()` / `Roster` hints in `src/cmd/shll/agent_setup.go`) to fit **≤1024 chars**: keep prioritized what+when trigger vocabulary — tool names front-loaded, task-shaped trigger phrases — in the description.
- Move operational/recipe prose (e.g. the "read `shll skill run-kit` for the proxied-iframe recipe before opening a browser"-style instructions) into the placed skill's **body**, which is read at activation; the body content is generated alongside the description in `setup agent`.
- Compression prioritizes **trigger coverage over prose completeness**: preserve the highest-value proactive triggers from the ProactiveHint vocabulary shipped by 260721-xv71, 260722-e09x, 260826-e0gt.
- Add a **≤1024-char test** on the generated description in `agent_setup_test.go` (the mechanical enforcement §2 requires).

Rejected: softening the cap to a SHOULD in the standard (undermines strict cross-client conformance) and deferring compression to a follow-up change (leaves a known self-conformance gap).

## Affected Memory

- `cli/standards-content`: (modify) The skill standard's contract gains description-writing rules, placed-skill agentskills.io conformance + `skills-ref validate`, mechanical-enforcement requirements, and recorded rejected absorptions.
- `cli/setup`: (modify) `setup agent`'s placed-skill contract (description rules, ≤1024 cap, spec conformance) — including the compress+relocate rework: trigger vocabulary in the description, operational prose in the skill body.
- `cli/standards-conformance`: (modify) shll's conformance state vs. the amended skill standard (new checks, any new deferrals).
- `ci/…`: (possibly new/modify) only if a `skills-ref validate` (or equivalent) step lands in `ci.yml`; no memory file covers `ci.yml` today.

## Impact

- `docs/site/standards/skill.md` — primary edit (currently 93 lines; sections touched: "Rules with teeth", "Topic pages", "Landed design: `shll setup agent`", "Verifying conformance", plus a rejected-absorptions passage).
- `src/cmd/shll/standards/skill.md` — synced embedded copy (`scripts/sync-standards.sh`).
- `src/cmd/shll/agent_setup.go` + `agent_setup_test.go` — description compression to ≤1024 chars (trigger vocabulary kept, operational prose relocated to the generated skill body) plus a ≤1024-char test (`TestAgentSetup_DescriptionSingleLine` exists; no length test today).
- `src/cmd/shll/skill_test.go` — audit-and-fill for §3 (budget test exists at 150; topics-contract tests exist).
- `.github/workflows/ci.yml` — possible `skills-ref validate` step ("where applicable").
- Other toolkit repos (run-kit, fab-kit, etc.) — **out of scope**; bound on their own adoption cadence.
- No runtime behavior change to `shll skill` / `shll standards` serving paths.

## Open Questions

- ~~The roster-generated bootstrap-skill description (~1.3KB) exceeds the agentskills.io ≤1024-char cap this change absorbs.~~ **Resolved — compress + relocate** (user decision, 2026-09-05): the standard enforces the ≤1024 cap strictly (no exemption, no softening to SHOULD), and this change reworks the roster-generated description to fit — prioritized what+when trigger vocabulary (front-loaded tool names, task-shaped trigger phrases) stays in the description; operational/recipe prose (e.g. the "read `shll skill run-kit` for the proxied-iframe recipe before opening a browser" instructions) moves into the skill BODY, which is read at activation. Compression prioritizes trigger coverage over prose completeness, preserving the highest-value proactive triggers from the ProactiveHint vocabulary shipped by 260721-xv71, 260722-e09x, 260826-e0gt. Rejected: softening the cap to a SHOULD (undermines strict cross-client conformance); deferring compression to a follow-up change (leaves a known self-conformance gap). <!-- clarified: >1024-char description conflict resolved by user — compress description to ≤1024 + relocate operational prose to skill body, both in scope for this change -->

## Clarifications

### Session 2026-09-05 (auto mode — deferred-row resolution)

| # | Action | Detail |
|---|--------|--------|
| 11 | Changed | User decision: **compress + relocate** — strict ≤1024 cap (no exemption/softening); description keeps prioritized what+when trigger vocabulary (tool names front-loaded, task-shaped phrases; ProactiveHint triggers from xv71/e09x/e0gt preserved by priority); operational/recipe prose moves to the skill body. Description rework + ≤1024 test in scope for this change. Rejected: SHOULD-softening, follow-up deferral. All four SRAD dimensions re-scored on resolution (promptless-deferred row). |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope is this repo's standard (+ its consequences here) only; run-kit/fab-kit implementations excluded, bound on their own adoption cadence | Discussed — explicitly scoped in the conversation | S:95 R:90 A:95 D:95 |
| 2 | Certain | Absorb exactly the three discussed items; record the four rejected absorptions (500-line budget, bundle frontmatter, `allowed-tools`, folder conventions) as design rationale, not tasks | Discussed — decisions made with specific values | S:95 R:85 A:90 D:90 |
| 3 | Certain | Core designs do not converge: bundles stay binary-embedded, version-locked, frontmatter-free stdout payloads | Discussed — version-locking judged the better answer to their staleness problem | S:95 R:90 A:95 D:95 |
| 4 | Confident | Description-writing rules land as a subsection under "Landed design: `shll setup agent`", codifying existing `agentSkillDescription()` practice (front-loaded names, task-shaped phrases, what+when, ≤1024) | Discussed placement; exact wording is agent-authored from verified code behavior | S:75 R:85 A:80 D:75 |
| 5 | Certain | Refresh the embedded copy via `scripts/sync-standards.sh`; `skill.md` is one of the nine embedded standards drift-guarded by `TestStandardsEmbedMatchesCanonical` | Verified in repo — script and test both name skill.md | S:90 R:90 A:100 D:100 |
| 6 | Certain | shll ships its own bundle (`shll skill shll` serves embedded `docs/site/skill.md`, 56 lines) and already enforces the 150-line budget + topics contract in `skill_test.go`; §3 in this repo is standard text + audit-and-fill | Verified in repo — `TestSkillEmbedMatchesCanonical` caps at 150; `shll skill shll topics` tests exist | S:80 R:80 A:85 D:75 |
| 7 | Confident | `skills-ref validate` becomes a "Verifying conformance" checklist item for placed skills, with CI wiring "where applicable"; if the validator isn't practically installable in this repo's CI, equivalent Go-test checks satisfy the intent | Discussion said "and, where applicable, CI"; validator install mechanics resolvable at apply | S:60 R:85 A:60 D:60 |
| 8 | Confident | shll's own placed-skill conformance work is in scope (Constitution § Toolkit Standards self-binding); name/frontmatter rules already pass, description work gated on row 11 | Constitution mandates self-conformance; extent depends on the deferred resolution | S:70 R:75 A:85 D:70 |
| 9 | Confident | The standard mandates mechanical enforcement as an outcome (a failing test), leaving mechanism (extended drift-guard vs. shared conformance test) to each adopting repo | Discussed both mechanisms without choosing; outcome-not-mechanism keeps per-repo freedom | S:65 R:85 A:75 D:65 |
| 10 | Confident | Change type is `docs` — primary intent is a standard amendment; code edits are enforcement adjuncts | Discussion: "likely docs/standard-flavored"; taxonomy front-runner clear | S:70 R:90 A:80 D:70 |
| 11 | Certain | >1024-char conflict resolved as **compress + relocate**: strict ≤1024 cap in the standard (no exemption); description reworked in this change to keep prioritized what+when trigger vocabulary (front-loaded tool names, task-shaped phrases — trigger coverage over prose completeness, preserving the highest-value ProactiveHint triggers from xv71/e09x/e0gt); operational/recipe prose relocated to the skill body (read at activation) | Clarified — user chose compress+relocate; rejected cap-softening and follow-up deferral. Re-scored all four dimensions on resolution (deferred row): decision is explicit and detailed (S), a description/body text edit revisable by a later change (R), roster code + shipped hint vocabulary give the agent everything needed to execute (A), one chosen interpretation (D) | S:95 R:80 A:85 D:90 |

11 assumptions (6 certain, 5 confident, 0 tentative, 0 unresolved).
