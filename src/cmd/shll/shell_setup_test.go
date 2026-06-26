package main

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Canonical block fragments under the NEW combined sentinel, for test
// readability. The exact bytes are load-bearing (findBlock/uninstall match them
// literally), so they live in one place here mirroring the source constants.
const (
	tNewBlockZsh    = "# >>> shll >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll <<<\n"
	tNewBlockBash   = "# >>> shll >>>\neval \"$(shll shell-init bash)\"\n# <<< shll <<<\n"
	tLegacyBlockZsh = "# >>> shll shell-init >>>\neval \"$(shll shell-init zsh)\"\n# <<< shll shell-init <<<\n"
	// tStaleExportBlockZsh is a block left behind by a former `--trust-tap`
	// install: it carries the now-unmanaged export line above the eval line. The
	// next plain `shll shell-setup` run must rewrite it to the eval-only block.
	tStaleExportBlockZsh = "# >>> shll >>>\nexport HOMEBREW_REQUIRE_TAP_TRUST=1\neval \"$(shll shell-init zsh)\"\n# <<< shll <<<\n"
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

func TestPlainInstall_NewSentinelEvalOnly(t *testing.T) {
	// shll shell-setup on a fresh file uses the new sentinel and contains ONLY the
	// eval line (no export line — trust is no longer shell-setup's concern).
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
		t.Fatalf("plain install wrote a trust export line: %q", got)
	}
}

// --- --trust-tap removal ------------------------------------------------------

func TestTrustTapFlagRemoved(t *testing.T) {
	// --trust-tap is gone: the flag is unregistered and the Long help no longer
	// documents it.
	cmd := newShellSetupCmd()
	if cmd.Flags().Lookup("trust-tap") != nil {
		t.Fatal("--trust-tap flag should be removed")
	}
	if strings.Contains(cmd.Long, "--trust-tap") {
		t.Fatal("Long help should no longer mention --trust-tap")
	}
	// Passing --trust-tap must error as an unknown flag.
	rc := makeRC(t, "export FOO=bar\n")
	_, _, err := runShellSetupCmd(t, []string{"--trust-tap", "--rc-file", rc, "zsh"})
	if err == nil {
		t.Fatal("expected an unknown-flag error for --trust-tap")
	}
}

// --- stale export-line migration (change 260626-0854) -------------------------

func TestMigration_StripsStaleExportLine(t *testing.T) {
	// A block left by a former --trust-tap install carries the now-unmanaged
	// export line. The next plain shell-setup run rewrites it to eval-only,
	// dropping the stale line; surrounding content is preserved.
	rc := makeRC(t, "export FOO=bar\n"+tStaleExportBlockZsh+"export BAR=baz\n")
	_, stderr, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v, want nil; stderr=%q", err, stderr)
	}
	got := string(mustRead(t, rc))
	want := "export FOO=bar\n" + tNewBlockZsh + "export BAR=baz\n"
	if got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(got, "HOMEBREW_REQUIRE_TAP_TRUST") {
		t.Fatalf("stale export line survived the rewrite:\n%q", got)
	}
	// Exactly one block (no duplicate eval, no second block).
	if n := strings.Count(got, "# >>> shll >>>"); n != 1 {
		t.Fatalf("found %d shll blocks, want exactly 1: %q", n, got)
	}
}

func TestMigration_StaleExportThenReRunIsNoop(t *testing.T) {
	// After the stale export is stripped to eval-only, a second run is a
	// byte-identical no-op (idempotency restored).
	rc := makeRC(t, "export FOO=bar\n"+tStaleExportBlockZsh)
	if _, _, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"}); err != nil {
		t.Fatalf("first run err = %v", err)
	}
	afterFirst := string(mustRead(t, rc))
	_, stderr, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("second run err = %v", err)
	}
	if string(mustRead(t, rc)) != afterFirst {
		t.Fatalf("second run mutated the already-cleaned block")
	}
	if !strings.Contains(stderr, "already installed in "+rc) {
		t.Fatalf("stderr = %q, want already-installed no-op on the second run", stderr)
	}
}

// --- migration ----------------------------------------------------------------

func TestMigration_LegacyEvalOnlyMigratesOnPlainInstall(t *testing.T) {
	// Plain shell-setup migrates the legacy sentinel (carrying eval forward) to
	// the new sentinel.
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

func TestMigration_LegacyWithSurroundingPreserved(t *testing.T) {
	rc := makeRC(t, "export FOO=bar\n"+tLegacyBlockZsh+"export BAR=baz\n")
	_, _, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := string(mustRead(t, rc))
	if strings.Contains(got, "# >>> shll shell-init >>>") {
		t.Fatalf("legacy sentinel still present:\n%q", got)
	}
	want := "export FOO=bar\n" + tNewBlockZsh + "export BAR=baz\n"
	if got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
	}
}

func TestMigration_BothSentinelsPresentMergeToOne(t *testing.T) {
	// Hand-edited corrupted state: a legacy block AND a new block both present →
	// merge to a single new-sentinel eval-only block (no export line is ever
	// written, since shell-setup no longer manages trust).
	original := "export A=1\n" + tLegacyBlockZsh + "export B=2\n" + tNewBlockZsh + "export C=3\n"
	rc := makeRC(t, original)
	_, _, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
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
	if strings.Contains(got, "HOMEBREW_REQUIRE_TAP_TRUST") {
		t.Fatalf("merged block must not carry an export line:\n%q", got)
	}
}

func TestMigration_BothSentinelsPresentReverseOrderMergeToOne(t *testing.T) {
	// New block appears BEFORE the legacy block — exercises the descending-splice
	// ordering in rewriteBlocks (insertAt picks the earliest start regardless).
	original := "export A=1\n" + tNewBlockZsh + "export B=2\n" + tLegacyBlockZsh + "export C=3\n"
	rc := makeRC(t, original)
	_, _, err := runShellSetupCmd(t, []string{"--rc-file", rc, "zsh"})
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
	want := "export A=1\n" + tNewBlockZsh + "export B=2\nexport C=3\n"
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

func TestPrint_NoExportLine(t *testing.T) {
	// --print never emits the trust export line — trust is not shell-setup's job.
	rc := makeRC(t, "")
	stdout, _, err := runShellSetupCmd(t, []string{"--print", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	if strings.Contains(stdout, "HOMEBREW_REQUIRE_TAP_TRUST") {
		t.Fatalf("--print emitted a trust export line: %q", stdout)
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

func TestUninstall_RemovesStaleExportBlock(t *testing.T) {
	// Uninstall removes the WHOLE block even when it still carries a stale export
	// line (both sentinels + both lines go).
	original := "export FOO=bar\n" + tStaleExportBlockZsh + "export BAR=baz\n"
	rc := makeRC(t, original)
	_, _, err := runShellSetupCmd(t, []string{"--uninstall", "--rc-file", rc, "zsh"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	got := string(mustRead(t, rc))
	want := "export FOO=bar\nexport BAR=baz\n"
	if got != want {
		t.Fatalf("file =\n%q\nwant\n%q", got, want)
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
	want := map[string]bool{"install": false, "update": false, "shell-init": false, "shell-setup": false, "version": false}
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
	// shell_setup.go is file I/O only (Constitution I scope is subprocess
	// execution; this command invokes none). With the --trust-tap ceremony seam
	// removed, the file is strictly file I/O — this guard is now stronger: there
	// is no longer even a function-value seam bridging to brew.go.
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
	// Defensive: the trust ceremony seam (ensureTrustFunc) must be gone — its
	// presence would mean subprocess work crept back toward this file.
	if bytes.Contains(src, []byte("ensureTrustFunc")) {
		t.Errorf("shell_setup.go must not reference the removed ensureTrustFunc seam")
	}
}
