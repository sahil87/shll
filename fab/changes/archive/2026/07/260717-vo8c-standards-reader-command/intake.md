# Intake: shll standards — agent-facing reader for the toolkit's binding standards

**Change**: 260717-vo8c-standards-reader-command
**Created**: 2026-07-17

## Origin

Conversational — designed in a live discussion session on branch `torrid-fisher`, immediately after the three standards documents landed in this repo (commit f3cd931, PR sahil87/shll#40: `docs/site/principles.md`, `docs/site/help-dump.md`, `docs/site/readme-extraction.md`). Dispatched promptless via `/fab-proceed`; the conversation resolved naming, scope, sources, the single-source constraint, and the Constitution VII justification before dispatch.

> Feature: a new `shll standards` subcommand — the agent-facing reader for the toolkit's binding standards. Bare form lists every available standard with a one-line description; `shll standards <name>` prints the full markdown document to stdout. Content is the canonical `docs/site/*.md` pages, embedded into the binary at build time so output is offline and versioned with the release.

## Why

1. **The pain point**: the toolkit now has binding, producer-facing standards (the ten CLI principles, the help-dump contract, the README/docs-site structure standard), but no way for an AI agent working in any toolkit repo to read them without network access and prior knowledge of where they live. Agent entry files across the toolkit repos will say "run `shll standards`" **without explaining the standard names** — so the command itself must be the glossary: the bare list form is deliberately self-describing (name + what it governs + when it applies), letting an agent resolve a standard's name with zero prior knowledge.
2. **If we don't build it**: standards stay reachable only via the website (requires network plus an agent that already knows to fetch a specific URL) or via reading this repo's `docs/site/` tree (requires having the shll repo checked out — agents in `hop`/`wt`/etc. don't). Standards that agents can't reliably read don't get followed.
3. **Why this approach over alternatives** (both rejected in conversation):
   - **MCP server** — rejected: the content is static, an MCP server adds no capability over a subcommand printing markdown, and it costs per-environment configuration in every repo/agent setup. `shll` is already on every dev machine.
   - **Website-URL-only** (point agents at shll.ai) — rejected: requires network and an agent that knows to fetch; the command is offline, versioned with the release, and needs zero configuration.

**Constitution VII justification** (required for any new top-level subcommand): serving toolkit-wide standards is a cross-toolkit concern — exactly the meta-tool's job. It cannot be a flag on an existing subcommand (no existing subcommand reads or prints documents — `list` is the tool roster, `help-dump` is the machine help contract), and it belongs to no per-tool CLI (the standards govern all seven tools). This justification line must be carried into the plan per `fab/project/code-review.md` ("New top-level subcommands need a Constitution VII justification line in the spec").

## What Changes

### 1. New subcommand `shll standards` (bare form — the self-describing list)

Lists every available standard, one row per standard, with a one-line description of **what it governs and when it applies**. This list is the glossary contract: an agent that has only ever been told "run `shll standards`" must be able to pick the right document from this output alone.

- Output on **stdout** (it is data), diagnostics on **stderr**, exit 0 — per principle №2 of the very standard being served.
- Follow the `shll list` presentation pattern (aligned `text/tabwriter` table for humans; see `src/cmd/shll/list.go`).
- `--json` flag on the bare form: machine-readable array of `{name, description, source_path}` objects (source_path = the repo-relative canonical path, e.g. `docs/site/principles.md`), mirroring `shll list --json`'s stable-field-contract style.
- Descriptions are hardcoded alongside the standard roster in Go source (a named slice of structs, analogous to the tool `Roster` in `tools.go`) — the constitution's "explicit versioned lists are the contract" applies; do not parse descriptions out of the markdown at runtime.

### 2. New subcommand form `shll standards <name>` (document reader)

Prints the **full markdown document** for that standard to stdout, byte-identical to the canonical `docs/site/` source. Raw markdown, no rendering, no pager — agents consume it directly.

- Unknown name → actionable error on stderr **naming the valid names**, non-zero exit (principle №4: fail fast with actionable errors).

### 3. Initial standards roster (three entries)

| Name | Canonical source | One-line scope (for the list description) |
|------|------------------|--------------------------------------------|
| `principles` | `docs/site/principles.md` | The ten toolkit CLI principles every tool is built against |
| `help-dump` | `docs/site/help-dump.md` | Machine-readable help contract every tool must emit |
| `readme-extraction` | `docs/site/readme-extraction.md` | README + docs/site structure standard for toolkit repos |

(Exact description wording to be finalized at apply from each document's own opening paragraph — but stored hardcoded, not parsed.)

### 4. Single-source embedding with drift guard

The `docs/site/*.md` pages are **canonical** — they are pulled and rendered by shll.ai. The command must serve exactly that content, embedded into the binary at build time so output is offline and versioned with the release.

**Known constraint**: the Go module root is `src/` (`src/go.mod`), and `docs/site/` sits outside it — `//go:embed` cannot reach above the module root, so a plain embed of `docs/site/` is impossible. The implementation needs:

- a **copy step** bringing the three files inside `src/` for embedding (e.g. `go:generate` copy, or a `scripts/` copy invoked from the build path per Constitution VI's thin-justfile pattern — the exact mechanism is an apply-time design decision, deliberately left open by the conversation), and
- a **drift guard**: a test or CI check asserting the embedded copies byte-match `docs/site/` (a `standards_test.go` test comparing embedded bytes against `../../../docs/site/*.md` runs on every `go test` and in the existing CI workflow, making it the natural default — apply decides and records).

Staleness semantics are accepted: when `docs/site/principles.md` changes, the next shll **release** picks it up. This is analogous to the hardcoded tool roster — explicit versioned lists are the contract.

### 5. Repo-pattern conformance

- One file per subcommand: `src/cmd/shll/standards.go`, test alongside: `src/cmd/shll/standards_test.go` (config `test_paths: **/*_test.go`).
- Cobra command constructor `newStandardsCmd()`, registered in `newRootCmd()` (`src/cmd/shll/root.go`) alongside the existing ten subcommands; add the one-line entry to the `rootLong` subcommand listing.
- **No subprocess involved** — this command reads embedded bytes only, so `internal/proc` is not touched (Constitution I applies vacuously; no `os/exec` anywhere in this change).
- `help-dump` output picks up the new visible subcommand mechanically (programmatic cobra walk) — no changes needed to `help_dump.go`.
- Named constants for flag names/usage strings per `code-quality.md` (no magic strings).

### Out of scope (explicit, decided in conversation)

- **`shll audit`** — the future conformance checker that pairs with `standards`. Needs its own design (what it checks, where it runs). Not part of this change.
- **Deploying "run `shll standards`" stanzas** to the other toolkit repos' agent entry files. Separate, per-repo work.
- Any changes to shll.ai's pull/render pipeline — `docs/site/` remains canonical and untouched in structure.

## Affected Memory

- `cli/standards`: (new) `shll standards` — standards roster, list/read forms, embed + drift-guard mechanism, single-source rule
- `cli/commands`: (modify) subcommand wiring roster gains `newStandardsCmd()`; note any new sentinel/exit-code usage
- `cli/help-dump-contract`: unaffected mechanically (a new visible subcommand simply appears in the help-dump walk output) — no entry needed unless apply discovers otherwise

## Impact

- **Code**: new `src/cmd/shll/standards.go` + `standards_test.go`; one-line registration + `rootLong` addition in `src/cmd/shll/root.go`; a new embedded-assets location inside `src/` (first `go:embed` usage in this repo) plus the copy mechanism (possibly a `scripts/` script and/or `go:generate` directive, possibly a justfile recipe line).
- **Build**: the copy step must run before `go build` sees stale/missing embedded files — wiring into `scripts/build.sh` / CI is in scope for the mechanism decision. The embedded copies are committed (so `go build` from a clean checkout works); the drift guard keeps them honest.
- **CI**: existing PR workflow (build, vet, test) automatically runs the drift-guard test if it is a Go test; if a separate CI check is chosen instead, `.github/workflows/` gains a step.
- **Docs**: README command table / `docs/site/workflows.md` may warrant a mention (apply's judgment; low risk).
- **No dependencies added**; no network, no subprocess, no state (Constitution II holds — embedded bytes are not runtime state).

## Open Questions

- None blocking. The one deliberately-open design point (exact copy-step mechanism + drift-guard form) was explicitly delegated to this change by the conversation and is recorded as a Confident assumption below for apply to decide-and-record.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Command named `standards` (not `principles`) | Discussed — decided: it's a family of documents and pairs with a future `shll audit` | S:95 R:75 A:95 D:95 |
| 2 | Certain | Three initial standards `principles`/`help-dump`/`readme-extraction` mapping to `docs/site/{principles,help-dump,readme-extraction}.md` | Discussed — sources named explicitly (commit f3cd931, PR #40) | S:95 R:80 A:95 D:95 |
| 3 | Certain | Content embedded at build time from canonical `docs/site/*.md`, byte-identical output, offline; staleness-until-next-release accepted | Discussed — single-source requirement stated verbatim; roster-analogy accepted | S:95 R:70 A:90 D:90 |
| 4 | Certain | Unknown standard name → actionable stderr error naming valid names, non-zero exit; document/list content on stdout | Discussed — output conventions follow the toolkit principles the command serves | S:90 R:90 A:95 D:95 |
| 5 | Certain | `shll audit` and cross-repo "run shll standards" stanza deployment are OUT OF SCOPE | Discussed — explicitly excluded | S:95 R:90 A:95 D:95 |
| 6 | Certain | `<name>` form prints raw markdown to stdout — no rendering, no pager | Discussed — "prints the full markdown document to stdout"; agent-facing consumption | S:85 R:85 A:90 D:90 |
| 7 | Confident | Exact embed copy-step mechanism (go:generate vs. scripts/ copy) and drift-guard form (Go test vs. CI step) decided at apply | Conversation explicitly left it open for this change; reversible internal build detail; repo gives strong signals (Constitution VI scripts pattern, existing CI test job) | S:55 R:85 A:70 D:50 |
| 8 | Confident | Include `--json` on the bare list form emitting `{name, description, source_path}` | Conversation said "consider --json"; strong precedent in `shll list --json` and principle №3's machine-readability posture | S:60 R:90 A:85 D:75 |
| 9 | Confident | List descriptions hardcoded in a Go roster slice (like `tools.go` Roster), not parsed from markdown at runtime | Constitution: explicit versioned lists are the contract; parsing markdown headers is fragile | S:55 R:80 A:80 D:70 |
| 10 | Confident | Bare-list human output follows `shll list`'s aligned-tabwriter table pattern | Direct in-repo precedent for a roster listing; conversation mandated following existing repo patterns | S:60 R:90 A:85 D:80 |

10 assumptions (6 certain, 4 confident, 0 tentative, 0 unresolved).
