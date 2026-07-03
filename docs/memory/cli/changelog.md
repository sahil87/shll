---
type: memory
description: "`shll changelog` — positional `tool@old..new` release-notes command: no-range installed→latest (incl. shll-self in the bare sweep), `(old, new]` range semantics, 10-release cap, up-to-date/no-releases/unavailable notices, ASCII-degraded framing, and always-exit-0 fetch degradation."
---
# cli/changelog

`shll changelog` — show GitHub release notes for sahil87 tools. With no arguments it shows the pending releases for every installed tool (installed version → latest release) — "what would an update bring?". Named tools scope it; an explicit `tool@old..new` shows the releases in `(old, new]` regardless of what is installed.

Source: `src/cmd/shll/changelog.go`; release fetching in [internal/changelog](/internal/changelog.md). Introduced by change r01z.

## Constitution VII justification

Changelog display cannot be a flag on `update` — its core purpose is *deterministic re-display after the fact* (a user who wants to re-read notes must not have to re-run an upgrade), and it has standalone value independent of updating (pre-update preview, historical range queries). It does not belong in a per-tool CLI because its job is cross-toolkit aggregation — exactly shll's mandate. It is the **eighth** user-facing subcommand; the registration is in [cli/commands](/cli/commands.md#cobra-root).

## Argument grammar

`Args: cobra.ArbitraryArgs` — zero or more positional specs, each `tool` (no-range) or `tool@old..new` (explicit range). The separators are named constants (`specToolSep = "@"`, `specRangeSep = ".."`). The cobra factory `newChangelogCmd()` delegates to the test seam `runChangelog(ctx, stdout, stderr, args)` (explicit `io.Writer`s, mirroring `runUpdate`/`runList`), driven directly by `changelog_test.go` with `bytes.Buffer` writers, a fake `proc.Runner`, and the `internal/changelog` `SetTransportForTest` seam.

```
shll changelog                          # all installed tools: installed → latest (incl. shll first)
shll changelog tu                       # one tool: installed → latest
shll changelog tu@0.6.2..0.6.4          # explicit range: releases in (0.6.2, 0.6.4]
shll changelog tu@0.6.2..0.6.4 hop@0.1.16..0.1.18   # multiple (this is what `shll update`'s digest prints)
```

- **Valid names** — the six Roster names **plus `shll`** itself. Names are validated via the shared `resolveTargets(names, true)` (`allowShll=true`), which reports **all** unknowns at once listing valid targets, matching `update`'s diagnostic: `shll changelog: unknown target "foo" (valid targets: shll, wt, idea, tu, rk, hop, fab-kit)` — with **no** network/brew side effect. (`parseChangelogSpecs` ignores the resolver's returned ordering and re-derives roster order + ranges itself.)
- **Versions** accepted with or without a `v` prefix; brew `_N` revision suffixes stripped. A `tu@v0.6.2..v0.6.4` spec parses **and displays** identically to the unprefixed form (`NormalizeVer` applied to both bounds — never echoes the user's raw `v`). Pinned by `TestChangelog_VPrefixedSpecNormalizes`.
- **Dedup** — a repeated tool name keeps the last spec (map-by-name).
- An `@` with a malformed body (missing `..` or an empty side) is a hard error: `invalid range "tu@0.6.2" (want tool@old..new)` (`TestChangelog_InvalidRangeErrors`).
- **Output order is roster order regardless of arg order, shll first when named** (`TestChangelog_RosterOrderRegardlessOfArgOrder`).

## Resolution semantics

`resolveChangelog` resolves every spec **concurrently** — one goroutine per spec, results indexed by position to preserve spec/roster order (mirroring `FetchAll`/`probeRoster`). Each spec makes at most one brew read + one GitHub fetch:

- **Explicit range** (`tool@old..new`) — never consults brew; normalizes both bounds (`NormalizeVer`) and calls `changelog.FetchRange` with `(old, new]`. Works regardless of install state (`TestChangelog_ExplicitRangeWorksUninstalled`).
- **No-range** (bare-command sweep or a `tool`-only spec) — the range is `installed-version → latest-release`. The installed anchor comes from brew (`installedVersion` → `brew list --formula --versions`, via `internal/proc`); the latest comes from a single `changelog.LatestTag` fetch whose returned release list is then **filtered locally** via `changelog.ReleasesInRange` (one GET per repo, never a second fetch — `TestChangelog_NoRangeSingleFetchPerRepo` asserts GET count == 1).

### The bare sweep (no args)

A bare `shll changelog` builds `defaultChangelogSpecs()`: **shll itself first** (`self: true`, `rosterIx: -1`), then one no-range spec per roster tool — symmetry with bare `shll update` (which self-upgrades shll) and the intake's "shll first when included". shll-self's installed anchor is its **brew-formula** version (`installedVersion(ctx, shllFormula)`), **not** the running process's ldflags `shllSelfVersion()` — the changelog range should span the on-disk brew formula, not the live binary. Pinned by `TestChangelog_BareSweepIncludesShllSelf`.

- Bareness is tracked from the **arg count** (`bare := len(specs) == 0`), not inferred from the spec set — because it changes the missing-tool policy (below).
- A bare sweep **gracefully skips** uninstalled tools (they resolve to `skip: true` and drop silently). A bare sweep where **nothing** is installed prints the same nothing-to-do line as update — `No sahil87 tools installed.` (the shared `noToolsInstalledMsg` constant) — not silent empty output (`TestChangelog_BareSweepZeroInstalledPrintsMessage`).

### Named-but-not-installed

Missing-tool handling depends on the form:
- **No-range** (bare sweep member OR an explicitly-named `tool`) — a named-but-not-installed tool is an **error** only when explicitly named: `shll changelog: rk: not installed` (all missing names collected and reported at once, in spec/roster order, before any rendering). A bare-sweep member instead skips silently.
- **Explicit range** — **never** an error; it never consults brew, so it works whether or not the tool is installed.

Pinned by `TestChangelog_NamedNotInstalledErrorsNoRangeOnly`.

### Brew precondition — gated on whether brew is actually read

The no-range forms need brew to anchor the range, so `specsNeedBrew(specs)` (true iff any spec is non-explicit) gates a `hasBrew` check that prints the shared `brewMissingHint` and returns `errSilent` when brew is absent — mirroring how `install`/`update` gate on brew only when they will read it. A run made up **entirely** of explicit `tool@old..new` ranges skips the precondition (`TestChangelog_NoRangeBrewMissingHint`, `TestChangelog_ExplicitRangeSkipsBrewCheck`).

## Output shape

For each rendered tool (in roster order, shll first), `runChangelog` prints the shared per-tool header `printToolHeader(stdout, tool, pos, total, color)` (the `▸ [N/M]` / `==> [N/M]` framing from [ui.go](/cli/commands.md#shared-ui-helper-uigo)), then the body:

- **A release range** (`renderChangelogResult`) — a header line `{old} → {new} ({N} release[s])`, then each release newest-first as `{tag}  {title}` followed by the **full release body printed as-is** (the auto-generated "What's Changed" markdown, trailing newline trimmed). `old`/`new` are the already-normalized (v-stripped) forms, so both sides read in one form. `TestChangelog_ExplicitRangeHappyPath`.
- **10-release cap** (`changelogCapPerTool = 10`) — when the range holds more, only the 10 newest print, followed by a cap notice + the Full Changelog compare URL: `… {N-10} more — full changelog: {compareURL}` (`TestChangelog_CapOverflow`).
- **No releases in range** — an explicit range where `old == new` or that matched zero releases prints the header line then `no releases in range` (`TestChangelog_EmptyRange`).
- **Up-to-date notice** (no-range, installed already ≥ latest) — `up to date at {version} — {releasesURL}`, exit 0 (not an error). Decided via `changelog.CompareVer(latest, installed) <= 0` (`TestChangelog_UpToDateNotice`).
- **No-range latest fetch failed** — a dedicated `changelog unavailable — {releasesURL}` notice, **not** an `X → X — see {compareURL}` self-compare (the r01z rework: when the latest is unknown there is no genuine range to show, so pointing at the releases page is honest; a self-compare would misread as "no change").
- **Explicit-range fetch unavailable** — degrades to `{old} → {new} — see {compareURL}` and continues with the other tools.

The one-line notices (`noteKind`: `noteUpToDate`/`noteUnavailable`) are built at **render** time, not resolution time, so their `—` glyph ASCII-degrades with the stream's color decision.

## Degradation contract (Constitution V) + exit codes

A release fetch that is **unavailable** (any failure per [internal/changelog](/internal/changelog.md#degradation-contract-constitution-v)) degrades that tool's entry to a compare-URL (or releases-URL) fallback line and the run continues with the other tools; the fetch failure **never** changes the exit code — `shll changelog` exits 0 (`TestChangelog_UnavailableDegradesToCompareURL`). A multi-tool run still renders the tools whose fetches succeeded.

| Condition | Exit code |
|-----------|-----------|
| Successful render (incl. up-to-date / no-releases / unavailable-degraded / cap notices) | 0 |
| Unknown / malformed-range positional spec | 1 (via `errSilent`, before any network/brew) |
| Named-but-not-installed tool in a **no-range** form | 1 (via `errSilent`, after resolution, before render) |
| Brew missing AND at least one no-range spec | 1 (via `errSilent`, `brewMissingHint` on stderr) |
| Any release fetch failure | **never affects the exit code** — 0 |

## ASCII degrade (per-tool-output-separation spec)

The changelog surface's own Unicode framing degrades on a non-TTY / `NO_COLOR` stream per `docs/specs/per-tool-output-separation.md` § Header style: `→` → `->`, `—` → `--`, `…` → `...`. These are emitted via the shared `arrow(color)` / `dash(color)` / `more(color)` helpers in `ui.go` (glyph pairs `arrowGlyph`/`arrowASCII` etc.). The color decision is computed once (`colorEnabled(stdout)`) and threaded into the renderer. A `bytes.Buffer` test writer is never a TTY, so tests deterministically assert the ASCII forms (`0.6.2 -> 0.6.4`, `-- see`). This is distinct from the spec-mandated em-dash in the summary tail and the shell-init box-drawing chars, which the spec **exempts** from the degrade — those live in different surfaces; the changelog framing is covered by the general degrade rule.

## Cross-references

- Release fetch/filter/degradation package: [internal/changelog](/internal/changelog.md).
- Shared version transitions + the copy-pasteable `shll changelog` command that `shll update` prints: [cli/update §"What changed:" digest](/cli/update.md#version-capture--the-what-changed-digest-change-r01z). Both surfaces share the `internal/changelog` fetch/filter code and differ only in rendering (titles-only digest vs. full-body changelog).
- Root wiring + the eight-user-facing-subcommand count: [cli/commands](/cli/commands.md#cobra-root).
- Shared `ui.go` framing (`printToolHeader`, color gating, glyph degrade): [cli/commands §shared UI helper](/cli/commands.md#shared-ui-helper-uigo).
- The `rk`/`run-kit` repo-slug footgun (`Repo` field, single-sourced compare URLs): [cli/commands §hardcoded tool roster](/cli/commands.md#hardcoded-tool-roster).
- ASCII-degrade rule: `docs/specs/per-tool-output-separation.md`.
