---
type: memory
description: "`shll check-updates` — read-only update-check surface for shll-self + every roster tool: the `--source released|github` enum flag (default `released` = shll.ai versions manifest + notify policy; `github` = release tags), the `--json` machine contract (schema 1: `{schema, source, tools[]}` with unresolvable-row omission, `notify`/`notable` on released rows only), the notify-threshold `notable` semantics, version-style human table, and the 0/1/2 exit codes with per-tool github-backend degradation."
---
# cli/check-updates

`shll check-updates` — the toolkit's single update-check surface. It reports, for shll itself plus every roster tool, the installed version vs the latest available version, and (on the released backend) whether a pending bump crosses the tool's notify threshold. It is **check-only** — no brew mutation, no self-upgrade, no file writes, no caching. To apply updates the user runs `shll update`.

Source: `src/cmd/shll/check_updates.go`; the latest-version resolver seam in [internal/versions](/internal/versions.md); GitHub fetch/version compare in [internal/changelog](/internal/changelog.md). (puxw)

## Constitution VII justification

`check-updates` is a new top-level subcommand: a machine primitive for an external consumer (run-kit's update-check daemon, which runs `shll check-updates --json` — the released backend is the default) plus an internal consumer (`shll changelog`, which shares the resolver seam). It makes shll the single policy authority so consumers never compile in `versions.json` parsing, version comparison, or notify-threshold policy — future schema evolution is absorbed once, in [internal/versions](/internal/versions.md).

- **Cannot be a flag on `update`** — `update`'s contract is to *perform writes* (even `update --dry-run` previews brew commands rather than resolving latest versions), whereas `check-updates` resolves latest versions and writes nothing.
- **Cannot live in a per-tool CLI** — whole-roster version resolution against the toolkit manifest is inherently the meta-tool's job (Constitution IV).
- **`changelog` is the nearest neighbor but answers a different question** — "what are the release *notes*?" (prose, no policy verdicts, no machine contract) vs. "is anything outdated, and does it cross the notify threshold?" (data). The registration is in [cli/commands](/cli/commands.md#cobra-root).

## Command surface

`newCheckUpdatesCmd()` — `Use: "check-updates"`, `cobra.NoArgs` (no positional tool args in v1 — whole-roster sweep only), a layered `Long` help block (summary, the backend selector, concrete examples, exit codes). The cobra factory delegates to the writer-seam `runCheckUpdates(ctx, stdout, stderr, source string, jsonOut bool)` (explicit `io.Writer`s, mirroring `runList`/`runChangelog`), driven directly by `check_updates_test.go` with `bytes.Buffer` writers, a fake `proc.Runner`, and the `internal/versions` + `internal/changelog` transport seams.

Two flags:

- **`--source`** (string, default `released`, no shorthand) — the backend selector, an enum over the source constants `sourceReleased` (`released`) and `sourceGithub` (`github`):
  - `released` (the **default backend** — running with no `--source` behaves as `--source released`): resolve latest versions + notify policy from the shll.ai versions manifest.
  - `github`: resolve latest release tags via the GitHub API (unauthenticated). The value is deliberately `github`, **not** `homebrew`: the source is GitHub releases, not brew. No notify policy exists in this backend.
  The two values double as the `--json` envelope's `source` field values — flag name, flag value, and envelope output are one vocabulary. `cobra.RunE` reads the flag via `cmd.Flags().GetString(sourceFlag)` and passes it raw to the seam; validation lives in the seam (see § Exit codes and § Design Decisions), not in cobra.
- **`--json`** (the shared `jsonFlag` constant from `list.go`) — emit the machine contract instead of the human table.

The flag name, usage string, source names (`sourceReleased`/`sourceGithub`, reused as the enum values), the invalid-value diagnostic (`invalidSourceErrFmt`), schema tag (`checkUpdatesSchema = 1`), and status labels are all named constants (code-quality.md — no magic strings).

## The sweep target set — shll-first, roster order

`checkUpdateTargets()` builds the sweep: **shll itself first** (the unified [shll-first ordering principle](/cli/commands.md#unified-shll-first-ordering--the-principle)), then every `Roster` tool in roster order. shll is **not** added to `Roster` (Constitution III — `len(Roster)` stays 7, guarded by `TestShllSelf_NotInRoster`); the sweep prepends it as a `checkTarget`.

shll-self's installed anchor is its **brew-formula** version (`installedVersion(ctx, shllFormula)`), **not** the running binary's ldflags version — mirroring [`shll changelog`'s bare-sweep precedent](/cli/changelog.md#the-bare-sweep-no-args). Each `checkTarget` carries the tap-relative `formulaLeaf` (`strings.TrimPrefix(Formula, formulaPrefix)` → `run-kit`, `shll`) emitted as the JSON `formula` field, the fully-qualified `brewFormula` (the brew probe key), the `repo` slug (the github-backend fetch key), and the roster `tool` itself (zero for shll-self). A **delegated (non-brew) roster tool carries no `brewFormula`** (t26g): its `formulaLeaf` falls back to its `Name` (never a bare `brew install <name>` hint — it is not a formula) and its installed anchor resolves through its `Probe` spec (`probeToolInstalledVersion` → `rk desktop status`), never a brew read — see [cli/version §the shared install probe](/cli/version.md#the-shared-install-probe).

## Backends

### `released` — the manifest is the roster + policy authority

Exactly **one** HTTP GET of `https://shll.ai/versions.json` per invocation (via [`versions.FetchManifest`](/internal/versions.md), no caching — Constitution II). `latest` and `notify` come from the manifest, looked up by tool **name**. Because it is the single latest+policy source, a manifest fetch failure (transport error, timeout, non-200, decode failure, or an unsupported `schema`) **fails the whole check**: a stderr diagnostic + `errSilent` (exit 1). Pinned by `TestCheckUpdates_ManifestFetchFailureExit1`, `TestCheckUpdates_UnsupportedSchemaFailsCheck`. Selected by `--source released` (the default).

### `github` — delegated, concurrent, per-tool degradation

Each tool's latest release tag resolves via [`versions.LatestGitHub`](/internal/versions.md) (a thin delegation to `internal/changelog.LatestTag` — no duplicated GitHub fetch code). No notify policy exists here. A per-tool fetch failure **degrades per-tool** — the JSON row is omitted, the human row shows `unavailable` — and the run still exits 0 (the changelog degradation precedent, Constitution V). Pinned by `TestCheckUpdates_GithubPerToolFailureDegrades`. Selected by `--source github`.

The freshness caveat is accepted: `versions.json` regenerates on shll.ai site deploys from daily-refreshed help envelopes, so it can lag/lead the tap — fine for a check/notify surface.

## Resolution — concurrent, order-preserving

`resolveCheckUpdates` resolves every target **concurrently** (one goroutine per target, results written into a fixed-size slice **indexed by position** so output stays shll-first roster order — the `resolveChangelog`/`probeInstalled` pattern). `resolveOneTarget` makes one installed-anchor read (a brew read for brew-managed tools, the delegated `Probe` spec for a non-brew tool — t26g) plus, on the `github` backend, one GitHub releases fetch; the `released` backend looks the target up in the already-fetched manifest with no further network access. All versions are normalized via `changelog.NormalizeVer` (strip `v` prefix + brew `_N` revision) so both sides share one comparable form.

`rowResolved` (both `installed` and `latest` non-empty) is the JSON unresolvable-row emit condition. `rowUpdateAvailable` is `installed < latest` via `changelog.CompareVer`.

## Brew gate

The brew-managed installed anchors are brew reads (shll-self included), so brew must be present **regardless of backend** — mirroring [changelog's no-range gate](/cli/changelog.md#brew-precondition--gated-on-whether-brew-is-actually-read). `!hasBrew(ctx)` prints the shared `brewMissingHint` on stderr and returns `errSilent` (exit 1), checked **before** any backend fetch (the released manifest guard confirms the gate precedes the fetch). Pinned by `TestCheckUpdates_BrewMissingHint`.

## `--json` machine contract

The envelope is `checkUpdatesReport{Schema, Source, Tools}` → `{"schema": 1, "source": "released"|"github", "tools": [...]}`, one `checkUpdateItem` per **resolved** tool:

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

- **`source`** names the backend that produced the data.
- **Unresolvable-row rule** — a row is emitted only when both `installed` and `latest` resolve. Not-installed, missing-from-manifest (`released` backend), or fetch-failed (`github` backend) tools are **omitted** from `tools[]` (absent row = never matches for consumers). Human output still reports those tools (below), so nothing is hidden from humans.
- **`notify` / `notable` are released-backend-only.** On released rows both are present — including an explicit `"notable": false`. `github` rows omit both keys (no policy source exists there — honest omission over invented defaults). This is why `Notable` is a `*bool` with `omitempty` (nil on github rows, `&value` on released rows) and `Notify` is `string,omitempty` — a plain `bool` + `omitempty` would wrongly drop a legitimate `false`.
- **`update_available`** is `installed < latest` (`changelog.CompareVer`); **`notable`** is [`versions.Notable`](/internal/versions.md).
- **Encoding** follows the `list`/`doctor` precedent: `json.Encoder`, `SetEscapeHTML(false)`, 2-space indent, trailing newline. An empty resolved set emits `"tools": []`, never `null` (`make([]checkUpdateItem, 0, …)` guarantees non-nil — `TestCheckUpdates_EmptyResolvedSetEmitsEmptyArray`).
- **Evolution rule** (external): consumers tolerate unknown fields; additions are additive-only, so run-kit can vendor the output as a test fixture.

Pinned by `TestCheckUpdates_JSONContractReleased` (field values incl. the literal `"notable": false`, unresolved-row omission) and `TestCheckUpdates_GithubJSONOmitsNotifyNotable` (`source:"github"`, no `notify`/`notable` keys).

## Notify-threshold (`notable`) semantics

`notable` is true iff an update is available **and** the pending bump crosses the tool's notify threshold, computed by [`versions.Notable(notify, installed, latest)`](/internal/versions.md#requirement-notify-threshold-notable): `never` → never notable; `patch` → any pending bump notable; `minor` → notable iff a minor-or-higher component increases (a patch-only bump is not); a major bump crosses every non-`never` threshold; an unknown/future `notify` value is treated as `minor` (forward-compat conservatism). The worked example is consistent: `notify: minor` + a 3.8.1→3.8.2 patch bump → `update_available: true, notable: false`.

## Human (non-`--json`) output

`writeCheckUpdatesTable` — a column-aligned, self-labeling `tabwriter` table in the `shll version` style (same tabwriter config; shll first, roster order): `name`, the version cell, the status cell. Deliberately **no** `▸`/`==>` per-tool headers and **no** summary tail — the per-tool-output-separation spec scopes headers to commands that stream sub-tool output and excludes read-only self-labeling aggregations (the `version` precedent). `check-updates` streams no sub-tool output.

`checkUpdateCells` derives one row's version + status:

| Row state | Version cell | Status cell |
|-----------|--------------|-------------|
| Not installed (`installed == ""`) | `not installed` (`notInstalledLabel`, reused from `version.go`) | *(empty)* |
| `github` backend, per-tool fetch failed | installed version | `unavailable` |
| `released` backend, name absent from manifest | installed version | `not in manifest` |
| Up to date (`installed ≥ latest`) | installed version | `up to date` |
| Update available | `installed → latest` (arrow degrades to `->`) | `update available`, plus ` (notable)` when the `released` backend and the bump is notable |

The transition arrow degrades from `→` to `->` on a non-TTY / `NO_COLOR` stream via the shared `arrow(color)` helper ([ui.go](/cli/commands.md#shared-ui-helper-uigo)); the color decision is computed once via `colorEnabled(stdout)`. A `bytes.Buffer` test writer is never a TTY, so tests deterministically assert the ASCII forms. Pinned by `TestCheckUpdates_ReleasedHappyPathTable`, `TestCheckUpdates_NotInManifestRow`.

The `not in manifest` label (a released tool absent from the manifest) is distinct from `unavailable` (a github-backend per-tool fetch failure) so the two unresolved causes stay distinguishable for humans.

## Exit codes (the toolkit 0/1/2 convention)

Follows `translateExit` ([cli/commands §exit-code translation](/cli/commands.md#exit-code-translation)):

| Condition | Exit |
|-----------|------|
| Check ran successfully — regardless of whether updates are pending (verdicts live in the JSON/output) | 0 |
| Check itself failed: `released`-backend manifest fetch/schema failure, or brew missing | 1 (`errSilent` after a stderr diagnostic) |
| Usage error: unknown `--source` value, unknown flag/arg | 2 (`errExitCode{code: usageExitCode}`) |
| `github`-backend per-tool fetch failure | degrade per-tool (row omitted / `unavailable` note), run still exits 0 |

There is **no distinct exit code for "notable updates exist"** — verdicts are data, not exit codes; a third code would overload run-kit's skip-on-nonzero contract (it treats any non-zero/unparseable exit as "skip silently this pass"). The asymmetry — the `released` backend fails the whole check while `github` degrades per-tool — follows from cardinality: `released` has exactly one fetch, `github` has N (Constitution V). The unknown-`--source`-value usage error is pinned by `TestCheckUpdates_UnknownSourceValueUsageError` (probes with `"bogus"`; also asserts zero recorded subprocess calls — the usage error fires before any network/brew access).

## Design Decisions

### Enum validation in the run seam, not cobra machinery
**Decision**: `runCheckUpdates` receives the raw `--source` string and validates it as the first check — a value that is neither `sourceReleased` nor `sourceGithub` returns `&errExitCode{code: usageExitCode, msg: fmt.Sprintf(invalidSourceErrFmt, ...)}` (exit 2, naming the offending value and the valid set), before the brew gate and any network access.
**Why**: pflag has no native enum type, and cobra-side rejection (a custom `pflag.Value`, `MarkFlagsMutuallyExclusive`, or `PreRunE`) surfaces a plain error that matches none of `cobraUsageErrorPrefixes`, so it would exit 1 — the contract pins usage errors at exit 2. Validating in the seam keeps exit-code policy out of cobra's hands, follows the `shell-init`/`setup shell` precedent (`errExitCode{code: 2}` for user-invocation errors), and keeps the case testable through the writer seam (zero recorded subprocess calls, empty stdout).
**Rejected**: a custom `pflag.Value` enum type (its parse error routes through cobra's error path, exiting 1 unless coupled to cobra-internal message shapes); cobra `PreRunE` validation (same exit-code coupling, less testable through the seam).
*Introduced by*: 260720-ubys-check-updates-source-flag

### `--source released|github`, not `--backend shll|github`
**Decision**: the backend selector is a single string flag named `--source`, and its enum values are the existing envelope source constants `released`/`github`.
**Why**: the `--json` envelope already carries `"source": "released"|"github"` — naming the flag `--source` with those same values makes flag name, flag value, and envelope output one vocabulary, so a consumer reading the JSON can write the flag back verbatim.
**Rejected**: `--backend shll|github` (vocabulary mismatch with the envelope; `shll` is ambiguous as a source name — reads as the binary — and renaming the value would either mismatch the envelope's `"source": "released"` or force a breaking schema-2 change for zero gain); keeping the removed `--released`/`--github` bools as hidden deprecated aliases (not warranted for a day-old surface); bumping the JSON schema (nothing in the envelope changes).
*Introduced by*: 260720-ubys-check-updates-source-flag

### `notable` as `*bool` with `omitempty`
**Decision**: the JSON row's `Notable` field is `*bool` with `json:"notable,omitempty"` (nil on `github` rows, `&value` on `released` rows); `Notify` is `string` with `json:"notify,omitempty"`.
**Why**: the contract requires `"notable": false` to be emitted on released rows (the worked example) while the key is omitted entirely on github rows — a plain `bool` + `omitempty` would wrongly drop `false` everywhere.
**Rejected**: two row struct types per backend (more code, same bytes); always emitting `notable` with an invented default on github rows (the intake chose honest omission).
*Introduced by*: 260720-puxw-check-updates-command

## Cross-references

- The latest-version resolver seam (manifest fetch, `Notable`, `LatestGitHub`): [internal/versions](/internal/versions.md).
- GitHub fetch, version normalize/compare, and the `FirstDiffComponent` bump classifier: [internal/changelog](/internal/changelog.md).
- The internal consumer sharing the resolver seam: [cli/changelog](/cli/changelog.md#the-bare-sweep-no-args).
- Root wiring, exit-code translation, the shll-first ordering principle, the `Roster`: [cli/commands](/cli/commands.md).
- ASCII-degrade rule and per-tool-header scoping: `docs/specs/per-tool-output-separation.md`.
