# Intake: Roster Shell-Init Refresh

**Change**: 260509-tn8v-roster-shellinit-refresh
**Created**: 2026-05-09
**Status**: Draft

## Origin

This change was initiated to bring shll's `shell-init` composition into alignment with upstream
changes that have already shipped in two of the per-tool binaries:

- **`tu`** has gained a `shell-init <shell>` subcommand. It is now a third shell-integrating tool
  in the sahil87 toolkit, alongside `hop` and `wt`. Its argv must be added to shll's roster so
  `shll shell-init` composes its output.
- **`wt`** has renamed its `shell-setup` subcommand to `shell-init <shell>`. shll's roster currently
  invokes `wt shell-setup`; this argv must be updated to `wt shell-init <shell>`.

Mode: one-shot, mechanical roster refresh driven by upstream tool releases. There is no novel
design work — the per-tool CLIs are the source of truth (Constitution III, IV) and shll just
re-points its hardcoded argv slices.

> User-supplied description: refresh the shll shell-init tool roster to reflect upstream changes
> that have already shipped — add `tu` to the composition (`["tu", "shell-init", "<shell>"]`),
> and rename `wt`'s argv from `["wt", "shell-setup"]` to `["wt", "shell-init", "<shell>"]`. No
> backward compatibility for the old `wt shell-setup` invocation. Composition order: tu's position
> relative to hop and wt does not matter — leave it in the natural roster order (currently
> fab-kit, rk, tu, hop, wt, idea, which puts tu first among the three integrators).

Key decisions captured up front:

1. **No legacy fallback for `wt shell-setup`.** If a user has an outdated `wt` installed when
   they upgrade `shll`, the existing eval-safety contract (Constitution V; Design Decision #6)
   handles the failure gracefully: `wt`'s failed invocation drops its stdout, the error message
   goes to stderr, `shll shell-init` exits 1, but the user's shell still loads (degrading to no
   `wt` integration). No subcommand probing, no version sniffing, no fallback argv list.
2. **Composition order is incidental.** `tu` lands first among the three integrators only because
   that is its natural position in the existing `fab-kit, rk, tu, hop, wt, idea` roster order.
   This is not a designed sequencing decision — `tu`'s position relative to `hop` and `wt` does
   not matter for correctness.
3. **No new top-level subcommand.** The Constitution VII surface remains `update`, `shell-init`,
   `version`. This is roster maintenance, not surface-area expansion.

## Why

**Problem.** shll's hardcoded roster is the source of truth for shell-init composition
(Constitution III). Two upstream realities have drifted from what the roster currently encodes:

- `tu` has shipped a `shell-init` subcommand but shll does not invoke it, so users with `tu`
  installed get no `tu` integration loaded by `shll shell-init`. They would have to fall back to
  a separate `eval "$(tu shell-init zsh)"` line — defeating the "single eval line" value
  proposition shll exists for.
- `wt` has renamed `shell-setup` to `shell-init <shell>`. The current roster argv
  (`["wt", "shell-setup"]`) will fail outright on any `wt` installed at the new version, because
  the old subcommand no longer exists. Users with an up-to-date `wt` will hit the eval-safety
  failure branch (stderr error, dropped stdout, exit 1) on every `shll shell-init` invocation.

**Consequence if unfixed.** `shll shell-init` becomes incorrect for any user who has either tool
installed at a current version: missing integration for `tu`, broken integration for `wt`. Either
way the meta-CLI's job — produce a single concatenated init blob — is unfulfilled.

**Why this approach (roster argv update) over alternatives.** The roster is exactly the seam this
problem belongs at:

- *Alternative 1: dynamic discovery (probe each tool for its current shell-init subcommand).*
  Rejected — Constitution III explicitly forbids runtime discovery; the roster is the contract.
- *Alternative 2: legacy fallback for `wt shell-setup`.* Rejected — adds complexity (subcommand
  probing, error-class discrimination) for a transient compatibility concern. The eval-safety
  contract already gives users a non-fatal degradation path during the upgrade window. Per-tool
  CLIs continue to work standalone (Constitution IV), so a user with a stale `wt` can still run
  `eval "$(wt shell-setup)"` directly until they upgrade.
- *Alternative 3: hold the change until `tu` and `wt` both ship.* Already shipped — both upstream
  changes are out. shll is the lagging artifact.

The fix is exactly two lines in `tools.go`: add a `ShellInit` argv to `tu`'s entry, change `wt`'s.
Plus paired test and memory updates.

## What Changes

### 1. Roster (`src/cmd/shll/tools.go`)

Add `tu`'s `ShellInit` argv. Update `wt`'s argv. Final state of the integrating tools:

```go
{Name: "tu",  Formula: formulaPrefix + "tu",  ShellInit: []string{"tu", "shell-init", shellPlaceholder}},
{Name: "hop", Formula: formulaPrefix + "hop", ShellInit: []string{"hop", "shell-init", shellPlaceholder}},
{Name: "wt",  Formula: formulaPrefix + "wt",  ShellInit: []string{"wt", "shell-init", shellPlaceholder}},
```

Order in the full roster is unchanged: `fab-kit, rk, tu, hop, wt, idea`. The non-integrating tools
(`fab-kit`, `rk`, `idea`) are untouched — they still have an empty `ShellInit` slice and are
silently skipped by the composition loop.

After this change, three of six roster entries have a non-empty `ShellInit`. The composition order
they produce in `shll shell-init <shell>` output is `tu` first, then `hop`, then `wt`.

### 2. Tests (`src/cmd/shll/shell_init_test.go`)

The current test file covers the two-integrator world. It must be updated to cover three, with
linear per-tool skip-path coverage instead of combinatorial coverage.

**Updates to existing tests:**

- `TestShellInit_ZshBothInstalled` — rename to reflect three integrators (e.g.,
  `TestShellInit_ZshAllIntegratorsInstalled`). Update the expected concatenated stdout to include
  `tu`'s output, in order `tu` → `hop` → `wt`.
- `TestShellInit_DeterministicOrder` — extend the all-installed scenario to include `tu` so the
  byte-identical assertion covers all three integrators.
- `TestShellInit_BashHopOnly` — rename to `TestShellInit_OnlyHopInstalled`. It already proves the
  per-tool skip-path for the "only `hop`" case; the rename and shape adjust to fit the new
  per-tool linear coverage convention.
- `TestShellInit_SubToolFailure` — keep the scenario (one tool fails, others succeed, eval-safety
  holds) but update argv expectations to the new shapes.

**New tests (linear per-tool skip-path coverage, replacing any combinatorial instinct):**

- `TestShellInit_OnlyTuInstalled` — only `tu` installed; expected stdout is exactly `tu`'s output;
  no error to stderr; exit 0; `hop` and `wt` skipped silently.
- `TestShellInit_OnlyWtInstalled` — only `wt` installed; expected stdout is exactly `wt`'s output;
  exit 0; `tu` and `hop` skipped silently.

**Unchanged:**

- `TestShellInit_NoIntegratingToolsInstalled` — already covers the all-missing case.
- `TestShellInit_UnsupportedShell` and `TestShellInit_MissingShellArg` — argument-validation tests,
  unaffected by roster contents.

**Fake `proc.Runner` matcher updates:**

- Add a matcher that recognizes `tu shell-init <shell>` (returns canned `tu` stdout when "installed").
- Update `wt`'s matcher from `wt shell-setup` to `wt shell-init <shell>`.
- The substitution layer (`substituteShell`) still substitutes `<shell>` → `zsh`/`bash`, so the
  matcher should expect the substituted argv.

Final test list (after this change):

```
TestShellInit_ZshAllIntegratorsInstalled         (renamed from ZshBothInstalled)
TestShellInit_OnlyTuInstalled                    (new)
TestShellInit_OnlyHopInstalled                   (renamed from BashHopOnly)
TestShellInit_OnlyWtInstalled                    (new)
TestShellInit_NoIntegratingToolsInstalled        (unchanged)
TestShellInit_UnsupportedShell                   (unchanged)
TestShellInit_MissingShellArg                    (unchanged)
TestShellInit_DeterministicOrder                 (extended to three)
TestShellInit_SubToolFailure                     (argv updated)
```

### 3. Memory: `docs/memory/cli/shell-init.md`

Three localized updates:

- **Argv substitution table** — list all three integrators with their post-substitution argv:

  | Tool | Roster argv | After substitution (zsh) |
  |------|-------------|--------------------------|
  | `tu`  | `["tu", "shell-init", "<shell>"]`  | `["tu", "shell-init", "zsh"]`  |
  | `hop` | `["hop", "shell-init", "<shell>"]` | `["hop", "shell-init", "zsh"]` |
  | `wt`  | `["wt", "shell-init", "<shell>"]`  | `["wt", "shell-init", "zsh"]`  |

  Note: every integrator now substitutes the placeholder. The "no placeholder" footnote previously
  attached to `wt shell-setup` is removed — there is no longer an integrator without a `<shell>`
  argument.

- **Composition order prose** — replace "today only `hop` and `wt` produce output; in roster order
  that is `hop` first, then `wt`" with: "today `tu`, `hop`, and `wt` produce output; in roster
  order that is `tu` first, then `hop`, then `wt`. `tu`'s position is incidental — its
  natural place in the existing `fab-kit, rk, tu, hop, wt, idea` roster puts it first among the
  integrators, but ordering between the three is not a designed sequencing decision."

- **Test list** — refresh to match the per-tool linear coverage structure listed in the Tests
  section above. Replace `TestShellInit_ZshBothInstalled` with `TestShellInit_ZshAllIntegratorsInstalled`
  (and add per-tool coverage rows; remove the lone `TestShellInit_BashHopOnly` entry).

### 4. Memory: `docs/memory/cli/commands.md`

The `Hardcoded tool roster` section reproduces the `Roster` Go literal. Update it to the new
shape:

```go
var Roster = []Tool{
    {Name: "fab-kit", Formula: "sahil87/tap/fab-kit"},
    {Name: "rk",      Formula: "sahil87/tap/rk"},
    {Name: "tu",      Formula: "sahil87/tap/tu",  ShellInit: []string{"tu", "shell-init", "<shell>"}},
    {Name: "hop",     Formula: "sahil87/tap/hop", ShellInit: []string{"hop", "shell-init", "<shell>"}},
    {Name: "wt",      Formula: "sahil87/tap/wt",  ShellInit: []string{"wt", "shell-init", "<shell>"}},
    {Name: "idea",    Formula: "sahil87/tap/idea"},
}
```

Also update the bullet that currently says "`wt shell-setup` takes no shell arg, so its argv has
no placeholder" — after the change, every integrator's argv has a `<shell>` placeholder.

The roster invariants (`Six tools`, `Order matters`, `formulaPrefix`, etc.) are unchanged.

### 5. Out of scope (explicit non-changes)

- `update.go`, `version.go`, `brew.go`, `main.go`, `root.go` — untouched. This is purely a
  shell-init composition refresh.
- `fab/project/context.md` — the "Tool roster" table currently lists `tu` with "no" for shell-init.
  Updating that table is part of the **memory hydrate** step (driven by `/fab-continue` hydrate),
  not the apply step. Apply touches code + tests + memory; constitutional artifacts in `fab/`
  are addressed by hydrate or follow the project's separate update flow.

  _Subtle point_: `context.md` is a project-level reference, not a memory file. Whether it gets
  updated under this change or in a follow-up is a hydrate-stage decision. Flagging it here so
  spec/plan can address.

## Affected Memory

- `cli/shell-init`: (modify) — argv substitution table, composition order prose, and test list
  must reflect the three-integrator world (with `tu` and the renamed `wt`).
- `cli/commands`: (modify) — the `Hardcoded tool roster` Go literal and the surrounding bullet
  about argv placeholders must be updated to match the new roster.

## Impact

**Code areas changed (apply):**

- `src/cmd/shll/tools.go` — two roster lines: add `ShellInit` to `tu`, change `wt`'s argv.
- `src/cmd/shll/shell_init_test.go` — fake `proc.Runner` matchers, expected-output constants, test
  rename and additions per the per-tool linear coverage list above.

**Memory updated (hydrate):**

- `docs/memory/cli/shell-init.md`
- `docs/memory/cli/commands.md`

**Possibly touched (decided at hydrate):**

- `fab/project/context.md` — tool roster table's "Has shell-init/shell-setup?" column for `tu`
  becomes "yes (`shell-init`)", and `wt`'s entry changes from `shell-setup` to `shell-init`.

**Untouched:**

- `update.go`, `version.go`, `brew.go`, `main.go`, `root.go` — this change does not modify any
  other subcommand or shared helper.
- `internal/proc` — no change to subprocess wrapper behavior; the existing `proc.Run` already
  handles arbitrary argv slices.
- The Constitution — no principle additions or amendments. This is roster maintenance under the
  existing principles (especially III: Tool Roster Source of Truth, IV: Composition, V: Graceful
  Degradation).

**Dependencies / external assumptions:**

- Both upstream changes (`tu shell-init <shell>`, `wt`'s rename) have already shipped. shll is the
  lagging artifact. No release coordination is needed.
- Users with stale `wt` (still expecting `shell-setup`) gracefully degrade via the eval-safety
  contract — they get a stderr error and exit 1, but their shell still loads.

**Risk assessment:**

- *Risk*: A user upgrades `shll` before upgrading `wt`. Their next `shll shell-init` returns exit
  1 with a stderr complaint about `wt: shell-init: <error>`. *Mitigation*: documented behavior;
  Constitution V already covers this case. No code change required.
- *Risk*: `tu`'s `shell-init` subcommand has subtle differences from `hop`'s (e.g., output format).
  *Mitigation*: shll does not interpret sub-tool output — it concatenates verbatim. Whatever `tu`
  emits is just concatenated, eval-safety is preserved by the existing failure handling.
- *Risk*: Test rename churn breaks CI test parsers / dashboards that key on test names.
  *Mitigation*: project does not currently have such tooling (per `context.md` and codebase
  inspection); a single-snapshot rename is acceptable.

## Open Questions

- Should `fab/project/context.md`'s tool roster table be updated as part of this change's hydrate
  step, or deferred to a separate housekeeping change? (Tentative answer: include in hydrate, since
  the table is now stale and the change is small.)
- Should the test renames from `ZshBothInstalled` → `ZshAllIntegratorsInstalled` and `BashHopOnly`
  → `OnlyHopInstalled` keep the original shell name (`Zsh`/`Bash`) in the test name, or normalize
  to a shell-agnostic name now that the tests parameterize? (Confident default: drop the shell
  prefix to reflect the per-tool skip-path semantic; the test body still chooses a shell.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Add `tu` to shell-init composition with argv `["tu", "shell-init", "<shell>"]`. | User-supplied description states this verbatim; constitution III locks the roster as the seam. | S:95 R:80 A:95 D:95 |
| 2 | Certain | Rename `wt`'s `ShellInit` argv from `["wt", "shell-setup"]` to `["wt", "shell-init", "<shell>"]`. | User-supplied description states this verbatim; reflects upstream `wt` rename that has already shipped. | S:95 R:80 A:95 D:95 |
| 3 | Certain | No backward-compatibility fallback for `wt shell-setup`. Eval-safety contract handles the upgrade window. | User explicitly excluded this; Constitution V already provides graceful degradation; Design Decision #6 in shell-init.md preserves stdout eval-safety even on sub-tool failure. | S:90 R:60 A:90 D:90 |
| 4 | Certain | Composition order is `fab-kit, rk, tu, hop, wt, idea` (unchanged); integrators emit in order `tu, hop, wt`. | User stated `tu`'s position is incidental and to leave the natural roster order; current `tools.go` already places `tu` between `rk` and `hop`. | S:95 R:90 A:95 D:95 |
| 5 | Certain | Change type is `feat`. | User-supplied description explicitly classifies this as a `feat` (genuinely new tool integration via `tu`); SRAD inference would also pick `feat` (not `fix`/`refactor`/etc.). | S:95 R:95 A:95 D:95 |
| 6 | Certain | No new top-level subcommand; Constitution VII surface remains `update`, `shell-init`, `version`. | User stated this; constitution principle is preserved. | S:95 R:95 A:95 D:95 |
| 7 | Certain | Test strategy is per-tool linear skip-path coverage (one "only X installed" test per integrator), not combinatorial. | User explicitly directed: "Replace combinatorial-coverage instinct with linear per-tool skip-path coverage" with the exact test names. | S:95 R:70 A:90 D:90 |
| 8 | Confident | Rename existing `TestShellInit_ZshBothInstalled` to `TestShellInit_ZshAllIntegratorsInstalled` (drop shell-specific prefix style: keep `Zsh` in name since the test body uses zsh). | User said "rename it to reflect three integrators" without specifying the new name; "AllIntegrators" matches the per-tool linear coverage convention; keeping `Zsh` mirrors the existing convention (`BashHopOnly`). Spec/clarify can finalize. | S:60 R:80 A:75 D:65 |
| 9 | Confident | Update `fab/project/context.md` tool-roster table during hydrate (not a separate change). | Stale table is small; hydrate already touches related memory; user's "Affected memory" list mentions `commands.md` "potentially modify, if it references the roster" — same posture applies to context.md. | S:60 R:80 A:75 D:60 |
| 10 | Confident | Memory file `cli/commands.md` IS modified (not just "potentially"). | The `Hardcoded tool roster` section reproduces the Go literal verbatim, including `wt`'s argv and the "wt shell-setup takes no shell arg" footnote — both must update. Verified via direct read. | S:90 R:80 A:90 D:80 |
| 11 | Confident | `TestShellInit_SubToolFailure` argv expectations need updating (matcher must match new `wt shell-init <shell>` shape; test body's failure scenario can stay on `hop` or move to any tool). | User flagged "Update if needed to reflect the new argv shape" — the matcher absolutely needs the update; the failure scenario itself is interchangeable across tools. | S:75 R:80 A:80 D:70 |
| 12 | Confident | Constitution remains untouched. | User stated "Constitution implications: none." Mechanical roster refresh under existing principles. | S:90 R:95 A:95 D:90 |

12 assumptions (10 certain, 2 confident, 0 tentative, 0 unresolved).
