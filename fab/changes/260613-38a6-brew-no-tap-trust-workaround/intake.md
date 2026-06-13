# Intake: Bake HOMEBREW_NO_REQUIRE_TAP_TRUST=1 into shll's brew calls (Linux workaround)

**Change**: 260613-38a6-brew-no-tap-trust-workaround
**Created**: 2026-06-13
**Status**: Draft

## Origin

> Initiated from backlog item `[38a6]` (2026-06-13), sourced from `~/code/sahil87/shll/fab/backlog.md`
> (the main repo — this worktree's `fab/backlog.md` is stale and predates the item). Cold `/fab-new`
> invocation: the original arg `38a6` is the worktree/branch suffix that happens to equal the backlog ID.
> No prior `/fab-discuss`; the design below is lifted verbatim from the backlog item's diagnosis and
> implementation notes, plus call-site verification done at intake time against the actual source.

The backlog item documents a fully-diagnosed Homebrew bug (root-caused 2026-06-12 during an
`idea` v0.0.9→0.0.10 upgrade) and prescribes a scoped, temporary workaround. Its cleanup follow-up
is tracked separately as `[tkch]` — remove this workaround once the upstream fix lands.

## Why

**Problem.** Homebrew 6.0's Linux bubblewrap sandbox runs the formula build/install (`build.rb`)
inside `bwrap`, where `deny_read_home` (`Library/Homebrew/sandbox.rb` ~line 191) masks almost all of
`$HOME`. Its exception list covers `HOMEBREW_PREFIX` / `CACHE` / `LOGS` / `TEMP` but **not**
`HOMEBREW_USER_CONFIG_HOME` (`~/.homebrew`), where `trust.json` lives. So when
`HOMEBREW_REQUIRE_TAP_TRUST=1` is set, the sandboxed `build.rb` re-checks tap trust, cannot read
`~/.homebrew/trust.json`, and raises `Homebrew::UntrustedTapError` at `build.rb:264` — three lines
*before* the error pipe is connected (`build.rb:267`). The message is swallowed; brew surfaces only an
opaque `bwrap ... exited with 1` plus a giant arg dump.

**Consequence if unfixed.** This breaks `brew install`/`upgrade sahil87/tap/<formula>` for **every**
roster tool on any Linux box where the user set `HOMEBREW_REQUIRE_TAP_TRUST=1` — which shll itself
*actively encourages* via `shll shell-setup --trust-tap` (it runs `brew trust --tap sahil87/tap` and
writes `export HOMEBREW_REQUIRE_TAP_TRUST=1` into the user's rc). So shll's own pro-trust posture is
what walks users into the broken state. Both `shll install` and `shll update` (self-upgrade + the
brew-fallback path) are affected.

**Why this approach.** The verified workaround is to prefix the brew call with
`HOMEBREW_NO_REQUIRE_TAP_TRUST=1`. This keeps the sandbox **active** (security posture preserved) and
skips *only* the broken in-sandbox trust re-check. Confirmed working:
`HOMEBREW_NO_REQUIRE_TAP_TRUST=1 brew upgrade sahil87/tap/idea` completed cleanly. The alternative —
telling users to unset `HOMEBREW_REQUIRE_TAP_TRUST` globally — would disable trust enforcement
everywhere (including outside the sandbox), which is strictly worse than a sandbox-scoped, per-call,
Linux-gated override.

## What Changes

### 1. Add an `Env` field to `proc.Request`

`internal/proc/proc.go` — `Request` has no `Env` field today (`src/internal/proc/proc.go:59-64`), and
`defaultRunner` never sets `cmd.Env`, so children inherit the parent environment verbatim
(`proc.go:95-99`). Add a field for per-request env additions:

```go
type Request struct {
	Name      string
	Args      []string
	Transport Transport
	Dir       string
	Env       []string // extra "KEY=VALUE" entries appended to the inherited env; nil = inherit only
}
```

In `defaultRunner`, when `req.Env` is non-empty, set `cmd.Env = append(os.Environ(), req.Env...)`
(append-to-inherited, never replace). When `req.Env` is nil/empty, leave `cmd.Env` unset so the child
inherits the parent env exactly as before — preserving current behavior for every existing call.

> The package-level helpers `proc.Run` / `proc.RunForeground` take only `(ctx, name, args...)` and
> build the `Request` internally, so they cannot pass `Env`. See Design Decision #2 for how brew
> callers reach the new field.

### 2. Centralize the override in a `brewEnv()` helper in `brew.go`

`src/cmd/shll/brew.go` already owns `brewBinary` and the trust machinery (and legitimately imports
`internal/proc`). Add a single source of truth for the brew env so the follow-up removal (`[tkch]`)
is a one-spot edit:

```go
// brewEnv returns the extra environment entries shll injects into its brew
// install/upgrade/update subprocesses. On Linux it sets HOMEBREW_NO_REQUIRE_TAP_TRUST=1
// to work around a Homebrew 6.0 bubblewrap-sandbox bug (see backlog [38a6]); the
// in-sandbox trust re-check cannot read ~/.homebrew/trust.json under deny_read_home,
// so HOMEBREW_REQUIRE_TAP_TRUST=1 wrongly fails the build. The sandbox stays active —
// only the broken trust re-check is skipped. macOS is unaffected (no bwrap), so this
// returns nil there and trust enforcement is preserved.
//
// TEMPORARY: remove once the upstream fix lands — tracked in backlog [tkch].
func brewEnv() []string {
	if runtime.GOOS == "linux" {
		return []string{"HOMEBREW_NO_REQUIRE_TAP_TRUST=1"}
	}
	return nil
}
```

### 3. Wire the override into the brew foreground call sites — and ONLY those

The override must reach the four brew-spawning subprocesses, but **must not** leak into per-tool
`<tool> update` delegations (Constitution IV — shll composes per-tool CLIs; injecting brew-specific
env into a tool's own `update` would be wrong and could mask the tool's own trust behavior).

Because `proc.Run`/`proc.RunForeground` don't expose `Env`, the brew foreground sites get a thin new
transport that does: **`proc.RunForegroundEnv(ctx, env, name, args...)`** (confirmed — Decision #8). It
builds a `Request` with `Env` set and otherwise behaves exactly like `RunForeground` (same
`TransportForeground`, same `(code, error)` return contract).

The four call sites:

| # | Location | Current call | Becomes |
|---|----------|--------------|---------|
| a | `install.go:145` | `proc.RunForeground(ctx, brewBinary, "install", t.Formula)` | foreground-with-`brewEnv()` |
| b | `update.go:242` | `proc.RunForeground(ctx, brewBinary, "update", "--quiet")` | foreground-with-`brewEnv()` |
| c | `update.go:295` | `proc.RunForeground(ctx, brewBinary, "upgrade", shllFormula)` | foreground-with-`brewEnv()` |
| d | `update.go:404` (`upgradeTool` → `upgradeArgv`) | `proc.RunForeground(ctx, argv[0], argv[1:]...)` | inject `brewEnv()` **only when `argv[0] == brewBinary`** |

> **Subtlety at site (d)** — verified by reading `upgradeArgv` (`update.go:415-428`): it returns either
> a `brew upgrade <formula>` argv (when the tool has no `Update` argv, `len(t.Update) == 0`) **or** a
> per-tool `<tool> update [--skip-brew-update]` argv. The backlog says "inject on brew calls, NOT
> per-tool `<tool> update` delegations." So site (d) must gate the env on `argv[0] == brewBinary` —
> a blanket inject at `upgradeTool` would wrongly pollute per-tool delegations. Today the roster has no
> tool with an empty `Update` argv, so this branch is the "hypothetical future tool" fallback, but the
> gate is correct and cheap and keeps the contract honest.

### 4. Loud documentation + removal cross-reference

Code comments at `brewEnv()` (and a one-liner at each call site, or just at the helper) MUST:
- State this is a **temporary** Homebrew-bug workaround, not permanent design.
- Name the bug (deny_read_home masks `~/.homebrew`, swallowed `UntrustedTapError`).
- Cross-reference removal backlog item `[tkch]`.
- Note the Linux gate is deliberate (macOS sandboxing is unaffected and keeps enforcing trust).

### 5. Tests

Add/extend a `proc` or `brew` unit test asserting:
- On `linux`: the brew `Request`s carry `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` in `Env`.
- On `darwin`: they do **not** (no override).
- Per-tool `<tool> update` delegations carry **no** brew override on any platform.

`brewEnv()` keys off `runtime.GOOS`, which a unit test can't flip directly. Resolved (Decision #9):
extract the GOOS read behind a package-level injectable seam — e.g. `var goosFunc = func() string { return runtime.GOOS }`
(or a plain `var currentGOOS = runtime.GOOS`) that `brewEnv()` consults, so a test can swap it to assert
both the linux (override present) and darwin (override absent) branches in one table-driven test, with
no per-OS build tags. Mirrors the existing `nowFunc` (clock.go) and `proc.Runner` injectable seams.
Tests must conform to the implementation spec (Constitution — Test Integrity), not the reverse.

## Affected Memory

- `internal/proc`: (modify) document the new `Request.Env` field — append-to-inherited semantics, nil = inherit-only, and the foreground-with-env transport seam.
- `cli/install`: (modify) note the Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` override on `brew install`, why it exists, and the `[tkch]` removal cross-ref.
- `cli/update`: (modify) same override note for the `brew update`/self-upgrade/brew-fallback paths, and the explicit carve-out that per-tool `<tool> update` delegations are NOT touched.

## Impact

- **`src/internal/proc/proc.go`** — new `Request.Env` field + `defaultRunner` env wiring; possibly a new `RunForegroundEnv` helper (depends on Decision #2).
- **`src/cmd/shll/brew.go`** — new `brewEnv()` helper; new `runtime` import.
- **`src/cmd/shll/install.go`** — call site (a).
- **`src/cmd/shll/update.go`** — call sites (b), (c), (d) incl. the `argv[0] == brewBinary` gate.
- **Tests** — `proc` and/or `brew`/`update`/`install` test files.
- **No** new top-level subcommand, no new dependency, no state. Constitution I (proc-routed exec) is
  upheld — the env is set on the `exec.CommandContext` env, not via a shell string. Constitution IV
  (compose, don't absorb) is upheld by the per-tool carve-out at site (d).

## Open Questions

- None. The two design choices (transport shape for passing `Env`; how to make `runtime.GOOS`
  testable) were confirmed by the user during clarification (2026-06-13) and are now Certain — see
  Decisions #8 and #9.

## Clarifications

### Session 2026-06-13 (bulk confirm)

| # | Action | Detail |
|---|--------|--------|
| 4 | Confirmed | — |
| 5 | Confirmed | — |
| 6 | Confirmed | — |
| 7 | Confirmed | — |
| 8 | Confirmed | Tentative resolved → new `proc.RunForegroundEnv(ctx, env, name, args...)` helper |
| 9 | Confirmed | Tentative resolved → injectable GOOS seam (package-level `goosFunc`/var) |

User confirmed all Confident (#4–#7) and both Tentative (#8, #9) assumptions in one turn
("agreed to tentative. Also confident").

## Assumptions

| # | Grade | Decision | Rationale | Scores |
|---|-------|----------|-----------|--------|
| 1 | Certain | Workaround is the env var `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` injected per brew call (sandbox stays active) | Verified in backlog: `HOMEBREW_NO_REQUIRE_TAP_TRUST=1 brew upgrade sahil87/tap/idea` completed cleanly; named exactly | S:98 R:80 A:95 D:95 |
| 2 | Certain | Scope the override to Linux only (`runtime.GOOS == "linux"`) | Bug is bubblewrap-sandbox-specific; macOS unaffected and must keep enforcing trust (backlog PREFERRED scoping; aligns with Constitution V graceful behavior) | S:95 R:70 A:90 D:90 |
| 3 | Certain | Inject on brew `install`/`update`/`upgrade` calls ONLY, never per-tool `<tool> update` delegations | Backlog IMPLEMENTATION note is explicit; Constitution IV forbids polluting a sub-tool's own CLI invocation | S:95 R:75 A:90 D:88 |
| 4 | Certain | Add `Env []string` to `proc.Request` with append-to-`os.Environ()` semantics; nil = inherit-only (no behavior change for existing calls) | Clarified — user confirmed. Backlog prescribes the `Env` field + `defaultRunner` appending to `cmd.Environ()`; verified `Request` has no Env today | S:95 R:65 A:85 D:80 |
| 5 | Certain | Centralize the override in a single `brewEnv()` helper near `brewBinary` in `brew.go` | Clarified — user confirmed. One source of truth so `[tkch]` removal is a one-spot edit; `brew.go` already owns `brewBinary` + imports `internal/proc` | S:95 R:70 A:88 D:82 |
| 6 | Certain | Site (d) gates the env on `argv[0] == brewBinary` so the brew-fallback path gets it but per-tool delegations don't | Clarified — user confirmed. `upgradeArgv` (update.go:415-428) returns brew-upgrade argv OR per-tool argv; the gate is the correct discriminator | S:95 R:70 A:88 D:80 |
| 7 | Certain | Tests assert override present on linux / absent on darwin / absent for per-tool delegations, via the proc/brew test seam | Clarified — user confirmed. Backlog requires it; project convention is test-alongside with a fake `proc.Runner` | S:95 R:75 A:85 D:85 |
| 8 | Certain | Expose `Env` to brew callers via a new `proc.RunForegroundEnv(ctx, env, name, args...)` helper | Clarified — user confirmed. Matches the existing `Run`/`RunForeground` pairing and keeps the brew call sites readable | S:95 R:75 A:70 D:55 |
| 9 | Certain | Make `runtime.GOOS` testable via a package-level injectable seam (`goosFunc`/var defaulting to `runtime.GOOS`) rather than build-tagged test files | Clarified — user confirmed. Lets one test table assert both linux and darwin; mirrors existing seams (`nowFunc`, `proc.Runner`) | S:95 R:75 A:72 D:55 |

9 assumptions (9 certain, 0 confident, 0 tentative, 0 unresolved).
