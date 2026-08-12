# Plan: Update Standard — Prompt-Free, Unconditionally

**Change**: 260812-igh5-update-standard-prompt-free
**Intake**: `intake.md`

## Requirements

### Standards: update.md prompt-free clause

#### R1: New "Prompt-free, unconditionally" section in the update standard
`docs/site/standards/update.md` MUST gain a new `##` section titled "Prompt-free, unconditionally", placed immediately after `## Invocation contract` and before `## Advertise and honor --skip-brew-update`. The section MUST state, in the page's house register (bold-MUST bullets, em-dash asides):

- The obligation: `<tool> update` MUST run to completion without any interactive prompt in **every** environment — including when stdin is a TTY. No confirmation question, no pager, no "press enter to continue". The obligation extends to wrapped subprocesses (a wrapped `brew` call must be invoked non-interactively).
- An explicit note that this deliberately tightens principles.md №1 for this one subcommand: №1's TTY reconciliation permits `Proceed? [y/N]` on a TTY, but `shll update` delegates with inherited stdio, so stdin typically IS a TTY mid-compose — a №1-conformant prompt stalls the delegation loop.
- The rationale that no confirmation is needed at all: an in-place upgrade is not a destructive write in the №5 sense — invoking `update` is itself the consent; a tool that wants a guard can offer `--dry-run`, never a prompt.
- A **Failure mode.** paragraph mirroring the brew-timeout clause's shape: a tool whose `update` prompts only on a TTY is conformant to №1 and every other rule on this page, yet it breaks the compose in both directions. **The paragraph MUST NOT claim a №1-conformant tool hangs in headless runs** — №1 itself requires a fast non-TTY refusal, so that claim is self-contradictory. The two accurate outcomes to name instead: (a) **the pty case** — an agent driving `shll update` in a tmux/run-kit pane has a real TTY on stdin with no human watching, so the prompt hangs invisibly until the harness times out; (b) **the true non-TTY case** — the tool refuses fast per №1, and because `shll update`'s delegated argv is fixed (`<tool> update [--skip-brew-update]` — no way to thread `--yes`), the compose hard-fails with no recourse for the caller.
- Prose calibration for the interactive-compose harm: because stdio is inherited, an attended run DOES show the prompt and a watching user can answer it — do not claim "no one knows a keystroke is expected". The accurate harm is that the composed run silently acquires an N-interaction requirement and blocks indefinitely whenever nobody is watching (a walked-away `shll update`, an agent-driven pane).
- House shape: introduce the section's bullets with a colon-terminated lead-in line, matching every other bulleted section on the page ("Two rules with teeth:", "So:").

- **GIVEN** the canonical `docs/site/standards/update.md`
- **WHEN** the page is read after this change
- **THEN** a `## Prompt-free, unconditionally` section appears between `## Invocation contract` and `## Advertise and honor --skip-brew-update`, carrying the MUST obligation (TTY included, wrapped subprocesses included), the stricter-than-№1 note, the №5 consent rationale, and a **Failure mode.** paragraph in the page's house style that names the pty case and the fast-refusal/fixed-argv case — with no claim that a №1-conformant tool hangs headless

#### R2: Matching bullet in the "Verifying conformance" checklist
The `## Verifying conformance` section of the same file MUST gain one bullet stating that `<tool> update` runs to completion with no interactive prompt in any environment, TTY included — no code path reads stdin for a confirmation.

- **GIVEN** the `## Verifying conformance` checklist in `docs/site/standards/update.md`
- **WHEN** a tool author walks the checklist before shipping an `update` change
- **THEN** a bullet requires that no code path of `<tool> update` reads stdin for a confirmation, in any environment including a TTY

#### R4: principles.md contract-table row and page summaries reflect the new clause
The `update` row of the contract table in `docs/site/standards/principles.md` MUST list №1 in its Implements column (alongside №7 and №6), and the two prose summaries of the update page (the companion summary near the top and the row description) MUST mention the prompt-free clause. **№1's own clause text is NOT touched** — this is the same-page precedent: the brew-handling clause refines №6 and №6 is listed in that column.

- **GIVEN** `docs/site/standards/principles.md` after this change
- **WHEN** the contract table's `update` row and the page-summary lines are read
- **THEN** the row's Implements column includes №1 and both summaries name the prompt-free clause, while №1's own section text is byte-unchanged

#### R3: Embedded copy re-synced and drift guard passing
The embedded copies at `src/cmd/shll/standards/` (now `update.md` AND `principles.md`) MUST be refreshed via `scripts/sync-standards.sh` after the canonical edits, and `TestStandardsEmbedMatchesCanonical` MUST pass, confirming the embeds byte-match the canonical sources.

- **GIVEN** the edited canonical `docs/site/standards/update.md`
- **WHEN** `scripts/sync-standards.sh` runs and `go test ./cmd/shll/ -run TestStandardsEmbedMatchesCanonical` executes from `src/`
- **THEN** `src/cmd/shll/standards/update.md` is byte-identical to the canonical file and the test passes

### Non-Goals

- `principles.md` №1's own clause text is NOT modified — the stricter rule lives on `update.md` only (mirrors how the brew-handling clause refines №6 on this same page). The only `principles.md` diff permitted is R4's contract-table row + page-summary refresh.
- No Go source changes; no CLI surface changes; no roster-tool code (the six tools' conformance is enforced in their own repos)
- `shll uninstall`'s TTY-gated prompt remains the №1 reference implementation, unaffected (not an `update` flow)

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add the `## Prompt-free, unconditionally` section to `docs/site/standards/update.md` immediately after `## Invocation contract`, in the page's house register (bold-MUST bullets, em-dash asides, **Failure mode.** paragraph); check whether the page intro needs a coherent touch <!-- R1 --> <!-- rework: review must-fix — rewrite the Failure-mode paragraph per R1's corrected fourth bullet (pty case + fast-refusal/fixed-argv case; NO headless-hang-while-conformant claim); recalibrate the keystroke phrasing per R1's prose-calibration bullet; add the colon-terminated bullet lead-in -->
- [x] T002 Add the prompt-free bullet to `## Verifying conformance` in `docs/site/standards/update.md` <!-- R2 -->
- [x] T005 Update `docs/site/standards/principles.md`: add №1 to the `update` row's Implements column and mention the prompt-free clause in the row description and the companion page summary; №1's own section text stays byte-unchanged <!-- R4 -->

### Phase 3: Integration & Edge Cases

- [x] T003 Run `scripts/sync-standards.sh` to refresh the embedded copies at `src/cmd/shll/standards/` (update.md and principles.md) <!-- R3 --> <!-- rework: re-sync after the cycle-1 edits -->
- [x] T004 Run `go test ./cmd/shll/ -run TestStandardsEmbedMatchesCanonical` from `src/` and confirm it passes <!-- R3 --> <!-- rework: re-run after re-sync -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `docs/site/standards/update.md` carries a `## Prompt-free, unconditionally` section between `## Invocation contract` and `## Advertise and honor --skip-brew-update`, containing the unconditional MUST (TTY included), the wrapped-subprocess extension, the stricter-than-№1 note with the inherited-stdio compose rationale, the №5 invoking-is-consent rationale (with `--dry-run` as the sanctioned guard), and a **Failure mode.** paragraph
- [x] A-002 R2: `## Verifying conformance` in the same file carries a bullet requiring no interactive prompt in any environment, TTY included — no code path reads stdin for a confirmation
- [x] A-003 R3: `src/cmd/shll/standards/update.md` and `src/cmd/shll/standards/principles.md` are byte-identical to their canonical sources and `TestStandardsEmbedMatchesCanonical` passes
- [x] A-008 R4: The `update` row of principles.md's contract table lists №1 in its Implements column, both page summaries mention the prompt-free clause, and №1's own section text is byte-unchanged

### Behavioral Correctness

- [x] A-004 R1: The new section deliberately tightens №1 for the `update` subcommand only — №1's own clause text is unchanged; the only diff outside `update.md` and the embeds is R4's principles.md contract-table row + page-summary refresh

### Scenario Coverage

- [x] A-005 R1: The **Failure mode.** paragraph mirrors the brew-timeout clause's register — names how a tool can be conformant to №1 and every other rule on this page while breaking the compose via the pty case (real TTY, no human — invisible hang) and the true non-TTY case (fast №1 refusal against a fixed delegated argv with no `--yes` to thread — hard fail, no recourse). It makes NO claim that a №1-conformant tool hangs headless

### Code Quality

- [x] A-006 Pattern consistency: The new section matches the page's existing voice and structure (single `##` heading, colon-terminated bullet lead-in, bold-MUST bullets, em-dash asides, named **Failure mode.** paragraph)
- [x] A-007 No unnecessary duplication: The clause references №1/№5 by link rather than restating their full text; no shll consumer machinery is duplicated into the producer-facing page

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Section title is exactly "Prompt-free, unconditionally" | Intake says "titled along the lines of"; this is the intake's own phrasing, matches the page's sentence-case heading convention, trivially renameable | S:70 R:95 A:85 D:80 |
| 2 | Confident | Page intro and scope paragraphs left untouched; only the new section + checklist bullet added | Intake asks to "check whether the intro needs a coherent touch" — the intro already frames delegation/composition, which is exactly the new clause's motivation, so no edit is needed | S:60 R:90 A:80 D:75 |
| 3 | Certain | "No pager" and "press enter to continue" named as prohibited forms alongside confirmation questions | Intake's What Changes item 1 lists them verbatim | S:90 R:95 A:95 D:95 |
| 4 | Confident | Bullet lead-in line is "So:" — the same word the brew-handling clause uses | R1's house-shape bullet asks for a colon-terminated lead-in "matching the page's other bulleted sections"; reusing the page's own connective is maximal voice consistency, and the intro paragraph's harm statement flows into it naturally | S:65 R:95 A:85 D:80 |
| 5 | Confident | update.md's implements-principle line (line 9) DOES declare the №1-tightening — "and its prompt-free clause tightens principle №1 (non-interactive by default) for this one subcommand" | Superseded during review: the re-review flagged the unchanged line as its sole should-fix (both reviewers converged), and the orchestrator applied the reviewer-suggested wording post-verdict, with embeds re-synced and the drift guard green | S:80 R:95 A:90 D:85 |

5 assumptions (1 certain, 4 confident, 0 tentative).
