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

// RunForegroundEnv is RunForeground with extra KEY=VALUE entries appended to the
// child's inherited environment (env is a slice of KEY=VALUE strings). Passing
// nil/empty is equivalent to RunForeground (full inheritance, nothing appended).
// Appended entries win on duplicate keys (see Request.Env). Same (code, nil) /
// (-1, err) contract as RunForeground.
func RunForegroundEnv(ctx context.Context, env []string, name string, args ...string) (int, error)
```

That is the entire surface command code uses. Callers never import `os/exec` directly.

`RunForegroundEnv` (added in change 38a6) is the only env-carrying variant — `Run` and `RunForeground` keep their original signatures and pass `Env: nil`. It exists so shll's brew install/upgrade/update call sites can inject the Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` sandbox-trust workaround via `brewEnv()` (`src/cmd/shll/brew.go`) without disturbing every other caller. A capture-mode `RunEnv` was deliberately **not** added (deferred per the change's YAGNI assumption — no live capture-brew failure path needs it).

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
    Dir       string  // optional working dir; "" inherits parent cwd
    Env       []string // extra KEY=VALUE entries appended to the parent env; nil/empty = inherit only (change 38a6)
}

type RunnerFunc func(ctx context.Context, req Request) Result

// The package-level test seam — defaults to defaultRunner, swappable in tests.
var Runner RunnerFunc = defaultRunner
```

The `Result/Request/Transport` triple is internal — command code never constructs a `Request`. It exists so the test seam can inspect what `Run`/`RunForeground` would have done without spawning a real process.

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

`exitCode(err) (int, bool)` (`src/internal/proc/proc.go:135`) is the small helper that unwraps `*exec.ExitError` to its `ExitCode()`.

## Per-request environment (`Request.Env`, change 38a6)

`Request.Env` is an optional slice of extra `KEY=VALUE` entries to **append to** (not replace) the child's inherited environment. `defaultRunner` applies it once, before the transport switch, and only when non-empty:

```go
if len(req.Env) > 0 {
    cmd.Env = append(os.Environ(), req.Env...)
}
```

- **nil/empty `Env` → `cmd.Env` left unset** → the child inherits the full parent environment exactly as before this change. Every non-brew call (and `Run`/`RunForeground`) hits this path, so default behavior is unchanged.
- **non-empty `Env` → `cmd.Env = os.Environ() + Env`** → the child still inherits everything, plus the appended entries. Because Go's `exec` uses the **last** value for a duplicated key, an appended entry **overrides** any inherited value of the same key (this is what makes the `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` override take effect even if the parent already exports the opposite `HOMEBREW_REQUIRE_TAP_TRUST=1`).

`Env` reaches `defaultRunner` only via `RunForegroundEnv` today — command code never constructs a `Request` directly. The sole consumer is shll's brew install/upgrade/update wiring (see [cli/install](../cli/install.md) and [cli/update](../cli/update.md)); the env values come from `brewEnv()` in `src/cmd/shll/brew.go`, the single source of truth for the temporary Linux sandbox-trust workaround (paired with removal backlog `[tkch]`).

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
- `TestRunForegroundEnv_RecordsEnvAndTransport` *(change 38a6)* — `RunForegroundEnv` records a `Request` carrying both the supplied `Env` and `TransportForeground`.
- `TestRunForegroundEnv_TransportError` *(change 38a6)* — a transport error yields `(-1, err)`, mirroring `RunForeground`.
- `TestRunForeground_NoEnv` *(change 38a6)* — plain `RunForeground` records `Env: nil` (the unchanged-signature path).
- `TestDefaultRunner_EnvAppendedToParent` *(change 38a6)* — the only assertion exercising the real `defaultRunner` for env: a non-empty `Env` yields `cmd.Env = os.Environ() + Env` with appended entries winning on a duplicate key (last-wins), while an empty/nil `Env` leaves `cmd.Env` unset (inheritance untouched). A recording fake cannot prove the parent-env append, so this constructs a `Request` directly against the production runner.
- `TestDefaultRunner_RealBinary` — exercises the production path with `true`, `false`, and a missing binary; the only test that spawns real processes (and never spawns project tools).

## Cross-references

- All consumers in `src/cmd/shll/*.go` — see [cli/commands](../cli/commands.md), [cli/update](../cli/update.md), [cli/shell-init](../cli/shell-init.md), [cli/version](../cli/version.md).
- Constitution I (Security First) — the principle this package enforces.
- spec.md Design Decision #7 — package-level `Runner` is the chosen test seam.
