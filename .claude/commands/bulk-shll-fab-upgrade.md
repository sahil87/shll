---
description: "Run fab upgrade-repo across all shll toolkit repos — a mechanical per-repo move in a fresh worktree; an agent is started only when the upgrade prints that /fab-setup migrations is required. Ships a PR per repo only if anything changed. Zero-argument preset over bulk-shll-op's roster and conventions."
---

# /bulk-shll-fab-upgrade

Zero-argument preset over `/bulk-shll-op`'s roster and conventions. Runs the recurring fab upgrade across the full shll toolkit roster (`fab-kit, hop, idea, run-kit, shll, tu, wt`).

Unlike the generic `bulk-shll-op` loop, the core move here is **mechanical, not agent-driven**: `fab upgrade-repo` is a binary command, not a prompt handed to a spawned agent. No fab change is created and no pipeline (`/fab-fff`) runs. An agent enters the picture **only** when the upgrade reports that migrations are required.

This codifies the op improvised ad hoc in change 260718.

## Per-repo recipe

For each roster repo, in its own `<wt>-<repo>` window of the dedicated bulk session (default `bulk-fab-upgrade`):

1. **Resolve the main-worktree root** — `hop <repo> where`; on failure, ask the user for the location or skip the repo.
2. **Create a fresh worktree** — `wt create --non-interactive` with the target repo as the working directory. Capture `<wt>` and the worktree path.
3. **Run the binary command** `fab upgrade-repo` in the worktree and capture its console output.
4. **Branch on the output**:
   - **"/fab-setup migrations" NOT printed** — the diff is complete as-is. If the upgrade produced a diff, commit, push, and open a PR directly (no agent needed). If there is no diff, skip the PR entirely — no empty PRs (the `bulk-shll-op` PR-skip rule).
   - **"/fab-setup migrations" printed** — start an agent in that window using `fab agent` (also a binary command — it reads the repo's configured session command and launches the session) and run `/fab-setup migrations` in it. Once the migrations are done, ask the **same agent** to send the PR for the combined diff.

## Conventions (inherited from `bulk-shll-op`)

- One dedicated tmux session for the whole run (default `bulk-fab-upgrade`).
- `<wt>-<repo>` window names, one window per repo.
- No operator registration — the operator monitors one-directionally via `fab pane map --all-sessions`.
