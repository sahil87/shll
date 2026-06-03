# cli/help-dump-contract

The frozen `help/<tool>.json` contract and the rules for producing it. shll is one of 7 sahil87 tools that each expose a machine-readable export of their CLI surface for `sahil87/shll.ai`, which renders an expandable "Command reference" per tool page. shll.ai now **pulls** this export on a schedule (its change `oa63`): it `brew install`s each tool and runs the tool's `help-dump`, rather than receiving a push (the old push transport was torn down in change 7huv — see [ci/release-workflow](../ci/release-workflow.md)). **The contract is shared and frozen across all 7 tools** — the reference sample is shll.ai's `help/wt.json`; shll's producer mirrors its exact shape. Do not change the JSON shape without a coordinated 7-tool bump of `schema_version`.

Source: `src/cmd/shll/help_dump.go` (producer), `src/cmd/shll/help_dump_test.go` (conformance). `help-dump` emits the document to stdout; shll.ai's scheduled puller (`scheduled-help-refresh.yml`, on shll.ai's side) consumes it. This repo's release workflow no longer publishes to shll.ai — the push transport was torn down in change 7huv (see [ci/release-workflow](../ci/release-workflow.md)).

## The JSON contract (frozen — schema_version 1)

The document is a single JSON object:

```json
{
  "tool": "shll",
  "version": "v0.5.0",
  "schema_version": 1,
  "root": { "...Node..." }
}
```

> `captured_at` is **shll.ai-owned**: the puller stamps it on the captured document post-capture. The tool MUST NOT emit it — §3 of the contract and the pull-model teardown directive forbid it. It was dropped from `help-dump`'s envelope in change 7huv (along with the `capturedAt()` helper, the `capturedAtLayout` constant, and the `"time"` import); its old purpose — a date-granular value powering the CI no-op guard — died when the push CI was removed.

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
  "name": "install",
  "path": "shll install",
  "short": "brew install every sahil87 tool that isn't already installed",
  "usage": "shll install [flags]",
  "text": "<RAW -h output, byte-for-byte, newlines preserved>",
  "commands": []
}
```

Per-node field source (programmatic, from cobra's data model — **never regex on `-h`**):

| Field | Source |
|-------|--------|
| `name` | `cmd.Name()` |
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

> **Design Decision: prune the live tree, not just filter the array (change ep4z).**
> *Why*: An earlier implementation filtered only the `commands` array and built `text` from a tree that still held `completion`/`help`, producing an incoherent split (text lists them, array omits them) that also diverged from `wt.json`. The earlier assumption — that `text` comes from a walk that never sees `completion`/`help` — was WRONG for the real binary because `Execute()` registers them before `RunE`. Pruning the live tree first is the fix; verified end-to-end against the Execute-built binary and guarded by an Execute-path regression test (`TestHelpDump_RootTextExcludesAutoCommands`, `TestHelpDump_ExcludesAutoCommandsEverywhere`) that fails pre-fix and passes post-fix.
> *Consequence for tests*: tests MUST drive the dump through the real `rootCmd.Execute()` path (helper `dumpViaExecute`), not a bare `runHelpDump` call — a bare call never triggers cobra's lazy registration, so it would mask the incoherence the prune step exists to prevent.

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

## Why a hidden subcommand (not a standalone tool)

`help-dump` is a `Hidden: true`, `NoArgs` cobra subcommand registered in `newRootCmd()` (`src/cmd/shll/root.go`), not a separate Go tool under `scripts/`. The subcommand has free access to the live `rootCmd` and to `rootCmd.Version` (already ldflags-stamped), so VERSION is read from the binary for free with no second source of truth, and it self-excludes from its own dump via the `Hidden` filter rule. `Hidden` keeps it off the user-facing help surface, so it does not raise the Constitution VII (Minimal Surface Area) bar — it is documented as build tooling, not a user command.

## Constitution conformance

- **I (Security First)** — N/A to the producer: it does a pure in-process tree walk with no subprocess execution (no `os/exec`, no `internal/proc`). Constitution I governs Go subprocess invocation; the CI git/gh shell-out lives in YAML, not Go.
- **II (No State)** — the dump is re-derived from the live command tree on every invocation; no caching.
- **VII (Minimal Surface Area)** — `Hidden` build tooling, not a user-facing addition to the `update`/`shell-init`/`version`/`install` surface.
- **Dependencies** — standard library only (`encoding/json`, `strings`, `unicode`, `io`) plus the existing `github.com/spf13/cobra`. No new go.mod deps. (The `"time"` import was dropped in change 7huv along with `captured_at`.)

## Test coverage

`src/cmd/shll/help_dump_test.go` (7 tests):

- Contract-shape — synthetic root + visible/hidden/`completion`/`help` children: top-level keys present, `schema_version == 1`, `tool == "shll"`, leaf `commands` is `[]` (not null), filtered children absent, **and `captured_at` is absent** (the envelope must not emit the shll.ai-owned field).
- `text` byte-for-byte — every visible command in the real `newRootCmd()` compared against captured `cmd.Help()` output.
- Self-exclusion — `help-dump` absent from the real-tree dump.
- Version passthrough — `root.Version = "v9.9.9"` → `doc.version == "v9.9.9"`.
- Structural determinism — the envelope carries no time-varying field, so two successive dumps of the same tree are byte-identical.
- Execute-path regression — `TestHelpDump_RootTextExcludesAutoCommands` + `TestHelpDump_ExcludesAutoCommandsEverywhere`: drive via `dumpViaExecute` so cobra's lazy `completion`/`help` register exactly as on the shipped binary, then assert they appear in NEITHER `commands` NOR the rendered `text` `Available Commands:` block.

(Change 7huv removed `TestHelpDump_CapturedAtShape` and its `capturedAtRE`/`regexp` dependency, dropping the count from 8 to 7, and added the `captured_at`-absence assertion to the contract-shape test.)

## Cross-references

- Transport: `help-dump` writes to stdout; shll.ai's scheduled puller consumes it. The release workflow no longer publishes to shll.ai (push transport torn down in change 7huv): [ci/release-workflow](../ci/release-workflow.md).
- Root command wiring, version ldflags injection: [cli/commands](commands.md).
- The reference sample `help/wt.json` lives in `sahil87/shll.ai`, not this repo — the byte-for-byte `text` test against real `-h` is the enforceable fidelity contract.
