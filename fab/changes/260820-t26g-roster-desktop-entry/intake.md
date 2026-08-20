# Intake: Roster reorder + rk-desktop roster entry

**Change**: 260820-t26g-roster-desktop-entry
**Created**: 2026-08-20

## Origin

Drafted via `/fab-draft` from the authored plan `fab/plans/sahil/desktop-roster-and-install-ux.md` (Change A section), a planning session with decisions confirmed by Sahil. One-shot draft — the plan section was written to be intake-ready (problem, scope, decisions, acceptance). The plan's one Tentative item (roster tail order) was marked Confident per operator instruction, using the order the plan already gives.

> Change A — `roster-desktop-entry`: reorder the roster importance-descending and add `rk-desktop` as a roster entry (non-brew, delegates to `rk desktop install/update`, probe-gated on platform support).

## Why

1. **Pain point**: The desktop viewer shell is only installable via `rk desktop install` (or manual DMG download); the toolkit bootstrap and `shll install/update/list/doctor` are blind to it. A fresh machine bootstrapped via `curl shll.ai/install | sh` ends up with every CLI tool but no desktop app, and `shll update` never converges it.
2. **Consequence if unfixed**: rk-desktop stays a manually-managed side channel — it drifts stale, `shll doctor` can't diagnose it, and every new machine needs a remembered extra step.
3. **Why this approach**: The roster is the single hardcoded source of truth for what shll manages (Constitution: Tool Roster Source of Truth). Adding rk-desktop as a roster entry — delegating to `rk desktop` subcommands per Constitution III/IV (wrap, don't reinvent; composition, not replacement) — makes it visible to every roster-driven surface at once. Reordering the roster importance-descending at the same time gives the order meaning (today's `wt, idea, tu, run-kit, hop, fab-kit` carries none) and puts rk-desktop adjacent to its runtime dependency run-kit.

## What Changes

### Roster reorder (`src/cmd/shll/tools.go`)

Reorder the `Roster` slice to importance-descending with dependency adjacency:

```
run-kit, rk-desktop, fab-kit, wt, idea, tu, hop
```

The tail keeps the relative order given in the plan (marked Confident — see Assumptions). Roster order drives `install`/`update`/`uninstall` (reverse) walk order, `list`/`version`/`doctor` row order, and `shell-init` composition order — all inherit the new order.

### Non-brew Tool seam + rk-desktop entry

Extend the `Tool` model so a roster entry need not be brew-backed. rk-desktop has **no Formula**; instead:

- **Install**: delegate to `rk desktop install`
- **Update**: delegate to `rk desktop update`
- **Installed-probe**: `rk desktop status` — parse the `Installed:` line (it already distinguishes `not installed`)

Design the seam so a future non-brew tool reuses it (e.g. delegated-argv fields for install/update/probe alongside the existing `Formula`-driven default path), rather than special-casing rk-desktop by name. No formula and no `depends_on` is introduced — per the install-composition standard, rk-desktop's dependency on run-kit is expressed as a runtime probe + roster adjacency only.

### Platform/prerequisite gate (probe-based, never hardcoded)

At runtime, rk-desktop is actionable only when the `rk` (run-kit) binary is present AND `rk desktop` does not refuse the platform. `rk desktop` already refuses unsupported platforms itself (run-kit `cmd/rk/desktop.go` PersistentPreRunE, `errDesktopMacOnly`) — shll MUST NOT hardcode a darwin check; when run-kit later grows Linux support, shll needs zero changes.

- An unsupported-platform refusal is a **skip with note** in whole-roster runs (exit 0), and an explicit message on a targeted `shll install rk-desktop` — distinguished from a real failure.
- Preferred detection: match the existing `errDesktopMacOnly` message and freeze it with a test in run-kit. If that message proves too unstable to match, the fallback is a tiny run-kit companion change (stable stderr token or dedicated exit code) — not part of this change's queue.
- Whole-roster runs process rk-desktop immediately after run-kit; if run-kit's install failed that run, skip rk-desktop too.

### Hardcoded-roster sweep

Update every surface that hardcodes the roster listing or order:

- `install.go` / `update.go` / `uninstall.go` Long help strings
- `list`, `doctor`, `version` output and their tests
- agent-setup skill description generation (`SkillHint` composition)
- `TestRoster*` assertions in `tools_test.go` and per-command tests
- docs/site pages that enumerate the roster

### Standards conformance pass

The roster is a published surface — check the change against `shll standards principles`, `install-composition`, `update`, `help-dump`, `skill`, and `readme-extraction` (read the governing files under `docs/site/standards/` per the constitution's Toolkit Standards clause).

## Affected Memory

- `cli/commands`: (modify) Roster slice — new order, non-brew Tool seam fields
- `cli/install`: (modify) rk-desktop delegated install, skip-with-note gating, run-kit-failed cascade skip
- `cli/update`: (modify) rk-desktop delegated update path (non-brew, no keg relink/brew fallback)
- `cli/list`: (modify) new roster order + rk-desktop row with install status
- `cli/doctor`: (modify) rk-desktop checks (probe via `rk desktop status`, no formula-trust check)
- `cli/version`: (modify) rk-desktop row + installed-probe seam changes if `toolInstalled` grows a delegated variant
- `cli/uninstall`: (modify) reverse-roster order change; rk-desktop removal delegation or exclusion (decided at plan)
- `cli/standards-conformance`: (modify) conformance state after the roster-surface pass

## Impact

- **Code**: `src/cmd/shll/tools.go` (Roster + Tool model), `install.go`, `update.go`, `uninstall.go`, `list.go`, `doctor.go`, `version.go`, `agent_setup.go`/`setup.go` (skill description generation), and their `_test.go` files. All delegation through `internal/proc` (Constitution I).
- **Docs**: docs/site pages enumerating the roster; README only if it lists tools (it is slimmed to bootstrap+pointer).
- **External behavior**: `shll list/install/update/doctor/version/uninstall` output order changes; rk-desktop appears as a managed tool. `shell-init` composition order changes (eval-safe output unaffected — rk-desktop has no shell-init).
- **Cross-repo**: possible tiny run-kit companion (freeze the unsupported-platform error contract with a test) — preferred over new signaling; not queued here.
- **Sequencing**: this change lands **before** `install-update-terminal-ux` (Change C) — C touches the same files (`install.go`, `update.go`, `ui.go`) and its `k/n` should reflect the final roster.

## Open Questions

- (none — the plan resolved all decision points; see Assumptions)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Roster tail order is exactly `run-kit, rk-desktop, fab-kit, wt, idea, tu, hop` | Plan gives this order; operator instruction resolved the plan's "confirm tail with Sahil" note by marking it Confident as-given. Order is cosmetic-with-meaning, trivially reversible | S:75 R:90 A:70 D:70 |
| 2 | Certain | Gating is probe-based (rk present + `rk desktop` doesn't refuse), never a hardcoded darwin check | Explicit plan decision ("do not re-litigate"); zero shll changes when run-kit grows Linux support | S:90 R:80 A:90 D:95 |
| 3 | Certain | No `depends_on` between formulas; run-kit dependency = runtime probe + roster adjacency | install-composition standard + explicit plan decision | S:90 R:85 A:95 D:95 |
| 4 | Confident | Installed-probe parses the `Installed:` line of `rk desktop status` | Plan specifies it and notes the output already distinguishes `not installed`; parsing seam is testable and reversible | S:80 R:75 A:70 D:75 |
| 5 | Confident | Unsupported-platform detection matches the existing `errDesktopMacOnly` message, frozen by a run-kit test; stable token/exit code is the fallback only if matching proves unstable | Plan states the preference explicitly; fallback path documented | S:70 R:70 A:65 D:60 |
| 6 | Confident | Whole-roster runs skip rk-desktop when run-kit's install failed that run | Plan decision; adjacency ordering exists to support it | S:80 R:85 A:80 D:85 |

6 assumptions (2 certain, 4 confident, 0 tentative, 0 unresolved).
