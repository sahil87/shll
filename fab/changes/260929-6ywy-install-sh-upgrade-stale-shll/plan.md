# Plan: install.sh upgrades an already-installed shll before handing off

**Change**: 260929-6ywy-install-sh-upgrade-stale-shll
**Intake**: `intake.md`

## Requirements

### Bootstrap: shll handoff

#### R1: A brew-managed shll that is already installed is upgraded before `shll install`
When `command -v shll` succeeds AND `"$BREW" list --versions sahil87/tap/shll` exits 0, `scripts/install.sh` SHALL print one progress line to stdout, run the capability-probed trust step, then run `"$BREW" upgrade sahil87/tap/shll` — all before `shll install "$@"`.

- **GIVEN** brew is present and a brew-managed shll (e.g. v0.1.33) is on PATH
- **WHEN** the script reaches the shll handoff phase
- **THEN** it runs `"$BREW" trust --formula sahil87/tap/shll` (when `brew trust --help` succeeds), then `"$BREW" upgrade sahil87/tap/shll`, then `shll install "$@"`, then `exec shll update <tools>`

#### R2: The trust step is shared, capability-probed, and runs before both install and upgrade
The `brew trust --help` probe + `brew trust --formula sahil87/tap/shll` SHALL live in one helper function used by both the fresh-install and the upgrade branch. Pre-6.0 brews (probe fails) skip trust silently.

- **GIVEN** a brew whose `trust --help` exits non-zero
- **WHEN** either branch runs
- **THEN** no `brew trust` call is made and the install/upgrade proceeds

#### R3: Non-brew shll and fresh installs are unaffected
A shll on PATH that brew does not manage SHALL skip the upgrade silently (no line, no brew call beyond the `list` probe). When shll is missing, the existing trust + `brew install` path SHALL run unchanged, with no upgrade.

- **GIVEN** shll on PATH is a dev build (`~/.local/bin/shll`) and `brew list --versions sahil87/tap/shll` fails
- **WHEN** the handoff runs
- **THEN** no trust/upgrade/install brew call is made and `shll install "$@"` runs
- **GIVEN** shll is not on PATH
- **WHEN** the handoff runs
- **THEN** trust + `brew install sahil87/tap/shll` run and no `brew upgrade` runs

#### R4: A failing upgrade aborts the bootstrap
Under `set -eu`, a non-zero `brew upgrade` (or trust) SHALL abort the script with brew's own error output; `shll install` SHALL NOT run.

- **GIVEN** `brew upgrade sahil87/tap/shll` exits 1
- **WHEN** the upgrade branch runs
- **THEN** the script exits non-zero and neither `shll install` nor `shll update` runs

#### R5: Script comments and memory describe the upgrade step
The header comment and handoff comment in `scripts/install.sh` SHALL describe the upgrade-before-handoff step and why. Memory (`ci/install-bootstrap`, `cli/install`) is updated at hydrate.

- **GIVEN** a reader of the script header
- **WHEN** they read the hand-off description
- **THEN** it states an already-installed brew-managed shll is upgraded first so a stale shll never drives the install

### Non-Goals

- hexokit-site's stale-shll epilogue guard — removed by T2(b) after this is released
- An explicit `brew update` — brew's auto-update inside `brew upgrade` refreshes the tap
- Upgrading a non-brew shll — a dev build is the developer's own

### Design Decisions

#### Upgrade shll in the bootstrap, gated on brew management
**Decision**: When shll is already on PATH and brew manages `sahil87/tap/shll`, the script trust-probes and runs `brew upgrade sahil87/tap/shll` before `shll install "$@"`.
**Why**: A stale shll parses the bootstrap's args before anything can upgrade it (shll ≤ v0.1.33 rejects `hexokit`), and the script already owns installing shll itself. `shll update` also upgrades shll, but it runs after `shll install` — too late.
**Rejected**: (a) Upgrading inside `shll install` — the running binary is the stale one and cannot fix its own arg parsing. (b) Unconditional `brew upgrade` — errors under `set -e` on a non-brew dev shll. (c) Keeping site-side guards per incompatible change — a workaround per rename, forever.
*Introduced by*: 260929-6ywy-install-sh-upgrade-stale-shll

## Tasks

### Phase 2: Core Implementation

- [x] T001 Extract the capability-probed trust step into a `trust_shll` helper in `scripts/install.sh` (uses `$BREW`), and call it from the fresh-install branch <!-- R2 -->
- [x] T002 Add the `elif "$BREW" list --versions sahil87/tap/shll >/dev/null 2>&1` branch in `scripts/install.sh` main's shll handoff: progress line, `trust_shll`, `"$BREW" upgrade sahil87/tap/shll` <!-- R1 -->
- [x] T003 Update the header comment block and the handoff comment in `scripts/install.sh` <!-- R5 -->

### Phase 3: Integration & Edge Cases

- [x] T004 Verify: `sh -n` + `dash -n` + shellcheck (if available); run the script's handoff in isolation against stub `brew`/`shll` on a scratch PATH for the four cases (fresh, brew-managed present, non-brew present, upgrade fails), asserting the recorded call order <!-- R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: brew-managed present case records `trust --formula sahil87/tap/shll`, `upgrade sahil87/tap/shll`, then `shll install`, then `shll update`, in that order
- [x] A-002 R2: one `trust_shll` helper holds the probe + trust; both branches call it; a failing `trust --help` skips trust
- [x] A-003 R5: header + handoff comments describe the upgrade step

### Scenario Coverage

- [x] A-004 R3: non-brew present case records only the `list` probe (no trust/upgrade/install) before `shll install`
- [x] A-005 R3: fresh case records trust + `install sahil87/tap/shll`, no `upgrade`

### Edge Cases & Error Handling

- [x] A-006 R4: upgrade failure exits non-zero and `shll install` never runs

### Code Quality

- [x] A-007 Pattern consistency: POSIX sh, `"$BREW"` for every brew call, comment density matches the surrounding script
- [x] A-008 No unnecessary duplication: the trust probe exists once
- [x] A-009: `sh -n` and `dash -n` pass; shellcheck clean if available

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds new functionality without making existing code redundant (the previously inline trust probe was folded into the `trust_shll` helper in the same diff, not left orphaned)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Helper named `trust_shll`, defined at top level alongside the other helpers | Script already defines small top-level helpers (`phase_start`, `osc_progress`); `main()` truncation guard still holds since helpers only define | S:75 R:95 A:85 D:70 |
| 2 | Confident | Upgrade progress line wording: `shll found — upgrading sahil87/tap/shll via Homebrew...` | Mirrors the existing `shll not found — installing …` line | S:70 R:95 A:80 D:70 |

2 assumptions (0 certain, 2 confident, 0 tentative).
