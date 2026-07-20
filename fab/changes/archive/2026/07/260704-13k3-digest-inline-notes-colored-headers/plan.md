# Plan: Colored Headers + Inline Changelog Bodies in Update Digest

**Change**: 260704-13k3-digest-inline-notes-colored-headers
**Intake**: `intake.md`

## Requirements

### R1 — Inline full release notes in the `shll update` digest (in-process)

`printUpdateDigest` (`src/cmd/shll/update.go`) MUST render each bumped tool's full
release notes inline, in-process, from the release data the digest already fetches
(`changelog.FetchAll`) — never by shelling out to `shll changelog`.

- **R1.1** The per-tool transition line MUST be kept: `{tool} {old} → {new} ({N} release{s})`
  (the digest's transition line carries the tool name, unlike `shll changelog` where the
  tool name lives in the `printToolHeader` line above the body).
- **R1.2** Each release in range MUST render with its **full body** — `{tag}  {title}` then
  the body markdown (trailing newlines trimmed; an empty body is skipped) — newest-first,
  by REUSING the release-block rendering shared with `renderChangelogResult`
  (`src/cmd/shll/changelog.go`). The two surfaces MUST NOT drift.
- **R1.3** The digest MUST apply the existing per-tool cap (`changelogCapPerTool = 10`)
  newest releases, then the cap notice + compare URL (`… {N-10} more — full changelog:
  {compareURL}`), so a long-range bump never dumps unbounded text.
- **R1.4** The now-redundant `Full notes: shll changelog …` tail line MUST be dropped;
  the `digestFullNotes` constant and the `digestSpecs` helper MUST be removed (no other
  consumers).
- **R1.5** The data source MUST remain the digest's existing `changelog.FetchAll` call —
  no new fetch, no subprocess, no new dependency.
- **R1.6** Unchanged digest behavior MUST be preserved: empty `bumps` → prints nothing
  (byte-identical no-op run); unavailable-fetch / zero-releases-in-range → the degrade
  line `{tool} {old} → {new} — see {compareURL}` (no bodies exist); `--dry-run` /
  subset-run semantics untouched; the digest never influences the exit code.
- **R1.7** The digest layout with inline bodies MUST drop the cross-tool two-pass column
  alignment (tool-name + transition padding) and the per-block tag padding — meaningless
  with multi-line body blocks. Release blocks render in the shared `shll changelog` format;
  the per-tool transition line keeps the 2-space `digestToolIndent`.

GIVEN a `shll update` run that bumps `hop 0.1.16 → 0.1.18` with 2 releases in range
WHEN the digest renders
THEN it prints `What changed:`, then the indented transition line `hop 0.1.16 -> 0.1.18
(2 releases)` (ASCII-degraded on a non-TTY buffer), then each release `{tag}  {title}`
followed by its full body, newest-first, and NO `Full notes:` line.

GIVEN a bump whose release fetch is unavailable (or holds zero matching releases)
WHEN the digest renders that tool
THEN it prints the degrade line `hop 0.1.16 -> 0.1.18 -- see {compareURL}` and the run
exit code is unaffected.

GIVEN a `shll update` run where no tool bumped (all up-to-date / all failed)
WHEN the digest is invoked
THEN it prints nothing — stdout is byte-identical to before this change.

### R2 — Color the tool name in per-tool headers

`printToolHeader` (`src/cmd/shll/ui.go`) MUST, on the color branch, render the whole
`[N/M] name` run in bold-cyan (matching the `▸`), so tool boundaries pop visually.

- **R2.1** The color branch MUST render `▸`, `[N/M]`, and `name` all in a single bold-cyan
  ANSI span (`ansiBoldCyan` … `ansiReset`).
- **R2.2** The plain branch (`==> [N/M] name`, non-TTY / `NO_COLOR`) MUST stay
  byte-identical — no ANSI, ever.
- **R2.3** Because the three surfaces share `printToolHeader`, the change MUST cover
  `shll update`, `shll install`, and `shll changelog` with no per-surface edit.

GIVEN a color-enabled TTY
WHEN `printToolHeader(w, "hop", 5, 6, true)` runs
THEN the output styles `[5/6] hop` in bold-cyan alongside the bold-cyan `▸`, and the
plain form for the same inputs is exactly `==> [5/6] hop\n`.

### R3 — Bold visual anchors on changelog-surface subheadings

With bodies inline, the transition and tag/title lines are the navigational anchors and
MUST get bold (`ansiBold`) styling when color is enabled.

- **R3.1** The digest's per-tool transition line (`rk 2.4.3 → 2.5.0 (1 release)`) MUST be
  bold when color is enabled.
- **R3.2** The release tag/title lines (`v2.5.0  Title`) MUST be bold when color is
  enabled, in BOTH the digest and `shll changelog` (one shared code path).
- **R3.3** `shll changelog`'s own transition line (`{old} → {new} ({N} releases)`,
  `renderChangelogResult`) MUST get the same bold anchor for cross-surface symmetry.
- **R3.4** The anchor styling MUST be plain bold, NOT bold-cyan — bold-cyan stays reserved
  for the per-tool headers so the hierarchy header > anchor > body is preserved.
- **R3.5** All anchor color MUST be TTY-gated via the existing threaded `color bool`
  decision; on a non-TTY / `NO_COLOR` stream the lines stay ANSI-free (the `→`/`—`/`…`
  glyph degrade is unchanged).

GIVEN a color-enabled stream
WHEN the digest or `shll changelog` renders a transition line and a release tag/title line
THEN each is wrapped in `ansiBold` … `ansiReset`; GIVEN a non-TTY buffer, neither line
carries any ANSI escape.

### R4 — Tests conform to the new format (test-alongside)

The exact-byte test assertions MUST be updated to the new format per Test Integrity
(constitution) and the test-alongside strategy (code-quality.md): tests conform to this
plan's format, never the implementation to stale fixtures.

- **R4.1** `update_test.go` digest goldens (`TestUpdate_Digest*`) MUST be updated: inline
  bodies present, the `Full notes:` assertions removed, and the dropped column-alignment
  assertion (`TestUpdate_DigestColumnAlignment`) removed or rewritten to the new
  unaligned/inline shape.
- **R4.2** `ui_test.go` (`TestPrintToolHeader_ColorForm`) MUST assert the bold-cyan
  `[N/M] name` run; `TestPrintToolHeader_PlainForm` MUST stay unchanged.
- **R4.3** `changelog_test.go` release-rendering goldens MUST still pass under the shared
  renderer + bold anchors (non-TTY buffers → ANSI-free, so existing substring assertions
  hold); add a color-form assertion for the bold anchor if a `bold` helper is introduced.

### Non-Goals

- `shll shell-init`'s `# ── tool ──` separator (`toolComment`, `ui.go`) stays plain
  uncolored ASCII — its stdout is `eval`'d (Constitution V). Out of scope.
- No other subcommand has per-tool subheadings (`version`/`list` are tabular, `doctor`
  has its own format) — out of scope.
- No memory/spec edits in apply — the supersede of the r01z "titles only" decision and the
  spec header-style wording update happen at hydrate.

### Design Decisions

- **Extract `renderReleases(w, res, color)`** from `renderChangelogResult`: the shared
  helper renders only the release blocks (tag/title bold anchor + full body, newest-first)
  + the cap notice, taking `res.Releases` / `res.Repo` / `res.Old` / `res.New` / `n`.
  `renderChangelogResult` keeps ownership of its unavailable fallback, its own transition
  line (now bold), and the "no releases in range" line, then delegates the release blocks
  to `renderReleases`. `printUpdateDigest` prints its tool-name transition line (bold), then
  calls `renderReleases` for the same tool. This is the single-source-of-truth extraction
  the intake mandates. *Rejected*: duplicating the release loop in the digest (drift risk).
- **Add a `bold(color bool, s string) string` helper** in `ui.go`: wraps `s` in
  `ansiBold` … `ansiReset` when `color`, else returns `s` unchanged — the single place the
  anchor styling is applied, mirroring the existing `arrow`/`dash`/`more` glyph-helper
  idiom. *Rejected*: open-coding `ansiBold+s+ansiReset` at each call site (magic-string
  anti-pattern, code-quality.md).
- **Digest transition line format** keeps the 2-space `digestToolIndent` and reads
  `{indent}{tool} {old} → {new} ({N} release{s})` — the tool name is inline (no separate
  header), then `renderReleases` renders unindented release blocks in the shared format.
  Per-block tag padding and cross-tool column alignment are dropped (R1.7).

## Tasks

### Phase 1: Shared helpers (ui.go)

- [x] T001 Add a `bold(color bool, s string) string` helper to `src/cmd/shll/ui.go` that returns `ansiBold + s + ansiReset` when `color` is true and `s` unchanged otherwise; document it alongside the `arrow`/`dash`/`more` glyph helpers. <!-- R3 -->
- [x] T002 Change `printToolHeader`'s color branch in `src/cmd/shll/ui.go` to render the whole `[N/M] name` run in a single bold-cyan span (`▸` + `[N/M]` + `name` all inside `ansiBoldCyan` … `ansiReset`); leave the plain branch (`==> [N/M] name`) byte-identical. <!-- R2 -->

### Phase 2: Shared release renderer (changelog.go)

- [x] T003 Extract a `renderReleases(w io.Writer, res changelog.Result, color bool)` helper in `src/cmd/shll/changelog.go` from `renderChangelogResult`'s release-block loop + cap logic: it renders each release `{tag}  {title}` (tag/title line wrapped via `bold(color, …)`) followed by the trimmed non-empty body, newest-first, capped at `changelogCapPerTool` with the `… {N-10} more — full changelog: {compareURL}` notice on overflow. <!-- R1 R3 -->
- [x] T004 Rewrite `renderChangelogResult` in `src/cmd/shll/changelog.go` to keep its unavailable fallback, its transition line (now wrapped via `bold(color, …)` — R3.3), and the "no releases in range" line, then delegate the release blocks to `renderReleases`. <!-- R1 R3 -->

### Phase 3: Digest rewrite (update.go)

- [x] T005 Rewrite `printUpdateDigest` in `src/cmd/shll/update.go` to: print `What changed:`; for each result print the bold-anchored per-tool transition line `{digestToolIndent}{tool} {old} → {new} ({N} release{s})` (bold via `bold(color, …)`); on unavailable / zero-releases print the degrade line `{digestToolIndent}{tool} {old} → {new} — see {compareURL}`; otherwise call `renderReleases` for the release blocks. Drop the two-pass column alignment and per-block tag padding (R1.7). <!-- R1 R3 -->
- [x] T006 Remove the now-unused `digestFullNotes` constant and the `digestSpecs` helper from `src/cmd/shll/update.go` (and drop the trailing `Full notes:` write). Verify no other references remain. <!-- R1 -->

### Phase 4: Tests

- [x] T007 Update `ui_test.go`: `TestPrintToolHeader_ColorForm` asserts the bold-cyan `[N/M] name` run (arrow + counter + name in one bold-cyan span); `TestPrintToolHeader_PlainForm` unchanged. Add a `Test`-level assertion for the new `bold` helper (color wraps in `ansiBold`/`ansiReset`; plain returns the string unchanged). <!-- R2 R3 R4 -->
- [x] T008 Update `update_test.go` digest goldens: `TestUpdate_DigestPrintsForBumpedTools` asserts inline bodies present + no `Full notes:`; drop the `Full notes:` assertions in `TestUpdate_DigestSubsetNamesOnlyBumped` / `TestUpdate_DigestMixedAvailableAndUnavailable` (re-anchor the subset/roster-order checks on the transition lines); rewrite/remove `TestUpdate_DigestColumnAlignment` for the new unaligned inline shape; keep `TestUpdate_NoDigestWhenNothingBumped` byte-identical. <!-- R1 R4 -->
- [x] T009 Verify `changelog_test.go` release-rendering goldens (`TestChangelog_ExplicitRangeHappyPath`, `TestChangelog_CapOverflow`, `TestChangelog_EmptyRange`, no-range paths) still pass under the shared renderer (non-TTY buffers → ANSI-free); adjust any assertion that changed shape. <!-- R1 R3 R4 -->

### Phase 5: Verify

- [x] T010 Run `go build ./...` and `go test ./cmd/shll/...` from `src/`; fix any failures so the whole package is green. <!-- R1 R2 R3 R4 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `printUpdateDigest` renders each bumped tool's full release bodies inline, in-process, from `changelog.FetchAll` — no subprocess, no new fetch.
- [x] A-002 R1: The `Full notes: shll changelog …` tail line is gone and the `digestFullNotes` constant + `digestSpecs` helper are removed with no dangling references.
- [x] A-003 R1: The digest applies the `changelogCapPerTool = 10` cap + cap-notice/compare-URL via the shared release renderer.
- [x] A-004 R2: `printToolHeader`'s color branch renders the whole `[N/M] name` run in bold-cyan; the plain branch is byte-identical.
- [x] A-005 R3: The digest transition line, the release tag/title lines, and `shll changelog`'s transition line are bold when color is enabled and ANSI-free otherwise; bold-cyan is reserved for headers.
- [x] A-006 R1: The release-block rendering is single-sourced (`renderReleases`) between `renderChangelogResult` and `printUpdateDigest` — the two surfaces cannot drift.

### Behavioral Correctness

- [x] A-007 R1: An empty `bumps` slice prints nothing (byte-identical no-op run); `TestUpdate_NoDigestWhenNothingBumped` golden preserved.
- [x] A-008 R1: An unavailable / zero-release bump degrades to `{tool} {old} → {new} — see {compareURL}` and the exit code is unaffected.
- [x] A-009 R1: `--dry-run` and subset runs behave as before (no digest under dry-run; subset digest covers only bumped members).

### Removal Verification

- [x] A-010 R1: `grep` confirms `digestFullNotes` and `digestSpecs` are fully removed from `src/cmd/shll`.

### Edge Cases & Error Handling

- [x] A-011 R3: On a non-TTY / `NO_COLOR` stream, all transition/tag/title lines carry zero ANSI escapes; the `→`/`—`/`…` glyphs still ASCII-degrade as before.

### Code Quality

- [x] A-012 Pattern consistency: the new `bold` helper follows the existing `arrow`/`dash`/`more` glyph-helper idiom and named-SGR-constant convention (no open-coded ANSI); `renderReleases` mirrors the surrounding `changelog.go` style.
- [x] A-013 No unnecessary duplication: the release-block loop exists in exactly one place after the extraction; the digest and `shll changelog` share it.
- [x] A-014 Constitution I: no new subprocess is introduced — `ui.go` stays presentation-only and the digest renders in-process (verified against the intake's explicit no-new-subprocess constraint).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Extract release-block rendering into `renderReleases(w, res, color)` that both `renderChangelogResult` and `printUpdateDigest` call; `renderChangelogResult` keeps its transition + unavailable + no-releases lines | Intake Assumption #2 (Certain) mandates the shared helper; the extraction shape is the obvious mechanical realization — the release loop + cap logic is exactly what both surfaces need identical | S:90 R:85 A:90 D:85 |
| 2 | Certain | Add a `bold(color, s)` helper in ui.go for the anchor styling, mirroring the `arrow`/`dash`/`more` glyph helpers | code-quality.md forbids open-coded ANSI magic strings; a named helper is the established idiom in this exact file | S:80 R:90 A:90 D:80 |
| 3 | Certain | Digest transition line keeps `digestToolIndent` (2 spaces) and reads `{tool} {old} → {new} ({N} release{s})`; release blocks render unindented via `renderReleases`; drop cross-tool column alignment + per-block tag padding | Intake Assumption #8 (Confident) + What-Changes layout notes; alignment is meaningless across multi-line bodies | S:75 R:85 A:75 D:70 |
| 4 | Confident | Anchor styling is plain `ansiBold` (not bold-cyan) for the transition + tag/title lines; bold-cyan reserved for `printToolHeader` | Intake Assumption #6 (Confident) — preserves header > anchor > body hierarchy | S:75 R:90 A:75 D:60 |
| 5 | Certain | Header color scope: the whole `▸ [N/M] name` run in one bold-cyan span | Intake Assumption #5 (Certain) — user accepted the whole-run form; single ANSI span, maximal pop | S:85 R:95 A:80 D:70 |
| 6 | Certain | Drop `TestUpdate_DigestColumnAlignment` (or rewrite to the new inline shape) since the two-pass alignment it pins is removed; re-anchor subset/roster-order digest checks on the transition lines rather than the `Full notes:` line | Intake Assumption #12 (Certain) — tests conform to the new format (Test Integrity); the alignment contract no longer exists | S:80 R:85 A:90 D:80 |
| 7 | Confident | `renderReleases` renders the release blocks starting with a leading blank line before each `{tag}  {title}` (preserving `renderChangelogResult`'s existing `\n%s  %s\n` spacing) so both surfaces stay byte-consistent | The current `renderChangelogResult` already emits a leading `\n` before each release block; keeping that in the extracted helper is the least-surprise, drift-free choice | S:60 R:85 A:75 D:65 |

7 assumptions (5 certain, 2 confident, 0 tentative).

## Deletion Candidates

None — this change adds new functionality without making existing code redundant. The code it did obsolete (`digestFullNotes`, `digestSpecs`, `digestReleaseIndent`, and `printUpdateDigest`'s two-pass column alignment + per-block tag padding, all in `src/cmd/shll/update.go`) was removed within the apply diff itself; `grep` over `src/` confirms zero remaining references, and all surviving shared helpers (`arrow`/`dash`/`more`, `plural`, `specToolSep`/`specRangeSep`, `changelogCapPerTool`) retain live call sites.
