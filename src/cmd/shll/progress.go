package main

import (
	"fmt"
	"io"
	"os"
	"strings"

	"golang.org/x/term"
)

// OSC 9;4 terminal-progress sequence pieces (the ConEmu / Windows Terminal
// convention, parsed by xterm.js via @xterm/addon-progress — the run-kit
// dashboard tile consumer). A sequence is `ESC ] 9 ; 4 ; {state} ; {percent} BEL`;
// terminals that do not understand it consume and ignore it harmlessly. Named
// constants per code-quality.md (no magic strings).
const (
	oscProgressPrefix = "\x1b]9;4;"
	oscProgressSuffix = "\x07" // BEL terminator — the convention's canonical form
)

// OSC 9;4 progress-state codes. Only the four states shll uses are named; the
// convention's remaining state (4 = paused/warning) has no shll semantics.
const (
	progressStateRemove        = 0 // clear the terminal's progress state
	progressStateSet           = 1 // determinate progress, percent 0–100
	progressStateError         = 2 // error-colored progress state
	progressStateIndeterminate = 3 // activity without a known fraction
)

// tmux DCS passthrough envelope. Inside tmux an OSC sequence is consumed by tmux
// itself and never reaches the outer terminal; wrapping it as
// `ESC P tmux ; {sequence with every ESC doubled} ESC \` forwards it verbatim when
// the session has `allow-passthrough on` (run-kit sessions do). Only ESC bytes are
// doubled — the BEL terminator rides through untouched.
const (
	tmuxPassthroughPrefix = "\x1bPtmux;"
	tmuxPassthroughSuffix = "\x1b\\"
)

// tmuxEnv is the environment variable whose presence means the process runs
// inside a tmux pane (and progress sequences need the passthrough envelope).
const tmuxEnv = "TMUX"

// progressWriterIsTTY is the package-level seam reporting whether the progress
// writer is an interactive terminal — the enablement gate for OSC 9;4 emission.
// It is a swappable var (mirroring the stdinIsTTY / proc.Runner / nowFunc
// injection pattern) so tests can force the enabled branch deterministically: a
// bytes.Buffer test writer is never a real *os.File terminal, so without a seam
// the emission path could never be exercised. The default is
// defaultProgressWriterIsTTY.
var progressWriterIsTTY = defaultProgressWriterIsTTY

// defaultProgressWriterIsTTY is the production implementation: true only when w
// is a real terminal (w is an *os.File AND term.IsTerminal reports true for its
// descriptor). It mirrors defaultStdinIsTTY's structure exactly. Like that gate —
// and unlike colorEnabled — it does NOT consult NO_COLOR: that convention governs
// styling, and OSC 9;4 is terminal progress STATE, not styling; a pipe/CI/test
// stream is already excluded by the TTY check.
func defaultProgressWriterIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// progressReporter emits OSC 9;4 terminal-progress sequences for `shll update`'s
// write phase. It is presentation-only — no subprocess calls (Constitution I) and
// no state (Constitution II); every method is a no-op when the reporter is
// disabled, so a non-TTY stream (pipe, CI, test buffer) sees zero bytes and the
// feature is entirely inert there.
type progressReporter struct {
	// w receives the sequences — stderr in production, so the invisible control
	// channel stays out of piped stdout while still reaching the terminal.
	w io.Writer
	// enabled gates all emission: true only when w is a real TTY (via the
	// progressWriterIsTTY seam at construction).
	enabled bool
	// tmux wraps every sequence in the DCS passthrough envelope so it survives
	// tmux and reaches the outer terminal / xterm.js tile.
	tmux bool
}

// newProgressReporter builds the reporter for w. env resolves $TMUX (injected —
// production passes the same env func runUpdate already threads, so tests control
// tmux detection without touching the real environment).
func newProgressReporter(w io.Writer, env func(string) string) *progressReporter {
	return &progressReporter{
		w:       w,
		enabled: progressWriterIsTTY(w),
		tmux:    env(tmuxEnv) != "",
	}
}

// emit writes one OSC 9;4 sequence for (state, percent), applying the tmux
// passthrough envelope when needed. The single funnel every public method goes
// through — the wrap/no-wrap decision lives in exactly one place.
func (p *progressReporter) emit(state, percent int) {
	if !p.enabled {
		return
	}
	seq := fmt.Sprintf("%s%d;%d%s", oscProgressPrefix, state, percent, oscProgressSuffix)
	if p.tmux {
		seq = tmuxPassthroughPrefix + strings.ReplaceAll(seq, "\x1b", "\x1b\x1b") + tmuxPassthroughSuffix
	}
	fmt.Fprint(p.w, seq)
}

// set reports determinate progress (percent 0–100).
func (p *progressReporter) set(percent int) { p.emit(progressStateSet, percent) }

// indeterminate reports activity without a known fraction (e.g. the run-wide
// `brew update --quiet` before the per-tool loop).
func (p *progressReporter) indeterminate() { p.emit(progressStateIndeterminate, 0) }

// errorState reports an error-colored progress state at the given percent (a
// failed tool mid-run, or the run tail after any failure).
func (p *progressReporter) errorState(percent int) { p.emit(progressStateError, percent) }

// remove clears the terminal's progress state. Deferred by runUpdate at
// construction so every post-construction exit path — success, the brew-update
// failure return, a panic — leaves no stale progress bar behind.
func (p *progressReporter) remove() { p.emit(progressStateRemove, 0) }
