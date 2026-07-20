package main

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

// yesFlag is the bool flag (short -y) on `shll uninstall` that bypasses the
// removal-plan confirmation prompt (the scripting path). Named constant per
// code-quality.md (no magic strings).
const yesFlag = "yes"

// yesFlagShorthand is the single-letter shorthand for --yes.
const yesFlagShorthand = "y"

// yesFlagUsage is the cobra usage string for --yes/-y.
const yesFlagUsage = "skip the confirmation prompt (assume yes — for scripting)"

// uninstallProceedPrompt is the confirmation prompt printed after the removal plan
// when neither --yes nor --dry-run was given. Only an explicit affirmative proceeds.
const uninstallProceedPrompt = "Proceed? [y/N] "

// uninstallAbortedMsg is printed to stdout when the user declines the prompt (or
// supplies anything other than an affirmative). The run then exits 0 with no write.
const uninstallAbortedMsg = "Aborted — nothing was uninstalled."

// uninstallNoTTYHint is the stderr hint printed when stdin is not a TTY and --yes
// was not passed: shll cannot read an interactive answer, so it refuses rather than
// removing without consent (fail-safe for pipes/CI).
const uninstallNoTTYHint = "shll uninstall: refusing to uninstall without confirmation on a non-interactive stdin — pass --yes to proceed"

// uninstallPlanHeader precedes the removal plan (the per-tool name/formula/version
// rows) printed before the confirmation prompt.
const uninstallPlanHeader = "The following will be uninstalled:"

// uninstallNothingMsg is the nothing-to-do message for `shll uninstall` — printed when
// the actionable set is empty (an empty roster, or every named target already gone).
// Named per code-quality.md (no magic strings), mirroring install.go's allInstalledMsg.
const uninstallNothingMsg = "Nothing to uninstall."

// notBrewManagedFmt is the stderr error when `shll uninstall shll` is asked to remove
// a shll that was not installed via brew (a `go install` / local-build binary — there
// is no brew keg to uninstall). Takes the target name. Mirrors update.go's self-upgrade
// brew-managed gating.
const notBrewManagedFmt = "shll uninstall: %s: not brew-managed"

// shllFarewellFmt is the farewell note printed after shll uninstalls itself. The
// running process keeps working (unix unlink semantics — the mapped image survives);
// this points at the reinstall path. Takes the reinstall command.
const shllFarewellFmt = "shll has been uninstalled. Reinstall any time with: %s"

// runKitDaemonStopHintFmt is the print-only post-run hint shown when run-kit was
// removed: brew uninstall does not stop a running run-kit daemon (Constitution III —
// shll never stops another tool's process; it only prints the hint). Takes the tool name.
const runKitDaemonStopHintFmt = "note: a running %[1]s daemon (if any) was not stopped — run '%[1]s serve --stop' to stop it"

// shellUnwireHint is the print-only post-run hint shown when shell-integrated tools
// were removed roster-wide: their rc-file eval block is not touched by brew uninstall
// (Constitution III — shll never edits the rc file here; that is `shll shell-setup
// --uninstall`'s job). Print-only.
const shellUnwireHint = "note: shell integration was not removed from your rc file — run 'shll shell-setup --uninstall' to remove the shll block"

// uninstallTarget is one actionable tool in a `shll uninstall` run. self marks
// the shll-self target (removed last, with a farewell note). Every target is a
// single `brew uninstall <formula>`. version is the installed version captured
// from the same probe, shown in the removal plan.
type uninstallTarget struct {
	tool    Tool
	version string
	self    bool
}

func newUninstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "uninstall [tool...]",
		Short: "brew uninstall shll tools (a clean-slate repair path)",
		Long: `Uninstall shll toolkit tools via Homebrew — the clean-slate repair path
that pairs with ` + "`shll install`" + `.

With no arguments, shll uninstall removes every INSTALLED roster tool
(` + "`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit`" + `) in reverse-roster order
(dependents before leaves). Tools that are not installed are skipped silently —
uninstall is idempotent and its goal state ("gone") is a success even when a tool
was already absent. shll itself is NOT part of the no-args sweep.

Pass one or more tool names to uninstall only that subset (valid targets: shll, wt,
idea, tu, run-kit, hop, fab-kit; the legacy alias ` + "`rk`" + ` still resolves to
run-kit). ` + "`shll uninstall shll`" + ` is legal and explicit-only — it removes shll
itself (last, after the roster), and only when shll was installed via brew. The
running process keeps working; a farewell note points at the reinstall command.

By default shll uninstall prints the removal plan and asks for confirmation
(` + "`Proceed? [y/N]`" + `). Pass ` + "`--yes`" + ` (or ` + "`-y`" + `) to skip the prompt.
On a non-interactive stdin (a pipe / CI) without ` + "`--yes`" + `, shll uninstall
refuses rather than removing without consent. Pass ` + "`--dry-run`" + ` to preview the
exact brew commands without removing anything.

shll uninstall does NOT untap sahil87/tap, revoke trust, purge tool state/config, or
stop running processes (it prints hints for the daemon and rc-file cleanup instead).`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool(dryRunFlag)
			yes, _ := cmd.Flags().GetBool(yesFlag)
			return runUninstall(cmd.Context(), cmd.InOrStdin(), cmd.OutOrStdout(), cmd.ErrOrStderr(), dryRun, yes, args)
		},
	}
	cmd.Flags().Bool(dryRunFlag, false, dryRunFlagUsage)
	cmd.Flags().BoolP(yesFlag, yesFlagShorthand, false, yesFlagUsage)
	return cmd
}

// runUninstall is the implementation seam for `shll uninstall`. Extracted from the
// cobra factory so uninstall_test.go can drive it with bytes.Buffer writers, an
// injected stdin io.Reader, and a fake proc.Runner.
//
// Behavior:
//   - Resolve targets up front via resolveTargets(args, true) — unknown → stderr +
//     errSilent, no brew side effect. allowShll=true: `shll uninstall shll` is legal
//     (explicit-only). Empty args = the whole-roster sweep (shll-self excluded).
//   - brew missing → stderr hint, errSilent (exit 1).
//   - Build the ACTIONABLE set: probe each considered tool. A NAMED-but-not-installed
//     target reports `not installed` and is NOT an error (repair-path semantics —
//     absence is the goal state). The set is ordered REVERSE-roster (dependents
//     first); shll-self is last.
//   - Confirmation gate (unless --yes or --dry-run): print the removal plan then prompt
//     `Proceed? [y/N]`. Non-TTY stdin without --yes refuses (fail-safe). A non-affirmative
//     answer aborts at exit 0 with no write.
//   - --dry-run previews the exact `brew uninstall` commands and exits 0 with no write,
//     bypassing the confirmation gate (a preview mutates nothing).
//   - Best-effort removal loop: a per-tool failure is recorded and the loop continues;
//     the overall exit code reflects whether any failed. Skips are not failures.
//   - Post-run hints (print-only, Constitution III): run-kit daemon stop; rc-file unwire
//     when shell-integrated tools were removed roster-wide.
//
// stdin is the reader the confirmation prompt reads from (cmd.InOrStdin() in
// production; a strings.Reader/bytes.Buffer in tests). args are the positional
// tool-name targets.
func runUninstall(ctx context.Context, stdin io.Reader, stdout, stderr io.Writer, dryRun, yes bool, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Resolve the subset UP FRONT — before hasBrew and any probe — so an unknown
	// target fails loudly with no brew side effect. allowShll=true: shll itself is a
	// valid uninstall target (explicit-only). Empty args yields an empty selection and
	// the whole-roster sweep below.
	subset := len(args) > 0
	selected, selfSelected, aliased, err := resolveTargets(args, true)
	if err != nil {
		fmt.Fprintf(stderr, "shll uninstall: %v\n", err)
		return errSilent
	}

	if !hasBrew(ctx) {
		fmt.Fprintln(stderr, uninstallBrewMissingHint)
		return errSilent
	}

	// Legacy-alias notice (e.g. `shll uninstall rk` → run-kit), before any framing.
	// Shared wording with update/install via printAliasNotices.
	printAliasNotices(stdout, aliased)

	// The roster to consider: the full Roster for a whole-roster sweep, or just the
	// named subset (resolveTargets returns selected in roster order). shll-self is
	// handled separately below — it is NEVER in the roster (Constitution III).
	consider := Roster
	if subset {
		consider = selected
	}

	// rosterWide reports whether this run addresses the ENTIRE roster — the no-args sweep
	// (consider == Roster) OR a named sweep that listed all six roster tools. selected is
	// a de-duplicated roster-ordered set, so full coverage is exactly len(consider) ==
	// len(Roster). It keys the rc-unwire hint (a partial subset may leave other integrated
	// tools wired), and unlike !subset it correctly counts the named-all case.
	rosterWide := len(consider) == len(Roster)

	// Build the actionable set (tools that are actually present) plus the skip lines
	// for named-but-not-installed targets (repair-path: reported, not an error). The
	// probes are reads, so they run in dry-run too — only the writes are skipped.
	var actionable []uninstallTarget
	var skipped []string // names reported `not installed`
	for _, t := range consider {
		if installed, version := probeInstalledVersion(ctx, t.Formula); installed {
			actionable = append(actionable, uninstallTarget{tool: t, version: version})
		} else {
			skipped = append(skipped, t.Name)
		}
	}

	// shll-self: explicit-only (never in the no-args sweep). Gate on brew management —
	// a `go install`/local-build shll has no brew keg to uninstall (same fact update.go's
	// self-upgrade path keys on). Not brew-managed → hard error (the user explicitly asked
	// for it). Appended LAST so it is processed after every roster tool.
	if selfSelected {
		installed, version := probeInstalledVersion(ctx, shllFormula)
		if !installed {
			fmt.Fprintf(stderr, notBrewManagedFmt+"\n", shllTargetToken)
			return errSilent
		}
		actionable = append(actionable, uninstallTarget{tool: shllSelf, version: version, self: true})
	}

	// Reverse-roster order: dependents before leaves (the mirror of install's
	// leaves-first coherence). The actionable set was built in roster order (leaves
	// first) with shll-self appended last; reversing puts dependents first AND keeps
	// shll-self last only if we reverse the roster part alone. So reverse just the
	// roster tools, then re-append shll-self.
	actionable = reverseRosterOrder(actionable)

	// Report the graceful skips (named-but-not-installed / not-in-a-sweep). Repair-path
	// semantics: absence is the goal state, so this is NOT an error and does not affect
	// the exit code. Printed before the plan/loop so the user sees the full picture.
	for _, name := range skipped {
		fmt.Fprintf(stdout, "%s: %s\n", name, notInstalledLabel)
	}

	if len(actionable) == 0 {
		// Nothing to remove — either an empty roster or every named target already gone.
		fmt.Fprintln(stdout, uninstallNothingMsg)
		return nil
	}

	// Dry-run: the probes have run (reads); preview the exact commands the real run
	// WOULD execute and exit 0 with NO write. Bypasses the confirmation gate — a preview
	// mutates nothing. Sourced from the single-source-of-truth argv builder so the
	// preview cannot drift from the live run.
	if dryRun {
		rows := make([]previewRow, 0, len(actionable))
		for _, a := range actionable {
			rows = append(rows, previewRowsFor(a)...)
		}
		printUninstallPreview(stdout, rows)
		return nil
	}

	// Confirmation gate (unless --yes). Print the removal plan, then prompt. A non-TTY
	// stdin without --yes refuses (fail-safe). A non-affirmative answer aborts, exit 0.
	if !yes {
		printRemovalPlan(stdout, actionable)
		if !stdinIsTTY(stdin) {
			fmt.Fprintln(stderr, uninstallNoTTYHint)
			return errSilent
		}
		if !confirmProceed(stdin, stdout) {
			fmt.Fprintln(stdout, uninstallAbortedMsg)
			return nil
		}
	}

	// Per-tool boundary framing. The color decision is computed once against stdout and
	// reused for every header and the tail. succeeded feeds the summary tail by exit code
	// only. M (the counter denominator) is the actionable count — known up front.
	color := colorEnabled(stdout)
	total := len(actionable)
	succeeded := 0

	// Wall-clock start for the run-duration suffix, from the injectable nowFunc seam
	// (clock.go). Captured after the short-circuits/gate so it measures only the removal
	// phase the tail summarizes.
	start := nowFunc()

	anyFailed := false
	// runKitName is the display name of the run-kit tool once it is SUCCESSFULLY removed
	// (empty otherwise). The daemon-stop hint is keyed on the roster entry by NAME —
	// matched against the runKitToolName named constant (no magic string) — and
	// success-gated: it fires only when the run-kit roster entry was actually removed.
	// shellIntegratedRemoved tracks whether any SUCCESSFULLY removed tool carries shell
	// integration — the rc-unwire hint is success-gated on it (mirrors runKitName's
	// success gating), never fired for a merely-attempted-but-failed removal.
	runKitName := ""
	shellIntegratedRemoved := false
	for i, a := range actionable {
		// Section spacing: a blank line precedes every header EXCEPT the first.
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		printToolHeader(stdout, a.tool.Name, i+1, total, color)

		var failed bool
		if a.self {
			failed = uninstallOne(ctx, stderr, a.tool.Name, shllFormula)
			if !failed {
				fmt.Fprintf(stdout, shllFarewellFmt+"\n", argvString(brewBinary, "install", shllFormula))
			}
		} else {
			failed = uninstallOne(ctx, stderr, a.tool.Name, a.tool.Formula)
		}
		if failed {
			anyFailed = true
			continue
		}
		if a.tool.Name == runKitToolName {
			runKitName = a.tool.Name
		}
		// Success-gate the shell-integration signal on an actually-removed tool (shll-self
		// carries no ShellInit, so it never trips this).
		if !a.self && len(a.tool.ShellInit) > 0 {
			shellIntegratedRemoved = true
		}
		succeeded++
	}

	// Summary tail by exit-code counts plus the wall-clock run duration. A blank line
	// precedes it (same section-spacing rule as the headers). Presentation only — does
	// not influence the exit code.
	fmt.Fprintln(stdout)
	printSummaryTail(stdout, succeeded, total, nowFunc().Sub(start), color)

	// Post-run hints — PRINT ONLY (Constitution III — shll never stops a daemon or edits
	// the rc file here). run-kit daemon stop when run-kit was removed; rc-file unwire when
	// shell-integrated tools were removed ROSTER-WIDE. "Roster-wide" keys on coverage of
	// the roster SET — a no-args sweep OR a NAMED full-roster sweep (all six roster tools
	// listed explicitly) both qualify — not on !subset, which would miss the named-all
	// case. Scoping to roster-wide avoids a misleading rc-unwiring nudge on a partial
	// subset that may leave other integrated tools present and still wired. Both hints are
	// success-gated (they fire only on an actually-removed tool, never a failed attempt).
	if runKitName != "" {
		fmt.Fprintf(stdout, runKitDaemonStopHintFmt+"\n", runKitName)
	}
	if rosterWide && shellIntegratedRemoved {
		fmt.Fprintln(stdout, shellUnwireHint)
	}

	if anyFailed {
		return errSilent
	}
	return nil
}

// reverseRosterOrder returns the actionable set reordered dependents-first (reverse
// roster), keeping any shll-self target LAST. The input is built in roster order
// (leaves first) with shll-self appended at the end; this reverses only the roster
// portion so leaves come last among the roster tools, then re-appends shll-self so it
// stays the final removal (the running orchestrator is removed after everything it
// might have managed). Deriving the reverse from the single Roster-order slice keeps
// one source of truth (no second hardcoded order to drift).
func reverseRosterOrder(actionable []uninstallTarget) []uninstallTarget {
	rosterPart := make([]uninstallTarget, 0, len(actionable))
	var selfPart []uninstallTarget
	for _, a := range actionable {
		if a.self {
			selfPart = append(selfPart, a)
			continue
		}
		rosterPart = append(rosterPart, a)
	}
	out := make([]uninstallTarget, 0, len(actionable))
	for i := len(rosterPart) - 1; i >= 0; i-- {
		out = append(out, rosterPart[i])
	}
	return append(out, selfPart...)
}

// previewRowsFor returns the dry-run preview row(s) for an actionable target — the exact
// argv the live run would issue, so the preview cannot drift. Every target is a single
// `brew uninstall <formula>` row; for shll-self it is `brew uninstall sahil87/tap/shll`.
func previewRowsFor(a uninstallTarget) []previewRow {
	if a.self {
		return []previewRow{{label: a.tool.Name, cmd: argvString(brewUninstallArgv(shllFormula)...)}}
	}
	return []previewRow{{label: a.tool.Name, cmd: argvString(brewUninstallArgv(a.tool.Formula)...)}}
}

// brewUninstallArgv builds the `brew uninstall <formula>` argv. Named single-source
// builder so the live run and dry-run preview stay in lockstep (mirrors upgradeArgv).
func brewUninstallArgv(formula string) []string {
	return []string{brewBinary, "uninstall", formula}
}

// uninstallOne runs a single `brew uninstall <formula>` foregrounded and reports
// whether it FAILED (a transport error OR a non-zero exit — proc.RunForeground
// returns (code, nil) on a non-zero exit, so both are checked). A transport error is
// surfaced to stderr; a non-zero exit is left to brew's own foregrounded output. name
// is the display name for the stderr line. The argv is sourced from brewUninstallArgv
// — the SAME single-source-of-truth builder the dry-run preview renders — so the live
// run and the preview cannot drift (mirrors upgradeTool threading upgradeArgv). Routed
// through internal/proc (Constitution I).
func uninstallOne(ctx context.Context, stderr io.Writer, name, formula string) (failed bool) {
	argv := brewUninstallArgv(formula)
	code, err := proc.RunForeground(ctx, argv[0], argv[1:]...)
	if err != nil {
		fmt.Fprintf(stderr, "shll uninstall: %s: %v\n", name, err)
		return true
	}
	return code != 0
}

// printRemovalPlan prints the removal plan shown before the confirmation prompt: a
// header then one aligned row per actionable tool (name, formula, installed version),
// so the user sees exactly what `Proceed? [y/N]` will act on. Presentation-only.
func printRemovalPlan(stdout io.Writer, actionable []uninstallTarget) {
	fmt.Fprintln(stdout, uninstallPlanHeader)
	width := 0
	for _, a := range actionable {
		if len(a.tool.Name) > width {
			width = len(a.tool.Name)
		}
	}
	for _, a := range actionable {
		formula := a.tool.Formula
		if a.self {
			formula = shllFormula
		}
		pad := strings.Repeat(" ", width-len(a.tool.Name))
		version := a.version
		if version == "" {
			version = "?"
		}
		fmt.Fprintf(stdout, "%s%s%s%s%s  (%s)\n", previewIndent, a.tool.Name, pad, previewGap, formula, version)
	}
}

// confirmProceed prints the prompt and reads one line from stdin, returning true only
// on an explicit case-insensitive affirmative (`y` / `yes`). Anything else — a
// negative, whitespace, EOF, or a malformed answer — is treated as "no" (the fail-safe
// default matching the `[y/N]` capital-N convention). stdin is the injected reader
// (the TTY check is the caller's responsibility, done before this call).
func confirmProceed(stdin io.Reader, stdout io.Writer) bool {
	fmt.Fprint(stdout, uninstallProceedPrompt)
	reader := bufio.NewReader(stdin)
	line, _ := reader.ReadString('\n') // EOF with no newline still returns the read bytes.
	answer := strings.ToLower(strings.TrimSpace(line))
	return answer == "y" || answer == "yes"
}
