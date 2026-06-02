# Plan: Add `shll shell-install` rc-file installer

**Change**: 260510-vul4-shell-install-rc
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- **`fish` shell support** — fish convention is a drop-in `~/.config/fish/conf.d/shll.fish` file rather than editing `config.fish`; that is a separate design and out of scope here.
- **Brew `postinstall` hook that auto-runs `shell-install`** — explicitly rejected; brew formulas mutating user dotfiles is an anti-pattern. Installation MUST always be an explicit user command.
- **Auto-creating the rc file when absent** — a missing `~/.zshrc` is a meaningful signal (custom `$ZDOTDIR`, dotfile-manager not yet applied). Creating it would mask the user's real configuration.
- **Atomic `tmp + rename` write strategy** — replaces symlinks with regular files, silently breaking dotfile managers (chezmoi, dotbot, stow, yadm).
- **Making other sahil87 tools brew dependencies of `shll`** — collapses the per-tool opt-in story (Constitution IV) and creates uninstall friction.
- **Coupling `shll update` to rc-file knowledge** — e.g. warning when a third-party edited the rc file. Out of scope; would violate subcommand independence.
- **Multiple-block support** — only one shll-managed sentinel block is recognized per rc file. Idempotency assumes a single block; out of scope to manage multiple.

## CLI: `shll shell-install` Subcommand

### Requirement: New top-level subcommand

`shll` SHALL expose a new top-level subcommand `shell-install` wired into the cobra root via `newShellInstallCmd()`. The subcommand SHALL be added to `newRootCmd().AddCommand(...)` in `src/cmd/shll/root.go` so the v0.1.0 surface becomes exactly four subcommands (`update`, `shell-init`, `shell-install`, `version`). Per Constitution VII (Minimal Surface Area), this addition is justified because:

- It cannot be a flag on `shell-init` — `shell-install` *invokes* `shell-init`, so making it a sub-flag is structurally self-referential.
- It cannot live in a per-tool CLI — per-tool CLIs emit their own shell-init; `shell-install` writes the cross-tool composition `eval "$(shll shell-init <shell>)"`. Cross-tool composition is exactly what `shll` exists for.

The cobra command SHALL set `SilenceUsage: true` and `SilenceErrors: true`, mirroring sibling subcommands. The subcommand factory SHALL accept zero or one positional argument (`cobra.MaximumNArgs(1)`).

#### Scenario: Subcommand registered on the root
- **GIVEN** the user runs `shll --help`
- **WHEN** the help output is rendered
- **THEN** `shell-install` MUST appear in the subcommand list alongside `update`, `shell-init`, and `version`

#### Scenario: Constitution VII justification recorded in spec
- **GIVEN** a reviewer reads this spec
- **WHEN** they look for justification of the new subcommand
- **THEN** the rationale (cannot fold into `shell-init`; cannot live in a per-tool CLI) MUST be present

### Requirement: Shell resolution

When invoked without a positional argument, `shll shell-install` SHALL infer the shell from `$SHELL`:

1. Read `$SHELL` from the environment.
2. Take its basename via `filepath.Base`.
3. If the basename matches a member of `supportedShells = []string{"zsh", "bash"}`, use it as the resolved shell.
4. Otherwise, return an `errExitCode{code: 2, msg: ...}` with the message `shll shell-install: cannot infer shell from $SHELL=<value>. Pass shell explicitly: shll shell-install zsh`.

When invoked with a positional argument, that argument SHALL be used directly. If the positional is not a member of `supportedShells`, the command MUST exit with code 2 and a message of the form `shll shell-install: unsupported shell "<value>". Supported: zsh, bash`.

The resolved shell name SHALL be reused (a) in the eval line body of the sentinel block, and (b) for rc-file derivation.

#### Scenario: Inferring shell from $SHELL
- **GIVEN** `$SHELL=/bin/zsh` and no positional argument
- **WHEN** the user runs `shll shell-install`
- **THEN** the resolved shell MUST be `zsh`

#### Scenario: Explicit positional overrides $SHELL
- **GIVEN** `$SHELL=/bin/zsh` and the user runs `shll shell-install bash`
- **WHEN** the command resolves the shell
- **THEN** the resolved shell MUST be `bash`

#### Scenario: Unsupported $SHELL with no positional
- **GIVEN** `$SHELL=/usr/local/bin/fish` and no positional argument
- **WHEN** the user runs `shll shell-install`
- **THEN** the command MUST exit with code 2
- **AND** stderr MUST mention the inferred shell name and suggest passing the shell explicitly

#### Scenario: Unsupported positional argument
- **GIVEN** the user runs `shll shell-install fish`
- **WHEN** argument validation runs
- **THEN** the command MUST exit with code 2
- **AND** stderr MUST contain `Supported: zsh, bash`

### Requirement: Rc-file path derivation

When `--rc-file` is not provided, the rc-file path SHALL be derived from the resolved shell and the host operating system:

| Resolved shell | Operating system | Derived path |
|----------------|------------------|--------------|
| `zsh` | any | `${ZDOTDIR:-$HOME}/.zshrc` |
| `bash` | `runtime.GOOS == "darwin"` | `$HOME/.bash_profile` |
| `bash` | any other | `$HOME/.bashrc` |

The `darwin` vs. non-`darwin` branch for bash SHALL be the only platform-specific code path, isolated inside `resolveRcFile(shell)` per Constitution: Cross-Platform Behavior.

When `--rc-file <path>` is supplied, derivation SHALL be skipped and the supplied path used verbatim.

#### Scenario: zsh with $ZDOTDIR set
- **GIVEN** `$ZDOTDIR=/home/u/dotfiles/zsh` and the resolved shell is `zsh`
- **WHEN** the rc-file path is derived
- **THEN** the derived path MUST be `/home/u/dotfiles/zsh/.zshrc`

#### Scenario: zsh with $ZDOTDIR unset
- **GIVEN** `$ZDOTDIR` is unset and `$HOME=/home/u`
- **WHEN** the resolved shell is `zsh`
- **THEN** the derived path MUST be `/home/u/.zshrc`

#### Scenario: bash on Linux
- **GIVEN** `runtime.GOOS == "linux"` and `$HOME=/home/u`
- **WHEN** the resolved shell is `bash`
- **THEN** the derived path MUST be `/home/u/.bashrc`

#### Scenario: bash on macOS
- **GIVEN** `runtime.GOOS == "darwin"` and `$HOME=/Users/u`
- **WHEN** the resolved shell is `bash`
- **THEN** the derived path MUST be `/Users/u/.bash_profile`

#### Scenario: --rc-file overrides derivation
- **GIVEN** the user runs `shll shell-install --rc-file /tmp/custom-rc zsh`
- **WHEN** the rc-file path is resolved
- **THEN** the path MUST be `/tmp/custom-rc`
- **AND** `$ZDOTDIR` and `$HOME` MUST NOT be consulted

### Requirement: Sentinel block format (exact)

The sentinel-wrapped block written, matched, and removed by `shll shell-install` SHALL be exactly three lines, in this order:

```
# >>> shll shell-init >>>
eval "$(shll shell-init <shell>)"
# <<< shll shell-init <<<
```

Where `<shell>` is the resolved shell name (`zsh` or `bash`). The block SHALL terminate with a single `\n` after the close sentinel.

- **Open sentinel** (literal): `# >>> shll shell-init >>>` — including the spaces and the three `>` characters.
- **Close sentinel** (literal): `# <<< shll shell-init <<<` — including the spaces and the three `<` characters.
- **Body**: exactly one line, `eval "$(shll shell-init <shell>)"` (with `<shell>` substituted).

The block SHALL NOT include a "managed by shll, do not edit" warning line — the sentinels themselves are visually distinctive.

The format SHALL be identical across install, idempotency-check, `--print`, and `--uninstall` paths. Drift between paths is a defect.

#### Scenario: Block matches the spec verbatim
- **GIVEN** any successful install for shell `zsh`
- **WHEN** the user inspects the rc file
- **THEN** the appended text MUST be exactly:
  ```
  # >>> shll shell-init >>>
  eval "$(shll shell-init zsh)"
  # <<< shll shell-init <<<
  ```
  followed by a single `\n`

#### Scenario: Block body uses resolved shell
- **GIVEN** the resolved shell is `bash`
- **WHEN** the block is built
- **THEN** the body line MUST be `eval "$(shll shell-init bash)"`

### Requirement: Default install mode

In default mode (no `--print`, no `--uninstall`), `shll shell-install` SHALL execute the following sequence:

1. Resolve shell (per the shell-resolution requirement).
2. Resolve rc-file path (per the path-derivation requirement).
3. Stat the rc file. If it does not exist:
   - Without `--rc-file`: return `errExitCode{code: 2, msg: "shll shell-install: <path> does not exist. shll won't create rc files. Create it first, or pass --rc-file <path>."}`.
   - With `--rc-file`: return `errExitCode{code: 2, msg: "shll shell-install: <path> does not exist."}` (no "shll won't create rc files" hint, since the user named the path).
4. Read the file's full content. If unreadable due to permissions or other I/O failure, write a diagnostic to stderr and return `errSilent` (exit 1).
5. Idempotency check: search the content for the open sentinel `# >>> shll shell-init >>>`. If present, write `shll shell-install: already installed in <path> (no changes).` to stderr and return nil (exit 0). No file modification SHALL occur.
6. Build the block (per the sentinel-format requirement).
7. Trailing-newline guard: if the existing file content is non-empty AND the last byte is not `\n`, prepend `\n` to the block before writing. An empty file requires no leading `\n`.
8. Append the block via `os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)` (no `O_CREATE`, no perm bits — step 3 already guaranteed the file exists). On write failure, surface the OS error to stderr and return `errSilent` (exit 1).
9. Write to stdout: `Installed shll shell integration to <path>. Restart your shell or run: source <path>` and return nil (exit 0).

The append SHALL use plain `O_APPEND` (no `tmp + rename` strategy). Per POSIX, `write()` calls under `PIPE_BUF` (4 KiB on Linux, 512 bytes on macOS) are atomic with `O_APPEND`; the sentinel block is well under both limits. This preserves symlink behavior for dotfile-manager users.

#### Scenario: Install when rc file exists, sentinels absent
- **GIVEN** `~/.zshrc` exists, ends with `\n`, and contains no sentinel
- **WHEN** the user runs `shll shell-install zsh`
- **THEN** the block MUST be appended verbatim to the file
- **AND** the command MUST exit 0
- **AND** stdout MUST contain `Installed shll shell integration to <path>` and the source/restart hint

#### Scenario: Install is idempotent
- **GIVEN** `~/.zshrc` already contains the sentinel block
- **WHEN** the user runs `shll shell-install zsh` a second time
- **THEN** the file MUST NOT be modified
- **AND** the command MUST exit 0
- **AND** stderr MUST contain `already installed in <path>`

#### Scenario: Trailing-newline guard prepends \n
- **GIVEN** `~/.zshrc` exists, contains `export FOO=bar` with no trailing newline
- **WHEN** the user runs `shll shell-install zsh`
- **THEN** the resulting file MUST contain `export FOO=bar\n# >>> shll shell-init >>>\n...`
- **AND** the open sentinel MUST NOT share a line with `export FOO=bar`

#### Scenario: Install errors when rc file missing (no --rc-file)
- **GIVEN** `~/.zshrc` does not exist and `$SHELL=/bin/zsh`
- **WHEN** the user runs `shll shell-install`
- **THEN** the command MUST exit 2
- **AND** stderr MUST mention the path AND state `shll won't create rc files`
- **AND** stderr MUST suggest `--rc-file <path>`

#### Scenario: Install errors when rc file missing (with --rc-file)
- **GIVEN** the user runs `shll shell-install --rc-file /tmp/missing-rc zsh` and `/tmp/missing-rc` does not exist
- **WHEN** the existence check runs
- **THEN** the command MUST exit 2
- **AND** stderr MUST mention `/tmp/missing-rc does not exist`
- **AND** stderr MUST NOT include the `shll won't create rc files` boilerplate (the user explicitly named the path)

#### Scenario: Symlinked rc file preserved on install
- **GIVEN** `~/.zshrc` is a symlink to `~/dotfiles/zshrc`
- **WHEN** the user runs `shll shell-install zsh`
- **THEN** the block MUST be appended through the symlink to `~/dotfiles/zshrc`
- **AND** `~/.zshrc` MUST still be a symlink after the operation
- **AND** `~/dotfiles/zshrc` MUST contain the appended block

### Requirement: `--print` mode

When invoked with `--print`, `shll shell-install` SHALL:

1. Resolve shell (positional accepted; falls back to `$SHELL`).
2. Resolve rc-file path the same way as default mode (derivation or `--rc-file`).
3. Stat the rc file; if it does not exist, error per the default-mode rules (`--print` does not silently bypass the missing-rc-file check, because the user may be debugging that exact problem).
4. Skip the idempotency search, the trailing-newline guard, and the write entirely.
5. Print the exact block to stdout (the same three-line block default mode would write), with no surrounding informational messages.
6. Return nil (exit 0).

#### Scenario: --print emits exact block to stdout
- **GIVEN** `~/.zshrc` exists and the user runs `shll shell-install --print zsh`
- **WHEN** the command runs
- **THEN** stdout MUST equal:
  ```
  # >>> shll shell-init >>>
  eval "$(shll shell-init zsh)"
  # <<< shll shell-init <<<
  ```
  (followed by a single `\n`)
- **AND** the rc file MUST NOT be modified
- **AND** the command MUST exit 0

#### Scenario: --print accepts shell positional
- **GIVEN** `$SHELL=/bin/zsh` and the user runs `shll shell-install --print bash`
- **WHEN** the block is built
- **THEN** the body line MUST be `eval "$(shll shell-init bash)"`

#### Scenario: --print still errors when rc file missing
- **GIVEN** `~/.zshrc` does not exist
- **WHEN** the user runs `shll shell-install --print zsh`
- **THEN** the command MUST exit 2
- **AND** stderr MUST mention the missing file

### Requirement: `--uninstall` mode

When invoked with `--uninstall`, `shll shell-install` SHALL:

1. Resolve shell and rc-file path (same as default mode).
2. If the rc file does not exist, write `shll shell-install: <path> does not exist (nothing to uninstall).` to stderr and return nil (exit 0). Missing rc file MUST NOT be an error in `--uninstall` mode.
3. Read the full content.
4. Search for a block bounded inclusively by `# >>> shll shell-init >>>` and `# <<< shll shell-init <<<`. If absent, write `shll shell-install: not installed in <path> (nothing to uninstall).` to stderr and return nil (exit 0).
5. Remove the block, including the trailing `\n` that the install path produced after the close sentinel. Surrounding content (lines before and after) MUST be preserved byte-for-byte.
6. Resolve the symlink chain via `filepath.EvalSymlinks(path)` to obtain the real underlying file. Open the *resolved* path with `os.O_WRONLY|os.O_TRUNC` and write the modified content. This preserves the user's symlink at the original path while updating the dotfile-manager source-of-truth file.
7. Write `Removed shll shell integration from <path>.` to stdout and return nil (exit 0).

The `--uninstall` write path SHALL NOT use `os.Rename` or any `tmp + rename` strategy.

`--uninstall` and `--print` MUST NOT be combined — when both flags are set, the command MUST exit 2 with a message indicating the flags are mutually exclusive.

#### Scenario: --uninstall removes the block when present
- **GIVEN** `~/.zshrc` contains `export FOO=bar\n` followed by the sentinel block followed by `export BAR=baz\n`
- **WHEN** the user runs `shll shell-install --uninstall zsh`
- **THEN** the resulting file MUST contain `export FOO=bar\nexport BAR=baz\n` (the sentinel block and its trailing newline removed; surrounding content preserved)
- **AND** the command MUST exit 0
- **AND** stdout MUST contain `Removed shll shell integration from <path>`

#### Scenario: --uninstall when block absent
- **GIVEN** `~/.zshrc` exists but contains no sentinel block
- **WHEN** the user runs `shll shell-install --uninstall zsh`
- **THEN** the file MUST NOT be modified
- **AND** the command MUST exit 0
- **AND** stderr MUST contain `not installed in <path>`

#### Scenario: --uninstall when rc file absent
- **GIVEN** `~/.zshrc` does not exist
- **WHEN** the user runs `shll shell-install --uninstall zsh`
- **THEN** the command MUST exit 0
- **AND** stderr MUST contain `does not exist (nothing to uninstall)`

#### Scenario: --uninstall preserves symlink chain
- **GIVEN** `~/.zshrc` is a symlink to `~/dotfiles/zshrc`, and `~/dotfiles/zshrc` contains the sentinel block
- **WHEN** the user runs `shll shell-install --uninstall zsh`
- **THEN** `~/.zshrc` MUST still be a symlink to `~/dotfiles/zshrc` after the operation
- **AND** `~/dotfiles/zshrc` MUST have the block removed
- **AND** the command MUST exit 0

#### Scenario: --print and --uninstall are mutually exclusive
- **GIVEN** the user runs `shll shell-install --print --uninstall zsh`
- **WHEN** flag validation runs
- **THEN** the command MUST exit 2
- **AND** stderr MUST indicate the flags are mutually exclusive

### Requirement: Exit-code policy

`shll shell-install` SHALL use the existing `translateExit` policy in `src/cmd/shll/main.go` with these mappings:

| Exit code | Conditions |
|-----------|------------|
| 0 | Success (block appended; idempotency no-op; `--print` succeeded; `--uninstall` removed block; `--uninstall` no-op when block or file absent) |
| 1 | I/O failure (read error, write error, symlink-eval error during `--uninstall`) — emitted via `errSilent` after the diagnostic is written to stderr by the subcommand |
| 2 | User-invocation error (missing/unsupported shell positional; `$SHELL` could not be inferred to a supported shell; rc file does not exist in default or `--print` mode; `--print` and `--uninstall` both supplied) — emitted via `errExitCode{code: 2, msg: ...}` |

This policy SHALL match the convention already established by `shll shell-init` (exit 2 for user-invocation errors, exit 1 for runtime/I-O failures). No new sentinel types SHALL be introduced; the existing `errSilent` and `errExitCode` cover all cases.

#### Scenario: Exit 2 reserved for user-invocation errors
- **GIVEN** the user runs `shll shell-install fish`
- **WHEN** argument validation rejects the shell
- **THEN** the process MUST exit with code 2

#### Scenario: Exit 1 reserved for I/O failures
- **GIVEN** `~/.zshrc` exists but the calling user lacks read permission
- **WHEN** `shll shell-install zsh` attempts to read it
- **THEN** the process MUST exit with code 1
- **AND** stderr MUST contain a diagnostic describing the I/O error

### Requirement: No subprocess execution

`shll shell-install` SHALL NOT invoke any subprocess. Constitution Principle I (Security First) governs subprocess execution; this command performs file I/O only and therefore does not interact with `internal/proc`. Implementation SHALL use only the Go standard library packages `os`, `path/filepath`, and `runtime`.

#### Scenario: No proc imports in shell_install.go
- **GIVEN** the implementation file `src/cmd/shll/shell_install.go`
- **WHEN** a reviewer inspects its imports
- **THEN** it MUST NOT import `github.com/sahil87/shll/internal/proc`
- **AND** it MUST NOT import `os/exec`

### Requirement: Test seam

A test file `src/cmd/shll/shell_install_test.go` SHALL accompany the implementation per the project test-alongside convention. Tests SHALL operate on temporary files (e.g., `t.TempDir()`) to avoid touching the host's real rc files, and SHALL use buffered `io.Writer` arguments for stdout/stderr assertions, mirroring the test-seam pattern established by `update_test.go` and `shell_init_test.go`.

For tests of shell inference and rc-file derivation, environment variables (`$SHELL`, `$HOME`, `$ZDOTDIR`) SHALL be controlled via `t.Setenv` rather than mutating the parent process state.

For tests of macOS bash vs. Linux bash defaults, `runtime.GOOS` SHALL be abstracted behind a small package-level variable (e.g. `osGoos = runtime.GOOS`) so tests can override it without resorting to build tags.

#### Scenario: Tests do not touch host rc files
- **GIVEN** the `shell_install_test.go` test file
- **WHEN** any test executes
- **THEN** every file path it writes MUST be rooted under `t.TempDir()`
- **AND** the user's real `~/.zshrc` / `~/.bashrc` / `~/.bash_profile` MUST NOT be read or modified

#### Scenario: macOS vs. Linux bash defaults are testable
- **GIVEN** the test for "macOS bash → ~/.bash_profile"
- **WHEN** the test sets the GOOS abstraction to `darwin`
- **THEN** the derived path MUST be `<HOME>/.bash_profile`
- **AND** when set to `linux`, the derived path MUST be `<HOME>/.bashrc`

## CLI: README updates

### Requirement: README install section

`README.md` SHALL be updated so the install section recommends `shll shell-install` as the primary post-`brew install` step:

- The `brew install sahil87/tap/shll` block SHALL be followed by a `shll shell-install` one-liner shown as the recommended path.
- The manual `eval "$(shll shell-init zsh)"` instruction SHALL remain in the README as the documented manual fallback for users who prefer to edit the rc file by hand.

#### Scenario: README shows shell-install as the primary step
- **GIVEN** a new user reads the install section of `README.md`
- **WHEN** they look for the next step after `brew install`
- **THEN** they MUST see `shll shell-install` (with a brief comment) before the manual `eval` line
- **AND** the manual `eval` line MUST still be present as a fallback

## Design Decisions

1. **Plain `O_APPEND` for install (not atomic `tmp + rename`)**
   - *Why*: Preserves symlinks for dotfile-manager users (chezmoi, dotbot, stow, yadm). The sentinel block is well under POSIX `PIPE_BUF` (4 KiB Linux / 512 bytes macOS), so `O_APPEND` writes are atomic for our payload. The user explicitly chose this trade-off in clarify.
   - *Rejected*: `tmp + rename` — silently replaces a symlink with a regular file, diverging the user's source-of-truth from what their shell loads.

2. **Error rather than auto-create when rc file is absent**
   - *Why*: A missing rc file is a meaningful signal — custom `$ZDOTDIR`, dotfile manager not yet applied, non-standard layout. Creating one masks real configuration issues.
   - *Rejected*: auto-creating with mode 0644 — appears helpful but hides the actual problem and produces a file the user did not author.

3. **Resolve symlink before truncate in `--uninstall`**
   - *Why*: `--uninstall` must read-modify-write the file content, which `O_APPEND` cannot do. Resolving via `filepath.EvalSymlinks` and writing through the resolved path keeps the user's symlink chain intact while updating the dotfile-manager source.
   - *Rejected*: `os.Rename` of a temp file over the rc path — replaces the symlink with a regular file (same hazard as install case).

4. **Sentinel-wrapped block (not a single comment line)**
   - *Why*: Idempotency requires a deterministic match target; `--uninstall` requires a clean removal target. Bookend sentinels (`>>>` / `<<<`) survive future format changes inside the block and are visually distinct from typical shell comments.
   - *Rejected*: single-line markers — fragile when block content evolves.

5. **No brew `postinstall` hook**
   - *Why*: Brew formulas that mutate user dotfiles have a poor reputation; rc-file installation MUST always be an explicit user command.
   - *Rejected*: auto-running `shell-install` in formula `post_install` — sets a precedent users (rightfully) distrust.

6. **No `fish` support in this change**
   - *Why*: fish convention is a drop-in `~/.config/fish/conf.d/shll.fish` file; that is a different installation model and merits its own design pass when added.
   - *Rejected*: minimal fish support that edits `config.fish` — diverges from fish community conventions.

7. **`--print` and `--uninstall` are mutually exclusive flags**
   - *Why*: They describe incompatible operations. Allowing both creates an ambiguous user contract; rejecting at flag-parse time is the simplest, clearest behavior.
   - *Rejected*: silently letting the latter flag win — surprising and undocumentable.

## Tasks

<!-- Sequential work items for the apply stage. Checked off [x] as completed. -->

### Phase 1: Setup

- [x] T001 Create `src/cmd/shll/shell_install.go` skeleton — package `main`, imports (`bytes`, `errors`, `fmt`, `io`, `os`, `path/filepath`, `runtime`, `strings`, `github.com/spf13/cobra`), package-level `osGoos = runtime.GOOS` (override seam for tests), constants for the open/close sentinels and the eval-line format string.

### Phase 2: Core Implementation

- [x] T002 In `src/cmd/shll/shell_install.go`, add `newShellInstallCmd()` — cobra command factory with `Use: "shell-install [shell]"`, `SilenceUsage`/`SilenceErrors` true, `Args: cobra.MaximumNArgs(1)`, three flags (`--print` bool, `--uninstall` bool, `--rc-file` string), `RunE` that dispatches to `runShellInstall` with shell positional, flag values, ctx, stdout, stderr.
- [x] T003 In `src/cmd/shll/shell_install.go`, add `resolveShell(args []string, env func(string)string) (string, error)` — positional override wins; otherwise infer from `$SHELL` basename via `filepath.Base`; reject unsupported with `errExitCode{code:2,...}`. Reuse `supportedShells` and `isSupportedShell` from `shell_init.go`. Distinct error messages per spec for "unsupported positional" vs "cannot infer from $SHELL".
- [x] T004 In `src/cmd/shll/shell_install.go`, add `resolveRcFile(shell string, env func(string)string) string` — implements the spec table (`zsh` → `${ZDOTDIR:-$HOME}/.zshrc`; `bash` darwin → `$HOME/.bash_profile`; `bash` other → `$HOME/.bashrc`). Branch on `osGoos` package-level variable.
- [x] T005 In `src/cmd/shll/shell_install.go`, add `buildBlock(shell string) []byte` — returns the exact three-line sentinel block ending in a single `\n`, body line `eval "$(shll shell-init <shell>)"`.
- [x] T006 In `src/cmd/shll/shell_install.go`, add `findBlock(content []byte) (start, end int, ok bool)` — locates the inclusive byte range of the sentinel block from open sentinel to close sentinel + the trailing `\n` if present. Used by both idempotency check and `--uninstall`.
- [x] T007 In `src/cmd/shll/shell_install.go`, implement `runShellInstall(ctx, shell, rcFileFlag, printMode, uninstallMode, stdout, stderr)` dispatch — flag conflict check (`--print` AND `--uninstall` → exit 2); resolve shell; resolve rc file (use `--rc-file` verbatim if provided, else `resolveRcFile`); fan out to `runShellInstallDefault`, `runShellInstallPrint`, `runShellInstallUninstall`.
- [x] T008 In `src/cmd/shll/shell_install.go`, implement default-install path — stat → 2-on-missing (with/without `--rc-file` distinct messages); read content; idempotency (open-sentinel substring → exit 0 with stderr message); trailing-newline guard (only when content non-empty AND last byte != `\n`); `os.OpenFile(path, O_WRONLY|O_APPEND, 0)`; write block; close; success message to stdout.
- [x] T009 In `src/cmd/shll/shell_install.go`, implement `--print` path — resolve shell + rc file; stat (still error 2 on missing); skip read/idempotency/guard; write `buildBlock(shell)` to stdout; return nil.
- [x] T010 In `src/cmd/shll/shell_install.go`, implement `--uninstall` path — stat (missing → exit 0 with "nothing to uninstall"); read content; `findBlock` (absent → exit 0 with "not installed"); slice block out; `filepath.EvalSymlinks` on rc path → resolved real path; `os.OpenFile(resolved, O_WRONLY|O_TRUNC, 0)` and write modified content; success message to stdout.
- [x] T011 In `src/cmd/shll/root.go`, add `newShellInstallCmd()` to the `AddCommand` list and update the rootLong subcommand listing to include `shll shell-install`.

### Phase 3: Tests

- [x] T012 Create `src/cmd/shll/shell_install_test.go` — boilerplate (package main, imports), helper `setOsGoos(t, value)` that swaps `osGoos` and restores via `t.Cleanup`, helper `runCmd(t, args, env)` that builds the cobra command, sets bytes.Buffer stdout/stderr, sets HOME/ZDOTDIR/SHELL via `t.Setenv`, runs and returns (stdout, stderr, err).
- [x] T013 In `shell_install_test.go`, add unit tests for `resolveShell` — positional `zsh`, positional `bash`, positional `fish` (errExitCode{2}), no positional with `$SHELL=/bin/zsh` (returns `zsh`), no positional with `$SHELL=/usr/local/bin/fish` (errExitCode{2} and message mentions inferred shell).
- [x] T014 In `shell_install_test.go`, add unit tests for `resolveRcFile` — `zsh` with `$ZDOTDIR=/home/u/dotfiles/zsh` → `/home/u/dotfiles/zsh/.zshrc`; `zsh` no `$ZDOTDIR` with `$HOME=/home/u` → `/home/u/.zshrc`; `bash` darwin (osGoos=darwin) with `$HOME=/Users/u` → `/Users/u/.bash_profile`; `bash` linux (osGoos=linux) with `$HOME=/home/u` → `/home/u/.bashrc`.
- [x] T015 In `shell_install_test.go`, add unit test for `buildBlock` — body for `zsh` and for `bash` is exact and the trailing `\n` is present.
- [x] T016 In `shell_install_test.go`, add scenario test "Install when rc file exists, sentinels absent" — `t.TempDir()` rc file with `export FOO=bar\n`, run `shll shell-install --rc-file <path> zsh`, assert file ends with the block, exit 0, stdout contains `Installed shll shell integration to <path>`.
- [x] T017 In `shell_install_test.go`, add scenario test "Install is idempotent" — pre-populate rc with the block, run install, assert file unchanged, exit 0, stderr contains `already installed`.
- [x] T018 In `shell_install_test.go`, add scenario test "Trailing-newline guard" — rc file content `export FOO=bar` (no trailing `\n`), run install, assert file content is `export FOO=bar\n# >>> shll shell-init >>>\n...` and the open sentinel is on its own line.
- [x] T019 In `shell_install_test.go`, add scenario test "Install errors when rc file missing (no --rc-file)" — set `$HOME=t.TempDir()` (no `.zshrc`), `$SHELL=/bin/zsh`, run `shll shell-install`, assert exit code 2, stderr contains the path AND `shll won't create rc files` AND `--rc-file`.
- [x] T020 In `shell_install_test.go`, add scenario test "Install errors when rc file missing (with --rc-file)" — `--rc-file /tmp/missing-..., zsh`, assert exit code 2, stderr contains `does not exist` and does NOT include `shll won't create rc files`.
- [x] T021 In `shell_install_test.go`, add scenario test "Symlinked rc file preserved on install" — create real `dotfiles/zshrc`, symlink `home/.zshrc` → real, run `shll shell-install --rc-file <symlink> zsh`, assert symlink stays a symlink (`os.Lstat`) and the real file contains the appended block.
- [x] T022 In `shell_install_test.go`, add scenario test "--print emits exact block to stdout" — rc file exists empty, run with `--print zsh`, assert stdout equals the three-line block + `\n`, rc file unchanged, exit 0.
- [x] T023 In `shell_install_test.go`, add scenario test "--print accepts shell positional" — `$SHELL=/bin/zsh`, run `--print bash`, assert stdout body line is `eval "$(shll shell-init bash)"`.
- [x] T024 In `shell_install_test.go`, add scenario test "--print still errors when rc file missing" — no rc file, run `--print zsh`, assert exit 2 and stderr mentions the missing file.
- [x] T025 In `shell_install_test.go`, add scenario test "--uninstall removes the block when present" — rc file = `export FOO=bar\n` + block + `export BAR=baz\n`, run `--uninstall zsh`, assert resulting file is `export FOO=bar\nexport BAR=baz\n`, exit 0, stdout contains `Removed shll shell integration`.
- [x] T026 In `shell_install_test.go`, add scenario test "--uninstall when block absent" — rc file = `export FOO=bar\n`, run `--uninstall zsh`, assert file unchanged, exit 0, stderr contains `not installed`.
- [x] T027 In `shell_install_test.go`, add scenario test "--uninstall when rc file absent" — rc path does not exist, run `--uninstall --rc-file <missing> zsh`, assert exit 0, stderr contains `nothing to uninstall`.
- [x] T028 In `shell_install_test.go`, add scenario test "--uninstall preserves symlink chain" — symlink rc → real file containing block, run `--uninstall --rc-file <symlink> zsh`, assert symlink intact (`os.Lstat`) and real file has block removed.
- [x] T029 In `shell_install_test.go`, add scenario test "--print and --uninstall mutually exclusive" — run `shll shell-install --print --uninstall zsh`, assert exit 2 and stderr mentions mutually exclusive.
- [x] T030 In `shell_install_test.go`, add scenario test "Subcommand registered on root" — call `newRootCmd()`, walk subcommands, assert `shell-install` is present alongside `update`, `shell-init`, `version`.
- [x] T031 In `shell_install_test.go`, add scenario test "No proc imports" — verify by file-source inspection (`os.ReadFile` on `shell_install.go`) that the file does not import `internal/proc` or `os/exec`. Defensive check protecting Constitution I scoping.

### Phase 4: Polish

- [x] T032 Update `README.md` install section — after the `brew install sahil87/tap/shll` block, add a `shll shell-install` one-liner shown as the recommended path; keep manual `eval "$(shll shell-init zsh)"` line as a documented fallback. Update the top-level subcommand table to include `shll shell-install`.

## Execution Order

- T001 must precede all T002-T011 (skeleton must exist first).
- T002 depends on T003-T010 because the cobra factory dispatches into them; in practice implement them in the order T003, T004, T005, T006, T007, T008, T009, T010, then revisit T002 to wire up — or write T002 with stub bodies first and fill in. Either way, T011 (root wiring) lands last in the impl phase.
- T012 (test boilerplate) precedes T013-T031.
- T013-T015 are unit tests for individual helpers, independent of each other [P-able].
- T016-T031 are scenario tests, each independent (each uses its own `t.TempDir()` and `t.Setenv`) [P-able].
- T032 (README) can land any time after T002+T011 are in place.

## Acceptance

### Functional Completeness

- [x] A-001 New top-level subcommand `shell-install` is registered on the root cobra command and visible in `shll --help`.
- [x] A-002 Constitution VII justification is recorded in the spec.
- [x] A-003 Shell resolution: positional `zsh`/`bash` accepted; `$SHELL` basename used when no positional; unsupported shells return `errExitCode{code:2}`; `$SHELL=/usr/local/bin/fish` with no positional surfaces inference-error message.
- [x] A-004 Rc-file derivation matches the spec table for all four (shell, OS) combinations and `--rc-file` overrides derivation verbatim.
- [x] A-005 Sentinel block written, matched, and removed is exactly the three-line spec block ending in a single `\n`, body line `eval "$(shll shell-init <shell>)"`.
- [x] A-006 Default-install sequence executes per spec: stat → idempotency → trailing-newline guard → `O_APPEND` write → success message to stdout.
- [x] A-007 `--print` mode: resolves shell + rc, errors on missing rc file, prints exact block to stdout with no surrounding messages, no file modification.
- [x] A-008 `--uninstall` mode: missing rc file → exit 0 "nothing to uninstall"; block-absent → exit 0 "not installed"; block-present → block removed, surrounding content byte-preserved, symlink chain preserved via `EvalSymlinks` + `O_TRUNC` on resolved path.
- [x] A-009 Exit-code policy: 0 on success/no-op, 1 on I/O failure (`errSilent`), 2 on user-invocation error (`errExitCode{code:2}`); matches `shll shell-init`.
- [x] A-010 README install section recommends `shll shell-install` as primary post-install step and retains the manual `eval` line as fallback.

### Behavioral Correctness

- [x] A-011 Trailing-newline guard prepends `\n` only when existing file is non-empty AND last byte is not `\n`; empty files produce no leading `\n`.
- [x] A-012 Idempotency search uses the open sentinel substring; running install twice produces no file change on the second invocation.
- [x] A-013 `--uninstall` removes the trailing `\n` after the close sentinel along with the block (does not leave a stray blank line).
- [x] A-014 `--print` and `--uninstall` are mutually exclusive — combining them exits 2 with a message stating the conflict.

### Scenario Coverage

- [x] A-015 Test "Install when rc file exists, sentinels absent" passes.
- [x] A-016 Test "Install is idempotent" passes.
- [x] A-017 Test "Trailing-newline guard prepends \n" passes.
- [x] A-018 Test "Install errors when rc file missing (no --rc-file)" passes — stderr contains `shll won't create rc files`.
- [x] A-019 Test "Install errors when rc file missing (with --rc-file)" passes — stderr does NOT include the create-rc-files boilerplate.
- [x] A-020 Test "Symlinked rc file preserved on install" passes — symlink intact post-write.
- [x] A-021 Test "--print emits exact block to stdout" passes.
- [x] A-022 Test "--print accepts shell positional" passes.
- [x] A-023 Test "--print still errors when rc file missing" passes.
- [x] A-024 Test "--uninstall removes the block when present" passes.
- [x] A-025 Test "--uninstall when block absent" passes.
- [x] A-026 Test "--uninstall when rc file absent" passes.
- [x] A-027 Test "--uninstall preserves symlink chain" passes.
- [x] A-028 Test "--print and --uninstall mutually exclusive" passes.
- [x] A-029 Tests "macOS bash → ~/.bash_profile" and "Linux bash → ~/.bashrc" both pass via `osGoos` override.

### Edge Cases & Error Handling

- [x] A-030 Unsupported shell positional (e.g. `fish`) exits 2 with `Supported: zsh, bash` in the message.
- [x] A-031 Unsupported `$SHELL` with no positional exits 2 with the inferred shell name in the message.
- [x] A-032 Read-unreadable rc file path returns `errSilent` (exit 1) with diagnostic on stderr.

### Code Quality

- [x] A-033 Pattern consistency: factory + RunE → run helper, distinct exported helpers, named constants for sentinels and the eval-line format — mirrors `shell_init.go` / `update.go` patterns.
- [x] A-034 No unnecessary duplication: reuses `supportedShells`, `isSupportedShell`, `errSilent`, `errExitCode` from the existing package.
- [x] A-035 No god functions: each function focused on a single concern; long dispatch path split into `runShellInstallDefault` / `Print` / `Uninstall` per spec.
- [x] A-036 No magic strings: open/close sentinels, eval-line format, supported-shells list all extracted as named constants or reused from the package.
- [x] A-037 Cross-platform branch isolated to `resolveRcFile` via `osGoos` package-level variable per Constitution: Cross-Platform Behavior.
- [x] A-038 Test-alongside convention satisfied: `shell_install.go` ↔ `shell_install_test.go`.

### Security

- [x] A-039 No subprocess execution: `shell_install.go` imports neither `internal/proc` nor `os/exec` (Constitution I scope is subprocess execution; this command does file I/O only).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
