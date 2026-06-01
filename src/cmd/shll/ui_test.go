package main

import (
	"bytes"
	"strings"
	"testing"
)

func TestColorEnabled_NonFileWriterIsPlain(t *testing.T) {
	// A bytes.Buffer is not an *os.File, so it can never be a terminal — the
	// helper must select the plain branch regardless of NO_COLOR. This is the
	// seam that lets tests assert the ASCII forms without faking a TTY.
	t.Setenv(noColorEnv, "") // ensure NO_COLOR is unset for this sub-case
	if colorEnabled(&bytes.Buffer{}) {
		t.Fatal("colorEnabled(bytes.Buffer) = true, want false (not a terminal)")
	}
}

func TestColorEnabled_NoColorForcesPlain(t *testing.T) {
	// NO_COLOR set (to any value, even empty) disables color unconditionally,
	// honoring the no-color.org convention.
	t.Setenv(noColorEnv, "1")
	if colorEnabled(&bytes.Buffer{}) {
		t.Fatal("colorEnabled with NO_COLOR set = true, want false")
	}
}

func TestPrintToolHeader_PlainForm(t *testing.T) {
	var buf bytes.Buffer
	printToolHeader(&buf, "hop", false)
	if got, want := buf.String(), "==> hop\n"; got != want {
		t.Fatalf("plain header = %q, want %q", got, want)
	}
	// Plain form is pure ASCII — no ANSI escape and no Unicode arrow.
	if strings.Contains(buf.String(), "\033[") || strings.Contains(buf.String(), "▸") {
		t.Fatalf("plain header %q must contain no ANSI escape and no ▸ glyph", buf.String())
	}
}

func TestPrintToolHeader_ColorForm(t *testing.T) {
	var buf bytes.Buffer
	printToolHeader(&buf, "hop", true)
	got := buf.String()
	// Colored form uses the bold-cyan arrow glyph + bold name, terminated by a
	// reset, via the named SGR constants.
	if !strings.Contains(got, "▸") {
		t.Fatalf("color header %q must contain the ▸ glyph", got)
	}
	if !strings.Contains(got, ansiBoldCyan) || !strings.Contains(got, ansiBold) || !strings.Contains(got, ansiReset) {
		t.Fatalf("color header %q must use the named SGR constants", got)
	}
	if !strings.Contains(got, "hop") {
		t.Fatalf("color header %q must contain the tool name", got)
	}
}

func TestToolComment_AlwaysPlainASCIIShellComment(t *testing.T) {
	got := toolComment("tu")
	if want := "# ── tu ──"; got != want {
		t.Fatalf("toolComment = %q, want %q", got, want)
	}
	// Eval-safety: it is a shell comment (leading #) and carries no ANSI escapes,
	// regardless of color/TTY state (the function takes no color parameter).
	if !strings.HasPrefix(got, "#") {
		t.Fatalf("toolComment %q must be a shell comment (leading #)", got)
	}
	if strings.Contains(got, "\033[") {
		t.Fatalf("toolComment %q must contain no ANSI escape (eval-safety)", got)
	}
}

func TestPrintSummaryTail_AllSucceededPlain(t *testing.T) {
	var buf bytes.Buffer
	printSummaryTail(&buf, 6, 6, false)
	if got, want := buf.String(), "Done — 6 of 6 tools succeeded.\n"; got != want {
		t.Fatalf("plain all-success tail = %q, want %q", got, want)
	}
	if strings.Contains(buf.String(), "\033[") || strings.Contains(buf.String(), "✓") {
		t.Fatalf("plain tail %q must contain no ANSI escape and no ✓ glyph", buf.String())
	}
}

func TestPrintSummaryTail_AllSucceededColor(t *testing.T) {
	var buf bytes.Buffer
	printSummaryTail(&buf, 6, 6, true)
	got := buf.String()
	if !strings.Contains(got, "✓") || !strings.Contains(got, ansiGreen) || !strings.Contains(got, ansiReset) {
		t.Fatalf("color all-success tail %q must prefix a green ✓ via named SGR constants", got)
	}
	if !strings.Contains(got, "Done — 6 of 6 tools succeeded.") {
		t.Fatalf("color tail %q must contain the success wording", got)
	}
}

func TestPrintSummaryTail_PartialFailure(t *testing.T) {
	var buf bytes.Buffer
	printSummaryTail(&buf, 5, 6, false)
	if got, want := buf.String(), "5 succeeded, 1 failed — see above.\n"; got != want {
		t.Fatalf("partial-failure tail = %q, want %q", got, want)
	}
	// Honesty constraint: never claims updated / up-to-date.
	if strings.Contains(buf.String(), "updated") || strings.Contains(buf.String(), "up-to-date") {
		t.Fatalf("tail %q must not claim updated/up-to-date", buf.String())
	}
}
