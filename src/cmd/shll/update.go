package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"strings"
	"sync"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/changelog"
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
more tool names to update only that subset (valid targets: shll, wt, idea, tu,
run-kit, hop, fab-kit; the legacy alias ` + "`rk`" + ` still resolves to run-kit) — e.g.
` + "`shll update shll`" + ` to bump only shll itself, or
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
	// beforeVersion is the tool's installed version, captured from the SAME
	// `brew list --formula --versions` read the install probe runs (never from
	// streamed foreground output — code-quality.md). "" when not installed or the
	// version could not be parsed. Feeds the post-upgrade "What changed:" digest.
	// For a needsMigration tool this is the LEGACY keg's version (e.g. 2.5.13), so
	// the digest renders `run-kit 2.5.13 → 3.0.0` after migration.
	beforeVersion string
	// needsMigration marks a tool whose CURRENT formula is not installed but whose
	// LEGACY keg IS present (classified by keg leaf name — see probeTool). Such a
	// tool is upgraded via the brew-direct MIGRATION ACTION (migrateRunKit) instead
	// of delegating to `<tool> update`. Transitional for the rk→run-kit rename —
	// once legacy kegs die out this is always false and the guard is a cheap no-op.
	needsMigration bool
	// dualRack marks the observed "leftover" state where BOTH the current and legacy
	// kegs are installed (state C from the intake). The tool is migrated/present
	// (not needsMigration), but a one-line cleanup note is surfaced. Detection only —
	// shll never acts on the orphan (cleanup belongs to `shll uninstall` / manual brew).
	dualRack bool
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
	selected, selfSelected, aliased, err := resolveTargets(args, true)
	if err != nil {
		fmt.Fprintf(stderr, "shll update: %v\n", err)
		return errSilent
	}

	if !hasBrew(ctx) {
		fmt.Fprintln(stderr, brewMissingHint)
		return errSilent
	}

	// Legacy-alias notice: when the user named a retired token (e.g. `rk`), tell
	// them its canonical name once. Printed to stdout (not stderr) — it is a
	// friendly note, not an error — before the status line so it leads the output.
	printAliasNotices(stdout, aliased)

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
	// subset run, shll is acted on only when it was explicitly named. ONE
	// probeInstalledVersion read yields both the install fact AND shll's
	// pre-upgrade formula version (beforeShll) — the same single-read pattern
	// probeTool uses for roster tools, collapsing the former isInstalled +
	// separate installedVersion pair into one brew call.
	shllInstalled, beforeShll := probeInstalledVersion(ctx, shllFormula)
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
		// in roster order for a better one-shot fix. A LEGACY-KEG run-kit counts as
		// installed here (probeTool set installed=true, needsMigration=true), so
		// `shll update run-kit`/`shll update rk` on a legacy machine MIGRATES rather
		// than erroring "not installed" — the explicit user requirement.
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
			rows = append(rows, previewRow{label: t.Name, cmd: argvString(upgradeArgv(t, probes[i].supportsSkipFlag, probes[i].needsMigration)...)})
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

	// bumps records the version transitions of tools that ACTUALLY changed
	// (before != after, both known) after a successful upgrade — the input to the
	// "What changed:" digest tail. Collected in roster order (shll first) so the
	// digest renders deterministically. Presentation-only: it never touches
	// anyFailed or the exit code.
	var bumps []versionBump

	// Self-upgrade shll first so subsequent operations in this run benefit from
	// the updated binary on disk (the running process keeps its mapped image,
	// but a follow-up invocation picks up the new binary). shll has no `update`
	// subcommand to call on itself, so this stays a direct brew upgrade.
	if shllSelfInstalled {
		updateHeader(shllSelfLabel)
		// shll's pre-upgrade formula version (beforeShll) was captured above by the
		// single probeInstalledVersion read — a captured brew read, not the running
		// process's ldflags version, so the digest reports the brew-formula
		// transition the upgrade performed.
		code, err := proc.RunForeground(ctx, brewBinary, "upgrade", shllFormula)
		if err != nil {
			fmt.Fprintf(stderr, "shll update: shll: %v\n", err)
			anyFailed = true
		} else if code != 0 {
			anyFailed = true
		} else {
			succeeded++
			// Re-query the after-version and record a bump when it changed.
			if b, ok := makeBump(shllTargetToken, shllSelf.Repo, beforeShll, installedVersion(ctx, shllFormula)); ok {
				bumps = append(bumps, b)
			}
		}
	}

	// Sequentially upgrade each installed roster tool in roster order (Design
	// Decision #3 — brew lock + interleaved foreground output mean upgrades stay
	// serial). Per-tool dispatch (upgradeTool):
	//   - needsMigration (legacy rk keg) → the brew-direct migration action
	//     (`brew upgrade sahil87/tap/rk` + conditional `brew link run-kit` + notes)
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
		code, err := upgradeTool(ctx, stdout, stderr, t, probes[i])
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
		// Re-query the new version (a cheap captured read) and record a bump when
		// it actually changed from the pre-upgrade probe value. For a migrated
		// run-kit, beforeVersion is the LEGACY keg's version (from probeTool) and the
		// re-query reads the NEW formula (t.Formula is sahil87/tap/run-kit after the
		// rename), so the digest renders `run-kit 2.5.13 → 3.0.0`.
		if b, ok := makeBump(t.Name, t.Repo, probes[i].beforeVersion, installedVersion(ctx, t.Formula)); ok {
			bumps = append(bumps, b)
		}
	}

	// Summary tail: one line by exit-code counts (Done — N of M / X succeeded,
	// Y failed) plus the wall-clock run duration. A blank line precedes it so the
	// final tool's streamed output is separated from the tail (same section-spacing
	// rule as the per-tool headers). Printed only after the per-tool loop ran (the
	// empty-case short-circuit returned earlier with no header and no tail).
	// Presentation only — it does not influence the exit code below.
	fmt.Fprintln(stdout)
	printSummaryTail(stdout, succeeded, total, nowFunc().Sub(start), color)

	// "What changed:" digest tail: for every tool whose version actually changed,
	// fetch its release range and print a bold per-tool transition line followed
	// by its full release notes inline (shared `shll changelog` format). When no tool bumped
	// (all up-to-date or all failed), NOTHING prints — the output is byte-identical
	// to before this change. The color decision computed once above is threaded in
	// so the digest ASCII-degrades its `→`/`—` glyphs on a non-TTY/NO_COLOR stream
	// (per-tool-output-separation spec). Presentation-only: it never influences
	// anyFailed or the exit code below.
	printUpdateDigest(ctx, stdout, bumps, color)

	if anyFailed {
		return errSilent
	}
	return nil
}

// versionBump is one tool whose version changed during a `shll update` run — the
// digest tail's unit. Repo is the GitHub slug (not always the tool name — rk's is
// run-kit) so the digest can fetch releases and build the compare URL.
type versionBump struct {
	tool string
	repo string
	old  string
	new  string
}

// makeBump builds a versionBump when both versions are known AND they differ.
// Returns ok=false (and no bump) when either version is empty (unknown) or they
// are equal (nothing changed) — the guard that keeps a no-op run's output
// byte-identical to before this change.
//
// Both versions are normalized via changelog.NormalizeVer (strip a leading `v`
// and a brew `_N` revision suffix) BEFORE the equality guard, for two reasons:
// (1) brew can report revision-suffixed versions like `0.6.4_1`, and passing the
// raw form through would print a non-normalized transition and build a compare
// URL like `.../compare/v0.6.4_1...` that has no matching Git tag — normalizing
// here keeps the digest transition and changelog.CompareURL on the same footing
// as the toolkit's tags; (2) a revision-only change (`0.6.4` → `0.6.4_1`) is not
// a real version bump and normalizing before the equality check correctly
// suppresses it from the digest (no release notes exist for a revision bump).
func makeBump(tool, repo, old, new string) (versionBump, bool) {
	old = changelog.NormalizeVer(old)
	new = changelog.NormalizeVer(new)
	if old == "" || new == "" || old == new {
		return versionBump{}, false
	}
	return versionBump{tool: tool, repo: repo, old: old, new: new}, true
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
// status (with the rk→run-kit migration gate), plus `--skip-brew-update` support
// for installed tools that have an Update argv. The help probe is skipped for
// uninstalled tools, tools with no Update argv, and tools needing migration (there
// is nothing to delegate to — the migration is brew-direct).
//
// MIGRATION GATE (classify by keg LEAF NAME, never the LEGACY probe's exit code
// alone). For a tool that declares a LegacyFormula (only run-kit today):
//  1. Current formula (t.Formula) installed → migrated; normal delegation. Exit 0 on
//     the NEW formula means it is installed as the new formula, so no leaf check is
//     needed here (the leaf is `run-kit` when parseable; an exit-0-but-empty stdout
//     — a test edge, never real brew — is still the migrated formula). If the legacy
//     keg is ALSO present → dual-rack state C: still migrated, flag dualRack so the
//     caller surfaces a cleanup note.
//  2. Current formula NOT installed → probe LegacyFormula. Here the leaf is
//     LOAD-BEARING: after the tap rename, `brew list sahil87/tap/rk` can EXIT 0 yet
//     report leaf `run-kit` (rename-resolution — observed state B, which is MIGRATED,
//     not legacy). So classify by ITS leaf: leaf == t.LegacyName (`rk`) → a genuine
//     pre-rename keg → needsMigration, beforeVersion from the legacy keg; leaf ==
//     t.Name (`run-kit`, state B) → already migrated, normal delegation.
//  3. Neither → not installed.
//
// Tools with no LegacyFormula keep the plain single-probe behavior.
func probeTool(ctx context.Context, t Tool) probeResult {
	// One `brew list --formula --versions` read yields the exit-code install fact
	// (empty stdout with exit 0 still counts as installed), the keg leaf name, and
	// the before-version. The before-version is best-effort: "" means "unknown" and
	// suppresses only this tool's digest entry, never its upgrade.
	installed, leaf, before := probeInstalledLeaf(ctx, t.Formula)

	if t.LegacyFormula != "" {
		return probeRunKitMigration(ctx, t, installed, leaf, before)
	}

	if !installed {
		return probeResult{}
	}
	res := probeResult{installed: true, beforeVersion: before}
	if len(t.Update) > 0 {
		res.supportsSkipFlag = toolSupportsSkipFlag(ctx, t)
	}
	return res
}

// probeRunKitMigration is the migration gate for a tool that declares a
// LegacyFormula (run-kit). It takes the already-run current-formula probe result
// (installed/leaf/before — leaf is unused on this branch, see the note in
// probeTool) and returns the classified probeResult. Split out so `shll install`
// and `shll doctor` reuse the SAME detection path (Constitution III — one source of
// truth for the classification). See probeTool for the branch semantics.
func probeRunKitMigration(ctx context.Context, t Tool, installed bool, _ /*leaf*/, before string) probeResult {
	// Branch 1: current (renamed) formula installed → migrated. Exit 0 on the new
	// formula is authoritative here — the leaf is not re-checked (it disambiguates
	// only the LEGACY-formula probe in branch 2, where rename-resolution can make an
	// `rk`-formula probe report leaf `run-kit`).
	if installed {
		res := probeResult{installed: true, beforeVersion: before}
		// Dual-rack (state C): a genuine SEPARATE legacy keg is ALSO present. The
		// legacy-formula probe must report the LEGACY leaf (`rk`) to count — an
		// `sahil87/tap/rk` probe that exits 0 reporting leaf `run-kit` is just
		// rename-resolution pointing at the same single migrated keg (state B), NOT a
		// second rack. Keying dual-rack on the leaf avoids that false positive.
		// Detection is read-only; shll never acts on the orphan (cleanup is manual /
		// `shll uninstall`).
		if legacyInstalled, legacyLeaf, _ := probeInstalledLeaf(ctx, t.LegacyFormula); legacyInstalled && legacyLeaf == t.LegacyName {
			res.dualRack = true
		}
		if len(t.Update) > 0 {
			res.supportsSkipFlag = toolSupportsSkipFlag(ctx, t)
		}
		return res
	}

	// Branch 2: current formula not installed → probe the legacy formula and
	// classify by ITS leaf (never the exit code alone — state B exits 0 yet reports
	// leaf `run-kit`).
	legacyInstalled, legacyLeaf, legacyBefore := probeInstalledLeaf(ctx, t.LegacyFormula)
	if legacyInstalled && legacyLeaf == t.LegacyName {
		// Genuine legacy keg (leaf `rk`) → needs migration; before-version from it.
		return probeResult{installed: true, needsMigration: true, beforeVersion: legacyBefore}
	}
	if legacyInstalled && legacyLeaf == t.Name {
		// State B: the legacy-formula probe exits 0 and reports the CURRENT leaf
		// (rename resolution) → already migrated; normal delegation.
		res := probeResult{installed: true, beforeVersion: legacyBefore}
		if len(t.Update) > 0 {
			res.supportsSkipFlag = toolSupportsSkipFlag(ctx, t)
		}
		return res
	}

	// Branch 3: neither keg present → not installed.
	return probeResult{}
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

// Migration notes — named per code-quality.md (no magic strings). Printed to the
// user during a run-kit migration; the daemon note is PRINTED, never executed
// (Constitution III — shll never reimplements run-kit's own post-upgrade logic).
const (
	// migrationDaemonNoteFmt suggests restarting the run-kit daemon after a
	// brew-direct migration (which skips `run-kit update`'s restart side effect).
	// Takes the tool name. PRINT ONLY — shll never runs it.
	migrationDaemonNoteFmt = "note: %[1]s daemon (if running) was not restarted by the brew-direct migration — run '%[1]s serve --restart' if you use it"
	// migrationDualRackNoteFmt flags the state-C leftover (both kegs present). Takes
	// (legacy formula, current formula). Detection only — shll never removes the
	// orphan keg (that belongs to `shll uninstall` / manual brew).
	migrationDualRackNoteFmt = "note: a leftover %s keg remains alongside %s — remove it with 'shll uninstall' or 'brew uninstall %s'"
)

// upgradeTool upgrades a single installed roster tool, foregrounded. For a tool
// flagged needsMigration it runs the brew-direct MIGRATION ACTION (migrateRunKit)
// instead of delegating. Otherwise it delegates to the tool's own `update`
// subcommand when it has an Update argv (appending `--skip-brew-update` when
// supported), and falls back to `brew upgrade <formula>` for a tool with no Update
// argv. The exact argv for the non-migration + primary-migration path is built by
// upgradeArgv so the dry-run preview renders the same command without running it
// (single source of truth for the per-tool dispatch). stdout/stderr are threaded in
// so migrateRunKit can print its post-check/daemon/dual-rack notes.
func upgradeTool(ctx context.Context, stdout, stderr io.Writer, t Tool, p probeResult) (int, error) {
	if p.needsMigration {
		return migrateRunKit(ctx, stdout, stderr, t)
	}
	argv := upgradeArgv(t, p.supportsSkipFlag, p.needsMigration)
	code, err := proc.RunForeground(ctx, argv[0], argv[1:]...)
	// A non-migration run-kit upgrade that ends dual-rack (state C) still surfaces
	// the cleanup note (detection happened in probeTool).
	if p.dualRack {
		fmt.Fprintf(stdout, migrationDualRackNoteFmt+"\n", t.LegacyFormula, t.Formula, t.LegacyFormula)
	}
	return code, err
}

// migrateRunKit performs the brew-direct rk→run-kit migration for a legacy-keg
// tool. It NEVER delegates to the old binary (its rename handling is
// version-dependent and unfixable) and NEVER uninstall→installs (that failure-window
// repair path is the sibling `shll uninstall` change). Steps:
//  1. `brew upgrade <LegacyFormula>` — brew resolves the rename and migrates the keg
//     to the new formula. This is the exit code that drives success/failure; a
//     NON-SUCCESS upgrade (transport error OR non-zero exit — proc.RunForeground
//     returns (code, nil) on a non-zero exit, so the code must be checked) returns
//     immediately WITHOUT the post-steps, since a failed migration has nothing to
//     link, no daemon to note, and no reliable post-state to re-probe.
//  2. Post-check: if `<tool> --version` STILL returns proc.ErrNotFound (the
//     unlinked-keg pathology, observed state A), run `brew link <tool.Name>`.
//  3. PRINT (never run) a daemon-restart note — the brew-direct path skips
//     `run-kit update`'s post-upgrade restart side effect (Constitution III).
//  4. RE-PROBE the legacy formula: if a genuine legacy keg (leaf == t.LegacyName,
//     e.g. `rk`) STILL reports installed alongside the now-migrated new formula, the
//     migration left a dual-rack orphan (state C reached via a migration rather than
//     pre-existing) → PRINT a one-line cleanup note; shll never removes the orphan.
//     The dual-rack signal is DERIVED here from a post-migration probe, NOT from the
//     incoming probeResult: a `needsMigration` probe is branch 2 of
//     probeRunKitMigration, which never sets dualRack (the current formula was
//     not-installed pre-migration, so there was no second rack to detect then).
//
// step 2's link is best-effort (a link failure is surfaced to stderr but does not
// override the upgrade's exit code — the upgrade is what determines whether the tool
// migrated). Transitional — retire once legacy kegs die out.
func migrateRunKit(ctx context.Context, stdout, stderr io.Writer, t Tool) (int, error) {
	// proc.RunForeground returns (code, nil) on a non-zero exit — only exec-time
	// failures set err — so a failed migration is either err != nil OR code != 0.
	// Either way, return before the post-steps: there is no keg to link, no daemon
	// to note, and no trustworthy post-state to re-probe for a dual-rack orphan.
	code, err := proc.RunForeground(ctx, brewBinary, "upgrade", t.LegacyFormula)
	if err != nil || code != 0 {
		return code, err
	}

	// Post-check: on the unlinked-keg pathology (state A), the binary is still not
	// on PATH after the upgrade → link it. Only proc.ErrNotFound triggers the link
	// (a present-but-otherwise-failing --version is not an unlinked keg).
	if _, verr := probeVersionByName(ctx, t.Name); errors.Is(verr, proc.ErrNotFound) {
		if lcode, lerr := proc.RunForeground(ctx, brewBinary, "link", t.Name); lerr != nil {
			fmt.Fprintf(stderr, "shll update: %s: brew link failed: %v\n", t.Name, lerr)
		} else if lcode != 0 {
			fmt.Fprintf(stderr, "shll update: %s: brew link exited %d\n", t.Name, lcode)
		}
	}

	// Daemon-restart note — printed, never executed (Constitution III).
	fmt.Fprintf(stdout, migrationDaemonNoteFmt+"\n", t.Name)

	// Dual-rack cleanup note if the migration left a genuine legacy keg (leaf
	// t.LegacyName, e.g. `rk`) lingering alongside the now-migrated one. Detected by
	// RE-PROBING the legacy formula post-migration — keyed on the LEGACY leaf so a
	// `sahil87/tap/rk` probe that exits 0 reporting leaf `run-kit` (rename resolution
	// pointing at the single migrated keg) is NOT a false dual-rack positive.
	if legacyInstalled, legacyLeaf, _ := probeInstalledLeaf(ctx, t.LegacyFormula); legacyInstalled && legacyLeaf == t.LegacyName {
		fmt.Fprintf(stdout, migrationDualRackNoteFmt+"\n", t.LegacyFormula, t.Formula, t.LegacyFormula)
	}

	return code, nil
}

// upgradeArgv returns the exact argv `shll update` would run for an installed roster
// tool, per the same dispatch upgradeTool uses:
//   - needsMigration → the PRIMARY migration command `brew upgrade <LegacyFormula>`
//     (the conditional `brew link` + daemon/dual-rack notes are live-run side
//     effects, not previewable — migrateRunKit owns them)
//   - has Update argv + supports the flag → `<tool> update --skip-brew-update`
//   - has Update argv, no flag (version skew) → `<tool> update`
//   - no Update argv (hypothetical future tool) → `brew upgrade <formula>`
//
// It is the single source of truth shared by the live upgrade (upgradeTool) and the
// dry-run preview, so the preview can never drift from what the run would do.
func upgradeArgv(t Tool, supportsSkipFlag, needsMigration bool) []string {
	if needsMigration {
		return []string{brewBinary, "upgrade", t.LegacyFormula}
	}
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

// Digest tail literals — named per code-quality.md (no magic strings). The
// digest indent mirrors the tool nesting the user agreed to in the intake's
// sample output; release blocks render unindented via the shared renderReleases
// helper (the same format `shll changelog` uses).
const (
	digestHeader     = "What changed:"
	digestToolIndent = "  "
)

// printUpdateDigest prints the "What changed:" tail after the summary line, one
// block per tool that ACTUALLY bumped (bumps already excludes no-change/unknown
// tools, so an empty slice prints nothing — preserving the pre-change output
// byte-for-byte). For each bumped tool it fetches the release range (concurrently)
// and prints an indented per-tool transition line followed by the tool's full
// release notes inline:
//
//	What changed:
//	  tu 0.6.2 → 0.6.4 (2 releases)
//
//	v0.6.4  fix: opencode session parsing
//	## What's Changed
//	* fix: parse sessions with missing usage blocks
//
//	v0.6.3  feat: daily usage rollups
//	...
//
// The release bodies are rendered IN-PROCESS from the data changelog.FetchAll
// already returns (never a subprocess to `shll changelog`) via the shared
// renderReleases helper, so the digest and `shll changelog` render releases in
// one format — including the changelogCapPerTool cap with its compare-URL cap
// notice on overflow — and cannot drift. The tool-name-bearing transition line is a
// navigational ANCHOR, so it is bold when color is enabled (bold, NOT bold-cyan
// — bold-cyan is reserved for the per-tool header). Displayed versions are the
// normalized (v-stripped) forms carried on the bump. On a non-color stream the
// `→`/`—`/`…` glyphs ASCII-degrade to `->`/`--`/`...` and no ANSI is emitted
// (color threaded in from runUpdate; per-tool-output-separation spec).
//
// A tool whose fetch is unavailable (or whose range holds zero matching releases)
// degrades to `{tool} {old} → {new} — see {compareURL}` (Constitution V; no
// bodies exist to inline). The digest is presentation-only — it NEVER changes the
// process exit code, and a fetch failure here is silent beyond the fallback line.
func printUpdateDigest(ctx context.Context, w io.Writer, bumps []versionBump, color bool) {
	if len(bumps) == 0 {
		return
	}

	reqs := make([]changelog.RangeReq, len(bumps))
	for i, b := range bumps {
		reqs[i] = changelog.RangeReq{Tool: b.tool, Repo: b.repo, Old: b.old, New: b.new}
	}
	results := changelog.FetchAll(ctx, reqs)

	arr := arrow(color)

	fmt.Fprintln(w)
	fmt.Fprintln(w, digestHeader)

	for i, res := range results {
		// Blank line between tool blocks (mirroring runChangelog's per-tool
		// separation) so tools are never separated more weakly than the releases
		// within one tool.
		if i > 0 {
			fmt.Fprintln(w)
		}
		if res.Unavailable || len(res.Releases) == 0 {
			// Degrade: a fetch failure OR a tag-scheme mismatch that left zero
			// matching releases → point at the compare URL, no body lines. The
			// whole transition-plus-fallback line is a bold anchor like the
			// available branch.
			fmt.Fprintln(w, bold(color, fmt.Sprintf("%s%s %s %s %s %s see %s",
				digestToolIndent, res.Tool, res.Old, arr, res.New, dash(color), changelog.CompareURL(res.Repo, res.Old, res.New))))
			continue
		}
		n := len(res.Releases)
		// The per-tool transition line CARRIES the tool name (unlike `shll
		// changelog`, whose tool name lives in the printToolHeader above the body).
		fmt.Fprintln(w, bold(color, fmt.Sprintf("%s%s %s %s %s (%d release%s)",
			digestToolIndent, res.Tool, res.Old, arr, res.New, n, plural(n))))
		// Full release notes inline, in the shared `shll changelog` format.
		renderReleases(w, res, color)
	}
}
