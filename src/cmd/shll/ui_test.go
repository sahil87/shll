package main

import (
	"bytes"
	"strings"
	"testing"
	"time"
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
	printToolHeader(&buf, "hop", 5, 6, false)
	if got, want := buf.String(), "==> [5/6] hop\n"; got != want {
		t.Fatalf("plain header = %q, want %q", got, want)
	}
	// Plain form is pure ASCII — no ANSI escape and no Unicode arrow.
	if strings.Contains(buf.String(), "\033[") || strings.Contains(buf.String(), "▸") {
		t.Fatalf("plain header %q must contain no ANSI escape and no ▸ glyph", buf.String())
	}
}

func TestPrintToolHeader_ColorForm(t *testing.T) {
	var buf bytes.Buffer
	printToolHeader(&buf, "hop", 5, 6, true)
	got := buf.String()
	// Colored form uses the bold-cyan arrow glyph + bold name, terminated by a
	// reset, via the named SGR constants, with the [N/M] progress counter.
	if !strings.Contains(got, "▸") {
		t.Fatalf("color header %q must contain the ▸ glyph", got)
	}
	if !strings.Contains(got, "[5/6]") {
		t.Fatalf("color header %q must contain the [N/M] counter", got)
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

// testDur is the fixed duration the tail tests use; 72s renders as `1m12s` (the
// intake's verbatim example) after rounding to whole seconds.
const testDur = 72 * time.Second

func TestPrintSummaryTail_AllSucceededPlain(t *testing.T) {
	var buf bytes.Buffer
	printSummaryTail(&buf, 6, 6, testDur, false)
	if got, want := buf.String(), "Done — 6 of 6 tools succeeded in 1m12s.\n"; got != want {
		t.Fatalf("plain all-success tail = %q, want %q", got, want)
	}
	if strings.Contains(buf.String(), "\033[") || strings.Contains(buf.String(), "✓") {
		t.Fatalf("plain tail %q must contain no ANSI escape and no ✓ glyph", buf.String())
	}
}

func TestPrintSummaryTail_AllSucceededColor(t *testing.T) {
	var buf bytes.Buffer
	printSummaryTail(&buf, 6, 6, testDur, true)
	got := buf.String()
	if !strings.Contains(got, "✓") || !strings.Contains(got, ansiGreen) || !strings.Contains(got, ansiReset) {
		t.Fatalf("color all-success tail %q must prefix a green ✓ via named SGR constants", got)
	}
	if !strings.Contains(got, "Done — 6 of 6 tools succeeded in 1m12s.") {
		t.Fatalf("color tail %q must contain the success wording with duration", got)
	}
}

func TestPrintSummaryTail_PartialFailure(t *testing.T) {
	var buf bytes.Buffer
	printSummaryTail(&buf, 5, 6, testDur, false)
	// Partial-failure form places the duration BEFORE the em-dash.
	if got, want := buf.String(), "5 succeeded, 1 failed in 1m12s — see above.\n"; got != want {
		t.Fatalf("partial-failure tail = %q, want %q", got, want)
	}
	// Honesty constraint: never claims updated / up-to-date.
	if strings.Contains(buf.String(), "updated") || strings.Contains(buf.String(), "up-to-date") {
		t.Fatalf("tail %q must not claim updated/up-to-date", buf.String())
	}
}

func TestFormatDuration(t *testing.T) {
	cases := []struct {
		in   time.Duration
		want string
	}{
		{72 * time.Second, "1m12s"},
		{5 * time.Second, "5s"},
		{1500 * time.Millisecond, "2s"},     // rounds to whole seconds
		{400 * time.Millisecond, "0s"},      // sub-second rounds to 0s
		{90 * time.Minute, "1h30m0s"},
	}
	for _, c := range cases {
		if got := formatDuration(c.in); got != c.want {
			t.Errorf("formatDuration(%v) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestPrintUpdatePreview_AlignedColumns(t *testing.T) {
	var buf bytes.Buffer
	rows := []previewRow{
		{label: "shll (self)", cmd: "brew upgrade sahil87/tap/shll"},
		{label: "wt", cmd: "wt update --skip-brew-update"},
		{label: "idea", cmd: "idea update"},
	}
	printUpdatePreview(&buf, rows)
	// Longest label is "shll (self)" (11 chars); shorter labels are right-padded to
	// 11 with a 2-space indent and a 2-space gap before the command.
	want := "Would update 3 tools (brew metadata refresh first):\n" +
		"  shll (self)  brew upgrade sahil87/tap/shll\n" +
		"  wt           wt update --skip-brew-update\n" +
		"  idea         idea update\n"
	if got := buf.String(); got != want {
		t.Fatalf("update preview =\n%q\nwant\n%q", got, want)
	}
}

func TestPrintInstallPreview_AlignedColumns(t *testing.T) {
	var buf bytes.Buffer
	rows := []previewRow{
		{label: "idea", cmd: "brew install sahil87/tap/idea"},
		{label: "fab-kit", cmd: "brew install sahil87/tap/fab-kit"},
	}
	printInstallPreview(&buf, rows)
	// Longest label is "fab-kit" (7 chars); "idea" is right-padded to 7. No
	// metadata-refresh annotation in the install header.
	want := "Would install 2 tools:\n" +
		"  idea     brew install sahil87/tap/idea\n" +
		"  fab-kit  brew install sahil87/tap/fab-kit\n"
	if got := buf.String(); got != want {
		t.Fatalf("install preview =\n%q\nwant\n%q", got, want)
	}
	if strings.Contains(buf.String(), "metadata refresh") {
		t.Fatalf("install preview %q must not mention metadata refresh", buf.String())
	}
}
