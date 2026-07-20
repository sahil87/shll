# Intake: Per-tool targeting for `update` and `install`

**Change**: 260604-b2vg-update-install-tool-args
**Created**: 2026-06-04
**Status**: Draft

## Origin

This change was initiated conversationally via `/fab-discuss` → `/fab-new`. The user surfaced a recurring pain point:

> There is one problem I constantly face with this command. There's no way to update "only" the shll command. Should we have a variation of the update command that if a third argument is given (`shll update <toolname>`) then only that tool is updated? (this pattern is good even for `shll install`). Then one can do "shll update shll" to update only shll. What do you think?

The discussion explored the design space and settled the following decisions interactively (recorded as assumptions below):

- **Arity**: variadic (one-or-more positional args), not exactly-one. `shll update hop wt` is the same feature as `shll update shll`; restricting to one arg now would only be widened later.
- **Surface**: positional argument (`shll update <tool>`), not a `--only` flag. Reads naturally and mirrors `brew upgrade <formula>`.
- **`shll` as a target**: `shll update shll` MUST work (it is the motivating example) — it routes to shll's special self-upgrade brew path. `shll` is NOT in the `Roster` slice, so the valid-target set for `update` is the six roster tools **plus** `shll`.
- **Edge cases** (agreed in discussion): unknown name = hard error; named-but-not-installed = error; subset always processed in roster order; the run-wide `brew update --quiet` is preserved; `--dry-run` previews the filtered subset.

## Why

**Pain point.** Today `shll update` and `shll install` are all-or-nothing (`cobra.NoArgs`). The user frequently wants to upgrade a single tool — most commonly `shll` itself after a release — but the only options are running the whole roster (slow, noisy, touches tools they didn't intend to bump) or dropping down to the per-tool CLI / raw `brew upgrade`, which defeats the purpose of having a meta-CLI entry point.

**Consequence of not fixing.** The friction pushes the user *out* of shll for a common operation, eroding the "one entry point for cross-toolkit concerns" value proposition (context.md Overview). It also makes `shll update` unpleasant in tight feedback loops (e.g. iterating on a single tool's release and wanting to pull just that tool).

**Why this approach over alternatives.**
- *Variadic positional args* over a `--only` flag: positional reads naturally, mirrors the `brew upgrade <formula>` mental model the user already has, and keeps the grammar simple. The user explicitly chose this in discussion.
- *Hard error on unknown/uninstalled-named tools* over silent skip: the Constitution V graceful-degradation philosophy is about *uninstalled* tools in a whole-roster sweep ("don't make the user reason about what's installed"). Naming a tool explicitly **is** reasoning about it, so a typo or an absent named tool is almost certainly a mistake and must fail loudly — a silent no-op is the worst outcome (user believes they updated something they didn't).
- *Not a new subcommand*: this is positional args on two existing commands, so the Constitution VII (Minimal Surface Area) bar is the lower "new surface" bar, not the "new subcommand" bar. The justification (below) is straightforward.

**Constitution VII justification (Minimal Surface Area).** "Update/install a subset" cannot be expressed as a flag-free alternative on the existing commands (there is no current way to pass a selection), and it does not belong in a per-tool CLI (the whole point is composing *across* tools from one entry point). It adds no new top-level subcommand — only positional arguments to `update` and `install`. The whole-roster behavior is preserved verbatim at zero args, so nothing is taken away.

## What Changes

### `shll update [tool...]`

Change `Args: cobra.NoArgs` → accept zero or more positional args. The factory passes the parsed args into `runUpdate`.

- **Zero args** → today's behavior **exactly unchanged** (whole roster + shll self-upgrade). This is the back-compat anchor.
- **One or more args** → validate, then operate on just the named subset.

**Valid targets for `update`**: the six `Roster` names (`wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`) **plus** the literal `shll`. `shll` is special: it is not in `Roster`, and selecting it engages the existing self-upgrade path (`brew upgrade sahil87/tap/shll`, the `shllSelfLabel = "shll (self)"` step).

**Selection semantics:**
- The args form a *set*, not a sequence. Processing order is always **roster (leaves-first) order** regardless of the order args were given (see `shll-roster-order-rationale` memory — the order exists for output coherence so a dependent's internal `brew upgrade` doesn't re-touch a leaf already reported done). When `shll` is among the targets, it keeps its position as the **first** step (self-upgrade before the roster loop), exactly as today.
- Example: `shll update fab-kit wt` processes `wt` then `fab-kit` (roster order), not the arg order.
- Example: `shll update shll hop` → `shll (self)` first, then `hop`.

**Validation (up front, before any work):**
- An arg that is neither a roster name nor `shll` → **hard error**, exit non-zero, message lists the valid names. No brew/network work is performed. Validate *all* args first and report (at minimum the first) unknown name before doing anything.
- A named tool that is valid but **not installed** → **error** (distinct from the whole-roster skip). Naming it explicitly means the user expects it present; its absence is surfaced rather than silently skipped. (For `shll` itself: "named but not brew-installed" — e.g. a `go install` dev build — is the same error case.)

**Counter denominator `M`** in the per-tool header `[N/M]` and the summary tail becomes the **subset size** (count of validated, installed targets), not the whole-roster count. The spec `per-tool-output-separation` framing (per-tool headers, summary tail, TTY-gated color) stays valid — only the denominator changes.

**`brew update --quiet`**: still run **once, unconditionally**, even for a single-tool target. It is cheap and correctness-preserving; conditionalizing it would add a branch for marginal gain. The `--skip-brew-update` delegation logic per tool is unchanged.

**`--dry-run`**: previews only the validated subset, in roster order (shll-self first if targeted). Falls out naturally because filtering happens before the existing preview block.

### `shll install [tool...]`

Change `Args: cobra.NoArgs` → accept zero or more positional args, symmetric with `update`, with two differences:

- **Valid targets for `install` are the six roster tools ONLY** — `shll` is NOT a valid install target (you cannot `brew install` the running orchestrator). `shll install shll` → unknown-target error.
- The "named but not installed" case is the **happy path** for install (that is exactly what you'd install). The relevant inverse edge for install is: a named tool that is **already installed** → consistent with today's idempotent skip, but since it was named explicitly, it should still be reported (the existing all-installed short-circuit / per-tool skip messaging applies). The unknown-name hard error still applies.

**Selection semantics**, **counter denominator**, and **`--dry-run`** behave the same as `update`: roster-order processing, `M` = subset size, dry-run previews the filtered subset.

### Validation helper (shared)

A small shared resolver maps the positional args to the work set:
- Resolve each arg against the valid-target set for the command (`update`: roster + `shll`; `install`: roster only).
- On any unknown arg → return an error naming the offending arg(s) and listing valid targets; the command prints it and exits non-zero with no side effects.
- Otherwise return the selected `Tool` entries (in roster order) plus, for `update`, a flag indicating whether `shll`-self was selected.
- Keep it in `cmd/shll` alongside the roster (single-sourced with `Roster`); reuse it for both commands so the valid-name list can't drift.

### Non-goals

- **`shll version [tool...]`** is explicitly **out of scope** for this change. It was raised in discussion as a natural extension (the filtering would become a shared concern across three commands), but is deferred to keep this change focused. If pursued later, the shared resolver introduced here is the seam it would reuse.
- No change to the roster contents, ordering, or the `Tool` struct.
- No change to the `--skip-brew-update` probe/delegation mechanics.

## Affected Memory

- `cli/update`: (modify) document the new positional-arg subset targeting, the `shll`-as-target special case, unknown/uninstalled-named error behavior, roster-order processing of the subset, and `M` = subset size.
- `cli/install`: (modify) document the symmetric positional-arg subset targeting, roster-only valid targets (no `shll`), and the same error/order/counter semantics.

(The spec `per-tool-output-separation` does **not** need a new spec — its contract still holds with `M` redefined as subset size. The plan should note this explicitly so review confirms the headers/tail still conform.)

## Impact

**Code areas:**
- `src/cmd/shll/update.go` — `newUpdateCmd` (`Args`, pass args to `runUpdate`), `runUpdate` signature + filtering of the probe/upgrade loop and the dry-run preview, denominator `total`.
- `src/cmd/shll/install.go` — `newInstallCmd` (`Args`, pass args to `runInstall`), `runInstall` signature + filtering of `missing`/the install loop and the dry-run preview, denominator `total`.
- `src/cmd/shll/tools.go` (or a new small file) — shared target-resolution helper, single-sourced with `Roster`; a named constant for the `shll` self-target token.
- `src/cmd/shll/update_test.go`, `install_test.go` — new cases: unknown name, named-but-not-installed (update), `shll`-as-target (update), `shll`-rejected (install), arg-order independence, `M` = subset size, dry-run subset preview, zero-arg back-compat unchanged.

**APIs / dependencies:** No new external commands. Same `internal/proc` routing (Constitution I). No new third-party deps.

**Back-compat:** Zero-arg invocations are byte-for-byte unchanged. No scripts or muscle memory break.

**Help text:** `Use:` strings and `Long:` descriptions for both commands updated to document the optional `[tool...]` and the valid-target sets. (This also flows into `help-dump` output automatically — no separate work.)

## Open Questions

None blocking. The discussion resolved all material decisions. Two minor items the plan will pin down as Confident assumptions rather than questions:

- Exact wording of the unknown-target error message and whether it lists targets inline vs. on a second line (presentation detail; follow existing stderr message style — `shll update: ...`).
- Whether to report *all* unknown args or just the first when multiple are invalid (lean: report all for a better one-shot fix, but either is acceptable).

## Clarifications

### Session 2026-06-04 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 4 | Confirmed | — |
| 5 | Confirmed | — |
| 6 | Confirmed | — |
| 7 | Confirmed | — |
| 8 | Confirmed | — |
| 9 | Confirmed | — |
| 10 | Confirmed | — |

All seven Confident assumptions were settled during the preceding `/fab-discuss` session; user invoked "auto resolve" to bulk-confirm them as decided.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Variadic positional args (`[tool...]`); zero args = unchanged whole-roster behavior | Discussed — user explicitly chose variadic over exactly-one. Back-compat anchor is non-negotiable. | S:98 R:80 A:90 D:95 |
| 2 | Certain | Positional argument surface, not a `--only` flag | Discussed — user explicitly chose positional over flag. | S:98 R:75 A:85 D:95 |
| 3 | Certain | `shll` is a valid `update` target (self-upgrade path); NOT a valid `install` target | Discussed — user chose "include shll"; install-exclusion follows from "can't brew install the running orchestrator". | S:95 R:70 A:90 D:90 |
| 4 | Certain | Unknown/typo'd tool name → hard error, exit non-zero, validated up front before any work | Clarified — user confirmed. | S:95 R:65 A:80 D:85 |
| 5 | Certain | Named-but-not-installed tool → error (for `update`), distinct from whole-roster skip | Clarified — user confirmed. | S:95 R:60 A:75 D:80 |
| 6 | Certain | Subset always processed in roster (leaves-first) order regardless of arg order | Clarified — user confirmed. | S:95 R:70 A:85 D:88 |
| 7 | Certain | Keep the single unconditional `brew update --quiet` in `shll update` for subset targets | Clarified — user confirmed. | S:95 R:80 A:80 D:85 |
| 8 | Certain | `--dry-run` previews the filtered subset; `[N/M]` and summary tail use M = subset size | Clarified — user confirmed. | S:95 R:75 A:85 D:85 |
| 9 | Certain | Shared target-resolution helper single-sourced with `Roster`, reused by both commands | Clarified — user confirmed. | S:95 R:75 A:85 D:80 |
| 10 | Certain | `shll version [tool...]` is out of scope (non-goal), deferred to a later change | Clarified — user confirmed. | S:95 R:85 A:80 D:80 |

10 assumptions (10 certain, 0 confident, 0 tentative, 0 unresolved).
