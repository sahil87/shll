---
description: "Cut a patch release of every shll toolkit repo via `just release` — a direct sequential per-repo loop (no worktrees, no agents, no PRs) that skips unchanged repos and confirms the repo→version list before the first tag push."
---

# /bulk-shll-release

Release a patch version of every shll toolkit repo (`fab-kit, hop, idea, run-kit, shll, tu, wt`) via each repo's `just release` recipe (default bump: **patch**).

All 7 roster repos carry a `release` justfile recipe. shll's reference implementation is **tag-driven**: `scripts/release.sh` computes the next semver from the latest `v*` tag, creates and pushes the tag, and CI takes over (cross-compile, GitHub Release, Homebrew tap bump). No tracked files change, and the working tree must be clean.

> **Not the spawn loop.** Because `just release` writes no tracked files and opens no PR, this preset does **not** use the `bulk-shll-op` spawn loop — no worktree, no spawned agent, no PR. It is a **direct sequential per-repo loop** run by the invoking agent.

## Per-repo loop

For each roster repo, in order:

1. **Resolve the main-worktree root** — `hop <repo> where` (fallback `~/code/sahil87/<repo>`). Run the release from the repo root itself, not a worktree.
2. **Verify the repo is releasable**:
   - Working tree is clean (`git status --porcelain` empty).
   - `main` is up to date: `git fetch origin`, then confirm local `main` is not behind `origin/main`.
   - On any failure, record an **error** for this repo and move on — do not release from a dirty or stale checkout.
3. **Skip repos with no new commits since the last tag** — if `git describe --tags --exact-match HEAD` succeeds (HEAD already carries the latest tag), there is nothing to release: record a **skip** and move on. This is the release-only-if-changed analogue of the PR-skip rule; a zero-commit release is pointless.
4. **Release** — run `just release` (patch) in the repo root. Record the **new tag**.

## Confirmation gate

Pushing a tag publishes a release across up to 7 repos, and CI fires on the tag — this is outward-facing and not reversible in the "delete it quietly" sense. Therefore, **before the first tag push**:

1. Compute each releasable repo's next version (and note which repos will be skipped / errored).
2. Present the **repo → next-version** list to the user.
3. Obtain **explicit confirmation** before pushing any tag.

## Summary

Report a per-repo outcome table: for each repo, one of **new tag** (`v<x.y.z>`), **skip** (no new commits since last tag), or **error** (dirty/stale/failed), so the whole release wave is auditable at a glance.
