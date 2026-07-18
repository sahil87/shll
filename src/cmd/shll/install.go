package main

import (
	"context"
	"fmt"
	"io"
	"os"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

func newInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install [tool...]",
		Short: "brew install every sahil87 tool that isn't already installed",
		Long: `Install every roster tool that isn't already installed via Homebrew.

shll install iterates the roster (` + "`wt`, `idea`, `tu`, `run-kit`, `hop`, `fab-kit`" + `)
and runs ` + "`brew install sahil87/tap/<formula>`" + ` for each one that is missing.
Tools that are already installed are skipped silently — the command is
idempotent and safe to re-run. Brew's progress output streams directly to
your terminal.

With no arguments, shll install processes the whole roster as above. Pass one or
more tool names to install only that subset (valid targets: wt, idea, tu, run-kit,
hop, fab-kit; the legacy alias ` + "`rk`" + ` still resolves to run-kit) — e.g.
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

shll install does NOT upgrade already-installed tools. Use ` + "`shll update`" + `
for that.`,
		Args: cobra.ArbitraryArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, _ := cmd.Flags().GetBool(dryRunFlag)
			noTrust, _ := cmd.Flags().GetBool(noTrustFlag)
			return runInstall(cmd.Context(), os.Getenv, cmd.OutOrStdout(), cmd.ErrOrStderr(), dryRun, noTrust, args)
		},
	}
	cmd.Flags().Bool(dryRunFlag, false, dryRunFlagUsage)
	cmd.Flags().Bool(noTrustFlag, false, noTrustFlagUsage)
	return cmd
}

// installTarget is one actionable tool in a `shll install` run: either a fresh
// `brew install` (migrate=false) or a brew-direct rk→run-kit migration
// (migrate=true). Built by the missing-partition so the install loop and the
// dry-run preview share one classification (Constitution III — one detection path).
// The migration action (migrateRunKit) derives its own post-migration dual-rack
// signal, so no probe result needs carrying here.
type installTarget struct {
	tool    Tool
	migrate bool
}

// noTrustFlag is the bool flag on `shll install` that skips the per-formula trust
// step (`brew trust --formula sahil87/tap/<formula>`) before each install. Named
// constant per code-quality.md (no magic strings).
const noTrustFlag = "no-trust"

// noTrustFlagUsage is the cobra usage string for --no-trust.
const noTrustFlagUsage = "skip recording per-formula Homebrew trust before installing (manage trust yourself)"

// runInstall is the implementation seam for `shll install`. Extracted from
// the cobra factory so install_test.go can drive it with bytes.Buffer writers
// and a fake proc.Runner.
//
// Behavior:
//   - brew missing → stderr hint, errSilent (exit 1).
//   - For each roster tool in order, skip if already installed; else (unless
//     noTrust) record per-formula trust via `brew trust --formula <formula>`,
//     then run `brew install sahil87/tap/<formula>` foregrounded. A tool with a
//     LegacyFormula whose LEGACY keg is present (run-kit on a pre-rename machine)
//     is instead routed through the brew-direct MIGRATION action (migrateRunKit),
//     not a blind `brew install` — reusing `shll update`'s migration gate + action.
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
// env is the environment lookup threaded into the post-install "Next steps" nudge
// block's shell-setup gate (via resolveWiringFact) — production passes os.Getenv;
// tests pass a map-backed func pointing at a t.TempDir() rc file, mirroring
// runDoctor's established seam. It is used ONLY by the nudge block (printNextSteps),
// which reads the rc file read-only; runInstall never writes to it.
func runInstall(ctx context.Context, env func(string) string, stdout, stderr io.Writer, dryRun, noTrust bool, args []string) error {
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

	// Collect the tools that are ACTIONABLE (not yet present). The slice is built
	// by walking `consider` in order, so iterating `missing` below preserves roster
	// order deterministically. The probes are reads, so they run in dry-run too —
	// only the writes are skipped. A named-but-already-installed target is filtered
	// out here (idempotent skip).
	//
	// A tool with a LegacyFormula (run-kit) is classified via the SHARED migration
	// gate (probeRunKitMigration — same detection `shll update`/`shll doctor` use):
	//   - migrated/present → skip (idempotent)
	//   - legacy keg present → actionable with migrate=true (brew-direct migration,
	//     NOT a blind `brew install` that risks the observed dual-rack state)
	//   - fully absent → actionable with migrate=false (normal `brew install`)
	missing := make([]installTarget, 0, len(consider))
	for _, t := range consider {
		if t.LegacyFormula != "" {
			installed, leaf, before := probeInstalledLeaf(ctx, t.Formula)
			p := probeRunKitMigration(ctx, t, installed, leaf, before)
			switch {
			case p.needsMigration:
				missing = append(missing, installTarget{tool: t, migrate: true})
			case !p.installed:
				missing = append(missing, installTarget{tool: t})
			}
			// installed && !needsMigration → migrated/present: skip (idempotent).
			continue
		}
		if !isInstalled(ctx, t.Formula) {
			missing = append(missing, installTarget{tool: t})
		}
	}

	if len(missing) == 0 {
		fmt.Fprintln(stdout, allInstalledMsg)
		// Short-circuit path (decision 3): a re-runner who never wired their shell
		// still gets nudged. colorEnabled is computed here (the install-loop path
		// computes it once below for its own framing; this early-return path never
		// reaches that, so it needs its own decision). Suppressed under --dry-run:
		// decision 5 keeps `--dry-run` nudge-free, and this short-circuit precedes
		// the dry-run branch, so gate on !dryRun here too.
		if !dryRun {
			printNextSteps(env, stdout, colorEnabled(stdout))
		}
		return nil
	}

	// Dry-run: the probes have run (reads); now preview the exact commands the real
	// run WOULD execute and exit 0 with NO write. The preview lists only the missing
	// subset (actionable tools), in roster order. A migrate target previews the
	// migration argv (`brew upgrade <LegacyFormula>`) via the same upgradeArgv the
	// live migration's first step uses, so preview and run cannot drift.
	if dryRun {
		rows := make([]previewRow, 0, len(missing))
		for _, m := range missing {
			cmd := argvString(brewBinary, "install", m.tool.Formula)
			if m.migrate {
				cmd = argvString(upgradeArgv(m.tool, false, true)...)
			}
			rows = append(rows, previewRow{label: m.tool.Name, cmd: cmd})
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

	// Probe `brew trust` availability ONCE up front (a brew-wide capability, not a
	// per-tool fact). The per-formula trust step runs only when trust was requested
	// (default; --no-trust opts out) AND this brew ships `brew trust`. On a pre-6.0
	// brew the subcommand is absent, but trust isn't required there either, so
	// skipping it is safe (Constitution V — graceful degradation).
	trustEnabled := !noTrust && brewTrustAvailable(ctx)

	// Wall-clock start for the run-duration suffix in the summary tail, from the
	// injectable nowFunc seam (clock.go). Captured after the short-circuit/dry-run
	// returns so it measures only the install phase the tail summarizes.
	start := nowFunc()

	anyFailed := false
	for i, m := range missing {
		t := m.tool
		// Section spacing: a blank line precedes every header EXCEPT the first.
		if i > 0 {
			fmt.Fprintln(stdout)
		}
		printToolHeader(stdout, t.Name, i+1, total, color)

		// Legacy-keg run-kit → run the brew-direct MIGRATION action instead of a
		// blind `brew install` (which risks the observed dual-rack state). Reuses the
		// exact migrateRunKit used by `shll update`. The migration's exit code drives
		// success/failure like an install.
		//
		// Trust the NEW formula (sahil87/tap/run-kit) FIRST when trust is enabled —
		// installed ≠ trusted (that inequality is doctor's trust-WARN premise, change
		// 0854), so the legacy keg being present does not imply the renamed formula
		// carries a trust record. The migration is `brew upgrade sahil87/tap/rk`, which
		// resolves the rename to sahil87/tap/run-kit's sandboxed `def install`; on
		// Homebrew 6.0+ that install is refused without a real trust record for the
		// formula it lands on. So trust run-kit before migrating, matching the
		// trust-then-act contract of the normal install path below (a trust failure
		// warns + continues; the migration's own exit code is the authority).
		if m.migrate {
			if trustEnabled {
				if code, terr := brewTrustFormula(ctx, t.Formula); terr != nil {
					fmt.Fprintf(stderr, "shll install: %s: trust step failed: %v (continuing to migrate)\n", t.Name, terr)
				} else if code != 0 {
					fmt.Fprintf(stderr, "shll install: %s: trust step exited %d (continuing to migrate)\n", t.Name, code)
				}
			}
			code, err := migrateRunKit(ctx, stdout, stderr, t)
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
			continue
		}

		// Record per-formula trust before the install (default; --no-trust or an
		// older brew lacking `brew trust` skips this). Homebrew 6.0 makes tap-trust
		// a hard install requirement, and a binary-download formula runs a sandboxed
		// `def install` that needs a real trust record — so this is what unblocks
		// the install. `brew trust` is idempotent. A trust failure is best-effort:
		// warn and proceed to the install attempt (which will surface brew's own
		// untrusted-tap error if trust truly didn't land), and do NOT set anyFailed —
		// the install's exit code is the authority on whether this tool succeeded.
		if trustEnabled {
			if code, terr := brewTrustFormula(ctx, t.Formula); terr != nil {
				fmt.Fprintf(stderr, "shll install: %s: trust step failed: %v (continuing to install)\n", t.Name, terr)
			} else if code != 0 {
				fmt.Fprintf(stderr, "shll install: %s: trust step exited %d (continuing to install)\n", t.Name, code)
			}
		}

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

	// Post-install "Next steps" nudge block (decisions 1–4). Printed after the
	// summary tail, on stdout, reusing the loop's single `color` decision. It is
	// informational and orthogonal to install outcome, so it prints regardless of
	// anyFailed (the tail already conveys per-tool failures). Never reached by the
	// dry-run / brew-missing / unknown-target early returns above.
	printNextSteps(env, stdout, color)

	if anyFailed {
		return errSilent
	}
	return nil
}

// allInstalledMsg is the nothing-to-do message for `shll install` (every roster tool
// already installed). Shared by the normal short-circuit and the dry-run empty case so
// both read identically. Named per code-quality.md.
const allInstalledMsg = "All sahil87 tools already installed."

// shllSelfInstallNote is the shll-first informational line `shll install` prepends.
// shll is the manager-member of the toolkit and is always already present (it is the
// running orchestrator), so the line is informational — NOT a brew install action.
// Named per code-quality.md (no magic strings).
const shllSelfInstallNote = "shll — already present / self-managed"

// Post-install "Next steps" nudge strings (change 93r2; agent-setup graduation
// change agst). Each is a named constant per code-quality.md — the wording is part of
// the user contract, so it lives in one place (mirroring allInstalledMsg /
// shllSelfInstallNote and doctor's suggestNotWired).
//
//   - nextStepsHeader labels the block.
//   - shellSetupNudgeFmt is the shell-setup line; the single %s is the arrow glyph
//     (arrow(color) → `→` on a color TTY, `->` otherwise). Its wording tracks doctor's
//     suggestNotWired ("run 'shll shell-setup' then 'exec $SHELL'").
//   - agentSetupNudgeFmt is the agent-setup line; the single %s is the arrow glyph. It
//     GRADUATED from the former `run-kit agent-setup` nudge (change agst): the
//     cross-toolkit harness wiring now belongs to shll (the manager), which wires agent
//     harnesses with toolkit context AND delegates run-kit's dashboard hooks. It is
//     informational and marked "optional, once per machine" — shll cannot cheaply know
//     whether agent-setup already ran (it would have to read several harness files just
//     to gate a nudge; Constitution II/III argue against it), so the line prints
//     unconditionally on the outcome paths (the accepted trade-off, mirroring the old
//     run-kit line, which also printed for users who had already run it).
const (
	nextStepsHeader    = "Next steps:"
	shellSetupNudgeFmt = "  %s shll shell-setup    # wire shell integration into your rc file, then: exec $SHELL"
	agentSetupNudgeFmt = "  %s shll agent-setup    # optional, once per machine — wire agent harnesses (toolkit context + run-kit dashboard hooks)"
)

// runKitToolName is the run-kit binary name, invoked by agent_setup.go's
// delegateRunKitAgentSetup as the subprocess target for the `run-kit agent-setup`
// delegation (Constitution III/IV — compose, don't absorb). Named per code-quality.md
// (no magic strings).
const runKitToolName = "run-kit"

// printNextSteps writes the post-install "Next steps" nudge block to stdout (change
// 93r2; agent-setup graduation change agst):
//
//   - shell-setup nudge: printed only when shll's sentinel block is NOT wired in the
//     user's rc file. The gate reuses doctor's read-only resolveWiringFact(env)
//     detector (Constitution III — one detection path) and fires on
//     `shellResolved && !corrupt && !wired`: quiet on an unresolvable $SHELL (nudging
//     toward `shll shell-setup` would exit 2) and on a corrupt open-without-close block
//     (shell-setup refuses it — doctor owns that diagnostic). Strictly read-only:
//     resolveWiringFact only os.ReadFile's the rc file; `shll install` never writes it.
//   - agent-setup line: printed UNCONDITIONALLY (graduated from the run-kit-gated
//     `run-kit agent-setup` line, change agst). shll is by definition present, so a
//     presence gate is meaningless; and shll cannot cheaply know whether agent-setup
//     already ran without reading several harness files just to gate a nudge
//     (Constitution II/III argue against it). Informational, marked "optional, once per
//     machine" — the accepted trade-off (it may print for users who already ran it),
//     mirroring the old run-kit line.
//
// The agent-setup line always prints, so the block (and its "Next steps:" header)
// always prints on the outcome paths. The header and lines go to stdout with the same
// color/TTY framing as the headers/tail (the arrow glyph degrades via arrow(color)). A
// blank line precedes the block (the existing section-spacing rule). Never called on
// the dry-run / brew-missing / unknown-target paths (they return before the outcome).
func printNextSteps(env func(string) string, stdout io.Writer, color bool) {
	w := resolveWiringFact(env)
	shellSetup := w.shellResolved && !w.corrupt && !w.wired
	fmt.Fprintln(stdout)
	fmt.Fprintln(stdout, nextStepsHeader)
	glyph := arrow(color)
	if shellSetup {
		fmt.Fprintf(stdout, shellSetupNudgeFmt+"\n", glyph)
	}
	fmt.Fprintf(stdout, agentSetupNudgeFmt+"\n", glyph)
}
