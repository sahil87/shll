# Plan: Bulk-Op Operator Commands

**Change**: 260719-hh4t-bulk-op-operator-commands
**Intake**: `intake.md`

## Requirements

### Command Docs: Home & Trackability

#### R1: Docs live under `.claude/commands/`, tracked by git
The three bulk-op command docs SHALL be authored as Markdown slash-command docs under `.claude/commands/` — NOT under `.claude/skills/` (fab-sync-regenerated) and NOT as a `shll` Go subcommand (Constitution VII, Minimal Surface Area). The `.gitignore` MUST be narrowed so `.claude/commands/*.md` are trackable while `.claude/skills/**` and `.claude/settings.local.json` remain ignored.

- **GIVEN** the repo's `.gitignore` currently carries a bare `/.claude` line (excludes the directory itself, so any negation beneath it is dead)
- **WHEN** the line is replaced with `/.claude/*` followed by `!/.claude/commands/`
- **THEN** `git status --porcelain` lists `.claude/commands/*.md` as trackable (untracked-and-addable)
- **AND** `git check-ignore -v .claude/skills/<any> .claude/settings.local.json` reports both still ignored (matched by `/.claude/*`)

### Primitive: `bulk-shll-op`

#### R2: Generic bulk-operation primitive doc
`.claude/commands/bulk-shll-op.md` SHALL codify the per-repo spawn loop as a repeatable primitive that documents its parameters and the exact per-repo sequence, mirroring fab-operator §6's spawn sequence minus the operator's enrollment steps.

- **GIVEN** an operator (or any session with shll's commands loaded) invokes `/bulk-shll-op`
- **WHEN** the doc is read
- **THEN** it documents three inputs: **target repos** (default: the 7-repo roster `fab-kit, hop, idea, run-kit, shll, tu, wt`, resolved via `hop <repo> where` with a `~/code/sahil87/<repo>` fallback; an explicit subset may be passed), a **per-repo task** (a single prompt/slash-command delivered to each spawned agent), and a **PR-skip rule** (the dispatched prompt MUST instruct each agent to skip `/git-pr` when the recipe produced no diff)
- **AND** it specifies the per-repo loop: resolve main-worktree root → `wt create --non-interactive` (capture `<wt>` name + path) → read the target repo's session command via `fab agent --print --repo <repo-root>` → `tmux new-window -t <bulk-session> -n "<wt>-<repo>" -c <worktree-path> "<spawn_cmd> '<task>'"` with exactly **one** slash command per spawn (no `&&`-joined strings)

#### R3: Session, window, and no-registration conventions
The `bulk-shll-op` doc SHALL encode the grouping conventions and the no-registration rule.

- **GIVEN** a bulk task is invoked
- **WHEN** the loop runs
- **THEN** the doc directs creating one dedicated tmux session per bulk task (default name `bulk-<task-slug>`) and opening all per-repo windows in it, so `fab pane map --all-sessions` keeps them visible/groupable to the operator
- **AND** each window is named `<wt>-<repo>` (repo-name suffix, self-identifying; e.g. `swift-fox-hop`)
- **AND** the doc states the command writes NO operator state: no operator state file, no `branch_map` entries, no `»`/`›` marker renames — monitoring is the operator's one-directional job

### Preset: `bulk-shll-fab-upgrade`

#### R4: fab-upgrade preset doc
`.claude/commands/bulk-shll-fab-upgrade.md` SHALL be a thin zero-argument preset that invokes the `bulk-shll-op` recipe with the recurring fab-upgrade op.

- **GIVEN** `/bulk-shll-fab-upgrade` is invoked with no arguments
- **WHEN** the doc is read
- **THEN** it specifies: for each roster repo, in the fresh worktree run `fab upgrade-repo`, then drive the change through `/fab-fff`, shipping a PR **only if anything changed** (per R2's PR-skip rule)
- **AND** it identifies this as the exact op improvised ad hoc in change 260718, now a one-command invocation

### Preset: `bulk-shll-release`

#### R5: Patch-release preset doc — direct sequential loop
`.claude/commands/bulk-shll-release.md` SHALL release a patch version of every roster repo via `just release` as a **direct sequential per-repo loop** run by the invoking agent — NOT the spawn loop (no worktree, no spawned agent, no PR), because `just release` is tag-driven and changes no tracked files.

- **GIVEN** `/bulk-shll-release` is invoked
- **WHEN** the doc is read
- **THEN** it specifies the per-repo steps: resolve main-worktree root → verify clean tree and up-to-date `main` (fetch + compare against `origin/main`) → run `just release` (default bump: patch) in the repo root → report per-repo outcome (new tag or skip/error) in a summary table
- **AND** the doc notes all 7 roster repos carry a `release` justfile recipe and shll's reference implementation is tag-driven (CI cross-compiles, publishes the GitHub Release, bumps the Homebrew tap)

#### R6: Release preset — skip-unchanged rule
The `bulk-shll-release` doc SHALL skip repos with no new commits since the last tag (the release-only-if-changed analogue of the PR-skip rule) and report those skips.

- **GIVEN** a roster repo whose `HEAD` already carries the latest `v*` tag (an exact-match `git describe`)
- **WHEN** the release loop reaches that repo
- **THEN** the doc directs skipping the release for that repo and reporting it as a skip (a zero-commit release is pointless)

#### R7: Release preset — confirmation gate
The `bulk-shll-release` doc SHALL require explicit confirmation before the first tag push.

- **GIVEN** the loop has determined the per-repo next versions (and which repos it will skip)
- **WHEN** the agent is about to push the first tag
- **THEN** the doc directs presenting the repo → next-version list and obtaining explicit user confirmation first, because a tag push triggers CI releases across up to 7 repos (outward-facing, not quietly reversible)

### Non-Goals

- No multi-repo deployment of the command docs (bulk ops are invoked from a session with shll's commands loaded, never from inside each target repo's own session).
- No Go/CLI surface change, help-dump, README, or `docs/site/` change (Toolkit Standards clause not triggered).
- No fab-operator skill edits (fab-kit-owned and regenerated; upstreaming is a possible follow-up, not this change).
- No memory-file authoring in apply — `operator/bulk-op-commands` is created at hydrate.

### Design Decisions

#### Command docs, not skills or Go subcommand
**Decision**: Author the bulk-op recipes as slash-command docs under `.claude/commands/`.
**Why**: `.claude/skills/` is fully regenerated by fab-kit's embedded templates on every `fab sync`/`wt create`, so anything placed there is silently overwritten; `.claude/commands/` is untouched by fab sync. This also forces the conventions to live in the command docs rather than the fab-operator skill file.
**Rejected**: A `shll` Go subcommand — Constitution VII (Minimal Surface Area) would rightly bounce a new top-level command for operator prompt tooling, and no Go code is involved.
*Introduced by*: 260719-hh4t-bulk-op-operator-commands

#### `bulk-shll-release` is a direct loop, not a spawn loop
**Decision**: Run the release preset as a direct sequential per-repo loop in the invoking agent — no worktree, no spawned agent, no PR.
**Why**: `just release` (`scripts/release.sh`) is tag-driven and modifies no tracked files, so there is nothing to PR and a per-repo worktree adds nothing; a confirmation gate before the first push covers the outward-facing risk.
**Rejected**: Reusing the `bulk-shll-op` spawn loop — it would spin up worktrees and agents for an op that produces no diff and no PR, pure overhead.
*Introduced by*: 260719-hh4t-bulk-op-operator-commands

## Tasks

### Phase 1: Setup

- [x] T001 Narrow `.gitignore`: replace the bare `/.claude` line (line 41) with `/.claude/*` followed by `!/.claude/commands/` <!-- R1 -->
- [x] T002 Create the `.claude/commands/` directory (implicit on first file write) <!-- R1 -->

### Phase 2: Command Docs

- [x] T003 [P] Write `.claude/commands/bulk-shll-op.md` — the generic primitive: three documented inputs (target repos + default roster/resolution, per-repo task, PR-skip rule), the per-repo spawn loop (resolve root → `wt create --non-interactive` → `fab agent --print --repo` → `tmux new-window` with one slash command), and the session/window/no-registration conventions <!-- R2 R3 -->
- [x] T004 [P] Write `.claude/commands/bulk-shll-fab-upgrade.md` — zero-arg preset: `fab upgrade-repo` in fresh worktree per roster repo → `/fab-fff` → PR only if changed; references `bulk-shll-op` and change 260718 <!-- R4 -->
- [x] T005 [P] Write `.claude/commands/bulk-shll-release.md` — direct sequential release loop: resolve root → clean-tree + up-to-date `main` check → skip-if-no-new-commits-since-last-tag → `just release` (patch) → summary table, with the pre-first-push confirmation gate <!-- R5 R6 R7 -->

### Phase 3: Verification

- [x] T006 Verify gitignore narrowing: `git status --porcelain` shows `.claude/commands/*.md` trackable AND `git check-ignore -v` confirms `.claude/skills/**` + `.claude/settings.local.json` still ignored <!-- R1 -->

## Execution Order

- T001 blocks T006 (verification depends on the gitignore edit)
- T003, T004, T005 are independent (three separate new files); T002 is satisfied implicitly by the first of them

## Acceptance

### Functional Completeness

- [x] A-001 R1: `.gitignore` line 41's bare `/.claude` is replaced by exactly two lines — `/.claude/*` then `!/.claude/commands/` — and no `!/.claude/commands/**` third line or `settings.local.json` re-ignore is added
- [x] A-002 R2: `.claude/commands/bulk-shll-op.md` exists and documents the three inputs (target repos + default 7-repo roster with `hop <repo> where` resolution and `~/code/sahil87/<repo>` fallback, per-repo task, PR-skip rule) and the full per-repo spawn loop with one-slash-command-per-spawn
- [x] A-003 R3: `bulk-shll-op.md` encodes the one-session-per-bulk-task convention (default `bulk-<task-slug>`), the `<wt>-<repo>` window-name convention, and the explicit no-operator-registration rule
- [x] A-004 R4: `.claude/commands/bulk-shll-fab-upgrade.md` exists as a zero-arg preset specifying `fab upgrade-repo` → `/fab-fff` → PR-only-if-changed across the roster and cites change 260718
- [x] A-005 R5: `.claude/commands/bulk-shll-release.md` exists and specifies the direct sequential loop (resolve root → clean/up-to-date check → `just release` patch → summary table), explicitly not the spawn loop
- [x] A-006 R6: `bulk-shll-release.md` documents skipping repos with no new commits since the last tag and reporting the skip
- [x] A-007 R7: `bulk-shll-release.md` requires an explicit repo→next-version confirmation before the first tag push

### Scenario Coverage

- [x] A-008 R1: `git check-ignore -v` run against a representative `.claude/skills/**` path and `.claude/settings.local.json` reports both matched by `/.claude/*`, while `.claude/commands/*.md` are not ignored

### Code Quality

- [x] A-009 Pattern consistency: the three command docs share a consistent structure/tone and align with the fab-operator §6 spawn vocabulary they mirror (window/session conventions, `»`/`›` markers explicitly excluded)
- [x] A-010 No unnecessary duplication: the two preset docs reference the `bulk-shll-op` primitive rather than restating its full per-repo loop

## Notes

- Docs/config only — no Go source touched (`source_paths: cmd, internal` untouched), no Go tests to run.
- The `operator/bulk-op-commands` memory file is created at hydrate, not in apply.

## Deletion Candidates

None — this change adds new functionality without making existing code redundant (the `.gitignore` `/.claude` line was replaced in place, not orphaned).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Gitignore edit = replace bare `/.claude` with `/.claude/*` + `!/.claude/commands/`; no `**` third line, no `settings.local.json` re-ignore | Both approaches tested in a scratch repo during intake; results reproduced in intake § What Changes; verified live against this repo in T006 | S:90 R:95 A:100 D:95 |
| 2 | Certain | Docs live under `.claude/commands/` (not fab-sync-regenerated `.claude/skills/`, not a Go subcommand per Constitution VII) | Intake-explicit; skills-regeneration behavior and constitution rule both documented | S:95 R:90 A:95 D:95 |
| 3 | Certain | Three docs, user-named: `bulk-shll-op` primitive + `bulk-shll-fab-upgrade` + `bulk-shll-release` presets | User specified count and exact names (intake assumption 5) | S:95 R:90 A:95 D:95 |
| 4 | Certain | Default roster = the 7 repos (fab-kit, hop, idea, run-kit, shll, tu, wt); repo roots resolved via `hop <repo> where` (selection-first grammar) with `~/code/sahil87/<repo>` fallback | Backlog enumerates the list; `hop --help` confirms the `hop <selection> where` grammar | S:90 R:85 A:95 D:90 |
| 5 | Certain | Primitive mirrors fab-operator §6 spawn sequence minus enrollment; each spawn embeds exactly one slash command (no `&&` chains); no `»`/`›` markers, no operator state written | fab-operator §6 read directly — documents the one-command constraint and the enrollment steps the primitive omits; user stated the no-registration rule | S:90 R:90 A:95 D:90 |
| 6 | Confident | One dedicated tmux session per bulk task, default name `bulk-<task-slug>`; window name `<wt>-<repo>` | Conventions user-proposed; exact name/format patterns are my defaults, trivially changed and idempotent under the operator's `ensure-prefix` | S:60 R:90 A:70 D:55 |
| 7 | Certain | `bulk-shll-fab-upgrade` recipe = `fab upgrade-repo` in fresh worktree → `/fab-fff` → PR only if changed (the change-260718 op) | Backlog/intake name this as the exact ad-hoc op the preset codifies | S:80 R:85 A:80 D:80 |
| 8 | Confident | `bulk-shll-release` is a direct per-repo loop (no worktree/agent/PR); `just release` defaults to patch and is tag-driven with no tracked-file changes | Verified live: `justfile` `release bump="patch"` recipe + `scripts/release.sh` is tag-driven, requires a clean tree, writes no tracked files | S:70 R:85 A:95 D:80 |
| 9 | Confident | Release preset skips repos whose HEAD already carries the latest tag (exact-match `git describe`) and reports the skip | Analogue of the user-stated PR-skip rule; `release.sh` does not itself skip, so the doc must encode it; a zero-commit release is pointless | S:50 R:85 A:80 D:70 |
| 10 | Confident | Release preset requires an explicit repo→next-version confirmation before the first tag push | Tag push triggers CI releases across up to 7 repos (outward-facing); my default, consistent with intake assumption 13 | S:45 R:90 A:80 D:70 |

10 assumptions (5 certain, 5 confident, 0 tentative).
