---
type: memory
description: "`shll install` — brew detection, per-formula trust by default (`--no-trust` opt-out), bootstrap of missing roster tools via `brew install`, idempotent re-run; a legacy-keg run-kit is routed through the shared brew-direct migration action (trust-then-migrate) instead of a blind install. Also the `exec` target of the `curl … | sh` bootstrap (`scripts/install.sh`), whose arg pass-through is part of this command's public surface."
---
# cli/install

`shll install` — installs every roster tool that isn't already installed via Homebrew. Idempotent; safe to re-run.

Source: `src/cmd/shll/install.go`, with shared brew helpers in `src/cmd/shll/brew.go`.

## The `curl | sh` upstream entry point (change m1zt)

`shll install` is also the delegation target of the copy-paste install one-liner. The bootstrap script `scripts/install.sh` (served at `shll.ai/install`) requires Homebrew, trust-then-installs `shll` itself only if it is missing, then ends with `exec shll install "$@"` — forwarding every arg verbatim as the install subset:

```sh
curl -fsSL https://shll.ai/install | sh                # → exec shll install        (whole roster)
curl -fsSL https://shll.ai/install | sh -s -- hop wt   # → exec shll install hop wt  (subset)
```

Two implications for `shll install`'s contract:

- **The arg pass-through is now part of `shll install`'s public surface.** The [positional tool-name subset args](#positional-tool-name-args--subset-targeting-change-b2vg) are what a piped `sh -s -- <tools…>` reaches. The bootstrap adds no filtering of its own — it hands the args straight to `runInstall`, which validates them (`resolveTargets`, `allowShll=false`; unknown/`shll` targets still hard-error, the alias `rk` still resolves to `run-kit`).
- **The script owns only the shll-self bootstrap; `shll install` owns everything else.** Roster knowledge, subset filtering, per-formula trust for the other six tools, and graceful skips all live here, not in the script (Constitution III). The script's sole job is the circularity `shll install` cannot solve — trusting/installing `shll`'s own formula before that binary exists. See [ci/install-bootstrap](/ci/install-bootstrap.md) for the script contract, the shll.ai raw-fetch URL / merge-order constraint, and the dev-script rename to `scripts/install-local.sh`.

## Behavior contract

The full happy/unhappy paths, in the order `runInstall` evaluates them (`src/cmd/shll/install.go`):

1. **Brew missing.** If `hasBrew(ctx)` returns false, write `installBrewMissingHint` to stderr and return `errSilent`. Exit code: 1. The literal hint is `"shll install requires Homebrew. Install from https://brew.sh"` (`src/cmd/shll/brew.go`). It is intentionally separate from `brewMissingHint` (used by `shll update`) so each command's error tells the user which command they ran — the update spec scenario asserts its verbatim text, so reusing the same constant for both commands would either violate that lock or mislead `shll install` users. (Subset resolution via `resolveTargets(args, false)` runs *before* this guard — an unknown/`shll` target errors first, with no brew side effect; see [Positional tool-name args](#positional-tool-name-args--subset-targeting-change-b2vg).)

   **Then — shll-first informational line** (change bb7r): immediately after this brew-missing guard passes, `runInstall` writes `shllSelfInstallNote` (`"shll — already present / self-managed"`) to stdout. Placed *after* the brew-missing/unknown-target guards, so it leads the nothing-to-do, dry-run, and install-loop paths but **not** the early-error paths. Informational only — shll is never a `brew install` target. See [The prepended shll-first informational line](#the-prepended-shll-first-informational-line-change-bb7r).

2. **Partition the roster into a `[]installTarget`.** Iterate the roster in order. For a tool with **no `LegacyFormula`**, call `isInstalled(ctx, t.Formula)` and collect the missing entries as `installTarget{tool: t}` (a plain `brew install`). For a **`LegacyFormula`-bearing tool (run-kit)**, classify via the SHARED migration gate `probeRunKitMigration` (the same detection `shll update`/`shll doctor` use): a legacy keg → `installTarget{tool, migrate: true}` (brew-direct migration, not a blind install); a fully-absent run-kit → `installTarget{tool}` (normal install); a migrated/present run-kit → skipped (idempotent). See [Legacy-keg routing to migration](#legacy-keg-routing-to-migration-change-9bak).

3. **Nothing missing → short-circuit.** If `len(missing) == 0`, write `All sahil87 tools already installed.` to stdout and return nil. Exit code: 0. No `brew update` is invoked — there's nothing to install.

4. **No `brew update --quiet`.** Unlike `shll update`, `shll install` does NOT refresh brew metadata first. `brew install sahil87/tap/<formula>` resolves the formula via the tap directly, and the spec freezes this distinction (Design Decision: install ≠ update). `TestInstall_NoBrewUpdateInvoked` pins the contract.

5. **Sequential per-tool install — trust then install/migrate (change 0854; migration routing change 9bak).** For each `installTarget` in roster order, print its per-tool header (see [Per-tool output separation](#per-tool-output-separation-change-y630)), then — when trust is enabled — record per-formula trust via `brewTrustFormula(ctx, t.Formula)` *immediately before* the action. For a plain target the action is `proc.RunForeground(ctx, brewBinary, "install", t.Formula)`; for a **`migrate` target** it is the brew-direct `migrateRunKit(ctx, stdout, stderr, t)` (reused verbatim from `shll update`), NOT a blind `brew install`. The trust step is interleaved in the per-tool loop (not a separate up-front pass), so trust stays adjacent to the action it unblocks — and it trusts the **new** formula (`sahil87/tap/run-kit`) even on the migration route (installed ≠ trusted). Best-effort across the roster: on per-tool *install/migrate* failure (transport error or non-zero exit), set `anyFailed = true` and `continue`. See [Per-formula trust before install](#per-formula-trust-before-install-change-0854) and [Legacy-keg routing to migration](#legacy-keg-routing-to-migration-change-9bak).

6. **Summary tail.** After the loop, print one summary line via `printSummaryTail` (see [Per-tool output separation](#per-tool-output-separation-change-y630)), then — unchanged — if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0). The tail is presentation-only and does not change the exit code.

## The prepended shll-first informational line (change bb7r)

`runInstall` prepends a single shll-first line to stdout — `fmt.Fprintln(stdout, shllSelfInstallNote)` (`src/cmd/shll/install.go:94`) — so the toolkit reads as one family with `shll` as its manager-member (the discoverability goal shared with `list`/`doctor`). It is the install-side instance of the unified shll-first ordering — see [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor-change-bb7r).

```go
// install.go
const shllSelfInstallNote = "shll — already present / self-managed"
```

Two load-bearing properties:

- **Never a brew install action on the running binary.** You cannot `brew install` the running orchestrator, so the line is **informational only** — no subprocess, no `brew install sahil87/tap/shll`. shll is also rejected as an explicit positional install target (`resolveTargets(args, false)`, `allowShll=false`; change b2vg), so it can never enter the `missing` set. `TestInstall_ShllFirstInformationalLine` asserts no `brew install` of the shll formula is ever recorded.
- **Placement: after the guards, before the roster framing.** The line is written *after* the brew-missing guard (and after the up-front `resolveTargets` unknown-target check) but *before* the roster is partitioned. So it leads the three terminal paths that reach the install decision — **nothing-to-do** (`All sahil87 tools already installed.`), **`--dry-run` preview**, and the **install loop** — but is **NOT** emitted on the early-error paths (brew missing → only the stderr hint; unknown/`shll` target → only the stderr error). It goes to **stdout**, never stderr (`TestInstall_ShllFirstInformationalLine` also asserts this).

This is a deliberate *informational* exception to the symmetry between the inspect surface (`list`/`doctor`, which render shll as a full row/object) and `install` (which *acts*): shll cannot be acted on, so its representation here is a leading note rather than an actionable row.

> **Note — the empty/nothing-to-do golden is no longer just `allInstalledMsg`.** Before change bb7r, the all-already-installed stdout was exactly `All sahil87 tools already installed.\n`. With the prepended informational line, that path's stdout is now `shll — already present / self-managed\n` then `All sahil87 tools already installed.\n`. The [Per-tool output separation §empty case](#per-tool-output-separation-change-y630) statement that the empty-case stdout is "**exactly** `allInstalledMsg`" holds for the install-loop framing only (no `==>` header, no tail, no blank lines); the bb7r informational line precedes it on every non-early-error path.

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All installs succeeded (or all-already-installed branch) | 0 |
| Unknown/typo'd positional target — incl. `shll`, which is rejected (change b2vg) | 1 (via `errSilent`, before any brew work) |
| `brew` not on PATH | 1 (via `errSilent`, hint already on stderr) |
| Any per-tool `brew install` failed | 1 (via `errSilent`, after all missing tools attempted) |

## Per-tool output separation (change y630)

`shll install` mirrors `shll update`'s framing exactly, via the same shared helper `src/cmd/shll/ui.go` (see [cli/commands](/cli/commands.md#file-layout-srccmdshll)) — no TTY/`NO_COLOR`/glyph logic is duplicated in `install.go`.

- **Per-tool header with `[N/M]` progress counter (change 6vuo; header color form updated by change 13k3).** Before each missing tool's `brew install` output, `printToolHeader(stdout, t.Name, i+1, total, color)` (`install.go`) writes `▸ [N/M] <tool>` (color TTY — since change 13k3 the whole `▸ [N/M] <tool>` run is one bold-cyan span, mirroring update via the shared `printToolHeader`) / `==> [N/M] <tool>` (plain, byte-identical), in roster order, where `N` is the 1-based loop position and `M = len(missing)` — already known up front, so no separate denominator computation is needed (unlike `update`, where `M` is derived from the probe results). Since change auvj the roster is leaves-first (`wt, idea, tu, run-kit, hop, fab-kit`), so the headers for the *missing subset* print in that relative order — e.g. with `hop`+`wt` already installed, the missing set `{idea, tu, run-kit, fab-kit}` yields `==> [1/4] idea`, `==> [2/4] tu`, `==> [3/4] run-kit`, `==> [4/4] fab-kit` (`TestInstall_HeadersAndTail` golden, with the `Done — 4 of 4 tools succeeded in 1m12s.` tail). See the [leaves-first ordering rationale](/cli/commands.md#design-decision-leaves-first-roster-order-change-auvj).
- **Section spacing (change 6vuo).** A single blank line precedes each per-tool header **except the first** (`install.go:106`, `if i > 0`), and a single blank line precedes the summary tail (`install.go:128`) — so each tool's streamed output is separated from the next header and the tail. The all-already-installed short-circuit emits no blank lines.
- **Summary tail with run duration (change 6vuo).** After the loop, `printSummaryTail(stdout, succeeded, total, elapsed, color)` (`install.go:129`, `total = len(missing)`) writes `Done — N of M tools succeeded in <dur>.` (green `✓` when color) or `X succeeded, Y failed in <dur> — see above.` (duration before the em-dash), by **exit code only** — `succeeded` counts installs that exited 0, mirroring the same per-tool facts that drive `anyFailed`. The duration is a run fact, not an outcome claim — the tail still never claims "installed" vs. "up-to-date" (the honesty constraint). Presentation-only; does not change the exit code. Elapsed is measured via the injectable `nowFunc` clock seam (`clock.go`), captured at `install.go:101` **after** the short-circuit and the dry-run branch return, so it covers only the install phase.
- **Stream discipline.** Header and tail go to **stdout** (the stream `brew install` is foregrounded onto), never stderr.
- **Color gating.** One `colorEnabled(stdout)` decision (TTY via `golang.org/x/term` AND `NO_COLOR` unset), reused for headers and tail; `bytes.Buffer` test writers hit the plain-ASCII branch.
- **Empty case emits no header, no tail, no counter, no spacing, no duration.** The all-already-installed short-circuit (step 3) runs no loop, so the *install-loop framing* it would emit is absent — no `==> [N/M]` header, no tail, no blank lines, no duration; only the install-loop path carries those markers. Its install-message line stays `All sahil87 tools already installed.\n` (the `allInstalledMsg` constant). **Since change bb7r the shll-first informational line precedes it** (`shll — already present / self-managed\n` then `All sahil87 tools already installed.\n`) on this non-early-error path — see [The prepended shll-first informational line](#the-prepended-shll-first-informational-line-change-bb7r); the `TestInstall_AllAlreadyInstalled`/`TestInstall_EmptyCaseNoHeaderNoTail` goldens were updated for the prepended line.

The helper details (named SGR constants, the `colorEnabled` gating, the honesty constraint on the tail, the `[N/M]` counter, the `formatDuration` form, and the `nowFunc` clock seam) are documented once under [cli/update](/cli/update.md#per-tool-output-separation-change-y630); `install` consumes the identical helpers.

## Per-formula trust before install (change 0854)

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

> **The 38a6 Linux sandbox-trust workaround is REMOVED (change 0854, closes backlog `[tkch]`).** The temporary `brewEnv()` / `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` injection on `brew install` is gone — the upstream Homebrew bug it worked around is fixed in 6.0.4, and the per-formula trust above is the correct DX. The brew install call site is now plain `proc.RunForeground(ctx, brewBinary, "install", t.Formula)` (no env). The change requires Homebrew ≥ 6.0.4; the floor is documented in the README, not gated in code. See [cli/update §removal of the 38a6 workaround](/cli/update.md#removal-of-the-38a6-linux-workaround-change-0854) for the same removal on update/upgrade, and [internal/proc](/internal/proc.md) (the `Env`/`RunForegroundEnv` plumbing was reverted).

Tests (`install_test.go`): `TestInstall_TrustsEachFormulaBeforeInstall` (per-tool trust precedes the install, and is per-formula — never `--tap`), `TestInstall_NoTrustSkipsTrustStep` (`--no-trust` → no `brew trust` calls), `TestInstall_TrustUnavailableSkipsGracefully` (older brew → no trust calls, install proceeds, exit 0), `TestInstall_TrustFailureContinues` (trust non-zero → warning, install still attempted, exit reflects install only). The removed-workaround tests `TestInstall_BrewInstallCarriesWorkaroundEnvOnLinux` / `TestInstall_BrewInstallNoWorkaroundEnvOnDarwin` are gone.

## Legacy-keg routing to migration (change 9bak)

A blind `brew install sahil87/tap/run-kit` on a machine still holding the legacy `rk` keg is unsafe — brew may or may not auto-migrate on install, and the observed dual-rack state C suggests exactly that kind of sequence created it. So `shll install` reuses `shll update`'s migration gate + action for run-kit: the missing-partition classifies the run-kit entry via the shared `probeRunKitMigration` gate and marks a legacy keg `migrate: true` on its `installTarget`; the loop then runs the brew-direct `migrateRunKit` (`brew upgrade sahil87/tap/rk` + conditional `brew link run-kit` + printed daemon/dual-rack notes) instead of `brew install`.

- **Three outcomes for run-kit** (from the shared gate — one detection path, Constitution III): legacy keg → `migrate:true`; fully absent → plain `brew install sahil87/tap/run-kit`; migrated/present → skipped (idempotent, like any already-installed tool). See [cli/update §the migration gate + action](/cli/update.md#the-rkrun-kit-migration-guard-change-9bak) for the classification and `migrateRunKit` steps.
- **Trust-then-migrate, not just trust-then-install.** The migration route does NOT skip the per-formula trust step: **installed ≠ trusted** (that inequality is doctor's trust-WARN premise, change 0854), and the legacy keg being present does not imply the *renamed* formula carries a trust record. When `trustEnabled`, `shll install` trusts the **new** formula `sahil87/tap/run-kit` FIRST, then migrates — because `brew upgrade sahil87/tap/rk` resolves the rename to `run-kit`'s sandboxed `def install`, which Homebrew 6.0+ refuses without a real trust record for the formula it lands on. A trust failure warns and continues (the migration's own exit code is the authority), matching the trust-then-act contract of the plain install path.
- **`shll install rk` alias.** Routes through the same `legacyAliases`/`printAliasNotices` path as `update` (prints `note: rk is now run-kit`), then classifies + migrates/installs the canonical run-kit tool. `allowShll=false` still holds — `shll` is never an install target.
- **The `installTarget{tool, migrate}` struct** is the single classification the install loop and the dry-run preview share, so a migrate target previews `brew upgrade sahil87/tap/rk` (via the same `upgradeArgv(m.tool, false, true)` the live migration's first step uses) rather than `brew install`.

Tests (`install_test.go`): `TestInstall_LegacyKegRoutesThroughMigration` (legacy keg → `brew upgrade sahil87/tap/rk`, NOT `brew install`), `TestInstall_MigrationTrustsRunKitFormulaFirst` (trust of `sahil87/tap/run-kit` precedes the migration), `TestInstall_MigrationNoTrustSkipsTrustStep` (`--no-trust` → no trust call on the migration route), `TestInstall_AbsentRunKitStillBrewInstalls` (fully-absent run-kit → plain `brew install sahil87/tap/run-kit`), `TestInstall_LegacyAliasResolvesWithNotice` (`shll install rk` → alias notice + canonical resolution).

## `--dry-run` (change 6vuo)

`shll install --dry-run` previews the `brew install` commands the run **would** execute, then exits 0 **without any write**. It mirrors `shll update --dry-run` (see [cli/update](/cli/update.md#dry-run-change-6vuo) for the shared contract); the flag, usage string, and the `dryRun bool` parameter on `runInstall` are the same `dryRunFlag`/`dryRunFlagUsage` constants (defined in `update.go`, shared across both commands).

**Reads run; writes do not.** The `isInstalled` probes (`brew list --formula --versions`) that partition the roster still run in dry-run (they are reads, and the preview depends on them) — but **no `brew install`** is performed. The guarantee is structural: the dry-run branch (`install.go:80`) returns before the install loop and before `start := nowFunc()`. `TestInstall_DryRunNoWrites` asserts the `brew list` probe IS recorded, no `brew install <formula>` runs for any tool, and there are **zero `TransportForeground`** calls.

**The preview.** Preceded by the shll-first informational line (change bb7r — the dry-run path reaches the install decision, so it leads with `shllSelfInstallNote`), then a header line `Would install N tools:` (`installPreviewHeaderFmt`) — **no metadata-refresh annotation**, since `install` runs no `brew update` (consistent with [Design Decision #2](#2-no-metadata-refresh)) — then one aligned row per missing tool, in roster order, each reading `brew install sahil87/tap/<formula>` (built as `argvString(brewBinary, "install", t.Formula)`). Formatting reuses the same `ui.go` `printInstallPreview` → `printPreviewRows` aligned-column layout as `update`: 2-space indent, labels left-padded to the longest *missing* label present, 2-space gap before the command. No `[N/M]` counter, no blank-line spacing (the preview is a static table).

```
Would install 4 tools:
  idea     brew install sahil87/tap/idea
  tu       brew install sahil87/tap/tu
  run-kit  brew install sahil87/tap/run-kit
  fab-kit  brew install sahil87/tap/fab-kit
```

(`TestInstall_DryRunPreview` golden — `hop`+`wt` installed, the other four missing; run-kit here is fully absent so it previews the plain `brew install` — a legacy-keg run-kit would instead preview `brew upgrade sahil87/tap/rk`. The longest missing label `fab-kit`/`run-kit` (7) sets the column width. The test also asserts the preview does NOT mention "metadata refresh".)

**Graceful degradation (Constitution V).** Only the missing subset is listed; already-installed tools are omitted (they are filtered out into `missing` before the preview builds).

**Empty case.** When every roster tool is already installed, the dry-run path never reaches the preview builder — the shared all-already-installed short-circuit (step 3) fires first, so stdout is the shll-first informational line then `All sahil87 tools already installed.\n` (i.e. `shllSelfInstallNote + "\n" + allInstalledMsg + "\n"`, change bb7r), exit 0, no preview table, no install (`TestInstall_DryRunEmptyCase`).

**Brew-missing precondition unchanged.** A missing brew still writes `installBrewMissingHint` to stderr and exits 1 (the `hasBrew` check precedes the dry-run branch).

## Positional tool-name args — subset targeting (change b2vg)

`shll install [tool...]` accepts zero or more positional tool-name args (`Args: cobra.ArbitraryArgs`, parsed args threaded into `runInstall`), symmetric with [`shll update`](/cli/update.md#positional-tool-name-args--subset-targeting-change-b2vg) for the install lifecycle. The shared resolver is single-sourced with `Roster`; install differs from update in exactly one way — the valid-target set.

- **Zero args → whole-roster run, unchanged.** `subset := len(args) > 0` is false; the partition/install behavior above holds verbatim.
- **One or more args → operate on just the named subset.** The args form a *set*, not a sequence.

**Valid targets for `install` are the six `Roster` tools ONLY** (`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit`; the legacy alias `rk` also resolves to `run-kit` — change 9bak). **`shll` is NOT a valid install target** — you cannot `brew install` the running orchestrator. `runInstall` calls `resolveTargets(args, false)` (`allowShll=false`), so `shll install shll` falls into the unknown-target error path (`shll install: unknown target "shll" (valid targets: wt, idea, tu, run-kit, hop, fab-kit)`) — note `shll` is absent from the valid list (it appears only for `update`, where `allowShll=true`), and `rk` is absent too (accepted as an alias but never advertised).

**Roster-order processing.** A subset is processed in `Roster` (leaves-first) order regardless of arg order — `resolveTargets` returns the selected `Tool`s in roster order, and `runInstall` walks `consider = selected` (else the full `Roster`) to build `missing`, preserving that order. Example: `shll install fab-kit wt` installs `wt` then `fab-kit`. (Why leaves-first is output coherence, not correctness: [leaves-first ordering rationale](/cli/commands.md#design-decision-leaves-first-roster-order-change-auvj).)

**Validation up front (`runInstall` resolves the subset before `hasBrew` and any probe).** An unknown / typo'd name → `resolveTargets` returns a non-nil error; `runInstall` writes `shll install: <detail>` to stderr and returns `errSilent` (exit 1) with **no brew side effect**. All unknown args are reported at once.

**Named-already-installed → the existing nothing-to-do path.** For `install`, "not installed" is the happy path. The inverse edge — a tool named explicitly that is *already* installed — is **not** an error: it is filtered out into the (empty-for-it) `missing` set, exactly like the whole-roster idempotent skip. If every named target is already installed, the run hits the existing short-circuit and prints `All sahil87 tools already installed.` (exit 0). (Contrast `update`, where a named-but-not-installed target *is* an error — the asymmetry follows from the inverted precondition: install acts on absent tools, update acts on present ones.)

**Counter denominator `M` = subset size.** `M = len(missing)`, where `missing` is now restricted to the named-and-missing subset, so the per-tool `[N/M]` header and the summary-tail `M` reflect the subset, not the whole roster. The [per-tool output separation](#per-tool-output-separation-change-y630) contract is otherwise unchanged.

**`--dry-run` previews the filtered subset.** The dry-run branch runs after `missing` is built from the subset, so it previews only the named-and-missing tools in roster order, header `Would install N tools:` with `N` = subset size.

## Constitution VII justification

> *Why a new top-level subcommand?* `install` is a distinct lifecycle operation from `update`: different precondition (tool not installed vs. installed), different failure modes (no metadata-refresh dependency), and different discoverability (a new user wanting "get me the toolkit" looks for `install`). Cannot be cleanly expressed as a flag on `update` because `update`'s installed-only precondition would have to invert for a subset of the run.
>
> *Rejected*: `shll update --install-missing`. The branching gets messy and the verb mismatch hurts new-user discoverability.

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
- `TestInstall_ShllFirstInformationalLine` *(change bb7r)* — whole-roster run, all missing → the **first** stdout line is `shllSelfInstallNote` (`"shll — already present / self-managed"`), no `brew install` of the shll formula is ever recorded (informational only), and the line goes to stdout, never stderr.
- `TestInstall_AllAlreadyInstalled` — every `brew list` succeeds → stdout `shllSelfInstallNote + "\nAll sahil87 tools already installed.\n"` (the bb7r informational line precedes the nothing-to-do note), no install calls, exit 0.
- `TestInstall_NoneInstalled` — every `brew list` exit-1 → install all six roster tools, exit 0.
- `TestInstall_PartialInstalled` — only `hop` and `wt` installed → install the other four, skip hop/wt, no stderr.
- `TestInstall_NoBrewUpdateInvoked` — pin the no-metadata-refresh contract: `brew update --quiet` MUST NOT appear in the recorded calls.
- `TestInstall_OneInstallFails` — one roster install (the `fab-kit` formula, now last in the leaves-first order) exits non-zero → loop continues and attempts all six, exit 1. The test pins the formula by name (`fab-kit`), not by roster position, and asserts total install attempts == `len(Roster)`, so it is robust to the reorder.
- `TestInstall_HeadersAndTail` *(change 6vuo, golden updated)* — `hop`+`wt` installed; asserts the verbatim `[N/M]` headers over the missing subset (`==> [1/4] idea` … `==> [4/4] fab-kit`), the blank line before each subsequent header and before the tail, and the duration-bearing `Done — 4 of 4 tools succeeded in 1m12s.` tail (installs a deterministic clock).
- `TestInstall_EmptyCaseNoHeaderNoTail` *(change 6vuo; golden updated by bb7r)* — all installed → the shll-first informational line then the nothing-to-do note (`shllSelfInstallNote + "\nAll sahil87 tools already installed.\n"`), no `==>` header and no `Done —`/duration tail.
- `TestInstall_PartialFailureTail` *(change 6vuo)* — all six missing, `fab-kit` fails → partial-failure tail `5 succeeded, 1 failed in 1m12s — see above.` (duration before the em-dash).
- `TestInstall_CounterPartialInstall` *(change 6vuo)* — only `idea` installed → missing subset `wt, tu, run-kit, hop, fab-kit` (5 tools, roster order) yields headers `[1/5]`..`[5/5]` and the `Done — 5 of 5 …` tail (counter correctness).
- `TestInstall_DryRunPreview` *(change 6vuo)* — `hop`+`wt` installed → verbatim aligned-column preview `Would install 4 tools:` then `brew install sahil87/tap/<formula>` rows; asserts no "metadata refresh" mention.
- `TestInstall_DryRunNoWrites` *(change 6vuo)* — `brew list` probe IS recorded; no `brew install` for any tool; zero `TransportForeground` calls.
- `TestInstall_DryRunEmptyCase` *(change 6vuo)* — all installed → dry-run mirrors the nothing-to-do message, no preview table, no install, exit 0.
- `TestInstall_SubsetUnknownTargetHardErrors` *(change b2vg)* — `shll install <typo>` → `errSilent`, stderr lists valid targets, no `brew` subprocess runs.
- `TestInstall_SubsetShllRejected` *(change b2vg)* — `shll install shll` → the unknown-target error (`shll` is not a valid install target).
- `TestInstall_SubsetArgOrderIndependentRosterOrder` *(change b2vg)* — `shll install fab-kit wt` (both missing) → installs `wt` before `fab-kit` (roster order).
- `TestInstall_SubsetNamedAlreadyInstalled` *(change b2vg)* — `shll install hop` when hop is already installed → the `All sahil87 tools already installed.` nothing-to-do note, exit 0.
- `TestInstall_SubsetDryRunPreviewFiltered` *(change b2vg)* — `shll install --dry-run` of a subset → preview lists only the named-and-missing subset in roster order, exit 0, no write.
- `TestInstall_TrustsEachFormulaBeforeInstall` *(change 0854)* — per-tool `brew trust --formula sahil87/tap/<formula>` precedes that tool's `brew install` (asserts `trustIdx < installIdx`), and the trust call is per-formula, never `--tap`.
- `TestInstall_NoTrustSkipsTrustStep` *(change 0854)* — `shll install --no-trust` → no `brew trust` invocation recorded, every missing tool still installed.
- `TestInstall_TrustUnavailableSkipsGracefully` *(change 0854)* — older brew (no `brew trust`) → the trust step is skipped silently, install proceeds, exit 0.
- `TestInstall_TrustFailureContinues` *(change 0854)* — a per-formula trust exits non-zero → a warning is written to stderr and `brew install` for that tool is still attempted; a trust failure alone does not flip the run to exit 1.

The shared resolver is unit-tested directly in `tools_test.go` (shared with `update` — see [cli/update test seam](/cli/update.md#test-seam)); `install` is the `allowShll=false` caller.

Per-tool header/tail behavior (change y630) plus the change-6vuo `[N/M]` counter, duration, and install-preview helper are unit-tested against the `ui.go` helpers in `ui_test.go` (shared with `update`); `install_test.go` additionally asserts loop-path runs emit `==> [N/M] <tool>` headers and the plain tail to the **stdout** buffer (not stderr), and that the empty-case golden string is unchanged.

## Cross-references

- Subprocess wrapper conventions: [internal/proc](/internal/proc.md).
- The hardcoded roster: [cli/commands](/cli/commands.md#hardcoded-tool-roster).
- The shared `shllSelf` descriptor + the unified shll-first ordering (the informational line is install's instance): [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor-change-bb7r). The sibling inspect surfaces that render shll as a full entry: [cli/list](/cli/list.md#the-prepended-shll-first-row-change-bb7r) and [cli/doctor](/cli/doctor.md#the-prepended-shll-first-row-change-bb7r).
- Sibling lifecycle command: [cli/update](/cli/update.md) — the upgrade-already-installed counterpart; the [per-tool header/tail contract](/cli/update.md#per-tool-output-separation-change-y630) is documented there and shared via `ui.go`. `update` deliberately does NOT mutate trust (change 0854) — it relies on `install` having trusted the tools. The rk→run-kit migration gate + `migrateRunKit` action `install` reuses (change 9bak) live in [cli/update §the migration guard](/cli/update.md#the-rkrun-kit-migration-guard-change-9bak).
- **Counterpart lifecycle command: [cli/uninstall](/cli/uninstall.md)** (change kkaj) — the install/uninstall pairing. `shll uninstall` is the clean-slate repair path that removes what `shll install` bootstraps: it mirrors install's per-tool `ui.go` framing and dry-run preview but in **reverse-roster** order (dependents before leaves), gates a destructive removal behind a `Proceed? [y/N]` confirmation, and reuses the shared brew helpers (`brew.go`) and `resolveTargets` (with `allowShll=true`, unlike install's `allowShll=false`, so `shll uninstall shll` is a legal explicit target).
- Trust helpers `brewTrustFormula`/`brewTrustAvailable` live in `brew.go`: [cli/commands §brew.go helper inventory](/cli/commands.md#file-layout-srccmdshll). The read-only sibling check that surfaces an installed-but-untrusted tool: [cli/doctor §the trust sub-check](/cli/doctor.md#the-trust-sub-check-change-0854).
- Shared UI helper (`ui.go`): [cli/commands](/cli/commands.md#file-layout-srccmdshll).
- **The upstream bootstrap that execs into this command: [ci/install-bootstrap](/ci/install-bootstrap.md)** (change m1zt) — the `curl … | sh` script (`scripts/install.sh`, served at shll.ai/install), its shll.ai raw-fetch URL + merge-order contract, and the dev-script rename to `scripts/install-local.sh`.
- Constitution I (Security First — the trust ceremony routes through `internal/proc`), III (Wrap, Don't Reinvent), IV (Composition, Not Replacement), V (Graceful Degradation — trust degrades, not aborts, when `brew trust` is absent or fails), VII (Minimal Surface Area — `--no-trust` is a flag on existing `install`, no new command).
