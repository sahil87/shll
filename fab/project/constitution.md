# shll Constitution

## Core Principles

### I. Security First
All process execution MUST use `exec.CommandContext` with explicit argument slices — never shell strings or `exec.Command` without a context/timeout. Subprocess invocation SHALL route through an `internal/proc` package (mirroring hop) rather than calling `os/exec` directly from command code. Shell injection is a show-stopper. This is non-negotiable.

### II. No Database, No State
shll is stateless. Every invocation re-derives information at request time — installed versions from `brew list`, latest versions from `brew info`, shell-init output from each sub-tool's own `shell-init`/`shell-setup` command. shll SHALL NOT cache, persist, or otherwise carry state between invocations.

### III. Wrap, Don't Reinvent
- For Homebrew operations, shell out to `brew` — do not parse formula files or query the Homebrew API directly.
- For shell-init composition, invoke each sub-tool's existing `shell-init` / `shell-setup` subcommand and concatenate the output. shll SHALL NOT duplicate or rewrite per-tool shell logic.
- For version reporting, invoke `<tool> --version` (or equivalent) per sub-tool. shll SHALL NOT hardcode versions or read binaries directly.

### IV. Composition, Not Replacement
shll exists alongside per-tool CLIs, not as a replacement. Per-tool commands (`hop update`, `wt shell-setup`, etc.) MUST continue to work standalone. shll's job is to compose them — calling them as subprocesses, not absorbing their logic.

### V. Graceful Degradation
When a sub-tool is not installed, shll MUST skip it without erroring. `shll update` skips uninstalled formulas. `shll shell-init` omits init for missing tools. `shll version` reports `not installed`. The output of `shll shell-init` MUST always be eval-safe regardless of which sub-tools are present.

### VI. Thin Justfile, Fab-Kit Build Pattern
The build system MUST mirror fab-kit's structure: `justfile` recipes are one-liners that delegate to `scripts/`. Logic, loops, and conditionals belong in shell scripts — the justfile is an index, not an implementation. Releases SHALL be cut by tagging `v*`, with a GitHub Actions workflow that builds cross-platform binaries, publishes a GitHub Release, and updates `homebrew-tap`. Local development uses `just build` and `just install` to populate a local cache.

### VII. Minimal Surface Area
shll is a meta-tool, not a toolkit unto itself. New top-level subcommands require explicit justification in the change's intake — "could this be a flag on an existing subcommand, or does this belong in a per-tool CLI instead?" must be answered before adding one. The current scope is `update`, `shell-init`, `version` — additions raise the bar.

## Additional Constraints

### Test Integrity
Tests MUST conform to the implementation spec — never the other way around. When tests fail, the fix SHALL either (a) update the tests to match the spec, or (b) update the implementation to match the spec. Modifying implementation code solely to accommodate test fixtures or test infrastructure is prohibited. Specs are the source of truth; tests verify conformance to specs.

### Cross-Platform Behavior
Platform-specific code MUST be isolated behind a small abstraction. The binary SHALL build and run on darwin-arm64, darwin-amd64, linux-arm64, and linux-amd64. Windows is not supported.

### Tool Roster Source of Truth
The list of sub-tools shll knows about is hardcoded in shll's source. Adding a new tool to the sahil87 toolkit requires a shll release. shll SHALL NOT discover tools dynamically from `brew tap` or any other runtime source — the explicit, versioned list is the contract.

## Governance

**Version**: 1.0.0 | **Ratified**: 2026-05-09 | **Last Amended**: 2026-05-09
