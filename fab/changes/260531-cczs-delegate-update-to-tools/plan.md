# Plan: Delegate `shll update` to per-tool `update` subcommands

**Change**: 260531-cczs-delegate-update-to-tools
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

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
