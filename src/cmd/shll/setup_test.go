package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"

	"github.com/sahil87/shll/internal/proc"
)

// setupEnv points $HOME and $SHELL at a fresh t.TempDir() (and clears ZDOTDIR so
// the rc path derives to $HOME/.zshrc) so neither setup half touches the real ~.
// Returns the temp HOME. The shell half reads os.Getenv internally, so this uses
// t.Setenv rather than a map-backed env func.
func setupEnv(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	return home
}

// runSetupCmd builds a fresh `setup` cobra command, sets buffered stdout/stderr,
// and executes with the provided argv. Returns (stdout, stderr, error).
func runSetupCmd(t *testing.T, argv []string) (string, string, error) {
	t.Helper()
	cmd := newSetupCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(argv)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// --- bare `shll setup` (parent) ------------------------------------------------

func TestSetup_ParentRunsBothHalves(t *testing.T) {
	home := setupEnv(t)
	rc := filepath.Join(home, ".zshrc")
	if err := os.WriteFile(rc, []byte(""), 0o644); err != nil {
		t.Fatalf("write rc: %v", err)
	}
	installFakeRunner(t, runKitAbsentFake())

	var stdout, stderr bytes.Buffer
	if err := runSetup(context.Background(), os.Getenv, &stdout, &stderr, false); err != nil {
		t.Fatalf("runSetup err = %v, want nil; stderr=%q", err, stderr)
	}
	// Shell half ran: the sentinel block landed in the rc file.
	if got := string(mustRead(t, rc)); got != tNewBlockZsh {
		t.Fatalf("rc file = %q, want the eval-only block %q", got, tNewBlockZsh)
	}
	// Agent half ran: both skills placed (run-kit absent → delegation skipped).
	for _, p := range skillPaths(home) {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected SKILL.md at %s: %v", p, err)
		}
	}
}

func TestSetup_WorstWinsExit(t *testing.T) {
	// Shell half fails (rc file does not exist → exit-2 usage diagnostic); the
	// agent half must STILL run to completion, and the run exits 2 (worst-wins).
	home := setupEnv(t)
	installFakeRunner(t, runKitAbsentFake())

	var stdout, stderr bytes.Buffer
	err := runSetup(context.Background(), os.Getenv, &stdout, &stderr, false)
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2} (the shell half's missing-rc diagnostic)", err)
	}
	if !strings.Contains(withCode.msg, "shll won't create rc files") {
		t.Fatalf("msg = %q, want the shell half's missing-rc diagnostic", withCode.msg)
	}
	for _, p := range skillPaths(home) {
		if _, serr := os.Stat(p); serr != nil {
			t.Errorf("agent half must run even when the shell half failed — missing %s: %v", p, serr)
		}
	}
}

func TestSetup_YesForwardsToAgentHalf(t *testing.T) {
	home := setupEnv(t)
	if err := os.WriteFile(filepath.Join(home, ".zshrc"), []byte(""), 0o644); err != nil {
		t.Fatalf("write rc: %v", err)
	}
	f := &fakeRunner{respond: func(req proc.Request) proc.Result { return proc.Result{ExitCode: 0} }}
	installFakeRunner(t, f)

	if _, stderr, err := runSetupCmd(t, []string{"--yes"}); err != nil {
		t.Fatalf("setup --yes err = %v; stderr=%q", err, stderr)
	}
	if !invocationsContain(f.recordedCalls(), runKitToolName, "agent", "setup", "--"+yesFlag) {
		t.Errorf("expected a `run-kit agent setup --yes` delegation, calls: %+v", f.recordedCalls())
	}
}

func TestSetup_ParentOnlyYesFlag(t *testing.T) {
	// Minimal surface (intake assumption #10): the bare parent carries --yes/-y
	// ONLY; the halves' full flag sets live on the subcommands.
	cmd := newSetupCmd()
	fl := cmd.Flags().Lookup(yesFlag)
	if fl == nil || fl.Shorthand != yesFlagShorthand {
		t.Fatalf("setup must register --yes/-y, got %+v", fl)
	}
	for _, name := range []string{"print", "uninstall", "rc-file"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Errorf("bare setup must not carry --%s (it lives on the subcommands)", name)
		}
	}
}

// --- subcommand dispatch --------------------------------------------------------

func TestSetup_ShellSubcommandDispatch(t *testing.T) {
	setupEnv(t)
	rc := makeRC(t, "export FOO=bar\n")
	stdout, stderr, err := runSetupCmd(t, []string{"shell", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("setup shell err = %v; stderr=%q", err, stderr)
	}
	want := "export FOO=bar\n" + tNewBlockZsh
	if got := string(mustRead(t, rc)); got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	if !strings.Contains(stdout, "Installed shll shell integration to "+rc) {
		t.Fatalf("stdout = %q, want install confirmation", stdout)
	}
}

func TestSetup_AgentSubcommandDispatch(t *testing.T) {
	home := setupEnv(t)
	installFakeRunner(t, runKitAbsentFake())
	stdout, _, err := runSetupCmd(t, []string{"agent", "--print"})
	if err != nil {
		t.Fatalf("setup agent --print err = %v", err)
	}
	if !strings.HasPrefix(stdout, agentSkillContent) {
		t.Errorf("--print must print the canonical content, got:\n%s", stdout)
	}
	for _, p := range skillPaths(home) {
		if _, serr := os.Stat(p); !errors.Is(serr, os.ErrNotExist) {
			t.Errorf("--print via setup agent must write nothing, but %s was created", p)
		}
	}
}

// --- hidden deprecated old spellings (the compat contract) ----------------------

// TestCompat_OldSpellingsDispatchHiddenAndSilent drives the REAL root tree through
// Execute for both old spellings (plus the `shell-install` alias): each must
// dispatch with identical behavior, be Hidden, and print NO deprecation text
// (silent delegation — a cobra Deprecated warning would leak through the
// cross-release `shll update` refresh; the iags precedent).
func TestCompat_OldSpellingsDispatchHiddenAndSilent(t *testing.T) {
	setupEnv(t)
	installFakeRunner(t, runKitAbsentFake())

	t.Run("shell-setup", func(t *testing.T) {
		rc := makeRC(t, "")
		stdout, stderr, err := runRootCmd(t, []string{shellSetupSub, "--rc-file", rc, "zsh"})
		if err != nil {
			t.Fatalf("shell-setup err = %v; stderr=%q", err, stderr)
		}
		if got := string(mustRead(t, rc)); got != tNewBlockZsh {
			t.Fatalf("file = %q, want %q", got, tNewBlockZsh)
		}
		assertNoDeprecationText(t, stdout+stderr)
	})

	t.Run("shell-install alias", func(t *testing.T) {
		rc := makeRC(t, "")
		stdout, stderr, err := runRootCmd(t, []string{"shell-install", "--rc-file", rc, "zsh"})
		if err != nil {
			t.Fatalf("shell-install err = %v; stderr=%q", err, stderr)
		}
		if got := string(mustRead(t, rc)); got != tNewBlockZsh {
			t.Fatalf("file = %q, want %q", got, tNewBlockZsh)
		}
		assertNoDeprecationText(t, stdout+stderr)
	})

	t.Run("agent-setup", func(t *testing.T) {
		stdout, stderr, err := runRootCmd(t, []string{agentSetupSub, "--print"})
		if err != nil {
			t.Fatalf("agent-setup --print err = %v; stderr=%q", err, stderr)
		}
		if !strings.HasPrefix(stdout, agentSkillContent) {
			t.Errorf("agent-setup --print must print the canonical content, got:\n%s", stdout)
		}
		assertNoDeprecationText(t, stdout+stderr)
	})

	// Hidden markers: Find resolves each old spelling to a Hidden command.
	root := newRootCmd()
	for _, name := range []string{shellSetupSub, "shell-install", agentSetupSub} {
		cmd, _, err := root.Find([]string{name})
		if err != nil {
			t.Fatalf("Find %q: %v", name, err)
		}
		if !cmd.Hidden {
			t.Errorf("%q must be Hidden", name)
		}
	}
	// And the rename pointer is on their Short text.
	for _, name := range []string{shellSetupSub, agentSetupSub} {
		cmd, _, _ := root.Find([]string{name})
		if !strings.Contains(cmd.Short, "renamed to `shll setup") {
			t.Errorf("%q Short must note the rename, got %q", name, cmd.Short)
		}
	}
}

// runRootCmd drives the real root command through Execute with buffered writers.
func runRootCmd(t *testing.T, argv []string) (string, string, error) {
	t.Helper()
	root := newRootCmd()
	var stdout, stderr bytes.Buffer
	root.SetOut(&stdout)
	root.SetErr(&stderr)
	root.SetArgs(argv)
	err := root.Execute()
	return stdout.String(), stderr.String(), err
}

// assertNoDeprecationText fails when output carries any deprecation warning — the
// old spellings delegate SILENTLY (no cobra Deprecated field), so neither stdout
// nor stderr may mention a deprecation.
func assertNoDeprecationText(t *testing.T, out string) {
	t.Helper()
	if strings.Contains(strings.ToLower(out), "deprecat") {
		t.Fatalf("old spellings must delegate silently, got output:\n%s", out)
	}
}

// --- flag-surface parity (R5) ----------------------------------------------------

// flagSignature renders a flag's identity (name, shorthand, type, default, usage)
// so two commands' flag sets can be compared as sorted strings.
func flagSignature(f *pflag.Flag) string {
	return fmt.Sprintf("%s|%s|%s|%s|%s", f.Name, f.Shorthand, f.Value.Type(), f.DefValue, f.Usage)
}

func flagSignatures(cmd *cobra.Command) []string {
	var out []string
	cmd.Flags().VisitAll(func(f *pflag.Flag) {
		out = append(out, flagSignature(f))
	})
	return out // VisitAll walks in sorted order, so no explicit sort is needed.
}

func TestSetup_FlagSurfaceParity(t *testing.T) {
	// The new subcommands and the hidden old spellings share construction; this
	// test is the tripwire that the two faces of each pair cannot drift apart.
	pairs := []struct {
		name      string
		newCmd    *cobra.Command
		hiddenCmd *cobra.Command
	}{
		{"shell half", newSetupShellCmd(), newShellSetupCmd()},
		{"agent half", newSetupAgentCmd(), newAgentSetupCmd()},
	}
	for _, p := range pairs {
		got := strings.Join(flagSignatures(p.newCmd), "\n")
		want := strings.Join(flagSignatures(p.hiddenCmd), "\n")
		if got != want {
			t.Errorf("%s: flag sets drifted:\n--- %s ---\n%s\n--- hidden old spelling ---\n%s", p.name, p.newCmd.Name(), got, want)
		}
	}
}

// --- refreshArgv (the update self-refresh) --------------------------------------

func TestRefreshArgv_NewSpelling(t *testing.T) {
	if got, want := refreshArgv(false), []string{shllTargetToken, setupSub, setupAgentLeaf}; !slices.Equal(got, want) {
		t.Errorf("refreshArgv(false) = %v, want %v", got, want)
	}
	if got, want := refreshArgv(true), []string{shllTargetToken, setupSub, setupAgentLeaf, "--" + yesFlag}; !slices.Equal(got, want) {
		t.Errorf("refreshArgv(true) = %v, want %v", got, want)
	}
}

// --- worstError (the parent's exit-code aggregation) -----------------------------

func TestWorstError(t *testing.T) {
	if got := worstError(nil, nil); got != nil {
		t.Errorf("worstError(nil, nil) = %v, want nil", got)
	}
	usage := &errExitCode{code: usageExitCode, msg: "usage"}
	// Worst-wins: the higher code is returned regardless of order.
	if got := worstError(errSilent, usage); got != usage {
		t.Errorf("worstError(errSilent, usage) = %v, want the usage error (code 2)", got)
	}
	if got := worstError(usage, errSilent); got != usage {
		t.Errorf("worstError(usage, errSilent) = %v, want the usage error (code 2)", got)
	}
	// Tie: prefer the error whose message translateExit has not printed yet, so
	// its diagnostic is not shadowed by an already-printed errSilent.
	if got := worstError(errSilent, errSilent); !errors.Is(got, errSilent) {
		t.Errorf("worstError(errSilent, errSilent) = %v, want errSilent", got)
	}
}
