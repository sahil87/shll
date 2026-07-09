# Intake: shll uninstall Command

**Change**: 260709-kkaj-shll-uninstall-command
**Created**: 2026-07-09

## Origin

Conversational — same `/fab-discuss` session as the sibling change `260709-9bak-run-kit-rename-migration-guard`. The user's raw ask:

> When you actually implement it also create "shll uninstall" -> that removes all shll utilities. Which allows for a cleaner installation step if we get stuck anywhere.

The design was sketched in discussion and accepted: the user's original uninstall→install migration proposal was repurposed — routine migration goes through the brew-native rename (sibling change), and the uninstall→reinstall sequence becomes the **explicit repair path** this command provides. Key hazard discovered during the session: Homebrew's rename resolution means old-name commands can act on the *new* formula (observed: `brew list --formula --versions sahil87/tap/rk` reports `run-kit 3.0.0` post-migration), so a blind `brew uninstall sahil87/tap/rk` on a migrated machine could remove the good keg. The author's machine is currently in a **dual-rack state** (both `Cellar/rk/3.0.0` and `Cellar/run-kit/3.0.0` present, `brew list --formula` lists both) — a real fixture for this command's edge cases.

## Why

1. **No clean-slate path exists.** When brew state gets wedged — an unlinked keg (observed at session start: `Cellar/rk/2.5.13` installed but neither binary linked), a dual-rack orphan (observed now), a half-failed upgrade — the only recovery is hand-typed brew surgery across up to seven formulas. `shll install` bootstraps but cannot clear wedged state; nothing removes.
2. **The rename transition multiplies wedge-states.** The rk→run-kit migration (sibling change) creates machines where formula names, rack names, and binary names disagree. A one-command "remove it all cleanly, then `shll install`" escape hatch is the safety net the migration guard assumes exists (its dual-rack handling is deliberately detect-and-note-only, deferring cleanup here).
3. **Symmetry.** `shll install` exists; brew pairs install/uninstall; toolkit users expect the manager to manage the full lifecycle.

**Constitution VII justification** (new top-level subcommand — required): uninstall is a distinct destructive verb, not a variant of an existing operation. Hiding it behind a flag on `install` (e.g. `--clean`) would bury a destructive action inside an additive command; a per-tool CLI cannot own it (removing *all* toolkit members is inherently the meta-tool's job). The current scope question — "could this be a flag?" — was answered explicitly in discussion: no.

## What Changes

### 1. New subcommand: `shll uninstall [tool...]` (`src/cmd/shll/uninstall.go`)

- **No args**: uninstall every *installed* roster tool (brew-keg probe, same `probeInstalledVersion` seam update uses). Tools not installed are skipped gracefully with a `not installed` line, never an error (Constitution V). shll itself is **not** included in the no-args sweep.
- **Targeted**: `shll uninstall <tool...>` resolves via `resolveTargets` (`tools.go:164`) with `allowShll=true` — `shll uninstall shll` is legal and explicit-only. Unknown targets error with the valid-target list (same contract as update/install). Named-but-not-installed: report `not installed`, exit 0 (repair-path semantics — the goal state "gone" is already met; unlike `update`, absence is success).
- **Order**: reverse-roster (dependents before leaves — `fab-kit, hop, run-kit, tu, idea, wt`), the mirror of install's leaves-first coherence rationale. When shll-self is named, it is processed **last**, after all roster tools.
- **Mechanics per tool**: `brew uninstall <formula>` via `internal/proc` (Constitution I). No per-tool `uninstall` subcommands exist to delegate to, so direct brew is correct here (same precedent as install).

### 2. Confirmation gate

- Default: print the removal plan (tool, formula, installed version), then prompt `Proceed? [y/N]`. Anything but explicit yes aborts with exit 0.
- `--yes` (`-y`): skip the prompt (scripting).
- Non-TTY stdin without `--yes`: refuse with a hint to pass `--yes` (fail-safe for pipes/CI).
- `--dry-run`: print the exact brew commands the run would execute and exit 0 with no write — consistent with `shll update --dry-run` / `shll install --dry-run`, sharing the single-source-of-truth argv pattern (`upgradeArgv` precedent).

### 3. run-kit dual-name sweep (the rename-transition case)

The user's "uninstall `sahil87/tap/rk`, uninstall `run-kit` just to be safe" belongs here. But **never uninstall the old name blind**: post-rename, brew resolves `rk` → `run-kit`, so `brew uninstall sahil87/tap/rk` on a cleanly-migrated machine would delete the good keg. Instead, probe-then-act with leaf-name verification (same parser the sibling change adds to `brew.go`):

1. Probe `sahil87/tap/run-kit`; stdout leaf `run-kit` → `brew uninstall sahil87/tap/run-kit`.
2. Probe for a residual `rk`-leaf keg (legacy keg or dual-rack orphan — `brew list --formula` listing a bare `rk` line); only when confirmed → remove it (`brew uninstall rk`, escalating per the open question below).
   <!-- assumed: plain `brew uninstall rk` removes the orphan rack post-rename — unverified brew behavior; may need --force or rack-targeted escalation, verifiable at apply on the author's dual-rack machine -->
3. Order: new name first, then legacy leftover — after step 1 the name `rk` can only match the orphan rack.

This makes `shll uninstall run-kit` the supported cleanup for the dual-rack state the migration guard only warns about.

### 4. shll-self uninstall (explicit target only)

- Gate on brew management: `probeInstalledVersion(shllFormula)` — a `go install`/local-build shll errors with `not brew-managed` (same fact update.go's self-upgrade path keys on).
- `brew uninstall sahil87/tap/shll`, processed last. The running process keeps working (unix unlink semantics); print a farewell noting the reinstall path (`brew install sahil87/tap/shll`).

### 5. Output & exit codes

- Follows the `per-tool-output-separation` spec conventions: per-tool headers, summary tail (`N removed, M skipped, K failed`), TTY-gated color, plain-ASCII degradation.
- A failed `brew uninstall` records the failure, continues to the next tool, and the command exits non-zero at the end (update.go's aggregation pattern). Skips are not failures.
- **Post-run hints** (print-only, Constitution III — never executed): when run-kit was removed, note a running daemon is not stopped (`run-kit serve --stop`); when shell-integrated tools (`tu`, `hop`, `wt`) were removed roster-wide, point at `shll shell-setup --uninstall` for the rc-file block.

### 6. Wiring

- `root.go`: register `newUninstallCmd()`.
- `brew.go`: shared `brew uninstall` helper beside the existing install/upgrade helpers.
- If the hidden `help-dump` fixtures embed shll's own command tree, regenerate them (check `help/` contract — see `cli/help-dump-contract` memory).

### Non-Goals

- **No untap** of `sahil87/tap` and no trust revocation — the tap remains for reinstall (the whole point of the repair path).
- **No config/state purge** (rk daemon state, hop data, rc-file edits) — brew-uninstall semantics, not `--zap`; a purge flag can come later if wanted.
- **No stopping of running processes** (run-kit daemon) — print the hint, never execute (Constitution III).
- **No `shll uninstall` → `shll install` composite** — the repair recipe stays two explicit commands.

## Affected Memory

- `cli/uninstall`: (new) command behavior — target resolution, confirmation gate, reverse-roster order, dual-name run-kit sweep, self-uninstall, exit-code aggregation
- `cli/commands`: (modify) subcommand registration and the shll-self target handling note
- `cli/install`: (modify) cross-reference as the counterpart command; shared brew-helper seam if refactored
- `cli/update`: (modify) `migrationDualRackNoteFmt` retargeted at `shll uninstall <tool>` / leaf-name `brew uninstall rk` (the qualified old-name form was the rename-re-resolution footgun); §migration action step 4 wording is stale <!-- added post-review: the diff changed update.go's dual-rack note + state-C golden -->


## Impact

- **Source**: new `src/cmd/shll/uninstall.go` (+ `uninstall_test.go`); edits to `root.go`, `brew.go`, possibly `tools.go` (shared target/format helpers)
- **Tests**: fake `proc.Runner` covers all brew interactions; key cases — no-args sweep skips missing, targeted named-missing exits 0, prompt abort, `--yes`, non-TTY refusal, dry-run preview parity, dual-rack sweep ordering, rename-resolution footgun (old-name uninstall never issued without a verified `rk`-leaf keg), self-uninstall gating, failure aggregation
- **Depends on**: the leaf-name brew parser introduced by sibling change `260709-9bak` (sequence this change after it, or absorb the parser here if reordered)
- **Constitution touchpoints**: I (proc-only subprocesses), III (wrap brew; print-don't-run for sub-tool logic), V (graceful skips; exit-0 aborts), VII (justified above)

## Open Questions

- Post-rename, does `brew uninstall rk` remove an orphan `rk` rack (formula file gone from the tap), or does name resolution redirect to `run-kit` / refuse, requiring `brew uninstall --force rk` or another rack-targeted form? Verifiable at apply time — the author's machine is currently in exactly this dual-rack state.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Command shape: `shll uninstall [tool...]`; no-args = all installed roster tools; shll-self explicit-only, processed last | User's ask ("removes all shll utilities") plus discussed-and-accepted design sketch | S:85 R:80 A:90 D:85 |
| 2 | Confident | Confirmation prompt by default with removal plan; `--yes` bypass; non-TTY without `--yes` refuses | Destructive verb warrants a gate; fail-safe default for pipes; standard CLI convention | S:50 R:85 A:75 D:65 |
| 3 | Confident | `--dry-run` included | Consistency with install/update; reuses the argv single-source-of-truth pattern | S:40 R:90 A:80 D:70 |
| 4 | Certain | run-kit sweep is probe-then-act with leaf verification; old-name uninstall never issued blind | User asked for the both-names sweep; session-observed rename resolution proves blind old-name uninstall can delete the good keg | S:80 R:75 A:85 D:80 |
| 5 | Confident | Scope: brew uninstall only — no untap, no state purge, no rc-unwiring (hint at `shll shell-setup --uninstall`), daemon hint print-only | Discussed non-goals; keeps v1 minimal per Constitution VII; purge can layer on later | S:65 R:85 A:75 D:70 |
| 6 | Confident | Output per `per-tool-output-separation` conventions; failures aggregate to non-zero exit; named-but-missing exits 0 | Existing spec + update.go precedent; repair-path semantics make absence a success state | S:55 R:85 A:85 D:75 |
| 7 | Confident | shll-self uninstall gated on brew management, error otherwise | Mirrors update.go's self-upgrade gating; uninstalling a non-brew binary via brew cannot work | S:45 R:85 A:80 D:70 |
| 8 | Tentative | Orphan-rack removal mechanics: plain `brew uninstall rk` assumed sufficient, `--force`/rack-targeted escalation if not | Post-rename brew behavior unverified; author's machine is in the exact state to verify at apply time | S:35 R:75 A:40 D:40 |

8 assumptions (2 certain, 5 confident, 1 tentative, 0 unresolved).
