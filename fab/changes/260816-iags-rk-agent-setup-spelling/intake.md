# Intake: Switch run-kit delegation to the `rk agent setup` spelling

**Change**: 260816-iags-rk-agent-setup-spelling
**Created**: 2026-08-16

## Origin

Backlog item `[iags]` (2026-08-16), one-shot via `/fab-new iags`:

> shll agent-setup delegates via the deprecated `run-kit agent-setup` spelling — rk renamed the command family to `rk agent setup` (run-kit PR #620), so every shll agent-setup / update-refresh run surfaces a deprecation warning to users. Switch delegateRunKitAgentSetup and refreshPlacedAgentSkills' delegation to the new spelling once the minimum rk version carrying `rk agent setup` is settled; decide whether to probe/fall back for older rk (Constitution V)

Intake-time research settled the two open points the backlog names:

- **Minimum rk version**: run-kit PR #620 ("refactor: The rk agent family — `agent setup` / `agent hook`") merged 2026-08-15 11:17 UTC; tag-containment check (`compare/v3.16.22...d6480436` = ahead, `v3.16.23` = behind) shows the new spelling first shipped in **run-kit v3.16.23** (released 2026-08-15).
- **Live behavior verified** on installed run-kit v3.16.25: `run-kit agent setup --help` works with the same flag surface (`--uninstall`, `--yes`/`-y`, `--dry-run`); `run-kit agent-setup` still works but prints `Command "agent-setup" is deprecated, use `rk agent setup` instead` — the warning users currently see on every `shll agent-setup` / `shll update` refresh.
- **Probe/fallback decision**: no probe, no fallback (see Why and Assumptions).

## Why

1. **Pain point**: `shll agent-setup` (and transitively the end-of-run refresh in `shll update`) invokes `run-kit agent-setup`, the deprecated spelling. On any current rk (≥ v3.16.23) the delegation's inherited stdio surfaces rk's deprecation warning to the user on every run — noise that makes shll look out of date and will break outright whenever run-kit eventually removes the deprecated alias.

2. **If not fixed**: every `shll agent-setup` and every placement-gated `shll update` refresh keeps printing the deprecation line; when run-kit retires the alias, the hook-wiring delegation silently degrades to a warn-and-continue failure on every run.

3. **Approach**: switch the delegation argv to the new two-token spelling (`run-kit agent setup [--uninstall] [--yes]`), with **no version probe and no old-spelling fallback**. Reasoning:
   - The delegation is already a best-effort adjunct: `proc.ErrNotFound` → silent skip (Constitution V), any other error/non-zero exit → warn `(continuing)`, never failing the skill placement. An rk older than v3.16.23 fails with cobra's `unknown command "agent"` on inherited stderr plus shll's existing `(continuing)` warning — visible, non-fatal, and self-healing (see next point).
   - In the main exposure path (`shll update`), the agent-skill refresh deliberately runs **after** the roster loop, so rk has just been upgraded to latest before the delegation runs — the new spelling exists by construction.
   - Fresh machines get rk via `brew install`, which installs the latest release.
   - A retry-with-old-spelling fallback cannot distinguish "unknown command" from a genuine setup failure via the exit code, so it would re-run a genuinely failing (and possibly prompting) setup twice. A `--help` capability probe would add a subprocess to every run to protect a one-day-old version boundary. Both are over-engineering for a best-effort adjunct.
   - Constitution V's hard requirement (skip when the sub-tool is **absent**) is untouched — the `ErrNotFound` silent-skip path stays exactly as is.

## What Changes

All changes are in `src/cmd/shll/agent_setup.go` and `src/cmd/shll/agent_setup_test.go`. No new subcommand, no flag changes, no behavior change beyond the delegated argv and diagnostic wording.

### 1. Delegation argv: `agent-setup` → `agent setup` (two tokens)

`delegateRunKitAgentSetup` currently builds:

```go
args := []string{agentSetupSub}          // "agent-setup"
if uninstall { args = append(args, "--uninstall") }
if yes       { args = append(args, "--"+yesFlag) }
code, err := proc.RunForeground(ctx, runKitToolName, args...)
```

It changes to lead with the two-token family, e.g. via a new named constant pair (code-quality: no magic strings):

```go
// runKitAgentSetupArgs is the run-kit agent-setup command family in its
// post-PR-#620 spelling: `run-kit agent setup` (two tokens, min rk v3.16.23).
var runKitAgentSetupArgs = []string{"agent", "setup"}
```

and `args := append([]string{}, runKitAgentSetupArgs...)` (or equivalent) before the flag appends. Flags `--uninstall` and `--yes` are unchanged — verified identical on the new family.

### 2. `agentSetupSub` scope narrows to shll-self

`agentSetupSub = "agent-setup"` remains — it is still shll's OWN subcommand name (`Use: "agent-setup"` in the cobra factory, and `refreshArgv`'s `shll agent-setup [--yes]` self-invocation). Its doc comment currently says it is "shared by the run-kit hook-wiring delegation and by `shll update`'s self-refresh subprocess" — update the comment to reflect that the run-kit delegation no longer uses it.

**`refreshPlacedAgentSkills` needs no code change** (the backlog names it, but its delegation is `shll agent-setup` — shll's own, un-renamed subcommand, via `refreshArgv`). It re-execs the freshly installed shll binary, which picks up the new run-kit argv transitively. Record this as scope clarification, not an omission.

### 3. Diagnostic and doc strings

- The two stderr warnings in `delegateRunKitAgentSetup` (`"%s: run-kit agent-setup: %v (continuing)"` and `"%s: run-kit agent-setup exited %d (continuing)"`) change to the new spelling `run-kit agent setup` so the diagnostic names the command actually run.
- The cobra `Long` help text and file-level comments in `agent_setup.go` that say ``delegate … to `run-kit agent-setup` `` update to `run-kit agent setup`.

### 4. Tests

`agent_setup_test.go` assertions on the fake-runner recorded calls update from `len(c.Args) == 1 && c.Args[0] == agentSetupSub` (and the 2/3-arg `--uninstall`/`--yes` variants) to the two-token prefix `["agent", "setup", ...]`:

- install delegation (`~line 214`)
- uninstall delegation (`~line 280`)
- `TestAgentSetup_YesForwardsToDelegation` (`~line 510`)
- `TestAgentSetup_YesRidesUninstallDelegation` (`~line 530`)
- `TestAgentSetup_YesFlagWiredThroughCobra` (`~line 593`)
- the stderr-string assertions (`~lines 238, 264`) follow the new diagnostic wording

## Affected Memory

- `cli/agent-setup`: (modify) frontmatter description and the "run-kit delegation" section currently name `run-kit agent-setup`; update to the `run-kit agent setup` (two-token) spelling, note the min rk version v3.16.23 and the no-probe/no-fallback decision (Design Decision entry).
- `cli/commands`: (modify) spelling sweep — names the `run-kit agent-setup` delegation.
- `cli/update`: (modify) spelling sweep — names the delegation in the refresh section.
- `cli/standards-content`: (modify) spelling sweep — names the delegation.
- `internal/proc`: (modify) spelling sweep — names the delegation as a RunForeground consumer.

## Impact

- `src/cmd/shll/agent_setup.go` — delegation argv, constant doc comment, diagnostics, help text, file-header comment.
- `src/cmd/shll/agent_setup_test.go` — delegation argv + stderr assertions.
- No other Go files: `refreshArgv`/`update.go`/`install.go`/`doctor.go` all reference `shll agent-setup` (shll's own subcommand), which is not renamed.
- Runtime dependency note: hook wiring now requires rk ≥ v3.16.23 (2026-08-15); older rk degrades to the existing warn-and-continue path.

## Open Questions

- None — the backlog's two open points (minimum rk version; probe/fallback) were settled at intake with live verification and release-tag containment checks.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | No version probe and no old-spelling fallback for rk < v3.16.23 — rely on the existing best-effort warn-and-continue delegation | Backlog explicitly delegates this decision; delegation is already a non-fatal adjunct, `shll update` upgrades rk before the refresh, brew installs latest, and a blind retry can't distinguish "unknown command" from a real failure. Easily reversed later if field reports demand it | S:60 R:85 A:75 D:65 |
| 2 | Certain | `refreshPlacedAgentSkills` needs no code change — its `shll agent-setup` self-invocation is unaffected; only `delegateRunKitAgentSetup`'s argv changes | Verified in source: `refreshArgv` = `shll agent-setup [--yes]` (shll's own subcommand, not renamed); the backlog's mention of both functions is satisfied transitively | S:70 R:90 A:95 D:85 |
| 3 | Certain | New spelling is the two-token argv `["agent", "setup"]` with unchanged `--uninstall`/`--yes` flags, minimum rk v3.16.23 | Verified live on installed run-kit v3.16.25 (`--help` output) and by tag-containment check against PR #620's merge commit | S:85 R:90 A:95 D:95 |
| 4 | Certain | Keep binary name `run-kit` (not the `rk` alias) as the delegation target | Roster canonical Name is `run-kit` (rk is `LegacyName`); `runKitToolName` constant already encodes this; changing it is out of scope | S:80 R:90 A:95 D:95 |

4 assumptions (3 certain, 1 confident, 0 tentative, 0 unresolved).
