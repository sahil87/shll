# Plan: Teardown shll.ai push wiring (shll.ai now pulls)

**Change**: 260603-7huv-teardown-shllai-push
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### CI: Release workflow help-push teardown

#### R1: Remove the help-push transport from `release.yml`
The release workflow SHALL NOT contain the three help-push transport steps — "Build native binary for help-dump", "Generate help/shll.json", and "Publish to shll.ai". The `release.yml` file MUST be retained (not deleted): the `release` job keeps its purpose (cross-compile, GitHub Release, Homebrew tap). The workflow-level `permissions: contents: write` and the `HOMEBREW_TAP_TOKEN`-authed "Update Homebrew tap" step MUST remain untouched.

- **GIVEN** the current `release.yml` with the four-part release job (cross-compile, GitHub Release, help-push 3 steps, Homebrew tap)
- **WHEN** the teardown is applied
- **THEN** the three help-push steps are gone and `SHLLAI_TOKEN` is no longer referenced anywhere in the workflow
- **AND** the "Cross-compile", "Determine release notes base tag", "Create GitHub Release", and "Update Homebrew tap" steps remain byte-equivalent to before
- **AND** the file still parses as valid YAML and the `release` job retains a clear purpose

### CLI: help-dump envelope alignment

#### R2: Drop `captured_at` from the emitted envelope
The `help-dump` command SHALL emit the envelope `{tool, version, schema_version, root}` and MUST NOT emit a `captured_at` field. The `CapturedAt` struct field, its `CapturedAt: capturedAt()` assignment, the `capturedAt()` function, the `capturedAtLayout` constant, and the now-unused `"time"` import MUST all be removed. Every other behavior of `help-dump` — `Hidden: true` self-exclusion, programmatic tree walk, prune-before-render, byte-for-byte `nodeText`, version-from-binary, child filtering — MUST be preserved exactly.

- **GIVEN** a built `shll` binary
- **WHEN** `shll help-dump` is run
- **THEN** the JSON object has keys `tool`, `version`, `schema_version`, `root` and does NOT have `captured_at`
- **AND** `tool == "shll"`, `schema_version == 1`, and `root` is present
- **AND** the output remains valid JSON written to stdout with a single trailing newline

#### R3: Update help-dump tests for the new envelope
The test file `help_dump_test.go` SHALL reflect the new envelope: the `TestHelpDump_CapturedAtShape` test and the now-unused `capturedAtRE` regexp (and the `regexp` import if it becomes unused) MUST be removed; `TestHelpDump_StructuralDeterminism` MUST no longer reference `captured_at` in its rationale (the two dumps are simply byte-identical); `TestHelpDump_ContractShape` MUST assert `captured_at` is ABSENT from the output. All other tests (contract shape, text byte-for-byte, self-exclusion, version passthrough, Execute-path regression `dumpViaExecute`) MUST be kept so the contract surface stays protected.

- **GIVEN** the updated `help_dump.go`
- **WHEN** `go test ./cmd/shll/... -run HelpDump` is run
- **THEN** all help-dump tests pass
- **AND** there is no test asserting the presence/shape of `captured_at`
- **AND** a test asserts the absence of a `captured_at` key in the dump output

### Docs: memory alignment (hydrate-stage — NOT executed during apply)

#### R4: Correct memory docs to reflect the pull model and dropped `captured_at`
At the hydrate stage (not apply), `docs/memory/ci/release-workflow.md`, `docs/memory/cli/help-dump-contract.md`, and `docs/memory/ci/index.md` SHALL be corrected to describe shll.ai pulling via `help-dump` (push wiring + `SHLLAI_TOKEN` gone), drop `captured_at` from the envelope/field table/test list, and remove `capturedAt`/`capturedAtLayout`/`captured_at`-test references. The backlog item `ep4z` reconciliation is a hydrate/archive concern.

- **GIVEN** the apply changes are complete
- **WHEN** the hydrate stage runs
- **THEN** the three memory docs are corrected (this requirement is OUT OF SCOPE for apply per the orchestrator directive)

### Non-Goals

- Deleting `release.yml` entirely — the `release` job retains purpose.
- Touching the cross-compile, GitHub Release, or Homebrew-tap steps.
- Changing `HOMEBREW_TAP_TOKEN` or workflow-level `permissions`.
- Deleting the `SHLLAI_TOKEN` repo secret — a post-merge manual repo-settings action, flagged in the PR, not a code change.
- Editing `docs/memory/**` or `fab/backlog.md` during apply — those happen at hydrate.

### Design Decisions

1. **Drop `captured_at` as a contract-mandated exception to "do not touch help-dump"**: the teardown directive's own envelope invariant specifies `{tool, version, schema_version, root}` and "do not emit `captured_at`" — *Why*: §3 of the contract says the output MUST NOT include it, and `captured_at`'s only purpose (the date-granular value powering the CI no-op guard) dies with the push CI. — *Rejected*: keeping `captured_at` (leaves the envelope non-conformant with the pull contract shll.ai validates against).

## Tasks

### Phase 2: Core Implementation

- [x] T001 Remove the three help-push steps ("Build native binary for help-dump", "Generate help/shll.json", "Publish to shll.ai") from `.github/workflows/release.yml`; keep cross-compile, release-base, Create GitHub Release, and Update Homebrew tap intact; leave workflow-level `permissions` and `HOMEBREW_TAP_TOKEN` untouched <!-- R1 -->
- [x] T002 In `src/cmd/shll/help_dump.go`: remove the `CapturedAt` field from `helpDoc`, the `CapturedAt: capturedAt()` assignment in `runHelpDump`, the `capturedAt()` function, the `capturedAtLayout` constant, and the unused `"time"` import — leaving the envelope `{tool, version, schema_version, root}` and everything else exactly as-is <!-- R2 -->
- [x] T003 In `src/cmd/shll/help_dump_test.go`: remove `TestHelpDump_CapturedAtShape` and the `capturedAtRE` regexp (and the `regexp` import if unused); reword `TestHelpDump_StructuralDeterminism` to drop the `captured_at` rationale; add an absence assertion for `captured_at` to `TestHelpDump_ContractShape`; keep all other tests <!-- R3 -->

### Phase 3: Integration & Edge Cases

- [x] T004 Verify: `cd src && go build ./...`; `go test ./cmd/shll/... -run HelpDump`; `go test ./...`; build the binary and confirm `help-dump | jq 'has("captured_at")'` is `false` and `.tool=="shll" and .schema_version==1 and has("root")` succeeds; confirm `SHLLAI_TOKEN`/shll.ai push paths are absent from the workflow <!-- R1 R2 R3 -->

### Phase 4: Memory (hydrate-stage — DO NOT execute during apply)

- [ ] T005 [HYDRATE-ONLY] Correct `docs/memory/ci/release-workflow.md`, `docs/memory/cli/help-dump-contract.md`, `docs/memory/ci/index.md` for the pull model + dropped `captured_at`; reconcile backlog `ep4z`. Deferred to hydrate per orchestrator directive — NOT executed in apply <!-- R4 -->

## Execution Order

- T001, T002, T003 are independent edits (different files); execute then T004 verifies all three.
- T005 is explicitly deferred to the hydrate stage and is NOT executed during apply.

## Acceptance

### Functional Completeness

- [ ] A-001 R1: `release.yml` contains no "Build native binary for help-dump", "Generate help/shll.json", or "Publish to shll.ai" step and no `SHLLAI_TOKEN` reference; the file is retained and valid YAML.
- [ ] A-002 R2: `shll help-dump` emits `{tool, version, schema_version, root}` with no `captured_at`; `help_dump.go` has no `CapturedAt`/`capturedAt`/`capturedAtLayout`/`time` references.
- [ ] A-003 R3: `go test ./cmd/shll/... -run HelpDump` passes; `TestHelpDump_CapturedAtShape` is gone; a test asserts `captured_at` absence.

### Behavioral Correctness

- [ ] A-004 R1: cross-compile, GitHub Release, release-base, and Homebrew-tap steps plus workflow `permissions`/`HOMEBREW_TAP_TOKEN` are unchanged.
- [ ] A-005 R2: all preserved help-dump behaviors (self-exclusion, prune-before-render, byte-for-byte text, version-from-binary, child filtering) still pass their tests.

### Removal Verification

- [ ] A-006 R2: `jq 'has("captured_at")'` on the dump returns `false`; no dead `captured_at` code/import remains.

### Scenario Coverage

- [ ] A-007 R2: `help-dump | jq -e '.tool=="shll" and .schema_version==1 and has("root")'` exits 0.

### Code Quality

- [ ] A-008 Pattern consistency: edits follow existing file conventions (named constants retained, struct field-order contract intact, hop-style code).
- [ ] A-009 No unnecessary duplication: no new helpers introduced; the change is a pure removal (and drops the `time` dependency from `help_dump.go`).
- [ ] A-010 Magic strings/constants: `helpDumpTool` and `helpDumpSchemaVersion` named constants are preserved (code-quality.md anti-pattern: no magic strings).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- T005 (memory) is intentionally left unchecked — it is a hydrate-stage task, not an apply task.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Remove `capturedAtRE` regexp alongside `TestHelpDump_CapturedAtShape`, and drop the `regexp` import if it becomes unused. | `capturedAtRE` exists solely for the removed test; leaving it would be dead code (a `go vet`/compile concern for an unused package-level var is tolerated, but the import would be unused and fail to compile). The intake says drop the `captured_at` test; the regexp + import are its mechanical dependencies. | S:96 R:90 A:97 D:95 |
| 2 | Certain | Add the `captured_at` absence assertion to `TestHelpDump_ContractShape` (rather than a new standalone test). | The intake says `TestHelpDump_ContractShape` "must now assert absence"; it already builds the raw bytes + decoded doc, so the absence check belongs there with the other top-level-key assertions. Minimizes test surface churn. | S:90 R:88 A:92 D:88 |
| 3 | Certain | Defer all `docs/memory/**` and `fab/backlog.md` edits (T005/R4) to the hydrate stage; apply executes only code/workflow/test changes. | Explicit orchestrator directive: "memory updates happen at the HYDRATE stage, not apply." The plan records them for traceability but apply leaves them unexecuted. | S:99 R:95 A:99 D:98 |

3 assumptions (3 certain, 0 confident, 0 tentative, 0 unresolved).
