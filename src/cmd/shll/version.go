package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

// versionTimeout caps each per-tool `--version` invocation. 2s is generous
// (typical --version under 100ms) while bounding worst-case `shll version`
// runtime to len(roster) * versionTimeout. Spec Design Decision #5.
const versionTimeout = 2 * time.Second

// notInstalledLabel is the literal printed for a tool that is not installed
// or whose --version invocation fails/times out. Named constant to keep the
// formatting honest — magic strings forbidden by code-quality.md.
const notInstalledLabel = "not installed"

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "print versions of shll and every installed sahil87 tool",
		Long: `Print a column-aligned plain-text table showing the version of shll itself
plus every roster tool. Uninstalled tools show "not installed". Output is
plain text — no colors, no JSON — so it pastes cleanly into bug reports.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runVersion(cmd.Context(), cmd.OutOrStdout())
		},
	}
}

// runVersion writes the version table to stdout. Per-tool version invocation
// is bounded by versionTimeout (Constitution: bounded subprocess execution).
func runVersion(ctx context.Context, stdout io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	w := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "shll\t%s\n", version)
	for _, tool := range Roster {
		fmt.Fprintf(w, "%s\t%s\n", tool.Name, toolVersion(ctx, tool))
	}
	return w.Flush()
}

// toolVersion returns the version string for a single roster tool, or
// notInstalledLabel on any failure (binary missing, brew reports
// not-installed, --version errors, or timeout). The returned string never
// contains internal newlines — only the first non-empty line is reported.
func toolVersion(ctx context.Context, tool Tool) string {
	if !isInstalled(ctx, tool.Formula) {
		return notInstalledLabel
	}
	subCtx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	out, err := proc.Run(subCtx, tool.Name, "--version")
	if err != nil {
		return notInstalledLabel
	}
	return firstNonEmptyLine(string(out))
}

// firstNonEmptyLine returns the first non-blank line of s, trimmed of leading
// and trailing whitespace. Tools sometimes emit multi-line --version output
// (banner + version) — we only print the first useful line in the table.
func firstNonEmptyLine(s string) string {
	for _, line := range strings.Split(s, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" {
			return trimmed
		}
	}
	return ""
}
