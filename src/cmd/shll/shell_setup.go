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
	"strings"

	"github.com/spf13/cobra"
)

// osGoos is the operating-system identifier used for cross-platform branching
// inside resolveRcFile. Defaults to runtime.GOOS; tests override it via a
// package-level swap (Constitution: Cross-Platform Behavior — the darwin-vs-other
// branch is isolated behind this small abstraction).
var osGoos = runtime.GOOS

// Sentinel and managed-line constants. The exact byte sequences are part of the
// user contract (block location + uninstall removal target both depend on
// matching these literally), so they are extracted as named constants per
// fab/project/code-quality.md (no magic strings).
//
// The block uses the `# >>> shll >>>` / `# <<< shll <<<` sentinel pair (note the
// close sentinel uses `<<<`). It holds the single managed line — the eval line.
// The legacy `# >>> shll shell-init >>>` pair is recognized only for migration
// and uninstall of pre-existing blocks.
const (
	openSentinel  = "# >>> shll >>>"
	closeSentinel = "# <<< shll <<<"

	// legacyOpenSentinel / legacyCloseSentinel are the pre-rename sentinels. The
	// install path migrates a legacy block in place (carrying its eval line
	// forward); uninstall removes a legacy block so users who never re-installed
	// can still uninstall.
	legacyOpenSentinel  = "# >>> shll shell-init >>>"
	legacyCloseSentinel = "# <<< shll shell-init <<<"

	// evalLineFmt expands to `eval "$(shll shell-init <shell>)"` when fed a
	// resolved shell name. The %s is the shell.
	evalLineFmt = `eval "$(shll shell-init %s)"`
)

func newShellSetupCmd() *cobra.Command {
	var (
		printMode     bool
		uninstallMode bool
		rcFileFlag    string
	)
	cmd := &cobra.Command{
		Use:     "shell-setup [shell]",
		Aliases: []string{"shell-install"},
		Short:   "append the shll shell-init eval line to your rc file",
		Long: `Append a sentinel-wrapped eval block that wires shll shell-init into your
shell rc file. Idempotent — re-running is a no-op when the block is already
present. Plain O_APPEND so dotfile-manager symlinks are preserved.

Also available under the legacy alias ` + "`shll shell-install`" + ` (unchanged behavior).

Modes:
  shll shell-setup [shell]            install the block (default mode)
  shll shell-setup --print [shell]    print the block to stdout, do not modify
  shll shell-setup --uninstall [shell] remove the block from the rc file

shell-setup is pure rc-wiring — it maintains only the
` + "`eval \"$(shll shell-init <shell>)\"`" + ` line and touches no Homebrew state.
(Tap trust is established by ` + "`shll install`" + `, which trusts each formula it
installs; see ` + "`shll install --help`" + `.)

When [shell] is omitted, shll infers it from $SHELL. Supported shells: zsh, bash.

By default, the rc file path is derived per shell:
  zsh   → ${ZDOTDIR:-$HOME}/.zshrc
  bash  → $HOME/.bash_profile (macOS) or $HOME/.bashrc (Linux)

Use --rc-file <path> to override derivation entirely.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShellSetup(cmd.Context(), args, rcFileFlag, printMode, uninstallMode, cmd.OutOrStdout(), cmd.ErrOrStderr())
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
			return "", &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-setup: unsupported shell %q. Supported: zsh, bash", shell)}
		}
		return shell, nil
	}
	raw := env("SHELL")
	inferred := filepath.Base(raw)
	if !isSupportedShell(inferred) {
		return "", &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-setup: cannot infer shell from $SHELL=%s. Pass shell explicitly: shll shell-setup zsh", raw)}
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

// evalLine returns the eval body line for the resolved shell:
// `eval "$(shll shell-init <shell>)"`.
func evalLine(shell string) string {
	return fmt.Sprintf(evalLineFmt, shell)
}

// buildBlockBody wraps an ordered set of managed lines in the new sentinel pair,
// terminated by a single trailing \n. Callers pass the lines in canonical order;
// buildBlockBody does not reorder or dedup — the merge logic upstream is
// responsible for that.
//
// This is the single source of truth for the block contents; install, --print,
// and the migration rewrite all derive from the same constants.
func buildBlockBody(lines []string) []byte {
	out := openSentinel + "\n"
	for _, ln := range lines {
		out += ln + "\n"
	}
	out += closeSentinel + "\n"
	return []byte(out)
}

// wantLines computes the canonical set of managed lines a block should contain
// after this invocation. shell-setup is pure rc-wiring, so the only managed line
// is the eval line — it is always desired (so a block that somehow lacked it
// gains it, and a stale block that carried only the now-unmanaged export line is
// rewritten to the eval line).
//
// existing is accepted for signature symmetry with the merge call sites but is
// unused: the eval line is unconditional and there are no other managed lines to
// carry forward. A pre-existing block's no-longer-managed lines (e.g. a stale
// `export HOMEBREW_REQUIRE_TAP_TRUST=1` from a former --trust-tap install) are
// NOT recognized by findBlockWith and so are dropped when the block is rewritten
// — the active stale-line cleanup.
func wantLines(_ blockMatch, shell string) []string {
	return []string{evalLine(shell)}
}

// buildBlock returns the eval-only block under the new sentinel for the given
// shell. It routes through buildBlockBody so every path shares the same sentinel
// constants. Used by --print and as the fresh-install body.
func buildBlock(shell string) []byte {
	return buildBlockBody([]string{evalLine(shell)})
}

// blockMatch describes a located shll-managed block: its inclusive byte range
// (content[start:end] covers the open sentinel through the trailing \n after the
// close sentinel) and the managed lines extracted from its body.
type blockMatch struct {
	start, end int
	// hasEval reports whether the existing block carries the eval line.
	hasEval bool
}

// findBlockWith locates the inclusive byte range of a sentinel block delimited by
// the given open/close sentinels, and extracts whether it contains the eval line.
//
// Returns ok=false when the open sentinel is absent. Returns partial=true when
// the open sentinel is present but the matching close sentinel is not — an
// unclosed/corrupted block that the caller MUST refuse to auto-repair (guessing
// its bounds risks corrupting the rc file).
//
// Only the eval line is recognized as a managed line. Any other body line (e.g. a
// stale `export HOMEBREW_REQUIRE_TAP_TRUST=1` from a former --trust-tap install)
// is ignored here, so a rewrite that reconstructs the block from wantLines drops
// it — the stale-line cleanup.
func findBlockWith(content []byte, open, close string) (m blockMatch, ok, partial bool) {
	openBytes := []byte(open)
	closeBytes := []byte(close)
	s := bytes.Index(content, openBytes)
	if s < 0 {
		return blockMatch{}, false, false
	}
	rel := bytes.Index(content[s+len(openBytes):], closeBytes)
	if rel < 0 {
		// Open without close — corrupted/partial. Signal partial so the caller
		// can refuse with a diagnostic rather than guess the bounds.
		return blockMatch{}, false, true
	}
	bodyStart := s + len(openBytes)
	bodyEnd := s + len(openBytes) + rel
	e := bodyEnd + len(closeBytes)
	if e < len(content) && content[e] == '\n' {
		e++
	}
	m = blockMatch{start: s, end: e}
	for _, ln := range bytes.Split(content[bodyStart:bodyEnd], []byte("\n")) {
		trimmed := string(bytes.TrimSpace(ln))
		if strings.HasPrefix(trimmed, evalLinePrefix) {
			m.hasEval = true
		}
	}
	return m, true, false
}

// evalLinePrefix is the shell-agnostic prefix shared by every eval body line
// (`eval "$(shll shell-init zsh)"`, `... bash)"`). Used to recognize an existing
// eval line during a merge regardless of which shell it was installed for.
const evalLinePrefix = `eval "$(shll shell-init`

// runShellSetup is the implementation seam invoked by the cobra factory's
// RunE. Extracted so tests can drive it directly with bytes.Buffer writers and
// controlled environment. Pure file I/O — this file imports no subprocess-
// execution package (Constitution I scope is subprocess execution; shell-setup
// invokes none). The TestNoProcImports guard pins that invariant by scanning the
// source bytes.
func runShellSetup(ctx context.Context, args []string, rcFileFlag string, printMode, uninstallMode bool, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	_ = ctx // retained for signature stability; shell-setup performs no ctx-scoped work.
	if printMode && uninstallMode {
		return &errExitCode{code: 2, msg: "shll shell-setup: --print and --uninstall are mutually exclusive"}
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
		return runShellSetupPrint(shell, rcPath, stdout, stderr)
	case uninstallMode:
		return runShellSetupUninstall(shell, rcPath, stdout, stderr)
	default:
		return runShellSetupDefault(shell, rcPath, rcFileFlag != "", stdout, stderr)
	}
}

// locateBlock finds the shll-managed block in content, recognizing BOTH the new
// `# >>> shll >>>` sentinel and the legacy `# >>> shll shell-init >>>` sentinel.
// It is the single block-location entry point for install/migration.
//
// Returns:
//   - newM/newOK: the new-sentinel block (if present).
//   - legacyM/legacyOK: the legacy-sentinel block (if present).
//   - partial: true when EITHER sentinel is open without its matching close
//     (corrupted) — the caller MUST refuse to modify the file (exit 2) rather
//     than guess the block bounds.
func locateBlock(content []byte) (newM blockMatch, newOK bool, legacyM blockMatch, legacyOK bool, partial bool) {
	var np, lp bool
	newM, newOK, np = findBlockWith(content, openSentinel, closeSentinel)
	legacyM, legacyOK, lp = findBlockWith(content, legacyOpenSentinel, legacyCloseSentinel)
	return newM, newOK, legacyM, legacyOK, np || lp
}

// runShellSetupDefault implements the default install path as a rewrite into the
// single shll-managed block (new `# >>> shll >>>` sentinel). The flow:
//
//	stat (no O_CREATE) → read → locate block (new + legacy) → refuse on partial →
//	compute desired lines (eval-only) → no-op if the block is already
//	byte-identical → otherwise rewrite in place (in-block / migration /
//	both-sentinels-present) or append a fresh block.
//
// Two write strategies preserve the symlink invariant: a fresh append uses plain
// O_APPEND (follows the symlink, atomic for a sub-PIPE_BUF block); an in-place
// rewrite (existing block present, or migration) is read-modify-write and so
// resolves the symlink chain before an O_TRUNC write to the real file.
//
// A pre-existing block carrying a stale `export HOMEBREW_REQUIRE_TAP_TRUST=1`
// line (a former --trust-tap install) is rewritten to the eval-only block on this
// path: findBlockWith doesn't recognize the export line, and rewriteBlocks splices
// out the whole old block range and inserts the freshly-built eval-only block, so
// the stale line is dropped (active cleanup).
//
// userProvidedPath controls the missing-file error wording: with --rc-file the
// user named the path explicitly, so the "shll won't create rc files" hint is
// dropped.
func runShellSetupDefault(shell, rcPath string, userProvidedPath bool, stdout, stderr io.Writer) error {
	if _, err := os.Stat(rcPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			if userProvidedPath {
				return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-setup: %s does not exist.", rcPath)}
			}
			return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-setup: %s does not exist. shll won't create rc files. Create it first, or pass --rc-file <path>.", rcPath)}
		}
		fmt.Fprintf(stderr, "shll shell-setup: stat %s: %v\n", rcPath, err)
		return errSilent
	}
	content, err := os.ReadFile(rcPath)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-setup: read %s: %v\n", rcPath, err)
		return errSilent
	}

	newM, newOK, legacyM, legacyOK, partial := locateBlock(content)
	if partial {
		// Open-without-close sentinel — corrupted/partial. Guessing the bounds
		// risks corrupting the user's rc file, so refuse and direct manual cleanup
		// (deliberate divergence from the legacy short-circuit-as-"already-installed").
		return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-setup: %s has an shll block with an opening sentinel but no matching closing sentinel. Refusing to modify a corrupted block — fix or remove it manually, then re-run.", rcPath)}
	}

	desired := buildBlockBody(wantLines(blockMatch{}, shell))

	switch {
	case !newOK && !legacyOK:
		// No existing block — append a fresh one (plain O_APPEND, symlink-safe).
		return appendBlock(rcPath, content, desired, stdout, stderr)
	default:
		// One or both blocks exist — rewrite in place. Splice out every existing
		// shll block and insert the eval-only block at the earliest block position.
		// Any stale lines (e.g. a former export line) in the removed range are
		// dropped — the rebuilt block contains only the eval line.
		return rewriteBlocks(rcPath, content, desired, newM, newOK, legacyM, legacyOK, stdout, stderr)
	}
}

// appendBlock writes a fresh block to the end of the rc file using plain O_APPEND
// (symlink-preserving). The trailing-newline guard prepends \n only when the file
// is non-empty AND does not already end in \n (empty files get no leading blank
// line).
func appendBlock(rcPath string, content, block []byte, stdout, stderr io.Writer) error {
	if len(content) > 0 && content[len(content)-1] != '\n' {
		block = append([]byte("\n"), block...)
	}
	f, err := os.OpenFile(rcPath, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-setup: open %s: %v\n", rcPath, err)
		return errSilent
	}
	if _, werr := f.Write(block); werr != nil {
		_ = f.Close()
		fmt.Fprintf(stderr, "shll shell-setup: write %s: %v\n", rcPath, werr)
		return errSilent
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "shll shell-setup: close %s: %v\n", rcPath, cerr)
		return errSilent
	}
	fmt.Fprintf(stdout, "Installed shll shell integration to %s. Restart your shell or run: source %s\n", rcPath, rcPath)
	return nil
}

// rewriteBlocks performs an in-place rewrite: it splices out every existing shll
// block (new and/or legacy) and inserts the merged block at the position of the
// earliest removed block. This is read-modify-write, so the symlink chain is
// resolved before an O_TRUNC write to the real file (EvalSymlinks→O_TRUNC) — the
// same strategy uninstall uses. When the resulting content is byte-identical to
// the original (the desired eval-only block already the sole new-sentinel block),
// the file is left untouched and a no-op message is emitted (idempotency).
func rewriteBlocks(rcPath string, content, block []byte, newM blockMatch, newOK bool, legacyM blockMatch, legacyOK bool, stdout, stderr io.Writer) error {
	// Determine the insertion anchor (earliest existing block start) and splice out
	// all existing block ranges. Ranges never overlap (distinct sentinels), so we
	// remove from the later range first to keep indices valid.
	insertAt := -1
	type rng struct{ start, end int }
	var ranges []rng
	if newOK {
		ranges = append(ranges, rng{newM.start, newM.end})
	}
	if legacyOK {
		ranges = append(ranges, rng{legacyM.start, legacyM.end})
	}
	for _, r := range ranges {
		if insertAt == -1 || r.start < insertAt {
			insertAt = r.start
		}
	}
	// Remove ranges in descending start order so earlier indices stay valid.
	if len(ranges) == 2 && ranges[0].start > ranges[1].start {
		ranges[0], ranges[1] = ranges[1], ranges[0]
	}
	work := content
	for i := len(ranges) - 1; i >= 0; i-- {
		r := ranges[i]
		spliced := make([]byte, 0, len(work)-(r.end-r.start))
		spliced = append(spliced, work[:r.start]...)
		spliced = append(spliced, work[r.end:]...)
		work = spliced
	}
	// insertAt now indexes into `work` (removals at >= insertAt shift only content
	// after the anchor, so the anchor index itself is unchanged).
	merged := make([]byte, 0, len(work)+len(block))
	merged = append(merged, work[:insertAt]...)
	merged = append(merged, block...)
	merged = append(merged, work[insertAt:]...)

	if bytes.Equal(merged, content) {
		fmt.Fprintf(stderr, "shll shell-setup: already installed in %s (no changes).\n", rcPath)
		return nil
	}

	resolved, err := filepath.EvalSymlinks(rcPath)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-setup: resolve symlink %s: %v\n", rcPath, err)
		return errSilent
	}
	f, err := os.OpenFile(resolved, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-setup: open %s: %v\n", resolved, err)
		return errSilent
	}
	if _, werr := f.Write(merged); werr != nil {
		_ = f.Close()
		fmt.Fprintf(stderr, "shll shell-setup: write %s: %v\n", resolved, werr)
		return errSilent
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "shll shell-setup: close %s: %v\n", resolved, cerr)
		return errSilent
	}
	fmt.Fprintf(stdout, "Installed shll shell integration to %s. Restart your shell or run: source %s\n", rcPath, rcPath)
	return nil
}

// runShellSetupPrint implements --print mode. Resolves shell + rc file the
// same way as default, still errors on missing rc file (the user may be
// debugging exactly that), then prints the exact eval-only block to stdout with
// no surrounding messages. It modifies NO file.
func runShellSetupPrint(shell, rcPath string, stdout, stderr io.Writer) error {
	if _, err := os.Stat(rcPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-setup: %s does not exist.", rcPath)}
		}
		fmt.Fprintf(stderr, "shll shell-setup: stat %s: %v\n", rcPath, err)
		return errSilent
	}
	if _, err := stdout.Write(buildBlock(shell)); err != nil {
		fmt.Fprintf(stderr, "shll shell-setup: write stdout: %v\n", err)
		return errSilent
	}
	return nil
}

// runShellSetupUninstall implements --uninstall mode. It removes the ENTIRE
// shll-managed block (the eval line, both sentinels) in one operation,
// recognizing BOTH the new `# >>> shll >>>` sentinel AND a legacy
// `# >>> shll shell-init >>>` block (so users who never re-installed can still
// uninstall).
//
// Missing rc file is not an error here (nothing to uninstall is benign). When a
// block is present, the symlink chain is resolved before the truncate-write so
// dotfile-manager symlinks stay intact while the underlying source-of-truth file
// is updated.
func runShellSetupUninstall(shell, rcPath string, stdout, stderr io.Writer) error {
	_ = shell // shell isn't used during uninstall — sentinels are shell-agnostic.
	if _, err := os.Stat(rcPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stderr, "shll shell-setup: %s does not exist (nothing to uninstall).\n", rcPath)
			return nil
		}
		fmt.Fprintf(stderr, "shll shell-setup: stat %s: %v\n", rcPath, err)
		return errSilent
	}
	content, err := os.ReadFile(rcPath)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-setup: read %s: %v\n", rcPath, err)
		return errSilent
	}
	newM, newOK, legacyM, legacyOK, _ := locateBlock(content)
	if !newOK && !legacyOK {
		fmt.Fprintf(stderr, "shll shell-setup: not installed in %s (nothing to uninstall).\n", rcPath)
		return nil
	}
	// Splice out every shll block. Remove the later range first so earlier indices
	// stay valid (the two sentinels never overlap).
	type rng struct{ start, end int }
	var ranges []rng
	if newOK {
		ranges = append(ranges, rng{newM.start, newM.end})
	}
	if legacyOK {
		ranges = append(ranges, rng{legacyM.start, legacyM.end})
	}
	if len(ranges) == 2 && ranges[0].start > ranges[1].start {
		ranges[0], ranges[1] = ranges[1], ranges[0]
	}
	modified := content
	for i := len(ranges) - 1; i >= 0; i-- {
		r := ranges[i]
		spliced := make([]byte, 0, len(modified)-(r.end-r.start))
		spliced = append(spliced, modified[:r.start]...)
		spliced = append(spliced, modified[r.end:]...)
		modified = spliced
	}
	resolved, err := filepath.EvalSymlinks(rcPath)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-setup: resolve symlink %s: %v\n", rcPath, err)
		return errSilent
	}
	f, err := os.OpenFile(resolved, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		fmt.Fprintf(stderr, "shll shell-setup: open %s: %v\n", resolved, err)
		return errSilent
	}
	if _, werr := f.Write(modified); werr != nil {
		_ = f.Close()
		fmt.Fprintf(stderr, "shll shell-setup: write %s: %v\n", resolved, werr)
		return errSilent
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "shll shell-setup: close %s: %v\n", resolved, cerr)
		return errSilent
	}
	fmt.Fprintf(stdout, "Removed shll shell integration from %s.\n", rcPath)
	return nil
}
