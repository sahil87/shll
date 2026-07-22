# Plan: Per-Tool Skill Argv Override

**Change**: 260722-ktk5-skill-argv-override
**Intake**: `intake.md`

## Requirements

### CLI: skill argv override

#### R1: Roster `Skill` argv-override field
The `Tool` struct (`src/cmd/shll/tools.go`) MUST gain a `Skill []string` field — the argv of the tool's skill-bundle invocation — documented in the same style as `Update`/`ShellInit`. An empty slice means the default `{Name, "skill"}`; only tools whose skill surface diverges from their roster `Name` populate it. Exactly one roster entry — fab-kit — SHALL populate it, with `Skill: []string{"fab", "skill"}` (fab-kit serves its bundle on the `fab` router binary).

- **GIVEN** the hardcoded `Roster`
- **WHEN** the fab-kit entry is read
- **THEN** its `Skill` field is exactly `{"fab", "skill"}`
- **AND** every other roster entry's `Skill` field is empty (the default is derivable, so only the exception is stated — ShellInit-style semantics)

#### R2: `skillArgv` resolver
`src/cmd/shll/skill.go` MUST gain a small named resolver `skillArgv(tool Tool) []string` that returns the roster `Skill` override when set, else the default `{tool.Name, skillSubcommand}`. No call site may open-code the argv composition.

- **GIVEN** a roster tool with an empty `Skill` field (e.g. `wt`)
- **WHEN** `skillArgv(tool)` is called
- **THEN** it returns `{"wt", "skill"}`

- **GIVEN** the fab-kit roster entry
- **WHEN** `skillArgv(tool)` is called
- **THEN** it returns `{"fab", "skill"}`

#### R3: Bundle passthrough resolves the argv
`writeSkillBundle` MUST invoke the resolved argv — `argv := skillArgv(tool)` → `proc.RunCaptured(subCtx, argv[0], argv[1:]...)` — instead of the hardcoded `tool.Name, skillSubcommand`. `shll skill fab-kit` therefore invokes `fab skill`, never a literal `fab-kit skill`. Everything else in the function is untouched: error notices keep printing `tool.Name` (the user-facing name stays `fab-kit`), and the suppress-and-rewrap failure classification is unchanged.

- **GIVEN** a fake runner serving `fab skill` with a bundle on stdout, exit 0
- **WHEN** `shll skill fab-kit` runs
- **THEN** the recorded invocation is exactly `fab skill` via `TransportCaptureAll`
- **AND** stdout is byte-identical to the child's bundle, stderr empty

#### R4: Topic passthrough resolves the argv (aliasing-safe)
`writeSkillTopic` MUST use the same resolution with the topic appended — `proc.RunCaptured(subCtx, argv[0], ...)` where the final args are `argv[1:]` plus the topic. The final args MUST be built without mutating the roster slice (no `append` aliasing into `Tool.Skill`'s backing array). `shll skill fab-kit <topic>` therefore invokes `fab skill <topic>`. The two-arg failure classification (verbatim stderr/exit-code propagation on `code > 0`, the `code < 0` deadline guard) is unchanged.

- **GIVEN** a fake runner serving `fab skill dispatch` with a topic page on stdout, exit 0
- **WHEN** `shll skill fab-kit dispatch` runs
- **THEN** the recorded invocation is exactly `fab skill dispatch` via `TransportCaptureAll`
- **AND** stdout is byte-identical, and fab-kit's roster `Skill` slice is unchanged afterward

#### R5: Softened `skillUnsupportedFmt`
The constant MUST change from `"shll skill: %s does not support 'skill' yet — run 'shll update'"` to `"shll skill: %s does not support 'skill' — its installed version may predate it (try 'shll update %s')"` — hedged (names the likely cause without promising the remedy) and scoped to the one tool. The format now takes the tool name twice; the call site (one, in `writeSkillBundle`; the topic path propagates verbatim and never uses the constant) SHALL pass `tool.Name, tool.Name`.

- **GIVEN** a fake runner where `<tool> skill` exits non-zero with its own stderr
- **WHEN** `shll skill <tool>` runs
- **THEN** stderr is the one-line softened notice naming the tool and suggesting `shll update <tool>`
- **AND** the child's raw stderr is still suppressed, exit code 1

#### R6: Unchanged surrounding behavior
Everything outside the two subprocess sites and the one constant MUST stay unchanged: the bare glossary keeps the PATH-only `toolInstalled` probe on `tool.Name` (installedness ≠ skill argv; the fab-kit formula ships both binaries, so probing `fab-kit --version` remains correct), the `rk`→`run-kit` alias resolution, the `shll` self-token embed path, the unknown-name usage error, and both forms' failure classification.

- **GIVEN** the existing `skill_test.go` suite
- **WHEN** `go test ./cmd/shll/` runs after the change
- **THEN** every existing test still passes (with only the unsupported-notice wording assertion updated per R5)

### Non-Goals

- Renaming the roster entry to `fab` — `fab-kit` is the formula leaf, the probed binary, and the user-facing name across `list`/`version`/`update`.
- Hardcoding a `name == "fab-kit"` branch in `skill.go` — the roster stays the single source of truth (Constitution III).
- Any change to `update`/`install`/`version`/`list`/`doctor` (they key off `Name`/`Formula`/`Update`), the CLI surface, help text, `docs/site/skill.md`, or the standards documents.

### Design Decisions

#### Exception-only population of the Skill field
**Decision**: Only fab-kit populates `Skill`; all other tools derive the default from `Name` + `skillSubcommand` via `skillArgv`.
**Why**: The default is derivable, so stating only the exception (ShellInit's "empty means no divergence" semantics) avoids six redundant slices carrying no information.
**Rejected**: Populating all entries (Update-style "all entries populate") — adds noise with no signal; a wrong copy-paste in a redundant slice would be a new bug surface.
*Introduced by*: 260722-ktk5-skill-argv-override

## Tasks

### Phase 1: Core Implementation

- [x] T001 Add the `Skill []string` field to the `Tool` struct in `src/cmd/shll/tools.go` (doc comment in the `Update`/`ShellInit` style) and populate the fab-kit roster entry with `Skill: []string{"fab", "skill"}` <!-- R1 -->
- [x] T002 Add the `skillArgv(tool Tool) []string` resolver to `src/cmd/shll/skill.go` (override when set, else `{tool.Name, skillSubcommand}`) <!-- R2 -->
- [x] T003 Switch `writeSkillBundle`'s subprocess site to the resolved argv (`proc.RunCaptured(subCtx, argv[0], argv[1:]...)`), leaving notices and classification untouched <!-- R3 -->
- [x] T004 Switch `writeSkillTopic`'s subprocess site to the resolved argv with the topic appended, building the final args without aliasing into `Tool.Skill`'s backing array <!-- R4 -->
- [x] T005 Soften `skillUnsupportedFmt` in `src/cmd/shll/skill.go` (new wording, takes the tool name twice) and update the `writeSkillBundle` call site to pass `tool.Name, tool.Name` <!-- R5 -->

### Phase 2: Tests

- [x] T006 [P] Add fab-kit override tests to `src/cmd/shll/skill_test.go`: `shll skill fab-kit` asserts the exact `fab skill` argv (and no literal `fab-kit skill`), stdout byte-identical; `shll skill fab-kit <topic>` asserts the exact `fab skill <topic>` argv and that the roster `Skill` slice is unmutated <!-- R3, R4 -->
- [x] T007 [P] Add a `skillArgv` regression test pinning both branches: a non-override tool (`wt`) yields `{"wt", "skill"}`, fab-kit yields `{"fab", "skill"}` <!-- R2 -->
- [x] T008 [P] Update the existing unsupported-path test's wording assertion (softened notice, doubled tool-name arg → `shll update <tool>` suggestion, child stderr still suppressed) <!-- R5 -->
- [x] T009 Run `cd src && go test ./cmd/shll/` — the full existing suite stays green, pinning the unchanged surrounding behavior <!-- R6 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: The `Tool` struct carries a documented `Skill []string` field; fab-kit is the only roster entry populating it, with exactly `{"fab", "skill"}`
- [x] A-002 R2: `skillArgv` returns the override when `len(tool.Skill) > 0`, else `{tool.Name, skillSubcommand}` — no call site open-codes the argv (verified: `skillSubcommand` now appears only in the resolver; both `RunCaptured` sites use the resolved `argv`)
- [x] A-003 R3: `shll skill fab-kit` invokes `fab skill` via `TransportCaptureAll`, streaming stdout byte-identical on success (`TestSkill_Passthrough_FabKitOverrideInvokesFabSkill`)
- [x] A-004 R4: `shll skill fab-kit <topic>` invokes `fab skill <topic>`; the final args are built without mutating the roster slice (`TestSkillTopic_FabKitOverrideInvokesFabSkillTopic`)
- [x] A-005 R5: `skillUnsupportedFmt` carries the softened wording and the call site passes `tool.Name` twice

### Behavioral Correctness

- [x] A-006 R3: Non-override tools are unaffected — the existing `hop`/`wt`/`run-kit` passthrough tests still assert `<name> skill` argv (full suite green)
- [x] A-007 R5: The unsupported notice no longer asserts a bare `run 'shll update'` imperative; the suggestion is hedged (`may predate`) and scoped to the one tool (`'shll update wt'`)

### Scenario Coverage

- [x] A-008 R3: A test asserts no literal `fab-kit skill` invocation occurs when the override is in play (the fake fails the test on any `fab-kit` invocation, mirroring the rk-alias negative assertion)
- [x] A-009 R4: A test asserts the exact `fab skill <topic>` argv for the topic form
- [x] A-010 R6: The full existing suite passes — classification (suppress-and-rewrap one-arg, verbatim propagation two-arg, the `code < 0` guard), notices printing `tool.Name`, and the glossary's PATH probe on `tool.Name` all unchanged

### Edge Cases & Error Handling

- [x] A-011 R4: Appending the topic does not alias into `Tool.Skill`'s backing array — fab-kit's roster `Skill` slice is byte-equal `{"fab", "skill"}` after a topic invocation (asserted in `TestSkillTopic_FabKitOverrideInvokesFabSkillTopic`; copy-then-append idiom `append(append([]string(nil), argv[1:]...), topic)`)

### Code Quality

- [x] A-012 Pattern consistency: The field doc comment and resolver follow the `Update`/`ShellInit` documentation style; naming matches surrounding code
- [x] A-013 No unnecessary duplication: Both subprocess sites share the single `skillArgv` resolver
- [x] A-014 Subprocess discipline: All invocations remain routed through `internal/proc` with a bounded context (Constitution I) — no raw `os/exec`
- [x] A-015 No magic strings: The default branch reuses the existing `skillSubcommand` constant; no new unnamed literals

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

None — this change adds a new `Tool.Skill` field, a `skillArgv` resolver, and swaps two call sites onto it. The prior open-coded `tool.Name, skillSubcommand` composition at both `RunCaptured` sites is replaced in place (not left as dead code), and no existing symbol, file, or branch becomes redundant.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Aliasing-safe topic args built via a copy-then-append (`append(append([]string(nil), argv[1:]...), topic)`) | Intake mandates "no append aliasing into Tool.Skill's backing array"; the exact idiom is the agent's choice and trivially reversible | S:70 R:95 A:95 D:85 |
| 2 | Certain | No roster-shape test added to `tools_test.go` — the behavior tests in `skill_test.go` pin the fab-kit argv contract | Intake explicitly marks it "not required — behavior tests pin the contract" | S:75 R:95 A:90 D:85 |
| 3 | Certain | The fab-kit override test carries a negative assertion (no literal `fab-kit skill` invocation), mirroring the existing rk-alias test shape | Direct precedent in `TestSkill_Passthrough_RkAliasResolvesToRunKit`; strengthens the pin at zero cost | S:65 R:95 A:90 D:85 |

3 assumptions (3 certain, 0 confident, 0 tentative).
