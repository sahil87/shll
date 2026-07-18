# Intake: Add `shll agent-setup` + `shll skill`

**Change**: 260718-agst-agent-setup-skill-commands
**Created**: 2026-07-18

## Origin

> /fab-new agst

One-shot invocation resolving backlog item `[agst]` (2026-07-18): *"Add `shll agent-setup` + `shll skill` — graduate machine-level agent wiring from run-kit to shll."* The backlog entry pre-decides naming, grammar, composition mechanism, degradation policy, delegation, touchpoint graduation, and Constitution VII justifications.

**Post-intake redesign (same session, 2026-07-18)**: the user redirected `agent-setup`'s mechanism away from the backlog's context-file stanza injection (sentinel-wrapped block in `~/.claude/CLAUDE.md` / AGENTS.md-family files) to **mechanical placement of an Agent Skill** — "no merge operation, just mechanical placement of skills" — and explicitly rejected any skip-a-harness degradation: all four target harnesses must be covered via verified skills paths. The skills-placement design below supersedes the backlog's stanza design; everything else in `[agst]` stands.

Environment facts verified at intake time:

- `docs/site/standards/skill.md` **exists** — the skill standard (first declared dependency) is authored and shipped.
- The installed `run-kit` has **no `skill` subcommand** (`Error: unknown command "skill"`) — the coordinated run-kit change has NOT landed. Non-blocking for implementation (see §5).
- shll's standards-conformance audit (change `3sss`, memory `cli/standards-conformance`) reported the `skill` standard as *"deferred, not yet adopted — tracked in `[agst]`"* — this change is the tracked home for shll's own `skill` adoption, not just the composer.
- **Harness skills support verified from official docs (fetched 2026-07-18)** — all four target harnesses implement the [Agent Skills](https://agentskills.io) open standard (`<dir>/<name>/SKILL.md`, YAML frontmatter `name` + `description`):
  - **Claude Code** (code.claude.com/docs/en/skills): personal `~/.claude/skills/`, project `.claude/skills/`, enterprise, plugin. Does NOT read `~/.agents/skills/`. Symlinked skill dirs are followed and deduped.
  - **Codex** (learn.chatgpt.com/docs/build-skills): REPO `.agents/skills` (cwd upward), USER `$HOME/.agents/skills`, ADMIN `/etc/codex/skills`, plus built-ins. (`~/.codex/skills` appears in older Dec-2025 material; the current official contract is `.agents`.)
  - **Cursor** (cursor.com/docs/context/skills): native `~/.cursor/skills/` + `.cursor/skills/`, and compat-reads `.claude/skills/`, `.codex/skills/`, `.agents/skills/` at both levels.
  - **OpenCode** (opencode.ai/docs/skills): native `~/.config/opencode/skills/` + `.opencode/skills/`, and compat-reads `.claude/skills/` and `.agents/skills/` at both levels. Frontmatter recognizes ONLY `name`, `description`, `license`, `compatibility`, `metadata`; `name` must match `^[a-z0-9]+(-[a-z0-9]+)*$` and equal the directory name.

## Why

1. **The pain**: Agents driving toolkit tools have no offline, version-locked usage knowledge. `-h`/`help-dump` is flag reference, not "when to reach for which"; README/docs-site needs a checkout or network; `fab/project` context is repo-development-scoped. The `skill` standard defines the per-tool bundle that closes this gap — but nothing composes those bundles machine-wide, and nothing wires agent harnesses to find them. Meanwhile `run-kit agent-setup` carries the toolkit's harness context-injection today, where it is mis-homed: run-kit is a leaf tool, and its static stanza cannot scale to per-tool bundles.
2. **If we don't**: every harness/repo keeps wiring toolkit context ad hoc; agents operate tools from flag dumps and stale prose; run-kit retains a cross-toolkit responsibility that belongs to the manager (shll); and the skill standard's bundles, as leaf tools adopt them, have no aggregation/discovery path.
3. **Why this approach**: `shll skill` composes per-tool bundles the way `shell-init` composes shell-init — at runtime, by subprocess, so bundles stay version-locked to the *installed* tool (Constitution III). `shll agent-setup` **mechanically places one thin bootstrap skill** into the harnesses' skills directories instead of editing user-owned context files: install = write, upgrade = overwrite, uninstall = delete — idempotent by construction, no sentinel/merge machinery, no diff-and-confirm on files the user hand-maintains. A skill also costs one description line per session instead of an always-loaded CLAUDE.md stanza, and its body loads only when the agent is actually about to drive a toolkit tool. Run-kit's hook installation is *delegated* back to `run-kit agent-setup` (Constitution III/IV — compose, don't absorb).

## What Changes

### 1. `shll skill` (bare) — the cheap glossary

New subcommand. The bare form prints **one line per installed tool** — a context-economy glossary, NOT a concatenation of bundles (7 × ~150-line bundles into agent context would violate toolkit principle №9; the two-step "list, then per-tool on demand" is the deliberate contract).

- **Rows**: shll first (using `shllSelfDescription`, mirroring `list`/`version`'s shll-first ordering), then the `Roster` tools in declared (leaves-first) order, each with its hardcoded `Description` one-liner. Column-aligned like `version`/`list`.
- **Installed-only**: tools not on PATH are skipped silently (Constitution V; reuse the shared `toolInstalled` PATH probe from `version.go` — no brew calls, the glossary stays cheap). <!-- assumed: installed-only glossary — the placed skill's promise is "the tools you can drive"; full-roster-with-status is already `shll list`'s job -->
- **Trailing hint line** teaching the second step, e.g. `Run 'shll skill <tool>' for that tool's full agent skill bundle.`

Sketch:

```
shll      the manager for the shll toolkit
wt        Git worktree management — create, list, open, delete worktrees
idea      Backlog idea management from the terminal
tu        Token-usage tracker for AI coding tools (Claude Code, Codex, OpenCode)
run-kit   Run-kit — tmux session manager with a web UI (rk stays as an alias)
hop       Fast directory/project jumping across worktrees
fab-kit   Spec-driven workspace & workflow toolkit (the `fab` CLI)

Run 'shll skill <tool>' for that tool's full agent skill bundle.
```

### 2. `shll skill <tool>` — one tool's full bundle

- Resolves `<tool>` against the Roster + shll self-token, inheriting the legacy `rk` → `run-kit` alias via the shared resolver (as `changelog`/`uninstall` do). Unknown name → usage error, **exit 2** (the conformance convention from change `3sss`).
- For a Roster tool: invoke `<tool> skill` as a subprocess via `internal/proc` (short per-tool timeout, mirroring `version`'s probe pattern) and stream its stdout through **byte-identical** — no framing, no rendering (the standard's invocation contract: stdout is data).
- **Graceful degradation** (Constitution V): a tool that is missing from PATH, or whose installed version predates `skill` (subprocess fails with unknown-command / `ErrNotFound`), gets a clear one-line stderr notice (e.g. `wt does not support 'skill' yet — run 'shll update'`) and **exit 1** (operational, not usage). The probe is invoke-and-classify — `skill` either prints or errors, so no separate `--help`-substring probe. This is what lets the composer ship while zero leaf tools implement `skill`.

### 3. `shll skill shll` — shll's own bundle (adopting the skill standard)

shll's own `skill`-standard adoption was deferred to this change by the conformance audit, and the composer grammar collides with the standard's bare-form contract (`shll skill` can't be both the glossary and shll's own bundle), so shll's bundle is served through the self-target:

- Author shll's own bundle at **`docs/site/skill.md`** (the standards-directory restructure already reserved this path — see memory `cli/standards-content`): a ≤150-line, static-only usage briefing per the standard's content rules (when to use, capabilities map keyed to subcommands, composition patterns, output/exit-code contracts, gotchas).
- Embed it via the established **sync + drift-guard** mechanism (`scripts/sync-standards.sh` / `TestStandardsEmbedMatchesCanonical` precedent from `shll standards`): committed embedded copy, sync script refresh, drift-guard test.
- `shll skill shll` serves the embedded bundle **in-process** (a subprocess self-invocation would recurse into the composer). Byte-identical to `docs/site/skill.md`; renders at `shll.ai/shll/skill` for free.
<!-- assumed: shll's own bundle is in scope here — the backlog text is silent, but the conformance audit explicitly parked shll's skill adoption in [agst], and the self-target needs special-casing anyway -->

### 4. `shll agent-setup` — mechanical placement of the toolkit bootstrap skill

New subcommand. Places **one thin Agent Skill** — the toolkit bootstrap — into the harnesses' global skills directories. No context-file editing, no sentinel blocks, no merge: the skill directories are shll-owned, so install = write, re-run/upgrade = overwrite (idempotent), `--uninstall` = delete.

- **Placement set** (covers all four harnesses with exactly two writes — verified coverage matrix in Origin):
  - `~/.agents/skills/sahil87-toolkit/SKILL.md` — the agentskills.io open-standard path: read natively by **Codex** (USER scope) and compat-read by **Cursor** and **OpenCode**.
  - `~/.claude/skills/sahil87-toolkit/SKILL.md` — **Claude Code** (which does not read `~/.agents/`).
  - Both writes are unconditional — `agent-setup` is an explicit "wire this machine" command, the cost is two small files in `$HOME`, and any future harness adopting the open standard picks up `~/.agents/skills/` automatically. No harness detection, no skip logic. <!-- assumed: unconditional two-location write over detect-and-write — simpler, future-proof; revisit only if stray-dir creation proves objectionable -->
  - Cursor and OpenCode will see the same-name skill from both locations. The bytes are identical, so this is cosmetic at worst; neither harness documents cross-location same-name precedence. Verify at apply; if a harness warns, the fallback is symlinking `~/.claude/skills/sahil87-toolkit` → the `~/.agents` copy (Claude Code explicitly follows and dedupes symlinked skill dirs).
- **Skill content** — thin, static, portable frontmatter (`name` + `description` ONLY — the OpenCode-recognized common subset; name `sahil87-toolkit` satisfies the shared `^[a-z0-9]+(-[a-z0-9]+)*$`/match-directory rule). The body teaches the two-step; the description front-loads trigger words (tool names) for implicit activation. Sketch:

```markdown
---
name: sahil87-toolkit
description: Use when driving any sahil87 toolkit CLI — wt, idea, tu, run-kit (rk), hop, fab-kit, or shll itself. Run `shll skill` to list the installed tools; run `shll skill <tool>` for that tool's full usage bundle before using it.
---
# sahil87 toolkit

This machine has the sahil87 toolkit installed. Before driving one of its tools:

1. `shll skill` — the installed tools, one line each
2. `shll skill <tool>` — that tool's full agent skill bundle (when to use it,
   composition patterns, output and exit-code contracts, gotchas)

For toolkit-repo development, `shll standards` enumerates the binding CLI standards.
```

  The content stays version-locked in spirit: the skill only *points at* the runtime two-step, so bundles are always fetched from the installed binaries; the placed file itself is written by the installed shll and refreshed on any re-run. Canonical source is a Go string constant in `agent_setup.go` (it is neither a published standard nor a `<tool> skill` bundle, so the docs-site sync/drift-guard ceremony does not apply). <!-- assumed: inline constant over a docs/site canonical file — the bootstrap skill is an artifact of agent-setup, not a published document; revisit if it should render on shll.ai -->
- **UX**: `--print` (show the SKILL.md content and target paths without writing), `--uninstall` (remove both placed skill directories), and a written/updated/unchanged summary per path on the default run. **No diff-and-confirm and no `--yes` gate** — those existed to protect user-authored files; overwriting shll's own skill files needs neither.
- **Delegation**: after placement, invoke `run-kit agent-setup` as a subprocess for run-kit's harness hooks (Constitution III/IV — run-kit agent-setup keeps working standalone). run-kit absent → skip silently (Constitution V). `--print`/`--uninstall` runs do not trigger delegation side effects (pass-through of the equivalent run-kit flags settled at apply).

### 5. Coordinated run-kit change (external — NOT in this repo)

run-kit slims `agent-setup` to **hooks-only**: its context-injection stanza (pointing agents at `run-kit context`) is removed and that guidance moves into run-kit's own `run-kit skill` bundle, so agents still learn `run-kit context` via the composed path. With shll's mechanism now skills-placement, the two no longer write the same files — the old double-injection (two stanzas in one context file) cannot occur. What remains is **redundant guidance** (run-kit's stanza + the bootstrap skill both teaching toolkit context) until run-kit's slim lands, plus `shll skill run-kit` degrading until `run-kit skill` ships. Verified not yet landed. Merge here is safe; release coordination is an operator concern (see Open Questions).

### 6. Touchpoint graduation

Both existing pointers to `run-kit agent-setup` become `shll agent-setup`:

- **`install.go`'s post-install "Next steps" nudge** (change `93r2`; `runKitAgentSetupFmt`, `install.go:363`): currently `run-kit agent-setup # optional, once per machine — agent state in run-kit's dashboard`, gated on run-kit being installed after the run. Becomes a `shll agent-setup` nudge (reframed: places the toolkit skill for agent harnesses + installs run-kit dashboard hooks); gating shifts accordingly — shll is by definition present, so gate on "bootstrap skill not yet placed" (a stat of the two target paths), details settled at apply.
- **README install flow** (`README.md:16` command block + `:28` explanation paragraph): same graduation, same reframing.

### 7. Constitution VII justifications (new-subcommand bar)

- `skill` = composition of per-tool commands — the `shell-init` precedent, verbatim the pattern Constitution III/IV exist to bless.
- `agent-setup` = cross-toolkit machine wiring graduating from run-kit, where it was mis-homed; it belongs in the manager, and could not be a flag on any existing subcommand.

### Out of scope / rejected

- **Stanza injection into `~/.claude/CLAUDE.md` / AGENTS.md-family files (the backlog's original §2 mechanism) — REJECTED** in favor of skills placement (user decision, this session): no merge machinery, no user-file edits, cheaper per-session context, native on all four harnesses. The `shell_setup.go` sentinel-machinery reuse falls away with it.
- **Placing per-tool bundles as skill files — rejected**: placed copies go stale between updates and multiply listing lines; the thin bootstrap + runtime two-step keeps bundles version-locked by construction.
- The other six tools' `<tool> skill` implementations (they ride the per-repo `[std1]`/`[std2]` waves; the SEED RULE binds *those* authors, not this change).
- The run-kit slimming change itself (§5 — run-kit repo).
- fab-kit's `_cli-external.md` slimming to delegate to bundles (fab-kit backlog).
- Any change to the `skill` standard's text (a manager-exception note for shll's composer grammar, and its forward-design paragraph describing agent-setup as "aggregating bundles into context", are flagged as a likely small follow-up).

## Affected Memory

- `cli/skill`: (new) the `shll skill` composer — glossary form, per-tool bundle passthrough, self-target embed, degradation contract
- `cli/agent-setup`: (new) the `shll agent-setup` command — bootstrap-skill placement (two-location covering set), harness coverage matrix, run-kit delegation
- `cli/install`: (modify) the "Next steps" nudge graduates from `run-kit agent-setup` to `shll agent-setup`
- `cli/commands`: (modify) root wiring facts change — 13 factory funcs, user-facing surface of twelve, two new command files, their Constitution VII justifications *(added post-review — flagged by the review worker as an omission)*
- `cli/standards-conformance`: (modify) the `skill` standard moves from "deferred to [agst]" to adopted
- `cli/standards-content`: (modify) the skill standard's "forward design" (`shll agent-setup`) is now built — cross-reference update noting the mechanism landed as skills placement, not context aggregation

## Impact

- **New files**: `src/cmd/shll/skill.go` + `skill_test.go`, `src/cmd/shll/agent_setup.go` + `agent_setup_test.go`, `docs/site/skill.md` (shll's own bundle) + its committed embed copy.
- **Modified**: `install.go` + `install_test.go` (nudge graduation), `README.md` (install flow), `scripts/sync-standards.sh` or a sibling (sync shll's own bundle embed), root command wiring, help-dump goldens (`help/shll.json` — command tree changes, so the machine-help contract must be re-verified per the constitution's Toolkit Standards article). `shell_setup.go` is NO LONGER touched (sentinel reuse dropped with the stanza design).
- **Standards check** (constitution-mandated for CLI-surface changes): `principles` (exit codes, stdout/stderr split, TTY gating), `help-dump` (tree changed), `skill` (this change adopts it — run its "Verifying conformance" checklist against `shll skill shll`), `readme-extraction` (README edit stays within head/structure rules).
- **No new dependencies**; subprocess work (`<tool> skill`, `run-kit agent-setup`) through `internal/proc` (Constitution I); skill placement is plain file I/O in shll-owned directories.
- **Cross-repo**: release coordination with run-kit's hooks-only slimming (see Open Questions).

## Open Questions

- Release sequencing: the coordinated run-kit slim (hooks-only agent-setup + `run-kit skill`) has not landed. Merge here is safe; should the shll **release** that ships `agent-setup` wait for run-kit's change so machines never carry both run-kit's context stanza and the bootstrap skill (redundant, not corrupting), and so `shll skill run-kit` works day one? (Operator-level; the backlog says "land BEFORE or WITH".)
- Same-name dedup in Cursor/OpenCode when the skill is visible via both `~/.agents/skills/` and `~/.claude/skills/` (identical bytes — cosmetic at worst; neither documents precedence). Verify at apply; symlink fallback documented in §4.
- Does the `skill` standard's text need a small follow-up (manager-exception note for shll's composer grammar; forward-design paragraph now describing the landed skills-placement mechanism)? Likely a tiny docs change, not this one.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Command names + grammar: `skill` (bare = glossary, `<tool>` = that tool's bundle, mirrors `shll standards`) and `agent-setup` | Decided verbatim in backlog `[agst]`; `skill`-not-`agent` naming settled in the skill standard | S:95 R:70 A:95 D:95 |
| 2 | Certain | Bundles composed at runtime by invoking each installed tool's `<tool> skill` subprocess — never embedding other tools' docs | Backlog-decided; Constitution III; keeps bundles version-locked to the installed binary | S:95 R:75 A:95 D:90 |
| 3 | Certain | Bare form MUST NOT concatenate bundles — two-step (glossary, then per-tool on demand) is the context-economy contract | Backlog-decided explicitly (7×~150 lines violates principle №9) | S:95 R:80 A:95 D:95 |
| 4 | Confident | Glossary = installed tools only, shll-first then Roster (leaves-first) order, one-liners from hardcoded `Description`/`shllSelfDescription`, PATH probe only (no brew), trailing `shll skill <tool>` hint | Backlog says "one line per tool … shll-first row ordering like list/version"; installed-only fits the bootstrap skill's purpose — full-roster-with-status is `shll list`'s job | S:70 R:85 A:80 D:65 |
| 5 | Confident | Capability handling = invoke `<tool> skill` and classify failure (ErrNotFound → not installed; unknown-command/non-zero → predates skill), rather than a separate help-substring probe | The `--skip-brew-update` probe pattern's intent, simplified: `skill` either prints or errors | S:60 R:85 A:75 D:60 |
| 6 | Confident | Miss behavior on `shll skill <tool>`: unknown name → usage exit 2; known-but-missing/unsupported → one-line stderr notice + exit 1 | Conformance exit-code convention (change 3sss); Constitution V's silent skip is for composition, an explicit request earns an informative error | S:45 R:80 A:70 D:55 |
| 7 | Confident | shll's own bundle (docs/site/skill.md + sync/embed/drift-guard + in-process `shll skill shll`) is IN scope | Backlog silent, but the conformance audit parked shll's skill adoption in [agst]; the self-target must be special-cased anyway (subprocess self-invocation would recurse) | S:45 R:70 A:60 D:50 |
| 8 | Certain | agent-setup mechanism = mechanical Agent-Skill placement; NO context-file stanza injection, NO sentinel/merge machinery, NO skip-a-harness degradation | Discussed — user chose skills placement explicitly ("no merge operation, just mechanical placement"; "no degeneration") over the backlog's stanza design | S:95 R:80 A:90 D:95 |
| 9 | Certain | Harness placement paths as per the Origin coverage matrix (CC `~/.claude/skills/`; Codex USER `$HOME/.agents/skills`; Cursor + OpenCode native dirs plus `.claude`/`.agents` compat reads) | Verified 2026-07-18 from each harness's official docs, not from memory or search summaries | S:90 R:85 A:95 D:90 |
| 10 | Confident | Minimal covering set: exactly two unconditional writes — `~/.agents/skills/sahil87-toolkit/` + `~/.claude/skills/sahil87-toolkit/` — no harness detection; Cursor/OpenCode same-name double-visibility accepted as cosmetic (identical bytes; symlink fallback if a harness objects) | Two writes cover all four harnesses (user: no degradation); unconditional is simplest and future-proof (new standard-adopting harnesses ride `~/.agents`) | S:60 R:85 A:80 D:60 |
| 11 | Confident | Skill identity: name `sahil87-toolkit`, portable frontmatter (`name` + `description` only — the OpenCode-recognized common subset), trigger-word-front-loaded description, thin body teaching the two-step + `shll standards` pointer; canonical source = Go constant in `agent_setup.go` | Name satisfies the shared regex/dir-match rule; description drives implicit activation on all four harnesses; the bootstrap skill is an agent-setup artifact, not a published doc | S:55 R:85 A:75 D:55 |
| 12 | Confident | UX: `--print` + `--uninstall` + per-path written/updated/unchanged summary; drop diff-and-confirm and `--yes` | Those gates protected user-authored files; overwriting shll-owned skill files is idempotent and needs neither | S:55 R:85 A:75 D:60 |
| 13 | Confident | Implement + merge now; release coordination with run-kit's slim remains desirable but the failure mode is now redundant guidance, not file corruption (no shared files; delegation skips silently when run-kit absent) | Backlog's "land BEFORE or WITH" was driven by double-injection into one file — that risk fell away with the stanza; sequencing stays an operator concern surfaced in Open Questions | S:80 R:80 A:75 D:65 |

13 assumptions (5 certain, 8 confident, 0 tentative, 0 unresolved).
