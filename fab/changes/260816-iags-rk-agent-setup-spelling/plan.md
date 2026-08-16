# Plan: Switch run-kit delegation to the `rk agent setup` spelling

**Change**: 260816-iags-rk-agent-setup-spelling
**Intake**: `intake.md`

## Requirements

### CLI: run-kit delegation spelling

#### R1: Delegation uses the two-token `agent setup` family
`delegateRunKitAgentSetup` SHALL invoke the run-kit hook wiring as `run-kit agent setup [--uninstall] [--yes]` — the post-PR-#620 two-token command family (minimum run-kit v3.16.23) — instead of the deprecated single-token `run-kit agent-setup`. The two-token prefix SHALL live in a named constant/variable (code-quality: no magic strings). The binary name stays `runKitToolName` (`run-kit`), the flag order stays `[--uninstall] [--yes]`, and the invocation stays a `proc.RunForeground` call (Constitution I).

- **GIVEN** run-kit ≥ v3.16.23 is installed and the user runs `shll agent-setup`
- **WHEN** the install path delegates run-kit's hook wiring
- **THEN** the subprocess argv is `run-kit agent setup` (plus `--yes` when forwarded), and no deprecation warning is printed by run-kit

- **GIVEN** the user runs `shll agent-setup --uninstall --yes`
- **WHEN** the uninstall path delegates
- **THEN** the subprocess argv is `run-kit agent setup --uninstall --yes`

#### R2: No probe, no fallback — existing degradation paths unchanged
The delegation SHALL NOT add a version probe or an old-spelling fallback retry. The existing degradation behavior MUST remain byte-for-byte in structure: `proc.ErrNotFound` → silent skip (Constitution V); any other error or non-zero exit → stderr warning with `(continuing)`, never failing the skill placement.

- **GIVEN** run-kit is not on PATH
- **WHEN** the delegation runs
- **THEN** it returns silently with no output and the placement result is unaffected

- **GIVEN** an older run-kit (< v3.16.23) without the `agent` family
- **WHEN** the delegation runs
- **THEN** run-kit's own unknown-command error appears on inherited stderr, shll prints its non-fatal `exited N (continuing)` warning, and skill placement still succeeds

#### R3: Diagnostics, help text, and comments name the new spelling
The two stderr diagnostics in `delegateRunKitAgentSetup` SHALL say `run-kit agent setup` (matching the command actually run). The cobra `Long` help text, the file-header comment, and the `agentSetupSub` doc comment SHALL be updated: `agentSetupSub` remains solely shll's own subcommand token (`Use: "agent-setup"`, `refreshArgv`) and its comment MUST no longer claim it is shared with the run-kit delegation. `refreshPlacedAgentSkills`/`refreshArgv` are NOT modified (they invoke `shll agent-setup`, which is not renamed).

- **GIVEN** the delegation's child exits non-zero
- **WHEN** shll prints its warning
- **THEN** the message reads `…: run-kit agent setup exited N (continuing)`

#### R4: Tests pin the two-token argv
`agent_setup_test.go`'s fake-runner assertions SHALL expect the two-token prefix — install `["agent","setup"]`, uninstall `["agent","setup","--uninstall"]`, yes-forwarding `["agent","setup","--yes"]` and `["agent","setup","--uninstall","--yes"]` — and the stderr-message assertions SHALL match the new diagnostic wording. All existing tests pass.

- **GIVEN** the test suite runs (`go test ./cmd/shll/`)
- **WHEN** the delegation tests execute against the fake runner
- **THEN** they assert the two-token argv shapes and pass

### Non-Goals

- No change to `refreshPlacedAgentSkills` / `refreshArgv` (`shll agent-setup` self-invocation is un-renamed; the fix lands transitively via the re-exec'd binary).
- No rename of shll's own `agent-setup` subcommand.
- No switch of the delegation binary from `run-kit` to the `rk` alias.

### Design Decisions

#### No probe, no old-spelling fallback for rk < v3.16.23
**Decision**: Switch the delegation argv plainly to `agent setup`; rely on the existing warn-and-continue adjunct behavior for older rk.
**Why**: The delegation is already best-effort (ErrNotFound → silent skip; other failures → `(continuing)` warning). The dominant exposure path — `shll update`'s end-of-run refresh — runs after the roster loop has just upgraded rk to latest, so the new spelling exists by construction; fresh machines get latest rk from brew. A blind retry-on-nonzero cannot distinguish "unknown command" from a genuine setup failure and would re-run a failing (possibly prompting) setup twice; a `--help` capability probe adds a subprocess to every run to protect a one-day-old version boundary.
**Rejected**: Old-spelling fallback retry (indistinguishable failure signal, double-runs real failures); `--help` capability probe per run (over-engineering for a best-effort adjunct); version-parse gate (same cost, more parsing).
*Introduced by*: 260816-iags-rk-agent-setup-spelling

## Tasks

### Phase 2: Core Implementation

- [x] T001 In `src/cmd/shll/agent_setup.go`: add the named two-token prefix (e.g. `runKitAgentSetupArgs = []string{"agent", "setup"}` with a doc comment naming PR #620 and min rk v3.16.23), switch `delegateRunKitAgentSetup`'s argv build to it, update its two stderr diagnostics to `run-kit agent setup`, and leave the ErrNotFound/warn-continue structure untouched <!-- R1, R2, R3 -->
- [x] T002 In `src/cmd/shll/agent_setup.go`: update prose — the `agentSetupSub` doc comment (no longer shared with the run-kit delegation), the file-header comment, the `Long` help text, and `delegateRunKitAgentSetup`'s doc comment — to the `run-kit agent setup` spelling <!-- R3 -->
- [x] T003 In `src/cmd/shll/agent_setup_test.go`: update the delegation argv assertions (install, uninstall, `--yes` forwarding ×3) to the two-token prefix and the stderr-string assertions to the new wording; run `go test ./cmd/shll/ -run 'TestAgentSetup'` then the full package <!-- R4 -->
- [x] T004 In `src/cmd/shll/agent_setup.go`: fix the user-facing `agentSetupYesUsage` string (~line 147) — it still reads "pass --yes to the run-kit agent-setup delegation"; switch to the `run-kit agent setup` spelling (shows in `shll agent-setup --help`) <!-- rework: review must-fix — A-004 unmet, stale spelling in --help output --> <!-- R3 -->
- [x] T005 Sweep the stale `run-kit agent-setup` delegation spelling in shipped docs: `src/cmd/shll/skill/skill.md` (lines ~27, ~38) + its canonical `docs/site/skill.md` sync copy; `src/cmd/shll/standards/skill.md` (~70, ~74) + canonical `docs/site/standards/skill.md`; `README.md` (~245, ~265). Keep embedded/canonical pairs byte-identical (drift-guard tests); re-run `go test ./cmd/shll/ -count=1` <!-- rework: review should-fix — shipped docs factually stale, clear and low-effort --> <!-- R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `delegateRunKitAgentSetup` builds `run-kit agent setup [--uninstall] [--yes]` via a named constant/variable; no `agent-setup` token remains in the delegated argv
- [x] A-002 R3: `agentSetupSub` still backs shll's own `Use: "agent-setup"` and `refreshArgv`, and its doc comment no longer claims the run-kit delegation shares it

### Behavioral Correctness

- [x] A-003 R2: ErrNotFound silent-skip and warn-`(continuing)` non-fatal paths are structurally unchanged — no probe, no fallback retry added
- [x] A-004 R3: Both stderr diagnostics say `run-kit agent setup`; help text and comments carry no stale `run-kit agent-setup` spelling — verified rework cycle 1 (T004): `agentSetupYesUsage` now reads "pass --yes to the run-kit agent setup delegation …" (src/cmd/shll/agent_setup.go:148); remaining `run-kit agent-setup` hits are deliberate historical references (install.go/install_test.go describe the *former* nudge; agent_setup.go:128 documents the deprecated prior spelling)

### Scenario Coverage

- [x] A-005 R4: Tests assert the two-token argv for install, uninstall, and both `--yes` forwarding shapes, plus the updated exit-code warning string; `go test ./cmd/shll/` passes
- [x] A-008 R3: No shipped user/agent-facing text names `run-kit agent-setup` as the delegation target — `agentSetupYesUsage`, the skill bundle (embedded + canonical), the skill standard (embedded + canonical), and README are all on the `run-kit agent setup` spelling, with embedded/canonical pairs byte-identical (drift-guard tests pass) — verified rework cycle 1 (T005): both pairs byte-identical via `cmp`; `go test ./... -count=1` passes

### Code Quality

- [x] A-006 Pattern consistency: the new prefix follows the file's named-constant convention (agentSetupSub/runKitToolName style, code-quality "no magic strings")
- [x] A-007 No unnecessary duplication: the two-token prefix is defined once and used by both install and uninstall delegation paths

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Two-token prefix as a package-level `var runKitAgentSetupArgs = []string{"agent", "setup"}` (or equivalent named form) rather than two string constants | Matches the file's named-token convention; a slice is the natural shape for an argv prefix; exact form left to apply | S:80 R:95 A:95 D:90 |

1 assumptions (1 certain, 0 confident, 0 tentative).
