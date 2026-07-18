package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/sahil87/shll/internal/proc"
)

// --- post-install "Next steps" nudge env/golden helpers (change 93r2) ---------

// installWiredEnv returns an env func resolving zsh and pointing the rc path at a
// fresh t.TempDir() .zshrc that already contains shll's eval block, so the
// shell-setup nudge gate (resolveWiringFact → !wired) reports WIRED and the
// shell-setup line is suppressed. Existing golden-string tests use this so the only
// nudge that can fire is the (unconditional, change agst) shll agent-setup line.
// Mirrors doctor_test's rcEnv/writeWiredRC pattern; NEVER touches the real ~/.zshrc.
func installWiredEnv(t *testing.T) func(string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte("export FOO=bar\n"+tNewBlockZsh), 0o644); err != nil {
		t.Fatalf("write wired rc: %v", err)
	}
	return envFunc(map[string]string{"SHELL": "/bin/zsh", "ZDOTDIR": dir, "HOME": dir})
}

// installUnwiredEnv returns an env func resolving zsh and pointing the rc path at a
// fresh t.TempDir() .zshrc with NO shll block, so the shell-setup nudge gate fires.
func installUnwiredEnv(t *testing.T) func(string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte("export FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write unwired rc: %v", err)
	}
	return envFunc(map[string]string{"SHELL": "/bin/zsh", "ZDOTDIR": dir, "HOME": dir})
}

// agentSetupNudgeGolden is the plain (non-color, bytes.Buffer) shll agent-setup nudge
// line as it appears in stdout, INCLUDING the trailing newline. arrow(false) yields
// `->` on the non-TTY test writer. It GRADUATED from the former run-kit agent-setup
// line (change agst) and now prints unconditionally.
const agentSetupNudgeGolden = "  -> shll agent-setup    # optional, once per machine — wire agent harnesses (toolkit context + run-kit dashboard hooks)\n"

// nextStepsAgentOnly is the whole "Next steps" block a golden test sees when the
// shell-setup nudge is suppressed (wired env): the leading blank line, the header,
// then the always-printed agent-setup line only.
const nextStepsAgentOnly = "\n" + nextStepsHeader + "\n" + agentSetupNudgeGolden

func TestInstall_BrewMissing(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary {
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil)
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	// The wired env suppresses the shell-setup nudge; the shll agent-setup line is
	// unconditional (change agst), so it fires after the nothing-to-do note.
	if got, want := stdout.String(), shllSelfInstallNote+"\nAll shll tools already installed.\n"+nextStepsAgentOnly; got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
	for _, tool := range Roster {
		if invocationsContain(f.calls, brewBinary, "install", tool.Formula) {
			t.Errorf("brew install for %s should NOT be invoked when already installed", tool.Formula)
		}
	}
	// shll is never a brew-install target — it is informational only.
	if invocationsContain(f.calls, brewBinary, "install", formulaPrefix+"shll") {
		t.Errorf("shll must NOT be brew-installed (it is the running orchestrator)")
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
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
		formulaPrefix + "run-kit",
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
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
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil)
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	// Headers carry the [N/M] counter over the missing subset (M=4), each header
	// after the first is preceded by a blank line, and a blank line precedes the
	// duration-bearing tail.
	want := shllSelfInstallNote + "\n" +
		"==> [1/4] idea\n" +
		"\n==> [2/4] tu\n" +
		"\n==> [3/4] run-kit\n" +
		"\n==> [4/4] fab-kit\n" +
		"\nDone — 4 of 4 tools succeeded in 1m12s.\n" +
		nextStepsAgentOnly // wired env → shell-setup suppressed; run-kit runnable → agent-setup nudge (change 93r2)
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
	// header and no tail. The nothing-to-do note is followed by the run-kit
	// agent-setup nudge (wired env suppresses the shell-setup line; the fake
	// reports run-kit runnable — change 93r2). The nudge carries neither a `==>`
	// header nor a `Done —` tail, so the no-loop-framing assertion still holds.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	if got, want := stdout.String(), shllSelfInstallNote+"\nAll shll tools already installed.\n"+nextStepsAgentOnly; got != want {
		t.Fatalf("stdout = %q, want the shll-first note + one-line note + run-kit nudge (no header, no tail)", got)
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
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runInstall err = %v, want errSilent (one install failed)", err)
	}
	// Partial-failure tail carries the duration before the em-dash. The agent-setup
	// nudge follows the tail (nudges print regardless of anyFailed — the block is
	// informational and orthogonal to install outcome), so assert the tail appears
	// mid-stream and the nudge is the true suffix.
	if !strings.Contains(stdout.String(), "5 succeeded, 1 failed in 1m12s — see above.\n") {
		t.Fatalf("stdout = %q, want the partial-failure tail (5/1)", stdout.String())
	}
	if !strings.HasSuffix(stdout.String(), nextStepsAgentOnly) {
		t.Fatalf("stdout = %q, want the shll agent-setup nudge after the tail (nudges fire despite failures)", stdout.String())
	}
}

func TestInstall_DryRunPreview(t *testing.T) {
	// hop and wt installed; idea, tu, rk, fab-kit missing. Dry-run prints the
	// aligned-column preview of the `brew install` commands, in roster order, with
	// NO install performed.
	f := &fakeRunner{respond: installedOnly(formulaPrefix+"hop", formulaPrefix+"wt")}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, true, false, nil); err != nil {
		t.Fatalf("runInstall --dry-run err = %v, want nil", err)
	}
	// Longest missing label is "fab-kit" (7); shorter labels pad to 7. The shll-
	// first informational line precedes the preview.
	want := shllSelfInstallNote + "\n" +
		"Would install 4 tools:\n" +
		"  idea     brew install sahil87/tap/idea\n" +
		"  tu       brew install sahil87/tap/tu\n" +
		"  run-kit  brew install sahil87/tap/run-kit\n" +
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, true, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, true, false, nil); err != nil {
		t.Fatalf("runInstall --dry-run err = %v, want nil", err)
	}
	if got, want := stdout.String(), shllSelfInstallNote+"\n"+allInstalledMsg+"\n"; got != want {
		t.Fatalf("dry-run empty case stdout = %q, want the shll-first note + nothing-to-do note", got)
	}
	if strings.Contains(stdout.String(), "Would install") {
		t.Fatalf("dry-run empty case must not print a preview table, got %q", stdout.String())
	}
}

// --- shll-first informational line (change bb7r) ---

func TestInstall_ShllFirstInformationalLine(t *testing.T) {
	// Whole-roster run, all missing: the shll-first informational line is the
	// FIRST stdout line, and shll is NEVER brew-installed (it is informational).
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	firstLine := strings.SplitN(stdout.String(), "\n", 2)[0]
	if firstLine != shllSelfInstallNote {
		t.Errorf("first stdout line = %q, want the shll-first informational line %q", firstLine, shllSelfInstallNote)
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "install", formulaPrefix+"shll") {
		t.Error("shll must NOT be brew-installed — the line is informational only")
	}
	// The informational line must go to stdout, not stderr.
	if strings.Contains(stderr.String(), shllSelfInstallNote) {
		t.Errorf("shll informational line must go to stdout, not stderr; stderr = %q", stderr.String())
	}
}

// --- Subset targeting (`shll install [tool...]`, change b2vg) ---

func TestInstall_SubsetUnknownTargetHardErrors(t *testing.T) {
	// An unknown target must fail loudly BEFORE any brew work: exit non-zero,
	// stderr names the unknown arg and lists valid targets, NO brew subprocess.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{} }}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, []string{"hpo"})
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
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, []string{"shll"})
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, []string{"fab-kit", "wt"}); err != nil {
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
	for _, name := range []string{"idea", "tu", "run-kit", "hop"} {
		if invocationsContain(calls, brewBinary, "install", formulaPrefix+name) {
			t.Errorf("unnamed tool %s must NOT be installed", name)
		}
	}
	// Counter M=2 over the subset; success tail. The shll-first informational line
	// leads regardless of the named subset. run-kit isn't in this subset, but the
	// fake reports it runnable on PATH, so the post-run agent-setup nudge still fires
	// (decision 4 — the gate is "run-kit installed after the run", uniform across
	// subset runs where run-kit is present; change 93r2).
	want := shllSelfInstallNote + "\n" +
		"==> [1/2] wt\n" +
		"\n==> [2/2] fab-kit\n" +
		"\nDone — 2 of 2 tools succeeded in 1m12s.\n" +
		nextStepsAgentOnly
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, []string{"hop"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	// installedOnly reports run-kit runnable on PATH (default success for
	// `run-kit --version`), so the post-run agent-setup nudge fires after the
	// nothing-to-do note; the wired env suppresses the shell-setup line (change 93r2).
	if got, want := stdout.String(), shllSelfInstallNote+"\n"+allInstalledMsg+"\n"+nextStepsAgentOnly; got != want {
		t.Fatalf("stdout = %q, want the shll-first note + nothing-to-do note + run-kit nudge for a named-already-installed target", got)
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, true, false, []string{"fab-kit", "idea"}); err != nil {
		t.Fatalf("runInstall --dry-run subset err = %v, want nil", err)
	}
	want := shllSelfInstallNote + "\n" +
		"Would install 2 tools:\n" +
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
	// Counter correctness: only idea installed → missing subset is wt, tu, run-kit,
	// hop, fab-kit (5 tools, roster order), so headers read [1/5]..[5/5].
	f := &fakeRunner{respond: installedOnly(formulaPrefix + "idea")}
	installFakeRunner(t, f)
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	want := shllSelfInstallNote + "\n" +
		"==> [1/5] wt\n" +
		"\n==> [2/5] tu\n" +
		"\n==> [3/5] run-kit\n" +
		"\n==> [4/5] hop\n" +
		"\n==> [5/5] fab-kit\n" +
		"\nDone — 5 of 5 tools succeeded in 1m12s.\n" +
		nextStepsAgentOnly // run-kit runnable → agent-setup nudge; wired env → no shell-setup line (change 93r2)
	if got := stdout.String(); got != want {
		t.Fatalf("stdout = %q, want %q", got, want)
	}
}

// --- per-formula trust before install (change 260626-0854) --------------------

// noneInstalledTrustRunner reports brew present, `brew trust` available (so the
// trust step runs by default), and every roster formula not-installed (so every
// tool gets a `brew trust --formula` + `brew install`). trustResult lets a test
// override the per-formula trust ceremony outcome.
func noneInstalledTrustRunner(trustResult proc.Result) func(proc.Request) proc.Result {
	return func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help":
			return proc.Result{Stdout: []byte("Usage: brew trust --formula <formula>\n")}
		case req.Name == brewBinary && len(req.Args) >= 2 && req.Args[0] == "trust" && req.Args[1] == "--formula":
			return trustResult
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list":
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}
}

func TestInstall_TrustsEachFormulaBeforeInstall(t *testing.T) {
	// Default (trust on), brew ships `brew trust`, everything missing → each tool
	// is trusted per-formula BEFORE its install, in roster order, never --tap.
	f := &fakeRunner{respond: noneInstalledTrustRunner(proc.Result{ExitCode: 0})}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	for _, tool := range Roster {
		if !invocationsContain(calls, brewBinary, "trust", "--formula", tool.Formula) {
			t.Errorf("expected `brew trust --formula %s`, calls: %+v", tool.Formula, calls)
		}
		// The trust call must precede the install call for that tool.
		trustIdx, installIdx := -1, -1
		for i, c := range calls {
			if c.Name == brewBinary && len(c.Args) == 3 && c.Args[0] == "trust" && c.Args[1] == "--formula" && c.Args[2] == tool.Formula {
				trustIdx = i
			}
			if c.Name == brewBinary && len(c.Args) == 2 && c.Args[0] == "install" && c.Args[1] == tool.Formula {
				installIdx = i
			}
		}
		if trustIdx == -1 || installIdx == -1 {
			t.Fatalf("%s: missing trust/install (trust=%d, install=%d)", tool.Name, trustIdx, installIdx)
		}
		if trustIdx >= installIdx {
			t.Errorf("%s: trust (%d) must precede install (%d)", tool.Name, trustIdx, installIdx)
		}
	}
	// Per-formula, never whole-tap.
	for _, c := range calls {
		for _, a := range c.Args {
			if a == "--tap" || a == "--taps" {
				t.Fatalf("install used a whole-tap trust flag %q; want per-formula", a)
			}
		}
	}
}

func TestInstall_NoTrustSkipsTrustStep(t *testing.T) {
	// --no-trust → no `brew trust` invocation, installs still run.
	f := &fakeRunner{respond: noneInstalledTrustRunner(proc.Result{ExitCode: 0})}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, true /*noTrust*/, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	for _, c := range calls {
		if c.Name == brewBinary && len(c.Args) >= 1 && c.Args[0] == "trust" {
			t.Fatalf("--no-trust must record no `brew trust` call, got %+v", c)
		}
	}
	// Installs still happen for every missing tool.
	for _, tool := range Roster {
		if !invocationsContain(calls, brewBinary, "install", tool.Formula) {
			t.Errorf("expected brew install %s under --no-trust", tool.Formula)
		}
	}
}

func TestInstall_TrustUnavailableSkipsGracefully(t *testing.T) {
	// Older brew: `brew trust --help` errors (unrecognized) → trust unavailable, so
	// no `brew trust --formula` runs, the install proceeds, exit 0 (Constitution V).
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) >= 1 && req.Args[0] == "trust":
			return proc.Result{Err: errors.New("Error: Unknown command: trust")}
		case req.Name == brewBinary && len(req.Args) > 0 && req.Args[0] == "list":
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil (trust unavailable degrades)", err)
	}
	calls := f.recordedCalls()
	for _, c := range calls {
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "trust" && c.Args[1] == "--formula" {
			t.Fatalf("no `brew trust --formula` should run when trust is unavailable, got %+v", c)
		}
	}
	// Installs still proceed.
	if !invocationsContain(calls, brewBinary, "install", Roster[0].Formula) {
		t.Errorf("install must proceed even when trust is unavailable")
	}
}

func TestInstall_TrustFailureContinues(t *testing.T) {
	// Trust available but a per-formula trust exits non-zero → warning to stderr,
	// the install is still attempted, and the trust failure alone does NOT flip the
	// run to exit 1 (the installs all succeed here).
	f := &fakeRunner{respond: noneInstalledTrustRunner(proc.Result{ExitCode: 1})}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil (trust failure is best-effort, installs succeeded)", err)
	}
	calls := f.recordedCalls()
	// Installs were still attempted despite the trust failures.
	for _, tool := range Roster {
		if !invocationsContain(calls, brewBinary, "install", tool.Formula) {
			t.Errorf("expected brew install %s despite trust failure", tool.Formula)
		}
	}
	if !strings.Contains(stderr.String(), "trust step exited") {
		t.Errorf("stderr = %q, want a trust-failure warning", stderr.String())
	}
}

// --- rk→run-kit migration routing (change 9bak) -------------------------------

// installMigrationFake models an install-time run-kit migration state. Every roster
// formula EXCEPT run-kit is reported already installed (so the run focuses on
// run-kit); run-kit's current + legacy formulas report per legacyList/currentList
// ("" → not installed). `brew upgrade sahil87/tap/rk` + `run-kit --version` model
// the migration + post-check (linkOnUpgrade drives the linked/unlinked pathology).
// A clean migration consumes the legacy keg: after `brew upgrade`, the legacy
// formula reports not-installed, so the post-migration dual-rack re-probe finds no
// leftover (no false cleanup note). Trust probes (`brew trust --help` /
// `--formula`) succeed so the default trust-then-migrate path is exercised.
func installMigrationFake(legacyList, currentList string, linkOnUpgrade bool) *fakeRunner {
	migrated := false
	linked := false
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help":
			return proc.Result{Stdout: []byte("Usage: brew trust --formula <formula>\n")}
		case req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list":
			formula := req.Args[3]
			switch formula {
			case formulaPrefix + "rk":
				// After a clean migration the rename consumes the legacy keg → the
				// legacy formula reports not-installed (no dual-rack leftover).
				if migrated || legacyList == "" {
					return proc.Result{Err: errors.New("not installed")}
				}
				return proc.Result{Stdout: []byte(legacyList)}
			case formulaPrefix + "run-kit":
				if migrated {
					return proc.Result{Stdout: []byte("run-kit 3.0.0\n")}
				}
				if currentList == "" {
					return proc.Result{Err: errors.New("not installed")}
				}
				return proc.Result{Stdout: []byte(currentList)}
			default:
				// Every other roster tool is already installed (leaf + version).
				return proc.Result{Stdout: []byte(strings.TrimPrefix(formula, formulaPrefix) + " 1.0.0\n")}
			}
		case req.Name == brewBinary && len(req.Args) >= 2 && req.Args[0] == "upgrade" && req.Args[1] == formulaPrefix+"rk":
			// The keg migrates; whether run-kit is on PATH afterward depends on
			// linkOnUpgrade (linked vs. unlinked pathology).
			migrated = true
			if linkOnUpgrade {
				linked = true
			}
			return proc.Result{}
		case req.Name == brewBinary && len(req.Args) >= 2 && req.Args[0] == "link" && req.Args[1] == "run-kit":
			linked = true
			return proc.Result{}
		case req.Name == "run-kit" && len(req.Args) == 1 && req.Args[0] == "--version":
			if linked {
				return proc.Result{Stdout: []byte("run-kit 3.0.0\n")}
			}
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
}

func TestInstall_LegacyKegRoutesThroughMigration(t *testing.T) {
	// `shll install run-kit` on a legacy-keg machine runs the brew-direct MIGRATION
	// action (`brew upgrade sahil87/tap/rk`), NOT a blind `brew install
	// sahil87/tap/run-kit`.
	f := installMigrationFake("rk 2.5.13\n", "" /* current absent */, false)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	calls := f.calls
	if !invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"rk") {
		t.Fatalf("expected the brew-direct migration `brew upgrade sahil87/tap/rk`, calls: %+v", calls)
	}
	if invocationsContain(calls, brewBinary, "install", formulaPrefix+"run-kit") {
		t.Fatal("legacy-keg run-kit must NOT be blind-installed — it must be migrated")
	}
	// Unlinked → post-check `brew link run-kit` ran.
	if !invocationsContain(calls, brewBinary, "link", "run-kit") {
		t.Fatalf("expected `brew link run-kit` for the unlinked keg, calls: %+v", calls)
	}
}

func TestInstall_MigrationTrustsRunKitFormulaFirst(t *testing.T) {
	// The migration route must NOT skip install's per-formula trust step: installed ≠
	// trusted (doctor's trust-WARN premise, change 0854). With trust enabled (default),
	// `shll install run-kit` on a legacy-keg machine trusts the NEW formula
	// (sahil87/tap/run-kit) BEFORE running the migration `brew upgrade sahil87/tap/rk`
	// — matching the trust-then-act contract of the normal install path.
	f := installMigrationFake("rk 2.5.13\n", "" /* current absent */, true /* linked */)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false /*trust on*/, []string{"run-kit"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	// The new formula was trusted (never the legacy formula — we trust what we land on).
	if !invocationsContain(calls, brewBinary, "trust", "--formula", formulaPrefix+"run-kit") {
		t.Fatalf("expected `brew trust --formula %s` before migrating, calls: %+v", formulaPrefix+"run-kit", calls)
	}
	// Trust must PRECEDE the migration upgrade.
	trustIdx, upgradeIdx := -1, -1
	for i, c := range calls {
		if c.Name == brewBinary && len(c.Args) == 3 && c.Args[0] == "trust" && c.Args[1] == "--formula" && c.Args[2] == formulaPrefix+"run-kit" {
			trustIdx = i
		}
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "upgrade" && c.Args[1] == formulaPrefix+"rk" {
			upgradeIdx = i
		}
	}
	if trustIdx == -1 || upgradeIdx == -1 {
		t.Fatalf("missing trust/upgrade (trust=%d, upgrade=%d), calls: %+v", trustIdx, upgradeIdx, calls)
	}
	if trustIdx >= upgradeIdx {
		t.Fatalf("trust (%d) must precede the migration upgrade (%d)", trustIdx, upgradeIdx)
	}
}

func TestInstall_MigrationNoTrustSkipsTrustStep(t *testing.T) {
	// --no-trust must skip the trust step on the migration route too: no `brew trust`
	// call, but the migration upgrade still runs.
	f := installMigrationFake("rk 2.5.13\n", "", true)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, true /*noTrust*/, []string{"run-kit"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	for _, c := range calls {
		if c.Name == brewBinary && len(c.Args) >= 1 && c.Args[0] == "trust" && len(c.Args) >= 2 && c.Args[1] == "--formula" {
			t.Fatalf("--no-trust must record no `brew trust --formula` call, got %+v", c)
		}
	}
	if !invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"rk") {
		t.Fatal("the migration upgrade must still run under --no-trust")
	}
}

func TestInstall_AbsentRunKitStillBrewInstalls(t *testing.T) {
	// A fully-absent run-kit (no keg at all) still uses the plain `brew install
	// sahil87/tap/run-kit` — the migration path is ONLY for a legacy keg.
	f := installMigrationFake("" /* no legacy */, "" /* no current */, false)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	calls := f.calls
	if !invocationsContain(calls, brewBinary, "install", formulaPrefix+"run-kit") {
		t.Fatalf("absent run-kit must be `brew install`ed, calls: %+v", calls)
	}
	if invocationsContain(calls, brewBinary, "upgrade", formulaPrefix+"rk") {
		t.Fatal("an absent run-kit (no legacy keg) must NOT run the migration upgrade")
	}
}

func TestInstall_LegacyAliasResolvesWithNotice(t *testing.T) {
	// `shll install rk` resolves the alias to run-kit, prints the rename notice, and
	// (legacy keg present) routes through migration.
	f := installMigrationFake("rk 2.5.13\n", "", true /* linked after upgrade */)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, []string{"rk"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "note: rk is now run-kit") {
		t.Fatalf("out missing the `rk is now run-kit` alias notice:\n%s", stdout.String())
	}
	if !invocationsContain(f.calls, brewBinary, "upgrade", formulaPrefix+"rk") {
		t.Fatal("`shll install rk` must migrate the legacy keg via the alias")
	}
}

// --- post-install "Next steps" nudge (change 93r2) ----------------------------

// allInstalledRunKitState builds a fake where every roster formula is already
// installed (brew list succeeds) so runInstall hits the all-already-installed
// short-circuit — isolating the nudge gates from install-loop framing. runKitOnPath
// drives the run-kit presence probe (`run-kit --version` and its `rk` legacy
// fallback). Since change agst the shll agent-setup nudge is UNCONDITIONAL, so
// run-kit presence no longer gates it; the parameter is retained so tests can still
// vary the PATH state without affecting the nudge outcome.
func allInstalledRunKitState(runKitOnPath bool) *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		// run-kit / rk PATH probe (toolInstalled → probeToolVersion, incl. the
		// ErrNotFound-only legacy fallback).
		if (req.Name == "run-kit" || req.Name == "rk") && len(req.Args) == 1 && req.Args[0] == "--version" {
			if runKitOnPath {
				return proc.Result{Stdout: []byte("run-kit 3.0.0\n")}
			}
			return proc.Result{Err: proc.ErrNotFound}
		}
		// Everything else (brew --version, brew list, other tools' --version)
		// succeeds → every roster tool already installed → short-circuit.
		return proc.Result{}
	}}
}

func TestInstall_ShellSetupNudgeShownWhenUnwired(t *testing.T) {
	// Unwired rc → the shell-setup nudge fires; the shll agent-setup line is
	// unconditional (change agst), so BOTH lines appear after the nothing-to-do note.
	f := allInstalledRunKitState(false /* run-kit absent — no longer gates the nudge */)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installUnwiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, nextStepsHeader) {
		t.Fatalf("expected the %q header, got %q", nextStepsHeader, out)
	}
	if !strings.Contains(out, "shll shell-setup") || !strings.Contains(out, "exec $SHELL") {
		t.Fatalf("expected the shell-setup nudge (run 'shll shell-setup' … exec $SHELL), got %q", out)
	}
	// The agent-setup line prints unconditionally (graduated from run-kit, change agst).
	if !strings.Contains(out, "shll agent-setup") {
		t.Fatalf("expected the unconditional shll agent-setup nudge, got %q", out)
	}
	// The former run-kit agent-setup wording is gone.
	if strings.Contains(out, "run-kit agent-setup") {
		t.Fatalf("the nudge must point at 'shll agent-setup', not 'run-kit agent-setup', got %q", out)
	}
}

func TestInstall_ShellSetupNudgeHiddenWhenWired(t *testing.T) {
	// Wired rc → the shell-setup line is suppressed, but the shll agent-setup line is
	// unconditional (change agst), so the "Next steps" block STILL prints (agent-only).
	f := allInstalledRunKitState(false /* run-kit absent — irrelevant to the nudge now */)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	out := stdout.String()
	if strings.Contains(out, "shll shell-setup") {
		t.Fatalf("wired rc must suppress the shell-setup nudge, got %q", out)
	}
	// The agent-setup line still fires → the block prints with the agent line only.
	if got, want := out, shllSelfInstallNote+"\n"+allInstalledMsg+"\n"+nextStepsAgentOnly; got != want {
		t.Fatalf("stdout = %q, want the shll-note + nothing-to-do note + agent-only nudge block", got)
	}
}

func TestInstall_AgentSetupNudgeUnconditional(t *testing.T) {
	// The shll agent-setup line is UNCONDITIONAL (change agst) — it prints whether or
	// not run-kit is on PATH (shll is always present, and shll cannot cheaply know
	// whether agent-setup already ran). Wired rc isolates it from the shell-setup line.
	for _, runKitPresent := range []bool{true, false} {
		name := "run-kit absent"
		if runKitPresent {
			name = "run-kit present"
		}
		t.Run(name, func(t *testing.T) {
			f := allInstalledRunKitState(runKitPresent)
			installFakeRunner(t, f)
			var stdout, stderr bytes.Buffer
			if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
				t.Fatalf("runInstall err = %v, want nil", err)
			}
			out := stdout.String()
			if !strings.Contains(out, "shll agent-setup") {
				t.Fatalf("the shll agent-setup nudge must always show, got %q", out)
			}
			if !strings.Contains(out, "optional, once per machine") {
				t.Fatalf("agent-setup nudge must be marked 'optional, once per machine', got %q", out)
			}
		})
	}
}

func TestInstall_NoNudgesOnDryRun(t *testing.T) {
	// --dry-run is a preview, not an outcome → no nudge, even with an unwired rc and
	// run-kit present (both gates would otherwise fire). hop+wt installed so the
	// dry-run reaches the preview table (not the short-circuit).
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if (req.Name == "run-kit" || req.Name == "rk") && len(req.Args) == 1 && req.Args[0] == "--version" {
			return proc.Result{Stdout: []byte("run-kit 3.0.0\n")} // present
		}
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			formula := req.Args[3]
			if formula == formulaPrefix+"hop" || formula == formulaPrefix+"wt" {
				return proc.Result{Stdout: []byte(strings.TrimPrefix(formula, formulaPrefix) + " 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installUnwiredEnv(t), &stdout, &stderr, true /*dryRun*/, false, nil); err != nil {
		t.Fatalf("runInstall --dry-run err = %v, want nil", err)
	}
	out := stdout.String()
	if strings.Contains(out, nextStepsHeader) || strings.Contains(out, "shll shell-setup") || strings.Contains(out, "shll agent-setup") {
		t.Fatalf("--dry-run must print NO nudge (preview, not outcome), got %q", out)
	}
	// It should still be the preview table.
	if !strings.Contains(out, "Would install") {
		t.Fatalf("expected the dry-run preview table, got %q", out)
	}
}

func TestInstall_DryRunEmptyCaseNoNudge(t *testing.T) {
	// The all-already-installed short-circuit precedes the dry-run branch, but under
	// --dry-run it must still print NO nudge (decision 5 — --dry-run is nudge-free),
	// even with an unwired rc and run-kit present.
	f := allInstalledRunKitState(true /* run-kit present */)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installUnwiredEnv(t), &stdout, &stderr, true /*dryRun*/, false, nil); err != nil {
		t.Fatalf("runInstall --dry-run err = %v, want nil", err)
	}
	if got, want := stdout.String(), shllSelfInstallNote+"\n"+allInstalledMsg+"\n"; got != want {
		t.Fatalf("--dry-run empty case must be nudge-free; stdout = %q, want %q", got, want)
	}
}

func TestInstall_ShortCircuitPathNudgesWhenUnwired(t *testing.T) {
	// The all-already-installed short-circuit path (no install loop) still nudges a
	// re-runner who never wired their shell (decision 3). Unwired rc → the shell-setup
	// line fires; the agent-setup line is unconditional (change agst) → both fire.
	f := allInstalledRunKitState(true /* run-kit present — no longer gates the nudge */)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installUnwiredEnv(t), &stdout, &stderr, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	out := stdout.String()
	// The nothing-to-do note leads (short-circuit, no install loop framing).
	if !strings.HasPrefix(out, shllSelfInstallNote+"\n"+allInstalledMsg+"\n") {
		t.Fatalf("expected the short-circuit nothing-to-do note first, got %q", out)
	}
	if !strings.Contains(out, "shll shell-setup") {
		t.Fatalf("unwired short-circuit path must nudge shell-setup, got %q", out)
	}
	if !strings.Contains(out, "shll agent-setup") {
		t.Fatalf("the short-circuit path must show the unconditional agent-setup nudge, got %q", out)
	}
	// No install-loop framing on the short-circuit path.
	if strings.Contains(out, "==>") || strings.Contains(out, "Done —") {
		t.Fatalf("short-circuit path must emit no install-loop header/tail, got %q", out)
	}
}
