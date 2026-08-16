---
type: memory
description: "`shll agent-setup` — places ONE thin `shll-toolkit` Agent Skill at `~/.agents/skills/` + `~/.claude/skills/`, then delegates run-kit's dashboard hooks to `run-kit agent-setup` (`--yes`/`-y` forwards `--yes` to that delegation for unattended runs). Idempotent (write/overwrite/delete, no sentinel); `--print`/`--uninstall` modes. SKILL.md is a Go constant; `agentSkillDescription()` builds the frontmatter from the Roster: `SkillHint` clauses plus each `ProactiveHint`."
---
# cli/agent-setup

`shll agent-setup` — mechanically places one thin Agent Skill (the toolkit bootstrap) into the agent harnesses' global skills directories, then delegates run-kit's dashboard-hook wiring to `run-kit agent-setup`. Cross-toolkit harness wiring belongs in shll (the manager), not run-kit (a leaf tool); the composition half of the design is [cli/skill](/cli/skill.md). (agst)

Source: `src/cmd/shll/agent_setup.go` (+ `agent_setup_test.go`).

## The mechanism: skill placement, NOT stanza injection

The skill directories are shll-owned, so:

- **install = write, re-run/upgrade = overwrite, `--uninstall` = delete** — idempotent by construction.
- **No sentinel, no merge, no diff-and-confirm, no placement confirmation, no non-TTY refusal.** Those exist to protect user-authored files; overwriting shll-owned skill files needs none of them. (The command's `--yes`/`-y` flag gates nothing in shll — it exists solely to be forwarded to the run-kit delegation, whose hook wiring DOES prompt; see [run-kit delegation](#run-kit-delegation).)
- A skill costs one description line per agent session (loaded on demand) instead of an always-loaded CLAUDE.md stanza.

**Stanza machinery must not reappear**: `shell_setup.go`'s sentinel machinery is not reused here and no `sentinel_block.go` exists — see the Design Decision below.

## Design Decisions

### Skill placement, not context-stanza injection
**Decision**: `agent-setup` places an Agent Skill file (write/overwrite/delete); it never merges content into user-authored files.
**Why**: "No merge operation, just mechanical placement of skills" — skill directories are shll-owned, so the sentinel/merge/confirm machinery that protects user-authored rc files is unnecessary, and a skill's description line loads on demand instead of taxing every session like a CLAUDE.md stanza.
**Rejected**: Sentinel-wrapped context-stanza injection into `~/.claude/CLAUDE.md` / AGENTS.md-family files (reusing `shell_setup.go`'s sentinel machinery) — explicitly rejected by the user.
*Introduced by*: `260718-agst-agent-setup-skill-commands`

### Explicit `--yes` plumbing, not TTY detection
**Decision**: Unattended-run consent rides an explicit `--yes` flag threaded through the chain `shll update --yes` → `shll agent-setup --yes` → `run-kit agent-setup --yes`; shll never infers attendance from the terminal.
**Why**: The motivating failure is a pane-TTY-but-unattended session — run-kit's dashboard update button runs `shll update` in an rk-jobs tmux window, where stdin IS a TTY, so run-kit's non-TTY `--yes` refusal never triggers and its hook prompt hangs forever with nobody attached. That state is structurally undetectable from inside the process; only the caller knows nobody is watching, so the caller must say so.
**Rejected**: TTY detection (fails the motivating case exactly); making `shll update`'s agent-setup refresh unconditionally `--yes` (removes user consent for run-kit's hook writes on attended runs).
*Introduced by*: 260815-3ovi-yes-flag-update-agent-setup

### Design Decision: the `ProactiveHint` does three jobs
**Decision**: run-kit's `ProactiveHint` is a two-sentence value doing three load-bearing jobs. Sentence one carries agent-proactive trigger vocabulary (visual display + proxy a local http port + notify). Sentence two is a counter-instruction with two collision surfaces: (b) local `open`/`xdg-open`/localhost URLs may never reach a remote-dashboard user, so read `shll skill run-kit` before opening any file or local port in a browser; and (c) publishing to a hosted artifact page (e.g. claude.ai) forces a remote-dashboard user off the dashboard, so the same recipe applies before publishing an artifact or hosted page to show the user something.
**Why**: The placed skill fails to route agents to run-kit's proxy/visual recipe under **skill shadowing** — a competing content-generation plugin (e.g. `visual-explainer:generate-web-diagram`) carries its own complete delivery path and mentions nothing about rk/proxy/iframe, so the harness activates it and the toolkit skill's body never loads. The only shll-owned text guaranteed in every session's context is the placed skill's frontmatter **description** (all installed skills' descriptions are always listed; bodies load only on activation), so the fix lives there. More trigger vocabulary alone would not fire (a request like "show me examples" contains no proxy/remote/port words); a counter-instruction is what collides with the competing skill's delivery step and creates the unresolved gap that sends the agent to `shll skill run-kit`. The delivery paths that must collide are open-ended: the local-browser surface (b) is not enough by itself, because the Claude Code **Artifact tool** publishes HTML to claude.ai and returns a hosted URL — it opens no file and touches no local port, so it slips past both the proxy vocabulary and the local-browser counter-instruction. Observed 2026-07-22 in a run-kit-managed session: an agent delivered visuals **twice** via hosted artifacts, forcing a dashboard viewer off-dashboard. Function (c) names hosted publishing in both the *reason* clause (forced off the dashboard) and the *action* clause (read `shll skill run-kit` before publishing an artifact/hosted page), closing that vocabulary hole.
**Rejected**: Patching `visual-explainer` (third-party; the shadowing class is open-ended) — changing the skill BODY (loads only on activation, exactly what shadowing prevents) — changing run-kit's own bundle (different repo, already hop-2/unreachable in the failure mode) — a run-kit `agent-setup` session-start hook (the durable, deterministic escalation) **explicitly rejected by the user 2026-07-22 ("messes with user context")**; description wording is the chosen mechanism. The fix is probabilistic by design — a description line competes with a skill the harness has already committed to and can lose.
*Introduced by*: `260721-xv71-runkit-proactivehint-proxy-vocab` (functions a, b); function (c) hosted-artifact counter-instruction by `260722-e09x-runkit-proactivehint-artifact-vocab`

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
- **The `description` front-loads two kinds of trigger vocabulary**, both single-sourced from the Roster by `agentSkillDescription()` so they cannot drift from the managed set:
  - **Reactive task-domain clauses** — one `task-domain phrase (tool)` clause per tool (`git worktrees (wt)`, …, `tmux sessions (run-kit/rk)`), from each tool's `SkillHint` (with the `LegacyName` alias appended for run-kit). These match a user's own words ("create a worktree"), so every tool earns a clause.
  - **Agent-proactive sentence(s)** — each non-empty `ProactiveHint` appended verbatim (Roster order) **after** the tool clauses and **before** the closing two-step pointer. This is the vocabulary an agent should reach for *unprompted*, so the sprawl guard applies: **only run-kit carries a `ProactiveHint` today**, a two-sentence value that does three jobs. Sentence one carries the reach-for-unprompted vocabulary for run-kit's agent-proactive capabilities — visual display, proxying a local http port to the user's browser, and notify ("show the user visual content … in a browser window, to proxy a local http port to the user's browser, or … push a notification to their devices"). Sentence two is a **skill-shadowing counter-instruction** covering two delivery surfaces: "The user may be viewing this session remotely through run-kit's web dashboard, where `open`/`xdg-open` and localhost URLs never reach them and publishing to a hosted artifact page (e.g. claude.ai) forces them off the dashboard — before opening any file or local port in a browser, read `shll skill run-kit` for the proxied-iframe recipe; the same applies before publishing an artifact or hosted page to show the user something." Reactive tools stay clause-only; the user's words already name them.

  The trailer names the runtime two-step (`Run 'shll skill' to list … 'shll skill <tool>' for that tool's full usage bundle`). **The description trailer is deliberately NOT extended to the topic form** — it is single-line activation-trigger vocabulary (one YAML line, asserted by `TestAgentSetup_DescriptionSingleLine`), not a teaching surface; the topic form belongs in the body's step 2.
- **The body teaches the runtime discovery steps** (`shll skill` → `shll skill <tool>` → `shll skill <tool> <topic>`) plus a thin proactive-capabilities pointer line ("Run-kit also has agent-proactive capabilities — visual display … and push notifications; see `shll skill run-kit`.") and one `shll standards` pointer for toolkit-repo development. Step 2 notes that a large-scope tool's core bundle lists its topic pages and `shll skill <tool> <topic>` serves one on demand (extended by change tp2s). Body text loads only on activation, so the pointer line is activation-cost-only. It only *points at* the runtime steps, so bundles are always fetched from the installed binaries — the placed file stays version-locked in spirit and is refreshed by the installed shll on any re-run (the change-#50 refresh machinery propagates the new body on the next `shll update`).

### The description builder (`agentSkillDescription`)

`agentSkillDescription()` builds the single-line frontmatter description from the Roster in one pass:

```
Use when driving any shll toolkit CLI or shll itself — {clause, …}. {ProactiveHint, …} Run `shll skill` to list the installed tools; run `shll skill <tool>` for that tool's full usage bundle before using it.
```

- Each Roster tool contributes `"<SkillHint> (<name>)"` (name = `Name`, or `Name/LegacyName` when a `LegacyName` exists).
- Each non-empty `ProactiveHint` is collected in the same loop and joined with a single space, then spliced in between the clause list and the two-step pointer — so the proactive vocabulary always falls **after** the tool clauses and **before** `Run \`shll skill\``. With zero proactive hints the splice is skipped entirely (no stray spacing).
- **Single-line, `: `-free invariant.** The whole description MUST be one line with no `: ` sequence (it is an unquoted YAML scalar). `TestAgentSetup_DescriptionSingleLine` pins this; run-kit's `ProactiveHint` sentence is newline- and `: `-free so it satisfies the invariant as-is.

**`ProactiveHint` holds the complete two-sentence prose verbatim; the builder just appends it** — there is no builder-owned "Also use proactively" preamble composed around a stored fragment. This is the simplest faithful rendering while exactly one tool carries a hint; a composed preamble would be over-engineering. The value does **three** jobs, all load-bearing: **(a) proxy trigger vocabulary** ("to proxy a local http port to the user's browser") matching requests that name proxying/dev servers; **(b) a local-browser skill-shadowing counter-instruction** ("The user may be viewing this session remotely … before opening any file or local port in a browser, read `shll skill run-kit`") that fires the moment any competing skill's local delivery step (`open`/`xdg-open`/localhost URL) is about to run; and **(c) a hosted-artifact counter-instruction** ("publishing to a hosted artifact page (e.g. claude.ai) forces them off the dashboard … the same applies before publishing an artifact or hosted page") that fires when an Artifact-style hosted-publishing delivery step — which opens no file and touches no local port — is about to route visuals off the dashboard. All three fire regardless of which skill was activated (see the [skill-shadowing Design Decision](#design-decision-the-proactivehint-does-three-jobs) below). The sprawl guard (only agent-proactive capabilities earn description space) is enforced by `TestRosterProactiveHint`, which asserts **exactly run-kit** carries a `ProactiveHint`, that the value appears verbatim in the rendered description, that it is positioned between the tool clauses and the two-step pointer, and — pinning the three functions against a silent rewording — that the rendered description contains the three load-bearing fragments `"to proxy a local http port"` (function a), `"before opening any file or local port in a browser, read"` (function b), and `"publishing an artifact"` (function c). `SkillHint` is unaffected — run-kit's stays `"tmux sessions"` (the reactive task-domain phrase); `TestRosterSkillHints` still enforces the every-tool `SkillHint` contract.

## Modes and the run seam

`runAgentSetup(ctx, env, stdout, stderr, printMode, uninstallMode, yes)` (the test seam; the cobra factory passes `os.Getenv`, `Args: cobra.NoArgs`):

- **`--print --uninstall` together** → `errExitCode{code: usageExitCode}` (exit 2) — mutually exclusive, checked first.
- **`--print`** (`runAgentPrint`) → writes `agentSkillContent` then a `Target paths:` block listing both resolved absolute paths, and **modifies nothing**. **No run-kit delegation.**
- **`--uninstall`** (`runAgentUninstall`) → `os.RemoveAll` on each `shll-toolkit` **directory** (`filepath.Dir(path)`, not just the SKILL.md file); reports `removed`/`absent` per dir; then delegates `run-kit agent-setup --uninstall`.
- **default** (`runAgentInstall`) → `placeSkill` per target, then delegates `run-kit agent-setup`.
- **`--yes`/`-y`** (registered via the shared `yesFlag`/`yesFlagShorthand` constants from `uninstall.go`, with its own `agentSetupYesUsage` string) → forwards `--yes` to the run-kit delegation on both the install and `--uninstall` paths (3ovi). **`--print --yes` is a harmless no-op, NOT a usage error** — print never delegates, so there is no prompt to skip (deliberate contrast with `--print`+`--uninstall`, which are contradictory modes).

### `placeSkill` — the three-state per-path summary

`placeSkill(path, content, stdout, stderr)` distinguishes three states by reading existing bytes before writing (`os.ReadFile` → compare):

- **`wrote`** — file did not exist (`os.ErrNotExist`); `os.MkdirAll(dir, 0o755)` then `os.WriteFile(path, content, 0o644)`.
- **`unchanged`** — file existed and already held the canonical bytes; **no write is performed** (idempotent re-run).
- **`updated`** — file existed with different bytes; overwritten.

The compare uses a tiny local `bytesEqual` helper (the file's footprint is file-I/O-only — it avoids importing `bytes` for one call). A non-not-exist read error (permission, etc.) surfaces to stderr and is skipped (sets `anyFailed`). `anyFailed` on any target → `errSilent` (exit 1) after the loop.

### run-kit delegation

`delegateRunKitAgentSetup(ctx, uninstall, yes bool, stderr)` invokes `run-kit agent-setup [--uninstall] [--yes]` as a **foreground** subprocess via `proc.RunForeground` (Constitution I; `agentSetupSub = "agent-setup"`, binary name is `runKitToolName = "run-kit"`, **reused from `install.go`** — its only remaining consumer after the nudge graduation, see below):

- **`yes` appends `--yes`** to the delegated argv (after `--uninstall` when both apply), skipping run-kit's `Write these changes? [y/N]` hook-wiring confirmation — the unattended-run consent chain (3ovi; see the [Design Decision](#explicit---yes-plumbing-not-tty-detection) below).
- **run-kit absent** (`proc.ErrNotFound`) → skip silently (Constitution V).
- **a real delegation error** (non-absent) → surfaced to stderr with `(continuing)` — it does NOT fail the placement shll already did (placement is agent-setup's core work; run-kit hooks are the optional adjunct).
- Its stdio is inherited (foreground) so the user sees run-kit's own output.
- Only the default (install) and `--uninstall` paths delegate; **`--print` never does.**

## Touchpoints

Two surfaces point users at `shll agent-setup` (agst):

- **`install.go`'s post-install "Next steps" nudge** — the unconditional `agentSetupNudgeFmt` line (`shll agent-setup # optional, once per machine — wire agent harnesses (toolkit context + run-kit dashboard hooks)`). shll is by definition present, so the line carries no presence gate — it prints on both outcome paths, never on dry-run. See [cli/install §the post-install nudge](/cli/install.md#the-post-install-next-steps-nudge).
- **README install flow** — the command block and its explanation paragraph, plus `### shll agent-setup` and `### shll skill` command sections describing the skills-placement + two-step design (no stanza wording anywhere).

## Constitution VII justification

`agent-setup` = cross-toolkit machine wiring graduating from run-kit, where it was mis-homed on a leaf tool; it belongs in the manager (shll), and could not be a flag on any existing subcommand (it is a distinct machine-provisioning verb). Recorded in [cli/commands §Constitution VII per subcommand](/cli/commands.md#constitution-vii-justification-per-subcommand).

## Constitution fit

I — the ONE subprocess (run-kit delegation) routes through `internal/proc`; skill placement is plain `os` file I/O in shll-owned directories. II — stateless (no tracking of whether agent-setup ran; re-run re-derives via read-then-compare). III/IV — delegates run-kit's hooks by *pointing at* `run-kit agent-setup`, never absorbing them; run-kit agent-setup keeps working standalone. V — run-kit absent → silent skip. VII — justified above.

## Test seam

`agent_setup_test.go` drives `runAgentSetup` with `bytes.Buffer` writers, a controlled `env` (`HOME` → `t.TempDir()`), and a fake `proc.Runner`. Coverage grounds R6–R9: both files written with canonical content and a per-path summary; idempotent re-run (byte-identical → `unchanged`); `--print` writes nothing and does not delegate; `--uninstall` removes both dirs and delegates the uninstall pass-through; `--print --uninstall` exits 2; run-kit delegation present-when-installed / silent-when-absent; portable frontmatter (`name` + `description` only) with `name == shll-toolkit == dir name`. The `--yes` forwarding (3ovi) is pinned by `TestAgentSetup_YesForwardsToDelegation` (install argv `agent-setup --yes`), `TestAgentSetup_YesRidesUninstallDelegation` (`agent-setup --uninstall --yes`), `TestAgentSetup_PrintWithYesIsNoOp` (no write, no delegation, exit 0), and `TestAgentSetup_YesFlagWiredThroughCobra` (flag name/shorthand/usage-string wiring).

Description-vocabulary contracts are pinned separately: `TestRosterSkillHints` (every tool declares a `SkillHint`, each rendered as a `hint (name)` clause), `TestRosterProactiveHint` (exactly run-kit carries a `ProactiveHint`, rendered verbatim after the clauses and before the two-step pointer — the sprawl guard — plus the three load-bearing fragments `"to proxy a local http port"`, `"before opening any file or local port in a browser, read"`, and `"publishing an artifact"`, so a rewording cannot silently drop the proxy vocabulary, the local-browser counter-instruction, or the hosted-artifact counter-instruction), `TestAgentSetup_DescriptionSingleLine` (single-line, `: `-free), and `TestAgentSetup_BodyTeachesTwoStepAndStandards` (body teaches the two-step + `shll standards`, no stanza/sentinel wording).

## Cross-references

- The runtime steps the placed skill teaches (`shll skill` glossary → `shll skill <tool>` bundle → `shll skill <tool> <topic>` topic page): [cli/skill](/cli/skill.md).
- The nudge and the shared `runKitToolName` constant (consumed only by this file's delegation): [cli/install §the post-install nudge](/cli/install.md#the-post-install-next-steps-nudge).
- The subprocess wrapper the delegation uses: [internal/proc](/internal/proc.md).
- Root wiring (`newAgentSetupCmd`), the exit-code sentinels (`errExitCode`/`usageExitCode`/`errSilent`): [cli/commands](/cli/commands.md).
- The standard's landed-design note recording skills placement (not context aggregation): [cli/standards-content §landed design](/cli/standards-content.md#landed-design-shll-agent-setup-skills-placement-not-context-aggregation).
- Constitution I (Security First — the delegation routes through `internal/proc`), III (Wrap, Don't Reinvent), IV (Composition, Not Replacement), V (Graceful Degradation), VII (Minimal Surface Area).
