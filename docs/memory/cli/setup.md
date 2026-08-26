---
type: memory
description: "`shll setup` — the re-runnable machine-wiring family: bare `setup` runs the shell half then the agent half (both always run, worst-wins exit, `--yes`/`-y` only); `setup shell [shell]` (sentinel-wrapped rc block, pure rc-wiring, idempotent, `--print`/`--uninstall`/`--rc-file`); `setup agent` (skill placement at two global paths + `run-kit agent setup` delegation). Hidden old spellings `shell-setup` (alias `shell-install`) and `agent-setup` delegate silently for one release cycle."
---
# cli/setup

`shll setup` — the consolidated, re-runnable entry point for wiring a machine for the shll toolkit: the same two halves `shll install` runs in-process at the end of every install (see [cli/install §the post-install auto-run steps](/cli/install.md#the-post-install-auto-run-steps-and-the-next-steps-block)), exposed as one command family. Three visible commands:

- **`shll setup`** — the runnable parent: the shell half, then the agent half (install's `runPostInstallSetup` order). Its ONLY flag is `--yes`/`-y`, forwarded to the agent half's run-kit delegation — no composite `--print`/`--uninstall` modes, no `[shell]` positional; those live on the subcommands.
- **`shll setup shell [shell]`** — the shell half alone, carrying the full shell surface: `[shell]` positional, `--print`, `--uninstall`, `--rc-file`.
- **`shll setup agent`** — the agent half alone, carrying the full agent surface: `--print`, `--uninstall`, `--yes`/`-y`.

All three are THIN cobra faces over the existing internals (`runShellSetup*` / `runAgentSetup`) — no logic moved.

**Hidden compat spellings.** `shll shell-setup` (carrying its `shell-install` cobra alias) and `shll agent-setup` remain registered top-level commands marked `Hidden: true`, with identical flags and behavior, delegating to the same internals — kept for one release cycle because an OLD binary's `shll update` self-refresh executes `shll agent-setup --yes` against the NEW binary across the release boundary (see [the update self-refresh argv](#the-update-self-refresh-argv-refreshargv)). Delegation is SILENT — no cobra `Deprecated:` field, no stderr warning (a warning would leak through the cross-release update refresh — the iags precedent) — and their Short/Long carry only the rename pointer: ``renamed to `shll setup shell` (hidden; kept for one release cycle)`` (and the `agent` twin). Each new subcommand and its hidden old spelling share construction via a parameterized builder — `buildShellSetupCmd(shellSetupCmdSpec)` / `buildAgentSetupCmd(agentSetupCmdSpec)` — so the flag sets cannot drift (parity test-asserted by `TestSetup_FlagSurfaceParity`).

Source: `src/cmd/shll/setup.go` (the parent, the two new-spelling faces, the bare-`setup` seam, and the `setupSub`/`setupShellLeaf`/`setupAgentLeaf` token constants), `src/cmd/shll/shell_setup.go` (shell-half internals + the hidden `shell-setup` factory), `src/cmd/shll/agent_setup.go` (agent-half internals + the hidden `agent-setup` factory).

## Bare `shll setup` — the runnable parent

`runSetup(ctx, env, stdout, stderr, yes)` (`setup.go`) is the implementation seam, extracted from the cobra factory so `setup_test.go` can drive it with `bytes.Buffer` writers and a controlled env. It runs the shell half (`runShellSetup(ctx, nil, "", false, false, stdout, stderr)`) then the agent half (`runAgentSetup(ctx, env, stdout, stderr, false, false, yes)`) — ALWAYS both, mirroring the halves' independence in install's auto-run — and returns `worstError(shellErr, agentErr)`:

- **Worst-wins exit code** (`exitCodeOf`): an `errExitCode` maps to its code; `errSilent` and unclassified errors map to 1; nil maps to 0. The highest of the two halves' codes wins (0 success / 1 operational / 2 usage — the toolkit convention).
- **Tie-break** (`carriesUnprintedMsg`): on equal codes, the error whose message `translateExit` has NOT printed yet (an `errExitCode` with a `msg`) wins over an already-printed `errSilent`, so the unprinted actionable diagnostic is never shadowed.
- The shell half runs with STANDALONE semantics — an unresolvable `$SHELL` or a missing rc file is its exit-2 diagnostic, NOT install's quiet-skip gating (quiet-skip exists so a setup failure cannot fail an install; an explicit user command surfaces problems instead).
- The parent's `--yes` shares the agent half's `agentSetupYesUsage` string — the consent chain is identical (the flag's only consumption point on the parent is the same run-kit delegation).

Tests (`setup_test.go`): `TestSetup_ParentRunsBothHalves`, `TestSetup_WorstWinsExit`, `TestSetup_YesForwardsToAgentHalf`, `TestSetup_ParentOnlyYesFlag`, `TestSetup_ShellSubcommandDispatch`, `TestSetup_AgentSubcommandDispatch`, `TestSetup_FlagSurfaceParity`, `TestRefreshArgv_NewSpelling`, `TestWorstError`. The compat contract is pinned by `TestCompat_OldSpellingsDispatchHiddenAndSilent` (hidden dispatch of `shell-setup`, `agent-setup`, and the `shell-install` alias, with NO deprecation text on stderr).

## Shell half (`shll setup shell`)

`shll setup shell [shell]` — maintains a single sentinel-wrapped shll-managed block in the user's shell rc file. The block holds the cross-tool `eval "$(shll shell-init <shell>)"` line — **the only managed line**. Idempotent re-runs (per-line), optional `--print` (dry run) and `--uninstall` (removal) modes, plus `--rc-file` as a universal escape hatch for non-standard layouts.

> **Pure rc-wiring.** `setup shell` writes only the eval line. Homebrew trust is not a shell-wiring concern — it belongs with *installing* formulae, so per-formula trust lives in `shll install` (the Homebrew-recommended granularity for third-party taps — see [cli/install](/cli/install.md#per-formula-trust-before-install)). A stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line written by an older shll is actively stripped on the next run (see [Stale-export migration](#stale-export-migration)). (0854)

`shell_setup.go` performs **file I/O only** and imports neither `internal/proc` nor `os/exec` (Constitution I scope is subprocess execution) — there is not even a function-value bridge to `brew.go`. `TestNoProcImports` (`shell_setup_test.go`) enforces the no-import invariant by reading the source as bytes, and additionally asserts no `ensureTrustFunc` reference exists (0854).

### Usage

```sh
shll setup shell                         # auto-detect shell from $SHELL, ensure eval line in the block
shll setup shell zsh                     # explicit shell
shll setup shell --print zsh             # dry-run: print the block to stdout, no file change
shll setup shell --uninstall zsh         # remove the whole block from the rc file
shll setup shell --rc-file <path>        # override rc-file derivation entirely
```

The hidden spellings dispatch identically: `shll shell-setup zsh`, and `shll shell-install zsh` via the carried alias.

The single managed line this command writes:

```
eval "$(shll shell-init zsh)"             # always — the only managed line
```

The eval line is the cross-tool composition entry point — see [cli/shell-init](/cli/shell-init.md). `setup shell` exists so the user does not have to know which rc file to paste it into, nor remember to dedupe on re-install.

### Behavior contract

`runShellSetup(ctx, args, rcFileFlag, printMode, uninstallMode, stdout, stderr)` (`shell_setup.go`, `runShellSetup`) is the implementation seam. The cobra `RunE` wrapper (shared by both spellings via `buildShellSetupCmd`) builds the writers and delegates. The dispatch sequence:

1. **Default `ctx`.** A nil context is replaced with `context.Background()`, then immediately discarded (`_ = ctx`) — the parameter is retained only for signature stability; the shell half performs no ctx-scoped work.
2. **Flag conflict.** If both `--print` and `--uninstall` are set → return `errExitCode{code: 2, msg: "shll setup shell: --print and --uninstall are mutually exclusive"}`. Exit code **2**.
3. **Resolve shell.** Delegate to `resolveShell(args, os.Getenv)`.
4. **Resolve rc file.** If `--rc-file <path>` was passed, use it verbatim. Otherwise derive via `resolveRcFile(shell, os.Getenv)`.
5. **Mode dispatch.** `--print` → `runShellSetupPrint`; `--uninstall` → `runShellSetupUninstall`; otherwise → `runShellSetupDefault`.

`--print` and `--uninstall` are mutually-exclusive modes. The `userProvidedPath bool` passed to `runShellSetupDefault` is `true` exactly when `--rc-file` was supplied — it controls whether the missing-rc-file error includes the "shll won't create rc files" hint. All diagnostics carry the `shll setup shell:` prefix (string literals), shared by the hidden old spelling.

### Shell resolution

`resolveShell(args, env)`:

| Input | Output |
|-------|--------|
| Positional `zsh` or `bash` | the positional |
| Positional any other value (e.g. `fish`) | `errExitCode{code:2, msg: "shll setup shell: unsupported shell \"<v>\". Supported: zsh, bash"}` |
| No positional, `$SHELL` basename ∈ `{zsh, bash}` | the inferred shell |
| No positional, `$SHELL` basename unsupported | `errExitCode{code:2, msg: "shll setup shell: cannot infer shell from $SHELL=<raw>. Pass shell explicitly: shll setup shell zsh"}` |

The basename is computed via `filepath.Base($SHELL)`, so canonical absolute paths like `/bin/zsh` and `/usr/local/bin/zsh` collapse to `zsh`. The supported-shell predicate (`isSupportedShell`) is the same one `shell-init` uses — both subcommands share the `supportedShells = {"zsh", "bash"}` constant defined in `shell_init.go`. The two unsupported-shell error messages are deliberately distinct so users get actionable feedback for the path they took (positional rejection vs. environment inference).

### Rc-file derivation

`resolveRcFile(shell, env)` implements the platform-aware default:

| Resolved shell | OS | Derived path |
|----------------|----|----|
| `zsh` | any | `${ZDOTDIR:-$HOME}/.zshrc` |
| `bash` | `osGoos == "darwin"` | `$HOME/.bash_profile` |
| `bash` | any other (`linux` etc.) | `$HOME/.bashrc` |

`osGoos` (package-level variable, top of `shell_setup.go`) is initialized to `runtime.GOOS`. It is the only platform-specific code path in this command and is the abstraction surface required by Constitution: Cross-Platform Behavior. Tests swap it via `setOsGoos(t, value)` so darwin and linux defaults are both reachable from a single host. Because `osGoos` is package-private mutable state, `setOsGoos` saves+restores via `t.Cleanup` and tests that depend on it MUST NOT use `t.Parallel`.

The `--rc-file <path>` flag short-circuits derivation entirely: the supplied path is used verbatim, and `$ZDOTDIR` / `$HOME` are not consulted. This is the documented escape hatch for `$ZDOTDIR` users, dotfile managers writing to the source-of-truth file, and CI scripts that template the rc.

### Sentinel block format (exact)

The shll-managed block uses the **`# >>> shll >>>` / `# <<< shll <<<`** sentinel pair (note the close sentinel uses three `<` chars). It holds exactly **one** managed line — the eval line:

```
# >>> shll >>>
eval "$(shll shell-init <shell>)"
# <<< shll <<<
```

(A stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line from an older install is stripped on the next run — see [Stale-export migration](#stale-export-migration).)

#### Constants (top of `shell_setup.go`)

- `openSentinel = "# >>> shll >>>"` / `closeSentinel = "# <<< shll <<<"` — the **new** sentinels. Exact bytes are user contract (block location + uninstall removal both depend on literal matching).
- `legacyOpenSentinel = "# >>> shll shell-init >>>"` / `legacyCloseSentinel = "# <<< shll shell-init <<<"` — the **pre-rename** sentinels, recognized only for **migration** (install path) and **removal** (uninstall path) of pre-existing blocks. shll never *writes* the legacy sentinels.
- `evalLineFmt = `eval "$(shll shell-init %s)"`` — the eval body, with `%s` substituted by the resolved shell. `evalLine(shell)` formats it.
- `evalLinePrefix = `eval "$(shll shell-init`` — the shell-agnostic prefix used to recognize an existing eval line during a merge, regardless of which shell it was installed for.

#### Block builders

- `buildBlockBody(lines []string) []byte` is the **single source of truth** for block contents: it wraps an ordered set of managed lines in the new sentinel pair, each line plus a trailing `\n`, ending with a single trailing `\n` after the close sentinel. It does **not** reorder or dedup.
- `buildBlock(shell) []byte` is the eval-only convenience builder (routes through `buildBlockBody([]string{evalLine(shell)})`); used by `--print`.
- `wantLines(_ blockMatch, shell string) []string` computes the canonical managed-line set after this invocation. Since the shell half is pure rc-wiring, it returns **just `[evalLine(shell)]`** — the eval line is unconditional and there are no other managed lines to carry forward. The `blockMatch` parameter is **unused** (retained for signature symmetry with the merge call site). A pre-existing block's no-longer-managed lines (e.g. a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1`) are simply not recognized by `findBlockWith`, so a rewrite drops them.

Drift between the write, print, and migration paths is a defect — they all derive from the same constants via `buildBlockBody`. The block carries no "managed by shll, do not edit" line; the bookend sentinels are themselves the visual signal.

### Block location and parsing

`blockMatch` describes a located block: its inclusive byte range `[start, end)` (open sentinel through the trailing `\n` after the close sentinel) plus a single `hasEval` flag extracted from the body.

- `findBlockWith(content, open, close) (m blockMatch, ok, partial bool)` locates a block for a given sentinel pair and parses whether it carries the eval line (body lines are trimmed; **only** a line with `evalLinePrefix` is recognized — any other body line, e.g. a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` from a former `--trust-tap` install, is ignored and so dropped on rewrite). It returns `partial=true` when the open sentinel is present but its matching close is **absent** — an unclosed/corrupted block.
- `locateBlock(content)` is the single entry point used by install and uninstall. It calls `findBlockWith` for **both** the new and legacy sentinels and returns `(newM, newOK, legacyM, legacyOK, partial)`, where `partial` is the OR of either sentinel being open-without-close.

### Idempotency invariant (now per-line)

Idempotency is **per-line**, not a single substring match. The desired block body is `buildBlockBody(wantLines(...))` — just the eval line (the only managed line). A managed line already present is not duplicated.

The byte-identical no-op is detected in the **rewrite path** (`rewriteBlocks`): after splicing out existing block(s) and inserting the merged block, if `bytes.Equal(merged, content)` the file is left untouched, `shll setup shell: already installed in <path> (no changes).` is written to stderr, and the command exits 0. So a full re-run of `shll setup shell` against a block that already contains exactly the eval line is byte-identical before and after. `TestInstall_Idempotent` and `TestMigration_StaleExportThenReRunIsNoop` (the second run after a stale-export strip) assert this with byte-equality.

> **Note on the append path:** `appendBlock` (the no-existing-block case) does not perform an equality short-circuit — there is no block to compare against, so it always writes a fresh block. The no-op semantics live in `rewriteBlocks`, which is the path any *re-run* takes (a block now exists).

### Install path: per-line merge

`runShellSetupDefault(shell, rcPath, userProvidedPath, stdout, stderr)` flow:

1. `os.Stat` the rc file (**no `O_CREATE`** ever). Missing → `errExitCode{code:2}` (see [never creates rc files](#shll-never-creates-rc-files-invariant)). Other stat error → `errSilent` (exit 1).
2. `os.ReadFile` the content.
3. `locateBlock(content)`. If `partial` (open-without-close, either sentinel) → **refuse**: return `errExitCode{code:2, msg: "...has an shll block with an opening sentinel but no matching closing sentinel. Refusing to modify a corrupted block — fix or remove it manually, then re-run."}`. Guessing the bounds of an unclosed block risks corrupting the rc file, so refusing is deliberate.
4. **Compute the desired block.** `desired = buildBlockBody(wantLines(blockMatch{}, shell))` — the eval-only block (no ceremony, no union; the eval line is the only managed line, so a literal `blockMatch{}` is passed). This single call is byte-equivalent to `buildBlock(shell)`.
5. **Write.**
   - No existing block (`!newOK && !legacyOK`) → `appendBlock` (plain `O_APPEND`, symlink-safe).
   - One or both blocks exist → `rewriteBlocks` (read-modify-write → `EvalSymlinks`→`O_TRUNC`).

`appendBlock` applies the trailing-newline guard then `O_APPEND`-writes the block; on success prints `Installed shll shell integration to <path>. Restart your shell or run: source <path>`.

`rewriteBlocks` splices out every existing shll block (new and/or legacy), inserts the eval-only block at the **earliest** removed block's position, and either no-ops (byte-identical) or `EvalSymlinks`→`O_TRUNC`-writes the merged content to the resolved real path. Both the legacy migration rewrite and the stale-export strip route through here. Removal of ranges is done later-range-first so earlier indices stay valid (the two sentinels never overlap).

Covered scenarios (all exit 0): plain install writes the new-sentinel eval-only block, full re-run no-op, legacy-sentinel migration carries the eval line forward, and a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line is stripped to eval-only on the next run.

### Migration: legacy → new sentinel

A legacy `# >>> shll shell-init >>>` block is migrated **in place** on the next install:

- **Legacy-only present** → `locateBlock` finds it via `legacyOK`, `runShellSetupDefault` takes the rewrite branch, splices out the legacy block, and writes the eval-only block under the **new** sentinel — carrying the legacy eval line forward. No legacy sentinel remains. (`TestMigration_LegacyEvalOnlyMigratesOnPlainInstall`.)
- **Both sentinels present** (new + legacy, e.g. hand-edited) → `rewriteBlocks` removes **both** blocks and writes a single new-sentinel eval-only block (self-healing, exit 0). Order-independent (`TestMigration_BothSentinelsPresentMergeToOne`, `TestMigration_BothSentinelsPresentReverseOrderMergeToOne`).
- **Partial/unclosed** (either sentinel open without close) → **refuse**, exit 2, no modification (`TestMigration_PartialUnclosedRefuses`, `TestMigration_PartialUnclosedLegacyRefuses`).

Migration preserves the symlink, trailing-newline, and never-creates-rc-files invariants (it goes through the same `rewriteBlocks` write path).

### Stale-export migration

An rc block written by an older shll may still carry a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line. The next `shll setup shell` run **actively strips it** — no special-casing required, because the existing rewrite/merge path does it for free (0854):

- `findBlockWith` recognizes **only** the eval line as a managed line, so the export line is invisible to the block parse.
- `wantLines` returns just the eval line, so `desired` is the eval-only block.
- Because the block already exists, `runShellSetupDefault` takes the `rewriteBlocks` branch, which splices out the **entire** old block range (export line included) and inserts the freshly-built eval-only block — so the export line is dropped.

The export line was inert anyway (it only re-set Homebrew's default), so stripping it is pure cleanliness. The surrounding rc content is preserved. A plain re-run against a block that already contains only the eval line stays a byte-identical no-op (idempotency). Tests: `TestMigration_StripsStaleExportLine` (export+eval → eval-only, surrounding content preserved) and `TestMigration_StaleExportThenReRunIsNoop` (the second run is byte-identical with the "already installed" message). `TestTrustTapFlagRemoved` asserts cobra reports `--trust-tap` as an unknown flag and the Long help no longer mentions it.

### `--print` (dry-run)

`runShellSetupPrint(shell, rcPath, stdout, stderr)` is a dry-run: it resolves shell + rc file (still errors on a missing rc file — the user may be debugging exactly that), then writes the eval-only block (`buildBlock(shell)`) to stdout with no surrounding messages, and modifies **no file**.

### Symlink-preservation invariants

Two distinct write strategies, depending on whether the operation is read-modify-write:

#### Append (fresh block): plain `O_APPEND`

`appendBlock` opens the rc file with `os.OpenFile(rcPath, os.O_WRONLY|os.O_APPEND, 0)` — no `O_CREATE`, no perm bits. Plain `O_APPEND` follows symlinks to the underlying real file and writes there, so a `~/.zshrc` symlink to `~/dotfiles/zshrc` (chezmoi, dotbot, stow, yadm) stays a symlink and the dotfile-manager source-of-truth file receives the appended block. Per POSIX, `write()` calls under `PIPE_BUF` (4 KiB on Linux, 512 bytes on macOS) are atomic with `O_APPEND`; the sentinel block is well under both limits. `TestInstall_PreservesSymlink` asserts the symlink stays a symlink.

#### In-place rewrite + uninstall: `EvalSymlinks` → `O_TRUNC`

Both `rewriteBlocks` (in-place install / migration / both-sentinels merge) and `runShellSetupUninstall` are read-modify-write, so they cannot use `O_APPEND`. The mitigation:

1. Compute the modified in-memory content (splice out existing block range(s); for rewrite, insert the merged block at the earliest anchor).
2. Resolve the symlink chain: `resolved, _ := filepath.EvalSymlinks(rcPath)`.
3. Truncate-write the modified content to the *resolved* real path: `os.OpenFile(resolved, os.O_WRONLY|os.O_TRUNC, 0)`.

This preserves the user's symlink at the original path (it still points at the same real file) while the underlying source-of-truth file is updated — avoiding the `os.Rename`-of-temp-file hazard that would replace the symlink with a regular file. `TestUninstall_PreservesSymlink` asserts the symlink stays a symlink and the real file's block is removed.

### "shll never creates rc files" invariant

The default-install and `--print` paths both `os.Stat` the rc file and return `errExitCode{code:2, ...}` when it does not exist. They never call `O_CREATE`. The error message branches on whether the user passed `--rc-file`:

- Without `--rc-file`: `shll setup shell: <path> does not exist. shll won't create rc files. Create it first, or pass --rc-file <path>.`
- With `--rc-file`: `shll setup shell: <path> does not exist.` — no boilerplate, since the user explicitly named the path.

A missing rc file is a meaningful signal — custom `$ZDOTDIR`, dotfile manager pending `apply`, non-standard layout — and creating it would mask real configuration issues. The `--uninstall` path treats a missing rc file as benign ("nothing to uninstall", exit 0, stderr-only message).

### Trailing-newline guard

`appendBlock` prepends `\n` to the block exactly when the existing content is non-empty AND its last byte is not `\n`:

```go
if len(content) > 0 && content[len(content)-1] != '\n' {
    block = append([]byte("\n"), block...)
}
```

This prevents the open sentinel from landing on the same line as the user's previous content (e.g. `export FOO=bar# >>> shll >>>`). Empty files require no leading `\n` — a stray blank line at the top of an otherwise empty rc file would be visible noise. `TestInstall_TrailingNewlineGuard` and `TestInstall_EmptyFileNoLeadingNewline` pin both branches. (The guard lives in the append path only; the rewrite path reconstructs content around an existing block whose surrounding newlines are already settled.)

### Uninstall: whole-block removal, both sentinels

`runShellSetupUninstall(shell, rcPath, stdout, stderr)` removes the **entire** shll-managed block (both managed lines, both sentinels) in one operation:

- It recognizes BOTH the new `# >>> shll >>>` sentinel AND a legacy `# >>> shll shell-init >>>` block (so users who never re-installed can still uninstall), via `locateBlock`.
- It splices out every located block (later range first), then `EvalSymlinks`→`O_TRUNC`-writes the result. Both-blocks-present removes both.
- It runs **no Homebrew command at all** — the shell half is pure file I/O (any stale `export` line inside the block is removed with the block). The `shell` argument is unused (sentinels are shell-agnostic).
- Missing rc file or no block present → benign no-op message, exit 0.

On success: `Removed shll shell integration from <path>.` Tests: `TestUninstall_RemovesBlock` (new), `TestUninstall_RemovesLegacyBlock`, `TestUninstall_RemovesBothSentinelBlocks`, `TestUninstall_RemovesStaleExportBlock` (a block still carrying a stale `export` line is removed whole), `TestUninstall_PreservesSymlink`, `TestUninstall_BlockAbsent`, `TestUninstall_RcAbsent`.

### Exit-code policy

Mirrors the convention `shll shell-init` already established — see [cli/commands](/cli/commands.md#exit-code-translation). Both `errSilent` and `errExitCode` from `main.go` are reused; no new sentinel types are introduced.

| Exit code | Conditions |
|-----------|------------|
| **0** | Block written/merged; per-line no-op (byte-identical block already present); stale-export stripped; legacy migration; `--print` succeeded; `--uninstall` removed block or no-op (block/file absent) |
| **1** | I/O failure (read, write, close, `EvalSymlinks`) — emitted via `errSilent` after the diagnostic is written to stderr by the subcommand |
| **2** | User-invocation error — missing/unsupported shell positional, `$SHELL` could not be inferred, rc file does not exist in default or `--print` mode, `--print` and `--uninstall` both supplied, **partial/unclosed sentinel block (refuse-to-modify)** — emitted via `errExitCode{code: 2, msg: ...}` |

`translateExit` in `main.go` writes the `errExitCode.msg` to stderr automatically; subcommand code does not echo it. For `errSilent`, the subcommand has already written its own diagnostic via `fmt.Fprintf(stderr, ...)` and `translateExit` adds nothing.

### Test seam (shell half)

`shell_setup_test.go` (test-alongside, per `code-quality.md` `## Test Strategy`):

- **No `proc.Runner` fake — the shell half invokes no subprocess.** The command is pure file I/O, so every test goes through `runShellSetupCmd(t, argv)` (a fresh cobra command with `bytes.Buffer` writers) against a `t.TempDir()` rc file. The test file does not import `internal/proc`.
- **`t.TempDir()`** for every rc-file test — the user's real `~/.zshrc` / `~/.bashrc` / `~/.bash_profile` is never touched.
- **`osGoos` swap** via `setOsGoos(t, value)` for the macOS-vs-Linux bash defaults. Saves and restores the package-level variable through `t.Cleanup`.
- **`envFunc(map)`** — unit tests for `resolveShell` / `resolveRcFile` use a map-backed env lookup so they run without mutating process state.
- **`t.Setenv`** for end-to-end tests that go through the real cobra command.

Source-level guard: `TestNoProcImports` (`shell_setup_test.go`) reads `shell_setup.go` as bytes and fails if the source contains `internal/proc` or `"os/exec"`; it additionally fails if the source references `ensureTrustFunc` (a regression that would pull subprocess work back toward this file). This is a defensive check protecting Constitution I scoping.

Alias-coverage guard: `TestRoot_ShellInstallAliasResolves` (`shell_setup_test.go`, ri3h) asserts the backward-compat `shell-install` alias dispatches to the same `*cobra.Command` as the hidden `shell-setup` — it builds the root via `newRootCmd()` and checks `root.Find([]string{"shell-install"})` and `root.Find([]string{"shell-setup"})` return the identical command pointer (cobra's `Find` resolves aliases), and that the resolved command's `Name()` is `shell-setup`.

(`brewTrustAvailable` in `brew.go` is reused by `shll install`'s per-formula trust and `shll doctor`'s trust sub-check; its `TestBrewTrustAvailable_*` tests live in `brew_test.go`.)

## Agent half (`shll setup agent`)

`shll setup agent` — mechanically places one thin Agent Skill (the toolkit bootstrap) into the agent harnesses' global skills directories, then delegates run-kit's dashboard-hook wiring to `run-kit agent setup`. Cross-toolkit harness wiring belongs in shll (the manager), not run-kit (a leaf tool); the composition half of the design is [cli/skill](/cli/skill.md). (agst)

Source: `src/cmd/shll/agent_setup.go` (+ `agent_setup_test.go`). This half performs plain file I/O plus ONE subprocess (the run-kit delegation via `internal/proc` — Constitution I). All diagnostics carry the `agentSetupErrPrefix = "shll setup agent"` prefix, shared by the hidden old spelling.

### The mechanism: skill placement, NOT stanza injection

The skill directories are shll-owned, so:

- **install = write, re-run/upgrade = overwrite, `--uninstall` = delete** — idempotent by construction.
- **No sentinel, no merge, no diff-and-confirm, no placement confirmation, no non-TTY refusal.** Those exist to protect user-authored files; overwriting shll-owned skill files needs none of them. (The command's `--yes`/`-y` flag gates nothing in shll — it exists solely to be forwarded to the run-kit delegation, whose hook wiring DOES prompt; see [run-kit delegation](#run-kit-delegation).)
- A skill costs one description line per agent session (loaded on demand) instead of an always-loaded CLAUDE.md stanza.

**Stanza machinery must not reappear**: `shell_setup.go`'s sentinel machinery is not reused here and no `sentinel_block.go` exists — see the Design Decision below.

### The placement set (two writes cover four harnesses)

`skillTargetRelDirs` (relative to `$HOME`) is the **minimal covering set**, verified 2026-07-18 from each harness's official docs:

| Path (`$HOME`-relative) | Covers |
|-------------------------|--------|
| `.agents/skills` | the [agentskills.io](https://agentskills.io) open-standard path — read natively by **Codex** (USER scope), compat-read by **Cursor** and **OpenCode** |
| `.claude/skills` | **Claude Code** (which does NOT read `~/.agents/`) |

The full file is `<dir>/shll-toolkit/SKILL.md` (`skillDirName = "shll-toolkit"`, `skillFileName = "SKILL.md"` — the `<dir>/<name>/SKILL.md` shape the Agent Skills standard requires). `resolveSkillTargets(env)` joins `$HOME + rel + skillDirName + skillFileName`; an empty `$HOME` yields no targets (nothing to place).

Both writes are **unconditional** — `setup agent` is an explicit "wire this machine" command, the cost is two small files in `$HOME`, and any future harness adopting the open standard picks up `~/.agents/skills` automatically. **No harness detection, no skip logic, no skip-a-harness degradation** (user: "no degeneration"). Cursor and OpenCode will see the same-name skill from both locations; the bytes are identical, so this is cosmetic (neither documents cross-location precedence; the recorded fallback is symlinking `~/.claude/skills/shll-toolkit` → the `~/.agents` copy, since Claude Code follows and dedupes symlinked skill dirs).

### The canonical SKILL.md (a Go constant)

`agentSkillContent` is a Go string constant in `agent_setup.go` — **not** a docs-site file. The bootstrap skill is a `setup agent` artifact, neither a published standard nor a `<tool> skill` bundle, so the docs-site sync/embed/drift-guard ceremony (which `docs/site/skill.md` uses — see [cli/skill](/cli/skill.md#the-bundle-authored-embedded-drift-guarded-budget-bounded)) does **not** apply here.

- **Portable frontmatter — `name` + `description` ONLY** (the OpenCode-recognized common subset, valid on all four harnesses). `name: shll-toolkit` equals `skillDirName` (the same constant is spliced into both the frontmatter and the directory name, so they cannot drift) and satisfies the shared `^[a-z0-9]+(-[a-z0-9]+)*$` / match-directory-name rule.
- **The `description` front-loads two kinds of trigger vocabulary**, both single-sourced from the Roster by `agentSkillDescription()` so they cannot drift from the managed set:
  - **Reactive task-domain clauses** — one `task-domain phrase (tool)` clause per tool (`git worktrees (wt)`, …, `tmux sessions (run-kit/rk)`), from each tool's `SkillHint` (with the `LegacyName` alias appended for run-kit). These match a user's own words ("create a worktree"), so every tool earns a clause.
  - **Agent-proactive sentence(s)** — each non-empty `ProactiveHint` appended verbatim (Roster order) **after** the tool clauses and **before** the closing two-step pointer. This is the vocabulary an agent should reach for *unprompted*, so the sprawl guard applies: **only run-kit carries a `ProactiveHint` today**, a two-sentence value that does four jobs. Sentence one carries the reach-for-unprompted vocabulary for run-kit's agent-proactive capabilities — visual display, proxying a local http port to the user's browser, notify, and acting inside the user's code editor ("show the user visual content … in a browser window, to proxy a local http port to the user's browser, to push a notification to their devices, or to act inside the user's code editor — run any VS Code palette command (refresh a PR list, open a diff, focus a view) from the shell with `rk code exec`"). Sentence two is a **skill-shadowing counter-instruction** covering two delivery surfaces: "The user may be viewing this session remotely through run-kit's web dashboard, where `open`/`xdg-open` and localhost URLs never reach them and publishing to a hosted artifact page (e.g. claude.ai) forces them off the dashboard — before opening any file or local port in a browser, read `shll skill run-kit` for the proxied-iframe recipe; the same applies before publishing an artifact or hosted page to show the user something." Reactive tools stay clause-only; the user's words already name them.

  The trailer names the runtime two-step (`Run 'shll skill' to list … 'shll skill <tool>' for that tool's full usage bundle`). **The description trailer is deliberately NOT extended to the topic form** — it is single-line activation-trigger vocabulary (one YAML line, asserted by `TestAgentSetup_DescriptionSingleLine`), not a teaching surface; the topic form belongs in the body's step 2.
- **The body teaches the runtime discovery steps** (`shll skill` → `shll skill <tool>` → `shll skill <tool> <topic>`) plus a thin proactive-capabilities pointer line ("Run-kit also has agent-proactive capabilities — visual display in a browser window, push notifications, and running VS Code palette commands inside the user's code editor (`rk code exec`); see `shll skill run-kit` (and `shll skill run-kit code` for the editor bridge).") — the body is the one place the description's topic-page pointer is allowed, since it loads only on activation and one `shll standards` pointer for toolkit-repo development. Step 2 notes that a large-scope tool's core bundle lists its topic pages and `shll skill <tool> <topic>` serves one on demand (extended by change tp2s). Body text loads only on activation, so the pointer line is activation-cost-only. It only *points at* the runtime steps, so bundles are always fetched from the installed binaries — the placed file stays version-locked in spirit and is refreshed by the installed shll on any re-run (the change-#50 refresh machinery propagates the new body on the next `shll update`).

### The description builder (`agentSkillDescription`)

`agentSkillDescription()` builds the single-line frontmatter description from the Roster in one pass:

```
Use when driving any shll toolkit CLI or shll itself — {clause, …}. {ProactiveHint, …} Run `shll skill` to list the installed tools; run `shll skill <tool>` for that tool's full usage bundle before using it.
```

- Each Roster tool contributes `"<SkillHint> (<name>)"` (name = `Name`, or `Name/LegacyName` when a `LegacyName` exists).
- Each non-empty `ProactiveHint` is collected in the same loop and joined with a single space, then spliced in between the clause list and the two-step pointer — so the proactive vocabulary always falls **after** the tool clauses and **before** `Run \`shll skill\``. With zero proactive hints the splice is skipped entirely (no stray spacing).
- **Single-line, `: `-free invariant.** The whole description MUST be one line with no `: ` sequence (it is an unquoted YAML scalar). `TestAgentSetup_DescriptionSingleLine` pins this; run-kit's `ProactiveHint` sentence is newline- and `: `-free so it satisfies the invariant as-is.

**`ProactiveHint` holds the complete two-sentence prose verbatim; the builder just appends it** — there is no builder-owned "Also use proactively" preamble composed around a stored fragment. This is the simplest faithful rendering while exactly one tool carries a hint; a composed preamble would be over-engineering. The value does **four** jobs, all load-bearing: **(a) proxy trigger vocabulary** ("to proxy a local http port to the user's browser") matching requests that name proxying/dev servers; **(b) a local-browser skill-shadowing counter-instruction** ("The user may be viewing this session remotely … before opening any file or local port in a browser, read `shll skill run-kit`") that fires the moment any competing skill's local delivery step (`open`/`xdg-open`/localhost URL) is about to run; and **(c) a hosted-artifact counter-instruction** ("publishing to a hosted artifact page (e.g. claude.ai) forces them off the dashboard … the same applies before publishing an artifact or hosted page") that fires when an Artifact-style hosted-publishing delivery step — which opens no file and touches no local port — is about to route visuals off the dashboard; and **(d) editor-command trigger vocabulary** ("act inside the user's code editor — run any VS Code palette command … with `rk code exec`") matching requests to refresh a PR list, open a diff, or focus a view in the dashboard's `code` lens, where the user never names run-kit. All four fire regardless of which skill was activated (see the [skill-shadowing Design Decision](#design-decision-the-proactivehint-does-four-jobs) below). The sprawl guard (only agent-proactive capabilities earn description space) is enforced by `TestRosterProactiveHint`, which asserts **exactly run-kit** carries a `ProactiveHint`, that the value appears verbatim in the rendered description, that it is positioned between the tool clauses and the two-step pointer, and — pinning the four functions against a silent rewording — that the rendered description contains the four load-bearing fragments `"to proxy a local http port"` (function a), `"before opening any file or local port in a browser, read"` (function b), `"publishing an artifact"` (function c), and `"rk code exec"` (function d). `SkillHint` is unaffected — run-kit's stays `"tmux sessions"` (the reactive task-domain phrase); `TestRosterSkillHints` still enforces the every-tool `SkillHint` contract.

### Modes and the run seam

`runAgentSetup(ctx, env, stdout, stderr, printMode, uninstallMode, yes)` (the test seam; the cobra factory — shared by both spellings via `buildAgentSetupCmd` — passes `os.Getenv`, `Args: cobra.NoArgs`):

- **`--print --uninstall` together** → `errExitCode{code: usageExitCode}` (exit 2) — mutually exclusive, checked first.
- **`--print`** (`runAgentPrint`) → writes `agentSkillContent` then a `Target paths:` block listing both resolved absolute paths, and **modifies nothing**. **No run-kit delegation.**
- **`--uninstall`** (`runAgentUninstall`) → `os.RemoveAll` on each `shll-toolkit` **directory** (`filepath.Dir(path)`, not just the SKILL.md file); reports `removed`/`absent` per dir; then delegates `run-kit agent setup --uninstall`.
- **default** (`runAgentInstall`) → `placeSkill` per target, then delegates `run-kit agent setup`.
- **`--yes`/`-y`** (registered via the shared `yesFlag`/`yesFlagShorthand` constants from `uninstall.go`, with its own `agentSetupYesUsage` string — shared by `shll setup agent`, the hidden `shll agent-setup`, and bare `shll setup`) → forwards `--yes` to the run-kit delegation on both the install and `--uninstall` paths (3ovi). **`--print --yes` is a harmless no-op, NOT a usage error** — print never delegates, so there is no prompt to skip (deliberate contrast with `--print`+`--uninstall`, which are contradictory modes).

### `placeSkill` — the three-state per-path summary

`placeSkill(path, content, stdout, stderr)` distinguishes three states by reading existing bytes before writing (`os.ReadFile` → compare):

- **`wrote`** — file did not exist (`os.ErrNotExist`); `os.MkdirAll(dir, 0o755)` then `os.WriteFile(path, content, 0o644)`.
- **`unchanged`** — file existed and already held the canonical bytes; **no write is performed** (idempotent re-run).
- **`updated`** — file existed with different bytes; overwritten.

The compare uses `bytes.Equal` on the read content. A non-not-exist read error (permission, etc.) surfaces to stderr and is skipped (sets `anyFailed`). `anyFailed` on any target → `errSilent` (exit 1) after the loop.

### run-kit delegation

`delegateRunKitAgentSetup(ctx, uninstall, yes bool, stderr)` invokes `run-kit agent setup [--uninstall] [--yes]` as a **foreground** subprocess via `proc.RunForeground` (Constitution I). The command family is the two-token argv prefix `runKitAgentSetupArgs = []string{"agent", "setup"}` (minimum run-kit v3.16.23, per run-kit PR #620) against the binary `runKitToolName = "run-kit"`. `agentSetupSub = "agent-setup"` is solely the hidden compat token of shll's OWN agent half (the cobra `Use:` of the hidden `shll agent-setup` top-level command) — the delegation does not use it. An installed run-kit older than v3.16.23 lacks the `agent` family and exits non-zero, landing on the ordinary warn-and-continue path below (see the no-probe Design Decision):

- **`yes` appends `--yes`** to the delegated argv (after `--uninstall` when both apply), skipping run-kit's `Write these changes? [y/N]` hook-wiring confirmation — the unattended-run consent chain (3ovi; see the [Design Decision](#explicit---yes-plumbing-not-tty-detection) below).
- **run-kit absent** (`proc.ErrNotFound`) → skip silently (Constitution V).
- **a real delegation error** (non-absent) → surfaced to stderr with `(continuing)` — it does NOT fail the placement shll already did (placement is the agent half's core work; run-kit hooks are the optional adjunct).
- Its stdio is inherited (foreground) so the user sees run-kit's own output.
- Only the default (install) and `--uninstall` paths delegate; **`--print` never does.**

### The update self-refresh argv (`refreshArgv`)

`refreshArgv(yes)` (`agent_setup.go`) returns `[shll, setup, agent, (--yes)]` — built from `shllTargetToken` + the `setupSub`/`setupAgentLeaf` token constants in `setup.go`. It is the **single source of truth** shared by the live end-of-`shll update` refresh subprocess (`refreshPlacedAgentSkills`) and `shll update --dry-run`'s preview line (`Then: shll setup agent [--yes] (refresh placed agent skills)`), so the preview can never drift from what the run would do. Compat contract: an OLD binary's `refreshArgv` composes `shll agent-setup [--yes]` and executes it against the NEW binary on PATH after the brew self-upgrade — the reason the hidden `shll agent-setup` top-level command survives one release cycle. The refresh's placement gating and best-effort semantics live in [cli/update §end-of-run agent-skill refresh](/cli/update.md#end-of-run-agent-skill-refresh---yes).

### Touchpoints

Three surfaces run or point users at `shll setup agent` (agst, gjhx):

- **`install.go`'s post-install auto-run** — `shll install` runs the equivalent of `shll setup agent --yes` in-process (`runAgentSetup(ctx, env, stdout, stderr, false, false, true)`) at the end of every non-dry-run install, on both outcome paths, unless `--no-agent-setup`. The nudge line (`agentSetupNudgeFmt` — `shll setup agent # optional, once per machine — wire agent harnesses (toolkit context + run-kit dashboard hooks)`) survives as the fallback only: it prints when the step was opted out of or the placement failed — a run-kit *delegation* failure does not re-surface it (that nudge would dead-end). See [cli/install §the post-install auto-run steps](/cli/install.md#the-post-install-auto-run-steps-and-the-next-steps-block).
- **`shll update`'s end-of-run refresh** — the `shll setup agent` self-invocation ([`refreshArgv`](#the-update-self-refresh-argv-refreshargv)), consent-gated by update's own `--yes` chain (3ovi; see the [Design Decision](#explicit---yes-plumbing-not-tty-detection) below).
- **README install flow** — the command block and its explanation paragraph, plus the `### shll setup` and `### shll skill` command sections describing the skills-placement + two-step design (no stanza wording anywhere).

### Constitution fit (agent half)

I — the ONE subprocess (run-kit delegation) routes through `internal/proc`; skill placement is plain `os` file I/O in shll-owned directories. II — stateless (no tracking of whether the agent half ran; re-run re-derives via read-then-compare). III/IV — delegates run-kit's hooks by *pointing at* `run-kit agent setup`, never absorbing them; run-kit's own command keeps working standalone. V — run-kit absent → silent skip.

### Test seam (agent half)

`agent_setup_test.go` drives `runAgentSetup` with `bytes.Buffer` writers, a controlled `env` (`HOME` → `t.TempDir()`), and a fake `proc.Runner`. Coverage grounds R6–R9: both files written with canonical content and a per-path summary; idempotent re-run (byte-identical → `unchanged`); `--print` writes nothing and does not delegate; `--uninstall` removes both dirs and delegates the uninstall pass-through; `--print --uninstall` exits 2; run-kit delegation present-when-installed / silent-when-absent; portable frontmatter (`name` + `description` only) with `name == shll-toolkit == dir name`. The `--yes` forwarding (3ovi) is pinned by `TestAgentSetup_YesForwardsToDelegation` (install argv `agent setup --yes`), `TestAgentSetup_YesRidesUninstallDelegation` (`agent setup --uninstall --yes`), `TestAgentSetup_PrintWithYesIsNoOp` (no write, no delegation, exit 0), and `TestAgentSetup_YesFlagWiredThroughCobra` (flag name/shorthand/usage-string wiring). The delegation-argv assertions pin the two-token literals `"agent", "setup"` (not the constant) so a regression in `runKitAgentSetupArgs` is caught.

Description-vocabulary contracts are pinned separately: `TestRosterSkillHints` (every tool declares a `SkillHint`, each rendered as a `hint (name)` clause), `TestRosterProactiveHint` (exactly run-kit carries a `ProactiveHint`, rendered verbatim after the clauses and before the two-step pointer — the sprawl guard — plus the four load-bearing fragments `"to proxy a local http port"`, `"before opening any file or local port in a browser, read"`, `"publishing an artifact"`, and `"rk code exec"`, so a rewording cannot silently drop the proxy vocabulary, the local-browser counter-instruction, the hosted-artifact counter-instruction, or the editor-command vocabulary), `TestAgentSetup_DescriptionSingleLine` (single-line, `: `-free), and `TestAgentSetup_BodyTeachesTwoStepAndStandards` (body teaches the two-step + `shll standards`, names `rk code exec` and the `shll skill run-kit code` topic page, no stanza/sentinel wording).

## Constitution VII justification

`setup` = cross-toolkit machine wiring in the manager (shll): one visible command family in place of two hyphenated top-level commands — net −1 visible top-level command, and the hidden compat spellings do not count (same convention as `help-dump`). Per half: the shell half solves the manual-rc-edit cliff in the post-`brew install` onboarding flow — it cannot be a flag on `shell-init` (it *invokes* `shell-init`, so a sub-flag is structurally self-referential) and cannot live in a per-tool CLI (per-tool CLIs emit their own shell-init; this command writes the cross-tool composition `eval "$(shll shell-init <shell>)"`, which is exactly what shll exists for); the agent half solves the harness-wiring-is-mis-homed gap (wiring that points agent harnesses at the toolkit belongs in the manager, not a leaf tool — and a static stanza cannot scale to per-tool bundles) and cannot be a flag on any existing subcommand (a distinct machine-provisioning verb). Recorded in [cli/commands §Constitution VII per subcommand](/cli/commands.md#constitution-vii-justification-per-subcommand).

## Design Decisions

### Subcommands over a flag union
**Decision**: `shll setup` / `setup shell` / `setup agent` — a parent with two subcommands, not one command with selector flags.
**Why**: The halves have disjoint flag sets (`--rc-file`/positional shell vs `--yes`); a union surface would be awkward and error-prone. Matches run-kit's noun-verb precedent (`run-kit agent setup`).
**Rejected**: `--only-shell`/`--only-agent` flags on one command (flag union); keeping two top-level commands (the discoverability problem the consolidation exists to fix).
*Introduced by*: 260819-7p6b-consolidate-setup-command

### Hidden top-level commands, not cobra aliases, for the old spellings
**Decision**: The old spellings stay as `Hidden: true` top-level commands sharing construction with the new subcommands.
**Why**: Cobra aliases cannot relocate a command under a new parent, and the compat contract is argv acceptance — `refreshArgv` in OLD binaries executes `shll agent-setup --yes` against the NEW binary across the release boundary.
**Rejected**: cobra `Deprecated:` messages (leak through the update refresh — the iags-precedent UX bug); immediate removal (breaks every cross-boundary `shll update`).
*Introduced by*: 260819-7p6b-consolidate-setup-command

### Skill placement, not context-stanza injection
**Decision**: `setup agent` places an Agent Skill file (write/overwrite/delete); it never merges content into user-authored files.
**Why**: "No merge operation, just mechanical placement of skills" — skill directories are shll-owned, so the sentinel/merge/confirm machinery that protects user-authored rc files is unnecessary, and a skill's description line loads on demand instead of taxing every session like a CLAUDE.md stanza.
**Rejected**: Sentinel-wrapped context-stanza injection into `~/.claude/CLAUDE.md` / AGENTS.md-family files (reusing `shell_setup.go`'s sentinel machinery) — explicitly rejected by the user.
*Introduced by*: `260718-agst-agent-setup-skill-commands`

### Explicit `--yes` plumbing, not TTY detection
**Decision**: Unattended-run consent rides an explicit `--yes` flag threaded through the chain `shll update --yes` → `shll setup agent --yes` → `run-kit agent setup --yes`; shll never infers attendance from the terminal.
**Why**: The motivating failure is a pane-TTY-but-unattended session — run-kit's dashboard update button runs `shll update` in an rk-jobs tmux window, where stdin IS a TTY, so run-kit's non-TTY `--yes` refusal never triggers and its hook prompt hangs forever with nobody attached. That state is structurally undetectable from inside the process; only the caller knows nobody is watching, so the caller must say so.
**Rejected**: TTY detection (fails the motivating case exactly); making `shll update`'s agent-skill refresh unconditionally `--yes` (removes user consent for run-kit's hook writes on attended runs).
*Introduced by*: 260815-3ovi-yes-flag-update-agent-setup

### Install's auto-run always forwards `--yes`
**Decision**: `shll install`'s post-install auto-run invokes the agent-half seam with `yes=true` unconditionally — the equivalent of `shll setup agent --yes` — distinct from `shll update`'s attended-refresh path, which keeps the consent gate.
**Why**: install's terminal path is the unattended curl bootstrap — stdin is the pipe, there is no TTY, and run-kit's hook-wiring prompt refuses non-interactively without `--yes` (`errNonInteractiveConsent`), so omitting it would dead-end exactly the runs the auto-run exists for. run-kit's merge is verified idempotent and non-clobbering (rk-owned `settings.json` entries are marker-detected and replaced in place, never duplicated; malformed JSON is refused, not rewritten), so the prompt is a consent gate, not a data-loss guard.
**Rejected**: Prompting or a timed countdown (no TTY under `curl | sh`); skipping the delegation on the auto-run (leaves the machine half-wired — the adoption failure the auto-run removes).
*Introduced by*: 260819-gjhx-install-auto-shell-agent-setup

### No probe, no old-spelling fallback for rk < v3.16.23
**Decision**: The delegation invokes the two-token `run-kit agent setup` family plainly — no version probe, no retry with the deprecated `run-kit agent-setup` spelling.
**Why**: The delegation is already a best-effort adjunct (ErrNotFound → silent skip; other failures → `(continuing)` warning, never failing the placement). The dominant exposure path — `shll update`'s end-of-run refresh — runs after the roster loop has just upgraded run-kit, so the new family exists by construction; fresh machines get latest rk from brew. A blind retry-on-nonzero cannot distinguish "unknown command" from a genuine setup failure and would re-run a failing (possibly prompting) setup twice.
**Rejected**: Old-spelling fallback retry (indistinguishable failure signal, double-runs real failures); a `--help` capability probe per run (an extra subprocess to protect a one-day-old version boundary); a version-parse gate (same cost, more parsing).
*Introduced by*: 260816-iags-rk-agent-setup-spelling

### Design Decision: the `ProactiveHint` does four jobs
**Decision**: run-kit's `ProactiveHint` is a two-sentence value doing four load-bearing jobs. Sentence one carries agent-proactive trigger vocabulary (visual display + proxy a local http port + notify + (d) act inside the user's code editor via `rk code exec`, the run-kit code bridge that runs VS Code palette commands from the shell). The code-bridge vocabulary joins sentence one's list rather than forming a third sentence: it is a peer capability, not a counter-instruction, and the single YAML line stays shorter. Sentence two is a counter-instruction with two collision surfaces: (b) local `open`/`xdg-open`/localhost URLs may never reach a remote-dashboard user, so read `shll skill run-kit` before opening any file or local port in a browser; and (c) publishing to a hosted artifact page (e.g. claude.ai) forces a remote-dashboard user off the dashboard, so the same recipe applies before publishing an artifact or hosted page to show the user something.
**Why**: The placed skill fails to route agents to run-kit's proxy/visual recipe under **skill shadowing** — a competing content-generation plugin (e.g. `visual-explainer:generate-web-diagram`) carries its own complete delivery path and mentions nothing about rk/proxy/iframe, so the harness activates it and the toolkit skill's body never loads. The only shll-owned text guaranteed in every session's context is the placed skill's frontmatter **description** (all installed skills' descriptions are always listed; bodies load only on activation), so the fix lives there. More trigger vocabulary alone would not fire (a request like "show me examples" contains no proxy/remote/port words); a counter-instruction is what collides with the competing skill's delivery step and creates the unresolved gap that sends the agent to `shll skill run-kit`. The delivery paths that must collide are open-ended: the local-browser surface (b) is not enough by itself, because the Claude Code **Artifact tool** publishes HTML to claude.ai and returns a hosted URL — it opens no file and touches no local port, so it slips past both the proxy vocabulary and the local-browser counter-instruction. Observed 2026-07-22 in a run-kit-managed session: an agent delivered visuals **twice** via hosted artifacts, forcing a dashboard viewer off-dashboard. Function (c) names hosted publishing in both the *reason* clause (forced off the dashboard) and the *action* clause (read `shll skill run-kit` before publishing an artifact/hosted page), closing that vocabulary hole.
**Rejected**: Patching `visual-explainer` (third-party; the shadowing class is open-ended) — changing the skill BODY (loads only on activation, exactly what shadowing prevents) — changing run-kit's own bundle (different repo, already hop-2/unreachable in the failure mode) — a run-kit `agent-setup` session-start hook (the durable, deterministic escalation) **explicitly rejected by the user 2026-07-22 ("messes with user context")**; description wording is the chosen mechanism. The fix is probabilistic by design — a description line competes with a skill the harness has already committed to and can lose.
*Introduced by*: `260721-xv71-runkit-proactivehint-proxy-vocab` (functions a, b); function (c) hosted-artifact counter-instruction by `260722-e09x-runkit-proactivehint-artifact-vocab`; function (d) editor-command vocabulary by `260826-e0gt-code-bridge-skill-vocabulary`

## Cross-references

- Reuse by `doctor` and `shll install` (shell half): the detection primitives (`resolveShell`, `resolveRcFile`, `locateBlock`, `blockMatch.hasEval`) are composed by `doctor.go`'s `resolveWiringFact`, which both consumers share strictly READ-ONLY (it only `os.ReadFile`s the rc file). `install` additionally drives the WRITE path: when the gate reports unwired, its post-install auto-run pre-resolves shell/rc via `resolveShell`/`resolveRcFile` and calls `runShellSetupDefault` — the same write path this command's default mode uses — so the wiring contract above (sentinel format, idempotency, symlink preservation, never-creates-rc-files) is exactly what install inherits:
  - [cli/doctor](/cli/doctor.md#the-wiring-fact--resolvewiringfact-read-only-reuse) — `shll doctor`'s wiring check `os.ReadFile`s the rc file and inspects `hasEval` to mark each shell-init tool wired/unwired/corrupt.
  - [cli/install §the post-install auto-run steps](/cli/install.md#the-post-install-auto-run-steps-and-the-next-steps-block) — `shll install` gates on `resolveWiringFact` (`shellResolved && !corrupt && !wired`), then auto-wires via `runShellSetupDefault`; the gated shell-wiring nudge survives as the opt-out/failure fallback.
- Trust lives with install (0854): per-formula Homebrew trust is `shll install`'s job (the Homebrew-recommended granularity for third-party taps) — see [cli/install §per-formula trust before install](/cli/install.md#per-formula-trust-before-install). The `brewTrustAvailable` helper in `brew.go` is reused there and by `doctor`'s read-only trust sub-check ([cli/doctor §the trust sub-check](/cli/doctor.md#the-trust-sub-check)). The constant `tapName` (`"sahil87/tap"`) in `tools.go` is used only by `doctor`'s tap-level trust check.
- Subcommand registration and exit-code translation: [cli/commands](/cli/commands.md).
- The eval-line target: [cli/shell-init](/cli/shell-init.md) — `setup shell` writes the line that `shell-init` produces output for.
- The runtime steps the placed skill teaches (`shll skill` glossary → `shll skill <tool>` bundle → `shll skill <tool> <topic>` topic page): [cli/skill](/cli/skill.md).
- The standard's landed-design note recording skills placement (not context aggregation): [cli/standards-content §landed design](/cli/standards-content.md#landed-design-shll-setup-agent-skills-placement-not-context-aggregation).
- Subprocess execution: [internal/proc](/internal/proc.md) — the shell half invokes **none** (it is pure file I/O; the `TestNoProcImports` guard pins this); the agent half's run-kit delegation and `shll update`'s refresh subprocess route through it.
- Constitution I (Security First) → `shell_setup.go` is subprocess-free, enforced by the `TestNoProcImports` guard — there is no ceremony seam bridging to `brew.go` at all (0854).
- Cross-Platform Behavior → the darwin-vs-other branch in `resolveRcFile` is the only platform-specific code path, isolated behind the `osGoos` package-level variable (used by no brew call — 0854).
