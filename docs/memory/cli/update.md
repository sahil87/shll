# cli/update

`shll update` — composes `brew update` and per-tool `brew upgrade` calls to refresh every installed sahil87 tool.

Source: `src/cmd/shll/update.go`, with shared brew helpers in `src/cmd/shll/brew.go`.

## Behavior contract

The full happy/unhappy paths, in the order `runUpdate` evaluates them (`src/cmd/shll/update.go:32`):

1. **Brew missing.** If `hasBrew(ctx)` returns false, write `shll update requires Homebrew. Install from https://brew.sh` to stderr and return `errSilent`. Exit code: 1. The literal hint string is `brewMissingHint` in `src/cmd/shll/brew.go:15` — do not edit one without the other (the spec scenario asserts it verbatim).

2. **Filter installed roster.** Iterate `Roster` (in order) calling `isInstalled(ctx, t.Formula)`; collect the matches into a local `installed` slice.

3. **Zero installed → short-circuit.** If `len(installed) == 0`, write `No sahil87 tools installed.` to stdout and return nil. Exit code: 0. Critically, **`brew update --quiet` is NOT invoked in this branch** — see Design Decision #9 below.

4. **Refresh metadata once.** `proc.RunForeground(ctx, "brew", "update", "--quiet")` — foreground so users see brew's progress. On error, write `shll update: brew update failed: <err>` to stderr and return `errSilent` (exit 1).

5. **Sequential per-tool upgrade.** For each installed tool in roster order, run `proc.RunForeground(ctx, "brew", "upgrade", t.Formula)`. Best-effort across the roster: on per-tool failure (transport error or non-zero exit), set `anyFailed = true` and `continue` — never abort the loop. After the loop, if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0).

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All upgrades succeeded (or zero-installed branch) | 0 |
| `brew` not on PATH | 1 (via `errSilent`, hint already on stderr) |
| `brew update --quiet` failed | 1 (via `errSilent`) |
| Any per-tool `brew upgrade` failed | 1 (via `errSilent`, after all tools attempted) |

## Detection

`isInstalled(ctx, formula)` in `src/cmd/shll/brew.go:39` is the single source of truth for "is this brew formula installed":

- Calls `brew list --formula --versions <formula>` via `proc.Run` (capture transport).
- Returns `err == nil` — `brew list --versions <formula>` exits 0 when installed (with the version on stdout) and 1 when not. We don't parse stdout; the exit code is sufficient.

Constraints (Design Decision #2):

- **No regex** over plain `brew list` output. The `code-quality.md` anti-pattern explicitly forbids this.
- **No symlink-target inspection** (hop's `/Cellar/` trick). That works for the running binary only; we are querying *other* tools' install status.
- **No hardcoded `/opt/homebrew` or `/usr/local`** paths anywhere — the brew CLI is always invoked through PATH lookup via `exec`.

`hasBrew(ctx)` in `src/cmd/shll/brew.go:20` runs `brew --version` via `proc.Run` and returns true unless the error wraps `proc.ErrNotFound`. Any other brew failure (e.g. brew exits non-zero) still implies brew is installed — graceful degradation: only `ErrNotFound` is the "missing" signal.

## Foreground vs capture

| Subprocess | Transport | Why |
|------------|-----------|-----|
| `brew --version` (in `hasBrew`) | `proc.Run` (capture) | Internal probe; user does not need to see output. |
| `brew list --formula --versions <formula>` (in `isInstalled`) | `proc.Run` (capture) | Same — it's a probe, not user-facing. |
| `brew update --quiet` | `proc.RunForeground` | Brew's progress streamed to user's terminal. |
| `brew upgrade <formula>` | `proc.RunForeground` | Same — preserves brew's colored progress output. |

This split is a Constitution-aligned choice: probes capture (so shll can branch on the result), user-visible operations foreground (so the user sees brew working).

## Sequential, not parallel

`runUpdate` uses a plain `for` loop with synchronous `proc.RunForeground`. No goroutines (Design Decision #3):

- Brew serializes most internal operations behind its own lock; concurrency would not speed anything up.
- Parallel foregrounded subprocesses would interleave output incomprehensibly.
- `TestUpdate_OneUpgradeFails` asserts the loop continues through all six roster entries even when the first one fails.

## Spec-locked Design Decisions for this subcommand

These are reproduced verbatim from `spec.md` and lock the v0.1.0 contract.

### #2 Installed detection via `brew list`, not symlink resolution

> *Why*: `brew list --formula --versions sahil87/tap/<formula>` is the right primitive for querying *other* tools' install status. Hop's `/Cellar/` symlink trick works for the running tool only.
> *Rejected*: parsing plain `brew list` output (regex-fragile, see code-quality.md anti-pattern); inspecting filesystem paths directly (Constitution-violating hardcoded `/opt/homebrew` style paths).

### #3 Sequential brew upgrades

> *Why*: Brew serializes most internal operations behind its own lock; parallelism risks confusing interleaved output and lock contention with no measurable speedup.
> *Rejected*: parallel goroutine-per-tool. Real brew operations are I/O-bound on the single brew lock, so concurrency would not help.

### #9 `shll update` skips `brew update --quiet` when no roster tools are installed

> *Why*: The metadata refresh is only useful as a precursor to upgrades. When there is nothing to upgrade, the refresh is pure latency for no benefit; the user-visible message (`No sahil87 tools installed.`) is the primary signal and should print quickly.
> *Rejected*: refreshing brew metadata anyway. Considered for "freshness on every invocation" but rejected — `shll update` is not a brew metadata refresh tool, it's a sahil87 toolkit upgrader. Users who want a refresh have `brew update` directly.

This is the reason for the early short-circuit in step 3 above. Tests assert `brew update` is NOT in the recorded call list when the installed set is empty.

## Test seam

All `update_test.go` tests inject a fake via `proc.Runner` (`installFakeRunner` t.Cleanup helper at `src/cmd/shll/update_test.go:33`). No real brew subprocess is ever spawned. The fake records every `proc.Request` so tests assert: which formulas were queried, which upgrades ran, the order of operations, the exit code, and the captured stdout/stderr writers.

Covered scenarios (`src/cmd/shll/update_test.go`):

- `TestUpdate_BrewMissing` — `proc.Run("brew", "--version")` returns `ErrNotFound` → stderr hint, exit 1.
- `TestUpdate_HappyPath` — full roster installed → six `brew upgrade` calls, all succeed, exit 0.
- `TestUpdate_NoneInstalled` — `brew list` always exit-1 → "No sahil87 tools installed.", **no `brew update`**, exit 0.
- `TestUpdate_PartialInstalled` — only `hop` and `wt` installed → only those upgraded, no warning for others.
- `TestUpdate_OneUpgradeFails` — first upgrade exits non-zero → loop continues, exit 1.

## Cross-references

- Subprocess wrapper conventions: [internal/proc](../internal/proc.md).
- The hardcoded roster: [cli/commands](commands.md#hardcoded-tool-roster).
- Constitution III (Wrap, Don't Reinvent) and IV (Composition, Not Replacement).
- Constitution V (Graceful Degradation) — uninstalled tools are skipped silently in step 2.
