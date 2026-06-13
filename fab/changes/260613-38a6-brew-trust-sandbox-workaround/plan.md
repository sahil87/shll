# Plan: Linux brew-trust sandbox workaround (HOMEBREW_NO_REQUIRE_TAP_TRUST)

**Change**: 260613-38a6-brew-trust-sandbox-workaround
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

<!-- Derived from intake.md. The intake records a fully-specified design with 12
     graded assumptions; requirements below honor those decisions verbatim and do
     not re-open them. RFC 2119 keywords; every requirement has at least one
     GIVEN/WHEN/THEN scenario. -->

### internal/proc: Per-request environment plumbing

#### R1: `proc.Request` carries optional extra environment entries
`proc.Request` SHALL gain an `Env []string` field holding extra `KEY=VALUE` entries to be **appended** to (not replace) the child process's inherited environment. A nil/empty `Env` MUST preserve today's behavior (full parent-env inheritance, `cmd.Env` left nil).

- **GIVEN** a `Request` with `Env: nil` (or empty)
- **WHEN** `defaultRunner` builds the `exec.Cmd`
- **THEN** `cmd.Env` is left unset, so the child inherits the full parent environment exactly as before this change

#### R2: `defaultRunner` appends `Env` onto the parent environment when non-empty
`defaultRunner` SHALL set `cmd.Env = append(os.Environ(), req.Env...)` **only when** `len(req.Env) > 0`. Appended entries MUST win on duplicate keys (Go's `exec` uses the last value for a duplicated key).

- **GIVEN** a `Request` with `Env: ["HOMEBREW_NO_REQUIRE_TAP_TRUST=1"]`
- **WHEN** `defaultRunner` builds the `exec.Cmd`
- **THEN** `cmd.Env` equals the parent environment (`os.Environ()`) with `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` appended at the end
- **AND** an inherited value for the same key is overridden by the appended entry

#### R3: A foreground env-carrying helper variant exists
The proc package SHALL expose `RunForegroundEnv(ctx context.Context, env []string, name string, args ...string) (int, error)` that builds a `Request` with `Transport: TransportForeground` and `Env: env`. The existing `Run` and `RunForeground` signatures MUST remain unchanged (they pass `Env: nil`). No capture-mode `RunEnv` is added (deferred per intake assumption #11 — YAGNI).

- **GIVEN** a caller needs to inject env into a foreground brew invocation
- **WHEN** it calls `RunForegroundEnv(ctx, env, name, args...)`
- **THEN** the resulting `Request` carries `Env: env` and `Transport: TransportForeground`
- **AND** `RunForegroundEnv` returns `(-1, err)` on a transport error and `(code, nil)` on completion, mirroring `RunForeground`

### cli/brew: Single source of truth for the workaround env

#### R4: `brewEnv()` centralizes the Linux-gated override
`brew.go` SHALL define a `brewEnv() []string` helper next to `brewBinary` that returns `nil` unless `osGoos == "linux"`, in which case it returns `[]string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}`. The Linux gate MUST reuse the existing `var osGoos = runtime.GOOS` seam in `shell_setup.go` (reuse-in-place per intake assumption #12 — no hoist, no build constraints). A LOUD temporary-workaround comment MUST cross-reference backlog `[38a6]` (this change) and `[tkch]` (removal follow-up).

- **GIVEN** `osGoos == "linux"`
- **WHEN** `brewEnv()` is called
- **THEN** it returns `["HOMEBREW_NO_REQUIRE_TAP_TRUST=1"]`
- **AND** **GIVEN** `osGoos == "darwin"`, **WHEN** `brewEnv()` is called, **THEN** it returns `nil`

### cli/install & cli/update: Apply the env at live brew call sites

#### R5: Live foreground brew install/upgrade/update calls carry `brewEnv()`
The following live foreground brew calls SHALL pass `brewEnv()` via `proc.RunForegroundEnv`: `install.go` `brew install <formula>`; `update.go` `brew update --quiet`; `update.go` `brew upgrade <shllFormula>` (shll self-upgrade); and the `upgradeTool` `brew upgrade <formula>` fallback.

- **GIVEN** `osGoos == "linux"`
- **WHEN** `shll install` runs `brew install <formula>` for a missing tool
- **THEN** the recorded `Request` carries `Env` containing `HOMEBREW_NO_REQUIRE_TAP_TRUST=1`
- **AND** the same holds for `brew update --quiet`, `brew upgrade <shllFormula>`, and the `brew upgrade <formula>` fallback
- **AND** **GIVEN** `osGoos == "darwin"`, the same `Request`s carry **no** such env

#### R6: `upgradeTool` keys the env on whether the argv is a brew call
The env injection decision SHALL live in `upgradeTool` (the live runner), gated on `argv[0] == brewBinary`. Per-tool `<tool> update` delegations MUST NOT receive the env. `upgradeArgv` and `argvString` (the dry-run preview source) MUST remain env-free — env is a runtime concern of `upgradeTool` only.

- **GIVEN** a roster tool with a non-empty `Update` argv (so `argv[0] != brewBinary`)
- **WHEN** `upgradeTool` upgrades it via `<tool> update`
- **THEN** the recorded `Request` carries **no** env, on any OS
- **AND** **GIVEN** a tool with an empty `Update` argv (fallback to `brew upgrade <formula>`, `argv[0] == brewBinary`) on linux, **THEN** the recorded `Request` carries `HOMEBREW_NO_REQUIRE_TAP_TRUST=1`

#### R7: Capture/query brew calls and non-brew calls stay env-free
Brew capture/query calls (`brew --version` in `hasBrew`, `brew list` in `isInstalled`, `brew info`, `brew trust`/`brew trust --help`) and all non-brew subprocesses (per-tool `update` delegations, `<tool> shell-init`, `<tool> --version`) MUST NOT carry the workaround env. The dry-run preview MUST stay byte-for-byte unchanged.

- **GIVEN** any OS
- **WHEN** `shll update`/`shll install` issue read-only probes (`brew list`, `brew --version`, `<tool> update --help`) or per-tool `<tool> update` delegations
- **THEN** none of those recorded `Request`s carry the workaround env

### Design Decisions

1. **Env-carrying variant, not an options refactor** (intake assumption #7): add `RunForegroundEnv` rather than changing `Run`/`RunForeground` to variadic options — *Why*: smallest blast radius, leaves every existing call site untouched — *Rejected*: variadic-options refactor (broader churn for no current benefit).
2. **Inject in `upgradeTool` keyed on `argv[0] == brewBinary`** (intake assumption #8) — *Why*: `upgradeArgv` is shared with the dry-run preview and builds both brew and `<tool> update` argv; env must not leak into the displayed argv or onto per-tool delegations — *Rejected*: injecting in `upgradeArgv` (would taint the preview and delegations).
3. **Linux-only gate via existing `osGoos` seam, reused in place** (intake assumptions #2, #12) — *Why*: bug is bubblewrap-specific; `osGoos` already exists as a testable seam; reuse-in-place is the smaller diff — *Rejected*: `//go:build` constraints (untestable from one host), hoisting `osGoos` to a new file (unnecessary churn).
4. **`HOMEBREW_NO_REQUIRE_TAP_TRUST=1`, not `HOMEBREW_NO_SANDBOX`** (intake assumption #1) — *Why*: the verified minimal fix keeps the sandbox active and skips only the broken in-sandbox trust re-check — *Rejected*: disabling the sandbox (over-broad, removes a real security boundary).

### Non-Goals

- No capture-mode `RunEnv` helper (deferred per intake assumption #11 — no live capture-brew failure path).
- No change to the displayed dry-run preview argv (env is runtime-only).
- No new CLI flags, no changed output, no new subcommand (Constitution VII unaffected).

## Tasks

### Phase 1: proc plumbing

- [x] T001 Add `Env []string` field to `proc.Request` in `src/internal/proc/proc.go` with a doc comment ("extra KEY=VALUE entries appended to the parent env; nil = inherit only"). <!-- R1 -->
- [x] T002 In `defaultRunner` (`src/internal/proc/proc.go`), set `cmd.Env = append(os.Environ(), req.Env...)` only when `len(req.Env) > 0`, before the transport switch (`os` already imported). <!-- R2 -->
- [x] T003 Add `RunForegroundEnv(ctx, env []string, name string, args ...string) (int, error)` to `src/internal/proc/proc.go`, building a `Request{Name, Args, Transport: TransportForeground, Env: env}`; keep `Run`/`RunForeground` passing `Env: nil` (unchanged signatures). <!-- R3 -->

### Phase 2: brewEnv source of truth

- [x] T004 Add `brewEnv() []string` to `src/cmd/shll/brew.go` next to `brewBinary`, returning `nil` unless `osGoos == "linux"` else `[]string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}`, with a LOUD temporary-workaround comment cross-referencing backlog `[38a6]` and `[tkch]` (env var name extracted as a named const per code-quality.md). <!-- R4 -->

### Phase 3: apply brewEnv at live call sites

- [x] T005 In `src/cmd/shll/install.go`, switch the `brew install <formula>` call (line ~154) from `proc.RunForeground` to `proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "install", t.Formula)`. <!-- R5 -->
- [x] T006 In `src/cmd/shll/update.go` `runUpdate`, switch `brew update --quiet` (line ~242) and `brew upgrade <shllFormula>` (line ~295) from `proc.RunForeground` to `proc.RunForegroundEnv(ctx, brewEnv(), ...)`. <!-- R5 -->
- [x] T007 In `src/cmd/shll/update.go` `upgradeTool` (line ~402), inject `brewEnv()` only when `argv[0] == brewBinary`, calling `proc.RunForegroundEnv(ctx, env, argv[0], argv[1:]...)`; leave `upgradeArgv`/`argvString` untouched. <!-- R6 R7 -->

### Phase 4: tests

- [x] T008 [P] In `src/internal/proc/proc_test.go`: assert non-empty `Env` yields `cmd.Env = parent + appended` (last-wins on duplicate key) and empty `Env` leaves `cmd.Env` nil (inheritance untouched), via the real `defaultRunner`; assert `RunForegroundEnv` records a `Request` with the env and foreground transport. <!-- R1 R2 R3 -->
- [x] T009 [P] In `src/cmd/shll/install_test.go`: assert the `brew install` Request carries `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` when `osGoos=="linux"` and carries none when `osGoos=="darwin"` (override via `setOsGoos`, save/restore). <!-- R5 -->
- [x] T010 [P] In `src/cmd/shll/update_test.go`: assert `brew update --quiet`, `brew upgrade <shllFormula>`, and the `brew upgrade <formula>` fallback Requests carry the env on linux and not on darwin; assert per-tool `<tool> update` delegation Requests never carry the env on any OS. <!-- R5 R6 R7 -->
- [x] T011 [P] In `src/cmd/shll/brew_test.go`: assert `brewEnv()` returns the workaround entry on linux and `nil` on darwin (override via `setOsGoos`). <!-- R4 -->

## Execution Order

- T001 → T002 → T003 (same file, sequential; T002/T003 depend on the field from T001).
- T004 depends on nothing in this change (uses existing `osGoos`).
- T005, T006, T007 depend on T003 (`RunForegroundEnv`) and T004 (`brewEnv`).
- Phase 4 tests depend on the implementation tasks they cover; the four test tasks touch distinct files and are mutually `[P]`.

## Acceptance

### Functional Completeness

- [x] A-001 R1: `proc.Request` has an `Env []string` field documented as appended-not-replaced; nil/empty preserves full inheritance.
- [x] A-002 R2: `defaultRunner` sets `cmd.Env = append(os.Environ(), req.Env...)` only when `len(req.Env) > 0`; empty leaves `cmd.Env` nil.
- [x] A-003 R3: `RunForegroundEnv` exists, builds a foreground `Request` with `Env`, and `Run`/`RunForeground` signatures are unchanged (pass `Env: nil`); no `RunEnv` added.
- [x] A-004 R4: `brewEnv()` exists in `brew.go` next to `brewBinary`, returns the workaround entry on linux and nil otherwise, gated on the existing `osGoos` seam.
- [x] A-005 R5: all four live foreground brew install/upgrade/update calls route through `RunForegroundEnv(ctx, brewEnv(), …)`.
- [x] A-006 R6: `upgradeTool` injects the env only when `argv[0] == brewBinary`; `upgradeArgv`/`argvString` remain env-free.
- [x] A-007 R7: capture/query brew calls and non-brew subprocesses carry no env.

### Behavioral Correctness

- [x] A-008 R2: a duplicate key in the appended env overrides the inherited value (last-wins) — verified by a proc_test assertion.
- [x] A-009 R5: on linux the four live brew Requests carry `HOMEBREW_NO_REQUIRE_TAP_TRUST=1`; on darwin they carry none — verified by install/update tests overriding `osGoos`.
- [x] A-010 R6/R7: per-tool `<tool> update` delegation Requests carry no env on any OS — verified by an update test.
- [x] A-011 R7: the dry-run preview output is unchanged (env never enters `upgradeArgv`/`argvString`) — existing dry-run goldens still pass.

### Scenario Coverage

- [x] A-012 R3: `RunForegroundEnv` returns `(-1, err)` on transport error and `(code, nil)` on completion — exercised by proc_test.
- [x] A-013 R4: `brewEnv()` darwin/linux branches both exercised by brew_test via `setOsGoos`.

### Code Quality

- [x] A-014 Pattern consistency: new code follows existing proc/brew/test patterns (named const for the env var, `setOsGoos`/`fakeRunner` test seams, `osGoos` reused in place).
- [x] A-015 No unnecessary duplication: the workaround env has a single source of truth (`brewEnv()`); existing `proc.RunForegroundEnv`/`fakeRunner`/`setOsGoos` reused rather than reimplemented.
- [x] A-016 Constitution I (Security First): all new subprocess execution routes through `internal/proc` (no raw `os/exec` in command code); `TestNoProcImports` still passes; the env is an explicit `[]string`, no shell strings.
- [x] A-017 No magic strings (code-quality.md): the `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` override is a named constant, not open-coded.
- [x] A-018 Loud temporary-workaround documentation: `brewEnv()`'s comment cross-references both backlog `[38a6]` and `[tkch]` so removal is a one-spot edit.

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- If an item is not applicable, mark checked and prefix with **N/A**: `- [x] A-NNN **N/A**: {reason}`

## Assumptions

<!-- Inherits the intake's 12 graded assumptions (8 certain, 4 confident) verbatim;
     they fully specify the design and are not re-opened here. The rows below are
     the apply-stage decisions made while co-generating ## Requirements / ## Tasks
     beyond the intake's 12. -->

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Extract `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` as a named constant in `brew.go` rather than inlining the literal in `brewEnv()` | code-quality.md forbids magic strings ("formula names, timeout durations, tool roster entries all belong in named constants"); the env var name is the same class of literal | S:90 R:85 A:92 D:88 |
| 2 | Certain | Place the `brewEnv()` darwin/linux unit test in `brew_test.go` (alongside the other brew-helper unit tests) | Mirrors where `tapName`/brew-helper assertions already live; `setOsGoos` is package-level and reachable from any `cmd/shll` test file | S:88 R:80 A:90 D:85 |
| 3 | Confident | Add the proc-level `Env` plumbing assertion to `proc_test.go` using the real `defaultRunner` (constructing a `Request` directly), not the fake | proc_test.go already exercises `defaultRunner` directly (`TestDefaultRunner_RealBinary`); `cmd.Env` is observable only via the real runner, and a `Request`-recording fake cannot prove the parent-env append | S:75 R:70 A:80 D:72 |

3 apply-stage assumptions beyond the intake's 12 (2 certain, 1 confident, 0 tentative). Intake total: 12 (8 certain, 4 confident).
