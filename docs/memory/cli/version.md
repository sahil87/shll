# cli/version

`shll version` — prints a column-aligned plain-text table with the version of `shll` itself plus every roster tool.

Source: `src/cmd/shll/version.go`. Uses the shared brew helpers in `src/cmd/shll/brew.go` and the `Roster` from `src/cmd/shll/tools.go`.

## Output shape

```
shll      v0.1.0
wt        v0.1.0
idea      not installed
tu        v0.1.0
rk        v0.1.0
hop       v0.1.0
fab-kit   v0.1.0
```

- Exactly **7 rows**: one for `shll`, then one per roster tool in roster order (`wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit` — the leaves-first order, change auvj). `version` output is order-agnostic in test (assertions are index-paired to `Roster`, so reorder moves expected and actual in lockstep — no `version_test.go` change was needed for the reorder); only this example's ordering reflects the slice. See [cli/commands](commands.md#design-decision-leaves-first-roster-order-change-auvj).
- Column-aligned via `text/tabwriter` (`src/cmd/shll/version.go:56`) — minwidth 0, tabwidth 0, padding 2, padchar space, no flags.
- When the upstream tool's `--version` output contains a SemVer-shaped token, the row is normalized to a `v`-prefixed token (e.g. `v1.9.4`). When no such token is present, the row falls through the prefix-strip and raw-passthrough branches and may emit a non-`v` string (e.g. `dev`, or an unparseable banner verbatim) — see the `normalizeVersion` pipeline below for the full contract.
- **Plain text only.** No ANSI escapes, no JSON, no colors. The output is meant to paste cleanly into bug reports.

## Behavior contract

`runVersion(ctx, stdout)` (`src/cmd/shll/version.go:52`) is the implementation seam. The `runVersion`/`toolVersion` output contract — including `normalizeVersion`, the `not installed` label, the per-tool timeout, and the plain-text-no-JSON shape — is **unchanged by change lst7**: that change only extracted the probe into a shared helper (see [The shared install probe](#the-shared-install-probe-change-lst7)). It is:

1. Construct a `tabwriter.Writer` over stdout.
2. Write `shll\t<normalizeVersion(version)>\n` first, where `version` is the package-level variable (see Ldflags injection below). The shll row goes through the same normalizer as roster rows, so the column is uniform.
3. For each tool in `Roster` (in order), write `<tool.Name>\t<toolVersion(ctx, tool)>\n`.
4. `w.Flush()` — propagates any write error up.

`toolVersion(ctx, tool)` (`src/cmd/shll/version.go:101`) is the per-tool resolver:

1. Call `probeToolVersion(ctx, tool)` — the shared probe (see [The shared install probe](#the-shared-install-probe-change-lst7) below), which runs `proc.Run(subCtx, tool.Name, "--version")` under a `versionTimeout` deadline and returns `([]byte, error)`.
2. On any error (`proc.ErrNotFound` for missing binary, exit non-zero, deadline exceeded, etc.) → return `notInstalledLabel = "not installed"`.
3. On success → return `normalizeVersion(string(out))`.

"Installed" is detected via `proc.ErrNotFound` (binary not on PATH) rather than a brew probe — install-mechanism agnostic, and saves ~400ms per tool (no Homebrew/Ruby startup tax).

`normalizeVersion(raw string) string` (`src/cmd/shll/version.go:121`) is the single point of normalization shared by the shll row and every roster row. It is purely shape-based — there is no per-tool branching — so independent upstream `--version` standardization (e.g., tu/rk/fab-kit cleaning up their own output in parallel) is absorbed without shll code changes.

The normalization pipeline runs in this order on the input:

1. **First non-empty line.** Split on `\n`, find the first line whose `strings.TrimSpace` is non-empty, use that trimmed value. Empty / whitespace-only input returns `""`.
2. **Version-token regex.** Search the line for the first match of `versionTokenRE = v?\d+(\.\d+)*([.-][\w.+-]+)?` (`src/cmd/shll/version.go:30`). The token requires at least one numeric component; additional `.`-separated numerics and an optional `[.-]<suffix>` (pre-release / build metadata) are accepted, so `1`, `1.2`, `1.2.3`, `v1.2.3`, `1.2.3-rc1`, `1.2.3-rc1+build.42` all match. If a token is found, return it with a `v` prepended when absent (existing `v` is retained, never doubled).
3. **Generic prefix-strip heuristic.** If no version token was found, match the line against `versionPrefixRE = ^\S+\s+(?i:version)\s+(.+)$` (`src/cmd/shll/version.go:34`). The literal word `version` is case-insensitive (so `<word> Version <rest>` and `<word> version <rest>` are handled identically). On match, return the trimmed `<rest>` capture. The heuristic does NOT reference any tool name — it strips a leading `<word> version ` prefix regardless of what `<word>` is, which collapses `shll version dev` to `dev` without per-tool logic.
4. **Raw passthrough.** Otherwise, return the trimmed first non-empty line verbatim. This preserves whatever the tool emitted for the bug-report use case — losing information would be worse than displaying an unparseable banner.

The `v` prefix is **always-on**: matched tokens that lack `v` get one prepended; matched tokens that already start with `v` are returned unchanged. This matches SemVer tag convention and yields a uniform column.

The parser is **first-line-only**. It never scans deeper lines for a version token — even when the first non-empty line falls through to the raw-passthrough branch. If a tool puts a banner on line 1 and the version on line 2, the banner wins. The contract is predictable and testable as a single string-equality assertion.

The two regexes are compiled once via `regexp.MustCompile` at package scope; they are not recompiled per call.

## The shared install probe (change lst7)

The install probe is now a **shared helper** extracted from `toolVersion`, so `version` is no longer the sole definition of "installed = runnable on PATH":

- `probeToolVersion(ctx, tool) ([]byte, error)` (`src/cmd/shll/version.go:72`) is the **single** definition of the probe: it creates `subCtx, cancel := context.WithTimeout(ctx, versionTimeout)` (defer cancel), runs `proc.Run(subCtx, tool.Name, "--version")` (capture transport, Constitution I), and returns the captured output and any error. ANY error (`proc.ErrNotFound`, non-zero exit, timeout) means "not installed" — callers map that to their own representation.
- `toolInstalled(ctx, tool) bool` (`src/cmd/shll/version.go:85`) layers on `probeToolVersion` and returns `err == nil`. This is the boolean install-status helper consumed by `shll list` — see [cli/list §The install probe](list.md#the-install-probe-shared-toolinstalled).
- `toolVersion` now also layers on `probeToolVersion` (mapping a non-nil error to `notInstalledLabel`, success to `normalizeVersion`).

So there is **exactly one place** that defines "installed = runnable", shared today by `version` (string label) and `list` (bool), and reserved for a future `doctor`. This is the install-mechanism-agnostic notion — **NOT** the brew `isInstalled` probe (`src/cmd/shll/brew.go`) used by `install`/`update`.

**The extraction was behavior-preserving.** `shll version`'s output (the `not installed` label, the `versionTimeout` bound, the column layout, the plain-text-no-JSON shape) is byte-for-byte identical. `version_test.go` passes **unchanged** — all six `TestVersion_*` integration tests and the 12 `TestNormalizeVersion_*` unit tests required no edit.

## Ldflags injection (shll's own version)

The `shll` row's version comes from the package-level `version = "dev"` declared in `src/cmd/shll/main.go:18`, then passed through `normalizeVersion`. Build behavior:

- Default (uninjected): raw `dev` → normalized `dev`. Covers `go run` and unstamped local builds.
- Stamped: `scripts/build.sh` invokes `go build -ldflags "-X main.version=${VERSION}" ...`, where `VERSION=$(git describe --tags --always 2>/dev/null || echo dev)`. A stamped `v0.0.1` stays `v0.0.1`; a stamped bare `0.0.1` becomes `v0.0.1`.

Tests override the variable directly (`TestVersion_LdflagsInjection`) — no special build hook needed for testing.

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

Unit scenarios pinning the normalization contract (12 cases, all named `TestNormalizeVersion_*`):

- `_NamePrefixedBare` (`fab-kit version 1.9.4` → `v1.9.4`), `_NamePrefixedV` (`hop version v0.1.5` → `v0.1.5`, no doubling), `_Bare` (`0.4.10` → `v0.4.10`).
- `_BareDev` (`dev` → `dev`), `_NamePrefixedDev` (`shll version dev` → `dev` via prefix-strip), `_Unparseable` (raw passthrough).
- `_Empty` (`""` and whitespace-only → `""`), `_FirstLineOnly` (banner on line 1 wins; line 2 never searched), `_BlankLeadingLines` (leading blanks skipped to find the first non-empty line).
- `_PermissiveSemVer` (`1.2` and `1.2.3-rc1+build.42`), `_CaseInsensitiveVersionWord` (`MyTool Version 1.0` → `v1.0`), `_PrefixStripCase` (`shll Version dev` → `dev`).

## Cross-references

- Subprocess wrapper conventions: [internal/proc](../internal/proc.md) — including `proc.ErrNotFound` semantics.
- Roster definition: [cli/commands](commands.md#hardcoded-tool-roster).
- Brew detection (`isInstalled`) — used by `install` and `update` only, not here: [cli/update](update.md#detection).
- The shared `toolInstalled` helper's other consumer: [cli/list](list.md#the-install-probe-shared-toolinstalled) — `shll list` reuses the same `probeToolVersion` probe (as a bool) for its install-status column.
