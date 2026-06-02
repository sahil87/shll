# Intake: Help-Dump CLI Tree → shll.ai Command Reference

**Change**: 260602-ep4z-help-dump-cli-tree
**Created**: 2026-06-02
**Status**: Draft

## Origin

Initiated via `/fab-new` from backlog item `[ep4z]` (2026-06-02), one-shot with a fully
specified, **frozen** contract. Raw input:

> Add a build-time 'help-dump' step that emits shll's CLI help tree as `help/shll.json` and
> PRs it into `sahil87/shll.ai` (the shll.ai landing site renders it as an expandable
> 'Command reference' on the shll tool page). CONTRACT (frozen — copy the reference sample at
> `sahil87/shll.ai` path `help/wt.json`): JSON shape is `{tool, version, captured_at (ISO-8601 UTC),
> schema_version: 1, root: Node}` where `Node = {name, path (full invocation e.g. 'shll install'),
> short (one-line desc), usage, text (the RAW -h output byte-for-byte, newlines preserved),
> commands: Node[] (recursive; empty array = leaf)}`. PRODUCER (shll is Cobra/Go, binary 'shll',
> main at `src/cmd/shll`): walk the cobra command tree programmatically (`rootCmd.Commands()`
> recursively), NOT regex-parsing -h text; per node capture `cmd.Name` / `cmd.CommandPath()` /
> `cmd.Short` / `cmd.UseLine()` and `cmd.UsageString()` (or Long+UsageString) as 'text'. FILTER OUT
> cobra's auto-generated 'completion' and 'help' subcommands and any `cmd.Hidden==true`. VERSION:
> read from the built binary (`rootCmd.Version` / ldflags) — do NOT hardcode. PUSH: in CI after
> build, run the dump, write `help/shll.json`, validate it parses, then open a PR into
> `sahil87/shll.ai` using the existing repo secret `SHLLAI_TOKEN` (contents + pull-request write)
> with auto-merge enabled (PR, not direct push to main, to avoid the multi-repo push race). This is
> shll's slice of a 7-tool rollout; the shll.ai site-side consumer (Astro loader + reference UI) is
> tracked separately in the shll.ai repo.

**Decisions reached during this `/fab-new`** (two SRAD-surfaced questions, both answered by the user):

1. **CI trigger** → *release tag push only*. The dump + shll.ai PR is wired into the existing
   `.github/workflows/release.yml` (which already fires on `push: tags: v*`), not a separate
   per-`main`-push workflow. Consequence: `help/shll.json` always tracks **released** versions and
   the embedded `version` is always a clean release tag (e.g. `v0.5.0`), never a `git describe`
   dev string. One PR into shll.ai per shll release.
2. **Dump build in CI** → *dedicated native `linux/amd64` runner build*. CI builds one extra native
   binary (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`, stamped with the same
   `-ldflags "-X main.version=<tag>"`) **solely** to run `shll help-dump`, independent of the
   existing cross-compile matrix. This keeps the dump step decoupled from release-artifact packaging
   and runs natively on the `ubuntu-latest` runner.

The reference sample `help/wt.json` was fetched from `sahil87/shll.ai` and inspected byte-for-byte;
the shll producer mirrors its exact shape (see **What Changes → JSON contract** below).

## Why

1. **Problem.** The shll.ai landing site wants to render an always-current, structured "Command
   reference" for each sahil87 tool. Today there is no machine-readable export of shll's CLI surface;
   the only source is `shll --help` text, which a website would have to scrape and re-parse. Scraping
   `-h` output is brittle (cobra formatting changes break it) and lossy (loses the command tree
   structure). shll is one of 7 tools in a coordinated rollout — each tool publishes its own
   `help/<tool>.json` to shll.ai against a single frozen schema.

2. **Consequence of not doing this.** The shll tool page on shll.ai either ships no command reference,
   or ships a hand-maintained one that silently drifts from the real CLI on every subcommand/flag
   change. Manual sync across 7 tools does not scale and will rot.

3. **Why this approach.**
   - **Programmatic tree walk, not regex.** Walking `rootCmd.Commands()` recursively reads the
     command tree from cobra's own data model — the same source of truth that renders `-h`. It cannot
     drift from the real CLI and survives cobra formatting changes. Regex-parsing `-h` text would be
     fragile and is explicitly rejected by the contract.
   - **Hidden subcommand producer.** A `shll help-dump` cobra subcommand (marked `Hidden: true`) is the
     natural place to walk the tree: it has direct access to the live `rootCmd` and to
     `rootCmd.Version` (already ldflags-stamped at build), so VERSION is read from the built binary for
     free — no hardcoding, no second source of truth. Hidden keeps it off the user-facing help surface
     and lets the dump filter itself out via the existing `cmd.Hidden` rule.
   - **PR with auto-merge, not direct push.** shll.ai receives concurrent pushes from up to 7 tool
     repos during a coordinated rollout. Direct `git push` to `main` would race and reject. Opening a
     PR (via `SHLLAI_TOKEN`) with auto-merge enabled serializes the merges through GitHub's merge queue
     semantics and avoids the multi-repo push race the contract calls out.

## What Changes

### 1. New hidden `shll help-dump` subcommand (`src/cmd/shll/help_dump.go`, new)

A new cobra subcommand registered in `newRootCmd()` (`src/cmd/shll/root.go`) that walks the live
command tree and emits the JSON contract to stdout.

- **Registration** (in `root.go`, added to the existing `cmd.AddCommand(...)` block):
  ```go
  cmd.AddCommand(
      newInstallCmd(),
      newUpdateCmd(),
      newShellInitCmd(),
      newShellInstallCmd(),
      newVersionCmd(),
      newHelpDumpCmd(), // new
  )
  ```
- **Command definition** — `Hidden: true`, `Args: cobra.NoArgs`, writes to `cmd.OutOrStdout()`:
  ```go
  func newHelpDumpCmd() *cobra.Command {
      return &cobra.Command{
          Use:    "help-dump",
          Short:  "emit the shll CLI help tree as JSON (build tooling)",
          Hidden: true,
          Args:   cobra.NoArgs,
          RunE: func(cmd *cobra.Command, _ []string) error {
              return runHelpDump(cmd.Root(), cmd.OutOrStdout())
          },
      }
  }
  ```
  Using `cmd.Root()` (not a captured `rootCmd`) keeps the walk anchored to the actual root regardless
  of how the command was assembled in tests.

### 2. The tree walk and JSON contract (`src/cmd/shll/help_dump.go`)

**JSON shape (frozen — mirrors `help/wt.json` exactly):**

```json
{
  "tool": "shll",
  "version": "v0.5.0",
  "captured_at": "2026-06-02T00:00:00Z",
  "schema_version": 1,
  "root": { "...Node..." }
}
```

Where a **Node** is:

```json
{
  "name": "install",
  "path": "shll install",
  "short": "brew install every sahil87 tool that isn't already installed",
  "usage": "shll install [flags]",
  "text": "<RAW -h output, byte-for-byte, newlines preserved>",
  "commands": []
}
```

**Go structs** (field order and JSON tags matched to the reference; encode with
`json.MarshalIndent(doc, "", "  ")` to match the reference's 2-space indentation):

```go
type helpDoc struct {
    Tool          string   `json:"tool"`
    Version       string   `json:"version"`
    CapturedAt    string   `json:"captured_at"`
    SchemaVersion int      `json:"schema_version"`
    Root          helpNode `json:"root"`
}

type helpNode struct {
    Name     string     `json:"name"`
    Path     string     `json:"path"`
    Short    string     `json:"short"`
    Usage    string     `json:"usage"`
    Text     string     `json:"text"`
    Commands []helpNode `json:"commands"` // empty array, never null — see below
}
```

**Per-node field mapping** (programmatic, from cobra's data model — NOT regex):

| Field   | Source                                                                                 |
|---------|----------------------------------------------------------------------------------------|
| `name`  | `cmd.Name()`                                                                           |
| `path`  | `cmd.CommandPath()` (e.g. `"shll"`, `"shll install"`)                                   |
| `short` | `cmd.Short`                                                                            |
| `usage` | `cmd.UseLine()` (e.g. `"shll install [flags]"`, `"shll shell-init [shell] [flags]"`)    |
| `text`  | RAW `-h` output — see **text construction** below                                       |
| `commands` | recursive Node per visible child (see **filtering** below)                           |

**`text` construction (the RAW `-h` output, byte-for-byte).** Cobra's `-h` renders the default help
template, which is the **Long** description (falling back to **Short** when `Long == ""`) followed by
a blank line and `UsageString()` (the `Usage:` / `Available Commands:` / `Flags:` block). The
reference `help/wt.json` confirms this: the root `text` opens with wt's Long blurb, then `Usage:`,
`Available Commands:`, `Flags:`; leaf `text` opens with the command's Long/Short then `Usage:` /
`Flags:`. Construct it to match cobra's help template output exactly:

```go
func nodeText(cmd *cobra.Command) string {
    long := cmd.Long
    if long == "" {
        long = cmd.Short
    }
    usage := cmd.UsageString()
    if long == "" {
        return usage
    }
    return long + "\n\n" + usage
}
```

> Edge note to validate against the reference during apply: `wt update` (a leaf with `Long == Short`)
> renders its short line once at the top of `text`, then `Usage:` / `Flags:`. The construction above
> reproduces that. The acceptance criterion is byte-for-byte equality with `shll <cmd> -h`, captured
> by a test that compares `nodeText(cmd)` against the command's actual help output — so any residual
> template nuance (trailing newline handling, etc.) is caught by the test rather than guessed here.

**Filtering (applied to every node's children, recursively):** skip a child command when ANY of:
- `child.Name() == "completion"` (cobra auto-generated)
- `child.Name() == "help"` (cobra auto-generated)
- `child.Hidden == true` (this also excludes `help-dump` itself, since it is `Hidden: true`)
- `!child.IsAvailableCommand()` — defensive; covers deprecated/unavailable commands. (Confirm this
  doesn't over-filter during apply; the three explicit rules above are the contract minimum.)

**`commands` must serialize as `[]`, never `null`.** The reference uses `"commands": []` for leaves.
Initialize the slice non-nil (`children := []helpNode{}`) before appending, so `encoding/json` emits
`[]` rather than `null` for leaf nodes.

**Recursion:** iterate `cmd.Commands()` (cobra returns children); for each child that passes the
filter, recurse to build its Node, then append. Order: preserve cobra's command order (the reference
`wt.json` lists children alphabetically, which is cobra's default `Commands()` sort) — do not
re-sort beyond what cobra already returns.

**Top-level fields:**
- `tool`: literal `"shll"`.
- `version`: `root.Version` (i.e. `rootCmd.Version`, which `main.go` sets from the ldflags-stamped
  `var version`). Read from the binary — never hardcoded. In CI this is a clean release tag
  (`v0.5.0`); for a local unstamped build it is `dev`.
- `schema_version`: literal `1` (int).
- `captured_at`: ISO-8601 UTC at **date granularity** — `"2026-06-02T00:00:00Z"`, matching the
  reference `wt.json` (clarified 2026-06-03). Produce via date-truncation, e.g.
  `time.Now().UTC().Truncate(24*time.Hour).Format(time.RFC3339)` or formatting `time.Now().UTC()` with
  a date-only layout then appending `T00:00:00Z`. This makes same-day re-runs byte-identical so the
  CI no-op guard can suppress redundant PRs. The unit test asserts on the `…T00:00:00Z` shape (regex),
  not a fixed date.

**Output:** `json.MarshalIndent` with 2-space indent + a trailing newline, written to stdout. The
command writes ONLY the JSON to stdout (no log lines), so CI can redirect `> help/shll.json` cleanly.
This honors the project's per-tool output-separation convention (diagnostics → stderr, payload →
stdout).

### 3. CI integration in `.github/workflows/release.yml` (modify)

After the existing **Cross-compile** step (and after a GitHub Release exists), add steps to the
`release` job. Decision: **release-tag-push only** (no separate main-push workflow) and a **dedicated
native build** for the dump.

- **Build native dump binary** (new step, native `linux/amd64`, stamped with the release tag):
  ```yaml
  - name: Build native binary for help-dump
    working-directory: src
    run: |
      CGO_ENABLED=0 GOOS=linux GOARCH=amd64 \
        go build -ldflags "-X main.version=${{ steps.version.outputs.tag }}" \
        -o /tmp/shll-native ./cmd/shll
  ```
- **Generate + validate the dump** (new step):
  ```yaml
  - name: Generate help/shll.json
    run: |
      mkdir -p help
      /tmp/shll-native help-dump > help/shll.json
      jq empty help/shll.json   # fail the job if it does not parse
      # sanity: confirm the version embedded matches the release tag
      test "$(jq -r .version help/shll.json)" = "${{ steps.version.outputs.tag }}"
  ```
- **Open the shll.ai PR with auto-merge** (new step) — clone shll.ai with `SHLLAI_TOKEN`, copy
  `help/shll.json` to `help/shll.json` in that repo on a per-release branch, commit, push the branch,
  open a PR, enable auto-merge:
  ```yaml
  - name: Publish to shll.ai
    env:
      SHLLAI_TOKEN: ${{ secrets.SHLLAI_TOKEN }}
    run: |
      tag="${{ steps.version.outputs.tag }}"
      branch="shll-help-${tag}"
      git clone "https://x-access-token:${SHLLAI_TOKEN}@github.com/sahil87/shll.ai.git" /tmp/shll.ai
      cp help/shll.json /tmp/shll.ai/help/shll.json
      cd /tmp/shll.ai
      git config user.name  "github-actions[bot]"
      git config user.email "github-actions[bot]@users.noreply.github.com"
      git checkout -b "$branch"
      git add help/shll.json
      # No-op guard: skip cleanly if the reference is byte-identical to main
      if git diff --cached --quiet; then
        echo "help/shll.json unchanged — skipping PR"
        exit 0
      fi
      git commit -m "shll: update help/shll.json for ${tag}"
      git push origin "$branch"
      # When there IS a change: open the PR AND drive it to merge (auto-merge),
      # never leave it dangling (clarified 2026-06-03).
      pr_url=$(gh pr create --repo sahil87/shll.ai --base main --head "$branch" \
        --title "shll: command reference for ${tag}" \
        --body "Automated CLI help-tree export for shll ${tag}. Generated by shll's release workflow (help-dump).")
      gh pr merge "$pr_url" --auto --squash
    # GH_TOKEN for the gh CLI must be SHLLAI_TOKEN so PR create/merge target shll.ai
  ```
  > Apply-stage detail: `gh` in the step needs `GH_TOKEN=${SHLLAI_TOKEN}` (or `gh auth`) pointed at
  > shll.ai. Confirm `SHLLAI_TOKEN` carries `contents:write` + `pull-requests:write` for shll.ai and
  > that shll.ai has auto-merge enabled in repo settings (a prerequisite for `gh pr merge --auto`).
  > If auto-merge is disabled repo-side, `--auto` errors — handle by surfacing a clear failure (the
  > merge is a shll.ai-side setting, not something this repo controls).

- **`permissions:` block.** The existing workflow declares `permissions: contents: write` (for the
  GitHub Release on `sahil87/shll`). The shll.ai writes are authenticated entirely via `SHLLAI_TOKEN`
  (a cross-repo PAT/app token), so the workflow-level `GITHUB_TOKEN` permissions do not need to change
  for the cross-repo PR. Confirm during apply.

### 4. Tests (`src/cmd/shll/help_dump_test.go`, new)

Unit tests driving `runHelpDump` against a synthetic root command and against the real `newRootCmd()`:

- **Contract shape:** assemble a small cobra tree (root + 2 children, one hidden, one `completion`),
  run the dump, unmarshal, assert: top-level keys present, `schema_version == 1`, `tool == "shll"`,
  `commands` is `[]` (not null) for leaves, hidden/`completion`/`help` children excluded.
- **`text` byte-for-byte:** for each visible command in the real tree, assert `node.text` equals the
  command's actual help output (capture `cmd.Help()` / `-h` into a buffer and compare) — this is the
  enforceable form of "RAW -h output, byte-for-byte."
- **Self-exclusion:** assert `help-dump` does not appear in the dumped tree (it is `Hidden`).
- **Version passthrough:** set `root.Version = "v9.9.9"`, assert `doc.version == "v9.9.9"` (no
  hardcoding).
- **`captured_at` shape:** assert it matches an RFC3339 UTC regex (not a fixed value), since it is
  time-dependent.
- **Determinism of structure:** two successive dumps differ only in `captured_at` (everything else
  byte-identical).

### 5. Docs / README (modify, light)

Add a short note that shll publishes a machine-readable help tree to shll.ai on release. The
`help-dump` subcommand is hidden/internal, so it is documented as build tooling, not a user command.
(Scope-light — exact wording decided at apply; not a user-facing feature surface.)

## Affected Memory

- `cli/help-dump-contract`: (new) The frozen `help/<tool>.json` schema (`tool`, `version`,
  `captured_at`, `schema_version: 1`, recursive `root: Node`) and the producer rules (programmatic
  cobra-tree walk; filter `completion`/`help`/`Hidden`; version from `rootCmd.Version`). This is a
  shared 7-tool contract worth recording so future shll changes don't drift from it.
- `ci/release-workflow`: (modify) `release.yml` now, in addition to cross-compiling + GitHub Release +
  Homebrew-tap update, builds a native binary, runs `help-dump`, and opens an auto-merge PR into
  `sahil87/shll.ai` via `SHLLAI_TOKEN`. Note the release-only trigger and the multi-repo-push-race
  rationale for PR-not-push.

> Memory paths above are best-guess domains; the actual `docs/memory/` layout is read and reconciled
> at hydrate. If these domains/files don't exist yet they are created then.

## Impact

- **New code:** `src/cmd/shll/help_dump.go`, `src/cmd/shll/help_dump_test.go`.
- **Modified code:** `src/cmd/shll/root.go` (register `newHelpDumpCmd()`).
- **Modified CI:** `.github/workflows/release.yml` (3 new steps in the `release` job).
- **Dependencies:** standard library only (`encoding/json`, `time`) + existing `github.com/spf13/cobra`.
  No new Go modules.
- **External systems:** `sahil87/shll.ai` repo (receives an automated PR on each release);
  `SHLLAI_TOKEN` repo secret (already present on `sahil87/shll`, created 2026-06-02).
- **Constitution check:** No conflict.
  - VII (Minimal Surface Area): `help-dump` is a `Hidden` build-tooling subcommand, not a user-facing
    addition to the `update`/`shell-init`/`version`/`install` surface — it does not raise the
    user-facing surface bar.
  - II (No State): the dump is re-derived from the live command tree each invocation; no caching.
  - I (Security First): no subprocess execution in the producer (pure in-process tree walk); the CI
    shell-out to `git`/`gh` lives in the workflow, not in shll's Go code, so the `internal/proc`
    rule does not apply to the producer.
- **Risk / edge cases:**
  - **`text` byte-for-byte fidelity** is the highest-risk item — cobra's help template details
    (trailing newlines, flag column wrapping) must match. Mitigated by a test that compares against
    real `-h` output rather than a hand-written golden string.
  - **Cross-repo auth:** `gh pr merge --auto` requires shll.ai to have auto-merge enabled in settings
    and `SHLLAI_TOKEN` to carry `pull-requests:write` — both are shll.ai-side/secret-side prerequisites
    outside this repo's control; the step must fail loudly if either is missing.
  - **PR noise:** one PR per release; the no-op guard (`git diff --cached --quiet`) avoids empty PRs
    when the CLI surface didn't change between releases.

## Open Questions

_All intake-level questions resolved during clarification (2026-06-03). See `## Clarifications`._

- ~~`captured_at` determinism vs. PR noise.~~ **Resolved:** date-granularity (`T00:00:00Z`), matching
  the reference `wt.json`. Same-day re-runs are byte-identical and suppressed by the no-op guard.
- ~~No-op guard vs. always-PR.~~ **Resolved:** skip the PR only when `help/shll.json` is byte-identical
  to shll.ai's main; for real changes, open the PR **and** drive it to merge (`gh pr merge --auto`).
- **shll.ai `help/` path existence.** The publish step targets `help/shll.json` (mirroring
  `help/wt.json`). Confirmed the `help/` dir exists in shll.ai. No action needed unless the site
  expects a different filename — verified at apply, not blocking.

## Clarifications

### Session 2026-06-03

Tentative resolution (Open Questions / assumptions #11, #12):

| # | Action | Detail |
|---|--------|--------|
| 11 | Changed | `captured_at` → date-granularity (`T00:00:00Z`), matching reference `wt.json` |
| 12 | Changed | No-op guard: skip PR when byte-identical to main; otherwise open the PR AND drive it to merge (`gh pr merge --auto`) |

### Session 2026-06-03 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 5 | Confirmed | — |
| 6 | Confirmed | — |
| 9 | Confirmed | — |
| 10 | Confirmed | — |
| 13 | Confirmed | — |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | JSON shape, field names, `schema_version: 1`, recursive `Node` structure copied byte-for-byte from the frozen `help/wt.json` reference. | Contract is explicitly frozen and the reference was fetched and inspected; zero interpretation latitude. | S:98 R:60 A:95 D:95 |
| 2 | Certain | Producer is a programmatic `cobra` tree walk (`rootCmd.Commands()` recursive), not regex on `-h`. | Stated verbatim in the contract; the only correct mechanism. | S:98 R:70 A:95 D:98 |
| 3 | Certain | Filter out `completion`, `help`, and `Hidden==true` children. | Explicit in the contract; reference `wt.json` confirms (no completion/help nodes). | S:97 R:75 A:95 D:97 |
| 4 | Certain | `version` read from `rootCmd.Version` (ldflags-stamped), never hardcoded. | Explicit in the contract; `main.go:22` already sets `rootCmd.Version = version` from ldflags. | S:97 R:80 A:97 D:95 |
| 5 | Certain | Producer is a new `Hidden` `shll help-dump` cobra subcommand (vs. a standalone Go tool under `scripts/`). | Clarified — user confirmed. A subcommand has free access to the live `rootCmd` + `rootCmd.Version` and self-excludes via the `Hidden` filter rule; idiomatic for this Cobra app. Hidden keeps it off the user surface (Constitution VII). | S:95 R:55 A:80 D:75 |
| 6 | Certain | `text` = `Long` (fallback `Short`) + `"\n\n"` + `UsageString()`, matching cobra's `-h` template; enforced by a byte-for-byte test against real `-h`. | Clarified — user confirmed. Reference `wt.json` text fields exhibit exactly this structure (Long blurb then Usage/Flags); template nuance mitigated by the byte-for-byte test. | S:95 R:50 A:78 D:72 |
| 7 | Certain | CI trigger is release-tag-push only — extend existing `release.yml`, no separate main-push workflow. | User decision during `/fab-new` (asked). Keeps `version` a clean release tag. | S:95 R:55 A:90 D:95 |
| 8 | Certain | Dump produced by a dedicated native `linux/amd64` runner build, stamped with the release tag, independent of the cross-compile matrix. | User decision during `/fab-new` (asked). Decouples dump from artifact packaging; runs natively on the runner. | S:95 R:65 A:90 D:92 |
| 9 | Certain | Push is a PR into `sahil87/shll.ai` (per-release branch) with `gh pr merge --auto`, authed via `SHLLAI_TOKEN`; never a direct push to main. | Clarified — user confirmed. Explicit in the contract (PR + auto-merge to avoid the multi-repo push race). `SHLLAI_TOKEN` confirmed present on the repo. | S:95 R:60 A:85 D:88 |
| 10 | Certain | `commands` serializes as `[]` (non-nil slice), never `null`, for leaves; output is `MarshalIndent` 2-space + trailing newline. | Clarified — user confirmed. Reference uses `"commands": []` and 2-space indentation; standard `encoding/json` nil-slice behavior requires explicit non-nil init. | S:95 R:70 A:88 D:80 |
| 11 | Certain | `captured_at` uses date-granularity (`T00:00:00Z`, via `time.Now().UTC().Truncate(24h).Format(RFC3339)` or equivalent date-only formatting) rather than full wall-clock, matching the reference. | Clarified — user confirmed date-only to match `wt.json` (`2026-06-02T00:00:00Z`) and minimize per-release PR churn. | S:95 R:65 A:60 D:50 |
| 12 | Certain | No-op guard (`git diff --cached --quiet`) skips the PR ONLY when `help/shll.json` is byte-identical to shll.ai's main; otherwise the workflow both opens the PR AND drives it to merge (`gh pr merge --auto`). | Clarified — user confirmed: skip identical PRs, but for real changes open *and* merge the PR (not leave it dangling). Pairs with date-only timestamp (#11) so same-day re-runs are true no-ops. | S:95 R:70 A:65 D:58 |
| 13 | Certain | `internal/proc` (Constitution I) does NOT apply to the producer — it does no subprocess execution (pure in-process tree walk); CI's git/gh shell-out lives in YAML, not Go. | Clarified — user confirmed. Constitution I governs Go subprocess invocation; the producer has none. The workflow's shell steps are outside Go code. | S:95 R:70 A:85 D:80 |

13 assumptions (13 certain, 0 confident, 0 tentative, 0 unresolved).
