# Intake: Add `shll shell-install` rc-file installer

**Change**: 260510-vul4-shell-install-rc
**Created**: 2026-05-10
**Status**: Draft

## Origin

This change emerged from a conversation about lowering day-zero friction for new `shll` users. After installing `shll` via `brew install sahil87/tap/shll` (or `sahil87/tap/all`), the user must add a line to their shell rc file to wire up shell integration:

```sh
eval "$(shll shell-init zsh)"
```

In practice users often: paste it into the wrong file (e.g., `.bash_profile` vs `.bashrc` on Linux), forget to do it at all, get `command not found` because they pasted before installing, or duplicate the line on re-install.

The proposed mitigation: a new `shll shell-install` subcommand that appends the eval line to the user's rc file idempotently. Pattern is well-established (`starship init`, `zoxide init`, `direnv hook`, `fnm env` all expose similar installers).

The discussion explicitly considered and rejected several implementation choices:

- **Making other tools brew dependencies of `shll`** — rejected. Collapses the per-tool opt-in story (Constitution IV, Composition Not Replacement), conflicts with `shll`'s own "uninstalled tools skipped silently" behavior (Constitution V), duplicates the role of the `all` meta-formula, and creates uninstall friction (`brew uninstall hop` errors when `shll` depends on it).
- **`tmp + rename` atomic write** — rejected. Replaces symlinks with regular files, which silently breaks dotfile managers (chezmoi, dotbot, stow, yadm) that symlink `~/.zshrc` → `~/dotfiles/zshrc`. After the rename, the user's source-of-truth file diverges from what's actually loaded by their shell.
- **Auto-creating the rc file when absent** — rejected. A missing `~/.zshrc` is a meaningful signal (user hasn't set up zsh, uses non-standard `$ZDOTDIR`, or has a dotfile manager pending `apply`). Creating it for them masks real configuration issues.
- **Brew postinstall hook that runs `shell-install` automatically** — rejected. Brew formulas that mutate user dotfiles have a bad reputation; this must always be an explicit user command.

The selected design: explicit subcommand, sentinel-wrapped block, plain `O_APPEND` (preserves symlinks), error-on-missing-rc-file, `--print` for dry-run, `--uninstall` for removal.

> Add a `shll shell-install` subcommand that appends `eval "$(shll shell-init <shell>)"` to the user's shell rc file. Idempotent via sentinel comments. Plain `O_APPEND` (preserve symlinks). Don't create the rc file if missing. Support `--print` and `--uninstall`.

## Why

**Problem.** The single biggest cliff in the `shll` onboarding flow is the manual rc-file edit. Once a user runs `brew install sahil87/tap/shll`, they get a working binary, but `hop`, `wt`, and (future) other shell-integrating tools won't have their shell functions defined until the eval line is in their rc file. The current install instructions in `README.md` show the user the line to paste, but there's no command-driven way to put it there.

**Consequence if we don't fix it.**

- New users hit "I installed shll, why doesn't `hop foo` work?" — because `hop` is a shell function, not a binary, and the function never got loaded.
- Users on multiple machines (laptop, desktop, dev VM) repeat the manual step each time.
- Users who follow the README copy the line for the wrong shell (`zsh` block on a bash user's machine, etc.).
- On re-install or update, users sometimes paste the line a second time, producing duplicate functions/aliases at shell startup.
- Dotfile-manager users have to remember to commit the rc change to their dotfiles repo separately from installing `shll`.

**Why this approach over alternatives.**

- **Why an installer subcommand and not just better docs?** The README is necessary regardless, but a one-shot command is dramatically lower-friction than read-and-paste. Every comparable tool (`starship`, `zoxide`, `direnv`, `fnm`, `mise`, `rtx`) ships an installer for exactly this reason.
- **Why not absorb this into `shell-init`?** `shell-init` is invoked from *inside* `eval "$(...)"` — adding a `--write-to-rc` flag to it would be self-referential and confusing. Users don't run `shell-init` directly; they run `eval "$(shell-init zsh)"`. The installer is a separate, user-facing entry point.
- **Why error rather than auto-create the rc file?** A missing rc file is a meaningful signal. Creating it would mask the user's actual config setup (`$ZDOTDIR`, dotfile manager not yet applied, custom shell setup). Erroring with clear guidance keeps the user in control.
- **Why `O_APPEND` instead of atomic `tmp + rename`?** `tmp + rename` replaces symlinks with regular files. For a single sentinel-wrapped block of a few hundred bytes, `O_APPEND` is the correct trade — POSIX guarantees `write()` calls under `PIPE_BUF` (4KB on Linux) are atomic with `O_APPEND`, and we preserve symlinks transparently for dotfile-manager users.

**Constitution VII (Minimal Surface Area) justification.** Adding a new top-level subcommand requires explicit justification in the change's intake.

- **Could this be a flag on an existing subcommand?** No. `shell-init` is what `shell-install` *invokes*; making `shell-install` a flag on `shell-init` is structurally awkward. `update` and `version` are unrelated. The natural shape is a sibling subcommand.
- **Could this belong in a per-tool CLI?** No. Per-tool CLIs (`hop`, `wt`) emit *their own* `shell-init` blob. `shll shell-install` writes a single composed eval line that wraps `shll shell-init`, which is itself the cross-tool composition. This is exactly the kind of cross-tool concern `shll` exists for.

## What Changes

### New subcommand: `shll shell-install`

Cobra command with the following surface:

```
shll shell-install [<shell>] [flags]

Flags:
  --print              Print what would be written, do not modify any file
  --uninstall          Remove the shll-managed block from the rc file
  --rc-file <path>     Override the rc file path (escape hatch for non-standard layouts)

Args:
  <shell>              zsh or bash. If omitted, infer from $SHELL.
```

#### Default invocation: `shll shell-install`

1. **Resolve shell.** If no positional arg, read `$SHELL`, take basename. If basename is in `supportedShells = []string{"zsh", "bash"}`, use it. Otherwise error with `shll shell-install: cannot infer shell from $SHELL=<value>. Pass shell explicitly: shll shell-install zsh`.
2. **Resolve rc file path** (unless `--rc-file` overrides):
   - `zsh` → `${ZDOTDIR:-$HOME}/.zshrc`
   - `bash` → on Linux, `$HOME/.bashrc`; on macOS (`runtime.GOOS == "darwin"`), `$HOME/.bash_profile`. Constitution: cross-platform behavior MUST be isolated behind a small abstraction — this `darwin`-vs-other branch is the abstraction surface.
3. **Stat the rc file.** If `os.Stat` returns `os.ErrNotExist`, error with `shll shell-install: <path> does not exist. shll won't create rc files. Create it first, or pass --rc-file <path>.` Exit code 2 (matches `shell-init` convention for user-invocation errors).
4. **Read existing content.** If the file exists but is unreadable, surface the OS error with exit code 1.
5. **Idempotency check.** Search the file content for the open sentinel `# >>> shll shell-init >>>`. If present, exit 0 with message to stderr: `shll shell-install: already installed in <path> (no changes).`
6. **Build the block.** The block is exactly:
   ```
   # >>> shll shell-init >>>
   eval "$(shll shell-init <shell>)"
   # <<< shll shell-init <<<
   ```
   Where `<shell>` is the resolved shell name. The block always ends with a single trailing newline.
7. **Trailing-newline guard.** Read the last byte of the existing rc file. If it is not `\n`, prepend `\n` to the block before writing — prevents the open sentinel from landing on the same line as the user's previous content.
8. **Append.** Open the file with `os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)` (note: no `O_CREATE`, no perm bits — we erred earlier if the file was absent). Write the block. Close.
9. **Confirm.** Print to stdout: `Installed shll shell integration to <path>. Restart your shell or run: source <path>` and exit 0.

#### `--print` mode

Resolve shell + rc file path the same way (still error on missing rc file — the user might be debugging a real problem). Skip the idempotency check, the read, the trailing-newline guard, and the write. Print the exact block to stdout that would be written, without surrounding messages. Exit 0.

```sh
$ shll shell-install --print zsh
# >>> shll shell-init >>>
eval "$(shll shell-init zsh)"
# <<< shll shell-init <<<
```

This is the suggested escape hatch for any case where the auto-install would error (custom rc layout, file not present, file not writable).

#### `--uninstall` mode

1. Resolve shell + rc file path.
2. If rc file does not exist, exit 0 with message `shll shell-install: <path> does not exist (nothing to uninstall).`
3. Read full content.
4. Search for the block bounded by `# >>> shll shell-init >>>` and `# <<< shll shell-init <<<` (inclusive). If absent, exit 0 with message `shll shell-install: not installed in <path> (nothing to uninstall).`
5. Remove the block (and its trailing newline if it produced one). For `--uninstall`, plain `O_APPEND` is not enough — we need a read-modify-write through the path. Open with `O_WRONLY|O_TRUNC` on the *resolved-realpath* target (so dotfile-manager symlinks are preserved). Write the modified content.
6. Print to stdout: `Removed shll shell integration from <path>.` Exit 0.

The `--uninstall` write path is the one place where atomic-rename's symlink hazard reappears. The chosen mitigation is to resolve the symlink first via `filepath.EvalSymlinks` and then rewrite the resolved target — this preserves the symlink chain (the symlink at the original path still points where it always did, and the underlying file is updated).

#### `--rc-file <path>` flag

Universal escape hatch. When present, skips both the rc-file derivation and the existence check's error guidance (the user explicitly named the path; if it doesn't exist, we still error, but the message says `<the path you passed> does not exist` rather than suggesting it be created). Useful for: `$ZDOTDIR` users, dotfile managers that want to write to the source file, CI scripts that template the rc.

### Sentinel block format (exact)

The sentinel format is non-negotiable and must match exactly across install / idempotency-check / uninstall. Reproduced verbatim:

```
# >>> shll shell-init >>>
eval "$(shll shell-init zsh)"
# <<< shll shell-init <<<
```

- Open: `# >>> shll shell-init >>>` (literal, including the spaces and the three `>` chars).
- Close: `# <<< shll shell-init <<<` (literal, including the spaces and the three `<` chars).
- Body: exactly one line, `eval "$(shll shell-init <shell>)"` with `<shell>` replaced by the resolved name.
- The block always terminates with a single `\n` after the close sentinel.

The sentinel chosen mirrors the `>>>`/`<<<` convention used by `mise activate` and similar tools — visually distinct from typical shell comments, easy to search for, easy to pattern-match in `--uninstall`. The block does not include a "managed by shll, do not edit" warning line — keeping the block to three lines minimizes the rc-file footprint and the visual noise of three sentinel lines is its own warning.

### File layout impact

New file: `src/cmd/shll/shell_install.go` — `newShellInstallCmd()`, `runShellInstall(ctx, args, flags, stdout, stderr)`, helpers `resolveShell`, `resolveRcFile`, `buildBlock`, `findExistingBlock`, `appendBlock`, `removeBlock`.

New file: `src/cmd/shll/shell_install_test.go` — covers the scenarios listed in **Test scenarios** below.

Modified: `src/cmd/shll/root.go` — add `newShellInstallCmd()` to the `AddCommand` list.

Modified: `src/cmd/shll/main.go` — no changes expected; `errExitCode` and `errSilent` already cover the exit-code needs (2 for missing-arg / unsupported-shell / missing-rc-file; 1 for I/O failures).

### Test scenarios (intake-level — full GIVEN/WHEN/THEN scenarios go in spec)

- Install when rc file exists, sentinels absent → block appended, exit 0.
- Install when rc file exists, sentinels present → no-op, exit 0, message to stderr.
- Install when rc file exists but does not end with `\n` → block prepended with leading `\n`, no merging with previous line.
- Install when rc file does not exist → exit 2, error mentions the path and that shll won't create it.
- Install when `$SHELL` is not zsh/bash and no positional → exit 2, error suggests passing shell explicitly.
- Install with `--rc-file` → uses passed path, ignores `$ZDOTDIR` / `$HOME` derivation.
- Install when rc file is a symlink → `O_APPEND` follows; underlying target receives the block; symlink still exists.
- `--print` mode → prints exact block to stdout, no file modification, exit 0.
- `--uninstall` when block present → block removed, surrounding content untouched, symlink chain preserved.
- `--uninstall` when block absent → exit 0, "nothing to uninstall" message.
- `--uninstall` when rc file absent → exit 0, "nothing to uninstall" message.
- macOS bash defaults to `~/.bash_profile`; Linux bash defaults to `~/.bashrc` — both detected via `runtime.GOOS`.

### README updates

`README.md` install section gets a follow-up paragraph after the `brew install` block:

```sh
shll shell-install                  # auto-detect shell, append eval line to rc file
```

Showing a one-shot install path replaces the manual `eval "$(shll shell-init zsh)"` instruction as the primary recommendation. The `eval` line stays in the README as the manual fallback for users who prefer to edit their rc file themselves.

## Affected Memory

- `cli/commands`: (modify) Add `shell-install` to the cobra root subcommand list. Update the Constitution VII justification block with the four-bullet rationale for this subcommand. Update the file-layout table with `shell_install.go`. Bump v0.1.0 surface count from "exactly three" to "exactly four" (or rephrase to reflect the new closed set).
- `cli/shell-install`: (new) New memory file documenting the subcommand: behavior contract (default / `--print` / `--uninstall` modes), shell detection, rc-file derivation table per shell × OS, sentinel format, idempotency invariant, symlink-preservation invariant, "shll never creates rc files" invariant, exit-code table, test seam.
- `cli/shell-init`: (modify) Add a cross-reference in the **Cross-references** section pointing to `cli/shell-install` as the installer that wraps the eval invocation.

## Impact

**Code areas.**

- `src/cmd/shll/` — new file `shell_install.go` (~150–200 LoC) and its test file. Modifications to `root.go` (one-line addition) and the README.
- No changes to `internal/proc` — `shell-install` does not invoke subprocesses; it does file I/O only. (Constitution I applies to subprocess execution; file I/O follows ordinary Go safety practices.)
- No changes to `tools.go` / `Roster` — the installer is shell-agnostic at the roster level; what it writes is `shll shell-init <shell>`, which is the existing entry point.

**Dependencies.** No new Go dependencies. Uses `os`, `path/filepath`, `runtime` only (already in stdlib usage).

**APIs.** New CLI surface: `shll shell-install`. No changes to existing subcommands' surfaces. Exit-code policy reuses `errExitCode` (2) and `errSilent` (1) sentinels.

**Systems.** Touches the user's home directory (rc files only). No network calls, no subprocess execution.

**Brew formula.** No formula changes required — the binary just gains a subcommand. Explicitly excluded: a brew postinstall hook that auto-runs `shell-install`. That is an anti-pattern (Origin section).

**Cross-platform.** macOS bash default rc file (`.bash_profile`) differs from Linux (`.bashrc`). The `darwin`-vs-other branch is isolated to `resolveRcFile()` per Constitution: Cross-Platform Behavior.

**Constitution interactions.**
- **I (Security First)** — does not apply (no subprocess execution); ordinary Go file-I/O safety.
- **II (No State)** — fine; the rc file is the user's state, written once per invocation, never read back later.
- **III (Wrap, Don't Reinvent)** — emits a literal `eval` line; does not duplicate `shell-init`'s composition logic.
- **IV (Composition, Not Replacement)** — does not replace the manual rc-file edit pathway; users can still edit by hand. The README will keep the manual `eval` line as a fallback.
- **V (Graceful Degradation)** — divergent: this subcommand is *user-invoked*, not *user-affecting*. Errors here are surfaced loudly (the user wants feedback). Compare to `shell-init` where uninstalled tools are silently omitted.
- **VII (Minimal Surface Area)** — addressed in **Why** above.

## Open Questions

- Should `--print` accept a shell positional, or always derive from `$SHELL`? (Tentatively yes — accept positional, fall back to `$SHELL` — for consistency with default mode.)
- Should the success message on default install include the `source <rc>` hint, or just say "restart your shell"? (Tentatively include both — "Restart your shell or run: source <path>".)
- Is there a future `fish` story? Not in this change; if added later, fish convention is a drop-in dir (`~/.config/fish/conf.d/shll.fish`) rather than editing `config.fish`. Out of scope here.
- Should `shll update` (the existing subcommand) gain awareness of an installed shell-init block, e.g. warn if the rc file was edited by a tool other than shll? Out of scope; would couple `update` to rc-file knowledge, violating subcommand independence.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Sentinel-wrapped block for idempotency | Discussed — user agreed sentinels survive future format changes and give `--uninstall` a clean removal target | S:95 R:90 A:90 D:90 |
| 2 | Certain | Plain `O_APPEND` (no `tmp + rename`) for install | Discussed — user explicitly chose option 1 to preserve dotfile-manager symlinks; POSIX atomicity holds for our block size | S:95 R:85 A:90 D:95 |
| 3 | Certain | Error rather than create rc file when absent | Discussed — user explicitly chose this; missing rc file is a meaningful signal (ZDOTDIR, dotfile manager pending) | S:95 R:85 A:90 D:95 |
| 4 | Certain | Don't make other tools brew dependencies of shll | Discussed — user raised, agent recommended against; Constitution IV/V conflicts and `all` formula already covers bundle-install | S:95 R:80 A:90 D:90 |
| 5 | Certain | No brew postinstall hook for auto shell-install | Discussed — must always be explicit user command; brew formulas mutating rc files have a bad reputation | S:90 R:75 A:85 D:90 |
| 6 | Confident | Sentinel format `# >>> shll shell-init >>>` / `# <<< shll shell-init <<<` | Mirrors `mise activate` / `direnv hook` convention; visually distinct, easy to grep, easy to pattern-match in uninstall | S:75 R:70 A:80 D:75 |
| 7 | Confident | Subcommand name `shell-install` (vs `init`, `setup`, `bootstrap`) | `init` clashes with `shell-init`; `setup` is generic; `install` is unambiguous and matches user intent. Pairs naturally with `--uninstall` flag | S:70 R:80 A:80 D:75 |
| 8 | Confident | Shell detection from `$SHELL` basename when no positional | Standard pattern (starship, zoxide, mise all do this); explicit positional remains the canonical form | S:80 R:80 A:85 D:85 |
| 9 | Confident | macOS bash → `~/.bash_profile`; Linux bash → `~/.bashrc` | POSIX login-shell convention on macOS; isolated behind `runtime.GOOS` per Constitution Cross-Platform | S:75 R:80 A:90 D:80 |
| 10 | Confident | Trailing-newline guard before append | Cheap, prevents block from merging with user's last line; standard hygiene for append-based installers | S:80 R:85 A:90 D:85 |
| 11 | Certain | `--print` for dry-run; `--uninstall` for removal; `--rc-file` for override | User explicitly named `--print` and `--uninstall` in the change request; `--rc-file` follows from the missing-rc-file error guidance | S:90 R:80 A:85 D:85 |
| 12 | Confident | Exit codes — 2 for user-invocation errors, 1 for I/O failure, 0 for success/no-op | Reuses existing `errExitCode`/`errSilent` policy from `shell-init` (cli/commands.md) | S:85 R:85 A:90 D:90 |
| 13 | Confident | `--uninstall` resolves symlink first then truncates the real target | Preserves dotfile-manager symlinks for the destructive path too; otherwise truncate would leave the symlink intact but the source-of-truth file unchanged | S:75 R:65 A:80 D:75 |
| 14 | Confident | README install section keeps manual `eval` line as fallback | Constitution IV — composition not replacement; users with custom setups should retain the manual path | S:80 R:85 A:90 D:80 |
| 15 | Confident | New memory file `cli/shell-install.md`, modify `cli/commands.md` and `cli/shell-init.md` | Mirrors existing memory shape; per-subcommand file is the established pattern | S:85 R:85 A:95 D:85 |
| 16 | Confident | Default-install success message includes both "restart shell" and "source <path>" hints | Both are useful; either alone is fine. Trivial output-only choice, fully reversible | S:65 R:90 A:80 D:60 |
| 17 | Confident | `--print` accepts a shell positional rather than deriving solely from `$SHELL` | Consistency with default mode argues for positional; trivial UX choice, fully reversible | S:60 R:90 A:80 D:60 |
| 18 | Confident | No `fish` support in this change | Explicit scope exclusion — fish convention differs (drop-in dir, not rc edit); deferring keeps scope tight, follow-up change can add it | S:70 R:80 A:80 D:70 |

18 assumptions (6 certain, 12 confident, 0 tentative, 0 unresolved).
