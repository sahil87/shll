package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

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
	err := runUpdate(context.Background(), &stdout, &stderr, false)
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	// The status line prints first (unconditionally, before the short-circuit),
	// then the nothing-to-do message.
	wantOut := updateStatusLine + "\nNo sahil87 tools installed.\n"
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	err := runUpdate(context.Background(), &stdout, &stderr, false)
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
	err := runUpdate(context.Background(), &stdout, &stderr, false)
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

// installedOnly returns a respond function where only the named formulas report
// installed (via `brew list`), with all other requests succeeding (empty stdout).
// shll itself is reported not-brew-installed so the self-upgrade is skipped and
// the test stays focused on roster delegation.
func installedOnly(formulas ...string) func(proc.Request) proc.Result {
	set := make(map[string]bool, len(formulas))
	for _, f := range formulas {
		set[f] = true
	}
	return func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			if set[req.Args[3]] {
				return proc.Result{Stdout: []byte(req.Args[3] + " 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}
}

func TestUpdate_FlagSupported(t *testing.T) {
	// rk is installed and `rk update --help` advertises --skip-brew-update → rk
	// is upgraded via `rk update --skip-brew-update`, NOT brew upgrade.
	base := installedOnly(formulaPrefix + "rk")
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "rk" && isUpdateHelpProbe(req) {
			return helpAdvertisesSkipFlag()
		}
		return base(req)
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, "rk", "update", skipBrewUpdateFlag) {
		t.Fatalf("expected `rk update %s`, calls: %+v", skipBrewUpdateFlag, calls)
	}
	if invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"rk") {
		t.Fatal("should NOT brew upgrade rk — must delegate to `rk update --skip-brew-update`")
	}
	if invocationsContain(calls, "rk", "update") {
		t.Fatal("expected the flagged form, not a bare `rk update`")
	}
}

func TestUpdate_FlagUnsupportedVersionSkew(t *testing.T) {
	// hop is installed but `hop update --help` does NOT advertise the flag
	// (version skew) → hop is upgraded via `hop update` with no flag, and does
	// NOT fall back to brew upgrade.
	f := &fakeRunner{respond: installedOnly(formulaPrefix + "hop")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
		formulaPrefix+"rk",
		formulaPrefix+"hop",
		formulaPrefix+"wt",
	)}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	// Deterministic clock: t0 then t0+72s → the tail reads `in 1m12s`.
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}

	// Headers now carry the [N/M] counter (shll (self) is [1/7] and first), each
	// header after the first is preceded by a blank line, and a blank line precedes
	// the duration-bearing tail.
	want := updateStatusLine + "\n" +
		"==> [1/7] shll (self)\n" +
		"\n==> [2/7] wt\n" +
		"\n==> [3/7] idea\n" +
		"\n==> [4/7] tu\n" +
		"\n==> [5/7] rk\n" +
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

	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
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
	err := runUpdate(context.Background(), &stdout, &stderr, false)
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
	if err := runUpdate(context.Background(), &stdout, &stderr, false); err != nil {
		t.Fatalf("runUpdate err = %v, want nil", err)
	}
	if got := stdout.String(); got != updateStatusLine+"\nNo sahil87 tools installed.\n" {
		t.Fatalf("stdout = %q, want status line + note only (no header, no tail)", got)
	}
	if strings.Contains(stdout.String(), "==>") || strings.Contains(stdout.String(), "Done —") {
		t.Fatalf("empty case must emit no header and no tail, got %q", stdout.String())
	}
}

func TestUpdate_DryRunPreview(t *testing.T) {
	// shll itself NOT brew-installed (installedOnly); the full roster installed; rk
	// and hop advertise --skip-brew-update, the rest do not. Dry-run prints the
	// aligned-column preview with the exact per-tool argv, in roster order.
	base := installedOnly(
		formulaPrefix+"wt", formulaPrefix+"idea", formulaPrefix+"tu",
		formulaPrefix+"rk", formulaPrefix+"hop", formulaPrefix+"fab-kit",
	)
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if (req.Name == "rk" || req.Name == "hop") && isUpdateHelpProbe(req) {
			return helpAdvertisesSkipFlag()
		}
		return base(req)
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr, true); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	// Longest label is "fab-kit" (7) since shll (self) is absent here; labels are
	// padded to 7. rk and hop carry the flag; wt/idea/tu/fab-kit do not.
	want := updateStatusLine + "\n" +
		"Would update 6 tools (brew metadata refresh first):\n" +
		"  wt       wt update\n" +
		"  idea     idea update\n" +
		"  tu       tu update\n" +
		"  rk       rk update --skip-brew-update\n" +
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
	// shll itself brew-installed + full roster; no tool advertises the flag. The
	// preview lists shll (self) FIRST with `brew upgrade sahil87/tap/shll`, and
	// "shll (self)" (11 chars) is the widest label, so commands align under it.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUpdate(context.Background(), &stdout, &stderr, true); err != nil {
		t.Fatalf("runUpdate --dry-run err = %v, want nil", err)
	}
	want := updateStatusLine + "\n" +
		"Would update 7 tools (brew metadata refresh first):\n" +
		"  shll (self)  brew upgrade sahil87/tap/shll\n" +
		"  wt           wt update\n" +
		"  idea         idea update\n" +
		"  tu           tu update\n" +
		"  rk           rk update\n" +
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
	if err := runUpdate(context.Background(), &stdout, &stderr, true); err != nil {
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
	if err := runUpdate(context.Background(), &stdout, &stderr, true); err != nil {
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
	if err := runUpdate(context.Background(), &stdout, &stderr, true); err != nil {
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
