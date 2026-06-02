# Plan: Roster Shell-Init Refresh

**Change**: 260509-tn8v-roster-shellinit-refresh
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

## Non-Goals

- No backward-compatibility shim for `wt shell-setup` — no subcommand probing, no fallback argv list. Stale-`wt` users degrade via the existing eval-safety contract.
- No changes to `update.go`, `version.go`, `brew.go`, `main.go`, `root.go`, or `internal/proc` — this change is scoped to shell-init composition.
- No new top-level subcommand. Constitution VII surface remains `update`, `shell-init`, `version`.
- No designed sequencing between `tu`, `hop`, `wt` — order between them is incidental, not a contract.
- No constitution amendments.

## cli/shell-init: Composition Roster

### Requirement: Roster integrators

The composition roster SHALL include `tu`, `hop`, and `wt` as the three shell-integrating tools. Each integrator's `ShellInit` argv MUST invoke the sub-tool's own `shell-init <shell>` subcommand with the placeholder substitution token.

The integrator argvs SHALL be:

| Tool | `ShellInit` argv |
|------|------------------|
| `tu`  | `["tu", "shell-init", "<shell>"]`  |
| `hop` | `["hop", "shell-init", "<shell>"]` |
| `wt`  | `["wt", "shell-init", "<shell>"]`  |

The non-integrating roster entries (`fab-kit`, `rk`, `idea`) MUST retain an empty `ShellInit` slice and continue to be skipped by the composition loop.

The full roster order MUST remain `fab-kit, rk, tu, hop, wt, idea`.

#### Scenario: All three integrators emit in roster order

- **GIVEN** `tu`, `hop`, and `wt` are all installed via brew
- **AND** each sub-tool's `shell-init <shell>` returns its own canned stdout
- **WHEN** the user runs `shll shell-init zsh`
- **THEN** stdout contains `tu`'s output, then `hop`'s output, then `wt`'s output, in that exact order
- **AND** stderr is empty
- **AND** the exit code is 0

#### Scenario: wt's argv uses the renamed subcommand

- **GIVEN** `wt` is installed at a version that supports `wt shell-init <shell>`
- **WHEN** `shll shell-init zsh` invokes `wt`'s entry
- **THEN** the subprocess argv passed to `proc.Run` is exactly `["wt", "shell-init", "zsh"]`
- **AND** is NOT `["wt", "shell-setup"]`

#### Scenario: tu's argv uses the new subcommand

- **GIVEN** `tu` is installed at a version that supports `tu shell-init <shell>`
- **WHEN** `shll shell-init bash` invokes `tu`'s entry
- **THEN** the subprocess argv passed to `proc.Run` is exactly `["tu", "shell-init", "bash"]`

### Requirement: Per-tool independent skip path

The composition loop MUST treat each integrator's installation state independently. When only one integrator is installed, that tool's stdout SHALL appear in the output and the other two SHALL be silently skipped (no stderr output, exit 0). When no integrators are installed, stdout SHALL be empty and the exit code SHALL be 0.

#### Scenario: Only tu installed

- **GIVEN** `tu` is installed; `hop` and `wt` are not
- **WHEN** the user runs `shll shell-init zsh`
- **THEN** stdout contains exactly `tu`'s shell-init output
- **AND** stderr is empty
- **AND** the exit code is 0

#### Scenario: Only hop installed

- **GIVEN** `hop` is installed; `tu` and `wt` are not
- **WHEN** the user runs `shll shell-init bash`
- **THEN** stdout contains exactly `hop`'s shell-init output
- **AND** stderr is empty
- **AND** the exit code is 0

#### Scenario: Only wt installed

- **GIVEN** `wt` is installed; `tu` and `hop` are not
- **WHEN** the user runs `shll shell-init zsh`
- **THEN** stdout contains exactly `wt`'s shell-init output
- **AND** stderr is empty
- **AND** the exit code is 0

#### Scenario: No integrators installed

- **GIVEN** none of `tu`, `hop`, or `wt` are installed
- **WHEN** the user runs `shll shell-init zsh`
- **THEN** stdout is empty
- **AND** stderr is empty
- **AND** the exit code is 0

### Requirement: Eval-safety preserved across the roster change

The eval-safety contract (Constitution V; existing Design Decision #6) MUST hold for the new roster. Specifically:

- A failed sub-tool invocation (including a stale `wt` returning a non-zero exit on the new `shell-init` subcommand) MUST NOT contribute any bytes to stdout.
- Sub-tool failure MUST emit a single `shll shell-init: <tool>: <err>` line on stderr and continue with the remaining roster entries.
- The composition loop MUST exit 1 (via `errSilent`) when at least one sub-tool failed, and 0 when all installed sub-tools succeeded (or none were installed).

#### Scenario: One integrator fails, others succeed

- **GIVEN** `tu`, `hop`, and `wt` are all installed
- **AND** `hop`'s `shell-init zsh` invocation returns a non-nil error
- **WHEN** the user runs `shll shell-init zsh`
- **THEN** stdout contains exactly `tu`'s output followed by `wt`'s output, with no fragment of `hop`'s stdout
- **AND** stderr contains a line mentioning `hop`
- **AND** the exit code is 1

#### Scenario: Stale wt without backward-compat shim

- **GIVEN** `wt` is installed at an older version that still uses `wt shell-setup`
- **WHEN** the user runs `shll shell-init zsh`
- **THEN** `wt`'s sub-tool invocation fails (because `wt shell-init zsh` is unknown to the old binary)
- **AND** `wt`'s stdout fragment is dropped from `shll`'s stdout
- **AND** stderr contains a line mentioning `wt`
- **AND** the exit code is 1
- **AND** the user's shell still loads correctly (other integrators concatenated normally)

### Requirement: Argument validation unchanged

`shll shell-init` MUST continue to validate its shell argument before invoking any sub-tool. A missing or unsupported shell SHALL produce empty stdout, a usage line on stderr, and exit code 2 (via `errExitCode`). The supported shell list (`zsh`, `bash`) is unchanged by this refresh.

#### Scenario: Missing shell argument

- **GIVEN** the user invokes `shll shell-init` with no positional argument
- **WHEN** cobra dispatches to `RunE`
- **THEN** stdout is empty
- **AND** stderr contains a usage line
- **AND** the exit code is 2

#### Scenario: Unsupported shell

- **GIVEN** the user invokes `shll shell-init fish`
- **WHEN** the shell argument is validated
- **THEN** stdout is empty
- **AND** stderr contains a message about the unsupported shell
- **AND** the exit code is 2

## cli/shell-init: Test Coverage

### Requirement: Per-tool linear skip-path coverage

The test suite for `shell-init` SHALL include exactly one "only X installed" test per integrator (`tu`, `hop`, `wt`), proving that integrator's stdout is emitted in isolation while the other two are silently skipped. The suite MUST NOT use combinatorial pair tests (`{tu, hop}`, `{hop, wt}`, etc.) — combinations of independent skip paths are covered transitively by the per-tool tests plus the all-installed assembly test.

The final test list SHALL be:

| Test | Coverage |
|------|----------|
| `TestShellInit_ZshAllIntegratorsInstalled` (renamed from `TestShellInit_ZshBothInstalled`) | All three integrators installed → roster-ordered concatenation, exit 0 |
| `TestShellInit_OnlyTuInstalled` (new) | Only `tu` installed → only `tu`'s stdout, exit 0 |
| `TestShellInit_OnlyHopInstalled` (renamed from `TestShellInit_BashHopOnly`) | Only `hop` installed → only `hop`'s stdout, exit 0 |
| `TestShellInit_OnlyWtInstalled` (new) | Only `wt` installed → only `wt`'s stdout, exit 0 |
| `TestShellInit_NoIntegratingToolsInstalled` (unchanged) | None installed → empty stdout, exit 0 |
| `TestShellInit_UnsupportedShell` (unchanged) | Bad shell arg → exit 2 |
| `TestShellInit_MissingShellArg` (unchanged) | No shell arg → exit 2 |
| `TestShellInit_DeterministicOrder` (extended) | All three installed → byte-identical output across two runs, in roster order |
| `TestShellInit_SubToolFailure` (argv updated) | One integrator fails → others succeed, eval-safety holds, exit 1 |

#### Scenario: Per-tool tests use the renamed argv shapes

- **GIVEN** the test file's fake `proc.Runner` matcher recognizes `tu shell-init <shell>` and `wt shell-init <shell>`
- **AND** the matcher does NOT recognize the obsolete `wt shell-setup` argv
- **WHEN** any per-tool test runs
- **THEN** the matcher correctly returns the canned stdout for the installed integrator
- **AND** the assertion that other integrators are silently skipped passes

#### Scenario: All-installed test asserts roster order

- **GIVEN** all three integrators are installed in the fake runner
- **AND** each integrator returns a distinct canned stdout fragment
- **WHEN** `runShellInit` runs
- **THEN** the captured stdout is exactly `tu`'s fragment + `hop`'s fragment + `wt`'s fragment, concatenated with no separator
- **AND** the exit code is 0

#### Scenario: Determinism includes tu

- **GIVEN** all three integrators are installed in the fake runner
- **WHEN** `runShellInit` runs twice with identical inputs
- **THEN** the two stdout buffers are byte-identical
- **AND** the byte sequence matches the roster order (`tu` first, then `hop`, then `wt`)

### Requirement: Argument-validation tests unaffected

`TestShellInit_UnsupportedShell` and `TestShellInit_MissingShellArg` MUST remain functionally unchanged. They cover argument validation, which is roster-independent.

#### Scenario: Argument-validation tests still pass

- **GIVEN** the test bodies are not modified by this change
- **WHEN** the test suite runs after the roster refresh
- **THEN** both tests pass
- **AND** their exit-code assertions (2) are unchanged

## cli/commands: Roster Documentation

### Requirement: Memory reflects new roster shape

The hardcoded roster snippet in `docs/memory/cli/commands.md` MUST be updated to reproduce the new `Roster` Go literal verbatim, including `tu`'s and `wt`'s new `ShellInit` argvs. The accompanying prose about argv placeholders MUST be updated — every integrator's argv now includes the `<shell>` placeholder, so the previous "`wt shell-setup` takes no shell arg" bullet is removed.

The roster invariants statement (six tools, order matters, named `formulaPrefix`) is unchanged.

#### Scenario: commands.md roster snippet matches tools.go

- **GIVEN** `commands.md` has been hydrated for this change
- **WHEN** a reader compares the snippet in `commands.md`'s `Hardcoded tool roster` section to `src/cmd/shll/tools.go`'s `Roster`
- **THEN** the two represent the same six entries in the same order, with the same per-tool `Name`, `Formula`, and `ShellInit` argv values
- **AND** every integrator's `ShellInit` argv ends in the `"<shell>"` placeholder
- **AND** the snippet MAY use a normalized reader-friendly form (fully-expanded formula strings, literal `"<shell>"` placeholder) rather than reproducing the Go literal verbatim with named constants (`formulaPrefix`, `shellPlaceholder`) — the snippet is documentation, not a Go source copy

### Requirement: shell-init.md reflects three-integrator world

`docs/memory/cli/shell-init.md` MUST be updated in three places:

1. The argv substitution table SHALL list all three integrators (`tu`, `hop`, `wt`) with their post-substitution argvs for `zsh`.
2. The "Composition order" prose SHALL reflect three integrators emitting in order `tu, hop, wt`, and SHALL note that `tu`'s position is incidental rather than designed.
3. The covered-test list SHALL match the test list in the Test Coverage requirement above.

The eval-safety section, Design Decision #6 description, exit-code table, and cross-references are unchanged.

#### Scenario: shell-init.md substitution table is complete

- **GIVEN** `shell-init.md` has been hydrated for this change
- **WHEN** a reader inspects the argv substitution table
- **THEN** the table has rows for `tu`, `hop`, and `wt`
- **AND** each row's "After substitution (zsh)" column reflects the literal substitution of the placeholder token with `zsh`

#### Scenario: shell-init.md test list matches the test file

- **GIVEN** `shell-init.md` has been hydrated for this change
- **WHEN** a reader compares the test list in `shell-init.md` to `shell_init_test.go`
- **THEN** every test in the file appears in the memory list with a matching scenario summary
- **AND** no obsolete test names (e.g., `TestShellInit_ZshBothInstalled`, `TestShellInit_BashHopOnly`) remain

## Design Decisions

1. **Per-tool linear test coverage, not combinatorial.**
   - *Why*: Each integrator's installed/missing branch is independent in the composition loop. The "only X installed" test for each of the three integrators, plus the all-installed assembly test and the all-missing test, transitively covers every meaningful combination without N-wise blow-up. Five focused tests beat seven (or fifteen) overlapping ones.
   - *Rejected*: pairwise combinations (`{tu, hop}`, `{tu, wt}`, `{hop, wt}`) — every pairwise combination is the sum of two independent skip paths the per-tool tests already prove. Adding pair tests bloats the suite without strengthening the invariant.

2. **No legacy fallback for `wt shell-setup`.**
   - *Why*: Constitution V's eval-safety contract already gives stale-`wt` users a graceful degradation: failed sub-tool drops its stdout, error to stderr, shll exits 1, shell still loads. Adding a probe-and-fallback path would introduce subcommand sniffing, error-class discrimination, and a transient maintenance burden — Constitution III ("wrap, don't reinvent") leans against it.
   - *Rejected*: try `wt shell-init <shell>` first, retry with `wt shell-setup` on "unknown command" error. Adds complexity for a transient compatibility window the existing contract already handles.

3. **Composition order between `tu`, `hop`, `wt` is incidental.**
   - *Why*: User explicitly stated `tu`'s position relative to `hop` and `wt` does not matter for correctness. Leaving `tu` first (its natural roster position) avoids a contrived re-ordering. The roster-order invariant is preserved (deterministic byte-identical output across runs), but the *specific* order between integrators carries no semantic weight.
   - *Rejected*: explicitly placing `tu` last to preserve the historical "hop, then wt" ordering. Pointless — the existing ordering is already incidental, and reordering for sentiment violates the "natural roster order" contract.

4. **Drop the `Zsh`/`Bash` prefix from renamed test names.**
   - *Why*: The existing `TestShellInit_ZshBothInstalled` and `TestShellInit_BashHopOnly` named the chosen shell, but the shell choice is incidental to what the test proves (per-tool skip paths). The new convention names the integrator state. Specifically: the all-installed test keeps `Zsh` (its body uses zsh) but switches `BothInstalled` → `AllIntegratorsInstalled`; the per-tool tests drop the shell prefix entirely (`OnlyTuInstalled`, `OnlyHopInstalled`, `OnlyWtInstalled`).
   - *Rejected*: keep shell prefixes everywhere (`TestShellInit_ZshOnlyTuInstalled`, etc.) — the shell isn't what the test asserts. Rejected: drop `Zsh` from the all-installed test too — it's the only test that exercises the substitution path explicitly, so naming the shell is informative there.

5. **`fab/project/context.md` updated during hydrate, not apply.**
   - *Why*: `context.md`'s tool roster table reads "no" for `tu`'s shell-init column and lists `wt`'s as `shell-setup` — both stale. Updating it under hydrate keeps apply scoped to code + tests + memory and keeps project-level reference docs in the same step as the related memory updates.
   - *Rejected*: defer to a separate housekeeping change — table is small, change is small, splitting adds a follow-up tax.
   - *Rejected*: update under apply — apply is for code and tests; project-level reference docs in `fab/project/` belong with their related memory changes.

## Tasks

### Phase 1: Roster + Help Text

- [x] T001 Update `src/cmd/shll/tools.go` `Roster`: set `tu`'s `ShellInit` to `[]string{"tu", "shell-init", shellPlaceholder}` and change `wt`'s `ShellInit` from `[]string{"wt", "shell-setup"}` to `[]string{"wt", "shell-init", shellPlaceholder}`. Roster order must remain `fab-kit, rk, tu, hop, wt, idea`.
- [x] T002 Update the `Tool.ShellInit` doc comment in `src/cmd/shll/tools.go` so the obsolete `wt shell-setup` example no longer appears (every integrator now uses the `<shell>` placeholder).
- [x] T003 Update the Long help block in `src/cmd/shll/shell_init.go` so it names `tu`, `hop`, and `wt` as the integrating roster tools (replace the current "Today, hop and wt are the only roster tools with shell integration" sentence).

### Phase 2: Test Suite Refresh

- [x] T004 In `src/cmd/shll/shell_init_test.go`, rename `TestShellInit_ZshBothInstalled` to `TestShellInit_ZshAllIntegratorsInstalled`, add `tu` to the `installedFormulas` map, add `"tu shell-init zsh": "## tu init\nexport TU=1\n"` to the outputs map, change `wt` outputs key from `"wt shell-setup"` to `"wt shell-init zsh"`, and update the `want` string to the roster-ordered concatenation `tu` + `hop` + `wt`.
- [x] T005 In `src/cmd/shll/shell_init_test.go`, rename `TestShellInit_BashHopOnly` to `TestShellInit_OnlyHopInstalled`. Test body unchanged — still asserts only `hop`'s output, exit 0, empty stderr.
- [x] T006 [P] Add `TestShellInit_OnlyTuInstalled` to `src/cmd/shll/shell_init_test.go`: only `tu` formula installed, outputs `"tu shell-init zsh": "## tu only\n"`, asserts stdout equals `"## tu only\n"`, exit 0, empty stderr.
- [x] T007 [P] Add `TestShellInit_OnlyWtInstalled` to `src/cmd/shll/shell_init_test.go`: only `wt` formula installed, outputs `"wt shell-init zsh": "## wt only\n"`, asserts stdout equals `"## wt only\n"`, exit 0, empty stderr.
- [x] T008 Update `TestShellInit_DeterministicOrder` in `src/cmd/shll/shell_init_test.go` to install all three integrators (`tu`, `hop`, `wt`), add `"tu shell-init zsh": "TU\n"`, change `wt` key from `"wt shell-setup"` to `"wt shell-init zsh"`, and update the expected concatenation to `"TU\nHOP\nWT\n"`.
- [x] T009 Update `TestShellInit_SubToolFailure` in `src/cmd/shll/shell_init_test.go`: change the canned-output map key from `"wt shell-setup"` to `"wt shell-init zsh"`. Failure scenario stays on `hop` (errors map keyed `"hop shell-init zsh"`).

### Phase 3: Verification

- [x] T010 Run `go test ./...` from `src/` and confirm all package tests pass (in particular `TestShellInit*` tests).

## Acceptance

### Functional Completeness

- [ ] A-001 Roster integrators: `tools.go` `Roster` declares `tu`, `hop`, and `wt` with `ShellInit` argvs `["tu", "shell-init", shellPlaceholder]`, `["hop", "shell-init", shellPlaceholder]`, and `["wt", "shell-init", shellPlaceholder]` respectively; non-integrators (`fab-kit`, `rk`, `idea`) keep an empty `ShellInit` slice; roster order is `fab-kit, rk, tu, hop, wt, idea`.
- [ ] A-002 Per-tool independent skip path: when only one integrator is installed, only that tool's stdout appears, stderr is empty, exit code 0; when none are installed, stdout is empty and exit code is 0.
- [ ] A-003 Eval-safety preserved: when one integrator's invocation fails, its stdout fragment is dropped, a `shll shell-init: <tool>: <err>` line is written to stderr, the loop continues with remaining integrators, and the command exits via `errSilent` (exit 1).
- [ ] A-004 Argument validation unchanged: missing shell argument and unsupported shell each produce empty stdout, a usage line on stderr, and exit code 2 via `errExitCode`.

### Behavioral Correctness

- [ ] A-005 wt argv shape: the subprocess argv passed to `proc.Run` for `wt` is exactly `["wt", "shell-init", "zsh"]` (or the requested shell), never `["wt", "shell-setup"]`.
- [ ] A-006 tu argv shape: the subprocess argv passed to `proc.Run` for `tu` is exactly `["tu", "shell-init", "<shell>"]` with the placeholder substituted to the requested shell.
- [ ] A-007 Help text reflects three integrators: `shell_init.go` Long help mentions `tu` alongside `hop` and `wt`.

### Removal Verification

- [ ] A-008 No `wt shell-setup` literal remains in `src/cmd/shll/tools.go` or `src/cmd/shll/shell_init_test.go` (both the Roster argv and any test fake-runner output keys are migrated).
- [ ] A-009 Obsolete test names (`TestShellInit_ZshBothInstalled`, `TestShellInit_BashHopOnly`) are removed from the test file.

### Scenario Coverage

- [ ] A-010 `TestShellInit_ZshAllIntegratorsInstalled` proves roster-ordered concatenation (`tu` then `hop` then `wt`) when all three are installed, with empty stderr and nil error.
- [ ] A-011 `TestShellInit_OnlyTuInstalled` proves `tu`'s stdout is emitted in isolation when `hop` and `wt` are missing.
- [ ] A-012 `TestShellInit_OnlyHopInstalled` proves `hop`'s stdout is emitted in isolation when `tu` and `wt` are missing.
- [ ] A-013 `TestShellInit_OnlyWtInstalled` proves `wt`'s stdout is emitted in isolation when `tu` and `hop` are missing (using the new `wt shell-init <shell>` argv).
- [ ] A-014 `TestShellInit_NoIntegratingToolsInstalled` proves empty stdout when no integrators are installed (unchanged).
- [ ] A-015 `TestShellInit_DeterministicOrder` proves byte-identical output across two runs and that the byte sequence matches roster order across all three integrators.
- [ ] A-016 `TestShellInit_SubToolFailure` proves failed integrator's stdout fragment is dropped, stderr mentions the failing tool, and `runShellInit` returns `errSilent` — using updated `wt shell-init zsh` argv.

### Edge Cases & Error Handling

- [ ] A-017 Stale-`wt` graceful degradation: a stale `wt` binary that does not understand `shell-init <shell>` causes its sub-tool invocation to fail, its stdout is dropped, stderr contains a `wt`-tagged diagnostic, and the user's shell still loads from the remaining integrators. Covered indirectly by `TestShellInit_SubToolFailure` semantics.

### Code Quality

- [ ] A-018 No magic strings: the `<shell>` placeholder uses the existing `shellPlaceholder` named constant; no new ad-hoc string literals are introduced for tool names or argv tokens.
- [ ] A-019 Subprocess invocation routes through `internal/proc` only (Constitution I); no direct `os/exec` use in changed code.
- [ ] A-020 Wrap, don't reinvent (Constitution III): `tu`'s shell-init composition shells out to `tu shell-init <shell>` rather than reimplementing `tu` shell logic in shll.
- [ ] A-021 Pattern consistency: new test functions follow the existing fake-runner / `installFakeRunner` / buffer-based assertion pattern in `shell_init_test.go`.
- [ ] A-022 No unnecessary duplication: the per-tool tests reuse the existing `shellInitFake` helper; no new helper is introduced unless reuse demands it.
- [ ] A-023 Test integrity: tests assert spec behavior rather than implementation details; the implementation was not contorted to satisfy a test fixture.

## Notes

- Memory updates (`docs/memory/cli/shell-init.md`, `docs/memory/cli/commands.md`) and `fab/project/context.md` updates are explicitly out of scope for apply per spec Non-Goals — they belong to the hydrate stage.
- `proc.Runner` matcher behavior is updated implicitly by changing the input map keys passed to `shellInitFake` — no structural change to the helper is required.
