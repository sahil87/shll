---
type: memory
description: "Hidden `shll help-dump` subcommand — the frozen `help/<tool>.json` JSON contract (shared 7-tool, `wt.json` reference) and producer rules (programmatic cobra walk, filter completion/help/Hidden, prune-before-render, the optional `omitempty` `aliases` node field from `cmd.Aliases`)."
---
# cli/help-dump-contract

The frozen `help/<tool>.json` contract and the rules for producing it. shll is one of the 7 toolkit tools that each expose a machine-readable export of their CLI surface for `sahil87/shll.ai`, which renders an expandable "Command reference" per tool page. shll.ai **pulls** this export on a schedule (its change `oa63`): it `brew install`s each tool and runs the tool's `help-dump` (see [ci/release-workflow](/ci/release-workflow.md)). **The contract is shared and frozen across all 7 tools** — the reference sample is shll.ai's `help/wt.json`, which is a *post-capture* file (it carries the shll.ai-stamped `captured_at`). shll's producer mirrors that shape **minus `captured_at`**: the tool-emitted stdout envelope is `{tool, version, schema_version, root}`, and shll.ai adds `captured_at` when it stores the pulled document. Do not change the JSON shape without a coordinated 7-tool bump of `schema_version`.

Source: `src/cmd/shll/help_dump.go` (producer), `src/cmd/shll/help_dump_test.go` (conformance). `help-dump` emits the document to stdout; shll.ai's scheduled puller (`scheduled-help-refresh.yml`, on shll.ai's side) consumes it. This repo's release workflow publishes nothing to shll.ai (7huv — see [ci/release-workflow](/ci/release-workflow.md)).

## The JSON contract (frozen — schema_version 1)

The **tool-emitted envelope** — exactly what `shll help-dump` writes to stdout — is a single JSON object:

```json
{
  "tool": "shll",
  "version": "v0.5.0",
  "schema_version": 1,
  "root": { "...Node..." }
}
```

`captured_at` is **shll.ai-owned**: shll.ai's puller stamps it onto the captured document post-capture, so the *stored* `help/<tool>.json` (e.g. the `wt.json` reference) does carry it — but the tool-emitted stdout envelope above MUST NOT. §3 of the contract forbids the tool emitting it — a tool cannot know its own capture time. (7huv)

Top-level field meanings (field order is contractual — encoded via Go struct field order, see below):

| Field | Meaning |
|-------|---------|
| `tool` | literal `"shll"` (constant `helpDumpTool`). |
| `version` | the binary's version — read from `cmd.Root().Version` (ldflags-stamped `main.version`), **never hardcoded**. When shll.ai's puller `brew install`s shll, this is the released tag (`v0.5.0`); a local unstamped build emits `dev`. |
| `schema_version` | literal int `1` (constant `helpDumpSchemaVersion`). Bump only on a breaking shape change, coordinated across all 7 tools. |
| `root` | the recursive `Node` tree, anchored at the cobra root command. |

A **Node** is recursive:

```json
{
  "name": "shell-setup",
  "aliases": ["shell-install"],
  "path": "shll shell-setup",
  "short": "append the shll shell-init eval line to your rc file",
  "usage": "shll shell-setup [shell] [flags]",
  "text": "<RAW -h output, byte-for-byte, newlines preserved>",
  "commands": []
}
```

`aliases` is **optional** and appears only on a command that has aliases (see [The optional `aliases` field](#the-optional-aliases-field)); a command with none — `install` above would be one — emits no `aliases` key. Per-node field source (programmatic, from cobra's data model — **never regex on `-h`**):

| Field | Source |
|-------|--------|
| `name` | `cmd.Name()` |
| `aliases` | `cmd.Aliases` (optional, `omitempty` — see below; key absent when the slice is nil/empty). |
| `path` | `cmd.CommandPath()` (e.g. `"shll"`, `"shll install"`) |
| `short` | `cmd.Short` |
| `usage` | `cmd.UseLine()` (e.g. `"shll install [flags]"`) |
| `text` | the command's raw `-h` output, byte-for-byte — see [`text` construction](#text-byte-for-byte). |
| `commands` | recursive `[]Node` over **visible** children (after filtering); serialized as `[]` for leaves, never `null`. |

The Go structs (`helpDoc`, `helpNode`) pin field order and JSON tags. The document is encoded with `json.MarshalIndent(doc, "", "  ")` (2-space indent) plus a **single trailing newline**, and nothing else is written to stdout — so CI can redirect `> help/shll.json` cleanly (honors the project's per-tool output separation: diagnostics → stderr, payload → stdout).

## Producer rules

These are the durable invariants the producer must uphold for the dump to stay coherent and contract-faithful.

### Programmatic tree walk, never regex

`runHelpDump(root, w)` walks the live cobra tree via `cmd.Commands()` recursively (`buildNode`), reading cobra's own data model — the same source `-h` renders from. It cannot drift from the real CLI and survives cobra formatting changes. Regex-parsing `-h` text is explicitly rejected by the contract.

### Child filtering (`shouldSkip`)

Applied to every node's **children**, recursively (the root is the dump anchor and is never filtered). A child is skipped when ANY holds:

- `cmd.Name() == "completion"` — cobra auto-generated (constant `cmdNameCompletion`).
- `cmd.Name() == "help"` — cobra auto-generated (constant `cmdNameHelp`).
- `cmd.Hidden == true` — this **self-excludes `help-dump`**, which is itself `Hidden: true`.
- `!cmd.IsAvailableCommand()` — defensive; covers deprecated/unavailable commands.

### Prune-before-render (the text↔commands coherence rule)

This is the subtle, load-bearing rule. The real binary invokes `help-dump` via `rootCmd.Execute()`, which **lazily registers cobra's `completion` and `help` subcommands BEFORE the matched `RunE` fires** — so at walk time they exist as live children of root. The `commands` array correctly omits them (via `shouldSkip`), but `nodeText` renders `cmd.UsageString()`, whose `Available Commands:` block reflects the *live* children. Without intervention the root's `text` would list `completion`/`help` while its `commands` array omits them — internally incoherent and divergent from the frozen `wt.json` reference.

Resolution: `pruneSkipped(root)` runs **before** building any node. It force-registers cobra's lazy `help`/`completion` (`InitDefaultHelpCmd` / `InitDefaultCompletionCmd` — idempotent no-ops if absent or already present), then recursively `RemoveCommand`s every skip-listed child from the live tree, recursing only into survivors. After pruning, every node's `UsageString()` `Available Commands:` block lists exactly its surviving `commands` entries.

> **Design Decision: prune the live tree, not just filter the array.**
> *Why*: An earlier implementation filtered only the `commands` array and built `text` from a tree that still held `completion`/`help`, producing an incoherent split (text lists them, array omits them) that also diverged from `wt.json`. The earlier assumption — that `text` comes from a walk that never sees `completion`/`help` — was WRONG for the real binary because `Execute()` registers them before `RunE`. Pruning the live tree first is the fix; verified end-to-end against the Execute-built binary and guarded by an Execute-path regression test (`TestHelpDump_RootTextExcludesAutoCommands`, `TestHelpDump_ExcludesAutoCommandsEverywhere`) that fails pre-fix and passes post-fix.
> *Consequence for tests*: tests MUST drive the dump through the real `rootCmd.Execute()` path (helper `dumpViaExecute`), not a bare `runHelpDump` call — a bare call never triggers cobra's lazy registration, so it would mask the incoherence the prune step exists to prevent.
> *Introduced by*: `260602-ep4z-help-dump-cli-tree`

### `text` byte-for-byte

`text` equals the command's `cmd.Help()` (help-template) output byte-for-byte — the enforceable form of "RAW `-h` output". `nodeText` reproduces cobra's default help func (cobra v1.10.2 `defaultHelpFunc`):

```
trimRightSpace(Long || Short)  +  "\n\n"  +  UsageString()
```

via `strings.TrimRightFunc(blurb, unicode.IsSpace)`. When both `Long` and `Short` are empty, only `UsageString()` is emitted (the blurb and its trailing blank line are omitted entirely) — matching cobra.

`buildNode` calls `cmd.InitDefaultHelpFlag()` and `cmd.InitDefaultVersionFlag()` on each node before rendering, because cobra adds the `-h`/`--help` (and root `-v`/`--version`) flags lazily at Execute time. Without this, `UsageString()` would omit those flags and the `[flags]` UseLine suffix — diverging from real `-h`. (`InitDefaultVersionFlag` is a no-op unless `cmd.Version != ""`.)

### `commands` is `[]`, never `null`

The children slice is initialized non-nil (`children := []helpNode{}`) before appending, so `encoding/json` emits `[]` for leaves rather than `null`. The reference `wt.json` uses `"commands": []` for leaves.

### Order preservation

Child order is whatever cobra's `Commands()` returns (its default alphabetical sort). The dump does not re-sort beyond that — matching `wt.json`, whose children are alphabetical.

### The optional `aliases` field

A node carries an `aliases` array of the command's registered alias names, populated by `buildNode` directly from cobra's `cmd.Aliases` (a `[]string`) — the same walk-never-parse discipline as every other field. Aliases are emitted in cobra's **declared order** (the order they appear in the command's `Aliases:` slice), never re-sorted, mirroring the child order-preservation rule above.

`aliases` uses `json:"aliases,omitempty"` and its Go struct field sits immediately after `Name` (Go struct field order pins JSON key order), so the key renders right after `name`, before `path`. Under `omitempty`, a nil or empty `cmd.Aliases` serializes to **no `aliases` key at all** — absence, never `[]` or `null`. This is the deliberate contrast with `commands`, a required v1 field that is always `[]` for a leaf: `aliases` is an *optional additive field*, so an unaliased node stays byte-identical to pre-`aliases` output and only aliased nodes change. That byte-stability is what lets the field ship with no consumer coordination.

Today shll has exactly one aliased command — `shell-setup` (alias `shell-install`, registered in `src/cmd/shll/shell_setup.go`) — so its node is the only one in `help/shll.json` that carries an `aliases` key (`["shell-install"]`); every other node is unchanged. The alias already appeared inside the node's raw `text` (cobra renders an `Aliases:` help section), but structured-field consumers could not see it without regex-parsing `text` — which the contract forbids. Emitting `aliases` makes the alias a first-class structured field.

> **Design Decision: optional field vs. duplicate nodes; no `schema_version` bump.**
> *Why a field, not extra nodes*: The alternative — emitting each alias as its own Node — would fabricate tree structure (synthesized `name`/`path` values cobra never registers as distinct commands) and break the text↔commands coherence rule: each node's `Available Commands:` block lists canonical names only, so a `commands` array padded with alias nodes would diverge from the rendered `text`, the exact incoherence prune-before-render exists to prevent.
> *Why no `schema_version` bump*: `schema_version` stays `1`. The [help-dump standard](/cli/standards-content.md)'s § Schema evolution is the authority — it reserves bumps for breaking shape changes and names **optional additive fields** as the non-breaking evolution path (each tool adopts on its own release cadence, no seven-repo flag-day, older captures keep validating). `aliases` is the first field added under that clause.
> *Introduced by*: `260718-whd7-help-dump-emit-aliases`

## Why a hidden subcommand (not a standalone tool)

`help-dump` is a `Hidden: true`, `NoArgs` cobra subcommand registered in `newRootCmd()` (`src/cmd/shll/root.go`), not a separate Go tool under `scripts/`. The subcommand has free access to the live `rootCmd` and to `rootCmd.Version` (already ldflags-stamped), so VERSION is read from the binary for free with no second source of truth, and it self-excludes from its own dump via the `Hidden` filter rule. `Hidden` keeps it off the user-facing help surface, so it does not raise the Constitution VII (Minimal Surface Area) bar — it is documented as build tooling, not a user command.

## Constitution conformance

- **I (Security First)** — N/A to the producer: it does a pure in-process tree walk with no subprocess execution (no `os/exec`, no `internal/proc`). Constitution I governs Go subprocess invocation; the CI git/gh shell-out lives in YAML, not Go.
- **II (No State)** — the dump is re-derived from the live command tree on every invocation; no caching.
- **VII (Minimal Surface Area)** — `Hidden` build tooling, not a user-facing addition to the `update`/`shell-init`/`version`/`install` surface.
- **Dependencies** — standard library only (`encoding/json`, `strings`, `unicode`, `io`) plus the existing `github.com/spf13/cobra`. No other go.mod deps.

## Test coverage

`src/cmd/shll/help_dump_test.go` (8 tests):

- Contract-shape — synthetic root + plain-visible/aliased-visible/hidden/`completion`/`help` children: top-level keys present, `schema_version == 1`, `tool == "shll"`, leaf `commands` is `[]` (not null), filtered children absent, **and `captured_at` is absent** (the envelope must not emit the shll.ai-owned field). The synthetic tree's `aliased` child carries two aliases in a fixed order (`["alias-one", "alias-two"]`), and the test asserts (a) that child's `aliases` equals the declared list **in order**, and (b) via a **raw-JSON key-presence** decode (a `json.RawMessage` per node — a typed `helpNode` cannot distinguish an absent key from a zero slice), that the root and the unaliased `visible` node emit **no `aliases` key** while `aliased` emits one.
- `text` byte-for-byte — every visible command in the real `newRootCmd()` compared against captured `cmd.Help()` output.
- Self-exclusion — `help-dump` absent from the real-tree dump.
- Version passthrough — `root.Version = "v9.9.9"` → `doc.version == "v9.9.9"`.
- Structural determinism — the envelope carries no time-varying field, so two successive dumps of the same tree are byte-identical (`aliases` adds no time-varying data; `nodeText` is untouched, so this and the byte-for-byte `text` test are unaffected).
- Execute-path regression — `TestHelpDump_RootTextExcludesAutoCommands` + `TestHelpDump_ExcludesAutoCommandsEverywhere`: drive via `dumpViaExecute` so cobra's lazy `completion`/`help` register exactly as on the shipped binary, then assert they appear in NEITHER `commands` NOR the rendered `text` `Available Commands:` block.
- Real-tree aliases — `TestHelpDump_EmitsAliasesRealTree`: drives the real `rootCmd.Execute()` path via `dumpViaExecute` (per prune-before-render) and asserts the `shell-setup` node carries exactly `["shell-install"]`, pinning the shipped binary's one aliased command.


## Cross-references

- Transport: `help-dump` writes to stdout; shll.ai's scheduled puller consumes it; the release workflow publishes nothing to shll.ai (7huv): [ci/release-workflow](/ci/release-workflow.md).
- Root command wiring, version ldflags injection: [cli/commands](/cli/commands.md).
- The reference sample `help/wt.json` lives in `sahil87/shll.ai`, not this repo — the byte-for-byte `text` test against real `-h` is the enforceable fidelity contract.
