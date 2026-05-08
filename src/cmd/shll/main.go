// Command shll is the meta-CLI for the sahil87 toolkit. It composes operations
// that span every per-tool CLI (hop, wt, fab-kit, rk, tu, idea) so users have one
// entry point for cross-toolkit concerns.
//
// See `shll --help` for the user-facing surface; the canonical contract for this
// binary lives in the active fab change spec (under fab/changes/) until hydrated
// to docs/memory/.
package main

import (
	"errors"
	"fmt"
	"os"
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

// translateExit maps RunE errors to process exit codes. Cobra prints its own
// usage errors when SilenceUsage is true, so this layer only surfaces our own
// sentinels.
//
// Sentinels:
//   - errSilent       → 1 (caller already wrote the diagnostic to stderr)
//   - errExitCode{...} → custom code (used by shell-init for exit 2 on bad shell)
//
// Default: print the error to stderr and exit 1.
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
	fmt.Fprintln(os.Stderr, err)
	return 1
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
