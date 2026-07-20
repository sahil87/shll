# Intake: Install Docs: Policy B Conformance (README slim + docs/site destination curation)

**Change**: 260720-d4o6-install-docs-policy-b
**Created**: 2026-07-20

## Origin

One-shot `/fab-new` invocation with a detailed brief:

> Conform shll's install documentation to the install-composition standard, Policy B — with shll's special role in mind. Read docs/site/standards/install-composition.md in this repo first. shll is the composition point: its docs/site tree is rendered on https://shll.ai and IS the canonical install destination, so this repo's docs/site install page(s) are the one place per-formula and full-toolkit install steps legitimately live — curate them as the destination (curl bootstrap primary, shll install / subset form, troubleshooting), do not delete them. The README however should slim to the curl bootstrap one-liner plus a pointer to https://shll.ai for everything else — remove any per-formula brew install instructions for sibling tools. KEEP incidental mentions: error-hint examples in standards text (Policy A mandates them) and the install-composition standard's own example hints. Mechanical docs-only change; keep usage/feature content intact.

Key decisions were carried in the brief itself (curate-don't-delete docs/site, slim README, keep incidental mentions, docs-only). No SRAD questions were needed.

## Why

The [install-composition standard](../../../docs/site/standards/install-composition.md) (merged in PR #69, change w6ay) sets Policy B: install documentation is centralized on shll.ai — per-tool READMEs MUST NOT carry per-formula `brew install` instructions, because "seven copies of the install dance drift, and every change to the install story (a tap-trust requirement, a bootstrap change) has to be chased across every repo plus the tap."

shll's own README is formally **out of Policy B's producer scope** (the standard's scope carve-out: shll's README + shll.ai *is* the centralized destination), so nothing here is a violation today — `docs/memory/cli/standards-conformance.md` records the Policy B row as "Carve-out — not a violation." But the carve-out names shll as the *destination*, not as a repo licensed to keep a second full copy of the install dance:

1. **The drift the standard warns about is already visible inside this one repo.** The README's Install + Troubleshooting sections duplicate `docs/site/install.md` nearly paragraph-for-paragraph (manual bootstrap, tap-trust two-gates explanation, Homebrew version floor, `--no-trust`). Every install-story change currently has to be written twice in the same repo.
2. **The docs still recommend the retired `all` meta-formula.** The standard's Precedent paragraph states "the `all` meta-formula is retired in favor of `shll install`", yet README (line ~42), `docs/site/install.md` (§Bootstrap via Homebrew), and `docs/site/workflows.md` (step 1 parenthetical) all still document it as a supported path.
3. **If we don't do this**, shll — the repo that *publishes* the standard — models the exact per-repo install sprawl the standard tells the six roster repos to remove, right as the 6-repo rollout wave starts (see memory: toolkit standards roadmap). The publisher should demonstrate the target shape.

The approach: make `docs/site/install.md` (rendered at shll.ai/shll/install) the single curated install destination in this repo, and slim the README's install surface to the curl bootstrap plus pointers — mirroring what every sibling README will do, while legitimately keeping the full detail one click away on the site.

## What Changes

Docs-only. Three files: `README.md`, `docs/site/install.md`, `docs/site/workflows.md`. No source code, no embedded-doc rebuild (`shll standards` embeds only `docs/site/standards/*`, `shll skill shll` embeds only `docs/site/skill.md` — neither is touched).

### README.md — slim the install surface

**Keep** in `## Install`:

- The 4-line clean-machine block (it is the bootstrap flow itself, not per-formula detail):

  ```sh
  curl -fsSL https://shll.ai/install | sh          # install shll + the whole roster
  shll shell-setup                                 # wire shell integration into your rc file
  shll agent-setup                                 # optional, once per machine: place the toolkit skill for agent harnesses
  exec $SHELL                                      # reload so the shell integration takes effect
  ```

- The subset variant (`curl -fsSL https://shll.ai/install | sh -s -- hop wt`).
- A short requirements sentence (Homebrew ≥ 6.0.4; the script never auto-installs brew) — one or two sentences, not the current full paragraph.
- The `shll agent-setup` explainer paragraph MAY be trimmed but its existence is feature content — keep at least a one-line version with the link it carries today.

**Add** a pointer paragraph closing the Install section: everything else — manual brew bootstrap, from-source builds, shell wiring detail, tap-trust troubleshooting — lives on the site. Link both the natural repo-relative page (`[install guide](docs/site/install.md)` — readme-extraction rule 8: README is the hub; the site rewrites it to `/shll/install`, GitHub resolves it) and the absolute `https://shll.ai`.

**Remove** from the README (all content already exists on `docs/site/install.md`; none of it is lost):

- The `> **Why brew trust first?**` blockquote (the two-trust-gates detail — duplicated in install.md §Tap-trust troubleshooting).
- The `### Manual bootstrap (brew)` subsection (the `brew trust --formula sahil87/tap/shll && brew install sahil87/tap/shll` walkthrough — duplicated in install.md §Bootstrap via Homebrew).
- The `all` meta-formula sentence ("`shll` is also installed transitively via the `all` meta-formula…") — retired formula, see below.
- The `### From source` subsection (duplicated in install.md §From source).
- The `## Troubleshooting` section (the tap-trust deep-dive — duplicated in install.md §Tap-trust troubleshooting). The `## Reference` list already links `docs/site/install.md`; keep that entry.

**Keep intact** (usage/feature content, per the brief):

- The whole `## Commands` section — including the incidental one-time-bootstrap mention inside `### shll install` ("you can't brew-install the running orchestrator (it's the one-time bootstrap `brew trust … && brew install sahil87/tap/shll`)") and `### shll doctor`'s error-hint examples (`run 'brew install …'`). These are feature docs / error hints, not install instructions.
- `## Why shll?`, `## How composition works` (its `shll install` table row describes what the command runs — feature content), `## Reference`.

Internal anchors to the removed Troubleshooting section (the "Why brew trust" blockquote's `#tap-sahil87tap-must-be-trusted-before-install` link) go away with their referrers; grep for dangling `#` anchors after the edit.

### docs/site/install.md — curate as the canonical destination

The page already has the destination shape (curl bootstrap primary → manual bootstrap → `shll install` + subset → from-source → shell-setup → shell-init → tap-trust troubleshooting). Curation is minimal:

- **Remove the `all` meta-formula block** in §Bootstrap via Homebrew: the paragraph "`shll` is also pulled in transitively by the `all` meta-formula…", its code block (`brew trust --formula sahil87/tap/all && brew install sahil87/tap/all`), and the "Use the single formula when… use `all` when…" guidance paragraph. The standard retires `all` in favor of `shll install`.
- **Adjust the intro line** ("The README's Install section is the short version; this page covers every install path…") only if needed for accuracy after the README slims — the statement stays true, so likely no edit.
- Everything else stays: this page is the one legitimate home of per-formula and full-toolkit install steps (the manual `brew trust`+`brew install` bootstrap of shll itself included).

### docs/site/workflows.md — drop the `all` alternative

In §Clean-machine bootstrap step 1, remove only the trailing alternative sentence inside the parenthetical: "Or `brew trust --formula sahil87/tap/all && brew install sahil87/tap/all` to pull the whole toolkit at once, in which case the next step is a no-op." The Homebrew-version-floor sentence in the same parenthetical stays.

### Explicitly out of scope

- `docs/site/standards/*.md` — untouched. Policy A's example install hint (`wt is not installed. Install it: brew install sahil87/tap/wt`) and every standards-text `brew install` mention stay verbatim (the brief mandates keeping them; Policy A requires hints of exactly this shape).
- `docs/site/skill.md` — untouched (agent bundle; its bootstrap line is the canonical hint on the destination site, and touching it would require the embed/drift-guard sync).
- The `rk` → `run-kit` roster-naming drift in `docs/site/install.md`/`workflows.md` tables — pre-existing, unrelated to install composition; not fixed in this mechanical change.
- The tap repo (`sahil87/homebrew-tap`) and its README, and the six roster repos — separate rollout work per the standards roadmap.
- No Go code, no tests, no `scripts/install.sh` changes.

## Affected Memory

- `cli/standards-conformance`: (modify) Update the Policy B row: the carve-out determination stands, but shll's README is now voluntarily slimmed to the bootstrap + pointer shape (modeling what the six roster repos will do), with docs/site/install.md curated as the destination; the `all` meta-formula no longer appears anywhere in shll's docs.

## Impact

- **Files**: `README.md` (~60 lines removed, ~5 added), `docs/site/install.md` (~8 lines removed), `docs/site/workflows.md` (1 sentence removed).
- **Rendered site**: shll.ai's README slice (`/shll/readme`) shrinks; `/shll/install` and `/shll/workflows` re-render on the next pull. No reserved-name, image, or link-shape issues introduced (verify per readme-extraction §Verifying conformance: no new relative links leaving the published set).
- **No binary/test impact**: neither edited page is embedded in the binary; the drift-guard tests (`TestStandardsEmbedMatchesCanonical`, skill embed) are unaffected.
- **Constitution**: the Toolkit Standards clause requires checking README/docs-site changes against the governing standards — this change is that check's implementation for `install-composition` (Policy B) and stays conformant with `readme-extraction` (Install section kept in the slice, hub cross-links, tail rule unaffected).

## Open Questions

None — the brief resolved scope, keep-list, and destination-vs-README split explicitly.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Curate docs/site install pages as the canonical destination (do not delete); slim README to bootstrap + pointer; keep standards-text incidental mentions | User-stated verbatim in the brief | S:95 R:90 A:95 D:95 |
| 2 | Confident | README keeps the full 4-line quick-start block (the curl bootstrap, shell-setup, agent-setup, exec $SHELL) + subset variant — "one-liner" means the curl bootstrap path, not literally one line | The block is shll's own clean-machine flow (feature content); slimming targets duplicated brew/per-formula detail, not the wiring steps | S:70 R:85 A:80 D:70 |
| 3 | Confident | Remove README's "Why brew trust" blockquote, Manual bootstrap, From source, and Troubleshooting sections — all duplicated on docs/site/install.md, replaced by the pointer | "Pointer to https://shll.ai for everything else"; every removed paragraph has a live equivalent on the destination page, so nothing is lost | S:75 R:85 A:80 D:70 |
| 4 | Confident | Drop all `all` meta-formula documentation (README, install.md, workflows.md) | Standard's Precedent paragraph: "the `all` meta-formula is retired in favor of `shll install`"; the canonical destination must not recommend a retired formula. Apply-time sanity check of the tap's current state before wording anything as "removed" | S:65 R:85 A:80 D:75 |
| 5 | Certain | Keep the entire Commands section, composition tables, doctor error hints, and docs/site/skill.md untouched | Brief: "keep usage/feature content intact" + "KEEP incidental mentions"; skill.md additionally carries the embed drift-guard coupling | S:90 R:90 A:90 D:90 |
| 6 | Certain | README pointer uses both the natural repo-relative `docs/site/install.md` link and the absolute https://shll.ai | Determined by readme-extraction rule 8 (README is the hub; natural docs/site links are the one rewritten form) plus the brief's explicit shll.ai pointer | S:70 R:95 A:90 D:80 |
| 7 | Confident | Hydrate updates `cli/standards-conformance`'s Policy B row only (no new memory file) | Docs-only change; the one spec-level fact that moves is the conformance posture already recorded there | S:60 R:90 A:85 D:75 |
| 8 | Certain | Out of scope: rk→run-kit naming drift in docs/site tables, tap repo, roster repos, install.sh, Go code | Brief scopes this as a mechanical docs-only change in this repo; sibling-repo rollout is separately tracked (standards roadmap) | S:80 R:85 A:85 D:80 |

8 assumptions (4 certain, 4 confident, 0 tentative, 0 unresolved).
