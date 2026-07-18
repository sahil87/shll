package main

import (
	"context"
	"embed"
	"errors"
	"fmt"
	"io"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"github.com/sahil87/shll/internal/proc"
)

//go:generate ../../../scripts/sync-standards.sh

// skillFS holds shll's OWN agent skill bundle (docs/site/skill.md), copied into
// this package dir by scripts/sync-standards.sh and embedded at build time — the
// same sync + drift-guard mechanism `shll standards` uses (the Go module root is
// src/ and docs/site/ sits above it, so //go:embed cannot reach the canonical file
// directly). `shll skill shll` serves this in-process; a subprocess self-invocation
// would recurse into the composer. TestSkillEmbedMatchesCanonical keeps the embedded
// copy byte-honest against docs/site/skill.md on every `go test`.
//
//go:embed skill/skill.md
var skillFS embed.FS

// skillEmbedPath is the embed path of shll's own bundle under this package dir.
// Named constant so the embed-path composition is single-sourced (code-quality.md).
const skillEmbedPath = "skill/skill.md"

// skillSubcommand is the literal subcommand name shll invokes on each tool for the
// per-tool passthrough (`<tool> skill`) — the toolkit's `skill` standard command.
const skillSubcommand = "skill"

// skillProbeTimeout bounds each `<tool> skill` subprocess. A static bundle prints
// fast, so a small deadline caps a hung tool without truncating real output —
// mirroring `version`'s per-tool --version probe bound (versionTimeout). Named per
// code-quality.md (no magic numbers).
const skillProbeTimeout = 2 * time.Second

// skillHintLine is the trailing line the bare glossary prints, teaching the second
// step (per-tool bundle on demand). Named per code-quality.md.
const skillHintLine = "Run 'shll skill <tool>' for that tool's full agent skill bundle."

// skillUnsupportedFmt is the one-line stderr notice for a valid tool whose installed
// version predates `skill` (the `<tool> skill` subprocess exits non-zero). Takes the
// tool name. Operational (exit 1), not usage.
const skillUnsupportedFmt = "shll skill: %s does not support 'skill' yet — run 'shll update'"

// skillNotInstalledFmt is the one-line stderr notice for a valid tool that is not on
// PATH (proc.ErrNotFound). Takes the tool name. Operational (exit 1).
const skillNotInstalledFmt = "shll skill: %s is not installed — run 'shll install %s'"

func newSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill [tool]",
		Short: "read the agent skill bundle for a sahil87 tool (or list installed tools)",
		Long: `Read the offline agent skill bundle for a sahil87 tool — the one-page usage
briefing an agent loads before driving the tool (per the toolkit's ` + "`skill`" + ` standard).

Bare ` + "`shll skill`" + ` prints a one-line glossary of the installed tools (shll first,
then the roster) — NOT a dump of every bundle. Pick one and ask for it by name:

  shll skill          list installed tools, one line each
  shll skill <tool>   print that tool's full agent skill bundle (raw markdown, stdout)

` + "`shll skill <tool>`" + ` streams the tool's own ` + "`<tool> skill`" + ` output byte-for-byte
(` + "`shll skill shll`" + ` serves shll's own bundle from an embedded copy). A tool that is
not installed, or whose version predates its ` + "`skill`" + ` subcommand, prints a one-line
notice to stderr and exits 1; an unknown tool name is a usage error (exit 2).`,
		Args: cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkill(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args)
		},
	}
}

// runSkill is the implementation seam for `shll skill`, extracted from the cobra
// factory so skill_test.go can drive it with bytes.Buffers and a fake proc.Runner.
// No args → the installed-only glossary; one arg → that tool's bundle (shll served
// in-process, a roster tool via a byte-identical `<tool> skill` passthrough).
func runSkill(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	if len(args) == 0 {
		return writeSkillGlossary(ctx, stdout)
	}
	return writeSkillBundle(ctx, stdout, stderr, args[0])
}

// writeSkillGlossary prints one line per INSTALLED tool — shll first (always present,
// using shllSelf), then the Roster in leaves-first order, each filtered by the shared
// PATH-only toolInstalled probe (no brew calls — the glossary stays cheap). It never
// concatenates bundles: the two-step "list, then per-tool on demand" is the deliberate
// context-economy contract. A trailing hint teaches the second step. Column-aligned
// via the same tabwriter config as `shll version` / `shll list`.
func writeSkillGlossary(ctx context.Context, stdout io.Writer) error {
	tw := tabwriter.NewWriter(stdout, 0, 0, 2, ' ', 0)
	// shll first — it is the running binary, so always present.
	fmt.Fprintf(tw, "%s\t%s\n", shllSelf.Name, shllSelf.Description)
	for _, tool := range Roster {
		if toolInstalled(ctx, tool) {
			fmt.Fprintf(tw, "%s\t%s\n", tool.Name, tool.Description)
		}
	}
	if err := tw.Flush(); err != nil {
		return fmt.Errorf("shll skill: write: %w", err)
	}
	fmt.Fprintf(stdout, "\n%s\n", skillHintLine)
	return nil
}

// writeSkillBundle serves one tool's bundle. It resolves `name` against the Roster
// (inheriting the `rk` → `run-kit` legacy alias via the shared resolver) plus the
// `shll` self-token:
//   - unknown name → actionable stderr diagnostic + errExitCode{code: 2} (usage).
//   - `shll` self → serve the embedded bundle in-process, byte-identical (a subprocess
//     self-invocation would recurse into the composer).
//   - a Roster tool → invoke `<tool> skill` via proc.RunCaptured (bounded), stream its
//     stdout byte-identical on success; on ErrNotFound / non-zero exit write ONE
//     stderr notice + errSilent (exit 1), suppressing the child's own raw stderr.
func writeSkillBundle(ctx context.Context, stdout, stderr io.Writer, name string) error {
	// Legacy alias (rk → run-kit) via the same map resolveTargets consults, so skill
	// carries no bespoke alias logic (intake: name-matching reuses the shared helper).
	if canonical, ok := legacyAliases[name]; ok && rosterHas(canonical) {
		name = canonical
	}

	if name == shllTargetToken {
		return writeShllOwnBundle(stdout, stderr)
	}

	tool, ok := rosterTool(name)
	if !ok {
		// Unknown name → usage error, exit 2 (the conformance exit-code convention).
		// validTargets(true) includes shll, which IS a valid skill target here.
		return &errExitCode{code: usageExitCode, msg: fmt.Sprintf("shll skill: unknown tool %q (valid: %s)", name, validTargets(true))}
	}

	subCtx, cancel := context.WithTimeout(ctx, skillProbeTimeout)
	defer cancel()
	out, _, code, err := proc.RunCaptured(subCtx, tool.Name, skillSubcommand)
	if errors.Is(err, proc.ErrNotFound) {
		// Not on PATH — operational, exit 1. One line; the tool's own error (if any)
		// is captured (not passed through) so only this notice reaches the user.
		fmt.Fprintf(stderr, skillNotInstalledFmt+"\n", tool.Name, tool.Name)
		return errSilent
	}
	if err != nil {
		// A pre-start I/O failure with no usable exit code — operational, exit 1.
		fmt.Fprintf(stderr, "shll skill: %s: %v\n", tool.Name, err)
		return errSilent
	}
	if code != 0 {
		// Ran to completion but failed — the installed version predates `skill`
		// (unknown-command), so it exited non-zero. Its captured stderr is suppressed
		// in favor of this single actionable notice.
		fmt.Fprintf(stderr, skillUnsupportedFmt+"\n", tool.Name)
		return errSilent
	}
	// Success — byte-identical passthrough of the tool's stdout (no framing, no
	// rendering; stdout is data per the skill standard's invocation contract).
	if _, werr := stdout.Write(out); werr != nil {
		fmt.Fprintf(stderr, "shll skill: %s: write: %v\n", tool.Name, werr)
		return errSilent
	}
	return nil
}

// writeShllOwnBundle serves shll's own embedded bundle (docs/site/skill.md) in-process,
// byte-identical to the committed embed copy. A read failure is a build-integrity bug
// (the sync step / drift guard should have caught it), not user error.
func writeShllOwnBundle(stdout, stderr io.Writer) error {
	data, err := skillFS.ReadFile(skillEmbedPath)
	if err != nil {
		return fmt.Errorf("shll skill: read embedded bundle: %w", err)
	}
	if _, err := stdout.Write(data); err != nil {
		fmt.Fprintf(stderr, "shll skill: write: %v\n", err)
		return errSilent
	}
	return nil
}
