# Plan: Revert two-region terminal UX to linear streamed output

**Change**: 260821-0ia2-revert-two-region-linear-output
**Intake**: `intake.md`

## Requirements

### CLI: Remove the region primitive

#### R1: The statusRegion primitive SHALL be deleted entirely
`src/cmd/shll/region.go` and `src/cmd/shll/region_test.go` MUST be deleted: the `statusRegion` type and lifecycle (`newStatusRegion`/`start`/`setHeader`/`stop`, `applyLocked`/`paintHeaderLocked`/`restoreLocked`), the DECSTBM escape constants (`regionMarginSetFmt`, `regionMarginReset`, `regionCursorHome`, `regionCursorPosFmt`, `regionEraseLine`, `regionScrollTop`, `regionCursorPos`), the enablement seams (`regionWriterIsTTY`/`defaultRegionWriterIsTTY`, `terminalSize`/`defaultTerminalSize`), the SIGWINCH/SIGINT/SIGTERM machinery (`installHandlers`/`removeHandlers`/`watchSignals`), `statusHeaderText`, and `truncateHeader`. No reference to any of these identifiers may survive anywhere in `src/`.

- **GIVEN** the repo after this change
- **WHEN** `grep -rn "statusRegion\|regionMargin\|regionCursor\|regionEraseLine\|regionScrollTop\|regionWriterIsTTY\|statusHeaderText\|truncateHeader\|forceRegionTTY" src/` runs
- **THEN** it returns no matches
- **AND** `src/cmd/shll/region.go` and `src/cmd/shll/region_test.go` do not exist

#### R2: `runUpdate` SHALL run its write phase as a plain linear stream
`src/cmd/shll/update.go` MUST lose all region wiring: the `region := newStatusRegion(stdout)` / `defer region.stop()` / `region.start()` block (~317–319), the pre-loop brew-refresh `region.setHeader(statusHeaderText(...))` call (~341), the per-tool `region.setHeader(...)` inside `updateHeader` (~406), the `actionableNames`/`nextName` lookahead machinery (built solely for the pinned header's `· next:` clause), and the constants `updateRegionVerb` and `updateRegionBrewLabel` (43–46). The kept behavior — `updateStatusLine`, `printToolHeader` `[N/M]` headers with section spacing, `progress.set`/`indeterminate`/`errorState` OSC 9;4 calls, `printSummaryTail`, `printUpdateDigest`, the agent-skill refresh, and every upgrade/self-heal/fallback path — MUST be untouched.

- **GIVEN** a tty run of `shll update` with several installed tools
- **WHEN** the write phase executes
- **THEN** output streams linearly (status line, per-tool `▸ [N/M]` headers, child output, summary tail, digest) with no DECSTBM/cursor escape sequences emitted by shll
- **AND** a non-tty run's output is byte-identical to the pre-change non-tty output

#### R3: `runInstall` SHALL run its write phase as a plain linear stream
`src/cmd/shll/install.go` MUST lose the same wiring: `region := newStatusRegion(stdout)` / `defer region.stop()` / `region.start()` (~284–286), the `region.setHeader(statusHeaderText(installRegionVerb, ...))` call inside `installHeader` (~315), the `actionableNames`/`nextName` lookahead, and the `installRegionVerb` constant (~519). `printToolHeader`, `progress.set`, the trust step, delegated installs, and the post-outcome setup hand-off MUST be untouched.

- **GIVEN** a tty run of `shll install` with missing tools
- **WHEN** the write phase executes
- **THEN** output streams linearly with no region escape sequences
- **AND** non-tty output is byte-identical to the pre-change non-tty output

### CLI: Simplify the streamed-child helper

#### R4: `runStreamedChild` SHALL reduce to a thin null-stdin transport seam
`src/cmd/shll/brew.go` `runStreamedChild` MUST drop both the `region *statusRegion` and `name string` parameters and the region-gated `printFailureTail` branch, reducing to a thin wrapper over `proc.RunStreamedTail(ctx, stdout, stderr, argv[0], argv[1:]...)` — kept (not inlined at the 6+ call sites) as the single seam documenting the null-stdin write-phase transport decision. `brewTrustFormula` (brew.go) and `upgradeTool` (update.go) MUST drop their `region` parameters; the `runChild` closures in `runUpdate`/`runInstall`/`upgradeTool` drop the region/name plumbing. `printFailureTail` and `failureTailFrameFmt` MUST be removed from `src/cmd/shll/ui.go` (sole consumer was the region-gated branch; linear output keeps everything in scrollback). `src/internal/proc/` MUST NOT be modified — `RunStreamedTail`'s `tail` return and the `tailRing` stay (API intact, intake Assumption #3). No direct `os/exec` use may be introduced (Constitution I).

- **GIVEN** a write-phase child fails on a tty
- **WHEN** `runStreamedChild` returns its non-zero code
- **THEN** no `--- last output: ... ---` frame is printed (the cause is already visible in the linear scrollback)
- **AND** exit-code semantics pass through `proc.RunStreamedTail` unchanged (non-zero exit → `(code, nil)`; pre-start failure → `(-1, err)`)

### Tests: Conform to the post-revert spec

#### R5: Region tests SHALL be deleted and signature-dependent call sites adapted
Delete the yud0 region test blocks and helper: `TestUpdate_RegionModeSequencesAndTransports`, `TestUpdate_RegionShllSelfHeader`, `TestUpdate_RegionFailurePrintsTail`, the region-mode agent-refresh stdin test (update_test.go ~2544), `TestInstall_RegionModeSequencesAndTransport`, `TestInstall_RegionFailurePrintsTail`, the non-tty zero-region-sequences assertion test (install_test.go ~1614/1660 blocks as applicable), and the shared `forceRegionTTY` helper. Adapt call sites to the new signatures: `brew_test.go:61,89,104` (`brewTrustFormula` without a region) and `update_test.go:2171` (`upgradeTool` without a region). The `proc.TransportStreamTail` transport assertions in brew_test.go/update_test.go/install_test.go and all of `src/internal/proc/proc_test.go` MUST be kept and passing.

- **GIVEN** the adapted test suite
- **WHEN** `go test ./...` runs from `src/`
- **THEN** all tests pass with zero references to deleted identifiers
- **AND** the kept `TransportStreamTail` assertions still pin the null-stdin streamed transport

### Verification

#### R6: The build and full test suite MUST be green with no dangling references
- **GIVEN** all removals are complete
- **WHEN** `go build ./...`, `go vet ./...`, and `go test ./...` run from `src/`
- **THEN** all succeed
- **AND** the existing non-tty golden tests (`TestUpdate_HeadersAndTail`, `TestUpdate_NoToolsInstalled`, digest goldens, install goldens) pass unmodified — the byte-identical non-tty guarantee

### Non-Goals

- No fix-in-place of the region (DECSC/DECRC) and no bottom-pinned bar — explicitly rejected in the intake.
- No changes to `src/internal/proc/` — the streamed transport, tail ring, and public API stay verbatim.
- No changes to headers, summary tail, digest, OSC 9;4 progress, or the RunForeground agent-skill refresh.
- No spec edit — `docs/specs/per-tool-output-separation.md` has no region references (verified at intake).

### Design Decisions

#### Forward-edit removal, not `git revert` of cc3e864
**Decision**: Surgically remove the region code by forward edits rather than reverting commit cc3e864.
**Why**: Most of #88 is kept (streamed transport, headers, tails, digest, progress); a revert would destroy kept scope and conflict heavily.
**Rejected**: `git revert cc3e864` — would also revert `proc.RunStreamedTail`, the null-stdin hardening, and the test infrastructure the kept scope depends on.
*Introduced by*: 260821-0ia2-revert-two-region-linear-output

#### Keep `runStreamedChild` as the transport seam; keep proc's tail API intact
**Decision**: `runStreamedChild(ctx, stdout, stderr, argv...)` survives as a thin wrapper; `proc.RunStreamedTail` keeps returning the (now-unconsumed) tail.
**Why**: The helper is the single place documenting the null-stdin write-phase transport (code-quality: single source of truth); leaving proc's API intact is lower-risk than an API amputation touching proc_test.go.
**Rejected**: Inlining `proc.RunStreamedTail` at 6+ call sites (scatters the transport decision); removing the tail return from proc (larger diff in the Constitution-I-critical wrapper for zero behavior gain).
*Introduced by*: 260821-0ia2-revert-two-region-linear-output

## Tasks

### Phase 2: Core Implementation

- [x] T001 Delete `src/cmd/shll/region.go` and `src/cmd/shll/region_test.go` entirely <!-- R1 -->
- [x] T002 Unwire `src/cmd/shll/update.go`: remove the region construct/start/defer-stop block, both `region.setHeader(statusHeaderText(...))` calls, the `actionableNames`/`nextName` lookahead, the `updateRegionVerb`/`updateRegionBrewLabel` constants, and the `name` argument from the `runChild` closure; leave headers/progress/tail/digest/refresh untouched <!-- R2 -->
- [x] T003 Unwire `src/cmd/shll/install.go`: remove the region construct/start/defer-stop block, the `region.setHeader(...)` call in `installHeader`, the `actionableNames`/`nextName` lookahead, the `installRegionVerb` constant, and the `name` argument from its `runChild` closure <!-- R3 -->
- [x] T004 Simplify `src/cmd/shll/brew.go`: `runStreamedChild(ctx, stdout, stderr, argv ...string)` (drop `region`+`name`, drop the `printFailureTail` branch), update `brewTrustFormula` and `update.go`'s `upgradeTool` signatures; remove `printFailureTail` + `failureTailFrameFmt` from `src/cmd/shll/ui.go` <!-- R4 -->

### Phase 3: Integration & Edge Cases

- [x] T005 Adapt tests: delete the yud0 region test blocks + `forceRegionTTY` in `update_test.go`/`install_test.go`; update `brew_test.go:61,89,104` and `update_test.go:2171` to the new signatures; keep all `TransportStreamTail` assertions and `proc_test.go` untouched <!-- R5 -->
- [x] T006 Verify: `go build ./... && go vet ./... && go test ./...` from `src/`, and grep-confirm zero surviving references to the deleted identifiers across `src/` <!-- R6 -->

## Execution Order

- T001 → T002/T003/T004 (deleting region.go first lets the compiler surface every dangling reference); T002–T004 are one coordinated edit wave (T004's signature change touches update.go/install.go closures)
- T005 after T002–T004; T006 last

## Acceptance

### Functional Completeness

- [x] A-001 R1: `region.go`/`region_test.go` deleted; zero matches for the region identifier set anywhere in `src/`
- [x] A-002 R2: `runUpdate` carries no region wiring, no lookahead machinery, no `updateRegionVerb`/`updateRegionBrewLabel`; headers, OSC progress, summary tail, digest, and refresh calls unchanged
- [x] A-003 R3: `runInstall` carries no region wiring, no lookahead, no `installRegionVerb`; trust step and delegated installs unchanged
- [x] A-004 R4: `runStreamedChild` is a thin `(ctx, stdout, stderr, argv...)` wrapper over `proc.RunStreamedTail`; `brewTrustFormula`/`upgradeTool` have no region parameter; `printFailureTail`/`failureTailFrameFmt` removed from ui.go

### Behavioral Correctness

- [x] A-005 R2/R3: tty runs emit no terminal escape sequences from shll beyond the existing SGR color spans and OSC 9;4 progress (no DECSTBM, no cursor addressing)
- [x] A-006 R4: a failed child prints no `--- last output ---` frame; exit-code pass-through semantics unchanged

### Removal Verification

- [x] A-007 R1/R4: no dead code remains — no orphaned constants, helpers, or imports left by the unwiring (e.g. unused `os/signal`, `syscall`, `golang.org/x/term` imports in cmd files that lost their consumers; `x/term` itself stays in go.mod via ui.go/progress.go)

### Scenario Coverage

- [x] A-008 R5: region test blocks and `forceRegionTTY` deleted; adapted call sites compile; kept `TransportStreamTail` assertions pass; `proc_test.go` untouched and green
- [x] A-009 R6: `go build ./...`, `go vet ./...`, `go test ./...` all pass from `src/`; pre-existing non-tty golden tests pass unmodified

### Code Quality

- [x] A-010 Pattern consistency: surviving code reads like the pre-yud0 layout (headers + streamed children); comment blocks referencing the region are removed or rewritten, not left stale
- [x] A-011 No unnecessary duplication: `runStreamedChild` remains the single write-phase transport seam; no call site inlines `proc.RunStreamedTail`

### Security

- [x] A-012 R4: no `os/exec` import appears outside `src/internal/proc`; all subprocess calls still route through `internal/proc` (Constitution I)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- `src/internal/proc/proc.go` — `RunStreamedTail`'s `tail` return value and the `tailRing` capture — no consumer remains after `printFailureTail`'s removal; retention is a deliberate plan decision (Design Decision "Keep `runStreamedChild` as the transport seam; keep proc's tail API intact", intake Assumption #3), so this is informational for the human reviewer, not actionable in this change

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Delete region.go first, then fix compile errors outward (T001 before T002–T004) | The compiler enumerates every dangling reference — safer than hand-tracking call sites | S:85 R:95 A:95 D:90 |
| 2 | Confident | The `runChild` closures keep their closure shape (losing only the region/name args) rather than being flattened away | Smallest diff; the closures still centralize the per-run writer threading (intake Assumption #5's helper-keep decision applied at the call sites) | S:70 R:90 A:85 D:75 |
| 3 | Confident | Stale-comment sweep is in scope for the touched files only (update.go/install.go/brew.go/ui.go mention the region in comments); memory rewrites stay in hydrate | Code comments referencing a deleted mechanism are dead weight the reviewer would flag; docs/memory is the hydrate stage's contract | S:75 R:85 A:85 D:70 |

3 assumptions (1 certain, 2 confident).
