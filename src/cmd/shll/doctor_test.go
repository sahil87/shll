package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// doctorVersionState is how a single tool's `--version` probe should behave in a
// test, mapped to a proc.Result by doctorFake.
type doctorVersionState int

// dvOK is the zero value so tools absent from a state map default to "installed
// and runnable" — a test only names the tools it wants to misbehave.
const (
	dvOK           doctorVersionState = iota // on PATH, reports a version
	dvMissing                                // proc.ErrNotFound (binary absent)
	dvUnreportable                           // on PATH but --version errors
	dvEmpty                                  // on PATH, --version prints nothing (normalize → "")
)

// doctorFake builds a fakeRunner whose `<tool> --version` response is driven by a
// per-tool state map. Tools absent from the map default to dvOK with a generic
// version, so a test only needs to name the tools it wants to misbehave.
func doctorFake(states map[string]doctorVersionState) *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if len(req.Args) == 1 && req.Args[0] == "--version" {
			switch states[req.Name] {
			case dvMissing:
				return proc.Result{Err: proc.ErrNotFound}
			case dvUnreportable:
				return proc.Result{Err: errors.New("boom")}
			case dvEmpty:
				return proc.Result{Stdout: []byte("\n  \n")}
			default: // dvOK
				return proc.Result{Stdout: []byte(req.Name + " v1.2.3\n")}
			}
		}
		return proc.Result{}
	}}
}

// rcEnv returns an env func that resolves zsh and points the rc path at the given
// rc file (via ZDOTDIR), so the wiring check reads a t.TempDir() file and NEVER
// touches the real ~/.zshrc.
func rcEnv(rcDir string) func(string) string {
	return envFunc(map[string]string{
		"SHELL":   "/bin/zsh",
		"ZDOTDIR": rcDir,
		"HOME":    rcDir,
	})
}

// writeWiredRC creates a .zshrc inside a fresh temp dir containing shll's eval
// block and returns the dir (suitable for ZDOTDIR).
func writeWiredRC(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte("export FOO=bar\n"+tNewBlockZsh), 0o644); err != nil {
		t.Fatalf("write rc: %v", err)
	}
	return dir
}

// writeUnwiredRC creates a .zshrc with no shll block and returns its dir.
func writeUnwiredRC(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte("export FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write rc: %v", err)
	}
	return dir
}

// writeCorruptRC creates a .zshrc with an shll open sentinel but NO matching
// close sentinel (locateBlock reports partial) and returns its dir.
func writeCorruptRC(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	corrupt := "export FOO=bar\n" + openSentinel + "\neval \"$(shll shell-init zsh)\"\n" // open, no close
	if err := os.WriteFile(filepath.Join(dir, ".zshrc"), []byte(corrupt), 0o644); err != nil {
		t.Fatalf("write rc: %v", err)
	}
	return dir
}

// resultByTool indexes a JSON-decoded result slice by tool name.
func resultByTool(results []doctorResult) map[string]doctorResult {
	m := make(map[string]doctorResult, len(results))
	for _, r := range results {
		m[r.Tool] = r
	}
	return m
}

// --- text output, marker derivation ------------------------------------------

func TestDoctor_AllOKWired(t *testing.T) {
	installFakeRunner(t, doctorFake(nil)) // all dvOK
	dir := writeWiredRC(t)

	var stdout, stderr bytes.Buffer
	err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr)
	if err != nil {
		t.Fatalf("runDoctor err = %v, want nil (all OK)", err)
	}
	out := stdout.String()
	// One line per tool, every line OK, no problem tail.
	for _, tool := range Roster {
		if !lineFor(out, tool.Name) {
			t.Errorf("missing line for %q in:\n%s", tool.Name, out)
		}
	}
	if strings.Contains(out, markerFail) || strings.Contains(out, markerWarn) {
		t.Errorf("all-OK output contains WARN/FAIL:\n%s", out)
	}
	if strings.Contains(out, "have problems") {
		t.Errorf("all-OK output should have no problem tail:\n%s", out)
	}
	// Shell-init tools show "wired"; others do not.
	for _, line := range strings.Split(out, "\n") {
		if strings.HasPrefix(line, "wt ") || strings.HasPrefix(line, "tu ") || strings.HasPrefix(line, "hop ") {
			if !strings.Contains(line, "wired") {
				t.Errorf("shell-init tool line missing 'wired': %q", line)
			}
		}
	}
}

func TestDoctor_MissingBinaryFails(t *testing.T) {
	installFakeRunner(t, doctorFake(map[string]doctorVersionState{"hop": dvMissing}))
	dir := writeWiredRC(t)

	var stdout, stderr bytes.Buffer
	err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent (a tool is FAIL)", err)
	}
	out := stdout.String()
	if !lineHas(out, "hop", markerFail) {
		t.Errorf("hop line not FAIL:\n%s", out)
	}
	if !strings.Contains(out, "brew install "+formulaPrefix+"hop") {
		t.Errorf("missing install suggestion for hop:\n%s", out)
	}
	if !strings.Contains(out, "have problems") {
		t.Errorf("expected problem-count tail:\n%s", out)
	}
}

// TestDoctor_ProblemTailDenominatorExcludesShll guards that the summary-tail
// denominator counts only the checkable roster tools (len(Roster)), NOT the
// prepended always-OK shll row. With exactly one roster failure (hop) the tail
// must read "1 of 6 tools have problems", never "1 of 7" — the shll row can
// never register a problem, so including it in the denominator would misreport.
func TestDoctor_ProblemTailDenominatorExcludesShll(t *testing.T) {
	installFakeRunner(t, doctorFake(map[string]doctorVersionState{"hop": dvMissing}))
	dir := writeWiredRC(t)

	var stdout, stderr bytes.Buffer
	_ = runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr)
	out := stdout.String()

	want := fmt.Sprintf("1 of %d tools have problems", len(Roster))
	if !strings.Contains(out, want) {
		t.Errorf("problem tail denominator wrong: want %q in output:\n%s", want, out)
	}
	// Explicitly reject the off-by-one that includes the always-OK shll row.
	if strings.Contains(out, fmt.Sprintf("1 of %d tools have problems", len(Roster)+1)) {
		t.Errorf("problem tail counted the always-OK shll row in the denominator:\n%s", out)
	}
}

func TestDoctor_UnreportableVersionFails(t *testing.T) {
	for _, state := range []doctorVersionState{dvUnreportable, dvEmpty} {
		installFakeRunner(t, doctorFake(map[string]doctorVersionState{"fab-kit": state}))
		dir := writeWiredRC(t)
		var stdout, stderr bytes.Buffer
		err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr)
		if !errors.Is(err, errSilent) {
			t.Fatalf("state %d: err = %v, want errSilent", state, err)
		}
		out := stdout.String()
		if !lineHas(out, "fab-kit", markerFail) {
			t.Errorf("state %d: fab-kit not FAIL:\n%s", state, out)
		}
		if !strings.Contains(out, "fab-kit --version' failed") {
			t.Errorf("state %d: missing reinstall suggestion:\n%s", state, out)
		}
		if !strings.Contains(out, "brew reinstall "+formulaPrefix+"fab-kit") {
			t.Errorf("state %d: missing reinstall formula:\n%s", state, out)
		}
	}
}

func TestDoctor_UnwiredShellInitWarnsExitZero(t *testing.T) {
	installFakeRunner(t, doctorFake(nil)) // all installed + runnable
	dir := writeUnwiredRC(t)

	var stdout, stderr bytes.Buffer
	err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr)
	if err != nil {
		t.Fatalf("err = %v, want nil (WARN must not fail exit)", err)
	}
	out := stdout.String()
	// wt/tu/hop ship shell-init → WARN when unwired; idea/rk/fab-kit → OK.
	for _, name := range []string{"wt", "tu", "hop"} {
		if !lineHas(out, name, markerWarn) {
			t.Errorf("%s not WARN when unwired:\n%s", name, out)
		}
	}
	for _, name := range []string{"idea", "rk", "fab-kit"} {
		if lineHas(out, name, markerWarn) || lineHas(out, name, markerFail) {
			t.Errorf("%s should be OK (no wiring check):\n%s", name, out)
		}
	}
	if !strings.Contains(out, suggestNotWired) {
		t.Errorf("missing not-wired suggestion:\n%s", out)
	}
}

func TestDoctor_CorruptBlockWarnsWithDistinctSuggestion(t *testing.T) {
	// An rc file with an unclosed shll sentinel: shell-setup would refuse to
	// modify it, so doctor must surface the corrupt-block suggestion (manual
	// cleanup) — NOT the plain "run shll shell-setup" not-wired hint — and stay
	// exit 0 (WARN, not FAIL).
	installFakeRunner(t, doctorFake(nil)) // all installed + runnable
	dir := writeCorruptRC(t)

	var stdout, stderr bytes.Buffer
	err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr)
	if err != nil {
		t.Fatalf("err = %v, want nil (corrupt block is WARN, not FAIL)", err)
	}
	out := stdout.String()
	for _, name := range []string{"wt", "tu", "hop"} {
		if !lineHas(out, name, markerWarn) {
			t.Errorf("%s not WARN on corrupt block:\n%s", name, out)
		}
	}
	if !strings.Contains(out, suggestCorruptBlock) {
		t.Errorf("missing corrupt-block suggestion:\n%s", out)
	}
	if strings.Contains(out, suggestNotWired) {
		t.Errorf("corrupt block should NOT emit the plain not-wired suggestion:\n%s", out)
	}
}

func TestDoctor_MissingDominatesWiring(t *testing.T) {
	// wt is a shell-init tool that is ALSO missing on PATH; the binary FAIL must
	// dominate the (would-be) wiring WARN.
	installFakeRunner(t, doctorFake(map[string]doctorVersionState{"wt": dvMissing}))
	dir := writeUnwiredRC(t)
	var stdout, stderr bytes.Buffer
	err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent", err)
	}
	out := stdout.String()
	if !lineHas(out, "wt", markerFail) {
		t.Errorf("wt should be FAIL (binary missing dominates wiring):\n%s", out)
	}
}

// --- unresolvable $SHELL degradation ------------------------------------------

func TestDoctor_UnresolvableShellDegradesToWarn(t *testing.T) {
	installFakeRunner(t, doctorFake(nil)) // all installed + runnable
	// $SHELL is unsupported → wiring cannot resolve. Binary checks still run.
	env := envFunc(map[string]string{"SHELL": "/bin/sh"})

	var stdout, stderr bytes.Buffer
	err := runDoctor(context.Background(), false, env, &stdout, &stderr)
	if err != nil {
		t.Fatalf("err = %v, want nil (degradation is WARN, not FAIL)", err)
	}
	out := stdout.String()
	for _, name := range []string{"wt", "tu", "hop"} {
		if !lineHas(out, name, markerWarn) {
			t.Errorf("%s should WARN on unresolvable $SHELL:\n%s", name, out)
		}
	}
	if !strings.Contains(out, "$SHELL is") {
		t.Errorf("missing unresolvable-$SHELL suggestion:\n%s", out)
	}
	// Non-shell-init tools are unaffected (still OK).
	if lineHas(out, "idea", markerWarn) {
		t.Errorf("idea must not WARN on unresolvable $SHELL:\n%s", out)
	}
}

// --- JSON output --------------------------------------------------------------

func TestDoctor_JSONShapeAndExit(t *testing.T) {
	installFakeRunner(t, doctorFake(map[string]doctorVersionState{
		"hop": dvMissing,
		"tu":  dvOK, // tu OK but unwired below → WARN
	}))
	dir := writeUnwiredRC(t)

	var stdout, stderr bytes.Buffer
	err := runDoctor(context.Background(), true, rcEnv(dir), &stdout, &stderr)
	// hop is FAIL → exit 1, same as text.
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent (FAIL present)", err)
	}
	raw := stdout.Bytes()
	// No ANSI in JSON mode.
	if bytes.Contains(raw, []byte("\x1b[")) {
		t.Errorf("JSON output contains ANSI escape:\n%s", raw)
	}
	// Trailing newline.
	if len(raw) == 0 || raw[len(raw)-1] != '\n' {
		t.Errorf("JSON output missing trailing newline")
	}
	var results []doctorResult
	if err := json.Unmarshal(raw, &results); err != nil {
		t.Fatalf("JSON unmarshal err = %v\noutput:\n%s", err, raw)
	}
	// One shll-first object plus one per roster tool.
	if len(results) != len(Roster)+1 {
		t.Fatalf("JSON has %d objects, want %d (shll + one per roster tool)", len(results), len(Roster)+1)
	}
	// shll-first object, then roster order preserved (offset by 1).
	if results[0].Tool != shllSelf.Name {
		t.Errorf("JSON[0].tool = %q, want %q (shll-first)", results[0].Tool, shllSelf.Name)
	}
	for i, tool := range Roster {
		if results[i+1].Tool != tool.Name {
			t.Errorf("JSON[%d].tool = %q, want %q (roster order)", i+1, results[i+1].Tool, tool.Name)
		}
	}
	by := resultByTool(results)

	hop := by["hop"]
	if hop.Status != markerFail || hop.OnPath || hop.VersionOK || hop.Version != "" {
		t.Errorf("hop json = %+v, want FAIL/missing", hop)
	}
	if !hop.ShellInit {
		t.Errorf("hop.shell_init = false, want true (hop ships shell-init)")
	}
	if hop.Suggestion == "" {
		t.Errorf("hop.suggestion empty, want install hint")
	}

	tu := by["tu"]
	if tu.Status != markerWarn || !tu.OnPath || !tu.VersionOK || tu.Wired {
		t.Errorf("tu json = %+v, want WARN/onpath/version_ok/unwired", tu)
	}
	if tu.Version != "v1.2.3" {
		t.Errorf("tu.version = %q, want v1.2.3", tu.Version)
	}

	// idea ships no shell-init → shell_init:false, OK, no wiring concern.
	idea := by["idea"]
	if idea.ShellInit {
		t.Errorf("idea.shell_init = true, want false (idea ships no shell-init)")
	}
	if idea.Status != markerOK {
		t.Errorf("idea.status = %q, want OK", idea.Status)
	}
	if idea.Wired {
		t.Errorf("idea.wired = true, want false")
	}
}

func TestDoctor_JSONAllOKExitZero(t *testing.T) {
	installFakeRunner(t, doctorFake(nil))
	dir := writeWiredRC(t)
	var stdout, stderr bytes.Buffer
	err := runDoctor(context.Background(), true, rcEnv(dir), &stdout, &stderr)
	if err != nil {
		t.Fatalf("err = %v, want nil (all OK)", err)
	}
	var results []doctorResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	for _, r := range results {
		if r.Status != markerOK {
			t.Errorf("%s json status = %q, want OK", r.Tool, r.Status)
		}
	}
	// Wired shell-init tools report wired:true.
	by := resultByTool(results)
	for _, name := range []string{"wt", "tu", "hop"} {
		if !by[name].Wired {
			t.Errorf("%s.wired = false, want true (block present)", name)
		}
	}
}

// --- shll-first self row (change bb7r) ----------------------------------------

func TestDoctor_ShllFirstRowText(t *testing.T) {
	installFakeRunner(t, doctorFake(nil)) // all roster tools OK
	dir := writeWiredRC(t)

	var stdout, stderr bytes.Buffer
	if err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr); err != nil {
		t.Fatalf("runDoctor err = %v, want nil (all OK incl. shll)", err)
	}
	out := stdout.String()
	// shll is the FIRST line and is OK.
	firstLine := strings.SplitN(out, "\n", 2)[0]
	if !strings.HasPrefix(firstLine, shllSelf.Name+" ") {
		t.Errorf("first line = %q, want to start with %q (shll-first)", firstLine, shllSelf.Name)
	}
	if !lineHas(out, shllSelf.Name, markerOK) {
		t.Errorf("shll row not OK:\n%s", out)
	}
	// shll ships no shell-init → its row must NOT say "wired".
	if strings.Contains(firstLine, "wired") {
		t.Errorf("shll row = %q, must not show wiring detail (shll ships no shell-init)", firstLine)
	}
}

func TestDoctor_ShllFirstObjectJSON(t *testing.T) {
	installFakeRunner(t, doctorFake(nil))
	dir := writeWiredRC(t)

	var stdout, stderr bytes.Buffer
	if err := runDoctor(context.Background(), true, rcEnv(dir), &stdout, &stderr); err != nil {
		t.Fatalf("runDoctor(json) err = %v, want nil", err)
	}
	var results []doctorResult
	if err := json.Unmarshal(stdout.Bytes(), &results); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(results) == 0 || results[0].Tool != shllSelf.Name {
		t.Fatalf("results[0] = %+v, want shll-first object", results)
	}
	shll := results[0]
	if shll.Status != markerOK || !shll.OnPath || !shll.VersionOK {
		t.Errorf("shll json = %+v, want OK/onpath/version_ok", shll)
	}
	if shll.ShellInit {
		t.Errorf("shll.shell_init = true, want false (shll ships no shell-init)")
	}
	if shll.Wired {
		t.Errorf("shll.wired = true, want false (no wiring check applies to shll)")
	}
	if strings.TrimSpace(shll.Version) == "" {
		t.Errorf("shll.version = %q, want a non-empty version from the package var", shll.Version)
	}
}

func TestDoctor_ShllRowNeverPerturbsExit(t *testing.T) {
	// The always-OK shll row must NOT mask a roster FAIL: a missing roster tool
	// still drives exit 1, and an all-clean roster still exits 0 — the shll row
	// is transparent to the any-FAIL→exit-1 contract.
	t.Run("roster FAIL still exits non-zero", func(t *testing.T) {
		installFakeRunner(t, doctorFake(map[string]doctorVersionState{"hop": dvMissing}))
		dir := writeWiredRC(t)
		var stdout, stderr bytes.Buffer
		err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr)
		if !errors.Is(err, errSilent) {
			t.Fatalf("err = %v, want errSilent (a roster tool FAILs despite the OK shll row)", err)
		}
		if !lineHas(stdout.String(), shllSelf.Name, markerOK) {
			t.Errorf("shll row should still be OK even when the run fails:\n%s", stdout.String())
		}
	})
	t.Run("clean roster still exits 0", func(t *testing.T) {
		installFakeRunner(t, doctorFake(nil))
		dir := writeWiredRC(t)
		var stdout, stderr bytes.Buffer
		if err := runDoctor(context.Background(), false, rcEnv(dir), &stdout, &stderr); err != nil {
			t.Fatalf("err = %v, want nil", err)
		}
	})
}

// --- registration -------------------------------------------------------------

func TestDoctor_RegisteredOnRoot(t *testing.T) {
	root := newRootCmd()
	found := false
	for _, sub := range root.Commands() {
		if sub.Name() == "doctor" {
			found = true
		}
	}
	if !found {
		t.Error("doctor not registered on root")
	}
	if !strings.Contains(rootLong, "shll doctor") {
		t.Error("rootLong does not document shll doctor")
	}
}

// --- helpers ------------------------------------------------------------------

// lineFor reports whether output has a line starting with the tool name.
func lineFor(output, tool string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, tool+" ") {
			return true
		}
	}
	return false
}

// lineHas reports whether the line for tool contains the given marker.
func lineHas(output, tool, marker string) bool {
	for _, line := range strings.Split(output, "\n") {
		if strings.HasPrefix(line, tool+" ") && strings.Contains(line, marker) {
			return true
		}
	}
	return false
}
