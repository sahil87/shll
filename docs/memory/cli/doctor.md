# cli/doctor

`shll doctor` — verifies that every roster tool is installed, runnable, and (where applicable) wired into the shell. One status line per tool with an `OK` / `WARN` / `FAIL` marker; each non-OK line carries an actionable suggestion. Exits non-zero if **any** tool is FAIL, so it is scriptable in CI. Strictly read-only — it never installs, upgrades, or edits the rc file.

Source: `src/cmd/shll/doctor.go`. Reuses the version probe primitives from `src/cmd/shll/version.go` (`proc.Run` + `versionTimeout` + `normalizeVersion`), the wiring detector from `src/cmd/shll/shell_setup.go` (`resolveShell`/`resolveRcFile`/`locateBlock`/`blockMatch.hasEval` — read-only), `ui.go`'s `colorEnabled` + ANSI constants for optional TTY color, and the `Roster` + `errSilent` from `src/cmd/shll/tools.go` / `main.go`. No new mechanism (Constitution III).

## Output shape

Text (default), one tabwriter-aligned line per tool in roster order, with a problem-count tail when any tool is non-OK:

```
$ shll doctor
wt       OK    v1.4.0   wired
idea     OK    v0.3.1
tu       WARN  v2.0.0   not wired — run 'shll shell-setup' then 'exec $SHELL'
rk       OK    v0.9.2
hop      FAIL           run 'brew install sahil87/tap/hop'
fab-kit  FAIL           installed but 'fab-kit --version' failed — try 'brew reinstall sahil87/tap/fab-kit'

3 of 6 tools have problems. Run the suggested commands above, then re-run shll doctor.
```

- Roster order is `wt, idea, tu, rk, hop, fab-kit` (leaves-first, change auvj) — `doctor` walks `Roster` directly, so its line/object order is the roster order. See [cli/commands](commands.md#design-decision-leaves-first-roster-order-change-auvj).
- Columns: `<name>  <MARKER>  <version>  <detail>`, aligned via `text/tabwriter` (`tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)` — same parameters as `version.go`).
- The `<detail>` column is the suggestion on non-OK lines; on an OK wired shell-init tool it is the literal `wired`; on an OK non-shell-init tool (or an OK shell-init tool with no detail) it is empty.
- Color: the **OK** marker MAY be colored green on a real TTY (`colorEnabled(stdout)` — TTY + `NO_COLOR` gated, via `markerGlyph`). WARN/FAIL are left plain in both modes (no green-equivalent affordance in `ui.go`'s palette, and the wording carries the signal). `doctor`'s stdout is human-facing, not eval-consumed, so the `shell-init` eval-safety color exception does NOT apply here — color is appropriate, mirroring `update`/`install`.
- The problem-count tail (`%d of %d tools have problems. ...`) is printed only when at least one tool is non-OK (WARN counts as a "problem" for the tail count, even though WARN does not affect the exit code).

`--json` emits a machine-readable array instead (see [`--json` output mode](#--json-output-mode) below).

## The three per-tool checks

For each `Tool` in `Roster`, `doctor` runs up to three checks:

1. **Binary on PATH** — derived from the version probe: `proc.ErrNotFound` from `<tool> --version` means the binary is absent. Install-mechanism agnostic (brew, from-source, etc.), matching `version`/`shell-init`.
2. **Reports a version** — the binary on PATH must *also* successfully report a non-empty normalized version. A binary that exists but whose `--version` errors, times out, or normalizes to `""` is the "half-installed / stale brew link" case. Checks 1 and 2 are a **single** `<tool> --version` subprocess (one probe per tool, matching `toolVersion`).
3. **Shell integration wired** — runs **only** for tools where `len(tool.ShellInit) > 0`. The "wired" fact is whether shll's *own* composed eval block (`# >>> shll >>>` or the legacy `# >>> shll shell-init >>>` sentinel, with the `eval "$(shll shell-init <shell>)"` line) is present in the resolved rc file.

**Which tools get the wiring check (derived from `Roster`, not the backlog prose).** "Ships shell-init" is `len(tool.ShellInit) > 0`, evaluated against the live `Roster`. The shell-init integrators are exactly **`wt`, `tu`, `hop`**. `idea`, `rk`, and `fab-kit` carry an empty `ShellInit` slice and get **checks 1+2 only** (`shell_init:false`, no wiring check). This **corrects the backlog prose**, which listed shell-init as "relevant for hop/wt/tu/**idea**" — `idea` ships no shell-init, so it is NOT wiring-checked. Per Constitution III (Tool Roster Source of Truth), `doctor` derives this from `Roster` so it stays correct as the roster evolves. ("run-kit" in the backlog prose is the roster tool `rk`.)

The wiring fact is a **single rc-file fact** shared by every shell-init tool (shll's composed block covers them all), so `resolveWiringFact(env)` resolves it **once** up front and attributes it to each shell-init-shipping tool's line.

## Marker derivation (worst-applicable-check wins: FAIL > WARN > OK)

`evaluateTool(ctx, tool, fact)` composes the checks into a `doctorResult` whose `Status` is the worst applicable check:

| Condition | Marker | Status set | JSON fields |
|-----------|--------|-----------|-------------|
| Binary missing (`versionMissing`) | **FAIL** | first | `on_path:false`, `version_ok:false`, `version:""` |
| Binary present but version unreportable (`versionUnreportable`) | **FAIL** | first | `on_path:true`, `version_ok:false`, `version:""` |
| Checks 1+2 pass; shell-init tool whose `$SHELL` is unresolvable | **WARN** | after binary | `on_path:true`, `version_ok:true`, `wired:false` |
| Checks 1+2 pass; shell-init tool whose rc file has a **corrupted** shll block (open sentinel, no close) | **WARN** | after binary | `on_path:true`, `version_ok:true`, `wired:false` |
| Checks 1+2 pass; shell-init tool whose wiring is absent | **WARN** | after binary | `on_path:true`, `version_ok:true`, `wired:false` |
| Checks 1+2 pass; non-shell-init tool (`idea`/`rk`/`fab-kit`) | **OK** | — | `shell_init:false`, `wired:false` |
| All applicable checks pass (incl. wired shell-init tool) | **OK** | — | `wired:true` for shell-init tools |

A binary FAIL **dominates** the wiring check — `evaluateTool` returns immediately on `versionMissing`/`versionUnreportable` before any wiring is considered, so a shell-init tool that is also missing on PATH is FAIL, not WARN (`TestDoctor_MissingDominatesWiring`).

### Marker constants (exact, `doctor.go`)

```go
markerOK   = "OK"
markerWarn = "WARN"
markerFail = "FAIL"
```

Plain ASCII so they render identically on a non-TTY and inside `--json`. Named constants per `code-quality.md` (no magic strings).

### Suggestion constants (exact wording, `doctor.go`)

The actionable hint on each non-OK line (and in the JSON `suggestion` field) is one of these named format strings — the exact wording is part of the user contract, so it lives in one place:

```go
suggestMissingFmt           = "run 'brew install %s'"                                                                                        // %s = tool.Formula (e.g. sahil87/tap/hop)
suggestUnreportableFmt      = "installed but '%s --version' failed — try 'brew reinstall %s'"                                                  // (tool.Name, tool.Formula)
suggestNotWired             = "not wired — run 'shll shell-setup' then 'exec $SHELL'"                                                          // fixed text
suggestShellUnresolvableFmt = "cannot verify shell wiring — $SHELL is %q; pass a supported shell environment or run 'shll shell-setup zsh'"   // %q = raw $SHELL value
suggestCorruptBlock         = "shll block in your rc file is corrupted (unclosed sentinel) — fix or remove it manually, then run 'shll shell-setup'" // fixed text
```

`suggestCorruptBlock` is deliberately distinct from `suggestNotWired`: when `locateBlock` reports `partial` (an opening `# >>> shll >>>` sentinel with no matching close), `shll shell-setup` would **refuse** to modify the file (exit 2), so telling the user to "run `shll shell-setup`" plainly would send them into a dead end. The corrupt-block hint points at manual cleanup first.

OK tools carry no suggestion (empty string).

## The version probe — `probeVersion` (why a local helper)

`probeVersion(ctx, tool) (string, versionState)` (`doctor.go`) runs a single `<tool> --version` bounded by `versionTimeout` (2s) through `internal/proc` (Constitution I), and classifies the outcome into a three-way `versionState`:

| State | Trigger | Drives |
|-------|---------|--------|
| `versionMissing` | `proc.Run` returns `proc.ErrNotFound` | FAIL |
| `versionUnreportable` | any other `proc.Run` error/timeout, OR `normalizeVersion(out) == ""` | FAIL |
| `versionOK` | `proc.Run` succeeds and `normalizeVersion(out)` is non-empty | OK (captures the version string) |

It reuses the **same primitives** as `version.go`'s `toolVersion` — `proc.Run` + `versionTimeout` + `normalizeVersion` — so the version-reporting behavior stays single-sourced and the two cannot drift. **Why a separate helper instead of calling `toolVersion` directly**: `toolVersion` collapses both the missing case AND the unreportable case into the single `notInstalledLabel` (`"not installed"`) label, because `version` doesn't need to tell them apart. `doctor` *does* need them apart — "not installed" → install suggestion, "stale brew link" → reinstall suggestion — so `probeVersion` keeps the three states distinct while leaving `toolVersion` untouched (its callers don't need the distinction). See [cli/version](version.md#per-tool-timeout). (Design Decision, change d0ct.)

## The wiring fact — `resolveWiringFact` (read-only reuse)

`resolveWiringFact(env func(string) string) wiringFact` (`doctor.go`) computes the single shared rc-file fact:

1. `resolveShell([]string{}, env)` — infer the shell from `$SHELL` (no positional). On error → `wiringFact{shellResolved:false, rawShell:env("SHELL")}` (the unresolvable-`$SHELL` case).
2. `resolveRcFile(shell, env)` — derive the rc path.
3. `os.ReadFile(rcPath)` — **read-only**. A missing/unreadable rc file → `wiringFact{shellResolved:true, wired:false}` (the shell resolved fine; wiring simply isn't there yet — a plain "not wired", not the unresolvable-shell case).
4. `locateBlock(content)` → `(m, newOK, legacyM, legacyOK, partial)`. If `partial` (an open sentinel with no matching close) → `wiringFact{shellResolved:true, corrupt:true}` (the corrupted-block case — `shell-setup` would refuse to repair it, so the plain not-wired hint would mislead). Otherwise `wired := (newOK && m.hasEval) || (legacyOK && legacyM.hasEval)` — true when shll's eval block is present under either the new or the legacy sentinel.

```go
type wiringFact struct {
    shellResolved bool   // false when $SHELL is unset/unsupported
    wired         bool
    corrupt       bool   // true when locateBlock reports partial (unclosed sentinel)
    rawShell      string // the unresolved $SHELL value, for the suggestion
}
```

`doctor` reuses `resolveShell`/`resolveRcFile`/`locateBlock`/`blockMatch.hasEval` **strictly read-only** — it calls `os.ReadFile` only and NEVER any of `shell_setup.go`'s write paths (`appendBlock`/`rewriteBlocks`/`buildBlockBody`). See [cli/shell-setup](shell-setup.md#block-location-and-parsing).

## `--json` output mode

`shll doctor --json` emits a machine-readable JSON array (one object per roster tool, roster order) instead of the aligned text table, so CI can parse structured per-tool results. `--json` is a cobra bool flag on the `doctor` command — a **flag on the command, not a second subcommand** (Constitution VII's "could this be a flag?" test is satisfied).

The array is a marshal of the typed `doctorResult` struct (no hand-built JSON), so text and JSON derive from one source and cannot drift:

```go
type doctorResult struct {
    Tool       string `json:"tool"`
    Status     string `json:"status"`
    Version    string `json:"version"`
    OnPath     bool   `json:"on_path"`
    VersionOK  bool   `json:"version_ok"`
    ShellInit  bool   `json:"shell_init"`
    Wired      bool   `json:"wired"`
    Suggestion string `json:"suggestion"`
}
```

```json
[
  {"tool": "wt",  "status": "OK",   "version": "v1.4.0", "on_path": true,  "version_ok": true,  "shell_init": true,  "wired": true,  "suggestion": ""},
  {"tool": "tu",  "status": "WARN", "version": "v2.0.0", "on_path": true,  "version_ok": true,  "shell_init": true,  "wired": false, "suggestion": "not wired — run 'shll shell-setup' then 'exec $SHELL'"},
  {"tool": "hop", "status": "FAIL", "version": "",       "on_path": false, "version_ok": false, "shell_init": true,  "wired": false, "suggestion": "run 'brew install sahil87/tap/hop'"}
]
```

- `shell_init` is `true` exactly when `len(tool.ShellInit) > 0` (so `idea`/`rk`/`fab-kit` are `false`, and `wired` is `false`/not meaningful for them).
- Rendered via `json.NewEncoder(stdout)` with `SetIndent("", "  ")` — **indented (two-space) output with a trailing newline**. (The intake speculated a compact array; the implementation chose indented for readability — `json.Encoder` always appends the trailing newline.)
- **No ANSI color regardless of TTY** — machine consumers must get clean JSON. The `--json` path never touches `colorEnabled`.
- `--json` is gated by the **same checks** and the **same any-FAIL→exit-1 contract** as text; only the rendering differs. Diagnostics that text prints inline are carried in the `suggestion` field; nothing extraneous is written to stdout.

## Exit-code contract (scriptable in CI)

`runDoctor(ctx, jsonOut, env, stdout, stderr)` is the implementation seam (extracted so tests drive it with `bytes.Buffer` writers, a fake `proc.Runner`, and a map-backed env — mirroring `resolveShell`/`resolveRcFile`; production passes `os.Getenv`). It walks `Roster`, evaluates each tool, renders, and returns:

| Exit code | Condition |
|-----------|-----------|
| **0** | No tool is FAIL (every tool is OK or WARN). WARN alone NEVER affects the exit — returns nil. |
| **1** | At least one tool is FAIL → returns `errSilent` (→ `translateExit` maps to 1). The per-tool diagnostics are already on stdout (text) or in the JSON `suggestion` fields, so `errSilent` suppresses a redundant stderr line. |

A render error (`--json` marshal failure) writes `shll doctor: <err>` to stderr and also returns `errSilent` (exit 1). The exit logic is **identical for text and `--json`** — `--json` changes only the rendering, never the check logic or the exit contract. See [cli/commands](commands.md#exit-code-translation) for `errSilent`.

## Edge cases

- **Unwired shell-init tool** (binary OK, but shll's eval block absent from the rc file) → **WARN**, exit unaffected. The tool *works* when invoked directly; wiring is a convenience, not function — so it stays green-for-CI. (`TestDoctor_UnwiredShellInitWarnsExitZero`.)
- **Unresolvable / unsupported `$SHELL`** (e.g. CI with `$SHELL=/bin/sh`, or unset) → the wiring check cannot resolve an rc path, so shell-init tools degrade to **WARN** with the `suggestShellUnresolvableFmt` explanation; the binary checks (1+2) still run normally and the exit code is unaffected. Non-shell-init tools are untouched (still OK). (`TestDoctor_UnresolvableShellDegradesToWarn`.)
- **Missing / unreadable rc file** (but `$SHELL` resolved) → treated as "not wired" (a plain WARN with `suggestNotWired`), distinct from the unresolvable-shell case — the shell resolved, the wiring just isn't there yet.
- **Corrupted shll block** (rc file has an opening `# >>> shll >>>` sentinel with no matching close — `locateBlock` reports `partial`) → shell-init tools → **WARN** with `suggestCorruptBlock` (manual-cleanup hint), exit unaffected. Distinct from plain "not wired" because `shell-setup` refuses to modify a corrupted block (exit 2), so the plain "run `shll shell-setup`" hint would dead-end. (`TestDoctor_CorruptBlockWarnsWithDistinctSuggestion`.)
- **Binary FAIL on a shell-init tool** → FAIL dominates; no wiring WARN is shown.

## Test seam

`doctor_test.go` (test-alongside, per `code-quality.md`) drives `runDoctor` with `bytes.Buffer` writers, a fake `proc.Runner` (`doctorFake(states map[string]doctorVersionState)` — per-tool `--version` behavior, defaulting absent tools to `dvOK`), and a map-backed env (`rcEnv(rcDir)` resolves zsh and points `ZDOTDIR`/`HOME` at a `t.TempDir()` rc file, so the wiring check NEVER touches the real `~/.zshrc`). `writeWiredRC`/`writeUnwiredRC`/`writeCorruptRC` build the rc fixtures (wired uses `tNewBlockZsh`; corrupt writes an `openSentinel` with no matching close).

- `TestDoctor_AllOKWired` — all tools installed + a wired rc → every line OK, no problem tail; shell-init tools show `wired`, others do not.
- `TestDoctor_MissingBinaryFails` — `hop` missing → FAIL line, install suggestion (`brew install sahil87/tap/hop`), problem tail, exit `errSilent`.
- `TestDoctor_UnreportableVersionFails` — `fab-kit` both `dvUnreportable` (proc error) and `dvEmpty` (empty normalize) → FAIL, reinstall suggestion (`fab-kit --version' failed`, `brew reinstall sahil87/tap/fab-kit`), exit `errSilent`.
- `TestDoctor_UnwiredShellInitWarnsExitZero` — installed but unwired rc → `wt`/`tu`/`hop` WARN, `idea`/`rk`/`fab-kit` OK (no wiring check), not-wired suggestion, exit 0.
- `TestDoctor_CorruptBlockWarnsWithDistinctSuggestion` — rc file with an unclosed shll sentinel (`writeCorruptRC`) → `wt`/`tu`/`hop` WARN with `suggestCorruptBlock` (NOT the plain `suggestNotWired`), exit 0.
- `TestDoctor_MissingDominatesWiring` — `wt` missing AND unwired → FAIL (binary failure dominates the would-be wiring WARN).
- `TestDoctor_UnresolvableShellDegradesToWarn` — `$SHELL=/bin/sh` → `wt`/`tu`/`hop` WARN with the `$SHELL is` suggestion, binary checks still run, `idea` stays OK, exit 0.
- `TestDoctor_JSONShapeAndExit` — `--json` with `hop` missing + `tu` unwired → valid JSON, no ANSI, trailing newline, `len == len(Roster)`, roster order preserved, per-tool field values per marker (`hop` FAIL/missing, `tu` WARN/onpath/version_ok/unwired with `version:"v1.2.3"`, `idea` `shell_init:false`/OK), `hop` is FAIL → exit `errSilent` (same as text).
- `TestDoctor_JSONAllOKExitZero` — `--json` all-OK + wired rc → every status OK, wired shell-init tools report `wired:true`, exit 0.
- `TestDoctor_RegisteredOnRoot` — `doctor` is registered on `newRootCmd()` and `rootLong` documents `shll doctor`.

`lineFor`/`lineHas` are line-scanning helpers; `resultByTool` indexes a decoded JSON result slice by tool name.

## Cross-references

- Subprocess wrapper conventions and `proc.ErrNotFound` semantics: [internal/proc](../internal/proc.md). All probe subprocess work routes through `internal/proc` (Constitution I).
- Shared version probe: [cli/version](version.md) — `doctor`'s `probeVersion` reuses `version.go`'s `proc.Run`/`versionTimeout`/`normalizeVersion`, so the two share the version-probe contract and cannot drift.
- Shared wiring detector: [cli/shell-setup](shell-setup.md#block-location-and-parsing) — `doctor` reuses `resolveShell`/`resolveRcFile`/`locateBlock`/`blockMatch.hasEval` strictly read-only (never the write paths).
- Registration, exit-code sentinels, and the `Roster`: [cli/commands](commands.md) — `doctor` is the sixth user-facing subcommand (the hidden `help-dump` is not counted).
- Constitution I (Security First) → the version probe routes through `internal/proc`; rc-file access is read-only `os.ReadFile`.
- Constitution III (Wrap, Don't Reinvent + Tool Roster Source of Truth) → `doctor` reuses existing probe/wiring primitives rather than reimplementing them, and derives "ships shell-init" from the live `Roster` (`len(tool.ShellInit) > 0`), not the backlog prose.
- Constitution V (Graceful Degradation) → an uninstalled or unwired tool degrades to FAIL/WARN with an actionable suggestion rather than crashing; an unresolvable `$SHELL` degrades wiring to WARN while binary checks proceed.
- Constitution VII (Minimal Surface Area) → `doctor` is a new top-level subcommand (sixth), justified as a read-only cross-tool diagnostic distinct from `version` (reporting, always exit 0), `install`/`update` (mutating), and per-tool CLIs (no single tool can see the *composed* shll block). `--json` is a flag on `doctor`, not a separate subcommand. Justification recorded in the change intake (260609-d0ct) and [cli/commands](commands.md#constitution-vii-justification-per-subcommand).
