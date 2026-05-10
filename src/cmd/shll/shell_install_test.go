package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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

// runShellInstallCmd builds a fresh cobra command, sets buffered stdout/stderr,
// and executes with the provided argv. Returns (stdout, stderr, error).
func runShellInstallCmd(t *testing.T, argv []string) (string, string, error) {
	t.Helper()
	cmd := newShellInstallCmd()
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
	want := "# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\n"
	if got != want {
		t.Fatalf("block = %q, want %q", got, want)
	}
}

func TestBuildBlock_Bash(t *testing.T) {
	got := string(buildBlock("bash"))
	want := "# >>> shll shell-init >>>\neval \"$(shll shell-init bash)\"\n# <<< shll shell-init <<<\n"
	if got != want {
		t.Fatalf("block = %q, want %q", got, want)
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
	stdout, stderr, err := runShellInstallCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "export FOO=bar\n# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\n"
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
	original := "export FOO=bar\n# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\n"
	rc := makeRC(t, original)
	_, stderr, err := runShellInstallCmd(t, []string{"--rc-file", rc, "zsh"})
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
	_, stderr, err := runShellInstallCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "export FOO=bar\n# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\n"
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	// Guard test: open sentinel must NOT share a line with the previous content.
	if !strings.Contains(string(got), "export FOO=bar\n# >>> shll shell-init >>>") {
		t.Fatalf("open sentinel shares line with previous content: %q", got)
	}
}

func TestInstall_EmptyFileNoLeadingNewline(t *testing.T) {
	// Trailing-newline guard MUST NOT prepend \n on empty files.
	rc := makeRC(t, "")
	_, stderr, err := runShellInstallCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got, _ := os.ReadFile(rc)
	want := "# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\n"
	if string(got) != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestInstall_ErrorsWhenRcMissingNoFlag(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("ZDOTDIR", "")
	t.Setenv("SHELL", "/bin/zsh")
	_, _, err := runShellInstallCmd(t, []string{})
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
	_, _, err := runShellInstallCmd(t, []string{"--rc-file", missing, "zsh"})
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
	_, stderr, err := runShellInstallCmd(t, []string{"--rc-file", link, "zsh"})
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
	if !strings.Contains(string(got), "# >>> shll shell-init >>>") {
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
	_, stderr, err := runShellInstallCmd(t, []string{"--rc-file", rc, "zsh"})
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
	stdout, stderr, err := runShellInstallCmd(t, []string{"--print", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	want := "# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\n"
	if stdout != want {
		t.Fatalf("stdout = %q, want %q", stdout, want)
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
	stdout, _, err := runShellInstallCmd(t, []string{"--print", "--rc-file", rc, "bash"})
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
	_, _, err := runShellInstallCmd(t, []string{"--print", "--rc-file", missing, "zsh"})
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
	original := "export FOO=bar\n# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\nexport BAR=baz\n"
	rc := makeRC(t, original)
	stdout, stderr, err := runShellInstallCmd(t, []string{"--uninstall", "--rc-file", rc, "zsh"})
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
	_, stderr, err := runShellInstallCmd(t, []string{"--uninstall", "--rc-file", rc, "zsh"})
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
	_, stderr, err := runShellInstallCmd(t, []string{"--uninstall", "--rc-file", missing, "zsh"})
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
	original := "export FOO=bar\n# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\nexport BAR=baz\n"
	if err := os.WriteFile(real, []byte(original), 0o644); err != nil {
		t.Fatalf("write real: %v", err)
	}
	link := filepath.Join(dir, ".zshrc")
	if err := os.Symlink(real, link); err != nil {
		t.Fatalf("symlink: %v", err)
	}
	_, _, err := runShellInstallCmd(t, []string{"--uninstall", "--rc-file", link, "zsh"})
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
	_, _, err := runShellInstallCmd(t, []string{"--print", "--uninstall", "zsh"})
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

// --- root wiring + import discipline -----------------------------------------

func TestRoot_ShellInstallRegistered(t *testing.T) {
	root := newRootCmd()
	want := map[string]bool{"update": false, "shell-init": false, "shell-install": false, "version": false}
	for _, sub := range root.Commands() {
		// Use Name() to get just the first word of Use (e.g. "shell-install").
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

func TestNoProcImports(t *testing.T) {
	// Defensive: shell_install.go is file I/O only (Constitution I scope is
	// subprocess execution; this command does not invoke subprocesses). Guard
	// against future regressions that import internal/proc or os/exec.
	src, err := os.ReadFile("shell_install.go")
	if err != nil {
		t.Fatalf("read shell_install.go: %v", err)
	}
	if bytes.Contains(src, []byte("internal/proc")) {
		t.Errorf("shell_install.go must not import internal/proc")
	}
	if bytes.Contains(src, []byte(`"os/exec"`)) {
		t.Errorf("shell_install.go must not import os/exec")
	}
}
