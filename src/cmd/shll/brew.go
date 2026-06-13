package main

import (
	"context"
	"errors"
	"strings"

	"github.com/sahil87/shll/internal/proc"
)

// brewBinary is the Homebrew CLI name. Named constant so callers do not open-code it.
const brewBinary = "brew"

// noRequireTapTrustEnv is the environment entry shll injects into its brew
// install/upgrade/update subprocesses on Linux as the temporary sandbox-trust
// workaround. Named constant per code-quality.md (no magic strings). See brewEnv.
const noRequireTapTrustEnv = "HOMEBREW_NO_REQUIRE_TAP_TRUST=1"

// brewEnv returns the extra environment entries shll injects into its brew
// subprocesses. It is the SINGLE source of truth for the temporary Homebrew
// Linux-sandbox workaround.
//
// TEMPORARY WORKAROUND (backlog [38a6]; remove per backlog [tkch]):
// Homebrew 6.0's Linux bubblewrap sandbox masks ~/.homebrew (HOMEBREW_USER_CONFIG_HOME)
// via deny_read_home, so the sandboxed build.rb cannot read trust.json and wrongly
// raises UntrustedTapError when HOMEBREW_REQUIRE_TAP_TRUST=1 is set (the error is also
// swallowed — raised before build.rb connects its error pipe — surfacing as an opaque
// "bwrap exited with 1"). We set HOMEBREW_NO_REQUIRE_TAP_TRUST=1 to skip ONLY the broken
// in-sandbox trust re-check; the sandbox itself stays active and trust was already
// verified out-of-sandbox. Gated to Linux: the bug is bubblewrap-specific and macOS must
// keep enforcing trust. DELETE this (and its wiring/tests) once the upstream fix lands —
// see backlog [tkch] for the verification recipe.
func brewEnv() []string {
	if osGoos != "linux" {
		return nil
	}
	return []string{noRequireTapTrustEnv}
}

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
// Constitution I.
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

// brewTrustTap runs the trust ceremony — `brew trust --tap sahil87/tap` — and
// returns its exit code and any transport error. The tap argument comes from the
// tapName constant (NOT a formula reference). Foregrounded so the user sees
// brew's own "Trusted tap" / "Already trusted tap" output.
//
// Callers invoke this unconditionally during --trust-tap: `brew trust`/`untrust`
// are idempotent (verified on brew 5.1.14 — re-run exits 0), so no pre-check for
// existing trust is needed. Routed through internal/proc per Constitution I.
func brewTrustTap(ctx context.Context) (int, error) {
	return proc.RunForeground(ctx, brewBinary, "trust", "--tap", tapName)
}

// trustHatchHint names the lighter env-var escape hatches the user can set
// themselves when genuine trust is unavailable. Used verbatim in the degradation
// diagnostic so the user has an actionable alternative.
const trustHatchHint = "set HOMEBREW_NO_REQUIRE_TAP_TRUST=1 or HOMEBREW_NO_ENV_HINTS=1 to silence the warning instead"

// ensureTapTrust performs the full genuine-trust ceremony for --trust-tap and
// reports whether the policy line (export HOMEBREW_REQUIRE_TAP_TRUST=1) should be
// written. It is the single proc-touching seam the file-I/O-only shell_setup.go
// calls — keeping every subprocess invocation (capability probe + ceremony) in
// brew.go, which legitimately imports internal/proc (Constitution I; the
// TestNoProcImports guard pins shell_setup.go to file I/O only).
//
// Degradation policy (Constitution V): the policy line is written ONLY when brew
// is present, `brew trust` is available, AND the ceremony exits 0. In every other
// case — brew absent, `trust` unrecognized on an older brew, or a non-zero/error
// ceremony exit — writeExport is false and diag explains why, naming the lighter
// env-var escape hatches. shell_setup.go still writes the eval line regardless,
// so the user keeps shell integration; only the trust half is skipped.
//
// Returns (writeExport, diag):
//   - writeExport true, diag ""   → ceremony succeeded; caller includes export line.
//   - writeExport false, diag set → degraded; caller skips export line, prints diag.
func ensureTapTrust(ctx context.Context) (writeExport bool, diag string) {
	if !hasBrew(ctx) {
		return false, "shll shell-setup: Homebrew is not installed, so the sahil87 tap cannot be trusted. " +
			"Skipped the trust policy line (writing it without a trust record would block the tap). " +
			"Install Homebrew from https://brew.sh, then re-run `shll shell-setup --trust-tap`; or " + trustHatchHint + "."
	}
	if !brewTrustAvailable(ctx) {
		return false, "shll shell-setup: this Homebrew does not support `brew trust` (it requires a newer Homebrew). " +
			"Skipped the trust policy line (writing it without a trust record would block the tap). " +
			"Upgrade Homebrew, then re-run `shll shell-setup --trust-tap`; or " + trustHatchHint + "."
	}
	code, err := brewTrustTap(ctx)
	if err != nil || code != 0 {
		return false, "shll shell-setup: `brew trust --tap " + tapName + "` did not succeed, " +
			"so the trust policy line was skipped (writing it without a trust record would block the tap). " +
			"Re-run `shll shell-setup --trust-tap` once brew can reach the tap; or " + trustHatchHint + "."
	}
	return true, ""
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
