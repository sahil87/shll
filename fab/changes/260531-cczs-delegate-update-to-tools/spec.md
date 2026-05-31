# Spec: Delegate `shll update` to per-tool `update` subcommands

**Change**: 260531-cczs-delegate-update-to-tools
**Created**: 2026-05-31
**Affected memory**: `docs/memory/cli/update.md`, `docs/memory/cli/commands.md`

## Non-Goals

- **Changing per-tool `update` behavior** — the `--skip-brew-update` flag is already implemented in each tool's own repo. This change only makes `shll` *consume* it.
- **Adding/removing roster tools or top-level subcommands** — Constitution VII is not engaged; this is a behavior change to the existing `update` command.
- **Parallelizing upgrades** — Design Decision #3 stands; only read-only probes are parallelized.
- **Caching probe results across invocations** — Constitution II (no state); every invocation re-probes.
- **Restarting the rk daemon from shll directly** — the restart is rk's responsibility, reached by delegating to `rk update` (Constitution IV).

## cli/update: Delegation to per-tool `update`

### Requirement: Upgrade via the tool's own `update` subcommand

`shll update` SHALL upgrade each installed roster tool by invoking that tool's own `update` subcommand (the `Tool.Update` argv) rather than calling `brew upgrade <formula>` directly, so that each tool's post-upgrade side effects (e.g. rk's daemon restart) are preserved (Constitution IV — Composition, Not Replacement). When a tool advertises support for `--skip-brew-update`, that flag SHALL be appended to the tool's `update` argv. When a roster tool has no `Update` argv (a hypothetical future tool), `shll update` SHALL fall back to `brew upgrade <formula>` for that tool.

#### Scenario: Installed tool that supports the flag

- **GIVEN** `rk` is installed and `rk update --help` advertises `--skip-brew-update`
- **WHEN** `shll update` runs
- **THEN** shll invokes `rk update --skip-brew-update` (via `proc.RunForeground`)
- **AND** shll does NOT invoke `brew upgrade sahil87/tap/rk` for rk

#### Scenario: rk daemon restart is preserved

- **GIVEN** `rk` is installed and supports the flag
- **WHEN** `shll update` upgrades rk via `rk update --skip-brew-update`
- **THEN** rk's own `update` performs its post-upgrade daemon restart
- **AND** the running rk daemon is no longer stale after `shll update` completes

#### Scenario: Future tool with no `Update` argv falls back to brew upgrade

- **GIVEN** a roster tool whose `Tool.Update` is an empty slice and is installed
- **WHEN** `shll update` runs
- **THEN** shll invokes `brew upgrade <formula>` for that tool (today's behavior)

### Requirement: Probe-first detection of `--skip-brew-update` support

`shll update` SHALL determine whether a tool's `update` accepts `--skip-brew-update` by invoking `<tool> update --help` (captured via `proc.Run`) and checking whether the output contains the literal substring `--skip-brew-update`. This is a presence check, not a flag parser, and MUST NOT use regex over the help output (code-quality.md anti-pattern). When the flag is not advertised (e.g. an older installed tool predating the contract), `shll update` SHALL invoke the tool's `update` argv WITHOUT the flag (graceful version-skew handling — Constitution V).

#### Scenario: Installed tool that does not advertise the flag (version skew)

- **GIVEN** `hop` is installed but `hop update --help` does NOT contain `--skip-brew-update`
- **WHEN** `shll update` runs
- **THEN** shll invokes `hop update` with no `--skip-brew-update` flag
- **AND** shll does NOT fall back to `brew upgrade sahil87/tap/hop`

#### Scenario: Help probe issued only for installed tools

- **GIVEN** `idea` is NOT installed
- **WHEN** `shll update` runs its capability probes
- **THEN** shll does NOT invoke `idea update --help` (uninstalled tools are skipped)

### Requirement: Hoisted single `brew update --quiet`

`shll update` SHALL invoke `brew update --quiet` exactly once per run (foregrounded via `proc.RunForeground`), as the shared tap-metadata refresh, after capability probing and before the per-tool upgrades. Because each delegated `<tool> update --skip-brew-update` skips its own internal `brew update`, the metadata refresh happens exactly once for the whole run rather than once per tool. On `brew update` failure, `shll update` SHALL write `shll update: brew update failed: <detail>` to stderr and return `errSilent` (exit 1) without attempting upgrades.

#### Scenario: brew update runs once for multiple tools

- **GIVEN** `rk`, `hop`, and `wt` are all installed and support the flag
- **WHEN** `shll update` runs
- **THEN** `brew update --quiet` is invoked exactly once
- **AND** each tool is upgraded via `<tool> update --skip-brew-update` (which each skip their own `brew update`)

#### Scenario: brew update failure aborts before upgrades

- **GIVEN** `brew update --quiet` exits non-zero
- **WHEN** `shll update` runs
- **THEN** shll writes `shll update: brew update failed: …` to stderr
- **AND** returns `errSilent` (exit 1)
- **AND** no per-tool upgrade is attempted

### Requirement: Parallel read-only capability probes

`shll update` SHALL perform its per-tool capability probes (installed check via `brew list --formula --versions`, and `--skip-brew-update` support via `<tool> update --help`) concurrently across the roster. Concurrency is permitted because these probes are read-only — they take no Homebrew write lock and their output is captured (not foregrounded), so there is no output interleaving. This is an explicit carve-out to the "sequential, not parallel" design decision, which applies to upgrades only. The probe results MUST be assembled in roster order regardless of completion order, because the upgrade loop relies on roster ordering. Probe concurrency is unbounded at the current roster size (6).

#### Scenario: Probes run concurrently, results ordered by roster

- **GIVEN** all six roster tools are installed
- **WHEN** `shll update` probes capabilities
- **THEN** the probes are dispatched concurrently
- **AND** the resulting per-tool upgrades are still issued in roster order (fab-kit, rk, tu, hop, wt, idea)

### Requirement: Instant first-byte status line

`shll update` SHALL write the line `Checking installed sahil87 tools…` to stdout before beginning capability probes, so the user receives immediate feedback rather than staring at a blank terminal during the probe phase. This line SHALL be printed unconditionally before probing, including in the nothing-to-do case.

#### Scenario: Status line precedes probes

- **GIVEN** any invocation where brew is present
- **WHEN** `shll update` runs
- **THEN** the first line written to stdout is `Checking installed sahil87 tools…`
- **AND** it appears before any `brew update` output

#### Scenario: Status line in the nothing-to-do case

- **GIVEN** no roster tool is installed AND shll itself is not brew-installed
- **WHEN** `shll update` runs
- **THEN** stdout contains `Checking installed sahil87 tools…` followed by `No sahil87 tools installed.`
- **AND** `brew update` is NOT invoked (Design Decision #9 preserved)

### Requirement: Preserved best-effort and graceful-degradation semantics

`shll update` SHALL preserve its existing best-effort policy: a failure upgrading any single tool (delegated `update` exits non-zero, transport error, or brew-upgrade fallback failure) sets an internal failure flag and the loop continues to the next tool; after the loop, if any upgrade failed, `shll update` returns `errSilent` (exit 1), otherwise nil (exit 0). The brew-missing hint (`shll update requires Homebrew. Install from https://brew.sh`) SHALL be emitted verbatim when brew is absent. The shll self-upgrade (`brew upgrade sahil87/tap/shll`, when shll is brew-installed) SHALL run before the roster loop with the same best-effort handling and is unaffected by delegation (shll has no `update` subcommand to call on itself).

#### Scenario: One tool's update fails, others continue

- **GIVEN** `rk`, `hop`, `wt` are installed and the delegated `rk update` exits non-zero
- **WHEN** `shll update` runs
- **THEN** shll still attempts `hop` and `wt` upgrades
- **AND** returns `errSilent` (exit 1) because at least one upgrade failed

#### Scenario: brew missing

- **GIVEN** `brew` is not on PATH
- **WHEN** `shll update` runs
- **THEN** stderr contains exactly `shll update requires Homebrew. Install from https://brew.sh`
- **AND** returns `errSilent` (exit 1)

#### Scenario: shll self-upgrade still runs and is not delegated

- **GIVEN** shll itself is brew-installed and at least one roster tool is installed
- **WHEN** `shll update` runs
- **THEN** shll runs `brew upgrade sahil87/tap/shll` before the roster loop
- **AND** does not attempt to call any `shll update` subcommand on itself

## cli/commands: Roster `Update` capability field

### Requirement: `Tool` struct carries an `Update` argv

The `Tool` struct (`src/cmd/shll/tools.go`) SHALL gain an `Update []string` field holding the argv of the tool's update invocation, mirroring the existing `ShellInit []string` field (an empty slice means the tool exposes no `update` subcommand and is upgraded via `brew upgrade` fallback). Every current roster entry SHALL populate `Update` because all six tools (fab-kit, rk, tu, hop, wt, idea) expose an `update` subcommand.

#### Scenario: All six roster tools have an Update argv

- **GIVEN** the `Roster` definition in `src/cmd/shll/tools.go`
- **WHEN** the roster is inspected
- **THEN** every entry has a non-empty `Update` argv whose first element is the tool's binary name and second is `update`

## Design Decisions

1. **Delegate to `<tool> update` instead of `brew upgrade <formula>`**
   - *Why*: Preserves each tool's post-upgrade side effects (rk's daemon restart), satisfying Constitution IV. `brew upgrade` alone reproduces only the binary swap, not the tool's own post-upgrade logic.
   - *Rejected*: Hardcoding rk's daemon restart into shll (Principle IV smell, doesn't generalize); documenting the gap as a known limitation (leaves the correctness bug live).

2. **Hoist `brew update --quiet` into shll once, via `--skip-brew-update`**
   - *Why*: Each tool's `update` would otherwise run its own `brew update`, causing N redundant metadata refreshes. The flag lets shll do it once.
   - *Rejected*: Letting each tool refresh independently (N× latency); having shll suppress refresh by other means (no cross-tool contract).

3. **Probe-first detection via `<tool> update --help`**
   - *Why*: Knowing flag support before calling avoids the false-positive where a genuine upgrade failure is mistaken for a flag-parse error (which a post-failure retry would suffer). Presence-check on `--help` is simple and side-effect-free.
   - *Rejected*: Assume-all-support (breaks on version skew — old tool rejects unknown flag); assume-and-retry-on-failure (false positives; re-runs side effects like daemon restart).

4. **Parallelize read-only probes, keep upgrades sequential**
   - *Why*: Probes take no brew write lock and are captured, so concurrency is safe and collapses ~8 sequential brew spawns into ~1 wall-clock. Upgrades stay sequential per Design Decision #3 (brew lock + interleaved foreground output).
   - *Rejected*: Sequential probes (the original latency source); parallel upgrades (DD#3 — lock contention, garbled output).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Delegate to `<tool> update` (with `--skip-brew-update` when supported), not `brew upgrade <formula>` | Confirmed from intake #1/#2; root of the Constitution IV fix; verified daemon-restart gap | S:98 R:55 A:92 D:92 |
| 2 | Certain | Probe-first detection via `<tool> update --help` literal substring check, no regex | Confirmed from intake #3; user-chosen; code-quality anti-pattern forbids regex over CLI output | S:95 R:70 A:90 D:92 |
| 3 | Certain | `Update []string` capability field on `Tool`, mirroring `ShellInit`; all 6 tools populated | Confirmed from intake #4; established struct pattern | S:95 R:75 A:95 D:95 |
| 4 | Certain | Hoist `brew update --quiet` once; each delegated update skips its own via the flag | Confirmed from intake #2; the reason the flag exists | S:95 R:65 A:92 D:90 |
| 5 | Certain | Parallelize read-only probes, assemble results in roster order; upgrades stay sequential | Confirmed from intake #5 (bulk confirm); carve-out to DD#3 noted in memory | S:95 R:70 A:85 D:80 |
| 6 | Certain | Status line `Checking installed sahil87 tools…` printed before probes, including nothing-to-do case | Confirmed from intake #6/#9 (bulk confirm + question) | S:95 R:90 A:80 D:75 |
| 7 | Certain | Preserve DD#9 (nothing-to-do skips brew update), DD#3 (sequential upgrades), best-effort anyFailed, verbatim brew-missing hint | Confirmed from intake #7 (bulk confirm); pinned existing scenarios must not regress | S:95 R:60 A:90 D:85 |
| 8 | Certain | Version skew = omit the flag when probe says unsupported (delegated update without flag, no brew-upgrade fallback) | Confirmed from intake #8 (bulk confirm); probe-first implements retry-without-flag safely | S:95 R:75 A:88 D:82 |
| 9 | Certain | Probe concurrency unbounded at roster size 6 | Confirmed from intake #10 (question); fine for 6 local probes | S:95 R:85 A:70 D:60 |
| 10 | Certain | shll self-upgrade unchanged (brew upgrade shllFormula, before roster loop, best-effort) | Confirmed from intake; shll has no update subcommand on itself; not in Roster | S:95 R:70 A:92 D:90 |

10 assumptions (10 certain, 0 confident, 0 tentative, 0 unresolved).
