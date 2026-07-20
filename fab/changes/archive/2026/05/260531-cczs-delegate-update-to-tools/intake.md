# Intake: Delegate `shll update` to per-tool `update` subcommands

**Change**: 260531-cczs-delegate-update-to-tools
**Created**: 2026-05-31
**Status**: Draft

## Origin

Initiated from a `/fab-discuss` session exploring two complaints about `shll update`:

1. **Unresponsive UX** — first visible output arrives too late.
2. **Suspected daemon-restart gap** — does `shll update` restart the rk daemon the way `rk update` does?

The discussion verified the gap against real source and settled a design interactively (multiple `AskUserQuestion` rounds). Key interaction decisions:

- Confirmed (by reading `run-kit/app/backend/cmd/rk/upgrade.go`) that `rk update` calls `daemon.RestartWithBinary(...)` after `brew upgrade` — and that `shll update`, which calls `brew upgrade sahil87/tap/rk` directly, **never** triggers that restart. The restart lives in rk's CLI, not a brew post-install hook, so `brew upgrade` alone cannot reproduce it.
- User chose the **toolkit-wide convention** path: a `--skip-brew-update` flag added to every tool's `update` subcommand (now in place across all 6 tools — fab-kit, rk, tu, hop, wt, idea). The flag makes a tool's own `update` skip only its internal `brew update --quiet` step.
- User chose **probe-first detection** (over assume-support or assume+retry) for deciding whether a tool's `update` accepts the flag — checking `<tool> update --help` for the literal `--skip-brew-update` string.
- User chose **retry-without-flag** as the version-skew contract; probe-first implements that safely by simply not passing the flag when unsupported (avoids the false-positive where a real upgrade failure looks like a flag-parse error).
- For latency: combine a **status line** (instant first byte) with **parallelized read-only probes** (collapse ~8 sequential `brew` spawns into ~1 wall-clock).

## Why

**Problem 1 — Correctness (Constitution IV violation).** `runUpdate` in `src/cmd/shll/update.go:95` upgrades each roster tool with `proc.RunForeground(ctx, "brew", "upgrade", t.Formula)`. This bypasses each tool's own `update` logic. For rk specifically, the consequence is concrete and user-visible: after `shll update` upgrades rk, the brew symlink points at the new binary but the **running rk daemon** (`rk serve` under tmux) keeps its old mapped image. The web UI serves stale code until the user manually runs `rk daemon restart` or `rk update`. This is exactly the "absorbing logic minus a step" failure mode Constitution Principle IV warns against — shll partially reimplements `rk update` and silently drops the restart. Any future tool with post-upgrade side effects has the same latent bug.

**Problem 2 — UX latency.** Before the first visible byte, `runUpdate` performs, all silent and sequential: `hasBrew` (1 `brew --version`), the roster install-filter loop (6 `brew list` probes), and the shll-self probe (1 more `brew list`) — ~8 sequential brew spawns. Each pays Homebrew's Ruby-startup tax (~200–500ms), so the user stares at a blank terminal for ~2–4s before `brew update --quiet` produces the first output.

**Consequence if unfixed.** The rk daemon staleness is a silent correctness bug that erodes trust in `shll update` (users think they updated, but the running service didn't). The latency makes the tool feel broken/hung on every invocation.

**Why this approach over alternatives.** Delegating to `<tool> update` is the faithful Principle IV composition — each tool stays authoritative over its own upgrade + side effects. The `--skip-brew-update` flag resolves the only real objection (N redundant `brew update` metadata refreshes) by hoisting that one shared step into shll. Rejected alternatives, from the discussion: (a) hardcoding rk's daemon restart into shll — a Principle IV smell and doesn't generalize; (b) documenting the gap as a known limitation — leaves the correctness bug live; (c) reordering `brew update` first to fix latency — breaks Design Decision #9's nothing-to-do short-circuit.

## What Changes

### Change Area 1 — `Tool` struct gains an `Update []string` capability field

In `src/cmd/shll/tools.go`, add an `Update []string` field to the `Tool` struct, mirroring the existing `ShellInit []string` field exactly (same "empty slice means no capability" semantics, same documentation style). It holds the argv of the tool's update invocation.

Populate it for all six roster entries — every current tool has an `update` subcommand (verified during discussion; this also corrects the stale roster comment and `context.md` table that claimed only some tools have `update`):

```go
var Roster = []Tool{
	{Name: "fab-kit", Formula: formulaPrefix + "fab-kit", Update: []string{"fab-kit", "update"}},
	{Name: "rk", Formula: formulaPrefix + "rk", Update: []string{"rk", "update"}},
	{Name: "tu", Formula: formulaPrefix + "tu", ShellInit: []string{"tu", "shell-init", shellPlaceholder}, Update: []string{"tu", "update"}},
	{Name: "hop", Formula: formulaPrefix + "hop", ShellInit: []string{"hop", "shell-init", shellPlaceholder}, Update: []string{"hop", "update"}},
	{Name: "wt", Formula: formulaPrefix + "wt", ShellInit: []string{"wt", "shell-init", shellPlaceholder}, Update: []string{"wt", "update"}},
	{Name: "idea", Formula: formulaPrefix + "idea", Update: []string{"idea", "update"}},
}
```

### Change Area 2 — `runUpdate` reworked to delegate + parallel probe + status line

In `src/cmd/shll/update.go`, restructure `runUpdate` (`src/cmd/shll/update.go:34`):

**a. Status line first.** Before any probing, write an instant-feedback line to stdout, e.g. `Checking installed sahil87 tools…`. This is the first visible byte. (Exact wording is a Tentative detail — see Assumptions.)

**b. Parallel capability probes.** Replace the sequential install-filter loop with concurrent probing across the roster. For each tool determine two facts:
- **Installed?** — existing `isInstalled(ctx, t.Formula)` (`brew list --formula --versions`).
- **Supports `--skip-brew-update`?** — run `<tool> update --help` via `proc.Run` (capture) and check whether the output contains the literal string `--skip-brew-update` (presence check, not a flag parser). Only meaningful for installed tools that have an `Update` argv.

These probes are read-only (no brew write lock, captured output) and therefore safe to run concurrently — this is the carve-out to the existing "Sequential, not parallel" design decision, which applies to **upgrades** only. The implementation must preserve roster order in the **results** even though probing is concurrent (e.g., index the results slice, or sort after collection), because the subsequent upgrade loop and `shll shell-init` semantics rely on roster order.

**c. Hoisted `brew update --quiet` once.** After probing, run `proc.RunForeground(ctx, brewBinary, "update", "--quiet")` exactly once — the common metadata refresh, foregrounded, first visible brew output. Preserve the existing failure handling (`shll update: brew update failed: …` → `errSilent`).

**d. Per-tool upgrade, roster order, best-effort.** For each installed tool:
- has `Update` argv **and** supports `--skip-brew-update` → `proc.RunForeground(ctx, <tool>, "update", "--skip-brew-update")` (i.e., the `Update` argv with the flag appended).
- has `Update` argv but **not** flag-supported (version skew) → `proc.RunForeground(ctx, <Update argv...>)` with no flag.
- has **no** `Update` argv (hypothetical future tool) → fall back to `proc.RunForeground(ctx, brewBinary, "upgrade", t.Formula)` (today's behavior).

Keep the existing `anyFailed` best-effort policy: a per-tool failure sets `anyFailed = true` and continues; after the loop, `anyFailed` → return `errSilent`. (Constitution V — Graceful Degradation; spec Assumption #16.)

**e. shll self-upgrade unchanged.** Still `proc.RunForeground(ctx, brewBinary, "upgrade", shllFormula)` when shll itself is brew-installed, before the roster loop, with the same best-effort handling. shll has no `update` subcommand to call on itself, and is intentionally not in `Roster`.

**f. Preserved invariants:**
- Brew-missing hint verbatim (`brewMissingHint`, asserted literally by an existing spec scenario).
- Design Decision #9 nothing-to-do short-circuit: when no roster tool is installed AND shll itself is not brew-installed, print `No sahil87 tools installed.` and return nil **without** running `brew update`. (The status line must not break this — consider whether the status line should print before or after the short-circuit; see Open Questions.)
- All subprocess calls route through `internal/proc` (Constitution I).

### Change Area 3 — Tests

Update `src/cmd/shll/update_test.go` (uses the `proc.Runner` fake seam). New/updated assertions:
- A tool that advertises `--skip-brew-update` is upgraded via `<tool> update --skip-brew-update`, **not** `brew upgrade <formula>`.
- A tool whose `update --help` does **not** advertise the flag is upgraded via `<tool> update` (no flag).
- A (hypothetical) tool with no `Update` argv falls back to `brew upgrade <formula>`.
- The `--help` probe is issued for installed tools.
- `brew update --quiet` still runs exactly once; the nothing-to-do short-circuit still skips it (`TestUpdate_NoToolsInstalled` semantics preserved).
- Existing ordering/self-upgrade/best-effort scenarios still hold under the new structure.

### Change Area 4 — Docs / memory

- `docs/memory/cli/update.md` — rewrite the behavior contract to describe delegation, probe-first detection, parallel probes, the status line, and the `Update` capability field. Update the "Foreground vs capture" and "Sequential, not parallel" sections with the probe carve-out.
- `docs/memory/cli/commands.md` — update the roster description (`Update` field, all 6 tools have `update`).
- `fab/project/context.md` — correct the inverted roster table (it currently claims `tu` has `update` and `wt`/`idea` don't; reality: all 6 have `update`). Done at hydrate.

## Affected Memory

- `cli/update`: (modify) Rewrite the behavior contract for delegation + probe-first detection + parallel probes + status line.
- `cli/commands`: (modify) Roster now carries an `Update` capability field; note all 6 tools expose `update`.
- `internal/proc`: (no change expected) — still the sole subprocess wrapper; concurrency is in the caller, not proc.

## Impact

- **Code**: `src/cmd/shll/tools.go` (struct + roster), `src/cmd/shll/update.go` (`runUpdate` rework), `src/cmd/shll/update_test.go` (tests). Possibly a small concurrency helper if the parallel-probe logic warrants extraction.
- **External dependency**: Relies on the toolkit-wide `--skip-brew-update` contract (added to all 6 tools' `update` commands). Probe-first detection means shll degrades gracefully if a given installed tool predates the flag — it simply runs `<tool> update` without it.
- **Constitution**: Directly serves Principle IV (composition) and III (wrap, don't reinvent). Concurrency must not violate I (all calls through `internal/proc`) or muddy foreground output (probes are captured, so no interleaving — only the serial upgrades are foregrounded).
- **No new top-level subcommand** → Principle VII not triggered (this is a behavior change to existing `update`, not a new surface).

## Open Questions

_All intake-level open questions resolved during the 2026-05-31 clarify session — see ## Clarifications._

- ~~Status-line wording~~ → `Checking installed sahil87 tools…` (clarified). <!-- clarified: status-line wording -->
- ~~Status line before/after the nothing-to-do short-circuit~~ → probe first, status line printed before probes (and before the short-circuit); the empty case prints "Checking…" then "No sahil87 tools installed." (clarified). <!-- clarified: status-line order vs short-circuit -->
- ~~Bound probe concurrency?~~ → unbounded at roster size 6 (clarified); revisit only if the roster grows substantially. <!-- clarified: probe concurrency unbounded -->

> Implementation note for spec: the status line prints unconditionally before probing; the nothing-to-do short-circuit (Design Decision #9) still runs after probing and still skips `brew update`, so the empty-case output is `Checking installed sahil87 tools…\nNo sahil87 tools installed.`

## Clarifications

### Session 2026-05-31 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 5 | Confirmed | — |
| 6 | Confirmed | — |
| 7 | Confirmed | — |
| 8 | Confirmed | — |

### Session 2026-05-31 (questions)

| # | Q | Answer |
|---|---|--------|
| 9 | Status-line wording? | `Checking installed sahil87 tools…` |
| — | Status line before/after the nothing-to-do short-circuit? | Probe first; status line before probes (and before the short-circuit) |
| 10 | Bound probe concurrency? | Unbounded at roster size 6 |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Daemon-restart gap is real: `rk update` restarts the daemon, `shll update`'s `brew upgrade rk` does not | Verified by reading `run-kit/app/backend/cmd/rk/upgrade.go` during discussion — `daemon.RestartWithBinary` is in rk's CLI, not a brew hook | S:98 R:80 A:95 D:95 |
| 2 | Certain | Fix is delegation to `<tool> update` with hoisted `brew update` via `--skip-brew-update` | User explicitly chose toolkit-wide convention in discussion; flag now built into all 6 tools | S:95 R:55 A:90 D:90 |
| 3 | Certain | Detection is probe-first via `<tool> update --help` string check | User explicitly chose probe-first over assume-support and assume+retry | S:95 R:70 A:90 D:92 |
| 4 | Certain | `Update []string` capability field on `Tool`, mirroring `ShellInit` | Established pattern in the same struct; user-directed; all 6 tools get an argv | S:95 R:75 A:95 D:95 |
| 5 | Certain | Parallelize read-only probes; keep upgrades sequential | Clarified — user confirmed | S:95 R:70 A:85 D:80 |
| 6 | Certain | Status line printed before probes for instant first byte | Clarified — user confirmed | S:95 R:90 A:80 D:75 |
| 7 | Certain | Preserve Design Decision #9 (nothing-to-do skips `brew update`) and #3 (upgrades sequential), best-effort `anyFailed`, verbatim brew-missing hint | Clarified — user confirmed | S:95 R:60 A:90 D:85 |
| 8 | Certain | Version-skew handling = omit the flag when probe says unsupported (implements "retry without flag" safely) | Clarified — user confirmed | S:95 R:75 A:88 D:82 |
| 9 | Certain | Status-line wording is exactly `Checking installed sahil87 tools…`, printed before the probes (and before the nothing-to-do short-circuit) | Clarified — user confirmed | S:95 R:95 A:60 D:55 |
| 10 | Certain | Probe concurrency is unbounded (no worker pool) at current roster size | Clarified — user confirmed | S:95 R:85 A:70 D:60 |

10 assumptions (10 certain, 0 confident, 0 tentative, 0 unresolved).
