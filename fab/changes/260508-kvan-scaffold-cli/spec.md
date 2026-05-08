# Spec: Scaffold shll CLI

**Change**: 260508-kvan-scaffold-cli
**Created**: 2026-05-09
**Affected memory**: `docs/memory/cli/commands.md`, `docs/memory/cli/update.md`, `docs/memory/cli/shell-init.md`, `docs/memory/cli/version.md`, `docs/memory/internal/proc.md`

## Non-Goals

- The `all.rb` formula bump in `sahil87/homebrew-tap` — separate change in the tap repo, tracked here only for traceability.
- Soft-deprecation of per-tool `update` commands (e.g., `hop update`, `fab-kit update`) — Constitution IV mandates they continue to work standalone.
- A `shll update --dry-run` / `--check` flag — deferred to v0.2.0 unless trivial; not part of v0.1.0 scope.
- A `--json` output mode for `shll version` — primary use case (bug reports) is plain text; deferred until a real script-consumer emerges.
- Windows support — Constitution explicitly excludes Windows.
- Dynamic discovery of sahil87 tools (e.g., from `brew tap`) — Constitution III requires the roster to be hardcoded.
- A `shll completion` subcommand — cobra provides one for free; not a v0.1.0 deliverable to write or document.

## Repository: Scaffold

### Requirement: Repo Layout
The repository SHALL mirror hop's structure. The top level SHALL contain `src/` (Go module), `scripts/` (build/install/release shell scripts), `justfile`, `README.md`, `LICENSE` (MIT), `.gitignore`, and `.github/workflows/release.yml`. Existing `fab/` and `docs/` directories SHALL remain in place.

#### Scenario: Top-level layout
- **GIVEN** a fresh checkout of the shll repo
- **WHEN** the scaffold is complete
- **THEN** the repo root contains `src/`, `scripts/`, `justfile`, `README.md`, `LICENSE`, `.gitignore`, `.github/workflows/release.yml`
- **AND** `fab/` and `docs/` are unchanged

#### Scenario: Source tree shape
- **GIVEN** the scaffolded `src/` directory
- **WHEN** inspecting its contents
- **THEN** `src/go.mod` declares the module path and Go version ≥1.22
- **AND** `src/cmd/shll/` contains `main.go`, `root.go`, `update.go`, `shell_init.go`, `version.go`, and `tools.go` (or the roster lives in `root.go`)
- **AND** `src/internal/proc/` contains `proc.go` exporting `Run`, `RunForeground`, and `ErrNotFound`

### Requirement: Build System
The build system SHALL follow Constitution VI (Thin Justfile, Fab-Kit Build Pattern). Justfile recipes SHALL be one-line delegations to `scripts/`. Logic, loops, and conditionals SHALL live in shell scripts, never in the justfile.

#### Scenario: Justfile recipes are thin
- **GIVEN** the project's `justfile`
- **WHEN** any recipe body is inspected
- **THEN** the recipe body is a single command line invoking `scripts/<name>.sh`
- **AND** no recipe contains shell loops, conditionals, or multi-step pipelines

#### Scenario: Build produces a working binary
- **GIVEN** Go ≥1.22 is installed
- **WHEN** `just build` is run from the repo root
- **THEN** `scripts/build.sh` cross-compiles or compiles for the host platform
- **AND** the resulting `shll` binary, when invoked with `--version`, prints a non-empty version string

#### Scenario: Local install
- **GIVEN** a successful `just build`
- **WHEN** `just install` is run
- **THEN** `scripts/install.sh` places the `shll` binary on the user's `PATH` (matching hop's local install convention)

### Requirement: Cross-Platform
The binary SHALL build and run on `darwin/arm64`, `darwin/amd64`, `linux/arm64`, and `linux/amd64`. Platform-specific code SHALL be isolated behind a small abstraction. Windows is not supported.

#### Scenario: Cross-compile matrix
- **GIVEN** the release workflow
- **WHEN** a `v*` tag is pushed
- **THEN** binaries are produced for `darwin/arm64`, `darwin/amd64`, `linux/arm64`, `linux/amd64`
- **AND** no Windows artifact is produced

## CLI: Root Command

### Requirement: Cobra Root
The root command SHALL be implemented using `github.com/spf13/cobra`. The root command SHALL expose three subcommands: `update`, `shell-init`, `version`. Constitution VII justification for each subcommand SHALL be documented in this spec (see Design Decisions).

#### Scenario: Help lists three subcommands
- **GIVEN** the built `shll` binary
- **WHEN** `shll --help` is invoked
- **THEN** stdout includes the lines for `update`, `shell-init`, and `version`

#### Scenario: Unknown subcommand
- **GIVEN** the built `shll` binary
- **WHEN** `shll bogus` is invoked
- **THEN** the binary exits non-zero
- **AND** stderr contains a cobra-style "unknown command" message

### Requirement: Tool Roster
The tool roster SHALL be a hardcoded slice of `Tool{Name, Formula, ShellInit}` entries defined in `cmd/shll/tools.go` (or `root.go`). The initial roster SHALL contain exactly: `fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea`. Each entry SHALL declare its brew formula (`sahil87/tap/<name>`) and its shell-init invocation (or empty string when the tool has no shell integration). The roster SHALL NOT be discovered at runtime.

#### Scenario: Roster contents
- **GIVEN** the built `shll` binary
- **WHEN** the roster is referenced internally
- **THEN** the roster contains exactly six entries with names: `fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea`
- **AND** `hop`'s shell-init invocation is `hop shell-init <shell>`
- **AND** `wt`'s shell-init invocation is `wt shell-setup`
- **AND** the other four tools have empty shell-init invocations

#### Scenario: No dynamic discovery
- **GIVEN** the source code
- **WHEN** the roster is searched for
- **THEN** there is no code path that calls `brew tap`, parses tap output, reads filesystem directories, or otherwise enumerates sahil87 tools at runtime

## CLI: update

### Requirement: Update Behavior
The `shll update` subcommand SHALL update every installed sahil87 tool by composing `brew update` with `brew upgrade <formula>` per installed tool. The subcommand SHALL be idempotent — re-running with no upstream changes SHALL produce no errors.

#### Scenario: Happy path with installed tools
- **GIVEN** brew is on PATH
- **AND** at least one sahil87 tool is installed
- **WHEN** `shll update` runs
- **THEN** `brew update --quiet` is invoked first
- **AND** for each installed tool in the roster, `brew upgrade sahil87/tap/<formula>` is invoked
- **AND** brew's output is streamed to the user's terminal (stdout/stderr inherited)
- **AND** the binary exits zero

#### Scenario: Brew not installed
- **GIVEN** `brew` is not on PATH
- **WHEN** `shll update` runs
- **THEN** stderr contains the message `shll update requires Homebrew. Install from https://brew.sh`
- **AND** the binary exits non-zero

#### Scenario: No sahil87 tools installed
- **GIVEN** brew is on PATH
- **AND** no sahil87 tools are installed
- **WHEN** `shll update` runs
- **THEN** stdout contains `No sahil87 tools installed.`
- **AND** the binary exits zero
- **AND** `brew update` is NOT invoked (skip the metadata refresh when there is nothing to upgrade — see Design Decision #9)

#### Scenario: Some tools installed, some not
- **GIVEN** brew is on PATH
- **AND** `hop` and `wt` are installed but `idea` is not
- **WHEN** `shll update` runs
- **THEN** `brew upgrade sahil87/tap/hop` and `brew upgrade sahil87/tap/wt` are invoked
- **AND** `brew upgrade sahil87/tap/idea` is NOT invoked
- **AND** no warning or error is printed for `idea` (Constitution V — graceful degradation)

#### Scenario: Brew upgrade of one tool fails
- **GIVEN** brew is on PATH
- **AND** `hop` and `wt` are installed
- **AND** `brew upgrade sahil87/tap/hop` fails (non-zero exit)
- **WHEN** `shll update` runs
- **THEN** the failure is surfaced to the user (brew's stderr is visible)
- **AND** `shll update` continues with the next tool (`brew upgrade sahil87/tap/wt`)
- **AND** the binary exits non-zero, reflecting that at least one upgrade failed

### Requirement: Installed Detection
Installation status of each roster tool SHALL be determined by invoking `brew list --formula --versions sahil87/tap/<formula>` (or equivalent that does not parse `brew list` plain output with regex). Detection SHALL NOT depend on the running binary's symlink target — that approach only works for the running tool, not for querying others.

#### Scenario: Detection method
- **GIVEN** the source code for `shll update`
- **WHEN** installation status of a roster tool is checked
- **THEN** detection is via `brew list --formula --versions sahil87/tap/<formula>` exit code (or `brew info --json=v2` parsing — never regex over plain `brew list` output)

### Requirement: Sequential Execution
`shll update` SHALL run upgrades sequentially, not in parallel. This avoids brew lock contention and keeps streamed output coherent.

#### Scenario: Sequential upgrade
- **GIVEN** brew is on PATH
- **AND** multiple sahil87 tools are installed
- **WHEN** `shll update` runs
- **THEN** at most one `brew upgrade` subprocess is in flight at any time

## CLI: shell-init

### Requirement: Shell-init Composition
The `shll shell-init <shell>` subcommand SHALL emit a single concatenated shell-init blob composed from each roster tool that exposes shell integration. The output SHALL be eval-safe — no error messages, no usage hints, no human-readable diagnostics on stdout.

#### Scenario: zsh with hop and wt installed
- **GIVEN** `hop` and `wt` are installed
- **WHEN** `shll shell-init zsh` runs
- **THEN** stdout is the concatenation of `hop shell-init zsh` stdout and `wt shell-setup` stdout (in roster order)
- **AND** the binary exits zero
- **AND** the resulting output is eval-safe in a real zsh shell

#### Scenario: bash with hop installed and wt missing
- **GIVEN** `hop` is installed and `wt` is not
- **WHEN** `shll shell-init bash` runs
- **THEN** stdout contains only `hop shell-init bash` output
- **AND** no message is printed for `wt`
- **AND** the binary exits zero

#### Scenario: No integrating tools installed
- **GIVEN** neither `hop` nor `wt` are installed
- **WHEN** `shll shell-init zsh` runs
- **THEN** stdout is empty (eval-safe no-op)
- **AND** the binary exits zero

#### Scenario: Unsupported shell
- **GIVEN** the user runs `shll shell-init fish`
- **WHEN** the subcommand executes
- **THEN** stdout is empty
- **AND** stderr contains usage text indicating only `zsh` and `bash` are supported
- **AND** the binary exits non-zero

#### Scenario: Missing shell argument
- **GIVEN** the user runs `shll shell-init` with no argument
- **WHEN** the subcommand executes
- **THEN** stdout is empty
- **AND** stderr contains usage text
- **AND** the binary exits non-zero

### Requirement: Composition Order
Shell-init output SHALL be concatenated in roster order (the order tools appear in the hardcoded roster). The order SHALL be deterministic so users can reason about init sequencing.

#### Scenario: Deterministic order
- **GIVEN** both `hop` and `wt` are installed
- **WHEN** `shll shell-init zsh` runs twice
- **THEN** both invocations produce byte-identical stdout

### Requirement: Sub-tool Failure Handling
If a sub-tool's shell-init invocation fails (non-zero exit), `shll shell-init` SHALL skip that tool's output without polluting stdout, log a single line to stderr noting the failure, and continue with subsequent tools. The overall exit code SHALL be non-zero if any sub-tool's shell-init failed, but stdout MUST remain eval-safe.

#### Scenario: One sub-tool fails
- **GIVEN** `hop` is installed but `hop shell-init zsh` exits non-zero
- **AND** `wt` is installed and works
- **WHEN** `shll shell-init zsh` runs
- **THEN** stdout contains only `wt shell-setup` output (eval-safe)
- **AND** stderr contains a one-line note about the `hop` failure
- **AND** the binary exits non-zero

## CLI: version

### Requirement: Version Output
The `shll version` subcommand SHALL print a column-aligned plain-text table showing the version of `shll` itself plus every roster tool. Output SHALL be plain text without colors, easy to paste into bug reports.

#### Scenario: All tools installed
- **GIVEN** all six sahil87 tools are installed
- **WHEN** `shll version` runs
- **THEN** stdout contains seven rows: one for `shll` followed by one per roster tool in roster order
- **AND** each row contains the tool name and its version string, column-aligned
- **AND** the binary exits zero

#### Scenario: Some tools missing
- **GIVEN** `idea` is not installed
- **WHEN** `shll version` runs
- **THEN** the row for `idea` reads `idea      not installed`
- **AND** the binary exits zero

#### Scenario: shll version injected via ldflags
- **GIVEN** the binary is built via `scripts/build.sh`
- **WHEN** `shll version` runs
- **THEN** the row for `shll` shows the version string injected at build time via `-ldflags`
- **AND** an unset version string defaults to `dev` (or equivalent, matching hop's pattern)

### Requirement: Version Invocation Timeout
Each per-tool version invocation SHALL have a 2-second timeout. A tool that does not respond within the timeout SHALL be reported as `not installed` (or a similar non-blocking status). The timeout SHALL be a named constant.

#### Scenario: A tool hangs
- **GIVEN** `hop` is installed but its `--version` invocation hangs
- **WHEN** `shll version` runs
- **THEN** after 2 seconds, the `hop` row is finalized as `not installed` (or a similar status)
- **AND** subsequent tools still produce their version rows
- **AND** the total runtime is bounded by `len(roster) * 2s` worst case

### Requirement: Plain-text Format
The output of `shll version` SHALL be plain text. No colors, no JSON, no other format MAY be added in this change.

#### Scenario: No ANSI in output
- **GIVEN** the built `shll` binary
- **WHEN** `shll version` runs (in any terminal, with or without TTY)
- **THEN** stdout contains no ANSI escape sequences

## Subprocess Execution: internal/proc

### Requirement: Proc Wrapper
All subprocess invocations from command code SHALL route through `internal/proc`. The package SHALL expose at minimum: `Run` (capture stdout/stderr), `RunForeground` (inherit stdio for interactive/colored output), and `ErrNotFound` (sentinel for "binary not on PATH"). The package SHALL use `exec.CommandContext` with explicit argument slices — never shell strings, never `exec.Command` without a context.

#### Scenario: No raw os/exec in command code
- **GIVEN** the source tree under `cmd/shll/`
- **WHEN** the source is grep'd for `os/exec` imports or `exec.Command(`
- **THEN** there are zero direct usages — all subprocess execution goes through `internal/proc`

#### Scenario: Context required
- **GIVEN** the source code for `internal/proc`
- **WHEN** subprocess invocation is inspected
- **THEN** every `exec.*` call uses `exec.CommandContext` with a non-nil context
- **AND** no call uses `exec.Command` without a context wrapper

#### Scenario: Argument-slice form only
- **GIVEN** the source code for `internal/proc`
- **WHEN** subprocess invocation is inspected
- **THEN** every invocation passes a binary path and an explicit `[]string` of arguments
- **AND** no invocation uses a shell-interpreted command string (`sh -c "..."` is not used to assemble shll's own commands)

#### Scenario: ErrNotFound when binary missing
- **GIVEN** a binary not on PATH
- **WHEN** `proc.Run` is invoked with that binary
- **THEN** the returned error wraps `proc.ErrNotFound`
- **AND** callers can use `errors.Is(err, proc.ErrNotFound)` to branch

### Requirement: RunForeground for User-Visible Subprocesses
`brew update` and `brew upgrade` SHALL be invoked via `RunForeground` so that brew's progress output is visible to the user. `Run` (captured) SHALL be used for `brew list`, `brew info`, and per-tool `--version` / shell-init invocations where shll consumes the output.

#### Scenario: brew upgrade is foregrounded
- **GIVEN** the source code for `shll update`
- **WHEN** the brew upgrade invocation is inspected
- **THEN** the call is `proc.RunForeground` (or equivalent foreground-streaming variant)

#### Scenario: brew list is captured
- **GIVEN** the source code for installed-tool detection
- **WHEN** the `brew list` invocation is inspected
- **THEN** the call is `proc.Run` (output captured for inspection)

## Distribution: Homebrew & Release

### Requirement: Release Workflow
A GitHub Actions workflow at `.github/workflows/release.yml` SHALL trigger on `v*` tag pushes and SHALL: (1) cross-compile binaries for the four supported platforms, (2) create a GitHub Release with the binaries attached, (3) open or update a PR in `sahil87/homebrew-tap` updating `Formula/shll.rb` to the new version and SHA-256.

#### Scenario: Tag triggers workflow
- **GIVEN** the `release.yml` workflow file is present
- **WHEN** the file is inspected
- **THEN** `on:` includes `push: tags: [v*]`
- **AND** `jobs:` includes a build matrix covering darwin-arm64, darwin-amd64, linux-arm64, linux-amd64
- **AND** `jobs:` includes a step that creates a GitHub Release
- **AND** `jobs:` includes a step that opens/updates a PR against `sahil87/homebrew-tap`

### Requirement: Tap Formula
A `Formula/shll.rb` SHALL be added to `sahil87/homebrew-tap` (out of scope for this repo's change but documented for traceability). After that lands, `Formula/all.rb` SHALL gain `depends_on "sahil87/tap/shll"` so `brew install sahil87/tap/all` includes shll.

#### Scenario: Traceability documentation
- **GIVEN** the README of this repo
- **WHEN** a user reads the install section
- **THEN** there is a clear note that `shll` is available at `sahil87/tap/shll` and via `sahil87/tap/all`

## Testing

### Requirement: Test-Alongside
Unit tests SHALL live alongside source files. Each command file (`update.go`, `shell_init.go`, `version.go`) SHALL have a paired `_test.go`. The `internal/proc` package SHALL have a `proc_test.go`.

#### Scenario: Test files present
- **GIVEN** the scaffolded source tree
- **WHEN** test files are enumerated
- **THEN** `cmd/shll/` contains `update_test.go`, `shell_init_test.go`, `version_test.go`
- **AND** `internal/proc/` contains `proc_test.go`

### Requirement: Subprocess Mocking
Tests SHALL NOT shell out to real `brew` or per-tool binaries. The `internal/proc` package SHALL be designed so command code can inject a fake runner (e.g., via a package-level `Runner` variable, an interface, or a function-typed field). Tests SHALL exercise command logic against a fake that records invocations and returns canned output.

#### Scenario: Fake runner in tests
- **GIVEN** an `update_test.go` test
- **WHEN** the test runs
- **THEN** no real `brew` subprocess is spawned
- **AND** the test asserts that `brew update --quiet` and the expected `brew upgrade ...` calls were recorded

#### Scenario: Test for graceful degradation
- **GIVEN** an `update_test.go` test simulating zero installed sahil87 tools
- **WHEN** the test runs
- **THEN** the recorded invocations include no `brew upgrade` calls
- **AND** stdout output is `No sahil87 tools installed.\n`

### Requirement: Tests Pass on Scaffold Completion
At the end of the apply stage, `go test ./...` (run from `src/`) SHALL exit zero.

#### Scenario: All tests pass
- **GIVEN** the completed scaffold
- **WHEN** `cd src && go test ./...` is run
- **THEN** all tests pass and the command exits zero

## Documentation

### Requirement: README
The repo SHALL contain a `README.md` that documents installation (via tap), the three subcommands with one-line descriptions, an example for each, and a link to the constitution. The README SHALL NOT duplicate per-tool CLI documentation — it links out for those.

#### Scenario: README contents
- **GIVEN** `README.md` at the repo root
- **WHEN** the file is inspected
- **THEN** it contains an Install section referencing `brew install sahil87/tap/shll`
- **AND** it contains a Commands section with a one-line summary for `update`, `shell-init`, `version`
- **AND** it contains an example for each subcommand (e.g., `eval "$(shll shell-init zsh)"` in an rc file)

### Requirement: License
The repo SHALL include an MIT `LICENSE` file at the root.

#### Scenario: License file present
- **GIVEN** the repo root
- **WHEN** files are listed
- **THEN** a `LICENSE` file exists
- **AND** it is the MIT license text with year `2026` and copyright holder `Sahil Ahuja` (or equivalent matching hop's holder)

## Design Decisions

1. **Subcommand justification (Constitution VII)**:
   - `update` — Solves a concrete pain (no single command to update the toolkit; `brew upgrade sahil87/tap/all` does NOT propagate to dependencies). Cannot be a flag on an existing tool because the entry point itself is what's missing.
   - `shell-init` — Solves the cold-start cost and dual-eval-line maintenance burden for users with multiple shell-integrating tools. Per-tool `shell-init` / `shell-setup` continue to work standalone (Constitution IV); shll just composes them.
   - `version` — Solves a triage pain. Cannot live on a per-tool CLI because the value is the cross-tool aggregation.

2. **Installed detection via `brew list`, not symlink resolution**:
   - *Why*: `brew list --formula --versions sahil87/tap/<formula>` is the right primitive for querying *other* tools' install status. Hop's `/Cellar/` symlink trick works for the running tool only.
   - *Rejected*: parsing plain `brew list` output (regex-fragile, see code-quality.md anti-pattern); inspecting filesystem paths directly (Constitution-violating hardcoded `/opt/homebrew` style paths).

3. **Sequential brew upgrades**:
   - *Why*: Brew serializes most internal operations behind its own lock; parallelism risks confusing interleaved output and lock contention with no measurable speedup.
   - *Rejected*: parallel goroutine-per-tool. Real brew operations are I/O-bound on the single brew lock, so concurrency would not help.

4. **Plain-text `shll version` output, no `--json`**:
   - *Why*: Primary use case is bug reports — pasting output into a Slack thread or GitHub issue. Plain text is universally legible.
   - *Rejected*: `--json` flag for v0.1.0. Add later if a real script-consumer emerges; YAGNI for now.

5. **Per-tool `--version` invocations have a 2-second timeout**:
   - *Why*: Protects against deadlocked sub-tools. 2s is generous for `--version` (typical < 100ms) but bounded enough that worst-case `shll version` finishes in under 15 seconds even if every roster tool hangs.
   - *Rejected*: no timeout (one bad tool blocks the whole command); 500ms (too aggressive — some tools may legitimately take longer on a cold start, especially on macOS first-run gatekeeper checks).

6. **`shll shell-init` exits non-zero on sub-tool failure but keeps stdout eval-safe**:
   - *Why*: Eval-safety is non-negotiable (Constitution V) — a broken sub-tool MUST NOT corrupt the user's shell. A non-zero exit code surfaces the failure to scripts that check it; stderr provides a human-readable note.
   - *Rejected*: silent success on sub-tool failure (hides real problems); writing the error to stdout (breaks eval-safety).

7. **Subprocess injection via package-level `Runner` variable**:
   - *Why*: Simplest, most-Go-idiomatic test seam — matches hop's pattern.
   - *Rejected*: a full DI container (overkill); interface-typed parameters threaded through every call site (verbose).

8. **No `--check` / `--dry-run` for `shll update`**:
   - *Why*: Out of scope for v0.1.0 scaffold. `brew upgrade --dry-run` exists; users can compose it with the brace-expansion form for now.
   - *Rejected*: shipping `--check` in v0.1.0 (scope creep — not the pain we're solving).

9. **`shll update` skips `brew update --quiet` when no roster tools are installed**:
   - *Why*: The metadata refresh is only useful as a precursor to upgrades. When there is nothing to upgrade, the refresh is pure latency for no benefit; the user-visible message (`No sahil87 tools installed.`) is the primary signal and should print quickly.
   - *Rejected*: refreshing brew metadata anyway. Considered for "freshness on every invocation" but rejected — `shll update` is not a brew metadata refresh tool, it's a sahil87 toolkit upgrader. Users who want a refresh have `brew update` directly.

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Repo lives at `sahil87/shll`, mirroring hop's structure | Confirmed from intake #1; user-stated in `/fab-new` prompt | S:95 R:80 A:95 D:90 |
| 2 | Certain | Three subcommands: `update`, `shell-init`, `version` (no others in v0.1.0) | Confirmed from intake #2; Constitution VII justification recorded in Design Decisions | S:95 R:75 A:95 D:90 |
| 3 | Certain | Tool roster is hardcoded in `cmd/shll/tools.go` (or `root.go`) | Confirmed from intake #3; locked in Constitution III | S:95 R:70 A:95 D:90 |
| 4 | Certain | Composition not absorption — shll shells out to per-tool CLIs | Confirmed from intake #4; locked in Constitution III/IV | S:95 R:75 A:95 D:95 |
| 5 | Certain | Graceful degradation when sub-tools missing (no warnings, no errors) | Confirmed from intake #5; locked in Constitution V | S:95 R:80 A:95 D:90 |
| 6 | Certain | Go ≥1.22 + cobra; `internal/proc` for subprocess execution | Confirmed from intake #6; Constitution I and code-quality.md mandate | S:95 R:65 A:95 D:90 |
| 7 | Certain | `shll update` skips uninstalled tools silently | Confirmed from intake #7; locked in Constitution V | S:95 R:80 A:90 D:85 |
| 8 | Certain | `shll shell-init` supports zsh and bash only | Confirmed from intake #8; matches hop's supported shells | S:90 R:75 A:90 D:85 |
| 9 | Certain | `shll version` is plain text, no JSON in v0.1.0 | Upgraded from intake Confident #9 — Design Decision #4 documents the rejected JSON option, locking the v0.1.0 contract | S:90 R:85 A:85 D:85 |
| 10 | Certain | Per-tool `update` commands are NOT deprecated | Confirmed from intake #10; Constitution IV mandate; recorded as a Non-Goal | S:90 R:70 A:90 D:85 |
| 11 | Certain | `shll` formula is a peer in the tap; `all` gains `depends_on "shll"` | Confirmed from intake #11; user-stated in `/fab-new` prompt | S:95 R:70 A:90 D:90 |
| 12 | Certain | Installed detection uses `brew list --formula --versions sahil87/tap/<formula>` | Upgraded from intake Confident #12 — Design Decision #2 documents the alternative explicitly; code-quality.md anti-pattern blocks regex over plain `brew list` | S:90 R:80 A:90 D:80 |
| 13 | Certain | `shll version` per-tool invocation timeout is 2 seconds (named constant) | Upgraded from intake Confident #13 — Design Decision #5 documents the rationale and rejected alternatives | S:85 R:85 A:85 D:80 |
| 14 | Certain | shll's own version is injected via `-ldflags` at build time, with `dev` default when unset | Upgraded from intake Confident #14 — hop's pattern, codified in Constitution VI | S:90 R:80 A:90 D:80 |
| 15 | Certain | `shll update` runs upgrades sequentially, not in parallel | Upgraded from intake Confident #15 — Design Decision #3 documents the rejected parallel option | S:85 R:75 A:85 D:80 |
| 16 | Certain | `shll update` exits non-zero if any per-tool upgrade fails, but continues through the roster | Spec-level analysis: matches "best effort with accurate exit code" pattern; Constitution V says skip *missing* tools, not silently swallow real failures | S:90 R:80 A:85 D:80 |
| 17 | Certain | `shll shell-init` exits non-zero on sub-tool failure; stdout remains eval-safe | New from spec analysis — Design Decision #6 documents; Constitution V (eval-safety) requires this | S:90 R:80 A:90 D:85 |
| 18 | Confident | Subprocess injection for tests via package-level `Runner` variable in `internal/proc` | New from spec analysis — Design Decision #7; matches hop's pattern; alternatives (DI container, interface threading) rejected as heavier | S:75 R:80 A:80 D:75 |
| 19 | Confident | Test files live alongside source (`update_test.go` next to `update.go`) | Confirmed from `code-quality.md` Test Strategy: `test-alongside` | S:90 R:85 A:90 D:90 |
| 20 | Confident | LICENSE is MIT, copyright `Sahil Ahuja`, year 2026 | Intake states MIT; year is current; copyright matches the user's name. Easily reversed by editing one file | S:75 R:90 A:80 D:80 |
| 21 | Certain | `shll update` skips `brew update --quiet` when no roster tools are installed; runs it once before upgrades when at least one tool is installed | Implementation choice — Design Decision #9 documents the rejected "refresh-anyway" alternative. Spec scenario explicitly disallows the refresh in the zero-installed case. Upgraded from intake Confident #21 (which had it inverted) after spec/code reconciliation during review | S:90 R:80 A:85 D:80 |
| 22 | Confident | The `release.yml` workflow opens/updates a PR against `sahil87/homebrew-tap` rather than direct-pushing | PR-based workflow matches hop's pattern; safer than direct push; gives a review checkpoint. Easily reversed if a frictionless flow is preferred later | S:75 R:80 A:80 D:75 |
| 23 | Confident | Existing `fab/` and `docs/` directories are untouched by the scaffold | Spec-level analysis: scaffold creates new files, does not modify the workflow/docs structure that is already initialized | S:90 R:90 A:95 D:95 |

23 assumptions (18 certain, 5 confident, 0 tentative, 0 unresolved).
