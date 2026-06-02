# Spec: Reorder Roster to Leaves-First Dependency Order

**Change**: 260601-auvj-reorder-roster-leaves-first
**Created**: 2026-06-01
**Affected memory**: `docs/memory/cli/commands.md`, `docs/memory/cli/update.md`, `docs/memory/cli/install.md`, `docs/memory/cli/shell-init.md`

## Non-Goals

- **NOT a correctness fix.** Brew already resolves formula dependencies correctly and idempotently inside each tool's own `brew upgrade`; each `<tool> update` is self-update-only and no tool's `update` cascades into another tool's upgrade. The roster order cannot break or improve that. This change is output-coherence polish, not a behavior repair.
- **NOT a `DependsOn` data model.** The `Tool` struct gains no `DependsOn` field. Modeling the inter-tool dependency graph as data inside shll is rejected (Constitution III/VII) — the roster *list* is the contract; the dependency relationships stay implicit in slice order and are enforced only by a test.
- **NOT a runtime brew-deps query.** shll SHALL NOT call `brew deps` (or any runtime discovery) to derive the order — the order is statically encoded in the literal, consistent with the hardcoded-roster constitution.
- **NOT a new subcommand or flag.** No top-level surface area is added (Constitution VII), so no new-command justification is required.
- **NOT an update-only iteration order.** No second ordering is introduced for `shll update` alone — the single shared `Roster` is reordered so all three consumers (`update`, `install`, `shell-init`) share one source of truth.

## Roster: Ordering Contract

### Requirement: Leaves-First Roster Order

The `Roster` slice in `src/cmd/shll/tools.go` SHALL be declared in leaves-first dependency order: `wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`. Every tool that depends on another (by brew-upgrade or by runtime invocation) MUST appear after all of its dependencies in the slice. Only the entry order changes; each entry's `Name`, `Formula`, `ShellInit`, and `Update` fields MUST remain unchanged.

#### Scenario: Declared order is leaves-first

- **GIVEN** the `Roster` literal in `src/cmd/shll/tools.go`
- **WHEN** its entries are read top to bottom
- **THEN** the order is exactly `wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`
- **AND** `wt`, `idea`, and `tu` (the leaves — no outgoing dependency edges) precede `rk`, `hop`, and `fab-kit` (the dependents)

#### Scenario: Per-entry fields are preserved

- **GIVEN** the reordered `Roster`
- **WHEN** any entry is compared field-by-field against its pre-change definition
- **THEN** its `Name`, `Formula`, `ShellInit`, and `Update` values are identical
- **AND** no field is added to the `Tool` struct (no `DependsOn`)

### Requirement: Rationale Documented in the Roster Comment

The `Roster` doc comment SHALL explain the leaves-first dependency rationale. The current comment mentions only shell-init sequencing; it MUST be extended so a future reader understands that order also exists to keep each tool's per-tool output section (`shll update` / `shll install`) complete before any dependent's `brew upgrade` can re-touch it.

#### Scenario: Comment explains leaves-first intent

- **GIVEN** the `Roster` doc comment after this change
- **WHEN** a maintainer reads it
- **THEN** it states that order is leaves-first so dependents are processed after their dependencies
- **AND** it does not claim the order changes upgrade *correctness* (which brew owns)

## Roster: Invariant Test

### Requirement: TestRosterLeavesBeforeDependents Encodes All Dependency Edges

A test `TestRosterLeavesBeforeDependents` in `src/cmd/shll/tools_test.go` SHALL encode the full dependency graph as data and assert, for every edge `dependent -> dep`, that the dependent's index in `Roster` is strictly greater than the dep's index. The encoded edges MUST cover both kinds and be labeled by kind via comments:

- `fab-kit -> wt` — `// brew-upgrade dep`
- `fab-kit -> idea` — `// brew-upgrade dep`
- `hop -> wt` — `// brew-upgrade dep` AND `// runtime-invocation dep` (`hop open` delegates to wt's menu; `hop ls --trees` fans out `wt list --json`)
- `rk -> wt` — `// runtime-invocation dep` (`rk riff` shells out to `wt create`)

The test SHALL build a `name -> roster index` map from `Roster`, then check each edge. On violation, the failure message MUST name the offending edge with both indices (e.g. `"fab-kit (index N) must come after wt (index M)"`) so a future re-alphabetize or accidental reorder fails loudly and legibly.

#### Scenario: All edges satisfied by the leaves-first order

- **GIVEN** the `Roster` in leaves-first order
- **WHEN** `TestRosterLeavesBeforeDependents` runs
- **THEN** every encoded edge satisfies `index[dependent] > index[dep]`
- **AND** the test passes

#### Scenario: A regression to declaration/alphabetical order fails loudly

- **GIVEN** a hypothetical reorder that places a dependent before one of its deps (e.g. `fab-kit` ahead of `wt`)
- **WHEN** `TestRosterLeavesBeforeDependents` runs
- **THEN** the test fails
- **AND** the failure message names the specific offending edge and both indices

### Requirement: Test States the Contract Is a Superset of Output-Coherence

The invariant test SHALL carry a comment clarifying that it guards the toolkit's full ordering contract (brew-upgrade AND runtime-invocation edges) — a superset of what output-coherence strictly requires (which depends only on the brew-upgrade edges). The comment MUST prevent a reader from inferring that a runtime edge (e.g. `rk -> wt`) means `rk update` touches `wt` during `shll update` (it does not).

#### Scenario: Reader is not misled about runtime edges

- **GIVEN** the encoded `rk -> wt` runtime edge
- **WHEN** a maintainer reads the test
- **THEN** a comment makes clear this edge reflects runtime invocation (`rk riff -> wt create`), not an `rk update`-time upgrade of `wt`

## CLI: Update — Order-Sensitive Tests

### Requirement: update_test.go Conforms to the New Roster Order

`shll update` iterates `Roster` in order for its probe results, per-tool `▸`/`==>` headers, and partial-install sequencing. The order-sensitive assertions in `src/cmd/shll/update_test.go` SHALL be updated so the expected sequence matches the new leaves-first order. These are test-fixture updates to match the re-declared order (Test Integrity: the spec/declared order is the source of truth; tests conform — never the reverse). No `update.go` implementation logic changes.

#### Scenario: Header/sequence assertions reflect leaves-first order

- **GIVEN** a fully-installed roster
- **WHEN** `shll update` runs and `update_test.go` asserts the order of per-tool headers / recorded upgrade calls
- **THEN** the expected order is `wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit` (after the `shll (self)` step)
- **AND** the assertions pass without any change to `update.go`

#### Scenario: Order-independent invariants are untouched

- **GIVEN** the update behavior contract (brew-missing bail, status line, single `brew update`, self-upgrade-before-roster, best-effort loop, summary tail, exit codes)
- **WHEN** the roster is reordered
- **THEN** those invariants are unchanged
- **AND** golden strings that do not depend on inter-tool order (e.g. the `No sahil87 tools installed.` empty case) remain verbatim

## CLI: Install — Order-Sensitive Tests

### Requirement: install_test.go Conforms to the New Roster Order

`shll install` partitions and installs missing tools in `Roster` order, mirroring update's header/sequence framing. The order-sensitive assertions in `src/cmd/shll/install_test.go` SHALL be updated to the new leaves-first order, as test-fixture conformance — no `install.go` logic changes.

#### Scenario: Install sequence reflects leaves-first order

- **GIVEN** none of the roster tools are installed
- **WHEN** `shll install` runs and `install_test.go` asserts the order of per-tool headers / recorded `brew install` calls
- **THEN** the expected order is `wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`
- **AND** the assertions pass without any change to `install.go`

## CLI: Shell-Init — Composition Order

### Requirement: Integrator Concatenation Order Becomes wt, tu, hop

`shll shell-init` concatenates the output of installed integrators in `Roster` order (`runShellInit` iterates `Roster` in ascending index order). With the new order, the three shell-integrating tools appear in roster positions `wt` (index 0), `tu` (index 2), `hop` (index 4) — so the emitted concatenation order flips from `tu, hop, wt` to `wt, tu, hop` (ascending by index). The `shell_init_test.go` golden assertions SHALL be updated to this new order. Each block's `# ── <tool> ──` separator and the eval-safety invariants are unchanged; only the inter-block order changes.

#### Scenario: All integrators installed concatenate in wt, tu, hop order

- **GIVEN** `tu`, `wt`, and `hop` all installed
- **WHEN** `shll shell-init zsh` runs
- **THEN** stdout is `# ── wt ──` + wt's block, then `# ── tu ──` + tu's block, then `# ── hop ──` + hop's block
- **AND** the determinism guarantee (byte-identical output across consecutive runs) still holds

#### Scenario: Eval-safety is preserved across the reorder

- **GIVEN** the reordered composition
- **WHEN** any integrator is missing or errors
- **THEN** stdout remains eval-safe (no `▸`/`==>` header, no color, only `#`-prefixed separators and successful sub-tool bytes)
- **AND** a tool that is absent or errors emits neither its block nor its separator

## CLI: Version — Order-Sensitivity Verification

### Requirement: shll version Needs No Golden-String Change

`shll version` iterates `Roster` and prints one self-labeled line per tool. Its tests assert each printed line against the *same* `Roster` they iterate (index-paired: `lines[i+1]` is checked against `Roster[i].Name`), so reordering `Roster` reorders expected and actual in lockstep. No `version_test.go` change is therefore required. This was the intake's Open Question; it is resolved here as a verification item, not an ambiguity — confirmed by inspection of `version_test.go` (`for i, tool := range Roster { ... lines[i+1] ... }`).

#### Scenario: Version output stays correct after the reorder

- **GIVEN** the reordered `Roster`
- **WHEN** `shll version` runs with all tools installed
- **THEN** each line self-labels with its tool name and version
- **AND** `version_test.go` passes unchanged because its assertions are paired to `Roster` order rather than to a hardcoded sequence

## Design Decisions

1. **Reorder the shared `Roster` (not an update-only iteration order)**: Statically reorder the single shared `Roster` literal to leaves-first; all three consumers (`update`, `install`, `shell-init`) inherit the order.
   - *Why*: One source of truth. The user explicitly chose this over a second, update-scoped ordering during the discussion. A shared order keeps `version`, `install`, and `shell-init` consistent with `update` and avoids two diverging lists to maintain.
   - *Rejected*: An `update`-only iteration order (keeping `Roster` as declared and sorting at iteration time in `update.go`). Adds a second ordering concept, more code, and a divergence risk for marginal isolation benefit.

2. **Guard with an invariant test, not a comment-only note**: Add `TestRosterLeavesBeforeDependents` encoding all edges (brew + runtime) labeled by kind, asserting `index[dependent] > index[dep]` with an edge-naming failure message.
   - *Why*: A comment cannot fail CI. A future re-alphabetize or careless reorder must fail loudly and point at the exact offending edge. Encoding runtime edges too (a superset of what output-coherence needs) documents the toolkit's full ordering contract in executable form.
   - *Rejected*: A comment-only guard above the literal. No enforcement — silently rots the moment someone reorders for unrelated reasons.

3. **No `DependsOn` field on the `Tool` struct (Constitution III/VII)**: Keep the dependency model implicit in slice order plus the invariant test; do not add data fields or a runtime topological sort.
   - *Why*: shll is a meta-tool whose contract is the hardcoded roster *list* (Constitution III). It should not own a data model of how the tools relate to each other; brew already owns the brew graph, and runtime edges are the sub-tools' concern. Constitution VII argues against expanding shll's surface for something a static order + test already satisfies.
   - *Rejected*: (a) Add `DependsOn []string` and topologically sort at runtime — models the inter-tool graph as shll-owned data, more code, more failure modes. (b) Query `brew deps` at runtime — more brew coupling, more latency, runtime discovery the constitution discourages for roster concerns.

4. **Frame as output coherence, not correctness**: The spec, scenarios, and Non-Goals explicitly state this is presentation polish.
   - *Why*: The premise that "fab-kit update gets wt also updated" was pressure-tested and refuted — each `<tool> update` is self-update-only, and brew resolves formula deps idempotently. The only observable effect of the old order was that a dependent's internal `brew upgrade` could re-touch a leaf already reported done under its own `▸` header, under-representing the per-tool framing (change y630). Over-claiming a correctness benefit would mislead the review stage.
   - *Rejected*: Framing this as a bug fix / "avoid redundant upgrades." Inaccurate (the second touch is a near-instant idempotent no-op) and would invite scope creep toward a brew-deps query.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Goal is output coherence, not correctness | Confirmed from intake #1 — premise refuted during discussion (each `<tool> update` is self-update-only; brew resolves deps idempotently); spec frames this in Non-Goals + Design Decision #4 | S:98 R:80 A:95 D:95 |
| 2 | Certain | New order is `wt, idea, tu, rk, hop, fab-kit` | Confirmed from intake #2 — derived from the full dependency graph; satisfies every brew + runtime edge; `fab-kit` is a pure dependent, no cycle | S:98 R:70 A:90 D:90 |
| 3 | Certain | Reorder the shared `Roster`, not an update-only iteration order | Confirmed from intake #3 — single source of truth chosen by user; Design Decision #1 records the rejected alternative | S:95 R:60 A:90 D:90 |
| 4 | Certain | Guard with `TestRosterLeavesBeforeDependents`, encoding all edges labeled by kind, with edge-naming failure messages | Confirmed from intake #4 — invariant test over comment-only; encodes brew + runtime edges (superset of output-coherence need) | S:95 R:85 A:90 D:90 |
| 5 | Certain | Do NOT add a `DependsOn` field — dependency model stays implicit + test-enforced | Confirmed from intake #5 — Constitution III/VII; Design Decision #3 + Non-Goals record the rejected data-model and runtime-query alternatives | S:92 R:55 A:92 D:88 |
| 6 | Certain | shell-init integrator concatenation order becomes `wt, tu, hop` | Upgraded from intake #6 (Confident → Certain) — derivation from the locked new order is deterministic: integrators are wt@0, tu@2, hop@4, and `runShellInit` iterates `Roster` in ascending index order, so the concatenation is `wt, tu, hop`. (Corrected at review: intake and an earlier draft of this spec mis-sorted the indices to `tu, wt, hop`; the code-true order verified by `TestShellInit_*` is `wt, tu, hop`.) | S:95 R:75 A:92 D:90 |
| 7 | Certain | Change type is `refactor` | Upgraded from intake #7 (Confident → Certain) — already set in `.status.yaml` (per task brief); behavior-preserving reorder + test conformance, no functional change | S:95 R:80 A:95 D:90 |
| 8 | Certain | Golden-string tests in `update_test.go`, `install_test.go`, `shell_init_test.go` must be updated to the new order | Upgraded from intake #8 (Confident → Certain) — Test Integrity mandates tests conform to the declared order; all three consume `Roster` order; the affected assertions are identified per file | S:95 R:80 A:92 D:90 |
| 9 | Certain | `shll version` needs no golden-string change | Upgraded from intake #9 (Confident → Certain, Open Question resolved) — verified by inspecting `version_test.go`: assertions are index-paired to `Roster` itself (`for i, tool := range Roster { ... lines[i+1] ... }`), so reorder moves expected and actual in lockstep. Carried as a verification item, not [NEEDS CLARIFICATION] | S:95 R:90 A:95 D:90 |

9 assumptions (9 certain, 0 confident, 0 tentative, 0 unresolved).

<!-- Merged into plan.md ## Requirements on 2026-06-02 — safe to delete. -->
