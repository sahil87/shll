# Plan: Roster Shell-Init Refresh

**Change**: 260509-tn8v-roster-shellinit-refresh
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

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
