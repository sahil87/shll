# Intake: Skill Topic Passthrough

**Change**: 260718-tp2s-skill-topic-passthrough
**Created**: 2026-07-18

## Origin

Backlog item `[tp2s]` (2026-07-18), one-shot `/fab-new tp2s`:

> Support `shll skill <tool> <topic>` passthrough in the skill composer. PROBLEM: run-kit ships a topic page per the skill standard's topic-pages section (`rk skill display`, run-kit PR #386), but `shll skill run-kit display` fails with "accepts at most 1 arg(s), received 2" (usage error, exit 2) — the composer accepts exactly one arg. The placed bootstrap skill teaches `shll skill <tool>` as the route to depth; topic pages are then reachable only by switching to the tool's own binary (`rk skill display`) — the core bundle's topic index does name that command, so this is a uniformity gap in the shll front door, not a dead end, but if shll is the documented route the route should reach topics too. IMPLEMENTATION: accept an optional second arg and delegate to `<tool> skill <topic>` verbatim, preserving the standard's topic contract (raw markdown to stdout; stderr empty on success; unknown topic → non-zero exit with the valid topics on stderr — propagate, do not swallow or rewrap). Before extending, check the composer's documented grammar in docs/memory/cli/skill.md and the manager-exception/composer-grammar note flagged in fab/changes/260718-agst-agent-setup-skill-commands/intake.md (~line 124) so the standard text and the composer stay consistent; update the bootstrap body's step 2 and the `shll skill` trailer line ("Run 'shll skill <tool>' for…") to mention the topic form. TESTS: passthrough happy path (stub tool), unknown-topic stderr/exit propagation, arg-count contract (>2 args still exit 2).

Pre-intake consistency checks performed (per the backlog's directive):

- `docs/memory/cli/skill.md` documents the composer grammar as two shapes on `cobra.MaximumNArgs(1)` — bare glossary and one-arg bundle. This change adds the third shape; that memory file is an Affected Memory entry below.
- The `agst` intake (~line 124) flags "a manager-exception note for shll's composer grammar" in the `skill` standard's text as a **likely small follow-up** — explicitly out of scope there. Verified: `docs/site/standards/skill.md` contains **zero** references to the composer/manager today, so extending the composer grammar creates no new inconsistency with the standard's text. The manager-exception note remains that separate flagged follow-up (see Assumptions #6).

## Why

1. **Pain point**: the toolkit's `skill` standard defines topic pages (`<tool> skill <topic>`) as the depth mechanism for large-scope tools, and run-kit already ships one (`rk skill display`, run-kit PR #386). But the shll front door — the route the placed bootstrap skill actually teaches agents — dead-ends at one arg: `shll skill run-kit display` exits 2 with a cobra arg-count usage error.
2. **Consequence if unfixed**: agents following the documented route (`shll skill <tool>`) must switch binaries mid-flow (`rk skill display`) to reach depth. The core bundle's topic index does name the tool-native command, so it is a uniformity gap rather than a hard dead end — but every tool that ships topics widens the gap, and the bootstrap's promise ("shll is the route to depth") quietly stops being true.
3. **Why passthrough over alternatives**: delegation to `<tool> skill <topic>` verbatim is the same composition pattern the one-arg form already uses (Constitution III — Wrap, Don't Reinvent; IV — Composition, Not Replacement). shll gains no topic knowledge of its own: topic validity, valid-topic listings, and page content all stay owned by the tool that ships them. Re-implementing a topic registry in shll was never on the table — it would violate Constitution III and the standard's ownership model (topics are canonical at each tool's `docs/site/skill/<topic>.md`).

## What Changes

### 1. Composer grammar: `skill [tool] [topic]` (`src/cmd/shll/skill.go`)

- `newSkillCmd()`: `Use: "skill [tool] [topic]"`, `Args: cobra.MaximumNArgs(2)` (from `MaximumNArgs(1)`). Three or more args remains a cobra usage error → exit 2 via the existing `errExitCode` wrap (root `SetFlagErrorFunc`/args handling) — the arg-count contract shifts from >1 to >2, nothing else.
- A topic with no tool (`shll skill <topic>` — one arg) is indistinguishable from a tool name and stays what it is today: the one arg is resolved as a tool name; an unknown name is the existing usage exit 2 with the valid-tools list. No grammar ambiguity is introduced.
- `runSkill(ctx, stdout, stderr, args)` dispatch gains the third shape:
  - `len(args) == 0` → glossary (unchanged)
  - `len(args) == 1` → `writeSkillBundle` (unchanged)
  - `len(args) == 2` → topic passthrough (new — `writeSkillTopic` or an extended `writeSkillBundle` taking an optional topic; naming per existing file conventions)

### 2. Topic passthrough behavior (two-arg form)

Resolution order mirrors the one-arg form:

1. **Legacy alias first**: `shll skill rk <topic>` → the `legacyAliases` rewrite applies to the tool arg before any dispatch (so it invokes `run-kit skill <topic>`, never `rk skill <topic>`). No bespoke alias logic — same shared map.
2. **`shll` self-token**: `shll skill shll <topic>` — shll ships zero topic pages today. Served in-process per the standard's unknown-topic contract: one stderr line stating shll ships no topic pages, usage exit 2 (matching the unknown-tool convention; the standard requires only "non-zero exit, valid topics on stderr" — the valid set is empty). No subprocess (a self-invocation would recurse into the composer).
3. **Unknown tool name** → existing usage exit 2 with `validTargets(true)` — unchanged, and checked before any subprocess.
4. **Roster tool** → `proc.RunCaptured(subCtx, tool.Name, skillSubcommand, topic)` under the existing `skillProbeTimeout` (2s — topic pages are static bundles, same bound as the core). On success (`code == 0`): captured stdout written byte-identical, stderr empty — the standard's invocation contract flows through untouched.

**Failure classification for the two-arg form** (this is where it deliberately diverges from the one-arg form):

- `proc.ErrNotFound` (tool not on PATH) → existing `skillNotInstalledFmt` one-liner, exit 1. Classified before any exit-code question arises — unchanged.
- Pre-start I/O failure (`err != nil`, no usable exit code) → existing `shll skill: <tool>: <err>` one-liner, exit 1 — unchanged.
- **`code != 0` → propagate, do not swallow or rewrap** (the backlog's explicit contract): write the child's captured stderr bytes through to shll's stderr verbatim, and exit with the **child's own exit code** via `errExitCode{code: childCode}` (empty msg — the child's stderr already said everything). This preserves the standard's unknown-topic contract end-to-end: "non-zero exit with the valid topics on stderr" survives the composer unmodified.
- Accepted consequence: a tool whose installed version predates `skill`, when invoked **with a topic**, surfaces its own raw unknown-command stderr instead of the curated `skillUnsupportedFmt` notice. The two failure shapes (predates-skill vs. unknown-topic) are not reliably distinguishable from exit codes, and stderr-sniffing is fragile — verbatim propagation is the honest behavior and what the backlog mandates. The one-arg form keeps its existing suppress-and-rewrap classification exactly as is.

### 3. Teaching-surface updates (the two the backlog names, plus the surfaces that document the grammar)

- **`skillHintLine`** (`skill.go` — the bare-glossary trailer): extend to mention the topic form, e.g.
  `Run 'shll skill <tool>' for that tool's full agent skill bundle ('shll skill <tool> <topic>' for a topic page).`
  Exact wording finalized at apply; must keep the single-line shape (it trails the tabwriter table after a blank line).
- **Bootstrap body step 2** (`agent_setup.go` `agentSkillContent`): extend the step-2 list item so the placed skill teaches that a tool's core bundle carries a topic index and `shll skill <tool> <topic>` serves a topic page. The placed-skill refresh machinery (change #50 — update refresh, doctor staleness) propagates the new body on the next `shll update`; no extra wiring needed.
- **`newSkillCmd` `Long` text** (`skill.go`): document the two-arg form and its exit contract (propagated child stderr/exit code) alongside the existing usage block.
- **shll's own bundle `docs/site/skill.md`**: the grammar line (`shll skill [tool]` — line 27) and the two-step gotcha (line 50) gain the topic form. Requires `scripts/sync-standards.sh` re-run so the embedded copy `src/cmd/shll/skill/skill.md` stays byte-identical (`TestSkillEmbedMatchesCanonical` enforces; the ≤150-line budget test still applies — bundle is 53 lines today, a one-to-two-line growth is safe).
- **NOT updated**: `agentSkillDescription()`'s frontmatter trailer ("run `shll skill <tool>` for that tool's full usage bundle…") — it is single-line activation-trigger vocabulary, not a teaching surface; the body's step 2 is where the topic form belongs (see Assumptions #5).

### 4. Tests (`skill_test.go`, `agent_setup_test.go`)

Backlog-mandated:

- **Passthrough happy path**: fake `proc.Runner` asserting `<tool> skill <topic>` is invoked (correct argv including the topic) and captured stdout is streamed byte-identical, stderr empty, exit 0.
- **Unknown-topic propagation**: fake child exits non-zero (e.g. 2) with valid-topics text on stderr → shll's stderr carries the child's bytes verbatim, shll's exit code mirrors the child's.
- **Arg-count contract**: `shll skill a b c` (3 args) → usage exit 2 (cobra arg validation).

Additional coverage grounding the design:

- Topic form + tool not on PATH → `skillNotInstalledFmt`, exit 1 (classification precedes propagation).
- `shll skill shll <topic>` → in-process error, exit 2, no subprocess spawned.
- `shll skill rk <topic>` → alias resolves; the subprocess argv targets `run-kit`.
- Existing glossary test updated for the new `skillHintLine` text; `TestSkillEmbedMatchesCanonical` + line-budget re-verified against the edited bundle; any `agent_setup_test.go` assertion pinning the bootstrap body text updated.

## Affected Memory

- `cli/skill`: (modify) grammar gains the third shape (`shll skill <tool> <topic>`), the two-arg failure classification (propagate stderr + mirror exit code for `code > 0`, curated exit-1 notice for the `-1` timeout/kill sentinel; divergence from the one-arg suppress-and-rewrap), the new `skillHintLine` text, the `shll skill shll <topic>` no-topics contract
- `cli/agent-setup`: (modify) bootstrap body step 2 now teaches the topic form; note the frontmatter description trailer deliberately unchanged
- `cli/commands`: (modify) the per-subcommand surface row pins `Use: "skill [tool]"` + `cobra.MaximumNArgs(1)` + the suppress-stderr classification — all three falsified by this change *(added in rework cycle 1 — review should-fix: hydrate scope undercount)*
- `internal/proc`: (modify) §TransportCaptureAll / §RunCaptured describe skill as always suppressing the child's stderr — the two-arg form now propagates it; also the deadline-kill → `ExitCode() == -1` behavior clarified *(added in rework cycle 1 — review should-fix)*

## Impact

- **Modified files**: `src/cmd/shll/skill.go` + `skill_test.go` (grammar, passthrough, hint line, Long text), `src/cmd/shll/agent_setup.go` + `agent_setup_test.go` (bootstrap body step 2), `docs/site/skill.md` + `src/cmd/shll/skill/skill.md` (grammar/gotcha lines + synced embed via `scripts/sync-standards.sh`).
- **No new files, no new subcommands** — Constitution VII bar not triggered (arg-grammar extension of an existing subcommand). Subprocess work stays on `internal/proc` (Constitution I); delegation-not-reimplementation per Constitution III/IV; not-installed degradation per Constitution V.
- **Standards check** (constitution-mandated for CLI-surface changes; performed against `docs/site/standards/` at intake):
  - `skill.md` § topic pages — the composer preserves the topic contract end-to-end (raw markdown stdout, stderr empty on success, unknown topic → non-zero + valid topics on stderr, propagated). The standard's text nowhere references the composer, so no standard-text edit is required (the manager-exception note stays the `agst`-flagged separate follow-up).
  - `principles.md` § exit codes — 0/1/2 semantics preserved; the mirrored child exit code keeps usage-vs-operational meaning intact (an unknown topic is a usage error at the child and stays one through the composer).
  - `help-dump.md` — `Use`/`Long` strings change (content-only); the frozen JSON envelope shape is untouched and no local goldens exist (shll.ai pulls on schedule), so no golden re-record.
- **No new dependencies**; `README.md` untouched.

## Open Questions

*(none — the backlog entry specifies grammar, delegation, propagation contract, teaching-surface updates, and tests; residual design calls are graded below)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Grammar: `Args: cobra.MaximumNArgs(2)`, `Use: "skill [tool] [topic]"`; ≥3 args stays usage exit 2 | Backlog specifies the optional second arg and the >2-args contract verbatim; cobra mechanics deterministic | S:90 R:90 A:95 D:95 |
| 2 | Certain | Two-arg form delegates `<tool> skill <topic>` verbatim via `proc.RunCaptured` under the existing `skillProbeTimeout` (2s); `rk`→`run-kit` alias rewrite applies unchanged before dispatch | Backlog mandates verbatim delegation; transport, timeout, and alias reuse follow the one-arg form's established pattern exactly | S:85 R:85 A:95 D:90 |
| 3 | Confident | Non-zero child exit on the two-arg form: propagate captured stderr bytes verbatim + mirror the child's exit code via `errExitCode{code: childCode}`; consequence accepted that a predates-`skill` tool invoked with a topic shows its raw unknown-command error (no reliable way to distinguish from unknown-topic; stderr-sniffing rejected as fragile). `ErrNotFound`/pre-start failures keep their existing curated notices | Backlog says "propagate, do not swallow or rewrap"; mirroring the exit code (vs. flattening to 1) is the design call — most faithful to the standard's contract and keeps usage-vs-operational semantics | S:75 R:80 A:70 D:65 |
| 4 | Confident | `shll skill shll <topic>` → in-process one-line stderr error stating shll ships no topic pages, usage exit 2, no subprocess | shll has zero topics today; standard requires only non-zero + valid topics on stderr; exit 2 matches the sibling unknown-tool usage convention (1 vs 2 both defensible — 2 is the front-runner) | S:55 R:85 A:70 D:60 |
| 5 | Confident | Teaching surfaces updated: `skillHintLine`, bootstrap body step 2, `newSkillCmd` Long, `docs/site/skill.md` (+ embed re-sync); `agentSkillDescription()` frontmatter trailer left unchanged | Backlog names the hint line and body step 2; Long/bundle document the same grammar and must not go stale; the frontmatter description is single-line trigger vocabulary, not a teaching surface | S:70 R:90 A:80 D:70 |
| 6 | Confident | No edit to the `skill` standard's text (`docs/site/standards/skill.md`) in this change | Verified the standard never references the composer, so the two-arg extension creates no inconsistency; the manager-exception note is the `agst` intake's separately-flagged follow-up and stays out of scope | S:65 R:90 A:80 D:70 |

6 assumptions (2 certain, 4 confident, 0 tentative, 0 unresolved).
