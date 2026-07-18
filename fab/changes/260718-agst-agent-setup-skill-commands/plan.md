# Plan: Add `shll agent-setup` + `shll skill`

**Change**: 260718-agst-agent-setup-skill-commands
**Intake**: `intake.md`

## Requirements

<!-- Derived from the CURRENT intake (§1–§7). agent-setup is the skills-placement
     design (§4); the backlog's stanza mechanism is REJECTED (intake "Out of scope").
     Some requirements re-verify still-valid prior work (proc transport, skill
     composer, root wiring, install/README graduation) against the current design. -->

### CLI: `shll skill` composer

#### R1: Bare `shll skill` glossary
`shll skill` with no arguments SHALL print a one-line-per-installed-tool glossary — shll first (using `shllSelf`), then the `Roster` tools in declared leaves-first order, each with its hardcoded `Description` one-liner, column-aligned like `version`/`list`. It MUST NOT concatenate per-tool bundles (the two-step context-economy contract). Installed-only: a tool not on PATH is skipped silently via the shared `toolInstalled` probe (no brew calls). A trailing hint line teaches the second step.

- **GIVEN** shll plus `wt` and `hop` are on PATH and the other four roster tools are not
- **WHEN** the user runs `shll skill`
- **THEN** stdout lists the shll row first, then `wt`, then `hop` (roster order), each with its description, followed by a blank line and the `Run 'shll skill <tool>' …` hint
- **AND** no roster tool absent from PATH appears, no `brew` subprocess is invoked, and no bundle H1 (`# … skill`) is printed

#### R2: `shll skill <tool>` per-tool passthrough
`shll skill <tool>` SHALL resolve `<tool>` against the Roster (inheriting the `rk` → `run-kit` legacy alias via the shared resolver) plus the `shll` self-token. For a Roster tool it MUST invoke `<tool> skill` as a bounded subprocess through `internal/proc` and stream its stdout **byte-identical** on success (no framing, no rendering).

- **GIVEN** `hop` is installed and supports `skill`
- **WHEN** the user runs `shll skill hop`
- **THEN** stdout is byte-identical to `hop skill`'s stdout, stderr is empty, exit 0
- **AND** the invocation uses the capture-all transport; `shll skill rk` resolves to `run-kit skill` (never a literal `rk skill`)

#### R3: `shll skill shll` self-bundle
`shll skill shll` SHALL serve shll's own bundle from the embedded copy **in-process** (a subprocess self-invocation would recurse into the composer), byte-identical to `docs/site/skill.md`.

- **GIVEN** shll is the running binary
- **WHEN** the user runs `shll skill shll`
- **THEN** stdout is byte-identical to the embedded `skill/skill.md` copy, no subprocess is spawned, stderr empty, exit 0

#### R4: `shll skill` degradation and miss behavior
For `shll skill <tool>`, a tool missing from PATH (`proc.ErrNotFound`) or whose installed version predates `skill` (subprocess exits non-zero) MUST get a single clear stderr notice and **exit 1** (operational), with the child's own stderr suppressed. An unknown tool name MUST be a usage error with an actionable diagnostic listing valid targets and **exit 2**.

- **GIVEN** `idea` is not on PATH
- **WHEN** the user runs `shll skill idea`
- **THEN** exactly one stderr line notes it is not installed, stdout is empty, exit 1
- **AND** `shll skill wombat` (unknown) writes a usage diagnostic naming `wombat` and the valid targets, makes no subprocess call, exit 2
- **AND** `shll skill wt` where `wt` predates `skill` writes one unsupported notice, suppresses `wt`'s raw stderr, exit 1

#### R5: shll's own bundle authored, embedded, drift-guarded, budget-bounded
shll's own bundle SHALL be authored at `docs/site/skill.md` as a ≤150-line static usage briefing per the `skill` standard, embedded via the established sync + drift-guard mechanism (committed `src/cmd/shll/skill/skill.md`, `scripts/sync-standards.sh` extension, drift-guard test), and MUST accurately describe the CURRENT commands — including `shll agent-setup` as **skills placement**, never stanza injection.

- **GIVEN** `docs/site/skill.md` is the canonical source
- **WHEN** `go test` runs the drift guard and budget test
- **THEN** the embedded copy is byte-identical to canonical and the bundle is ≤150 lines
- **AND** the `shll agent-setup` line in the bundle describes placing the toolkit skill (not writing a context stanza)

### CLI: `shll agent-setup` (mechanical skill placement)

#### R6: Two-location unconditional placement
`shll agent-setup` (default run) SHALL mechanically place ONE thin Agent Skill named `sahil87-toolkit` at exactly TWO unconditional global locations: `~/.agents/skills/sahil87-toolkit/SKILL.md` (agentskills.io path — Codex USER scope, Cursor + OpenCode compat) and `~/.claude/skills/sahil87-toolkit/SKILL.md` (Claude Code). Install = write, re-run = overwrite (idempotent). There MUST be NO harness detection, NO sentinel machinery, NO diff-and-confirm, NO `--yes` gate, NO non-TTY refusal. Both parent directories are created as needed (shll owns these skill dirs).

- **GIVEN** a clean `$HOME`
- **WHEN** the user runs `shll agent-setup`
- **THEN** both `SKILL.md` files exist with identical canonical content, and a per-path written/updated/unchanged summary is printed
- **AND** re-running is idempotent (byte-identical files) and reports each path as unchanged; no confirmation prompt is shown and a non-interactive stdin does not cause a refusal

#### R7: Canonical SKILL.md content (portable frontmatter + two-step body)
The placed `SKILL.md` SHALL be a Go string constant in `agent_setup.go` with portable frontmatter containing `name` + `description` ONLY (OpenCode's recognized subset). `name` MUST be `sahil87-toolkit`, matching `^[a-z0-9]+(-[a-z0-9]+)*$` and equalling its directory name. The `description` MUST front-load trigger words (toolkit tool names) for implicit activation. The body MUST teach the two-step (`shll skill`, then `shll skill <tool>`) plus one trailing `shll standards` pointer.

- **GIVEN** the canonical content constant
- **WHEN** the skill is placed or `--print` is run
- **THEN** the frontmatter carries exactly `name: sahil87-toolkit` and a `description:`, no other keys; the body teaches `shll skill` / `shll skill <tool>` and points at `shll standards`
- **AND** `name` equals the directory name `sahil87-toolkit` and satisfies the portable-name regex

#### R8: `--print` and `--uninstall` modes
`shll agent-setup --print` SHALL show the SKILL.md content and the two target paths without writing any file and without triggering run-kit delegation. `shll agent-setup --uninstall` SHALL delete both `sahil87-toolkit` skill directories and MUST NOT trigger run-kit delegation side effects beyond the equivalent run-kit uninstall pass-through. `--print` and `--uninstall` together SHALL be a usage error (exit 2).

- **GIVEN** the skill is placed
- **WHEN** the user runs `shll agent-setup --print`
- **THEN** stdout shows the content and both target paths, no file is written or removed, and no run-kit delegation runs
- **AND** `shll agent-setup --uninstall` removes both skill directories; `--print --uninstall` exits 2

#### R9: run-kit delegation
After a default-run placement, `shll agent-setup` SHALL invoke `run-kit agent-setup` as a subprocess through `internal/proc` for run-kit's harness hooks, skipping silently when run-kit is absent (`proc.ErrNotFound`). `--print` MUST NOT delegate; `--uninstall` SHALL delegate the equivalent `run-kit agent-setup --uninstall`.

- **GIVEN** run-kit is installed
- **WHEN** the user runs `shll agent-setup`
- **THEN** a `run-kit agent-setup` subprocess is invoked after placement
- **AND** when run-kit is absent the delegation is skipped with no error surfaced; `--print` triggers no delegation; `--uninstall` delegates `run-kit agent-setup --uninstall`

### CLI: wiring and touchpoint graduation

#### R10: Root command wiring
Both `shll skill` and `shll agent-setup` SHALL be registered on the root cobra command and appear in the root long-help subcommand list, with short descriptions matching the skills-placement design (agent-setup wording MUST NOT describe stanza injection).

- **GIVEN** the root command
- **WHEN** `shll --help` and `shll help-dump` run
- **THEN** both subcommands are present in the tree, and the agent-setup short/long help describes placing the toolkit skill

#### R11: `install.go` post-install nudge graduation
The post-install "Next steps" nudge SHALL point at `shll agent-setup` instead of `run-kit agent-setup`, reframed to describe wiring agent harnesses (toolkit skill placement + run-kit dashboard hooks). Because shll is by definition present, the agent-setup line SHALL print unconditionally on the outcome paths (no run-kit presence gate); the shell-setup line's gate is unchanged. The wording MUST NOT describe stanza injection.

- **GIVEN** an all-installed / partial-install outcome path (not dry-run, not brew-missing)
- **WHEN** `shll install` runs
- **THEN** the "Next steps" block prints the `shll agent-setup` line unconditionally, marked "optional, once per machine", with wiring-agent-harnesses wording
- **AND** `--dry-run` prints no nudge; the shell-setup line still gates on the unwired-rc fact

#### R12: README install-flow graduation
The README install flow command block and its explanation paragraph SHALL graduate `run-kit agent-setup` to `shll agent-setup`, and the new `shll skill` / `shll agent-setup` command sections MUST describe the CURRENT design (skills placement, two-step glossary) — NO stanza-injection wording anywhere.

- **GIVEN** the README
- **WHEN** a reader reads the install flow and the command sections
- **THEN** the flow uses `shll agent-setup`, the agent-setup section describes placing the `sahil87-toolkit` skill at the two global skill paths and delegating run-kit hooks, and no "context stanza"/"sentinel"/"AGENTS.md-family" stanza wording remains

### internal/proc: capture-all transport

#### R13: `RunCaptured` / `TransportCaptureAll`
`internal/proc` SHALL provide a `TransportCaptureAll` transport and a `RunCaptured` helper that buffer BOTH stdout and stderr into the `Result` (separate fields) and report the child's exit code, passing neither stream through to the parent — so a caller can stream captured stdout byte-identical on success and suppress captured stderr on failure. `ErrNotFound` yields code −1; a non-zero child exit yields `err == nil` with the code surfaced.

- **GIVEN** a child that writes to both streams and exits non-zero
- **WHEN** `RunCaptured` invokes it
- **THEN** both buffers are populated, the exit code is surfaced with `err == nil`, and neither stream reaches the parent's stdio
- **AND** a not-on-PATH binary returns `ErrNotFound` with code −1

### Constitution conformance

#### R14: Constitution and standards fit
All new subprocess work (`<tool> skill`, `run-kit agent-setup`) MUST route through `internal/proc` (Constitution I); skill placement is plain file I/O in shll-owned directories. Both new subcommands MUST carry a Constitution VII justification (in the intake). The change MUST NOT reintroduce the rejected stanza machinery: `shell_setup.go` stays at HEAD and no `sentinel_block.go` exists.

- **GIVEN** the implemented change
- **WHEN** the source is inspected
- **THEN** no command code calls `os/exec` directly, `shell_setup.go` is unmodified from HEAD, `sentinel_block.go` does not exist, and no stanza/sentinel machinery remains for agent-setup

### Non-Goals

- The other six tools' `<tool> skill` implementations (ride the per-repo standards waves).
- The run-kit slimming change (external run-kit repo).
- fab-kit's `_cli-external.md` slimming (fab-kit backlog).
- Any change to the `skill` standard's text (a likely small follow-up, not this change).
- Stanza injection into `~/.claude/CLAUDE.md` / AGENTS.md-family files (REJECTED in favor of skills placement).

### Design Decisions

1. **agent-setup = mechanical skill placement, not stanza injection**: install = write, re-run = overwrite, `--uninstall` = delete two dirs — *Why*: idempotent by construction, no merge machinery, no edits to user-owned files, cheaper per-session context; *Rejected*: sentinel-wrapped context stanza in harness context files (the backlog's original mechanism) — user chose skills placement explicitly this session.
2. **Exactly two unconditional writes cover all four harnesses**: `~/.agents/skills/` (Codex USER + Cursor/OpenCode compat) + `~/.claude/skills/` (Claude Code) — *Why*: verified coverage matrix; unconditional is simplest and future-proof (new standard-adopting harnesses ride `~/.agents`); *Rejected*: harness detection + skip logic, and a skip-a-harness degradation (user: "no degeneration").
3. **Canonical SKILL.md is a Go string constant in `agent_setup.go`**: *Why*: the bootstrap skill is an agent-setup artifact, not a published document or a `<tool> skill` bundle, so the docs-site sync/drift-guard ceremony does not apply; *Rejected*: a `docs/site` canonical file with sync/embed.
4. **Portable frontmatter `name` + `description` only**: the OpenCode-recognized common subset, valid on all four harnesses — *Why*: maximizes cross-harness compatibility; the description front-loads trigger words for implicit activation.
5. **Keep the still-valid prior work, re-verified against the current design**: the `internal/proc` capture transport, `skill.go`/`skill_test.go` composer, `docs/site/skill.md` bundle + embed + sync + drift-guard, root wiring, and the install/README graduation are correct under the current design and are kept — but each is re-checked (e.g. wording that described stanza injection is corrected) before being marked complete.

## Tasks

### Phase 1: Setup

- [x] T001 Revert `src/cmd/shll/shell_setup.go` to HEAD (`git checkout --`) and delete `src/cmd/shll/sentinel_block.go` — the rejected stanza mechanism's shared engine and shell-setup edits fall away with the stanza design <!-- R14 -->
- [x] T002 Verify the kept `internal/proc` capture-all transport (`TransportCaptureAll`, `RunCaptured`, `Result.Stderr`) in `src/internal/proc/proc.go` and its tests in `src/internal/proc/proc_test.go` against R13 — confirm streams are captured (not passed through), non-zero exit yields `err == nil`, `ErrNotFound` yields code −1 <!-- R13 -->

### Phase 2: Core Implementation

- [x] T003 Rewrite `src/cmd/shll/agent_setup.go` to the mechanical skill-placement design: canonical `SKILL.md` Go constant (`name: sahil87-toolkit` + `description` only; two-step body + `shll standards` pointer), two-location placement (`~/.agents/skills/sahil87-toolkit/SKILL.md` + `~/.claude/skills/sahil87-toolkit/SKILL.md`), install/overwrite with per-path written/updated/unchanged summary, `--print` (content + paths, no write, no delegation), `--uninstall` (delete both dirs), `--print --uninstall` → exit 2, run-kit delegation via `internal/proc` (skip silently when absent; not on `--print`). Remove all sentinel/stanza/diff-and-confirm/`--yes`/non-TTY-refusal machinery <!-- R6 --> <!-- R7 --> <!-- R8 --> <!-- R9 -->
- [x] T004 Rewrite `src/cmd/shll/agent_setup_test.go` to the placement design: assert both files written with canonical content, idempotent re-run (unchanged summary), `--print` writes nothing and does not delegate, `--uninstall` removes both dirs and delegates uninstall, `--print --uninstall` exits 2, run-kit delegation present-when-installed / silent-when-absent, canonical content has portable frontmatter (`name` + `description` only) and `name == sahil87-toolkit == dir name`. Drop the stanza/TTY/`--yes` tests <!-- R6 --> <!-- R7 --> <!-- R8 --> <!-- R9 -->
- [x] T005 [P] Verify the kept `src/cmd/shll/skill.go` composer + `src/cmd/shll/skill_test.go` against R1–R4: bare glossary (installed-only, shll-first, no brew, hint, no bundle concat), byte-identical `<tool> skill` passthrough via `RunCaptured`, `rk`→`run-kit` alias, `shll skill shll` in-process embed, not-installed/unsupported → one-line stderr + exit 1, unknown name → exit 2 <!-- R1 --> <!-- R2 --> <!-- R3 --> <!-- R4 -->

### Phase 3: Integration & Wiring

- [x] T006 Verify root wiring in `src/cmd/shll/root.go`: `newSkillCmd()` + `newAgentSetupCmd()` registered and listed in the long-help subcommand block; confirm the agent-setup short/long help (in `agent_setup.go`) describes skill placement, not stanza injection <!-- R10 -->
- [x] T007 Verify the `install.go` nudge graduation in `src/cmd/shll/install.go` + `src/cmd/shll/install_test.go` against R11: the "Next steps" block points at `shll agent-setup` (unconditional, reframed wording — toolkit skill + run-kit hooks), no run-kit presence gate on the agent line, shell-setup gate unchanged, no nudge on dry-run, no stanza wording <!-- R11 -->
- [x] T008 Correct and verify shll's own bundle `docs/site/skill.md` against R5/R12: rewrite the `shll agent-setup` capabilities line to describe placing the `sahil87-toolkit` skill (NOT "wire … with a toolkit-context stanza"); re-run `scripts/sync-standards.sh` to refresh the embedded `src/cmd/shll/skill/skill.md`; confirm ≤150 lines <!-- R5 --> <!-- R12 -->
- [x] T009 Rewrite the README `shll agent-setup` install-flow paragraph and the `### shll agent-setup` command section in `README.md` to the skills-placement design (place the `sahil87-toolkit` skill at the two global skill paths, `--print`/`--uninstall`, delegate run-kit hooks); remove all stanza/sentinel/AGENTS.md-family stanza wording; verify the `### shll skill` section and the "How composition works" table rows match the current design <!-- R12 --> <!-- R10 -->
- [x] T010 Verify `scripts/sync-standards.sh` extension syncs `docs/site/skill.md` → `src/cmd/shll/skill/skill.md` and that the drift-guard + budget tests in `skill_test.go` pass <!-- R5 -->

### Phase 4: Validation

- [x] T011 Build (`cd src && go build ./...`) and run the full `cd src && go test ./...`; confirm the constitution/standards guards hold (`TestNoProcImports` on the reverted `shell_setup.go`, help-dump tree self-check picks up the two new commands, drift guards green) <!-- R14 --> <!-- R10 -->

## Execution Order

- T001 and T002 (Phase 1) precede everything (T001 removes the rejected mechanism so the tree compiles under the new design; T002 confirms the transport T003/T005 depend on).
- T003 must precede T004 (tests target the rewritten implementation).
- T005 is independent of T003/T004 (different files) and may run alongside.
- T008 must precede T010 (sync refreshes the embed the drift guard checks); T008 must precede T009 only for content consistency (both are wording, no hard code dependency).
- T011 runs last (whole-tree build + test).

## Acceptance

### Functional Completeness

- [x] A-001 R1: Bare `shll skill` prints the installed-only, shll-first, roster-ordered glossary with descriptions and the trailing hint; no brew call; no bundle concatenation
- [x] A-002 R2: `shll skill <tool>` streams `<tool> skill` byte-identical via the capture-all transport; the `rk` alias resolves to `run-kit`
- [x] A-003 R3: `shll skill shll` serves the embedded bundle in-process, byte-identical, with no subprocess
- [x] A-004 R4: `shll skill` miss behavior is correct — not-installed/unsupported → one-line stderr + exit 1 (child stderr suppressed); unknown name → usage diagnostic + exit 2, no subprocess
- [x] A-005 R5: `docs/site/skill.md` exists (≤150 lines), is embedded + drift-guarded, and describes `shll agent-setup` as skill placement (not a stanza)
- [x] A-006 R6: `shll agent-setup` writes both `sahil87-toolkit/SKILL.md` files with identical canonical content and prints a per-path written/updated/unchanged summary; re-run is idempotent; no confirmation prompt and no non-TTY refusal exist
- [x] A-007 R7: the canonical SKILL.md constant has portable frontmatter (`name` + `description` only), `name == sahil87-toolkit == directory name` matching the portable regex, and a body teaching the two-step + `shll standards`
- [x] A-008 R8: `--print` shows content + both paths without writing and without delegating; `--uninstall` removes both dirs; `--print --uninstall` exits 2
- [x] A-009 R9: default run delegates `run-kit agent-setup` via `internal/proc` (silent skip when absent); `--print` does not delegate; `--uninstall` delegates the uninstall pass-through
- [x] A-010 R10: `shll skill` and `shll agent-setup` are registered on root, listed in long-help, and present in `help-dump`; the agent-setup help describes skill placement
- [x] A-011 R11: the `install` post-install nudge points at `shll agent-setup` unconditionally with reframed wording; no dry-run nudge; shell-setup gate unchanged
- [x] A-012 R12: the README install flow uses `shll agent-setup`, its command sections describe the current skills-placement + two-step design, and no stanza-injection wording remains
- [x] A-013 R13: `internal/proc` exposes `TransportCaptureAll` + `RunCaptured` capturing both streams (not passed through), with correct `ErrNotFound`/non-zero-exit semantics

### Behavioral Correctness

- [x] A-014 R6: agent-setup's re-run overwrites idempotently (byte-identical files, unchanged summary) rather than merging or prompting
- [x] A-015 R11: the agent-setup nudge line no longer gates on run-kit presence (shll is always present) yet still prints on both the install-loop and short-circuit outcome paths, and not on dry-run

### Removal Verification

- [x] A-016 R14: `sentinel_block.go` is deleted, `shell_setup.go` is byte-identical to HEAD, and no stanza/sentinel/diff-and-confirm/`--yes`/non-TTY-refusal machinery remains in `agent_setup.go`

### Scenario Coverage

- [x] A-017 R2: a test exercises the byte-identical passthrough and asserts the capture-all transport is used
- [x] A-018 R6: a test exercises the clean-`$HOME` two-file placement and the idempotent re-run
- [x] A-019 R8: tests exercise `--print` (no write, no delegation), `--uninstall` (both dirs removed), and `--print --uninstall` (exit 2)

### Edge Cases & Error Handling

- [x] A-020 R4: `shll skill idea` (absent) and `shll skill wt` (unsupported) each emit exactly one stderr line and exit 1, suppressing the child's raw stderr
- [x] A-021 R9: run-kit absent during agent-setup is a silent skip (no delegation error on stderr), while the placement still succeeds

### Code Quality

- [x] A-022 Pattern consistency: new code follows the surrounding `cmd/shll` patterns — cobra factory + extracted run-seam for testing, named constants for user-facing strings and paths (no magic strings), `internal/proc` for all subprocess work
- [x] A-023 No unnecessary duplication: the skill composer reuses the shared `toolInstalled` probe, `legacyAliases`/`rosterTool` resolver, `shllSelf` descriptor, and `RunCaptured`; agent-setup reuses `runKitToolName` and the `errExitCode`/`errSilent`/`usageExitCode` sentinels rather than reimplementing them

### Security

- [x] A-024 R14: no command code invokes `os/exec` directly — every subprocess (`<tool> skill`, `run-kit agent-setup`) routes through `internal/proc` with `exec.CommandContext` (Constitution I)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Deletion Candidates

- `printNextSteps` `ctx context.Context` parameter (`src/cmd/shll/install.go:397`) — the run-kit presence probe it fed was removed with the unconditional nudge; the parameter is now unused at both call sites
- `delegateRunKitAgentSetup` `stdout io.Writer` parameter (`src/cmd/shll/agent_setup.go:285`) — never written to; only `stderr` is used
- `runKitToolName` doc comment (`src/cmd/shll/install.go:372-374`) — claims the constant "resolve[s] its Tool descriptor from the live Roster", but the roster resolution was deleted with the nudge gate; the sole remaining consumer is `agent_setup.go`'s delegation (binary name), so the comment is stale and the constant arguably belongs in `agent_setup.go`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | agent-setup = mechanical two-location skill placement; no stanza/sentinel/merge, no `--yes`/non-TTY refusal, no harness detection | Intake §4 + assumption 8/10 decide this verbatim (user chose "no merge operation, just mechanical placement") | S:95 R:80 A:90 D:95 |
| 2 | Certain | Placement paths `~/.agents/skills/sahil87-toolkit/SKILL.md` + `~/.claude/skills/sahil87-toolkit/SKILL.md`, both unconditional, dirs created as needed | Intake §4 + assumption 9/10; verified coverage matrix from harness docs (2026-07-18) | S:90 R:85 A:95 D:90 |
| 3 | Certain | Portable frontmatter `name` + `description` only; `name: sahil87-toolkit` equals dir name and matches `^[a-z0-9]+(-[a-z0-9]+)*$` | OpenCode-recognized subset; the shared regex/dir-match rule (intake §4 + assumption 11) | S:90 R:85 A:95 D:90 |
| 4 | Certain | Canonical SKILL.md is a Go string constant in `agent_setup.go` (no docs-site sync ceremony) | Intake §4 + assumption 11: the bootstrap skill is an agent-setup artifact, not a published doc | S:85 R:80 A:90 D:90 |
| 5 | Confident | `--uninstall` deletes both skill directories (not just the SKILL.md file), and delegates `run-kit agent-setup --uninstall` | Intake §4 "`--uninstall` = delete both skill directories"; symmetric with the write path; delegation mirrors install | S:70 R:80 A:75 D:65 |
| 6 | Confident | Per-path written/updated/unchanged summary distinguishes first-write vs overwrite-identical vs overwrite-changed by comparing existing bytes to canonical before writing | Intake §4 "per-path written/updated/unchanged summary on the default run"; the three states follow from stat+compare | S:65 R:80 A:75 D:60 |
| 7 | Confident | `--print` prints the canonical SKILL.md content plus the two absolute target paths (HOME-resolved) and triggers no run-kit delegation | Intake §4 "show content + target paths without writing"; `--print`/`--uninstall` "must not trigger delegation side effects" | S:70 R:80 A:80 D:70 |
| 8 | Confident | The kept prior work (proc transport, skill composer + tests, docs/site/skill.md embed/sync/drift-guard, root wiring, install/README graduation) is correct under the current design and is verified-then-kept, with only the stanza-wording in `docs/site/skill.md` and `README.md` corrected | Rework context: these pieces are design-neutral or already reframed to neutral wording; only agent-setup's mechanism and its prose need change | S:75 R:75 A:80 D:70 |
| 9 | Confident | `install.go`'s nudge line (already reworded to "wire agent harnesses (toolkit context + run-kit dashboard hooks)") is design-neutral and needs no further change beyond verification | The prior apply already graduated it to `shll agent-setup` with wording that fits skills placement; "toolkit context" is accurate for a skill that teaches toolkit context | S:70 R:80 A:75 D:65 |

9 assumptions (4 certain, 5 confident, 0 tentative).
