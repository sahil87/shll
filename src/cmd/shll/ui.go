package main

import (
	"fmt"
	"io"
	"os"

	"golang.org/x/term"
)

// ANSI SGR (Select Graphic Rendition) escape sequences used to style shll's own
// framing output. Hand-rolled standard codes declared as named constants so the
// call sites never open-code escape strings (code-quality.md: named constants, no
// magic strings). shll deliberately takes no external color-library dependency —
// these four constants cover the two styled glyphs (a bold-cyan header arrow and a
// green success check). Sub-tool output is never styled by shll; it streams through
// untouched.
const (
	ansiReset    = "\033[0m"
	ansiBold     = "\033[1m"
	ansiBoldCyan = "\033[1;36m"
	ansiGreen    = "\033[32m"
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
// foregrounded output, used by `shll update` and `shll install`. With color it reads
// `▸ <name>` (bold-cyan arrow + bold name); plain it reads `==> <name>` in pure ASCII
// with no ANSI. The `==>` idiom matches Homebrew's own convention so the plain form
// reads naturally alongside brew output. The color decision is passed in (computed
// once by the caller via colorEnabled) so the function is trivially testable.
func printToolHeader(w io.Writer, name string, color bool) {
	if color {
		fmt.Fprintf(w, "%s▸%s %s%s%s\n", ansiBoldCyan, ansiReset, ansiBold, name, ansiReset)
		return
	}
	fmt.Fprintf(w, "==> %s\n", name)
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

// printSummaryTail writes the one-line run summary emitted by `shll update` and
// `shll install` after all tools have run, derived from EXIT CODES only (succeeded
// reflects success, total reflects total tools attempted). On full success it reads
// `Done — N of M tools succeeded.`; on partial failure `X succeeded, Y failed — see
// above.`. It NEVER claims "updated" vs. "up-to-date" — streamed sub-tool output means
// shll knows only exit codes (the honesty constraint). With color, the full-success
// line is prefixed with a green `✓` (the only color/glyph the ASCII-degrade rule
// strips); the literal tail wording — including the em-dash, the spec-mandated form —
// is identical in both branches. The tail never influences the process exit code.
func printSummaryTail(w io.Writer, succeeded, total int, color bool) {
	if succeeded == total {
		if color {
			fmt.Fprintf(w, "%s✓%s Done — %d of %d tools succeeded.\n", ansiGreen, ansiReset, succeeded, total)
			return
		}
		fmt.Fprintf(w, "Done — %d of %d tools succeeded.\n", succeeded, total)
		return
	}
	failed := total - succeeded
	fmt.Fprintf(w, "%d succeeded, %d failed — see above.\n", succeeded, failed)
}
