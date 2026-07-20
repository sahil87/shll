# Plan: Delegate `shll update` to per-tool `update` subcommands

**Change**: 260531-cczs-delegate-update-to-tools
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

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

## Tasks

<!-- Sequential work items for the apply stage. Checked off [x] as completed. -->

### Phase 1: Setup

- [x] T001 Add `Update []string` field to the `Tool` struct in `src/cmd/shll/tools.go`, mirroring the existing `ShellInit []string` field (same "empty slice means no capability" semantics, same doc-comment style). It holds the argv of the tool's update invocation. <!-- A-008 -->
- [x] T002 Populate `Update` for all six `Roster` entries in `src/cmd/shll/tools.go`: `{"<name>", "update"}` for fab-kit, rk, tu, hop, wt, idea (first element is the binary name, second is `update`). <!-- A-007 -->

### Phase 2: Core Implementation

- [x] T003 In `src/cmd/shll/update.go`, write the instant-feedback status line `Checking installed sahil87 tools…` to stdout before any probing (first visible byte), printed unconditionally — before the nothing-to-do short-circuit. Introduce a named constant for the status-line text (no magic string). <!-- A-006 -->
- [x] T004 In `src/cmd/shll/update.go`, replace the sequential install-filter loop with concurrent read-only probes across the roster. For each tool capture two facts: installed (via existing `isInstalled(ctx, t.Formula)`) and whether `<tool> update --help` (via `proc.Run`) output contains the literal substring `--skip-brew-update` (presence check, no regex — only probed for installed tools that have an `Update` argv). Assemble results in roster order regardless of completion order (index into a fixed-size slice). Concurrency lives in the caller; all subprocess calls route through `internal/proc`. Introduce a named constant for the `--skip-brew-update` flag string. <!-- A-004, A-005 -->
- [x] T005 In `src/cmd/shll/update.go`, keep the hoisted single `proc.RunForeground(ctx, brewBinary, "update", "--quiet")` after probing (preserving existing `shll update: brew update failed: …` → `errSilent` handling), and keep the nothing-to-do short-circuit (`No sahil87 tools installed.`, no `brew update`, DD#9) — now running after probing and after the status line prints. <!-- A-003, A-006 -->
- [x] T006 In `src/cmd/shll/update.go`, keep the shll self-upgrade (`proc.RunForeground(ctx, brewBinary, "upgrade", shllFormula)`) before the roster loop, then rework the per-tool upgrade loop (roster order, best-effort `anyFailed`): supports-flag → `<tool> update --skip-brew-update`; has `Update` argv but no flag → `<tool> update` (argv as-is); no `Update` argv → `brew upgrade <formula>` fallback. <!-- A-001, A-002 -->

### Phase 3: Integration & Edge Cases

- [x] T007 Make the `fakeRunner` in `src/cmd/shll/update_test.go` goroutine-safe (add a `sync.Mutex` guarding `calls` and the `respond` dispatch) since probes now run concurrently. <!-- A-013 -->
- [x] T008 Update/add tests in `src/cmd/shll/update_test.go` per the spec scenarios: flag-supported → `<tool> update --skip-brew-update` and not `brew upgrade <formula>`; no-flag (help lacks substring) → `<tool> update` with no flag and no brew-upgrade fallback; tool with empty `Update` argv → `brew upgrade <formula>` fallback; `--help` probe issued only for installed tools; `brew update --quiet` runs exactly once; nothing-to-do still skips `brew update`; status line precedes probes (and appears in nothing-to-do case); ordering/self-upgrade/best-effort preserved under the new structure. <!-- A-009, A-010, A-011, A-012, A-014 -->

### Phase 4: Polish

- [x] T009 Run `cd src && go test ./cmd/shll/... ./internal/proc/...`, `go build ./...`, and `go vet ./...`; fix any failures. <!-- A-015 -->

## Execution Order

- T001 blocks T002 (field must exist before roster populates it).
- T002 blocks T004/T006 (upgrade/probe logic reads `Update`).
- T003-T006 all edit `runUpdate` in `update.go` — execute sequentially as one coherent rework.
- T007 blocks T008 (fake must be concurrency-safe before concurrent-probe tests run).
- T009 runs last (validation gate).

## Acceptance

### Functional Completeness

- [x] A-001 Upgrade via tool's own `update`: each installed roster tool with an `Update` argv is upgraded via its `update` subcommand (with `--skip-brew-update` appended when supported), not `brew upgrade <formula>`.
- [x] A-002 Brew-upgrade fallback: a roster tool with an empty `Update` argv is upgraded via `brew upgrade <formula>`.
- [x] A-003 Hoisted single `brew update`: `brew update --quiet` is invoked exactly once per run, foregrounded, after probing and before upgrades; on failure shll writes `shll update: brew update failed: …` to stderr and returns `errSilent` without attempting upgrades.
- [x] A-004 Probe-first detection: flag support is determined by `<tool> update --help` literal-substring check on `--skip-brew-update` (no regex); when unsupported, the tool's `update` runs without the flag (no brew-upgrade fallback).
- [x] A-005 Parallel read-only probes: per-tool installed + flag-support probes are dispatched concurrently, with results assembled in roster order; all subprocess calls route through `internal/proc`.
- [x] A-006 Status line: `Checking installed sahil87 tools…` is written to stdout before probes and before the nothing-to-do short-circuit; the nothing-to-do case still skips `brew update` (DD#9).
- [x] A-007 Roster `Update` populated: all six roster entries have a non-empty `Update` argv whose first element is the binary name and second is `update`.
- [x] A-008 `Tool.Update` field: the `Tool` struct gains an `Update []string` field mirroring `ShellInit` (empty slice = no `update` subcommand → brew-upgrade fallback).

### Scenario Coverage

- [x] A-009 Test: flag-supported tool upgraded via `<tool> update --skip-brew-update`, not `brew upgrade <formula>`.
- [x] A-010 Test: version-skew tool (help lacks substring) upgraded via `<tool> update` with no flag and no brew-upgrade fallback.
- [x] A-011 Test: tool with empty `Update` argv falls back to `brew upgrade <formula>`.
- [x] A-012 Test: `update --help` probe issued only for installed tools; `brew update --quiet` runs exactly once; nothing-to-do skips `brew update`; status line precedes probes and appears in nothing-to-do output.
- [x] A-014 Test: ordering (self-upgrade before roster), self-not-brewed skip, and best-effort continue-through-failure preserved under the new structure.

### Edge Cases & Error Handling

- [x] A-013 Concurrency safety: the test `fakeRunner` is goroutine-safe (mutex-guarded) so concurrent probes do not race; `go test -race` is clean.

### Code Quality

- [x] A-007Q Pattern consistency: new code follows surrounding naming/structure (named constants for the status line and flag string, `ShellInit`-style doc comment for `Update`).
- [x] A-008Q No unnecessary duplication: existing helpers (`isInstalled`, `proc.Run`, `proc.RunForeground`, named brew constants) reused; no reimplementation.
- [x] A-016 No magic strings: the status-line text and `--skip-brew-update` flag are named constants (code-quality.md anti-pattern).
- [x] A-017 No regex over brew/help output: flag detection is a literal substring presence check (`strings.Contains`), not regex (code-quality.md anti-pattern).
- [x] A-018 Subprocess routing: every subprocess call goes through `internal/proc`; concurrency lives in the caller, not `proc` (Constitution I).
- [x] A-019 Sub-tool composition: upgrade delegates to each tool's own `update` rather than reimplementing per-tool logic (Constitution III/IV).

### Build & Validation

- [x] A-015 `go test ./cmd/shll/... ./internal/proc/...`, `go build ./...`, and `go vet ./...` all pass.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
