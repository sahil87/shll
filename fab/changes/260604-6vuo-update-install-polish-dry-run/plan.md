# Plan: Output polish + --dry-run for shll update and install

**Change**: 260604-6vuo-update-install-polish-dry-run
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

This change adds four additive features to BOTH `shll update` and `shll install`, building on
the already-shipped per-tool-output framing (`printToolHeader`, `printSummaryTail`, `colorEnabled`
in `ui.go`). It introduces no new top-level subcommand, no TUI, and no new heavy dependency. All
subprocess execution stays through `internal/proc` (Constitution I); `ui.go` stays presentation-only.

### UI: Progress counters in the per-tool header

#### R1: `[N/M]` progress counter on each per-tool header
`printToolHeader` SHALL accept a 1-based position `N` and total `M`, and emit
`▸ [N/M] <tool>` (color TTY) / `==> [N/M] <tool>` (plain). `M` MUST be known before the per-tool
loop begins. For `update`, `M = (count of installed roster tools) + (1 if shll is brew-installed)`,
and `shll (self)` is `[1/M]` and the FIRST header. For `install`, `M = len(missing)`.

- **GIVEN** shll is brew-installed and the full roster is installed
- **WHEN** `shll update` runs (plain, non-TTY)
- **THEN** the headers read `==> [1/7] shll (self)`, `==> [2/7] wt`, `==> [3/7] idea`, `==> [4/7] tu`, `==> [5/7] rk`, `==> [6/7] hop`, `==> [7/7] fab-kit`

- **GIVEN** hop and wt are already installed and the other four are missing
- **WHEN** `shll install` runs (plain)
- **THEN** the headers read `==> [1/4] idea`, `==> [2/4] tu`, `==> [3/4] rk`, `==> [4/4] fab-kit`

### UI: Section spacing

#### R2: blank line before each header except the first, and before the tail
The per-tool loop SHALL print a blank line before each per-tool header EXCEPT the first, and a
single blank line SHALL precede the summary tail (the last tool's streamed output is separated from
the tail by one blank line).

- **GIVEN** two or more tools are acted on
- **WHEN** the run streams tool output
- **THEN** each header after the first is preceded by exactly one `\n`, and the tail is preceded by exactly one `\n`
- **AND** the first header has no leading blank line, and the empty/short-circuit cases emit no blank lines

### UI: Summary timing

#### R3: wall-clock duration appended to both tail forms
`printSummaryTail` SHALL accept a `time.Duration` and append it to BOTH forms:
success → `Done — N of M tools succeeded in <dur>.` (green `✓` prefix on color TTY);
partial-failure → `X succeeded, Y failed in <dur> — see above.` (duration BEFORE the em-dash).
Duration is a run fact, not an outcome claim — the existing honesty constraint (never "updated"
vs "up-to-date") MUST be preserved. Format is restrained: rounded to whole seconds for multi-second
runs via `time.Duration.Round`.

- **GIVEN** a 72-second run that fully succeeds
- **WHEN** the tail prints (plain)
- **THEN** it reads `Done — 7 of 7 tools succeeded in 1m12s.`

- **GIVEN** a 72-second run where 1 of 6 failed
- **WHEN** the tail prints (plain)
- **THEN** it reads `5 succeeded, 1 failed in 1m12s — see above.`

### Seam: Injectable clock

#### R4: package-level swappable clock seam (mirrors `proc.Runner`)
A package-level `var nowFunc = time.Now` (in a small `clock.go`) SHALL be the single source of
wall-clock time used to compute run duration. Production wiring uses real `time.Now`; tests swap it
via a `t.Cleanup` helper (mirroring `installFakeRunner`) to a deterministic clock so golden strings
stay exact. `runUpdate`/`runInstall` capture a start time at the top of the acted-on path (after the
nothing-to-do short-circuit) and compute `elapsed` immediately before printing the tail.

- **GIVEN** a test installs a deterministic clock returning t0 then t0+72s
- **WHEN** `runUpdate`/`runInstall` complete a loop run
- **THEN** the tail shows exactly `in 1m12s`
- **AND** the empty/short-circuit cases (which print no tail) call `nowFunc` zero or harmlessly and assert no duration text

### Update: `--dry-run` flag

#### R5: `shll update --dry-run` previews without writes
`update` SHALL gain a cobra bool flag `--dry-run`. When set, `runUpdate` runs the read-only probes
(`brew list`, `<tool> update --help`, shll-self `brew list`) but performs NO writes: NO
`brew update --quiet`, NO `brew upgrade`, NO `<tool> update`. It prints an aligned-column preview
and returns nil (exit 0).

- Header line: `Would update N tools (brew metadata refresh first):`
- Tool labels left-padded to the longest label present (including `shll (self)`), 2 spaces between
  the label column and the command, 2-space row indent.
- Rows in roster order, `shll (self)` first when brew-installed.
- Per-tool argv EXACTLY as `upgradeTool` would build it:
  has-flag → `<tool> update --skip-brew-update`; no-flag → `<tool> update`;
  no-Update-argv → `brew upgrade sahil87/tap/<formula>`; self → `brew upgrade sahil87/tap/shll`.
- Empty case (nothing installed AND shll not brew-installed): print `No sahil87 tools installed.`
  (mirrors the non-dry-run short-circuit), exit 0.
- Brew-missing still bails first (stderr hint, exit 1) — dry-run does not change that precondition.

- **GIVEN** shll not brew-installed; wt, idea, tu, rk, hop, fab-kit installed; only rk and hop advertise the flag
- **WHEN** `shll update --dry-run` runs (plain)
- **THEN** stdout contains `Would update 6 tools (brew metadata refresh first):` then aligned rows in roster order
- **AND** the recorded proc calls contain the read-only probes (`brew list`, `<tool> update --help`) but NOT `brew update --quiet`, `brew upgrade`, or any `<tool> update` write

#### R6: `--dry-run` lists only actionable tools (graceful degradation)
The update preview SHALL list only installed tools (plus `shll (self)` when brew-installed);
uninstalled tools are omitted (Constitution V).

- **GIVEN** only hop and wt installed, shll not brew-installed
- **WHEN** `shll update --dry-run` runs
- **THEN** the preview lists exactly `wt` and `hop` (roster order), header `Would update 2 tools (brew metadata refresh first):`

### Install: `--dry-run` flag

#### R7: `shll install --dry-run` previews without writes
`install` SHALL gain a cobra bool flag `--dry-run`. When set, `runInstall` runs the read-only
`isInstalled` probes but performs NO `brew install`. It prints an aligned-column preview and returns
nil (exit 0). Header line: `Would install N tools:` (no metadata-refresh line — install has none).
Rows list `brew install sahil87/tap/<formula>` per missing tool, in roster order, with the same
aligned-column layout (labels padded to the longest missing label). Empty case (all already
installed): print `All sahil87 tools already installed.` (mirrors the short-circuit), exit 0.

- **GIVEN** hop and wt installed; idea, tu, rk, fab-kit missing
- **WHEN** `shll install --dry-run` runs (plain)
- **THEN** stdout reads `Would install 4 tools:` then aligned rows `idea`/`tu`/`rk`/`fab-kit` → `brew install sahil87/tap/<formula>`
- **AND** the recorded proc calls contain `brew list` probes but NOT any `brew install`

### Design Decisions

1. **Clock seam = package-level `var nowFunc = time.Now`** — *Why*: mirrors the established
   `proc.Runner` package-level-swappable pattern exactly (intake preferred, Assumption #10), keeps
   `runUpdate`/`runInstall` signatures unchanged, and lets a `t.Cleanup` helper swap it deterministically.
   *Rejected*: function parameter on `runUpdate`/`runInstall` (churns the signature and every call site/test);
   a `clock` interface (heavier than the one-function need).
2. **Duration format = `elapsed.Round(time.Second).String()`** — *Why*: yields the intake's exact
   `1m12s` example for multi-second runs and degrades sanely for sub-second runs (rounds to `0s`).
   The deterministic test clock uses a 72s delta to assert `1m12s` verbatim.
   *Rejected*: a custom sub-second formatter (intake gives the format; YAGNI).
3. **Dry-run preview helpers live in `ui.go`** (`printUpdatePreview`, `printInstallPreview` taking
   already-computed `[]previewRow`) — *Why*: keeps presentation/formatting in the presentation-only
   file (Constitution I; `ui.go` makes no subprocess calls); the commands compute the rows (argv) from
   probe results and pass them in. *Rejected*: inlining the formatting in `update.go`/`install.go`
   (duplicates the alignment logic across two files).
4. **Preview rows carry no counter/spacing** — the `[N/M]`/blank-line treatment is for the streaming
   loop only; the preview is a static aligned table (intake examples show no counters in the preview).

### Non-Goals

- No TUI/dashboard, no capture-and-reframe of child output, no lipgloss/charm dependency.
- `version` and `shell-init` are untouched (out of scope).
- No new top-level subcommand (`--dry-run` is a flag — Constitution VII not triggered).

## Tasks

### Phase 1: Clock seam (prerequisite for the duration work)

- [x] T001 Add `src/cmd/shll/clock.go` with a package-level `var nowFunc = time.Now` and a short doc comment explaining it mirrors the `proc.Runner` injection seam (production = real `time.Now`; tests swap via a cleanup helper). <!-- R4 -->
- [x] T002 Add `installFakeClock(t *testing.T, times ...time.Time)` test helper in `src/cmd/shll/clock_test.go` (mirroring `installFakeRunner`): swaps `nowFunc` to return the provided times in sequence (last value repeats), restores via `t.Cleanup`. Include a small unit test asserting the helper's sequencing. <!-- R4 -->

### Phase 2: ui.go presentation extensions

- [x] T003 Extend `printToolHeader` in `src/cmd/shll/ui.go` to take `pos, total int` and emit `▸ [N/M] <tool>` (color) / `==> [N/M] <tool>` (plain). <!-- R1 -->
- [x] T004 Extend `printSummaryTail` in `src/cmd/shll/ui.go` to take a `time.Duration` and append `in <dur>` to both forms: success `Done — N of M tools succeeded in <dur>.`; partial `X succeeded, Y failed in <dur> — see above.` (duration before the em-dash). Add a named `formatDuration` helper using `d.Round(time.Second).String()`. Preserve the honesty constraint. <!-- R3 -->
- [x] T005 Add named constants + `previewRow` struct + `printUpdatePreview`/`printInstallPreview` aligned-column helpers in `src/cmd/shll/ui.go`. Header constants: `updatePreviewHeaderFmt = "Would update %d tools (brew metadata refresh first):"`, `installPreviewHeaderFmt = "Would install %d tools:"`. Helpers compute the longest label across rows, left-pad labels, 2-space gap, 2-space indent; presentation-only (no subprocess calls). <!-- R5 R7 -->

### Phase 3: update.go + install.go wiring

- [x] T006 In `src/cmd/shll/update.go`: add `--dry-run` cobra bool flag; thread it into `runUpdate` (add a `dryRun bool` parameter, update `RunE`). Compute `M` up front (count `probes[i].installed` + `1` if `shllSelfInstalled`). Pass `[N/M]` positions into `printToolHeader`; emit blank line before each header except the first and before the tail; capture `start := nowFunc()` after the short-circuit and pass `nowFunc().Sub(start)` to `printSummaryTail`. <!-- R1 R2 R3 -->
- [x] T007 In `src/cmd/shll/update.go`: implement the dry-run branch — after probes + `shllSelfInstalled`, if `dryRun`: handle the empty case (`No sahil87 tools installed.`), else build `[]previewRow` (self first when brew-installed, then installed roster tools in order) using a `previewArgv` helper that returns the exact argv `upgradeTool` would build, call `printUpdatePreview`, return nil. NO `brew update`/`brew upgrade`/`<tool> update`. <!-- R5 R6 -->
- [x] T008 In `src/cmd/shll/install.go`: add `--dry-run` cobra bool flag; thread it into `runInstall` (add `dryRun bool`, update `RunE`). For the loop path, pass `[N/M]` (`M = len(missing)`) into `printToolHeader`, blank line before each header except the first and before the tail, capture start/elapsed via `nowFunc`, pass elapsed to `printSummaryTail`. If `dryRun`: handle empty case (`All sahil87 tools already installed.`), else build `[]previewRow` (`brew install <formula>` per missing tool) and call `printInstallPreview`, return nil. NO `brew install`. <!-- R3 R1 R2 R7 -->

### Phase 4: Tests

- [x] T009 Update `src/cmd/shll/ui_test.go`: header tests assert `[N/M]` in both forms; tail tests assert the `in <dur>` suffix in both forms (use a fixed duration, e.g. `72*time.Second` → `1m12s`); keep honesty assertions. Add tests for `printUpdatePreview`/`printInstallPreview` aligned-column output and `formatDuration`. <!-- R1 R3 R5 R7 -->
- [x] T010 Update `src/cmd/shll/update_test.go` golden strings: `TestUpdate_HeadersAndTail` now expects `[N/M]` headers, blank lines between sections + before tail, and `Done — 7 of 7 tools succeeded in 1m12s.` (install a deterministic clock). Preserve `TestUpdate_EmptyCaseNoHeaderNoTail`/`TestUpdate_NoToolsInstalled` golden strings EXACTLY (no counter/spacing/duration). Update calls to `runUpdate` for the new `dryRun` arg (pass `false`). <!-- R1 R2 R3 R4 -->
- [x] T011 Add `src/cmd/shll/update_test.go` dry-run tests: `TestUpdate_DryRunPreview` (exact aligned-column stdout for a known roster state), `TestUpdate_DryRunNoWrites` (assert read-only probes ARE recorded but `brew update --quiet`/`brew upgrade`/`<tool> update` writes are NOT), `TestUpdate_DryRunEmptyCase` (`No sahil87 tools installed.`), and counter correctness for a partial-install scenario. <!-- R5 R6 R1 -->
- [x] T012 Update `src/cmd/shll/install_test.go` golden strings: `TestInstall_HeadersAndTail` now expects `[N/M]` headers, blank-line spacing, and the `in 1m12s` tail (deterministic clock). Preserve `TestInstall_EmptyCaseNoHeaderNoTail`/`TestInstall_AllAlreadyInstalled` golden strings EXACTLY. Update `runInstall` calls for the new `dryRun` arg. Add `TestInstall_DryRunPreview`, `TestInstall_DryRunNoWrites`, `TestInstall_DryRunEmptyCase`, and a counter-correctness test for partial installs. <!-- R7 R1 R2 R3 R4 -->

### Phase 5: Verify

- [x] T013 `cd src && go build ./... && go vet ./...` clean; `go test ./cmd/shll/ -run 'Update|Install|Ui|DryRun|Header|Tail|Counter'` then `go test -race ./...` green. Fix failures. <!-- R1 R2 R3 R4 R5 R6 R7 -->

## Execution Order

- T001 → T002 (clock seam) before T004/T006/T008/T010/T012 (duration depends on the seam).
- T003, T004, T005 (ui.go) before T006, T007, T008 (commands call the extended helpers).
- T006, T007 (update.go) and T008 (install.go) before their respective test phases.
- T009–T012 after the implementation they assert; T013 last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `printToolHeader` emits `▸ [N/M] <tool>` (color) / `==> [N/M] <tool>` (plain); `M` is computed before the loop; update sequence shows `[1/7] shll (self)` first.
- [x] A-002 R2: A blank line precedes every per-tool header except the first, and a blank line precedes the summary tail; the first header has no leading blank.
- [x] A-003 R3: `printSummaryTail` appends `in <dur>` to both forms — `Done — N of M tools succeeded in 1m12s.` and `X succeeded, Y failed in 1m12s — see above.`
- [x] A-004 R4: A package-level `nowFunc` clock seam exists in `clock.go`; a `t.Cleanup` helper swaps it deterministically; production uses real `time.Now`.
- [x] A-005 R5: `shll update --dry-run` prints `Would update N tools (brew metadata refresh first):` with aligned columns and exact per-tool argv, exit 0, no writes.
- [x] A-006 R6: The update preview lists only installed tools (plus `shll (self)` when brew-installed); uninstalled tools omitted.
- [x] A-007 R7: `shll install --dry-run` prints `Would install N tools:` with aligned `brew install` rows, exit 0, no `brew install`.

### Behavioral Correctness

- [x] A-008 R3: Duration is additive/factual — the tail never claims "updated"/"up-to-date" (honesty assertions still pass).
- [x] A-009 R5: In `update --dry-run`, the read-only probes (`brew list`, `<tool> update --help`) ARE recorded while `brew update --quiet`, `brew upgrade`, and `<tool> update` writes are NOT.
- [x] A-010 R7: In `install --dry-run`, `brew list` probes ARE recorded while `brew install` is NOT.

### Scenario Coverage

- [x] A-011 R1: A counter-correctness test verifies `[N/M]` for a partial-install scenario (update and install).
- [x] A-012 R3 R4: A test installs a deterministic clock and asserts the exact `in 1m12s` in the tail.

### Edge Cases & Error Handling

- [x] A-013 R5 R7: Empty-case dry-run previews print the existing nothing-to-do messages (`No sahil87 tools installed.` / `All sahil87 tools already installed.`), exit 0, no preview table.
- [x] A-014 R2: Empty/short-circuit (non-dry-run) cases preserve their golden strings EXACTLY — no counter, no spacing, no duration, no header, no tail.
- [x] A-015 R5: Brew-missing precondition still bails first in `--dry-run` (stderr hint, exit 1).

### Code Quality

- [x] A-016 Pattern consistency: clock seam mirrors `proc.Runner`; preview helpers live in `ui.go` and follow the existing helper signature style.
- [x] A-017 No unnecessary duplication: the aligned-column logic is shared; argv construction reuses the same dispatch as `upgradeTool`.
- [x] A-018 Named constants (code-quality.md): new literal strings (preview headers, format) are named constants — no magic strings.
- [x] A-019 Constitution I: `ui.go` stays presentation-only (no subprocess calls); all execution stays through `internal/proc`; dry-run spawns no write subprocess. `TestNoProcImports` (if present) still passes.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Clock seam = package-level `var nowFunc = time.Now` in `clock.go`, swapped by a `t.Cleanup` helper. | Intake Assumption #10 prefers the package-level-var pattern mirroring `proc.Runner`; keeps signatures stable. | S:90 R:80 A:90 D:85 |
| 2 | Certain | Duration format = `elapsed.Round(time.Second).String()`. | Intake gives `1m12s` verbatim; `Round(Second)` yields exactly that for 72s. | S:88 R:80 A:90 D:85 |
| 3 | Confident | Dry-run preview formatters (`printUpdatePreview`/`printInstallPreview`) live in `ui.go` and take pre-built `[]previewRow`; commands compute argv. | Intake Impact + Assumption #13 keep `ui.go` presentation-only with no subprocess calls; commands own probe results. | S:82 R:70 A:85 D:78 |
| 4 | Confident | Column layout = 2-space row indent, labels padded to the longest label present, 2 spaces between label and command. | Matches the intake's worked examples (`  shll (self)  brew upgrade …`) — two leading spaces, two between columns. | S:80 R:75 A:80 D:75 |
| 5 | Confident | `runUpdate`/`runInstall` gain a `dryRun bool` parameter (vs. reading a package var); `RunE` reads the cobra flag and passes it. | Mirrors the existing explicit-writer test-seam style; tests drive `run*` directly. | S:82 R:78 A:85 D:80 |
| 6 | Confident | Preview rows carry no `[N/M]` counter or blank-line spacing (static aligned table). | Intake preview examples show no counters/blank lines; counters/spacing are streaming-loop concerns. | S:80 R:80 A:82 D:78 |

6 assumptions (2 certain, 4 confident, 0 tentative).
