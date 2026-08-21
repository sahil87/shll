# Intake: Revert two-region terminal UX to linear streamed output

**Change**: 260821-0ia2-revert-two-region-linear-output
**Created**: 2026-08-21

## Origin

Promptless dispatch (`/fab-proceed`-style create-new; questioning mode `promptless-defer`). Synthesized from a live-debugging conversation — the description below is the source of truth:

> Revert the two-region terminal UX in `shll install` / `shll update` back to linear streamed output. Change 260820-yud0-install-update-terminal-ux (#88, commit cc3e864) introduced a two-region tty layout: a pinned one-line status header at row 1 (via DECSTBM scroll margins, `src/cmd/shll/region.go`) with child output scrolling beneath it. A live run of `shll update` produced badly corrupted terminal output, diagnosed to root causes in region.go. Decision (user's, explicit): the two-region approach was acceptable only if absolutely error-free; it is not. Go back to the linear design. Keep everything else #88 delivered (the null-stdin streamed transport, per-tool headers, summary tail, digest, OSC 9;4 progress).

Interaction mode: one-shot dispatch carrying the full diagnosis and an explicit user decision — the alternatives (fix-in-place, bottom-pinned bar) were considered in conversation and rejected by the user.

## Why

1. **The pain point.** A live `shll update` run on a real terminal produced badly corrupted output under the yud0 two-region layout. Diagnosed root causes, all in `src/cmd/shll/region.go`:
   - `paintHeaderLocked` (region.go:192) ends every header repaint by re-parking the cursor at the scroll-region top (`regionCursorPos(regionScrollTop)` — row 2, col 1). `setHeader` fires at every tool boundary (`updateHeader`/`installHeader`), so each tool's output section restarts painting at row 2, overprinting the previous tool's rows **without any erase** — producing spliced "palimpsest" lines (e.g. `Current version: v0.2.63.5).1.22 already installed`) and stale leftover rows (a `▸ [3/7] fab-kit` header still visible below the `[7/7]` done tail).
   - `start()` never clears rows 2..N, so the run also overprints whatever screen content pre-existed.
   - The concurrency comment (region.go:97–100) claims the child tee writes are serialized through the region mutex — **false**: exec's copy goroutines write via `io.MultiWriter` without taking `mu`, so a SIGWINCH repaint can splice escape sequences mid-line.
   - Inherent limitation even if all of the above were fixed: DECSTBM region scrolling (top margin = row 2) **discards** scrolled lines instead of pushing them to terminal scrollback in most terminals, so the run log beyond one screenful is unrecoverable — a regression for an update tool whose failures live in brew output.
2. **Consequence of not fixing.** Every tty `shll update`/`shll install` run risks a corrupted, misleading terminal transcript, and long runs lose their log entirely. For the toolkit's flagship UX commands this is worse than the plain linear output the region replaced.
3. **Why this approach (full revert to linear) over alternatives.** The user's explicit bar was: the two-region approach was acceptable only if absolutely error-free — it is not. Rejected alternatives: (a) fix the cursor discipline in place (DECSC/DECRC save/restore instead of re-parking); (b) rework into an apt-style bottom-pinned status bar. Both still carry terminal-state fragility (tmux, SSH, resize races), and the DECSTBM variants keep the scrollback loss. Linear streamed output has none of these failure modes.

## What Changes

### Remove: the region primitive

- **Delete `src/cmd/shll/region.go`** entirely: the `statusRegion` type (`newStatusRegion`/`start`/`setHeader`/`stop`, `applyLocked`/`paintHeaderLocked`/`restoreLocked`), the DECSTBM escape constants (`regionMarginSetFmt`, `regionMarginReset`, `regionCursorHome`, `regionCursorPosFmt`, `regionEraseLine`, `regionScrollTop`, `regionCursorPos`), the enablement seams (`regionWriterIsTTY`/`defaultRegionWriterIsTTY`, `terminalSize`/`defaultTerminalSize`), the SIGWINCH/SIGINT/SIGTERM handler machinery (`installHandlers`/`removeHandlers`/`watchSignals`), `statusHeaderText`, and `truncateHeader`.
- **Delete `src/cmd/shll/region_test.go`** entirely.
- The `golang.org/x/term` dependency **stays** — `ui.go` (`colorEnabled`/`stdinIsTTY`) and `progress.go` still consume it (verified).

### Remove: statusRegion wiring in update.go / install.go

- `src/cmd/shll/update.go` (`runUpdate`): remove `region := newStatusRegion(stdout)` / `defer region.stop()` / `region.start()` (lines ~317–319), both `region.setHeader(statusHeaderText(...))` calls (the pre-loop brew-refresh header at ~341 and the per-tool one inside `updateHeader` at ~406), and the `region` argument threading. The `actionableNames`/`nextName` lookahead machinery exists solely to feed the pinned header's `· next:` clause — remove it with the header calls (the OSC 9;4 `progress.set` calls and `printToolHeader` stay).
- `src/cmd/shll/install.go` (`runInstall`): same removals — `newStatusRegion`/`defer region.stop()`/`region.start()` (~284–286), the `region.setHeader(statusHeaderText(installRegionVerb, ...))` call inside `installHeader` (~315), and the `actionableNames`/`nextName` lookahead built only for it.
- Remove the region constants that lose their consumer: `updateRegionVerb`, `updateRegionBrewLabel` (update.go:43–46) and `installRegionVerb` (install.go:519). Verified: `updateRegionBrewLabel` ("brew metadata refresh") is used only (a) as region-header text and (b) as the `name` argument to `runChild` for the run-wide `brew update --quiet` — and the `name` parameter itself goes away (next section), so the constant has no surviving consumer.

### Remove: region threading through the streamed-child helper

- `src/cmd/shll/brew.go` — `runStreamedChild(ctx, stdout, stderr, region *statusRegion, name string, argv ...string)` (brew.go:28): drop the `region` parameter and the region-gated failure-tail re-print (`printFailureTail(stderr, name, tail)`). With the tail re-print gone, the `name` parameter has no consumer either — drop it too. The helper reduces to a thin wrapper over `proc.RunStreamedTail` (kept as the single seam documenting the null-stdin transport decision for all write-phase children).
- `brewTrustFormula` (brew.go:113) drops its `region` parameter; `upgradeTool` (update.go:670) drops its `region` parameter; the `runChild` closures in `runUpdate`/`runInstall`/`upgradeTool` drop the region/name plumbing. No other call sites exist (verified by grep — all non-test `*statusRegion` references are in region.go, brew.go, update.go, install.go).
- `src/cmd/shll/ui.go` — remove `printFailureTail` and `failureTailFrameFmt` (ui.go:200–213): the sole call site is the region-gated branch in `runStreamedChild`, and in linear mode the full child output is already in terminal scrollback, so a failure re-print is redundant (pre-yud0 output had none). Verified: no other consumers.

### Remove/adapt: tests

- Delete the "tty-mode region tests (change yud0)" blocks: `TestUpdate_RegionModeSequencesAndTransports`, `TestUpdate_RegionShllSelfHeader`, `TestUpdate_RegionFailurePrintsTail` (+ the region-mode agent-refresh stdin test at update_test.go:~2542), `TestInstall_RegionModeSequencesAndTransport`, `TestInstall_RegionFailurePrintsTail`, the non-tty "zero region sequences" assertion test (install_test.go:~1718), and the shared `forceRegionTTY` helper.
- Adapt signature-dependent call sites: `brew_test.go` passes `newStatusRegion(&stdout)` to `brewTrustFormula` (lines 61, 89, 104); `update_test.go:2171` passes one to `upgradeTool`.
- **Keep** the transport assertions that pin kept behavior: `proc.TransportStreamTail` checks in brew_test.go/update_test.go/install_test.go, and all of `src/internal/proc/proc_test.go` (the transport is kept — see below).

### Keep (everything else #88 delivered — explicitly out of scope to touch)

- `proc.RunStreamedTail` / `TransportStreamTail` in `src/internal/proc/proc.go`: live tee to caller writers via `io.MultiWriter`, **null stdin** (prompt-free enforcement for write-phase children), bounded 4KB interleaved `tailRing` capture. The null-stdin streamed transport is valuable independent of the region. The now-consumerless `tail` return value / ring buffer **stays in proc** (API intact — lower-risk; see Assumptions #3).
- Per-tool in-stream headers `▸ [N/M] tool` (`printToolHeader`), the summary tail (`✓ Done — N of M tools succeeded in Xs`, `printSummaryTail`), the "What changed:" digest (`printUpdateDigest`), and OSC 9;4 progress reporting (`progress.go`) — all untouched.
- The end-of-run agent-skill refresh stays on `proc.RunForeground` (inherited stdin — the documented interactive path).
- **Non-tty output must remain byte-identical** to current non-tty behavior. The region was already a no-op off-tty and the failure tail was region-gated, so this change alters tty output only (region escapes and failure-tail frames disappear; the linear stream of headers + child output + tails is unchanged).

## Affected Memory

Hydrated with region content by 260820-yud0; the region material must be removed/rewritten at hydrate:

- `cli/update`: (modify) remove the "pinned status region (tty two-region write phase)" section and region references; the "null-stdin streamed children" section survives minus the failure-tail half (the `#null-stdin-streamed-children--the-failure-tail` anchor is cross-referenced from internal/proc.md — fix the links)
- `cli/install`: (modify) remove the "pinned status region + determinate OSC 9;4" region half (12 region mentions; the OSC 9;4 and streamed-transport content stays)
- `cli/commands`: (modify) remove the `region.go` file-inventory row (~line 202), the region halves of the `install.go`/`update.go`/`ui.go` rows (`printFailureTail`/`failureTailFrameFmt` in the ui.go row), the tty-region paragraph (~line 216), and the `region.go` mention in the Constitution VII helper-file list (~line 241)
- `internal/proc`: (modify) rewrite the two region-referencing passages (proc.md:153, 198) — the `tailRing`/`Result.Tail` mechanics stay as-is, but the "re-printed after they scrolled out of a DECSTBM region" rationale and the region-section cross-links must go
- `cli/index` + `internal/index`: regenerated by `fab memory-index` after the file edits (generated — not hand-edited)

Verified: `docs/specs/per-tool-output-separation.md` contains **no** region references — the spec predates yud0's region and needs no edit. No other memory file carries region content (`ci/*`, `cli/setup`, `cli/version`, `cli/changelog`, `cli/check-updates` matches were false positives on unrelated words).

## Impact

- **Code**: `src/cmd/shll/region.go` (delete, ~367 lines), `src/cmd/shll/region_test.go` (delete), `src/cmd/shll/update.go`, `src/cmd/shll/install.go`, `src/cmd/shll/brew.go`, `src/cmd/shll/ui.go` (unwiring — net deletion), plus test adaptations in `update_test.go`, `install_test.go`, `brew_test.go`. `src/internal/proc/` untouched.
- **Behavior**: tty runs of `shll install`/`shll update` return to linear streamed output (headers, child output, tails scroll normally with full scrollback). Non-tty behavior byte-identical. No CLI surface change (no flags, no subcommands — Constitution VII untouched); no new subprocess invocations (Constitution I unaffected).
- **Docs**: four memory files rewritten at hydrate (above); two generated indexes regenerated; no spec edit.
- **Dependencies**: none added or removed (`golang.org/x/term` retained by ui.go/progress.go).

## Open Questions

- None. The dispatch description carried the diagnosis, the explicit user decision, the rejected alternatives, and the keep/remove scope; every residual decision graded Confident or better (see Assumptions).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Full revert to linear streamed output — remove the two-region layout entirely, not fix-in-place | Discussed — user's explicit decision ("acceptable only if absolutely error-free; it is not"); DECSC/DECRC fix and bottom-pinned bar explicitly rejected | S:95 R:70 A:95 D:95 |
| 2 | Certain | Keep `proc.RunStreamedTail`/`TransportStreamTail` (null-stdin, live tee) as the write-phase child transport | Explicit keep-scope in the description — prompt-free enforcement is valuable independent of the region | S:95 R:80 A:90 D:90 |
| 3 | Confident | Keep proc's `tail` return value + `tailRing` intact even though its sole consumer (`printFailureTail`) is removed | Description delegates this as an implementation decision and leans "leaving proc's API intact is lower-risk"; flagged here per its instruction | S:70 R:90 A:80 D:65 |
| 4 | Confident | Remove `printFailureTail` + `failureTailFrameFmt` from ui.go | Verified sole call site is the region-gated branch in `runStreamedChild`; linear mode keeps all output in scrollback, so the re-print is redundant (pre-yud0 had none) — description directed a dead-code check | S:75 R:85 A:85 D:65 |
| 5 | Confident | Drop BOTH the `region` and `name` params from `runStreamedChild`; keep the helper as the thin null-stdin-transport seam rather than inlining `proc.RunStreamedTail` at 6+ call sites | `name`'s only consumer was the failure-tail frame; the helper documents the transport decision in one place (code-quality: single source of truth) | S:65 R:90 A:85 D:70 |
| 6 | Confident | Remove `updateRegionVerb`, `updateRegionBrewLabel`, `installRegionVerb` constants and the `actionableNames`/`nextName` lookahead machinery | Verified: `updateRegionBrewLabel`'s only non-header use is as the `name` arg (which is removed per #5); the lookahead exists solely for the pinned header's `· next:` clause | S:70 R:80 A:85 D:75 |
| 7 | Certain | Non-tty output remains byte-identical; tty output changes only by losing region escapes and failure-tail frames | Explicit requirement in the description; verified the region and the tail were both tty/region-gated | S:90 R:75 A:90 D:90 |
| 8 | Certain | Forward-edit removal, not `git revert` of cc3e864 | Most of #88 is kept (transport, headers, tail, digest, progress) — a revert would destroy kept scope and conflict; removal is surgical | S:70 R:80 A:90 D:85 |
| 9 | Confident | Delete the yud0 region test blocks + `forceRegionTTY`; adapt `newStatusRegion(...)`-passing call sites in brew_test.go/update_test.go; KEEP the `TransportStreamTail` transport assertions and all of proc_test.go | Tests must conform to the post-revert spec (Test Integrity); transport assertions pin kept behavior | S:70 R:85 A:85 D:70 |
| 10 | Certain | `golang.org/x/term` dependency stays in go.mod | Verified: ui.go and progress.go still import it | S:85 R:95 A:100 D:95 |
| 11 | Certain | `printToolHeader` headers, summary tail, "What changed:" digest, OSC 9;4 progress, and the RunForeground agent-skill refresh are untouched | Explicit keep-scope list in the description | S:90 R:85 A:90 D:90 |

11 assumptions (6 certain, 5 confident, 0 tentative, 0 unresolved).
