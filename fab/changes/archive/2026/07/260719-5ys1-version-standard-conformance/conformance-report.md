## Conformance report — `version` standard, audited on repo HEAD

**Audit method.** The `version` standard (`docs/site/standards/version.md`, served by
`shll standards version`) was audited clause-by-clause against this repo's HEAD
(branch base `main` @ `6056d34`) by source inspection of `src/cmd/shll/main.go`,
`root.go`, `version.go`, `scripts/build.sh`, and `.github/workflows/release.yml`,
plus an **empirical probe of a stamped dev build**
(`go build -ldflags "-X main.version=v9.9.9-audit"`): exit `0`,
stdout=`shll version v9.9.9-audit` on line 1, stderr empty, 2 ms response.

**Scope note.** Only the `version` standard applies to shll as a producer. The
`update` standard names shll the consumer (shll self-updates via a direct
`brew upgrade`, and `shll update` delegates to each roster tool's own `update`),
and shll has no shell-init of its own (`shll shell-init` composes the tools') —
both are out of producer scope per the standards themselves.

---

### version — 8 PASS; 1 gap (fixed here)

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
| 9 | Keep (or add) a minimal test pinning the above — exit 0, version on line 1, matches the shape | **No test exercised root `--version`** (`version_test.go` covered the `shll version` subcommand table + `normalizeVersion`; `main_test.go` exit-code translation; `help_dump_test.go` set `root.Version` only to feed the JSON doc). **Fixed here** — `TestRootVersionFlag_VersionStandardConformance` in `src/cmd/shll/version_test.go` pins exit 0 (nil `Execute` error), exact first-line `shll version v1.2.3`, the shape via the repo's own published parse (`normalizeVersion` / `versionTokenRE` / `versionPrefixRE`), empty stderr, plus a `dev`-default subtest keeping unstamped builds parseable | **fixed here** |

**Net: behavior was already fully conformant; the sole gap was the missing
pinning test (the standard's *Verifying conformance* bullet 5), closed by a
test-only diff** — `main.go`/`root.go`/`version.go` untouched (constitution
Test Integrity: the test conforms to the spec being pinned, never the other
way around).

---

## Summary

| Standard | Result | Disposition |
|----------|--------|-------------|
| version | 8 PASS, 1 gap | clause 9 pinning test → **fixed here** (`src/cmd/shll/version_test.go`) |
| update | N/A | shll is the consumer, not a producer — out of scope per the standard |
| shell-init | N/A | shll has no shell-init of its own (composer) — out of scope per the standard |

**Deferrals created:** none.

**Verification:** `cd src && go test ./cmd/shll/` passes (new test green, no
regressions); `go vet ./...` clean.
