package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

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
	err := runInstall(context.Background(), &stdout, &stderr, false, nil)
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
	if err := runInstall(context.Background(), &stdout, &stderr, false, nil); err != nil {
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
	if err := runInstall(context.Background(), &stdout, &stderr, false, nil); err != nil {
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
	if err := runInstall(context.Background(), &stdout, &stderr, false, nil); err != nil {
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
	if err := runInstall(context.Background(), &stdout, &stderr, false, nil); err != nil {
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
	err := runInstall(context.Background(), &stdout, &stderr, false, nil)
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
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	// Headers carry the [N/M] counter over the missing subset (M=4), each header
	// after the first is preceded by a blank line, and a blank line precedes the
	// duration-bearing tail.
	want := "==> [1/4] idea\n" +
		"\n==> [2/4] tu\n" +
		"\n==> [3/4] rk\n" +
		"\n==> [4/4] fab-kit\n" +
		"\nDone — 4 of 4 tools succeeded in 1m12s.\n"
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
	if err := runInstall(context.Background(), &stdout, &stderr, false, nil); err != nil {
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
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	err := runInstall(context.Background(), &stdout, &stderr, false, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runInstall err = %v, want errSilent (one install failed)", err)
	}
	// Partial-failure tail carries the duration before the em-dash.
	if !strings.HasSuffix(stdout.String(), "5 succeeded, 1 failed in 1m12s — see above.\n") {
		t.Fatalf("stdout = %q, want to end with partial-failure tail (5/1)", stdout.String())
	}
}

func TestInstall_DryRunPreview(t *testing.T) {
	// hop and wt installed; idea, tu, rk, fab-kit missing. Dry-run prints the
	// aligned-column preview of the `brew install` commands, in roster order, with
	// NO install performed.
	f := &fakeRunner{respond: installedOnly(formulaPrefix+"hop", formulaPrefix+"wt")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runInstall --dry-run err = %v, want nil", err)
	}
	// Longest missing label is "fab-kit" (7); shorter labels pad to 7.
	want := "Would install 4 tools:\n" +
		"  idea     brew install sahil87/tap/idea\n" +
		"  tu       brew install sahil87/tap/tu\n" +
		"  rk       brew install sahil87/tap/rk\n" +
		"  fab-kit  brew install sahil87/tap/fab-kit\n"
	if got := stdout.String(); got != want {
		t.Fatalf("dry-run preview =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(stdout.String(), "metadata refresh") {
		t.Fatalf("install dry-run must not mention metadata refresh, got %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestInstall_DryRunNoWrites(t *testing.T) {
	// Dry-run runs the isInstalled probes but performs NO `brew install`. Everything
	// missing → all six would be installed, but none actually are.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list" {
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runInstall --dry-run err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	// Read-only probe (brew list) IS present.
	if !invocationsContain(calls, brewBinary, "list", "--formula", "--versions", formulaPrefix+"wt") {
		t.Errorf("expected brew list probe, calls: %+v", calls)
	}
	// No `brew install` write, and no foreground transport at all.
	for _, tool := range Roster {
		if invocationsContain(calls, brewBinary, "install", tool.Formula) {
			t.Errorf("brew install %s must NOT run in dry-run", tool.Formula)
		}
	}
	for _, c := range calls {
		if c.Transport == proc.TransportForeground {
			t.Errorf("dry-run must spawn no foreground (write) subprocess, got %+v", c)
		}
	}
}

func TestInstall_DryRunEmptyCase(t *testing.T) {
	// Everything already installed → dry-run mirrors the non-dry-run nothing-to-do
	// message, exit 0, no preview table, no installs.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr, true, nil); err != nil {
		t.Fatalf("runInstall --dry-run err = %v, want nil", err)
	}
	if got := stdout.String(); got != allInstalledMsg+"\n" {
		t.Fatalf("dry-run empty case stdout = %q, want the nothing-to-do note", got)
	}
	if strings.Contains(stdout.String(), "Would install") {
		t.Fatalf("dry-run empty case must not print a preview table, got %q", stdout.String())
	}
}

// --- Subset targeting (`shll install [tool...]`, change b2vg) ---

func TestInstall_SubsetUnknownTargetHardErrors(t *testing.T) {
	// An unknown target must fail loudly BEFORE any brew work: exit non-zero,
	// stderr names the unknown arg and lists valid targets, NO brew subprocess.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runInstall(context.Background(), &stdout, &stderr, false, []string{"hpo"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runInstall err = %v, want errSilent for unknown target", err)
	}
	if !strings.Contains(stderr.String(), `"hpo"`) {
		t.Errorf("stderr = %q, want to name the unknown arg %q", stderr.String(), "hpo")
	}
	if !strings.Contains(stderr.String(), "wt") || !strings.Contains(stderr.String(), "fab-kit") {
		t.Errorf("stderr = %q, want to list valid roster targets", stderr.String())
	}
	if len(f.recordedCalls()) != 0 {
		t.Fatalf("expected NO subprocess calls on unknown target, got %+v", f.recordedCalls())
	}
}

func TestInstall_SubsetShllRejected(t *testing.T) {
	// `shll install shll` → shll is NOT a valid install target (cannot brew-install
	// the running orchestrator) → the unknown-target hard error, no brew work.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runInstall(context.Background(), &stdout, &stderr, false, []string{"shll"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runInstall err = %v, want errSilent for `shll install shll`", err)
	}
	if !strings.Contains(stderr.String(), `"shll"`) {
		t.Fatalf("stderr = %q, want to reject `shll` as an unknown install target", stderr.String())
	}
	// The valid-target list must NOT advertise shll (it is roster-only).
	if strings.Contains(stderr.String(), "valid targets: shll") {
		t.Errorf("stderr = %q, install valid-target list must NOT include shll", stderr.String())
	}
	if len(f.recordedCalls()) != 0 {
		t.Fatalf("expected NO subprocess calls, got %+v", f.recordedCalls())
	}
}

func TestInstall_SubsetArgOrderIndependentRosterOrder(t *testing.T) {
	// `shll install fab-kit wt` (both missing) → installed in roster order: wt
	// before fab-kit, regardless of arg order. M=2.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list" {
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr, false, []string{"fab-kit", "wt"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	wtIdx, fabIdx := -1, -1
	for i, c := range calls {
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "install" && c.Args[1] == formulaPrefix+"wt" {
			wtIdx = i
		}
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "install" && c.Args[1] == formulaPrefix+"fab-kit" {
			fabIdx = i
		}
	}
	if wtIdx == -1 || fabIdx == -1 {
		t.Fatalf("missing wt/fab-kit installs (wt=%d, fab-kit=%d), calls: %+v", wtIdx, fabIdx, calls)
	}
	if wtIdx >= fabIdx {
		t.Fatalf("wt (%d) must be installed before fab-kit (%d) — roster order, not arg order", wtIdx, fabIdx)
	}
	// Unnamed tools are NOT installed.
	for _, name := range []string{"idea", "tu", "rk", "hop"} {
		if invocationsContain(calls, brewBinary, "install", formulaPrefix+name) {
			t.Errorf("unnamed tool %s must NOT be installed", name)
		}
	}
	// Counter M=2 over the subset; success tail.
	want := "==> [1/2] wt\n" +
		"\n==> [2/2] fab-kit\n" +
		"\nDone — 2 of 2 tools succeeded in 1m12s.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

func TestInstall_SubsetNamedAlreadyInstalled(t *testing.T) {
	// `shll install hop` when hop is already installed → the named target is
	// filtered out (idempotent skip), so the subset is empty → nothing-to-do note,
	// exit 0, no install.
	f := &fakeRunner{respond: installedOnly(formulaPrefix + "hop")}
	// installedOnly reports ONLY hop installed; flip so hop IS installed by
	// reusing it directly (hop installed, others not — but they're not named).
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr, false, []string{"hop"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	if got := stdout.String(); got != allInstalledMsg+"\n" {
		t.Fatalf("stdout = %q, want the nothing-to-do note for a named-already-installed target", got)
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "install", formulaPrefix+"hop") {
		t.Fatal("already-installed named target must NOT be re-installed")
	}
}

func TestInstall_SubsetDryRunPreviewFiltered(t *testing.T) {
	// `shll install --dry-run idea fab-kit` (both missing) → preview lists exactly
	// the two-tool subset in roster order (idea then fab-kit), no install.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list" {
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr, true, []string{"fab-kit", "idea"}); err != nil {
		t.Fatalf("runInstall --dry-run subset err = %v, want nil", err)
	}
	want := "Would install 2 tools:\n" +
		"  idea     brew install sahil87/tap/idea\n" +
		"  fab-kit  brew install sahil87/tap/fab-kit\n"
	if got := stdout.String(); got != want {
		t.Fatalf("subset dry-run preview =\n%q\nwant\n%q", got, want)
	}
	for _, c := range f.recordedCalls() {
		if c.Transport == proc.TransportForeground {
			t.Errorf("subset dry-run must spawn no foreground (write) subprocess, got %+v", c)
		}
	}
}

func TestInstall_CounterPartialInstall(t *testing.T) {
	// Counter correctness: only idea installed → missing subset is wt, tu, rk, hop,
	// fab-kit (5 tools, roster order), so headers read [1/5]..[5/5].
	f := &fakeRunner{respond: installedOnly(formulaPrefix + "idea")}
	installFakeRunner(t, f)
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), &stdout, &stderr, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	want := "==> [1/5] wt\n" +
		"\n==> [2/5] tu\n" +
		"\n==> [3/5] rk\n" +
		"\n==> [4/5] hop\n" +
		"\n==> [5/5] fab-kit\n" +
		"\nDone — 5 of 5 tools succeeded in 1m12s.\n"
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}
