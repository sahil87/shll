package main

import (
	"context"
	"fmt"
	"io"
	"sync"
	"time"

	"github.com/sahil87/shll/internal/proc"
)

// childSilenceHeartbeat is the initial stretch of child silence — not one byte
// on either stream — after which runStreamedChild prints a still-waiting line
// for a write-phase child (brew update/upgrade/install/link/trust, a delegated
// `<tool> update`). Sized for the live case that motivated it (260912): a
// `brew upgrade` whose bottle download crawled for minutes on a degraded route
// to GitHub's release-asset host. Homebrew 6's download queue renders its
// progress only on a TTY, and shll's tee hands brew a pipe, so from `==>
// Fetching` until the download lands the terminal shows nothing — and shll
// deliberately imposes no deadline on brew (the update standard's brew-safety
// clause), so nothing else ever breaks the silence. A few seconds of quiet is
// normal while brew resolves a formula; tens of seconds with no output is where
// a user starts to suspect a hang. Named per code-quality.md (no magic numbers).
const childSilenceHeartbeat = 30 * time.Second

// heartbeatPollInterval is how often the watcher goroutine samples the silence
// clock. 1s bounds heartbeat lateness to about a second past the threshold
// while costing nothing measurable. Named per code-quality.md.
const heartbeatPollInterval = time.Second

// childHeartbeatFmt is the still-waiting line runStreamedChild writes to stderr
// (diagnostics, not data — principle №2) when a write-phase child has been
// silent for the current threshold. %[1]s is the child's argv rendered by
// argvString (`brew upgrade sahil87/tap/shll`), %[2]s the formatted silence so
// far (`30s`, `1m0s`). Plain ASCII on purpose: the line goes to stderr on every
// stream kind, so it carries none of the glyphs ui.go degrades for non-TTY
// output. It names the usual cause and states that no deadline is imposed, so
// the reader knows the wait is brew's and will end when brew ends. Named per
// code-quality.md (no magic strings).
const childHeartbeatFmt = "shll: still waiting on '%s' (no output for %s; a slow or stalled download is the usual cause, and no deadline is imposed)"

// silenceWatch tracks the most recent child output time and decides when a
// still-waiting heartbeat is due. The required silence doubles after every
// heartbeat (30s, 1m, 2m, 4m, …) so a long stall produces a short back-off
// series rather than a fixed-interval drumbeat (principle №9 — bounded,
// high-signal output); any child output resets both the clock and the
// threshold. Safe for concurrent use: the tee'd writers touch it from the exec
// copy goroutines while the watcher goroutine polls it.
//
// It runs on the caller-supplied wall clock, NOT the nowFunc seam (clock.go):
// that seam exists so the summary tail's duration golden can consume a scripted
// sequence of times, and a watcher polling it would silently eat those values.
type silenceWatch struct {
	mu         sync.Mutex
	initial    time.Duration
	threshold  time.Duration
	lastOutput time.Time
}

// newSilenceWatch starts a watch at now with the given initial silence
// threshold (childSilenceHeartbeat in production; tests pass milliseconds).
func newSilenceWatch(now time.Time, initial time.Duration) *silenceWatch {
	return &silenceWatch{initial: initial, threshold: initial, lastOutput: now}
}

// touch records child output at now: the silence clock restarts and the
// back-off threshold returns to its initial value.
func (s *silenceWatch) touch(now time.Time) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.lastOutput = now
	s.threshold = s.initial
}

// due reports how long the child has been silent as of now and whether a
// heartbeat is due (silence has reached the current threshold). When it is,
// the threshold doubles, so the next heartbeat needs twice the silence.
func (s *silenceWatch) due(now time.Time) (silent time.Duration, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	silent = now.Sub(s.lastOutput)
	if silent < s.threshold {
		return silent, false
	}
	s.threshold *= 2
	return silent, true
}

// activityWriter tees a child's stream to w and stamps the silence watch on
// every write. It never alters, buffers, or withholds bytes — the live-tee
// contract of the streamed-tail transport is untouched.
type activityWriter struct {
	w     io.Writer
	watch *silenceWatch
}

// Write forwards p to the wrapped writer after touching the watch.
func (a *activityWriter) Write(p []byte) (int, error) {
	a.watch.touch(time.Now())
	return a.w.Write(p)
}

// runStreamedChildWithHeartbeat is runStreamedChild with the heartbeat timings
// as parameters, so tests can drive real silence with millisecond values
// instead of waiting on the production constants. It wraps both tee writers in
// an activityWriter, runs the child through proc.RunStreamedTail, and polls the
// watch from one goroutine that prints childHeartbeatFmt to stderr whenever a
// heartbeat is due. The watcher is stopped and joined before returning, so no
// heartbeat can land after the child's exit has been reported.
//
// The heartbeat writes to the caller's stderr directly (not through the
// activityWriter and not through proc), so it neither resets the silence clock
// nor lands in the transport's bounded tail ring — the tail stays pure child
// output. Exit-code semantics pass through unchanged (non-zero exit via code,
// err only for exec failures).
func runStreamedChildWithHeartbeat(ctx context.Context, stdout, stderr io.Writer, silence, poll time.Duration, argv ...string) (int, error) {
	watch := newSilenceWatch(time.Now(), silence)
	stop := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		heartbeatLoop(watch, stderr, argvString(argv...), poll, stop)
	}()
	code, _, err := proc.RunStreamedTail(ctx,
		&activityWriter{w: stdout, watch: watch},
		&activityWriter{w: stderr, watch: watch},
		argv[0], argv[1:]...)
	close(stop)
	wg.Wait()
	return code, err
}

// heartbeatLoop polls watch every poll interval until stop is closed, writing
// one still-waiting line to w each time the watch reports a heartbeat due. cmd
// is the child's rendered argv for the line.
func heartbeatLoop(watch *silenceWatch, w io.Writer, cmd string, poll time.Duration, stop <-chan struct{}) {
	ticker := time.NewTicker(poll)
	defer ticker.Stop()
	for {
		select {
		case <-stop:
			return
		case now := <-ticker.C:
			if silent, ok := watch.due(now); ok {
				fmt.Fprintf(w, childHeartbeatFmt+"\n", cmd, formatDuration(silent))
			}
		}
	}
}
