# Plan: Normalize shll version output

**Change**: 260509-6hx0-normalize-version-output
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- Adding a `--json` flag — explicitly deferred in the original scaffold intake (260508-kvan #9); no script-consumer has emerged.
- Modifying any sub-tool's own `--version` output — that is a separate, parallel effort across tu/rk/fab-kit and shll MUST NOT depend on, duplicate, or block on it.
- Changing `not installed` semantics — uninstalled tools and `--version` failures keep the existing `notInstalledLabel` literal.
- Changing the `versionTimeout` (2s), tabwriter formatting, roster, or any of the `update`/`shell-init`/`root` subcommands.
- Discovering a version on lines beyond the first non-empty line. Predictable contract: only the first non-empty line is parsed.

## cli/version: Output normalization

### Requirement: Shape-based version extraction

`shll version` SHALL extract the displayed version for each successfully-probed tool by parsing the tool's `--version` stdout for a SemVer-shaped token. The parser MUST be purely shape-based — it MUST NOT branch on tool names, formula names, or any other tool-identifying value.

The recognized version-token shape is the regex `v?\d+(\.\d+)*([.-][\w.+-]+)?`. The token requires at least one numeric component; additional `.`-separated numeric components and an optional `[.-]<suffix>` (pre-release / build-metadata) are accepted.

#### Scenario: Name-prefixed bare version (rk, fab-kit pre-standardization shape)

- **GIVEN** a tool whose `--version` stdout is `fab-kit version 1.9.4\n`
- **WHEN** `shll version` runs and the tool is installed
- **THEN** the tool's row in the table SHALL display `v1.9.4`
- **AND** the displayed version SHALL NOT contain the substring `fab-kit version`

#### Scenario: Name-prefixed v-prefixed version (hop, wt, idea, shll standard shape)

- **GIVEN** a tool whose `--version` stdout is `hop version v0.1.5\n`
- **WHEN** `shll version` runs and the tool is installed
- **THEN** the tool's row SHALL display `v0.1.5`
- **AND** the displayed token SHALL retain the existing `v` prefix without doubling (no `vv0.1.5`)

#### Scenario: Bare version (tu pre-standardization shape)

- **GIVEN** a tool whose `--version` stdout is `0.4.10\n`
- **WHEN** `shll version` runs and the tool is installed
- **THEN** the tool's row SHALL display `v0.4.10`

#### Scenario: Permissive numeric component count

- **GIVEN** a hypothetical tool whose `--version` stdout is `mytool version 1.2\n`
- **WHEN** `shll version` runs
- **THEN** the row SHALL display `v1.2`
- **AND** GIVEN a hypothetical tool emitting `mytool version 1.2.3-rc1+build.42\n`, THEN the row SHALL display `v1.2.3-rc1+build.42`

### Requirement: Always-on `v` prefix

The displayed version token MUST be prefixed with `v` if the matched token does not already begin with `v`. The parser MUST NOT strip an existing `v` prefix; it MUST NOT add a `v` to a token already prefixed with `v`.

#### Scenario: Prefix added when absent

- **GIVEN** matched token `1.9.4`
- **WHEN** the parser emits the displayed value
- **THEN** the displayed value SHALL be `v1.9.4`

#### Scenario: Prefix retained when present

- **GIVEN** matched token `v0.1.5`
- **WHEN** the parser emits the displayed value
- **THEN** the displayed value SHALL be `v0.1.5` (no doubling)

### Requirement: Generic prefix-strip fallback when no version token is found

When the first non-empty line contains no token matching the version-token shape, the parser SHALL apply a single generic heuristic: if the line matches the pattern `^\S+\s+version\s+(?P<rest>.+)$` (the literal word `version`, case-insensitive, between two whitespace-separated segments), the parser SHALL emit the captured `<rest>` portion (trimmed).

This heuristic SHALL NOT reference any tool name. It strips a leading `<word> version ` prefix regardless of what `<word>` is.

#### Scenario: Dev build with name-prefixed line

- **GIVEN** a tool emits `shll version dev\n` (the literal token `dev` is not version-shaped, so it falls through the version-extraction path; the line matches `<word> version <rest>`)
- **WHEN** the parser processes this line
- **THEN** the displayed value SHALL be `dev`
- **AND** the displayed value SHALL NOT contain the substring `shll version`

### Requirement: Raw-line passthrough when neither rule matches

When the first non-empty line contains no version-shaped token AND does not match the prefix-strip heuristic, the parser SHALL emit the trimmed first non-empty line verbatim.

#### Scenario: Bare unparseable line

- **GIVEN** a tool emits `some unparseable banner\n`
- **WHEN** the parser processes this line
- **THEN** the displayed value SHALL be `some unparseable banner`

#### Scenario: Bare `dev` (no name prefix)

- **GIVEN** a tool emits `dev\n`
- **WHEN** the parser processes this line
- **THEN** the displayed value SHALL be `dev` (the prefix-strip heuristic does not match a line with only one whitespace-delimited token)

#### Scenario: Empty input

- **GIVEN** a tool emits `""` or whitespace-only output
- **WHEN** the parser processes this output
- **THEN** the displayed value SHALL be `""` (empty)

### Requirement: First-line-only parsing

When the input contains multiple lines, only the first non-empty line SHALL be parsed. The parser MUST NOT scan deeper lines for a version token. This SHALL hold even when the first non-empty line yields the raw-passthrough branch — the parser SHALL NOT then search line 2 for a version token.

#### Scenario: Banner on line 1, version on line 2

- **GIVEN** a tool emits `MyTool — the swiss army knife\n0.4.10\n`
- **WHEN** the parser processes the input
- **THEN** the displayed value SHALL be `MyTool — the swiss army knife`
- **AND** the displayed value SHALL NOT be `v0.4.10`

#### Scenario: Blank lines before content

- **GIVEN** a tool emits `\n\nfab-kit version 1.9.4\n`
- **WHEN** the parser processes the input
- **THEN** the displayed value SHALL be `v1.9.4` (leading blank lines are skipped to find the first non-empty line; that line is then parsed)

### Requirement: Apply normalization to the `shll` row

The `shll` row in the version table SHALL pass shll's own `version` package variable through the same normalization helper used for roster rows. This includes the ldflags-injected build (`v0.0.1` → `v0.0.1`), an unprefixed injected version (`0.0.1` → `v0.0.1`), and the default unstamped value (`dev` → `dev`).

#### Scenario: Stamped shll build

- **GIVEN** the package variable `version` has been injected as `v0.0.1` via ldflags
- **WHEN** `shll version` runs
- **THEN** the `shll` row SHALL display `v0.0.1` (no doubling)

#### Scenario: Unstamped shll build

- **GIVEN** the package variable `version` is the default `dev`
- **WHEN** `shll version` runs
- **THEN** the `shll` row SHALL display `dev`

#### Scenario: Unprefixed injection

- **GIVEN** the package variable `version` has been injected as `0.0.1` (no `v`)
- **WHEN** `shll version` runs
- **THEN** the `shll` row SHALL display `v0.0.1`

### Requirement: `not installed` behavior unchanged

The normalization helper SHALL only be applied to successful `proc.Run` output. When `isInstalled` returns `false` for a tool, OR when `proc.Run` returns an error (transport, non-zero exit, deadline exceeded), the row SHALL display the literal `notInstalledLabel` (`not installed`). Normalization SHALL NOT alter this code path.

#### Scenario: Tool not installed

- **GIVEN** a roster tool whose formula is not installed
- **WHEN** `shll version` runs
- **THEN** the row SHALL display `not installed` (unchanged)

#### Scenario: `--version` times out

- **GIVEN** a roster tool whose `--version` invocation exceeds `versionTimeout`
- **WHEN** `shll version` runs
- **THEN** the row SHALL display `not installed` (unchanged)

### Requirement: No new flags, no JSON, no ANSI

`shll version` SHALL continue to take no arguments and emit no ANSI escape sequences. The output SHALL remain column-aligned plain text via `text/tabwriter`.

#### Scenario: Output is plain text

- **WHEN** `shll version` runs in any context
- **THEN** the output SHALL NOT contain any ANSI escape sequence (`\x1b[...`)
- **AND** the output SHALL NOT contain JSON braces, brackets, or quoted-string syntax characteristic of structured formats

## Design Decisions

1. **Shape-based extraction over per-tool table**: The parser is purely regex-and-heuristic, with no map of `tool.Name → format-specific stripper`.
   - *Why*: Sub-tools (tu, rk, fab-kit) may independently standardize their `--version` output as a parallel effort; shll must transparently absorb whatever they produce without code changes here. A per-tool table would couple shll to each upstream's current format and require shll updates whenever an upstream format changes.
   - *Rejected*: per-tool stripper map — couples shll to upstream formats, requires synchronized releases, and creates subtle bugs when an upstream changes shape between releases.

2. **Always-on `v` prefix in the displayed value**: When the matched version token does not start with `v`, prepend one.
   - *Why*: Matches SemVer tag convention and 4 of 7 current toolkit tools (hop, wt, idea, shll). Consistent prefix makes the column scannable.
   - *Rejected*: always-off (strip the `v`) — the column would be cleaner-looking but breaks the "matches the git tag" intuition that 4 tools already provide.

3. **Generic `<word> version <rest>` prefix-strip heuristic** (not a per-tool name match): When no version-shaped token is found, emit the substring after `<word> version ` if that pattern is present.
   - *Why*: Solves the `shll version dev` self-duplication case (`shll` row would otherwise read `shll version dev`) without introducing tool-specific logic. Generic over `<word>` so future tools printing `<name> version <non-semver-tag>` (e.g., a dev build with a date or git short SHA) collapse correctly.
   - *Rejected*: special-casing `shll` only — re-introduces a per-tool branch via the back door.

4. **Permissive version regex (≥1 numeric component, suffix-tolerant)**: `v?\d+(\.\d+)*([.-][\w.+-]+)?` rather than strict 3-component SemVer.
   - *Why*: Some tools may emit 2-component versions (`1.2`); pre-release tags vary widely (`-rc1`, `+build.42`, `.dev0`). A permissive regex absorbs these without falling to the raw-passthrough branch unnecessarily.
   - *Rejected*: strict `v?\d+\.\d+\.\d+` — too narrow; would push 2-component versions and uncommon pre-release shapes to the unparseable branch.

5. **First-non-empty-line-only parsing**: Do not search line 2 for a version token if line 1 is unparseable.
   - *Why*: Predictable, easy-to-explain contract. If a tool's first line is a banner and version is on line 2, the user sees the banner — that signal is more useful than guessing. The contract is also testable as a single string-equality assertion.
   - *Rejected*: search-all-lines-for-first-version-token — opaque behavior, harder to reason about, and a tool could put a non-version-version token (a date, a build hash matching the regex) on a later line and surprise the reader.

6. **`regexp.MustCompile` at package scope**: Compile the version regex once at init time, not per call.
   - *Why*: Standard Go idiom for fixed regexes; avoids per-call allocation in a function called once per roster tool per `shll version` invocation.
   - *Rejected*: lazy compile / `sync.Once` — overkill for a regex of this size; the package-scope `var` is the idiomatic choice.

## Tasks

<!-- Sequential work items for the apply stage. Checked off [x] as completed. -->

### Phase 2: Core Implementation

- [x] T001 Replace `firstNonEmptyLine` with `normalizeVersion(raw string) string` in `src/cmd/shll/version.go`. Add `regexp` import. Compile two regexes once at package scope: `var versionTokenRE = regexp.MustCompile(\`v?\d+(\.\d+)*([.-][\w.+-]+)?\`)` (the version-token shape) and `var versionPrefixRE = regexp.MustCompile(\`^\S+\s+(?i:version)\s+(.+)$\`)` (the generic `<word> version <rest>` prefix-strip). Implement `normalizeVersion`: (1) take first non-empty line trimmed of whitespace; if no non-empty line, return `""`; (2) `versionTokenRE.FindString(line)` — if found, prepend `v` if absent and return; (3) else apply prefix-strip via `versionPrefixRE.FindStringSubmatch` and return the captured `<rest>` trimmed; (4) else return the trimmed first non-empty line verbatim.
- [x] T002 In `runVersion` in `src/cmd/shll/version.go`, route shll's own row through `normalizeVersion(version)` instead of writing the raw `version` package var. The shll row becomes uniformly formatted with roster rows.
- [x] T003 Update existing tests in `src/cmd/shll/version_test.go` so assertions reflect normalized output: `TestVersion_AllInstalled` keeps `v0.1.0` per-row substring (input `tool.Name + " v0.1.0"` normalizes to `v0.1.0`) and the `shll` row substring stays `v9.9.9` (input `"v9.9.9"` is already prefixed; normalizer keeps it). `TestVersion_LdflagsInjection` (input `v1.2.3-test`) keeps its existing assertion. `TestVersion_DefaultDev` (input `dev`) keeps `dev` as expected substring.
- [x] T004 Add new `normalizeVersion` unit tests in `src/cmd/shll/version_test.go` (single-file, idiomatic for this repo): `TestNormalizeVersion_NamePrefixedBare` (`fab-kit version 1.9.4\n` → `v1.9.4`), `TestNormalizeVersion_NamePrefixedV` (`hop version v0.1.5\n` → `v0.1.5`, no doubling), `TestNormalizeVersion_Bare` (`0.4.10\n` → `v0.4.10`), `TestNormalizeVersion_BareDev` (`dev` → `dev`), `TestNormalizeVersion_NamePrefixedDev` (`shll version dev\n` → `dev`, prefix-strip), `TestNormalizeVersion_Unparseable` (`some unparseable banner` → unchanged), `TestNormalizeVersion_Empty` (`""` → `""`; `\n\n  \n` → `""`), `TestNormalizeVersion_FirstLineOnly` (`MyTool — the swiss army knife\n0.4.10\n` → first line verbatim, line 2 NOT searched), `TestNormalizeVersion_BlankLeadingLines` (`\n\nfab-kit version 1.9.4\n` → `v1.9.4`), `TestNormalizeVersion_PermissiveSemVer` (`mytool version 1.2` → `v1.2`; `mytool version 1.2.3-rc1+build.42` → `v1.2.3-rc1+build.42`), `TestNormalizeVersion_CaseInsensitiveVersionWord` (`MyTool Version 1.0` → `v1.0`), `TestNormalizeVersion_PrefixStripCase` (`shll Version dev` → `dev`).

### Phase 4: Polish

- [x] T005 Run `cd src && go vet ./...` and the test suite (`cd src && go test ./...`). All checks must pass.

## Acceptance

<!-- Declarative acceptance criteria used by the review stage. -->

### Functional Completeness

- [x] A-001 Shape-based version extraction: `normalizeVersion` extracts the first version-token-shaped substring (`v?\d+(\.\d+)*([.-][\w.+-]+)?`) from the first non-empty line of the input, with no per-tool branching.
- [x] A-002 Always-on `v` prefix: extracted tokens that lack a leading `v` get one prepended; tokens already starting with `v` are returned unchanged (no doubling).
- [x] A-003 Generic prefix-strip fallback: when no version-shaped token is found, lines matching `<word> <version> <rest>` (the literal word `version`, case-insensitive) emit the trimmed `<rest>`. The heuristic does not reference any tool name.
- [x] A-004 Raw-line passthrough: when neither extraction nor prefix-strip applies, the trimmed first non-empty line is emitted verbatim.
- [x] A-005 First-line-only parsing: `normalizeVersion` only inspects the first non-empty line; subsequent lines are never searched even when the first line yields the raw-passthrough branch.
- [x] A-006 Apply normalization to the shll row: `runVersion` passes the package-level `version` variable through `normalizeVersion` so the shll row is uniform with roster rows (`v0.0.1` → `v0.0.1`, `0.0.1` → `v0.0.1`, `dev` → `dev`).
- [x] A-007 `not installed` behavior unchanged: `normalizeVersion` is only called on successful `proc.Run` output; uninstalled tools and `--version` errors continue to display the literal `notInstalledLabel`.
- [x] A-008 No new flags or output formats: `shll version` still takes no arguments and emits no ANSI escapes; output remains plain text via `tabwriter`.

### Edge Cases & Error Handling

- [x] A-009 Empty / whitespace-only input returns `""`.
- [x] A-010 Permissive numeric component count: 2-component versions (`1.2`) and rich pre-release/build-metadata suffixes (`1.2.3-rc1+build.42`) are matched and emitted with `v` prefix.

### Code Quality

- [x] A-011 Pattern consistency: new helper follows the package's existing naming and structural conventions (lowercase exported-only-when-needed, short Go-idiomatic name, consistent with `firstNonEmptyLine`'s prior shape).
- [x] A-012 No unnecessary duplication: existing utilities (`strings.Split`, `strings.TrimSpace`) are reused; no reinvented helpers.
- [x] A-013 No magic strings: the version regex and prefix-strip regex are named `var` declarations at package scope, compiled once via `regexp.MustCompile`.
- [x] A-014 Readability over cleverness: `normalizeVersion`'s control flow is a linear sequence of clearly-labelled steps (first non-empty line → token search → prefix-strip → raw passthrough); no clever one-liners.
- [x] A-015 Test integrity: tests assert the normalized contract from `spec.md`, not reverse-engineer the implementation; existing tests are updated to assert on normalized values.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
