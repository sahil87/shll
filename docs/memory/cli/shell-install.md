# cli/shell-install

`shll shell-install [shell]` — appends a sentinel-wrapped `eval "$(shll shell-init <shell>)"` block to the user's shell rc file. Idempotent re-runs, optional `--print` (dry run) and `--uninstall` (removal) modes, plus `--rc-file` as a universal escape hatch for non-standard layouts.

Source: `src/cmd/shll/shell_install.go`. No `internal/proc` involvement — this command performs file I/O only and imports neither `internal/proc` nor `os/exec` (Constitution I scope is subprocess execution; A-039 is enforced by `TestNoProcImports`).

## Usage

```sh
shll shell-install                  # auto-detect shell from $SHELL, append block to derived rc
shll shell-install zsh              # explicit shell
shll shell-install --print zsh      # dry-run: print the block to stdout, no file change
shll shell-install --uninstall zsh  # remove the block from the rc file
shll shell-install --rc-file <path> # override rc-file derivation entirely
```

The block this command writes is the same line every README onboarding flow has historically asked the user to paste manually:

```
eval "$(shll shell-init zsh)"
```

That line is the cross-tool composition entry point — see [cli/shell-init](shell-init.md). `shell-install` exists so the user does not have to know which rc file to paste it into, nor remember to dedupe on re-install.

## Behavior contract

`runShellInstall(ctx, args, rcFileFlag, printMode, uninstallMode, stdout, stderr)` (`src/cmd/shll/shell_install.go:172`) is the implementation seam. The cobra `RunE` wrapper builds the writers and delegates. The dispatch sequence:

1. **Flag conflict.** If both `--print` and `--uninstall` are set → return `errExitCode{code: 2, msg: "shll shell-install: --print and --uninstall are mutually exclusive"}` (`shell_install.go:176`). Exit code: **2**.
2. **Resolve shell.** Delegate to `resolveShell(args, os.Getenv)` (`shell_install.go:78`).
3. **Resolve rc file.** If `--rc-file <path>` was passed, use it verbatim. Otherwise derive via `resolveRcFile(shell, os.Getenv)` (`shell_install.go:106`).
4. **Mode dispatch.** `--print` → `runShellInstallPrint`; `--uninstall` → `runShellInstallUninstall`; otherwise → `runShellInstallDefault`.

The `userProvidedPath bool` parameter to `runShellInstallDefault` (`shell_install.go:202`) is `true` exactly when `--rc-file` was supplied — it controls whether the missing-rc-file error includes the "shll won't create rc files" hint.

## Shell resolution

`resolveShell(args, env)` (`shell_install.go:78`):

| Input | Output |
|-------|--------|
| Positional `zsh` or `bash` | the positional |
| Positional any other value (e.g. `fish`) | `errExitCode{code:2, msg: "shll shell-install: unsupported shell \"<v>\". Supported: zsh, bash"}` |
| No positional, `$SHELL` basename ∈ `{zsh, bash}` | the inferred shell |
| No positional, `$SHELL` basename unsupported | `errExitCode{code:2, msg: "shll shell-install: cannot infer shell from $SHELL=<raw>. Pass shell explicitly: shll shell-install zsh"}` |

The basename is computed via `filepath.Base($SHELL)` (`shell_install.go:87`), so `/bin/zsh`, `/usr/bin/env zsh`, and `/usr/local/bin/zsh` all collapse to `zsh`. The supported-shell predicate (`isSupportedShell`) is the same one `shell-init` uses — both subcommands share the `supportedShells = {"zsh", "bash"}` constant defined in `shell_init.go`. The two unsupported-shell error messages are deliberately distinct so users get actionable feedback for the path they took (positional rejection vs. environment inference).

## Rc-file derivation

`resolveRcFile(shell, env)` (`shell_install.go:106`) implements the platform-aware default:

| Resolved shell | OS | Derived path |
|----------------|----|----|
| `zsh` | any | `${ZDOTDIR:-$HOME}/.zshrc` |
| `bash` | `osGoos == "darwin"` | `$HOME/.bash_profile` |
| `bash` | any other (`linux` etc.) | `$HOME/.bashrc` |

`osGoos` (`shell_install.go:20`) is a package-level variable initialized to `runtime.GOOS`. It is the only platform-specific code path in this command and is the abstraction surface required by Constitution: Cross-Platform Behavior. Tests swap it via `setOsGoos(t, value)` (`shell_install_test.go:15`) so darwin and linux defaults are both reachable from a single host. Because `osGoos` is package-private mutable state, `setOsGoos` saves+restores via `t.Cleanup` and tests that depend on it MUST NOT use `t.Parallel`.

The `--rc-file <path>` flag short-circuits derivation entirely: the supplied path is used verbatim, and `$ZDOTDIR` / `$HOME` are not consulted. This is the documented escape hatch for `$ZDOTDIR` users, dotfile managers writing to the source-of-truth file, and CI scripts that template the rc.

## Sentinel block format (exact)

`buildBlock(shell)` (`shell_install.go:132`) returns these exact bytes (no leading whitespace, single trailing `\n`):

```
# >>> shll shell-init >>>
eval "$(shll shell-init <shell>)"
# <<< shll shell-init <<<
```

The two sentinels are package-level constants (`shell_install.go:27-31`):

- `openSentinel = "# >>> shll shell-init >>>"` — including the spaces and the three `>` chars.
- `closeSentinel = "# <<< shll shell-init <<<"` — including the spaces and the three `<` chars.
- `evalLineFmt = `eval "$(shll shell-init %s)"`` — the body, with `%s` substituted by the resolved shell.

The block format is the single source of truth for install (write), idempotency (substring-match the open sentinel), `--print` (write to stdout), and `--uninstall` (locate + remove). Drift between paths is a defect — they all derive from the same three constants. The block carries no "managed by shll, do not edit" line; the bookend sentinels are themselves the visual signal.

## Idempotency invariant

The default install path checks `bytes.Contains(content, []byte(openSentinel))` (`shell_install.go:218`) before appending. If the open sentinel is already present, the file is left untouched, a stderr message `shll shell-install: already installed in <path> (no changes).` is emitted, and the command exits 0. Re-running `shll shell-install` against an installed rc file is a guaranteed no-op — the rc file is byte-identical before and after. `TestInstall_Idempotent` (`shell_install_test.go:195`) asserts this with byte-equality.

The check uses substring match on the open sentinel only (not a full block parse) — fast, robust to body changes, and the most natural fit for the only required outcome ("don't append twice").

## Symlink-preservation invariants

Two distinct write strategies, depending on whether the operation is read-modify-write:

### Install: plain `O_APPEND`

`runShellInstallDefault` opens the rc file with `os.OpenFile(rcPath, os.O_WRONLY|os.O_APPEND, 0)` (`shell_install.go:229`) — no `O_CREATE`, no perm bits (the file's existence was confirmed in step 3, and we never want to create files). Plain `O_APPEND` follows symlinks to the underlying real file and writes there, so a `~/.zshrc` symlink to `~/dotfiles/zshrc` (chezmoi, dotbot, stow, yadm) stays a symlink and the dotfile-manager source-of-truth file receives the appended block. Per POSIX, `write()` calls under `PIPE_BUF` (4 KiB on Linux, 512 bytes on macOS) are atomic with `O_APPEND`; the sentinel block is well under both limits. `TestInstall_PreservesSymlink` (`shell_install_test.go:285`) asserts the symlink stays a symlink (`os.Lstat` checks `os.ModeSymlink`).

### Uninstall: EvalSymlinks → `O_TRUNC`

`runShellInstallUninstall` (`shell_install.go:271`) cannot use `O_APPEND` because removal is read-modify-write. The chosen mitigation:

1. Read the full file content.
2. Locate the block via `findBlock` (`shell_install.go:144`) — open sentinel → close sentinel → optional trailing `\n`.
3. Slice the block out of the in-memory content.
4. Resolve the symlink chain: `resolved, _ := filepath.EvalSymlinks(rcPath)` (`shell_install.go:294`).
5. Truncate-write the modified content to the *resolved* real path: `os.OpenFile(resolved, os.O_WRONLY|os.O_TRUNC, 0)` (`shell_install.go:299`).

This preserves the user's symlink at the original path (it still points at the same real file) while the underlying source-of-truth file is updated. Going through `os.Rename` of a temp file would replace the symlink with a regular file — the same hazard the install path avoids, repeated in the removal path. `TestUninstall_PreservesSymlink` (`shell_install_test.go:429`) asserts the symlink stays a symlink and the real file's block is removed.

## "shll never creates rc files" invariant

The default-install and `--print` paths both `os.Stat` the rc file and return `errExitCode{code:2, ...}` when it does not exist. They never call `O_CREATE`. The error message branches on whether the user passed `--rc-file`:

- Without `--rc-file`: `shll shell-install: <path> does not exist. shll won't create rc files. Create it first, or pass --rc-file <path>.` (`shell_install.go:208`)
- With `--rc-file`: `shll shell-install: <path> does not exist.` (`shell_install.go:206`) — no boilerplate, since the user explicitly named the path.

A missing rc file is a meaningful signal — custom `$ZDOTDIR`, dotfile manager pending `apply`, non-standard layout — and creating it would mask real configuration issues. The `--uninstall` path treats a missing rc file as benign ("nothing to uninstall", exit 0, stderr-only message at `shell_install.go:275`).

## Trailing-newline guard

`runShellInstallDefault` (`shell_install.go:226`) prepends `\n` to the block exactly when the existing content is non-empty AND its last byte is not `\n`:

```go
if len(content) > 0 && content[len(content)-1] != '\n' {
    block = append([]byte("\n"), block...)
}
```

This prevents the open sentinel from landing on the same line as the user's previous content (e.g. `export FOO=bar# >>> shll shell-init >>>`). Empty files require no leading `\n` — a stray blank line at the top of an otherwise empty rc file would be visible noise. `TestInstall_TrailingNewlineGuard` and `TestInstall_EmptyFileNoLeadingNewline` (`shell_install_test.go:211`, `:228`) pin both branches.

## Exit-code policy

Mirrors the convention `shll shell-init` already established — see [cli/commands](commands.md#exit-code-translation). Both `errSilent` and `errExitCode` from `main.go` are reused; no new sentinel types are introduced.

| Exit code | Conditions |
|-----------|------------|
| **0** | Block appended; idempotency no-op; `--print` succeeded; `--uninstall` removed block; `--uninstall` no-op when block or file absent |
| **1** | I/O failure (read, write, close, `EvalSymlinks` during `--uninstall`) — emitted via `errSilent` after the diagnostic is written to stderr by the subcommand |
| **2** | User-invocation error — missing/unsupported shell positional, `$SHELL` could not be inferred, rc file does not exist in default or `--print` mode, `--print` and `--uninstall` both supplied — emitted via `errExitCode{code: 2, msg: ...}` |

`translateExit` in `main.go` writes the `errExitCode.msg` to stderr automatically; subcommand code does not echo it. For `errSilent`, the subcommand has already written its own diagnostic via `fmt.Fprintf(stderr, ...)` and `translateExit` adds nothing.

## Test seam

`shell_install_test.go` (test-alongside, per `code-quality.md` `## Test Strategy`):

- **No `proc.Runner` fake.** This subcommand does not execute subprocesses, so the `installFakeRunner` pattern used by `update_test.go` / `shell_init_test.go` / `version_test.go` is not needed. Tests drive `runShellInstallCmd(t, argv)` (`shell_install_test.go:30`) which builds a fresh cobra command with `bytes.Buffer` writers and returns `(stdout, stderr, err)`.
- **`t.TempDir()`** for every rc-file test — the user's real `~/.zshrc` / `~/.bashrc` / `~/.bash_profile` is never touched.
- **`osGoos` swap** via `setOsGoos(t, value)` (`shell_install_test.go:15`) for the macOS-vs-Linux bash defaults. Saves and restores the package-level variable through `t.Cleanup`.
- **`envFunc(map)`** (`shell_install_test.go:24`) — unit tests for `resolveShell` / `resolveRcFile` use a map-backed env lookup so they run without mutating process state.
- **`t.Setenv`** for end-to-end tests that go through the real cobra command (e.g. `TestInstall_ErrorsWhenRcMissingNoFlag` sets `HOME`/`ZDOTDIR`/`SHELL` to drive derivation).

Source-level guard: `TestNoProcImports` (`shell_install_test.go:493`) reads `shell_install.go` as bytes and fails if the source contains `internal/proc` or `"os/exec"`. This is a defensive check protecting Constitution I scoping — any future regression that pulls in subprocess execution will fail at test time.

## Cross-references

- Subcommand registration and exit-code translation: [cli/commands](commands.md).
- The eval-line target: [cli/shell-init](shell-init.md) — `shell-install` writes the line that `shell-init` produces output for.
- Constitution I (Security First) → does not apply to this command (no subprocess execution); the `TestNoProcImports` guard documents the boundary.
- Constitution V (Graceful Degradation) → divergent here. `shell-init` silently omits missing tools; `shell-install` is user-invoked and surfaces errors loudly, since the user wants feedback on rc-file editing.
- Constitution VII (Minimal Surface Area) → justification recorded in `spec.md` Requirement: New top-level subcommand.
- Cross-Platform Behavior → the darwin-vs-other branch in `resolveRcFile` is the only platform-specific code path, isolated behind the `osGoos` package-level variable.
