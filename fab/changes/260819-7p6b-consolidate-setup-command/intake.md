# Intake: Consolidate setup commands into `shll setup`

**Change**: 260819-7p6b-consolidate-setup-command
**Created**: 2026-08-20

## Origin

Promptless dispatch (`/fab-proceed` create-new, `{questioning-mode} = promptless-defer`) from a synthesized user-conversation description. The conversation settled the command shape, the compat constraint, and the docs/standards obligations; unsettled points are recorded as Deferred Unresolved rows in `## Assumptions`.

> Consolidate `shll agent-setup` and `shll shell-setup` into a single rerunnable `shll setup` command family, with the old spellings kept as hidden deprecated commands for one release cycle.

Key decisions from the conversation (verbatim intent):

1. **Subcommands, not flags**: `shll setup` (runs both halves), `shll setup shell [shell]`, `shll setup agent`. The two halves have disjoint flag sets (`--rc-file`/positional shell vs `--yes`), so a flag-based union on one command would be awkward; matches run-kit's noun-verb precedent (`run-kit agent setup`, renamed from `agent-setup` in run-kit PR #620).
2. **Old spellings survive as hidden deprecated top-level commands for ≥1 release** — a hard compat constraint (see Why), not politeness. Cobra aliases cannot relocate a command under a new parent, so the old spellings are hidden top-level commands delegating to the same internals. `shell-setup`'s existing `shell-install` alias is carried on the hidden deprecated command.
3. **`scripts/install.sh` needs NO change**: it only does `exec shll install "$@"`, and `shll install` runs the setups in-process via `runPostInstallSetup` → `runShellSetupDefault` / `runAgentSetup` (Go function calls, no subprocess). The new cobra commands are thin CLI faces over the same internal functions.
4. **Sweep human-facing pointer strings** naming the old commands (doctor hints, install help/nudges, Long help, README, shll.ai docs-site pages).
5. **Standards check is mandatory** (Constitution "Toolkit Standards" clause) — performed at intake; findings below.
6. **Prompting semantics unchanged**: bare `shll setup` run interactively lets run-kit's hook-wiring prompt fire; `--yes` remains for unattended runs (forwarded to the run-kit delegation).
7. **Install opt-out flags `--no-shell-setup` / `--no-agent-setup` stay as-is.**

## Why

1. **Pain point — discoverability of the re-run entry point.** The two setup commands are idempotent and auto-run by `shll install` (PR #82, change gjhx), but as *re-runnable* entry points they are hard to rediscover: a user who installs a new agent harness (or a new shell) later doesn't know which of the two hyphenated commands to re-run. One memorable `shll setup` is a better story.
2. **Consequence of not fixing.** Users fall back to re-running `shll install` (heavier, brew-touching) or hunting through `shll --help` for the right hyphenated command; the setup surface stays two commands where one would do.
3. **Why this approach.** Consolidation *reduces* top-level surface, which Constitution VII favors — **Constitution VII justification: the new `setup` subcommand replaces two existing top-level commands (`agent-setup`, `shell-setup`), net −1 visible top-level command.** Subcommands beat a flag union because the halves have disjoint flag sets, and the noun-verb shape matches run-kit's own `agent setup` precedent.
4. **Why the old spellings must survive one release (hard constraint).** `shll update`'s end-of-run agent-skill self-refresh runs a *subprocess*: `refreshArgv` (`src/cmd/shll/agent_setup.go` ~line 450) has the OLD running binary composing the argv `shll agent-setup [--yes]` and executing it against the NEW binary on PATH after the brew self-upgrade. The new binary must therefore still accept the old spelling, or every `shll update` crossing this release boundary breaks its refresh step. In the new release, `refreshArgv` flips to emit `shll setup agent [--yes]`.

## What Changes

### 1. New `shll setup` command family (`src/cmd/shll/` — likely a new `setup.go` plus edits to `shell_setup.go`/`agent_setup.go`)

- `shll setup` — parent command that is itself runnable: executes the shell half then the agent half (the same order as install's `runPostInstallSetup`). Supports **only** `--yes`/`-y`, forwarded to the agent half's run-kit delegation — no composite `--print`/`--uninstall` modes, no `[shell]` positional; those live on the subcommands. <!-- clarified: bare `shll setup` surface is --yes only — user confirmed the minimal-surface default -->
- `shll setup shell [shell]` — keeps `shell-setup`'s full surface: `[shell]` positional, `--print`, `--uninstall`, `--rc-file`.
- `shll setup agent` — keeps `agent-setup`'s full surface: `--print`, `--uninstall`, `--yes`/`-y`.
- All three are thin cobra faces over the existing internals (`runShellSetupDefault`, `runAgentSetup`, and the print/uninstall paths) — no logic moves, matching how `install.go` already calls those functions in-process (`src/cmd/shll/install.go` ~lines 407, 430).
- Bare `shll setup` failure semantics: runs both halves, exits non-zero if either fails (worst-wins per the toolkit exit-code convention in `docs/site/standards/principles.md`) — unlike install's warn-and-continue, which exists because setup failures must not fail an install.
- Interactive prompting semantics are today's standalone behavior verbatim: bare interactive run lets run-kit's hook-wiring prompt fire; `--yes` for unattended.

### 2. Hidden deprecated old spellings (`shell_setup.go`, `agent_setup.go`, `root.go`)

- `shll shell-setup` (with its `shell-install` alias) and `shll agent-setup` remain registered top-level commands, marked `Hidden: true`, delegating to the same internals with identical flags — for ≥1 release cycle.
- No cobra `Deprecated:` message / stderr warning on the old spellings: silent delegation (Assumptions #9 — backlog iags showed deprecation warnings leaking through the update refresh are a UX bug the user actively fixed).
- Removal of the old spellings is a *future* change after one release cycle — out of scope here.

### 3. `refreshArgv` flip (`src/cmd/shll/agent_setup.go` ~line 450, plus `update.go` preview text)

- `refreshArgv(yes)` changes from `[shll, agent-setup, (--yes)]` to `[shll, setup, agent, (--yes)]`. It is the single source of truth shared by the live refresh subprocess and `shll update`'s dry-run preview line, so both flip together.
- `agentSetupSub` constant (`agent_setup.go` line 124) and its comments update accordingly.
- `update.go`'s Long help and `updateYesUsage` prose naming `shll agent-setup` update to the new spelling.

### 4. Pointer-string sweep (human-facing text naming the old commands)

Verified occurrences to update to the new spellings:

- `src/cmd/shll/doctor.go`: `suggestNotWired` ("run 'shll shell-setup' then 'exec $SHELL'"), `suggestShellUnresolvableFmt` ("…run 'shll shell-setup zsh'"), `suggestCorruptBlock`, `suggestSkillStale` ("run 'shll agent-setup'"), and doctor's Long help prose.
- `src/cmd/shll/install.go`: Long help (lines ~45–52 describe the auto-run steps by old names), `shellSetupNudgeFmt`, `agentSetupNudgeFmt` (post-install nudge lines, ~333–334). Flags `--no-shell-setup`/`--no-agent-setup` **stay** (decision 7).
- `src/cmd/shll/uninstall.go`: `shellUnwireHint` ("run 'shll shell-setup --uninstall'…", line 68).
- `src/cmd/shll/shell_setup.go` / `agent_setup.go`: Long help moves to the new commands; hidden old spellings can carry a one-line "renamed to `shll setup …`" Short/Long.
- Comment-only mentions (e.g. `tools.go` lines 58/67) updated opportunistically; not behavior.

### 5. Tests

- `shell_setup_test.go`, `agent_setup_test.go`, `install_test.go`, `doctor_test.go`, `uninstall_test.go`, `update_test.go` — follow the string/argv changes; add coverage that the hidden old spellings still dispatch (the compat contract) and that `shll setup agent --yes` is the new refresh argv.
- `help_dump_test.go` (~line 270): its aliased-node example asserts on the *visible* `shell-setup` node carrying `[shell-install]` — hiding `shell-setup` prunes it from the dump (the help-dump standard filters `Hidden`), so the test must be adapted (different subject or synthetic-tree assertion).

### 6. Docs + standards (Constitution "Toolkit Standards" clause — check performed)

- **Standards check findings (verified at intake)**: `docs/site/standards/install-composition.md` does **NOT** name `shell-setup`/`agent-setup` at all — contrary to the conversation's expectation that it "will need a matching revision". The one standards file naming an old spelling is `docs/site/standards/principles.md` line 88: "`shll install` and `shll shell-setup` are idempotent by contract". `help-dump.md` imposes no subcommand-shape rules (its Hidden-filter rule is what removes the old spellings from the rendered reference). **Both standards edits are in scope**: fix the principles.md line-88 spelling AND expand install-composition.md to document the install→setup composition. <!-- clarified: standards revision scope — user chose both edits (expand install-composition.md + fix principles.md spelling) -->
- Any edit under `docs/site/standards/` requires re-running `scripts/sync-standards.sh` so the build-time embedded copies (`src/cmd/shll/standards/*.md`) match — enforced by `TestStandardsEmbedMatchesCanonical`.
- `README.md` and shll.ai docs-site pages naming the old commands: `docs/site/install.md`, `docs/site/workflows.md`, `docs/site/skill.md`, `docs/site/standards/shell-init.md`, `docs/site/standards/skill.md` (verified grep hits) — update spellings where they refer to shll's own commands (not run-kit's or wt's).

### 7. Explicit non-changes

- `scripts/install.sh` — untouched (execs `shll install`, which calls the internals in-process).
- `--no-shell-setup` / `--no-agent-setup` install flags — unchanged names and behavior.
- No behavior change to what the setup halves *do* — pure CLI-surface relocation.

## Affected Memory

- `cli/shell-setup`: (modify) command respelled to `shll setup shell`; old spelling hidden-deprecated with `shell-install` alias carried
- `cli/agent-setup`: (modify) command respelled to `shll setup agent`; refreshArgv flip; old spelling hidden-deprecated
- `cli/commands`: (modify) root registration — new `setup` family, hidden deprecated old spellings
- `cli/install`: (modify) help text and post-install nudge lines name the new spellings; in-process internals unchanged
- `cli/doctor`: (modify) suggestion hint strings name the new spellings
- `cli/update`: (modify) end-of-run refresh argv becomes `shll setup agent [--yes]`; help prose
- `cli/uninstall`: (modify) `shellUnwireHint` names `shll setup shell --uninstall`
- `cli/help-dump-contract`: (modify) hidden old spellings drop from the dump; aliased-node test subject changes
- `cli/standards-content`: (modify) if principles.md (and possibly install-composition.md) are revised

(Hydrate MAY consolidate `cli/shell-setup` + `cli/agent-setup` into a single `cli/setup` file — hydrate's call.)

## Impact

- **Code**: `src/cmd/shll/agent_setup.go`, `shell_setup.go`, `root.go`, `install.go` (text only), `doctor.go` (strings), `uninstall.go` (string), `update.go` (prose/preview), new `setup.go` (or equivalent); tests alongside each plus `help_dump_test.go`.
- **Docs**: `README.md`, `docs/site/install.md`, `docs/site/workflows.md`, `docs/site/skill.md`, `docs/site/standards/principles.md` (line 88), possibly `docs/site/standards/install-composition.md`, `docs/site/standards/shell-init.md`, `docs/site/standards/skill.md`; embedded standards re-sync via `scripts/sync-standards.sh`.
- **Compat surface**: one release cycle where both spellings work; `shll update` across the release boundary exercises old-binary→new-binary `shll agent-setup --yes` (must keep working).
- **Rendered reference**: help/shll.json (shll.ai) loses the old spellings (Hidden-filtered) and gains the `setup` family.

## Open Questions

*(none — both deferred questions resolved via /fab-clarify, see ## Clarifications)*

## Clarifications

### Session 2026-08-20

| # | Question | Answer |
|---|----------|--------|
| 10 | Bare `shll setup` surface beyond `--yes`? | Just `--yes` — minimal surface; `--print`/`--uninstall`/`[shell]` stay on the subcommands only |
| 11 | Standards revision scope: expand install-composition.md vs only fix principles.md line 88? | Both — expand install-composition.md to document the install→setup composition AND fix the principles.md spelling |

### Session 2026-08-20 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 8 | Confirmed | — |
| 9 | Confirmed | — |
| 11 | Confirmed | — |
| 12 | Confirmed | — |
| 13 | Confirmed | — |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Subcommand family `shll setup` / `setup shell [shell]` / `setup agent` — subcommands, not flags | Discussed — user chose subcommands over a flag union (disjoint flag sets; run-kit `agent setup` noun-verb precedent) | S:95 R:70 A:90 D:90 |
| 2 | Certain | Old spellings kept as hidden deprecated top-level commands for ≥1 release | Hard compat constraint verified in code — old binary's `refreshArgv` executes `shll agent-setup --yes` against the new binary during `shll update` self-upgrade | S:95 R:60 A:95 D:90 |
| 3 | Certain | `shell-install` alias carried on the hidden deprecated `shell-setup` command | Discussed — user decided its fate explicitly; existing alias precedent at shell_setup.go:56 | S:90 R:80 A:85 D:85 |
| 4 | Certain | `scripts/install.sh` and install's in-process setup calls unchanged; new commands are thin faces over `runShellSetupDefault`/`runAgentSetup` | Verified — install.sh only execs `shll install`; install.go calls the internals as Go functions (lines ~407/430) | S:90 R:85 A:95 D:90 |
| 5 | Certain | `refreshArgv` flips to emit `shll setup agent [--yes]` in the new release | Discussed and verified — single source of truth shared with update's dry-run preview, so both flip together | S:90 R:75 A:90 D:90 |
| 6 | Certain | Install opt-out flags `--no-shell-setup`/`--no-agent-setup` keep their names | Discussed — user decided renaming is not required by the consolidation | S:90 R:80 A:90 D:90 |
| 7 | Certain | Flag carry-over: `setup shell` keeps `[shell]`, `--print`, `--uninstall`, `--rc-file`; `setup agent` keeps `--print`, `--uninstall`, `--yes` | Stated expectation in the conversation; matches current flag sets in shell_setup.go/agent_setup.go | S:85 R:75 A:90 D:85 |
| 8 | Confident | Bare `shll setup` runs shell half then agent half, and exits non-zero if either half fails (worst-wins) | Clarified — user confirmed (bulk confirm). Order mirrors install's `runPostInstallSetup`; exit-code aggregation per principles.md | S:95 R:70 A:75 D:70 |
| 9 | Confident | Hidden old spellings delegate silently — no cobra `Deprecated:` stderr warning | Clarified — user confirmed (bulk confirm). iags precedent: a warning would leak into the transition-release update refresh | S:95 R:75 A:70 D:60 |
| 10 | Certain | Bare `shll setup` supports only `--yes`/`-y` — no composite `--print`/`--uninstall`, no `[shell]` positional (those stay on the subcommands) | Clarified — user confirmed the minimal `--yes`-only surface. R/A/D re-scored post-answer: expanding the surface later is additive (R), the user directive is the clear answer (A), one settled interpretation (D) | S:95 R:75 A:90 D:90 |
| 11 | Confident | Standards revision does both: expand `install-composition.md` to document the install→setup composition AND fix the `principles.md` line-88 spelling | Clarified — user chose both edits, re-confirmed in bulk confirm. R stays moderate (binding-standard ripple + embedded-copy re-sync) | S:95 R:50 A:85 D:85 |
| 12 | Confident | `help_dump_test.go`'s aliased-node assertion (visible `shell-setup` + `[shell-install]`) is adapted rather than keeping `shell-setup` visible | Clarified — user confirmed (bulk confirm). Compat requirement is argv acceptance, not dump visibility; mechanism is an apply-level detail | S:95 R:80 A:75 D:65 |
| 13 | Confident | Hydrate consolidates `cli/shell-setup` + `cli/agent-setup` memory into one `cli/setup` file | Clarified — user confirmed (bulk confirm); memory shape easily reshaped at hydrate | S:95 R:85 A:50 D:40 |

13 assumptions (8 certain, 5 confident, 0 tentative, 0 unresolved).
