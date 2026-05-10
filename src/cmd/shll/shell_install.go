package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"runtime"

	"github.com/spf13/cobra"
)

// osGoos is the operating-system identifier used for cross-platform branching
// inside resolveRcFile. Defaults to runtime.GOOS; tests override it via a
// package-level swap (Constitution: Cross-Platform Behavior — the darwin-vs-other
// branch is isolated behind this small abstraction).
var osGoos = runtime.GOOS

// Sentinel and eval-line format constants. The exact byte sequences are part of
// the user contract (idempotency check + uninstall removal target both depend on
// matching these literally), so they are extracted as named constants per
// fab/project/code-quality.md (no magic strings).
const (
	openSentinel  = "# >>> shll shell-init >>>"
	closeSentinel = "# <<< shll shell-init <<<"
	// evalLineFmt expands to `eval "$(shll shell-init <shell>)"` when fed a
	// resolved shell name. The %s is the shell.
	evalLineFmt = `eval "$(shll shell-init %s)"`
)

func newShellInstallCmd() *cobra.Command {
	var (
		printMode     bool
		uninstallMode bool
		rcFileFlag    string
	)
	cmd := &cobra.Command{
		Use:   "shell-install [shell]",
		Short: "append the shll shell-init eval line to your rc file",
		Long: `Append a sentinel-wrapped eval block that wires shll shell-init into your
shell rc file. Idempotent — re-running is a no-op when the block is already
present. Plain O_APPEND so dotfile-manager symlinks are preserved.

Modes:
  shll shell-install [shell]            install the block (default mode)
  shll shell-install --print [shell]    print the block to stdout, do not modify
  shll shell-install --uninstall [shell] remove the block from the rc file

When [shell] is omitted, shll infers it from $SHELL. Supported shells: zsh, bash.

By default, the rc file path is derived per shell:
  zsh   → ${ZDOTDIR:-$HOME}/.zshrc
  bash  → $HOME/.bash_profile (macOS) or $HOME/.bashrc (Linux)

Use --rc-file <path> to override derivation entirely.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShellInstall(cmd.Context(), args, rcFileFlag, printMode, uninstallMode, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&printMode, "print", false, "print the block to stdout, do not modify any file")
	cmd.Flags().BoolVar(&uninstallMode, "uninstall", false, "remove the shll-managed block from the rc file")
	cmd.Flags().StringVar(&rcFileFlag, "rc-file", "", "override the rc file path (escape hatch for non-standard layouts)")
	return cmd
}

// resolveShell determines the shell to install for. A positional argument wins;
// otherwise the shell is inferred from $SHELL (basename). Returns an
// errExitCode{code:2} on unsupported shells, with distinct messages for the
// two failure paths so users get actionable feedback.
//
// env is supplied for testability — production callers pass os.Getenv.
func resolveShell(args []string, env func(string) string) (string, error) {
	if len(args) >= 1 {
		shell := args[0]
		if !isSupportedShell(shell) {
			return "", &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-install: unsupported shell %q. Supported: zsh, bash", shell)}
		}
		return shell, nil
	}
	raw := env("SHELL")
	inferred := filepath.Base(raw)
	if !isSupportedShell(inferred) {
		return "", &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-install: cannot infer shell from $SHELL=%s. Pass shell explicitly: shll shell-install zsh", raw)}
	}
	return inferred, nil
}

// resolveRcFile derives the rc-file path from the resolved shell and the host
// operating system. Spec table:
//
//	zsh  | any         | ${ZDOTDIR:-$HOME}/.zshrc
//	bash | darwin      | $HOME/.bash_profile
//	bash | other       | $HOME/.bashrc
//
// The darwin-vs-other branch is the only platform-specific code path in this
// command (Constitution: Cross-Platform Behavior). osGoos is overridable by
// tests so darwin and linux defaults are both reachable from the same host.
//
// env is supplied for testability — production callers pass os.Getenv.
func resolveRcFile(shell string, env func(string) string) string {
	switch shell {
	case "zsh":
		dir := env("ZDOTDIR")
		if dir == "" {
			dir = env("HOME")
		}
		return filepath.Join(dir, ".zshrc")
	case "bash":
		home := env("HOME")
		if osGoos == "darwin" {
			return filepath.Join(home, ".bash_profile")
		}
		return filepath.Join(home, ".bashrc")
	}
	// Unreachable — resolveShell guarantees the shell is supported before this
	// function is called. Returning empty here would surface as a stat error
	// downstream rather than a misleading derived path.
	return ""
}

// buildBlock returns the exact three-line sentinel block, terminated by a
// single trailing \n. The body line is `eval "$(shll shell-init <shell>)"`.
//
// This is the single source of truth for the block contents; install, --print,
// and --uninstall search/match are all derived from the same constants.
func buildBlock(shell string) []byte {
	body := fmt.Sprintf(evalLineFmt, shell)
	return []byte(openSentinel + "\n" + body + "\n" + closeSentinel + "\n")
}

// findBlock locates the inclusive byte range of the sentinel block in content.
// Returns (start, end, true) where content[start:end] covers the open sentinel
// through the trailing \n that follows the close sentinel (if present).
//
// Used by --uninstall to slice content[:start] + content[end:] (removing the
// block plus its trailing newline). The default install path does NOT call
// this — its idempotency check is a simpler bytes.Contains scan for the open
// sentinel only.
func findBlock(content []byte) (start, end int, ok bool) {
	openBytes := []byte(openSentinel)
	closeBytes := []byte(closeSentinel)
	s := bytes.Index(content, openBytes)
	if s < 0 {
		return 0, 0, false
	}
	// Search for the close sentinel after the open sentinel.
	rel := bytes.Index(content[s+len(openBytes):], closeBytes)
	if rel < 0 {
		// Open without close — findBlock returns not-found, but note this is
		// only consulted by --uninstall. The default install path's idempotency
		// check uses bytes.Contains on the open sentinel alone, so an
		// open-without-close partial block causes install to short-circuit as
		// "already installed" (no auto-repair). Users with a corrupted partial
		// block must clean it up manually before re-installing.
		return 0, 0, false
	}
	e := s + len(openBytes) + rel + len(closeBytes)
	// Include the trailing \n that the install path produced after the close
	// sentinel (Assumption #23 — symmetric removal).
	if e < len(content) && content[e] == '\n' {
		e++
	}
	return s, e, true
}

// runShellInstall is the implementation seam invoked by the cobra factory's
// RunE. Extracted so tests can drive it directly with bytes.Buffer writers and
// controlled environment.
func runShellInstall(ctx context.Context, args []string, rcFileFlag string, printMode, uninstallMode bool, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if printMode && uninstallMode {
		return &errExitCode{code: 2, msg: "shll shell-install: --print and --uninstall are mutually exclusive"}
	}
	shell, err := resolveShell(args, os.Getenv)
	if err != nil {
		return err
	}
	rcPath := rcFileFlag
	if rcPath == "" {
		rcPath = resolveRcFile(shell, os.Getenv)
	}
	switch {
	case printMode:
		return runShellInstallPrint(shell, rcPath, stdout, stderr)
	case uninstallMode:
		return runShellInstallUninstall(shell, rcPath, stdout, stderr)
	default:
		return runShellInstallDefault(shell, rcPath, rcFileFlag != "", stdout, stderr)
	}
}

// runShellInstallDefault implements the default install path per spec:
// stat → idempotency → trailing-newline guard → O_APPEND write → success
// message. The userProvidedPath flag controls the wording of the missing-file
// error: when the user passed --rc-file, we drop the "shll won't create rc
// files" hint because they explicitly named the path.
func runShellInstallDefault(shell, rcPath string, userProvidedPath bool, stdout, stderr io.Writer) error {
	if _, err := os.Stat(rcPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if userProvidedPath {
				return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-install: %s does not exist.", rcPath)}
			}
			return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-install: %s does not exist. shll won't create rc files. Create it first, or pass --rc-file <path>.", rcPath)}
		}
		fmt.Fprintf(stderr, "shll shell-install: stat %s: %v\n", rcPath, err)
		return errSilent
	}
	content, err := os.ReadFile(rcPath)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-install: read %s: %v\n", rcPath, err)
		return errSilent
	}
	if bytes.Contains(content, []byte(openSentinel)) {
		fmt.Fprintf(stderr, "shll shell-install: already installed in %s (no changes).\n", rcPath)
		return nil
	}
	block := buildBlock(shell)
	// Trailing-newline guard: prepend \n only when the file is non-empty AND
	// its last byte is not \n. Empty files require no leading \n (Assumption
	// #22 — prevents stray blank lines).
	if len(content) > 0 && content[len(content)-1] != '\n' {
		block = append([]byte("\n"), block...)
	}
	f, err := os.OpenFile(rcPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-install: open %s: %v\n", rcPath, err)
		return errSilent
	}
	if _, werr := f.Write(block); werr != nil {
		_ = f.Close()
		fmt.Fprintf(stderr, "shll shell-install: write %s: %v\n", rcPath, werr)
		return errSilent
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "shll shell-install: close %s: %v\n", rcPath, cerr)
		return errSilent
	}
	fmt.Fprintf(stdout, "Installed shll shell integration to %s. Restart your shell or run: source %s\n", rcPath, rcPath)
	return nil
}

// runShellInstallPrint implements --print mode. Resolves shell + rc file the
// same way as default, still errors on missing rc file (the user may be
// debugging exactly that), then prints the exact block to stdout with no
// surrounding messages.
func runShellInstallPrint(shell, rcPath string, stdout, stderr io.Writer) error {
	if _, err := os.Stat(rcPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-install: %s does not exist.", rcPath)}
		}
		fmt.Fprintf(stderr, "shll shell-install: stat %s: %v\n", rcPath, err)
		return errSilent
	}
	if _, err := stdout.Write(buildBlock(shell)); err != nil {
		fmt.Fprintf(stderr, "shll shell-install: write stdout: %v\n", err)
		return errSilent
	}
	return nil
}

// runShellInstallUninstall implements --uninstall mode. Missing rc file is not
// an error here (nothing to uninstall is a benign condition). When the block
// is present, the symlink chain is resolved before the truncate-write so
// dotfile-manager symlinks stay intact while the underlying source-of-truth
// file is updated.
func runShellInstallUninstall(shell, rcPath string, stdout, stderr io.Writer) error {
	_ = shell // shell isn't used during uninstall — sentinels are shell-agnostic.
	if _, err := os.Stat(rcPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "shll shell-install: %s does not exist (nothing to uninstall).\n", rcPath)
			return nil
		}
		fmt.Fprintf(stderr, "shll shell-install: stat %s: %v\n", rcPath, err)
		return errSilent
	}
	content, err := os.ReadFile(rcPath)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-install: read %s: %v\n", rcPath, err)
		return errSilent
	}
	start, end, ok := findBlock(content)
	if !ok {
		fmt.Fprintf(stderr, "shll shell-install: not installed in %s (nothing to uninstall).\n", rcPath)
		return nil
	}
	modified := make([]byte, 0, len(content)-(end-start))
	modified = append(modified, content[:start]...)
	modified = append(modified, content[end:]...)
	resolved, err := filepath.EvalSymlinks(rcPath)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-install: resolve symlink %s: %v\n", rcPath, err)
		return errSilent
	}
	f, err := os.OpenFile(resolved, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-install: open %s: %v\n", resolved, err)
		return errSilent
	}
	if _, werr := f.Write(modified); werr != nil {
		_ = f.Close()
		fmt.Fprintf(stderr, "shll shell-install: write %s: %v\n", resolved, werr)
		return errSilent
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "shll shell-install: close %s: %v\n", resolved, cerr)
		return errSilent
	}
	fmt.Fprintf(stdout, "Removed shll shell integration from %s.\n", rcPath)
	return nil
}

