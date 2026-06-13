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

// shllSelfLabel is the per-tool header / preview label for shll's own self-upgrade
// step. shll is not in Roster, so its label is a named constant rather than a
// Tool.Name. Named per code-quality.md (no magic strings).
const shllSelfLabel = "shll (self)"

// noToolsInstalledMsg is the nothing-to-do message for `shll update` (no roster tool
// installed AND shll itself not brew-installed). Shared by the normal short-circuit and
// the dry-run empty case so both read identically. Named per code-quality.md.
const noToolsInstalledMsg = "No sahil87 tools installed."

func newUpdateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "update [tool...]",
		Short: "brew update + per-tool update for shll and every installed sahil87 tool",
		Long: `Update shll itself and every installed sahil87 tool via Homebrew.

shll update runs ` + "`brew update --quiet`" + ` once, then ` + "`brew upgrade sahil87/tap/shll`" + `
(when shll itself was installed via brew), then delegates to each installed roster
tool's own ` + "`update`" + ` subcommand (with ` + "`--skip-brew-update`" + ` when the tool
advertises it) so each tool's post-upgrade side effects (e.g. rk's daemon restart)
are preserved. A roster tool that exposes no ` + "`update`" + ` is upgraded via
` + "`brew upgrade sahil87/tap/<formula>`" + ` instead. Uninstalled tools (including shll
itself, e.g. on a ` + "`go install`" + ` dev build) are skipped silently. Brew and per-tool
progress output streams directly to your terminal.

With no arguments, shll update processes the whole roster as above. Pass one or
more tool names to update only that subset (valid targets: shll, wt, idea, tu, rk,
hop, fab-kit) — e.g. ` + "`shll update shll`" + ` to bump only shll itself, or
` + "`shll update hop wt`" + ` for a pair. The subset is always processed in roster order
regardless of the order given. An unknown name, or a named tool that is not
installed, is a hard error (a named tool, unlike the whole-roster sweep, is not
silently skipped).`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool(dryRunFlag)
			return runUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), dryRun, args)
		},
	}
	cmd.Flags().Bool(dryRunFlag, false, dryRunFlagUsage)
	return cmd
}

// dryRunFlag is the bool flag (on both `update` and `install`) that previews the plan
// and exits without any side effect. Named constant per code-quality.md.
const dryRunFlag = "dry-run"

// dryRunFlagUsage is the shared cobra usage string for the --dry-run flag.
const dryRunFlagUsage = "preview what would run, without making any changes"

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
// writers and a fake proc.Runner. When dryRun is set, the read-only probes still
// run (they are reads) but NO write is performed — no `brew update --quiet`, no
// `brew upgrade`, no `<tool> update` — and the planned commands are previewed.
//
// args are the positional tool-name targets. Empty args = the whole-roster run
// (unchanged behavior). One or more args restrict the run to that validated
// subset (valid targets: the Roster names plus shll itself). An unknown name is a
// hard error reported before any work; a named-but-not-installed target is an
// error too (distinct from the whole-roster graceful skip — explicitly naming a
// tool means the user expects it present).
func runUpdate(ctx context.Context, stdout, stderr io.Writer, dryRun bool, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Resolve the subset UP FRONT — before hasBrew, the status line, and any
	// probe — so an unknown target fails loudly with no brew/network side effect.
	// allowShll=true: shll itself is a valid `update` target (the motivating
	// `shll update shll` case). Empty args yields an empty selection and the
	// subset==false path below keeps the whole-roster behavior.
	subset := len(args) > 0
	selected, selfSelected, err := resolveTargets(args, true)
	if err != nil {
		fmt.Fprintf(stderr, "shll update: %v\n", err)
		return errSilent
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
	// local-build shll is not brew-managed and brew upgrade would error. For a
	// subset run, shll is acted on only when it was explicitly named.
	shllInstalled := isInstalled(ctx, shllFormula)
	shllSelfInstalled := shllInstalled
	if subset {
		shllSelfInstalled = selfSelected && shllInstalled
	}

	// Apply the subset to the probe results: a subset run acts on the named tools
	// only. First enforce the named-but-not-installed error, THEN mark every
	// non-selected roster tool as not-installed so the existing
	// total/upgrade-loop/dry-run/tail code paths operate on the subset with no
	// structural change (they all key off probes[i].installed). The whole-roster
	// run leaves probes untouched.
	if subset {
		want := make(map[string]bool, len(selected))
		for _, t := range selected {
			want[t.Name] = true
		}

		// Named-but-not-installed is an error (distinct from the whole-roster
		// graceful skip): explicitly naming a tool means the user expects it
		// present, so its absence is surfaced rather than silently skipped. Probe
		// results for selected tools still carry their true install status (only
		// non-selected tools get zeroed, below). Check every selected target (incl.
		// shll-self) before any brew write, and report all missing targets at once
		// in roster order for a better one-shot fix.
		var missingNamed []string
		if selfSelected && !shllInstalled {
			missingNamed = append(missingNamed, shllTargetToken)
		}
		for i, t := range Roster {
			if want[t.Name] && !probes[i].installed {
				missingNamed = append(missingNamed, t.Name)
			}
		}
		if len(missingNamed) > 0 {
			for _, name := range missingNamed {
				fmt.Fprintf(stderr, "shll update: %s: not installed\n", name)
			}
			return errSilent
		}

		for i := range Roster {
			if !want[Roster[i].Name] {
				probes[i].installed = false
			}
		}
	}

	anyInstalled := false
	for _, p := range probes {
		if p.installed {
			anyInstalled = true
			break
		}
	}

	if !anyInstalled && !shllSelfInstalled {
		fmt.Fprintln(stdout, noToolsInstalledMsg)
		return nil
	}

	// Dry-run: probes have run (they are reads); now preview the exact commands
	// the real run WOULD execute and exit 0 with NO write. The preview lists only
	// actionable tools — shll (self) first when brew-installed, then each installed
	// roster tool in roster order — using the same argv upgradeTool would build.
	// Critically, NO `brew update --quiet`, NO `brew upgrade`, NO `<tool> update`
	// is invoked below this point in dry-run.
	if dryRun {
		rows := make([]previewRow, 0, len(Roster)+1)
		if shllSelfInstalled {
			rows = append(rows, previewRow{label: shllSelfLabel, cmd: argvString(brewBinary, "upgrade", shllFormula)})
		}
		for i, t := range Roster {
			if !probes[i].installed {
				continue
			}
			rows = append(rows, previewRow{label: t.Name, cmd: argvString(upgradeArgv(t, probes[i].supportsSkipFlag)...)})
		}
		printUpdatePreview(stdout, rows)
		return nil
	}

	// Wall-clock start for the run-duration suffix in the summary tail. Captured
	// from the injectable nowFunc seam (clock.go) so tests pin it deterministically.
	// Taken after the short-circuit/dry-run returns so it measures only the
	// write-phase the tail summarizes.
	start := nowFunc()

	// Refresh brew metadata once. Foregrounded so users see progress. Because
	// each delegated `<tool> update --skip-brew-update` skips its own internal
	// brew update, this run-wide refresh happens exactly once.
	// proc.RunForeground returns (code, nil) when the subprocess exits non-zero
	// (it only sets err when exec itself fails before/after spawn), so we must
	// check both code != 0 and err != nil to treat any non-success as failure.
	// brewEnv() carries the Linux-only HOMEBREW_NO_REQUIRE_TAP_TRUST=1 sandbox
	// workaround (nil on macOS) — see brewEnv in brew.go (backlog [38a6]/[tkch]).
	if code, err := proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "update", "--quiet"); err != nil || code != 0 {
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

	// Per-tool boundary framing. The color decision is computed once against the
	// stdout writer (a TTY+NO_COLOR check) and reused for every header and the
	// tail, so headers and tail stay on the same stream the foregrounded sub-tool
	// output is written to (stdout) and never to stderr. succeeded feeds the
	// summary tail; it counts by exit code only, mirroring the anyFailed facts.
	//
	// total (M) is the per-tool counter denominator and MUST be known before the
	// loop so each header can read `[N/M]`. It is the count of installed roster
	// tools plus 1 when shll itself is brew-installed — derived up front from the
	// probe results and shllSelfInstalled, not incremented inside the loop. pos is
	// the running 1-based position; shll (self) is [1/M] and the first header.
	color := colorEnabled(stdout)
	succeeded := 0
	total := 0
	for _, p := range probes {
		if p.installed {
			total++
		}
	}
	if shllSelfInstalled {
		total++
	}
	pos := 0

	// updateHeader emits the per-tool header with a blank line before every header
	// EXCEPT the first (section spacing — make tool boundaries obvious).
	updateHeader := func(name string) {
		pos++
		if pos > 1 {
			fmt.Fprintln(stdout)
		}
		printToolHeader(stdout, name, pos, total, color)
	}

	// Self-upgrade shll first so subsequent operations in this run benefit from
	// the updated binary on disk (the running process keeps its mapped image,
	// but a follow-up invocation picks up the new binary). shll has no `update`
	// subcommand to call on itself, so this stays a direct brew upgrade.
	if shllSelfInstalled {
		updateHeader(shllSelfLabel)
		// brewEnv() carries the Linux-only HOMEBREW_NO_REQUIRE_TAP_TRUST=1 sandbox
		// workaround (nil on macOS) — see brewEnv in brew.go (backlog [38a6]/[tkch]).
		code, err := proc.RunForegroundEnv(ctx, brewEnv(), brewBinary, "upgrade", shllFormula)
		if err != nil {
			fmt.Fprintf(stderr, "shll update: shll: %v\n", err)
			anyFailed = true
		} else if code != 0 {
			anyFailed = true
		} else {
			succeeded++
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
		updateHeader(t.Name)
		code, err := upgradeTool(ctx, t, probes[i].supportsSkipFlag)
		if err != nil {
			fmt.Fprintf(stderr, "shll update: %s: %v\n", t.Name, err)
			anyFailed = true
			continue
		}
		if code != 0 {
			anyFailed = true
			continue
		}
		succeeded++
	}

	// Summary tail: one line by exit-code counts (Done — N of M / X succeeded,
	// Y failed) plus the wall-clock run duration. A blank line precedes it so the
	// final tool's streamed output is separated from the tail (same section-spacing
	// rule as the per-tool headers). Printed only after the per-tool loop ran (the
	// empty-case short-circuit returned earlier with no header and no tail).
	// Presentation only — it does not influence the exit code below.
	fmt.Fprintln(stdout)
	printSummaryTail(stdout, succeeded, total, nowFunc().Sub(start), color)

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
// <formula>` for a tool with no Update argv. The exact argv is built by
// upgradeArgv so the dry-run preview can render the same command without running
// it (single source of truth for the per-tool dispatch).
func upgradeTool(ctx context.Context, t Tool, supportsSkipFlag bool) (int, error) {
	argv := upgradeArgv(t, supportsSkipFlag)
	// Inject the brew sandbox-trust workaround env ONLY when this is a brew call
	// (the `brew upgrade <formula>` fallback). A `<tool> update` delegation runs
	// the tool's own process — not shll's to configure — so it gets no env. The
	// decision lives here (the runtime), keeping upgradeArgv/argvString (the shared
	// dry-run preview source) env-free. See brewEnv in brew.go (backlog [38a6]/[tkch]).
	var env []string
	if argv[0] == brewBinary {
		env = brewEnv()
	}
	return proc.RunForegroundEnv(ctx, env, argv[0], argv[1:]...)
}

// upgradeArgv returns the exact argv `shll update` would run for an installed roster
// tool, per the same dispatch upgradeTool uses:
//   - has Update argv + supports the flag → `<tool> update --skip-brew-update`
//   - has Update argv, no flag (version skew) → `<tool> update`
//   - no Update argv (hypothetical future tool) → `brew upgrade <formula>`
//
// It is the single source of truth shared by the live upgrade (upgradeTool) and the
// dry-run preview, so the preview can never drift from what the run would do.
func upgradeArgv(t Tool, supportsSkipFlag bool) []string {
	if len(t.Update) == 0 {
		return []string{brewBinary, "upgrade", t.Formula}
	}
	// Copy the shared, read-only Update argv into a fresh slice before optionally
	// appending the flag — never mutate the roster's backing array (same
	// slice-aliasing guard as appendArg).
	argv := make([]string, len(t.Update))
	copy(argv, t.Update)
	if supportsSkipFlag {
		argv = appendArg(argv, skipBrewUpdateFlag)
	}
	return argv
}

// argvString joins a command argv into a single display string for the dry-run
// preview (e.g. {"wt","update","--skip-brew-update"} → "wt update --skip-brew-update").
// Presentation-only: the real run passes the argv slice to proc, never this string.
func argvString(argv ...string) string {
	return strings.Join(argv, " ")
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
