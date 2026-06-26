package main

import (
	"context"
	"encoding/json"
	"errors"
	"strings"

	"github.com/sahil87/shll/internal/proc"
)

// brewBinary is the Homebrew CLI name. Named constant so callers do not open-code it.
const brewBinary = "brew"

// brewMissingHint is the exact stderr line printed by `shll update` when the
// brew binary is not on PATH. Matches the original spec's required text verbatim
// (260508-kvan scenario asserts this string literally — do not edit without
// also updating that scenario).
const brewMissingHint = "shll update requires Homebrew. Install from https://brew.sh"

// installBrewMissingHint is the install-command counterpart to brewMissingHint.
// `shll install` uses an install-specific message so the error tells the user
// which command they ran; the update spec's verbatim assertion is preserved by
// keeping `brewMissingHint` separate.
const installBrewMissingHint = "shll install requires Homebrew. Install from https://brew.sh"

// shllFormula is the brew formula for shll itself. Used by `shll update` to
// self-upgrade alongside the roster (shll is not in Roster — Roster is the
// sub-tool list per Constitution III).
const shllFormula = formulaPrefix + "shll"

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

// brewTrustAvailable reports whether this Homebrew supports the `trust`
// subcommand (it is newer; older brews lack it). It capability-probes via
// `brew trust --help`, mirroring the read-only `<tool> update --help` substring
// probe in update.go — never a version-floor check (the probe is the contract).
//
//   - brew absent (proc.ErrNotFound) → false (the caller degrades).
//   - `trust` unrecognized → brew exits non-zero, so proc.Run returns a non-nil
//     error → false.
//   - `trust` recognized → exit 0, nil error → true.
//
// A captured non-ErrNotFound error means the subcommand is unknown on this brew,
// so any error degrades to "unavailable". Routed through internal/proc per
// Constitution I. Reused as the capability gate by both `shll install`'s
// per-formula trust step and `shll doctor`'s read-only trust sub-check.
func brewTrustAvailable(ctx context.Context) bool {
	out, err := proc.Run(ctx, brewBinary, "trust", "--help")
	if err != nil {
		return false
	}
	// Defensive: a brew that prints help-style output but does not actually carry
	// `trust` would be a contradiction (it exited 0 on `trust --help`), so the
	// exit-0 signal alone is authoritative. The substring guard below is a belt-
	// and-suspenders check that the help text concerns trust, costing nothing.
	return strings.Contains(string(out), "trust")
}

// brewTrustFormula runs the per-formula trust ceremony — `brew trust --formula
// sahil87/tap/<formula>` — and returns its exit code and any transport error.
// The granularity is per-formula (NOT whole-tap): Homebrew recommends trusting
// the specific formula you need for third-party taps, and shll knows its exact
// roster, so it trusts only what it actually manages. Foregrounded so the user
// sees brew's own "Trusted formula:" / "Already trusted formula:" output.
//
// `brew trust` is idempotent (re-running an already-trusted formula exits 0 with
// an "Already trusted formula:" line), so callers invoke this unconditionally
// before each install — no pre-check for existing trust is needed. Routed through
// internal/proc per Constitution I.
func brewTrustFormula(ctx context.Context, formula string) (int, error) {
	return proc.RunForeground(ctx, brewBinary, "trust", "--formula", formula)
}

// brewTrustList queries Homebrew's current trust state via `brew trust --json=v1`
// and returns the trusted tap names and trusted (fully-qualified) formula names,
// plus ok=false on any failure. It is the read-only primitive `shll doctor` uses
// to determine whether an installed roster formula is trusted — Constitution III:
// shll NEVER reads ~/.homebrew/trust.json directly; it asks brew via its public
// JSON contract.
//
// The brew JSON shape (verified on Homebrew 6.0.4) is:
//
//	{"taps": [...], "formulae": [...], "casks": [...], "commands": [...]}
//
// A formula counts as trusted when its qualified name appears in `formulae` OR
// its tap appears in `taps` (tap- and formula-level trust both count). Callers
// derive that membership; this helper only fetches and decodes.
//
// Degradation (Constitution V): on brew absent, an older brew lacking `trust`, a
// non-zero exit, or a JSON decode failure, it returns ok=false — the caller then
// skips the trust check silently rather than WARNing on a state it cannot
// determine. The decode uses encoding/json (never a regex over brew output —
// code-quality.md anti-pattern). Routed through internal/proc per Constitution I.
func brewTrustList(ctx context.Context) (taps, formulae []string, ok bool) {
	out, err := proc.Run(ctx, brewBinary, "trust", "--json=v1")
	if err != nil {
		return nil, nil, false
	}
	var parsed struct {
		Taps     []string `json:"taps"`
		Formulae []string `json:"formulae"`
	}
	if jsonErr := json.Unmarshal(out, &parsed); jsonErr != nil {
		return nil, nil, false
	}
	return parsed.Taps, parsed.Formulae, true
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
