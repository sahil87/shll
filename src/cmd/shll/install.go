package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [tool...]",
		Short: "brew install every shll tool that isn't already installed",
		Long: `Install every roster tool that isn't already installed.

shll install iterates the roster (` + "`run-kit`, `rk-desktop`, `fab-kit`, `wt`, `idea`, `tu`, `hop`" + `).
Brew-managed tools install via ` + "`brew install sahil87/tap/<formula>`" + `;
rk-desktop is not a brew formula — it delegates to ` + "`rk desktop install`" + `
(managed by run-kit, so it is actionable only when ` + "`rk`" + ` is installed
and the platform supports it; otherwise it is skipped with a note, never a
failure — on a targeted ` + "`shll install rk-desktop`" + ` the refusal is
printed explicitly). Tools that are already installed are skipped silently —
the command is idempotent and safe to re-run. Brew's progress output streams
directly to your terminal.

With no arguments, shll install processes the whole roster as above. Pass one or
more tool names to install only that subset (valid targets: run-kit, rk-desktop,
fab-kit, wt, idea, tu, hop; the legacy alias ` + "`rk`" + ` still resolves to run-kit) — e.g.
` + "`shll install hop wt`" + `. The subset is processed in roster order
regardless of the order given; an unknown name is a hard error. Unlike
` + "`shll update`" + `, ` + "`shll`" + ` itself is NOT a valid install target — you cannot
brew-install the running orchestrator.

By default, shll install records per-formula Homebrew trust before each install —
it runs ` + "`brew trust --formula sahil87/tap/<formula>`" + ` for each tool in the
install set first. Homebrew 6.0 makes tap-trust a hard install requirement, and a
binary-download formula runs a sandboxed ` + "`def install`" + ` that requires a real
trust record, so this is what lets the install actually proceed. ` + "`brew trust`" + ` is
idempotent, so re-runs stay clean. Pass ` + "`--no-trust`" + ` to skip the trust step
(for users who manage trust themselves). If your Homebrew is too old to ship
` + "`brew trust`" + `, the trust step is skipped gracefully and the install proceeds.

After the install outcome, shll install also wires the machine automatically. It
runs the equivalent of ` + "`shll setup shell`" + ` (adds the
` + "`eval \"$(shll shell-init <shell>)\"`" + ` line to your rc file — sentinel-managed and
idempotent, so re-runs are no-ops), then ` + "`shll setup agent --yes`" + ` (places the
shll-toolkit skill for agent harnesses and delegates run-kit's dashboard hooks,
forwarding --yes so nothing can prompt on an unattended run). Both steps are
best-effort: a failure warns and prints the step's manual nudge instead, and
never changes the install's exit code. Opt out with ` + "`--no-shell-setup`" + ` (e.g.
dotfile-manager users) and/or ` + "`--no-agent-setup`" + `. Neither step runs under
` + "`--dry-run`" + `. After a fresh wire, restart your shell or run: exec $SHELL.

shll install does NOT upgrade already-installed tools. Use ` + "`shll update`" + `
for that.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool(dryRunFlag)
			noTrust, _ := cmd.Flags().GetBool(noTrustFlag)
			noShellSetup, _ := cmd.Flags().GetBool(noShellSetupFlag)
			noAgentSetup, _ := cmd.Flags().GetBool(noAgentSetupFlag)
			return runInstall(cmd.Context(), os.Getenv, cmd.OutOrStdout(), cmd.ErrOrStderr(), dryRun, noTrust, noShellSetup, noAgentSetup, args)
		},
	}
	cmd.Flags().Bool(dryRunFlag, false, dryRunFlagUsage)
	cmd.Flags().Bool(noTrustFlag, false, noTrustFlagUsage)
	cmd.Flags().Bool(noShellSetupFlag, false, noShellSetupFlagUsage)
	cmd.Flags().Bool(noAgentSetupFlag, false, noAgentSetupFlagUsage)
	return cmd
}

// noTrustFlag is the bool flag on `shll install` that skips the per-formula trust
// step (`brew trust --formula sahil87/tap/<formula>`) before each install. Named
// constant per code-quality.md (no magic strings).
const noTrustFlag = "no-trust"

// noTrustFlagUsage is the cobra usage string for --no-trust.
const noTrustFlagUsage = "skip recording per-formula Homebrew trust before installing (manage trust yourself)"

// noShellSetupFlag is the bool flag on `shll install` that opts out of the automatic
// shell-setup step at the end of install (for dotfile-manager users who wire their rc
// files themselves). Named constant per code-quality.md (mirrors noTrustFlag).
const noShellSetupFlag = "no-shell-setup"

// noShellSetupFlagUsage is the cobra usage string for --no-shell-setup.
const noShellSetupFlagUsage = "skip the automatic shell-setup step at the end of install (wire your rc file yourself)"

// noAgentSetupFlag is the bool flag on `shll install` that opts out of the automatic
// agent-setup step at the end of install. Named constant per code-quality.md
// (mirrors noTrustFlag).
const noAgentSetupFlag = "no-agent-setup"

// noAgentSetupFlagUsage is the cobra usage string for --no-agent-setup.
const noAgentSetupFlagUsage = "skip the automatic agent-setup step at the end of install (wire agent harnesses yourself)"

// runInstall is the implementation seam for `shll install`. Extracted from
// the cobra factory so install_test.go can drive it with bytes.Buffer writers
// and a fake proc.Runner.
//
// Behavior:
//   - brew missing → stderr hint, errSilent (exit 1).
//   - For each roster tool in order, skip if already installed; else (unless
//     noTrust) record per-formula trust via `brew trust --formula <formula>`,
//     then run `brew install sahil87/tap/<formula>` foregrounded.
//   - Best-effort across the roster: a per-tool install failure does not abort
//     the loop. The overall exit code reflects whether any failed.
//   - If everything is already installed, write a one-line note to stdout and
//     exit 0 — mirrors `shll update`'s "nothing to do" UX.
//
// Note: no `brew update --quiet` — `brew install` resolves the formula via
// the tap directly and doesn't need a metadata refresh as a precondition.
//
// noTrust skips the per-formula trust step entirely. When false (the default),
// each missing tool's formula is trusted via `brewTrustFormula` immediately
// before its install — gated by `brewTrustAvailable` so a brew too old to ship
// `brew trust` degrades silently (Constitution V; pre-6.0 brew doesn't require
// trust anyway). A trust failure warns to stderr and proceeds to the install
// attempt rather than aborting — and does NOT set the run's failure flag (the
// install's own exit code is the authority).
//
// args are the positional tool-name targets. Empty args = the whole-roster run
// (unchanged behavior). One or more args restrict the run to that validated subset
// (valid targets: the Roster names ONLY — shll is rejected, since the running
// orchestrator cannot be brew-installed). An unknown name is a hard error reported
// before any work; a named tool that is already installed is filtered out of the
// install set (the idempotent skip, same as the whole-roster behavior).
//
// env is the environment lookup threaded into the post-install setup steps
// (runPostInstallSetup): the shell-setup gate (resolveWiringFact), the shell/rc
// resolution for the auto-run, and agent-setup's skill-target derivation.
// Production passes os.Getenv; tests pass a map-backed func pointing at a
// t.TempDir() rc file and HOME, mirroring runDoctor's established seam.
//
// noShellSetup / noAgentSetup are the --no-shell-setup / --no-agent-setup opt-outs:
// when set, the matching post-install auto-run step is skipped and its nudge line
// prints instead (see runPostInstallSetup).
func runInstall(ctx context.Context, env func(string) string, stdout, stderr io.Writer, dryRun, noTrust, noShellSetup, noAgentSetup bool, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}

	// Resolve the subset UP FRONT — before hasBrew and any probe — so an unknown
	// target (including `shll`, which is rejected here) fails loudly with no brew
	// side effect. allowShll=false: shll is not a valid install target. Empty args
	// yields an empty selection and the whole-roster walk below.
	subset := len(args) > 0
	selected, _, aliased, err := resolveTargets(args, false)
	if err != nil {
		fmt.Fprintf(stderr, "shll install: %v\n", err)
		return errSilent
	}

	if !hasBrew(ctx) {
		fmt.Fprintln(stderr, installBrewMissingHint)
		return errSilent
	}

	// Legacy-alias notice (e.g. `shll install rk` → run-kit), before any roster
	// framing. Shared wording with `shll update` via printAliasNotices.
	printAliasNotices(stdout, aliased)

	// shll-first informational line: shll is the manager-member of the toolkit and
	// is always already present (it is the running orchestrator), so it leads the
	// output for family-discoverability — but it is INFORMATIONAL only, never a
	// `brew install` action (you cannot brew-install the running binary; shll is
	// also rejected as an explicit install target above). Printed once, before any
	// roster framing, on every path that reaches the install decision (nothing-to-
	// do, dry-run preview, and the install loop).
	fmt.Fprintln(stdout, shllSelfInstallNote)

	// The roster to consider: the full Roster for a whole-roster run, or just the
	// named subset (in roster order — resolveTargets returns selected in roster
	// order) for a subset run.
	consider := Roster
	if subset {
		consider = selected
	}

	// Collect the tools that are ACTIONABLE (not yet present), keyed by the
	// install dispatch each will take. Brew-managed tools probe via
	// `brew list --versions`; delegated (non-brew) tools via their Probe spec
	// (rk-desktop's `rk desktop status`). A delegated tool whose probe invocation
	// itself is REFUSED by the platform (run-kit's errDesktopMacOnly) is never
	// actionable: on a whole-roster run it is a skip-with-note (exit unaffected);
	// on a targeted run the refusal is named explicitly — neither is a failure.
	// The probes are reads, so they run in dry-run too — only the writes are
	// skipped. A named-but-already-installed target is filtered out here
	// (idempotent skip).
	var missingBrew, missingDelegated []Tool
	var skippedNotes []string
	for _, t := range consider {
		if !t.brewManaged() {
			state, note := delegatedInstallState(ctx, t)
			switch state {
			case delegatedAbsent:
				missingDelegated = append(missingDelegated, t)
			case delegatedRefused, delegatedUnprobed:
				// Platform refusal (or an unprobeable prerequisite — rk missing
				// or the status call failing) → skip-with-note, never an install
				// attempt and never a failure.
				skippedNotes = append(skippedNotes, note)
			}
			continue
		}
		if !isInstalled(ctx, t.Formula) {
			missingBrew = append(missingBrew, t)
		}
	}

	// Report the delegated skips before any framing (the same posture as
	// uninstall's graceful-skip lines): the user sees the full picture, and the
	// notes name the cause (unsupported platform / prerequisite missing).
	for _, note := range skippedNotes {
		fmt.Fprintln(stdout, note)
	}

	if len(missingBrew) == 0 && len(missingDelegated) == 0 {
		fmt.Fprintln(stdout, allInstalledMsg)
		// Short-circuit path: a re-runner who never wired their shell still gets
		// wired (the auto-run steps are idempotent, so this path is their exact
		// beneficiary). colorEnabled is computed here (the install-loop path
		// computes it once below for its own framing; this early-return path never
		// reaches that, so it needs its own decision). Suppressed under --dry-run:
		// the auto-run steps are writes and MUST NOT run on the preview path, and
		// this short-circuit precedes the dry-run branch, so gate on !dryRun here.
		if !dryRun {
			runPostInstallSetup(ctx, env, stdout, stderr, colorEnabled(stdout), noShellSetup, noAgentSetup)
		}
		return nil
	}

	// Dry-run: the probes have run (reads); now preview the exact commands the real
	// run WOULD execute and exit 0 with NO write. The preview lists only the missing
	// subset (actionable tools), in roster order.
	if dryRun {
		rows := make([]previewRow, 0, len(missingBrew)+len(missingDelegated))
		for _, t := range missingBrew {
			rows = append(rows, previewRow{label: t.Name, cmd: argvString(brewBinary, "install", t.Formula)})
		}
		for _, t := range missingDelegated {
			rows = append(rows, previewRow{label: t.Name, cmd: argvString(t.Install...)})
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
	total := len(missingBrew) + len(missingDelegated)
	succeeded := 0

	// Probe `brew trust` availability ONCE up front (a brew-wide capability, not a
	// per-tool fact). The per-formula trust step runs only when trust was requested
	// (default; --no-trust opts out) AND this brew ships `brew trust`. On a pre-6.0
	// brew the subcommand is absent, but trust isn't required there either, so
	// skipping it is safe (Constitution V — graceful degradation). Delegated tools
	// carry no formula, so the trust step never applies to them.
	trustEnabled := !noTrust && brewTrustAvailable(ctx)

	// Wall-clock start for the run-duration suffix in the summary tail, from the
	// injectable nowFunc seam (clock.go). Captured after the short-circuit/dry-run
	// returns so it measures only the install phase the tail summarizes.
	start := nowFunc()

	// OSC 9;4 terminal progress + pinned status region, both tty-gated no-ops
	// elsewhere. Constructed only once the write phase begins — the
	// dry-run/short-circuit/pre-write error paths above construct nothing — and
	// removed/stopped via defer so EVERY post-construction exit clears the
	// terminal's progress state and restores the scroll region. Deferred LIFO
	// order runs stop() before remove() so the region is restored before the
	// progress state clears (mirrors update.go).
	progress := newProgressReporter(stderr, env)
	defer progress.remove()
	region := newStatusRegion(stdout)
	defer region.stop()
	region.start()

	// pos is the running 1-based header position across BOTH install phases
	// (brew-managed first, then delegated — the delegated tools sit behind their
	// runtime prerequisite in roster order, and run-kit's brew install just ran).
	// actionableNames is the flat list in the same order, so the pinned header
	// can look ahead one entry for the `· next:` clause.
	actionableNames := make([]string, 0, total)
	for _, t := range missingBrew {
		actionableNames = append(actionableNames, t.Name)
	}
	for _, t := range missingDelegated {
		actionableNames = append(actionableNames, t.Name)
	}
	nextName := func(pos int) string {
		if pos < len(actionableNames) {
			return actionableNames[pos]
		}
		return ""
	}
	pos := 0
	installHeader := func(name string) {
		pos++
		if pos > 1 {
			fmt.Fprintln(stdout)
		}
		printToolHeader(stdout, name, pos, total, color)
		// Pinned header (tty only): verb + tool + honest k/n + next-tool
		// lookahead, sharing the run's single color decision. A no-op off-tty.
		region.setHeader(statusHeaderText(installRegionVerb, name, pos, total, nextName(pos), color))
		// Determinate OSC 9;4 progress at the boundary, mirroring update.go.
		progress.set((pos - 1) * 100 / total)
	}

	// runChild runs one install-phase child (brew install / delegated argv)
	// via the shared streamed-tail helper (null stdin, live tee, region-mode
	// failure tail — see runStreamedChild).
	runChild := func(name string, argv ...string) (int, error) {
		return runStreamedChild(ctx, stdout, stderr, region, name, argv...)
	}

	anyFailed := false
	for _, t := range missingBrew {
		installHeader(t.Name)

		// Record per-formula trust before the install (default; --no-trust or an
		// older brew lacking `brew trust` skips this). Homebrew 6.0 makes tap-trust
		// a hard install requirement, and a binary-download formula runs a sandboxed
		// `def install` that needs a real trust record — so this is what unblocks
		// the install. `brew trust` is idempotent. A trust failure is best-effort:
		// warn and proceed to the install attempt (which will surface brew's own
		// untrusted-tap error if trust truly didn't land), and do NOT set anyFailed —
		// the install's exit code is the authority on whether this tool succeeded.
		if trustEnabled {
			if code, terr := brewTrustFormula(ctx, stdout, stderr, region, t.Formula); terr != nil {
				fmt.Fprintf(stderr, "shll install: %s: trust step failed: %v (continuing to install)\n", t.Name, terr)
			} else if code != 0 {
				fmt.Fprintf(stderr, "shll install: %s: trust step exited %d (continuing to install)\n", t.Name, code)
			}
		}

		code, err := runChild(t.Name, brewBinary, "install", t.Formula)
		if err != nil {
			fmt.Fprintf(stderr, "shll install: %s: %v\n", t.Name, err)
			anyFailed = true
			progress.errorState(pos * 100 / total)
			continue
		}
		if code != 0 {
			anyFailed = true
			progress.errorState(pos * 100 / total)
			continue
		}
		succeeded++
	}

	// Delegated (non-brew) phase: after every brew install, so a delegated tool's
	// runtime prerequisite (run-kit for rk-desktop) is freshly installed when the
	// delegation runs. Per tool, RE-PROBE first: on a whole-roster run a failed
	// prerequisite install (e.g. run-kit failed above) cascades to a skip-with-note
	// rather than a doomed delegation attempt; the same re-probe also catches a
	// platform that refuses only at install time. A refusal/unsuccessful probe is
	// never a failure; the delegation's own non-zero exit IS. Skip notes print
	// inline, before the next tool's header.
	for _, t := range missingDelegated {
		if !subset {
			state, note := delegatedInstallState(ctx, t)
			if state != delegatedAbsent {
				if pos > 0 {
					fmt.Fprintln(stdout)
				}
				fmt.Fprintln(stdout, note)
				continue
			}
		}
		installHeader(t.Name)
		code, err := runChild(t.Name, t.Install...)
		if err != nil {
			fmt.Fprintf(stderr, "shll install: %s: %v\n", t.Name, err)
			anyFailed = true
			progress.errorState(pos * 100 / total)
			continue
		}
		if code != 0 {
			anyFailed = true
			progress.errorState(pos * 100 / total)
			continue
		}
		succeeded++
	}

	// Progress tail: the bar reads complete — error-colored when any tool
	// failed — while the summary tail and post-install setup run; the deferred
	// remove clears it at exit (mirrors update.go).
	if anyFailed {
		progress.errorState(100)
	} else {
		progress.set(100)
	}

	// Summary tail by exit-code counts plus the wall-clock run duration. A blank
	// line precedes it (same section-spacing rule as the headers). Printed only
	// after the install loop ran (the all-already-installed short-circuit returned
	// earlier with no header and no tail). Presentation only — does not influence
	// the exit code.
	fmt.Fprintln(stdout)
	printSummaryTail(stdout, succeeded, total, nowFunc().Sub(start), color)

	// Post-install auto-run steps (shell-setup, then agent-setup) plus the adapted
	// "Next steps" block. Runs after the summary tail, reusing the loop's single
	// `color` decision. The steps are best-effort: a failure warns to stderr and
	// falls back to that step's nudge line — the install's own exit code (anyFailed)
	// stays the sole authority. Never reached by the dry-run / brew-missing /
	// unknown-target early returns above, so the write steps cannot fire on a
	// preview run.
	runPostInstallSetup(ctx, env, stdout, stderr, color, noShellSetup, noAgentSetup)

	if anyFailed {
		return errSilent
	}
	return nil
}

// delegatedInstallState classifies a delegated (non-brew) tool's install state
// for `shll install`, with the note to print when the tool is not actionable.
// It runs the tool's Probe spec via proc.RunCaptured (both streams captured so
// run-kit's platform refusal — printed to stderr by cobra — is detectable) and
// maps the outcome:
//
//   - transport error (e.g. the `rk` binary missing) → delegatedUnprobed, with
//     a prerequisite-missing note;
//   - non-zero exit carrying the unsupported-platform refusal →
//     delegatedRefused, with the refusal message as the note;
//   - any other non-zero exit → delegatedUnprobed, with a generic skip note;
//   - exit 0 → the `Installed:` line decides: delegatedAbsent (actionable) or
//     delegatedPresent (idempotent skip).
//
// The note embeds the tool name and cause so whole-roster runs read as a
// skip-with-note (exit unaffected) while a targeted run surfaces the same text
// as its explicit answer. Constitution V — graceful degradation; Constitution
// I — via internal/proc.
func delegatedInstallState(ctx context.Context, t Tool) (state delegatedState, note string) {
	stdout, stderrOut, code, err := proc.RunCaptured(ctx, t.Probe.Argv[0], t.Probe.Argv[1:]...)
	if err != nil {
		return delegatedUnprobed, fmt.Sprintf(delegatedSkipPrereqFmt, t.Name, err)
	}
	if code != 0 {
		if isRkDesktopRefusal(stderrOut) || isRkDesktopRefusal(stdout) {
			return delegatedRefused, fmt.Sprintf(delegatedSkipRefusalFmt, t.Name, strings.TrimSpace(string(firstLine(stderrOut, stdout))))
		}
		return delegatedUnprobed, fmt.Sprintf(delegatedSkipPrereqFmt, t.Name, strings.TrimSpace(string(firstLine(stderrOut, stdout))))
	}
	_, installed := parseProbeStatusLine(string(stdout), t.Probe)
	if installed {
		return delegatedPresent, ""
	}
	return delegatedAbsent, ""
}

// delegatedState is the three-way install-state classification for a delegated
// (non-brew) roster tool.
type delegatedState int

const (
	// delegatedAbsent — probe succeeded and reports not installed → actionable.
	delegatedAbsent delegatedState = iota
	// delegatedPresent — probe succeeded and reports installed → idempotent skip.
	delegatedPresent
	// delegatedRefused — the platform refuses the tool outright (run-kit's
	// errDesktopMacOnly) → skip-with-note, never a failure.
	delegatedRefused
	// delegatedUnprobed — the probe itself could not answer (prerequisite
	// binary missing, transport error, unexpected failure) → skip-with-note.
	delegatedUnprobed
)

// firstLine returns the first non-empty line of the first non-empty byte slice.
func firstLine(blobs ...[]byte) []byte {
	for _, b := range blobs {
		for _, line := range strings.Split(string(b), "\n") {
			if trimmed := strings.TrimSpace(line); trimmed != "" {
				return []byte(trimmed)
			}
		}
	}
	return nil
}

// delegatedSkipRefusalFmt is the skip-with-note line printed when a delegated
// tool's platform refuses it (e.g. rk-desktop on Linux). Takes (tool name,
// refusal message). Named per code-quality.md (no magic strings).
const delegatedSkipRefusalFmt = "note: %s skipped — %s"

// delegatedSkipPrereqFmt is the skip-with-note line printed when a delegated
// tool's prerequisite cannot answer the probe (e.g. `rk` not installed, or the
// probe failing for another reason). Takes (tool name, cause). On a
// whole-roster run after a failed run-kit install this reads as the cascade
// skip: rk-desktop is skipped because run-kit is unavailable.
const delegatedSkipPrereqFmt = "note: %s skipped — prerequisite unavailable (%s)"

// allInstalledMsg is the nothing-to-do message for `shll install` (every roster tool
// already installed). Shared by the normal short-circuit and the dry-run empty case so
// both read identically. Named per code-quality.md.
const allInstalledMsg = "All shll tools already installed."

// shllSelfInstallNote is the shll-first informational line `shll install` prepends.
// shll is the manager-member of the toolkit and is always already present (it is the
// running orchestrator), so the line is informational — NOT a brew install action.
// Named per code-quality.md (no magic strings).
const shllSelfInstallNote = "shll — already present / self-managed"

// installRegionVerb is the pinned-header verb for `shll install`'s status region
// (e.g. `Installing run-kit (2/7) · next: rk-desktop`). Named per code-quality.md.
const installRegionVerb = "Installing"

// Post-install "Next steps" strings (change 93r2; agent-setup graduation change
// agst; auto-run change gjhx). Each is a named constant per code-quality.md — the
// wording is part of the user contract, so it lives in one place (mirroring
// allInstalledMsg / shllSelfInstallNote and doctor's suggestNotWired).
//
//   - nextStepsHeader labels the block. The block prints only when at least one
//     line applies (the fully-wired happy path prints no header at all).
//   - shellSetupNudgeFmt is the shell-setup line; the single %s is the arrow glyph
//     (arrow(color) → `→` on a color TTY, `->` otherwise). Its wording tracks
//     doctor's suggestNotWired ("run 'shll setup shell' then 'exec $SHELL'"). It now
//     prints only when the auto shell-setup step was opted out of or failed (and
//     the resolveWiringFact gate is open).
//   - agentSetupNudgeFmt is the agent-setup line; the single %s is the arrow glyph.
//     It now prints only when the auto agent-setup step was opted out of or failed —
//     after a successful auto-run the "shll cannot cheaply know whether agent-setup
//     already ran" rationale no longer applies on this path.
//   - execShellReminderFmt is the reminder shown after a successful auto shell-setup
//     wire: a freshly wired rc file isn't loaded in the current shell (and under the
//     curl bootstrap the install process's parent shell dies anyway), so the user
//     must exec/restart to pick it up.
const (
	nextStepsHeader      = "Next steps:"
	shellSetupNudgeFmt   = "  %s shll setup shell    # wire shell integration into your rc file, then: exec $SHELL"
	agentSetupNudgeFmt   = "  %s shll setup agent    # optional, once per machine — wire agent harnesses (toolkit context + run-kit dashboard hooks)"
	execShellReminderFmt = "  %s exec $SHELL         # load the just-wired shll integration into your current shell (or open a new terminal)"
)

// Auto-run failure warnings (stderr). A failed post-install step never changes the
// install's exit code (the install outcome is the sole authority — the same posture
// as the trust step), so the warning says "continuing" and the step's nudge line is
// the printed fallback.
const (
	shellSetupAutoRunWarn = "shll install: automatic shell setup failed (continuing)"
	agentSetupAutoRunWarn = "shll install: automatic agent setup failed (continuing)"
)

// runPostInstallSetup runs the two post-install auto-run steps — shell-setup, then
// agent-setup — and renders the adapted "Next steps" block after them. It replaces
// the nudge-only printNextSteps (changes 93r2/agst): the steps now RUN instead of
// being nudged, and each nudge line survives only as the fallback for an opted-out
// or failed step.
//
// Step 1 — shell wiring. Gated by doctor's read-only resolveWiringFact(env)
// (Constitution III — one detection path):
//
//   - Unresolvable $SHELL or a corrupt open-without-close block → QUIET skip, no
//     nudge (the 93r2 quiet edge states: a nudge would dead-end; doctor owns the
//     corrupt-block diagnostic).
//   - Already wired → silent skip (no write, no reminder — idempotency makes the
//     auto-run a no-op, so there is nothing to announce).
//   - Unwired → auto-run in-process via the same write path the standalone command
//     uses: resolve the shell and rc path through the env seam (resolveShell /
//     resolveRcFile), then runShellSetupDefault. On success the shell-setup output
//     announces the wire and the execShellReminderFmt line is queued for the block.
//     On failure (e.g. the rc file does not exist — shell-setup never creates one)
//     the actionable error text goes to stderr with shellSetupAutoRunWarn and the
//     gated nudge is the fallback. The error is consumed, never propagated.
//   - --no-shell-setup → skip the auto-run; print the gated nudge instead.
//
// Step 2 — agent wiring. Runs the equivalent of `shll setup agent --yes` in-process
// via runAgentSetup (placing the shll-toolkit skill files, then delegating
// `run-kit agent setup --yes` — forwarding --yes so the delegation cannot hang on
// run-kit's hook-wiring prompt in an unattended install). The per-path
// wrote/unchanged/updated summary plus run-kit's own output are the announcement.
// A run-kit delegation failure stays non-fatal and does NOT trigger the nudge
// (inherited standalone semantics — re-running would hit the same failure, so the
// nudge would dead-end); only a placement failure (runAgentSetup's return) warns
// and falls back to the agent-setup nudge. --no-agent-setup → skip and nudge.
//
// The "Next steps:" block renders only when at least one line applies — the
// fully-wired happy path (shell wired or just auto-wired... reminder only) prints
// the reminder alone; a machine that needed nothing prints no header at all. A
// blank line precedes the block (the existing section-spacing rule) and the arrow
// glyph degrades via arrow(color). Never called on the dry-run / brew-missing /
// unknown-target paths (the call sites return before the outcome).
func runPostInstallSetup(ctx context.Context, env func(string) string, stdout, stderr io.Writer, color, noShellSetup, noAgentSetup bool) {
	glyph := arrow(color)
	var lines []string

	// Step 1: shell wiring.
	w := resolveWiringFact(env)
	shellGateOpen := w.shellResolved && !w.corrupt && !w.wired
	switch {
	case noShellSetup:
		// Opted out (dotfile-manager users) — the gated nudge is the fallback.
		if shellGateOpen {
			lines = append(lines, fmt.Sprintf(shellSetupNudgeFmt, glyph))
		}
	case !shellGateOpen:
		// Quiet edge states (unresolvable $SHELL, corrupt block) or already wired:
		// silent skip, no write, no nudge, no reminder.
	default:
		// Unwired — auto-run the shell-setup write path. The gate guarantees
		// resolveShell succeeds here; the error branch is belt-and-braces.
		shell, err := resolveShell(nil, env)
		if err == nil {
			err = runShellSetupDefault(shell, resolveRcFile(shell, env), false, stdout, stderr)
		}
		if err != nil {
			// Surface the actionable message (errExitCode carries it; the errSilent
			// paths already wrote their own diagnostic to stderr from inside
			// shell-setup), warn, and fall back to the nudge. Never propagated.
			var ec *errExitCode
			if errors.As(err, &ec) && ec.msg != "" {
				fmt.Fprintln(stderr, ec.msg)
			}
			fmt.Fprintln(stderr, shellSetupAutoRunWarn)
			lines = append(lines, fmt.Sprintf(shellSetupNudgeFmt, glyph))
		} else {
			lines = append(lines, fmt.Sprintf(execShellReminderFmt, glyph))
		}
	}

	// Step 2: agent wiring.
	switch {
	case noAgentSetup:
		// Opted out — the nudge is the fallback.
		lines = append(lines, fmt.Sprintf(agentSetupNudgeFmt, glyph))
	default:
		if err := runAgentSetup(ctx, env, stdout, stderr, false, false, true); err != nil {
			// Placement failure — the per-path diagnostics were already written to
			// stderr by agent-setup itself; warn and fall back to the nudge. Never
			// propagated (the install's exit code stays the authority).
			fmt.Fprintln(stderr, agentSetupAutoRunWarn)
			lines = append(lines, fmt.Sprintf(agentSetupNudgeFmt, glyph))
		}
	}

	// The block prints only when at least one line applies (no empty header).
	if len(lines) == 0 {
		return
	}
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, nextStepsHeader)
	for _, ln := range lines {
		fmt.Fprintln(stdout, ln)
	}
}
