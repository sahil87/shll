# Plan: Changelog command + update release digest

**Change**: 260703-r01z-changelog-command-update-digest
**Intake**: `intake.md`

## Requirements

### internal/changelog: GitHub-releases fetch package

#### R1: Isolated stdlib-HTTP fetch package
The GitHub-releases network I/O SHALL live in a new internal package `src/internal/changelog/`, using only Go stdlib `net/http`. Command code (`src/cmd/shll/`) SHALL NOT import `net/http` — it calls this package's exported API instead (mirroring the `internal/proc` isolation for subprocesses, Constitution I spirit).

- **GIVEN** the repo today has zero `net/http` imports in `src/cmd/shll/`
- **WHEN** the changelog feature is implemented
- **THEN** `net/http` is imported only within `src/internal/changelog/`, and `src/cmd/shll/changelog.go` / `update.go` call the package's exported functions
- **AND** no new module dependency is added to `src/go.mod`

#### R2: Fetch releases for a repo
The package SHALL expose a function that fetches a repo's releases via `GET {baseURL}/repos/sahil87/{Repo}/releases?per_page=100`, unauthenticated, with a bounded per-request timeout. The API base URL and timeout SHALL be named constants (base default `https://api.github.com`, timeout `10s`). Each release carries at least its tag name, title, and body.

- **GIVEN** a repo slug `hop`
- **WHEN** releases are fetched
- **THEN** the request goes to `{baseURL}/repos/sahil87/hop/releases?per_page=100` with no Authorization header and a 10s context timeout
- **AND** the decoded releases expose tag, title (name), and body fields

#### R3: Range filter with version normalization
The package SHALL select releases whose tag falls in the half-open range `(old, new]`, newest first, after normalizing versions: strip a leading `v` prefix and a brew revision suffix (`_N`), then compare numerically by dot-separated components. A release exactly equal to `old` is excluded; one equal to `new` is included.

- **GIVEN** releases tagged `v0.6.2`, `v0.6.3`, `v0.6.4` and a range `0.6.2..0.6.4`
- **WHEN** the range filter runs
- **THEN** it returns `v0.6.4` then `v0.6.3` (newest first), excluding `v0.6.2`
- **AND** a brew version `0.6.4_1` normalizes to `0.6.4` for the compare
- **AND** versions with differing component counts (e.g. `0.6` vs `0.6.0`) compare by numeric components with missing components treated as `0`

#### R4: Typed unavailable result on any failure (degradation contract)
Any fetch failure — network error, non-200 status (including 403 rate-limit), JSON parse failure, or timeout — SHALL be reported as a typed "unavailable" outcome rather than a fatal error, so callers render a compare-URL fallback and continue. No retries in v1.

- **GIVEN** the GitHub API returns 403 (rate limited) or the request times out
- **WHEN** a caller requests a tool's changelog range
- **THEN** the package returns an unavailable result (a sentinel error or an `unavailable`-flagged value), never a panic or a process-fatal error
- **AND** the caller can distinguish "unavailable" from "fetched but zero releases in range"

#### R5: Concurrent fetch, roster-ordered results
Fetches for multiple tools MAY run concurrently (read-only HTTP, mirroring the `probeRoster` carve-out), but results SHALL be assembled in roster order regardless of completion order.

- **GIVEN** changelog is requested for `tu` and `hop`
- **WHEN** their release fetches run concurrently
- **THEN** the rendered output lists them in roster order (`tu` before `hop`), independent of which fetch finished first

### cli/changelog: the `shll changelog` command

#### R6: New top-level `changelog` subcommand
A new cobra command `shll changelog` SHALL be added in `src/cmd/shll/changelog.go` and wired in `root.go` alongside the existing factories. Its `RunE` SHALL delegate to a testable `runChangelog(ctx, stdout, stderr, args)` seam taking explicit `io.Writer`s (mirroring `runUpdate`/`runList`).

- **GIVEN** the shll binary
- **WHEN** `shll changelog --help` is run
- **THEN** the command exists, is registered on the root, and shows usage
- **AND** `runChangelog` is invokable directly by tests with `bytes.Buffer` writers and a fake `proc.Runner`

#### R7: Positional `tool@old..new` argument grammar
`shll changelog` SHALL accept zero or more positional specs. A bare spec `tool` means "installed → latest"; `tool@old..new` is an explicit range. Valid tool names are the Roster names plus `shll` itself (reuse `resolveTargets`-style validation with `allowShll=true`). An unknown name is a hard error listing valid targets, mirroring `update`. Versions are accepted with or without a `v` prefix.

- **GIVEN** `shll changelog tu@0.6.2..0.6.4 hop@0.1.16..0.1.18`
- **WHEN** the args are parsed
- **THEN** each spec resolves to a tool + explicit `(old, new]` range
- **AND** `shll changelog foo` errors with `shll changelog: unknown target "foo" (valid targets: shll, wt, idea, tu, rk, hop, fab-kit)` and no network/brew side effect
- **AND** a spec `tu@v0.6.2..v0.6.4` (with `v` prefixes) parses identically to the unprefixed form

#### R8: No-range forms resolve installed → latest
For a bare-command run (`shll changelog`, all installed tools) or a `tool`-only spec, the range is `installed-version → latest-release`. The installed version comes from the brew probe (`brew list --formula --versions`, via `internal/proc`); the latest comes from the fetched releases (newest tag). A tool already at the latest version prints an "up to date" line plus the releases-page URL (not an error). A named-but-not-installed tool is an error ONLY for the no-range forms (they need an installed anchor); an explicit `tool@old..new` never consults brew and works regardless of install state.

- **GIVEN** `tu` is installed at `0.6.4` and its latest release is `v0.6.4`
- **WHEN** `shll changelog tu` runs
- **THEN** it prints an up-to-date notice for `tu` plus its releases-page URL, exit 0
- **AND** `shll changelog rk` with `rk` not installed prints `shll changelog: rk: not installed` (error), whereas `shll changelog rk@0.1.0..0.2.0` works without consulting brew

#### R9: Per-tool full changelog output
For each tool in a run, output SHALL be a header line `{tool} {old} → {new} ({N} releases)` followed by each release in the range, newest first — tag, title, and the full release body printed as-is. Output SHALL be in roster order regardless of argument order, with `shll` first when included. Framing SHALL follow existing `ui.go` conventions (TTY-gated via `colorEnabled`, per-tool separation).

- **GIVEN** a resolved range for `hop` with 2 releases
- **WHEN** `shll changelog hop@0.1.16..0.1.18` renders
- **THEN** it prints `hop 0.1.16 → 0.1.18 (2 releases)` then each release's tag + title + body, newest first
- **AND** `shll changelog hop tu` and `shll changelog tu hop` both print `tu` before `hop` (roster order)

#### R10: Release cap and empty-range handling
Output SHALL be capped at the 10 most recent releases per tool; when the range holds more, a cap notice plus the `Full Changelog` compare URL (`https://github.com/sahil87/{Repo}/compare/v{old}...v{new}`) SHALL be printed. An explicit range where `old == new` or that contains zero matching releases prints "no releases in range".

- **GIVEN** a range containing 15 matching releases
- **WHEN** it renders
- **THEN** only the 10 newest are shown, followed by a cap notice and the compare URL
- **AND** an explicit `tu@0.6.4..0.6.4` (old == new) prints "no releases in range" for `tu`

#### R11: Fetch-failure degradation in changelog
When the release fetch for a tool is unavailable (R4), `shll changelog` SHALL degrade that tool's entry to a compare-URL fallback line and continue with the other tools; the fetch failure SHALL NOT change the command's exit code (Constitution V).

- **GIVEN** the API is rate-limited (403) for `hop`
- **WHEN** `shll changelog hop@0.1.16..0.1.18` runs
- **THEN** `hop`'s entry degrades to `hop 0.1.16 → 0.1.18 — see {compare URL}` and exit code is 0
- **AND** a multi-tool run still renders the tools whose fetches succeeded

### cli/update: version capture + digest tail

#### R12: Capture before-versions from the existing probe
`shll update` SHALL capture each installed tool's before-version from the `brew list --formula --versions` output the probe already runs (a captured `proc.Run` read — NOT parsing streamed foreground output). The captured version is the second whitespace-separated field of the `brew list --versions` stdout. shll-self's before-version comes from the existing `shllSelfVersion()` package-var path.

- **GIVEN** `probeTool` runs `brew list --formula --versions sahil87/tap/hop` returning `hop 0.1.16`
- **WHEN** the probe result is built
- **THEN** `probeResult` carries `beforeVersion = "0.1.16"` for hop
- **AND** no streamed foreground brew/tool output is ever parsed for versions (code-quality anti-pattern)

#### R13: Capture after-versions post-upgrade
After each successful upgrade (exit 0), `shll update` SHALL re-query `brew list --formula --versions <formula>` (a cheap captured read) to obtain the new version. shll-self's after-version is re-read the same way against `shllFormula`.

- **GIVEN** hop upgraded successfully from `0.1.16`
- **WHEN** the post-upgrade re-query runs
- **THEN** an after-version is captured for hop from a fresh `brew list --versions` read
- **AND** the re-query runs only for tools whose upgrade exited 0

#### R14: Digest tail for bumped tools
After the existing summary tail, for every tool whose version actually changed (`before != after` and both known), `shll update` SHALL print a "What changed:" digest: per-tool `{tool} {old} → {new} ({N} releases)` lines plus one title line per release (tag + release title only, NO bodies), then a copy-pasteable full-notes `shll changelog ...` command naming exactly the bumped tools with their ranges. Output is in roster order (shll first when bumped).

- **GIVEN** `tu` bumped `0.6.2→0.6.4` (2 releases) and `hop` bumped `0.1.16→0.1.18` (2 releases)
- **WHEN** the run completes
- **THEN** a `What changed:` block prints per-tool transition + release title lines, followed by `Full notes: shll changelog tu@0.6.2..0.6.4 hop@0.1.16..0.1.18`
- **AND** the digest is rendered in roster order regardless of upgrade order

#### R15: Digest edge cases and presentation-only guarantee
The digest SHALL:
- print nothing (no digest, no command line) when no tool's version changed (all up-to-date or all failed) — silence, exactly as today;
- print nothing under `--dry-run` (nothing was upgraded);
- for a subset run, cover only bumped subset members and name only those tools in the command;
- degrade a bumped tool whose fetch fails or whose fetched range holds zero matching releases to `{tool} {old} → {new} — see {compare URL}` (Constitution V);
- never influence `anyFailed` or the process exit code.

- **GIVEN** a run where no version changed
- **WHEN** the run completes
- **THEN** stdout is byte-identical to today's (no `What changed:` block) — every existing golden string is preserved
- **AND** a fetch failure for a bumped tool degrades that line to the compare URL without changing the exit code

### cli/commands: subcommand wiring

#### R16: Register changelog on the root
`root.go` SHALL wire `newChangelogCmd()` into the cobra root's `AddCommand`, and `rootLong` SHALL list `shll changelog` among the user-facing subcommands. This is the eighth user-facing subcommand (Constitution VII justification is recorded in the intake).

- **GIVEN** `shll --help`
- **WHEN** the root help renders
- **THEN** `changelog` appears in the subcommand list
- **AND** `newRootCmd()` registers the changelog factory

### Non-Goals

- No `GITHUB_TOKEN`/auth support in v1 (7 unauthenticated requests/run vs. the 60/hr limit is ample).
- No caching (Constitution II — stateless).
- No retry/backoff on fetch failure.
- No changes to `shell-init`/`version`/`install`/`list`/`doctor` behavior.
- No new module dependencies.

### Design Decisions

1. **Package name `internal/changelog`**: matches the Affected-Memory anchor (`internal/changelog` (new)) and the domain naming. — *Why*: the intake's load-bearing constraint is the boundary (no `net/http` in cmd code), and `changelog` names the boundary's purpose clearly. — *Rejected*: `internal/ghrel` (less discoverable; the memory domain is already `changelog`).
2. **HTTP test seam = injectable base URL + swappable `*http.Client`**: a package-level `baseURL` var (default `https://api.github.com`) and a package-level `httpClient` the tests point at an `httptest.Server`. — *Why*: mirrors `proc.Runner`'s package-level-swappable seam; lets tests exercise real `net/http` code paths (status codes, JSON decode, timeouts) against a local server without network. — *Rejected*: an interface-typed transport threaded through every call (heavier than the established package-var seam).
3. **Before-version parsing = second field of `brew list --versions` stdout**: the output is `<leaf> <version>`. — *Why*: it is the captured read the probe already pays for; no extra subprocess, no streamed-output parsing. — *Rejected*: a separate `brew info --json=v2` call (extra latency; the version is already on the `brew list` line).
4. **Digest fetches reuse the same `internal/changelog` package** the `changelog` command uses. — *Why*: single source of truth for range-filtering + degradation; `update`'s digest and `changelog`'s full output differ only in rendering (titles-only vs. full bodies).

## Tasks

### Phase 1: internal/changelog package (fetch + filter + degradation)

- [x] T001 Create `src/internal/changelog/changelog.go`: package doc; named constants `apiBaseDefault = "https://api.github.com"` and `requestTimeout = 10 * time.Second`; package-level seams `var baseURL = apiBaseDefault` and `var httpClient = &http.Client{}`; a `Release` struct (`Tag`/`TagName`, `Name`/title, `Body`) decoded from the GitHub JSON (`tag_name`, `name`, `body`). <!-- R1 R2 --> <!-- rework: single-source the sahil87 owner segment inside the package (ownerPrefix/orgRepoPrefix re-encode it twice at changelog.go:34-44); fix the stale non-200 "drain" comment at :229-232 (branch never reads resp.Body) — either drain a bounded amount into the error or correct the comment --> (Reworked: added a single `owner = "sahil87"` constant; `githubOrgBase` and `orgRepoPrefix` now derive from it, so the owner is encoded once. Corrected the non-200 comment to state the body is intentionally not read — the status code alone is the degradation signal — rather than claiming a drain.)
- [x] T002 Implement release fetch in `changelog.go`: `func fetchReleases(ctx, repo string) ([]Release, error)` issuing `GET {baseURL}/repos/sahil87/{repo}/releases?per_page=100` with a `context.WithTimeout(ctx, requestTimeout)`, unauthenticated; return a sentinel `ErrUnavailable` (wrapped with detail) on non-200, transport error, timeout, or JSON decode failure. <!-- R2 R4 -->
- [x] T003 Implement version normalization + compare in `changelog.go`: `normalizeVer(s) string` (strip leading `v`, strip brew `_N` suffix) and `compareVer(a, b) int` (numeric dot-component compare, missing components as 0). <!-- R3 -->
- [x] T004 Implement range filter in `changelog.go`: `func releasesInRange(rels []Release, old, new string) []Release` selecting tags in `(old, new]`, newest first (sort by normalized version descending). <!-- R3 R10 -->
- [x] T005 Define the public result type + entry point in `changelog.go`: a `Result` carrying `Tool`, `Old`, `New`, `Releases []Release`, `Unavailable bool` (and/or error), plus `func FetchRange(ctx, repo, old, new string) Result` composing fetch+filter and setting `Unavailable` on `ErrUnavailable`. Add `func CompareURL(repo, old, new string) string` and `func ReleasesURL(repo string) string` for the fallback/up-to-date URLs. <!-- R4 R10 R11 -->
- [x] T006 [P] Implement concurrent multi-tool fetch helper in `changelog.go`: `func FetchAll(ctx, reqs []RangeReq) []Result` (one goroutine per request, results indexed to preserve caller order). <!-- R5 -->

### Phase 2: internal/changelog tests

- [x] T007 Create `src/internal/changelog/changelog_test.go`: drive `fetchReleases`/`FetchRange` against an `httptest.Server` (swap `baseURL` via a t.Cleanup helper) — happy path (200 + JSON), 403 rate-limit → unavailable, malformed JSON → unavailable, request path/query assertion (`/repos/sahil87/hop/releases?per_page=100`, no Authorization header). <!-- R2 R4 -->
- [x] T008 [P] Unit-test normalize/compare/range in `changelog_test.go`: `v`-prefix + `_N` suffix normalization, numeric component compare (incl. differing component counts), `(old, new]` half-open selection newest-first, `old == new` → empty, cap boundary. <!-- R3 R10 -->
- [x] T009 [P] Test `FetchAll` roster-order preservation and `CompareURL`/`ReleasesURL` formatting in `changelog_test.go`. <!-- R5 R10 -->

### Phase 3: cli/changelog command

- [x] T010 Add spec parsing to `src/cmd/shll/changelog.go`: parse `tool[@old..new]` positional specs; reuse `resolveTargets(names, true)` for name validation (all specs' tool names collected, unknowns reported at once listing valid targets); represent each resolved spec as a struct carrying the `Tool`/self flag, explicit-range flag, and old/new. Named constants for the `@` and `..` separators. <!-- R7 -->
- [x] T011 Implement `runChangelog(ctx, stdout, stderr, args)` in `changelog.go`: for no-range specs, probe installed version via `brew list --formula --versions` (through `internal/proc`) as `old` and fetched latest as `new`; error on named-but-not-installed for no-range forms only; for explicit ranges skip brew entirely; assemble results in roster order (shll first when included) via `changelog.FetchAll`. <!-- R6 R8 R9 --> <!-- rework: (1) eliminate the no-range double-fetch — LatestTag's returned release list must be threaded through (fetch once, filter locally), never re-fetched by FetchAll, and the up-to-date branch must not enqueue a discarded fetch; ~7 GETs per bare sweep, not 14; (2) resolve no-range specs concurrently (mirror FetchAll/probeRoster) instead of serially in the loop; (3) include shll-self in the bare sweep (symmetry with bare `shll update`; intake: "shll first when included") using the brew-formula version as anchor, not ldflags shllSelfVersion(); (4) bare run with zero installed tools prints "No sahil87 tools installed." like update, not silence; (5) no-range forms check hasBrew first and print the brew-missing hint like update/install (explicit ranges rightly skip brew) --> (Reworked: replaced the two-phase resolve+FetchAll design with a single concurrent `resolveChangelog` (one goroutine per spec). No-range specs now call `LatestTag` once and filter its returned list locally via the new exported `changelog.ReleasesInRange` — exactly one GET per repo (asserted by `TestChangelog_NoRangeSingleFetchPerRepo`). shll-self is prepended to the bare sweep and anchored on `installedVersion(ctx, shllFormula)` (brew), not ldflags. Bare sweep with nothing installed prints `noToolsInstalledMsg`. A `specsNeedBrew` gate runs `hasBrew` + prints `brewMissingHint` for any no-range form; explicit-only runs skip it.)
- [x] T012 Implement full-changelog rendering in `changelog.go`: per-tool header `{tool} {old} → {new} ({N} releases)`, each release tag+title+body newest-first; up-to-date notice + releases URL when `old == new` at latest (no-range); "no releases in range" when explicit range empty; 10-release cap + cap notice + compare URL on overflow; unavailable → compare-URL fallback line; TTY framing via `colorEnabled`/`printToolHeader` conventions from `ui.go`. Fetch failures never change exit code. <!-- R9 R10 R11 --> <!-- rework: (1) normalize displayed versions to one form — don't echo the user's raw v-prefixed spec into headers or mix brew-form old with tag-form new (`0.6.2 → v0.6.4`); display the normalized (v-stripped) form on both sides; (2) non-TTY/NO_COLOR output must ASCII-degrade the `→`/`—`/`…` glyphs per docs/specs/per-tool-output-separation.md:52-54 (`→` becomes `->`) — pass the color/TTY decision into the renderer; (3) the LatestTag-failure fallback must emit an explicit unavailable note rather than an old==new self-compare request --> (Reworked: added `changelog.NormalizeVer` and normalize every displayed bound (both explicit-spec and no-range latest) so a `tu@v0.6.2..v0.6.4` spec renders `0.6.2 -> 0.6.4`. Added shared `arrow`/`dash`/`more` glyph helpers in `ui.go` (degrade `→/—/…` → `->/--/...`); `renderChangelogResult` takes `color` and the caller prints the header. LatestTag-failure for a no-range spec now renders a dedicated `noteUnavailable` — `changelog unavailable — <releases URL>` — not an `X → X — see <compareURL>` self-compare. Notices are built at render time so their `—` also ASCII-degrades.)
- [x] T013 Add `newChangelogCmd()` factory in `changelog.go` (cobra `Use: "changelog [tool[@old..new]]..."`, `Args: cobra.ArbitraryArgs`, `RunE` → `runChangelog`) and register it in `src/cmd/shll/root.go` `AddCommand`; add the `shll changelog` line to `rootLong`. <!-- R6 R16 -->

### Phase 4: cli/update version capture + digest

- [x] T014 Extend `probeResult` in `src/cmd/shll/update.go` with `beforeVersion string`; parse it from the `brew list --formula --versions` stdout. Add a small captured-read helper `installedVersion(ctx, formula) string` (in `brew.go`) that runs `brew list --formula --versions <formula>` and returns the second whitespace field (empty on any failure), and use it inside `probeTool`; capture shll-self before-version from `shllSelfVersion()`. <!-- R12 --> (Implemented as `probeInstalledVersion` returning both the exit-code install fact and the version in one brew read — keeps isInstalled's contract intact and parses the version separately; shll-self before-version is the brew-formula version, symmetric with roster tools.) <!-- rework: MUST-FIX — collapse the duplicated brew invocation: isInstalled (brew.go:130-133) must become a wrapper over probeInstalledVersion (`installed, _ := probeInstalledVersion(ctx, formula); return installed`) so the identical proc.Run("brew","list","--formula","--versions") exists exactly once (per plan Deletion Candidates entry 1). ALSO: parseBrewVersion must not take fields[1] blindly — `brew list --versions` can list multiple kegs in arbitrary order (`tu 0.6.2 0.6.4`); pick the max across fields[1:] via the version compare so multi-keg hosts don't report wrong/suppressed transitions --> (Reworked: `isInstalled` is now the one-line wrapper `installed, _ := probeInstalledVersion(ctx, formula); return installed`, so `probeInstalledVersion` holds the sole `brew list --formula --versions` invocation in cmd/shll. `parseBrewVersion` now picks the max across `fields[1:]` via `changelog.CompareVer` (multi-keg lines like `tu 0.6.4 0.6.2` report the current version, not fields[1]); covered by `TestParseBrewVersion_MultiKegPicksMax`.)
- [x] T015 Capture after-versions in `runUpdate` (`update.go`): after each successful (exit 0) upgrade — roster tool and shll-self — re-query the new version via `installedVersion(ctx, t.Formula)` / `installedVersion(ctx, shllFormula)`; record `(tool, before, after)` transitions for tools where both are known and `before != after`. <!-- R13 -->
- [x] T016 Implement the digest tail in `runUpdate` + a renderer (`printUpdateDigest` in `update.go`): after `printSummaryTail`, for the recorded bumped set (roster order, shll first), call `changelog.FetchAll` per tool, print `What changed:` + per-tool transition + one title line per release (no bodies) + the `Full notes: shll changelog <specs>` command; degrade unavailable/zero-range tools to `{tool} {old} → {new} — see {compareURL}`. No digest when the bumped set is empty or under `--dry-run`. Presentation-only: never touch `anyFailed`/exit code. <!-- R14 R15 --> <!-- rework: (1) consolidate the shll-self triple brew read — use probeInstalledVersion once at update.go:154 for install fact + before-version, dropping the separate installedVersion call at :311 (per plan Deletion Candidates entry 2); (2) pass runUpdate's already-computed color decision into printUpdateDigest and ASCII-degrade `→`/`—`/`…` when off, per docs/specs/per-tool-output-separation.md; (3) column-align the digest transition lines per the intake's agreed sample (printPreviewRows label-padding idiom) --> (Reworked: the shll-self install fact + before-version now come from a single `probeInstalledVersion(ctx, shllFormula)` at the top of runUpdate (`beforeShll`), dropping the former separate `installedVersion` read before the upgrade. `printUpdateDigest` takes the `color` computed once by runUpdate and degrades `→/—` via the shared `arrow`/`dash` helpers. Transition lines are two-pass column-aligned: tool-name column padded to the widest name, `{old} → {new}` transition column padded so the `(N releases)` counts line up, and each block's release tags padded to the widest tag — matching the intake sample. Covered by `TestUpdate_DigestColumnAlignment` and `TestUpdate_DigestMixedAvailableAndUnavailable`.)

### Phase 5: cli/update + wiring tests

- [x] T017 Add tests to `src/cmd/shll/changelog_test.go`: `runChangelog` explicit-range happy path (fake `proc.Runner` + `internal/changelog` `baseURL` → `httptest.Server`), roster-order output, no-range installed→latest, up-to-date notice, named-not-installed error (no-range only), explicit-range works uninstalled, unknown-target error, cap + empty-range, unavailable→compare-URL fallback with exit 0. <!-- R6 R7 R8 R9 R10 R11 --> <!-- rework: update tests for the reworked behavior (single-fetch no-range path — assert request COUNT against the httptest server, ~1 GET per repo; shll-self in bare sweep; zero-installed message; brew-missing hint; normalized display forms; ASCII degrade in non-TTY buffers) and ADD the missing R7 v-prefixed spec test (tu@v0.6.2..v0.6.4) --> (Reworked: existing assertions updated to the ASCII-degraded/normalized forms (`0.6.2 -> 0.6.4`, `-- see`). Added `TestChangelog_NoRangeSingleFetchPerRepo` (GET count == 1), `TestChangelog_VPrefixedSpecNormalizes` (R7 v-prefix), `TestChangelog_BareSweepIncludesShllSelf`, `TestChangelog_BareSweepZeroInstalledPrintsMessage`, `TestChangelog_NoRangeBrewMissingHint`, `TestChangelog_ExplicitRangeSkipsBrewCheck`.)
- [x] T018 Add tests to `src/cmd/shll/update_test.go`: version bump → `What changed:` digest with correct transition + title lines + `Full notes:` command; NO digest when before==after (assert existing golden strings unchanged); NO digest under `--dry-run`; subset run names only bumped subset; unavailable fetch → compare-URL degradation, exit code unaffected. <!-- R14 R15 --> <!-- rework: update goldens for column-aligned + (in non-TTY test buffers) ASCII-degraded digest; add a multi-keg `brew list --versions` line test (max-version pick); add a mixed available/unavailable FetchAll digest test (partial degradation in one run) --> (Reworked: digest assertions updated to ASCII-degraded arrow/dash. Added `TestUpdate_DigestColumnAlignment` (exact aligned goldens for a two-tool digest with differing name+transition widths), `TestUpdate_DigestMixedAvailableAndUnavailable` (one served + one 404 tool in the same digest), and `TestParseBrewVersion_MultiKegPicksMax`. The no-bump golden (`TestUpdate_NoDigestWhenNothingBumped`) remains byte-identical.)
- [x] T019 Add a root-wiring test (extend `help_dump`/root coverage or a focused test) asserting `changelog` is registered and appears in `rootLong`; add a `changelog`-package `net/http`-isolation check consistent with the existing no-`os/exec`-in-cmd discipline. <!-- R1 R16 -->

### Phase 6: build + full verification

- [x] T020 `cd src && go build ./...`, `go vet ./...`, `gofmt -l` on touched files, then `go test ./... -race`; fix any failures. <!-- R1 R2 R3 R4 R5 R6 R7 R8 R9 R10 R11 R12 R13 R14 R15 R16 --> <!-- rework: re-verify after rework edits; also add the missing `shll changelog` entry to README.md's Commands section (every other command is documented there) --> (Reworked: build/vet clean, `gofmt -l` reports no diffs on touched files, `go test ./... -race` all green. Added a `### shll changelog` section to README.md's Commands, a `shll changelog` digest note under `shll update`, and a `shll changelog` row to the "How composition works" table. Also smoke-tested the built binary against the live GitHub API — explicit range, bare sweep (shll first), and roster ordering confirmed.)

## Execution Order

- Phase 1 (T001–T006) before Phase 2 tests and before any cmd code that imports the package.
- T014 (probeResult before-version) before T015 (after-version) before T016 (digest).
- T010–T012 before T013 (factory needs `runChangelog`).
- T016 depends on Phase 1 (`changelog.FetchRange`).
- Phase 5 tests after their Phase 3/4 implementation.
- T020 last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `net/http` is imported only in `src/internal/changelog/`; `src/cmd/shll/` has zero `net/http` imports (grep-verifiable).
- [x] A-002 R2: `fetchReleases` GETs `{baseURL}/repos/sahil87/{repo}/releases?per_page=100`, unauthenticated, with a 10s timeout; base URL and timeout are named constants.
- [x] A-003 R3: version normalization strips `v` prefix and `_N` suffix; numeric dot-component compare selects `(old, new]` newest-first.
- [x] A-004 R4: any fetch failure (non-200/403, transport, timeout, JSON parse) yields a typed unavailable result, distinguishable from "zero releases in range".
- [x] A-005 R5: multi-tool fetches run concurrently but results render in roster order.
- [x] A-006 R6: `shll changelog` is a registered cobra subcommand delegating to a `runChangelog(ctx, stdout, stderr, args)` test seam.
- [x] A-007 R7: `tool@old..new` and bare `tool` specs parse (with/without `v` prefix); unknown names hard-error listing valid targets with no side effect.
- [x] A-008 R8: no-range forms resolve installed→latest; up-to-date prints a notice + releases URL; named-not-installed errors for no-range only; explicit range works uninstalled.
- [x] A-009 R9: per-tool full output is header + tag/title/body newest-first, in roster order regardless of arg order, shll first when included.
- [x] A-010 R10: output caps at 10 releases with a cap notice + compare URL on overflow; explicit `old==new`/empty range prints "no releases in range".
- [x] A-011 R11: a changelog fetch failure degrades that tool to a compare-URL line and does not change the exit code.
- [x] A-012 R12: before-versions are captured from the existing `brew list --versions` captured read (second field); no streamed-output parsing.
- [x] A-013 R13: after-versions are re-queried via `brew list --versions` only after a successful upgrade.
- [x] A-014 R14: the `What changed:` digest prints per-tool transition + title-only release lines + a `Full notes: shll changelog ...` command, in roster order.
- [x] A-015 R15: no digest when nothing bumped or under `--dry-run`; subset runs name only bumped members; unavailable degrades to compare URL; digest never changes the exit code.
- [x] A-016 R16: `changelog` is wired in `root.go` `AddCommand` and listed in `rootLong`.

### Behavioral Correctness

- [x] A-017 R15: existing `shll update` golden strings (headers/tail/empty-case/dry-run) are byte-for-byte unchanged when no version changes (the fake returns the same version before and after).
- [x] A-018 R8: `shll changelog` and `shll update`'s digest degrade gracefully (Constitution V) — a missing tool / rate-limited API never crashes or non-zero-exits the changelog surface.

### Scenario Coverage

- [x] A-019 R2 R4: `changelog_test.go` exercises real `net/http` paths against an `httptest.Server` (200, 403, malformed JSON, path/header assertions).
- [x] A-020 R14: `update_test.go` covers the bump→digest path and the no-bump→no-digest path.

### Edge Cases & Error Handling

- [x] A-021 R10: `old == new` and zero-in-range produce "no releases in range", not an error.
- [x] A-022 R11 R15: 403 rate-limit / timeout produce compare-URL fallbacks in both `changelog` and `update` digest, exit code unaffected.

### Code Quality

- [x] A-023 Pattern consistency: new code follows the surrounding cmd/shll patterns — `newXxxCmd()` factory + `runXxx` writer-seam, named constants (no magic strings), one file per subcommand with a paired `_test.go`.
- [x] A-024 No unnecessary duplication: the digest and `changelog` command share the `internal/changelog` fetch/filter/degradation code; `resolveTargets` reused for name validation; `ui.go` framing reused rather than reimplemented.
- [x] A-025 Subprocess isolation: every subprocess call routes through `internal/proc`; `net/http` is isolated in `internal/changelog` and never imported by cmd code (Constitution I).
- [x] A-026 No regex over brew output: before/after version capture uses field-splitting on the `brew list --versions` line, not a regex (code-quality anti-pattern).
- [x] A-027 Stateless: no caching/persistence introduced (Constitution II); every invocation re-derives from brew + the GitHub API.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

Both prior candidates (cycle-1 rework subjects) are now RESOLVED — the redundant brew invocations have been collapsed, so they are no longer candidates:

- ~~`src/cmd/shll/brew.go` (`isInstalled` duplicate `brew list` read)~~ — DONE. `isInstalled` (brew.go:130) is now the one-line wrapper `installed, _ := probeInstalledVersion(ctx, formula); return installed`; the sole `brew list --formula --versions` invocation lives in `probeInstalledVersion` (brew.go:151). The symbol legitimately stays (boolean-only consumers: `install.go:139`, `update.go:159`, `changelog.go` via `installedVersion`).
- ~~`src/cmd/shll/update.go` (shll-self `isInstalled` + `installedVersion` two-read pair)~~ — DONE. runUpdate now does one `probeInstalledVersion(ctx, shllFormula)` (update.go:158) yielding both the install fact and `beforeShll`; the former separate pre-upgrade `installedVersion` read is gone. (The post-upgrade `installedVersion(ctx, shllFormula)` at update.go:325 is a NECESSARY second read — the after-version — not a redundant one.)

No new deletion candidates. The change adds new functionality (`shll changelog`, the update digest, the `internal/changelog` package); it does not render any additional existing code redundant. `SetTransportForTest`, `FetchRange`, `FetchAll`, `LatestTag`, `ReleasesInRange`, `CompareVer`, `NormalizeVer`, `CompareURL`, and `ReleasesURL` each have live production or cross-package-test call sites (verified) — no dead exports.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Package name kept as `internal/changelog` (not renamed) | Matches the Affected-Memory `(new)` anchor and the memory domain naming; intake gave the plan latitude but the boundary is the load-bearing part | S:70 R:80 A:80 D:75 |
| 2 | Confident | HTTP test seam = package-level `baseURL` var (default `https://api.github.com`) + swappable `httpClient`, tests point at an `httptest.Server` | Mirrors the established `proc.Runner` package-var seam; exercises real net/http paths (status/decode/timeout) locally; intake said "injectable transport or base URL" | S:70 R:75 A:80 D:70 |
| 3 | Confident | Before-version = second whitespace field of `brew list --formula --versions` stdout (`<leaf> <version>`) | The output shape is fixed; it is the captured read the probe already runs; existing test fake already returns `<formula> 1.0.0`, so tests keep before==after and preserve goldens | S:75 R:80 A:80 D:80 |
| 4 | Confident | `Result` type exposes `Unavailable bool` (plus wrapped error detail) so callers distinguish "unavailable" from "zero releases in range" | R4 requires the distinction; a bool flag on the result is the simplest carrier and keeps degradation branch-free at call sites | S:65 R:80 A:80 D:70 |
| 5 | Confident | Digest reuses `changelog.FetchRange` rather than a second fetch path | Single source of truth for range/degradation; the two surfaces differ only in rendering (titles vs. full bodies) | S:70 R:80 A:85 D:80 |
| 6 | Confident | `changelog` full output uses the existing `ui.go` `colorEnabled`/`printToolHeader` framing rather than a bespoke framer | Intake says framing follows `ui.go` conventions; reuse avoids a parallel color/TTY path | S:65 R:80 A:80 D:70 |
| 7 | Confident | Compare/releases URLs are helper functions in `internal/changelog` (`CompareURL`/`ReleasesURL`) built from `githubOrgBase`-style constants | Both the command and the digest need them; centralizing avoids the `rk`/`run-kit` slug footgun being re-open-coded | S:70 R:85 A:80 D:80 |

7 assumptions (0 certain, 7 confident, 0 tentative).
