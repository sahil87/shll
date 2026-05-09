# Plan: Normalize shll version output

**Change**: 260509-6hx0-normalize-version-output
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

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
