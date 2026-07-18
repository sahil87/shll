---
type: memory
description: "Centralized subprocess wrapper — `Run` (capture stdout, pass stderr through), `RunForeground` (inherited stdio), `RunCaptured` (capture BOTH streams + exit code, pass neither through — change agst), `ErrNotFound` sentinel, `Runner` test seam."
---
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

// RunCaptured captures BOTH stdout and stderr into separate buffers and reports
// the child's exit code, passing NEITHER stream through to the parent. (change agst)
// ErrNotFound (code -1) when the binary is missing; (err == nil, code) when the
// child ran to completion (non-zero code surfaced, NOT an error); (err, -1) on a
// pre-start I/O failure.
func RunCaptured(ctx context.Context, name string, args ...string) (stdout, stderr []byte, code int, err error)
```

That is the entire surface command code uses. Callers never import `os/exec` directly. The child always inherits the parent environment as-is — there is **no per-request environment override**.

> **History — the reverted 38a6 env plumbing (change 0854).** Change 38a6 had briefly added a third variant, `RunForegroundEnv(ctx, env []string, name string, args ...string)`, plus a `Request.Env` field and an env-append branch in `defaultRunner`, so shll's brew call sites could inject a Linux-only `HOMEBREW_NO_REQUIRE_TAP_TRUST=1` sandbox-trust workaround. That workaround was **removed in change 0854** (the upstream Homebrew bug is fixed in 6.0.4; per-formula trust in `shll install` is the correct DX), and since the workaround was its **only** consumer, the entire env plumbing was **stripped** — `proc` is back to its pre-38a6 `Run`/`RunForeground` surface. See [cli/install](/cli/install.md#per-formula-trust-before-install-change-0854) and [cli/update §removal of the 38a6 workaround](/cli/update.md#removal-of-the-38a6-linux-workaround-change-0854).

## Internal types

```go
type Result struct {
    Stdout   []byte
    Stderr   []byte // populated ONLY by TransportCaptureAll (nil otherwise); change agst
    ExitCode int
    Err      error
}

type Transport int
const (
    TransportCapture    Transport = iota // buffer stdout; pass stderr THROUGH to parent
    TransportForeground                  // inherit stdin/stdout/stderr
    TransportCaptureAll                  // buffer BOTH streams; pass NEITHER through (change agst)
)

type Request struct {
    Name      string
    Args      []string
    Transport Transport
    Dir       string  // optional working dir; "" inherits parent cwd
}

type RunnerFunc func(ctx context.Context, req Request) Result

// The package-level test seam — defaults to defaultRunner, swappable in tests.
var Runner RunnerFunc = defaultRunner
```

The `Result/Request/Transport` triple is internal — command code never constructs a `Request`. It exists so the test seam can inspect what `Run`/`RunForeground`/`RunCaptured` would have done without spawning a real process.

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

The fake records every `Request` it receives and returns canned `Result` values (matched by binary name + args). This is how the `src/cmd/shll/*_test.go` files avoid spawning real `brew` or per-tool subprocesses — including `skill_test.go` (fakes `<tool> skill` via `TransportCaptureAll`) and `agent_setup_test.go` (fakes the `run-kit agent-setup` delegation), both added by change agst.

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

`exitCode(err) (int, bool)` (`src/internal/proc/proc.go`) is the small helper that unwraps `*exec.ExitError` to its `ExitCode()` — shared by the `TransportForeground` and `TransportCaptureAll` branches.

### `TransportCaptureAll` (used by `proc.RunCaptured`) — change agst

- `cmd.Stdout = &stdout`, `cmd.Stderr = &stderr` — **both** captured into `Result.Stdout` / `Result.Stderr`; **neither** passed through to the parent (unlike `TransportCapture`, which passes stderr through).
- On `exec.ErrNotFound` → `Result{ExitCode: -1, Err: ErrNotFound}`.
- On `*exec.ExitError` (child ran and exited non-zero) → `Result{Stdout, Stderr, ExitCode: <code>}` with **`Err == nil`** — the caller branches on the non-zero code, not on an error (same contract as `TransportForeground`, but with the captured output attached).
- On any other error (I/O failure pre-spawn) → `Result{Stdout, Stderr, ExitCode: -1, Err: err}`.
- On success → `Result{Stdout, Stderr, ExitCode: 0}`.

**Why a third transport, not `TransportCapture` + a stderr tweak.** The two existing transports both leak the child's stderr to the parent (Capture passes it through; Foreground inherits it). `shll skill <tool>` needs the opposite: it must stream the child's stdout **byte-identical on success** AND **suppress the child's own stderr on failure** so it can emit its own clean one-line notice (a tool that predates `skill` prints an unknown-command error to stderr that the user should never see). Capturing both streams and passing neither through is the only combination that lets the caller fully own presentation. `RunCaptured` is the sole consumer today (see [cli/skill §the byte-identical passthrough](/cli/skill.md#the-byte-identical-passthrough-procruncaptured)).

### `RunCaptured` / `TransportCaptureAll` (change agst)

`RunCaptured(ctx, name, args...) (stdout, stderr []byte, code int, err error)` is the public helper over `TransportCaptureAll`. It returns four values (not `([]byte, error)` like `Run`, nor `(int, error)` like `RunForeground`) because its callers need all of: the captured stdout to stream on success, the (suppressed) stderr, the exit code to classify a non-zero-but-completed child, and the error to distinguish `ErrNotFound`. The `Result.Stderr` field was added to the struct specifically for this transport — it is `nil` for `Run`/`RunForeground`. The `_` discard in `shll skill`'s call (`out, _, code, err := proc.RunCaptured(...)`) is deliberate: skill suppresses the child's stderr rather than reading it.

## No per-request environment override

`defaultRunner` never sets `cmd.Env` — the child always inherits the full parent environment as-is. There is no env-carrying variant on the public surface (the 38a6 `RunForegroundEnv`/`Request.Env`/env-append plumbing was reverted in change 0854 when its sole consumer, the Linux sandbox-trust workaround, was removed — see the History note under [Public API](#public-api)). If a future caller needs to inject an env override, it would re-add a `Request.Env` field + an `append(os.Environ(), req.Env...)` branch (last value wins on a duplicate key) — but nothing needs it today.

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

The 38a6 env tests (`TestRunForegroundEnv_RecordsEnvAndTransport`, `TestRunForegroundEnv_TransportError`, `TestRunForeground_NoEnv`, `TestDefaultRunner_EnvAppendedToParent`) were removed with the `Env` plumbing (change 0854).

## Cross-references

- All consumers in `src/cmd/shll/*.go` — see [cli/commands](/cli/commands.md), [cli/update](/cli/update.md), [cli/shell-init](/cli/shell-init.md), [cli/version](/cli/version.md).
- The sole `RunCaptured`/`TransportCaptureAll` consumer (byte-identical stdout passthrough + suppressed stderr): [cli/skill §the byte-identical passthrough](/cli/skill.md#the-byte-identical-passthrough-procruncaptured). The `run-kit agent-setup` delegation uses `RunForeground`: [cli/agent-setup §run-kit delegation](/cli/agent-setup.md#run-kit-delegation).
- Constitution I (Security First) — the principle this package enforces.
- spec.md Design Decision #7 — package-level `Runner` is the chosen test seam.
