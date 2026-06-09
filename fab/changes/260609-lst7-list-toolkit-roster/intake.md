# Intake: `shll list` — toolkit roster command

**Change**: 260609-lst7-list-toolkit-roster
**Created**: 2026-06-09
**Status**: Draft

## Origin

Backlog item `[lst7]` (2026-06-03), invoked one-shot via `/fab-new`:

> Add a `shll list` command that prints the roster of toolkit tools shll manages — one line per tool with its name, a one-line description, and a link/hint to its GitHub repo (github.com/sahil87/<tool>), plus an installed/missing indicator (reuse the binary+version probe from `shll doctor` [d0ct]). Default to a human-readable aligned table; add `--json` for scripting. This is the real command behind the `shll list` mock shown on the shll.ai homepage (index.mdx), which currently depicts output for a command that doesn't exist. Keep the roster definition single-sourced with whatever `shll install`/`shll update` iterate over, so list can't drift from what's actually managed.

Key resolutions made before generating this intake:

- **`shll doctor` [d0ct] does not exist yet.** The task says "reuse the binary+version probe from `shll doctor`", but `doctor` is an unimplemented sibling backlog item. The probe it *would* use already exists today as `toolVersion` in `src/cmd/shll/version.go` (`proc.Run(ctx, tool.Name, "--version")`, returning `notInstalledLabel` on any failure). This is the PATH-runnable, install-mechanism-agnostic notion of "installed" already used by `shll version` and `shll shell-init` — **not** the brew probe (`isInstalled` in `brew.go`) used by `install`/`update`. `list` reuses the version-style probe and factors it into a shared, named helper so a future `doctor` can also consume it.
- **The GitHub repo for `rk` is `run-kit`, not `rk`.** Verified by HTTP probe at intake time: `github.com/sahil87/rk` → **404**, `github.com/sahil87/run-kit` → **200**. The other five tools' repos equal their binary name (`wt`, `hop`, `idea`, `tu`, `fab-kit` all → 200), and `README.md:202` already links `[run-kit](https://github.com/sahil87/run-kit)`. A naive `github.com/sahil87/<binary-name>` would emit a dead link for `rk`, defeating the feature's purpose. Therefore the repo slug is stored explicitly on the roster, defaulting to the binary name but overridden to `run-kit` for `rk`.
- **`--json` is in scope here even though `shll version` rejected it** (version Design Decision #4: "no `--json`, YAGNI"). `list` has an explicit, named script-consumer in the task ("add `--json` for scripting"), so the YAGNI rejection that applied to `version` does not apply here.

## Why

**Problem.** The shll.ai homepage (`index.mdx`, in the separate `sahil87/shll.ai` repo) shows a `shll list` mock — a roster of the managed tools with descriptions, repo links, and an installed indicator — but **the command does not exist**. The site advertises a capability the binary cannot deliver. A user who copies the mock command gets `Error: unknown command "list" for "shll"`. This is the same class of gap that motivated the `shll doctor` and `help-dump` backlog items: shll.ai references a command shll never shipped.

**Consequence if unfixed.** The homepage stays a lie. There is also no first-class way to answer "what does shll actually manage, and where do those tools live?" — today you reverse-engineer it from `shll version` (versions only, no descriptions, no repo links) or read the source `Roster`. New users have no in-CLI discovery of the toolkit.

**Why this approach.**
- **A new top-level subcommand (Constitution VII gate).** `list` cannot fold into an existing command. `version` is the closest sibling, but it is deliberately frozen as plain-text-only, no-JSON, versions-only output meant to paste into bug reports (Design Decision #4). `list` answers a different question — *what is the toolkit, described, with repo links, and is each piece present?* — and explicitly needs structured `--json` output. Bolting roster metadata + repo URLs + a `--json` mode onto `version` would either break version's frozen bug-report contract or create a `version --verbose --json` mode that is `list` in disguise. It is also not a per-tool concern (Constitution IV) — the value is the cross-tool aggregation, which is exactly what shll exists for. This justification is recorded for the surface-area count (the sixth user-facing subcommand: `install`, `update`, `shell-init`, `shell-setup`, `version`, **`list`**).
- **Single-sourced roster (the task's explicit anti-drift requirement).** `list` iterates the same `Roster` slice in `src/cmd/shll/tools.go` that `install`/`update`/`version`/`shell-init` consume. The one-line description and repo slug it needs are added as **fields on the existing `Tool` struct**, so they live in the same declaration and cannot drift from "what's actually managed." Adding/removing a tool is one edit to one slice (and a `shll` release per Constitution III), and every consumer — including `list` — updates in lockstep.
- **Reuse the existing probe.** The "installed/missing" indicator reuses the version-style PATH probe rather than inventing a third notion of installed.

## What Changes

### 1. New `Tool` struct fields: `Description` and `Repo` (`src/cmd/shll/tools.go`)

The `Tool` struct gains two fields so the roster single-sources everything `list` prints:

```go
type Tool struct {
    Name      string
    Formula   string
    ShellInit []string
    Update    []string
    // Description is a one-line, human-readable summary of what the tool does,
    // printed by `shll list`. Single-sourced here so the roster cannot drift
    // from the managed set (Constitution III).
    Description string
    // Repo is the github.com/sahil87/<Repo> slug for the tool's source
    // repository. It defaults to Name for most tools, but is NOT always equal
    // to Name: rk's repository is `run-kit` (github.com/sahil87/rk is a 404).
    // Stored explicitly so `shll list` never emits a dead link.
    Repo string
}
```

Populated roster (descriptions are first-draft one-liners; refine during apply against each tool's own `--help`/README if a better summary exists):

```go
var Roster = []Tool{
    {Name: "wt",      Formula: formulaPrefix + "wt",      ShellInit: []string{"wt", "shell-init", shellPlaceholder},  Update: []string{"wt", "update"},      Repo: "wt",      Description: "Git worktree manager — create, list, and switch worktrees"},
    {Name: "idea",    Formula: formulaPrefix + "idea",                                                                Update: []string{"idea", "update"},    Repo: "idea",    Description: "Capture and manage ideas from the terminal"},
    {Name: "tu",      Formula: formulaPrefix + "tu",      ShellInit: []string{"tu", "shell-init", shellPlaceholder},  Update: []string{"tu", "update"},      Repo: "tu",      Description: "Terminal utility belt"},
    {Name: "rk",      Formula: formulaPrefix + "rk",                                                                  Update: []string{"rk", "update"},      Repo: "run-kit", Description: "Run-kit — local web UI / iframe windows and proxy for tmux"},
    {Name: "hop",     Formula: formulaPrefix + "hop",     ShellInit: []string{"hop", "shell-init", shellPlaceholder}, Update: []string{"hop", "update"},     Repo: "hop",     Description: "Fast directory/project jumping across worktrees"},
    {Name: "fab-kit", Formula: formulaPrefix + "fab-kit",                                                             Update: []string{"fab-kit", "update"}, Repo: "fab-kit", Description: "Spec-driven change workflow toolkit (the `fab` CLI)"},
}
```

> **Description accuracy is an apply-time task, not a guess to ship blind.** The strings above are placeholders grounded in existing memory (`rk riff`/iframe windows from `cli/commands.md`, `wt` worktrees, `fab` from this very repo's tooling). During apply, confirm each against the tool's own one-line `Short`/README where available; do not invent capabilities. If a description cannot be confirmed, keep it conservative and factual.

A named constant holds the GitHub org base so the URL is not open-coded (code-quality.md — no magic strings):

```go
const githubOrgBase = "https://github.com/sahil87/" // + tool.Repo
```

### 2. Extract the shared install-probe helper (`src/cmd/shll/version.go` or a small shared seam)

`shll version` currently inlines the probe inside `toolVersion`. Extract a small, reusable, named helper that reports "is this tool runnable on PATH" so `list` (and a future `doctor`) reuse it rather than duplicating the probe:

```go
// toolInstalled reports whether the tool's binary is runnable on PATH, by
// invoking `<tool> --version` (bounded by versionTimeout) and treating ANY
// error (proc.ErrNotFound, non-zero exit, timeout) as not-installed. This is
// the install-mechanism-agnostic notion of "installed" shared by `version`,
// `list`, and (future) `doctor` — NOT the brew probe (`isInstalled`) used by
// `install`/`update`.
func toolInstalled(ctx context.Context, tool Tool) bool
```

`toolVersion` is refactored to layer on top of this (or the two share the single `proc.Run(... "--version")` call) so there is exactly one place that defines "installed = runnable". The `versionTimeout` constant (2s per tool) is reused — `list`'s worst-case runtime is bounded identically to `version`.

> **Probe-cost note.** Like `version`, `list`'s probe runs `<tool> --version` per tool. Default: run the per-tool probes concurrently (mirroring `update.go`'s established `probeRoster`/`sync.WaitGroup` pattern), collecting results indexed by roster position so output stays deterministically in roster order. This is a safe, well-precedented choice; sequential (like `version`) is an equally valid fallback if simpler. Either way `version` is untouched.

### 3. New `list` subcommand (`src/cmd/shll/list.go` + `list_test.go`)

Factory `newListCmd()` returning `*cobra.Command`, wired into `newRootCmd()`'s `AddCommand` (`src/cmd/shll/root.go`). `RunE` calls a thin seam `runList(ctx, stdout, jsonOut bool)` (the established test-seam pattern — driven directly with `bytes.Buffer` + fake `proc.Runner`).

```go
func newListCmd() *cobra.Command {
    cmd := &cobra.Command{
        Use:   "list",
        Short: "list the sahil87 tools shll manages, with install status and repo links",
        Long:  `...`,
        Args:  cobra.NoArgs,
        RunE: func(cmd *cobra.Command, _ []string) error {
            jsonOut, _ := cmd.Flags().GetBool(jsonFlag)
            return runList(cmd.Context(), cmd.OutOrStdout(), jsonOut)
        },
    }
    cmd.Flags().Bool(jsonFlag, false, jsonFlagUsage)
    return cmd
}
```

#### 3a. Default human-readable aligned table

One row per roster tool, in roster order (leaves-first: `wt, idea, tu, rk, hop, fab-kit`), column-aligned via `text/tabwriter` (same writer config as `version`: minwidth 0, tabwidth 0, padding 2, padchar space). Columns: **status indicator · name · description · repo URL**.

Proposed shape (status as a plain-ASCII marker; ANSI color applied only when stdout is a TTY and `NO_COLOR` is unset, via the existing `colorEnabled` helper in `ui.go`):

```
✓  wt        Git worktree manager — create, list, and switch worktrees   https://github.com/sahil87/wt
✓  idea      Capture and manage ideas from the terminal                  https://github.com/sahil87/idea
✓  tu        Terminal utility belt                                       https://github.com/sahil87/tu
✗  rk        Run-kit — local web UI / iframe windows and proxy           https://github.com/sahil87/run-kit
✓  hop       Fast directory/project jumping across worktrees             https://github.com/sahil87/hop
✓  fab-kit   Spec-driven change workflow toolkit (the `fab` CLI)         https://github.com/sahil87/fab-kit
```

- Installed indicator: a single glyph/word marking installed vs missing. <!-- assumed: ✓/✗ glyphs when color/TTY, ASCII fallback (e.g. "ok"/"--") for non-TTY/NO_COLOR, mirroring ui.go's glyph-vs-ASCII split for eval/paste safety -->
- The repo column is the full `https://github.com/sahil87/<Repo>` URL built from `githubOrgBase + tool.Repo`.
- No `shll` self-row (unlike `version`): `list` is the roster of *managed sub-tools*; shll itself is the manager, not a managed tool. <!-- assumed: no self-row — list enumerates the managed set, consistent with install/update which also never list shll itself as a roster tool -->

#### 3b. `--json` flag for scripting

A cobra bool flag `--json` (named constants `jsonFlag`/`jsonFlagUsage`). When set, emit a JSON array (or `{tools: [...]}` object — see Open Questions) of objects, one per roster tool, machine-stable field names. Plain JSON only — no ANSI, no table framing, regardless of TTY:

```json
[
  {"name": "wt",      "description": "Git worktree manager — ...", "repo": "https://github.com/sahil87/wt",      "installed": true},
  {"name": "idea",    "description": "Capture and manage ideas ...", "repo": "https://github.com/sahil87/idea",  "installed": true},
  {"name": "rk",      "description": "Run-kit — ...",               "repo": "https://github.com/sahil87/run-kit", "installed": false}
]
```

- Encoded via `encoding/json` with `MarshalIndent("", "  ")` and a single trailing newline (mirroring `help-dump`'s JSON discipline) so it diffs cleanly and pipes into `jq`.
- Field names are a lightweight contract; keep them stable and obvious (`name`, `description`, `repo`, `installed`). Repo is the full URL (resolved), not the bare slug, so consumers don't re-derive it — and it matches what the table column shows.

### 4. Root help text update (`src/cmd/shll/root.go`)

Add `shll list` to the `rootLong` subcommand listing so `shll --help` and the help-dump tree include it. This flows into `help/shll.json` automatically via the existing `help-dump` walk (no manual JSON edit) — which is how shll.ai picks it up.

### 5. Tests (`src/cmd/shll/list_test.go`)

Mirror `version_test.go` conventions — `installFakeRunner(t, f)` + a per-tool canned-response fake. Scenarios:

- `TestList_AllInstalled` — six rows in roster order, all marked installed, table column-aligned.
- `TestList_SomeMissing` — a tool whose `--version` fails (e.g. `rk`) marked missing; the rest installed.
- `TestList_RepoLinks` — every row's repo column is `https://github.com/sahil87/<Repo>`, and the `rk` row specifically resolves to `.../run-kit` (regression guard against the 404 footgun).
- `TestList_JSON` — `--json` emits valid JSON, len == len(Roster), field names/values correct, `installed` reflects the probe, no ANSI escapes, trailing newline.
- `TestList_NoANSI_Plain` — default output to a `bytes.Buffer` (non-TTY) has no `\x1b[` escapes (ASCII status marker path).
- `TestList_Order` — rows/array entries follow `Roster` order (index-paired to the live `Roster`, so a future reorder moves expected and actual in lockstep — no edit needed, matching `version_test.go`).
- A unit assertion that every roster `Description` is non-empty and every `Repo` is non-empty (guards against adding a tool without filling the new fields).

## Affected Memory

- `cli/list`: (new) New memory file documenting `shll list` — output shape (table + `--json`), the installed-probe reuse (`toolInstalled`), the repo-slug-≠-name footgun for `rk`/`run-kit`, and the Constitution VII justification.
- `cli/commands`: (modify) Update the `Tool` struct description (now carries `Description` + `Repo`), bump the subcommand count from five to six and add the `list` Constitution VII justification bullet, add `list.go` to the file-layout table, and note the new roster fields in the roster invariants. Update `root.go`'s "five subcommands"/"three subcommands" stale references while here.
- `cli/version`: (modify) Note that the install probe is now a shared `toolInstalled` helper (extracted from `toolVersion`), consumed by `version` and `list`; cross-link `cli/list`. The version output contract is otherwise unchanged.
- `docs/memory/index.md`: (modify) Update the `cli` domain row to include `list`.

## Impact

- **Code (new)**: `src/cmd/shll/list.go`, `src/cmd/shll/list_test.go`.
- **Code (modified)**: `src/cmd/shll/tools.go` (`Tool` struct + `Roster` + `githubOrgBase` const), `src/cmd/shll/root.go` (`AddCommand` + `rootLong`), `src/cmd/shll/version.go` (extract `toolInstalled`; `toolVersion` refactor — behavior-preserving).
- **Dependencies**: none new. `text/tabwriter`, `encoding/json`, `golang.org/x/term` (via `ui.go`'s `colorEnabled`) are all already in the codebase.
- **Constitution**: I (subprocess via `internal/proc` — the probe routes through `proc.Run`, no new `os/exec`). II (stateless — `list` re-derives install status every invocation, no cache). III (roster stays hardcoded + single-sourced; new fields live on the same slice; `Repo` resolves a real footgun without runtime discovery). IV (no per-tool logic reimplemented; the probe just runs `<tool> --version`). V (graceful degradation — a missing tool is shown as missing, never an error; `list` never exits non-zero on a missing tool). VII (new subcommand — justified above; count 5 → 6).
- **Tests**: existing `help_dump_test.go` is contract-shape only (synthetic tree) — adding a real subcommand does not break it. `version_test.go` must still pass after the `toolInstalled` extraction (behavior-preserving refactor).
- **External**: `sahil87/shll.ai` `index.mdx` mock becomes real. No change is made in that repo by this change; shll.ai pulls the help-dump on its own schedule. The mock's exact column layout in `index.mdx` is not visible from this repo — match it in spirit (status · name · description · repo); if the real output diverges from the mock, the mock is the thing that should follow the binary, not vice versa.

## Open Questions

These have working defaults (recorded as Confident assumptions below); they are listed here only as the small set of points to confirm-in-passing at apply, not blockers.

- **JSON top-level shape** *(default: bare array)*: bare array `[ {...} ]` is simpler and `jq`-friendly (`.[].name`) and symmetric with the headerless table. A wrapped object `{"tools": [...]}` would leave room for future top-level metadata but adds ceremony now (YAGNI). The help-dump contract chose a wrapped object, but that is a *versioned schema document* (different contract, with `schema_version`/`captured_at`); a flat roster listing does not need that envelope. Confirm at apply.
- **Exact installed-marker glyphs / words** for the table (`✓`/`✗` vs `ok`/`missing`, color-only?) and their non-TTY ASCII fallback — a small presentation detail to settle against `ui.go`'s existing glyph conventions during apply.
- **Description wording** — the curated one-liners in §1 are first-draft; confirm each against the tool's own `Short`/README at apply (see the §1 note). Sourcing approach is decided (curated-in-roster, per Constitution III), only the exact strings are open.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Reuse the version-style PATH probe (`proc.Run <tool> --version`, error ⇒ missing) for the installed indicator, not the brew `isInstalled` probe | Task says "binary+version probe"; `version.md` documents this as the install-mechanism-agnostic notion; brew probe is for `install`/`update` only. Config/constitution/codebase determine this. | S:90 R:80 A:90 D:85 |
| 2 | Certain | New top-level subcommand `list` is justified under Constitution VII (cannot fold into `version`; not a per-tool concern; needs `--json`) | Constitution VII requires explicit justification; the reasoning is in-scope and decided. Surface count 5→6. | S:85 R:70 A:90 D:80 |
| 3 | Certain | Store the repo slug explicitly on the roster; `rk`'s repo is `run-kit` | HTTP-probed at intake: `sahil87/rk` 404, `sahil87/run-kit` 200; `README.md:202` confirms. A naive `<name>` URL ships a dead link. | S:95 R:55 A:90 D:95 |
| 4 | Certain | Single-source description + repo as fields on the existing `Tool` struct / `Roster` slice | Task's explicit anti-drift requirement ("single-sourced with whatever install/update iterate over"); the struct already is that single source. | S:90 R:75 A:95 D:90 |
| 5 | Certain | `--json` is in scope for `list` despite `version`'s Design Decision #4 rejecting it | Direct task instruction ("add `--json` for scripting"); the YAGNI rejection was version-specific. No interpretation needed — only the reconciliation, which is decided. | S:95 R:75 A:90 D:90 |
| 6 | Certain | Default output is a `tabwriter` aligned table (reusing `version`'s writer config); `--json` is plain JSON via `encoding/json` MarshalIndent | Task says "human-readable aligned table" + "--json"; the codebase has exactly these two mechanisms (`version`→tabwriter, `help-dump`→encoding/json). Determined by instruction + codebase convention. | S:90 R:80 A:90 D:90 |
| 7 | Confident | No `shll` self-row in `list` (unlike `version`) | `list` enumerates the *managed* sub-tools; shll is the manager. Consistent with install/update never listing shll as a roster tool. | S:70 R:80 A:80 D:75 |
| 8 | Confident | Probe the roster concurrently (mirror `update.go`'s `probeRoster`/`WaitGroup`), output indexed by roster position | Well-precedented pattern already in the codebase; sequential is an equally safe fallback. Obvious default, low blast radius. | S:70 R:80 A:80 D:75 |
| 9 | Confident | JSON `repo` field is the full resolved URL, not the bare slug | Consumer convenience and parity with the table column; the full URL is the useful value. One obvious interpretation. | S:75 R:75 A:80 D:75 |
| 10 | Confident | Curated one-line descriptions in the roster (not runtime `--help` scraping) | Constitution III ("hardcoded roster is the contract") makes curated-in-roster the clear answer; runtime scraping contradicts it. Only the exact strings are open (apply-time confirm). | S:75 R:70 A:80 D:75 |
| 11 | Confident | JSON top-level is a bare array (default), not a wrapped `{tools: [...]}` object | `jq`-friendly, symmetric with the headerless table; help-dump's wrapped envelope is a versioned-schema concern that does not apply to a flat roster listing. Clear front-runner. | S:65 R:65 A:75 D:65 |

11 assumptions (6 certain, 5 confident, 0 tentative, 0 unresolved).
