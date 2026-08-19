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

// --- post-install env/golden helpers (change 93r2; auto-run change gjhx) -------

// installEnvDir returns an env func resolving the given shell and pointing the rc
// path and $HOME at a fresh t.TempDir() holding a .zshrc with rcContent, plus the
// dir itself — for tests that assert on the rc file or the files the agent-setup
// step writes under $HOME. NEVER touches the real ~/.zshrc or $HOME.
func installEnvDir(t *testing.T, shell, rcContent string) (func(string) string, string) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(rcContent), 0o644); err != nil {
		t.Fatalf("write rc: %v", err)
	}
	return envFunc(map[string]string{"SHELL": shell, "ZDOTDIR": dir, "HOME": dir}), dir
}

// installWiredEnv returns an env func resolving zsh and pointing the rc path at a
// fresh t.TempDir() .zshrc that already contains shll's eval block, so the
// resolveWiringFact gate reports WIRED and the auto shell-setup step is a silent
// skip. Mirrors doctor_test's rcEnv/writeWiredRC pattern; NEVER touches the real
// ~/.zshrc.
func installWiredEnv(t *testing.T) func(string) string {
	t.Helper()
	env, _ := installEnvDir(t, "/bin/zsh", "export FOO=bar\n"+tNewBlockZsh)
	return env
}

// installWiredEnvDir is installWiredEnv plus the temp dir.
func installWiredEnvDir(t *testing.T) (func(string) string, string) {
	t.Helper()
	return installEnvDir(t, "/bin/zsh", "export FOO=bar\n"+tNewBlockZsh)
}

// installUnwiredEnv returns an env func resolving zsh and pointing the rc path at a
// fresh t.TempDir() .zshrc with NO shll block, so the resolveWiringFact gate is
// open (shellResolved && !corrupt && !wired).
func installUnwiredEnv(t *testing.T) func(string) string {
	t.Helper()
	env, _ := installEnvDir(t, "/bin/zsh", "export FOO=bar\n")
	return env
}

// agentSetupNudgeGolden is the plain (non-color, bytes.Buffer) shll agent-setup nudge
// line as it appears in stdout, INCLUDING the trailing newline. arrow(false) yields
// `->` on the non-TTY test writer. It GRADUATED from the former run-kit agent-setup
// line (change agst); since the auto-run change (gjhx) it prints as the
// --no-agent-setup / failed-step fallback.
const agentSetupNudgeGolden = "  -> shll agent-setup    # optional, once per machine — wire agent harnesses (toolkit context + run-kit dashboard hooks)\n"

// nextStepsAgentOnly is the whole "Next steps" block a golden test sees when the
// shell step is quiet (wired env) and the agent step was opted out of
// (--no-agent-setup): the leading blank line, the header, then the agent-setup
// nudge line only.
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
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil)
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	// The wired env makes the auto shell-setup a silent skip; --no-agent-setup
	// opts out of the agent step, so its nudge prints as the fallback after the
	// nothing-to-do note (keeping this golden deterministic — an auto agent-setup
	// would print temp-dir placement paths).
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	for _, tool := range Roster {
		if !invocationsContain(f.calls, brewBinary, "install", tool.Formula) {
			t.Errorf("expected brew install %s, calls: %+v", tool.Formula, f.calls)
		}
	}
	// The loop path also ran the post-install agent-setup auto-run (the wired env
	// silent-skips the shell step): the run-kit delegation is forwarded --yes.
	if !invocationsContain(f.calls, runKitToolName, "agent", "setup", "--yes") {
		t.Errorf("expected the auto agent-setup delegation `run-kit agent setup --yes`, calls: %+v", f.calls)
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil); err != nil {
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
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil)
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil); err != nil {
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
		nextStepsAgentOnly // wired env → shell step silent-skips; --no-agent-setup → the agent nudge prints as the opt-out fallback (change gjhx)
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
	// header and no tail. The nothing-to-do note is followed by the agent-setup
	// nudge (--no-agent-setup opt-out fallback; the wired env silent-skips the
	// shell step — change gjhx). The nudge carries neither a `==>` header nor a
	// `Done —` tail, so the no-loop-framing assertion still holds.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil); err != nil {
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
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runInstall err = %v, want errSilent (one install failed)", err)
	}
	// Partial-failure tail carries the duration before the em-dash. The agent-setup
	// nudge (opt-out fallback) follows the tail — the post-install block is
	// orthogonal to install outcome, so it prints regardless of anyFailed — so
	// assert the tail appears mid-stream and the nudge is the true suffix.
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, true, false, false, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, true, false, false, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, true, false, false, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil); err != nil {
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
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, []string{"hpo"})
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
	err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, []string{"shll"})
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, true /*noAgentSetup*/, []string{"fab-kit", "wt"}); err != nil {
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
	// leads regardless of the named subset. The run passes --no-agent-setup, so the
	// agent-setup nudge prints as the opt-out fallback after the tail (wired env →
	// the shell step silent-skips).
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, true /*noAgentSetup*/, []string{"hop"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	// The run passes --no-agent-setup, so the agent-setup nudge prints as the
	// opt-out fallback after the nothing-to-do note; the wired env silent-skips
	// the shell step (change gjhx).
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, true, false, false, false, []string{"fab-kit", "idea"}); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	want := shllSelfInstallNote + "\n" +
		"==> [1/5] wt\n" +
		"\n==> [2/5] tu\n" +
		"\n==> [3/5] run-kit\n" +
		"\n==> [4/5] hop\n" +
		"\n==> [5/5] fab-kit\n" +
		"\nDone — 5 of 5 tools succeeded in 1m12s.\n" +
		nextStepsAgentOnly // --no-agent-setup → the agent nudge prints as the opt-out fallback; wired env → shell step silent-skips (change gjhx)
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, true /*noTrust*/, false, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil); err != nil {
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
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, nil); err != nil {
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

// --- run-kit after the migration-guard retirement (change h3f6) ----------------

// installRunKitMissingFake models a machine where every roster formula EXCEPT
// run-kit is already installed and run-kit's formula (sahil87/tap/run-kit) is
// not — the shape of BOTH a fresh machine and a never-migrated legacy-only
// machine (the legacy `rk` keg is invisible to shll: it never probes
// sahil87/tap/rk anymore). Trust probes succeed so the default trust-then-install
// path is exercised.
func installRunKitMissingFake() *fakeRunner {
	installed := false
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help":
			return proc.Result{Stdout: []byte("Usage: brew trust --formula <formula>\n")}
		case req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list":
			formula := req.Args[3]
			if formula == formulaPrefix+"run-kit" {
				if installed {
					return proc.Result{Stdout: []byte("run-kit 3.0.0\n")}
				}
				return proc.Result{Err: errors.New("not installed")}
			}
			// Every other roster tool is already installed.
			return proc.Result{Stdout: []byte(strings.TrimPrefix(formula, formulaPrefix) + " 1.0.0\n")}
		case req.Name == brewBinary && len(req.Args) >= 2 && req.Args[0] == "install" && req.Args[1] == formulaPrefix+"run-kit":
			installed = true
			return proc.Result{}
		}
		return proc.Result{}
	}}
}

func TestInstall_LegacyOnlyMachineBrewInstallsRunKit(t *testing.T) {
	// A legacy-only machine classifies run-kit as MISSING (the plain
	// isInstalled(sahil87/tap/run-kit) check): `shll install run-kit` records
	// per-formula trust then runs the normal bootstrap `brew install
	// sahil87/tap/run-kit`. No call ever references the retired legacy formula and
	// no migration upgrade runs (orphan `rk` keg cleanup is manual per run-kit's
	// README).
	f := installRunKitMissingFake()
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	assertNoLegacyFormulaReference(t, calls)
	if !invocationsContain(calls, brewBinary, "trust", "--formula", formulaPrefix+"run-kit") {
		t.Fatalf("expected `brew trust --formula %s` before the install, calls: %+v", formulaPrefix+"run-kit", calls)
	}
	if !invocationsContain(calls, brewBinary, "install", formulaPrefix+"run-kit") {
		t.Fatalf("missing run-kit must be `brew install`ed, calls: %+v", calls)
	}
	for _, c := range calls {
		if c.Name == brewBinary && len(c.Args) > 0 && c.Args[0] == "upgrade" {
			t.Fatalf("install must never run a migration `brew upgrade`, got %+v", c)
		}
	}
}

func TestInstall_LegacyAliasResolvesWithNotice(t *testing.T) {
	// `shll install rk` resolves the alias to run-kit, prints the rename notice, and
	// bootstraps the new formula like any missing tool.
	f := installRunKitMissingFake()
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, false, []string{"rk"}); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "note: rk is now run-kit") {
		t.Fatalf("out missing the `rk is now run-kit` alias notice:\n%s", stdout.String())
	}
	if !invocationsContain(f.calls, brewBinary, "install", formulaPrefix+"run-kit") {
		t.Fatal("`shll install rk` must brew-install the canonical run-kit formula")
	}
	assertNoLegacyFormulaReference(t, f.recordedCalls())
}

// --- post-install auto-run steps + "Next steps" block (93r2, agst, gjhx) --------

// allInstalledRunKitState builds a fake where every roster formula is already
// installed (brew list succeeds) so runInstall hits the all-already-installed
// short-circuit — isolating the post-install steps from install-loop framing.
// runKitOnPath drives the run-kit presence probe (`run-kit --version` and its `rk`
// legacy fallback). Note the agent-setup auto-run's DELEGATION call
// (`run-kit agent setup --yes`) does not match the --version probe shape, so it
// falls through to the default success regardless of runKitOnPath — tests that need
// the delegation itself to fail or be absent build a dedicated fake.
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
	// Both opt-outs restore the nudge-era block: with --no-shell-setup and
	// --no-agent-setup on an unwired machine, BOTH nudge lines appear after the
	// nothing-to-do note — and neither step writes anything (change gjhx).
	f := allInstalledRunKitState(false /* run-kit absent — irrelevant to the nudges */)
	installFakeRunner(t, f)

	env, dir := installEnvDir(t, "/bin/zsh", "export FOO=bar\n")
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, false, false, true /*noShellSetup*/, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, nextStepsHeader) {
		t.Fatalf("expected the %q header, got %q", nextStepsHeader, out)
	}
	if !strings.Contains(out, "shll shell-setup") || !strings.Contains(out, "exec $SHELL") {
		t.Fatalf("expected the shell-setup nudge (run 'shll shell-setup' … exec $SHELL), got %q", out)
	}
	if !strings.Contains(out, "shll agent-setup") {
		t.Fatalf("expected the agent-setup nudge, got %q", out)
	}
	// The former run-kit agent-setup wording is gone.
	if strings.Contains(out, "run-kit agent-setup") {
		t.Fatalf("the nudge must point at 'shll agent-setup', not 'run-kit agent-setup', got %q", out)
	}
	// No writes: the rc file is untouched, no skill files exist under $HOME, and
	// no run-kit delegation ran.
	rc, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
	if err != nil {
		t.Fatalf("read rc: %v", err)
	}
	if got, want := string(rc), "export FOO=bar\n"; got != want {
		t.Fatalf("--no-shell-setup must not write the rc file: got %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("--no-agent-setup must write no skill files under $HOME")
	}
	if invocationsContain(f.recordedCalls(), runKitToolName, "agent", "setup", "--yes") {
		t.Fatalf("--no-agent-setup must not invoke the run-kit delegation")
	}
}

func TestInstall_ShellSetupNudgeHiddenWhenWired(t *testing.T) {
	// Wired rc + both steps opted out → the shell-setup line is suppressed (its
	// gate is closed), so the "Next steps" block prints with the agent-setup
	// opt-out fallback line only (change gjhx).
	f := allInstalledRunKitState(false /* run-kit absent — irrelevant to the nudges */)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, true /*noShellSetup*/, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	out := stdout.String()
	if strings.Contains(out, "shll shell-setup") {
		t.Fatalf("wired rc must suppress the shell-setup nudge, got %q", out)
	}
	// The agent-setup opt-out fallback fires → the block prints agent-only.
	if got, want := out, shllSelfInstallNote+"\n"+allInstalledMsg+"\n"+nextStepsAgentOnly; got != want {
		t.Fatalf("stdout = %q, want the shll-note + nothing-to-do note + agent-only nudge block", got)
	}
}

func TestInstall_AgentSetupNudgeOnOptOut(t *testing.T) {
	// The agent-setup nudge is the --no-agent-setup fallback (change gjhx) — it
	// prints whether or not run-kit is on PATH (the nudge points at `shll
	// agent-setup`, and shll is by definition present). Wired rc isolates it from
	// the shell-setup line.
	for _, runKitPresent := range []bool{true, false} {
		name := "run-kit absent"
		if runKitPresent {
			name = "run-kit present"
		}
		t.Run(name, func(t *testing.T) {
			f := allInstalledRunKitState(runKitPresent)
			installFakeRunner(t, f)
			var stdout, stderr bytes.Buffer
			if err := runInstall(context.Background(), installWiredEnv(t), &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil); err != nil {
				t.Fatalf("runInstall err = %v, want nil", err)
			}
			out := stdout.String()
			if !strings.Contains(out, "shll agent-setup") {
				t.Fatalf("the shll agent-setup nudge must show on opt-out, got %q", out)
			}
			if !strings.Contains(out, "optional, once per machine") {
				t.Fatalf("agent-setup nudge must be marked 'optional, once per machine', got %q", out)
			}
		})
	}
}

func TestInstall_NoNudgesOnDryRun(t *testing.T) {
	// --dry-run is a preview, not an outcome → no nudge, and NEITHER auto-run step
	// runs (the steps are writes), even with an unwired rc and run-kit present.
	// hop+wt installed so the dry-run reaches the preview table (not the
	// short-circuit).
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

	env, dir := installEnvDir(t, "/bin/zsh", "export FOO=bar\n")
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, true /*dryRun*/, false, false, false, nil); err != nil {
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
	// Neither auto-run step ran: rc untouched, no skill files, no run-kit delegation.
	assertDryRunNoSetupWrites(t, f, dir)
}

// assertDryRunNoSetupWrites asserts the dry-run contract on the post-install
// auto-run steps (change gjhx): the rc file still holds exactly its unwired
// content (no sentinel block appended), no skill files exist under $HOME, and no
// run-kit delegation was invoked.
func assertDryRunNoSetupWrites(t *testing.T, f *fakeRunner, dir string) {
	t.Helper()
	rc, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
	if err != nil {
		t.Fatalf("read rc: %v", err)
	}
	if got, want := string(rc), "export FOO=bar\n"; got != want {
		t.Fatalf("dry-run must not wire the rc file: got %q, want %q", got, want)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("dry-run must write no skill files under $HOME")
	}
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName {
			t.Fatalf("dry-run must not invoke run-kit, got %+v", c)
		}
	}
}

func TestInstall_DryRunEmptyCaseNoNudge(t *testing.T) {
	// The all-already-installed short-circuit precedes the dry-run branch, but under
	// --dry-run it must still print NO nudge and run NEITHER auto-run step
	// (--dry-run is write-free by contract), even with an unwired rc and run-kit
	// present.
	f := allInstalledRunKitState(true /* run-kit present */)
	installFakeRunner(t, f)

	env, dir := installEnvDir(t, "/bin/zsh", "export FOO=bar\n")
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, true /*dryRun*/, false, false, false, nil); err != nil {
		t.Fatalf("runInstall --dry-run err = %v, want nil", err)
	}
	if got, want := stdout.String(), shllSelfInstallNote+"\n"+allInstalledMsg+"\n"; got != want {
		t.Fatalf("--dry-run empty case must be nudge-free; stdout = %q, want %q", got, want)
	}
	assertDryRunNoSetupWrites(t, f, dir)
}

func TestInstall_ShortCircuitPathNudgesWhenUnwired(t *testing.T) {
	// The all-already-installed short-circuit path (no install loop) reaches the
	// post-install block too: with both steps opted out, an unwired re-runner gets
	// both nudge lines (change gjhx — the nudge-era "re-runner still gets nudged"
	// decision becomes "opted-out re-runner still gets nudged"; by default the
	// short-circuit path WIRES them instead — see TestInstall_AutoShellSetupWiresRcFile).
	f := allInstalledRunKitState(true /* run-kit present */)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), installUnwiredEnv(t), &stdout, &stderr, false, false, true /*noShellSetup*/, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	out := stdout.String()
	// The nothing-to-do note leads (short-circuit, no install loop framing).
	if !strings.HasPrefix(out, shllSelfInstallNote+"\n"+allInstalledMsg+"\n") {
		t.Fatalf("expected the short-circuit nothing-to-do note first, got %q", out)
	}
	if !strings.Contains(out, "shll shell-setup") {
		t.Fatalf("unwired short-circuit path must nudge shell-setup when the step is opted out, got %q", out)
	}
	if !strings.Contains(out, "shll agent-setup") {
		t.Fatalf("the short-circuit path must show the agent-setup opt-out nudge, got %q", out)
	}
	// No install-loop framing on the short-circuit path.
	if strings.Contains(out, "==>") || strings.Contains(out, "Done —") {
		t.Fatalf("short-circuit path must emit no install-loop header/tail, got %q", out)
	}
}

// --- post-install auto-run steps (change gjhx) --------------------------------

func TestInstall_AutoShellSetupWiresRcFile(t *testing.T) {
	// Fresh machine on the short-circuit path: resolvable $SHELL, existing unwired
	// rc → the auto shell-setup appends the sentinel block (shell-setup's own
	// output announces the wire) and the Next steps block carries the exec $SHELL
	// reminder instead of the nudge. --no-agent-setup isolates the shell step.
	f := allInstalledRunKitState(false)
	installFakeRunner(t, f)

	env, dir := installEnvDir(t, "/bin/zsh", "export FOO=bar\n")
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	rc, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
	if err != nil {
		t.Fatalf("read rc: %v", err)
	}
	if got, want := string(rc), "export FOO=bar\n"+tNewBlockZsh; got != want {
		t.Fatalf("rc = %q, want the sentinel block appended (%q)", got, want)
	}
	out := stdout.String()
	if !strings.Contains(out, "Installed shll shell integration") {
		t.Fatalf("expected shell-setup's own install announcement, got %q", out)
	}
	if !strings.Contains(out, "exec $SHELL") {
		t.Fatalf("expected the exec $SHELL reminder after a successful wire, got %q", out)
	}
	if strings.Contains(out, "shll shell-setup") {
		t.Fatalf("a successful auto wire must not print the shell-setup nudge, got %q", out)
	}
}

func TestInstall_AutoShellSetupIdempotentRewire(t *testing.T) {
	// The auto-run inherits shell-setup's idempotency: running install twice wires
	// the first time and silent-skips the second (the gate re-reads the now-wired
	// rc) — the file is byte-identical and no reminder prints on the second run.
	f := allInstalledRunKitState(false)
	installFakeRunner(t, f)

	env, dir := installEnvDir(t, "/bin/zsh", "export FOO=bar\n")
	var out1 bytes.Buffer
	if err := runInstall(context.Background(), env, &out1, &bytes.Buffer{}, false, false, false, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("first runInstall err = %v, want nil", err)
	}
	wired, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
	if err != nil {
		t.Fatalf("read rc: %v", err)
	}
	var out2 bytes.Buffer
	if err := runInstall(context.Background(), env, &out2, &bytes.Buffer{}, false, false, false, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("second runInstall err = %v, want nil", err)
	}
	again, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
	if err != nil {
		t.Fatalf("re-read rc: %v", err)
	}
	if !bytes.Equal(wired, again) {
		t.Fatalf("second run must be a byte-identical no-op:\nfirst:  %q\nsecond: %q", wired, again)
	}
	if strings.Contains(out2.String(), "exec $SHELL") || strings.Contains(out2.String(), "shll shell-setup") {
		t.Fatalf("already-wired re-run must print neither reminder nor nudge, got %q", out2.String())
	}
}

func TestInstall_AutoShellSetupQuietSkips(t *testing.T) {
	// The 93r2 quiet edge states survive the auto-run: an already-wired rc, an
	// unresolvable $SHELL (e.g. fish), and a corrupt (open-without-close) block all
	// skip the shell step silently — no write, no nudge, no reminder. --no-agent-setup
	// isolates the shell step (its nudge is the only Next steps line).
	corrupt := "export FOO=bar\n# >>> shll >>>\neval \"$(shll shell-init zsh)\"\n"
	cases := map[string]struct {
		shell     string
		rcContent string
	}{
		"wired":               {"/bin/zsh", "export FOO=bar\n" + tNewBlockZsh},
		"unresolvable $SHELL": {"/usr/bin/fish", "export FOO=bar\n"},
		"corrupt block":       {"/bin/zsh", corrupt},
	}
	for name, tc := range cases {
		t.Run(name, func(t *testing.T) {
			f := allInstalledRunKitState(false)
			installFakeRunner(t, f)

			env, dir := installEnvDir(t, tc.shell, tc.rcContent)
			var stdout, stderr bytes.Buffer
			if err := runInstall(context.Background(), env, &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil); err != nil {
				t.Fatalf("runInstall err = %v, want nil", err)
			}
			rc, err := os.ReadFile(filepath.Join(dir, ".zshrc"))
			if err != nil {
				t.Fatalf("read rc: %v", err)
			}
			if got := string(rc); got != tc.rcContent {
				t.Fatalf("quiet edge state must not modify the rc file: %q → %q", tc.rcContent, got)
			}
			out := stdout.String()
			if strings.Contains(out, "shll shell-setup") || strings.Contains(out, "exec $SHELL") {
				t.Fatalf("quiet edge state must print neither nudge nor reminder, got %q", out)
			}
			if stderr.Len() != 0 {
				t.Fatalf("quiet edge state must not warn, stderr = %q", stderr.String())
			}
		})
	}
}

func TestInstall_AutoShellSetupFailureDegrades(t *testing.T) {
	// Missing rc file (shell-setup never creates one): the gate is open
	// (shellResolved && !wired — resolveWiringFact reads a missing rc as unwired),
	// the auto-run fails with the actionable exit-2 message, the warning fires, the
	// gated nudge is the fallback, and the install exit code is unchanged.
	f := allInstalledRunKitState(false)
	installFakeRunner(t, f)

	dir := t.TempDir() // no .zshrc inside — the meaningful-signal case
	env := envFunc(map[string]string{"SHELL": "/bin/zsh", "ZDOTDIR": dir, "HOME": dir})
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, false, false, false, true /*noAgentSetup*/, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil (a shell-setup failure must not fail the install)", err)
	}
	if !strings.Contains(stderr.String(), "does not exist") {
		t.Fatalf("expected the actionable missing-rc message on stderr, got %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), shellSetupAutoRunWarn) {
		t.Fatalf("expected the auto-run failure warning, stderr = %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "shll shell-setup") {
		t.Fatalf("the failed step must fall back to its nudge, got %q", stdout.String())
	}
	if _, err := os.Stat(filepath.Join(dir, ".zshrc")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("install must never create the rc file")
	}
}

func TestInstall_AutoAgentSetupPlacesSkillsAndDelegatesYes(t *testing.T) {
	// Wired rc (the shell step silent-skips) + default flags → the agent step
	// places both skill files with the canonical bytes and delegates
	// `run-kit agent setup --yes` foregrounded; the fully-wired happy path prints
	// NO Next steps block at all (empty-block suppression).
	f := allInstalledRunKitState(true /* run-kit present */)
	installFakeRunner(t, f)

	env, dir := installWiredEnvDir(t)
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, false, false, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	for _, rel := range []string{".agents/skills", ".claude/skills"} {
		path := filepath.Join(dir, rel, skillDirName, skillFileName)
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("skill not placed at %s: %v", path, err)
		}
		if string(content) != agentSkillContent {
			t.Fatalf("skill at %s does not hold the canonical bytes", path)
		}
	}
	// The delegation ran foregrounded with --yes forwarded (so run-kit's hook
	// confirmation cannot hang an unattended install).
	found := false
	for _, c := range f.recordedCalls() {
		if c.Name == runKitToolName && c.Transport == proc.TransportForeground &&
			len(c.Args) == 3 && c.Args[0] == "agent" && c.Args[1] == "setup" && c.Args[2] == "--yes" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected a foreground `run-kit agent setup --yes` delegation, calls: %+v", f.recordedCalls())
	}
	out := stdout.String()
	if strings.Contains(out, "shll agent-setup") {
		t.Fatalf("a successful auto agent-setup must not print the nudge, got %q", out)
	}
	if strings.Contains(out, nextStepsHeader) {
		t.Fatalf("the fully-wired happy path must print no Next steps header, got %q", out)
	}
}

func TestInstall_AutoAgentSetupRunKitAbsentSilentSkip(t *testing.T) {
	// run-kit absent → the delegation skips silently (inherited Constitution V
	// behavior); the placement still happens and the step counts as success — no
	// nudge, no stderr.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == runKitToolName || req.Name == "rk" {
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	env, dir := installWiredEnvDir(t)
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, false, false, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil", err)
	}
	if _, err := os.Stat(filepath.Join(dir, ".agents", "skills", skillDirName, skillFileName)); err != nil {
		t.Fatalf("skill files must still be placed when run-kit is absent: %v", err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("run-kit-absent delegation must be silent, stderr = %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "shll agent-setup") {
		t.Fatalf("run-kit-absent delegation must not fall back to the nudge, got %q", stdout.String())
	}
}

func TestInstall_AutoAgentSetupDelegationFailureContinues(t *testing.T) {
	// rk version skew (an installed run-kit older than v3.16.23 lacks the `agent`
	// family and exits non-zero): the inherited warn-and-(continuing) path fires,
	// the placement already succeeded, NO agent-setup nudge prints (re-running
	// would hit the same delegation failure, so a nudge would dead-end), and the
	// install is unaffected.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == runKitToolName && len(req.Args) >= 2 && req.Args[0] == "agent" && req.Args[1] == "setup" {
			return proc.Result{ExitCode: 1}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	env, _ := installWiredEnvDir(t)
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, false, false, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil (a delegation failure is non-fatal)", err)
	}
	if !strings.Contains(stderr.String(), "(continuing)") {
		t.Fatalf("expected the warn-and-continue delegation diagnostic, stderr = %q", stderr.String())
	}
	if strings.Contains(stdout.String(), "shll agent-setup") {
		t.Fatalf("a delegation failure must NOT trigger the agent-setup nudge, got %q", stdout.String())
	}
}

func TestInstall_AutoAgentSetupFailureDegrades(t *testing.T) {
	// Unwritable skill target: the placement fails (agent-setup's own per-path
	// diagnostic reaches stderr), the install warns and falls back to the
	// agent-setup nudge, and the exit code is unchanged.
	f := allInstalledRunKitState(true /* run-kit present */)
	installFakeRunner(t, f)

	env, dir := installWiredEnvDir(t)
	// A regular FILE at ~/.agents makes the skill-dir MkdirAll fail, so placement
	// errors on that target.
	if err := os.WriteFile(filepath.Join(dir, ".agents"), []byte("not a dir"), 0o644); err != nil {
		t.Fatalf("write .agents blocker: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := runInstall(context.Background(), env, &stdout, &stderr, false, false, false, false, nil); err != nil {
		t.Fatalf("runInstall err = %v, want nil (an agent-setup failure must not fail the install)", err)
	}
	if !strings.Contains(stderr.String(), agentSetupErrPrefix) {
		t.Fatalf("expected agent-setup's own per-path diagnostic, stderr = %q", stderr.String())
	}
	if !strings.Contains(stderr.String(), agentSetupAutoRunWarn) {
		t.Fatalf("expected the auto-run failure warning, stderr = %q", stderr.String())
	}
	if !strings.Contains(stdout.String(), "shll agent-setup") {
		t.Fatalf("the failed step must fall back to its nudge, got %q", stdout.String())
	}
}
