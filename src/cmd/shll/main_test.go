package main

import (
	"errors"
	"fmt"
	"testing"
)

// TestTranslateExit_Contract pins the toolkit exit-code convention
// (0 success / 1 operational failure / 2 usage error) at the translateExit
// seam. It covers the sentinel paths (errExitCode, errSilent), the cobra
// usage-error prefix classification, and the operational-failure default.
func TestTranslateExit_Contract(t *testing.T) {
	cases := []struct {
		name string
		err  error
		want int
	}{
		{"nil is success", nil, 0},
		{"errSilent is operational failure", errSilent, 1},
		{"plain error is operational failure", errors.New("brew upgrade failed"), 1},
		{"errExitCode carries its code (2)", &errExitCode{code: 2, msg: "bad shell"}, 2},
		{"errExitCode carries its code (1)", &errExitCode{code: 1, msg: "x"}, 1},

		// Cobra arg/command usage errors — classified to exit 2 by prefix.
		{"unknown command", errors.New(`unknown command "bogus" for "shll"`), 2},
		{"unknown flag", errors.New("unknown flag: --bogus"), 2},
		{"unknown shorthand flag", errors.New("unknown shorthand flag: 'x' in -x"), 2},
		{"invalid argument", errors.New(`invalid argument "a" for "shll x"`), 2},
		{"accepts N args", errors.New("accepts at most 1 arg(s), received 3"), 2},
		{"requires N args", errors.New("requires at least 1 arg(s), only received 0"), 2},

		// An operational error whose message merely CONTAINS a usage word (not
		// as a prefix) must stay exit 1 — classification is prefix-anchored.
		{"operational error mentioning 'unknown' mid-message", errors.New("brew reported unknown formula state"), 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := translateExit(tc.err); got != tc.want {
				t.Errorf("translateExit(%v) = %d, want %d", tc.err, got, tc.want)
			}
		})
	}
}

// TestTranslateExit_WrappedErrExitCode confirms errExitCode is matched via
// errors.As even when wrapped (defense against a caller that fmt.Errorf-wraps a
// sentinel), so the explicit code is honored rather than falling through to 1.
func TestTranslateExit_WrappedErrExitCode(t *testing.T) {
	wrapped := fmt.Errorf("context: %w", &errExitCode{code: 2, msg: "usage"})
	if got := translateExit(wrapped); got != 2 {
		t.Errorf("translateExit(wrapped errExitCode) = %d, want 2", got)
	}
}

// TestRootCmd_FlagErrorIsUsageExit verifies the root SetFlagErrorFunc wraps a
// flag-parse error into errExitCode{code: usageExitCode}, so a bad flag exits 2
// through the same seam as the arg/command usage errors. Driving Execute() (with
// a subcommand that inherits the root's FlagErrorFunc) exercises the real hook
// wiring, not just translateExit in isolation.
func TestRootCmd_FlagErrorIsUsageExit(t *testing.T) {
	root := newRootCmd()
	root.SetArgs([]string{"list", "--definitely-not-a-flag"})
	// Silence cobra's own writers so the test output stays clean; the root
	// already sets SilenceUsage/SilenceErrors, but SetArgs-driven runs still
	// print via the command's out/err — redirect them to a discard sink.
	root.SetOut(nil)
	root.SetErr(nil)

	err := root.Execute()
	if err == nil {
		t.Fatal("expected a flag error, got nil")
	}
	var withCode *errExitCode
	if !errors.As(err, &withCode) {
		t.Fatalf("flag error was not wrapped in errExitCode: %v", err)
	}
	if withCode.code != usageExitCode {
		t.Errorf("flag error exit code = %d, want %d (usageExitCode)", withCode.code, usageExitCode)
	}
	if translateExit(err) != usageExitCode {
		t.Errorf("translateExit(flag error) = %d, want %d", translateExit(err), usageExitCode)
	}
}
