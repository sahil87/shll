---
type: memory
description: "`release.yml` — cross-compile, publish a GitHub Release, and update the Homebrew tap on `v*` tags. Carries no shll.ai publish step — shll.ai pulls via its own scheduled `shll help-dump` job."
---
# ci/release-workflow

The GitHub Actions release pipeline for shll. Source: `.github/workflows/release.yml`.

Per Constitution VI, releases are cut by tagging `v*`; the workflow cross-compiles, publishes a GitHub Release, and updates the Homebrew tap.

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

## shll.ai help-tree integration

shll.ai's integration is pull-based: its own scheduled job (`scheduled-help-refresh.yml`, on shll.ai's side) `brew install`s shll, runs `shll help-dump`, and commits the captured JSON itself. The producer is the `help-dump` command (shipped with shll); the transport lives entirely in shll.ai. This workflow publishes nothing to `sahil87/shll.ai` and references no `SHLLAI_TOKEN`. (7huv)

The JSON contract `help-dump` produces is documented in [cli/help-dump-contract](/cli/help-dump-contract.md).

## Design Decisions

### No push transport — shll.ai pulls
**Decision**: The release workflow carries no shll.ai publish step; the `help-dump` producer stays in shll, and shll.ai's scheduled puller owns the transport.
**Why**: A push transport runs on every release for no consumer (shll.ai's puller runs `shll help-dump` itself — its change `oa63`), and fails loudly once the shll.ai-side auto-merge / `SHLLAI_TOKEN` prerequisites are revoked. Keeping the `help-dump` command intact preserves the producer while eliminating the dead cross-repo push.
**Rejected**: Removing the `help-dump` command along with the transport — shll.ai's puller still consumes it.
*Introduced by*: `260603-7huv-teardown-shllai-push`

## Constitution conformance

- **VI (Thin Justfile, Fab-Kit Build Pattern)** — releases cut by tagging `v*`; cross-platform build + GitHub Release + Homebrew-tap update via the workflow.
- **I (Security First)** — the remaining git shell-out (the Homebrew-tap push) lives in YAML, not Go; shll's `internal/proc` rule governs Go subprocess code only and does not apply here.

## Cross-references

- The frozen `help/<tool>.json` contract and producer rules: [cli/help-dump-contract](/cli/help-dump-contract.md).
- Version ldflags injection (`main.version`): [cli/commands](/cli/commands.md).
