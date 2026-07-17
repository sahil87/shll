// Command shll is the meta-CLI for the sahil87 toolkit. It composes operations
// that span every per-tool CLI (hop, wt, fab-kit, run-kit, tu, idea) so users have
// one entry point for cross-toolkit concerns.
//
// See `shll --help` for the user-facing surface; the canonical contract for this
// binary lives in the active fab change spec (under fab/changes/) until hydrated
// to docs/memory/.
package main

import (
	"errors"
	"fmt"
	"os"
	"strings"
)

// version is the binary version, overridden via -ldflags "-X main.version=..." at
// build time. The default value `dev` covers `go run` and unstamped local builds.
var version = "dev"

func main() {
	rootCmd := newRootCmd()
	rootCmd.Version = version

	if err := rootCmd.Execute(); err != nil {
		os.Exit(translateExit(err))
	}
}

// translateExit maps RunE (and cobra-internal) errors to process exit codes,
// honoring the toolkit convention documented in the principles standard:
//
//	0 — success
//	1 — operational failure (a command ran but the operation failed)
//	2 — usage error (bad invocation: unknown command/flag, wrong arg count)
//
// Sentinels and classification, in order:
//   - errExitCode{...}  → its explicit code. Flag-parse errors are wrapped in
//     errExitCode{code: 2} by the root SetFlagErrorFunc (root.go), and
//     shell-init wraps its own bad-shell usage error as code 2. So the exit-2
//     usage cases that flow through a hook or a RunE arrive already typed here.
//   - errSilent         → 1 (caller already wrote its own diagnostic to stderr).
//   - a cobra usage error → 2. Cobra v1.10.2 exposes no typed sentinel for the
//     arg/command usage errors it raises OUTSIDE flag parsing (unknown command,
//     invalid argument, wrong arg count); they are plain fmt.Errorf values, so
//     they are classified here by their stable message prefixes (isUsageError).
//     Cobra still prints these itself only when SilenceUsage is false — the root
//     silences usage, so we print the diagnostic here to keep stderr populated.
//   - anything else     → 1 (operational failure); print it to stderr.
func translateExit(err error) int {
	if err == nil {
		return 0
	}
	var withCode *errExitCode
	if errors.As(err, &withCode) {
		if withCode.msg != "" {
			fmt.Fprintln(os.Stderr, withCode.msg)
		}
		return withCode.code
	}
	if errors.Is(err, errSilent) {
		return 1
	}
	if isUsageError(err) {
		fmt.Fprintln(os.Stderr, err)
		return usageExitCode
	}
	fmt.Fprintln(os.Stderr, err)
	return 1
}

// usageExitCode is the toolkit-convention exit code for a usage error (bad
// invocation). Named constant so main.go and root.go's SetFlagErrorFunc share
// one source of truth (code-quality.md: no magic numbers).
const usageExitCode = 2

// cobraUsageErrorPrefixes are the stable leading substrings of the arg/command
// usage errors cobra v1.10.2 raises outside flag parsing (see the upstream
// args.go / command.go error strings). Cobra exposes no typed sentinel for
// these, so translateExit classifies them by prefix. Flag-parse errors do NOT
// need to appear here — they are wrapped into errExitCode{code: 2} by the root
// SetFlagErrorFunc before Execute returns — but the flag-error prefixes are
// included as defense-in-depth in case a flag error ever reaches this seam
// unwrapped (e.g. a subcommand that overrode FlagErrorFunc).
var cobraUsageErrorPrefixes = []string{
	"unknown command ",
	"unknown flag: ",
	"unknown shorthand flag: ",
	"invalid argument ",
	"accepts ",  // e.g. "accepts 1 arg(s), received 2"
	"requires ", // e.g. "requires at least 1 arg(s), only received 0"
}

// isUsageError reports whether err is a cobra usage error (bad invocation),
// classified by its stable message prefix. Anchored to the message shape cobra
// produces, not a loose substring search, so an operational error whose message
// merely contains one of these words elsewhere is not misclassified.
func isUsageError(err error) bool {
	msg := err.Error()
	for _, p := range cobraUsageErrorPrefixes {
		if strings.HasPrefix(msg, p) {
			return true
		}
	}
	return false
}

// errSilent is returned by subcommands that have already written their own
// diagnostic to stderr; translateExit suppresses the default stderr write.
var errSilent = errors.New("shll: silent error")

// errExitCode carries an explicit exit code plus an optional stderr message.
// Used by subcommands that need to exit with codes other than 0 or 1
// (e.g. `shll shell-init` exits 2 on bad shell argument).
type errExitCode struct {
	code int
	msg  string
}

func (e *errExitCode) Error() string { return e.msg }
