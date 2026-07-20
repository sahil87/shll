# Intake: Colored Headers + Inline Changelog Bodies in Update Digest

**Change**: 260704-13k3-digest-inline-notes-colored-headers
**Created**: 2026-07-04

## Origin

> User reviewed a screenshot of `shll update` output (2026-07-03) and flagged the UX of the
> "What changed:" digest tail and the per-tool headers. Three numbered requests, all agreed in
> discussion: (1) inline the full release notes in the `shll update` digest — render in-process,
> do NOT shell out to `shll changelog`; (2) color the tool name in the per-tool headers
> (`printToolHeader`); (3) give visual-anchor styling to the changelog-surface subheadings —
> the digest's per-tool transition lines and the release tag/title lines in both the digest and
> `shll changelog` output.

Conversational origin (screenshot review + discussion), dispatched promptless via the
create-intake procedure. Key decisions from the discussion are encoded verbatim in
**What Changes** and graded in **Assumptions**. Change type: **feat** (UX improvement,
presentation-layer only).

## Why

1. **The pain point.** After `shll update` bumps tools, the digest shows only title lines per
   release plus a copy-pasteable `Full notes: shll changelog tool@old..new` command
   (`printUpdateDigest`, `src/cmd/shll/update.go:560`). "Here's a command you could run" is poor
   UX — the release bodies are *already in memory* (the digest's `changelog.FetchAll` returns
   full `Release` structs including `Body`, `src/internal/changelog/changelog.go:101-108`) and
   the digest throws them away. The update should just show the notes.
2. **Visual scanning.** With full bodies inline, a multi-tool digest becomes long-form text.
   The per-tool headers and the transition/tag-title lines become the navigational anchors —
   today they carry little or no color (`printToolHeader` renders the `▸` bold-cyan but the
   name only bold-default; the digest transition and tag/title lines are unstyled), so tool
   boundaries don't pop.
3. **If we don't fix it**: users keep re-running a second command to read notes the process
   already fetched (a wasted GitHub re-fetch + brew re-probe per run), and long update output
   stays hard to scan.
4. **Why this approach**: render from the in-memory `FetchAll` results and share
   `renderChangelogResult`'s release rendering so both surfaces show releases in one format.
   **Rejected alternative** (explicitly, in discussion): shelling out to `shll changelog` as a
   subprocess — it would re-fetch the same data from GitHub and re-probe brew for information
   the digest already holds.

## What Changes

### 1. Inline full release notes in the `shll update` digest (in-process)

`printUpdateDigest` (`src/cmd/shll/update.go:560`) currently prints, per bumped tool: an
aligned transition line, TITLE-ONLY release lines (`{tag}  {title}`, tag-padded per block),
then a trailing `Full notes: shll changelog tool@old..new ...` command line.

New behavior — for each bumped tool:

- Keep the per-tool transition line `{tool} {old} → {new} ({N} release{s})` (the digest's line
  carries the tool name — unlike `shll changelog`, whose tool name lives in the
  `printToolHeader` line above the body).
- Render each release **with its full body** — `{tag}  {title}` followed by the body markdown
  (trailing newlines trimmed, empty bodies skipped), newest-first — by **reusing
  `renderChangelogResult`'s release rendering** (`src/cmd/shll/changelog.go:391`). Concretely:
  extract the release-block loop + cap logic into a shared helper (e.g.
  `renderReleases(w, res, color)`) that both `renderChangelogResult` and `printUpdateDigest`
  call, so the two surfaces cannot drift.
- Apply the existing per-tool cap pattern to the digest: `changelogCapPerTool = 10`
  (`src/cmd/shll/changelog.go:26`) newest releases, then the cap notice + compare URL
  (`… {N-10} more — full changelog: {compareURL}`) — a long-range bump never dumps unbounded
  text.
- **Drop the now-redundant `Full notes: shll changelog ...` tail line** — remove the
  `digestFullNotes` constant and the `digestSpecs` helper (`update.go:531,627`), which have no
  other consumers.
- Data source is the digest's **existing** `changelog.FetchAll` call (`update.go:569`) — no new
  fetch, no subprocess, no new dependency.

Unchanged digest behavior:

- Empty `bumps` → digest prints nothing (byte-identical no-op run).
- Unavailable fetch / zero releases in range → the degrade line
  `{tool} {old} → {new} — see {compareURL}` (no bodies exist to inline; Constitution V).
- `--dry-run` and subset-run semantics untouched.
- The digest never influences the exit code (`anyFailed` untouched) — presentation-only.

Illustrative new shape (plain ASCII branch shown; blank-line placement finalized by the
apply-stage goldens):

```
What changed:
  rk 2.4.3 -> 2.5.0 (1 release)

v2.5.0  fix: session parsing hardening
## What's Changed
* fix: parse sessions with missing usage blocks
**Full Changelog**: https://github.com/sahil87/run-kit/compare/v2.4.3...v2.5.0
```

Layout notes (superseding the r01z compact-table layout): with multi-line bodies inline, the
cross-tool two-pass column alignment (tool-name padding + transition padding so `(N releases)`
counts line up) and the per-block tag padding become meaningless and are dropped — release
lines adopt the shared `shll changelog` format verbatim. The per-tool transition line keeps the
2-space `digestToolIndent` under the `What changed:` header; release blocks render unindented
via the shared helper (Assumption #8).

### 2. Color the tool name in per-tool headers

`printToolHeader` (`src/cmd/shll/ui.go:57-63`) color branch currently renders:
`▸` in bold-cyan, `[N/M]` unstyled, `name` in bold-default:

```go
fmt.Fprintf(w, "%s▸%s [%d/%d] %s%s%s\n", ansiBoldCyan, ansiReset, pos, total, ansiBold, name, ansiReset)
```

New: render the whole `[N/M] name` run in **bold-cyan** (matching the `▸`) so tool boundaries
pop visually. The user accepted either "the whole `[N/M] name` or at least the name" — the
whole-run form is chosen (single ANSI span, maximum visual pop).

The plain branch `==> [N/M] name` (non-TTY / `NO_COLOR`) stays **byte-identical** — no ANSI
there, ever.

Because all three surfaces share the function, fixing `printToolHeader` automatically covers
`shll update`, `shll install`, and `shll changelog` (`src/cmd/shll/changelog.go:150`).

### 3. Bold visual anchors on changelog-surface subheadings

With bodies inline, the transition and tag/title lines become the navigational anchors. Give
them bold (`ansiBold`) styling when color is enabled:

- The digest's per-tool transition lines — `rk 2.4.3 → 2.5.0 (1 release)`.
- The release tag/title lines — `v2.5.0  Title` — in **both** the digest and `shll changelog`
  (they render through the shared helper, so this is one code path).
- `shll changelog`'s own transition line (`{old} → {new} ({N} releases)`,
  `renderChangelogResult`) gets the same bold anchor for cross-surface symmetry.

Plain bold, not bold-cyan — bold-cyan stays reserved for the per-tool headers so the visual
hierarchy (header > anchor > body) is preserved.

No other subcommand has per-tool subheadings (`version`/`list` are tabular, `doctor` has its
own format) — out of scope.

### Constraints (all agreed in discussion)

- **`shll shell-init`'s `# ── tool ──` separator stays plain uncolored ASCII** (`toolComment`,
  `ui.go:119`) — its stdout is eval'd (Constitution V, eval-safety). Explicitly out of scope;
  the per-tool-output-separation spec forbids "unifying" it onto the header.
- **All color TTY-gated** via the existing `colorEnabled` decision threaded from the caller
  (`runUpdate`/`runChangelog` compute it once). Non-TTY/`NO_COLOR` output stays plain ASCII
  with zero ANSI, per the per-tool-output-separation spec's degrade rule. The `→`/`—`/`…`
  glyph degrade (`arrow`/`dash`/`more` helpers) is unchanged.
- **Presentation-only**: the digest and headers never influence exit codes.
- **Spec/memory supersede**: this change deliberately supersedes the r01z "titles only, NO
  bodies — one copy-paste away" digest decision. That mandate lives in the memory file
  (`docs/memory/cli/update.md` § digest rendering) and the `update.go` code comment — NOT in
  `docs/specs/per-tool-output-separation.md` (verified: the spec covers headers/tail/separator
  only). During hydrate: supersede the title-only decision in the cli memory files, and update
  the spec's Header-style wording (`▸ <tool>` "bold cyan arrow, bold tool name" → bold-cyan
  name) to match the new header.
- **Tests**: existing tests assert exact digest/header bytes and will be updated to the new
  format — `update_test.go` (digest goldens: `TestUpdate_Digest*`), `ui_test.go`
  (`TestPrintToolHeader_ColorForm`), `changelog_test.go` (release-rendering goldens). Test
  strategy is **test-alongside** (code-quality.md). Per Test Integrity (constitution), the
  tests conform to this intake's new format — never the implementation to stale fixtures.

## Affected Memory

- `cli/update`: (modify) Digest section — full bodies inline via the shared release renderer,
  10-release cap + compare-URL cap notice, dropped `Full notes:` command line, dropped
  column/tag alignment, bold anchors; supersedes the r01z "titles only" decision. Header
  color-form update in the per-tool-output-separation section.
- `cli/changelog`: (modify) Shared release rendering now consumed by the update digest
  (extracted helper); bold anchors on transition + tag/title lines; the "differ only in
  rendering (titles-only digest vs. full-body changelog)" cross-reference becomes "same release
  rendering, digest adds the tool-name transition line".
- `cli/commands`: (modify) Shared UI helper (`ui.go`) section — `printToolHeader` color form
  now bold-cyan `[N/M] name`; any new bold-anchor styling helper.
- `cli/install`: (modify) Minor — the header color-form note it mirrors from update changes
  identically (shared `printToolHeader`).

## Impact

- `src/cmd/shll/update.go` — `printUpdateDigest` rewrite (shared release rendering, cap, bold
  anchors, drop `digestFullNotes`/`digestSpecs`/alignment constants no longer used).
- `src/cmd/shll/changelog.go` — extract shared release-rendering helper from
  `renderChangelogResult`; bold anchors on transition + tag/title lines.
- `src/cmd/shll/ui.go` — `printToolHeader` color branch (bold-cyan `[N/M] name`); possibly a
  small bold-anchor helper (e.g. `bold(color, s)`); plain branches untouched.
- `src/cmd/shll/update_test.go`, `changelog_test.go`, `ui_test.go` — golden-string updates to
  the new format; new assertions for inline bodies + digest cap + header color form.
- No new dependencies, no new subprocesses (Constitution I untouched — ui.go stays
  presentation-only), no exit-code changes, no new subcommands (Constitution VII N/A).
- `docs/specs/per-tool-output-separation.md` — header-style wording update at hydrate.

## Open Questions

None — all decisions were resolved in the originating discussion; remaining latitude is graded
below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Render digest bodies in-process from the existing `changelog.FetchAll` results (Release.Body already in memory); never shell out to `shll changelog` | Agreed in discussion; subprocess re-fetch explicitly rejected (re-fetches GitHub + re-probes brew) | S:95 R:90 A:95 D:95 |
| 2 | Certain | Share `renderChangelogResult`'s release rendering with the digest by extracting the release-block loop + cap logic into one helper both call | Agreed in discussion ("reuse/share ... so the digest and shll changelog render releases in the same format"); extraction shape is the obvious mechanical realization | S:90 R:85 A:90 D:85 |
| 3 | Certain | Apply the existing `changelogCapPerTool = 10` cap + cap-notice/compare-URL pattern to the digest | Agreed in discussion; constant already exists and the shared helper carries it | S:95 R:90 A:95 D:90 |
| 4 | Certain | Drop the `Full notes: shll changelog ...` tail line and remove the now-unused `digestFullNotes`/`digestSpecs` | Agreed in discussion ("now-redundant ... is dropped"); no other consumers (verified) | S:95 R:90 A:95 D:95 |
| 5 | Certain | Header color scope: whole `[N/M] name` run in bold-cyan (not just the name) | User explicitly accepted either form ("the whole `[N/M] name` or at least the name"); whole-run is a single ANSI span and maximizes the stated goal (boundaries pop); trivially reversible | S:85 R:95 A:80 D:65 |
| 6 | Confident | Anchor styling is plain bold (`ansiBold`) for transition + tag/title lines; bold-cyan reserved for headers | User said "e.g. bold" — exact styling left open; bold preserves header > anchor > body hierarchy | S:75 R:90 A:75 D:60 |
| 7 | Confident | `shll changelog`'s own transition line (`{old} → {new} (N releases)`) gets the same bold anchor | Description names the digest's transition lines explicitly; the changelog line is the same navigational anchor and likely the same shared code path — symmetry | S:65 R:90 A:80 D:70 |
| 8 | Confident | Digest layout with inline bodies: keep 2-space `digestToolIndent` on the transition line; drop cross-tool column alignment + per-block tag padding; release blocks render unindented in the shared changelog format | Description silent on layout specifics, but "render releases in the same format" implies the changelog format; alignment is meaningless across multi-line body blocks; goldens make any adjustment cheap | S:45 R:85 A:65 D:45 |
| 9 | Certain | `shll shell-init`'s `# ── tool ──` separator stays plain uncolored ASCII — out of scope | Agreed constraint; Constitution V eval-safety; the spec explicitly forbids unifying it onto the header | S:95 R:95 A:100 D:95 |
| 10 | Certain | All color TTY-gated via the existing threaded `colorEnabled` decision; plain `==> [N/M] name` branch and all non-TTY output stay ANSI-free | Agreed constraint; existing seam (`color bool` threaded from callers); per-tool-output-separation degrade rule | S:90 R:90 A:95 D:95 |
| 11 | Certain | Presentation-only: digest and headers never influence exit codes (`anyFailed` untouched) | Agreed constraint; matches the existing digest contract ("never changes the process exit code") | S:95 R:90 A:95 D:95 |
| 12 | Certain | Update the exact-byte test assertions (update_test.go, ui_test.go, changelog_test.go) to the new format, test-alongside | Agreed; code-quality.md test strategy; constitution Test Integrity — tests conform to the intake'd format | S:90 R:85 A:95 D:90 |
| 13 | Certain | The title-only-digest mandate lives in memory (`cli/update.md`) + the update.go comment, not in the per-tool-output-separation spec; hydrate supersedes it there and updates the spec's header-style wording only | Verified against the repo — the spec (176 lines) never mentions the digest; description's "spec mandates" attribution corrected with evidence | S:85 R:80 A:95 D:85 |
| 14 | Certain | Unavailable/zero-release digest degrade line (`{tool} {old} → {new} — see {compareURL}`) unchanged | No bodies exist to inline in those branches; existing Constitution V contract | S:75 R:85 A:90 D:85 |

14 assumptions (11 certain, 3 confident, 0 tentative, 0 unresolved).
