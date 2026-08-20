package main

import (
	"bytes"
	"io"
	"testing"
)

// forceRegionTTY swaps the region seams so a bytes.Buffer drives the enabled
// branch with a pinned geometry, and restores both after the test (mirrors
// forceProgressTTY). Rows/cols are the injected terminal size.
func forceRegionTTY(t *testing.T, cols, rows int) {
	t.Helper()
	prevTTY := regionWriterIsTTY
	prevSize := terminalSize
	t.Cleanup(func() {
		regionWriterIsTTY = prevTTY
		terminalSize = prevSize
	})
	regionWriterIsTTY = func(io.Writer) bool { return true }
	terminalSize = func(io.Writer) (int, int) { return cols, rows }
}

// forceRegionResize re-points the terminalSize seam mid-test (the SIGWINCH
// simulation — the next apply reads the new geometry).
func forceRegionResize(cols, rows int) {
	terminalSize = func(io.Writer) (int, int) { return cols, rows }
}

func TestStatusRegion_StartHeaderStopSequences(t *testing.T) {
	forceRegionTTY(t, 80, 24)

	var buf bytes.Buffer
	r := newStatusRegion(&buf)
	r.start()
	wantStart := "\n" + // the blank line the header consumes
		"\x1b[2;24r" + // margins: header on row 1, scroll region 2–24
		"\x1b[2;1H" + // park cursor at the top of the region
		"\x1b[H" + "\x1b[2K" + "\x1b[2;1H" // initial (empty) header paint
	if got := buf.String(); got != wantStart {
		t.Fatalf("start() = %q, want %q", got, wantStart)
	}

	buf.Reset()
	r.setHeader("Installing hop (2/7)")
	wantHeader := "\x1b[H" + "\x1b[2K" + "Installing hop (2/7)" + "\x1b[2;1H"
	if got := buf.String(); got != wantHeader {
		t.Fatalf("setHeader() = %q, want %q", got, wantHeader)
	}

	buf.Reset()
	r.stop()
	wantStop := "\x1b[r" + "\x1b[H" + "\x1b[2K" + "\x1b[H"
	if got := buf.String(); got != wantStop {
		t.Fatalf("stop() = %q, want %q", got, wantStop)
	}
}

func TestStatusRegion_ResizeReapplies(t *testing.T) {
	forceRegionTTY(t, 80, 24)

	var buf bytes.Buffer
	r := newStatusRegion(&buf)
	r.start()
	r.setHeader("Updating run-kit (1/3)")
	buf.Reset()

	// SIGWINCH path: the watcher is asynchronous, so exercise the resize body
	// directly — same code the signal branch runs (mutex-locked, as the
	// watcher does).
	forceRegionResize(120, 40)
	if cols, rows := terminalSize(r.w); rows >= 2 && cols >= 1 {
		r.mu.Lock()
		r.cols = cols
		r.rows = rows
		r.applyLocked()
		r.mu.Unlock()
	}
	want := "\x1b[2;40r" + "\x1b[2;1H" + // margins re-applied for the new size
		"\x1b[H" + "\x1b[2K" + "Updating run-kit (1/3)" + "\x1b[2;1H"
	if got := buf.String(); got != want {
		t.Fatalf("resize re-apply = %q, want %q", got, want)
	}
	r.stop()
}

func TestStatusRegion_DisabledNoOp(t *testing.T) {
	// No seam override: a bytes.Buffer is not a real TTY, so the default gate
	// disables the region and every method must write zero bytes.
	var buf bytes.Buffer
	r := newStatusRegion(&buf)
	if r.enabled {
		t.Fatal("region should be disabled for a non-TTY writer")
	}
	r.start()
	r.setHeader("Installing hop (2/7)")
	r.stop()
	if buf.Len() != 0 {
		t.Fatalf("disabled region emitted %q, want zero bytes", buf.String())
	}
}

func TestStatusRegion_DisabledOnDegenerateGeometry(t *testing.T) {
	// A tty reporting <2 rows cannot host a header plus a scroll line.
	forceRegionTTY(t, 80, 1)
	var buf bytes.Buffer
	r := newStatusRegion(&buf)
	if r.enabled {
		t.Fatal("region should be disabled when the terminal has <2 rows")
	}
	r.start()
	r.setHeader("x")
	r.stop()
	if buf.Len() != 0 {
		t.Fatalf("disabled region emitted %q, want zero bytes", buf.String())
	}
}

func TestStatusRegion_StopIdempotent(t *testing.T) {
	forceRegionTTY(t, 80, 24)
	var buf bytes.Buffer
	r := newStatusRegion(&buf)
	r.start()
	r.stop()
	buf.Reset()
	r.stop() // second stop: restores again, must not deadlock on closed channels
	if got, want := buf.String(), "\x1b[r"+"\x1b[H"+"\x1b[2K"+"\x1b[H"; got != want {
		t.Fatalf("second stop() = %q, want %q", got, want)
	}
}

func TestStatusHeaderText_Forms(t *testing.T) {
	cases := []struct {
		name       string
		verb, tool string
		pos, total int
		next       string
		color      bool
		want       string
	}{
		{"plain with next", "Installing", "rk-desktop", 2, 3, "fab-kit", false, "Installing rk-desktop (2/3) - next: fab-kit"},
		{"plain final tool", "Installing", "fab-kit", 3, 3, "", false, "Installing fab-kit (3/3)"},
		{"plain updating", "Updating", "hop", 1, 7, "tu", false, "Updating hop (1/7) - next: tu"},
		{"color with next", "Updating", "run-kit", 2, 7, "fab-kit", true, ansiBoldCyan + "Updating run-kit (2/7) · next: fab-kit" + ansiReset},
		{"color final tool", "Installing", "hop", 7, 7, "", true, ansiBoldCyan + "Installing hop (7/7)" + ansiReset},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := statusHeaderText(tc.verb, tc.tool, tc.pos, tc.total, tc.next, tc.color); got != tc.want {
				t.Fatalf("statusHeaderText() = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestStatusRegion_HeaderTruncatedToWidth(t *testing.T) {
	forceRegionTTY(t, 10, 24)
	var buf bytes.Buffer
	r := newStatusRegion(&buf)
	r.start()
	buf.Reset()
	r.setHeader("this header is far too long for ten columns")
	want := "\x1b[H" + "\x1b[2K" + "this heade" + "\x1b[2;1H"
	if got := buf.String(); got != want {
		t.Fatalf("paintHeader() = %q, want %q (truncated to width)", got, want)
	}
	r.stop()
}
