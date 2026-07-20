# Intake: Reorder Roster to Leaves-First Dependency Order

**Change**: 260601-auvj-reorder-roster-leaves-first
**Created**: 2026-06-01
**Status**: Draft

## Origin

Conversational — emerged from a `/fab-discuss` session. The user proposed processing
sahil87 tools in **reverse dependency order** in `shll`, reasoning that "the tools shll
deals with have dependencies between them (e.g. fab-kit update gets wt also updated),
therefore deal with independent tools like idea and wt first."

The premise was pressure-tested during the discussion and **partially refuted**, which
reshaped the change from a *correctness fix* into a *presentation refinement*:

- **Refuted**: each `<tool> update` is self-update-only. Verified via `--help`:
  `fab-kit update` = "Update fab-kit itself"; `wt update` = "self-update the wt binary";
  `rk update` = "Update rk to the latest version" (+ daemon restart). **No tool's `update`
  cascades into another tool's upgrade.** So "fab-kit update gets wt also updated" does not
  hold at the `<tool> update` level.
- **Confirmed but handled by brew**: `brew deps` shows real formula dependencies
  (`fab-kit`→`wt,idea`; `hop`→`wt`). But brew already resolves these correctly and
  idempotently during each tool's *internal* `brew upgrade` — shll's loop order cannot
  break or improve that.

Decisions reached interactively (via AskUserQuestion):
1. **Goal = output coherence** (not "avoid redundant upgrades", not "fix an observed bug").
2. **Mechanism = static reorder of the shared `Roster`** (user chose shared over an
   update-only iteration order — single source of truth).
3. **Guard = invariant test** encoding all edges, labeled by kind (user chose over a
   comment-only approach).
4. **shell-init safety = verify first** — inspected `command {tu,wt,hop} shell-init zsh`;
   all blocks self-contained.
5. **Process = open a fab change** (this intake).

The user also supplied two runtime-invocation edges that `brew deps` cannot surface:
`rk riff`→`wt create`, and `hop open`/`hop ls --trees`→`wt`.

## Why

**Problem (the pain point).** `shll update` (and `install`) iterate the roster and stream
each tool's foregrounded output framed by a per-tool header (`▸ <tool>`) plus a summary
tail (`N of M tools succeeded`) — the per-tool-output-separation work (change y630). When a
*dependent* tool (e.g. `fab-kit`, which brew-depends on `wt`) runs its internal
`brew upgrade` and brew pulls up `wt` to satisfy a version constraint, a tool that shll may
have *already reported as done* under its own `▸ wt` header gets silently re-touched inside
the dependent's section. The per-tool framing then under-represents what actually happened:
`wt`'s real upgrade work can be split across `wt`'s own section and `fab-kit`'s section.

**Consequence if unfixed.** Purely cosmetic — the run is still *correct* (brew is
idempotent; the second touch is a near-instant no-op). But the per-tool headers and the
summary tail are the feature's whole reason for existing (legibility), and processing
dependents before leaves quietly undermines that legibility.

**Why this approach over alternatives.**
- *Reorder the roster (chosen)* — leaves processed first means each tool's section completes
  and is counted before any dependent's `brew upgrade` can re-touch it. Cheapest, no new
  brew calls, no new data model.
- *Rejected: add a `DependsOn` field + topological sort at runtime* — models the inter-tool
  dependency graph as data inside shll, which Constitution III/VII argues against (shll
  should not own a model of how the tools relate; the roster *list* is the contract).
- *Rejected: query `brew deps` at runtime* — more brew coupling, more latency, runtime
  discovery the constitution discourages for roster concerns.
- *Rejected: update-only iteration order (keep `Roster` as-is)* — the user explicitly chose
  a single shared source of truth over a second ordering for just `update`.

This is honestly **polish, not a bug fix** — the intake states that plainly so the spec
and review stages don't over-claim a correctness benefit.

## What Changes

### Reorder the shared `Roster` slice

`src/cmd/shll/tools.go` — change the order of the `Roster` literal from the current
declaration order to **leaves-first dependency order**.

**Current order:**
```go
var Roster = []Tool{
    {Name: "fab-kit", ...},
    {Name: "rk",      ...},
    {Name: "tu",      ...},
    {Name: "hop",     ...},
    {Name: "wt",      ...},
    {Name: "idea",    ...},
}
```

**New order (leaves first, dependents last):**
```go
var Roster = []Tool{
    {Name: "wt",      ...},  // leaf
    {Name: "idea",    ...},  // leaf
    {Name: "tu",      ...},  // leaf
    {Name: "rk",      ...},  // depends on wt (runtime: rk riff -> wt create)
    {Name: "hop",     ...},  // depends on wt (brew + runtime: hop open / hop ls --trees)
    {Name: "fab-kit", ...},  // depends on wt, idea (brew)
}
```

The per-entry fields (`Name`, `Formula`, `ShellInit`, `Update`) are unchanged — only the
ordering of the entries changes. Update the `Roster` doc comment to explain the leaves-first
rationale (currently it only mentions shell-init sequencing).

### Dependency graph driving the order

| From | To | Kind | Evidence |
|------|----|------|----------|
| `fab-kit` | `wt` | brew-upgrade | `brew deps sahil87/tap/fab-kit` |
| `fab-kit` | `idea` | brew-upgrade | `brew deps sahil87/tap/fab-kit` |
| `hop` | `wt` | brew-upgrade | `brew deps sahil87/tap/hop` |
| `hop` | `wt` | runtime | `hop open` delegates to wt's menu; `hop ls --trees` fans out `wt list --json` |
| `rk` | `wt` | runtime | `rk riff` shells out to `wt create` (help: "'wt' must be on your PATH") |

Leaves (no outgoing edges): `wt`, `idea`, `tu`. Dependents: `rk`, `hop`, `fab-kit`.
`fab-kit` is a pure dependent — nothing depends on it; no cycle. The chosen order
`wt, idea, tu, rk, hop, fab-kit` satisfies every edge (every dependent lands after all its
deps).

### Add an invariant test guarding the order

`src/cmd/shll/tools_test.go` (new or extended) — add `TestRosterLeavesBeforeDependents`:

- Encode the dependency edges as data in the test, **labeled by kind** via comments
  distinguishing `// brew-upgrade dep` from `// runtime-invocation dep`.
- Build a `name -> roster index` map from `Roster`, then assert for every edge that the
  dependent's index is strictly greater than each of its deps' indices.
- On failure, the message MUST name the offending edge (e.g.
  `"fab-kit (index N) must come after wt (index M)"`) so a future re-alphabetize fails
  loudly and legibly.
- The test guards the toolkit's **full ordering contract** (brew + runtime edges) — a
  superset of what output-coherence strictly requires (which depends only on the
  brew-upgrade edges). A comment in the test SHALL state this so a reader does not infer
  that `rk update` touches `wt` (it does not).

### Update order-sensitive golden-string tests

The roster order is consumed by three commands, so their order-sensitive assertions must be
updated to the new order (Test Integrity: the spec is the source of truth; tests conform):

- `update_test.go` — sequence/`▸ <tool>` header assertions, partial-install ordering.
- `install_test.go` — mirrors update's header/sequence assertions.
- `shell_init_test.go` — the integrator concatenation order flips from `tu, hop, wt` to
  `tu, wt, hop`.

These are *test fixture* updates to match the new declared order — not implementation
changes to accommodate tests.

## Affected Memory

- `cli/commands`: (modify) Roster invariants — the "Order matters" bullet should note the
  leaves-first dependency ordering and point to the rationale; the `Roster` code block in the
  doc reflects the new order.
- `cli/update`: (modify) the per-tool sequence / examples reflect leaves-first order.
- `cli/install`: (modify) same — install processes roster in the new order.
- `cli/shell-init`: (modify) integrator concatenation order is now `tu, wt, hop`.

(These memory updates happen at hydrate, after apply/review.)

## Impact

- **Code**: `src/cmd/shll/tools.go` (the `Roster` literal + its doc comment); new/extended
  `src/cmd/shll/tools_test.go`.
- **Tests**: `update_test.go`, `install_test.go`, `shell_init_test.go` golden strings.
- **Behavior**: `shll update`, `shll install`, `shll shell-init` all iterate in the new
  order. `shll version` also iterates the roster but its lines self-label, so order is
  cosmetic there (no golden-string churn expected, but verify).
- **No new dependencies, no new subcommand, no API change.** The `Tool` struct is unchanged
  (no `DependsOn` field added — Constitution III/VII).
- **Constitution**: III (Tool Roster Source of Truth) — the list stays hardcoded; only its
  order changes. VII (Minimal Surface Area) — no new command, so no new-command
  justification needed. The dependency model stays *implicit in slice order + test-enforced*,
  not encoded as a data field shll would own.

## Open Questions

- Does `shll version`'s output have any order-sensitive golden test that needs updating?
  (Expected no — version lines self-label — but confirm during apply.)

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Goal is output coherence, not correctness | Discussed — user explicitly chose "output coherence" over "avoid redundant upgrades" / "observed bug" via AskUserQuestion; each `<tool> update` verified self-update-only via `--help` | S:98 R:80 A:95 D:95 |
| 2 | Certain | New order is `wt, idea, tu, rk, hop, fab-kit` | Discussed — derived from the confirmed dependency graph (brew + runtime edges); user confirmed "fab-kit is a pure dependent, no cycle" and locked this order | S:98 R:70 A:90 D:90 |
| 3 | Certain | Reorder the shared `Roster`, not an update-only iteration order | Discussed — user chose "Reorder shared Roster" over "update-only order" via AskUserQuestion (single source of truth) | S:95 R:60 A:90 D:90 |
| 4 | Certain | Guard with `TestRosterLeavesBeforeDependents`, encoding all edges labeled by kind | Discussed — user chose "Invariant test" over "Comment only", and "Encode rk→wt too, labeled by kind" over "brew edges only" | S:95 R:85 A:90 D:90 |
| 5 | Certain | Do NOT add a `DependsOn` field — keep the dependency model implicit + test-enforced | Discussed — Constitution III/VII framing accepted; the roster list is the contract, the test enforces order | S:90 R:55 A:90 D:85 |
| 6 | Confident | shell-init reorder is safe; integrator order becomes `tu, wt, hop` | Verified — inspected `command {tu,wt,hop} shell-init zsh`; each block self-contained (defines own function, references nothing from another's block). hop→wt is binary/runtime, not shell-init-time | S:90 R:75 A:85 D:85 |
| 7 | Confident | Change type is `refactor` (behavior-preserving reorder + presentation polish) | No functional behavior changes; reordering a data structure + tests. Brew correctness unaffected | S:85 R:80 A:90 D:80 |
| 8 | Confident | Golden-string tests in update/install/shell_init must be updated to the new order | Roster order is consumed by all three; Test Integrity requires tests conform to the (re)declared order | S:90 R:80 A:90 D:85 |
| 9 | Confident | `shll version` needs no golden-string change (lines self-label) | version output self-labels per tool; order is cosmetic there. Flagged as an Open Question to confirm during apply | S:80 R:90 A:80 D:80 |

9 assumptions (5 certain, 4 confident, 0 tentative, 0 unresolved).
