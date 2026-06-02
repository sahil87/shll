# Plan: Help-Dump CLI Tree → shll.ai Command Reference

**Change**: 260602-ep4z-help-dump-cli-tree
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### help-dump: Subcommand

#### R1: Hidden help-dump subcommand
The binary SHALL expose a `help-dump` cobra subcommand that is `Hidden: true`, takes `cobra.NoArgs`, and writes its payload to `cmd.OutOrStdout()`. It SHALL be registered in `newRootCmd()`'s existing `cmd.AddCommand(...)` block via `newHelpDumpCmd()`.

- **GIVEN** a built `shll` binary
- **WHEN** the user runs `shll --help`
- **THEN** `help-dump` does NOT appear in the listed commands (it is hidden)
- **AND** running `shll help-dump` succeeds and emits JSON to stdout

#### R2: Anchored to the live root
The subcommand SHALL walk the tree starting from `cmd.Root()` (the actual root the command is attached to), not a captured package-level variable, so the walk is correct regardless of how the tree was assembled in tests.

- **GIVEN** a synthetic root command assembled in a test
- **WHEN** `runHelpDump` is invoked with that root
- **THEN** the emitted tree reflects that synthetic root's children, not the production root

### help-dump: JSON Contract

#### R3: Top-level document shape
The dump SHALL emit a single JSON object with fields, in order: `tool` (literal `"shll"`), `version`, `captured_at`, `schema_version` (literal int `1`), and `root` (a Node). It SHALL be encoded with `json.MarshalIndent(doc, "", "  ")` (2-space indent) followed by a single trailing newline, and nothing else SHALL be written to stdout.

- **GIVEN** the real root command
- **WHEN** `shll help-dump` runs
- **THEN** stdout parses as JSON with keys `tool`, `version`, `captured_at`, `schema_version`, `root`
- **AND** `tool == "shll"`, `schema_version == 1`
- **AND** the output ends with exactly one `\n` and contains no non-JSON log lines

#### R4: Node shape and field mapping
Each Node SHALL carry, in order: `name` (`cmd.Name()`), `path` (`cmd.CommandPath()`), `short` (`cmd.Short`), `usage` (`cmd.UseLine()`), `text` (the raw `-h` output — see R6), and `commands` (`[]Node`, recursive over visible children).

- **GIVEN** the `install` command in the tree
- **WHEN** the dump is produced
- **THEN** its Node has `name == "install"`, `path == "shll install"`, `short` equal to `cmd.Short`, `usage == "shll install [flags]"`

#### R5: `commands` is `[]`, never `null`
A leaf Node's `commands` field SHALL serialize as `[]`, not `null`. The children slice SHALL be initialized non-nil before appending.

- **GIVEN** a leaf command (no visible children)
- **WHEN** the dump is produced
- **THEN** its `commands` field is `[]` in the JSON

#### R6: `text` equals the command's help-template output byte-for-byte
The `text` field SHALL equal the command's `cmd.Help()` (help-template) output byte-for-byte — the enforceable form of "RAW -h output". It SHALL be constructed to match cobra's default help func: `trimRightSpace(Long || Short)` followed by `"\n\n"` then `UsageString()` (the leading blurb+blank-line is omitted when both `Long` and `Short` are empty). The producer SHALL initialize each command's `-h`/`--version` flags (`InitDefaultHelpFlag`/`InitDefaultVersionFlag`) before capturing usage/text, so the `[flags]` UseLine and `Flags:` block render exactly as the binary's `-h`. The real binary invokes `help-dump` via `rootCmd.Execute()`, which lazily registers cobra's `completion`/`help` SUBcommands BEFORE the matched RunE fires — so at walk time they exist on the live tree and `UsageString()` would otherwise list them. The producer SHALL therefore prune all skip-listed children (`completion`/`help`/Hidden) from the live tree before rendering (see Assumptions #1), so a parent node's `Available Commands:` text lists exactly its dumped children.

- **GIVEN** any visible command in the real tree
- **WHEN** the dump is produced AND the command's `cmd.Help()` output is captured into a buffer
- **THEN** `node.text` equals that captured output byte-for-byte
- **AND** the root node's `Available Commands:` block lists exactly the dumped children (no `completion`/`help`)

### help-dump: Filtering & Ordering

#### R7: Child filtering
For every node's children (recursively), the walk SHALL skip a child when ANY holds: `child.Name() == "completion"`, `child.Name() == "help"`, `child.Hidden == true`, or `!child.IsAvailableCommand()`. Because `help-dump` is itself `Hidden`, this rule self-excludes it.

- **GIVEN** a tree containing a `completion` command, a `help` command, and a hidden command
- **WHEN** the dump is produced
- **THEN** none of those three appear anywhere in the emitted tree
- **AND** `help-dump` does not appear in the dump of the real root

#### R8: Order preservation
The walk SHALL preserve cobra's `Commands()` ordering (cobra's default alphabetical sort) and SHALL NOT re-sort children beyond what cobra returns.

- **GIVEN** the real root's visible children
- **WHEN** the dump is produced
- **THEN** the `commands` array order matches `cmd.Commands()` (filtered) order

### help-dump: Top-level Field Derivation

#### R9: Version from the binary
`version` SHALL be read from `cmd.Root().Version` (ldflags-stamped via `main.version`), never hardcoded.

- **GIVEN** a root with `Version = "v9.9.9"`
- **WHEN** the dump is produced
- **THEN** `doc.version == "v9.9.9"`

#### R10: `captured_at` at date granularity
`captured_at` SHALL be an ISO-8601 UTC timestamp truncated to date granularity (`YYYY-MM-DDT00:00:00Z`), so same-day re-runs are byte-identical.

- **GIVEN** two successive dumps on the same day
- **WHEN** both are produced
- **THEN** `captured_at` matches the regex `^\d{4}-\d{2}-\d{2}T00:00:00Z$` in both
- **AND** the two dumps are byte-identical except (at most) `captured_at`

### help-dump: CI Publishing

#### R11: Native dump binary build
`release.yml`'s `release` job SHALL build a dedicated native `linux/amd64` binary (`CGO_ENABLED=0 GOOS=linux GOARCH=amd64`) stamped with the release tag via `-ldflags "-X main.version=<tag>"`, independent of the cross-compile matrix, solely to run `help-dump`.

- **GIVEN** a release tag push
- **WHEN** the workflow runs
- **THEN** a native binary is built at a temp path stamped with the release tag

#### R12: Generate and validate the dump
The workflow SHALL run the native binary's `help-dump`, redirect stdout to `help/shll.json`, validate it parses (`jq empty`), and assert the embedded `version` equals the release tag.

- **GIVEN** the native binary
- **WHEN** the generate step runs
- **THEN** `help/shll.json` exists, parses as JSON, and its `.version` equals the tag (job fails otherwise)

#### R13: Publish to shll.ai via auto-merge PR
The workflow SHALL publish `help/shll.json` into `sahil87/shll.ai` via a per-release branch (`shll-help-<tag>`), commit, push, `gh pr create`, and `gh pr merge --auto --squash`, authed via `SHLLAI_TOKEN` — never a direct push to `main`. A no-op guard (`git diff --cached --quiet`) SHALL skip the PR cleanly when the file is byte-identical to shll.ai's `main`.

- **GIVEN** a generated `help/shll.json` that differs from shll.ai's `main`
- **WHEN** the publish step runs
- **THEN** a PR is opened into `sahil87/shll.ai` from branch `shll-help-<tag>` and set to auto-merge (squash)
- **AND GIVEN** the file is byte-identical to `main`, **THEN** the step prints a skip message and exits 0 without opening a PR

## Tasks

### Phase 1: Core Implementation

- [x] T001 Create `src/cmd/shll/help_dump.go`: package `main`; `helpDoc`/`helpNode` structs with exact JSON tags and field order; `newHelpDumpCmd()` factory (`Hidden: true`, `Args: cobra.NoArgs`, `RunE` → `runHelpDump(cmd.Root(), cmd.OutOrStdout())`); `runHelpDump(root *cobra.Command, w io.Writer) error` building the doc and writing `MarshalIndent` + trailing `\n`; named constants for `tool`/`schema_version`/excluded command names; `nodeText` matching cobra's `defaultHelpFunc` (`trimRightSpace` via `strings.TrimRightFunc(..., unicode.IsSpace)`); `buildNode` recursive walk; child filter helper; `capturedAt()` date-granularity helper. Also prune skip-listed commands (`completion`/`help`/Hidden) from the live tree (via `pruneSkipped`: `InitDefaultHelpCmd`/`InitDefaultCompletionCmd` then recursive `RemoveCommand`) BEFORE rendering, so each node's `text` "Available Commands:" block lists exactly its `commands` entries. <!-- R1 R2 R3 R4 R5 R6 R7 R8 R9 R10 --> <!-- rework: text/commands incoherence — completion/help leaked into root text under Execute(); tests bypassed Execute() -->
- [x] T002 Edit `src/cmd/shll/root.go`: add `newHelpDumpCmd()` to the existing `cmd.AddCommand(...)` block (after `newVersionCmd()`). <!-- R1 -->

### Phase 2: Tests

- [x] T003 Create `src/cmd/shll/help_dump_test.go`: contract-shape test (synthetic root + visible child + hidden child + `completion` child + `help`; assert top-level keys, `schema_version==1`, `tool=="shll"`, leaf `commands` is `[]` not null, filtered children absent); `text` byte-for-byte test (every visible command in real `newRootCmd()` compared against captured `cmd.Help()` output); self-exclusion test (`help-dump` absent from real-tree dump); version-passthrough test (`root.Version="v9.9.9"` → `doc.version=="v9.9.9"`); `captured_at` regex shape test; structural-determinism test (two dumps differ only in `captured_at`). Also drive the dump through the real `rootCmd.Execute()` path (`dumpViaExecute` helper) so cobra's lazy `completion`/`help` are registered exactly as on the shipped binary, and assert (per node) they appear in NEITHER `commands` NOR the rendered `text` "Available Commands:" block (`TestHelpDump_RootTextExcludesAutoCommands` + `TestHelpDump_ExcludesAutoCommandsEverywhere`). <!-- R3 R4 R5 R6 R7 R9 R10 --> <!-- rework: text/commands incoherence — completion/help leaked into root text under Execute(); tests bypassed Execute() -->

### Phase 3: CI Integration

- [x] T004 Edit `.github/workflows/release.yml`: after the **Cross-compile** step (or after Create GitHub Release), add three steps to the `release` job — (a) "Build native binary for help-dump" (`working-directory: src`, native linux/amd64, ldflags tag); (b) "Generate help/shll.json" (`mkdir -p help`, run dump > file, `jq empty`, version==tag assertion); (c) "Publish to shll.ai" (env `SHLLAI_TOKEN` + `GH_TOKEN`, clone, copy, per-release branch, no-op guard, commit/push, `gh pr create`, `gh pr merge --auto --squash`). Match existing pinned-SHA / step-naming / working-directory style. <!-- R11 R12 R13 -->

### Phase 4: Polish

- [x] T005 Add a light note in `README.md` that shll publishes a machine-readable help tree to shll.ai on release, documenting `help-dump` as hidden build tooling (not a user command). <!-- R1 -->

## Execution Order

- T001 blocks T002, T003 (they reference its symbols / registration)
- T003 depends on T001 + T002 (tests run against the registered real tree)
- T004, T005 are independent of the Go code beyond the command existing

## Acceptance

### Functional Completeness

- [x] A-001 R1: `help-dump` is a `Hidden: true`, `NoArgs` cobra subcommand registered in `newRootCmd()`; absent from `shll --help`; `shll help-dump` emits JSON to stdout.
- [x] A-002 R2: `runHelpDump` walks from the passed root; a synthetic-root test reflects that root's children.
- [x] A-003 R3: Output is a single JSON object with ordered keys `tool`/`version`/`captured_at`/`schema_version`/`root`, 2-space `MarshalIndent`, one trailing newline, no extra stdout lines; `tool=="shll"`, `schema_version==1`.
- [x] A-004 R4: Each Node maps `name`/`path`/`short`/`usage`/`text`/`commands` from `Name()`/`CommandPath()`/`Short`/`UseLine()`/raw-help/recursive children.
- [x] A-005 R5: Leaf `commands` serializes as `[]`, never `null` (verified in test).
- [x] A-009 R9: `version` is read from `cmd.Root().Version`; version-passthrough test asserts `v9.9.9` (no hardcoding).
- [x] A-010 R10: `captured_at` matches `^\d{4}-\d{2}-\d{2}T00:00:00Z$`.
- [x] A-011 R11: `release.yml` builds a dedicated native linux/amd64 binary stamped with the tag.
- [x] A-012 R12: `release.yml` generates `help/shll.json`, runs `jq empty`, and asserts `.version` == tag.
- [x] A-013 R13: `release.yml` publishes via per-release branch + `gh pr create` + `gh pr merge --auto --squash` authed by `SHLLAI_TOKEN`, never direct push; no-op guard skips cleanly.

### Behavioral Correctness

- [x] A-006 R6: For every visible command in the real tree, `node.text` equals captured `cmd.Help()` output byte-for-byte (test enforces); verified out-of-band that this equals the live binary's `<cmd> -h` for all nodes.
- [x] A-023 R6: A parent node's `text` `Available Commands:` block lists exactly its dumped children — `completion`/`help` appear in neither text nor `commands` (TestHelpDump_RootTextExcludesAutoCommands).
- [x] A-007 R7: `completion`, `help`, and any `Hidden` child (incl. `help-dump`) are excluded recursively.
- [x] A-008 R8: Children order matches cobra's filtered `Commands()` order (no extra sort).

### Scenario Coverage

- [x] A-014 R10: Structural-determinism test confirms two same-day dumps differ only in `captured_at` (here, are byte-identical).
- [x] A-015 R7: Contract-shape test exercises a synthetic tree with hidden/`completion`/`help` children and asserts their absence.

### Edge Cases & Error Handling

- [x] A-016 R6: `nodeText` omits the leading blurb+blank-line when both `Long` and `Short` are empty (matches cobra), and applies `trimRightSpace` to the blurb.
- [x] A-017 R13: When `git diff --cached --quiet` shows no change, the publish step prints a skip message and exits 0 (no empty PR).

### Code Quality

- [x] A-018 Pattern consistency: `help_dump.go` matches sibling files — package `main`, `newXxxCmd()` factory + extracted `runXxx(io.Writer)` seam, doc comments on important identifiers.
- [x] A-019 No unnecessary duplication: reuses cobra's data model (no regex on `-h`); no reinvented help rendering beyond the documented template-match.
- [x] A-020 No magic strings: `"shll"`, `1`, `"completion"`, `"help"`, and the `captured_at` layout are named constants (code-quality.md forbids magic strings).
- [x] A-021 No subprocess in producer: `help_dump.go` performs a pure in-process tree walk — no `os/exec`, no `internal/proc` (Constitution I N/A to the producer per intake #13).
- [x] A-022 Cross-platform / stdlib-only: only `encoding/json`, `time`, `strings`, `unicode`, `io`, `cobra` — no new go.mod deps.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- Reference `help/wt.json` lives in `sahil87/shll.ai`, not this repo; the byte-for-byte `text` test against real `-h` is the enforceable fidelity contract (intake risk note).
- The cobra `defaultHelpFunc` (v1.10.2) renders `trimRightSpace(Long||Short)` + `"\n\n"` + `UsageString()`; `nodeText` matches this exactly. See `## Assumptions` #1.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Skip-listed commands (`completion`/`help`/Hidden) are removed from the LIVE command tree (via `pruneSkipped`: `InitDefaultHelpCmd`/`InitDefaultCompletionCmd` to force cobra's lazy registration, then recursive `RemoveCommand`) BEFORE any node's `text` is rendered. Consequence: for every node, `UsageString()`'s `Available Commands:` block lists EXACTLY that node's `commands` entries — `completion`/`help` appear in neither — so `text` ↔ `commands` are coherent and match the frozen `wt.json` reference. | Reworked after review — A-023. The earlier assumption (text comes from a pure in-process walk that never sees completion/help) was WRONG for the real binary: `help-dump` runs under `rootCmd.Execute()`, which lazily registers `completion`/`help` BEFORE the RunE fires, so they were live at walk time and leaked into the root's rendered `text` while the `commands` array (correctly filtered) omitted them — an internally incoherent split that also diverged from `wt.json`. Pruning the live tree first is the resolution; verified end-to-end against the Execute-built binary (`/tmp/shll-rework help-dump`) and by an Execute-path regression test that fails pre-fix and passes post-fix. | S:97 R:80 A:95 D:92 |
| 6 | Certain | `nodeText` mirrors cobra v1.10.2 `defaultHelpFunc` exactly: `strings.TrimRightFunc(blurb, unicode.IsSpace)` then `+ "\n\n" + UsageString()`, blurb+blank-line omitted when blurb empty; flags initialized via `InitDefaultHelpFlag`/`InitDefaultVersionFlag` in `buildNode` so `[flags]`/`Flags:` render like real `-h`. | Read the actual cobra source (`command.go:2046` `defaultHelpFunc`, `1219` `InitDefaultHelpFlag`); without flag-init, leaf `text` lacked the `-h` flag and `[flags]` suffix that real `-h` shows. Now leaf `text` matches the live binary's `<cmd> -h` byte-for-byte (verified against `/tmp/shll-ep4z`). | S:97 R:80 A:95 D:90 |
| 2 | Certain | `gh` in the publish step is authed by exporting `GH_TOKEN: ${{ secrets.SHLLAI_TOKEN }}` in the step `env` (alongside `SHLLAI_TOKEN`), so `gh pr create`/`merge` target shll.ai. | Intake's apply-stage note explicitly says `gh` needs `GH_TOKEN=${SHLLAI_TOKEN}`; this is the standard `gh` auth mechanism in Actions. | S:95 R:75 A:90 D:88 |
| 3 | Certain | Workflow-level `permissions: contents: write` is left unchanged; the cross-repo PR is authed entirely via `SHLLAI_TOKEN`. | Intake §3 says GITHUB_TOKEN perms need not change for the cross-repo write; confirmed by reading the existing workflow. | S:96 R:70 A:92 D:90 |
| 4 | Certain | The three CI steps are inserted after **Create GitHub Release** (so a Release exists, per intake "after build and after a GitHub Release exists"), before/around **Update Homebrew tap**. | Intake §3: "After the existing Cross-compile step (and after a GitHub Release exists)". Placement after the Release step satisfies both. | S:95 R:65 A:88 D:85 |
| 5 | Confident | Recursion uses `cmd.Commands()` per node and applies the filter to each child; the root node itself is always included (root is never filtered — filtering applies to *children* per intake). | Intake: "Filtering (applied to every node's children, recursively)"; the root is the dump anchor. | S:92 R:70 A:85 D:80 |

6 assumptions (5 certain, 1 confident, 0 tentative).
