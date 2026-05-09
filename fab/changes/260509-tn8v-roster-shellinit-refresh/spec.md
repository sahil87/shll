# Spec: Roster Shell-Init Refresh

**Change**: 260509-tn8v-roster-shellinit-refresh
**Created**: 2026-05-09
**Affected memory**: `docs/memory/cli/shell-init.md`, `docs/memory/cli/commands.md`

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
- **WHEN** a reader compares the Go literal in `commands.md`'s `Hardcoded tool roster` section to `src/cmd/shll/tools.go`'s `Roster`
- **THEN** the two are identical except for whitespace and comment differences
- **AND** every integrator's `ShellInit` argv ends in the `"<shell>"` placeholder

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

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Add `tu` to shell-init composition with argv `["tu", "shell-init", "<shell>"]`. | Confirmed from intake #1; user-supplied description states this verbatim; Constitution III locks the roster as the seam. | S:95 R:80 A:95 D:95 |
| 2 | Certain | Rename `wt`'s `ShellInit` argv from `["wt", "shell-setup"]` to `["wt", "shell-init", "<shell>"]`. | Confirmed from intake #2; reflects upstream `wt` rename that has already shipped; user explicitly stated this. | S:95 R:80 A:95 D:95 |
| 3 | Certain | No backward-compatibility fallback for `wt shell-setup`. | Confirmed from intake #3; user explicitly excluded this; Constitution V already provides graceful degradation. | S:95 R:60 A:90 D:95 |
| 4 | Certain | Composition order remains `fab-kit, rk, tu, hop, wt, idea`; integrators emit in order `tu, hop, wt` (incidental). | Confirmed from intake #4; user stated `tu`'s position is incidental; current `tools.go` already places `tu` between `rk` and `hop`. | S:95 R:90 A:95 D:95 |
| 5 | Certain | Change type is `feat` (genuinely new tool integration via `tu`). | Confirmed from intake #5; user-supplied description explicitly classifies this as `feat`. | S:95 R:95 A:95 D:95 |
| 6 | Certain | No new top-level subcommand; Constitution VII surface remains `update`, `shell-init`, `version`. | Confirmed from intake #6; user stated this; constitution principle is preserved. | S:95 R:95 A:95 D:95 |
| 7 | Certain | Test strategy is per-tool linear skip-path coverage (one "only X installed" test per integrator), not combinatorial. | Confirmed from intake #7; user explicitly directed this with the exact test names; design decision #1 documents the rationale. | S:95 R:70 A:90 D:90 |
| 8 | Certain | Constitution remains untouched. | Confirmed from intake #12; user stated "Constitution implications: none." Mechanical roster refresh under existing principles. | S:95 R:95 A:95 D:95 |
| 9 | Certain | Memory file `cli/commands.md` IS modified (not just "potentially"). | Upgraded from intake #10 Confident to Certain after direct inspection: `commands.md` reproduces `wt`'s argv (`["wt", "shell-setup"]`) and the "wt shell-setup takes no shell arg" footnote — both must update. | S:95 R:80 A:95 D:90 |
| 10 | Certain | The all-installed test is renamed `TestShellInit_ZshAllIntegratorsInstalled`; per-tool tests drop the shell prefix (`OnlyTuInstalled`, `OnlyHopInstalled`, `OnlyWtInstalled`). | Upgraded from intake #8 Confident to Certain via Design Decision #4: drop the shell prefix from the per-tool tests (shell choice incidental to skip-path semantics) but retain `Zsh` on the all-installed test (its body exercises substitution). | S:90 R:75 A:90 D:85 |
| 11 | Certain | `fab/project/context.md` tool-roster table updated during hydrate, not apply. | Upgraded from intake #9 Confident to Certain via Design Decision #5: project-level reference docs belong with their related memory changes (hydrate stage); apply stays scoped to code + tests + memory. | S:90 R:80 A:90 D:90 |
| 12 | Certain | `TestShellInit_SubToolFailure` argv expectations updated; matcher recognizes `wt shell-init <shell>`; failure scenario stays on `hop` (or any tool — interchangeable). | Upgraded from intake #11 Confident to Certain after direct inspection: matcher key is `req.Name + req.Args` joined; updating the expected canned-output map key from `wt shell-setup` to `wt shell-init zsh` is mechanical. The failure scenario is currently keyed on `hop shell-init zsh` and stays. | S:90 R:80 A:90 D:85 |
| 13 | Certain | The Long help text in `shell_init.go` (`"Today, hop and wt are the only roster tools..."`) is updated to mention `tu`. | Discovered during spec generation: `shell_init.go:23` carries a Long help block naming the integrators. Leaving it stale would make `shll shell-init --help` documentation diverge from reality. Mechanical fix, low blast radius. | S:90 R:90 A:95 D:90 |

13 assumptions (13 certain, 0 confident, 0 tentative, 0 unresolved).
