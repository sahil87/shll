package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/sahil87/shll/internal/proc"
)

// installStdinTTY swaps the package-level stdinIsTTY seam for the test duration so
// the confirmation-gate prompt path (which requires an interactive stdin) can be
// exercised with a bytes.Buffer / strings.Reader test stdin. tty controls the forced
// answer to stdinIsTTY. Mirrors installFakeRunner / installFakeClock.
func installStdinTTY(t *testing.T, tty bool) {
	t.Helper()
	prev := stdinIsTTY
	t.Cleanup(func() { stdinIsTTY = prev })
	stdinIsTTY = func(io.Reader) bool { return tty }
}

// installedFormulasFake returns a fakeRunner whose `brew list --formula --versions
// <formula>` reports installed (with a version line) exactly for the formulas in
// `installed`, and not-installed otherwise. brew --version succeeds. brew
// uninstall/install succeed (exit 0). The keg name reported for a formula is its
// last path segment (so sahil87/tap/run-kit → `run-kit`).
func installedFormulasFake(installed map[string]bool) *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name != brewBinary {
			return proc.Result{}
		}
		if len(req.Args) > 0 && req.Args[0] == "list" {
			formula := req.Args[len(req.Args)-1]
			if installed[formula] {
				leaf := formula
				if i := strings.LastIndex(formula, "/"); i >= 0 {
					leaf = formula[i+1:]
				}
				return proc.Result{Stdout: []byte(leaf + " 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		}
		return proc.Result{}
	}}
}

func fixedClock(t *testing.T) {
	t0 := time.Unix(1000, 0)
	installFakeClock(t, t0, t0.Add(72*time.Second))
}

// --- R2 / R5: no-args sweep skips missing + reverse-roster order ---

func TestUninstall_NoArgsSweepReverseOrderSkipsMissing(t *testing.T) {
	// hop and wt installed; the other four absent. shll not part of the sweep.
	f := installedFormulasFake(map[string]bool{
		formulaPrefix + "hop": true,
		formulaPrefix + "wt":  true,
	})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, nil); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}

	calls := f.recordedCalls()
	// hop and wt uninstalled; the missing four not.
	if !invocationsContain(calls, brewBinary, "uninstall", formulaPrefix+"hop") {
		t.Errorf("expected brew uninstall of hop, calls: %+v", calls)
	}
	if !invocationsContain(calls, brewBinary, "uninstall", formulaPrefix+"wt") {
		t.Errorf("expected brew uninstall of wt")
	}
	for _, name := range []string{"idea", "tu", "run-kit", "fab-kit"} {
		if invocationsContain(calls, brewBinary, "uninstall", formulaPrefix+name) {
			t.Errorf("did not expect brew uninstall of not-installed %s", name)
		}
	}
	// shll is NEVER part of a no-args sweep.
	if invocationsContain(calls, brewBinary, "uninstall", shllFormula) {
		t.Errorf("shll must NOT be uninstalled in a no-args sweep")
	}
	// Reverse-roster order: hop (a dependent) is uninstalled before wt (a leaf).
	hopIdx, wtIdx := -1, -1
	for i, c := range calls {
		if c.Name == brewBinary && len(c.Args) == 2 && c.Args[0] == "uninstall" {
			switch c.Args[1] {
			case formulaPrefix + "hop":
				hopIdx = i
			case formulaPrefix + "wt":
				wtIdx = i
			}
		}
	}
	if hopIdx < 0 || wtIdx < 0 || hopIdx > wtIdx {
		t.Errorf("expected hop uninstalled before wt (reverse-roster), hopIdx=%d wtIdx=%d", hopIdx, wtIdx)
	}
	// The absent tools are reported `not installed` (repair-path skip).
	if !strings.Contains(stdout.String(), "idea: "+notInstalledLabel) {
		t.Errorf("expected `idea: not installed` skip line, stdout: %q", stdout.String())
	}
}

// --- R4: named-but-not-installed exits 0 ---

func TestUninstall_NamedNotInstalledExitsZero(t *testing.T) {
	f := installedFormulasFake(map[string]bool{}) // nothing installed
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"hop"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil (named-but-absent is a success)", err)
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "uninstall", formulaPrefix+"hop") {
		t.Errorf("no brew uninstall expected for an absent named tool")
	}
	if !strings.Contains(stdout.String(), "hop: "+notInstalledLabel) {
		t.Errorf("expected `hop: not installed`, stdout: %q", stdout.String())
	}
}

// --- R3: unknown-target hard error, no brew side effect ---

func TestUninstall_UnknownTargetHardErrors(t *testing.T) {
	f := installedFormulasFake(map[string]bool{})
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"bogus"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUninstall err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), `unknown target "bogus"`) {
		t.Errorf("stderr = %q, want unknown-target diagnostic", stderr.String())
	}
	// Valid-target list includes shll (allowShll=true) but never rk.
	if !strings.Contains(stderr.String(), "shll, run-kit, rk-desktop, fab-kit, wt, idea, tu, hop") {
		t.Errorf("stderr = %q, want the canonical valid-target list", stderr.String())
	}
	// No brew work at all — resolution happens before hasBrew.
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary {
			t.Errorf("no brew call expected on unknown target, got %+v", c)
		}
	}
}

// --- R3: rk alias resolves to run-kit with the shared notice ---

func TestUninstall_LegacyAliasResolvesWithNotice(t *testing.T) {
	// A migrated machine: run-kit installed (leaf run-kit), no residual rk keg.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "run-kit": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"rk"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "note: rk is now run-kit") {
		t.Errorf("expected rk→run-kit alias notice, stdout: %q", stdout.String())
	}
	if !invocationsContain(f.recordedCalls(), brewBinary, "uninstall", formulaPrefix+"run-kit") {
		t.Errorf("expected brew uninstall of run-kit via the rk alias")
	}
}

// --- R6: prompt abort on `n` ---

func TestUninstall_PromptAbortNoWrite(t *testing.T) {
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	installStdinTTY(t, true)

	var stdout, stderr bytes.Buffer
	// Not --yes, not --dry-run: the gate prints the plan + prompt, reads `n\n`.
	if err := runUninstall(context.Background(), strings.NewReader("n\n"), &stdout, &stderr, false, false, []string{"hop"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil (a declined prompt aborts at exit 0)", err)
	}
	if !strings.Contains(stdout.String(), uninstallPlanHeader) {
		t.Errorf("expected the removal plan header, stdout: %q", stdout.String())
	}
	if !strings.Contains(stdout.String(), uninstallProceedPrompt) {
		t.Errorf("expected the Proceed? prompt, stdout: %q", stdout.String())
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "uninstall", formulaPrefix+"hop") {
		t.Errorf("a declined prompt must not issue any brew uninstall")
	}
	if !strings.Contains(stdout.String(), uninstallAbortedMsg) {
		t.Errorf("expected the aborted message, stdout: %q", stdout.String())
	}
}

// --- R6 (edge): a malformed/whitespace answer is treated as no ---

func TestUninstall_PromptMalformedAnswerAborts(t *testing.T) {
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	installStdinTTY(t, true)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader("maybe\n"), &stdout, &stderr, false, false, []string{"hop"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "uninstall", formulaPrefix+"hop") {
		t.Errorf("`maybe` must be treated as no — no brew uninstall expected")
	}
}

// --- R6 (edge): an affirmative `y` on a TTY proceeds ---

func TestUninstall_PromptAffirmativeProceeds(t *testing.T) {
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	installStdinTTY(t, true)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader("y\n"), &stdout, &stderr, false, false, []string{"hop"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if !invocationsContain(f.recordedCalls(), brewBinary, "uninstall", formulaPrefix+"hop") {
		t.Errorf("an affirmative answer must proceed to brew uninstall")
	}
}

// --- R7: --yes bypasses the prompt ---

func TestUninstall_YesBypassesPrompt(t *testing.T) {
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"hop"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if strings.Contains(stdout.String(), uninstallProceedPrompt) {
		t.Errorf("--yes must NOT print the prompt, stdout: %q", stdout.String())
	}
	if !invocationsContain(f.recordedCalls(), brewBinary, "uninstall", formulaPrefix+"hop") {
		t.Errorf("--yes must proceed to brew uninstall")
	}
}

// --- R8: non-TTY stdin without --yes refuses ---

func TestUninstall_NonTTYRefuses(t *testing.T) {
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	installStdinTTY(t, false) // non-interactive stdin

	var stdout, stderr bytes.Buffer
	err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, false, []string{"hop"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUninstall err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), "--yes") {
		t.Errorf("expected a --yes hint on stderr, got %q", stderr.String())
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "uninstall", formulaPrefix+"hop") {
		t.Errorf("a non-TTY refusal must not issue any brew uninstall")
	}
}

// --- R9: --dry-run preview parity, no writes, gate bypass ---

func TestUninstall_DryRunPreviewNoWritesBypassesGate(t *testing.T) {
	f := installedFormulasFake(map[string]bool{
		formulaPrefix + "hop": true,
		formulaPrefix + "wt":  true,
	})
	installFakeRunner(t, f)
	installStdinTTY(t, false) // non-TTY — but dry-run bypasses the gate entirely

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, true, false, nil); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "Would uninstall 2 tools:") {
		t.Errorf("expected dry-run preview header, stdout: %q", out)
	}
	if !strings.Contains(out, "brew uninstall "+formulaPrefix+"hop") ||
		!strings.Contains(out, "brew uninstall "+formulaPrefix+"wt") {
		t.Errorf("expected preview rows for hop and wt, stdout: %q", out)
	}
	// Reverse-roster order in the preview: hop before wt.
	if strings.Index(out, "brew uninstall "+formulaPrefix+"hop") > strings.Index(out, "brew uninstall "+formulaPrefix+"wt") {
		t.Errorf("dry-run preview should list hop (dependent) before wt (leaf), stdout: %q", out)
	}
	// No writes: brew list probes ran (reads), but no brew uninstall.
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary && len(c.Args) > 0 && c.Args[0] == "uninstall" {
			t.Errorf("dry-run must issue no brew uninstall, got %+v", c)
		}
	}
	// The gate was bypassed — no prompt, no refusal error.
	if strings.Contains(out, uninstallProceedPrompt) {
		t.Errorf("dry-run must not prompt")
	}
}

// --- run-kit after the migration-guard retirement (change h3f6) ---

func TestUninstall_RunKitPlainRemoval(t *testing.T) {
	// run-kit is a plain reverse-roster target: one `brew uninstall
	// sahil87/tap/run-kit`, and NO reference to the retired legacy formula or a
	// residual `rk` keg (orphan cleanup is manual per run-kit's README).
	f := installedFormulasFake(map[string]bool{formulaPrefix + "run-kit": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"run-kit"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	if !invocationsContain(calls, brewBinary, "uninstall", formulaPrefix+"run-kit") {
		t.Errorf("expected brew uninstall of run-kit")
	}
	if invocationsContain(calls, brewBinary, "uninstall", "rk") {
		t.Errorf("run-kit is a plain target — must not issue a residual `brew uninstall rk`")
	}
	assertNoLegacyFormulaReference(t, calls)
}

func TestUninstall_LegacyOnlyMachineReportsNotInstalled(t *testing.T) {
	// A legacy-only machine (only the invisible legacy `rk` keg; the current
	// run-kit formula absent): `shll uninstall run-kit` reports `not installed`
	// and skips it — repair-path semantics, exit 0 — and never probes or removes
	// the retired legacy formula.
	f := installedFormulasFake(map[string]bool{}) // run-kit formula not installed
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"run-kit"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil (named-but-absent is a success)", err)
	}
	if !strings.Contains(stdout.String(), "run-kit: "+notInstalledLabel) {
		t.Errorf("expected `run-kit: not installed` skip line, stdout: %q", stdout.String())
	}
	calls := f.recordedCalls()
	for _, c := range calls {
		if c.Name == brewBinary && len(c.Args) > 0 && c.Args[0] == "uninstall" {
			t.Errorf("nothing to remove on a legacy-only machine, got %+v", c)
		}
	}
	assertNoLegacyFormulaReference(t, calls)
}

// --- R12: shll-self uninstall brew-managed + farewell ---

func TestUninstall_ShllSelfBrewManaged(t *testing.T) {
	f := installedFormulasFake(map[string]bool{shllFormula: true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"shll"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if !invocationsContain(f.recordedCalls(), brewBinary, "uninstall", shllFormula) {
		t.Errorf("expected brew uninstall of the shll formula")
	}
	if !strings.Contains(stdout.String(), "brew install "+shllFormula) {
		t.Errorf("expected a farewell note naming the reinstall command, stdout: %q", stdout.String())
	}
}

// --- R12: shll-self processed LAST when named with roster tools ---

func TestUninstall_ShllSelfProcessedLast(t *testing.T) {
	f := installedFormulasFake(map[string]bool{
		shllFormula:           true,
		formulaPrefix + "hop": true,
		formulaPrefix + "wt":  true,
	})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"shll", "hop", "wt"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	shllIdx, hopIdx, wtIdx := -1, -1, -1
	for i, c := range calls {
		if c.Name == brewBinary && len(c.Args) == 2 && c.Args[0] == "uninstall" {
			switch c.Args[1] {
			case shllFormula:
				shllIdx = i
			case formulaPrefix + "hop":
				hopIdx = i
			case formulaPrefix + "wt":
				wtIdx = i
			}
		}
	}
	if shllIdx < 0 || hopIdx < 0 || wtIdx < 0 {
		t.Fatalf("expected all three uninstalled, shll=%d hop=%d wt=%d", shllIdx, hopIdx, wtIdx)
	}
	if shllIdx < hopIdx || shllIdx < wtIdx {
		t.Errorf("shll must be uninstalled LAST (shll=%d hop=%d wt=%d)", shllIdx, hopIdx, wtIdx)
	}
	// Reverse-roster among roster tools: hop before wt.
	if hopIdx > wtIdx {
		t.Errorf("hop (dependent) must precede wt (leaf), hop=%d wt=%d", hopIdx, wtIdx)
	}
}

// --- R12: shll-self NOT brew-managed → hard error, no uninstall ---

func TestUninstall_ShllSelfNotBrewManaged(t *testing.T) {
	f := installedFormulasFake(map[string]bool{}) // shll formula not installed (dev build)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"shll"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUninstall err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), "not brew-managed") {
		t.Errorf("expected a `not brew-managed` error, stderr: %q", stderr.String())
	}
	if invocationsContain(f.recordedCalls(), brewBinary, "uninstall", shllFormula) {
		t.Errorf("a non-brew-managed shll must NOT be brew-uninstalled")
	}
}

// --- R13: failure aggregation → exit 1; skips are not failures ---

func TestUninstall_FailureAggregationExitOne(t *testing.T) {
	// hop and wt installed; the `brew uninstall` of hop's formula exits non-zero.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name != brewBinary {
			return proc.Result{}
		}
		switch {
		case len(req.Args) > 0 && req.Args[0] == "list":
			formula := req.Args[len(req.Args)-1]
			if formula == formulaPrefix+"hop" || formula == formulaPrefix+"wt" {
				leaf := formula[strings.LastIndex(formula, "/")+1:]
				return proc.Result{Stdout: []byte(leaf + " 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		case len(req.Args) == 2 && req.Args[0] == "uninstall" && req.Args[1] == formulaPrefix+"hop":
			return proc.Result{ExitCode: 1} // hop uninstall fails
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUninstall err = %v, want errSilent (a per-tool failure → exit 1)", err)
	}
	// The loop continued: wt was still attempted after hop failed.
	if !invocationsContain(f.recordedCalls(), brewBinary, "uninstall", formulaPrefix+"wt") {
		t.Errorf("the loop must continue past a failure and attempt wt")
	}
	// A `not installed` skip (the other four tools) is NOT a failure — the exit code
	// is driven by the hop uninstall failure only, and the summary tail reflects it.
	if !strings.Contains(stdout.String(), "failed") {
		t.Errorf("expected a partial-failure summary tail, stdout: %q", stdout.String())
	}
}

// --- R14: post-run hints are print-only ---

func TestUninstall_PostRunHintsPrintOnly(t *testing.T) {
	// A no-args sweep with hop (shell-integrated) and run-kit installed.
	f := installedFormulasFake(map[string]bool{
		formulaPrefix + "hop":     true,
		formulaPrefix + "run-kit": true,
	})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, nil); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	out := stdout.String()
	// run-kit daemon stop hint (print-only).
	if !strings.Contains(out, "run-kit serve --stop") {
		t.Errorf("expected the run-kit daemon-stop hint, stdout: %q", out)
	}
	// rc-file unwire hint (roster-wide shell-integrated removal).
	if !strings.Contains(out, "shll setup shell --uninstall") {
		t.Errorf("expected the shell-setup --uninstall hint, stdout: %q", out)
	}
	// Neither hint is executed — no `run-kit serve` and no `shll setup shell` subprocess.
	for _, c := range f.recordedCalls() {
		if c.Name == "run-kit" || c.Name == "shll" {
			t.Errorf("post-run hints must be print-only, got a subprocess %+v", c)
		}
	}
}

// --- R14: the rc-file hint is NOT printed for a partial subset ---

func TestUninstall_ShellHintScopedToRosterWide(t *testing.T) {
	// A subset run that removes only hop (a shell-integrated tool) — other integrated
	// tools (tu, wt) may still be present and wired, so the rc-unwire hint would mislead.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"hop"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if strings.Contains(stdout.String(), "shll setup shell --uninstall") {
		t.Errorf("the rc-unwire hint must be scoped to a roster-wide sweep, not a partial subset")
	}
}

// --- R14 (A-013a): a NAMED full-roster sweep counts as roster-wide and prints the hint ---

func TestUninstall_NamedFullRosterSweepPrintsShellHint(t *testing.T) {
	// All seven roster tools named explicitly (NOT the no-args case). hop is installed and
	// shell-integrated. "Roster-wide" must key on coverage of the roster set, not !subset,
	// so this named-all sweep must print the rc-unwire hint just like a no-args sweep.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	args := []string{"run-kit", "rk-desktop", "fab-kit", "wt", "idea", "tu", "hop"}
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, args); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "shll setup shell --uninstall") {
		t.Errorf("a named full-roster sweep must print the rc-unwire hint (roster-wide keys on coverage, not !subset), stdout: %q", stdout.String())
	}
}

// --- R14 (A-013b): the rc-unwire hint is success-gated — a FAILED shell-integrated removal suppresses it ---

func TestUninstall_ShellHintSuppressedOnFailedRemoval(t *testing.T) {
	// A no-args (roster-wide) sweep where the ONLY installed shell-integrated tool (hop)
	// FAILS to uninstall. The rc-unwire hint must NOT fire — it is success-gated on a tool
	// actually removed (mirroring the run-kit daemon hint), not merely attempted.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name != brewBinary {
			return proc.Result{}
		}
		switch {
		case len(req.Args) > 0 && req.Args[0] == "list":
			formula := req.Args[len(req.Args)-1]
			if formula == formulaPrefix+"hop" {
				return proc.Result{Stdout: []byte("hop 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		case len(req.Args) == 2 && req.Args[0] == "uninstall" && req.Args[1] == formulaPrefix+"hop":
			return proc.Result{ExitCode: 1} // hop (the shell-integrated tool) fails to uninstall
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUninstall err = %v, want errSilent (the hop uninstall failed)", err)
	}
	if strings.Contains(stdout.String(), "shll setup shell --uninstall") {
		t.Errorf("the rc-unwire hint must be success-gated — a FAILED shell-integrated removal must not print it, stdout: %q", stdout.String())
	}
}

// --- R14 (A-019): the run-kit daemon hint fires on the roster entry's removal ---

func TestUninstall_RunKitDaemonHintUsesToolName(t *testing.T) {
	// The daemon-stop hint is keyed on the run-kit roster entry by name (the
	// runKitToolName constant) and fires when run-kit is removed successfully.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "run-kit": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"run-kit"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "run-kit serve --stop") {
		t.Errorf("expected the run-kit daemon-stop hint naming the tool, stdout: %q", stdout.String())
	}
}

// --- R14 (A-013b): the run-kit daemon hint is suppressed when the removal FAILED ---

func TestUninstall_RunKitDaemonHintSuppressedOnFailure(t *testing.T) {
	// run-kit installed but its `brew uninstall` fails → the daemon hint (success-gated)
	// must NOT print.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name != brewBinary {
			return proc.Result{}
		}
		switch {
		case len(req.Args) > 0 && req.Args[0] == "list":
			formula := req.Args[len(req.Args)-1]
			if formula == formulaPrefix+"run-kit" {
				return proc.Result{Stdout: []byte("run-kit 1.0.0\n")}
			}
			return proc.Result{Err: errors.New("not installed")}
		case len(req.Args) == 2 && req.Args[0] == "uninstall" && req.Args[1] == formulaPrefix+"run-kit":
			return proc.Result{ExitCode: 1} // run-kit fails to uninstall
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"run-kit"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUninstall err = %v, want errSilent (run-kit uninstall failed)", err)
	}
	if strings.Contains(stdout.String(), "run-kit serve --stop") {
		t.Errorf("the daemon hint must be success-gated — a FAILED run-kit removal must not print it, stdout: %q", stdout.String())
	}
}

// --- run-kit --dry-run preview is a single plain row (change h3f6) ---

func TestUninstall_DryRunRunKitPreviewSingleRow(t *testing.T) {
	// The run-kit preview is one plain `brew uninstall sahil87/tap/run-kit` row —
	// no residual `brew uninstall rk` row exists anymore — with no write.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "run-kit": true})
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, true, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "brew uninstall "+formulaPrefix+"run-kit") {
		t.Errorf("preview must include the run-kit formula uninstall, stdout: %q", out)
	}
	if strings.Contains(out, "brew uninstall rk") {
		t.Errorf("preview must not show a residual `brew uninstall rk` row, stdout: %q", out)
	}
	// No writes — dry-run.
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary && len(c.Args) > 0 && c.Args[0] == "uninstall" {
			t.Errorf("dry-run must issue no brew uninstall, got %+v", c)
		}
	}
}

// --- R1: brew missing → uninstall-specific hint, exit 1 ---

func TestUninstall_BrewMissing(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary {
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, nil)
	if !errors.Is(err, errSilent) {
		t.Fatalf("runUninstall err = %v, want errSilent", err)
	}
	if !strings.Contains(stderr.String(), uninstallBrewMissingHint) {
		t.Errorf("stderr = %q, want the uninstall-specific brew-missing hint", stderr.String())
	}
	// The hint must say "shll uninstall", not "shll update"/"shll install".
	if strings.Contains(stderr.String(), "shll update requires") || strings.Contains(stderr.String(), "shll install requires") {
		t.Errorf("stderr = %q, must not use another command's brew-missing hint", stderr.String())
	}
}

// --- rk-desktop delegated exclusion (change t26g) ----------------------------

func TestUninstall_RkDesktopTargetedSkipsWithNote(t *testing.T) {
	// `shll uninstall rk-desktop`: a valid named target, but non-brew — skipped
	// with a note pointing at its own manager, exit 0 (repair-path semantics:
	// absence is the goal state), NO `brew uninstall` of anything.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"rk-desktop"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil (a non-brew target is a skip, not an error)", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "rk-desktop") || !strings.Contains(out, delegatedUninstallNote) {
		t.Fatalf("stdout = %q, want the rk-desktop skip-with-note", out)
	}
	if !strings.Contains(out, uninstallNothingMsg) {
		t.Fatalf("stdout = %q, want the nothing-to-do message (nothing actionable)", out)
	}
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary && len(c.Args) > 0 && c.Args[0] == "uninstall" {
			t.Fatalf("no brew uninstall must run for rk-desktop, got %+v", c)
		}
	}
}

func TestUninstall_WholeRosterSweepSkipsRkDesktop(t *testing.T) {
	// The no-args sweep never brew-touches rk-desktop: it prints the skip note
	// alongside the not-installed lines and still removes the installed brew
	// tools.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, nil); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "rk-desktop: "+delegatedUninstallNote) {
		t.Fatalf("stdout = %q, want the rk-desktop skip note in the sweep", out)
	}
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary && len(c.Args) >= 2 && c.Args[0] == "uninstall" {
			if c.Args[1] == "" || !strings.HasPrefix(c.Args[1], formulaPrefix) {
				t.Fatalf("brew uninstall with a non-formula target: %+v", c)
			}
		}
	}
}
