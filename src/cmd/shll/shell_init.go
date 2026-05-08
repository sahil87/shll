package main

import (
	"context"
	"fmt"
	"io"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

// supportedShells lists the shells `shll shell-init` accepts. Matches hop's
// supported set (Constitution: graceful degradation; spec: zsh and bash only).
var supportedShells = []string{"zsh", "bash"}

func newShellInitCmd() *cobra.Command {
	return &cobra.Command{
		Use:           "shell-init <shell>",
		Short:         "emit composed shell-init for all installed sahil87 tools",
		SilenceUsage:  true,
		SilenceErrors: true,
		Long: `Emit a single concatenated shell-init blob for every installed sahil87 tool
that exposes shell integration.

Today, hop and wt are the only roster tools with shell integration. The output
is eval-safe: missing tools produce no output, errors go to stderr, and stdout
is shell code only.

Use:  eval "$(shll shell-init zsh)"   # in your ~/.zshrc
      eval "$(shll shell-init bash)"  # in your ~/.bashrc`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if len(args) == 0 {
				return &errExitCode{code: 2, msg: "shll shell-init: missing shell. Supported: zsh, bash"}
			}
			shell := args[0]
			if !isSupportedShell(shell) {
				return &errExitCode{code: 2, msg: fmt.Sprintf("shll shell-init: unsupported shell %q. Supported: zsh, bash", shell)}
			}
			return runShellInit(cmd.Context(), shell, cmd.OutOrStdout(), cmd.ErrOrStderr())
		},
	}
}

// isSupportedShell returns whether the named shell is one of the supported
// shells (zsh or bash). Defined as a function rather than a map so the supported
// list stays inline at the call site (no closure capture in tests).
func isSupportedShell(shell string) bool {
	for _, s := range supportedShells {
		if s == shell {
			return true
		}
	}
	return false
}

// runShellInit composes shell-init output from every installed roster tool
// with a non-empty ShellInit argv. Per spec:
//   - stdout is eval-safe even when sub-tools are missing (missing → no output).
//   - stderr receives a single line per sub-tool that fails.
//   - exit code is non-zero if any sub-tool's shell-init failed.
//
// Order is roster order (deterministic — spec requirement).
func runShellInit(ctx context.Context, shell string, stdout, stderr io.Writer) error {
	if ctx == nil {
		ctx = context.Background()
	}
	anyFailed := false
	for _, tool := range Roster {
		if len(tool.ShellInit) == 0 {
			continue
		}
		if !isInstalled(ctx, tool.Formula) {
			// Graceful degradation — uninstalled means no output, no error.
			continue
		}
		argv := substituteShell(tool.ShellInit, shell)
		out, err := proc.Run(ctx, argv[0], argv[1:]...)
		if err != nil {
			fmt.Fprintf(stderr, "shll shell-init: %s: %v\n", tool.Name, err)
			anyFailed = true
			continue
		}
		if _, werr := stdout.Write(out); werr != nil {
			return fmt.Errorf("shll shell-init: write: %w", werr)
		}
	}
	if anyFailed {
		return errSilent
	}
	return nil
}

// substituteShell returns a copy of argv with every shellPlaceholder token
// replaced by shell. Tools without the placeholder (e.g. wt shell-setup) come
// through unchanged.
func substituteShell(argv []string, shell string) []string {
	out := make([]string, len(argv))
	for i, a := range argv {
		if a == shellPlaceholder {
			out[i] = shell
		} else {
			out[i] = a
		}
	}
	return out
}
