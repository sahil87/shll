---
type: memory
description: "`shll list` — toolkit roster with install status, descriptions, and repo links; aligned table + `--json`; reuses the shared `toolInstalled` probe; the `rk`/`run-kit` repo-slug footgun."
---
# cli/list

`shll list` — prints the roster of sahil87 tools shll manages: one row per tool with an install-status indicator, a one-line description, and its GitHub repo URL. Default output is a column-aligned table; `--json` emits a plain JSON array for scripting.

Source: `src/cmd/shll/list.go` (+ `list_test.go`). Reuses the shared install probe in `src/cmd/shll/version.go` (`toolInstalled`/`probeToolVersion`), the `Roster` and `githubOrgBase` from `src/cmd/shll/tools.go`, and `colorEnabled` from `src/cmd/shll/ui.go`.

`list` is the real command behind the `shll list` mock shown on the shll.ai homepage (`index.mdx`, in the separate `sahil87/shll.ai` repo) — before change lst7 that mock advertised output for a command that did not exist. This is the same class of gap that motivated `doctor` and `help-dump`. shll.ai picks `list` up automatically via the `help-dump` walk (the `rootLong` listing flows into `help/shll.json`); no edit is made in the shll.ai repo by this change.

## Output shapes

`list` has exactly two output modes, selected by the `--json` flag.

### Default: aligned table

A shll-first row, then one row per roster tool in `Roster` order (leaves-first: `wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`). Columns: **status indicator · name · description · repo URL**. Column-aligned via `text/tabwriter` (`src/cmd/shll/list.go:130`) with the **same writer config as `version`**: minwidth 0, tabwidth 0, padding 2, padchar space, no flags.

```
ok  shll      the manager for the shll toolkit                                         https://github.com/sahil87/shll
ok  wt        Git worktree management — create, list, open, delete worktrees           https://github.com/sahil87/wt
ok  idea      Backlog idea management from the terminal                                https://github.com/sahil87/idea
ok  tu        Token-usage tracker for AI coding tools (Claude Code, Codex, OpenCode)   https://github.com/sahil87/tu
--  rk        Run-kit — tmux session manager with a web UI                             https://github.com/sahil87/run-kit
ok  hop       Fast directory/project jumping across worktrees                          https://github.com/sahil87/hop
ok  fab-kit   Spec-driven workspace & workflow toolkit (the `fab` CLI)                 https://github.com/sahil87/fab-kit
```

(The example shows the non-TTY ASCII status markers and `rk` missing; on a color-enabled terminal the status cells are the green `✓` / red `✗` glyphs.)

- **A shll-first self-row (change bb7r — reverses lst7's "no self-row").** `list` now prepends a `shll` row using the **plain installed marker** (`ok` / green `✓` — the *same* rendering as an installed tool, NOT a distinct "self" marker: maximum visual uniformity was chosen), the manager description `"the manager for the shll toolkit"`, and the repo URL `https://github.com/sahil87/shll`. shll is always present (it is the running binary), so the marker is always installed. This **reverses change lst7's earlier "No `shll` self-row" decision** — see [The prepended shll-first row](#the-prepended-shll-first-row-change-bb7r). There are now `len(Roster)+1` rows.
- The repo column is the full `https://github.com/sahil87/<Repo>` URL, built by `repoURL(t)` (the single URL-composition point — see [The rk/run-kit footgun](#the-rkrun-kit-footgun) below). For the shll row it is `repoURL(shllSelf)` → `https://github.com/sahil87/shll`.

### `--json`: bare JSON array

`shll list --json` emits a **bare JSON array** (not a wrapped `{"tools": […]}` envelope) — a shll-first object, then one object per roster tool in roster order. Field names are a lightweight, stable contract:

```json
[
  {
    "name": "shll",
    "description": "the manager for the shll toolkit",
    "repo": "https://github.com/sahil87/shll",
    "installed": true,
    "self": true
  },
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

- **The first object is the shll-first object (change bb7r):** `name:"shll"`, the manager description, `repo:"https://github.com/sahil87/shll"`, `installed:true` (shll is the running binary), and — uniquely — `self:true`. The `self` field is `json:"self,omitempty"`, so it is **absent on the 6 managed tools** and present (`true`) only on shll. A scripting consumer driving `brew install` recovers exactly the managed set with `select(.self != true)` (the managed tools have no `self` key, so the filter keeps them; the shll object has `self:true`, so the filter drops it). There are now `len(Roster)+1` objects. See [The prepended shll-first row](#the-prepended-shll-first-row-change-bb7r).
- `repo` is the **full resolved URL** (not the bare slug), so consumers don't re-derive it and it matches what the table column shows. `installed` is the version-style PATH probe result. The serialized struct is `listItem` (`src/cmd/shll/list.go:42`), which gained the `Self bool `json:"self,omitempty"`` field in change bb7r.
- Emitted via a `json.Encoder` (`writeListJSON`, `src/cmd/shll/list.go:168`) configured with `SetEscapeHTML(false)`, `SetIndent("", "  ")` (2-space indent), and `enc.Encode` (which appends a single trailing newline). **`SetEscapeHTML(false)` is load-bearing:** the default `encoding/json` encoder escapes `&`/`<`/`>` to their JSON `\uXXXX` Unicode-escape forms (`&` → the six characters `&`, `<` → `<`, `>` → `>` — **not** HTML entities like `&amp;`), which would mangle fab-kit's `"Spec-driven workspace & workflow toolkit"` in the raw `--json` bytes and diverge from the table column. Disabling it keeps the literal characters so the scripting output stays legible and matches the table. It remains valid JSON either way (a decoder turns `&` back into `&`); this is purely about the human-readable raw form. Guarded by `TestList_JSON`, which asserts the escaped `&` is **absent** from the raw bytes and the literal `workspace & workflow` is **present**.
- **Plain JSON only** — no ANSI, no table framing, regardless of TTY. `TestList_JSON` asserts no `\x1b[` escapes and a trailing newline.

## The prepended shll-first row (change bb7r)

Both renderers prepend a shll-first entry before walking the roster — `writeListTable` (`src/cmd/shll/list.go:134`) writes a leading table row, and `writeListJSON` (`src/cmd/shll/list.go:173`) prepends a leading `listItem`. Both derive their fields from the shared `shllSelf` descriptor (`src/cmd/shll/tools.go:131`): `shllSelf.Name` (`"shll"`), `shllSelf.Description` (`"the manager for the shll toolkit"`), and `repoURL(shllSelf)` (`https://github.com/sahil87/shll`). This is the single source of truth for "shll as a displayable entry", reused by `list`/`doctor`/`install` — see [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor-change-bb7r).

- **Table row** uses the **plain installed marker** — `statusMarker(true, color)` — the *same* rendering as an installed tool. Maximum visual uniformity was deliberately chosen over a distinct "self" marker; shll is always present (it is the running binary), so it always shows installed.
- **`--json` object** carries `Installed:true` and `Self:true`. The `Self` field is `omitempty`, so it is absent on the 6 managed tools and present only on shll — letting consumers filter shll out via `select(.self != true)` before driving `brew install` (you cannot brew-install the running orchestrator).

### Why a self-row — and why this reverses change lst7

`list` previously documented **"No `shll` self-row"** (change lst7: "shll is the manager, not a managed tool ... Exactly `len(Roster)` rows"). Change bb7r **reverses** that decision **for discoverability**: a newcomer running `shll list` (or `shll doctor`) to map the toolkit should see `shll` itself as part of the family — the toolkit reads as one unified family with shll as its manager-member, consistent with the already-shll-first `version`/`update`. The reversal is **NOT a constitutional rule** — it is a display decision recorded here, in the [`runList` doc comment](#behavior-contract), and in `README.md` (the three live locations where lst7's "no self-row" claim lived). `shll` is still **NOT** in `Roster` (Constitution III — Roster is the *managed sub-tool* list); the shll-first row is a *prepended display entry* via the shared `shllSelf` descriptor, not a Roster member, so `len(Roster)` stays 6 and the leaves-first invariant (`TestRosterLeavesBeforeDependents`) is untouched. There are now `len(Roster)+1` rows / objects.

## Behavior contract

`runList(ctx, stdout, jsonOut bool)` (`src/cmd/shll/list.go:84`) is the implementation seam (the established `runXxx(ctx, writers…)` pattern — tests drive it directly with a `bytes.Buffer` and a fake `proc.Runner`). Its doc comment (`src/cmd/shll/list.go:79-83`) documents the shll-first row **for discoverability** and notes it reverses change lst7's "no self-row" decision (the in-code half of the three-location reversal reconciliation):

1. If `ctx == nil`, default to `context.Background()` (mirrors `runVersion`).
2. `installed := probeInstalled(ctx)` — probe the whole roster's install status concurrently (see below). Note the probe is roster-only: the shll-first row's install status is not probed (shll is the running binary — always installed), so `installed` stays `len(Roster)`-long and the renderers prepend the shll entry separately.
3. Dispatch on `jsonOut`: `writeListJSON(stdout, installed)` or `writeListTable(stdout, installed)` — each prepends the shll-first entry (see [The prepended shll-first row](#the-prepended-shll-first-row-change-bb7r)).
4. Return only the writer's error — **never** an install-status error. A missing tool is reported as missing, never as a failure; `shll list` always exits 0 regardless of install status (Constitution V — Graceful Degradation). `TestList_SomeMissing` fatals on any returned error.

## The install probe (shared `toolInstalled`)

`list` reuses the **same install probe as `version`** — `toolInstalled(ctx, tool) bool` (`src/cmd/shll/version.go:85`), which layers on the single `probeToolVersion` call (`src/cmd/shll/version.go:72`): run `<tool> --version` via `proc.Run` (Constitution I — subprocess via `internal/proc`), bounded by `versionTimeout` (2s per tool, reused from `version`), and treat **ANY** error (`proc.ErrNotFound` for a missing binary, non-zero exit, timeout) as not-installed.

This is the **install-mechanism-agnostic** notion of "installed = runnable on PATH" — **NOT** the brew `isInstalled` probe (`src/cmd/shll/brew.go`) that `install`/`update` use. `list` answers "is this tool runnable?", not "is this formula brew-installed?". Sharing the helper means there is exactly one place that defines "installed = runnable", consumed today by `version` and `list` and reserved for a future `doctor`. See [cli/version §The shared install probe](/cli/version.md#the-shared-install-probe-change-lst7).

> **Note on `shll doctor`.** The originating backlog item said "reuse the binary+version probe from `shll doctor` [d0ct]", but `doctor` is an unimplemented sibling backlog item. The probe it *would* use already existed as the version-style PATH probe; change lst7 extracted it into the shared `toolInstalled`/`probeToolVersion` helpers so a future `doctor` can also consume it.

### Concurrent probe (`probeInstalled`)

`probeInstalled(ctx)` (`src/cmd/shll/list.go:101`) dispatches **one goroutine per roster tool**, joined on a `sync.WaitGroup`, writing each result into a fixed-size `[]bool` **indexed by roster position** — so output stays deterministically in roster order regardless of probe-completion order. This mirrors `update.go`'s established `probeRoster` pattern (see [cli/update §Sequential, not parallel](/cli/update.md#sequential-not-parallel--scoped-to-upgrades)). Only the dispatch is concurrent; every subprocess call still routes through `internal/proc` (Constitution I). Concurrency bounds the wall-clock to ~`versionTimeout`, not `N × versionTimeout`. (`list`'s `probeInstalled` intentionally parallels `update`'s `probeRoster` rather than reusing it — they probe different things: version-runnable vs. brew-installed + `--skip-brew-update` capability — so neither is redundant.)

## Status indicator (color/glyph gating)

`statusMarker(installed, color bool)` (`src/cmd/shll/list.go:145`) renders the install-status cell:

- **Color (TTY, `NO_COLOR` unset)** → ANSI-wrapped glyphs: green `✓` installed (`ansiGreen + statusGlyphInstalled + ansiReset`), red `✗` missing (`ansiRed + statusGlyphMissing + ansiReset`), using `ui.go`'s named ANSI constants.
- **Plain (non-TTY or `NO_COLOR`)** → ASCII tokens: `ok` installed / `--` missing, with no ANSI escapes — so non-TTY output and `NO_COLOR` stay escape-free and paste-safe, mirroring `ui.go`'s glyph-vs-ASCII split.

The color decision is computed once by `writeListTable` via `colorEnabled(w)` (`src/cmd/shll/ui.go:38`) and passed in, so `statusMarker` is trivially testable and the non-TTY/`NO_COLOR` path is guaranteed escape-free (a `bytes.Buffer` is never an `*os.File`, so tests deterministically hit the ASCII branch). The four marker strings are named constants — `statusGlyphInstalled`/`statusGlyphMissing`/`statusASCIIInstalled`/`statusASCIIMissing` (`src/cmd/shll/list.go:26`) — per code-quality.md (no magic strings). The shll-first table row reuses `statusMarker(true, color)`, so it always shows the installed marker.

## The rk/run-kit footgun

`Tool.Repo` is stored **explicitly** on the roster because a naive `github.com/sahil87/<binary-name>` URL would ship a dead link: **`github.com/sahil87/rk` is a 404** — rk's repository is named `run-kit` (HTTP-probed at intake: `sahil87/rk` → 404, `sahil87/run-kit` → 200; `README.md` already links `run-kit`). Every other tool's `Repo` equals its `Name`; only `rk` overrides it to `"run-kit"`.

`repoURL(t)` (`src/cmd/shll/list.go:119`) = `githubOrgBase + t.Repo` is the **single URL-composition point**, so the table column and the JSON `repo` field can never drift. The shll-first row also routes through it (`repoURL(shllSelf)` → `https://github.com/sahil87/shll`), so shll's repo link cannot drift either. `githubOrgBase` (`"https://github.com/sahil87/"`, `src/cmd/shll/tools.go:60`) is a named constant — no open-coded URL prefix at any call site (code-quality.md). See [cli/commands §Hardcoded tool roster](/cli/commands.md#hardcoded-tool-roster) for the `Tool` struct's `Description`/`Repo` fields and roster invariants.

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

- `TestList_AllInstalled` — `len(Roster)+1` rows: row 0 is the shll-first row (installed marker, `shllSelf.Name`, `shllSelf.Description`, `https://github.com/sahil87/shll`), then the roster in order (offset by 1), each carrying the installed ASCII marker (non-TTY path).
- `TestList_SomeMissing` — `rk`'s `--version` fails → its row shows the `--` missing marker while `hop` shows `ok`; `runList` returns nil (must never error on a missing tool). (Rows are matched by name field, robust to the shll-first prepend.)
- `TestList_RepoLinks` — every row's repo column is `https://github.com/sahil87/<Repo>`; `rk` resolves to `.../run-kit` and the dead `.../rk` link is absent (the 404 regression guard).
- `TestList_JSON` — `--json` is valid JSON, `len == len(Roster)+1`; **object 0 is the shll-first object** (`name == shllSelf.Name`, `self:true`, `installed:true`, manager description, `repo == https://github.com/sahil87/shll`); the managed-tool objects follow in roster order (offset by 1), each with `self == false` (the `omitempty` field absent), correct per-field `name`/`description`/`repo`/`installed` reflecting the probe (rk missing); trailing newline, no ANSI, and the HTML-escaped `&` is absent while the literal `workspace & workflow` is present (the `SetEscapeHTML(false)` guard).
- `TestList_NoANSI_Plain` — default output to a `bytes.Buffer` (non-TTY) has no `\x1b[` escapes.
- `TestList_Order` — JSON: `len == len(Roster)+1`, position 0 is the shll-first object, then the roster entries index-paired to the live `Roster` (offset by 1), so a future reorder moves expected and actual in lockstep (no edit needed — matching `version_test.go`).
- `TestList_RosterFieldsNonEmpty` — guards that every roster `Description` and every `Repo` is non-empty (regression against adding a tool without filling the new fields).

## Cross-references

- The shared install probe (`toolInstalled`/`probeToolVersion`) and the `version` output contract: [cli/version](/cli/version.md#the-shared-install-probe-change-lst7).
- The `Tool` struct's `Description`/`Repo` fields, `githubOrgBase`, the roster invariants, and `list.go`'s file-layout row: [cli/commands](/cli/commands.md#hardcoded-tool-roster).
- The shared `shllSelf` descriptor (the single source of the prepended shll-first row's Name/Description/Repo): [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor-change-bb7r). The sibling surfaces that also prepend it: [cli/doctor](/cli/doctor.md#the-prepended-shll-first-row-change-bb7r) (always-OK row + `--json` object) and [cli/install](/cli/install.md#the-prepended-shll-first-informational-line-change-bb7r) (informational line).
- Subprocess wrapper conventions (`proc.ErrNotFound` semantics): [internal/proc](/internal/proc.md).
- Brew detection (`isInstalled`) — used by `install`/`update`, **not** `list`: [cli/update §Detection](/cli/update.md#detection).
- Constitution V (Graceful Degradation) — a missing tool is shown as missing, `shll list` always exits 0.
- Constitution VII (Minimal Surface Area) — the new sixth subcommand, justified above.
