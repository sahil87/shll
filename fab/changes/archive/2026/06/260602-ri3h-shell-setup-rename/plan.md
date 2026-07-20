# Plan: Rename shell-setup command (shell-install becomes back-compat alias)

**Change**: 260602-ri3h-shell-setup-rename
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### CLI: Canonical command name

#### R1: `shell-setup` is the canonical command name with `shell-install` as a cobra alias
The command factory SHALL declare `Use: "shell-setup [shell]"` and SHALL add `Aliases: []string{"shell-install"}`. The command MUST remain reachable under both names; the alias dispatches to the same `*cobra.Command`.

- **GIVEN** the root command built via `newRootCmd()`
- **WHEN** a user runs `shll shell-setup ...` or `shll shell-install ...`
- **THEN** both resolve to the same command and execute identical behavior

#### R2: Full Go identifier rename off the `ShellInstall` stem
The source file `shell_install.go` SHALL be renamed (via `git mv`) to `shell_setup.go`. The factory `newShellInstallCmd` SHALL become `newShellSetupCmd`; the top-level run helper `runShellInstall` SHALL become `runShellSetup`; and the mode helpers `runShellInstallDefault`/`runShellInstallPrint`/`runShellInstallUninstall` SHALL become `runShellSetupDefault`/`runShellSetupPrint`/`runShellSetupUninstall`.

- **GIVEN** the renamed source file
- **WHEN** the package is built
- **THEN** no identifier carries the `ShellInstall` stem and `go build ./...` succeeds

#### R3: User-facing help and message text uses the canonical name
The factory's `Short`/`Long` help text SHALL present `shll shell-setup ...` as the canonical usage (one alias mention is permitted for discoverability). User-facing runtime message/error prefixes inside the command (`shll shell-install:`) SHALL flip to `shll shell-setup:`.

- **GIVEN** a user who ran `shll shell-setup`
- **WHEN** an error or success message is emitted
- **THEN** the message is prefixed `shll shell-setup:` and help examples reference `shll shell-setup`

### CLI: Root registration

#### R4: Root registers the renamed factory and updates the usage line
`root.go` SHALL call `newShellSetupCmd()` in `AddCommand(...)`. The `rootLong` usage line SHALL flip `shll shell-install [shell]` to canonical `shll shell-setup [shell]`, preserving column alignment with sibling lines.

- **GIVEN** `shll --help`
- **WHEN** the subcommand list renders
- **THEN** the line reads `shll shell-setup [shell]  append the shell-init eval line to your rc file (idempotent)` aligned with siblings

### Tests

#### R5: Test file renamed; existing cases pass against renamed seams
`shell_install_test.go` SHALL be renamed (via `git mv`) to `shell_setup_test.go`. Test-internal identifiers carrying the stem (helper `runShellInstallCmd` → `runShellSetupCmd`; all `newShellInstallCmd()` call sites → `newShellSetupCmd()`) SHALL be updated. Existing assertions on the `shll shell-install:` stderr prefix SHALL flip to `shll shell-setup:` to conform to R3. All existing test cases MUST keep passing.

- **GIVEN** the renamed test file
- **WHEN** `go test ./cmd/shll/` runs
- **THEN** all pre-existing tests pass

#### R6: New alias back-compat test
A new test SHALL assert the `shell-install` alias resolves to the same `*cobra.Command` as `shell-setup` — e.g. `root.Find([]string{"shell-install"})` and `root.Find([]string{"shell-setup"})` return the same command. An end-to-end `SetArgs([]string{"shell-install", "--print", ...})` execution check is an acceptable addition.

- **GIVEN** the root command
- **WHEN** `Find` is called with each name
- **THEN** both return the identical `*cobra.Command` pointer

#### R7: `TestNoProcImports` reads the renamed source file
The `TestNoProcImports` guard SHALL read `shell_setup.go` (its hardcoded filename reference updated from `shell_install.go`), and SHALL continue to assert the file imports neither `internal/proc` nor `os/exec`.

- **GIVEN** the renamed source file
- **WHEN** `TestNoProcImports` runs
- **THEN** it reads `shell_setup.go` and passes (no proc/exec imports)

### Code comments

#### R8: `brew.go` explanatory comment names the renamed file
The `TestNoProcImports` explanatory comment in `brew.go` (~line 92) that names `shell_install.go` SHALL be updated to `shell_setup.go`. Comment-only; no behavior change.

- **GIVEN** `brew.go`
- **WHEN** a reader inspects the `ensureTapTrust` doc comment
- **THEN** it references `shell_setup.go`

### Docs: README

#### R9: README canonical references flipped, alias noted, anchors kept consistent
`README.md` SHALL flip canonical `shll shell-install` references to `shll shell-setup` at: the feature bullet, the Quick-start example, the section header `### shll shell-install — wire the rc file (recommended)` and its example block, and the troubleshooting references. The section-header rename changes its in-page anchor; any in-page link pointing at that header SHALL be kept in sync (verified: no in-page link currently targets it, so no link-target edit is required — the unaffected `--trust-tap` subsection anchor must not be disturbed). A one-line note that `shell-install` still works as an alias SHALL be added under the section header. Out-of-scope verbatim items (sentinel block, the `eval "$(shll shell-init …)"` line, `shll shell-init`) MUST NOT change.

- **GIVEN** the rendered README
- **WHEN** a reader follows the canonical references and the in-page links
- **THEN** canonical refs read `shll shell-setup`, the alias is noted once, and all `#...` fragment links still resolve

### Non-Goals

- The sentinel constants (`openSentinel`/`closeSentinel`, `legacyOpenSentinel`/`legacyCloseSentinel`) — command-name-agnostic / legacy migration machinery.
- `evalLineFmt` / `evalLinePrefix` and the README sentinel-block code samples — these reference the separate `shll shell-init` command. Leave verbatim.
- The `shll shell-init`, `install`, `update`, `version` commands — untouched.
- `docs/memory/cli/shell-install.md` and `docs/memory/index.md` — HYDRATE-stage work, not apply.

### Design Decisions

1. **`shell-setup` canonical, `shell-install` alias**: single cobra `Aliases` field — *Why*: zero-runtime-cost back-compat, native cobra support — *Rejected*: keeping `shell-install` canonical (explicitly rejected by user).
2. **Flip the `shll shell-install:` runtime message prefixes to `shll shell-setup:`**: — *Why*: user-facing diagnostics must agree with the canonical name a user typed; the `Use:` line and message prefixes should be consistent — *Rejected*: leaving prefixes as `shll shell-install:` (would desync help/diagnostics). Existing test assertions on the literal prefix flip too (tests conform to spec, never weakened).

## Tasks

### Phase 1: Renames (git mv to preserve history)

- [x] T001 `git mv src/cmd/shll/shell_install.go src/cmd/shll/shell_setup.go` <!-- R2 -->
- [x] T002 `git mv src/cmd/shll/shell_install_test.go src/cmd/shll/shell_setup_test.go` <!-- R5 -->

### Phase 2: Core rename in source

- [x] T003 In `src/cmd/shll/shell_setup.go`: set `Use: "shell-setup [shell]"`, add `Aliases: []string{"shell-install"}`; rename `newShellInstallCmd` → `newShellSetupCmd`, `runShellInstall` → `runShellSetup`, `runShellInstallDefault/Print/Uninstall` → `runShellSetupDefault/Print/Uninstall`; flip `Short`/`Long` canonical examples to `shll shell-setup` (note alias once); flip all `shll shell-install:` runtime message prefixes to `shll shell-setup:` <!-- R1 R2 R3 -->
- [x] T004 In `src/cmd/shll/root.go`: change `AddCommand(... newShellInstallCmd() ...)` → `newShellSetupCmd()`; flip the `rootLong` usage line to `shll shell-setup [shell]` keeping alignment <!-- R4 -->

### Phase 3: Tests & comment

- [x] T005 In `src/cmd/shll/shell_setup_test.go`: rename helper `runShellInstallCmd` → `runShellSetupCmd` and all call sites; update all `newShellInstallCmd()` → `newShellSetupCmd()`; update `TestNoProcImports` to read `shell_setup.go` and its comment to name `shell_setup.go`; update the root-registration test map key/comment if it references `shell-install` (it should track `shell-setup` now); flip any stderr-prefix assertions to `shll shell-setup:` <!-- R5 R7 -->
- [x] T006 Add a new alias back-compat test in `src/cmd/shll/shell_setup_test.go` asserting `newRootCmd()`'s `Find([]string{"shell-install"})` and `Find([]string{"shell-setup"})` return the same `*cobra.Command` <!-- R6 -->
- [x] T007 In `src/cmd/shll/brew.go` (~line 92): update the `TestNoProcImports` explanatory comment `shell_install.go` → `shell_setup.go` <!-- R8 -->

### Phase 4: Docs

- [x] T008 In `README.md`: flip canonical `shll shell-install` refs to `shll shell-setup` (feature bullet ~L10, Quick-start ~L23, section header ~L67 + example block ~L70-74, troubleshooting ~L101/107/174/177); add one-line alias note under the section header; verify no in-page link targets the renamed header (none does) and leave the `--trust-tap` subsection anchor + sentinel/eval-line samples verbatim <!-- R9 -->

### Phase 5: Build & verify

- [x] T009 From `src/`: run `gofmt -l .`, `go vet ./...`, `go build ./...`, `go test ./cmd/shll/` — all must pass <!-- R1 R2 R3 R4 R5 R6 R7 -->

## Execution Order

- T001, T002 (renames) before all source/test edits.
- T003, T004 before T009. T005, T006, T007, T008 before T009.
- T009 last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: Factory declares `Use: "shell-setup [shell]"` and `Aliases: []string{"shell-install"}`
- [x] A-002 R2: `shell_install.go` is renamed to `shell_setup.go` (git-tracked rename) and no `ShellInstall`-stem identifier remains in the package source
- [x] A-003 R3: `Short`/`Long` help shows `shll shell-setup` canonically; runtime message prefixes read `shll shell-setup:`
- [x] A-004 R4: `root.go` registers `newShellSetupCmd()` and `rootLong` shows the aligned `shll shell-setup [shell]` line
- [x] A-005 R5: `shell_install_test.go` is renamed to `shell_setup_test.go`; helper and call sites use the `ShellSetup` stem
- [x] A-006 R6: A new test asserts the `shell-install` alias and `shell-setup` resolve to the same `*cobra.Command`
- [x] A-007 R7: `TestNoProcImports` reads `shell_setup.go` and passes
- [x] A-008 R8: `brew.go` explanatory comment names `shell_setup.go`
- [x] A-009 R9: README canonical refs flipped, alias noted once, anchors/links and out-of-scope verbatim samples intact

### Behavioral Correctness

- [x] A-010 R1: Executing via the `shell-install` alias produces identical behavior to `shell-setup` (e.g. `--print` output matches)
- [x] A-011 R3: A failure path (e.g. unsupported shell, mutually-exclusive flags) emits a `shll shell-setup:`-prefixed message

### Scenario Coverage

- [x] A-012 R5/R6: `go test ./cmd/shll/` passes all pre-existing tests plus the new alias test

### Edge Cases & Error Handling

- [x] A-013 R3: Unsupported-shell and `--print`/`--uninstall` mutual-exclusion errors carry the `shll shell-setup:` prefix and their assertions in tests pass

### Code Quality

- [x] A-014 Pattern consistency: New/renamed code follows the package's naming and structural patterns (one-file-per-subcommand, factory naming `new<Name>Cmd`)
- [x] A-015 No unnecessary duplication: No logic duplicated; rename is mechanical, existing helpers reused
- [x] A-016 No magic strings: Sentinel/eval constants left untouched (command-name-agnostic), per code-quality named-constants principle
- [x] A-017 Security (Constitution I): `shell_setup.go` remains file-I/O only — `TestNoProcImports` still guards against `internal/proc`/`os/exec` imports

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `shell-setup` canonical `Use:`; `shell-install` as cobra `Aliases` entry | Explicit user choice; rejected inverse | S:98 R:75 A:95 D:95 |
| 2 | Certain | Full identifier rename (file, factory, run helpers, test file/helpers) off the `ShellInstall` stem | User explicitly chose full internal-consistency rename | S:95 R:70 A:92 D:92 |
| 3 | Confident | Flip the `shll shell-install:` runtime message/error prefixes to `shll shell-setup:` (and conform existing test assertions) | SRAD graded decision: user-facing diagnostics must match the canonical name a user typed; the `Use:` line and prefixes should agree; tests conform to spec rather than being weakened. Orchestrator guidance HIGH-confidence | S:78 R:80 A:82 D:78 |
| 4 | Confident | Help `Long`/`Short` example lines flip to `shll shell-setup`; the `Long` "Modes:" block uses the canonical name (alias mention permitted but not required) | Canonical name in help output is the default; discoverability of the alias is optional | S:78 R:82 A:82 D:75 |
| 5 | Confident | New alias test asserts both names resolve to the same `*cobra.Command` via `root.Find` | Standard cobra alias-coverage idiom; satisfies "alias dispatches to same command" | S:80 R:82 A:80 D:75 |
| 6 | Confident | Update `brew.go` ~L92 comment `shell_install.go` → `shell_setup.go` | Dangling stale source-path reference; comment-only, low risk; consistent with full-rename intent | S:75 R:88 A:82 D:80 |
| 7 | Certain | README canonical refs flipped, alias noted, in-page anchor consistency preserved (verified no link targets the renamed header) | Explicit intake direction; anchor sync is a mechanical entailment; grep confirms no link points at the `### shll shell-install` header | S:92 R:85 A:90 D:88 |
| 8 | Certain | `docs/memory/cli/shell-install.md` + memory index left untouched (HYDRATE-stage) | Explicit out-of-scope direction; hydrate owns memory-file lifecycle | S:95 R:80 A:92 D:90 |
| 9 | Certain | `TestRoot_ShellInstallRegistered` map/comment updated to track `shell-setup` (cobra `Name()` returns the first word of `Use`, now `shell-setup`) | Mechanical consequence of the `Use:` rename — `sub.Name()` returns `shell-setup`, so the test's tracked key must flip or the test fails | S:90 R:85 A:90 D:88 |

9 assumptions (5 certain, 4 confident, 0 tentative).
