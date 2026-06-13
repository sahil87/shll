# cli/install

`shll install` — installs every roster tool that isn't already installed via Homebrew. Idempotent; safe to re-run.

Source: `src/cmd/shll/install.go`, with shared brew helpers in `src/cmd/shll/brew.go`.

## Behavior contract

The full happy/unhappy paths, in the order `runInstall` evaluates them (`src/cmd/shll/install.go`):

1. **Brew missing.** If `hasBrew(ctx)` returns false, write `installBrewMissingHint` to stderr and return `errSilent`. Exit code: 1. The literal hint is `"shll install requires Homebrew. Install from https://brew.sh"` (`src/cmd/shll/brew.go`). It is intentionally separate from `brewMissingHint` (used by `shll update`) so each command's error tells the user which command they ran — the update spec scenario asserts its verbatim text, so reusing the same constant for both commands would either violate that lock or mislead `shll install` users.

2. **Partition the roster.** Iterate `Roster` in order, calling `isInstalled(ctx, t.Formula)`; collect the *missing* entries into a local `missing` slice.

3. **Nothing missing → short-circuit.** If `len(missing) == 0`, write `All sahil87 tools already installed.` to stdout and return nil. Exit code: 0. No `brew update` is invoked — there's nothing to install.

4. **No `brew update --quiet`.** Unlike `shll update`, `shll install` does NOT refresh brew metadata first. `brew install sahil87/tap/<formula>` resolves the formula via the tap directly, and the spec freezes this distinction (Design Decision: install ≠ update). `TestInstall_NoBrewUpdateInvoked` pins the contract.

5. **Sequential per-tool install.** For each missing tool in roster order, print its per-tool header (see [Per-tool output separation](#per-tool-output-separation-change-y630)) then run `proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "install", t.Formula)` (`install.go:147`). `brewEnv()` injects the Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` trust workaround — see [Linux brew trust workaround](#linux-brew-trust-workaround-change-38a6); it returns nil on macOS, so the call is equivalent to a plain `proc.RunForeground` there. Best-effort across the roster: on per-tool failure (transport error or non-zero exit), set `anyFailed = true` and `continue` — never abort the loop.

6. **Summary tail.** After the loop, print one summary line via `printSummaryTail` (see [Per-tool output separation](#per-tool-output-separation-change-y630)), then — unchanged — if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0). The tail is presentation-only and does not change the exit code.

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All installs succeeded (or all-already-installed branch) | 0 |
| Unknown/typo'd positional target — incl. `shll`, which is rejected (change b2vg) | 1 (via `errSilent`, before any brew work) |
| `brew` not on PATH | 1 (via `errSilent`, hint already on stderr) |
| Any per-tool `brew install` failed | 1 (via `errSilent`, after all missing tools attempted) |

## Per-tool output separation (change y630)

`shll install` mirrors `shll update`'s framing exactly, via the same shared helper `src/cmd/shll/ui.go` (see [cli/commands](commands.md#file-layout-srccmdshll)) — no TTY/`NO_COLOR`/glyph logic is duplicated in `install.go`.

- **Per-tool header with `[N/M]` progress counter (change 6vuo).** Before each missing tool's `brew install` output, `printToolHeader(stdout, t.Name, i+1, total, color)` (`install.go:109`) writes `▸ [N/M] <tool>` (color TTY) / `==> [N/M] <tool>` (plain), in roster order, where `N` is the 1-based loop position and `M = len(missing)` — already known up front, so no separate denominator computation is needed (unlike `update`, where `M` is derived from the probe results). Since change auvj the roster is leaves-first (`wt, idea, tu, rk, hop, fab-kit`), so the headers for the *missing subset* print in that relative order — e.g. with `hop`+`wt` already installed, the missing set `{idea, tu, rk, fab-kit}` yields `==> [1/4] idea`, `==> [2/4] tu`, `==> [3/4] rk`, `==> [4/4] fab-kit` (`TestInstall_HeadersAndTail` golden at `src/cmd/shll/install_test.go:190`, with the `Done — 4 of 4 tools succeeded in 1m12s.` tail). See the [leaves-first ordering rationale](commands.md#design-decision-leaves-first-roster-order-change-auvj).
- **Section spacing (change 6vuo).** A single blank line precedes each per-tool header **except the first** (`install.go:106`, `if i > 0`), and a single blank line precedes the summary tail (`install.go:128`) — so each tool's streamed output is separated from the next header and the tail. The all-already-installed short-circuit emits no blank lines.
- **Summary tail with run duration (change 6vuo).** After the loop, `printSummaryTail(stdout, succeeded, total, elapsed, color)` (`install.go:129`, `total = len(missing)`) writes `Done — N of M tools succeeded in <dur>.` (green `✓` when color) or `X succeeded, Y failed in <dur> — see above.` (duration before the em-dash), by **exit code only** — `succeeded` counts installs that exited 0, mirroring the same per-tool facts that drive `anyFailed`. The duration is a run fact, not an outcome claim — the tail still never claims "installed" vs. "up-to-date" (the honesty constraint). Presentation-only; does not change the exit code. Elapsed is measured via the injectable `nowFunc` clock seam (`clock.go`), captured at `install.go:101` **after** the short-circuit and the dry-run branch return, so it covers only the install phase.
- **Stream discipline.** Header and tail go to **stdout** (the stream `brew install` is foregrounded onto), never stderr.
- **Color gating.** One `colorEnabled(stdout)` decision (TTY via `golang.org/x/term` AND `NO_COLOR` unset), reused for headers and tail; `bytes.Buffer` test writers hit the plain-ASCII branch.
- **Empty case emits no header, no tail, no counter, no spacing, no duration.** The all-already-installed short-circuit (step 3) runs no loop, so its stdout stays **exactly** `All sahil87 tools already installed.\n` (the `allInstalledMsg` constant, `install.go:140`) — the `TestInstall_AllAlreadyInstalled` and `TestInstall_EmptyCaseNoHeaderNoTail` golden strings are preserved verbatim. Only the install-loop path carries the `==> [N/M]`/blank-line/duration markers.

The helper details (named SGR constants, the `colorEnabled` gating, the honesty constraint on the tail, the `[N/M]` counter, the `formatDuration` form, and the `nowFunc` clock seam) are documented once under [cli/update](update.md#per-tool-output-separation-change-y630); `install` consumes the identical helpers.

## `--dry-run` (change 6vuo)

`shll install --dry-run` previews the `brew install` commands the run **would** execute, then exits 0 **without any write**. It mirrors `shll update --dry-run` (see [cli/update](update.md#dry-run-change-6vuo) for the shared contract); the flag, usage string, and the `dryRun bool` parameter on `runInstall` are the same `dryRunFlag`/`dryRunFlagUsage` constants (defined in `update.go`, shared across both commands).

**Reads run; writes do not.** The `isInstalled` probes (`brew list --formula --versions`) that partition the roster still run in dry-run (they are reads, and the preview depends on them) — but **no `brew install`** is performed. The guarantee is structural: the dry-run branch (`install.go:80`) returns before the install loop and before `start := nowFunc()`. `TestInstall_DryRunNoWrites` asserts the `brew list` probe IS recorded, no `brew install <formula>` runs for any tool, and there are **zero `TransportForeground`** calls.

**The preview.** A header line `Would install N tools:` (`installPreviewHeaderFmt`) — **no metadata-refresh annotation**, since `install` runs no `brew update` (consistent with [Design Decision #2](#2-no-metadata-refresh)) — then one aligned row per missing tool, in roster order, each reading `brew install sahil87/tap/<formula>` (built as `argvString(brewBinary, "install", t.Formula)`, `install.go:83`). Formatting reuses the same `ui.go` `printInstallPreview` → `printPreviewRows` aligned-column layout as `update`: 2-space indent, labels left-padded to the longest *missing* label present, 2-space gap before the command. No `[N/M]` counter, no blank-line spacing (the preview is a static table).

```
Would install 4 tools:
  idea     brew install sahil87/tap/idea
  tu       brew install sahil87/tap/tu
  rk       brew install sahil87/tap/rk
  fab-kit  brew install sahil87/tap/fab-kit
```

(`TestInstall_DryRunPreview` golden — `hop`+`wt` installed, the other four missing; the longest missing label `fab-kit` (7) sets the column width. The test also asserts the preview does NOT mention "metadata refresh".)

**Graceful degradation (Constitution V).** Only the missing subset is listed; already-installed tools are omitted (they are filtered out into `missing` before the preview builds).

**Empty case.** When every roster tool is already installed, the dry-run path never reaches the preview builder — the shared all-already-installed short-circuit (step 3) fires first, so stdout is exactly `All sahil87 tools already installed.\n` (the `allInstalledMsg` constant), exit 0, no preview table, no install (`TestInstall_DryRunEmptyCase`).

**Brew-missing precondition unchanged.** A missing brew still writes `installBrewMissingHint` to stderr and exits 1 (the `hasBrew` check precedes the dry-run branch).

## Positional tool-name args — subset targeting (change b2vg)

`shll install [tool...]` accepts zero or more positional tool-name args (`Args: cobra.ArbitraryArgs`, parsed args threaded into `runInstall`), symmetric with [`shll update`](update.md#positional-tool-name-args--subset-targeting-change-b2vg) for the install lifecycle. The shared resolver is single-sourced with `Roster`; install differs from update in exactly one way — the valid-target set.

- **Zero args → whole-roster run, unchanged.** `subset := len(args) > 0` is false; the partition/install behavior above holds verbatim.
- **One or more args → operate on just the named subset.** The args form a *set*, not a sequence.

**Valid targets for `install` are the six `Roster` tools ONLY** (`wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`). **`shll` is NOT a valid install target** — you cannot `brew install` the running orchestrator. `runInstall` calls `resolveTargets(args, false)` (`allowShll=false`), so `shll install shll` falls into the unknown-target error path (`shll install: unknown target "shll" (valid targets: wt, idea, tu, rk, hop, fab-kit)`) — note `shll` is absent from the valid list (it appears only for `update`, where `allowShll=true`).

**Roster-order processing.** A subset is processed in `Roster` (leaves-first) order regardless of arg order — `resolveTargets` returns the selected `Tool`s in roster order, and `runInstall` walks `consider = selected` (else the full `Roster`) to build `missing`, preserving that order. Example: `shll install fab-kit wt` installs `wt` then `fab-kit`. (Why leaves-first is output coherence, not correctness: [leaves-first ordering rationale](commands.md#design-decision-leaves-first-roster-order-change-auvj).)

**Validation up front (`runInstall` resolves the subset before `hasBrew` and any probe).** An unknown / typo'd name → `resolveTargets` returns a non-nil error; `runInstall` writes `shll install: <detail>` to stderr and returns `errSilent` (exit 1) with **no brew side effect**. All unknown args are reported at once.

**Named-already-installed → the existing nothing-to-do path.** For `install`, "not installed" is the happy path. The inverse edge — a tool named explicitly that is *already* installed — is **not** an error: it is filtered out into the (empty-for-it) `missing` set, exactly like the whole-roster idempotent skip. If every named target is already installed, the run hits the existing short-circuit and prints `All sahil87 tools already installed.` (exit 0). (Contrast `update`, where a named-but-not-installed target *is* an error — the asymmetry follows from the inverted precondition: install acts on absent tools, update acts on present ones.)

**Counter denominator `M` = subset size.** `M = len(missing)`, where `missing` is now restricted to the named-and-missing subset, so the per-tool `[N/M]` header and the summary-tail `M` reflect the subset, not the whole roster. The [per-tool output separation](#per-tool-output-separation-change-y630) contract is otherwise unchanged.

**`--dry-run` previews the filtered subset.** The dry-run branch runs after `missing` is built from the subset, so it previews only the named-and-missing tools in roster order, header `Would install N tools:` with `N` = subset size.

## Linux brew trust workaround (change 38a6)

> **TEMPORARY — slated for removal.** This whole behavior exists to route around a Homebrew 6.0 bug and is tracked for removal under backlog `[tkch]` once the upstream fix lands. It is **not** permanent design. A future reader should expect it to disappear, not to extend it.

The single `brew install` foreground site (`install.go:147`) calls `proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "install", t.Formula)` instead of plain `proc.RunForeground`. `brewEnv()` (in `src/cmd/shll/brew.go:36`) is the **one source of truth** for the override:

- **On Linux** it returns `[]string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}`.
- **On macOS** (and anything not Linux) it returns `nil`, so the call degrades to a plain foreground spawn with no env change — trust enforcement is preserved.

**Why the override exists.** Homebrew 6.0's Linux build runs the formula `build.rb` inside a `bwrap` (bubblewrap) sandbox whose `deny_read_home` masks almost all of `$HOME`. Its exception list covers `HOMEBREW_PREFIX`/`CACHE`/`LOGS`/`TEMP` but **not** `~/.homebrew`, where `trust.json` lives. So when the user has `HOMEBREW_REQUIRE_TAP_TRUST=1` set, the sandboxed `build.rb` re-checks tap trust, cannot read `~/.homebrew/trust.json`, and raises a `Homebrew::UntrustedTapError` — which is swallowed (it fires before the error pipe is connected), surfacing only an opaque `bwrap … exited with 1`. shll itself *encourages* `HOMEBREW_REQUIRE_TAP_TRUST=1` via `shll shell-setup --trust-tap`, so shll's own pro-trust posture is what walks Linux users into the broken state. Setting `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` per call keeps the sandbox **active** and skips *only* the broken in-sandbox trust re-check (verified working: `HOMEBREW_NO_REQUIRE_TAP_TRUST=1 brew upgrade sahil87/tap/idea` completed cleanly).

**Cross-references.** The same `brewEnv()` override is applied at `shll update`'s brew sites (`brew update --quiet`, the shll self-upgrade, and the brew-fallback path) — see [cli/update §Linux brew trust workaround](update.md#linux-brew-trust-workaround-change-38a6) for the helper's full comment contract, the injectable `goosFunc` GOOS seam, and the per-tool carve-out. The env-passing transport is `proc.RunForegroundEnv` — see [internal/proc](../internal/proc.md#design-decisions). Removal item: backlog `[tkch]`.

## Constitution VII justification

> *Why a new top-level subcommand?* `install` is a distinct lifecycle operation from `update`: different precondition (tool not installed vs. installed), different failure modes (no metadata-refresh dependency), and different discoverability (a new user wanting "get me the toolkit" looks for `install`). Cannot be cleanly expressed as a flag on `update` because `update`'s installed-only precondition would have to invert for a subset of the run.
>
> *Rejected*: `shll update --install-missing`. The branching gets messy and the verb mismatch hurts new-user discoverability.

## Spec-locked Design Decisions for this subcommand

### #1 Skip-already-installed semantics (not re-install)

> *Why*: Idempotent re-runs are the common case for bootstrap — a user runs `shll install`, installs four tools, then later adds two more to the roster (after a shll release). The second `shll install` should pick up only the new ones. Re-installing what's already present is wasted I/O and noise.
> *Rejected*: `--force` flag for re-install. YAGNI for v0.1.0; users can `brew reinstall sahil87/tap/<formula>` directly when they want it.

### #2 No metadata refresh

> *Why*: `brew install` resolves the formula via the tap directly without needing `brew update --quiet`. Skipping it is faster and the distinction from `shll update` is the point — install and update are separate lifecycle operations.
> *Rejected*: running `brew update --quiet` for "freshness". `shll install` is not a brew metadata refresh tool — users who want a refresh have `brew update` directly, or `shll update` for the combined flow.

### #3 Best-effort across the roster

> *Why*: Mirrors `shll update`'s loop semantics (Constitution V — Graceful Degradation). One failed install (e.g. a tap-side transient error) shouldn't block the rest. The user gets exit 1 with a stderr line per failure and can retry.
> *Rejected*: abort-on-first-failure. Less useful, and inconsistent with `update`.

## Test seam

All `install_test.go` tests inject a fake via `proc.Runner` (`installFakeRunner` t.Cleanup helper, shared with `update_test.go`). No real brew subprocess is ever spawned.

Covered scenarios (`src/cmd/shll/install_test.go`):

- `TestInstall_BrewMissing` — `proc.Run("brew", "--version")` returns `ErrNotFound` → stderr hint, exit 1, no install attempted.
- `TestInstall_AllAlreadyInstalled` — every `brew list` succeeds → stdout `"All sahil87 tools already installed."`, no install calls, exit 0.
- `TestInstall_NoneInstalled` — every `brew list` exit-1 → install all six roster tools, exit 0.
- `TestInstall_PartialInstalled` — only `hop` and `wt` installed → install the other four, skip hop/wt, no stderr.
- `TestInstall_NoBrewUpdateInvoked` — pin the no-metadata-refresh contract: `brew update --quiet` MUST NOT appear in the recorded calls.
- `TestInstall_OneInstallFails` — one roster install (the `fab-kit` formula, now last in the leaves-first order) exits non-zero → loop continues and attempts all six, exit 1. The test pins the formula by name (`fab-kit`), not by roster position, and asserts total install attempts == `len(Roster)`, so it is robust to the reorder.
- `TestInstall_HeadersAndTail` *(change 6vuo, golden updated)* — `hop`+`wt` installed; asserts the verbatim `[N/M]` headers over the missing subset (`==> [1/4] idea` … `==> [4/4] fab-kit`), the blank line before each subsequent header and before the tail, and the duration-bearing `Done — 4 of 4 tools succeeded in 1m12s.` tail (installs a deterministic clock).
- `TestInstall_EmptyCaseNoHeaderNoTail` *(change 6vuo)* — all installed → the one-line note only, no `==>` header and no `Done —`/duration tail.
- `TestInstall_PartialFailureTail` *(change 6vuo)* — all six missing, `fab-kit` fails → partial-failure tail `5 succeeded, 1 failed in 1m12s — see above.` (duration before the em-dash).
- `TestInstall_CounterPartialInstall` *(change 6vuo)* — only `idea` installed → missing subset `wt, tu, rk, hop, fab-kit` (5 tools, roster order) yields headers `[1/5]`..`[5/5]` and the `Done — 5 of 5 …` tail (counter correctness).
- `TestInstall_DryRunPreview` *(change 6vuo)* — `hop`+`wt` installed → verbatim aligned-column preview `Would install 4 tools:` then `brew install sahil87/tap/<formula>` rows; asserts no "metadata refresh" mention.
- `TestInstall_DryRunNoWrites` *(change 6vuo)* — `brew list` probe IS recorded; no `brew install` for any tool; zero `TransportForeground` calls.
- `TestInstall_DryRunEmptyCase` *(change 6vuo)* — all installed → dry-run mirrors the nothing-to-do message, no preview table, no install, exit 0.
- `TestInstall_SubsetUnknownTargetHardErrors` *(change b2vg)* — `shll install <typo>` → `errSilent`, stderr lists valid targets, no `brew` subprocess runs.
- `TestInstall_SubsetShllRejected` *(change b2vg)* — `shll install shll` → the unknown-target error (`shll` is not a valid install target).
- `TestInstall_SubsetArgOrderIndependentRosterOrder` *(change b2vg)* — `shll install fab-kit wt` (both missing) → installs `wt` before `fab-kit` (roster order).
- `TestInstall_SubsetNamedAlreadyInstalled` *(change b2vg)* — `shll install hop` when hop is already installed → the `All sahil87 tools already installed.` nothing-to-do note, exit 0.
- `TestInstall_SubsetDryRunPreviewFiltered` *(change b2vg)* — `shll install --dry-run` of a subset → preview lists only the named-and-missing subset in roster order, exit 0, no write.
- `TestInstall_BrewTrustOverride_PerGOOS` *(change 38a6)* — swaps `goosFunc`: on linux the recorded `brew install` Request carries `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` in `Env`; on darwin the same Request carries no override (`Env` empty). Pins the Linux brew trust workaround at the install site.

The shared resolver is unit-tested directly in `tools_test.go` (shared with `update` — see [cli/update test seam](update.md#test-seam)); `install` is the `allowShll=false` caller.

Per-tool header/tail behavior (change y630) plus the change-6vuo `[N/M]` counter, duration, and install-preview helper are unit-tested against the `ui.go` helpers in `ui_test.go` (shared with `update`); `install_test.go` additionally asserts loop-path runs emit `==> [N/M] <tool>` headers and the plain tail to the **stdout** buffer (not stderr), and that the empty-case golden string is unchanged.

## Changelog

- **change 38a6** (`260613-38a6-brew-no-tap-trust-workaround`): The `brew install` foreground site now calls `proc.RunForegroundEnv(ctx, brewEnv(), …)` to inject the Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` trust workaround (`install.go:147`). See [Linux brew trust workaround](#linux-brew-trust-workaround-change-38a6). **TEMPORARY** — removed under backlog `[tkch]`.

## Cross-references

- Subprocess wrapper conventions: [internal/proc](../internal/proc.md).
- The hardcoded roster: [cli/commands](commands.md#hardcoded-tool-roster).
- Sibling lifecycle command: [cli/update](update.md) — the upgrade-already-installed counterpart; the [per-tool header/tail contract](update.md#per-tool-output-separation-change-y630) is documented there and shared via `ui.go`.
- Shared UI helper (`ui.go`): [cli/commands](commands.md#file-layout-srccmdshll).
- Constitution III (Wrap, Don't Reinvent), IV (Composition, Not Replacement), V (Graceful Degradation), VII (Minimal Surface Area).
