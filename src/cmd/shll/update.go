package main

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

// updateStatusLine is the instant-feedback line `shll update` writes to stdout
// before any probing, so the user sees output immediately rather than staring at
// a blank terminal during the (now concurrent) probe phase. Named constant per
// code-quality.md (no magic strings); the exact wording is asserted by a spec
// scenario.
const updateStatusLine = "Checking installed sahil87 tools…"

// skipBrewUpdateFlag is the toolkit-wide flag that makes a tool's own `update`
// skip its internal `brew update --quiet` step. `shll update` hoists that refresh
// into a single run-wide `brew update` (see runUpdate), then appends this flag to
// each delegated `<tool> update` that advertises support for it. Detection is a
// literal-substring presence check on `<tool> update --help` output — never a
// regex (code-quality.md anti-pattern).
const skipBrewUpdateFlag = "--skip-brew-update"

func newUpdateCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "update",
		Short: "brew update + per-tool update for shll and every installed sahil87 tool",
		Long: `Update shll itself and every installed sahil87 tool via Homebrew.

shll update runs ` + "`brew update --quiet`" + ` once, then ` + "`brew upgrade sahil87/tap/shll`" + `
(when shll itself was installed via brew), then delegates to each installed roster
tool's own ` + "`update`" + ` subcommand (with ` + "`--skip-brew-update`" + ` when the tool
advertises it) so each tool's post-upgrade side effects (e.g. rk's daemon restart)
are preserved. A roster tool that exposes no ` + "`update`" + ` is upgraded via
` + "`brew upgrade sahil87/tap/<formula>`" + ` instead. Uninstalled tools (including shll
itself, e.g. on a ` + "`go install`" + ` dev build) are skipped silently. Brew and per-tool
progress output streams directly to your terminal.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

// probeResult is the per-roster-tool outcome of the read-only capability probes.
// Indexed by roster position so results stay in roster order regardless of the
// order the concurrent probes complete in.
type probeResult struct {
	// installed reports whether the tool's formula is brew-installed.
	installed bool
	// supportsSkipFlag reports whether the tool's `update --help` advertises
	// the `--skip-brew-update` flag. Only meaningful when installed and the tool
	// has a non-empty Update argv.
	supportsSkipFlag bool
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

	// Instant first byte: tell the user we're working before the probe phase.
	// Printed unconditionally — before the nothing-to-do short-circuit — so the
	// empty case reads "Checking…\nNo sahil87 tools installed.".
	fmt.Fprintln(stdout, updateStatusLine)

	// Concurrent read-only capability probes across the roster. These take no
	// brew write lock, so they are safe to run in parallel — the explicit
	// carve-out to the "sequential, not parallel" decision, which governs
	// upgrades only. Their stdout is captured by proc.Run (not foregrounded);
	// proc.Run's TransportCapture still streams stderr to the terminal, but the
	// probes run here (`brew list --versions` and `<tool> update --help`, only
	// for installed tools that have an Update argv) write their meaningful output
	// to stdout and are silent on stderr in the normal case, so concurrent stderr
	// interleaving is a rare, cosmetic edge rather than a correctness concern.
	// Results are indexed by roster position so they stay in roster order
	// regardless of completion order (the upgrade loop below relies on roster
	// ordering). Concurrency lives here in the caller; every subprocess call still
	// routes through internal/proc (Constitution I).
	probes := probeRoster(ctx)

	// Self-upgrade only when shll was installed via brew. A `go install` or
	// local-build shll is not brew-managed and brew upgrade would error.
	shllSelfInstalled := isInstalled(ctx, shllFormula)

	anyInstalled := false
	for _, p := range probes {
		if p.installed {
			anyInstalled = true
			break
		}
	}

	if !anyInstalled && !shllSelfInstalled {
		fmt.Fprintln(stdout, "No sahil87 tools installed.")
		return nil
	}

	// Refresh brew metadata once. Foregrounded so users see progress. Because
	// each delegated `<tool> update --skip-brew-update` skips its own internal
	// brew update, this run-wide refresh happens exactly once.
	// proc.RunForeground returns (code, nil) when the subprocess exits non-zero
	// (it only sets err when exec itself fails before/after spawn), so we must
	// check both code != 0 and err != nil to treat any non-success as failure.
	if code, err := proc.RunForeground(ctx, brewBinary, "update", "--quiet"); err != nil || code != 0 {
		if err != nil {
			fmt.Fprintf(stderr, "shll update: brew update failed: %v\n", err)
		} else {
			fmt.Fprintf(stderr, "shll update: brew update failed: exit code %d\n", code)
		}
		return errSilent
	}

	// Best-effort: failures are recorded and reflected in the exit code, but
	// never abort the loop — same policy for shll-self and every roster tool.
	anyFailed := false

	// Self-upgrade shll first so subsequent operations in this run benefit from
	// the updated binary on disk (the running process keeps its mapped image,
	// but a follow-up invocation picks up the new binary). shll has no `update`
	// subcommand to call on itself, so this stays a direct brew upgrade.
	if shllSelfInstalled {
		code, err := proc.RunForeground(ctx, brewBinary, "upgrade", shllFormula)
		if err != nil {
			fmt.Fprintf(stderr, "shll update: shll: %v\n", err)
			anyFailed = true
		} else if code != 0 {
			anyFailed = true
		}
	}

	// Sequentially upgrade each installed roster tool in roster order (Design
	// Decision #3 — brew lock + interleaved foreground output mean upgrades stay
	// serial). Per-tool dispatch:
	//   - has Update argv + supports the flag → `<tool> update --skip-brew-update`
	//   - has Update argv but no flag (version skew) → `<tool> update` (no flag)
	//   - no Update argv (hypothetical future tool) → `brew upgrade <formula>`
	// A failure in one tool is surfaced but does not abort the run — best-effort
	// across the roster (Constitution V — Graceful Degradation). The overall exit
	// code reflects whether any failed.
	for i, t := range Roster {
		if !probes[i].installed {
			continue
		}
		code, err := upgradeTool(ctx, t, probes[i].supportsSkipFlag)
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

// probeRoster runs the read-only capability probes for every roster tool
// concurrently and returns the results indexed by roster position. Each probe
// determines whether the tool's formula is installed and, for installed tools
// that expose an `update` subcommand, whether that subcommand advertises
// `--skip-brew-update`. All subprocess calls route through internal/proc; only
// the dispatch is concurrent.
func probeRoster(ctx context.Context) []probeResult {
	results := make([]probeResult, len(Roster))
	var wg sync.WaitGroup
	for i, t := range Roster {
		wg.Add(1)
		go func(i int, t Tool) {
			defer wg.Done()
			results[i] = probeTool(ctx, t)
		}(i, t)
	}
	wg.Wait()
	return results
}

// probeTool performs the read-only capability probes for a single tool: install
// status, plus `--skip-brew-update` support for installed tools that have an
// Update argv. The help probe is skipped for uninstalled tools and for tools with
// no Update argv (there is nothing to delegate to).
func probeTool(ctx context.Context, t Tool) probeResult {
	if !isInstalled(ctx, t.Formula) {
		return probeResult{}
	}
	res := probeResult{installed: true}
	if len(t.Update) > 0 {
		res.supportsSkipFlag = toolSupportsSkipFlag(ctx, t)
	}
	return res
}

// toolSupportsSkipFlag reports whether `<tool> update --help` advertises the
// `--skip-brew-update` flag. It is a literal-substring presence check on the
// captured help output — never a regex (code-quality.md anti-pattern). A probe
// transport error (e.g. the binary missing despite being brew-installed) is
// treated as "not supported"; shll then degrades to a plain `<tool> update`.
func toolSupportsSkipFlag(ctx context.Context, t Tool) bool {
	out, err := proc.Run(ctx, t.Update[0], appendArg(t.Update[1:], "--help")...)
	if err != nil {
		return false
	}
	return strings.Contains(string(out), skipBrewUpdateFlag)
}

// upgradeTool upgrades a single installed roster tool, foregrounded. It delegates
// to the tool's own `update` subcommand when it has an Update argv (appending
// `--skip-brew-update` when supported), and falls back to `brew upgrade
// <formula>` for a tool with no Update argv.
func upgradeTool(ctx context.Context, t Tool, supportsSkipFlag bool) (int, error) {
	if len(t.Update) == 0 {
		return proc.RunForeground(ctx, brewBinary, "upgrade", t.Formula)
	}
	args := t.Update[1:]
	if supportsSkipFlag {
		args = appendArg(args, skipBrewUpdateFlag)
	}
	return proc.RunForeground(ctx, t.Update[0], args...)
}

// appendArg returns base with extra appended, without ever mutating base's
// backing array. The roster's Update argvs are shared, read-only slices; a naive
// append could write into the shared backing array when spare capacity exists, so
// we always allocate a fresh slice.
func appendArg(base []string, extra string) []string {
	out := make([]string, len(base), len(base)+1)
	copy(out, base)
	return append(out, extra)
}
