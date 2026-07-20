# Plan: Update, Version, and Shell-Init Toolkit Standards

**Change**: 260719-y367-update-version-shell-init-standards
**Intake**: `intake.md`

## Requirements

<!-- Requirements derived from intake.md What Changes §1–§4. RFC-2119 keywords;
     each requirement carries at least one GIVEN/WHEN/THEN scenario. This change
     is docs-typed: it authors three new producer-facing standards documents and
     registers them. No shll behavior change (intake §5) — the standards codify
     shll's already-implemented probe behavior verbatim. -->

### Standards Documents: `update` standard

#### R1: `update` standard document exists and is producer-facing
A new canonical document `docs/site/standards/update.md` SHALL specify the producer-facing obligations for a tool's `update` surface, in RFC-2119 form matching the existing standards' voice (single `#` H1 titled `Standard: update`, an "implements principle №N" cross-reference line, "rules with teeth", and a "Verifying conformance" section). It MUST bind the six roster tools (`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit`) and MUST explicitly place shll itself out of producer scope (shll is the consumer; `shll update` self-upgrades via `brew upgrade sahil87/tap/shll`).

- **GIVEN** the standards tree at `docs/site/standards/`
- **WHEN** `docs/site/standards/update.md` is authored
- **THEN** it opens with `# Standard: update`, names the principle it implements, and states its producer scope is the six roster tools (shll excluded as consumer)

#### R2: `update` standard states the subcommand, flag-probe, exit-code, and brew-safety obligations
`update.md` MUST require: (a) a tool MUST expose an `update` subcommand that upgrades the tool in place and owns its own post-upgrade side effects; (b) a tool MUST advertise `--skip-brew-update` as a literal substring in `<tool> update --help` and honor it (skipping the tool's internal `brew update`), documenting that discovery is a substring probe — a frozen textual contract; (c) exit code 0 on success including already-up-to-date, non-zero only on genuine failure; (d) the brew-handling safety clause — a tool MUST NOT SIGKILL a package-manager subprocess mid-transaction and MUST NOT impose short hard timeouts on `brew upgrade`; any bound SHOULD be generous and terminate gracefully (SIGTERM + grace), never SIGKILL.

- **GIVEN** a tool author reading `update.md`
- **WHEN** they implement or audit their `update` subcommand
- **THEN** the four obligations (subcommand, `--skip-brew-update` literal substring + honor, exit-code semantics, brew-safety MUST-NOTs) are each stated as RFC-2119 rules with their failure mode
- **AND** the brew-safety clause cites the observed failure (a stalled GitHub-API call inside `brew upgrade` exceeding a 120s hard kill, corrupting the keg mid-swap)

#### R3: `update` standard folds in the naming/release-alignment conventions
`update.md` MUST include the naming and release-alignment conventions (no fourth standard is minted): GitHub repo name == roster/tool name == tap formula leaf == binary name; releases tagged `v{semver}` matching the brew formula version (consumed by `shll changelog` and the update digest); a rename MUST ship a tap `formula_renames.json` entry. It SHOULD also state that a tool self-updates via brew only when brew-installed (detected via `os.Executable()` symlink resolution containing `/Cellar/`), degrading with a clear message on a non-brew install rather than erroring.

- **GIVEN** the naming-alignment rules have no standalone home
- **WHEN** `update.md` is authored
- **THEN** the repo==roster==formula-leaf==binary identity, the `v{semver}` tag rule, and the `formula_renames.json` rename obligation appear as a section of `update.md`

### Standards Documents: `version` standard

#### R4: `version` standard document exists and binds all seven binaries
A new canonical document `docs/site/standards/version.md` SHALL specify the producer-facing obligations for the `--version` surface, in the existing standards' voice, binding all seven binaries (the six roster tools **plus shll itself**).

- **GIVEN** the standards tree
- **WHEN** `docs/site/standards/version.md` is authored
- **THEN** it opens with `# Standard: version`, names the principle it implements, and states its scope is all seven binaries including shll

#### R5: `version` standard states the support, timing, first-line-token, and binary-name obligations
`version.md` MUST require: (a) a tool MUST support `--version` and exit 0; (b) it MUST respond within 2 seconds, which implies no network I/O on the version path (consumers run it under a 2s timeout); (c) the version MUST appear on the first non-empty line, containing a token matching `v?\d+(\.\d+)*([.-][\w.+-]+)?` or the `<word> version <rest>` prefix shape — the parser never scans past line 1, so a banner-first layout is non-conformant; the RECOMMENDED canonical shape is `<tool> version vX.Y.Z`; (d) the binary name on PATH MUST equal the tool name, since the version probe doubles as shll's install-mechanism-agnostic install probe (a differently-named binary reads as "not installed").

- **GIVEN** a tool author reading `version.md`
- **WHEN** they implement or audit their `--version` output
- **THEN** the four obligations are each stated as RFC-2119 rules with their failure mode and the consumer rationale (2s `versionTimeout`, first-non-empty-line-only parse, name-equals-tool install probe)

### Standards Documents: `shell-init` standard

#### R6: `shell-init` standard document exists and binds shell-integration tools
A new canonical document `docs/site/standards/shell-init.md` SHALL specify the producer-facing obligations for the `shell-init` surface, in the existing standards' voice, binding tools that expose shell integration (today `tu`, `hop`, `wt`), noting `shll shell-init` is the consumer/composer that conforms by construction.

- **GIVEN** the standards tree
- **WHEN** `docs/site/standards/shell-init.md` is authored
- **THEN** it opens with `# Standard: shell-init`, names the principle it implements, and states its scope is the shell-integration tools with shll as the composer

#### R7: `shell-init` standard states the eval-safety, stream-split, and failure-exit obligations
`shell-init.md` MUST require: (a) `<tool> shell-init <shell>` for `zsh` and `bash` MUST emit eval-safe shell code on stdout and exit 0, with stdout containing ONLY shell source for the named shell (no prompts, colors, banners, or warnings); (b) diagnostics go to stderr only; (c) on any failure the tool MUST exit non-zero, because the composer drops a tool's stdout from the blob only when the exit code signals failure — printing junk to stdout while exiting 0 poisons every `eval "$(shll shell-init …)"`; (d) an unsupported/missing shell argument MUST exit non-zero with a usage message on stderr (shll's own convention is exit 2 for usage errors).

- **GIVEN** a tool author reading `shell-init.md`
- **WHEN** they implement or audit their `shell-init` subcommand
- **THEN** the four obligations are each stated as RFC-2119 rules with the eval-safety invariant and its failure mode

### Registration: sync + embed + roster

#### R8: The three standards are synced and committed as embed copies
`scripts/sync-standards.sh`'s `STANDARDS=(…)` array MUST include `update`, `version`, and `shell-init`; running the script MUST copy the three canonical sources (and the re-synced `principles.md`) into `src/cmd/shll/standards/`, and those embed copies MUST be committed so a clean `go build ./...` compiles.

- **GIVEN** the sync script naming four standards
- **WHEN** `update version shell-init` are added to the array and the script is run
- **THEN** `src/cmd/shll/standards/{update,version,shell-init}.md` exist byte-identical to their `docs/site/standards/` sources, and `principles.md` is re-synced

#### R9: The three standards are registered in `standardsRoster` at scope `binary`
`standardsRoster` (`src/cmd/shll/standards.go`) MUST gain three entries — `update`, `version`, `shell-init` — each with a `Name`, a one-line `Description`, `Scope: "binary"` (obligations live in the tool's CLI binary), the repo-relative `SourcePath` under `docs/site/standards/`, and matching `EmbedName`. The roster grows from four to seven entries.

- **GIVEN** the four-entry roster
- **WHEN** the three entries are appended
- **THEN** `shll standards` lists seven standards, `shll standards --json` emits seven objects, and `shll standards {update,version,shell-init}` each print their embedded document byte-identically
- **AND** `TestStandardsRosterIntegrity` passes (non-empty fields, no duplicate names, `Scope` ∈ valid set, `SourcePath` under `docs/site/standards/`, basename == `EmbedName`)

#### R10: Existing drift-guard and roster tests extend automatically with no count-pinned golden broken
The roster-driven tests (`TestStandardsEmbedMatchesCanonical`, `TestStandards_ListTable`, `TestStandards_ListJSON`, `TestStandards_ReadDoc_ByteIdentical`, `TestStandardsRosterIntegrity`, `TestStandards_UnknownName`) MUST pass unchanged against the seven-entry roster. No test or golden pinning a literal four-standard count may remain broken.

- **GIVEN** the roster-driven test suite
- **WHEN** the roster grows to seven
- **THEN** `go test ./cmd/shll/` passes with no test asserting a hardcoded count of 4

### Cross-references: `principles.md`

#### R11: `principles.md` names the three new companion standards and each new page cross-references its principle
`docs/site/standards/principles.md` MUST update its companion-standards sentence (currently "Three companion standards make principles №3 and №10 concrete…") to also name the three new pages, and the "The contracts" table MUST gain a row per new standard (Contract link, Implements principle №, Scope, What it standardizes). Each new page MUST carry an "implements principle №N" cross-reference line (exact principle numbers chosen at authoring from the ten in `principles.md`). Because `principles.md` is itself an embedded standard, its edit re-runs the sync.

- **GIVEN** `principles.md`'s companion-standards prose and "The contracts" table naming three mechanical contracts
- **WHEN** the three new standards are published
- **THEN** the companion sentence names them, the table gains three rows, and each new page names the principle(s) it implements
- **AND** the re-synced `principles.md` embed copy stays byte-identical (drift guard green)

### Non-Goals

- No shll behavior change — probes, timeouts, and regexes stay byte-identical (intake §5). The standards codify shll's existing implemented behavior; they do not redesign it.
- No new shll subcommand (Constitution VII untouched); `shll standards` simply enumerates more entries.
- No conformance audits of the six tool repos — rollout/enforcement (the std1/std2 directive pattern, fab-kit's SIGKILL fix) is separate work.
- shll-side consumer machinery documentation stays in `docs/memory/cli/{update,version,shell-init}.md` — not moved into the standards.

### Design Decisions

#### Fold release/naming-alignment rules into `update.md`, not a fourth standard
**Decision**: The repo==roster==formula-leaf==binary identity, `v{semver}` tagging, and `formula_renames.json` rename rule live as a section of `update.md`.
**Why**: The update path is where brew/formula interaction lives, so the alignment rules are load-bearing exactly there; the intake recommended folding "into whichever comes first"; it is cheap to relocate to a standalone standard later.
**Rejected**: Minting a fourth `naming`/`release` standard now — premature; adds a roster entry and a page for rules that only bite on the update/brew path.
*Introduced by*: `260719-y367-update-version-shell-init-standards`

#### Scope `binary` for all three new standards
**Decision**: Each new roster entry carries `Scope: "binary"`.
**Why**: The obligations are satisfied by the compiled tool at runtime (the `update`/`--version`/`shell-init` subcommands' behavior), exactly like help-dump's `binary` scope — not by repo file structure.
**Rejected**: `binary+repo` (nothing in these three lives canonically as a repo file the way the `skill` bundle does); a new scope value (the four-value vocabulary already fits, and `TestStandardsRosterIntegrity` pins that set).
*Introduced by*: `260719-y367-update-version-shell-init-standards`

## Tasks

### Phase 1: Author the canonical standards documents

- [x] T001 [P] Author `docs/site/standards/update.md` <!-- rework: review cycle 1 — (should-fix) line 7 asserts shll "has no `update` subcommand" which is false as written (src/cmd/shll/update.go defines `shll update`) — reword to the delegation-loop sense; (should-fix) lines 13–16 and 29–34 state the update-subcommand and exit-code obligations without RFC-2119 keywords — add MUSTs; (nice-to-have, optional) line 29 "exit code is the only truth the summary tail and digest read" is imprecise — digest keys on brew-read version transitions --> — producer-facing `update` standard: `# Standard: update` H1, implements-principle line, scope (six roster tools; shll excluded as consumer), and the obligation sections (update subcommand + own side effects; `--skip-brew-update` literal-substring probe + honor; exit-code semantics incl. already-up-to-date=0; brew-safety MUST-NOTs with the 2026-07-19 incident; brew-only-when-brew-installed SHOULD via `/Cellar/` detection; naming/release alignment fold), plus a "Verifying conformance" section. Match the voice/register of `docs/site/standards/help-dump.md`. <!-- R1 R2 R3 -->
- [x] T002 [P] Author `docs/site/standards/version.md` <!-- rework: review cycle 1 — (MUST-FIX) line 7 claims shll's self-update post-check parses `shll --version`; no such probe exists (digest reads brew via installedVersion, update.go:369; memory pins "never a shll --version self-subprocess") — reword the scope rationale; (should-fix) lines 32/35 "finds no token → unreportable" mismatches normalizeVersion's first-line-verbatim fallback — tighten failure-mode sentence; (nice-to-have, optional) line 41 absolute overlooks the transitional rk→run-kit legacy-name fallback --> — producer-facing `version` standard: `# Standard: version` H1, implements-principle line, scope (all seven binaries incl. shll), and the obligations (`--version` exit 0; 2s response / no network on version path; version on first non-empty line matching the token regex or `<word> version <rest>` prefix, parser never scans past line 1, RECOMMENDED `<tool> version vX.Y.Z`; binary name on PATH == tool name / install-probe rationale), plus "Verifying conformance". Match the existing standards' voice. <!-- R4 R5 -->
- [x] T003 [P] Author `docs/site/standards/shell-init.md` <!-- rework: review cycle 1 — (should-fix) line 40's Verifying-conformance bullet requires "nothing to stderr" on the happy path, contradicting §Diagnostics (line 20) which blesses stderr hints/warnings — align the checklist with the section --> — producer-facing `shell-init` standard: `# Standard: shell-init` H1, implements-principle line, scope (shell-integration tools tu/hop/wt; shll composer conforms by construction), and the obligations (eval-safe stdout-only shell code for zsh/bash + exit 0; diagnostics to stderr only; any failure MUST exit non-zero — the exit-code-gated drop invariant; unsupported/missing shell → non-zero + usage on stderr, exit 2 convention), plus "Verifying conformance". Match the existing standards' voice. <!-- R6 R7 -->

### Phase 2: Cross-reference principles.md

- [x] T004 Update `docs/site/standards/principles.md` <!-- rework: review cycle 1 — (should-fix) README.md Reference list (lines ~310–313) names the four existing standards pages but none of the three new ones — add them per the per-page listing pattern; (nice-to-have, optional) principles.md:35 contracts-table row consistency: shell-init row omits №4 though shell-init.md leans on it for exit-2 usage errors -->: revise the companion-standards sentence to also name `update`/`version`/`shell-init`, add a row per new standard to "The contracts" table (Implements/Scope/What it standardizes), and add each new page's "Consuming these standards" companion-link if that pattern applies. Choose the exact "implements principle №N" mapping per page (update → №7 compose-don't-reinvent + brew safety; version → №4 fail-fast/exit + №2 stdout-is-data; shell-init → №2 stdout-is-data eval-safety) and ensure each Phase-1 page's implements-line matches. <!-- R11 -->

### Phase 3: Registration — sync, embed, roster

- [x] T005 Add `update version shell-init` to the `STANDARDS=(…)` array in `scripts/sync-standards.sh`. <!-- R8 -->
- [x] T006 Register three `standardsRoster` entries in `src/cmd/shll/standards.go` (`update`, `version`, `shell-init`), each `Scope: "binary"`, one-line `Description`, `SourcePath: docs/site/standards/<name>.md`, `EmbedName: <name>.md`, appended after the `skill` entry (roster order = output order). <!-- R9 -->
- [x] T007 Run `scripts/sync-standards.sh` <!-- rework: review cycle 1 — re-sync embed copies after the T001–T004 wording fixes (drift guard enforces canonical/embed pairing) --> (or `just sync-standards`) to copy the three new canonical sources and the re-synced `principles.md` into `src/cmd/shll/standards/`, producing the committed embed copies. <!-- R8 R11 -->

### Phase 4: Verify

- [x] T008 Run `go test ./cmd/shll/` <!-- rework: review cycle 1 — re-verify after rework edits --> (from `src/`) — confirm the roster-driven tests pass at seven entries (drift guard, list table/JSON, byte-identical reader, roster integrity, unknown-name valid-list) and no count-pinned golden breaks. Also `go build ./...` and `go vet ./cmd/shll/`. <!-- R10 -->

## Execution Order

- T001, T002, T003 are `[P]` (three independent new files).
- T004 depends on T001–T003 (its implements-№ mapping must match each page's implements-line).
- T005 and T006 may proceed after Phase 1/2 authoring (they name the files).
- T007 depends on T001–T006 (it copies the finalized canonical sources, including the edited `principles.md`, into the embed dir).
- T008 depends on T007 (the drift guard compares committed embed copies against canonical sources).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `docs/site/standards/update.md` exists, opens with `# Standard: update`, is producer-facing, binds the six roster tools, and excludes shll as consumer.
- [x] A-002 R2: `update.md` states the update-subcommand, `--skip-brew-update` literal-substring-probe-and-honor, exit-code (incl. already-up-to-date=0), and brew-safety (no SIGKILL / no short hard timeout, SIGTERM+grace) obligations as RFC-2119 rules.
- [x] A-003 R3: `update.md` folds in the naming/release-alignment rules (repo==roster==formula-leaf==binary, `v{semver}` tags, `formula_renames.json` rename) and the brew-only-when-brew-installed SHOULD.
- [x] A-004 R4: `docs/site/standards/version.md` exists, opens with `# Standard: version`, and binds all seven binaries including shll.
- [x] A-005 R5: `version.md` states the `--version`/exit-0, 2s/no-network, first-non-empty-line token (regex + prefix shape, no scan past line 1, RECOMMENDED shape), and binary-name-equals-tool-name obligations.
- [x] A-006 R6: `docs/site/standards/shell-init.md` exists, opens with `# Standard: shell-init`, and binds tu/hop/wt with shll as composer.
- [x] A-007 R7: `shell-init.md` states the eval-safe-stdout-only + exit-0, diagnostics-to-stderr, failure-MUST-exit-non-zero (exit-code-gated drop), and unsupported-shell → non-zero+usage obligations.
- [x] A-008 R8: `scripts/sync-standards.sh` `STANDARDS` array names all seven standards, and running it produces committed `src/cmd/shll/standards/{update,version,shell-init}.md` byte-identical to canonical.
- [x] A-009 R9: `standardsRoster` has seven entries; the three new ones carry `Scope: "binary"`, correct `SourcePath`/`EmbedName`; `shll standards` lists seven, `--json` emits seven, and each new `shll standards <name>` prints its document byte-identically.
- [x] A-010 R11: `principles.md`'s companion sentence names the three new standards, "The contracts" table gains three rows, and each new page carries an "implements principle №N" line consistent with the table.

### Behavioral Correctness

- [x] A-011 R10: `go test ./cmd/shll/` passes; `TestStandardsEmbedMatchesCanonical`, `TestStandards_ListTable`, `TestStandards_ListJSON`, `TestStandards_ReadDoc_ByteIdentical`, `TestStandardsRosterIntegrity`, and `TestStandards_UnknownName` all pass against the seven-entry roster with no count-pinned golden broken.

### Scenario Coverage

- [x] A-012 R5: The `version.md` first-line-token rule matches shll's live `versionTokenRE` (`v?\d+(\.\d+)*([.-][\w.+-]+)?`) and `versionPrefixRE` (`<word> version <rest>`) verbatim — the standard codifies the existing probe, not a redesign.
- [x] A-013 R2: The `update.md` `--skip-brew-update` rule names the literal substring exactly as shll's `skipBrewUpdateFlag` constant and documents substring (not regex) discovery.

### Edge Cases & Error Handling

- [x] A-014 R7: `shell-init.md` makes explicit that a tool exiting 0 while printing non-shell content to stdout is the poisoning failure mode the exit-code-gated drop cannot catch.
- [x] A-015 R2: `update.md`'s brew-safety clause cites the concrete 2026-07-19 failure (stalled GitHub-API call inside `brew upgrade` exceeding a 120s hard kill, corrupting the keg mid-swap).

### Code Quality

- [x] A-016 Pattern consistency: The three new pages follow the existing standards' structure (single `#` H1, implements-principle line, rules-with-teeth, "Verifying conformance"); the roster entries follow the `standard` struct field pattern and roster ordering.
- [x] A-017 No unnecessary duplication: The roster additions reuse the existing struct/table/JSON/drift-guard machinery (no parallel renderer, no new test scaffolding); the sync array reuses the existing loop.
- [x] A-018 No shll behavior change: no source outside `standards.go`'s roster is modified; probes/timeouts/regexes stay byte-identical (intake §5); no new subcommand added.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- Docs-typed change: review's parsimony / deletion-candidate pass is skipped for `change_type: docs`; no `## Deletion Candidates` section is expected.

## Assumptions

<!-- Graded SRAD decisions made while co-generating ## Requirements. Three grades
     only (Certain/Confident/Tentative). Scores column required. -->

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Author all three new pages in the register of `help-dump.md`/`readme-extraction.md` (single `#` H1 `Standard: <name>`, implements-principle line, rules-with-teeth, Verifying-conformance section) | Existing standards establish an unambiguous house style; matching it is the low-risk default, fully answerable from the two sibling docs | S:85 R:85 A:95 D:90 |
| 2 | Certain | Append the three roster entries after `skill` (roster order = output order) at `Scope: "binary"` | Intake §4 + assumption #5 fix the scope; appending preserves existing indices so index-paired tests move in lockstep | S:90 R:90 A:95 D:90 |
| 3 | Confident | Principle mapping: update→№7 (+brew-safety under №8 graceful degradation), version→№4 (+№2), shell-init→№2 | Intake defers exact №s to authoring; chosen from the ten principles by closest obligation match; low-risk editorial, easily revised | S:60 R:85 A:75 D:65 |
| 4 | Confident | Fold naming/release-alignment into `update.md` rather than minting a fourth standard | Intake assumption #4 (Confident); update owns the brew/formula surface; relocatable later | S:65 R:85 A:80 D:60 |
| 5 | Confident | `update.md` includes the brew-safety MUST-NOT (no SIGKILL / no short hard timeout on brew), phrased with SIGTERM+grace as the graceful alternative | Intake assumption #8 (Confident); motivated by the diagnosed 2026-07-19 incident; exact RFC-2119 phrasing chosen at authoring | S:70 R:80 A:80 D:70 |
| 6 | Confident | One-line roster `Description`s for the three new entries authored to match the terse glossary voice of the existing four descriptions | The existing descriptions establish the length/voice; only the wording is open, and it is trivially editable | S:70 R:90 A:80 D:75 |
| 7 | Certain | No source outside `standardsRoster` and the sync array changes; probes/timeouts/regexes untouched | Intake §5 is binding — the standards codify existing behavior; verified the roster-driven tests need no edit | S:90 R:80 A:95 D:90 |

7 assumptions (3 certain, 4 confident).
