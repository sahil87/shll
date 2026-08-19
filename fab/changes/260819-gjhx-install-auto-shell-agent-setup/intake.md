# Intake: Auto-run shell-setup and agent-setup at the end of `shll install`

**Change**: 260819-gjhx-install-auto-shell-agent-setup
**Created**: 2026-08-19

## Origin

Promptless dispatch (`/fab-proceed` create-new) from a synthesized user-conversation description. The user reported a recurring real-world adoption failure and made the design decisions below in that conversation; this intake transfers them verbatim.

> Wire `shll shell-setup` and `shll agent-setup` to run automatically at the end of `shll install`, so the curl-bootstrap flow (`curl -fsSL https://shll.ai/install | sh`, which execs into `shll install`) leaves a fully wired machine instead of printing nudges that users routinely ignore.

Key decisions from the conversation (full detail in What Changes):

1. `shll install` runs `shll shell-setup` automatically at the end — unconditional, no prompt, no timer; `--no-shell-setup` opt-out flag.
2. `shll install` runs `shll agent-setup --yes` automatically, announced; `--no-agent-setup` opt-out flag.
3. Failures of either auto-run step MUST degrade gracefully — never fail the install; fall back to printing that step's existing nudge line (Constitution V).
4. The existing conditional nudge logic adapts: after a successful auto shell-setup the shell-setup nudge is unneeded, but the `exec $SHELL` reminder must still be surfaced; an opted-out or failed step prints its existing nudge.
5. The wiring lives in the Go `shll install` command, not in `scripts/install.sh` — the bootstrap already execs `shll install "$@"`, so it inherits the behavior and the new flags pass through (`sh -s -- --no-agent-setup`).

Alternatives the user explicitly rejected:

- **Timed countdown** ("executing in 10 seconds, press esc to cancel"): under `curl | sh`, stdin is the script pipe, not a TTY, and the hardened headless/fresh-VM path (NONINTERACTIVE brew bootstrap, PR #81 install-bootstrap gaps) has no `/dev/tty` at all — the countdown would hang, silently auto-run, or need a third code path; timed defaults also penalize everyone and make scripted runs non-deterministic.
- **Keeping the nudges as-is**: demonstrated to fail (the run-kit adoption problem below).
- **Prompting y/N**: same no-TTY problem under `curl | sh`.

## Why

Today `shll install` only prints "Next steps" nudge lines for `shll shell-setup` and `shll agent-setup` (`src/cmd/shll/install.go` — `shellSetupNudgeFmt`/`agentSetupNudgeFmt`, printed by `printNextSteps`). The user reports a recurring real-world failure: people install run-kit but never run agent-setup, so their agents never learn run-kit's capabilities and they never get the full run-kit experience. A nudge that's routinely ignored is a **silent adoption failure, not graceful degradation** — the toolkit's first-run experience terminates in `shll install`'s output (the curl bootstrap execs into it), and if the machine isn't wired at that point, for many users it never is.

If we don't fix it: fresh installs keep landing half-configured — shell integration unwired (no `shll shell-init` eval line, so `hop`/`wt`/`tu` shell functions silently absent) and agent harnesses unwired (no `shll-toolkit` skill, no run-kit dashboard hooks) — and the demonstrated run-kit adoption failure keeps recurring.

Why auto-run over the alternatives: both target commands are **idempotent and reversible by construction** — `shell-setup` manages a sentinel block (`# >>> shll >>>` in the rc file; re-run is a no-op; `--uninstall` removes it), and `agent-setup` overwrites shll-owned skill files (has `--uninstall`) then delegates to run-kit. Auto-running an idempotent, reversible, announced step with an opt-out flag is strictly better UX than a nudge, and is the only design that works identically on TTY, `curl | sh` pipe, and headless/fresh-VM paths (see rejected alternatives in Origin).

Consistency note (not a philosophy reversal): `scripts/install.sh` deliberately leaves *brew's* shellenv line to the user ("brew's shellenv line is the user's to keep") — that philosophy governs OTHER tools' territory. shll's own eval line is exactly what `shell-setup` owns (sentinel-managed), so auto-wiring shll's own contract is consistent with it.

## What Changes

All changes are additive behavior + two flags on the existing `install` subcommand (Constitution VII: no new top-level commands).

### 1. Auto-run `shll shell-setup` at the end of `shll install`

After the install outcome (summary tail on the loop path; the `allInstalledMsg` line on the all-already-installed short-circuit path — both of the current `printNextSteps` emission points), `runInstall` runs the shell-setup step automatically:

- **Unconditional** — no prompt, no timer, no TTY dependence. Idempotency is the safety property: re-run on an already-wired rc file is a no-op (the sentinel block `# >>> shll >>>` already holds the eval line), and `shll shell-setup --uninstall` reverses it.
- **In-process invocation** of the existing seam `runShellSetup(ctx, nil, "", false, false, stdout, stderr)` (`src/cmd/shll/shell_setup.go`) — same `main` package, no subprocess needed. (If a subprocess were used instead, it MUST route through `internal/proc` per Constitution I — but in-process reuse is the expected shape; extract a narrower callable seam only if the existing one doesn't fit.)
- **Announced** — the step says what it did, e.g. wired the eval line into `~/.zshrc` (shell-setup's own output already reports `installed`/`already installed`/`migrated`), and the `exec $SHELL` reminder is surfaced after a successful wire (see §4).
- **`--no-shell-setup` opt-out flag** on `shll install` (named constants per code-quality.md, mirroring `noTrustFlag`/`noTrustFlagUsage`) — for dotfile-manager users who manage rc files themselves. When passed, the step is skipped and the existing gated shell-setup nudge prints instead (see §4).

### 2. Auto-run `shll agent-setup --yes` at the end of `shll install`

Immediately after the shell-setup step:

- **Runs the equivalent of `shll agent-setup --yes`** — in-process via `runAgentSetup(ctx, env, stdout, stderr, false, false, true)` (`src/cmd/shll/agent_setup.go`). `agent-setup` writes the `shll-toolkit` SKILL.md into the two shll-OWNED global skill directories (`~/.agents/skills/`, `~/.claude/skills/` — overwrite = idempotent by construction, has `--uninstall`), then delegates to `run-kit agent setup`, forwarding `--yes` to skip run-kit's `Write these changes? [y/N]` hook-wiring confirmation.
- The delegation is **already skipped silently when run-kit is absent** (`proc.ErrNotFound` → silent skip, Constitution V) and a real delegation error is surfaced to stderr with `(continuing)` but is non-fatal (`delegateRunKitAgentSetup`, `src/cmd/shll/agent_setup.go`) — this existing behavior is inherited, not reimplemented.
- **Announced** — agent-setup's own per-path `wrote`/`unchanged`/`updated` summary plus run-kit's delegated output are the announcement.
- **`--no-agent-setup` opt-out flag** on `shll install`. When passed, the step is skipped and the existing agent-setup nudge prints instead (see §4).

### 3. Graceful degradation — auto-run failures never fail the install

- A failure of either auto-run step (non-nil error from the seam) MUST NOT change `shll install`'s exit code — the install outcome (`anyFailed` from the brew loop) remains the sole authority. Mirrors the existing trust-step posture ("warn and continue; the install's own exit code is the authority").
- On failure, warn to stderr and **fall back to printing that step's existing nudge line** so the user still knows the manual path.
- Known shell-setup failure modes that must degrade to the nudge, not error the install: unsupported/uninferable `$SHELL` (`resolveShell` returns `errExitCode{code: 2}` — e.g. fish), unwritable/corrupt rc file (open-sentinel-without-close is refused by shell-setup). Note the current nudge gate is *quiet* on unresolvable-`$SHELL` and corrupt-block states (nudging would dead-end); the failure fallback should preserve that nuance — surface the actionable error, don't print a dead-end nudge.
- Known agent-setup failure modes: empty `$HOME` (no targets), unwritable skill dirs (`errSilent` after per-path stderr), rk version skew (see Impact/verification) — all degrade to warn + nudge.

### 4. Nudge adaptation in `printNextSteps`

The existing "Next steps" block adapts to what the auto-run steps actually did:

- **After a successful auto shell-setup**: the shell-setup nudge line is unneeded (the block is now wired — the existing `resolveWiringFact` gate would suppress it anyway on a re-read), but the **`exec $SHELL` reminder must still be surfaced** (a freshly wired rc file isn't loaded in the parent shell; and under the curl bootstrap the `exec shll install` process's parent shell dies anyway, so the message should tell the user to restart/exec their shell).
- **When a step was opted out (`--no-shell-setup`/`--no-agent-setup`) or its auto-run failed**: print that step's existing nudge line (`shellSetupNudgeFmt` — still behind its `resolveWiringFact` gate — / `agentSetupNudgeFmt`).
- **After a successful auto agent-setup**: the agent-setup nudge is unneeded (the step just ran — the "shll cannot cheaply know whether agent-setup already ran" rationale for the unconditional nudge no longer applies on this path).
- `--dry-run` remains auto-run-free and nudge-free on the preview path, and nudge-only-suppressed on the short-circuit path, exactly as today (decision 5 of change 93r2); the auto-run steps are writes and MUST NOT run under `--dry-run`.
- Existing named-constant convention holds: all new user-facing strings (announce lines, `exec $SHELL` reminder, flag usage strings) are named constants in `install.go`.

### 5. Bootstrap passthrough — no `scripts/install.sh` changes expected

The curl bootstrap already ends with `exec shll install "$@"`, so it inherits the auto-run behavior with zero script changes, and the new flags ride the existing argument passthrough: `curl -fsSL https://shll.ai/install | sh -s -- --no-agent-setup` → `exec shll install --no-agent-setup`. Verify during apply that the passthrough handles flag-shaped args (not just tool names) — the script forwards `"$@"` verbatim, so it should; if the script filters or validates args anywhere, fix that there.

### 6. Docs

- `shll install --help` Long text gains the auto-run + opt-out description (check `docs/site/standards/` — notably `principles.md` and `install-composition.md` — before changing CLI surface/help output, per Constitution "Toolkit Standards"; `help-dump` fixtures may need regeneration if the frozen `help/<tool>.json` contract covers shll).
- README install-flow wording and shll.ai install docs (Policy B centralizes install docs on shll.ai) updated to describe the fully-wired outcome and the opt-out flags.

## Affected Memory

- `cli/install`: (modify) auto-run steps, the two new flags, nudge adaptation, emission points, dry-run exclusion, exit-code posture
- `cli/agent-setup`: (modify) new invoker (install's auto-run) joins the touchpoints; the `--yes` consent chain gains the install path
- `cli/shell-setup`: (modify) new invoker (install's auto-run) noted; wiring contract unchanged
- `ci/install-bootstrap`: (modify) the curl-bootstrap outcome is now a fully wired machine; flag passthrough (`sh -s -- --no-*`) documented as public surface

## Impact

- **Code**: `src/cmd/shll/install.go` (auto-run orchestration, `--no-shell-setup`/`--no-agent-setup` flags, nudge adaptation, announce constants); possibly small callable-seam adjustments in `src/cmd/shll/shell_setup.go` / `src/cmd/shll/agent_setup.go`; tests in `install_test.go` (existing nudge tests will need updating — e.g. `TestInstall_ShellSetupNudgeShownWhenUnwired`, `TestInstall_AgentSetupNudgeUnconditional` semantics change), possibly `shell_setup_test.go`/`agent_setup_test.go`.
- **Behavioral surface**: `shll install` (and therefore the `curl | sh` bootstrap) now writes to the user's rc file and skill dirs by default. Both writes are idempotent and reversible; opt-outs exist.
- **Constitution fit**: I — in-process seam reuse, or `internal/proc` if any new subprocess appears; III/IV — install *composes* shell-setup/agent-setup by calling their existing seams, never absorbing their logic; V — every auto-run failure degrades to warn + nudge, never fails the install; VII — flags on the existing subcommand, no new commands.
- **Verify during planning/apply** (user-flagged):
  - (a) What `run-kit agent setup`'s confirmation prompt protects — forwarding `--yes` by default inherits that risk; confirm rk's harness-file merge is idempotent and never clobbers user content. Prior changes `260815-3ovi-yes-flag-update-agent-setup` and `260816-iags-rk-agent-setup-spelling` touched this seam — read their intakes / `docs/memory/cli/agent-setup.md` (already reflects both). Note the 3ovi Design Decision explicitly rejected making `shll update`'s refresh unconditionally `--yes` (consent on attended runs); the user has consciously decided install's auto-run DOES forward `--yes` — the verification is the guard on that decision.
  - (b) rk version skew — an installed run-kit predating the `agent setup` family (min v3.16.23) exits non-zero; the existing warn-and-`(continuing)` path must degrade to the nudge, not error mid-install (the iags Design Decision deliberately has no version probe/fallback spelling).
  - (c) `shll shell-setup` failure modes (unsupported `$SHELL` → exit-2 `errExitCode`, unwritable rc) must degrade to the nudge/warn, not fail the install.
- **Standards**: `install-composition.md` (change w6ay) — sibling invocations stay probed/graceful (rk delegation already conforms); Policy B — install docs updates land on shll.ai, not per-repo READMEs.

## Open Questions

- None — the conversation resolved the design decisions; the three user-flagged verification items above are apply-time checks, not open design questions.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Auto-run `shll shell-setup` unconditionally at end of install (no prompt/timer), with `--no-shell-setup` opt-out | Discussed — user decided explicitly; countdown and y/N prompt rejected for the no-TTY curl-pipe/headless paths | S:95 R:85 A:95 D:95 |
| 2 | Certain | Auto-run `shll agent-setup --yes` at end of install, announced, with `--no-agent-setup` opt-out | Discussed — user decided explicitly, aware `--yes` inherits run-kit's prompt-skip risk and ordered the apply-time verification of rk's merge idempotency | S:95 R:80 A:90 D:95 |
| 3 | Certain | Auto-run failures never fail the install — warn to stderr and fall back to that step's existing nudge line | Discussed — user decided; matches Constitution V and the existing trust-step warn-and-continue posture | S:95 R:90 A:95 D:95 |
| 4 | Certain | Wiring lives in Go `install.go`, not `scripts/install.sh`; new flags ride the existing `exec shll install "$@"` passthrough | Discussed — user decided; bootstrap inherits behavior with zero script changes | S:95 R:85 A:95 D:95 |
| 5 | Confident | Invoke the steps in-process via the existing same-package seams (`runShellSetup`, `runAgentSetup`) rather than spawning `shll` as a subprocess | Both seams live in package `main` with injectable env/writers; in-process avoids a self-exec subprocess entirely (strictly stronger than Constitution I's proc-routing requirement); user's description anticipated "callable seams" | S:70 R:80 A:85 D:80 |
| 6 | Confident | Auto-run fires on both non-preview outcome paths (install loop AND all-already-installed short-circuit), for whole-roster and subset runs alike; never under `--dry-run` | Mirrors `printNextSteps`'s existing emission points; idempotency makes the short-circuit re-runner (the user who ignored the nudge) the exact beneficiary; dry-run stays write-free by contract | S:60 R:85 A:85 D:75 |
| 7 | Confident | Step order: shell-setup then agent-setup, after the install outcome line (summary tail / `allInstalledMsg`) | Matches the existing nudge order and the natural narrative (install → wire shell → wire agents); no cross-dependency between the steps | S:55 R:90 A:85 D:80 |
| 8 | Confident | Shell-setup auto-run respects the existing quiet edge states: unresolvable `$SHELL` or corrupt sentinel block → warn/skip without a dead-end nudge (the current gate already suppresses the nudge there because it would dead-end) | Preserves the deliberate 93r2 gate nuance; doctor owns the corrupt-block diagnostic | S:55 R:85 A:80 D:70 |
| 9 | Confident | Exact announce/reminder wording (e.g. the `exec $SHELL` reminder line, opt-out flag usage strings) chosen at apply time as named constants following `install.go` conventions <!-- assumed: wording not specified in the conversation; only the information content (announce what was done + exec $SHELL reminder) was decided --> | Content decided, strings not; low-stakes and trivially revisable during review | S:45 R:90 A:75 D:55 |

9 assumptions (4 certain, 5 confident, 0 tentative, 0 unresolved).
