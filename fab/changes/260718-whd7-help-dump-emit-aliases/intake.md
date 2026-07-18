# Intake: Help-Dump Emits Command Aliases

**Change**: 260718-whd7-help-dump-emit-aliases
**Created**: 2026-07-18

## Origin

One-shot `/fab-new` invocation. User's raw input:

> shll's help-dump command (src/cmd/shll/help_dump.go) only emits each cobra command's canonical Name(), never its Aliases() — so alias subcommands like 'shell-install' (an alias for shell-setup, defined at src/cmd/shll/shell_setup.go:56) don't appear in the generated help/shll.json, causing shll.ai's README-drift checker to flag 'shll shell-install' from the README as an unknown/nonexistent command even though it works fine as a real cobra alias. Fix: extend help-dump's tree-walk to also emit registered aliases for each command node (as additional Node entries, or an aliases field alongside name — pick whichever shape fits the existing help/*.json schema_version:1 contract used across all 7 tools without breaking it) so alias-form invocations are discoverable in the dump.

The one open design decision (Node-entry duplication vs. an `aliases` field) was explicitly delegated to the agent with a criterion: fit the frozen `schema_version: 1` contract without breaking it. The standard `docs/site/standards/help-dump.md` § Schema evolution resolves it: *"new fields MUST be added as **optional**, so each tool adopts them on its own release cadence — no seven-repo flag-day, and older captures keep validating."* → optional `aliases` field.

## Why

1. **Pain point**: `buildNode` (src/cmd/shll/help_dump.go:133) populates each node from `cmd.Name()`, `cmd.CommandPath()`, `cmd.Short`, `cmd.UseLine()`, and `nodeText(cmd)` — it never reads `cmd.Aliases`. shll has exactly one aliased command today (`shell-setup`, alias `shell-install`, src/cmd/shll/shell_setup.go:56), and that alias is invisible in the structured dump. The alias *does* appear inside the node's raw `text` field (cobra renders an `Aliases:` section in `-h` output), but consumers of the structured fields cannot see it without regex-parsing `text` — which the standard explicitly forbids as a discovery mechanism.
2. **Consequence if unfixed**: shll.ai's README-drift checker validates README-documented invocations against `help/shll.json` and flags `shll shell-install` as an unknown command — a standing false positive. The workarounds are all worse: drop the documented alias from the README (hides a real, supported invocation), or special-case the checker (per-tool exception lists rot). The dump is supposed to be drift-proof *because* it reads cobra's own data model; omitting `Aliases` makes it an unfaithful projection of the real CLI surface.
3. **Why this approach**: an optional `aliases` field on Node is the standard-sanctioned evolution path (additive optional field, no `schema_version` bump, per-tool adoption cadence). The rejected alternative — emitting aliases as additional Node entries — would fabricate tree structure: duplicate nodes sharing one command's `short`/`usage`/`text` but with synthesized `name`/`path` values cobra never registers as distinct commands, and it would break the dump's text↔commands coherence rule (each node's `Available Commands:` block in `text` lists canonical names only, so the `commands` array would diverge from the rendered text — the exact incoherence the prune-before-render rule exists to prevent).

## What Changes

### 1. `helpNode` gains an optional `aliases` field (src/cmd/shll/help_dump.go)

```go
type helpNode struct {
	Name     string     `json:"name"`
	Aliases  []string   `json:"aliases,omitempty"`
	Path     string     `json:"path"`
	Short    string     `json:"short"`
	Usage    string     `json:"usage"`
	Text     string     `json:"text"`
	Commands []helpNode `json:"commands"`
}
```

- Placed immediately after `Name` (alias data is name-adjacent; Go struct field order pins JSON key order).
- `omitempty` is deliberate and contrasts with `Commands` (a required v1 field, always `[]`, never `null`): `aliases` is an *optional* schema addition, so a command with no aliases emits **no** `aliases` key at all. Every currently-emitted node stays byte-identical; only aliased nodes change. This is what keeps older captures and non-adopting tools valid with zero consumer coordination.

### 2. `buildNode` reads `cmd.Aliases`

```go
return helpNode{
	Name:    cmd.Name(),
	Aliases: cmd.Aliases,
	Path:    cmd.CommandPath(),
	...
}
```

- Source is cobra's own data model (`cmd.Aliases`, a `[]string`) — consistent with the walk-never-parse rule.
- Emitted in declared order, no re-sorting (mirrors the existing order-preservation rule for `commands`).
- A nil or empty `Aliases` slice serializes to nothing under `omitempty` — no normalization needed.

Resulting node for `shell-setup` (elided):

```json
{
  "name": "shell-setup",
  "aliases": ["shell-install"],
  "path": "shll shell-setup",
  "short": "append the shll shell-init eval line to your rc file",
  "usage": "shll shell-setup [shell] [flags]",
  "text": "…",
  "commands": []
}
```

`schema_version` stays `1` — the standard reserves bumps for breaking shape changes; optional additive fields are the non-breaking path.

### 3. Standard document: `docs/site/standards/help-dump.md`

This repo is the canonical home of the 7-tool standard (Constitution § Toolkit Standards), and shll must not emit a field the standard doesn't define. Update the standard's **Output shape** Node example to include the optional field, e.g.:

```jsonc
{
  "name": "create",             // command name at this level
  "aliases": ["mk"],            // optional; alias names registered for this command — omitted when none
  "path": "wt create",          // full invocation path
  ...
}
```

plus a sentence (in Output shape or Schema evolution) stating: `aliases` is an optional additive field under `schema_version: 1`; producers SHOULD emit it when the framework exposes alias metadata (Cobra `cmd.Aliases`); consumers MUST treat alias-form invocations (`<path with name replaced by an alias>`) as valid commands. Other tools adopt on their own release cadence — no flag-day.

### 4. Embedded standards copy re-sync

`shll standards` serves a build-time embed of the standards documents: `src/cmd/shll/standards/help-dump.md` is a committed copy synced from `docs/site/standards/help-dump.md` via `scripts/sync-standards.sh`, drift-guarded by `TestStandardsEmbedMatchesCanonical`. After editing the canonical standard, run the sync script so the embedded copy matches — otherwise the drift-guard test fails the build.

### 5. Tests (src/cmd/shll/help_dump_test.go)

- **Contract-shape extension**: give the synthetic tree an aliased visible child → assert its node carries `"aliases": ["…"]` in declared order; assert unaliased nodes (and the root) have **no** `aliases` key (absence, not `[]`/`null`).
- **Real-tree assertion**: in the Execute-path dump (`dumpViaExecute` — tests MUST drive the real `rootCmd.Execute()` path per the prune-before-render design decision), the `shell-setup` node carries exactly `["shell-install"]`.
- **Structural determinism / byte-for-byte `text` tests**: unchanged — `aliases` adds no time-varying data and `nodeText` is untouched (cobra already renders the `Aliases:` help section independently).

## Affected Memory

- `cli/help-dump-contract`: (modify) — document the optional `aliases` node field: source (`cmd.Aliases`), `omitempty` absence semantics (vs. `commands`' always-`[]`), declared-order emission, no `schema_version` bump, and the standard's schema-evolution rule as the authority.
- `cli/standards-content`: (modify) — note the help-dump standard now defines the optional `aliases` field (first exercise of its § Schema evolution clause).

## Impact

- **Code**: `src/cmd/shll/help_dump.go` (struct + one `buildNode` line — minimal), `src/cmd/shll/help_dump_test.go`.
- **Docs**: `docs/site/standards/help-dump.md` (canonical) + `src/cmd/shll/standards/help-dump.md` (synced embed via `scripts/sync-standards.sh`).
- **Contract**: additive, non-breaking under `schema_version: 1`. Unaliased nodes byte-identical to today's output; the only byte-level delta in `help/shll.json` is the `shell-setup` node gaining one key.
- **Cross-repo (out of scope, producer-half only)**: shll.ai must (a) accept the optional field in its Zod validation of pulled dumps — the standard's evolution rule implies non-strict/forward-compatible validation, but if its schema is `.strict()` the next capture would fail and the puller would keep last-good — and (b) teach the README-drift checker to resolve alias-form invocations via `aliases`. The false positive on `shll shell-install` clears only once (b) lands; this change delivers the data it needs.
- **Other 6 tools**: unaffected; they adopt the field on their own release cadence per the standard.
- **Dependencies**: none added — stdlib `encoding/json` + existing cobra.

## Open Questions

*(none — the one delegated decision is resolved by the standard; see Assumptions)*

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Emit aliases as an optional `aliases` field on Node — NOT as duplicate Node entries | User delegated the shape with an explicit fit-the-contract criterion; standard § Schema evolution mandates optional additive fields; duplicate nodes would fabricate `name`/`path` values and break text↔commands coherence | S:85 R:70 A:95 D:90 |
| 2 | Confident | `omitempty` semantics — `aliases` key absent when a command has no aliases (never `[]`), field placed after `name` | Keeps every existing node byte-identical (only aliased nodes change), matching "optional field" evolution semantics; deliberate contrast with required-field `commands` (always `[]`) | S:60 R:75 A:85 D:70 |
| 3 | Certain | No `schema_version` bump | Standard reserves bumps for breaking shape changes and names optional additions as the non-breaking path; coordinated 7-tool bump explicitly not needed | S:70 R:65 A:95 D:95 |
| 4 | Confident | Scope includes updating the canonical standard doc + re-syncing the embedded copy | Constitution § Toolkit Standards: shll hosts the canonical standard and must conform to it — emitting an undocumented field would diverge; embed sync is forced by the `TestStandardsEmbedMatchesCanonical` drift guard | S:50 R:80 A:85 D:75 |
| 5 | Confident | shll.ai consumer-side changes (Zod schema tolerance, drift-checker alias resolution) are out of scope — this change is the producer half | User's fix statement scopes to help-dump's tree-walk; shll.ai is a separate repo with its own pipeline; noted as follow-up in Impact | S:75 R:70 A:80 D:80 |
| 6 | Certain | Aliases emitted in cobra-declared order, no sorting | Mirrors the dump's existing order-preservation rule (child order is cobra's, never re-sorted); single-alias reality makes this moot today but pins the rule for adopters | S:55 R:85 A:90 D:85 |

6 assumptions (3 certain, 3 confident, 0 tentative, 0 unresolved).
