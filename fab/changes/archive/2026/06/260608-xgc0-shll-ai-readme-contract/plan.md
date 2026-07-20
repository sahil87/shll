# Plan: Conform repo to shll.ai README-extraction contract

**Change**: 260608-xgc0-shll-ai-readme-contract
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

> This is a DOCS-ONLY change. There is no Go source, no test suite to run.
> "Requirements" here are the shll.ai README-extraction contract clauses the repo
> must satisfy; acceptance is asserted by inspection (grep sweep + manual review),
> not by a test runner.

### Contract Conformance: README.md

#### R1: README head order is preserved and conformant
The README.md head MUST be, in order: a single `# H1`, then the `> toolkit` blockquote, then any contiguous badge/image lines, then the first prose line (the site intro). Nothing (frontmatter, HTML comment, `<h1>`) may appear above the H1. The repo is already conformant here; this change MUST NOT regress it.

- **GIVEN** the rendered shll.ai page pulls the README head slice
- **WHEN** the contract reads the top of README.md
- **THEN** line 1 is `# shll`, line 3 is the `> Part of [@sahil87's … toolkit]` blockquote, and the first prose line is the "One command to install, update, and shell-wire…" intro
- **AND** no frontmatter/HTML-comment/`<h1>` sits above the H1

#### R2: README → docs/site pointers use the natural repo-relative form
The README SHALL link to the new depth pages using the natural repo-relative path `docs/site/<page>.md` (the only relative form allowed besides in-page `#anchor` fragments). shll.ai rewrites these to `/tools/shll/<page>` on render. Pointers MUST be added in `## Quick start` (after the bootstrap block), `## Install`, and `## Reference`.

- **GIVEN** the README references the new install/workflows depth pages
- **WHEN** a pointer is authored
- **THEN** it is written as `docs/site/install.md` / `docs/site/workflows.md` (repo-relative `.md`), never as `./` or `../` or an absolute site URL
- **AND** the existing two in-page `#anchor` links are left unchanged

#### R3: README absolute-link / no-image invariants are preserved
Every site-leaving link in the README MUST remain an absolute `https://…` URL; the README MUST contain zero images, zero mermaid fences, and zero `#gh-dark-mode-only`/`#gh-light-mode-only` fragments. This change MUST NOT introduce any of those.

- **GIVEN** the contract's image/mermaid/theme-fragment denylist and absolute-link rule
- **WHEN** the README is inspected after edits
- **THEN** all external links are absolute `https://…`, and there are no images, no mermaid, no theme fragments

### Contract Conformance: docs/site tree

#### R4: docs/site/install.md exists and conforms to closure rules
A new `docs/site/install.md` SHALL exist, starting with a single `# Title` H1, covering: brew install (single formula vs `all` meta-formula), `shll install` bootstrap semantics, from-source `just install`, `shll shell-setup` rc-wiring (`--print`/`--uninstall`/`--rc-file`/`--trust-tap`, sentinel block, O_APPEND symlink-safety), `shll shell-init <shell>` (eval line + per-tool contribution table), and the full tap-trust troubleshooting matrix. All content MUST be grounded in README + `docs/memory/cli/*`. Intra-docs/site links (to `workflows.md`) MUST be relative; site-leaving links (`https://shll.ai`, `https://github.com/sahil87/shll`) MUST be absolute; no images.

- **GIVEN** the contract's docs/site closure rules
- **WHEN** `docs/site/install.md` is authored
- **THEN** every relative link/image resolves inside `docs/site/` (no `..` escape), every site-leaving link is absolute `https://…`, and there are no images
- **AND** the page is not named `overview`/`readme`/`commands`

#### R5: docs/site/workflows.md exists and conforms to closure rules
A new `docs/site/workflows.md` SHALL exist, starting with a single `# Title` H1, covering: clean-machine bootstrap, `shll update` day-to-day (single `brew update --quiet`, self-upgrade, delegated per-tool upgrade, skip-uninstalled, progress/timing/`--dry-run`), composing `shll shell-init` (roster-order concat, eval-safety), `shll version` for bug reports (timeout, `not installed` rows), and the composition-model table. Content MUST be grounded in README + `docs/memory/cli/*` + Constitution Principle IV. Same closure rules as R4; "See also" links `install.md` (relative) and `https://shll.ai` (absolute).

- **GIVEN** the contract's docs/site closure rules
- **WHEN** `docs/site/workflows.md` is authored
- **THEN** every relative link/image resolves inside `docs/site/`, every site-leaving link is absolute `https://…`, there are no images, and the page is not named `overview`/`readme`/`commands`

### Non-Goals
- No Go source, CI, justfile, or formula changes — behavior is unchanged.
- No `docs/site/index.md` / landing page — the contract doesn't require one; pages are addressable at their slugs (Assumption #1, Tentative).
- No README restructuring/rewrite — the README is already conformant; only the three pointer additions are made.
- No `docs/memory/**` changes — site-presentation content is distinct from behavior memory.

## Tasks

### Phase 1: README pointers

- [x] T001 Add a `docs/site/install.md` pointer in `README.md` `## Quick start`, after the bootstrap code block (before the `--trust-tap` paragraph or as a trailing sentence in the section) <!-- R2 -->
- [x] T002 Add a `docs/site/install.md` pointer in `README.md` `## Install` (full guide: brew vs `all`, from-source, shell wiring, trust-tap) <!-- R2 -->
- [x] T003 Add `docs/site/install.md` + `docs/site/workflows.md` bullets to `README.md` `## Reference` <!-- R2 -->

### Phase 2: docs/site depth pages

- [x] T004 [P] Create `docs/site/install.md` — "Install & shell wiring" deep guide, grounded in README + `docs/memory/cli/{install,shell-setup,shell-init,update,version}.md`; relative link to `workflows.md`, absolute links to `https://shll.ai` and `https://github.com/sahil87/shll`; no images <!-- R4 --> <!-- rework cycle 1 (fix code): corrected roster order to leaves-first `wt, idea, tu, rk, hop, fab-kit` (matches src/cmd/shll/tools.go Roster + docs/memory/cli/commands.md) at the `shll install` enumeration and the shell-init contributor table; the page had copied the README's stale order -->
- [x] T005 [P] Create `docs/site/workflows.md` — "Workflows" task-oriented walkthroughs, grounded in README + `docs/memory/cli/{update,version,shell-init,commands}.md` + Constitution IV; relative link to `install.md`, absolute link to `https://shll.ai`; no images <!-- R5 --> <!-- rework cycle 1 (fix code): corrected roster order to leaves-first at the `shll install` walk and the `shll version` example output block -->

> **Rework log — cycle 1 (fix code):** Outward review caught a Must-fix — both new pages used the README's stale roster order (`fab-kit, rk, tu, hop, wt, idea`), contradicting the actual leaves-first `Roster` in `src/cmd/shll/tools.go` (`wt, idea, tu, rk, hop, fab-kit`, enforced by `TestRosterLeavesBeforeDependents`, documented in `docs/memory/cli/{version,commands}.md`). Fixed all four enumerations in the two docs/site pages. The README's own stale order is a pre-existing, out-of-scope issue (Non-Goal: no README rewrite) and does not affect the contract render — flagged for a follow-up backlog item.

### Phase 3: Verify

- [x] T006 Run the contract Verify checklist (8 items) + grep sweep over README.md and docs/site/** for `](./`, `](../`, `](docs/`, relative image patterns, `gh-dark-mode-only`, `gh-light-mode-only`, `mermaid`; confirm head order, first prose line, no-image, no-theme-fragment, no reserved page names <!-- R1 R3 R4 R5 -->

## Execution Order

- T001–T003 edit the same file (`README.md`) — run sequentially, not `[P]`.
- T004 and T005 are independent new files — `[P]`.
- T006 runs last (verifies the whole tree).

## Acceptance

### Functional Completeness

- [x] A-001 R1: README top is `#` H1 → `>` toolkit blockquote → (badges, if any) → first prose line, with nothing above the H1 (verified by inspection of README.md lines 1–5). README.md:1 `# shll`, :3 `> Part of [@sahil87's…]`, :5 first prose "One command to install, update, and shell-wire…"; nothing above the H1. No badges (optional).
- [x] A-002 R2: README carries `docs/site/install.md` pointers in `## Quick start` and `## Install`, and `docs/site/install.md` + `docs/site/workflows.md` bullets in `## Reference`, each written as repo-relative `.md` paths (no `./`, `../`, or site URL); the two pre-existing `#anchor` links are unchanged. Pointers at README.md:29 (Quick start), :41 (Install), :196–197 (Reference); all `docs/site/<page>.md`. Pre-existing `#anchor` links at :31 and :183 unchanged.
- [x] A-003 R3: README has all external links absolute `https://…`, zero images, zero mermaid fences, zero `#gh-*-mode-only` fragments. Grep sweep: zero `![`, zero `<img`, zero `mermaid`, zero `gh-*-mode-only`; all external links are `https://shll.ai` / `https://github.com/sahil87/…`.
- [x] A-004 R4: `docs/site/install.md` exists, starts with a single `# Title` H1, covers the required topics grounded in README + memory, links `workflows.md` relatively and site-leaving targets absolutely, contains no images, and is not named a reserved slug. H1 `# Install & shell wiring` (install.md:1). Covers brew (single vs `all`), `shll install` bootstrap, from-source, `shll shell-setup` (`--print`/`--uninstall`/`--rc-file`/`--trust-tap`, sentinel block, O_APPEND symlink-safety, never-create invariant), `shll shell-init` + contribution table, tap-trust matrix. `workflows.md` links relative; `https://shll.ai`/`github.com` absolute. No images. Slug `install` not reserved.
- [x] A-005 R5: `docs/site/workflows.md` exists, starts with a single `# Title` H1, covers the required topics grounded in README + memory + Constitution IV, links `install.md` relatively and `https://shll.ai` absolutely, contains no images, and is not named a reserved slug. H1 `# Workflows` (workflows.md:1). Covers clean-machine bootstrap, `shll update` day-to-day (single `brew update --quiet`, self-upgrade, delegation, skip-uninstalled, `[N/M]`+timing, `--dry-run`), composing `shll shell-init` (roster-order concat, eval-safety), `shll version` (2s timeout, `not installed`), composition-model table citing Constitution IV. `install.md` relative; `https://shll.ai` absolute. No images. Slug `workflows` not reserved.

### Scenario Coverage

- [x] A-006 R4 R5: Grep sweep over README.md and docs/site/** finds no `](./`, `](../`, `](docs/site/…)` inside docs/site (no `..` escape), no relative-image patterns, no `gh-dark-mode-only`/`gh-light-mode-only`, no `mermaid` — matching the contract Verify checklist. All seven greps returned "none".

### Edge Cases & Error Handling

- [x] A-007 R4 R5: docs/site closure holds — every relative link inside docs/site/** resolves to a path that exists inside docs/site/ (only `install.md` ↔ `workflows.md` cross-links and in-page `#anchor` fragments are relative; all else absolute). install.md links only `workflows.md` (relative, file exists) + intra-page `#` anchors + absolute https; workflows.md links only `install.md` (relative, file exists) + absolute https. All 8 anchor targets verified to resolve to real heading slugs via GitHub slug algorithm.

### Code Quality

- [x] A-008 Pattern consistency: New docs/site pages match the README's terse, technical, accurate voice; no invented behavior (all claims trace to README or `docs/memory/cli/*`). Grounding spot-check (9 key claims) all traced to memory: `--dry-run` (update.md:176–210, install.md:45–67), `[N/M]`+timing tail w/ honesty constraint (update.md:136–138), subset targeting (update.md:40–64, install.md:69–86), `shll` not a valid install target (install.md:33,76), `# ── <tool> ──` separator (shell-init.md:50,61), never-create + O_APPEND symlink safety (shell-setup.md:88,186–207), default rc targets (shell-setup.md:62–64), `shll install requires Homebrew. Install from https://brew.sh` verbatim (install.md:11), 2s version timeout / "well under 15s" (version.md:68,88). No ungrounded claims found.
- [x] A-009 No unnecessary duplication: docs/site pages add depth (install guide, workflow walkthroughs) rather than restating the README verbatim; README pointers reference them instead of inlining. Pages add material absent from README (from-source `dev`/no-self-upgrade caveats, shell auto-detection table, rc-file target table, subset error semantics, dry-run write/read split, eval-safety-by-construction). The version-dump example block is reused verbatim from README (acceptable — canonical sample output). README adds pointers, does not inline the depth.

## Notes

- Check items as you review: `- [x]`
- This is a docs change (`change_type: docs`) — no test runner; acceptance is by inspection + grep.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Tentative | No `docs/site/index.md` / landing page added | Contract does not require one; pages are addressable directly at their slugs. Carried forward from intake Assumption #9. | S:55 R:75 A:60 D:55 |
| 2 | Confident | Quick-start pointer placed as a trailing sentence after the bootstrap block, before the existing `--trust-tap` explainer paragraph | Keeps the natural reading flow (bootstrap → "go deeper" → trust-tap nuance) and matches the intake's "after the bootstrap block" instruction | S:80 R:85 A:85 D:80 |
| 3 | Confident | docs/site pages use only `install.md` ↔ `workflows.md` relative cross-links plus in-page anchors; everything else absolute | Satisfies closure rule 6 (no `..` escape) trivially since the tree has exactly two flat files | S:85 R:85 A:90 D:85 |

3 assumptions (0 certain, 2 confident, 1 tentative).
