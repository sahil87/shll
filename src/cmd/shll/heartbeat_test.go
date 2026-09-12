package main

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"

	"github.com/sahil87/shll/internal/proc"
)

// TestSilenceWatch_DueDoublesThenResetsOnOutput pins the back-off contract:
// the first heartbeat needs the initial silence, each later one needs double
// the previous, and any child output resets both the clock and the threshold.
func TestSilenceWatch_DueDoublesThenResetsOnOutput(t *testing.T) {
	t0 := time.Unix(0, 0)
	w := newSilenceWatch(t0, 30*time.Second)

	if silent, ok := w.due(t0.Add(29 * time.Second)); ok {
		t.Fatalf("due at 29s = true (silent %s), want false before the initial threshold", silent)
	}
	silent, ok := w.due(t0.Add(30 * time.Second))
	if !ok || silent != 30*time.Second {
		t.Fatalf("due at 30s = (%s, %v), want (30s, true)", silent, ok)
	}
	// Threshold doubled to 60s: 45s of silence is not due; 60s is.
	if _, ok := w.due(t0.Add(45 * time.Second)); ok {
		t.Fatal("due at 45s = true, want false after the threshold doubled to 60s")
	}
	silent, ok = w.due(t0.Add(60 * time.Second))
	if !ok || silent != 60*time.Second {
		t.Fatalf("due at 60s = (%s, %v), want (60s, true)", silent, ok)
	}
	// Child output at 70s resets the clock AND the threshold back to 30s.
	w.touch(t0.Add(70 * time.Second))
	if _, ok := w.due(t0.Add(99 * time.Second)); ok {
		t.Fatal("due at 99s = true, want false (29s after output, threshold reset to 30s)")
	}
	silent, ok = w.due(t0.Add(100 * time.Second))
	if !ok || silent != 30*time.Second {
		t.Fatalf("due at 100s = (%s, %v), want (30s, true) after reset", silent, ok)
	}
}

// TestActivityWriter_ForwardsBytesAndTouches pins that the tee wrapper passes
// bytes through unchanged and counts as child output for the watch.
func TestActivityWriter_ForwardsBytesAndTouches(t *testing.T) {
	// Start the watch far in the past so a heartbeat would be due unless a
	// write touched it.
	w := newSilenceWatch(time.Now().Add(-time.Hour), time.Minute)
	var sink bytes.Buffer
	aw := &activityWriter{w: &sink, watch: w}

	n, err := aw.Write([]byte("==> Fetching shll\n"))
	if err != nil || n != len("==> Fetching shll\n") {
		t.Fatalf("Write = (%d, %v), want full length and nil", n, err)
	}
	if got := sink.String(); got != "==> Fetching shll\n" {
		t.Fatalf("sink = %q, want the bytes forwarded verbatim", got)
	}
	if silent, ok := w.due(time.Now()); ok {
		t.Fatalf("due right after a write = true (silent %s), want false", silent)
	}
}

// TestRunStreamedChild_HeartbeatOnSilentChild drives real silence with
// millisecond timings: a child that produces nothing for 60ms against a 10ms
// initial threshold yields at least one still-waiting line on stderr, naming
// the child's argv, and nothing on stdout. Exit-code passthrough is unchanged.
func TestRunStreamedChild_HeartbeatOnSilentChild(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		time.Sleep(60 * time.Millisecond)
		return proc.Result{ExitCode: 0}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	code, err := runStreamedChildWithHeartbeat(context.Background(), &stdout, &stderr,
		10*time.Millisecond, 2*time.Millisecond, brewBinary, "upgrade", shllFormula)
	if err != nil || code != 0 {
		t.Fatalf("runStreamedChildWithHeartbeat = (%d, %v), want (0, nil)", code, err)
	}
	want := "shll: still waiting on 'brew upgrade " + shllFormula + "' (no output for "
	if !strings.Contains(stderr.String(), want) {
		t.Fatalf("stderr = %q, want a heartbeat line containing %q", stderr.String(), want)
	}
	if !strings.Contains(stderr.String(), "a slow or stalled download is the usual cause") {
		t.Fatalf("stderr = %q, want the usual-cause hint", stderr.String())
	}
	if stdout.Len() != 0 {
		t.Fatalf("stdout = %q, want empty — the heartbeat is diagnostics, never data", stdout.String())
	}
	// The child rode the streamed-tail transport with the tee writers wrapped.
	calls := f.recordedCalls()
	if len(calls) != 1 || calls[0].Transport != proc.TransportStreamTail {
		t.Fatalf("recorded calls = %+v, want exactly one TransportStreamTail request", calls)
	}
}

// TestRunStreamedChild_ChildOutputSuppressesHeartbeat pins the reset: a child
// that keeps writing (every 10ms for 150ms) never crosses a 200ms threshold,
// so no heartbeat is printed, and its output reaches the caller's stdout
// verbatim through the wrapper.
func TestRunStreamedChild_ChildOutputSuppressesHeartbeat(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		for i := 0; i < 15; i++ {
			time.Sleep(10 * time.Millisecond)
			_, _ = req.Stdout.Write([]byte("line\n"))
		}
		return proc.Result{ExitCode: 0}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	code, err := runStreamedChildWithHeartbeat(context.Background(), &stdout, &stderr,
		200*time.Millisecond, 5*time.Millisecond, "hop", "update")
	if err != nil || code != 0 {
		t.Fatalf("runStreamedChildWithHeartbeat = (%d, %v), want (0, nil)", code, err)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want no heartbeat while the child keeps producing output", stderr.String())
	}
	if got := strings.Count(stdout.String(), "line\n"); got != 15 {
		t.Fatalf("stdout has %d child lines, want 15 forwarded verbatim", got)
	}
}

// TestRunStreamedChild_FastChildNoHeartbeat pins that the production wiring
// (runStreamedChild with the 30s threshold) stays byte-silent for a child that
// returns immediately — the existing install/update goldens depend on it — and
// still passes a non-zero exit code through as (code, nil).
func TestRunStreamedChild_FastChildNoHeartbeat(t *testing.T) {
	f := &fakeRunner{respond: func(req proc.Request) proc.Result {
		return proc.Result{ExitCode: 3}
	}}
	installFakeRunner(t, f)

	var stdout, stderr bytes.Buffer
	code, err := runStreamedChild(context.Background(), &stdout, &stderr, brewBinary, "update", "--quiet")
	if err != nil || code != 3 {
		t.Fatalf("runStreamedChild = (%d, %v), want (3, nil) — exit-code passthrough unchanged", code, err)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("stdout = %q, stderr = %q, want both empty for an instant child", stdout.String(), stderr.String())
	}
}
