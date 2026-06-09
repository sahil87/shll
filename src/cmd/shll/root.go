package main

import (
	"github.com/spf13/cobra"
)

const rootLong = `shll — meta-CLI for the sahil87 toolkit.

shll composes operations that span every per-tool CLI (hop, wt, fab-kit, rk, tu, idea)
so you have one entry point for cross-toolkit concerns.

Subcommands:
  shll install                brew install every sahil87 tool that isn't already installed
  shll update                 brew update + brew upgrade for shll and every installed sahil87 tool
  shll shell-init <shell>     emit a single eval-safe shell-init blob for all installed tools
  shll shell-setup [shell]    append the shell-init eval line to your rc file (idempotent)
  shll version                print versions of shll and every installed sahil87 tool
  shll list                   list the managed sahil87 tools with install status and repo links

Per-tool CLIs continue to work standalone — shll wraps them, it does not replace them.`

func newRootCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:           "shll",
		Short:         "meta-CLI for the sahil87 toolkit",
		Long:          rootLong,
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	cmd.AddCommand(
		newInstallCmd(),
		newUpdateCmd(),
		newShellInitCmd(),
		newShellSetupCmd(),
		newVersionCmd(),
		newListCmd(),
		newHelpDumpCmd(),
	)
	return cmd
}
