# Intake: Standards directory restructure + the `<tool> skill` contract standard

**Change**: 260717-i70w-standards-dir-skill-contract
**Created**: 2026-07-17

## Origin

Promptless dispatch from a live design conversation (synthesized description as source of truth; no questions asked — `{questioning-mode} = promptless-defer`). The conversation reached decided outcomes on every major point; this intake transfers them verbatim.

> Move the toolkit standards into `docs/site/standards/`, author the fourth standard (`skill.md` — the `<tool> skill` contract), and update the `shll standards` command accordingly (new roster entry + scope column).

HEAD context: detached at origin/main = 6ca7bdf, which already contains the docs/site standards pages (#40) and the `shll standards` command (#41, change vo8c).

## Why

1. **Genre separation.** `docs/site/` currently mixes shll's *own* tool docs (`install.md`, `workflows.md`) with *toolkit-wide* standards (`principles.md`, `help-dump.md`, `readme-extraction.md`). A browser of the tree — human or agent — cannot tell that `help-dump.md` is a 7-repo standard rather than a shll feature doc. Moving the standards into `docs/site/standards/` makes the genre boundary structural.

2. **URL mirrors the command.** shll.ai renders nested `docs/site` trees, so the moved pages land at `shll.ai/shll/standards/<name>` — mirroring `shll standards <name>` exactly. The web URL and the CLI invocation become the same name.

3. **Resolves a coming filename collision.** Per the new skill standard (authored by this change), each tool's own bundle will live at `docs/site/skill.md` in that tool's repo. shll is itself a toolkit tool — its own future `docs/site/skill.md` bundle would have collided with the standard *document* if standards stayed flat in `docs/site/`. The subdirectory removes the collision by construction.

4. **The toolkit has a real gap the fourth standard fills.** Nothing today serves the agent *using* an installed tool from any repo, offline: `-h`/help-dump is flag reference, README/docs-site needs the repo or network, and `fab/project` context is repo-development-scoped. A `<tool> skill` bundle — embedded in the binary, versioned with it — closes that gap and is version-locked by construction (the prose can never describe flags the installed binary doesn't have).

If we don't do this now: the standards tree grows flat and ambiguous, the `docs/site/skill.md` collision lands later as a breaking move (URL churn after external links exist), and the skill-bundle rollout across 7 repos starts without a binding contract to conform to.

**Naming rationale (decided; record it).** Rejected filename prefixes for the standards: `principle-*`, `subcommand-*`/`repo-*`, `contract-*` — filenames should be the artifact name; taxonomy is expressed via location (the `standards/` dir) + list metadata (the new scope column). Rejected subcommand name `agent` for the bundle command: collides with `fab agent` (launches an agent session), reads as "run an agent", and run-kit's `agent-*` family means harness wiring. `skill` is collision-free across all 7 command trees and is the anc.dev P8 vocabulary (SKILL.md skill bundles).

## What Changes

### Part A — directory restructure (docs/site/standards/)

Move the three standards pages into a new subdirectory, filenames otherwise unchanged:

- `docs/site/principles.md` → `docs/site/standards/principles.md`
- `docs/site/help-dump.md` → `docs/site/standards/help-dump.md`
- `docs/site/readme-extraction.md` → `docs/site/standards/readme-extraction.md`

shll's own tool docs (`docs/site/install.md`, `docs/site/workflows.md`) stay where they are. Intra-family relative links (principles ↔ help-dump ↔ readme-extraction) survive the move unchanged (same directory). README links must be updated to the new paths (Part E). docs/site closure rules still hold — relative links stay inside `docs/site/` (readme-extraction standard, Conformance rule 1: no `..` escapes).

### Part B — new standard: `docs/site/standards/skill.md` (the `<tool> skill` contract)

Author a fourth producer-facing standard specifying that every toolkit CLI exposes a `<tool> skill` subcommand printing a **stable, one-page markdown skill bundle for agent consumption, versioned with the binary**. Content decisions (all decided):

- **Audience/gap**: serves the agent *using* an installed tool (any repo, offline) — distinct from `-h`/help-dump (flag reference), README/docs-site (needs repo or network), and the repo-development context in `fab/project`. Version-locked by construction: the prose can never describe flags the installed binary doesn't have.
- **Precedent**: `run-kit context` (a.k.a. `rk context`) — 102 lines of agent-optimized markdown, the toolkit's existing prior art. Nuance to state in the standard: `run-kit context` mixes static capability prose with a small **dynamic** Environment header (session/pane/server URL); the `skill` bundle genre is **static-only** (embedded, byte-identical, drift-guarded) — dynamic environment info stays in separate commands like `run-kit context`.
- **Contract shape** (mirror the help-dump standard's structure — invocation contract, rules with teeth, verification section):
  - Command name exactly `skill`.
  - Prints raw markdown to stdout, byte-identical to the repo's canonical `docs/site/skill.md`.
  - stderr empty, exit 0.
  - Content embedded at build with a sync + drift-guard pattern (the one `shll standards` established: committed copies + sync script + drift-guard test).
  - Page renders at `/<tool>/skill` on shll.ai automatically (nested docs/site tree).
- **Genre discipline**: when-to-use, capabilities map, composition patterns, output/exit-code contracts, gotchas. NOT a second README, NOT flag reference (defer to `-h` and the shll.ai commands page). **Hard length budget: ≤150 lines** (principle №9 — agents load this into context every session; bundles must stay cheap, especially since they'll later be aggregated).
- **Name rationale** (record in the standard): `agent` rejected — collides with `fab agent` and reads as "run an agent"; run-kit's `agent-*` family means harness wiring. `skill` is collision-free across all 7 command trees and is the anc.dev P8 vocabulary (SKILL.md skill bundles).
- **Implements principles №3 + №10; scope: binary + repo.**
- **Adoption**: phased per-repo rollout (like help-dump's was); no tool ships `skill` today. This change authors the STANDARD only — implementing `shll skill` itself and the 6 other tools' bundles is explicitly OUT OF SCOPE (per-repo follow-up wave).
- **Forward-design note to include (clearly marked as planned, one short paragraph)**: a planned `shll agent-setup` will graduate from `run-kit agent-setup` — it will aggregate every installed tool's `<tool> skill` output into the agent's context AND delegate run-kit hook installation to `run-kit agent-setup` (whose context-injection part will be removed). This is why bundles must stay small and static.

### Part C — `shll standards` command updates

- **`scripts/sync-standards.sh`**: source paths → `docs/site/standards/*.md` (`SRC_DIR="docs/site/standards"`); now syncs **4** files (`skill` added to the `STANDARDS` array).
- **Roster (`src/cmd/shll/standards.go`)**: `standardsRoster` gains a 4th entry `skill`; the `standard` struct gains a **scope** field. Scope values: `principles`=`foundation`, `help-dump`=`binary`, `readme-extraction`=`repo`, `skill`=`binary+repo`.
- **List output gains a scope column** (still aligned tabwriter, same config as today — minwidth 0, tabwidth 0, padding 2, padchar space, no color):

  ```
  principles         foundation     The ten toolkit CLI principles every tool is built against
  help-dump          binary         Machine-readable help contract every tool must emit
  readme-extraction  repo           README + docs/site structure standard for toolkit repos
  skill              binary+repo    Agent skill bundle standard: docs/site/skill.md served by `<tool> skill`
  ```

- **`--json` gains a `scope` field** (additive — allowed by the toolkit's format-stability rule, principle №2: new fields land as optional); `source_path` values update to `docs/site/standards/<name>.md`.
- **Drift-guard test** (`TestStandardsEmbedMatchesCanonical`) now covers 4 files at the new paths. Other roster/reader tests (`TestStandardsRosterIntegrity`, list/JSON/reader tests) updated for the 4-entry roster, new paths, and the scope column/field. Note: `TestStandardsRosterIntegrity`'s `strings.HasPrefix(s.SourcePath, "docs/site/")` check still passes at the new paths; whether to tighten it to `docs/site/standards/` is apply's call.
- **Embedded copies** live under `src/cmd/shll/standards/` as today (filenames unchanged flat there, or mirrored into a subdir — **apply's call**; the drift guard is the contract, per the originating conversation).

### Part D — principles.md content updates

(Applied to the moved file `docs/site/standards/principles.md`.)

- **Companions paragraph** (currently: "Two companion standards make principles №3 and №10 concrete…"): now names **three** contracts incl. skill, at new relative links (same-directory: `help-dump.md`, `readme-extraction.md`, `skill.md`).
- **New short section "The contracts"** after the summary table: lists each mechanical contract under the principle(s) it implements (№3 → help-dump, skill; №10 → readme-extraction, skill), stating the two-tier structure (foundation vs mechanical contracts) and the scope vocabulary (binary/repo).
- **№3 and №10** gain a sentence referencing the skill contract where natural (№10's enforcement note can mention `<tool> skill` as SHOULD-phased).
- **"Consuming these standards" section**: URLs update to `/shll/standards/…` (and the companions list gains skill).

### Part E — README updates

- **Reference-section links** → `docs/site/standards/<name>.md` (+ add a skill line describing the new standard).
- **`### shll standards` section**: update the example output to the 4-row scope-column form (block above); the `source_path` mention updates to `docs/site/standards/<name>.md`.

### Out of scope (explicit)

- Implementing `shll skill` or any tool's bundle content (per-repo wave).
- `shll agent-setup` (future graduation change).
- The shll.ai banner-URL follow-up PR (separate repo; handled outside this change).
- The 6-repo constitution amendments (separate wave; they'll cite the new URLs).

## Affected Memory

- `cli/standards`: (modify) — new source dir `docs/site/standards/`, 4-entry roster, scope field/column, the skill standard, updated sync script + drift-guard paths.
- `cli/commands`: (modify) — only if the roster description line / standards row summary changes there (apply's judgment); no subcommand-surface change (still ten user-facing subcommands).

## Impact

- **Docs**: 3 file moves within `docs/site/` (+ content updates to principles.md), 1 new file `docs/site/standards/skill.md`, README.md link + example updates.
- **Code**: `src/cmd/shll/standards.go` (roster entry, scope field, table + JSON renderers), `src/cmd/shll/standards_test.go` (4 files, new paths, scope column/JSON), `scripts/sync-standards.sh` (new source dir, 4th file), embedded copies under `src/cmd/shll/standards/` (re-synced: 3 refreshed + 1 new).
- **Constitution/quality notes**: no new top-level subcommand (Constitution VII not triggered; `standards` already justified — the roster grows a row, exactly the designed extension path). No subprocess changes (`internal/proc` untouched — Constitution I vacuous). `--json` change is additive-only (format-stability rule). `help-dump` output unaffected (no command-tree change beyond help text if the Long/description mentions counts — none does).
- **Tests**: `standards_test.go` updated for 4 files, new paths, scope column/JSON; drift guard remains the enforcement seam. Existing CI PR workflow (build, vet, test) covers everything — no workflow edit.
- **External**: shll.ai picks up the moved/new pages via its existing daily mirror-and-prune pull; old flat URLs stop existing (banner/redirect handling is the separate shll.ai follow-up, out of scope here).

## Open Questions

None. Every decision point was resolved in the originating conversation; two low-stakes implementation details are explicitly delegated to apply (embed-dir layout under `src/cmd/shll/standards/`; whether `cli/commands` memory needs a touch) and are recorded as assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Move exactly the three standards pages into `docs/site/standards/`, filenames unchanged; shll's own tool docs (`install.md`, `workflows.md`) stay flat | Discussed — decided with three-part rationale (genre separation, URL mirrors command, resolves the coming `docs/site/skill.md` collision); prefix-name alternatives explicitly rejected | S:95 R:70 A:90 D:95 |
| 2 | Certain | The fourth standard is named `skill` (`<tool> skill` subcommand; `docs/site/standards/skill.md`) | Discussed — `agent` rejected (collides with `fab agent`, reads as "run an agent", run-kit `agent-*` = harness wiring); `skill` collision-free across all 7 trees, anc.dev P8 vocabulary | S:95 R:60 A:85 D:90 |
| 3 | Certain | This change authors the standard only — implementing `shll skill` and the 6 other tools' bundles is out of scope (per-repo follow-up wave) | Discussed — explicit scope decision; adoption is phased like help-dump's was | S:95 R:85 A:90 D:95 |
| 4 | Certain | Scope vocabulary and values: `principles`=foundation, `help-dump`=binary, `readme-extraction`=repo, `skill`=binary+repo; list output gains the scope column in the exact 4-row form reproduced in Part C | Discussed — exact output block agreed in conversation | S:95 R:80 A:85 D:90 |
| 5 | Certain | `--json` gains an additive `scope` field; `source_path` values update to `docs/site/standards/<name>.md` | Discussed — additive field explicitly allowed by the toolkit's format-stability rule (principle №2) | S:90 R:75 A:90 D:90 |
| 6 | Certain | skill.md mirrors help-dump.md's structure: `skill` prints raw markdown to stdout byte-identical to canonical `docs/site/skill.md`, stderr empty, exit 0, embedded at build with sync + drift guard, ≤150-line hard budget, static-only genre (dynamic env info stays in commands like `run-kit context`), implements №3 + №10, includes the clearly-marked planned `shll agent-setup` paragraph | Discussed — every listed content decision made in conversation, incl. the run-kit context static/dynamic nuance | S:90 R:70 A:85 D:85 |
| 7 | Certain | principles.md updates: companions paragraph names three contracts at same-directory links; new "The contracts" section after the summary table (№3 → help-dump, skill; №10 → readme-extraction, skill; foundation-vs-mechanical two-tier + binary/repo scope vocabulary); №3/№10 gain a natural skill sentence; "Consuming these standards" URLs → `/shll/standards/…` | Discussed — section-level content decided; exact prose is apply's normal work | S:90 R:75 A:80 D:85 |
| 8 | Certain | No new top-level subcommand — Constitution VII not triggered (`standards` already justified; the roster row is the designed extension path); `internal/proc` untouched | Constitution + memory (`cli/standards` documents "adding a standard is a roster row + canonical file") give a deterministic answer | S:90 R:85 A:95 D:90 |
| 9 | Confident | Embedded-copy layout under `src/cmd/shll/standards/` (flat filenames vs mirrored subdir) is left to apply; the drift guard is the contract | Discussed — explicitly delegated ("apply's call") in the conversation; low stakes, easily changed, either layout satisfies the drift guard | S:70 R:90 A:80 D:60 |
| 10 | Confident | `cli/commands` memory is touched only if its standards roster description line changes (apply's judgment); `cli/standards` is the primary memory update | Discussed — flagged as "possibly touch"; hydrate-time judgment call with trivial reversal cost | S:70 R:90 A:80 D:70 |
| 11 | Confident | shll.ai renders nested docs/site trees so the moved pages land at `shll.ai/shll/standards/<name>` with no shll.ai-side change in this repo; old-URL banner/redirect handling is the separate shll.ai follow-up | Stated as established fact in the conversation (rationale #2 for the move); external-repo behavior not re-verified here, and the follow-up PR is explicitly out of scope | S:85 R:60 A:60 D:80 |
| 12 | Confident | `TestStandardsRosterIntegrity`'s `docs/site/` SourcePath prefix check remains valid at the new paths; tightening it to `docs/site/standards/` is apply's call | Verified against the live test (`strings.HasPrefix(s.SourcePath, "docs/site/")` — new paths still match); tightening is a reversible test-strictness detail | S:65 R:90 A:85 D:65 |
| 13 | Confident | Change type is `feat` — the change adds a new standard + command surface behavior (scope column/JSON field, 4th roster entry) alongside the docs restructure; not docs-only | Mixed docs+code change; keyword inference could plausibly land elsewhere, verified/overridden at Step 6 per procedure | S:60 R:90 A:75 D:65 |

13 assumptions (8 certain, 5 confident, 0 tentative, 0 unresolved).
