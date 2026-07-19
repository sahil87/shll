---
type: memory
description: "`shll uninstall [tool...]` — the clean-slate repair counterpart to `shll install`: reverse-roster removal with shll-self last, a `Proceed? [y/N]` confirmation gate (`--yes`/`-y`, non-TTY refusal, `--dry-run` bypass), the leaf-verified dual-name run-kit sweep (never a blind old-name uninstall), shll-self brew-managed gating, best-effort failure aggregation (named-missing exits 0), print-only daemon/rc-unwire hints, and the single-source `brewUninstallArgv` builder."
---
# cli/uninstall

`shll uninstall [tool...]` — brew-uninstalls shll toolkit tools. It is the **clean-slate repair path** that pairs with [`shll install`](/cli/install.md): when brew state gets wedged (an unlinked keg, a dual-rack orphan, a half-failed upgrade), `shll uninstall` removes it all cleanly so `shll install` can bootstrap fresh. Absence is a *success* state — the goal "gone" is already met — so a named-but-missing target is not an error (the inverse of `shll update`'s precondition).

Source: `src/cmd/shll/uninstall.go`, with the `stdinIsTTY` seam + `printUninstallPreview` in `src/cmd/shll/ui.go`, the `uninstallBrewMissingHint` in `src/cmd/shll/brew.go`, and wiring in `src/cmd/shll/root.go`.

## Behavior contract

The full happy/unhappy paths, in the order `runUninstall(ctx, stdin io.Reader, stdout, stderr io.Writer, dryRun, yes bool, args []string) error` evaluates them (`src/cmd/shll/uninstall.go:156`):

1. **Resolve targets up front — before `hasBrew` and any probe.** `resolveTargets(args, true)` (`allowShll=true`) validates the positional args and returns `(selected, selfSelected, aliased, err)`. An unknown target → `shll uninstall: <detail>` on stderr + `errSilent` (exit 1) with **no brew side effect**. `subset := len(args) > 0`; empty args yields the whole-roster sweep. The legacy alias `rk` resolves to `run-kit` and is recorded in `aliased` for the notice (below). See [cli/commands §the legacy target alias](/cli/commands.md#the-legacy-target-alias-rk--run-kit) and [cli/update §positional args](/cli/update.md#positional-tool-name-args--subset-targeting).

2. **Brew missing.** If `hasBrew(ctx)` is false, write `uninstallBrewMissingHint` (`"shll uninstall requires Homebrew. Install from https://brew.sh"`, `src/cmd/shll/brew.go`) to stderr and return `errSilent` (exit 1). It is intentionally separate from `brewMissingHint`/`installBrewMissingHint` so each command's error names the command the user ran (the update spec asserts its hint verbatim). The up-front target resolution (step 1) runs *before* this guard, so an unknown target errors first.

3. **Alias notice.** `printAliasNotices(stdout, aliased)` writes one `note: rk is now run-kit` line per aliased token (shared wording with `update`/`install`), before any framing. `resolveTargets` is IO-free; the caller prints.

4. **Build the actionable set + graceful skips.** Iterate `consider` (the full `Roster` for a whole-roster sweep, or `selected` for a subset — both in leaves-first roster order). For each tool:
   - A **`LegacyFormula`-bearing tool (run-kit)** is classified via `probeRunKitInstalled` (the leaf gate): a current `run-kit` keg OR a leaf-verified legacy `rk` keg makes it actionable as a `runKit` target carrying the classification facts (`runKitNewInstalled`/`runKitLegacyKeg`); neither present → skip. See [The run-kit dual-name sweep](#the-run-kit-dual-name-sweep).
   - Every **other** tool: `probeInstalledVersion(ctx, t.Formula)` — installed → actionable `uninstallTarget{tool, version}`; not installed → recorded in `skipped`.
   The probes are **reads**, so they run in dry-run too — only the writes are gated.

5. **shll-self classification (explicit-only, gated).** Only when `selfSelected` (never in the no-args sweep). Gate on `probeInstalledVersion(ctx, shllFormula)`: a `go install`/local-build shll (no brew keg) → `shll uninstall: shll: not brew-managed` on stderr + `errSilent` (the user explicitly asked for it, so absence is a hard error here, unlike a roster tool). Brew-managed → appended **LAST** so it is processed after every roster tool. Mirrors the fact `update.go`'s self-upgrade path keys on.

6. **Reverse-roster ordering.** `reverseRosterOrder(actionable)` reverses only the roster portion (dependents before leaves — the mirror of install's leaves-first coherence) and re-appends any shll-self target so it stays the final removal. The reverse is *derived* from the single leaves-first `Roster` slice — no second hardcoded dependents-first order to drift.

7. **Report graceful skips.** Each `skipped` name prints `<name>: not installed` to stdout (the shared `notInstalledLabel` from `version.go`). Repair-path semantics: absence is the goal state, so a skip is **not** an error and does not affect the exit code. Printed before the plan/loop so the user sees the full picture.

8. **Nothing actionable → short-circuit.** If the actionable set is empty (an empty roster, or every named target already gone), write `uninstallNothingMsg` (`"Nothing to uninstall."`, mirroring install's `allInstalledMsg`) and return nil (exit 0).

9. **`--dry-run` → preview + exit 0, no write.** Build the preview rows from `previewRowsFor` (per actionable target) and print via `printUninstallPreview`. Returns before the confirmation gate and the removal loop — **bypassing the gate** (a preview mutates nothing, so there is no prompt and no non-TTY refusal). See [`--dry-run`](#dry-run).

10. **Confirmation gate (unless `--yes`).** `printRemovalPlan` prints the plan (per-tool aligned name / formula / version rows), then: if `!stdinIsTTY(stdin)` refuse with `uninstallNoTTYHint` on stderr + `errSilent` (fail-safe for pipes/CI); else `confirmProceed` reads one line and proceeds only on a case-insensitive `y`/`yes`. Any non-affirmative — a negative, whitespace, EOF, or malformed answer — aborts with `uninstallAbortedMsg` on stdout and exit 0, no write. See [The confirmation gate](#the-confirmation-gate).

11. **Best-effort removal loop.** Compute `color := colorEnabled(stdout)` once; `total := len(actionable)`; capture `start := nowFunc()` (the clock seam) after the gate so the duration covers only the removal phase. Per actionable target: a blank line before every header except the first, then `printToolHeader(stdout, name, i+1, total, color)`, then dispatch:
    - `runKit` → `uninstallRunKit(ctx, stdout, stderr, tool)`.
    - `self` → `uninstallOne(ctx, stderr, name, shllFormula)`; on success print the farewell (`shllFarewellFmt` naming `brew install sahil87/tap/shll`).
    - default → `uninstallOne(ctx, stderr, name, tool.Formula)`.
    A failed removal sets `anyFailed`, records nothing further, and `continue`s (skips are never failures). On success `succeeded++`.

12. **Summary tail + print-only hints.** A blank line, then `printSummaryTail(stdout, succeeded, total, nowFunc().Sub(start), color)` (shared with `update`/`install` — exit-code counts + run duration, honesty constraint preserved). Then the [post-run hints](#post-run-hints-print-only). Finally: `anyFailed` → `errSilent` (exit 1); else nil (exit 0).

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All removals succeeded (or nothing-to-do / prompt-abort branch) | 0 |
| Every named target absent (all skipped, none failed) — absence is success | 0 |
| Unknown/typo'd positional target | 1 (via `errSilent`, before any brew work) |
| `brew` not on PATH | 1 (via `errSilent`, hint on stderr) |
| `shll uninstall shll` on a non-brew dev build | 1 (via `errSilent`, `not brew-managed` on stderr) |
| Non-TTY stdin without `--yes` (and not `--dry-run`) | 1 (via `errSilent`, refusal hint on stderr) |
| Any per-tool `brew uninstall` failed | 1 (via `errSilent`, after all actionable tools attempted) |

A prompt abort (a non-affirmative answer) is exit **0** — the user chose not to proceed and nothing was written; it is not a failure.

## Target resolution and ordering

- **Valid targets**: the six `Roster` names (`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit`) **plus** `shll` (`allowShll=true` — `shll uninstall shll` is legal, explicit-only). The legacy alias `rk` resolves to `run-kit` but is never advertised in the valid-targets diagnostic (accepted-but-unadvertised, same contract as `update`/`install`). `shll` is not in `Roster` (Constitution III), so it is handled separately (step 5), never in the no-args sweep.
- **Reverse-roster (dependents before leaves)**: `fab-kit, hop, run-kit, tu, idea, wt`, the mirror of install's leaves-first coherence rationale (removing dependents before the leaves they depend on). Derived by reversing the single leaves-first `Roster` — see [cli/commands §leaves-first Roster order](/cli/commands.md#design-decision-leaves-first-roster-order). A subset is processed in reverse-roster order regardless of arg order.
- **shll-self last**: the running orchestrator is removed after everything it might have managed. `reverseRosterOrder` reverses only the roster portion and re-appends the shll-self target so it stays final even in a mixed `shll uninstall shll hop wt` run.

## The confirmation gate

`shll uninstall` is a destructive verb, so it gates on explicit consent by default (`src/cmd/shll/uninstall.go`).

- **Removal plan** (`printRemovalPlan`): a header (`uninstallPlanHeader = "The following will be uninstalled:"`) then one aligned row per actionable tool — name (padded to the widest), formula, and installed version in parens (`?` when unknown). The formula shown for a run-kit target is its **current** formula; the sweep may also remove a residual `rk` keg (surfaced in the run's foregrounded output). Uses the shared `previewIndent`/`previewGap` from `ui.go`.
- **`Proceed? [y/N] `** (`uninstallProceedPrompt`): `confirmProceed` reads one line via `bufio.NewReader(stdin).ReadString('\n')` and returns true only on a case-insensitive `y`/`yes`. Everything else — negative, whitespace, EOF, `maybe` — is "no" (the fail-safe capital-`N` default). Abort prints `uninstallAbortedMsg = "Aborted — nothing was uninstalled."` and exits 0.
- **`--yes` / `-y`** (`yesFlag`/`yesFlagShorthand`, `cmd.Flags().BoolP`): skips the plan and prompt entirely, proceeding straight to removal (the scripting path).
- **Non-TTY refusal**: when `!stdinIsTTY(stdin)` and neither `--yes` nor `--dry-run` was given, the plan is printed but the prompt cannot be answered, so shll refuses with `uninstallNoTTYHint` on stderr + `errSilent` (fail-safe for pipes/CI) rather than removing without consent.
- **`--dry-run` bypasses the gate** — the dry-run branch returns before the gate, so a preview never prompts and never refuses on a non-TTY stdin (it mutates nothing).

### The `stdinIsTTY` seam (`ui.go`)

The gate reads an **injected** `io.Reader` stdin (`cmd.InOrStdin()` in production; a `strings.Reader`/`bytes.Buffer` in tests) — no global `os.Stdin` reference in command code, matching the established writer-injection test seam. `stdinIsTTY` is a **swappable package-level var** (`var stdinIsTTY = defaultStdinIsTTY`, `src/cmd/shll/ui.go`) mirroring the `proc.Runner` / `nowFunc` injection pattern: `defaultStdinIsTTY(r)` is true only when `r` is a real `*os.File` terminal (mirroring `colorEnabled`'s structure, but *without* the `NO_COLOR` check — that governs styling, not interactivity). A `bytes.Buffer`/`strings.Reader` test reader is never a terminal, so it deterministically hits the non-TTY branch; the swappable var lets a test force the interactive branch so the prompt path is exercisable.

## The run-kit dual-name sweep

The user's "uninstall `rk`, uninstall `run-kit` just to be safe" is handled here — but **never by uninstalling the old name blind**. Post-rename, brew resolves `rk` → `run-kit`, so a blind `brew uninstall sahil87/tap/rk` on a cleanly-migrated machine would delete the *good* keg (the session-observed footgun). run-kit removal is a dedicated **probe-then-act with leaf verification**, mirroring `migrateRunKit`'s structure and reusing the same `probeInstalledLeaf`/`parseBrewLeaf` parser.

`uninstallRunKit(ctx, stdout, stderr, t)` (`src/cmd/shll/uninstall.go`), new name first then residual legacy keg:

1. **Probe `t.Formula`** (`sahil87/tap/run-kit`); if installed → `brew uninstall sahil87/tap/run-kit` (via `uninstallOne`). The current formula is removed by its qualified name, which brew resolves unambiguously to the migrated keg.
2. **Re-probe `t.LegacyFormula`** (`sahil87/tap/rk`) and act **only when its leaf == `t.LegacyName` (`rk`)** — a genuine residual keg, not rename-resolution pointing at the migrated keg already removed in step 1. Remove it by the **legacy leaf name** (`brew uninstall rk`), **never** the qualified `sahil87/tap/rk` (which brew would re-resolve through the rename).

Failure aggregates across both steps; a step is attempted even when the other found no keg. This is the classification the actionable-set builder uses too:

- **`probeRunKitInstalled(ctx, t)`** probes BOTH formulas (no short-circuit on the current keg) and returns `(newInstalled, legacyKeg, version)`. `legacyKeg` is leaf-verified (`legInst && legLeaf == t.LegacyName`). The target is actionable when **either** keg is present, so a **legacy-only machine** (only the `rk` keg, and `shll uninstall run-kit`/`rk`) still counts as installed and removes the residual keg rather than erroring `not installed`. `version` prefers the current keg, falling back to the legacy keg on a legacy-only machine.
- **Dual-rack** (both a `run-kit` keg and a separate `rk` keg): `newInstalled && legacyKeg`, so the sweep removes both — the new formula first, the residual `rk` second. This makes `shll uninstall run-kit` the **supported cleanup** for the dual-rack state that [`shll update`](/cli/update.md#the-rkrun-kit-migration-guard)'s migration guard and [`shll doctor`](/cli/doctor.md) only *warn* about.

### Dry-run preview parity via classification facts

The dry-run preview must render the SAME argv the live sweep would issue. `uninstallTarget` carries the run-kit classification facts (`runKitNewInstalled`/`runKitLegacyKeg`) from `probeRunKitInstalled`, and `previewRowsFor` sources the preview rows from those facts (not a re-probe): a dual-rack target previews BOTH the new-formula uninstall AND the residual `brew uninstall rk`; a legacy-only machine previews **only** `brew uninstall rk` (never a spurious new-formula uninstall). The live `uninstallRunKit` keeps its own independent fresh-state re-probe (fake-runner determinism makes the two agree). Without these facts the preview omitted the residual `rk` removal on a dual-rack machine and mis-previewed a legacy-only machine as a new-formula uninstall.

## shll-self uninstall

`shll uninstall shll` is legal and **explicit-only** — never part of the no-args sweep (Constitution III: shll is not in `Roster`).

- **Brew-managed gate**: `probeInstalledVersion(ctx, shllFormula)`. A `go install`/local-build shll (no brew keg) → `notBrewManagedFmt` (`"shll uninstall: shll: not brew-managed"`) on stderr + `errSilent`. This is the same fact `update.go`'s self-upgrade path keys on; uninstalling a non-brew binary via brew cannot work.
- **Processed last**: appended to the actionable set after the roster, and kept last through `reverseRosterOrder`.
- **Farewell**: on a successful `brew uninstall sahil87/tap/shll`, print `shllFarewellFmt` (`"shll has been uninstalled. Reinstall any time with: <cmd>"`) naming `brew install sahil87/tap/shll` (built via `argvString`). The running process keeps working — unix unlink semantics: the mapped image survives the on-disk removal.

## Output, exit codes, and aggregation

Follows the `per-tool-output-separation` conventions via the shared `ui.go` helpers (see [cli/update §per-tool output separation](/cli/update.md#per-tool-output-separation)):

- **Per-tool header** `printToolHeader(stdout, name, i+1, total, color)` → `▸ [N/M] <tool>` (bold-cyan run on a color TTY) / `==> [N/M] <tool>` (plain ASCII), reverse-roster order, `M = len(actionable)` known up front. A blank line precedes every header except the first.
- **Summary tail** `printSummaryTail(stdout, succeeded, total, elapsed, color)` — `Done — N of M tools succeeded in <dur>.` / `X succeeded, Y failed in <dur> — see above.`, by exit code only (the honesty constraint holds — shll never claims "removed" vs. "was absent" beyond the exit code). A blank line precedes it. Elapsed is measured via the `nowFunc` clock seam (`clock.go`), captured after the gate so it covers only the removal phase.
- **Stream discipline**: headers, tail, plan, prompt, skip lines, and hints go to **stdout** (the stream `brew uninstall` foregrounds onto); the brew-missing / non-TTY / not-brew-managed / unknown-target diagnostics and per-tool transport errors go to **stderr**. `colorEnabled(stdout)` is decided once; `bytes.Buffer` test writers hit the plain-ASCII branch.
- **Failure aggregation** (`update.go` pattern): a per-tool failure (transport error OR non-zero exit — `uninstallOne` checks both, since `proc.RunForeground` returns `(code, nil)` on a non-zero exit) is recorded, the loop continues, and the run returns `errSilent` (exit 1) at the end. Skips (`not installed`) are **not** failures and never flip the exit code. **Named-missing exits 0** — the repair-path inversion of `shll update`.

### The single-source `brewUninstallArgv` builder

`brewUninstallArgv(formula) []string` returns `{brewBinary, "uninstall", formula}` — the single source of truth for the uninstall argv (mirrors `upgradeArgv`). It is threaded into **BOTH** the live run and the dry-run preview so they cannot drift:

- **Live run**: `uninstallOne` builds `argv := brewUninstallArgv(formula)` and runs `proc.RunForeground(ctx, argv[0], argv[1:]...)` (Constitution I — routed through `internal/proc`). The run-kit residual removal uses `brewUninstallArgv(t.LegacyName)` (`brew uninstall rk`).
- **Preview**: `previewRowsFor` renders each row's command via `argvString(brewUninstallArgv(...)...)`.

(Matches the `upgradeArgv`/`upgradeTool` single-source precedent.)

## Post-run hints (print-only)

After the removal loop, `shll uninstall` prints (never executes — Constitution III) up to two hints:

- **run-kit daemon stop** (`runKitDaemonStopHintFmt`): when run-kit was **successfully removed**, note that a running daemon is not stopped (`<tool> serve --stop`). The tool name comes from the actionable entry (`runKitName = a.tool.Name`), not a `"run-kit"` string literal (no magic string).
- **rc-file unwire** (`shellUnwireHint`): when shell-integrated tools (`tu`/`hop`/`wt` — those with a non-empty `Tool.ShellInit`) were **successfully removed roster-wide**, point at `shll shell-setup --uninstall` for the rc-file block.

Two load-bearing properties:

- **Success-gated**: both signals are set only on a tool that actually **removed** (`!failed`), never on a merely-attempted-but-failed removal — mirroring `update`'s daemon-note success gating. shll-self carries no `ShellInit`, so it never trips the rc-unwire signal.
- **Roster-wide keys on roster-set COVERAGE**, not `!subset`: `rosterWide := len(consider) == len(Roster)`. Because `resolveTargets` returns a de-duplicated roster-ordered set, full coverage is exactly the length equality — so a **NAMED full-roster sweep** (all six roster tools listed explicitly) qualifies alongside the no-args case. Scoping the rc-unwire hint to roster-wide avoids a misleading rc-unwiring nudge on a partial subset that may leave other integrated tools present and still wired.

## Non-Goals

- **No untap** of `sahil87/tap` and no trust revocation — the tap stays for reinstall (the whole point of the repair path).
- **No config/state purge** (rk daemon state, hop data, rc-file edits) — brew-uninstall semantics, not `--zap`; a purge flag can layer on later.
- **No stopping of running processes** (run-kit daemon) — print the hint, never execute (Constitution III).
- **No `shll uninstall` → `shll install` composite** — the repair recipe stays two explicit commands.
- **No `--force`/rack-targeted escalation** in v1 — plain `brew uninstall rk` is the assumed-sufficient orphan-removal action.

## Design Decisions

### Confirmation gate reads an injected stdin + a TTY seam
**Decision**: `runUninstall` takes an explicit `stdin io.Reader` (wired from `cmd.InOrStdin()`), and `stdinIsTTY(io.Reader) bool` (a swappable `ui.go` var mirroring `colorEnabled`) detects a non-terminal stdin.
**Why**: matches the established writer-injection test seam — `bytes.Buffer`/`strings.Reader` in tests hit the non-TTY branch deterministically, and the swappable var lets tests force the interactive branch; no global `os.Stdin` reference in command code.
**Rejected**: reading `os.Stdin` directly (untestable without process-level fixtures).
*Introduced by*: `260709-kkaj-shll-uninstall-command`.

### run-kit removal is a dedicated `uninstallRunKit` reusing the leaf parser
**Decision**: probe the current formula and act on leaf `run-kit`, then re-probe the legacy formula and act only on a confirmed `rk` leaf — a single detection path (the shared `probeInstalledLeaf`/`parseBrewLeaf`).
**Why**: mirrors `migrateRunKit`'s structure (Constitution III — one detection path); never a blind old-name uninstall (the session-observed rename-resolution footgun that would delete the migrated keg).
**Rejected**: a blind `brew uninstall sahil87/tap/rk` (deletes the good keg post-rename).
*Introduced by*: `260709-kkaj-shll-uninstall-command`.

### Reverse-roster via slice iteration, not a second declared order
**Decision**: reverse the actionable set built in leaves-first `Roster` order (`reverseRosterOrder`), keeping shll-self last.
**Why**: one source of truth (the `Roster` slice); the dependents-first order is derived, mirroring the leaves-first coherence rationale.
**Rejected**: a second hardcoded dependents-first slice (drift risk).
*Introduced by*: `260709-kkaj-shll-uninstall-command`.

### The Tentative orphan-rack assumption
**Decision**: plain `brew uninstall rk` is assumed sufficient to remove an orphan `rk` rack post-rename; `--force`/rack-targeted escalation is deferred.
**Why**: post-rename brew behavior for an orphan `rk` rack is unverified and cannot be exercised in fake-runner tests (the author's dual-rack machine is the live verification point). The code is shaped (single-source `brewUninstallArgv` builder) so escalation to `--force`/a rack-targeted form is a one-line change in `uninstallRunKit`/`previewRowsFor` if the live run shows plain removal is insufficient.
**Rejected**: pre-emptively adding `--force` (unverified as necessary, and heavier than the observed need warrants).
*Introduced by*: `260709-kkaj-shll-uninstall-command` (plan Assumption 8 / intake Open Question — Tentative).

## Test seam

`uninstall_test.go` drives `runUninstall` with `bytes.Buffer` writers, a `strings.Reader`/`bytes.Buffer` stdin, the shared `fakeRunner`/`installFakeRunner` seam, and `installFakeClock` for the duration tail. No real brew subprocess is spawned. Covered: no-args sweep skips missing + reverse order; targeted named-missing exits 0; unknown-target hard error; prompt abort on `n`; `--yes` bypass; non-TTY refusal; dry-run preview parity + no writes + gate bypass; dual-rack sweep ordering + never-blind-old-name; legacy-keg / `rk`-alias run-kit uninstalls; self-uninstall brew-managed + not-brew-managed gating + farewell; failure aggregation → exit 1 (skips not failures); post-run hints print-only.

## Cross-references

- Counterpart lifecycle command: [cli/install](/cli/install.md) — install/uninstall are the paired bootstrap/repair verbs; `shll uninstall` reuses install's brew helpers and the shared `ui.go` framing.
- The rk→run-kit migration guard whose dual-rack warning this command's cleanup resolves: [cli/update §the migration guard](/cli/update.md#the-rkrun-kit-migration-guard). Its `migrationDualRackNoteFmt` points at `shll uninstall <tool>` + `brew uninstall rk`.
- The hardcoded roster, `resolveTargets`, the legacy alias, and the keg-leaf parser: [cli/commands](/cli/commands.md#the-rkrun-kit-rename--migration-fields).
- Shared UI helpers (header/tail/color/preview + the `stdinIsTTY` seam): [cli/commands §file layout](/cli/commands.md#file-layout-srccmdshll).
- Subprocess wrapper conventions: [internal/proc](/internal/proc.md).
- Constitution I (Security First — all subprocesses via `internal/proc`), III (Wrap, Don't Reinvent — wrap `brew uninstall`; print-don't-run the daemon/rc hints), V (Graceful Degradation — graceful skips, exit-0 aborts, named-missing exits 0), VII (Minimal Surface Area — justified in [cli/commands §Constitution VII per subcommand](/cli/commands.md#constitution-vii-justification-per-subcommand)).
