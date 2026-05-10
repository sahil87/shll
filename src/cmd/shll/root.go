package main

import (
	"github.com/spf13/cobra"
)

const rootLong = `shll — meta-CLI for the sahil87 toolkit.

shll composes operations that span every per-tool CLI (hop, wt, fab-kit, rk, tu, idea)
so you have one entry point for cross-toolkit concerns.

Subcommands:
  shll update                 brew update + brew upgrade for every installed sahil87 tool
  shll shell-init <shell>     emit a single eval-safe shell-init blob for all installed tools
  shll shell-install [shell]  append the shell-init eval line to your rc file (idempotent)
  shll version                print versions of shll and every installed sahil87 tool

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
		newUpdateCmd(),
		newShellInitCmd(),
		newShellInstallCmd(),
		newVersionCmd(),
	)
	return cmd
}
