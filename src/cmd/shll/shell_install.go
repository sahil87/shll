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
// The combined block uses the new `# >>> shll >>>` / `# <<< shll <<<` sentinel
// pair (note the close sentinel uses `<<<`). It holds the union of managed lines
// that apply — the export line (when genuine tap-trust is in effect) and/or the
// eval line — in export-before-eval order. The legacy `# >>> shll shell-init >>>`
// pair is recognized only for migration and uninstall of pre-existing blocks.
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

	// exportTrustLine is the policy line that opts the user into Homebrew's
	// require-tap-trust mode. Written only alongside a successful `brew trust`
	// ceremony — the policy line without a trust record would cause brew to BLOCK
	// the tap (strictly worse than the warning), so degradation paths skip it.
	exportTrustLine = "export HOMEBREW_REQUIRE_TAP_TRUST=1"
)

func newShellInstallCmd() *cobra.Command {
	var (
		printMode     bool
		uninstallMode bool
		trustTap      bool
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

The --trust-tap flag records genuine Homebrew trust for the sahil87 tap. It is
not a mode — it composes with the default, --print, and --uninstall paths:
  shll shell-install --trust-tap          run ` + "`brew trust --tap sahil87/tap`" + ` and add
                                          ` + "`export HOMEBREW_REQUIRE_TAP_TRUST=1`" + ` to the block
  shll shell-install --trust-tap --print  print the resulting combined block, change nothing
If ` + "`brew trust`" + ` is unavailable (older brew) or brew is missing, the export line
is skipped and only the eval line is written — shll never sets the policy line
without a backing trust record.

When [shell] is omitted, shll infers it from $SHELL. Supported shells: zsh, bash.

By default, the rc file path is derived per shell:
  zsh   → ${ZDOTDIR:-$HOME}/.zshrc
  bash  → $HOME/.bash_profile (macOS) or $HOME/.bashrc (Linux)

Use --rc-file <path> to override derivation entirely.`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runShellInstall(cmd.Context(), args, rcFileFlag, printMode, uninstallMode, trustTap, ensureTapTrust, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
	cmd.Flags().BoolVar(&printMode, "print", false, "print the block to stdout, do not modify any file")
	cmd.Flags().BoolVar(&uninstallMode, "uninstall", false, "remove the shll-managed block from the rc file")
	cmd.Flags().BoolVar(&trustTap, "trust-tap", false, "run `brew trust --tap sahil87/tap` and add the require-tap-trust policy line to the block")
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

// evalLine returns the eval body line for the resolved shell:
// `eval "$(shll shell-init <shell>)"`.
func evalLine(shell string) string {
	return fmt.Sprintf(evalLineFmt, shell)
}

// buildBlockBody wraps an ordered set of managed lines in the new sentinel pair,
// terminated by a single trailing \n. Callers pass the lines in canonical order
// (export before eval); buildBlockBody does not reorder or dedup — the merge
// logic upstream is responsible for that.
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

// wantLines computes the canonical, ordered set of managed lines a block should
// contain after this invocation. It is the per-line MERGE rule: the union of
// (a) what an existing block already carries (existing) and (b) what this
// invocation adds — the eval line always, plus the export line when
// wantExport is true (trust ceremony succeeded). Order is canonical:
// export before eval (policy set before any eval-time brew invocation reads it).
//
// existing carries the parse result of the located block (zero value when no
// block exists yet — the fresh-install case). The eval line is always desired,
// so a block that somehow lacked it gains it; this also covers an export-only
// block later running a plain `shll shell-install`.
func wantLines(existing blockMatch, shell string, wantExport bool) []string {
	export := existing.hasExport || wantExport
	lines := make([]string, 0, 2)
	if export {
		lines = append(lines, exportTrustLine)
	}
	lines = append(lines, evalLine(shell))
	return lines
}

// buildBlock returns the eval-only block under the new sentinel for the given
// shell. Retained as the canonical single-line builder used by --print (no
// --trust-tap) and as the simplest fresh-install body; it routes through
// buildBlockBody so every path shares the same sentinel constants.
func buildBlock(shell string) []byte {
	return buildBlockBody([]string{evalLine(shell)})
}

// blockMatch describes a located shll-managed block: its inclusive byte range
// (content[start:end] covers the open sentinel through the trailing \n after the
// close sentinel) and the managed lines extracted from its body.
type blockMatch struct {
	start, end int
	// hasExport / hasEval report which managed lines the existing block carries.
	hasExport bool
	hasEval   bool
}

// findBlockWith locates the inclusive byte range of a sentinel block delimited by
// the given open/close sentinels, and extracts which managed lines it contains.
//
// Returns ok=false when the open sentinel is absent. Returns partial=true when
// the open sentinel is present but the matching close sentinel is not — an
// unclosed/corrupted block that the caller MUST refuse to auto-repair (guessing
// its bounds risks corrupting the rc file).
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
		switch {
		case trimmed == exportTrustLine:
			m.hasExport = true
		case strings.HasPrefix(trimmed, evalLinePrefix):
			m.hasEval = true
		}
	}
	return m, true, false
}

// evalLinePrefix is the shell-agnostic prefix shared by every eval body line
// (`eval "$(shll shell-init zsh)"`, `... bash)"`). Used to recognize an existing
// eval line during a merge regardless of which shell it was installed for.
const evalLinePrefix = `eval "$(shll shell-init`

// ensureTrustFunc is the ceremony seam: given a context it runs the genuine-trust
// ceremony and reports whether the policy (export) line should be written, plus a
// diagnostic to print when it should not. The production implementation lives in
// brew.go (ensureTapTrust), which is the file that legitimately performs
// subprocess execution. Threading it as a function value keeps this file free of
// any subprocess-execution import (the TestNoProcImports guard pins it to file
// I/O only) while still letting tests drive the ceremony through a swapped fake
// runner against ensureTapTrust.
type ensureTrustFunc func(ctx context.Context) (writeExport bool, diag string)

// runShellInstall is the implementation seam invoked by the cobra factory's
// RunE. Extracted so tests can drive it directly with bytes.Buffer writers and
// controlled environment.
//
// trustTap is an orthogonal selector (NOT a mutually-exclusive mode): it composes
// with the default and --print paths. ensureTrust is the ceremony seam (see
// ensureTrustFunc) — invoked only on the default --trust-tap path.
func runShellInstall(ctx context.Context, args []string, rcFileFlag string, printMode, uninstallMode, trustTap bool, ensureTrust ensureTrustFunc, stdout, stderr io.Writer) error {
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
		return runShellInstallPrint(shell, rcPath, trustTap, stdout, stderr)
	case uninstallMode:
		return runShellInstallUninstall(shell, rcPath, stdout, stderr)
	default:
		return runShellInstallDefault(ctx, shell, rcPath, rcFileFlag != "", trustTap, ensureTrust, stdout, stderr)
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

// runShellInstallDefault implements the default install path as a per-line MERGE
// into the single shll-managed block (new `# >>> shll >>>` sentinel). The flow:
//
//	stat (no O_CREATE) → read → locate block (new + legacy) → refuse on partial →
//	run ceremony (--trust-tap only) → compute desired line union → no-op if the
//	block is already byte-identical → otherwise rewrite in place (in-block /
//	migration / both-sentinels-present) or append a fresh block.
//
// Two write strategies preserve the symlink invariant: a fresh append uses plain
// O_APPEND (follows the symlink, atomic for a sub-PIPE_BUF block); an in-place
// rewrite (existing block present, or migration) is read-modify-write and so
// resolves the symlink chain before an O_TRUNC write to the real file.
//
// userProvidedPath controls the missing-file error wording: with --rc-file the
// user named the path explicitly, so the "shll won't create rc files" hint is
// dropped.
func runShellInstallDefault(ctx context.Context, shell, rcPath string, userProvidedPath, trustTap bool, ensureTrust ensureTrustFunc, stdout, stderr io.Writer) error {
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

	newM, newOK, legacyM, legacyOK, partial := locateBlock(content)
	if partial {
		// Open-without-close sentinel — corrupted/partial. Guessing the bounds
		// risks corrupting the user's rc file, so refuse and direct manual cleanup
		// (deliberate divergence from the legacy short-circuit-as-"already-installed").
		return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-install: %s has an shll block with an opening sentinel but no matching closing sentinel. Refusing to modify a corrupted block — fix or remove it manually, then re-run.", rcPath)}
	}

	// Run the ceremony before composing the block: its outcome decides whether the
	// export line belongs in the desired set. The eval line is always written, even
	// on degradation (Constitution V), so the user keeps shell integration.
	wantExport := false
	if trustTap {
		ok, diag := ensureTrust(ctx)
		wantExport = ok
		if diag != "" {
			fmt.Fprintln(stderr, diag)
		}
	}

	// Merge the managed lines from any pre-existing block(s) so already-present
	// lines carry forward (per-line union). Both-sentinels-present folds the legacy
	// block's lines into the new block and removes the legacy one (self-healing).
	var existing blockMatch
	existing.hasExport = (newOK && newM.hasExport) || (legacyOK && legacyM.hasExport)
	existing.hasEval = (newOK && newM.hasEval) || (legacyOK && legacyM.hasEval)
	desired := buildBlockBody(wantLines(existing, shell, wantExport))

	switch {
	case !newOK && !legacyOK:
		// No existing block — append a fresh one (plain O_APPEND, symlink-safe).
		return appendBlock(rcPath, content, desired, stdout, stderr)
	default:
		// One or both blocks exist — rewrite in place. Splice out every existing
		// shll block and insert the merged block at the earliest block position.
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

// rewriteBlocks performs an in-place rewrite: it splices out every existing shll
// block (new and/or legacy) and inserts the merged block at the position of the
// earliest removed block. This is read-modify-write, so the symlink chain is
// resolved before an O_TRUNC write to the real file (EvalSymlinks→O_TRUNC) — the
// same strategy uninstall uses. When the resulting content is byte-identical to
// the original (every desired line already present, single new-sentinel block),
// the file is left untouched and a no-op message is emitted (per-line idempotency).
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
		fmt.Fprintf(stderr, "shll shell-install: already installed in %s (no changes).\n", rcPath)
		return nil
	}

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
	if _, werr := f.Write(merged); werr != nil {
		_ = f.Close()
		fmt.Fprintf(stderr, "shll shell-install: write %s: %v\n", resolved, werr)
		return errSilent
	}
	if cerr := f.Close(); cerr != nil {
		fmt.Fprintf(stderr, "shll shell-install: close %s: %v\n", resolved, cerr)
		return errSilent
	}
	fmt.Fprintf(stdout, "Installed shll shell integration to %s. Restart your shell or run: source %s\n", rcPath, rcPath)
	return nil
}

// runShellInstallPrint implements --print mode. Resolves shell + rc file the
// same way as default, still errors on missing rc file (the user may be
// debugging exactly that), then prints the exact block to stdout with no
// surrounding messages. It runs NO ceremony and modifies NO file. With
// --trust-tap the printed block is the combined block (export line before eval
// line) so the dry-run reflects what a real --trust-tap install would write.
func runShellInstallPrint(shell, rcPath string, trustTap bool, stdout, stderr io.Writer) error {
	if _, err := os.Stat(rcPath); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-install: %s does not exist.", rcPath)}
		}
		fmt.Fprintf(stderr, "shll shell-install: stat %s: %v\n", rcPath, err)
		return errSilent
	}
	block := buildBlock(shell)
	if trustTap {
		// --print is a dry-run: it cannot probe brew (no ceremony), so it shows the
		// block a successful --trust-tap install would produce (export + eval).
		block = buildBlockBody(wantLines(blockMatch{}, shell, true))
	}
	if _, err := stdout.Write(block); err != nil {
		fmt.Fprintf(stderr, "shll shell-install: write stdout: %v\n", err)
		return errSilent
	}
	return nil
}

// runShellInstallUninstall implements --uninstall mode. It removes the ENTIRE
// shll-managed block (both managed lines, both sentinels) in one operation,
// recognizing BOTH the new `# >>> shll >>>` sentinel AND a legacy
// `# >>> shll shell-init >>>` block (so users who never re-installed can still
// uninstall). It does NOT run `brew untrust` — the trust record is inert without
// the policy line and is the user's to reverse (`brew untrust` is idempotent).
//
// Missing rc file is not an error here (nothing to uninstall is benign). When a
// block is present, the symlink chain is resolved before the truncate-write so
// dotfile-manager symlinks stay intact while the underlying source-of-truth file
// is updated.
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
	newM, newOK, legacyM, legacyOK, _ := locateBlock(content)
	if !newOK && !legacyOK {
		fmt.Fprintf(stderr, "shll shell-install: not installed in %s (nothing to uninstall).\n", rcPath)
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
