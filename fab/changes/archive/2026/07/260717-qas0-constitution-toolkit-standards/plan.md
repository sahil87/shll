# Plan: Constitution Toolkit Standards Article

**Change**: 260717-qas0-constitution-toolkit-standards
**Intake**: `intake.md`

## Requirements

### Constitution: Toolkit Standards Article

#### R1: New "Toolkit Standards" article under Additional Constraints
The constitution `fab/project/constitution.md` MUST carry a new `### Toolkit Standards` article, appended as the fourth `###` article under the existing `## Additional Constraints` section — after `### Tool Roster Source of Truth` and before `## Governance`. The article body MUST be the verbatim prose from the intake's What Changes §1 (typography already adapted to the file's conventions: em-dashes, backticked paths). It MUST bind this repo to conform to the sahil87 toolkit's producer-facing standards, direct future changes to check CLI-surface/help/README/`docs/site/` edits against the relevant file(s) under `docs/site/standards/`, and state that standards added or revised there bind the repo without further amendment.

- **GIVEN** the constitution with `## Additional Constraints` containing Test Integrity, Cross-Platform Behavior, and Tool Roster Source of Truth articles
- **WHEN** the change is applied
- **THEN** a `### Toolkit Standards` article exists immediately after `### Tool Roster Source of Truth` and immediately before `## Governance`
- **AND** its body matches the intake's verbatim article text exactly

#### R2: Article content honors the deliberate constraints
The `### Toolkit Standards` article MUST NOT enumerate standard names, counts, or per-standard URLs (the `docs/site/standards/` tree is the enumeration, keeping the article evergreen), and MUST NOT reference the `shll standards` command (a circular dependency — the binary being governed would need to already ship the command whose output the obligation depends on). The article references the `docs/site/standards/` tree directly.

- **GIVEN** the verbatim article text from the intake
- **WHEN** the article is present in the constitution
- **THEN** it contains no standard names, no standard counts, and no per-standard URLs
- **AND** it contains no mention of the `shll standards` command
- **AND** it references the `docs/site/standards/` tree directly

#### R3: Governance line semver + Last Amended bump
The `## Governance` line MUST be updated from `**Version**: 1.0.0 | **Ratified**: 2026-05-09 | **Last Amended**: 2026-05-09` to `**Version**: 1.1.0 | **Ratified**: 2026-05-09 | **Last Amended**: 2026-07-18` — a MINOR bump (new additive article) with `Ratified` unchanged.

- **GIVEN** the current governance line at version 1.0.0, last amended 2026-05-09
- **WHEN** the change is applied
- **THEN** the governance line reads Version 1.1.0, Ratified 2026-05-09, Last Amended 2026-07-18

### Non-Goals

- Conformance fixes to shll's CLI surface, help output, README, or docs/site — explicitly out of scope per the directive ("Nothing else is in scope — no conformance fixes in this change").
- Any file other than `fab/project/constitution.md`.
- Copying/enumerating the standards themselves into the constitution — the article references the tree, not its contents.

## Tasks

### Phase 1: Core Implementation

- [x] T001 Append the `### Toolkit Standards` article (verbatim intake text) under `## Additional Constraints`, after `### Tool Roster Source of Truth` and before `## Governance`, in `fab/project/constitution.md` <!-- R1 R2 -->
- [x] T002 Update the `## Governance` line in `fab/project/constitution.md` to `**Version**: 1.1.0 | **Ratified**: 2026-05-09 | **Last Amended**: 2026-07-18` <!-- R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `fab/project/constitution.md` contains a `### Toolkit Standards` article positioned as the fourth article under `## Additional Constraints`, after `### Tool Roster Source of Truth` and before `## Governance`, with body matching the intake's verbatim text.
- [x] A-002 R3: The `## Governance` line reads `**Version**: 1.1.0 | **Ratified**: 2026-05-09 | **Last Amended**: 2026-07-18`.

### Behavioral Correctness

- [x] A-003 R2: The `### Toolkit Standards` article contains no standard names, counts, or per-standard URLs, no reference to the `shll standards` command, and references the `docs/site/standards/` tree directly.

### Edge Cases & Error Handling

- [x] A-004 R1: No other section of `fab/project/constitution.md` (Core Principles, other Additional Constraints articles) is altered; only the new article and the governance line change.

### Code Quality

- [x] A-005 Pattern consistency: The new article follows the file's existing article structure and typography (`### {Title}` heading, em-dashes, backticked paths) matching the surrounding Additional Constraints articles.
- [x] A-006 No unnecessary duplication: The article references the `docs/site/standards/` tree rather than duplicating standard content, keeping a single source of truth.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Article text and governance-line values used verbatim from the intake (typography already adapted to the file's conventions) | Intake fully specifies the article prose, placement, and exact old/new governance strings — no interpretation needed | S:95 R:90 A:95 D:95 |
| 2 | Certain | Placement as the fourth `###` article under the existing `## Additional Constraints`, after `### Tool Roster Source of Truth`, before `## Governance` | Section already exists; intake and constitution structure make placement unambiguous | S:95 R:95 A:95 D:95 |
| 3 | Confident | MINOR version bump 1.0.0 → 1.1.0 for an additive new article | Governance line records semver but no explicit bump policy; spec-kit-lineage convention treats a new additive article as MINOR (intake Assumption #3) | S:65 R:95 A:70 D:75 |

3 assumptions (2 certain, 1 confident, 0 tentative).
