package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/sahil87/shll/internal/proc"
)

// Canonical block fragments under the NEW combined sentinel, for test
// readability. The exact bytes are load-bearing (findBlock/uninstall match them
// literally), so they live in one place here mirroring the source constants.
const (
	tNewBlockZsh    = "# >>> shll >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll <<<\n"
	tNewBlockBash   = "# >>> shll >>>\neval \"$(shll shell-init bash)\"\n# <<< shll <<<\n"
	tCombinedZsh    = "# >>> shll >>>\nexport HOMEBREW_REQUIRE_TAP_TRUST=1\neval \"$(shll shell-init zsh)\"\n# <<< shll <<<\n"
	tLegacyBlockZsh = "# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\n"
)

// installTrustSuccessRunner installs a fake proc.Runner that simulates a modern
// brew where `brew trust` is available and the ceremony succeeds. Returns the fake
// so callers can inspect recorded calls (e.g. assert `brew trust --tap sahil87/tap`
// ran).
func installTrustSuccessRunner(t *testing.T) *fakeRunner {
	t.Helper()
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 1 && req.Args[0] == "--version":
			return proc.Result{Stdout: []byte("Homebrew 5.1.14\n")}
		case req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help":
			return proc.Result{Stdout: []byte("trust\n")}
		case req.Name == brewBinary && len(req.Args) == 3 && req.Args[0] == "trust":
			return proc.Result{ExitCode: 0}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	return f
}

// setOsGoos swaps the package-level osGoos variable for the duration of a test
// and restores it via t.Cleanup. Used by tests that exercise the darwin vs.
// linux bash defaults from a single host.
func setOsGoos(t *testing.T, value string) {
	t.Helper()
	prev := osGoos
	t.Cleanup(func() { osGoos = prev })
	osGoos = value
}

// envFunc returns an env-lookup function backed by a map. Useful for testing
// resolveShell and resolveRcFile without mutating process state.
func envFunc(env map[string]string) func(string) string {
	return func(key string) string { return env[key] }
}

// runShellSetupCmd builds a fresh cobra command, sets buffered stdout/stderr,
// and executes with the provided argv. Returns (stdout, stderr, error).
func runShellSetupCmd(t *testing.T, argv []string) (string, string, error) {
	t.Helper()
	cmd := newShellSetupCmd()
	var stdout, stderr bytes.Buffer
	cmd.SetOut(&stdout)
	cmd.SetErr(&stderr)
	cmd.SetArgs(argv)
	err := cmd.Execute()
	return stdout.String(), stderr.String(), err
}

// --- resolveShell -------------------------------------------------------------

func TestResolveShell_PositionalZsh(t *testing.T) {
	got, err := resolveShell([]string{"zsh"}, envFunc(nil))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "zsh" {
		t.Fatalf("shell = %q, want \"zsh\"", got)
	}
}

func TestResolveShell_PositionalBash(t *testing.T) {
	got, err := resolveShell([]string{"bash"}, envFunc(nil))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "bash" {
		t.Fatalf("shell = %q, want \"bash\"", got)
	}
}

func TestResolveShell_PositionalUnsupported(t *testing.T) {
	_, err := resolveShell([]string{"fish"}, envFunc(map[string]string{"SHELL": "/bin/zsh"}))
	if err == nil {
		t.Fatal("expected error for fish")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if !strings.Contains(withCode.msg, "Supported: zsh, bash") {
		t.Fatalf("msg = %q, want to mention supported list", withCode.msg)
	}
}

func TestResolveShell_InferredFromShellEnv(t *testing.T) {
	got, err := resolveShell(nil, envFunc(map[string]string{"SHELL": "/bin/zsh"}))
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	if got != "zsh" {
		t.Fatalf("shell = %q, want \"zsh\"", got)
	}
}

func TestResolveShell_InferredUnsupported(t *testing.T) {
	_, err := resolveShell(nil, envFunc(map[string]string{"SHELL": "/usr/local/bin/fish"}))
	if err == nil {
		t.Fatal("expected error for fish $SHELL")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if !strings.Contains(withCode.msg, "/usr/local/bin/fish") {
		t.Fatalf("msg = %q, want to include the inferred $SHELL value", withCode.msg)
	}
	if !strings.Contains(withCode.msg, "Pass shell explicitly") {
		t.Fatalf("msg = %q, want to suggest passing shell explicitly", withCode.msg)
	}
}

// --- resolveRcFile ------------------------------------------------------------

func TestResolveRcFile_ZshWithZdotdir(t *testing.T) {
	got := resolveRcFile("zsh", envFunc(map[string]string{
		"ZDOTDIR": "/home/u/dotfiles/zsh",
		"HOME":    "/home/u",
	}))
	want := "/home/u/dotfiles/zsh/.zshrc"
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestResolveRcFile_ZshNoZdotdir(t *testing.T) {
	got := resolveRcFile("zsh", envFunc(map[string]string{"HOME": "/home/u"}))
	want := "/home/u/.zshrc"
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestResolveRcFile_BashLinux(t *testing.T) {
	setOsGoos(t, "linux")
	got := resolveRcFile("bash", envFunc(map[string]string{"HOME": "/home/u"}))
	want := "/home/u/.bashrc"
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

func TestResolveRcFile_BashDarwin(t *testing.T) {
	setOsGoos(t, "darwin")
	got := resolveRcFile("bash", envFunc(map[string]string{"HOME": "/Users/u"}))
	want := "/Users/u/.bash_profile"
	if got != want {
		t.Fatalf("path = %q, want %q", got, want)
	}
}

// --- buildBlock ---------------------------------------------------------------

func TestBuildBlock_Zsh(t *testing.T) {
	got := string(buildBlock("zsh"))
	if got != tNewBlockZsh {
		t.Fatalf("block = %q, want %q", got, tNewBlockZsh)
	}
}

func TestBuildBlock_Bash(t *testing.T) {
	got := string(buildBlock("bash"))
	if got != tNewBlockBash {
		t.Fatalf("block = %q, want %q", got, tNewBlockBash)
	}
}

func TestBuildBlock_CombinedTrust(t *testing.T) {
	// buildBlockBody with both managed lines: export must precede eval, single
	// trailing newline, new sentinel pair (note close uses `<<<`).
	got := string(buildBlockBody(wantLines(blockMatch{}, "zsh", true)))
	if got != tCombinedZsh {
		t.Fatalf("combined block = %q, want %q", got, tCombinedZsh)
	}
}

// --- default install ----------------------------------------------------------

// makeRC writes initial content to a file inside t.TempDir() and returns its
// path. The trailing-newline behavior of content is exactly what the caller
// passes — no implicit \n appended.
func makeRC(t *testing.T, content string) string {
	t.Helper()
	dir := t.TempDir()
	path := filepath.Join(dir, ".zshrc")
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write rc: %v", err)
	}
	return path
}

func TestInstall_AppendsBlockWhenAbsent(t *testing.T) {
	rc := makeRC(t, "export FOO=bar\n")
	stdout, stderr, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "export FOO=bar\n" + tNewBlockZsh
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	if !strings.Contains(stdout, "Installed shll shell integration to "+rc) {
		t.Fatalf("stdout = %q, want install confirmation", stdout)
	}
	if !strings.Contains(stdout, "Restart your shell") || !strings.Contains(stdout, "source "+rc) {
		t.Fatalf("stdout = %q, want both restart and source hints", stdout)
	}
}

func TestInstall_Idempotent(t *testing.T) {
	original := "export FOO=bar\n" + tNewBlockZsh
	rc := makeRC(t, original)
	_, stderr, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil", err)
	}
	got, _ := os.ReadFile(rc)
	if string(got) != original {
		t.Fatalf("file mutated; got %q, want unchanged %q", got, original)
	}
	if !strings.Contains(stderr, "already installed in "+rc) {
		t.Fatalf("stderr = %q, want already-installed message", stderr)
	}
}

func TestInstall_TrailingNewlineGuard(t *testing.T) {
	rc := makeRC(t, "export FOO=bar")
	_, stderr, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "export FOO=bar\n" + tNewBlockZsh
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	// Guard test: open sentinel must NOT share a line with the previous content.
	if !strings.Contains(string(got), "export FOO=bar\n# >>> shll >>>") {
		t.Fatalf("open sentinel shares line with previous content: %q", got)
	}
}

func TestInstall_EmptyFileNoLeadingNewline(t *testing.T) {
	// Trailing-newline guard MUST NOT prepend \n on empty files.
	rc := makeRC(t, "")
	_, stderr, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	if string(got) != tNewBlockZsh {
		t.Fatalf("file =\n%q\nwant\n%q", got, tNewBlockZsh)
	}
}

func TestInstall_ErrorsWhenRcMissingNoFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	_, _, err := runShellSetupCmd(t, []string{})
	if err == nil {
		t.Fatal("expected error for missing rc file")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if !strings.Contains(withCode.msg, filepath.Join(dir, ".zshrc")) {
		t.Fatalf("msg = %q, want to mention path", withCode.msg)
	}
	if !strings.Contains(withCode.msg, "shll won't create rc files") {
		t.Fatalf("msg = %q, want create-warning hint", withCode.msg)
	}
	if !strings.Contains(withCode.msg, "--rc-file") {
		t.Fatalf("msg = %q, want --rc-file hint", withCode.msg)
	}
}

func TestInstall_ErrorsWhenRcMissingWithFlag(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing-rc")
	_, _, err := runShellSetupCmd(t, []string{"--rc-file", missing, "zsh"})
	if err == nil {
		t.Fatal("expected error")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if !strings.Contains(withCode.msg, missing+" does not exist") {
		t.Fatalf("msg = %q, want path + does-not-exist", withCode.msg)
	}
	if strings.Contains(withCode.msg, "shll won't create rc files") {
		t.Fatalf("msg = %q, must NOT include create-warning when --rc-file was passed", withCode.msg)
	}
}

func TestInstall_PreservesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "zshrc")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.WriteFile(real, []byte("export FOO=bar\n"), 0o644); err != nil {
		t.Fatalf("write real: %v", err)
	}
	link := filepath.Join(dir, ".zshrc")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, stderr, err := runShellSetupCmd(t, []string{"--rc-file", link, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	// Symlink must still be a symlink.
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced with regular file: mode=%v", info.Mode())
	}
	// Real file must contain the appended block.
	got, _ := os.ReadFile(real)
	if !strings.Contains(string(got), "# >>> shll >>>") {
		t.Fatalf("real file missing block:\n%s", got)
	}
}

func TestInstall_UnreadableRcFile(t *testing.T) {
	if os.Getuid() == 0 {
		t.Skip("running as root; permission bits do not gate read access")
	}
	rc := makeRC(t, "export FOO=bar\n")
	if err := os.Chmod(rc, 0o000); err != nil {
		t.Fatalf("chmod: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(rc, 0o644) })
	_, stderr, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if !errors.Is(err, errSilent) {
		t.Fatalf("err = %v, want errSilent", err)
	}
	if stderr == "" {
		t.Fatal("stderr empty, expected diagnostic")
	}
}

// --- --print -----------------------------------------------------------------

func TestPrint_EmitsExactBlock(t *testing.T) {
	rc := makeRC(t, "")
	stdout, stderr, err := runShellSetupCmd(t, []string{"--print", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	if stdout != tNewBlockZsh {
		t.Fatalf("stdout = %q, want %q", stdout, tNewBlockZsh)
	}
	// No file modification.
	got, _ := os.ReadFile(rc)
	if len(got) != 0 {
		t.Fatalf("file mutated under --print: %q", got)
	}
}

func TestPrint_AcceptsShellPositional(t *testing.T) {
	rc := makeRC(t, "")
	t.Setenv("SHELL", "/bin/zsh")
	stdout, _, err := runShellSetupCmd(t, []string{"--print", "--rc-file", rc, "bash"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if !strings.Contains(stdout, `eval "$(shll shell-init bash)"`) {
		t.Fatalf("stdout = %q, want bash body line", stdout)
	}
}

func TestPrint_ErrorsWhenRcMissing(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, ".zshrc")
	_, _, err := runShellSetupCmd(t, []string{"--print", "--rc-file", missing, "zsh"})
	if err == nil {
		t.Fatal("expected error")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if !strings.Contains(withCode.msg, missing) {
		t.Fatalf("msg = %q, want path", withCode.msg)
	}
}

// --- --uninstall --------------------------------------------------------------

func TestUninstall_RemovesBlock(t *testing.T) {
	original := "export FOO=bar\n" + tNewBlockZsh + "export BAR=baz\n"
	rc := makeRC(t, original)
	stdout, stderr, err := runShellSetupCmd(t, []string{"--uninstall", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "export FOO=bar\nexport BAR=baz\n"
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	if !strings.Contains(stdout, "Removed shll shell integration from "+rc) {
		t.Fatalf("stdout = %q, want removal message", stdout)
	}
}

func TestUninstall_BlockAbsent(t *testing.T) {
	original := "export FOO=bar\n"
	rc := makeRC(t, original)
	_, stderr, err := runShellSetupCmd(t, []string{"--uninstall", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	if string(got) != original {
		t.Fatalf("file mutated: %q", got)
	}
	if !strings.Contains(stderr, "not installed in "+rc) {
		t.Fatalf("stderr = %q, want not-installed message", stderr)
	}
}

func TestUninstall_RcAbsent(t *testing.T) {
	dir := t.TempDir()
	missing := filepath.Join(dir, "missing-rc")
	_, stderr, err := runShellSetupCmd(t, []string{"--uninstall", "--rc-file", missing, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	if !strings.Contains(stderr, "does not exist (nothing to uninstall)") {
		t.Fatalf("stderr = %q, want nothing-to-uninstall message", stderr)
	}
}

func TestUninstall_PreservesSymlink(t *testing.T) {
	dir := t.TempDir()
	real := filepath.Join(dir, "dotfiles", "zshrc")
	if err := os.MkdirAll(filepath.Dir(real), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	original := "export FOO=bar\n" + tNewBlockZsh + "export BAR=baz\n"
	if err := os.WriteFile(real, []byte(original), 0o644); err != nil {
		t.Fatalf("write real: %v", err)
	}
	link := filepath.Join(dir, ".zshrc")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, _, err := runShellSetupCmd(t, []string{"--uninstall", "--rc-file", link, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	info, err := os.Lstat(link)
	if err != nil {
		t.Fatalf("lstat: %v", err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("symlink replaced with regular file: mode=%v", info.Mode())
	}
	got, _ := os.ReadFile(real)
	want := "export FOO=bar\nexport BAR=baz\n"
	if string(got) != want {
		t.Fatalf("real file =\n%q\nwant\n%q", got, want)
	}
}

func TestPrintAndUninstallMutuallyExclusive(t *testing.T) {
	_, _, err := runShellSetupCmd(t, []string{"--print", "--uninstall", "zsh"})
	if err == nil {
		t.Fatal("expected error")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if !strings.Contains(withCode.msg, "mutually exclusive") {
		t.Fatalf("msg = %q, want mutually-exclusive message", withCode.msg)
	}
}

// --- --trust-tap: flag wiring -------------------------------------------------

func TestTrustTap_FlagRecognized(t *testing.T) {
	// --help must mention --trust-tap, and the flag must not error as unknown.
	installTrustSuccessRunner(t)
	rc := makeRC(t, "export FOO=bar\n")
	_, _, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("--trust-tap rejected: %v", err)
	}
	// Help text documents the flag.
	cmd := newShellSetupCmd()
	if cmd.Flags().Lookup("trust-tap") == nil {
		t.Fatal("--trust-tap flag not registered")
	}
	if !strings.Contains(cmd.Long, "--trust-tap") {
		t.Fatal("Long help does not document --trust-tap")
	}
}

func TestTrustTap_MutualExclusionUnchangedWithTrustTap(t *testing.T) {
	_, _, err := runShellSetupCmd(t, []string{"--print", "--uninstall", "--trust-tap", "zsh"})
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if !strings.Contains(withCode.msg, "mutually exclusive") {
		t.Fatalf("msg = %q, want mutual-exclusion message unchanged", withCode.msg)
	}
}

// --- --trust-tap: genuine trust (ceremony + policy) ---------------------------

func TestTrustTap_FreshCombinedBlock(t *testing.T) {
	f := installTrustSuccessRunner(t)
	rc := makeRC(t, "export FOO=bar\n")
	stdout, stderr, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "export FOO=bar\n" + tCombinedZsh
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	if !invocationsContain(f.recordedCalls(), brewBinary, "trust", "--tap", tapName) {
		t.Fatalf("ceremony not invoked; calls=%+v", f.recordedCalls())
	}
	if !strings.Contains(stdout, "Installed shll shell integration to "+rc) {
		t.Fatalf("stdout = %q, want install confirmation", stdout)
	}
}

func TestTrustTap_MergeExportIntoEvalOnlyBlock(t *testing.T) {
	// Already-set-up user (eval-only block) adds trust → export merged in place.
	f := installTrustSuccessRunner(t)
	rc := makeRC(t, "export FOO=bar\n"+tNewBlockZsh)
	_, stderr, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "export FOO=bar\n" + tCombinedZsh
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	// Exactly one block (no duplicate eval, no second block).
	if n := strings.Count(string(got), "# >>> shll >>>"); n != 1 {
		t.Fatalf("found %d shll blocks, want exactly 1: %q", n, got)
	}
	if !invocationsContain(f.recordedCalls(), brewBinary, "trust", "--tap", tapName) {
		t.Fatal("ceremony not invoked")
	}
}

func TestTrustTap_MergeEvalIntoExportOnlyBlock(t *testing.T) {
	// Trust-first user (export-only block) later runs plain shell-setup →
	// eval merged in; export preserved.
	exportOnly := "# >>> shll >>>\nexport HOMEBREW_REQUIRE_TAP_TRUST=1\n# <<< shll <<<\n"
	rc := makeRC(t, "export FOO=bar\n"+exportOnly)
	_, stderr, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "export FOO=bar\n" + tCombinedZsh
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestTrustTap_FullReRunIsByteIdenticalNoop(t *testing.T) {
	installTrustSuccessRunner(t)
	original := "export FOO=bar\n" + tCombinedZsh
	rc := makeRC(t, original)
	_, stderr, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	if string(got) != original {
		t.Fatalf("file mutated on no-op re-run:\n got %q\nwant %q", got, original)
	}
	if !strings.Contains(stderr, "already installed in "+rc) {
		t.Fatalf("stderr = %q, want already-installed no-op message", stderr)
	}
}

func TestPlainInstall_NewSentinelEvalOnly(t *testing.T) {
	// shll shell-setup (no --trust-tap) on a fresh file uses the new sentinel
	// and contains ONLY the eval line (no export line).
	rc := makeRC(t, "")
	_, _, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got, _ := os.ReadFile(rc)
	if string(got) != tNewBlockZsh {
		t.Fatalf("file = %q, want eval-only new-sentinel block %q", got, tNewBlockZsh)
	}
	if strings.Contains(string(got), "HOMEBREW_REQUIRE_TAP_TRUST") {
		t.Fatalf("plain install wrote the export line: %q", got)
	}
}

// --- migration ----------------------------------------------------------------

func TestMigration_LegacyEvalOnlyMigratesOnTrustTap(t *testing.T) {
	installTrustSuccessRunner(t)
	rc := makeRC(t, "export FOO=bar\n"+tLegacyBlockZsh+"export BAR=baz\n")
	_, stderr, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got := string(mustRead(t, rc))
	// Legacy sentinel gone, new combined block in its place, surrounding content
	// preserved.
	if strings.Contains(got, "# >>> shll shell-init >>>") {
		t.Fatalf("legacy sentinel still present:\n%q", got)
	}
	want := "export FOO=bar\n" + tCombinedZsh + "export BAR=baz\n"
	if got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestMigration_LegacyEvalOnlyMigratesOnPlainInstall(t *testing.T) {
	// Plain shell-setup also migrates the sentinel (carrying eval forward),
	// without adding the export line.
	rc := makeRC(t, tLegacyBlockZsh)
	_, _, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := string(mustRead(t, rc))
	if got != tNewBlockZsh {
		t.Fatalf("file = %q, want migrated eval-only new block %q", got, tNewBlockZsh)
	}
}

func TestMigration_BothSentinelsPresentMergeToOne(t *testing.T) {
	installTrustSuccessRunner(t)
	// Hand-edited corrupted state: a legacy block AND a new block both present.
	original := "export A=1\n" + tLegacyBlockZsh + "export B=2\n" + tNewBlockZsh + "export C=3\n"
	rc := makeRC(t, original)
	_, _, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := string(mustRead(t, rc))
	if strings.Contains(got, "# >>> shll shell-init >>>") {
		t.Fatalf("legacy sentinel survived merge:\n%q", got)
	}
	if n := strings.Count(got, "# >>> shll >>>"); n != 1 {
		t.Fatalf("found %d new-sentinel blocks, want exactly 1:\n%q", n, got)
	}
	if !strings.Contains(got, "HOMEBREW_REQUIRE_TAP_TRUST=1") {
		t.Fatalf("merged block missing export line:\n%q", got)
	}
}

func TestMigration_BothSentinelsPresentReverseOrderMergeToOne(t *testing.T) {
	installTrustSuccessRunner(t)
	// New block appears BEFORE the legacy block — exercises the descending-splice
	// ordering in rewriteBlocks (insertAt picks the earliest start regardless).
	original := "export A=1\n" + tNewBlockZsh + "export B=2\n" + tLegacyBlockZsh + "export C=3\n"
	rc := makeRC(t, original)
	_, _, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := string(mustRead(t, rc))
	if strings.Contains(got, "# >>> shll shell-init >>>") {
		t.Fatalf("legacy sentinel survived merge:\n%q", got)
	}
	if n := strings.Count(got, "# >>> shll >>>"); n != 1 {
		t.Fatalf("found %d new-sentinel blocks, want exactly 1:\n%q", n, got)
	}
	// Merged block lands at the earliest block position (where the new block was).
	want := "export A=1\n" + tCombinedZsh + "export B=2\nexport C=3\n"
	if got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestMigration_PartialUnclosedRefuses(t *testing.T) {
	// Open sentinel without a matching close → refuse, exit 2, file untouched.
	original := "export FOO=bar\n# >>> shll >>>\neval \"$(shll shell-init zsh)\"\n"
	rc := makeRC(t, original)
	_, _, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if !strings.Contains(withCode.msg, "no matching closing sentinel") {
		t.Fatalf("msg = %q, want corrupted-block diagnostic", withCode.msg)
	}
	got := string(mustRead(t, rc))
	if got != original {
		t.Fatalf("file mutated despite refusal:\n%q", got)
	}
}

func TestMigration_PartialUnclosedLegacyRefuses(t *testing.T) {
	original := "# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n"
	rc := makeRC(t, original)
	_, _, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	var withCode *errExitCode
	if !errors.As(err, &withCode) || withCode.code != 2 {
		t.Fatalf("err = %v, want errExitCode{code:2}", err)
	}
	if string(mustRead(t, rc)) != original {
		t.Fatal("file mutated despite refusal")
	}
}

// --- uninstall: legacy + new sentinels ----------------------------------------

func TestUninstall_RemovesLegacyBlock(t *testing.T) {
	original := "export FOO=bar\n" + tLegacyBlockZsh + "export BAR=baz\n"
	rc := makeRC(t, original)
	_, stderr, err := runShellSetupCmd(t, []string{"--uninstall", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got := string(mustRead(t, rc))
	want := "export FOO=bar\nexport BAR=baz\n"
	if got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestUninstall_RemovesBothSentinelBlocks(t *testing.T) {
	original := "export A=1\n" + tLegacyBlockZsh + "export B=2\n" + tNewBlockZsh + "export C=3\n"
	rc := makeRC(t, original)
	_, _, err := runShellSetupCmd(t, []string{"--uninstall", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := string(mustRead(t, rc))
	want := "export A=1\nexport B=2\nexport C=3\n"
	if got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestUninstall_DoesNotUntrust(t *testing.T) {
	f := installTrustSuccessRunner(t)
	rc := makeRC(t, "export FOO=bar\n"+tCombinedZsh)
	_, _, err := runShellSetupCmd(t, []string{"--uninstall", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	for _, c := range f.recordedCalls() {
		if c.Name == brewBinary && len(c.Args) > 0 && c.Args[0] == "untrust" {
			t.Fatalf("uninstall invoked brew untrust: %+v", c)
		}
	}
}

// --- --print --trust-tap ------------------------------------------------------

func TestPrintTrustTap_CombinedNoFileNoCeremony(t *testing.T) {
	f := installTrustSuccessRunner(t)
	rc := makeRC(t, "export FOO=bar\n")
	stdout, _, err := runShellSetupCmd(t, []string{"--print", "--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if stdout != tCombinedZsh {
		t.Fatalf("stdout = %q, want combined block %q", stdout, tCombinedZsh)
	}
	// No file modification.
	if string(mustRead(t, rc)) != "export FOO=bar\n" {
		t.Fatal("--print mutated the file")
	}
	// No ceremony under --print.
	if invocationsContain(f.recordedCalls(), brewBinary, "trust", "--tap", tapName) {
		t.Fatal("--print ran the trust ceremony")
	}
}

// --- degradation --------------------------------------------------------------

func TestTrustTap_DegradesWhenTrustUnavailable(t *testing.T) {
	// brew present but `brew trust` unrecognized: write eval, skip export, exit 0.
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 1 && req.Args[0] == "--version":
			return proc.Result{Stdout: []byte("Homebrew 3.0.0\n")}
		case req.Name == brewBinary && len(req.Args) >= 1 && req.Args[0] == "trust":
			return proc.Result{Err: errors.New("Error: Unknown command: trust")}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	rc := makeRC(t, "export FOO=bar\n")
	stdout, stderr, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil (degraded success)", err)
	}
	got := string(mustRead(t, rc))
	want := "export FOO=bar\n" + tNewBlockZsh // eval written, NO export
	if got != want {
		t.Fatalf("file =\n%q\nwant eval-only %q", got, want)
	}
	if strings.Contains(got, "HOMEBREW_REQUIRE_TAP_TRUST") {
		t.Fatalf("export line written despite unavailable trust:\n%q", got)
	}
	if !strings.Contains(stderr, "HOMEBREW_NO_REQUIRE_TAP_TRUST=1") || !strings.Contains(stderr, "HOMEBREW_NO_ENV_HINTS=1") {
		t.Fatalf("stderr = %q, want it to name the env-var escape hatches", stderr)
	}
	_ = stdout
}

func TestTrustTap_DegradesWhenBrewAbsent(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{Err: proc.ErrNotFound}
	}}
	installFakeRunner(t, f)
	rc := makeRC(t, "export FOO=bar\n")
	_, stderr, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil (degraded success)", err)
	}
	got := string(mustRead(t, rc))
	if got != "export FOO=bar\n"+tNewBlockZsh {
		t.Fatalf("file = %q, want eval-only block (export skipped, brew absent)", got)
	}
	if !strings.Contains(stderr, "Homebrew is not installed") {
		t.Fatalf("stderr = %q, want brew-absent diagnostic", stderr)
	}
}

func TestTrustTap_DegradesWhenCeremonyNonZero(t *testing.T) {
	// brew present, trust available, but the ceremony itself fails (non-zero).
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		switch {
		case req.Name == brewBinary && len(req.Args) == 1 && req.Args[0] == "--version":
			return proc.Result{Stdout: []byte("Homebrew 5.1.14\n")}
		case req.Name == brewBinary && len(req.Args) == 2 && req.Args[0] == "trust" && req.Args[1] == "--help":
			return proc.Result{Stdout: []byte("trust\n")}
		case req.Name == brewBinary && len(req.Args) == 3 && req.Args[0] == "trust":
			return proc.Result{ExitCode: 1}
		}
		return proc.Result{}
	}}
	installFakeRunner(t, f)
	rc := makeRC(t, "export FOO=bar\n")
	_, stderr, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil (degraded success)", err)
	}
	got := string(mustRead(t, rc))
	if got != "export FOO=bar\n"+tNewBlockZsh {
		t.Fatalf("file = %q, want eval-only block (export skipped, ceremony failed)", got)
	}
	if stderr == "" {
		t.Fatal("stderr empty, want a ceremony-failure diagnostic")
	}
}

// mustRead reads a file or fails the test.
func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return b
}

// --- root wiring + import discipline -----------------------------------------

func TestRoot_ShellSetupRegistered(t *testing.T) {
	root := newRootCmd()
	want := map[string]bool{"update": false, "shell-init": false, "shell-setup": false, "version": false}
	for _, sub := range root.Commands() {
		// Use Name() to get just the first word of Use (e.g. "shell-setup").
		if _, tracked := want[sub.Name()]; tracked {
			want[sub.Name()] = true
		}
	}
	for name, found := range want {
		if !found {
			t.Errorf("subcommand %q not registered on root", name)
		}
	}
}

// TestRoot_ShellInstallAliasResolves asserts the backward-compat `shell-install`
// alias dispatches to the same *cobra.Command as the canonical `shell-setup`.
// cobra's Find resolves aliases, so both lookups must return the identical
// command pointer.
func TestRoot_ShellInstallAliasResolves(t *testing.T) {
	root := newRootCmd()
	setupCmd, _, err := root.Find([]string{"shell-setup"})
	if err != nil {
		t.Fatalf("Find shell-setup: %v", err)
	}
	aliasCmd, _, err := root.Find([]string{"shell-install"})
	if err != nil {
		t.Fatalf("Find shell-install: %v", err)
	}
	if setupCmd != aliasCmd {
		t.Fatalf("alias shell-install resolves to %p, want same command as shell-setup %p", aliasCmd, setupCmd)
	}
	if setupCmd.Name() != "shell-setup" {
		t.Fatalf("resolved command Name() = %q, want \"shell-setup\"", setupCmd.Name())
	}
}

func TestNoProcImports(t *testing.T) {
	// Defensive: shell_setup.go is file I/O only (Constitution I scope is
	// subprocess execution; this command does not invoke subprocesses). Guard
	// against future regressions that import internal/proc or os/exec.
	src, err := os.ReadFile("shell_setup.go")
	if err != nil {
		t.Fatalf("read shell_setup.go: %v", err)
	}
	if bytes.Contains(src, []byte("internal/proc")) {
		t.Errorf("shell_setup.go must not import internal/proc")
	}
	if bytes.Contains(src, []byte(`"os/exec"`)) {
		t.Errorf("shell_setup.go must not import os/exec")
	}
}
