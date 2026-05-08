package main

import (
	"context"
	"errors"

	"github.com/sahil87/shll/internal/proc"
)

// brewBinary is the Homebrew CLI name. Named constant so callers do not open-code it.
const brewBinary = "brew"

// brewMissingHint is the exact stderr line printed when the brew binary is not
// on PATH. Matches the spec's required text verbatim.
const brewMissingHint = "shll update requires Homebrew. Install from https://brew.sh"

// hasBrew reports whether the brew binary is on PATH. It does this by invoking
// `brew --version` via proc.Run (so tests can swap behavior) and checking for
// proc.ErrNotFound. Per Constitution I, no manual PATH parsing — let exec do it.
func hasBrew(ctx context.Context) bool {
	_, err := proc.Run(ctx, brewBinary, "--version")
	if errors.Is(err, proc.ErrNotFound) {
		return false
	}
	// Any other error (e.g. brew exits non-zero for some reason) still implies
	// brew is on PATH — graceful degradation: only ErrNotFound is the "missing"
	// signal.
	return true
}

// isInstalled reports whether the named formula is installed. Detection is via
// `brew list --formula --versions <formula>` exit code (Design Decision #2 —
// no regex over plain `brew list` output, no symlink-target inspection).
//
// `brew list --versions <formula>` exits 0 with the version string on stdout
// when installed, and exits 1 with empty stdout when not. We treat any non-nil
// captured-error as "not installed" — this covers both the exit-1 path and the
// rare ErrNotFound path (brew itself missing — caller should have checked).
func isInstalled(ctx context.Context, formula string) bool {
	_, err := proc.Run(ctx, brewBinary, "list", "--formula", "--versions", formula)
	return err == nil
}
