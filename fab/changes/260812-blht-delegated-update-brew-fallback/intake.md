# Intake: Delegated Update Brew Fallback

**Change**: 260812-blht-delegated-update-brew-fallback
**Created**: 2026-08-12

## Origin

Conversational — emerged from a `/fab-discuss` debugging session (2026-08-12) diagnosing a recurring `shll update` failure on the `idea` roster tool.

> But this happens on another system. And I am sure there are many victims. What we could do is educate shll about this issue. The next time someone run shll update, the will get a get version of shll that knows about the issue and the fix. And the time after that that they run "shll update", idea should update for them, taking users out of this catch 22. Thoughts?

Key decisions from the discussion:

- **Root cause established first**: `idea` ≤ v0.1.2 armed `context.WithTimeout(120s)` around its own `brew upgrade` child. On machines where brew stalls (e.g. Homebrew 6's un-timed `api.github.com` call), the deadline SIGKILLs brew mid-fetch → `ERROR: brew upgrade exited with code -1` (Go's signal-kill sentinel). The fix (remove the deadlines, idea commit `874f7be`) shipped in idea v0.1.3 — but affected users are stuck in a **catch-22**: upgrading idea requires the *old* binary's brew upgrade to finish inside its own 120s deadline.
- **Generic over version-pinned**: the agent recommended, and the user approved, a *generic* fallback (delegated update failed → try `brew upgrade` directly) instead of teaching shll the specific fact "idea ≤ 0.1.2 is broken". Version-pinned knowledge of another tool's bug would be a permanent quirk covering one incident; the generic fallback rescues this case and future ones.
- **Two-run rollout accepted**: shll self-upgrades first in the roster loop but the running process keeps executing old code — so run N delivers the smarter shll, run N+1 rescues idea. This matches the user's framing exactly.
- **Reinstall escalation explicitly deferred** (see Non-Goals in What Changes): the mid-pour-kill broken-keg case (brew believes the new version is installed while the binary is broken) is out of scope for this change.

## Why

**Problem**: when a roster tool's own `update` subcommand is broken — the live case being idea ≤ v0.1.2's SIGKILL-bearing 120s timeout — `shll update` faithfully delegates to it, the delegated update fails, and the tool can never be upgraded past its own bug. Every `shll update` run repeats the failure. The user hit this on two systems, and any idea ≤ 0.1.2 install on a slow-brew machine is a standing victim.

**Consequence of not fixing**: affected users stay wedged on the broken version indefinitely. The only escapes are manual (`brew upgrade sahil87/tap/idea` by hand) or luck (one attempt completing inside 120s). Neither is discoverable from shll's output, which just reports the tool failed.

**Why this approach**: shll already has both halves of the answer. Its own brew calls carry **no deadline** (plain background context throughout — verified during diagnosis), so a direct `brew upgrade` from shll survives arbitrarily slow brew runs. And `brew upgrade sahil87/tap/<formula>` is already shll's documented fallback for roster tools with no `update` subcommand (Constitution IV), plus there is precedent for a printed self-heal in the delegation path (the unlinked-keg `brew link` retry, `relinkNoteFmt`). Extending that pattern — *delegation failed → note + one direct brew upgrade* — is composition-preserving (delegation stays the primary path), constitution-clean (Principle V best-effort), and conformant with the toolkit update standard's brew-safety clause (shll's fallback brew call has no timeout and never signals brew).

## What Changes

### `upgradeTool` fallback (src/cmd/shll/update.go)

`upgradeTool` currently returns the delegated `<tool> update`'s failure directly (after the existing unlinked-keg relink heal). New behavior, **delegation path only** (`len(t.Update) > 0` — the no-Update-argv path already *is* `brew upgrade`):

1. Run the primary delegation exactly as today, including the ErrNotFound → `brew link` → retry heal. The relink heal stays first — it is the more specific remedy.
2. If the final delegated outcome is still a failure — **any** failure: non-zero exit code *or* a transport error (covers a corrupted binary that fails to exec) — print a note to stdout and fall back **once** to `brew upgrade sahil87/tap/<formula>` via the same foreground proc call the no-argv path uses.
3. Fallback success (exit 0, nil error) → the tool counts as succeeded; the caller's existing post-success version re-query and "What changed:" digest work unchanged.
4. Fallback failure → return the fallback's code/error (the note line already documented the original delegated failure); the caller's existing `anyFailed` accounting and stderr reporting apply.

The note is a named constant mirroring `relinkNoteFmt` (code-quality.md: no magic strings), printed to stdout before the fallback runs, and it MUST carry the delegated failure detail (exit code or error text), e.g.:

```
note: idea's own update failed (exit code 1) — falling back to 'brew upgrade sahil87/tap/idea'
```

Exact wording is the implementer's choice; the required ingredients are the tool name, the delegated failure detail, and the exact fallback command.

### Why this rescues the idea catch-22

Old idea kills *its own* brew child at 120s and exits 1. shll's fallback `brew upgrade` carries no deadline, so brew can stall for minutes and still land 0.1.3. The run after that, delegation works normally again. Bounded cost for legitimate failures: when a delegated update fails for an unrelated reason *after* brew already succeeded (e.g. a post-upgrade hook error), the fallback `brew upgrade` no-ops ("already installed", exit 0) — harmless, though it converts that tool's outcome to success; the note line keeps the underlying failure visible on the terminal.

### Standards conformance note

The toolkit `update` standard (docs/site/standards/update.md) makes delegation the composition rule so post-upgrade side effects are never lost. The fallback deviates **only on the failure path**, where the alternative is no upgrade at all; a fallback-upgraded tool skips its own post-upgrade side effects (e.g. run-kit's daemon restart) for that one run — an accepted trade-off, documented in the memory update. The fallback brew call conforms to the standard's brew-safety clause: no timeout, no signals, routed through `internal/proc` (Constitution I).

### Tests (src/cmd/shll/update_test.go)

Fake `proc.Runner` scenarios, alongside the existing relink-heal tests:

- Delegated update exits non-zero → note printed, `brew upgrade <formula>` argv recorded, fallback success → tool counts succeeded, exit 0.
- Delegated update fails AND fallback fails → tool counts failed, both failures visible, exit reflects failure.
- Delegated update succeeds → no fallback invocation (argv recorder shows no brew upgrade for that tool).
- ErrNotFound path → relink heal runs first; fallback fires only if the healed retry still fails.
- Tool with no Update argv → unchanged single `brew upgrade` (no double-upgrade).

### Non-Goals

- **No broken-keg reinstall escalation.** If a prior mid-pour SIGKILL left brew believing the new version is installed while the binary is broken, the fallback `brew upgrade` no-ops and the tool stays broken. Detecting that (delegated exec failure + brew "already installed" → `brew reinstall`) is a possible follow-up, deliberately excluded here.
- **No post-fallback binary verification probe** (`<tool> --version` after fallback).
- **No version-pinned knowledge** of idea's bug anywhere in shll.
- **No `HOMEBREW_NO_GITHUB_API` injection** — the underlying brew stall is environment-specific and a separate concern.
- **`shll install`, dry-run preview, and `upgradeArgv` untouched** — the fallback is conditional runtime recovery, not part of the planned argv, so the preview keeps showing the primary command only.

## Affected Memory

- `cli/update`: (modify) document the delegation-failure fallback: trigger (any delegated failure, after the relink heal), the note constant, single-attempt semantics, success accounting, the side-effect trade-off, and the idea ≤ 0.1.2 catch-22 incident that motivated it.

## Impact

- `src/cmd/shll/update.go` — `upgradeTool` gains the fallback branch + one new note constant (~15–25 lines).
- `src/cmd/shll/update_test.go` — new fake-runner scenarios (~5 cases).
- No CLI surface change (no new flags/subcommands — Constitution VII untouched), no roster change, no `internal/proc` change.

## Open Questions

- None — the design was settled in the originating discussion.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Generic roster-wide fallback, not version-pinned knowledge of idea's bug | Discussed — agent recommended, user approved ("yes, go ahead") | S:90 R:80 A:90 D:90 |
| 2 | Certain | Fallback lives in `upgradeTool` (update.go), delegation path only | The no-Update-argv path already is `brew upgrade`; single obvious location | S:85 R:85 A:95 D:95 |
| 3 | Certain | Note line is a named constant carrying tool name + failure detail + fallback command, mirroring `relinkNoteFmt` | Direct precedent in the same function; easily reworded | S:70 R:90 A:85 D:80 |
| 4 | Confident | Trigger is ANY delegated failure (non-zero exit or exec error), evaluated after the existing relink heal | User said "exits non-zero"; broadening to exec errors also rescues corrupted-binary cases and keeps one coherent rule | S:70 R:75 A:80 D:70 |
| 5 | Confident | Fallback success counts as full success (succeeded++, digest re-query unchanged) | Matches best-effort accounting; note line keeps the delegated failure visible | S:65 R:85 A:80 D:75 |
| 6 | Confident | Reinstall escalation + post-fallback verification probe excluded (Non-Goals) | Discussed — agent scoped as follow-up, user approved; rare mid-pour case, easily added later | S:70 R:80 A:60 D:55 |
| 7 | Confident | Side-effect trade-off accepted: a fallback upgrade skips the tool's post-upgrade hooks for that run | Discussed explicitly; rescue path only, next delegated run restores normal behavior | S:80 R:70 A:75 D:75 |

7 assumptions (3 certain, 4 confident, 0 tentative, 0 unresolved).
