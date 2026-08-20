# Intake: install.sh convergence + phase polish

**Change**: 260820-bau2-install-sh-convergence
**Created**: 2026-08-20

## Origin

Drafted via `/fab-draft` from the authored plan `fab/plans/sahil/desktop-roster-and-install-ux.md` (Change B section), a planning session with decisions confirmed by Sahil. One-shot draft — the plan section was written to be intake-ready (problem, scope, decisions, acceptance).

> Change B — `install-sh-convergence`: the curl bootstrap converges the machine to *complete and current* (`shll install` then `shll update`), plus phase-line polish in install.sh.

## Why

1. **Pain point**: `curl shll.ai/install | sh` fills gaps but leaves already-installed tools stale — a re-run on an existing machine installs nothing and upgrades nothing, so the bootstrap is not a convergence command. Separately, the script's own three phases (preflight → brew bootstrap → shll handoff) give no structured feedback while running.
2. **Consequence if unfixed**: users must remember to run `shll update` after the bootstrap; machines bootstrapped "again" silently keep stale tools; the script's output stays an undifferentiated wall.
3. **Why this approach**: convergence is implemented **in install.sh as two steps** (`shll install "$@"` then `exec shll update "$@"`), not by changing `shll install` semantics — `install` and `update` stay distinct verbs (install = fill gaps via brew/delegation; update = converge installed tools via each tool's own `update`; explicit plan decision, do not re-litigate). This keeps the layering contract: the script owns pre-brew, `shll` owns post-brew, and no roster knowledge enters the script.

## What Changes

### install.sh convergence (`scripts/install.sh`, served at shll.ai/install)

The current last line is:

```sh
exec shll install "$@"
```

It becomes install-then-update:

```sh
shll install "$@" && exec shll update "$@"
```

- The arg subset passes through to **both** verbs (e.g. `… | sh -s -- hop` installs and updates only hop; `shll install`/`shll update` validate the names themselves).
- Freshly installed tools are cheap no-op updates — the double pass is acceptable by design.
- Document the new side effect in the script's header comment **and** on shll.ai's install page: updating installed tools runs their `update` contracts (e.g. run-kit's daemon restart).

### Phase-line polish

Colored `✓/→` announcements for the script's three phases (preflight → brew bootstrap → shll handoff), gated on tty (`test -t 1`) and `NO_COLOR`. Keep the script dumb — no scroll regions, no percentages.

### Optional nicety: OSC 9;4 indeterminate around brew bootstrap

Emit OSC 9;4 indeterminate (a single `printf`) before the brew bootstrap and clear it after; harmless on non-supporting terminals. Same tty gating as the phase lines.

### Layering contract (unchanged, restated as a guard)

No roster knowledge enters the script — it installs brew and `shll` itself, then hands off. All per-tool logic stays behind `shll install`/`shll update`.

## Affected Memory

- `cli/install`: (modify) the bootstrap handoff contract changes — the script now runs install-then-update, and the "exec target of the curl … | sh bootstrap" description plus arg/flag pass-through surface must reflect both verbs

## Impact

- **Code**: `scripts/install.sh` only (shell). No Go changes.
- **Docs**: install.sh header comment; shll.ai install page (website repo `hop shll.ai where` → `/home/sahil/code/sahil87/shll.ai`) documents the update side effect. Docs-site edits in *this* repo only if the install page is sourced here.
- **External behavior**: `curl shll.ai/install | sh` now also upgrades already-installed tools (running their update contracts, e.g. run-kit's daemon restart); tty runs gain phase lines and an indeterminate progress hint.
- **Standards**: installs/updates are prompt-free by standard — the script adds no interactive questions. Piped/non-tty output must remain free of color/escape sequences.
- **Sequencing**: independent of Changes A and C (script + docs only); queued A → B → C for conflict-free sequential execution.

## Open Questions

- (none — the plan resolved all decision points; see Assumptions)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Convergence lives in install.sh as two steps (`shll install "$@" && exec shll update "$@"`), not in `shll install` semantics | Explicit plan decision ("do not re-litigate"); verbs stay distinct | S:90 R:80 A:90 D:95 |
| 2 | Certain | No roster knowledge enters the script | Explicit plan decision; existing layering contract | S:90 R:85 A:95 D:95 |
| 3 | Confident | `&&`-chain (not `;`): a failed `shll install` stops the bootstrap and skips the update pass | Matches the acceptance sketch's fail-visible posture and the existing `exec` handoff; trivially reversible one-character decision | S:65 R:90 A:80 D:75 |
| 4 | Confident | Phase lines use `✓/→` with color gated on `test -t 1` and `NO_COLOR`; no scroll regions or percent in the script | Plan specifies the exact gating and the keep-it-dumb constraint | S:85 R:90 A:85 D:85 |
| 5 | Confident | OSC 9;4 indeterminate around brew bootstrap is included (it is a one-printf optional nicety) | Plan lists it as optional; cost is near zero and Change C wires the determinate counterpart in Go, so terminal-progress coverage becomes end-to-end | S:70 R:95 A:80 D:70 |

5 assumptions (2 certain, 3 confident, 0 tentative, 0 unresolved).
