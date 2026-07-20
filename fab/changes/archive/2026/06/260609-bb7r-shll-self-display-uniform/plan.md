# Plan: Uniform shll-self representation across inspect/manage commands

**Change**: 260609-bb7r-shll-self-display-uniform
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

<!-- Derived from the intake-design. All core decisions were resolved in a
     /fab-discuss session and recorded as 14 graded assumptions in intake.md;
     this section restates them as RFC-2119 requirements. -->

### Descriptor: shared `shllSelf` source of truth

#### R1: A single shared descriptor represents shll as a displayable entry
A package-level `shllSelf` descriptor SHALL exist in `src/cmd/shll/tools.go`, reusing the existing `Tool` struct shape, with `Name = "shll"`, `Description = "the manager for the shll toolkit"`, and `Repo = "shll"`. It MUST NOT be added to the `Roster` slice. Its version SHALL be read from the package-level `version` var in `main.go`, never via a `shll --version` self-subprocess.

- **GIVEN** the binary is running
- **WHEN** any command renders the shll-self entry
- **THEN** it reads `Name`/`Description`/`Repo` from the shared `shllSelf` descriptor and the version from the package `version` var
- **AND** `Roster` still contains exactly the 6 managed sub-tools

#### R2: Roster invariant is preserved
`Roster` SHALL remain exactly the 6 managed sub-tools and `TestRosterLeavesBeforeDependents` MUST remain untouched and passing.

- **GIVEN** the change is applied
- **WHEN** `TestRosterLeavesBeforeDependents` runs
- **THEN** it passes unmodified, and `len(Roster) == 6`

### Ordering: shll-first, then leaves-first roster

#### R3: Unified ordering across commands that show the toolkit
Every command that meaningfully shows the toolkit SHALL render shll FIRST, then the leaves-first `Roster` order (`shll, wt, idea, tu, rk, hop, fab-kit`).

- **GIVEN** `list`/`doctor`/`install` output
- **WHEN** the toolkit is shown
- **THEN** the shll entry is the first row/object/line, followed by the roster in its existing order

### doctor

#### R4: doctor prepends an always-OK shll row without perturbing the exit contract
`doctor` SHALL prepend a shll-first row running checks 1 (on PATH) + 2 (version) only — no wiring check (shll ships no shell-init, treated like `idea`/`rk`/`fab-kit`, `shell_init:false`). The binary is always present (it is the running process) and the version comes from the package `version` var, so the row is effectively always OK. It MUST NOT affect the any-FAIL→exit-1 scriptable contract. The `--json` output SHALL include a shll object too.

- **GIVEN** `shll doctor` (text or `--json`)
- **WHEN** doctor evaluates the toolkit
- **THEN** the shll row/object is first, marked OK, `shell_init:false`, `wired:false`, version from the package var
- **AND** the OK shll row never sets `anyFail`, so a clean roster still exits 0 and a roster with a FAIL still exits 1
- **AND** the text summary-tail denominator counts only the checkable roster tools (`len(Roster)`), NOT the always-OK shll row — so a single roster failure reads "1 of 6 tools have problems", not "1 of 7"

### list

#### R5: list table prepends a shll-first row with the plain installed marker
`shll list` (table) SHALL prepend a shll-first row using the PLAIN installed marker (`ok` / green `✓` — same rendering as an installed tool), Description "the manager for the shll toolkit", repo `https://github.com/sahil87/shll`.

- **GIVEN** `shll list` (table)
- **WHEN** the roster is rendered
- **THEN** the first row is shll with the installed marker, the manager description, and the resolved repo URL

#### R6: list --json prepends a shll-first object carrying self:true
`shll list --json` SHALL prepend a shll-first object with `installed: true` and a `self` field that is `true` for shll and absent/false on the 6 managed tools (use `omitempty` so it is absent on managed tools).

- **GIVEN** `shll list --json`
- **WHEN** the array is built
- **THEN** the first object is `{name:"shll", ..., installed:true, self:true}` and managed-tool objects have no `self` field
- **AND** a consumer can filter shll out via `select(.self != true)`

### install

#### R7: install prepends a shll-first informational line
`shll install` SHALL prepend a shll-first INFORMATIONAL line (e.g. "shll — already present / self-managed"). It MUST NOT be a brew install action on the running binary.

- **GIVEN** `shll install` (any subset / whole-roster / dry-run that reaches the install phase)
- **WHEN** install begins
- **THEN** an informational shll-first line is emitted, and no `brew install` is run for shll

### shell-init (exception) + comment reconciliation

#### R8: shell-init is the documented exception; the list.go comment is reconciled
`shell-init` SHALL NOT be touched (its stdout is `eval`'d — Constitution V eval-safety; shll has no own shell-init output). The `runList` doc comment in `list.go` (~lines 73-74) that asserts "no self-row" SHALL be updated to reflect that there is now a shll self-row for discoverability.

- **GIVEN** the change is applied
- **WHEN** the `list.go` doc comment is read
- **THEN** it documents the shll self-row (discoverability) rather than asserting no self-row
- **AND** `shell_init.go` is unchanged

### Non-Goals

- README and `docs/memory/` updates — handled in hydrate, not apply (the in-code `list.go` comment IS a code change and is in scope).
- `version` and `update` code changes — already shll-first; only memory cross-refs later in hydrate.

### Design Decisions

1. **Shared descriptor, not a Roster entry**: a single `shllSelf` `Tool` value prepended by each command — *Why*: one source of truth, reuses existing `Tool`/`repoURL` plumbing — *Rejected*: adding shll to `Roster` (violates Constitution III, breaks the leaves-first invariant, would make install/update/shell-init operate on shll itself).
2. **`self` field via `omitempty`**: `self bool `json:"self,omitempty"`` so the field is absent on the 6 managed tools and `true` only on shll — *Why*: cleanest scripting filter (`select(.self != true)`) and minimal diff to existing managed-tool objects — *Rejected*: always-emitting `self:false`.

## Tasks

### Phase 1: Setup

- [x] T001 Add the shared `shllSelf` descriptor to `src/cmd/shll/tools.go` (a package-level `Tool` value: `Name: shllTargetToken`, `Description: "the manager for the shll toolkit"`, `Repo: "shll"`; no `Formula`/`ShellInit`/`Update`), with a doc comment explaining it is NOT a Roster entry (Constitution III + leaves-first invariant) and which commands prepend it. Add a `shllSelfVersion()` helper (or inline read) that returns the version from the package `version` var via `normalizeVersion`. <!-- R1 -->

### Phase 2: Core Implementation

- [x] T002 [P] `list` table: in `src/cmd/shll/list.go`, prepend a shll-first row in `writeListTable` using the plain installed marker (`statusMarker(true, color)`), `shllSelf.Description`, and `repoURL(shllSelf)`. <!-- R5 -->
- [x] T003 [P] `list --json`: add a `Self bool `json:"self,omitempty"`` field to `listItem` and prepend a shll-first object in `writeListJSON` with `Installed: true, Self: true`; managed tools keep `Self` zero (omitted). <!-- R6 -->
- [x] T004 [P] `doctor`: in `src/cmd/shll/doctor.go`, build a shll-first `doctorResult` (`Tool:"shll"`, `Status:markerOK`, `OnPath:true`, `VersionOK:true`, `Version: shll's normalized version`, `ShellInit:false`, `Wired:false`) and prepend it to `results` in `runDoctor`. It must NOT set `anyFail`. Both text and `--json` render it via the existing `results` walk. The text summary tail's denominator MUST count only checkable roster tools (`len(Roster)`), never the always-OK shll row. <!-- R4 --> <!-- rework: summary-tail denominator used len(results)=len(Roster)+1, so a roster failure mis-read "1 of 7" instead of "1 of 6"; the prepended always-OK shll row must be excluded from the denominator, with a regression test -->
- [x] T005 [P] `install`: in `src/cmd/shll/install.go`, prepend a shll-first informational line ("shll — already present / self-managed") to stdout. Emit it on the paths that reach the install phase / preview / nothing-to-do without turning shll into a brew-install target. <!-- R7 -->

### Phase 3: Integration & Edge Cases

- [x] T006 Update the `runList` doc comment in `src/cmd/shll/list.go` (~lines 73-74) to state there IS now a shll self-row (discoverability) rather than "no self-row". <!-- R8 -->
- [x] T007 Update `len(Roster)`-based row-count assertions in `src/cmd/shll/list_test.go` and `src/cmd/shll/doctor_test.go` to account for the prepended self row (`len(Roster)+1`), and adjust roster-index-paired assertions to offset by the leading shll row. Add new assertions for the shll-first row in list (table + json `self:true`), doctor (text + json object, OK/shell_init:false/wired:false, version present, exit contract intact), and install (informational line present, no shll brew install). Use existing fakes (`installFakeRunner`, `listFake`, `doctorFake`, etc.). `TestRosterLeavesBeforeDependents` stays untouched. <!-- R2 R3 R4 R5 R6 R7 -->

### Phase 4: Polish

- [x] T008 Run `gofmt`, `go vet`, and `go test ./...` from `src/`; confirm `shell_init.go` untouched and `Roster`/`TestRosterLeavesBeforeDependents` unchanged. <!-- R2 R8 -->

## Execution Order

- T001 blocks T002–T005 (they consume `shllSelf`).
- T002–T005 are `[P]` (different files / independent sections).
- T006 is in `list.go` (sequence after T002/T003 to avoid edit churn).
- T007 depends on T001–T006 (asserts their behavior).
- T008 last (whole-suite gate).

## Acceptance

### Functional Completeness

- [x] A-001 R1: A `shllSelf` descriptor exists in `tools.go` with Name `shll`, Description "the manager for the shll toolkit", Repo `shll`; its version is read from the package `version` var, not a self-subprocess.
- [x] A-002 R2: `Roster` has exactly 6 entries and `TestRosterLeavesBeforeDependents` passes unmodified.
- [x] A-003 R3: `list`, `doctor`, and `install` all render shll first, then leaves-first roster order.
- [x] A-004 R4: `shll doctor` (text + `--json`) shows an OK shll-first row/object with `shell_init:false`, `wired:false`, version present.
- [x] A-005 R5: `shll list` table first row is shll with the plain installed marker, manager description, and `https://github.com/sahil87/shll`.
- [x] A-006 R6: `shll list --json` first object has `installed:true, self:true`; managed-tool objects omit `self`.
- [x] A-007 R7: `shll install` emits a shll-first informational line and never runs `brew install` for shll.

### Behavioral Correctness

- [x] A-008 R4: The always-OK shll doctor row does not set `anyFail` — a clean roster still exits 0, a roster with a FAIL still exits 1.
- [x] A-009 R8: `shell_init.go` is unchanged; the `runList` doc comment documents the new self-row for discoverability.

### Scenario Coverage

- [x] A-010 R6: A `select(.self != true)` style filter over `list --json` yields exactly the 6 managed tools.
- [x] A-011 R4: `doctor --json` array length is `len(Roster)+1` with shll first.

### Edge Cases & Error Handling

- [x] A-012 R7: On the install "all already installed" / dry-run paths, the shll informational line is present and no shll brew subprocess is attempted.

### Code Quality

- [x] A-013 Pattern consistency: New code follows surrounding patterns (named constants over magic strings, single source of truth via `shllSelf`/`repoURL`, table/json derive from one descriptor).
- [x] A-014 No unnecessary duplication: shll-self representation is single-sourced in `shllSelf` and reused by every command; no per-command re-declaration of name/description/repo.
- [x] A-015 Subprocess discipline: No new `os/exec` in command code; shll version comes from the package var (no self-subprocess) — Constitution I & III honored.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Deletion Candidates

- None — this change adds new functionality (the shared `shllSelf` display descriptor prepended by `list`/`doctor`/`install`) without making existing code redundant. The pre-existing `shllTargetToken` const is now additionally reused by `shllSelf.Name`/`shllSelf.Repo`, which reinforces single-sourcing rather than leaving dead code.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `shllSelf` lives in `tools.go` reusing the `Tool` struct; version via `normalizeVersion(version)` from `main.go` | Intake assumptions #3, #11 fixed this; `tools.go` already holds roster/repo plumbing and `version.go` already calls `normalizeVersion(version)` for the shll row | S:95 R:80 A:90 D:90 |
| 2 | Certain | `list --json` `self` field uses `json:"self,omitempty"` so it is absent on managed tools and true on shll | Intake assumption #6 picked the omitempty variant explicitly ("cleanest") | S:95 R:80 A:90 D:95 |
| 3 | Confident | doctor builds the shll `doctorResult` directly (not via `evaluateTool`, which probes `--version` as a subprocess) and prepends it before the roster walk | Intake #3/#5 forbid a self-subprocess; `evaluateTool` always calls `probeVersion`, so a direct struct is the faithful implementation. Keeps the always-OK row out of `anyFail` | S:85 R:80 A:85 D:80 |
| 4 | Confident | install emits the informational line once, on every path that reaches the install decision (nothing-to-do, dry-run preview, and the install loop), before the roster framing | Intake #8 says "prepend a shll-first informational line"; emitting it consistently across the command's terminal paths matches "shll-first" without making shll a brew target | S:78 R:85 A:80 D:70 |

4 assumptions (2 certain, 2 confident, 0 tentative).
