---
type: memory
description: "`shll list` — toolkit roster with install status, descriptions, and repo links; aligned table + `--json`; reuses the shared `toolInstalled`/`probeToolVersion` probe (PATH `--version` for brew-managed tools, the `rk desktop status` Probe spec for the delegated rk-desktop entry); repo URLs compose from each tool's explicit `Repo` slug — rk-desktop's is `run-kit`, not `rk-desktop` (the live repo-slug footgun)."
---
# cli/list

`shll list` — prints the roster of shll tools shll manages: one row per tool with an install-status indicator, a one-line description, and its GitHub repo URL. Default output is a column-aligned table; `--json` emits a plain JSON array for scripting.

Source: `src/cmd/shll/list.go` (+ `list_test.go`). Reuses the shared install probe in `src/cmd/shll/version.go` (`toolInstalled`/`probeToolVersion`), the `Roster` and `githubOrgBase` from `src/cmd/shll/tools.go`, and `colorEnabled` from `src/cmd/shll/ui.go`.

`list` is the real command behind the `shll list` example on the shll.ai homepage (`index.mdx`, in the separate `sahil87/shll.ai` repo). shll.ai picks `list` up automatically via the `help-dump` walk (the `rootLong` listing flows into `help/shll.json`).

## Output shapes

`list` has exactly two output modes, selected by the `--json` flag.

### Default: aligned table

A shll-first row, then one row per roster tool in `Roster` order (importance-descending with dependency adjacency: `run-kit`, `rk-desktop`, `fab-kit`, `wt`, `idea`, `tu`, `hop` — t26g). Columns: **status indicator · name · description · repo URL**. Column-aligned via `text/tabwriter` (`src/cmd/shll/list.go:130`) with the **same writer config as `version`**: minwidth 0, tabwidth 0, padding 2, padchar space, no flags.

```
ok  shll       the manager for the shll toolkit                                                                                                      https://github.com/sahil87/shll
ok  run-kit    Run-kit — tmux session manager with a web UI; can display web pages/HTML to the user and push notifications (rk stays as an alias)  https://github.com/sahil87/run-kit
--  rk-desktop Run-kit desktop viewer shell — the macOS companion app, managed via `rk desktop install`/`rk desktop update`                         https://github.com/sahil87/run-kit
ok  fab-kit    Spec-driven workspace & workflow toolkit (the `fab` CLI)                                                                               https://github.com/sahil87/fab-kit
ok  wt         Git worktree management — create, list, open, delete worktrees                                                                         https://github.com/sahil87/wt
ok  idea       Backlog idea management from the terminal                                                                                              https://github.com/sahil87/idea
ok  tu         Token-usage tracker for AI coding tools (Claude Code, Codex, OpenCode)                                                                 https://github.com/sahil87/tu
ok  hop        Fast directory/project jumping across worktrees                                                                                        https://github.com/sahil87/hop
```

(The example shows the non-TTY ASCII status markers and `rk-desktop` missing; on a color-enabled terminal the status cells are the green `✓` / red `✗` glyphs. rk-desktop's status comes from its delegated `rk desktop status` probe — no `--version` surface exists — and its repo column is `.../run-kit`, not `.../rk-desktop` (see [The run-kit repo-slug footgun](#the-run-kit-repo-slug-footgun)). A pre-rename install whose binary is still `rk` on PATH is shown *installed* via the [legacy-name probe fallback](/cli/version.md#the-legacy-name-path-probe-fallback), still under the display name `run-kit`.)

- **A shll-first self-row (bb7r).** `list` prepends a `shll` row using the **plain installed marker** (`ok` / green `✓` — the *same* rendering as an installed tool, NOT a distinct "self" marker: maximum visual uniformity was chosen), the manager description `"the manager for the shll toolkit"`, and the repo URL `https://github.com/sahil87/shll`. shll is always present (it is the running binary), so the marker is always installed — see [The prepended shll-first row](#the-prepended-shll-first-row). There are `len(Roster)+1` = 8 rows.
- The repo column is the full `https://github.com/sahil87/<Repo>` URL, built by `repoURL(t)` (the single URL-composition point — see [The run-kit repo-slug footgun](#the-run-kit-repo-slug-footgun) below). For the shll row it is `repoURL(shllSelf)` → `https://github.com/sahil87/shll`.

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
    "name": "run-kit",
    "description": "Run-kit — tmux session manager with a web UI; can display web pages/HTML to the user and push notifications (rk stays as an alias)",
    "repo": "https://github.com/sahil87/run-kit",
    "installed": true
  },
  {
    "name": "rk-desktop",
    "description": "Run-kit desktop viewer shell — the macOS companion app, managed via `rk desktop install`/`rk desktop update`",
    "repo": "https://github.com/sahil87/run-kit",
    "installed": false
  }
]
```

- **The first object is the shll-first object (bb7r):** `name:"shll"`, the manager description, `repo:"https://github.com/sahil87/shll"`, `installed:true` (shll is the running binary), and — uniquely — `self:true`. The `self` field is `json:"self,omitempty"`, so it is **absent on the 7 managed tools** and present (`true`) only on shll. A scripting consumer driving installs recovers exactly the managed set with `select(.self != true)` (the managed tools have no `self` key, so the filter keeps them; the shll object has `self:true`, so the filter drops it). There are `len(Roster)+1` = 8 objects. See [The prepended shll-first row](#the-prepended-shll-first-row).
- `repo` is the **full resolved URL** (not the bare slug), so consumers don't re-derive it and it matches what the table column shows — rk-desktop's is `https://github.com/sahil87/run-kit` (its explicit `Repo`, not its `Name`). `installed` is the shared `toolInstalled` probe result (the PATH `--version` probe for brew-managed tools, the delegated Probe spec for rk-desktop). The serialized struct is `listItem` (`src/cmd/shll/list.go:42`), whose `Self bool `json:"self,omitempty"`` field marks the shll object (bb7r).
- Emitted via a `json.Encoder` (`writeListJSON`, `src/cmd/shll/list.go:174`) configured with `SetEscapeHTML(false)`, `SetIndent("", "  ")` (2-space indent), and `enc.Encode` (which appends a single trailing newline). **`SetEscapeHTML(false)` is load-bearing:** the default `encoding/json` encoder escapes `&`/`<`/`>` to their JSON `\uXXXX` Unicode-escape forms (`&` → the six characters `&`, `<` → `<`, `>` → `>` — **not** HTML entities like `&amp;`), which would mangle fab-kit's `"Spec-driven workspace & workflow toolkit"` in the raw `--json` bytes and diverge from the table column. Disabling it keeps the literal characters so the scripting output stays legible and matches the table. It remains valid JSON either way (a decoder turns `&` back into `&`); this is purely about the human-readable raw form. Guarded by `TestList_JSON`, which asserts the escaped `&` is **absent** from the raw bytes and the literal `workspace & workflow` is **present**.
- **Plain JSON only** — no ANSI, no table framing, regardless of TTY. `TestList_JSON` asserts no `\x1b[` escapes and a trailing newline.

## The prepended shll-first row

Both renderers prepend a shll-first entry before walking the roster — `writeListTable` (`src/cmd/shll/list.go:128`) writes a leading table row, and `writeListJSON` (`src/cmd/shll/list.go:174`) prepends a leading `listItem`. Both derive their fields from the shared `shllSelf` descriptor (`src/cmd/shll/tools.go:268`): `shllSelf.Name` (`"shll"`), `shllSelf.Description` (`"the manager for the shll toolkit"`), and `repoURL(shllSelf)` (`https://github.com/sahil87/shll`). This is the single source of truth for "shll as a displayable entry", reused by `list`/`doctor`/`install` — see [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor).

- **Table row** uses the **plain installed marker** — `statusMarker(true, color)` — the *same* rendering as an installed tool. Maximum visual uniformity was deliberately chosen over a distinct "self" marker; shll is always present (it is the running binary), so it always shows installed.
- **`--json` object** carries `Installed:true` and `Self:true`. The `Self` field is `omitempty`, so it is absent on the 7 managed tools and present only on shll — letting consumers filter shll out via `select(.self != true)` before driving installs (you cannot brew-install the running orchestrator).

### Why a self-row

The self-row exists **for discoverability**: a newcomer running `shll list` (or `shll doctor`) to map the toolkit should see `shll` itself as part of the family — the toolkit reads as one unified family with shll as its manager-member, consistent with the shll-first `version`/`update`. **Rejected**: no self-row ("shll is the manager, not a managed tool" — lst7's original posture, reversed by bb7r). The self-row is **NOT a constitutional rule** — it is a display decision recorded here, in the [`runList` doc comment](#behavior-contract), and in `README.md`. `shll` is still **NOT** in `Roster` (Constitution III — Roster is the *managed sub-tool* list); the shll-first row is a *prepended display entry* via the shared `shllSelf` descriptor, not a Roster member, so `len(Roster)` stays 7 and the exact-order invariant (`TestRosterOrder`) is untouched. There are `len(Roster)+1` rows / objects.

## Behavior contract

`runList(ctx, stdout, jsonOut bool)` (`src/cmd/shll/list.go:84`) is the implementation seam (the established `runXxx(ctx, writers…)` pattern — tests drive it directly with a `bytes.Buffer` and a fake `proc.Runner`). Its doc comment (`src/cmd/shll/list.go:73-83`) documents the shll-first row **for discoverability** (the in-code location of the three-location decision record):

1. If `ctx == nil`, default to `context.Background()` (mirrors `runVersion`).
2. `installed := probeInstalled(ctx)` — probe the whole roster's install status concurrently (see below). Note the probe is roster-only: the shll-first row's install status is not probed (shll is the running binary — always installed), so `installed` stays `len(Roster)`-long and the renderers prepend the shll entry separately.
3. Dispatch on `jsonOut`: `writeListJSON(stdout, installed)` or `writeListTable(stdout, installed)` — each prepends the shll-first entry (see [The prepended shll-first row](#the-prepended-shll-first-row)).
4. Return only the writer's error — **never** an install-status error. A missing tool is reported as missing, never as a failure; `shll list` always exits 0 regardless of install status (Constitution V — Graceful Degradation). `TestList_SomeMissing` fatals on any returned error.

## The install probe (shared `toolInstalled`)

`list` reuses the **same install probe as `version`** — `toolInstalled(ctx, tool) bool` (`src/cmd/shll/version.go:192`), which layers on the single `probeToolVersion` call (`src/cmd/shll/version.go:94`). For a brew-managed tool that is the bounded `<tool> --version` PATH invocation via `proc.Run` (Constitution I — subprocess via `internal/proc`), capped by `versionTimeout` (2s per tool, reused from `version`), treating **ANY** error (`proc.ErrNotFound` for a missing binary, non-zero exit, timeout) as not-installed. For the delegated rk-desktop entry the same helper runs the `Probe` spec (`rk desktop status`, parsing the `Installed:` line) instead — rk-desktop ships no `--version` surface (t26g).

This is the **install-mechanism-agnostic** notion of "installed = runnable" — **NOT** the brew `isInstalled` probe (`src/cmd/shll/brew.go`) that `install`/`update` use. `list` answers "is this tool runnable?", not "is this formula brew-installed?". Sharing the helper means there is exactly one place that defines "installed = runnable", consumed by `version`, `list`, and `doctor` — PATH-based for brew-managed tools, Probe-spec-based for the delegated entry. See [cli/version §The shared install probe](/cli/version.md#the-shared-install-probe).

### Concurrent probe (`probeInstalled`)

`probeInstalled(ctx)` (`src/cmd/shll/list.go:101`) dispatches **one goroutine per roster tool**, joined on a `sync.WaitGroup`, writing each result into a fixed-size `[]bool` **indexed by roster position** — so output stays deterministically in roster order regardless of probe-completion order. This mirrors `update.go`'s established `probeRoster` pattern (see [cli/update §Sequential, not parallel](/cli/update.md#sequential-not-parallel--scoped-to-upgrades)). Only the dispatch is concurrent; every subprocess call still routes through `internal/proc` (Constitution I). Concurrency bounds the wall-clock to ~`versionTimeout`, not `N × versionTimeout`. (`list`'s `probeInstalled` intentionally parallels `update`'s `probeRoster` rather than reusing it — they probe different things: version-runnable vs. brew-installed + `--skip-brew-update` capability — so neither is redundant.)

## Status indicator (color/glyph gating)

`statusMarker(installed, color bool)` (`src/cmd/shll/list.go:145`) renders the install-status cell:

- **Color (TTY, `NO_COLOR` unset)** → ANSI-wrapped glyphs: green `✓` installed (`ansiGreen + statusGlyphInstalled + ansiReset`), red `✗` missing (`ansiRed + statusGlyphMissing + ansiReset`), using `ui.go`'s named ANSI constants.
- **Plain (non-TTY or `NO_COLOR`)** → ASCII tokens: `ok` installed / `--` missing, with no ANSI escapes — so non-TTY output and `NO_COLOR` stay escape-free and paste-safe, mirroring `ui.go`'s glyph-vs-ASCII split.

The color decision is computed once by `writeListTable` via `colorEnabled(w)` (`src/cmd/shll/ui.go:38`) and passed in, so `statusMarker` is trivially testable and the non-TTY/`NO_COLOR` path is guaranteed escape-free (a `bytes.Buffer` is never an `*os.File`, so tests deterministically hit the ASCII branch). The four marker strings are named constants — `statusGlyphInstalled`/`statusGlyphMissing`/`statusASCIIInstalled`/`statusASCIIMissing` (`src/cmd/shll/list.go:26`) — per code-quality.md (no magic strings). The shll-first table row reuses `statusMarker(true, color)`, so it always shows the installed marker.

## The run-kit repo-slug footgun

`Tool.Repo` is stored **explicitly** on the roster (not derived from `Name`) — and the divergence it guards is **live**: rk-desktop's `Repo` is `run-kit` (it ships with the run-kit repo and has no repo of its own), so a URL derived from its `Name` would be the dead `https://github.com/sahil87/rk-desktop` link (t26g). The field guards a footgun class: a tool whose binary name and repo slug diverge ships a **dead link** if the URL is derived from the binary name (the pre-rename `rk` binary lived in the `run-kit` repo — `github.com/sahil87/rk` was a 404). `Name == Repo` holds for every brew-managed entry; rk-desktop is the sole exception. (The `legacyAliases`/`LegacyName` machinery, not `Repo`, carries the `rk` compatibility surface — see [cli/commands §the rk→run-kit rename](/cli/commands.md#the-rkrun-kit-rename).)

`repoURL(t)` (`src/cmd/shll/list.go:119`) = `githubOrgBase + t.Repo` is the **single URL-composition point**, so the table column and the JSON `repo` field can never drift. The shll-first row also routes through it (`repoURL(shllSelf)` → `https://github.com/sahil87/shll`), so shll's repo link cannot drift either. `githubOrgBase` (`"https://github.com/sahil87/"`, `src/cmd/shll/tools.go:162`) is a named constant — no open-coded URL prefix at any call site (code-quality.md). See [cli/commands §Hardcoded tool roster](/cli/commands.md#hardcoded-tool-roster) for the `Tool` struct's `Description`/`Repo` fields and roster invariants.

Regression-guarded by `TestList_RepoLinks`, which asserts every row's repo column is `https://github.com/sahil87/<Repo>`, that `run-kit` resolves to `.../run-kit`, **and** that the dead `.../rk` link is *absent*. rk-desktop's row resolves to `.../run-kit` through the same per-row assertion — the explicit `Repo` field doing its job.

## Constitution VII justification

> *Why a new top-level subcommand?* Per Constitution VII (Minimal Surface Area), `list` needs justification.
>
> `list` cannot fold into an existing command. `version` is the closest sibling, but it is deliberately **frozen** as plain-text-only, no-JSON, versions-only output meant to paste into bug reports (version Design Decision #4). `list` answers a *different* question — *what is the toolkit, described, with repo links, and is each piece present?* — and explicitly needs structured `--json` output. Bolting roster metadata + repo URLs + a `--json` mode onto `version` would either break version's frozen bug-report contract or create a `version --verbose --json` mode that is `list` in disguise.
>
> It is also **not a per-tool concern** (Constitution IV): the value is the cross-tool aggregation, which is exactly what shll exists for. And the roster it iterates is **single-sourced** with what `install`/`update`/`version`/`shell-init` consume (the same `Roster` slice), so `list` cannot drift from what's actually managed — the task's explicit anti-drift requirement.
>
> *Rejected*: `version --verbose --json` (would break version's frozen plain-text bug-report contract, or just be `list` in disguise); deriving the repo URL from `Name` alone (ships the dead `sahil87/rk` link); runtime tool discovery (Constitution III — the hardcoded roster is the contract).

## Spec-locked Design Decisions for this subcommand

### #1 Reuse the version-style probe, not the brew probe

> *Why*: install-mechanism-agnostic, matches `version`/`shell-init` semantics; `doctor` reuses the same helper. The shared helper itself branches on the tool's install seam — the `<tool> --version` PATH probe for brew-managed tools, the `Probe` spec for the delegated rk-desktop entry (t26g).
> *Rejected*: the brew `isInstalled` probe (couples "installed" to Homebrew; `install`/`update` use it, but `list` answers "is it runnable").

### #2 Repo slug stored explicitly on the roster

> *Why*: a binary-name/repo-slug divergence ships a dead link if the URL is derived from `Name` alone (the pre-rename `rk` binary lived in the `run-kit` repo — `github.com/sahil87/rk` was a 404). rk-desktop is the live divergence: its `Repo` is `run-kit` (it ships with the run-kit repo), so a `Name`-derived URL would 404 at `.../rk-desktop` (t26g). `Name == Repo` holds for every brew-managed entry.
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

`list_test.go` installs a fake via `installFakeRunner(t, f)` and uses `listFake(installed map[string]bool)` — a per-tool canned-response fake keyed off `req.Name`: a brew-managed tool matches the `--version` arg (installed → success; absent → `proc.ErrNotFound`, mirroring `exec.LookPath` when the binary is missing), and rk-desktop's delegated probe (`rk desktop status`) answers from the same map with an `Installed:` line — a version when installed, the spec's absent value otherwise. `allInstalled()` builds the full-roster installed map. `runList` is driven directly with a `bytes.Buffer`. Mirrors `version_test.go` conventions. (The same package also carries the shared `isRkDesktopProbe`/`rkDesktopStatusResult` fake helpers in `update_test.go`, used by the update/doctor rk-desktop scenarios; there are no `TestList_RkDesktop*` tests — rk-desktop's row is covered by the roster-index-paired assertions below plus `listFake`'s delegated-probe branch, and by `version_test.go`'s `TestVersion_RkDesktopRow` for the shared probe.)

Scenarios (`src/cmd/shll/list_test.go`):

- `TestList_AllInstalled` — `len(Roster)+1` rows: row 0 is the shll-first row (installed marker, `shllSelf.Name`, `shllSelf.Description`, `https://github.com/sahil87/shll`), then the roster in order (offset by 1), each carrying the installed ASCII marker (non-TTY path).
- `TestList_SomeMissing` — `run-kit`'s `--version` fails (and the legacy `rk` fallback is likewise absent) → its row shows the `--` missing marker while `hop` shows `ok`; `runList` returns nil (must never error on a missing tool). (Rows are matched by name field, robust to the shll-first prepend.)
- `TestList_RepoLinks` — every row's repo column is `https://github.com/sahil87/<Repo>`; `run-kit` resolves to `.../run-kit` and the dead `.../rk` link is absent (the 404 regression guard). rk-desktop's row resolves to `.../run-kit` via its explicit `Repo` — the live divergence the field exists for.
- `TestList_JSON` — `--json` is valid JSON, `len == len(Roster)+1`; **object 0 is the shll-first object** (`name == shllSelf.Name`, `self:true`, `installed:true`, manager description, `repo == https://github.com/sahil87/shll`); the managed-tool objects follow in roster order (offset by 1), each with `self == false` (the `omitempty` field absent — the raw bytes contain exactly one `"self"` key), correct per-field `name`/`description`/`repo`/`installed` reflecting the probe (run-kit missing); trailing newline, no ANSI, and the HTML-escaped `&` is absent while the literal `workspace & workflow` is present (the `SetEscapeHTML(false)` guard).
- `TestList_NoANSI_Plain` — default output to a `bytes.Buffer` (non-TTY) has no `\x1b[` escapes.
- `TestList_Order` — JSON: `len == len(Roster)+1`, position 0 is the shll-first object, then the roster entries index-paired to the live `Roster` (offset by 1), so a reorder moves expected and actual in lockstep (no edit needed — matching `version_test.go`).
- `TestList_RosterFieldsNonEmpty` — guards that every roster `Description` and every `Repo` is non-empty (regression against adding a tool without filling the fields).

## Cross-references

- The shared install probe (`toolInstalled`/`probeToolVersion`) and the `version` output contract: [cli/version](/cli/version.md#the-shared-install-probe).
- The `Tool` struct's `Description`/`Repo` fields, `githubOrgBase`, the roster invariants, and `list.go`'s file-layout row: [cli/commands](/cli/commands.md#hardcoded-tool-roster).
- The shared `shllSelf` descriptor (the single source of the prepended shll-first row's Name/Description/Repo): [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor). The sibling surfaces that also prepend it: [cli/doctor](/cli/doctor.md#the-prepended-shll-first-row) (always-OK row + `--json` object) and [cli/install](/cli/install.md#the-prepended-shll-first-informational-line) (informational line).
- Subprocess wrapper conventions (`proc.ErrNotFound` semantics): [internal/proc](/internal/proc.md).
- Brew detection (`isInstalled`) — used by `install`/`update`, **not** `list`: [cli/update §Detection](/cli/update.md#detection).
- Constitution V (Graceful Degradation) — a missing tool is shown as missing, `shll list` always exits 0.
- Constitution VII (Minimal Surface Area) — justified above.
