# Intake: Conform repo to shll.ai README-extraction contract

**Change**: 260608-xgc0-shll-ai-readme-contract
**Created**: 2026-06-08
**Status**: Draft

## Origin

> Task: conform this repo to shll.ai's README-extraction contract. shll.ai renders this
> tool's page by mechanically pulling a slice of README.md and the docs/site/** tree on a
> daily schedule — nothing is hand-copied, nothing is pushed. (1) Find this repo's row in
> the directive's per-tool table for slug + reserved page names. (2) Part 1 — restructure
> README.md: head order (# H1 → toolkit blockquote → badges → prose), drop GitHub-footer
> sections below the tail denylist, absolute https image URLs, render mermaid to a committed
> image, write site-leaving links as absolute URLs. (3) Part 2 (encouraged) — add
> docs/site/**/*.md for depth (docs/site/install.md, docs/site/workflows.md), following the
> four closed-set rules. (4) Run the Verify checklist before the PR. Ship as a single PR;
> do not touch shll.ai.

One-shot invocation. The contract was fetched and read in full before authoring this intake
(https://github.com/sahil87/shll.ai/blob/main/docs/specs/readme-extraction-contract.md).

## Why

1. **Problem.** shll.ai is a *pull* consumer: a daily cron extracts a slice of this repo's
   `README.md` plus its `docs/site/**` tree and renders the `/tools/shll/` page mechanically.
   If the repo doesn't match the contract's structural expectations, the rendered page is
   wrong in silent ways — relative links 404, relative images vanish (the site vendors zero
   image binaries), the intro is cut at the wrong line, or footer sections leak onto the site.
2. **Consequence of inaction.** The page renders today but is fragile and shallow: it leans
   entirely on the README slice, with no `docs/site/**` depth pages. There is no install
   guide or workflow walkthrough on the site, and any future README image/mermaid/footer
   addition would break the render with no warning.
3. **Approach.** Audit the current README against every clause of the contract's §Producer
   conformance directive, fix the few deviations, and add a `docs/site/` tree for depth.
   Docs-only change — no Go source, no behavior change.

## What Changes

### Contract facts for `shll` (from the per-tool table)

| Field | Value |
|-------|-------|
| Repo / file slug | `shll` |
| Content collector | `content/shll/` |
| URL space | `/tools/shll/` |
| Reserved static slugs (MUST NOT name a page these) | `overview`, `readme`, `commands` |
| `install` / `workflows` | **Not** reserved — allowed page names owned by this repo |

### Part 1 — `README.md` conformance

Current state audit (against the directive):

- **Head order** — already `# shll` (H1) → `> Part of [@sahil87's toolkit](https://shll.ai)…`
  (toolkit blockquote) → prose. No frontmatter, no HTML comment, no `<h1>` above the H1.
  **Conformant.** No badges today; badges are optional, so none are added.
- **First prose line** — "One command to install, update, and shell-wire every tool…" is the
  intended site intro. **Conformant.**
- **Tail denylist** (`Contributing`, `Development`, `Building`, `License`, `Acknowledgements`)
  — the README contains **none** of these headings. `LICENSE` is a separate top-level file,
  not a README section. The trailing `## Reference` section is **not** denylisted, so it stays.
  **Conformant — no trimming needed.**
- **Images** — the README contains **zero** image references. **Conformant** (and `docs/site/`
  pages added in Part 2 will likewise carry no relative images).
- **Mermaid** — **none present.** Nothing to render. (Noted: if one is ever added, it must be
  committed as a rendered SVG/PNG referenced by absolute URL.)
- **`#gh-dark-mode-only` / `#gh-light-mode-only`** — **none present.** **Conformant.**
- **Links that leave the site** — all external links are already absolute `https://…`
  (`https://shll.ai`, `https://github.com/sahil87/...`). **Conformant.**
- **In-page anchor links** — two exist: `[Troubleshooting](#tap-sahil87tap-is-allowed-by-default-warning)`
  and a back-reference `(#--trust-tap--resolve-the-homebrew-tap-trust-warning)`. These are
  fragment-only links to headings *within the same rendered page*; both target headings sit
  inside surviving (non-denylisted) sections, so they resolve on the rendered page. Kept as-is.

Net Part 1 work: the README is **already largely conformant**. The substantive Part 1 change
is to add README links *into* the new `docs/site/` pages, written as the natural repo-relative
path `docs/site/<page>.md` (per closed-set rule 4, shll.ai rewrites these to `/tools/shll/<page>`
on render). These appear in `## Quick start`, `## Install`, and a new pointer in `## Reference`.

### Part 2 — `docs/site/**/*.md` depth tree

Add two pages (the directive's reserved-for-this-repo names), authored to the four closed-set
rules: closure (no relative link/image escapes `docs/site/`), external links absolute-by-author,
all images absolute, README→site links written as natural `docs/site/<page>.md` paths.

- **`docs/site/install.md`** — a deeper install guide than the README quick-start: brew install
  (single formula vs. `all` meta-formula), `shll install` bootstrap semantics, from-source build
  via `just install`, the `shll shell-setup` rc-file wiring (including `--trust-tap`, `--print`,
  `--uninstall`, `--rc-file`), and the full tap-trust troubleshooting matrix. Drawn from the
  README plus `docs/memory/cli/install.md`, `shell-setup.md`, `shell-init.md`.
- **`docs/site/workflows.md`** — task-oriented walkthroughs: clean-machine bootstrap, day-to-day
  `shll update`, composing shell-init, reading `shll version` for bug reports, and the
  composition model (how each subcommand fans out to per-tool CLIs / brew). Drawn from the
  README "How composition works" plus `docs/memory/cli/{update,version,commands}.md`.

Neither page is named `overview`, `readme`, or `commands`. No `docs/site/` index page is added
(the contract does not require one; pages are addressable directly at their slugs).

### Verify checklist (run before PR)

Each item from the directive's Verify checklist is asserted against the final tree: H1→blockquote→
badges head with nothing above the H1; first prose line is the intended intro; every relative
link/image target points into `docs/site/` (README) or stays inside it (tree pages), else absolute
`https://…`; no relative images anywhere; no `#gh-*-mode-only` fragments; any diagram is a committed
rendered image referenced absolutely (n/a — none); no `docs/site/` page named `overview`/`readme`/
`commands`; `install`/`workflows` names are allowed.

## Affected Memory

Docs-only change to repo-presentation files. No `docs/memory/**` behavior memory is created,
modified, or removed — the binary's behavior is unchanged. (`docs/site/**` is site-presentation
content, distinct from `docs/memory/**` system-behavior memory.)

## Impact

- `README.md` — minor edits: add README→`docs/site/` pointers in Quick start / Install / Reference.
- `docs/site/install.md` — **new** file.
- `docs/site/workflows.md` — **new** file.
- No Go source (`cmd/`, `internal/`, `src/`) touched. No CI, justfile, or formula changes.
- `true_impact_exclude` already lists `docs/` and `fab/`, so this change is correctly scoped as
  documentation. No tests apply (no code paths change).

## Open Questions

None. The contract is explicit and the repo's deviations are minimal; all decisions resolve from
the contract text and existing repo content.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Slug `shll`; content `content/shll/`; URL `/tools/shll/`; reserved `overview`/`readme`/`commands` | Read verbatim from the directive's per-tool table row for repo `shll` | S:100 R:90 A:100 D:100 |
| 2 | Certain | README head order (`# shll` → toolkit blockquote → prose) already conforms; no change | Inspected README lines 1–5 against the §1 head-order rule | S:95 R:85 A:100 D:95 |
| 3 | Certain | No tail-denylist headings present; `## Reference` is not denylisted and stays; nothing to trim | Denylist is exactly Contributing/Development/Building/License/Acknowledgements; README has none | S:95 R:85 A:100 D:100 |
| 4 | Certain | No images, no mermaid, no `#gh-*-mode-only` fragments in README → those clauses are already satisfied | Full README scanned; zero image refs and zero theme fragments | S:95 R:90 A:100 D:100 |
| 5 | Certain | All external/site-leaving links already absolute `https://…` | README links are shll.ai + github.com, all absolute | S:90 R:85 A:100 D:95 |
| 6 | Confident | Add `docs/site/install.md` and `docs/site/workflows.md` as the Part 2 depth pages | Directive explicitly names these two filenames as the intended slugs; both are non-reserved | S:85 R:80 A:90 D:80 |
| 7 | Confident | Keep the two in-page `#anchor` README links as-is | Fragment-only links to headings in surviving sections resolve on the single rendered page; removing them would degrade the README on GitHub | S:70 R:85 A:75 D:75 |
| 8 | Confident | Docs-only — touch no Go source, CI, or memory; no tests run | Contract is purely about repo presentation for the site puller; behavior is unchanged | S:90 R:80 A:90 D:85 |
| 9 | Tentative | No `docs/site/index.md` or landing page added | Contract doesn't require one and pages are directly addressable; an index could aid navigation but isn't mandated <!-- assumed: skip docs/site index — not required by contract, pages addressable at their slugs --> | S:55 R:75 A:60 D:55 |

9 assumptions (5 certain, 3 confident, 1 tentative, 0 unresolved). Run /fab-clarify to review.
