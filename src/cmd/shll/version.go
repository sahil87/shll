package main

import (
	"context"
	"errors"
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
		Short: "print versions of shll and every installed shll tool",
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
// "installed = runnable" probe shared by toolVersion and toolInstalled. For a
// BREW-MANAGED tool it invokes `<tool.Name> --version` via proc.Run
// (Constitution I — subprocess via internal/proc), bounded by versionTimeout,
// and returns the captured output and any error. ANY error (proc.ErrNotFound
// for a missing binary, non-zero exit, timeout) means "not installed"; callers
// map that to their own representation (notInstalledLabel for version, a bool
// for toolInstalled). This is NOT the brew probe (isInstalled in brew.go) used
// by install/update.
//
// DELEGATED (NON-BREW) SEAM: for a tool with a Probe spec (rk-desktop) there is
// no `--version` surface — the probe runs the spec's Argv
// (`rk desktop status`) via proc.RunCaptured under the same versionTimeout
// bound, parses the LinePrefix line (`Installed:`), and returns a synthetic
// `<Name> version <value>` line on success (which normalizeVersion's
// prefix-strip renders as the raw value — run-kit's `v<X>` passes through
// verbatim). Any failure — transport error, non-zero exit (including an
// unsupported-platform refusal), a missing line, or the AbsentValue — returns
// an error, which every caller maps to its not-installed representation.
//
// LEGACY-NAME FALLBACK (rk→run-kit rename): when the primary probe fails with
// proc.ErrNotFound ONLY — i.e. `<tool.Name>` is not on PATH — AND the tool declares
// a LegacyName, it retries once with the legacy binary name so a pre-rename install
// (whose binary is still `rk`, not `run-kit`) is reported installed by
// list/version/doctor. The fallback fires ONLY on ErrNotFound: a present-but-broken
// `run-kit` (non-zero exit, timeout/deadline) must NOT silently defer to `rk` — its
// own error is returned. This serves DISPLAY surfaces only; the display name stays
// tool.Name regardless of which probe name succeeded. A retained LegacyName
// surface — the run-kit formula still installs `rk` as an interchangeable alias.
func probeToolVersion(ctx context.Context, tool Tool) ([]byte, error) {
	if tool.Probe != nil {
		return probeDelegatedVersion(ctx, tool)
	}
	out, err := probeVersionByName(ctx, tool.Name)
	if errors.Is(err, proc.ErrNotFound) && tool.LegacyName != "" {
		return probeVersionByName(ctx, tool.LegacyName)
	}
	return out, err
}

// probeDelegatedVersion runs a delegated (non-brew) tool's Probe spec and maps
// the outcome onto probeToolVersion's ([]byte, error) contract (see its doc
// comment). Both streams are captured (proc.RunCaptured) so a refusal/status
// message on stderr is visible to the refusal matcher, and the invocation is
// bounded by the same versionTimeout as the `--version` path.
func probeDelegatedVersion(ctx context.Context, tool Tool) ([]byte, error) {
	subCtx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	stdout, stderr, code, err := proc.RunCaptured(subCtx, tool.Probe.Argv[0], tool.Probe.Argv[1:]...)
	if err != nil {
		return nil, err
	}
	if code != 0 {
		// A non-zero exit (e.g. run-kit's unsupported-platform refusal) means
		// "not actionable here" — collapse to the not-installed signal; callers
		// that need the refusal detail re-run the spec (see install.go).
		return nil, errors.New(string(stderr))
	}
	value, installed := parseProbeStatusLine(string(stdout), tool.Probe)
	if !installed {
		return nil, errors.New(tool.Name + ": " + notInstalledLabel)
	}
	// Synthetic `<Name> version <value>` line: normalizeVersion's prefix-strip
	// renders the raw value (run-kit's `v<X>` passes through verbatim).
	return []byte(tool.Name + " version " + value + "\n"), nil
}

// probeToolInstalledVersion is the update-surface counterpart of brew.go's
// probeInstalledVersion: a single probe read yielding BOTH the install fact and
// the pre-upgrade version. Brew-managed tools take the brew probe; delegated
// (non-brew) tools take the Probe spec via probeToolVersion (the version is
// normalizeVersion's render of the `Installed:` line, "" when absent — best-
// effort, suppressing only a digest entry, never the upgrade).
func probeToolInstalledVersion(ctx context.Context, t Tool) (installed bool, version string) {
	if !t.brewManaged() {
		out, err := probeToolVersion(ctx, t)
		if err != nil {
			return false, ""
		}
		return true, normalizeVersion(string(out))
	}
	return probeInstalledVersion(ctx, t.Formula)
}

// parseProbeStatusLine scans captured probe output for a line whose FIRST
// whitespace field is the spec's LinePrefix (trimmed) and classifies it: the
// remaining value == AbsentValue → not installed; any other value → installed,
// and the value is the installed version; a missing line (or a prefix line with
// no value) reports ("", false). Whitespace-separated prefix/value, never a
// regex (code-quality.md anti-pattern). Callers distinguish the three outcomes
// via (value, installed): ("", false) = no status line found;
// (AbsentValue, false) = cleanly reported absent; (version, true) = installed.
func parseProbeStatusLine(out string, spec *ToolProbe) (value string, installed bool) {
	prefix := strings.TrimSpace(spec.LinePrefix)
	for _, line := range strings.Split(out, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 || fields[0] != prefix {
			continue
		}
		if len(fields) == 1 {
			return "", false
		}
		value = strings.Join(fields[1:], " ")
		if value == spec.AbsentValue {
			return value, false
		}
		return value, true
	}
	return "", false
}

// probeVersionByName runs `<name> --version` under a versionTimeout deadline via
// proc.Run (capture). Factored out of probeToolVersion so the legacy-name fallback
// reuses the exact same bounded invocation for the retry.
func probeVersionByName(ctx context.Context, name string) ([]byte, error) {
	subCtx, cancel := context.WithTimeout(ctx, versionTimeout)
	defer cancel()
	return proc.Run(subCtx, name, "--version")
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
