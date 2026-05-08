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
// state and shell-init outputs. installedFormulas selects which roster formulas
// are present; outputs maps tool argv (joined by space) to stdout, missing entries
// produce empty stdout success.
func shellInitFake(installedFormulas map[string]bool, outputs map[string]string, errors map[string]error) *fakeRunner {
	return &fakeRunner{respond: func(req proc.Request) proc.Result {
		if req.Name == brewBinary && len(req.Args) >= 4 && req.Args[0] == "list" {
			formula := req.Args[3]
			if installedFormulas[formula] {
				return proc.Result{Stdout: []byte(formula + " 1.0.0\n")}
			}
			return proc.Result{Err: stdErr("not installed")}
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

func TestShellInit_ZshBothInstalled(t *testing.T) {
	f := shellInitFake(
		map[string]bool{formulaPrefix + "hop": true, formulaPrefix + "wt": true},
		map[string]string{
			"hop shell-init zsh": "## hop init\nexport HOP=1\n",
			"wt shell-setup":     "## wt init\nexport WT=1\n",
		},
		nil,
	)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	if err := runShellInit(context.Background(), "zsh", &stdout, &stderr); err != nil {
		t.Fatalf("runShellInit err = %v", err)
	}
	want := "## hop init\nexport HOP=1\n## wt init\nexport WT=1\n"
	if stdout.String() != want {
		t.Fatalf("stdout = %q, want %q", stdout.String(), want)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr should be empty, got %q", stderr.String())
	}
}

func TestShellInit_BashHopOnly(t *testing.T) {
	f := shellInitFake(
		map[string]bool{formulaPrefix + "hop": true}, // wt missing
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
		t.Fatalf("stderr should be empty for missing wt, got %q", stderr.String())
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
		map[string]bool{formulaPrefix + "hop": true, formulaPrefix + "wt": true},
		map[string]string{
			"hop shell-init zsh": "HOP\n",
			"wt shell-setup":     "WT\n",
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
	if a.String() != "HOP\nWT\n" {
		t.Fatalf("order = %q, want HOP then WT (roster order)", a.String())
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
	f := shellInitFake(
		map[string]bool{formulaPrefix + "hop": true, formulaPrefix + "wt": true},
		map[string]string{"wt shell-setup": "WT\n"},
		map[string]error{"hop shell-init zsh": stdErr("boom")},
	)
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	err := runShellInit(context.Background(), "zsh", &stdout, &stderr)
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent", err)
	}
	// stdout must be eval-safe — only wt's output, no hop diagnostic on stdout.
	if stdout.String() != "WT\n" {
		t.Fatalf("stdout = %q, want \"WT\\n\" (hop's failure must not pollute stdout)", stdout.String())
	}
	if !strings.Contains(stderr.String(), "hop") {
		t.Fatalf("stderr should mention hop failure, got %q", stderr.String())
	}
}
