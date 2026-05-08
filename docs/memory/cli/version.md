# cli/version

`shll version` — prints a column-aligned plain-text table with the version of `shll` itself plus every roster tool.

Source: `cmd/shll/version.go`. Uses the shared brew helpers in `brew.go` and the `Roster` from `tools.go`.

## Output shape

```
shll      v0.1.0
fab-kit   v0.4.2
rk        v0.7.1
tu        v0.2.0
hop       v0.0.3
wt        v0.1.5
idea      not installed
```

- Exactly **7 rows**: one for `shll`, then one per roster tool in roster order (`fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea`).
- Column-aligned via `text/tabwriter` (`version.go:46`) — minwidth 0, tabwidth 0, padding 2, padchar space, no flags.
- **Plain text only.** No ANSI escapes, no JSON, no colors. The output is meant to paste cleanly into bug reports.

## Behavior contract

`runVersion(ctx, stdout)` (`version.go:42`) is the implementation seam:

1. Construct a `tabwriter.Writer` over stdout.
2. Write `shll\t<version>\n` first, where `version` is the package-level variable (see Ldflags injection below).
3. For each tool in `Roster` (in order), write `<tool.Name>\t<toolVersion(ctx, tool)>\n`.
4. `w.Flush()` — propagates any write error up.

`toolVersion(ctx, tool)` (`version.go:58`) is the per-tool resolver:

1. If `!isInstalled(ctx, tool.Formula)` → return `notInstalledLabel = "not installed"`.
2. Else create `subCtx, cancel := context.WithTimeout(ctx, versionTimeout)`. Defer cancel.
3. Run `proc.Run(subCtx, tool.Name, "--version")` (capture transport).
4. On any error (transport error, exit non-zero, deadline exceeded) → return `notInstalledLabel`.
5. On success → return `firstNonEmptyLine(string(out))` — the first non-blank line trimmed of surrounding whitespace.

`firstNonEmptyLine` exists because some tools emit multi-line `--version` output (banner + version). The table only shows the first useful line per tool.

## Ldflags injection (shll's own version)

The `shll` row's version comes from the package-level `version = "dev"` declared in `main.go:18`. Build behavior:

- Default (uninjected): `dev`. Covers `go run` and unstamped local builds.
- Stamped: `scripts/build.sh` invokes `go build -ldflags "-X main.version=${VERSION}" ...`, where `VERSION=$(git describe --tags --always 2>/dev/null || echo dev)`.

Tests override the variable directly (`TestVersion_LdflagsInjection`) — no special build hook needed for testing.

## Per-tool timeout

`versionTimeout = 2 * time.Second` (`version.go:19`) — a named constant; magic numbers are forbidden by `code-quality.md`.

Properties (Design Decision #5):

- 2s is generous (typical `--version` runs in well under 100ms).
- Bounds worst-case `shll version` runtime to `len(Roster) * versionTimeout` ≈ 12 seconds even if every tool hangs.
- A timeout is treated as "not installed" — we don't differentiate hung-but-installed from missing in the output. The user gets a usable table either way.
- The deadline applies only to the `--version` invocation, not to the `brew list` probe. The probe runs against the parent ctx (typically unbounded for the CLI invocation).

`TestVersion_TimeoutHandling` simulates `context.DeadlineExceeded` by having the fake runner block until the sub-context's deadline fires, then asserts the row reads `not installed`.

## Spec-locked Design Decisions for this subcommand

### #4 Plain-text output, no `--json`

> *Why*: Primary use case is bug reports — pasting output into a Slack thread or GitHub issue. Plain text is universally legible.
> *Rejected*: `--json` flag for v0.1.0. Add later if a real script-consumer emerges; YAGNI for now.

### #5 Per-tool `--version` invocations have a 2-second timeout

> *Why*: Protects against deadlocked sub-tools. 2s is generous for `--version` (typical < 100ms) but bounded enough that worst-case `shll version` finishes in under 15 seconds even if every roster tool hangs.
> *Rejected*: no timeout (one bad tool blocks the whole command); 500ms (too aggressive — some tools may legitimately take longer on a cold start, especially on macOS first-run gatekeeper checks).

## Test seam

`version_test.go` installs a fake via `installFakeRunner(t, f)` and uses helper builders like `versionFake(installed map, versions map)` to canned-respond per-tool.

Covered scenarios:

- `TestVersion_AllInstalled` — seven rows in roster order, column-aligned.
- `TestVersion_SomeMissing` — `idea` not installed → row reads `idea  not installed`.
- `TestVersion_LdflagsInjection` — overrides `version` package var → `shll` row reflects it.
- `TestVersion_DefaultDev` — leaves `version` at `"dev"` → `shll` row reads `dev`.
- `TestVersion_TimeoutHandling` — fake blocks until ctx deadline → row reads `not installed`.
- `TestVersion_NoANSI` — asserts no `\x1b[` escape in output.

## Cross-references

- Subprocess wrapper conventions: [internal/proc](../internal/proc.md).
- Roster definition: [cli/commands](commands.md#hardcoded-tool-roster).
- Brew detection (`isInstalled`): [cli/update](update.md#detection).
