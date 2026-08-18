# Plan: Install Bootstrap Gaps

**Change**: 260817-nava-install-bootstrap-gaps
**Intake**: `intake.md`

## Requirements

### Bootstrap Script: Preflight

#### R1: Platform-correct dependency probes before the brew check
`scripts/install.sh` SHALL run a preflight step, before any Homebrew check, that probes: **git** (Darwin: `xcode-select -p >/dev/null 2>&1`; Linux: `command -v git >/dev/null 2>&1`), **curl** (`command -v curl >/dev/null 2>&1`), and **tmux** (`command -v tmux >/dev/null 2>&1`). On Darwin the git probe MUST NOT use `command -v git` — the CLT shim at `/usr/bin/git` false-positives when the Command Line Tools are not installed.

- **GIVEN** a fresh macOS machine without Command Line Tools (the `/usr/bin/git` shim present)
- **WHEN** the script's preflight runs
- **THEN** the git probe reports git/CLT missing (`xcode-select -p` exits non-zero), never "present"

#### R2: Consolidated missing-deps report with per-platform fix commands
The preflight SHALL collect every failed probe and print them **all at once** in a single consolidated block, each line carrying a fix command appropriate to the detected platform (macOS: `xcode-select --install` for git/CLT; Debian/Ubuntu-style `sudo apt-get install -y <pkg>` hints on Linux, with generic package-manager wording as fallback). It MUST NOT fail on the first missing dependency.

- **GIVEN** a Linux VM missing both git and tmux
- **WHEN** the preflight runs
- **THEN** one block lists both misses with their fix commands before any exit

#### R3: Fatality matrix
After the consolidated report, the preflight SHALL apply these exit semantics:
- **curl missing → fatal** (exit 1) — brew.sh and brew both require it.
- **git missing → fatal on Linux when brew is absent** (brew.sh's Linux prerequisite); **fatal on macOS when brew is already present** (brew's git/tap operations need real CLT); **informational-only on macOS when brew is absent** (the bootstrap's `NONINTERACTIVE=1` brew.sh installs CLT itself via `softwareupdate` — print a note saying so instead of failing).
- **tmux missing → warn-only, never fatal** (run-kit *runtime* dependency, not an install prerequisite — Constitution V).

- **GIVEN** a fresh macOS machine with neither CLT nor Homebrew
- **WHEN** the preflight completes
- **THEN** the script proceeds to the Homebrew bootstrap, printing an informational CLT note rather than exiting

- **GIVEN** a machine where only tmux is missing
- **WHEN** the preflight completes
- **THEN** a warning with the tmux fix command is printed and the script proceeds (exit path unaffected)

### Bootstrap Script: Homebrew bootstrap

#### R4: Bootstrap Homebrew headlessly when absent
When `command -v brew` fails, the script SHALL print a progress line and run the official installer headlessly: `NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`. The bootstrap is unconditional — no opt-in flag (stdin is the pipe; positional args are the `shll install` subset). The previous hard stop ("Homebrew is required … exit 1") is removed. Under `set -eu` a failing brew.sh aborts the script with the installer's own error output.

- **GIVEN** preflight passed and `brew` is not on `PATH`
- **WHEN** `main` reaches the brew step
- **THEN** the official installer runs with `NONINTERACTIVE=1` via `/bin/bash -c` and the script continues on success
- **AND** when `brew` IS on `PATH`, no bootstrap runs and behavior is unchanged from today

#### R5: Absolute brew path (`$BREW`) for the rest of the run
The script SHALL resolve a `BREW` variable once — `command -v brew` when brew is on `PATH`; otherwise (post-bootstrap) the first executable among `/opt/homebrew/bin/brew` (Apple Silicon), `/usr/local/bin/brew` (Intel macOS), `/home/linuxbrew/.linuxbrew/bin/brew` (Linux) — and use `$BREW` for **every** subsequent brew invocation (trust probe, `brew trust`, `brew install`). If no candidate is executable after a bootstrap, the script SHALL print a clear error and exit 1 (never proceed with a bare `brew` that cannot resolve).

- **GIVEN** the bootstrap just installed Homebrew (not on `PATH` in this process)
- **WHEN** the script trusts/installs `sahil87/tap/shll`
- **THEN** those commands run via the absolute `$BREW` path and succeed

#### R6: shellenv — in-process eval plus printed rc guidance
After a bootstrap, the script SHALL run `eval "$($BREW shellenv)"` in its own process **before** `exec shll install "$@"` (the freshly installed `shll` lives in the brew prefix and is otherwise not found), and SHALL print the persistent rc line for the user (e.g. `eval "$(/opt/homebrew/bin/brew shellenv)"`) with a one-line pointer that this is what makes brew (and the installed tools) resolvable in future shells. No shellenv output is printed when brew was already on `PATH`.

- **GIVEN** the bootstrap ran on a fresh machine
- **WHEN** the script reaches the exec hand-off
- **THEN** `shll` resolves (shellenv eval'd in-process) and the user has seen the exact rc line to persist

#### R7: Script invariants preserved
The script SHALL remain POSIX `sh` (brew.sh is invoked via `/bin/bash -c` — the outer script itself stays sh), keep `set -eu`, keep the whole body inside `main() { … }` with `main "$@"` as the last line (truncation guard), remain fully non-interactive, and stay at the pinned path `scripts/install.sh` (shll.ai raw-fetch URL contract). The shll trust-then-install block and the `exec shll install "$@"` hand-off remain in substance (now via `$BREW`). `sh -n scripts/install.sh` MUST pass; shellcheck when available.

- **GIVEN** the rewritten script
- **WHEN** `sh -n scripts/install.sh` runs
- **THEN** it exits 0, and inspection confirms `main "$@"` is still the sole invocation on the last line

### Docs: Fresh-VM corrections

#### R8: Failed-download pitfall documented
The docs SHALL explain that a failed download makes `curl -fsSL … | sh` **exit 0 silently** — `sh` runs empty input successfully, so `&& next-step` chains continue; curl's error appears on stderr (`-S`) but the exit code cannot be trusted; the `main()` wrapper protects against *partial* execution, not *failed* download. The piped one-liner remains the recommended form. Full note in `docs/site/install.md`; a brief note or pointer in `README.md`.

- **GIVEN** a reader of the install guide
- **WHEN** they read the one-liner section
- **THEN** they learn what a silent no-op means and to check `command -v shll` / stderr

#### R9: Minimal-Ubuntu curl prerequisite documented
The docs SHALL state that minimal Ubuntu/Debian images ship without curl and the one-liner needs `sudo apt-get install -y curl` first.

- **GIVEN** a minimal Ubuntu user
- **WHEN** they follow the install guide
- **THEN** the curl prerequisite is stated before the one-liner is presented

#### R10: Tap-qualified formula names everywhere
Every brew formula reference in `README.md` and `docs/site/` SHALL be tap-qualified (`sahil87/tap/<formula>`), and the docs SHALL carry a warning that homebrew/core now has an **unrelated** `run-kit` formula (a bare `brew install run-kit` installs the wrong software).

- **GIVEN** the docs after this change
- **WHEN** grepping for roster formula names in brew commands
- **THEN** zero bare (untap-qualified) references remain, and the run-kit collision warning exists

#### R11: Homebrew requirement reframed
`README.md`, `docs/site/install.md`, and `docs/site/workflows.md` SHALL replace the "requires Homebrew / never auto-installs it" framing with "bootstraps Homebrew headlessly when absent (official installer, `NONINTERACTIVE=1`)", including the post-bootstrap shellenv guidance. The manual `brew trust … && brew install sahil87/tap/shll` bootstrap remains documented as the by-hand alternative. Per the constitution's Toolkit Standards clause, the README/docs edits are checked against the governing standards under `docs/site/standards/` (at minimum `install-composition`).

- **GIVEN** the docs after this change
- **WHEN** searching for "never auto-installs"
- **THEN** no doc makes that claim, and all three files describe the bootstrap consistently

### Non-Goals

- No Go changes — `shll install` (including `installBrewMissingHint`) is untouched; everything added runs pre-brew/pre-shll.
- No CI wiring for script tests — the gate stays `sh -n` + shellcheck-when-available (per the existing ci/install-bootstrap contract).
- No wget fallback for the one-liner; no auto-install of tmux (warn + fix command only).
- No change to the shll.ai raw-fetch path or the site repo.

### Design Decisions

#### Reverse "Require Homebrew; never auto-install it"
**Decision**: The bootstrap script installs Homebrew headlessly (`NONINTERACTIVE=1` brew.sh) when absent.
**Why**: User-directed reversal after fresh-VM testing (2026-08-17) proved the installer runs fully headless on macOS (including CLT via `softwareupdate`) and Linux (given git) — removing the original "large, surprising side effect" objection; the hard stop contradicted the "clean machine to wired toolkit" promise.
**Rejected**: Keeping the https://brew.sh pointer + exit 1 — proven to be the single biggest fresh-VM friction point, and the brew-not-on-PATH trap made even the manual path fail on re-run.
*Introduced by*: 260817-nava-install-bootstrap-gaps

#### Preflight and bootstrap live in the script, not `shll install`
**Decision**: All new logic goes in `scripts/install.sh`; `shll install` is unchanged.
**Why**: Every added step runs before brew and shll exist — exactly the circularity carve-out the thin-bootstrap design grants the script (Constitution III). Post-brew intelligence stays in Go.
**Rejected**: Teaching `shll install` to bootstrap brew — unreachable code (you cannot have shll without brew having worked).
*Introduced by*: 260817-nava-install-bootstrap-gaps

## Tasks

### Phase 1: Setup

- [x] T001 Read the governing standards and current state: `docs/site/standards/install-composition.md` (and any README/docs-structure standard present under `docs/site/standards/`), current `scripts/install.sh`, and the three docs targets (`README.md`, `docs/site/install.md`, `docs/site/workflows.md`); confirm edit constraints before touching them <!-- R7, R10, R11 -->

### Phase 2: Core Implementation (scripts/install.sh)

- [x] T002 Add `preflight()` to `scripts/install.sh`: platform detection via `uname -s`, the three probes (Darwin git via `xcode-select -p`, Linux git via `command -v`; curl; tmux), consolidated all-at-once missing-deps report with per-platform fix commands, and the fatality matrix (curl fatal; git conditional on platform × brew presence; tmux warn-only) <!-- R1, R2, R3 -->
- [x] T003 Add the Homebrew bootstrap to `scripts/install.sh`: when `command -v brew` fails, progress line + `NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh)"`; then resolve `$BREW` (PATH hit, else the three known prefixes, else clear error + exit 1) <!-- R4, R5 -->
- [x] T004 Thread `$BREW` through the trust probe / `brew trust` / `brew install` block and add the shellenv handling: post-bootstrap `eval "$($BREW shellenv)"` before `exec shll install "$@"`, plus the printed rc line; preserve all invariants (POSIX sh, `set -eu`, `main()` wrapper, pinned path) <!-- R5, R6, R7 -->
- [x] T005 Lint gate: run `sh -n scripts/install.sh`; run shellcheck if available; fix any findings <!-- R7 --> (`sh -n` and `dash -n` pass; shellcheck not installed on this machine)

### Phase 3: Docs

- [x] T006 [P] Update `README.md` `## Install`: reframe "never auto-installs Homebrew" → headless bootstrap, add the minimal-Ubuntu curl prerequisite and a brief failed-download pitfall note (or pointer to the install guide), audit for tap-qualified names <!-- R8, R9, R10, R11 -->
- [x] T007 [P] Update `docs/site/install.md`: bootstrap section reframe (headless brew bootstrap + shellenv guidance, manual path retained), full failed-download pitfall note, curl prerequisite, tap-qualified name audit + run-kit collision warning <!-- R8, R9, R10, R11 -->
- [x] T008 [P] Update `docs/site/workflows.md`: fresh-machine walkthrough reframe to match the new bootstrap behavior, tap-qualified name audit <!-- R10, R11 -->

## Execution Order

- T002 → T003 → T004 are sequential edits to the same file; T005 gates after them
- T006–T008 are parallel-safe (different files) and independent of Phase 2 content except the final behavior they describe — run after T004 so the described behavior is settled

## Acceptance

### Functional Completeness

- [x] A-001 R1: The preflight probes git (Darwin: `xcode-select -p`, with no `command -v git` on the Darwin path), curl, and tmux before any brew check
- [x] A-002 R2: All missing deps are reported in one consolidated block with per-platform fix commands (no fail-on-first)
- [x] A-003 R3: The fatality matrix is implemented exactly: curl fatal; git fatal on Linux-sans-brew and macOS-with-brew, informational on macOS-sans-brew; tmux warn-only
- [x] A-004 R4: With brew absent, the official installer runs via `NONINTERACTIVE=1 /bin/bash -c "$(curl -fsSL …/install.sh)"` after a progress line, unconditionally (no flag)
- [x] A-005 R5: Every brew invocation after resolution goes through `$BREW`; the three known prefixes are probed post-bootstrap
- [x] A-006 R6: Post-bootstrap, `eval "$($BREW shellenv)"` runs in-process before the exec, and the persistent rc line is printed
- [x] A-007 R7: `sh -n scripts/install.sh` passes; `set -eu`, the `main()` wrapper with last-line `main "$@"`, and the pinned path are intact

### Behavioral Correctness

- [x] A-008 R4: With brew already on `PATH`, no bootstrap runs, no shellenv line is printed, and the trust/install/exec flow matches today's behavior (via `$BREW = command -v brew`)

### Removal Verification

- [x] A-009 R4: The old "Homebrew is required but was not found … exit 1" hard-stop block is gone from `scripts/install.sh`

### Scenario Coverage

- [x] A-010 R3: The four preflight scenarios trace correctly through the script logic by inspection: (a) fresh macOS no-CLT-no-brew → informational CLT note + bootstrap proceeds; (b) macOS CLT-missing-brew-present → fatal with `xcode-select --install`; (c) Linux no-git-no-brew → fatal with apt hint; (d) only tmux missing → warn + proceed

### Edge Cases & Error Handling

- [x] A-011 R5: Bootstrap reports success but no brew is found at any known prefix → clear error message and exit 1 (never a silent bare-`brew` failure)
- [x] A-012 R8: The pitfall note states both facts: failed download → pipeline exits 0, and curl's error is visible on stderr

### Docs Completeness

- [x] A-013 R9: The minimal-Ubuntu curl prerequisite is documented before the one-liner in the install guide
- [x] A-014 R10: Zero bare roster-formula brew references remain in `README.md`/`docs/site/`; the unrelated-run-kit collision warning is present
- [x] A-015 R11: No doc claims Homebrew is "never auto-installed"; README, install.md, and workflows.md describe the headless bootstrap consistently, with the manual path retained

### Code Quality

- [x] A-016 Pattern consistency: The script edits match the existing script's POSIX-sh style (comment voice, guard style); docs edits match each file's voice and structure
- [x] A-017 No unnecessary duplication: probe/report helpers are single-sourced within the script (no per-platform copy-paste blocks beyond the platform matrix itself)

### Security

- [x] A-018 R4: The bootstrap fetches brew.sh only over HTTPS from the official Homebrew raw URL; the script itself never invokes sudo; no new code paths eval remote content beyond the official installer invocation

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- None — this change adds new functionality (preflight, bootstrap, `$BREW`/shellenv handling) without making existing code redundant. The old brew-required hard-stop block was a *planned* removal (R4), verified under Acceptance § Removal Verification (A-009), not a discovered candidate.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | brew.sh URL is the official `https://raw.githubusercontent.com/Homebrew/install/HEAD/install.sh` invoked via `/bin/bash -c` | Homebrew's documented install command; intake row 12 fixed the bash-invocation decision | S:90 R:85 A:95 D:90 |
| 2 | Confident | Linux fix commands use Debian/Ubuntu `sudo apt-get install -y <pkg>` wording with a generic package-manager fallback line | Fresh-VM testing was Ubuntu; Debian-family dominates fresh-VM installs; other distros get the generic line | S:60 R:90 A:75 D:70 |
| 3 | Confident | Post-bootstrap `$BREW` resolution probes exactly three prefixes: `/opt/homebrew/bin/brew`, `/usr/local/bin/brew`, `/home/linuxbrew/.linuxbrew/bin/brew` | These are the official installer's only install prefixes on supported platforms (Constitution: darwin/linux only) | S:70 R:85 A:85 D:80 |
| 4 | Confident | README carries a brief pitfall note with the full explanation living in `docs/site/install.md` | install-composition standard centralizes install docs on shll.ai; README is deliberately slim (bootstrap + pointer) | S:65 R:90 A:80 D:75 |
| 5 | Confident | git missing on Linux *with brew present* → warn-only (not fatal) | The R3 matrix enumerates only Linux-sans-brew (fatal), macOS-with-brew (fatal), macOS-sans-brew (informational); the Linux-with-brew cell is unspecified — brew's presence implies git was there at brew install time, so this is a rare degraded state and the consolidated fix hint (`apt-get install git`) is the actionable surface without blocking the run | S:45 R:80 A:60 D:50 |

5 assumptions (1 certain, 4 confident, 0 tentative).
