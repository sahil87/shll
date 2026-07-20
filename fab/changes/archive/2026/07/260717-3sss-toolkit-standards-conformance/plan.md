# Plan: Toolkit Standards Conformance

**Change**: 260717-3sss-toolkit-standards-conformance
**Intake**: `intake.md`

## Requirements

<!-- Requirements derived from intake.md: a runtime-enumerated conformance audit
     of shll against the 4 toolkit standards (principles, help-dump,
     readme-extraction, skill), plus proportionate in-scope fixes and a persisted
     conformance report. Audit target = dev build from repo HEAD; standards text =
     installed shll v0.0.23 (byte-matched to repo by the drift guard). -->

### Audit: Runtime enumeration & mechanical checklists

#### R1: Runtime-authoritative standards enumeration
The audit SHALL enumerate the standards at runtime via `shll standards` and `shll standards <name>` for each listed entry, and treat that list — not the intake's advisory list — as authoritative. The report SHALL cite the audited shll version from `shll version`'s shll row.

- **GIVEN** the apply worker begins the audit
- **WHEN** it runs `shll standards` (installed binary) and `shll version`
- **THEN** it records the 4 entries (`principles`, `help-dump`, `readme-extraction`, `skill`) and audits against shll **v0.0.23**
- **AND** the audit target for behavioral checks is a dev build from repo HEAD (`cd src && go build -o /tmp/shll-audit ./cmd/shll`), not the installed binary

#### R2: help-dump mechanical checklist executed verbatim
The dev build SHALL pass the help-dump standard's own "Verifying conformance" checklist: `help-dump` exits 0, writes valid JSON to stdout only with stderr empty; envelope is `{tool, version, schema_version, root}` with no `captured_at`; `completion`/`help`/hidden commands absent from the tree; `version` reflects the built binary (ldflags), not a literal; a minimal test pins exit 0 + valid JSON + expected `tool`/`schema_version`.

- **GIVEN** the dev build `/tmp/shll-audit`
- **WHEN** `help-dump` is run and its output inspected
- **THEN** every checklist item holds and `help_dump_test.go` passes

#### R3: readme-extraction mechanical checklist executed verbatim
The repo SHALL pass the readme-extraction standard's "Verifying conformance" checklist: README head order (`#` H1 → toolkit blockquote → **badges** → prose); every relative link target points into `docs/site/` (from README), stays inside `docs/site/` (between tree pages), or is absolute; no relative images; no `#gh-*-mode-only` fragments; no site-destined mermaid fences; no `docs/site/` page named `overview`/`readme`/`commands`; README cross-links its `docs/site/` pages and the absolute command-reference URL `https://shll.ai/shll/commands/`.

- **GIVEN** the repo's `README.md` and `docs/site/**`
- **WHEN** each checklist item is checked
- **THEN** all pass, including a contiguous badge run after the toolkit blockquote (matching the 6 sibling repos' convention)

#### R4: principles assessment against actual behavior
Each of the 10 principles SHALL be assessed against the dev build's actual subcommand behavior across the surface (`update`, `install`, `list`, `doctor`, `version`, `changelog`, `shell-init`, `shell-setup`, `standards`, `uninstall`): TTY prompts, stream separation, `--json`/`--dry-run`/`--yes` coverage, exit codes, error wording, idempotency, output volume/caps.

- **GIVEN** the dev build and its subcommand surface
- **WHEN** each principle's obligation is checked against observed behavior
- **THEN** each principle is dispositioned PASS or gap (with a fixed-here / deferred disposition)

### Fixes: In-scope proportionate corrections

#### R5: Usage errors exit with code 2 (principle №4)
Cobra-level usage errors (unknown command, unknown flag/shorthand, wrong arg count, invalid argument) SHALL exit with code **2**, matching the toolkit convention (`0` success, `1` operational failure, `2` usage error) already stated in the principles standard and already honored by `shll shell-init`'s own bad-shell path. The fix MUST NOT change the exit code of operational failures (which stay `1`).

- **GIVEN** the shll binary
- **WHEN** `shll bogus`, `shll list --bogus`, or `shll doctor extra-arg` is run
- **THEN** the process exits **2** and the diagnostic still goes to stderr
- **AND WHEN** an operational failure occurs (e.g. `shll standards nonexistent`, a failed brew call, a doctor FAIL)
- **THEN** the process still exits **1** (unchanged)

#### R6: README badge run added (readme-extraction head structure)
`README.md` SHALL carry a contiguous run of badge lines immediately after the canonical toolkit blockquote and before the intro prose, matching the standard's head order and the byte-identical form used by all 6 sibling toolkit repos (Latest release / Downloads / Stars, pointing at `sahil87/shll`).

- **GIVEN** the README head (`# shll` → blockquote → prose)
- **WHEN** the badge run is inserted after the blockquote
- **THEN** the head reads `# shll` → blockquote → badges → intro prose, and the badge URLs resolve against the shll repo

### Deliverable: Conformance report & deferrals

#### R7: Conformance report persisted in the change folder
A `conformance-report.md` SHALL be written to the change folder with a heading citing the audited shll version, one section per standard, and PASS or per-gap disposition lines (fixed here — naming the fix and the proving file/test, with commit sha added at ship — or deferred to a `[ref]`).

- **GIVEN** the completed audit and fixes
- **WHEN** the report is written
- **THEN** it has one section per standard (`principles`, `help-dump`, `readme-extraction`, `skill`), each PASS or with dispositioned gaps, and the heading names shll v0.0.23

#### R8: skill standard reported deferred, not implemented
The report's `skill` section SHALL read "deferred, not yet adopted" and reference backlog `[agst]`. No `shll skill` subcommand SHALL be implemented in this change.

- **GIVEN** shll has no `shll skill` subcommand and the standard's Adoption section states a tool without one is not yet in violation
- **WHEN** the report's skill section is written
- **THEN** it reads "deferred, not yet adopted" referencing `[agst]`, and `src/cmd/shll/` gains no `skill` command

#### R9: Larger gaps deferred as backlog items
Any gap that would restructure the tool (new subsystem, breaking output-contract change, redesigned command) SHALL be recorded as a `fab/backlog.md` item with a fresh 4-char ID (following the existing item format) and referenced by that ID in the report. Gaps small enough to fix additively (a flag, a rerouted stream, an unhelpful error, a cap notice, an exit code) are fixed here instead.

- **GIVEN** a gap identified during the audit
- **WHEN** its scope is assessed against the "small and additive" threshold
- **THEN** it is either fixed here (additive) or recorded as a new `fab/backlog.md` item and referenced in the report

### Verification tail

#### R10: Tests green; help-dump re-verified if the tree changed
`cd src && go test ./...` SHALL pass. If any fix changes the command tree (new flag/subcommand), the help-dump mechanical checklist SHALL be re-run and `help_dump_test.go` re-confirmed afterward.

- **GIVEN** all fixes are applied
- **WHEN** `go test ./...` runs
- **THEN** it passes
- **AND** since R5 adds no flag/subcommand and R6 touches only the README, the command tree is unchanged — help-dump is re-run once to confirm regardless

### Non-Goals

- Implementing `shll skill` — deferred to `[agst]` (R8).
- Adding `shll version --json` — `version` is human-bug-report-paste by explicit design; `list --json` and `doctor --json` (whose JSON carries per-tool version) cover the programmatic surface. Not a gap under principle №2 (see Assumption 4).
- Restructuring any command's output contract or behavior — additive-only scope (R9).
- A footer-heading split of the README — footer headings are pull-*stop* markers, not required content; the whole README is site-worthy and nothing maintainer-only leaks (see Assumption 3).

### Design Decisions

1. **Usage-exit-2 at the `translateExit` seam + `SetFlagErrorFunc`**: route flag-parse errors through a root `SetFlagErrorFunc` that wraps them in the existing `errExitCode{code: 2}` sentinel, and classify cobra's arg/command usage errors (stable prefixes: `unknown command`, `unknown flag`, `unknown shorthand flag`, `invalid argument`, `accepts `, `requires `) in `translateExit` — *Why*: cobra v1.10.2 exposes a clean hook only for flag errors; arg/command errors are plain `fmt.Errorf` with no typed sentinel, so prefix classification at the single exit seam is the least-invasive root-cause fix (reuses the existing `errExitCode` machinery, one file). — *Rejected*: wrapping every command's `Args` validator (invasive across 11 commands); a bespoke error type per command (churn with no benefit over the shared seam).
2. **Badges copied byte-identically from sibling repos**: use the exact 3-badge shields.io run (Latest release / Downloads / Stars) the other 6 repos carry, re-pointed at `sahil87/shll`. — *Why*: the standard mandates the head order and the toolkit already has one canonical badge form; byte-consistency across repos is the point. — *Rejected*: a bespoke badge set (drifts from the toolkit convention).

## Tasks

### Phase 1: Audit (runtime enumeration + checklists)

- [x] T001 Re-enumerate standards at runtime (`shll standards`, `shll standards <name>` ×4) and capture the audited version (`shll version` shll row = v0.0.23); build the dev binary `cd src && go build -o /tmp/shll-audit ./cmd/shll` <!-- R1 -->
- [x] T002 [P] Execute the help-dump "Verifying conformance" checklist verbatim against `/tmp/shll-audit` (exit 0, JSON to stdout, empty stderr, envelope shape, no `captured_at`, filtered tree, version-from-binary); confirm `help_dump_test.go` <!-- R2 -->
- [x] T003 [P] Execute the readme-extraction "Verifying conformance" checklist verbatim against `README.md` + `docs/site/**` (head order, relative-link/image grep, gh-mode fragments, mermaid, reserved names, cross-links + command-reference URL); compare badge presence against the 6 sibling repos <!-- R3 -->
- [x] T004 [P] Assess all 10 principles against the dev build's subcommand behavior (TTY prompts, stream split, `--json`/`--dry-run`/`--yes` coverage, exit codes, error wording, idempotency, output caps) <!-- R4 -->

### Phase 2: Fixes (additive, proportionate)

- [x] T005 Fix usage-error exit code to 2 in `src/cmd/shll/main.go`: add a root `SetFlagErrorFunc` (in `src/cmd/shll/root.go`) wrapping flag errors in `errExitCode{code: 2}`, and classify cobra arg/command usage errors by stable prefix in `translateExit` → exit 2; keep operational failures at exit 1 <!-- R5 -->
- [x] T006 Add a `main_test.go` (new) pinning the exit-code contract: usage errors (unknown command, unknown flag, bad arg count) → 2; operational failures (unknown standard name, errSilent) → 1; success → 0 <!-- R5 -->
- [x] T007 [P] Add the contiguous 3-badge run to `README.md` immediately after the toolkit blockquote (Latest release / Downloads / Stars, pointing at `sahil87/shll`), matching the sibling-repo form <!-- R6 -->

### Phase 3: Deliverable & deferrals

- [x] T008 Write `fab/changes/260717-3sss-toolkit-standards-conformance/conformance-report.md`: heading cites shll v0.0.23; one section per standard; PASS or per-gap disposition (fixed here — name the fix + proving file/test, commit sha at ship — or deferred to `[ref]`); skill section = "deferred, not yet adopted" → `[agst]` <!-- R7 R8 -->
- [x] T009 If any gap exceeds the additive threshold, add a `fab/backlog.md` item (fresh 4-char ID, existing format) and reference it in the report; otherwise record "no new deferrals beyond [agst]" — outcome: no new deferrals (both gaps additive/fixed here; skill tracked in `[agst]`) <!-- R9 -->

### Phase 4: Verification tail

- [x] T010 Run `cd src && go test ./...`; fix any failures. Re-run the help-dump checklist against a fresh dev build (command tree unchanged by R5/R6, so this is a confirmation) <!-- R10 -->

## Execution Order

- T001 precedes T002–T004 (dev build + enumeration are audit inputs).
- Phase 1 (audit) precedes Phase 2 (fixes are scoped by audit findings).
- T005 blocks T006 (test pins the T005 behavior).
- T007 is independent (README-only), parallelizable with T005/T006.
- Phase 2 precedes T008 (report cites the fixes) and T010 (tests cover the fixes).

## Acceptance

### Functional Completeness

- [x] A-001 R1: The report cites shll v0.0.23 (from `shll version`) and the audit was run against a dev build from repo HEAD; the 4 runtime-enumerated standards each have a report section
- [x] A-002 R2: The dev build passes every help-dump checklist item and `help_dump_test.go` passes
- [x] A-003 R3: The repo passes every readme-extraction checklist item, including a badge run in the README head
- [x] A-004 R4: All 10 principles are dispositioned (PASS or fixed/deferred gap) against observed behavior
- [x] A-005 R7: `conformance-report.md` exists in the change folder with the required per-standard/per-gap structure
- [x] A-006 R8: The report's skill section reads "deferred, not yet adopted" referencing `[agst]`, and no `shll skill` command exists in `src/cmd/shll/`
- [x] A-007 R9: Every deferred gap (if any) is a `fab/backlog.md` item with a fresh 4-char ID referenced from the report — **no new deferrals**; the only deferred standard (`skill`) is pre-existing `[agst]`

### Behavioral Correctness

- [x] A-008 R5: `shll bogus`, `shll list --bogus`, and `shll doctor extra-arg` exit 2; `shll standards nonexistent` and other operational failures still exit 1; `shll shell-init fish` still exits 2 (unchanged) — verified end-to-end on a fresh dev build at review
- [x] A-009 R6: The README head reads `# shll` → toolkit blockquote → badge run → intro prose; badge targets resolve against the shll repo — byte-consistent with the 6 sibling repos' badge run

### Scenario Coverage

- [x] A-010 R5: `main_test.go` exercises usage-error (→2), operational-failure (→1), and success (→0) paths
- [x] A-011 R10: `cd src && go test ./...` passes (re-run with `-count=1` at review); the help-dump checklist re-run confirms the (unchanged) command tree of 10 visible commands

### Edge Cases & Error Handling

- [x] A-012 R5: The exit-2 classification does not misclassify an operational error whose message coincidentally contains a usage prefix (classification is anchored to cobra's error path / flag-error hook, not a loose substring match) — prefix-anchored (`strings.HasPrefix`), pinned by the mid-message test case; repo grep found no operational error message starting with any classified prefix (`tools.go`'s `unknown target…` does not match `unknown command `)

### Code Quality

- [x] A-013 Pattern consistency: The exit-code fix reuses the existing `errExitCode` sentinel and `translateExit` seam; new constants are named (no magic strings); tests follow the table-driven, `bytes.Buffer`-driven style of the existing `*_test.go` files
- [x] A-014 No unnecessary duplication: Flag-error handling reuses cobra's `SetFlagErrorFunc` hook rather than re-parsing; the badge run reuses the sibling-repo form
- [x] A-015 Security (Constitution I): No new subprocess invocations are added; no change to `internal/proc` routing (the fixes are exit-code plumbing + a README edit)
- [x] A-016 Constitution (statelessness / wrap-don't-reinvent / minimal surface): No new state, no new top-level subcommand, no reimplemented sub-tool logic; the change is additive exit-code plumbing + docs

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- `src/cmd/shll/shell_init.go:36,40` — the `errExitCode{code: 2, …}` literals now duplicate the `usageExitCode` constant this change introduced in `main.go`; the magic `2` is redundant with the named constant and can be swapped for it
- `src/cmd/shll/shell_setup.go:104,111,261,324,326,342,465` — same: seven `errExitCode{code: 2, …}` usage-error literals expressible via `usageExitCode` (no code becomes unused or dead by this change; these are the only discovered redundancies — literal representations superseded by the new named constant)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Audit against a dev build from HEAD; standards text from installed v0.0.23 (drift-guard byte-matched); report cites v0.0.23 | Intake states it verbatim; drift guard `TestStandardsEmbedMatchesCanonical` pins repo↔embed equality | S:90 R:85 A:95 D:90 |
| 2 | Certain | Usage errors (unknown command/flag, bad arg count) must exit 2 per the principles standard's stated `0/1/2` convention; a small additive fix | Principles №4 states the convention verbatim; shll already uses exit 2 for shell-init usage errors — the gap is a real inconsistency | S:90 R:80 A:90 D:85 |
| 3 | Confident | README footer-heading absence is conformant — footer headings are pull-STOP markers, not required content; whole README is site-worthy, nothing maintainer-only leaks | Standard §2 defines footer headings only as slice-end markers; no `Contributing`/`Development`/`License` content exists to leak | S:70 R:85 A:80 D:75 |
| 4 | Confident | `version` lacking `--json` is not a №2 gap — it is human-paste by explicit design; `list --json`/`doctor --json` (carrying per-tool version) cover the programmatic surface | version.go's own help states "no JSON … pastes cleanly into bug reports"; the programmatic version surface exists in doctor's JSON | S:65 R:75 A:75 D:70 |
| 5 | Confident | README badge run IS required by the head-structure standard (not optional chrome) — resolving intake item (a) | Standard §1 lists badges in the exact head order and the checklist names them; all 6 sibling repos carry the identical run | S:70 R:85 A:80 D:75 |
| 6 | Confident | Exit-2 classification: flag errors via `SetFlagErrorFunc`; arg/command errors via cobra's stable error prefixes at `translateExit` | Cobra v1.10.2 has a clean hook only for flag errors; arg/command errors are untyped `fmt.Errorf` — prefix match at the single seam is the least-invasive root fix | S:65 R:70 A:75 D:65 |
| 7 | Confident | Report persisted as `conformance-report.md` in the change folder; `/git-pr` carries it into the PR body at ship | Intake Assumption 7; apply and ship are separate stages, so a change-folder artifact is the durable hand-off | S:60 R:80 A:80 D:70 |
| 8 | Confident | Expected outcome: no new backlog deferrals beyond `[agst]` (skill) — the two other gaps (exit 2, badges) are both additive and fixed here | Audit found only additive gaps; anything larger would defer per R9, but none surfaced | S:60 R:75 A:75 D:65 |

8 assumptions (2 certain, 6 confident, 0 tentative).
