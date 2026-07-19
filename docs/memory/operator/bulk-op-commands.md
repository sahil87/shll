---
type: memory
description: "The `.claude/commands/` bulk-orchestration slash commands — `bulk-shll-op` primitive (fresh-worktree agent per roster repo via `fab agent`, one slash command each) plus `bulk-shll-fab-upgrade` (mechanical `fab upgrade-repo`; agent only for migrations) and `bulk-shll-release` (direct loop, no worktree/agent/PR): session/window conventions, ask-or-skip root resolution, no operator registration, and the `.gitignore` carve-out."
---
# operator/bulk-op-commands

**Domain**: operator

## Overview

Three repo-tooling slash-command docs under `.claude/commands/` that codify running one operation across the shll toolkit roster: a generic `bulk-shll-op` primitive plus two presets, `bulk-shll-fab-upgrade` and `bulk-shll-release`. These are operator-facing prompt tooling, not Go CLI surface — the first repo-tooling memory in a tree otherwise covering shll's Go binary (`cli`, `internal`, `ci`).

Files:

- `.claude/commands/bulk-shll-op.md` — the generic per-repo spawn primitive.
- `.claude/commands/bulk-shll-fab-upgrade.md` — zero-argument preset over the primitive.
- `.claude/commands/bulk-shll-release.md` — patch-release preset (a direct loop, *not* the spawn primitive).

The default roster is the 7 sahil87 toolkit repos: `fab-kit, hop, idea, run-kit, shll, tu, wt`. Each repo's main-worktree root is resolved with `hop <repo> where` (hop's selection-first grammar); when resolution fails (hop unavailable or the repo absent from `hop.yaml`), the agent asks the user for the location or skips that repo — there is no guessed fallback path. Wherever a bulk command needs to start an agent in a worktree, it does so via `fab agent` (a binary call that resolves and launches the repo's own configured session command) — the invoker never reads or composes another repo's session command.

## `bulk-shll-op` — the generic primitive

Runs a single per-repo operation as a batch. Inputs:

1. **Target repos** — default: the full 7-repo roster; an explicit subset may be passed.
2. **Per-repo task** — the recipe to run in each fresh worktree, expressed as the **single** prompt / slash command handed to the spawned agent (e.g. `/fab-new <description>` or `/fab-fff <change>`).
3. **PR-skip rule** — the dispatched prompt MUST instruct each agent to skip `/git-pr` when the recipe produced no diff. No empty PRs.

**Per-repo loop** — mirrors fab-operator §6's spawn sequence, minus the operator's enrollment steps. For each target repo:

1. Resolve the main-worktree root (`hop <repo> where`; on failure ask the user or skip the repo).
2. `wt create --non-interactive` **with the target repo as the working directory**, so the worktree lands under `$(dirname <repo-root>)/<repo>.worktrees/` rather than the invoker's repo. Capture the generated worktree name `<wt>` and absolute path.
3. Start the agent in the worktree: `tmux new-window -t <bulk-session> -n "<wt>-<repo>" -c <worktree-path> "fab agent"` — `fab agent` resolves and launches the repo's own configured session command from the worktree.
4. Dispatch the task once the session is up: `tmux send-keys -t "<bulk-session>:<wt>-<repo>" "<task>" Enter`, sending **exactly one** slash command per agent (no `&&`-joined strings — a spawned agent reads one leading `/command` per prompt, per fab-operator §6).

## `bulk-shll-fab-upgrade` — fab-upgrade preset

A zero-argument preset over `bulk-shll-op`'s roster and conventions whose core move is **mechanical, not agent-driven**: `fab upgrade-repo` is a binary command — no fab change is created and no pipeline runs. Per roster repo, in its `<wt>-<repo>` window of the `bulk-fab-upgrade` session: create a fresh worktree, run `fab upgrade-repo`, and branch on its console output. If "/fab-setup migrations" is **not** printed, commit/push/PR the diff directly (skipping entirely when there is no diff — the PR-skip rule). If it **is** printed, start an agent in that window via `fab agent`, run `/fab-setup migrations` in it, then ask the same agent to send the PR for the combined diff. The session and window conventions and the no-operator-registration rule apply unchanged. This codifies the exact op that was improvised ad hoc in change 260718.

## `bulk-shll-release` — patch-release preset

Cuts a patch release of every roster repo via each repo's `just release` recipe (default bump: patch). All 7 roster repos carry a `release` recipe; shll's reference implementation is **tag-driven** — `scripts/release.sh` computes the next semver from the latest `v*` tag, creates and pushes the tag, and CI takes over (cross-compile, GitHub Release, Homebrew-tap bump). No tracked files change; the working tree must be clean.

Because `just release` writes no tracked files and opens no PR, this preset is a **direct sequential per-repo loop** run by the invoking agent — not the `bulk-shll-op` spawn loop. No worktree, no spawned agent, no PR. Per roster repo, in order:

1. Resolve the main-worktree root; run the release from the repo root itself.
2. Verify releasable: working tree clean (`git status --porcelain` empty) and local `main` not behind `origin/main` (via `git fetch origin`). On any failure, record an **error** for the repo and move on.
3. **Skip repos with no new commits since the last tag** — if `git describe --tags --exact-match HEAD` succeeds (HEAD already carries the latest tag), record a **skip**. This is the release-only-if-changed analogue of the primitive's PR-skip rule; a zero-commit release is pointless.
4. Run `just release` (patch) in the repo root; record the **new tag**.

The run ends with a per-repo outcome table: each repo is **new tag** (`v<x.y.z>`), **skip** (no new commits since last tag), or **error** (dirty / stale / failed).

### Confirmation gate

Before the **first** tag push, the agent computes each releasable repo's next version, presents the **repo → next-version** list (noting skips/errors), and obtains **explicit user confirmation**. A tag push publishes a release across up to 7 repos and fires CI on the tag — outward-facing and not reversible in the "delete it quietly" sense — so the gate is mandatory.

## No operator registration

The commands perform **none** of the operator's enrollment actions: they write no operator state file, add no `branch_map` entries, and never rename windows with the operator's `»` / `›` markers. Those are the operator's own enrollment actions. Monitoring is the fab-operator's job and is **one-directional** — the operator sweeps every session on the server with `fab pane map --all-sessions`; spawned agents do not register back. The session-per-task and `<wt>-<repo>` window conventions exist purely to make the spawned agents discoverable and groupable by the operator.

## Conventions

- **One tmux session per bulk task.** Create a dedicated session (default name `bulk-<task-slug>`) and open every per-repo window in it, so the group stays visible to the operator via `fab pane map --all-sessions`.
- **Repo-name window suffix.** Each window is named `<wt>-<repo>` (e.g. `swift-fox-hop`), self-identifying at a glance.

## `.gitignore` carve-out

The command docs live under `.claude/commands/`, which must be tracked, while the rest of `.claude/` (notably `.claude/skills/` and `.claude/settings.local.json`) must stay ignored. The `.gitignore` achieves this with exactly two lines:

```gitignore
/.claude/*
!/.claude/commands/
```

`/.claude/*` matches the **direct children** of `.claude/` (so `skills/` and `settings.local.json` stay ignored) while still letting git descend into the directory; `!/.claude/commands/` re-includes the `commands/` subtree. No `!/.claude/commands/**` third line and no explicit `settings.local.json` re-ignore are needed — `/.claude/*` matches direct children only, and nothing else ignores paths under the negated `commands/` dir. A bare `/.claude` line does **not** work: it excludes the directory itself, so git never descends and any negation beneath it is dead.

This is what keeps `commands/` a hand-authored, tracked home while `skills/` remains fab-sync-regenerated (see Design Decisions).

## Design Decisions

### Command docs, not skills or a Go subcommand
**Decision**: Author the bulk-op recipes as slash-command docs under `.claude/commands/`.
**Why**: `.claude/skills/` is fully regenerated by fab-kit's embedded templates on every `fab sync` / `wt create`, so anything placed there is silently overwritten; `.claude/commands/` is untouched by fab sync. This also forces the conventions to live in the command docs rather than the fab-kit-owned fab-operator skill file, which this repo cannot durably edit.
**Rejected**: A `shll` Go subcommand — Constitution VII (Minimal Surface Area) would rightly bounce a new top-level command for operator prompt tooling, and no Go code is involved.
*Introduced by*: 260719-hh4t-bulk-op-operator-commands

### `bulk-shll-release` is a direct loop, not a spawn loop
**Decision**: Run the release preset as a direct sequential per-repo loop in the invoking agent — no worktree, no spawned agent, no PR.
**Why**: `just release` (`scripts/release.sh`) is tag-driven and modifies no tracked files, so there is nothing to PR and a per-repo worktree adds nothing; a confirmation gate before the first push covers the outward-facing risk.
**Rejected**: Reusing the `bulk-shll-op` spawn loop — it would spin up worktrees and agents for an op that produces no diff and no PR, pure overhead.
*Introduced by*: 260719-hh4t-bulk-op-operator-commands
