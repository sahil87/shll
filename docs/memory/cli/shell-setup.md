---
type: memory
description: "`shll shell-setup [shell]` (alias `shell-install`) — sentinel-wrapped rc-file block, pure rc-wiring (eval line only), idempotent install/`--print`/`--uninstall`, stale-export migration."
---
# cli/shell-setup

`shll shell-setup [shell]` — maintains a single sentinel-wrapped shll-managed block in the user's shell rc file. The block holds the cross-tool `eval "$(shll shell-init <shell>)"` line — **the only managed line**. Idempotent re-runs (per-line), optional `--print` (dry run) and `--uninstall` (removal) modes, plus `--rc-file` as a universal escape hatch for non-standard layouts.

> **Pure rc-wiring since change 0854 — `--trust-tap` removed.** Earlier versions (change l6lo) carried a `--trust-tap` flag that wrote an `export HOMEBREW_REQUIRE_TAP_TRUST=1` policy line **and** ran a whole-tap `brew trust --tap` ceremony. Change 0854 **removed `--trust-tap` entirely** (the flag, the export line + its merge logic, the `ensureTrustFunc` ceremony seam, `blockMatch.hasExport`, and the whole-tap ceremony helpers in `brew.go`). Trust is no longer a shell-wiring concern — it belongs with *installing* formulae, so per-formula trust moved to `shll install` (per-formula, the Homebrew-recommended granularity for third-party taps — see [cli/install](/cli/install.md#per-formula-trust-before-install-change-0854)). `shell-setup` is now **pure rc-wiring**: a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line from a former `--trust-tap` install is actively stripped on the next run (see [Stale-export migration](#stale-export-migration-change-0854)).

**Canonical name + back-compat alias.** `shell-setup` is the canonical command name (renamed from `shell-install` by change ri3h). `shell-install` is retained as a cobra alias (`Aliases: []string{"shell-install"}`) that dispatches to the same `*cobra.Command` — existing rc files, scripts, and muscle memory keep working with zero breakage. The rename was a full Go-identifier rename (file, factory `newShellSetupCmd`, run helpers, test file/helpers) off the `ShellInstall` stem; behavior is identical, only names/help/message-prefixes changed.

Source: `src/cmd/shll/shell_setup.go`. This file performs **file I/O only** and imports neither `internal/proc` nor `os/exec` (Constitution I scope is subprocess execution). Since change 0854 removed the `--trust-tap` ceremony seam, the file is **strictly** file-I/O — there is no longer even a function-value bridge to `brew.go`. `TestNoProcImports` (`func TestNoProcImports` in `shell_setup_test.go`) enforces the no-import invariant by reading the source as bytes, and is now **stronger**: it additionally asserts the removed `ensureTrustFunc` seam is absent.

## Usage

```sh
shll shell-setup                         # auto-detect shell from $SHELL, ensure eval line in the block
shll shell-setup zsh                     # explicit shell
shll shell-setup --print zsh             # dry-run: print the block to stdout, no file change
shll shell-setup --uninstall zsh         # remove the whole block from the rc file
shll shell-setup --rc-file <path>        # override rc-file derivation entirely
shll shell-install zsh                   # alias — back-compat, dispatches to the same command
```

The single managed line this command writes:

```
eval "$(shll shell-init zsh)"             # always — the only managed line
```

The eval line is the cross-tool composition entry point — see [cli/shell-init](/cli/shell-init.md). `shell-setup` exists so the user does not have to know which rc file to paste it into, nor remember to dedupe on re-install. (Homebrew tap-trust is no longer touched here — it moved to `shll install` as of change 0854.)

## Behavior contract

`runShellSetup(ctx, args, rcFileFlag, printMode, uninstallMode, stdout, stderr)` (`shell_setup.go`, `runShellSetup`) is the implementation seam. The cobra `RunE` wrapper builds the writers and delegates — there is no ceremony function to pass (change 0854). The dispatch sequence:

1. **Default `ctx`.** A nil context is replaced with `context.Background()`, then immediately discarded (`_ = ctx`) — the parameter is retained only for signature stability; shell-setup performs no ctx-scoped work after the ceremony seam was removed.
2. **Flag conflict.** If both `--print` and `--uninstall` are set → return `errExitCode{code: 2, msg: "shll shell-setup: --print and --uninstall are mutually exclusive"}`. Exit code **2**.
3. **Resolve shell.** Delegate to `resolveShell(args, os.Getenv)`.
4. **Resolve rc file.** If `--rc-file <path>` was passed, use it verbatim. Otherwise derive via `resolveRcFile(shell, os.Getenv)`.
5. **Mode dispatch.** `--print` → `runShellSetupPrint`; `--uninstall` → `runShellSetupUninstall`; otherwise → `runShellSetupDefault`.

`--print` and `--uninstall` are mutually-exclusive modes. The `userProvidedPath bool` passed to `runShellSetupDefault` is `true` exactly when `--rc-file` was supplied — it controls whether the missing-rc-file error includes the "shll won't create rc files" hint.

## Shell resolution

`resolveShell(args, env)`:

| Input | Output |
|-------|--------|
| Positional `zsh` or `bash` | the positional |
| Positional any other value (e.g. `fish`) | `errExitCode{code:2, msg: "shll shell-setup: unsupported shell \"<v>\". Supported: zsh, bash"}` |
| No positional, `$SHELL` basename ∈ `{zsh, bash}` | the inferred shell |
| No positional, `$SHELL` basename unsupported | `errExitCode{code:2, msg: "shll shell-setup: cannot infer shell from $SHELL=<raw>. Pass shell explicitly: shll shell-setup zsh"}` |

The basename is computed via `filepath.Base($SHELL)`, so canonical absolute paths like `/bin/zsh` and `/usr/local/bin/zsh` collapse to `zsh`. The supported-shell predicate (`isSupportedShell`) is the same one `shell-init` uses — both subcommands share the `supportedShells = {"zsh", "bash"}` constant defined in `shell_init.go`. The two unsupported-shell error messages are deliberately distinct so users get actionable feedback for the path they took (positional rejection vs. environment inference).

## Rc-file derivation

`resolveRcFile(shell, env)` implements the platform-aware default:

| Resolved shell | OS | Derived path |
|----------------|----|----|
| `zsh` | any | `${ZDOTDIR:-$HOME}/.zshrc` |
| `bash` | `osGoos == "darwin"` | `$HOME/.bash_profile` |
| `bash` | any other (`linux` etc.) | `$HOME/.bashrc` |

`osGoos` (package-level variable, top of `shell_setup.go`) is initialized to `runtime.GOOS`. It is the only platform-specific code path in this command and is the abstraction surface required by Constitution: Cross-Platform Behavior. Tests swap it via `setOsGoos(t, value)` so darwin and linux defaults are both reachable from a single host. Because `osGoos` is package-private mutable state, `setOsGoos` saves+restores via `t.Cleanup` and tests that depend on it MUST NOT use `t.Parallel`.

The `--rc-file <path>` flag short-circuits derivation entirely: the supplied path is used verbatim, and `$ZDOTDIR` / `$HOME` are not consulted. This is the documented escape hatch for `$ZDOTDIR` users, dotfile managers writing to the source-of-truth file, and CI scripts that template the rc.

## Sentinel block format (exact)

The shll-managed block uses the **`# >>> shll >>>` / `# <<< shll <<<`** sentinel pair (note the close sentinel uses three `<` chars). Since change 0854 it holds exactly **one** managed line — the eval line:

```
# >>> shll >>>
eval "$(shll shell-init <shell>)"
# <<< shll <<<
```

(Before change 0854, a `--trust-tap` install could additionally carry an `export HOMEBREW_REQUIRE_TAP_TRUST=1` line above the eval line; that export line is no longer written, and a stale one is stripped on the next run — see [Stale-export migration](#stale-export-migration-change-0854).)

### Constants (top of `shell_setup.go`)

- `openSentinel = "# >>> shll >>>"` / `closeSentinel = "# <<< shll <<<"` — the **new** sentinels. Exact bytes are user contract (block location + uninstall removal both depend on literal matching).
- `legacyOpenSentinel = "# >>> shll shell-init >>>"` / `legacyCloseSentinel = "# <<< shll shell-init <<<"` — the **pre-rename** sentinels, recognized only for **migration** (install path) and **removal** (uninstall path) of pre-existing blocks. shll never *writes* the legacy sentinels.
- `evalLineFmt = `eval "$(shll shell-init %s)"`` — the eval body, with `%s` substituted by the resolved shell. `evalLine(shell)` formats it.
- `evalLinePrefix = `eval "$(shll shell-init`` — the shell-agnostic prefix used to recognize an existing eval line during a merge, regardless of which shell it was installed for.

(The `exportTrustLine` constant was removed by change 0854 along with `--trust-tap`.)

### Block builders

- `buildBlockBody(lines []string) []byte` is the **single source of truth** for block contents: it wraps an ordered set of managed lines in the new sentinel pair, each line plus a trailing `\n`, ending with a single trailing `\n` after the close sentinel. It does **not** reorder or dedup.
- `buildBlock(shell) []byte` is the eval-only convenience builder (routes through `buildBlockBody([]string{evalLine(shell)})`); used by `--print`.
- `wantLines(_ blockMatch, shell string) []string` computes the canonical managed-line set after this invocation. Since shell-setup is pure rc-wiring, it returns **just `[evalLine(shell)]`** — the eval line is unconditional and there are no other managed lines to carry forward. The `blockMatch` parameter is **unused** (retained for signature symmetry with the merge call site); it no longer carries the dropped export-branch logic. A pre-existing block's no-longer-managed lines (e.g. a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1`) are simply not recognized by `findBlockWith`, so a rewrite drops them.

Drift between the write, print, and migration paths is a defect — they all derive from the same constants via `buildBlockBody`. The block carries no "managed by shll, do not edit" line; the bookend sentinels are themselves the visual signal.

## Block location and parsing

`blockMatch` describes a located block: its inclusive byte range `[start, end)` (open sentinel through the trailing `\n` after the close sentinel) plus a single `hasEval` flag extracted from the body. (The `hasExport` flag was removed by change 0854 along with the export line.)

- `findBlockWith(content, open, close) (m blockMatch, ok, partial bool)` locates a block for a given sentinel pair and parses whether it carries the eval line (body lines are trimmed; **only** a line with `evalLinePrefix` is recognized — any other body line, e.g. a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` from a former `--trust-tap` install, is ignored and so dropped on rewrite). It returns `partial=true` when the open sentinel is present but its matching close is **absent** — an unclosed/corrupted block.
- `locateBlock(content)` is the single entry point used by install and uninstall. It calls `findBlockWith` for **both** the new and legacy sentinels and returns `(newM, newOK, legacyM, legacyOK, partial)`, where `partial` is the OR of either sentinel being open-without-close.

## Idempotency invariant (now per-line)

Idempotency is **per-line**, not a single substring match. The desired block body is `buildBlockBody(wantLines(...))` — since change 0854 that is just the eval line (the only managed line). A managed line already present is not duplicated.

The byte-identical no-op is detected in the **rewrite path** (`rewriteBlocks`): after splicing out existing block(s) and inserting the merged block, if `bytes.Equal(merged, content)` the file is left untouched, `shll shell-setup: already installed in <path> (no changes).` is written to stderr, and the command exits 0. So a full re-run of `shll shell-setup` against a block that already contains exactly the eval line is byte-identical before and after. `TestInstall_Idempotent` and `TestMigration_StaleExportThenReRunIsNoop` (the second run after a stale-export strip) assert this with byte-equality.

> **Note on the append path:** `appendBlock` (the no-existing-block case) does not perform an equality short-circuit — there is no block to compare against, so it always writes a fresh block. The no-op semantics live in `rewriteBlocks`, which is the path any *re-run* takes (a block now exists).

## Install path: per-line merge

`runShellSetupDefault(shell, rcPath, userProvidedPath, stdout, stderr)` flow:

1. `os.Stat` the rc file (**no `O_CREATE`** ever). Missing → `errExitCode{code:2}` (see [never creates rc files](#shll-never-creates-rc-files-invariant)). Other stat error → `errSilent` (exit 1).
2. `os.ReadFile` the content.
3. `locateBlock(content)`. If `partial` (open-without-close, either sentinel) → **refuse**: return `errExitCode{code:2, msg: "...has an shll block with an opening sentinel but no matching closing sentinel. Refusing to modify a corrupted block — fix or remove it manually, then re-run."}`. This is a deliberate divergence from the legacy short-circuit-as-"already-installed" behavior (guessing the bounds of an unclosed block risks corrupting the rc file).
4. **Compute the desired block.** `desired = buildBlockBody(wantLines(blockMatch{}, shell))` — the eval-only block (no ceremony, no union; change 0854 made the eval line the only managed line, so a synthesized `existing` blockMatch is no longer needed and a literal `blockMatch{}` is passed). This single call is byte-equivalent to `buildBlock(shell)`.
5. **Write.**
   - No existing block (`!newOK && !legacyOK`) → `appendBlock` (plain `O_APPEND`, symlink-safe).
   - One or both blocks exist → `rewriteBlocks` (read-modify-write → `EvalSymlinks`→`O_TRUNC`).

`appendBlock` applies the trailing-newline guard then `O_APPEND`-writes the block; on success prints `Installed shll shell integration to <path>. Restart your shell or run: source <path>`.

`rewriteBlocks` splices out every existing shll block (new and/or legacy), inserts the eval-only block at the **earliest** removed block's position, and either no-ops (byte-identical) or `EvalSymlinks`→`O_TRUNC`-writes the merged content to the resolved real path. Both the legacy migration rewrite and the stale-export strip route through here. Removal of ranges is done later-range-first so earlier indices stay valid (the two sentinels never overlap).

Covered scenarios (all exit 0): plain install writes the new-sentinel eval-only block, full re-run no-op, legacy-sentinel migration carries the eval line forward, and a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line is stripped to eval-only on the next run.

## Migration: legacy → new sentinel

A legacy `# >>> shll shell-init >>>` block is migrated **in place** on the next install:

- **Legacy-only present** → `locateBlock` finds it via `legacyOK`, `runShellSetupDefault` takes the rewrite branch, splices out the legacy block, and writes the eval-only block under the **new** sentinel — carrying the legacy eval line forward. No legacy sentinel remains. (`TestMigration_LegacyEvalOnlyMigratesOnPlainInstall`.)
- **Both sentinels present** (new + legacy, e.g. hand-edited) → `rewriteBlocks` removes **both** blocks and writes a single new-sentinel eval-only block (self-healing, exit 0). Order-independent (`TestMigration_BothSentinelsPresentMergeToOne`, `TestMigration_BothSentinelsPresentReverseOrderMergeToOne`).
- **Partial/unclosed** (either sentinel open without close) → **refuse**, exit 2, no modification (`TestMigration_PartialUnclosedRefuses`, `TestMigration_PartialUnclosedLegacyRefuses`).

Migration preserves the symlink, trailing-newline, and never-creates-rc-files invariants (it goes through the same `rewriteBlocks` write path).

## Stale-export migration (change 0854)

When `--trust-tap` was removed, an existing rc block written by a former `--trust-tap` install may still carry a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` line. The next `shll shell-setup` run **actively strips it** — no special-casing required, because the existing rewrite/merge path does it for free:

- `findBlockWith` recognizes **only** the eval line as a managed line, so the export line is invisible to the block parse.
- `wantLines` returns just the eval line, so `desired` is the eval-only block.
- Because the block already exists, `runShellSetupDefault` takes the `rewriteBlocks` branch, which splices out the **entire** old block range (export line included) and inserts the freshly-built eval-only block — so the export line is dropped.

The export line was inert anyway (it only re-set Homebrew's default), so stripping it is pure cleanliness. The surrounding rc content is preserved. A plain re-run against a block that already contains only the eval line stays a byte-identical no-op (idempotency). Tests: `TestMigration_StripsStaleExportLine` (export+eval → eval-only, surrounding content preserved) and `TestMigration_StaleExportThenReRunIsNoop` (the second run is byte-identical with the "already installed" message). `TestTrustTapFlagRemoved` asserts cobra reports `--trust-tap` as an unknown flag and the Long help no longer mentions it.

## `--print` (dry-run)

`runShellSetupPrint(shell, rcPath, stdout, stderr)` is a dry-run: it resolves shell + rc file (still errors on a missing rc file — the user may be debugging exactly that), then writes the eval-only block (`buildBlock(shell)`) to stdout with no surrounding messages, and modifies **no file**. (Change 0854 removed the `trustTap` parameter and the combined-block print path — there is now only the eval-only block.)

## Symlink-preservation invariants

Two distinct write strategies, depending on whether the operation is read-modify-write:

### Append (fresh block): plain `O_APPEND`

`appendBlock` opens the rc file with `os.OpenFile(rcPath, os.O_WRONLY|os.O_APPEND, 0)` — no `O_CREATE`, no perm bits. Plain `O_APPEND` follows symlinks to the underlying real file and writes there, so a `~/.zshrc` symlink to `~/dotfiles/zshrc` (chezmoi, dotbot, stow, yadm) stays a symlink and the dotfile-manager source-of-truth file receives the appended block. Per POSIX, `write()` calls under `PIPE_BUF` (4 KiB on Linux, 512 bytes on macOS) are atomic with `O_APPEND`; the sentinel block is well under both limits. `TestInstall_PreservesSymlink` asserts the symlink stays a symlink.

### In-place rewrite + uninstall: `EvalSymlinks` → `O_TRUNC`

Both `rewriteBlocks` (in-place install / migration / both-sentinels merge) and `runShellSetupUninstall` are read-modify-write, so they cannot use `O_APPEND`. The mitigation:

1. Compute the modified in-memory content (splice out existing block range(s); for rewrite, insert the merged block at the earliest anchor).
2. Resolve the symlink chain: `resolved, _ := filepath.EvalSymlinks(rcPath)`.
3. Truncate-write the modified content to the *resolved* real path: `os.OpenFile(resolved, os.O_WRONLY|os.O_TRUNC, 0)`.

This preserves the user's symlink at the original path (it still points at the same real file) while the underlying source-of-truth file is updated — avoiding the `os.Rename`-of-temp-file hazard that would replace the symlink with a regular file. `TestUninstall_PreservesSymlink` asserts the symlink stays a symlink and the real file's block is removed.

## "shll never creates rc files" invariant

The default-install and `--print` paths both `os.Stat` the rc file and return `errExitCode{code:2, ...}` when it does not exist. They never call `O_CREATE`. The error message branches on whether the user passed `--rc-file`:

- Without `--rc-file`: `shll shell-setup: <path> does not exist. shll won't create rc files. Create it first, or pass --rc-file <path>.`
- With `--rc-file`: `shll shell-setup: <path> does not exist.` — no boilerplate, since the user explicitly named the path.

A missing rc file is a meaningful signal — custom `$ZDOTDIR`, dotfile manager pending `apply`, non-standard layout — and creating it would mask real configuration issues. The `--uninstall` path treats a missing rc file as benign ("nothing to uninstall", exit 0, stderr-only message).

## Trailing-newline guard

`appendBlock` prepends `\n` to the block exactly when the existing content is non-empty AND its last byte is not `\n`:

```go
if len(content) > 0 && content[len(content)-1] != '\n' {
    block = append([]byte("\n"), block...)
}
```

This prevents the open sentinel from landing on the same line as the user's previous content (e.g. `export FOO=bar# >>> shll >>>`). Empty files require no leading `\n` — a stray blank line at the top of an otherwise empty rc file would be visible noise. `TestInstall_TrailingNewlineGuard` and `TestInstall_EmptyFileNoLeadingNewline` pin both branches. (The guard lives in the append path only; the rewrite path reconstructs content around an existing block whose surrounding newlines are already settled.)

## Uninstall: whole-block removal, both sentinels

`runShellSetupUninstall(shell, rcPath, stdout, stderr)` removes the **entire** shll-managed block (both managed lines, both sentinels) in one operation:

- It recognizes BOTH the new `# >>> shll >>>` sentinel AND a legacy `# >>> shll shell-init >>>` block (so users who never re-installed can still uninstall), via `locateBlock`.
- It splices out every located block (later range first), then `EvalSymlinks`→`O_TRUNC`-writes the result. Both-blocks-present removes both.
- It runs **no Homebrew command at all** — `shell-setup` is pure file I/O (change 0854; there is no longer any trust state for it to touch, and any stale `export` line inside the block is removed with the block). The `shell` argument is unused (sentinels are shell-agnostic).
- Missing rc file or no block present → benign no-op message, exit 0.

On success: `Removed shll shell integration from <path>.` Tests: `TestUninstall_RemovesBlock` (new), `TestUninstall_RemovesLegacyBlock`, `TestUninstall_RemovesBothSentinelBlocks`, `TestUninstall_RemovesStaleExportBlock` (a block still carrying a stale `export` line is removed whole), `TestUninstall_PreservesSymlink`, `TestUninstall_BlockAbsent`, `TestUninstall_RcAbsent`.

## Exit-code policy

Mirrors the convention `shll shell-init` already established — see [cli/commands](/cli/commands.md#exit-code-translation). Both `errSilent` and `errExitCode` from `main.go` are reused; no new sentinel types are introduced.

| Exit code | Conditions |
|-----------|------------|
| **0** | Block written/merged; per-line no-op (byte-identical block already present); stale-export stripped; legacy migration; `--print` succeeded; `--uninstall` removed block or no-op (block/file absent) |
| **1** | I/O failure (read, write, close, `EvalSymlinks`) — emitted via `errSilent` after the diagnostic is written to stderr by the subcommand |
| **2** | User-invocation error — missing/unsupported shell positional, `$SHELL` could not be inferred, rc file does not exist in default or `--print` mode, `--print` and `--uninstall` both supplied, **partial/unclosed sentinel block (refuse-to-modify)** — emitted via `errExitCode{code: 2, msg: ...}` |

`translateExit` in `main.go` writes the `errExitCode.msg` to stderr automatically; subcommand code does not echo it. For `errSilent`, the subcommand has already written its own diagnostic via `fmt.Fprintf(stderr, ...)` and `translateExit` adds nothing.

## Test seam

`shell_setup_test.go` (test-alongside, per `code-quality.md` `## Test Strategy`):

- **No `proc.Runner` fake — shell-setup invokes no subprocess (change 0854).** With `--trust-tap` and its ceremony seam removed, the command is pure file I/O, so every test goes through `runShellSetupCmd(t, argv)` (a fresh cobra command with `bytes.Buffer` writers) against a `t.TempDir()` rc file. The prior trust-path tests (`TestTrustTap_*`, `TestBuildBlock_CombinedTrust`, `TestPrintTrustTap_*`, `TestMigration_*OnTrustTap`, the `installTrustSuccessRunner` helper) are gone; the test file no longer imports `internal/proc`.
- **`t.TempDir()`** for every rc-file test — the user's real `~/.zshrc` / `~/.bashrc` / `~/.bash_profile` is never touched.
- **`osGoos` swap** via `setOsGoos(t, value)` for the macOS-vs-Linux bash defaults. Saves and restores the package-level variable through `t.Cleanup`.
- **`envFunc(map)`** — unit tests for `resolveShell` / `resolveRcFile` use a map-backed env lookup so they run without mutating process state.
- **`t.Setenv`** for end-to-end tests that go through the real cobra command.

Source-level guard: `TestNoProcImports` (`func TestNoProcImports` in `shell_setup_test.go`; its hardcoded filename argument was updated from `shell_install.go` to `shell_setup.go` by change ri3h) reads `shell_setup.go` as bytes and fails if the source contains `internal/proc` or `"os/exec"`. Change 0854 made it **stronger**: it additionally fails if the source still references the removed `ensureTrustFunc` seam (a regression that pulled subprocess work back toward this file). This is a defensive check protecting Constitution I scoping.

Alias-coverage guard: `TestRoot_ShellInstallAliasResolves` (`func TestRoot_ShellInstallAliasResolves` in `shell_setup_test.go`, added by change ri3h) asserts the backward-compat `shell-install` alias dispatches to the same `*cobra.Command` as the canonical `shell-setup` — it builds the root via `newRootCmd()` and checks `root.Find([]string{"shell-install"})` and `root.Find([]string{"shell-setup"})` return the identical command pointer (cobra's `Find` resolves aliases), and that the resolved command's `Name()` is `shell-setup`. The registration test was renamed `TestRoot_ShellInstallRegistered` → `TestRoot_ShellSetupRegistered` (cobra's `Name()` returns the first word of `Use`, now `shell-setup`).

(The whole-tap ceremony helpers `brewTrustTap`/`ensureTapTrust`/`trustHatchHint` and their `brew_test.go` tests were removed by change 0854; `brewTrustAvailable` survives — reused by `shll install`'s per-formula trust and `shll doctor`'s trust sub-check — and its `TestBrewTrustAvailable_*` tests remain.)

## Cross-references

- Read-only reuse by `doctor`: [cli/doctor](/cli/doctor.md) — `shll doctor`'s wiring check reuses `resolveShell`, `resolveRcFile`, `locateBlock`, and `blockMatch.hasEval` **strictly READ-ONLY** (it `os.ReadFile`s the rc file and inspects `hasEval`; it NEVER calls any of the write paths — `appendBlock`, `rewriteBlocks`, `buildBlockBody`). `doctor` never writes to, creates, or migrates the rc file.
- Where trust went (change 0854): per-formula Homebrew trust moved to `shll install` (the Homebrew-recommended granularity for third-party taps) — see [cli/install §per-formula trust before install](/cli/install.md#per-formula-trust-before-install-change-0854). The surviving `brewTrustAvailable` helper in `brew.go` is reused there and by `doctor`'s read-only trust sub-check ([cli/doctor §the trust sub-check](/cli/doctor.md#the-trust-sub-check-change-0854)). The constant `tapName` (`"sahil87/tap"`) survives in `tools.go` but is now used only by `doctor`'s tap-level trust check, not by any `shell-setup` ceremony.
- Subcommand registration and exit-code translation: [cli/commands](/cli/commands.md).
- The eval-line target: [cli/shell-init](/cli/shell-init.md) — `shell-setup` writes the line that `shell-init` produces output for.
- Subprocess execution: [internal/proc](/internal/proc.md) — `shell-setup` invokes **none** (it is pure file I/O); the `TestNoProcImports` guard pins this.
- Constitution I (Security First) → `shell_setup.go` is subprocess-free, enforced by the (now stronger) `TestNoProcImports` guard — since change 0854 there is no ceremony seam bridging to `brew.go` at all.
- Constitution VII (Minimal Surface Area) → change 0854 **removed** the `--trust-tap` flag (a net surface-area reduction); the `shell-setup` rename (change ri3h) keeps the `shell-install` alias without changing the count. Justifications recorded in the respective change intakes.
- Cross-Platform Behavior → the darwin-vs-other branch in `resolveRcFile` is the only platform-specific code path, isolated behind the `osGoos` package-level variable (`osGoos` is no longer used by any brew call — change 0854).
