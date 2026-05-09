# Intake: Normalize shll version output

**Change**: 260509-6hx0-normalize-version-output
**Created**: 2026-05-09
**Status**: Draft

## Origin

Initiated from a `/fab-discuss` exploration of `shll version`'s current output. Running the command today produces:

```
shll     dev
fab-kit  fab-kit version 1.9.4
rk       rk version 1.5.2
tu       0.4.10
hop      hop version v0.1.5
wt       wt version v0.0.3
idea     idea version v0.0.2
```

Three different shapes leak through from upstream tools:

1. `<name> version <X.Y.Z>` — fab-kit, rk (no `v` prefix)
2. `<name> version v<X.Y.Z>` — hop, wt, idea, shll (with `v` prefix)
3. `<X.Y.Z>` — tu (bare)

Half the rows visually duplicate the tool name (`fab-kit  fab-kit version 1.9.4`) and the `v` prefix is inconsistent. A bug-report reader has to mentally normalize this.

The user explicitly chose to fix the inconsistency in shll itself, with **no per-tool logic** — partly because tu, rk, and fab-kit may independently standardize their own `--version` output as a separate effort, and shll must not depend on or duplicate that work. Pure shape-based extraction only.

Two specific decisions locked in during the discussion:

- **`v` prefix policy**: always-on (matches SemVer tag convention; 4 of 7 current tools already do this)
- **Unparseable fallback**: emit the raw first line (preserves information for bug reports, which is the primary use case for this command)

## Why

**The pain**: `shll version` is the bug-report-triage entry point. Today's output mixes three formats and visually repeats tool names, which makes it slower to read and less useful when pasted into an issue. A reader has to scan past `fab-kit version` and `rk version` boilerplate to see the actual numbers.

**Consequence of not fixing**: Every bug report that includes `shll version` output forces the reader through cosmetic noise. As the toolkit grows (current roster has 6 tools + shll = 7 rows), the noise compounds. The output becomes harder to diff visually between users, harder to grep, and undermines the "paste it into the issue" use case the command was built for.

**Why this approach over alternatives**:

- Fixing each tool's `--version` upstream (tu, rk, fab-kit) is the cleanest long-term answer, but it's three separate repos and three separate releases. shll can't block on that, and shouldn't — it's a meta-CLI whose job is composition.
- Adding `--json` was considered and explicitly deferred in the original scaffold intake (260508-kvan): "Bug-report use case suggests plain text is fine. JSON could be added later if a real script-consumer emerges." That decision still holds.
- A per-tool parser table (e.g., `fab-kit` → strip `fab-kit version `, `tu` → use as-is) would work but couples shll to each upstream's current format. The user explicitly rejected this — shll must stay generic so any independent upstream standardization is automatically picked up without code changes here.

The chosen approach: **shape-based extraction**. Find the first version-looking token in the line; if found, normalize its `v` prefix; otherwise fall back to a generic prefix-strip heuristic that doesn't reference any tool name.

## What Changes

### Single helper in `src/cmd/shll/version.go`

Replace the existing `firstNonEmptyLine` helper (lines 74-82) with `normalizeVersion(raw string) string`. The new helper is the single point of normalization for both the `shll` row and every roster row in `runVersion`.

Behavior — applied in order:

1. Take the first non-empty line of `raw`, trimmed of leading/trailing whitespace. If the input has no non-empty lines, return `""`.
2. Search the line for the first token matching the version regex: `v?\d+(\.\d+)*([.-][\w.+-]+)?`. Use `regexp.FindString`.
   - The regex is unanchored: `FindString` returns the first version-shaped substring it encounters, which can land inside a larger token (e.g., `go1.22.0` would yield `1.22.0`). This is acceptable for the current roster — every tool emits the version as a separate token — and tightening the boundary is captured as a known follow-up if an adversarial input ever lands in production.
   - At least one numeric component is required (so `1`, `1.2`, `1.2.3`, `v1.2.3`, `1.2.3-rc1`, `1.2.3.dev0` all match).
   - The regex is permissive on suffix (pre-release tags, build metadata) so non-strict-SemVer producers don't regress to the fallback.
3. If a token was found:
   - If it does not start with `v`, prepend `v`.
   - Return the (now `v`-prefixed) token.
4. If no version-shaped token was found, apply the **generic prefix-strip heuristic**:
   - If the line matches the pattern `<word> version <rest>` (case-insensitive on the word `version`, where `<word>` is `\S+` and `<rest>` is the remainder after the space), return `<rest>` (trimmed).
   - This heuristic does NOT reference any tool name — it strips any leading `<word> version ` regardless of what `<word>` is. It exists so `shll version dev` (when shll is built without ldflags and `version = "dev"`) collapses to `dev` instead of duplicating the binary name.
5. Otherwise return the (trimmed) first non-empty line verbatim.

The regex and the heuristic together MUST handle every current shape:

| Raw input | Step 2 token found? | Output |
|-----------|--------------------|--------|
| `fab-kit version 1.9.4` | `1.9.4` | `v1.9.4` |
| `rk version 1.5.2` | `1.5.2` | `v1.5.2` |
| `0.4.10` | `0.4.10` | `v0.4.10` |
| `hop version v0.1.5` | `v0.1.5` | `v0.1.5` |
| `wt version v0.0.3` | `v0.0.3` | `v0.0.3` |
| `idea version v0.0.2` | `v0.0.2` | `v0.0.2` |
| `shll version v0.0.1` | `v0.0.1` | `v0.0.1` |
| `dev` (raw shll var, no ldflags) | none | `dev` (no prefix-strip needed; raw first line) |
| `shll version dev` (full output when var is `dev`) | none | `dev` (via prefix-strip heuristic) |
| `some unparseable banner` | none | `some unparseable banner` (raw passthrough) |

### Apply normalization to shll's own row too

Currently the `shll` row in `runVersion` prints the raw `version` package variable directly (`fmt.Fprintf(w, "shll\t%s\n", version)`). Change this so `version` is also passed through `normalizeVersion` — that way `0.0.1` → `v0.0.1`, `v0.0.1` stays `v0.0.1`, and `dev` stays `dev` (consistent with how a hypothetical `shll --version` output would be normalized if it were a roster tool).

Note: shll's own `version` var is not formatted as `<name> version <rest>`; it's just the bare token. The version regex catches the SemVer cases, and the bare `dev` falls through to the raw-line branch correctly.

### No changes to

- `not installed` behavior — uninstalled tools or `--version` failures continue to print the literal `notInstalledLabel`. The normalization helper is only called on successful `proc.Run` output.
- The 2-second per-tool timeout (`versionTimeout`).
- Column-aligned `tabwriter` output, no ANSI, no JSON.
- Roster (`tools.go`) — unchanged.
- `internal/proc` — unchanged.
- `update`, `shell-init`, `root`, `main` — unchanged.

### Resulting `shll version` output

```
shll     v0.0.1
fab-kit  v1.9.4
rk       v1.5.2
tu       v0.4.10
hop      v0.1.5
wt       v0.0.3
idea     v0.0.2
```

(When shll is built without ldflags: row 1 reads `shll  dev`.)

## Affected Memory

- `cli/version`: (modify) — document the normalization rules: SemVer-shaped extraction, always-on `v` prefix, generic prefix-strip fallback, raw passthrough as last resort. Note explicitly that the helper is shape-based and contains no per-tool logic, so independent upstream `--version` standardization is transparently absorbed.

No other memory files are affected — this is a single-helper change with no cross-cutting impact.

## Impact

- **Code**: `src/cmd/shll/version.go` (replace `firstNonEmptyLine` with `normalizeVersion`; update `runVersion` to apply it to the shll row too). Adds `regexp` import.
- **Tests**: `src/cmd/shll/version_test.go` — extend with cases covering each input shape (see Test Coverage below). Existing tests for the all-installed/some-missing/timeout matrix continue to apply but their expected version strings need updating to the normalized form.
- **APIs**: None — `shll version` has no flags, no JSON, no programmatic consumer. The change is observable only in stdout text.
- **Dependencies**: None new (`regexp` is stdlib).
- **Sub-tools**: None — by design, shll does not touch any sub-tool's `--version` output.
- **Cross-platform**: None — pure string processing, no syscalls.

### Test Coverage

The new test cases that MUST exist in `version_test.go`:

1. `normalizeVersion("fab-kit version 1.9.4")` → `v1.9.4` (name-version-bare shape)
2. `normalizeVersion("hop version v0.1.5")` → `v0.1.5` (name-version-v-prefixed shape)
3. `normalizeVersion("0.4.10")` → `v0.4.10` (bare shape)
4. `normalizeVersion("dev")` → `dev` (passthrough, no version token, no prefix to strip)
5. `normalizeVersion("shll version dev")` → `dev` (prefix-strip heuristic kicks in)
6. `normalizeVersion("banner line\n\nfab-kit version 1.9.4\n")` → `v1.9.4` (multi-line: first non-empty line is parsed; later lines are not searched if the first line yields a token... but if the first non-empty line is the banner with no version token, fallback uses the prefix-strip heuristic on that first line, NOT a deeper search — keep semantics simple and predictable)

   Sub-case 6a: `normalizeVersion("MyTool — the swiss army knife\n0.4.10\n")` → `MyTool — the swiss army knife` (raw first line passthrough, because the first non-empty line has no version token and doesn't match the prefix-strip pattern). This documents that we deliberately don't scan past the first non-empty line — keeps the contract predictable.

7. `normalizeVersion("some unparseable banner")` → `some unparseable banner` (raw passthrough — no version token, no `<word> version <rest>` match)
8. `normalizeVersion("")` → `""` (empty input)
9. `normalizeVersion("\n\n  \n")` → `""` (whitespace-only)
10. The integration test for `runVersion` (existing all-installed test) MUST update its expected output to use normalized values, AND assert that the `shll` row is also normalized when the test sets the package-level `version` var.

## Open Questions

None. The discussion locked in `v`-prefix-always-on, raw-line fallback, ≥1-component regex, and no-per-tool-logic. The helper signature, regex, and heuristic are concrete and testable.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Normalization happens in `shll`, not by changing upstream tool `--version` output | Discussed — user explicitly stated tu/rk/fab-kit may standardize independently in parallel; shll must not depend on or duplicate that work | S:95 R:90 A:95 D:90 |
| 2 | Certain | No per-tool logic in `normalizeVersion` — purely shape-based | Discussed — user explicitly chose this so independent upstream standardization is transparently absorbed without shll code changes | S:95 R:85 A:95 D:90 |
| 3 | Certain | `v` prefix is always-on (added if absent) | Discussed — user chose option 1 over option 2 (always-off). Matches SemVer tag convention and 4 of 7 current tools | S:95 R:80 A:95 D:95 |
| 4 | Certain | Unparseable input falls back to raw first non-empty line | Discussed — user chose option 1 over `unknown` placeholder. Preserves information for bug reports, the primary use case | S:95 R:85 A:95 D:90 |
| 5 | Certain | Generic `<word> version <rest>` prefix-strip heuristic for the unparseable branch | I proposed this in discussion to fix the `shll version dev` duplication; user accepted with "your choice good for both". Heuristic is generic — no tool names | S:90 R:80 A:90 D:85 |
| 6 | Certain | Version regex permits ≥1 numeric component (`v?\d+(\.\d+)*([.-][\w.+-]+)?`) | I proposed permissive matching in discussion (handles `1.2`, `1.2.3-rc1`, `0.4.10` alike); user accepted with "your choice good for both" | S:90 R:80 A:90 D:80 |
| 7 | Certain | Apply normalization to the `shll` row too (not just roster rows) | Codebase signal — current code (`version.go:47`) treats shll's row specially with raw `version` var. Normalizing it makes the table uniformly formatted, which is the entire point of this change | S:90 R:90 A:95 D:90 |
| 8 | Certain | `not installed` behavior unchanged — only successful `--version` output is normalized | Constitution V (Graceful Degradation) and existing behavior. Normalization is a presentation concern; presence detection is a correctness concern | S:95 R:90 A:95 D:95 |
| 9 | Certain | No new flags — `shll version` continues to take no args, no `--json` | Original scaffold intake (260508-kvan #9) explicitly deferred JSON until a script-consumer emerges. Still no consumer | S:95 R:75 A:90 D:90 |
| 10 | Confident | Helper name is `normalizeVersion(raw string) string`, replacing `firstNonEmptyLine` | Codebase signal — short Go-idiomatic name; replaces a helper of similar scope. Minor; rename has no external effect | S:75 R:90 A:85 D:80 |
| 11 | Confident | Multi-line input: only the first non-empty line is parsed for a version token; deeper lines are not searched | Predictable contract — keeps the helper's behavior explainable. If a tool puts the version on line 2, we'd rather emit the banner verbatim than guess. Documented in test case 6a | S:65 R:70 A:80 D:65 |
| 12 | Confident | `regexp.FindString` (not `FindStringSubmatch`) — first match wins | Simplest API for "first version-shaped token"; no capture groups needed. Compiled once via `regexp.MustCompile` at package level | S:75 R:90 A:85 D:80 |

12 assumptions (9 certain, 3 confident, 0 tentative, 0 unresolved).
