# Plan: rk-desktop in the roster, bootstrap convergence, and install/update terminal UX

**Authored**: 2026-08-20
**Author**: planning session with Claude (fab-discuss in run-kit, decisions confirmed by Sahil)
**Executor**: fab-operator (autopilot queue) or individual fab agents
**Status**: Executed 2026-08-20 — all three changes merged: A #86 (260820-t26g), B #87 (260820-bau2), C #88 (260820-yud0). Changes not yet archived. Deferred items below remain open.

## Goal

Three shll changes, shipped as three PRs:

1. **Change A — `roster-desktop-entry`**: reorder the roster importance-descending and add `rk-desktop` as a roster entry (non-brew, delegates to `rk desktop install/update`, probe-gated on platform support).
2. **Change B — `install-sh-convergence`**: the curl bootstrap converges the machine to *complete and current* (`shll install` then `shll update`), plus phase-line polish in install.sh.
3. **Change C — `install-update-terminal-ux`**: two-region terminal experience for `shll install`/`shll update` in Go — pinned status header, streaming child output, prompt-hang hardening, determinate OSC 9;4 progress.

Everything below lives in the **shll repo**. run-kit is touched only if A's probe seam needs stabilizing (see A, scope note) — the Linux/Windows desktop installer arms are explicitly **deferred and not part of this plan**.

## Decisions already made (do not re-litigate at intake)

- `shll install` and `shll update` stay distinct verbs. Install = fill gaps via brew (or delegation); update = converge installed tools via each tool's own `update`. No upgrade logic folded into install.
- Bootstrap convergence is implemented in **install.sh** as two steps (`shll install "$@"` then `exec shll update "$@"`), not by changing `shll install` semantics.
- rk-desktop gating is **probe-based, never a hardcoded darwin check in shll**. `rk desktop` already refuses unsupported platforms itself (run-kit `cmd/rk/desktop.go` PersistentPreRunE, `errDesktopMacOnly`). When run-kit later grows Linux support, shll must need **zero changes**.
- Roster order becomes importance-descending with dependency adjacency: **run-kit, rk-desktop, fab-kit, wt, idea, tu, hop** (tail keeps current relative order — confirm tail order with Sahil at intake if it matters).
- No sibling `depends_on` between formulas (install-composition standard) — rk-desktop's dependency on run-kit is expressed as a runtime probe + roster adjacency only.
- Installs/updates are **prompt-free by standard**. UI work does not accommodate interactive questions; it hardens against them (stdin `</dev/null` for children, fail fast + show captured tail). If a genuine question is ever unavoidable, read `/dev/tty` explicitly.
- Progress is honest `k/n` steps, never a synthetic percent across tools. OSC 9;4 emission reuses the existing plumbing in `src/cmd/shll/progress.go`.
- Windows desktop = manual NSIS download (no rk CLI there); Linux desktop installer arm = demand-gated, deferred to run-kit.

## Change A — roster reorder + rk-desktop roster entry

**Repo**: shll · **Type**: feat · **Suggested slug**: `roster-desktop-entry`

**Problem**: The desktop viewer shell is only installable via `rk desktop install` (or manual DMG); the toolkit bootstrap and `shll install/update/list/doctor` are blind to it. Roster order (`wt, idea, tu, run-kit, hop, fab-kit`) carries no meaning.

**Scope**:
- Reorder `Roster` in `src/cmd/shll/tools.go` to: run-kit, rk-desktop, fab-kit, wt, idea, tu, hop.
- Extend the `Tool` model for a non-brew entry: rk-desktop has no Formula; install delegates to `rk desktop install`, update to `rk desktop update`, installed-probe to `rk desktop status` (parse the `Installed:` line — it already distinguishes `not installed`). Design the seam so a future non-brew tool reuses it.
- Platform/prerequisite gate at runtime: rk (run-kit) binary present AND `rk desktop` doesn't refuse. An unsupported-platform refusal is a **skip with note** in whole-roster runs, an explicit message on `shll install rk-desktop`; distinguish it from a real failure. If the current `errDesktopMacOnly` message is too unstable to match, the run-kit-side fix is a tiny companion change (stable stderr token or dedicated exit code) — decide at intake; prefer matching the existing message + freezing it with a test in run-kit.
- Whole-roster runs process rk-desktop right after run-kit; if run-kit's install failed that run, skip rk-desktop too.
- Sweep every hardcoded roster listing: `install.go`/`update.go`/`uninstall.go` Long help strings, `list`, `doctor`, `version`, agent-setup skill description generation, `TestRoster*` assertions, docs/site pages.
- Standards conformance pass: `shll standards principles`, `install-composition`, `update`, `help-dump`, `skill`, `readme-extraction` — the roster is a published surface.

**Acceptance sketch**: `shll list` shows the new order incl. rk-desktop; `shll install` on a darwin box with rk present installs the app (idempotent on re-run); on this Linux box it prints a skip note and exits 0 for the whole-roster run; `shll update` delegates to `rk desktop update` when installed; no formula `depends_on` introduced.

## Change B — install.sh convergence + phase polish

**Repo**: shll · **Type**: feat · **Suggested slug**: `install-sh-convergence`

**Problem**: `curl shll.ai/install | sh` fills gaps but leaves already-installed tools stale. The script's own three phases (preflight → brew bootstrap → shll) give no structured feedback.

**Scope**:
- Last line becomes install-then-update: `shll install "$@" && exec shll update "$@"` (subset passes through to both; freshly installed tools are cheap no-op updates).
- Document the new side effect in the script header and on shll.ai's install page: updating installed tools runs their `update` contracts (e.g. run-kit's daemon restart).
- Phase-line polish: colored `✓/→` announcements for the three script phases, gated on tty (`test -t 1`) and `NO_COLOR`. Keep the script dumb — no scroll regions, no percent.
- Optional nicety: emit OSC 9;4 indeterminate (one `printf`) around the brew bootstrap, cleared after; harmless on non-supporting terminals.
- Keep the layering contract: no roster knowledge enters the script.

**Acceptance sketch**: fresh box ends complete AND current; re-run on a stale box upgrades tools; subset run (`… | sh -s -- hop`) installs+updates only hop; piped-to-file output has no color/escape garbage.

## Change C — two-region install/update terminal UX (Go)

**Repo**: shll · **Type**: feat · **Suggested slug**: `install-update-terminal-ux`

**Problem**: Long roster walks interleave per-tool output with no persistent view of overall progress; a hypothetical child prompt would hang invisibly.

**Scope**:
- Pinned status region for `shll install` and `shll update` (tty only): top line(s) show current tool, `k/n`, and next tool (e.g. `Installing run-kit (2/7) · next: rk-desktop`); child output streams beneath via a DECSTBM scroll region. Restore the region on every exit path (defer + signal) and re-apply on SIGWINCH.
- Degradation: not-a-tty (CI, pipes) keeps today's sequential `printToolHeader` output exactly; `NO_COLOR` respected as today.
- Prompt-hang hardening: children run with stdin from `/dev/null`; on failure, print the captured output tail. This enforces the prompt-free standard rather than accommodating prompts.
- Wire determinate OSC 9;4 (`pos/total`) through the existing `progress.go` plumbing (incl. its tmux passthrough) so the terminal tab and the run-kit dashboard tile show a real progress bar; clear on finish, error-state on failure.
- Reuse/extend `ui.go` helpers; keep everything unit-testable through the existing writer/tty seams.

**Acceptance sketch**: tty run shows pinned header + scrolling output and leaves the terminal clean after Ctrl-C; `shll update | cat` output is byte-identical in structure to today's; a child that tries to prompt fails fast with visible output instead of hanging.

## Dependencies & order

- **A first.** C touches the same files (`install.go`, `update.go`, `ui.go`) — running it after A avoids merge conflicts and lets C's `k/n` reflect the final roster.
- **B is independent** (script + docs only) but sequential execution is fine.
- Queue order: **A → B → C** (implicit chaining is acceptable; there is no hard runtime dependency of B or C on A beyond conflict avoidance).
- Possible run-kit companion (only if A's intake decides the probe signal needs stabilizing): tiny change in run-kit freezing the unsupported-platform error contract. Not queued here.

## Operator pickup

Prerequisites (agent or Sahil, in the shll repo root, before autopilot):

1. Draft the three intakes from the sections above: `/fab-draft` per change (slugs above). Each section is written to be intake-ready — problem, scope, decisions, acceptance. Resolve the one open Tentative (roster tail order) with Sahil or mark it Confident with the order given here.
2. Commit the intakes and this plan (`chore(fab): draft desktop-roster/install-ux intakes + plan`) via the normal PR flow — no direct pushes to main.

Then start the queue from an operator session in the shll repo:

```
fab operator
# when it reports ready:
fab operator autopilot start <A-id> <B-id> <C-id>
```

Each spawned agent runs `/fab-switch <change> && /fab-proceed` (full pipeline through review-pr). Review each PR as it opens; A merges before C starts apply if possible (conflict avoidance), which the sequential queue already guarantees.

## Deferred (tracked here so it isn't lost)

- **run-kit: Linux desktop installer arm** — AppImage download + SHA256 (reuse) + chmod + place + `.desktop` entry; needs a display-server check (`$DISPLAY`/`$WAYLAND_DISPLAY`) so headless boxes refuse. Build when a real Linux desktop machine enters the fleet. shll needs zero changes when it lands (probe-based gate).
- **Windows**: viewer shell already works pointed at a remote rk host; install stays manual NSIS download. Revisit winget/scoop only on demand.
