# cli/install

`shll install` — installs every roster tool that isn't already installed via Homebrew. Idempotent; safe to re-run.

Source: `src/cmd/shll/install.go`, with shared brew helpers in `src/cmd/shll/brew.go`.

## Behavior contract

The full happy/unhappy paths, in the order `runInstall` evaluates them (`src/cmd/shll/install.go`):

1. **Brew missing.** If `hasBrew(ctx)` returns false, write `installBrewMissingHint` to stderr and return `errSilent`. Exit code: 1. The literal hint is `"shll install requires Homebrew. Install from https://brew.sh"` (`src/cmd/shll/brew.go`). It is intentionally separate from `brewMissingHint` (used by `shll update`) so each command's error tells the user which command they ran — the update spec scenario asserts its verbatim text, so reusing the same constant for both commands would either violate that lock or mislead `shll install` users.

2. **Partition the roster.** Iterate `Roster` in order, calling `isInstalled(ctx, t.Formula)`; collect the *missing* entries into a local `missing` slice.

3. **Nothing missing → short-circuit.** If `len(missing) == 0`, write `All sahil87 tools already installed.` to stdout and return nil. Exit code: 0. No `brew update` is invoked — there's nothing to install.

4. **No `brew update --quiet`.** Unlike `shll update`, `shll install` does NOT refresh brew metadata first. `brew install sahil87/tap/<formula>` resolves the formula via the tap directly, and the spec freezes this distinction (Design Decision: install ≠ update). `TestInstall_NoBrewUpdateInvoked` pins the contract.

5. **Sequential per-tool install.** For each missing tool in roster order, print its per-tool header (see [Per-tool output separation](#per-tool-output-separation-change-y630)) then run `proc.RunForeground(ctx, "brew", "install", t.Formula)`. Best-effort across the roster: on per-tool failure (transport error or non-zero exit), set `anyFailed = true` and `continue` — never abort the loop.

6. **Summary tail.** After the loop, print one summary line via `printSummaryTail` (see [Per-tool output separation](#per-tool-output-separation-change-y630)), then — unchanged — if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0). The tail is presentation-only and does not change the exit code.

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All installs succeeded (or all-already-installed branch) | 0 |
| `brew` not on PATH | 1 (via `errSilent`, hint already on stderr) |
| Any per-tool `brew install` failed | 1 (via `errSilent`, after all missing tools attempted) |

## Per-tool output separation (change y630)

`shll install` mirrors `shll update`'s framing exactly, via the same shared helper `src/cmd/shll/ui.go` (see [cli/commands](commands.md#file-layout-srccmdshll)) — no TTY/`NO_COLOR`/glyph logic is duplicated in `install.go`.

- **Per-tool header.** Before each missing tool's `brew install` output, `printToolHeader(stdout, t.Name, color)` writes `▸ <tool>` (color TTY) / `==> <tool>` (plain), in roster order. Since change auvj that order is leaves-first (`wt, idea, tu, rk, hop, fab-kit`), so the headers for the *missing subset* print in that relative order — e.g. with `hop`+`wt` already installed, the missing set `{idea, tu, rk, fab-kit}` yields `==> idea`, `==> tu`, `==> rk`, `==> fab-kit` (`TestInstall_HeadersAndTail` golden at `src/cmd/shll/install_test.go`, with the `Done — 4 of 4 tools succeeded.` tail). See the [leaves-first ordering rationale](commands.md#design-decision-leaves-first-roster-order-change-auvj).
- **Summary tail.** After the loop, `printSummaryTail(stdout, succeeded, len(missing), color)` writes `Done — N of M tools succeeded.` (green `✓` when color) or `X succeeded, Y failed — see above.`, by **exit code only** — `succeeded` counts installs that exited 0, mirroring the same per-tool facts that drive `anyFailed`. Presentation-only; does not change the exit code.
- **Stream discipline.** Header and tail go to **stdout** (the stream `brew install` is foregrounded onto), never stderr.
- **Color gating.** One `colorEnabled(stdout)` decision (TTY via `golang.org/x/term` AND `NO_COLOR` unset), reused for headers and tail; `bytes.Buffer` test writers hit the plain-ASCII branch.
- **Empty case emits no header and no tail.** The all-already-installed short-circuit (step 3) runs no loop, so its stdout stays **exactly** `All sahil87 tools already installed.\n` — the `TestInstall_AllAlreadyInstalled` golden string is preserved verbatim. Only the install-loop path carries the `==>`/tail markers.

The helper details (named SGR constants, the `colorEnabled` gating, the honesty constraint on the tail) are documented once under [cli/update](update.md#per-tool-output-separation-change-y630); `install` consumes the identical helpers.

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

Per-tool header/tail behavior (change y630) is unit-tested against the `ui.go` helpers in `ui_test.go`; `install_test.go` additionally asserts loop-path runs emit `==> <tool>` headers and the plain tail to the **stdout** buffer (not stderr), and that the empty-case golden string is unchanged.

## Cross-references

- Subprocess wrapper conventions: [internal/proc](../internal/proc.md).
- The hardcoded roster: [cli/commands](commands.md#hardcoded-tool-roster).
- Sibling lifecycle command: [cli/update](update.md) — the upgrade-already-installed counterpart; the [per-tool header/tail contract](update.md#per-tool-output-separation-change-y630) is documented there and shared via `ui.go`.
- Shared UI helper (`ui.go`): [cli/commands](commands.md#file-layout-srccmdshll).
- Constitution III (Wrap, Don't Reinvent), IV (Composition, Not Replacement), V (Graceful Degradation), VII (Minimal Surface Area).
