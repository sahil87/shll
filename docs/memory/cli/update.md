# cli/update

`shll update` — composes `brew update`, a `shll`-self-upgrade step, and per-tool `brew upgrade` calls to refresh every installed sahil87 tool.

Source: `src/cmd/shll/update.go`, with shared brew helpers in `src/cmd/shll/brew.go`.

## Behavior contract

The full happy/unhappy paths, in the order `runUpdate` evaluates them (`src/cmd/shll/update.go`):

1. **Brew missing.** If `hasBrew(ctx)` returns false, write `shll update requires Homebrew. Install from https://brew.sh` to stderr and return `errSilent`. Exit code: 1. The literal hint string is `brewMissingHint` in `src/cmd/shll/brew.go:15` — do not edit one without the other (the spec scenario asserts it verbatim).

2. **Filter installed roster.** Iterate `Roster` (in order) calling `isInstalled(ctx, t.Formula)`; collect the matches into a local `installed` slice.

3. **Detect shll-self brew install.** Call `isInstalled(ctx, shllFormula)` (where `shllFormula = "sahil87/tap/shll"` in `src/cmd/shll/brew.go`). The result drives whether the self-upgrade step in (6) runs.

4. **Nothing-to-do → short-circuit.** If `len(installed) == 0` AND shll itself is not brew-installed, write `No sahil87 tools installed.` to stdout and return nil. Exit code: 0. Critically, **`brew update --quiet` is NOT invoked in this branch** — see Design Decision #9 below. Note: when shll itself is brew-installed but no roster tools are, the short-circuit does NOT fire — the run proceeds and only self-upgrades shll. Pinned by `TestUpdate_OnlyShllInstalled`.

5. **Refresh metadata once.** `proc.RunForeground(ctx, "brew", "update", "--quiet")` — foreground so users see brew's progress. On error, write `shll update: brew update failed: <err>` to stderr and return `errSilent` (exit 1).

6. **shll self-upgrade (when brew-installed).** If step (3) reported shll itself as brew-installed, run `proc.RunForeground(ctx, "brew", "upgrade", shllFormula)` *before* the roster loop. See [shll self-upgrade](#shll-self-upgrade) below for the rationale and edge cases. Failures here go through the same best-effort `anyFailed` path as roster failures.

7. **Sequential per-tool upgrade.** For each installed tool in roster order, run `proc.RunForeground(ctx, "brew", "upgrade", t.Formula)`. Best-effort across the roster: on per-tool failure (transport error or non-zero exit), set `anyFailed = true` and `continue` — never abort the loop. After the loop, if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0).

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All upgrades succeeded (or nothing-to-do branch) | 0 |
| `brew` not on PATH | 1 (via `errSilent`, hint already on stderr) |
| `brew update --quiet` failed | 1 (via `errSilent`) |
| `shll` self-upgrade failed | 1 (via `errSilent`, after roster also attempted) |
| Any per-tool `brew upgrade` failed | 1 (via `errSilent`, after all tools attempted) |

## shll self-upgrade

`shll update` self-upgrades `shll` itself before iterating the roster. The behavior is contingent on detection:

- **Brew-installed shll** (`brew install sahil87/tap/shll`) → self-upgrade runs as `brew upgrade sahil87/tap/shll` immediately after the metadata refresh, before any roster upgrade. The mid-run binary on disk gets replaced; the running process keeps its mapped image and finishes normally; a follow-up `shll` invocation picks up the new binary. Pinned by `TestUpdate_AllInstalled` and `TestUpdate_SelfUpgradeOrdering`.
- **Dev build** (e.g. `go install ./cmd/shll`) → `isInstalled(ctx, shllFormula)` returns false, the self-upgrade is skipped silently, and the roster loop proceeds normally. Pinned by `TestUpdate_SelfNotBrewInstalled`. This avoids `brew upgrade` errors that would otherwise fire on a non-brew-managed binary (Constitution V — Graceful Degradation).

The `shllFormula` constant (`src/cmd/shll/brew.go`) is the single source of truth for the self-upgrade target — `shll` is intentionally **not** added to `Roster`. `Roster` is the *sub-tool* roster (Constitution III — Tool Roster Source of Truth); commingling shll itself would distort `shll version`'s output (which already prints shll separately) and `shll shell-init`'s iteration semantics.

Ordering rationale: self-upgrade runs *before* the roster loop so the on-disk binary is updated as early as possible. Subsequent operations within the same invocation still execute the original mapped image (POSIX semantics — replacing the file on disk doesn't affect a running process), so there is no risk of partial-version mixing within one run.

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

### #9 `shll update` skips `brew update --quiet` when there is nothing to upgrade

> *Why*: The metadata refresh is only useful as a precursor to upgrades. When there is nothing to upgrade (no roster tools installed AND shll itself not brew-installed), the refresh is pure latency for no benefit; the user-visible message (`No sahil87 tools installed.`) is the primary signal and should print quickly.
> *Rejected*: refreshing brew metadata anyway. Considered for "freshness on every invocation" but rejected — `shll update` is not a brew metadata refresh tool, it's a sahil87 toolkit upgrader. Users who want a refresh have `brew update` directly.

This is the reason for the early short-circuit in step 4 above. The check is now a logical AND — both the roster set and shll-itself must be empty/uninstalled — so a brew-installed shll with zero roster tools still proceeds (and just self-upgrades). Tests assert `brew update` is NOT in the recorded call list only when the full nothing-to-do branch fires (`TestUpdate_NoToolsInstalled`).

## Test seam

All `update_test.go` tests inject a fake via `proc.Runner` (`installFakeRunner` t.Cleanup helper at `src/cmd/shll/update_test.go:33`). No real brew subprocess is ever spawned. The fake records every `proc.Request` so tests assert: which formulas were queried, which upgrades ran, the order of operations, the exit code, and the captured stdout/stderr writers.

Covered scenarios (`src/cmd/shll/update_test.go`):

- `TestUpdate_BrewMissing` — `proc.Run("brew", "--version")` returns `ErrNotFound` → stderr hint, exit 1.
- `TestUpdate_AllInstalled` — shll itself + full roster installed → 1 self-upgrade + 6 roster upgrades, all succeed, exit 0.
- `TestUpdate_SelfUpgradeOrdering` — pin that the shll self-upgrade call appears before the first roster upgrade in the recorded sequence.
- `TestUpdate_SelfNotBrewInstalled` — dev build (shll not brew-installed) → self-upgrade skipped, roster upgrades still happen.
- `TestUpdate_OnlyShllInstalled` — shll brew-installed but no roster tools → metadata refresh runs, self-upgrade runs, no roster upgrades, no short-circuit message, exit 0.
- `TestUpdate_NoToolsInstalled` — neither shll nor any roster tool is brew-installed → "No sahil87 tools installed.", **no `brew update`**, no upgrade calls, exit 0.
- `TestUpdate_PartialInstalled` — only `hop` and `wt` installed (shll not brew-installed in this fake) → only those upgraded, no warning for others.
- `TestUpdate_BrewUpdateFails` — `brew update --quiet` exits non-zero → stderr "brew update failed", no upgrades attempted (including shll-self), exit 1.
- `TestUpdate_OneUpgradeFails` — first roster upgrade exits non-zero → loop continues; total upgrade attempts = `len(Roster) + 1` (self + roster).

## Cross-references

- Subprocess wrapper conventions: [internal/proc](../internal/proc.md).
- The hardcoded roster: [cli/commands](commands.md#hardcoded-tool-roster).
- Constitution III (Wrap, Don't Reinvent) and IV (Composition, Not Replacement).
- Constitution V (Graceful Degradation) — uninstalled tools are skipped silently in step 2.
