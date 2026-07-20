# Spec: Add `shll shell-install` rc-file installer

**Change**: 260510-vul4-shell-install-rc
**Created**: 2026-05-10
**Affected memory**: `docs/memory/cli/shell-install.md` (new), `docs/memory/cli/commands.md` (modify), `docs/memory/cli/shell-init.md` (modify)

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

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Sentinel-wrapped block (`# >>> shll shell-init >>>` / `# <<< shll shell-init <<<`) for idempotency and uninstall | Confirmed from intake #1 + #6; required by the install/idempotency/uninstall requirements; user-confirmed during clarify | S:95 R:90 A:90 D:90 |
| 2 | Certain | Plain `O_APPEND` for install (no `tmp + rename`) | Confirmed from intake #2; preserves symlinks for dotfile managers; POSIX atomicity holds for the block size; user-confirmed | S:95 R:85 A:90 D:95 |
| 3 | Certain | Error (exit 2) rather than create rc file when absent in default/`--print` mode | Confirmed from intake #3; missing rc file is a meaningful signal; user-confirmed | S:95 R:85 A:90 D:95 |
| 4 | Certain | Other sahil87 tools are NOT brew dependencies of `shll` | Confirmed from intake #4; preserves Constitution IV/V; out-of-scope here | S:95 R:80 A:90 D:90 |
| 5 | Certain | No brew `postinstall` hook for auto shell-install | Confirmed from intake #5; explicit user command only; out-of-scope here | S:90 R:75 A:85 D:90 |
| 6 | Certain | Sentinel format exact: `# >>> shll shell-init >>>` open / `# <<< shll shell-init <<<` close, body `eval "$(shll shell-init <shell>)"` | Confirmed from intake #6; format reproduced verbatim in spec; user-confirmed | S:95 R:70 A:80 D:75 |
| 7 | Certain | Subcommand name `shell-install` (not `init` / `setup` / `bootstrap`) | Confirmed from intake #7; user-confirmed | S:95 R:80 A:80 D:75 |
| 8 | Certain | Shell inference from `filepath.Base($SHELL)` when no positional | Confirmed from intake #8; user-confirmed | S:95 R:80 A:85 D:85 |
| 9 | Certain | macOS bash → `~/.bash_profile`; Linux bash → `~/.bashrc`; both via `runtime.GOOS` abstraction | Confirmed from intake #9; user-confirmed | S:95 R:80 A:90 D:80 |
| 10 | Certain | Trailing-newline guard before append (prepend `\n` when last byte is not `\n` and file is non-empty) | Confirmed from intake #10; user-confirmed | S:95 R:85 A:90 D:85 |
| 11 | Certain | Three flags only: `--print`, `--uninstall`, `--rc-file` | Confirmed from intake #11; matches the problem surface; no need for additional flags | S:90 R:80 A:85 D:85 |
| 12 | Certain | Exit codes — 2 for user-invocation errors, 1 for I/O failures, 0 for success/no-op | Confirmed from intake #12; matches `shll shell-init` convention; user-confirmed | S:95 R:85 A:90 D:90 |
| 13 | Certain | `--uninstall` resolves symlink first (`filepath.EvalSymlinks`) then truncates the real target | Confirmed from intake #13; preserves dotfile-manager symlinks; user-confirmed | S:95 R:65 A:80 D:75 |
| 14 | Certain | README install section keeps the manual `eval` line as a fallback alongside `shll shell-install` | Confirmed from intake #14; user-confirmed | S:95 R:85 A:90 D:80 |
| 15 | Certain | New memory file `cli/shell-install.md`; modify `cli/commands.md` and `cli/shell-init.md` | Confirmed from intake #15; user-confirmed | S:95 R:85 A:95 D:85 |
| 16 | Certain | Default-install success message includes both "Restart your shell" and `source <path>` hints | Confirmed from intake #16; user-confirmed; reproduced verbatim in install requirement | S:95 R:90 A:80 D:60 |
| 17 | Certain | `--print` accepts a shell positional rather than deriving solely from `$SHELL` | Confirmed from intake #17; consistent with default mode; user-confirmed | S:95 R:90 A:80 D:60 |
| 18 | Certain | No `fish` shell support in this change | Confirmed from intake #18; out-of-scope; documented in Non-Goals | S:95 R:80 A:80 D:70 |
| 19 | Certain | `--print` and `--uninstall` are mutually exclusive flags (combining them exits 2) | Spec-level refinement; intake describes them as separate modes — the mutually-exclusive contract is the simplest interpretation, and the alternative (silent precedence) is undocumentable | S:90 R:90 A:90 D:85 |
| 20 | Certain | Implementation uses only stdlib (`os`, `path/filepath`, `runtime`); no `internal/proc` import | Determined by intake's "no subprocess execution" plus Constitution I scoping (Principle I governs subprocess execution, not file I/O) | S:95 R:90 A:95 D:90 |
| 21 | Certain | `runtime.GOOS` is abstracted behind a package-level variable (e.g., `osGoos`) for testability | Spec-level refinement; the alternative (build tags) is heavier and the project test-strategy is `test-alongside` with table-driven tests; pattern matches `version` package's approach to package-level overridable values | S:90 R:90 A:90 D:80 |
| 22 | Certain | Empty rc file requires no leading `\n` from the trailing-newline guard | Spec-level refinement; clarifies the edge case implicit in intake's "last byte is not `\n`" wording — an empty file has no last byte, so prepending `\n` would create a stray blank line | S:90 R:90 A:90 D:85 |
| 23 | Certain | `--uninstall` removes the trailing `\n` that the install path produced after the close sentinel (does not leave a stray blank line) | Spec-level refinement; the install requirement says the block "terminates with a single `\n`", so symmetric removal removes that `\n` along with the block | S:90 R:90 A:90 D:85 |

23 assumptions (23 certain, 0 confident, 0 tentative, 0 unresolved).

<!-- Merged into plan.md ## Requirements on 2026-06-02 — safe to delete. -->
