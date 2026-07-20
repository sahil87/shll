# Plan: Version Standard Conformance

**Change**: 260719-5ys1-version-standard-conformance
**Intake**: `intake.md`

## Requirements

### CLI: Root `--version` conformance pinning

The intake's audit (performed during intake on this worktree's HEAD, branch base `main` @ 6056d34, empirically probed with a `-ldflags "-X main.version=v9.9.9-audit"` build) found `shll --version` behavior fully conformant with the toolkit `version` standard (`docs/site/standards/version.md`) — 8 of 9 clauses PASS. The sole gap is clause 9 (the standard's *Verifying conformance* bullet 5): "Keep (or add) a minimal test pinning the above — exit 0, version on line 1, matches the shape — so the contract stays protected." No test in the repo exercises the root `--version` flag; `version_test.go` covers the `shll version` subcommand table + `normalizeVersion`, `main_test.go` covers exit-code translation, and `help_dump_test.go` sets `root.Version` only to feed the JSON doc. The change is therefore **test-only** — no implementation code (`main.go`/`root.go`/`version.go`) is touched.

Audit table (carried from the intake, verbatim — the apply-stage inheritance record):

| # | Standard clause (MUST/SHOULD) | Mechanism in repo | Verdict |
|---|-------------------------------|-------------------|---------|
| 1 | MUST support `--version` and exit `0` | `rootCmd.Version = version` (main.go) enables cobra's built-in root `--version` flag; cobra prints and returns nil → exit 0. Probe: `exit=0` | PASS |
| 2 | Writes the version to **stdout** (principle №2: stdout is data) | Cobra's version template renders to `cmd.OutOrStdout()`. Probe: stdout=`shll version v9.9.9-audit`, stderr empty | PASS |
| 3 | MUST respond within 2 seconds | Version path is a ldflags-injected package variable — no subprocess, no I/O. Probe: 2 ms | PASS |
| 4 | MUST do no network I/O on the version path | Same — purely local read of `main.version` | PASS |
| 5 | Version token MUST be on the first non-empty line (bare token `versionTokenRE` OR `<word> version <rest>` `versionPrefixRE`) | Output is exactly one line: `shll version <ver>` — matches the prefix shape always; release builds also carry a bare `vX.Y.Z` token | PASS |
| 6 | No banner/copyright/update-check line above the version | Cobra's default version template emits the single version line only | PASS |
| 7 | RECOMMENDED canonical shape `<tool> version vX.Y.Z` | Release builds inject the git tag verbatim (`steps.version.outputs.tag`, `v*`-prefixed per release convention) → `shll version v1.2.3`, cobra's stable default form. Dev builds emit `shll version dev` / `shll version v0.0.x-N-g<sha>` (`git describe`) — still parseable via clause 5 | PASS |
| 8 | Binary name on PATH MUST equal the tool name | Formula, repo, and binary are all `shll`; build outputs `bin/shll`, release tarballs contain `shll` | PASS |
| 9 | Keep (or add) a minimal test pinning the above — exit 0, version on line 1, matches the shape | **No test exercises root `--version`.** | **GAP — test missing** |

#### R1: Root `--version` conformance test

A test `TestRootVersionFlag_VersionStandardConformance` SHALL exist in `src/cmd/shll/version_test.go` (alongside the existing version-surface tests), pinning the producer-side contract of `shll --version` per the standard's *Verifying conformance* section. The test SHALL, following the existing buffer style in that file:

- Build the command via `newRootCmd()`, set `root.Version = "v1.2.3"` (mirroring main.go's `rootCmd.Version = version` wiring — the same seam `help_dump_test.go` already uses), capture stdout via `root.SetOut(&buf)` (and `SetErr` to assert nothing lands on stderr), `root.SetArgs([]string{"--version"})`.
- Assert, mapping one assertion per standard clause:
  1. `root.Execute()` returns nil (→ exit 0 through `translateExit`).
  2. Output's **first non-empty line** is exactly `shll version v1.2.3` (the RECOMMENDED canonical shape, and pins "no banner above the version").
  3. The first line satisfies the repo's own published parse: `normalizeVersion(buf.String()) == "v1.2.3"` — reusing the in-repo `versionTokenRE`/`versionPrefixRE` machinery so the test pins "shll parses its own output" rather than a hand-rolled regex.
  4. Stderr buffer is empty (stdout-is-data).
- Also pin the dev default: a second case (or subtest) with `root.Version = "dev"` asserting the first line `shll version dev` still satisfies `versionPrefixRE` extraction (`normalizeVersion` → `dev`) — so unstamped builds stay parseable, not just release builds.
- No timeout/network assertion in-process (the path has no I/O to fake; clauses 3–4 are structural) — the test's comment SHALL record that rationale.

- **GIVEN** a root command built via `newRootCmd()` with `root.Version = "v1.2.3"` and captured out/err buffers
- **WHEN** `Execute()` runs with args `["--version"]`
- **THEN** it returns nil, stdout's first non-empty line is exactly `shll version v1.2.3`, `normalizeVersion` of the output yields `v1.2.3`, and the stderr buffer is empty

- **GIVEN** the same setup with `root.Version = "dev"` (the unstamped-build default)
- **WHEN** `Execute()` runs with args `["--version"]`
- **THEN** the first line is `shll version dev` and `normalizeVersion` of the output yields `dev` (the `versionPrefixRE` prefix-strip path)

#### R2: Conformance report artifact

Per the conformance-report-in-PR-body convention (`docs/memory/cli/standards-conformance.md`), apply SHALL write `conformance-report.md` into this change folder carrying the audit table above (one section: `version` standard — PASS × 8, fixed-here × 1 with the test), citing the audited HEAD, for `/git-pr` to carry verbatim into the PR body at ship (where the fixing commit sha is stamped in).

- **GIVEN** the apply stage of this change
- **WHEN** apply completes
- **THEN** `fab/changes/260719-5ys1-version-standard-conformance/conformance-report.md` exists, carries the 9-row audit table with 8 PASS verdicts and clause 9 marked fixed-here-with-the-test, and cites the audited base (`main` @ 6056d34, probe build `v9.9.9-audit`)

### Non-Goals

- `update` standard: shll is the consumer (self-updates via direct `brew upgrade`), not a producer — per the intake Origin.
- `shell-init` standard: shll has no shell-init of its own (it composes the tools') — per the intake Origin.
- Any change to `shll version` (the subcommand table), `normalizeVersion`, or the probe machinery — audited surfaces, all conformant, all already test-covered.
- No `SetVersionTemplate` / output reshaping — cobra's default already IS the recommended canonical shape.
- No CLI surface, help output, README, or docs/site change → no other standard is triggered by the constitution's Toolkit Standards clause.

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add `TestRootVersionFlag_VersionStandardConformance` to `src/cmd/shll/version_test.go`: `newRootCmd()` + `root.Version = "v1.2.3"`, `SetOut`/`SetErr` buffers, `SetArgs(["--version"])`; assert Execute() nil, first non-empty line exactly `shll version v1.2.3`, `normalizeVersion(out) == "v1.2.3"`, stderr empty; plus a `dev` subtest (`root.Version = "dev"` → first line `shll version dev`, `normalizeVersion` → `dev`); comment records why clauses 3–4 (timeout/network) carry no in-process assertion <!-- R1 -->
- [x] T002 [P] Write `fab/changes/260719-5ys1-version-standard-conformance/conformance-report.md`: one `version`-standard section carrying the intake's 9-row audit table (PASS × 8, clause 9 fixed here by T001's test), citing the audited base `main` @ 6056d34 and the `v9.9.9-audit` probe, following the shape of `fab/changes/260717-3sss-toolkit-standards-conformance/conformance-report.md` <!-- R2 -->

### Phase 3: Integration & Edge Cases

- [x] T003 Run scoped tests (`cd src && go test ./cmd/shll/ -run 'TestRootVersionFlag|TestNormalizeVersion|TestVersion'`) then the full package (`go test ./cmd/shll/`) to confirm the new test passes and no regressions <!-- R1 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `TestRootVersionFlag_VersionStandardConformance` exists in `src/cmd/shll/version_test.go` and passes, asserting exit-0 (nil Execute error), exact first-line `shll version v1.2.3`, `normalizeVersion` round-trip, and empty stderr
- [x] A-002 R2: `conformance-report.md` exists in the change folder with the 9-row audit table (8 PASS, clause 9 fixed-here) citing the audited base

### Behavioral Correctness

- [x] A-003 R1: The test asserts via the repo's own `normalizeVersion` (`versionTokenRE`/`versionPrefixRE`) rather than a hand-rolled regex — shll provably parses its own `--version` output

### Scenario Coverage

- [x] A-004 R1: The dev-default subtest (`root.Version = "dev"`) passes — unstamped builds stay parseable via the `versionPrefixRE` prefix-strip path
- [x] A-005 R1: Full `go test ./cmd/shll/` run is green — no regressions in the existing version-surface, help-dump, or exit-code tests

### Edge Cases & Error Handling

- [x] A-006 R1: A comment in the test records why clauses 3–4 (2-second response, no network I/O) carry no in-process assertion (the path has no I/O to fake — structural clauses)

### Code Quality

- [x] A-007 Pattern consistency: New test follows the existing `version_test.go` buffer/`t.Fatalf("got %q, want %q")` style and the `help_dump_test.go` `root.Version` seam
- [x] A-008 No unnecessary duplication: Reuses `newRootCmd()`, `normalizeVersion`, and cobra's built-in version flag — no new helpers, no fake runner (the root `--version` path invokes no subprocess)
- [x] A-009 Test Integrity (constitution): Tests conform to the spec (the published `version` standard) — no implementation code bent to accommodate the test (`main.go`/`root.go`/`version.go` untouched)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds a conformance-pinning test only; it makes no existing code, function, branch, or config redundant (`main.go`/`root.go`/`version.go` untouched; the new test reuses `newRootCmd()`/`normalizeVersion`/cobra's built-in version flag).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Cobra's default root version template emits exactly `shll version <Version>\n` to `OutOrStdout()` — pinned by exact first-line equality | Verified empirically during the intake probe (stdout=`shll version v9.9.9-audit`, stderr empty) and by cobra's default `VersionTemplate` rendering to `cmd.OutOrStdout()` | S:90 R:90 A:95 D:90 |
| 2 | Certain | No fake runner (`installFakeRunner`) is needed for the new test — the root `--version` path reads only the package `version` var and invokes no subprocess | Source-verified: cobra's version handling short-circuits before any RunE; `main.go` wires a plain package var | S:85 R:95 A:95 D:90 |
| 3 | Confident | The stamped and dev cases run as two subtests (`t.Run`) inside the single named test function | Intake says "a second case (or subtest)" — subtests keep the standard-conformance pin one discoverable function, matching the intake's single named-test design | S:70 R:95 A:90 D:80 |

3 assumptions (2 certain, 1 confident, 0 tentative).
