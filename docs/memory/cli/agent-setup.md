---
type: memory
description: "`shll agent-setup` — mechanically places ONE thin `shll-toolkit` Agent Skill at two unconditional global paths (`~/.agents/skills/` + `~/.claude/skills/`, the minimal covering set for all four harnesses), then delegates run-kit's dashboard hooks to `run-kit agent-setup`. Idempotent by construction (write/overwrite/delete, no merge/sentinel/prompt); `--print` (content + paths, no write, no delegate), `--uninstall` (delete both dirs + delegate uninstall), `--print --uninstall` → exit 2. Canonical SKILL.md is a Go constant; NO context-file stanza injection."
---
# cli/agent-setup

`shll agent-setup` — mechanically places one thin Agent Skill (the toolkit bootstrap) into the agent harnesses' global skills directories, then delegates run-kit's dashboard-hook wiring to `run-kit agent-setup`. It graduates the cross-toolkit harness wiring from run-kit (a leaf tool, where it was mis-homed) up to shll (the manager) — the graduation half of change `agst`; the composition half is [cli/skill](/cli/skill.md).

Source: `src/cmd/shll/agent_setup.go` (+ `agent_setup_test.go`). Added by change `agst` (`260718-agst-agent-setup-skill-commands`).

## The mechanism: skill placement, NOT stanza injection

**Load-bearing design decision.** The backlog's original `[agst]` mechanism was sentinel-wrapped context-stanza injection into `~/.claude/CLAUDE.md` / AGENTS.md-family files (reusing `shell_setup.go`'s sentinel machinery). The user **rejected** that this session — *"no merge operation, just mechanical placement of skills"* — in favor of placing an **Agent Skill**. The skill directories are shll-owned, so:

- **install = write, re-run/upgrade = overwrite, `--uninstall` = delete** — idempotent by construction.
- **No sentinel, no merge, no diff-and-confirm, no `--yes` gate, no non-TTY refusal.** Those existed to protect user-authored files; overwriting shll-owned skill files needs none of them.
- A skill costs one description line per agent session (loaded on demand) instead of an always-loaded CLAUDE.md stanza.

The rejected stanza machinery must not reappear: `shell_setup.go` stays byte-identical to HEAD and no `sentinel_block.go` exists (guarded by the removal-verification acceptance; the change reverted the apply's first-pass stanza work).

## The placement set (two writes cover four harnesses)

`skillTargetRelDirs` (relative to `$HOME`) is the **minimal covering set**, verified 2026-07-18 from each harness's official docs:

| Path (`$HOME`-relative) | Covers |
|-------------------------|--------|
| `.agents/skills` | the [agentskills.io](https://agentskills.io) open-standard path — read natively by **Codex** (USER scope), compat-read by **Cursor** and **OpenCode** |
| `.claude/skills` | **Claude Code** (which does NOT read `~/.agents/`) |

The full file is `<dir>/shll-toolkit/SKILL.md` (`skillDirName = "shll-toolkit"`, `skillFileName = "SKILL.md"` — the `<dir>/<name>/SKILL.md` shape the Agent Skills standard requires). `resolveSkillTargets(env)` joins `$HOME + rel + skillDirName + skillFileName`; an empty `$HOME` yields no targets (nothing to place).

Both writes are **unconditional** — agent-setup is an explicit "wire this machine" command, the cost is two small files in `$HOME`, and any future harness adopting the open standard picks up `~/.agents/skills` automatically. **No harness detection, no skip logic, no skip-a-harness degradation** (user: "no degeneration"). Cursor and OpenCode will see the same-name skill from both locations; the bytes are identical, so this is cosmetic (neither documents cross-location precedence; the recorded fallback is symlinking `~/.claude/skills/shll-toolkit` → the `~/.agents` copy, since Claude Code follows and dedupes symlinked skill dirs).

## The canonical SKILL.md (a Go constant)

`agentSkillContent` is a Go string constant in `agent_setup.go` — **not** a docs-site file. The bootstrap skill is an agent-setup artifact, neither a published standard nor a `<tool> skill` bundle, so the docs-site sync/embed/drift-guard ceremony (which `docs/site/skill.md` uses — see [cli/skill](/cli/skill.md#the-bundle-authored-embedded-drift-guarded-budget-bounded)) does **not** apply here.

- **Portable frontmatter — `name` + `description` ONLY** (the OpenCode-recognized common subset, valid on all four harnesses). `name: shll-toolkit` equals `skillDirName` (the same constant is spliced into both the frontmatter and the directory name, so they cannot drift) and satisfies the shared `^[a-z0-9]+(-[a-z0-9]+)*$` / match-directory-name rule.
- **The `description` front-loads trigger words** (the tool names plus each tool's task-domain phrase) so the skill activates implicitly when an agent is about to drive a toolkit tool. Its trailer names the runtime two-step (`Run 'shll skill' to list … 'shll skill <tool>' for that tool's full usage bundle`). **The description trailer is deliberately NOT extended to the topic form** — it is single-line activation-trigger vocabulary (one YAML line, asserted by `TestAgentSetup_DescriptionSingleLine`), not a teaching surface; the topic form belongs in the body's step 2.
- **The body teaches the runtime discovery steps** (`shll skill` → `shll skill <tool>` → `shll skill <tool> <topic>`) plus one `shll standards` pointer for toolkit-repo development. Step 2 notes that a large-scope tool's core bundle lists its topic pages and `shll skill <tool> <topic>` serves one on demand (extended by change tp2s). It only *points at* the runtime steps, so bundles are always fetched from the installed binaries — the placed file stays version-locked in spirit and is refreshed by the installed shll on any re-run (the change-#50 refresh machinery propagates the new body on the next `shll update`).

## Modes and the run seam

`runAgentSetup(ctx, env, stdout, stderr, printMode, uninstallMode)` (the test seam; the cobra factory passes `os.Getenv`, `Args: cobra.NoArgs`):

- **`--print --uninstall` together** → `errExitCode{code: usageExitCode}` (exit 2) — mutually exclusive, checked first.
- **`--print`** (`runAgentPrint`) → writes `agentSkillContent` then a `Target paths:` block listing both resolved absolute paths, and **modifies nothing**. **No run-kit delegation.**
- **`--uninstall`** (`runAgentUninstall`) → `os.RemoveAll` on each `shll-toolkit` **directory** (`filepath.Dir(path)`, not just the SKILL.md file); reports `removed`/`absent` per dir; then delegates `run-kit agent-setup --uninstall`.
- **default** (`runAgentInstall`) → `placeSkill` per target, then delegates `run-kit agent-setup`.

### `placeSkill` — the three-state per-path summary

`placeSkill(path, content, stdout, stderr)` distinguishes three states by reading existing bytes before writing (`os.ReadFile` → compare):

- **`wrote`** — file did not exist (`os.ErrNotExist`); `os.MkdirAll(dir, 0o755)` then `os.WriteFile(path, content, 0o644)`.
- **`unchanged`** — file existed and already held the canonical bytes; **no write is performed** (idempotent re-run).
- **`updated`** — file existed with different bytes; overwritten.

The compare uses a tiny local `bytesEqual` helper (the file's footprint is file-I/O-only — it avoids importing `bytes` for one call). A non-not-exist read error (permission, etc.) surfaces to stderr and is skipped (sets `anyFailed`). `anyFailed` on any target → `errSilent` (exit 1) after the loop.

### run-kit delegation

`delegateRunKitAgentSetup(ctx, uninstall bool, stdout, stderr)` invokes `run-kit agent-setup [--uninstall]` as a **foreground** subprocess via `proc.RunForeground` (Constitution I; `runKitAgentSetupSub = "agent-setup"`, binary name is `runKitToolName = "run-kit"`, **reused from `install.go`** — its only remaining consumer after the nudge graduation, see below):

- **run-kit absent** (`proc.ErrNotFound`) → skip silently (Constitution V).
- **a real delegation error** (non-absent) → surfaced to stderr with `(continuing)` — it does NOT fail the placement shll already did (placement is agent-setup's core work; run-kit hooks are the optional adjunct).
- Its stdio is inherited (foreground) so the user sees run-kit's own output.
- Only the default (install) and `--uninstall` paths delegate; **`--print` never does.**

## Touchpoint graduation (both former `run-kit agent-setup` pointers)

Change `agst` graduated the two existing pointers at `run-kit agent-setup` to `shll agent-setup`:

- **`install.go`'s post-install "Next steps" nudge** — the former run-kit-gated `runKitAgentSetupFmt` line became the unconditional `agentSetupNudgeFmt` (`shll agent-setup # optional, once per machine — wire agent harnesses (toolkit context + run-kit dashboard hooks)`). Because shll is by definition present, the presence gate was removed — the line prints unconditionally on both outcome paths, still not on dry-run. See [cli/install §the post-install nudge graduation](/cli/install.md#the-post-install-next-steps-nudge-change-93r2).
- **README install flow** — the command block and its explanation paragraph, plus new `### shll agent-setup` and `### shll skill` command sections describing the current skills-placement + two-step design (no stanza wording anywhere).

## Constitution VII justification

`agent-setup` = cross-toolkit machine wiring graduating from run-kit, where it was mis-homed on a leaf tool; it belongs in the manager (shll), and could not be a flag on any existing subcommand (it is a distinct machine-provisioning verb). Recorded in [cli/commands §Constitution VII per subcommand](/cli/commands.md#constitution-vii-justification-per-subcommand).

## Constitution fit

I — the ONE subprocess (run-kit delegation) routes through `internal/proc`; skill placement is plain `os` file I/O in shll-owned directories. II — stateless (no tracking of whether agent-setup ran; re-run re-derives via read-then-compare). III/IV — delegates run-kit's hooks by *pointing at* `run-kit agent-setup`, never absorbing them; run-kit agent-setup keeps working standalone. V — run-kit absent → silent skip. VII — justified above.

## Test seam

`agent_setup_test.go` drives `runAgentSetup` with `bytes.Buffer` writers, a controlled `env` (`HOME` → `t.TempDir()`), and a fake `proc.Runner`. Coverage grounds R6–R9: both files written with canonical content and a per-path summary; idempotent re-run (byte-identical → `unchanged`); `--print` writes nothing and does not delegate; `--uninstall` removes both dirs and delegates the uninstall pass-through; `--print --uninstall` exits 2; run-kit delegation present-when-installed / silent-when-absent; portable frontmatter (`name` + `description` only) with `name == shll-toolkit == dir name`.

## Cross-references

- The runtime steps the placed skill teaches (`shll skill` glossary → `shll skill <tool>` bundle → `shll skill <tool> <topic>` topic page): [cli/skill](/cli/skill.md).
- The nudge graduation and the shared `runKitToolName` constant (now consumed only by this file's delegation): [cli/install §the post-install nudge](/cli/install.md#the-post-install-next-steps-nudge-change-93r2).
- The subprocess wrapper the delegation uses: [internal/proc](/internal/proc.md).
- Root wiring (`newAgentSetupCmd`), the exit-code sentinels (`errExitCode`/`usageExitCode`/`errSilent`): [cli/commands](/cli/commands.md).
- The standard paragraph this command realizes (which originally described "aggregating bundles into context" — now the standard's own landed-design note records skills placement, not context aggregation): [cli/standards-content §landed design](/cli/standards-content.md#landed-design-shll-agent-setup-skills-placement-not-context-aggregation).
- Constitution I (Security First — the delegation routes through `internal/proc`), III (Wrap, Don't Reinvent), IV (Composition, Not Replacement), V (Graceful Degradation), VII (Minimal Surface Area).
