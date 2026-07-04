---
type: memory
description: "`internal/changelog` — shll's isolated stdlib-`net/http` GitHub-releases fetch layer: fetch + version normalize/compare + `(old, new]` range filter, concurrent `FetchAll`, and the typed `Unavailable` degradation Result."
---
# internal/changelog

The GitHub-releases fetch layer for `shll changelog` and `shll update`'s "What changed:" digest. It is the **only** package in the repo that imports `net/http` — command code in `src/cmd/shll` never talks to `net/http` directly, mirroring how `internal/proc` isolates `os/exec` (Constitution I spirit). It holds no state (Constitution II): every call re-fetches.

Source: `src/internal/changelog/changelog.go`, tests in `src/internal/changelog/changelog_test.go`.

## Overview

The package (a) fetches a repo's GitHub Releases (unauthenticated, stdlib `net/http` only — no new module dependency), (b) normalizes versions and filters releases to a half-open `(old, new]` range newest-first, and (c) degrades **any** failure to a typed `Unavailable` `Result` so callers render a compare-URL fallback and keep going (Constitution V). Introduced by change r01z.

## Endpoint, constants, and the test seams

The fetch targets `GET {baseURL}/repos/sahil87/{repo}/releases?per_page=100`, unauthenticated (no `Authorization` header; sets `Accept: application/vnd.github+json`), with a per-request context timeout. Every magic value is a named constant (code-quality.md — no magic strings):

| Constant | Value | Role |
|----------|-------|------|
| `apiBaseDefault` | `https://api.github.com` | production API host; assigned to the `baseURL` seam |
| `requestTimeout` | `10 * time.Second` | per-fetch `context.WithTimeout` bound (generous for one unauthenticated call, keeps a stall from hanging a run) |
| `perPage` | `100` | max page size in one shot — the toolkit's repos have far fewer than 100 releases, so a single page always covers any real range (no pagination in v1) |
| `owner` | `sahil87` | **single source of truth** for the GitHub owner segment (see below) |
| `githubOrgBase` | `https://github.com/` + `owner` + `/` | browser-facing URL base (compare/releases links) — NOT the API host |
| `orgRepoPrefix` | `owner` + `/` | the owner segment of every API repo path (`repos/{owner}/{repo}`) |

**Owner single-sourcing (r01z rework).** The `owner = "sahil87"` constant is the one place the owner is encoded; both `githubOrgBase` (browser URLs) and `orgRepoPrefix` (API path) derive from it, so the owner is never re-encoded and cannot drift. It deliberately *mirrors* `cmd/shll`'s `githubOrgBase` value rather than importing across the package boundary (this package MUST NOT depend on `cmd/shll`).

**Test seams — package-level vars, mirroring `proc.Runner`.** Two package-level vars are the injection points: `var baseURL = apiBaseDefault` (API host) and `var httpClient = &http.Client{}` (the client; requests carry their own context timeout, so the client needs no `Timeout` field). Tests swap them to point at an `httptest.Server`, exercising the **real** `net/http` code paths (status codes, JSON decode, timeout) without network. The exported `SetTransportForTest(base string, client *http.Client) (restore func())` is the one cross-package entry (used by `cmd/shll`'s `changelog_test.go`/`update_test.go`) — it swaps both seams and returns a restore closure; not for production use.

## API surface

| Symbol | Contract |
|--------|----------|
| `Release{Tag, Title, Body}` | one release decoded from the JSON (`tag_name`→`Tag`, `name`→`Title`, `body`→`Body`); only rendered fields decoded, rest of the large payload ignored |
| `RangeReq{Tool, Repo, Old, New}` | names one tool's fetch — tool name (for result labelling), repo slug (**not** always the name — rk's is `run-kit`), and the `(old, new]` bounds |
| `Result{Tool, Repo, Old, New, Releases, Unavailable, Err}` | the fetch+filter outcome (see § Degradation) |
| `FetchRange(ctx, RangeReq) Result` | fetch repo's releases + filter to `(old, new]`; folds any failure into `Unavailable=true, Err=…` — never returns an error |
| `FetchAll(ctx, []RangeReq) []Result` | concurrent multi-tool fetch, order-preserving (see § Concurrency) |
| `LatestTag(ctx, repo) (string, []Release, error)` | newest tag (by normalized-version order) **and** the fetched release list, degrading to `ErrUnavailable` on failure (see § LatestTag single-fetch) |
| `ReleasesInRange(rels, old, new) []Release` | filter an already-fetched list to `(old, new]` newest-first, WITHOUT a network call (see § LatestTag single-fetch) |
| `CompareVer(a, b) int` | exported numeric version compare (−1/0/+1); lets `cmd` decide "installed ≥ latest" without re-implementing the rules |
| `NormalizeVer(s) string` | exported normalize (strip `v` + `_N`); lets `cmd` render ONE normalized form on both sides of a transition |
| `CompareURL(repo, old, new) string` | browser "Full Changelog" compare link `github.com/sahil87/{repo}/compare/v{old}...v{new}` (tags always v-prefixed via `vTag`) |
| `ReleasesURL(repo) string` | browser releases page `github.com/sahil87/{repo}/releases` (up-to-date notice / fallback anchor) |

`CompareURL`/`ReleasesURL` are single-sourced here so neither `shll changelog` nor `shll update`'s digest re-open-codes the `rk`/`run-kit` slug footgun.

## Version normalization + compare

`normalizeVer(s)` trims whitespace, strips a leading `v`, and strips a brew revision suffix (`_N`, everything from the first `_`) — so a tag `v0.6.4` and a brew version `0.6.4_1` share one form. It leaves any pre-release/build suffix on the numeric core untouched (the compare only reads numeric components).

`compareVer(a, b)` splits both (post-normalize) on `.` and compares numeric dot-components, **missing trailing components treated as `0`** (so `0.6` == `0.6.0`). `verComponent` reads only the leading integer run of each component (handles `4`, `4-rc1`, `4+build`); a non-numeric or absent component is `0` (best-effort — the toolkit tags are plain numeric SemVer). Returns −1/0/+1. Covered by `TestNormalizeVer` and `TestCompareVer` (incl. differing component counts).

## Range filter — `(old, new]`, newest first

`releasesInRange(rels, old, new)` (exported as `ReleasesInRange`) selects releases whose tag is **`> old` AND `≤ new`** by `compareVer`, then `sort.SliceStable` descending by normalized version. A release equal to `old` is **excluded**; one equal to `new` is **included**; `old == new` yields an empty slice. Pinned by `TestReleasesInRange_HalfOpenNewestFirst`.

## LatestTag single-fetch contract

`LatestTag(ctx, repo)` returns `(newestTag, allReleases, err)` — it fetches **once** and returns the release list too, so a caller resolving a no-range `shll changelog tool` (installed → latest) does NOT re-fetch: it takes `latest`, then range-filters the returned `[]Release` locally via `ReleasesInRange`. This is the "one GET per repo, not two" contract (the r01z rework that eliminated the no-range double-fetch). An empty release set returns `("", nil, nil)` — a successful fetch with no releases, not an error. Covered by `TestLatestTag`; the single-GET guarantee is asserted cross-package by `TestChangelog_NoRangeSingleFetchPerRepo`.

## Degradation contract (Constitution V)

`fetchReleases` is the single degradation point: it returns `ErrUnavailable` (wrapped with detail via `fmt.Errorf("%w: …")`) on a transport error, timeout, **any** non-200 status (403 rate-limit, 404, 5xx alike), a body-read error, or a JSON decode failure. On a non-200 the body is **intentionally not read** — the status code alone is the degradation signal (the deferred `resp.Body.Close()` still releases the connection; the r01z rework corrected a stale "drain" comment here). **No retries in v1.**

`FetchRange` folds `ErrUnavailable` into `Result{Unavailable: true, Err: …}` and never returns an error, so call sites branch on the `bool` without importing the sentinel. Callers match the raw sentinel via `errors.Is(err, ErrUnavailable)` on the `LatestTag`/`fetchReleases` paths.

**The load-bearing distinction**: `Unavailable == true` ("fetch failed → render a compare-URL fallback") is distinct from a successful fetch with an empty `Releases` slice ("zero releases in range" → the "no releases in range" line). Pinned by `TestFetchRange_UnavailableVsEmpty`; the HTTP paths (200/403/malformed JSON/path+header assertions) by `TestFetchReleases_HappyPath`, `TestFetchReleases_RateLimitedIsUnavailable`, `TestFetchReleases_MalformedJSONIsUnavailable`.

## Concurrency + order preservation

`FetchAll(ctx, reqs)` runs one goroutine per request (read-only HTTP — the same carve-out as `update.go`'s `probeRoster`; no shared brew lock) and writes each `Result` into a fixed-size slice **indexed by request position**, not appended on completion. So the returned order equals the caller's `reqs` order regardless of which fetch finished first — the caller assembles `reqs` in roster order and output stays roster-ordered. Pinned by `TestFetchAll_PreservesRequestOrder`.

## The net/http isolation guard

The Constitution-I-style boundary — `net/http` confined to this package exactly as `os/exec` is confined to `internal/proc` — is enforced by a test that lives in `cmd/shll`: `TestCmdShllNoNetHTTP` (`src/cmd/shll/changelog_test.go`) reads every non-`_test.go` file in `cmd/shll` and fails if any imports `"net/http"` (it also re-asserts the existing `"os/exec"` ban). Command code calls this package's exported API instead. (Test files legitimately import `net/http/httptest` to drive the changelog server.) The guard is a *test*, not a comment, because a comment cannot fail CI.

## Cross-references

- Command surface consuming this package: [cli/changelog](/cli/changelog.md) (full output) and [cli/update](/cli/update.md#version-capture--the-what-changed-digest-change-r01z) (the "What changed:" digest) — both surfaces share one release rendering (`renderReleases`, change 13k3); the digest adds a tool-name-bearing transition line above the shared release blocks.
- Subprocess isolation sibling (the pattern this mirrors): [internal/proc](/internal/proc.md).
- The `rk`/`run-kit` repo-slug footgun that `RangeReq.Repo` / `CompareURL` avoid re-open-coding: [cli/commands §hardcoded tool roster](/cli/commands.md#hardcoded-tool-roster).
- Constitution I (net/http isolated in an internal package), II (stateless — every call re-fetches, no caching), V (any fetch failure degrades to a typed `Unavailable` Result).
