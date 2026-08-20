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
	"io"
	"os"
	"os/exec"
	"sync"
)

// ErrNotFound is returned by Run/RunForeground when the named binary is not on PATH.
// Callers can match this with errors.Is to produce install-hint messages.
var ErrNotFound = errors.New("binary not found on PATH")

// Result is the structured outcome of a single subprocess invocation. Stdout carries
// captured bytes when the transport was Run; for RunForeground stdout/stderr stream
// directly to the parent and Stdout is nil. Stderr carries captured stderr bytes only
// for TransportCaptureAll (nil otherwise — Run passes stderr through, Foreground
// inherits it). Tail carries the bounded interleaved output tail only for
// TransportStreamTail (nil otherwise). ExitCode is the subprocess's exit status
// when it ran to completion;
// for RunForeground / RunCaptured transports, callers consume ExitCode to mirror the
// child's status. Run callers usually inspect Err and ignore ExitCode.
type Result struct {
	Stdout   []byte
	Stderr   []byte
	Tail     []byte
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
	// TransportCaptureAll buffers BOTH stdout and stderr into the Result (Stdout /
	// Stderr) and reports the child's exit code, without passing either stream
	// through to the parent. Used by callers that must stream a child's stdout
	// byte-identical on success and then decide, per caller, what to do with the
	// captured stderr on failure: `shll skill <tool>` suppresses it in favor of one
	// clean diagnostic, while `shll skill <tool> <topic>` propagates it verbatim
	// (the `skill` standard's unknown-topic contract must survive the composer).
	TransportCaptureAll
	// TransportStreamTail streams the child's stdout/stderr LIVE to the
	// caller-supplied writers (tee'd, never buffered-until-exit) while capturing a
	// bounded interleaved tail (the last tailRingSize bytes) into Result.Tail, and
	// runs the child with stdin from the null device (cmd.Stdin = nil — Go's
	// documented null-device behavior) so a child that attempts an interactive
	// prompt reads EOF and fails fast instead of hanging. Used by the install /
	// update write phases: the tee keeps output streaming (no withholding), the
	// tail preserves the cause of a failure whose lines scrolled out of the
	// terminal's scroll region, and the null stdin enforces the toolkit's
	// prompt-free standard.
	TransportStreamTail
)

// tailRingSize bounds the interleaved output tail retained per TransportStreamTail
// child (~4KB). The tail exists so a failed child's last lines can be re-printed
// after they scrolled out of a DECSTBM region; it is deliberately small so a
// chatty child cannot grow memory unbounded. Named per code-quality.md.
const tailRingSize = 4096

// tailRing is a fixed-capacity ring buffer capturing the most recent
// tailRingSize bytes written to it, in write order. It backs the
// TransportStreamTail interleaved tail: both child streams tee into ONE ring, so
// the tail preserves the stdout/stderr interleaving the user saw. Writes from the
// two exec copy goroutines race, so Write is mutex-guarded.
type tailRing struct {
	mu   sync.Mutex
	buf  [tailRingSize]byte
	pos  int  // next write index
	full bool // the ring has wrapped at least once (all tailRingSize bytes valid)
}

// Write appends p to the ring, evicting the oldest bytes when p exceeds the
// remaining capacity. It never fails and never blocks on a reader (io.Writer
// contract for the tee).
func (r *tailRing) Write(p []byte) (int, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	n := len(p)
	if n >= tailRingSize {
		// p alone fills the ring: keep only its tail.
		copy(r.buf[:], p[n-tailRingSize:])
		r.pos = 0
		r.full = true
		return n, nil
	}
	for _, b := range p {
		r.buf[r.pos] = b
		r.pos++
		if r.pos == tailRingSize {
			r.pos = 0
			r.full = true
		}
	}
	return n, nil
}

// Bytes returns the captured tail in write order (oldest first), copied out so
// the caller's slice is detached from the ring.
func (r *tailRing) Bytes() []byte {
	r.mu.Lock()
	defer r.mu.Unlock()
	if !r.full {
		return append([]byte(nil), r.buf[:r.pos]...)
	}
	out := make([]byte, 0, tailRingSize)
	out = append(out, r.buf[r.pos:]...)
	out = append(out, r.buf[:r.pos]...)
	return out
}

// Request describes a subprocess invocation. The binary path and explicit []string
// of arguments are passed verbatim to exec.CommandContext (Constitution I —
// no shell interpretation). Dir is optional; empty string inherits the parent cwd.
// Stdout/Stderr are the live-tee destinations used ONLY by TransportStreamTail
// (ignored by every other transport); both must be non-nil for that transport
// (the runner rejects a nil writer with a structured error rather than
// panicking in io.MultiWriter).
type Request struct {
	Name      string
	Args      []string
	Transport Transport
	Dir       string
	Stdout    io.Writer
	Stderr    io.Writer
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

// RunStreamedTail invokes name+args via TransportStreamTail: the child's
// stdout/stderr stream LIVE to the given writers (tee'd through
// io.MultiWriter — never buffered-until-exit), a bounded interleaved tail (last
// tailRingSize bytes) is captured and returned as tail, and the child's stdin is
// the null device (cmd.Stdin = nil) so an attempted interactive prompt reads EOF
// and fails fast. Exit-code semantics mirror RunForeground: when the subprocess
// runs to completion, code is its exit code and err is nil (non-zero exit is NOT
// an error); when exec fails pre-start (binary not found → ErrNotFound, dir
// missing, other I/O), code is -1 and err is non-nil.
func RunStreamedTail(ctx context.Context, stdout, stderr io.Writer, name string, args ...string) (code int, tail []byte, err error) {
	res := Runner(ctx, Request{Name: name, Args: args, Transport: TransportStreamTail, Stdout: stdout, Stderr: stderr})
	if res.Err != nil {
		return -1, res.Tail, res.Err
	}
	return res.ExitCode, res.Tail, nil
}

// RunCaptured invokes name+args capturing BOTH stdout and stderr into separate
// buffers (TransportCaptureAll) and returning the child's exit code. Neither stream
// is passed through to the parent, so the caller fully owns presentation: it can
// stream the captured stdout byte-identical on success and decide per-caller what to
// do with the captured stderr on failure. When the binary is not on PATH, err is
// ErrNotFound and code is -1. A pre-start I/O error returns a non-nil err with code
// -1. Otherwise err is nil and code is what the process yielded: its own exit status
// (0 on success, > 0 on a clean failure) when it ran to completion, or -1 when it was
// signal-killed — notably by the ctx deadline/cancel, which reports code -1 with nil
// err (Go's *exec.ExitError.ExitCode() sentinel). A caller that must distinguish a
// real non-zero exit from a deadline kill therefore treats a negative code (nil err)
// as "no usable exit status", not as a mirrorable child code.
func RunCaptured(ctx context.Context, name string, args ...string) (stdout, stderr []byte, code int, err error) {
	res := Runner(ctx, Request{Name: name, Args: args, Transport: TransportCaptureAll})
	return res.Stdout, res.Stderr, res.ExitCode, res.Err
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
	case TransportCaptureAll:
		var stdout, stderr bytes.Buffer
		cmd.Stdout = &stdout
		cmd.Stderr = &stderr
		err := cmd.Run()
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return Result{ExitCode: -1, Err: ErrNotFound}
			}
			if code, ok := exitCode(err); ok {
				// The process started and cmd.Run returned an *exec.ExitError. This
				// covers BOTH a clean non-zero exit (ExitCode() is the child's own
				// status, > 0) AND a signal kill — including the ctx deadline/cancel,
				// which SIGKILLs the child so ExitCode() is -1. err stays nil so callers
				// branch on ExitCode; a caller that must distinguish a real failure from
				// a deadline kill treats a negative code as "no usable exit status".
				return Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: code}
			}
			// A non-ExitError err: the process never started (pre-start I/O failure —
			// binary not found handled above, dir missing, etc.), so there is no child
			// exit status at all.
			return Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: -1, Err: err}
		}
		return Result{Stdout: stdout.Bytes(), Stderr: stderr.Bytes(), ExitCode: 0}
	case TransportStreamTail:
		// The tee writers are mandatory for this transport: a nil writer in
		// io.MultiWriter panics on the child's first write, so reject the
		// request up front with a structured error instead.
		if req.Stdout == nil || req.Stderr == nil {
			return Result{ExitCode: -1, Err: errors.New("proc: TransportStreamTail requires non-nil Stdout and Stderr")}
		}
		// stdin is left nil — Go documents a nil Stdin as reading from the null
		// device, so a child attempting an interactive prompt reads EOF and fails
		// fast instead of hanging the walk (prompt-free enforcement). Both output
		// streams tee LIVE to the caller's writers AND into one shared bounded
		// ring, so output is never withheld while the failure tail survives a
		// scroll region.
		ring := &tailRing{}
		cmd.Stdout = io.MultiWriter(req.Stdout, ring)
		cmd.Stderr = io.MultiWriter(req.Stderr, ring)
		err := cmd.Run()
		if err != nil {
			if errors.Is(err, exec.ErrNotFound) {
				return Result{Tail: ring.Bytes(), ExitCode: -1, Err: ErrNotFound}
			}
			if code, ok := exitCode(err); ok {
				return Result{Tail: ring.Bytes(), ExitCode: code}
			}
			return Result{Tail: ring.Bytes(), ExitCode: -1, Err: err}
		}
		return Result{Tail: ring.Bytes(), ExitCode: 0}
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
