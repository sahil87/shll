package main

import (
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"golang.org/x/term"
)

// Terminal escape sequences for the two-region layout (a pinned one-line
// status header above a DECSTBM scroll region). Hand-rolled standard codes
// declared as named constants so call sites never open-code escape strings
// (code-quality.md) — the same posture as ui.go's SGR constants. DECSTBM sets
// the scroll margins; the header lives in the line above the top margin, so
// child output scrolls beneath it while the header stays pinned. Terminals
// that do not understand DECSTBM consume the sequences harmlessly.
const (
	// regionMarginSetFmt sets top/bottom scroll margins: ESC[{top};{bottom}r.
	regionMarginSetFmt = "\x1b[%d;%dr"
	// regionMarginReset resets the scroll margins to the full screen: ESC[r.
	regionMarginReset = "\x1b[r"
	// regionCursorHome moves the cursor to row 1, column 1: ESC[H.
	regionCursorHome = "\x1b[H"
	// regionCursorPosFmt moves the cursor to {row}, column 1: ESC[{row};1H.
	regionCursorPosFmt = "\x1b[%d;1H"
	// regionEraseLine clears the line under the cursor (EL 2): ESC[2K.
	regionEraseLine = "\x1b[2K"
)

// regionScrollTop is the 1-based terminal row where the scroll region starts —
// the pinned header owns row 1, so child output scrolls from row 2 down. It
// feeds both the DECSTBM top margin and the cursor parking position. Named per
// code-quality.md (no magic numbers).
const regionScrollTop = 2

// regionWriterIsTTY is the package-level seam reporting whether the region
// writer is an interactive terminal — the enablement gate for the scroll
// region. It is a swappable var mirroring progressWriterIsTTY exactly: a
// bytes.Buffer test writer is never a real terminal, so without a seam the
// enabled branch could never be exercised. Like the OSC 9;4 gate it does NOT
// consult NO_COLOR — that convention governs styling, and a scroll region is
// terminal STATE, not styling (Design Decision: region gate is tty-only).
var regionWriterIsTTY = defaultRegionWriterIsTTY

// defaultRegionWriterIsTTY is the production implementation, mirroring
// defaultProgressWriterIsTTY byte-for-byte in structure.
func defaultRegionWriterIsTTY(w io.Writer) bool {
	f, ok := w.(*os.File)
	if !ok {
		return false
	}
	return term.IsTerminal(int(f.Fd()))
}

// terminalSize is the package-level seam reporting the terminal's (cols, rows)
// for the region writer — injectable so tests pin a deterministic size (a
// bytes.Buffer carries no fd to size). The production default reads the fd of
// an *os.File writer; a non-*os.File reports (0, 0) and the region stays
// disabled (an enabled region needs real geometry).
var terminalSize = defaultTerminalSize

// defaultTerminalSize is the production implementation: the fd's window size
// when w is an *os.File, (0, 0) otherwise.
func defaultTerminalSize(w io.Writer) (cols, rows int) {
	f, ok := w.(*os.File)
	if !ok {
		return 0, 0
	}
	cols, rows, err := term.GetSize(int(f.Fd()))
	if err != nil {
		return 0, 0
	}
	return cols, rows
}

// statusRegion manages the two-region terminal layout for `shll install` /
// `shll update` on a tty: a pinned one-line status header at the top of the
// screen and a DECSTBM scroll region beneath it in which child output streams
// naturally (no alternate screen, no output withholding). A disabled instance
// (non-tty writer, or a terminal reporting no usable geometry) is a total
// no-op — every method writes zero bytes, so the non-tty output stays
// byte-identical to the pre-region behavior.
//
// Lifecycle: start() draws the layout, setHeader(text) repaints the header,
// stop() restores the margins and releases the header line. start() also
// installs signal handlers: SIGWINCH re-applies the margins and header with
// the freshly queried size; SIGINT/SIGTERM restore the margins, remove the
// handlers, and re-raise the signal so the process keeps its default exit
// posture (no new child-kill logic — children receive the terminal-generated
// signal natively).
//
// Concurrency: the SIGWINCH goroutine can apply/repaint while the command
// goroutine sets headers and the child's tee writes stream through the same
// underlying writer, so ALL mutable state (cols/rows/header/active) and every
// write to r.w are serialized through mu.
type statusRegion struct {
	w       io.Writer
	enabled bool
	mu      sync.Mutex
	rows    int // terminal rows at start / last resize
	cols    int // terminal columns at start / last resize
	header  string
	active  bool // the region is currently applied (margins set)
	// winch/term are the signal channels. They are written only by
	// installHandlers (before the watcher goroutine starts) and nilled only by
	// removeHandlers under mu; the watcher receives them as arguments, so it
	// never reads the fields.
	winch chan os.Signal
	term  chan os.Signal
}

// newStatusRegion builds the region for w. The instance is enabled only when
// the regionWriterIsTTY seam reports a terminal AND terminalSize reports
// usable geometry (≥2 rows — one header line plus one scrollable line); every
// other case yields a disabled no-op instance.
func newStatusRegion(w io.Writer) *statusRegion {
	r := &statusRegion{w: w}
	if !regionWriterIsTTY(w) {
		return r
	}
	cols, rows := terminalSize(w)
	if rows < 2 || cols < 1 {
		return r
	}
	r.enabled = true
	r.cols = cols
	r.rows = rows
	return r
}

// start applies the layout: paint a blank line (consumed by the header line
// once the margins are set), pin the scroll region to the lines beneath the
// header, and park the cursor at the top of the region so subsequent output
// streams beneath the header. On an enabled instance it also installs the
// SIGWINCH/SIGINT/SIGTERM handlers.
func (r *statusRegion) start() {
	if !r.enabled {
		return
	}
	r.mu.Lock()
	fmt.Fprintln(r.w)
	r.applyLocked()
	r.mu.Unlock()
	r.installHandlers()
}

// applyLocked sets the margins for the current geometry, parks the cursor at
// the top of the scroll region, and repaints the header. Shared by start and
// the SIGWINCH path. Caller must hold mu.
func (r *statusRegion) applyLocked() {
	fmt.Fprintf(r.w, regionMarginSetFmt, regionScrollTop, r.rows)
	fmt.Fprint(r.w, regionCursorPos(regionScrollTop))
	r.paintHeaderLocked()
	r.active = true
}

// regionCursorPos renders the cursor-position sequence for a row.
func regionCursorPos(row int) string {
	return fmt.Sprintf(regionCursorPosFmt, row)
}

// setHeader updates the pinned header text. Plain text only — styling is the
// caller's job (the header builder applies the run's single color decision).
// The header is truncated to the current terminal width so it never wraps and
// corrupts the region geometry.
func (r *statusRegion) setHeader(text string) {
	if !r.enabled {
		return
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	r.header = text
	if r.active {
		r.paintHeaderLocked()
	}
}

// paintHeaderLocked redraws the header line in place: home the cursor, erase
// the line, write the (width-capped) text, then return the cursor to the top
// of the scroll region so streaming output continues where the user expects.
// Caller must hold mu.
func (r *statusRegion) paintHeaderLocked() {
	text := r.header
	if w := r.cols; w > 0 && len(text) > w {
		text = text[:w]
		// Truncation can cut through a trailing SGR span — re-assert reset so
		// the terminal never stays styled.
		if strings.Contains(text, "\x1b") {
			text += ansiReset
		}
	}
	fmt.Fprint(r.w, regionCursorHome, regionEraseLine, text, regionCursorPos(regionScrollTop))
}

// stop restores the terminal on the normal/error return path: remove the
// signal handlers, reset the scroll margins, release the header line (home,
// erase, cursor home — output resumes at the top of the now-full screen).
// Safe to call multiple times and on a never-started enabled instance.
func (r *statusRegion) stop() {
	if !r.enabled {
		return
	}
	r.removeHandlers()
	r.mu.Lock()
	defer r.mu.Unlock()
	r.restoreLocked()
}

// restoreLocked resets the margins and releases the header line. Shared by
// stop and the SIGINT/SIGTERM path. Caller must hold mu.
func (r *statusRegion) restoreLocked() {
	fmt.Fprint(r.w, regionMarginReset, regionCursorHome, regionEraseLine, regionCursorHome)
	r.active = false
}

// installHandlers wires the resize and termination signal paths. The channels
// are captured into locals and passed to the watcher as arguments — the
// goroutine never reads the struct fields, so it cannot race removeHandlers'
// nil-ing of them (a stop() that lands before the goroutine's first statement
// is then harmless). Both channels are buffered so a signal burst never blocks
// the runtime's signal delivery.
func (r *statusRegion) installHandlers() {
	r.mu.Lock()
	r.winch = make(chan os.Signal, 1)
	r.term = make(chan os.Signal, 1)
	winch, termc := r.winch, r.term
	r.mu.Unlock()
	signal.Notify(winch, syscall.SIGWINCH)
	signal.Notify(termc, syscall.SIGINT, syscall.SIGTERM)
	go r.watchSignals(winch, termc)
}

// removeHandlers detaches the signal.Notify registrations and closes the
// channels (closing lets the watcher goroutine exit). Idempotent — a second
// call after the channels were consumed is a no-op, so stop() is safe to
// repeat and safe after the SIGINT/SIGTERM path already detached.
func (r *statusRegion) removeHandlers() {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.winch == nil && r.term == nil {
		return
	}
	signal.Stop(r.winch)
	signal.Stop(r.term)
	close(r.winch)
	close(r.term)
	r.winch = nil
	r.term = nil
}

// watchSignals services the region's two signal paths until both channels
// close: SIGWINCH re-queries the terminal size and re-applies margins +
// header; SIGINT/SIGTERM restore the margins, detach the handlers, and
// re-raise the signal to the process's default disposition so the exit code
// and semantics stay the platform default (no new kill/timeout logic —
// children in the foreground process group receive the terminal-generated
// signal natively). All region state and writer access funnels through mu, so
// this goroutine cannot race the command goroutine's header/tee writes.
func (r *statusRegion) watchSignals(winch, termc chan os.Signal) {
	for {
		select {
		case _, ok := <-winch:
			if !ok {
				winch = nil
				if termc == nil {
					return
				}
				continue
			}
			if cols, rows := terminalSize(r.w); rows >= 2 && cols >= 1 {
				r.mu.Lock()
				r.cols = cols
				r.rows = rows
				r.applyLocked()
				r.mu.Unlock()
			}
		case sig, ok := <-termc:
			if !ok {
				termc = nil
				if winch == nil {
					return
				}
				continue
			}
			r.mu.Lock()
			r.restoreLocked()
			r.mu.Unlock()
			signal.Stop(winch)
			signal.Stop(termc)
			p, err := os.FindProcess(os.Getpid())
			if err == nil {
				_ = p.Signal(sig)
			}
		}
	}
}

// statusHeaderText builds the pinned-header line: verb + tool + honest (k/n)
// step count + an optional next-tool lookahead — e.g.
// `Installing rk-desktop (2/3) · next: fab-kit`; the final tool omits the
// `· next:` clause. The `·` separator ASCII-degrades to `-` on a non-color
// run (the run's single colorEnabled decision threaded in, mirroring ui.go's
// glyph helpers). With color the whole line is a single bold-cyan span so it
// reads as one pinned status line (printToolHeader's in-stream sibling).
func statusHeaderText(verb, tool string, pos, total int, next string, color bool) string {
	sep := "·"
	if !color {
		sep = "-"
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s %s (%d/%d)", verb, tool, pos, total)
	if next != "" {
		fmt.Fprintf(&b, " %s next: %s", sep, next)
	}
	if color {
		return ansiBoldCyan + b.String() + ansiReset
	}
	return b.String()
}
