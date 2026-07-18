package main

import (
	"github.com/spf13/cobra"
)

const rootLong = `shll — meta-CLI for the sahil87 toolkit.

shll composes operations that span every per-tool CLI (hop, wt, fab-kit, run-kit, tu, idea)
so you have one entry point for cross-toolkit concerns.

Subcommands:
  shll doctor                 verify every sahil87 tool is installed, runnable, and wired (read-only)
  shll install                brew install every sahil87 tool that isn't already installed
  shll update                 brew update + brew upgrade for shll and every installed sahil87 tool
  shll uninstall              brew uninstall sahil87 tools (a clean-slate repair path)
  shll changelog              show release notes for sahil87 tools (what an update would bring)
  shll shell-init <shell>     emit a single eval-safe shell-init blob for all installed tools
  shll shell-setup [shell]    append the shell-init eval line to your rc file (idempotent)
  shll version                print versions of shll and every installed sahil87 tool
  shll list                   list the managed sahil87 tools with install status and repo links
  shll standards [name]       read the toolkit's binding standards (list them, or print one)
  shll skill [tool] [topic]   read a tool's agent skill bundle or one of its topic pages (or list installed tools)
  shll agent-setup            place the sahil87 toolkit skill for agent harnesses

Per-tool CLIs continue to work standalone — shll wraps them, it does not replace them.`

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "shll",
		Short:         "meta-CLI for the sahil87 toolkit",
		Long:          rootLong,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	// Route flag-parse errors (unknown flag, bad flag value) to the toolkit
	// usage-error exit code (2) by wrapping them in the errExitCode sentinel
	// translateExit already understands. Set on the root so every subcommand
	// inherits it (cobra's FlagErrorFunc walks up to the root when a command
	// sets none — see cobra command.go FlagErrorFunc). This is the clean hook
	// for flag errors; the arg/command usage errors cobra raises outside flag
	// parsing carry no such hook and are classified by prefix in translateExit.
	cmd.SetFlagErrorFunc(func(_ *cobra.Command, err error) error {
		return &errExitCode{code: usageExitCode, msg: err.Error()}
	})
	cmd.AddCommand(
		newDoctorCmd(),
		newInstallCmd(),
		newUpdateCmd(),
		newUninstallCmd(),
		newChangelogCmd(),
		newShellInitCmd(),
		newShellSetupCmd(),
		newVersionCmd(),
		newListCmd(),
		newStandardsCmd(),
		newSkillCmd(),
		newAgentSetupCmd(),
		newHelpDumpCmd(),
	)
	return cmd
}
