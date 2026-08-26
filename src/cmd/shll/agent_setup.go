package main

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

// agent_setup.go implements the agent half of `shll setup` (`shll setup agent`;
// hidden compat spelling `shll agent-setup`) — mechanically place ONE thin Agent
// Skill (the toolkit bootstrap) into the harnesses' global skills directories, then
// delegate run-kit's dashboard-hook wiring to `run-kit agent setup` (Constitution
// III/IV — compose, don't absorb). It graduates the cross-toolkit harness wiring from
// run-kit (a leaf tool) to shll (the manager).
//
// The skill directories are shll-OWNED, so there is no merge, no sentinel, no
// diff-and-confirm: install = write, re-run/upgrade = overwrite (idempotent by
// construction), --uninstall = delete. This file performs plain file I/O plus ONE
// subprocess (the run-kit delegation via internal/proc — Constitution I).

// agentSetupErrPrefix is the diagnostic prefix stamped on this command's stderr.
const agentSetupErrPrefix = "shll setup agent"

// skillDirName is the Agent-Skill directory (and `name:` frontmatter value) placed at
// each target. It satisfies the agentskills.io portable-name rule
// (^[a-z0-9]+(-[a-z0-9]+)*$) and MUST equal the frontmatter `name:` — the same string
// is used for both so they cannot drift. Named per code-quality.md (no magic strings).
const skillDirName = "shll-toolkit"

// skillFileName is the SKILL.md filename the Agent Skills open standard requires
// inside each skill directory (<dir>/<name>/SKILL.md).
const skillFileName = "SKILL.md"

// agentSkillContent is the canonical bytes of the placed SKILL.md — the toolkit
// bootstrap skill. Portable frontmatter carries `name` + `description` ONLY (the
// OpenCode-recognized common subset, valid on all four harnesses); `name` equals
// skillDirName. The description front-loads trigger vocabulary — the tool names AND
// each tool's task-domain phrase (Roster.SkillHint) — for implicit activation: the
// description is the only text in an agent's context BEFORE the skill is invoked, so
// task-shaped requests ("create a worktree") must match there, not just tool names.
// The body stays thin — it teaches the runtime two-step plus one `shll standards`
// pointer, deferring the live tool list to `shll skill` (re-derived at request time,
// Constitution II). This is an agent-setup artifact (neither a published standard nor
// a `<tool> skill` bundle), so it is built here from the Roster (Constitution III —
// one source of truth), not maintained as a docs-site file.
var agentSkillContent = `---
name: ` + skillDirName + `
description: ` + agentSkillDescription() + `
---
# shll toolkit

This machine has the shll toolkit installed. Before driving one of its tools:

1. ` + "`shll skill`" + ` — the installed tools, one line each
2. ` + "`shll skill <tool>`" + ` — that tool's full agent skill bundle (when to use it,
   composition patterns, output and exit-code contracts, gotchas). A large-scope tool's
   core bundle lists topic pages; ` + "`shll skill <tool> <topic>`" + ` serves one on demand.

Run-kit also has agent-proactive capabilities — visual display in a browser window, push notifications, and running VS Code palette commands inside the user's code editor (` + "`rk code exec`" + `); see ` + "`shll skill run-kit`" + ` (and ` + "`shll skill run-kit code`" + ` for the editor bridge).

For toolkit-repo development, ` + "`shll standards`" + ` enumerates the binding CLI standards.
`

// agentSkillDescription builds the frontmatter description line from the Roster, one
// `task-domain phrase (tool)` clause per tool, so both the tool names and the task
// vocabulary act as activation triggers. A tool with a LegacyName renders both tokens
// (`run-kit/rk`) — the alias is trigger vocabulary too. Each non-empty ProactiveHint
// is then appended verbatim (Roster order) as an additional sentence AFTER the tool
// clauses and BEFORE the closing two-step pointer — the agent-proactive trigger
// vocabulary (today only run-kit's display + notify). Single-sourced with the Roster
// so the description cannot drift from the managed set; the output MUST stay a single
// line (YAML frontmatter value — asserted by TestAgentSetup_DescriptionSingleLine).
func agentSkillDescription() string {
	clauses := make([]string, 0, len(Roster))
	var proactive []string
	for _, t := range Roster {
		name := t.Name
		if t.LegacyName != "" {
			name += "/" + t.LegacyName
		}
		clauses = append(clauses, fmt.Sprintf("%s (%s)", t.SkillHint, name))
		if t.ProactiveHint != "" {
			proactive = append(proactive, t.ProactiveHint)
		}
	}
	desc := "Use when driving any shll toolkit CLI or shll itself — " +
		strings.Join(clauses, ", ") + "."
	if len(proactive) > 0 {
		desc += " " + strings.Join(proactive, " ")
	}
	return desc +
		" Run `shll skill` to list the installed tools; run `shll skill <tool>` for that tool's full usage bundle before using it."
}

// skillTargetRelDirs are the two global skill-DIRECTORY paths (relative to $HOME) at
// which the toolkit bootstrap skill is placed — the minimal covering set for all four
// harnesses (verified 2026-07-18):
//
//   - .agents/skills — the agentskills.io open-standard path: read natively by Codex
//     (USER scope) and compat-read by Cursor and OpenCode.
//   - .claude/skills — Claude Code, which does NOT read ~/.agents/.
//
// Both writes are unconditional (agent-setup is an explicit "wire this machine"
// command) and shll owns these dirs, so they are created as needed. Any future harness
// adopting the open standard picks up ~/.agents/skills automatically.
var skillTargetRelDirs = []string{
	".agents/skills",
	".claude/skills",
}

// agentSetupSub is the hidden deprecated top-level spelling of shll's OWN agent
// half — the cobra `Use:` token of the old `shll agent-setup` command, kept
// registered (Hidden, silent) for one release cycle because an OLD binary's
// `shll update` self-refresh executes `shll agent-setup --yes` against the NEW
// binary across the release boundary. The new binary's own refreshArgv emits the
// new spelling (`shll setup agent`, built from setupSub/setupAgentLeaf in
// setup.go). The run-kit delegation never shared this token — run-kit renamed
// its command family to the two-token `agent setup` (runKitAgentSetupArgs).
// Named per code-quality.md (no magic strings).
const agentSetupSub = "agent-setup"

// runKitAgentSetupArgs is the run-kit hook-wiring command family in its post-rename
// two-token spelling `run-kit agent setup` (run-kit PR #620; first shipped in
// v3.16.23). The prior `run-kit agent-setup` spelling is deprecated upstream and
// prints a deprecation warning on every delegation. Deliberately no version probe and
// no old-spelling fallback: the delegation is a best-effort adjunct (warn-and-continue
// below), and `shll update`'s refresh runs after the roster loop has just upgraded
// run-kit, so the new family exists by construction there.
var runKitAgentSetupArgs = []string{"agent", "setup"}

// runKitToolName is the run-kit binary name — the subprocess target for
// delegateRunKitAgentSetup's `run-kit agent setup` delegation (Constitution III/IV —
// compose, don't absorb), and matched against Roster entry names by uninstall.go's
// daemon-stop hint. Named per code-quality.md (no magic strings).
const runKitToolName = "run-kit"

// agentSetupYesUsage is the cobra usage string for --yes/-y on the agent-setup
// surface (`shll setup agent`, the hidden `shll agent-setup`, and bare
// `shll setup`, whose --yes forwards to the same place).
// Distinct from uninstall.go's yesFlagUsage because the prompt being skipped is not
// shll's own (the skill placement is promptless by construction) — it belongs to the
// delegated `run-kit agent setup`, whose hook-wiring confirmation would otherwise hang
// an unattended run (a pane TTY with nobody attached is structurally undetectable, so
// the consent must be explicit, never TTY-derived).
const agentSetupYesUsage = "pass --yes to the run-kit agent setup delegation (assume yes — for unattended runs)"

// agentSetupCmdSpec carries the per-spelling surface differences between the new
// `shll setup agent` subcommand and the hidden deprecated `shll agent-setup`
// top-level command. Both spellings are built by buildAgentSetupCmd from a spec,
// so their flag sets and behavior share construction and cannot drift.
type agentSetupCmdSpec struct {
	use    string
	short  string
	long   string
	hidden bool
}

// newAgentSetupCmd builds the hidden deprecated `shll agent-setup` top-level
// command. Its Short/Long carry only the rename pointer — the full help moved to
// `shll setup agent` (setupAgentLong).
func newAgentSetupCmd() *cobra.Command {
	return buildAgentSetupCmd(agentSetupCmdSpec{
		use:   agentSetupSub,
		short: "renamed to `shll setup agent` (hidden; kept for one release cycle)",
		long: "Renamed to `shll setup agent`. This old spelling still works — hidden and\n" +
			"silent — for one release cycle, then it will be removed. See `shll setup agent --help`.",
		hidden: true,
	})
}

// buildAgentSetupCmd is the shared parameterized builder for both agent-half
// spellings (new `setup agent` subcommand and hidden `agent-setup` top-level).
func buildAgentSetupCmd(spec agentSetupCmdSpec) *cobra.Command {
	var (
		printMode     bool
		uninstallMode bool
		yesMode       bool
	)
	cmd := &cobra.Command{
		Use:           spec.use,
		Short:         spec.short,
		Long:          spec.long,
		Hidden:        spec.hidden,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentSetup(cmd.Context(), os.Getenv, cmd.OutOrStdout(), cmd.ErrOrStderr(), printMode, uninstallMode, yesMode)
		},
	}
	cmd.Flags().BoolVar(&printMode, "print", false, "print the SKILL.md content and target paths, do not write any file")
	cmd.Flags().BoolVar(&uninstallMode, "uninstall", false, "remove both placed shll-toolkit skill directories")
	cmd.Flags().BoolVarP(&yesMode, yesFlag, yesFlagShorthand, false, agentSetupYesUsage)
	return cmd
}

// resolveSkillTargets returns the absolute SKILL.md paths (one per skillTargetRelDirs
// entry) under $HOME. An empty $HOME yields no targets (nothing to place).
func resolveSkillTargets(env func(string) string) []string {
	home := env("HOME")
	if home == "" {
		return nil
	}
	out := make([]string, 0, len(skillTargetRelDirs))
	for _, rel := range skillTargetRelDirs {
		out = append(out, filepath.Join(home, rel, skillDirName, skillFileName))
	}
	return out
}

// runAgentSetup is the implementation seam, extracted from the cobra factory so
// agent_setup_test.go can drive it with bytes.Buffer writers, a controlled env
// (HOME → t.TempDir()), and a fake proc.Runner.
//
//	env           resolves $HOME for skill-path derivation.
//	printMode     print the content + paths, touch nothing, no delegation.
//	uninstallMode delete both skill directories, then delegate run-kit's uninstall.
//	yes           forward --yes to the run-kit delegation (skips its confirmation
//	              prompt for unattended runs). A no-op under printMode, which never
//	              delegates — deliberately NOT a usage error, unlike --print+--uninstall,
//	              because the combination is harmless rather than contradictory.
func runAgentSetup(ctx context.Context, env func(string) string, stdout, stderr io.Writer, printMode, uninstallMode, yes bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if printMode && uninstallMode {
		return &errExitCode{code: usageExitCode, msg: agentSetupErrPrefix + ": --print and --uninstall are mutually exclusive"}
	}

	targets := resolveSkillTargets(env)

	// --print: emit the content + both target paths, touch nothing (no delegation).
	if printMode {
		return runAgentPrint(targets, stdout, stderr)
	}
	if uninstallMode {
		return runAgentUninstall(ctx, targets, yes, stdout, stderr)
	}
	return runAgentInstall(ctx, targets, yes, stdout, stderr)
}

// runAgentPrint writes the canonical SKILL.md content followed by the two target paths
// it WOULD be written to, and modifies nothing. It does not delegate to run-kit.
func runAgentPrint(targets []string, stdout, stderr io.Writer) error {
	if _, err := io.WriteString(stdout, agentSkillContent); err != nil {
		fmt.Fprintf(stderr, "%s: write stdout: %v\n", agentSetupErrPrefix, err)
		return errSilent
	}
	fmt.Fprintln(stdout, "\nTarget paths:")
	for _, p := range targets {
		fmt.Fprintf(stdout, "  %s\n", p)
	}
	return nil
}

// runAgentInstall writes the canonical SKILL.md to every target path (creating the
// skill directory as needed — shll owns it), printing a per-path written/updated/
// unchanged summary, then delegates run-kit's harness hooks.
func runAgentInstall(ctx context.Context, targets []string, yes bool, stdout, stderr io.Writer) error {
	content := []byte(agentSkillContent)
	anyFailed := false
	for _, path := range targets {
		if err := placeSkill(path, content, stdout, stderr); err != nil {
			anyFailed = true
		}
	}

	// Delegate run-kit's harness hooks (Constitution III/IV). Skip silently when
	// run-kit is absent (Constitution V).
	delegateRunKitAgentSetup(ctx, false, yes, stderr)

	if anyFailed {
		return errSilent
	}
	return nil
}

// placeSkill writes content to a single target SKILL.md path, creating its parent
// skill directory when absent. It reports one of three states per path:
//
//   - "wrote"     — the file did not exist (first placement).
//   - "unchanged" — the file existed and already held the canonical bytes (idempotent
//     re-run — no write is performed).
//   - "updated"   — the file existed with different bytes and was overwritten.
func placeSkill(path string, content []byte, stdout, stderr io.Writer) error {
	existing, err := os.ReadFile(path)
	switch {
	case err == nil:
		// File exists — compare before writing so a no-op re-run is reported as such.
		if bytes.Equal(existing, content) {
			fmt.Fprintf(stdout, "unchanged  %s\n", path)
			return nil
		}
		if werr := os.WriteFile(path, content, 0o644); werr != nil {
			fmt.Fprintf(stderr, "%s: write %s: %v\n", agentSetupErrPrefix, path, werr)
			return errSilent
		}
		fmt.Fprintf(stdout, "updated    %s\n", path)
		return nil
	case errors.Is(err, os.ErrNotExist):
		// New placement — create the skill directory (shll owns it), then write.
		if derr := os.MkdirAll(filepath.Dir(path), 0o755); derr != nil {
			fmt.Fprintf(stderr, "%s: create %s: %v\n", agentSetupErrPrefix, filepath.Dir(path), derr)
			return errSilent
		}
		if werr := os.WriteFile(path, content, 0o644); werr != nil {
			fmt.Fprintf(stderr, "%s: write %s: %v\n", agentSetupErrPrefix, path, werr)
			return errSilent
		}
		fmt.Fprintf(stdout, "wrote      %s\n", path)
		return nil
	default:
		// A non-not-exist read error (permission, etc.) — surface and skip this path.
		fmt.Fprintf(stderr, "%s: read %s: %v\n", agentSetupErrPrefix, path, err)
		return errSilent
	}
}

// runAgentUninstall removes each placed skill DIRECTORY (the shll-toolkit dir under
// each target, not just the SKILL.md file), then delegates `run-kit agent setup
// --uninstall`. Removing an shll-owned directory is safe and needs no confirmation.
func runAgentUninstall(ctx context.Context, targets []string, yes bool, stdout, stderr io.Writer) error {
	anyFailed := false
	for _, path := range targets {
		dir := filepath.Dir(path) // .../shll-toolkit
		if _, err := os.Stat(dir); errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(stdout, "absent     %s\n", dir)
			continue
		}
		if err := os.RemoveAll(dir); err != nil {
			fmt.Fprintf(stderr, "%s: remove %s: %v\n", agentSetupErrPrefix, dir, err)
			anyFailed = true
			continue
		}
		fmt.Fprintf(stdout, "removed    %s\n", dir)
	}

	// Delegate run-kit's own uninstall.
	delegateRunKitAgentSetup(ctx, true, yes, stderr)

	if anyFailed {
		return errSilent
	}
	return nil
}

// delegateRunKitAgentSetup invokes `run-kit agent setup [--uninstall] [--yes]` as a
// foreground subprocess (via internal/proc — Constitution I) for run-kit's dashboard
// hooks. When run-kit is not on PATH (proc.ErrNotFound) the delegation is skipped
// silently (Constitution V — graceful degradation); its stdio is inherited
// (proc.RunForeground always wires the real os.Stdout/os.Stderr) so the user sees
// run-kit's own output — this helper only writes its own diagnostics to stderr, so it
// takes no stdout writer. yes forwards --yes so run-kit's hook-wiring confirmation is
// skipped (unattended runs) — appended on both the install and uninstall paths, the
// delegation being the same helper either way. Only the default (install) and
// --uninstall paths call this; --print never does. An installed run-kit older than
// v3.16.23 lacks the `agent` family and exits non-zero — that lands on the same
// warn-and-continue path as any other delegation failure (see runKitAgentSetupArgs).
func delegateRunKitAgentSetup(ctx context.Context, uninstall, yes bool, stderr io.Writer) {
	args := append([]string{}, runKitAgentSetupArgs...)
	if uninstall {
		args = append(args, "--uninstall")
	}
	if yes {
		args = append(args, "--"+yesFlag)
	}
	code, err := proc.RunForeground(ctx, runKitToolName, args...)
	if errors.Is(err, proc.ErrNotFound) {
		return // run-kit absent — skip silently.
	}
	if err != nil {
		// A real delegation error (not "absent") is worth surfacing, but it does not
		// fail the skill placement shll already did — placement is agent-setup's core
		// work; run-kit hooks are the optional adjunct.
		fmt.Fprintf(stderr, "%s: run-kit agent setup: %v (continuing)\n", agentSetupErrPrefix, err)
		return
	}
	if code != 0 {
		// RunForeground returns err == nil when the child starts and exits non-zero
		// (the code carries the outcome). Same adjunct rule: warn, never fail the
		// placement (mirrors install's delegated-trust-step precedent).
		fmt.Fprintf(stderr, "%s: run-kit agent setup exited %d (continuing)\n", agentSetupErrPrefix, code)
	}
}

// agentSkillPlacementState reports the on-disk state of the placed skills, read-only:
// placed is true when ANY skill target file exists (the user opted in via a prior
// `shll setup agent`); stale is true when any EXISTING target's bytes differ from the
// running binary's canonical content. An existing-but-unreadable target counts as
// placed with staleness unknown (never reported stale — Constitution V: don't warn on
// a state we can't determine). Consumed by `shll update`'s conditional refresh (placed
// only) and `shll doctor`'s shll-row staleness check (both facts).
func agentSkillPlacementState(env func(string) string) (placed, stale bool) {
	for _, path := range resolveSkillTargets(env) {
		data, err := os.ReadFile(path)
		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				placed = true
			}
			continue
		}
		placed = true
		if !bytes.Equal(data, []byte(agentSkillContent)) {
			stale = true
		}
	}
	return placed, stale
}

// agentSkillRefreshHeader is the section line `shll update` prints before the
// self-refresh subprocess output. Named per code-quality.md (no magic strings).
const agentSkillRefreshHeader = "Refreshing placed agent skills (shll setup agent)…"

// refreshPlacedAgentSkills re-places the agent skills at the end of a `shll update`
// run, but ONLY when a prior placement exists — a user who never
// opted in gets no unsolicited writes. It invokes `shll setup agent` as a SUBPROCESS
// (resolved from PATH, via internal/proc — Constitution I) rather than calling
// runAgentSetup in-process: after a brew self-upgrade the RUNNING binary still holds
// the OLD embedded skill content, and only the freshly installed binary on PATH can
// place the new bytes. The subprocess also re-runs the run-kit hook delegation, so a
// run-kit upgrade's hook changes land too — which is why the caller runs this AFTER
// the roster loop.
//
// Best-effort adjunct, mirroring delegateRunKitAgentSetup: an `shll` binary missing
// from PATH (a non-brew dev build) is a silent skip (Constitution V — `shll doctor`
// still surfaces staleness), and any other failure warns and continues without
// affecting the update's exit code — the tool upgrades are the run's core work.
//
// yes threads `shll update --yes` through to the subprocess (`shll setup agent --yes`),
// which in turn forwards it to the run-kit delegation — the explicit consent chain
// that keeps an unattended `shll update` from hanging on run-kit's hook prompt.
func refreshPlacedAgentSkills(ctx context.Context, env func(string) string, yes bool, stdout, stderr io.Writer) {
	if placed, _ := agentSkillPlacementState(env); !placed {
		return
	}
	fmt.Fprintf(stdout, "\n%s\n", agentSkillRefreshHeader)
	argv := refreshArgv(yes)
	code, err := proc.RunForeground(ctx, argv[0], argv[1:]...)
	if errors.Is(err, proc.ErrNotFound) {
		return // shll not on PATH (dev build) — skip silently.
	}
	if err != nil {
		fmt.Fprintf(stderr, "shll update: agent skill refresh: %v (continuing)\n", err)
		return
	}
	if code != 0 {
		fmt.Fprintf(stderr, "shll update: agent skill refresh exited %d (continuing)\n", code)
	}
}

// refreshArgv is the exact argv the end-of-run agent-skill refresh runs
// (`shll setup agent [--yes]`) — the single source of truth shared by the live
// subprocess above and `shll update`'s dry-run preview line, so the preview can
// never drift from what the run would do (mirrors update.go's upgradeArgv pattern).
//
// Compat note: this spelling flip is exactly why the hidden `shll agent-setup`
// top-level command must survive one release cycle — an OLD running binary's
// refreshArgv composes `shll agent-setup [--yes]` and executes it against the
// NEW binary on PATH after the brew self-upgrade.
func refreshArgv(yes bool) []string {
	argv := []string{shllTargetToken, setupSub, setupAgentLeaf}
	if yes {
		argv = append(argv, "--"+yesFlag)
	}
	return argv
}
