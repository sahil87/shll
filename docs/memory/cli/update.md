---
type: memory
description: "`shll update` — brew detection, probe-spec installed-detection for delegated (non-brew) roster entries, sequential delegated upgrades with brew-managed-only self-heals (relink, brew-upgrade fallback), exit-code aggregation, the `What changed:` release digest, placement-gated agent-skill refresh with `--yes`/`-y` forwarding, a linear streamed write phase with null-stdin streamed-tail children, determinate OSC 9;4 progress on TTY stderr (tmux-wrapped), and the `rk` legacy alias."
---
# cli/update

`shll update` — refreshes brew metadata once, self-upgrades `shll`, then upgrades every installed shll tool by **delegating to that tool's own `update` subcommand** (falling back to `brew upgrade` only for a brew-managed tool that exposes no `update`; a delegated (non-brew) tool's `Update` argv IS its update path — there is no brew fallback for it).

Source: `src/cmd/shll/update.go`, with shared brew helpers in `src/cmd/shll/brew.go`.

> **Delegation, not `brew upgrade`.** `shll update` delegates to `<tool> update` so each tool stays authoritative over its own upgrade + side effects — e.g. `run-kit update`'s daemon restart, which lives in run-kit's CLI rather than a brew post-install hook (Constitution IV — Composition, Not Replacement; Constitution III — Wrap, Don't Reinvent). Rationale: [the delegate Design Decision](#delegate-to-tool-update-not-brew-upgrade-formula) below. (cczs)
>
> **run-kit is a plain roster tool.** Every roster tool — run-kit included — takes the single-probe delegated-upgrade path. A never-migrated machine (legacy `rk` keg only) probes `sahil87/tap/run-kit`, finds it not installed, and skips run-kit gracefully (Constitution V); straggler migration is the manual README path (`brew uninstall rk`, then `shll install`), not a shll code path.
>
> **rk-desktop is the roster's delegated (non-brew) entry.** It carries no `Formula`: its install probe runs the `Probe` spec (`rk desktop status`, parsing the `Installed:` line — `not installed` = absent, any other value = installed and IS the version) instead of `brew list --versions`, and its upgrade streams the `Update` argv verbatim (`rk desktop update`) with no `--skip-brew-update` help probe, no unlinked-keg relink heal, and no brew-upgrade fallback. It is skipped when `rk` is absent or the platform is unsupported (a probe error reads as not-installed). Rationale: [the delegated-entry Design Decision](#a-delegated-tools-update-is-its-update-argv-verbatim--no-brew-safety-net) below. (t26g)

## Behavior contract

The full happy/unhappy paths, in the order `runUpdate` evaluates them (`src/cmd/shll/update.go`):

1. **Brew missing.** If `hasBrew(ctx)` returns false, write `shll update requires Homebrew. Install from https://brew.sh` to stderr and return `errSilent`. Exit code: 1. The literal hint string is `brewMissingHint` in `src/cmd/shll/brew.go` — do not edit one without the other (the spec scenario asserts it verbatim). The status line (step 2) is NOT printed before this bail-out — brew presence is checked first (`TestUpdate_BrewMissing` asserts empty stdout).

2. **Instant status line.** Write `Checking installed shll tools…` to stdout (named constant `updateStatusLine`, `src/cmd/shll/update.go`). This is the first visible byte, printed **unconditionally** before any probing — including before the nothing-to-do short-circuit — so the user gets immediate feedback during the (now concurrent) probe phase rather than staring at a blank terminal.

3. **Parallel read-only capability probes.** `probeRoster(ctx)` dispatches one goroutine per roster tool and joins on a `sync.WaitGroup`; each goroutine runs `probeTool` and writes its result into a fixed-size `[]probeResult` slice **indexed by roster position** so results stay in roster order regardless of completion order. Per tool, `probeTool` determines these facts:
   - **Installed? + before-version** — a single `probeToolInstalledVersion(ctx, t)` read (`src/cmd/shll/version.go`) yields the install fact AND the installed version (`probeResult.beforeVersion`), branching on `t.brewManaged()`: brew-managed tools take the `brew list --formula --versions` read (`probeInstalledVersion(ctx, t.Formula)`), delegated (non-brew) tools take their `Probe` spec (`rk desktop status` → the `Installed:` line, via `parseProbeStatusLine`; a transport error, non-zero exit — including an unsupported-platform refusal — or the absent value all read as not-installed). This is the captured read the probe already pays for — never streamed foreground output (code-quality.md). `beforeVersion` is `""` when not installed or unparseable (suppresses only this tool's digest entry, never its upgrade). See also [version capture](#version-capture--the-what-changed-digest).
   - **Supports `--skip-brew-update`?** — only for installed **brew-managed** tools that have a non-empty `Update` argv: `toolSupportsSkipFlag` runs `<tool> update --help` via `proc.Run` (capture) and checks whether the output contains the literal substring `--skip-brew-update` (`strings.Contains`, never a regex — code-quality.md anti-pattern). A probe transport error is treated as "not supported" → graceful degradation to a plain `<tool> update`. A delegated (non-brew) tool is never help-probed — the flag exists to skip a tool's internal brew refresh, which a non-brew update never runs (`rk desktop update` downloads a DMG, not brew).

4. **Detect shll-self brew install + before-version.** A single `probeInstalledVersion(ctx, shllFormula)` (`shllFormula = "sahil87/tap/shll"`, `src/cmd/shll/brew.go`) yields BOTH `shllInstalled` (drives whether the self-upgrade step in (7) runs) AND `beforeShll` (shll's pre-upgrade **brew-formula** version, feeding the digest — not the running process's ldflags version). (This single probe runs after `probeRoster`, not inside it — shll is intentionally not in `Roster`.)

5. **Nothing-to-do → short-circuit.** If no roster tool is installed AND shll itself is not brew-installed, write `No shll tools installed.` to stdout and return nil. Exit code: 0. Critically, **`brew update --quiet` is NOT invoked in this branch** — see Design Decision #9 below. Because the status line (step 2) already printed, the empty-case stdout reads exactly `Checking installed shll tools…\nNo shll tools installed.\n` (`TestUpdate_NoToolsInstalled`). When shll itself is brew-installed but no roster tools are, the short-circuit does NOT fire — the run proceeds and only self-upgrades shll (`TestUpdate_OnlyShllInstalled`).

6. **Refresh metadata once.** `runChild(brewBinary, "update", "--quiet")` (`update.go`) — routed through the shared streamed-tail helper (null stdin, live tee — see [Null-stdin streamed children](#null-stdin-streamed-children)), so users see brew's progress as it runs. Run exactly **once** per invocation, after probing and before any upgrade. Because each delegated `<tool> update --skip-brew-update` skips its own internal `brew update`, this is the only metadata refresh for the whole run (vs. N redundant refreshes if each tool refreshed independently — Design Decision #2 of this change). The streamed transport returns `(code, nil)` on a non-zero subprocess exit and `(_, err)` only on an exec/transport failure, so the branch checks **both** `code != 0` and `err != nil`. On failure, write `shll update: brew update failed: <detail>` to stderr and return `errSilent` (exit 1) — no upgrades attempted.

7. **shll self-upgrade (when brew-installed).** If step (4) reported shll itself as brew-installed, print the `shll (self)` per-tool header (see [Per-tool output separation](#per-tool-output-separation)) then run `brew upgrade sahil87/tap/shll` via the streamed-tail helper (`runChild(brewBinary, "upgrade", shllFormula)`) *before* the roster loop. shll has no `update` subcommand to call on itself, so this stays a direct `brew upgrade` (not delegated). See [shll self-upgrade](#shll-self-upgrade) for rationale and edge cases. Failures here go through the same best-effort `anyFailed` path as roster failures, and contribute to the `total`/`succeeded` counts feeding the summary tail. **On success**, `shll update` re-queries `installedVersion(ctx, shllFormula)` (a cheap captured read) and records a bump against `beforeShll` if it changed — the digest input (r01z).

8. **Sequential per-tool upgrade (delegated).** For each installed tool in roster order, print its per-tool header then call `upgradeTool(ctx, stdout, stderr, t, probes[i])`. On a successful (exit 0) upgrade, re-query the version via `toolAfterVersion(ctx, t)` (a brew read for brew-managed tools, the Probe spec for delegated tools — never a brew read for a formula-less tool) and record a bump against `probes[i].beforeVersion` if it changed (the digest input, r01z). Dispatch:
   - **delegated (non-brew) tool** → its `Update` argv verbatim (`rk desktop update`) — no `--skip-brew-update` probe or append, no brew fallback. (t26g)
   - **has `Update` argv + supports the flag** → `<tool> update --skip-brew-update` (the `Update` argv with the flag appended).
   - **has `Update` argv but no flag (version skew)** → `<tool> update` with no flag — the dispatch never *chooses* `brew upgrade` over a present `update` subcommand. This is the retry-without-flag contract for an installed tool predating the `--skip-brew-update` convention (Constitution V — Graceful Degradation). (A *failing* delegated update is a separate matter — see [delegation-failure brew fallback](#delegation-failure-brew-fallback).)
   - **no `Update` argv (hypothetical future brew-managed tool)** → `brew upgrade <formula>` fallback, via the streamed-tail helper. Because every current roster tool populates `Update`, this fallback is **presently unreachable for the roster** — it is wired for correctness and future tools.

   On the delegation path, `upgradeTool` carries two self-heals before giving up on a tool, both **brew-managed-only** (`t.brewManaged()` guards): the **unlinked-keg relink heal** (delegated `update` returns `proc.ErrNotFound` despite the install probe → `brew link <tool>`, print `relinkNoteFmt`, retry the delegation once; a failed link skips the retry) and the **[delegation-failure brew fallback](#delegation-failure-brew-fallback)** (any remaining failure → one direct `brew upgrade <formula>`). A delegated (non-brew) tool has no keg to link and no formula to upgrade, so its failure surfaces as that tool's failure — no relink note, no `fallbackNoteFmt`, no brew call. Best-effort across the roster: on final per-tool failure, set `anyFailed = true` and `continue` — never abort the loop.

9. **End-of-run agent-skill refresh (placement-gated).** After the roster loop and before the tail, `refreshPlacedAgentSkills(ctx, env, yes, stdout, stderr)` (`agent_setup.go`) re-runs `shll setup agent` as a **subprocess** — see [the refresh + `--yes`](#end-of-run-agent-skill-refresh--yes) below. Skipped entirely when no prior `shll setup agent` placement exists; best-effort (never changes the exit code). This subprocess deliberately keeps `proc.RunForeground` (inherited stdin) — it is the one interactive path the null-stdin hardening excludes (see [Design Decision: stdin hardening scoped to the write-phase children](#stdin-hardening-scoped-to-the-write-phase-children-only)).

10. **Summary tail, then the digest.** After the loop, print one summary line via `printSummaryTail` (see [Per-tool output separation](#per-tool-output-separation)), then the **"What changed:" digest** for the recorded bumps (r01z — see [version capture](#version-capture--the-what-changed-digest)), then, if `anyFailed`, return `errSilent` (exit 1); else return nil (exit 0). Both the tail and the digest are presentation-only and do **not** influence the exit code.

> **Slice-aliasing guard.** The roster's `Update` argvs are shared, read-only slices. `upgradeTool` appends the flag via `appendArg` (`src/cmd/shll/update.go`), which always allocates a fresh slice (`make` + `copy`) so a naive `append` can never write into the shared backing array when spare capacity exists. The same helper builds the `--help` probe argv.

## Positional tool-name args — subset targeting

`shll update [tool...]` accepts zero or more positional tool-name args (`Args: cobra.ArbitraryArgs`, parsed args threaded into `runUpdate`). The grammar mirrors `brew upgrade <formula>` — positional, not a `--only` flag.

- **Zero args → whole-roster run, byte-for-byte unchanged.** `subset := len(args) > 0` is false, so the contract above (probe whole roster + shll self-upgrade) holds verbatim. This is the back-compat anchor.
- **One or more args → operate on just the named subset.** The args form a *set*, not a sequence.

**Valid targets for `update`**: the seven `Roster` names (`run-kit`, `rk-desktop`, `fab-kit`, `wt`, `idea`, `tu`, `hop`) **plus** the literal `shll`. `shll` is special — it is not in `Roster` (Constitution III — `Roster` is the sub-tool list), so the self-target name is the named constant `shllTargetToken = "shll"` (`src/cmd/shll/tools.go`). Naming `shll` engages the existing self-upgrade path (`brew upgrade sahil87/tap/shll`); see [shll self-upgrade](#shll-self-upgrade).

**The legacy alias `rk` → `run-kit`.** `shll update rk` still works: `resolveTargets` resolves `rk` via the `legacyAliases` map to the canonical `run-kit` tool and reports it via its `aliased []string` return, so `runUpdate` prints a one-line `note: rk is now run-kit` (`printAliasNotices`, to stdout, before the status line). The alias is **not** advertised in the valid-targets diagnostic — `shll update foo` still lists canonical names only (`valid targets: shll, run-kit, rk-desktop, fab-kit, wt, idea, tu, hop`, no `rk`). `shll update rk` resolves to the canonical `run-kit` and then follows the plain single-probe delegated-upgrade path (or the graceful skip / named `not installed` error when run-kit is absent). See [cli/commands §the legacy target alias](/cli/commands.md#the-legacy-target-alias-rk--run-kit).

**Roster-order processing.** A subset is always processed in `Roster` order regardless of arg order — `resolveTargets` returns the selected `Tool`s in roster order. Example: `shll update wt fab-kit` processes `fab-kit` then `wt`. When `shll` is among the targets it keeps its position as the **first** step (self-upgrade before the roster loop), exactly as in a whole-roster run — `shll update shll hop` runs `shll (self)` first, then `hop`. (The ordering contract: [Roster order](#roster-order).)

**Validation is up front, before any work (`runUpdate` resolves the subset before `hasBrew`, the status line, and `probeRoster`).** Two error classes, both exit non-zero via `errSilent`:

1. **Unknown / typo'd name** → `resolveTargets(args, true)` returns a non-nil error; `runUpdate` writes `shll update: <detail>` to stderr and returns `errSilent` with **zero brew/network side effect** (no probe, no `brew update`). All unknown args are reported at once (a better one-shot fix), e.g. `shll update: unknown targets "foo", "bar" (valid targets: shll, run-kit, rk-desktop, fab-kit, wt, idea, tu, hop)`.
2. **Named-but-not-installed** → distinct from the whole-roster *graceful skip*. Because the user named the tool explicitly, its absence is surfaced as an error, not silently skipped. Enforced **after** probing (where install facts exist): every selected target that probed not-installed — including `shll` itself on a non-brew dev build (e.g. a `go install` binary, where `isInstalled(shllFormula)` is false) — is reported as `shll update: <name>: not installed`, all missing targets at once in roster order, before any `brew update`/upgrade.

**How the subset is applied (no parallel loop).** `runUpdate` reuses the existing whole-roster code paths: after enforcing the not-installed error, it marks `probes[i].installed = false` for every roster tool *not* in the selection, and overrides `shllSelfInstalled` to `selfSelected && shllInstalled`. The existing `total`/upgrade-loop/dry-run/tail code then operates on the subset with no structural change (Design Decision #3 of this change — smallest diff, preserves every order-independent invariant).

**Counter denominator `M` = subset size.** For a subset run the per-tool header `[N/M]` denominator and the summary-tail `M` become the count of validated, processed targets (installed roster tools in the selection, plus 1 when `shll`-self was selected and installed) — not the whole-roster count. The [per-tool output separation](#per-tool-output-separation) contract is otherwise unchanged: `M` is simply redefined as subset size, and the headers/tail still conform (the `per-tool-output-separation` spec stays valid as-is).

**`brew update --quiet` still runs once for a subset.** Unconditional, exactly once, even for a single-tool target — it sits below the nothing-to-do short-circuit, which cannot fire for a validated *installed* subset (a named-but-not-installed target already errored out above). The `--skip-brew-update` per-tool delegation is unchanged.

**`--dry-run` previews the filtered subset.** The dry-run branch runs after the subset filter, so it previews only the validated subset in roster order (shll-self first when selected), header `Would update N tools (brew metadata refresh first):` with `N` = subset size. See [`--dry-run`](#--dry-run).

**Shared resolver, single-sourced with `Roster`.** Both `runUpdate` and `runInstall` call `resolveTargets(args, allowShll)` (`src/cmd/shll/tools.go`) — `update` passes `allowShll=true`, `install` passes `false`. It performs **name validation only** (no brew/subprocess calls — the install-status check stays in the run functions where brew facts exist), and derives its valid-name list from the live `Roster` (via `rosterHas`/`validTargets`) so the two commands can never drift. See [cli/install §positional args](/cli/install.md#positional-tool-name-args--subset-targeting) for the symmetric (roster-only) install behavior.

## Version capture + the "What changed:" digest

The **"What changed:"** digest tail surfaces what a run changed — version transitions and release notes that would otherwise be buried in streamed brew/tool output — built from before/after versions the run already captures (r01z). The digest is **presentation-only**: it never touches `anyFailed` or the process exit code. Each release's **full body renders inline**, in-process, from the same `changelog.FetchAll` data the digest already holds — never a subprocess to `shll changelog`; the release blocks are rendered by the **shared `renderReleases` helper** ([`renderChangelogResult`](/cli/changelog.md#output-shape)), so the digest and `shll changelog` cannot drift (13k3). See [Digest rendering](#digest-rendering-printupdatedigest) below.

### Version capture (before + after)

- **Before-version** — captured for free from the existing probe. `probeToolInstalledVersion(ctx, t)` (`src/cmd/shll/version.go`) is the one read per roster tool, branching on `t.brewManaged()`: brew-managed tools take `probeInstalledVersion(ctx, t.Formula)` (`brew.go`) — the **sole** `brew list --formula --versions <formula>` invocation in `cmd/shll`, returning the exit-code install fact and the parsed version from one read (`isInstalled`/`installedVersion` are thin wrappers over it — the identical brew read exists in exactly one place, see [Detection](#detection)); delegated (non-brew) tools take the shared `Probe` spec via `probeToolVersion` (`rk desktop status`; `parseProbeStatusLine` parses the `Installed:` line — see [cli/version §the shared install probe](/cli/version.md#the-shared-install-probe)). `probeTool` stores the version as `probeResult.beforeVersion`; shll-self's `beforeShll` comes from the single `probeInstalledVersion(ctx, shllFormula)` in step (4). Never streamed foreground output (code-quality.md anti-pattern).
- **After-version** — after each **successful** (exit 0) upgrade, the version is re-queried (a fresh cheap captured read): `toolAfterVersion(ctx, t)` for each roster tool — a thin wrapper over `probeToolInstalledVersion`, so brew-managed tools re-read the brew keg version and delegated tools re-run their Probe spec (rk-desktop's after-version comes from `rk desktop status`, never a brew read) — and `installedVersion(ctx, shllFormula)` for shll-self. The re-query runs **only** for tools that exited 0.
- **Multi-keg pick.** `parseBrewVersion` (`brew.go`) takes the max across `fields[1:]` of the `brew list --versions` line via `changelog.CompareVer` — a multi-keg host lists every installed version in **arbitrary** order (`tu 0.6.4 0.6.2`), so blindly taking `fields[1]` could report an oldest keg and suppress/misreport a transition. Pinned by `TestParseBrewVersion_MultiKegPicksMax`.

`makeBump(tool, repo, old, new)` builds a `versionBump` **only** when both versions are known and differ (`old != "" && new != "" && old != new`) — the guard that keeps a no-op run's output byte-identical. Bumps are collected in roster order (shll first). Each `versionBump` carries `repo` (the GitHub slug — a separate field because it is not always the tool name: rk-desktop ships with the run-kit repo, so its `Repo` is `run-kit`) so the digest can fetch releases and build the compare URL.

### Digest rendering (`printUpdateDigest`)

For the recorded bumps, `printUpdateDigest(ctx, stdout, bumps, color)` fetches every tool's release range **concurrently** via `changelog.FetchAll` (reusing the [internal/changelog](/internal/changelog.md) fetch/filter code — single source of truth with `shll changelog`; both surfaces share one release rendering, and the digest adds a tool-name-bearing transition line) and prints, after the summary tail (full bodies inline, 13k3):

```
What changed:
  tu 0.6.2 → 0.6.4 (2 releases)

v0.6.4  fix: opencode session parsing
## What's Changed
* fix: parse sessions with missing usage blocks

v0.6.3  feat: daily usage rollups
...

  hop 0.1.16 → 0.1.18 (2 releases)

v0.1.18 feat: non-interactive agent support
...
```

(Full release bodies inline, per-tool blocks blank-line separated. The exact ASCII-degraded goldens are `TestUpdate_DigestPrintsForBumpedTools`; on a `bytes.Buffer` writer the `→` degrades to `->` and no ANSI is emitted.)

- **Full release bodies inline (13k3)** — each release renders as `{tag}  {title}` followed by its **full body markdown** (trailing newlines trimmed, empty bodies skipped), newest-first, via the shared **`renderReleases`** helper — the same rendering `shll changelog` uses (see [cli/changelog §output shape](/cli/changelog.md#output-shape)), so the two surfaces cannot drift. In-process from the existing `changelog.FetchAll` data — never a subprocess. Displayed versions are the normalized (v-stripped) forms carried on the bump.
- **10-release cap + compare-URL cap notice (13k3)** — the digest now applies the same `changelogCapPerTool = 10` cap as `shll changelog`: only the 10 newest releases print, then `… {N-10} more — full changelog: {compareURL}` (the cap logic lives inside `renderReleases`), so a long-range bump never dumps unbounded text.
- **No cross-tool alignment, no per-block tag padding (13k3)** — with multi-line bodies inline, cross-tool column alignment would be meaningless; release blocks adopt the shared `shll changelog` format verbatim. The per-tool transition line keeps the 2-space `digestToolIndent` under the `What changed:` header; release blocks render **unindented** (each preceded by a leading blank line) via `renderReleases`. `digestHeader` / `digestToolIndent` are the only digest layout constants.
- **Bold-anchor transition line (13k3)** — the per-tool transition line `{tool} {old} → {new} ({N} release{s})` is a plain-bold navigational anchor (`bold(color, …)`, NOT bold-cyan — bold-cyan stays reserved for the [per-tool header](#per-tool-output-separation); the [tag/title lines are bold too](/cli/changelog.md#output-shape) via the shared helper). The transition line **carries the tool name** — unlike `shll changelog`, whose tool name lives in the `printToolHeader` above the body.
- **Tool blocks are blank-line separated (13k3)** — a blank line precedes each per-tool block **except the first** (`if i > 0`), mirroring `runChangelog`'s per-tool separation so tools are never separated more weakly than the releases within one tool.
- **No `Full notes:` command line** — bodies are inline, so the digest never prints a copy-paste `shll changelog` command. (13k3)

### Edge cases + degradation

- **Nothing bumped** (all up-to-date or all failed) → `bumps` is empty → `printUpdateDigest` prints **nothing** (no `What changed:` block, no command). Output is byte-identical to before this change — the existing goldens hold (the test fake returns the same version before and after, so no bump). Pinned by `TestUpdate_NoDigestWhenNothingBumped`.
- **`--dry-run`** → the dry-run branch returns before the write phase, so no upgrade runs, no bump is recorded, and no digest prints (`TestUpdate_NoDigestUnderDryRun`).
- **Subset runs** → the digest covers only the bumped subset members and the command names only those tools (`TestUpdate_DigestSubsetNamesOnlyBumped`).
- **Fetch unavailable / zero-in-range** → a bumped tool whose fetch fails, or whose fetched range holds zero matching releases (tag-scheme mismatch), degrades to `{tool} {old} → {new} — see {compareURL}` (no body lines exist to inline; Constitution V); the rest of the digest still renders (`TestUpdate_DigestUnavailableDegradesToCompareURL`, `TestUpdate_DigestMixedAvailableAndUnavailable`). Never changes the exit code. **The whole degrade line is bolded** (13k3) — a deliberate choice so it reads as a bold anchor like the available branch's transition line (this differs from `shll changelog`'s own unavailable line, which is left unstyled — a [known, reviewed-as-acceptable asymmetry](/cli/changelog.md#output-shape)).
- **ASCII degrade** → the digest's `→`/`—`/`…` glyphs degrade to `->`/`--`/`...` on a non-TTY / `NO_COLOR` stream (per-tool-output-separation spec). The `color` decision `runUpdate` computed once for the headers/tail is threaded into `printUpdateDigest` and through to `renderReleases`; the shared `arrow(color)`/`dash(color)`/`more(color)` helpers (`ui.go`) do the swap, and `bold(color, …)` wraps the anchor lines only when color is on. `bytes.Buffer` test writers deterministically hit the ASCII/no-ANSI branch (`0.6.2 -> 0.6.4`).

Covered end-to-end by `TestUpdate_DigestPrintsForBumpedTools` (bump → digest with inline bodies, no `Full notes:` line) and `TestUpdate_NoDigestWhenNothingBumped` (no bump → no digest).

## Exit codes

| Condition | Exit code |
|-----------|-----------|
| All upgrades succeeded (or nothing-to-do branch) | 0 |
| Unknown/typo'd positional target (b2vg) | 1 (via `errSilent`, before any brew work) |
| Named-but-not-installed positional target, incl. `shll` on a dev build (b2vg) | 1 (via `errSilent`, after probing, before any upgrade) |
| `brew` not on PATH | 1 (via `errSilent`, hint already on stderr) |
| `brew update --quiet` failed (non-zero exit OR transport error) | 1 (via `errSilent`) |
| `shll` self-upgrade failed | 1 (via `errSilent`, after roster also attempted) |
| Any per-tool upgrade failed (delegated `update` or brew-upgrade fallback) | 1 (via `errSilent`, after all tools attempted) |

## shll self-upgrade

`shll update` self-upgrades `shll` itself before iterating the roster. The behavior is contingent on detection (step 4):

- **Brew-installed shll** (`brew install sahil87/tap/shll`) → self-upgrade runs as `brew upgrade sahil87/tap/shll` immediately after the metadata refresh, before any roster upgrade. The mid-run binary on disk gets replaced; the running process keeps its mapped image and finishes normally; a follow-up `shll` invocation picks up the new binary. Pinned by `TestUpdate_AllInstalled` and `TestUpdate_SelfUpgradeOrdering`.
- **Dev build** (e.g. `go install ./cmd/shll`) → `isInstalled(ctx, shllFormula)` returns false, the self-upgrade is skipped silently, and the roster loop proceeds normally. Pinned by `TestUpdate_SelfNotBrewInstalled`. This avoids `brew upgrade` errors that would otherwise fire on a non-brew-managed binary (Constitution V — Graceful Degradation).

The self-upgrade does not delegate — shll has no `update` subcommand to call on itself, so it stays a direct `brew upgrade shllFormula`. `shll` is intentionally **not** added to `Roster`: `Roster` is the *sub-tool* roster (Constitution III — Tool Roster Source of Truth); commingling shll itself would distort `shll version`'s output (which already prints shll separately) and `shll shell-init`'s iteration semantics.

Ordering rationale: self-upgrade runs *before* the roster loop so the on-disk binary is updated as early as possible. Subsequent operations within the same invocation still execute the original mapped image (POSIX semantics — replacing the file on disk doesn't affect a running process), so there is no risk of partial-version mixing within one run.

> **The `shll (self)` first step is the manage-side instance of the shared display pattern.** `update` leads with a `shll (self)` step as `[1/M]` before the roster loop. shll is self-upgraded via this dedicated `brew upgrade shllFormula` path (it has no `update` subcommand to delegate to), not via the shared `shllSelf` descriptor — the descriptor is a *display* source of truth for the inspect surface (`list`/`doctor`/`install`), while update's self-step is a *brew action*. See [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor). (bb7r)

## Trust posture and the Homebrew 6.0.4 floor

`shll update`'s brew write calls (`brew update --quiet`, the `brew upgrade sahil87/tap/shll` self-upgrade, and the no-`Update`-argv `brew upgrade <formula>` fallback) ride the streamed-tail transport (`proc.RunStreamedTail` via `runStreamedChild`) with **no environment injection** — in particular, no `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` override (0854, closing backlog `[tkch]`; the Homebrew Linux-sandbox trust bug that once motivated one is fixed upstream in 6.0.4). shll requires Homebrew ≥ 6.0.4, documented in the README, not gated in code.

**`shll update` deliberately does NOT mutate trust** — silently changing trust state on an upgrade command violates least-surprise. Per-formula trust is established by `shll install` (see [cli/install §per-formula trust before install](/cli/install.md#per-formula-trust-before-install)); [cli/doctor](/cli/doctor.md#the-trust-sub-check) surfaces any installed-but-untrusted tool so the user can re-run `shll install`. `internal/proc` carries no per-request env surface (no `Request.Env`, no `RunForegroundEnv`) — see [internal/proc](/internal/proc.md).

## Detection

`probeInstalledVersion(ctx, formula)` in `src/cmd/shll/brew.go` is the single source of truth for "is this brew formula installed" and for the installed version (r01z):

- Calls `brew list --formula --versions <formula>` via `proc.Run` (capture transport) — the **sole** such invocation in `cmd/shll` (the boolean- and version-only helpers are wrappers over it).
- Returns `(installed bool, version string)`: `installed` is `err == nil` (`brew list --versions <formula>` exits 0 when installed, 1 when not); `version` is `parseBrewVersion(stdout)` (the max keg version, `""` on any failure). The install fact keys on the exit code; the version parse is a best-effort second return.
- `isInstalled(ctx, formula)` wraps it (`installed, _ := probeInstalledVersion(...)`) so boolean-only callers (`install.go`'s partition, the shll-self check, `changelog.go`'s no-range probe) share the one read; `installedVersion(ctx, formula)` returns just the version (see [version capture](#version-capture--the-what-changed-digest)).

Constraints (Design Decision #2):

- **No regex** over plain `brew list` output. The `code-quality.md` anti-pattern explicitly forbids this. (The `--skip-brew-update` capability probe (cczs) holds the same line — it is a `strings.Contains` presence check on `<tool> update --help` output, never a regex.)
- **No symlink-target inspection** (hop's `/Cellar/` trick). That works for the running binary only; we are querying *other* tools' install status.
- **No hardcoded `/opt/homebrew` or `/usr/local`** paths anywhere — the brew CLI is always invoked through PATH lookup via `exec`.

`hasBrew(ctx)` in `src/cmd/shll/brew.go` runs `brew --version` via `proc.Run` and returns true unless the error wraps `proc.ErrNotFound`. Any other brew failure (e.g. brew exits non-zero) still implies brew is installed — graceful degradation: only `ErrNotFound` is the "missing" signal.

## Probe-first detection of `--skip-brew-update`

`shll update` decides whether to append `--skip-brew-update` to a delegated `<tool> update` *before* invoking it — by probing, not by trying and retrying:

- **Why probe-first** (Design Decision #3, cczs): knowing flag support up front avoids the false-positive where a genuine upgrade failure is mistaken for a flag-parse error. An "assume-support-then-retry-on-failure" strategy would re-run the tool's `update` after a real failure — which could re-trigger side effects (e.g. run-kit's daemon restart) it had already partially performed. A presence check on `--help` is side-effect-free.
- **Version-skew handling**: when the probe reports the flag is *not* advertised (an installed tool predating the toolkit-wide `--skip-brew-update` contract), shll runs the tool's `update` **without the flag** — the dispatch never picks `brew upgrade` over a present `update` subcommand, because the tool's own `update` is still the faithful composition (Constitution IV). The tool will then run its own internal `brew update`; correctness is preserved at the cost of a redundant metadata refresh for that one tool. Pinned by `TestUpdate_FlagUnsupportedVersionSkew` (which also pins that a *succeeding* flagless update triggers no fallback brew upgrade).
- The probe is issued **only for installed brew-managed tools that have a non-empty `Update` argv** — uninstalled tools, delegated (non-brew) tools, and tools with no `update` subcommand are never probed (`TestUpdate_PartialInstalled`, `TestUpdate_NoUpdateArgvFallsBackToBrew`, `TestUpdate_RkDesktopDelegatesToRkDesktopUpdate`).

## Delegation-failure brew fallback

When a delegated `<tool> update` fails — **any** failure: non-zero exit or a transport error (a binary too broken to exec) — after the unlinked-keg relink heal has had its chance, `upgradeTool` (`src/cmd/shll/update.go`) falls back **once** to `brew upgrade <t.Formula>` via the streamed-tail helper, announced beforehand on stdout via the named constant `fallbackNoteFmt`:

```
note: idea's own update failed (exit code 1) — falling back to 'brew upgrade sahil87/tap/idea'
```

The note carries the tool name, the delegated failure detail (exit code, or the error text for transport failures), and the exact fallback command — so the underlying failure stays visible even when the fallback rescues the tool.

- **Trigger + ordering.** Brew-managed delegation path only (`len(t.Update) > 0 && t.brewManaged()` — the no-argv path's primary command already IS `brew upgrade`, so it never gets a second attempt, and a delegated (non-brew) tool has no formula to fall back to). The relink heal runs first (the more specific remedy); the fallback evaluates the *final* delegated outcome, so it also rescues the degraded relink branch (link failed → the fallback's `brew upgrade` relinks the keg as a side effect).
- **Single attempt.** Exactly one fallback `brew upgrade` per tool per run — never a retry loop.
- **Accounting.** Fallback success (exit 0) counts the tool as succeeded — the normal `succeeded++`, after-version re-query, and digest path run unchanged. Fallback failure is returned as the tool's outcome, feeding the existing `anyFailed`/stderr reporting.
- **Brew-safety.** The fallback call carries the caller's context with **no deadline and no signals** — conformant with the update standard's brew-safety clause (`docs/site/standards/update.md`) and Constitution I. shll's deadline-free brew call is the whole point: it survives brew runs that stall for minutes.
- **Side-effect trade-off.** A fallback upgrade skips the tool's own post-upgrade side effects (e.g. run-kit's daemon restart) for that one run; the next delegated run restores normal composition (Constitution IV). Rescue-path-only deviation, accepted deliberately.
- **Known limit (deliberate non-goal).** No broken-keg reinstall escalation: if a prior mid-pour kill left brew believing the new version is installed while the binary is broken, the fallback `brew upgrade` no-ops and the tool stays broken. Possible follow-up; excluded from the fallback's scope.

Pinned by `TestUpdate_DelegatedFailureBrewFallbackRescues` (fail → note + fallback follows the failed delegation → run succeeds; the note carries `exit code 1`), `TestUpdate_DelegatedFailureFallbackAlsoFails` (both fail → `errSilent`, exactly one fallback attempt), `TestUpdate_NoArgvFailureNoDoubleBrewUpgrade` (no-argv path: one `brew upgrade`, no note), and `TestUpdate_DelegationUnlinkedKegLinkFails` (failed link → no retry, then the fallback rescues).

## Probe capture vs. write transports

| Subprocess | Transport | Why |
|------------|-----------|-----|
| `brew --version` (in `hasBrew`) | `proc.Run` (capture) | Internal probe; user does not need to see output. |
| `brew list --formula --versions <formula>` (in `isInstalled`) | `proc.Run` (capture) | Same — it's a probe, not user-facing. |
| `rk desktop status` (delegated installed-probe) | `proc.RunCaptured` (both streams captured) | Same — a probe; both streams are captured so a platform-refusal message on either stream is detectable. |
| `<tool> update --help` (brew-managed capability probe) | `proc.Run` (capture) | Probe — captured so shll can branch on flag support. `proc.Run` (TransportCapture) captures **stdout** but still streams **stderr** through; the probe writes its meaningful output to stdout and is silent on stderr in the normal case, so concurrent stderr interleaving is a rare, cosmetic edge (see "Sequential, not parallel" below). Never issued for a delegated (non-brew) tool. |
| `brew update --quiet` | `proc.RunStreamedTail` (null stdin, live tee + bounded tail) | Brew's progress streamed to user's terminal; a prompt attempt reads EOF instead of hanging. |
| `<tool> update [--skip-brew-update]` / `rk desktop update` (delegated upgrade) | `proc.RunStreamedTail` (null stdin, live tee + bounded tail) | User-visible upgrade; the tool's own progress + side-effect output streams to the terminal. |
| `brew upgrade <formula>` (self-upgrade + no-`Update`-argv fallback) / `brew link <tool>` (relink heal) | `proc.RunStreamedTail` (null stdin, live tee + bounded tail) | Same — preserves brew's colored progress output. |
| `shll setup agent [--yes]` (end-of-run agent-skill refresh) | `proc.RunForeground` (inherited stdio) | The one deliberate exception — run-kit's hook confirmation is a documented interactive path when `--yes` is absent (Design Decision below). |

This split is a Constitution-aligned choice: probes capture (so shll can branch on the result), user-visible write operations stream live (so the user sees brew / the tool working) with null stdin (so a prompt attempt fails fast). All write-phase children route through `proc.RunStreamedTail` via the shared `runStreamedChild` helper (`brew.go`) — there is no per-request env override (see [trust posture](#trust-posture-and-the-homebrew-604-floor)). See [Null-stdin streamed children](#null-stdin-streamed-children) and [internal/proc §TransportStreamTail](/internal/proc.md#transportstreamtail-used-by-procrunstreamedtail).

## Sequential, not parallel — scoped to *upgrades*

Design Decision #3 ("sequential, not parallel") governs **upgrades only** — the read-only capability probes carry an explicit carve-out (cczs):

- **Probes are parallel.** `probeRoster` dispatches one goroutine per roster tool. This is safe — the probes (`brew list`, the delegated `rk desktop status`, `<tool> update --help`) take **no Homebrew write lock**, so there is no lock contention. Their **stdout** is captured by `proc.Run` (not foregrounded). Note that `proc.Run`'s `TransportCapture` still streams **stderr** to the terminal, so stderr emitted by a probe *can* interleave during the concurrent phase; in practice the `--help` probes run only for installed brew-managed tools and write their meaningful output to stdout, so this is a rare, cosmetic edge rather than a correctness concern. (If truly-silent probes were ever required, the fix would be a `proc` transport that also captures/discards stderr — deliberately not added here for so marginal a case, to avoid expanding the Constitution-I-critical wrapper.) Concurrency collapses the ~8 sequential probe spawns of the old install-filter into ~1 wall-clock. Results are written into a fixed-size slice indexed by roster position, so the upgrade loop still sees roster order regardless of completion order. Probe concurrency is unbounded at the current roster size (7) — revisit only if the roster grows substantially.
- **Upgrades remain sequential.** The per-tool upgrade loop is a plain `for` with synchronous streamed-tail calls. Upgrades stay serial because (a) brew serializes most internal operations behind its own lock, and (b) parallel *streamed* subprocesses would interleave output incomprehensibly. `TestUpdate_OneUpgradeFails` asserts the loop continues through all roster entries even when the first one fails.

## Per-tool output separation

`shll update` frames each tool's foregrounded output with a labeled boundary so a multi-tool run is no longer one undifferentiated wall of text. All framing logic lives in the shared helper `src/cmd/shll/ui.go` (see [cli/commands](/cli/commands.md#file-layout-srccmdshll)); `update.go` only computes the color decision once and calls into it.

- **Per-tool header with `[N/M]` progress counter (6vuo; color form 13k3).** Immediately before each tool's foregrounded output, `printToolHeader(stdout, name, pos, total, color)` (`src/cmd/shll/ui.go`) writes `▸ [N/M] <tool>` on a color-enabled TTY, or `==> [N/M] <tool>` in pure ASCII otherwise. **On the color branch the WHOLE `▸ [N/M] <tool>` run is a single bold-cyan span** (13k3), so tool boundaries pop visually. The plain branch carries no ANSI. The `==>` idiom matches Homebrew's convention so the plain form reads naturally alongside brew's own output. `N` is the running 1-based position; `M` is the total tools acted on this run, **computed up front before the loop** (`update.go` — `total` is the count of `probes[i].installed` plus `1` when shll is brew-installed) so every header can carry a stable denominator. The self-upgrade step (step 7) gets the header `shll (self)` and is **`[1/M]`** — it counts as a tool like any other, so the counter agrees with the summary tail's `total` (which also includes the self step); each roster tool (step 8) gets `t.Name` at its position. (The header stays minimal — just `▸ [N/M] <tool>`; a dimmed command echo like `$ tu update --skip-brew-update` was considered and rejected as noise duplicating `--help`.) See [Worked header example](#worked-header-example).
- **Section spacing (6vuo).** A single blank line precedes each per-tool header **except the first**, and a single blank line precedes the summary tail — so each tool's streamed output is visually separated from the next header and from the tail. The loop emits the leading `\n` via the `updateHeader` closure (`update.go`, `if pos > 1`); the pre-tail blank is `fmt.Fprintln(stdout)` immediately before `printSummaryTail` (`update.go`). The empty/short-circuit case emits NO blank lines (its golden string is preserved — see [Empty case](#per-tool-output-separation)).
- **Summary tail with run duration (6vuo).** After the loop, `printSummaryTail(stdout, succeeded, total, elapsed, color)` (`src/cmd/shll/ui.go`) writes exactly one line derived from **exit codes only**, now with the wall-clock run duration appended to **both** forms: `Done — N of M tools succeeded in <dur>.` on full success (prefixed with a green `✓` when color), or `X succeeded, Y failed in <dur> — see above.` on partial failure (the duration sits **before** the em-dash). `total` counts every tool attempted (self-upgrade + each installed roster tool); `succeeded` counts those that exited 0 — these mirror the same per-tool facts that drive `anyFailed`. The duration is a **fact about the run, not an outcome claim** — the tail still **never** claims "updated" vs. "up-to-date" (the honesty constraint — streamed sub-tool output means shll knows only exit codes), and never changes the process exit code. Duration is rendered by `formatDuration` (`ui.go`) as `elapsed.Round(time.Second).String()` (e.g. `1m12s`; sub-second runs round to `0s`). See [Run duration and the clock seam](#run-duration-and-the-clock-seam).
- **Stream discipline (critical).** The header and tail are written to **stdout** — the same stream the streamed-tail transport tees sub-tool output onto (in production `cmd.OutOrStdout()` is `os.Stdout`). They are **never** written to stderr: a different buffer with independent flush timing would interleave unpredictably against the streamed output it labels. `TestUpdate_*` drive `runUpdate` with separate stdout/stderr buffers and assert header/tail text appears only in stdout.
- **Color gating.** `colorEnabled(stdout)` (`src/cmd/shll/ui.go`) is evaluated once and reused for every header and the tail. It returns true only when **both** (1) stdout is a real terminal — the writer is an `*os.File` AND `term.IsTerminal(fd)` (from `golang.org/x/term`, the codebase's first terminal inspection), and (2) `NO_COLOR` is unset (no-color.org convention). A `bytes.Buffer` test writer is never an `*os.File`, so tests deterministically hit the plain-ASCII branch. The ASCII degrade swaps both the glyph (`▸`→`==>`) and any Unicode in shll's own framing; sub-tool bytes are passed through untouched in both forms.
- **Empty case emits no header, no tail, no counter, no spacing, no duration.** The nothing-to-do short-circuit (step 5, `No shll tools installed.`) runs no per-tool loop, so there is nothing to separate, count, or time. Its stdout is still **exactly** `Checking installed shll tools…\nNo shll tools installed.\n` — the `TestUpdate_NoToolsInstalled` and `TestUpdate_EmptyCaseNoHeaderNoTail` golden strings are preserved verbatim (no `[N/M]` header, no blank lines, no tail, no `in <dur>`). Only the loop path (step 8 reached) carries the `==> [N/M]`/blank-line/duration markers in its golden strings.

### Worked header example

With shll brew-installed and the six brew-managed tools present (rk-desktop absent, per its probe), `shll update` (plain, non-TTY) frames the run as (blank lines shown explicitly):

```
Checking installed shll tools…
==> [1/7] shll (self)
<shll's brew upgrade output…>

==> [2/7] run-kit
<run-kit's update output…>

==> [3/7] fab-kit

==> [4/7] wt

==> [5/7] idea

==> [6/7] tu

==> [7/7] hop

Done — 7 of 7 tools succeeded in 1m12s.
```

An installed rk-desktop slots in as its own `==> [N/M] rk-desktop` header directly after run-kit (roster adjacency — the delegated `rk desktop update` streams like any other), raising `M` by one. This exact sequence (status line, `[1/7] shll (self)` first, a blank line before each subsequent header and before the tail, and the duration-bearing tail) is the `TestUpdate_HeadersAndTail` golden in `src/cmd/shll/update_test.go` (which installs a deterministic clock returning `t0` then `t0+72s` so the tail reads `in 1m12s`). `TestUpdate_HeaderPrecedesOutput` pins that the `==> [1/1] hop` header is in the buffer *before* hop's streamed upgrade runs; `TestUpdate_PartialFailureTail` pins the partial-failure tail `1 succeeded, 1 failed in 1m12s — see above.` and asserts the honesty constraint (no "updated"/"up-to-date").

### Run duration and the clock seam

The duration in the summary tail is measured via an injectable package-level clock seam — `var nowFunc = time.Now` in `src/cmd/shll/clock.go`. This mirrors the `proc.Runner` package-level-swappable injection pattern (`src/internal/proc/proc.go`) exactly: production wiring uses the real `time.Now`; tests swap it through the `installFakeClock(t, times...)` t.Cleanup helper (`src/cmd/shll/clock_test.go`, mirroring `installFakeRunner`) to a deterministic clock that returns the supplied times in sequence (the last value repeats), so the duration-bearing golden strings stay exact rather than racing a real wall clock.

`runUpdate` captures `start := nowFunc()` in `runUpdate` — **after** the nothing-to-do short-circuit *and* the dry-run branch have returned — so the measured elapsed (`nowFunc().Sub(start)`) covers only the write phase the tail summarizes (the metadata refresh + self-upgrade + roster loop), not the read-only probe phase. The seam keeps `runUpdate`'s signature stable apart from the `dryRun bool` parameter (see [`--dry-run`](#dry-run)). `TestInstallFakeClock_Sequences` unit-tests the helper's sequencing.

## Null-stdin streamed children

Every write-phase child — `brew update --quiet`, the shll-self `brew upgrade`, the relink heal's `brew link`, each delegated `<tool> update [--skip-brew-update]` / `rk desktop update`, and the brew-upgrade fallbacks — runs through `runStreamedChild(ctx, stdout, stderr, argv...)` (`src/cmd/shll/brew.go`), the shared thin wrapper over `proc.RunStreamedTail` (yud0):

- **Null stdin (`cmd.Stdin = nil`).** A child that attempts an interactive prompt reads EOF and fails fast instead of hanging the walk — the toolkit's prompt-free standard is *enforced*, not accommodated (a genuinely unavoidable question reads `/dev/tty` in the tool itself). The end-of-run agent-skill refresh is the one exception — it keeps `proc.RunForeground` (inherited stdin) because its run-kit hook confirmation is a documented interactive path when `--yes` is absent.
- **Live tee, never buffered.** Child stdout/stderr stream to the run's writers in real time; children see a pipe, so tty-only child rendering (e.g. brew progress bars) degrades — accepted, since the toolkit standard already requires children to behave non-interactively.
- **Bounded tail capture, currently unconsumed.** The transport also captures the last ~4KB of interleaved output (`tailRingSize`) into `Result.Tail`; no caller consumes it (kept for API stability — see [internal/proc §TransportStreamTail](/internal/proc.md#transportstreamtail-used-by-procrunstreamedtail)). A failed child's cause is already visible in the linear scrollback, so no tail re-print frame exists.
- **Exit-code semantics unchanged.** A non-zero exit is reported via `code` with `err == nil` (the caller branches on the code); `ErrNotFound`/pre-start I/O failures surface as `(-1, err)` — the same contract `proc.RunForeground` had, so the relink-heal/fallback branching is untouched. Delegation argvs (`upgradeArgv`, `--skip-brew-update` probing, `t.Install`) are byte-identical; only the transport changed.

## OSC 9;4 terminal progress

`shll update`'s write phase emits OSC 9;4 (ConEmu/Windows Terminal convention) terminal-progress sequences on **stderr**, so a progress-aware terminal — most importantly a run-kit dashboard tile, whose xterm.js renders them via `@xterm/addon-progress` — shows a live progress bar for the run. All emission logic lives in `src/cmd/shll/progress.go` (`progressReporter`); `update.go` only constructs the reporter and calls its four methods. Presentation-only: no subprocess calls (Constitution I), no state (Constitution II), never influences the exit code. (rbdd)

- **Sequence forms.** BEL-terminated: `set(percent)` → `ESC ]9;4;1;{percent} BEL` (determinate), `indeterminate()` → `…;3;0 BEL`, `errorState(percent)` → `…;2;{percent} BEL`, `remove()` → `…;0;0 BEL`. Pieces are named constants (`oscProgressPrefix`/`oscProgressSuffix`, `progressState*`).
- **Gating.** Enabled only when stderr is a real TTY, via the swappable seam `var progressWriterIsTTY = defaultProgressWriterIsTTY` (an `*os.File` + `term.IsTerminal` check mirroring `defaultStdinIsTTY`). Deliberately **independent of `NO_COLOR`** — that convention governs styling, and OSC 9;4 is terminal progress *state* (same styling-vs-not line `stdinIsTTY` draws). A disabled reporter's methods write zero bytes, so pipes/CI/tests see an entirely inert feature.
- **stderr, not stdout.** The framing headers/tail stay on stdout (stream discipline with the streamed sub-tool output); the invisible OSC control channel rides stderr so it never lands in piped stdout while still reaching the terminal.
- **tmux passthrough.** When `env("TMUX")` is non-empty (the same injected `env` seam `runUpdate` threads), every sequence is wrapped in the DCS envelope `ESC Ptmux; {sequence with each ESC doubled} ESC \` so it survives tmux and reaches the outer terminal — run-kit sessions run `allow-passthrough on`, so rk tiles receive it.
- **Lifecycle.** Constructed on stderr at write-phase start (with `start := nowFunc()`, i.e. after the dry-run return) with an immediate `defer remove()` — every post-construction exit (brew-update failure, success, panic) clears the terminal's progress state. `indeterminate()` covers the run-wide `brew update --quiet`; the `updateHeader` closure emits `set((pos-1)*100/total)` at each tool boundary; each failure site (self-upgrade or `upgradeTool`, error or non-zero exit) pulses `errorState(pos*100/total)` — the slot-consumed percent, so the next header's `set` resumes monotonically at the same value; after the loop the tail emits `errorState(100)` when `anyFailed` else `set(100)`, holding through the agent-skill refresh and digest until the deferred `remove()` fires at return.
- **No emission on non-write paths.** Dry-run, the nothing-to-do short-circuit, and every pre-write error return never construct the reporter, so they emit no OSC bytes.

Pinned by `progress_test.go` (byte-exact sequence forms, disabled no-op, `bytes.Buffer` constructor disablement, NO_COLOR independence, tmux ESC-doubling) and the `TestUpdate_Progress*` wiring tests (see [Test seam](#test-seam)).

## `--dry-run`

`shll update --dry-run` previews the exact commands the run **would** execute, then exits 0 **without any write**. The flag is a cobra bool (`dryRunFlag = "dry-run"`, usage `dryRunFlagUsage`, both named constants in `update.go`), wired in `newUpdateCmd` and read in `RunE` into the new `dryRun bool` parameter on `runUpdate`.

**Reads run; writes do not — the safety contract.** Dry-run is *not* a no-op: the read-only probes the command already runs still run (they are reads, and the preview accuracy depends on them) — `hasBrew`, the full `probeRoster` (install detection via `brew list --formula --versions` or the delegated `rk desktop status` Probe spec, plus the `<tool> update --help` `--skip-brew-update` capability check for brew-managed tools), and the shll-self `brew list`. But **no write** is performed below the probe phase: NO `brew update --quiet` (it mutates brew's local metadata — itself a side effect), NO `brew upgrade`, NO `<tool> update` / `rk desktop update`. The guarantee is **structural**: the dry-run branch (`update.go`) returns before `start := nowFunc()` and the whole write phase, so no write path is reachable. `TestUpdate_DryRunNoWrites` asserts both directions — the read-only probes (`brew list`, a `<tool> update --help`) ARE recorded, while `brew update --quiet`, `brew upgrade shllFormula`, every `<tool> update` write, and every `brew upgrade <formula>` are NOT — and additionally asserts **zero `TransportForeground`/`TransportStreamTail` calls** (all writes ride one of the two write transports, so their absence is a clean structural check).

**The preview.** A header line `Would update N tools (brew metadata refresh first):` (`updatePreviewHeaderFmt`), then one aligned row per actionable tool. The "brew metadata refresh first" annotation reflects that the *real* run calls `brew update --quiet` once up front — but dry-run does NOT run it. Rows are built in `runUpdate` (`update.go`) from probe results: `shll (self)` first when brew-installed (`brew upgrade sahil87/tap/shll`), then each installed roster tool in roster order. The per-tool command string is `argvString(upgradeArgv(t, probes[i].supportsSkipFlag)...)` — i.e. rendered from the **same `upgradeArgv` the live run uses** (`update.go`, the single source of truth shared by `upgradeTool` and the preview), so the preview can never drift from what the run would do. Per-tool argv dispatch:

- delegated (non-brew) tool → its `Update` argv verbatim (`rk desktop update`)
- has `Update` argv + supports the flag → `<tool> update --skip-brew-update`
- has `Update` argv, no flag (version skew) → `<tool> update`
- no `Update` argv (hypothetical future brew-managed tool) → `brew upgrade sahil87/tap/<formula>`
- `shll (self)` (when brew-installed) → `brew upgrade sahil87/tap/shll`

Formatting lives in `ui.go`'s `printUpdatePreview` → `printPreviewRows`: a 2-space row indent (`previewIndent`), tool labels left-padded to the **longest label present** (including `shll (self)`, the widest at 11 chars when present), then a 2-space gap (`previewGap`) before the command — so commands line up in a readable column. The preview rows carry **no `[N/M]` counter and no blank-line spacing** — those are streaming-loop concerns; the preview is a static aligned table.

```
Would update 7 tools (brew metadata refresh first):
  shll (self)  brew upgrade sahil87/tap/shll
  run-kit      run-kit update
  fab-kit      fab-kit update
  wt           wt update
  idea         idea update
  tu           tu update
  hop          hop update
```

(`TestUpdate_DryRunPreviewWithSelf` golden — shll brew-installed, the six brew-managed tools present, rk-desktop absent, no tool advertises the flag.)

**Graceful degradation (Constitution V).** The preview lists only actionable tools — uninstalled roster tools are omitted, exactly as they are skipped in the real upgrade loop. With only `hop` and `wt` installed and shll not brew-installed, the preview is exactly those two in roster order (`wt` then `hop`), header `Would update 2 tools (brew metadata refresh first):` (`TestUpdate_DryRunGracefulDegradation`). `TestUpdate_DryRunPreview` covers the six brew-managed tools with shll *not* brew-installed and `run-kit`/`hop` advertising the flag (so they read `… update --skip-brew-update`); `TestUpdate_RkDesktopDryRunPreview` pins that an installed rk-desktop previews its delegated `rk desktop update` row without executing it.

**Empty case.** When nothing is installed AND shll itself is not brew-installed, the dry-run path never reaches the preview builder — the shared nothing-to-do short-circuit (step 5) fires first, so stdout is exactly `Checking installed shll tools…\nNo shll tools installed.\n` (the `noToolsInstalledMsg` constant, shared with the non-dry-run short-circuit), exit 0, no preview table, no `brew update` (`TestUpdate_DryRunEmptyCase`).

**Brew-missing precondition unchanged.** `--dry-run` does not relax the `hasBrew` bail — a missing brew still writes `brewMissingHint` to stderr and exits 1 (the brew-missing check in `runUpdate` precedes the dry-run branch).

Exit code: always 0 in dry-run (no writes, nothing can fail) except the brew-missing precondition (exit 1).

## End-of-run agent-skill refresh + `--yes`

When a prior `shll setup agent` placement exists (`agentSkillPlacementState(env)` — ANY skill target file present under `$HOME`), `shll update` ends the write phase by re-running `shll setup agent` as a **subprocess** resolved from PATH (`refreshPlacedAgentSkills` in `agent_setup.go`, header `Refreshing placed agent skills (shll setup agent)…`). A subprocess, not an in-process call, because after a brew self-upgrade the RUNNING binary still holds the OLD embedded skill content — only the freshly installed binary on PATH can place the new bytes. It runs AFTER the roster loop so the run-kit hook delegation inside the refresh uses the just-upgraded run-kit. No placement → no unsolicited writes (silent skip); shll off PATH (dev build) → silent skip; any other failure warns `(continuing)` and never changes the exit code. Pinned by `TestUpdate_RefreshesPlacedAgentSkills`, `TestUpdate_NoPlacementSkipsRefresh`, `TestUpdate_RefreshFailureWarnsAndContinues`, `TestUpdate_RefreshShllNotOnPathSkipsSilently`.

**Cross-release compat.** An OLD running binary's `refreshArgv` composes `shll agent-setup [--yes]` and executes it against the NEW binary on PATH after the brew self-upgrade — so the new binary keeps `shll agent-setup` registered as a hidden, silent top-level command (delegating to the same internals) for one release cycle. See [cli/setup §the update self-refresh argv](/cli/setup.md#the-update-self-refresh-argv-refreshargv).

**`--yes`/`-y` (3ovi).** `shll update` accepts the shared `yesFlag`/`yesFlagShorthand` flag (usage string `updateYesUsage`), threaded as `yes bool` through `runUpdate`. Its **only consumption point** is this refresh: `refreshArgv(yes)` (`agent_setup.go`) builds `shll setup agent [--yes]`, so `shll update --yes` makes the refresh forward `--yes` onward to the `run-kit agent setup` delegation — the unattended-run consent chain that keeps run-kit's `Write these changes? [y/N]` hook prompt from hanging a nobody-attached pane (see [cli/setup §run-kit delegation](/cli/setup.md#run-kit-delegation) and its explicit-plumbing Design Decision). The per-tool delegated `<tool> update [--skip-brew-update]` argvs and the shll self-upgrade `brew upgrade` are **untouched** by the flag — they are already bound prompt-free by the update standard (`docs/site/standards/update.md` § Prompt-free, unconditionally). `refreshArgv` is the single source of truth shared by the live subprocess and the dry-run preview line `Then: %s (refresh placed agent skills)` (`updatePreviewSkillRefreshFmt`) — under `--yes --dry-run` the preview reads `Then: shll setup agent --yes (refresh placed agent skills)`; the preview prints under the same placement gate as the live refresh. Pinned by `TestUpdate_YesThreadsIntoRefresh`, `TestUpdate_YesLeavesToolArgvsUntouched`, `TestUpdate_DryRunPreviewsYesRefresh` (+ the no-flag `TestUpdate_DryRunPreviewsSkillRefresh` / `TestUpdate_DryRunNoPlacementOmitsRefreshLine`), and `TestUpdate_YesFlagWiredThroughCobra` (flag wiring + the `--skip-brew-update` help literal intact).

The downstream consumer is run-kit's dashboard update button (`handleShllUpdate` in run-kit's `app/backend/api/update.go`), which appends `--yes` to its unattended `shll update` job argv — run-kit-repo change, out of shll's scope.

## Roster order

`shll update` probes and upgrades in `Roster` order (step 8 iterates `Roster`) — **importance-descending with dependency adjacency**: `run-kit, rk-desktop, fab-kit, wt, idea, tu, hop`. The tools a user reaches for first lead the list, and a tool sits immediately after the runtime it depends on (rk-desktop directly after run-kit, whose `rk desktop …` subcommands it delegates to). With shll itself brew-installed and the six brew-managed tools present (rk-desktop absent), the per-tool headers print as `==> [1/7] shll (self)` then `==> [2/7] run-kit`, `==> [3/7] fab-kit`, `==> [4/7] wt`, `==> [5/7] idea`, `==> [6/7] tu`, `==> [7/7] hop` (the `[N/M]` counters, 6vuo), each header after the first preceded by a blank line, with the `Done — 7 of 7 tools succeeded in 1m12s.` duration-bearing tail (`TestUpdate_HeadersAndTail` golden — see [Worked header example](#worked-header-example)).

This ordering is **output coherence and meaning**, not correctness: brew owns formula-dependency resolution (correct and idempotent regardless of walk order), each `<tool> update` is self-update-only (no tool's `update` cascades into another tool's upgrade during `shll update`), and each tool's `==> <tool>` section completes and is counted in the summary tail under its own header before the next tool runs. The exact order is enforced by `TestRosterOrder` (`src/cmd/shll/tools_test.go`) — a comment cannot fail CI, so the test guards against an accidental reorder. The roster declaration, the seam fields, and the shared ordering contract live in [cli/commands §hardcoded tool roster](/cli/commands.md#hardcoded-tool-roster). The order-independent update invariants (brew-missing bail, status line, single `brew update`, self-upgrade-before-roster, best-effort loop, summary tail, exit codes) are unaffected by the order; `TestUpdate_SelfUpgradeOrdering`/`TestUpdate_OneUpgradeFails` reference `Roster[0]` (run-kit) dynamically. (t26g)

## Spec-locked Design Decisions for this subcommand

These lock the contract. #2/#3/#9 come from the original `update` spec; the delegation/probe/parallel-probe decisions from cczs; the header/tail/stream-discipline contract from y630.

### #2 Installed detection via `brew list`, not symlink resolution

> *Why*: `brew list --formula --versions sahil87/tap/<formula>` is the right primitive for querying *other* tools' install status. Hop's `/Cellar/` symlink trick works for the running tool only.
> *Rejected*: parsing plain `brew list` output (regex-fragile, see code-quality.md anti-pattern); inspecting filesystem paths directly (Constitution-violating hardcoded `/opt/homebrew` style paths).

### #3 Sequential brew upgrades (upgrade-scoped)

> *Why*: Brew serializes most internal operations behind its own lock; parallelism risks confusing interleaved output and lock contention with no measurable speedup.
> *Rejected*: parallel goroutine-per-tool *upgrades*. Real brew operations are I/O-bound on the single brew lock, so concurrency would not help.

Scope note (cczs): this decision applies to **upgrades**. Read-only probes are explicitly carved out and run concurrently (see "Sequential, not parallel — scoped to upgrades" above).

### #9 `shll update` skips `brew update --quiet` when there is nothing to upgrade

> *Why*: The metadata refresh is only useful as a precursor to upgrades. When there is nothing to upgrade (no roster tools installed AND shll itself not brew-installed), the refresh is pure latency for no benefit; the user-visible message (`No shll tools installed.`) is the primary signal and should print quickly.
> *Rejected*: refreshing brew metadata anyway. Considered for "freshness on every invocation" but rejected — `shll update` is not a brew metadata refresh tool, it's a shll toolkit upgrader. Users who want a refresh have `brew update` directly.

This is the reason for the early short-circuit in step 5 above. The check is a logical AND — both the roster set and shll-itself must be empty/uninstalled — so a brew-installed shll with zero roster tools still proceeds (and just self-upgrades). The status line (step 2) still prints first, so DD#9 only suppresses `brew update`, not the status line. Tests assert `brew update` is NOT in the recorded call list when the full nothing-to-do branch fires (`TestUpdate_NoToolsInstalled`).

### Delegate to `<tool> update`, not `brew upgrade <formula>`

> *Why*: Preserves each tool's post-upgrade side effects (run-kit's daemon restart), satisfying Constitution IV. `brew upgrade` alone reproduces only the binary swap, not the tool's own post-upgrade logic.
> *Rejected*: hardcoding run-kit's daemon restart into shll (Principle IV smell, doesn't generalize); documenting the gap as a known limitation (leaves the correctness bug live).

### Hoist `brew update --quiet` into shll once, via `--skip-brew-update`

> *Why*: Each tool's `update` would otherwise run its own `brew update`, causing N redundant metadata refreshes. The flag lets shll do it once for the whole run.
> *Rejected*: letting each tool refresh independently (N× latency); having shll suppress refresh by other means (no cross-tool contract).

### Retire the rk→run-kit brew-formula migration guard

> **Decision**: Every roster tool, run-kit included, takes the plain single-probe delegated-upgrade path — there is no legacy-keg detection, no brew-direct migration action, and no dual-rack note in `shll update`.
> **Why**: The migration window is closed. The tap's `formula_renames.json` rename mapping is permanent by design (removing it would strand pre-rename kegs), so `brew` prints `Warning: Formula sahil87/tap/rk was renamed …` on every reference to the legacy formula — and the guard referenced it on every run (even on fully-migrated machines, where the probe existed only for dual-rack detection). `internal/proc`'s capture transport streams subprocess stderr through, so that warning landed in the user's face on every `shll update`. Retiring the guard removes both the noise and ~350 lines of transitional classification machinery at the root.
> **Rejected**: Silencing only the probe's stderr (`proc.RunCaptured`) — keeps the dead machinery alive and still runs a pointless probe on every migrated machine. Removing the tap's rename mapping — would strand a straggler's pre-rename keg with "formula not found".
> **Migration**: Stragglers on a legacy-`rk`-only machine follow run-kit's README manual path (`brew uninstall rk`, then `shll install`); shll no longer automates it.
> *Introduced by*: 260720-h3f6-retire-rk-migration-guard

### Generic delegation-failure fallback over version-pinned knowledge

> **Decision**: When a delegated `<tool> update` fails, fall back once to `brew upgrade <formula>` — for any roster tool, with no knowledge of which tool/version is broken.
> **Why**: Rescues the live idea ≤ 0.1.2 self-update catch-22 (the old binary SIGKILLed its own brew child at 120s, so it could never upgrade past its own bug) and any future tool that ships a broken `update`. shll's brew calls carry no deadline, so the outside upgrade succeeds where the tool's own could not.
> **Rejected**: Hardcoding "idea ≤ 0.1.2 is broken" in shll — covers one incident, requires a shll release per future incident, and rots.
> *Introduced by*: 260812-blht-delegated-update-brew-fallback

### Progress emission is consumer-side, not a toolkit standard

> **Decision**: OSC 9;4 progress is implemented directly in `shll update` (the compose consumer); the producer-facing `update` standard (`docs/site/standards/update.md`) is untouched and no roster tool is asked to emit anything.
> **Why**: OSC 9;4 is a singleton terminal channel — one progress state per terminal. A delegated tool's own emission inside the compose would conflict (a tool's terminal `remove` clears shll's roster-level bar mid-loop). Only the orchestrator knows "tool N of M", which is the signal the run-kit tile consumer wants; the standard already assigns consumer-side compose behavior to shll.
> **Rejected**: A producer-standard clause obligating each roster tool to emit — requires a remove/re-assert coordination protocol plus a 6-repo rollout while no tool emits today. Standardize from working practice only when a second emitter appears.
> *Introduced by*: 260819-rbdd-update-osc-progress

### Progress on stderr, TTY-gated, independent of NO_COLOR

> **Decision**: Progress sequences ride stderr, enabled only when stderr is a real TTY (`progressWriterIsTTY` seam), and the gate does not consult `NO_COLOR`.
> **Why**: Headers/tail must stay on stdout (stream discipline with the streamed output); OSC is an invisible control channel and stderr keeps it out of piped stdout while still reaching the terminal. `NO_COLOR` governs styling per no-color.org — the codebase already draws the styling-vs-not line at `defaultStdinIsTTY`, and the TTY gate alone excludes pipes/CI.
> **Rejected**: stdout emission (couples to the color gate and risks captured-output pollution); honoring `NO_COLOR` (wrong scope — a styling convention gating non-styling terminal state).
> *Introduced by*: 260819-rbdd-update-osc-progress

### Linear streamed output, not a pinned-region tty layout

> **Decision**: The install/update write phases render as one linear stream — status line, per-tool `▸ [N/M]` headers, live child output, summary tail, digest — with no pinned status header or DECSTBM scroll region.
> **Why**: A two-region tty layout (pinned header above a DECSTBM scroll region) is acceptable only if absolutely error-free, and it was not: live runs produced corrupted output (header repaints re-parked the cursor at the region top and overprinted prior rows without erasing; child tee writes raced the repaint mutex) and region-scrolled lines never reached terminal scrollback, losing the run log an update tool's failures live in. The linear stream has none of these failure modes and keeps the full transcript in scrollback.
> **Rejected**: fixing the cursor discipline in place (DECSC/DECRC save/restore) and an apt-style bottom-pinned status bar — both keep terminal-state fragility (tmux, SSH, resize races) and the DECSTBM scrollback loss.
> *Introduced by*: 260821-0ia2-revert-two-region-linear-output

### Tee-with-pipe transport for write-phase children, not pty and not inherited fds

> **Decision**: Write-phase children get piped stdout/stderr tee'd live to the run's writers plus a bounded tail ring (`TransportStreamTail`); they do not inherit the terminal fds.
> **Why**: A tee streams child output live in one linear stream with full scrollback (no output withholding) while giving the transport a capture seam (the bounded tail ring). Children seeing a pipe degrade their own tty-only rendering (e.g. brew progress bars) — accepted: the toolkit standard already requires children to behave non-interactively.
> **Rejected**: pty allocation (new dependency, platform surface, overkill for a framing feature); inherited fds (no capture seam, and child rendering would assume an interactive terminal the prompt-free standard forbids).
> *Introduced by*: 260820-yud0-install-update-terminal-ux

### Stdin hardening scoped to the write-phase children only

> **Decision**: The null-stdin transport covers brew and roster-tool children in install/update; the end-of-run agent-skill refresh subprocess keeps `proc.RunForeground` (inherited stdin).
> **Why**: The refresh re-runs `shll setup agent`, whose run-kit hook delegation legitimately confirms interactively when `--yes` was not passed — null stdin would break the interactive path that exists by design.
> **Rejected**: hardening every subprocess in the two commands uniformly (breaks the documented interactive refresh contract).
> *Introduced by*: 260820-yud0-install-update-terminal-ux

### Fallback on any failure, ordered after the relink heal

> **Decision**: The fallback triggers on any failure (non-zero exit or exec/transport error) of the *final* delegated outcome, evaluated after the ErrNotFound → `brew link` → retry heal.
> **Why**: One coherent rule; also rescues corrupted-binary cases where the delegated binary cannot exec at all. The relink heal stays first because it is the more specific remedy.
> **Rejected**: A non-zero-exit-only trigger — leaves the exec-error family (broken binary) unrescued for no simplicity gain.
> *Introduced by*: 260812-blht-delegated-update-brew-fallback

### A delegated tool's update is its `Update` argv verbatim — no brew safety net

> **Decision**: For a delegated (non-brew) roster entry, `upgradeArgv` returns the `Update` argv verbatim — no `--skip-brew-update` capability probe, no flag append — and both delegation-path self-heals (the unlinked-keg relink heal, the delegation-failure brew-upgrade fallback) are gated on `t.brewManaged()`. A delegated tool's update failure surfaces as that tool's failure: no relink note, no fallback note, no brew call.
> **Why**: There is no formula and no keg, so a brew fallback or relink heal is meaningless and would mask the real error. The `--skip-brew-update` flag exists to skip a tool's internal brew refresh; a non-brew update (`rk desktop update` downloads a DMG) never runs one, so probing for the flag would be noise.
> **Rejected**: A `brew upgrade` fallback for a delegated tool (no formula exists); help-probing a delegated tool for the flag (a brew-specific capability applied to a non-brew update).
> *Introduced by*: 260820-t26g-roster-desktop-entry

## Test seam

All `update_test.go` tests inject a fake via `proc.Runner` (`installFakeRunner` t.Cleanup helper in `src/cmd/shll/update_test.go`). No real brew or sub-tool subprocess is ever spawned. The fake records every `proc.Request` so tests assert: which formulas were queried, which `--help` probes ran, which upgrades ran (delegated vs. brew-upgrade), the order of operations, the exit code, and the captured stdout/stderr writers.

**Goroutine-safety (cczs).** Because `probeRoster` dispatches its probes concurrently, the fake is concurrency-safe: a `sync.Mutex` (`fakeRunner.mu`) guards both the `calls` slice and the `respond` dispatch, so concurrent probe calls do not race. Tests assert against a stable snapshot via `recordedCalls()` (`src/cmd/shll/update_test.go`), called *after* `runUpdate` returns (all probes have joined). `go test -race` is clean. Respond functions run **under `mu`**, so they must not call back into the runner. Helpers: `helpAdvertisesSkipFlag()` (returns help output containing the flag substring), `isUpdateHelpProbe(req)` (identifies a `<tool> update --help` probe by its trailing `--help` arg), `installedOnly(formulas...)` (a respond function where only the named formulas report installed and shll-self is not-brewed), and the rk-desktop probe helpers `isRkDesktopProbe(req)` (matches the delegated `rk desktop status` argv), `rkDesktopStatusResult(installed bool)` (renders the Probe spec's `Installed:` line), and `rkDesktopRefusalResult` (run-kit's macOS-only refusal on stderr). `installedOnly` answers the rk-desktop probe installed=true by default (an empty-success probe answer reads as installed); tests exercising the skip/gate paths intercept `isRkDesktopProbe` first and answer `rkDesktopStatusResult(false)` to keep rk-desktop out of their actionable sets and goldens.

Covered scenarios (`src/cmd/shll/update_test.go`):

- `TestUpdate_BrewMissing` — `proc.Run("brew", "--version")` returns `ErrNotFound` → stderr hint, **empty stdout** (status line not yet printed), exit 1.
- `TestUpdate_NoToolsInstalled` — neither shll nor any roster tool installed → stdout is exactly `Checking installed shll tools…\nNo shll tools installed.\n`, **no `brew update`**, no upgrade calls, exit 0.
- `TestUpdate_AllInstalled` — shll itself + full roster installed, help advertises no flag → `brew update --quiet`, self-upgrade, each brew-managed roster tool delegated via `<tool> update` (no flag) and rk-desktop via its delegated `rk desktop update` (no help probe), and NOT `brew upgrade <formula>`, exit 0.
- `TestUpdate_SelfUpgradeOrdering` — pin that the shll self-upgrade (`brew upgrade shllFormula`) appears before the first roster *upgrade* in the recorded sequence (excluding the concurrent `<tool> update --help` probe).
- `TestUpdate_SelfNotBrewInstalled` — dev build (shll not brew-installed) → self-upgrade skipped, roster still delegated via `<tool> update`.
- `TestUpdate_OnlyShllInstalled` — shll brew-installed but no roster tools → metadata refresh runs, self-upgrade runs, no roster delegation/upgrade, no short-circuit message, exit 0.
- `TestUpdate_PartialInstalled` — only `hop` and `wt` installed → only those delegated via `<tool> update`; uninstalled tools neither delegated nor brew-upgraded; the `--help` probe is issued **only** for installed tools (`hop`/`wt` probed; `idea`/`fab-kit` not probed).
- `TestUpdate_BrewUpdateFails` — `brew update --quiet` exits non-zero → stderr "brew update failed", no upgrade attempted (delegated or fallback), exit 1.
- `TestUpdate_OneUpgradeFails` — first roster tool's delegated `update` AND its fallback `brew upgrade` exit non-zero → loop continues; every roster delegation (including `rk desktop update`) + the self-upgrade + the one fallback are attempted, exit 1.
- `TestUpdate_FlagSupported` — `run-kit` installed and `run-kit update --help` advertises `--skip-brew-update` → upgraded via `run-kit update --skip-brew-update`, NOT `brew upgrade sahil87/tap/run-kit`, and NOT a bare `run-kit update`.
- `TestUpdate_FlagUnsupportedVersionSkew` — `hop` installed but its `--help` lacks the flag → upgraded via bare `hop update` (no flag), and does NOT fall back to `brew upgrade hop`.
- `TestUpdate_NoUpdateArgvFallsBackToBrew` — a temporary single-entry roster with a `legacy` tool that has an empty `Update` argv → falls back to `brew upgrade <formula>`; no delegated update, no `--help` probe.
- `TestUpdate_StatusLinePrecedesProbes` — stdout starts with `updateStatusLine` + `\n` before any probe/brew output.
- `TestUpdate_BrewUpdateRunsExactlyOnce` — with `run-kit`/`hop`/`wt` installed, `brew update --quiet` runs exactly once for the whole run.
- `TestUpdate_HeadersAndTail` — shll + the six brew-managed tools installed (rk-desktop absent per its probe); asserts the verbatim `[N/M]` headers (`==> [1/7] shll (self)` first, then `run-kit`, `fab-kit`, `wt`, `idea`, `tu`, `hop`), the blank line before each subsequent header and before the tail, and the duration-bearing `Done — 7 of 7 tools succeeded in 1m12s.` tail (installs a deterministic clock).
- `TestUpdate_HeaderPrecedesOutput` *(6vuo)* — the `==> [1/1] hop` header is in the buffer before hop's streamed upgrade runs.
- `TestUpdate_PartialFailureTail` *(6vuo)* — `hop`+`wt` installed (shll not brewed → `total=2`), hop's delegated update AND its fallback fail → partial-failure tail `1 succeeded, 1 failed in 1m12s — see above.` with the duration before the em-dash; asserts the honesty constraint (no "updated"/"up-to-date").
- `TestUpdate_DelegatedFailureBrewFallbackRescues` — `idea update` exits 1 → fallback note printed (carrying `exit code 1`), `brew upgrade sahil87/tap/idea` follows the failed delegation, run exits 0.
- `TestUpdate_DelegatedFailureFallbackAlsoFails` — delegation and fallback both exit 1 → exit 1, exactly one fallback attempt.
- `TestUpdate_NoArgvFailureNoDoubleBrewUpgrade` — a no-`Update`-argv tool whose primary `brew upgrade` fails → exactly one brew upgrade, no fallback note.
- `TestUpdate_DelegationUnlinkedKegSelfHeal` / `TestUpdate_DelegationUnlinkedKegLinkFails` — the relink heal: ErrNotFound → `brew link` between the two delegation attempts, relink note on stdout, no fallback after a successful retry; a failed link skips the retry (link failure on stderr, no relink note) and the fallback then rescues the tool.
- `TestUpdate_EmptyCaseNoHeaderNoTail` *(6vuo)* — nothing installed → status line + `No shll tools installed.` only, with no `==>` header and no `Done —`/duration tail.
- `TestUpdate_DryRunPreview` *(6vuo)* — shll NOT brew-installed, the six brew-managed tools installed (rk-desktop absent), `run-kit`/`hop` advertise the flag → verbatim aligned-column preview (`Would update 6 tools (brew metadata refresh first):` then padded rows in roster order, `run-kit`/`hop` reading `… update --skip-brew-update`).
- `TestUpdate_DryRunPreviewWithSelf` *(6vuo)* — shll brew-installed + the six brew-managed tools, no flag advertised → preview lists `shll (self)` first (`brew upgrade sahil87/tap/shll`) then the roster order (`run-kit`, `fab-kit`, `wt`, `idea`, `tu`, `hop`), `shll (self)` is the widest label so all commands align under it.
- `TestUpdate_DryRunNoWrites` *(6vuo)* — read-only probes (`brew list`, a `<tool> update --help`) ARE recorded; `brew update --quiet`/`brew upgrade`/every `<tool> update`/every `brew upgrade <formula>` are NOT; and **zero** `TransportForeground`/`TransportStreamTail` calls.
- `TestUpdate_DryRunGracefulDegradation` *(6vuo)* — only `hop`+`wt` installed → preview lists exactly `wt`, `hop` (roster order), header `Would update 2 tools (brew metadata refresh first):`.
- `TestUpdate_DryRunEmptyCase` *(6vuo)* — nothing installed → dry-run mirrors the nothing-to-do message, no preview table, no `brew update`, exit 0.
- `TestUpdate_SubsetUnknownTargetHardErrors` *(b2vg)* — `shll update <typo>` → `errSilent`, stderr names the unknown arg and lists valid targets, and **no `brew` subprocess runs** (validated before `hasBrew`/probe).
- `TestUpdate_SubsetMultipleUnknownAllReported` *(b2vg)* — multiple unknown args → all reported in one error.
- `TestUpdate_SubsetNamedNotInstalledErrors` — a valid name that is not installed (`shll update run-kit` with `sahil87/tap/run-kit` absent) → `shll update: run-kit: not installed`, `errSilent`, nothing upgraded (distinct from the whole-roster graceful skip). A legacy-`rk`-keg-only machine reports run-kit's current formula not installed and is treated identically — the graceful skip on a whole-roster run, this named error on `shll update run-kit`/`rk`.
- `TestUpdate_SubsetShllSelfTargetOnly` *(b2vg)* — `shll update shll` (shll brew-installed) → only the self-upgrade runs (`brew upgrade shllFormula`), no roster tool upgraded, `M=1`.
- `TestUpdate_SubsetShllSelfNotBrewInstalledErrors` *(b2vg)* — `shll update shll` on a dev build (shll not brew-installed) → the not-installed error for `shll`.
- `TestUpdate_SubsetSelfFirstThenRosterOrder` *(b2vg)* — `shll update shll hop` → `shll (self)` first, then `hop`.
- `TestUpdate_SubsetArgOrderIndependentRosterOrder` *(b2vg)* — `shll update fab-kit wt` → `fab-kit` before `wt` (roster order, not arg order).
- `TestUpdate_SubsetBrewUpdateRunsOnce` *(b2vg)* — a single-tool subset still runs `brew update --quiet` exactly once.
- `TestUpdate_SubsetDryRunPreviewFiltered` *(b2vg)* — `shll update --dry-run hop wt` → preview lists exactly the two-tool subset in roster order, header `Would update 2 tools (brew metadata refresh first):`, exit 0, no write.
- `TestUpdate_RkDesktop*` *(t26g)* — the delegated (non-brew) path: `TestUpdate_RkDesktopDelegatesToRkDesktopUpdate` (installed per its probe → streams `rk desktop update`; never brew-upgraded, never help-probed for `--skip-brew-update`), `TestUpdate_RkDesktopAbsentIsSkipped` (probe reports the absent value → skipped, no `rk desktop update`), `TestUpdate_RkDesktopRefusalSkipsSilently` (the probe itself refuses — `rk desktop is macOS-only` → graceful skip, exit 0), `TestUpdate_RkDesktopFailureNoBrewFallback` (`rk desktop update` exits 1 → `errSilent`, no `brew upgrade`, no `brew link`), `TestUpdate_RkDesktopDryRunPreview` (the preview renders the delegated `rk desktop update` row without executing it).
- `TestUpdate_ProgressEmissionOrder_Success` *(rbdd)* — TTY seam forced, shll-self only (`total=1`), all succeed → stderr is byte-exactly `indeterminate` → `set(0)` → `set(100)` → `remove`.
- `TestUpdate_ProgressErrorPulseAndErrorTail` *(rbdd)* — self-upgrade exits non-zero → the failure pulse `errorState(100)` and the `anyFailed` tail `errorState(100)`, then the deferred `remove`.
- `TestUpdate_ProgressRemoveOnBrewUpdateFailure` *(rbdd)* — `brew update` fails after the reporter started → `indeterminate` then the deferred `remove`, no `set`/`errorState` ever emitted.
- `TestUpdate_ProgressSilentOnNonWritePaths` *(rbdd)* — dry-run and the no-tools short-circuit emit zero OSC bytes even with the TTY seam forced.
- `TestUpdate_RefreshSubprocessKeepsForegroundTransport` *(yud0)* — the end-of-run agent-skill refresh subprocess keeps `TransportForeground` (inherited stdin) while the write-phase children ride `TransportStreamTail`.

The `progressReporter` itself (sequence forms, gating, NO_COLOR independence, tmux ESC-doubling envelope) is unit-tested in `progress_test.go`, which forces the enabled branch via the `forceProgressTTY` t.Cleanup helper (swapping the `progressWriterIsTTY` seam).

The shared resolver is unit-tested directly in `tools_test.go` (`TestResolveTargets_RosterOrderRegardlessOfArgOrder`, `TestResolveTargets_ShllGatedByAllowShll`, `TestResolveTargets_MultipleUnknownAllReported`, `TestResolveTargets_EmptyArgs`).

Per-tool output separation (y630) plus the change-6vuo `[N/M]` counter, duration, and preview helpers are unit-tested directly against the `ui.go` helpers in `ui_test.go` (`TestPrintToolHeader_PlainForm`/`_ColorForm` now assert the `[N/M]` counter; `TestPrintSummaryTail_AllSucceeded*`/`_PartialFailure` assert the `in 1m12s` suffix; `TestFormatDuration`, `TestPrintUpdatePreview_AlignedColumns`, `TestPrintInstallPreview_AlignedColumns`, `TestColorEnabled_*`, `TestToolComment_*`); the clock seam helper is exercised by `TestInstallFakeClock_Sequences` (`clock_test.go`). `update_test.go` additionally asserts that the `==> [N/M] shll (self)` and per-tool `==> [N/M] <tool>` headers and the plain summary tail appear in the **stdout** buffer and never in the stderr buffer (the stream-discipline guarantee).

## Cross-references

- The "What changed:" digest's release fetch/filter (concurrent, degradation): [internal/changelog](/internal/changelog.md). The shared release rendering (`renderReleases`) the digest and `shll changelog` both use: [cli/changelog](/cli/changelog.md#output-shape).
- Subprocess wrapper conventions: [internal/proc](/internal/proc.md).
- The hardcoded roster — the new importance-descending order, the `Update` capability field, and the delegated (non-brew) `Install`/`Probe` seam fields: [cli/commands](/cli/commands.md#hardcoded-tool-roster).
- The shared delegated installed-probe (`probeToolVersion` / `probeToolInstalledVersion` / `parseProbeStatusLine`) the update probe and version-bump re-query ride on: [cli/version §the shared install probe](/cli/version.md#the-shared-install-probe).
- The shared `shllSelf` display descriptor (bb7r): [cli/commands §the shared `shllSelf` descriptor](/cli/commands.md#the-shared-shllself-descriptor). `update`'s `shll (self)` first step is the established manage-side pattern that descriptor generalizes to `list`/`doctor`/`install`; `update.go` itself is unchanged (self-upgrade stays a dedicated `brew upgrade`, not a `shllSelf` consumer).
- Shared UI helper (`ui.go`) for the header/tail/color logic: [cli/commands](/cli/commands.md#file-layout-srccmdshll); the sibling [cli/install](/cli/install.md#per-tool-output-separation) mirrors this header/tail behavior. The null-stdin streamed-tail transport lives in [internal/proc §TransportStreamTail](/internal/proc.md#transportstreamtail-used-by-procrunstreamedtail), consumed via the shared `runStreamedChild` helper (`brew.go`).
- Constitution III (Wrap, Don't Reinvent) and IV (Composition, Not Replacement) — the delegation in step 8 is the direct expression of both.
- Constitution V (Graceful Degradation) — uninstalled tools are skipped during probing; version-skew tools degrade to a flagless `<tool> update`.
