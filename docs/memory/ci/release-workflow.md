---
type: memory
description: "`release.yml` — cross-compile, publish a GitHub Release, and update the Homebrew tap. No longer pushes to shll.ai (help-push transport torn down in change 7huv; shll.ai now pulls via `shll help-dump`)."
---
# ci/release-workflow

The GitHub Actions release pipeline for shll. Source: `.github/workflows/release.yml`.

Per Constitution VI, releases are cut by tagging `v*`; the workflow cross-compiles, publishes a GitHub Release, and updates the Homebrew tap.

> **shll.ai integration is now pull-based (change 7huv).** This workflow no longer pushes anything to `sahil87/shll.ai`. shll.ai's own scheduled job (`scheduled-help-refresh.yml`, on shll.ai's side) `brew install`s shll, runs `shll help-dump`, and commits the captured JSON itself. The former help-push transport — a dedicated native build, `help/shll.json` generation, and an auto-merged cross-repo PR authed by `SHLLAI_TOKEN` (added in change ep4z) — was torn down in change 7huv. `SHLLAI_TOKEN` is no longer referenced anywhere in this workflow.

## Triggers

- `push: tags: v*` — the canonical release path (tag-driven). `version`/`tag` come from `${GITHUB_REF#refs/tags/}`.
- `workflow_dispatch` with a `bump` choice (`patch`/`minor`/`major`) — runs `scripts/release.sh <bump>` to create the tag from `main`, then proceeds. Dispatch must run from `main` (`if: github.event_name == 'push' || github.ref == 'refs/heads/main'`).

`concurrency: group: release, cancel-in-progress: false` serializes releases. Workflow-level `permissions: contents: write` covers the GitHub Release on `sahil87/shll`.

## Step order (the `release` job)

1. **Checkout** (`fetch-depth: 0`; dispatch checks out `main`).
2. **Create tag (manual dispatch)** — only on `workflow_dispatch`.
3. **setup-go** (from `src/go.mod`).
4. **Extract version from tag** (`steps.version`) — emits `tag` (e.g. `v0.5.0`) and `version` (`0.5.0`).
5. **Cross-compile** — `darwin/{arm64,amd64}` + `linux/{arm64,amd64}`, `CGO_ENABLED=0`, ldflags `-X main.version=<tag>`, tarred into `dist/`.
6. **Determine release notes base tag** + **Create GitHub Release** (`softprops/action-gh-release`, attaches `dist/*.tar.gz`).
7. **Update Homebrew tap** — clones `sahil87/homebrew-tap` with `HOMEBREW_TAP_TOKEN`, renders `Formula/shll.rb` from a template, commits and pushes directly (single-repo, no race).

All third-party actions are pinned to commit SHAs.

## shll.ai help-tree integration (now pull-based — teardown change 7huv)

This workflow no longer publishes anything to `sahil87/shll.ai`. Change ep4z originally added a help-push transport here (a dedicated native `help-dump` build, `help/shll.json` generation + validation, and an auto-merged cross-repo PR authed by `SHLLAI_TOKEN`); change 7huv removed all three steps once shll.ai inverted the integration to **pull**.

Today shll.ai's own scheduled job (`scheduled-help-refresh.yml`, on shll.ai's side) `brew install`s shll, runs `shll help-dump`, and commits the captured JSON itself — so the producer remains the `help-dump` command (still shipped), but the transport lives entirely in shll.ai. The JSON contract `help-dump` produces is documented in [cli/help-dump-contract](/cli/help-dump-contract.md).

> **Design Decision: retire the push, let shll.ai pull (change 7huv).**
> *Why*: shll.ai migrated to a scheduled puller (its change `oa63`) that runs `shll help-dump` itself, so the push transport ran on every release for no consumer — wasted work at best, and a loudly-failing release step once shll.ai's auto-merge / `SHLLAI_TOKEN` prerequisites were revoked. Removing only the transport (not the `help-dump` command) keeps the producer intact while eliminating the dead cross-repo push. `SHLLAI_TOKEN` is gone from the workflow; its repo-secret deletion is a post-merge manual settings action flagged in the change's PR.

## Constitution conformance

- **VI (Thin Justfile, Fab-Kit Build Pattern)** — releases cut by tagging `v*`; cross-platform build + GitHub Release + Homebrew-tap update via the workflow.
- **I (Security First)** — the remaining git shell-out (the Homebrew-tap push) lives in YAML, not Go; shll's `internal/proc` rule governs Go subprocess code only and does not apply here.

## Cross-references

- The frozen `help/<tool>.json` contract and producer rules: [cli/help-dump-contract](/cli/help-dump-contract.md).
- Version ldflags injection (`main.version`): [cli/commands](/cli/commands.md).
