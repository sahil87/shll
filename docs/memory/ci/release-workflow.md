# ci/release-workflow

The GitHub Actions release pipeline for shll. Source: `.github/workflows/release.yml`.

Per Constitution VI, releases are cut by tagging `v*`; the workflow cross-compiles, publishes a GitHub Release, updates the Homebrew tap, and (since change ep4z) publishes a machine-readable CLI help tree to `sahil87/shll.ai`.

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
7. **help-dump publish steps (change ep4z)** — see below. Placed after the Release exists, before the tap update.
8. **Update Homebrew tap** — clones `sahil87/homebrew-tap` with `HOMEBREW_TAP_TOKEN`, renders `Formula/shll.rb` from a template, commits and pushes directly (single-repo, no race).

All third-party actions are pinned to commit SHAs.

## help-dump → shll.ai publishing (change ep4z)

Three steps, decoupled from release-artifact packaging. The JSON contract these produce is documented in [cli/help-dump-contract](../cli/help-dump-contract.md).

### 1. Build native binary for help-dump

A dedicated native `linux/amd64` build (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`, ldflags `-X main.version=<tag>`) to `/tmp/shll-native`, **independent of the cross-compile matrix**. It exists solely to run `help-dump` natively on the `ubuntu-latest` runner, keeping the dump step separate from the packaged release artifacts.

### 2. Generate help/shll.json

Runs `/tmp/shll-native help-dump > help/shll.json`, then validates: `jq empty help/shll.json` (fails the job if it doesn't parse) and `test "$(jq -r .version help/shll.json)" = "<tag>"` (the embedded version must equal the release tag). Because the trigger is release-tag-only, the embedded `version` is always a clean release tag — never a `git describe` dev string.

### 3. Publish to shll.ai

Authed by `SHLLAI_TOKEN` (a cross-repo PAT/app token carrying `contents:write` + `pull-requests:write` for shll.ai), exported as both `SHLLAI_TOKEN` (for the `git clone` URL) and `GH_TOKEN` (so the `gh` CLI targets shll.ai). Flow:

1. Clone `sahil87/shll.ai`, copy `help/shll.json` onto a per-release branch `shll-help-<tag>`.
2. **No-op guard**: `git diff --cached --quiet` → if byte-identical to shll.ai's `main`, print a skip message and `exit 0` (no empty PR). Pairs with the date-granular `captured_at` so same-day re-runs are true no-ops.
3. Otherwise commit and **force-push** the branch (`git push -f`): the branch is per-tag, so a release re-run would fail a plain push on an existing branch — force-push makes it track this run's commit and refreshes any open PR.
4. **Reuse-or-create PR**: `gh pr list --head <branch> --state open` first; create via `gh pr create` only if none exists (re-run resilience).
5. `gh pr merge --repo sahil87/shll.ai <pr> --auto --squash` — drive it to merge, never leave it dangling.

> **Design Decision: PR with auto-merge, not direct push (change ep4z).**
> *Why*: shll.ai receives concurrent pushes from up to 7 tool repos during a coordinated rollout. A direct `git push` to `main` would race and reject. An auto-merge PR serializes merges through GitHub's merge-queue semantics and avoids the multi-repo push race. (Contrast the Homebrew-tap step, which pushes directly — it is single-repo with no concurrent writers.)
> *Prerequisites (shll.ai-side, outside this repo's control)*: auto-merge must be enabled in shll.ai repo settings, and `SHLLAI_TOKEN` must carry `pull-requests:write`. If auto-merge is disabled, `gh pr merge --auto` errors — the step is meant to fail loudly rather than silently leave a dangling PR.

> **Design Decision: release-tag-only trigger, dedicated native dump build (change ep4z).**
> *Why release-only (vs. a per-`main`-push workflow)*: keeps `help/shll.json`'s `version` a clean release tag and yields exactly one shll.ai PR per shll release. *Why a dedicated native build (vs. reusing a cross-compiled artifact)*: decouples the dump from release-artifact packaging and runs natively on the runner without emulation.

## Constitution conformance

- **VI (Thin Justfile, Fab-Kit Build Pattern)** — releases cut by tagging `v*`; cross-platform build + GitHub Release + Homebrew-tap update via the workflow.
- **I (Security First)** — the git/gh shell-out lives in YAML, not Go; shll's `internal/proc` rule governs Go subprocess code only and does not apply here.

## Cross-references

- The frozen `help/<tool>.json` contract and producer rules: [cli/help-dump-contract](../cli/help-dump-contract.md).
- Version ldflags injection (`main.version`): [cli/commands](../cli/commands.md).
