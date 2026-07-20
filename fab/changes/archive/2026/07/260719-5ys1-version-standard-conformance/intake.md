# Intake: Version Standard Conformance

**Change**: 260719-5ys1-version-standard-conformance
**Created**: 2026-07-20

## Origin

One-shot `/fab-new` invocation:

> Bring this repo into conformance with the shll toolkit 'version' standard (docs/site/standards/version.md in the shll repo, or https://shll.ai/standards). Note: shll is the consumer/composer for the 'update' and 'shell-init' standards, not a producer, so only 'version' applies here (shll self-updates via a direct brew upgrade and has no shell-init of its own). Audit the 'shll --version' output against every MUST/SHOULD in the version standard, fix any gaps found, and add/update tests pinning the fixed behavior. If the audit finds the repo is already fully conformant with no code changes needed, skip /git-pr entirely — do not open an empty PR.

The audit was performed **during intake** (source inspection of `src/cmd/shll/main.go`, `root.go`, `version.go`, `scripts/build.sh`, `.github/workflows/release.yml`, plus an empirical probe of a stamped build). Its findings are recorded verbatim below so the apply-entry agent inherits them without re-deriving.

## Why

1. **Pain point**: shll publishes the toolkit's `version` standard (`docs/site/standards/version.md`) and the standard explicitly scopes itself to **all seven binaries — including `shll` itself** ("shll holds itself to the shape it enforces"). The standard's *Verifying conformance* section requires: "Keep (or add) a minimal test pinning the above — exit 0, version on line 1, matches the shape — so the contract stays protected." No test in the repo exercises the root `shll --version` flag; existing tests cover the `shll version` subcommand table and `normalizeVersion` parsing, but nothing pins the producer-side contract of `shll --version` itself.
2. **Consequence if unfixed**: the conformance is unprotected. A future change (custom `SetVersionTemplate`, a banner in the version path, removing `rootCmd.Version = version`, a `--version` that shells out) would silently break the exact contract shll enforces on the six roster tools — the standard's failure mode ("shll doctor flags an otherwise-healthy tool as unreportable") would then apply to shll itself, and the repo hosting the standard would be its first violator.
3. **Approach**: behavior is already conformant (audit table below), so the fix is test-only — add a conformance-pinning test for the root `--version` flag. No implementation change; the constitution's Test Integrity rule (tests conform to the spec) is satisfied because the standard is the spec being pinned.

## What Changes

### Audit result: `shll --version` vs. every MUST/SHOULD in the version standard

Audited on this worktree's HEAD (branch base `main` @ 6056d34), empirically probed with a `-ldflags "-X main.version=v9.9.9-audit"` build:

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
| 9 | Keep (or add) a minimal test pinning the above — exit 0, version on line 1, matches the shape | **No test exercises root `--version`.** `version_test.go` covers the `shll version` subcommand table + `normalizeVersion`; `main_test.go` covers exit-code translation; `help_dump_test.go` sets `root.Version` only to feed the JSON doc | **GAP — test missing** |

**Net: behavior fully conformant; one gap, and it is test-only (clause 9).** Because a test file is added, this change produces a real diff → `/git-pr` proceeds normally at ship (the "skip if no code changes" condition in the Origin does not trigger).

### 1. Add a root `--version` conformance test (the only code change)

Add `TestRootVersionFlag_VersionStandardConformance` to `src/cmd/shll/version_test.go` (alongside the existing version-surface tests). Shape, following the existing table-driven/buffer style in that file:

- Build the command via `newRootCmd()`, set `root.Version = "v1.2.3"` (mirroring main.go's `rootCmd.Version = version` wiring — same seam `help_dump_test.go` already uses), capture stdout via `root.SetOut(&buf)` (and `SetErr` to assert nothing lands on stderr), `root.SetArgs([]string{"--version"})`.
- Assert, mapping one assertion per standard clause:
  1. `root.Execute()` returns nil (→ exit 0 through `translateExit`).
  2. Output's **first non-empty line** is exactly `shll version v1.2.3` (the RECOMMENDED canonical shape, and pins "no banner above the version").
  3. The first line satisfies the repo's own published parse: `normalizeVersion(buf.String()) == "v1.2.3"` — reusing the in-repo `versionTokenRE`/`versionPrefixRE` machinery so the test pins "shll parses its own output" rather than a hand-rolled regex.
  4. Stderr buffer is empty (stdout-is-data).
- Also pin the dev default: a second case (or subtest) with `root.Version = "dev"` asserting the first line `shll version dev` still satisfies `versionPrefixRE` extraction (`normalizeVersion` → `dev`) — so unstamped builds stay parseable, not just release builds.

No timeout/network assertion in-process (the path has no I/O to fake; clauses 3–4 are structural) — the test's comment records that rationale.

### 2. Conformance report artifact (apply stage, per repo convention)

Per the conformance-report-in-PR-body convention (`docs/memory/cli/standards-conformance.md`), apply SHALL write `conformance-report.md` into this change folder carrying the audit table above (one section: `version` standard — PASS × 8, fixed-here × 1 with the test), for `/git-pr` to carry verbatim into the PR body at ship.

### Explicitly out of scope

- `update` standard: shll is the consumer (self-updates via direct `brew upgrade`), not a producer — per Origin.
- `shell-init` standard: shll has no shell-init of its own (it composes the tools') — per Origin.
- Any change to `shll version` (the subcommand table), `normalizeVersion`, or the probe machinery — audited surfaces, all conformant, all already test-covered.
- No `SetVersionTemplate` / output reshaping — cobra's default already IS the recommended canonical shape.

## Affected Memory

- `cli/version`: (modify) note that the root `--version` flag (producer side) is now pinned by a conformance test, distinct from the subcommand table (consumer side).
- `cli/standards-conformance`: (modify) upgrade the `version` row from "conformant by construction" to "conformant, behaviorally audited on HEAD and pinned by test"; record this change as shll's own `[std2]`-pattern conformance pass for `version`.

## Impact

- `src/cmd/shll/version_test.go` — one new test function (test-only diff; `main.go`/`root.go`/`version.go` untouched).
- `fab/changes/260719-5ys1-version-standard-conformance/conformance-report.md` — new apply-stage artifact for the PR body.
- No CLI surface, help output, README, or docs/site change → no other standard is triggered by the constitution's Toolkit Standards clause.

## Open Questions

- None — the audit resolved the conditional in the Origin (a test is added, so `/git-pr` proceeds).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Scope is the `version` standard only; `update`/`shell-init` are out of scope (shll is consumer/composer there) | Stated explicitly in the user's invocation | S:95 R:90 A:95 D:95 |
| 2 | Certain | Audit verdict: `shll --version` behavior is fully conformant; the sole gap is the missing pinning test (standard's "Verifying conformance" bullet 5) | Verified by source inspection AND an empirical stamped-build probe during intake (exit 0, `shll version v9.9.9-audit` on stdout line 1, empty stderr, 2 ms) | S:90 R:85 A:95 D:90 |
| 3 | Certain | The change is therefore test-only — no implementation code is touched | Follows from #2; constitution Test Integrity: tests conform to the spec (the standard), never implementation bent to tests | S:85 R:85 A:90 D:90 |
| 4 | Confident | New test lives in `src/cmd/shll/version_test.go` and asserts via `normalizeVersion` + exact first-line equality, with a `dev`-default subtest | Repo convention (tests alongside the surface they pin; `help_dump_test.go` precedent for setting `root.Version`); reusing the published parse machinery beats a hand-rolled regex | S:70 R:90 A:85 D:75 |
| 5 | Confident | Apply writes `conformance-report.md` into the change folder for the PR body | Documented repo convention for standards-conformance changes (`cli/standards-conformance.md` § deliverable shape) | S:65 R:90 A:85 D:80 |
| 6 | Certain | `/git-pr` proceeds at ship (the skip-empty-PR condition does not trigger — a test file is added) | The Origin's conditional, resolved by audit finding #2 | S:90 R:90 A:95 D:95 |

6 assumptions (4 certain, 2 confident, 0 tentative, 0 unresolved).
