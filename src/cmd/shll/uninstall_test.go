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
// uninstall/install succeed (exit 0). The keg leaf reported for a formula is its
// last path segment (so sahil87/tap/rk → leaf `rk`, sahil87/tap/run-kit → `run-kit`).
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
	if !strings.Contains(stderr.String(), "shll, wt, idea, tu, run-kit, hop, fab-kit") {
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

// --- R10 / R16: dual-rack sweep ordering, never blind old-name ---

func TestUninstall_RunKitDualRackSweepOrder(t *testing.T) {
	// Dual-rack: run-kit installed (leaf run-kit) AND a residual rk keg (leaf rk).
	f := installedFormulasFake(map[string]bool{
		formulaPrefix + "run-kit": true,
		formulaPrefix + "rk":      true,
	})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"run-kit"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	calls := f.recordedCalls()
	// New name first (qualified), then residual by LEGACY LEAF NAME (`rk`, not the
	// qualified sahil87/tap/rk which brew would re-resolve through the rename).
	newIdx, legacyIdx := -1, -1
	for i, c := range calls {
		if c.Name == brewBinary && len(c.Args) == 2 && c.Args[0] == "uninstall" {
			switch c.Args[1] {
			case formulaPrefix + "run-kit":
				newIdx = i
			case "rk":
				legacyIdx = i
			}
		}
	}
	if newIdx < 0 {
		t.Fatalf("expected brew uninstall of the new run-kit formula, calls: %+v", calls)
	}
	if legacyIdx < 0 {
		t.Fatalf("expected brew uninstall of the residual `rk` leaf keg, calls: %+v", calls)
	}
	if newIdx > legacyIdx {
		t.Errorf("new formula must be uninstalled before the residual rk keg (newIdx=%d legacyIdx=%d)", newIdx, legacyIdx)
	}
	// Never a blind qualified old-name uninstall (`brew uninstall sahil87/tap/rk`).
	if invocationsContain(calls, brewBinary, "uninstall", formulaPrefix+"rk") {
		t.Errorf("must NEVER issue a blind `brew uninstall sahil87/tap/rk` (post-rename it would delete the good keg)")
	}
}

// --- R10: migrated machine (no residual rk) removes only the new formula ---

func TestUninstall_RunKitMigratedNoResidual(t *testing.T) {
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
	// No residual keg → no `brew uninstall rk`.
	if invocationsContain(calls, brewBinary, "uninstall", "rk") {
		t.Errorf("no residual rk keg present — must not issue `brew uninstall rk`")
	}
}

// --- R11: legacy-only machine (only rk keg) still uninstalls run-kit ---

func TestUninstall_RunKitLegacyOnlyUninstalls(t *testing.T) {
	// Only the legacy rk keg present; current run-kit formula absent.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "rk": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, []string{"run-kit"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil (a legacy keg counts as installed)", err)
	}
	calls := f.recordedCalls()
	// The current formula is absent → no `brew uninstall sahil87/tap/run-kit`.
	if invocationsContain(calls, brewBinary, "uninstall", formulaPrefix+"run-kit") {
		t.Errorf("current run-kit formula is absent — no uninstall of it expected")
	}
	// The residual rk keg is removed by its legacy leaf name.
	if !invocationsContain(calls, brewBinary, "uninstall", "rk") {
		t.Errorf("expected removal of the legacy rk keg, calls: %+v", calls)
	}
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
	if !strings.Contains(out, "shll shell-setup --uninstall") {
		t.Errorf("expected the shell-setup --uninstall hint, stdout: %q", out)
	}
	// Neither hint is executed — no `run-kit serve` and no `shll shell-setup` subprocess.
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
	if strings.Contains(stdout.String(), "shll shell-setup --uninstall") {
		t.Errorf("the rc-unwire hint must be scoped to a roster-wide sweep, not a partial subset")
	}
}

// --- R14 (A-013a): a NAMED full-roster sweep counts as roster-wide and prints the hint ---

func TestUninstall_NamedFullRosterSweepPrintsShellHint(t *testing.T) {
	// All six roster tools named explicitly (NOT the no-args case). hop is installed and
	// shell-integrated. "Roster-wide" must key on coverage of the roster set, not !subset,
	// so this named-all sweep must print the rc-unwire hint just like a no-args sweep.
	f := installedFormulasFake(map[string]bool{formulaPrefix + "hop": true})
	installFakeRunner(t, f)
	fixedClock(t)

	var stdout, stderr bytes.Buffer
	args := []string{"wt", "idea", "tu", "run-kit", "hop", "fab-kit"}
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, false, true, args); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	if !strings.Contains(stdout.String(), "shll shell-setup --uninstall") {
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
	if strings.Contains(stdout.String(), "shll shell-setup --uninstall") {
		t.Errorf("the rc-unwire hint must be success-gated — a FAILED shell-integrated removal must not print it, stdout: %q", stdout.String())
	}
}

// --- R14 (A-019): the run-kit daemon hint names the tool from the actionable entry ---

func TestUninstall_RunKitDaemonHintUsesToolName(t *testing.T) {
	// The daemon-stop hint must name the run-kit tool via a.tool.Name, not a "run-kit"
	// literal. On a migrated machine (run-kit removed successfully) the hint prints and
	// names the tool.
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

// --- R9 / R10 (finding 5): dual-rack --dry-run preview includes the residual `brew uninstall rk` ---

func TestUninstall_DryRunDualRackPreviewIncludesResidual(t *testing.T) {
	// Dual-rack: run-kit (leaf run-kit) AND a residual rk keg (leaf rk). The dry-run
	// preview must show BOTH the new-formula uninstall and the residual `brew uninstall rk`
	// the live sweep would issue — not just the primary — with no write.
	f := installedFormulasFake(map[string]bool{
		formulaPrefix + "run-kit": true,
		formulaPrefix + "rk":      true,
	})
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, true, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "brew uninstall "+formulaPrefix+"run-kit") {
		t.Errorf("dual-rack preview must include the new-formula uninstall, stdout: %q", out)
	}
	// The residual is previewed by the LEGACY LEAF NAME (`brew uninstall rk`), never the
	// qualified `sahil87/tap/rk` (which brew would re-resolve through the rename).
	newIdx := strings.Index(out, "brew uninstall "+formulaPrefix+"run-kit")
	rkIdx := strings.Index(out, "brew uninstall rk")
	if rkIdx < 0 {
		t.Errorf("dual-rack preview must include the residual `brew uninstall rk`, stdout: %q", out)
	}
	if strings.Contains(out, "brew uninstall "+formulaPrefix+"rk") {
		t.Errorf("preview must never show the qualified `brew uninstall sahil87/tap/rk`, stdout: %q", out)
	}
	// New formula previewed before the residual (matches the live sweep order).
	if newIdx >= 0 && rkIdx >= 0 && newIdx > rkIdx {
		t.Errorf("preview must list the new formula before the residual rk keg, stdout: %q", out)
	}
	// No writes — dry-run.
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary && len(c.Args) > 0 && c.Args[0] == "uninstall" {
			t.Errorf("dry-run must issue no brew uninstall, got %+v", c)
		}
	}
}

// --- R9 / R11 (finding 5): legacy-only --dry-run preview shows only `brew uninstall rk` ---

func TestUninstall_DryRunLegacyOnlyPreviewShowsResidualOnly(t *testing.T) {
	// Legacy-only machine (only the rk keg). The preview must show `brew uninstall rk`
	// and NOT a spurious `brew uninstall sahil87/tap/run-kit` (the live run issues only
	// the residual removal).
	f := installedFormulasFake(map[string]bool{formulaPrefix + "rk": true})
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runUninstall(context.Background(), strings.NewReader(""), &stdout, &stderr, true, false, []string{"run-kit"}); err != nil {
		t.Fatalf("runUninstall err = %v, want nil", err)
	}
	out := stdout.String()
	if !strings.Contains(out, "brew uninstall rk") {
		t.Errorf("legacy-only preview must show `brew uninstall rk`, stdout: %q", out)
	}
	if strings.Contains(out, "brew uninstall "+formulaPrefix+"run-kit") {
		t.Errorf("legacy-only preview must NOT show a spurious new-formula uninstall, stdout: %q", out)
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
