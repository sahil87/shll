# Plan: Auto-run shell-setup and agent-setup at the end of `shll install`

**Change**: 260819-gjhx-install-auto-shell-agent-setup
**Intake**: `intake.md`

## Requirements

### Install: auto-run shell-setup

- **R1**: `shll install` MUST run the shell-setup step automatically after the install outcome, on both non-preview outcome paths (the install-loop tail and the all-already-installed short-circuit), and MUST NOT run it under `--dry-run`. The step reuses the existing in-process seams: gate via `resolveWiringFact(env)` (doctor's read-only detector), then — when the gate says unwired — resolve `shell := resolveShell(nil, env)` / `rcPath := resolveRcFile(shell, env)` and call `runShellSetupDefault(shell, rcPath, false, stdout, stderr)` (`src/cmd/shll/shell_setup.go:320`). Passing the pre-resolved shell and rc path keeps the step hermetic under the existing `env func(string) string` test seam (no internal `os.Getenv`).
  - GIVEN a fresh machine with a resolvable `$SHELL` and an existing unwired rc file, WHEN `shll install` completes its outcome line, THEN the shll sentinel block is appended to the rc file (shell-setup's own `Installed shll shell integration to <path>` output announces it) and an `exec $SHELL` reminder line is surfaced.
  - GIVEN an already-wired rc file, WHEN install completes, THEN the step is a silent skip — no write, no nudge, no reminder.
  - GIVEN an unresolvable `$SHELL` (e.g. fish) or a corrupt open-without-close sentinel block, WHEN install completes, THEN the step is skipped quietly with no nudge (preserving the 93r2 quiet edge states — a nudge would dead-end; doctor owns the corrupt-block diagnostic).

- **R2**: A `--no-shell-setup` bool flag on `shll install` MUST skip the auto-run; the existing gated shell-setup nudge line (`shellSetupNudgeFmt`, still behind `resolveWiringFact`) prints instead. Flag name and usage string are named constants (mirroring `noTrustFlag`/`noTrustFlagUsage`).
  - GIVEN `shll install --no-shell-setup` on an unwired machine, WHEN install completes, THEN no rc write occurs and the shell-setup nudge line prints.

### Install: auto-run agent-setup

- **R3**: `shll install` MUST run the agent-setup step automatically immediately after the shell-setup step, on the same two outcome paths and never under `--dry-run`, by calling the existing in-process seam `runAgentSetup(ctx, env, stdout, stderr, false, false, true)` (`src/cmd/shll/agent_setup.go:217`) — i.e. the equivalent of `shll agent-setup --yes`, forwarding `--yes` to the `run-kit agent setup` delegation so it cannot hang on its hook-wiring prompt in unattended runs. agent-setup's own per-path `wrote`/`unchanged`/`updated` summary plus run-kit's delegated output are the announcement.
  - GIVEN a machine with `$HOME` set, WHEN install completes, THEN `~/.agents/skills/shll-toolkit/SKILL.md` and `~/.claude/skills/shll-toolkit/SKILL.md` hold the canonical skill bytes and the run-kit delegation was invoked with argv `agent setup --yes`.
  - GIVEN run-kit is absent (`proc.ErrNotFound`), WHEN the step runs, THEN the delegation is skipped silently (inherited Constitution V behavior) and the step still counts as success.
  - GIVEN an installed run-kit predating the `agent setup` family (< v3.16.23), WHEN the delegation exits non-zero, THEN the inherited warn-and-`(continuing)` path surfaces it to stderr and the install is unaffected (rk version-skew verification item b).

- **R4**: A `--no-agent-setup` bool flag on `shll install` MUST skip the auto-run; the existing unconditional agent-setup nudge line (`agentSetupNudgeFmt`) prints instead.
  - GIVEN `shll install --no-agent-setup`, WHEN install completes, THEN no skill files are written, no run-kit delegation runs, and the agent-setup nudge line prints.

### Install: graceful degradation and exit-code authority

- **R5**: A failure of either auto-run step MUST NOT change `shll install`'s exit code — the install outcome (`anyFailed`) remains the sole authority (mirroring the trust-step posture). On failure the step warns to stderr and falls back to printing that step's existing nudge line. Known failure modes that must degrade this way: shell-setup missing-rc-file / unwritable-rc errors (`errExitCode`/`errSilent` from `runShellSetupDefault` — surface the actionable message, then the gated nudge); agent-setup placement failure (`errSilent` after its own per-path stderr — print the agent nudge). Errors returned by the seams are consumed, never propagated.
  - GIVEN the rc file does not exist (shell-setup refuses to create rc files, exit-2 semantics), WHEN the auto shell-setup step fails, THEN its actionable error text reaches stderr, the shell-setup nudge prints, and `shll install` still exits 0 when all installs succeeded.
  - GIVEN an unwritable skill directory, WHEN the auto agent-setup step fails, THEN the agent-setup nudge prints and the install exit code is unchanged.

### Install: Next-steps block adaptation

- **R6**: The post-install block adapts to what the steps actually did. It is emitted after both auto-run steps, on stdout, with the existing `arrow(color)` framing, and MUST contain only the lines that apply: the gated shell-setup nudge (opt-out or failure), the agent-setup nudge (opt-out or failure), and a new `exec $SHELL` reminder line (named constant) after a successful auto shell-setup wire. When no line applies, no block and no `Next steps:` header print (restoring the pre-agst empty-block suppression). `--dry-run` remains entirely auto-run-free and nudge-free on the preview path and nudge-only-suppressed on the short-circuit path, exactly as today.
  - GIVEN a successful auto shell-setup and auto agent-setup, WHEN the block renders, THEN it contains only the `exec $SHELL` reminder (no nudges).
  - GIVEN both steps opted out, WHEN the block renders, THEN it matches today's nudge block (gated shell line + unconditional agent line).
  - GIVEN an already-wired machine and successful agent-setup, WHEN install completes, THEN no `Next steps:` header prints at all.

### Docs and help

- **R7**: `shll install --help` Long text MUST describe the auto-run behavior and both opt-out flags; `README.md`'s install flow and `docs/site/install.md` MUST describe the fully-wired outcome and the opt-outs (Policy B: install docs centralize on shll.ai, whose source is this repo's `docs/site/`). Before changing help output or docs, the surface MUST be checked against `docs/site/standards/principles.md` and `docs/site/standards/install-composition.md` (Constitution: Toolkit Standards); if the frozen machine-readable help contract covers `shll install` (see `help_dump_test.go` and any `help/*.json` fixtures), regenerate the fixture in the same task.
  - GIVEN `shll install --help`, WHEN rendered, THEN it names the two auto-run steps and `--no-shell-setup`/`--no-agent-setup`.

### Non-Goals

- No changes to `scripts/install.sh` — the bootstrap already ends with `exec shll install "$@"`, so it inherits the behavior and the flags ride the verbatim passthrough (verify only, task T008).
- No countdown, prompt, or TTY detection anywhere in the flow.
- No changes to `shll update`'s agent-setup refresh consent posture (3ovi's rejected-unconditional-`--yes` decision there stands; install's auto-run is a distinct, user-decided path).
- No new top-level subcommands (Constitution VII — two flags on the existing `install`).

### Design Decisions

- **Decision**: Invoke the steps in-process via `runShellSetupDefault` (with install-side `resolveShell`/`resolveRcFile` pre-resolution) and `runAgentSetup`, not via a `shll`-self subprocess.
  **Why**: Both seams live in package `main` with injectable env/writers; in-process reuse is hermetic under the existing test seams and strictly stronger than Constitution I's proc-routing requirement (no subprocess at all). Pre-resolving shell/rc for `runShellSetupDefault` avoids `runShellSetup`'s internal `os.Getenv`, keeping install's `env` seam authoritative.
  **Rejected**: `proc.RunForeground(ctx, "shll", "shell-setup")` self-exec — an unnecessary subprocess, PATH-dependent, and untestable without a real binary.
  *Introduced by*: 260819-gjhx-install-auto-shell-agent-setup

- **Decision**: A run-kit delegation failure inside the auto agent-setup step stays non-fatal and does NOT trigger the agent-setup nudge (only a `runAgentSetup` error — placement failure — does).
  **Why**: Inherits the standalone command's exact semantics: `delegateRunKitAgentSetup` warns with `(continuing)` and never fails placement; re-running `shll agent-setup` would hit the same delegation failure, so nudging would dead-end (same logic as the 93r2 quiet edge states).
  **Rejected**: Plumbing a delegation-outcome signal out of `runAgentSetup` — changes the standalone command's contract for a nudge that would dead-end anyway.
  *Introduced by*: 260819-gjhx-install-auto-shell-agent-setup

- **Decision**: The `Next steps:` block returns to print-only-when-non-empty (the agst change made it unconditional because the agent line always printed; after auto-run, the happy path has no nudge lines).
  **Why**: An empty header is noise; the happy-path output should read as "done and wired", with only the `exec $SHELL` reminder.
  **Rejected**: Keeping an unconditional header with a "all wired" line — adds a new string with no action for the user.
  *Introduced by*: 260819-gjhx-install-auto-shell-agent-setup

## Tasks

### Phase 2: Core Implementation

- [x] T001 Add `noShellSetupFlag`/`noShellSetupFlagUsage` and `noAgentSetupFlag`/`noAgentSetupFlagUsage` named constants, register both bool flags in `newInstallCmd`, and thread `noShellSetup, noAgentSetup bool` through `runInstall` in `src/cmd/shll/install.go` <!-- R2, R4 -->
- [x] T002 Implement the post-install setup orchestration in `src/cmd/shll/install.go` (e.g. `runPostInstallSetup(ctx, env, stdout, stderr, color, noShellSetup, noAgentSetup)`): shell step per R1's gate table (quiet skip on unresolved/corrupt/wired; auto-run via `resolveShell`+`resolveRcFile`+`runShellSetupDefault` when unwired; failure → stderr warn + gated nudge; success → exec-reminder), replacing the `printNextSteps` call sites on both outcome paths <!-- R1 -->
- [x] T003 Add the agent step to the same orchestration: call `runAgentSetup(ctx, env, stdout, stderr, false, false, true)` unless opted out; error → stderr warn + agent nudge; success → no nudge; never propagate the error (exit-code authority stays with `anyFailed`) in `src/cmd/shll/install.go` <!-- R3, R5 -->
- [x] T004 Rework the block emission: keep `shellSetupNudgeFmt`/`agentSetupNudgeFmt`, add the `exec $SHELL` reminder constant, render header + lines only when ≥1 line applies, preserve `--dry-run` exclusion on both paths, and adapt/retire `printNextSteps` accordingly in `src/cmd/shll/install.go` <!-- R6 -->

### Phase 3: Integration & Edge Cases

- [x] T005 Update existing nudge tests to the new semantics and add auto-run coverage in `src/cmd/shll/install_test.go`: auto-wire writes the sentinel block to a `t.TempDir()` rc file and prints the reminder; wired/unresolved/corrupt quiet skips; `--no-shell-setup`/`--no-agent-setup` restore the nudges with no writes; `--dry-run` runs neither step (both paths); shell-setup failure (missing rc) and agent-setup failure (unwritable dir) degrade per R5 with exit code unchanged; agent step records delegation argv `agent setup --yes` via the fake `proc.Runner`; empty-block suppression on the fully-wired happy path <!-- R1, R2, R3, R4, R5, R6 -->
- [x] T006 Run verification items (a)/(b): read run-kit's `agent setup` implementation (locate via `hop run-kit where` or `~/code/sahil87/run-kit`) to confirm its `--yes` hook-merge is idempotent and never clobbers user settings content; confirm the version-skew path (rk < v3.16.23) lands on the inherited warn-and-continue; record both findings under `## Notes` in this plan <!-- R3 -->
- [x] T007 Read `docs/site/standards/principles.md` and `docs/site/standards/install-composition.md`, then update `shll install`'s Long help text for the auto-run + opt-outs in `src/cmd/shll/install.go`, and regenerate the machine-readable help fixture if `help_dump_test.go`'s frozen contract covers it <!-- R7 -->
- [x] T008 Verify `scripts/install.sh` forwards flag-shaped args untouched (`exec shll install "$@"` — read-only check, fix only if some path filters args) and run `go test ./...` in `src/` <!-- R5 -->

### Phase 4: Polish

- [x] T009 Update `README.md` install flow and `docs/site/install.md` to describe the fully-wired outcome and the two opt-out flags (Policy B wording) <!-- R7 -->

## Acceptance

### Functional Completeness

- [x] A-001 R1: On both non-preview outcome paths, an unwired machine gets its rc file wired automatically and an `exec $SHELL` reminder is shown; wired/unresolved-`$SHELL`/corrupt-block states skip quietly with no write.
- [x] A-002 R2: `--no-shell-setup` skips the rc write and restores the gated shell-setup nudge.
- [x] A-003 R3: The agent step places both skill files and invokes the run-kit delegation with `agent setup --yes`; rk-absent is a silent skip; rk-too-old lands on warn-and-continue.
- [x] A-004 R4: `--no-agent-setup` skips placement and delegation and restores the agent-setup nudge.
- [x] A-005 R5: Neither step's failure changes the install exit code; failures warn to stderr and fall back to that step's nudge.
- [x] A-006 R6: The `Next steps:` block prints only when it has content; the fully-wired happy path prints no block; `--dry-run` runs neither step and prints no nudges on either path.
- [x] A-007 R7: Help text, README, and `docs/site/install.md` describe the new behavior and flags; the standards files were consulted; help fixtures regenerated if covered. **N/A (partial)**: the frozen machine-readable help fixture (`help/shll.json`) lives in the shll.ai repo, not this one — in-repo `help_dump_test.go` freezes schema/aliases only, so no fixture regeneration applies here.

### Behavioral Correctness

- [x] A-008 R1: Auto shell-setup re-run on an already-wired file is a byte-identical no-op (idempotency inherited from `rewriteBlocks`), and install never creates an rc file (missing rc degrades per R5).
- [x] A-009 R3: Placement re-run reports `unchanged` (idempotent by construction); `--yes` forwarding matches the 3ovi chain semantics.

### Scenario Coverage

- [x] A-010 R1: Fresh-machine scenario (resolvable `$SHELL`, existing unwired rc) ends with block written + reminder shown, exit 0.
- [x] A-011 R5: Missing-rc scenario surfaces shell-setup's actionable error and still exits 0 when installs succeeded.

### Edge Cases & Error Handling

- [x] A-012 R6: Short-circuit path (all tools already installed) runs both auto-run steps (non-dry-run) — the nudge-era "re-runner still gets nudged" decision becomes "re-runner still gets wired".
- [x] A-013 R5: Both opt-outs together produce exactly today's nudge block and no writes.

### Code Quality

- [x] A-014 Pattern consistency: all new user-facing strings and flag names are named constants; new code follows `install.go`'s existing comment/structure conventions; no raw `os/exec` (Constitution I — in-process seams, zero new subprocesses).
- [x] A-015 No unnecessary duplication: shell/rc resolution and wiring detection reuse `resolveShell`/`resolveRcFile`/`resolveWiringFact`/`runShellSetupDefault`/`runAgentSetup` — no reimplementation of sentinel, placement, or delegation logic in `install.go`.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Call `runShellSetupDefault` directly with install-side `resolveShell(nil, env)`/`resolveRcFile(shell, env)` pre-resolution, not the `runShellSetup` wrapper | The wrapper resolves via `os.Getenv` internally, which would break the hermetic `env` test seam; the default-mode core takes pre-resolved inputs and is the exact write path the standalone command uses | S:65 R:80 A:90 D:80 |
| 2 | Confident | Already-wired machines skip the shell step silently — no nudge, no `exec $SHELL` reminder | The block is already loaded in existing shells or will be at next login; today's behavior prints nothing for wired users either | S:60 R:90 A:85 D:75 |
| 3 | Confident | run-kit delegation failure does not trigger the agent nudge (only placement failure does) | Inherited standalone semantics; a nudge would dead-end on the same delegation failure | S:55 R:85 A:80 D:70 |
| 4 | Confident | `Next steps:` header suppressed when no lines apply | Restores pre-agst suppression; an empty header is noise on the now-common happy path | S:60 R:95 A:85 D:80 |
| 5 | Tentative | The `exec $SHELL` reminder renders as a line inside the `Next steps:` block (rather than appended to shell-setup's own success output) <!-- assumed: placement of the reminder line — intake fixed the content, not the position; block placement keeps one framing path --> | Keeps a single emission path and the existing arrow framing; trivially movable during review | S:45 R:90 A:70 D:50 |

5 assumptions (0 certain, 4 confident, 1 tentative, 0 unresolved).

## Notes

- Verification item (a)/(b) findings land here via T006.
- **T006 (a) — `run-kit agent setup --yes` hook-merge is idempotent and non-clobbering** (verified in `~/code/sahil87/run-kit`, `app/backend/cmd/rk/agent_setup.go`): the prompt `--yes` skips is `consent.authorizeWrite` (`agent_setup.go:313`) — without `--yes` on a non-TTY the command refuses with `errNonInteractiveConsent`, so forwarding `--yes` is *required* for unattended installs, and nothing writes silently without it. The `settings.json` hooks merge strips rk-owned entries (marker-detected, `removeRkEntries`/`isRkEntry`, `:765-791/:860-880`) then appends fresh ones — re-run replaces rk entries in place, never duplicates, and preserves all non-rk keys; a no-op is detected by before/after JSON comparison and skips the prompt entirely. Malformed JSON is never clobbered (`readSettings` errors on non-JSON, `:722-741`); the tmux shim is only overwritten when marker-owned or absent (`:1069-1141`); the PATH block is sentinel-marked and a malformed block is refused, not repaired (`markerBlockBounds`, `:951-967`). Worst case on re-run is a JSON reformat/key-sort of `settings.json` — no user-authored content can be destroyed. The prompt is a consent gate, not a data-loss guard.
- **T006 (b) — rk < v3.16.23 version skew lands on the inherited warn-and-continue path**: `agent setup` first shipped in v3.16.23 (rename commit d6480436, PR #620). An older run-kit invoked as `run-kit agent setup --yes` fails cobra's `Find` with `unknown command "agent"` → exit 2, before touching anything — which lands exactly on `delegateRunKitAgentSetup`'s `exited <code> (continuing)` warn path in `agent_setup.go`, leaving the install's exit code untouched. The old `agent-setup` spelling survives as a hidden deprecated alias (prints a deprecation warning to stderr, then runs the identical setup). Covered by `TestInstall_AutoAgentSetupDelegationFailureContinues`.
- The cli/shell-setup memory's "install consumes these primitives strictly READ-ONLY" invariant is intentionally superseded by this change (install now calls the write path when auto-running); hydrate must update that statement in `docs/memory/cli/shell-setup.md` and the install/agent-setup/bootstrap files per the intake's Affected Memory.

## Deletion Candidates

None — this change adds new functionality without making existing code redundant. The nudge-only `printNextSteps` was replaced in place by `runPostInstallSetup` (no orphaned symbol remains), and both nudge constants (`shellSetupNudgeFmt`/`agentSetupNudgeFmt`) survive as the opt-out/failure fallbacks per R6.
