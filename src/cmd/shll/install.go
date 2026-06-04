package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [tool...]",
		Short: "brew install every sahil87 tool that isn't already installed",
		Long: `Install every roster tool that isn't already installed via Homebrew.

shll install iterates the roster (` + "`wt`, `idea`, `tu`, `rk`, `hop`, `fab-kit`" + `)
and runs ` + "`brew install sahil87/tap/<formula>`" + ` for each one that is missing.
Tools that are already installed are skipped silently — the command is
idempotent and safe to re-run. Brew's progress output streams directly to
your terminal.

With no arguments, shll install processes the whole roster as above. Pass one or
more tool names to install only that subset (valid targets: wt, idea, tu, rk, hop,
fab-kit) — e.g. ` + "`shll install hop wt`" + `. The subset is processed in roster order
regardless of the order given; an unknown name is a hard error. Unlike
` + "`shll update`" + `, ` + "`shll`" + ` itself is NOT a valid install target — you cannot
brew-install the running orchestrator.

shll install does NOT upgrade already-installed tools. Use ` + "`shll update`" + `
for that.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool(dryRunFlag)
			return runInstall(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), dryRun, args)
		},
	}
	cmd.Flags().Bool(dryRunFlag, false, dryRunFlagUsage)
	return cmd
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
//
// args are the positional tool-name targets. Empty args = the whole-roster run
// (unchanged behavior). One or more args restrict the run to that validated subset
// (valid targets: the Roster names ONLY — shll is rejected, since the running
// orchestrator cannot be brew-installed). An unknown name is a hard error reported
// before any work; a named tool that is already installed is filtered out of the
// install set (the idempotent skip, same as the whole-roster behavior).
func runInstall(ctx context.Context, stdout, stderr io.Writer, dryRun bool, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Resolve the subset UP FRONT — before hasBrew and any probe — so an unknown
	// target (including `shll`, which is rejected here) fails loudly with no brew
	// side effect. allowShll=false: shll is not a valid install target. Empty args
	// yields an empty selection and the whole-roster walk below.
	subset := len(args) > 0
	selected, _, err := resolveTargets(args, false)
	if err != nil {
		fmt.Fprintf(stderr, "shll install: %v\n", err)
		return errSilent
	}

	if !hasBrew(ctx) {
		fmt.Fprintln(stderr, installBrewMissingHint)
		return errSilent
	}

	// The roster to consider: the full Roster for a whole-roster run, or just the
	// named subset (in roster order — resolveTargets returns selected in roster
	// order) for a subset run.
	consider := Roster
	if subset {
		consider = selected
	}

	// Collect the tools that are not yet installed. The slice is built by
	// walking `consider` in order, so iterating `missing` below preserves roster
	// order deterministically. The isInstalled probes are reads, so they run in
	// dry-run too — only the `brew install` writes are skipped. A named-but-
	// already-installed target is simply filtered out here (idempotent skip).
	missing := make([]Tool, 0, len(consider))
	for _, t := range consider {
		if !isInstalled(ctx, t.Formula) {
			missing = append(missing, t)
		}
	}

	if len(missing) == 0 {
		fmt.Fprintln(stdout, allInstalledMsg)
		return nil
	}

	// Dry-run: the probes have run (reads); now preview the exact `brew install`
	// commands the real run WOULD execute and exit 0 with NO write. The preview
	// lists only the missing subset (actionable tools), in roster order.
	if dryRun {
		rows := make([]previewRow, 0, len(missing))
		for _, t := range missing {
			rows = append(rows, previewRow{label: t.Name, cmd: argvString(brewBinary, "install", t.Formula)})
		}
		printInstallPreview(stdout, rows)
		return nil
	}

	// Per-tool boundary framing. The color decision is computed once against the
	// stdout writer and reused for every header and the tail, so they share the
	// stream the foregrounded `brew install` output is written to (stdout), never
	// stderr. succeeded feeds the summary tail by exit code only, mirroring the
	// anyFailed facts. M (the counter denominator) is len(missing) — known up front.
	color := colorEnabled(stdout)
	total := len(missing)
	succeeded := 0

	// Wall-clock start for the run-duration suffix in the summary tail, from the
	// injectable nowFunc seam (clock.go). Captured after the short-circuit/dry-run
	// returns so it measures only the install phase the tail summarizes.
	start := nowFunc()

	anyFailed := false
	for i, t := range missing {
		// Section spacing: a blank line precedes every header EXCEPT the first.
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		printToolHeader(stdout, t.Name, i+1, total, color)
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

	// Summary tail by exit-code counts plus the wall-clock run duration. A blank
	// line precedes it (same section-spacing rule as the headers). Printed only
	// after the install loop ran (the all-already-installed short-circuit returned
	// earlier with no header and no tail). Presentation only — does not influence
	// the exit code.
	fmt.Fprintln(stdout)
	printSummaryTail(stdout, succeeded, total, nowFunc().Sub(start), color)

	if anyFailed {
		return errSilent
	}
	return nil
}

// allInstalledMsg is the nothing-to-do message for `shll install` (every roster tool
// already installed). Shared by the normal short-circuit and the dry-run empty case so
// both read identically. Named per code-quality.md.
const allInstalledMsg = "All sahil87 tools already installed."
