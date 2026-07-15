# Plan: Copy-paste installer one-liner — `scripts/install.sh` served at shll.ai/install

**Change**: 260715-m1zt-install-bootstrap-one-liner
**Intake**: `intake.md`

## Requirements

### Bootstrap script: `scripts/install.sh`

#### R1: Canonical thin bootstrap at the pinned path
The repo SHALL ship a POSIX-sh bootstrap script at `scripts/install.sh` whose entire body is wrapped in `main() { … }; main "$@"` (truncation guard) and which requires nothing beyond a POSIX `sh`. It SHALL be fully non-interactive (stdin is the pipe; nothing prompts) and a few dozen auditable lines.

- **GIVEN** a user runs `curl -fsSL https://shll.ai/install | sh`
- **WHEN** the script is piped to `sh`
- **THEN** it executes `main "$@"` only after the full body is defined, so a truncated download cannot execute a half-script
- **AND** no step reads from a TTY or prompts

#### R2: Homebrew required, never auto-installed
When `brew` is not on `PATH`, the script SHALL print a pointer to https://brew.sh on stderr and `exit 1`. It SHALL NOT attempt to install Homebrew.

- **GIVEN** `brew` is absent from `PATH`
- **WHEN** the script runs
- **THEN** it writes a "Homebrew is required" message and the https://brew.sh pointer to stderr
- **AND** exits with status 1 without installing anything

#### R3: Idempotent shll short-circuit
When `shll` is already on `PATH`, the script SHALL skip the bootstrap install entirely and proceed straight to the exec hand-off.

- **GIVEN** `shll` is already installed and on `PATH`
- **WHEN** the script runs (brew present)
- **THEN** it performs no `brew trust`/`brew install` for shll
- **AND** proceeds directly to `exec shll install "$@"`

#### R4: Trust-then-install with capability-probed trust tolerance
When `shll` is not on `PATH`, the script SHALL install `sahil87/tap/shll` via Homebrew, running `brew trust --formula sahil87/tap/shll` first **only when** the trust subcommand is available. Trust availability SHALL be detected by a capability probe — `brew trust --help` exit 0 — mirroring `brewTrustAvailable` (`src/cmd/shll/brew.go:67`), never a version-floor check. On pre-6.0 Homebrew (no `brew trust`, no trust requirement) the trust step SHALL be skipped silently; on 6.0+ a trust or install failure SHALL surface with brew's own error output (under `set -e`, no swallowing).

- **GIVEN** `shll` is not on `PATH` and `brew trust --help` exits 0
- **WHEN** the script runs
- **THEN** it runs `brew trust --formula sahil87/tap/shll` then `brew install sahil87/tap/shll`
- **AND GIVEN** `brew trust --help` exits non-zero (pre-6.0), **THEN** the trust step is skipped and only `brew install sahil87/tap/shll` runs
- **AND** any failing `brew trust`/`brew install` on 6.0+ aborts the script with brew's own error output (`set -e`)

#### R5: Exec hand-off delegates all intelligence to `shll install`
The final action SHALL be `exec shll install "$@"`, passing every argument through unchanged. The script SHALL NOT duplicate roster knowledge, subset filtering, per-formula trust for the other tools, or graceful skips — all of that stays in `shll install` (Constitution III).

- **GIVEN** the script is invoked as `sh -s -- hop wt`
- **WHEN** it reaches the hand-off
- **THEN** it runs `exec shll install hop wt` (args forwarded verbatim)
- **AND** the script itself contains no roster/subset logic

### Dev-script rename: `scripts/install-local.sh`

#### R6: Preserve the local dev install under a new path
The existing dev install script (bash; `./scripts/build.sh` then copy to `~/.local/bin/shll`) SHALL be moved to `scripts/install-local.sh` with its content unchanged, and the `justfile` `install` recipe SHALL invoke `./scripts/install-local.sh`. The recipe name `install` and its comment SHALL remain, so `just install` UX is unchanged.

- **GIVEN** the bootstrap must own `scripts/install.sh`
- **WHEN** the rename is applied
- **THEN** `scripts/install-local.sh` holds the former dev-script content verbatim
- **AND** `just install` runs `./scripts/install-local.sh` and still builds + copies the binary to `~/.local/bin/shll`

### Docs: README + site install page

#### R7: README leads with the one-liner
`README.md` SHALL gain a new `## Install` section immediately after the intro line (after the one-line toolkit description, before `## Why shll?`), leading with the install-everything form, then the subset form, followed by a short description (requires Homebrew with a https://brew.sh pointer; bootstraps `shll` itself recording Homebrew 6.0 tap trust, then hands off to `shll install`; idempotent). The former `## Install` section (manual brew bootstrap, `all` meta-formula note, `### From source`) SHALL be absorbed into the new section as manual/alternative paths and the old section removed. Quick start's first two bootstrap lines SHALL be replaced by the curl one-liner (then `shll shell-setup`, then `exec $SHELL`), trimming the bootstrap-explanation paragraph while keeping the tap-trust deep-dive in Troubleshooting.

- **GIVEN** a reader opens `README.md`
- **WHEN** they reach the top of the document
- **THEN** the first install instruction is `curl -fsSL https://shll.ai/install | sh`, with `curl -fsSL https://shll.ai/install | sh -s -- hop wt` shown as the subset form
- **AND** the manual brew bootstrap, `all` meta-formula, and from-source paths remain reachable in the same `## Install` section
- **AND** there is exactly one `## Install` section

#### R8: Site install page leads with the one-liner (light touch)
`docs/site/install.md` SHALL present the one-liner (both forms) at the top of the "Bootstrap via Homebrew" section as the recommended path, keeping the manual trust-then-install flow as the explicit alternative. No structural reorganization beyond this addition.

- **GIVEN** a reader opens `docs/site/install.md`
- **WHEN** they reach the "Bootstrap via Homebrew" section
- **THEN** the curl one-liner (everything + subset forms) leads the section as recommended
- **AND** the existing manual `brew trust … && brew install …` flow remains as the explicit alternative
- **AND** the page structure is otherwise unchanged

### Non-Goals

- No new `shll` subcommand and no Go code changes (`src/` untouched) — Constitution VII untouched.
- No CI wiring for shellcheck — the shellcheck-clean requirement is a local apply/review gate (shellcheck if available, plus `sh -n`).
- No changes to the shll.ai repo (PR #84 is in flight there).
- No auto-install of Homebrew.

### Design Decisions

1. **Thin bootstrap, not a fat installer**: the script solves only the bootstrap circularity (shll cannot trust/install its own formula before it exists), then `exec shll install "$@"`. — *Why*: keeps all roster/subset/trust intelligence versioned and tested in Go (Constitution III). — *Rejected*: (a) pointing users at the `all` meta-formula — still demands the trust ceremony, no subset form; (b) a fat script re-implementing roster logic — drifts from `shll install`.
2. **Capability probe for trust, not a version check**: `brew trust --help` exit 0 gates the trust step. — *Why*: mirrors the codebase contract `brewTrustAvailable` ("the probe is the contract"); pre-6.0 brews have no `brew trust` and need no trust. — *Rejected*: parsing `brew --version` for a 6.0 floor — brittle and off-contract.
3. **Rename the colliding dev script**: `scripts/install.sh` → `scripts/install-local.sh`. — *Why*: the bootstrap path is pinned by shll.ai#84's raw-fetch URL, so the dev script must move; only the rename target was open. — *Rejected*: keeping the dev script and putting the bootstrap elsewhere — breaks the pinned URL contract.

## Tasks

### Phase 1: Path rename (must precede writing the new bootstrap)

- [x] T001 `git mv scripts/install.sh scripts/install-local.sh` (content unchanged) <!-- R6 -->
- [x] T002 Update `justfile` line 11 recipe body `./scripts/install.sh` → `./scripts/install-local.sh` (recipe name/comment unchanged) <!-- R6 -->

### Phase 2: Bootstrap script

- [x] T003 Write the new POSIX-sh bootstrap at `scripts/install.sh`: `#!/bin/sh`, `set -eu`, header comment with the two curl usage forms, `main() { … }; main "$@"` wrapper; brew-required guard (stderr pointer to https://brew.sh + `exit 1`); shll-present short-circuit; trust-probe (`brew trust --help` exit 0) → `brew trust --formula sahil87/tap/shll`; `brew install sahil87/tap/shll`; one progress line before install; final `exec shll install "$@"`. `chmod +x`. <!-- R1 -->
- [x] T004 Ensure R2 (brew-required guard), R3 (shll short-circuit), R4 (trust probe + tolerance), R5 (exec hand-off, zero roster logic) are all realized in the T003 script — verify each contract clause is present and correct <!-- R2 -->

### Phase 3: Documentation

- [x] T005 [P] `README.md`: add new `## Install` section after the intro line leading with the everything + subset one-liners and short description; absorb the old `## Install` body (manual brew bootstrap, `all` meta-formula, `### From source`) into it and remove the old section; replace Quick start's first two bootstrap lines with the curl one-liner and trim the bootstrap-explanation paragraph <!-- R7 -->
- [x] T006 [P] `docs/site/install.md`: add the one-liner (both forms) at the top of the "Bootstrap via Homebrew" section as the recommended path, keeping the manual trust-then-install flow as the explicit alternative (light touch, no restructure) <!-- R8 -->

### Phase 4: Verification

- [x] T007 Run the script test gate: `sh -n scripts/install.sh` (syntax) and shellcheck if available (unavailable in this env → `sh -n` is the gate); confirm no Go changes needed <!-- R1 -->

## Execution Order

- T001 blocks T003 (the new bootstrap must not clobber the un-renamed dev script; move it first).
- T001 blocks T002 (rename before updating the reference is cleaner, though order between them is not strictly load-bearing).
- T005 and T006 are independent docs edits, parallelizable, and independent of the script tasks.
- T007 runs last (needs the final `scripts/install.sh`).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `scripts/install.sh` is POSIX sh, `main() { … }; main "$@"`-wrapped, non-interactive, a few dozen lines
- [x] A-002 R2: brew-absent path prints the https://brew.sh pointer to stderr and exits 1, with no Homebrew auto-install
- [x] A-003 R3: when `shll` is already on `PATH`, no `brew trust`/`brew install` for shll runs; script goes straight to exec
- [x] A-004 R4: trust step is gated by `brew trust --help` exit 0 (capability probe, not a version check) and runs `brew trust --formula sahil87/tap/shll` before `brew install sahil87/tap/shll`
- [x] A-005 R5: script ends with `exec shll install "$@"` (args forwarded verbatim) and contains no roster/subset/per-tool-trust logic
- [x] A-006 R6: `scripts/install-local.sh` holds the former dev-script content unchanged and `justfile` `install` invokes `./scripts/install-local.sh`
- [x] A-007 R7: `README.md` has exactly one `## Install` section, placed after the intro line, leading with the curl one-liner (everything + subset), absorbing the former manual/`all`/from-source paths; Quick start leads with the one-liner
- [x] A-008 R8: `docs/site/install.md` "Bootstrap via Homebrew" leads with the one-liner (both forms) as recommended, manual flow retained as the alternative

### Behavioral Correctness

- [x] A-009 R4: on pre-6.0 Homebrew (`brew trust --help` non-zero) the trust step is skipped silently; on 6.0+ a trust/install failure aborts the script with brew's own output (`set -e`, no swallowing)
- [x] A-010 R6: `just install` still builds and copies the binary to `~/.local/bin/shll` (dev flow behavior unchanged)

### Scenario Coverage

- [x] A-011 R5: `sh -s -- hop wt` forwards to `exec shll install hop wt` (arg pass-through verified by inspection)

### Edge Cases & Error Handling

- [x] A-012 R1: a truncated download cannot execute — `main "$@"` is the last line and the only invocation, so a partial body defines nothing runnable

### Code Quality

- [x] A-013 Pattern consistency: the bootstrap follows the project's thin-wrapper posture and shell-script conventions (scripts delegate; logic stays in the tool) — Constitution VI
- [x] A-014 No unnecessary duplication: the script duplicates none of `shll install`'s roster/subset/trust intelligence (Constitution III)
- [x] A-015 Script gate: `sh -n scripts/install.sh` passes (and shellcheck passes when available)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- `README.md:72` ("For the deeper install guide — … see docs/site/install.md") — near-duplicate of the pointer at README.md:43 inside the new `## Install` section, 30 lines apart after the restructure; Quick start already links to [Install](#install), so one of the two can go.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Dev script renamed to `scripts/install-local.sh` (the rename target chosen in intake assumption 5) | Bootstrap path is forced by the raw-fetch URL contract; only the rename target was open (low-stakes) | S:35 R:75 A:60 D:55 |
| 2 | Confident | README `## Install` placed after the intro line (before `## Why shll?`), absorbing the former manual/`all`/from-source body; Quick start's first two lines become the one-liner | Placement + absorption discussed in intake; exact shape is agent judgment | S:70 R:90 A:70 D:60 |
| 3 | Confident | `docs/site/install.md` gets a light one-liner-first touch in "Bootstrap via Homebrew" only | Page is pulled by shll.ai and not touched by #84; omitting the one-liner would leave the deep guide leading with the ceremony it replaces | S:35 R:85 A:70 D:55 |
| 4 | Confident | Script emits one progress line before the bootstrap install; brew and `shll install` stream their own output | Low-stakes presentation, consistent with the thin-wrapper posture | S:45 R:90 A:70 D:60 |
| 5 | Certain | Trust availability is the `brew trust --help` exit-0 capability probe, never a version floor | Codebase contract — mirrors `brewTrustAvailable` (`src/cmd/shll/brew.go:67`) | S:60 R:85 A:95 D:85 |
| 6 | Certain | Test gate is `sh -n` (shellcheck unavailable in this environment); no CI shellcheck wiring | Intake phrases shellcheck conditionally ("if available"); shellcheck is absent here, so `sh -n` is the enforceable gate | S:50 R:90 A:70 D:60 |

6 assumptions (2 certain, 4 confident, 0 tentative).
