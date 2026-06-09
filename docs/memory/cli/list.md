# cli/list

`shll list` — prints the roster of sahil87 tools shll manages: one row per tool with an install-status indicator, a one-line description, and its GitHub repo URL. Default output is a column-aligned table; `--json` emits a plain JSON array for scripting.

Source: `src/cmd/shll/list.go` (+ `list_test.go`). Reuses the shared install probe in `src/cmd/shll/version.go` (`toolInstalled`/`probeToolVersion`), the `Roster` and `githubOrgBase` from `src/cmd/shll/tools.go`, and `colorEnabled` from `src/cmd/shll/ui.go`.

`list` is the real command behind the `shll list` mock shown on the shll.ai homepage (`index.mdx`, in the separate `sahil87/shll.ai` repo) — before change lst7 that mock advertised output for a command that did not exist. This is the same class of gap that motivated `doctor` and `help-dump`. shll.ai picks `list` up automatically via the `help-dump` walk (the `rootLong` listing flows into `help/shll.json`); no edit is made in the shll.ai repo by this change.

## Output shapes

`list` has exactly two output modes, selected by the `--json` flag.

### Default: aligned table

One row per roster tool, in `Roster` order (leaves-first: `wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`). Columns: **status indicator · name · description · repo URL**. Column-aligned via `text/tabwriter` (`src/cmd/shll/list.go:121`) with the **same writer config as `version`**: minwidth 0, tabwidth 0, padding 2, padchar space, no flags.

```
ok  wt        Git worktree management — create, list, open, delete worktrees           https://github.com/sahil87/wt
ok  idea      Backlog idea management from the terminal                                https://github.com/sahil87/idea
ok  tu        Token-usage tracker for AI coding tools (Claude Code, Codex, OpenCode)   https://github.com/sahil87/tu
--  rk        Run-kit — tmux session manager with a web UI                             https://github.com/sahil87/run-kit
ok  hop       Fast directory/project jumping across worktrees                          https://github.com/sahil87/hop
ok  fab-kit   Spec-driven workspace & workflow toolkit (the `fab` CLI)                 https://github.com/sahil87/fab-kit
```

(The example shows the non-TTY ASCII status markers and `rk` missing; on a color-enabled terminal the status cells are the green `✓` / red `✗` glyphs.)

- **No `shll` self-row.** Unlike `version` (which prints a `shll` row first), `list` enumerates the *managed sub-tools* — shll itself is the manager, not a managed tool. This is consistent with `install`/`update`, which never list `shll` as a roster tool. Exactly `len(Roster)` rows.
- The repo column is the full `https://github.com/sahil87/<Repo>` URL, built by `repoURL(t)` (the single URL-composition point — see [The rk/run-kit footgun](#the-rkrun-kit-footgun) below).

### `--json`: bare JSON array

`shll list --json` emits a **bare JSON array** (not a wrapped `{"tools": […]}` envelope), one object per roster tool in roster order. Field names are a lightweight, stable contract:

```json
[
  {
    "name": "wt",
    "description": "Git worktree management — create, list, open, delete worktrees",
    "repo": "https://github.com/sahil87/wt",
    "installed": true
  },
  {
    "name": "rk",
    "description": "Run-kit — tmux session manager with a web UI",
    "repo": "https://github.com/sahil87/run-kit",
    "installed": false
  }
]
```

- `repo` is the **full resolved URL** (not the bare slug), so consumers don't re-derive it and it matches what the table column shows. `installed` is the version-style PATH probe result. The serialized struct is `listItem` (`src/cmd/shll/list.go:37`).
- Emitted via a `json.Encoder` (`writeListJSON`, `src/cmd/shll/list.go:155`) configured with `SetEscapeHTML(false)`, `SetIndent("", "  ")` (2-space indent), and `enc.Encode` (which appends a single trailing newline). **`SetEscapeHTML(false)` is load-bearing:** the default encoder HTML-escapes `&`/`<`/`>` to their `\uXXXX` forms (`&` → the six characters `&`, `<` → `<`, `>` → `>`), which would mangle fab-kit's `"Spec-driven workspace & workflow toolkit"` in the raw `--json` bytes and diverge from the table column. Disabling it keeps the literal characters so the scripting output stays legible and matches the table. It remains valid JSON either way (a decoder turns `&` back into `&`); this is purely about the human-readable raw form. Guarded by `TestList_JSON`, which asserts the escaped `&` is **absent** from the raw bytes and the literal `workspace & workflow` is **present**.
- **Plain JSON only** — no ANSI, no table framing, regardless of TTY. `TestList_JSON` asserts no `\x1b[` escapes and a trailing newline.

## Behavior contract

`runList(ctx, stdout, jsonOut bool)` (`src/cmd/shll/list.go:75`) is the implementation seam (the established `runXxx(ctx, writers…)` pattern — tests drive it directly with a `bytes.Buffer` and a fake `proc.Runner`):

1. If `ctx == nil`, default to `context.Background()` (mirrors `runVersion`).
2. `installed := probeInstalled(ctx)` — probe the whole roster's install status concurrently (see below).
3. Dispatch on `jsonOut`: `writeListJSON(stdout, installed)` or `writeListTable(stdout, installed)`.
4. Return only the writer's error — **never** an install-status error. A missing tool is reported as missing, never as a failure; `shll list` always exits 0 regardless of install status (Constitution V — Graceful Degradation). `TestList_SomeMissing` fatals on any returned error.

## The install probe (shared `toolInstalled`)

`list` reuses the **same install probe as `version`** — `toolInstalled(ctx, tool) bool` (`src/cmd/shll/version.go:85`), which layers on the single `probeToolVersion` call (`src/cmd/shll/version.go:72`): run `<tool> --version` via `proc.Run` (Constitution I — subprocess via `internal/proc`), bounded by `versionTimeout` (2s per tool, reused from `version`), and treat **ANY** error (`proc.ErrNotFound` for a missing binary, non-zero exit, timeout) as not-installed.

This is the **install-mechanism-agnostic** notion of "installed = runnable on PATH" — **NOT** the brew `isInstalled` probe (`src/cmd/shll/brew.go`) that `install`/`update` use. `list` answers "is this tool runnable?", not "is this formula brew-installed?". Sharing the helper means there is exactly one place that defines "installed = runnable", consumed today by `version` and `list` and reserved for a future `doctor`. See [cli/version §The shared install probe](version.md#the-shared-install-probe-change-lst7).

> **Note on `shll doctor`.** The originating backlog item said "reuse the binary+version probe from `shll doctor` [d0ct]", but `doctor` is an unimplemented sibling backlog item. The probe it *would* use already existed as the version-style PATH probe; change lst7 extracted it into the shared `toolInstalled`/`probeToolVersion` helpers so a future `doctor` can also consume it.

### Concurrent probe (`probeInstalled`)

`probeInstalled(ctx)` (`src/cmd/shll/list.go:92`) dispatches **one goroutine per roster tool**, joined on a `sync.WaitGroup`, writing each result into a fixed-size `[]bool` **indexed by roster position** — so output stays deterministically in roster order regardless of probe-completion order. This mirrors `update.go`'s established `probeRoster` pattern (see [cli/update §Sequential, not parallel](update.md#sequential-not-parallel--scoped-to-upgrades)). Only the dispatch is concurrent; every subprocess call still routes through `internal/proc` (Constitution I). Concurrency bounds the wall-clock to ~`versionTimeout`, not `N × versionTimeout`. (`list`'s `probeInstalled` intentionally parallels `update`'s `probeRoster` rather than reusing it — they probe different things: version-runnable vs. brew-installed + `--skip-brew-update` capability — so neither is redundant.)

## Status indicator (color/glyph gating)

`statusMarker(installed, color bool)` (`src/cmd/shll/list.go:132`) renders the install-status cell:

- **Color (TTY, `NO_COLOR` unset)** → ANSI-wrapped glyphs: green `✓` installed (`ansiGreen + statusGlyphInstalled + ansiReset`), red `✗` missing (`ansiRed + statusGlyphMissing + ansiReset`), using `ui.go`'s named ANSI constants.
- **Plain (non-TTY or `NO_COLOR`)** → ASCII tokens: `ok` installed / `--` missing, with no ANSI escapes — so non-TTY output and `NO_COLOR` stay escape-free and paste-safe, mirroring `ui.go`'s glyph-vs-ASCII split.

The color decision is computed once by `writeListTable` via `colorEnabled(w)` (`src/cmd/shll/ui.go:38`) and passed in, so `statusMarker` is trivially testable and the non-TTY/`NO_COLOR` path is guaranteed escape-free (a `bytes.Buffer` is never an `*os.File`, so tests deterministically hit the ASCII branch). The four marker strings are named constants — `statusGlyphInstalled`/`statusGlyphMissing`/`statusASCIIInstalled`/`statusASCIIMissing` (`src/cmd/shll/list.go:26`) — per code-quality.md (no magic strings).

## The rk/run-kit footgun

`Tool.Repo` is stored **explicitly** on the roster because a naive `github.com/sahil87/<binary-name>` URL would ship a dead link: **`github.com/sahil87/rk` is a 404** — rk's repository is named `run-kit` (HTTP-probed at intake: `sahil87/rk` → 404, `sahil87/run-kit` → 200; `README.md` already links `run-kit`). Every other tool's `Repo` equals its `Name`; only `rk` overrides it to `"run-kit"`.

`repoURL(t)` (`src/cmd/shll/list.go:110`) = `githubOrgBase + t.Repo` is the **single URL-composition point**, so the table column and the JSON `repo` field can never drift. `githubOrgBase` (`"https://github.com/sahil87/"`, `src/cmd/shll/tools.go:60`) is a named constant — no open-coded URL prefix at any call site (code-quality.md). See [cli/commands §Hardcoded tool roster](commands.md#hardcoded-tool-roster) for the `Tool` struct's `Description`/`Repo` fields and roster invariants.

Regression-guarded by `TestList_RepoLinks`, which asserts every row's repo column is `https://github.com/sahil87/<Repo>`, that `rk` resolves to `.../run-kit`, **and** that the dead `.../rk` link is *absent* — the headline footgun guard.

## Constitution VII justification

> *Why a new top-level subcommand?* `list` is the **sixth user-facing subcommand** (`install`, `update`, `shell-init`, `shell-setup`, `version`, **`list`**). Per Constitution VII (Minimal Surface Area), it needs justification.
>
> `list` cannot fold into an existing command. `version` is the closest sibling, but it is deliberately **frozen** as plain-text-only, no-JSON, versions-only output meant to paste into bug reports (version Design Decision #4). `list` answers a *different* question — *what is the toolkit, described, with repo links, and is each piece present?* — and explicitly needs structured `--json` output. Bolting roster metadata + repo URLs + a `--json` mode onto `version` would either break version's frozen bug-report contract or create a `version --verbose --json` mode that is `list` in disguise.
>
> It is also **not a per-tool concern** (Constitution IV): the value is the cross-tool aggregation, which is exactly what shll exists for. And the roster it iterates is **single-sourced** with what `install`/`update`/`version`/`shell-init` consume (the same `Roster` slice), so `list` cannot drift from what's actually managed — the task's explicit anti-drift requirement.
>
> *Rejected*: `version --verbose --json` (would break version's frozen plain-text bug-report contract, or just be `list` in disguise); deriving the repo URL from `Name` alone (ships the dead `sahil87/rk` link); runtime tool discovery (Constitution III — the hardcoded roster is the contract).

## Spec-locked Design Decisions for this subcommand

### #1 Reuse the version-style PATH probe, not the brew probe

> *Why*: install-mechanism-agnostic, matches `version`/`shell-init` semantics; a future `doctor` reuses the same helper. The task asked for the "binary+version probe".
> *Rejected*: the brew `isInstalled` probe (couples "installed" to Homebrew; `install`/`update` use it, but `list` answers "is it runnable").

### #2 Repo slug stored explicitly on the roster

> *Why*: `github.com/sahil87/rk` is a 404; a naive `<name>` URL ships a dead link. `Repo` defaults to `Name`, overridden to `run-kit` for `rk`.
> *Rejected*: deriving the URL from `Name` alone.

### #3 Bare JSON array top-level

> *Why*: `jq`-friendly (`.[].name`), symmetric with the headerless table.
> *Rejected*: a `{"tools": [...]}` envelope (YAGNI; help-dump's wrapped envelope is a *versioned-schema* concern — `schema_version`/`captured_at` — that does not apply to a flat roster listing).

### #4 Concurrent probe via a `probeRoster`-style WaitGroup

> *Why*: well-precedented in `update.go`; bounds wall-clock to ~`versionTimeout` not `N × versionTimeout`; results indexed by roster position keep output in deterministic roster order.
> *Rejected*: sequential probing (valid but slower; the concurrent pattern is already established in the codebase).

### #5 Disable HTML escaping in `--json`

> *Why*: the default encoder escapes `&`/`<`/`>` to their `\uXXXX` forms (e.g. `&` → the six characters `&`), mangling fab-kit's `"workspace & workflow"` description in the raw bytes and diverging from the table column. `SetEscapeHTML(false)` keeps the literal characters legible; the output stays valid JSON.
> *Rejected*: leaving HTML escaping on (valid JSON, but the raw `--json` form is harder to read and inconsistent with the table).

## Test seam

`list_test.go` installs a fake via `installFakeRunner(t, f)` and uses `listFake(installed map[string]bool)` — a per-tool canned-response fake keyed off `req.Name` + the `--version` arg (an installed tool responds with success; an absent tool returns `proc.ErrNotFound`, mirroring `exec.LookPath` when the binary is missing). `runList` is driven directly with a `bytes.Buffer`. Mirrors `version_test.go` conventions.

Scenarios (`src/cmd/shll/list_test.go`):

- `TestList_AllInstalled` — exactly `len(Roster)` rows in roster order, each carrying the installed ASCII marker (non-TTY path).
- `TestList_SomeMissing` — `rk`'s `--version` fails → its row shows the `--` missing marker while `hop` shows `ok`; `runList` returns nil (must never error on a missing tool).
- `TestList_RepoLinks` — every row's repo column is `https://github.com/sahil87/<Repo>`; `rk` resolves to `.../run-kit` and the dead `.../rk` link is absent (the 404 regression guard).
- `TestList_JSON` — `--json` is valid JSON, `len == len(Roster)`, correct per-field `name`/`description`/`repo`/`installed`, `installed` reflects the probe (rk missing), trailing newline, no ANSI, and the HTML-escaped `&` is absent while the literal `workspace & workflow` is present (the `SetEscapeHTML(false)` guard).
- `TestList_NoANSI_Plain` — default output to a `bytes.Buffer` (non-TTY) has no `\x1b[` escapes.
- `TestList_Order` — JSON entries index-paired to the live `Roster`, so a future reorder moves expected and actual in lockstep (no edit needed — matching `version_test.go`).
- `TestList_RosterFieldsNonEmpty` — guards that every roster `Description` and every `Repo` is non-empty (regression against adding a tool without filling the new fields).

## Cross-references

- The shared install probe (`toolInstalled`/`probeToolVersion`) and the `version` output contract: [cli/version](version.md#the-shared-install-probe-change-lst7).
- The `Tool` struct's `Description`/`Repo` fields, `githubOrgBase`, the roster invariants, and `list.go`'s file-layout row: [cli/commands](commands.md#hardcoded-tool-roster).
- Subprocess wrapper conventions (`proc.ErrNotFound` semantics): [internal/proc](../internal/proc.md).
- Brew detection (`isInstalled`) — used by `install`/`update`, **not** `list`: [cli/update §Detection](update.md#detection).
- Constitution V (Graceful Degradation) — a missing tool is shown as missing, `shll list` always exits 0.
- Constitution VII (Minimal Surface Area) — the new sixth subcommand, justified above.
