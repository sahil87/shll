# Plan: Config-Home Toolkit Standard

**Change**: 260823-km8t-config-home-standard
**Intake**: `intake.md`

## Requirements

### Standards: The config-home document

#### R1: New standard page `docs/site/standards/config-home.md`
The repo SHALL gain a ninth canonical standard page, `docs/site/standards/config-home.md`, following the house register (single `#` H1 `Standard: config-home`; producer-facing intro naming the implemented principles — №6 stateless/deterministic and №4 fail-fast — and scope `binary`; MUST/SHOULD obligation sections; closing `## Verifying conformance` checklist). It SHALL carry these obligations:

1. **One fixed config root (MUST)**: a tool with a config file resolves it under `$HOME/.config/<tool-name>/`, built with `filepath.Join` from `$HOME` (the only environment input); `<tool-name>` is the full tool name (`run-kit`, not `rk`); MUST NOT honor `$XDG_CONFIG_HOME`, MUST NOT use `os.UserConfigDir` (macOS → `~/Library/Application Support`). Rationale stated in the page: determinism — the path is identical on every platform and in every process context by construction (daemon, CLI, agent-in-a-pane read the same file); dotfiles users symlink the directory. Unset `$HOME` → actionable error. A SHOULD-level pin: a test asserting env vars cannot move the path.
2. **Override order (MUST)**: code defaults < config file < env < CLI flag — one cascade, no per-key exceptions.
3. **Env is deployment bootstrap only (MUST)**: env forms exist only for keys needed at/before process start, per-deployment (e.g. run-kit's `RK_PORT`/`RK_HOST`); env is never an override channel for preference keys (naming the RK_AUTO_NAME failure mode).
4. **State is not config — the XDG asymmetry (MAY, bounded)**: XDG-honoring state dirs (`$XDG_STATE_HOME/<tool-name>/`) only for droppable, never-authoritative files; the asymmetry is deliberate and documented here.
5. **The fab-kit exception**: `~/.fab-kit/` is the documented, closed exception (config co-located with its version cache; XDG rejected in fab-kit's own decision record). New tools get no exception.
6. **Conformance receipts** (verified 2026-08-23): hop reference implementation (`src/internal/config/resolve.go` + env-immovability test), idea conforms (`systemConfigDir`), run-kit adopting (config-consolidation phase 1), wt/tu no config file today (bound when they grow one).

- **GIVEN** a fresh clone of any toolkit repo, **WHEN** an author adds a config file to a tool, **THEN** `shll standards config-home` states the fixed-root path rule, the cascade, the env restriction, the state-dir carve-out, and the closed exception list, with a Verifying-conformance checklist to audit against.

### Standards: Index integration

#### R2: `principles.md` references the eighth companion
`docs/site/standards/principles.md` SHALL be updated in its three companion-set locations: (a) the "Seven companion standards" intro paragraph becomes eight, gaining a clause for the new category (per-tool configuration) since config-home is neither documentation/help nor shll-composition; (b) "The contracts" table gains a `config-home` row (Implements №6, №4 | scope `binary` | one-line summary); (c) the closing "Consuming these standards" parenthesized list gains `config-home`. The ten principle sections themselves are untouched.

- **GIVEN** the rendered principles page, **WHEN** a reader scans the companion enumeration, the contracts table, and the consuming paragraph, **THEN** all three consistently list eight companions including `config-home`, and the intra-directory link resolves.

### Standards: Roster and embed registration

#### R3: `shll standards` serves `config-home`
The command surfaces SHALL be registered: `scripts/sync-standards.sh` `STANDARDS=(…)` array gains `config-home`; `src/cmd/shll/standards.go` `standardsRoster` gains a ninth (last) entry — `Name: "config-home"`, one-line `Description`, `Scope: "binary"`, `SourcePath: "docs/site/standards/config-home.md"`, `EmbedName: "config-home.md"`; the sync script is run and the refreshed embed copies committed (`src/cmd/shll/standards/config-home.md` new, `principles.md` copy updated). All roster-driven tests (list, JSON, byte-identical reader, `TestStandardsEmbedMatchesCanonical`, `TestStandardsRosterIntegrity`) pass with no test-code changes.

- **GIVEN** a build from HEAD, **WHEN** `shll standards` runs bare, **THEN** the list shows nine rows with `config-home` last at scope `binary`; **AND WHEN** `shll standards config-home` runs, **THEN** stdout is byte-identical to `docs/site/standards/config-home.md`.

### Non-Goals

- run-kit's actual config relocation (its own repo, phases 1–4 of the consolidated plan)
- Migrating fab-kit off `~/.fab-kit/` (the documented exception)
- Retrofitting wt/tu (no config file today)
- README.md reference-list changes (precedent: the install-composition addition made none)
- Any shll binary behavior beyond the embed roster

### Design Decisions

#### Principle mapping: №6 + №4
**Decision**: The standard implements №6 (stateless, therefore retry-safe — the deterministic-by-construction path is the config-side face of environment-independent behavior) and №4 (fail fast — the unset-`$HOME` error is actionable, never a silent fallback).
**Why**: №6's failure mode ("acts on a world that no longer exists") is exactly what an env-var-movable config path causes across process contexts; №4 covers the one error path the standard mandates.
**Rejected**: №8 (graceful degradation — a missing config file is each tool's own concern, not this standard's); a new eleventh principle (the ten are stable; contracts implement them).
*Introduced by*: 260823-km8t-config-home-standard

#### Scope `binary`
**Decision**: Roster scope is `binary`.
**Why**: The obligations are satisfied by the compiled tool's runtime path resolution, like `update`/`version`/`shell-init`; the SHOULD-level pin test does not create a repo half (those three standards also imply tests and stay `binary`).
**Rejected**: `binary+repo` (nothing lives canonically as a repo file the way the `skill` bundle does); a new scope value (`TestStandardsRosterIntegrity` pins the four-value vocabulary).
*Introduced by*: 260823-km8t-config-home-standard

## Tasks

### Phase 1: Core Implementation

- [x] T001 Author `docs/site/standards/config-home.md` — house register, six obligation areas per R1, conformance receipts, `## Verifying conformance` checklist; intra-family links same-directory only <!-- R1 -->
- [x] T002 [P] Update `docs/site/standards/principles.md` — companion enumeration (seven→eight, new per-tool-configuration clause), contracts table row, consuming-list addition <!-- R2 -->
- [x] T003 [P] Register the standard: append `config-home` to `scripts/sync-standards.sh` `STANDARDS` array; append the ninth `standardsRoster` entry in `src/cmd/shll/standards.go` <!-- R3 -->

### Phase 2: Integration & Verification

- [x] T004 Run `scripts/sync-standards.sh`; confirm `src/cmd/shll/standards/config-home.md` created and `principles.md` embed refreshed <!-- R3 -->
- [x] T005 Run `go test ./...` in `src/` — roster-driven standards tests and drift guard green with no test-code edits <!-- R3 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: `docs/site/standards/config-home.md` exists with the house register (H1 `Standard: config-home`, implements-principles intro naming №6/№4 and scope `binary`, obligation sections, `## Verifying conformance`)
- [x] A-002 R2: `principles.md` lists eight companions consistently in all three locations (intro enumeration, contracts table, consuming paragraph)
- [x] A-003 R3: `standardsRoster` and the sync array carry `config-home` (scope `binary`, source path/embed name correct); embed copies present in the working tree (ship commits them)

### Behavioral Correctness

- [x] A-004 R1: The page's obligations match the settled decisions verbatim — fixed `$HOME/.config/<tool-name>/` via `filepath.Join`, full tool name, no `$XDG_CONFIG_HOME`/`os.UserConfigDir`, cascade `defaults < config file < env < CLI flag`, env = deployment bootstrap keys only, XDG state gated on droppability, fab-kit the closed exception
- [x] A-005 R3: `shll standards` (dev build or test seam) lists nine entries with `config-home` last; `shll standards config-home` output byte-matches the canonical file (verified: 9 roster entries, `cmp` clean, `TestStandardsEmbedMatchesCanonical` green)

### Scenario Coverage

- [x] A-006 R3: `go test ./...` passes in `src/` — including `TestStandardsEmbedMatchesCanonical` and `TestStandardsRosterIntegrity` — with zero test-code changes

### Edge Cases & Error Handling

- [x] A-007 R1: Intra-family links in the new page resolve inside `docs/site/standards/` with no `..` escape; links leaving the published set are absolute URLs (docs/site closure rule)

### Code Quality

- [x] A-008 Pattern consistency: the new page follows the register and tone of the existing eight; the roster entry mirrors the existing entries' field shapes
- [x] A-009 No unnecessary duplication: the page states obligations once and cites receipts, rather than duplicating hop/run-kit implementation detail; no magic strings added to `standards.go` beyond the roster row

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Principle mapping finalized as №6 + №4 (see Design Decisions) | Best textual fit; contracts-table Implements cell needs exactly this | S:50 R:85 A:65 D:50 |
| 2 | Confident | Roster position: appended last (ninth), matching every prior addition | Roster order == output order; additions have always appended | S:60 R:90 A:85 D:80 |
| 3 | Certain | Description line: "Fixed $HOME/.config/<tool>/ config root, override cascade, env restricted to deployment bootstrap keys" (≈ the glossary contract) | Mirrors existing description style; hardcoded-not-parsed rule | S:70 R:95 A:90 D:85 |

3 assumptions (1 certain, 2 confident, 0 tentative).
