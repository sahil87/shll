// Package proc is the centralized subprocess-execution wrapper for the shll binary.
// All external-tool invocations (brew, hop, wt, ...) MUST go through this package —
// Constitution Principle I (Security First) requires this. No package outside
// internal/proc may import os/exec directly.
//
// The package exposes two transports — Run (captured stdout, ErrNotFound on missing
// binary) and RunForeground (inherited stdio, exit code reporting) — plus an
// indirection (the package-level Runner variable) that tests can swap out for a fake
// recorder. This is the test seam mandated by spec Design Decision #7: command code
// always calls into this package, and tests inject behavior here rather than spawning
// real subprocesses.
//
// The package surface is intentionally minimal: the binary name plus an explicit
// []string of arguments, never a shell-interpreted command string, and no
// per-request environment override (the child always inherits the parent
// environment as-is).
package proc

import (
	"bytes"
	"context"
	"errors"
	"os"
	"os/exec"
)

// ErrNotFound is returned by Run/RunForeground when the named binary is not on PATH.
// Callers can match this with errors.Is to produce install-hint messages.
var ErrNotFound = errors.New("binary not found on PATH")

// Result is the structured outcome of a single subprocess invocation. Stdout carries
// captured bytes when the transport was Run; for RunForeground stdout/stderr stream
// directly to the parent and Stdout is nil. ExitCode is the subprocess's exit status
// when it ran to completion; for RunForeground transports, callers consume ExitCode
// to mirror the child's status. Run callers usually inspect Err and ignore ExitCode.
type Result struct {
	Stdout   []byte
	ExitCode int
	Err      error
}

// RunnerFunc is the signature of the package-level Runner indirection. It receives
// a fully-built request (binary, args, transport, optional working dir) and returns
// a Result. Tests assign a fake to Runner to record invocations without spawning a
// real subprocess.
type RunnerFunc func(ctx context.Context, req Request) Result

// Transport selects between captured-output and inherited-stdio modes.
type Transport int

const (
	// TransportCapture buffers stdout into Result.Stdout while passing stderr
	// through to the parent. Used for queries shll consumes (brew list, brew info,
	// per-tool --version, per-tool shell-init).
	TransportCapture Transport = iota
	// TransportForeground inherits stdin/stdout/stderr from the parent. Used for
	// commands whose output the user should see directly (brew update, brew upgrade).
	TransportForeground
)

// Request describes a subprocess invocation. The binary path and explicit []string
// of arguments are passed verbatim to exec.CommandContext (Constitution I —
// no shell interpretation). Dir is optional; empty string inherits the parent cwd.
type Request struct {
	Name      string
	Args      []string
	Transport Transport
	Dir       string
}

// Runner is the indirection that tests swap to inject fakes. The default
// implementation (defaultRunner) actually spawns subprocesses via os/exec.
var Runner RunnerFunc = defaultRunner

// Run captures stdout from name+args using TransportCapture. stderr passes through
// to the parent's stderr so subprocess error messages reach the user. If the binary
// is not on PATH, the returned error is ErrNotFound (callers can match it directly
// or via errors.Is).
func Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	res := Runner(ctx, Request{Name: name, Args: args, Transport: TransportCapture})
	return res.Stdout, res.Err
}

// RunForeground invokes name+args with stdin/stdout/stderr inherited from the parent.
// The exit code of the subprocess is returned via the (code, error) pair: when the
// subprocess runs to completion, code is its exit code and error is nil. When exec
// fails before the subprocess starts (binary not found, dir does not exist, or other
// I/O error), code is -1 and error is non-nil. Use errors.Is(err, ErrNotFound) to
// detect missing binary.
func RunForeground(ctx context.Context, name string, args ...string) (int, error) {
	res := Runner(ctx, Request{Name: name, Args: args, Transport: TransportForeground})
	if res.Err != nil {
		return -1, res.Err
	}
	return res.ExitCode, nil
}

// defaultRunner is the production implementation of RunnerFunc. It spawns a real
// subprocess via exec.CommandContext (always — no exec.Command without ctx).
func defaultRunner(ctx context.Context, req Request) Result {
	cmd := exec.CommandContext(ctx, req.Name, req.Args...)
	if req.Dir != "" {
		cmd.Dir = req.Dir
	}
	switch req.Transport {
	case TransportCapture:
		var stdout bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return Result{Err: ErrNotFound}
			}
			return Result{Stdout: stdout.Bytes(), Err: err}
		}
		return Result{Stdout: stdout.Bytes()}
	case TransportForeground:
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		err := cmd.Run()
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return Result{ExitCode: -1, Err: ErrNotFound}
			}
			if code, ok := exitCode(err); ok {
				return Result{ExitCode: code}
			}
			return Result{ExitCode: -1, Err: err}
		}
		return Result{ExitCode: 0}
	default:
		return Result{ExitCode: -1, Err: errors.New("proc: unknown transport")}
	}
}

// exitCode reports the subprocess exit code carried by err. It returns (code, true)
// when err wraps an *exec.ExitError, and (0, false) otherwise.
func exitCode(err error) (int, bool) {
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode(), true
	}
	return 0, false
}
