package main

import (
	"context"
	"fmt"
	"io"
	"regexp"
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

// versionTokenRE matches a SemVer-shaped token: optional leading `v`, at
// least one numeric component, optional `.`-separated additional numeric
// components, optional `[.-]<suffix>` (pre-release / build-metadata).
var versionTokenRE = regexp.MustCompile(`v?\d+(\.\d+)*([.-][\w.+-]+)?`)

// versionPrefixRE matches lines of the shape `<word> version <rest>` where
// `version` is case-insensitive. The captured group is `<rest>`.
var versionPrefixRE = regexp.MustCompile(`^\S+\s+(?i:version)\s+(.+)$`)

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
	fmt.Fprintf(w, "shll\t%s\n", normalizeVersion(version))
	for _, tool := range Roster {
		fmt.Fprintf(w, "%s\t%s\n", tool.Name, toolVersion(ctx, tool))
	}
	return w.Flush()
}

// probeToolVersion is the single definition of the install-mechanism-agnostic
// "installed = runnable on PATH" probe shared by toolVersion and toolInstalled.
// It invokes `<tool> --version` via proc.Run (Constitution I — subprocess via
// internal/proc), bounded by versionTimeout, and returns the captured output and
// any error. ANY error (proc.ErrNotFound for a missing binary, non-zero exit,
// timeout) means "not installed"; callers map that to their own representation
// (notInstalledLabel for version, a bool for toolInstalled). This is NOT the
// brew probe (isInstalled in brew.go) used by install/update.
func probeToolVersion(ctx context.Context, tool Tool) ([]byte, error) {
	subCtx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	return proc.Run(subCtx, tool.Name, "--version")
}

// toolInstalled reports whether the tool's binary is runnable on PATH, by
// invoking `<tool> --version` (bounded by versionTimeout) and treating ANY error
// (proc.ErrNotFound, non-zero exit, timeout) as not-installed. This is the
// install-mechanism-agnostic notion of "installed" shared by `version`, `list`,
// and (future) `doctor` — NOT the brew probe (isInstalled) used by
// install/update. It layers on the single probeToolVersion call so there is
// exactly one place that defines "installed = runnable".
func toolInstalled(ctx context.Context, tool Tool) bool {
	_, err := probeToolVersion(ctx, tool)
	return err == nil
}

// toolVersion returns the version string for a single roster tool, or
// notInstalledLabel on any failure (binary missing from PATH, --version
// errors, or timeout). The returned string never contains internal newlines —
// only the first non-empty line is reported.
//
// "Installed" here means "runnable on PATH" — proc.Run returns proc.ErrNotFound
// when the binary is missing, and any other failure mode (non-zero exit,
// timeout, etc.) falls under the same notInstalledLabel branch. This is
// install-mechanism agnostic (brew, from-source, etc.). It shares the single
// probeToolVersion call with toolInstalled so the "installed = runnable"
// definition lives in exactly one place.
func toolVersion(ctx context.Context, tool Tool) string {
	out, err := probeToolVersion(ctx, tool)
	if err != nil {
		return notInstalledLabel
	}
	return normalizeVersion(string(out))
}

// normalizeVersion extracts a displayable version string from a tool's
// `--version` output. Behavior is purely shape-based — no per-tool logic —
// so independent upstream `--version` standardization is absorbed without
// shll code changes.
//
// Order of operations on the first non-empty line:
//  1. If a SemVer-shaped token is present, return it with a `v` prefix
//     (added if absent).
//  2. Else if the line matches `<word> version <rest>`, return `<rest>`.
//  3. Else return the trimmed first non-empty line verbatim.
//
// Empty / whitespace-only input returns `""`.
func normalizeVersion(raw string) string {
	line := ""
	for _, candidate := range strings.Split(raw, "\n") {
		trimmed := strings.TrimSpace(candidate)
		if trimmed != "" {
			line = trimmed
			break
		}
	}
	if line == "" {
		return ""
	}
	if token := versionTokenRE.FindString(line); token != "" {
		if !strings.HasPrefix(token, "v") {
			return "v" + token
		}
		return token
	}
	if m := versionPrefixRE.FindStringSubmatch(line); m != nil {
		return strings.TrimSpace(m[1])
	}
	return line
}
