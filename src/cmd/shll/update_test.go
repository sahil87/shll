package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/sahil87/shll/internal/changelog"
	"github.com/sahil87/shll/internal/proc"
)

// fakeRunner is a test double for proc.Runner. Each invocation is recorded;
// behavior is driven by a per-Request response function so tests can simulate
// brew presence/absence, installed/not-installed, upgrade success/failure.
//
// runUpdate now dispatches its read-only capability probes concurrently, so the
// fake must be safe for concurrent calls — mu guards both the calls slice and the
// respond dispatch.
type fakeRunner struct {
	mu    sync.Mutex
	calls []proc.Request
	// respond returns the Result for a given Request. Default behavior (when
	// nil) is success with no captured stdout. Invoked under mu, so respond
	// functions must not call back into the runner.
	respond func(req proc.Request) proc.Result
}

func (f *fakeRunner) Runner(_ context.Context, req proc.Request) proc.Result {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, req)
	if f.respond != nil {
		return f.respond(req)
	}
	return proc.Result{}
}

// recordedCalls returns a snapshot copy of the recorded requests, taken under mu.
// Tests call this after runUpdate returns (probes have joined) to assert against a
// stable slice without racing the fake's internal state.
func (f *fakeRunner) recordedCalls() []proc.Request {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]proc.Request, len(f.calls))
	copy(out, f.calls)
	return out
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
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), brewMissingHint) {
		t.Fatalf("stderr = %q, want to contain %q", stderr.String(), brewMissingHint)
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "update", "--quiet") {
		t.Fatal("brew update should not be invoked when brew is missing")
	}
	// The status line is NOT printed before the brew-missing bail-out — brew
	// presence is checked first.
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty when brew is missing", stdout.String())
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
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	// The status line prints first (unconditionally, before the short-circuit),
	// then the nothing-to-do message.
	wantOut := updateStatusLine + "\nNo shll tools installed.\n"
	if got := stdout.String(); got != wantOut {
		t.Fatalf("stdout = %q, want %q", got, wantOut)
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "update", "--quiet") {
		t.Fatal("brew update --quiet should NOT be invoked when nothing is installed")
	}
}

// helpAdvertisesSkipFlag returns help output containing the --skip-brew-update
// substring, so a probed tool reports flag support. Used by respond functions to
// drive the "supports the flag" path.
func helpAdvertisesSkipFlag() proc.Result {
	return proc.Result{Stdout: []byte("Usage: update [flags]\n  --skip-brew-update  skip brew update\n")}
}

// isUpdateHelpProbe reports whether req is a `<tool> update --help` capability
// probe (captured transport). The probe argv is the tool's Update[1:] followed by
// --help; for the current roster that is exactly ["update", "--help"].
func isUpdateHelpProbe(req proc.Request) bool {
	return len(req.Args) >= 1 && req.Args[len(req.Args)-1] == "--help"
}

func TestUpdate_AllInstalled(t *testing.T) {
	// Every brew list/--version call succeeds → shll itself plus every roster
	// tool are all installed. Help probes return empty stdout (no
	// --skip-brew-update), so each tool delegates to `<tool> update` with no
	// flag.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, brewBinary, "update", "--quiet") {
		t.Fatalf("expected brew update --quiet, calls: %+v", calls)
	}
	if !invocationsContain(calls, brewBinary, "upgrade", shllFormula) {
		t.Fatalf("expected self-upgrade brew upgrade %s, calls: %+v", shllFormula, calls)
	}
	// Each roster tool is upgraded via its own `update` (no flag), and NOT via
	// brew upgrade <formula>.
	for _, tool := range Roster {
		if !invocationsContain(calls, tool.Update[0], tool.Update[1]) {
			t.Errorf("expected delegated %s update, calls: %+v", tool.Name, calls)
		}
		if invocationsContain(calls, brewBinary, "upgrade", tool.Formula) {
			t.Errorf("did NOT expect brew upgrade %s — should delegate to `%s update`", tool.Formula, tool.Name)
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
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v", err)
	}

	// Find the indices of the shll self-upgrade (`brew upgrade shllFormula`) and
	// the first roster upgrade (Roster[0], now wt, delegated to `wt update`) in
	// the recorded call sequence. The first roster upgrade is a delegated
	// `<tool> update` invocation, not a brew upgrade.
	calls := f.recordedCalls()
	first := Roster[0]
	selfIdx, firstRosterIdx := -1, -1
	for i, c := range calls {
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "upgrade" && c.Args[1] == shllFormula {
			if selfIdx == -1 {
				selfIdx = i
			}
			continue
		}
		// The delegated upgrade is `<tool> update[ --skip-brew-update]` — exclude
		// the concurrent `<tool> update --help` capability probe.
		if c.Name == first.Update[0] && len(c.Args) >= 1 && c.Args[0] == first.Update[1] && !isUpdateHelpProbe(c) {
			if firstRosterIdx == -1 {
				firstRosterIdx = i
			}
		}
	}
	if selfIdx == -1 || firstRosterIdx == -1 {
		t.Fatalf("missing expected upgrade calls (self=%d, firstRoster=%d), calls: %+v", selfIdx, firstRosterIdx, calls)
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
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if invocationsContain(calls, brewBinary, "upgrade", shllFormula) {
		t.Fatal("brew upgrade for shll should NOT run when shll itself isn't brew-installed")
	}
	// Roster upgrades still happen — delegated to each tool's own `update`.
	for _, tool := range Roster {
		if !invocationsContain(calls, tool.Update[0], tool.Update[1]) {
			t.Errorf("expected delegated %s update", tool.Name)
		}
	}
}

func TestUpdate_OnlyShllInstalled(t *testing.T) {
	// shll itself installed via brew, but no roster tools installed. shll
	// update must still self-upgrade and exit 0 — the previous "No shll
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
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, brewBinary, "update", "--quiet") {
		t.Fatal("expected brew update --quiet to run when shll is brewed even with no roster tools")
	}
	if !invocationsContain(calls, brewBinary, "upgrade", shllFormula) {
		t.Fatal("expected brew upgrade for shll itself")
	}
	// No roster upgrades — neither brew upgrade nor delegated `<tool> update`.
	for _, tool := range Roster {
		if invocationsContain(calls, brewBinary, "upgrade", tool.Formula) {
			t.Errorf("brew upgrade for uninstalled %s should NOT run", tool.Formula)
		}
		if invocationsContain(calls, tool.Update[0], tool.Update[1]) {
			t.Errorf("delegated update for uninstalled %s should NOT run", tool.Name)
		}
	}
	if strings.Contains(stdout.String(), "No shll tools installed") {
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
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v", err)
	}
	calls := f.recordedCalls()
	// hop and wt are upgraded via their own `update` (help advertises no flag).
	if !invocationsContain(calls, "hop", "update") {
		t.Error("expected delegated hop update")
	}
	if !invocationsContain(calls, "wt", "update") {
		t.Error("expected delegated wt update")
	}
	// Uninstalled tools: neither delegated update nor brew-upgrade fallback.
	if invocationsContain(calls, "idea", "update") || invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"idea") {
		t.Error("idea (uninstalled) should NOT be upgraded")
	}
	if invocationsContain(calls, "fab-kit", "update") || invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"fab-kit") {
		t.Error("fab-kit (uninstalled) should NOT be upgraded")
	}
	// The --help capability probe is issued only for installed tools.
	if !invocationsContain(calls, "hop", "update", "--help") {
		t.Error("expected hop update --help probe (hop is installed)")
	}
	if !invocationsContain(calls, "wt", "update", "--help") {
		t.Error("expected wt update --help probe (wt is installed)")
	}
	if invocationsContain(calls, "idea", "update", "--help") {
		t.Error("idea update --help should NOT be probed (idea is not installed)")
	}
	if invocationsContain(calls, "fab-kit", "update", "--help") {
		t.Error("fab-kit update --help should NOT be probed (fab-kit is not installed)")
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
		// Match `brew update --quiet` specifically (not a `<tool> update --help`
		// capability probe, whose Name is the tool binary, not brew).
		if req.Name == brewBinary && len(req.Args) >= 1 && req.Args[0] == "update" {
			return proc.Result{ExitCode: 1}
		}
		// Everything else (brew --version, brew list, capability probes,
		// upgrades) succeeds — keeps the test focused on the brew-update branch.
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent (brew update non-zero exit)", err)
	}
	if !strings.Contains(stderr.String(), "brew update failed") {
		t.Fatalf("stderr = %q, want to contain \"brew update failed\"", stderr.String())
	}
	calls := f.recordedCalls()
	// No upgrade — neither delegated `<tool> update` nor brew-upgrade fallback —
	// is attempted after the metadata refresh fails.
	if invocationsContain(calls, "hop", "update") || invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"hop") {
		t.Fatal("no upgrade should be invoked after brew update fails")
	}
}

func TestUpdate_OneUpgradeFails(t *testing.T) {
	// All installed (including shll itself); the first roster tool's delegated
	// `update` fails; the rest must still be attempted. Exit non-zero overall.
	first := Roster[0]
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		// Fail only the first roster entry's delegated update (not its --help
		// probe). Self-upgrade (brew upgrade shll) and the rest of the roster
		// succeed.
		if req.Name == first.Update[0] && len(req.Args) == 1 && req.Args[0] == first.Update[1] {
			return proc.Result{ExitCode: 1}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent (overall failure)", err)
	}
	calls := f.recordedCalls()
	// Self-upgrade (brew upgrade) + every roster entry's delegated `update` must
	// have been attempted despite the roster[0] failure — best-effort policy.
	// Count brew-upgrade calls and delegated `<tool> update` calls (excluding the
	// `--help` capability probes).
	gotUpgrades := 0
	for _, c := range calls {
		switch {
		case c.Name == brewBinary && len(c.Args) >= 1 && c.Args[0] == "upgrade":
			gotUpgrades++
		case len(c.Args) == 1 && c.Args[0] == "update":
			gotUpgrades++
		}
	}
	want := len(Roster) + 1 // +1 for the shll self-upgrade (brew upgrade)
	if gotUpgrades != want {
		t.Fatalf("upgrade attempts = %d, want %d (self + roster, must continue through failure)", gotUpgrades, want)
	}
}

// allInstalledResponder models a REALISTIC fully-installed, migrated machine: every
// `brew list --formula --versions <formula>` succeeds and emits the formula's keg
// LEAF NAME (like real brew: `run-kit 1.0.0`, not the qualified formula), EXCEPT the
// LEGACY run-kit formula (`sahil87/tap/rk`), which reports not-installed — a clean
// single-rack post-rename machine has no `rk` keg. This keeps the migration gate
// classifying run-kit as migrated (primary probe exit 0) with NO spurious dual-rack
// note, so the golden-string tests stay stable. All non-`brew list` calls succeed
// with empty stdout (delegated updates, upgrades, --help probes).
func allInstalledResponder() func(proc.Request) proc.Result {
	return func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			formula := req.Args[3]
			if formula == formulaPrefix+"rk" {
				return proc.Result{Err: errors.New("not installed")}
			}
			leaf := strings.TrimPrefix(formula, formulaPrefix)
			return proc.Result{Stdout: []byte(leaf + " 1.0.0\n")}
		}
		return proc.Result{}
	}
}

// installedOnly returns a respond function where only the named formulas report
// installed (via `brew list`), with all other requests succeeding (empty stdout).
// shll itself is reported not-brew-installed so the self-upgrade is skipped and
// the test stays focused on roster delegation. The `brew list` stdout emits the
// KEG LEAF NAME (the formula's leaf, matching real brew output like `run-kit 3.0.0`,
// not the fully-qualified formula) so the migration gate's leaf classification sees
// realistic input — a formula listed here reports leaf == its own name.
func installedOnly(formulas ...string) func(proc.Request) proc.Result {
	set := make(map[string]bool, len(formulas))
	for _, f := range formulas {
		set[f] = true
	}
	return func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			if set[req.Args[3]] {
				leaf := strings.TrimPrefix(req.Args[3], formulaPrefix)
				return proc.Result{Stdout: []byte(leaf + " 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}
}

func TestUpdate_FlagSupported(t *testing.T) {
	// run-kit is installed (migrated) and `run-kit update --help` advertises
	// --skip-brew-update → run-kit is upgraded via `run-kit update --skip-brew-update`,
	// NOT brew upgrade. installedOnly reports the run-kit keg present with leaf
	// `run-kit` (its parseBrewLeaf token), so the migration gate classifies it
	// migrated → normal delegation.
	base := installedOnly(formulaPrefix + "run-kit")
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "run-kit" && isUpdateHelpProbe(req) {
			return helpAdvertisesSkipFlag()
		}
		return base(req)
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, "run-kit", "update", skipBrewUpdateFlag) {
		t.Fatalf("expected `run-kit update %s`, calls: %+v", skipBrewUpdateFlag, calls)
	}
	if invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"run-kit") {
		t.Fatal("should NOT brew upgrade run-kit — must delegate to `run-kit update --skip-brew-update`")
	}
	if invocationsContain(calls, "run-kit", "update") {
		t.Fatal("expected the flagged form, not a bare `run-kit update`")
	}
}

func TestUpdate_FlagUnsupportedVersionSkew(t *testing.T) {
	// hop is installed but `hop update --help` does NOT advertise the flag
	// (version skew) → hop is upgraded via `hop update` with no flag, and does
	// NOT fall back to brew upgrade.
	f := &fakeRunner{respond: installedOnly(formulaPrefix + "hop")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, "hop", "update") {
		t.Fatalf("expected bare `hop update` (no flag), calls: %+v", calls)
	}
	if invocationsContain(calls, "hop", "update", skipBrewUpdateFlag) {
		t.Fatal("flag should NOT be passed when the tool does not advertise it")
	}
	if invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"hop") {
		t.Fatal("version-skew tool must run `hop update`, NOT fall back to brew upgrade")
	}
}

func TestUpdate_NoUpdateArgvFallsBackToBrew(t *testing.T) {
	// A (hypothetical future) roster tool with an empty Update argv that is
	// installed falls back to `brew upgrade <formula>`. Swap a single-entry
	// roster in for the duration of the test.
	prev := Roster
	t.Cleanup(func() { Roster = prev })
	legacy := Tool{Name: "legacy", Formula: formulaPrefix + "legacy"} // no Update argv
	Roster = []Tool{legacy}

	f := &fakeRunner{respond: installedOnly(legacy.Formula)}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, brewBinary, "upgrade", legacy.Formula) {
		t.Fatalf("expected brew upgrade fallback for a tool with no Update argv, calls: %+v", calls)
	}
	// No delegated update, and no --help probe (nothing to delegate to).
	if invocationsContain(calls, "legacy", "update") {
		t.Fatal("a tool with no Update argv must not be delegated to")
	}
	if invocationsContain(calls, "legacy", "update", "--help") {
		t.Fatal("a tool with no Update argv must not be capability-probed")
	}
}

func TestUpdate_StatusLinePrecedesProbes(t *testing.T) {
	// The status line is the first thing written to stdout, before any probe or
	// brew output. All installed; help advertises no flag.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if !strings.HasPrefix(stdout.String(), updateStatusLine+"\n") {
		t.Fatalf("stdout = %q, want to start with %q", stdout.String(), updateStatusLine)
	}
}

func TestUpdate_BrewUpdateRunsExactlyOnce(t *testing.T) {
	// With multiple roster tools installed, the hoisted `brew update --quiet`
	// runs exactly once for the whole run.
	f := &fakeRunner{respond: installedOnly(
		formulaPrefix+"run-kit",
		formulaPrefix+"hop",
		formulaPrefix+"wt",
	)}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	count := 0
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "update" && c.Args[1] == "--quiet" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("brew update --quiet ran %d times, want exactly 1", count)
	}
}

func TestUpdate_HeadersAndTail(t *testing.T) {
	// shll itself + the full roster are installed. With a bytes.Buffer (non-TTY)
	// stdout, the helper takes the plain branch, so the framing reads in the
	// ASCII `==>` / `Done — …` forms. The fakeRunner records calls but writes no
	// sub-tool bytes, so stdout contains exactly shll's own framing: the status
	// line, a `==> shll (self)` header before the self-upgrade, a `==> <tool>`
	// header per roster tool in order, then the all-succeeded tail.
	f := &fakeRunner{respond: allInstalledResponder()}
	installFakeRunner(t, f)
	// Deterministic clock: t0 then t0+72s → the tail reads `in 1m12s`.
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}

	// Headers now carry the [N/M] counter (shll (self) is [1/7] and first), each
	// header after the first is preceded by a blank line, and a blank line precedes
	// the duration-bearing tail. run-kit is migrated (no legacy keg), so no
	// migration/dual-rack note appears.
	want := updateStatusLine + "\n" +
		"==> [1/7] shll (self)\n" +
		"\n==> [2/7] wt\n" +
		"\n==> [3/7] idea\n" +
		"\n==> [4/7] tu\n" +
		"\n==> [5/7] run-kit\n" +
		"\n==> [6/7] hop\n" +
		"\n==> [7/7] fab-kit\n" +
		"\nDone — 7 of 7 tools succeeded in 1m12s.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	// Stream discipline: headers and tail go to stdout, never stderr.
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty (framing must not go to stderr)", stderr.String())
	}
}

func TestUpdate_HeaderPrecedesOutput(t *testing.T) {
	// The per-tool header must be written immediately BEFORE that tool's
	// foregrounded upgrade is invoked. We assert ordering by having the fake
	// snapshot stdout's length at the moment each delegated `<tool> update` runs:
	// the corresponding `==> <tool>` header must already be present in the buffer.
	base := installedOnly(formulaPrefix + "hop")
	var stdout, stderr bytes.Buffer
	var seenAtHopUpgrade string
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		// Delegated `hop update` (foreground), not the `hop update --help` probe.
		if req.Name == "hop" && req.Transport == proc.TransportForeground {
			seenAtHopUpgrade = stdout.String()
		}
		return base(req)
	}}
	installFakeRunner(t, f)

	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	// Only hop is installed (shll not brew-installed via installedOnly), so M=1 and
	// hop is [1/1].
	if !strings.Contains(seenAtHopUpgrade, "==> [1/1] hop\n") {
		t.Fatalf("at hop upgrade, stdout was %q, want it to already contain \"==> [1/1] hop\\n\"", seenAtHopUpgrade)
	}
}

func TestUpdate_PartialFailureTail(t *testing.T) {
	// hop and wt installed (shll itself not brew-installed via installedOnly, so
	// it is excluded from the count → total = 2). hop's delegated update fails,
	// wt succeeds → the partial-failure tail form with counts 1 succeeded,
	// 1 failed. Exit stays errSilent (unchanged).
	base := installedOnly(formulaPrefix+"hop", formulaPrefix+"wt")
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "hop" && req.Transport == proc.TransportForeground {
			return proc.Result{ExitCode: 1}
		}
		return base(req)
	}}
	installFakeRunner(t, f)
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent (one tool failed)", err)
	}
	// Partial-failure tail carries the duration before the em-dash.
	if !strings.HasSuffix(stdout.String(), "1 succeeded, 1 failed in 1m12s — see above.\n") {
		t.Fatalf("stdout = %q, want to end with partial-failure tail", stdout.String())
	}
	// Honesty constraint: the tail never claims updated/up-to-date.
	if strings.Contains(stdout.String(), "updated") || strings.Contains(stdout.String(), "up-to-date") {
		t.Fatalf("stdout = %q, must not claim updated/up-to-date", stdout.String())
	}
}

func TestUpdate_EmptyCaseNoHeaderNoTail(t *testing.T) {
	// Nothing installed (neither shll nor any roster tool) → the short-circuit
	// fires with no per-tool loop, so no header and no tail. The golden string is
	// exactly the status line + the one-line note.
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
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if got := stdout.String(); got != updateStatusLine+"\nNo shll tools installed.\n" {
		t.Fatalf("stdout = %q, want status line + note only (no header, no tail)", got)
	}
	if strings.Contains(stdout.String(), "==>") || strings.Contains(stdout.String(), "Done —") {
		t.Fatalf("empty case must emit no header and no tail, got %q", stdout.String())
	}
}

func TestUpdate_DryRunPreview(t *testing.T) {
	// shll itself NOT brew-installed (installedOnly); the full roster installed
	// (run-kit migrated — leaf run-kit); run-kit and hop advertise
	// --skip-brew-update, the rest do not. Dry-run prints the aligned-column preview
	// with the exact per-tool argv, in roster order.
	base := installedOnly(
		formulaPrefix+"wt", formulaPrefix+"idea", formulaPrefix+"tu",
		formulaPrefix+"run-kit", formulaPrefix+"hop", formulaPrefix+"fab-kit",
	)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if (req.Name == "run-kit" || req.Name == "hop") && isUpdateHelpProbe(req) {
			return helpAdvertisesSkipFlag()
		}
		return base(req)
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	// Longest label is "fab-kit" (7) since shll (self) is absent here; labels are
	// padded to 7. run-kit and hop carry the flag; wt/idea/tu/fab-kit do not.
	want := updateStatusLine + "\n" +
		"Would update 6 tools (brew metadata refresh first):\n" +
		"  wt       wt update\n" +
		"  idea     idea update\n" +
		"  tu       tu update\n" +
		"  run-kit  run-kit update --skip-brew-update\n" +
		"  hop      hop update --skip-brew-update\n" +
		"  fab-kit  fab-kit update\n"
	if got := stdout.String(); got != want {
		t.Fatalf("dry-run preview =\n%q\nwant\n%q", got, want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestUpdate_DryRunPreviewWithSelf(t *testing.T) {
	// shll itself brew-installed + full roster (run-kit migrated); no tool advertises
	// the flag. The preview lists shll (self) FIRST with `brew upgrade
	// sahil87/tap/shll`, and "shll (self)" (11 chars) is the widest label, so
	// commands align under it. allInstalledResponder must report shll's formula
	// installed too (it is not the legacy rk formula), so the self-step is present.
	f := &fakeRunner{respond: allInstalledResponder()}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	want := updateStatusLine + "\n" +
		"Would update 7 tools (brew metadata refresh first):\n" +
		"  shll (self)  brew upgrade sahil87/tap/shll\n" +
		"  wt           wt update\n" +
		"  idea         idea update\n" +
		"  tu           tu update\n" +
		"  run-kit      run-kit update\n" +
		"  hop          hop update\n" +
		"  fab-kit      fab-kit update\n"
	if got := stdout.String(); got != want {
		t.Fatalf("dry-run preview with self =\n%q\nwant\n%q", got, want)
	}
}

func TestUpdate_DryRunNoWrites(t *testing.T) {
	// Dry-run must run the read-only probes but perform NO write: no brew update,
	// no brew upgrade, no `<tool> update`. shll itself + full roster installed.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	calls := f.recordedCalls()

	// Read-only probes ARE present: brew list (install detection) and the
	// `<tool> update --help` capability probe.
	if !invocationsContain(calls, brewBinary, "list", "--formula", "--versions", shllFormula) {
		t.Errorf("expected brew list probe for shll itself, calls: %+v", calls)
	}
	probedHelp := false
	for _, c := range calls {
		if isUpdateHelpProbe(c) {
			probedHelp = true
			break
		}
	}
	if !probedHelp {
		t.Errorf("expected at least one `<tool> update --help` probe, calls: %+v", calls)
	}

	// Writes are FORBIDDEN in dry-run.
	if invocationsContain(calls, brewBinary, "update", "--quiet") {
		t.Error("brew update --quiet must NOT run in dry-run")
	}
	if invocationsContain(calls, brewBinary, "upgrade", shllFormula) {
		t.Error("brew upgrade (self) must NOT run in dry-run")
	}
	for _, tool := range Roster {
		if invocationsContain(calls, tool.Update[0], tool.Update[1]) {
			t.Errorf("`%s update` write must NOT run in dry-run", tool.Name)
		}
		if invocationsContain(calls, brewBinary, "upgrade", tool.Formula) {
			t.Errorf("brew upgrade %s must NOT run in dry-run", tool.Formula)
		}
	}
	// No foreground transport at all in dry-run (all writes are foreground).
	for _, c := range calls {
		if c.Transport == proc.TransportForeground {
			t.Errorf("dry-run must spawn no foreground (write) subprocess, got %+v", c)
		}
	}
}

func TestUpdate_DryRunGracefulDegradation(t *testing.T) {
	// Only hop and wt installed, shll not brew-installed → the preview lists exactly
	// those two (roster order: wt then hop), counter M=2 in the header.
	f := &fakeRunner{respond: installedOnly(formulaPrefix+"hop", formulaPrefix+"wt")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	want := updateStatusLine + "\n" +
		"Would update 2 tools (brew metadata refresh first):\n" +
		"  wt   wt update\n" +
		"  hop  hop update\n"
	if got := stdout.String(); got != want {
		t.Fatalf("dry-run graceful preview =\n%q\nwant\n%q", got, want)
	}
}

// --- Subset targeting (`shll update [tool...]`, change b2vg) ---

func TestUpdate_SubsetUnknownTargetHardErrors(t *testing.T) {
	// A typo'd / unknown target must fail loudly BEFORE any brew work: exit
	// non-zero, stderr names the unknown arg and lists valid targets, and NO brew
	// subprocess (not even `brew --version`) runs.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"hpo"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent for unknown target", err)
	}
	if !strings.Contains(stderr.String(), `"hpo"`) {
		t.Errorf("stderr = %q, want to name the unknown arg %q", stderr.String(), "hpo")
	}
	// The valid-target list is present so the user can self-correct (shll + roster).
	for _, want := range []string{shllTargetToken, "wt", "fab-kit"} {
		if !strings.Contains(stderr.String(), want) {
			t.Errorf("stderr = %q, want to list valid target %q", stderr.String(), want)
		}
	}
	// No brew/network work — validation is up front.
	if len(f.recordedCalls()) != 0 {
		t.Fatalf("expected NO subprocess calls on unknown target, got %+v", f.recordedCalls())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty on unknown target", stdout.String())
	}
}

func TestUpdate_SubsetMultipleUnknownAllReported(t *testing.T) {
	// When multiple args are unknown, all are reported in one error (better
	// one-shot fix).
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"foo", "bar"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), `"foo"`) || !strings.Contains(stderr.String(), `"bar"`) {
		t.Fatalf("stderr = %q, want to name BOTH unknown args", stderr.String())
	}
}

func TestUpdate_SubsetNamedNotInstalledErrors(t *testing.T) {
	// Only hop is installed; the user names `run-kit`, which is NOT installed AND has
	// no legacy keg → error (distinct from the whole-roster graceful skip). Exit
	// non-zero, nothing upgraded. installedOnly(hop) reports both sahil87/tap/run-kit
	// and sahil87/tap/rk not-installed, so the migration gate → not installed.
	f := &fakeRunner{respond: installedOnly(formulaPrefix + "hop")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"run-kit"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent for named-not-installed", err)
	}
	if !strings.Contains(stderr.String(), "run-kit: not installed") {
		t.Fatalf("stderr = %q, want to report `run-kit: not installed`", stderr.String())
	}
	calls := f.recordedCalls()
	// No write: no brew update, no upgrade of any kind.
	if invocationsContain(calls, brewBinary, "update", "--quiet") {
		t.Error("brew update --quiet must NOT run when a named target is not installed")
	}
	if invocationsContain(calls, "run-kit", "update") || invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"run-kit") {
		t.Error("nothing should be upgraded when a named target is not installed")
	}
}

func TestUpdate_SubsetShllSelfTargetOnly(t *testing.T) {
	// `shll update shll` with shll brew-installed → only the self-upgrade runs;
	// no roster tool is upgraded. The single `brew update --quiet` still runs once.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{} }}
	installFakeRunner(t, f)
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0) // sub-second → "0s"

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{shllTargetToken}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, brewBinary, "update", "--quiet") {
		t.Error("brew update --quiet should still run once for a subset (incl. shll-only)")
	}
	if !invocationsContain(calls, brewBinary, "upgrade", shllFormula) {
		t.Error("expected the shll self-upgrade")
	}
	// No roster tool is acted on.
	for _, tool := range Roster {
		if invocationsContain(calls, tool.Update[0], tool.Update[1]) {
			t.Errorf("roster tool %s must NOT be upgraded for `shll update shll`", tool.Name)
		}
		if invocationsContain(calls, brewBinary, "upgrade", tool.Formula) {
			t.Errorf("roster tool %s must NOT be brew-upgraded for `shll update shll`", tool.Name)
		}
	}
	// Subset of 1 → header [1/1] shll (self), tail M=1.
	if !strings.Contains(stdout.String(), "==> [1/1] shll (self)\n") {
		t.Errorf("stdout = %q, want `==> [1/1] shll (self)` header", stdout.String())
	}
	if !strings.HasSuffix(stdout.String(), "Done — 1 of 1 tools succeeded in 0s.\n") {
		t.Errorf("stdout = %q, want `Done — 1 of 1 tools …` tail", stdout.String())
	}
}

func TestUpdate_SubsetShllSelfNotBrewInstalledErrors(t *testing.T) {
	// `shll update shll` on a dev build (shll not brew-installed) → named-but-not-
	// installed error, the SAME error case as a roster tool.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" && req.Args[3] == shllFormula {
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{shllTargetToken})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent for `shll update shll` on a dev build", err)
	}
	if !strings.Contains(stderr.String(), "shll: not installed") {
		t.Fatalf("stderr = %q, want to report `shll: not installed`", stderr.String())
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "upgrade", shllFormula) {
		t.Error("self-upgrade must NOT run when shll itself is not brew-installed")
	}
}

func TestUpdate_SubsetSelfFirstThenRosterOrder(t *testing.T) {
	// `shll update hop shll` → shll self-upgrade first, then hop. Arg order is
	// (hop, shll) but processing is (shll-self, then roster order).
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"hop", shllTargetToken}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	selfIdx, hopIdx := -1, -1
	for i, c := range calls {
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "upgrade" && c.Args[1] == shllFormula {
			selfIdx = i
		}
		if c.Name == "hop" && len(c.Args) == 1 && c.Args[0] == "update" {
			hopIdx = i
		}
	}
	if selfIdx == -1 || hopIdx == -1 {
		t.Fatalf("missing expected upgrades (self=%d, hop=%d), calls: %+v", selfIdx, hopIdx, calls)
	}
	if selfIdx >= hopIdx {
		t.Fatalf("shll self-upgrade (%d) must come before hop (%d)", selfIdx, hopIdx)
	}
	// Subset of 2 → headers [1/2] shll (self), [2/2] hop.
	if !strings.Contains(stdout.String(), "==> [1/2] shll (self)\n") || !strings.Contains(stdout.String(), "==> [2/2] hop\n") {
		t.Errorf("stdout = %q, want [1/2] shll (self) and [2/2] hop headers", stdout.String())
	}
}

func TestUpdate_SubsetArgOrderIndependentRosterOrder(t *testing.T) {
	// `shll update fab-kit wt` (both installed) → processed in roster order: wt
	// before fab-kit, regardless of arg order. M=2.
	f := &fakeRunner{respond: installedOnly(formulaPrefix+"fab-kit", formulaPrefix+"wt")}
	installFakeRunner(t, f)
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0) // sub-second → "0s"

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"fab-kit", "wt"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	wtIdx, fabIdx := -1, -1
	for i, c := range calls {
		if c.Name == "wt" && len(c.Args) == 1 && c.Args[0] == "update" {
			wtIdx = i
		}
		if c.Name == "fab-kit" && len(c.Args) == 1 && c.Args[0] == "update" {
			fabIdx = i
		}
	}
	if wtIdx == -1 || fabIdx == -1 {
		t.Fatalf("missing wt/fab-kit upgrades (wt=%d, fab-kit=%d), calls: %+v", wtIdx, fabIdx, calls)
	}
	if wtIdx >= fabIdx {
		t.Fatalf("wt (%d) must be processed before fab-kit (%d) — roster order, not arg order", wtIdx, fabIdx)
	}
	// Only the two named tools are acted on (shll not named → no self-upgrade; idea/tu/rk/hop untouched).
	if invocationsContain(calls, brewBinary, "upgrade", shllFormula) {
		t.Error("shll self-upgrade must NOT run when shll is not named")
	}
	for _, name := range []string{"idea", "tu", "run-kit", "hop"} {
		if invocationsContain(calls, name, "update") {
			t.Errorf("unnamed tool %s must NOT be upgraded", name)
		}
	}
	// Counter denominator M=2.
	if !strings.Contains(stdout.String(), "==> [1/2] wt\n") || !strings.Contains(stdout.String(), "==> [2/2] fab-kit\n") {
		t.Errorf("stdout = %q, want [1/2] wt and [2/2] fab-kit headers", stdout.String())
	}
	if !strings.HasSuffix(stdout.String(), "Done — 2 of 2 tools succeeded in 0s.\n") {
		t.Errorf("stdout = %q, want `Done — 2 of 2 …` tail (M = subset size)", stdout.String())
	}
}

func TestUpdate_SubsetBrewUpdateRunsOnce(t *testing.T) {
	// `brew update --quiet` runs exactly once even for a single-tool subset.
	f := &fakeRunner{respond: installedOnly(formulaPrefix + "hop")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"hop"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	count := 0
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "update" && c.Args[1] == "--quiet" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("brew update --quiet ran %d times for a single-tool subset, want exactly 1", count)
	}
}

func TestUpdate_SubsetDryRunPreviewFiltered(t *testing.T) {
	// `shll update --dry-run hop wt` (both installed, shll not named) → preview
	// lists exactly the two-tool subset in roster order (wt then hop), M=2, no write.
	f := &fakeRunner{respond: installedOnly(formulaPrefix+"hop", formulaPrefix+"wt")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, true, []string{"hop", "wt"}); err != nil {
		t.Fatalf("runUpdate --dry-run subset err = %v, want nil", err)
	}
	want := updateStatusLine + "\n" +
		"Would update 2 tools (brew metadata refresh first):\n" +
		"  wt   wt update\n" +
		"  hop  hop update\n"
	if got := stdout.String(); got != want {
		t.Fatalf("subset dry-run preview =\n%q\nwant\n%q", got, want)
	}
	// No write of any kind in dry-run.
	for _, c := range f.recordedCalls() {
		if c.Transport == proc.TransportForeground {
			t.Errorf("subset dry-run must spawn no foreground (write) subprocess, got %+v", c)
		}
	}
}

func TestUpdate_DryRunEmptyCase(t *testing.T) {
	// Nothing installed → dry-run mirrors the non-dry-run nothing-to-do message,
	// exit 0, no preview table, no writes.
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
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	if got := stdout.String(); got != updateStatusLine+"\n"+noToolsInstalledMsg+"\n" {
		t.Fatalf("dry-run empty case stdout = %q, want status line + nothing-to-do", got)
	}
	if strings.Contains(stdout.String(), "Would update") {
		t.Fatalf("dry-run empty case must not print a preview table, got %q", stdout.String())
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "update", "--quiet") {
		t.Fatal("brew update --quiet must NOT run in dry-run empty case")
	}
}

// --- "What changed:" digest tail (change r01z) ---

// versionTransitionRunner is a stateful fake whose `brew list --formula
// --versions <formula>` returns beforeByFormula[formula] on its FIRST call for a
// formula (the pre-upgrade probe / before-capture) and afterByFormula[formula] on
// every subsequent call (the post-upgrade re-query) — so a tool's version
// "changes" across the upgrade. `brew --version` reports brew present; all other
// calls (brew update, upgrades, delegated `<tool> update`, `--help` probes)
// succeed with empty output. It is concurrency-safe (probeRoster runs probes in
// parallel) via its own mutex.
type versionTransitionRunner struct {
	mu     sync.Mutex
	seen   map[string]int // formula → number of `brew list` calls so far
	before map[string]string
	after  map[string]string
}

func (r *versionTransitionRunner) respond(req proc.Request) proc.Result {
	if req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "--version" {
		return proc.Result{Stdout: []byte("Homebrew 4.0\n")}
	}
	if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
		formula := req.Args[3]
		r.mu.Lock()
		defer r.mu.Unlock()
		n := r.seen[formula]
		r.seen[formula]++
		leaf := strings.TrimPrefix(formula, formulaPrefix)
		if n == 0 {
			if v, ok := r.before[formula]; ok {
				return proc.Result{Stdout: []byte(leaf + " " + v + "\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		}
		if v, ok := r.after[formula]; ok {
			return proc.Result{Stdout: []byte(leaf + " " + v + "\n")}
		}
		if v, ok := r.before[formula]; ok {
			return proc.Result{Stdout: []byte(leaf + " " + v + "\n")}
		}
		return proc.Result{Err: errors.New("not installed")}
	}
	return proc.Result{}
}

func TestUpdate_DigestPrintsForBumpedTools(t *testing.T) {
	// hop bumps 0.1.16 → 0.1.18 (2 releases); shll not brew-installed so it's out.
	r := &versionTransitionRunner{
		seen:   map[string]int{},
		before: map[string]string{formulaPrefix + "hop": "0.1.16"},
		after:  map[string]string{formulaPrefix + "hop": "0.1.18"},
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{
		"hop": relJSON(
			[3]string{"v0.1.18", "feat: non-interactive agent support", "## What's Changed\n* agent mode"},
			[3]string{"v0.1.17", "fix: shim hardening", "## What's Changed\n* shim fix"},
			[3]string{"v0.1.16", "old", "old body"},
		),
	})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "What changed:") {
		t.Fatalf("out missing digest header:\n%s", out)
	}
	// Non-TTY buffer → ASCII-degraded arrow (`->`). With inline bodies the digest
	// drops the cross-tool column alignment; the transition line carries the tool
	// name (unlike `shll changelog`, whose name is in the header above the body).
	if !strings.Contains(out, "hop 0.1.16 -> 0.1.18 (2 releases)") {
		t.Fatalf("out missing hop transition line:\n%s", out)
	}
	// Tag/title lines for each release, newest first, EACH FOLLOWED BY ITS FULL
	// BODY inline (change 13k3 — the notes render in-process, not one copy-paste away).
	if !strings.Contains(out, "v0.1.18  feat: non-interactive agent support") ||
		!strings.Contains(out, "v0.1.17  fix: shim hardening") {
		t.Fatalf("out missing release tag/title lines:\n%s", out)
	}
	if !strings.Contains(out, "* agent mode") || !strings.Contains(out, "* shim fix") {
		t.Fatalf("out missing inline release bodies:\n%s", out)
	}
	// v0.1.16 == old is excluded (outside (0.1.16, 0.1.18]).
	if strings.Contains(out, "old body") {
		t.Fatalf("out should exclude the release equal to the old bound:\n%s", out)
	}
	// The `Full notes:` tail line is GONE — the notes are inline now.
	if strings.Contains(out, "Full notes:") {
		t.Fatalf("out must NOT carry the dropped `Full notes:` line:\n%s", out)
	}
	// The digest is AFTER the summary tail.
	if strings.Index(out, "Done —") > strings.Index(out, "What changed:") {
		t.Fatalf("digest must follow the summary tail, out:\n%s", out)
	}
}

func TestUpdate_NoDigestWhenNothingBumped(t *testing.T) {
	// Same before/after version → no bump → the output is byte-identical to the
	// pre-change golden (no "What changed:" block). Uses the existing all-installed
	// golden from TestUpdate_HeadersAndTail.
	f := &fakeRunner{respond: allInstalledResponder()}
	installFakeRunner(t, f)
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	want := updateStatusLine + "\n" +
		"==> [1/7] shll (self)\n" +
		"\n==> [2/7] wt\n" +
		"\n==> [3/7] idea\n" +
		"\n==> [4/7] tu\n" +
		"\n==> [5/7] run-kit\n" +
		"\n==> [6/7] hop\n" +
		"\n==> [7/7] fab-kit\n" +
		"\nDone — 7 of 7 tools succeeded in 1m12s.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout with no bumps must equal the pre-change golden.\n got=%q\nwant=%q", got, want)
	}
	if strings.Contains(stdout.String(), "What changed:") {
		t.Fatalf("no digest expected when nothing bumped, got:\n%s", stdout.String())
	}
}

func TestUpdate_NoDigestUnderDryRun(t *testing.T) {
	// --dry-run performs no upgrade, so there are no bumps and no digest.
	r := &versionTransitionRunner{
		seen:   map[string]int{},
		before: map[string]string{formulaPrefix + "hop": "0.1.16"},
		after:  map[string]string{formulaPrefix + "hop": "0.1.18"},
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	if strings.Contains(stdout.String(), "What changed:") {
		t.Fatalf("dry-run must print no digest, got:\n%s", stdout.String())
	}
}

func TestUpdate_DigestSubsetNamesOnlyBumped(t *testing.T) {
	// Subset run `shll update hop`: only hop is acted on and bumped, so the digest
	// names ONLY hop.
	r := &versionTransitionRunner{
		seen:   map[string]int{},
		before: map[string]string{formulaPrefix + "hop": "0.1.16", formulaPrefix + "wt": "1.0.0"},
		after:  map[string]string{formulaPrefix + "hop": "0.1.18", formulaPrefix + "wt": "1.1.0"},
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{
		"hop": relJSON([3]string{"v0.1.18", "hop18", "b"}, [3]string{"v0.1.17", "hop17", "b"}),
		"wt":  relJSON([3]string{"v1.1.0", "wt110", "b"}),
	})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"hop"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	out := stdout.String()
	// The digest covers ONLY the bumped subset member (hop) — its transition line
	// is present and wt (outside the subset) never appears.
	if !strings.Contains(out, "hop 0.1.16 -> 0.1.18 (2 releases)") {
		t.Fatalf("out missing hop transition line:\n%s", out)
	}
	if strings.Contains(out, "wt 1.0.0") || strings.Contains(out, "wt110") {
		t.Fatalf("subset digest must NOT render wt (not in the subset), out:\n%s", out)
	}
}

func TestUpdate_DigestUnavailableDegradesToCompareURL(t *testing.T) {
	// hop bumps, but the release fetch fails (server 404s) → the digest degrades
	// hop's entry to the compare URL and the exit code stays 0.
	r := &versionTransitionRunner{
		seen:   map[string]int{},
		before: map[string]string{formulaPrefix + "hop": "0.1.16"},
		after:  map[string]string{formulaPrefix + "hop": "0.1.18"},
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{}) // no repos → 404 → unavailable

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil (fetch failure must not change exit code)", err)
	}
	out := stdout.String()
	// Non-TTY buffer → ASCII-degraded arrow + dash.
	if !strings.Contains(out, "hop 0.1.16 -> 0.1.18 -- see "+changelog.CompareURL("hop", "0.1.16", "0.1.18")) {
		t.Fatalf("out missing compare-URL degradation for hop:\n%s", out)
	}
}

func TestUpdate_DigestMixedAvailableAndUnavailable(t *testing.T) {
	// Two tools bump in one run: wt's releases are served (available), run-kit's repo
	// 404s (unavailable). The digest must render wt as a full transition + title
	// lines AND run-kit as a compare-URL fallback in the SAME block — partial
	// degradation, exit code unaffected. wt precedes run-kit (roster order). run-kit
	// is MIGRATED here (leaf run-kit from the current formula), a normal delegated
	// bump, not a migration.
	r := &versionTransitionRunner{
		seen:   map[string]int{},
		before: map[string]string{formulaPrefix + "wt": "1.0.0", formulaPrefix + "run-kit": "0.1.0"},
		after:  map[string]string{formulaPrefix + "wt": "1.1.0", formulaPrefix + "run-kit": "0.2.0"},
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	// wt served; run-kit's repo slug absent → 404 → unavailable.
	changelogServer(t, map[string]string{
		"wt": relJSON([3]string{"v1.1.0", "wt110", "b"}),
	})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	out := stdout.String()
	// wt: full transition + a release tag/title line (tag+title separated by two
	// spaces), rendered via the shared renderReleases helper.
	if !strings.Contains(out, "wt 1.0.0 -> 1.1.0 (1 release)") || !strings.Contains(out, "v1.1.0  wt110") {
		t.Fatalf("out missing available wt entry:\n%s", out)
	}
	// run-kit: compare-URL fallback (run-kit slug) — no bodies exist to inline.
	if !strings.Contains(out, "run-kit 0.1.0 -> 0.2.0 -- see "+changelog.CompareURL("run-kit", "0.1.0", "0.2.0")) {
		t.Fatalf("out missing unavailable run-kit fallback:\n%s", out)
	}
	// wt precedes run-kit (roster order).
	if strings.Index(out, "wt 1.0.0") > strings.Index(out, "run-kit 0.1.0") {
		t.Fatalf("digest must render wt before run-kit (roster order):\n%s", out)
	}
	// Tool blocks are blank-line separated (mirroring runChangelog's per-tool
	// separation): wt's last body line ("b") is followed by a blank line before
	// run-kit's transition line — tools are never separated more weakly than the
	// releases within one tool.
	if !strings.Contains(out, "b\n\n  run-kit 0.1.0") {
		t.Fatalf("out missing blank line between wt and run-kit digest blocks:\n%s", out)
	}
}

func TestUpdate_DigestNoColumnAlignmentWithInlineBodies(t *testing.T) {
	// With full bodies inline (change 13k3), the cross-tool two-pass column
	// alignment is DROPPED — the per-tool transition line keeps only the 2-space
	// digestToolIndent and is otherwise unpadded (padding is meaningless across
	// multi-line body blocks). tu (2) and fab-kit (7) differ in name width and their
	// transitions differ in width, so under the OLD contract they would have been
	// padded to align; here neither is padded.
	r := &versionTransitionRunner{
		seen:   map[string]int{},
		before: map[string]string{formulaPrefix + "tu": "0.6.2", formulaPrefix + "fab-kit": "1.0.0"},
		after:  map[string]string{formulaPrefix + "tu": "0.6.4", formulaPrefix + "fab-kit": "1.10.0"},
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{
		"tu":      relJSON([3]string{"v0.6.4", "t4", "tu body"}, [3]string{"v0.6.3", "t3", "b"}),
		"fab-kit": relJSON([3]string{"v1.10.0", "f", "fab body"}),
	})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	out := stdout.String()
	// Unpadded transition lines: just the 2-space indent + `{tool} {old} -> {new}
	// (N release[s])`, no column padding after the tool name or the transition.
	if !strings.Contains(out, "  tu 0.6.2 -> 0.6.4 (2 releases)\n") {
		t.Fatalf("out missing UNpadded tu transition line:\n%s", out)
	}
	if !strings.Contains(out, "  fab-kit 1.0.0 -> 1.10.0 (1 release)\n") {
		t.Fatalf("out missing UNpadded fab-kit transition line:\n%s", out)
	}
	// Bodies render inline for both tools.
	if !strings.Contains(out, "tu body") || !strings.Contains(out, "fab body") {
		t.Fatalf("out missing inline bodies for tu/fab-kit:\n%s", out)
	}
}

func TestParseBrewVersion_MultiKegPicksMax(t *testing.T) {
	// A multi-keg `brew list --versions` line lists every installed version in
	// ARBITRARY order; parseBrewVersion must pick the MAX by numeric compare so a
	// multi-keg host reports the current version, not an oldest-keg stale value.
	cases := map[string]string{
		"tu 0.6.2 0.6.4":        "0.6.4",  // ascending
		"tu 0.6.4 0.6.2":        "0.6.4",  // descending — fields[1] is the OLDEST
		"tu 0.6.2 0.6.10 0.6.4": "0.6.10", // numeric, not lexical
		"tu 0.6.4":              "0.6.4",  // single keg
		"tu":                    "",       // no version field
	}
	for in, want := range cases {
		if got := parseBrewVersion(in + "\n"); got != want {
			t.Errorf("parseBrewVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestMakeBump_NormalizesVersions(t *testing.T) {
	// brew can report revision-suffixed versions like `0.6.4_1`. makeBump must
	// normalize BOTH sides (strip `v` + `_N`) so the digest transition and the
	// compare URL stay on the toolkit's tag footing — and a revision-only change
	// (`0.6.4` → `0.6.4_1`) must NOT register as a bump (they normalize equal).
	cases := []struct {
		name       string
		old, new   string
		wantOK     bool
		wantOldNew [2]string // only checked when wantOK
	}{
		{"revision suffix stripped", "0.6.2", "0.6.4_1", true, [2]string{"0.6.2", "0.6.4"}},
		{"v-prefix stripped", "v0.6.2", "v0.6.4", true, [2]string{"0.6.2", "0.6.4"}},
		{"both forms mixed", "0.6.2_1", "v0.6.4", true, [2]string{"0.6.2", "0.6.4"}},
		{"revision-only change is no bump", "0.6.4", "0.6.4_1", false, [2]string{}},
		{"empty new suppresses", "0.6.2", "", false, [2]string{}},
		{"empty old suppresses", "", "0.6.4", false, [2]string{}},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			b, ok := makeBump("tu", "tu", c.old, c.new)
			if ok != c.wantOK {
				t.Fatalf("makeBump(%q, %q) ok = %v, want %v", c.old, c.new, ok, c.wantOK)
			}
			if !c.wantOK {
				return
			}
			if b.old != c.wantOldNew[0] || b.new != c.wantOldNew[1] {
				t.Errorf("makeBump(%q, %q) = {old:%q new:%q}, want {old:%q new:%q}",
					c.old, c.new, b.old, b.new, c.wantOldNew[0], c.wantOldNew[1])
			}
		})
	}
}

// --- rk→run-kit migration guard (change 9bak) ---

const (
	runKitFormula       = formulaPrefix + "run-kit"
	runKitLegacyFormula = formulaPrefix + "rk"
)

// migrationRunner is a stateful fake modeling a machine mid-migration. brewList maps
// a formula → the `brew list` stdout it should emit (leaf + version) BEFORE the
// migration; a formula absent from the map reports not-installed. After a
// `brew upgrade <runKitLegacyFormula>` runs, the current formula begins reporting
// afterList (so the digest re-query sees the migrated version).
//
// legacyAfterList models what `brew list <runKitLegacyFormula>` reports AFTER the
// migration upgrade — driving migrateRunKit's POST-migration dual-rack re-probe:
//   - "" (default) → the rename consumed the legacy keg (clean migration) → the
//     legacy formula reports not-installed afterward → NO dual-rack note.
//   - a `rk <ver>` line → the migration left the legacy keg behind (a dual-rack
//     orphan, leaf `rk`) → migrateRunKit's re-probe fires the cleanup note.
//
// The `run-kit --version` post-check models the linked/unlinked pathology:
//   - linkOnUpgrade=true  → the migration upgrade itself puts run-kit on PATH, so the
//     post-check succeeds and NO `brew link` is needed (the linked case).
//   - linkOnUpgrade=false → run-kit stays OFF PATH after the upgrade (the unlinked
//     keg, state A) until a `brew link run-kit` runs, which flips it on. So the
//     post-check sees ErrNotFound and `brew link run-kit` fires.
//
// upgradeFails makes `brew upgrade <runKitLegacyFormula>` exit non-zero (proc returns
// (code, nil) — the failed-migration path): the migration must then run NO
// link/daemon-note/re-probe and surface the exit code.
//
// Concurrency-safe (probes run in parallel).
type migrationRunner struct {
	mu              sync.Mutex
	brewList        map[string]string // formula → `brew list` stdout (before migration)
	afterList       string            // current-formula `brew list` stdout after `brew upgrade rk`
	legacyAfterList string            // legacy-formula `brew list` stdout after migration ("" → gone)
	linkOnUpgrade   bool              // upgrade alone links the binary (no separate brew link needed)
	upgradeFails    bool              // `brew upgrade <legacy>` exits non-zero (failed migration)
	migrated        bool              // set once `brew upgrade <legacy>` runs
	runKitPath      bool              // `run-kit --version` succeeds once true
}

func (r *migrationRunner) respond(req proc.Request) proc.Result {
	r.mu.Lock()
	defer r.mu.Unlock()
	switch {
	case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "--version":
		return proc.Result{Stdout: []byte("Homebrew 6.0.4\n")}
	case req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list":
		formula := req.Args[3]
		if formula == runKitFormula && r.migrated && r.afterList != "" {
			return proc.Result{Stdout: []byte(r.afterList)}
		}
		// After the migration, the legacy formula's `brew list` reflects the
		// post-migration reality (legacyAfterList), not its pre-migration value — so
		// the dual-rack re-probe sees whether the rename consumed the legacy keg.
		if formula == runKitLegacyFormula && r.migrated {
			if r.legacyAfterList == "" {
				return proc.Result{Err: errors.New("not installed")}
			}
			return proc.Result{Stdout: []byte(r.legacyAfterList)}
		}
		if out, ok := r.brewList[formula]; ok {
			return proc.Result{Stdout: []byte(out)}
		}
		return proc.Result{Err: errors.New("not installed")}
	case req.Name == brewBinary && len(req.Args) >= 2 && req.Args[0] == "upgrade" && req.Args[1] == runKitLegacyFormula:
		// The brew-direct migration. A failed upgrade exits non-zero (proc returns
		// (code, nil)) and does NOT migrate the keg. Otherwise the keg is migrated;
		// whether run-kit is on PATH afterward depends on linkOnUpgrade (the linked
		// vs. unlinked pathology).
		if r.upgradeFails {
			return proc.Result{ExitCode: 1}
		}
		r.migrated = true
		if r.linkOnUpgrade {
			r.runKitPath = true
		}
		return proc.Result{}
	case req.Name == brewBinary && len(req.Args) >= 2 && req.Args[0] == "link" && req.Args[1] == "run-kit":
		// `brew link run-kit` links the unlinked keg → run-kit is now on PATH.
		r.runKitPath = true
		return proc.Result{}
	case req.Name == "run-kit" && len(req.Args) == 1 && req.Args[0] == "--version":
		if r.runKitPath {
			return proc.Result{Stdout: []byte("run-kit 3.0.0\n")}
		}
		return proc.Result{Err: proc.ErrNotFound}
	}
	return proc.Result{}
}

func TestUpdate_MigrationStateA_UnlinkedLegacyKeg(t *testing.T) {
	// State A: legacy keg present (leaf `rk`), UNLINKED — `run-kit --version` fails
	// with ErrNotFound after the brew-direct upgrade, so `brew link run-kit` runs.
	// The migration is `brew upgrade sahil87/tap/rk`; the old binary is NEVER
	// delegated to. Digest reads before from the legacy keg, after from the new
	// formula → `run-kit 2.5.13 → 3.0.0`.
	r := &migrationRunner{
		brewList:   map[string]string{runKitLegacyFormula: "rk 2.5.13\n"},
		afterList:  "run-kit 3.0.0\n",
		runKitPath: false, // unlinked until `brew link`
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{
		"run-kit": relJSON([3]string{"v3.0.0", "run-kit 3.0", "## What's Changed\n* rename"}),
	})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	// Brew-direct migration ran; the old binary was NOT delegated to.
	if !invocationsContain(calls, brewBinary, "upgrade", runKitLegacyFormula) {
		t.Fatalf("expected the brew-direct migration `brew upgrade %s`, calls: %+v", runKitLegacyFormula, calls)
	}
	if invocationsContain(calls, "run-kit", "update") || invocationsContain(calls, "rk", "update") {
		t.Fatal("migration must be brew-direct — never delegate to the (old) binary's update")
	}
	// Unlinked → `brew link run-kit` ran (post-check saw ErrNotFound).
	if !invocationsContain(calls, brewBinary, "link", "run-kit") {
		t.Fatalf("expected `brew link run-kit` for the unlinked-keg pathology, calls: %+v", calls)
	}
	out := stdout.String()
	// Daemon-restart note is PRINTED (never executed).
	if !strings.Contains(out, "run-kit serve --restart") {
		t.Fatalf("out missing the daemon-restart note:\n%s", out)
	}
	// Clean migration: the rename consumed the legacy keg (legacyAfterList defaults
	// to ""), so the POST-migration dual-rack re-probe finds no leftover → NO
	// dual-rack cleanup note.
	if strings.Contains(out, "leftover") {
		t.Fatalf("a clean migration (legacy keg consumed) must NOT print a dual-rack note:\n%s", out)
	}
	// Digest renders the migration transition using the legacy before-version and
	// the new-formula after-version.
	if !strings.Contains(out, "run-kit 2.5.13 -> 3.0.0 (1 release)") {
		t.Fatalf("out missing `run-kit 2.5.13 -> 3.0.0` digest transition:\n%s", out)
	}
}

func TestUpdate_MigrationLeftoverDualRackNote(t *testing.T) {
	// A migration that leaves the legacy keg behind (state C reached via migration):
	// pre-migration only the legacy `rk` keg exists (→ needsMigration), and AFTER
	// `brew upgrade sahil87/tap/rk` the legacy formula STILL reports leaf `rk`
	// alongside the migrated run-kit keg. The POST-migration re-probe inside
	// migrateRunKit detects the leftover and prints the one-line cleanup note. This
	// is the note's ONLY live path — a needsMigration probe never carries dualRack.
	r := &migrationRunner{
		brewList:        map[string]string{runKitLegacyFormula: "rk 2.5.13\n"},
		afterList:       "run-kit 3.0.0\n",
		legacyAfterList: "rk 3.0.0\n", // legacy keg lingers after migration → dual-rack
		linkOnUpgrade:   true,         // linked, to keep this focused on the re-probe
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "leftover "+runKitLegacyFormula+" keg remains alongside "+runKitFormula) {
		t.Fatalf("out missing the POST-migration dual-rack cleanup note:\n%s", out)
	}
	// Detection only — shll never removes the orphan keg.
	if invocationsContain(f.recordedCalls(), brewBinary, "uninstall", runKitLegacyFormula) {
		t.Fatal("post-migration dual-rack is detection-only — shll must NEVER uninstall the orphan keg")
	}
}

func TestUpdate_MigrationFailedUpgradeSkipsPostSteps(t *testing.T) {
	// A FAILED `brew upgrade sahil87/tap/rk` (non-zero exit; proc returns (code, nil))
	// must abort the migration BEFORE the post-steps: NO `brew link`, NO daemon note,
	// NO dual-rack re-probe. The failure flips the run to errSilent.
	r := &migrationRunner{
		brewList:     map[string]string{runKitLegacyFormula: "rk 2.5.13\n"},
		upgradeFails: true,
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))

	var stdout, stderr bytes.Buffer
	err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"run-kit"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUpdate err = %v, want errSilent (failed migration)", err)
	}
	calls := f.recordedCalls()
	// The upgrade WAS attempted.
	if !invocationsContain(calls, brewBinary, "upgrade", runKitLegacyFormula) {
		t.Fatalf("expected the migration upgrade to be attempted, calls: %+v", calls)
	}
	// No post-steps: no `brew link run-kit`.
	if invocationsContain(calls, brewBinary, "link", "run-kit") {
		t.Fatal("a failed migration must NOT run `brew link run-kit`")
	}
	out := stdout.String()
	// No daemon note, no dual-rack note.
	if strings.Contains(out, "run-kit serve --restart") {
		t.Fatalf("a failed migration must NOT print the daemon note:\n%s", out)
	}
	if strings.Contains(out, "leftover") {
		t.Fatalf("a failed migration must NOT print a dual-rack note:\n%s", out)
	}
	// The post-check `run-kit --version` (which precedes `brew link`) must not run
	// either — the post-steps are gated as a block on the upgrade's success.
	if invocationsContain(calls, "run-kit", "--version") {
		t.Fatal("a failed migration must NOT run the `run-kit --version` post-check")
	}
}

func TestUpdate_MigrationStateA_Linked(t *testing.T) {
	// A LINKED migration (the `brew upgrade` puts run-kit on PATH immediately) must
	// NOT run `brew link` — the post-check succeeds, so the conditional link is
	// skipped. Only the unlinked pathology triggers `brew link run-kit`.
	r := &migrationRunner{
		brewList:      map[string]string{runKitLegacyFormula: "rk 2.5.13\n"},
		afterList:     "run-kit 3.0.0\n",
		linkOnUpgrade: true, // upgrade alone links the binary
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, brewBinary, "upgrade", runKitLegacyFormula) {
		t.Fatal("expected the brew-direct migration upgrade")
	}
	if invocationsContain(calls, brewBinary, "link", "run-kit") {
		t.Fatal("`brew link run-kit` must NOT run when the migrated binary is already on PATH")
	}
}

func TestUpdate_MigrationStateB_AlreadyMigrated(t *testing.T) {
	// State B: `brew list sahil87/tap/run-kit` reports leaf `run-kit` (installed via
	// the current formula) → MIGRATED. Even though a legacy-formula probe could
	// exit 0, exit-code alone must not gate: the current-formula probe classifies it
	// migrated → normal delegation (`run-kit update`), NO migration action.
	r := &migrationRunner{
		brewList: map[string]string{
			runKitFormula: "run-kit 3.0.0\n",
			// state B: `brew list sahil87/tap/rk` also exits 0 via rename resolution,
			// reporting the CURRENT leaf `run-kit` — this must NOT be read as legacy.
			runKitLegacyFormula: "run-kit 3.0.0\n",
		},
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	// Normal delegation — NOT the migration action.
	if !invocationsContain(calls, "run-kit", "update") {
		t.Fatalf("state B must delegate to `run-kit update`, calls: %+v", calls)
	}
	if invocationsContain(calls, brewBinary, "upgrade", runKitLegacyFormula) {
		t.Fatal("state B (already migrated) must NOT run the migration `brew upgrade sahil87/tap/rk`")
	}
	// State B is a SINGLE migrated keg: the `sahil87/tap/rk` probe exits 0 only via
	// rename resolution, reporting the CURRENT leaf `run-kit` — not a second rack. So
	// dual-rack must NOT be flagged (the detection keys on the LEGACY leaf `rk`), and
	// no leftover-keg cleanup note prints.
	if strings.Contains(stdout.String(), "leftover") {
		t.Fatalf("state B (single migrated keg, rename resolution) must NOT print a dual-rack note:\n%s", stdout.String())
	}
}

func TestUpdate_MigrationStateC_DualRack(t *testing.T) {
	// State C: BOTH kegs present. The current-formula probe reports installed (leaf
	// run-kit) → migrated (normal delegation); the legacy-formula probe also exits 0
	// → dual-rack detected → a one-line cleanup note is printed (detection only, no
	// destructive action). Leaf `rk` for the legacy keg here.
	r := &migrationRunner{
		brewList: map[string]string{
			runKitFormula:       "run-kit 3.0.0\n",
			runKitLegacyFormula: "rk 3.0.0\n",
		},
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	// Migrated → normal delegation; no migration action.
	if !invocationsContain(calls, "run-kit", "update") {
		t.Fatalf("state C must delegate to `run-kit update`, calls: %+v", calls)
	}
	if invocationsContain(calls, brewBinary, "upgrade", runKitLegacyFormula) {
		t.Fatal("state C (migrated, dual-rack) must NOT run the migration upgrade")
	}
	// Dual-rack cleanup note printed; shll never removes the orphan keg.
	out := stdout.String()
	if !strings.Contains(out, "leftover "+runKitLegacyFormula+" keg remains alongside "+runKitFormula) {
		t.Fatalf("out missing the dual-rack cleanup note:\n%s", out)
	}
	// The note points at `shll uninstall run-kit` (the now-supported, leaf-verified dual-
	// rack cleanup) and gives the brew-direct alternative by the LEGACY LEAF NAME only.
	if !strings.Contains(out, "shll uninstall run-kit") {
		t.Errorf("dual-rack note must point at `shll uninstall run-kit`, out:\n%s", out)
	}
	if !strings.Contains(out, "brew uninstall rk") {
		t.Errorf("dual-rack note's brew-direct alternative must be the leaf name `brew uninstall rk`, out:\n%s", out)
	}
	// It must NOT suggest the qualified old-name form — post-rename brew re-resolves
	// `sahil87/tap/rk` → run-kit, so that would delete the good keg (the footgun).
	if strings.Contains(out, "brew uninstall "+runKitLegacyFormula) {
		t.Errorf("dual-rack note must NOT suggest the qualified `brew uninstall %s` (rename re-resolution deletes the good keg), out:\n%s", runKitLegacyFormula, out)
	}
	if invocationsContain(calls, brewBinary, "uninstall", runKitLegacyFormula) {
		t.Fatal("dual-rack is detection-only — shll must NEVER uninstall the orphan keg")
	}
}

func TestUpdate_MigrationWholeRosterFlowsThroughGuard(t *testing.T) {
	// A whole-roster `shll update` on a legacy-keg machine migrates run-kit rather
	// than silently skipping it (the graceful-degradation trap the guard fixes).
	r := &migrationRunner{
		brewList:  map[string]string{runKitLegacyFormula: "rk 2.5.13\n"},
		afterList: "run-kit 3.0.0\n",
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if !invocationsContain(f.recordedCalls(), brewBinary, "upgrade", runKitLegacyFormula) {
		t.Fatal("whole-roster update must migrate a legacy-keg run-kit, not skip it")
	}
}

func TestUpdate_MigrationViaLegacyAliasWithNotice(t *testing.T) {
	// `shll update rk` on a legacy-keg machine resolves the alias to run-kit, prints
	// the rename notice, and migrates.
	r := &migrationRunner{
		brewList:  map[string]string{runKitLegacyFormula: "rk 2.5.13\n"},
		afterList: "run-kit 3.0.0\n",
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)
	installFakeClock(t, time.Unix(1000, 0), time.Unix(1000, 0))
	changelogServer(t, map[string]string{})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, false, []string{"rk"}); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "note: rk is now run-kit") {
		t.Fatalf("out missing the `rk is now run-kit` alias notice:\n%s", out)
	}
	if !invocationsContain(f.recordedCalls(), brewBinary, "upgrade", runKitLegacyFormula) {
		t.Fatal("`shll update rk` must migrate the legacy keg via the alias")
	}
	// The migration also treats the legacy keg as installed — no "not installed" error.
	if strings.Contains(stderr.String(), "not installed") {
		t.Fatalf("legacy keg must count as installed, not error; stderr=%q", stderr.String())
	}
}

func TestUpdate_MigrationDryRunShowsRealArgv(t *testing.T) {
	// Dry-run previews the REAL migration argv (`brew upgrade sahil87/tap/rk`) for a
	// legacy-keg run-kit, from the same upgradeArgv the live run uses — and performs
	// NO write.
	r := &migrationRunner{
		brewList:  map[string]string{runKitLegacyFormula: "rk 2.5.13\n"},
		afterList: "run-kit 3.0.0\n",
	}
	f := &fakeRunner{respond: r.respond}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), envFunc(nil), &stdout, &stderr, true, []string{"run-kit"}); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "run-kit  brew upgrade "+runKitLegacyFormula) {
		t.Fatalf("dry-run preview must show the real migration argv `brew upgrade %s`:\n%s", runKitLegacyFormula, out)
	}
	// No write of any kind (all writes are foreground).
	for _, c := range f.recordedCalls() {
		if c.Transport == proc.TransportForeground {
			t.Errorf("migration dry-run must spawn no foreground (write) subprocess, got %+v", c)
		}
	}
}

// --- end-of-run agent-skill refresh -------------------------------------------

// shllOnlyInstalledFake responds as if ONLY shll itself is brew-installed: the
// shll-formula `brew list` probe reports a version, every other formula probe fails
// (not installed), and everything else (brew --version, brew update, brew upgrade,
// shll agent-setup) succeeds silently. The minimal fixture for exercising the
// end-of-run refresh without roster-tool noise.
func shllOnlyInstalledFake() *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list" {
			if req.Args[len(req.Args)-1] == shllFormula {
				return proc.Result{Stdout: []byte("shll 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
}

// placeAgentSkill writes a SKILL.md at the ~/.claude skill target under a fresh
// temp HOME and returns the matching env func. content may be stale or canonical —
// the refresh guard keys on existence only.
func placeAgentSkill(t *testing.T, content string) func(string) string {
	t.Helper()
	home := t.TempDir()
	path := filepath.Join(home, ".claude", "skills", skillDirName, skillFileName)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}
	return envFunc(map[string]string{"HOME": home})
}

func TestUpdate_RefreshesPlacedAgentSkills(t *testing.T) {
	f := shllOnlyInstalledFake()
	installFakeRunner(t, f)
	env := placeAgentSkill(t, "# stale placement\n")

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), env, &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	// The refresh runs as a SUBPROCESS (`shll agent-setup`) — the new binary on
	// PATH places the new bytes, never the running (old) process in-memory.
	if !invocationsContain(f.recordedCalls(), shllTargetToken, agentSetupSub) {
		t.Fatalf("expected an `shll agent-setup` refresh subprocess, calls: %+v", f.recordedCalls())
	}
	if !strings.Contains(stdout.String(), agentSkillRefreshHeader) {
		t.Errorf("stdout must carry the refresh header %q, got:\n%s", agentSkillRefreshHeader, stdout.String())
	}
	// The refresh runs after the roster loop but is not a tool: the summary tail
	// still counts only shll (self).
	if !strings.Contains(stdout.String(), "1 of 1 tools succeeded") {
		t.Errorf("refresh must not perturb the tool count, got:\n%s", stdout.String())
	}
}

func TestUpdate_NoPlacementSkipsRefresh(t *testing.T) {
	f := shllOnlyInstalledFake()
	installFakeRunner(t, f)
	// HOME exists but holds no placed skill — the user never opted in.
	env := envFunc(map[string]string{"HOME": t.TempDir()})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), env, &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if invocationsContain(f.recordedCalls(), shllTargetToken, agentSetupSub) {
		t.Fatal("no prior placement → no unsolicited `shll agent-setup` run")
	}
	if strings.Contains(stdout.String(), agentSkillRefreshHeader) {
		t.Errorf("no placement → no refresh header, got:\n%s", stdout.String())
	}
}

func TestUpdate_DryRunPreviewsSkillRefresh(t *testing.T) {
	f := shllOnlyInstalledFake()
	installFakeRunner(t, f)
	env := placeAgentSkill(t, "# stale placement\n")

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), env, &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	// The preview mirrors the live path's placement guard (principle №5)…
	if !strings.Contains(stdout.String(), updatePreviewSkillRefreshLine) {
		t.Errorf("dry-run with a placement must preview the refresh, got:\n%s", stdout.String())
	}
	// … without running it.
	if invocationsContain(f.recordedCalls(), shllTargetToken, agentSetupSub) {
		t.Fatal("dry-run must not spawn the `shll agent-setup` refresh")
	}
}

func TestUpdate_DryRunNoPlacementOmitsRefreshLine(t *testing.T) {
	f := shllOnlyInstalledFake()
	installFakeRunner(t, f)
	env := envFunc(map[string]string{"HOME": t.TempDir()})

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), env, &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	if strings.Contains(stdout.String(), updatePreviewSkillRefreshLine) {
		t.Errorf("dry-run without a placement must not preview the refresh, got:\n%s", stdout.String())
	}
}

func TestUpdate_RefreshFailureWarnsAndContinues(t *testing.T) {
	f := shllOnlyInstalledFake()
	base := f.respond
	f.respond = func(req proc.Request) proc.Result {
		if req.Name == shllTargetToken && len(req.Args) == 1 && req.Args[0] == agentSetupSub {
			return proc.Result{ExitCode: 2} // child ran and failed; RunForeground → (2, nil)
		}
		return base(req)
	}
	installFakeRunner(t, f)
	env := placeAgentSkill(t, "# stale placement\n")

	var stdout, stderr bytes.Buffer
	// Best-effort adjunct: a failed refresh never fails the update run.
	if err := runUpdate(context.Background(), env, &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("a failed refresh must not fail the update, err = %v", err)
	}
	if want := "agent skill refresh exited 2 (continuing)"; !strings.Contains(stderr.String(), want) {
		t.Errorf("stderr = %q, want it to contain %q", stderr.String(), want)
	}
}

func TestUpdate_RefreshShllNotOnPathSkipsSilently(t *testing.T) {
	f := shllOnlyInstalledFake()
	base := f.respond
	f.respond = func(req proc.Request) proc.Result {
		if req.Name == shllTargetToken {
			return proc.Result{ExitCode: -1, Err: proc.ErrNotFound} // dev build, not on PATH
		}
		return base(req)
	}
	installFakeRunner(t, f)
	env := placeAgentSkill(t, "# stale placement\n")

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), env, &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if strings.Contains(stderr.String(), "agent skill refresh") {
		t.Errorf("shll missing from PATH must skip silently (doctor surfaces staleness), stderr = %q", stderr.String())
	}
}
