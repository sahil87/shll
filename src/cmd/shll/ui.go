package main

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"
)

// ANSI SGR (Select Graphic Rendition) escape sequences used to style shll's own
// framing output. Hand-rolled standard codes declared as named constants so the
// call sites never open-code escape strings (code-quality.md: named constants, no
// magic strings). shll deliberately takes no external color-library dependency —
// these constants cover shll's few styling roles: the bold-cyan per-tool header
// run, the bold anchor lines on the changelog surfaces (via bold), and the
// green/red status glyphs. Sub-tool output is never styled by shll; it streams
// through untouched.
const (
	ansiReset    = "\033[0m"
	ansiBold     = "\033[1m"
	ansiBoldCyan = "\033[1;36m"
	ansiGreen    = "\033[32m"
	ansiRed      = "\033[31m"
)

// noColorEnv is the environment variable that, when set to any value, disables
// color/Unicode per the no-color.org convention. Named constant per code-quality.md.
const noColorEnv = "NO_COLOR"

// colorEnabled reports whether shll may emit ANSI color and Unicode glyphs to w.
// Both conditions must hold: (1) w is a real terminal — w is an *os.File AND
// term.IsTerminal reports true for its descriptor, AND (2) NO_COLOR is unset
// (no-color.org). A bytes.Buffer (test writer) or any non-*os.File is never a
// terminal, so it deterministically selects the plain-ASCII branch. This is shll's
// first terminal inspection — the lone reason for the golang.org/x/term dependency.
func colorEnabled(w io.Writer) bool {
	if _, ok := os.LookupEnv(noColorEnv); ok {
		return false
	}
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// printToolHeader writes one labeled boundary line to w immediately before a tool's
// foregrounded output, used by `shll update`, `shll install`, and `shll changelog`. It
// carries a 1-based progress counter `[pos/total]` so the user can see how far along a
// multi-tool run is (`total` is computed once by the caller before the loop). With color
// the WHOLE `▸ [N/M] <name>` run is a single bold-cyan span so tool boundaries pop
// visually; plain it reads `==> [N/M] <name>` in pure ASCII with no ANSI. The `==>`
// idiom matches Homebrew's own convention so the plain form reads naturally alongside
// brew output. The color decision is passed in (computed once by the caller via
// colorEnabled) so the function is trivially testable. Because all three surfaces share
// this function, the bold-cyan header form covers them uniformly.
func printToolHeader(w io.Writer, name string, pos, total int, color bool) {
	if color {
		fmt.Fprintf(w, "%s▸ [%d/%d] %s%s\n", ansiBoldCyan, pos, total, name, ansiReset)
		return
	}
	fmt.Fprintf(w, "==> [%d/%d] %s\n", pos, total, name)
}

// stdinIsTTY is the package-level seam reporting whether an io.Reader is an
// interactive terminal — used by the `shll uninstall` confirmation gate to decide
// whether a `Proceed? [y/N]` prompt can be answered. It is a swappable var (mirroring
// the proc.Runner / nowFunc injection pattern) so tests can force the interactive
// branch deterministically: a bytes.Buffer / strings.Reader test stdin is never a real
// *os.File terminal, so without a seam the prompt path could never be exercised. The
// default is defaultStdinIsTTY.
var stdinIsTTY = defaultStdinIsTTY

// defaultStdinIsTTY is the production implementation: true only when r is a real
// terminal (r is an *os.File AND term.IsTerminal reports true for its descriptor). It
// mirrors colorEnabled's structure exactly. A pipe, a file redirect, or any
// non-*os.File is never a terminal, so a non-TTY stdin without --yes is refused
// (fail-safe for pipes/CI). Unlike colorEnabled it does NOT consult NO_COLOR — that
// governs styling, not interactivity.
func defaultStdinIsTTY(r io.Reader) bool {
	f, ok := r.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// Glyph pairs for shll's own framing output in the changelog surface (the
// `shll changelog` command and `shll update`'s "What changed:" digest). Each
// pairs a Unicode glyph (used on a color-enabled TTY) with an ASCII fallback
// (used on a non-TTY / NO_COLOR stream) per the per-tool-output-separation
// spec's degrade rule: shll's OWN Unicode output degrades to ASCII so piped
// output and CI logs stay clean. Named constants per code-quality.md.
//
// This is distinct from the summary-tail em-dash and the shell-init box-drawing
// chars, which the spec explicitly EXEMPTS from the degrade (spec-defined
// wording kept verbatim in both branches). These three glyphs sit in the
// changelog surface's own framing, which the spec's general degrade rule covers.
const (
	arrowGlyph = "→"
	arrowASCII = "->"
	dashGlyph  = "—"
	dashASCII  = "--"
	moreGlyph  = "…"
	moreASCII  = "..."
)

// arrow returns the version-transition arrow (`old → new`), degraded to `->` on
// a non-color stream.
func arrow(color bool) string {
	if color {
		return arrowGlyph
	}
	return arrowASCII
}

// dash returns the compare-URL fallback separator (`… — see <url>`), degraded to
// `--` on a non-color stream.
func dash(color bool) string {
	if color {
		return dashGlyph
	}
	return dashASCII
}

// more returns the cap-notice ellipsis (`… N more`), degraded to `...` on a
// non-color stream.
func more(color bool) string {
	if color {
		return moreGlyph
	}
	return moreASCII
}

// bold wraps s in a plain bold ANSI span (ansiBold … ansiReset) when color is
// enabled, returning s unchanged otherwise. It is the single place the
// changelog-surface anchor styling (the digest / `shll changelog` transition
// lines and release tag/title lines) is applied, so no call site open-codes an
// escape string (code-quality.md: named constants, no magic strings). Plain
// BOLD, not bold-cyan — bold-cyan (printToolHeader) stays reserved for the
// per-tool headers so the visual hierarchy header > anchor > body is preserved.
// Like the glyph helpers above, it is TTY-gated via the color decision threaded
// in from the caller; a non-TTY / NO_COLOR stream stays ANSI-free.
func bold(color bool, s string) string {
	if color {
		return ansiBold + s + ansiReset
	}
	return s
}

// toolComment returns the shell-comment separator emitted by `shll shell-init` before
// each contributing tool's init block, e.g. `# ── tu ──`. Unlike printToolHeader it is
// ALWAYS plain ASCII-safe shell-comment text — never colored, never TTY-gated — because
// shell-init stdout is consumed by `eval` (Constitution V, eval-safety): a bare header
// would be eval'd as a command and ANSI escapes would corrupt the shell, whereas a
// `#`-prefixed line is a shell no-op. The returned string has no trailing newline; the
// caller appends one.
func toolComment(name string) string {
	return fmt.Sprintf("# ── %s ──", name)
}

// formatDuration renders a run duration in the restrained `1m12s` form used by the
// summary tail. Multi-second runs round to whole seconds (so `time.Duration`'s default
// String() doesn't surface noisy nanosecond fractions); sub-second runs round to `0s`.
// This is the single place the duration string is shaped, so the tail wording stays a
// named contract rather than open-coded.
func formatDuration(d time.Duration) string {
	return d.Round(time.Second).String()
}

// printSummaryTail writes the one-line run summary emitted by `shll update` and
// `shll install` after all tools have run, derived from EXIT CODES only (succeeded
// reflects success, total reflects total tools attempted). It appends the wall-clock
// run duration to BOTH forms: full success reads `Done — N of M tools succeeded in
// <dur>.`; partial failure reads `X succeeded, Y failed in <dur> — see above.` (the
// duration sits BEFORE the em-dash). The duration is a FACT about the run, not an
// outcome claim — the tail still NEVER claims "updated" vs. "up-to-date" (streamed
// sub-tool output means shll knows only exit codes — the honesty constraint). With
// color, the full-success line is prefixed with a green `✓` (the only color/glyph the
// ASCII-degrade rule strips); the literal tail wording — including the em-dash, the
// spec-mandated form — is identical in both branches. The tail never influences the
// process exit code.
func printSummaryTail(w io.Writer, succeeded, total int, elapsed time.Duration, color bool) {
	dur := formatDuration(elapsed)
	if succeeded == total {
		if color {
			fmt.Fprintf(w, "%s✓%s Done — %d of %d tools succeeded in %s.\n", ansiGreen, ansiReset, succeeded, total, dur)
			return
		}
		fmt.Fprintf(w, "Done — %d of %d tools succeeded in %s.\n", succeeded, total, dur)
		return
	}
	failed := total - succeeded
	fmt.Fprintf(w, "%d succeeded, %d failed in %s — see above.\n", succeeded, failed, dur)
}

// previewRow is one line of a `--dry-run` preview: a tool label and the exact argv
// `shll update` / `shll install` WOULD run for it, rendered as a display string. The
// command code builds these from probe results (so the argv mirrors the real run);
// ui.go only aligns and prints them, keeping subprocess knowledge out of the
// presentation layer (Constitution I — ui.go makes no subprocess calls).
type previewRow struct {
	label string
	cmd   string
}

// updatePreviewHeaderFmt and installPreviewHeaderFmt are the header lines of the
// dry-run previews. Named constants per code-quality.md (no magic strings). The update
// header annotates that the real run refreshes brew metadata first — but dry-run does
// NOT run `brew update` (it is a write). Install has no metadata-refresh step, so its
// header omits the annotation.
const (
	updatePreviewHeaderFmt    = "Would update %d tools (brew metadata refresh first):"
	installPreviewHeaderFmt   = "Would install %d tools:"
	uninstallPreviewHeaderFmt = "Would uninstall %d tools:"
	// previewIndent prefixes every preview row; previewGap separates the padded
	// label column from the command. Both are named so the alignment is not an
	// open-coded literal.
	previewIndent = "  "
	previewGap    = "  "
)

// printUpdatePreview prints the `shll update --dry-run` preview: a header line then one
// aligned row per tool, labels left-padded to the longest label present so the commands
// line up. Presentation-only — no subprocess calls.
func printUpdatePreview(w io.Writer, rows []previewRow) {
	fmt.Fprintf(w, updatePreviewHeaderFmt+"\n", len(rows))
	printPreviewRows(w, rows)
}

// printInstallPreview prints the `shll install --dry-run` preview, mirroring
// printUpdatePreview's aligned-column layout with the install-specific header (no
// metadata-refresh annotation). Presentation-only — no subprocess calls.
func printInstallPreview(w io.Writer, rows []previewRow) {
	fmt.Fprintf(w, installPreviewHeaderFmt+"\n", len(rows))
	printPreviewRows(w, rows)
}

// printUninstallPreview prints the `shll uninstall --dry-run` preview, mirroring
// printUpdatePreview's aligned-column layout with the uninstall-specific header (no
// metadata-refresh annotation). Presentation-only — no subprocess calls.
func printUninstallPreview(w io.Writer, rows []previewRow) {
	fmt.Fprintf(w, uninstallPreviewHeaderFmt+"\n", len(rows))
	printPreviewRows(w, rows)
}

// printPreviewRows writes the aligned tool/command rows shared by both previews. Labels
// are left-padded to the widest label so the command column lines up.
func printPreviewRows(w io.Writer, rows []previewRow) {
	width := 0
	for _, r := range rows {
		if len(r.label) > width {
			width = len(r.label)
		}
	}
	for _, r := range rows {
		pad := strings.Repeat(" ", width-len(r.label))
		fmt.Fprintf(w, "%s%s%s%s%s\n", previewIndent, r.label, pad, previewGap, r.cmd)
	}
}
