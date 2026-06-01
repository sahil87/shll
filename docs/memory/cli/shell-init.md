# cli/shell-init

`shll shell-init <shell>` — emits a single concatenated shell-init blob composed from every installed roster tool with shell integration.

Source: `src/cmd/shll/shell_init.go`. Uses the `Roster` from `src/cmd/shll/tools.go`. Does **not** call brew — install-state is detected via `proc.ErrNotFound` from the sub-tool invocation itself (i.e. "is the binary on PATH?"), which is install-mechanism agnostic (brew, from-source via `just install`, etc.).

## Usage

```sh
eval "$(shll shell-init zsh)"   # in ~/.zshrc
eval "$(shll shell-init bash)"  # in ~/.bashrc
```

A single eval line replaces what would otherwise be N per-tool eval lines (today: `wt shell-init <shell>`, `tu shell-init <shell>`, and `hop shell-init <shell>`).

## Behavior contract

`runShellInit(ctx, shell, stdout, stderr)` (`src/cmd/shll/shell_init.go:74`) is the implementation seam. The cobra `RunE` wrapper handles argument validation before delegating:

1. **Missing shell argument.** No positional → return `errExitCode{code: 2, msg: "shll shell-init: missing shell. Supported: zsh, bash"}`. Exit code: **2**. stdout: empty.

2. **Unsupported shell.** Argument is not in `supportedShells = []string{"zsh", "bash"}` → return `errExitCode{code: 2, msg: ...}`. Exit code: **2**. stdout: empty.

3. **Composition loop.** For each tool in `Roster` (in order):
   - Skip if `len(tool.ShellInit) == 0` (tool has no shell integration).
   - Build argv via `substituteShell(tool.ShellInit, shell)` (replaces every `"<shell>"` token with the user-supplied shell name).
   - Run `proc.Run(ctx, argv[0], argv[1:]...)` (capture transport).
   - On `proc.ErrNotFound` (binary not on PATH): skip silently — Constitution V (graceful degradation). Install-mechanism agnostic: any source-built or non-brew install of the tool participates as long as its binary is on PATH.
   - On any other error: write `shll shell-init: <tool>: <err>` to stderr, set `anyFailed = true`, **and skip this tool's stdout** (eval-safety — failing tool's partial output never reaches stdout). Continue with the next tool. **No separator** is emitted for this tool.
   - On success (the only success-write path): emit the shell-comment separator `# ── <tool> ──` followed by a newline (see [Shell-comment separator](#shell-comment-separator-change-y630)), then write the captured stdout bytes verbatim to the output writer.

4. **Final exit.** If `anyFailed`, return `errSilent` (exit 1). Else return nil (exit 0).

## Eval-safety invariants

This is the central correctness property of `shell-init` and is non-negotiable (Constitution V; Design Decision #6):

- **stdout MUST always be eval-safe.** No matter the error path:
  - Missing/unsupported shell → empty stdout, message on stderr.
  - Sub-tool not installed → silently omitted from output.
  - Sub-tool execution fails → its (partial) stdout is dropped; the error message goes to stderr only.
  - Sub-tool returns non-zero exit → falls under the "fails" branch (proc returns a non-nil err).
- **stderr is the only diagnostic channel.** Any human-readable text — usage, error notes — goes to stderr.
- **`shll` injects only eval-safe shell comments.** stdout consists of the bytes returned by successful sub-tools, concatenated, plus shll's own `# ── <tool> ──` comment separators (change y630). shll injects no executable shell code — a `#`-prefixed line is a shell no-op, so the comments are eval-safe and the invariant holds. A separator is emitted **only** on the success-write path, so stdout never contains a separator for a tool whose output is absent (see [Shell-comment separator](#shell-comment-separator-change-y630)).

This means `eval "$(shll shell-init zsh)"` is safe even when shll exits non-zero or sub-tools are broken — at worst the user gets a shell with one fewer integration loaded, never a parse error.

## Shell-comment separator (change y630)

`shll shell-init` frames each contributing tool's init block with a separator so a composed blob is no longer one undifferentiated wall of shell code. The separator is `# ── <tool> ──`, produced by `toolComment(name)` in the shared helper `src/cmd/shll/ui.go:67` and written (with a trailing newline) immediately before the tool's captured output on the success-write path of step 3.

```
# ── wt ──
...
# ── tu ──
export PATH=...
# ── hop ──
alias h='hop'
```

### The deliberate exception — DO NOT unify onto the `▸`/`==>` header

This is the single most important fact in this file for a future maintainer. `shell-init` uses a **shell comment**, **not** the `▸`/`==>` header that `update` and `install` use, and the difference is **intentional and load-bearing**:

- **No `▸`/`==>` header, no color, no TTY-gating.** `toolComment` takes no `color` parameter and never consults `colorEnabled`. The separator is emitted **identically** whether or not stdout is a terminal and regardless of `NO_COLOR` — always plain ASCII shell-comment text.
- **Why.** `shll shell-init` stdout is consumed by `eval "$(shll shell-init <shell>)"`. A bare `▸ <tool>` line would be eval'd as a command and break the user's shell; ANSI color escapes inside eval'd output would corrupt it. A `#`-prefixed line is a shell no-op — the only eval-safe separator here. This is mandated by **Constitution V (Graceful Degradation — `shll shell-init` output MUST always be eval-safe)**.
- **This is NOT an oversight.** A future "consistency" refactor that unifies `shell-init` onto the `▸`/`==>` header (or adds color/TTY-gating to it) **reintroduces the eval-break** and MUST NOT be done. The inconsistency with `update`/`install` is the correct, guarded design — recorded here so the guard survives.

### Separator emitted only when the tool's output reaches stdout

The separator is written **only** on the success-write path — the tool is installed (binary on PATH) **and** its `shell-init` did not error:

- **Not installed** (`proc.ErrNotFound`, skipped silently): no separator, no block — the `continue` happens before the separator write.
- **Errored** (any other error): its stdout is dropped to preserve eval-safety, the error note goes to stderr, and **no separator** is emitted — the `continue` happens before the separator write.

This preserves the eval-safety invariant exactly: stdout consists only of bytes from successful sub-tools, concatenated, now plus shll's own comment separators — never a dangling separator for a tool that produced nothing.

## Composition order

Output is concatenated in `Roster` order. This is deterministic (Spec: Composition Order). Today `wt`, `tu`, and `hop` produce output; in the leaves-first roster (`wt, idea, tu, rk, hop, fab-kit` — change auvj) the three integrators sit at indices `wt`@0, `tu`@2, `hop`@4, so `runShellInit` emits them in ascending-index order: **`wt` first, then `tu`, then `hop`**. This order is *intentional*, not incidental — it follows the toolkit's leaves-first dependency order (`wt` is a leaf that `hop` depends on at runtime; `hop open` delegates to wt's menu), so users reading a composed blob see a dependency before its dependent. `TestShellInit_DeterministicOrder` asserts byte-identical stdout across two consecutive runs. The roster-order invariant itself is guarded by `TestRosterLeavesBeforeDependents` — see [cli/commands](commands.md#design-decision-leaves-first-roster-order-change-auvj).

> The earlier framing here said the order was `tu, hop, wt` and that tu's position was "incidental". Both are superseded by change auvj: the order is now `wt, tu, hop` and it is a deliberate leaves-first sequencing decision. (Note: `wt, tu, hop`, NOT `tu, wt, hop` — the integrators in ascending `Roster` index order are wt@0, tu@2, hop@4.)

## Argv substitution

`substituteShell(argv, shell)` (`src/cmd/shll/shell_init.go:107`) replaces every literal `<shell>` token with `shell`, returning a fresh slice (does not mutate the roster):

| Tool | Roster argv | After substitution (zsh) |
|------|-------------|--------------------------|
| `wt`  | `["wt", "shell-init", "<shell>"]`  | `["wt", "shell-init", "zsh"]`  |
| `tu`  | `["tu", "shell-init", "<shell>"]`  | `["tu", "shell-init", "zsh"]`  |
| `hop` | `["hop", "shell-init", "<shell>"]` | `["hop", "shell-init", "zsh"]` |

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

- `TestShellInit_ZshAllIntegratorsInstalled` — `wt`, `tu`, and `hop` all installed → roster-ordered concatenation, each block preceded by its `# ── <tool> ──` separator (`# ── wt ──`, then wt's block, `# ── tu ──`, then tu's block, `# ── hop ──`, then hop's block), exit 0.
- `TestShellInit_OnlyTuInstalled` — only `tu` installed → `# ── tu ──` + tu's stdout only; no separator for the uninstalled tools, exit 0.
- `TestShellInit_OnlyHopInstalled` — only `hop` installed → `# ── hop ──` + hop's stdout only, exit 0.
- `TestShellInit_OnlyWtInstalled` — only `wt` installed → `# ── wt ──` + wt's stdout (using new `wt shell-init <shell>` argv), exit 0.
- `TestShellInit_NoIntegratingToolsInstalled` — none installed → empty stdout (no separators), exit 0.
- `TestShellInit_UnsupportedShell` — `fish` → empty stdout, stderr usage line, exit 2.
- `TestShellInit_MissingShellArg` — no arg → empty stdout, stderr usage line, exit 2.
- `TestShellInit_DeterministicOrder` — all three integrators installed → byte-identical output (separators included) across two runs, in roster order (`wt`, then `tu`, then `hop`).
- `TestShellInit_SubToolFailure` — one integrator (`hop shell-init zsh`, last in roster order) errors → its stdout fragment **and its `# ── hop ──` separator** are both dropped, the surviving two emit separator + block in `wt, tu` order, eval-safety holds, exit 1.

## Cross-references

- Roster definition and `<shell>` placeholder: [cli/commands](commands.md#hardcoded-tool-roster).
- Subprocess wrapper conventions: [internal/proc](../internal/proc.md) — including `proc.ErrNotFound` semantics.
- Brew detection (`isInstalled`) — used by `install` and `update` only, not here: [cli/update](update.md#detection).
- Rc-file installer: [cli/shell-install](shell-install.md) — wraps `shll shell-init <shell>` in an `eval` line and writes it to the user's rc file (idempotent install / `--print` dry-run / `--uninstall` removal).
- Shared UI helper (`ui.go`): [cli/commands](commands.md#file-layout-srccmdshll). `shell-init` consumes only `toolComment` from it — **not** the `▸`/`==>` header or color logic that `update`/`install` use (the [deliberate exception](#the-deliberate-exception--do-not-unify-onto-the--header)).
- Constitution V (Graceful Degradation) — tools not on PATH are omitted silently; eval-safety mandates the shell-comment separator over the `▸`/`==>` header.
