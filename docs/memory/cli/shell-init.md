# cli/shell-init

`shll shell-init <shell>` — emits a single concatenated shell-init blob composed from every installed roster tool with shell integration.

Source: `src/cmd/shll/shell_init.go`. Uses the shared brew helpers in `src/cmd/shll/brew.go` and the `Roster` from `src/cmd/shll/tools.go`.

## Usage

```sh
eval "$(shll shell-init zsh)"   # in ~/.zshrc
eval "$(shll shell-init bash)"  # in ~/.bashrc
```

A single eval line replaces what would otherwise be N per-tool eval lines (today: `hop shell-init <shell>` and `wt shell-setup`).

## Behavior contract

`runShellInit(ctx, shell, stdout, stderr)` (`src/cmd/shll/shell_init.go:65`) is the implementation seam. The cobra `RunE` wrapper handles argument validation before delegating:

1. **Missing shell argument.** No positional → return `errExitCode{code: 2, msg: "shll shell-init: missing shell. Supported: zsh, bash"}`. Exit code: **2**. stdout: empty.

2. **Unsupported shell.** Argument is not in `supportedShells = []string{"zsh", "bash"}` → return `errExitCode{code: 2, msg: ...}`. Exit code: **2**. stdout: empty.

3. **Composition loop.** For each tool in `Roster` (in order):
   - Skip if `len(tool.ShellInit) == 0` (tool has no shell integration).
   - Skip silently if `!isInstalled(ctx, tool.Formula)` — Constitution V (graceful degradation).
   - Otherwise build argv via `substituteShell(tool.ShellInit, shell)` (replaces every `"<shell>"` token with the user-supplied shell name).
   - Run `proc.Run(ctx, argv[0], argv[1:]...)` (capture transport).
   - On error: write `shll shell-init: <tool>: <err>` to stderr, set `anyFailed = true`, **and skip this tool's stdout** (eval-safety — failing tool's partial output never reaches stdout). Continue with the next tool.
   - On success: write the captured stdout bytes verbatim to the output writer.

4. **Final exit.** If `anyFailed`, return `errSilent` (exit 1). Else return nil (exit 0).

## Eval-safety invariants

This is the central correctness property of `shell-init` and is non-negotiable (Constitution V; Design Decision #6):

- **stdout MUST always be eval-safe.** No matter the error path:
  - Missing/unsupported shell → empty stdout, message on stderr.
  - Sub-tool not installed → silently omitted from output.
  - Sub-tool execution fails → its (partial) stdout is dropped; the error message goes to stderr only.
  - Sub-tool returns non-zero exit → falls under the "fails" branch (proc returns a non-nil err).
- **stderr is the only diagnostic channel.** Any human-readable text — usage, error notes — goes to stderr.
- **`shll` itself never injects shell code.** stdout consists of bytes returned by sub-tools, concatenated. shll does not prepend or append anything.

This means `eval "$(shll shell-init zsh)"` is safe even when shll exits non-zero or sub-tools are broken — at worst the user gets a shell with one fewer integration loaded, never a parse error.

## Composition order

Output is concatenated in `Roster` order. This is deterministic (Spec: Composition Order). Today only `hop` and `wt` produce output; in roster order that is `hop` first, then `wt`. `TestShellInit_DeterministicOrder` asserts byte-identical stdout across two consecutive runs.

## Argv substitution

`substituteShell(argv, shell)` (`src/cmd/shll/shell_init.go:98`) replaces every literal `<shell>` token with `shell`, returning a fresh slice (does not mutate the roster):

| Tool | Roster argv | After substitution (zsh) |
|------|-------------|--------------------------|
| `hop` | `["hop", "shell-init", "<shell>"]` | `["hop", "shell-init", "zsh"]` |
| `wt`  | `["wt", "shell-setup"]` | `["wt", "shell-setup"]` (unchanged — no placeholder) |

The placeholder constant (`shellPlaceholder = "<shell>"`) lives in `src/cmd/shll/tools.go:31`.

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| Success (or no integrating tools installed) | 0 |
| Missing shell arg / unsupported shell | **2** (via `errExitCode`) |
| One or more sub-tools failed | 1 (via `errSilent`, after all tools attempted) |

Exit 2 specifically distinguishes user-error (bad CLI invocation) from runtime failure (sub-tool problem). Scripts can branch on this.

## Spec-locked Design Decisions for this subcommand

### #6 `shll shell-init` exits non-zero on sub-tool failure but keeps stdout eval-safe

> *Why*: Eval-safety is non-negotiable (Constitution V) — a broken sub-tool MUST NOT corrupt the user's shell. A non-zero exit code surfaces the failure to scripts that check it; stderr provides a human-readable note.
> *Rejected*: silent success on sub-tool failure (hides real problems); writing the error to stdout (breaks eval-safety).

## Test seam

`shell_init_test.go` installs a fake `proc.Runner` via the same `installFakeRunner(t, f)` helper used by `update_test.go`. The fake matches on `(name, argv)` to decide which sub-tool's canned stdout to return.

Covered scenarios:

- `TestShellInit_ZshBothInstalled` — hop and wt both installed → concatenated output, deterministic order, exit 0.
- `TestShellInit_BashHopOnly` — hop only → only hop output, no message about wt, exit 0.
- `TestShellInit_NoIntegratingToolsInstalled` — neither installed → empty stdout, exit 0.
- `TestShellInit_UnsupportedShell` — `fish` → empty stdout, stderr usage line, exit 2.
- `TestShellInit_MissingShellArg` — no arg → empty stdout, stderr usage line, exit 2.
- `TestShellInit_DeterministicOrder` — two runs produce byte-identical output.
- `TestShellInit_SubToolFailure` — `hop shell-init zsh` errors → stdout contains only wt output (eval-safe), stderr has the hop error line, exit 1.

## Cross-references

- Roster definition and `<shell>` placeholder: [cli/commands](commands.md#hardcoded-tool-roster).
- Subprocess wrapper conventions: [internal/proc](../internal/proc.md).
- Brew detection (`isInstalled`): [cli/update](update.md#detection).
- Constitution V (Graceful Degradation) — uninstalled tools omitted silently.
