# Plan: README clarity pass — dedupe trust-tap, orient outsiders

**Change**: 260609-oghj-readme-clarity-pass
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### Docs: De-duplicate the `--trust-tap` explanation

#### R1: Canonical home is the `#### --trust-tap` command subsection
The full `--trust-tap` explanation MUST remain in exactly one canonical location: the
`#### `--trust-tap` — resolve the Homebrew tap-trust warning` command subsection (the `brew trust`
ceremony, the `HOMEBREW_REQUIRE_TAP_TRUST=1` export, composition with `--print`/`--uninstall`, the
graceful degradation when `brew trust` is unavailable, and the `--uninstall`-does-not-`brew untrust`
note). The heading text and its anchor `#--trust-tap--resolve-the-homebrew-tap-trust-warning` MUST
NOT change.

- **GIVEN** the README has three places explaining `--trust-tap`
- **WHEN** the clarity pass is applied
- **THEN** the command subsection retains the complete explanation unchanged
- **AND** its heading text (and therefore its anchor) is untouched

#### R2: Quick-start callout trims to a one-liner + canonical link
The Quick-start `--trust-tap` callout paragraph MUST be reduced to roughly one sentence that states
what `--trust-tap` does and that you can drop it, with an intra-page link to the canonical
subsection using the exact anchor `#--trust-tap--resolve-the-homebrew-tap-trust-warning`. The
existing Troubleshooting link for lighter alternatives MAY be preserved.

- **GIVEN** the Quick-start callout currently re-explains the `HOMEBREW_REQUIRE_TAP_TRUST=1` side
  effect on other taps and the drop-the-flag alternative in full
- **WHEN** the pass is applied
- **THEN** the callout is a one-liner pointing to the canonical subsection
- **AND** the link anchor resolves to the canonical heading

#### R3: Troubleshooting recommended-fix trims to command + pointer; alternatives table preserved
The Troubleshooting "Recommended fix — record genuine trust" prose MUST be reduced to the command
plus a pointer to the canonical subsection. The "Lighter alternatives" env-var table and the
"shll will **not** set these for you" note MUST be kept intact (they are troubleshooting-specific and
not duplicated in the command subsection). The existing link to the canonical subsection MUST be
preserved.

- **GIVEN** the recommended-fix prose re-explains what `brew trust` and `HOMEBREW_REQUIRE_TAP_TRUST=1`
  do
- **WHEN** the pass is applied
- **THEN** the recommended-fix is the command + a pointer to the canonical subsection
- **AND** the "Lighter alternatives" table and the "shll will not set these" note are unchanged

#### R4: All trust-tap anchors still resolve after editing
After editing, every `#--trust-tap…` and `#tap-sahil87tap-is-allowed-by-default-warning` anchor
reference in the README MUST match a real heading in the same file.

- **GIVEN** anchor links to `#--trust-tap--resolve-the-homebrew-tap-trust-warning` and
  `#tap-sahil87tap-is-allowed-by-default-warning`
- **WHEN** a grep-based anchor check is run over the edited README
- **THEN** every anchor reference maps to an existing heading (slugified)

### Docs: One-line "what it's for" gloss per roster tool

#### R5: Add a "What it's for" column to the shell-init contribution table
The existing shell-init contribution table (header `| Tool | What it adds to your shell |`) MUST gain
a new column giving a one-line "what it's for" gloss per tool, in roster order (hop, wt, tu, idea, rk,
fab-kit). The table's intro sentence MUST be updated so it covers both "what it is" and "what it
adds". The existing "what it adds to your shell" content MUST be preserved.

- **GIVEN** the table currently has only a "What it adds to your shell" column
- **WHEN** the pass is applied
- **THEN** a "What it's for" column is added with one short gloss per tool in roster order
- **AND** the intro sentence frames both dimensions

#### R6: Tool glosses are factually accurate (verified, not guessed)
Each gloss MUST describe the tool's actual purpose, verified against `<tool> --help` and/or the
tool's own GitHub repo — not the intake's placeholder text. Specifically the `tu`, `idea`, and `rk`
glosses MUST reflect verified purpose.

- **GIVEN** the intake's `tu`/`idea`/`rk` glosses are flagged placeholders
- **WHEN** each tool's purpose is verified
- **THEN** the shipped gloss matches the verified purpose (notably `tu` = AI coding-assistant cost
  tracker, not a "terminal/task utility")

### Non-Goals

- No edits to any file other than `README.md` (no `cmd/`, no `internal/`, no `docs/site/`).
- No behavior, flag, or command-surface changes.
- Review items 3–6 (license/CONTRIBUTING/requirements sections, `version` example staleness,
  Quick-start `all`-path note, "roster" term definition) are out of scope.
- The optional line-5 toolkit-framing half-clause is not required; only the per-tool glosses are the
  required deliverable for orientation (kept out unless trivially clean).

### Design Decisions

1. **Canonical-home + pointers over deletion or triplication**: keep the full explanation once
   (command subsection), link from the other two — *Why*: the trust-tap detail is useful where a
   reader hits the warning; single source avoids three-way sync — *Rejected*: deleting the detail
   (loses useful context) and leaving it triplicated (sync burden).
2. **Glosses as a new table column over a separate list**: one source of truth — *Why*: avoids a
   second list to keep in sync (intake Assumption #7, user-confirmed) — *Rejected*: separate list,
   or both.

## Tasks

### Phase 2: Core Implementation

- [x] T001 Verify each roster tool's purpose (`hop`, `wt`, `tu`, `idea`, `rk`, `fab-kit`) via `<tool> --help` and/or its GitHub repo; record the authoritative one-liner for each <!-- R6 -->
- [x] T002 Edit `README.md` Quick-start `--trust-tap` callout: trim to a one-liner + link to `#--trust-tap--resolve-the-homebrew-tap-trust-warning` (keep the Troubleshooting lighter-alternatives link) <!-- R2 -->
- [x] T003 Edit `README.md` Troubleshooting "Recommended fix" prose: reduce to the command + pointer to the canonical subsection; leave the "Lighter alternatives" table and the "shll will not set these" note untouched <!-- R3 -->
- [x] T004 Edit `README.md` shell-init contribution table: add a "What it's for" column (verified glosses, roster order) and update the intro sentence to cover "what it is" + "what it adds" <!-- R5 -->

### Phase 3: Integration & Edge Cases

- [x] T005 Grep-check that the command subsection heading is unchanged and every `#--trust-tap…` / `#tap-sahil87tap-is-allowed-by-default-warning` anchor reference resolves to a real heading <!-- R1 R4 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The `#### --trust-tap` command subsection retains the full explanation, with its heading text (and anchor) unchanged
- [x] A-002 R2: The Quick-start `--trust-tap` callout is a one-liner with a link to the canonical subsection
- [x] A-003 R3: The Troubleshooting recommended-fix is the command + pointer; the "Lighter alternatives" table and "shll will not set these" note are intact
- [x] A-004 R5: The shell-init table has a "What it's for" column with one gloss per tool in roster order, plus an updated intro sentence
- [x] A-005 R6: Each gloss matches verified tool purpose — `tu` reflects AI coding-assistant cost tracking (not "terminal/task utility"); `idea` and `rk` reflect their verified purposes

### Behavioral Correctness

- [x] A-006 R2 R3: No `--trust-tap` explanatory prose is duplicated outside the canonical subsection (only one-liners + pointers remain in Quick-start and Troubleshooting)

### Scenario Coverage

- [x] A-007 R4: A grep over the edited README confirms every `#--trust-tap--resolve-the-homebrew-tap-trust-warning` and `#tap-sahil87tap-is-allowed-by-default-warning` anchor maps to an existing heading

### Edge Cases & Error Handling

- [x] A-008 R1: shll.ai extraction contract not broken — no content added to denylisted/footer sections; edits stay within surviving sections (Quick start, shell-init table, Troubleshooting prose)

### Code Quality

- [x] A-009 Pattern consistency: New prose and the table column follow the README's existing style and markdown conventions
- [x] A-010 No unnecessary duplication: The clarity pass reduces duplication (its primary goal) and introduces none

## Notes

- Check items as you review: `- [x]`
- README.md is the only file edited; "tests" = the grep-based anchor/heading check (T005 / A-007)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Canonical home for the full `--trust-tap` explanation is the command subsection; Quick-start + Troubleshooting trim to one-liner + link | Intake Assumption #1, user-confirmed | S:98 R:80 A:90 D:95 |
| 2 | Certain | Per-tool glosses placed as a new "What it's for" column in the shell-init table | Intake Assumption #7, user-confirmed (column over separate-list/both) | S:95 R:85 A:90 D:90 |
| 3 | Certain | Keep Troubleshooting's "Lighter alternatives" table and "shll will not set these" note; only the recommended-fix prose is trimmed | Intake Assumption #5, user-confirmed | S:95 R:80 A:85 D:85 |
| 4 | Confident | `tu` gloss = "AI coding-assistant cost/usage tracker" — corrects the intake placeholder ("terminal/task utility"), which was wrong | Verified via `tu --help` (sources: Claude Code/Codex/OpenCode; daily/monthly cost) AND the tu GitHub repo tagline "AI coding assistant cost tracking CLI" | S:95 R:80 A:90 D:85 |
| 5 | Confident | `idea` gloss = "worktree-aware idea / backlog capture from the terminal (markdown-first)" — refines the intake placeholder | Verified via `idea --help` ("Backlog idea management (current worktree)") AND the idea GitHub repo tagline "worktree-aware idea capture and backlog tracker — markdown-first, CLI-native" | S:95 R:80 A:90 D:85 |
| 6 | Confident | `rk` gloss = "run-kit — web-based tmux orchestration for parallel agent workspaces" — confirms the intake placeholder | Verified via `rk --help` ("tmux session manager with web UI") AND the run-kit GitHub repo tagline "Web-based tmux orchestration dashboard for long-running AI agent tasks" | S:95 R:80 A:90 D:85 |
| 7 | Confident | `hop` gloss = "fast directory navigation / bookmarks (`cd` on steroids)"; `wt` = "git worktree manager — create, switch, clean up worktrees"; `fab-kit` = "`fab` — spec-driven change workflow" | hop from context.md + existing shell-init row (its `--help` is shadowed by the shell function); wt from `wt --help` ("Git worktree management — create, list, open, delete"); fab from `fab --help` ("workspace & workflow toolkit") | S:90 R:80 A:90 D:85 |
| 8 | Certain | Edits stay within README sections that survive the shll.ai extraction contract (Quick start, shell-init table, Troubleshooting prose) — no denylisted/footer content touched | Per change 260608-xgc0: README is conformant; these sections are non-denylisted and edits add no images/footer headings | S:95 R:80 A:90 D:90 |

8 assumptions (4 certain, 4 confident, 0 tentative).
