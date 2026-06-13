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

6. **Refresh metadata once.** `proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "update", "--quiet")` (`update.go:244`) — foreground so users see brew's progress. Run exactly **once** per invocation, after probing and before any upgrade. `brewEnv()` injects the Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` trust workaround (nil on macOS) — see [Linux brew trust workaround](#linux-brew-trust-workaround-change-38a6). Because each delegated `<tool> update --skip-brew-update` skips its own internal `brew update`, this is the only metadata refresh for the whole run (vs. N redundant refreshes if each tool refreshed independently — Design Decision #2 of this change). `proc.RunForeground` returns `(code, nil)` on a non-zero subprocess exit and `(_, err)` only on an exec/transport failure, so the branch checks **both** `code != 0` and `err != nil`. On failure, write `shll update: brew update failed: <detail>` to stderr and return `errSilent` (exit 1) — no upgrades attempted.

7. **shll self-upgrade (when brew-installed).** If step (4) reported shll itself as brew-installed, print the `shll (self)` per-tool header (see [Per-tool output separation](#per-tool-output-separation-change-y630)) then run `proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "upgrade", shllFormula)` (`update.go:299`) *before* the roster loop. The self-upgrade is a brew call, so it carries the Linux trust workaround (`brewEnv()`) — see [Linux brew trust workaround](#linux-brew-trust-workaround-change-38a6). shll has no `update` subcommand to call on itself, so this stays a direct `brew upgrade` (not delegated). See [shll self-upgrade](#shll-self-upgrade) for rationale and edge cases. Failures here go through the same best-effort `anyFailed` path as roster failures, and contribute to the `total`/`succeeded` counts feeding the summary tail.

8. **Sequential per-tool upgrade (delegated).** For each installed tool in roster order, print its per-tool header then call `upgradeTool(ctx, t, probes[i].supportsSkipFlag)` (`src/cmd/shll/update.go:249`). Dispatch:
   - **has `Update` argv + supports the flag** → `<tool> update --skip-brew-update` (the `Update` argv with the flag appended).
   - **has `Update` argv but no flag (version skew)** → `<tool> update` with no flag — and it does **not** fall back to `brew upgrade`. This is the retry-without-flag contract for an installed tool predating the `--skip-brew-update` convention (Constitution V — Graceful Degradation).
   - **no `Update` argv (hypothetical future tool)** → `brew upgrade <formula>` fallback (today's pre-delegation behavior, retained for tools with no `update` subcommand).

   `upgradeTool` (`update.go:406`) gates the Linux trust workaround on `argv[0] == brewBinary`: the brew-fallback path uses `proc.RunForegroundEnv(ctx, brewEnv(), …)` (`update.go:416`), while a per-tool `<tool> update [--skip-brew-update]` delegation uses plain `proc.RunForeground` (`update.go:418`) and receives **no** brew override on any platform. See [Linux brew trust workaround §per-tool carve-out](#linux-brew-trust-workaround-change-38a6).

   Best-effort across the roster: on per-tool failure (transport error or non-zero exit), set `anyFailed = true` and `continue` — never abort the loop.

9. **Summary tail.** After the loop, print one summary line via `printSummaryTail` (see [Per-tool output separation](#per-tool-output-separation-change-y630)), then — unchanged — if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0). The tail is presentation-only and does **not** influence the exit code.

> **Slice-aliasing guard.** The roster's `Update` argvs are shared, read-only slices. `upgradeTool` appends the flag via `appendArg` (`src/cmd/shll/update.go:236`), which always allocates a fresh slice (`make` + `copy`) so a naive `append` can never write into the shared backing array when spare capacity exists. The same helper builds the `--help` probe argv.

## Positional tool-name args — subset targeting (change b2vg)

`shll update [tool...]` accepts zero or more positional tool-name args (`Args: cobra.ArbitraryArgs`, parsed args threaded into `runUpdate`). The grammar mirrors `brew upgrade <formula>` — positional, not a `--only` flag.

- **Zero args → whole-roster run, byte-for-byte unchanged.** `subset := len(args) > 0` is false, so the contract above (probe whole roster + shll self-upgrade) holds verbatim. This is the back-compat anchor.
- **One or more args → operate on just the named subset.** The args form a *set*, not a sequence.

**Valid targets for `update`**: the six `Roster` names (`wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`) **plus** the literal `shll`. `shll` is special — it is not in `Roster` (Constitution III — `Roster` is the sub-tool list), so the self-target name is the named constant `shllTargetToken = "shll"` (`src/cmd/shll/tools.go`). Naming `shll` engages the existing self-upgrade path (`brew upgrade sahil87/tap/shll`); see [shll self-upgrade](#shll-self-upgrade).

**Roster-order processing.** A subset is always processed in `Roster` (leaves-first) order regardless of arg order — `resolveTargets` returns the selected `Tool`s in roster order. Example: `shll update fab-kit wt` processes `wt` then `fab-kit`. When `shll` is among the targets it keeps its position as the **first** step (self-upgrade before the roster loop), exactly as in a whole-roster run — `shll update shll hop` runs `shll (self)` first, then `hop`. (Why leaves-first is output coherence, not correctness: [Leaves-first Roster order](#leaves-first-roster-order-change-auvj).)

**Validation is up front, before any work (`runUpdate` resolves the subset before `hasBrew`, the status line, and `probeRoster`).** Two error classes, both exit non-zero via `errSilent`:

1. **Unknown / typo'd name** → `resolveTargets(args, true)` returns a non-nil error; `runUpdate` writes `shll update: <detail>` to stderr and returns `errSilent` with **zero brew/network side effect** (no probe, no `brew update`). All unknown args are reported at once (a better one-shot fix), e.g. `shll update: unknown targets "foo", "bar" (valid targets: shll, wt, idea, tu, rk, hop, fab-kit)`.
2. **Named-but-not-installed** → distinct from the whole-roster *graceful skip*. Because the user named the tool explicitly, its absence is surfaced as an error, not silently skipped. Enforced **after** probing (where install facts exist): every selected target that probed not-installed — including `shll` itself on a non-brew dev build (e.g. a `go install` binary, where `isInstalled(shllFormula)` is false) — is reported as `shll update: <name>: not installed`, all missing targets at once in roster order, before any `brew update`/upgrade.

**How the subset is applied (no parallel loop).** `runUpdate` reuses the existing whole-roster code paths: after enforcing the not-installed error, it marks `probes[i].installed = false` for every roster tool *not* in the selection, and overrides `shllSelfInstalled` to `selfSelected && shllInstalled`. The existing `total`/upgrade-loop/dry-run/tail code then operates on the subset with no structural change (Design Decision #3 of this change — smallest diff, preserves every order-independent invariant).

**Counter denominator `M` = subset size.** For a subset run the per-tool header `[N/M]` denominator and the summary-tail `M` become the count of validated, processed targets (installed roster tools in the selection, plus 1 when `shll`-self was selected and installed) — not the whole-roster count. The [per-tool output separation](#per-tool-output-separation-change-y630) contract is otherwise unchanged: `M` is simply redefined as subset size, and the headers/tail still conform (the `per-tool-output-separation` spec stays valid as-is).

**`brew update --quiet` still runs once for a subset.** Unconditional, exactly once, even for a single-tool target — it sits below the nothing-to-do short-circuit, which cannot fire for a validated *installed* subset (a named-but-not-installed target already errored out above). The `--skip-brew-update` per-tool delegation is unchanged.

**`--dry-run` previews the filtered subset.** The dry-run branch runs after the subset filter, so it previews only the validated subset in roster order (shll-self first when selected), header `Would update N tools (brew metadata refresh first):` with `N` = subset size. See [`--dry-run`](#dry-run-change-6vuo).

**Shared resolver, single-sourced with `Roster`.** Both `runUpdate` and `runInstall` call `resolveTargets(args, allowShll)` (`src/cmd/shll/tools.go`) — `update` passes `allowShll=true`, `install` passes `false`. It performs **name validation only** (no brew/subprocess calls — the install-status check stays in the run functions where brew facts exist), and derives its valid-name list from the live `Roster` (via `rosterHas`/`validTargets`) so the two commands can never drift. See [cli/install §positional args](install.md#positional-tool-name-args--subset-targeting-change-b2vg) for the symmetric (roster-only) install behavior.

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All upgrades succeeded (or nothing-to-do branch) | 0 |
| Unknown/typo'd positional target (change b2vg) | 1 (via `errSilent`, before any brew work) |
| Named-but-not-installed positional target, incl. `shll` on a dev build (change b2vg) | 1 (via `errSilent`, after probing, before any upgrade) |
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
| `brew update --quiet` | `proc.RunForegroundEnv` (change 38a6) | Brew's progress streamed to user's terminal. Carries the Linux trust workaround (`brewEnv()`). |
| `<tool> update [--skip-brew-update]` (delegated upgrade) | `proc.RunForeground` | User-visible upgrade; the tool's own progress + side-effect output streams to the terminal. **No** brew override — it is a per-tool CLI, not a brew call (Constitution IV). |
| `brew upgrade <formula>` (self-upgrade + no-`Update`-argv fallback) | `proc.RunForegroundEnv` (change 38a6) | Same — preserves brew's colored progress output. Carries the Linux trust workaround (`brewEnv()`). |

This split is a Constitution-aligned choice: probes capture (so shll can branch on the result), user-visible operations foreground (so the user sees brew / the tool working).

The three brew-write rows route through `proc.RunForegroundEnv` (change 38a6) to carry the Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` override; the per-tool delegation deliberately does **not** — see [Linux brew trust workaround](#linux-brew-trust-workaround-change-38a6).

## Sequential, not parallel — scoped to *upgrades*

Design Decision #3 ("sequential, not parallel") governs **upgrades only**. Change cczs added an explicit carve-out for the read-only capability probes:

- **Probes are parallel.** `probeRoster` dispatches one goroutine per roster tool. This is safe — the probes (`brew list`, `<tool> update --help`) take **no Homebrew write lock**, so there is no lock contention. Their **stdout** is captured by `proc.Run` (not foregrounded). Note that `proc.Run`'s `TransportCapture` still streams **stderr** to the terminal, so stderr emitted by a probe *can* interleave during the concurrent phase; in practice the probes run only for installed tools that have an `update` subcommand and write their meaningful output to stdout, so this is a rare, cosmetic edge rather than a correctness concern. (If truly-silent probes were ever required, the fix would be a `proc` transport that also captures/discards stderr — deliberately not added here for so marginal a case, to avoid expanding the Constitution-I-critical wrapper.) Concurrency collapses the ~7 sequential brew/help spawns of the old install-filter into ~1 wall-clock. Results are written into a fixed-size slice indexed by roster position, so the upgrade loop still sees roster order regardless of completion order. Probe concurrency is unbounded at the current roster size (6) — revisit only if the roster grows substantially.
- **Upgrades remain sequential.** The per-tool upgrade loop is a plain `for` with synchronous `proc.RunForeground`. Upgrades stay serial because (a) brew serializes most internal operations behind its own lock, and (b) parallel *foregrounded* subprocesses would interleave output incomprehensibly. `TestUpdate_OneUpgradeFails` asserts the loop continues through all roster entries even when the first one fails.

## Per-tool output separation (change y630)

`shll update` frames each tool's foregrounded output with a labeled boundary so a multi-tool run is no longer one undifferentiated wall of text. All framing logic lives in the shared helper `src/cmd/shll/ui.go` (see [cli/commands](commands.md#file-layout-srccmdshll)); `update.go` only computes the color decision once and calls into it.

- **Per-tool header with `[N/M]` progress counter (change 6vuo).** Immediately before each tool's foregrounded output, `printToolHeader(stdout, name, pos, total, color)` (`src/cmd/shll/ui.go:56`) writes `▸ [N/M] <tool>` (bold-cyan arrow + bold name) on a color-enabled TTY, or `==> [N/M] <tool>` in pure ASCII otherwise. The `==>` idiom matches Homebrew's convention so the plain form reads naturally alongside brew's own output. `N` is the running 1-based position; `M` is the total tools acted on this run, **computed up front before the loop** (`update.go:194` — `total` is the count of `probes[i].installed` plus `1` when shll is brew-installed) so every header can carry a stable denominator. The self-upgrade step (step 7) gets the header `shll (self)` and is **`[1/M]`** — it counts as a tool like any other, so the counter agrees with the summary tail's `total` (which also includes the self step); each roster tool (step 8) gets `t.Name` at its position. (The header stays minimal — just `▸ [N/M] <tool>`; a dimmed command echo like `$ tu update --skip-brew-update` was considered and rejected as noise duplicating `--help`.) See [Worked header example](#worked-header-example-change-6vuo).
- **Section spacing (change 6vuo).** A single blank line precedes each per-tool header **except the first**, and a single blank line precedes the summary tail — so each tool's streamed output is visually separated from the next header and from the tail. The loop emits the leading `\n` via the `updateHeader` closure (`update.go:206`, `if pos > 1`); the pre-tail blank is `fmt.Fprintln(stdout)` immediately before `printSummaryTail` (`update.go:264`). The empty/short-circuit case emits NO blank lines (its golden string is preserved — see [Empty case](#per-tool-output-separation-change-y630)).
- **Summary tail with run duration (change 6vuo).** After the loop, `printSummaryTail(stdout, succeeded, total, elapsed, color)` (`src/cmd/shll/ui.go:96`) writes exactly one line derived from **exit codes only**, now with the wall-clock run duration appended to **both** forms: `Done — N of M tools succeeded in <dur>.` on full success (prefixed with a green `✓` when color), or `X succeeded, Y failed in <dur> — see above.` on partial failure (the duration sits **before** the em-dash). `total` counts every tool attempted (self-upgrade + each installed roster tool); `succeeded` counts those that exited 0 — these mirror the same per-tool facts that drive `anyFailed`. The duration is a **fact about the run, not an outcome claim** — the tail still **never** claims "updated" vs. "up-to-date" (the honesty constraint — streamed sub-tool output means shll knows only exit codes), and never changes the process exit code. Duration is rendered by `formatDuration` (`ui.go:80`) as `elapsed.Round(time.Second).String()` (e.g. `1m12s`; sub-second runs round to `0s`). See [Run duration and the clock seam](#run-duration-and-the-clock-seam-change-6vuo).
- **Stream discipline (critical).** The header and tail are written to **stdout** — the same stream `proc.RunForeground` foregrounds sub-tool output onto (in production `cmd.OutOrStdout()` is `os.Stdout`). They are **never** written to stderr: a different buffer with independent flush timing would interleave unpredictably against the streamed output it labels. `TestUpdate_*` drive `runUpdate` with separate stdout/stderr buffers and assert header/tail text appears only in stdout.
- **Color gating.** `colorEnabled(stdout)` (`src/cmd/shll/ui.go:37`) is evaluated once and reused for every header and the tail. It returns true only when **both** (1) stdout is a real terminal — the writer is an `*os.File` AND `term.IsTerminal(fd)` (from `golang.org/x/term`, the codebase's first terminal inspection), and (2) `NO_COLOR` is unset (no-color.org convention). A `bytes.Buffer` test writer is never an `*os.File`, so tests deterministically hit the plain-ASCII branch. The ASCII degrade swaps both the glyph (`▸`→`==>`) and any Unicode in shll's own framing; sub-tool bytes are passed through untouched in both forms.
- **Empty case emits no header, no tail, no counter, no spacing, no duration.** The nothing-to-do short-circuit (step 5, `No sahil87 tools installed.`) runs no per-tool loop, so there is nothing to separate, count, or time. Its stdout is still **exactly** `Checking installed sahil87 tools…\nNo sahil87 tools installed.\n` — the `TestUpdate_NoToolsInstalled` and `TestUpdate_EmptyCaseNoHeaderNoTail` golden strings are preserved verbatim (no `[N/M]` header, no blank lines, no tail, no `in <dur>`). Only the loop path (step 8 reached) carries the `==> [N/M]`/blank-line/duration markers in its golden strings.

### Worked header example (change 6vuo)

With shll brew-installed and the full roster present, `shll update` (plain, non-TTY) frames the run as (blank lines shown explicitly):

```
Checking installed sahil87 tools…
==> [1/7] shll (self)
<shll's brew upgrade output…>

==> [2/7] wt
<wt's update output…>

==> [3/7] idea

==> [4/7] tu

==> [5/7] rk

==> [6/7] hop

==> [7/7] fab-kit

Done — 7 of 7 tools succeeded in 1m12s.
```

This exact sequence (status line, `[1/7] shll (self)` first, a blank line before each subsequent header and before the tail, and the duration-bearing tail) is the `TestUpdate_HeadersAndTail` golden at `src/cmd/shll/update_test.go:571` (which installs a deterministic clock returning `t0` then `t0+72s` so the tail reads `in 1m12s`). `TestUpdate_HeaderPrecedesOutput` pins that the `==> [1/1] hop` header is in the buffer *before* hop's foregrounded upgrade runs; `TestUpdate_PartialFailureTail` pins the partial-failure tail `1 succeeded, 1 failed in 1m12s — see above.` and asserts the honesty constraint (no "updated"/"up-to-date").

### Run duration and the clock seam (change 6vuo)

The duration in the summary tail is measured via an injectable package-level clock seam — `var nowFunc = time.Now` in `src/cmd/shll/clock.go`. This mirrors the `proc.Runner` package-level-swappable injection pattern (`src/internal/proc/proc.go`) exactly: production wiring uses the real `time.Now`; tests swap it through the `installFakeClock(t, times...)` t.Cleanup helper (`src/cmd/shll/clock_test.go:13`, mirroring `installFakeRunner`) to a deterministic clock that returns the supplied times in sequence (the last value repeats), so the duration-bearing golden strings stay exact rather than racing a real wall clock.

`runUpdate` captures `start := nowFunc()` at `update.go:159` — **after** the nothing-to-do short-circuit *and* the dry-run branch have returned — so the measured elapsed (`nowFunc().Sub(start)` at `update.go:265`) covers only the write phase the tail summarizes (the metadata refresh + self-upgrade + roster loop), not the read-only probe phase. The seam keeps `runUpdate`'s signature stable; the only signature change is the new `dryRun bool` parameter (see [`--dry-run`](#dry-run-change-6vuo)). `TestInstallFakeClock_Sequences` unit-tests the helper's sequencing.

## `--dry-run` (change 6vuo)

`shll update --dry-run` previews the exact commands the run **would** execute, then exits 0 **without any write**. The flag is a cobra bool (`dryRunFlag = "dry-run"`, usage `dryRunFlagUsage`, both named constants in `update.go:66`), wired in `newUpdateCmd` and read in `RunE` into the new `dryRun bool` parameter on `runUpdate`.

**Reads run; writes do not — the safety contract.** Dry-run is *not* a no-op: the read-only probes the command already runs still run (they are reads, and the preview accuracy depends on them) — `hasBrew`, the full `probeRoster` (`brew list --formula --versions` install detection + the `<tool> update --help` `--skip-brew-update` capability check), and the shll-self `brew list`. But **no write** is performed below the probe phase: NO `brew update --quiet` (it mutates brew's local metadata — itself a side effect), NO `brew upgrade`, NO `<tool> update`. The guarantee is **structural**: the dry-run branch (`update.go:140`) returns before `start := nowFunc()` and the whole write phase, so no write path is reachable. `TestUpdate_DryRunNoWrites` asserts both directions — the read-only probes (`brew list`, a `<tool> update --help`) ARE recorded, while `brew update --quiet`, `brew upgrade shllFormula`, every `<tool> update` write, and every `brew upgrade <formula>` are NOT — and additionally asserts **zero `TransportForeground` calls** (all writes are foreground, so their absence is a clean structural check).

**The preview.** A header line `Would update N tools (brew metadata refresh first):` (`updatePreviewHeaderFmt`), then one aligned row per actionable tool. The "brew metadata refresh first" annotation reflects that the *real* run calls `brew update --quiet` once up front — but dry-run does NOT run it. Rows are built in `runUpdate` (`update.go:141`) from probe results: `shll (self)` first when brew-installed (`brew upgrade sahil87/tap/shll`), then each installed roster tool in roster order. The per-tool command string is `argvString(upgradeArgv(t, probes[i].supportsSkipFlag)...)` — i.e. rendered from the **same `upgradeArgv` the live run uses** (`update.go:340`, the single source of truth shared by `upgradeTool` and the preview), so the preview can never drift from what the run would do. Per-tool argv dispatch:

- has `Update` argv + supports the flag → `<tool> update --skip-brew-update`
- has `Update` argv, no flag (version skew) → `<tool> update`
- no `Update` argv (hypothetical future tool) → `brew upgrade sahil87/tap/<formula>`
- `shll (self)` (when brew-installed) → `brew upgrade sahil87/tap/shll`

Formatting lives in `ui.go`'s `printUpdatePreview` → `printPreviewRows`: a 2-space row indent (`previewIndent`), tool labels left-padded to the **longest label present** (including `shll (self)`, the widest at 11 chars when present), then a 2-space gap (`previewGap`) before the command — so commands line up in a readable column. The preview rows carry **no `[N/M]` counter and no blank-line spacing** — those are streaming-loop concerns; the preview is a static aligned table.

```
Would update 7 tools (brew metadata refresh first):
  shll (self)  brew upgrade sahil87/tap/shll
  wt           wt update
  idea         idea update
  tu           tu update
  rk           rk update
  hop          hop update
  fab-kit      fab-kit update
```

(`TestUpdate_DryRunPreviewWithSelf` golden — shll brew-installed, no tool advertises the flag.)

**Graceful degradation (Constitution V).** The preview lists only actionable tools — uninstalled roster tools are omitted, exactly as they are skipped in the real upgrade loop. With only `hop` and `wt` installed and shll not brew-installed, the preview is exactly those two in roster order (`wt` then `hop`), header `Would update 2 tools (brew metadata refresh first):` (`TestUpdate_DryRunGracefulDegradation`). `TestUpdate_DryRunPreview` covers the full roster with shll *not* brew-installed and `rk`/`hop` advertising the flag (so they read `… update --skip-brew-update`).

**Empty case.** When nothing is installed AND shll itself is not brew-installed, the dry-run path never reaches the preview builder — the shared nothing-to-do short-circuit (step 5) fires first, so stdout is exactly `Checking installed sahil87 tools…\nNo sahil87 tools installed.\n` (the `noToolsInstalledMsg` constant, shared with the non-dry-run short-circuit), exit 0, no preview table, no `brew update` (`TestUpdate_DryRunEmptyCase`).

**Brew-missing precondition unchanged.** `--dry-run` does not relax the `hasBrew` bail — a missing brew still writes `brewMissingHint` to stderr and exits 1 (the brew-missing check at `update.go:92` precedes the dry-run branch).

Exit code: always 0 in dry-run (no writes, nothing can fail) except the brew-missing precondition (exit 1).

## Leaves-first Roster order (change auvj)

`shll update` probes and upgrades in `Roster` order (step 8 iterates `Roster`). Since change auvj, that order is **leaves-first**: `wt, idea, tu, rk, hop, fab-kit`. With shll itself brew-installed and the full roster present, the per-tool headers print as `==> [1/7] shll (self)` then `==> [2/7] wt`, `==> [3/7] idea`, `==> [4/7] tu`, `==> [5/7] rk`, `==> [6/7] hop`, `==> [7/7] fab-kit` (the `[N/M]` counters added by change 6vuo), each header after the first preceded by a blank line, with the `Done — 7 of 7 tools succeeded in 1m12s.` duration-bearing tail (`TestUpdate_HeadersAndTail` golden at `src/cmd/shll/update_test.go:571` — see [Worked header example](#worked-header-example-change-6vuo)).

This ordering is **output coherence**, not correctness: it ensures each tool's `==> <tool>` section completes and is counted in the summary tail before a *dependent* tool's internal `brew upgrade` can re-touch a leaf already reported done under its own header. It is **not** a correctness fix — brew resolves formula deps idempotently and each `<tool> update` is self-update-only, so no tool's `update` cascades into another tool's upgrade during `shll update`; the order can neither break nor improve upgrade correctness. The full rationale, the dependency graph (brew-upgrade + runtime edges), the invariant test `TestRosterLeavesBeforeDependents`, and the "no `DependsOn` field" decision live in [cli/commands](commands.md#design-decision-leaves-first-roster-order-change-auvj). The order-independent update invariants (brew-missing bail, status line, single `brew update`, self-upgrade-before-roster, best-effort loop, summary tail, exit codes) are unaffected by the reorder; `TestUpdate_SelfUpgradeOrdering`/`TestUpdate_OneUpgradeFails` reference `Roster[0]` (now `wt`) dynamically and need no edit.

## Linux brew trust workaround (change 38a6)

> **TEMPORARY — slated for removal.** This whole behavior (the `brewEnv()` helper, the `goosFunc` seam, the `RunForegroundEnv` wiring, and its tests) exists to route around a Homebrew 6.0 bug and is tracked for removal under backlog `[tkch]` once the upstream fix lands. It is **not** permanent design — a future reader should expect it to disappear, not extend it.

`shll update`'s three brew-write subprocesses inject a Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` override; per-tool `<tool> update` delegations do **not**.

**The helper (`brewEnv()`, single source of truth).** `src/cmd/shll/brew.go:36` is the one spot that defines the override, so the `[tkch]` removal is a one-spot edit:

```go
func brewEnv() []string {
    if goosFunc() == "linux" {
        return []string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}
    }
    return nil
}
```

- **On Linux** → `[]string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}`.
- **On macOS / non-Linux** → `nil`, so the brew calls degrade to a plain `proc.RunForeground` with no env change and trust enforcement is preserved (the Linux gate is deliberate: macOS has no bwrap sandbox).
- The override key/value reuses the literal already named in `trustHatchHint` (`brew.go:114`); it appears in source exactly once inside `brewEnv()`.

**Why the override exists.** Homebrew 6.0's Linux build runs the formula `build.rb` inside a `bwrap` (bubblewrap) sandbox whose `deny_read_home` masks almost all of `$HOME`. The exception list covers `HOMEBREW_PREFIX`/`CACHE`/`LOGS`/`TEMP` but **not** `~/.homebrew`, where `trust.json` lives. With `HOMEBREW_REQUIRE_TAP_TRUST=1` set, the sandboxed `build.rb` re-checks tap trust, cannot read `~/.homebrew/trust.json`, and raises a (swallowed) `Homebrew::UntrustedTapError`, surfacing only an opaque `bwrap … exited with 1`. shll *encourages* `HOMEBREW_REQUIRE_TAP_TRUST=1` via `shll shell-setup --trust-tap`, so shll's own pro-trust posture is what walks Linux users into the broken state. Setting `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` per call keeps the sandbox **active** and skips *only* the broken in-sandbox trust re-check (verified working: `HOMEBREW_NO_REQUIRE_TAP_TRUST=1 brew upgrade sahil87/tap/idea` completed cleanly).

**The `goosFunc` GOOS seam.** `brewEnv()` reads the OS through `var goosFunc = func() string { return runtime.GOOS }` (`brew.go:19`) rather than `runtime.GOOS` directly. This injectable package-level seam lets one table-driven test (`TestBrewEnv_PerGOOS`) assert both the linux (override present) and darwin (override absent) branches in a single run with no per-OS build tags. It **mirrors the existing `nowFunc` clock seam** (`clock.go:14`) and the `proc.Runner` seam (`internal/proc/proc.go`) — the established pattern for injectable package-level seams in this codebase. (Pattern note for future reuse: when a behavior keys off `runtime.GOOS` and both branches must be unit-tested, prefer a `goosFunc`-style var over build-tagged test files.)

**Per-tool carve-out (Constitution IV).** The four foreground brew sites that receive the override:

| Site | Call | Override |
|------|------|----------|
| `brew update --quiet` (`update.go:244`) | `RunForegroundEnv(ctx, brewEnv(), …)` | yes (Linux) |
| shll self-upgrade `brew upgrade shllFormula` (`update.go:299`) | `RunForegroundEnv(ctx, brewEnv(), …)` | yes (Linux) |
| brew-fallback `brew upgrade <formula>` (`update.go:416`) | `RunForegroundEnv(ctx, brewEnv(), …)` | yes (Linux) |
| `shll install` `brew install <formula>` (`install.go:147`) | `RunForegroundEnv(ctx, brewEnv(), …)` | yes (Linux) — see [cli/install](install.md#linux-brew-trust-workaround-change-38a6) |

`upgradeTool` (`update.go:406`) gates the env on `argv[0] == brewBinary` so a per-tool `<tool> update [--skip-brew-update]` delegation stays on plain `proc.RunForeground` (`update.go:418`) and receives **no** brew override on any platform. Injecting brew-specific env into a sub-tool's own CLI would violate Constitution IV (compose, don't absorb) and could mask the tool's own trust behavior. `upgradeArgv` returns either a `brew upgrade <formula>` argv or a per-tool argv, so the binary name is the correct discriminator.

**Cross-references.** Env-passing transport: [internal/proc §RunForegroundEnv](../internal/proc.md#design-decisions). Sibling install site: [cli/install §Linux brew trust workaround](install.md#linux-brew-trust-workaround-change-38a6). Removal item: backlog `[tkch]`.

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
- `TestUpdate_HeadersAndTail` *(change 6vuo, golden updated)* — shll + full roster installed; asserts the verbatim `[N/M]` headers (`==> [1/7] shll (self)` first), the blank line before each subsequent header and before the tail, and the duration-bearing `Done — 7 of 7 tools succeeded in 1m12s.` tail (installs a deterministic clock).
- `TestUpdate_HeaderPrecedesOutput` *(change 6vuo)* — the `==> [1/1] hop` header is in the buffer before hop's foregrounded upgrade runs.
- `TestUpdate_PartialFailureTail` *(change 6vuo)* — `hop`+`wt` installed (shll not brewed → `total=2`), hop fails → partial-failure tail `1 succeeded, 1 failed in 1m12s — see above.` with the duration before the em-dash; asserts the honesty constraint (no "updated"/"up-to-date").
- `TestUpdate_EmptyCaseNoHeaderNoTail` *(change 6vuo)* — nothing installed → status line + `No sahil87 tools installed.` only, with no `==>` header and no `Done —`/duration tail.
- `TestUpdate_DryRunPreview` *(change 6vuo)* — shll NOT brew-installed, full roster, `rk`/`hop` advertise the flag → verbatim aligned-column preview (`Would update 6 tools (brew metadata refresh first):` then padded rows, `rk`/`hop` reading `… update --skip-brew-update`).
- `TestUpdate_DryRunPreviewWithSelf` *(change 6vuo)* — shll brew-installed + full roster, no flag advertised → preview lists `shll (self)` first (`brew upgrade sahil87/tap/shll`), `shll (self)` is the widest label so all commands align under it.
- `TestUpdate_DryRunNoWrites` *(change 6vuo)* — read-only probes (`brew list`, a `<tool> update --help`) ARE recorded; `brew update --quiet`/`brew upgrade`/every `<tool> update`/every `brew upgrade <formula>` are NOT; and **zero** `TransportForeground` calls.
- `TestUpdate_DryRunGracefulDegradation` *(change 6vuo)* — only `hop`+`wt` installed → preview lists exactly `wt`, `hop` (roster order), header `Would update 2 tools (brew metadata refresh first):`.
- `TestUpdate_DryRunEmptyCase` *(change 6vuo)* — nothing installed → dry-run mirrors the nothing-to-do message, no preview table, no `brew update`, exit 0.
- `TestUpdate_SubsetUnknownTargetHardErrors` *(change b2vg)* — `shll update <typo>` → `errSilent`, stderr names the unknown arg and lists valid targets, and **no `brew` subprocess runs** (validated before `hasBrew`/probe).
- `TestUpdate_SubsetMultipleUnknownAllReported` *(change b2vg)* — multiple unknown args → all reported in one error.
- `TestUpdate_SubsetNamedNotInstalledErrors` *(change b2vg)* — a valid name that is not installed (`shll update rk` with rk uninstalled) → `shll update: rk: not installed`, `errSilent`, nothing upgraded (distinct from the whole-roster graceful skip).
- `TestUpdate_SubsetShllSelfTargetOnly` *(change b2vg)* — `shll update shll` (shll brew-installed) → only the self-upgrade runs (`brew upgrade shllFormula`), no roster tool upgraded, `M=1`.
- `TestUpdate_SubsetShllSelfNotBrewInstalledErrors` *(change b2vg)* — `shll update shll` on a dev build (shll not brew-installed) → the not-installed error for `shll`.
- `TestUpdate_SubsetSelfFirstThenRosterOrder` *(change b2vg)* — `shll update shll hop` → `shll (self)` first, then `hop`.
- `TestUpdate_SubsetArgOrderIndependentRosterOrder` *(change b2vg)* — `shll update fab-kit wt` → `wt` before `fab-kit` (roster order, not arg order).
- `TestUpdate_SubsetBrewUpdateRunsOnce` *(change b2vg)* — a single-tool subset still runs `brew update --quiet` exactly once.
- `TestUpdate_SubsetDryRunPreviewFiltered` *(change b2vg)* — `shll update --dry-run hop wt` → preview lists exactly the two-tool subset in roster order, header `Would update 2 tools (brew metadata refresh first):`, exit 0, no write.
- `TestUpdate_BrewTrustOverride_PerGOOS` *(change 38a6)* — swaps `goosFunc`: on linux every brew-write Request (`brew update --quiet`, self-upgrade `brew upgrade shllFormula`) carries `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` in `Env` while the per-tool `<tool> update` delegation Request carries an **empty** `Env`; on darwin no Request carries the override. Pins both the override and the Constitution-IV carve-out in one run.
- `TestUpdate_BrewFallbackCarriesOverride` *(change 38a6)* — a tool with an empty `Update` argv (brew-fallback path) → its `brew upgrade <formula>` Request carries the override on linux, confirming the gate keys off the binary name (`argv[0] == brewBinary`), not the call site.

The shared resolver is unit-tested directly in `tools_test.go` (`TestResolveTargets_RosterOrderRegardlessOfArgOrder`, `TestResolveTargets_ShllGatedByAllowShll`, `TestResolveTargets_MultipleUnknownAllReported`, `TestResolveTargets_EmptyArgs`).

Per-tool output separation (change y630) plus the change-6vuo `[N/M]` counter, duration, and preview helpers are unit-tested directly against the `ui.go` helpers in `ui_test.go` (`TestPrintToolHeader_PlainForm`/`_ColorForm` now assert the `[N/M]` counter; `TestPrintSummaryTail_AllSucceeded*`/`_PartialFailure` assert the `in 1m12s` suffix; `TestFormatDuration`, `TestPrintUpdatePreview_AlignedColumns`, `TestPrintInstallPreview_AlignedColumns`, `TestColorEnabled_*`, `TestToolComment_*`); the clock seam helper is exercised by `TestInstallFakeClock_Sequences` (`clock_test.go`). `update_test.go` additionally asserts that the `==> [N/M] shll (self)` and per-tool `==> [N/M] <tool>` headers and the plain summary tail appear in the **stdout** buffer and never in the stderr buffer (the stream-discipline guarantee).

## Changelog

- **change 38a6** (`260613-38a6-brew-no-tap-trust-workaround`): The three brew-write sites (`brew update --quiet`, the shll self-upgrade, and the brew-fallback inside `upgradeTool`) now route through `proc.RunForegroundEnv(ctx, brewEnv(), …)` to inject the Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` trust workaround; per-tool `<tool> update` delegations are deliberately **not** touched (gated on `argv[0] == brewBinary`). Adds the injectable `goosFunc` GOOS seam (mirrors `nowFunc`). See [Linux brew trust workaround](#linux-brew-trust-workaround-change-38a6). **TEMPORARY** — removed under backlog `[tkch]`.

## Cross-references

- Subprocess wrapper conventions: [internal/proc](../internal/proc.md).
- The hardcoded roster and the `Update` capability field: [cli/commands](commands.md#hardcoded-tool-roster).
- Shared UI helper (`ui.go`) for the header/tail/color logic: [cli/commands](commands.md#file-layout-srccmdshll); the sibling [cli/install](install.md#per-tool-output-separation-change-y630) mirrors this header/tail behavior.
- Constitution III (Wrap, Don't Reinvent) and IV (Composition, Not Replacement) — the delegation in step 8 is the direct expression of both.
- Constitution V (Graceful Degradation) — uninstalled tools are skipped during probing; version-skew tools degrade to a flagless `<tool> update`.
