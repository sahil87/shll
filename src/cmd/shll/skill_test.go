package main

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// skillGlossaryFake builds a fake where the named formulas report installed via the
// PATH probe (`<tool> --version` succeeds); every other tool's --version returns
// ErrNotFound (not on PATH). The bare glossary uses toolInstalled (→ `--version`),
// never brew — so this fake also fails any `brew` call, pinning the no-brew contract.
func skillGlossaryFake(installed map[string]bool) *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary {
			// The glossary must never shell out to brew — surface a loud failure if it does.
			return proc.Result{Err: errors.New("brew must not be invoked by shll skill")}
		}
		if len(req.Args) == 1 && req.Args[0] == "--version" {
			if installed[req.Name] {
				return proc.Result{Stdout: []byte(req.Name + " v1.0.0\n")}
			}
			return proc.Result{Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
}

// --- Bare glossary (T012 / R1, R2) -------------------------------------------

func TestSkill_Glossary_InstalledOnlyShllFirst(t *testing.T) {
	// wt and hop installed; the other four not on PATH.
	f := skillGlossaryFake(map[string]bool{"wt": true, "hop": true})
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runSkill(context.Background(), &stdout, &stderr, nil); err != nil {
		t.Fatalf("runSkill(bare) err = %v", err)
	}
	if stderr.Len() != 0 {
		t.Errorf("glossary wrote to stderr: %q", stderr.String())
	}
	out := stdout.String()
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	// Expect: shll row, wt row, hop row, blank line, hint line = 5 lines.
	if len(lines) != 5 {
		t.Fatalf("glossary line count = %d, want 5 (shll + 2 installed + blank + hint). output:\n%s", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], shllSelf.Name) {
		t.Errorf("first row must be shll, got %q", lines[0])
	}
	if !strings.Contains(lines[0], shllSelf.Description) {
		t.Errorf("shll row must carry its description, got %q", lines[0])
	}
	// Roster order: wt precedes hop (leaves-first).
	if !strings.HasPrefix(lines[1], "wt") || !strings.HasPrefix(lines[2], "hop") {
		t.Errorf("installed rows must be wt then hop (roster order), got %q, %q", lines[1], lines[2])
	}
	// Uninstalled tools are omitted.
	for _, absent := range []string{"idea", "tu", "run-kit", "fab-kit"} {
		if strings.Contains(out, "\n"+absent+" ") || strings.HasPrefix(out, absent+" ") {
			t.Errorf("uninstalled tool %q must be omitted from the glossary, got:\n%s", absent, out)
		}
	}
	// Trailing hint teaches the second step.
	if !strings.Contains(out, skillHintLine) {
		t.Errorf("glossary must end with the hint %q, got:\n%s", skillHintLine, out)
	}
	// No bundle contents (a bundle would contain the H1 "# shll skill").
	if strings.Contains(out, "# shll skill") {
		t.Errorf("bare glossary must NOT concatenate bundles, got:\n%s", out)
	}
}

func TestSkill_Glossary_NoBrewCalls(t *testing.T) {
	f := skillGlossaryFake(map[string]bool{"wt": true})
	installFakeRunner(t, f)
	var stdout, stderr bytes.Buffer
	if err := runSkill(context.Background(), &stdout, &stderr, nil); err != nil {
		t.Fatalf("runSkill(bare) err = %v", err)
	}
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary {
			t.Errorf("glossary must make NO brew calls, but recorded %+v", c)
		}
	}
}

func TestSkill_Glossary_AlwaysShowsShll(t *testing.T) {
	// No roster tool installed → only the shll row + hint survive.
	f := skillGlossaryFake(map[string]bool{})
	installFakeRunner(t, f)
	var stdout, stderr bytes.Buffer
	if err := runSkill(context.Background(), &stdout, &stderr, nil); err != nil {
		t.Fatalf("runSkill(bare) err = %v", err)
	}
	out := stdout.String()
	if !strings.HasPrefix(out, shllSelf.Name) {
		t.Errorf("glossary must always lead with the shll row even when nothing else is installed, got:\n%s", out)
	}
	if !strings.Contains(out, skillHintLine) {
		t.Errorf("glossary must always print the hint, got:\n%s", out)
	}
}

// --- Per-tool passthrough (T012 / R3, R4, R5, R6) ----------------------------

func TestSkill_Passthrough_ByteIdentical(t *testing.T) {
	bundle := "# hop skill\n\nWhen to use hop …\n"
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "hop" && len(req.Args) == 1 && req.Args[0] == skillSubcommand {
			return proc.Result{Stdout: []byte(bundle), ExitCode: 0}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runSkill(context.Background(), &stdout, &stderr, []string{"hop"}); err != nil {
		t.Fatalf("runSkill(hop) err = %v", err)
	}
	if stdout.String() != bundle {
		t.Errorf("stdout = %q, want byte-identical %q", stdout.String(), bundle)
	}
	if stderr.Len() != 0 {
		t.Errorf("stderr must be empty on success, got %q", stderr.String())
	}
	// The invocation must be `hop skill`, via the capture-all transport.
	var found bool
	for _, c := range f.recordedCalls() {
		if c.Name == "hop" && len(c.Args) == 1 && c.Args[0] == skillSubcommand {
			found = true
			if c.Transport != proc.TransportCaptureAll {
				t.Errorf("passthrough transport = %v, want TransportCaptureAll", c.Transport)
			}
		}
	}
	if !found {
		t.Errorf("expected a `hop skill` invocation, calls: %+v", f.recordedCalls())
	}
}

func TestSkill_Passthrough_RkAliasResolvesToRunKit(t *testing.T) {
	bundle := "# run-kit skill\n"
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "run-kit" && len(req.Args) == 1 && req.Args[0] == skillSubcommand {
			return proc.Result{Stdout: []byte(bundle), ExitCode: 0}
		}
		// The alias must NOT invoke a literal `rk skill`.
		if req.Name == "rk" {
			t.Errorf("rk alias must resolve to run-kit, not invoke `rk`: %+v", req)
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runSkill(context.Background(), &stdout, &stderr, []string{"rk"}); err != nil {
		t.Fatalf("runSkill(rk) err = %v", err)
	}
	if stdout.String() != bundle {
		t.Errorf("stdout = %q, want run-kit's bundle %q", stdout.String(), bundle)
	}
}

func TestSkill_UnknownName_UsageExit2(t *testing.T) {
	f := &fakeRunner{}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runSkill(context.Background(), &stdout, &stderr, []string{"wombat"})
	var ec *errExitCode
	if !errors.As(err, &ec) {
		t.Fatalf("unknown name err = %v, want *errExitCode", err)
	}
	if ec.code != usageExitCode {
		t.Errorf("exit code = %d, want %d (usage)", ec.code, usageExitCode)
	}
	if !strings.Contains(ec.msg, "wombat") {
		t.Errorf("diagnostic must name the offending input, got %q", ec.msg)
	}
	// The diagnostic must list the valid tools so an agent can recover.
	for _, name := range []string{"shll", "wt", "hop", "fab-kit"} {
		if !strings.Contains(ec.msg, name) {
			t.Errorf("diagnostic must list valid target %q, got %q", name, ec.msg)
		}
	}
	if stdout.Len() != 0 {
		t.Errorf("unknown name must write nothing to stdout, got %q", stdout.String())
	}
	// No subprocess should have been attempted for an unknown name.
	if len(f.recordedCalls()) != 0 {
		t.Errorf("unknown name must make no subprocess call, got %+v", f.recordedCalls())
	}
}

func TestSkill_NotInstalled_OneLineNoticeExit1(t *testing.T) {
	// `idea skill` → ErrNotFound (not on PATH).
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "idea" {
			return proc.Result{ExitCode: -1, Err: proc.ErrNotFound}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runSkill(context.Background(), &stdout, &stderr, []string{"idea"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("not-installed err = %v, want errSilent (exit 1)", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("not-installed must write nothing to stdout, got %q", stdout.String())
	}
	diag := stderr.String()
	if !strings.Contains(diag, "idea") || !strings.Contains(diag, "not installed") {
		t.Errorf("stderr should be a one-line not-installed notice, got %q", diag)
	}
	if strings.Count(strings.TrimRight(diag, "\n"), "\n") != 0 {
		t.Errorf("notice must be exactly one line, got %q", diag)
	}
}

func TestSkill_Unsupported_SuppressesChildStderrExit1(t *testing.T) {
	// `wt skill` exits non-zero (version predates the subcommand) and writes its own
	// error to stderr. shll must emit ONE notice and NOT leak the child's stderr.
	childErr := `Error: unknown command "skill" for "wt"` + "\n"
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == "wt" && len(req.Args) == 1 && req.Args[0] == skillSubcommand {
			return proc.Result{Stderr: []byte(childErr), ExitCode: 1}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runSkill(context.Background(), &stdout, &stderr, []string{"wt"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("unsupported err = %v, want errSilent (exit 1)", err)
	}
	if stdout.Len() != 0 {
		t.Errorf("unsupported must write nothing to stdout, got %q", stdout.String())
	}
	diag := stderr.String()
	if !strings.Contains(diag, "wt") || !strings.Contains(diag, "does not support 'skill'") {
		t.Errorf("stderr should be the one-line unsupported notice, got %q", diag)
	}
	// The child's own error text must NOT leak.
	if strings.Contains(diag, "unknown command") {
		t.Errorf("child's raw stderr must be suppressed, got %q", diag)
	}
	if strings.Count(strings.TrimRight(diag, "\n"), "\n") != 0 {
		t.Errorf("notice must be exactly one line, got %q", diag)
	}
}

func TestSkill_ShllSelf_ByteIdenticalToEmbed(t *testing.T) {
	// `shll skill shll` serves the embedded bundle in-process — no subprocess.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		t.Errorf("shll skill shll must NOT spawn a subprocess (would recurse), got %+v", req)
		return proc.Result{}
	}}
	installFakeRunner(t, f)

	want, err := skillFS.ReadFile(skillEmbedPath)
	if err != nil {
		t.Fatalf("read embedded bundle: %v", err)
	}
	var stdout, stderr bytes.Buffer
	if err := runSkill(context.Background(), &stdout, &stderr, []string{"shll"}); err != nil {
		t.Fatalf("runSkill(shll) err = %v", err)
	}
	if !bytes.Equal(stdout.Bytes(), want) {
		t.Errorf("shll skill shll stdout is not byte-identical to the embedded bundle (got %d bytes, want %d)", stdout.Len(), len(want))
	}
	if stderr.Len() != 0 {
		t.Errorf("shll skill shll stderr must be empty, got %q", stderr.String())
	}
}

// --- Drift guard + budget (T014 / R7) ----------------------------------------

// TestSkillEmbedMatchesCanonical is the drift guard: the embedded shll bundle bytes
// MUST equal the canonical docs/site/skill.md. The test file lives at
// src/cmd/shll/, so the canonical source is three levels up. Mirrors
// TestStandardsEmbedMatchesCanonical — the same sync + drift-guard mechanism.
func TestSkillEmbedMatchesCanonical(t *testing.T) {
	embedded, err := skillFS.ReadFile(skillEmbedPath)
	if err != nil {
		t.Fatalf("read embedded bundle: %v", err)
	}
	canonicalPath := filepath.Join("..", "..", "..", "docs", "site", "skill.md")
	canonical, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatalf("read canonical %s: %v", canonicalPath, err)
	}
	if !bytes.Equal(embedded, canonical) {
		t.Errorf("embedded skill/skill.md has drifted from canonical docs/site/skill.md — run scripts/sync-standards.sh and commit the refreshed copy")
	}
}

func TestSkillBundle_WithinLineBudget(t *testing.T) {
	// The skill standard caps the bundle at ≤150 lines (principle №9).
	data, err := skillFS.ReadFile(skillEmbedPath)
	if err != nil {
		t.Fatalf("read embedded bundle: %v", err)
	}
	n := bytes.Count(data, []byte("\n"))
	if len(data) > 0 && data[len(data)-1] != '\n' {
		n++ // a final line with no trailing newline still counts
	}
	if n > 150 {
		t.Errorf("skill bundle is %d lines, over the 150-line hard budget", n)
	}
}
