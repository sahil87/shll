# Intake: shll doctor — verify toolkit install + wiring

**Change**: 260609-d0ct-doctor-verify-install
**Created**: 2026-06-09
**Status**: Draft

## Origin

This change is backlog item `[d0ct]` (recorded 2026-06-03 in `fab/backlog.md`), invoked
one-shot via `/fab-new` with the backlog description as the prompt. No prior `/fab-discuss`
session — the design below is derived from the description plus a read of the existing
`version`, `shell-init`, and `shell-setup` implementations (the three commands `doctor` reuses).

> Add a `shll doctor` command that verifies the toolkit is correctly installed and wired. For
> every roster tool (idea, hop, fab-kit, wt, run-kit, tu), check: (1) the binary is on PATH, (2)
> it reports a version (so a half-installed/stale brew link is caught), and (3) its shell
> integration is wired where applicable (the composed `shll shell-init` eval block is present in
> the user's rc file — relevant for hop/wt/tu/idea which ship shell-init). Output one line per
> tool with a clear OK / WARN / FAIL marker; each non-OK line MUST carry an actionable suggestion
> (e.g. `brew install sahil87/tap/<tool>`, or `run 'shll shell-setup' then 'exec $SHELL'`). Exit
> non-zero if any tool is FAIL so it's scriptable in CI. Motivation: the shll.ai install + idea
> pages referenced `shll doctor` as the post-install verification step, but the command does not
> exist yet — this closes that gap. Pairs with `[lst7]` (`shll list`) below.

## Why

1. **The pain point — a referenced command that doesn't exist.** The shll.ai install page and the
   `idea` tool page both name `shll doctor` as *the* post-install verification step. A user who
   follows the documented onboarding flow runs `shll doctor`, gets `unknown command "doctor"`, and
   loses trust in the docs on their first interaction. The command is a dangling reference.

2. **The consequence of not fixing it — silent half-installs stay silent.** Today nothing in the
   toolkit answers "is everything actually wired up?" A stale or broken brew link (binary present
   but `--version` fails), a tool that brew installed but whose shell-init was never added to the
   rc file, or a tool simply not installed all fail *quietly* — the user discovers them later, one
   confusing symptom at a time. `shll version` shows installed-vs-not but not *wiring*, and treats
   a non-running binary identically to a missing one (both print `not installed`). There is no
   single "green check" verification surface.

3. **Why this approach (a new top-level subcommand) over alternatives.** See the Constitution VII
   justification in **Impact**. Briefly: `doctor` is a read-only *diagnostic* with its own
   lifecycle (verify, never mutate) and its own output contract (per-tool OK/WARN/FAIL + a non-zero
   exit for CI). It is not a flag on `version` (verification ≠ version reporting — different output,
   different exit semantics, different checks), not a flag on `install`/`update` (those mutate; a
   doctor must never write), and cannot live in a per-tool CLI (its value is the *cross-tool
   aggregation* plus the shll-owned rc-block wiring check — exactly what shll exists for).

4. **Why now / why cheap.** Every check `doctor` performs is already implemented and tested
   elsewhere in the binary — the PATH+version probe in `version.go`, and the rc-file wiring
   detection in `shell_setup.go`. `doctor` is overwhelmingly *composition of existing, single-
   sourced primitives*, not new mechanism. This keeps it aligned with Constitution III (Wrap, Don't
   Reinvent) and makes drift between `doctor` and the commands it diagnoses structurally hard.

## What Changes

A new read-only top-level subcommand `shll doctor`. It takes no positional arguments and one flag,
`--json` (see **JSON output mode** below). It walks the hardcoded `Roster`
(`src/cmd/shll/tools.go`) in roster order, runs up to three checks per tool, prints one status line
per tool (or a JSON array with `--json`), and exits non-zero if **any** tool's worst check is FAIL.

### The three per-tool checks

For each `Tool` in `Roster`:

1. **Binary on PATH.** Reuse the same mechanism `version`/`shell-init` use: invoke the tool and
   treat `proc.ErrNotFound` as "missing" (install-mechanism agnostic — brew, from-source, etc.).
   Combined with check 2 below into a single `<tool> --version` invocation (one subprocess per
   tool, matching `toolVersion`).

2. **Reports a version.** The binary on PATH must *also* successfully report a version — this is
   what catches a half-installed / stale-brew-link binary that exists but errors, times out, or
   prints nothing. Reuse `version.go`'s `proc.Run(ctx, tool.Name, "--version")` bounded by
   `versionTimeout` (2s) and `normalizeVersion`. The mapping:
   - `proc.Run` returns `ErrNotFound` → **binary missing** (check 1 fails).
   - `proc.Run` returns any other error / timeout, OR `normalizeVersion` yields `""` → **binary on
     PATH but version unreportable** (check 1 passes, check 2 fails — the "stale link" case the
     description specifically calls out).
   - `proc.Run` succeeds and `normalizeVersion` is non-empty → checks 1 and 2 both pass; capture the
     version string for display.

3. **Shell integration wired (only for tools that ship shell-init).** "Wired" means shll's *own*
   composed eval block is present in the user's rc file — i.e. the rc file contains the
   `# >>> shll >>>` sentinel block (or the legacy `# >>> shll shell-init >>>` block) with the
   `eval "$(shll shell-init <shell>)"` line. This is a **single rc-file fact** shared by all
   shell-init tools (the block composes them all), evaluated once and attributed to each
   shell-init-shipping tool's line. Reuse `shell_setup.go`'s `resolveShell(args, os.Getenv)` →
   `resolveRcFile(shell, os.Getenv)` → read file → `locateBlock(content)` → `blockMatch.hasEval`.

> **IMPORTANT — roster discrepancy (must follow source of truth, not the description).** The
> backlog description says shell-init is "relevant for **hop/wt/tu/idea**." This is **factually
> wrong about `idea`.** The live `Roster` (`src/cmd/shll/tools.go:76–83`, corroborated by
> `docs/memory/cli/shell-init.md` and `docs/memory/cli/commands.md`) shows the shell-init
> integrators are exactly **`wt`, `tu`, `hop`** — `idea`'s `ShellInit` slice is **empty**. Per
> Constitution III (Tool Roster Source of Truth), the implementation MUST derive "ships shell-init"
> from `len(tool.ShellInit) > 0`, NOT from the prose list. So `idea` gets **only** checks 1+2 (no
> wiring check), same as `rk` and `fab-kit`. "run-kit" in the description is the binary/formula
> `rk` (same tool). This keeps `doctor` correct even as the roster evolves.

### Per-tool status derivation (worst-check-wins)

Each tool's line shows a single marker = the worst of its applicable checks:

| Condition | Marker | Meaning |
|-----------|--------|---------|
| Binary missing (check 1 fails) | **FAIL** | not installed / not on PATH |
| Binary present but version unreportable (check 2 fails) | **FAIL** | half-installed / stale brew link |
| All applicable checks pass, but shell-init tool whose wiring is absent (check 3 fails) | **WARN** | installed & runnable, but not wired into the shell |
| All applicable checks pass | **OK** | installed, runnable, (and wired if applicable) |

Rationale for FAIL vs WARN split: a missing/broken binary is a hard install problem (FAIL → drives
the non-zero exit). Missing shell wiring is real but non-fatal — the tool *works* when invoked
directly; it just isn't auto-sourced — so it is a WARN and does **not** by itself fail the exit
code. <!-- clarified: unwired-shell-integration is WARN-not-FAIL (exit stays 0) — user confirmed: the binary is functional, wiring is a convenience; keeps `doctor` green-for-CI when a tool is installed but the user hasn't run shell-setup. -->

### Output format

One line per tool, aligned (reuse the `text/tabwriter` approach from `version.go`), in roster
order. Each non-OK line MUST carry an actionable suggestion. Illustrative (final wording settled
in plan):

```
$ shll doctor
wt       OK    v1.4.0   wired
idea     OK    v0.3.1
tu       WARN  v2.0.0   not wired — run 'shll shell-setup' then 'exec $SHELL'
rk       OK    v0.9.2
hop      FAIL  not installed — run 'brew install sahil87/tap/hop'
fab-kit  FAIL  installed but 'fab-kit --version' failed — try 'brew reinstall sahil87/tap/fab-kit'

1 of 6 tools have problems. Run the suggested commands above, then re-run shll doctor.
```

Suggestion text by failure mode (exact strings finalized in plan; named constants per
code-quality.md — no magic strings):
- **Binary missing** → `run 'brew install sahil87/tap/<formula-leaf>'` (use `tool.Formula`).
- **Version unreportable** → `installed but '<tool> --version' failed — try 'brew reinstall
  sahil87/tap/<formula-leaf>'`.
- **Shell-init not wired** → `not wired — run 'shll shell-setup' then 'exec $SHELL'`.

Color: reuse `ui.go`'s `colorEnabled(w)` (TTY + `NO_COLOR`) gating, mirroring the existing
`printSummaryTail`/`printToolHeader` discipline — a green check / glyph only on a real TTY, plain
ASCII markers (`OK`/`WARN`/`FAIL`) otherwise. `doctor`'s output is human-facing (not eval'd), so —
unlike `shell-init` — it MAY use the color/glyph path. <!-- assumed: doctor may use ui.go color path because its stdout is human-facing, not eval-consumed; the shell-init eval-safety exception does not apply. -->

### Exit-code contract (scriptable in CI)

- **Exit 0** — no tool is FAIL (every tool is OK or WARN). WARN alone does NOT fail the exit.
- **Exit 1** — at least one tool is FAIL. Reuse the `errSilent` sentinel (the command writes its own
  per-tool diagnostics to stdout, then returns `errSilent` → `translateExit` maps to 1 without an
  extra stderr line). <!-- clarified: any-FAIL → exit 1 via errSilent; WARN does not affect exit (user confirmed both unwired-shell-init and bad-$SHELL are WARN/exit-0). "Exit non-zero" is satisfied by 1; no distinct code per failure class in v1. -->

The exit code is computed the **same way for text and `--json` output** — `--json` changes only the
rendering, never the check logic or the exit contract.

### JSON output mode (`--json`)

`shll doctor --json` emits a machine-readable JSON array (one object per roster tool, in roster
order) instead of the aligned text table, so CI can parse structured per-tool results rather than
scraping text. The flag is a cobra bool on the `doctor` command — a **flag on the new command, not a
second subcommand** (Constitution VII's "could this be a flag?" test is satisfied). Per-object shape
(exact field set finalized in plan; `encoding/std` `json` marshal of a typed struct, no hand-rolled
string building):

```json
[
  {"tool": "wt",  "status": "OK",   "version": "v1.4.0", "on_path": true,  "version_ok": true,  "shell_init": true,  "wired": true,  "suggestion": ""},
  {"tool": "tu",  "status": "WARN", "version": "v2.0.0", "on_path": true,  "version_ok": true,  "shell_init": true,  "wired": false, "suggestion": "run 'shll shell-setup' then 'exec $SHELL'"},
  {"tool": "hop", "status": "FAIL", "version": "",       "on_path": false, "version_ok": false, "shell_init": true,  "wired": false, "suggestion": "run 'brew install sahil87/tap/hop'"}
]
```

- `shell_init` is `true` exactly when `len(tool.ShellInit) > 0` (so `idea`/`rk`/`fab-kit` are
  `false`, and `wired`/wiring-suggestion are not meaningful for them — `wired:false`, no wiring
  suggestion).
- `--json` is still gated by the same per-tool checks and the same any-FAIL→exit-1 contract; only
  the output rendering differs. `--json` output is plain JSON to stdout with **no ANSI color**
  regardless of TTY (machine consumers must get clean JSON).
- Diagnostics that the text path prints inline are carried in the `suggestion` field; nothing
  extraneous is written to stdout in `--json` mode.
<!-- clarified: --json included in v1 (user chose it over deferring to `shll list`). Flag on doctor, not a new subcommand; same checks + same exit contract as text; no color in JSON. -->

### Shell resolution edge case

The wiring check needs a shell to resolve the rc path. `doctor` takes no shell argument, so it
infers from `$SHELL` via the existing `resolveShell([], os.Getenv)`. If `$SHELL` is unsupported or
unset (e.g. CI with `$SHELL=/bin/sh`), the wiring check cannot run. Treat an unresolvable shell as:
shell-init tools' wiring shows **WARN** (`cannot verify shell wiring — $SHELL is <x>; pass a
supported shell environment or run 'shll shell-setup zsh'`), the binary checks still run normally,
and the exit code is unaffected (WARN, not FAIL). This keeps `doctor` usable in CI where the binary
checks are the point and shell wiring is irrelevant. <!-- clarified: unresolvable/unsupported $SHELL degrades the wiring check to a WARN with an explanatory suggestion; binary checks proceed and exit is unaffected (user confirmed over hard-error/exit-2). -->

### Registration

Wire `newDoctorCmd()` into `newRootCmd()` (`root.go`) alongside the existing subcommands, and add
its one-line summary to `rootLong`. The command registers a single `--json` bool flag. Follow the
established `newXxxCmd()` factory pattern with a thin `runDoctor(ctx, jsonOut bool, stdout, stderr)`
seam taking explicit `io.Writer`(s) for the `bytes.Buffer` + fake-`proc.Runner` test pattern.

## Affected Memory

- `cli/doctor`: (new) Behavior contract for `shll doctor` — the three checks, worst-check-wins
  marker derivation, OK/WARN/FAIL + suggestion strings, the `--json` output mode (per-tool object
  shape, no-color invariant, same checks + same exit contract as text), the any-FAIL→exit-1
  contract, the unwired→WARN and `$SHELL`-degrades-to-WARN edge cases (both exit-0), and the
  explicit note that "ships shell-init" derives from `tool.ShellInit` (so `idea` is correctly
  excluded from the wiring check, contra the backlog prose).
- `cli/commands`: (modify) Bump the subcommand count (five → six) and add `doctor` to the
  registration list, the Constitution VII justification table, and the `File layout` table
  (`doctor.go`). Note `doctor` reuses `version.go`'s probe and `shell_setup.go`'s wiring detection.
- `cli/version`: (modify) Cross-reference — `doctor` reuses `toolVersion`/`normalizeVersion`/
  `versionTimeout` for its binary+version checks; note the shared probe so the two cannot drift.
- `cli/shell-setup`: (modify) Cross-reference — `doctor`'s wiring check reuses `resolveShell`,
  `resolveRcFile`, `locateBlock`, and `blockMatch.hasEval` (read-only; `doctor` never writes).

## Impact

**Code areas:**
- `src/cmd/shll/doctor.go` (new) — `newDoctorCmd()` (registers `--json`) + `runDoctor` + per-tool
  check logic + the typed result struct (text + JSON render from the same struct) +
  marker/suggestion constants.
- `src/cmd/shll/doctor_test.go` (new) — test-alongside (code-quality.md). Drives `runDoctor` with
  `bytes.Buffer` and a fake `proc.Runner`; covers each marker path, the worst-check-wins precedence,
  exit-code (any-FAIL→1, WARN-only→0), the `idea`-has-no-wiring-check assertion, the
  unresolvable-`$SHELL` degradation, and `--json` output (valid JSON, field values per marker, same
  exit code as text, no ANSI). Use `t.TempDir()` rc files for the wiring check (never touch the real
  `~/.zshrc`).
- `src/cmd/shll/root.go` (modify) — register `newDoctorCmd()`; add the summary line to `rootLong`.

**Reused primitives (no new mechanism — Constitution III):**
- `version.go`: `toolVersion`, `normalizeVersion`, `versionTimeout`, `notInstalledLabel`.
- `shell_setup.go`: `resolveShell`, `resolveRcFile`, `locateBlock`, `blockMatch.hasEval`,
  `evalLinePrefix` (all read-only here).
- `ui.go`: `colorEnabled` and the ANSI constants for optional TTY color.
- `tools.go`: `Roster`, `Tool.ShellInit` (the source of truth for "ships shell-init"),
  `Tool.Formula`, `formulaPrefix`.
- `main.go`: `errSilent` for the exit-1 path (no new error sentinel needed).
- `internal/proc`: `Run` + `ErrNotFound` (all subprocess work stays routed through proc —
  Constitution I).

**Dependencies:** none new. No new Go modules; reuses `golang.org/x/term` (already present via
`ui.go`) only transitively through `colorEnabled`.

**Constitution VII — Minimal Surface Area justification (REQUIRED for a new subcommand).**
`shll doctor` solves the post-install verification gap referenced by the shll.ai docs. It cannot be
a flag on `version` — verification is a different concern (wiring checks, OK/WARN/FAIL semantics, a
CI-meaningful non-zero exit) from version *reporting* (a plain table that always exits 0). It cannot
be a flag on `install`/`update` — those are *mutating* commands, whereas a doctor must be strictly
read-only. It cannot live in a per-tool CLI — its value is the cross-tool aggregation plus the
shll-owned rc-block wiring check (no single tool can see "is the *composed* shll block present?").
This raises the user-facing surface from five to six subcommands (the hidden `help-dump` is build
tooling, not part of the count); the bar is met because the capability is genuinely new, read-only,
and cross-cutting. Surface-count bookkeeping updated in `cli/commands`.

**Cross-platform:** the only platform-specific path is rc-file derivation, already isolated behind
`resolveRcFile`/`osGoos` — `doctor` inherits it for free, adds none of its own.

## Open Questions

All three intake-time design questions were resolved with the user during `/fab-new` (see the
clarified markers and the Certain-graded assumptions 10/11/13):

- **Unwired shell-init → WARN/exit-0** (vs FAIL). Resolved: WARN, exit stays 0.
- **Unresolvable `$SHELL` → WARN/exit-0** (vs hard error exit 2). Resolved: WARN, binary checks
  still run, exit unaffected.
- **`--json` in v1** (vs defer to `[lst7]`). Resolved: included in v1 as a `--json` flag on `doctor`.

Remaining (deferred to plan time, not blocking):

- Final exact wording of the per-tool text line, markers, JSON field names, and suggestion strings —
  settled at plan time with named constants; the intake fixes the *semantics* and illustrative
  shapes only.
- Whether the binary+version probe should be factored into a tiny shared helper now (so `[lst7]`
  `shll list` can reuse it cleanly) or left in `version.go` and called directly — a refactor detail
  for plan time; either way the probe stays single-sourced.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Reuse `version.go`'s `toolVersion`/`normalizeVersion`/`versionTimeout` for the PATH+version checks rather than reimplementing | Constitution III + code-quality.md mandate single-sourcing; the probe already exists and is tested | S:95 R:90 A:95 D:95 |
| 2 | Certain | Reuse `shell_setup.go`'s `resolveShell`/`resolveRcFile`/`locateBlock`/`hasEval` (read-only) for the wiring check | Same single-source mandate; the wiring detector already exists. `doctor` never writes — read-only reuse | S:95 R:90 A:95 D:90 |
| 3 | Certain | "Ships shell-init" derives from `len(tool.ShellInit) > 0`, so `idea` is NOT wiring-checked despite the description listing it | Constitution III: Roster is source of truth. Live Roster + memory confirm only wt/tu/hop integrate; the prose is factually wrong about idea | S:90 R:80 A:98 D:95 |
| 4 | Certain | "run-kit" in the description denotes the roster tool `rk` (binary + formula `sahil87/tap/rk`) | Confirmed by Roster, context.md, and commands.md — rk is run-kit's binary name | S:95 R:95 A:98 D:98 |
| 5 | Certain | New top-level subcommand justified under Constitution VII (read-only, cross-tool, distinct from version/install/update) | Constitution VII requires the justification; verification is a distinct concern with distinct output + exit semantics | S:85 R:70 A:90 D:90 |
| 6 | Certain | `doctor` takes no positional args and one flag, `--json`; infers shell from `$SHELL` for the wiring check | Clarified — user chose to include `--json` in v1. No shell positional in v1 (a flag could be added later, cheaply) | S:90 R:80 A:90 D:90 |
| 7 | Confident | Per-tool marker = worst applicable check (FAIL > WARN > OK) on a single line, roster order, tabwriter-aligned | "one line per tool with a clear OK/WARN/FAIL marker" — worst-wins is the natural single-marker rule; mirrors version.go's tabwriter | S:75 R:75 A:80 D:75 |
| 8 | Confident | Exit 0 unless ≥1 tool is FAIL (exit 1 via `errSilent`); WARN never fails the exit | "Exit non-zero if any tool is FAIL" — names FAIL specifically, so WARN is excluded; errSilent is the established exit-1 path | S:80 R:75 A:85 D:80 |
| 9 | Confident | `doctor` MAY use `ui.go`'s color/glyph path (TTY + NO_COLOR gated) since its stdout is human-facing, not eval-consumed | The shell-init eval-safety exception is specific to eval'd output; doctor is a human diagnostic, so color is appropriate and consistent with update/install | S:75 R:85 A:85 D:80 |
| 10 | Certain | Missing shell-integration wiring is WARN (exit stays 0), not FAIL — the binary still works when invoked directly | Clarified — user confirmed WARN over FAIL. Wiring is a convenience, not function; keeps doctor green-for-CI on installed-but-unwired tools | S:90 R:75 A:85 D:90 |
| 11 | Certain | Unresolvable/unsupported `$SHELL` degrades the wiring check to WARN with an explanatory suggestion; binary checks still run, exit unaffected | Clarified — user confirmed WARN over hard-error/exit-2. Keeps doctor usable in CI (where $SHELL is often /bin/sh) | S:90 R:75 A:85 D:90 |
| 12 | Confident | Single subprocess per tool (`<tool> --version`) covers BOTH check 1 (PATH) and check 2 (version), matching `toolVersion` | `proc.ErrNotFound` vs other-error already distinguishes missing-vs-broken in one call; no second probe needed | S:75 R:85 A:90 D:80 |
| 13 | Confident | `--json` emits a JSON array (one object per tool, roster order) via marshal of a typed struct; no color; same checks + same exit contract as text | Clarified — user chose `--json` in v1. Typed-struct marshal is the idiomatic, drift-safe rendering; no-color is required for machine consumers | S:80 R:80 A:85 D:80 |

13 assumptions (8 certain, 5 confident, 0 tentative, 0 unresolved).
