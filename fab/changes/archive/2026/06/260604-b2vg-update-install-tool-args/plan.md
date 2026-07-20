# Plan: Per-tool targeting for `update` and `install`

**Change**: 260604-b2vg-update-install-tool-args
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### CLI: `shll update [tool...]` subset targeting

#### R1: Variadic positional args with unchanged zero-arg behavior
`shll update` SHALL accept zero or more positional tool-name arguments. With zero args, behavior MUST be byte-for-byte unchanged from today (whole-roster + shll self-upgrade). With one or more args, it MUST operate on only the named subset.

- **GIVEN** the user runs `shll update` with no positional args
- **WHEN** the command executes
- **THEN** it probes/upgrades the whole roster (plus shll self-upgrade when brew-installed), exactly as before
- **AND** `Args` is no longer `cobra.NoArgs` (it accepts any number of positional args)

#### R2: `shll` is a valid `update` target routing to the self-upgrade path
`shll update` SHALL accept the literal token `shll` as a target. When `shll` is named, it MUST route to the existing self-upgrade path (`brew upgrade sahil87/tap/shll`) and MUST keep its position as the first processed step (before the roster loop), exactly as in the whole-roster run.

- **GIVEN** the user runs `shll update shll`
- **WHEN** the command executes (and shll is brew-installed)
- **THEN** only the shll self-upgrade runs (`brew upgrade sahil87/tap/shll`), no roster tool is upgraded
- **AND** when the user runs `shll update shll hop`, the shll self-upgrade runs first, then `hop`

#### R3: Subset processed in roster (leaves-first) order regardless of arg order
The named subset SHALL be processed in `Roster` (leaves-first) order, independent of the order the args were supplied. When `shll` is among the targets it stays the first step.

- **GIVEN** the user runs `shll update fab-kit wt`
- **WHEN** the command executes
- **THEN** `wt` is processed before `fab-kit` (roster order, not arg order)

#### R4: Unknown target name is a hard up-front error
An argument that is neither a roster name nor (for `update`) `shll` SHALL cause a hard error: the command MUST exit non-zero, write a diagnostic to stderr that lists the valid target names, and perform NO brew/network work. Validation MUST happen before any probe or `brew update`.

- **GIVEN** the user runs `shll update hpo` (typo)
- **WHEN** the command executes
- **THEN** it writes an error to stderr naming the unknown arg and listing valid targets, returns `errSilent` (exit 1), and runs no `brew` subprocess
- **AND** when multiple args are unknown (e.g. `shll update foo bar`), all unknown args are reported

#### R5: Named-but-not-installed target is an error (update)
For `shll update`, a named target that is a valid name but is NOT installed (brew-not-installed) SHALL be an error — distinct from the whole-roster graceful skip. This applies to `shll` itself (e.g. a `go install` dev build that is not brew-installed).

- **GIVEN** only `hop` is installed, and the user runs `shll update rk`
- **WHEN** the command executes
- **THEN** it writes an error to stderr naming `rk` as not installed, returns `errSilent` (exit 1), and does not upgrade anything
- **AND** when the user runs `shll update shll` but shll is not brew-installed, the same not-installed error is produced for `shll`

#### R6: `brew update --quiet` still runs once for a subset
`shll update` SHALL run `brew update --quiet` exactly once, unconditionally, even for a single-tool subset target (after the nothing-to-do short-circuit cannot apply, since a validated installed subset is non-empty).

- **GIVEN** the user runs `shll update hop` (hop installed)
- **WHEN** the command executes
- **THEN** `brew update --quiet` runs exactly once before the per-tool upgrade

#### R7: Counter denominator and summary tail use subset size
For a subset run, the per-tool header `[N/M]` denominator and the summary tail `M` SHALL be the subset size (count of validated, processed targets including shll-self when selected), not the whole-roster count.

- **GIVEN** the user runs `shll update hop wt` (both installed, shll not selected)
- **WHEN** the command executes
- **THEN** headers read `[1/2] wt` and `[2/2] hop`, and the tail reads `Done — 2 of 2 tools succeeded in <dur>.`

#### R8: `--dry-run` previews the filtered subset
`shll update --dry-run [tool...]` SHALL preview only the validated subset, in roster order (shll-self first when selected), and exit 0 with no write. Unknown/not-installed validation still applies before the preview.

- **GIVEN** the user runs `shll update --dry-run hop wt`
- **WHEN** the command executes (both installed)
- **THEN** the preview header reads `Would update 2 tools (brew metadata refresh first):` and lists exactly `wt` then `hop`

### CLI: `shll install [tool...]` subset targeting

#### R9: Variadic positional args, symmetric with update, roster-only targets
`shll install` SHALL accept zero or more positional tool-name args, symmetric with `update`, with zero args preserving today's whole-roster behavior. The valid-target set for `install` is the six roster tools ONLY — `shll` is NOT a valid install target.

- **GIVEN** the user runs `shll install` with no args
- **WHEN** the command executes
- **THEN** it installs every missing roster tool, exactly as before
- **AND** `shll install hop wt` installs only the missing subset of {hop, wt}

#### R10: `shll` rejected as an install target
`shll install shll` SHALL produce the unknown-target hard error (R4 semantics) — you cannot `brew install` the running orchestrator.

- **GIVEN** the user runs `shll install shll`
- **WHEN** the command executes
- **THEN** it writes the unknown-target error to stderr, returns `errSilent` (exit 1), and runs no `brew install`

#### R11: install subset selection, counter, and dry-run mirror update
For `install`, the named subset SHALL be processed in roster order, the counter `M` SHALL be the count of named-and-missing targets, and `--dry-run` SHALL preview the filtered missing subset — mirroring `update` (R3, R7, R8) for the install lifecycle. A named target that is already installed is reported via the existing idempotent skip (it is filtered out of the install set, like the whole-roster behavior); unknown names still hard-error (R10).

- **GIVEN** the user runs `shll install fab-kit wt` and both are missing
- **WHEN** the command executes
- **THEN** it installs `wt` then `fab-kit` (roster order), headers read `[1/2]`/`[2/2]`, tail `Done — 2 of 2 …`
- **AND** when `shll install hop` is run and hop is already installed, it reports the all-already-installed (nothing-to-do) note and exits 0

### Internal: shared target resolver

#### R12: Single-sourced target resolver reused by both commands
A shared resolver in `cmd/shll` (alongside `Roster`) SHALL map positional args to the work set, single-sourced with `Roster` so the valid-name list cannot drift. It MUST: (a) validate each arg against the command's valid-target set (`update`: roster + `shll`; `install`: roster only); (b) on any unknown arg, return an error naming the offending arg(s) and the valid targets; (c) otherwise return the selected roster `Tool` entries in roster order plus a flag indicating whether shll-self was selected. The `shll` self-target token MUST be a named constant (no magic string).

- **GIVEN** both `runUpdate` and `runInstall` need to resolve args
- **WHEN** each calls the shared resolver
- **THEN** they obtain the same validation behavior and roster-ordered selection from one code path
- **AND** the `shll` token is a named constant

### Non-Goals

- `shll version [tool...]` — explicitly out of scope, deferred to a later change. The shared resolver introduced here is the seam it would reuse.
- No change to roster contents, ordering, or the `Tool` struct (no `DependsOn` field, no new fields).
- No change to the `--skip-brew-update` probe/delegation mechanics, or to `brew install`'s no-metadata-refresh contract.
- The `per-tool-output-separation` spec contract is unchanged — `M` is merely redefined as subset size. No new spec.

### Design Decisions

1. **Variadic positional args, not a `--only` flag** — *Why*: positional reads naturally, mirrors `brew upgrade <formula>`, user explicitly chose it. *Rejected*: `--only` flag (extra grammar, no upside).
2. **Resolver does name-validation only; install-status check stays in the run functions** — *Why*: `isInstalled`/`probeRoster` need brew context the resolver shouldn't own (keeps the resolver pure/data-only, single-sourced with `Roster`). The not-installed error (R5) is layered on after probing where install facts already exist. *Rejected*: folding brew probing into the resolver (couples a pure name-mapper to subprocess work, harder to unit test, duplicates probe logic).
3. **Filter the existing probe/upgrade loop rather than re-architecting** — *Why*: smallest diff, preserves all order-independent invariants (status line, single `brew update`, self-first, best-effort, tail, exit codes). For `update`, the subset is applied by zeroing `installed` on probe results for tools not in the selection, so the existing `total`/loop/dry-run code paths keep working with no structural change. *Rejected*: a parallel subset-only loop (divergence risk, duplicate code).
4. **Report all unknown args, not just the first** — *Why*: better one-shot fix for the user (intake Open Question; either acceptable, lean to all). *Rejected*: first-only (worse UX on multi-typo).
5. **Error message style follows existing stderr convention** — *Why*: matches `shll update: ...` / `shll install: ...` prefixes already used in the run functions. The valid-target list is appended so the user can self-correct.

## Tasks

### Phase 1: Shared resolver (foundation)

- [x] T001 Add the `shll` self-target named constant and the shared target resolver to `src/cmd/shll/tools.go`: `const shllTargetToken = "shll"`; a `resolveTargets(args []string, allowShll bool) (selected []Tool, selfSelected bool, err error)` that validates each arg against `Roster` names (plus `shllTargetToken` when `allowShll`), returns selected `Tool`s in roster order and `selfSelected`, and on any unknown arg returns an error naming all unknown args and listing valid targets. <!-- R12 -->

### Phase 2: `update` subset targeting

- [x] T002 In `src/cmd/shll/update.go`, change `newUpdateCmd` `Args` from `cobra.NoArgs` to `cobra.ArbitraryArgs`, update `Use`/`Long` to document the optional `[tool...]` and valid targets (roster + `shll`), and pass parsed `args` into `runUpdate`. <!-- R1 R2 -->
- [x] T003 In `src/cmd/shll/update.go`, add an `args []string` parameter to `runUpdate`. When `len(args) > 0`, call `resolveTargets(args, true)` UP FRONT (before `hasBrew`/status line/probe) and on error write `shll update: <detail>` to stderr and return `errSilent`. <!-- R4 R12 -->
- [x] T004 In `src/cmd/shll/update.go`, apply the resolved subset to the probe results: for a subset run, mark `probes[i].installed = false` for any roster tool not in the selection, and override `shllSelfInstalled` to consider only whether `shll` was selected. This makes `total`, the upgrade loop, the dry-run preview, and the summary tail all operate on the subset with no further structural change. <!-- R3 R6 R7 R8 -->
- [x] T005 In `src/cmd/shll/update.go`, enforce the named-but-not-installed error (R5): after probing, for each selected target (including shll-self) that is not installed, write `shll update: <name>: not installed` to stderr and return `errSilent` before any `brew update`/upgrade. Validate all selected targets before bailing. <!-- R5 -->

### Phase 3: `install` subset targeting

- [x] T006 In `src/cmd/shll/install.go`, change `newInstallCmd` `Args` to `cobra.ArbitraryArgs`, update `Use`/`Long` to document the optional `[tool...]` and roster-only valid targets (no `shll`), and pass parsed `args` into `runInstall`. <!-- R9 R10 -->
- [x] T007 In `src/cmd/shll/install.go`, add an `args []string` parameter to `runInstall`. When `len(args) > 0`, call `resolveTargets(args, false)` UP FRONT and on error write `shll install: <detail>` to stderr and return `errSilent`; otherwise restrict the roster walk that builds `missing` to the selected subset (roster order preserved). The counter `M = len(missing)`, dry-run, and tail then operate on the subset automatically. <!-- R10 R11 R12 -->

### Phase 4: Tests (test-alongside)

- [x] T008 [P] Add `update_test.go` cases: unknown name → `errSilent` + stderr lists valid targets + no `brew` call; named-but-not-installed (`shll update rk` with rk uninstalled) → `errSilent`; `shll update shll` self-upgrade-only path; `shll update shll` when shll not brew-installed → not-installed error; arg-order independence (`shll update fab-kit wt` → wt before fab-kit); `M` = subset size in headers/tail; `--dry-run` subset preview; zero-arg back-compat unchanged (existing tests still pass). <!-- R1 R2 R3 R4 R5 R6 R7 R8 -->
- [x] T009 [P] Add `install_test.go` cases: unknown name → error; `shll install shll` rejected (unknown-target error); arg-order independence (`shll install fab-kit wt` → wt before fab-kit); `M` = subset size in headers/tail; `--dry-run` subset preview; named-already-installed → nothing-to-do note; zero-arg back-compat unchanged. <!-- R9 R10 R11 -->
- [x] T010 [P] Add a `tools_test.go` (or resolver-focused) case for `resolveTargets`: roster-order selection regardless of arg order; `allowShll` gating (`shll` accepted when true, rejected when false); multiple unknown args all reported; empty args yields empty selection + `selfSelected=false`. <!-- R12 -->

## Execution Order

- T001 blocks T003, T004, T005, T007 (resolver must exist first).
- T002–T005 (update) and T006–T007 (install) are independent of each other once T001 lands.
- T008/T009/T010 follow their respective implementation tasks.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll update` accepts variadic positional args; zero-arg run is byte-for-byte unchanged (whole roster + self-upgrade); existing zero-arg `update_test.go` goldens pass.
- [x] A-002 R2: `shll update shll` routes to the self-upgrade path only; with `shll hop` the self step is first then hop.
- [x] A-003 R3: a subset is processed in roster order regardless of arg order (`fab-kit wt` → wt then fab-kit).
- [x] A-004 R6: `brew update --quiet` runs exactly once for a single-tool subset.
- [x] A-005 R7: per-tool header `[N/M]` and the summary tail use M = subset size.
- [x] A-006 R8: `shll update --dry-run hop wt` previews exactly the two-tool subset in roster order, exit 0, no write.
- [x] A-007 R9: `shll install` accepts variadic args; zero-arg run unchanged; `install hop wt` installs only that missing subset.
- [x] A-008 R11: install subset is processed in roster order; counter and dry-run use the subset; a named-already-installed target yields the nothing-to-do note.
- [x] A-009 R12: a single shared `resolveTargets` (single-sourced with `Roster`) is used by both commands; the `shll` token is a named constant.

### Behavioral Correctness

- [x] A-010 R1/R9: `Args` on both commands is `cobra.ArbitraryArgs` and the parsed args are threaded into `runUpdate`/`runInstall`.

### Edge Cases & Error Handling

- [x] A-011 R4: an unknown/typo'd `update` target hard-errors (exit non-zero), stderr lists valid targets, and NO brew subprocess runs (validated up front).
- [x] A-012 R4: multiple unknown args are all reported in one error.
- [x] A-013 R5: a named-but-not-installed `update` target (incl. `shll` on a non-brew dev build) errors with exit non-zero, distinct from the whole-roster graceful skip; nothing is upgraded.
- [x] A-014 R10: `shll install shll` is rejected with the unknown-target error (cannot brew-install the orchestrator).

### Scenario Coverage

- [x] A-015 R1–R12: each requirement above has at least one `update_test.go`/`install_test.go`/`tools_test.go` case exercising it (unknown, not-installed, self-target, rejected-self, arg-order, counter, dry-run, back-compat, resolver unit).

### Code Quality

- [x] A-016 Pattern consistency: new code follows the cobra factory + thin `runXxx` seam pattern, the named-constant convention, and the existing stderr message style (`shll update: …`).
- [x] A-017 No unnecessary duplication: target validation lives in one shared `resolveTargets` reused by both commands; no second copy of the valid-name list (single-sourced with `Roster`).
- [x] A-018 Magic strings/numbers: the `shll` self-target token is a named constant (`shllTargetToken`); no open-coded tool-name literals at call sites (code-quality.md anti-pattern).
- [x] A-019 Subprocess routing (Constitution I): no new `os/exec`; all subprocess work still routes through `internal/proc`; the resolver makes no subprocess calls.
- [x] A-020 Graceful Degradation honored (Constitution V): zero-arg behavior is unchanged; the explicit-named-tool errors are intentional (explicit naming ≠ graceful skip) and documented in this plan, not a violation.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate).
- The `per-tool-output-separation` spec contract still holds with `M` redefined as subset size — review should confirm headers/tail still conform.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Resolver validates names only; the brew install-status check (R5) stays in the run functions after probing | Intake says resolver returns selected Tools + selfSelected flag; install-status needs brew context the resolver shouldn't own. Keeps resolver pure/unit-testable, single-sourced with Roster. | S:90 R:75 A:85 D:85 |
| 2 | Certain | Subset applied to `update` by zeroing `installed` on non-selected probe results (reuse existing total/loop/dry-run/tail) | Smallest diff; preserves every order-independent invariant. Intake explicitly says filtering happens before the existing preview/loop blocks. | S:88 R:75 A:85 D:82 |
| 3 | Certain | `cobra.ArbitraryArgs` for both commands (replaces `cobra.NoArgs`) | Standard cobra idiom for variadic positionals; intake says "accept zero or more positional args". | S:95 R:85 A:90 D:90 |
| 4 | Confident | Report ALL unknown args (not just the first) in one error | Intake Open Question leaned this way ("better one-shot fix"); either acceptable, low blast radius. | S:80 R:80 A:80 D:70 |
| 5 | Confident | Error wording: `shll update: unknown target "x" (valid: shll, wt, idea, tu, rk, hop, fab-kit)` style; not-installed: `shll update: <name>: not installed` | Intake says follow existing `shll update: ...` stderr style and list targets; presentation detail, easily adjusted. | S:80 R:80 A:80 D:65 |

5 assumptions (3 certain, 2 confident, 0 tentative, 0 unresolved).
</content>
</invoke>
