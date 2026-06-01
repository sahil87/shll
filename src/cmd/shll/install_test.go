package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

func TestInstall_BrewMissing(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary {
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runInstall(context.Background(), &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runInstall err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), installBrewMissingHint) {
		t.Fatalf("stderr = %q, want to contain %q", stderr.String(), installBrewMissingHint)
	}
	// The install-specific hint must say "shll install", not "shll update" —
	// using the update-specific hint here would mislead users about which
	// command produced the error.
	if strings.Contains(stderr.String(), "shll update requires") {
		t.Fatalf("stderr = %q, must not contain update-specific hint from `shll install`", stderr.String())
	}
	if invocationsContain(f.calls, brewBinary, "install", formulaPrefix+"hop") {
		t.Fatal("brew install should not be invoked when brew is missing")
	}
}

func TestInstall_AllAlreadyInstalled(t *testing.T) {
	// Every brew list/--version succeeds → every roster tool already installed.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	if got := stdout.String(); got != "All sahil87 tools already installed.\n" {
		t.Fatalf("stdout = %q, want \"All sahil87 tools already installed.\\n\"", got)
	}
	for _, tool := range Roster {
		if invocationsContain(f.calls, brewBinary, "install", tool.Formula) {
			t.Errorf("brew install for %s should NOT be invoked when already installed", tool.Formula)
		}
	}
}

func TestInstall_NoneInstalled(t *testing.T) {
	// brew --version succeeds but every brew list exits non-zero → install all.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "--version":
			return proc.Result{Stdout: []byte("Homebrew 4.0\n")}
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list":
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	for _, tool := range Roster {
		if !invocationsContain(f.calls, brewBinary, "install", tool.Formula) {
			t.Errorf("expected brew install %s, calls: %+v", tool.Formula, f.calls)
		}
	}
	// Sanity: stdout should NOT contain the "all already installed" message.
	if strings.Contains(stdout.String(), "already installed") {
		t.Errorf("stdout should not announce already-installed when nothing is installed, got %q", stdout.String())
	}
}

func TestInstall_PartialInstalled(t *testing.T) {
	// hop and wt are already installed; the other four must be installed.
	installedFormulas := map[string]bool{
		formulaPrefix + "hop": true,
		formulaPrefix + "wt":  true,
	}
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			formula := req.Args[3]
			if installedFormulas[formula] {
				return proc.Result{Stdout: []byte(formula + " 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runInstall err = %v", err)
	}
	// Already-installed tools must NOT receive an install call.
	if invocationsContain(f.calls, brewBinary, "install", formulaPrefix+"hop") {
		t.Error("brew install for already-installed hop should NOT be invoked")
	}
	if invocationsContain(f.calls, brewBinary, "install", formulaPrefix+"wt") {
		t.Error("brew install for already-installed wt should NOT be invoked")
	}
	// Missing tools MUST receive an install call.
	for _, formula := range []string{
		formulaPrefix + "fab-kit",
		formulaPrefix + "rk",
		formulaPrefix + "tu",
		formulaPrefix + "idea",
	} {
		if !invocationsContain(f.calls, brewBinary, "install", formula) {
			t.Errorf("expected brew install %s", formula)
		}
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr should be empty, got %q", stderr.String())
	}
}

func TestInstall_NoBrewUpdateInvoked(t *testing.T) {
	// `shll install` does NOT run `brew update --quiet` — install resolves
	// formulas via the tap directly. Pin this behavior to prevent drift toward
	// `shll update`'s metadata-refresh shape.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list" {
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runInstall err = %v", err)
	}
	if invocationsContain(f.calls, brewBinary, "update", "--quiet") {
		t.Fatal("brew update --quiet should NOT be invoked from shll install")
	}
}

func TestInstall_OneInstallFails(t *testing.T) {
	// All missing; first install fails, the rest must still be attempted.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list":
			return proc.Result{Err: errors.New("not installed")}
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "install":
			// Fail the first roster install (fab-kit), succeed the rest.
			if len(req.Args) >= 2 && req.Args[1] == formulaPrefix+"fab-kit" {
				return proc.Result{ExitCode: 1}
			}
			return proc.Result{ExitCode: 0}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runInstall(context.Background(), &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runInstall err = %v, want errSilent (overall failure)", err)
	}
	gotInstalls := 0
	for _, c := range f.calls {
		if c.Name == brewBinary && len(c.Args) >= 1 && c.Args[0] == "install" {
			gotInstalls++
		}
	}
	if gotInstalls != len(Roster) {
		t.Fatalf("install attempts = %d, want %d (must continue through failure)", gotInstalls, len(Roster))
	}
}

func TestInstall_HeadersAndTail(t *testing.T) {
	// hop and wt already installed; the other four are missing. With a
	// bytes.Buffer (non-TTY) stdout, the helper takes the plain branch: a
	// `==> <tool>` header precedes each missing tool's install (roster order),
	// then the all-succeeded tail. The fake records calls but writes no bytes, so
	// stdout is exactly shll's own framing.
	f := &fakeRunner{respond: installedOnly(formulaPrefix+"hop", formulaPrefix+"wt")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	want := "==> idea\n==> tu\n==> rk\n==> fab-kit\nDone — 4 of 4 tools succeeded.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	// Stream discipline: framing goes to stdout, never stderr.
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty (framing must not go to stderr)", stderr.String())
	}
}

func TestInstall_EmptyCaseNoHeaderNoTail(t *testing.T) {
	// Everything already installed → short-circuit, no install loop, so no
	// header and no tail. Golden string unchanged (the one-line note).
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	if got := stdout.String(); got != "All sahil87 tools already installed.\n" {
		t.Fatalf("stdout = %q, want the one-line note only (no header, no tail)", got)
	}
	if strings.Contains(stdout.String(), "==>") || strings.Contains(stdout.String(), "Done —") {
		t.Fatalf("empty case must emit no header and no tail, got %q", stdout.String())
	}
}

func TestInstall_PartialFailureTail(t *testing.T) {
	// All six missing; fab-kit's install fails, the rest succeed → partial-failure
	// tail with counts 5 succeeded, 1 failed. Exit stays errSilent.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list":
			return proc.Result{Err: errors.New("not installed")}
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "install":
			if len(req.Args) >= 2 && req.Args[1] == formulaPrefix+"fab-kit" {
				return proc.Result{ExitCode: 1}
			}
			return proc.Result{ExitCode: 0}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runInstall(context.Background(), &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runInstall err = %v, want errSilent (one install failed)", err)
	}
	if !strings.HasSuffix(stdout.String(), "5 succeeded, 1 failed — see above.\n") {
		t.Fatalf("stdout = %q, want to end with partial-failure tail (5/1)", stdout.String())
	}
}
