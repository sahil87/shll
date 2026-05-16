package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// shellInitFakeBuilder constructs a fakeRunner that simulates per-tool installation
// state and shell-init outputs.
//
// installedFormulas selects which roster formulas are "installed" — for a tool
// whose formula is absent from this map, the fake returns proc.ErrNotFound from
// its `<tool> shell-init <shell>` invocation, mirroring real exec.LookPath
// behavior when the binary is missing from PATH. For installed tools, outputs
// supplies the canned stdout (missing entries default to empty stdout success).
func shellInitFake(installedFormulas map[string]bool, outputs map[string]string, errors map[string]error) *fakeRunner {
	// Map binary name → formula so the fake can resolve "is this tool's binary
	// on the simulated PATH?" from the Roster.
	formulaByName := map[string]string{}
	for _, t := range Roster {
		formulaByName[t.Name] = t.Formula
	}
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		// Simulate ErrNotFound for tools whose formula isn't installed.
		if formula, ok := formulaByName[req.Name]; ok && !installedFormulas[formula] {
			return proc.Result{Err: proc.ErrNotFound}
		}
		key := strings.Join(append([]string{req.Name}, req.Args...), " ")
		if e, ok := errors[key]; ok {
			return proc.Result{Err: e}
		}
		if out, ok := outputs[key]; ok {
			return proc.Result{Stdout: []byte(out)}
		}
		return proc.Result{}
	}}
}

// stdErr is a tiny helper for "any non-nil error" cases.
func stdErr(msg string) error { return errors.New(msg) }

func TestShellInit_ZshAllIntegratorsInstalled(t *testing.T) {
	f := shellInitFake(
		map[string]bool{
			formulaPrefix + "tu":  true,
			formulaPrefix + "hop": true,
			formulaPrefix + "wt":  true,
		},
		map[string]string{
			"tu shell-init zsh":  "## tu init\nexport TU=1\n",
			"hop shell-init zsh": "## hop init\nexport HOP=1\n",
			"wt shell-init zsh":  "## wt init\nexport WT=1\n",
		},
		nil,
	)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runShellInit(context.Background(), "zsh", &stdout, &stderr); err != nil {
		t.Fatalf("runShellInit err = %v", err)
	}
	want := "## tu init\nexport TU=1\n## hop init\nexport HOP=1\n## wt init\nexport WT=1\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
}

func TestShellInit_OnlyTuInstalled(t *testing.T) {
	f := shellInitFake(
		map[string]bool{formulaPrefix + "tu": true}, // hop, wt missing
		map[string]string{"tu shell-init zsh": "## tu only\n"},
		nil,
	)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runShellInit(context.Background(), "zsh", &stdout, &stderr); err != nil {
		t.Fatalf("runShellInit err = %v", err)
	}
	if stdout.String() != "## tu only\n" {
		t.Fatalf("stdout = %q, want only tu", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty for missing hop and wt, got %q", stderr.String())
	}
}

func TestShellInit_OnlyHopInstalled(t *testing.T) {
	f := shellInitFake(
		map[string]bool{formulaPrefix + "hop": true}, // tu, wt missing
		map[string]string{"hop shell-init bash": "## hop bash\n"},
		nil,
	)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runShellInit(context.Background(), "bash", &stdout, &stderr); err != nil {
		t.Fatalf("runShellInit err = %v", err)
	}
	if stdout.String() != "## hop bash\n" {
		t.Fatalf("stdout = %q, want only hop", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty for missing tu and wt, got %q", stderr.String())
	}
}

func TestShellInit_OnlyWtInstalled(t *testing.T) {
	f := shellInitFake(
		map[string]bool{formulaPrefix + "wt": true}, // tu, hop missing
		map[string]string{"wt shell-init zsh": "## wt only\n"},
		nil,
	)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runShellInit(context.Background(), "zsh", &stdout, &stderr); err != nil {
		t.Fatalf("runShellInit err = %v", err)
	}
	if stdout.String() != "## wt only\n" {
		t.Fatalf("stdout = %q, want only wt", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty for missing tu and hop, got %q", stderr.String())
	}
}

func TestShellInit_NoIntegratingToolsInstalled(t *testing.T) {
	f := shellInitFake(map[string]bool{}, nil, nil)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runShellInit(context.Background(), "zsh", &stdout, &stderr); err != nil {
		t.Fatalf("runShellInit err = %v", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout should be empty (eval-safe no-op), got %q", stdout.String())
	}
}

func TestShellInit_DeterministicOrder(t *testing.T) {
	f := shellInitFake(
		map[string]bool{
			formulaPrefix + "tu":  true,
			formulaPrefix + "hop": true,
			formulaPrefix + "wt":  true,
		},
		map[string]string{
			"tu shell-init zsh":  "TU\n",
			"hop shell-init zsh": "HOP\n",
			"wt shell-init zsh":  "WT\n",
		},
		nil,
	)
	installFakeRunner(t, f)

	var a, b bytes.Buffer
	for _, dst := range []*bytes.Buffer{&a, &b} {
		if err := runShellInit(context.Background(), "zsh", dst, &bytes.Buffer{}); err != nil {
			t.Fatalf("runShellInit err = %v", err)
		}
	}
	if a.String() != b.String() {
		t.Fatalf("non-deterministic output: %q vs %q", a.String(), b.String())
	}
	if a.String() != "TU\nHOP\nWT\n" {
		t.Fatalf("order = %q, want TU then HOP then WT (roster order)", a.String())
	}
}

func TestShellInit_UnsupportedShell(t *testing.T) {
	cmd := newShellInitCmd()
	cmd.SetArgs([]string{"fish"})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for fish shell")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must be empty (eval-safe), got %q", stdout.String())
	}
	if !strings.Contains(withCode.msg, "unsupported shell") {
		t.Fatalf("error msg = %q, want to mention unsupported shell", withCode.msg)
	}
}

func TestShellInit_MissingShellArg(t *testing.T) {
	cmd := newShellInitCmd()
	cmd.SetArgs([]string{})
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	err := cmd.Execute()
	if err == nil {
		t.Fatal("expected error for missing shell arg")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout must be empty (eval-safe), got %q", stdout.String())
	}
}

func TestShellInit_SubToolFailure(t *testing.T) {
	// All three integrators installed; hop (the middle one in roster order) fails.
	// Asserts eval-safety on both sides of the failure: tu's stdout (before hop)
	// and wt's stdout (after hop) both reach the user, while hop's bytes do not.
	f := shellInitFake(
		map[string]bool{
			formulaPrefix + "tu":  true,
			formulaPrefix + "hop": true,
			formulaPrefix + "wt":  true,
		},
		map[string]string{
			"tu shell-init zsh": "TU\n",
			"wt shell-init zsh": "WT\n",
		},
		map[string]error{"hop shell-init zsh": stdErr("boom")},
	)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runShellInit(context.Background(), "zsh", &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent", err)
	}
	// stdout must be eval-safe — tu before hop, wt after hop, no hop bytes.
	if stdout.String() != "TU\nWT\n" {
		t.Fatalf("stdout = %q, want \"TU\\nWT\\n\" (hop's failure must not pollute stdout; tu and wt still contribute)", stdout.String())
	}
	if !strings.Contains(stderr.String(), "hop") {
		t.Fatalf("stderr should mention hop failure, got %q", stderr.String())
	}
}
