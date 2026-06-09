# Plan: `shll list` — toolkit roster command

**Change**: 260609-lst7-list-toolkit-roster
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### Roster: Single-Sourced Metadata

#### R1: Tool struct carries Description and Repo
The `Tool` struct in `src/cmd/shll/tools.go` SHALL gain two fields — `Description` (a one-line human-readable summary) and `Repo` (the `github.com/sahil87/<Repo>` slug) — so the roster single-sources everything `shll list` prints. The `Roster` slice MUST populate both fields for all six tools. `rk`'s `Repo` MUST be `"run-kit"`; every other tool's `Repo` MUST equal its `Name`. There MUST be no parallel list of descriptions/repos.

- **GIVEN** the `Roster` slice
- **WHEN** a consumer reads any `Tool`
- **THEN** `Description` is a non-empty one-line summary
- **AND** `Repo` is non-empty, equal to `Name` for all tools except `rk`, whose `Repo` is `run-kit`

#### R2: GitHub org base is a named constant
The GitHub org base URL (`https://github.com/sahil87/`) SHALL be a named constant in `src/cmd/shll/tools.go`, not an open-coded string at any call site (code-quality.md: no magic strings).

- **GIVEN** the need to build a tool's repo URL
- **WHEN** any code constructs `https://github.com/sahil87/<Repo>`
- **THEN** it concatenates the named constant with `tool.Repo`, never a literal URL prefix

### Probe: Shared Install Helper

#### R3: Shared toolInstalled helper
A reusable helper `toolInstalled(ctx context.Context, tool Tool) bool` SHALL exist in `src/cmd/shll/version.go`. It MUST report install status by invoking `<tool> --version` via `proc.Run` (routed through `internal/proc`, Constitution I), bounded by the existing `versionTimeout`, treating ANY error (`proc.ErrNotFound`, non-zero exit, timeout) as not-installed. This is the install-mechanism-agnostic notion of "installed" — NOT the brew `isInstalled` probe.

- **GIVEN** a roster tool whose `--version` succeeds
- **WHEN** `toolInstalled` is called
- **THEN** it returns true
- **AND** GIVEN a tool whose `--version` returns any error, it returns false

#### R4: toolVersion layers on toolInstalled (behavior-preserving)
`toolVersion` SHALL be refactored to share the single `proc.Run(ctx, tool.Name, "--version")` call defining "installed = runnable", so there is exactly one place that defines the probe. The refactor MUST be behavior-preserving: `version_test.go` MUST continue to pass unchanged, and `shll version` output (including the `not installed` label and `versionTimeout` bound) MUST be identical.

- **GIVEN** the existing `version_test.go` suite
- **WHEN** the refactor lands
- **THEN** every `TestVersion_*` test passes without modification

### Command: `shll list`

#### R5: New list subcommand wired into root
A new subcommand SHALL be added in `src/cmd/shll/list.go` via factory `newListCmd()` returning `*cobra.Command` (`Use: "list"`, `Args: cobra.NoArgs`), wired into `newRootCmd()` in `src/cmd/shll/root.go` via `AddCommand`. `RunE` MUST call a thin seam `runList(ctx context.Context, stdout io.Writer, jsonOut bool) error`. `shll list` MUST be added to the `rootLong` subcommand listing.

- **GIVEN** the built binary
- **WHEN** the user runs `shll list`
- **THEN** the command resolves (no "unknown command" error) and prints the roster
- **AND** `shll --help` lists `shll list`

#### R6: Default aligned table output
With no `--json` flag, `runList` SHALL emit one row per roster tool, in `Roster` order (leaves-first: wt, idea, tu, rk, hop, fab-kit), column-aligned via `text/tabwriter` (minwidth 0, tabwidth 0, padding 2, padchar space — same config as `version`). Columns MUST be: status indicator · name · description · repo URL. There MUST be no `shll` self-row. The repo column MUST be the full `https://github.com/sahil87/<Repo>` URL.

- **GIVEN** the roster
- **WHEN** `runList` runs with `jsonOut=false`
- **THEN** output has exactly `len(Roster)` rows in roster order
- **AND** each row shows a status indicator, the tool name, its description, and its full repo URL
- **AND** no row is for `shll` itself

#### R7: Status indicator with color/glyph gating
The installed indicator SHALL use a color glyph (`✓` installed / `✗` missing) only when `colorEnabled(stdout)` is true; otherwise an ASCII fallback (`ok` installed / `--` missing) with no ANSI escapes. The decision MUST reuse `colorEnabled` from `ui.go`. Glyph/marker strings MUST be named constants (code-quality.md).

- **GIVEN** a non-TTY writer (e.g. `bytes.Buffer`) or `NO_COLOR` set
- **WHEN** `runList` renders
- **THEN** output contains no `\x1b[` ANSI escape sequences and uses the ASCII status markers

#### R8: --json flag emits plain JSON array
A cobra bool flag `--json` (named constants `jsonFlag` / `jsonFlagUsage`) SHALL emit a bare JSON array of objects — one per roster tool in roster order — via `encoding/json` `MarshalIndent("", "  ")` with a single trailing newline. Each object MUST have fields `name`, `description`, `repo` (full resolved URL), and `installed` (bool). JSON output MUST contain no ANSI escapes and no table framing, regardless of TTY.

- **GIVEN** `--json` is set
- **WHEN** `runList` runs
- **THEN** output is valid JSON, a top-level array of length `len(Roster)`
- **AND** each element has `name`/`description`/`repo`/`installed`, `repo` is the full URL, `installed` reflects the probe
- **AND** there are no ANSI escapes and the output ends in a single `\n`

### Behavior: Graceful Degradation

#### R9: list never errors on a missing tool
`shll list` SHALL show a missing tool as missing, never as an error, and MUST exit 0 regardless of install status (Constitution V). Concurrent probing (mirroring `update.go`'s `probeRoster`/`sync.WaitGroup`) MAY be used, with results indexed by roster position for deterministic order; sequential is an acceptable fallback. All subprocess calls MUST route through `internal/proc` (no new `os/exec`).

- **GIVEN** a roster where one or more tools are not installed
- **WHEN** `shll list` runs
- **THEN** missing tools are marked missing, present tools are marked installed
- **AND** `runList` returns nil (exit 0)

### Non-Goals

- No runtime tool discovery — the roster stays hardcoded (Constitution III).
- No change to `shll version` output contract (R4 is behavior-preserving).
- No edit to `index.mdx` in the `sahil87/shll.ai` repo — it pulls the help-dump on its own schedule.
- No wrapped `{tools: [...]}` JSON envelope — bare array (intake assumption #11).

### Design Decisions

1. **Reuse the version-style PATH probe, not the brew probe**: `toolInstalled` runs `<tool> --version` — *Why*: install-mechanism-agnostic, matches `version`/`shell-init` semantics, future `doctor` reuses it — *Rejected*: brew `isInstalled` (couples "installed" to Homebrew; `install`/`update` use it but `list` answers "is it runnable").
2. **Repo slug stored explicitly on the roster**: defaults to `Name`, overridden to `run-kit` for `rk` — *Why*: `github.com/sahil87/rk` is a 404; a naive `<name>` URL ships a dead link — *Rejected*: deriving URL from `Name` alone.
3. **Bare JSON array top-level**: `[ {...} ]` — *Why*: `jq`-friendly, symmetric with the headerless table — *Rejected*: `{"tools": [...]}` envelope (YAGNI; help-dump's envelope is a versioned-schema concern that doesn't apply to a flat listing).
4. **Concurrent probe via `probeRoster`-style WaitGroup**: results indexed by roster position — *Why*: well-precedented in `update.go`, bounds wall-clock to ~`versionTimeout` not `N*versionTimeout` — *Rejected*: sequential (valid but slower; chosen pattern is established).

## Tasks

### Phase 1: Roster Metadata

- [x] T001 Add `Description` and `Repo` fields to the `Tool` struct in `src/cmd/shll/tools.go` with doc comments explaining single-sourcing and the `rk`/`run-kit` footgun <!-- R1 -->
- [x] T002 Add the `githubOrgBase` named constant (`"https://github.com/sahil87/"`) to `src/cmd/shll/tools.go` <!-- R2 -->
- [x] T003 Populate `Description` and `Repo` for all six `Roster` entries in `src/cmd/shll/tools.go` (`rk.Repo = "run-kit"`, others = `Name`) <!-- R1 -->

### Phase 2: Shared Probe

- [x] T004 Extract `toolInstalled(ctx, tool) bool` in `src/cmd/shll/version.go` (PATH-runnable probe via `proc.Run(... "--version")`, bounded by `versionTimeout`, any error ⇒ not installed) <!-- R3 -->
- [x] T005 Refactor `toolVersion` in `src/cmd/shll/version.go` to share the single `proc.Run` probe call (behavior-preserving) <!-- R4 -->

### Phase 3: list Command

- [x] T006 Create `src/cmd/shll/list.go` with `newListCmd()` factory, `jsonFlag`/`jsonFlagUsage` constants, the `--json` flag, and the `runList(ctx, stdout, jsonOut)` seam <!-- R5 -->
- [x] T007 Implement concurrent roster probing in `runList` (mirror `update.go`'s `probeRoster`/`WaitGroup`, indexed by roster position) using `toolInstalled` <!-- R9 -->
- [x] T008 Implement the default aligned `tabwriter` table path: status indicator · name · description · repo URL, in roster order, no self-row, full repo URL via `githubOrgBase + tool.Repo` <!-- R6 -->
- [x] T009 Implement the status indicator with color/glyph gating via `colorEnabled(stdout)` and ASCII fallback, using named status-marker constants <!-- R7 -->
- [x] T010 Implement the `--json` path: bare array of `{name, description, repo, installed}` via `encoding/json` `MarshalIndent`, trailing newline, no ANSI <!-- R8 -->

### Phase 4: Wiring & Help

- [x] T011 Wire `newListCmd()` into `newRootCmd()`'s `AddCommand` in `src/cmd/shll/root.go` <!-- R5 -->
- [x] T012 Add `shll list` to the `rootLong` subcommand listing in `src/cmd/shll/root.go` <!-- R5 -->

### Phase 5: Tests

- [x] T013 Create `src/cmd/shll/list_test.go` with `TestList_AllInstalled`, `TestList_SomeMissing`, `TestList_RepoLinks` (incl. `rk` → `run-kit` regression), `TestList_JSON` (valid JSON, len == len(Roster), fields, no ANSI, trailing newline), `TestList_NoANSI_Plain`, `TestList_Order`, and a non-empty `Description`/`Repo` guard <!-- R1 R6 R7 R8 R9 -->

## Execution Order

- Phase 1 (T001-T003) before Phase 3 (the list code reads the new fields).
- T004 before T005 (toolVersion layers on the shared probe).
- T006 before T007-T010 (they fill in the seam).
- Phase 4 wiring after the command exists.
- Tests last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `Tool` has `Description` + `Repo` fields; all six `Roster` entries populate both; `rk.Repo == "run-kit"`, others equal `Name`; no parallel list exists. (tools.go:49-54 fields; :91-97 roster all populate both; rk.Repo="run-kit", others=Name; no parallel slice.)
- [x] A-002 R2: A `githubOrgBase` named constant holds `https://github.com/sahil87/`; no open-coded URL prefix at any call site. (tools.go:60; sole composition site is `repoURL` list.go:110-112.)
- [x] A-003 R3: `toolInstalled(ctx, tool) bool` exists in `version.go`, probes `<tool> --version` via `proc.Run` bounded by `versionTimeout`, returns false on any error. (version.go:85-88 → probeToolVersion :72-76 WithTimeout(versionTimeout)+proc.Run; any err ⇒ false.)
- [x] A-004 R5: `newListCmd()` exists, is wired into `newRootCmd()`, and `shll list` appears in `rootLong`; `runList(ctx, stdout, jsonOut)` is the seam. (list.go:44 factory; root.go:36 AddCommand; root.go:18 rootLong row; runList list.go:75.)
- [x] A-005 R6: Default output is a `tabwriter` table with status · name · description · repo-URL columns, `len(Roster)` rows in roster order, no `shll` self-row, full repo URLs. (writeListTable list.go:119-126; tabwriter(0,0,2,' ',0) matches version; one row per Roster tool; no self-row; repoURL full URL.)
- [x] A-006 R8: `--json` emits a bare JSON array of `{name, description, repo, installed}`, length `len(Roster)`, full repo URL, trailing newline. (writeListJSON list.go:150-171 uses `json.NewEncoder` + `SetEscapeHTML(false)` + `SetIndent("","  ")` + `enc.Encode` which appends the newline; bare `[]listItem`; repo=repoURL. RESOLVED review Should-fix: `SetEscapeHTML(false)` keeps `&`/`<`/`>` literal in the raw bytes — Go's *default* encoder escapes `&` to the six-character `&`, which would diverge from the table column. Guarded by `TestList_JSON`.)

### Behavioral Correctness

- [x] A-007 R4: After the refactor, `version_test.go` passes unchanged and `shll version` output is identical (behavior-preserving). (toolVersion version.go:101-107 now layers on probeToolVersion; runVersion path unchanged; all 6 TestVersion_* + 12 normalizeVersion tests pass unmodified.)
- [x] A-008 R7: Status indicator uses color glyphs only when `colorEnabled(stdout)`; ASCII fallback (`ok`/`--`) for non-TTY/`NO_COLOR`. (statusMarker list.go:132-143 gated on `color`; color decided once via colorEnabled(w) list.go:120; verified live: table to non-TTY prints `ok`.)

### Scenario Coverage

- [x] A-009 R6: `TestList_AllInstalled` verifies six rows in roster order all marked installed; `TestList_Order` index-pairs to live `Roster`. (list_test.go:39-59 line count == len(Roster), per-row name+installed marker; :173-194 index-paired to Roster. Both pass.)
- [x] A-010 R9: `TestList_SomeMissing` verifies a failing-`--version` tool is marked missing while the rest are installed, and `runList` returns nil. (list_test.go:61-88 rk→missing marker, hop→installed, fatal-on-error guard. Passes.)
- [x] A-011 R1: `TestList_RepoLinks` verifies every row's repo column is `https://github.com/sahil87/<Repo>` and `rk` resolves to `.../run-kit` (404 regression guard). (list_test.go:90-112 asserts every repo URL present, requires `.../run-kit`, AND asserts `.../rk` is absent — the headline footgun guard. Passes.)
- [x] A-012 R8: `TestList_JSON` verifies valid JSON, `len == len(Roster)`, correct fields/values, `installed` reflects the probe, no ANSI, trailing newline. (list_test.go:114-157 unmarshal, len, per-field name/description/repo/installed, trailing newline, no `\x1b[`. Passes.)

### Edge Cases & Error Handling

- [x] A-013 R9: `shll list` never returns a non-nil error for a missing tool; exits 0 regardless of install status (Constitution V). (runList list.go:75-84 returns only the writer's err — never an install-status error; toolInstalled maps any probe error to false, never propagates. Verified live `shll list` exits 0; TestList_SomeMissing fatals on any error.)
- [x] A-014 R7: `TestList_NoANSI_Plain` verifies default output to a `bytes.Buffer` (non-TTY) has no `\x1b[` escapes. (list_test.go:159-171. Passes.)
- [x] A-015 R1: A unit assertion guards that every roster `Description` and every `Repo` is non-empty (regression against adding a tool without filling the fields). (TestList_RosterFieldsNonEmpty list_test.go:196-207. Passes.)

### Code Quality

- [x] A-016 Pattern consistency: `list.go` follows the `newXxxCmd()`/`runXxx(ctx, writers...)` seam, `tabwriter` config, and `colorEnabled`/glyph conventions of `version.go`/`ui.go`. (Factory+seam matches version.go; tabwriter(0,0,2,' ',0) identical; color decided once and passed to statusMarker, mirroring printToolHeader/printSummaryTail.)
- [x] A-017 No unnecessary duplication: `list` reuses `toolInstalled`, `colorEnabled`, `githubOrgBase`, and the `Roster` — it reimplements no probe, color check, or roster. (Confirmed: probeInstalled calls toolInstalled; statusMarker uses colorEnabled result; repoURL uses githubOrgBase; both paths iterate live Roster. probeInstalled mirrors update.go's probeRoster shape — see Nice-to-have note.)
- [x] A-018 Named constants (no magic strings): `githubOrgBase`, `jsonFlag`, `jsonFlagUsage`, and the status markers are named constants, not literals at call sites. (tools.go:60; list.go:17,20,26-31. All four status markers named; no literal glyphs/flags at call sites.)
- [x] A-019 Subprocess via internal/proc (Constitution I): the install probe routes through `proc.Run`; no new `os/exec` import anywhere. (probeToolVersion version.go:75 → proc.Run; grep confirms no os/exec in any cmd/shll/*.go non-test file.)
- [x] A-020 Graceful degradation (Constitution V): a missing tool is shown as missing, never an error. (toolInstalled returns bool, never errors; runList returns nil regardless of install status; verified live + TestList_SomeMissing.)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

None — this change adds new functionality without making existing code redundant. The `toolVersion` refactor (version.go) extracts the shared `probeToolVersion`/`toolInstalled` helpers but `toolVersion` itself is retained (still the sole probe for `shll version`) — nothing it replaced is now dead. `probeInstalled` (list.go) intentionally parallels `probeRoster` (update.go) rather than replacing it; the two probe different things (version-runnable vs. brew-installed + skip-flag support), so neither is redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Reuse the version-style PATH probe (`proc.Run <tool> --version`) for installed status, not the brew `isInstalled` probe | Intake assumption #1 (Certain); task says "binary+version probe"; codebase/constitution determine it | S:90 R:80 A:90 D:85 |
| 2 | Certain | Store repo slug explicitly on the roster; `rk.Repo = "run-kit"` | Intake assumption #3; `sahil87/rk` is a 404, `run-kit` is 200; naive `<name>` URL ships a dead link | S:95 R:55 A:90 D:95 |
| 3 | Certain | `--json` emits a bare array via `encoding/json` MarshalIndent; default is a `tabwriter` table | Intake assumptions #6/#11; task says "aligned table" + "--json"; codebase has exactly these two mechanisms | S:90 R:80 A:90 D:90 |
| 4 | Confident | Status markers: `✓`/`✗` glyphs under color/TTY, ASCII `ok`/`--` fallback for non-TTY/NO_COLOR | Intake §3a assumed marker; mirrors ui.go's glyph-vs-ASCII split (`✓` green check already used); plain-paste safety. `ok`/`--` chosen as the concrete ASCII tokens | S:70 R:80 A:80 D:65 |
| 5 | Confident | No `shll` self-row; probe roster concurrently via `probeRoster`-style WaitGroup, indexed by position | Intake assumptions #7/#8; consistent with install/update; well-precedented pattern, low blast radius | S:70 R:80 A:80 D:75 |

5 assumptions (3 certain, 2 confident, 0 tentative, 0 unresolved).
