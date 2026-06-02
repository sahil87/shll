# cli/commands

Top-level command surface for the `shll` binary — the cobra root, the five subcommands it wires up, the exit-code translation layer, and the hardcoded tool roster every subcommand consumes.

## Binary entry point

`shll` is a single Go binary. The entry point is `src/cmd/shll/main.go:20`:

- The package-level `version = "dev"` (`src/cmd/shll/main.go:18`) is overridden via `-ldflags "-X main.version=<v>"` at build time. The literal default is `dev` (covers `go run` and unstamped local builds).
- `main()` builds the cobra root via `newRootCmd()`, sets `rootCmd.Version = version`, executes, and translates any `RunE` error to an exit code via `translateExit`.

## Cobra root

`newRootCmd()` (`src/cmd/shll/root.go:19`) returns the cobra command with:

- `Use: "shll"`
- `Short: "meta-CLI for the sahil87 toolkit"`
- A `Long` block listing the five subcommands and noting that per-tool CLIs continue to work standalone.
- `SilenceUsage: true` and `SilenceErrors: true` — usage is not printed on RunE errors, and cobra's default error printer is suppressed. The `translateExit` layer in `main.go` owns stderr.
- `AddCommand` for `newInstallCmd()`, `newUpdateCmd()`, `newShellInitCmd()`, `newShellSetupCmd()`, `newVersionCmd()`.

Per Constitution VII (Minimal Surface Area), every top-level subcommand requires explicit justification in the change's intake. The current surface is five subcommands.

### Constitution VII justification per subcommand

These were locked in at spec time (Design Decision #1 for the original three; `install` and `shell-setup` (formerly `shell-install`) were added later, each with its own Constitution VII justification — see [cli/install](install.md#constitution-vii-justification) and the `shell-setup` bullet below) and are reproduced here:

- **`install`** — solves the bootstrap pain (a new user wants "get me the toolkit"). Distinct lifecycle from `update`: different precondition (not-installed vs. installed), different failure modes, different discoverability. Cannot cleanly fold into `update --install-missing` without inverting that command's installed-only precondition. See [cli/install](install.md) for the full justification and behavior.
- **`update`** — solves the no-single-update-command pain (`brew upgrade sahil87/tap/all` does NOT propagate to deps). Cannot be a flag on an existing tool because the entry point itself is what's missing. Also self-upgrades shll itself when shll was brew-installed — see [cli/update](update.md#shll-self-upgrade).
- **`shell-init`** — solves the cold-start cost and N-eval-line burden when multiple shell-integrating tools are installed. Per-tool `shell-init` keeps working standalone (Constitution IV).
- **`shell-setup`** (canonical name since change ri3h; `shell-install` retained as a back-compat alias) — solves the manual-rc-edit cliff in the post-`brew install` onboarding flow. Cannot be a flag on `shell-init` (it *invokes* `shell-init`, so making it a sub-flag is structurally self-referential). Cannot live in a per-tool CLI (per-tool CLIs emit their own shell-init; this command writes the cross-tool composition `eval "$(shll shell-init <shell>)"`, which is exactly what shll exists for). The `shell-setup` rename adds the alias without changing the surface-area count (still five commands). Its `--trust-tap` flag (change l6lo) records genuine Homebrew tap-trust — an orthogonal selector, not a new command (Constitution VII) — see [cli/shell-setup](shell-setup.md#the-trust-tap-flag-and-the-ceremony-seam).
- **`version`** — solves the bug-report triage pain. Cannot live on a per-tool CLI because the value is the cross-tool aggregation.

## Exit-code translation

`translateExit(err error) int` in `src/cmd/shll/main.go:38` is the single mapping from `RunE` errors to OS exit codes. It uses two error sentinels defined in `src/cmd/shll/main.go`:

- `errSilent = errors.New("shll: silent error")` (`src/cmd/shll/main.go:58`) — returned by subcommands that have already written their own diagnostic to stderr. Maps to exit code 1; `translateExit` does not write anything else.
- `errExitCode{code, msg}` (`src/cmd/shll/main.go:63`) — used when a subcommand needs an exit code other than 0 or 1. Today `shll shell-init` and `shll shell-setup` use this, exiting 2 on bad/missing shell argument and on related user-invocation errors (missing rc file, mutually-exclusive flags). If `msg` is non-empty, `translateExit` writes it to stderr.

Default fallback: any other error is printed to stderr and exits 1.

This layered design keeps cobra's own error printing out of the way (`SilenceErrors: true`) and concentrates exit-code policy in one place. The hop binary uses the same pattern; shll mirrors it.

## Subcommand factory pattern

Every subcommand follows `newXxxCmd()` returning `*cobra.Command` (no globals, no init() side effects). The cobra command's `RunE` calls a thin top-level helper (`runUpdate`, `runShellInit`, `runShellSetup`, `runVersion`) that takes explicit `io.Writer` arguments — this is the test seam: tests drive these directly with `bytes.Buffer` writers and a fake `proc.Runner` (or, for `shell-setup`, no fake — the command does file I/O only).

## Hardcoded tool roster

Defined in `src/cmd/shll/tools.go`. Constitution III (Tool Roster Source of Truth) requires this to be hardcoded and versioned with the binary — there is NO runtime discovery (no `brew tap` parsing, no filesystem walk).

```go
var Roster = []Tool{
    {Name: "wt",      Formula: "sahil87/tap/wt",  ShellInit: []string{"wt", "shell-init", "<shell>"},        Update: []string{"wt", "update"}},
    {Name: "idea",    Formula: "sahil87/tap/idea",                                                           Update: []string{"idea", "update"}},
    {Name: "tu",      Formula: "sahil87/tap/tu",  ShellInit: []string{"tu", "shell-init", "<shell>"},        Update: []string{"tu", "update"}},
    {Name: "rk",      Formula: "sahil87/tap/rk",                                                             Update: []string{"rk", "update"}},
    {Name: "hop",     Formula: "sahil87/tap/hop", ShellInit: []string{"hop", "shell-init", "<shell>"},       Update: []string{"hop", "update"}},
    {Name: "fab-kit", Formula: "sahil87/tap/fab-kit",                                                        Update: []string{"fab-kit", "update"}},
}
```

Roster invariants:

- **Order matters — leaves-first dependency order (change auvj).** The slice is declared `wt, idea, tu, rk, hop, fab-kit`: every tool appears *after* all of its dependencies. The leaves `wt, idea, tu` (no outgoing edges) precede the dependents `rk, hop, fab-kit`. `shll shell-init` concatenates output in roster order, `shll update`/`shll install` probe/upgrade/install in roster order, and `shll version` prints rows in roster order — so the single declared order drives all four consumers. **Why leaves-first**: it is *output coherence*, not a correctness fix (brew already resolves formula dependencies correctly and idempotently, and each `<tool> update` is self-update-only, so the order can neither break nor improve upgrade correctness). What it buys is that each tool's per-tool output section in `shll update` / `shll install` completes (and is counted under its own `▸ <tool>`/`==> <tool>` header) before a dependent's internal `brew upgrade` can re-touch a leaf already reported done. The invariant is enforced by `TestRosterLeavesBeforeDependents` (`src/cmd/shll/tools_test.go`) — a comment cannot fail CI, so the test guards against an accidental re-alphabetize or reorder and names the offending edge with both indices on violation. See the [leaves-first ordering Design Decision](#design-decision-leaves-first-roster-order-change-auvj) below.
- **Six tools.** Adding a tool is a `shll` release, not a runtime configuration change.
- **`Tool.ShellInit`** is the argv of the tool's shell-init invocation. Empty slice = no shell integration. The literal token `<shell>` (declared as `shellPlaceholder` in `src/cmd/shll/tools.go:39`) is substituted with the user-supplied shell name (`zsh`/`bash`) at composition time. All three integrators (`tu`, `hop`, `wt`) substitute the placeholder uniformly — three of the six roster entries carry shell integration.
- **`Tool.Update`** (added in change cczs) is the argv of the tool's own `update` invocation, mirroring `ShellInit`'s "empty slice means no capability" semantics. `shll update` delegates to this argv (appending `--skip-brew-update` when the tool advertises it) instead of calling `brew upgrade <formula>` directly, so each tool's post-upgrade side effects (e.g. rk's daemon restart) are preserved (Constitution IV). An empty slice means the tool exposes no `update` subcommand → `shll update` falls back to `brew upgrade <formula>`. **All six current roster entries populate `Update`** (`{"<name>", "update"}`) — every sahil87 tool ships an `update` subcommand. See [cli/update](update.md#behavior-contract) for the delegation/probe logic.
- **`formulaPrefix = "sahil87/tap/"`** (`src/cmd/shll/tools.go:5`) is a named constant — no magic string at the call sites.

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
| `root.go` | `newRootCmd()` — cobra root with three subcommands wired in. |
| `tools.go` | `Tool` struct (`Name`, `Formula`, `ShellInit`, `Update`), `Roster`, `formulaPrefix`, `shellPlaceholder`, and `tapName` (`"sahil87/tap"`, the `brew trust --tap` argument — distinct from `formulaPrefix`'s trailing-slash formula qualifier; added by change l6lo). |
| `brew.go` | Shared brew helpers used by the brew-coupled subcommands (`install`, `update`): `hasBrew`, `isInstalled`, `brewBinary`, `brewMissingHint`, `installBrewMissingHint`, `shllFormula`. Also hosts the `shell-setup --trust-tap` ceremony (change l6lo): `brewTrustAvailable` (capability probe), `brewTrustTap` (`brew trust --tap sahil87/tap`), `ensureTapTrust` (the function-value seam `shell_setup.go` calls — keeps subprocess work out of the file-I/O-only `shell_setup.go`), and `trustHatchHint`. `shell-init` and `version` are install-mechanism agnostic and do NOT consult brew — they detect runnable tools via `proc.ErrNotFound` from the sub-tool invocation itself. See [update](update.md) and [shell-setup](shell-setup.md#the-trust-tap-flag-and-the-ceremony-seam) for details. |
| `install.go` | `newInstallCmd()` + `runInstall`. See [install](install.md). |
| `update.go` | `newUpdateCmd()` + `runUpdate`. See [update](update.md). |
| `shell_init.go` | `newShellInitCmd()` + `runShellInit`. See [shell-init](shell-init.md). |
| `shell_setup.go` | `newShellSetupCmd()` + `runShellSetup` (renamed from `shell_install.go`/`newShellInstallCmd`/`runShellInstall` by change ri3h; `shell-install` kept as a cobra alias). See [shell-setup](shell-setup.md). |
| `version.go` | `newVersionCmd()` + `runVersion`. See [version](version.md). |
| `ui.go` *(change y630)* | Shared UI helper — TTY/`NO_COLOR` detection (`colorEnabled`), the per-tool header printer (`printToolHeader` → `▸ <tool>` / `==> <tool>`), the summary-tail printer (`printSummaryTail`), the shell-init comment-separator emitter (`toolComment` → `# ── <tool> ──`), and the named ANSI SGR constants (`ansiReset`, `ansiBold`, `ansiBoldCyan`, `ansiGreen`). Holds presentation logic only — **no** subprocess calls, **no** command of its own. Consumed by `update.go`/`install.go` (header + tail + color) and `shell_init.go` (`toolComment` only — never the color/header path, per the [eval-safety exception](shell-init.md#the-deliberate-exception--do-not-unify-onto-the--header)). |

Each command file has a paired `_test.go` (test-alongside per `code-quality.md`); `ui.go` has `ui_test.go`, which unit-tests the helpers directly (`bytes.Buffer` writers naturally hit the plain-ASCII branch).

## Shared UI helper (`ui.go`)

Three commands frame their per-tool output via the shared `ui.go` helper (change y630), so the TTY/`NO_COLOR`/glyph logic lives in exactly one place:

- [cli/update](update.md#per-tool-output-separation-change-y630) — per-tool `▸`/`==>` header (including `shll (self)`) + summary tail.
- [cli/install](install.md#per-tool-output-separation-change-y630) — mirrors update's header + tail.
- [cli/shell-init](shell-init.md#shell-comment-separator-change-y630) — `# ── <tool> ──` comment separator only; deliberately **not** the header/color path (eval-safety).

`golang.org/x/term` (TTY detection) is the codebase's only color/terminal dependency, added by change y630 — it is the codebase's first terminal inspection. `version` is deliberately untouched by this change (its lines already self-label).

## Cross-references

- Constitution I (Security First) → all subprocesses go through [`internal/proc`](../internal/proc.md). (`ui.go` makes no subprocess calls — it is pure presentation.)
- Constitution III (Wrap, Don't Reinvent) + IV (Composition, Not Replacement) → every subcommand shells out; nothing reimplements brew or per-tool logic. shll's framing prints only *around* each subprocess; sub-tool bytes are never rewritten.
- Constitution V (Graceful Degradation) → uninstalled tools never produce errors; missing tools are skipped silently. Eval-safety drives `shell-init`'s comment-separator exception (see [cli/shell-init](shell-init.md#the-deliberate-exception--do-not-unify-onto-the--header)).
- Constitution VII (Minimal Surface Area) → subcommand list is closed at five for v0.1.0 (`install`, `update`, `shell-init`, `shell-setup` (formerly `shell-install`, kept as an alias), `version`); change y630 added behavior to existing commands and a non-command helper file (`ui.go`), not a new subcommand; change ri3h renamed `shell-install` → `shell-setup` (alias preserves the old name) without changing the count.
