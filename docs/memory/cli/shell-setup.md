# cli/shell-setup

`shll shell-setup [shell]` — maintains a single sentinel-wrapped shll-managed block in the user's shell rc file. The block holds the cross-tool `eval "$(shll shell-init <shell>)"` line and, when genuine Homebrew tap-trust is requested via `--trust-tap`, an `export HOMEBREW_REQUIRE_TAP_TRUST=1` policy line. Idempotent re-runs (per-line), optional `--print` (dry run) and `--uninstall` (removal) modes, the orthogonal `--trust-tap` selector, plus `--rc-file` as a universal escape hatch for non-standard layouts.

**Canonical name + back-compat alias.** `shell-setup` is the canonical command name (renamed from `shell-install` by change ri3h). `shell-install` is retained as a cobra alias (`Aliases: []string{"shell-install"}`) that dispatches to the same `*cobra.Command` — existing rc files, scripts, and muscle memory keep working with zero breakage. The rename was a full Go-identifier rename (file, factory `newShellSetupCmd`, run helpers, test file/helpers) off the `ShellInstall` stem; behavior is identical, only names/help/message-prefixes changed.

Source: `src/cmd/shll/shell_setup.go`. This file performs **file I/O only** and imports neither `internal/proc` nor `os/exec` (Constitution I scope is subprocess execution). The `--trust-tap` ceremony (the only subprocess work this command involves) is delegated to `brew.go` via a function-value seam — see [The ceremony seam](#the-trust-tap-flag-and-the-ceremony-seam). `TestNoProcImports` (`func TestNoProcImports` in `shell_setup_test.go`) enforces the no-import invariant by reading the source as bytes.

## Usage

```sh
shll shell-setup                         # auto-detect shell from $SHELL, ensure eval line in the block
shll shell-setup zsh                     # explicit shell
shll shell-setup --trust-tap             # full setup: brew trust ceremony + export line + eval line
shll shell-setup --trust-tap --print     # dry-run: print the combined block, change nothing, run no ceremony
shll shell-setup --print zsh             # dry-run: print the block to stdout, no file change
shll shell-setup --uninstall zsh         # remove the whole block from the rc file
shll shell-setup --rc-file <path>        # override rc-file derivation entirely
shll shell-install zsh                   # alias — back-compat, dispatches to the same command
```

The managed lines this command writes:

```
export HOMEBREW_REQUIRE_TAP_TRUST=1       # only with --trust-tap, only if the ceremony succeeded
eval "$(shll shell-init zsh)"             # always
```

The eval line is the cross-tool composition entry point — see [cli/shell-init](shell-init.md). `shell-setup` exists so the user does not have to know which rc file to paste it into, nor remember to dedupe on re-install. The export line opts the user into Homebrew's require-tap-trust mode (resolves the recurring "Tap sahil87/tap is allowed by default" warning).

## Behavior contract

`runShellSetup(ctx, args, rcFileFlag, printMode, uninstallMode, trustTap, ensureTrust, stdout, stderr)` (`shell_setup.go`, `runShellSetup`) is the implementation seam. The cobra `RunE` wrapper builds the writers, passes the production ceremony function `ensureTapTrust` (from `brew.go`) as the `ensureTrust` argument, and delegates. The dispatch sequence:

1. **Default `ctx`.** A nil context is replaced with `context.Background()` (the ceremony seam needs a context).
2. **Flag conflict.** If both `--print` and `--uninstall` are set → return `errExitCode{code: 2, msg: "shll shell-setup: --print and --uninstall are mutually exclusive"}`. Exit code **2**. `--trust-tap` is orthogonal and never participates in this guard (see below).
3. **Resolve shell.** Delegate to `resolveShell(args, os.Getenv)`.
4. **Resolve rc file.** If `--rc-file <path>` was passed, use it verbatim. Otherwise derive via `resolveRcFile(shell, os.Getenv)`.
5. **Mode dispatch.** `--print` → `runShellSetupPrint`; `--uninstall` → `runShellSetupUninstall`; otherwise → `runShellSetupDefault`. `trustTap` and (for the default path) `ensureTrust` thread through.

`--trust-tap` is an **orthogonal selector, not a dispatch mode**. `--print` and `--uninstall` are mutually-exclusive modes; `--trust-tap` composes with the default and `--print` paths (it is accepted alongside `--uninstall` too, but uninstall removes the whole block regardless and runs no ceremony). The `userProvidedPath bool` passed to `runShellSetupDefault` is `true` exactly when `--rc-file` was supplied — it controls whether the missing-rc-file error includes the "shll won't create rc files" hint.

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

The shll-managed block uses the **combined `# >>> shll >>>` / `# <<< shll <<<`** sentinel pair (note the close sentinel uses three `<` chars). It holds the union of managed lines that apply, in canonical **export-before-eval** order:

```
# >>> shll >>>
export HOMEBREW_REQUIRE_TAP_TRUST=1
eval "$(shll shell-init <shell>)"
# <<< shll <<<
```

When only one managed line applies (e.g. a plain `shll shell-setup` with no trust), only that line appears between the sentinels:

```
# >>> shll >>>
eval "$(shll shell-init <shell>)"
# <<< shll <<<
```

### Constants (top of `shell_setup.go`)

- `openSentinel = "# >>> shll >>>"` / `closeSentinel = "# <<< shll <<<"` — the **new combined** sentinels. Exact bytes are user contract (block location + uninstall removal both depend on literal matching).
- `legacyOpenSentinel = "# >>> shll shell-init >>>"` / `legacyCloseSentinel = "# <<< shll shell-init <<<"` — the **pre-rename** sentinels, recognized only for **migration** (install path) and **removal** (uninstall path) of pre-existing blocks. shll never *writes* the legacy sentinels.
- `evalLineFmt = `eval "$(shll shell-init %s)"`` — the eval body, with `%s` substituted by the resolved shell. `evalLine(shell)` formats it.
- `evalLinePrefix = `eval "$(shll shell-init`` — the shell-agnostic prefix used to recognize an existing eval line during a merge, regardless of which shell it was installed for.
- `exportTrustLine = "export HOMEBREW_REQUIRE_TAP_TRUST=1"` — the trust policy line. Written **only** alongside a successful ceremony (see degradation below).

### Block builders

- `buildBlockBody(lines []string) []byte` is the **single source of truth** for block contents: it wraps an ordered set of managed lines in the new sentinel pair, each line plus a trailing `\n`, ending with a single trailing `\n` after the close sentinel. It does **not** reorder or dedup — the upstream merge logic (`wantLines`) is responsible for canonical order and uniqueness.
- `buildBlock(shell) []byte` is the eval-only convenience builder (routes through `buildBlockBody([]string{evalLine(shell)})`); used by `--print` without `--trust-tap`.
- `wantLines(existing blockMatch, shell string, wantExport bool) []string` is the **per-line MERGE rule**: it returns `[export?, eval]` where the export line is included when `existing.hasExport || wantExport`. The eval line is **always** included (so an export-only block gains the eval line on a plain re-run, and degradation still yields working shell integration). Canonical order: export first, eval second.

Drift between the write, print, and migration paths is a defect — they all derive from the same constants via `buildBlockBody`. The block carries no "managed by shll, do not edit" line; the bookend sentinels are themselves the visual signal.

## Block location and parsing

`blockMatch` describes a located block: its inclusive byte range `[start, end)` (open sentinel through the trailing `\n` after the close sentinel) plus `hasExport` / `hasEval` flags extracted from the body.

- `findBlockWith(content, open, close) (m blockMatch, ok, partial bool)` locates a block for a given sentinel pair and parses which managed lines it carries (body lines are trimmed; `exportTrustLine` and any line with `evalLinePrefix` are recognized). It returns `partial=true` when the open sentinel is present but its matching close is **absent** — an unclosed/corrupted block.
- `locateBlock(content)` is the single entry point used by install and uninstall. It calls `findBlockWith` for **both** the new and legacy sentinels and returns `(newM, newOK, legacyM, legacyOK, partial)`, where `partial` is the OR of either sentinel being open-without-close.

## Idempotency invariant (now per-line)

Idempotency is **per-line**, not a single substring match. The desired block body is `buildBlockBody(wantLines(...))` — the **union** of (a) managed lines already present in any existing block and (b) lines this invocation adds (eval always; export when `--trust-tap` and the ceremony succeeded). A managed line already present is not duplicated.

The byte-identical no-op is detected in the **rewrite path** (`rewriteBlocks`): after splicing out existing block(s) and inserting the merged block, if `bytes.Equal(merged, content)` the file is left untouched, `shll shell-setup: already installed in <path> (no changes).` is written to stderr, and the command exits 0. So a full re-run of `shll shell-setup --trust-tap` against a block that already contains both managed lines (with the tap already trusted) is byte-identical before and after. `TestTrustTap_FullReRunIsByteIdenticalNoop` and `TestInstall_Idempotent` assert this with byte-equality.

> **Note on the append path:** `appendBlock` (the no-existing-block case) does not perform an equality short-circuit — there is no block to compare against, so it always writes a fresh block. The no-op semantics live in `rewriteBlocks`, which is the path any *re-run* takes (a block now exists).

## Install path: per-line merge

`runShellSetupDefault(ctx, shell, rcPath, userProvidedPath, trustTap, ensureTrust, stdout, stderr)` flow:

1. `os.Stat` the rc file (**no `O_CREATE`** ever). Missing → `errExitCode{code:2}` (see [never creates rc files](#shll-never-creates-rc-files-invariant)). Other stat error → `errSilent` (exit 1).
2. `os.ReadFile` the content.
3. `locateBlock(content)`. If `partial` (open-without-close, either sentinel) → **refuse**: return `errExitCode{code:2, msg: "...has an shll block with an opening sentinel but no matching closing sentinel. Refusing to modify a corrupted block — fix or remove it manually, then re-run."}`. This is a deliberate divergence from the legacy short-circuit-as-"already-installed" behavior (guessing the bounds of an unclosed block risks corrupting the rc file).
4. **Run the ceremony** (only when `trustTap`): call `ensureTrust(ctx)` → `(writeExport, diag)`. `wantExport = writeExport`. Any non-empty `diag` is printed to stderr. (The ceremony runs *before* composing the block, because its outcome decides whether the export line belongs in the desired set.)
5. **Compute the union.** Synthesize an `existing` blockMatch whose `hasExport`/`hasEval` are the OR across the new and legacy blocks (folding a both-sentinels-present state into one). `desired = buildBlockBody(wantLines(existing, shell, wantExport))`.
6. **Write.**
   - No existing block (`!newOK && !legacyOK`) → `appendBlock` (plain `O_APPEND`, symlink-safe).
   - One or both blocks exist → `rewriteBlocks` (read-modify-write → `EvalSymlinks`→`O_TRUNC`).

`appendBlock` applies the trailing-newline guard then `O_APPEND`-writes the block; on success prints `Installed shll shell integration to <path>. Restart your shell or run: source <path>`.

`rewriteBlocks` splices out every existing shll block (new and/or legacy), inserts the merged block at the **earliest** removed block's position, and either no-ops (byte-identical) or `EvalSymlinks`→`O_TRUNC`-writes the merged content to the resolved real path. Both the migration rewrite and the both-sentinels-present merge route through here. Removal of ranges is done later-range-first so earlier indices stay valid (the two sentinels never overlap).

Covered scenarios (all exit 0): already-set-up user adds trust (export merged into an eval-only block), trust-first user later adds shell-init (eval merged into an export-only block), full re-run no-op, plain install writes the new-sentinel eval-only block.

## Migration: legacy → new sentinel

A legacy `# >>> shll shell-init >>>` block is migrated **in place** on the next install:

- **Legacy-only present** → `locateBlock` finds it via `legacyOK`, `runShellSetupDefault` takes the rewrite branch, splices out the legacy block, and writes the merged block under the **new** sentinel — carrying the legacy eval line forward and merging the export line when `--trust-tap` succeeded. No legacy sentinel remains. (`TestMigration_LegacyEvalOnlyMigratesOnTrustTap`, `TestMigration_LegacyEvalOnlyMigratesOnPlainInstall`.)
- **Both sentinels present** (new + legacy, e.g. hand-edited) → `rewriteBlocks` removes **both** blocks and writes a single new-sentinel block with the union of their managed lines (self-healing, exit 0). Order-independent (`TestMigration_BothSentinelsPresentMergeToOne`, `TestMigration_BothSentinelsPresentReverseOrderMergeToOne`).
- **Partial/unclosed** (either sentinel open without close) → **refuse**, exit 2, no modification (`TestMigration_PartialUnclosedRefuses`, `TestMigration_PartialUnclosedLegacyRefuses`).

Migration preserves the symlink, trailing-newline, and never-creates-rc-files invariants (it goes through the same `rewriteBlocks` write path).

## The `--trust-tap` flag and the ceremony seam

`--trust-tap` does **full genuine-trust setup**: it ensures both the eval and export lines are in the block **and** runs the `brew trust` ceremony. The two halves must travel together — the export (policy) line without a backing trust record would cause brew to **block** the tap (strictly worse than the warning), and the trust record without the policy line leaves the warning in place.

### Why the ceremony lives in `brew.go`, not `shell_setup.go`

`shell_setup.go` is pinned to file-I/O-only by `TestNoProcImports` (and by its documented character). The subprocess work — capability probe + ceremony — lives in `brew.go`, which legitimately imports `internal/proc`. The two files are bridged by a **function-value seam**:

- `shell_setup.go` declares the seam type `ensureTrustFunc = func(ctx context.Context) (writeExport bool, diag string)` and a `runShellSetup` parameter `ensureTrust ensureTrustFunc`.
- The cobra `RunE` passes `ensureTapTrust` (defined in `brew.go`) as the production implementation.
- This keeps `shell_setup.go` free of any proc/exec import while letting tests drive the ceremony by installing a fake `proc.Runner` and exercising `ensureTapTrust` through it.

### Ceremony helpers in `brew.go`

- `tapName = "sahil87/tap"` (in `tools.go`) — the tap argument for `brew trust --tap`. **Distinct from `formulaPrefix = "sahil87/tap/"`** (trailing slash, used to build formula references like `sahil87/tap/shll`). The trust ceremony acts on the *tap*, not a formula, so it must NOT carry the trailing slash. Named constant per code-quality (no magic strings).
- `brewTrustAvailable(ctx) bool` — capability-probes via `proc.Run(ctx, brewBinary, "trust", "--help")`. Returns false on any error (brew absent → `proc.ErrNotFound`; `trust` unrecognized on an older brew → non-zero exit → non-nil error). On exit 0 it additionally requires the help output to contain `"trust"` (belt-and-suspenders). Mirrors the read-only `<tool> update --help` substring probe in `update.go` — the probe is the contract, never a version-floor check.
- `brewTrustTap(ctx) (int, error)` — runs `brew trust --tap sahil87/tap` (using `tapName`) via `proc.RunForeground` (foregrounded so the user sees brew's "Trusted tap" / "Already trusted tap" output) and returns the exit code/error. Invoked **unconditionally** during `--trust-tap`: `brew trust`/`untrust` are idempotent (verified on brew 5.1.14, re-run exits 0), so no trust pre-check is needed.
- `ensureTapTrust(ctx) (writeExport bool, diag string)` — the seam target. The full ladder:
  1. `!hasBrew(ctx)` → `(false, "...Homebrew is not installed...")`.
  2. `!brewTrustAvailable(ctx)` → `(false, "...does not support brew trust (requires a newer Homebrew)...")`.
  3. `brewTrustTap(ctx)` returns err or non-zero code → `(false, "...brew trust --tap sahil87/tap did not succeed...")`.
  4. Ceremony exit 0 → `(true, "")`.

  Every degraded `diag` names the lighter escape hatches via `trustHatchHint = "set HOMEBREW_NO_REQUIRE_TAP_TRUST=1 or HOMEBREW_NO_ENV_HINTS=1 to silence the warning instead"`.

### Degradation behavior (Constitution V)

When the ceremony degrades (brew absent / `trust` unavailable / ceremony non-zero or error), `ensureTapTrust` returns `writeExport=false` with a diagnostic. `runShellSetupDefault` then **skips the export line but still writes the eval line** (pure file I/O, always succeeds) and **exits 0** (degraded success). The user keeps working shell integration; only the trust half is skipped. Tests: `TestTrustTap_DegradesWhenTrustUnavailable`, `TestTrustTap_DegradesWhenBrewAbsent`, `TestTrustTap_DegradesWhenCeremonyNonZero` (all assert eval written, no export, exit 0); `brew_test.go` `TestEnsureTapTrust_*` pin the ladder.

### `--print` with `--trust-tap`

`runShellSetupPrint(shell, rcPath, trustTap, stdout, stderr)` is a dry-run: it resolves shell + rc file (still errors on a missing rc file — the user may be debugging exactly that), then writes the block to stdout with no surrounding messages, runs **no ceremony**, and modifies **no file**. With `--trust-tap` it prints the *combined* block (export before eval) that a successful `--trust-tap` install would produce — it cannot probe brew, so it optimistically shows both lines. (`TestPrintTrustTap_CombinedNoFileNoCeremony` asserts no file write and no ceremony invocation.)

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
- It does **NOT** run `brew untrust` — the trust record is inert without the policy line and is the user's to reverse (`brew untrust` is idempotent). The `shell` argument is unused (sentinels are shell-agnostic).
- Missing rc file or no block present → benign no-op message, exit 0.

On success: `Removed shll shell integration from <path>.` Tests: `TestUninstall_RemovesBlock` (new), `TestUninstall_RemovesLegacyBlock`, `TestUninstall_RemovesBothSentinelBlocks`, `TestUninstall_DoesNotUntrust`, `TestUninstall_PreservesSymlink`, `TestUninstall_BlockAbsent`, `TestUninstall_RcAbsent`.

## Exit-code policy

Mirrors the convention `shll shell-init` already established — see [cli/commands](commands.md#exit-code-translation). Both `errSilent` and `errExitCode` from `main.go` are reused; no new sentinel types are introduced.

| Exit code | Conditions |
|-----------|------------|
| **0** | Block written/merged; per-line no-op (byte-identical block already present); `--print` succeeded; `--uninstall` removed block or no-op (block/file absent); `--trust-tap` **degraded success** (trust unavailable/brew absent/ceremony non-zero → eval written, export skipped) |
| **1** | I/O failure (read, write, close, `EvalSymlinks`) — emitted via `errSilent` after the diagnostic is written to stderr by the subcommand |
| **2** | User-invocation error — missing/unsupported shell positional, `$SHELL` could not be inferred, rc file does not exist in default or `--print` mode, `--print` and `--uninstall` both supplied, **partial/unclosed sentinel block (refuse-to-modify)** — emitted via `errExitCode{code: 2, msg: ...}` |

`translateExit` in `main.go` writes the `errExitCode.msg` to stderr automatically; subcommand code does not echo it. For `errSilent`, the subcommand has already written its own diagnostic via `fmt.Fprintf(stderr, ...)` and `translateExit` adds nothing.

## Test seam

`shell_setup_test.go` (test-alongside, per `code-quality.md` `## Test Strategy`):

- **`proc.Runner` fake — now used for the trust-path tests.** The trust-path cases drive the ceremony seam by installing a fake runner (`installFakeRunner(t, f)`) and exercising the production `ensureTapTrust`. `installTrustSuccessRunner(t)` (`func installTrustSuccessRunner` in `shell_setup_test.go`) is the common "ceremony succeeds" fake; bespoke `fakeRunner`s cover the degradation paths (`ErrNotFound`, unrecognized `trust`, non-zero exit). The *non-trust* tests still need no fake — they go through `runShellSetupCmd(t, argv)` which builds a fresh cobra command with `bytes.Buffer` writers.
  > This is a change from the prior memory: the **test file** now imports `internal/proc`. The **production file** `shell_setup.go` still imports neither `internal/proc` nor `os/exec` — that invariant is unchanged and is the one `TestNoProcImports` guards.
- **`t.TempDir()`** for every rc-file test — the user's real `~/.zshrc` / `~/.bashrc` / `~/.bash_profile` is never touched.
- **`osGoos` swap** via `setOsGoos(t, value)` for the macOS-vs-Linux bash defaults. Saves and restores the package-level variable through `t.Cleanup`.
- **`envFunc(map)`** — unit tests for `resolveShell` / `resolveRcFile` use a map-backed env lookup so they run without mutating process state.
- **`t.Setenv`** for end-to-end tests that go through the real cobra command.

Source-level guard: `TestNoProcImports` (`func TestNoProcImports` in `shell_setup_test.go`; its hardcoded filename argument was updated from `shell_install.go` to `shell_setup.go` by change ri3h) reads `shell_setup.go` as bytes and fails if the source contains `internal/proc` or `"os/exec"`. This is a defensive check protecting Constitution I scoping — any future regression that pulls subprocess execution directly into this file (rather than via the `ensureTrustFunc` seam) will fail at test time.

Alias-coverage guard: `TestRoot_ShellInstallAliasResolves` (`func TestRoot_ShellInstallAliasResolves` in `shell_setup_test.go`, added by change ri3h) asserts the backward-compat `shell-install` alias dispatches to the same `*cobra.Command` as the canonical `shell-setup` — it builds the root via `newRootCmd()` and checks `root.Find([]string{"shell-install"})` and `root.Find([]string{"shell-setup"})` return the identical command pointer (cobra's `Find` resolves aliases), and that the resolved command's `Name()` is `shell-setup`. The registration test was renamed `TestRoot_ShellInstallRegistered` → `TestRoot_ShellSetupRegistered` (cobra's `Name()` returns the first word of `Use`, now `shell-setup`).

Ceremony helpers are unit-tested in `brew_test.go` (added by this change): `TestBrewTrustAvailable_*`, `TestBrewTrustTap_BuildsTapArg` (asserts `tapName`, not a formula reference), `TestBrewTrustTap_SurfacesNonZeroExit` / `_SurfacesError`, and `TestEnsureTapTrust_*` (the degradation ladder).

## Cross-references

- Subcommand registration and exit-code translation: [cli/commands](commands.md). The ceremony constant `tapName` lives in `tools.go` alongside `formulaPrefix`; `brew.go` gained `brewTrustAvailable`, `brewTrustTap`, `ensureTapTrust`, and `trustHatchHint`.
- The eval-line target: [cli/shell-init](shell-init.md) — `shell-setup` writes the line that `shell-init` produces output for.
- Subprocess execution: [internal/proc](../internal/proc.md) — the ceremony uses `proc.Run` (probe) and `proc.RunForeground` (ceremony), routed entirely through `brew.go`; `shell_setup.go` reaches them only via the `ensureTrustFunc` seam.
- Constitution I (Security First) → the `brew trust` ceremony routes through `internal/proc` from `brew.go`; `shell_setup.go` remains subprocess-free (the `TestNoProcImports` guard documents the boundary, and the function-value seam is how the ceremony is threaded in without breaking it).
- Constitution V (Graceful Degradation) → `--trust-tap` degrades to eval-line-only (exit 0) when trust cannot be recorded, rather than hard-failing.
- Constitution VII (Minimal Surface Area) → `--trust-tap` is a flag on the existing `shell-setup` (formerly `shell-install`), not a new top-level command; the `shell-setup` rename (change ri3h) adds the `shell-install` alias without changing the surface-area count. Justifications recorded in the respective change intakes.
- Cross-Platform Behavior → the darwin-vs-other branch in `resolveRcFile` is the only platform-specific code path, isolated behind the `osGoos` package-level variable.
