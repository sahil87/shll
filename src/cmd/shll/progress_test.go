package main

import (
	"bytes"
	"io"
	"testing"
)

// forceProgressTTY swaps the progressWriterIsTTY seam to always report true and
// restores the default after the test — the deterministic way to exercise the
// enabled branch (a bytes.Buffer is never a real TTY).
func forceProgressTTY(t *testing.T) {
	t.Helper()
	prev := progressWriterIsTTY
	t.Cleanup(func() { progressWriterIsTTY = prev })
	progressWriterIsTTY = func(io.Writer) bool { return true }
}

func TestProgressReporter_SequenceForms(t *testing.T) {
	forceProgressTTY(t)

	cases := []struct {
		name string
		call func(p *progressReporter)
		want string
	}{
		{"set", func(p *progressReporter) { p.set(50) }, "\x1b]9;4;1;50\x07"},
		{"indeterminate", func(p *progressReporter) { p.indeterminate() }, "\x1b]9;4;3;0\x07"},
		{"errorState", func(p *progressReporter) { p.errorState(75) }, "\x1b]9;4;2;75\x07"},
		{"remove", func(p *progressReporter) { p.remove() }, "\x1b]9;4;0;0\x07"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			var buf bytes.Buffer
			p := newProgressReporter(&buf, envFunc(nil))
			tc.call(p)
			if got := buf.String(); got != tc.want {
				t.Fatalf("emitted %q, want %q", got, tc.want)
			}
		})
	}
}

func TestProgressReporter_DisabledEmitsNothing(t *testing.T) {
	// No seam override: a bytes.Buffer is not an *os.File, so the default
	// gate disables the reporter and every method must write zero bytes.
	var buf bytes.Buffer
	p := newProgressReporter(&buf, envFunc(nil))
	p.set(50)
	p.indeterminate()
	p.errorState(10)
	p.remove()
	if buf.Len() != 0 {
		t.Fatalf("disabled reporter emitted %q, want nothing", buf.String())
	}
}

func TestProgressReporter_IndependentOfNoColor(t *testing.T) {
	// NO_COLOR governs styling, not progress state — with the TTY seam forced
	// true, emission proceeds even when NO_COLOR is set in the process env.
	forceProgressTTY(t)
	t.Setenv(noColorEnv, "1")

	var buf bytes.Buffer
	p := newProgressReporter(&buf, envFunc(nil))
	p.set(25)
	if got, want := buf.String(), "\x1b]9;4;1;25\x07"; got != want {
		t.Fatalf("emitted %q, want %q (NO_COLOR must not gate progress)", got, want)
	}
}

func TestProgressReporter_TmuxPassthroughWrap(t *testing.T) {
	forceProgressTTY(t)

	var buf bytes.Buffer
	p := newProgressReporter(&buf, envFunc(map[string]string{tmuxEnv: "/tmp/tmux-1000/default,123,0"}))
	p.set(50)
	// DCS envelope with every ESC of the inner sequence doubled; BEL untouched.
	want := "\x1bPtmux;\x1b\x1b]9;4;1;50\x07\x1b\\"
	if got := buf.String(); got != want {
		t.Fatalf("emitted %q, want %q", got, want)
	}
}

func TestProgressReporter_NoTmuxNoWrap(t *testing.T) {
	forceProgressTTY(t)

	var buf bytes.Buffer
	p := newProgressReporter(&buf, envFunc(nil))
	p.remove()
	if got, want := buf.String(), "\x1b]9;4;0;0\x07"; got != want {
		t.Fatalf("emitted %q, want %q (no envelope outside tmux)", got, want)
	}
}
