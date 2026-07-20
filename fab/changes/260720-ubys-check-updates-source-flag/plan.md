# Plan: check-updates --source enum flag

**Change**: 260720-ubys-check-updates-source-flag
**Intake**: `intake.md`

## Requirements

### CLI: `--source` enum flag replaces the backend bool pair

#### R1: Single `--source` string flag
`newCheckUpdatesCmd` SHALL register exactly one backend flag: a string flag `--source` (constant `sourceFlag`, usage constant `sourceFlagUsage`) with default `sourceReleased`. The existing envelope value constants `sourceReleased`/`sourceGithub` SHALL double as the flag's valid enum values — no new value constants. No shorthand (`-s`).

- **GIVEN** `shll check-updates` with no flags
- **WHEN** the command runs
- **THEN** the released backend is used (default `sourceReleased`), identical to today's flagless behavior

#### R2: Run seam takes `source string`
`runCheckUpdates` SHALL have the signature `runCheckUpdates(ctx context.Context, stdout, stderr io.Writer, source string, jsonOut bool) error`. The cobra `RunE` SHALL read the flag via `cmd.Flags().GetString(sourceFlag)` and pass it through raw (no cobra-side validation).

- **GIVEN** `shll check-updates --source github --json`
- **WHEN** `RunE` fires
- **THEN** `runCheckUpdates` receives `source == sourceGithub` and the github backend resolves latest tags

#### R3: Unknown `--source` value → usage error, exit 2, before any work
`runCheckUpdates` SHALL validate `source` in the run seam (not cobra machinery): a value that is neither `sourceReleased` nor `sourceGithub` returns `&errExitCode{code: usageExitCode, msg: ...}` where the message names the offending value and the valid set. Validation MUST fire before the brew gate and before any network access.

- **GIVEN** `runCheckUpdates(ctx, out, err, "bogus", false)` with a fake runner and a guarded manifest endpoint
- **WHEN** the seam runs
- **THEN** it returns `errExitCode{code: usageExitCode}` naming `"bogus"` and the valid set, with zero recorded subprocess calls and empty stdout

#### R4: Clean break — both-flags path deleted
The bool flag constants (`releasedFlag`, `releasedFlagUsage`, `githubFlag`, `githubFlagUsage`), their registrations, `bothBackendsErrMsg`, the `if released && github` check, and the `source := sourceReleased; if github {...}` derivation SHALL all be deleted. No hidden/deprecated aliases remain.

- **GIVEN** `shll check-updates --released`
- **WHEN** cobra parses flags
- **THEN** it is an unknown-flag error (cobra usage error, exit 2 via the existing `translateExit` path) — no code path in `check_updates.go` recognizes the old flags

#### R5: `Long` help rewritten for the enum flag
The cobra `Long` block's "Two backends, mutually exclusive:" section SHALL be rewritten to describe the single `--source` flag (per the intake sketch: "One backend, selected by --source:"), preserving the layered structure (summary, backends, examples, exit codes). The exit-codes paragraph keeps "2 on a usage error" (which covers the unknown-value case) and drops any both-flags implication.

- **GIVEN** `shll check-updates --help`
- **WHEN** help renders
- **THEN** it documents `--source released` (default) and `--source github`, shows `--source github` in the examples, and mentions no mutual exclusion

#### R6: Tests updated to the new seam
All `runCheckUpdates` call sites in `check_updates_test.go` SHALL move to the `(source string, jsonOut bool)` signature using the source constants. `TestCheckUpdates_BothBackendFlagsUsageError` SHALL become an unknown-`--source`-value usage-error test (R3's scenario). All other test assertions are unchanged in substance.

- **GIVEN** the converted test suite
- **WHEN** `go test ./cmd/shll/...` runs from `src/`
- **THEN** all tests pass, including the new unknown-value test pinning exit 2 + zero subprocess calls + empty stdout

#### R7: Docs collateral rewritten to the `--source` form
`README.md` (the check-updates examples block ~L107–110, the backends paragraph ~L113, the `--github`-row mentions ~L117, and the command-table row ~L281) and `docs/site/skill.md` (the check-updates line ~L22) SHALL be rewritten to the `--source` vocabulary. After editing `docs/site/skill.md`, `scripts/sync-standards.sh` SHALL be run so the committed embed copy (`src/cmd/shll/skill/skill.md`, drift-guarded by `TestSkillEmbedMatchesCanonical`) matches the canonical file. Content-only edits; the readme-extraction structure is untouched.

- **GIVEN** the edited docs
- **WHEN** `grep -r -- '--released\|--github'` runs over `README.md`, `docs/site/skill.md`, and the embed copies
- **THEN** no old-flag references remain, and the embed drift-guard tests pass

### Non-Goals

- The `--json` machine contract — `schema` stays 1; envelope `source` field/values, row shape, unresolvable-row rule, `notify`/`notable` released-rows-only rule all unchanged
- `docs/site/standards/` — no new convention line (offered and declined at intake)
- `internal/versions` / `internal/changelog` — resolver seams untouched
- No deprecated-alias transition period (day-old surface)
- `docs/memory/cli/check-updates.md` — hydrate-stage work, not apply's

### Design Decisions

#### Enum validation in the run seam, not cobra machinery
**Decision**: unknown `--source` values are rejected inside `runCheckUpdates` with `&errExitCode{code: usageExitCode}`, before the brew gate and any network access.
**Why**: pflag has no native enum type, and cobra-side errors (flag groups, custom `Value` parse errors) are plain errors that would exit 1 — the toolkit contract pins usage errors at exit 2. Keeps the case testable through the writer seam, exactly like the both-flags check it replaces.
**Rejected**: a custom `pflag.Value` enum type (its parse error routes through cobra's error path, exiting 1 unless coupled to cobra-internal message shapes); cobra `PreRunE` validation (same exit-code coupling, less testable through the seam).
*Introduced by*: 260720-ubys-check-updates-source-flag

#### `--source released|github`, not `--backend shll|github`
**Decision**: the flag is named `--source` and its values are the existing envelope source constants `released`/`github`.
**Why**: the `--json` envelope already carries `"source": "released"|"github"` — flag name, flag value, and envelope output become one vocabulary; a consumer reading the JSON can write the flag back verbatim.
**Rejected**: `--backend` naming (vocabulary mismatch with the envelope); `shll` as the default value's name (ambiguous — reads as the binary; and it would either mismatch the envelope or force a breaking schema-2 change for zero gain).
*Introduced by*: 260720-ubys-check-updates-source-flag

## Tasks

### Phase 2: Core Implementation

- [x] T001 In `src/cmd/shll/check_updates.go`: delete the `releasedFlag`/`releasedFlagUsage`/`githubFlag`/`githubFlagUsage` constants and `bothBackendsErrMsg`; add `sourceFlag`/`sourceFlagUsage` constants and an `invalidSourceErrFmt` diagnostic format constant; register `cmd.Flags().String(sourceFlag, sourceReleased, sourceFlagUsage)` in place of the two bool registrations; update the source-constants comment (they now double as flag values) <!-- R1, R4 -->
- [x] T002 In `src/cmd/shll/check_updates.go`: change `runCheckUpdates` to `(ctx, stdout, stderr, source string, jsonOut bool)`; `RunE` reads `GetString(sourceFlag)` and passes it raw; add the unknown-value validation (→ `errExitCode{code: usageExitCode}`) as the first check before the brew gate; delete the both-flags check and the bool→source derivation; update the seam's flow doc comment <!-- R2, R3, R4 -->
- [x] T003 In `src/cmd/shll/check_updates.go`: rewrite the `Long` help block for the enum flag ("One backend, selected by --source:", updated examples, exit-codes prose without both-flags implication); sweep remaining in-file comments that name the removed flags to backend-name form <!-- R5 -->

### Phase 3: Integration & Edge Cases

- [x] T004 In `src/cmd/shll/check_updates_test.go`: convert all `runCheckUpdates` call sites to the `(source, jsonOut)` signature using `sourceReleased`/`sourceGithub`; replace `TestCheckUpdates_BothBackendFlagsUsageError` with `TestCheckUpdates_UnknownSourceValueUsageError` (asserts `errExitCode{code: usageExitCode}`, message names `"bogus"` + the valid set, zero recorded subprocess calls, empty stdout); run `go test ./cmd/shll/...` from `src/` <!-- R6 -->

### Phase 4: Polish

- [x] T005 [P] Rewrite `README.md` check-updates references: examples block (`--json --released` → `--json`, `--github` → `--source github`), the "Two mutually exclusive backends" paragraph, the `--github`-row mentions in the JSON-contract paragraph, and the L~281 command-table row <!-- R7 -->
- [x] T006 [P] Rewrite `docs/site/skill.md` L~22's check-updates flag description to the `--source` form, then run `scripts/sync-standards.sh` so `src/cmd/shll/skill/skill.md` matches <!-- R7 -->
- [x] T007 Run the full package suite from `src/` (`go test ./...`) and `go build ./...`; verify no `--released`/`--github` references remain outside `docs/memory/` (hydrate's scope) and the change folder <!-- R6, R7 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `check-updates` registers exactly one backend flag — `--source` (string, default `released`, no shorthand) — plus the unchanged `--json`
- [x] A-002 R2: `runCheckUpdates` has the `(ctx, stdout, stderr, source string, jsonOut bool)` signature and `RunE` passes the raw `GetString` value through
- [x] A-003 R3: an unknown `--source` value returns `errExitCode{code: usageExitCode}` naming the offending value and the valid set, before any brew/network access
- [x] A-004 R7: `README.md` and `docs/site/skill.md` describe the `--source` form and the embed copy matches the canonical file (sync script run, drift guards green)

### Behavioral Correctness

- [x] A-005 R1: flagless `shll check-updates` still uses the released backend — the existing released-backend tests pass unchanged in substance
- [x] A-006 R5: `--help` documents `--source released`/`--source github` with the layered structure preserved and no mutual-exclusion language

### Removal Verification

- [x] A-007 R4: `releasedFlag`, `releasedFlagUsage`, `githubFlag`, `githubFlagUsage`, `bothBackendsErrMsg`, the both-flags check, and the bool→source derivation are gone — no dead code, and `--released`/`--github` are unknown-flag errors

### Scenario Coverage

- [x] A-008 R6: `TestCheckUpdates_UnknownSourceValueUsageError` exists and pins exit 2 + offending-value/valid-set message + zero subprocess calls + empty stdout; the both-flags test is gone
- [x] A-009 R6: `go test ./cmd/shll/...` (and the full `go test ./...`) pass — the JSON-contract, degradation, table, and manifest-guard tests are unchanged in substance

### Edge Cases & Error Handling

- [x] A-010 R3: the empty string `""` is rejected like any other unknown value (the default is applied by the flag layer, not the seam) — the validation `source != sourceReleased && source != sourceGithub` catches `""`; the `--source` flag layer defaults to `released` so `""` only reaches the seam via a direct `runCheckUpdates(..., "", ...)` call

### Code Quality

- [x] A-011 Pattern consistency: new constants follow the existing `xxxFlag`/`xxxFlagUsage` naming shape; no magic strings (flag name, usage, diagnostic format all named constants)
- [x] A-012 No unnecessary duplication: the existing `sourceReleased`/`sourceGithub` constants are reused as the enum values — no new value constants

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this refactor already deleted the code it made redundant (the `releasedFlag`/`releasedFlagUsage`/`githubFlag`/`githubFlagUsage` constants, `bothBackendsErrMsg`, the `if released && github` check, and the `source := sourceReleased; if github {...}` derivation), verified absent via `grep`. No further existing code is left unused by this change.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Usage-error diagnostic: `shll check-updates: invalid --source value "x" (valid: released, github)` via an `invalidSourceErrFmt` format constant interpolating the source constants | Intake pins the shape (names offending value + valid set) and grants apply the exact wording; format-constant form keeps code-quality's no-magic-strings rule | S:70 R:90 A:90 D:80 |
| 2 | Confident | New test named `TestCheckUpdates_UnknownSourceValueUsageError`, probing with `"bogus"` | Intake specifies the conversion and the `"bogus"` probe; name follows the file's `TestCheckUpdates_*` convention | S:65 R:95 A:90 D:85 |
| 3 | Certain | The skill-bundle embed copy is `src/cmd/shll/skill/skill.md` (guard `TestSkillEmbedMatchesCanonical`), not `src/cmd/shll/standards/skill.md` as the intake states — the latter is the *skill standard* document, untouched here | `scripts/sync-standards.sh` read directly: it copies `docs/site/skill.md` → `src/cmd/shll/skill/skill.md`; running the script satisfies the intake's intent either way | S:80 R:95 A:95 D:90 |
| 4 | Confident | Test call sites use the explicit constants (`sourceReleased`/`sourceGithub`), never `""` | Intake states it explicitly ("the default is applied by the flag layer, so tests pass the explicit constant") | S:75 R:95 A:90 D:90 |
| 5 | Confident | In-file comments that name the removed flags (`--released`/`--github`) are swept to backend-name form (`released`/`github` backend) where they would otherwise describe nonexistent flags | Clean break coherence; low-risk comment-only edits within the file already being touched | S:55 R:95 A:85 D:80 |

5 assumptions (1 certain, 4 confident, 0 tentative).
