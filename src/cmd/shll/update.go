package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "brew update + brew upgrade for every installed sahil87 tool",
		Long: `Update every installed sahil87 tool via Homebrew.

shll update runs ` + "`brew update --quiet`" + ` then ` + "`brew upgrade sahil87/tap/<formula>`" + ` for
every roster tool that is currently installed. Uninstalled tools are skipped
silently. Brew's progress output streams directly to your terminal.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

// runUpdate is the implementation seam for `shll update`. Extracted from the
// cobra factory so update_test.go can drive it directly with bytes.Buffer
// writers and a fake proc.Runner.
func runUpdate(ctx context.Context, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if !hasBrew(ctx) {
		fmt.Fprintln(stderr, brewMissingHint)
		return errSilent
	}

	// Filter the roster down to installed tools first — this lets us short-circuit
	// the "no sahil87 tools installed" branch before doing the (cheap) brew update.
	installed := make([]Tool, 0, len(Roster))
	for _, t := range Roster {
		if isInstalled(ctx, t.Formula) {
			installed = append(installed, t)
		}
	}

	if len(installed) == 0 {
		fmt.Fprintln(stdout, "No sahil87 tools installed.")
		return nil
	}

	// Refresh brew metadata once. Foregrounded so users see progress.
	if _, err := proc.RunForeground(ctx, brewBinary, "update", "--quiet"); err != nil {
		fmt.Fprintf(stderr, "shll update: brew update failed: %v\n", err)
		return errSilent
	}

	// Sequentially upgrade each installed roster tool. A failure in one tool is
	// surfaced but does not abort the run — we want best-effort across the roster
	// (spec Assumption #16). The overall exit code reflects whether any failed.
	anyFailed := false
	for _, t := range installed {
		code, err := proc.RunForeground(ctx, brewBinary, "upgrade", t.Formula)
		if err != nil {
			fmt.Fprintf(stderr, "shll update: %s: %v\n", t.Name, err)
			anyFailed = true
			continue
		}
		if code != 0 {
			anyFailed = true
		}
	}
	if anyFailed {
		return errSilent
	}
	return nil
}
