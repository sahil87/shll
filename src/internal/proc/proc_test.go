package proc

import (
	"context"
	"errors"
	"strings"
	"testing"
)

// withFakeRunner installs a fake Runner for the duration of t and restores the
// production runner afterward. The fake records every Request it receives.
func withFakeRunner(t *testing.T, behavior func(req Request) Result) *[]Request {
	t.Helper()
	prev := Runner
	t.Cleanup(func() { Runner = prev })
	var calls []Request
	Runner = func(_ context.Context, req Request) Result {
		calls = append(calls, req)
		return behavior(req)
	}
	return &calls
}

func TestRun_CaptureHappyPath(t *testing.T) {
	calls := withFakeRunner(t, func(req Request) Result {
		return Result{Stdout: []byte("hello\n")}
	})
	out, err := Run(context.Background(), "echo", "hello")
	if err != nil {
		t.Fatalf("Run() returned error: %v", err)
	}
	if string(out) != "hello\n" {
		t.Fatalf("Run() stdout = %q, want %q", string(out), "hello\n")
	}
	if len(*calls) != 1 {
		t.Fatalf("Runner call count = %d, want 1", len(*calls))
	}
	got := (*calls)[0]
	if got.Name != "echo" || len(got.Args) != 1 || got.Args[0] != "hello" {
		t.Fatalf("recorded request = %+v, want echo hello", got)
	}
	if got.Transport != TransportCapture {
		t.Fatalf("transport = %v, want TransportCapture", got.Transport)
	}
}

func TestRun_ErrNotFound(t *testing.T) {
	withFakeRunner(t, func(req Request) Result {
		return Result{Err: ErrNotFound}
	})
	_, err := Run(context.Background(), "nonesuch")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("Run() err = %v, want ErrNotFound", err)
	}
}

func TestRunForeground_ExitCode(t *testing.T) {
	withFakeRunner(t, func(req Request) Result {
		return Result{ExitCode: 7}
	})
	code, err := RunForeground(context.Background(), "fake", "arg")
	if err != nil {
		t.Fatalf("RunForeground() err = %v", err)
	}
	if code != 7 {
		t.Fatalf("RunForeground() code = %d, want 7", code)
	}
}

func TestRunForeground_ErrNotFound(t *testing.T) {
	withFakeRunner(t, func(req Request) Result {
		return Result{ExitCode: -1, Err: ErrNotFound}
	})
	code, err := RunForeground(context.Background(), "nonesuch")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("RunForeground() err = %v, want ErrNotFound", err)
	}
	if code != -1 {
		t.Fatalf("RunForeground() code = %d, want -1", code)
	}
}

func TestRunner_RecordsTransportSelection(t *testing.T) {
	calls := withFakeRunner(t, func(req Request) Result {
		if req.Transport == TransportForeground {
			return Result{ExitCode: 0}
		}
		return Result{Stdout: []byte("ok")}
	})
	if _, err := Run(context.Background(), "a"); err != nil {
		t.Fatalf("Run err: %v", err)
	}
	if _, err := RunForeground(context.Background(), "b"); err != nil {
		t.Fatalf("RunForeground err: %v", err)
	}
	if len(*calls) != 2 {
		t.Fatalf("call count = %d, want 2", len(*calls))
	}
	if (*calls)[0].Transport != TransportCapture {
		t.Fatalf("first transport = %v, want capture", (*calls)[0].Transport)
	}
	if (*calls)[1].Transport != TransportForeground {
		t.Fatalf("second transport = %v, want foreground", (*calls)[1].Transport)
	}
}

func TestRunCaptured_HappyPath(t *testing.T) {
	calls := withFakeRunner(t, func(req Request) Result {
		return Result{Stdout: []byte("bundle\n"), Stderr: nil, ExitCode: 0}
	})
	stdout, stderr, code, err := RunCaptured(context.Background(), "wt", "skill")
	if err != nil {
		t.Fatalf("RunCaptured err = %v", err)
	}
	if string(stdout) != "bundle\n" {
		t.Fatalf("stdout = %q, want %q", stdout, "bundle\n")
	}
	if len(stderr) != 0 {
		t.Fatalf("stderr = %q, want empty", stderr)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	if got := (*calls)[0].Transport; got != TransportCaptureAll {
		t.Fatalf("transport = %v, want TransportCaptureAll", got)
	}
}

func TestRunCaptured_ErrNotFound(t *testing.T) {
	withFakeRunner(t, func(req Request) Result {
		return Result{ExitCode: -1, Err: ErrNotFound}
	})
	_, _, code, err := RunCaptured(context.Background(), "nonesuch", "skill")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("err = %v, want ErrNotFound", err)
	}
	if code != -1 {
		t.Fatalf("code = %d, want -1", code)
	}
}

func TestRunCaptured_NonZeroExitCapturesBothStreams(t *testing.T) {
	// A child that predates `skill` exits non-zero and writes to its own stderr —
	// RunCaptured must surface the code with err == nil AND capture (not pass
	// through) the stderr so the caller can suppress it.
	withFakeRunner(t, func(req Request) Result {
		return Result{Stdout: nil, Stderr: []byte(`Error: unknown command "skill"` + "\n"), ExitCode: 1}
	})
	stdout, stderr, code, err := RunCaptured(context.Background(), "wt", "skill")
	if err != nil {
		t.Fatalf("err = %v, want nil (non-zero exit is not a proc error)", err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if len(stdout) != 0 {
		t.Fatalf("stdout = %q, want empty", stdout)
	}
	if !strings.Contains(string(stderr), "unknown command") {
		t.Fatalf("stderr = %q, want the child's captured error", stderr)
	}
}

// TestDefaultRunner_CaptureAllRealBinary exercises the production
// TransportCaptureAll path end-to-end: a real `sh -c` that writes to both streams
// and exits non-zero. Both buffers must be populated and neither must reach the
// parent's stdio.
func TestDefaultRunner_CaptureAllRealBinary(t *testing.T) {
	res := defaultRunner(context.Background(), Request{
		Name:      "sh",
		Args:      []string{"-c", "printf out; printf err 1>&2; exit 3"},
		Transport: TransportCaptureAll,
	})
	if res.Err != nil {
		t.Fatalf("err = %v, want nil", res.Err)
	}
	if res.ExitCode != 3 {
		t.Fatalf("code = %d, want 3", res.ExitCode)
	}
	if string(res.Stdout) != "out" {
		t.Fatalf("stdout = %q, want %q", res.Stdout, "out")
	}
	if string(res.Stderr) != "err" {
		t.Fatalf("stderr = %q, want %q", res.Stderr, "err")
	}
}

// TestDefaultRunner_RealBinary exercises the production runner end-to-end with
// `true` (always succeeds) and `false` (always exits 1) — both POSIX shell
// builtins available as standalone binaries on linux/darwin. This is the only
// test that spawns a real process; it does NOT shell out to brew or any project
// tool.
func TestDefaultRunner_RealBinary(t *testing.T) {
	res := defaultRunner(context.Background(), Request{Name: "true", Transport: TransportForeground})
	if res.Err != nil {
		t.Fatalf("defaultRunner true: err = %v", res.Err)
	}
	if res.ExitCode != 0 {
		t.Fatalf("defaultRunner true: code = %d, want 0", res.ExitCode)
	}

	res = defaultRunner(context.Background(), Request{Name: "false", Transport: TransportForeground})
	if res.Err != nil {
		t.Fatalf("defaultRunner false: err = %v", res.Err)
	}
	if res.ExitCode != 1 {
		t.Fatalf("defaultRunner false: code = %d, want 1", res.ExitCode)
	}

	res = defaultRunner(context.Background(), Request{Name: "shll-nonesuch-binary-xyz", Transport: TransportCapture})
	if !errors.Is(res.Err, ErrNotFound) {
		t.Fatalf("defaultRunner missing binary: err = %v, want ErrNotFound", res.Err)
	}
}
