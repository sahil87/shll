# Intake: Constitution Toolkit Standards Article

**Change**: 260717-qas0-constitution-toolkit-standards
**Created**: 2026-07-18

## Origin

One-shot `/fab-new` invocation with a fully-specified task directive. This is the **shll-repo leg of backlog item `[std1]`** (Toolkit-standards rollout, wave 1 — "optionally `shll` itself — same amendment"), adapted for shll's unique position: shll is both a toolkit member AND the canonical author/publisher of the standards themselves.

> Task: Amend this repo's fab constitution to bind it to the sahil87 toolkit standards.
>
> This repo (shll) is the toolkit's meta-CLI and is ALSO the canonical author/publisher of the sahil87 toolkit standards themselves (docs/site/standards/ tree, rendered on https://shll.ai, readable offline via the `shll standards` command that shll itself implements). Because shll's own constitution amendment cannot depend on the `shll standards` command it is simultaneously the author of (a circular dependency — the binary being amended would need to already ship the very command whose output the constitution's obligation depends on), this repo's article must reference the docs/site/standards/ tree directly instead of the `shll standards` command.
>
> Make this change:
>
> 1. In fab/project/constitution.md, add a new article under Additional Constraints (create the section if this constitution lacks it, matching the file's existing structure): *(article text reproduced in full under What Changes below)*
> 2. Bump the constitution's Last Amended date (and version, per this file's own governance line).
> 3. Deliberate constraint: do NOT copy standard names, counts, or per-standard URLs into the constitution — the docs/site/standards/ tree is the enumeration, and the article must stay correct as standards evolve. Do NOT reference the `shll standards` command in this repo's own constitution article (circular).
>
> Ship per this repo's normal flow (docs-type fab change → PR). Nothing else is in scope — no conformance fixes in this change.

**Key divergence from the generic `[std1]` Directive-1 article** (deliberate, user-specified): the tool repos' article enumerates standards via `shll standards` and falls back to the shll repo's docs tree; shll's own article references `docs/site/standards/` directly, because binding shll's constitution to the output of a command shll itself implements would be circular.

## Why

1. **The pain point**: The toolkit now publishes binding, producer-facing standards (CLI principles + mechanical contracts), canonically authored in this very repo. Every other toolkit repo is being bound to them via a constitution article (`[std1]` wave 1). But shll itself — the publisher — has no constitutional obligation to conform to the standards it authors. Its constitution's always-load guarantee (every fab pipeline run reads `fab/project/constitution.md`) is the enforcement seam, and today that seam carries nothing about the standards.
2. **Consequence of not fixing**: shll changes to its CLI surface, help output, README, or docs/site could silently drift from the standards it publishes — the worst repo to drift, since it is the reference implementation and canonical source. The rollout would bind six repos while exempting the author.
3. **Why this approach**: A constitution article is the established mechanism — fab loads the constitution on every run, so the obligation is enforced at every future pipeline stage without new tooling. Referencing the `docs/site/standards/` **tree** (not the `shll standards` command, and not an enumerated list) keeps the article evergreen: standards added or revised in the tree bind this repo with no further amendment, and no circular dependency on the binary being governed.

## What Changes

### 1. New article in `fab/project/constitution.md` under `## Additional Constraints`

The `## Additional Constraints` section **already exists** (articles: Test Integrity, Cross-Platform Behavior, Tool Roster Source of Truth) — no section creation needed. Append the new article as the fourth `###` article, after `### Tool Roster Source of Truth`, before `## Governance`.

The article text, verbatim (typography adapted to the file's existing conventions — em-dashes and backticked paths, matching the surrounding articles):

```markdown
### Toolkit Standards

This repo publishes and MUST itself conform to the sahil87 toolkit's binding, producer-facing standards — CLI design principles plus mechanical contracts (machine-readable help output, README/docs-site structure, and others over time), canonically authored in this repo's own `docs/site/standards/` tree and rendered on https://shll.ai. Before changing the CLI surface, help output, `README.md`, or `docs/site/`, the change MUST be checked against the standards governing that surface by reading the relevant file(s) directly under `docs/site/standards/`. Standards added or revised there bind this repo without further amendment to this constitution.
```

**Deliberate constraints on the article content** (user-specified, MUST hold):

- NO standard names, counts, or per-standard URLs — the tree is the enumeration; the article must stay correct as standards evolve.
- NO reference to the `shll standards` command — circular (the binary being amended would need to already ship the command whose output the obligation depends on).

### 2. Governance line bump

Current line (constitution `## Governance`):

```markdown
**Version**: 1.0.0 | **Ratified**: 2026-05-09 | **Last Amended**: 2026-05-09
```

New line:

```markdown
**Version**: 1.1.0 | **Ratified**: 2026-05-09 | **Last Amended**: 2026-07-18
```

MINOR bump (1.0.0 → 1.1.0): a new article is additive governance material (new section/principle added = MINOR; the governance line records semver but states no explicit bump policy — see Assumptions #3). `Ratified` is unchanged.

### 3. Nothing else

No conformance fixes, no other files. The single deliverable is the amended `fab/project/constitution.md`, shipped as a docs-type change via the normal PR flow.

**Context on the path reference**: on `main` today the standards live flat at `docs/site/{principles,help-dump,readme-extraction}.md`. The in-flight change `260717-i70w-standards-dir-skill-contract` (ship stage, PR recorded) moves them to `docs/site/standards/` and adds `skill.md`. The article's `docs/site/standards/` reference matches the post-move canonical layout — backlog `[std2]`'s prerequisite pins this path ("`shll standards --json` source_paths read `docs/site/standards/…`"). If this PR merges before i70w's, the reference is briefly forward-looking; that ordering risk is accepted (see Assumptions #5).

## Affected Memory

None. This change amends `fab/project/constitution.md` — governance material that is itself part of the always-load context layer for every pipeline run. No system behavior, CLI surface, or spec-level design changes; duplicating the constitutional obligation into `docs/memory/` would add a second source of truth for content fab already loads unconditionally.

## Impact

- **Files**: `fab/project/constitution.md` only (one new `###` article, one governance-line edit).
- **Code**: none. **Tests**: none. **CLI surface**: unchanged.
- **Process impact**: every future fab pipeline run in this repo loads the new article and must check CLI-surface/help/README/docs-site changes against `docs/site/standards/` before applying them.
- **Change type**: docs.

## Open Questions

None — the directive fully specifies the article text, placement, constraints, and shipping flow.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Article text used verbatim as provided, with typography adapted to the file's conventions (em-dashes for `--`, backticks on `docs/site/standards/`, `README.md`, `docs/site/`) | User provided the prose; surrounding articles backtick paths/commands and use em-dashes — mechanical style match | S:95 R:90 A:95 D:90 |
| 2 | Certain | Placement: appended as the fourth article under the existing `## Additional Constraints`, after `### Tool Roster Source of Truth` | Section already exists (no creation branch needed); appending matches the file's structure and the directive's "matching the file's existing structure" | S:90 R:95 A:95 D:90 |
| 3 | Confident | Version bump is MINOR: 1.0.0 → 1.1.0 | Governance line records semver but states no bump policy; constitution convention (spec-kit lineage) is MAJOR for removals/redefinitions, MINOR for a new article/section, PATCH for wording — a new additive article is MINOR | S:65 R:95 A:70 D:75 |
| 4 | Certain | Last Amended → 2026-07-18 (today); Ratified unchanged at 2026-05-09 | Directive says bump Last Amended; Ratified marks original adoption by definition | S:85 R:95 A:95 D:95 |
| 5 | Confident | Reference `docs/site/standards/` even though `main` still has the standards flat under `docs/site/` — the in-flight change 260717-i70w (PR open) performs the directory move | Directive names the path explicitly and repeatedly; backlog `[std2]` pins the post-move path; worst case is a briefly forward-looking path if this PR merges first, trivially correct once i70w lands | S:75 R:80 A:75 D:70 |
| 6 | Certain | `change_type: docs` | Directive states it outright ("docs-type fab change → PR") | S:95 R:90 A:95 D:95 |
| 7 | Confident | Change carries a fresh random ID, NOT backlog ID `std1` | `[std1]` is the multi-repo wave-1 rollout item (shll leg marked "optionally"); claiming its ID would mark the whole wave done on archive. Relationship recorded in Origin instead | S:70 R:85 A:80 D:75 |
| 8 | Certain | Affected Memory: none | Constitution is always-loaded context; governance amendment changes no system behavior — a memory copy would be a second source of truth | S:80 R:90 A:90 D:85 |

8 assumptions (5 certain, 3 confident, 0 tentative, 0 unresolved).
