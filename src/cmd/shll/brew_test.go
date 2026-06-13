package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// --- brewTrustAvailable -------------------------------------------------------

func TestBrewTrustAvailable_True(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help" {
			return proc.Result{Stdout: []byte("Usage: brew trust --tap <tap>\n")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	if !brewTrustAvailable(context.Background()) {
		t.Fatal("brewTrustAvailable = false, want true when `brew trust --help` is recognized")
	}
}

func TestBrewTrustAvailable_UnrecognizedSubcommand(t *testing.T) {
	// Older brew: `trust` is unknown, so brew exits non-zero → captured error.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: errors.New("Error: Unknown command: trust")}
	}}
	installFakeRunner(t, f)
	if brewTrustAvailable(context.Background()) {
		t.Fatal("brewTrustAvailable = true, want false when `trust` is unrecognized")
	}
}

func TestBrewTrustAvailable_BrewMissing(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: proc.ErrNotFound}
	}}
	installFakeRunner(t, f)
	if brewTrustAvailable(context.Background()) {
		t.Fatal("brewTrustAvailable = true, want false when brew is absent (ErrNotFound)")
	}
}

// --- brewTrustTap -------------------------------------------------------------

func TestBrewTrustTap_BuildsTapArg(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{ExitCode: 0}
	}}
	installFakeRunner(t, f)
	code, err := brewTrustTap(context.Background())
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if code != 0 {
		t.Fatalf("code = %d, want 0", code)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, brewBinary, "trust", "--tap", tapName) {
		t.Fatalf("calls = %+v, want `brew trust --tap %s`", calls, tapName)
	}
	// Guard the tap-vs-formula distinction: the arg must be `sahil87/tap`, NOT a
	// formula reference `sahil87/tap/<formula>`.
	for _, c := range calls {
		for _, a := range c.Args {
			if strings.HasPrefix(a, formulaPrefix) && a != tapName {
				t.Fatalf("ceremony passed a formula reference %q; want the tap %q", a, tapName)
			}
		}
	}
	if tapName != "sahil87/tap" {
		t.Fatalf("tapName = %q, want \"sahil87/tap\" (no trailing slash)", tapName)
	}
}

func TestBrewTrustTap_SurfacesNonZeroExit(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{ExitCode: 1}
	}}
	installFakeRunner(t, f)
	code, err := brewTrustTap(context.Background())
	if err != nil {
		t.Fatalf("err = %v, want nil (non-zero exit is reported via code)", err)
	}
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
}

func TestBrewTrustTap_SurfacesError(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: proc.ErrNotFound}
	}}
	installFakeRunner(t, f)
	code, err := brewTrustTap(context.Background())
	if err == nil {
		t.Fatal("err = nil, want non-nil for transport failure")
	}
	if code != -1 {
		t.Fatalf("code = %d, want -1 on transport error", code)
	}
}

// --- ensureTapTrust (orchestrator) --------------------------------------------

func TestEnsureTapTrust_Success(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 1 && req.Args[0] == "--version":
			return proc.Result{Stdout: []byte("Homebrew 5.1.14\n")}
		case req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help":
			return proc.Result{Stdout: []byte("trust\n")}
		case req.Name == brewBinary && len(req.Args) == 3 && req.Args[0] == "trust":
			return proc.Result{ExitCode: 0}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	write, diag := ensureTapTrust(context.Background())
	if !write {
		t.Fatal("writeExport = false, want true on a successful ceremony")
	}
	if diag != "" {
		t.Fatalf("diag = %q, want empty on success", diag)
	}
}

func TestEnsureTapTrust_BrewMissing(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: proc.ErrNotFound}
	}}
	installFakeRunner(t, f)
	write, diag := ensureTapTrust(context.Background())
	if write {
		t.Fatal("writeExport = true, want false when brew is absent")
	}
	if !strings.Contains(diag, "HOMEBREW_NO_REQUIRE_TAP_TRUST=1") || !strings.Contains(diag, "HOMEBREW_NO_ENV_HINTS=1") {
		t.Fatalf("diag = %q, want it to name the env-var escape hatches", diag)
	}
}

func TestEnsureTapTrust_TrustUnavailable(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 1 && req.Args[0] == "--version":
			return proc.Result{Stdout: []byte("Homebrew 3.0.0\n")}
		case req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust":
			return proc.Result{Err: errors.New("Error: Unknown command: trust")} // `trust` unknown
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	write, diag := ensureTapTrust(context.Background())
	if write {
		t.Fatal("writeExport = true, want false when `brew trust` is unavailable")
	}
	if !strings.Contains(diag, "newer Homebrew") {
		t.Fatalf("diag = %q, want it to mention a newer Homebrew is required", diag)
	}
}

func TestEnsureTapTrust_CeremonyNonZero(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 1 && req.Args[0] == "--version":
			return proc.Result{Stdout: []byte("Homebrew 5.1.14\n")}
		case req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help":
			return proc.Result{Stdout: []byte("trust\n")}
		case req.Name == brewBinary && len(req.Args) == 3 && req.Args[0] == "trust":
			return proc.Result{ExitCode: 1} // ceremony fails at runtime
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	write, diag := ensureTapTrust(context.Background())
	if write {
		t.Fatal("writeExport = true, want false when the ceremony exits non-zero")
	}
	if diag == "" {
		t.Fatal("diag empty, want a ceremony-failure diagnostic")
	}
}

// --- brewEnv (Linux sandbox-trust workaround; backlog [38a6]/[tkch]) ----------

func TestBrewEnv_LinuxInjectsWorkaround(t *testing.T) {
	setOsGoos(t, "linux")
	env := brewEnv()
	if len(env) != 1 || env[0] != noRequireTapTrustEnv {
		t.Fatalf("brewEnv() on linux = %v, want [%s]", env, noRequireTapTrustEnv)
	}
}

func TestBrewEnv_DarwinReturnsNil(t *testing.T) {
	setOsGoos(t, "darwin")
	if env := brewEnv(); env != nil {
		t.Fatalf("brewEnv() on darwin = %v, want nil", env)
	}
}
