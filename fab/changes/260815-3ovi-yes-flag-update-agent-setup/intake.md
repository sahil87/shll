# Intake: Add -y/--yes to shll update and shll agent-setup

**Change**: 260815-3ovi-yes-flag-update-agent-setup
**Created**: 2026-08-15

## Origin

One-shot creation from backlog idea `[3ovi]` (2026-08-15) via `/fab-new 3ovi`. Raw backlog text:

> Add -y/--yes to shll update and shll agent-setup, propagated into run-kit agent-setup --yes (rk already supports it). WHY: run-kit's dashboard update button runs shll update unattended in an rk-jobs tmux window; the terminal agent-setup refresh re-runs run-kit agent-setup bare, whose interactive 'Write these changes? [y/N]' prompt fires (the pane IS a TTY, so rk's non-TTY --yes refusal never triggers) and the job hangs forever with nobody attached. MECHANISM: explicit --yes plumbing, NOT TTY detection — pane-TTY-but-unattended is structurally undetectable. SCOPE: shll update gains -y/--yes and passes it through its terminal shll agent-setup re-run; shll agent-setup gains -y/--yes and appends --yes to its run-kit agent-setup delegation (its own skill placement is promptless already). CONSUMER: run-kit's handleShllUpdate (app/backend/api/update.go) will append --yes to the job argv once this ships — see run-kit's matching change.

The backlog entry pre-decides the mechanism (explicit flag plumbing, not TTY detection), the scope (both commands), and names the downstream consumer — those decisions are recorded as Certain assumptions below, not re-litigated.

## Why

1. **The pain point**: `shll update`'s end-of-run agent-skill refresh (`refreshPlacedAgentSkills`, `src/cmd/shll/update.go`) re-runs `shll agent-setup` as a subprocess, which in turn delegates to `run-kit agent-setup` (`delegateRunKitAgentSetup`, `src/cmd/shll/agent_setup.go`). When run-kit's hook wiring has pending changes, `run-kit agent-setup` asks `Write these changes? [y/N]` interactively. Run-kit's dashboard update button launches `shll update` unattended inside an rk-jobs tmux window — the pane **is** a TTY, so run-kit's non-TTY `--yes` refusal never triggers, the prompt fires with nobody attached, and the job hangs forever.

2. **If we don't fix it**: every dashboard-triggered `shll update` on a machine with pending run-kit hook changes silently stalls. This also violates the spirit of the toolkit's `update` standard (`docs/site/standards/update.md` § Prompt-free, unconditionally): a prompt surfacing from a wrapped subprocess stalls the compose exactly like one from shll's own code — the standard's failure-mode paragraph describes this exact pathology (TTY present, no human watching).

3. **Why this approach**: explicit `--yes` plumbing end-to-end, **not** TTY detection — a pane-TTY-but-unattended session is structurally undetectable (the backlog states this as the decided mechanism). The flag chain is `shll update --yes` → `shll agent-setup --yes` (subprocess) → `run-kit agent-setup --yes` (delegation). run-kit already supports `--yes` on its `agent-setup`, so this change is purely shll-side plumbing. The in-repo precedent for the flag is `shll uninstall`'s `--yes`/`-y` (`yesFlag`, `yesFlagShorthand`, `yesFlagUsage` constants in `src/cmd/shll/uninstall.go`).

## What Changes

### `shll agent-setup` gains `-y`/`--yes`

`src/cmd/shll/agent_setup.go`:

- `newAgentSetupCmd()` registers a `--yes`/`-y` bool flag, reusing the existing shared constants from `uninstall.go` (`yesFlag`, `yesFlagShorthand`) — same package, no duplication. The usage string may need an agent-setup-appropriate wording (the prompt being skipped belongs to the *delegated* `run-kit agent-setup`, not to shll itself), e.g. `"pass --yes to the run-kit agent-setup delegation (assume yes — for unattended runs)"`.
- `runAgentSetup(...)` gains a `yes bool` parameter, threaded through to `delegateRunKitAgentSetup`.
- `delegateRunKitAgentSetup(ctx, uninstall, yes, stderr)` appends `--yes` to the `run-kit agent-setup` argv when `yes` is true — on **both** the install path and the `--uninstall` path (the delegation helper is shared; the backlog scopes the flag to "its run-kit agent-setup delegation" without excluding uninstall).
- shll's own skill placement is already promptless (write/overwrite/delete, no sentinel) — `--yes` changes nothing about it.
- `--print` never delegates, so `--yes` combined with `--print` is a harmless no-op, **not** a mutual-exclusion error (unlike `--print`+`--uninstall`).
- Behavior without the flag is byte-identical to today.

### `shll update` gains `-y`/`--yes`

`src/cmd/shll/update.go`:

- `newUpdateCmd()` registers the same `--yes`/`-y` bool flag.
- `runUpdate(...)` gains a `yes bool` parameter (test seam unchanged in style — update_test.go drives `runUpdate` directly).
- `refreshPlacedAgentSkills(ctx, env, yes, stdout, stderr)` appends `--yes` to the `shll agent-setup` subprocess argv when `yes` is true. This is the **only** place `shll update` consumes the flag — the per-tool delegated `<tool> update` calls stay untouched (they are already bound prompt-free by the update standard; threading `--yes` into them is explicitly out of scope).
- **Dry-run preview accuracy** (toolkit principle №5 — an inaccurate preview is worse than none): when `--yes` is set and a placement exists, the preview line reflects the real argv, i.e. `Then: shll agent-setup --yes (refresh placed agent skills)`. Implementation detail: either a second constant or a small formatter next to `updatePreviewSkillRefreshLine`.
- Help text (`Long`) for both commands gains a sentence explaining the flag's purpose (unattended/agent-driven runs); `shll update`'s Long already documents the agent-setup refresh tail, so the sentence lands there.

### Flag propagation chain (end state)

```
run-kit dashboard button (consumer, separate run-kit change)
  └─ shll update --yes                       # this change
       └─ shll agent-setup --yes             # this change (subprocess, refreshPlacedAgentSkills)
            └─ run-kit agent-setup --yes     # already supported by run-kit
```

### Tests

Alongside source (`update_test.go`, `agent_setup_test.go`), driving the extracted seams with a fake `proc.Runner`:

- `shll agent-setup --yes` → delegation argv is `run-kit agent-setup --yes`; without the flag → argv unchanged (`run-kit agent-setup`).
- `shll agent-setup --yes --uninstall` → delegation argv is `run-kit agent-setup --uninstall --yes` (flag order per implementation; assert on argv contents).
- `shll update --yes` with a placement present → refresh subprocess argv is `shll agent-setup --yes`; without the flag → `shll agent-setup`.
- `shll update --yes --dry-run` with a placement present → preview line shows `shll agent-setup --yes`.
- Help output of both commands contains `--yes` (cobra registers it; a light assertion suffices).

## Affected Memory

- `cli/update`: (modify) document the `--yes`/`-y` flag and its single consumption point (the agent-setup refresh subprocess argv), plus the dry-run preview variant.
- `cli/agent-setup`: (modify) document the `--yes`/`-y` flag and its propagation into the `run-kit agent-setup` delegation (install and uninstall paths).

## Impact

- **Code**: `src/cmd/shll/update.go`, `src/cmd/shll/agent_setup.go` (+ their `_test.go` files). Small, additive; no behavior change without the flag.
- **CLI surface**: two new flags — checked against `docs/site/standards/` (update standard § Prompt-free; principles №1 non-interactive-by-default, №5 preview accuracy). No new top-level subcommand (Constitution VII untouched).
- **Help output**: `shll help-dump` derives JSON programmatically from the cobra tree, so the new flags flow into the contract automatically; no frozen-string breakage (the `--skip-brew-update` substring contract is untouched).
- **External consumer**: run-kit's `handleShllUpdate` (`app/backend/api/update.go` in the run-kit repo) appends `--yes` to its job argv once this ships — separate change in the run-kit repo, out of this change's scope.
- **No constitution friction**: subprocess argv changes stay inside `internal/proc` call sites (Principle I); composition posture unchanged (III/IV); graceful degradation paths (run-kit absent, shll off PATH) untouched (V).

## Open Questions

- None — the backlog entry pre-decides mechanism, scope, and consumer; remaining choices are graded assumptions below.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Explicit `--yes` plumbing on both commands, not TTY detection | Backlog states this verbatim as the decided mechanism ("NOT TTY detection — pane-TTY-but-unattended is structurally undetectable") | S:95 R:70 A:95 D:95 |
| 2 | Certain | `shll update --yes` affects only the agent-setup refresh argv; per-tool delegated updates are untouched | Backlog scope names only the terminal agent-setup re-run; the update standard already binds per-tool `update` prompt-free, and its § Failure mode notes the delegated argv is deliberately fixed | S:90 R:80 A:90 D:90 |
| 3 | Certain | Flag spelling `--yes` with shorthand `-y`, reusing `yesFlag`/`yesFlagShorthand` constants from `uninstall.go` | In-repo precedent in the same package; code-quality.md forbids magic strings, and duplicating the constants would be the anti-pattern | S:85 R:90 A:95 D:90 |
| 4 | Confident | `--yes` also rides the `--uninstall` delegation (`run-kit agent-setup --uninstall --yes`) | The delegation helper is shared and the backlog says "appends --yes to its run-kit agent-setup delegation" without carving out uninstall; harmless if run-kit's uninstall path never prompts | S:60 R:85 A:70 D:70 |
| 5 | Confident | `--yes` + `--print` is a harmless no-op, not a usage error | `--print` never delegates, so there is no prompt to skip; erroring would add friction with no safety benefit (contrast `--print`+`--uninstall`, which are contradictory modes) | S:55 R:90 A:80 D:75 |
| 6 | Certain | Dry-run preview line reflects `--yes` when set (`Then: shll agent-setup --yes (…)`) | Principle №5 (preview accuracy) is cited in the existing preview comment ("an inaccurate preview is worse than none"); mirrors `upgradeArgv`'s single-source-of-truth pattern | S:60 R:90 A:85 D:80 |
| 7 | Confident | agent-setup gets its own usage string for `--yes` instead of reusing `yesFlagUsage` | uninstall's wording ("skip the confirmation prompt") describes shll's own prompt; agent-setup's skipped prompt belongs to the delegated run-kit command — a tailored string is clearer, but reusing the generic one is defensible | S:45 R:95 A:70 D:55 |

7 assumptions (4 certain, 3 confident, 0 tentative, 0 unresolved).
