package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

// agent_setup.go implements `shll agent-setup` — mechanically place ONE thin Agent
// Skill (the toolkit bootstrap) into the harnesses' global skills directories, then
// delegate run-kit's dashboard-hook wiring to `run-kit agent-setup` (Constitution
// III/IV — compose, don't absorb). It graduates the cross-toolkit harness wiring from
// run-kit (a leaf tool) to shll (the manager).
//
// The skill directories are shll-OWNED, so there is no merge, no sentinel, no
// diff-and-confirm: install = write, re-run/upgrade = overwrite (idempotent by
// construction), --uninstall = delete. This file performs plain file I/O plus ONE
// subprocess (the run-kit delegation via internal/proc — Constitution I).

// agentSetupErrPrefix is the diagnostic prefix stamped on this command's stderr.
const agentSetupErrPrefix = "shll agent-setup"

// skillDirName is the Agent-Skill directory (and `name:` frontmatter value) placed at
// each target. It satisfies the agentskills.io portable-name rule
// (^[a-z0-9]+(-[a-z0-9]+)*$) and MUST equal the frontmatter `name:` — the same string
// is used for both so they cannot drift. Named per code-quality.md (no magic strings).
const skillDirName = "sahil87-toolkit"

// skillFileName is the SKILL.md filename the Agent Skills open standard requires
// inside each skill directory (<dir>/<name>/SKILL.md).
const skillFileName = "SKILL.md"

// agentSkillContent is the canonical bytes of the placed SKILL.md — the toolkit
// bootstrap skill. Portable frontmatter carries `name` + `description` ONLY (the
// OpenCode-recognized common subset, valid on all four harnesses); `name` equals
// skillDirName. The description front-loads trigger words (the tool names) for
// implicit activation; the body teaches the runtime two-step plus one `shll standards`
// pointer. This is an agent-setup artifact (neither a published standard nor a
// `<tool> skill` bundle), so it lives as an inline constant, not a docs-site file.
const agentSkillContent = `---
name: ` + skillDirName + `
description: Use when driving any sahil87 toolkit CLI — wt, idea, tu, run-kit (rk), hop, fab-kit, or shll itself. Run ` + "`shll skill`" + ` to list the installed tools; run ` + "`shll skill <tool>`" + ` for that tool's full usage bundle before using it.
---
# sahil87 toolkit

This machine has the sahil87 toolkit installed. Before driving one of its tools:

1. ` + "`shll skill`" + ` — the installed tools, one line each
2. ` + "`shll skill <tool>`" + ` — that tool's full agent skill bundle (when to use it,
   composition patterns, output and exit-code contracts, gotchas)

For toolkit-repo development, ` + "`shll standards`" + ` enumerates the binding CLI standards.
`

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

// runKitAgentSetupSub is the run-kit subcommand shll delegates hook wiring to.
const runKitAgentSetupSub = "agent-setup"

func newAgentSetupCmd() *cobra.Command {
	var (
		printMode     bool
		uninstallMode bool
	)
	cmd := &cobra.Command{
		Use:   "agent-setup",
		Short: "place the sahil87 toolkit skill for agent harnesses",
		Long: `Mechanically place one thin Agent Skill — the sahil87 toolkit bootstrap — into the
agent harnesses' global skills directories, then delegate run-kit's dashboard-hook
wiring to ` + "`run-kit agent-setup`" + `. The skill teaches an agent to load ` + "`shll skill`" + ` before
driving a toolkit tool.

The skill is written to exactly two global locations (covering all four harnesses):
  ~/.agents/skills/` + skillDirName + `/SKILL.md   Codex (USER scope), Cursor + OpenCode
  ~/.claude/skills/` + skillDirName + `/SKILL.md   Claude Code

The skill directories are shll-owned, so placement is idempotent by construction —
install writes them, a re-run overwrites them, and there is no merge, prompt, or
sentinel machinery. A per-path written/updated/unchanged summary is printed.

Modes:
  shll agent-setup             place the skill at both locations (overwrites; idempotent)
  shll agent-setup --print     print the SKILL.md content and both target paths, write nothing
  shll agent-setup --uninstall remove both placed skill directories`,
		SilenceUsage:  true,
		SilenceErrors: true,
		Args:          cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runAgentSetup(cmd.Context(), os.Getenv, cmd.OutOrStdout(), cmd.ErrOrStderr(), printMode, uninstallMode)
		},
	}
	cmd.Flags().BoolVar(&printMode, "print", false, "print the SKILL.md content and target paths, do not write any file")
	cmd.Flags().BoolVar(&uninstallMode, "uninstall", false, "remove both placed sahil87-toolkit skill directories")
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
func runAgentSetup(ctx context.Context, env func(string) string, stdout, stderr io.Writer, printMode, uninstallMode bool) error {
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
		return runAgentUninstall(ctx, targets, stdout, stderr)
	}
	return runAgentInstall(ctx, targets, stdout, stderr)
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
func runAgentInstall(ctx context.Context, targets []string, stdout, stderr io.Writer) error {
	content := []byte(agentSkillContent)
	anyFailed := false
	for _, path := range targets {
		if err := placeSkill(path, content, stdout, stderr); err != nil {
			anyFailed = true
		}
	}

	// Delegate run-kit's harness hooks (Constitution III/IV). Skip silently when
	// run-kit is absent (Constitution V).
	delegateRunKitAgentSetup(ctx, false, stderr)

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
		if bytesEqual(existing, content) {
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

// runAgentUninstall removes each placed skill DIRECTORY (the sahil87-toolkit dir under
// each target, not just the SKILL.md file), then delegates `run-kit agent-setup
// --uninstall`. Removing an shll-owned directory is safe and needs no confirmation.
func runAgentUninstall(ctx context.Context, targets []string, stdout, stderr io.Writer) error {
	anyFailed := false
	for _, path := range targets {
		dir := filepath.Dir(path) // .../sahil87-toolkit
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
	delegateRunKitAgentSetup(ctx, true, stderr)

	if anyFailed {
		return errSilent
	}
	return nil
}

// bytesEqual reports whether a and b hold identical bytes. A tiny local helper so
// placeSkill's idempotency compare reads clearly without importing bytes just for one
// call (mirrors the file's file-I/O-only footprint).
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// delegateRunKitAgentSetup invokes `run-kit agent-setup [--uninstall]` as a foreground
// subprocess (via internal/proc — Constitution I) for run-kit's dashboard hooks. When
// run-kit is not on PATH (proc.ErrNotFound) the delegation is skipped silently
// (Constitution V — graceful degradation); its stdio is inherited (proc.RunForeground
// always wires the real os.Stdout/os.Stderr) so the user sees run-kit's own output —
// this helper only writes its own diagnostics to stderr, so it takes no stdout writer.
// Only the default (install) and --uninstall paths call this; --print never does.
func delegateRunKitAgentSetup(ctx context.Context, uninstall bool, stderr io.Writer) {
	args := []string{runKitAgentSetupSub}
	if uninstall {
		args = append(args, "--uninstall")
	}
	_, err := proc.RunForeground(ctx, runKitToolName, args...)
	if errors.Is(err, proc.ErrNotFound) {
		return // run-kit absent — skip silently.
	}
	if err != nil {
		// A real delegation error (not "absent") is worth surfacing, but it does not
		// fail the skill placement shll already did — placement is agent-setup's core
		// work; run-kit hooks are the optional adjunct.
		fmt.Fprintf(stderr, "%s: run-kit agent-setup: %v (continuing)\n", agentSetupErrPrefix, err)
	}
}
