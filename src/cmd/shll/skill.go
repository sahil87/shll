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
// step (per-tool bundle on demand) and the third (a topic page). Named per
// code-quality.md; kept to a single line — it trails the tabwriter table after a blank line.
const skillHintLine = "Run 'shll skill <tool>' for that tool's full agent skill bundle ('shll skill <tool> <topic>' for a topic page)."

// skillNoTopicsFmt is the one-line stderr notice for `shll skill shll <topic>`: shll
// ships zero topic pages today. Served in-process (a self-invocation would recurse into
// the composer), usage exit 2 — matching the unknown-tool usage convention (the `skill`
// standard requires only a non-zero exit with the valid topics on stderr; the valid set
// is empty). Takes the requested topic name. Named per code-quality.md (no magic strings).
const skillNoTopicsFmt = "shll skill: shll ships no topic pages (unknown topic %q)"

// skillUnsupportedFmt is the one-line stderr notice for a valid tool whose skill
// invocation ran to completion but exited non-zero — most likely an installed
// version predating `skill`. Hedged rather than imperative (the tool may already be
// current, in which case updating changes nothing), and the update suggestion is
// scoped to the one tool. Takes the tool name twice. Operational (exit 1), not usage.
const skillUnsupportedFmt = "shll skill: %s does not support 'skill' — its installed version may predate it (try 'shll update %s')"

// skillNotInstalledFmt is the one-line stderr notice for a valid tool that is not on
// PATH (proc.ErrNotFound). Takes the tool name. Operational (exit 1).
const skillNotInstalledFmt = "shll skill: %s is not installed — run 'shll install %s'"

// skillTopicTimeoutFmt is the one-line stderr notice for a two-arg topic passthrough
// whose `<tool> skill <topic>` child never produced a usable exit code: it was killed
// by the skillProbeTimeout deadline (or another signal), which proc.RunCaptured surfaces
// as code -1 with nil err (Go's *exec.ExitError.ExitCode() signal sentinel). Mirroring
// that -1 into the exit code would wrap to process exit 255 with no diagnostic, so this
// case is treated as operational (exit 1) with a curated notice instead. Takes the tool
// name then the topic. Named per code-quality.md (no magic strings).
const skillTopicTimeoutFmt = "shll skill: %s skill %s timed out or was killed — re-run, and check the tool if it persists"

// skillArgv returns the argv of the tool's skill-bundle invocation: the roster
// Skill override when set (fab-kit → {"fab", "skill"} — its `skill` subcommand
// lives on the `fab` router binary), else the default {Name, skillSubcommand}.
// Always a FRESH slice — the override branch copies tool.Skill so no caller can
// mutate the shared Roster entry through the returned argv (the same aliasing
// concern writeSkillTopic guards its topic append against). Single-sourced here
// so neither passthrough open-codes the argv composition.
func skillArgv(tool Tool) []string {
	if len(tool.Skill) > 0 {
		return append([]string(nil), tool.Skill...)
	}
	return []string{tool.Name, skillSubcommand}
}

func newSkillCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "skill [tool] [topic]",
		Short: "read the agent skill bundle for a shll tool (or list installed tools)",
		Long: `Read the offline agent skill bundle for a shll tool — the one-page usage
briefing an agent loads before driving the tool (per the toolkit's ` + "`skill`" + ` standard).

Bare ` + "`shll skill`" + ` prints a one-line glossary of the installed tools (shll first,
then the roster) — NOT a dump of every bundle. Pick one and ask for it by name:

  shll skill                 list installed tools, one line each
  shll skill <tool>          print that tool's full agent skill bundle (raw markdown, stdout)
  shll skill <tool> <topic>  print one of that tool's topic pages (large-scope tools)

` + "`shll skill <tool>`" + ` streams the tool's own ` + "`<tool> skill`" + ` output byte-for-byte
(` + "`shll skill shll`" + ` serves shll's own bundle from an embedded copy). A tool that is
not installed, or whose version predates its ` + "`skill`" + ` subcommand, prints a one-line
notice to stderr and exits 1; an unknown tool name is a usage error (exit 2).

` + "`shll skill <tool> <topic>`" + ` delegates to ` + "`<tool> skill <topic>`" + ` verbatim (a tool's
core bundle lists its topics). On success it streams the topic page byte-for-byte. On a
child failure it propagates the child's own stderr and exit code UNCHANGED — so an unknown
topic surfaces the tool's own diagnostic (valid topics on stderr, non-zero exit), not a
shll-rewritten one. shll ships no topics of its own, so ` + "`shll skill shll <topic>`" + ` is a
usage error (exit 2).`,
		Args: cobra.MaximumNArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			return runSkill(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(), args)
		},
	}
}

// runSkill is the implementation seam for `shll skill`, extracted from the cobra
// factory so skill_test.go can drive it with bytes.Buffers and a fake proc.Runner.
// No args → the installed-only glossary; one arg → that tool's bundle (shll served
// in-process, a roster tool via a byte-identical `<tool> skill` passthrough); two args
// → that tool's topic page (a byte-identical `<tool> skill <topic>` passthrough whose
// failures are propagated, not rewrapped — see writeSkillTopic). cobra caps args at 2.
func runSkill(ctx context.Context, stdout, stderr io.Writer, args []string) error {
	if ctx == nil {
		ctx = context.Background()
	}
	switch len(args) {
	case 0:
		return writeSkillGlossary(ctx, stdout)
	case 1:
		return writeSkillBundle(ctx, stdout, stderr, args[0])
	default:
		return writeSkillTopic(ctx, stdout, stderr, args[0], args[1])
	}
}

// writeSkillGlossary prints one line per INSTALLED tool — shll first (always present,
// using shllSelf), then the Roster in roster order, each filtered by the shared
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
//   - a Roster tool → invoke its resolved skill argv (skillArgv: the roster Skill
//     override, else `<tool.Name> skill`) via proc.RunCaptured (bounded), stream its
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

	// Resolve the skill argv from the roster (fab-kit → `fab skill`; default
	// `<tool.Name> skill`). Notices below keep printing tool.Name — the
	// user-facing name is the roster name regardless of which binary serves it.
	argv := skillArgv(tool)
	subCtx, cancel := context.WithTimeout(ctx, skillProbeTimeout)
	defer cancel()
	out, _, code, err := proc.RunCaptured(subCtx, argv[0], argv[1:]...)
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
		// Ran to completion but failed — most likely the installed version predates
		// `skill` (unknown-command), so it exited non-zero. Its captured stderr is
		// suppressed in favor of this single hedged notice (tool name twice: once
		// naming the tool, once scoping the update suggestion to it).
		fmt.Fprintf(stderr, skillUnsupportedFmt+"\n", tool.Name, tool.Name)
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

// writeSkillTopic serves one of a tool's topic pages via a byte-identical
// `<tool> skill <topic>` passthrough (argv resolved by skillArgv, so fab-kit's
// topics route through `fab skill <topic>`). Tool-arg resolution mirrors writeSkillBundle
// (legacy alias → shll self-token → roster → unknown), but its FAILURE classification
// deliberately DIVERGES for a child that RAN TO COMPLETION with a positive exit code:
// it propagates the child's own stderr and exit code verbatim (the `skill` standard's
// unknown-topic contract — "non-zero exit with the valid topics on stderr" — must
// survive the composer unmodified), rather than suppressing the child's stderr in favor
// of a curated notice the way the one-arg form does. A child that never ran to
// completion is NOT propagated: proc.RunCaptured reports a deadline/signal kill as code
// -1 with nil err (not a real exit status), so that case gets a curated operational
// exit-1 notice (mirroring -1 would wrap to process exit 255 with no diagnostic). shll
// ships no topics of its own, so `shll skill shll <topic>` is a usage error served
// in-process (a subprocess self-invocation would recurse into the composer).
func writeSkillTopic(ctx context.Context, stdout, stderr io.Writer, name, topic string) error {
	// Legacy alias (rk → run-kit) via the same map resolveTargets consults, applied
	// before any dispatch so a topic invocation targets the canonical binary.
	if canonical, ok := legacyAliases[name]; ok && rosterHas(canonical) {
		name = canonical
	}

	if name == shllTargetToken {
		// shll ships zero topic pages — usage error, exit 2, no subprocess. The standard
		// requires only a non-zero exit with the valid topics on stderr; the valid set is
		// empty, so one line naming the unknown topic is the honest diagnostic.
		fmt.Fprintf(stderr, skillNoTopicsFmt+"\n", topic)
		return &errExitCode{code: usageExitCode}
	}

	tool, ok := rosterTool(name)
	if !ok {
		// Unknown name → usage error, exit 2 — same diagnostic as the one-arg form,
		// checked before any subprocess. validTargets(true) includes shll.
		return &errExitCode{code: usageExitCode, msg: fmt.Sprintf("shll skill: unknown tool %q (valid: %s)", name, validTargets(true))}
	}

	// Resolve the skill argv from the roster (fab-kit → `fab skill <topic>`;
	// default `<tool.Name> skill <topic>`). The topic is appended onto a COPY of
	// the resolved tail — never `append(argv[1:], topic)` directly. skillArgv
	// already returns a fresh slice, but the explicit copy keeps this call site
	// safe on its own terms, independent of that guarantee.
	argv := skillArgv(tool)
	args := append(append([]string(nil), argv[1:]...), topic)
	subCtx, cancel := context.WithTimeout(ctx, skillProbeTimeout)
	defer cancel()
	out, childErr, code, err := proc.RunCaptured(subCtx, argv[0], args...)
	if errors.Is(err, proc.ErrNotFound) {
		// Not on PATH — operational, exit 1. Classified BEFORE the exit-code question
		// (there is no usable child exit code), so it keeps the curated one-line notice.
		fmt.Fprintf(stderr, skillNotInstalledFmt+"\n", tool.Name, tool.Name)
		return errSilent
	}
	if err != nil {
		// A pre-start I/O failure with no usable exit code — operational, exit 1.
		fmt.Fprintf(stderr, "shll skill: %s: %v\n", tool.Name, err)
		return errSilent
	}
	if code < 0 {
		// No usable child exit code despite a nil err: proc.RunCaptured surfaces a
		// deadline/signal kill as code -1 (Go's *exec.ExitError.ExitCode() sentinel),
		// NOT a real exit status. Mirroring -1 would wrap to process exit 255 with no
		// diagnostic, so treat it as operational (exit 1) with a curated notice. R5
		// scopes verbatim mirroring to a child that RAN TO COMPLETION — this did not.
		fmt.Fprintf(stderr, skillTopicTimeoutFmt+"\n", tool.Name, topic)
		return errSilent
	}
	if code > 0 {
		// Ran to completion but failed (an unknown topic, or a version predating the
		// topic — indistinguishable from exit codes, and stderr-sniffing is fragile).
		// PROPAGATE, do not rewrap: write the child's captured stderr bytes verbatim and
		// mirror the child's own exit code. errExitCode with an empty msg exits with the
		// code and writes nothing further, so the child's stderr is the only diagnostic.
		// A write failure here can only be swallowed — stderr is already the broken sink,
		// so re-diagnosing to it would be futile; still mirror the child's own code.
		_, _ = stderr.Write(childErr)
		return &errExitCode{code: code}
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
