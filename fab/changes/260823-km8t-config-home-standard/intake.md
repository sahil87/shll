# Intake: Config-Home Toolkit Standard

**Change**: 260823-km8t-config-home-standard
**Created**: 2026-08-23

## Origin

One-shot `/fab-new km8t` invocation. Backlog entry (fab/backlog.md):

> [km8t] 2026-08-23: New config-home standard: fixed $HOME/.config/<tool-name>/ config path (filepath.Join from $HOME — no XDG_CONFIG_HOME, no os.UserConfigDir; hop's resolve.go pattern), override order defaults < config file < env < CLI flag, env forms only for deployment bootstrap keys, XDG-honoring state dirs allowed only for droppable never-authoritative files; fab-kit's ~/.fab-kit/ is the documented exception. Source: run-kit fab/plans/sahil/26-08-23-config-consolidated.md

The cited source doc lives in the run-kit worktree `nimble-panther` (not yet on run-kit main): `run-kit.worktrees/nimble-panther/fab/plans/sahil/26-08-23-config-consolidated.md`. Its "§ Location & resolution" names this change explicitly as the companion deliverable: *"shll `config-home` standard (companion deliverable, shll repo): fixed `$HOME/.config/<tool-name>/` config path (full tool name, `filepath.Join`, no XDG env, no UserConfigDir); override order below; env forms restricted to deployment bootstrap keys; XDG-honoring state gated on droppability; fab-kit listed as the exception. None of the eight existing standards covers this."*

The policy arrives pre-decided (settled in the 2026-08-23 run-kit config brainstorm; record: the "Config Sanitization Board" artifact referenced in the source doc). The intake's job is codification — where the standard lives and how it hooks into the existing standards set — following the exact playbook of the eighth standard's addition (`260720-w6ay-install-composition-standard`).

## Why

1. **The pain point.** The toolkit has no stated standard for where a tool's configuration lives or how override layers stack. run-kit's config audit (2026-08-22/23, triggered by the RK_AUTO_NAME misstep on its PR #711) found config scattered across seven surfaces with no precedence rule — the wrong home (an env var) was reachable faster than the right one. Meanwhile two tools (`hop`, `idea`) independently converged on the same correct pattern — a fixed `$HOME/.config/<tool>/` path built with `filepath.Join`, deliberately ignoring `$XDG_CONFIG_HOME` and `os.UserConfigDir` — with near-identical code comments justifying it, but nothing binds the next tool to it.
2. **The consequence of not fixing it.** run-kit is about to build its config root against this pattern (consolidated plan phase 1), and future tools will grow config files. Without a binding standard, each tool re-litigates XDG-vs-fixed, env-vs-file precedence, and state-vs-config boundaries — and the divergence is user-facing (a dotfiles user must chase per-tool locations; an agent driving a daemon+CLI tool can read a different config than the daemon did when an XDG env var differs between process contexts). The constitution's Toolkit Standards clause makes `docs/site/standards/` the enforcement surface; an undocumented policy is not binding.
3. **Why this approach.** Same as the seven existing companion standards: a new mechanical-contract page in `docs/site/standards/` (canonical, rendered on shll.ai, served offline via `shll standards`), registered in the `standardsRoster` and the sync/embed pipeline. The determinism rationale is hop's, verbatim: the config path must be identical on every platform and in every process context *by construction* — a daemon, a CLI invocation, and an agent-in-a-pane must provably read the same file.

## What Changes

### 1. New standard document: `docs/site/standards/config-home.md`

A new mechanical-contract page following the established shape (`# Standard: config-home` title; producer-facing intro naming the [toolkit CLI principles](principles.md) it implements and its scope; MUST/SHOULD obligation sections; closing `## Verifying conformance` checklist). Content plan — concise, sectioned:

- **Intro**: where a toolkit tool's configuration lives and how override layers stack. Producer-facing standard; scope `binary` (obligations are satisfied by the compiled tool's runtime path resolution). Principle mapping proposal: №6 (stateless, therefore retry-safe — the deterministic-by-construction path is the config-side face of "same invocation, same behavior") and №4 (fail fast — the unset-`$HOME` error is actionable, never a silent fallback); finalize at plan time.
- **Section: One fixed config root (MUST)**:
  - A tool that has a config file MUST resolve it under `$HOME/.config/<tool-name>/`, built with `filepath.Join` from `$HOME` (the only environment input, unavoidable).
  - `<tool-name>` is the **full tool name** (`run-kit`, not `rk`) — matching the `run-kit-desktop` precedent.
  - MUST NOT honor `$XDG_CONFIG_HOME` and MUST NOT use `os.UserConfigDir` (which resolves to `~/Library/Application Support` on macOS). The path is identical on every platform and in every process context by construction.
  - **Rationale, stated in the standard**: determinism. For a tool that is simultaneously a daemon, a CLI, and agent-driven (run-kit is the motivating case), an env var that differs between those contexts silently forks which config is read. Dotfiles users symlink the directory instead.
  - Unset `$HOME` → actionable error (hop: `"hop: $HOME is not set; cannot locate config"`).
  - SHOULD pin the fixed path with a test asserting env vars cannot move it (hop has one).
- **Section: Override order (MUST)**: code defaults < config file < env < CLI flag. One cascade, no per-key exceptions (run-kit's `RK_SSH_HOST` file-wins-over-env case is being removed precisely to keep this rule exception-free).
- **Section: Env vars are deployment bootstrap only (MUST)**: env forms exist ONLY for deployment bootstrap keys — values needed at or before process start, per-deployment (e.g. run-kit's `RK_PORT`, `RK_HOST`). Env is never an override channel for preference keys (the RK_AUTO_NAME failure mode, named). This is what makes the env layer's position in the cascade safe.
- **Section: State is not config — the XDG asymmetry (MAY, bounded)**: XDG-honoring state dirs (`$XDG_STATE_HOME/<tool-name>/`) are allowed ONLY for droppable, never-authoritative files (caches, snapshots, recovery backups) — an env mismatch there cannot fork behavior, which is exactly why config gets no such latitude. The asymmetry is deliberate and this section is where it is documented.
- **Section: The fab-kit exception**: `~/.fab-kit/` (config co-located with its version cache) is the documented exception to the fixed-`~/.config` pattern — grandfathered per fab-kit's own decision record ("decision 5; XDG rejected"). The exception list is closed: new tools get no exception.
- **Conformance receipts** (verified 2026-08-23): `hop` is the reference implementation (`src/internal/config/resolve.go` — fixed `$HOME/.config/hop/hop.yaml`, env-immovability test); `idea` conforms (`systemConfigDir` → `~/.config/idea`, "$XDG_CONFIG_HOME is intentionally ignored"); `run-kit` adopts in its config-consolidation phase 1 (`$HOME/.config/run-kit/`); `wt`/`tu` have no config file today and are bound when they grow one; `fab-kit` is the documented exception.
- **`## Verifying conformance`** checklist: config path is built with `filepath.Join` from `$HOME`; no `$XDG_CONFIG_HOME`/`os.UserConfigDir` reference on the config path; directory uses the full tool name; override order matches the cascade; every env-var key is deployment-bootstrap-shaped; any XDG state dir holds only droppable files; a test pins the fixed path.

### 2. Standards index updates: `docs/site/standards/principles.md`

Three places reference the companion set (same three as the install-composition addition):

- The "Seven companion standards" intro paragraph — becomes eight. `config-home` fits neither existing category sentence ("Three cover documentation and help… Four cover how `shll` composes the toolkit"), so the enumeration gains a third clause, e.g. "One covers per-tool configuration: [config-home](config-home.md) (the fixed config root and override cascade)".
- The "The contracts" table — new row: `config-home` | Implements (per the principle mapping above) | scope `binary` | one-line "what it standardizes".
- The closing "Consuming these standards" paragraph — the parenthesized companion list gains `config-home`.

No changes to the ten principle sections themselves.

### 3. Embed wiring (mechanically required by this repo's architecture)

- `scripts/sync-standards.sh`: append `config-home` to the `STANDARDS=(…)` array (line 16).
- `src/cmd/shll/standards.go`: append a `standardsRoster` entry — `Name: "config-home"`, one-line `Description` (e.g. "Fixed $HOME/.config/<tool>/ config root, override cascade, env restricted to deployment bootstrap keys"), `Scope: "binary"`, `SourcePath: "docs/site/standards/config-home.md"`, `EmbedName: "config-home.md"`.
- Run `scripts/sync-standards.sh` and commit the refreshed embed copies (`src/cmd/shll/standards/config-home.md` new, `principles.md` copy updated).
- All `standards_test.go` tests are roster-driven (list, JSON, byte-identical reader, `TestStandardsEmbedMatchesCanonical` drift guard) — they pick up the new entry with no test-code changes; `binary` is already in the pinned scope vocabulary.

### Out of scope

- run-kit's actual config relocation/registry work — that is the consolidated plan's phases 1–4, in the run-kit repo; this standard is its companion deliverable, not its implementation.
- Migrating `fab-kit` off `~/.fab-kit/` — it is the documented exception, by decision.
- Retrofitting `wt`/`tu` — they have no config file; the standard binds them when they grow one.
- README.md reference-list changes — the precedent addition (install-composition) did not add a README bullet and the current list intentionally ends at shell-init.
- shll binary behavior — nothing behavioral changes beyond the embed roster (shll itself has no config file, so the standard is N/A-conformant for shll by construction).

## Affected Memory

- `cli/standards-content`: (modify) the standards-documents memory gains the ninth standard — the fixed-root obligation, the cascade, the env-bootstrap restriction, the XDG-state asymmetry, the fab-kit exception, and the naming choice (`config-home`)
- `cli/standards`: (modify) `shll standards` roster/embed/sync-script gain a ninth entry; the drift guard's "all eight standards" phrasing becomes nine
- `cli/standards-conformance`: (modify) conformance state extends to the new standard (shll has no config file — conformant by construction/N-A; note hop/idea as the toolkit's conforming implementations)

## Impact

- `docs/site/standards/config-home.md` — new (the deliverable)
- `docs/site/standards/principles.md` — intro enumeration, contracts table, consuming-list
- `scripts/sync-standards.sh` — one array element
- `src/cmd/shll/standards.go` — one roster entry
- `src/cmd/shll/standards/` — refreshed embed copies (new file + principles.md)
- Tests: `go test ./...` in `src/` — roster-driven standards tests stay green; drift guard requires the sync to have run
- Renders on shll.ai at `/shll/standards/config-home` via the existing pull pipeline; readable offline via `shll standards config-home`

## Open Questions

*(none — the policy is pre-decided by the source doc; the registration playbook is fixed by precedent)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Standard named `config-home` at `docs/site/standards/config-home.md`, registered via roster + sync array + embed | Backlog and source doc name it; registration surfaces are mechanical, fixed by the install-composition precedent (260720-w6ay) | S:90 R:85 A:95 D:95 |
| 2 | Certain | Obligation content taken verbatim from the source doc: fixed `$HOME/.config/<tool-name>/` via `filepath.Join` (no XDG env, no `os.UserConfigDir`, full tool name), cascade `defaults < config file < env < CLI flag`, env = deployment bootstrap keys only, XDG state gated on droppability, fab-kit `~/.fab-kit/` the closed exception | Source doc (26-08-23-config-consolidated.md § Location & resolution) settles every value; backlog entry restates them | S:95 R:80 A:95 D:95 |
| 3 | Certain | Conformance receipts: hop reference implementation (`resolve.go` + env-immovability test), idea conforms (`systemConfigDir`), run-kit adopting (phase 1), wt/tu no config today, fab-kit exception | Verified by reading hop's resolve.go and grepping wt/tu/idea internals 2026-08-23 | S:70 R:85 A:85 D:80 |
| 4 | Confident | Scope classified `binary` — obligations are satisfied by the compiled tool's runtime path resolution; no repo-structure half | Matches the scope vocabulary's definition; the SHOULD-level pin test doesn't create a repo scope half (update/version are `binary` with tests too) | S:60 R:90 A:75 D:70 |
| 5 | Confident | Principle mapping: implements №6 (stateless/deterministic) and №4 (fail fast on unset `$HOME`) | Multiple defensible mappings; №6's determinism is the standard's own stated rationale — plan finalizes the exact Implements cell | S:45 R:85 A:60 D:45 |
| 6 | Confident | README.md reference list untouched | install-composition precedent added no README bullet; current list ends at shell-init — consistent, not drift | S:55 R:90 A:75 D:70 |
| 7 | Certain | `change_type` overridden to `docs` | Direct precedent: 260720-w6ay (same shape of change) is explicitly `docs`; keyword inference gave `feat` | S:80 R:95 A:95 D:90 |

7 assumptions (4 certain, 3 confident, 0 tentative, 0 unresolved).
