# Plan: Install Docs: Policy B Conformance (README slim + docs/site destination curation)

**Change**: 260720-d4o6-install-docs-policy-b
**Intake**: `intake.md`

## Requirements

### Docs: README install-surface slim

#### R1: README Install keeps the bootstrap flow, trimmed
The README `## Install` section MUST keep (a) the 4-line clean-machine block (curl bootstrap, `shll shell-setup`, `shll agent-setup`, `exec $SHELL`) verbatim, (b) the subset variant (`curl -fsSL https://shll.ai/install | sh -s -- hop wt`), (c) a requirements statement of one or two sentences (Homebrew ≥ 6.0.4; the script never auto-installs brew), replacing the current full paragraph, and (d) a trimmed `shll agent-setup` explainer of at least one line carrying the run-kit link it carries today.

- **GIVEN** the current README with a full requirements paragraph and a long agent-setup paragraph
- **WHEN** the Install section is slimmed
- **THEN** the 4-line block and subset variant are byte-identical to before
- **AND** the requirements statement is 1–2 sentences and the agent-setup explainer survives as a shortened paragraph with its [run-kit](https://github.com/sahil87/run-kit) link

#### R2: README duplicated install detail removed
The README MUST NOT carry: the `> **Why brew trust first?**` blockquote, the `### Manual bootstrap (brew)` subsection, the `all` meta-formula sentence, the `### From source` subsection, or the `## Troubleshooting` section. Every removed paragraph has a live equivalent on `docs/site/install.md`. The `## Reference` entry for `docs/site/install.md` is kept, with its description reworded so it no longer names the retired `all` formula ("brew vs `all`").

- **GIVEN** the README carrying those five install-detail blocks
- **WHEN** the slim is applied
- **THEN** none of the five blocks remain, no other section is touched, and no internal anchor dangles (the `#tap-sahil87tap-must-be-trusted-before-install` link is removed with its referrer)

#### R3: README Install closes with a pointer paragraph
The Install section MUST end with a pointer paragraph stating that everything else — manual brew bootstrap, from-source builds, shell-wiring detail, tap-trust troubleshooting — lives on the site, linking BOTH the natural repo-relative page (`[install guide](docs/site/install.md)`, per readme-extraction rule 8) AND the absolute `https://shll.ai`.

- **GIVEN** the slimmed Install section
- **WHEN** a reader needs any removed detail
- **THEN** the closing paragraph routes them to `docs/site/install.md` (rewritten to `/shll/install` on the site, resolved by GitHub in-repo) and to https://shll.ai

### Docs: docs/site destination curation

#### R4: install.md drops the retired `all` meta-formula
`docs/site/install.md` §Bootstrap via Homebrew MUST NOT document the `all` meta-formula: the transitively-pulled-in paragraph, its `brew trust --formula sahil87/tap/all && brew install sahil87/tap/all` code block, and the "Use the single formula when… use `all` when…" guidance paragraph are removed. Everything else on the page stays — it is the one legitimate home of per-formula and full-toolkit install steps.

- **GIVEN** install.md documenting `all` as a supported bootstrap path
- **WHEN** the block is removed
- **THEN** `tap/all` and the `all` meta-formula appear nowhere on the page, and the manual `brew trust`+`brew install sahil87/tap/shll` bootstrap, `shll install`, from-source, shell-setup/init, and tap-trust troubleshooting sections are untouched

#### R5: workflows.md drops the `all` alternative
`docs/site/workflows.md` §Clean-machine bootstrap step 1 MUST NOT carry the trailing alternative sentence ("Or `brew trust --formula sahil87/tap/all && brew install sahil87/tap/all` to pull the whole toolkit at once, in which case the next step is a no-op."). The Homebrew-version-floor sentence in the same parenthetical stays.

- **GIVEN** step 1's parenthetical with both the version-floor sentence and the `all` alternative
- **WHEN** the sentence is removed
- **THEN** the parenthetical reads "(Requires Homebrew ≥ 6.0.4; on 6.0.0–6.0.3, `brew update` first.)" and nothing else in the file changes

### Docs: conformance and keep-intact guarantees

#### R6: usage/feature content and out-of-scope surfaces untouched
The change MUST leave intact: the entire README `## Commands` section (including `### shll install`'s one-time-bootstrap mention and `### shll doctor`'s error hints), `## Why shll?`, `## How composition works`, `## Reference` (list membership), `docs/site/standards/*.md` (Policy A's example hint included), `docs/site/skill.md`, and all Go code/tests/`scripts/install.sh`.

- **GIVEN** the three-file docs-only scope
- **WHEN** the diff is inspected
- **THEN** only `README.md`, `docs/site/install.md`, and `docs/site/workflows.md` are modified, and within README only the Install/Troubleshooting/Reference-description surfaces change

#### R7: post-edit conformance verification passes
After the edits, verification MUST show: no dangling `#` anchors to removed README sections, no new relative links leaving the published set (readme-extraction §Verifying conformance), and `tap/all` / the `all` meta-formula absent from `README.md` and `docs/site/**` except `docs/site/standards/install-composition.md`'s own Precedent line (out-of-scope keep).

- **GIVEN** the completed edits
- **WHEN** the verification greps run (`grep -n "](#" README.md` targets vs existing headings; `grep -rn "](\./\|](\.\./\|](docs/" README.md docs/site/`; `grep -rn "tap/all\|meta-formula" README.md docs/site/`)
- **THEN** every result is clean or an intake-sanctioned keep

## Tasks

### Phase 1: Core Implementation

- [x] T001 In `README.md` `## Install`: replace the full requirements paragraph with a 1–2 sentence version (Homebrew ≥ 6.0.4, 6.0.0–6.0.3 → `brew update` first, never auto-installs brew) and trim the `shll agent-setup` explainer to a short form keeping the run-kit link; keep the 4-line block and subset variant byte-identical <!-- R1 -->
- [x] T002 In `README.md`: remove the `> **Why brew trust first?**` blockquote, `### Manual bootstrap (brew)` (incl. the `all` sentence), and `### From source`; replace the "For the full guide — brew vs `all`…" line with the closing pointer paragraph linking `[install guide](docs/site/install.md)` and `https://shll.ai` <!-- R2, R3 -->
- [x] T003 In `README.md`: remove the entire `## Troubleshooting` section; reword the `## Reference` entry description for `docs/site/install.md` to drop "brew vs `all`" <!-- R2 -->
- [x] T004 [P] In `docs/site/install.md` §Bootstrap via Homebrew: remove the `all` meta-formula paragraph, its code block, and the "Use the single formula when…" guidance paragraph <!-- R4 -->
- [x] T005 [P] In `docs/site/workflows.md` §Clean-machine bootstrap step 1: remove the trailing "Or `brew trust … tap/all …` … no-op." sentence from the parenthetical, keeping the version-floor sentence <!-- R5 -->

### Phase 2: Verification

- [x] T006 Run the verification greps across `README.md` + `docs/site/`: no dangling `](#…)` anchors in README, no new relative links leaving the published set, `tap/all`/meta-formula absent except install-composition.md's Precedent line; confirm `git diff --stat` touches only the three files <!-- R6, R7 -->

## Execution Order

- T001–T003 are sequential edits to the same file (README.md); T004 and T005 are independent and parallelizable
- T006 runs last, after all edits

## Acceptance

### Functional Completeness

- [ ] A-001 R1: README Install keeps the 4-line block + subset variant unchanged, with a 1–2 sentence requirements statement and a trimmed agent-setup explainer that still links run-kit
- [ ] A-002 R3: README Install ends with a pointer paragraph linking both `docs/site/install.md` (repo-relative) and https://shll.ai
- [ ] A-003 R4: install.md no longer documents the `all` meta-formula; all other sections of the page are unchanged
- [ ] A-004 R5: workflows.md step 1 parenthetical keeps the Homebrew-floor sentence and drops the `all` alternative

### Removal Verification

- [ ] A-005 R2: README carries no "Why brew trust first?" blockquote, no Manual bootstrap subsection, no From source subsection, no Troubleshooting section, and no `all` meta-formula mention; the Reference entry for install.md remains (reworded)
- [ ] A-006 R2: no README link targets a removed anchor (grep `](#` — every target resolves to a surviving heading)

### Scenario Coverage

- [ ] A-007 R7: verification greps are clean — no new relative links leaving the published set; `tap/all` appears nowhere in README/docs/site; `meta-formula` survives only in docs/site/standards/install-composition.md

### Edge Cases & Error Handling

- [ ] A-008 R6: `git diff` shows exactly three files changed (README.md, docs/site/install.md, docs/site/workflows.md); `docs/site/standards/`, `docs/site/skill.md`, Go source, and scripts are untouched — no embedded-doc drift-guard implicated

### Code Quality

- [ ] A-009 Pattern consistency: surviving README/docs prose keeps the existing voice and formatting conventions (em-dash asides, bold leads, fenced `sh` blocks)
- [ ] A-010 No unnecessary duplication: the README no longer duplicates any paragraph that lives on docs/site/install.md; install.md remains the single in-repo home of full install detail

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Reword (not delete) the `## Reference` entry description for install.md, dropping "brew vs `all`" | Intake keeps the entry but Assumption 4 drops all `all` documentation; the description naming a retired formula would be stale — rewording preserves the entry while honoring the drop | S:70 R:90 A:85 D:80 |
| 2 | Confident | Skip the apply-time tap-state sanity check from intake Assumption 4 | That check gates wording anything as "removed"; this change only deletes `all` mentions and asserts nothing about the tap's current state, so there is nothing to verify | S:60 R:90 A:85 D:80 |
| 3 | Confident | The trimmed agent-setup explainer keeps both the skill-placement gist and the run-kit link in ~2 sentences | Intake: "keep at least a one-line version with the link it carries today" — the run-kit repo link is the one markdown link in that paragraph | S:65 R:90 A:85 D:75 |
| 4 | Certain | install.md's intro line ("The README's Install section is the short version…") stays unchanged | Intake pre-resolves this: the statement becomes *more* true after the README slims | S:80 R:95 A:90 D:85 |

4 assumptions (1 certain, 3 confident, 0 tentative).
