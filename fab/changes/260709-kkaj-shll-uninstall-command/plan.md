# Plan: shll uninstall Command

**Change**: 260709-kkaj-shll-uninstall-command
**Intake**: `intake.md`

## Requirements

### CLI: `shll uninstall` command surface

#### R1: New top-level `uninstall` subcommand
The binary SHALL expose a new top-level `shll uninstall [tool...]` subcommand, registered in `newRootCmd()` and added to the `rootLong` subcommand list (surface 8 → 9 user-facing; Constitution VII justified in the intake).

- **GIVEN** the shll binary is built
- **WHEN** the user runs `shll uninstall --help`
- **THEN** cobra prints usage for the `uninstall` subcommand and it appears in `shll --help`'s subcommand list

#### R2: No-args sweep = all installed roster tools (shll-self excluded)
With no positional args, `shll uninstall` SHALL uninstall every *installed* roster tool (probed via the brew keg seam `probeInstalledLeaf`/`isInstalled`), in reverse-roster order. Tools not installed SHALL be skipped gracefully with a `not installed` line, never an error (Constitution V). shll itself SHALL NOT be included in the no-args sweep.

- **GIVEN** `hop`, `wt` are installed and `idea`, `tu`, `run-kit`, `fab-kit` are not
- **WHEN** the user runs `shll uninstall --yes`
- **THEN** `brew uninstall sahil87/tap/hop` and `brew uninstall sahil87/tap/wt` are issued (dependents-first order), the four uninstalled tools each report `not installed`, no `brew uninstall sahil87/tap/shll` is issued, and the run exits 0

#### R3: Targeted subset with `shll` legal (explicit-only), unknown-target hard error
`shll uninstall <tool...>` SHALL resolve args via `resolveTargets(args, true)` (`allowShll=true`) so `shll uninstall shll` is legal and explicit-only. Unknown targets SHALL be a hard error listing the valid targets (same diagnostic contract as update). The legacy alias `rk` SHALL resolve to `run-kit` with the shared `note: rk is now run-kit` notice.

- **GIVEN** the user runs `shll uninstall bogus`
- **WHEN** the args are resolved
- **THEN** `shll uninstall: unknown target "bogus" (valid targets: shll, wt, idea, tu, run-kit, hop, fab-kit)` is written to stderr and the run exits 1 with no brew side effect

#### R4: Named-but-not-installed exits 0 (repair-path semantics)
A named target that is not installed SHALL report `not installed` and NOT be treated as an error — unlike `shll update`, absence is a success state for uninstall (the goal "gone" is already met). The run SHALL exit 0 when every named target is absent (and none failed).

- **GIVEN** `hop` is not installed
- **WHEN** the user runs `shll uninstall hop --yes`
- **THEN** stdout reports `hop` as `not installed`, no `brew uninstall` is issued, and the run exits 0

#### R5: Reverse-roster order; shll-self processed last
Actionable tools SHALL be processed in reverse-roster order (dependents before leaves: `fab-kit, hop, run-kit, tu, idea, wt`), the mirror of install's leaves-first coherence. When `shll` is a named target it SHALL be processed **last**, after all roster tools.

- **GIVEN** the user runs `shll uninstall shll hop wt --yes` with all three installed
- **WHEN** the removals execute
- **THEN** `hop` and `wt` are uninstalled in reverse-roster order (hop before wt) and `shll` is uninstalled last

### CLI: confirmation gate

#### R6: Removal-plan print + `Proceed? [y/N]` prompt by default
Absent `--yes`, `shll uninstall` SHALL print a removal plan (per tool: name, formula, installed version) then prompt `Proceed? [y/N] `. Only an explicit affirmative (`y`/`yes`, case-insensitive) SHALL proceed; anything else SHALL abort with exit 0 and no write.

- **GIVEN** `hop` is installed and stdin supplies `n\n`
- **WHEN** the user runs `shll uninstall hop` (TTY stdin, no `--yes`)
- **THEN** the removal plan and prompt are printed, no `brew uninstall` is issued, and the run exits 0

#### R7: `--yes`/`-y` bypasses the prompt
The `--yes` (short `-y`) bool flag SHALL skip the plan/prompt entirely and proceed directly to removal (scripting path).

- **GIVEN** `hop` is installed
- **WHEN** the user runs `shll uninstall hop --yes`
- **THEN** no prompt is printed and `brew uninstall sahil87/tap/hop` is issued

#### R8: Non-TTY stdin without `--yes` refuses (fail-safe)
When stdin is not a TTY and `--yes` was not passed, `shll uninstall` SHALL refuse with a stderr hint to pass `--yes`, exiting non-zero, with no write (fail-safe for pipes/CI).

- **GIVEN** stdin is a pipe (non-TTY) and `--yes` was not passed
- **WHEN** the user runs `shll uninstall hop`
- **THEN** a stderr hint naming `--yes` is written, no `brew uninstall` is issued, and the run exits 1

#### R9: `--dry-run` previews exact brew commands, exit 0, no write
`--dry-run` SHALL print the exact `brew uninstall` commands the run would execute (aligned-column preview via the shared `ui.go` helper, sourced from a single-source-of-truth argv builder) and exit 0 with no write. `--dry-run` SHALL bypass the confirmation gate (no prompt, no non-TTY refusal — a preview mutates nothing).

- **GIVEN** `hop`, `wt` are installed
- **WHEN** the user runs `shll uninstall --dry-run` (even with non-TTY stdin, no `--yes`)
- **THEN** a `Would uninstall N tools:` preview lists `brew uninstall sahil87/tap/hop` / `...wt` in reverse-roster order, exits 0, and issues no `brew uninstall`

### CLI: run-kit dual-name sweep

#### R10: run-kit uninstall is probe-then-act with leaf verification (never blind old-name)
When run-kit is an actionable target, `shll uninstall` SHALL NEVER issue a blind `brew uninstall sahil87/tap/rk` (post-rename brew resolves `rk` → `run-kit` and would delete the good keg). It SHALL: (1) probe `sahil87/tap/run-kit`; if leaf `run-kit` is installed → `brew uninstall sahil87/tap/run-kit`; then (2) probe the legacy formula and remove a residual `rk`-leaf keg only when confirmed (leaf == `rk`), via `brew uninstall rk`. New name first, then the residual legacy keg.

- **GIVEN** a dual-rack machine: `run-kit` (leaf `run-kit`) installed AND a separate `rk` keg (leaf `rk`) present
- **WHEN** the user runs `shll uninstall run-kit --yes`
- **THEN** `brew uninstall sahil87/tap/run-kit` is issued first, then `brew uninstall rk` for the confirmed residual keg, and no blind `brew uninstall sahil87/tap/rk` is issued before the leaf is verified as `rk`

#### R11: run-kit named via legacy keg / legacy alias still uninstalls
On a legacy-only machine (only the `rk` keg present, leaf `rk`), `shll uninstall run-kit` (and `shll uninstall rk` via the alias) SHALL treat run-kit as installed and remove the residual `rk`-leaf keg (`brew uninstall rk`) rather than erroring `not installed`.

- **GIVEN** only the legacy `rk` keg is installed (current `run-kit` formula absent)
- **WHEN** the user runs `shll uninstall run-kit --yes`
- **THEN** run-kit counts as installed, `brew uninstall rk` is issued, and the run exits 0

### CLI: shll-self uninstall

#### R12: shll-self uninstall gated on brew management, farewell note
An explicit `shll uninstall shll` SHALL uninstall shll only when `probeInstalledVersion(shllFormula)` reports it brew-managed; a `go install`/local-build shll SHALL error `not brew-managed` (same fact update.go's self-upgrade path keys on). When brew-managed it SHALL run `brew uninstall sahil87/tap/shll` (processed last), and print a farewell note pointing at the reinstall path (`brew install sahil87/tap/shll`). The running process keeps working (unix unlink semantics).

- **GIVEN** shll is brew-managed
- **WHEN** the user runs `shll uninstall shll --yes`
- **THEN** `brew uninstall sahil87/tap/shll` is issued last and a farewell note naming `brew install sahil87/tap/shll` is printed
- **AND GIVEN** shll is a dev build (not brew-managed), **THEN** `shll uninstall: shll: not brew-managed` is written to stderr and shll is not brew-uninstalled

### CLI: output, exit codes, and hints

#### R13: Per-tool output separation + summary tail + failure aggregation
`shll uninstall` SHALL follow the `per-tool-output-separation` spec: per-tool `▸/==>` headers via `ui.go` helpers, a summary tail (`N removed, M skipped, K failed` semantics via `printSummaryTail`), TTY-gated color, ASCII degradation. A failed `brew uninstall` SHALL be recorded, the loop SHALL continue to the next tool, and the command SHALL exit non-zero at the end (update.go aggregation pattern). Skips (`not installed`) SHALL NOT be failures and SHALL NOT flip the exit code.

- **GIVEN** two tools are actionable and one `brew uninstall` exits non-zero
- **WHEN** the run completes
- **THEN** both tools were attempted, per-tool headers framed each, the summary tail reflects the failure, and the run exits 1

#### R14: Post-run hints are print-only (never executed)
When run-kit was removed, `shll uninstall` SHALL print (never run) a note that a running run-kit daemon is not stopped (`run-kit serve --stop`). When shell-integrated tools (`tu`, `hop`, `wt`) were removed **roster-wide** (a no-args or full-roster sweep, not a partial subset), it SHALL print a note pointing at `shll shell-setup --uninstall` for the rc-file block. shll SHALL never execute these (Constitution III).

- **GIVEN** a no-args sweep removes `hop` (a shell-integrated tool) and `run-kit`
- **WHEN** the run completes
- **THEN** a `run-kit serve --stop` hint and a `shll shell-setup --uninstall` hint are printed to stdout, and neither is executed

### Non-Goals

- **No untap** of `sahil87/tap` and no trust revocation — the tap stays for reinstall.
- **No config/state purge** (rk daemon state, hop data, rc-file edits) — brew-uninstall semantics, not `--zap`.
- **No stopping of running processes** (run-kit daemon) — print the hint only (Constitution III).
- **No `shll uninstall` → `shll install` composite** — the repair recipe stays two explicit commands.
- **No `--force`/rack-targeted escalation** in v1 — plain `brew uninstall rk` is the assumed-sufficient orphan-removal action; code is shaped so escalation is a small change if needed (see Assumption 8).

### Design Decisions

1. **Confirmation gate reads an injected `io.Reader` stdin + a TTY seam**: `runUninstall` takes an explicit `stdin io.Reader` (wired from `cmd.InOrStdin()`), and a `stdinIsTTY(io.Reader) bool` helper (mirroring `ui.go`'s `colorEnabled`) detects a non-`*os.File`/non-terminal stdin. — *Why*: matches the established writer-injection test seam (bytes.Buffer / strings.Reader in tests hit the non-TTY branch deterministically); no global `os.Stdin` reference in command code. — *Rejected*: reading `os.Stdin` directly (untestable without process-level fixtures).
2. **run-kit removal is a dedicated `uninstallRunKit` action reusing the leaf parser**: mirrors `migrateRunKit`'s structure — probe current formula, act on leaf `run-kit`, then re-probe legacy formula and act only on a confirmed `rk` leaf. — *Why*: single detection path (Constitution III), never a blind old-name uninstall (the session-observed footgun). — *Rejected*: a blind `brew uninstall sahil87/tap/rk` (deletes the migrated keg post-rename).
3. **Reverse-roster via slice iteration, not a second declared order**: iterate the actionable set built in roster order, then reverse it (or iterate `Roster` backwards). — *Why*: one source of truth (the `Roster` slice); the reverse is derived, mirroring the leaves-first rationale. — *Rejected*: a second hardcoded dependents-first slice (drift risk).

## Tasks

### Phase 1: Setup

- [x] T001 Add `stdinIsTTY(r io.Reader) bool` helper to `src/cmd/shll/ui.go` (mirrors `colorEnabled`: true only when `r` is an `*os.File` AND `term.IsTerminal(fd)`); a `bytes.Buffer`/`strings.Reader` test reader deterministically returns false. <!-- R6 R8 -->

### Phase 2: Core Implementation

- [x] T002 Add `brewUninstallArgv(formula string) []string` (returns `{brewBinary, "uninstall", formula}`) and the run-kit residual argv `{brewBinary, "uninstall", <LegacyName>}` single-source-of-truth builders in `src/cmd/shll/uninstall.go` — shared by the live run and the dry-run preview so they cannot drift (mirrors `upgradeArgv`). <!-- R9 R10 --> <!-- rework: A-020 — brewUninstallArgv feeds only the dry-run preview; the live run (uninstallOne, uninstall.go:389) open-codes the identical argv. Thread the builder into the live run: argv := brewUninstallArgv(formula); proc.RunForeground(ctx, argv[0], argv[1:]...) — same for the run-kit residual argv. -->
- [x] T003 Create `src/cmd/shll/uninstall.go` with `newUninstallCmd()` (`Use: "uninstall [tool...]"`, `cobra.ArbitraryArgs`, `--dry-run` reusing `dryRunFlag`/`dryRunFlagUsage`, and a new `--yes`/`-y` bool flag via `cmd.Flags().BoolP`), its `RunE` threading `cmd.InOrStdin()`, `cmd.OutOrStdout()`, `cmd.ErrOrStderr()`, `dryRun`, `yes`, and `args` into `runUninstall`. <!-- R1 R7 R9 -->
- [x] T004 Implement `runUninstall(ctx, stdin io.Reader, stdout, stderr io.Writer, dryRun, yes bool, args []string) error` in `uninstall.go`: resolve targets up front via `resolveTargets(args, true)` (unknown → stderr + `errSilent`); `hasBrew` guard (install-style hint constant); `printAliasNotices`; classify each considered tool into an actionable set (probe install status, run-kit via the leaf gate, named-but-not-installed → `not installed` skip line at exit 0 per R4, whole-roster-absent → skip); build the actionable set in reverse-roster order with shll-self last. <!-- R2 R3 R4 R5 R11 -->
- [x] T005 Implement the shll-self classification in `runUninstall`: only when `selfSelected` (never in the no-args sweep); gate on `probeInstalledVersion(shllFormula)` — not brew-managed → `shll uninstall: shll: not brew-managed` on stderr + `errSilent`; brew-managed → actionable, appended LAST. <!-- R12 -->
- [x] T006 Implement the confirmation gate in `runUninstall`: when not `--yes` and not `--dry-run`, print the removal plan (aligned name/formula/version rows) then prompt `Proceed? [y/N] `; if `!stdinIsTTY(stdin)` refuse with a `--yes` hint on stderr + `errSilent`; else read one line via `bufio.NewReader(stdin).ReadString('\n')` and proceed only on a case-insensitive `y`/`yes` (else abort, exit 0, no write). <!-- R6 R7 R8 -->
- [x] T007 Implement the `--dry-run` branch in `runUninstall`: after the actionable set is built (reads only), print the `Would uninstall N tools:` preview via `printUninstallPreview` (new `ui.go` header const `uninstallPreviewHeaderFmt` + reuse `printPreviewRows`), rows sourced from `brewUninstallArgv` / the run-kit argv builder in reverse-roster order, exit 0, no write, bypassing the confirmation gate. <!-- R9 -->
- [x] T008 Implement the removal loop in `runUninstall`: per-tool `printToolHeader` (reverse-roster `[N/M]`, section spacing, color decision computed once), dispatch each actionable tool — run-kit → `uninstallRunKit(ctx, stdout, stderr, t)`, shll-self → `brew uninstall shllFormula` + farewell note (`brew install sahil87/tap/shll`), all others → the `brewUninstallArgv`-sourced `proc.RunForeground` call; best-effort aggregation (record failure, continue), capture `start := nowFunc()` after the short-circuits; summary tail via `printSummaryTail`; return `errSilent` iff any failed. <!-- R5 R12 R13 --> <!-- rework: A-019/A-020 — route the live uninstall through brewUninstallArgv (see T002 rework), and promote the inline "Nothing to uninstall." user-facing literal (uninstall.go:230) to a named constant per the allInstalledMsg convention. -->
- [x] T009 Implement `uninstallRunKit(ctx, stdout, stderr, t Tool) (failed bool)` in `uninstall.go`: probe `t.Formula`; if installed (leaf `run-kit`) → `brew uninstall t.Formula`; then re-probe `t.LegacyFormula` and, only when leaf == `t.LegacyName` (`rk`), `brew uninstall t.LegacyName` for the residual keg; never a blind old-name uninstall before the leaf is verified. Aggregate failure across both steps. <!-- R10 R11 -->
- [x] T010 Implement the post-run hints in `runUninstall` (print-only): track whether run-kit was removed and whether the run was a roster-wide sweep that removed shell-integrated tools (`tu`/`hop`/`wt`); after the loop, print the `run-kit serve --stop` hint (named const) when run-kit was removed and the `shll shell-setup --uninstall` hint (named const) when shell-integrated tools were removed roster-wide. Never execute either. <!-- R14 --> <!-- rework: A-013 — (a) "roster-wide" must include a NAMED full-roster sweep (all six roster tools listed explicitly), not just the no-args case: key on coverage of the roster set, not !subset; (b) success-gate the rc-unwire hint on shell-integrated tools actually REMOVED (mirror runKitRemoved's success gating), not merely attempted; (c) pass the run-kit tool name from the actionable entry (a.tool.Name), not a "run-kit" literal (A-019). -->

### Phase 3: Integration & Edge Cases

- [x] T011 Wire `newUninstallCmd()` into `newRootCmd()` (`src/cmd/shll/root.go`) and add the `shll uninstall` line to `rootLong`'s subcommand list (surface 8 → 9 user-facing). <!-- R1 -->
- [x] T012 Add the brew-missing hint constant `uninstallBrewMissingHint` (`"shll uninstall requires Homebrew. Install from https://brew.sh"`) to `src/cmd/shll/brew.go`, matching the install/update per-command-hint convention. <!-- R1 -->

### Phase 4: Tests

- [x] T013 Write `src/cmd/shll/uninstall_test.go` (test-alongside) driving `runUninstall` with `bytes.Buffer` writers, a `strings.Reader`/`bytes.Buffer` stdin, the shared `fakeRunner`/`installFakeRunner` seam, and `installFakeClock` for the duration tail. Cover: no-args sweep skips missing + reverse order (R2/R5); targeted named-missing exits 0 (R4); unknown-target hard error (R3); prompt abort on `n` (R6); `--yes` bypass (R7); non-TTY refusal (R8); dry-run preview parity + no writes + gate bypass (R9); dual-rack sweep ordering + never-blind-old-name (R10); legacy-keg/`rk`-alias run-kit uninstalls (R11); self-uninstall brew-managed + not-brew-managed gating + farewell (R12); failure aggregation → exit 1, skips not failures (R13); post-run hints print-only (R14). <!-- R1 R2 R3 R4 R5 R6 R7 R8 R9 R10 R11 R12 R13 R14 -->
- [x] T014 Run `cd src && gofmt -l . && go vet ./... && go test ./...`; fix any failures. Verify `help/` needs no fixture regeneration (help-dump is emitted at runtime, not stored in this repo — per `cli/help-dump-contract`). <!-- R1 R13 -->

## Execution Order

- T001 and T002 are independent scaffolding (can precede everything).
- T003 depends on T002 (argv builders) and T001 (TTY seam).
- T004–T010 build `runUninstall` incrementally in one file; execute in listed order (T004 establishes the classification skeleton the rest extend).
- T011–T012 (wiring) can run after T003 (the factory exists).
- T013 depends on the full implementation (T001–T012); T014 is last.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll uninstall` is a registered top-level subcommand, appears in `shll --help`, and `shll uninstall --help` prints its usage.
- [x] A-002 R2: A no-args run uninstalls every installed roster tool (reverse-roster order), skips uninstalled ones with `not installed`, never touches shll, and exits 0.
- [x] A-003 R3: A targeted run resolves via `resolveTargets(args, true)`; `shll` is a legal explicit target; an unknown target hard-errors with the valid-target list and exits 1 with no brew side effect; `rk` resolves to `run-kit` with the shared alias notice.
- [x] A-004 R4: A named-but-not-installed target reports `not installed` and exits 0 (not an error).
- [x] A-005 R5: Actionable tools are removed dependents-first (`fab-kit, hop, run-kit, tu, idea, wt`), and shll-self is processed last when named.
- [x] A-006 R6: Without `--yes`/`--dry-run`, a removal plan + `Proceed? [y/N]` prompt is printed and only an affirmative proceeds; a negative aborts at exit 0 with no write.
- [x] A-007 R7: `--yes` (and `-y`) skip the prompt and proceed to removal.
- [x] A-008 R8: Non-TTY stdin without `--yes` refuses with a `--yes` hint, exits non-zero, no write.
- [x] A-009 R9: `--dry-run` prints the exact `brew uninstall` commands (reverse-roster), exits 0, performs no write, and bypasses the confirmation gate even on non-TTY stdin. Dual-rack parity added (finding 5): the preview now sources its rows from the run-kit classification facts (`runKitNewInstalled`/`runKitLegacyKeg`), so a dual-rack target previews BOTH the new-formula uninstall and the residual `brew uninstall rk`, and a legacy-only machine previews only `brew uninstall rk` — matching the live sweep exactly.
- [x] A-010 R10: run-kit removal issues `brew uninstall sahil87/tap/run-kit` first, then `brew uninstall rk` only for a leaf-verified residual keg; a blind `brew uninstall sahil87/tap/rk` is never issued before leaf verification.
- [x] A-011 R11: A legacy-only or `rk`-aliased run-kit target is treated as installed and removes the residual `rk` keg (exit 0).
- [x] A-012 R12: shll-self uninstall runs `brew uninstall sahil87/tap/shll` last + farewell note when brew-managed; errors `not brew-managed` (no brew uninstall) on a dev build.
- [x] A-013 R14: Post-run hints (`run-kit serve --stop`; `shll shell-setup --uninstall` for roster-wide shell-integrated removals) are printed to stdout and never executed. The rc-unwire hint now keys on roster-set COVERAGE (`rosterWide := len(consider) == len(Roster)`), so a NAMED full-roster sweep qualifies alongside the no-args case; and both hints are success-gated on a tool actually REMOVED (`runKitName`/`shellIntegratedRemoved` set only on `!failed`), never on a failed attempt.

### Behavioral Correctness

- [x] A-014 R13: A per-tool `brew uninstall` failure is recorded, the loop continues, and the run exits non-zero; the summary tail reflects the failure. A `not installed` skip is not a failure and does not flip the exit code.
- [x] A-015 R13: Per-tool `▸/==>` headers, section spacing, TTY-gated color, and ASCII degradation follow the `per-tool-output-separation` spec (shared `ui.go` helpers; `bytes.Buffer` writers exercise the plain-ASCII branch).

### Scenario Coverage

- [x] A-016 R10: The dual-rack cleanup scenario (`run-kit` + separate `rk` keg) removes both in the correct order (new formula first, residual `rk` keg second).

### Edge Cases & Error Handling

- [x] A-017 R1: brew missing → `shll uninstall`-specific stderr hint, exit 1, no brew side effect.
- [x] A-018 R6: A malformed/empty prompt response (EOF, whitespace, `maybe`) is treated as "no" → abort, exit 0, no write.

### Code Quality

- [x] A-019 Pattern consistency: `uninstall.go` mirrors `install.go`/`update.go` conventions — `internal/proc` for all subprocesses (Constitution I), named constants for all literals (no magic strings), shared `ui.go` helpers reused (not re-implemented), one-file-per-subcommand + paired `_test.go`. The two remaining literals are now named: `"Nothing to uninstall."` → `uninstallNothingMsg` (mirroring `allInstalledMsg`), and the daemon hint names the run-kit tool from the actionable entry (`runKitName = a.tool.Name`) instead of a `"run-kit"` string literal.
- [x] A-020 No unnecessary duplication: the run-kit leaf gate reuses `probeInstalledLeaf`/`parseBrewLeaf`; the reverse order is derived from the single `Roster` slice; the brew-uninstall argv is a single-source-of-truth builder shared by the live run and the dry-run preview. The live run now sources its argv from `brewUninstallArgv` inside `uninstallOne` (`argv := brewUninstallArgv(formula); proc.RunForeground(ctx, argv[0], argv[1:]...)`), so preview and run cannot drift — matching the `upgradeArgv`/`upgradeTool` precedent. The dry-run preview shares the same builder (via `previewRowsFor`).

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | New `shll uninstall [tool...]` subcommand; no-args = all installed roster tools; shll-self explicit-only, processed last; reverse-roster order | Intake §1 + Assumption 1 (Certain) — user's ask plus discussed-and-accepted design sketch | S:85 R:80 A:90 D:85 |
| 2 | Confident | Confirmation gate: removal-plan print + `Proceed? [y/N]`; `--yes`/`-y` bypass; non-TTY stdin without `--yes` refuses; `--dry-run` bypasses the gate | Intake §2 + Assumption 2 (Confident); a preview mutates nothing so gating it is pointless, matching install/update --dry-run's short-circuit-before-write shape | S:55 R:85 A:80 D:70 |
| 3 | Confident | Confirmation reads an injected `io.Reader` stdin (`cmd.InOrStdin()`) + a `stdinIsTTY` seam in ui.go mirroring colorEnabled | No existing stdin-reading command, but the writer-injection test seam is the established pattern; a `bytes.Buffer`/`strings.Reader` reader deterministically hits the non-TTY branch, matching how colorEnabled is tested | S:45 R:80 A:80 D:75 |
| 4 | Certain | run-kit sweep is probe-then-act with leaf verification; old-name uninstall never issued blind; new name first, residual `rk`-leaf keg second | Intake §3 + Assumption 4 (Certain); session-observed rename resolution proves a blind old-name uninstall deletes the good keg; mirrors migrateRunKit's structure | S:80 R:75 A:85 D:80 |
| 5 | Confident | shll-self uninstall gated on `probeInstalledVersion(shllFormula)`, `not brew-managed` error otherwise; farewell note with `brew install sahil87/tap/shll` | Intake §4 + Assumption 7 (Confident); mirrors update.go's self-upgrade brew-managed gating | S:50 R:85 A:80 D:70 |
| 6 | Confident | Output per per-tool-output-separation spec (ui.go headers/tail/color/ASCII); failure aggregation → non-zero exit; named-but-missing exits 0; skips are not failures | Intake §5 + Assumption 6 (Confident); existing spec + update.go aggregation precedent; repair-path semantics make absence a success | S:55 R:85 A:85 D:75 |
| 7 | Confident | Post-run hints print-only: `run-kit serve --stop` when run-kit removed; `shll shell-setup --uninstall` when shell-integrated tools (tu/hop/wt) removed roster-wide (not a partial subset) | Intake §5 (Constitution III print-don't-run); scoping the shell-setup hint to roster-wide removals avoids a misleading rc-unwiring nudge on a single-tool subset that leaves other integrated tools present | S:50 R:85 A:75 D:65 |
| 8 | Tentative | Orphan-rack removal is plain `brew uninstall rk`; `--force`/rack-targeted escalation deferred, code shaped (single-source argv builder) so escalation is a small change | Intake §3 + Assumption 8 (Tentative) + Open Question — post-rename brew behavior for an orphan `rk` rack is unverified and cannot be exercised in fake-runner tests; author's dual-rack machine is the live verification point at ship time | S:35 R:75 A:40 D:40 |
| 9 | Certain | Rework (A-013): "roster-wide" keys on roster-set COVERAGE (`len(consider) == len(Roster)`), so a named full-roster sweep qualifies alongside no-args; both post-run hints are success-gated on a tool actually removed | resolveTargets returns a de-duplicated roster-ordered set, so full coverage is exactly the length equality; success-gating mirrors the existing runKit hint's `!failed` gate — a mechanical, low-risk derivation | S:80 R:85 A:90 D:85 |
| 10 | Confident | Rework (finding 5): dry-run preview parity carries the run-kit classification facts (`runKitNewInstalled`/`runKitLegacyKeg` on `uninstallTarget`) into `previewRowsFor` rather than re-probing; the live `uninstallRunKit` keeps its own independent re-probe (unchanged, T009) | The preview must render the SAME argv the live sweep issues; sourcing it from the classification probe (already run in dry-run) keeps a single fact source for the preview without disturbing the live-run's fresh-state re-probe design, and fake-runner determinism makes the two agree | S:55 R:85 A:80 D:75 |

10 assumptions (3 certain, 6 confident, 1 tentative).

## Deletion Candidates

- `suggestDualRackFmt` (`src/cmd/shll/doctor.go:73`, call site `doctor.go:365`) — superseded advice now that `shll uninstall run-kit` exists: it still suggests the bare `'shll uninstall'` (which is now a whole-roster sweep, not the targeted cleanup) and the qualified `'brew uninstall sahil87/tap/rk'` — the rename-re-resolving footgun form this change deliberately removed from `update.go`'s twin `migrationDualRackNoteFmt`. Align it to `'shll uninstall run-kit' (or 'brew uninstall rk')` in a follow-up (outside this change's diff; also surfaced as a should-fix review finding).
- *Prior candidate resolved*: `migrationDualRackNoteFmt` (`src/cmd/shll/update.go`) was reworked in this change — it now points at `shll uninstall <tool>` with the leaf-name brew alternative (`brew uninstall rk`), both call sites plus the state-C golden updated — so it is no longer a candidate.
- Otherwise none — this change adds new functionality (a new subcommand reusing existing probes/helpers) without making existing code redundant.
