---
type: memory
description: "`shll install` — per-formula trust (`--no-trust` opt-out), bootstrap of missing roster tools via `brew install`, idempotent re-run. Post-outcome it auto-runs `shll shell-setup` and `shll agent-setup --yes` in-process (opt-outs `--no-shell-setup`/`--no-agent-setup`; failures warn + nudge, never failing the install; never under `--dry-run`). The `exec` target of the `curl … | sh` bootstrap: the script owns pre-brew, this command post-brew; arg/flag pass-through is public surface."
---
# cli/install

`shll install` — installs every roster tool that isn't already installed via Homebrew. Idempotent; safe to re-run.

Source: `src/cmd/shll/install.go`, with shared brew helpers in `src/cmd/shll/brew.go`.

## The `curl | sh` upstream entry point

`shll install` is also the delegation target of the copy-paste install one-liner. The bootstrap script `scripts/install.sh` (served at `shll.ai/install`) owns the whole pre-brew phase — it preflights git/CLT, curl, and tmux (consolidated report, per-platform fix commands), bootstraps Homebrew headlessly (`NONINTERACTIVE=1` official installer) when brew is absent and threads the absolute `$BREW` path plus shellenv — then trust-then-installs `shll` itself only if it is missing, and ends with `exec shll install "$@"` — forwarding every arg verbatim as the install subset:

```sh
curl -fsSL https://shll.ai/install | sh                # → exec shll install        (whole roster)
curl -fsSL https://shll.ai/install | sh -s -- hop wt   # → exec shll install hop wt  (subset)
```

Two implications for `shll install`'s contract:

- **The arg pass-through is part of `shll install`'s public surface.** The [positional tool-name subset args](#positional-tool-name-args--subset-targeting) are what a piped `sh -s -- <tools…>` reaches — and the pass-through is verbatim, so flags ride it too: `curl -fsSL https://shll.ai/install | sh -s -- --no-agent-setup` lands as `exec shll install --no-agent-setup`. The bootstrap adds no filtering of its own — it hands the args straight to `runInstall`, which validates them (`resolveTargets`, `allowShll=false`; unknown/`shll` targets still hard-error, the alias `rk` still resolves to `run-kit`).
- **The script owns the pre-brew phase; `shll install` owns everything post-brew.** Preflight, the Homebrew bootstrap, and the shll-self trust/install live in the script; roster knowledge, subset filtering, per-formula trust for the other six tools, and graceful skips all live here, not in the script (Constitution III). The script's job is the phase `shll install` cannot reach — the user cannot have `shll` without Homebrew having worked. See [ci/install-bootstrap](/ci/install-bootstrap.md) for the script contract and the shll.ai raw-fetch URL contract.

## Behavior contract

The full happy/unhappy paths, in the order `runInstall` evaluates them (`src/cmd/shll/install.go`):

1. **Brew missing.** If `hasBrew(ctx)` returns false, write `installBrewMissingHint` to stderr and return `errSilent`. Exit code: 1. The literal hint is `"shll install requires Homebrew. Install from https://brew.sh"` (`src/cmd/shll/brew.go`). It is intentionally separate from `brewMissingHint` (used by `shll update`) so each command's error tells the user which command they ran — the update spec scenario asserts its verbatim text, so reusing the same constant for both commands would either violate that lock or mislead `shll install` users. (Subset resolution via `resolveTargets(args, false)` runs *before* this guard — an unknown/`shll` target errors first, with no brew side effect; see [Positional tool-name args](#positional-tool-name-args--subset-targeting).)

   **Then — shll-first informational line** (bb7r): immediately after this brew-missing guard passes, `runInstall` writes `shllSelfInstallNote` (`"shll — already present / self-managed"`) to stdout. Placed *after* the brew-missing/unknown-target guards, so it leads the nothing-to-do, dry-run, and install-loop paths but **not** the early-error paths. Informational only — shll is never a `brew install` target. See [The prepended shll-first informational line](#the-prepended-shll-first-informational-line).

2. **Partition the roster into a `[]Tool` of missing tools.** Iterate the roster in order, call `isInstalled(ctx, t.Formula)` for each, and collect the missing entries — every tool (run-kit included) goes through the same plain check. A missing run-kit is a plain `brew install sahil87/tap/run-kit`; a legacy-`rk`-keg-only machine classifies run-kit as missing and installs the new formula (the orphan `rk` keg is manual cleanup per run-kit's README). A present run-kit is skipped (idempotent).

3. **Nothing missing → short-circuit.** If `len(missing) == 0`, write `All shll tools already installed.` to stdout, then (unless `--dry-run`) run the [post-install auto-run steps](#the-post-install-auto-run-steps-and-the-next-steps-block) via `runPostInstallSetup(ctx, env, stdout, stderr, colorEnabled(stdout), noShellSetup, noAgentSetup)`, and return nil. Exit code: 0. No `brew update` is invoked — there's nothing to install. (A re-runner who never wired their shell gets wired from this path — the steps are idempotent, so the short-circuit re-runner is their exact beneficiary.)

4. **No `brew update --quiet`.** Unlike `shll update`, `shll install` does NOT refresh brew metadata first. `brew install sahil87/tap/<formula>` resolves the formula via the tap directly, and the spec freezes this distinction (Design Decision: install ≠ update). `TestInstall_NoBrewUpdateInvoked` pins the contract.

5. **Sequential per-tool install — trust then install (0854).** For each missing tool in roster order, print its per-tool header (see [Per-tool output separation](#per-tool-output-separation)), then — when trust is enabled — record per-formula trust via `brewTrustFormula(ctx, t.Formula)` *immediately before* the action `proc.RunForeground(ctx, brewBinary, "install", t.Formula)`. The trust step is interleaved in the per-tool loop (not a separate up-front pass), so trust stays adjacent to the install it unblocks (installed ≠ trusted). Best-effort across the roster: on per-tool install failure (transport error or non-zero exit), set `anyFailed = true` and `continue`. See [Per-formula trust before install](#per-formula-trust-before-install).

6. **Summary tail, then the post-install auto-run steps.** After the loop, print one summary line via `printSummaryTail` (see [Per-tool output separation](#per-tool-output-separation)), then run the [post-install auto-run steps](#the-post-install-auto-run-steps-and-the-next-steps-block) via `runPostInstallSetup(ctx, env, stdout, stderr, color, noShellSetup, noAgentSetup)` — reusing the loop's single `color` decision, and run **regardless of `anyFailed`** (the steps are best-effort and orthogonal to install outcome). Then — unchanged — if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0). The tail is presentation-only and does not change the exit code.

## The prepended shll-first informational line

`runInstall` prepends a single shll-first line to stdout — `fmt.Fprintln(stdout, shllSelfInstallNote)` (`src/cmd/shll/install.go:94`) — so the toolkit reads as one family with `shll` as its manager-member (the discoverability goal shared with `list`/`doctor`). It is the install-side instance of the unified shll-first ordering — see [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor).

```go
// install.go
const shllSelfInstallNote = "shll — already present / self-managed"
```

Two load-bearing properties:

- **Never a brew install action on the running binary.** You cannot `brew install` the running orchestrator, so the line is **informational only** — no subprocess, no `brew install sahil87/tap/shll`. shll is also rejected as an explicit positional install target (`resolveTargets(args, false)`, `allowShll=false`; b2vg), so it can never enter the `missing` set. `TestInstall_ShllFirstInformationalLine` asserts no `brew install` of the shll formula is ever recorded.
- **Placement: after the guards, before the roster framing.** The line is written *after* the brew-missing guard (and after the up-front `resolveTargets` unknown-target check) but *before* the roster is partitioned. So it leads the three terminal paths that reach the install decision — **nothing-to-do** (`All shll tools already installed.`), **`--dry-run` preview**, and the **install loop** — but is **NOT** emitted on the early-error paths (brew missing → only the stderr hint; unknown/`shll` target → only the stderr error). It goes to **stdout**, never stderr (`TestInstall_ShllFirstInformationalLine` also asserts this).

This is a deliberate *informational* exception to the symmetry between the inspect surface (`list`/`doctor`, which render shll as a full row/object) and `install` (which *acts*): shll cannot be acted on, so its representation here is a leading note rather than an actionable row.

> **Note — the empty/nothing-to-do golden.** The all-already-installed stdout is `shll — already present / self-managed\n` then `All shll tools already installed.\n` (bb7r). The [Per-tool output separation §empty case](#per-tool-output-separation) statement that the empty-case stdout is "**exactly** `allInstalledMsg`" holds for the install-loop framing only (no `==>` header, no tail, no blank lines); the informational line precedes it on every non-early-error path.

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All installs succeeded (or all-already-installed branch) | 0 |
| Unknown/typo'd positional target — incl. `shll`, which is rejected (b2vg) | 1 (via `errSilent`, before any brew work) |
| `brew` not on PATH | 1 (via `errSilent`, hint already on stderr) |
| Any per-tool `brew install` failed | 1 (via `errSilent`, after all missing tools attempted) |

## Per-tool output separation

`shll install` mirrors `shll update`'s framing exactly, via the same shared helper `src/cmd/shll/ui.go` (see [cli/commands](/cli/commands.md#file-layout-srccmdshll)) — no TTY/`NO_COLOR`/glyph logic is duplicated in `install.go`.

- **Per-tool header with `[N/M]` progress counter (6vuo; color form 13k3).** Before each missing tool's `brew install` output, `printToolHeader(stdout, t.Name, i+1, total, color)` (`install.go`) writes `▸ [N/M] <tool>` (color TTY — the whole `▸ [N/M] <tool>` run is one bold-cyan span, mirroring update via the shared `printToolHeader`) / `==> [N/M] <tool>` (plain ASCII), in roster order, where `N` is the 1-based loop position and `M = len(missing)` — already known up front, so no separate denominator computation is needed (unlike `update`, where `M` is derived from the probe results). The roster is leaves-first (`wt, idea, tu, run-kit, hop, fab-kit` — auvj), so the headers for the *missing subset* print in that relative order — e.g. with `hop`+`wt` already installed, the missing set `{idea, tu, run-kit, fab-kit}` yields `==> [1/4] idea`, `==> [2/4] tu`, `==> [3/4] run-kit`, `==> [4/4] fab-kit` (`TestInstall_HeadersAndTail` golden, with the `Done — 4 of 4 tools succeeded in 1m12s.` tail). See the [leaves-first ordering rationale](/cli/commands.md#design-decision-leaves-first-roster-order).
- **Section spacing (6vuo).** A single blank line precedes each per-tool header **except the first** (`install.go:106`, `if i > 0`), and a single blank line precedes the summary tail (`install.go:128`) — so each tool's streamed output is separated from the next header and the tail. The all-already-installed short-circuit emits no blank lines.
- **Summary tail with run duration (6vuo).** After the loop, `printSummaryTail(stdout, succeeded, total, elapsed, color)` (`install.go:129`, `total = len(missing)`) writes `Done — N of M tools succeeded in <dur>.` (green `✓` when color) or `X succeeded, Y failed in <dur> — see above.` (duration before the em-dash), by **exit code only** — `succeeded` counts installs that exited 0, mirroring the same per-tool facts that drive `anyFailed`. The duration is a run fact, not an outcome claim — the tail still never claims "installed" vs. "up-to-date" (the honesty constraint). Presentation-only; does not change the exit code. Elapsed is measured via the injectable `nowFunc` clock seam (`clock.go`), captured at `install.go:101` **after** the short-circuit and the dry-run branch return, so it covers only the install phase.
- **Stream discipline.** Header and tail go to **stdout** (the stream `brew install` is foregrounded onto), never stderr.
- **Color gating.** One `colorEnabled(stdout)` decision (TTY via `golang.org/x/term` AND `NO_COLOR` unset), reused for headers and tail; `bytes.Buffer` test writers hit the plain-ASCII branch.
- **Empty case emits no header, no tail, no counter, no spacing, no duration.** The all-already-installed short-circuit (step 3) runs no loop, so the *install-loop framing* it would emit is absent — no `==> [N/M]` header, no tail, no blank lines, no duration; only the install-loop path carries those markers. Its install-message line stays `All shll tools already installed.\n` (the `allInstalledMsg` constant). **The shll-first informational line precedes it** (`shll — already present / self-managed\n` then `All shll tools already installed.\n`) on this non-early-error path (bb7r) — see [The prepended shll-first informational line](#the-prepended-shll-first-informational-line); pinned by the `TestInstall_AllAlreadyInstalled`/`TestInstall_EmptyCaseNoHeaderNoTail` goldens.

The helper details (named SGR constants, the `colorEnabled` gating, the honesty constraint on the tail, the `[N/M]` counter, the `formatDuration` form, and the `nowFunc` clock seam) are documented once under [cli/update](/cli/update.md#per-tool-output-separation); `install` consumes the identical helpers.

## Per-formula trust before install

Homebrew 6.0 turned tap-trust from an advisory warning into a **hard install requirement** (`HOMEBREW_REQUIRE_TAP_TRUST` now defaults to `true`). shll's tap formulae are binary-download formulae with a `def install` (not a `bottle do` pour), so `brew install sahil87/tap/<formula>` runs a *sandboxed* install whose in-sandbox trust re-check requires a **persisted** trust record — naming the qualified formula on the CLI is not enough. So `shll install` now establishes that trust itself, per-formula, before each install.

```sh
brew trust --formula sahil87/tap/<formula>   # per tool in the install set, before its brew install
```

- **Default behavior.** `shll install` (and a subset like `shll install hop wt`) records per-formula trust for each missing tool before installing it. `brew trust` is idempotent (`Already trusted formula: …`, exit 0), so re-runs stay clean.
- **`--no-trust` opt-out.** The cobra bool flag `--no-trust` (`noTrustFlag`/`noTrustFlagUsage` constants, `install.go`) skips the trust step entirely, for users who manage trust themselves. The install attempts proceed unchanged.
- **Per-formula granularity, NOT whole-tap.** Trust is `brew trust --formula sahil87/tap/<formula>`, never `brew trust --tap` — Homebrew recommends per-formula trust for third-party taps, and shll knows its exact roster, so it trusts only what it actually manages. (The removed `shell-setup --trust-tap` did whole-tap — see [cli/shell-setup](/cli/shell-setup.md).)
- **The trust capability is probed ONCE up front.** `trustEnabled := !noTrust && brewTrustAvailable(ctx)` (`install.go:175`) is computed before the install loop — `brewTrustAvailable` is the shared capability probe (`brew trust --help`), reused (not reimplemented) from `brew.go`. The per-tool trust call runs only when `trustEnabled`.
- **Graceful degradation (Constitution V).** When `brew trust` is unavailable (brew too old to ship it — pre-6.0, where trust isn't required anyway) the step is skipped silently. When a per-formula `brewTrustFormula` *fails* (transport error or non-zero exit), `shll install` writes a warning to stderr (`shll install: <tool>: trust step failed: … (continuing to install)` or `… trust step exited <code> …`) and **continues to the install attempt** rather than aborting — and a trust failure **does NOT set `anyFailed`**. The install's own exit code is the sole authority on whether the tool succeeded (so a genuine untrusted-tap failure surfaces as brew's own install error, not a duplicate trust error). The new `brewTrustFormula(ctx, formula) (int, error)` helper in `brew.go` routes through `proc.RunForeground` (Constitution I), foregrounded so the user sees brew's own `Trusted formula:` / `Already trusted formula:` line.
- **Bootstrap note.** shll cannot trust its own formula before it exists — `brew trust --formula sahil87/tap/shll && brew install sahil87/tap/shll` remains the one-time README bootstrap. `shll install` owns trust for the other six.

> **No sandbox-trust env workaround.** The brew install call site is plain `proc.RunForeground(ctx, brewBinary, "install", t.Formula)` — no `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` injection (0854, closes backlog `[tkch]`; the upstream Homebrew bug is fixed in 6.0.4, and the per-formula trust above is the correct DX). shll requires Homebrew ≥ 6.0.4; the floor is documented in the README, not gated in code. See [cli/update §trust posture](/cli/update.md#trust-posture-and-the-homebrew-604-floor) and [internal/proc](/internal/proc.md).

Tests (`install_test.go`): `TestInstall_TrustsEachFormulaBeforeInstall` (per-tool trust precedes the install, and is per-formula — never `--tap`), `TestInstall_NoTrustSkipsTrustStep` (`--no-trust` → no `brew trust` calls), `TestInstall_TrustUnavailableSkipsGracefully` (older brew → no trust calls, install proceeds, exit 0), `TestInstall_TrustFailureContinues` (trust non-zero → warning, install still attempted, exit reflects install only).

## run-kit is a plain install target

run-kit carries no special install path — the missing-partition classifies it via the same `isInstalled(ctx, t.Formula)` check as every other tool. A missing run-kit (including a legacy-`rk`-keg-only machine, where `sahil87/tap/run-kit` probes not-installed) is a plain trust + `brew install sahil87/tap/run-kit`; orphan `rk`-keg cleanup on such a machine is manual per run-kit's README (`brew uninstall sahil87/tap/rk`). `shll install rk` routes through the shared `legacyAliases`/`printAliasNotices` path (prints `note: rk is now run-kit`), then installs the canonical run-kit tool — `allowShll=false` still holds, so `shll` is never an install target.

Tests (`install_test.go`): `TestInstall_LegacyAliasResolvesWithNotice` (`shll install rk` → alias notice + canonical resolution).

## The post-install auto-run steps and the "Next steps" block

`shll install` ends by **running** the two follow-on wirings itself, not nudging toward them: after the install outcome it auto-runs the shell-setup step, then the agent-setup step, and renders an adapted "Next steps" block containing only the lines that still apply. It exists because `shll install` is the delegation target of the `curl … | sh` bootstrap (`scripts/install.sh` ends with `exec shll install "$@"` — see [The `curl | sh` upstream entry point](#the-curl--sh-upstream-entry-point)): the homepage copy-paster's *entire* first-run experience terminates in this command's output, and a nudge that's routinely ignored is a silent adoption failure — the machine leaves this command fully wired. Both steps are idempotent and reversible (the sentinel-block re-run is a no-op and `shell-setup --uninstall` removes it; agent-setup overwrites shll-owned skill files and has `--uninstall`), announced by the steps' own output, and opt-out-able via `--no-shell-setup` / `--no-agent-setup`. (gjhx)

The orchestrator is `runPostInstallSetup(ctx, env, stdout, stderr, color, noShellSetup, noAgentSetup)` (`src/cmd/shll/install.go`). Illustrative happy-path shape on a fresh machine:

```
Done — 3 of 3 tools succeeded in 42s.
Installed shll shell integration to /Users/x/.zshrc. Restart your shell or run: source /Users/x/.zshrc
wrote /Users/x/.agents/skills/shll-toolkit/SKILL.md
wrote /Users/x/.claude/skills/shll-toolkit/SKILL.md
… (run-kit's own delegated output)

Next steps:
  → exec $SHELL         # load the just-wired shll integration into your current shell (or open a new terminal)
```

### Step 1 — shell wiring (auto `shll shell-setup`)

Gated by doctor's read-only `resolveWiringFact(env)` (`shellGateOpen := w.shellResolved && !w.corrupt && !w.wired` — one detection path, Constitution III):

- **Unresolvable `$SHELL`** (e.g. fish) or a **corrupt open-without-close block** → **quiet skip**: no write, no nudge, no reminder, no stderr. A nudge would dead-end (`shell-setup` would exit 2 on the first, refuses the second); doctor owns the corrupt-block diagnostic. (The 93r2 quiet edge states.)
- **Already wired** → silent skip — idempotency makes the auto-run a no-op, so there is nothing to announce.
- **Unwired** → auto-run **in-process** via the same write path the standalone command uses: pre-resolve `shell := resolveShell(nil, env)` / `rcPath := resolveRcFile(shell, env)` through the env seam, then `runShellSetupDefault(shell, rcPath, false, stdout, stderr)` (`src/cmd/shll/shell_setup.go`). On success shell-setup's own `Installed shll shell integration to <path>.` output announces the wire and the `exec $SHELL` reminder line is queued for the block. On failure (e.g. the rc file does not exist — shell-setup never creates one) the actionable error text goes to stderr, `shellSetupAutoRunWarn` warns, and the **gated** shell-setup nudge is the fallback. The error is consumed, never propagated.
- **`--no-shell-setup`** (`noShellSetupFlag`/`noShellSetupFlagUsage` constants, mirroring `noTrustFlag`) → skip the auto-run; the gated nudge prints instead — for dotfile-manager users who wire rc files themselves.

See [cli/doctor §the wiring fact](/cli/doctor.md#the-wiring-fact--resolvewiringfact-read-only-reuse) for the `wiringFact` shape and [cli/shell-setup](/cli/shell-setup.md) for the write-path contract install inherits.

### Step 2 — agent wiring (auto `shll agent-setup --yes`)

Immediately after the shell step, unless `--no-agent-setup` (`noAgentSetupFlag`/`noAgentSetupFlagUsage` constants): run `runAgentSetup(ctx, env, stdout, stderr, false, false, true)` (`src/cmd/shll/agent_setup.go`) **in-process** — the equivalent of `shll agent-setup --yes`. It places the shll-toolkit skill into both shll-owned global skill dirs, then delegates `run-kit agent setup --yes` — `--yes` forwarded so run-kit's hook-wiring confirmation cannot hang an unattended install (under the curl bootstrap stdin is the pipe; without `--yes` run-kit refuses non-interactively). The per-path `wrote`/`unchanged`/`updated` summary plus run-kit's own output are the announcement. run-kit absent (`proc.ErrNotFound`) → silent skip (inherited Constitution V behavior), and the step still counts as success. A run-kit **delegation** failure stays non-fatal and does **not** trigger the nudge — re-running `shll agent-setup` would hit the same delegation failure, so the nudge would dead-end (the same logic as the quiet edge states); only a **placement** failure (`runAgentSetup`'s return) warns (`agentSetupAutoRunWarn`) and falls back to the agent-setup nudge. See [cli/agent-setup](/cli/agent-setup.md).

### Emission points — loop tail + short-circuit (both non-preview outcome paths)

`runPostInstallSetup` is called on exactly the two paths that report an *outcome*, after the outcome line:

- **Install-loop path** — after `printSummaryTail(...)` (`runInstall`, reusing the loop's single `color`). Runs regardless of `anyFailed` — the steps are best-effort, and the tail already conveys per-tool failures.
- **Short-circuit path** — after the `allInstalledMsg` line (`"All shll tools already installed."`), gated on `!dryRun`. This path computes its own `colorEnabled(stdout)` (the loop's single decision is never reached here). The re-runner who never wired their shell is this path's exact beneficiary.

It is **never** reached on the [`--dry-run`](#--dry-run) path (a command preview, not an outcome — the auto-run steps are writes and MUST NOT run there), nor on the brew-missing / unknown-target early returns (they `return` before any outcome). The short-circuit call is explicitly `!dryRun`-guarded because that short-circuit *precedes* the dry-run branch; the loop-tail call is naturally after the dry-run branch's early return.

### Exit-code authority

A failure of either auto-run step **never changes `shll install`'s exit code** — the install outcome (`anyFailed` from the brew loop) remains the sole authority, the same posture as the trust step. Errors returned by the seams are consumed, never propagated.

### The adapted block and the named constants

The block goes to **stdout** after both steps, preceded by a blank line (the existing section-spacing rule), with the arrow glyph degrading via `arrow(color)` — and prints **only when at least one line applies** (no empty `Next steps:` header): the gated shell-setup nudge (opt-out or failure), the agent-setup nudge (opt-out or placement failure), and the `exec $SHELL` reminder (after a successful auto wire). The fully-wired happy path — wired rc or fresh wire, successful agent-setup — prints no block at all, or the reminder alone. Every user-facing string is a **named constant** in `install.go`:

```go
const (
	nextStepsHeader      = "Next steps:"
	shellSetupNudgeFmt   = "  %s shll shell-setup    # wire shell integration into your rc file, then: exec $SHELL"
	agentSetupNudgeFmt   = "  %s shll agent-setup    # optional, once per machine — wire agent harnesses (toolkit context + run-kit dashboard hooks)"
	execShellReminderFmt = "  %s exec $SHELL         # load the just-wired shll integration into your current shell (or open a new terminal)"
)
const (
	shellSetupAutoRunWarn = "shll install: automatic shell setup failed (continuing)"
	agentSetupAutoRunWarn = "shll install: automatic agent setup failed (continuing)"
)
```

The `%s` in each block line is the arrow glyph. The shell-setup nudge wording tracks doctor's `suggestNotWired` (`run 'shll shell-setup' then 'exec $SHELL'`); the agent-setup nudge wording describes `shll agent-setup`'s work (toolkit-skill placement + run-kit hook delegation).

> **The `runKitToolName = "run-kit"` constant lives in `install.go`, but its sole consumer is `agent_setup.go`'s `delegateRunKitAgentSetup`** (the run-kit binary name for the delegation subprocess). See [cli/agent-setup §run-kit delegation](/cli/agent-setup.md#run-kit-delegation).

### The `env` seam on `runInstall`

`runInstall` takes an `env func(string) string` parameter — threaded into `runPostInstallSetup` for the shell-setup gate (`resolveWiringFact(env)`), the shell/rc pre-resolution for the auto-run, and agent-setup's skill-target derivation — mirroring `runDoctor`'s established test seam (`runDoctor(ctx, jsonOut, env, stdout, stderr)`). The cobra factory `newInstallCmd` passes `os.Getenv`; `install_test.go` passes a map-backed `envFunc` pointing at a `t.TempDir()` rc file and `$HOME` reached via a faked `$SHELL`/`$ZDOTDIR`/`$HOME`, so the wiring probe and the auto-run writes NEVER touch the real `~/.zshrc` or real skill dirs. The auto shell-setup pre-resolves via `resolveShell(nil, env)`/`resolveRcFile(shell, env)` rather than the `runShellSetup` wrapper, so no internal `os.Getenv` escapes the seam.

### Constitution fit

I — no new subprocess path in command code (both steps are in-process seam calls; agent-setup's own run-kit delegation routes through `internal/proc` as before). II — the gates are re-derived per invocation, no state. III/IV — install *composes* `shll shell-setup` / `shll agent-setup` by calling their existing seams, never absorbing their logic. V — every auto-run failure degrades to warn + nudge; the quiet edge states suppress dead-end nudges; run-kit absent is a silent skip. VII — two flags on the existing `install` subcommand, no new commands.

Tests (`install_test.go`): `TestInstall_AutoShellSetupWiresRcFile` (fresh unwired rc → sentinel block appended, shell-setup's own announcement + the `exec $SHELL` reminder, no nudge), `TestInstall_AutoShellSetupIdempotentRewire` (second run is a byte-identical silent skip — no reminder, no nudge), `TestInstall_AutoShellSetupQuietSkips` (wired / unresolvable `$SHELL` / corrupt block → no write, no nudge, no reminder, no stderr), `TestInstall_AutoShellSetupFailureDegrades` (missing rc → actionable stderr message + warn + gated nudge, exit 0, rc never created), `TestInstall_AutoAgentSetupPlacesSkillsAndDelegatesYes` (both skill files placed with canonical bytes, foreground `run-kit agent setup --yes` argv pinned, fully-wired happy path prints no `Next steps:` header), `TestInstall_AutoAgentSetupRunKitAbsentSilentSkip` (placement still happens, no nudge, no stderr), `TestInstall_AutoAgentSetupDelegationFailureContinues` (rk < v3.16.23 version skew → warn-and-`(continuing)`, NO nudge, install unaffected), `TestInstall_AutoAgentSetupFailureDegrades` (unwritable skill dir → per-path diagnostic + warn + agent nudge, exit unchanged). The nudge-era tests survive with adapted semantics: `TestInstall_ShellSetupNudgeShownWhenUnwired` (both opt-outs restore the nudge-era block — both lines, no writes), `TestInstall_ShellSetupNudgeHiddenWhenWired` (wired rc + both opt-outs → agent-only block, `nextStepsAgentOnly`), `TestInstall_AgentSetupNudgeOnOptOut` (the agent nudge is the `--no-agent-setup` fallback, run-kit present and absent), `TestInstall_NoNudgesOnDryRun` (dry-run runs neither step and prints no nudge), `TestInstall_DryRunEmptyCaseNoNudge`, `TestInstall_ShortCircuitPathNudgesWhenUnwired`. The golden-string tests thread a **wired** env via the shared `installWiredEnv(t)` helper.

## `--dry-run`

`shll install --dry-run` previews the `brew install` commands the run **would** execute, then exits 0 **without any write**. It mirrors `shll update --dry-run` (see [cli/update](/cli/update.md#dry-run) for the shared contract); the flag, usage string, and the `dryRun bool` parameter on `runInstall` are the same `dryRunFlag`/`dryRunFlagUsage` constants (defined in `update.go`, shared across both commands).

**Reads run; writes do not.** The `isInstalled` probes (`brew list --formula --versions`) that partition the roster still run in dry-run (they are reads, and the preview depends on them) — but **no `brew install`** is performed. The guarantee is structural: the dry-run branch (`install.go:80`) returns before the install loop and before `start := nowFunc()`. `TestInstall_DryRunNoWrites` asserts the `brew list` probe IS recorded, no `brew install <formula>` runs for any tool, and there are **zero `TransportForeground`** calls.

**The preview.** Preceded by the shll-first informational line (bb7r — the dry-run path reaches the install decision, so it leads with `shllSelfInstallNote`), then a header line `Would install N tools:` (`installPreviewHeaderFmt`) — **no metadata-refresh annotation**, since `install` runs no `brew update` (consistent with [Design Decision #2](#2-no-metadata-refresh)) — then one aligned row per missing tool, in roster order, each reading `brew install sahil87/tap/<formula>` (built as `argvString(brewBinary, "install", t.Formula)`). Formatting reuses the same `ui.go` `printInstallPreview` → `printPreviewRows` aligned-column layout as `update`: 2-space indent, labels left-padded to the longest *missing* label present, 2-space gap before the command. No `[N/M]` counter, no blank-line spacing (the preview is a static table).

```
Would install 4 tools:
  idea     brew install sahil87/tap/idea
  tu       brew install sahil87/tap/tu
  run-kit  brew install sahil87/tap/run-kit
  fab-kit  brew install sahil87/tap/fab-kit
```

(`TestInstall_DryRunPreview` golden — `hop`+`wt` installed, the other four missing; run-kit previews the plain `brew install sahil87/tap/run-kit` like any missing tool. The longest missing label `fab-kit`/`run-kit` (7) sets the column width. The test also asserts the preview does NOT mention "metadata refresh".)

**Graceful degradation (Constitution V).** Only the missing subset is listed; already-installed tools are omitted (they are filtered out into `missing` before the preview builds).

**Empty case.** When every roster tool is already installed, the dry-run path never reaches the preview builder — the shared all-already-installed short-circuit (step 3) fires first, so stdout is the shll-first informational line then `All shll tools already installed.\n` (i.e. `shllSelfInstallNote + "\n" + allInstalledMsg + "\n"`, bb7r), exit 0, no preview table, no install (`TestInstall_DryRunEmptyCase`). Under `--dry-run` the short-circuit's `runPostInstallSetup` call is `!dryRun`-gated, so **neither auto-run step runs and no nudge** prints even here (`TestInstall_DryRunEmptyCaseNoNudge`) — see [The post-install auto-run steps §emission points](#emission-points--loop-tail--short-circuit-both-non-preview-outcome-paths).

**Brew-missing precondition unchanged.** A missing brew still writes `installBrewMissingHint` to stderr and exits 1 (the `hasBrew` check precedes the dry-run branch).

## Positional tool-name args — subset targeting

`shll install [tool...]` accepts zero or more positional tool-name args (`Args: cobra.ArbitraryArgs`, parsed args threaded into `runInstall`), symmetric with [`shll update`](/cli/update.md#positional-tool-name-args--subset-targeting) for the install lifecycle. The shared resolver is single-sourced with `Roster`; install differs from update in exactly one way — the valid-target set.

- **Zero args → whole-roster run, unchanged.** `subset := len(args) > 0` is false; the partition/install behavior above holds verbatim.
- **One or more args → operate on just the named subset.** The args form a *set*, not a sequence.

**Valid targets for `install` are the six `Roster` tools ONLY** (`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit`; the legacy alias `rk` also resolves to `run-kit`). **`shll` is NOT a valid install target** — you cannot `brew install` the running orchestrator. `runInstall` calls `resolveTargets(args, false)` (`allowShll=false`), so `shll install shll` falls into the unknown-target error path (`shll install: unknown target "shll" (valid targets: wt, idea, tu, run-kit, hop, fab-kit)`) — note `shll` is absent from the valid list (it appears only for `update`, where `allowShll=true`), and `rk` is absent too (accepted as an alias but never advertised).

**Roster-order processing.** A subset is processed in `Roster` (leaves-first) order regardless of arg order — `resolveTargets` returns the selected `Tool`s in roster order, and `runInstall` walks `consider = selected` (else the full `Roster`) to build `missing`, preserving that order. Example: `shll install fab-kit wt` installs `wt` then `fab-kit`. (Why leaves-first is output coherence, not correctness: [leaves-first ordering rationale](/cli/commands.md#design-decision-leaves-first-roster-order).)

**Validation up front (`runInstall` resolves the subset before `hasBrew` and any probe).** An unknown / typo'd name → `resolveTargets` returns a non-nil error; `runInstall` writes `shll install: <detail>` to stderr and returns `errSilent` (exit 1) with **no brew side effect**. All unknown args are reported at once.

**Named-already-installed → the existing nothing-to-do path.** For `install`, "not installed" is the happy path. The inverse edge — a tool named explicitly that is *already* installed — is **not** an error: it is filtered out into the (empty-for-it) `missing` set, exactly like the whole-roster idempotent skip. If every named target is already installed, the run hits the existing short-circuit and prints `All shll tools already installed.` (exit 0). (Contrast `update`, where a named-but-not-installed target *is* an error — the asymmetry follows from the inverted precondition: install acts on absent tools, update acts on present ones.)

**Counter denominator `M` = subset size.** `M = len(missing)`, where `missing` is now restricted to the named-and-missing subset, so the per-tool `[N/M]` header and the summary-tail `M` reflect the subset, not the whole roster. The [per-tool output separation](#per-tool-output-separation) contract is otherwise unchanged.

**`--dry-run` previews the filtered subset.** The dry-run branch runs after `missing` is built from the subset, so it previews only the named-and-missing tools in roster order, header `Would install N tools:` with `N` = subset size.

## Constitution VII justification

> *Why a new top-level subcommand?* `install` is a distinct lifecycle operation from `update`: different precondition (tool not installed vs. installed), different failure modes (no metadata-refresh dependency), and different discoverability (a new user wanting "get me the toolkit" looks for `install`). Cannot be cleanly expressed as a flag on `update` because `update`'s installed-only precondition would have to invert for a subset of the run.
>
> *Rejected*: `shll update --install-missing`. The branching gets messy and the verb mismatch hurts new-user discoverability.

## Design Decisions

### Post-install steps run in-process via the existing seams
**Decision**: The auto-run steps invoke `runShellSetupDefault` (with install-side `resolveShell`/`resolveRcFile` pre-resolution) and `runAgentSetup` directly, not a `shll`-self subprocess.
**Why**: Both seams live in package `main` with injectable env/writers; in-process reuse is hermetic under the existing test seams and strictly stronger than Constitution I's proc-routing requirement (no subprocess at all). Pre-resolving shell/rc for `runShellSetupDefault` avoids `runShellSetup`'s internal `os.Getenv`, keeping install's `env` seam authoritative.
**Rejected**: `proc.RunForeground(ctx, "shll", "shell-setup")` self-exec — an unnecessary subprocess, PATH-dependent, and untestable without a real binary.
*Introduced by*: 260819-gjhx-install-auto-shell-agent-setup

### A run-kit delegation failure does not trigger the agent-setup nudge
**Decision**: Inside the auto agent-setup step, a delegation failure stays non-fatal and does NOT fall back to the nudge; only a `runAgentSetup` placement failure does.
**Why**: Inherits the standalone command's exact semantics — `delegateRunKitAgentSetup` warns with `(continuing)` and never fails placement; re-running `shll agent-setup` would hit the same delegation failure, so nudging would dead-end (the same logic as the shell-setup quiet edge states).
**Rejected**: Plumbing a delegation-outcome signal out of `runAgentSetup` — changes the standalone command's contract for a nudge that would dead-end anyway.
*Introduced by*: 260819-gjhx-install-auto-shell-agent-setup

### The "Next steps:" block prints only when non-empty
**Decision**: The block (header + lines) renders only when at least one line applies; the fully-wired happy path prints no header at all.
**Why**: An empty header is noise; the happy-path output should read as "done and wired", with only the `exec $SHELL` reminder.
**Rejected**: An unconditional header with an "all wired" line — a new string with no action for the user.
*Introduced by*: 260819-gjhx-install-auto-shell-agent-setup

## Spec-locked Design Decisions for this subcommand

### #1 Skip-already-installed semantics (not re-install)

> *Why*: Idempotent re-runs are the common case for bootstrap — a user runs `shll install`, installs four tools, then later adds two more to the roster (after a shll release). The second `shll install` should pick up only the new ones. Re-installing what's already present is wasted I/O and noise.
> *Rejected*: `--force` flag for re-install. YAGNI for v0.1.0; users can `brew reinstall sahil87/tap/<formula>` directly when they want it.

### #2 No metadata refresh

> *Why*: `brew install` resolves the formula via the tap directly without needing `brew update --quiet`. Skipping it is faster and the distinction from `shll update` is the point — install and update are separate lifecycle operations.
> *Rejected*: running `brew update --quiet` for "freshness". `shll install` is not a brew metadata refresh tool — users who want a refresh have `brew update` directly, or `shll update` for the combined flow.

### #3 Best-effort across the roster

> *Why*: Mirrors `shll update`'s loop semantics (Constitution V — Graceful Degradation). One failed install (e.g. a tap-side transient error) shouldn't block the rest. The user gets exit 1 with a stderr line per failure and can retry.
> *Rejected*: abort-on-first-failure. Less useful, and inconsistent with `update`.

## Test seam

All `install_test.go` tests inject a fake via `proc.Runner` (`installFakeRunner` t.Cleanup helper, shared with `update_test.go`). No real brew subprocess is ever spawned.

Covered scenarios (`src/cmd/shll/install_test.go`):

- `TestInstall_BrewMissing` — `proc.Run("brew", "--version")` returns `ErrNotFound` → stderr hint, exit 1, no install attempted.
- `TestInstall_ShllFirstInformationalLine` *(bb7r)* — whole-roster run, all missing → the **first** stdout line is `shllSelfInstallNote` (`"shll — already present / self-managed"`), no `brew install` of the shll formula is ever recorded (informational only), and the line goes to stdout, never stderr.
- `TestInstall_AllAlreadyInstalled` — every `brew list` succeeds → stdout `shllSelfInstallNote + "\nAll shll tools already installed.\n"` (the bb7r informational line precedes the nothing-to-do note), no install calls, exit 0.
- `TestInstall_NoneInstalled` — every `brew list` exit-1 → install all six roster tools, exit 0.
- `TestInstall_PartialInstalled` — only `hop` and `wt` installed → install the other four, skip hop/wt, no stderr.
- `TestInstall_NoBrewUpdateInvoked` — pin the no-metadata-refresh contract: `brew update --quiet` MUST NOT appear in the recorded calls.
- `TestInstall_OneInstallFails` — one roster install (the `fab-kit` formula, now last in the leaves-first order) exits non-zero → loop continues and attempts all six, exit 1. The test pins the formula by name (`fab-kit`), not by roster position, and asserts total install attempts == `len(Roster)`, so it is robust to the reorder.
- `TestInstall_HeadersAndTail` — `hop`+`wt` installed; asserts the verbatim `[N/M]` headers over the missing subset (`==> [1/4] idea` … `==> [4/4] fab-kit`), the blank line before each subsequent header and before the tail, and the duration-bearing `Done — 4 of 4 tools succeeded in 1m12s.` tail (installs a deterministic clock).
- `TestInstall_EmptyCaseNoHeaderNoTail` — all installed → the shll-first informational line then the nothing-to-do note (`shllSelfInstallNote + "\nAll shll tools already installed.\n"`), no `==>` header and no `Done —`/duration tail.
- `TestInstall_PartialFailureTail` *(6vuo)* — all six missing, `fab-kit` fails → partial-failure tail `5 succeeded, 1 failed in 1m12s — see above.` (duration before the em-dash).
- `TestInstall_CounterPartialInstall` *(6vuo)* — only `idea` installed → missing subset `wt, tu, run-kit, hop, fab-kit` (5 tools, roster order) yields headers `[1/5]`..`[5/5]` and the `Done — 5 of 5 …` tail (counter correctness).
- `TestInstall_DryRunPreview` *(6vuo)* — `hop`+`wt` installed → verbatim aligned-column preview `Would install 4 tools:` then `brew install sahil87/tap/<formula>` rows; asserts no "metadata refresh" mention.
- `TestInstall_DryRunNoWrites` *(6vuo)* — `brew list` probe IS recorded; no `brew install` for any tool; zero `TransportForeground` calls.
- `TestInstall_DryRunEmptyCase` *(6vuo)* — all installed → dry-run mirrors the nothing-to-do message, no preview table, no install, exit 0.
- `TestInstall_SubsetUnknownTargetHardErrors` *(b2vg)* — `shll install <typo>` → `errSilent`, stderr lists valid targets, no `brew` subprocess runs.
- `TestInstall_SubsetShllRejected` *(b2vg)* — `shll install shll` → the unknown-target error (`shll` is not a valid install target).
- `TestInstall_SubsetArgOrderIndependentRosterOrder` *(b2vg)* — `shll install fab-kit wt` (both missing) → installs `wt` before `fab-kit` (roster order).
- `TestInstall_SubsetNamedAlreadyInstalled` *(b2vg)* — `shll install hop` when hop is already installed → the `All shll tools already installed.` nothing-to-do note, exit 0.
- `TestInstall_SubsetDryRunPreviewFiltered` *(b2vg)* — `shll install --dry-run` of a subset → preview lists only the named-and-missing subset in roster order, exit 0, no write.
- `TestInstall_TrustsEachFormulaBeforeInstall` *(0854)* — per-tool `brew trust --formula sahil87/tap/<formula>` precedes that tool's `brew install` (asserts `trustIdx < installIdx`), and the trust call is per-formula, never `--tap`.
- `TestInstall_NoTrustSkipsTrustStep` *(0854)* — `shll install --no-trust` → no `brew trust` invocation recorded, every missing tool still installed.
- `TestInstall_TrustUnavailableSkipsGracefully` *(0854)* — older brew (no `brew trust`) → the trust step is skipped silently, install proceeds, exit 0.
- `TestInstall_TrustFailureContinues` *(0854)* — a per-formula trust exits non-zero → a warning is written to stderr and `brew install` for that tool is still attempted; a trust failure alone does not flip the run to exit 1.

The shared resolver is unit-tested directly in `tools_test.go` (shared with `update` — see [cli/update test seam](/cli/update.md#test-seam)); `install` is the `allowShll=false` caller.

Per-tool header/tail behavior (y630) plus the change-6vuo `[N/M]` counter, duration, and install-preview helper are unit-tested against the `ui.go` helpers in `ui_test.go` (shared with `update`); `install_test.go` additionally asserts loop-path runs emit `==> [N/M] <tool>` headers and the plain tail to the **stdout** buffer (not stderr), and that the empty-case golden string is unchanged.

## Cross-references

- Subprocess wrapper conventions: [internal/proc](/internal/proc.md).
- The hardcoded roster: [cli/commands](/cli/commands.md#hardcoded-tool-roster).
- The shared `shllSelf` descriptor + the unified shll-first ordering (the informational line is install's instance): [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor). The sibling inspect surfaces that render shll as a full entry: [cli/list](/cli/list.md#the-prepended-shll-first-row) and [cli/doctor](/cli/doctor.md#the-prepended-shll-first-row).
- Sibling lifecycle command: [cli/update](/cli/update.md) — the upgrade-already-installed counterpart; the [per-tool header/tail contract](/cli/update.md#per-tool-output-separation) is documented there and shared via `ui.go`. `update` deliberately does NOT mutate trust (0854) — it relies on `install` having trusted the tools.
- **Counterpart lifecycle command: [cli/uninstall](/cli/uninstall.md)** (kkaj) — the install/uninstall pairing. `shll uninstall` is the clean-slate repair path that removes what `shll install` bootstraps: it mirrors install's per-tool `ui.go` framing and dry-run preview but in **reverse-roster** order (dependents before leaves), gates a destructive removal behind a `Proceed? [y/N]` confirmation, and reuses the shared brew helpers (`brew.go`) and `resolveTargets` (with `allowShll=true`, unlike install's `allowShll=false`, so `shll uninstall shll` is a legal explicit target).
- Trust helpers `brewTrustFormula`/`brewTrustAvailable` live in `brew.go`: [cli/commands §brew.go helper inventory](/cli/commands.md#file-layout-srccmdshll). The read-only sibling check that surfaces an installed-but-untrusted tool: [cli/doctor §the trust sub-check](/cli/doctor.md#the-trust-sub-check).
- **The read-only wiring detector the [post-install auto-run steps](#the-post-install-auto-run-steps-and-the-next-steps-block) gate on**: `resolveWiringFact` lives in `doctor.go` and is shared strictly read-only — [cli/doctor §the wiring fact](/cli/doctor.md#the-wiring-fact--resolvewiringfact-read-only-reuse), built on [cli/shell-setup §block location and parsing](/cli/shell-setup.md#block-location-and-parsing). When the gate reports unwired, install then drives shell-setup's write path (`runShellSetupDefault`) in-process.
- **The `shll agent-setup` command the agent step runs (and the nudge points at)**: [cli/agent-setup](/cli/agent-setup.md). The two-step skill discovery it wires: [cli/skill](/cli/skill.md).
- Shared UI helper (`ui.go`): [cli/commands](/cli/commands.md#file-layout-srccmdshll).
- **The upstream bootstrap that execs into this command: [ci/install-bootstrap](/ci/install-bootstrap.md)** (m1zt) — the `curl … | sh` script (`scripts/install.sh`, served at shll.ai/install) and its shll.ai raw-fetch URL contract.
- Constitution I (Security First — the trust ceremony routes through `internal/proc`), III (Wrap, Don't Reinvent), IV (Composition, Not Replacement), V (Graceful Degradation — trust degrades, not aborts, when `brew trust` is absent or fails), VII (Minimal Surface Area — `--no-trust` is a flag on existing `install`, no new command).
