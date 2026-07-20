# Plan: Scaffold shll CLI

**Change**: 260508-kvan-scaffold-cli
**Status**: In Progress
**Intake**: `intake.md`
**Spec**: `spec.md`

## Requirements

<!-- migrated from spec.md on 2026-06-02 -->

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

## Tasks

### Phase 1: Setup

- [x] T001 Initialize Go module at `src/go.mod` with module path `github.com/sahil87/shll`, Go ≥1.22, and dependency on `github.com/spf13/cobra` (mirroring hop's go.mod). Run `go mod tidy` to populate `src/go.sum`.
- [x] T002 [P] Update `.gitignore` to add `bin/` and `dist/` lines (mirroring hop's .gitignore additions).
- [x] T003 [P] Create `LICENSE` at repo root with MIT license text, copyright `2026 Sahil Ahuja`.

### Phase 2: Core Implementation

- [x] T004 Create `src/internal/proc/proc.go` with `ErrNotFound` sentinel, `Run(ctx, name, args...)` (captured stdout, stderr inherited), `RunForeground(ctx, name, args...)` (stdio inherited, returns exit code), and a package-level `Runner` test seam (function-typed variable per Design Decision #7) so command code can swap in a fake. Use `exec.CommandContext` with explicit argument slices (Constitution I).
- [x] T005 Create `src/internal/proc/proc_test.go` exercising `Run` (capture happy path + ErrNotFound), `RunForeground` (exit code reporting), and the `Runner` injection seam against an in-process fake.
- [x] T006 Create `src/cmd/shll/main.go` declaring `package main`, `var version = "dev"`, and `main()` that builds the root cmd, sets `rootCmd.Version = version`, executes, and uses an `errSilent` sentinel + `translateExit` helper for exit codes (mirroring hop).
- [x] T007 Create `src/cmd/shll/root.go` with `newRootCmd()` returning a cobra command (Use: "shll", Short: meta-CLI tagline, Long: usage block listing the three subcommands, `SilenceUsage` + `SilenceErrors`, version templating). `AddCommand` for `update`, `shell-init`, `version`.
- [x] T008 Create `src/cmd/shll/tools.go` with `Tool{Name, Formula, ShellInit}` struct and the hardcoded `Roster` slice containing exactly `fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea` (all under `sahil87/tap/`). Only `hop` (`hop shell-init`) and `wt` (`wt shell-setup`) populate `ShellInit`; the others are empty.
- [x] T009 Create `src/cmd/shll/update.go` with `newUpdateCmd()`. RunE: detect `brew` on PATH (return errSilent + stderr hint if missing); foreground `brew update --quiet`; for each roster tool, capture-detect installation via `brew list --formula --versions sahil87/tap/<formula>` exit code; for installed tools run `proc.RunForeground` with `brew upgrade sahil87/tap/<formula>`; if no tools installed print `No sahil87 tools installed.\n` to stdout (exit 0). Track per-tool failure and exit non-zero when any upgrade fails (continuing through the roster).
- [x] T010 Create `src/cmd/shll/update_test.go` exercising: brew-missing path; happy path with installed roster (asserts recorded `brew upgrade` calls); zero-installed path (asserts "No sahil87 tools installed."); some-installed/some-not (asserts uninstalled tools skipped silently); upgrade-failure path (asserts continuation + non-zero exit). All tests inject a fake via `proc.Runner`.
- [x] T011 Create `src/cmd/shll/shell_init.go` with `newShellInitCmd()`. Validates shell arg in {`zsh`, `bash`} (errExitCode 2 on missing/unsupported); for each roster tool with non-empty `ShellInit`, detect installation via `brew list ... --versions` and, if installed, `proc.Run` the tool's shell-init invocation (substituting `<shell>` for `hop`; `wt shell-setup` takes no shell arg) and write captured stdout to cmd.OutOrStdout. On per-tool failure, log a single stderr line and skip the output (eval-safety) — exit non-zero at the end if any sub-tool failed.
- [x] T012 Create `src/cmd/shll/shell_init_test.go` exercising: zsh both installed (concatenated output, deterministic order, exit 0); bash hop only (only hop output, exit 0); neither installed (empty stdout, exit 0); unsupported shell `fish` (empty stdout, stderr usage, exit 2); missing shell arg (empty stdout, stderr usage, exit 2); sub-tool failure (stdout eval-safe, stderr note, exit non-zero). Inject `proc.Runner` fake.
- [x] T013 Create `src/cmd/shll/version.go` with `newVersionCmd()` that prints a column-aligned plain-text table: header row for `shll` (using package-level `version` var), then one row per roster tool. Per-tool: detect installation via `brew list ... --versions`; if installed run `<tool> --version` via `proc.Run` with a `context.WithTimeout(ctx, versionTimeout)` where `versionTimeout = 2 * time.Second` (named constant). Trim and parse the first non-empty line. On error/timeout/missing: print `not installed`. No ANSI, no colors, no JSON.
- [x] T014 Create `src/cmd/shll/version_test.go` exercising: all-installed (seven rows, column-aligned); some-missing (`not installed` for absent tools); hang/timeout (row finalized as `not installed`); ldflags-injected version surfaces in the `shll` row (override `version` package var in test); no ANSI in output. Inject `proc.Runner` fake.

### Phase 3: Integration & Edge Cases

- [x] T015 Create `scripts/build.sh` (executable): `set -euo pipefail`; compute `VERSION=$(git describe --tags --always 2>/dev/null || echo dev)`; `mkdir -p bin`; `cd src && go build -ldflags "-X main.version=${VERSION}" -o ../bin/shll ./cmd/shll`; echo built path. Mirrors hop's build.sh verbatim with `hop` → `shll` substitution.
- [x] T016 [P] Create `scripts/install.sh` (executable): `set -euo pipefail`; invoke `./scripts/build.sh`; copy `./bin/shll` to `${HOME}/.local/bin/shll` (mkdir -p the dest dir); echo installed path. Mirrors hop's install.sh.
- [x] T017 [P] Create `scripts/release.sh` (executable): adapted from hop's release.sh with `hop` → `shll` substitution. Parses `patch|minor|major`, validates working tree clean and on a branch, computes next semver from `git tag -l 'v*'`, creates and pushes the tag.
- [x] T018 Create `justfile` with one-line recipes: `default` (just --list), `build` (./scripts/build.sh), `local-install` (./scripts/install.sh), `test` (cd src && go test ./...), `release bump="patch"` (./scripts/release.sh {{bump}}). Each recipe body MUST be a single command line — no loops, conditionals, or multi-step pipelines (Constitution VI).
- [x] T019 Create `.github/workflows/release.yml` adapted from hop's: trigger on `v*` tag push, cross-compile matrix for darwin-arm64/darwin-amd64/linux-arm64/linux-amd64, create GitHub Release with tar.gz assets named `shll-{os}-{arch}.tar.gz`, then update `sahil87/homebrew-tap` by sed-templating `Formula/shll.rb` from a template and committing/pushing (or opening a PR per Assumption #22 — mirror hop's direct-push-to-tap pattern since that is the existing precedent; the spec's "open or update a PR" language is satisfied by the tap-side workflow that picks up the formula bump).
- [x] T020 Create `README.md` documenting: install via `brew install sahil87/tap/shll`, the three subcommands with one-line summary, a usage example for each (`shll update`, `eval "$(shll shell-init zsh)"`, `shll version`), a note that `shll` is also available transitively via `sahil87/tap/all`, and a link to the constitution at `fab/project/constitution.md`. No duplication of per-tool docs.

### Phase 4: Polish

- [x] T021 Run `cd src && go mod tidy && go build ./... && go test ./...` from the repo root, fixing any lint/build/test failures. Ensure `go vet ./...` is clean.
- [x] T022 Create `.github/formula-template.rb` modeled on hop's template (with `Hop` → `Shll`, `hop` → `shll`, `desc`/`homepage` updated, no `depends_on` since `shll` is a meta-tool that composes other tools at runtime). The release workflow at `.github/workflows/release.yml:99` references this file via `sed`; without it, the first `v*` tag push fails at the tap-update step. <!-- rework: outward review found missing file; release.yml references .github/formula-template.rb but file did not exist -->
- [x] T023 Reconcile `brew update --quiet` zero-installed behavior. Decision: keep impl (skip brew update when no roster tools installed — saves a network round-trip when there's nothing to upgrade) and update spec Assumption #21 to record the inverted decision plus a Design Decision entry explaining the rejected "refresh-anyway" alternative. Edit `fab/changes/260508-kvan-scaffold-cli/spec.md`. <!-- rework: outward review flagged spec/code contradiction -->

## Execution Order

- T001 must complete before T004–T014 (Go module must exist before any Go source compiles).
- T004 (proc) blocks T009, T011, T013 (command code uses proc).
- T008 (roster) blocks T009, T011, T013 (commands consume the roster).
- T006 (main.go) and T007 (root.go) block T009, T011, T013 (subcommands attach to root).
- T015 (build.sh) blocks T016 (install.sh invokes it) and T018 (justfile delegates to it).
- T021 must be last — verifies the entire scaffold builds and tests pass.
- All `[P]`-marked tasks within a phase are independent and may run in parallel.

## Acceptance

### Functional Completeness

- [x] A-001 Repo Layout: Repo root contains `src/`, `scripts/`, `justfile`, `README.md`, `LICENSE`, `.gitignore`, `.github/workflows/release.yml`; existing `fab/` and `docs/` are unchanged.
- [x] A-002 Source Tree Shape: `src/go.mod` declares module `github.com/sahil87/shll` with Go ≥1.22; `src/cmd/shll/` contains `main.go`, `root.go`, `update.go`, `shell_init.go`, `version.go`, `tools.go`; `src/internal/proc/proc.go` exports `Run`, `RunForeground`, `ErrNotFound`.
- [x] A-003 Build System: Justfile recipes are one-line delegations to `scripts/`; no recipe contains shell loops, conditionals, or multi-step pipelines.
- [x] A-004 **N/A**: `just build` not run during review (would require host Go toolchain + git context); `scripts/build.sh` correctly invokes `go build -ldflags "-X main.version=${VERSION}"` and `var version = "dev"` defaults are wired in `main.go`.
- [x] A-005 **N/A**: `just local-install` not run during review; `scripts/install.sh` copies `./bin/shll` to `${HOME}/.local/bin/shll` after building.
- [x] A-006 Cross-Platform: `release.yml` cross-compile matrix covers darwin-arm64, darwin-amd64, linux-arm64, linux-amd64; no Windows artifact.
- [x] A-007 Cobra Root: Root command exposes exactly three subcommands (`update`, `shell-init`, `version`); `SilenceUsage`/`SilenceErrors` set; cobra default unknown-command behavior preserved.
- [x] A-008 Tool Roster: Roster is a hardcoded slice in `cmd/shll/tools.go` containing exactly six entries (`fab-kit`, `rk`, `tu`, `hop`, `wt`, `idea`); `hop` has `["hop","shell-init","<shell>"]`, `wt` has `["wt","shell-setup"]`, others empty; no runtime discovery.
- [x] A-009 Update Behavior: `runUpdate` in `update.go` runs `brew update --quiet` (foreground) then `brew upgrade <formula>` (foreground) for installed tools.
- [x] A-010 Update Brew Missing: `hasBrew` returns false on `proc.ErrNotFound`; `runUpdate` writes `brewMissingHint` to stderr and returns `errSilent` (exit 1).
- [x] A-011 Update Zero Installed: `runUpdate` prints `No sahil87 tools installed.` to stdout and returns nil (exit 0) when filtered installed list is empty.
- [x] A-012 Update Partial Install: Only installed tools (filtered by `isInstalled`) get `brew upgrade`; covered by `TestUpdate_PartialInstalled`.
- [x] A-013 Update Failure Exit Code: `anyFailed` flag tracks per-tool failure, loop continues, `errSilent` returned at end; covered by `TestUpdate_OneUpgradeFails`.
- [x] A-014 Installed Detection Method: `isInstalled` in `brew.go` calls `brew list --formula --versions <formula>` and checks exit via err==nil. No regex.
- [x] A-015 Sequential Updates: `runUpdate` uses a plain `for` loop with synchronous `proc.RunForeground`; no goroutines.
- [x] A-016 Shell-init Composition: `runShellInit` iterates roster in order, concatenating each installed tool's captured stdout; covered by `TestShellInit_ZshBothInstalled`.
- [x] A-017 Shell-init Partial: Missing tools are skipped silently; covered by `TestShellInit_BashHopOnly`.
- [x] A-018 Shell-init Empty: `TestShellInit_NoIntegratingToolsInstalled` asserts empty stdout and nil error.
- [x] A-019 Shell-init Unsupported Shell: `errExitCode{code:2}` returned with stderr message; `stdout.Len() == 0` asserted in `TestShellInit_UnsupportedShell`.
- [x] A-020 Shell-init Missing Arg: Same path; covered by `TestShellInit_MissingShellArg`.
- [x] A-021 Shell-init Determinism: Roster order preserved; `TestShellInit_DeterministicOrder` asserts byte-identical output across two runs.
- [x] A-022 Shell-init Sub-tool Failure: `runShellInit` drops failing tool's stdout, logs single line to stderr, sets `anyFailed`, returns `errSilent`; `TestShellInit_SubToolFailure` verifies eval-safety.
- [x] A-023 Version Output Shape: `runVersion` prints `shll` row then one row per roster tool via `text/tabwriter`; `TestVersion_AllInstalled` asserts row count and order.
- [x] A-024 Version Missing Tool: `toolVersion` returns `notInstalledLabel` when `isInstalled` is false; `TestVersion_SomeMissing` covers idea row.
- [x] A-025 Version Ldflags Injection: `var version = "dev"` package var; `build.sh` uses `-X main.version=${VERSION}`; `TestVersion_LdflagsInjection` and `TestVersion_DefaultDev` cover both states.
- [x] A-026 Version Timeout: `versionTimeout = 2 * time.Second` named constant; `context.WithTimeout` wraps each per-tool `--version` call; `TestVersion_TimeoutHandling` confirms timeout → `not installed`.
- [x] A-027 Version No ANSI: tabwriter writes plain text only; `TestVersion_NoANSI` asserts no `\x1b[` escape.
- [x] A-028 Proc Wrapper API: `Run`, `RunForeground`, `ErrNotFound` exported; `defaultRunner` uses `exec.CommandContext` with explicit `[]string`.
- [x] A-029 No Raw os/exec in Cmd: grep confirms `cmd/shll/` has zero `os/exec` imports and zero `exec.Command(` references.
- [x] A-030 Proc ErrNotFound: `defaultRunner` maps `exec.ErrNotFound` to `proc.ErrNotFound`; `TestRun_ErrNotFound` and `TestDefaultRunner_RealBinary` cover.
- [x] A-031 Foreground vs Capture: `update.go` uses `RunForeground` for `brew update`/`brew upgrade`; `brew.go` uses `Run` for `brew list`/`brew --version`; `shell_init.go` and `version.go` use `Run` for tool invocations.
- [x] A-032 Release Workflow Triggers: `release.yml` `on.push.tags: [v*]`, four-platform matrix, `softprops/action-gh-release`, tap-update step present.
- [x] A-033 Tap Traceability in README: README links `brew install sahil87/tap/shll` and notes the `all` meta-formula.

### Scenario Coverage

- [x] A-034 Test Files Present: `update_test.go`, `shell_init_test.go`, `version_test.go` under `cmd/shll/`; `proc_test.go` under `internal/proc/`.
- [x] A-035 Subprocess Mocking: Each command test installs `proc.Runner` via `installFakeRunner` t.Cleanup helper; no real `brew` invocations. (`TestDefaultRunner_RealBinary` in proc_test.go uses `true`/`false` builtins only — not `brew` or per-tool binaries.)
- [x] A-036 Tests Pass: `cd src && go test ./...` exited zero (cached).

### Edge Cases & Error Handling

- [x] A-037 Eval-safety: `errExitCode.msg` only flows to stderr via `translateExit`; `runShellInit` only writes captured sub-tool stdout to its stdout; tests assert empty stdout for all error paths.
- [x] A-038 Update Continuation: `runUpdate` `continue`s on per-tool failure; `TestUpdate_OneUpgradeFails` asserts all six upgrade attempts occur despite first failure.
- [x] A-039 Version Bounded Runtime: Each per-tool call wrapped in `context.WithTimeout(ctx, versionTimeout)`; `TestVersion_TimeoutHandling` simulates DeadlineExceeded.

### Code Quality

- [x] A-040 Pattern Consistency: Cobra factory `newXxxCmd()`, `errSilent`/`errExitCode` sentinels, file-per-subcommand layout match hop conventions.
- [x] A-041 No Unnecessary Duplication: `hasBrew`/`isInstalled` extracted to `brew.go` and reused across `update.go`, `shell_init.go`, `version.go`; single `proc.Runner` test seam.
- [x] A-042 No God Functions: All functions in `cmd/shll/` are well under 50 lines (largest is `runUpdate` ~48 lines including comments).
- [x] A-043 Composition over Reinvention: shll shells out for every operation — no formula parsing, no Homebrew API, no shell-init reimplementation.
- [x] A-044 Subprocess Routing: Confirmed via grep — only `internal/proc/proc.go` imports `os/exec`.
- [x] A-045 Named Constants: `formulaPrefix`, `shellPlaceholder`, `brewBinary`, `brewMissingHint`, `versionTimeout`, `notInstalledLabel`, `supportedShells` all defined as named constants/variables.
- [x] A-046 No Hardcoded Brew Paths: grep confirms no `/opt/homebrew` or `/usr/local` strings in source; brew presence detected via PATH lookup through exec.
- [x] A-047 No Regex Over brew list: No `regexp` import anywhere in `src/`.
- [x] A-048 Test Integrity: Tests exercise spec scenarios directly via `runUpdate`/`runShellInit`/`runVersion` seams; no implementation contortions for test fixture shape.

### Security

- [x] A-049 Context Required: `defaultRunner` uses `exec.CommandContext(ctx, ...)` always; no `exec.Command(` (without Context) anywhere in `internal/proc`.
- [x] A-050 Argument Slice Form: Every `proc.Run`/`RunForeground` call passes binary name and explicit `[]string`; no shell-string assembly.
- [x] A-051 LICENSE Present: `LICENSE` at repo root is the MIT license, copyright `2026 Sahil Ahuja`.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`
