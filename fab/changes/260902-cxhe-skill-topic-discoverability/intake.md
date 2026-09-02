# Intake: Skill Topic Discoverability

**Change**: 260902-cxhe-skill-topic-discoverability
**Created**: 2026-09-02

## Origin

Promptless dispatch (`/fab-proceed` create-new path, `{questioning-mode} = promptless-defer`) from a synthesized change description. Raw input:

> The toolkit `skill` standard (canonical at `docs/site/standards/skill.md` in this repo) mandates topic-page discoverability in two places — the core bundle's topic index, and the unknown-topic error naming valid topics on stderr — and rk conforms to both. But the places a user/agent looks *before* pulling a 150-line bundle have no enumeration: `rk skill --help` mentions topics only generically ("Pass a topic (e.g. `run-kit skill display`)") without naming them, and there is no machine-readable way to enumerate a tool's topics. Agreed changes (both go in): (1) a help-text topic-enumeration MUST for tools that ship topic pages; (2) a reserved `topics` topic (`<tool> skill topics`) printing the tool's topic names one per line, raw to stdout, exit 0.

The description carried explicit agreed decisions, rejected alternatives, and two open design decisions; per the promptless-defer contract the open decisions are recorded below as Unresolved rows (`Deferred — promptless dispatch`), not asked.

## Why

1. **The pain point**: Topic pages are the skill standard's mechanism for depth beyond the ≤150-line core bundle, but their discoverability only works *after* you've pulled the core bundle (its topic index) or *after* you've guessed wrong (the unknown-topic stderr error). The surfaces a user or agent consults *before* paying the 150-line context cost — `<tool> skill --help` — name no topics at all (rk's says only "Pass a topic (e.g. `run-kit skill display`)"). And there is **no machine-readable way to enumerate a tool's topics**: a script or harness that wants the topic list must parse a prose topic index out of the core bundle.

2. **Consequence of not fixing**: Topic depth stays invisible from the cheap surfaces. Agents either pull full bundles to learn what topics exist (defeating the context-economy design), guess topic names and burn a round-trip on the error path, or never discover topics at all. Tooling that wants to enumerate topics (docs pipelines, conformance checks, harness integrations) has no contract to build on.

3. **Why this approach**: Both additions ride existing surfaces at zero marginal cost. Topics are embedded at build time, so a static help-text line and a static name list are free — no runtime lookups, consistent with the standard's static-only posture. The reserved-topic form (vs. a `--list` flag) was chosen specifically because shll's composer passthrough (`shll skill <tool> <topic>`) forwards two positional args verbatim today, so `shll skill run-kit topics` works through the composer with **zero shll changes** — whereas a flag would be intercepted by shll's own cobra flag parsing. And per Constitution VII (minimal surface), the reserved topic adds **no new subcommand** — it rides the existing `skill [topic]` surface.

**Rejected alternatives** (recorded from the discussion):

- **`--list` flag**: breaks the shll composer passthrough — cobra in shll would eat the flag (fixing that needs `DisableFlagParsing` or flag whitelisting in shll); also redundant with help-text enumeration for humans.
- **Topic names in shll's hardcoded roster**: version skew by construction — a tool's topics change on its release cadence, not shll's.
- **Annotating shll's bare-glossary lines with per-tool topics**: requires shll to spawn a subprocess per installed tool at glossary time, changing the glossary's deliberately-cheap (PATH-probe-only) cost contract. Per-tool discovery is the right layer.

## What Changes

Both agreed amendments land in the canonical standard `docs/site/standards/skill.md` (this repo owns it; it renders on shll.ai and is embedded in the shll binary via `shll standards`).

### 1. Help-text topic enumeration (MUST) — amend the topic-pages section

Add to the standard's "Topic pages (large-scope tools)" section: a tool that ships topic pages **MUST enumerate its valid topic names in the `skill` subcommand's help text** — e.g. a `Topics: code, display, mux, tutorial` line in the long help. <!-- assumed: the `Topics: <comma-separated names>` line is presented as the example form ("e.g."); the mandate is that the names appear in the skill subcommand's help text, not an exact format -->

Constraints carried into the standard's wording:

- Topics are embedded at build time, so the enumeration is **static and free** — it MUST NOT require runtime lookups. (Help text is not the bundle, so the static-only rule isn't violated either way, but the enumeration must be static regardless.)
- The mandate binds only tools that ship ≥1 topic page — a core-bundle-only tool's help text is unaffected.

### 2. Reserved `topics` topic — machine-readable enumeration

Amend the standard to define **`<tool> skill topics` as a reserved topic name** that prints the tool's topic names, **one per line, raw to stdout, exit 0**.

- The standard declares `topics` **reserved in every tool's topic namespace**: no tool may ship a content topic named `topics`.
- Rationale recorded in the standard (or its name-rationale style): the positional-reserved-topic form composes through `shll skill <tool> topics` with zero shll changes, where a `--list` flag would be eaten by the composer's own flag parsing.
- The output is static by construction (the topic set is embedded at build time). It otherwise follows the standard's uniform invocation contract: raw output to stdout, stderr empty on success, exit 0. <!-- assumed: stderr-empty-on-success carries over from the standard's existing invocation contract; not separately discussed -->
- Ordering of the printed names is left to the tool (e.g. matching its core bundle's topic index order); the standard does not mandate an order. <!-- clarified: user confirmed 2026-09-02 — ordering stays tool-chosen -->
- The reserved `topics` name does NOT appear in the `Topics:` help line or the core bundle's topic index — those enumerate content topics only; `topics` is a machine affordance discovered from the standard itself. <!-- clarified: user confirmed 2026-09-02 — content-topics-only enumeration -->
- **Applicability: the reserved topic is mandated for ALL adopting tools.** A topic-less tool prints **empty stdout + exit 0** for `<tool> skill topics` — "what topics do you have?" always has a scriptable answer, keeping the seam uniform for tooling. <!-- clarified: user chose 2026-09-02 — all adopting tools, empty-output-exit-0 for topic-less -->

### 3. Verifying-conformance checklist additions

Extend the standard's "Verifying conformance" section (the topic-pages bullet) with checks for both mandates, in the style of the existing items:

- The `skill` subcommand's help text names every shipped topic.
- `<tool> skill topics` prints the shipped topic names one per line, raw to stdout, stderr empty, exit 0.
- No content topic is named `topics`.

(Exact checklist wording is generation-time detail; the three checks above are the required substance.)

### 4. Embedded-copy sync (mechanical, this repo)

`docs/site/standards/skill.md` is embedded in the shll binary via the committed copy `src/cmd/shll/standards/skill.md`, refreshed by `scripts/sync-standards.sh` and pinned by the `TestStandardsEmbedMatchesCanonical` drift guard. Any amendment to the canonical file MUST re-run the sync so the drift guard passes — even a "doc-only" version of this change touches the Go package tree.

### 5. shll's own conformance (in scope) <!-- clarified: user chose 2026-09-02 — include shll conformance in this change -->

shll itself ships **zero topic pages**: today `shll skill shll <anything>` is a usage error (exit 2, `shll skill: shll ships no topic pages (unknown topic %q)` — `skillNoTopicsFmt` in `src/cmd/shll/skill.go`). With the reserved topic mandated for ALL adopting tools (#11 resolved), shll's own conformance ships in this change:

- **`shll skill shll topics`**: prints **empty output + exit 0** (a change to `writeSkillTopic`'s shll-self branch; other topic names keep today's exit-2 no-topics usage error). Tests in `src/cmd/shll/skill_test.go` updated accordingly.
- **shll's own `skill --help`** (`newSkillCmd` Long text in `src/cmd/shll/skill.go`): mention the reserved `topics` topic (e.g. that `shll skill <tool> topics` lists a tool's topics). The help-text enumeration MUST itself does not bind shll (zero content topics shipped).
- `docs/site/skill.md` (shll's own bundle, + its synced embed `src/cmd/shll/skill/skill.md` and `TestSkillEmbedMatchesCanonical`): mention `shll skill <tool> topics` where the bundle teaches the two-step discovery, if it fits the ≤150-line budget.

### 6. Cross-reference consistency (verified at intake)

Constitution's Toolkit Standards clause binds this repo to `docs/site/standards/`. A grep of the other standards files at intake time found: `principles.md` references the skill standard generically (companion-standard table, principle №3/№10 prose — no topic-page detail, no touch expected); `help-dump.md` and `readme-extraction.md` contain **no** references to the skill standard. The consistency sweep is in scope but is expected to conclude "no touches needed" outside `skill.md` itself.

## Affected Memory

- `cli/standards-content`: (modify) the skill standard's contract summary gains the two discoverability mandates (help-text topic enumeration MUST; reserved `topics` topic, one-name-per-line stdout, exit 0, namespace reservation) and the --list-vs-positional composition rationale.
- `cli/standards-conformance`: (modify) shll's conformance posture against the amended skill standard — whether `shll skill shll topics` / the skill help text conform, or why the mandates don't bind a zero-topic tool (per the deferred-decision outcome).
- `cli/skill`: (modify) the `shll skill shll <topic>` contract changes — `topics` now prints empty stdout + exit 0 (other unknown topics keep exit 2), and the help Long text gains the reserved-topic mention.

## Impact

- **Canonical standard**: `docs/site/standards/skill.md` — topic-pages section, possibly name-rationale-adjacent prose for the reservation, Verifying conformance checklist.
- **Embedded copy**: `src/cmd/shll/standards/skill.md` via `scripts/sync-standards.sh`; `TestStandardsEmbedMatchesCanonical` (`src/cmd/shll/standards_test.go`) enforces the sync.
- **Contingent code**: `src/cmd/shll/skill.go` (`newSkillCmd` Long text; `writeSkillTopic` shll-self branch; `skillNoTopicsFmt`) + `src/cmd/shll/skill_test.go`; possibly `docs/site/skill.md` + `src/cmd/shll/skill/skill.md` (shll's own bundle + embed).
- **Out of scope**: rk / fab-kit / every other tool's implementation of the two mandates — phased per-repo on each tool's release cadence, like every other standard rollout (the standard's own Adoption section already establishes this posture).
- **No new subcommand**, no roster change, no composer change — `shll skill <tool> topics` already works through the existing two-positional passthrough (verified against `writeSkillTopic`: the topic is appended verbatim to the tool's skill argv).

## Open Questions

*(None — both questions resolved in the 2026-09-02 clarification session; see `## Clarifications`.)*

- ~~Reserved-topic applicability~~ — **Resolved**: mandated for ALL adopting tools; a topic-less tool prints empty stdout + exit 0 (`shll skill shll topics` changes accordingly).
- ~~shll code scope~~ — **Resolved**: shll's own conformance (skill.go, tests, possibly bundle) ships in this change; other tools adopt on their own cadence.

## Clarifications

### Session 2026-09-02

| # | Action | Detail |
|---|--------|--------|
| 10 | Confirmed | Content-topics-only enumeration; ordering tool-chosen |
| 11 | Changed | "Reserved `topics` mandated for ALL adopting tools — topic-less tool prints empty stdout + exit 0" |
| 12 | Changed | "shll's own conformance (skill.go, tests, possibly bundle) ships in this change" |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Help-text topic enumeration is a MUST in the standard's topic-pages section: a tool shipping topic pages must name its valid topics in the `skill` subcommand's help text, statically (no runtime lookups) | Discussed — explicitly agreed ("both go in"); static-and-free constraint stated verbatim | S:90 R:85 A:90 D:90 |
| 2 | Certain | Machine-readable enumeration is a reserved positional topic (`<tool> skill topics`, names one per line, raw stdout, exit 0), not a `--list` flag | Discussed — chosen for zero-shll-change composer passthrough (cobra would eat a flag); `--list`, roster-hardcoding, and glossary annotation explicitly rejected with reasons | S:90 R:80 A:90 D:95 |
| 3 | Certain | `topics` is reserved in every adopting tool's topic namespace — no tool may ship a content topic named `topics` | Discussed — stated verbatim in the agreed design | S:90 R:75 A:85 D:90 |
| 4 | Certain | Scope boundary: this repo only — the standard amendment + Verifying-conformance additions (+ shll's own conformance where applicable); rk/fab-kit/other tools' adoption is out, phased per-repo on their release cadence | Discussed — stated verbatim; matches the standard's existing Adoption posture and every prior standard rollout | S:90 R:80 A:85 D:90 |
| 5 | Certain | Constitution VII is satisfied without a justification escalation: no new top-level subcommand — the reserved topic rides the existing `skill [topic]` surface | Discussed — stated as part of why the positional form was chosen; constitution text confirms the bar applies to new top-level subcommands | S:90 R:90 A:95 D:95 |
| 6 | Certain | The amendment must re-run `scripts/sync-standards.sh` so the embedded `src/cmd/shll/standards/skill.md` matches; `TestStandardsEmbedMatchesCanonical` gates it | Determined by the codebase — the sync + drift-guard mechanism is established and test-enforced | S:80 R:90 A:95 D:95 |
| 7 | Confident | Cross-reference sweep of `docs/site/standards/` is in scope but expected to be a no-op outside skill.md: principles.md references the skill standard only generically; help-dump.md and readme-extraction.md don't reference it | Verified by grep at intake time; sweep retained as an acceptance check rather than assumed edits | S:70 R:85 A:80 D:75 |
| 8 | Confident | `<tool> skill topics` otherwise inherits the standard's uniform invocation contract (raw stdout, stderr empty on success, exit 0, static-only by construction) | Direct extension of the standard's existing invocation contract for `skill` and topic pages | S:60 R:75 A:80 D:75 |
| 9 | Confident | The `Topics:` help line's exact format is an example, not a mandate — the standard requires the names to appear in the skill help text, illustrated with `Topics: code, display, mux, tutorial` | Description phrased it as "e.g."; mandating exact formatting would exceed the discussed agreement | S:65 R:75 A:70 D:60 |
| 10 | Confident | The help-text enumeration and the core bundle's topic index list content topics only — the reserved `topics` name is not listed in either; ordering of printed names is left to the tool | Clarified — user confirmed | S:95 R:65 A:45 D:40 |
| 11 | Certain | Reserved-topic applicability: mandated for ALL adopting tools — a topic-less tool prints empty stdout + exit 0 for `<tool> skill topics`; `shll skill shll topics` changes from today's exit-2 usage error to empty output + exit 0 | Clarified — user changed to "all adopting tools, empty-output-exit-0 for topic-less"; dimensions re-scored for the new decision (the deferral scores described the open question, not this resolved directive) | S:95 R:75 A:90 D:90 |
| 12 | Certain | shll's own binary conformance (skill.go `--help` Long text, the shll-self `topics` path, tests, possibly shll's own bundle + embed) ships in this change; other tools adopt on their own cadence | Clarified — user changed to "include shll conformance in this change"; dimensions re-scored for the new decision (same rationale as #11) | S:95 R:80 A:90 D:90 |

12 assumptions (8 certain, 4 confident, 0 tentative, 0 unresolved).
