package main

import (
	"context"
	"errors"
	"io"
	"os"

	"github.com/spf13/cobra"
)

// setup.go implements the `shll setup` command family — the consolidated,
// re-runnable entry point for wiring a machine for the shll toolkit:
//
//	shll setup                both halves: shell integration, then agent harnesses
//	shll setup shell [shell]  the shell half only (full shell-setup surface)
//	shll setup agent          the agent half only (full agent-setup surface)
//
// All three are THIN cobra faces over the existing internals (runShellSetup* /
// runAgentSetup) — no logic moved. The subcommands share command construction
// with the hidden deprecated old spellings (buildShellSetupCmd /
// buildAgentSetupCmd) so the flag sets cannot drift apart. The old spellings
// stay registered (Hidden, silent) for one release cycle because an OLD binary's
// `shll update` self-refresh executes `shll agent-setup --yes` against the NEW
// binary across the release boundary.

const (
	// setupSub is the `setup` parent command token. Named per code-quality.md (no
	// magic strings); refreshArgv builds the self-refresh argv from it.
	setupSub = "setup"
	// setupShellLeaf / setupAgentLeaf are the `setup` subcommand tokens.
	setupShellLeaf = "shell"
	setupAgentLeaf = "agent"
)

// setupLong is the parent command's Long help. The bare form's surface is
// --yes-only by design (minimal surface; the halves' full flag sets live on the
// subcommands).
const setupLong = `Wire this machine for the shll toolkit — both halves, in order:
shell integration (the ` + "`eval \"$(shll shell-init <shell>)\"`" + ` line in your rc file),
then agent-harness wiring (the shll-toolkit skill plus run-kit's dashboard
hooks). Both halves are idempotent — re-running is safe, e.g. after installing
a new shell or a new agent harness.

This is the same wiring ` + "`shll install`" + ` runs automatically at the end of an
install; ` + "`shll setup`" + ` is the standalone re-run entry point. Pass ` + "`--yes`" + ` (or
` + "`-y`" + `) to forward ` + "`--yes`" + ` to the run-kit delegation so nothing can prompt on an
unattended run.

Both halves always run — the agent half runs even when the shell half failed —
and the exit code is the worst of the two.

Subcommands:
  shll setup shell [shell]   the shell half only (rc-file block; --print/--uninstall/--rc-file)
  shll setup agent           the agent half only (skill placement; --print/--uninstall/--yes)`

// setupShellLong is the full help for `shll setup shell` — the shell-setup
// surface under its new spelling (the hidden `shll shell-setup` keeps a
// one-line rename pointer instead).
const setupShellLong = `Append a sentinel-wrapped eval block that wires shll shell-init into your
shell rc file. Idempotent — re-running is a no-op when the block is already
present. Plain O_APPEND so dotfile-manager symlinks are preserved.

Modes:
  shll setup shell [shell]             install the block (default mode)
  shll setup shell --print [shell]     print the block to stdout, do not modify
  shll setup shell --uninstall [shell] remove the block from the rc file

shll setup shell is pure rc-wiring — it maintains only the
` + "`eval \"$(shll shell-init <shell>)\"`" + ` line and touches no Homebrew state.
(Tap trust is established by ` + "`shll install`" + `, which trusts each formula it
installs; see ` + "`shll install --help`" + `.)

When [shell] is omitted, shll infers it from $SHELL. Supported shells: zsh, bash.

By default, the rc file path is derived per shell:
  zsh   → ${ZDOTDIR:-$HOME}/.zshrc
  bash  → $HOME/.bash_profile (macOS) or $HOME/.bashrc (Linux)

Use --rc-file <path> to override derivation entirely.

The pre-consolidation spelling ` + "`shll shell-setup`" + ` (alias ` + "`shll shell-install`" + `)
still works for one release cycle — hidden and silent — and will be removed in a
future release.`

// setupAgentLong is the full help for `shll setup agent` — the agent-setup
// surface under its new spelling.
const setupAgentLong = `Mechanically place one thin Agent Skill — the shll toolkit bootstrap — into the
agent harnesses' global skills directories, then delegate run-kit's dashboard-hook
wiring to ` + "`run-kit agent setup`" + `. The skill teaches an agent to load ` + "`shll skill`" + ` before
driving a toolkit tool.

The skill is written to exactly two global locations (covering all four harnesses):
  ~/.agents/skills/` + skillDirName + `/SKILL.md   Codex (USER scope), Cursor + OpenCode
  ~/.claude/skills/` + skillDirName + `/SKILL.md   Claude Code

The skill directories are shll-owned, so placement is idempotent by construction —
install writes them, a re-run overwrites them, and there is no merge, prompt, or
sentinel machinery. A per-path written/updated/unchanged summary is printed.

Modes:
  shll setup agent             place the skill at both locations (overwrites; idempotent)
  shll setup agent --print     print the SKILL.md content and both target paths, write nothing
  shll setup agent --uninstall remove both placed skill directories

Pass ` + "`--yes`" + ` (or ` + "`-y`" + `) to forward ` + "`--yes`" + ` to the run-kit delegation so its own
confirmation prompt is skipped — for unattended runs (shll's skill placement itself
never prompts). With ` + "`--print`" + ` the flag is a no-op (print never delegates).

The pre-consolidation spelling ` + "`shll agent-setup`" + ` still works for one release
cycle — hidden and silent — and will be removed in a future release.`

// newSetupCmd builds the runnable `shll setup` parent: it runs the shell half
// then the agent half via the existing internals (no logic moved), with
// --yes/-y as its ONLY flag (forwarded to the agent half's run-kit delegation).
func newSetupCmd() *cobra.Command {
	var yesMode bool
	cmd := &cobra.Command{
		Use:           setupSub,
		Short:         "wire this machine for the shll toolkit (shell + agent harnesses)",
		Long:          setupLong,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runSetup(cmd.Context(), os.Getenv, cmd.OutOrStdout(), cmd.ErrOrStderr(), yesMode)
		},
	}
	// The parent shares the agent half's --yes usage string: the consent chain is
	// identical (the flag's only consumption point is the run-kit delegation).
	cmd.Flags().BoolVarP(&yesMode, yesFlag, yesFlagShorthand, false, agentSetupYesUsage)
	cmd.AddCommand(newSetupShellCmd(), newSetupAgentCmd())
	return cmd
}

// newSetupShellCmd builds `shll setup shell` — the shell half under the new
// spelling, sharing construction with the hidden old spelling so the two cannot
// drift (flag-surface parity).
func newSetupShellCmd() *cobra.Command {
	return buildShellSetupCmd(shellSetupCmdSpec{
		use:   setupShellLeaf + " [shell]",
		short: "append the shll shell-init eval line to your rc file",
		long:  setupShellLong,
	})
}

// newSetupAgentCmd builds `shll setup agent` — the agent half under the new
// spelling, sharing construction with the hidden old spelling.
func newSetupAgentCmd() *cobra.Command {
	return buildAgentSetupCmd(agentSetupCmdSpec{
		use:   setupAgentLeaf,
		short: "place the shll toolkit skill for agent harnesses",
		long:  setupAgentLong,
	})
}

// runSetup is the implementation seam for bare `shll setup`, extracted from the
// cobra factory so setup_test.go can drive it with bytes.Buffer writers and a
// controlled env. It runs the shell half (standalone semantics — a missing rc
// file is an exit-2 diagnostic, not install's quiet skip) then the agent half,
// ALWAYS both (mirroring the halves' independence in install's auto-run), and
// returns the worst of the two outcomes (worst-wins per the toolkit exit-code
// convention). yes forwards to the agent half's run-kit delegation only.
func runSetup(ctx context.Context, env func(string) string, stdout, stderr io.Writer, yes bool) error {
	if ctx == nil {
		ctx = context.Background()
	}
	shellErr := runShellSetup(ctx, nil, "", false, false, stdout, stderr)
	agentErr := runAgentSetup(ctx, env, stdout, stderr, false, false, yes)
	return worstError(shellErr, agentErr)
}

// exitCodeOf maps a half's error to the toolkit exit code translateExit would
// produce for it, so runSetup can aggregate without re-printing.
func exitCodeOf(err error) int {
	if err == nil {
		return 0
	}
	var withCode *errExitCode
	if errors.As(err, &withCode) {
		return withCode.code
	}
	return 1 // errSilent and unclassified errors are operational failures
}

// carriesUnprintedMsg reports whether err is an errExitCode whose message
// translateExit has NOT printed yet (as opposed to errSilent paths, which
// already wrote their own diagnostics to stderr).
func carriesUnprintedMsg(err error) bool {
	var withCode *errExitCode
	return errors.As(err, &withCode) && withCode.msg != ""
}

// worstError picks the error carrying the highest exit code (worst-wins). On a
// tie it prefers an error whose message has not been printed yet, so
// translateExit still emits it — an already-printed errSilent diagnostic would
// otherwise shadow an unprinted usage message.
func worstError(errs ...error) error {
	var worst error
	for _, err := range errs {
		if err == nil {
			continue
		}
		if worst == nil ||
			exitCodeOf(err) > exitCodeOf(worst) ||
			(exitCodeOf(err) == exitCodeOf(worst) && carriesUnprintedMsg(err) && !carriesUnprintedMsg(worst)) {
			worst = err
		}
	}
	return worst
}
