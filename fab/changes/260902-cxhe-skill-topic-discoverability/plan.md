# Plan: Skill Topic Discoverability

**Change**: 260902-cxhe-skill-topic-discoverability
**Intake**: `intake.md`

## Requirements

### Standard: skill.md amendments

#### R1: Help-text topic enumeration (MUST)
The `skill` standard's "Topic pages (large-scope tools)" section MUST gain a mandate: a tool that ships ≥1 topic page MUST enumerate its valid topic names in the `skill` subcommand's help text, statically (embedded at build time — no runtime lookups). The `Topics: code, display, mux, tutorial` line is the illustrative form ("e.g."), not an exact-format mandate. A core-bundle-only tool's help text is unaffected by this mandate.

- **GIVEN** a tool that ships topic pages (e.g. run-kit)
- **WHEN** a user or agent runs `<tool> skill --help`
- **THEN** the help text names every shipped content topic
- **AND** the enumeration is static — identical bytes for a given release

#### R2: Reserved `topics` topic (machine-readable enumeration)
The standard MUST define `<tool> skill topics` as a **reserved topic name** printing the tool's content-topic names **one per line, raw to stdout, stderr empty, exit 0**. It is mandated for **ALL adopting tools**: a topic-less tool prints **empty stdout + exit 0** (zero topics → zero lines). `topics` is reserved in every tool's topic namespace — no tool may ship a content topic named `topics`. Ordering of printed names is left to the tool. The reserved name is a machine affordance defined by the standard: it need NOT appear in the `Topics:` help line or the core bundle's topic index (those enumerate content topics only). The standard SHOULD record the rationale for the positional-reserved-topic form over a `--list` flag: it rides the shll composer's existing two-positional-arg passthrough (`shll skill <tool> topics`) with zero composer changes, where a flag would be intercepted by the composer's own flag parsing.

- **GIVEN** run-kit shipping topics `code, display, mux, tutorial`
- **WHEN** `rk skill topics` (or `shll skill run-kit topics`) runs
- **THEN** the four names print one per line to stdout, stderr empty, exit 0
- **GIVEN** a tool shipping zero topic pages
- **WHEN** `<tool> skill topics` runs
- **THEN** stdout is empty (zero bytes), stderr empty, exit 0

#### R3: Verifying-conformance checklist additions
The standard's "Verifying conformance" section MUST gain checks for both mandates, in the style of the existing items: (a) the `skill` subcommand's help text names every shipped topic; (b) `<tool> skill topics` prints the shipped topic names one per line, raw to stdout, stderr empty, exit 0 — empty output for a topic-less tool; (c) no content topic is named `topics`.

- **GIVEN** the amended standard
- **WHEN** a tool author runs the Verifying conformance checklist
- **THEN** both new mandates have verifiable checklist entries

### shll: own conformance

#### R4: `shll skill shll topics` prints empty, exit 0
`writeSkillTopic`'s shll-self branch MUST special-case the reserved topic: `shll skill shll topics` prints **empty stdout (zero bytes), stderr empty, exit 0** — shll ships zero topic pages, so the honest machine answer is the empty list. Every other topic name keeps today's behavior: the `skillNoTopicsFmt` notice on stderr + usage exit 2. Roster-tool passthrough is untouched — `shll skill <tool> topics` already forwards verbatim, and conformance of the output is the tool's own obligation.

- **GIVEN** the shll binary
- **WHEN** `shll skill shll topics` runs
- **THEN** stdout is empty, stderr is empty, exit code is 0
- **GIVEN** the shll binary
- **WHEN** `shll skill shll nosuchtopic` runs
- **THEN** stderr carries the no-topics notice and the exit code is 2 (unchanged)

#### R5: shll's `skill --help` mentions the reserved topic
`newSkillCmd`'s Long text MUST mention the reserved `topics` topic — that `shll skill <tool> topics` lists a tool's topic names (one per line; empty for a tool with none). The help-text *enumeration* MUST (R1) does not bind shll — shll ships zero content topics.

- **GIVEN** the shll binary
- **WHEN** `shll skill --help` runs
- **THEN** the Long text names `topics` as the reserved enumeration topic

#### R6: shll's own bundle mentions the machine seam
`docs/site/skill.md` (the bundle `shll skill shll` serves) SHOULD mention `shll skill <tool> topics` where it teaches topic discovery (the capabilities-map `skill` line and/or the two-step gotcha), staying within the ≤150-line budget. The synced embed `src/cmd/shll/skill/skill.md` follows via the sync script.

- **GIVEN** the amended bundle
- **WHEN** an agent reads `shll skill shll`
- **THEN** it learns `shll skill <tool> topics` enumerates a tool's topics
- **AND** the bundle is ≤150 lines and `TestSkillEmbedMatchesCanonical` passes

#### R7: Embed sync and drift guards
`scripts/sync-standards.sh` MUST be re-run after amending the canonical files so the committed embeds (`src/cmd/shll/standards/skill.md`, `src/cmd/shll/skill/skill.md`) match byte-for-byte; `TestStandardsEmbedMatchesCanonical` and `TestSkillEmbedMatchesCanonical` MUST pass, along with the full package test suite.

- **GIVEN** the amended canonical `docs/site/standards/skill.md` and `docs/site/skill.md`
- **WHEN** `scripts/sync-standards.sh` runs and `go test ./...` executes under `src/`
- **THEN** both drift guards and all other tests pass

### Non-Goals

- rk / fab-kit / other tools' implementation of the two mandates — phased per-repo on each tool's release cadence (the standard's Adoption section posture).
- Annotating the bare `shll skill` glossary with per-tool topic lists — rejected in the intake (subprocess-per-tool cost; per-tool discovery is the right layer).
- Any change to the roster, the composer's flag parsing, or the one-arg passthrough classification.

### Design Decisions

#### Reserved topic served in-process for shll-self; passthrough untouched for roster tools
**Decision**: `shll skill shll topics` is answered in-process by the shll-self branch (empty stdout, exit 0); `shll skill <tool> topics` for roster tools stays a verbatim passthrough with no reserved-name special-casing in the composer.
**Why**: The reserved topic is each tool's own producer obligation; the composer forwarding it verbatim is exactly the zero-shll-change property that motivated the positional form. Only the shll-self branch answers in-process, because a self-invocation subprocess would recurse into the composer (the established `writeShllOwnBundle` rationale).
**Rejected**: Composer-side interception of `topics` for roster tools (would mask a tool's non-conformance and re-couple shll's release cadence to the topic contract).
*Introduced by*: 260902-cxhe-skill-topic-discoverability

## Tasks

### Phase 2: Core Implementation

- [x] T001 Amend `docs/site/standards/skill.md`: topic-pages section gains the help-text enumeration MUST (static, example-form `Topics:` line) and the reserved `topics` topic definition (one name per line, raw stdout, stderr empty, exit 0; mandated for all adopting tools with empty-output-exit-0 for topic-less tools; namespace reservation; ordering tool-chosen; content-topics-only enumeration; positional-over-flag composer rationale); "Verifying conformance" gains the three new checks <!-- R1,R2,R3 -->
- [x] T002 Amend `src/cmd/shll/skill.go`: add a `skillReservedTopic = "topics"` named constant; `writeSkillTopic`'s shll-self branch returns empty stdout + exit 0 for the reserved topic (other topics keep the `skillNoTopicsFmt` exit-2 path); `newSkillCmd` Long text mentions the reserved `topics` topic <!-- R4,R5 -->
- [x] T003 Update `src/cmd/shll/skill_test.go`: pin `shll skill shll topics` → empty stdout, empty stderr, exit 0, no subprocess; pin `shll skill shll <other>` → exit 2 unchanged; assert the Long text mentions `topics` <!-- R4,R5 -->
- [x] T004 [P] Amend `docs/site/skill.md`: the capabilities-map `skill` line and/or the two-step gotcha mention `shll skill <tool> topics` (stay ≤150 lines) <!-- R6 -->
- [x] T005 Run `scripts/sync-standards.sh` to refresh both embeds, then `cd src && go test ./...` — drift guards and full suite green <!-- R7 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The standard's topic-pages section carries the help-text topic-enumeration MUST with the static constraint and example-not-mandate framing
- [x] A-002 R2: The standard defines the reserved `topics` topic — one name per line, raw stdout, stderr empty, exit 0; mandated for all adopting tools (empty output for topic-less); namespace reservation; ordering tool-chosen; content-topics-only enumeration; positional-over-flag rationale recorded
- [x] A-003 R3: "Verifying conformance" carries the three new checks in the existing items' style
- [x] A-004 R4: `shll skill shll topics` prints empty stdout with stderr empty and exit 0; every other shll topic keeps the exit-2 no-topics notice — both pinned by tests
- [x] A-005 R5: `shll skill --help` Long text names the reserved `topics` topic
- [x] A-006 R6: shll's bundle teaches `shll skill <tool> topics` and stays ≤150 lines

### Behavioral Correctness

- [x] A-007 R4: The reserved-topic branch is in-process (no subprocess spawned for `shll skill shll topics`), and roster-tool topic passthrough behavior is byte-unchanged

### Scenario Coverage

- [x] A-008 R7: `scripts/sync-standards.sh` re-run; `TestStandardsEmbedMatchesCanonical` and `TestSkillEmbedMatchesCanonical` pass; full `go test ./...` green
- [x] A-009 R1: Cross-reference sweep of `docs/site/standards/` confirms no other standards file requires a consistency touch (expected no-op per intake grep)

### Code Quality

- [x] A-010 Pattern consistency: New code follows the existing skill.go conventions (named constants for magic strings, doc comments in the house style, test seam via `runSkill` + fake `proc.Runner`)
- [x] A-011 No unnecessary duplication: The reserved-topic name is single-sourced in one constant; no logic duplicated from the sub-tools
- [x] A-012 No new subprocess paths: no direct `os/exec` — the change adds no subprocess invocation at all (Constitution I posture preserved)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | No composer-side special-casing of `topics` for roster tools — passthrough stays verbatim | The zero-shll-change composition property is the intake's stated rationale for the positional form | S:90 R:85 A:90 D:90 |
| 2 | Confident | "Empty output" for a topic-less tool means zero bytes on stdout (no trailing newline) | "One name per line" with zero names is zero lines; a lone newline would be a phantom empty name for line-splitting consumers | S:70 R:80 A:80 D:75 |
| 3 | Confident | The reserved-topic definition lives inside the standard's existing "Topic pages" section (a new sub-heading), not a new top-level section | It is a topic-namespace rule; the section already owns the `<tool> skill <topic>` contract, and the house register keeps sections few | S:65 R:85 A:80 D:70 |
| 4 | Confident | The glossary's `skillHintLine` stays unchanged | The hint already teaches the `<tool> <topic>` form; enumerating the reserved name there is optional per the content-topics-only decision (intake #10) | S:60 R:90 A:80 D:70 |

4 assumptions (1 certain, 3 confident, 0 tentative).
