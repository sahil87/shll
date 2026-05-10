# Plan: Add `shll shell-install` rc-file installer

**Change**: 260510-vul4-shell-install-rc
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

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
