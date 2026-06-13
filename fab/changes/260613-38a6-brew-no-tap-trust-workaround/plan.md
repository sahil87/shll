# Plan: Bake HOMEBREW_NO_REQUIRE_TAP_TRUST=1 into shll's brew calls (Linux workaround)

**Change**: 260613-38a6-brew-no-tap-trust-workaround
**Status**: In Progress
**Intake**: `intake.md`

## Requirements

### internal/proc: Per-request environment additions

#### R1: `proc.Request` carries optional per-request env additions
`proc.Request` SHALL gain an `Env []string` field holding extra `KEY=VALUE` entries.
`defaultRunner` SHALL set `cmd.Env = append(os.Environ(), req.Env...)` when `req.Env`
is non-empty, and SHALL leave `cmd.Env` unset (nil) when `req.Env` is empty — so existing
callers that pass no env inherit the parent environment exactly as before.

- **GIVEN** a `Request` with a non-empty `Env`
- **WHEN** `defaultRunner` builds the `exec.Cmd`
- **THEN** the child's environment is the parent environment (`os.Environ()`) with the
  `Env` entries appended (never a replacement of the inherited env)
- **AND** **GIVEN** a `Request` with nil/empty `Env`, **THEN** `cmd.Env` is left unset and
  the child inherits the parent env verbatim (no behavior change for existing calls)

#### R2: `proc.RunForegroundEnv` exposes `Env` to foreground callers
The proc package SHALL expose `RunForegroundEnv(ctx, env, name, args...)` with the same
`(code, error)` contract as `RunForeground` and the same `TransportForeground`, building a
`Request` with `Env` set. `Run`/`RunForeground` SHALL remain unchanged (they cannot pass `Env`).

- **GIVEN** a caller invokes `proc.RunForegroundEnv(ctx, env, name, args...)`
- **WHEN** the runner records the `Request`
- **THEN** the recorded `Request` has `Transport == TransportForeground` and `Env` equal to
  the passed `env`
- **AND** the return contract matches `RunForeground` exactly: `(code, nil)` on completion,
  `(-1, err)` when exec fails before the subprocess starts

### cmd/shll: Linux-only brew trust-trust workaround

#### R3: A single `brewEnv()` helper is the source of truth for the override
`brew.go` SHALL define `brewEnv()` returning `[]string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}`
on Linux and `nil` otherwise. The GOOS read SHALL go through a package-level injectable seam
(`goosFunc`, defaulting to `runtime.GOOS`) so tests can assert both branches without build tags.
The helper SHALL carry loud comments: temporary Homebrew-bug workaround, the named bug
(`deny_read_home` masks `~/.homebrew` so the sandboxed trust re-check cannot read `trust.json`),
removal cross-ref `[tkch]`, and the deliberate Linux gate.

- **GIVEN** `goosFunc()` returns `"linux"`
- **WHEN** `brewEnv()` is called
- **THEN** it returns `[]string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}`
- **AND** **GIVEN** `goosFunc()` returns `"darwin"`, **THEN** `brewEnv()` returns `nil`

#### R4: The override reaches the four brew foreground call sites
The override SHALL be injected via `proc.RunForegroundEnv(ctx, brewEnv(), ...)` at exactly
four brew-spawning foreground sites: `install.go` brew install, `update.go` brew update
`--quiet`, `update.go` shll self-upgrade, and the brew-fallback inside `upgradeTool`.

- **GIVEN** `shll install` installs a missing formula
- **WHEN** it foregrounds `brew install <formula>`
- **THEN** the recorded `Request` carries `brewEnv()` in `Env`
- **AND** the same holds for `brew update --quiet`, `brew upgrade <shllFormula>`, and the
  brew-fallback `brew upgrade <formula>` path

#### R5: Per-tool delegations never receive the brew override (Constitution IV)
At the `upgradeTool` site, `brewEnv()` SHALL be injected ONLY when `argv[0] == brewBinary`.
Per-tool `<tool> update [--skip-brew-update]` delegations SHALL receive NO override on any
platform — shll composes per-tool CLIs and MUST NOT pollute a tool's own `update` invocation.

- **GIVEN** an installed roster tool with an `Update` argv
- **WHEN** `shll update` delegates to `<tool> update [--skip-brew-update]`
- **THEN** the recorded `Request` for that delegation carries NO brew override (`Env` empty),
  even on Linux
- **AND** **GIVEN** a tool with no `Update` argv (brew-fallback), **THEN** its `brew upgrade
  <formula>` Request DOES carry the override on Linux

### Non-Goals

- macOS behavior changes — `brewEnv()` returns nil on darwin; trust enforcement is preserved.
- Changing `Run`/`RunForeground` signatures — they keep their current `(ctx, name, args...)` shape.
- Touching capture-transport (`proc.Run`) call sites — only the four foreground brew sites are wired.
- A permanent design — this is a temporary workaround removed under backlog `[tkch]`.

### Design Decisions

1. **New `proc.RunForegroundEnv` helper** (intake Decision #8): expose `Env` to brew callers
   via a dedicated foreground-with-env helper — *Why*: mirrors the existing `Run`/`RunForeground`
   pairing and keeps the brew call sites readable — *Rejected*: widening `RunForeground`'s
   signature (breaks every existing caller) or having callers build `Request` directly (command
   code never constructs a `Request`).
2. **Injectable GOOS seam** (intake Decision #9): `var goosFunc = func() string { return runtime.GOOS }`
   consulted by `brewEnv()` — *Why*: lets one table-driven test assert both linux and darwin
   branches; mirrors existing `nowFunc` (clock.go) and `proc.Runner` seams — *Rejected*:
   build-tagged per-OS test files (can't exercise both branches in one run).
3. **`argv[0] == brewBinary` gate at site (d)** (intake Decision #6): the brew-fallback path gets
   the override, per-tool delegations don't — *Why*: `upgradeArgv` returns either a brew-upgrade
   argv or a per-tool argv; the binary name is the correct discriminator — *Rejected*: a blanket
   inject at `upgradeTool` (would pollute per-tool delegations, violating Constitution IV).

## Tasks

### Phase 1: Core Implementation (proc)

- [x] T001 Add `Env []string` field to `proc.Request` and wire `cmd.Env = append(os.Environ(), req.Env...)` (only when non-empty) in `defaultRunner` in `src/internal/proc/proc.go` <!-- R1 -->
- [x] T002 Add `proc.RunForegroundEnv(ctx, env, name, args...)` in `src/internal/proc/proc.go` — builds a `Request` with `Env` set, `TransportForeground`, same `(code, error)` contract as `RunForeground` <!-- R2 -->

### Phase 2: Core Implementation (cmd/shll)

- [x] T003 Add the `goosFunc` package-level seam and the `brewEnv()` helper (with loud workaround comments + `[tkch]` cross-ref) in `src/cmd/shll/brew.go`; add the `runtime` import <!-- R3 -->
- [x] T004 Wire `proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "install", t.Formula)` at the brew install site in `src/cmd/shll/install.go` <!-- R4 -->
- [x] T005 Wire `brewEnv()` at the `brew update --quiet` and `brew upgrade shllFormula` self-upgrade sites in `src/cmd/shll/update.go` <!-- R4 -->
- [x] T006 Gate `brewEnv()` injection on `argv[0] == brewBinary` inside `upgradeTool` in `src/cmd/shll/update.go` — per-tool delegations stay on plain `proc.RunForeground` <!-- R5 -->

### Phase 3: Tests

- [x] T007 [P] Extend `src/internal/proc/proc_test.go`: assert `RunForegroundEnv` records `Env` on the `Request` and preserves the `RunForeground` `(code, error)` contract; assert `defaultRunner` env behavior (non-empty Env appends to inherited; empty Env leaves `cmd.Env` nil) <!-- R1 R2 -->
- [x] T008 [P] Add a table-driven test in `src/cmd/shll/brew_test.go` swapping `goosFunc` to assert `brewEnv()` returns the override on linux and nil on darwin <!-- R3 -->
- [x] T009 Add tests in `src/cmd/shll/install_test.go` and `src/cmd/shll/update_test.go` asserting brew Requests carry the override on linux, do NOT on darwin, and per-tool `<tool> update` delegations carry NO override on any platform (using the fake `proc.Runner` seam + `goosFunc` swap) <!-- R4 R5 -->

## Execution Order

- T001 blocks T002 (both edit proc.go; RunForegroundEnv relies on the Env field).
- T003 blocks T004, T005, T006 (call sites consume `brewEnv()`).
- T002 blocks T004, T005, T006 (call sites consume `RunForegroundEnv`).
- T007–T009 follow their implementation tasks; T007 and T008 are independent ([P]).

## Acceptance

### Functional Completeness

- [x] A-001 R1: `proc.Request` has an `Env []string` field and `defaultRunner` appends it to `os.Environ()` only when non-empty (nil/empty leaves `cmd.Env` unset) — `proc.go:64-68` (field), `proc.go:124-126` (`if len(req.Env) > 0 { cmd.Env = append(os.Environ(), req.Env...) }`)
- [x] A-002 R2: `proc.RunForegroundEnv(ctx, env, name, args...)` exists, sets `Env` + `TransportForeground`, and matches `RunForeground`'s `(code, error)` contract — `proc.go:105-111`
- [x] A-003 R3: `brewEnv()` returns `["HOMEBREW_NO_REQUIRE_TAP_TRUST=1"]` on linux and nil otherwise, reading GOOS through the injectable `goosFunc` seam — `brew.go:19` (seam), `brew.go:36-41` (helper)
- [x] A-004 R4: All four brew foreground sites (install, update --quiet, self-upgrade, brew-fallback) inject `brewEnv()` — `install.go:147`, `update.go:244`, `update.go:299`, `update.go:416`
- [x] A-005 R5: The `upgradeTool` site gates the override on `argv[0] == brewBinary`; per-tool delegations receive none — `update.go:415-418`

### Behavioral Correctness

- [x] A-006 R1: Existing proc callers (those passing no env) see no behavior change — `cmd.Env` stays nil and the child inherits the parent env verbatim — guard at `proc.go:124`; verified by `TestDefaultRunner_EnvAppendsToInherited` (empty-Env arm) and `TestRunForegroundEnv_NilEnvMatchesRunForeground`
- [x] A-007 R5: On linux, a per-tool `<tool> update` delegation Request carries an empty `Env` while the same run's brew Requests carry the override — `TestUpdate_BrewTrustOverride_PerGOOS/linux` asserts both halves

### Scenario Coverage

- [x] A-008 R1 R2: `proc_test.go` exercises the new `Env` field and `RunForegroundEnv` contract via the fake runner — `TestRunForegroundEnv_RecordsEnv`, `_ErrNotFound`, `_NilEnvMatchesRunForeground`, `TestDefaultRunner_EnvAppendsToInherited`
- [x] A-009 R3: `brew_test.go` table test exercises both the linux (override present) and darwin (override absent) branches via `goosFunc` — `TestBrewEnv_PerGOOS` (`brew_test.go:23`)
- [x] A-010 R4 R5: `install_test.go`/`update_test.go` assert override present on linux brew sites, absent on darwin, and absent for per-tool delegations on any platform — `TestInstall_BrewTrustOverride_PerGOOS`, `TestUpdate_BrewTrustOverride_PerGOOS`

### Edge Cases & Error Handling

- [x] A-011 R5: The brew-fallback path (tool with empty `Update` argv) carries the override on linux, confirming the gate keys off the binary name, not the call site — `TestUpdate_BrewFallbackCarriesOverride` (`update_test.go:1209`)

### Code Quality

- [x] A-012 Pattern consistency: New code follows surrounding naming/structure — `goosFunc` mirrors `nowFunc` (`clock.go:14`); `RunForegroundEnv` mirrors `RunForeground`; the override string is the existing `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` literal already named in `trustHatchHint` (`brew.go:114`)
- [x] A-013 No unnecessary duplication: A single `brewEnv()` helper is the one source of truth so the `[tkch]` removal is a one-spot edit — all 4 sites call `brewEnv()`; the literal appears once in source (`brew.go:38`)
- [x] A-014 Constitution I (Security First): env is set on `exec.CommandContext`'s `cmd.Env`, never via a shell string; all exec stays routed through `internal/proc` — `proc.go:116,125`; no `os/exec` import added to cmd/shll
- [x] A-015 Constitution IV (Composition): per-tool `<tool> update` delegations are not polluted with brew-specific env — `update.go:418` plain `RunForeground`; pinned by `TestUpdate_BrewTrustOverride_PerGOOS` delegation arm
- [x] A-016 No magic strings: `brewBinary` is reused for the gate (`update.go:415`); the override key/value is centralized in `brewEnv()` (`brew.go:38`)

## Notes

- Check items as you review: `- [x]`
- All acceptance items must pass before `/fab-continue` (hydrate)
- `TEMPORARY`: this whole change is removed under backlog `[tkch]` once the upstream Homebrew fix lands.

## Deletion Candidates

None — this change adds new functionality without making existing code redundant

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Override is `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` injected per brew call (sandbox stays active) | Inherited from intake #1 (user-confirmed, verified working) | S:98 R:80 A:95 D:95 |
| 2 | Certain | Linux-only scope via the `goosFunc` seam (`runtime.GOOS == "linux"`) | Inherited from intake #2/#9 — bug is bwrap-specific; macOS keeps enforcing trust | S:95 R:70 A:90 D:90 |
| 3 | Certain | Inject on brew install/update/upgrade only, never per-tool delegations (`argv[0] == brewBinary` gate) | Inherited from intake #3/#6 — Constitution IV | S:95 R:75 A:90 D:88 |
| 4 | Certain | `Env []string` on `proc.Request`, append-to-`os.Environ()`, nil = inherit-only | Inherited from intake #4 — no behavior change for existing calls | S:95 R:65 A:85 D:80 |
| 5 | Certain | Single `brewEnv()` helper in `brew.go` as the source of truth | Inherited from intake #5 — one-spot `[tkch]` removal | S:95 R:70 A:88 D:82 |
| 6 | Certain | New `proc.RunForegroundEnv(ctx, env, name, args...)` exposes `Env` to brew callers | Inherited from intake #8 — mirrors `Run`/`RunForeground` pairing | S:95 R:75 A:70 D:55 |
| 7 | Certain | Injectable `goosFunc` seam (defaults to `runtime.GOOS`) for testability | Inherited from intake #9 — mirrors `nowFunc`, `proc.Runner` seams | S:95 R:75 A:72 D:55 |

7 assumptions (7 certain, 0 confident, 0 tentative, 0 unresolved). All inherited from the intake's 9 user-confirmed Certain decisions; no new SRAD decisions arose during planning (the design is fully settled).
