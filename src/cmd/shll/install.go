package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

func newInstallCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "install",
		Short: "brew install every sahil87 tool that isn't already installed",
		Long: `Install every roster tool that isn't already installed via Homebrew.

shll install iterates the roster (` + "`wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`" + `)
and runs ` + "`brew install sahil87/tap/<formula>`" + ` for each one that is missing.
Tools that are already installed are skipped silently — the command is
idempotent and safe to re-run. Brew's progress output streams directly to
your terminal.

shll install does NOT upgrade already-installed tools. Use ` + "`shll update`" + `
for that.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runInstall(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

// runInstall is the implementation seam for `shll install`. Extracted from
// the cobra factory so install_test.go can drive it with bytes.Buffer writers
// and a fake proc.Runner.
//
// Behavior:
//   - brew missing → stderr hint, errSilent (exit 1).
//   - For each roster tool in order, skip if already installed; else run
//     `brew install sahil87/tap/<formula>` foregrounded.
//   - Best-effort across the roster: a per-tool install failure does not abort
//     the loop. The overall exit code reflects whether any failed.
//   - If everything is already installed, write a one-line note to stdout and
//     exit 0 — mirrors `shll update`'s "nothing to do" UX.
//
// Note: no `brew update --quiet` — `brew install` resolves the formula via
// the tap directly and doesn't need a metadata refresh as a precondition.
func runInstall(ctx context.Context, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !hasBrew(ctx) {
		fmt.Fprintln(stderr, installBrewMissingHint)
		return errSilent
	}

	// Collect the tools that are not yet installed. The slice is built by
	// walking Roster in order, so iterating `missing` below preserves roster
	// order deterministically.
	missing := make([]Tool, 0, len(Roster))
	for _, t := range Roster {
		if !isInstalled(ctx, t.Formula) {
			missing = append(missing, t)
		}
	}

	if len(missing) == 0 {
		fmt.Fprintln(stdout, "All sahil87 tools already installed.")
		return nil
	}

	// Per-tool boundary framing. The color decision is computed once against the
	// stdout writer and reused for every header and the tail, so they share the
	// stream the foregrounded `brew install` output is written to (stdout), never
	// stderr. succeeded/total feed the summary tail by exit code only, mirroring
	// the anyFailed facts.
	color := colorEnabled(stdout)
	succeeded := 0

	anyFailed := false
	for _, t := range missing {
		printToolHeader(stdout, t.Name, color)
		code, err := proc.RunForeground(ctx, brewBinary, "install", t.Formula)
		if err != nil {
			fmt.Fprintf(stderr, "shll install: %s: %v\n", t.Name, err)
			anyFailed = true
			continue
		}
		if code != 0 {
			anyFailed = true
			continue
		}
		succeeded++
	}

	// Summary tail by exit-code counts. Printed only after the install loop ran
	// (the all-already-installed short-circuit returned earlier with no header and
	// no tail). Presentation only — it does not influence the exit code.
	printSummaryTail(stdout, succeeded, len(missing), color)

	if anyFailed {
		return errSilent
	}
	return nil
}
