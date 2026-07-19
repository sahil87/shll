---
description: "Run fab upgrade-repo across all shll toolkit repos — one fresh-worktree agent per repo, each driving the upgrade through /fab-fff and shipping a PR only if anything changed. Zero-argument preset over bulk-shll-op."
---

# /bulk-shll-fab-upgrade

Zero-argument preset over `/bulk-shll-op`. Runs the recurring fab upgrade across the full shll toolkit roster (`fab-kit, hop, idea, run-kit, shll, tu, wt`).

This is the exact op that was improvised ad hoc in change 260718 ("run `fab upgrade-repo` in a fresh worktree in every repo, then ship a PR if anything changed"), now a one-command invocation.

## Recipe

Invoke the `/bulk-shll-op` per-repo loop (see `bulk-shll-op.md`) with:

- **Target repos**: the default 7-repo roster.
- **Per-repo task**: in each repo's fresh worktree, run `fab upgrade-repo`, then drive the resulting change through the full pipeline with `/fab-fff` — shipping a PR **only if anything changed** (the PR-skip rule from `bulk-shll-op`: an agent whose upgrade produced no diff skips `/git-pr` entirely, so there are no empty PRs).

All the `bulk-shll-op` conventions apply unchanged: one dedicated tmux session (default `bulk-fab-upgrade`), `<wt>-<repo>` window names, and no operator registration (the operator monitors one-directionally).
