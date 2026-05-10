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
		// Every brew list/--version call succeeds → shll itself plus every
		// roster tool are all installed.
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
	if !invocationsContain(f.calls, brewBinary, "upgrade", shllFormula) {
		t.Fatalf("expected self-upgrade brew upgrade %s, calls: %+v", shllFormula, f.calls)
	}
	for _, tool := range Roster {
		if !invocationsContain(f.calls, brewBinary, "upgrade", tool.Formula) {
			t.Errorf("expected brew upgrade %s, calls: %+v", tool.Formula, f.calls)
		}
	}
}

func TestUpdate_SelfUpgradeOrdering(t *testing.T) {
	// shll self-upgrade must run BEFORE the roster loop so a follow-up
	// invocation picks up the new binary.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		// shll itself + full roster all installed.
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runUpdate err = %v", err)
	}

	// Find the indices of the shll self-upgrade and the first roster upgrade
	// (fab-kit, the first roster entry) in the recorded call sequence.
	selfIdx, firstRosterIdx := -1, -1
	for i, c := range f.calls {
		if c.Name != brewBinary || len(c.Args) < 2 || c.Args[0] != "upgrade" {
			continue
		}
		switch c.Args[1] {
		case shllFormula:
			if selfIdx == -1 {
				selfIdx = i
			}
		case Roster[0].Formula:
			if firstRosterIdx == -1 {
				firstRosterIdx = i
			}
		}
	}
	if selfIdx == -1 || firstRosterIdx == -1 {
		t.Fatalf("missing expected upgrade calls (self=%d, firstRoster=%d), calls: %+v", selfIdx, firstRosterIdx, f.calls)
	}
	if selfIdx >= firstRosterIdx {
		t.Fatalf("shll self-upgrade index %d must be < first roster upgrade index %d", selfIdx, firstRosterIdx)
	}
}

func TestUpdate_SelfNotBrewInstalled(t *testing.T) {
	// Dev build (e.g. `go install`) — shll itself is not brew-installed.
	// shll update must skip the self-upgrade silently and still upgrade the
	// roster.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			if req.Args[3] == shllFormula {
				return proc.Result{Err: errors.New("not installed")}
			}
			return proc.Result{Stdout: []byte(req.Args[3] + " 1.0.0\n")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if invocationsContain(f.calls, brewBinary, "upgrade", shllFormula) {
		t.Fatal("brew upgrade for shll should NOT run when shll itself isn't brew-installed")
	}
	// Roster upgrades still happen.
	for _, tool := range Roster {
		if !invocationsContain(f.calls, brewBinary, "upgrade", tool.Formula) {
			t.Errorf("expected brew upgrade %s", tool.Formula)
		}
	}
}

func TestUpdate_OnlyShllInstalled(t *testing.T) {
	// shll itself installed via brew, but no roster tools installed. shll
	// update must still self-upgrade and exit 0 — the previous "No sahil87
	// tools installed." short-circuit no longer fires when shll is brewed.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			if req.Args[3] == shllFormula {
				return proc.Result{Stdout: []byte(shllFormula + " 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if !invocationsContain(f.calls, brewBinary, "update", "--quiet") {
		t.Fatal("expected brew update --quiet to run when shll is brewed even with no roster tools")
	}
	if !invocationsContain(f.calls, brewBinary, "upgrade", shllFormula) {
		t.Fatal("expected brew upgrade for shll itself")
	}
	// No roster upgrades.
	for _, tool := range Roster {
		if invocationsContain(f.calls, brewBinary, "upgrade", tool.Formula) {
			t.Errorf("brew upgrade for uninstalled %s should NOT run", tool.Formula)
		}
	}
	if strings.Contains(stdout.String(), "No sahil87 tools installed") {
		t.Errorf("short-circuit message should NOT print when shll itself is brewed, got %q", stdout.String())
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

func TestUpdate_BrewUpdateFails(t *testing.T) {
	// brew update --quiet exits non-zero (with nil err — see proc.RunForeground
	// contract). shll must surface this as failure rather than silently
	// continuing into the upgrade loop.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 1 && req.Args[0] == "update" {
			return proc.Result{ExitCode: 1}
		}
		// Everything else (brew --version, brew list, brew upgrade) succeeds —
		// keeps the test focused on the brew-update branch.
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent (brew update non-zero exit)", err)
	}
	if !strings.Contains(stderr.String(), "brew update failed") {
		t.Fatalf("stderr = %q, want to contain \"brew update failed\"", stderr.String())
	}
	if invocationsContain(f.calls, brewBinary, "upgrade", formulaPrefix+"hop") {
		t.Fatal("brew upgrade should NOT be invoked after brew update fails")
	}
}

func TestUpdate_OneUpgradeFails(t *testing.T) {
	// All installed (including shll itself); the first roster upgrade fails;
	// the rest must still be attempted. Exit non-zero overall.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 2 && req.Args[0] == "upgrade" {
			// Fail only the first roster entry. Self-upgrade (shll) and the
			// rest of the roster succeed.
			if req.Args[1] == Roster[0].Formula {
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
	// Self-upgrade + every roster entry must have been attempted despite the
	// roster[0] failure — best-effort policy.
	gotUpgrades := 0
	for _, c := range f.calls {
		if c.Name == brewBinary && len(c.Args) >= 1 && c.Args[0] == "upgrade" {
			gotUpgrades++
		}
	}
	want := len(Roster) + 1 // +1 for the shll self-upgrade
	if gotUpgrades != want {
		t.Fatalf("upgrade attempts = %d, want %d (self + roster, must continue through failure)", gotUpgrades, want)
	}
}
