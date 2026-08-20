---
type: memory
description: "`shll uninstall [tool...]` — the clean-slate repair counterpart to `shll install`: reverse-roster brew removal with shll-self last and delegated (non-brew) entries like rk-desktop skipped with a note, a `Proceed? [y/N]` confirmation gate (`--yes`/`-y`, non-TTY refusal, `--dry-run` bypass), shll-self brew-managed gating, best-effort failure aggregation (named-missing exits 0), print-only daemon/rc-unwire hints, and the single-source `brewUninstallArgv` builder."
---
# cli/uninstall

`shll uninstall [tool...]` — brew-uninstalls shll toolkit tools. It is the **clean-slate repair path** that pairs with [`shll install`](/cli/install.md): when brew state gets wedged (an unlinked keg, a half-failed upgrade), `shll uninstall` removes it cleanly so `shll install` can bootstrap fresh. Absence is a *success* state — the goal "gone" is already met — so a named-but-missing target is not an error (the inverse of `shll update`'s precondition). Delegated (non-brew) roster entries — rk-desktop — carry no brew keg, so they are excluded from removal: naming one (or hitting it in the sweep) prints a skip-with-note line and never runs `brew uninstall`.

Source: `src/cmd/shll/uninstall.go`, with the `stdinIsTTY` seam + `printUninstallPreview` in `src/cmd/shll/ui.go`, the `uninstallBrewMissingHint` in `src/cmd/shll/brew.go`, and wiring in `src/cmd/shll/root.go`.

## Behavior contract

The full happy/unhappy paths, in the order `runUninstall(ctx, stdin io.Reader, stdout, stderr io.Writer, dryRun, yes bool, args []string) error` evaluates them (`src/cmd/shll/uninstall.go:154`):

1. **Resolve targets up front — before `hasBrew` and any probe.** `resolveTargets(args, true)` (`allowShll=true`) validates the positional args and returns `(selected, selfSelected, aliased, err)`. An unknown target → `shll uninstall: <detail>` on stderr + `errSilent` (exit 1) with **no brew side effect**. `subset := len(args) > 0`; empty args yields the whole-roster sweep. The legacy alias `rk` resolves to `run-kit` and is recorded in `aliased` for the notice (below). See [cli/commands §the legacy target alias](/cli/commands.md#the-legacy-target-alias-rk--run-kit) and [cli/update §positional args](/cli/update.md#positional-tool-name-args--subset-targeting).

2. **Brew missing.** If `hasBrew(ctx)` is false, write `uninstallBrewMissingHint` (`"shll uninstall requires Homebrew. Install from https://brew.sh"`, `src/cmd/shll/brew.go`) to stderr and return `errSilent` (exit 1). It is intentionally separate from `brewMissingHint`/`installBrewMissingHint` so each command's error names the command the user ran (the update spec asserts its hint verbatim). The up-front target resolution (step 1) runs *before* this guard, so an unknown target errors first.

3. **Alias notice.** `printAliasNotices(stdout, aliased)` writes one `note: rk is now run-kit` line per aliased token (shared wording with `update`/`install`), before any framing. `resolveTargets` is IO-free; the caller prints.

4. **Build the actionable set + graceful skips.** Iterate `consider` (the full `Roster` for a whole-roster sweep, or `selected` for a subset — both in roster order). A delegated (non-brew) tool — `!t.brewManaged()`, today only rk-desktop — has no brew keg, so it is never actionable: it is recorded in `skipped` without probing, and the skip report (step 7) prints the `delegatedUninstallNote` for it instead of `not installed`. For each brew-managed tool — run-kit included — `probeInstalledVersion(ctx, t.Formula)`: installed → actionable `uninstallTarget{tool, version}`; not installed → recorded in `skipped`. A legacy-`rk`-keg-only machine reports `sahil87/tap/run-kit` not installed, so run-kit is skipped there (orphan-keg cleanup is the manual README path). The probes are **reads**, so they run in dry-run too — only the writes are gated.

5. **shll-self classification (explicit-only, gated).** Only when `selfSelected` (never in the no-args sweep). Gate on `probeInstalledVersion(ctx, shllFormula)`: a `go install`/local-build shll (no brew keg) → `shll uninstall: shll: not brew-managed` on stderr + `errSilent` (the user explicitly asked for it, so absence is a hard error here, unlike a roster tool). Brew-managed → appended **LAST** so it is processed after every roster tool. Mirrors the fact `update.go`'s self-upgrade path keys on.

6. **Reverse-roster ordering.** `reverseRosterOrder(actionable)` reverses only the roster portion and re-appends any shll-self target so it stays the final removal. The reverse is *derived* from the single importance-descending `Roster` slice — no second hardcoded order to drift.

7. **Report graceful skips.** Each `skipped` name prints one line to stdout: a delegated (non-brew) tool prints `<name>: not brew-managed — remove it via its own manager (rk desktop)` (`delegatedUninstallNote`); anything else prints `<name>: not installed` (the shared `notInstalledLabel` from `version.go`). Repair-path semantics: absence is the goal state, so a skip is **not** an error and does not affect the exit code. Printed before the plan/loop so the user sees the full picture.

8. **Nothing actionable → short-circuit.** If the actionable set is empty (an empty roster, or every named target already gone), write `uninstallNothingMsg` (`"Nothing to uninstall."`, mirroring install's `allInstalledMsg`) and return nil (exit 0).

9. **`--dry-run` → preview + exit 0, no write.** Build the preview rows from `previewRowsFor` (per actionable target) and print via `printUninstallPreview`. Returns before the confirmation gate and the removal loop — **bypassing the gate** (a preview mutates nothing, so there is no prompt and no non-TTY refusal). See [`--dry-run`](#dry-run).

10. **Confirmation gate (unless `--yes`).** `printRemovalPlan` prints the plan (per-tool aligned name / formula / version rows), then: if `!stdinIsTTY(stdin)` refuse with `uninstallNoTTYHint` on stderr + `errSilent` (fail-safe for pipes/CI); else `confirmProceed` reads one line and proceeds only on a case-insensitive `y`/`yes`. Any non-affirmative — a negative, whitespace, EOF, or malformed answer — aborts with `uninstallAbortedMsg` on stdout and exit 0, no write. See [The confirmation gate](#the-confirmation-gate).

11. **Best-effort removal loop.** Compute `color := colorEnabled(stdout)` once; `total := len(actionable)`; capture `start := nowFunc()` (the clock seam) after the gate so the duration covers only the removal phase. Per actionable target: a blank line before every header except the first, then `printToolHeader(stdout, name, i+1, total, color)`, then dispatch:
    - `self` → `uninstallOne(ctx, stderr, name, shllFormula)`; on success print the farewell (`shllFarewellFmt` naming `brew install sahil87/tap/shll`).
    - default (every brew-managed roster tool, run-kit included) → `uninstallOne(ctx, stderr, name, tool.Formula)`.
    A failed removal sets `anyFailed`, records nothing further, and `continue`s (skips are never failures). On success `succeeded++`, and if the removed tool's name matches `runKitToolName` the [daemon-stop hint](#post-run-hints-print-only) is armed.

12. **Summary tail + print-only hints.** A blank line, then `printSummaryTail(stdout, succeeded, total, nowFunc().Sub(start), color)` (shared with `update`/`install` — exit-code counts + run duration, honesty constraint preserved). Then the [post-run hints](#post-run-hints-print-only). Finally: `anyFailed` → `errSilent` (exit 1); else nil (exit 0).

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All removals succeeded (or nothing-to-do / prompt-abort branch) | 0 |
| Every named target absent (all skipped, none failed) — absence is success | 0 |
| Delegated (non-brew) target named or swept (rk-desktop) — skip-with-note, never `brew uninstall`ed | 0 |
| Unknown/typo'd positional target | 1 (via `errSilent`, before any brew work) |
| `brew` not on PATH | 1 (via `errSilent`, hint on stderr) |
| `shll uninstall shll` on a non-brew dev build | 1 (via `errSilent`, `not brew-managed` on stderr) |
| Non-TTY stdin without `--yes` (and not `--dry-run`) | 1 (via `errSilent`, refusal hint on stderr) |
| Any per-tool `brew uninstall` failed | 1 (via `errSilent`, after all actionable tools attempted) |

A prompt abort (a non-affirmative answer) is exit **0** — the user chose not to proceed and nothing was written; it is not a failure.

## Target resolution and ordering

- **Valid targets**: the seven `Roster` names (`run-kit`, `rk-desktop`, `fab-kit`, `wt`, `idea`, `tu`, `hop`) **plus** `shll` (`allowShll=true` — `shll uninstall shll` is legal, explicit-only). rk-desktop is a valid named target but never actionable — naming it (or hitting it in the no-args sweep) prints the `delegatedUninstallNote` skip line without affecting the exit code; there is no `rk desktop uninstall` delegation. The legacy alias `rk` resolves to `run-kit` but is never advertised in the valid-targets diagnostic (accepted-but-unadvertised, same contract as `update`/`install`). `shll` is not in `Roster` (Constitution III), so it is handled separately (step 5), never in the no-args sweep.
- **Reverse-roster order**: `hop, tu, idea, wt, fab-kit, rk-desktop, run-kit` — the reverse of the single importance-descending `Roster` (adjacency places rk-desktop before the run-kit runtime it delegates to, though in practice it skips-with-note rather than entering the actionable set). Derived by reversing the single `Roster` slice — see [cli/commands §hardcoded tool roster](/cli/commands.md#hardcoded-tool-roster). A subset is processed in reverse-roster order regardless of arg order.
- **shll-self last**: the running orchestrator is removed after everything it might have managed. `reverseRosterOrder` reverses only the roster portion and re-appends the shll-self target so it stays final even in a mixed `shll uninstall shll hop wt` run.

## The confirmation gate

`shll uninstall` is a destructive verb, so it gates on explicit consent by default (`src/cmd/shll/uninstall.go`).

- **Removal plan** (`printRemovalPlan`): a header (`uninstallPlanHeader = "The following will be uninstalled:"`) then one aligned row per actionable tool — name (padded to the widest), formula, and installed version in parens (`?` when unknown). run-kit's row shows its current formula `sahil87/tap/run-kit` like any other tool. Uses the shared `previewIndent`/`previewGap` from `ui.go`.
- **`Proceed? [y/N] `** (`uninstallProceedPrompt`): `confirmProceed` reads one line via `bufio.NewReader(stdin).ReadString('\n')` and returns true only on a case-insensitive `y`/`yes`. Everything else — negative, whitespace, EOF, `maybe` — is "no" (the fail-safe capital-`N` default). Abort prints `uninstallAbortedMsg = "Aborted — nothing was uninstalled."` and exits 0.
- **`--yes` / `-y`** (`yesFlag`/`yesFlagShorthand`, `cmd.Flags().BoolP`): skips the plan and prompt entirely, proceeding straight to removal (the scripting path).
- **Non-TTY refusal**: when `!stdinIsTTY(stdin)` and neither `--yes` nor `--dry-run` was given, the plan is printed but the prompt cannot be answered, so shll refuses with `uninstallNoTTYHint` on stderr + `errSilent` (fail-safe for pipes/CI) rather than removing without consent.
- **`--dry-run` bypasses the gate** — the dry-run branch returns before the gate, so a preview never prompts and never refuses on a non-TTY stdin (it mutates nothing).

### The `stdinIsTTY` seam (`ui.go`)

The gate reads an **injected** `io.Reader` stdin (`cmd.InOrStdin()` in production; a `strings.Reader`/`bytes.Buffer` in tests) — no global `os.Stdin` reference in command code, matching the established writer-injection test seam. `stdinIsTTY` is a **swappable package-level var** (`var stdinIsTTY = defaultStdinIsTTY`, `src/cmd/shll/ui.go`) mirroring the `proc.Runner` / `nowFunc` injection pattern: `defaultStdinIsTTY(r)` is true only when `r` is a real `*os.File` terminal (mirroring `colorEnabled`'s structure, but *without* the `NO_COLOR` check — that governs styling, not interactivity). A `bytes.Buffer`/`strings.Reader` test reader is never a terminal, so it deterministically hits the non-TTY branch; the swappable var lets a test force the interactive branch so the prompt path is exercisable.

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

- **Live run**: `uninstallOne` builds `argv := brewUninstallArgv(formula)` and runs `proc.RunForeground(ctx, argv[0], argv[1:]...)` (Constitution I — routed through `internal/proc`). run-kit is removed by its current formula `sahil87/tap/run-kit` like any other tool.
- **Preview**: `previewRowsFor` renders each row's command via `argvString(brewUninstallArgv(...)...)`.

(Matches the `upgradeArgv`/`upgradeTool` single-source precedent.)

## Post-run hints (print-only)

After the removal loop, `shll uninstall` prints (never executes — Constitution III) up to two hints:

- **run-kit daemon stop** (`runKitDaemonStopHintFmt`): when run-kit was **successfully removed**, note that a running daemon is not stopped (`<tool> serve --stop`). The hint is keyed on the removed roster entry's name matching the `runKitToolName` named constant (`a.tool.Name == runKitToolName` on the success path) — `runKitName` records the name from that entry, not a `"run-kit"` string literal (no magic string).
- **rc-file unwire** (`shellUnwireHint`): when shell-integrated tools (`tu`/`hop`/`wt` — those with a non-empty `Tool.ShellInit`) were **successfully removed roster-wide**, point at `shll setup shell --uninstall` for the rc-file block.

Two load-bearing properties:

- **Success-gated**: both signals are set only on a tool that actually **removed** (`!failed`), never on a merely-attempted-but-failed removal — mirroring `update`'s daemon-note success gating. shll-self carries no `ShellInit`, so it never trips the rc-unwire signal.
- **Roster-wide keys on roster-set COVERAGE**, not `!subset`: `rosterWide := len(consider) == len(Roster)`. Because `resolveTargets` returns a de-duplicated roster-ordered set, full coverage is exactly the length equality — so a **NAMED full-roster sweep** (all seven roster tools listed explicitly) qualifies alongside the no-args case. Scoping the rc-unwire hint to roster-wide avoids a misleading rc-unwiring nudge on a partial subset that may leave other integrated tools present and still wired.

## Non-Goals

- **No untap** of `sahil87/tap` and no trust revocation — the tap stays for reinstall (the whole point of the repair path).
- **No config/state purge** (rk daemon state, hop data, rc-file edits) — brew-uninstall semantics, not `--zap`; a purge flag can layer on later.
- **No stopping of running processes** (run-kit daemon) — print the hint, never execute (Constitution III).
- **No `shll uninstall` → `shll install` composite** — the repair recipe stays two explicit commands.
- **No `rk desktop uninstall` delegation** — rk-desktop is excluded with a skip-with-note line (see the Design Decision); uninstall's contract is brew-keg removal and rk-desktop has no keg.
- **No orphan-`rk`-keg sweep** — run-kit is a plain `sahil87/tap/run-kit` target; a residual legacy `rk` keg is manual cleanup per run-kit's README (`brew uninstall sahil87/tap/rk`), not a shll action.

## Design Decisions

### Confirmation gate reads an injected stdin + a TTY seam
**Decision**: `runUninstall` takes an explicit `stdin io.Reader` (wired from `cmd.InOrStdin()`), and `stdinIsTTY(io.Reader) bool` (a swappable `ui.go` var mirroring `colorEnabled`) detects a non-terminal stdin.
**Why**: matches the established writer-injection test seam — `bytes.Buffer`/`strings.Reader` in tests hit the non-TTY branch deterministically, and the swappable var lets tests force the interactive branch; no global `os.Stdin` reference in command code.
**Rejected**: reading `os.Stdin` directly (untestable without process-level fixtures).
*Introduced by*: `260709-kkaj-shll-uninstall-command`.

### Reverse-roster via slice iteration, not a second declared order
**Decision**: reverse the actionable set built in `Roster` order (`reverseRosterOrder`), keeping shll-self last.
**Why**: one source of truth — the reverse is derived from the single importance-descending `Roster` slice, so no second declared order can drift from it.
**Rejected**: a second hardcoded reverse-roster slice (drift risk).
*Introduced by*: `260709-kkaj-shll-uninstall-command`.

### Delegated (non-brew) entries skip with a note, never `brew uninstall`
**Decision**: `shll uninstall` excludes formula-less roster entries (rk-desktop) from the actionable set — a named or swept rk-desktop prints the `delegatedUninstallNote` skip line and does not affect the exit code; there is no `rk desktop uninstall` delegation.
**Why**: uninstall is the brew clean-slate repair path — its contract is brew-keg removal, and rk-desktop has no keg. Whether `rk desktop uninstall` even exists is unverified; delegating would invent a contract.
**Rejected**: delegating to `rk desktop uninstall` (an unverified subcommand — inventing a removal contract uninstall does not own).
*Introduced by*: `260820-t26g-roster-desktop-entry`.

### run-kit is a plain reverse-roster target
**Decision**: run-kit takes the same `probeInstalledVersion` + `uninstallOne(t.Name, t.Formula)` path as every other tool — no dedicated dual-name sweep, no leaf verification, no residual-`rk` removal.
**Why**: the rk→run-kit migration window is closed, so shll no longer probes or acts on the legacy `sahil87/tap/rk` formula anywhere (retiring the guard removed brew's permanent rename warning — see [cli/update §Retire the migration guard](/cli/update.md#retire-the-rkrun-kit-brew-formula-migration-guard)). A residual `rk` keg is manual cleanup per run-kit's README.
**Rejected**: keeping the leaf-verified `uninstallRunKit`/`probeRunKitInstalled` sweep — dead machinery once the migration guard is retired, and it referenced the legacy formula that produces the warning.
*Introduced by*: `260720-h3f6-retire-rk-migration-guard`.

## Test seam

`uninstall_test.go` drives `runUninstall` with `bytes.Buffer` writers, a `strings.Reader`/`bytes.Buffer` stdin, the shared `fakeRunner`/`installFakeRunner` seam, and `installFakeClock` for the duration tail. No real brew subprocess is spawned. Covered: no-args sweep skips missing + reverse order; targeted named-missing exits 0; unknown-target hard error; prompt abort on `n`; `--yes` bypass; non-TTY refusal; dry-run preview parity + no writes + gate bypass; run-kit as a plain target (+ `rk`-alias resolution + daemon-stop hint on successful removal + a legacy-`rk`-keg-only machine treated as run-kit not installed, no `sahil87/tap/rk` reference); the rk-desktop delegated exclusion — a targeted `shll uninstall rk-desktop` prints the skip-with-note line, exits 0, and runs no `brew uninstall` (`TestUninstall_RkDesktopTargetedSkipsWithNote`), and the no-args sweep prints the note alongside the not-installed lines while still removing the installed brew tools (`TestUninstall_WholeRosterSweepSkipsRkDesktop`); self-uninstall brew-managed + not-brew-managed gating + farewell; failure aggregation → exit 1 (skips not failures); post-run hints print-only.

## Cross-references

- Counterpart lifecycle command: [cli/install](/cli/install.md) — install/uninstall are the paired bootstrap/repair verbs; `shll uninstall` reuses install's brew helpers and the shared `ui.go` framing.
- The hardcoded roster, `resolveTargets`, and the `rk` legacy alias: [cli/commands](/cli/commands.md#the-rkrun-kit-rename).
- Shared UI helpers (header/tail/color/preview + the `stdinIsTTY` seam): [cli/commands §file layout](/cli/commands.md#file-layout-srccmdshll).
- Subprocess wrapper conventions: [internal/proc](/internal/proc.md).
- Constitution I (Security First — all subprocesses via `internal/proc`), III (Wrap, Don't Reinvent — wrap `brew uninstall`; print-don't-run the daemon/rc hints), V (Graceful Degradation — graceful skips, exit-0 aborts, named-missing exits 0), VII (Minimal Surface Area — justified in [cli/commands §Constitution VII per subcommand](/cli/commands.md#constitution-vii-justification-per-subcommand)).
