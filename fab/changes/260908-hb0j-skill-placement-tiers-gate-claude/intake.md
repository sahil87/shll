# Intake: Codify Two-Tier Skill Placement Taxonomy and Gate ~/.claude on the claude CLI

**Change**: 260908-hb0j-skill-placement-tiers-gate-claude
**Created**: 2026-09-08

## Origin

Promptless dispatch (`/fab-proceed` create-new, `{questioning-mode} = promptless-defer`) from a completed design conversation. Decisions below are final unless explicitly marked open.

> Codify the two-tier skill placement taxonomy in the skill standard, and gate shll's `~/.claude/skills/` placement on the `claude` CLI. Two parts, ONE shll change (user decided: "Do both"). Part 1: add a "Placement directories" section to `docs/site/standards/skill.md` codifying the toolkit's two-tier placement taxonomy as a RULE (unconditional tier = the portable Agent Skills open-standard directory `.agents/skills/`; gated tier = brand-specific surfaces, each deployed only when its brand CLI is on PATH), covering both repo-level deployment (fab-kit's `fab sync`) and global/machine-level placement (`shll setup agent`), and update the standard's two stale passages that describe the global pair as unconditional. Part 2: `shll setup agent`'s `~/.claude/skills/` write becomes gated on `claude` being on PATH; `~/.agents/skills/` stays unconditional; uninstall and the staleness/placement probe keep covering BOTH paths.

The taxonomy and its rationale come from fab-kit's change `260908-yd9s-repoint-agents-skills-gate-claude` (intake read at grounding time: `/home/sahil/code/sahil87/fab-kit.worktrees/rustic-tern/fab/changes/260908-yd9s-repoint-agents-skills-gate-claude/intake.md`). Key facts established there and in the design conversation:

- fab-kit PR #647 made both `.claude` and `.agents` repo-level deploy targets `AlwaysOn`; yd9s re-gates `.claude` on the `claude` CLI and keeps `.agents` AlwaysOn. The clean tiering: **unconditional = the cross-client standard directory; gated = brand surfaces** (`.claude/skills/` on `claude`, `.opencode/commands/` on `opencode`).
- Claude Code does NOT read `.agents/skills/` (verified in shll 2026-07-18; re-verified in yd9s against Claude Code 2.1.263 docs + a live probe), so `.claude/skills/` remains the Claude channel — but only on machines that have Claude.
- "If you run Claude Code, `claude` is on PATH" — the CLI gate is near-perfect for that brand.
- Rationale: de-brand internals, avoid littering non-Claude machines/repos with unread `.claude/` trees, taxonomy purity, and a stepping stone to retiring `.claude/skills/` if Claude Code ever adopts the open standard.
- fab-kit's **one-target-per-skill-set invariant**: deploying per-brand copies for CLIs that already read `.agents/skills` produced duplicate-skill conflict warnings — the standard should record this rationale.

Constitution § Toolkit Standards binds this repo to its own standards — here the standards edit IS the standard being changed, so no conflict; wording stays consistent with the other `docs/site/standards/` pages.

## Why

1. **The standard is stale and the taxonomy is unwritten.** `docs/site/standards/skill.md` § "Landed design: `shll setup agent`" and § "The placed skill conforms to the Agent Skills spec" describe the global pair (`~/.agents/skills/` + `~/.claude/skills/`) as an unconditional placement set. After yd9s, the toolkit's actual placement contract is two-tiered — unconditional open-standard dir, CLI-gated brand surfaces — but no standard records the rule. Without codification, each repo re-derives (or contradicts) the tiering ad hoc, and the next placement surface added anywhere in the toolkit has no rule to conform to.
2. **shll itself violates the tiering.** `shll setup agent` writes `~/.claude/skills/shll-toolkit/SKILL.md` unconditionally (`skillTargetRelDirs`, `src/cmd/shll/agent_setup.go` ~line 126), creating a `~/.claude/` tree on machines that will never run Claude Code — the same litter yd9s removes at repo scope. The manager CLI should model the standard it publishes (Constitution § Toolkit Standards).
3. **If not fixed**: the published standard and the toolkit's real behavior diverge (the standard says unconditional, fab-kit ships gated); non-Claude machines keep accumulating an unread `~/.claude/` tree on every `shll setup agent` / `shll update` refresh; and the eventual `.claude/skills/` retirement (if Claude Code adopts the open standard) has no recorded stepping stone at the global scope.
4. **Why one change for both parts**: the code change is the standard's first global-scope conformance instance — landing them together keeps the standard and shll's behavior atomically consistent (the user explicitly chose "Do both" in one shll change over splitting).

**Supersession note**: this reverses the explicit `agst` decision recorded in memory ("Both writes are unconditional … No harness detection, no skip logic, no skip-a-harness degradation") — deliberate: the yd9s tiering re-scopes it (unconditional only for the open-standard dir), mirroring how yd9s re-scoped fab-kit's #647 always-on-pair decision rather than deleting it.

## What Changes

### 1. Standards addition: "Placement directories" section (`docs/site/standards/skill.md`)

Add a **"Placement directories"** section codifying the two-tier skill placement taxonomy as a RULE, not a snapshot folder list:

- **Unconditional tier** — the portable Agent Skills open-standard directory (`.agents/skills/`): always deployed, guaranteed present, the canonical harness-neutral read channel.
- **Gated tier** — brand-specific surfaces, each deployed only when its brand CLI is on PATH: `.claude/skills/` gated on `claude`, `.opencode/commands/` gated on `opencode`.

The section covers BOTH scopes:

- **Repo-level deployment** — fab-kit's `fab sync`, which deploys kit skills to repo-local `.claude/skills/`, `.opencode/commands/`, `.agents/skills/` (post-yd9s: `.agents` AlwaysOn, `.claude` gated on `claude`, `.opencode` gated on `opencode`).
- **Global/machine-level placement** — `shll setup agent`, which places the `shll-toolkit` bootstrap skill under `$HOME` (post–Part 2: `~/.agents/skills/` unconditional, `~/.claude/skills/` gated on `claude`).

Record the rationale in the section: de-branded internals (the open-standard dir is the canonical read channel), no littering non-brand machines/repos with unread brand trees, taxonomy purity, the retirement stepping stone, the "if you run Claude Code, `claude` is on PATH" gate-fit argument, the verified fact that Claude Code does not read `.agents/skills/` (which is why the gated `.claude` channel must exist at all), and fab-kit's **one-target-per-skill-set invariant** (per-brand copies for CLIs that already read `.agents/skills` produced duplicate-skill conflict warnings — a skill set deploys to exactly one directory a given client reads).

Also update the two existing stale passages to the gated contract for `.claude`:

- § "Landed design: `shll setup agent`" — currently: "places one thin bootstrap Agent Skill (`shll-toolkit`) into the harnesses' global skills directories (`~/.agents/skills/` and `~/.claude/skills/`)". Becomes: placed into `~/.agents/skills/` always, and into `~/.claude/skills/` when the `claude` CLI is on PATH.
- § "The placed skill conforms to the Agent Skills spec" — currently: "places into harness-owned skills directories (`~/.agents/skills/`, `~/.claude/skills/`)". Same gated-contract rewording (conformance rules themselves unchanged).

Wording stays consistent with the other standards pages (`principles.md` etc. — declarative, producer-facing, RFC-2119 where binding).

**Embed re-sync (required)**: `docs/site/standards/*.md` are embedded in the shll binary (`shll standards` serves them; committed copies under `src/cmd/shll/standards/`, synced by `scripts/sync-standards.sh`, drift-guarded by `TestStandardsEmbedMatchesCanonical`). The skill.md edit must be followed by the embed re-sync so the drift guard stays green.

### 2. Gate shll's global `~/.claude/skills/` write (`src/cmd/shll/agent_setup.go`)

Current behavior: `skillTargetRelDirs = [".agents/skills", ".claude/skills"]` (agent_setup.go ~line 126); `resolveSkillTargets` derives both absolute SKILL.md paths and `runAgentInstall` writes both unconditionally.

Change: `~/.agents/skills/` stays unconditional; the `~/.claude/skills/` WRITE becomes gated on `claude` being on PATH (the global-scope mirror of yd9s's repo-scope tiering). Concretely:

- The **install path** (`runAgentInstall`) writes `~/.claude/skills/shll-toolkit/SKILL.md` only when `claude` is on PATH. On machines without `claude`, `shll setup agent` no longer creates `~/.claude/` at all.
- The **gate never deletes**: an existing `~/.claude/` (or a previously placed `~/.claude/skills/shll-toolkit/`) is never removed by the gate — only `--uninstall` deletes.
- The **`--uninstall` path** (`runAgentUninstall`) keeps removing BOTH skill directories regardless of the gate — an existing `~/.claude` placement removes cleanly on a machine where `claude` has since disappeared.
- The **staleness/placement probe** (`agentSkillPlacementState`, consumed by `shll update`'s conditional refresh and `shll doctor`'s shll-row staleness check) keeps checking BOTH paths regardless of the gate — an existing `~/.claude` placement still refreshes and reports staleness cleanly.
- **Post-hoc Claude install is picked up automatically (desirable property — record it)**: a user who installs Claude Code AFTER running setup gets `~/.claude/skills/` on their next `shll update`, because the end-of-run refresh re-runs `shll setup agent` as a subprocess (`refreshPlacedAgentSkills` → `refreshArgv`) and the gate is then open. The refresh's placed-only gating is satisfied by the always-present `~/.agents` copy.
- **Callers inherit the gate by construction**: bare `shll setup` and `shll install`'s post-install auto-run both call the same `runAgentSetup` seam — no changes needed there.
- **Gate mechanism**: a PATH presence check (Go `exec.LookPath` equivalent — how fab-kit's `agentAvailable` gates its rows). shll has no existing pure-presence helper (its pattern is `proc.ErrNotFound` from a real invocation, e.g. `probeToolVersion`); Constitution I governs subprocess *invocation* and a PATH lookup spawns nothing, but code-review rules flag direct `os/exec` in command code — so route the lookup through a small `internal/proc` helper (e.g. `proc.LookPath`) or an injectable package-level func var in `agent_setup.go`, keeping command code `os/exec`-free and giving tests their seam. Final micro-placement is apply's call (see Assumptions #6).
- **`--print` mode reflects the gate**: `--print` prints the SKILL.md content plus the target paths a real run would write — on a no-`claude` machine it lists only the `~/.agents/skills/` path (delegated decision — see Assumptions #7).

### 3. Tests (`src/cmd/shll/agent_setup_test.go`)

The test seam is `runAgentSetup` driven with `bytes.Buffer` writers, controlled env (`HOME` → `t.TempDir()`), and a fake `proc.Runner`; the gate must be testable without a real `claude` on PATH — an injection seam for the lookup, consistent with how the file already injects env and the Runner. Cases:

- `claude` present → both `~/.agents/skills/` and `~/.claude/skills/` written (existing coverage adjusted to run under an open gate).
- `claude` absent → only `~/.agents/skills/` written; **no `~/.claude/` directory created** (assert dir absence, not just file absence).
- `--uninstall` removes both dirs regardless of gate state.
- `agentSkillPlacementState` still covers both paths regardless of gate state (a stale pre-existing `~/.claude` copy reports `stale` on a no-`claude` machine).
- `--print` output matches the gate state (per the assumption above).

Go changes ship tests (project norm); existing description/frontmatter contract tests (`TestAgentSetup_Description*`, `TestRosterProactiveHint`, etc.) are unaffected — SKILL.md content does not change.

### 4. Memory sweep (present-truth claims about the unconditional pair)

See Affected Memory. Grep-verified carriers of the unconditional-pair claim: `docs/memory/cli/setup.md` (§ "The placement set", the agst Design Decision "no harness detection … user: 'no degeneration'"), `docs/memory/cli/install.md` (example output block showing both `wrote` lines, ~lines 151–152), `docs/memory/cli/standards-content.md` (describes the skill standard's content, which gains the Placement directories section). The agst decision is **re-scoped, not deleted** (record the reversal rationale, yd9s-style).

### Not touched

- The `shll skill` runtime two-step (glossary → bundle → topic).
- The placed SKILL.md content itself (`agentSkillContent`, its description builder, all vocabulary contracts).
- The run-kit delegation (`run-kit agent setup`) and its `--yes` consent chain.
- Uninstall semantics (still both-path, still `os.RemoveAll` on the shll-owned dirs).
- `.opencode` at global scope — shll places nothing there; the gated-tier `.opencode/commands/` entry in the standard describes fab-kit's repo-level target only.
- No new subcommands (Constitution VII — no justification needed; the change is behavior inside existing commands).

## Affected Memory

- `cli/setup`: (modify) § "The placement set (two writes cover four harnesses)" → gated contract; re-scope the agst "Skill placement …"/unconditional-pair Design Decision with reversal rationale; document the gate seam, `--print` gate behavior, and the new test cases; the "post-hoc claude install → next `shll update` places it" behavior contract
- `cli/standards-content`: (modify) record the skill standard's new "Placement directories" section (two-tier taxonomy, both scopes, one-target-per-skill-set invariant) and the updated landed-design passages
- `cli/install`: (modify) present-truth sweep — the post-install auto-run example output showing both `wrote` lines becomes gate-dependent
- `cli/update`: (modify) verify; the end-of-run refresh description stays true (placement-gated on ANY target present), add/verify the post-hoc-claude-pickup note only if its present claims go stale
- `cli/standards-conformance`: (modify) verify — shll's conformance state vs. the skill standard changes with the standard itself (the placed-skill rules are unchanged; note the placement-tier rule is now self-conformant by this change)

## Impact

- **Files**: `docs/site/standards/skill.md` (new section + two passage updates), `src/cmd/shll/standards/skill.md` (embed re-sync via `scripts/sync-standards.sh`), `src/cmd/shll/agent_setup.go` (`skillTargetRelDirs`/`resolveSkillTargets`/`runAgentInstall`/`runAgentPrint` gating + lookup seam), possibly `src/internal/proc/proc.go` (a `LookPath` helper, apply's call), `src/cmd/shll/agent_setup_test.go`, memory files per Affected Memory.
- **Behavior contract change**: on machines without `claude`, `shll setup agent` (and therefore `shll install`'s auto-run and `shll update`'s refresh) stops creating `~/.claude/`; existing dirs never deleted by the gate; uninstall/probe unchanged in coverage.
- **Release tier**: feat → MINOR.
- **Constitution fit**: I (any lookup routed cleanly; the one subprocess — run-kit delegation — is untouched), V (graceful degradation — a missing `claude` skips that surface silently, exactly the pattern), VII (no new subcommands), § Toolkit Standards (the docs/site edit is checked against — and IS — the governing standard; other standards pages' style honored). Test Integrity: tests updated to the new spec, not vice versa.
- **Risk**: low — additive standard section + a write-gate on one of two file writes; the probe/uninstall keeping both paths removes the stale-placement risk class. The main sweep risk is present-truth wording in memory/standard passages (three grep-verified carriers, enumerated above).

## Open Questions

None — the design conversation resolved all decision points; the one delegated decision (`--print` gate behavior) is recorded as a graded assumption below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Both parts (standards section + shll gate) ship in ONE shll change | Discussed — user explicitly chose "Do both" in one change | S:95 R:70 A:90 D:95 |
| 2 | Certain | Taxonomy codified as a RULE with two tiers (unconditional `.agents/skills/`; brand surfaces gated on their CLI), covering both repo-level (`fab sync`) and global (`shll setup agent`) scopes | Source-of-truth description states this verbatim; grounded in yd9s intake | S:95 R:75 A:90 D:90 |
| 3 | Certain | `~/.claude/skills/` WRITE gated on `claude` on PATH; `~/.agents/skills/` stays unconditional; gate never deletes existing dirs | Discussed — the core decision, mirrors yd9s at global scope | S:95 R:70 A:90 D:90 |
| 4 | Certain | `--uninstall` and `agentSkillPlacementState` (update refresh + doctor) keep checking BOTH paths regardless of gate | Discussed — explicit design decision in the conversation | S:95 R:80 A:90 D:90 |
| 5 | Certain | Post-hoc Claude install picked up on next `shll update` (refresh re-runs `setup agent` as subprocess) — recorded as desirable behavior contract | Discussed — "this is a desirable property, record it"; verified against `refreshPlacedAgentSkills`/`refreshArgv` in code | S:90 R:85 A:95 D:90 |
| 6 | Confident | Gate lookup = PATH presence check behind an injectable seam; prefer routing through `internal/proc` (e.g. a small `proc.LookPath`) so command code stays `os/exec`-free; exact placement is apply's call | Conversation said "follow existing shll patterns"; no pure-presence helper exists today; code-review rules flag direct `os/exec` in command code, and the test seam requires injection either way | S:70 R:85 A:80 D:70 |
| 7 | Confident | `--print` reflects the gate: prints only the target paths a real run would write | Explicitly delegated ("decide and record as an assumption"); two valid options — always-both is also defensible for debugging — chose dry-run truthfulness; trivially reversible | S:60 R:90 A:70 D:50 |
| 8 | Certain | The standard's two stale unconditional-pair passages (§ Landed design, § The placed skill conforms) updated to the gated contract for `.claude` | Source-of-truth description names both passages as MUST-update; verified present at skill.md lines 84 and 90 | S:95 R:85 A:95 D:95 |
| 9 | Certain | Standards edit requires embed re-sync (`scripts/sync-standards.sh` → `src/cmd/shll/standards/skill.md`) to keep `TestStandardsEmbedMatchesCanonical` green | Derived from codebase/memory (`cli/standards` memory documents the drift guard over all standards) | S:85 R:90 A:95 D:95 |
| 10 | Certain | The standard records fab-kit's one-target-per-skill-set invariant rationale (per-brand copies for `.agents`-reading CLIs caused duplicate-skill conflict warnings) | Source-of-truth description explicitly requires it | S:90 R:85 A:90 D:90 |
| 11 | Certain | Memory sweep set = `cli/setup` + `cli/standards-content` + `cli/install` (+ verify-only `cli/update`, `cli/standards-conformance`); agst decision re-scoped, not deleted | Grep-verified carriers of the unconditional-pair claim; re-scope-not-delete mirrors yd9s's handling of the #647 decision | S:75 R:90 A:85 D:80 |
| 12 | Certain | Change type feat → MINOR; no new subcommands (Constitution VII untriggered); Go changes ship tests | Discussed and stated in the design summary; matches project norms | S:90 R:90 A:95 D:95 |
| 13 | Certain | Bare `shll setup` and `shll install`'s auto-run inherit the gate with no code changes (both call the `runAgentSetup` seam) | Derived from code (`runSetup`, install's `runPostInstallSetup` both route through the same seam) | S:75 R:85 A:90 D:85 |

13 assumptions (11 certain, 2 confident, 0 tentative, 0 unresolved).
