# internal/proc

The centralized subprocess-execution wrapper used by every command in `src/cmd/shll/`. Constitution Principle I (Security First) requires this routing — **no package outside `src/internal/proc` may import `os/exec`**. Verified by `src/cmd/shll/` having zero `os/exec` imports today (acceptance A-029, A-044).

Source: `src/internal/proc/proc.go`.

## Public API

```go
package proc

// Sentinel: returned by Run/RunForeground when the named binary is not on PATH.
var ErrNotFound = errors.New("binary not found on PATH")

// Run captures stdout from name+args. stderr passes through to the parent.
// Returns ErrNotFound (matchable via errors.Is) if the binary is missing.
func Run(ctx context.Context, name string, args ...string) ([]byte, error)

// RunForeground inherits stdin/stdout/stderr from the parent and reports the
// child's exit code. (code, nil) on completion (any exit code); (-1, err) when
// exec fails before the subprocess starts.
func RunForeground(ctx context.Context, name string, args ...string) (int, error)

// RunForegroundEnv is RunForeground plus per-request env: it sets Request.Env so
// the child receives the given "KEY=VALUE" entries APPENDED to the inherited
// environment. Same TransportForeground and identical (code, error) contract as
// RunForeground. env nil/empty is exactly equivalent to RunForeground (child
// inherits the parent env verbatim).
func RunForegroundEnv(ctx context.Context, env []string, name string, args ...string) (int, error)
```

That is the entire surface command code uses. Callers never import `os/exec` directly.

`RunForegroundEnv` (change 38a6, `proc.go:105`) is the only env-passing transport.
The plain `Run`/`RunForeground` helpers take just `(ctx, name, args...)` and build the
`Request` internally, so they cannot set `Env`; a caller that needs per-request env
additions (today: the brew Linux trust workaround — see [cli/update](../cli/update.md)
and [cli/install](../cli/install.md)) uses `RunForegroundEnv`. Adding it as a sibling
helper, rather than widening `RunForeground`'s signature, keeps every existing
`RunForeground` caller untouched (Design Decision below).

## Internal types

```go
type Result struct {
    Stdout   []byte
    ExitCode int
    Err      error
}

type Transport int
const (
    TransportCapture    Transport = iota // buffer stdout; pass stderr through
    TransportForeground                  // inherit stdin/stdout/stderr
)

type Request struct {
    Name      string
    Args      []string
    Transport Transport
    Dir       string   // optional working dir; "" inherits parent cwd
    Env       []string // optional extra "KEY=VALUE" entries APPENDED to os.Environ(); nil = inherit-only (change 38a6)
}

type RunnerFunc func(ctx context.Context, req Request) Result

// The package-level test seam — defaults to defaultRunner, swappable in tests.
var Runner RunnerFunc = defaultRunner
```

The `Result/Request/Transport` triple is internal — command code never constructs a `Request`. It exists so the test seam can inspect what `Run`/`RunForeground`/`RunForegroundEnv` would have done without spawning a real process.

**`Request.Env` (change 38a6, `proc.go:64-68`)** — extra `KEY=VALUE` entries the runner *appends* to the inherited environment. The semantics are deliberately append-only and opt-in:

- `defaultRunner` sets `cmd.Env = append(os.Environ(), req.Env...)` **only when `len(req.Env) > 0`** (`proc.go:124-126`). It never *replaces* the inherited env — the parent environment is always the base.
- When `Env` is nil/empty, `defaultRunner` leaves `cmd.Env` unset, so the child inherits the parent env exactly as before. **This is a no-op for every pre-existing caller** — `Run`/`RunForeground` set no `Env`, so their behavior is byte-for-byte unchanged.

`RunForegroundEnv` is the only public helper that populates this field; it sets `Transport == TransportForeground` and `Env == <passed env>`.

## Test seam: `Runner`

Per Design Decision #7 (spec): the simplest, most-Go-idiomatic seam — a package-level function-typed variable. Tests swap it for a recording fake:

```go
// In tests (src/cmd/shll/update_test.go:33):
func installFakeRunner(t *testing.T, f *fakeRunner) {
    prev := proc.Runner
    t.Cleanup(func() { proc.Runner = prev })
    proc.Runner = f.Runner
}
```

The fake records every `Request` it receives and returns canned `Result` values (matched by binary name + args). This is how all five `src/cmd/shll/*_test.go` files avoid spawning real `brew` or per-tool subprocesses.

The proc package's own `proc_test.go` uses the same pattern (`withFakeRunner`) — the only test that actually spawns subprocesses is `TestDefaultRunner_RealBinary`, which uses `true`/`false` POSIX builtins (never project tools).

## Constitution I conformance

Every external-tool invocation:

- Routes through this package (verified by no `os/exec` imports outside `src/internal/proc`).
- Uses `exec.CommandContext(ctx, name, args...)` in `defaultRunner` — never `exec.Command` (no context).
- Passes binary name + explicit `[]string` arguments — never a shell-interpreted command string. There is no `sh -c "..."` anywhere in shll's call sites.

These properties are tested at the source level (acceptance A-029, A-044, A-049, A-050) and are required for any future addition to the wrapper.

## Transport semantics

### `TransportCapture` (used by `proc.Run`)

- `cmd.Stdout = &buf` (captured into `Result.Stdout`).
- `cmd.Stderr = os.Stderr` (passes through to user — subprocess error messages reach the user even when stdout is captured).
- `cmd.Run()` blocks until completion.
- On `exec.ErrNotFound` → return `Result{Err: ErrNotFound}` (mapped to package sentinel).
- On any other error → return `Result{Stdout: buf.Bytes(), Err: err}` (callers get the partial stdout plus the error).
- On success → return `Result{Stdout: buf.Bytes()}`.

### `TransportForeground` (used by `proc.RunForeground`)

- `cmd.Stdin = os.Stdin`, `cmd.Stdout = os.Stdout`, `cmd.Stderr = os.Stderr` — full inherit.
- On `exec.ErrNotFound` → return `Result{ExitCode: -1, Err: ErrNotFound}`.
- On `*exec.ExitError` (subprocess ran and exited non-zero) → return `Result{ExitCode: <code>}` (no Err — the public wrapper translates to `(code, nil)`). Callers branch on the code.
- On any other error (I/O failure pre-spawn) → return `Result{ExitCode: -1, Err: err}`.
- On success → return `Result{ExitCode: 0}`.

`exitCode(err)` (`src/internal/proc/proc.go:162`) is the small helper that unwraps `*exec.ExitError` to its `ExitCode()`.

### Per-request env (both transports)

The `cmd.Env` wiring is transport-independent — it runs in `defaultRunner` (`proc.go:124-126`) *before* the transport switch, so it applies to capture and foreground alike. Today only foreground callers reach it (via `RunForegroundEnv`), but the field and the runner wiring are not transport-gated. Append-to-`os.Environ()`, opt-in on `len(req.Env) > 0`; see [`Request.Env`](#internal-types) above.

## ErrNotFound contract

The package sentinel `ErrNotFound` is the only "binary missing" signal callers need to match:

```go
// from src/cmd/shll/brew.go:20
if errors.Is(err, proc.ErrNotFound) {
    return false  // brew not installed
}
```

`defaultRunner` translates `exec.ErrNotFound` (the stdlib sentinel) into `proc.ErrNotFound` so callers do not need to import `os/exec`. Tests assert this in `TestRun_ErrNotFound` and `TestDefaultRunner_RealBinary` (the latter using a deliberately-not-real binary name `shll-nonesuch-binary-xyz`).

## API divergence from hop's proc

shll's wrapper is intentionally lighter than hop's:

- **No `dir` argument** in the public `RunForeground` signature. hop has `RunForeground(ctx, dir, name, args...)` because hop spawns subprocesses scoped to git worktree directories. shll has no cwd-scoped subprocesses today — every brew/tool invocation runs in the parent cwd. The `Request.Dir` field exists internally for forward compatibility, but no public API takes it.

If a future shll subcommand needs cwd scoping, the path forward is to either (a) add a `RunIn(ctx, dir, name, args...)` overload, or (b) thread `Dir` via a small option struct. Do not introduce silent cwd changes.

## Test coverage

`src/internal/proc/proc_test.go`:

- `TestRun_CaptureHappyPath` — fake records the Request, Run returns its Stdout.
- `TestRun_ErrNotFound` — fake returns `ErrNotFound` → `errors.Is(err, ErrNotFound)` matches.
- `TestRunForeground_ExitCode` — fake returns `ExitCode: 7` → `RunForeground` returns `(7, nil)`.
- `TestRunForeground_ErrNotFound` — fake returns `ErrNotFound` → `(-1, ErrNotFound)`.
- `TestRunner_RecordsTransportSelection` — `Run` records `TransportCapture`, `RunForeground` records `TransportForeground`.
- `TestDefaultRunner_RealBinary` — exercises the production path with `true`, `false`, and a missing binary; the only test that spawns real processes (and never spawns project tools).
- `TestRunForegroundEnv_RecordsEnv` *(change 38a6)* — `RunForegroundEnv(ctx, env, …)` records a `Request` with `Transport == TransportForeground` and `Env` equal to the passed env.
- `TestRunForegroundEnv_ErrNotFound` *(change 38a6)* — exec failure before spawn → `(-1, ErrNotFound)`, matching `RunForeground`.
- `TestRunForegroundEnv_NilEnvMatchesRunForeground` *(change 38a6)* — `nil` env behaves identically to `RunForeground` (records nil `Env`, same `(code, error)` contract).
- `TestDefaultRunner_EnvAppendsToInherited` *(change 38a6)* — production path: a non-empty `Env` arm asserts `os.Environ()` entries plus the appended pair are present on the child; the empty-`Env` arm asserts `cmd.Env` is left nil and the child inherits the parent env verbatim (the no-behavior-change guard).

## Design Decisions

### `RunForegroundEnv` sibling helper, not a widened `RunForeground` (change 38a6)

> *Why*: A brew caller needed to pass `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` per call (the Linux trust workaround — see [cli/update](../cli/update.md)/[cli/install](../cli/install.md)). The env had to reach `exec.CommandContext`'s `cmd.Env` (Constitution I — env on the cmd, never a shell string), but command code never constructs a `Request` directly. A dedicated `RunForegroundEnv(ctx, env, name, args...)` mirrors the existing `Run`/`RunForeground` pairing and keeps the brew call sites readable.
> *Rejected*: (a) widening `RunForeground`'s signature to `(ctx, env, name, args...)` — breaks every existing caller for a feature only one caller uses; (b) having callers build a `Request` literal directly — command code never touches `Request`, and exposing it would invite ad-hoc transport selection outside the two blessed helpers.

### Append-to-inherited, opt-in env semantics (change 38a6)

> *Why*: `defaultRunner` appends `req.Env` to `os.Environ()` rather than replacing it, and only when `len(req.Env) > 0`. Appending preserves the full parent environment (the brew workaround *adds* one var; it must not drop the user's PATH/HOME/etc.). The `len > 0` guard means callers passing no env leave `cmd.Env` nil — the child inherits the parent env exactly as Go's default, so the change is a provable no-op for every pre-existing call site.
> *Rejected*: always setting `cmd.Env = append(os.Environ(), req.Env...)` unconditionally — functionally equivalent for current Go, but the explicit nil-when-empty guard documents the no-behavior-change intent and is the property the test (`TestDefaultRunner_EnvAppendsToInherited`, empty arm) pins.

## Changelog

- **change 38a6** (`260613-38a6-brew-no-tap-trust-workaround`): Added the `Env []string` field to `Request` (append-to-`os.Environ()`, opt-in on non-empty; nil = inherit-only, no behavior change for existing callers) and the `RunForegroundEnv(ctx, env, name, args...)` foreground-with-env transport (mirrors `RunForeground`'s `(code, error)` contract). Sole consumer today is the brew Linux trust workaround in `cmd/shll` — itself **TEMPORARY**, slated for removal under backlog `[tkch]`. The `proc` additions are general-purpose and not themselves tied to the workaround's removal.

## Cross-references

- All consumers in `src/cmd/shll/*.go` — see [cli/commands](../cli/commands.md), [cli/update](../cli/update.md), [cli/install](../cli/install.md), [cli/shell-init](../cli/shell-init.md), [cli/version](../cli/version.md).
- Constitution I (Security First) — the principle this package enforces.
- spec.md Design Decision #7 — package-level `Runner` is the chosen test seam.
