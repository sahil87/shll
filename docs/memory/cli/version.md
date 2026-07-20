---
type: memory
description: "`shll version` — column-aligned plain-text table, per-tool 2s timeout, ldflags-injected `shll` version; also hosts the shared `toolInstalled`/`probeToolVersion` install probe and its rk→run-kit legacy-name PATH fallback (ErrNotFound-only), plus the root `shll --version` flag (producer surface) pinned by a `version`-standard conformance test — distinct from the subcommand's consumer-side table."
---
# cli/version

`shll version` — prints a column-aligned plain-text table with the version of `shll` itself plus every roster tool.

Source: `src/cmd/shll/version.go`. Uses the shared brew helpers in `src/cmd/shll/brew.go` and the `Roster` from `src/cmd/shll/tools.go`.

## Output shape

```
shll      v0.1.0
wt        v0.1.0
idea      not installed
tu        v0.1.0
run-kit   v0.1.0
hop       v0.1.0
fab-kit   v0.1.0
```

- Exactly **7 rows**: one for `shll`, then one per roster tool in roster order (`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit` — the leaves-first order, auvj). `version` output is order-agnostic in test (assertions are index-paired to `Roster`, so reorder moves expected and actual in lockstep); only this example's ordering reflects the slice. The `run-kit` row displays `run-kit` regardless of whether the primary `run-kit --version` probe or the legacy `rk --version` fallback succeeded (see [The legacy-name PATH-probe fallback](#the-legacy-name-path-probe-fallback)). See [cli/commands](/cli/commands.md#design-decision-leaves-first-roster-order).
- Column-aligned via `text/tabwriter` (`src/cmd/shll/version.go:56`) — minwidth 0, tabwidth 0, padding 2, padchar space, no flags.
- When the upstream tool's `--version` output contains a SemVer-shaped token, the row is normalized to a `v`-prefixed token (e.g. `v1.9.4`). When no such token is present, the row falls through the prefix-strip and raw-passthrough branches and may emit a non-`v` string (e.g. `dev`, or an unparseable banner verbatim) — see the `normalizeVersion` pipeline below for the full contract.
- **Plain text only.** No ANSI escapes, no JSON, no colors. The output is meant to paste cleanly into bug reports.

## Behavior contract

`runVersion(ctx, stdout)` (`src/cmd/shll/version.go:52`) is the implementation seam. The `runVersion`/`toolVersion` output contract — `normalizeVersion`, the `not installed` label, the per-tool timeout, and the plain-text-no-JSON shape (the probe itself is a shared helper — see [The shared install probe](#the-shared-install-probe)):

1. Construct a `tabwriter.Writer` over stdout.
2. Write `shll\t<normalizeVersion(version)>\n` first, where `version` is the package-level variable (see Ldflags injection below). The shll row goes through the same normalizer as roster rows, so the column is uniform.
3. For each tool in `Roster` (in order), write `<tool.Name>\t<toolVersion(ctx, tool)>\n`.
4. `w.Flush()` — propagates any write error up.

`toolVersion(ctx, tool)` (`src/cmd/shll/version.go:101`) is the per-tool resolver:

1. Call `probeToolVersion(ctx, tool)` — the shared probe (see [The shared install probe](#the-shared-install-probe) below), which runs `<tool.Name> --version` under a `versionTimeout` deadline (via `probeVersionByName`) and returns `([]byte, error)`, retrying once with `tool.LegacyName` on `proc.ErrNotFound` only (the rk→run-kit fallback).
2. On any error (`proc.ErrNotFound` for missing binary, exit non-zero, deadline exceeded, etc.) → return `notInstalledLabel = "not installed"`.
3. On success → return `normalizeVersion(string(out))`.

"Installed" is detected via `proc.ErrNotFound` (binary not on PATH) rather than a brew probe — install-mechanism agnostic, and saves ~400ms per tool (no Homebrew/Ruby startup tax).

`shll doctor` (d0ct) reuses the version probe through its own `probeVersion` helper — which **calls `probeToolVersion` directly** (9bak) (not just the same primitives), so the bounded invocation AND the rk→run-kit legacy-name fallback live in exactly one place (`version.go`) and cannot drift. `doctor` does NOT call `toolVersion` because `toolVersion` collapses the missing case and the unreportable (stale-brew-link) case into the single `notInstalledLabel`, whereas `doctor` needs them apart (install vs. reinstall suggestion); `probeVersion` adds only the three-way classification on top of `probeToolVersion`, leaving `toolVersion` untouched. See [cli/doctor](/cli/doctor.md#the-version-probe--probeversion-why-a-local-helper).

`normalizeVersion(raw string) string` (`src/cmd/shll/version.go`) is the single point of normalization shared by the shll row and every roster row. It is purely shape-based — there is no per-tool branching — so independent upstream `--version` standardization (e.g., tu/run-kit/fab-kit cleaning up their own output in parallel) is absorbed without shll code changes.

The normalization pipeline runs in this order on the input:

1. **First non-empty line.** Split on `\n`, find the first line whose `strings.TrimSpace` is non-empty, use that trimmed value. Empty / whitespace-only input returns `""`.
2. **Version-token regex.** Search the line for the first match of `versionTokenRE = v?\d+(\.\d+)*([.-][\w.+-]+)?` (`src/cmd/shll/version.go:30`). The token requires at least one numeric component; additional `.`-separated numerics and an optional `[.-]<suffix>` (pre-release / build metadata) are accepted, so `1`, `1.2`, `1.2.3`, `v1.2.3`, `1.2.3-rc1`, `1.2.3-rc1+build.42` all match. If a token is found, return it with a `v` prepended when absent (existing `v` is retained, never doubled).
3. **Generic prefix-strip heuristic.** If no version token was found, match the line against `versionPrefixRE = ^\S+\s+(?i:version)\s+(.+)$` (`src/cmd/shll/version.go:34`). The literal word `version` is case-insensitive (so `<word> Version <rest>` and `<word> version <rest>` are handled identically). On match, return the trimmed `<rest>` capture. The heuristic does NOT reference any tool name — it strips a leading `<word> version ` prefix regardless of what `<word>` is, which collapses `shll version dev` to `dev` without per-tool logic.
4. **Raw passthrough.** Otherwise, return the trimmed first non-empty line verbatim. This preserves whatever the tool emitted for the bug-report use case — losing information would be worse than displaying an unparseable banner.

The `v` prefix is **always-on**: matched tokens that lack `v` get one prepended; matched tokens that already start with `v` are returned unchanged. This matches SemVer tag convention and yields a uniform column.

The parser is **first-line-only**. It never scans deeper lines for a version token — even when the first non-empty line falls through to the raw-passthrough branch. If a tool puts a banner on line 1 and the version on line 2, the banner wins. The contract is predictable and testable as a single string-equality assertion.

The two regexes are compiled once via `regexp.MustCompile` at package scope; they are not recompiled per call.

## The shared install probe

The install probe is a **shared helper** (lst7), so `version` is not the sole definition of "installed = runnable on PATH":

- `probeToolVersion(ctx, tool) ([]byte, error)` (`src/cmd/shll/version.go`) is the **single** definition of the probe: it calls `probeVersionByName(ctx, tool.Name)` — the bounded invocation (`subCtx, cancel := context.WithTimeout(ctx, versionTimeout)`, `proc.Run(subCtx, name, "--version")`, capture transport, Constitution I) — and returns the captured output and any error. ANY error (`proc.ErrNotFound`, non-zero exit, timeout) means "not installed" — callers map that to their own representation. It carries the **legacy-name fallback** (below, 9bak).
- `probeVersionByName(ctx, name) ([]byte, error)` is the bounded `<name> --version` invocation, factored out so the fallback retry reuses the exact same deadline/transport.
- `toolInstalled(ctx, tool) bool` (`src/cmd/shll/version.go`) layers on `probeToolVersion` and returns `err == nil`. This is the boolean install-status helper consumed by `shll list` — see [cli/list §The install probe](/cli/list.md#the-install-probe-shared-toolinstalled).
- `toolVersion` also layers on `probeToolVersion` (mapping a non-nil error to `notInstalledLabel`, success to `normalizeVersion`).

So there is **exactly one place** that defines "installed = runnable", shared by `version` (string label), `list` (bool), and `doctor` (three-way state via `probeVersion`, which delegates to `probeToolVersion` — 9bak). This is the install-mechanism-agnostic notion — **NOT** the brew `isInstalled` probe (`src/cmd/shll/brew.go`) used by `install`/`update`.

### The legacy-name PATH-probe fallback

When the primary `<tool.Name> --version` fails with `proc.ErrNotFound` **only** AND the tool declares a non-empty `LegacyName`, `probeToolVersion` retries once with the legacy binary name (`probeVersionByName(ctx, tool.LegacyName)`). For `run-kit` the legacy name is `rk`, so a pre-rename install whose binary is still `rk` on PATH (no `run-kit` alias binary) is reported **installed** by `list`/`version`/`doctor` rather than "not installed".

- **`ErrNotFound` only — never a non-zero exit or timeout.** A present-but-broken `run-kit` (e.g. exits non-zero, or hangs to the deadline) must NOT silently defer to `rk`; its own error is returned, so the surface still reports it via the primary probe. Missing-binary is the only state the fallback is for.
- **Display name is untouched.** The row/label stays `tool.Name` (`run-kit`) regardless of which probe name succeeded — the fallback affects *detection*, not display.
- **Scope: DISPLAY surfaces only.** This PATH probe is a pure display-surface fallback: an `rk` install is *shown* by list/version/doctor, but shll performs no brew-formula migration anywhere (the migration guard was retired — see [cli/update §Retire the migration guard](/cli/update.md#retire-the-rkrun-kit-brew-formula-migration-guard)). Detection and migration are unrelated concerns.
- **A retained binary-alias/display surface.** `LegacyName` is the field this fallback keys on — kept because the run-kit formula still installs `rk` as an interchangeable command alias, so a machine may legitimately have only `rk` on PATH. It is NOT tied to formula migration (see [cli/commands §the rk→run-kit rename](/cli/commands.md#the-rkrun-kit-rename)).

Pinned by `version_test.go`: a run-kit visible only under the legacy `rk` binary (`run-kit --version` → `proc.ErrNotFound`, `rk --version` → a version) is reported installed with display name `run-kit`; a present-but-broken `run-kit` (non-`ErrNotFound` error) does NOT fall back.

## Ldflags injection (shll's own version)

The `shll` row's version comes from the package-level `version = "dev"` declared in `src/cmd/shll/main.go:18`, then passed through `normalizeVersion`. Build behavior:

- Default (uninjected): raw `dev` → normalized `dev`. Covers `go run` and unstamped local builds.
- Stamped: `scripts/build.sh` invokes `go build -ldflags "-X main.version=${VERSION}" ...`, where `VERSION=$(git describe --tags --always 2>/dev/null || echo dev)`. A stamped `v0.0.1` stays `v0.0.1`; a stamped bare `0.0.1` becomes `v0.0.1`.

Tests override the variable directly (`TestVersion_LdflagsInjection`) — no special build hook needed for testing.

> **The shll-first row is the canonical instance of the shared display pattern.** `version` leads with a `shll` row (step 2 of the behavior contract), reading shll's version from the package `version` var via `normalizeVersion` — never a `shll --version` self-subprocess. `version.go` writes its own `shll\t…` row inline rather than consuming the shared `shllSelf` descriptor; its version source is the same package var `shllSelfVersion()` reads, so the two surfaces agree by construction (bb7r). See [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor).

## Per-tool timeout

`versionTimeout = 2 * time.Second` (`src/cmd/shll/version.go:20`) — a named constant; magic numbers are forbidden by `code-quality.md`.

Properties (Design Decision #5):

- 2s is generous (typical `--version` runs in well under 100ms).
- Bounds worst-case `shll version` runtime to `len(Roster) * versionTimeout` ≈ 12 seconds even if every tool hangs.
- A timeout is treated as "not installed" — we don't differentiate hung-but-installed from missing in the output. The user gets a usable table either way.
- The deadline applies only to the `--version` invocation. There is no separate install probe — installation is inferred from `proc.ErrNotFound` returned by the same `--version` call.

`TestVersion_TimeoutHandling` simulates the timeout path by having the fake runner return `context.DeadlineExceeded` immediately for the targeted tool (no real wall-clock wait), then asserts the row reads `not installed` and that the test's elapsed time stays under `versionTimeout`.

## Spec-locked Design Decisions for this subcommand

### #4 Plain-text output, no `--json`

> *Why*: Primary use case is bug reports — pasting output into a Slack thread or GitHub issue. Plain text is universally legible.
> *Rejected*: `--json` flag for v0.1.0. Add later if a real script-consumer emerges; YAGNI for now.

### #5 Per-tool `--version` invocations have a 2-second timeout

> *Why*: Protects against deadlocked sub-tools. 2s is generous for `--version` (typical < 100ms) but bounded enough that worst-case `shll version` finishes in under 15 seconds even if every roster tool hangs.
> *Rejected*: no timeout (one bad tool blocks the whole command); 500ms (too aggressive — some tools may legitimately take longer on a cold start, especially on macOS first-run gatekeeper checks).

## Test seam

`version_test.go` installs a fake via `installFakeRunner(t, f)` and uses helper builders like `versionFake(installed map, versions map)` to canned-respond per-tool.

Integration scenarios:

- `TestVersion_AllInstalled` — seven rows in roster order, column-aligned, normalized values.
- `TestVersion_SomeMissing` — `idea` not installed → row reads `idea  not installed`.
- `TestVersion_LdflagsInjection` — overrides `version` package var → `shll` row reflects it (after normalization).
- `TestVersion_DefaultDev` — leaves `version` at `"dev"` → `shll` row reads `dev`.
- `TestVersion_TimeoutHandling` — fake returns `context.DeadlineExceeded` immediately for the targeted tool (no real wall-clock wait) → row reads `not installed`. The test also asserts elapsed time stays under `versionTimeout` to confirm the fake short-circuited rather than actually blocking.
- `TestVersion_NoANSI` — asserts no `\x1b[` escape in output.

Legacy-name fallback (9bak):

- `TestProbeToolVersion_LegacyNameFallbackOnErrNotFound` — run-kit's primary `run-kit --version` returns `proc.ErrNotFound`, the `rk --version` retry returns a version → reported installed (display name stays `run-kit`).
- `TestProbeToolVersion_NoFallbackOnNonErrNotFound` — a present-but-broken `run-kit` (non-`ErrNotFound` error) does NOT retry `rk`; the primary error is returned.

Unit scenarios pinning the normalization contract (12 cases, all named `TestNormalizeVersion_*`):

- `_NamePrefixedBare` (`fab-kit version 1.9.4` → `v1.9.4`), `_NamePrefixedV` (`hop version v0.1.5` → `v0.1.5`, no doubling), `_Bare` (`0.4.10` → `v0.4.10`).
- `_BareDev` (`dev` → `dev`), `_NamePrefixedDev` (`shll version dev` → `dev` via prefix-strip), `_Unparseable` (raw passthrough).
- `_Empty` (`""` and whitespace-only → `""`), `_FirstLineOnly` (banner on line 1 wins; line 2 never searched), `_BlankLeadingLines` (leading blanks skipped to find the first non-empty line).
- `_PermissiveSemVer` (`1.2` and `1.2.3-rc1+build.42`), `_CaseInsensitiveVersionWord` (`MyTool Version 1.0` → `v1.0`), `_PrefixStripCase` (`shll Version dev` → `dev`).

## The root `--version` flag (producer side) — pinned by conformance test

`shll --version` (the cobra root flag) and `shll version` (the subcommand table above) are **two distinct surfaces**. The subcommand is shll's **consumer/composer** surface — it probes and normalizes every roster tool's `--version`. The root `--version` flag is shll's own **producer** surface: it is what the toolkit `version` standard binds `shll` to as one of the seven binaries (the standard holds shll to the shape it enforces — see [cli/standards-conformance §producer-surface standards](/cli/standards-conformance.md#the-three-producer-surface-standards-updateversionshell-init)).

Mechanism: `main.go` wires `rootCmd.Version = version` (the same ldflags-injected package var the shll row reads), enabling cobra's built-in root version flag. Cobra's default template renders exactly `shll version <Version>\n` to `OutOrStdout()` — the RECOMMENDED canonical `<tool> version vX.Y.Z` shape — with nothing on stderr and no banner above it.

`TestRootVersionFlag_VersionStandardConformance` (`version_test.go`) pins this producer contract, mapping one assertion per standard clause: `newRootCmd()` + `root.Version = "v1.2.3"` (the same seam `help_dump_test.go` uses), `SetArgs(["--version"])`, then asserts `Execute()` returns nil (exit 0 via `translateExit`), the first non-empty stdout line is exactly `shll version v1.2.3`, `normalizeVersion(output) == "v1.2.3"` (reusing the repo's own `versionTokenRE`/`versionPrefixRE` so shll provably parses its own output, not a hand-rolled regex), and stderr is empty. A `dev_default` subtest (`root.Version = "dev"` → first line `shll version dev`, `normalizeVersion` → `dev`) keeps unstamped builds parseable. Clauses 3–4 (respond within 2s, no network I/O) carry no in-process assertion — the path is a purely local read of the package var with no subprocess or I/O seam to fake; a test comment records that rationale.

## Cross-references

- Subprocess wrapper conventions: [internal/proc](/internal/proc.md) — including `proc.ErrNotFound` semantics.
- Roster definition: [cli/commands](/cli/commands.md#hardcoded-tool-roster).
- Brew detection (`isInstalled`) — used by `install` and `update` only, not here: [cli/update](/cli/update.md#detection).
- The shared `toolInstalled` helper's other consumer: [cli/list](/cli/list.md#the-install-probe-shared-toolinstalled) — `shll list` reuses the same `probeToolVersion` probe (as a bool) for its install-status column.
- Shared version probe: [cli/doctor](/cli/doctor.md) — `doctor`'s `probeVersion` **delegates to `probeToolVersion`** (change 9bak — inheriting the legacy-name fallback), adding only the three-way `versionState` classification that keeps the missing-vs-unreportable distinction `toolVersion` collapses; the two cannot drift.
- The shared `shllSelf` display descriptor + `shllSelfVersion()` (bb7r): [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor). `version`'s shll-first row is the pattern the descriptor generalizes to `list`/`doctor`/`install`; `version.go` itself does not consume it.
