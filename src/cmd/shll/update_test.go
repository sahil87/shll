package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// fakeRunner is a test double for proc.Runner. Each invocation is recorded;
// behavior is driven by a per-Request response function so tests can simulate
// brew presence/absence, installed/not-installed, upgrade success/failure.
type fakeRunner struct {
	calls []proc.Request
	// respond returns the Result for a given Request. Default behavior (when
	// nil) is success with no captured stdout.
	respond func(req proc.Request) proc.Result
}

func (f *fakeRunner) Runner(_ context.Context, req proc.Request) proc.Result {
	f.calls = append(f.calls, req)
	if f.respond != nil {
		return f.respond(req)
	}
	return proc.Result{}
}

// installFakeRunner swaps proc.Runner for f.Runner and restores the prior runner
// after the test.
func installFakeRunner(t *testing.T, f *fakeRunner) {
	t.Helper()
	prev := proc.Runner
	t.Cleanup(func() { proc.Runner = prev })
	proc.Runner = f.Runner
}

// invocationsContain reports whether any recorded request matches the given
// (name, args...) prefix exactly. Helper for asserting brew commands.
func invocationsContain(calls []proc.Request, name string, args ...string) bool {
	for _, c := range calls {
		if c.Name != name {
			continue
		}
		if len(c.Args) != len(args) {
			continue
		}
		match := true
		for i := range args {
			if c.Args[i] != args[i] {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

func TestUpdate_BrewMissing(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary {
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), brewMissingHint) {
		t.Fatalf("stderr = %q, want to contain %q", stderr.String(), brewMissingHint)
	}
	if invocationsContain(f.calls, brewBinary, "update", "--quiet") {
		t.Fatal("brew update should not be invoked when brew is missing")
	}
}

func TestUpdate_NoToolsInstalled(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "--version":
			return proc.Result{Stdout: []byte("Homebrew 4.0\n")}
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list":
			// Always exit non-zero — nothing installed.
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if got := stdout.String(); got != "No sahil87 tools installed.\n" {
		t.Fatalf("stdout = %q, want \"No sahil87 tools installed.\\n\"", got)
	}
	if invocationsContain(f.calls, brewBinary, "update", "--quiet") {
		t.Fatal("brew update --quiet should NOT be invoked when nothing is installed")
	}
}

func TestUpdate_AllInstalled(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		// Every brew list/--version call succeeds → every roster tool installed.
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if !invocationsContain(f.calls, brewBinary, "update", "--quiet") {
		t.Fatalf("expected brew update --quiet, calls: %+v", f.calls)
	}
	for _, tool := range Roster {
		if !invocationsContain(f.calls, brewBinary, "upgrade", tool.Formula) {
			t.Errorf("expected brew upgrade %s, calls: %+v", tool.Formula, f.calls)
		}
	}
}

func TestUpdate_PartialInstalled(t *testing.T) {
	// Only hop and wt are installed.
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
	if err := runUpdate(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runUpdate err = %v", err)
	}
	if !invocationsContain(f.calls, brewBinary, "upgrade", formulaPrefix+"hop") {
		t.Error("expected brew upgrade for hop")
	}
	if !invocationsContain(f.calls, brewBinary, "upgrade", formulaPrefix+"wt") {
		t.Error("expected brew upgrade for wt")
	}
	if invocationsContain(f.calls, brewBinary, "upgrade", formulaPrefix+"idea") {
		t.Error("brew upgrade for idea (uninstalled) should NOT be invoked")
	}
	if invocationsContain(f.calls, brewBinary, "upgrade", formulaPrefix+"fab-kit") {
		t.Error("brew upgrade for fab-kit (uninstalled) should NOT be invoked")
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr should be empty for graceful degradation, got %q", stderr.String())
	}
}

func TestUpdate_OneUpgradeFails(t *testing.T) {
	// All installed; first upgrade fails, second succeeds. Exit non-zero,
	// continue through the roster.
	upgradeCalls := 0
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 1 && req.Args[0] == "upgrade" {
			upgradeCalls++
			if upgradeCalls == 1 {
				return proc.Result{ExitCode: 1}
			}
			return proc.Result{ExitCode: 0}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent (overall failure)", err)
	}
	// All six roster tools should have been attempted despite the first failure.
	gotUpgrades := 0
	for _, c := range f.calls {
		if c.Name == brewBinary && len(c.Args) >= 1 && c.Args[0] == "upgrade" {
			gotUpgrades++
		}
	}
	if gotUpgrades != len(Roster) {
		t.Fatalf("upgrade attempts = %d, want %d (must continue through failure)", gotUpgrades, len(Roster))
	}
}
