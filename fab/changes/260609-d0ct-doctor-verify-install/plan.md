# Plan: shll doctor — verify toolkit install + wiring

**Change**: 260609-d0ct-doctor-verify-install
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### Doctor: Per-tool checks

#### R1: Binary-on-PATH + version probe (checks 1 + 2)
For every `Tool` in `Roster`, `shll doctor` SHALL run a single `<tool> --version`
probe through `internal/proc` (Constitution I), bounded by `versionTimeout` (2s),
and classify the outcome into exactly one of three states using the SAME
mechanism `version.go`'s `toolVersion` uses (`proc.Run` + `normalizeVersion`):

- **missing** — `proc.Run` returns `proc.ErrNotFound` (binary not on PATH).
- **unreportable** — `proc.Run` returns any other error/timeout, OR
  `normalizeVersion(out) == ""` (binary on PATH but version not reportable —
  the stale-link case).
- **ok** — `proc.Run` succeeds and `normalizeVersion(out)` is non-empty; the
  normalized version string is captured for display.

- **GIVEN** a roster tool whose binary is absent from PATH
- **WHEN** `shll doctor` probes it
- **THEN** the probe classifies it `missing` (drives a FAIL marker) and captures no version
- **AND GIVEN** a tool on PATH whose `--version` errors, times out, or yields an empty normalized string
- **WHEN** probed
- **THEN** the probe classifies it `unreportable` (drives a FAIL marker)
- **AND GIVEN** a tool on PATH that reports a parseable version
- **WHEN** probed
- **THEN** the probe classifies it `ok` and captures the normalized `vX.Y.Z` string

#### R2: Shell-init wiring check (check 3)
`shll doctor` SHALL run the wiring check ONLY for tools where
`len(tool.ShellInit) > 0` (so `wt`, `tu`, `hop` are checked; `idea`, `rk`,
`fab-kit` are not — derived from `Roster`, NOT the backlog prose). The check is a
SINGLE rc-file fact shared by all shell-init tools: resolve the shell from
`$SHELL` via `resolveShell([]string{}, env)`, derive the rc path via
`resolveRcFile(shell, env)`, `os.ReadFile` it, `locateBlock(content)`, and read
`blockMatch.hasEval` (covers BOTH the new `# >>> shll >>>` and legacy sentinels).
The check is strictly READ-ONLY — `doctor` MUST NEVER write to the rc file.

- **GIVEN** a shell-init tool and an rc file whose shll block has the eval line
- **WHEN** `shll doctor` runs the wiring check
- **THEN** the tool is reported `wired: true`
- **AND GIVEN** a shell-init tool and an rc file with no shll eval block (or no rc file)
- **WHEN** the wiring check runs
- **THEN** the tool is reported `wired: false` (drives a WARN marker if binary checks pass)
- **AND GIVEN** a non-shell-init tool (`idea`, `rk`, `fab-kit`)
- **WHEN** `shll doctor` runs
- **THEN** NO wiring check is performed for it and `shell_init` is `false`

#### R3: Unresolvable `$SHELL` degrades wiring to WARN
GIVEN `$SHELL` is unset or names an unsupported shell, the wiring check cannot
resolve an rc path. `shll doctor` SHALL degrade the wiring check for shell-init
tools to WARN with an explanatory suggestion; binary checks (R1) still run
normally and the exit code is unaffected.

- **GIVEN** `$SHELL=/bin/sh` (unsupported) and an otherwise-installed shell-init tool
- **WHEN** `shll doctor` runs
- **THEN** the tool's marker is WARN, the suggestion explains `$SHELL` cannot be resolved, and exit stays 0 (assuming no FAIL elsewhere)

### Doctor: Marker derivation + exit contract

#### R4: Worst-applicable-check marker (FAIL > WARN > OK)
Each tool's marker SHALL be the worst of its applicable checks:
binary `missing` or `unreportable` → **FAIL**; otherwise a shell-init tool whose
wiring is absent (or whose shell is unresolvable) → **WARN**; all applicable
checks pass → **OK**.

- **GIVEN** a tool that is FAIL on the binary check but would also be unwired
- **WHEN** the marker is derived
- **THEN** the marker is FAIL (binary failure dominates)
- **AND GIVEN** an installed, runnable shell-init tool whose wiring is absent
- **WHEN** the marker is derived
- **THEN** the marker is WARN

#### R5: Exit 1 iff any tool is FAIL
`shll doctor` SHALL return `errSilent` (→ exit 1 via `translateExit`) when ANY
tool's marker is FAIL, and nil (exit 0) otherwise. WARN NEVER affects the exit.
The exit logic SHALL be identical for text and `--json` output.

- **GIVEN** at least one tool with a FAIL marker
- **WHEN** `shll doctor` finishes (text or `--json`)
- **THEN** it returns `errSilent` and the process exits 1
- **AND GIVEN** every tool is OK or WARN (no FAIL)
- **WHEN** `shll doctor` finishes
- **THEN** it returns nil and the process exits 0

### Doctor: Output rendering

#### R6: Text output (default)
`shll doctor` SHALL print one tabwriter-aligned line per tool in Roster order:
`<name>  <MARKER>  [<version>]  [<status detail / suggestion>]`. Non-OK lines
MUST carry an actionable suggestion (named constants — no magic strings).
A trailing summary line SHALL report how many tools have problems when any tool
is non-OK. The marker glyph MAY use `ui.go`'s `colorEnabled(stdout)` gating
(green for OK on a TTY); plain ASCII markers (`OK`/`WARN`/`FAIL`) on non-TTY and
when `NO_COLOR` is set.

- **GIVEN** a mixed roster (some OK, one WARN, one FAIL)
- **WHEN** `shll doctor` renders text to a non-TTY buffer
- **THEN** each line shows the plain-ASCII marker, every non-OK line carries its suggestion, and no ANSI escape bytes are emitted
- **AND** a summary tail reports the count of tools with problems

#### R7: `--json` output mode
`shll doctor --json` SHALL emit a JSON array (one object per roster tool, roster
order) via `encoding/json` marshal of a TYPED struct (no hand-built JSON). Fields:
`tool`, `status`, `version`, `on_path`, `version_ok`, `shell_init`, `wired`,
`suggestion`. `shell_init` is `true` iff `len(tool.ShellInit) > 0`. Output SHALL
contain NO ANSI color regardless of TTY, SHALL be valid JSON with a trailing
newline, and SHALL be gated by the same checks and the same any-FAIL→exit-1
contract as text.

- **GIVEN** a mixed roster
- **WHEN** `shll doctor --json` runs against a buffer
- **THEN** stdout is a valid JSON array of one object per tool in roster order, with the field set above, no ANSI bytes, and a trailing newline
- **AND** the exit code matches what the text path would produce for the same roster state

### Doctor: Registration

#### R8: Command registration + rootLong
`newDoctorCmd()` (registering a single `--json` bool flag, `cobra.NoArgs`) SHALL
be added to `newRootCmd()` and a one-line `doctor` summary added to `rootLong`.
This raises the user-facing surface to six subcommands (the hidden `help-dump` is
not counted) — justified under Constitution VII in the intake.

- **GIVEN** the assembled root command
- **WHEN** its subcommands are enumerated
- **THEN** `doctor` is present and `rootLong` documents it
- **AND** the help-dump tree and any root-registration test account for `doctor`

### Design Decisions

1. **`--json` is included in v1** (deliberate, contra `version`'s no-json YAGNI
   decision): `doctor` is a CI verification surface whose primary consumer is a
   script that needs structured per-tool results — a machine-consumption use case
   `version` (a paste-into-bug-report table) does not have. Rendered via a typed
   struct marshal so text and JSON derive from one source and cannot drift.
   *Rejected*: a separate `shll list` subcommand (defers value; Constitution VII
   "could this be a flag?" is satisfied by a `--json` flag).
2. **"Ships shell-init" derives from `len(tool.ShellInit) > 0`**, NOT the backlog
   prose that listed `idea`. The live `Roster` is the source of truth
   (Constitution III), so `idea`/`rk`/`fab-kit` get checks 1+2 only. *Why*: the
   prose is factually wrong about `idea`; deriving from `Roster` keeps `doctor`
   correct as the roster evolves.
3. **Single subprocess per tool** covers checks 1 and 2 (`proc.ErrNotFound` vs
   other-error distinguishes missing-vs-broken in one call), matching
   `toolVersion`. *Why*: bounds worst-case runtime and single-sources the probe.
4. **`env func(string) string` seam on `runDoctor`** mirrors
   `resolveShell`/`resolveRcFile`. Production passes `os.Getenv`; tests pass a
   map-backed func so the wiring check reads a `t.TempDir()` rc file (via
   `ZDOTDIR`) and NEVER touches the real `~/.zshrc`. *Rejected*: a `--rc-file`
   flag on `doctor` (unnecessary surface; the env seam suffices for testing).
5. **New probe helper `probeVersion`** returns `(version string, state
   versionState)` rather than reusing `toolVersion` directly, because `doctor`
   needs the missing-vs-unreportable distinction that `toolVersion` collapses into
   `notInstalledLabel`. It uses the SAME `proc.Run`/`versionTimeout`/
   `normalizeVersion` primitives, so the probe stays single-sourced; `toolVersion`
   is left untouched (its callers don't need the distinction).

## Tasks

### Phase 1: Core check logic

- [x] T001 Add `src/cmd/shll/doctor.go` with the marker/status/suggestion named constants, the typed `doctorResult` struct (JSON tags: `tool`/`status`/`version`/`on_path`/`version_ok`/`shell_init`/`wired`/`suggestion`), and the `versionState` enum. <!-- R1 R7 -->
- [x] T002 Implement `probeVersion(ctx, tool)` in `doctor.go` reusing `proc.Run`+`versionTimeout`+`normalizeVersion`, returning the three-way state (missing/unreportable/ok) plus the captured version. <!-- R1 -->
- [x] T003 Implement the shell-wiring fact in `doctor.go`: resolve shell via `resolveShell([]string{}, env)`, derive rc path via `resolveRcFile`, `os.ReadFile`, `locateBlock`, read `hasEval`; return a (wired bool, shellResolved bool) result. Read-only. <!-- R2 R3 -->

### Phase 2: Aggregation, markers, exit

- [x] T004 Implement `evaluateTool(ctx, tool, env, wiringFact)` in `doctor.go` that composes R1+R2/R3 into a `doctorResult` with the worst-applicable marker and the matching suggestion constant. <!-- R4 -->
- [x] T005 Implement `runDoctor(ctx, jsonOut bool, env func(string) string, stdout, stderr io.Writer) error`: walk `Roster` in order, build results, compute any-FAIL, render, and return `errSilent` iff any FAIL. <!-- R5 -->

### Phase 3: Rendering

- [x] T006 Implement text rendering in `doctor.go` via `text/tabwriter`, using `colorEnabled(stdout)` for the OK glyph and plain ASCII markers otherwise, plus the problem-count summary tail. <!-- R6 -->
- [x] T007 Implement `--json` rendering in `doctor.go` via `encoding/json` marshal of `[]doctorResult` with a trailing newline and no ANSI. <!-- R7 -->

### Phase 4: Registration + tests

- [x] T008 Add `newDoctorCmd()` (cobra `--json` bool flag, `cobra.NoArgs`) in `doctor.go` and register it in `src/cmd/shll/root.go`'s `newRootCmd()`; add the `doctor` summary line to `rootLong`. <!-- R8 -->
- [x] T009 Add `src/cmd/shll/doctor_test.go`: drive `runDoctor` with `bytes.Buffer` + a fake `proc.Runner` and a map-backed env; cover each marker path, worst-check-wins, exit (any-FAIL→1, WARN-only→0), the `idea`-has-no-wiring-check assertion, unresolvable-`$SHELL`→WARN, and `--json` (valid JSON, field values, same exit, no ANSI). Use `t.TempDir()` rc files. <!-- R1 R2 R3 R4 R5 R6 R7 -->
- [x] T010 Update `src/cmd/shll/help_dump_test.go` / any `TestRoot_*` expectation to account for `doctor` (the tree legitimately grows by one real command). <!-- R8 -->

## Execution Order

- T001 precedes T002–T007 (defines the shared types/constants).
- T002 and T003 are independent inputs to T004; T004 precedes T005.
- T006 and T007 depend on the `doctorResult` shape (T001) and feed T005.
- T008 depends on `newDoctorCmd`/`runDoctor` existing; T009/T010 are validation.

## Acceptance

### Functional Completeness

- [ ] A-001 R1: The version probe classifies missing (ErrNotFound), unreportable (other error/timeout/empty normalize), and ok (non-empty normalized version) correctly, via the shared `proc.Run`/`versionTimeout`/`normalizeVersion` primitives.
- [ ] A-002 R2: The wiring check runs only for `len(tool.ShellInit) > 0` tools (`wt`/`tu`/`hop`), reads `blockMatch.hasEval` from the resolved rc file, never writes, and reports `shell_init:false` for `idea`/`rk`/`fab-kit`.
- [ ] A-003 R3: An unsupported/unset `$SHELL` degrades the wiring check to WARN with an explanatory suggestion; binary checks still run and exit is unaffected.
- [ ] A-004 R4: Each tool's marker is the worst applicable check (FAIL > WARN > OK).
- [ ] A-005 R5: `runDoctor` returns `errSilent` iff any tool is FAIL; identical exit logic for text and `--json`.
- [ ] A-006 R6: Text output is one tabwriter-aligned line per tool in roster order, non-OK lines carry a suggestion, a problem-count tail appears when any tool is non-OK, and no ANSI bytes are emitted to a non-TTY buffer.
- [ ] A-007 R7: `--json` emits a valid JSON array (typed-struct marshal) with the specified field set, roster order, trailing newline, no ANSI, and the same exit contract as text.
- [ ] A-008 R8: `doctor` is registered on the root command and documented in `rootLong`; six user-facing subcommands total.

### Behavioral Correctness

- [ ] A-009 R5: A WARN-only roster (e.g. installed-but-unwired) exits 0; a roster with any FAIL exits 1.

### Scenario Coverage

- [ ] A-010 R1 R4: `doctor_test.go` exercises the missing→FAIL, unreportable→FAIL, unwired→WARN, and all-pass→OK paths against a fake runner.
- [ ] A-011 R7: `doctor_test.go` unmarshals the `--json` output and asserts field values per marker.

### Edge Cases & Error Handling

- [ ] A-012 R3: `doctor_test.go` covers the unresolvable-`$SHELL` degradation (binary checks still reported, wiring WARN, exit unaffected).
- [ ] A-013 R2: `doctor_test.go` asserts `idea` (and other non-shell-init tools) receives no wiring check / `shell_init:false`.

### Code Quality

- [ ] A-014 Pattern consistency: `doctor.go` follows the `newXxxCmd()` factory + thin `runDoctor` seam pattern and `version.go`/`shell_setup.go` style.
- [ ] A-015 No unnecessary duplication: the version probe and wiring detection reuse existing primitives rather than reimplementing them (Constitution III, code-quality.md).
- [ ] A-016 Named constants: all markers, statuses, and suggestion strings are named constants — no magic strings (code-quality.md).
- [ ] A-017 Security: the version probe routes through `internal/proc`; rc-file access is read-only `os.ReadFile` (Constitution I).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Reuse `proc.Run`/`versionTimeout`/`normalizeVersion` for the PATH+version checks; do not reimplement | Intake assumption 1 + Constitution III; primitives exist and are tested | S:95 R:90 A:95 D:95 |
| 2 | Certain | Reuse `resolveShell`/`resolveRcFile`/`locateBlock`/`hasEval` (read-only) for the wiring check | Intake assumption 2; the wiring detector already exists | S:95 R:90 A:95 D:90 |
| 3 | Certain | "Ships shell-init" derives from `len(tool.ShellInit) > 0` (`idea` NOT wiring-checked) | Intake assumption 3 + Constitution III; Roster is source of truth | S:90 R:80 A:98 D:95 |
| 4 | Certain | Any-FAIL → exit 1 via `errSilent`; WARN never affects exit; same for text + `--json` | Intake assumptions 8/10/13; `errSilent` is the established exit-1 path | S:90 R:80 A:90 D:90 |
| 5 | Certain | `--json` is a bool flag on `doctor`, emitting a typed-struct marshal with no ANSI | Intake assumption 13; flag-not-subcommand satisfies Constitution VII | S:85 R:80 A:90 D:85 |
| 6 | Confident | New `probeVersion` returns (version, three-way state) reusing the shared primitives, rather than calling `toolVersion` (which collapses missing/unreportable into `notInstalledLabel`) | `doctor` needs a distinction `toolVersion` discards; keeping the probe primitives shared single-sources behavior while leaving `toolVersion` untouched | S:80 R:80 A:85 D:80 |
| 7 | Confident | `runDoctor` takes an `env func(string) string` seam (prod: `os.Getenv`) for the wiring check, mirroring `resolveShell`/`resolveRcFile` | Enables `t.TempDir()` rc-file testing without touching `~/.zshrc`; matches the established env-seam pattern | S:80 R:85 A:90 D:80 |
| 8 | Confident | Text line shape `<name> <MARKER> [<version>] [<detail/suggestion>]` + problem-count tail; OK glyph via `colorEnabled` on TTY | Intake fixes semantics + illustrative shape only; mirrors `version.go` tabwriter and `ui.go` color discipline | S:80 R:80 A:80 D:75 |

8 assumptions (5 certain, 3 confident, 0 tentative, 0 unresolved).
</content>
</invoke>
