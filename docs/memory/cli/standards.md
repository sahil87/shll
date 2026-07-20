---
type: memory
description: "`shll standards` — agent-facing reader for the toolkit's binding standards: self-describing list (name · scope · description) + `--json` ({name, description, scope, source_path}), byte-identical `<name>` reader, unknown-name errSilent error; build-time embed (committed `src/cmd/shll/standards/*.md` + `scripts/sync-standards.sh` from `docs/site/standards/`) with the roster-driven `TestStandardsEmbedMatchesCanonical` drift guard over all eight standards; docs/site/standards/ is canonical."
---
# cli/standards

`shll standards` — the agent-facing reader for the shll toolkit's binding, producer-facing standards. Bare form lists every available standard with its **scope** and a one-line "what it governs and when it applies" description (a self-describing glossary); `shll standards <name>` prints the full markdown document to stdout, byte-identical to its canonical `docs/site/standards/` source. Content is embedded into the binary at build time, so output is offline and versioned with the release.

> The standards *documents* themselves (the `docs/site/standards/` restructure rationale and the `skill` standard's contract) are documented in [cli/standards-content](/cli/standards-content.md). This file is the **command** — the reader surface, roster, embed mechanism, and drift guard.

Source: `src/cmd/shll/standards.go` (+ `standards_test.go`) and the embedded copies under `src/cmd/shll/standards/*.md`. Reuses `errSilent` from `src/cmd/shll/main.go` for the unknown-name exit path; the `text/tabwriter` + `json.Encoder(SetEscapeHTML(false))` idioms mirror `shll list`. No subprocess, no network — the command reads embedded bytes only.

## Why it exists

The toolkit gained binding standards (the ten CLI principles, the help-dump contract, the README/docs-site structure standard) but had no way for an AI agent working in *any* toolkit repo to read them without network access and prior knowledge of where they live. Agent entry files across the toolkit repos say "run `shll standards`" **without explaining the standard names** — so the command itself is the glossary: the bare list form is deliberately self-describing (name + what it governs + when it applies), letting an agent resolve a standard's name with zero prior knowledge.

Rejected alternatives (decided in the originating conversation): an **MCP server** (static content, adds no capability over printing markdown, costs per-environment config in every repo); **website-URL-only** (requires network plus an agent that already knows to fetch a specific URL). `shll` is already on every dev machine, and the embedded content is offline and versioned with the release.

## The two forms

`shll standards` has exactly two forms, selected by the positional-arg count (`cobra.MaximumNArgs(1)`): bare (list) and single-name (read).

### Bare `shll standards` — the self-describing glossary list

One row per standard, in roster order, `name` + **`scope`** + one-line description (three columns), printed to **stdout** (it is data). Human output uses `text/tabwriter` with the **same config as `shll list` / `shll version`** (minwidth 0, tabwidth 0, padding 2, padchar space, no flags). Deliberately **no status column and no color glyphs** — the standards list is a static glossary, not an install-status view — so the table output is escape-free on every writer (unlike `shll list`, which carries a color-gated status cell). The `scope` column sits **between** name and description. Exit 0.

```
principles         foundation     The ten toolkit CLI principles every tool is built against
help-dump          binary         Machine-readable help contract every tool must emit
readme-extraction  repo           README + docs/site structure standard for toolkit repos
skill              binary+repo    Agent skill bundle standard: docs/site/skill.md served by `<tool> skill`
```

### `shll standards --json` — the machine-readable roster array

`--json` (bool flag on the bare form, `standardsJSONFlag = "json"`) emits a **bare JSON array** — one `{name, description, scope, source_path}` object per standard in roster order (see [The scope field](#the-scope-field)). `source_path` is the repo-relative canonical path (e.g. `docs/site/standards/principles.md`), so a consumer can locate the doc in the shll repo without re-deriving it. Emitted via a `json.Encoder` configured with `SetEscapeHTML(false)`, `SetIndent("", "  ")` (2-space), and `enc.Encode` (single trailing newline) — the exact idiom `shll list --json` uses, and the same rationale: `SetEscapeHTML(false)` keeps `&`/`<`/`>` as literal characters (the default encoder would emit their `\uXXXX` forms) so the raw output stays legible and matches the table. Plain JSON only — no ANSI, no table framing, regardless of TTY.

The `standardJSONItem` struct field order is `{name, description, scope, source_path}` (`scope` declared before `source_path`):

```json
[
  {
    "name": "principles",
    "description": "The ten toolkit CLI principles every tool is built against",
    "scope": "foundation",
    "source_path": "docs/site/standards/principles.md"
  }
]
```

The `scope` field is **additive** — new optional fields are explicitly allowed by the toolkit's format-stability rule (principle №2), so a consumer keyed on the older `{name, description, source_path}` shape does not break on it. `TestStandards_ListJSONFieldNames` asserts `"scope"` is present alongside the pre-existing field names.

`--json` is a **list-form flag**: when a name argument is present the reader path runs and emits raw markdown regardless of the flag (`runStandards` dispatches on arg count first). `TestStandards_ReadDoc_JSONFlagIgnoredForReader` pins this.

### `shll standards <name>` — byte-identical document reader

Prints the full markdown document for that standard to **stdout**, byte-identical to the canonical `docs/site/standards/<name>.md` source — raw markdown, **no rendering, no pager, no added framing**. Agents consume it directly. The bytes come from `standardsFS.ReadFile(standardsEmbedDir + "/" + s.EmbedName)` — the embedded copy — so output is offline and versioned with the release. Exit 0.

### Unknown name → actionable stderr error, `errSilent`, exit 1

`shll standards <name>` with an unknown name writes an actionable diagnostic to **stderr** that names the offending input **and every valid standard name** (`unknown standard %q (valid: %s)`, valid list derived live from the roster via `validStandards()`), writes **nothing to stdout**, and returns the `errSilent` sentinel so `main.go`'s `translateExit` exits 1 without double-printing (principle №4 — fail fast with actionable errors). This is the same `errSilent` idiom the other subcommands use (see [cli/commands §exit-code translation](/cli/commands.md#exit-code-translation)); no new exit-code sentinel was introduced by this change.

## Output conventions follow the standard being served

The stdout/stderr split is not incidental — it obeys the very standard the command serves: content (list, JSON, document) on **stdout** because it is data (principle №2); diagnostics on **stderr**; fail-fast actionable errors (principle №4). `TestStandards_ListTable` / `TestStandards_ListJSON` assert the list forms write nothing to stderr; `TestStandards_UnknownName` asserts stdout is empty on the error path.

## The standards roster (hardcoded source of truth)

`standardsRoster` (`src/cmd/shll/standards.go`) is a hardcoded `[]standard` slice — the source of truth for names, descriptions, canonical source paths, and embed filenames, mirroring `tools.go`'s `Roster` pattern. **Descriptions are hardcoded here, NOT parsed from the markdown at runtime** (the constitution's "explicit versioned lists are the contract" — parsing markdown headers is fragile and couples the CLI surface to prose). Roster order is the output order for both the table and `--json`.

The `standard` struct carries **five** fields:

- **`Name`** — the standard's identifier (the `<name>` argument and the JSON `name`).
- **`Description`** — the one-line "what it governs / when it applies" glossary line printed by the bare list form and emitted as JSON `description`.
- **`Scope`** — where the standard's obligations live (i70w): `foundation` (the principles every tool is built against), `binary` (satisfied by the compiled tool at runtime), `repo` (satisfied by the repo's file structure), or `binary+repo` (spans both). Printed as the **middle** column of the bare list form and emitted as JSON `scope`. See [The scope field](#the-scope-field).
- **`SourcePath`** — the repo-relative canonical path (`docs/site/standards/<name>.md`), emitted as JSON `source_path` and used as the drift-guard comparison target. Single-sourced so JSON and the drift guard agree.
- **`EmbedName`** — the base filename of the embedded copy under `standardsEmbedDir` (`principles.md`); the full embed path is `standardsEmbedDir + "/" + EmbedName`. **Embedded copies stay flat** (a bare basename) under `src/cmd/shll/standards/`, NOT mirrored into a `standards/` subdir — `SourcePath`'s basename equals `EmbedName` and the `//go:embed standards/*.md` glob is unaffected by the canonical sources living in `docs/site/standards/` (i70w Design Decision — the drift guard is the contract, so the flat layout is the minimal diff).

The roster is **eight** entries, in this order (roster order == output order for both the table and `--json`):

| Name | Scope | SourcePath (canonical) | Description |
|------|-------|------------------------|-------------|
| `principles` | `foundation` | `docs/site/standards/principles.md` | The ten toolkit CLI principles every tool is built against |
| `help-dump` | `binary` | `docs/site/standards/help-dump.md` | Machine-readable help contract every tool must emit |
| `readme-extraction` | `repo` | `docs/site/standards/readme-extraction.md` | README + docs/site structure standard for toolkit repos |
| `skill` | `binary+repo` | `docs/site/standards/skill.md` | Agent skill bundle standard: docs/site/skill.md served by `<tool> skill` |
| `update` | `binary` | `docs/site/standards/update.md` | In-place `update` upgrade contract: `--skip-brew-update` probe, exit codes, brew-handling safety |
| `version` | `binary` | `docs/site/standards/version.md` | `--version` shape shll probes: 2s budget, first-line token, binary-name install probe |
| `shell-init` | `binary` | `docs/site/standards/shell-init.md` | Eval-safe `shell-init` output every shell-integration tool emits on stdout |
| `install-composition` | `binary+repo` | `docs/site/standards/install-composition.md` | No sibling `depends_on` between toolkit formulas; probe siblings at runtime; install docs centralized on shll.ai |

The three producer-surface standards (`update`, `version`, `shell-init`) are appended after `skill` at `Scope: "binary"` — their obligations are satisfied by the compiled tool at runtime (the `update` / `--version` / `shell-init` subcommand behavior), the same scope as `help-dump`. `install-composition` is appended last at `Scope: "binary+repo"` (the formula-edge half is a repo-file obligation, the runtime-probe half a binary one — the same span as `skill`). Content is [cli/standards-content](/cli/standards-content.md); shll's own posture against them is [cli/standards-conformance](/cli/standards-conformance.md).

Adding a standard is a roster row **plus** its canonical `docs/site/standards/` file (synced in by `scripts/sync-standards.sh`) — an explicit, versioned list, exactly like the tool roster and adding a tool. Roster integrity is guarded by `TestStandardsRosterIntegrity` (non-empty `Name`/`Description`/`EmbedName`, no duplicate names, `SourcePath` under **`docs/site/standards/`** — and `SourcePath`'s basename equals `EmbedName` so JSON's `source_path` and the embedded copy refer to the same document).

### The scope field

`Scope` names where a standard's obligations are satisfied, and doubles as the taxonomy that keeps the standards *filenames* plain (the intake rejected `principle-*`/`repo-*`/`contract-*` filename prefixes — taxonomy is expressed via location, the `docs/site/standards/` dir, plus this list metadata, not the filename). The vocabulary is single-sourced from the principles "The contracts" table (see [cli/standards-content](/cli/standards-content.md)):

- **`foundation`** — the principles document itself, the base every tool is built against.
- **`binary`** — an obligation the compiled tool satisfies at runtime (help-dump: the tool must emit the machine-readable help JSON).
- **`repo`** — an obligation the repo's file structure satisfies (readme-extraction: README + docs/site layout).
- **`binary+repo`** — spans both (skill: the `skill` subcommand ships in the binary; the canonical bundle lives in the repo).

The column reuses the existing tabwriter writer and roster loop — no parallel renderer — and the JSON `scope` reuses the existing encoder path (change i70w Acceptance A-016). The three-column list output (name · scope · description) and the `{name, description, scope, source_path}` JSON are the CLI-surface shape; the two forms above show both.

Two named constants keep the embed path and flag free of magic strings (code-quality.md): `standardsEmbedDir = "standards"` (matches the `//go:embed standards/*.md` pattern) and `standardsJSONFlag`/`standardsJSONFlagUsage`.

## The build-time embed mechanism

> `docs/site/standards/` is the single canonical source for the eight standards documents, pulled and rendered by shll.ai (each page lands at `shll.ai/shll/standards/<name>`, mirroring `shll standards <name>`) — see [cli/standards-content](/cli/standards-content.md) for the directory rationale and the individual standards' contracts. (vo8c, i70w)

**The known constraint** (why a plain embed is impossible): the Go module root is `src/` (`src/go.mod`), and `docs/site/` sits **above** it — `//go:embed` cannot reach above the module root, so `//go:embed ../../../docs/site/*.md` is not allowed by the toolchain. The mechanism bridges the gap:

1. **A copy step** — `scripts/sync-standards.sh` copies the canonical sources into `src/cmd/shll/standards/` (a subdir beside `standards.go`, keeping embed assets colocated with the consuming command and the `*.md` glob scoped). It sets `SRC_DIR="docs/site/standards"` and its `STANDARDS=(principles help-dump readme-extraction skill update version shell-init install-composition)` array names **eight** files; the `DEST_DIR="src/cmd/shll/standards"` and the embedded filenames stay flat (see `EmbedName` above). A second block in the same script copies shll's own `docs/site/skill.md` bundle into `src/cmd/shll/skill/skill.md` (a separate concern from the standards array — see [cli/skill](/cli/skill.md)). `set -euo pipefail`, runs from the repo root regardless of caller CWD, one-liner-delegating per Constitution VI (thin justfile).
2. **The copies are committed** — the embedded `src/cmd/shll/standards/*.md` files are in the tree, so a clean `go build ./...` (which does **not** run the sync script — nor does CI, which builds directly) compiles.
3. **Embedded from there** — `//go:embed standards/*.md` into a package-level `var standardsFS embed.FS` in `standards.go` (this repo's **first `//go:embed` usage**).
4. **Wired into the build path three ways** (all single-sourced on the sync script):
   - `scripts/build.sh` runs `./scripts/sync-standards.sh` before `go build`, so a local `just build` re-syncs any drift.
   - A `//go:generate ../../../scripts/sync-standards.sh` directive in `standards.go` so `go generate ./...` refreshes the copies.
   - A `just sync-standards` recipe (one-line delegation to the script) for the explicit refresh.

### The drift guard (`TestStandardsEmbedMatchesCanonical`)

Because the committed copies could silently diverge from the canonical `docs/site/standards/` sources, `TestStandardsEmbedMatchesCanonical` (`src/cmd/shll/standards_test.go`) asserts, for **each** roster entry, that the embedded bytes **byte-match** the canonical source. It is **roster-driven** — it iterates `standardsRoster`, so it covers all eight entries automatically and picks up a newly appended standard with no test edit. The test file lives at `src/cmd/shll/`, so it resolves the canonical path as `../../../<SourcePath>` (`filepath.Join("..","..","..", s.SourcePath)`). On drift it fails naming the file, pointing at `just sync-standards` (or `scripts/sync-standards.sh`) to refresh and commit.

**Why a Go test, not a separate CI step** (Design Decision, resolving intake assumption #7): it runs on every `go test ./...` — locally **and** in the existing CI PR workflow (build, vet, test) — with **no `.github/workflows/` edit needed**. Rejected: a symlink into `src/` (embed does not follow symlinks reliably across the toolchain and breaks the byte-copy guarantee); parsing markdown at runtime (fragile, violates the hardcoded-contract principle); a separate CI-only diff step (a Go test is simpler and runs everywhere `go test` runs).

### Single-source rule and accepted staleness

`docs/site/` is **canonical**; the embedded copies are a build-time snapshot. **Staleness-until-next-release is accepted**: when a canonical doc changes, the drift guard fails until `sync-standards` is re-run and committed, and the change ships to users with the next shll **release**. This is analogous to the hardcoded tool roster — explicit, versioned lists are the contract (Constitution III in spirit; embedded bytes are a build snapshot, not runtime state, so Constitution II — No State — also holds).

## Behavior contract

`runStandards(stdout, stderr io.Writer, args []string, jsonOut bool)` (`src/cmd/shll/standards.go`) is the implementation seam extracted from the cobra factory `newStandardsCmd()` — the established `runXxx(writers…)` pattern, so `standards_test.go` drives it directly with `bytes.Buffer`s. Unlike most subcommands it needs **no fake `proc.Runner`** — there is no subprocess.

1. `len(args) == 0` → list form: `writeStandardsJSON(stdout)` when `jsonOut`, else `writeStandardsTable(stdout)`.
2. `len(args) == 1` → `writeStandardDoc(stdout, stderr, args[0])`: look up via `standardByName`; unknown → stderr diagnostic + `errSilent`; known → write the embedded bytes to stdout.
3. `> 1` positional arg is rejected by cobra `Args: cobra.MaximumNArgs(1)` before `RunE` runs.

Helpers: `standardByName(name) (standard, bool)` and `validStandards() string` both derive from the live `standardsRoster`, so the valid-name list in the error never drifts from the roster (`validStandards` uses `strings.Join`, matching `tools.go`'s `validTargets` diagnostic idiom).

## Wiring

`newStandardsCmd()` is registered in `newRootCmd()` (`src/cmd/shll/root.go`) alongside the other subcommands, and a one-line `shll standards [name]` entry was added to the `rootLong` help text. It is one of the twelve user-facing subcommands (see [cli/commands §Constitution VII](/cli/commands.md#constitution-vii-justification-per-subcommand)). No subprocess is involved, so `internal/proc` is untouched (Constitution I applies vacuously — no `os/exec`, no shell surface). `help-dump` picks up the new visible subcommand **mechanically** via its programmatic cobra walk — **no `help_dump.go` change** (see [cli/help-dump-contract](/cli/help-dump-contract.md)).

## Constitution VII justification

> *Why a new top-level subcommand?* Serving toolkit-wide standards is a **cross-toolkit concern** — exactly the meta-tool's job.
>
> It **cannot be a flag on an existing subcommand**: no existing subcommand reads or prints documents (`list` is the tool roster, `help-dump` is the machine help contract, `version` is a frozen bug-report table). And it **belongs to no per-tool CLI** — the standards govern all seven tools, so no single tool can own them (Constitution IV). `shll` is the natural home.
>
> *Rejected*: an MCP server (static content, adds no capability over printing markdown, per-environment config cost); website-URL-only (requires network + an agent that knows to fetch). See [Why it exists](#why-it-exists).

## Out of scope

- **`shll audit`** — the future conformance checker that pairs with `standards` (what it checks, where it runs) needs its own design. Not part of this change.
- **Deploying "run `shll standards`" stanzas** to other toolkit repos' agent entry files — separate, per-repo work.
- Any change to shll.ai's pull/render pipeline — `docs/site/` stays canonical and structurally untouched.
- Rendering markdown, paging, or syntax-highlighting the document output — raw bytes only.

## Test seam

`standards_test.go` drives `runStandards` with `bytes.Buffer`s (no fake proc runner). Scenarios (`src/cmd/shll/standards_test.go`):

- `TestStandards_ListTable` — bare table has one row per roster entry carrying name, **scope**, and description, roster order, no ANSI escapes, nothing on stderr.
- `TestStandards_ListJSON` — `--json` is valid JSON, `len == len(standardsRoster)`, index-paired to the live roster (a future reorder moves expected and actual in lockstep), every field incl. `scope` and `source_path` correct, trailing newline, no ANSI, nothing on stderr.
- `TestStandards_ListJSONFieldNames` — the raw `"name"`/`"description"`/`"scope"`/`"source_path"` field names are present (the stable JSON contract mirroring `shll list --json`).
- `TestStandards_ReadDoc_ByteIdentical` — for every roster standard, `shll standards <name>` stdout equals the embedded bytes byte-for-byte, nothing on stderr.
- `TestStandards_ReadDoc_JSONFlagIgnoredForReader` — a name argument runs the reader path emitting raw markdown even with `--json` set.
- `TestStandards_UnknownName` — unknown name → `errSilent`, empty stdout, stderr names the offending input and every valid standard (all eight).
- `TestStandardsEmbedMatchesCanonical` — the drift guard: each embedded standard's bytes equal `../../../<SourcePath>` (the canonical `docs/site/standards/` source), failing (naming the file) on drift. Roster-driven, so it covers all eight entries.
- `TestStandardsRosterIntegrity` — non-empty fields, no duplicate names, `SourcePath` under **`docs/site/standards/`**, `SourcePath` basename == `EmbedName`.

## Cross-references

- shll's own audited conformance to the standards this command serves (the constitution-mandated receipt, audited on HEAD against v0.0.23): [cli/standards-conformance](/cli/standards-conformance.md) — the manual precursor to the future `shll audit` noted in [Out of scope](#out-of-scope).
- Subcommand wiring, the `errSilent` exit-code sentinel, and the hardcoded-roster pattern this mirrors: [cli/commands](/cli/commands.md#subcommand-factory-pattern) and [§hardcoded tool roster](/cli/commands.md#hardcoded-tool-roster).
- The sibling read-only listing command whose `tabwriter` + `json.Encoder(SetEscapeHTML(false))` idioms this reuses: [cli/list](/cli/list.md#output-shapes).
- `help-dump` picks up the new subcommand mechanically (no producer change): [cli/help-dump-contract](/cli/help-dump-contract.md).
- Constitution I (Security First) — no subprocess, no `os/exec`, `internal/proc` untouched (applies vacuously). Constitution II (No State) — embedded bytes are a build snapshot, not runtime state. Constitution III (explicit versioned lists are the contract) — the hardcoded roster + accepted staleness-until-next-release. Constitution VI (thin justfile) — the copy logic lives in `scripts/sync-standards.sh`. Constitution VII (Minimal Surface Area) — justified above.
