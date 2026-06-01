# cli/update

`shll update` — refreshes brew metadata once, self-upgrades `shll`, then upgrades every installed sahil87 tool by **delegating to that tool's own `update` subcommand** (falling back to `brew upgrade` only for a tool that exposes no `update`).

Source: `src/cmd/shll/update.go`, with shared brew helpers in `src/cmd/shll/brew.go`.

> **Delegation, not `brew upgrade` (change cczs).** Earlier versions upgraded each roster tool with `brew upgrade sahil87/tap/<formula>` directly. That reproduced only the binary swap and silently dropped each tool's own post-upgrade side effects — most visibly, `rk update`'s daemon restart (`daemon.RestartWithBinary`), which lives in rk's CLI rather than a brew post-install hook. The result was a stale running rk daemon after `shll update`. `shll update` now delegates to `<tool> update` so each tool stays authoritative over its own upgrade + side effects (Constitution IV — Composition, Not Replacement; Constitution III — Wrap, Don't Reinvent).

## Behavior contract

The full happy/unhappy paths, in the order `runUpdate` evaluates them (`src/cmd/shll/update.go:66`):

1. **Brew missing.** If `hasBrew(ctx)` returns false, write `shll update requires Homebrew. Install from https://brew.sh` to stderr and return `errSilent`. Exit code: 1. The literal hint string is `brewMissingHint` in `src/cmd/shll/brew.go:17` — do not edit one without the other (the spec scenario asserts it verbatim). The status line (step 2) is NOT printed before this bail-out — brew presence is checked first (`TestUpdate_BrewMissing` asserts empty stdout).

2. **Instant status line.** Write `Checking installed sahil87 tools…` to stdout (named constant `updateStatusLine`, `src/cmd/shll/update.go:20`). This is the first visible byte, printed **unconditionally** before any probing — including before the nothing-to-do short-circuit — so the user gets immediate feedback during the (now concurrent) probe phase rather than staring at a blank terminal.

3. **Parallel read-only capability probes.** `probeRoster(ctx)` (`src/cmd/shll/update.go:175`) dispatches one goroutine per roster tool and joins on a `sync.WaitGroup`; each goroutine runs `probeTool` (`src/cmd/shll/update.go:193`) and writes its result into a fixed-size `[]probeResult` slice **indexed by roster position** so results stay in roster order regardless of completion order. Per tool, `probeTool` determines two facts:
   - **Installed?** — `isInstalled(ctx, t.Formula)` (`brew list --formula --versions`).
   - **Supports `--skip-brew-update`?** — only for installed tools that have a non-empty `Update` argv: `toolSupportsSkipFlag` (`src/cmd/shll/update.go:209`) runs `<tool> update --help` via `proc.Run` (capture) and checks whether the output contains the literal substring `--skip-brew-update` (`strings.Contains`, never a regex — code-quality.md anti-pattern). A probe transport error is treated as "not supported" → graceful degradation to a plain `<tool> update`.

4. **Detect shll-self brew install.** `isInstalled(ctx, shllFormula)` (`shllFormula = "sahil87/tap/shll"`, `src/cmd/shll/brew.go:28`). Drives whether the self-upgrade step in (7) runs. (This single probe runs after `probeRoster`, not inside it — shll is intentionally not in `Roster`.)

5. **Nothing-to-do → short-circuit.** If no roster tool is installed AND shll itself is not brew-installed, write `No sahil87 tools installed.` to stdout and return nil. Exit code: 0. Critically, **`brew update --quiet` is NOT invoked in this branch** — see Design Decision #9 below. Because the status line (step 2) already printed, the empty-case stdout reads exactly `Checking installed sahil87 tools…\nNo sahil87 tools installed.\n` (`TestUpdate_NoToolsInstalled`). When shll itself is brew-installed but no roster tools are, the short-circuit does NOT fire — the run proceeds and only self-upgrades shll (`TestUpdate_OnlyShllInstalled`).

6. **Refresh metadata once.** `proc.RunForeground(ctx, brewBinary, "update", "--quiet")` — foreground so users see brew's progress. Run exactly **once** per invocation, after probing and before any upgrade. Because each delegated `<tool> update --skip-brew-update` skips its own internal `brew update`, this is the only metadata refresh for the whole run (vs. N redundant refreshes if each tool refreshed independently — Design Decision #2 of this change). `proc.RunForeground` returns `(code, nil)` on a non-zero subprocess exit and `(_, err)` only on an exec/transport failure, so the branch checks **both** `code != 0` and `err != nil`. On failure, write `shll update: brew update failed: <detail>` to stderr and return `errSilent` (exit 1) — no upgrades attempted.

7. **shll self-upgrade (when brew-installed).** If step (4) reported shll itself as brew-installed, print the `shll (self)` per-tool header (see [Per-tool output separation](#per-tool-output-separation-change-y630)) then run `proc.RunForeground(ctx, brewBinary, "upgrade", shllFormula)` *before* the roster loop. shll has no `update` subcommand to call on itself, so this stays a direct `brew upgrade` (not delegated). See [shll self-upgrade](#shll-self-upgrade) for rationale and edge cases. Failures here go through the same best-effort `anyFailed` path as roster failures, and contribute to the `total`/`succeeded` counts feeding the summary tail.

8. **Sequential per-tool upgrade (delegated).** For each installed tool in roster order, print its per-tool header then call `upgradeTool(ctx, t, probes[i].supportsSkipFlag)` (`src/cmd/shll/update.go:249`). Dispatch:
   - **has `Update` argv + supports the flag** → `<tool> update --skip-brew-update` (the `Update` argv with the flag appended).
   - **has `Update` argv but no flag (version skew)** → `<tool> update` with no flag — and it does **not** fall back to `brew upgrade`. This is the retry-without-flag contract for an installed tool predating the `--skip-brew-update` convention (Constitution V — Graceful Degradation).
   - **no `Update` argv (hypothetical future tool)** → `brew upgrade <formula>` fallback (today's pre-delegation behavior, retained for tools with no `update` subcommand).

   Best-effort across the roster: on per-tool failure (transport error or non-zero exit), set `anyFailed = true` and `continue` — never abort the loop.

9. **Summary tail.** After the loop, print one summary line via `printSummaryTail` (see [Per-tool output separation](#per-tool-output-separation-change-y630)), then — unchanged — if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0). The tail is presentation-only and does **not** influence the exit code.

> **Slice-aliasing guard.** The roster's `Update` argvs are shared, read-only slices. `upgradeTool` appends the flag via `appendArg` (`src/cmd/shll/update.go:236`), which always allocates a fresh slice (`make` + `copy`) so a naive `append` can never write into the shared backing array when spare capacity exists. The same helper builds the `--help` probe argv.

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All upgrades succeeded (or nothing-to-do branch) | 0 |
| `brew` not on PATH | 1 (via `errSilent`, hint already on stderr) |
| `brew update --quiet` failed (non-zero exit OR transport error) | 1 (via `errSilent`) |
| `shll` self-upgrade failed | 1 (via `errSilent`, after roster also attempted) |
| Any per-tool upgrade failed (delegated `update` or brew-upgrade fallback) | 1 (via `errSilent`, after all tools attempted) |

## shll self-upgrade

`shll update` self-upgrades `shll` itself before iterating the roster. The behavior is contingent on detection (step 4):

- **Brew-installed shll** (`brew install sahil87/tap/shll`) → self-upgrade runs as `brew upgrade sahil87/tap/shll` immediately after the metadata refresh, before any roster upgrade. The mid-run binary on disk gets replaced; the running process keeps its mapped image and finishes normally; a follow-up `shll` invocation picks up the new binary. Pinned by `TestUpdate_AllInstalled` and `TestUpdate_SelfUpgradeOrdering`.
- **Dev build** (e.g. `go install ./cmd/shll`) → `isInstalled(ctx, shllFormula)` returns false, the self-upgrade is skipped silently, and the roster loop proceeds normally. Pinned by `TestUpdate_SelfNotBrewInstalled`. This avoids `brew upgrade` errors that would otherwise fire on a non-brew-managed binary (Constitution V — Graceful Degradation).

The self-upgrade is **unaffected by the delegation change** — shll has no `update` subcommand to call on itself, so it stays a direct `brew upgrade shllFormula`. `shll` is intentionally **not** added to `Roster`: `Roster` is the *sub-tool* roster (Constitution III — Tool Roster Source of Truth); commingling shll itself would distort `shll version`'s output (which already prints shll separately) and `shll shell-init`'s iteration semantics.

Ordering rationale: self-upgrade runs *before* the roster loop so the on-disk binary is updated as early as possible. Subsequent operations within the same invocation still execute the original mapped image (POSIX semantics — replacing the file on disk doesn't affect a running process), so there is no risk of partial-version mixing within one run.

## Detection

`isInstalled(ctx, formula)` in `src/cmd/shll/brew.go:52` is the single source of truth for "is this brew formula installed":

- Calls `brew list --formula --versions <formula>` via `proc.Run` (capture transport).
- Returns `err == nil` — `brew list --versions <formula>` exits 0 when installed (with the version on stdout) and 1 when not. We don't parse stdout; the exit code is sufficient.

Constraints (Design Decision #2):

- **No regex** over plain `brew list` output. The `code-quality.md` anti-pattern explicitly forbids this. (The `--skip-brew-update` capability probe added in change cczs holds the same line — it is a `strings.Contains` presence check on `<tool> update --help` output, never a regex.)
- **No symlink-target inspection** (hop's `/Cellar/` trick). That works for the running binary only; we are querying *other* tools' install status.
- **No hardcoded `/opt/homebrew` or `/usr/local`** paths anywhere — the brew CLI is always invoked through PATH lookup via `exec`.

`hasBrew(ctx)` in `src/cmd/shll/brew.go:33` runs `brew --version` via `proc.Run` and returns true unless the error wraps `proc.ErrNotFound`. Any other brew failure (e.g. brew exits non-zero) still implies brew is installed — graceful degradation: only `ErrNotFound` is the "missing" signal.

## Probe-first detection of `--skip-brew-update`

`shll update` decides whether to append `--skip-brew-update` to a delegated `<tool> update` *before* invoking it — by probing, not by trying and retrying:

- **Why probe-first** (Design Decision #3 of change cczs): knowing flag support up front avoids the false-positive where a genuine upgrade failure is mistaken for a flag-parse error. An "assume-support-then-retry-on-failure" strategy would re-run the tool's `update` after a real failure — which could re-trigger side effects (e.g. rk's daemon restart) it had already partially performed. A presence check on `--help` is side-effect-free.
- **Version-skew handling**: when the probe reports the flag is *not* advertised (an installed tool predating the toolkit-wide `--skip-brew-update` contract), shll runs the tool's `update` **without the flag** — it does not fall back to `brew upgrade`, because the tool's own `update` is still the faithful composition (Constitution IV). The tool will then run its own internal `brew update`; correctness is preserved at the cost of a redundant metadata refresh for that one tool. Pinned by `TestUpdate_FlagUnsupportedVersionSkew`.
- The probe is issued **only for installed tools that have a non-empty `Update` argv** — uninstalled tools and tools with no `update` subcommand are never probed (`TestUpdate_PartialInstalled`, `TestUpdate_NoUpdateArgvFallsBackToBrew`).

## Foreground vs capture

| Subprocess | Transport | Why |
|------------|-----------|-----|
| `brew --version` (in `hasBrew`) | `proc.Run` (capture) | Internal probe; user does not need to see output. |
| `brew list --formula --versions <formula>` (in `isInstalled`) | `proc.Run` (capture) | Same — it's a probe, not user-facing. |
| `<tool> update --help` (capability probe) | `proc.Run` (capture) | Probe — captured so shll can branch on flag support. `proc.Run` (TransportCapture) captures **stdout** but still streams **stderr** through; the probe writes its meaningful output to stdout and is silent on stderr in the normal case, so concurrent stderr interleaving is a rare, cosmetic edge (see "Sequential, not parallel" below). |
| `brew update --quiet` | `proc.RunForeground` | Brew's progress streamed to user's terminal. |
| `<tool> update [--skip-brew-update]` (delegated upgrade) | `proc.RunForeground` | User-visible upgrade; the tool's own progress + side-effect output streams to the terminal. |
| `brew upgrade <formula>` (self-upgrade + no-`Update`-argv fallback) | `proc.RunForeground` | Same — preserves brew's colored progress output. |

This split is a Constitution-aligned choice: probes capture (so shll can branch on the result), user-visible operations foreground (so the user sees brew / the tool working).

## Sequential, not parallel — scoped to *upgrades*

Design Decision #3 ("sequential, not parallel") governs **upgrades only**. Change cczs added an explicit carve-out for the read-only capability probes:

- **Probes are parallel.** `probeRoster` dispatches one goroutine per roster tool. This is safe — the probes (`brew list`, `<tool> update --help`) take **no Homebrew write lock**, so there is no lock contention. Their **stdout** is captured by `proc.Run` (not foregrounded). Note that `proc.Run`'s `TransportCapture` still streams **stderr** to the terminal, so stderr emitted by a probe *can* interleave during the concurrent phase; in practice the probes run only for installed tools that have an `update` subcommand and write their meaningful output to stdout, so this is a rare, cosmetic edge rather than a correctness concern. (If truly-silent probes were ever required, the fix would be a `proc` transport that also captures/discards stderr — deliberately not added here for so marginal a case, to avoid expanding the Constitution-I-critical wrapper.) Concurrency collapses the ~7 sequential brew/help spawns of the old install-filter into ~1 wall-clock. Results are written into a fixed-size slice indexed by roster position, so the upgrade loop still sees roster order regardless of completion order. Probe concurrency is unbounded at the current roster size (6) — revisit only if the roster grows substantially.
- **Upgrades remain sequential.** The per-tool upgrade loop is a plain `for` with synchronous `proc.RunForeground`. Upgrades stay serial because (a) brew serializes most internal operations behind its own lock, and (b) parallel *foregrounded* subprocesses would interleave output incomprehensibly. `TestUpdate_OneUpgradeFails` asserts the loop continues through all roster entries even when the first one fails.

## Per-tool output separation (change y630)

`shll update` frames each tool's foregrounded output with a labeled boundary so a multi-tool run is no longer one undifferentiated wall of text. All framing logic lives in the shared helper `src/cmd/shll/ui.go` (see [cli/commands](commands.md#file-layout-srccmdshll)); `update.go` only computes the color decision once and calls into it.

- **Per-tool header.** Immediately before each tool's foregrounded output, `printToolHeader(stdout, name, color)` (`src/cmd/shll/ui.go:52`) writes `▸ <tool>` (bold-cyan arrow + bold name) on a color-enabled TTY, or `==> <tool>` in pure ASCII otherwise. The `==>` idiom matches Homebrew's convention so the plain form reads naturally alongside brew's own output. The self-upgrade step (step 7) gets a header labeled `shll (self)`; each roster tool (step 8) gets one labeled `t.Name`.
- **Summary tail.** After the loop, `printSummaryTail(stdout, succeeded, total, color)` (`src/cmd/shll/ui.go:80`) writes exactly one line derived from **exit codes only**: `Done — N of M tools succeeded.` on full success (prefixed with a green `✓` when color), or `X succeeded, Y failed — see above.` on partial failure. `total` counts every tool attempted (self-upgrade + each installed roster tool); `succeeded` counts those that exited 0 — these mirror the same per-tool facts that drive `anyFailed`. The tail **never** claims "updated" vs. "up-to-date" (the honesty constraint — streamed sub-tool output means shll knows only exit codes), and never changes the process exit code.
- **Stream discipline (critical).** The header and tail are written to **stdout** — the same stream `proc.RunForeground` foregrounds sub-tool output onto (in production `cmd.OutOrStdout()` is `os.Stdout`). They are **never** written to stderr: a different buffer with independent flush timing would interleave unpredictably against the streamed output it labels. `TestUpdate_*` drive `runUpdate` with separate stdout/stderr buffers and assert header/tail text appears only in stdout.
- **Color gating.** `colorEnabled(stdout)` (`src/cmd/shll/ui.go:35`) is evaluated once and reused for every header and the tail. It returns true only when **both** (1) stdout is a real terminal — the writer is an `*os.File` AND `term.IsTerminal(fd)` (from `golang.org/x/term`, the codebase's first terminal inspection), and (2) `NO_COLOR` is unset (no-color.org convention). A `bytes.Buffer` test writer is never an `*os.File`, so tests deterministically hit the plain-ASCII branch. The ASCII degrade swaps both the glyph (`▸`→`==>`) and any Unicode in shll's own framing; sub-tool bytes are passed through untouched in both forms.
- **Empty case emits no header and no tail.** The nothing-to-do short-circuit (step 5, `No sahil87 tools installed.`) runs no per-tool loop, so there is nothing to separate or count. Its stdout is still **exactly** `Checking installed sahil87 tools…\nNo sahil87 tools installed.\n` — the `TestUpdate_NoToolsInstalled` golden string is preserved verbatim (no header, no tail). Only the loop path (step 8 reached) carries the new `==>`/tail markers in its golden strings.

## Spec-locked Design Decisions for this subcommand

These lock the contract. #2/#3/#9 are reproduced from the original `update` spec; the delegation/probe/parallel-probe decisions come from change cczs's `spec.md`. The header/tail/stream-discipline contract comes from change y630's `spec.md`.

### #2 Installed detection via `brew list`, not symlink resolution

> *Why*: `brew list --formula --versions sahil87/tap/<formula>` is the right primitive for querying *other* tools' install status. Hop's `/Cellar/` symlink trick works for the running tool only.
> *Rejected*: parsing plain `brew list` output (regex-fragile, see code-quality.md anti-pattern); inspecting filesystem paths directly (Constitution-violating hardcoded `/opt/homebrew` style paths).

### #3 Sequential brew upgrades (upgrade-scoped)

> *Why*: Brew serializes most internal operations behind its own lock; parallelism risks confusing interleaved output and lock contention with no measurable speedup.
> *Rejected*: parallel goroutine-per-tool *upgrades*. Real brew operations are I/O-bound on the single brew lock, so concurrency would not help.

Scope note (change cczs): this decision applies to **upgrades**. Read-only probes are explicitly carved out and run concurrently (see "Sequential, not parallel — scoped to upgrades" above).

### #9 `shll update` skips `brew update --quiet` when there is nothing to upgrade

> *Why*: The metadata refresh is only useful as a precursor to upgrades. When there is nothing to upgrade (no roster tools installed AND shll itself not brew-installed), the refresh is pure latency for no benefit; the user-visible message (`No sahil87 tools installed.`) is the primary signal and should print quickly.
> *Rejected*: refreshing brew metadata anyway. Considered for "freshness on every invocation" but rejected — `shll update` is not a brew metadata refresh tool, it's a sahil87 toolkit upgrader. Users who want a refresh have `brew update` directly.

This is the reason for the early short-circuit in step 5 above. The check is a logical AND — both the roster set and shll-itself must be empty/uninstalled — so a brew-installed shll with zero roster tools still proceeds (and just self-upgrades). The status line (step 2) still prints first, so DD#9 only suppresses `brew update`, not the status line. Tests assert `brew update` is NOT in the recorded call list when the full nothing-to-do branch fires (`TestUpdate_NoToolsInstalled`).

### Delegate to `<tool> update`, not `brew upgrade <formula>` (change cczs)

> *Why*: Preserves each tool's post-upgrade side effects (rk's daemon restart), satisfying Constitution IV. `brew upgrade` alone reproduces only the binary swap, not the tool's own post-upgrade logic.
> *Rejected*: hardcoding rk's daemon restart into shll (Principle IV smell, doesn't generalize); documenting the gap as a known limitation (leaves the correctness bug live).

### Hoist `brew update --quiet` into shll once, via `--skip-brew-update` (change cczs)

> *Why*: Each tool's `update` would otherwise run its own `brew update`, causing N redundant metadata refreshes. The flag lets shll do it once for the whole run.
> *Rejected*: letting each tool refresh independently (N× latency); having shll suppress refresh by other means (no cross-tool contract).

## Test seam

All `update_test.go` tests inject a fake via `proc.Runner` (`installFakeRunner` t.Cleanup helper at `src/cmd/shll/update_test.go:53`). No real brew or sub-tool subprocess is ever spawned. The fake records every `proc.Request` so tests assert: which formulas were queried, which `--help` probes ran, which upgrades ran (delegated vs. brew-upgrade), the order of operations, the exit code, and the captured stdout/stderr writers.

**Goroutine-safety (change cczs).** Because `probeRoster` now dispatches its probes concurrently, the fake is concurrency-safe: a `sync.Mutex` (`fakeRunner.mu`) guards both the `calls` slice and the `respond` dispatch, so concurrent probe calls do not race. Tests assert against a stable snapshot via `recordedCalls()` (`src/cmd/shll/update_test.go:43`), called *after* `runUpdate` returns (all probes have joined). `go test -race` is clean. Respond functions run **under `mu`**, so they must not call back into the runner. Helpers: `helpAdvertisesSkipFlag()` (returns help output containing the flag substring), `isUpdateHelpProbe(req)` (identifies a `<tool> update --help` probe by its trailing `--help` arg), and `installedOnly(formulas...)` (a respond function where only the named formulas report installed and shll-self is not-brewed).

Covered scenarios (`src/cmd/shll/update_test.go`):

- `TestUpdate_BrewMissing` — `proc.Run("brew", "--version")` returns `ErrNotFound` → stderr hint, **empty stdout** (status line not yet printed), exit 1.
- `TestUpdate_NoToolsInstalled` — neither shll nor any roster tool installed → stdout is exactly `Checking installed sahil87 tools…\nNo sahil87 tools installed.\n`, **no `brew update`**, no upgrade calls, exit 0.
- `TestUpdate_AllInstalled` — shll itself + full roster installed, help advertises no flag → `brew update --quiet`, self-upgrade, and each roster tool delegated via `<tool> update` (no flag), and NOT `brew upgrade <formula>`, exit 0.
- `TestUpdate_SelfUpgradeOrdering` — pin that the shll self-upgrade (`brew upgrade shllFormula`) appears before the first roster *upgrade* in the recorded sequence (excluding the concurrent `<tool> update --help` probe).
- `TestUpdate_SelfNotBrewInstalled` — dev build (shll not brew-installed) → self-upgrade skipped, roster still delegated via `<tool> update`.
- `TestUpdate_OnlyShllInstalled` — shll brew-installed but no roster tools → metadata refresh runs, self-upgrade runs, no roster delegation/upgrade, no short-circuit message, exit 0.
- `TestUpdate_PartialInstalled` — only `hop` and `wt` installed → only those delegated via `<tool> update`; uninstalled tools neither delegated nor brew-upgraded; the `--help` probe is issued **only** for installed tools (`hop`/`wt` probed; `idea`/`fab-kit` not probed).
- `TestUpdate_BrewUpdateFails` — `brew update --quiet` exits non-zero → stderr "brew update failed", no upgrade attempted (delegated or fallback), exit 1.
- `TestUpdate_OneUpgradeFails` — first roster tool's delegated `update` exits non-zero → loop continues; total upgrade attempts = `len(Roster) + 1` (self brew-upgrade + every roster delegation), exit 1.
- `TestUpdate_FlagSupported` — `rk` installed and `rk update --help` advertises `--skip-brew-update` → upgraded via `rk update --skip-brew-update`, NOT `brew upgrade rk`, and NOT a bare `rk update`.
- `TestUpdate_FlagUnsupportedVersionSkew` — `hop` installed but its `--help` lacks the flag → upgraded via bare `hop update` (no flag), and does NOT fall back to `brew upgrade hop`.
- `TestUpdate_NoUpdateArgvFallsBackToBrew` — a temporary single-entry roster with a `legacy` tool that has an empty `Update` argv → falls back to `brew upgrade <formula>`; no delegated update, no `--help` probe.
- `TestUpdate_StatusLinePrecedesProbes` — stdout starts with `updateStatusLine` + `\n` before any probe/brew output.
- `TestUpdate_BrewUpdateRunsExactlyOnce` — with `rk`/`hop`/`wt` installed, `brew update --quiet` runs exactly once for the whole run.

Per-tool output separation (change y630) is unit-tested directly against the `ui.go` helpers in `ui_test.go` (`TestPrintToolHeader_PlainForm`/`_ColorForm`, `TestPrintSummaryTail_AllSucceeded*`/`_PartialFailure`, `TestColorEnabled_*`, `TestToolComment_*`); `update_test.go` additionally asserts that the `==> shll (self)` and per-tool `==> <tool>` headers and the plain summary tail appear in the **stdout** buffer and never in the stderr buffer (the stream-discipline guarantee).

## Cross-references

- Subprocess wrapper conventions: [internal/proc](../internal/proc.md).
- The hardcoded roster and the `Update` capability field: [cli/commands](commands.md#hardcoded-tool-roster).
- Shared UI helper (`ui.go`) for the header/tail/color logic: [cli/commands](commands.md#file-layout-srccmdshll); the sibling [cli/install](install.md#per-tool-output-separation-change-y630) mirrors this header/tail behavior.
- Constitution III (Wrap, Don't Reinvent) and IV (Composition, Not Replacement) — the delegation in step 8 is the direct expression of both.
- Constitution V (Graceful Degradation) — uninstalled tools are skipped during probing; version-skew tools degrade to a flagless `<tool> update`.
