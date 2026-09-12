# Plan: Standards consumer-site sweep (D14 second pass)

**Change**: 260912-dk8f-standards-consumer-site-sweep
**Intake**: `intake.md`

## Requirements

### Standards: consuming-site identity

#### R1: The nine standards name hexokit.com as the consuming site and the HexoKit toolkit as the family
Every `docs/site/standards/*.md` page MUST contain zero occurrences of the string `shll.ai`. The nine intro phrases `[shll toolkit](https://shll.ai)` SHALL become `[HexoKit toolkit](https://hexokit.com/toolkit/)`; the three `[shll](https://shll.ai)` self-links SHALL point at `https://hexokit.com/shll/`; `install-composition.md`'s `[`shll install`](https://shll.ai)` SHALL point at `https://hexokit.com/shll/install/`; every consuming-site prose mention (`pulls`, `renders`, `validates`, `vendors`, `owned by`, `round-trip to`, `is …'s job`) SHALL say `hexokit.com`; the two machine-anchor contract links and the specs-tree link SHALL point at `https://github.com/sahil87/hexokit-site/...`; readme-extraction rule 8's URL SHALL be `https://hexokit.com/<tool>/commands/`; `skill.md:49`'s commands-page link SHALL be `the tool's [hexokit.com commands page](https://hexokit.com/toolkit/)`. The exact per-line before/after table is intake § What Changes A1–A5.

- **GIVEN** the nine canonical standards pages after apply
- **WHEN** `grep -c "shll\.ai" docs/site/standards/*.md` runs
- **THEN** every file reports `0`
- **AND** every outbound link swapped is an absolute `https://…` URL and every intra-family link is unchanged (readme-extraction closure rule 1)

#### R2: The embedded standards copies are byte-identical to the canonical pages
After R1, `scripts/sync-standards.sh` (via `just sync-standards`) SHALL regenerate `src/cmd/shll/standards/*.md`, and the regenerated set MUST be committed so `TestStandardsEmbedMatchesCanonical` passes. `src/cmd/shll/skill/skill.md` MUST be unchanged by the sync (its canonical source has no `shll.ai` left).

- **GIVEN** R1 is applied
- **WHEN** `just sync-standards` runs and `go test ./cmd/shll/ -run 'Embed'` runs from `src/`
- **THEN** exactly the nine standards copies change and both embed drift-guard tests pass

### shll docs: install surface and host

#### R3: shll's own live docs point at hexokit.com and describe the product-first install correctly
`README.md`, `docs/site/install.md`, `docs/site/workflows.md`, and the header comment of `scripts/install.sh` MUST contain no `shll.ai` mention except a parenthetical naming shll.ai as the `versions.json` / `/install` byte-copy fallback. The install one-liner MUST read `curl -fsSL https://hexokit.com/install | sh` and MUST NOT claim it installs the whole roster; it SHALL be followed by a `shll install` line described as installing the rest of the toolkit. The subset and flag-passthrough examples SHALL keep their semantics with the host flipped. README line 296 SHALL state the pull-based help-tree truth (hexokit.com pulls daily; no "CI publishes"). The exact per-line edits are intake § What Changes C1–C4.

- **GIVEN** the four files after apply
- **WHEN** `grep -n "shll\.ai" README.md docs/site/install.md docs/site/workflows.md scripts/install.sh` runs
- **THEN** the only matches are README lines 93/272's "shll.ai fallback/byte copy" parentheticals and `scripts/install.sh`'s "(shll.ai/install is a byte copy)" line
- **AND** the README's H1 → blockquote → badges order is intact and `docs/site/install.md` is still linked repo-relatively (readme-extraction rules 1 and 8)

### Binary prose: family name

#### R4: Help text, the self-description constant, and the placed skill say "HexoKit toolkit"
Every non-test `src/**/*.go` occurrence of the phrase `shll toolkit` (help `Short`/`Long` strings in `root.go`, `setup.go`, `list.go`, `doctor.go`, `uninstall.go`, `check_updates.go`, `standards.go`; the `shllSelfDescription` constant in `tools.go`; the placed `SKILL.md` H1, intro sentence, and `agentSkillDescription()` prefix in `agent_setup.go`; doc comments in `tools.go`, `setup.go`, `main.go`, `changelog.go`) SHALL become `HexoKit toolkit`. `skillDirName = "shll-toolkit"`, the frontmatter `name:`, `manifestURLFallback`, and every other `shll.ai` mention in `src/` MUST be unchanged. `go test ./...` from `src/` MUST pass.

- **GIVEN** the Go sources after apply
- **WHEN** `grep -rn "shll toolkit" src --include='*.go' | grep -v _test.go` runs
- **THEN** it prints nothing
- **AND** `grep -n 'skillDirName = "shll-toolkit"' src/cmd/shll/agent_setup.go` still matches
- **AND** `cd src && go test ./...` passes

### Project context

#### R5: `fab/project/` identity lines name the HexoKit toolkit and hexokit.com
`fab/project/config.yaml`'s `project.description`, `fab/project/context.md` lines 5/7/29, and `fab/project/constitution.md` § Tool Roster Source of Truth and § Toolkit Standards SHALL be edited exactly per intake § What Changes E; the constitution's Governance line SHALL read `**Version**: 1.1.1 | **Ratified**: 2026-05-09 | **Last Amended**: 2026-09-12`.

- **GIVEN** the three project files after apply
- **WHEN** `grep -n "shll\.ai\|shll toolkit" fab/project/config.yaml fab/project/context.md fab/project/constitution.md` runs
- **THEN** the only matches are context.md's rewritten line 7 (which names shll.ai as the redirect host and `sahil87/shll.ai` as the stub repo)

### Non-Goals

- No rename of any standard, file, command, skill directory, or rc sentinel (D14); roster fields untouched (R1/R2 of the plan).
- No change to `versions.go` / `check_updates.go` network behavior or their shll.ai fallback text (D4).
- No edit to the hexokit-site composition block or its `$# -eq 0` check (cross-repo, recorded in the intake).
- No edits under `fab/changes/archive/`, `fab/plans/`, or memory `log.md` history (D11).

## Tasks

### Phase 1: Standards (the plan-row scope)

- [x] T001 Apply intake § A1–A5 to the nine `docs/site/standards/*.md` pages: the nine intro phrases, the three `[shll]` self-links, the `install-composition` `shll install` link, the 18 consuming-site prose lines, the two contract links and the specs-tree link, rule 8's URL, and `skill.md:49`'s commands link; verify `grep -c "shll\.ai" docs/site/standards/*.md` is 0 everywhere <!-- R1 -->
- [x] T002 Run `just sync-standards`; confirm exactly `src/cmd/shll/standards/*.md` (9 files) changed and `src/cmd/shll/skill/skill.md` did not; run `cd src && go test ./cmd/shll/ -run 'Embed|Skill'` <!-- R2 -->

### Phase 2: shll's own live docs

- [x] T003 Apply intake § C1–C4 to `README.md` (lines 7, 14 → three-line block, 21, 28, 30, 93, 272, 290, 296), `docs/site/install.md` (lines 3, 10, 14 → two lines, 20, 36, 194), `docs/site/workflows.md` (lines 3, 10 → two aligned lines, 98), and the `scripts/install.sh` header comment (lines 2–5) <!-- R3 -->

### Phase 3: Binary prose and project context

- [x] T004 Flip every non-test `shll toolkit` in `src/**/*.go` to `HexoKit toolkit` (root.go, tools.go incl. `shllSelfDescription`, setup.go, list.go, doctor.go, uninstall.go, check_updates.go, standards.go, agent_setup.go H1/intro/description prefix, main.go, internal/changelog/changelog.go); leave `skillDirName`, the frontmatter `name:`, and all `shll.ai` fallback text; update README sample-output lines 185/237 to match; run `cd src && go test ./...` <!-- R4 -->
- [x] T005 Apply intake § E to `fab/project/config.yaml`, `fab/project/context.md`, and `fab/project/constitution.md` (two identity lines + Governance → 1.1.1 / 2026-09-12); run the final sweep grep from intake § Impact "Tests to run" and confirm only the enumerated survivors remain <!-- R5 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `grep -c "shll\.ai" docs/site/standards/*.md` reports 0 for all nine files, and the nine intros read `[HexoKit toolkit](https://hexokit.com/toolkit/)`
- [x] A-002 R2: `src/cmd/shll/standards/*.md` are byte-identical to `docs/site/standards/*.md` (`TestStandardsEmbedMatchesCanonical` passes) and `src/cmd/shll/skill/skill.md` is unchanged (`TestSkillEmbedMatchesCanonical` passes)
- [x] A-003 R3: README, install.md, workflows.md, and install.sh contain `shll.ai` only in the enumerated fallback/byte-copy parentheticals; the install block is the hexokit.com bootstrap + `shll install` two-step
- [x] A-004 R4: no non-test `shll toolkit` remains in `src/`; `skillDirName` and `manifestURLFallback` unchanged
- [x] A-005 R5: `fab/project/` identity lines match intake § E; constitution Governance reads 1.1.1 / 2026-09-12

### Behavioral Correctness

- [x] A-006 R3: the README/install.md/workflows.md install prose no longer claims the one-liner installs the whole roster, and states that `shll install` installs the rest
- [x] A-007 R4: `cd src && go test ./...` passes, including the agent-setup description tests and the help-dump fidelity test

### Removal Verification

- [x] A-008 R1: the three `https://github.com/sahil87/shll.ai/...` links are gone from the standards (replaced, not deleted)

### Scenario Coverage

- [x] A-009 R1: the readme-extraction closure rule holds — every intra-family `[name](name.md)` link in `docs/site/standards/` is unchanged and every swapped link is an absolute `https://…` URL (spot-check via `git diff -- docs/site/standards | grep '^[-+]' | grep -c '](http'` equal counts of removed and added absolute links)

### Edge Cases & Error Handling

- [x] A-010 R3: the `scripts/install.sh` edit touches comment lines only — `git diff -- scripts/install.sh | grep '^[-+]' | grep -v '^[-+]#' | grep -v '^[-+][-+]'` prints nothing

### Code Quality

- [x] A-011 Pattern consistency: edits preserve surrounding punctuation (em-dashes, backticks, trailing periods) and column alignment in the workflows.md code block
- [x] A-012 No unnecessary duplication: no new constants, files, or helpers introduced; `shllSelfDescription` remains the single source for the self row

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
- The task grouping is deliberately five tasks (light lane): each is one focused, mechanically verifiable edit set.
- Review-cycle notes: the placed-skill description prefix reads `Use when driving any HexoKit toolkit CLI or shll — ` (the intake table said `…or shll itself — `; "itself" was dropped to stay under the agentskills.io 1024-byte cap, which the 3-byte family-name growth had crossed). The review's should-fix on `.github/formula-template.rb` `desc` (the same identity sentence, shipped to the tap on release) was taken; `docs/site/install.md` comment alignment fixed.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Five tasks, phase-ordered standards → docs → binary/project; no test additions | Every edit is prose; existing drift-guard and description tests are the coverage; intake § Impact names them | S:85 R:95 A:90 D:90 |
| 2 | Confident | The workflows.md new `shll install` line keeps the existing 73-column comment alignment of that code block | The block aligns comments at one column today; a misaligned line would read as a typo on the rendered page | S:60 R:95 A:85 D:80 |
| 3 | Confident | README lines 185/237 (sample output) flip with `shllSelfDescription` in T004 rather than T003 | They are renderings of the constant, so they belong with the constant's edit | S:70 R:95 A:90 D:85 |

3 assumptions (1 certain, 2 confident, 0 tentative).
