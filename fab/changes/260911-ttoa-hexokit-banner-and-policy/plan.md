# Plan: HexoKit banner and policy (D14 first pass)

**Change**: 260911-ttoa-hexokit-banner-and-policy
**Intake**: `intake.md`

## Requirements

### Standards content: brand surfaces (D14 first pass)

#### R1: The mandated README blockquote names HexoKit
`docs/site/standards/readme-extraction.md` rule 1's fenced example — "this exact line in all seven repos" — MUST read `> Part of [HexoKit](https://hexokit.com) — see all projects there.` (em-dash and trailing period kept, the article "the" dropped). shll's own `README.md` line 3 MUST carry the same line, because the constitution's Toolkit Standards clause binds this repo to the standards it publishes. No other line of either file changes.

- **GIVEN** the readme-extraction standard's rule 1 fenced example
- **WHEN** a companion repo (C7) or run-kit (C3) copies the mandated line
- **THEN** the line it copies is `> Part of [HexoKit](https://hexokit.com) — see all projects there.`
- **AND** shll's `README.md` H1 → blockquote → badges order is intact with the new blockquote

#### R2: Policy B's centralized install-docs location is hexokit.com
`docs/site/standards/install-composition.md` MUST state hexokit.com as Policy B's location in all four location sentences (the two-halves summary, the scope carve-out, the Policy B MUST NOT bullet's link, and the Verifying-conformance bullet), and the `shll standards` roster Description for `install-composition` in `src/cmd/shll/standards.go` MUST say "install docs centralized on hexokit.com". The intro sentence's "[shll toolkit](https://shll.ai)" and "[`shll install`](https://shll.ai)" phrases MUST stay unchanged (X4's scope).

- **GIVEN** the install-composition standard
- **WHEN** `grep -n 'shll\.ai' docs/site/standards/install-composition.md` runs
- **THEN** the only matches are on the intro line (line 3)
- **AND** `shll standards` lists `install-composition` with "install docs centralized on hexokit.com"

#### R3: config-home's example path is `~/.config/hexokit`
`docs/site/standards/config-home.md` § Conformance's run-kit row MUST give the example path as `$HOME/.config/hexokit/config.yaml`. The line-16 tool-name example (`run-kit`, not `rk`) and the `RK_*` prose MUST stay unchanged.

- **GIVEN** the config-home standard
- **WHEN** C4 implements D9's home migration and checks the standard
- **THEN** the standard's example path is `$HOME/.config/hexokit/config.yaml`
- **AND** `config/run-kit` no longer appears anywhere in `docs/site/standards/`

### Binary: versions manifest fetch

#### R4: Manifest fetch tries hexokit.com first and falls back to shll.ai
`internal/versions` MUST fetch the manifest from an ordered URL list — `https://hexokit.com/versions.json` (primary) then `https://shll.ai/versions.json` (fallback). `FetchManifest` SHALL attempt each URL in order with its own `requestTimeout`-bounded context and MUST move to the next URL on any failure class it degrades today (transport error, timeout, non-200, decode failure, unsupported schema). It MUST return the first manifest whose `schema` equals `manifestSchema` without contacting later URLs, and when every URL fails it MUST return an error wrapping `ErrUnavailable`. No retries within a URL, no caching. `SetTransportForTest(url, client)` MUST keep its exported signature and collapse the list to `[url]`, restoring the previous list on the returned func.

- **GIVEN** the primary URL answers 500 and the fallback answers 200 with a schema-1 manifest
- **WHEN** `FetchManifest` runs
- **THEN** it returns the fallback's manifest with no error

- **GIVEN** the primary URL answers 200 with a schema-1 manifest
- **WHEN** `FetchManifest` runs
- **THEN** the fallback server receives zero requests

- **GIVEN** both URLs fail (primary connection refused, fallback 500)
- **WHEN** `FetchManifest` runs
- **THEN** the error satisfies `errors.Is(err, ErrUnavailable)`

- **GIVEN** a test calls `SetTransportForTest(srv.URL, srv.Client())`
- **WHEN** `FetchManifest` runs
- **THEN** exactly that one URL is attempted (existing tests pass unchanged)

#### R5: Surfaces that describe the manifest host say hexokit.com
The `check-updates` long help line (`--source released   latest versions + notify policy from …`) MUST name `https://hexokit.com/versions.json` and mention the shll.ai fallback; the `sourceFlagUsage` string MUST say "hexokit.com versions manifest"; `versions.go`'s package and constant comments MUST describe the hexokit.com manifest and the fallback; `docs/site/skill.md` MUST say "[HexoKit toolkit](https://hexokit.com/toolkit/)" in its intro and "hexokit.com versions manifest" in its check-updates line.

- **GIVEN** `shll check-updates --help`
- **WHEN** a user reads the `--source released` line
- **THEN** it names the host the binary fetches first (hexokit.com) and the fallback

- **GIVEN** `shll skill shll`
- **WHEN** an agent reads the intro line
- **THEN** it links the HexoKit toolkit page at `https://hexokit.com/toolkit/`

### Repo: embedded copies

#### R6: Embedded standards and skill bundle match canonical
After the canonical edits, `src/cmd/shll/standards/{readme-extraction,install-composition,config-home}.md` and `src/cmd/shll/skill/skill.md` MUST be byte-identical to their `docs/site/` sources (regenerated via `just sync-standards`), so `TestStandardsEmbedMatchesCanonical` and `TestSkillEmbedMatchesCanonical` pass.

- **GIVEN** the canonical files are edited
- **WHEN** `cd src && go test ./cmd/shll/` runs
- **THEN** both drift-guard tests pass

### Non-Goals

- Renaming any standard, file, or the `shll standards` command; roster `Name`/`Formula`/`Repo`/`LegacyName` (→ R1/R2 of the plan doc).
- Every other `shll.ai` / "shll toolkit" mention: standards intros, consuming-site sentences, `docs/site/install.md`, README body, `fab/project/*`, `agent_setup.go`'s placed-skill prose (→ X4).
- The `hexokit.com/install` script default (→ S5) and the manifest's `hexokit` row (→ R1).

### Design Decisions

#### Ordered URL list for the manifest fetch
**Decision**: `manifestURLs = []string{manifestURLDefault, manifestURLFallback}` with a sequential attempt loop inside `FetchManifest`; the exported `SetTransportForTest` collapses the list to a single URL.
**Why**: Preserves the single-degradation `ErrUnavailable` contract and the exported test seam the check-updates tests already use; the happy path costs no extra request; the fallback covers the shll.ai → hexokit.com cutover window without any caller change.
**Rejected**: Parallel racing of both hosts (extra request on every happy-path run, more complex cancellation for no user-visible gain); a config/env override for the URL (config-home standard forbids env preference channels, and shll has no config file).
*Introduced by*: 260911-ttoa-hexokit-banner-and-policy

#### shll's own README flips with the standard
**Decision**: `README.md` line 3 adopts the new mandated blockquote in the same change.
**Why**: The constitution's Toolkit Standards clause says this repo MUST itself conform; shipping a standard the publisher violates on merge is the worst-case non-conformance.
**Rejected**: Deferring to X4 (leaves shll non-conformant with its own rule 1 for the whole Phase 1/2 window).
*Introduced by*: 260911-ttoa-hexokit-banner-and-policy

## Tasks

### Phase 1: Canonical content

- [x] T001 Edit the three standards and the README: `docs/site/standards/readme-extraction.md` (rule 1 fenced blockquote), `docs/site/standards/install-composition.md` (lines 5, 7, 31, 54 → hexokit.com), `docs/site/standards/config-home.md` (line 54 path → `$HOME/.config/hexokit/config.yaml`), `README.md` (line 3 blockquote). Touch no other line. <!-- R1, R2, R3 -->

### Phase 2: Binary

- [x] T002 `src/internal/versions/versions.go`: rename the URL constant to hexokit.com, add `manifestURLFallback`, replace `var manifestURL` with `var manifestURLs []string`, loop in `FetchManifest` (extract the single-URL attempt into a helper so the function stays focused), make `SetTransportForTest` collapse/restore the list; update the package/const doc comments. <!-- R4, R5 -->
- [x] T003 `src/internal/versions/versions_test.go`: add fallback tests — primary 500 → fallback 200 returns the manifest; primary 200 → fallback receives zero requests; primary connection-refused + fallback 500 → `ErrUnavailable`. Run `cd src && go test ./internal/versions/`. <!-- R4 -->
- [x] T004 [P] Descriptive surfaces: `src/cmd/shll/check_updates.go` (`sourceFlagUsage` and the `--source released` long-help line), `src/cmd/shll/standards.go` (install-composition Description → "centralized on hexokit.com"), `docs/site/skill.md` (line 3 intro link, line 22 manifest host). <!-- R2, R5 -->

### Phase 3: Sync & verify

- [x] T005 Run `just sync-standards`; then `cd src && go test ./internal/versions/ ./cmd/shll/`, then `go test ./...`; confirm `grep -rn 'shll\.ai' docs/site/standards/install-composition.md` matches only line 3 and `grep -rn 'config/run-kit' docs/site/standards/` is empty. <!-- R6 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `docs/site/standards/readme-extraction.md` rule 1's fenced example is exactly `> Part of [HexoKit](https://hexokit.com) — see all projects there.`
- [x] A-002 R1: `README.md` line 3 carries the same blockquote; H1 → blockquote → badges order is intact
- [x] A-003 R2: `install-composition.md` lines 5, 7, 31, 54 say hexokit.com; line 3's two shll.ai phrases are unchanged; no other shll.ai match remains in the file
- [x] A-004 R2: `standards.go` install-composition Description ends "install docs centralized on hexokit.com"
- [x] A-005 R3: `config-home.md` conformance row's example path is `$HOME/.config/hexokit/config.yaml`; line 16 and the `RK_*` prose are unchanged
- [x] A-006 R4: `versions.go` has `manifestURLDefault = "https://hexokit.com/versions.json"`, `manifestURLFallback = "https://shll.ai/versions.json"`, and an ordered `manifestURLs` slice consumed by `FetchManifest`
- [x] A-007 R5: `check_updates.go` `sourceFlagUsage` and the long-help `--source released` line name hexokit.com (long help mentions the fallback); `versions.go` comments no longer describe a shll.ai-primary fetch
- [x] A-008 R5: `docs/site/skill.md` intro links `[HexoKit toolkit](https://hexokit.com/toolkit/)` and the check-updates line says "hexokit.com versions manifest"
- [x] A-009 R6: `src/cmd/shll/standards/*.md` and `src/cmd/shll/skill/skill.md` are byte-identical to their `docs/site/` sources

### Behavioral Correctness

- [x] A-010 R4: A primary failure of any degraded class (500, refused connection, bad JSON, wrong schema) falls through to the fallback URL; a primary success never contacts the fallback
- [x] A-011 R4: `SetTransportForTest(url, client)` keeps its signature; `check_updates_test.go` and the pre-existing `versions_test.go` tests pass unchanged

### Scenario Coverage

- [x] A-012 R4: Tests exist for primary-fail→fallback-success, primary-success→fallback-untouched, and both-fail→`ErrUnavailable`
- [x] A-013 R6: `TestStandardsEmbedMatchesCanonical` and `TestSkillEmbedMatchesCanonical` pass; `cd src && go test ./...` is green

### Edge Cases & Error Handling

- [x] A-014 R4: When every URL fails, the returned error wraps `ErrUnavailable` (so `shll check-updates --source released` keeps its exit-1 diagnostic path)
- [x] A-015 R4: Each attempt has its own `requestTimeout` context; a nil parent context still works

### Code Quality

- [x] A-016 Pattern consistency: URL hosts are named constants (no magic strings); the seam stays a package-level swappable var mirroring `internal/changelog`
- [x] A-017 No unnecessary duplication: the per-URL attempt is one helper reused by the loop; no second HTTP path
- [x] A-018 Scope discipline: no roster field, no standard name/file name, and no out-of-scope `shll.ai` / "shll toolkit" mention (X4 set, `agent_setup.go`, `install.md`, README body, `fab/project/*`) is touched
- [x] A-019 No new subprocess code (Constitution I) and no caching or persisted state (Constitution II)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Extract the single-URL attempt into an unexported helper (`fetchManifestFrom(ctx, url)`) and loop over `manifestURLs` in `FetchManifest` | Keeps `FetchManifest` under the codebase's typical function size; one HTTP path reused | S:70 R:90 A:90 D:80 |
| 2 | Confident | When all URLs fail, return the last attempt's error (wrapping `ErrUnavailable`) rather than a joined multi-error | Matches today's `%w: …` shape the check-updates diagnostic prints; the fallback host is the one a user would expect to be reachable last | S:60 R:90 A:85 D:65 |
| 3 | Confident | Long-help fallback wording: `(falls back to https://shll.ai/versions.json)` on a continuation line under `--source released` | Fits the existing two-line layout of that help block | S:60 R:95 A:85 D:75 |

3 assumptions (0 certain, 3 confident, 0 tentative).
