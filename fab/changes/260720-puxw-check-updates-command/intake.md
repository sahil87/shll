# Intake: Add `shll check-updates` — the toolkit's single update-check surface

**Change**: 260720-puxw-check-updates-command
**Created**: 2026-07-20

## Origin

Promptless dispatch via `/fab-proceed` from a live conversation (synthesized description). Raw input:

> Add `shll check-updates` — the toolkit's single update-check surface (check-only, never updates).
>
> run-kit's daemon currently checks for shll-toolkit updates itself (`app/backend/internal/updatecheck` in the run-kit repo): it fetches `https://shll.ai/versions.json` directly, joins against `brew list --versions`, and evaluates the per-tool notify policy. That is a layering smell — shll is the toolkit's meta-CLI and should own "is anything outdated?". A parallel run-kit change (already being drafted in the run-kit repo) will replace that logic with one exec of `shll check-updates --json --released`. This new command is also the single surface to absorb future `versions.json` schema evolution and notify-policy semantics (patch vs minor) — consumers never compile in version-comparison policy.

Key decisions were made in the originating conversation (backend flag names, the `--json` contract with exact field values, the resolver-seam architecture, exit-code proposal, explicit out-of-scope items) — they are reproduced verbatim in **What Changes** and encoded in **Assumptions**. Live facts verified during intake: `https://shll.ai/versions.json` fetched 2026-07-20 — schema 1, all **seven** tools present **including `shll` itself** (`notify` values observed: `minor` for six tools, `patch` for shll).

## Why

1. **The pain**: run-kit's daemon owns update-check logic that belongs to the toolkit's meta-CLI. It fetches `shll.ai/versions.json`, joins against `brew list --versions`, and evaluates per-tool notify policy — all inside a leaf tool. Every consumer that wants "is anything outdated?" would have to re-implement manifest parsing, version comparison, and notify-threshold policy.
2. **The consequence of not fixing it**: `versions.json` schema evolution and notify-policy semantics get compiled into N consumers. When schema 2 lands, or when the patch-vs-minor threshold semantics need refinement, every consumer needs a coordinated release. The layering smell also inverts the toolkit's composition model (Constitution IV): the meta-concern lives in a leaf.
3. **Why this approach**: a check-only `shll check-updates` command with a machine `--json` contract makes shll the single policy authority. run-kit's parallel change (already drafted in the run-kit repo) replaces its internal `updatecheck` package with one exec of `shll check-updates --json --released`. `shll changelog` becomes an internal consumer of the same resolver at the package seam. Consumers never compile in version-comparison policy.

**Constitution VII justification (new top-level subcommand)**: it is a machine primitive for an external consumer (run-kit's daemon) plus an internal consumer (`shll changelog`), and check-only semantics cannot be a flag on `update` — `update`'s contract is to *perform writes* (`update --dry-run` previews commands but never resolves latest versions). It cannot live in a per-tool CLI: whole-roster version resolution against the toolkit manifest is inherently the meta-tool's job (Constitution IV). `changelog` is the nearest neighbor but answers a different question ("what are the release *notes*?" — prose, no policy verdicts, no machine contract); `check-updates` answers "is anything outdated, and does it cross the notify threshold?" as data.

## What Changes

### 1. New subcommand `shll check-updates` (read-only)

Prints, for shll itself plus every roster tool (shll first, then leaves-first `Roster` order — the unified shll-first ordering principle), the installed version vs the latest available version. **Never writes** — no brew mutation, no self-upgrade, no side effects.

Two backends, selected by mutually exclusive flags:

- **`--released`** (**DEFAULT** — running with no backend flag behaves as `--released`): fetch `https://shll.ai/versions.json`. Live today, schema 1:

  ```json
  {
    "schema": 1,
    "generated_at": "2026-07-19T09:32:11Z",
    "tools": {
      "run-kit": { "latest": "3.8.2", "notify": "minor", "formula": "run-kit" },
      "shll":    { "latest": "0.1.6", "notify": "patch", "formula": "shll" }
    }
  }
  ```

  The manifest is the **roster + policy authority** for this backend: `latest` and `notify` come from it. Verified live during intake: all seven tools present including `shll` itself. One HTTP GET per invocation.

- **`--github`**: latest release tag per tool via the unauthenticated GitHub API — the same source `shll changelog` uses today via `internal/changelog` (`LatestTag`). Deliberately **NOT** named `--homebrew`: the backend is GitHub releases, not brew. No notify policy exists in this backend (see JSON contract below). Per-tool concurrent fetches, mirroring `FetchAll`/`resolveChangelog` (order preserved by index).

- Passing both flags is a usage error (exit 2 via `errExitCode{usageExitCode}` / cobra `MarkFlagsMutuallyExclusive`).

Installed versions come from brew reads via the existing probe patterns (`installedVersion`/`probeInstalledLeaf` in `brew.go`, routed through `internal/proc` — Constitution I). shll-self's installed anchor is its **brew-formula** version (`installedVersion(ctx, shllFormula)`), not the running binary's ldflags version — mirroring `shll changelog`'s bare-sweep precedent. Brew absence is gated exactly like `changelog`'s no-range forms: `brewMissingHint` on stderr + `errSilent` (the installed anchor always needs brew here).

**Freshness caveat (accepted)**: `versions.json` regenerates on shll.ai site deploys from daily-refreshed help envelopes, so it can lag/lead the tap — fine for a check/notify surface.

### 2. `--json` flag (the machine contract)

For machine consumers — run-kit is the motivating caller (`shll check-updates --json --released`; `--released` is the default so the explicit flag is belt-and-braces). Agreed contract, which run-kit will vendor as a test fixture — **consumers tolerate unknown fields** (additive evolution is safe):

```json
{
  "schema": 1,
  "source": "released",
  "tools": [
    { "name": "run-kit", "formula": "run-kit",
      "installed": "3.8.1", "latest": "3.8.2",
      "notify": "minor", "update_available": true, "notable": false }
  ]
}
```

- `source` — `"released"` or `"github"` (which backend produced the data).
- `update_available` — `installed < latest` (via `changelog.CompareVer` semantics: normalize `v` prefix + brew `_N` revision suffix).
- `notable` — the pending bump crosses the tool's notify threshold, from the manifest's `notify` field: `never` → never notable; `patch` → any pending bump is notable; `minor` → notable iff a minor-or-higher component increases (a patch-only bump is not notable); a major bump crosses every threshold. (The worked example row is consistent: `notify: minor` + a 3.8.1→3.8.2 patch bump → `update_available: true, notable: false`.)
- **`--github` backend**: no notify policy exists, so rows **omit** `notify` and `notable` (`omitempty`) — honest omission over invented defaults; the tolerate-unknown/missing-fields rule covers consumers (Assumption 11).
- **Unresolvable-row rule (documented)**: a JSON row is emitted only when both `installed` and `latest` resolve. A tool that is not installed, missing from the manifest (`--released`), or whose per-tool fetch failed (`--github`) is **omitted** from `tools[]` — absent row = never matches for consumers (the description's sanctioned rule). Human output still reports uninstalled tools as `not installed` (Constitution V reporting), so nothing is hidden from humans.
- JSON encoding follows the `list`/`doctor` precedent: `json.Encoder` with `SetEscapeHTML(false)`, 2-space indent, trailing newline.

### 3. Internal resolver seam

A new internal package — working name `src/internal/versions` — providing "latest version per tool" with the two backends:

- Owns the `versions.json` fetch (URL as a named constant, per-request context timeout mirroring `internal/changelog`'s `requestTimeout`), schema-1 decode, and the notify-threshold (`notable`) computation. This is the single surface that absorbs future `versions.json` schema evolution.
- The `--github` backend delegates to `internal/changelog`'s existing fetch (`LatestTag`, `CompareVer`, `NormalizeVer`) — no duplicated GitHub code.
- **HTTP stays in internal packages only** (the `internal/changelog` precedent — `cmd/shll` never imports `net/http`; `TestCmdShllNoNetHTTP` continues to enforce this). Test seams mirror `internal/changelog`'s package-level `baseURL`/`httpClient` vars + a `SetTransportForTest`-style swap so tests drive an `httptest.Server`.
- Package home is a plan-stage refinement point: extending `internal/changelog` instead of a new package is an acceptable alternative if the seam lands more naturally there (Assumption 14).

### 4. `shll changelog` becomes a consumer at the package seam

`shll changelog`'s "installed → latest" anchor (its no-range resolution) consumes the shared resolver at the **package** seam. Its CLI surface, output, and default GitHub-notes behavior stay **unchanged**. Hard constraint carried from memory: the seam MUST preserve changelog's single-GET no-range contract (`LatestTag` returns the release list so no second fetch happens — pinned by `TestChangelog_NoRangeSingleFetchPerRepo`).

### 5. Exit-code semantics (machine callers)

Following the toolkit `0/1/2` convention (`translateExit`):

| Condition | Exit |
|-----------|------|
| Check ran successfully — regardless of whether updates are pending (verdicts live in the JSON/output) | 0 |
| The check itself failed: `--released` manifest fetch failure, brew missing | 1 (`errSilent` after a stderr diagnostic) |
| Usage error: both backend flags, unknown flag/arg | 2 |
| `--github` per-tool fetch failures | degrade per-tool (row omitted in JSON; `unavailable` note in human output), run still exits 0 — the changelog degradation precedent |

No distinct exit code for "notable updates exist" — verdicts live in the output; a third code would overload run-kit's skip-on-nonzero contract (run-kit treats non-zero/unparseable as "skip silently this pass"). `--released` has exactly one fetch, so its failure fails the whole check; `--github` is per-tool, so it degrades per-tool (Constitution V).

### 6. Human (non-`--json`) output

A column-aligned, self-labeling table in the `shll version` style (shll first, roster order) — **no** `▸`/`==>` per-tool headers and no summary tail: the per-tool-output-separation spec scopes headers to commands that stream sub-tool output, and explicitly excludes read-only self-labeling aggregations (the `version` precedent). Unicode (`→`) degrades to ASCII (`->`) on non-TTY/`NO_COLOR` via the shared `ui.go` helpers. Illustrative (non-binding) sketch:

```
shll     0.1.5 → 0.1.6   update available (notable)
wt       0.1.3           up to date
run-kit  3.8.1 → 3.8.2   update available
idea     not installed
```

### 7. Explicitly out of scope

- **No `--released`/`--github` modes on `shll update`** — update stays brew-driven (brew cannot install pinned versions; Constitution III wrap-brew). `check-updates` is check-only.
- No caching of any kind (Constitution II — every invocation fetches fresh).
- No change to run-kit itself (that is the parallel change in the run-kit repo).
- No positional tool args in v1 — whole-roster sweep only (the motivating consumers need the full sweep); subset targeting via `resolveTargets` is a compatible later addition (Assumption 17).

### Standards check obligation (Constitution — Toolkit Standards)

This change alters the CLI surface, so apply MUST read the governing files under `docs/site/standards/` (at minimum the CLI principles standard and the help-dump contract) before implementation, and update `README.md`/`docs/site/` per the readme-extraction standard (the root `Long` help lists user-facing subcommands — twelve becomes thirteen). The hidden `help-dump` envelope picks the new subcommand up automatically (programmatic cobra walk).

## Affected Memory

- `cli/check-updates`: (new) the `shll check-updates` command surface — backends, `--json` contract, notify-threshold semantics, exit codes, degradation
- `cli/commands`: (modify) subcommand count 12→13, Constitution VII justification list entry, file layout table row for `check_updates.go`
- `cli/changelog`: (modify) note the no-range "installed → latest" anchor now consumes the shared resolver seam (single-GET contract preserved)
- `internal/versions`: (new) the resolver package — manifest fetch, backend seam, notable computation (file lands wherever the package lands; if the seam is folded into `internal/changelog` instead, this becomes a `(modify)` on `internal/changelog`)
- `internal/changelog`: (modify) exports/seam consumed by the new resolver's GitHub backend

## Impact

- **New**: `src/cmd/shll/check_updates.go` + `check_updates_test.go`; `src/internal/versions/` package + tests (subject to the package-home assumption)
- **Modified**: `src/cmd/shll/root.go` (AddCommand + `rootLong` — thirteen user-facing subcommands), `src/cmd/shll/changelog.go` (consume the resolver seam), `src/internal/changelog/changelog.go` (possible small exports for the seam)
- **Docs**: `README.md` + `docs/site/` per the toolkit standards
- **Invariants that must stay green**: `TestCmdShllNoNetHTTP` (no `net/http` in `cmd/shll`), `TestChangelog_NoRangeSingleFetchPerRepo` (single-GET), the changelog output golden strings (CLI surface unchanged), `TestShllSelf_NotInRoster` (`len(Roster) == 6` — check-updates iterates shll-self + Roster, never adds shll to Roster)
- **Dependencies**: none new — stdlib `net/http` (internal only), existing `internal/proc` brew probes
- **External coupling**: run-kit will vendor the `--json` output as a test fixture; the contract's evolution rule is additive-only (consumers tolerate unknown fields)

## Open Questions

None — all decision points were either settled in the originating conversation or recorded as graded assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | New top-level subcommand `check-updates` (check-only, never writes), with the Constitution VII justification as stated | Discussed — justification supplied verbatim in the originating conversation; machine primitive for run-kit + internal consumer | S:95 R:70 A:95 D:95 |
| 2 | Certain | Backends: `--released` (default, `shll.ai/versions.json`) and `--github` (GitHub releases), mutually exclusive; NOT named `--homebrew` | Discussed — names and default chosen explicitly | S:95 R:80 A:90 D:95 |
| 3 | Certain | `--json` contract exactly as given (`schema:1`, `source`, per-tool rows with `name/formula/installed/latest/notify/update_available/notable`); consumers tolerate unknown fields | Discussed with exact values; run-kit vendors it as a fixture | S:95 R:60 A:90 D:90 |
| 4 | Certain | Stateless: every invocation fetches fresh, no caching | Constitution II mandates it | S:90 R:90 A:100 D:100 |
| 5 | Certain | `shll changelog` consumes the shared resolver at the package seam; CLI surface/output unchanged; single-GET no-range contract preserved | Discussed; memory pins the single-fetch contract (`TestChangelog_NoRangeSingleFetchPerRepo`) | S:85 R:75 A:90 D:85 |
| 6 | Certain | No `--released`/`--github` modes on `shll update` in this change; update stays brew-driven | Discussed — explicit out-of-scope with Constitution III rationale | S:95 R:90 A:95 D:95 |
| 7 | Certain | `versions.json` is the roster+policy authority for `--released`; schema 1 verified live 2026-07-20 (all 7 tools incl. shll; `notify` minor/patch observed) | Verified by fetching the live manifest during intake | S:90 R:80 A:95 D:90 |
| 8 | Confident | Exit codes: 0 = check ran (regardless of pending updates), 1 = check itself failed (manifest fetch failure, brew missing), 2 = usage; NO distinct exit code for "notable updates exist" | Origin proposed this and instructed refine-as-assumption; a third code would overload run-kit's skip-on-nonzero rule; toolkit 0/1/2 convention | S:80 R:70 A:75 D:70 |
| 9 | Confident | Failure granularity: `--released` manifest fetch failure fails the whole check (exit 1); `--github` per-tool fetch failures degrade per-tool (row omitted / `unavailable` note), run exits 0 | One fetch vs N fetches; changelog's Constitution V degradation precedent | S:70 R:75 A:80 D:70 |
| 10 | Confident | JSON unresolvable-row rule: emit a row only when both `installed` and `latest` resolve; not-installed / not-in-manifest / fetch-failed tools omitted from `tools[]` (absent row = never matches); human output still prints `not installed` | Origin sanctioned "omitted … per a consistent documented rule"; keeps run-kit's join trivial; Constitution V satisfied by the human surface | S:75 R:65 A:80 D:65 |
| 11 | Confident | `--github` rows omit `notify`/`notable` (`omitempty`) — no policy source exists in that backend | Origin offered "notify defaults or omit"; honest omission over invented defaults; missing-field tolerance covers consumers | S:70 R:75 A:75 D:65 |
| 12 | Confident | Notify-threshold semantics: `never` → never notable; `patch` → any bump notable; `minor` → notable iff minor-or-higher component increases; major crosses every threshold | The only reading consistent with the origin's worked example (`notify:minor`, patch bump → `notable:false`) | S:75 R:70 A:80 D:75 |
| 13 | Confident | Unknown/future `notify` value in the manifest treated as `minor` | Not discussed; forward-compat conservatism — mild over-notification beats silent suppression on a notify surface; easily revised | S:45 R:85 A:55 D:45 |
| 14 | Confident | Resolver home: new `src/internal/versions` package, GitHub backend delegating to `internal/changelog`; extending `internal/changelog` instead is an acceptable plan-stage alternative | Origin's "e.g." named both; internal-only and reversible at plan/apply | S:55 R:80 A:70 D:45 |
| 15 | Confident | Human output: `version`-style self-labeling aligned table, no `▸`/`==>` headers, no summary tail; `ui.go` ASCII degrade applies | The per-tool-output-separation spec excludes read-only self-labeling aggregations (the `version` precedent); check-updates streams no sub-tool output | S:65 R:85 A:80 D:70 |
| 16 | Certain | shll-self's installed anchor is the brew-formula version (`installedVersion(ctx, shllFormula)`), not the running binary's ldflags version; brew absence → `brewMissingHint` + exit 1 | Mirrors `shll changelog`'s bare-sweep precedent exactly | S:75 R:80 A:85 D:80 |
| 17 | Confident | v1 takes no positional tool args (`cobra.NoArgs`) — whole-roster sweep only; subset targeting is a compatible later addition | Not discussed; both motivating consumers need the full sweep; additive to relax later | S:45 R:80 A:65 D:45 |

17 assumptions (8 certain, 9 confident, 0 tentative, 0 unresolved).
