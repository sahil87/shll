---
type: memory
description: "`shll doctor` — read-only per-tool verification (binary on PATH, reports a version, formula trusted, shell-init wired; delegated non-brew tools get a probe-only check from their Probe spec — no trust/wiring), worst-check-wins `OK`/`WARN`/`FAIL` markers, `--json` mode, any-FAIL→exit-1. Reuses `version`'s probe (incl. its legacy-name fallback and the shared `parseProbeStatusLine`), the `setup shell` wiring detector, and `brew.go`'s trust list."
---
# cli/doctor

`shll doctor` — verifies that every roster tool is installed, runnable, and (where applicable) wired into the shell. One status line per tool with an `OK` / `WARN` / `FAIL` marker; each non-OK line carries an actionable suggestion. Exits non-zero if **any** tool is FAIL, so it is scriptable in CI. Strictly read-only — it never installs, upgrades, or edits the rc file.

Source: `src/cmd/shll/doctor.go`. Reuses the version probe from `src/cmd/shll/version.go` (`probeVersion` delegates to `probeToolVersion` — inheriting the bounded `versionTimeout` invocation, `normalizeVersion`, AND the rk→run-kit legacy-name fallback), the shared delegated probe-line parser `parseProbeStatusLine` (`src/cmd/shll/version.go:157`) for the formula-less rk-desktop entry (t26g), the wiring detector from `src/cmd/shll/shell_setup.go` (`resolveShell`/`resolveRcFile`/`locateBlock`/`blockMatch.hasEval` — read-only), the trust-state primitives from `src/cmd/shll/brew.go` (`brewTrustAvailable` + `brewTrustList`, change 0854 — read-only), `ui.go`'s `colorEnabled` + ANSI constants for optional TTY color, and the `Roster` + `errSilent` from `src/cmd/shll/tools.go` / `main.go`. No new mechanism (Constitution III).

## Output shape

Text (default), a shll-first row then one tabwriter-aligned line per roster tool, with a problem-count tail when any tool is non-OK:

```
$ shll doctor
shll       OK    v0.1.0
run-kit    OK    v3.0.0
rk-desktop FAIL           run 'rk desktop install'
fab-kit    FAIL           installed but 'fab-kit --version' failed — try 'brew reinstall sahil87/tap/fab-kit'
wt         OK    v1.4.0   wired
idea       OK    v0.3.1
tu         WARN  v2.0.0   not wired — run 'shll setup shell' then 'exec $SHELL'
hop        FAIL           run 'brew install sahil87/tap/hop'

4 of 8 tools have problems. Run the suggested commands above, then re-run shll doctor.
```

- Ordering is **shll-first, then roster-declared order** (bb7r): the always-OK `shll` row is prepended (`runDoctor`), followed by the `Roster` in its declared `run-kit, rk-desktop, fab-kit, wt, idea, tu, hop` order — importance-descending with dependency adjacency (rk-desktop sits directly after run-kit, whose `rk desktop …` subcommands it delegates to), enforced by `TestRosterOrder` (t26g). The unified shll-first ordering is universal across the inspect/manage surface — see [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor) and [cli/commands §hardcoded tool roster](/cli/commands.md#hardcoded-tool-roster). The `shll` row reads OK with a version and no detail; it never carries a wiring detail (`shell_init:false`).
- **The problem-count denominator is every rendered row, shll included.** In the example three roster tools FAIL and one is WARN, so the tail reads `4 of 8` — the denominator is `len(results)` (`len(Roster)+1`), not `len(Roster)`. The shll row is itself checkable (it can WARN), so excluding it from the denominator could mis-report a count larger than the denominator. See [The prepended shll-first row](#the-prepended-shll-first-row) below.
- Columns: `<name>  <MARKER>  <version>  <detail>`, aligned via `text/tabwriter` (`tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)` — same parameters as `version.go`).
- The `<detail>` column is the suggestion on non-OK lines; on an OK wired shell-init tool it is the literal `wired`; on an OK non-shell-init tool (or an OK shell-init tool with no detail) it is empty.
- Color: the **OK** marker MAY be colored green on a real TTY (`colorEnabled(stdout)` — TTY + `NO_COLOR` gated, via `markerGlyph`). WARN/FAIL are left plain in both modes (no green-equivalent affordance in `ui.go`'s palette, and the wording carries the signal). `doctor`'s stdout is human-facing, not eval-consumed, so the `shell-init` eval-safety color exception does NOT apply here — color is appropriate, mirroring `update`/`install`.
- The problem-count tail (`%d of %d tools have problems. ...`) is printed only when at least one tool is non-OK (WARN counts as a "problem" for the tail count, even though WARN does not affect the exit code).

`--json` emits a machine-readable array instead (see [`--json` output mode](#--json-output-mode) below).

## The per-tool checks

For each brew-managed `Tool` in `Roster`, `doctor` runs up to four checks (a delegated, formula-less tool takes the probe-only path — see [Delegated (non-brew) tools](#delegated-non-brew-tools--probe-only-evaluation) below):

1. **Binary on PATH** — derived from the version probe: `proc.ErrNotFound` from `<tool> --version` means the binary is absent. Install-mechanism agnostic (brew, from-source, etc.), matching `version`/`shell-init`.
2. **Reports a version** — the binary on PATH must *also* successfully report a non-empty normalized version. A binary that exists but whose `--version` errors, times out, or normalizes to `""` is the "half-installed / stale brew link" case. Checks 1 and 2 are a **single** `<tool> --version` subprocess (one probe per tool, matching `toolVersion`).
3. **Formula trusted** (0854) — for an installed tool (checks 1+2 passed), whether its Homebrew formula is trusted. An installed-but-untrusted tool still *runs*, but its next `brew upgrade` (via `shll update` or plain brew) is refused on Homebrew 6.0+, so this is a **WARN** (not FAIL). Applies to **all** installed brew-managed roster tools (not just shell-init ones), since every formula needs trust to upgrade; delegated tools carry no formula and are excluded (t26g). The trust fact is a single brew-wide query, resolved once per run — see [The trust sub-check](#the-trust-sub-check).
4. **Shell integration wired** — runs **only** for tools where `len(tool.ShellInit) > 0`. The "wired" fact is whether shll's *own* composed eval block (`# >>> shll >>>` or the legacy `# >>> shll shell-init >>>` sentinel, with the `eval "$(shll shell-init <shell>)"` line) is present in the resolved rc file.

**Which tools get the wiring check (derived from `Roster`, not the backlog prose).** "Ships shell-init" is `len(tool.ShellInit) > 0`, evaluated against the live `Roster`. The shell-init integrators are exactly **`wt`, `tu`, `hop`**. `idea`, `run-kit`, and `fab-kit` carry an empty `ShellInit` slice and get **checks 1+2 only** (`shell_init:false`, no wiring check). `idea` ships no shell-init, so it is NOT wiring-checked. `rk-desktop` ships no shell-init either — and as a delegated (formula-less) tool it never reaches `evaluateTool` at all (t26g). Per Constitution III (Tool Roster Source of Truth), `doctor` derives this from `Roster` so it stays correct as the roster evolves.

The wiring fact is a **single rc-file fact** shared by every shell-init tool (shll's composed block covers them all), so `resolveWiringFact(env)` resolves it **once** up front and attributes it to each shell-init-shipping tool's line.

### Delegated (non-brew) tools — probe-only evaluation

`runDoctor` branches on `tool.brewManaged()` (`Formula != ""`) in the roster loop (`src/cmd/shll/doctor.go:181`): brew-managed tools take `evaluateTool`; a delegated tool (empty `Formula`, delegated `Install` argv + `Probe` spec — today only `rk-desktop`) takes `evaluateDelegatedTool` (`src/cmd/shll/doctor.go:424`), where the `Probe` spec is the **only** check:

- The probe argv (`rk desktop status`) runs via `proc.RunCaptured` under the same `versionTimeout` bound as the `--version` probe, and `parseProbeStatusLine` (`src/cmd/shll/version.go:157` — the shared probe-line parser, see [cli/version §the shared install probe](/cli/version.md#the-shared-install-probe)) scans stdout for the `Installed:` line.
- **Installed** (`Installed: v<X>` — any value other than the spec's `AbsentValue`) → **OK**: `on_path:true`, `version_ok:true`, `version` = `normalizeVersion(value)`.
- **Cleanly absent** (the value equals `AbsentValue` — `Installed: not installed`) → **FAIL** with `suggestDelegatedMissingFmt`, naming the delegated install argv (`run 'rk desktop install'`) — never a `brew install` hint (there is no formula).
- **Probe unanswerable** (the `rk` prerequisite binary missing from PATH — `proc.ErrNotFound`, a transport error, or a non-zero exit incl. run-kit's unsupported-platform refusal) → **FAIL** with the same delegated install hint (the delegated install command prints the fuller explanation).
- **Probe unreportable** (no `Installed:` line, or a value that is neither a version nor the absent value) → **FAIL** with `suggestDelegatedUnreportableFmt`.

Delegated tools are **excluded from the formula-trust sub-check and the wiring check** — no formula to trust, no shell-init to wire; the row reports `shell_init:false`, `wired:false` like `idea`/`run-kit`/`fab-kit`. The exit-code contract is unchanged: a delegated FAIL sets `anyFail` in the caller exactly like a brew tool's. (t26g)

## The prepended shll-first row

`doctor` prepends an always-OK `shll` row before walking the roster — `results = append(results, shllDoctorResult())` then the per-tool loop (`runDoctor`, `src/cmd/shll/doctor.go:133`). The row is built by `shllDoctorResult()` (`src/cmd/shll/doctor.go:268`), **directly — NOT via `evaluateTool`**, and is deliberately different from a roster tool in three load-bearing ways:

1. **No self-subprocess.** `evaluateTool` always calls `probeVersion`, which spawns `<tool> --version`. `doctor` must not spawn `shll --version` on itself: shll is the running process, so its binary is definitionally on PATH and its version is read from the package `version` var via `shllSelfVersion()` (`src/cmd/shll/tools.go:143` → `normalizeVersion(version)`) — the same source as `shll version`'s own first row, so the two surfaces agree. Building the `doctorResult` directly avoids the circular, wasteful self-spawn.
2. **Checks 1+2 only — no wiring check.** shll ships no shell-init of its own (shell-init is the [documented eval-safety exception](/cli/shell-init.md#the-deliberate-exception--do-not-unify-onto-the--header)), so it is treated like `idea`/`run-kit`/`fab-kit`/`rk-desktop`: `ShellInit:false`, `Wired:false`, no wiring check applies.
3. **Always OK, never touches `anyFail`.** The row's `Status` is hardcoded `markerOK` (binary present + version present), and the prepend in `runDoctor` does **not** run the `if res.Status == markerFail { anyFail = true }` branch that the roster loop runs. So the always-OK shll row **cannot perturb the scriptable any-FAIL→exit-1 contract**: a clean roster still exits 0, a roster with a FAIL still exits 1.

```go
func shllDoctorResult() doctorResult {
    return doctorResult{
        Tool:      shllSelf.Name,      // "shll" — the shared shllSelf descriptor
        Status:    markerOK,
        Version:   shllSelfVersion(),  // package version var, NOT a self-subprocess
        OnPath:    true,
        VersionOK: true,
        ShellInit: false,
        Wired:     false,
    }
}
```

The `Tool` name comes from the shared `shllSelf` descriptor (`src/cmd/shll/tools.go:131`) — the single source of truth for "shll as a displayable entry", reused by `list`/`install`/`doctor` (see [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor)). Both the text and `--json` renderers consume the shll row through the **same `results` walk** as every roster tool — no special-casing in the renderers.

### The problem-count denominator is `len(results)` — every rendered row, shll included

`renderDoctorText` (`src/cmd/shll/doctor.go:517`) counts `problems` by walking `results`, and the summary-tail denominator is **`len(results)`** (`len(Roster)+1`) — every rendered row, including the shll row. The shll row is itself checkable (it can register a WARN), so a `len(Roster)` denominator could mis-report a problems count larger than the denominator. (Guarded by `TestDoctor_ProblemTailDenominatorCountsAllRows`, which asserts the tail reads `1 of len(Roster)+1`.)

## Marker derivation (worst-applicable-check wins: FAIL > WARN > OK)

`evaluateTool(ctx, tool, fact, trust)` composes the checks into a `doctorResult` whose `Status` is the worst applicable check. `evaluateTool` first runs the `--version` probe and sets the machine-readable `on_path`/`version_ok`/`version` fields **honestly from the probe outcome** (see below). It then applies, in order: binary FAIL → trust check → wiring check. Delegated tools skip `evaluateTool` entirely — `evaluateDelegatedTool` composes their row probe-only (see [Delegated (non-brew) tools](#delegated-non-brew-tools--probe-only-evaluation)); their marker space is FAIL/OK (no WARN tier: no trust, no wiring) (t26g).

| Condition | Marker | Status set | JSON fields |
|-----------|--------|-----------|-------------|
| Binary missing (`versionMissing`) | **FAIL** | first | `on_path:false`, `version_ok:false`, `version:""` |
| Binary present but version unreportable (`versionUnreportable`) | **FAIL** | first | `on_path:true`, `version_ok:false`, `version:""` |
| Delegated tool absent (probe reports its `AbsentValue`) or probe unanswerable (`rk` missing, transport error, platform refusal) | **FAIL** | first (`evaluateDelegatedTool`) | `on_path:false`, `version_ok:false`, `version:""` (`suggestDelegatedMissingFmt`) |
| Delegated tool probe unreportable (no `Installed:` line / unrecognized value) | **FAIL** | first (`evaluateDelegatedTool`) | `on_path:false`, `version_ok:false`, `version:""` (`suggestDelegatedUnreportableFmt`) |
| Checks 1+2 pass; trust available and the formula is **not** trusted (0854; brew-managed only — delegated tools have no formula) | **WARN** | after binary, before wiring | `on_path:true`, `version_ok:true` (`suggestNotTrustedFmt`) |
| Checks 1+2 pass; shell-init tool whose `$SHELL` is unresolvable | **WARN** | after trust | `on_path:true`, `version_ok:true`, `wired:false` |
| Checks 1+2 pass; shell-init tool whose rc file has a **corrupted** shll block (open sentinel, no close) | **WARN** | after trust | `on_path:true`, `version_ok:true`, `wired:false` |
| Checks 1+2 pass; shell-init tool whose wiring is absent | **WARN** | after trust | `on_path:true`, `version_ok:true`, `wired:false` |
| Checks 1+2 pass; non-shell-init tool (`idea`/`run-kit`/`fab-kit`), trusted (or trust unavailable) | **OK** | — | `shell_init:false`, `wired:false` |
| Delegated tool installed + version (`Installed: v<X>`) | **OK** | — | `on_path:true`, `version_ok:true`, `shell_init:false`, `wired:false` |
| All applicable checks pass (incl. wired shell-init tool) | **OK** | — | `wired:true` for shell-init tools |

A binary FAIL **dominates** the trust/wiring checks — `evaluateTool` returns on `versionMissing`/`versionUnreportable` before either (`TestDoctor_MissingDominatesWiring`, `TestDoctor_BinaryFailDominatesTrust`). The trust and wiring WARN tiers are co-equal (same exit), so ordering between them only decides *which* suggestion surfaces.

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
suggestMissingFmt               = "run 'brew install %s'"                                                                                        // %s = tool.Formula (e.g. sahil87/tap/hop); brew-managed tools only
suggestUnreportableFmt          = "installed but '%s --version' failed — try 'brew reinstall %s'"                                                  // (tool.Name, tool.Formula)
suggestDelegatedMissingFmt      = "run '%s'"                                                                                                      // %s = delegated install argv joined (e.g. rk desktop install) (t26g)
suggestDelegatedUnreportableFmt = "'%s' reported no installed version — try reinstalling it"                                                      // %s = probe argv joined (e.g. rk desktop status) (t26g)
suggestNotWired                 = "not wired — run 'shll setup shell' then 'exec $SHELL'"                                                          // fixed text
suggestShellUnresolvableFmt     = "cannot verify shell wiring — $SHELL is %q; pass a supported shell environment or run 'shll setup shell zsh'"   // %q = raw $SHELL value
suggestCorruptBlock             = "shll block in your rc file is corrupted (unclosed sentinel) — fix or remove it manually, then run 'shll setup shell'" // fixed text
suggestNotTrustedFmt            = "formula not trusted — run 'shll install' (or 'brew trust --formula %s'); future upgrades will fail without it"      // %s = tool.Formula (0854)
```

`suggestCorruptBlock` is deliberately distinct from `suggestNotWired`: when `locateBlock` reports `partial` (an opening `# >>> shll >>>` sentinel with no matching close), `shll setup shell` would **refuse** to modify the file (exit 2), so telling the user to "run `shll setup shell`" plainly would send them into a dead end. The corrupt-block hint points at manual cleanup first.

OK tools carry no suggestion (empty string).

## The version probe — `probeVersion` (why a local helper)

`probeVersion(ctx, tool) (string, versionState)` (`doctor.go`) classifies the `<tool> --version` outcome into a three-way `versionState`:

| State | Trigger | Drives |
|-------|---------|--------|
| `versionMissing` | `probeToolVersion` returns `proc.ErrNotFound` | FAIL |
| `versionUnreportable` | any other error/timeout, OR `normalizeVersion(out) == ""` | FAIL |
| `versionOK` | success and `normalizeVersion(out)` is non-empty | OK (captures the version string) |

It **calls `version.go`'s `probeToolVersion(ctx, tool)` directly** (not just the same primitives). This single-sources BOTH the bounded `versionTimeout` invocation AND the rk→run-kit **legacy-name fallback** (`probeToolVersion` retries `<tool.LegacyName> --version` on `proc.ErrNotFound` only), so a run-kit install whose binary is still `rk` on PATH reports `versionOK` here rather than `versionMissing` (`TestDoctor_MigratedRunKitLegacyBinaryOnPathNotFail`), while a present-but-broken `run-kit` stays unreportable and never defers to `rk`. **Why a separate helper instead of calling `toolVersion` directly**: `toolVersion` collapses both the missing case AND the unreportable case into the single `notInstalledLabel` (`"not installed"`) label, because `version` doesn't need to tell them apart. `doctor` *does* need them apart — "not installed" → install suggestion, "stale brew link" → reinstall suggestion — so `probeVersion` adds only the three-way classification on top of `probeToolVersion`, leaving `toolVersion` untouched (its callers don't need the distinction). See [cli/version §the legacy-name fallback](/cli/version.md#the-legacy-name-path-probe-fallback). (Design Decision, d0ct.) Only brew-managed tools flow through `probeVersion` — a delegated tool has no `--version` surface, so its row is composed probe-only by `evaluateDelegatedTool`, parsing the probe output with the shared `parseProbeStatusLine` instead (t26g).

## The wiring fact — `resolveWiringFact` (read-only reuse)

`resolveWiringFact(env func(string) string) wiringFact` (`doctor.go`) computes the single shared rc-file fact:

1. `resolveShell([]string{}, env)` — infer the shell from `$SHELL` (no positional). On error → `wiringFact{shellResolved:false, rawShell:env("SHELL")}` (the unresolvable-`$SHELL` case).
2. `resolveRcFile(shell, env)` — derive the rc path.
3. `os.ReadFile(rcPath)` — **read-only**. A missing/unreadable rc file → `wiringFact{shellResolved:true, wired:false}` (the shell resolved fine; wiring simply isn't there yet — a plain "not wired", not the unresolvable-shell case).
4. `locateBlock(content)` → `(m, newOK, legacyM, legacyOK, partial)`. If `partial` (an open sentinel with no matching close) → `wiringFact{shellResolved:true, corrupt:true}` (the corrupted-block case — `shll setup shell` would refuse to repair it, so the plain not-wired hint would mislead). Otherwise `wired := (newOK && m.hasEval) || (legacyOK && legacyM.hasEval)` — true when shll's eval block is present under either the new or the legacy sentinel.

```go
type wiringFact struct {
    shellResolved bool   // false when $SHELL is unset/unsupported
    wired         bool
    corrupt       bool   // true when locateBlock reports partial (unclosed sentinel)
    rawShell      string // the unresolved $SHELL value, for the suggestion
}
```

`doctor` reuses `resolveShell`/`resolveRcFile`/`locateBlock`/`blockMatch.hasEval` **strictly read-only** — it calls `os.ReadFile` only and NEVER any of `shell_setup.go`'s write paths (`appendBlock`/`rewriteBlocks`/`buildBlockBody`). See [cli/setup](/cli/setup.md#block-location-and-parsing).

> **Also reused by `shll install`'s post-install auto-run.** `resolveWiringFact` is not doctor-only: `shll install` calls it to gate its post-install shell-setup step (act only when `shellResolved && !corrupt && !wired`). The detector reuse is in-place (same `main` package, no move) and itself strictly read-only (`os.ReadFile` only) — but when the gate reports unwired, install's auto-run then drives the shell half's **write** path (`runShellSetupDefault`) to wire the rc file, falling back to the gated nudge on opt-out/failure. The `corrupt`/`shellResolved` edge fields carry the same meaning there (a corrupt block or unresolvable `$SHELL` quiet-skips the step, mirroring doctor's `suggestCorruptBlock`/`suggestShellUnresolvableFmt` distinction). See [cli/install §the post-install auto-run steps](/cli/install.md#the-post-install-auto-run-steps-and-the-next-steps-block).

## The trust sub-check

Resolved by `resolveTrustFact(ctx)`. Homebrew 6.0 made tap-trust a hard install/upgrade requirement, so an installed roster tool whose formula is *not* trusted will have its next `brew upgrade` refused. `doctor` surfaces this with a read-only per-installed-tool sub-check, mirroring the single-shared-fact pattern of the wiring check: the trust state is a **single brew-wide fact**, so `resolveTrustFact(ctx)` resolves it **once** up front (`runDoctor`, `doctor.go:140`) and `evaluateTool` checks each tool against it.

```go
type trustFact struct {
    available  bool            // false when brew is absent or too old to ship `brew trust`
    tapTrusted bool            // the whole sahil87 tap is trusted (trusts every formula under it)
    formulae   map[string]bool // individually-trusted fully-qualified formula names
}

func (tf trustFact) trusts(formula string) bool {
    return tf.tapTrusted || tf.formulae[formula]   // tap- OR formula-level trust counts
}
```

`resolveTrustFact(ctx)`:

1. **Gate on `brewTrustAvailable(ctx)`** — the same capability probe (`brew trust --help`) `shll install` uses. If brew is absent or too old to ship `brew trust` (pre-6.0, where trust isn't required anyway), return `trustFact{available:false}` → the sub-check is **skipped silently** (Constitution V — never WARN on a state doctor cannot determine).
2. **Query `brewTrustList(ctx)`** — runs `brew trust --json=v1` (via `proc.Run`), JSON-decoding `{taps, formulae}` (the verified Homebrew 6.0.4 shape is `{taps, formulae, casks, commands}`; doctor reads only the first two). On any failure (non-zero exit, garbled JSON) `brewTrustList` reports `ok=false` and `resolveTrustFact` degrades to `available:false` rather than guessing.
3. **Build the set** — `tapTrusted` is true when `tapName` (`"sahil87/tap"`) is in the `taps` array; `formulae` is the set of trusted fully-qualified formula names.

**A formula counts as trusted when its qualified name (`sahil87/tap/<formula>`) is in `formulae` OR `sahil87/tap` is in `taps`** — tap-level trust covers every formula under the tap. This is the read-only, wrap-don't-reinvent path (Constitution III): doctor **NEVER** reads `~/.homebrew/trust.json` directly — it asks brew via its public `--json=v1` contract, and decodes with `encoding/json` (never a regex over brew output — code-quality.md anti-pattern).

**Ordering and scope inside `evaluateTool`.** The trust check runs **after** the binary checks (which return FAIL first, so a binary FAIL dominates) and **before** the wiring check. It applies to **all** installed brew-managed roster tools, not just shell-init ones — every formula needs trust to upgrade, and delegated tools never reach `evaluateTool` (`runDoctor` branches on `tool.brewManaged()` first), so no formula-less trust lookup can occur (t26g). When `trust.available && !trust.trusts(tool.Formula)`, `evaluateTool` sets `markerWarn` with `suggestNotTrustedFmt` and returns. The trust WARN and the unwired WARN are **co-equal under worst-check-wins** (both WARN → identical exit), so when a tool is both untrusted and unwired, the trust warning is the one surfaced — an untrusted tool's next upgrade is refused outright (higher user impact than an unwired-but-functional tool). The decision is presentation only — the exit code is unaffected.

**The `shll` self row is unchanged** — it is built directly by `shllDoctorResult()` (no `evaluateTool`, no trust check); shll's own trust is the README bootstrap concern, not a roster-tool check. Not-installed tools already FAIL on the binary check, so trust is moot there.

**No `trusted` JSON field** — the untrusted state is reflected through the existing `Status: "WARN"` + `Suggestion` fields, so text and `--json` stay derived from the one `doctorResult` struct (adding a field would be a larger, less-reversible schema change). The read-only and any-FAIL→exit-1 / `--json` contracts hold — the trust sub-check's own subprocesses are `brew trust --help` (gate) and `brew trust --json=v1` (query), both read-only. (The legacy `rk --version` PATH fallback belongs to the [version probe](#the-version-probe--probeversion-why-a-local-helper), not the trust query; all remain strictly read-only, no brew writes.)

## `--json` output mode

`shll doctor --json` emits a machine-readable JSON array (a shll-first object, then one object per roster tool in roster order) instead of the aligned text table, so CI can parse structured per-tool results. `--json` is a cobra bool flag on the `doctor` command — a **flag on the command, not a second subcommand** (Constitution VII's "could this be a flag?" test is satisfied).

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
  {"tool": "shll", "status": "OK",   "version": "v0.1.0", "on_path": true,  "version_ok": true,  "shell_init": false, "wired": false, "suggestion": ""},
  {"tool": "wt",  "status": "OK",   "version": "v1.4.0", "on_path": true,  "version_ok": true,  "shell_init": true,  "wired": true,  "suggestion": ""},
  {"tool": "rk-desktop", "status": "OK", "version": "v1.2.3", "on_path": true, "version_ok": true, "shell_init": false, "wired": false, "suggestion": ""},
  {"tool": "tu",  "status": "WARN", "version": "v2.0.0", "on_path": true,  "version_ok": true,  "shell_init": true,  "wired": false, "suggestion": "not wired — run 'shll setup shell' then 'exec $SHELL'"},
  {"tool": "hop", "status": "FAIL", "version": "",       "on_path": false, "version_ok": false, "shell_init": true,  "wired": false, "suggestion": "run 'brew install sahil87/tap/hop'"}
]
```

- **The first object is the always-OK shll-first object** (bb7r): `tool:"shll"`, `status:"OK"`, version from the package var, `shell_init:false`, `wired:false`, empty suggestion — see [The prepended shll-first row](#the-prepended-shll-first-row). The array length is `len(Roster)+1` (`TestDoctor_JSONShapeAndExit` asserts `len(results) == len(Roster)+1` and `results[0].Tool == shllSelf.Name`). The shll object carries no distinct self-marker in the doctor schema — the doctor `doctorResult` struct is unchanged; the shll-first *position* identifies it (unlike `list --json`, which adds a `self` field — the two surfaces serve different schemas).
- `shell_init` is `true` exactly when `len(tool.ShellInit) > 0` (so `idea`/`run-kit`/`fab-kit`/`rk-desktop` are `false`, and `wired` is `false`/not meaningful for them; `shll` is also `false`). A delegated tool's object is probe-derived: `on_path`/`version_ok`/`version` come from the `Installed:` line of its Probe spec (`TestDoctor_RkDesktopJSONFields`).
- Rendered via `json.NewEncoder(stdout)` with `SetIndent("", "  ")` — **indented (two-space) output with a trailing newline**. (The intake speculated a compact array; the implementation chose indented for readability — `json.Encoder` always appends the trailing newline.)
- **No ANSI color regardless of TTY** — machine consumers must get clean JSON. The `--json` path never touches `colorEnabled`.
- `--json` is gated by the **same checks** and the **same any-FAIL→exit-1 contract** as text; only the rendering differs. Diagnostics that text prints inline are carried in the `suggestion` field; nothing extraneous is written to stdout.

## Exit-code contract (scriptable in CI)

`runDoctor(ctx, jsonOut, env, stdout, stderr)` is the implementation seam (extracted so tests drive it with `bytes.Buffer` writers, a fake `proc.Runner`, and a map-backed env — mirroring `resolveShell`/`resolveRcFile`; production passes `os.Getenv`). It walks `Roster`, evaluates each tool, renders, and returns:

| Exit code | Condition |
|-----------|-----------|
| **0** | No roster tool is FAIL (every roster tool is OK or WARN). WARN alone NEVER affects the exit — returns nil. |
| **1** | At least one roster tool is FAIL → returns `errSilent` (→ `translateExit` maps to 1). The per-tool diagnostics are already on stdout (text) or in the JSON `suggestion` fields, so `errSilent` suppresses a redundant stderr line. |

The always-OK shll-first row **never** sets `anyFail` — only the roster loop does — so it cannot move the exit code in either direction (`TestDoctor_ShllRowNeverPerturbsExit`).

A render error (`--json` marshal failure) writes `shll doctor: <err>` to stderr and also returns `errSilent` (exit 1). The exit logic is **identical for text and `--json`** — `--json` changes only the rendering, never the check logic or the exit contract. See [cli/commands](/cli/commands.md#exit-code-translation) for `errSilent`.

## Edge cases

- **Unwired shell-init tool** (binary OK, but shll's eval block absent from the rc file) → **WARN**, exit unaffected. The tool *works* when invoked directly; wiring is a convenience, not function — so it stays green-for-CI. (`TestDoctor_UnwiredShellInitWarnsExitZero`.)
- **Unresolvable / unsupported `$SHELL`** (e.g. CI with `$SHELL=/bin/sh`, or unset) → the wiring check cannot resolve an rc path, so shell-init tools degrade to **WARN** with the `suggestShellUnresolvableFmt` explanation; the binary checks (1+2) still run normally and the exit code is unaffected. Non-shell-init tools are untouched (still OK). (`TestDoctor_UnresolvableShellDegradesToWarn`.)
- **Missing / unreadable rc file** (but `$SHELL` resolved) → treated as "not wired" (a plain WARN with `suggestNotWired`), distinct from the unresolvable-shell case — the shell resolved, the wiring just isn't there yet.
- **Corrupted shll block** (rc file has an opening `# >>> shll >>>` sentinel with no matching close — `locateBlock` reports `partial`) → shell-init tools → **WARN** with `suggestCorruptBlock` (manual-cleanup hint), exit unaffected. Distinct from plain "not wired" because `shll setup shell` refuses to modify a corrupted block (exit 2), so the plain "run `shll setup shell`" hint would dead-end. (`TestDoctor_CorruptBlockWarnsWithDistinctSuggestion`.)
- **Installed but untrusted formula** (0854) → **WARN** with `suggestNotTrustedFmt` (`run 'shll install' (or 'brew trust --formula <formula>') …`), exit unaffected. Applies to any installed brew-managed roster tool (not just shell-init ones; delegated tools have no formula to check). A tap-level trust (`sahil87/tap` in the `taps` array) counts as trusting every formula → no WARN. (`TestDoctor_InstalledUntrustedWarns`, `TestDoctor_TapLevelTrustCounts`.)
- **Trust state undeterminable** (0854) — brew absent or too old to ship `brew trust` → the trust sub-check is skipped silently; no trust WARN appears regardless of actual trust state, exit 0. (`TestDoctor_TrustUnavailableSkipsCheck`.)
- **run-kit visible only under the legacy `rk` binary** → OK (not FAIL): `probeVersion` inherits the ErrNotFound-only legacy-name fallback from `probeToolVersion`, so `rk --version` reporting a version keeps the row green (the `rk` binary alias the run-kit formula still installs). (`TestDoctor_MigratedRunKitLegacyBinaryOnPathNotFail`.)
- **Delegated tool absent or probe unanswerable** (`rk desktop status` reports `Installed: not installed`; the `rk` prerequisite is missing from PATH; or the probe errors / exits non-zero — incl. run-kit's macOS-only platform refusal) → **FAIL** with the delegated install hint (`run 'rk desktop install'`) — never a `brew install` suggestion (no formula) and never a trust WARN; exit 1 like any FAIL. (`TestDoctor_RkDesktopMissingFailsWithDelegatedHint`, `TestDoctor_RkDesktopNoTrustCheck`.)
- **Delegated probe unreportable** (no `Installed:` line in the probe output, or a value that is neither a version nor the absent value — a broken app / a refused mid-check probe) → **FAIL** with `suggestDelegatedUnreportableFmt` (`'rk desktop status' reported no installed version — try reinstalling it`).
- **Binary FAIL dominates trust/wiring** → FAIL dominates; no trust or wiring WARN is shown. (`TestDoctor_BinaryFailDominatesTrust`.)

## Test seam

`doctor_test.go` (test-alongside, per `code-quality.md`) drives `runDoctor` with `bytes.Buffer` writers, a fake `proc.Runner` (`doctorFake(states map[string]doctorVersionState)` — per-tool `--version` behavior, defaulting absent tools to `dvOK`), and a map-backed env (`rcEnv(rcDir)` resolves zsh and points `ZDOTDIR`/`HOME` at a `t.TempDir()` rc file, so the wiring check NEVER touches the real `~/.zshrc`). `writeWiredRC`/`writeUnwiredRC`/`writeCorruptRC` build the rc fixtures (wired uses `tNewBlockZsh`; corrupt writes an `openSentinel` with no matching close). **Trust (0854):** the fake also answers `brew trust --help` (the availability gate) and `brew trust --json=v1` (the trust set) with a configurable trusted-set, **defaulting to "all trusted"** so the existing all-OK goldens stay unchanged. **Delegated probe (t26g):** the fake also answers rk-desktop's `rk desktop status` probe (matched via `isRkDesktopProbe`, answered with `rkDesktopStatusResult`), keyed on `states["rk-desktop"]` and defaulting to installed like the `--version` branch.

- `TestDoctor_AllOKWired` — all tools installed + a wired rc → every line OK, no problem tail; shell-init tools show `wired`, others do not.
- `TestDoctor_MissingBinaryFails` — `hop` missing → FAIL line, install suggestion (`brew install sahil87/tap/hop`), problem tail, exit `errSilent`.
- `TestDoctor_UnreportableVersionFails` — `fab-kit` both `dvUnreportable` (proc error) and `dvEmpty` (empty normalize) → FAIL, reinstall suggestion (`fab-kit --version' failed`, `brew reinstall sahil87/tap/fab-kit`), exit `errSilent`.
- `TestDoctor_UnwiredShellInitWarnsExitZero` — installed but unwired rc → `wt`/`tu`/`hop` WARN, `idea`/`run-kit`/`fab-kit` OK (no wiring check), not-wired suggestion, exit 0.
- `TestDoctor_CorruptBlockWarnsWithDistinctSuggestion` — rc file with an unclosed shll sentinel (`writeCorruptRC`) → `wt`/`tu`/`hop` WARN with `suggestCorruptBlock` (NOT the plain `suggestNotWired`), exit 0.
- `TestDoctor_MissingDominatesWiring` — `wt` missing AND unwired → FAIL (binary failure dominates the would-be wiring WARN).
- `TestDoctor_UnresolvableShellDegradesToWarn` — `$SHELL=/bin/sh` → `wt`/`tu`/`hop` WARN with the `$SHELL is` suggestion, binary checks still run, `idea` stays OK, exit 0.
- `TestDoctor_JSONShapeAndExit` — `--json` with `hop` missing + `tu` unwired → valid JSON, no ANSI, trailing newline, `len == len(Roster)+1` (shll-first object + one per roster tool), `results[0].Tool == shllSelf.Name`, roster order preserved (offset by 1), per-tool field values per marker (`hop` FAIL/missing, `tu` WARN/onpath/version_ok/unwired with `version:"v1.2.3"`, `idea` `shell_init:false`/OK), `hop` is FAIL → exit `errSilent` (same as text).
- `TestDoctor_JSONAllOKExitZero` — `--json` all-OK + wired rc → every status OK, wired shell-init tools report `wired:true`, exit 0.
- `TestDoctor_RegisteredOnRoot` — `doctor` is registered on `newRootCmd()` and `rootLong` documents `shll doctor`.

trust sub-check guards (0854):

- `TestDoctor_InstalledUntrustedWarns` — an installed tool absent from the trust set → WARN with the not-trusted suggestion, exit 0.
- `TestDoctor_TapLevelTrustCounts` — `sahil87/tap` in the `taps` array → every formula counts trusted, no trust WARN.
- `TestDoctor_TrustUnavailableSkipsCheck` — older brew (no `brew trust`) → no trust WARN even when untrusted, exit 0.
- `TestDoctor_BinaryFailDominatesTrust` — a missing binary on an untrusted tool → FAIL (with the install — not trust — suggestion), not a trust WARN (worst-check-wins).
- `TestDoctor_ShllRowNoTrustCheck` — the `shll` self row stays OK with no trust check.
- `TestDoctor_UntrustedJSONWarn` — `--json` parity: an installed-untrusted tool renders WARN with the not-trusted suggestion in the JSON object too.

shll-first row guards (bb7r):

- `TestDoctor_ShllFirstRowText` — text output's first row is `shll`, marked OK with the package-var version and no detail (binary always present, no wiring), even with the rest of the roster all-OK → exit 0.
- `TestDoctor_ShllFirstObjectJSON` — `--json` first object is the shll-self object (`tool:"shll"`, `status:"OK"`, `shell_init:false`, `wired:false`, version present from the package var).
- `TestDoctor_ShllRowNeverPerturbsExit` — the always-OK shll row does not set `anyFail`: a clean roster still exits 0 and a roster with a FAIL still exits 1 (the row cannot move the exit either way).
- `TestDoctor_ProblemTailDenominatorCountsAllRows` — one roster FAIL (`hop`) → the tail reads `1 of len(Roster)+1` (i.e. `1 of 8`): the denominator counts every rendered row, shll included.

delegated-tool guards (t26g):

- `TestDoctor_RkDesktopOK` — rk-desktop installed per its probe → OK row carrying the `Installed:`-line version, `shell_init:false`, no trust/wiring check.
- `TestDoctor_RkDesktopMissingFailsWithDelegatedHint` — rk-desktop absent → FAIL with the delegated `rk desktop install` hint, never a `brew install` suggestion, exit `errSilent`.
- `TestDoctor_RkDesktopNoTrustCheck` — trust available with nothing trusted → every installed brew tool WARNs, but rk-desktop stays OK (the trust sub-check does not apply to a formula-less tool).
- `TestDoctor_RkDesktopJSONFields` — `--json` parity: the rk-desktop object reports OK, `on_path:true`, `version_ok:true`, `shell_init:false`, `wired:false`, and the probe-derived version.

legacy-name fallback guard:

- `TestDoctor_MigratedRunKitLegacyBinaryOnPathNotFail` — a run-kit whose binary is only `rk` on PATH → OK (legacy-name fallback), not FAIL.

`lineFor`/`lineHas` are line-scanning helpers; `resultByTool` indexes a decoded JSON result slice by tool name.

## Design Decisions

### Probe-only evaluation for delegated (non-brew) tools
**Decision**: A delegated roster tool (empty `Formula`; today rk-desktop) is composed by `evaluateDelegatedTool`, where the Tool's `Probe` spec is the only check — no `--version` PATH probe, no formula-trust sub-check, no wiring check.
**Why**: there is no formula to trust and no shell-init to wire, and the artifact is a macOS app rather than a CLI — rk-desktop ships no `--version` surface, so the `Installed:` line of `rk desktop status` is the only honest installed/version fact. The brew-centric checks would WARN/FAIL on states that cannot exist for it.
**Rejected**: running the shared trust/wiring checks against a formula-less tool (a `trusts("")` lookup is meaningless, and there is no rc-file wiring to verify); probing `<name> --version` like a brew tool (no version subcommand exists to probe).
*Introduced by*: `260820-t26g-roster-desktop-entry`

## Cross-references

- Subprocess wrapper conventions and `proc.ErrNotFound` semantics: [internal/proc](/internal/proc.md). All probe subprocess work routes through `internal/proc` (Constitution I).
- Shared version probe: [cli/version](/cli/version.md) — `doctor`'s `probeVersion` reuses `version.go`'s `proc.Run`/`versionTimeout`/`normalizeVersion`, so the two share the version-probe contract and cannot drift. For the delegated rk-desktop entry, `evaluateDelegatedTool` reuses `version.go`'s shared probe-line parser `parseProbeStatusLine` — see [cli/version §the shared install probe](/cli/version.md#the-shared-install-probe).
- Shared wiring detector: [cli/setup](/cli/setup.md#block-location-and-parsing) — `doctor` reuses `resolveShell`/`resolveRcFile`/`locateBlock`/`blockMatch.hasEval` strictly read-only (never the write paths). `doctor`'s `resolveWiringFact` composition is **also** reused (read-only, in place) by [cli/install §the post-install auto-run steps](/cli/install.md#the-post-install-auto-run-steps-and-the-next-steps-block) to gate its shell-setup auto-run (which, when unwired, drives the write path).
- Shared trust-state primitives (0854): `brewTrustAvailable` + `brewTrustList` live in `brew.go` — see [cli/commands §brew.go helper inventory](/cli/commands.md#file-layout-srccmdshll). The trust-*mutating* sibling that establishes per-formula trust (so re-running it clears doctor's WARN): [cli/install §per-formula trust before install](/cli/install.md#per-formula-trust-before-install).
- Registration, exit-code sentinels, and the `Roster`: [cli/commands](/cli/commands.md) — `doctor` is one of the twelve user-facing subcommands (the hidden `help-dump` is not counted).
- The shared `shllSelf` descriptor + `shllSelfVersion()` (the single source of truth for the prepended shll-first row): [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor). The sibling surfaces that also prepend it: [cli/list](/cli/list.md#the-prepended-shll-first-row) (table row + `--json` `self:true`) and [cli/install](/cli/install.md#the-prepended-shll-first-informational-line) (informational line). `version`/`update` carry their own inline shll-first handling.
- Constitution I (Security First) → the version probe routes through `internal/proc`; rc-file access is read-only `os.ReadFile`.
- Constitution III (Wrap, Don't Reinvent + Tool Roster Source of Truth) → `doctor` reuses existing probe/wiring primitives rather than reimplementing them, derives "ships shell-init" from the live `Roster` (`len(tool.ShellInit) > 0`), and reads trust state via `brew trust --json=v1` (never parsing `~/.homebrew/trust.json`).
- Constitution V (Graceful Degradation) → an uninstalled or unwired tool degrades to FAIL/WARN with an actionable suggestion rather than crashing; an unresolvable `$SHELL` degrades wiring to WARN while binary checks proceed; an undeterminable trust state (brew absent / too old) skips the trust sub-check silently.
- Constitution VII (Minimal Surface Area) → `doctor` is a new top-level subcommand (sixth), justified as a read-only cross-tool diagnostic distinct from `version` (reporting, always exit 0), `install`/`update` (mutating), and per-tool CLIs (no single tool can see the *composed* shll block). `--json` is a flag on `doctor`, not a separate subcommand. Justification recorded in the change intake (260609-d0ct) and [cli/commands](/cli/commands.md#constitution-vii-justification-per-subcommand).
