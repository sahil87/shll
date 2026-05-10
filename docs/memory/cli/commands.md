# cli/commands

Top-level command surface for the `shll` binary — the cobra root, the four subcommands it wires up, the exit-code translation layer, and the hardcoded tool roster every subcommand consumes.

## Binary entry point

`shll` is a single Go binary. The entry point is `src/cmd/shll/main.go:20`:

- The package-level `version = "dev"` (`src/cmd/shll/main.go:18`) is overridden via `-ldflags "-X main.version=<v>"` at build time. The literal default is `dev` (covers `go run` and unstamped local builds).
- `main()` builds the cobra root via `newRootCmd()`, sets `rootCmd.Version = version`, executes, and translates any `RunE` error to an exit code via `translateExit`.

## Cobra root

`newRootCmd()` (`src/cmd/shll/root.go:19`) returns the cobra command with:

- `Use: "shll"`
- `Short: "meta-CLI for the sahil87 toolkit"`
- A `Long` block listing the four subcommands and noting that per-tool CLIs continue to work standalone.
- `SilenceUsage: true` and `SilenceErrors: true` — usage is not printed on RunE errors, and cobra's default error printer is suppressed. The `translateExit` layer in `main.go` owns stderr.
- `AddCommand` for `newUpdateCmd()`, `newShellInitCmd()`, `newShellInstallCmd()`, `newVersionCmd()`.

Per Constitution VII (Minimal Surface Area), the v0.1.0 surface is exactly these four subcommands. Adding a new top-level subcommand requires explicit justification in the change's intake.

### Constitution VII justification per subcommand

These are locked in at spec time and are reproduced here:

- **`update`** — solves the no-single-update-command pain (`brew upgrade sahil87/tap/all` does NOT propagate to deps). Cannot be a flag on an existing tool because the entry point itself is what's missing.
- **`shell-init`** — solves the cold-start cost and N-eval-line burden when multiple shell-integrating tools are installed. Per-tool `shell-init` keeps working standalone (Constitution IV).
- **`shell-install`** — solves the manual-rc-edit cliff in the post-`brew install` onboarding flow. Cannot be a flag on `shell-init` (it *invokes* `shell-init`, so making it a sub-flag is structurally self-referential). Cannot live in a per-tool CLI (per-tool CLIs emit their own shell-init; this command writes the cross-tool composition `eval "$(shll shell-init <shell>)"`, which is exactly what shll exists for).
- **`version`** — solves the bug-report triage pain. Cannot live on a per-tool CLI because the value is the cross-tool aggregation.

## Exit-code translation

`translateExit(err error) int` in `src/cmd/shll/main.go:38` is the single mapping from `RunE` errors to OS exit codes. It uses two error sentinels defined in `src/cmd/shll/main.go`:

- `errSilent = errors.New("shll: silent error")` (`src/cmd/shll/main.go:58`) — returned by subcommands that have already written their own diagnostic to stderr. Maps to exit code 1; `translateExit` does not write anything else.
- `errExitCode{code, msg}` (`src/cmd/shll/main.go:63`) — used when a subcommand needs an exit code other than 0 or 1. Today `shll shell-init` and `shll shell-install` use this, exiting 2 on bad/missing shell argument and on related user-invocation errors (missing rc file, mutually-exclusive flags). If `msg` is non-empty, `translateExit` writes it to stderr.

Default fallback: any other error is printed to stderr and exits 1.

This layered design keeps cobra's own error printing out of the way (`SilenceErrors: true`) and concentrates exit-code policy in one place. The hop binary uses the same pattern; shll mirrors it.

## Subcommand factory pattern

Every subcommand follows `newXxxCmd()` returning `*cobra.Command` (no globals, no init() side effects). The cobra command's `RunE` calls a thin top-level helper (`runUpdate`, `runShellInit`, `runShellInstall`, `runVersion`) that takes explicit `io.Writer` arguments — this is the test seam: tests drive these directly with `bytes.Buffer` writers and a fake `proc.Runner` (or, for `shell-install`, no fake — the command does file I/O only).

## Hardcoded tool roster

Defined in `src/cmd/shll/tools.go`. Constitution III (Tool Roster Source of Truth) requires this to be hardcoded and versioned with the binary — there is NO runtime discovery (no `brew tap` parsing, no filesystem walk).

```go
var Roster = []Tool{
    {Name: "fab-kit", Formula: "sahil87/tap/fab-kit"},
    {Name: "rk",      Formula: "sahil87/tap/rk"},
    {Name: "tu",      Formula: "sahil87/tap/tu",  ShellInit: []string{"tu", "shell-init", "<shell>"}},
    {Name: "hop",     Formula: "sahil87/tap/hop", ShellInit: []string{"hop", "shell-init", "<shell>"}},
    {Name: "wt",      Formula: "sahil87/tap/wt",  ShellInit: []string{"wt", "shell-init", "<shell>"}},
    {Name: "idea",    Formula: "sahil87/tap/idea"},
}
```

Roster invariants:

- **Order matters.** `shll shell-init` concatenates output in roster order (deterministic for users who reason about init sequencing).
- **Six tools.** Adding a tool is a `shll` release, not a runtime configuration change.
- **`Tool.ShellInit`** is the argv of the tool's shell-init invocation. Empty slice = no shell integration. The literal token `<shell>` (declared as `shellPlaceholder` in `src/cmd/shll/tools.go:31`) is substituted with the user-supplied shell name (`zsh`/`bash`) at composition time. All three integrators (`tu`, `hop`, `wt`) substitute the placeholder uniformly — three of the six roster entries carry shell integration.
- **`formulaPrefix = "sahil87/tap/"`** (`src/cmd/shll/tools.go:5`) is a named constant — no magic string at the call sites.

## File layout (src/cmd/shll/)

| File | Role |
|------|------|
| `main.go` | Entry point, version variable, `translateExit`, `errSilent`, `errExitCode`. |
| `root.go` | `newRootCmd()` — cobra root with three subcommands wired in. |
| `tools.go` | `Tool` struct, `Roster`, `formulaPrefix`, `shellPlaceholder`. |
| `brew.go` | Shared brew helpers used by every subcommand: `hasBrew`, `isInstalled`, `brewBinary`, `brewMissingHint`. See [update](update.md) for details. |
| `update.go` | `newUpdateCmd()` + `runUpdate`. See [update](update.md). |
| `shell_init.go` | `newShellInitCmd()` + `runShellInit`. See [shell-init](shell-init.md). |
| `shell_install.go` | `newShellInstallCmd()` + `runShellInstall`. See [shell-install](shell-install.md). |
| `version.go` | `newVersionCmd()` + `runVersion`. See [version](version.md). |

Each command file has a paired `_test.go` (test-alongside per `code-quality.md`).

## Cross-references

- Constitution I (Security First) → all subprocesses go through [`internal/proc`](../internal/proc.md).
- Constitution III (Wrap, Don't Reinvent) + IV (Composition, Not Replacement) → every subcommand shells out; nothing reimplements brew or per-tool logic.
- Constitution V (Graceful Degradation) → uninstalled tools never produce errors; missing tools are skipped silently.
- Constitution VII (Minimal Surface Area) → subcommand list is closed at four for v0.1.0 (`update`, `shell-init`, `shell-install`, `version`).
