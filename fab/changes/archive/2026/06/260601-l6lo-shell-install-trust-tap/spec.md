# Spec: shell-install --trust-tap (Homebrew tap-trust resolution)

**Change**: 260601-l6lo-shell-install-trust-tap
**Created**: 2026-06-01
**Affected memory**: `docs/memory/cli/shell-install.md`, `docs/memory/cli/commands.md` (cross-ref only)

## Non-Goals

- Setting `HOMEBREW_NO_ENV_HINTS` / `HOMEBREW_NO_REQUIRE_TAP_TRUST` on shll's own brew subprocesses — shll does not silence brew's advisory chatter at runtime; the user opts into genuine trust explicitly. (These lighter escape hatches are documented in the README for users who prefer them, but shll does not write them.)
- Trusting individual formulae/casks/commands — only `--tap`-level trust for `sahil87/tap` is in scope.
- A new top-level `shll trust-tap` command — the capability is a flag on the existing `shell-install` (Constitution VII).
- Discovering taps dynamically — the tap name is the existing hardcoded contract (Constitution: Tool Roster Source of Truth).
- Auto-running `brew untrust` on uninstall — uninstall removes the rc block only.

## cli/shell-install: The `--trust-tap` flag

### Requirement: New `--trust-tap` flag composes with existing modes

`shll shell-install` SHALL accept a boolean `--trust-tap` flag. It is NOT a dispatch mode like `--print`/`--uninstall` (which are mutually exclusive); it is an orthogonal selector that composes with the default, `--print`, and `--uninstall` paths. When `--trust-tap` is set on a non-print, non-uninstall invocation, the command SHALL perform genuine Homebrew tap trust: both the trust ceremony AND the rc-file policy line.

#### Scenario: flag is recognized
- **GIVEN** the `shll shell-install` command
- **WHEN** the user runs `shll shell-install --trust-tap`
- **THEN** the command SHALL accept the flag without an "unknown flag" error
- **AND** `shll shell-install --help` SHALL document `--trust-tap`.

#### Scenario: --trust-tap composes with --print
- **GIVEN** any valid rc-file resolution
- **WHEN** the user runs `shll shell-install --trust-tap --print`
- **THEN** the command SHALL print the resulting combined block to stdout
- **AND** SHALL NOT modify any file
- **AND** SHALL NOT run the `brew trust` ceremony.

#### Scenario: --print and --uninstall remain mutually exclusive
- **GIVEN** the existing flag-conflict guard
- **WHEN** the user runs `shll shell-install --print --uninstall` (with or without `--trust-tap`)
- **THEN** the command SHALL return `errExitCode{code:2}` with the existing mutual-exclusion message, unchanged.

### Requirement: Genuine trust applies both ceremony and policy atomically

`shll shell-install --trust-tap` (default mode) SHALL perform BOTH halves of genuine trust:

1. **Ceremony** — run `brew trust --tap sahil87/tap` via `internal/proc`.
2. **Policy** — ensure `export HOMEBREW_REQUIRE_TAP_TRUST=1` is present in the shll-managed rc block.

Neither half alone is acceptable: the policy line without the trust record causes brew to BLOCK the tap; the trust record without the policy line leaves the warning in place. The command SHALL apply both, and a re-run SHALL repair a partially-applied state (missing line added; ceremony re-run — idempotent).

#### Scenario: fresh trust on a clean machine
- **GIVEN** `sahil87/tap` is not yet trusted AND the rc block has no export line
- **WHEN** the user runs `shll shell-install --trust-tap`
- **THEN** `brew trust --tap sahil87/tap` SHALL be invoked via `internal/proc`
- **AND** the rc block SHALL contain `export HOMEBREW_REQUIRE_TAP_TRUST=1`
- **AND** the command SHALL exit 0 with a success message naming the rc file.

#### Scenario: partial state self-repairs
- **GIVEN** the rc block already has the export line but the tap is not trusted (or vice versa)
- **WHEN** the user re-runs `shll shell-install --trust-tap`
- **THEN** the missing half SHALL be applied
- **AND** the already-present half SHALL NOT be duplicated or error
- **AND** the command SHALL exit 0.

### Requirement: `brew trust` ceremony routes through internal/proc, NOT from shell_install.go

The `brew trust` / capability-probe subprocess calls SHALL be implemented through `internal/proc` (Constitution I — never raw `os/exec`). The implementation SHALL NOT add `internal/proc` or `"os/exec"` imports to `src/cmd/shll/shell_install.go`: that file is guarded by `TestNoProcImports` (`shell_install_test.go:493`), which fails the build if either import appears. The trust subprocess logic SHALL live in a file that already legitimately performs subprocess execution (e.g. `src/cmd/shll/brew.go`, which imports `internal/proc`), exposed as a function `shell_install.go` calls. `shell_install.go` retains its file-I/O-only character; the guard MUST continue to pass.

#### Scenario: TestNoProcImports still passes
- **GIVEN** the implemented change
- **WHEN** `TestNoProcImports` reads `shell_install.go` as bytes
- **THEN** it SHALL find neither `internal/proc` nor `"os/exec"`
- **AND** the test SHALL pass.

#### Scenario: ceremony uses proc
- **GIVEN** the trust ceremony helper (in `brew.go` or equivalent)
- **WHEN** it invokes `brew trust --tap sahil87/tap`
- **THEN** the invocation SHALL go through `proc.Run` or `proc.RunForeground` — never `exec.Command`/`exec.CommandContext` directly.

### Requirement: Tap name is a named constant distinct from the formula prefix

The tap argument to `brew trust --tap` SHALL be the tap `sahil87/tap`, sourced from a named constant. Note the existing `formulaPrefix` constant is `sahil87/tap/` (with trailing slash, for building *formula* references like `sahil87/tap/shll`); the tap name is `sahil87/tap` (no trailing slash). The implementation SHALL define/derive a `tapName` constant rather than open-coding the string (code-quality: no magic strings).

#### Scenario: tap vs formula distinction
- **GIVEN** the named constants
- **WHEN** the ceremony builds its arguments
- **THEN** it SHALL pass `sahil87/tap` (the tap), not `sahil87/tap/<formula>`
- **AND** the string SHALL come from a named constant, not a literal at the call site.

## cli/shell-install: Single combined sentinel block + migration

### Requirement: One combined shll-managed block under a new sentinel

The rc file SHALL carry exactly ONE shll-managed block, wrapped by a new combined sentinel pair `# >>> shll >>>` / `# <<< shll <<<` (replacing the legacy `# >>> shll shell-init >>>` / `# <<< shll shell-init <<<`). The block body holds the managed lines that apply, in this order:

```sh
# >>> shll >>>
export HOMEBREW_REQUIRE_TAP_TRUST=1
eval "$(shll shell-init <shell>)"
# <<< shll <<<
```
<!-- clarified: close sentinel corrected from `# <<< shll >>>` to `# <<< shll <<<` — the `>>>` was a typo in the canonical block-format example; every other occurrence in this spec (lines for combined-block, migration, uninstall scenarios) and the intake use `<<<`, matching the legacy `# <<< shll shell-init <<<` close-sentinel convention. The exact bytes are load-bearing for findBlock/uninstall. -->

The `export` line precedes the `eval` line (policy set before any eval-time brew invocation reads it). When only one managed line applies, only that line appears between the sentinels.

#### Scenario: combined block format
- **GIVEN** a fresh install via `shll shell-install --trust-tap zsh`
- **WHEN** the block is written
- **THEN** it SHALL be wrapped by `# >>> shll >>>` and `# <<< shll <<<`
- **AND** the `export` line SHALL appear before the `eval` line
- **AND** the block SHALL end with a single trailing `\n` (preserving the existing trailing-`\n` contract).

#### Scenario: shell-init-only install uses the new sentinel
- **GIVEN** a fresh rc file
- **WHEN** the user runs `shll shell-install zsh` (no `--trust-tap`)
- **THEN** the block SHALL use the new `# >>> shll >>>` sentinel
- **AND** SHALL contain only the `eval` line (no export line).

### Requirement: Install is a per-line merge into the single block

The install path SHALL compose the block body from the UNION of (a) managed lines already present in the existing block and (b) managed lines this invocation adds (`eval` always for a normal/`--trust-tap` install; `export` when `--trust-tap`). It SHALL locate any existing shll block, and rewrite it in place with the unioned body; if no block exists, it SHALL append a new one. Idempotency is now per-line: a managed line already present SHALL NOT be duplicated.

#### Scenario: already-set-up user adds trust
- **GIVEN** an rc file whose shll block contains only the `eval` line
- **WHEN** the user runs `shll shell-install --trust-tap`
- **THEN** the `export` line SHALL be merged INTO the existing block (no second block, no duplicate `eval`)
- **AND** the ceremony SHALL run
- **AND** the command SHALL exit 0.

#### Scenario: trust-first user later adds shell-init
- **GIVEN** an rc file whose shll block contains only the `export` line
- **WHEN** the user runs `shll shell-install` (no `--trust-tap`)
- **THEN** the `eval` line SHALL be merged into the existing block
- **AND** the `export` line SHALL be preserved.

#### Scenario: full re-run is a no-op
- **GIVEN** an rc file whose shll block already contains both managed lines and the tap is trusted
- **WHEN** the user re-runs `shll shell-install --trust-tap`
- **THEN** the rc file content SHALL be byte-identical before and after
- **AND** the command SHALL exit 0 (idempotency preserved per-line).

### Requirement: Legacy sentinel blocks are migrated in place

When the install path encounters a legacy `# >>> shll shell-init >>>` block, it SHALL migrate it: read the legacy block's managed line(s), rewrite the block under the new `# >>> shll >>>` sentinel with the unioned body, and remove the legacy sentinel. Migration SHALL preserve the existing symlink-preservation, trailing-newline, and "never creates rc files" invariants.

#### Scenario: legacy eval-only block migrates on next install
- **GIVEN** an rc file with a legacy `# >>> shll shell-init >>>` block containing the `eval` line
- **WHEN** the user runs `shll shell-install --trust-tap`
- **THEN** the legacy sentinels SHALL be replaced by `# >>> shll >>>` / `# <<< shll <<<`
- **AND** the `eval` line SHALL be carried forward
- **AND** the `export` line SHALL be merged in
- **AND** no legacy `# >>> shll shell-init >>>` sentinel SHALL remain.

#### Scenario: both legacy and new sentinel present (corrupted state)
- **GIVEN** an rc file that contains BOTH a legacy `# >>> shll shell-init >>>` block and a new `# >>> shll >>>` block (e.g. hand-edited)
- **WHEN** the user runs any install
- **THEN** the command SHALL resolve to a single new-sentinel block containing the union of managed lines from both, removing the legacy block
- **AND** SHALL NOT leave two shll-managed blocks.

> **Resolution (clarify 2026-06-01)**: merge into one new-sentinel block. Refusing would strand the user in a two-block state shll cannot itself repair; merging to a single block is the safe, self-healing default.

#### Scenario: partial/unclosed legacy block
- **GIVEN** an rc file with an open legacy sentinel but no matching close (corrupted/partial)
- **WHEN** the user runs an install
- **THEN** the command SHALL emit a clear diagnostic and refuse to modify the file (exit 2), directing the user to clean up manually
- **AND** SHALL NOT produce a malformed block.

> **Resolution (clarify 2026-06-01)**: refuse with a diagnostic (exit 2). Guessing the bounds of an unclosed block risks corrupting the user's rc file; an explicit refuse-and-explain is safer than auto-repair. This is a deliberate, documented divergence from the current short-circuit-as-"already installed" behavior.

### Requirement: Per-line idempotency message and exit codes

The default install path's messaging SHALL reflect per-line outcomes: a fully-present block is a no-op (existing "already installed (no changes)" semantics, exit 0); a merge that adds line(s) reports success. The existing exit-code policy (0 success/no-op, 1 I/O failure, 2 user-invocation error) SHALL be preserved.

#### Scenario: exit codes unchanged for existing paths
- **GIVEN** the existing exit-code contract in `docs/memory/cli/shell-install.md`
- **WHEN** any pre-existing path executes (missing shell, missing rc file, I/O error, mutual-exclusion)
- **THEN** the exit codes (0/1/2) SHALL match the documented policy exactly.

## cli/shell-install: Uninstall removes the whole block

### Requirement: Uninstall removes the entire shll block, both sentinels

`shll shell-install --uninstall` SHALL remove the entire shll-managed block — both managed lines — in one operation, recognizing BOTH the new `# >>> shll >>>` sentinel AND a legacy `# >>> shll shell-init >>>` block (so users who never re-installed can still uninstall). It SHALL NOT run `brew untrust`. The existing EvalSymlinks→O_TRUNC symlink-preservation strategy SHALL be retained.

#### Scenario: uninstall removes new-sentinel block
- **GIVEN** an rc file with a `# >>> shll >>>` block (both lines)
- **WHEN** the user runs `shll shell-install --uninstall`
- **THEN** the entire block SHALL be removed (both lines and both sentinels)
- **AND** `brew untrust` SHALL NOT be invoked
- **AND** the rc-file symlink (if any) SHALL remain a symlink.

#### Scenario: uninstall removes legacy block
- **GIVEN** an rc file with only a legacy `# >>> shll shell-init >>>` block
- **WHEN** the user runs `shll shell-install --uninstall`
- **THEN** the legacy block SHALL be removed
- **AND** the command SHALL exit 0.

#### Scenario: uninstall with nothing present
- **GIVEN** an rc file with no shll block (neither sentinel)
- **WHEN** the user runs `shll shell-install --uninstall`
- **THEN** the command SHALL report "nothing to uninstall" and exit 0 (existing benign-no-op semantics).

## cli/shell-install: Graceful degradation

### Requirement: Capability-probe brew trust; degrade without erroring

Before invoking `brew trust`, the implementation SHALL probe whether the `brew trust` subcommand is available (e.g. `brew trust --help` recognized), and SHALL handle `brew` being entirely absent. When `brew trust` is unavailable (older brew) or `brew` is missing, the command SHALL NOT write the `export HOMEBREW_REQUIRE_TAP_TRUST=1` line (writing it without a trust record would cause brew to block the tap), SHALL emit a clear diagnostic pointing the user at the lighter `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` / `HOMEBREW_NO_ENV_HINTS=1` escape hatches, and SHALL degrade rather than hard-fail (Constitution V). The shell-init `eval` line SHALL STILL be written in this case (it is pure file I/O and always succeeds), so the user retains shell integration; only the trust half is skipped. The command SHALL exit 0 (degraded success).

> **Resolution (clarify 2026-06-01)**: when trust is unavailable, write the eval line anyway and skip only the trust half. This honors the "full setup" intent of `--trust-tap` as far as the environment allows — a fresh user on old brew still gets working shell integration rather than nothing.

#### Scenario: older brew lacks `brew trust`
- **GIVEN** a Homebrew version without the `trust` subcommand
- **WHEN** the user runs `shll shell-install --trust-tap`
- **THEN** the command SHALL NOT run `brew trust`
- **AND** SHALL NOT write `export HOMEBREW_REQUIRE_TAP_TRUST=1`
- **AND** SHALL still write the `eval` line (shell integration preserved)
- **AND** SHALL emit a diagnostic naming the env-var escape hatches and that trust was skipped
- **AND** SHALL exit 0 (degrade gracefully, not a hard error).

#### Scenario: brew entirely absent
- **GIVEN** `brew` is not on PATH (`proc.ErrNotFound`)
- **WHEN** the user runs `shll shell-install --trust-tap`
- **THEN** the command SHALL detect the missing binary
- **AND** SHALL degrade with a clear message rather than panicking or writing the policy line.

### Requirement: `brew trust` / `brew untrust` invoked unconditionally (idempotency verified)

Because `brew trust --tap` and `brew untrust --tap` are idempotent (verified on brew 5.1.14: re-run exits 0 with "Already trusted" / "Not trusted"), the implementation SHALL invoke `brew trust` unconditionally during `--trust-tap` (no pre-check for existing trust). A non-zero exit from the ceremony SHALL be surfaced as a diagnostic but SHALL follow the degradation policy (do not write the policy line on ceremony failure).

#### Scenario: re-trusting an already-trusted tap
- **GIVEN** `sahil87/tap` is already trusted
- **WHEN** `shll shell-install --trust-tap` runs the ceremony again
- **THEN** the ceremony SHALL exit 0
- **AND** the command SHALL treat it as success (no guard needed).

#### Scenario: ceremony fails at runtime (brew present, trust available, non-zero exit)
- **GIVEN** `brew trust` is available but the ceremony returns a non-zero exit (e.g. a transient brew/network error)
- **WHEN** `shll shell-install --trust-tap` runs the ceremony
- **THEN** the command SHALL surface the ceremony failure as a diagnostic
- **AND** SHALL NOT write `export HOMEBREW_REQUIRE_TAP_TRUST=1` (no policy line without a trust record, per the degradation policy)
- **AND** SHALL still write the `eval` line (pure file I/O, shell integration preserved — same as the unavailable-trust degradation path)
- **AND** SHALL exit 0 (degraded success, consistent with the Constitution V degradation policy).
<!-- clarified: scenario added to cover ceremony non-zero exit at runtime — the requirement text already specifies this behavior ("A non-zero exit from the ceremony SHALL be surfaced as a diagnostic but SHALL follow the degradation policy (do not write the policy line on ceremony failure)") but had no GWT scenario. Resolved from the requirement text plus the graceful-degradation requirement (write eval anyway, skip trust half, exit 0). -->


## cli/commands: Documentation

### Requirement: README documents the flag and a Troubleshooting section

`README.md` SHALL document the `--trust-tap` flag in the `shll shell-install` section, and SHALL add a Troubleshooting section explaining the Homebrew tap-trust warning (that it is a brew env-hint, not a shll error; that shll surfaces it because it wraps brew; that `shll update` may show it 2–3×), presenting `shll shell-install --trust-tap` as the recommended resolution and the lighter `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` / `HOMEBREW_NO_ENV_HINTS=1` env vars as alternatives the user may set themselves. It SHALL state that shll does not set these for the user.

#### Scenario: README troubleshooting present
- **GIVEN** the updated README
- **WHEN** a user searches for the warning text
- **THEN** README SHALL contain a Troubleshooting entry naming the warning and the `--trust-tap` resolution
- **AND** SHALL mention the env-var escape hatches.

## Design Decisions

1. **Ceremony lives outside `shell_install.go`**: the `brew trust` subprocess logic is placed in `brew.go` (which already imports `internal/proc`), exposed as a helper that `shell_install.go` calls.
   - *Why*: `shell_install.go` is guarded by `TestNoProcImports` and documented as file-I/O-only (Constitution I boundary). Adding proc there would break the guard and the documented invariant.
   - *Rejected*: importing `proc` into `shell_install.go` — breaks `TestNoProcImports`; violates the documented "no subprocess execution" character of the file.

2. **Single combined block with new `# >>> shll >>>` sentinel + migration**: one shll footprint, migrate legacy blocks in place.
   - *Why*: user decision — tidier dotfile, single unambiguous uninstall.
   - *Rejected*: two separate sentinel blocks (simpler code, but two shll sections in the dotfile); keeping the legacy `shell-init` sentinel (no migration, but a misleading name once it holds the export line).

3. **`--trust-tap` does full setup (both lines)**: ensures eval + export + ceremony.
   - *Why*: user decision — one-command full setup for a fresh user.
   - *Rejected*: export-line-only (avoids surprising a trust-only user, but requires two commands for full setup).

4. **Uninstall does NOT `brew untrust`**: removes the rc block only.
   - *Why*: user decision — the trust record is inert without the policy var and harmless; minimal blast radius; `brew untrust` is idempotent so manual reversal stays available.
   - *Rejected*: symmetric untrust on uninstall — mutates brew state the user may not have meant to reverse.

5. **Invoke ceremony unconditionally (no trust pre-check)**: rely on verified idempotency.
   - *Why*: `brew trust`/`untrust` exit 0 on re-run (empirically verified, brew 5.1.14); a pre-check adds a subprocess round-trip for no benefit.
   - *Rejected*: query-then-trust — extra complexity, brew's trust-storage location is version-dependent and undocumented.

## Clarifications

### Session 2026-06-01 (spec stage)

| # | Question | Resolution |
|---|----------|------------|
| #13 | Both legacy + new sentinel present (corrupted/hand-edited) | **Merge** into one new-sentinel block (agent default — safe, self-healing; refusing would strand the user with two blocks shll can't fix). |
| #14 | Partial/unclosed legacy block | **Refuse** with diagnostic, exit 2 (agent default — guessing bounds risks corrupting the rc file). Deliberate divergence from the current short-circuit-as-"already-installed" behavior. |
| #15 | `--trust-tap` when `brew trust` unavailable / brew missing | **Still write the eval line**, skip only the trust half, exit 0 (user decision — graceful full-setup degradation; user retains shell integration). |

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | `--trust-tap` is a flag on existing `shell-install`, composes with modes; not a new command | Confirmed from intake #1; Constitution VII | S:95 R:80 A:90 D:90 |
| 2 | Certain | Genuine trust = `brew trust` ceremony + `HOMEBREW_REQUIRE_TAP_TRUST=1` policy, applied atomically | Confirmed from intake #2/#6; verified brew semantics | S:95 R:60 A:85 D:85 |
| 3 | Certain | Ceremony routes through `internal/proc`, implemented OUTSIDE shell_install.go (TestNoProcImports guard) | Upgraded — discovered the guard reading cli/shell-install memory; non-negotiable Constitution I + existing test | S:95 R:55 A:95 D:90 |
| 4 | Certain | Single combined block under NEW `# >>> shll >>>` sentinel; legacy blocks migrated in place | Confirmed from intake #4/#11 (clarified); user decision | S:95 R:45 A:75 D:80 |
| 5 | Certain | `--trust-tap` does full setup (both eval + export lines + ceremony) | Confirmed from intake #13 (clarified); user decision | S:95 R:55 A:75 D:80 |
| 6 | Certain | `--uninstall` removes the whole block (new + legacy sentinel); does NOT run `brew untrust` | Confirmed from intake #5/#12 (clarified); user decision | S:95 R:60 A:80 D:85 |
| 7 | Certain | `brew trust`/`untrust` idempotent → invoke unconditionally, no guard | Confirmed from intake #14 (empirically verified brew 5.1.14) | S:95 R:60 A:90 D:80 |
| 8 | Certain | Tap name `sahil87/tap` from a named constant, distinct from `formulaPrefix` (`sahil87/tap/`) | Confirmed from intake #8/#9; code-quality no-magic-strings | S:90 R:85 A:90 D:85 |
| 9 | Certain | Install becomes a per-line MERGE composing block body from union of managed lines | Confirmed from intake #7; forced by single-block decision | S:90 R:50 A:80 D:80 |
| 10 | Confident | Capability-probe `brew trust`; degrade without erroring; brew-absent handled via proc.ErrNotFound | Confirmed from intake #7-degradation; Constitution V + hasBrew precedent | S:80 R:65 A:85 D:75 |
| 11 | Confident | Preserve ALL existing invariants: symlink (O_APPEND install / EvalSymlinks+O_TRUNC uninstall), trailing-newline, never-creates-rc-files, exit-code policy | Derived from cli/shell-install memory — these are test-pinned; must not regress | S:85 R:55 A:90 D:80 |
| 12 | Confident | README gains Troubleshooting section + flag docs; states shll won't set env vars for the user | Confirmed from intake #10; explicit docs requirement | S:85 R:90 A:90 D:85 |
| 13 | Confident | Both-sentinels-present (corrupted) → MERGE into one new block | Clarified — agent default (safe, self-healing; refusing strands the user); review may revisit | S:75 R:50 A:65 D:65 |
| 14 | Confident | Partial/unclosed legacy block → REFUSE with diagnostic (exit 2), don't auto-repair | Clarified — agent default (guessing bounds risks corruption); deliberate divergence from current behavior | S:75 R:55 A:65 D:65 |
| 15 | Certain | When `--trust-tap` can't trust (old/missing brew), STILL write the eval line, skip only trust, exit 0 | Clarified — user chose graceful full-setup degradation over all-or-nothing | S:95 R:60 A:75 D:80 |

15 assumptions (10 certain, 5 confident, 0 tentative, 0 unresolved).
