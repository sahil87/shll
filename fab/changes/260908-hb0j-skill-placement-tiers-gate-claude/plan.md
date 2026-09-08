# Plan: Codify Two-Tier Skill Placement Taxonomy and Gate ~/.claude on the claude CLI

**Change**: 260908-hb0j-skill-placement-tiers-gate-claude
**Intake**: `intake.md`

## Requirements

### Standards: Placement directories section

#### R1: The skill standard codifies the two-tier placement taxonomy
`docs/site/standards/skill.md` SHALL gain a "Placement directories" section stating the taxonomy as a RULE (not a snapshot folder list): the **unconditional tier** is the portable Agent Skills open-standard directory (`.agents/skills/`) — always deployed, guaranteed present, the canonical harness-neutral read channel; the **gated tier** is brand-specific surfaces, each deployed only when its brand CLI is on PATH (`.claude/skills/` on `claude`, `.opencode/commands/` on `opencode`). The section MUST cover both scopes — repo-level deployment (fab-kit's `fab sync`) and global/machine-level placement (`shll setup agent`) — and record the rationale: de-branded internals, no littering non-brand machines/repos, taxonomy purity, the `.claude` retirement stepping stone, the "if you run Claude Code, `claude` is on PATH" gate-fit argument, the verified fact that Claude Code does not read `.agents/skills/` (why the gated `.claude` channel must exist), and fab-kit's one-target-per-skill-set invariant (per-brand copies for CLIs that already read `.agents/skills` produced duplicate-skill conflict warnings).

- **GIVEN** a reader of the published skill standard
- **WHEN** they consult it for where toolkit skills are placed
- **THEN** they find the two-tier rule covering both scopes, with the rationale above, in wording consistent with the other standards pages

#### R2: The standard's two stale unconditional-pair passages are updated
The two passages currently describing the global pair as unconditional SHALL be updated to the gated contract: § "Landed design: `shll setup agent`" (currently "places … into the harnesses' global skills directories (`~/.agents/skills/` and `~/.claude/skills/`)") and § "The placed skill conforms to the Agent Skills spec" (currently "places into harness-owned skills directories (`~/.agents/skills/`, `~/.claude/skills/`)"). Both become: placed into `~/.agents/skills/` always, and into `~/.claude/skills/` when the `claude` CLI is on PATH. The conformance rules themselves are unchanged.

- **GIVEN** the updated standard
- **WHEN** grepping it for placement descriptions
- **THEN** no passage claims the global pair is written unconditionally

#### R3: The standards embed stays byte-identical
After editing `docs/site/standards/skill.md`, the committed embedded copy (`src/cmd/shll/standards/skill.md`) MUST be re-synced via `scripts/sync-standards.sh` so `TestStandardsEmbedMatchesCanonical` stays green.

- **GIVEN** the edited canonical standard
- **WHEN** `scripts/sync-standards.sh` runs and `go test` executes the drift guard
- **THEN** the embedded copy is byte-identical and the test passes

### CLI: Gate the global ~/.claude write

#### R4: `~/.claude/skills/` write gated on `claude`; `~/.agents/skills/` unconditional
`shll setup agent`'s install path SHALL write `~/.claude/skills/shll-toolkit/SKILL.md` only when `claude` is on PATH; `~/.agents/skills/shll-toolkit/SKILL.md` stays unconditional. On machines without `claude`, `shll setup agent` MUST NOT create `~/.claude/` at all. The gate never deletes: an existing `~/.claude/` placement is never removed by the gate — only `--uninstall` deletes. Bare `shll setup` and `shll install`'s post-install auto-run inherit the gate by construction (both call the `runAgentSetup` seam).

- **GIVEN** a machine where `claude` is NOT on PATH and `$HOME` has no `.claude/` dir
- **WHEN** `shll setup agent` runs
- **THEN** only `~/.agents/skills/shll-toolkit/SKILL.md` is written and no `~/.claude/` directory exists afterward
- **GIVEN** a machine where `claude` IS on PATH
- **WHEN** `shll setup agent` runs
- **THEN** both paths are written exactly as today

#### R5: PATH lookup behind an injectable, proc-routed seam
The gate check SHALL be a pure PATH presence lookup (no subprocess spawned) routed so command code stays `os/exec`-free — a small helper in `internal/proc` (e.g. `proc.LookPath`) or an injectable package-level func var in `agent_setup.go`, with a test seam allowing the gate to be exercised without a real `claude` on PATH. Exact micro-placement is apply's call (intake Assumption #6).

- **GIVEN** the shipped gate implementation
- **WHEN** inspecting `src/cmd/shll/agent_setup.go`
- **THEN** no direct `os/exec` usage appears in command code, and tests can force the gate open or closed

#### R6: Uninstall and the placement/staleness probe keep covering BOTH paths
`--uninstall` (`runAgentUninstall`) SHALL keep removing both skill directories regardless of gate state, and `agentSkillPlacementState` (consumed by `shll update`'s conditional refresh and `shll doctor`) SHALL keep checking both paths regardless of gate state — an existing `~/.claude` placement still refreshes, reports staleness, and removes cleanly on a machine where `claude` has since disappeared. The post-hoc pickup property holds: a user who installs Claude Code after setup gets `~/.claude/skills/` on their next `shll update` (the refresh re-runs `shll setup agent` as a subprocess and the gate is then open).

- **GIVEN** a stale pre-existing `~/.claude/skills/shll-toolkit/SKILL.md` on a machine without `claude`
- **WHEN** `agentSkillPlacementState` runs
- **THEN** it reports `placed` and `stale`
- **GIVEN** the same machine
- **WHEN** `shll setup agent --uninstall` runs
- **THEN** both skill directories are removed

#### R7: `--print` reflects the gate
`--print` SHALL list only the target paths a real run would write: on a no-`claude` machine, only the `~/.agents/skills/` path (intake Assumption #7 — dry-run truthfulness).

- **GIVEN** a machine where `claude` is NOT on PATH
- **WHEN** `shll setup agent --print` runs
- **THEN** the `Target paths:` block lists only the `~/.agents/skills/…` path, and nothing is written

### Docs: present-truth sweep (repo docs; memory is hydrate's)

#### R8: Repo-doc carriers of the unconditional-pair claim updated to the gated contract
The following SHALL be updated to gated wording, with their embeds re-synced where applicable: `docs/site/skill.md` line ~28 (the setup-agent capability line "place … at two global skill paths" — this file is embedded as `src/cmd/shll/skill/skill.md` with drift guard `TestSkillEmbedMatchesCanonical` and a ≤150-line budget), `docs/site/install.md` line ~71 ("placed at the two global skill paths"), and `README.md`'s install/auto-run and `shll setup agent` sections where they state the two-path placement. Wording class discipline: these are deploy-target descriptions — they stay naming both dirs but describe the gated contract ("`~/.claude/skills/` when `claude` is present"); historical artifacts (change logs, findings) are never touched.

- **GIVEN** the swept repo docs
- **WHEN** grepping `docs/site/` and `README.md` for the placement description
- **THEN** every present-truth claim matches the gated contract, `TestSkillEmbedMatchesCanonical` passes, and `docs/site/skill.md` stays ≤150 lines

### Non-Goals

- No changes to the placed SKILL.md content (`agentSkillContent`, description builder, vocabulary contracts).
- No changes to the run-kit delegation or the `--yes` consent chain.
- No changes to uninstall semantics beyond keeping both paths covered.
- No `.opencode` surface at global scope (shll places nothing there; the standard's `.opencode` row describes fab-kit's repo-level target only).
- No new subcommands (Constitution VII untriggered).
- `docs/memory/` updates are NOT apply tasks — hydrate owns them (intake § Affected Memory).

### Design Decisions

#### Two target lists: all-candidates vs gated-install
**Decision**: Split the target model into "all candidate paths" (both dirs — used by `--uninstall` and `agentSkillPlacementState`) and "install targets" (the gated subset — used by `runAgentInstall` and `--print`).
**Why**: The intake mandates asymmetric coverage (gate the write; keep probe/uninstall on both). Two derivations from one shared `skillTargetRelDirs`-style source keep the paths single-sourced while making the asymmetry explicit at each call site.
**Rejected**: Threading a `gated bool` per path through one list (hides the asymmetry, easy to misuse at a future call site).
*Introduced by*: 260908-hb0j-skill-placement-tiers-gate-claude

#### PATH lookup as a proc helper with a swappable seam
**Decision**: Add a tiny `LookPath`-style helper to `internal/proc` (wrapping `exec.LookPath`), consumed via an injectable seam so `agent_setup_test.go` forces gate state without touching PATH.
**Why**: Keeps command code `os/exec`-free (code-review rule), matches fab-kit's `agentAvailable` precedent, and `internal/proc` is already the sanctioned home for process-adjacent primitives.
**Rejected**: `exec.LookPath` directly in `agent_setup.go` (violates the review rule's spirit); a full capability-probe subprocess (spawns a process for a presence question — Constitution I ceremony for nothing).
*Introduced by*: 260908-hb0j-skill-placement-tiers-gate-claude

## Tasks

### Phase 1: Setup

- [x] T001 Add a pure PATH-presence helper to `src/internal/proc/proc.go` (e.g. `LookPath(name string) bool` wrapping `exec.LookPath`, with whatever seam shape the consumer injection needs) + unit test in `src/internal/proc/proc_test.go` <!-- R5 -->

### Phase 2: Core Implementation

- [x] T002 Gate the install path in `src/cmd/shll/agent_setup.go`: restructure `skillTargetRelDirs`/`resolveSkillTargets` into the two-list model (all-candidates vs gated-install per the Design Decision), `runAgentInstall` writes only gated targets via an injectable `claude`-lookup seam; gate never deletes <!-- R4 -->
- [x] T003 `runAgentPrint` consumes the gated install-target list so `--print` shows only paths a real run would write <!-- R7 -->
- [x] T004 `runAgentUninstall` and `agentSkillPlacementState` iterate the all-candidates list (both paths) regardless of gate state <!-- R6 -->
- [x] T005 Update `src/cmd/shll/agent_setup_test.go`: gate-open → both written; gate-closed → only `~/.agents/skills/` written AND no `~/.claude/` dir created (assert dir absence); `--uninstall` removes both under a closed gate; `agentSkillPlacementState` reports placed+stale for a pre-existing `~/.claude` copy under a closed gate; `--print` output matches gate state; adjust existing placement tests to run under an open gate <!-- R4 -->

### Phase 3: Standards & Embeds

- [x] T006 Add the "Placement directories" section to `docs/site/standards/skill.md` (two-tier rule, both scopes, full rationale incl. one-target-per-skill-set invariant) and update the two stale passages (§ Landed design, § The placed skill conforms) to the gated contract <!-- R1 -->
- [x] T007 Run `scripts/sync-standards.sh` to refresh `src/cmd/shll/standards/skill.md`; verify `TestStandardsEmbedMatchesCanonical` passes <!-- R3 -->

### Phase 4: Docs Sweep & Verification

- [x] T008 Gated-contract wording sweep: `docs/site/skill.md` (~line 28, keep ≤150 lines, re-sync embed `src/cmd/shll/skill/skill.md` so `TestSkillEmbedMatchesCanonical` passes), `docs/site/install.md` (~line 71), `README.md` install/auto-run + `shll setup agent` sections <!-- R8 -->
- [x] T009 Full verification: `cd src && gofmt -l . && go test ./...` — all green <!-- R4 -->

## Execution Order

- T001 blocks T002 (the seam T002 injects)
- T002 blocks T003, T004, T005
- T006 blocks T007
- T008 depends on T006's final wording for consistency; T009 last

## Acceptance

### Functional Completeness

- [x] A-001 R1: `docs/site/standards/skill.md` carries a "Placement directories" section stating the two-tier rule for both scopes with the full recorded rationale (including the one-target-per-skill-set invariant and the Claude-Code-doesn't-read-`.agents` fact)
- [x] A-002 R2: Neither stale passage still describes the global pair as unconditional; both state `~/.agents/skills/` always + `~/.claude/skills/` when `claude` is on PATH
- [x] A-003 R4: With `claude` absent, `shll setup agent` writes only `~/.agents/skills/shll-toolkit/SKILL.md` and creates no `~/.claude/` directory; with `claude` present, both paths are written
- [x] A-004 R6: `--uninstall` removes both skill dirs and `agentSkillPlacementState` covers both paths, in both gate states
- [x] A-005 R7: `--print` lists only the paths a real run would write for the current gate state, and modifies nothing

### Behavioral Correctness

- [x] A-006 R4: The gate never deletes — a pre-existing `~/.claude/skills/shll-toolkit/` survives an install run on a no-`claude` machine untouched
- [x] A-007 R5: No direct `os/exec` usage in `src/cmd/shll/agent_setup.go`; the lookup rides the proc helper / injectable seam and spawns no subprocess

### Scenario Coverage

- [x] A-008 R4: Test asserts `~/.claude/` **directory absence** (not just file absence) after a gate-closed install into a clean `$HOME`
- [x] A-009 R6: Test covers the stale pre-existing `~/.claude` copy on a no-`claude` machine reporting placed+stale
- [x] A-010 R3: `TestStandardsEmbedMatchesCanonical` passes after the standards edit + re-sync

### Edge Cases & Error Handling

- [x] A-011 R4: Empty `$HOME` still yields no targets (existing behavior preserved); gate logic introduces no new error paths that could fail a placement that previously succeeded

### Code Quality

- [x] A-012 Pattern consistency: gate seam and test injection follow the file's existing seam style (env func, fake `proc.Runner`); named constants per code-quality.md (no magic strings)
- [x] A-013 No unnecessary duplication: path derivation stays single-sourced; the two lists derive from one relative-dir source
- [x] A-014 Constitution V: a missing `claude` is a silent skip of that surface — no warning, no error, no output noise beyond the per-path summary
- [x] A-015 R8: `docs/site/skill.md` remains ≤150 lines and `TestSkillEmbedMatchesCanonical` passes after the sweep

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before hydrate
- If an item is not applicable, mark checked and prefix with **N/A**

## Deletion Candidates

None — this change adds the two-tier gate and refactors the single target list into the two-list model in place (`skillTargetsUnder` shares the derivation); no existing file, function, branch, or config became redundant or unused.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Two-list target model (all-candidates vs gated-install) as the restructure shape | Cleanest expression of the intake's asymmetric coverage mandate; exact naming left to apply | S:70 R:90 A:85 D:70 |
| 2 | Confident | Repo-doc sweep includes `docs/site/skill.md`, `docs/site/install.md`, and README (grep-verified carriers found at plan time, beyond the intake's memory-only sweep list); memory files stay hydrate-owned | Intake's Impact named docs/site/standards only, but the review's holistic pass would flag these stale claims; adding them at apply avoids a rework cycle | S:75 R:85 A:85 D:80 |
| 3 | Certain | `--print` shows gate-state-dependent paths | Intake Assumption #7, delegated and decided there | S:90 R:90 A:90 D:85 |

3 assumptions (1 certain, 2 confident, 0 tentative).
