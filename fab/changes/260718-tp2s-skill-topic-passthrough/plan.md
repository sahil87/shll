# Plan: Skill Topic Passthrough

**Change**: 260718-tp2s-skill-topic-passthrough
**Intake**: `intake.md`

## Requirements

### Composer: `skill [tool] [topic]` grammar

#### R1: Optional second (topic) argument
The `skill` subcommand SHALL accept an optional second positional argument (the topic), changing its arg contract from `cobra.MaximumNArgs(1)` to `cobra.MaximumNArgs(2)`. Three or more args SHALL remain a cobra usage error (exit 2). The `Use` string SHALL become `skill [tool] [topic]`.

- **GIVEN** the `shll skill` command
- **WHEN** it is invoked with two positional args (`shll skill <tool> <topic>`)
- **THEN** cobra accepts them and dispatch reaches the two-arg (topic) path
- **AND WHEN** invoked with three or more args (`shll skill a b c`)
- **THEN** cobra rejects it as a usage error, exit 2 (via the existing `errExitCode`/`isUsageError` wrap)

#### R2: One-arg and zero-arg behavior unchanged
The zero-arg (glossary) and one-arg (bundle) shapes SHALL behave exactly as today; a single arg SHALL still be resolved as a tool name (an unknown one-arg name stays the existing usage exit 2 with the valid-tools list). No grammar ambiguity is introduced by the second arg.

- **GIVEN** `shll skill` with zero args
- **WHEN** dispatched
- **THEN** the installed-only glossary prints (unchanged)
- **AND GIVEN** `shll skill <tool>` with one arg
- **WHEN** dispatched
- **THEN** that tool's bundle is served exactly as before

### Composer: two-arg topic passthrough

#### R3: Verbatim delegation to `<tool> skill <topic>`
For the two-arg form with a roster tool, the composer SHALL invoke `<tool> skill <topic>` verbatim via `proc.RunCaptured` under the existing `skillProbeTimeout` (2s). On success (`code == 0`) the child's captured stdout SHALL be written byte-identical to shll's stdout, and shll's stderr SHALL be empty — preserving the `skill` standard's invocation contract (raw markdown to stdout, stderr empty on success).

- **GIVEN** a roster tool whose `<tool> skill <topic>` exits 0 with bundle bytes on stdout
- **WHEN** `shll skill <tool> <topic>` runs
- **THEN** the captured stdout is streamed byte-identical, stderr is empty, exit 0
- **AND** the subprocess argv is exactly `<tool> skill <topic>` via the capture-all transport

#### R4: Legacy-alias and self-token resolution mirror the one-arg form
Resolution order for the tool arg SHALL mirror the one-arg form: (1) the shared `legacyAliases` rewrite applies before any dispatch (so `shll skill rk <topic>` invokes `run-kit skill <topic>`, never `rk skill <topic>`); (2) an unknown tool name SHALL be the existing usage exit 2 with `validTargets(true)`, checked before any subprocess; (3) `shll skill shll <topic>` SHALL be served in-process (no subprocess) per R6.

- **GIVEN** `shll skill rk <topic>`
- **WHEN** dispatched
- **THEN** the alias rewrites `rk`→`run-kit` and the subprocess argv targets `run-kit skill <topic>`, never a literal `rk`
- **AND GIVEN** `shll skill wombat <topic>` (unknown tool)
- **WHEN** dispatched
- **THEN** it is a usage error exit 2 with the valid-targets list, and no subprocess is spawned

#### R5: Non-zero child exit — propagate stderr verbatim + mirror the exit code
For the two-arg form, when the delegated `<tool> skill <topic>` runs to completion but exits non-zero (`code != 0`), the composer SHALL write the child's captured stderr bytes through to shll's stderr **verbatim** (no swallowing, no rewrapping) and SHALL exit with the **child's own exit code** via `errExitCode{code: childCode}` (empty `msg`). `proc.ErrNotFound` (tool not on PATH) SHALL keep its existing curated `skillNotInstalledFmt` notice + exit 1, classified before the exit-code question; a pre-start I/O failure (`err != nil`, no usable exit code) SHALL keep its existing `shll skill: <tool>: <err>` notice + exit 1. This deliberately diverges from the one-arg form's suppress-and-rewrap classification, which stays exactly as is.

- **GIVEN** a roster tool whose `<tool> skill <topic>` exits 2 with valid-topics text on stderr
- **WHEN** `shll skill <tool> <topic>` runs
- **THEN** shll's stderr carries the child's stderr bytes verbatim and shll exits with the child's exit code (2)
- **AND GIVEN** the same tool is not on PATH (`ErrNotFound`)
- **WHEN** `shll skill <tool> <topic>` runs
- **THEN** shll prints the one-line `skillNotInstalledFmt` notice and exits 1 (classification precedes propagation)

#### R6: `shll skill shll <topic>` — no-topics contract, in-process
`shll skill shll <topic>` SHALL be served in-process (no subprocess — a self-invocation would recurse into the composer). Because shll ships zero topic pages today, it SHALL emit a single stderr line stating shll ships no topic pages and exit with usage exit 2 (matching the sibling unknown-tool usage convention; the standard requires only "non-zero exit, valid topics on stderr" — the valid set is empty).

- **GIVEN** `shll skill shll display` (or any topic)
- **WHEN** dispatched
- **THEN** shll writes one stderr line stating it ships no topic pages, exits 2, and spawns no subprocess

### Teaching surfaces

#### R7: `skillHintLine` mentions the topic form
The bare-glossary trailer `skillHintLine` SHALL mention the two-arg topic form while remaining a single line (it trails the tabwriter table after a blank line).

- **GIVEN** `shll skill` (bare glossary)
- **WHEN** the trailing hint prints
- **THEN** it names both `shll skill <tool>` (bundle) and `shll skill <tool> <topic>` (topic page) on one line

#### R8: `newSkillCmd` `Long` documents the two-arg form and its exit contract
The `Long` help text SHALL document the two-arg form (`shll skill <tool> <topic>`) and its exit contract (on child failure the child's stderr and exit code are propagated), alongside the existing usage block.

- **GIVEN** `shll skill --help`
- **WHEN** the `Long` text renders
- **THEN** it describes the topic form and that a failing child's stderr/exit code are propagated verbatim

#### R9: Bootstrap body step 2 teaches the topic form
The placed bootstrap skill body (`agentSkillContent` step 2 in `agent_setup.go`) SHALL be extended so the placed skill teaches that a tool's core bundle carries a topic index and `shll skill <tool> <topic>` serves a topic page. The frontmatter `description` trailer (`agentSkillDescription()`) SHALL be left unchanged (single-line activation-trigger vocabulary, not a teaching surface).

- **GIVEN** the placed `shll-toolkit` SKILL.md body
- **WHEN** an agent reads step 2
- **THEN** it learns the two-step (`shll skill` → `shll skill <tool>`) still holds AND that `shll skill <tool> <topic>` serves a topic page named in the core bundle's topic index
- **AND** the frontmatter description line is byte-unchanged (its single-line YAML shape and existing trigger clauses are preserved)

#### R10: shll's own bundle (`docs/site/skill.md`) gains the topic form, embed stays byte-identical
The canonical bundle `docs/site/skill.md` grammar line (the `shll skill [tool]` capabilities-map line) and the two-step gotcha SHALL gain the topic form, and `scripts/sync-standards.sh` SHALL be re-run so the embedded copy `src/cmd/shll/skill/skill.md` stays byte-identical (`TestSkillEmbedMatchesCanonical`). The bundle SHALL remain ≤150 lines (`TestSkillBundle_WithinLineBudget`).

- **GIVEN** an edit to `docs/site/skill.md`
- **WHEN** `scripts/sync-standards.sh` is run and `go test ./cmd/shll/` runs
- **THEN** `TestSkillEmbedMatchesCanonical` passes (embed matches canonical) and the ≤150-line budget test still passes

### Non-Goals

- No topic registry, topic knowledge, or valid-topic listing in shll — topic validity and page content stay owned by the tool that ships them (Constitution III; the standard's ownership model).
- No new subcommand and no new file — this is an arg-grammar extension of the existing `skill` subcommand (Constitution VII bar not triggered).
- No edit to the `skill` standard's text (`docs/site/standards/skill.md`) — verified it never references the composer, so the two-arg extension creates no inconsistency; the manager-exception note is a separate `agst`-flagged follow-up, out of scope here.
- The one-arg form's suppress-and-rewrap failure classification (`skillUnsupportedFmt`) is unchanged — the divergent verbatim-propagation behavior applies only to the two-arg form.

### Design Decisions

1. **Two-arg dispatch shape**: `runSkill` gains a third branch (`len(args) == 2`). — *Why*: the intake's explicit dispatch table (`0`→glossary, `1`→bundle, `2`→topic passthrough); mirrors the existing dispatch-on-arg-count structure. — *Rejected*: overloading `writeSkillBundle` with a variadic/optional topic that muddles the two forms' divergent failure classification (the two-arg form propagates verbatim; the one-arg form rewraps) — a distinct `writeSkillTopic` keeps each form's classification legible.
2. **Non-zero child exit mirrors the child's code (vs. flattening to 1)**: return `errExitCode{code: childCode}` with empty `msg`. — *Why*: most faithful to the standard's unknown-topic contract ("non-zero exit with valid topics on stderr" survives the composer unmodified) and keeps usage-vs-operational semantics intact; `translateExit` handles an empty-`msg` `errExitCode` by exiting with the code and writing nothing to stderr, so the child's already-written stderr bytes are the only diagnostic. — *Rejected*: flattening every child failure to exit 1 (loses the child's usage-vs-operational distinction); stderr-sniffing to distinguish predates-`skill` from unknown-topic (fragile — the intake rejects it explicitly).
3. **`shll skill shll <topic>` → usage exit 2**: — *Why*: shll ships no topics; the standard requires only non-zero + valid topics on stderr (empty set here); exit 2 matches the sibling unknown-tool usage convention. — *Rejected*: exit 1 (defensible but the front-runner is 2 for consistency with the unknown-tool sibling).

## Tasks

### Phase 1: Grammar

- [x] T001 In `src/cmd/shll/skill.go`, change `newSkillCmd`'s `Args` to `cobra.MaximumNArgs(2)` and `Use` to `"skill [tool] [topic]"`. <!-- R1 -->

### Phase 2: Core Implementation

- [x] T002 In `src/cmd/shll/skill.go`, extend `runSkill` dispatch with the two-arg branch: `len(args) == 2` → a new `writeSkillTopic(ctx, stdout, stderr, tool, topic)` call; keep the `0`→glossary and `1`→`writeSkillBundle` branches unchanged. <!-- R1 R2 -->
- [x] T003 In `src/cmd/shll/skill.go`, implement `writeSkillTopic`: apply the shared `legacyAliases` rewrite (same guard as `writeSkillBundle`); handle the `shll` self-token via a no-topics path (one stderr line, `errExitCode{code: usageExitCode}`, no subprocess) per R6; unknown tool name → the existing `errExitCode{code: usageExitCode, msg: "...unknown tool..."}` with `validTargets(true)`, no subprocess; a roster tool → `proc.RunCaptured(subCtx, tool.Name, skillSubcommand, topic)` under `skillProbeTimeout`. <!-- R3 R4 R6 -->
- [x] T004 In `src/cmd/shll/skill.go` `writeSkillTopic`, classify the delegated result: `ErrNotFound` → `skillNotInstalledFmt` + `errSilent` (exit 1); pre-start `err != nil` → `shll skill: <tool>: <err>` + `errSilent` (exit 1); `code > 0` → write the child's captured stderr bytes verbatim to stderr then return `errExitCode{code: code}` (empty msg — child code mirrored); `code < 0` (Go's -1 sentinel — child timed out under `skillProbeTimeout` or was signal-killed, NOT a child exit code) → one curated stderr notice (named constant, e.g. `skillTopicTimeoutFmt`) + `errSilent` (exit 1, operational); `code == 0` → `stdout.Write(out)` byte-identical (write error → `errSilent`). Add a named constant for the shll-ships-no-topics stderr message per code-quality.md (no magic strings). Also correct the two stale `internal/proc` doc comments this fix touches on: `proc.go` TransportCaptureAll's caller description (topic form now propagates stderr rather than suppressing) and the `defaultRunner` comment falsely claiming a deadline kill yields a non-ExitError err (it yields `*exec.ExitError` with `ExitCode() == -1`). <!-- R3 R5 R6 --> <!-- rework: review must-fix — propagate branch mirrored -1 into os.Exit → silent exit 255 with zero stderr on a hung/killed child; gate propagation on code > 0 and route code < 0 to an operational exit-1 notice -->

### Phase 3: Teaching surfaces

- [x] T005 In `src/cmd/shll/skill.go`, extend the `skillHintLine` constant to name the topic form on one line, e.g. `Run 'shll skill <tool>' for that tool's full agent skill bundle ('shll skill <tool> <topic>' for a topic page).` <!-- R7 -->
- [x] T006 In `src/cmd/shll/skill.go`, extend `newSkillCmd`'s `Long` text to document the two-arg form and its propagated child stderr/exit-code contract, alongside the existing usage block. <!-- R8 -->
- [x] T007 In `src/cmd/shll/agent_setup.go`, extend `agentSkillContent` body step 2 to teach that a tool's core bundle carries a topic index and `shll skill <tool> <topic>` serves a topic page. Leave `agentSkillDescription()` (frontmatter trailer) byte-unchanged. <!-- R9 -->
- [x] T008 Edit `docs/site/skill.md`: extend the `shll skill [tool]` capabilities-map line (grammar) and the two-step gotcha to mention the `shll skill <tool> <topic>` topic form; keep it within the ≤150-line budget. <!-- R10 -->
- [x] T009 Run `scripts/sync-standards.sh` so `src/cmd/shll/skill/skill.md` is refreshed byte-identical from the edited `docs/site/skill.md`. <!-- R10 -->

### Phase 4: Tests

- [x] T010 [P] In `src/cmd/shll/skill_test.go`, add the two-arg tests: passthrough happy path (fake asserts `<tool> skill <topic>` argv via `TransportCaptureAll`, stdout byte-identical, stderr empty, exit 0); unknown-topic propagation (child exits 2 with valid-topics stderr → shll stderr carries child bytes verbatim, exit mirrors child); timed-out/killed child (fake returns code -1, nil err) → one curated stderr notice, exit 1, no negative code leaked; arg-count contract (3 args → usage exit 2 — drive real cobra); topic-form + not-on-PATH → `skillNotInstalledFmt` exit 1; `shll skill shll <topic>` → in-process error exit 2, no subprocess; `shll skill rk <topic>` → alias resolves to `run-kit skill <topic>`. Update the existing bare-glossary test's `skillHintLine` expectation for the new text. <!-- R1 R3 R4 R5 R6 R7 --> <!-- rework: review must-fix — add the code<0 timeout/killed-child classification test -->
- [x] T011 [P] In `src/cmd/shll/agent_setup_test.go`, update any assertion pinning the bootstrap body text so it accommodates the new step-2 topic wording (keep the existing two-step/standards/no-stanza assertions green; the single-line description assertion stays unchanged). <!-- R9 -->
- [x] T012 Run `cd src && go test ./cmd/shll/` — confirm `TestSkillEmbedMatchesCanonical` and `TestSkillBundle_WithinLineBudget` are green alongside the new/updated tests; then `cd src && go build ./...` and `cd src && go vet ./cmd/shll/`. Also run `cd src && go test ./internal/proc/` (its doc comments changed in T004's rework). <!-- R10 --> <!-- rework: re-verify after the code<0 fix -->

## Execution Order

Phase 1 → Phase 2 → Phase 3 must run in order (T003/T004 depend on T001-T002's grammar; T009 depends on T008's canonical edit). Phase 4 tests (T010, T011 are `[P]` — different files) run after their implementation lands; T012 runs last (drift guard depends on T009's sync).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll skill` accepts an optional topic arg (`MaximumNArgs(2)`, `Use: "skill [tool] [topic]"`); 3+ args is a usage error exit 2.
- [x] A-002 R2: the zero-arg glossary and one-arg bundle forms behave exactly as before; a single unknown arg stays usage exit 2 with the valid-tools list.
- [x] A-003 R3: the two-arg form delegates `<tool> skill <topic>` verbatim via `proc.RunCaptured` under `skillProbeTimeout`, streaming stdout byte-identical on success with empty stderr.
- [x] A-004 R4: `rk` alias rewrites to `run-kit` before dispatch; an unknown tool name is usage exit 2 with `validTargets(true)` and spawns no subprocess.
- [x] A-005 R5: a child that ran to completion with a positive exit propagates the child's stderr bytes verbatim and mirrors the child's exit code (branch gated on `code > 0`); `ErrNotFound`/pre-start failures keep their curated exit-1 notices; a timed-out/signal-killed child (`code < 0`, nil err — the `*exec.ExitError.ExitCode()` -1 sentinel, NOT a child exit code) is routed to a curated operational exit-1 notice (`skillTopicTimeoutFmt`) rather than mirroring -1 into os.Exit(-1)→255. <!-- rework applied: propagate gated on code > 0; code < 0 → skillTopicTimeoutFmt + errSilent (exit 1); proc.go doc comments corrected. -->
- [x] A-006 R6: `shll skill shll <topic>` emits one stderr line, exits 2, and spawns no subprocess.
- [x] A-007 R7: the bare-glossary hint line names both the bundle and topic forms on one line.
- [x] A-008 R8: `newSkillCmd` `Long` documents the two-arg form and its propagated child stderr/exit-code contract.
- [x] A-009 R9: the bootstrap body step 2 teaches the topic form; the frontmatter description is byte-unchanged.
- [x] A-010 R10: `docs/site/skill.md` gains the topic form and the embedded copy is re-synced byte-identical, within the ≤150-line budget.

### Behavioral Correctness

- [x] A-011 R5: the two-arg failure classification diverges from the one-arg form as specified (verbatim propagation vs. suppress-and-rewrap), and the one-arg form's `skillUnsupportedFmt` classification is unchanged.
- [x] A-012 R2: the one-arg tool-resolution path (alias, self-token, unknown) is not altered by the second-arg addition.

### Scenario Coverage

- [x] A-013 R3: passthrough happy-path test asserts the exact `<tool> skill <topic>` argv via `TransportCaptureAll`, byte-identical stdout, empty stderr, exit 0.
- [x] A-014 R5: unknown-topic-propagation test asserts the child's stderr bytes reach shll's stderr verbatim and shll's exit code mirrors the child's.
- [x] A-015 R1: arg-count-contract test asserts `shll skill a b c` (3 args) is a usage error exit 2.

### Edge Cases & Error Handling

- [x] A-016 R5: topic-form + tool not on PATH yields `skillNotInstalledFmt` exit 1 (classification precedes propagation).
- [x] A-017 R6: `shll skill shll <topic>` spawns no subprocess (recursion guard) and exits 2.
- [x] A-018 R4: `shll skill rk <topic>` alias-resolution test asserts the subprocess targets `run-kit`, never a literal `rk`.

### Code Quality

- [x] A-019 Pattern consistency: the two-arg path mirrors `writeSkillBundle`'s structure (alias guard, self-token, roster resolution, bounded `RunCaptured`) and reuses shared helpers (`legacyAliases`, `rosterTool`, `validTargets`, `skillProbeTimeout`); the shll-ships-no-topics message is a named constant (no magic strings).
- [x] A-020 No unnecessary duplication: subprocess work stays on `internal/proc` (Constitution I); delegation is verbatim, not reimplemented (Constitution III/IV); not-installed degradation preserved (Constitution V).
- [x] A-021 R10: the drift guard `TestSkillEmbedMatchesCanonical` and the ≤150-line budget test both pass after the canonical bundle edit and re-sync.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Grammar: `Args: cobra.MaximumNArgs(2)`, `Use: "skill [tool] [topic]"`; ≥3 args stays usage exit 2 | Backlog/intake specify the optional second arg and the >2-args contract verbatim; cobra mechanics deterministic | S:90 R:90 A:95 D:95 |
| 2 | Certain | Two-arg form delegates `<tool> skill <topic>` verbatim via `proc.RunCaptured` under `skillProbeTimeout` (2s); `rk`→`run-kit` alias applies unchanged before dispatch | Intake mandates verbatim delegation; transport, timeout, and alias reuse follow the one-arg form's established pattern exactly | S:85 R:85 A:95 D:90 |
| 3 | Confident | Non-zero child exit on the two-arg form: propagate captured stderr bytes verbatim + mirror the child's exit code via `errExitCode{code: childCode}` (empty msg); `ErrNotFound`/pre-start failures keep their curated exit-1 notices | Intake says "propagate, do not swallow or rewrap"; mirroring the exit code (vs. flattening to 1) is the design call most faithful to the standard's contract; `translateExit` confirmed to exit with the code and write nothing on empty-msg `errExitCode` | S:75 R:80 A:75 D:70 |
| 4 | Confident | `shll skill shll <topic>` → in-process one-line stderr error, usage exit 2, no subprocess | shll has zero topics today; standard requires only non-zero + valid topics on stderr; exit 2 matches the sibling unknown-tool usage convention (1 vs 2 both defensible — 2 chosen for sibling consistency) | S:60 R:85 A:75 D:65 |
| 5 | Confident | Introduce a distinct `writeSkillTopic` helper (vs. an optional-topic overload of `writeSkillBundle`) | Keeps the two forms' divergent failure classification legible (two-arg propagates verbatim; one-arg suppresses-and-rewraps); mirrors the existing per-shape helper structure; naming follows the `writeSkill*` convention | S:70 R:85 A:80 D:70 |
| 6 | Confident | Teaching surfaces updated: `skillHintLine`, bootstrap body step 2, `newSkillCmd` Long, `docs/site/skill.md` (+ embed re-sync); `agentSkillDescription()` frontmatter trailer left byte-unchanged | Intake names the hint line and body step 2; Long/bundle document the same grammar and must not go stale; the frontmatter description is single-line trigger vocabulary, not a teaching surface | S:70 R:90 A:80 D:70 |
| 7 | Confident | No edit to the `skill` standard's text (`docs/site/standards/skill.md`) | Verified the standard never references the composer, so the two-arg extension creates no inconsistency; the manager-exception note is the `agst` intake's separately-flagged follow-up, out of scope | S:65 R:90 A:80 D:70 |
| 8 | Certain | Rework (T004): a deadline/signal-killed child (`code < 0`, nil err — the `*exec.ExitError.ExitCode()` -1 sentinel, verified live) routes to a curated operational exit-1 notice (`skillTopicTimeoutFmt`, a named constant), NOT a mirrored exit code; verbatim propagation is gated to `code > 0` (a child that ran to completion) | Review must-fix + R5's "runs to completion" scoping both mandate this exact behavior; mirroring -1 wraps to `os.Exit(-1)`→process 255 with zero stderr (toolkit exit-code + no-silent-failure violation); exit 1 is the standard's operational code; one obvious interpretation | S:90 R:85 A:95 D:90 |
| 9 | Confident | Rework (T004): `newSkillCmd` `Long` and `docs/site/skill.md` left unchanged — the fix does not alter documented behavior | Both surfaces document the unknown-topic path (`code > 0` verbatim propagation, valid topics on stderr, non-zero exit), which is untouched; the deadline-kill case is an internal edge neither surface describes, so adding a caveat would bloat the help text past the standard's brevity for no truthfulness gain; a one-line doc edit stays trivially reversible if desired later | S:60 R:90 A:85 D:65 |

9 assumptions (3 certain, 6 confident, 0 tentative).

## Deletion Candidates

None — this change adds new functionality without making existing code redundant. (Re-verified at rework-cycle-1 review: `skillUnsupportedFmt` remains live on the one-arg form; `errSilent`/`errExitCode`/`legacyAliases`/`rosterTool`/`validTargets`/`skillProbeTimeout` all gained call sites rather than losing any; no symbol, branch, or config lost its last call site.)
