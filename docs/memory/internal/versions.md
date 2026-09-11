---
type: memory
description: "`internal/versions` — shll's \"latest version per tool\" resolver seam behind `shll check-updates` (both backends) and `shll changelog`'s GitHub anchor: owns the versions-manifest fetch over an ordered URL list (hexokit.com primary, shll.ai fallback; schema-1 decode, typed `ErrUnavailable`), the notify-threshold `Notable` policy mapping, and a thin `LatestGitHub` delegation to `internal/changelog.LatestTag`; package-level URL-list/client test seams + `SetTransportForTest`."
---
# internal/versions

The "latest version per tool" resolver seam — the single surface behind `shll check-updates` (both backends) and the GitHub anchor of `shll changelog`'s no-range resolution. It owns the versions-manifest fetch (hexokit.com, with shll.ai as the fallback host) and the notify-threshold (`notable`) computation, and delegates the GitHub backend to [internal/changelog](/internal/changelog.md)'s existing fetch — no duplicated GitHub code.

Together with `internal/changelog` it keeps `net/http` isolated in internal packages (Constitution I spirit): command code in `src/cmd/shll` never talks to `net/http` directly. It holds no state (Constitution II): every call re-fetches. **Future `versions.json` schema evolution is absorbed here**, so consumers (run-kit exec'ing `shll check-updates --json`, `shll changelog`) never compile in manifest or version-comparison policy.

Source: `src/internal/versions/versions.go`, tests in `src/internal/versions/versions_test.go`. (puxw)

## Overview

The package (a) GETs the versions manifest — `https://hexokit.com/versions.json`, falling back to `https://shll.ai/versions.json` only when that attempt is unavailable — and decodes schema 1, degrading failure on every URL to a typed `ErrUnavailable`; (b) maps a tool's notify policy over a pending version bump to decide whether it is `notable`; and (c) delegates the GitHub-releases backend to `internal/changelog.LatestTag`, preserving that package's single-fetch contract.

## Constants and the test seams

Every magic value is a named constant (code-quality.md):

| Constant | Value | Role |
|----------|-------|------|
| `manifestURLDefault` | `https://hexokit.com/versions.json` | production manifest URL — the roster + policy authority for `--released`; first entry of the `manifestURLs` seam |
| `manifestURLFallback` | `https://shll.ai/versions.json` | the previous manifest host, tried only when the primary attempt is unavailable (shll.ai serves a byte copy through the site cutover); second entry of `manifestURLs` |
| `manifestSchema` | `1` | the `versions.json` schema this binary understands; any other value is treated as unavailable |
| `requestTimeout` | `10 * time.Second` | per-request `context.WithTimeout` bound (mirrors `internal/changelog`'s per-request timeout) |
| `NotifyNever` / `NotifyPatch` / `NotifyMinor` | `never` / `patch` / `minor` | the manifest's per-tool `notify` policy values (exported — consumed by `Notable`) |

**Test seams — package-level vars, mirroring `internal/changelog` and `proc.Runner`.** `var manifestURLs = []string{manifestURLDefault, manifestURLFallback}` (the ordered URL list `FetchManifest` walks) and `var httpClient = &http.Client{}` (the client; requests carry their own context timeout, so the client needs no `Timeout` field) are the injection points. The exported `SetTransportForTest(url string, client *http.Client) (restore func())` collapses the list to the single test URL, swaps the client, and returns a restore closure — the one cross-package entry (used by `cmd/shll`'s `check_updates_test.go`), driving the **real** `net/http` code paths against an `httptest.Server` without network. Fallback ordering is exercised by this package's own tests, which assign a two-server list to `manifestURLs` directly (`manifestServers` helper). Not for production use.

## API surface

| Symbol | Contract |
|--------|----------|
| `ManifestTool{Latest, Notify, Formula}` | one tool's manifest entry (only the fields shll consumes decoded) |
| `Manifest{Schema, GeneratedAt, Tools}` | the decoded `versions.json`; `Tools` is keyed by tool **name** (carries shll itself plus every roster tool) |
| `FetchManifest(ctx) (Manifest, error)` | walks `manifestURLs` in order — one bounded GET + decode per URL (`fetchManifestFrom`) — returning the first intact manifest; wraps `ErrUnavailable` when every URL fails (see § Degradation) |
| `Notable(notify, installed, latest) bool` | the notify-threshold policy mapping (see § Notify threshold) |
| `LatestGitHub(ctx, repo) (latest string, rels []changelog.Release, err error)` | thin delegation to `changelog.LatestTag` (see § GitHub delegation) |
| `ErrUnavailable` | sentinel wrapped by every manifest-fetch failure |

## Requirements

### Requirement: Manifest fetch + degradation (Constitution II, V)
`FetchManifest` SHALL try each entry of `manifestURLs` in order (`manifestURLDefault` on hexokit.com, then `manifestURLFallback` on shll.ai), performing one HTTP GET per URL with its own `requestTimeout`-bounded context, and MUST NOT cache. An attempt fails on a transport error, timeout, non-200 status, body-read error, JSON decode failure, or a `Schema` other than `manifestSchema`; a failed attempt moves to the next URL, and the first intact manifest is returned immediately, so a reachable primary never contacts the fallback. When every URL fails, the last attempt's error MUST wrap `ErrUnavailable`. On a non-200 the body is intentionally not read — the status code alone is the degradation signal (the deferred `Body.Close()` still releases the connection). There are no retries within a URL.

For the `--released` backend there is exactly one manifest fetch (one or two GETs), so unavailability on every URL fails the whole check (unlike the per-tool GitHub degradation) — the caller writes a diagnostic and exits 1.

#### Scenario: schema-1 manifest decodes
- **GIVEN** an httptest server serving a schema-1 manifest
- **WHEN** `versions.FetchManifest(ctx)` runs against the swapped seam
- **THEN** it returns the decoded `Manifest` with its `Tools` map populated

#### Scenario: primary failure falls through to the fallback
- **GIVEN** the primary URL answers a non-200, malformed JSON, `schema != 1`, or refuses the connection, and the fallback URL serves a schema-1 manifest
- **WHEN** `FetchManifest` runs
- **THEN** it returns the fallback's `Manifest` with no error

#### Scenario: a reachable primary never contacts the fallback
- **GIVEN** the primary URL serves a schema-1 manifest
- **WHEN** `FetchManifest` runs
- **THEN** the fallback server receives zero requests

#### Scenario: unavailable causes wrap the sentinel
- **GIVEN** every URL returns a non-200 status, malformed JSON, or `schema != 1`
- **WHEN** `FetchManifest` runs
- **THEN** it returns an error satisfying `errors.Is(err, ErrUnavailable)`

### Requirement: Notify threshold (`Notable`)
`Notable(notify, installed, latest)` SHALL report whether the pending installed→latest bump crosses the tool's notify threshold. It MUST return `false` when no update is pending (`installed >= latest` by `changelog.CompareVer`, or either side empty). Otherwise: `NotifyNever` → false; `NotifyPatch` → true; `NotifyMinor` → true iff a minor-or-higher component increases (a patch-only bump is not notable); a major bump crosses every non-`never` threshold. Any other `notify` value (unknown/future, empty) MUST be treated as `NotifyMinor`. Bump classification delegates to `changelog.FirstDiffComponent` (index 0 = major, 1 = minor, ≥2 = patch), so the `minor`/unknown branch is `idx >= 0 && idx <= 1`.

#### Scenario: minor policy ignores a patch bump
- **GIVEN** `notify: minor` and a 3.8.1 → 3.8.2 bump
- **WHEN** `Notable("minor", "3.8.1", "3.8.2")` runs
- **THEN** it returns `false` (patch-only bump does not cross the minor threshold)
- **AND** `Notable("patch", "3.8.1", "3.8.2")` returns `true`

### Requirement: GitHub delegation preserves the single-fetch contract
`LatestGitHub(ctx, repo)` SHALL be a thin delegation to `changelog.LatestTag`, returning the fetched release list too so a caller that also needs the releases (the no-range `shll changelog` anchor) never fetches twice. Failures degrade to `changelog.ErrUnavailable` per that package's contract; the `check-updates` `--github` backend degrades them per-tool (row omitted), never failing the run (Constitution V).

#### Scenario: no second fetch for the changelog anchor
- **GIVEN** `shll changelog`'s no-range resolution consuming `LatestGitHub`
- **WHEN** a tool resolves installed → latest
- **THEN** exactly one GitHub GET occurs per repo (`TestChangelog_NoRangeSingleFetchPerRepo` stays green)

## Design Decisions

### Ordered URL list with sequential fallback, not a single URL or a race
**Decision**: The manifest host is an ordered package-level list `manifestURLs = {manifestURLDefault (hexokit.com), manifestURLFallback (shll.ai)}`; `FetchManifest` loops over it, returning the first intact manifest and wrapping `ErrUnavailable` only after every URL fails. The exported `SetTransportForTest` collapses the list to one URL so its signature and every existing consumer are unchanged.
**Why**: The toolkit site moves from shll.ai to hexokit.com in phases; shll.ai keeps serving a byte copy of the manifest through the cutover, so a sequential fallback keeps every shipped binary reading a live manifest whichever host is up, with zero extra requests on the happy path and the single-degradation `ErrUnavailable` contract intact.
**Rejected**: Racing both hosts in parallel (an extra GET on every run and cancellation complexity for no user-visible gain); an env or config override for the host (the `config-home` standard bans env preference channels and shll has no config file); leaving shll.ai as the sole URL (every shipped binary would depend forever on the redirect host's byte-copy endpoint).
*Introduced by*: `260911-ttoa-hexokit-banner-and-policy`.

### Version parsing stays in `internal/changelog`; `versions` owns only policy
**Decision**: `Notable` maps policy over `changelog.CompareVer` + `changelog.FirstDiffComponent` rather than re-implementing version-component parsing; `LatestGitHub` delegates to `changelog.LatestTag` rather than fetching GitHub itself.
**Why**: version-component parsing (`verComponent`, the normalize rules) and the GitHub fetch already live in `internal/changelog`; duplicating either in `versions` is a code-quality anti-pattern and a drift risk. `versions` owns only the manifest fetch, the schema contract, and the notify-policy mapping.
**Rejected**: re-implementing component parsing in `versions` (drift risk); exporting the raw `verComponent` (weaker — the caller would have to split/normalize itself). `FirstDiffComponent` is the right-altitude export.
*Introduced by*: 260720-puxw-check-updates-command

### Unsupported schema fails loudly, not best-effort
**Decision**: a `Schema != manifestSchema` manifest wraps `ErrUnavailable` rather than best-effort decoding the tools it recognizes.
**Why**: shll is the single surface that absorbs schema evolution, so failing loudly on an unknown schema beats silently misreading policy; run-kit's skip-on-nonzero contract tolerates the failure. A schema bump ships with a shll release that raises the constant.
**Rejected**: best-effort partial decode — risks silently applying the wrong notify policy under a changed schema.
*Introduced by*: 260720-puxw-check-updates-command

## Cross-references

- The GitHub-releases fetch layer this package delegates to, and the `FirstDiffComponent` / `CompareVer` / `NormalizeVer` / `LatestTag` primitives it consumes: [internal/changelog](/internal/changelog.md).
- The command surface consuming this resolver (both backends + the `--json` contract): [cli/check-updates](/cli/check-updates.md).
- The changelog command whose no-range anchor also consumes `LatestGitHub`: [cli/changelog](/cli/changelog.md#the-bare-sweep-no-args).
- Subprocess-isolation sibling (the pattern this mirrors): [internal/proc](/internal/proc.md).
- Constitution I (net/http isolated in an internal package), II (stateless — every call re-fetches), V (fetch failure degrades — per-tool for `--github`, whole-check for `--released`).
