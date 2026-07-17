# Plan: shll standards — agent-facing reader for the toolkit's binding standards

**Change**: 260717-vo8c-standards-reader-command
**Intake**: `intake.md`

## Requirements

> **Constitution VII justification** (required for any new top-level subcommand,
> per `fab/project/code-review.md`): serving toolkit-wide standards is a
> cross-toolkit concern — exactly the meta-tool's job. It cannot be a flag on an
> existing subcommand (no existing subcommand reads or prints documents — `list`
> is the tool roster, `help-dump` is the machine help contract), and it belongs
> to no per-tool CLI (the standards govern all seven tools). This is the bar
> Constitution VII sets for a new top-level subcommand, and this change clears it.

### CLI: `shll standards` (list form)

#### R1: Bare `shll standards` lists every standard with a one-line scope description
`shll standards` with no arguments SHALL print, to stdout, one row per available
standard: the standard's `name`, its one-line description of *what it governs and
when it applies*, so an agent told only "run `shll standards`" can pick the right
document from this output alone. Human output SHALL follow `shll list`'s
aligned-`text/tabwriter` presentation. The command SHALL exit 0.

- **GIVEN** a built `shll` binary
- **WHEN** the user runs `shll standards`
- **THEN** stdout contains one aligned row per standard (name + description)
- **AND** the process exits 0
- **AND** diagnostics (if any) go to stderr, never stdout (principle №2)

#### R2: `shll standards --json` emits a machine-readable roster array
`shll standards --json` SHALL emit, to stdout, a plain JSON array of
`{name, description, source_path}` objects (one per standard, roster order),
where `source_path` is the repo-relative canonical path (e.g.
`docs/site/principles.md`). Output SHALL be free of ANSI escapes and table
framing regardless of TTY, mirroring `shll list --json`.

- **GIVEN** a built `shll` binary
- **WHEN** the user runs `shll standards --json`
- **THEN** stdout is a valid JSON array of `{name, description, source_path}` objects
- **AND** the array carries one object per standard in roster order
- **AND** there are no ANSI escapes and a single trailing newline

### CLI: `shll standards <name>` (document reader)

#### R3: `shll standards <name>` prints the full canonical markdown to stdout
`shll standards <name>` SHALL print, to stdout, the full markdown document for
that standard, **byte-identical** to the canonical `docs/site/<name>.md` source —
raw markdown, no rendering, no pager. Output SHALL come from bytes embedded into
the binary at build time (offline, versioned with the release).

- **GIVEN** a built `shll` binary and a valid standard name (e.g. `principles`)
- **WHEN** the user runs `shll standards principles`
- **THEN** stdout is the exact bytes of the embedded `principles.md`
- **AND** the process exits 0

#### R4: Unknown standard name → actionable stderr error naming valid names, non-zero exit
`shll standards <name>` with an unknown name SHALL write an actionable error to
**stderr** that names the valid standard names, and exit non-zero (principle №4:
fail fast with actionable errors). It SHALL NOT write document content to stdout.

- **GIVEN** a built `shll` binary
- **WHEN** the user runs `shll standards bogus`
- **THEN** stderr names the valid standard names (`principles`, `help-dump`, `readme-extraction`)
- **AND** stdout is empty
- **AND** the process exits non-zero

### CLI: standards roster (source of truth)

#### R5: The standards roster is a hardcoded Go slice; descriptions are not parsed from markdown
The set of standards, their names, one-line descriptions, and canonical
`source_path`s SHALL be a hardcoded slice of structs in Go source (analogous to
`Roster` in `tools.go`). Descriptions SHALL NOT be parsed out of the markdown at
runtime. The initial roster SHALL be exactly three entries:
`principles` → `docs/site/principles.md`, `help-dump` → `docs/site/help-dump.md`,
`readme-extraction` → `docs/site/readme-extraction.md` (Constitution: explicit
versioned lists are the contract).

- **GIVEN** the standards roster in Go source
- **WHEN** the binary is built
- **THEN** the roster has exactly the three named entries with non-empty descriptions and source paths
- **AND** no runtime markdown parsing produces the descriptions

### Build: single-source embedding with drift guard

#### R6: Canonical `docs/site/*.md` are embedded into the binary via an in-`src/` copy step
Because the Go module root is `src/` and `docs/site/` sits above it (`//go:embed`
cannot reach above the module root), the three canonical files SHALL be copied
into a location inside `src/` (`src/cmd/shll/standards/`) for embedding. The copy
SHALL be produced by a `scripts/` copy step (Constitution VI thin-justfile
pattern) that is wired into the build path and re-runnable, with the copies
committed so a clean `go build ./...` (which does not run the script) works.

- **GIVEN** the canonical `docs/site/{principles,help-dump,readme-extraction}.md`
- **WHEN** the copy step runs (`scripts/sync-standards.sh`, also `go generate`)
- **THEN** `src/cmd/shll/standards/*.md` byte-match the canonical sources
- **AND** the embedded copies are committed and a clean `go build ./...` succeeds

#### R7: A drift guard asserts the embedded copies byte-match the canonical sources
A Go test in `standards_test.go` SHALL assert that each embedded standard's bytes
byte-match the canonical `../../../docs/site/<name>.md` source, so the check runs
on every `go test ./...` and in the existing CI PR workflow (build, vet, test).
When the canonical source drifts from the committed copy, the test SHALL fail.

- **GIVEN** the embedded copies and the canonical sources
- **WHEN** `go test ./...` runs (locally or in CI)
- **THEN** the drift-guard test passes when they match
- **AND** it fails, naming the drifted file, when they differ

### CLI: wiring & repo-pattern conformance

#### R8: `shll standards` is registered in the root command and documented in `rootLong`
`newStandardsCmd()` (in `src/cmd/shll/standards.go`) SHALL be registered in
`newRootCmd()` (`src/cmd/shll/root.go`) alongside the existing subcommands, and a
one-line `shll standards` entry SHALL be added to the `rootLong` subcommand
listing. Flag names and usage strings SHALL be named constants (no magic
strings, per `code-quality.md`). No subprocess is involved — the command reads
embedded bytes only, so `internal/proc` is untouched (Constitution I vacuous).

- **GIVEN** the root command
- **WHEN** the binary is built and `shll --help` is shown
- **THEN** `standards` is a registered, visible subcommand listed in `rootLong`
- **AND** `help-dump` picks it up mechanically via the programmatic cobra walk (no `help_dump.go` change)

### Non-Goals

- `shll audit` (the future conformance checker) — separate design, out of scope.
- Deploying "run `shll standards`" stanzas to other toolkit repos' agent entry files — separate per-repo work.
- Any change to shll.ai's pull/render pipeline — `docs/site/` stays canonical and structurally untouched.
- Rendering markdown, paging, or syntax-highlighting the document output — raw bytes only.

### Design Decisions

1. **Embed via committed in-`src/` copies + `scripts/sync-standards.sh` + drift-guard test** (resolves intake assumption #7): `//go:embed` cannot reach above the module root (`src/go.mod`), so the canonical `docs/site/*.md` are copied to `src/cmd/shll/standards/` and embedded from there. — *Why*: the embedded copies must be committed for a clean `go build ./...` (and CI, which builds directly, not via `scripts/build.sh`) to compile; the `scripts/` copy step (wired into `build.sh` + a `//go:generate` directive) refreshes them per Constitution VI; a Go drift-guard test keeps them honest on every `go test` and in the existing CI test job — exactly the intake's "natural default". — *Rejected*: a symlink into `src/` (embed does not follow symlinks reliably across the toolchain and breaks the byte-copy guarantee); parsing markdown at runtime (fragile, violates the hardcoded-contract principle); a separate CI-only diff step (a Go test is simpler, runs everywhere `go test` runs, and needs no workflow edit).
2. **`--json` on the bare list form emitting `{name, description, source_path}`** (resolves intake assumption #8): direct precedent in `shll list --json`; principle №3's machine-readability posture. — *Rejected*: omitting `--json` (agents scripting standards discovery benefit from the stable field contract at negligible cost).
3. **Roster as a hardcoded `[]standard` slice** (resolves intake assumption #9): mirrors `tools.go`'s `Roster`; the constitution's "explicit versioned lists are the contract". — *Rejected*: deriving descriptions from markdown H1/first-paragraph (fragile to doc edits, couples the CLI surface to prose).

## Tasks

### Phase 1: Setup — embed assets & copy mechanism

- [x] T001 Create `scripts/sync-standards.sh`: copy `docs/site/{principles,help-dump,readme-extraction}.md` into `src/cmd/shll/standards/`, `set -euo pipefail`, executable, one-liner-delegating per Constitution VI. <!-- R6 -->
- [x] T002 Run `scripts/sync-standards.sh` to create the committed embedded copies under `src/cmd/shll/standards/*.md` (byte-identical to canonical). <!-- R6 -->
- [x] T003 Wire the copy step into `scripts/build.sh` (run before `go build`) and add a `just sync-standards` recipe line to `justfile`. <!-- R6 -->

### Phase 2: Core Implementation — the subcommand

- [x] T004 Create `src/cmd/shll/standards.go`: `//go:embed standards/*.md` embed.FS + `//go:generate` directive pointing at the sync script; a hardcoded `standardsRoster` `[]standard` slice (fields Name, Description, SourcePath, embed filename) with the three entries; named flag-name/usage constants; a `standardJSONItem` struct `{name, description, source_path}`. <!-- R5 -->
- [x] T005 Implement `newStandardsCmd()` (cobra `Use: "standards [name]"`, `Args: cobra.MaximumNArgs(1)`, `--json` flag) plus a testable `runStandards(stdout, args, jsonOut)` seam. <!-- R1 R2 R3 R4 R8 -->
- [x] T006 Implement the list form: no-arg → aligned `text/tabwriter` table (name + description) matching `shll list`'s writer config; `--json` → JSON array of `{name, description, source_path}` (SetEscapeHTML(false), 2-space indent, trailing newline), roster order. <!-- R1 R2 -->
- [x] T007 Implement the reader form: `shll standards <name>` → write the embedded bytes for that standard to stdout byte-identically; unknown name → actionable stderr error naming valid names + non-zero exit (reuse the `errSilent` sentinel, writing the diagnostic to `cmd.ErrOrStderr()`), nothing on stdout. <!-- R3 R4 -->

### Phase 3: Integration & wiring

- [x] T008 Register `newStandardsCmd()` in `newRootCmd()` (`src/cmd/shll/root.go`) and add the `shll standards` one-line entry to `rootLong`. <!-- R8 -->

### Phase 4: Tests

- [x] T009 Create `src/cmd/shll/standards_test.go` — list form: bare table has one row per roster entry (name + description, no ANSI on non-TTY); `--json` is valid JSON, len == roster, correct fields incl. `source_path`, no ANSI, trailing newline. <!-- R1 R2 -->
- [x] T010 Add reader-form tests: `shll standards principles` stdout == embedded bytes byte-for-byte; unknown name → error, stdout empty, valid names in the message. <!-- R3 R4 -->
- [x] T011 Add the drift-guard test `TestStandardsEmbedMatchesCanonical`: for each roster entry, embedded bytes == `../../../docs/site/<name>.md` bytes; and a roster-integrity test (non-empty fields, SourcePath under `docs/site/`). <!-- R7 R5 -->

## Execution Order

- T001 → T002 → T003 (copy step exists → copies produced → build/justfile wired)
- T002 blocks T004 (embed needs the files present to compile)
- T004 → T005 → {T006, T007} → T008
- Test tasks T009–T011 follow their implementation; T011 depends on T002 (embedded copies present).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `shll standards` lists all three standards, one aligned row each (name + description), exit 0.
- [x] A-002 R2: `shll standards --json` emits a valid JSON array of `{name, description, source_path}`, one per standard in roster order.
- [x] A-003 R3: `shll standards principles` prints the full canonical markdown byte-identical to the embedded source, exit 0.
- [x] A-004 R4: unknown name writes an actionable, valid-names-listing error to stderr, empty stdout, non-zero exit.
- [x] A-005 R5: the standards roster is a hardcoded Go slice with exactly the three named entries; descriptions are not parsed from markdown at runtime.
- [x] A-006 R6: the canonical files are copied into `src/` by `scripts/sync-standards.sh`, wired into the build path, and the committed copies let a clean `go build ./...` succeed.
- [x] A-007 R7: the drift-guard test asserts embedded == canonical and fails (naming the file) on drift.
- [x] A-008 R8: `standards` is registered in `newRootCmd()`, documented in `rootLong`, and appears in `help-dump` output with no `help_dump.go` change.

### Behavioral Correctness

- [x] A-009 R2: `--json` output is free of ANSI escapes and table framing regardless of TTY, with a single trailing newline (parity with `shll list --json`).
- [x] A-010 R3: reader output is raw markdown — no rendering, no pager, no added framing.

### Scenario Coverage

- [x] A-011 R1/R2/R3/R4: list (table + json), read, and unknown-name scenarios each have a test in `standards_test.go`.
- [x] A-012 R7: the drift guard runs under `go test ./...` (local and the existing CI PR workflow) with no workflow edit needed.

### Edge Cases & Error Handling

- [x] A-013 R4: `shll standards` with `>1` positional arg is rejected by cobra `Args` (MaximumNArgs(1)); unknown single name is the actionable-error path.
- [x] A-014 R3: byte-identity holds including trailing newline / no added/stripped bytes.

### Code Quality

- [x] A-015 Pattern consistency: `standards.go` follows `list.go`'s factory + `runX` seam, tabwriter config, and JSON encoder conventions; struct-slice roster mirrors `tools.go`.
- [x] A-016 No unnecessary duplication: reuses the `errSilent` sentinel and the `text/tabwriter` + JSON-encoder idioms rather than re-inventing; no magic strings (flag names, usage, source paths are named constants / roster fields).
- [x] A-017 Security (Constitution I): no subprocess, no `os/exec`, no `internal/proc` use — the command reads embedded bytes only (Constitution I applies vacuously; no shell surface introduced).

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Confident | Embed mechanism = committed `src/cmd/shll/standards/*.md` copies + `scripts/sync-standards.sh` (wired into `build.sh` + `//go:generate`) + Go drift-guard test | Intake assumption #7 delegated the mechanism to apply; committed copies are required for clean `go build ./...`/CI, `scripts/` copy matches Constitution VI, a Go test is the simplest guard that runs everywhere `go test` runs (no CI workflow edit) | S:70 R:85 A:80 D:75 |
| 2 | Confident | Embedded copies live at `src/cmd/shll/standards/` (subdir beside `standards.go`), embedded via `//go:embed standards/*.md` | Keeps embed assets colocated with the consuming command file; a subdir keeps `*.md` glob scoped and avoids polluting the package dir | S:65 R:85 A:80 D:70 |
| 3 | Confident | Unknown-name error uses the `errSilent` sentinel (write diagnostic to stderr, return errSilent → exit 1) | Direct in-repo idiom (`main.go` `translateExit`); avoids cobra double-printing; non-zero exit satisfies R4/principle №4 | S:75 R:85 A:85 D:80 |
| 4 | Confident | `--json` `source_path` is the repo-relative canonical path (`docs/site/<name>.md`), sourced from the roster's `SourcePath` field | Intake states source_path = repo-relative canonical path explicitly; single-sourced in the roster so JSON and the drift guard agree | S:75 R:85 A:85 D:80 |
| 5 | Certain | `standards` cobra `Args` = `cobra.MaximumNArgs(1)` (bare = list, one arg = read) | Intake defines exactly two forms (bare list, single-name read); MaximumNArgs(1) is the minimal cobra guard expressing that | S:90 R:85 A:90 D:90 |

5 assumptions (1 certain, 4 confident, 0 tentative).
