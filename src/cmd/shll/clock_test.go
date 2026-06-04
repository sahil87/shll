package main

import (
	"testing"
	"time"
)

// installFakeClock swaps the package-level nowFunc for a deterministic clock that
// returns the provided times in sequence (the final value repeats for any extra
// calls), and restores the prior nowFunc after the test. It mirrors
// installFakeRunner (update_test.go) so duration-bearing golden strings stay
// exact. With no times provided it pins the clock to a fixed instant.
func installFakeClock(t *testing.T, times ...time.Time) {
	t.Helper()
	prev := nowFunc
	t.Cleanup(func() { nowFunc = prev })
	if len(times) == 0 {
		fixed := time.Unix(0, 0)
		nowFunc = func() time.Time { return fixed }
		return
	}
	i := 0
	nowFunc = func() time.Time {
		tm := times[i]
		if i < len(times)-1 {
			i++
		}
		return tm
	}
}

func TestInstallFakeClock_Sequences(t *testing.T) {
	t0 := time.Unix(1000, 0)
	t1 := t0.Add(72 * time.Second)
	installFakeClock(t, t0, t1)

	if got := nowFunc(); !got.Equal(t0) {
		t.Fatalf("first nowFunc() = %v, want %v", got, t0)
	}
	if got := nowFunc(); !got.Equal(t1) {
		t.Fatalf("second nowFunc() = %v, want %v", got, t1)
	}
	// The final value repeats for any extra calls.
	if got := nowFunc(); !got.Equal(t1) {
		t.Fatalf("third nowFunc() = %v, want %v (last value repeats)", got, t1)
	}
}
