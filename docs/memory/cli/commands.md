# cli/commands

Top-level command surface for the `shll` binary — the cobra root, the six subcommands it wires up, the exit-code translation layer, and the hardcoded tool roster every subcommand consumes.

## Binary entry point

`shll` is a single Go binary. The entry point is `src/cmd/shll/main.go:20`:

- The package-level `version = "dev"` (`src/cmd/shll/main.go:18`) is overridden via `-ldflags "-X main.version=<v>"` at build time. The literal default is `dev` (covers `go run` and unstamped local builds).
- `main()` builds the cobra root via `newRootCmd()`, sets `rootCmd.Version = version`, executes, and translates any `RunE` error to an exit code via `translateExit`.

## Cobra root

`newRootCmd()` (`src/cmd/shll/root.go:19`) returns the cobra command with:

- `Use: "shll"`
- `Short: "meta-CLI for the sahil87 toolkit"`
- A `Long` block (`rootLong`, `src/cmd/shll/root.go:7`) listing the seven user-facing subcommands and noting that per-tool CLIs continue to work standalone.
- `SilenceUsage: true` and `SilenceErrors: true` — usage is not printed on RunE errors, and cobra's default error printer is suppressed. The `translateExit` layer in `main.go` owns stderr.
- `AddCommand` (`src/cmd/shll/root.go:30`) wires **8 factory funcs**: `newDoctorCmd()`, `newInstallCmd()`, `newUpdateCmd()`, `newShellInitCmd()`, `newShellSetupCmd()`, `newVersionCmd()`, `newListCmd()`, and the hidden `newHelpDumpCmd()`. The hidden `help-dump` is not counted in the user-facing surface (see [cli/help-dump-contract](help-dump-contract.md)), so the **user-facing surface is seven**.

Per Constitution VII (Minimal Surface Area), every top-level subcommand requires explicit justification in the change's intake. The current user-facing surface is seven subcommands (the hidden `help-dump` is not counted — see [cli/help-dump-contract](help-dump-contract.md)).

### Constitution VII justification per subcommand

These were locked in at spec time (Design Decision #1 for the original three; `install`, `shell-setup` (formerly `shell-install`), and `doctor` were added later, each with its own Constitution VII justification — see [cli/install](install.md#constitution-vii-justification), the `shell-setup` bullet below, and [cli/doctor](doctor.md)) and are reproduced here:

- **`doctor`** (change d0ct) — solves the post-install verification gap referenced by the shll.ai docs (the install + idea pages name `shll doctor` as *the* verification step, but the command did not exist). Cannot be a flag on `version` — verification is a different concern (wiring checks, OK/WARN/FAIL semantics, a CI-meaningful non-zero exit) from version *reporting* (a plain table that always exits 0). Cannot be a flag on `install`/`update` — those *mutate*, whereas a doctor must be strictly read-only. Cannot live in a per-tool CLI — its value is the cross-tool aggregation plus the shll-owned rc-block wiring check (no single tool can see whether the *composed* shll block is present). Its `--json` is a flag on `doctor`, not a second subcommand. See [cli/doctor](doctor.md) for the full behavior contract.
- **`install`** — solves the bootstrap pain (a new user wants "get me the toolkit"). Distinct lifecycle from `update`: different precondition (not-installed vs. installed), different failure modes, different discoverability. Cannot cleanly fold into `update --install-missing` without inverting that command's installed-only precondition. See [cli/install](install.md) for the full justification and behavior.
- **`update`** — solves the no-single-update-command pain (`brew upgrade sahil87/tap/all` does NOT propagate to deps). Cannot be a flag on an existing tool because the entry point itself is what's missing. Also self-upgrades shll itself when shll was brew-installed — see [cli/update](update.md#shll-self-upgrade).
- **`shell-init`** — solves the cold-start cost and N-eval-line burden when multiple shell-integrating tools are installed. Per-tool `shell-init` keeps working standalone (Constitution IV).
- **`shell-setup`** (canonical name since change ri3h; `shell-install` retained as a back-compat alias) — solves the manual-rc-edit cliff in the post-`brew install` onboarding flow. Cannot be a flag on `shell-init` (it *invokes* `shell-init`, so making it a sub-flag is structurally self-referential). Cannot live in a per-tool CLI (per-tool CLIs emit their own shell-init; this command writes the cross-tool composition `eval "$(shll shell-init <shell>)"`, which is exactly what shll exists for). The `shell-setup` rename adds the alias without changing the surface-area count (five at the time of change ri3h; seven now that changes lst7 and d0ct added `list` and `doctor`). Its `--trust-tap` flag (change l6lo) records genuine Homebrew tap-trust — an orthogonal selector, not a new command (Constitution VII) — see [cli/shell-setup](shell-setup.md#the-trust-tap-flag-and-the-ceremony-seam).
- **`version`** — solves the bug-report triage pain. Cannot live on a per-tool CLI because the value is the cross-tool aggregation.
- **`list`** (change lst7) — solves the toolkit-discovery gap (the shll.ai homepage advertised a `shll list` mock for a command that did not exist). Cannot fold into `version`: `version` is deliberately frozen as plain-text-only, no-JSON, versions-only output for bug reports (version Design Decision #4), whereas `list` aggregates roster *metadata* (one-line descriptions + repo links) and needs structured `--json` — bolting that on would either break version's contract or be `list` in disguise. Not a per-tool concern (Constitution IV) — the value is the cross-tool aggregation. The roster it iterates is single-sourced with what `install`/`update`/`version`/`shell-init` consume, so it cannot drift from what's actually managed. See [cli/list](list.md#constitution-vii-justification) for the full justification and behavior.

## Exit-code translation

`translateExit(err error) int` in `src/cmd/shll/main.go:38` is the single mapping from `RunE` errors to OS exit codes. It uses two error sentinels defined in `src/cmd/shll/main.go`:

- `errSilent = errors.New("shll: silent error")` (`src/cmd/shll/main.go:58`) — returned by subcommands that have already written their own diagnostic to stderr. Maps to exit code 1; `translateExit` does not write anything else.
- `errExitCode{code, msg}` (`src/cmd/shll/main.go:63`) — used when a subcommand needs an exit code other than 0 or 1. Today `shll shell-init` and `shll shell-setup` use this, exiting 2 on bad/missing shell argument and on related user-invocation errors (missing rc file, mutually-exclusive flags). If `msg` is non-empty, `translateExit` writes it to stderr.

Default fallback: any other error is printed to stderr and exits 1.

This layered design keeps cobra's own error printing out of the way (`SilenceErrors: true`) and concentrates exit-code policy in one place. The hop binary uses the same pattern; shll mirrors it.

## Subcommand factory pattern

Every subcommand follows `newXxxCmd()` returning `*cobra.Command` (no globals, no init() side effects). The cobra command's `RunE` calls a thin top-level helper (`runDoctor`, `runUpdate`, `runShellInit`, `runShellSetup`, `runVersion`, `runList`) that takes explicit `io.Writer` arguments — this is the test seam: tests drive these directly with `bytes.Buffer` writers and a fake `proc.Runner` (or, for `shell-setup`, no fake — the command does file I/O only). `runList(ctx, stdout, jsonOut bool)` adds a `bool` flag arg alongside the writer (the `--json` selector). `runDoctor` additionally takes an `env func(string) string` seam (production passes `os.Getenv`) so the wiring check reads a `t.TempDir()` rc file via a map-backed env — mirroring `resolveShell`/`resolveRcFile`.

## Hardcoded tool roster

Defined in `src/cmd/shll/tools.go`. Constitution III (Tool Roster Source of Truth) requires this to be hardcoded and versioned with the binary — there is NO runtime discovery (no `brew tap` parsing, no filesystem walk).

The `Tool` struct carries six fields: `Name`, `Formula`, `ShellInit`, `Update`, and (added by change lst7) `Description` and `Repo`. Each `Roster` entry populates all the fields its capabilities require:

```go
var Roster = []Tool{
    {Name: "wt",      Formula: "sahil87/tap/wt",      ShellInit: []string{"wt", "shell-init", "<shell>"},  Update: []string{"wt", "update"},      Repo: "wt",      Description: "Git worktree management — create, list, open, delete worktrees"},
    {Name: "idea",    Formula: "sahil87/tap/idea",                                                         Update: []string{"idea", "update"},    Repo: "idea",    Description: "Backlog idea management from the terminal"},
    {Name: "tu",      Formula: "sahil87/tap/tu",      ShellInit: []string{"tu", "shell-init", "<shell>"},  Update: []string{"tu", "update"},      Repo: "tu",      Description: "Token-usage tracker for AI coding tools (Claude Code, Codex, OpenCode)"},
    {Name: "rk",      Formula: "sahil87/tap/rk",                                                           Update: []string{"rk", "update"},      Repo: "run-kit", Description: "Run-kit — tmux session manager with a web UI"},
    {Name: "hop",     Formula: "sahil87/tap/hop",     ShellInit: []string{"hop", "shell-init", "<shell>"}, Update: []string{"hop", "update"},     Repo: "hop",     Description: "Fast directory/project jumping across worktrees"},
    {Name: "fab-kit", Formula: "sahil87/tap/fab-kit",                                                      Update: []string{"fab-kit", "update"}, Repo: "fab-kit", Description: "Spec-driven workspace & workflow toolkit (the `fab` CLI)"},
}
```

Roster invariants:

- **Order matters — leaves-first dependency order (change auvj).** The slice is declared `wt, idea, tu, rk, hop, fab-kit`: every tool appears *after* all of its dependencies. The leaves `wt, idea, tu` (no outgoing edges) precede the dependents `rk, hop, fab-kit`. `shll shell-init` concatenates output in roster order, `shll update`/`shll install` probe/upgrade/install in roster order, and `shll version` prints rows in roster order — so the single declared order drives all four consumers. **Why leaves-first**: it is *output coherence*, not a correctness fix (brew already resolves formula dependencies correctly and idempotently, and each `<tool> update` is self-update-only, so the order can neither break nor improve upgrade correctness). What it buys is that each tool's per-tool output section in `shll update` / `shll install` completes (and is counted under its own `▸ <tool>`/`==> <tool>` header) before a dependent's internal `brew upgrade` can re-touch a leaf already reported done. The invariant is enforced by `TestRosterLeavesBeforeDependents` (`src/cmd/shll/tools_test.go`) — a comment cannot fail CI, so the test guards against an accidental re-alphabetize or reorder and names the offending edge with both indices on violation. See the [leaves-first ordering Design Decision](#design-decision-leaves-first-roster-order-change-auvj) below.
- **Six tools.** Adding a tool is a `shll` release, not a runtime configuration change.
- **`Tool.ShellInit`** is the argv of the tool's shell-init invocation. Empty slice = no shell integration. The literal token `<shell>` (declared as `shellPlaceholder` in `src/cmd/shll/tools.go:65`) is substituted with the user-supplied shell name (`zsh`/`bash`) at composition time. All three integrators (`tu`, `hop`, `wt`) substitute the placeholder uniformly — three of the six roster entries carry shell integration.
- **`Tool.Update`** (added in change cczs) is the argv of the tool's own `update` invocation, mirroring `ShellInit`'s "empty slice means no capability" semantics. `shll update` delegates to this argv (appending `--skip-brew-update` when the tool advertises it) instead of calling `brew upgrade <formula>` directly, so each tool's post-upgrade side effects (e.g. rk's daemon restart) are preserved (Constitution IV). An empty slice means the tool exposes no `update` subcommand → `shll update` falls back to `brew upgrade <formula>`. **All six current roster entries populate `Update`** (`{"<name>", "update"}`) — every sahil87 tool ships an `update` subcommand. See [cli/update](update.md#behavior-contract) for the delegation/probe logic.
- **`Tool.Description`** (added in change lst7) is a one-line, human-readable summary printed by `shll list`. Single-sourced on the roster so it cannot drift from the managed set (Constitution III). All six entries populate it (guarded by `TestList_RosterFieldsNonEmpty`).
- **`Tool.Repo`** (added in change lst7) is the `github.com/sahil87/<Repo>` slug for the tool's source repository, used by `shll list` to render the repo column. **It defaults to `Name` but is NOT always equal to it:** `rk`'s repo is `run-kit` (`github.com/sahil87/rk` is a 404 — stored explicitly so `shll list` never emits a dead link). Every other entry's `Repo` equals its `Name`. The single URL-composition point is `repoURL(t)` (`src/cmd/shll/list.go`) = `githubOrgBase + t.Repo`. The `rk`/`run-kit` regression is guarded by `TestList_RepoLinks`. See [cli/list §The rk/run-kit footgun](list.md#the-rkrun-kit-footgun).
- **`formulaPrefix = "sahil87/tap/"`** (`src/cmd/shll/tools.go:10`) is a named constant — no magic string at the call sites.
- **`githubOrgBase = "https://github.com/sahil87/"`** (`src/cmd/shll/tools.go:60`, added by change lst7) is the named constant for the GitHub org base URL — `shll list` never open-codes the URL prefix (code-quality.md). A tool's repo URL is `githubOrgBase + tool.Repo`.

## The shared `shllSelf` descriptor (change bb7r)

`shll` is the manager-member of the toolkit, but it is **deliberately NOT in `Roster`** — `Roster` is the *managed sub-tool* list (Constitution III). To represent shll uniformly across the commands that show the toolkit, change bb7r added a **single shared descriptor** in `src/cmd/shll/tools.go` — the one source of truth for "shll as a displayable entry":

```go
const shllSelfDescription = "the manager for the shll toolkit"

var shllSelf = Tool{
    Name:        shllTargetToken, // "shll"
    Description:  shllSelfDescription,
    Repo:        shllTargetToken, // → https://github.com/sahil87/shll via repoURL
}

func shllSelfVersion() string { return normalizeVersion(version) } // package var, never `shll --version`
```

- **Reuses the `Tool` struct shape, but is not a `Roster` entry.** Only `Name`, `Description`, and `Repo` are populated. shll has no managed `Formula`, no own `ShellInit` to compose, and is self-upgraded via `update.go`'s dedicated path rather than a `Roster` `Update` argv. **Why not add shll to `Roster`** (the rejected alternative): it would violate Constitution III, break the leaves-first invariant guarded by `TestRosterLeavesBeforeDependents`, and make `install`/`update`/`shell-init` try to operate on the running orchestrator itself (e.g. `brew install` the live binary). `len(Roster)` stays **6**; `TestShllSelf_NotInRoster` guards `len(Roster) == 6` and `!rosterHas("shll")`.
- **Version from the package var, never a self-subprocess.** `shllSelfVersion()` returns `normalizeVersion(version)` (the same package-level `version` var `shll version`'s first row reads), so shll's version display is consistent across surfaces and no `shll --version` self-spawn is ever issued (shll is always present — it is the running process). `TestShllSelfVersion_FromPackageVar` pins this; `TestShllSelf_Descriptor` pins the field contract (`Name`/`Description`/`Repo`, and that `Formula`/`ShellInit`/`Update` are empty).

### Unified shll-first ordering — the principle

Every command that meaningfully shows the toolkit renders **shll FIRST, then the leaves-first `Roster`** (`shll, wt, idea, tu, rk, hop, fab-kit`):

| Command | shll-first instance | bb7r code change? |
|---------|---------------------|-------------------|
| `version` | leads with a `shll` row (version from the package var) | **No** — already shll-first |
| `update` | leads with the `shll (self)` step as `[1/M]` | **No** — already shll-first |
| `list` | prepends a shll-first table row (plain installed marker) + a `--json` object carrying `self:true` (`omitempty`) | **Yes** ([reverses lst7's "no self-row"](list.md#why-a-self-row--and-why-this-reverses-change-lst7)) |
| `doctor` | prepends an always-OK shll-first row/object (checks 1+2 only, `shell_init:false`, never touches `anyFail`, excluded from the problem-count denominator) | **Yes** ([cli/doctor](doctor.md#the-prepended-shll-first-row-change-bb7r)) |
| `install` | prepends a shll-first **informational** line (`"shll — already present / self-managed"`) — never a brew action | **Yes** ([cli/install](install.md#the-prepended-shll-first-informational-line-change-bb7r)) |
| `shell-init` | **excluded** — the one documented exception | n/a |

`version` and `update` were **already** shll-first; change bb7r introduced `shllSelf`/`shllSelfVersion()` to *generalize* that pattern to the inspect/manage surface (`list`/`doctor`/`install`). `version.go` and `update.go` were **not** changed — they keep their own inline shll-first handling, agreeing with `shllSelf` by construction (same package version var). `list`/`doctor`/`install` consume `shllSelf` for the displayable Name/Description/Repo.

**`shell-init` is the one deliberate exception.** It is the only toolkit-showing command that does NOT prepend a shll entry: shll has no shell-init output of its own to compose, and `shell-init`'s stdout is `eval`'d, so a stray shll line risks breaking the user's shell (Constitution V eval-safety). See [cli/shell-init §the deliberate exception](shell-init.md#the-deliberate-exception--do-not-unify-onto-the--header).

**The lst7 "no self-row" reversal is a display decision, not a Constitution rule.** Change lst7 had documented "No `shll` self-row" in `list`; bb7r reverses it **for discoverability** (a newcomer should see shll as part of the family). The reversal is recorded in exactly three live locations — `docs/memory/cli/list.md`, the `runList` doc comment in `list.go`, and `README.md` — and is **not** promoted to a constitutional rule (Constitution VII's new-subcommand bar is not triggered: bb7r adds *behavior* to existing commands, not a new subcommand).

### Design Decision: Leaves-first Roster order (change auvj)

The `Roster` is declared in **leaves-first dependency order** — every tool that depends on another (by brew-upgrade *or* by runtime invocation) appears *after* all of its dependencies. The order is `wt, idea, tu, rk, hop, fab-kit`.

The dependency edges driving this order (encoded as data in `TestRosterLeavesBeforeDependents`, each labeled by kind):

| Dependent → dep | Kind | Cause |
|-----------------|------|-------|
| `fab-kit → wt` | brew-upgrade | fab-kit's brew formula upgrades wt |
| `fab-kit → idea` | brew-upgrade | fab-kit's brew formula upgrades idea |
| `hop → wt` | brew-upgrade **and** runtime-invocation | hop's formula upgrades wt; `hop open` delegates to wt's menu and `hop ls --trees` fans out `wt list --json` |
| `rk → wt` | runtime-invocation | `rk riff` shells out to `wt create` |

So the leaves `wt, idea, tu` (no outgoing edges) precede the dependents `rk, hop, fab-kit`; `fab-kit` is a pure dependent (no cycle).

- **Why this ordering exists**: *output coherence* in `shll update` / `shll install`, **not** correctness. Brew owns formula-dependency resolution (correct and idempotent), and each `<tool> update` is self-update-only — no tool's `update` cascades into another tool's upgrade during `shll update`. The only observable effect of the old order (`fab-kit, rk, tu, hop, wt, idea`) was that a dependent's *internal* `brew upgrade` could re-touch a leaf already reported done under its own `▸ <tool>`/`==> <tool>` header (an idempotent near-instant no-op), under-representing the per-tool framing introduced by change y630. Leaves-first keeps each tool's section complete and counted before any dependent runs.
- **Why a test, not just a comment**: a comment cannot fail CI. `TestRosterLeavesBeforeDependents` (`src/cmd/shll/tools_test.go`) builds a `name → roster index` map from the live `Roster` and asserts `index[dependent] > index[dep]` for every edge, failing with the offending edge and both indices named (e.g. `"fab-kit (index N) must come after wt (index M)"`). It encodes **both** brew-upgrade and runtime-invocation edges — a *superset* of what output-coherence strictly needs (which depends only on the brew-upgrade edges) — so the executable contract documents the toolkit's full ordering relationship. A test comment makes clear a runtime edge (e.g. `rk → wt`) does **not** mean `rk update` touches `wt` during `shll update`.
- **Why the dependency model stays implicit (Constitution III/VII)**: the `Tool` struct gains **no** `DependsOn` field, and there is no runtime topological sort or `brew deps` query. shll's contract is the hardcoded roster *list* (Constitution III); it does not own a data model of how the tools relate — brew owns the brew graph and runtime edges are the sub-tools' concern. The dependency graph lives only as slice order + the invariant test. Rejected: (a) a `DependsOn []string` field with runtime topo-sort (models the inter-tool graph as shll-owned data, more code/failure modes); (b) a runtime `brew deps` query (more brew coupling, latency, runtime discovery the constitution discourages for roster concerns).
- **Why the shared `Roster`, not an update-only iteration order**: one source of truth. Reordering the single shared slice lets `update`, `install`, `shell-init`, and `version` all inherit the order. Rejected: a second, `update`-scoped iteration order (sorting at iteration time in `update.go`) — adds a divergence risk and a second ordering concept for marginal isolation benefit.

Traceability: change `260601-auvj-reorder-roster-leaves-first`. Output coherence is an `update`/`install` concern — see [cli/update](update.md#leaves-first-roster-order-change-auvj).

## File layout (src/cmd/shll/)

| File | Role |
|------|------|
| `main.go` | Entry point, version variable, `translateExit`, `errSilent`, `errExitCode`. |
| `root.go` | `newRootCmd()` — cobra root that `AddCommand`s 8 factory funcs (7 user-facing — `doctor`, `install`, `update`, `shell-init`, `shell-setup`, `version`, `list` — plus the hidden `help-dump`); also holds the `rootLong` help text listing the seven user-facing subcommands. |
| `tools.go` | `Tool` struct (`Name`, `Formula`, `ShellInit`, `Update`, and `Description`/`Repo` added by change lst7), `Roster`, `formulaPrefix`, `githubOrgBase` (`"https://github.com/sahil87/"`, change lst7 — the repo-URL base for `shll list`), `shellPlaceholder`, `tapName` (`"sahil87/tap"`, the `brew trust --tap` argument — distinct from `formulaPrefix`'s trailing-slash formula qualifier; added by change l6lo), the positional-arg target resolver `resolveTargets` + `shllTargetToken` (change b2vg — name-validation only, single-sourced with `Roster`, shared by `update`/`install`; see [update](update.md#positional-tool-name-args--subset-targeting-change-b2vg)), and the shared shll-self display entry: `shllSelf` (the `Tool`-shaped descriptor), `shllSelfDescription` (`"the manager for the shll toolkit"`), and `shllSelfVersion()` (= `normalizeVersion(version)`), all added by change bb7r — see [the shared `shllSelf` descriptor](#the-shared-shllself-descriptor-change-bb7r). |
| `brew.go` | Shared brew helpers used by the brew-coupled subcommands (`install`, `update`): `hasBrew`, `isInstalled`, `brewBinary`, `brewMissingHint`, `installBrewMissingHint`, `shllFormula`. Also hosts the `shell-setup --trust-tap` ceremony (change l6lo): `brewTrustAvailable` (capability probe), `brewTrustTap` (`brew trust --tap sahil87/tap`), `ensureTapTrust` (the function-value seam `shell_setup.go` calls — keeps subprocess work out of the file-I/O-only `shell_setup.go`), and `trustHatchHint`. `shell-init` and `version` are install-mechanism agnostic and do NOT consult brew — they detect runnable tools via `proc.ErrNotFound` from the sub-tool invocation itself. See [update](update.md) and [shell-setup](shell-setup.md#the-trust-tap-flag-and-the-ceremony-seam) for details. |
| `doctor.go` *(change d0ct; extended by bb7r)* | `newDoctorCmd()` (registers the `--json` bool flag) + `runDoctor` + `evaluateTool` + `probeVersion` (three-way version state) + `resolveWiringFact` (read-only rc-block check) + the typed `doctorResult` struct (text + JSON render from one source) + marker/suggestion named constants. Change bb7r added `shllDoctorResult()` — the always-OK shll-first row built directly (not via `evaluateTool`, no self-subprocess), prepended in `runDoctor` and excluded from the problem-count denominator. Read-only diagnostic. See [doctor](doctor.md). |
| `install.go` *(extended by bb7r)* | `newInstallCmd()` + `runInstall` + `shllSelfInstallNote` (`"shll — already present / self-managed"`, change bb7r — the prepended shll-first informational line). See [install](install.md). |
| `update.go` | `newUpdateCmd()` + `runUpdate`. See [update](update.md). |
| `shell_init.go` | `newShellInitCmd()` + `runShellInit`. See [shell-init](shell-init.md). |
| `shell_setup.go` | `newShellSetupCmd()` + `runShellSetup` (renamed from `shell_install.go`/`newShellInstallCmd`/`runShellInstall` by change ri3h; `shell-install` kept as a cobra alias). See [shell-setup](shell-setup.md). |
| `version.go` | `newVersionCmd()` + `runVersion`; also the shared install probe `probeToolVersion` + `toolInstalled` (extracted by change lst7, consumed by `version` and `list`). See [version](version.md). |
| `list.go` *(change lst7; extended by bb7r)* | `newListCmd()` + `runList`; the two output renderers (`writeListTable` aligned `tabwriter` table, `writeListJSON` bare JSON array via `json.Encoder` with `SetEscapeHTML(false)`) — both prepend the shll-first entry (change bb7r); `probeInstalled` (concurrent per-roster-tool install probe via `toolInstalled`, indexed by position, mirroring `update.go`'s `probeRoster`); `repoURL` (the single `githubOrgBase + t.Repo` composition point); `statusMarker` (color-glyph/ASCII status cell); `jsonFlag`/`jsonFlagUsage` and the status-marker constants; the `listItem` JSON struct (gained the `Self bool `json:"self,omitempty"`` field in change bb7r). See [list](list.md). |
| `ui.go` *(change y630; extended by change 6vuo)* | Shared UI helper — TTY/`NO_COLOR` detection (`colorEnabled`), the per-tool header printer (`printToolHeader(w, name, pos, total, color)` → `▸ [N/M] <tool>` / `==> [N/M] <tool>`), the summary-tail printer (`printSummaryTail(w, succeeded, total, elapsed, color)` → appends `in <dur>` to both forms), the `formatDuration` helper (`d.Round(time.Second).String()`), the dry-run preview helpers (`previewRow` struct + `printUpdatePreview`/`printInstallPreview` → shared `printPreviewRows` aligned-column printer; header constants `updatePreviewHeaderFmt`/`installPreviewHeaderFmt`; `previewIndent`/`previewGap`), the shell-init comment-separator emitter (`toolComment` → `# ── <tool> ──`), and the named ANSI SGR constants (`ansiReset`, `ansiBold`, `ansiBoldCyan`, `ansiGreen`). Holds presentation logic only — **no** subprocess calls, **no** command of its own. Consumed by `update.go`/`install.go` (header + tail + color + preview) and `shell_init.go` (`toolComment` only — never the color/header path, per the [eval-safety exception](shell-init.md#the-deliberate-exception--do-not-unify-onto-the--header)). |
| `clock.go` *(change 6vuo)* | The injectable wall-clock seam — a single package-level `var nowFunc = time.Now`. Mirrors the `proc.Runner` package-level-swappable injection pattern: production uses the real `time.Now`; tests swap it via the `installFakeClock` t.Cleanup helper (`clock_test.go`) to a deterministic clock so the duration-bearing summary-tail golden strings stay exact. `runUpdate`/`runInstall` capture a start time after their nothing-to-do/dry-run short-circuits and compute elapsed immediately before the tail. No subprocess calls. |

Each command file has a paired `_test.go` (test-alongside per `code-quality.md`) — including `doctor_test.go` (change d0ct), which drives `runDoctor` with `bytes.Buffer` writers, a fake `proc.Runner`, and a `t.TempDir()` rc file via a map-backed env; `ui.go` has `ui_test.go`, which unit-tests the helpers directly (`bytes.Buffer` writers naturally hit the plain-ASCII branch).

## Shared UI helper (`ui.go`)

Three commands frame their per-tool output via the shared `ui.go` helper (change y630), so the TTY/`NO_COLOR`/glyph logic lives in exactly one place:

- [cli/update](update.md#per-tool-output-separation-change-y630) — per-tool `▸ [N/M]`/`==> [N/M]` header (including `shll (self)`) + summary tail + the `--dry-run` preview.
- [cli/install](install.md#per-tool-output-separation-change-y630) — mirrors update's header + tail + preview.
- [cli/shell-init](shell-init.md#shell-comment-separator-change-y630) — `# ── <tool> ──` comment separator only; deliberately **not** the header/color path (eval-safety).

`golang.org/x/term` (TTY detection) is the codebase's only color/terminal dependency, added by change y630 — it is the codebase's first terminal inspection. `version` is deliberately untouched (its lines already self-label).

### Output polish + `--dry-run` (change 6vuo)

Change 6vuo extended `ui.go` (and added `clock.go`) without adding any subcommand or dependency, applying four additive features to **both** `update` and `install`:

1. **`[N/M]` progress counters** in the per-tool header — `printToolHeader` gained `pos, total int` parameters. For `update`, `M = installed-roster-count + (1 if shll is brew-installed)`, computed up front, with `shll (self)` as `[1/M]` and first; for `install`, `M = len(missing)`.
2. **Section spacing** — a blank line before each per-tool header except the first, and a blank line before the summary tail (emitted by the commands, not the helper). Empty/short-circuit cases emit none.
3. **Run duration in the summary tail** — `printSummaryTail` gained an `elapsed time.Duration` parameter, appending `in <dur>` to both the success and partial-failure forms (duration before the em-dash in the partial form). Measured via the `nowFunc` clock seam (`clock.go`). Duration is a run fact, not an outcome claim — the honesty constraint is preserved.
4. **`--dry-run` flag on `update` and `install`** — a cobra bool flag (`dryRunFlag`/`dryRunFlagUsage` named constants in `update.go`, shared by both commands; threaded as a `dryRun bool` parameter into `runUpdate`/`runInstall`). When set, the read-only probes still run but **no write** is performed; the command prints an aligned-column preview (`printUpdatePreview`/`printInstallPreview`) of the exact per-tool commands and exits 0. This is a **flag on existing commands, NOT a new subcommand** — Constitution VII's "could this be a flag?" test is satisfied, so the surface-area count was unchanged by change 6vuo (five at that time; six since change lst7 added `list`) (recorded per the code-review rule). See [cli/update §`--dry-run`](update.md#dry-run-change-6vuo) and [cli/install §`--dry-run`](install.md#dry-run-change-6vuo).

The `update` dry-run preview renders each tool's command from the **shared `upgradeArgv`** (`update.go`) — the single source of truth `upgradeTool` uses for the live run, so the preview can never drift from what the run would do — joined to a display string via `argvString`. `ui.go` stays presentation-only: it receives already-built `[]previewRow` and only aligns/prints them, keeping all subprocess knowledge in the command files (Constitution I).

## Cross-references

- Constitution I (Security First) → all subprocesses go through [`internal/proc`](../internal/proc.md). (`ui.go` makes no subprocess calls — it is pure presentation.)
- Constitution III (Wrap, Don't Reinvent) + IV (Composition, Not Replacement) → every subcommand shells out; nothing reimplements brew or per-tool logic. shll's framing prints only *around* each subprocess; sub-tool bytes are never rewritten.
- Constitution V (Graceful Degradation) → uninstalled tools never produce errors; missing tools are skipped silently. Eval-safety drives `shell-init`'s comment-separator exception (see [cli/shell-init](shell-init.md#the-deliberate-exception--do-not-unify-onto-the--header)).
- Constitution VII (Minimal Surface Area) → the user-facing subcommand list is seven (`doctor`, `install`, `update`, `shell-init`, `shell-setup` (formerly `shell-install`, kept as an alias), `version`, `list`); change y630 added behavior to existing commands and a non-command helper file (`ui.go`), not a new subcommand; change ri3h renamed `shell-install` → `shell-setup` (alias preserves the old name) without changing the count; change 6vuo added the `--dry-run` flag to `update`/`install` and the `clock.go` seam — flags and a helper file, not a new subcommand; change lst7 added `list` (with its own Constitution VII justification — see the [per-subcommand justification](#constitution-vii-justification-per-subcommand) and [cli/list](list.md#constitution-vii-justification)); change d0ct added `doctor`, justified as a read-only cross-tool diagnostic — its `--json` is a flag on `doctor`, not a separate subcommand (see [cli/doctor](doctor.md)); change bb7r added *behavior* to existing commands (the shll-first row/object/line in `list`/`doctor`/`install`) and no new subcommand — its only recorded decision is the lst7 "no self-row" reversal (display, not constitutional) — see [the shared `shllSelf` descriptor](#the-shared-shllself-descriptor-change-bb7r).
- Constitution III (Tool Roster Source of Truth) → `shll` is deliberately NOT in `Roster` (the managed sub-tool list); the shll-first display entry is the prepended shared `shllSelf` descriptor, so `len(Roster)` stays 6 and the leaves-first invariant is untouched — see [the shared `shllSelf` descriptor](#the-shared-shllself-descriptor-change-bb7r).
